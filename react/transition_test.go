package react

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// concurrentHost builds a host element without depending on CreateElement, so
// the concurrency tests stay independent of the element-construction layer. It
// is named for this slice rather than reusing the engine test's `host` so the
// two files cannot collide.
func concurrentHost(tag string, children ...Node) *Element {
	return &Element{Type: tag, Props: Props{ChildrenKey: children}}
}

// concurrentComponent wraps a component function in the element the tests mount.
func concurrentComponent(c Component, props Props) *Element {
	if props == nil {
		props = Props{}
	}
	return &Element{Type: c, Props: props}
}

// concurrentLog records one line per render and lets a test goroutine wait for a
// particular line to appear. Renders in these tests happen on whichever
// goroutine triggered the update, so the log is the only safe way for the test
// to observe them: reading the fiber tree while another goroutine renders would
// be a data race, and polling with a sleep would be both slow and flaky.
type concurrentLog struct {
	mu      sync.Mutex
	entries []string
	ping    chan string
}

func newConcurrentLog() *concurrentLog {
	return &concurrentLog{ping: make(chan string, 256)}
}

// add records a line. The notification send never blocks: a render must not
// depend on the test consuming its output.
func (l *concurrentLog) add(line string) {
	l.mu.Lock()
	l.entries = append(l.entries, line)
	l.mu.Unlock()
	select {
	case l.ping <- line:
	default:
	}
}

func (l *concurrentLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.entries))
	copy(out, l.entries)
	return out
}

func (l *concurrentLog) last() string {
	all := l.all()
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

// waitFor blocks until line has been logged. The timeout is a deadlock guard,
// not a sequencing device — every test drives its asynchrony with channels it
// controls, so a timeout here always means a real failure.
func (l *concurrentLog) waitFor(t *testing.T, line string) {
	t.Helper()
	for _, e := range l.all() {
		if e == line {
			return
		}
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-l.ping:
			if got == line {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for render %q; saw %v", line, l.all())
		}
	}
}

// txStateBox is the payload of the tests' own state hook. The value is written
// from background goroutines and read during render, so it needs a lock of its
// own — the engine test's counterHook is deliberately simpler and single
// threaded.
type txStateBox struct {
	mu sync.Mutex
	v  string
}

// txState is a minimal, goroutine-safe string state hook built directly on the
// nextHook primitive, in the same spirit as engine_test.go's counterHook. It
// keeps these tests independent of whatever shape UseState lands in.
func txState(initial string) (string, func(string)) {
	h, first := nextHook("txstate")
	f := currentFiber()
	if first {
		h.value = &txStateBox{v: initial}
	}
	box := h.value.(*txStateBox)

	set := func(v string) {
		box.mu.Lock()
		box.v = v
		box.mu.Unlock()
		f.scheduleUpdate()
	}

	box.mu.Lock()
	defer box.mu.Unlock()
	return box.v, set
}

func TestUseTransitionPendingIsTrueDuringWorkAndFalseAfter(t *testing.T) {
	log := newConcurrentLog()
	release := make(chan struct{})

	var (
		capturedStart func(func())
		capturedSet   func(string)
	)
	var c Component = func(Props) Node {
		pending, start := UseTransition()
		label, set := txState("idle")
		capturedStart, capturedSet = start, set
		log.add(fmt.Sprintf("%s/%v", label, pending))
		return concurrentHost("b", label)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	if got, want := log.last(), "idle/false"; got != want {
		t.Fatalf("mount render = %q, want %q", got, want)
	}

	start, set := capturedStart, capturedSet
	done := make(chan struct{})
	go func() {
		defer close(done)
		start(func() {
			set("done")
			<-release // the transition is not finished until the test says so
		})
	}()

	// Pending must be observable while the work is still running.
	log.waitFor(t, "idle/true")

	close(release)
	<-done

	// …and false once start has returned, with the update applied.
	log.waitFor(t, "done/false")
	if got, want := log.last(), "done/false"; got != want {
		t.Errorf("final render = %q, want %q", got, want)
	}
	if got, want := dump(root.tree()), `(b "done")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseTransitionStaysPendingUntilAsyncWorkCompletes(t *testing.T) {
	log := newConcurrentLog()
	release := make(chan struct{})

	var (
		capturedStart func(func())
		capturedSet   func(string)
	)
	var c Component = func(Props) Node {
		pending, start := UseTransition()
		label, set := txState("idle")
		capturedStart, capturedSet = start, set
		log.add(fmt.Sprintf("%s/%v", label, pending))
		return concurrentHost("b", label)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	start, set := capturedStart, capturedSet
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The documented shape for asynchronous work: fn spawns goroutines and
		// does not return until they are finished, which is what keeps the
		// pending flag true across them.
		start(func() {
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-release
				set("fetched")
			}()
			wg.Wait()
		})
	}()

	log.waitFor(t, "idle/true")

	close(release)
	<-done

	log.waitFor(t, "fetched/false")
	if got, want := log.last(), "fetched/false"; got != want {
		t.Errorf("final render = %q, want %q", got, want)
	}
}

func TestUseTransitionClearsPendingWhenWorkPanics(t *testing.T) {
	log := newConcurrentLog()

	var capturedStart func(func())
	var c Component = func(Props) Node {
		pending, start := UseTransition()
		capturedStart = start
		log.add(fmt.Sprintf("%v", pending))
		return nil
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	start := capturedStart
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the panic from fn to propagate out of start")
			}
		}()
		start(func() { panic("boom") })
	}()

	if got, want := log.last(), "false"; got != want {
		t.Errorf("pending after a panicking transition = %q, want %q", got, want)
	}
}

func TestStartTransitionCoalescesUpdatesIntoOneRender(t *testing.T) {
	renders := 0
	var capturedSet func(string)
	var c Component = func(Props) Node {
		renders++
		label, set := txState("a")
		capturedSet = set
		return concurrentHost("b", label)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	set := capturedSet
	renders = 0
	StartTransition(func() {
		set("b")
		set("c")
		set("d")
	})

	if renders != 1 {
		t.Errorf("renders = %d, want 1 (three transition updates coalesced)", renders)
	}
	if got, want := dump(root.tree()), `(b "d")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}

	// A nil fn must be a no-op rather than a panic.
	StartTransition(nil)
}

func TestUseDeferredValueLagsExactlyOnePass(t *testing.T) {
	log := newConcurrentLog()
	var c Component = func(p Props) Node {
		v := p.Get("v").(int)
		d := UseDeferredValue(v)
		log.add(fmt.Sprintf("v=%d d=%d", v, d))
		return concurrentHost("b", d)
	}
	mk := func(v int) Node { return concurrentComponent(c, Props{"v": v}) }

	root := NewRoot(mk(1))
	defer root.Unmount()

	cases := []struct {
		name  string
		value int
		want  []string
	}{
		{
			name:  "mount returns the value unchanged",
			value: 1,
			want:  []string{"v=1 d=1"},
		},
		{
			name:  "a change lags one pass then catches up",
			value: 2,
			want:  []string{"v=1 d=1", "v=2 d=1", "v=2 d=2"},
		},
		{
			name:  "an unchanged value costs no extra pass",
			value: 2,
			want:  []string{"v=1 d=1", "v=2 d=1", "v=2 d=2", "v=2 d=2"},
		},
		{
			name:  "the next change lags again",
			value: 3,
			want: []string{
				"v=1 d=1", "v=2 d=1", "v=2 d=2", "v=2 d=2",
				"v=3 d=2", "v=3 d=3",
			},
		},
	}

	for i, tc := range cases {
		// The first case is the mount that already happened above; every later
		// one re-renders with its value.
		if i > 0 {
			root.Render(mk(tc.value))
		}
		got := log.all()
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Fatalf("%s: renders = %v, want %v", tc.name, got, tc.want)
		}
	}

	if got, want := dump(root.tree()), `(b "3")`; got != want {
		t.Errorf("tree = %s, want %s", got, want)
	}
}

func TestUseDeferredValueInitialShowsInitialOnMountThenCatchesUp(t *testing.T) {
	log := newConcurrentLog()
	var c Component = func(p Props) Node {
		v := p.String("v")
		d := UseDeferredValueInitial(v, "placeholder")
		log.add(d)
		return concurrentHost("b", d)
	}

	root := NewRoot(concurrentComponent(c, Props{"v": "real"}))
	defer root.Unmount()

	got := log.all()
	want := []string{"placeholder", "real"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("renders = %v, want %v", got, want)
	}

	// After the catch-up the hook behaves exactly like UseDeferredValue.
	root.Render(concurrentComponent(c, Props{"v": "next"}))
	got = log.all()
	want = []string{"placeholder", "real", "real", "next"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("renders = %v, want %v", got, want)
	}
}

func TestUseDeferredValueSkipsIntermediateValues(t *testing.T) {
	// While the hook is lagging, a value that changes twice is not replayed: the
	// catch-up pass adopts the newest value, matching React's behaviour of
	// abandoning superseded deferred work.
	log := newConcurrentLog()
	var capturedSet func(string)
	var c Component = func(Props) Node {
		v, set := txState("a")
		capturedSet = set
		d := UseDeferredValue(v)
		log.add(v + "/" + d)
		return concurrentHost("b", d)
	}

	root := NewRoot(concurrentComponent(c, nil))
	defer root.Unmount()

	set := capturedSet
	Batch(func() {
		set("b")
		set("c")
	})

	got := log.all()
	want := []string{"a/a", "c/a", "c/c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("renders = %v, want %v (b was superseded before the catch-up)", got, want)
	}
}
