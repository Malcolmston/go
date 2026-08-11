package react

import (
	"strings"
	"testing"
)

// refTestHandle is the sort of small imperative surface UseImperativeHandle
// exists to publish: a couple of operations, no internals.
type refTestHandle struct {
	Name string
	Call func() string
}

func TestUseRefPersistsAcrossRenders(t *testing.T) {
	var got *Ref[int]
	renders := 0

	var comp Component = func(Props) Node {
		renders++
		// initial is only honored on the first render; a later value must be
		// ignored, which is why it is derived from the render count here.
		r := UseRef(renders * 100)
		got = r
		return host("b", "", r.Current)
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)

	first := got
	if first.Current != 100 {
		t.Fatalf("initial Current = %d, want 100", first.Current)
	}

	root.Render(el)
	root.Render(el)

	if got != first {
		t.Error("UseRef returned a different *Ref across renders; refs must be stable")
	}
	if got.Current != 100 {
		t.Errorf("Current = %d after re-renders, want 100 (initial is first-render only)", got.Current)
	}
	if want := `(b "100")`; dump(root.tree()) != want {
		t.Errorf("tree = %s, want %s", dump(root.tree()), want)
	}
}

func TestUseRefWriteDoesNotTriggerRender(t *testing.T) {
	var got *Ref[string]
	renders := 0

	var comp Component = func(Props) Node {
		renders++
		got = UseRef("initial")
		return host("b", "", got.Current)
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	if renders != 1 {
		t.Fatalf("renders = %d after mount, want 1", renders)
	}

	got.Current = "written"

	if renders != 1 {
		t.Errorf("renders = %d after a ref write, want 1; writing a ref must not schedule work", renders)
	}
	if want := `(b "initial")`; dump(root.tree()) != want {
		t.Errorf("tree = %s, want %s; the tree must not update on a ref write", dump(root.tree()), want)
	}

	// The write is not lost, only invisible: the next render — triggered by
	// anything at all — picks it up. This asymmetry is precisely the impurity
	// hazard documented on Ref.
	root.Render(el)
	if got.Current != "written" {
		t.Errorf("Current = %q after re-render, want %q", got.Current, "written")
	}
	if want := `(b "written")`; dump(root.tree()) != want {
		t.Errorf("tree = %s, want %s", dump(root.tree()), want)
	}
}

func TestUseRefSlotsAreIndependentAndTyped(t *testing.T) {
	var comp Component = func(Props) Node {
		a := UseRef(1)
		b := UseRef("two")
		if a.Current != 1 || b.Current != "two" {
			t.Errorf("refs = %v/%v, want 1/two; slots must not be shared", a.Current, b.Current)
		}
		return nil
	}
	NewRoot(&Element{Type: comp, Props: Props{}})
}

func TestUseImperativeHandleAssignsInLayoutPhase(t *testing.T) {
	// The parent enqueues a passive effect before its child even renders, yet
	// the handle must already be installed by the time that effect runs:
	// every layout effect, including the handle assignment, drains before the
	// first passive effect.
	handle := &Ref[refTestHandle]{}
	var order []string

	var child Component = func(Props) Node {
		UseImperativeHandle(handle, func() refTestHandle {
			order = append(order, "assign")
			return refTestHandle{Name: "child", Call: func() string { return "called" }}
		}, []any{})
		return nil
	}

	var parent Component = func(Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() {
				order = append(order, "parent-passive:"+handle.Current.Name)
				return nil
			}, false)
		}
		return &Element{Type: child, Props: Props{}}
	}

	NewRoot(&Element{Type: parent, Props: Props{}})

	if got, want := strings.Join(order, ","), "assign,parent-passive:child"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if handle.Current.Call == nil || handle.Current.Call() != "called" {
		t.Error("published handle is not callable")
	}
}

func TestUseImperativeHandleVisibleToLaterLayoutEffect(t *testing.T) {
	// A layout effect enqueued after the handle owner rendered — here a
	// following sibling — observes the handle, because layout effects fire in
	// enqueue order within the layout phase.
	handle := &Ref[refTestHandle]{}
	seen := "<unset>"

	var owner Component = func(Props) Node {
		UseImperativeHandle(handle, func() refTestHandle {
			return refTestHandle{Name: "owner"}
		}, []any{})
		return nil
	}
	var reader Component = func(Props) Node {
		h, first := nextHook("layout")
		if first {
			enqueueEffect(h, func() func() {
				seen = handle.Current.Name
				return nil
			}, true)
		}
		return nil
	}

	NewRoot(host("div", "",
		&Element{Type: owner, Props: Props{}},
		&Element{Type: reader, Props: Props{}},
	))

	if seen != "owner" {
		t.Errorf("sibling layout effect saw %q, want %q", seen, "owner")
	}
}

func TestUseImperativeHandleRecomputesOnlyWhenDepsChange(t *testing.T) {
	cases := []struct {
		name        string
		deps        [][]any
		wantCreates int
	}{
		{"nil deps reassign every render", [][]any{nil, nil, nil}, 3},
		{"empty deps assign once", [][]any{{}, {}, {}}, 1},
		{"unchanged deps reuse the handle", [][]any{{"a"}, {"a"}}, 1},
		{"changed deps reassign", [][]any{{"a"}, {"b"}, {"b"}}, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handle := &Ref[refTestHandle]{}
			creates := 0
			render := 0

			var comp Component = func(Props) Node {
				deps := c.deps[render]
				render++
				UseImperativeHandle(handle, func() refTestHandle {
					creates++
					return refTestHandle{Name: "h"}
				}, deps)
				return nil
			}

			el := &Element{Type: comp, Props: Props{}}
			root := NewRoot(el)
			for range len(c.deps) - 1 {
				root.Render(el)
			}

			if creates != c.wantCreates {
				t.Errorf("create ran %d times, want %d", creates, c.wantCreates)
			}
			if handle.Current.Name != "h" {
				t.Errorf("handle = %+v, want it installed", handle.Current)
			}
		})
	}
}

func TestUseImperativeHandleClearsRefOnUnmount(t *testing.T) {
	handle := &Ref[refTestHandle]{}
	var comp Component = func(Props) Node {
		UseImperativeHandle(handle, func() refTestHandle {
			return refTestHandle{Name: "live"}
		}, []any{})
		return nil
	}

	root := NewRoot(&Element{Type: comp, Props: Props{}})
	if handle.Current.Name != "live" {
		t.Fatalf("handle = %+v, want it installed before unmount", handle.Current)
	}

	root.Unmount()

	if handle.Current.Name != "" {
		t.Errorf("handle = %+v after unmount, want the zero value", handle.Current)
	}
}

func TestUseImperativeHandleTolerateNilRef(t *testing.T) {
	// A nil ref must still claim its hook slot, so hook order stays stable for
	// a component whose parent sometimes forwards a ref and sometimes does not.
	var absent *Ref[refTestHandle]
	var comp Component = func(Props) Node {
		UseImperativeHandle(absent, func() refTestHandle { return refTestHandle{Name: "x"} }, []any{})
		r := UseRef(7)
		if r.Current != 7 {
			t.Errorf("following hook read %d, want 7; slot order shifted", r.Current)
		}
		return nil
	}

	el := &Element{Type: comp, Props: Props{}}
	root := NewRoot(el)
	root.Render(el) // must not panic with a hook-order diagnostic
}
