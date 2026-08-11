package react

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// cacheTestLoader builds a loader whose every call parks on a gate the test
// owns, along with the call counter and the release func. Single-flight is only
// observable while the work is in flight, so the loader has to be stoppable —
// a loader that returned immediately would make "one call" and "many calls that
// happened to be serialized" indistinguishable.
func cacheTestLoader() (load func(string) (string, error), calls *atomic.Int64, release func()) {
	gate := make(chan struct{})
	var once sync.Once
	counter := &atomic.Int64{}

	load = func(key string) (string, error) {
		counter.Add(1)
		<-gate
		if key == "bad" {
			return "", errUseTest
		}
		return "value:" + key, nil
	}
	return load, counter, func() { once.Do(func() { close(gate) }) }
}

func TestCacheSharesHandlesPerKey(t *testing.T) {
	load, calls, release := cacheTestLoader()
	release() // this test cares about identity, not about timing
	get := Cache(load)

	a1, a2 := get("x"), get("x")
	b := get("y")

	cases := []struct {
		name     string
		got      bool
		want     bool
		describe string
	}{
		{"same key yields the identical handle", a1 == a2, true, "get(x) twice"},
		{"different keys yield different handles", a1 == b, false, "get(x) vs get(y)"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %s compared %v, want %v", c.name, c.describe, c.got, c.want)
		}
	}

	if v, err := a1.Result(); v != "value:x" || err != nil {
		t.Errorf("x = (%q, %v), want (\"value:x\", nil)", v, err)
	}
	if v, err := b.Result(); v != "value:y" || err != nil {
		t.Errorf("y = (%q, %v), want (\"value:y\", nil)", v, err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("loader calls = %d, want 2 (one per distinct key)", got)
	}
}

func TestCacheIsSingleFlightUnderConcurrency(t *testing.T) {
	load, calls, release := cacheTestLoader()
	get := Cache(load)

	const goroutines = 32

	// Every goroutine is released from the same starting line, so they contend
	// on the cold key rather than arriving one after another.
	start := make(chan struct{})
	handles := make(chan *Async[string], goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			handles <- get("hot")
		}()
	}
	close(start)
	wg.Wait()
	close(handles)

	first := <-handles
	shared := 1
	for a := range handles {
		if a != first {
			t.Fatal("concurrent Get returned different handles for the same key")
		}
		shared++
	}
	if shared != goroutines {
		t.Errorf("collected %d handles, want %d", shared, goroutines)
	}

	// The loader is still parked at this point, so the count below is the number
	// of flights actually started, not the number that happened to finish.
	if got := calls.Load(); got != 1 {
		t.Errorf("loader calls = %d, want 1 (single flight)", got)
	}

	release()
	if v, err := first.Result(); v != "value:hot" || err != nil {
		t.Errorf("Result = (%q, %v), want (\"value:hot\", nil)", v, err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("loader calls after settling = %d, want 1", got)
	}
}

func TestCachedInvalidateAndClear(t *testing.T) {
	load, calls, release := cacheTestLoader()
	release()
	c := NewCache(load)

	a := c.Get("k")
	if _, err := a.Result(); err != nil {
		t.Fatalf("initial load failed: %v", err)
	}

	if peeked, ok := c.Peek("k"); !ok || peeked != a {
		t.Errorf("Peek(k) = (%v, %v), want the cached handle", peeked, ok)
	}
	if _, ok := c.Peek("absent"); ok {
		t.Error("Peek(absent) reported a hit; Peek must not load")
	}
	if got := c.Len(); got != 1 {
		t.Errorf("Len = %d, want 1", got)
	}

	if !c.Invalidate("k") {
		t.Error("Invalidate(k) = false, want true for a present key")
	}
	if c.Invalidate("k") {
		t.Error("Invalidate(k) twice = true, want false the second time")
	}

	// A handle taken before invalidation keeps working: invalidation stops the
	// cache handing it out, it does not abandon anyone already waiting on it.
	if v, err := a.Result(); v != "value:k" || err != nil {
		t.Errorf("stale handle = (%q, %v), want it still usable", v, err)
	}

	fresh := c.Get("k")
	if fresh == a {
		t.Error("Get after Invalidate returned the old handle")
	}
	if _, err := fresh.Result(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("loader calls = %d, want 2 (reloaded after invalidation)", got)
	}

	c.Get("other")
	if got := c.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2 before Clear", got)
	}
	c.Clear()
	if got := c.Len(); got != 0 {
		t.Errorf("Len after Clear = %d, want 0", got)
	}
	if _, ok := c.Peek("k"); ok {
		t.Error("Peek(k) hit after Clear")
	}
}

func TestCacheRemembersFailuresUntilInvalidated(t *testing.T) {
	load, calls, release := cacheTestLoader()
	release()
	c := NewCache(load)

	if _, err := c.Get("bad").Result(); !errors.Is(err, errUseTest) {
		t.Fatalf("err = %v, want %v", err, errUseTest)
	}
	if _, err := c.Get("bad").Result(); !errors.Is(err, errUseTest) {
		t.Fatalf("second err = %v, want %v", err, errUseTest)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("loader calls = %d, want 1 (a failure is cached like any result)", got)
	}

	c.Invalidate("bad")
	if _, err := c.Get("bad").Result(); !errors.Is(err, errUseTest) {
		t.Fatalf("err after invalidation = %v, want %v", err, errUseTest)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("loader calls = %d, want 2 (invalidation is the retry)", got)
	}
}

func TestCachePanickingLoaderSurfacesAsError(t *testing.T) {
	c := NewCache(func(key string) (int, error) { panic("loader exploded: " + key) })

	_, err := c.Get("k").Result()
	var ap *AsyncPanic
	if !errors.As(err, &ap) {
		t.Fatalf("err = %v, want an *AsyncPanic", err)
	}
	if got, _ := ap.Value.(string); !strings.Contains(got, "loader exploded: k") {
		t.Errorf("preserved panic value = %v, want the loader's message", ap.Value)
	}
}

func TestNewCacheRejectsNilLoader(t *testing.T) {
	got := useTestRecover(func() { NewCache[string, int](nil) })
	if got == nil {
		t.Fatal("expected a panic for a nil loader")
	}
	if !strings.Contains(toString(got), "non-nil loader") {
		t.Errorf("panic = %v, want a message naming the nil loader", got)
	}
}

func TestCachedResourceRendersThroughUse(t *testing.T) {
	// The end-to-end shape this slice exists for: a cached loader feeding Use,
	// with two components reading the same key and therefore the same handle.
	load, calls, release := cacheTestLoader()
	release()
	get := Cache(load)

	// Warm the entry before rendering: a freshly started load is still pending,
	// and this slice owns no Suspense boundary to catch the suspension that Use
	// would then raise. Warming keeps the test about sharing, not about timing.
	if _, err := get("same").Result(); err != nil {
		t.Fatalf("warm-up failed: %v", err)
	}

	var leaf Component = func(p Props) Node {
		return &Element{Type: "b", Props: Props{ChildrenKey: []Node{Use(get(p.String("id")))}}}
	}
	tree := &Element{Type: "div", Props: Props{ChildrenKey: []Node{
		&Element{Type: leaf, Key: "1", Props: Props{"id": "same"}},
		&Element{Type: leaf, Key: "2", Props: Props{"id": "same"}},
	}}}

	root := NewRoot(tree)
	if got, want := dump(root.tree()), `(div (b "value:same") (b "value:same"))`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("loader calls = %d, want 1 (both components shared the handle)", got)
	}
}
