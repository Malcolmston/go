package react

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// errorText is the fallback these tests use: it renders the recovered value as
// a paragraph so a single dump comparison asserts both that the boundary caught
// and what it caught.
func errorText(err any) Node { return boundaryHost("p", fmt.Sprint(err)) }

func TestErrorBoundaryCatchesRenderPanics(t *testing.T) {
	cases := []struct {
		name  string
		panic func()
		want  string
		check func(t *testing.T, recovered any)
	}{
		{
			name:  "string panic",
			panic: func() { panic("kaboom") },
			want:  `(div (p "kaboom"))`,
		},
		{
			name:  "error panic",
			panic: func() { panic(errors.New("wrapped")) },
			want:  `(div (p "wrapped"))`,
			check: func(t *testing.T, recovered any) {
				if _, ok := recovered.(error); !ok {
					t.Errorf("recovered %#v, want the original error value", recovered)
				}
			},
		},
		{
			name: "runtime error",
			// A genuine bug, not a deliberate panic. The boundary catches it —
			// but hands the runtime.Error through unchanged so nothing about the
			// diagnostic is lost.
			panic: func() {
				var m map[string]int
				m["x"] = 1
			},
			check: func(t *testing.T, recovered any) {
				if _, ok := recovered.(runtime.Error); !ok {
					t.Fatalf("recovered %#v, want the runtime.Error itself", recovered)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var recovered any
			var boom Component = func(Props) Node {
				c.panic()
				return nil
			}
			root := NewRoot(boundaryHost("div",
				ErrorBoundary(func(err any) Node {
					recovered = err
					return errorText(err)
				}, boundaryComp(boom))))

			if recovered == nil {
				t.Fatal("the boundary never rendered its fallback")
			}
			if c.want != "" {
				if got := dump(root.tree()); got != c.want {
					t.Errorf("tree = %s, want %s", got, c.want)
				}
			}
			if c.check != nil {
				c.check(t, recovered)
			}
		})
	}
}

func TestErrorBoundaryDoesNotCatchSuspensions(t *testing.T) {
	// A suspension is control flow, not a failure. An ErrorBoundary between the
	// suspending component and its Suspense boundary must be transparent to it.
	gate := make(chan struct{})
	notify := suspenseNotify()
	caught := false

	root := NewRoot(suspenseRetries(notify, boundaryHost("p", "loading"),
		ErrorBoundary(func(err any) Node {
			caught = true
			return errorText(err)
		}, suspendUntil(gate, "ready"))))

	if caught {
		t.Fatal("the ErrorBoundary swallowed a SuspendSignal")
	}
	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	close(gate)
	<-notify

	if caught {
		t.Error("the ErrorBoundary rendered its fallback for a suspension")
	}
	if got, want := dump(root.tree()), `(p "ready")`; got != want {
		t.Errorf("after resolving, tree = %s, want %s", got, want)
	}
}

func TestErrorBoundaryInnermostCatches(t *testing.T) {
	var boom Component = func(Props) Node { panic("inner failure") }
	outerCaught := false

	root := NewRoot(ErrorBoundary(func(err any) Node {
		outerCaught = true
		return boundaryHost("p", "outer")
	}, boundaryHost("div",
		ErrorBoundary(errorText, boundaryComp(boom)))))

	if outerCaught {
		t.Error("the outer boundary caught an error the inner one handled")
	}
	if got, want := dump(root.tree()), `(div (p "inner failure"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestErrorBoundaryDiscardsPartialSubtree(t *testing.T) {
	// Same contract as the Suspense boundary: committed children of the failed
	// subtree run their cleanups, and children that mounted during the failed
	// pass never run their effects.
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
	var boom Component = func(p Props) Node {
		if fail, _ := p.Bool("fail"); fail {
			panic("boom")
		}
		return boundaryHost("p", "content")
	}

	keep := &Element{Type: tracked, Key: "keep", Props: Props{}}
	tree := func(fail bool) Node {
		children := []Node{keep}
		if fail {
			children = append(children, &Element{Type: tracked, Key: "fresh", Props: Props{}})
		}
		children = append(children, &Element{Type: boom, Props: Props{"fail": fail}})
		return ErrorBoundary(errorText, children...)
	}

	root := NewRoot(tree(false))
	if mounts != 1 || cleanups != 0 {
		t.Fatalf("after mount: mounts = %d, cleanups = %d, want 1 and 0", mounts, cleanups)
	}

	root.Render(tree(true))
	if got, want := dump(root.tree()), `(p "boom")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}
	if cleanups != 1 {
		t.Errorf("cleanups = %d, want 1", cleanups)
	}
	if mounts != 1 {
		t.Errorf("mounts = %d, want 1: an effect from the failed pass must not fire", mounts)
	}
}

func TestErrorBoundaryRecoversWhenChildrenStopPanicking(t *testing.T) {
	// The Go shape has no setState-based reset, so the boundary re-attempts its
	// children every render and recovers on its own.
	var boom Component = func(p Props) Node {
		if fail, _ := p.Bool("fail"); fail {
			panic("transient")
		}
		return boundaryHost("p", "healthy")
	}
	tree := func(fail bool) Node {
		return ErrorBoundary(errorText, &Element{Type: boom, Props: Props{"fail": fail}})
	}

	root := NewRoot(tree(true))
	if got, want := dump(root.tree()), `(p "transient")`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	root.Render(tree(false))
	if got, want := dump(root.tree()), `(p "healthy")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestErrorBoundaryDoesNotCatchEffectPanics(t *testing.T) {
	// Effects run after the commit, off the boundary's call stack — exactly
	// React's rule that error boundaries catch errors during rendering only.
	var c Component = func(Props) Node {
		h, first := nextHook("effect")
		if first {
			enqueueEffect(h, func() func() { panic("effect exploded") }, false)
		}
		return nil
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected an effect panic to escape the ErrorBoundary")
		}
		if got, _ := r.(string); got != "effect exploded" {
			t.Errorf("recovered %#v, want the original effect panic", r)
		}
	}()

	NewRoot(ErrorBoundary(errorText, boundaryComp(c)))
}

func TestErrorBoundaryFallbackPanicIsAttributed(t *testing.T) {
	// A fallback that fails while handling an error must not be mistaken for the
	// original failure, so it is re-panicked wrapped in a RecoveredError.
	var boom Component = func(Props) Node { panic("original") }

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected the fallback's panic to escape")
		}
		re, ok := r.(RecoveredError)
		if !ok {
			t.Fatalf("recovered %#v, want a RecoveredError", r)
		}
		if !strings.Contains(re.Error(), "fallback") {
			t.Errorf("diagnostic = %q, want it to name the fallback", re.Error())
		}
		if got, _ := re.Value.(string); got != "fallback exploded" {
			t.Errorf("wrapped value = %#v, want the fallback's own panic", re.Value)
		}
	}()

	NewRoot(ErrorBoundary(func(any) Node { panic("fallback exploded") }, boundaryComp(boom)))
}

func TestRecoveredErrorUnwraps(t *testing.T) {
	inner := errors.New("disk on fire")
	cases := []struct {
		name      string
		err       RecoveredError
		wantIs    bool
		wantParts []string
	}{
		{
			name:      "named component wrapping an error",
			err:       RecoveredError{Value: inner, Component: "react.Lazy"},
			wantIs:    true,
			wantParts: []string{"react.Lazy", "disk on fire"},
		},
		{
			name:      "anonymous string panic",
			err:       RecoveredError{Value: "oops"},
			wantIs:    false,
			wantParts: []string{"component failed", "oops"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := errors.Is(c.err, inner); got != c.wantIs {
				t.Errorf("errors.Is = %v, want %v", got, c.wantIs)
			}
			for _, part := range c.wantParts {
				if !strings.Contains(c.err.Error(), part) {
					t.Errorf("Error() = %q, want it to mention %q", c.err.Error(), part)
				}
			}
		})
	}
}
