package react

import (
	"sync"
	"sync/atomic"
	"testing"
)

// storeTestHost builds a host element without going through CreateElement, so
// these tests exercise the store hook and nothing else. It is spelled with a
// storeTest prefix because several test files in this package need the same
// two-line helper.
func storeTestHost(tag string, children ...Node) *Element {
	return &Element{Type: tag, Props: Props{ChildrenKey: children}}
}

// storeTestCounter is the subscribe/unsubscribe bookkeeping every test below
// needs: it wraps a real subscription and counts both halves, so a test can
// assert that the hook subscribed once and unsubscribed exactly once.
type storeTestCounter struct {
	subs   atomic.Int32
	unsubs atomic.Int32
}

func (c *storeTestCounter) wrap(st *Store[int]) func(func()) func() {
	return func(onStoreChange func()) func() {
		c.subs.Add(1)
		unsubscribe := st.Subscribe(onStoreChange)
		return func() {
			c.unsubs.Add(1)
			unsubscribe()
		}
	}
}

func TestUseSyncExternalStoreRendersSnapshotAndTracksChanges(t *testing.T) {
	st := NewStore(0)
	var c Component = func(Props) Node {
		return storeTestHost("b", UseSyncExternalStore(st.Subscribe, st.Get))
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})
	if got, want := root.Tree().TextContent(), "0"; got != want {
		t.Fatalf("initial text = %q, want %q", got, want)
	}

	// A write from outside a render settles synchronously: the notification
	// schedules an update and the update flushes before the write returns.
	cases := []struct {
		name  string
		write func()
		want  string
	}{
		{"Set", func() { st.Set(1) }, "1"},
		{"Set again", func() { st.Set(2) }, "2"},
		{"Update", func() { st.Update(func(v int) int { return v * 20 }) }, "40"},
	}
	for _, tc := range cases {
		tc.write()
		if got := root.Tree().TextContent(); got != tc.want {
			t.Errorf("%s: text = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUseSyncExternalStoreBailsOutOnEqualSnapshot(t *testing.T) {
	st := NewStore(7)
	var renders atomic.Int32
	var c Component = func(Props) Node {
		renders.Add(1)
		return storeTestHost("b", UseSyncExternalStore(st.Subscribe, st.Get))
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})
	if got := renders.Load(); got != 1 {
		t.Fatalf("renders after mount = %d, want 1", got)
	}

	cases := []struct {
		name        string
		set         int
		wantRenders int32
		wantText    string
	}{
		// Store notifies unconditionally; the hook, not the store, decides that
		// nothing changed.
		{"same value notifies but does not render", 7, 1, "7"},
		{"different value renders", 8, 2, "8"},
		{"same value again still does not render", 8, 2, "8"},
		{"back to the original renders", 7, 3, "7"},
	}
	for _, tc := range cases {
		st.Set(tc.set)
		if got := renders.Load(); got != tc.wantRenders {
			t.Errorf("%s: renders = %d, want %d", tc.name, got, tc.wantRenders)
		}
		if got := root.Tree().TextContent(); got != tc.wantText {
			t.Errorf("%s: text = %q, want %q", tc.name, got, tc.wantText)
		}
	}
}

func TestUseSyncExternalStoreSubscribesOnceAndUnsubscribesOnUnmount(t *testing.T) {
	st := NewStore(0)
	counter := &storeTestCounter{}
	subscribe := counter.wrap(st)

	var c Component = func(Props) Node {
		return storeTestHost("b", UseSyncExternalStore(subscribe, st.Get))
	}
	root := NewRoot(&Element{Type: c, Props: Props{}})

	if got := counter.subs.Load(); got != 1 {
		t.Fatalf("subscriptions after mount = %d, want 1", got)
	}

	// Re-renders must not churn the subscription: subscribe did not change.
	st.Set(1)
	root.Render(&Element{Type: c, Props: Props{}})
	if got, want := counter.subs.Load(), int32(1); got != want {
		t.Errorf("subscriptions after updates = %d, want %d", got, want)
	}
	if got := counter.unsubs.Load(); got != 0 {
		t.Errorf("unsubscribes before unmount = %d, want 0", got)
	}

	root.Unmount()
	if got := counter.unsubs.Load(); got != 1 {
		t.Errorf("unsubscribes after unmount = %d, want 1", got)
	}

	// A late write must be inert rather than a panic or a resurrection.
	st.Set(99)
	if got, want := counter.subs.Load(), int32(1); got != want {
		t.Errorf("subscriptions after late write = %d, want %d", got, want)
	}
}

func TestUseSyncExternalStoreResubscribesWhenSubscribeChanges(t *testing.T) {
	st := NewStore(0)
	first, second := &storeTestCounter{}, &storeTestCounter{}

	// Two closures written at two source locations: distinct code pointers, which
	// is the only function identity Go offers. See the caveat on the hook.
	subscribeA := first.wrap(st)
	subscribeB := second.wrap(st)

	var c Component = func(p Props) Node {
		subscribe := subscribeA
		if useB, _ := p.Bool("useB"); useB {
			subscribe = subscribeB
		}
		return storeTestHost("b", UseSyncExternalStore(subscribe, st.Get))
	}

	root := NewRoot(&Element{Type: c, Props: Props{"useB": false}})
	if got, want := first.subs.Load(), int32(1); got != want {
		t.Fatalf("A subscriptions = %d, want %d", got, want)
	}
	if got := second.subs.Load(); got != 0 {
		t.Fatalf("B subscriptions = %d, want 0", got)
	}

	root.Render(&Element{Type: c, Props: Props{"useB": true}})
	if got, want := first.unsubs.Load(), int32(1); got != want {
		t.Errorf("A unsubscribes after swap = %d, want %d", got, want)
	}
	if got, want := second.subs.Load(), int32(1); got != want {
		t.Errorf("B subscriptions after swap = %d, want %d", got, want)
	}

	// The new subscription is the live one: a write still reaches the component.
	st.Set(5)
	if got, want := root.Tree().TextContent(), "5"; got != want {
		t.Errorf("text after swap = %q, want %q", got, want)
	}
	if got, want := first.subs.Load(), int32(1); got != want {
		t.Errorf("A re-subscribed unexpectedly: %d, want %d", got, want)
	}
}

func TestUseSyncExternalStoreDeliversChangeBetweenRenderAndCommit(t *testing.T) {
	st := NewStore(1)
	var mutated, renders atomic.Int32

	var c Component = func(Props) Node {
		renders.Add(1)
		n := UseSyncExternalStore(st.Subscribe, st.Get)

		// Mutate the store after the snapshot was read and before the commit
		// subscribes: the notification reaches nobody, so only the post-commit
		// re-read can rescue this render. A goroutine plus a channel makes the
		// write happen off the render goroutine without a sleep.
		if mutated.CompareAndSwap(0, 1) {
			done := make(chan struct{})
			go func() {
				st.Set(2)
				close(done)
			}()
			<-done
		}
		return storeTestHost("b", n)
	}

	root := NewRoot(&Element{Type: c, Props: Props{}})

	if got, want := root.Tree().TextContent(), "2"; got != want {
		t.Errorf("text = %q, want %q (the change between render and commit was lost)", got, want)
	}
	if got := renders.Load(); got != 2 {
		t.Errorf("renders = %d, want 2 (one stale pass, one corrected pass)", got)
	}
}

func TestUseSyncExternalStoreWithConcurrentWriters(t *testing.T) {
	st := NewStore(0)
	var renders atomic.Int32
	var c Component = func(Props) Node {
		renders.Add(1)
		return storeTestHost("b", UseSyncExternalStore(st.Subscribe, st.Get))
	}
	root := NewRoot(&Element{Type: c, Props: Props{}})

	const writers, each = 8, 25
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				st.Update(func(v int) int { return v + 1 })
			}
		}()
	}
	wg.Wait()
	root.Flush()

	if got, want := st.Get(), writers*each; got != want {
		t.Fatalf("store = %d, want %d", got, want)
	}
	// Every write notifies, and the last notification cannot be the one that
	// leaves the tree behind: the snapshot recorded before scheduling is always
	// re-read during the render that follows.
	if got, want := root.Tree().TextContent(), "200"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if renders.Load() < 2 {
		t.Errorf("renders = %d, want at least 2", renders.Load())
	}
}

func TestUseSyncExternalStoreRejectsNilArguments(t *testing.T) {
	cases := []struct {
		name string
		body func()
	}{
		{"nil subscribe", func() { UseSyncExternalStore(nil, func() int { return 0 }) }},
		{"nil getSnapshot", func() {
			UseSyncExternalStore(func(func()) func() { return func() {} }, (func() int)(nil))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c Component = func(Props) Node {
				tc.body()
				return nil
			}
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected a panic", tc.name)
				}
			}()
			NewRoot(&Element{Type: c, Props: Props{}})
		})
	}
}

func TestStoreSubscribeNotifyAndUnsubscribe(t *testing.T) {
	// The zero Store is usable, which is the property that lets it be embedded
	// in another struct without a constructor call.
	var st Store[string]
	if got := st.Get(); got != "" {
		t.Errorf("zero store = %q, want the zero value", got)
	}

	var a, b int
	unsubA := st.Subscribe(func() { a++ })
	unsubB := st.Subscribe(func() { b++ })

	st.Set("x")
	if a != 1 || b != 1 {
		t.Fatalf("after one write: a=%d b=%d, want 1 and 1", a, b)
	}

	unsubA()
	st.Set("y")
	if a != 1 {
		t.Errorf("unsubscribed listener fired: a=%d, want 1", a)
	}
	if b != 2 {
		t.Errorf("live listener count = %d, want 2", b)
	}

	// Unsubscribing twice is deliberately harmless.
	unsubA()
	unsubB()
	st.Set("z")
	if b != 2 {
		t.Errorf("listener fired after unsubscribe: b=%d, want 2", b)
	}
	if got, want := st.Get(), "z"; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
}

func TestStoreIsSafeForConcurrentUse(t *testing.T) {
	st := NewStore(0)
	var notifications atomic.Int32
	defer st.Subscribe(func() { notifications.Add(1) })()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				st.Update(func(v int) int { return v + 1 })
				// A reader inside the notification path would deadlock if Store
				// held its lock across the callback; reading here covers the
				// plain concurrent-read case.
				_ = st.Get()
			}
		}()
	}
	wg.Wait()

	if got, want := st.Get(), 400; got != want {
		t.Errorf("value = %d, want %d", got, want)
	}
	if got, want := notifications.Load(), int32(400); got != want {
		t.Errorf("notifications = %d, want %d", got, want)
	}
}

func TestStoreObjectsEqualHandlesFuncValues(t *testing.T) {
	fn := func() {}
	other := func() {}
	cases := []struct {
		name string
		a, b any
		want bool
	}{
		// ObjectsEqual itself panics on this pair; storeObjectsEqual exists for
		// exactly this reason.
		{"same func value", fn, fn, true},
		{"distinct funcs", fn, other, false},
		{"nil pair", nil, nil, true},
		{"one nil", nil, fn, false},
		{"plain values still delegate", 3, 3, true},
		{"different types", 3, "3", false},
	}
	for _, tc := range cases {
		if got := storeObjectsEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: storeObjectsEqual = %v, want %v", tc.name, got, tc.want)
		}
	}
}
