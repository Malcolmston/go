package react

import (
	"strings"
	"testing"
)

// effectTestLog is a tiny ordered recorder. Effect tests are almost entirely
// assertions about *when* something ran relative to something else, so nearly
// every case in this file boils down to comparing one joined string.
type effectTestLog struct{ entries []string }

func (l *effectTestLog) add(s string)   { l.entries = append(l.entries, s) }
func (l *effectTestLog) String() string { return strings.Join(l.entries, ",") }
func (l *effectTestLog) reset()         { l.entries = nil }
func (l *effectTestLog) len() int       { return len(l.entries) }

// effectTestElement wraps a component in an element with the given props. It
// exists only to keep the render calls below to one line each.
func effectTestElement(c Component, props Props) *Element {
	if props == nil {
		props = Props{}
	}
	return &Element{Type: c, Props: props}
}

// effectTestRegistrar names one of the three effect hooks so the shared tables
// below can run the identical case against all of them: the deps rules are one
// implementation and must behave identically whichever phase they run in.
type effectTestRegistrar struct {
	name     string
	register func(effect func() (cleanup func()), deps []any)
}

func effectTestRegistrars() []effectTestRegistrar {
	return []effectTestRegistrar{
		{"UseEffect", UseEffect},
		{"UseLayoutEffect", UseLayoutEffect},
		{"UseInsertionEffect", UseInsertionEffect},
	}
}

// TestEffectDepsSemantics is the heart of the hook: nil deps, empty deps and a
// real dependency list are three different behaviours, and confusing the first
// two is the mistake the Go signature makes easiest to commit (JavaScript can
// simply omit the argument; Go has to spell it nil).
func TestEffectDepsSemantics(t *testing.T) {
	cases := []struct {
		name string
		// deps holds the list passed on each successive render, so len(deps)
		// is also the number of renders the case performs.
		deps  [][]any
		fires int
	}{
		{"nil deps fire after every commit", [][]any{nil, nil, nil}, 3},
		{"empty deps fire on mount only", [][]any{{}, {}, {}}, 1},
		{"unchanged deps do not re-fire", [][]any{{1, "a"}, {1, "a"}, {1, "a"}}, 1},
		{"a changed dep re-fires", [][]any{{1}, {2}, {2}}, 2},
		{"a dep that changes back re-fires twice", [][]any{{1}, {2}, {1}}, 3},
		{"a length change re-fires", [][]any{{1}, {1, 2}}, 2},
		{"nil after a list re-fires every time", [][]any{{1}, nil, nil}, 3},
	}

	for _, reg := range effectTestRegistrars() {
		for _, c := range cases {
			t.Run(reg.name+"/"+c.name, func(t *testing.T) {
				fires, pass := 0, 0
				var comp Component = func(Props) Node {
					deps := c.deps[min(pass, len(c.deps)-1)]
					pass++
					reg.register(func() func() { fires++; return nil }, deps)
					return nil
				}

				root := effectTestRoot(comp)
				for range c.deps[1:] {
					root.Render(effectTestElement(comp, nil))
				}

				if pass != len(c.deps) {
					t.Fatalf("component rendered %d times, want %d", pass, len(c.deps))
				}
				if fires != c.fires {
					t.Errorf("effect fired %d times, want %d", fires, c.fires)
				}
			})
		}
	}
}

// effectTestRoot mounts a bare component with empty props.
func effectTestRoot(c Component) *Root { return NewRoot(effectTestElement(c, nil)) }

// TestEffectCleanupRunsBeforeNextBody pins the interleaving React guarantees on
// a re-firing effect: the previous cleanup first, then the new body — never two
// live setups at once, and never a teardown for a firing that did not happen.
func TestEffectCleanupRunsBeforeNextBody(t *testing.T) {
	for _, reg := range effectTestRegistrars() {
		t.Run(reg.name, func(t *testing.T) {
			var log effectTestLog
			var comp Component = func(p Props) Node {
				id := p.String("id")
				reg.register(func() func() {
					log.add("setup:" + id)
					return func() { log.add("cleanup:" + id) }
				}, []any{id})
				return nil
			}

			root := NewRoot(effectTestElement(comp, Props{"id": "a"}))
			if got, want := log.String(), "setup:a"; got != want {
				t.Fatalf("after mount = %q, want %q", got, want)
			}

			// A re-render with unchanged deps must not tear anything down.
			root.Render(effectTestElement(comp, Props{"id": "a"}))
			if got, want := log.String(), "setup:a"; got != want {
				t.Fatalf("after inert re-render = %q, want %q", got, want)
			}

			root.Render(effectTestElement(comp, Props{"id": "b"}))
			if got, want := log.String(), "setup:a,cleanup:a,setup:b"; got != want {
				t.Fatalf("after dep change = %q, want %q", got, want)
			}

			root.Unmount()
			if got, want := log.String(), "setup:a,cleanup:a,setup:b,cleanup:b"; got != want {
				t.Errorf("after unmount = %q, want %q", got, want)
			}
		})
	}
}

// TestEffectCleanupsRunLeafFirstOnUnmount mirrors the engine's own
// TestUnmountRunsCleanupsLeafFirst, but through the public hooks: a child's
// cleanup must never observe a parent that is already torn down. It also
// covers the removal-by-re-render path, which is where the queued layout and
// insertion cleanups could most easily be lost.
func TestEffectCleanupsRunLeafFirstOnUnmount(t *testing.T) {
	var log effectTestLog

	var leaf Component = func(Props) Node {
		UseEffect(func() func() { return func() { log.add("leaf-passive") } }, []any{})
		UseLayoutEffect(func() func() { return func() { log.add("leaf-layout") } }, []any{})
		return host("i", "")
	}
	var branch Component = func(Props) Node {
		UseInsertionEffect(func() func() { return func() { log.add("branch-insertion") } }, []any{})
		UseEffect(func() func() { return func() { log.add("branch-passive") } }, []any{})
		return host("div", "", effectTestElement(leaf, nil))
	}

	show := func(on bool) Node {
		if !on {
			return host("section", "")
		}
		return host("section", "", effectTestElement(branch, nil))
	}

	root := NewRoot(show(true))
	if log.len() != 0 {
		t.Fatalf("cleanups ran before unmount: %q", log.String())
	}

	root.Render(show(false))

	// Leaf before branch is the guarantee; within one fiber, cleanups run in
	// hook-call order.
	want := "leaf-passive,leaf-layout,branch-insertion,branch-passive"
	if got := log.String(); got != want {
		t.Errorf("cleanup order = %q, want %q", got, want)
	}
}

// TestEffectPhaseOrdering is the ordering contract across the whole commit:
// every insertion effect, then every layout effect, then every passive effect,
// each phase in tree order. Registering the hooks in the *reverse* order inside
// each component proves the phase — not the call site — is what decides.
func TestEffectPhaseOrdering(t *testing.T) {
	var log effectTestLog

	phases := func(who string) {
		UseEffectFn(func() { log.add(who + ":passive") }, nil)
		UseLayoutEffectFn(func() { log.add(who + ":layout") }, nil)
		UseInsertionEffect(func() func() { log.add(who + ":insertion"); return nil }, nil)
	}

	var child Component = func(Props) Node {
		phases("child")
		return nil
	}
	var parent Component = func(Props) Node {
		phases("parent")
		return effectTestElement(child, nil)
	}

	root := effectTestRoot(parent)

	want := strings.Join([]string{
		"parent:insertion", "child:insertion",
		"parent:layout", "child:layout",
		"parent:passive", "child:passive",
	}, ",")
	if got := log.String(); got != want {
		t.Fatalf("mount order = %q, want %q", got, want)
	}

	// The ordering is a property of every commit, not just the first: deps are
	// nil here, so a second render re-fires all six in the same sequence.
	log.reset()
	root.Render(effectTestElement(parent, nil))
	if got := log.String(); got != want {
		t.Errorf("re-render order = %q, want %q", got, want)
	}
}

// TestInsertionEffectPrecedesLayoutCleanup checks the finer half of the
// guarantee documented on [UseInsertionEffect]: insertion effects run before
// layout effects *including* their cleanups, so a layout effect can never tear
// down over the top of a style an insertion effect is in the middle of
// replacing. This is what the pump indirection buys over simply registering
// insertion effects in the runtime's layout phase.
func TestInsertionEffectPrecedesLayoutCleanup(t *testing.T) {
	var log effectTestLog

	var comp Component = func(p Props) Node {
		id := p.String("id")
		UseLayoutEffect(func() func() {
			log.add("layout-setup:" + id)
			return func() { log.add("layout-cleanup:" + id) }
		}, []any{id})
		UseInsertionEffect(func() func() {
			log.add("insertion-setup:" + id)
			return func() { log.add("insertion-cleanup:" + id) }
		}, []any{id})
		return nil
	}

	root := NewRoot(effectTestElement(comp, Props{"id": "a"}))
	log.reset()
	root.Render(effectTestElement(comp, Props{"id": "b"}))

	want := "insertion-cleanup:a,insertion-setup:b,layout-cleanup:a,layout-setup:b"
	if got := log.String(); got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}

// TestEffectFnVariantsNeedNoCleanup covers the convenience wrappers: they must
// share the deps rules and the hook slot layout of the full form, so that
// switching between them is not a behaviour change.
func TestEffectFnVariantsNeedNoCleanup(t *testing.T) {
	var log effectTestLog
	var comp Component = func(p Props) Node {
		id := p.String("id")
		UseEffectFn(func() { log.add("passive:" + id) }, []any{id})
		UseLayoutEffectFn(func() { log.add("layout:" + id) }, []any{})
		return nil
	}

	root := NewRoot(effectTestElement(comp, Props{"id": "a"}))
	root.Render(effectTestElement(comp, Props{"id": "b"}))
	root.Unmount() // must not panic on the nil cleanups

	want := "layout:a,passive:a,passive:b"
	if got := log.String(); got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

// effectTestMarkDirty is a stand-in for a state setter called from an effect
// body. It does what fiber.scheduleUpdate does — mark the fiber and its root as
// needing another pass — minus scheduleUpdate's eager call to Root.flush.
//
// That omission is deliberate and is a foundation limitation, not a choice:
// Root.flush holds renderMu for the whole pass *including* the effect commit,
// and Go mutexes are not reentrant, so a setter that calls flush from inside an
// effect body deadlocks. Marking dirty is enough for the case under test,
// because the flush loop re-checks the flag after runEffects returns and runs
// another pass — which is precisely the behaviour being asserted.
func effectTestMarkDirty(f *fiber) {
	f.markNeedsRender()
	r := f.root
	r.mu.Lock()
	r.dirty = true
	r.mu.Unlock()
}

// TestEffectSettingStateSettlesInOneExtraPass is the "subscribe, then deliver
// the first value" shape: an effect updates state, the flush loop notices and
// renders once more, and the tree is settled by the time the caller sees it —
// without looping forever, which would trip the runtime's pass limit.
func TestEffectSettingStateSettlesInOneExtraPass(t *testing.T) {
	for _, reg := range effectTestRegistrars() {
		t.Run(reg.name, func(t *testing.T) {
			renders, fires := 0, 0
			var comp Component = func(Props) Node {
				renders++
				h, first := nextHook("state")
				f := currentFiber()
				if first {
					h.value = 0
				}
				reg.register(func() func() {
					fires++
					h.value = 1
					effectTestMarkDirty(f)
					return nil
				}, []any{})
				return host("b", "", h.value.(int))
			}

			root := effectTestRoot(comp)

			if got, want := dump(root.tree()), `(b "1")`; got != want {
				t.Errorf("tree = %s, want %s", got, want)
			}
			if renders != 2 {
				t.Errorf("renders = %d, want 2 (mount plus the effect's update)", renders)
			}
			if fires != 1 {
				t.Errorf("effect fired %d times, want 1 (mount-only deps)", fires)
			}
		})
	}
}

// TestEffectSurvivesKeyedReorder is the case keys exist for. Moving a keyed
// child moves its fiber, so its hooks, its deps and its live cleanup move with
// it: the effect must not fire again, and above all must not be torn down and
// set up again, which for a real subscription would mean a dropped connection
// on every list sort.
func TestEffectSurvivesKeyedReorder(t *testing.T) {
	var log effectTestLog

	var item Component = func(p Props) Node {
		id := p.String("id")
		UseInsertionEffect(func() func() {
			log.add("insertion:" + id)
			return func() { log.add("insertion-cleanup:" + id) }
		}, []any{id})
		UseLayoutEffect(func() func() {
			log.add("layout:" + id)
			return func() { log.add("layout-cleanup:" + id) }
		}, []any{id})
		UseEffect(func() func() {
			log.add("passive:" + id)
			return func() { log.add("passive-cleanup:" + id) }
		}, []any{id})
		return host("li", "", id)
	}

	list := func(ids ...string) Node {
		var kids []Node
		for _, id := range ids {
			kids = append(kids, &Element{Type: item, Key: id, Props: Props{"id": id}})
		}
		return host("ul", "", kids)
	}

	root := NewRoot(list("a", "b", "c"))
	mounted := "insertion:a,insertion:b,insertion:c," +
		"layout:a,layout:b,layout:c," +
		"passive:a,passive:b,passive:c"
	if got := log.String(); got != mounted {
		t.Fatalf("mount log = %q, want %q", got, mounted)
	}

	log.reset()
	root.Render(list("c", "a", "b"))

	if got, want := dump(root.tree()), `(ul (li "c") (li "a") (li "b"))`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
	if log.len() != 0 {
		t.Errorf("a keyed move re-ran effects: %q", log.String())
	}

	// Dropping a key, on the other hand, is a real unmount and must clean up —
	// leaf-first, and only for the child that went away.
	log.reset()
	root.Render(list("c", "a"))
	want := "insertion-cleanup:b,layout-cleanup:b,passive-cleanup:b"
	if got := log.String(); got != want {
		t.Errorf("after removal = %q, want %q", got, want)
	}
}

// TestEffectBodyDoesNotRunDuringRender guards the separation the hook exists
// for: the component function may be called any number of times per commit, so
// nothing registered as an effect may run while it is on the stack.
func TestEffectBodyDoesNotRunDuringRender(t *testing.T) {
	var log effectTestLog
	var comp Component = func(Props) Node {
		log.add("render")
		UseInsertionEffect(func() func() { log.add("insertion"); return nil }, nil)
		UseLayoutEffectFn(func() { log.add("layout") }, nil)
		UseEffectFn(func() { log.add("passive") }, nil)
		log.add("render-end")
		return nil
	}

	effectTestRoot(comp)

	if got, want := log.String(), "render,render-end,insertion,layout,passive"; got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

// TestEffectHooksRejectNilBodies: a nil effect would otherwise fail deep inside
// the commit, far from the call site that is actually wrong.
func TestEffectHooksRejectNilBodies(t *testing.T) {
	calls := []struct {
		name string
		call func()
	}{
		{"UseEffect", func() { UseEffect(nil, nil) }},
		{"UseEffectFn", func() { UseEffectFn(nil, nil) }},
		{"UseLayoutEffect", func() { UseLayoutEffect(nil, nil) }},
		{"UseLayoutEffectFn", func() { UseLayoutEffectFn(nil, nil) }},
		{"UseInsertionEffect", func() { UseInsertionEffect(nil, nil) }},
	}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s(nil, nil) did not panic", c.name)
				}
				if !strings.Contains(toString(r), "non-nil effect function") {
					t.Errorf("panic = %v, want a nil-effect diagnostic", r)
				}
			}()
			var comp Component = func(Props) Node {
				c.call()
				return nil
			}
			effectTestRoot(comp)
		})
	}
}

// TestEffectHooksPanicOutsideRender: the hooks are ordinary functions with no
// receiver, so nothing stops a caller from invoking one from main(). The
// dispatcher check must catch it with React's own diagnostic.
func TestEffectHooksPanicOutsideRender(t *testing.T) {
	for _, reg := range effectTestRegistrars() {
		t.Run(reg.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s outside a render did not panic", reg.name)
				}
				if !strings.Contains(toString(r), "outside of a component render") {
					t.Errorf("panic = %v, want the dispatcher diagnostic", r)
				}
			}()
			reg.register(func() func() { return nil }, nil)
		})
	}
}
