package react

// Ref is a mutable box whose contents survive re-renders — the Go equivalent of
// the object React's useRef returns. React models it as `{ current: T }`; the
// port models it as a one-field struct with an exported field, so reads and
// writes are plain Go field access and the zero value of a *Ref is nil rather
// than a half-built object.
//
// Three properties define a ref, and all three are inherited from React:
//
//   - It survives. The same *Ref comes back from every [UseRef] call on the
//     same component instance, for as long as that instance lives.
//   - Writing to it never triggers a render. Current is an ordinary field; the
//     runtime does not watch it, there is no setter to intercept, and nothing is
//     scheduled. That is the point — a ref is where you put the values a render
//     must not depend on: a timer handle, a "did I already do this" flag, the
//     latest props for a callback that outlives them.
//   - Reading it during render is the classic impurity bug. Because a write is
//     invisible to the runtime, the tree on screen can disagree with what
//     Current holds, and the next render — which may be triggered by anything at
//     all, at any time — silently corrects the discrepancy. A value that the
//     output depends on belongs in state, not in a ref. Reading and writing
//     Current from effects and event handlers is the supported pattern.
//
// A Ref is not safe for concurrent use. Guard it yourself if a goroutine writes
// to it while the tree may be rendering.
//
// Unlike React, Ref is generic, so Current is typed and needs no assertion. Ref
// is also usable outside the hook system: a plain &Ref[T]{} is a perfectly good
// standalone box to hand to [UseImperativeHandle] from a parent that wants to
// own the handle.
type Ref[T any] struct {
	// Current is the boxed value. Read and write it freely; neither is
	// observed by the runtime.
	Current T
}

// UseRef returns a [Ref] that persists for the life of the component instance,
// initialized to initial on the first render and untouched by every render
// after that. It is React's useRef.
//
// initial is evaluated on every render even though it is only used on the
// first, because Go has no lazy arguments — this is the one place the port
// cannot match React's shape exactly. When constructing the initial value is
// expensive, build it inside a [UseMemo] with empty deps and store that, or
// leave the ref zero and fill it in on first use.
//
// The returned pointer is stable, so it is safe to capture in a closure, to
// pass down as a prop, and to use as a dependency of another hook — it never
// compares unequal to itself across renders and therefore never invalidates a
// deps list.
//
// Use [UseState] when a change should be visible on screen; a ref write is
// deliberately invisible.
func UseRef[T any](initial T) *Ref[T] {
	h, first := nextHook("ref")
	if first {
		h.value = &Ref[T]{Current: initial}
	}
	return h.value.(*Ref[T])
}

// UseImperativeHandle publishes a value into ref that a parent — or anyone else
// holding the ref — can call into, recomputing it only when deps changes per
// [DepsChanged]. It is React's useImperativeHandle, and it serves the same
// narrow purpose: a child that must expose a few imperative operations (focus,
// reset, scrollTo) hands out a small handle instead of its whole internals.
//
// The Go shape differs from React's in one way worth noting: React's version
// takes the ref as an opaque ref-or-callback, while here it is a typed
// *Ref[T] and create returns that same T, so the handle a parent reads is
// already the right type. Passing a nil ref is a no-op — the hook slot is still
// claimed, so the call stays unconditional and hook order is preserved.
//
// # Timing
//
// The assignment runs as a layout-phase effect, not during render. That is
// deliberate: render must stay pure, but the handle must exist before any
// passive effect anywhere in the tree runs, so that code reacting to the commit
// finds a usable handle rather than a zero value. Layout effects are exactly the
// phase with that guarantee — [runEffects] drains every layout effect before the
// first passive one.
//
// The deps semantics match [UseMemo]: nil deps reassigns after every render,
// empty non-nil deps assigns exactly once, and a non-empty list reassigns
// whenever an entry changes. Between reassignments ref.Current keeps pointing at
// the previously created handle, so a parent that captured it still holds a live
// value.
//
// When the handle is replaced, and again when the component unmounts, ref is
// reset to the zero value of T before the new handle (if any) is installed —
// React's `ref.current = null` on cleanup. A parent must therefore not hold a
// handle past the child's lifetime, which is the same rule as in React.
//
// # Known limitation
//
// In React, layout effects commit child-first, so a parent's layout effect can
// read a handle its child just published. This port's effect queue fires layout
// effects in enqueue order, which is parent-first, so a parent's layout effect
// runs before its child's handle is installed. A parent must read the handle
// from a passive effect, or from an event handler, until the runtime commits
// layout effects bottom-up. Siblings and ancestors-of-later-subtrees are
// unaffected: anything enqueued after the handle owner rendered does see it.
func UseImperativeHandle[T any](ref *Ref[T], create func() T, deps []any) {
	h, first := nextHook("imperative")

	changed := first || DepsChanged(h.deps, deps)
	h.deps = deps

	if ref == nil || create == nil || !changed {
		return
	}

	enqueueEffect(h, func() func() {
		ref.Current = create()
		return func() {
			var zero T
			ref.Current = zero
		}
	}, true)
}
