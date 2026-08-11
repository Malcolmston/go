package react

// This file implements React's three effect hooks: [UseEffect] (passive),
// [UseLayoutEffect] and [UseInsertionEffect], plus the cleanup-free
// conveniences [UseEffectFn] and [UseLayoutEffectFn].
//
// An effect is the port's answer to "do something outside the render", the same
// role it plays in React. A component body must be a pure function of props and
// hook state, because the runtime calls it more than once per visible update
// (see [Component]); anything that touches the world — a subscription, a timer,
// a log line, a channel send — belongs in an effect, which the runtime runs
// once per commit rather than once per render call.
//
// The Go shape differs from JavaScript's in one visible way: React's effect
// callback returns either undefined or a cleanup function, a union Go cannot
// express. The port makes the return explicit — `func() (cleanup func())` —
// and treats a nil return as "no cleanup". Because the overwhelmingly common
// case is an effect with no teardown, and writing `func() func() { …; return
// nil }` for it is pure ceremony, each of the two hooks a caller reaches for
// daily also has an Fn variant taking a plain `func()`.

// The effect hook kinds recorded on the hook slot. nextHook checks the kind on
// every render, so these strings are what turns "a UseEffect moved behind an
// if" into a diagnostic naming both hooks instead of silent state corruption.
//
// The layout and insertion hooks consume two slots per call site: one holding
// the deps and the live cleanup, one holding the pump entry described on
// [drainQueuedEffects]. Both are taken unconditionally on every render, so the
// slot cursor stays aligned regardless of whether the effect fires.
const (
	passiveEffectHookKind       = "effect"
	layoutEffectHookKind        = "layout-effect"
	layoutEffectPumpHookKind    = "layout-effect-pump"
	insertionEffectHookKind     = "insertion-effect"
	insertionEffectPumpHookKind = "insertion-effect-pump"
)

// queuedEffect is one layout- or insertion-phase effect waiting for the commit,
// held in this file's own queues rather than the root's so that the two phases
// can be ordered against each other (see [drainQueuedEffects]).
//
// owner is kept only so a queued effect belonging to a fiber that was torn down
// between render and commit — an unmounted root, a subtree dropped by a later
// render pass — is skipped instead of resurrecting a dead component.
type queuedEffect struct {
	owner  *fiber
	h      *hook
	effect func() func()
}

// pendingInsertionEffects and pendingLayoutEffects hold the current commit's
// queued effects in tree order.
//
// They are package-level for the same reason the hook dispatcher is: renders
// and their commits are serialized process-wide by renderMu, so at most one
// commit is ever draining them and a per-root queue would buy nothing. Both are
// appended to during render (under renderMu, from [useQueuedEffect]) and
// drained during the commit that follows it (also under renderMu).
var (
	pendingInsertionEffects []queuedEffect
	pendingLayoutEffects    []queuedEffect
)

// UseEffect registers a passive effect: a side effect that runs after the
// commit, once the whole tree for this update is rendered. It is React's
// useEffect.
//
// effect returns its cleanup function, which the runtime runs before the next
// firing of the same effect and once more when the component unmounts.
// Returning nil means "nothing to tear down" — that is the Go spelling of
// React's callback that returns undefined. Use [UseEffectFn] when the effect
// has no cleanup at all.
//
// deps decides when the effect fires again, with React's three-way rule that
// hinges on nil versus empty:
//
//   - deps == nil — no dependency list at all: the effect fires after every
//     commit. This is JavaScript's `useEffect(fn)` with the second argument
//     omitted. In Go the absence of an argument has to be spelled as a nil
//     slice, which is why the nil/empty distinction carries so much weight
//     here and why a caller who means "on mount only" must be careful to write
//     []any{} rather than nil.
//   - deps == []any{} — an empty but non-nil list: the effect fires once, on
//     mount, and its cleanup runs once, on unmount. JavaScript's `[]`.
//   - a non-empty list — the effect fires on mount and again whenever any
//     entry differs from the previous firing, compared with [DepsChanged] and
//     hence [ObjectsEqual], the port's Object.is.
//
// The deps recorded on the hook slot are the ones from the last firing, not
// from the last render, exactly as in React: an effect that is skipped because
// its deps are unchanged leaves the recorded list alone, so a value that
// changes and changes back between two firings correctly does not re-fire it.
//
// The effect body never runs during render. A component that calls a state
// setter from an effect causes a second render pass, which the flush loop
// settles before [NewRoot] or [Root.Render] returns.
func UseEffect(effect func() (cleanup func()), deps []any) {
	if effect == nil {
		panic("react: UseEffect needs a non-nil effect function")
	}

	h, first := nextHook(passiveEffectHookKind)
	if !first && !DepsChanged(h.deps, deps) {
		// Unchanged deps: leave the slot — and therefore the live cleanup —
		// untouched. Enqueueing here would make an ordinary re-render tear the
		// effect down and set it up again, which is the bug this whole hook
		// exists to avoid.
		return
	}

	h.deps = deps
	enqueueEffect(h, effect, false)
}

// UseEffectFn is [UseEffect] for the very common effect that has nothing to
// tear down. It exists purely so callers are not forced to write
// `func() func() { …; return nil }` around a one-line body; the deps rules,
// including the nil-versus-empty distinction, are identical.
//
// React needs no such variant because a JavaScript arrow function that returns
// nothing already satisfies the signature.
func UseEffectFn(effect func(), deps []any) {
	if effect == nil {
		panic("react: UseEffectFn needs a non-nil effect function")
	}
	UseEffect(func() func() {
		effect()
		return nil
	}, deps)
}

// UseLayoutEffect registers a layout effect: like [UseEffect] in every way
// except that it runs earlier in the commit — all layout effects in the tree,
// in tree order, before any passive effect of the same commit.
//
// In React that ordering exists so a layout effect can measure the DOM after
// mutation but before the browser paints. There is no DOM and no paint here, so
// what the port actually guarantees is the ordering itself: a layout effect
// observes a tree that no passive effect has had a chance to disturb, and a
// state update it makes is still folded into the same flush. Prefer [UseEffect]
// unless that ordering is the point — a layout effect blocks every passive
// effect in the commit behind it.
//
// deps follow exactly the rules documented on [UseEffect]. Use
// [UseLayoutEffectFn] for the cleanup-free case.
func UseLayoutEffect(effect func() (cleanup func()), deps []any) {
	if effect == nil {
		panic("react: UseLayoutEffect needs a non-nil effect function")
	}
	useQueuedEffect(effect, deps, layoutEffectHookKind, layoutEffectPumpHookKind, false)
}

// UseLayoutEffectFn is [UseLayoutEffect] for an effect with no cleanup, the
// same convenience [UseEffectFn] is for [UseEffect].
func UseLayoutEffectFn(effect func(), deps []any) {
	if effect == nil {
		panic("react: UseLayoutEffectFn needs a non-nil effect function")
	}
	UseLayoutEffect(func() func() {
		effect()
		return nil
	}, deps)
}

// UseInsertionEffect registers an insertion effect, the earliest of the three
// phases: every insertion effect of a commit runs — cleanup and body both —
// before any layout effect of that commit, which in turn runs before any
// passive effect.
//
// React added this hook for CSS-in-JS libraries, which must inject a style rule
// before layout effects measure anything that rule would affect. The port has
// no stylesheet, so the hook is offered for API parity and for the ordering
// guarantee itself; application code should reach for [UseEffect] or
// [UseLayoutEffect] instead. As in React, an insertion effect must not read
// state that layout has not computed yet, and calling a state setter from one
// is a mistake rather than merely inefficient.
//
// About the guarantee, precisely — the port's commit machinery has only two
// phases (runEffects fires everything registered as layout, then everything
// registered as passive), so there is no third queue to put these in. Rather
// than claim an ordering it does not have, this file implements the insertion
// phase itself: [UseInsertionEffect] and [UseLayoutEffect] do not hand their
// bodies to the runtime at all. They append them to the package-level queues
// above and register a *pump* in the layout phase; whichever pump the runtime
// reaches first drains the insertion queue in full and only then the layout
// queue (see [drainQueuedEffects]). Both queues are in tree order, so the
// resulting sequence is exactly all-insertion-then-all-layout across the whole
// tree, not merely within one component.
//
// The one thing that is genuinely not guaranteed: an effect registered straight
// with the runtime's own layout phase, bypassing [UseLayoutEffect], may run
// before the pump and therefore before the insertion effects. Nothing in this
// package does that outside of the engine's own tests.
//
// deps follow the rules documented on [UseEffect]. There is deliberately no Fn
// variant: an insertion effect that injects something almost always has
// something to remove again, and the missing convenience is a nudge to write
// the cleanup.
func UseInsertionEffect(effect func() (cleanup func()), deps []any) {
	if effect == nil {
		panic("react: UseInsertionEffect needs a non-nil effect function")
	}
	useQueuedEffect(effect, deps, insertionEffectHookKind, insertionEffectPumpHookKind, true)
}

// useQueuedEffect is the shared body of [UseLayoutEffect] and
// [UseInsertionEffect]. It performs the same deps check as [UseEffect] and then,
// instead of handing the body to the runtime, parks it in one of this file's
// queues and registers a pump in the runtime's layout phase.
//
// The pump gets a hook slot of its own — hence two slots per call site — so
// that its (always nil) cleanup can never collide with the real effect's
// cleanup, which lives on the first slot where unmount will find it. Both slots
// are claimed on every render, firing or not, because nextHook identifies a
// slot by call index and nothing else.
func useQueuedEffect(effect func() func(), deps []any, kind, pumpKind string, insertion bool) {
	h, first := nextHook(kind)
	pump, _ := nextHook(pumpKind)

	if !first && !DepsChanged(h.deps, deps) {
		return
	}

	h.deps = deps
	q := queuedEffect{owner: currentFiber(), h: h, effect: effect}
	if insertion {
		pendingInsertionEffects = append(pendingInsertionEffects, q)
	} else {
		pendingLayoutEffects = append(pendingLayoutEffects, q)
	}

	enqueueEffect(pump, runEffectPump, true)
}

// runEffectPump is the create function every queued effect registers with the
// runtime's layout phase. The first pump the commit reaches drains both queues;
// every later pump in the same commit finds them empty and does nothing, which
// is why they are all the same function value and hold no state.
//
// It returns nil: a pump has no cleanup of its own, and the real cleanups are
// written straight onto the effects' own hook slots by the drain.
func runEffectPump() func() {
	drainQueuedEffects(&pendingInsertionEffects)
	drainQueuedEffects(&pendingLayoutEffects)
	return nil
}

// drainQueuedEffects fires and empties one queue, running each entry's previous
// cleanup immediately before its new body — the same "tear down the old
// subscription, set up the new one" shape the runtime's own runEffects has, and
// the reason a cleanup is stored on the hook slot rather than captured here:
// unmount reads it from there to tear the effect down leaf-first.
//
// The queue is swapped out before it is walked, and the walk repeats, so an
// effect body that itself registers another queued effect is fired in this same
// commit instead of leaking into the next one.
func drainQueuedEffects(queue *[]queuedEffect) {
	for len(*queue) > 0 {
		batch := *queue
		*queue = nil
		for _, e := range batch {
			// A fiber torn down between its render and this commit — an
			// unmounted root, a subtree dropped by a later pass of the same
			// flush — must not have its effect run: its cleanup has already
			// been called and there is nothing left to tear down again.
			if e.owner != nil && e.owner.unmounted {
				continue
			}
			if e.h.cleanup != nil {
				cleanup := e.h.cleanup
				e.h.cleanup = nil
				cleanup()
			}
			e.h.cleanup = e.effect()
		}
	}
}
