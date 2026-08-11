package react

import "sync"

// Cached is a memoized resource loader: a function from a key to an [Async],
// where each distinct key is computed at most once and every caller of that key
// receives the identical handle. It is the port's version of React's cache(),
// with the piece React does not need bolted on — an explicit way to throw
// entries away.
//
// Why the extra type exists. React's cache() returns a bare function because in
// React Server Components the memo lives for exactly one request and is
// discarded with it; there is nothing to invalidate, because nothing outlives
// the render. A Go program has no such boundary: a package-level cache built at
// init lives for the life of the process, so a cache with no invalidation is a
// map that only ever grows and never notices that the world changed underneath
// it. [Cached.Invalidate] and [Cached.Clear] are the answer, and [Cache] is
// kept as the thin React-shaped wrapper for callers that want the plain
// function and have no need for either.
//
// Sharing the *Async rather than the value is the point of the design. Two
// components that ask for the same key during the same render get the same
// handle, so they suspend on the same channel, resume in the same pass, and see
// the same result — the deduplication React's cache() provides, expressed as
// pointer identity instead of a promise cache.
//
// Eviction policy: there is none. Entries live until [Cached.Invalidate] or
// [Cached.Clear] removes them, and the map grows without bound. That is a
// deliberate trade — the entries are shared handles whose identity is load
// bearing, so evicting one behind the caller's back would silently split
// consumers of the same key onto different resources — but it means a cache
// keyed by something unbounded (a user id, a URL, a timestamp) is a memory leak
// unless the owner prunes it. Cache what is small and enumerable; for anything
// else, keep the [Cached] value and invalidate on a schedule or on the same
// event that makes the data stale.
//
// A Cached is safe for concurrent use by multiple goroutines. The zero value is
// not usable; construct one with [NewCache].
type Cached[K comparable, V any] struct {
	fn func(K) (V, error)

	// mu guards entries and, crucially, spans the create-and-store step. That is
	// what makes the cache single-flight: two goroutines racing on a cold key
	// cannot both reach [Go], because the loser finds the winner's handle
	// already in the map. Holding a lock across the start of the work is cheap
	// here precisely because [Go] returns immediately — the user function runs
	// on its own goroutine, outside the lock.
	mu      sync.Mutex
	entries map[K]*Async[V]
}

// NewCache wraps fn in a [Cached]. fn is called at most once per distinct key
// and always on a goroutine of its own, so a slow loader never blocks the
// caller that asked for it — the caller gets a pending [Async] and suspends.
//
// A panic inside fn is captured by [Go] and surfaces as the entry's error, so a
// crashing loader cannot take the process down. Note that such a failure is
// cached like any other result: the key stays failed until it is invalidated.
// That is the same behavior React's cache() has for a rejected promise, and it
// is why retry belongs in [Cached.Invalidate] rather than in a hidden policy.
func NewCache[K comparable, V any](fn func(K) (V, error)) *Cached[K, V] {
	if fn == nil {
		panic("react: NewCache requires a non-nil loader function")
	}
	return &Cached[K, V]{fn: fn, entries: map[K]*Async[V]{}}
}

// Get returns the shared resource for key, starting the load on the first call
// and returning the very same *Async on every later one. Calls for the same
// cold key from several goroutines at once produce exactly one call to the
// loader (single flight) and one handle.
func (c *Cached[K, V]) Get(key K) *Async[V] {
	c.mu.Lock()
	defer c.mu.Unlock()

	if a, ok := c.entries[key]; ok {
		return a
	}

	k := key
	a := Go(func() (V, error) { return c.fn(k) })
	c.entries[key] = a
	return a
}

// Peek returns the cached resource for key without starting one, reporting
// whether an entry exists. It is the read a test or a metrics probe wants:
// asking "is this loaded?" through [Cached.Get] would answer by loading it.
func (c *Cached[K, V]) Peek(key K) (*Async[V], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.entries[key]
	return a, ok
}

// Invalidate drops the entry for key and reports whether one was present. The
// next [Cached.Get] for that key starts a fresh load and hands back a new
// handle.
//
// Invalidation is deliberately not cancellation. Anyone already holding the old
// *Async keeps it and still sees its result — a component that suspended on the
// stale handle resumes normally instead of hanging forever, which is the only
// behavior that is safe while a render is in flight. Invalidate means "stop
// handing this out", not "abandon it".
func (c *Cached[K, V]) Invalidate(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok {
		return false
	}
	delete(c.entries, key)
	return true
}

// Clear drops every entry, returning the cache to its just-constructed state.
// It is [Cached.Invalidate] for the whole keyspace — the reset to reach for
// between tests, or when a bulk reload makes everything stale at once.
func (c *Cached[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[K]*Async[V]{}
}

// Len reports how many keys are currently cached, settled or still loading. It
// exists so the unbounded-growth risk documented on [Cached] is observable
// rather than a matter of faith.
func (c *Cached[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Cache wraps fn so that each distinct argument is computed at most once and
// every caller shares the resulting [Async] — React 19's cache(), in the shape
// React gives it: a function in, a function out.
//
//	loadUser := react.Cache(fetchUser)
//	// …in a component:
//	user := react.Use(loadUser(id))
//
// Two calls with the same key return the identical *Async, so components that
// need the same data suspend on one channel and resume together; two calls with
// different keys return different handles and run concurrently.
//
// The returned function is safe to call from any goroutine and is single
// flight: a cold key concurrently requested by many callers invokes fn once.
//
// Cache has no eviction and no invalidation, matching cache()'s signature at
// the cost of growing forever — see [Cached] for the full discussion. When the
// cache outlives a request, which in Go it almost always does, prefer
// [NewCache] and keep the [Cached] value so that [Cached.Invalidate] and
// [Cached.Clear] remain reachable.
func Cache[K comparable, V any](fn func(K) (V, error)) func(K) *Async[V] {
	return NewCache(fn).Get
}
