package react

import (
	"fmt"
	"reflect"
	"sync"
)

// Hook slot kinds owned by this file. [UseSyncExternalStore] takes two slots
// because a slot holds exactly one effect cleanup, and this hook needs two
// independent effects: one that owns the subscription (torn down and rebuilt
// only when `subscribe` changes) and one that re-reads the snapshot after every
// commit.
const (
	syncExternalStoreHook      = "sync-external-store"
	syncExternalStoreCheckHook = "sync-external-store-check"
)

// UseSyncExternalStore subscribes a component to a mutable value that lives
// outside React — a global config, a cache, a channel-fed connection state, a
// Go value shared with a background goroutine — and is the only sanctioned way
// to read such a value from a component body.
//
// It is React 19's useSyncExternalStore, with the same two-function shape:
//
//   - subscribe is called with a callback and must arrange for that callback to
//     run whenever the store changes. It returns the function that cancels the
//     subscription.
//   - getSnapshot returns the current value. It must be cheap, must have no side
//     effects, and — critically — must return a value that compares equal per
//     [ObjectsEqual] when the store has not changed. A getSnapshot that builds a
//     fresh slice or map on every call reports "changed" forever and the render
//     loop panics with React's "too many re-renders" diagnostic. Return a
//     comparable value (a scalar, a struct of scalars, a pointer to an immutable
//     value) or cache the derived one in the store.
//
// The returned value is the snapshot read during this render, so a component
// body never observes a value cached from an earlier pass.
//
// # Why an external store hook exists at all
//
// A component that reads a mutable variable directly is not a pure function of
// its inputs: nothing tells the runtime to re-render when the variable changes,
// and two components reading it at different moments in the same render pass can
// disagree — the inconsistency React calls *tearing*. This hook is the sanctioned
// path because it gives the runtime the two things it needs to reason about the
// value: a notification channel and a snapshot function it may re-run.
//
// # What this implementation actually guarantees (and what it does not)
//
// Renders in this port are synchronous and serialized process-wide by renderMu,
// so there is no time-slicing and no interrupted-render class of tearing to
// begin with. Within that model the hook guarantees:
//
//   - Freshness. The value returned is read by getSnapshot during this render,
//     never a cached one.
//   - No lost update between render and commit. The subscription does not exist
//     while the component is rendering, so a change in that window notifies
//     nobody. The hook therefore re-reads the snapshot in a *layout*-phase effect
//     after every commit and, if it differs from what the component rendered
//     with, schedules another pass. Layout phase and not passive because layout
//     effects run first and synchronously within the same commit, before any
//     passive effect has had a chance to read the tree or fire a side effect
//     based on a value that is already stale.
//   - Settled output. Because those extra passes happen inside the flush loop,
//     [Root.Flush], [Root.Render] and [Act] do not return until the tree agrees
//     with a snapshot taken at or after the last commit.
//
// It does *not* guarantee that two components reading the same store in one
// render pass see the same value: each calls getSnapshot at its own point in the
// traversal, and a goroutine may write in between. What the port promises is
// that such a pass cannot be the last one — the post-commit check notices the
// change and re-renders until the tree is consistent. Callers who need a truly
// atomic multi-component read should hold one snapshot in a parent and pass it
// down as props.
//
// # Re-subscription and Go's missing function identity
//
// React re-subscribes when the `subscribe` reference changes. Go cannot compare
// closures: two closures created at the same source location have the same code
// pointer whatever they captured, and a fresh closure built on every render has
// that same pointer too. The hook therefore compares subscribe by code pointer,
// which gets the common cases right — an inline closure does not churn the
// subscription on every render, and a switch between two functions written at
// different source locations does re-subscribe. The case it cannot see is a
// *method value on a different receiver*, such as swapping storeA.Subscribe for
// storeB.Subscribe: those share a code pointer, so the subscription keeps
// pointing at the old store even though getSnapshot already reads the new one.
// To swap stores, remount the component by giving it a different key.
//
// The subscription is established on mount, rebuilt when subscribe changes, and
// cancelled when the component unmounts.
//
// React's third parameter, getServerSnapshot, is deliberately absent. It exists
// so that a server render and the client hydration that follows agree on a
// value; this port renders to a string and stops (see [RenderToString]) — there
// is no client, no hydration and therefore no second reader to agree with. A
// server render simply calls getSnapshot, which is the correct answer when
// nothing will hydrate the output. If hydration ever lands, the variant belongs
// here.
func UseSyncExternalStore[T any](subscribe func(onStoreChange func()) (unsubscribe func()), getSnapshot func() T) T {
	if subscribe == nil {
		panic("react: UseSyncExternalStore needs a non-nil subscribe function")
	}
	if getSnapshot == nil {
		panic("react: UseSyncExternalStore needs a non-nil getSnapshot function")
	}

	h, first := nextHook(syncExternalStoreHook)
	f := currentFiber()

	// Read before anything else: this is the value the component renders with,
	// and the value the post-commit check compares against.
	snapshot := getSnapshot()

	var st *syncExternalStoreState[T]
	if first {
		st = &syncExternalStoreState[T]{}
		h.value = st
	} else {
		var ok bool
		if st, ok = h.value.(*syncExternalStoreState[T]); !ok {
			panic(fmt.Sprintf("react: UseSyncExternalStore snapshot type changed between "+
				"renders: this hook slot holds %T; a hook must keep the same type parameter "+
				"on every render", h.value))
		}
	}

	key := syncStoreFuncKey(subscribe)

	st.mu.Lock()
	st.fiber = f
	st.getSnapshot = getSnapshot
	st.snapshot = snapshot
	resubscribe := first || key != st.subscribeKey
	st.subscribeKey = key
	st.mu.Unlock()

	if resubscribe {
		// Owns the subscription: the effect's cleanup is the unsubscribe, so the
		// runtime's ordinary "run the old cleanup before the new body, and again
		// at unmount" machinery is the entire subscription lifecycle.
		enqueueEffect(h, func() func() { return st.connect(subscribe) }, true)
	}

	// Enqueued on *every* render, not only when subscribe changed: the
	// render-to-commit window exists on updates just as much as on mount.
	check, _ := nextHook(syncExternalStoreCheckHook)
	enqueueEffect(check, func() func() {
		st.refresh(syncStoreAnyEpoch)
		return nil
	}, true)

	return snapshot
}

// syncExternalStoreState is the per-fiber state behind [UseSyncExternalStore].
// It is reachable from two places that run on different goroutines — the render
// path and a store's notification callback — so every field is guarded by mu.
type syncExternalStoreState[T any] struct {
	mu sync.Mutex

	// snapshot is the value the component last rendered with, and the baseline
	// every change notification is compared against.
	snapshot    T
	getSnapshot func() T

	// subscribeKey is the code pointer of the subscribe function from the last
	// render; see the identity caveat on [UseSyncExternalStore].
	subscribeKey uintptr

	// epoch identifies the live subscription. Every connect and every teardown
	// bumps it, so a notification that arrives from a cancelled subscription —
	// a store that fires one last callback on its way out, or a callback already
	// in flight on another goroutine — is recognized as stale and dropped
	// instead of scheduling work for a fiber that has moved on.
	epoch uint64

	fiber *fiber
}

// syncStoreAnyEpoch tells [syncExternalStoreState.refresh] to skip the epoch
// check. Real epochs start at 1 because connect pre-increments, so zero is free
// to mean "not from a subscription callback".
const syncStoreAnyEpoch uint64 = 0

// connect establishes the subscription and returns its teardown. It runs in the
// layout phase of a commit.
func (s *syncExternalStoreState[T]) connect(subscribe func(func()) func()) func() {
	s.mu.Lock()
	s.epoch++
	epoch := s.epoch
	s.mu.Unlock()

	// A store is allowed to invoke the callback synchronously from inside
	// subscribe. That needs no special handling: fiber.scheduleUpdate detects a
	// flush already in progress and records the update for its next pass rather
	// than re-entering the render loop.
	unsubscribe := subscribe(func() { s.refresh(epoch) })

	return func() {
		s.mu.Lock()
		s.epoch++
		s.mu.Unlock()
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

// refresh re-reads the store and schedules a render when the value moved. epoch
// is the subscription the caller belongs to, or [syncStoreAnyEpoch] for the
// post-commit check; a notification from a superseded subscription is dropped.
//
// The snapshot is updated here as well as during render, so a stream of
// notifications that do not change the value costs one comparison each and no
// renders at all — React's bailout, and the reason a store may notify liberally.
func (s *syncExternalStoreState[T]) refresh(epoch uint64) {
	s.mu.Lock()
	if s.getSnapshot == nil || (epoch != syncStoreAnyEpoch && epoch != s.epoch) {
		s.mu.Unlock()
		return
	}
	next := s.getSnapshot()
	if storeObjectsEqual(s.snapshot, next) {
		s.mu.Unlock()
		return
	}

	f := s.fiber
	s.snapshot = next
	s.mu.Unlock()

	// One path for every caller. fiber.scheduleUpdate is now safe from any
	// goroutine and at any point in the cycle: it records the fiber under the
	// Root's lock and leaves the ancestor marking to the next render pass, and
	// it declines to re-enter a flush that is already running. The three-way
	// split this replaced existed only to work around a foundation that did
	// neither.
	f.scheduleUpdate()
}

// syncStoreMarkDirty raises the root's dirty flag and returns the root, or nil
// when there is nothing left to schedule for — a detached fiber, or a tree that
// has already been unmounted, which is the "update on an unmounted component"
// case React answers by doing nothing.
func syncStoreMarkDirty(f *fiber) *Root {
	if f == nil {
		return nil
	}
	r := f.root
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unmounted {
		return nil
	}
	r.dirty = true
	return r
}

// syncStoreFuncKey reduces a function value to the code pointer used to decide
// whether `subscribe` changed. A nil or invalid function yields zero. See the
// identity caveat documented on [UseSyncExternalStore] for what this comparison
// can and cannot distinguish.
func syncStoreFuncKey(fn any) uintptr {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

// storeObjectsEqual is [ObjectsEqual] made safe for every value a snapshot may
// hold. ObjectsEqual reaches for reflect.Value.Len on uncomparable kinds, which
// panics for a func — and a snapshot of a func type, or a props value holding a
// callback, is entirely legal. Functions are compared by code pointer here and
// everything else is delegated unchanged.
//
// It is shared with the tree-inspection helpers in testing.go, which compare
// arbitrary prop values for the same reason.
func storeObjectsEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Kind() == reflect.Func {
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
	return ObjectsEqual(a, b)
}

// Store is a minimal observable container: a value plus a set of change
// listeners, shaped so that (*Store).Subscribe and (*Store).Get can be passed
// straight to [UseSyncExternalStore].
//
//	counter := react.NewStore(0)
//	var view react.Component = func(react.Props) react.Node {
//	    n := react.UseSyncExternalStore(counter.Subscribe, counter.Get)
//	    return react.H("b", nil, n)
//	}
//
// It is a convenience, not part of React's API — React deliberately ships no
// store implementation, because the point of the hook is to work with whatever
// store you already have. Use it when a small shared value needs no more than
// this; reach for your own type the moment it does.
//
// The zero Store is ready to use and holds the zero T. All methods are safe for
// concurrent use, and listeners are invoked without the store's lock held, so a
// listener may read the store (as getSnapshot does) without deadlocking.
type Store[T any] struct {
	mu    sync.RWMutex
	value T

	// subs is keyed by an id rather than being a slice so that unsubscribing is
	// O(1) and does not disturb the identity of the other entries.
	subs   map[uint64]func()
	nextID uint64
}

// NewStore returns a Store holding initial. It exists for the type inference —
// NewStore(0) is shorter than &Store[int]{} — and because a pointer is what the
// method values passed to [UseSyncExternalStore] must close over.
func NewStore[T any](initial T) *Store[T] {
	return &Store[T]{value: initial}
}

// Get returns the current value. It satisfies the getSnapshot parameter of
// [UseSyncExternalStore] directly, and is stable while the value is unchanged,
// which is the property that hook requires.
func (s *Store[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

// Set replaces the value and notifies every subscriber. It notifies
// unconditionally, even when the new value equals the old: deciding what
// "equal" means is the snapshot comparison's job, and a store that tries to be
// clever about it is a store that eventually swallows an update.
func (s *Store[T]) Set(v T) {
	s.mu.Lock()
	s.value = v
	listeners := s.listenersLocked()
	s.mu.Unlock()

	for _, fn := range listeners {
		fn()
	}
}

// Update applies fn to the current value and stores the result, holding the
// write lock across the read-modify-write so concurrent increments cannot lose
// each other the way Set(Get()+1) can.
func (s *Store[T]) Update(fn func(T) T) {
	s.mu.Lock()
	s.value = fn(s.value)
	listeners := s.listenersLocked()
	s.mu.Unlock()

	for _, fn := range listeners {
		fn()
	}
}

// Subscribe registers onStoreChange and returns the function that removes it.
// The signature is exactly the subscribe parameter of [UseSyncExternalStore], so
// the method value can be passed with no adapter.
func (s *Store[T]) Subscribe(onStoreChange func()) (unsubscribe func()) {
	if onStoreChange == nil {
		return func() {}
	}

	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[uint64]func(){}
	}
	s.nextID++
	id := s.nextID
	s.subs[id] = onStoreChange
	s.mu.Unlock()

	// Unsubscribing twice is a no-op rather than an error: an effect cleanup may
	// legitimately race with an explicit teardown.
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

// listenersLocked snapshots the listener set. The caller holds the write lock;
// the copy is what lets the notification loop run with the lock released, which
// is what makes it safe for a listener to call Get.
func (s *Store[T]) listenersLocked() []func() {
	if len(s.subs) == 0 {
		return nil
	}
	out := make([]func(), 0, len(s.subs))
	for _, fn := range s.subs {
		out = append(out, fn)
	}
	return out
}
