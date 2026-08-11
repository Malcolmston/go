package react

import (
	"fmt"
	"reflect"
	"sync"
)

// This file implements the state hooks: [UseState], [UseStateLazy],
// [UseReducer] and [UseReducerInit].
//
// Layering follows React's own source rather than the documentation's framing.
// React presents useState as the simple hook and useReducer as the advanced
// one, but internally there is a single update queue and useState is nothing
// more than useReducer with a fixed "basic state reducer" that either replaces
// the state with a value or applies an updater function to it. This port does
// the same: [stateHookQueue] is the one piece of machinery, [UseReducer] hands
// it a boxed user reducer, and [UseState] hands it [stateHookBasicReducer].
// Sharing the queue is what makes the two hooks agree on the properties that
// matter — stable dispatch identity, in-order draining, and the same-value
// bailout — instead of having two nearly-identical implementations drift.
//
// The Go shape differs from JS in two visible ways:
//
//   - Generics replace inference. React's setState is untyped; here the state
//     type is a parameter, so the queue stores values boxed in `any` and every
//     read goes back through [stateHookAs]. Boxing (rather than making the
//     queue itself generic) is deliberate: a hook slot is a single `any` field
//     shared by every hook, and a generic queue type would have to be
//     re-instantiated per T at every internal call site.
//
//   - React's setter is overloaded — `set(5)` and `set(n => n + 1)` are the
//     same function. Go cannot overload, and `SetStateFunc[func(T) T]` would be
//     genuinely ambiguous when T is itself a function type, so the functional
//     form is a method: [SetStateFunc.Update].

// stateHookAction is the queue entry a [SetStateFunc] pushes. Exactly one of
// its fields is meaningful: update is nil for the direct form `set(v)`, in
// which case value is the replacement; a non-nil update is the functional form
// and is applied to whatever state the drain has reached by the time it runs.
//
// It is a struct rather than two separate queue kinds so that the queue stays a
// plain []any that [UseReducer] can share without inspecting entries.
type stateHookAction struct {
	value  any
	update func(prev any) any
}

// stateHookBasicReducer is React's basicStateReducer: the fixed reducer that
// turns [UseState] into a special case of [UseReducer]. An action that is not
// one of ours is returned unchanged, which keeps the function total rather than
// panicking on a queue that could only be corrupt.
func stateHookBasicReducer(state, action any) any {
	a, ok := action.(stateHookAction)
	if !ok {
		return action
	}
	if a.update != nil {
		return a.update(state)
	}
	return a.value
}

// stateHookQueue is the runtime half of every state hook — the thing a hook
// slot's value field actually points at. It holds the committed state, the
// updates queued since the last render, the reducer that applies them, and the
// dispatch function handed back to the component.
//
// The queue, not the component, is what a setter closes over. That is the whole
// trick behind two of React's guarantees: the setter never captures a state
// value, so a setter saved during an early render still updates from the latest
// state; and the setter is created once and reused, so its identity is stable
// and safe to list in an effect's dependencies.
type stateHookQueue struct {
	// mu guards state, pending and reducer. Renders are serialized by renderMu,
	// but setters are explicitly allowed to be called from any goroutine — a
	// timer, a background fetch — so the queue itself must be safe even though
	// the render that drains it is not concurrent with another render.
	mu sync.Mutex

	// fiber is the component this queue belongs to. It is the scheduling
	// handle: a setter has no other way to say "this subtree changed".
	fiber *fiber

	// reducer is refreshed on every render, matching React's
	// lastRenderedReducer. A reducer written as a closure over props is a
	// different function value each render, and updates dispatched after this
	// render should use that newest closure, not the one captured at mount.
	reducer func(state, action any) any

	// state is the committed state, boxed. It is only ever advanced by drain.
	state any

	// pending are the queued actions in dispatch order. Draining in order is
	// what makes two Update calls inside one [Batch] compose rather than the
	// second one clobbering the first.
	pending []any

	// setter is the dispatch value handed to the component: a SetStateFunc[T]
	// for [UseState], a func(A) for [UseReducer]. It is created on the first
	// render and never replaced, which is the stable-identity guarantee. Only
	// render touches it, and renders are serialized, so it needs no lock.
	setter any
}

// drain applies every queued update to the committed state, in order. It runs
// at the top of each render of the owning component, before the component body
// observes its state, so a render always sees the result of every setter call
// that happened before it.
//
// The reducers run with the lock released. A reducer or an updater is required
// to be pure, but "pure" does not mean "does not touch another hook", and
// holding the queue lock across user code would turn a merely questionable
// program into a deadlocked one.
func (q *stateHookQueue) drain() {
	q.mu.Lock()
	pending, reducer, state := q.pending, q.reducer, q.state
	q.pending = nil
	q.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	for _, action := range pending {
		state = reducer(state, action)
	}

	q.mu.Lock()
	q.state = state
	q.mu.Unlock()
}

// dispatch queues one update and asks the runtime to re-render.
//
// The interesting part is the bailout. React eagerly computes the next state
// when the queue is empty, and when it turns out to equal the current state it
// returns without scheduling anything at all — "setting state to the same value
// bails out of re-rendering". Doing it here, at dispatch time, rather than
// after the drain is what makes the bailout observable: once a render pass has
// started there is no way to un-render it, so a drain-time check could only
// avoid a *second* pass, not the first. When the queue is non-empty the eager
// check is skipped, because the update ahead of this one has not been applied
// yet and comparing against a stale state would be wrong.
//
// A dispatch on an unmounted fiber is a silent no-op, which is the "can't
// update an unmounted component" case; scheduleUpdate independently ignores
// updates to an unmounted [Root], so a stale closure is safe from both ends.
func (q *stateHookQueue) dispatch(action any) {
	q.mu.Lock()

	// The fiber is read under the queue's lock, not before it. A setter may be
	// called from any goroutine, while the render goroutine rewrites q.fiber on
	// every render — reading it unlocked is a data race the detector catches.
	//
	// There is deliberately no f.unmounted check here either: that field belongs
	// to the renderer and reading it from a background goroutine races the
	// unmount that writes it. scheduleUpdate already ignores updates to an
	// unmounted Root, and a render pass skips marking an unmounted fiber, so a
	// stale closure is still safe — it just costs one wasted pass in the rare
	// case where the fiber died but the Root did not.
	f := q.fiber
	if f == nil {
		q.mu.Unlock()
		return
	}

	if len(q.pending) == 0 {
		if next := q.reducer(q.state, action); stateHookEqual(next, q.state) {
			q.mu.Unlock()
			return
		}
	}
	q.pending = append(q.pending, action)
	q.mu.Unlock()

	f.scheduleUpdate()
}

// stateHookEqual is the comparison behind the bailout: [ObjectsEqual] with one
// deliberate exception for state of a function type, which a component may
// legitimately hold (a callback kept in state).
//
// ObjectsEqual compares funcs by code pointer, so two closures built from the
// same literal on different renders compare equal however different their
// captured variables are — which would bail out of a state change that really
// happened. Reporting funcs as never equal is the conservative answer: it costs
// a re-render that may not have been needed, and never skips one that was.
func stateHookEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if reflect.TypeOf(a).Kind() == reflect.Func || reflect.TypeOf(b).Kind() == reflect.Func {
		return false
	}
	return ObjectsEqual(a, b)
}

// stateHookAs unboxes a queue value. A nil box yields the zero value rather
// than panicking, so that state of a pointer, slice, map or interface type
// round-trips through the queue even when it is nil — `x.(T)` on a nil
// interface always fails, including when T is `any`.
func stateHookAs[T any](v any) T {
	var zero T
	if v == nil {
		return zero
	}
	t, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("react: state hook holds a value of type %T, want %T; "+
			"a hook slot was reused by a different hook", v, zero))
	}
	return t
}

// stateHookSlot claims this call site's hook slot and returns its queue,
// creating it on the first render and draining it on every later one.
//
// initial is a thunk rather than a value so that a lazy initializer runs
// exactly once, on mount; the caller decides whether the thunk closes over an
// already-computed value ([UseState]) or over the user's init function
// ([UseStateLazy], [UseReducerInit]).
func stateHookSlot(kind string, reducer func(state, action any) any, initial func() any) *stateHookQueue {
	h, first := nextHook(kind)
	f := currentFiber()

	if first {
		q := &stateHookQueue{fiber: f, reducer: reducer, state: initial()}
		h.value = q
		return q
	}

	q, ok := h.value.(*stateHookQueue)
	if !ok {
		panic(fmt.Sprintf("react: hook slot %q holds %T, not a state queue", kind, h.value))
	}

	q.mu.Lock()
	q.fiber = f
	q.reducer = reducer
	q.mu.Unlock()

	q.drain()
	return q
}

// SetStateFunc is the setter [UseState] returns. It is a function type so the
// common case reads exactly like React's — call it with a value to replace the
// state — and it carries one method, [SetStateFunc.Update], for the functional
// form. React overloads a single setter to accept either a value or an updater;
// Go has no overloading, and a `set(fn)` overload would be undecidable when the
// state type is itself a function, so the two forms are split by name.
//
// A setter's identity is stable for the life of the component: the same value
// is returned by every render, so it is safe to put in a dependency list and it
// never causes an effect to re-fire. It also never captures the state value it
// was created alongside — a setter saved during the first render still updates
// from the latest state.
//
// Calling a setter after its [Root] has been unmounted is a silent no-op.
//
// The variadic parameter is plumbing for [SetStateFunc.Update], not part of the
// calling convention: call a setter with exactly one argument. Passing an
// updater positionally works, but Update reads better and is the documented
// form.
type SetStateFunc[T any] func(value T, updater ...func(prev T) T)

// Update sets state from the previous state, the equivalent of React's
// `set(prev => next)`. Use it whenever the next state depends on the current
// one: two direct calls to the setter in a single [Batch] collapse to the last
// value written, while two Update calls compose, because updates are queued and
// applied in order at the start of the next render.
//
// A nil setter or a nil fn is ignored, so an unconfigured optional callback
// does not have to be guarded at every call site.
//
// Implementation note: a method on a bare function type has nothing but the
// function value to work with and cannot reach the hook's queue, so the updater
// travels through the call itself as a variadic argument. An earlier design
// parked it in a package-level slot guarded by a mutex; that left one
// interleaving uncovered — a direct setter call on another goroutine landing
// inside another goroutine's Update — and passing the updater as an argument
// removes the shared state rather than narrowing the window.
func (s SetStateFunc[T]) Update(fn func(prev T) T) {
	if s == nil || fn == nil {
		return
	}
	// The value argument is ignored by a setter that receives an updater; the
	// zero is a placeholder for a parameter Go requires.
	var zero T
	s(zero, fn)
}

// UseState declares a piece of component state and returns its current value
// together with a setter, the direct equivalent of React's useState.
//
// initial is used only on the component's first render. Note that unlike React,
// where a lazy initializer is the only way to avoid the cost, Go evaluates the
// argument expression on every render even though the value is then discarded —
// use [UseStateLazy] when producing the initial value is expensive.
//
// The returned setter is stable across renders and does not capture the state
// value; see [SetStateFunc]. Setting the state to a value equal to the current
// one (by [ObjectsEqual]) does not re-render.
//
// A setter called from outside a render — a goroutine, a timer, a test — renders
// synchronously before it returns, unless it is inside [Batch]. A setter called
// during a render is a render-phase update: it does not recurse, it schedules
// another pass of the flush loop.
func UseState[T any](initial T) (T, SetStateFunc[T]) {
	return useStateHook[T](func() any { return any(initial) })
}

// UseStateLazy is [UseState] with a lazily computed initial value: init is
// called exactly once, on mount, and never again. It is React's
// `useState(() => expensive())` form, split into its own function because Go
// cannot distinguish "a function used as the initial value" from "a function
// that produces the initial value" by overloading — and state that holds a
// function is a real use case.
//
// init must not be nil.
func UseStateLazy[T any](init func() T) (T, SetStateFunc[T]) {
	if init == nil {
		panic("react: UseStateLazy needs a non-nil init function")
	}
	return useStateHook[T](func() any { return any(init()) })
}

// useStateHook is the shared body of [UseState] and [UseStateLazy]: claim the
// slot with the basic-state reducer, create the setter once, hand both back.
func useStateHook[T any](initial func() any) (T, SetStateFunc[T]) {
	q := stateHookSlot("state", stateHookBasicReducer, initial)

	if q.setter == nil {
		// Created once and stored on the queue — this is the stable identity.
		// The closure captures the queue, never a state value, which is what
		// keeps a setter held by a stale closure correct.
		q.setter = SetStateFunc[T](func(value T, updater ...func(prev T) T) {
			if len(updater) > 0 && updater[0] != nil {
				fn := updater[0]
				q.dispatch(stateHookAction{
					update: func(prev any) any { return any(fn(stateHookAs[T](prev))) },
				})
				return
			}
			q.dispatch(stateHookAction{value: any(value)})
		})
	}

	set, ok := q.setter.(SetStateFunc[T])
	if !ok {
		panic(fmt.Sprintf("react: UseState state type changed between renders: "+
			"slot holds %T, want %T; a hook's type parameter must be the same "+
			"on every render", q.setter, set))
	}
	return stateHookAs[T](q.state), set
}

// UseReducer declares component state managed by a reducer, returning the
// current state and a dispatch function. It is the hook to reach for when the
// next state is a function of several distinct events rather than of one value:
// the transition logic lives in one pure function instead of being spread
// across every call site that would otherwise call a setter.
//
// reducer is re-read on every render, so a reducer written as a closure over
// props sees the current props for updates dispatched after that render. It
// must be pure: the runtime may call it more than once for the same action
// while checking whether an update can bail out.
//
// The dispatch function has a stable identity and, like a [SetStateFunc],
// dispatching an action that leaves the state equal to the current state does
// not re-render.
func UseReducer[S, A any](reducer func(state S, action A) S, initial S) (S, func(A)) {
	return UseReducerInit(reducer, initial, stateHookIdentity[S])
}

// UseReducerInit is [UseReducer] with React's third `init` argument: the
// initial state is init(initialArg), computed exactly once on mount. Splitting
// initialization out of the mount path this way is how React lets the same
// function serve as both "build the initial state" and "reset to the initial
// state" in response to an action.
//
// reducer and init must not be nil.
func UseReducerInit[S, A, I any](reducer func(state S, action A) S, initialArg I, init func(I) S) (S, func(A)) {
	if reducer == nil {
		panic("react: UseReducer needs a non-nil reducer")
	}
	if init == nil {
		panic("react: UseReducerInit needs a non-nil init function")
	}

	// Box the typed reducer for the untyped queue. A fresh closure per render is
	// exactly what we want: it captures this render's reducer value.
	boxed := func(state, action any) any {
		return any(reducer(stateHookAs[S](state), stateHookAs[A](action)))
	}
	q := stateHookSlot("reducer", boxed, func() any { return any(init(initialArg)) })

	if q.setter == nil {
		q.setter = func(action A) { q.dispatch(any(action)) }
	}

	dispatch, ok := q.setter.(func(A))
	if !ok {
		panic(fmt.Sprintf("react: UseReducer action type changed between renders: "+
			"slot holds %T, want %T; a hook's type parameters must be the same "+
			"on every render", q.setter, dispatch))
	}
	return stateHookAs[S](q.state), dispatch
}

// stateHookIdentity is the init function [UseReducer] passes to
// [UseReducerInit] when the caller supplied the initial state directly.
func stateHookIdentity[S any](s S) S { return s }
