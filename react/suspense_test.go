package react

import (
	"strings"
	"testing"
)

// boundaryHost builds a host element from raw children. The boundary tests
// build trees out of &Element literals so they stay independent of the element
// construction layer; this is just the literal, spelled once.
func boundaryHost(tag string, children ...Node) *Element {
	return &Element{Type: tag, Props: Props{ChildrenKey: children}}
}

// boundaryComp wraps a component function in an element with no props, the
// other literal these tests repeat.
func boundaryComp(c Component) *Element {
	return &Element{Type: c, Props: Props{}}
}

// suspenseRetries builds a Suspense boundary wired to notify, so a test can
// wait for an asynchronous retry to finish rather than sleeping. Every
// suspension the boundary catches produces exactly one send on notify, after
// the retry render has completed — so a receive is a happens-before edge and the
// tree is safe to inspect on the test goroutine afterwards.
func suspenseRetries(notify chan struct{}, fallback Node, children ...Node) *Element {
	el := Suspense(fallback, children...)
	el.Props[suspenseRetryNotifyKey] = notify
	return el
}

// suspenseNotify returns a generously buffered notification channel, so a retry
// goroutine never blocks on a test that has stopped listening.
func suspenseNotify() chan struct{} { return make(chan struct{}, 8) }

// suspendUntil returns a component that suspends on gate until it is closed and
// then renders (p "text"). Closing the gate is the test's only lever, which
// keeps every asynchronous test deterministic: nothing is timing-dependent.
func suspendUntil(gate chan struct{}, text string) *Element {
	return boundaryComp(func(Props) Node {
		select {
		case <-gate:
			return boundaryHost("p", text)
		default:
			Suspend(gate)
			return nil
		}
	})
}

func TestSuspenseShowsFallbackThenContent(t *testing.T) {
	gate := make(chan struct{})
	notify := suspenseNotify()

	root := NewRoot(boundaryHost("div",
		suspenseRetries(notify, boundaryHost("p", "loading"), suspendUntil(gate, "ready"))))

	if got, want := dump(root.tree()), `(div (p "loading"))`; got != want {
		t.Fatalf("while pending, tree = %s, want %s", got, want)
	}

	close(gate)
	<-notify

	if got, want := dump(root.tree()), `(div (p "ready"))`; got != want {
		t.Errorf("after resolving, tree = %s, want %s", got, want)
	}
}

func TestSuspenseBoundaryRearmsAfterResolving(t *testing.T) {
	// Two independent gates, selected by props. The slice is written before the
	// first render and never again, so the retry goroutines only ever read it.
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	notify := suspenseNotify()

	var probe Component = func(p Props) Node {
		g := gates[p.Get("gate").(int)]
		select {
		case <-g:
			return boundaryHost("p", p.String("label"))
		default:
			Suspend(g)
			return nil
		}
	}
	tree := func(gate int, label string) Node {
		return suspenseRetries(notify, boundaryHost("p", "loading"),
			&Element{Type: probe, Props: Props{"gate": gate, "label": label}})
	}

	root := NewRoot(tree(0, "one"))
	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("first suspension, tree = %s, want %s", got, want)
	}
	close(gates[0])
	<-notify
	if got, want := dump(root.tree()), `(p "one")`; got != want {
		t.Fatalf("after first resolution, tree = %s, want %s", got, want)
	}

	// A second suspension in an already-resolved boundary must show the
	// fallback again: the boundary re-arms rather than latching resolved.
	root.Render(tree(1, "two"))
	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("second suspension, tree = %s, want %s", got, want)
	}
	close(gates[1])
	<-notify
	if got, want := dump(root.tree()), `(p "two")`; got != want {
		t.Errorf("after second resolution, tree = %s, want %s", got, want)
	}
}

func TestSuspenseInnermostBoundaryCatches(t *testing.T) {
	gate := make(chan struct{})
	inner := suspenseNotify()
	outer := suspenseNotify()

	root := NewRoot(suspenseRetries(outer, boundaryHost("p", "outer-fallback"),
		boundaryHost("div",
			suspenseRetries(inner, boundaryHost("p", "inner-fallback"),
				suspendUntil(gate, "ready")))))

	if got, want := dump(root.tree()), `(div (p "inner-fallback"))`; got != want {
		t.Fatalf("tree = %s, want %s (the inner boundary must catch)", got, want)
	}
	select {
	case <-outer:
		t.Fatal("the outer boundary caught a suspension the inner one should have absorbed")
	default:
	}

	close(gate)
	<-inner
	if got, want := dump(root.tree()), `(div (p "ready"))`; got != want {
		t.Errorf("after resolving, tree = %s, want %s", got, want)
	}
}

func TestSuspenseSiblingSuspensionsResolveOneAtATime(t *testing.T) {
	// Rendering is depth-first and aborts at the first suspension, so two
	// suspending siblings under one boundary take two retries: the first
	// resolution uncovers the second suspension.
	first, second := make(chan struct{}), make(chan struct{})
	notify := suspenseNotify()

	root := NewRoot(suspenseRetries(notify, boundaryHost("p", "loading"),
		suspendUntil(first, "a"), suspendUntil(second, "b")))

	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("initial tree = %s, want %s", got, want)
	}

	close(first)
	<-notify
	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("after the first sibling resolved, tree = %s, want %s "+
			"(the second is still pending)", got, want)
	}

	close(second)
	<-notify
	if got, want := dump(root.tree()), `(p "a") (p "b")`; got != want {
		t.Errorf("after both resolved, tree = %s, want %s", got, want)
	}
}

func TestSuspenseDiscardsPartialSubtree(t *testing.T) {
	// A boundary that catches a suspension must leave nothing behind: fibers
	// that were committed in an earlier pass run their cleanups, and fibers that
	// mounted during the abandoned pass must not run their effects at all —
	// their effect would otherwise fire at commit for a component that is no
	// longer in the tree, and its cleanup would never run.
	var mounts, cleanups int
	var tracked Component = func(Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() {
				mounts++
				return func() { cleanups++ }
			}, false)
		}
		return boundaryHost("i")
	}

	gate := make(chan struct{})
	notify := suspenseNotify()
	keep := &Element{Type: tracked, Key: "keep", Props: Props{}}

	settled := suspenseRetries(notify, boundaryHost("p", "loading"),
		keep, boundaryHost("p", "content"))
	suspending := suspenseRetries(notify, boundaryHost("p", "loading"),
		keep,
		&Element{Type: tracked, Key: "fresh", Props: Props{}},
		suspendUntil(gate, "never"))

	root := NewRoot(settled)
	if mounts != 1 || cleanups != 0 {
		t.Fatalf("after mount: mounts = %d, cleanups = %d, want 1 and 0", mounts, cleanups)
	}

	root.Render(suspending)
	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
	if cleanups != 1 {
		t.Errorf("cleanups = %d, want 1: the committed child of the discarded "+
			"subtree must be unmounted", cleanups)
	}
	if mounts != 1 {
		t.Errorf("mounts = %d, want 1: the child that mounted during the "+
			"abandoned pass must not run its effect", mounts)
	}

	// Leave nothing running: the retry goroutine is parked on gate, and an
	// unmounted root makes its eventual scheduleUpdate a no-op.
	root.Unmount()
	close(gate)
	<-notify
}

func TestSuspenseDoesNotCatchErrors(t *testing.T) {
	// A Suspense boundary is not an error boundary: an ordinary panic must pass
	// straight through it, with its value intact.
	var boom Component = func(Props) Node { panic("kaboom") }

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected the panic to escape the Suspense boundary")
		}
		if got, ok := r.(string); !ok || got != "kaboom" {
			t.Errorf("recovered %#v, want the original \"kaboom\" panic", r)
		}
	}()

	NewRoot(Suspense(boundaryHost("p", "loading"), boundaryComp(boom)))
}

func TestSuspendWithNoBoundaryPanicsWithDiagnostic(t *testing.T) {
	gate := make(chan struct{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected an uncaught suspension to panic")
		}
		sig, ok := r.(SuspendSignal)
		if !ok {
			t.Fatalf("recovered %#v, want a SuspendSignal", r)
		}
		// The signal implements error so Go's panic printer renders the
		// diagnostic rather than a struct literal.
		if !strings.Contains(sig.Error(), "outside of any Suspense boundary") {
			t.Errorf("diagnostic = %q, want it to name the missing boundary", sig.Error())
		}
	}()

	NewRoot(boundaryHost("div", suspendUntil(gate, "never")))
}

func TestSuspenseFallbackSurvivesUnrelatedRerenders(t *testing.T) {
	// Re-rendering a pending boundary must keep the mounted fallback rather than
	// re-attempting the children and remounting the fallback underneath them.
	var fallbackMounts int
	var fallback Component = func(Props) Node {
		if _, first := nextHook("state"); first {
			fallbackMounts++
		}
		return boundaryHost("p", "loading")
	}

	gate := make(chan struct{})
	notify := suspenseNotify()
	tree := func(label string) Node {
		return boundaryHost("div", label,
			suspenseRetries(notify, boundaryComp(fallback), suspendUntil(gate, "ready")))
	}

	root := NewRoot(tree("a"))
	root.Render(tree("b"))
	root.Render(tree("c"))

	if got, want := dump(root.tree()), `(div "c" (p "loading"))`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
	if fallbackMounts != 1 {
		t.Errorf("fallback mounted %d times, want 1", fallbackMounts)
	}

	root.Unmount()
	close(gate)
	<-notify
}
