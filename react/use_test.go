package react

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// errUseTest is the sentinel failure these tests hand to [Failed] and to
// producer functions, so an assertion can compare with errors.Is instead of
// matching message text.
var errUseTest = errors.New("use_test: resource failed")

// useTestGated builds a pending [Async] together with the function that settles
// it. Every asynchrony in this file is driven through one of these: the
// producer goroutine parks on a channel the test owns, so "still pending" and
// "now settled" are states the test moves between explicitly rather than
// timings it hopes for. The release func is idempotent so a deferred cleanup
// can call it after the test already did.
func useTestGated[T any](value T, err error) (a *Async[T], release func()) {
	gate := make(chan struct{})
	var once sync.Once
	a = Go(func() (T, error) {
		<-gate
		return value, err
	})
	return a, func() { once.Do(func() { close(gate) }) }
}

// useTestRecover runs fn and returns whatever it panicked with, or nil. Tests
// for [Use]'s suspend and error paths need the raw panic value, and this slice
// owns neither Suspense nor ErrorBoundary, so it recovers the signals itself.
func useTestRecover(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

func TestAsyncSettledConstructors(t *testing.T) {
	panicked := Go(func() (int, error) { panic("boom") })
	<-panicked.Ready()

	cases := []struct {
		name      string
		a         *Async[int]
		wantValue int
		wantErr   error
	}{
		{"resolved carries its value", Resolved(7), 7, nil},
		{"failed carries its error", Failed[int](errUseTest), 0, errUseTest},
		{"failed with nil error is a success", Failed[int](nil), 0, nil},
		{"go returns the produced value", Go(func() (int, error) { return 3, nil }), 3, nil},
		{"go returns the produced error", Go(func() (int, error) { return 0, errUseTest }), 0, errUseTest},
	}

	for _, c := range cases {
		// Every constructor above settles without help, so Done must already be
		// true once Ready has closed.
		<-c.a.Ready()
		if !c.a.Done() {
			t.Errorf("%s: Done() = false after Ready closed", c.name)
		}

		value, err, ok := c.a.TryResult()
		if !ok {
			t.Errorf("%s: TryResult reported not settled", c.name)
			continue
		}
		if value != c.wantValue || !errors.Is(err, c.wantErr) {
			t.Errorf("%s: TryResult = (%v, %v), want (%v, %v)", c.name, value, err, c.wantValue, c.wantErr)
		}

		blockedValue, blockedErr := c.a.Result()
		if blockedValue != value || !errors.Is(blockedErr, c.wantErr) {
			t.Errorf("%s: Result = (%v, %v), want (%v, %v)", c.name, blockedValue, blockedErr, c.wantValue, c.wantErr)
		}
	}

	// The panicking producer is checked separately: its error is a wrapper, not
	// a sentinel, so it does not fit the table's comparison.
	if _, err, _ := panicked.TryResult(); err == nil {
		t.Error("panicking producer: want an error, got nil")
	}
}

func TestAsyncPendingUntilReleased(t *testing.T) {
	a, release := useTestGated(42, nil)

	if a.Done() {
		t.Fatal("Done() = true before the producer was released")
	}
	if _, _, ok := a.TryResult(); ok {
		t.Fatal("TryResult reported settled before the producer was released")
	}
	select {
	case <-a.Ready():
		t.Fatal("Ready closed before the producer was released")
	default:
	}

	release()

	// A blocking read is the synchronization: it returns exactly when the
	// producer goroutine has settled the resource.
	if v, err := a.Result(); v != 42 || err != nil {
		t.Errorf("Result = (%v, %v), want (42, nil)", v, err)
	}
	if !a.Done() {
		t.Error("Done() = false after Result returned")
	}
}

func TestAsyncTerminatesWithNoReader(t *testing.T) {
	// A resource nobody reads must still finish: the producer only writes fields
	// and closes a channel, so there is no unread send to park on. Observing the
	// close after the fact is what proves the goroutine is gone rather than
	// blocked.
	ran := make(chan struct{})
	a := Go(func() (string, error) {
		close(ran)
		return "ignored", nil
	})
	<-ran
	<-a.Ready()
}

func TestAsyncPanicBecomesError(t *testing.T) {
	cases := []struct {
		name       string
		panicValue any
		wantIs     error
	}{
		{"string panic", "kaboom", nil},
		{"error panic unwraps", errUseTest, errUseTest},
		{"nil-ish panic value", 0, nil},
	}

	for _, c := range cases {
		pv := c.panicValue
		a := Go(func() (int, error) { panic(pv) })

		v, err := a.Result()
		if v != 0 {
			t.Errorf("%s: value = %v, want the zero value", c.name, v)
		}
		var ap *AsyncPanic
		if !errors.As(err, &ap) {
			t.Errorf("%s: err = %v, want an *AsyncPanic", c.name, err)
			continue
		}
		if ap.Value != c.panicValue {
			t.Errorf("%s: preserved panic value = %v, want %v", c.name, ap.Value, c.panicValue)
		}
		if len(ap.Stack) == 0 {
			t.Errorf("%s: captured stack is empty", c.name)
		}
		if c.wantIs != nil && !errors.Is(err, c.wantIs) {
			t.Errorf("%s: errors.Is(err, sentinel) = false, want true", c.name)
		}
	}
}

func TestUseReturnsSettledValueDuringRender(t *testing.T) {
	greeting := Resolved("hello")
	var comp Component = func(Props) Node {
		return &Element{Type: "p", Props: Props{ChildrenKey: []Node{Use(greeting)}}}
	}

	root := NewRoot(&Element{Type: comp, Props: Props{}})
	if got, want := dump(root.tree()), `(p "hello")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseSuspendsOnPendingResource(t *testing.T) {
	a, release := useTestGated("late", nil)
	defer release()

	rendered := 0
	var comp Component = func(Props) Node {
		rendered++
		return &Element{Type: "p", Props: Props{ChildrenKey: []Node{Use(a)}}}
	}

	// No Suspense boundary exists in this slice's tests, so the signal travels
	// all the way out of the render and the test plays the boundary's part.
	got := useTestRecover(func() { NewRoot(&Element{Type: comp, Props: Props{}}) })

	sig, ok := got.(SuspendSignal)
	if !ok {
		t.Fatalf("recovered %#v, want a SuspendSignal", got)
	}
	if rendered != 1 {
		t.Errorf("component rendered %d times, want 1", rendered)
	}
	select {
	case <-sig.Ready:
		t.Fatal("SuspendSignal.Ready was already closed while the resource was pending")
	default:
	}

	// The contract a boundary relies on: settling the resource closes the very
	// channel the signal handed out, which is its cue to retry.
	release()
	<-sig.Ready

	if v, err := a.Result(); v != "late" || err != nil {
		t.Errorf("Result = (%v, %v), want (late, nil)", v, err)
	}
}

func TestUsePanicsFailedResourceError(t *testing.T) {
	cases := []struct {
		name string
		a    *Async[int]
		want error
	}{
		{"already failed", Failed[int](errUseTest), errUseTest},
		{"failed by its producer", Go(func() (int, error) { return 0, errUseTest }), errUseTest},
	}

	for _, c := range cases {
		res := c.a
		var comp Component = func(Props) Node { return Use(res) }

		<-res.Ready() // settle before rendering so Use takes the error path, not the suspend path
		got := useTestRecover(func() { NewRoot(&Element{Type: comp, Props: Props{}}) })

		err, ok := got.(error)
		if !ok {
			t.Errorf("%s: recovered %#v, want an error", c.name, got)
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("%s: panicked %v, want %v", c.name, err, c.want)
		}
	}
}

func TestUseIsLegalConditionallyAndInLoops(t *testing.T) {
	// Use must be callable where no other hook may be: behind an if and inside a
	// loop whose trip count changes between renders. The component brackets
	// those calls with real hook slots, so a stray nextHook inside Use would
	// shift the cursor and trip fiber.go's hook-order panic.
	before := Resolved("B")
	optional := Resolved("O")
	items := []*Async[string]{Resolved("0"), Resolved("1"), Resolved("2")}

	var comp Component = func(p Props) Node {
		leading, firstLeading := nextHook("state")
		if firstLeading {
			leading.value = 0
		}
		leading.value = leading.value.(int) + 1

		out := []Node{Use(before)}

		if flag, _ := p.Bool("flag"); flag {
			out = append(out, Use(optional))
		}
		n := len(items)
		if flag, _ := p.Bool("flag"); !flag {
			n = 1
		}
		for i := range n {
			out = append(out, Use(items[i]))
		}

		trailing, firstTrailing := nextHook("effect")
		if firstTrailing {
			trailing.value = "kept"
		}
		out = append(out, trailing.value.(string))

		return &Element{Type: "div", Props: Props{ChildrenKey: out}}
	}

	root := NewRoot(&Element{Type: comp, Props: Props{"flag": true}})
	if got, want := dump(root.tree()), `(div "B" "O" "0" "1" "2" "kept")`; got != want {
		t.Fatalf("first render = %s, want %s", got, want)
	}

	// A different branch and a different loop length on the second render: legal
	// for Use, fatal for any hook.
	root.Render(&Element{Type: comp, Props: Props{"flag": false}})
	if got, want := dump(root.tree()), `(div "B" "0" "kept")`; got != want {
		t.Fatalf("second render = %s, want %s", got, want)
	}

	// The bracketing slots kept their identity across both renders, which is the
	// proof that the hook cursor never drifted.
	var renders int
	walk(root.tree(), func(f *fiber) {
		if len(f.hooks) == 0 {
			return
		}
		if len(f.hooks) != 2 {
			t.Errorf("fiber has %d hook slots, want 2", len(f.hooks))
			return
		}
		renders = f.hooks[0].value.(int)
	})
	if renders != 2 {
		t.Errorf("state slot = %d, want 2 (one per render, same slot both times)", renders)
	}
}

func TestUseOutsideRenderBlocksInsteadOfSuspending(t *testing.T) {
	a, release := useTestGated("eventually", nil)

	// Off-render there is no boundary to catch a suspension, so Use degrades to
	// a blocking read. The result channel is what proves it waited rather than
	// panicking or returning a zero value.
	result := make(chan string, 1)
	go func() { result <- Use(a) }()

	select {
	case v := <-result:
		t.Fatalf("Use returned %q before the resource settled", v)
	default:
	}

	release()
	if got := <-result; got != "eventually" {
		t.Errorf("Use = %q, want %q", got, "eventually")
	}
}

func TestUseOutsideRenderPanicsOnFailure(t *testing.T) {
	got := useTestRecover(func() { Use(Failed[int](errUseTest)) })
	err, ok := got.(error)
	if !ok || !errors.Is(err, errUseTest) {
		t.Errorf("recovered %#v, want %v", got, errUseTest)
	}
}

func TestUseNilResourcePanicsWithDiagnostic(t *testing.T) {
	got := useTestRecover(func() { Use[int](nil) })
	if got == nil {
		t.Fatal("expected a panic for a nil *Async")
	}
	if !strings.Contains(toString(got), "nil *Async") {
		t.Errorf("panic = %v, want a message naming the nil resource", got)
	}
}
