package react

import "sync"

// transitionState is the payload stored in a [UseTransition] hook slot: a count
// of the transitions started from this hook that have not finished yet.
//
// It is a counter rather than a bool because start may be called re-entrantly or
// from several goroutines at once, and "pending" must stay true until the last
// of them is done. The mutex is not optional: the count is written by whichever
// goroutine ran start and read during render, which may be a different one.
type transitionState struct {
	mu      sync.Mutex
	pending int
}

func (t *transitionState) begin() {
	t.mu.Lock()
	t.pending++
	t.mu.Unlock()
}

func (t *transitionState) end() {
	t.mu.Lock()
	if t.pending > 0 {
		t.pending--
	}
	t.mu.Unlock()
}

func (t *transitionState) isPending() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending > 0
}

// scheduleConcurrentUpdate marks f dirty and renders, for the hooks in this file
// and in action.go whose setters are explicitly designed to be called from a
// background goroutine.
//
// It is now a thin alias for fiber.scheduleUpdate, which handles the concurrent
// case itself: it parks the fiber on the Root under a lock and lets the next
// render pass do the ancestor marking on the rendering goroutine, so no caller
// touches the tree's pointers from a background goroutine. It also detects a
// flush already in progress instead of re-entering one.
//
// The wrapper survives because the hooks in this file and in action.go document
// their setters as safe to call from a background goroutine, and a named
// function is where that guarantee is worth stating. Unlike the version it
// replaces, it is safe to call from anywhere, including a component body.
func scheduleConcurrentUpdate(f *fiber) {
	f.scheduleUpdate()
}

// UseTransition returns a pending flag and a function that runs an update as a
// transition, mirroring React's useTransition.
//
// The Go shape differs from React's in one way that matters, and it is worth
// stating plainly rather than hiding behind the familiar name: **this port does
// not implement priority lanes or interruption.** In React a transition update
// is scheduled at a lower priority, may be interrupted by urgent input, and may
// be thrown away and restarted; that behaviour depends on a time-slicing
// scheduler which this runtime does not have — [Root.flush] renders
// synchronously to completion. What survives the port is the other half of
// useTransition, the half most code actually consumes: an honest pending flag
// that is true for exactly as long as the transition's work is running.
//
// So a transition here is an *asynchronous boundary*, not a priority:
//
//	isPending, start := react.UseTransition()
//	// …
//	start(func() {
//	    setResults(search(query)) // slow; isPending is true throughout
//	})
//
// start marks the hook pending, re-renders so the flag is visible, runs fn (with
// the state updates inside it batched, as React batches a transition), and then
// clears the flag and re-renders again. It returns only after fn has returned.
//
// Asynchronous work is supported, and is the interesting case, but the port
// cannot guess when a goroutine you spawned has finished — so the contract is
// that **fn must not return until the transition's work is done**. Callers own
// that synchronisation, which in Go is a one-liner:
//
//	start(func() {
//	    var wg sync.WaitGroup
//	    wg.Add(1)
//	    go func() {
//	        defer wg.Done()
//	        setResults(fetch(query)) // safe: setters are goroutine-safe
//	    }()
//	    wg.Wait() // isPending stays true until here
//	})
//
// Because start blocks, an event handler that must not block should itself run
// in a goroutine (`go start(fn)`); the pending flag is still observed by the
// renders start triggers. A nil fn is a no-op. start is an event-time API, as it
// is in React: it must be called from an event handler, an effect or a
// goroutine, never from a component body.
//
// See [StartTransition] for the standalone form, and [UseDeferredValue] for the
// other half of React's concurrent surface.
func UseTransition() (isPending bool, start func(fn func())) {
	h, first := nextHook("transition")
	f := currentFiber()

	if first {
		h.value = &transitionState{}
	}
	st, ok := h.value.(*transitionState)
	if !ok {
		panic("react: UseTransition hook slot holds an unexpected value; " +
			"this slot was claimed by another hook")
	}

	start = func(fn func()) {
		if fn == nil {
			return
		}
		st.begin()
		scheduleConcurrentUpdate(f)

		// The clear runs even if fn panics: a transition that failed must not
		// leave the component pending forever.
		defer func() {
			st.end()
			scheduleConcurrentUpdate(f)
		}()

		StartTransition(fn)
	}

	return st.isPending(), start
}

// StartTransition runs fn as a transition without a hook, the equivalent of
// React's standalone startTransition. Use it when the update is triggered from
// somewhere that has no component to hold a pending flag — a background
// goroutine, a timer, a package-level event bus.
//
// As with [UseTransition], the port has no scheduler, so there is no priority
// and no interruption: what StartTransition actually provides is React's other
// documented guarantee for a transition, namely that every state update fn makes
// is coalesced into a single render rather than one render apiece. It is
// therefore [Batch] with a name that says why you are batching — and, when
// paired with UseTransition, the thing that keeps the pending flag from
// flickering once per setter call.
//
// fn runs synchronously on the calling goroutine; StartTransition returns when
// it returns. Updates made from goroutines fn spawns are not part of the batch
// unless fn waits for them.
func StartTransition(fn func()) {
	if fn == nil {
		return
	}
	Batch(fn)
}

// deferredState is the payload of a [UseDeferredValue] hook slot. shown is the
// value the component is currently being handed; lagging records that the slot
// has already noticed a new value and re-rendered for it, so the *next* render
// should catch up.
//
// No mutex is needed here, unlike [transitionState]: the slot is only ever
// touched from a component body, and renders are serialized process-wide by
// renderMu.
type deferredState[T any] struct {
	shown   T
	lagging bool
}

// UseDeferredValue returns a copy of value that lags one render pass behind it:
// on the render where value changes it still returns the previous value, and it
// catches up on the render immediately after. It is the Go equivalent of React's
// useDeferredValue.
//
// The deviation from React is the same one described on [UseTransition], and
// again it is worth being explicit: React's useDeferredValue is defined in terms
// of the scheduler — the deferred re-render happens at transition priority and
// can be interrupted or skipped entirely if the value changes again first. This
// port has no scheduler, so the hook is implemented as a plain one-pass lag:
// notice the change, re-render once, then adopt the new value. That reproduces
// the observable behaviour the hook exists for — an expensive subtree keeps
// rendering the stale value for one pass while the cheap part of the UI updates
// immediately with the fresh one — without pretending to a priority system that
// is not there. The lag is exactly one pass, always; it is not "however long the
// browser is busy".
//
// Typical use is a fast input paired with a slow list:
//
//	query := p.String("query")          // updates immediately
//	deferred := react.UseDeferredValue(query) // lags one pass
//	return list(deferred)               // expensive; re-renders one pass later
//
// On the first render the value is returned as-is, matching React; use
// [UseDeferredValueInitial] when the first render should show something else.
// Values are compared with [ObjectsEqual], so a T that is uncomparable in Go
// (a slice, a map, a func) is treated as changed on every render unless it is
// literally the same underlying object — prefer a comparable T, or a pointer
// that is stable across renders.
func UseDeferredValue[T any](value T) T {
	return useDeferredImpl(value, value, false)
}

// UseDeferredValueInitial is [UseDeferredValue] with React 19's optional second
// argument: initial is what the hook returns on the very first render, and the
// hook then catches up to value on the next pass, exactly as it does for any
// later change.
//
// It exists as a separate function rather than a variadic or an option struct
// because Go has no optional arguments and `UseDeferredValue(v)` is by far the
// common case; splitting the two keeps the common call site free of a sentinel.
//
// Use it to render an empty or placeholder state on mount while the real value
// is prepared:
//
//	shown := react.UseDeferredValueInitial(results, nil) // mount shows nil
func UseDeferredValueInitial[T any](value T, initial T) T {
	return useDeferredImpl(value, initial, true)
}

// useDeferredImpl is the body shared by both deferred-value hooks. Both claim
// the same slot kind, which is correct: a single call site always calls one or
// the other, and the two are the same hook with a different mount behaviour.
func useDeferredImpl[T any](value T, initial T, hasInitial bool) T {
	h, first := nextHook("deferred")
	f := currentFiber()

	if first {
		st := &deferredState[T]{shown: initial}
		h.value = st
		if hasInitial {
			// Mounting with a distinct initial value is itself a lag: schedule
			// the catch-up pass now so the real value appears without waiting
			// for an unrelated update to come along.
			st.lagging = true
			f.scheduleUpdate()
		}
		return st.shown
	}

	st, ok := h.value.(*deferredState[T])
	if !ok {
		panic("react: UseDeferredValue was called with a different type parameter " +
			"than on the previous render; a hook slot's type must be stable")
	}

	switch {
	case st.lagging:
		// This is the catch-up pass. Adopt whatever the newest value is — if it
		// moved twice while we lagged, the intermediate one is simply skipped,
		// which is what React does too.
		st.shown = value
		st.lagging = false

	case !ObjectsEqual(any(value), any(st.shown)):
		// The value just changed. Keep showing the old one for this pass and ask
		// for one more render, on which the case above will adopt it.
		st.lagging = true
		f.scheduleUpdate()
	}

	return st.shown
}
