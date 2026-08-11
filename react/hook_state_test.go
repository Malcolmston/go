package react

import (
	"strings"
	"testing"
	"unsafe"
)

// stateTreeDump renders a fiber subtree as a compact S-expression, so a test can
// assert on the whole rendered shape with one string comparison. It is a private
// copy of the engine test's dumper rather than a shared helper: these tests are
// about hook values, and keeping the formatter next to them means a change to
// either file's expectations cannot silently break the other.
func stateTreeDump(f *fiber) string {
	var b strings.Builder
	var write func(*fiber)
	write = func(f *fiber) {
		switch t := f.typ.(type) {
		case string:
			b.WriteString("(" + t)
			if f.key != "" {
				b.WriteString("#" + f.key)
			}
			for c := f.child; c != nil; c = c.sibling {
				b.WriteString(" ")
				write(c)
			}
			b.WriteString(")")
		case textTag:
			s, _ := f.props[textValueKey].(string)
			b.WriteString(`"` + s + `"`)
		default:
			_ = t // fragments and components are transparent in the dump
			first := true
			for c := f.child; c != nil; c = c.sibling {
				if !first {
					b.WriteString(" ")
				}
				first = false
				write(c)
			}
		}
	}
	first := true
	for c := f.child; c != nil; c = c.sibling {
		if !first {
			b.WriteString(" ")
		}
		first = false
		write(c)
	}
	return b.String()
}

// stateHostEl builds a host element directly, keeping these tests independent of
// the element-construction layer.
func stateHostEl(tag, key string, children ...Node) *Element {
	return &Element{Type: tag, Key: key, Props: Props{ChildrenKey: children}}
}

// stateCompEl builds a component element with the given props.
func stateCompEl(c Component, key string, props Props) *Element {
	if props == nil {
		props = Props{}
	}
	return &Element{Type: c, Key: key, Props: props}
}

// stateSetterID reports the identity of a setter closure.
//
// It cannot be done with reflect: reflect.Value.Pointer returns a func value's
// *code* pointer, which is identical for every closure created from the same
// function literal, so it answers "same source line", not "same value". Go also
// refuses to compare func values with == at all. What does distinguish them is
// the closure record each one points at, and a func value is represented as a
// pointer to that record — so reading the word behind the variable yields an
// address that differs for two distinct closures and matches for one reused.
// This is a test-only white-box probe of a real guarantee, not a pattern for
// production code.
func stateSetterID[F any](fn F) uintptr {
	return uintptr(*(*unsafe.Pointer)(unsafe.Pointer(&fn)))
}

func TestUseStateMountAndUpdate(t *testing.T) {
	var set SetStateFunc[int]
	renders := 0

	var counter Component = func(Props) Node {
		renders++
		n, s := UseState(0)
		set = s
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", nil))
	if got, want := stateTreeDump(root.tree()), `(b "0")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	set(7)
	if got, want := stateTreeDump(root.tree()), `(b "7")`; got != want {
		t.Errorf("after set tree = %s, want %s", got, want)
	}

	set.Update(func(prev int) int { return prev * 3 })
	if got, want := stateTreeDump(root.tree()), `(b "21")`; got != want {
		t.Errorf("after Update tree = %s, want %s", got, want)
	}

	if renders != 3 {
		t.Errorf("renders = %d, want 3 (mount, set, Update)", renders)
	}
}

func TestUseStateSetterIdentityIsStable(t *testing.T) {
	var ids []uintptr

	var counter Component = func(Props) Node {
		n, set := UseState(0)
		ids = append(ids, stateSetterID(set))
		if n < 2 {
			// Grab the setter for the next update from the render body's local,
			// so each pass genuinely re-runs the hook.
			defer func() { set.Update(func(p int) int { return p + 1 }) }()
		}
		return nil
	}

	NewRoot(stateCompEl(counter, "", nil))

	if len(ids) < 3 {
		t.Fatalf("got %d renders, want at least 3", len(ids))
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Fatalf("setter identity changed on render %d: %#x != %#x", i, id, ids[0])
		}
	}
}

func TestUseStateSetterDoesNotCaptureStaleState(t *testing.T) {
	// A setter grabbed on the very first render must still see the latest state
	// when it is used much later. This is the property that lets a setter be
	// stored in a struct, a timer or an effect closure without going stale.
	var firstRenderSetter SetStateFunc[int]
	captured := false

	var counter Component = func(Props) Node {
		n, set := UseState(0)
		if !captured {
			firstRenderSetter = set
			captured = true
		}
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", nil))

	firstRenderSetter(10)
	firstRenderSetter.Update(func(prev int) int { return prev + 5 })
	firstRenderSetter.Update(func(prev int) int { return prev + 5 })

	if got, want := stateTreeDump(root.tree()), `(b "20")`; got != want {
		t.Errorf("tree = %s, want %s (the mount-time setter must read the latest state)", got, want)
	}
}

func TestUseStateBatchedUpdatesQueueInOrder(t *testing.T) {
	var set SetStateFunc[int]
	renders := 0

	var counter Component = func(Props) Node {
		renders++
		n, s := UseState(0)
		set = s
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", nil))
	inc := func(prev int) int { return prev + 1 }

	cases := []struct {
		name string
		run  func()
		want string
	}{
		{
			// The headline difference between the two forms: updaters compose,
			// direct sets do not.
			name: "two updaters accumulate",
			run:  func() { set.Update(inc); set.Update(inc) },
			want: `(b "2")`,
		},
		{
			name: "direct sets keep the last one",
			run:  func() { set(100); set(200) },
			want: `(b "200")`,
		},
		{
			name: "a direct set resets the chain, updaters resume from it",
			run:  func() { set.Update(inc); set(0); set.Update(inc); set.Update(inc) },
			want: `(b "2")`,
		},
	}

	for _, c := range cases {
		renders = 0
		Batch(c.run)
		if got := stateTreeDump(root.tree()); got != c.want {
			t.Errorf("%s: tree = %s, want %s", c.name, got, c.want)
		}
		if renders != 1 {
			t.Errorf("%s: renders = %d, want 1 (a batch renders once)", c.name, renders)
		}
	}
}

func TestUseStateSameValueBailsOut(t *testing.T) {
	var set SetStateFunc[string]
	renders := 0

	var label Component = func(Props) Node {
		renders++
		s, setter := UseState("hello")
		set = setter
		return stateHostEl("p", "", s)
	}

	root := NewRoot(stateCompEl(label, "", nil))
	if renders != 1 {
		t.Fatalf("renders after mount = %d, want 1", renders)
	}

	set("hello") // identical value: React bails out rather than re-rendering
	if renders != 1 {
		t.Errorf("renders after no-op set = %d, want 1", renders)
	}

	set.Update(func(prev string) string { return prev }) // same, via the updater form
	if renders != 1 {
		t.Errorf("renders after no-op Update = %d, want 1", renders)
	}

	set("world")
	if renders != 2 {
		t.Errorf("renders after a real set = %d, want 2", renders)
	}
	if got, want := stateTreeDump(root.tree()), `(p "world")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseStateLazyInitRunsOnceOnMount(t *testing.T) {
	inits := 0
	var set SetStateFunc[int]

	var counter Component = func(Props) Node {
		n, s := UseStateLazy(func() int {
			inits++
			return 100
		})
		set = s
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", nil))
	set(1)
	set(2)
	root.Render(stateCompEl(counter, "", Props{"unused": true}))

	if inits != 1 {
		t.Errorf("init calls = %d, want 1 (mount only)", inits)
	}
	if got, want := stateTreeDump(root.tree()), `(b "2")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseStateSurvivesParentRerender(t *testing.T) {
	var set SetStateFunc[int]

	var child Component = func(Props) Node {
		n, s := UseState(0)
		set = s
		return stateHostEl("b", "", n)
	}
	parent := func(title string) Node {
		return stateHostEl("div", "", title, stateCompEl(child, "", nil))
	}

	root := NewRoot(parent("one"))
	set(42)
	if got, want := stateTreeDump(root.tree()), `(div "one" (b "42"))`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	// The parent re-renders with different props; the child's fiber — and so its
	// hook slot — is reused, because type and position still match.
	root.Render(parent("two"))
	if got, want := stateTreeDump(root.tree()), `(div "two" (b "42"))`; got != want {
		t.Errorf("after parent re-render tree = %s, want %s", got, want)
	}
}

func TestUseStateIsIndependentPerInstance(t *testing.T) {
	setters := map[string]SetStateFunc[int]{}

	var item Component = func(p Props) Node {
		n, set := UseState(0)
		setters[p.String("id")] = set
		return stateHostEl("li", "", n)
	}
	tree := stateHostEl("ul", "",
		stateCompEl(item, "a", Props{"id": "a"}),
		stateCompEl(item, "b", Props{"id": "b"}),
	)

	root := NewRoot(tree)
	if got, want := stateTreeDump(root.tree()), `(ul (li "0") (li "0"))`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	setters["a"](5)
	if got, want := stateTreeDump(root.tree()), `(ul (li "5") (li "0"))`; got != want {
		t.Errorf("after updating a: tree = %s, want %s", got, want)
	}

	setters["b"].Update(func(prev int) int { return prev + 9 })
	if got, want := stateTreeDump(root.tree()), `(ul (li "5") (li "9"))`; got != want {
		t.Errorf("after updating b: tree = %s, want %s", got, want)
	}
}

func TestUseStateIsDiscardedOnRemount(t *testing.T) {
	var set SetStateFunc[int]

	var child Component = func(Props) Node {
		n, s := UseState(0)
		set = s
		return stateHostEl("b", "", n)
	}
	wrap := func(tag string) Node {
		return stateHostEl(tag, "", stateCompEl(child, "", nil))
	}

	root := NewRoot(wrap("div"))
	set(99)
	if got, want := stateTreeDump(root.tree()), `(div (b "99"))`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	// Changing the wrapper's element type cannot reuse the old fiber, so the
	// whole subtree — hooks included — is torn down and mounted fresh.
	root.Render(wrap("section"))
	if got, want := stateTreeDump(root.tree()), `(section (b "0"))`; got != want {
		t.Errorf("after remount tree = %s, want %s (state must not survive)", got, want)
	}
}

func TestUseStateAfterUnmountIsNoOp(t *testing.T) {
	var set SetStateFunc[int]
	var counter Component = func(Props) Node {
		_, s := UseState(0)
		set = s
		return nil
	}

	root := NewRoot(stateCompEl(counter, "", nil))
	root.Unmount()

	// Neither form may panic or render.
	set(1)
	set.Update(func(prev int) int { return prev + 1 })
}

func TestUseStateHoldsNilAndFunctionValues(t *testing.T) {
	// Two shapes that the boxed queue could plausibly break: a nil interface,
	// which cannot be type-asserted, and a func, which ObjectsEqual cannot
	// compare without panicking.
	var setErr SetStateFunc[error]
	var setFn SetStateFunc[func() string]
	renders := 0

	var c Component = func(Props) Node {
		renders++
		e, se := UseState[error](nil)
		fn, sf := UseState[func() string](nil)
		setErr, setFn = se, sf
		out := "nil/nil"
		if e != nil {
			out = e.Error()
		}
		if fn != nil {
			out += "/" + fn()
		}
		return stateHostEl("b", "", out)
	}

	root := NewRoot(stateCompEl(c, "", nil))
	if got, want := stateTreeDump(root.tree()), `(b "nil/nil")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	setFn(func() string { return "fn" })
	if got, want := stateTreeDump(root.tree()), `(b "nil/nil/fn")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}

	setErr(nil) // nil to nil is a no-op and must bail out
	if renders != 2 {
		t.Errorf("renders = %d, want 2 (the nil-to-nil set must bail out)", renders)
	}
}

func TestUseReducerAppliesActionsInOrder(t *testing.T) {
	type action struct {
		op string
		by int
	}
	reducer := func(state int, a action) int {
		switch a.op {
		case "add":
			return state + a.by
		case "mul":
			return state * a.by
		case "reset":
			return 0
		default:
			return state
		}
	}

	cases := []struct {
		name    string
		actions []action
		want    string
		renders int
	}{
		{"no actions", nil, `(b "1")`, 0},
		{"single add", []action{{"add", 4}}, `(b "5")`, 1},
		{"order matters", []action{{"add", 1}, {"mul", 10}}, `(b "20")`, 1},
		{"reverse order", []action{{"mul", 10}, {"add", 1}}, `(b "11")`, 1},
		{"reset then add", []action{{"add", 7}, {"reset", 0}, {"add", 3}}, `(b "3")`, 1},
		{"unknown action bails out", []action{{"noop", 0}}, `(b "1")`, 0},
	}

	for _, c := range cases {
		var dispatch func(action)
		renders := 0
		var counter Component = func(Props) Node {
			renders++
			n, d := UseReducer(reducer, 1)
			dispatch = d
			return stateHostEl("b", "", n)
		}

		root := NewRoot(stateCompEl(counter, "", nil))
		renders = 0

		Batch(func() {
			for _, a := range c.actions {
				dispatch(a)
			}
		})

		if got := stateTreeDump(root.tree()); got != c.want {
			t.Errorf("%s: tree = %s, want %s", c.name, got, c.want)
		}
		if renders != c.renders {
			t.Errorf("%s: renders = %d, want %d", c.name, renders, c.renders)
		}
	}
}

func TestUseReducerDispatchIdentityAndStaleClosure(t *testing.T) {
	reducer := func(state, delta int) int { return state + delta }

	var first func(int)
	var ids []uintptr
	captured := false

	var counter Component = func(Props) Node {
		n, dispatch := UseReducer(reducer, 0)
		if !captured {
			first = dispatch
			captured = true
		}
		// A func(A) has the same identity problem as a setter, and the probe is
		// generic over the function type, so it applies unchanged.
		ids = append(ids, stateSetterID(dispatch))
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", nil))
	first(3)
	first(4)

	if got, want := stateTreeDump(root.tree()), `(b "7")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
	for i, id := range ids {
		if id != ids[0] {
			t.Errorf("dispatch identity changed on render %d: %#x != %#x", i, id, ids[0])
		}
	}
}

func TestUseReducerInitRunsOnceOnMount(t *testing.T) {
	inits := 0
	init := func(seed int) []int {
		inits++
		return []int{seed}
	}
	reducer := func(state []int, add int) []int {
		return append(append([]int{}, state...), add)
	}

	var dispatch func(int)
	var list Component = func(Props) Node {
		items, d := UseReducerInit(reducer, 1, init)
		dispatch = d
		kids := make([]Node, 0, len(items))
		for _, n := range items {
			kids = append(kids, stateHostEl("li", "", n))
		}
		return stateHostEl("ul", "", kids)
	}

	root := NewRoot(stateCompEl(list, "", nil))
	dispatch(2)
	dispatch(3)
	root.Render(stateCompEl(list, "", Props{"x": 1}))

	if inits != 1 {
		t.Errorf("init calls = %d, want 1 (mount only)", inits)
	}
	if got, want := stateTreeDump(root.tree()), `(ul (li "1") (li "2") (li "3"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseReducerSeesTheLatestReducerClosure(t *testing.T) {
	// The reducer is re-read every render, so a reducer closing over props uses
	// the props from the most recent render rather than the ones it mounted with.
	var dispatch func(string)

	var counter Component = func(p Props) Node {
		step, _ := p.Get("step").(int)
		n, d := UseReducer(func(state int, _ string) int { return state + step }, 0)
		dispatch = d
		return stateHostEl("b", "", n)
	}

	root := NewRoot(stateCompEl(counter, "", Props{"step": 1}))
	dispatch("bump")
	if got, want := stateTreeDump(root.tree()), `(b "1")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	root.Render(stateCompEl(counter, "", Props{"step": 10}))
	dispatch("bump")
	if got, want := stateTreeDump(root.tree()), `(b "11")`; got != want {
		t.Errorf("tree = %s, want %s (the newest reducer closure must be used)", got, want)
	}
}

func TestUseStateRenderPhaseUpdateSettles(t *testing.T) {
	// The "derive state from props" escape hatch: a setter called in the render
	// body is a render-phase update, which the flush loop resolves with another
	// pass instead of recursing.
	renders := 0
	var c Component = func(p Props) Node {
		renders++
		want := p.String("want")
		got, set := UseState("")
		if got != want {
			set(want)
		}
		return stateHostEl("b", "", got)
	}

	root := NewRoot(stateCompEl(c, "", Props{"want": "a"}))
	if got, want := stateTreeDump(root.tree()), `(b "a")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
	if renders != 2 {
		t.Errorf("renders = %d, want 2 (one mount, one render-phase pass)", renders)
	}
}
