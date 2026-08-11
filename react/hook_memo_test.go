package react

import (
	"strings"
	"testing"
)

// memoTestRerender re-renders the same element into root the given number of
// extra times. Re-rendering the identical element is the cheapest way to prove
// a hook reuses its slot: reconciliation matches by type and key, so the fiber
// — and therefore every hook on it — survives untouched.
func memoTestRerender(root *Root, el *Element, times int) {
	for range times {
		root.Render(el)
	}
}

func TestUseMemoRecomputesOnlyWhenDepsChange(t *testing.T) {
	cases := []struct {
		name string
		// deps holds one dependency list per render, so a case describes an
		// entire render sequence rather than a single call.
		deps         [][]any
		wantComputes int
	}{
		{
			name:         "nil deps recompute on every render",
			deps:         [][]any{nil, nil, nil},
			wantComputes: 3,
		},
		{
			name:         "empty non-nil deps compute exactly once",
			deps:         [][]any{{}, {}, {}},
			wantComputes: 1,
		},
		{
			name:         "unchanged deps reuse the memo",
			deps:         [][]any{{1, "a"}, {1, "a"}, {1, "a"}},
			wantComputes: 1,
		},
		{
			name:         "a changed entry recomputes once, then reuses again",
			deps:         [][]any{{1}, {1}, {2}, {2}},
			wantComputes: 2,
		},
		{
			name:         "a changed length recomputes",
			deps:         [][]any{{1}, {1, 2}},
			wantComputes: 2,
		},
		{
			name:         "dropping to nil deps recomputes",
			deps:         [][]any{{1}, nil},
			wantComputes: 2,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			computes := 0
			render := 0

			var comp Component = func(Props) Node {
				deps := c.deps[render]
				render++
				n := UseMemo(func() int {
					computes++
					return computes
				}, deps)
				return host("b", "", n)
			}

			el := &Element{Type: comp, Props: Props{}}
			root := NewRoot(el)
			memoTestRerender(root, el, len(c.deps)-1)

			if render != len(c.deps) {
				t.Fatalf("component rendered %d times, want %d", render, len(c.deps))
			}
			if computes != c.wantComputes {
				t.Errorf("create ran %d times, want %d", computes, c.wantComputes)
			}
			// The rendered value must be whatever the last surviving computation
			// produced — proof that the reused value, not a fresh one, is what
			// the component actually saw.
			if got, want := dump(root.tree()), `(b "`+itoaTestMemo(c.wantComputes)+`")`; got != want {
				t.Errorf("tree = %s, want %s", got, want)
			}
		})
	}
}

// itoaTestMemo formats a small int without pulling strconv into the test file's
// import list for a single call.
func itoaTestMemo(n int) string { return textOf(n) }

func TestUseMemoCachesAZeroValue(t *testing.T) {
	// A memoized nil is the case a naive `h.value.(T)` implementation gets
	// wrong: the assertion fails on a nil interface and the hook recomputes
	// forever. Empty deps mean exactly one call, nil result or not.
	computes := 0
	var comp Component = func(Props) Node {
		v := UseMemo(func() any {
			computes++
			return nil
		}, []any{})
		if v != nil {
			t.Errorf("memoized value = %v, want nil", v)
		}
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	memoTestRerender(root, el, 2)

	if computes != 1 {
		t.Errorf("create ran %d times, want 1", computes)
	}
}

func TestUseMemoSlotsAreIndependent(t *testing.T) {
	// Two memos in one component must not share a slot: the second one's deps
	// change while the first one's stay put.
	firstComputes, secondComputes := 0, 0
	render := 0

	var comp Component = func(Props) Node {
		render++
		n := render
		UseMemo(func() int { firstComputes++; return 0 }, []any{"stable"})
		UseMemo(func() int { secondComputes++; return n }, []any{n})
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	memoTestRerender(root, el, 2)

	if firstComputes != 1 {
		t.Errorf("stable memo ran %d times, want 1", firstComputes)
	}
	if secondComputes != 3 {
		t.Errorf("changing memo ran %d times, want 3", secondComputes)
	}
}

func TestUseCallbackKeepsIdentityUntilDepsChange(t *testing.T) {
	// Comparing func values by pointer is useless here — every closure from the
	// same literal shares a code pointer. Instead the callback reports the
	// render number it closed over, so a retained callback answers with a stale
	// number and a freshly created one answers with the current render.
	render := 0
	dep := "a"
	var observed []int

	var comp Component = func(Props) Node {
		render++
		n := render
		cb := UseCallback(func() int { return n }, []any{dep})
		observed = append(observed, cb())
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	memoTestRerender(root, el, 2) // renders 2 and 3, deps unchanged

	dep = "b"
	root.Render(el) // render 4 recreates the callback

	memoTestRerender(root, el, 1) // render 5 reuses the render-4 callback

	want := []int{1, 1, 1, 4, 4}
	if len(observed) != len(want) {
		t.Fatalf("observed %v, want %v", observed, want)
	}
	for i := range want {
		if observed[i] != want[i] {
			t.Fatalf("observed %v, want %v", observed, want)
		}
	}
}

func TestUseCallbackWithNilDepsIsNeverStable(t *testing.T) {
	render := 0
	var observed []int
	var comp Component = func(Props) Node {
		render++
		n := render
		cb := UseCallback(func() int { return n }, nil)
		observed = append(observed, cb())
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	memoTestRerender(root, el, 2)

	for i, got := range observed {
		if got != i+1 {
			t.Errorf("render %d saw callback from render %d, want %d", i+1, got, i+1)
		}
	}
}

func TestUseIdIsStableAcrossRenders(t *testing.T) {
	var seen []string
	var comp Component = func(Props) Node {
		seen = append(seen, UseId())
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	memoTestRerender(root, el, 3)

	if len(seen) != 4 {
		t.Fatalf("rendered %d times, want 4", len(seen))
	}
	for _, id := range seen {
		if id != seen[0] {
			t.Fatalf("ids across renders = %v, want all %q", seen, seen[0])
		}
	}
	if !strings.HasPrefix(seen[0], ":r") || !strings.HasSuffix(seen[0], ":") {
		t.Errorf("id = %q, want the React-like :r…: shape", seen[0])
	}
}

func TestUseIdIsUniqueAcrossSiblingsAndCallSites(t *testing.T) {
	var ids []string
	var leaf Component = func(Props) Node {
		// Two calls in one component must not collide either: the hook index is
		// part of the id.
		ids = append(ids, UseId(), UseId())
		return nil
	}

	kids := []Node{
		&Element{Type: leaf, Props: Props{}},
		&Element{Type: leaf, Props: Props{}},
		&Element{Type: leaf, Props: Props{}},
	}
	NewRoot(host("ul", "", kids))

	if len(ids) != 6 {
		t.Fatalf("collected %d ids, want 6", len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate id %q in %v", id, ids)
		}
		seen[id] = true
	}
}

func TestUseIdIsDeterministicAcrossIdenticalRoots(t *testing.T) {
	// The whole point of the hook: the same tree rendered twice, into two
	// Roots that share nothing, yields the same ids in the same places. That is
	// what makes server- and client-generated markup agree.
	collect := func(sink *[]string) Component {
		return func(Props) Node {
			*sink = append(*sink, UseId())
			return nil
		}
	}
	build := func(sink *[]string) Node {
		c := collect(sink)
		return host("div", "",
			&Element{Type: c, Props: Props{}},
			host("span", "", &Element{Type: c, Props: Props{}}),
		)
	}

	var a, b []string
	NewRoot(build(&a))
	NewRoot(build(&b))

	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("collected %v and %v, want two ids each", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("id %d: root A = %q, root B = %q; ids must be position-derived", i, a[i], b[i])
		}
	}
	if a[0] == a[1] {
		t.Errorf("both positions produced %q; ids must differ by position", a[0])
	}
}

func TestDebugValuesCollectsLabelsLazily(t *testing.T) {
	formatCalls := 0
	var comp Component = func(Props) Node {
		UseDebugValue("eager")
		UseDebugValueFn(func() string {
			formatCalls++
			return "lazy"
		})
		return nil
	}

	root := NewRoot(&Element{Type: comp, Props: Props{}})

	if formatCalls != 0 {
		t.Errorf("deferred formatter ran %d times during render, want 0", formatCalls)
	}

	got := strings.Join(DebugValues(root), ",")
	if want := "eager,lazy"; got != want {
		t.Errorf("DebugValues = %q, want %q", got, want)
	}
	if formatCalls != 1 {
		t.Errorf("deferred formatter ran %d times, want 1", formatCalls)
	}
	if DebugValues(nil) != nil {
		t.Error("DebugValues(nil) must be nil")
	}
}

func TestMemoHooksPanicOutsideRender(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic when a hook is called outside a render")
		}
		if !strings.Contains(toString(r), "outside of a component render") {
			t.Errorf("panic = %v, want the dispatcher diagnostic", r)
		}
	}()
	UseMemo(func() int { return 1 }, nil)
}
