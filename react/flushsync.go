package react

// FlushSync, and what it can honestly mean in a port with no scheduler.
//
// Upstream, react-dom's flushSync exists because React 19 does not render when
// you tell it to. An update inside an event handler, a transition or an effect is
// queued into a lane, and the scheduler decides later — possibly after yielding
// to the browser — when to render it. flushSync is the escape hatch that says
// "render the queued work now, before this call returns", for the cases where the
// next line of imperative code has to observe the committed result.
//
// This port has no scheduler and no lanes. [Root.flush] is a plain loop that
// renders, commits, runs effects, and repeats until nothing is dirty, and a state
// setter calls it directly. The pending-work window that flushSync exists to
// close therefore does not exist here — except in exactly one place, which is
// where this function earns its keep: inside [Batch]. Batch is this port's stand-
// in for React's automatic batching around events, and while it is active an
// update only marks its Root dirty; the render is deferred to the outermost
// Batch's return.
//
// So the honest relationship, stated plainly:
//
//	FlushSync(fn) == fn() followed by flushing every Root with deferred work.
//
//   - Outside a [Batch] it is precisely fn(). Every update fn made already
//     rendered synchronously before the setter returned, and there is nothing
//     left to flush. The call is not a no-op in the sense of being useless — it
//     is a no-op because the guarantee it makes is already unconditional here —
//     and writing it costs nothing and keeps code that also runs the guarantee
//     explicit.
//   - Inside a [Batch] it forces the deferred renders to happen now, including
//     any that earlier statements in the same Batch had queued. That is the same
//     thing flushSync does when called inside React's automatic batch.
//   - It is not [Root.Flush]. Root.Flush settles one Root, and it does nothing at
//     all while a Batch is active, because [Root.flush] returns early under
//     batching. FlushSync settles every Root with pending work and does so by
//     suspending batching for the duration.
//
// What FlushSync deliberately does not do is end the enclosing Batch. Batching is
// restored the moment the flush finishes, so updates made after the FlushSync
// call are still coalesced into the Batch's single final render — which is what a
// caller reaching for flushSync in the middle of a batch wants.

// FlushSync runs fn and then renders every [Root] with work still pending,
// so that the tree is fully committed by the time it returns. It is react-dom's
// flushSync.
//
//	react.Batch(func() {
//	    setTitle("loading")
//	    react.FlushSync(func() { setSpinner(true) })
//	    // the spinner is on screen here, not after Batch returns
//	    setTitle(load())
//	})
//
// Outside a [Batch] this is exactly fn(): updates already flush as they are made
// (see the note at the top of this file), so there is never anything left over. A
// nil fn is allowed and flushes whatever was already pending.
//
// Effects scheduled by the flush run before FlushSync returns, and updates those
// effects raise are rendered too — the flush loop keeps going until the tree
// settles, so "committed" here means fully settled, not one pass.
//
// A panic from fn still flushes before it propagates, matching React, where
// flushSync commits the work it can and then rethrows. Nesting is safe: an inner
// FlushSync flushes what is pending at that moment and leaves the enclosing
// [Batch] in effect.
//
// The batching state it suspends is process-wide, exactly as [Batch]'s is, so a
// FlushSync on one goroutine briefly unbatches a concurrent Batch on another.
// That is inherited from Batch rather than introduced here; keep a Batch on one
// goroutine, as its own documentation asks.
func FlushSync(fn func()) {
	// Deferred so a panicking fn still commits the work it managed to schedule.
	defer flushPendingRoots()

	if fn != nil {
		fn()
	}
}

// FlushSyncValue is [FlushSync] for a function that produces a value, since
// React's flushSync returns whatever the callback returned and a Go func()
// cannot. The flush happens before the value is handed back, so the caller reads
// a settled tree and a result together:
//
//	id := react.FlushSyncValue(func() string { return addRow(row) })
//
// A nil fn returns the zero value of T after flushing.
func FlushSyncValue[T any](fn func() T) (result T) {
	defer flushPendingRoots()

	if fn == nil {
		return result
	}
	return fn()
}

// flushPendingRoots renders every Root that [Batch] has parked, with batching
// suspended so the flush actually happens.
//
// The suspension is the whole mechanism and is worth spelling out: [Root.flush]
// begins by asking batching(), and returns after re-parking the Root when the
// answer is yes. Calling [Root.Flush] from inside a Batch is therefore a no-op,
// and there is no other public way in. Zeroing batchDepth for the duration is
// what turns those calls into real renders, and restoring it afterwards is what
// leaves the enclosing Batch intact for the updates that follow.
//
// Taking batchRoots at the same time is not merely convenient — it is required.
// Once depth is zero, an update raised by an effect during the flush takes the
// non-batching path and renders inline, so the list cannot grow behind us and a
// second pass would find nothing.
func flushPendingRoots() {
	batchMu.Lock()
	roots := batchRoots
	batchRoots = nil
	depth := batchDepth
	batchDepth = 0
	batchMu.Unlock()

	// Restored under a defer so that a panic from a component, an effect or an
	// effect cleanup cannot leave the process permanently unbatched.
	defer func() {
		batchMu.Lock()
		batchDepth = depth
		batchMu.Unlock()
	}()

	for _, r := range roots {
		r.Flush()
	}
}
