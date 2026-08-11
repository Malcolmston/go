package react

import (
	"strings"
	"testing"
	"time"
)

// componentDump renders a fiber subtree as a compact S-expression, the same
// shape engine_test.go's dump produces. It is duplicated rather than shared
// because these tests care about one extra thing: that [Memo], [StrictMode] and
// [Profiler] fibers are *invisible* in the output, which only holds if the dump
// treats every non-host, non-text fiber as transparent.
func componentDump(f *fiber) string {
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
			_ = t
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

// componentHost builds a host element without going through CreateElement, so
// these tests exercise the wrappers and nothing else.
func componentHost(tag string, children ...Node) *Element {
	return &Element{Type: tag, Props: Props{ChildrenKey: children}}
}

// componentStateHook is a minimal state hook built straight on the nextHook
// primitive. These tests need component state to prove that a memo bailout
// preserves it and does not swallow updates, but they must not depend on
// whichever file eventually provides UseState.
func componentStateHook(initial int) (int, func(int)) {
	h, first := nextHook("state")
	f := currentFiber()
	if first {
		h.value = initial
	}
	set := func(v int) {
		h.value = v
		f.scheduleUpdate()
	}
	return h.value.(int), set
}

func TestMemoBailsOutOnEqualProps(t *testing.T) {
	renders := 0
	var label Component = func(p Props) Node {
		renders++
		return componentHost("b", p.String("label"))
	}
	memoized := Memo(label)
	el := func(v string) Node { return &Element{Type: memoized, Props: Props{"label": v}} }

	root := NewRoot(el("a"))
	if got, want := componentDump(root.tree()), `(b "a")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}
	if renders != 1 {
		t.Fatalf("renders after mount = %d, want 1", renders)
	}

	// A structurally identical element with a *different* props map: the whole
	// point is that equality, not identity, decides.
	root.Render(el("a"))
	if renders != 1 {
		t.Errorf("renders after equal-props re-render = %d, want 1 (memo should bail out)", renders)
	}
	if got, want := componentDump(root.tree()), `(b "a")`; got != want {
		t.Errorf("tree after bailout = %s, want %s", got, want)
	}
}

func TestMemoRerendersOnChangedProps(t *testing.T) {
	renders := 0
	var label Component = func(p Props) Node {
		renders++
		return componentHost("b", p.String("label"))
	}
	memoized := Memo(label)
	el := func(v string) Node { return &Element{Type: memoized, Props: Props{"label": v}} }

	root := NewRoot(el("a"))
	root.Render(el("b"))

	if renders != 2 {
		t.Errorf("renders = %d, want 2 (props changed)", renders)
	}
	if got, want := componentDump(root.tree()), `(b "b")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}

	// And back to a value equal to the one before: still a change relative to the
	// props actually on screen, so it must render.
	root.Render(el("a"))
	if renders != 3 {
		t.Errorf("renders = %d, want 3", renders)
	}
	if got, want := componentDump(root.tree()), `(b "a")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

// TestMemoDoesNotSwallowDescendantStateUpdate is the bug the wasDirty check
// exists to prevent, and the reason renderMemoFiber consults it before the
// comparator. The parent re-renders the memo with props that compare equal, so a
// comparator-only bailout would drop the update the grandchild scheduled and the
// tree would freeze at "0" forever.
func TestMemoDoesNotSwallowDescendantStateUpdate(t *testing.T) {
	var bump func(int)
	var counter Component = func(Props) Node {
		n, set := componentStateHook(0)
		bump = set
		return componentHost("i", n)
	}
	var shell Component = func(Props) Node {
		return componentHost("div", &Element{Type: counter, Props: Props{}})
	}
	memoized := Memo(shell)

	root := NewRoot(&Element{Type: memoized, Props: Props{"n": 1}})
	if got, want := componentDump(root.tree()), `(div (i "0"))`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	bump(5)
	if got, want := componentDump(root.tree()), `(div (i "5"))`; got != want {
		t.Errorf("tree after descendant update = %s, want %s "+
			"(the memo boundary swallowed its own child's state update)", got, want)
	}
}

// TestMemoBailsOutWhenAnUnrelatedSiblingUpdates is the complement of the test
// above: a state update *outside* the memoized subtree re-renders the whole
// tree from the root, and the memo must skip its own subtree because nothing
// beneath it is dirty.
func TestMemoBailsOutWhenAnUnrelatedSiblingUpdates(t *testing.T) {
	memoRenders := 0
	var leaf Component = func(p Props) Node {
		memoRenders++
		return componentHost("b", p.String("label"))
	}
	memoized := Memo(leaf)

	var bump func(int)
	var app Component = func(Props) Node {
		n, set := componentStateHook(0)
		bump = set
		return componentHost("div",
			componentHost("s", n),
			&Element{Type: memoized, Props: Props{"label": "fixed"}},
		)
	}

	root := NewRoot(&Element{Type: app, Props: Props{}})
	if memoRenders != 1 {
		t.Fatalf("memo renders after mount = %d, want 1", memoRenders)
	}

	bump(3)
	if memoRenders != 1 {
		t.Errorf("memo renders after sibling update = %d, want 1 (subtree is clean)", memoRenders)
	}
	if got, want := componentDump(root.tree()), `(div (s "3") (b "fixed"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestMemoPreservesChildStateAcrossBailout(t *testing.T) {
	var bump func(int)
	var counter Component = func(Props) Node {
		n, set := componentStateHook(0)
		bump = set
		return componentHost("i", n)
	}
	memoized := Memo(counter)
	el := func() Node { return &Element{Type: memoized, Props: Props{"label": "x"}} }

	root := NewRoot(el())
	bump(9)
	if got, want := componentDump(root.tree()), `(i "9")`; got != want {
		t.Fatalf("tree after update = %s, want %s", got, want)
	}

	// Re-render with equal props: the memo bails out, which means the child fiber
	// — and therefore its hook slot — is left untouched rather than rebuilt.
	root.Render(el())
	if got, want := componentDump(root.tree()), `(i "9")`; got != want {
		t.Errorf("tree after bailout = %s, want %s (child state was lost)", got, want)
	}

	// The state must still be live, not merely still displayed.
	bump(11)
	if got, want := componentDump(root.tree()), `(i "11")`; got != want {
		t.Errorf("tree after post-bailout update = %s, want %s", got, want)
	}
}

func TestMemoWithCustomComparator(t *testing.T) {
	renders := 0
	var row Component = func(p Props) Node {
		renders++
		return componentHost("b", p.String("id"))
	}
	// Only "id" is meaningful; "version" changes on every render and would defeat
	// the default comparator entirely.
	memoized := MemoWith(row, func(prev, next Props) bool {
		return prev.String("id") == next.String("id")
	})
	el := func(id string, version int) Node {
		return &Element{Type: memoized, Props: Props{"id": id, "version": version}}
	}

	root := NewRoot(el("a", 1))
	root.Render(el("a", 2))
	if renders != 1 {
		t.Errorf("renders after ignored prop changed = %d, want 1", renders)
	}

	root.Render(el("b", 3))
	if renders != 2 {
		t.Errorf("renders after significant prop changed = %d, want 2", renders)
	}
	if got, want := componentDump(root.tree()), `(b "b")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}

	// A nil comparator falls back to the default rather than panicking at render
	// time, since MemoWith(c, nil) plainly means "the ordinary memo".
	if MemoWith(row, nil).areEqual == nil {
		t.Error("MemoWith(c, nil) left a nil comparator")
	}
}

func TestMemoRejectsNilComponent(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Memo(nil) to panic")
		}
		if !strings.Contains(toString(r), "nil component") {
			t.Errorf("panic = %v, want a nil-component diagnostic", r)
		}
	}()
	Memo(nil)
}

func TestShallowEqualProps(t *testing.T) {
	slice := []int{1, 2}
	dict := map[string]int{"a": 1}
	fn := func() {}
	other := func() {}
	child := componentHost("li")

	cases := []struct {
		name string
		a, b Props
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil and empty carry the same keys", nil, Props{}, true},
		{"identical scalars", Props{"n": 1, "s": "x"}, Props{"n": 1, "s": "x"}, true},
		{"changed scalar", Props{"n": 1}, Props{"n": 2}, false},
		{"changed value type", Props{"n": 1}, Props{"n": "1"}, false},
		{"extra key on the right", Props{"n": 1}, Props{"n": 1, "m": 2}, false},
		{"extra key on the left", Props{"n": 1, "m": 2}, Props{"n": 1}, false},
		{"same count, different key names", Props{"a": 1}, Props{"b": 1}, false},
		{"present nil equals present nil", Props{"a": nil}, Props{"a": nil}, true},
		{"present nil differs from a value", Props{"a": nil}, Props{"a": 0}, false},

		// Uncomparable values: identity, never structure. Go's == would panic on
		// these, which is exactly why ObjectsEqual exists.
		{"same slice value", Props{"xs": slice}, Props{"xs": slice}, true},
		{"distinct but equal slices", Props{"xs": []int{1, 2}}, Props{"xs": []int{1, 2}}, false},
		{"same map value", Props{"m": dict}, Props{"m": dict}, true},
		{"distinct but equal maps", Props{"m": map[string]int{"a": 1}}, Props{"m": map[string]int{"a": 1}}, false},

		// Functions are compared by code pointer here rather than delegated to
		// ObjectsEqual, whose func branch panics inside reflect.Value.Len.
		{"same func value", Props{"on": fn}, Props{"on": fn}, true},
		{"different func literals", Props{"on": fn}, Props{"on": other}, false},

		// Children participate, with the documented consequence: the same child
		// slice compares equal, a freshly built one never does.
		{"same children slice", Props{ChildrenKey: []Node{child}}, Props{ChildrenKey: []Node{child}}, false},
		{"identical children value", Props{ChildrenKey: child}, Props{ChildrenKey: child}, true},
		{"changed children", Props{ChildrenKey: child}, Props{ChildrenKey: componentHost("li")}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ShallowEqualProps(c.a, c.b); got != c.want {
				t.Errorf("ShallowEqualProps(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
			}
			// Equality must be symmetric, or a memo bailout would depend on which
			// render happened first.
			if got := ShallowEqualProps(c.b, c.a); got != c.want {
				t.Errorf("reversed: ShallowEqualProps(%v, %v) = %v, want %v", c.b, c.a, got, c.want)
			}
		})
	}
}

// TestMemoWithInlineChildrenAlwaysRenders pins the caveat documented on
// ShallowEqualProps: children built fresh each render are new pointers, so a
// memo that takes children does not bail out. It is a real limitation, and a
// test is the honest place to record it.
func TestMemoWithInlineChildrenAlwaysRenders(t *testing.T) {
	renders := 0
	var box Component = func(p Props) Node {
		renders++
		return componentHost("div", p.Children())
	}
	memoized := Memo(box)
	el := func() Node {
		return &Element{Type: memoized, Props: Props{ChildrenKey: []Node{componentHost("li")}}}
	}

	root := NewRoot(el())
	root.Render(el())
	if renders != 2 {
		t.Errorf("renders = %d, want 2 (fresh children defeat the default comparator)", renders)
	}
	if got, want := componentDump(root.tree()), `(div (li))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestStrictModeIsTransparent(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)
	StrictModeDoubleRender = false

	root := NewRoot(componentHost("section",
		StrictMode(componentHost("p", "hi"), componentHost("p", "there")),
	))
	if got, want := componentDump(root.tree()), `(section (p "hi") (p "there"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestStrictModePreservesStateAcrossRenders(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)
	StrictModeDoubleRender = true

	var bump func(int)
	var counter Component = func(Props) Node {
		n, set := componentStateHook(0)
		bump = set
		return componentHost("i", n)
	}

	root := NewRoot(StrictMode(&Element{Type: counter, Props: Props{}}))
	if got, want := componentDump(root.tree()), `(i "0")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	bump(4)
	if got, want := componentDump(root.tree()), `(i "4")`; got != want {
		t.Errorf("tree after update = %s, want %s (double rendering lost state)", got, want)
	}
}

func TestStrictModeDoubleRenderTogglesSubtreeInvocations(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)

	cases := []struct {
		name   string
		double bool
		want   int
	}{
		{"disabled renders once", false, 1},
		{"enabled renders twice", true, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			StrictModeDoubleRender = c.double
			renders := 0
			var leaf Component = func(Props) Node {
				renders++
				return componentHost("b", "x")
			}

			root := NewRoot(StrictMode(&Element{Type: leaf, Props: Props{}}))
			if renders != c.want {
				t.Errorf("component invocations = %d, want %d", renders, c.want)
			}
			// Impurity aside, the rendered tree is the same either way.
			if got, want := componentDump(root.tree()), `(b "x")`; got != want {
				t.Errorf("tree = %s, want %s", got, want)
			}
		})
	}
}

// TestStrictModeSurfacesAnImpureComponent demonstrates what the double render is
// for: a component whose output depends on how many times it has been called
// renders its *second* answer, so the impurity is visible in the tree instead of
// hiding until some unrelated render happens to run twice.
func TestStrictModeSurfacesAnImpureComponent(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)
	StrictModeDoubleRender = true

	calls := 0
	var impure Component = func(Props) Node {
		calls++
		return componentHost("b", calls)
	}

	root := NewRoot(StrictMode(&Element{Type: impure, Props: Props{}}))
	if got, want := componentDump(root.tree()), `(b "2")`; got != want {
		t.Errorf("tree = %s, want %s (impurity should be visible)", got, want)
	}
}

func TestProfilerReportsMountThenUpdate(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)
	StrictModeDoubleRender = false

	var events []ProfilerEvent
	record := func(e ProfilerEvent) { events = append(events, e) }
	tree := func(label string) Node {
		return Profiler("list", record, componentHost("b", label))
	}

	root := NewRoot(tree("a"))
	if got, want := componentDump(root.tree()), `(b "a")`; got != want {
		t.Fatalf("initial tree = %s, want %s (Profiler must be transparent)", got, want)
	}
	if len(events) != 1 {
		t.Fatalf("events after mount = %d, want 1", len(events))
	}

	root.Render(tree("b"))
	if len(events) != 2 {
		t.Fatalf("events after update = %d, want 2", len(events))
	}
	if got, want := componentDump(root.tree()), `(b "b")`; got != want {
		t.Errorf("tree after update = %s, want %s", got, want)
	}

	wantPhases := []ProfilerPhase{ProfilerMount, ProfilerUpdate}
	for i, e := range events {
		if e.ID != "list" {
			t.Errorf("event %d: ID = %q, want %q", i, e.ID, "list")
		}
		if e.Phase != wantPhases[i] {
			t.Errorf("event %d: Phase = %v, want %v", i, e.Phase, wantPhases[i])
		}
		// The duration is a real time.Since over the subtree's reconciliation.
		// Asserting a strict lower bound would be a flaky assertion about clock
		// granularity, so this checks the only thing that is always true: the
		// measurement was taken and is not negative or absurd.
		if e.Duration < 0 || e.Duration > time.Minute {
			t.Errorf("event %d: Duration = %v, want a plausible measurement", i, e.Duration)
		}
	}
}

func TestProfilerIDChangeRemountsAndReportsMount(t *testing.T) {
	defer func(prev bool) { StrictModeDoubleRender = prev }(StrictModeDoubleRender)
	StrictModeDoubleRender = false

	var events []ProfilerEvent
	record := func(e ProfilerEvent) { events = append(events, e) }

	root := NewRoot(Profiler("one", record, componentHost("b", "x")))
	root.Render(Profiler("two", record, componentHost("b", "x")))

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[1].ID != "two" || events[1].Phase != ProfilerMount {
		t.Errorf("second event = %+v, want id %q phase %v", events[1], "two", ProfilerMount)
	}
}

func TestProfilerToleratesNilCallback(t *testing.T) {
	root := NewRoot(Profiler("silent", nil, componentHost("b", "x")))
	if got, want := componentDump(root.tree()), `(b "x")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
	root.Render(Profiler("silent", nil, componentHost("b", "y")))
}

func TestProfilerPhaseString(t *testing.T) {
	cases := []struct {
		phase ProfilerPhase
		want  string
	}{
		{ProfilerMount, "mount"},
		{ProfilerUpdate, "update"},
		{ProfilerPhase(7), "ProfilerPhase(7)"},
	}
	for _, c := range cases {
		if got := c.phase.String(); got != c.want {
			t.Errorf("ProfilerPhase(%d).String() = %q, want %q", int(c.phase), got, c.want)
		}
	}
}
