package react

import "sync"

// FormData is the payload an action receives, the port's stand-in for the
// browser's FormData object. There is no DOM here and no <form> element to
// serialize, so it is a plain map from field name to the values submitted under
// that name.
//
// The multi-value shape is not gratuitous: HTML forms genuinely can submit a
// name more than once (checkbox groups, multi-selects, repeated inputs), which
// is why the browser API has both get and getAll. Modelling that faithfully as
// map[string][]string means an action written against this type behaves the same
// way as one written against a real form post, and it makes FormData trivially
// convertible to and from net/url.Values without this package importing net/url.
//
// A FormData is an ordinary map, so the zero value (nil) is readable but not
// writable: [FormData.Get] and [FormData.Values] are nil-safe, while
// [FormData.Add] and [FormData.Set] panic on a nil map exactly as any map
// assignment would. Construct one with a literal — FormData{"q": {"go"}} — or
// with make.
//
// FormData carries no synchronisation. [UseActionState] snapshots the map it is
// handed before doing anything asynchronous with it, so a caller may reuse or
// mutate their copy after submitting.
type FormData map[string][]string

// Get returns the first value submitted for key, or "" when the key is absent or
// carries no values. It is the equivalent of the DOM's formData.get(), including
// its habit of collapsing the "missing" and "empty" cases — actions almost
// always want a plain string, and distinguishing the two is what
// [FormData.Values] is for.
func (f FormData) Get(key string) string {
	if len(f[key]) == 0 {
		return ""
	}
	return f[key][0]
}

// Values returns every value submitted for key, the equivalent of the DOM's
// formData.getAll(). It returns a copy, so the caller cannot reach back into the
// submitted data by mutating the result, and nil when the key is absent.
func (f FormData) Values(key string) []string {
	vs := f[key]
	if len(vs) == 0 {
		return nil
	}
	out := make([]string, len(vs))
	copy(out, vs)
	return out
}

// Add appends value to the values already recorded for key, the equivalent of
// formData.append(). Use it to build up a repeated field.
func (f FormData) Add(key, value string) {
	f[key] = append(f[key], value)
}

// Set replaces every value recorded for key with value, the equivalent of
// formData.set().
func (f FormData) Set(key, value string) {
	f[key] = []string{value}
}

// cloneFormData copies data so that state captured for the duration of a
// submission cannot be changed underneath the runtime by a caller who kept a
// reference to the map they submitted. It returns nil for nil, so an action that
// takes no data still sees a nil FormData rather than an empty one.
func cloneFormData(data FormData) FormData {
	if data == nil {
		return nil
	}
	out := make(FormData, len(data))
	for k, vs := range data {
		cp := make([]string, len(vs))
		copy(cp, vs)
		out[k] = cp
	}
	return out
}

// optimisticState is the payload of a [UseOptimistic] hook slot. base is the
// committed state as of the last render; value is the optimistic override, valid
// only while overridden is set.
//
// Keeping base is what makes the reset work: the hook cannot ask "did the caller
// commit something?" without remembering what it was handed last time.
type optimisticState[S any] struct {
	mu         sync.Mutex
	base       S
	value      S
	overridden bool
}

// UseOptimistic returns a possibly-optimistic view of state together with a
// function that applies an optimistic action to it, mirroring React 19's
// useOptimistic. Use it to show the result of a mutation before the mutation has
// actually completed — the sent message in the thread, the incremented like
// count — and let the real state take over when it arrives.
//
// The contract, which is the entire hook and the thing to keep in mind:
//
//  1. Calling the returned function applies apply(current, action) and
//     re-renders, so the optimistic value is visible immediately.
//  2. The optimistic value is discarded on the first render whose state argument
//     differs from the one the previous render was given. That is the "snap back
//     to the truth" step; it happens whether or not the truth matches what was
//     optimistically shown, and it happens automatically — there is nothing to
//     call to clear it.
//
// A typical use pairs it with [UseActionState]:
//
//	sent, addOptimistic := react.UseOptimistic(messages, appendMessage)
//	// … addOptimistic(draft) then submit(data); when the action's result lands
//	// as a new `messages`, the optimistic entry disappears in the same render
//	// the real one appears in.
//
// Unlike React, the "did state change?" test is [ObjectsEqual] rather than
// Object.is over a value the framework owns. The practical consequence is that S
// should be comparable in Go's sense: for a slice or map S, ObjectsEqual reports
// equal only for the identical underlying object, so a caller who rebuilds the
// slice on every render will see the optimistic value discarded on the very next
// render. Hold such state behind a pointer, or key the state on a comparable
// version counter.
//
// The returned function is safe to call from any goroutine. Like React's, it is
// an event-time API: calling it from inside a component body is not supported
// (see scheduleConcurrentUpdate for why the port cannot even survive it).
func UseOptimistic[S, A any](state S, apply func(current S, action A) S) (S, func(A)) {
	h, first := nextHook("optimistic")
	f := currentFiber()

	if first {
		h.value = &optimisticState[S]{base: state, value: state}
	}
	st, ok := h.value.(*optimisticState[S])
	if !ok {
		panic("react: UseOptimistic was called with a different state type than on " +
			"the previous render; a hook slot's type must be stable")
	}

	st.mu.Lock()
	if !ObjectsEqual(any(state), any(st.base)) {
		// New committed state: this is the render that discards the optimistic
		// view, per rule 2 above.
		st.base = state
		st.value = state
		st.overridden = false
	}
	shown := st.base
	if st.overridden {
		shown = st.value
	}
	st.mu.Unlock()

	add := func(action A) {
		if apply == nil {
			return
		}
		st.mu.Lock()
		current := st.base
		if st.overridden {
			current = st.value
		}
		// apply runs under the lock so that two concurrent optimistic updates
		// compose (each sees the other's result) instead of racing to overwrite
		// one another. It is expected to be a cheap pure function, as in React.
		st.value = apply(current, action)
		st.overridden = true
		st.mu.Unlock()

		scheduleConcurrentUpdate(f)
	}

	return shown, add
}

// FormStatus is what [UseFormStatus] reports about the action running in the
// nearest enclosing "form" — see that function for what plays the part of a form
// in a runtime with no DOM. The zero value means "no enclosing form, or nothing
// in flight", which is also what React's useFormStatus reports outside a form.
type FormStatus struct {
	// Pending is true while a submission is running or queued.
	Pending bool

	// Data is a copy of the FormData of the oldest submission that has not
	// finished — with one submission at a time, the one that is running — and
	// nil when nothing is in flight. It is a copy, so reading it cannot race
	// with the goroutine that submitted it.
	Data FormData
}

// formStatusSource is implemented by an action-state slot so that
// [UseFormStatus] can read it without knowing its state type. The hook state is
// generic; the status it reports is not, and this interface is the seam between
// the two.
type formStatusSource interface{ formStatus() FormStatus }

// formScopes maps the fiber of a component that called [UseActionState] to that
// hook's state, so a descendant can find it. This is the port's replacement for
// React's internal form context: [UseFormStatus] walks up the fiber tree looking
// for the nearest registered ancestor.
//
// A registration is removed when the owning fiber unmounts, which the hook
// arranges by putting a remover in its slot's cleanup — the same slot teardown
// that runs effect cleanups, and the only unmount notification a hook gets.
var (
	formScopesMu sync.RWMutex
	formScopes   = map[*fiber]formStatusSource{}
)

// actionState is the payload of a [UseActionState] hook slot.
//
// Two mutexes, deliberately. mu guards the fields and is never held across a
// call into user code. queue serializes the action runs themselves, so that a
// second submission always sees the first one's result as its prev — React
// queues actions on a form for the same reason, and without it "thread the
// previous state through" would be a lie under concurrency.
type actionState[S any] struct {
	mu    sync.Mutex
	state S

	// queued holds the FormData of every submission that has been accepted and
	// not yet finished, oldest first. Its length is the pending count, and its
	// head is the submission [FormStatus] reports — with one submission at a
	// time, the common case, that is exactly the running one.
	queued []FormData

	queue sync.Mutex
}

// begin records an accepted submission. It is called before the queue lock is
// taken, so a submission that is still waiting its turn already reads as
// pending.
func (a *actionState[S]) begin(data FormData) {
	a.mu.Lock()
	a.queued = append(a.queued, data)
	a.mu.Unlock()
}

// end retires the oldest outstanding submission.
func (a *actionState[S]) end() {
	a.mu.Lock()
	if len(a.queued) > 0 {
		a.queued = a.queued[1:]
	}
	a.mu.Unlock()
}

// formStatus implements [formStatusSource].
func (a *actionState[S]) formStatus() FormStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.queued) == 0 {
		return FormStatus{}
	}
	return FormStatus{Pending: true, Data: cloneFormData(a.queued[0])}
}

// UseActionState runs a form action and keeps its result as state, mirroring
// React 19's useActionState. It returns the current state, a submit function,
// and whether a submission is in flight.
//
// The action is called as action(prev, data) where prev is the state left by the
// previous submission — starting from initial — so an action is a reducer whose
// "dispatch" carries form data:
//
//	state, submit, pending := react.UseActionState(func(prev Result, d react.FormData) Result {
//	    return login(prev, d.Get("user"), d.Get("password"))
//	}, Result{})
//
// Differences from React worth knowing:
//
//   - submit runs the action synchronously on the calling goroutine and returns
//     when it has finished. React returns immediately and resolves a promise
//     later; Go's equivalent of "later" is a goroutine, and making the caller
//     write `go submit(data)` is both clearer and more flexible than baking one
//     in. isPending is true for the whole run either way, because the pending
//     flag is set (and a render scheduled) before the action is entered.
//   - Submissions are serialized: a submit that arrives while another is running
//     counts as pending immediately, then waits its turn and receives the
//     earlier submission's result as prev. This matches React's action queue and
//     makes concurrent submits well-defined rather than last-writer-wins.
//   - data is snapshotted before the action runs, so the caller may reuse the
//     map afterwards.
//
// A nil action, or a call on an unmounted tree, is a no-op. The state and the
// pending flag are read during render under a lock, so submitting from a
// goroutine is safe; submitting from a component body is not supported, exactly
// as in React.
//
// See [UseOptimistic] for showing a result before the action finishes, and
// [UseFormStatus] for reading this hook's status from a descendant.
func UseActionState[S any](action func(prev S, data FormData) S, initial S) (state S, submit func(FormData), isPending bool) {
	h, first := nextHook("actionState")
	f := currentFiber()

	if first {
		st := &actionState[S]{state: initial}
		h.value = st

		// Publish this component as the "enclosing form" for its subtree, and
		// arrange for the entry to go away when the fiber unmounts. Slot
		// cleanups are run by unmount for every hook, not just effects, which is
		// what makes this safe to hang here.
		formScopesMu.Lock()
		formScopes[f] = st
		formScopesMu.Unlock()
		h.cleanup = func() {
			formScopesMu.Lock()
			delete(formScopes, f)
			formScopesMu.Unlock()
		}
	}
	st, ok := h.value.(*actionState[S])
	if !ok {
		panic("react: UseActionState was called with a different state type than on " +
			"the previous render; a hook slot's type must be stable")
	}

	submit = func(data FormData) {
		if action == nil {
			return
		}
		snapshot := cloneFormData(data)

		// Count the submission as pending before queueing, so a caller that is
		// still waiting for its turn already shows as busy.
		st.begin(snapshot)
		scheduleConcurrentUpdate(f)

		// Retiring the submission is deferred so that a panicking action leaves
		// the component usable rather than permanently pending.
		defer func() {
			st.end()
			scheduleConcurrentUpdate(f)
		}()

		st.queue.Lock()
		defer st.queue.Unlock()

		st.mu.Lock()
		prev := st.state
		st.mu.Unlock()

		// User code runs with no lock held: an action is expected to block on
		// I/O, and holding mu across it would stall every render of the tree.
		result := action(prev, snapshot)

		st.mu.Lock()
		st.state = result
		st.mu.Unlock()
	}

	st.mu.Lock()
	state = st.state
	isPending = len(st.queued) > 0
	st.mu.Unlock()

	return state, submit, isPending
}

// UseFormStatus reports the status of the action running in the nearest
// enclosing form, mirroring React 19's useFormStatus — a hook for the submit
// button, which wants to disable itself while the form it lives in is busy
// without the form having to thread a prop down to it.
//
// React resolves "the enclosing form" through a context that the <form> element
// publishes. There is no DOM here and no form element, so the port substitutes
// the closest thing that carries the same meaning: **the nearest ancestor
// component that called [UseActionState] is the form.** Its submission state is
// what this hook reports. That is a real relationship in the fiber tree, not an
// approximation of one, and it gives the hook the property that makes it worth
// having — a deeply nested button can observe the submission without any
// intermediate component knowing about it.
//
// Two consequences of that substitution:
//
//   - Only ancestors count. Calling UseFormStatus in the very component that
//     called UseActionState returns the zero [FormStatus], because that
//     component *is* the form rather than being inside it. React behaves the
//     same way for the component that renders the <form>; read the third return
//     value of UseActionState there instead.
//   - With no such ancestor, the zero FormStatus is returned. A button used
//     outside a form is simply never pending.
//
// The status is a snapshot taken during render, including a copy of the
// submitted [FormData], so it never races with the goroutine running the action.
func UseFormStatus() FormStatus {
	// Claim a slot even though nothing is stored in it: the hook must consume a
	// slot index like every other hook, or adding a UseFormStatus call would
	// shift the numbering of the hooks around it in a way nextHook could not
	// diagnose.
	nextHook("formStatus")
	f := currentFiber()

	formScopesMu.RLock()
	defer formScopesMu.RUnlock()

	owner := findAncestor(f, func(a *fiber) bool {
		_, ok := formScopes[a]
		return ok
	})
	if owner == nil {
		return FormStatus{}
	}
	return formScopes[owner].formStatus()
}
