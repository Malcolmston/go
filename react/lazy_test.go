package react

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// lazyLabel is the component the lazy tests load: it renders its "label" prop,
// which proves the loaded component receives the lazy element's own props.
var lazyLabel Component = func(p Props) Node { return boundaryHost("p", p.String("label")) }

// lazyElement is the &Element literal a lazy type is used through.
func lazyElement(lt *LazyType, key, label string) *Element {
	return &Element{Type: lt, Key: key, Props: Props{"label": label}}
}

func TestLazyLoadsOnceAcrossElementsAndRoots(t *testing.T) {
	// The loader blocks until the test releases it, so every phase of this test
	// is driven by a channel rather than by timing.
	release := make(chan struct{})
	var loads atomic.Int32
	lt := Lazy(func() (Component, error) {
		<-release
		loads.Add(1)
		return lazyLabel, nil
	})

	notify := suspenseNotify()
	root := NewRoot(suspenseRetries(notify, boundaryHost("p", "loading"),
		lazyElement(lt, "one", "one"), lazyElement(lt, "two", "two")))

	if got, want := dump(root.tree()), `(p "loading")`; got != want {
		t.Fatalf("while loading, tree = %s, want %s", got, want)
	}

	close(release)
	<-notify

	if got, want := dump(root.tree()), `(p "one") (p "two")`; got != want {
		t.Fatalf("after loading, tree = %s, want %s", got, want)
	}
	if got := loads.Load(); got != 1 {
		t.Errorf("loads = %d, want 1: two elements share one LazyType", got)
	}

	// A second Root mounting the same LazyType neither reloads nor suspends,
	// because the load already finished — so this renders synchronously.
	second := NewRoot(suspenseRetries(suspenseNotify(), boundaryHost("p", "loading"),
		lazyElement(lt, "three", "three")))
	if got, want := dump(second.tree()), `(p "three")`; got != want {
		t.Errorf("second root tree = %s, want %s", got, want)
	}
	if got := loads.Load(); got != 1 {
		t.Errorf("loads = %d, want 1: a second Root must reuse the loaded component", got)
	}
}

func TestLazyRetriesDoNotReload(t *testing.T) {
	// Rendering the boundary repeatedly while the load is outstanding must not
	// start a second load: sync.Once, not "once per render".
	release := make(chan struct{})
	var loads atomic.Int32
	lt := Lazy(func() (Component, error) {
		<-release
		loads.Add(1)
		return lazyLabel, nil
	})

	notify := suspenseNotify()
	tree := func(marker string) Node {
		return boundaryHost("div", marker,
			suspenseRetries(notify, boundaryHost("p", "loading"), lazyElement(lt, "", "done")))
	}

	root := NewRoot(tree("a"))
	root.Render(tree("b"))
	root.Render(tree("c"))

	if got, want := dump(root.tree()), `(div "c" (p "loading"))`; got != want {
		t.Fatalf("tree = %s, want %s", got, want)
	}

	close(release)
	<-notify

	if got, want := dump(root.tree()), `(div "c" (p "done"))`; got != want {
		t.Errorf("after loading, tree = %s, want %s", got, want)
	}
	if got := loads.Load(); got != 1 {
		t.Errorf("loads = %d, want 1 across three render passes and a retry", got)
	}
}

func TestLazyLoadErrorReachesErrorBoundary(t *testing.T) {
	cases := []struct {
		name   string
		load   func(release chan struct{}) func() (Component, error)
		want   string
		unwrap bool
	}{
		{
			name: "loader returns an error",
			load: func(release chan struct{}) func() (Component, error) {
				return func() (Component, error) { <-release; return nil, errLazyTest }
			},
			want:   errLazyTest.Error(),
			unwrap: true,
		},
		{
			name: "loader panics",
			load: func(release chan struct{}) func() (Component, error) {
				return func() (Component, error) { <-release; panic("loader blew up") }
			},
			want: "loader panicked",
		},
		{
			name: "loader returns a nil component",
			load: func(release chan struct{}) func() (Component, error) {
				return func() (Component, error) { <-release; return nil, nil }
			},
			want: "nil Component",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release := make(chan struct{})
			lt := Lazy(c.load(release))

			var caught any
			notify := suspenseNotify()
			root := NewRoot(suspenseRetries(notify, boundaryHost("p", "loading"),
				ErrorBoundary(func(err any) Node {
					caught = err
					return boundaryHost("p", "failed")
				}, lazyElement(lt, "", "unused"))))

			if got, want := dump(root.tree()), `(p "loading")`; got != want {
				t.Fatalf("while loading, tree = %s, want %s", got, want)
			}

			close(release)
			<-notify

			if got, want := dump(root.tree()), `(p "failed")`; got != want {
				t.Fatalf("after a failed load, tree = %s, want %s", got, want)
			}

			re, ok := caught.(RecoveredError)
			if !ok {
				t.Fatalf("boundary caught %#v, want a RecoveredError", caught)
			}
			if re.Component != "react.Lazy" {
				t.Errorf("Component = %q, want %q", re.Component, "react.Lazy")
			}
			if !strings.Contains(re.Error(), c.want) {
				t.Errorf("Error() = %q, want it to mention %q", re.Error(), c.want)
			}
			if c.unwrap && !errors.Is(re, errLazyTest) {
				t.Error("errors.Is could not see through the RecoveredError")
			}
		})
	}
}

// errLazyTest is the sentinel a failing loader returns, so the test can assert
// that errors.Is still finds it after the runtime wrapped it.
var errLazyTest = errors.New("no such chunk")

func TestLazyRejectsNilLoader(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Lazy(nil) to panic")
		}
		if !strings.Contains(toString(r), "non-nil loader") {
			t.Errorf("panic = %v, want a diagnostic naming the loader", r)
		}
	}()
	Lazy(nil)
}
