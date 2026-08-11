package react

// suspenseTag is the element type of a [Suspense] boundary. It is a distinct
// empty struct rather than a string or a sentinel value so that
// [RegisterElementRenderer] can key on its Go type, and so that reconciliation's
// sameType comparison (struct equality) keeps a boundary's fiber — and therefore
// its pending/resolved state — alive across renders.
type suspenseTag struct{}

const (
	// suspenseFallbackKey is the props key holding the boundary's fallback node.
	// React passes the fallback as a JSX prop; Go has no JSX, so [Suspense]
	// takes it as the first positional argument and stows it here.
	suspenseFallbackKey = "fallback"

	// suspenseRetryNotifyKey is a test seam, not part of the public API. When an
	// element carries a chan struct{} under this key, every retry goroutine this
	// boundary starts sends on it once it has finished re-rendering the tree.
	// Tests use it to await an asynchronous retry deterministically instead of
	// sleeping; nothing in the public API ever sets it, and a boundary without
	// it behaves identically.
	suspenseRetryNotifyKey = "__react_suspense_retry"
)

func init() {
	RegisterElementRenderer(suspenseTag{}, renderSuspense)
}

// Suspense returns a boundary that renders children, and renders fallback
// instead while anything beneath it is waiting on asynchronous work. It is the
// Go equivalent of `<Suspense fallback={…}>…</Suspense>`.
//
// The Go shape differs from React's in two ways. JSX passes the fallback as a
// named prop; here it is the first positional argument, because Go has no
// keyword arguments and a boundary without a fallback is almost always a
// mistake. And the mechanism is panic/recover rather than a thrown promise:
// a component that cannot finish calls [Suspend], which panics a
// [SuspendSignal]; because [renderFiber] descends recursively, the nearest
// enclosing Suspense boundary is genuinely on the call stack and can recover it.
// That is the reason this port renders recursively at all.
//
// What a boundary does when it catches a suspension:
//
//   - It discards the partially built subtree, unmounting every fiber that was
//     created before the panic so their effect cleanups run and their
//     not-yet-fired mount effects are cancelled rather than leaking.
//   - It renders fallback in their place.
//   - It starts one goroutine per distinct signal that waits on the signal's
//     Ready channel and then schedules a re-render of the [Root], which retries
//     the children.
//
// A boundary is re-armable: once the children render successfully the boundary
// forgets the suspension, so a later one shows the fallback again. Boundaries
// nest the way they do in React — the innermost one on the stack recovers the
// signal, since it is the first deferred recover to run.
//
// A Suspense boundary is not an error boundary. Any panic that is not a
// [SuspendSignal] is re-panicked untouched so it reaches an [ErrorBoundary] or
// the caller.
//
// Elements returned by Suspense carry no key. Set the returned element's Key
// field directly when a boundary sits in a keyed list.
func Suspense(fallback Node, children ...Node) *Element {
	return &Element{
		Type: suspenseTag{},
		Props: Props{
			suspenseFallbackKey: fallback,
			ChildrenKey:         children,
		},
	}
}

// suspenseState is a boundary's memory, kept in the fiber's state slot (the
// scratch field the runtime reserves for element-type handlers). React keeps the
// equivalent in the fiber's mode flags and a set of "thenables"; the port needs
// only enough to answer three questions: are we waiting, on what, and is the
// fallback currently mounted?
type suspenseState struct {
	// pending is true between catching a suspension and the retry that renders
	// the children successfully.
	pending bool

	// ready is the channel of the signal currently awaited. It doubles as the
	// dedupe key for retry goroutines: re-rendering a pending boundary catches
	// the same signal again, and spawning a goroutine per attempt would pile up
	// waiters for one resolution.
	ready <-chan struct{}

	// showFallback records whether the boundary's children are currently the
	// fallback rather than the real subtree. The two are separate trees in
	// React (the real one is kept alive offscreen); this port has no offscreen
	// mode, so switching between them must explicitly tear down the outgoing
	// children rather than letting reconciliation match a fallback fiber up
	// with a same-typed content fiber and hand it the wrong hook state.
	showFallback bool
}

// renderSuspense is the boundary's element renderer. It has exactly three
// paths — still waiting, children rendered, children suspended — and every one
// of them ends in a single call to [reconcileChildren], which is the contract
// [RegisterElementRenderer] documents.
func renderSuspense(f *fiber) {
	st, _ := f.state.(*suspenseState)
	if st == nil {
		st = &suspenseState{}
		f.state = st
	}

	// Fast path: a re-render for some unrelated reason while the awaited work is
	// still outstanding. Re-attempting the children would only suspend again,
	// and would remount the fallback and re-run its effects for nothing, so keep
	// showing what is already there.
	if st.pending && !suspenseReady(st.ready) {
		reconcileChildren(f, []Node{f.props.Get(suspenseFallbackKey)})
		return
	}

	if suspenseAttempt(f, st) {
		st.pending, st.ready, st.showFallback = false, nil, false
		return
	}

	st.showFallback = true
	reconcileChildren(f, []Node{f.props.Get(suspenseFallbackKey)})
}

// suspenseAttempt renders the boundary's real children and reports whether they
// finished. It exists as its own function purely so that recover() has a frame
// to run in: recover only works in a function deferred directly by the frame
// that is panicking through, so the descent must be wrapped rather than inlined
// into [renderSuspense].
//
// A recovered [SuspendSignal] is absorbed; anything else is re-panicked with the
// original value, so an [ErrorBoundary] above sees exactly what was thrown.
func suspenseAttempt(f *fiber, st *suspenseState) (done bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		sig, ok := r.(SuspendSignal)
		if !ok {
			// Not ours. A Suspense boundary is not an error boundary.
			panic(r)
		}

		// Whatever was built before the panic is a half-rendered tree that will
		// never be committed. Drop it so cleanups run and its pending mount
		// effects never fire.
		boundaryDropChildren(f)
		st.pending = true

		// One waiter per distinct signal: a pending boundary that is re-rendered
		// catches the same signal again, and a goroutine per attempt would be an
		// unbounded pile of waiters on one channel.
		if st.ready != sig.Ready {
			st.ready = sig.Ready
			suspenseRetryWhenReady(f, sig.Ready)
		}
		done = false
	}()

	// Coming back from the fallback: tear it down first, so reconciliation
	// cannot match a fallback fiber with a content fiber of the same type and
	// hand the content the fallback's hooks.
	if st.showFallback {
		boundaryDropChildren(f)
		st.showFallback = false
	}

	reconcileChildren(f, f.props.Children())
	return true
}

// suspenseReady reports whether a signal's channel has already been closed,
// without blocking. A nil channel means "nothing awaited", which counts as
// ready.
func suspenseReady(ready <-chan struct{}) bool {
	if ready == nil {
		return true
	}
	select {
	case <-ready:
		return true
	default:
		return false
	}
}

// suspenseRetryWhenReady starts the goroutine that turns a resolved signal back
// into a render. This is the one place in the boundary that touches
// concurrency, and the locking deserves an explanation.
//
// Rendering is serialized behind renderMu, and this function is called from
// inside a render — renderMu is held by the goroutine on whose stack we sit. The
// retry therefore cannot render inline; it has to happen later, on another
// goroutine, after the current render has released the mutex. The goroutine does
// not take renderMu itself: it calls the fiber's scheduleUpdate, which is the
// foundation's own scheduling entry point and already handles the two orderings
// that matter. If the awaited work resolves while a render is still in flight,
// scheduleUpdate merely marks the root dirty and returns, and the flush loop
// already on the stack picks the work up on its next pass — no second lock
// acquisition, no deadlock. If it resolves when nothing is rendering,
// scheduleUpdate flushes, blocking on renderMu only as long as some other
// render is in progress; that render never waits on this goroutine, so the wait
// is bounded. An unmounted root makes scheduleUpdate a no-op, which is the
// "update on an unmounted component" case handled by ignoring it.
//
// The goroutine parks on ready forever if the awaited work never completes.
// That is the resource layer's business, not the boundary's: cancellation
// belongs to whoever created the signal.
func suspenseRetryWhenReady(f *fiber, ready <-chan struct{}) {
	// Read the test seam here, on the render goroutine, rather than inside the
	// goroutine: props are only safe to read while the render that installed
	// them is on the stack.
	notify, _ := f.props.Get(suspenseRetryNotifyKey).(chan struct{})

	go func() {
		<-ready
		f.scheduleUpdate()
		if notify != nil {
			// Sent only after scheduleUpdate has returned, so a test that
			// receives here is guaranteed to be looking at a settled tree.
			notify <- struct{}{}
		}
	}()
}

// boundaryDropChildren tears down every child of a boundary fiber and cancels
// the mount effects those children registered during the render that is being
// abandoned. Both [Suspense] and [ErrorBoundary] need it, for the same reason:
// the subtree they are discarding was built by a render pass that will never be
// committed, and half of it may already have queued effects on the root.
//
// The cancellation is the subtle half. unmount runs effect cleanups, but a fiber
// that has never committed has no cleanup yet — its effect is still sitting in
// the root's pendingEffects queue, and if it were left there the commit would
// run create() for a component that no longer exists in the tree and store the
// resulting cleanup on a hook nothing will ever unmount again. So the hooks of
// the doomed subtree are collected first (unmount clears the child links, which
// would make them unreachable) and any queued effect belonging to one of them is
// dropped from the queue.
func boundaryDropChildren(f *fiber) {
	var doomed map[*hook]bool
	for c := f.child; c != nil; c = c.sibling {
		walk(c, func(x *fiber) {
			for _, h := range x.hooks {
				if doomed == nil {
					doomed = map[*hook]bool{}
				}
				doomed[h] = true
			}
		})
	}

	if r := f.root; r != nil && doomed != nil {
		r.mu.Lock()
		kept := r.pendingEffects[:0]
		for _, e := range r.pendingEffects {
			if !doomed[e.h] {
				kept = append(kept, e)
			}
		}
		r.pendingEffects = kept
		r.mu.Unlock()
	}

	for c := f.child; c != nil; c = c.sibling {
		unmount(c)
	}
	f.child = nil
}

// Error makes a [SuspendSignal] describe itself when it escapes to the top of a
// [Root] with no [Suspense] boundary to catch it. Go's panic printer calls
// Error on a panicked value that implements error, so a runaway suspension
// fails with a sentence naming the problem instead of dumping a struct literal.
//
// The method lives in the Suspense file rather than beside the type because the
// diagnostic is a statement about boundaries: the runtime's render loop has no
// hook a boundary-layer package could attach to, so the message has to travel
// on the value itself. It also means [SuspendSignal] satisfies error, which is
// harmless — no code path treats a suspension as a failure.
func (s SuspendSignal) Error() string {
	return "react: a component suspended outside of any Suspense boundary; " +
		"wrap it in react.Suspense(fallback, …) so there is something to render while it loads"
}
