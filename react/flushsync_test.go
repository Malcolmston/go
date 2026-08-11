package react

import (
	"strconv"
	"testing"
)

// counterRoot mounts a component whose only job is to publish its setter and
// record how many times it rendered. Every test here is some variation on "when
// does the tree actually show the new value?", so they all need the same two
// observables.
func counterRoot(t *testing.T) (root *Root, set func(int), renders *int) {
	t.Helper()

	var (
		setter SetStateFunc[int]
		count  int
	)
	var counter Component = func(p Props) Node {
		n, s := UseState(0)
		setter = s
		count++
		return H("span", nil, strconv.Itoa(n))
	}

	root = NewRoot(CreateElement(counter, nil))
	t.Cleanup(root.Unmount)
	return root, func(n int) { setter(n) }, &count
}

// text reads the rendered number back out of the tree.
func counterText(r *Root) string { return r.Tree().FindByTag("span").TextContent() }

// TestFlushSyncOutsideBatch pins the honest claim in flushsync.go: with no
// scheduler and no batch in progress, FlushSync(fn) is exactly fn(), because the
// setter already flushed.
func TestFlushSyncOutsideBatch(t *testing.T) {
	root, set, renders := counterRoot(t)
	before := *renders

	FlushSync(func() {
		set(3)
		// Already committed inside fn, since nothing deferred the render.
		if got := counterText(root); got != "3" {
			t.Errorf("inside fn: text = %q, want %q", got, "3")
		}
	})

	if got := counterText(root); got != "3" {
		t.Errorf("after FlushSync: text = %q, want %q", got, "3")
	}
	if got := *renders - before; got != 1 {
		t.Errorf("renders = %d, want 1 — FlushSync should not add a pass", got)
	}
}

// TestFlushSyncInsideBatch is the case FlushSync exists for. Batch defers the
// render; FlushSync has to force it before returning, without ending the batch.
func TestFlushSyncInsideBatch(t *testing.T) {
	root, set, _ := counterRoot(t)

	Batch(func() {
		set(1)
		if got := counterText(root); got != "0" {
			t.Fatalf("Batch did not defer the render: text = %q, want %q", got, "0")
		}

		FlushSync(func() { set(2) })

		if got := counterText(root); got != "2" {
			t.Errorf("after FlushSync inside Batch: text = %q, want %q", got, "2")
		}

		// Batching must still be in effect for what follows.
		set(3)
		if got := counterText(root); got != "2" {
			t.Errorf("FlushSync ended the enclosing Batch: text = %q, want %q", got, "2")
		}
	})

	if got := counterText(root); got != "3" {
		t.Errorf("after Batch: text = %q, want %q", got, "3")
	}
}

// TestFlushSyncCommitsEarlierBatchedWork: work queued before the FlushSync call,
// by an unrelated statement in the same batch, is pending too and must be
// committed by the same flush.
func TestFlushSyncCommitsEarlierBatchedWork(t *testing.T) {
	root, set, _ := counterRoot(t)

	Batch(func() {
		set(7)
		FlushSync(nil) // no fn: flush whatever was already pending
		if got := counterText(root); got != "7" {
			t.Errorf("text = %q, want %q — FlushSync(nil) did not flush pending work", got, "7")
		}
	})
}

// TestFlushSyncSettlesEffectChains: "committed" must mean settled. An effect that
// sets state during the flush schedules another pass, and FlushSync may not
// return until that has run too.
func TestFlushSyncSettlesEffectChains(t *testing.T) {
	var doubled SetStateFunc[int]

	var chained Component = func(p Props) Node {
		n, setN := UseState(0)
		mirror, setMirror := UseState(-1)
		doubled = setN
		UseEffectFn(func() { setMirror(n * 2) }, []any{n})
		return H("span", nil, strconv.Itoa(mirror))
	}

	root := NewRoot(CreateElement(chained, nil))
	defer root.Unmount()

	Batch(func() {
		doubled(4)
		FlushSync(nil)
		if got := counterText(root); got != "8" {
			t.Errorf("text = %q, want %q — the effect's follow-up pass did not run", got, "8")
		}
	})
}

// TestFlushSyncFlushesOnPanic mirrors React: flushSync commits the work it can
// and then rethrows.
func TestFlushSyncFlushesOnPanic(t *testing.T) {
	root, set, _ := counterRoot(t)

	Batch(func() {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Error("the panic from fn did not propagate")
				}
			}()
			FlushSync(func() {
				set(9)
				panic("boom")
			})
		}()

		if got := counterText(root); got != "9" {
			t.Errorf("text = %q, want %q — a panicking fn skipped the flush", got, "9")
		}
	})
}

// TestFlushSyncNesting: an inner FlushSync flushes what is pending at that
// moment and leaves every enclosing Batch intact.
func TestFlushSyncNesting(t *testing.T) {
	root, set, _ := counterRoot(t)

	Batch(func() {
		Batch(func() {
			FlushSync(func() {
				FlushSync(func() { set(2) })
				if got := counterText(root); got != "2" {
					t.Errorf("inner FlushSync: text = %q, want %q", got, "2")
				}
			})
		})
		set(4)
		if got := counterText(root); got != "2" {
			t.Errorf("nesting ended the outer Batch: text = %q, want %q", got, "2")
		}
	})
	if got := counterText(root); got != "4" {
		t.Errorf("after Batch: text = %q, want %q", got, "4")
	}
}

// TestFlushSyncMultipleRoots: FlushSync settles every Root with pending work,
// not just the one the caller happened to touch last.
func TestFlushSyncMultipleRoots(t *testing.T) {
	rootA, setA, _ := counterRoot(t)
	rootB, setB, _ := counterRoot(t)

	Batch(func() {
		setA(1)
		setB(2)
		FlushSync(nil)
		if got := counterText(rootA); got != "1" {
			t.Errorf("root A text = %q, want %q", got, "1")
		}
		if got := counterText(rootB); got != "2" {
			t.Errorf("root B text = %q, want %q", got, "2")
		}
	})
}

// TestFlushSyncValue covers the generic form, which exists because React's
// flushSync returns the callback's value and a func() cannot.
func TestFlushSyncValue(t *testing.T) {
	root, set, _ := counterRoot(t)

	t.Run("returns the value after flushing", func(t *testing.T) {
		Batch(func() {
			got := FlushSyncValue(func() string {
				set(6)
				return "done"
			})
			if got != "done" {
				t.Errorf("FlushSyncValue = %q, want %q", got, "done")
			}
			if got := counterText(root); got != "6" {
				t.Errorf("text = %q, want %q", got, "6")
			}
		})
	})

	t.Run("nil fn yields the zero value", func(t *testing.T) {
		if got := FlushSyncValue[int](nil); got != 0 {
			t.Errorf("FlushSyncValue(nil) = %d, want 0", got)
		}
	})
}

// TestFlushSyncWithNoWork is the degenerate case: nothing pending, nothing to
// do, and no panic for a nil callback.
func TestFlushSyncWithNoWork(t *testing.T) {
	FlushSync(nil)
	FlushSync(func() {})
}
