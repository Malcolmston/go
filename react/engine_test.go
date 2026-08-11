package react

import (
	"strings"
	"testing"
)

// dump renders a fiber subtree as a compact S-expression so tests can assert on
// tree shape in one string comparison instead of walking pointers by hand.
func dump(f *fiber) string {
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
			// Fragments and components are transparent in the dump: only host
			// output shape matters to these tests.
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

// host builds a host element without depending on CreateElement, so the engine
// tests stay independent of the element-construction layer.
func host(tag string, key string, children ...Node) *Element {
	return &Element{Type: tag, Key: key, Props: Props{ChildrenKey: children}}
}

func TestRenderHostTreeAndText(t *testing.T) {
	root := NewRoot(host("div", "", host("span", "", "hi"), 42))
	if got, want := dump(root.tree()), `(div (span "hi") "42")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestNilAndBoolChildrenRenderNothing(t *testing.T) {
	root := NewRoot(host("ul", "", nil, false, true, host("li", ""), nil))
	if got, want := dump(root.tree()), `(ul (li))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestNestedSlicesAreFlattened(t *testing.T) {
	items := []Node{host("li", "a"), host("li", "b")}
	root := NewRoot(host("ul", "", items, host("li", "c")))
	if got, want := dump(root.tree()), `(ul (li#a) (li#b) (li#c))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

// counterHook is a minimal state hook built directly on the nextHook primitive.
// It exists so the engine's scheduling can be tested before the real UseState
// lands on top of the same primitive.
func counterHook(initial int) (int, func(int)) {
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

func TestHookStateSurvivesRerenderAndSchedulesUpdate(t *testing.T) {
	var bump func(int)
	renders := 0

	var counter Component = func(Props) Node {
		renders++
		n, set := counterHook(0)
		bump = set
		return host("b", "", n)
	}

	root := NewRoot(&Element{Type: counter, Props: Props{}})
	if got, want := dump(root.tree()), `(b "0")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	bump(7)
	if got, want := dump(root.tree()), `(b "7")`; got != want {
		t.Errorf("after update tree = %s, want %s", got, want)
	}
	if renders != 2 {
		t.Errorf("renders = %d, want 2 (one mount, one update)", renders)
	}
}

func TestKeyedReorderPreservesFiberIdentity(t *testing.T) {
	// Each child owns a hook slot seeded with its key. After a reorder the slot
	// must still travel with the key — that is the whole point of keys.
	var item Component = func(p Props) Node {
		h, first := nextHook("state")
		if first {
			h.value = p.String("id")
		}
		return host("li", "", h.value.(string))
	}
	mk := func(ids ...string) Node {
		var kids []Node
		for _, id := range ids {
			kids = append(kids, &Element{Type: item, Key: id, Props: Props{"id": id}})
		}
		return host("ul", "", kids)
	}

	root := NewRoot(mk("a", "b", "c"))
	if got, want := dump(root.tree()), `(ul (li "a") (li "b") (li "c"))`; got != want {
		t.Fatalf("initial = %s, want %s", got, want)
	}

	root.Render(mk("c", "a", "b"))
	if got, want := dump(root.tree()), `(ul (li "c") (li "a") (li "b"))`; got != want {
		t.Errorf("after reorder = %s, want %s", got, want)
	}
}

func TestUnmountRunsCleanupsLeafFirst(t *testing.T) {
	var order []string

	var leaf Component = func(Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() {
				return func() { order = append(order, "leaf") }
			}, false)
		}
		return host("i", "")
	}
	var branch Component = func(p Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() {
				return func() { order = append(order, "branch") }
			}, false)
		}
		return host("div", "", &Element{Type: leaf, Props: Props{}})
	}

	show := func(on bool) Node {
		if !on {
			return host("section", "")
		}
		return host("section", "", &Element{Type: branch, Props: Props{}})
	}

	root := NewRoot(show(true))
	root.Render(show(false))

	if got, want := strings.Join(order, ","), "leaf,branch"; got != want {
		t.Errorf("cleanup order = %q, want %q", got, want)
	}
}

func TestLayoutEffectsRunBeforePassiveEffects(t *testing.T) {
	var order []string
	var c Component = func(Props) Node {
		p, firstP := nextHook("effect")
		if firstP {
			enqueueEffect(p, func() func() { order = append(order, "passive"); return nil }, false)
		}
		l, firstL := nextHook("layout")
		if firstL {
			enqueueEffect(l, func() func() { order = append(order, "layout"); return nil }, true)
		}
		return nil
	}

	NewRoot(&Element{Type: c, Props: Props{}})
	if got, want := strings.Join(order, ","), "layout,passive"; got != want {
		t.Errorf("effect order = %q, want %q", got, want)
	}
}

func TestHookOrderChangePanics(t *testing.T) {
	pass := 0
	var c Component = func(Props) Node {
		pass++
		if pass == 1 {
			nextHook("state")
		}
		nextHook("effect")
		return nil
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic when hook order changed")
		}
		if !strings.Contains(toString(r), "hook order changed") {
			t.Errorf("panic = %v, want a hook-order diagnostic", r)
		}
	}()
	root.Render(&Element{Type: c, Props: Props{}})
}

func TestTooManyRerendersPanics(t *testing.T) {
	var c Component = func(Props) Node {
		h, _ := nextHook("state")
		f := currentFiber()
		h.value = 0
		// An unconditional update from the render body: the pathological case
		// the pass limit exists to diagnose.
		defer f.scheduleUpdate()
		return nil
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a too-many-re-renders panic")
		}
		if !strings.Contains(toString(r), "too many re-renders") {
			t.Errorf("panic = %v, want a re-render limit diagnostic", r)
		}
	}()
	NewRoot(&Element{Type: c, Props: Props{}})
}

func TestBatchCoalescesUpdatesIntoOneRender(t *testing.T) {
	var set func(int)
	renders := 0
	var c Component = func(Props) Node {
		renders++
		n, s := counterHook(0)
		set = s
		return host("b", "", n)
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})
	renders = 0

	Batch(func() {
		set(1)
		set(2)
		set(3)
	})

	if renders != 1 {
		t.Errorf("renders = %d, want 1 (three updates coalesced)", renders)
	}
	if got, want := dump(root.tree()), `(b "3")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUnmountedRootIgnoresLateUpdates(t *testing.T) {
	var set func(int)
	var c Component = func(Props) Node {
		_, s := counterHook(0)
		set = s
		return nil
	}
	root := NewRoot(&Element{Type: c, Props: Props{}})
	root.Unmount()
	set(1) // must not panic and must not render
}

func TestObjectsEqualHandlesUncomparableKinds(t *testing.T) {
	s := []int{1, 2}
	m := map[string]int{"a": 1}
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		{"identical ints", 1, 1, true},
		{"different ints", 1, 2, false},
		{"different types", 1, "1", false},
		{"both nil", nil, nil, true},
		{"one nil", nil, 1, false},
		{"same slice", s, s, true},
		{"distinct slices", []int{1, 2}, []int{1, 2}, false},
		{"same map", m, m, true},
	}
	for _, c := range cases {
		if got := ObjectsEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: ObjectsEqual(%v, %v) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}
}

func TestDepsChanged(t *testing.T) {
	cases := []struct {
		name       string
		prev, next []any
		want       bool
	}{
		{"nil prev means first run", nil, []any{1}, true},
		{"nil next means always re-run", []any{1}, nil, true},
		{"empty lists never change", []any{}, []any{}, false},
		{"same values", []any{1, "a"}, []any{1, "a"}, false},
		{"changed value", []any{1, "a"}, []any{2, "a"}, true},
		{"different length", []any{1}, []any{1, 2}, true},
	}
	for _, c := range cases {
		if got := DepsChanged(c.prev, c.next); got != c.want {
			t.Errorf("%s: DepsChanged = %v, want %v", c.name, got, c.want)
		}
	}
}

// toString renders a recovered panic value for assertion messages.
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return textOf(v)
}
