package react

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// Async is a value that is being computed somewhere else — the port's stand-in
// for a JavaScript Promise, and the only thing [Use] knows how to read.
//
// The mapping is worth stating plainly, because it is the whole reason this
// type exists. A Promise is two things glued together: a slot that will
// eventually hold either a value or a rejection, and a signal that says "the
// slot is filled now". JavaScript hides both behind .then, because its runtime
// has an event loop to resume the continuation on. Go has neither promises nor
// an event loop, but it has the two ingredients directly: a channel is a
// completion signal, and a struct field is a slot. So an Async is exactly
//
//	done  chan struct{}  — closed once, when the work settles (the signal)
//	value T, err error    — the settled result (the slot)
//	mu    sync.Mutex      — guards the slot against the writer/reader race
//
// and nothing else. Closing a channel is the one broadcast primitive Go gives
// for free: any number of waiters may select on it, it is safe to observe from
// any goroutine, and observing the close establishes a happens-before edge with
// everything the producer wrote beforehand. That is precisely a Promise's
// "settled" semantics, minus the microtask queue.
//
// Differences from a Promise that callers should know about:
//
//   - There is no .then, no chaining and no combinator set. Composition in Go
//     is a goroutine that reads several Asyncs, so [Go] plus [Async.Result] is
//     all the plumbing needed.
//   - An Async settles exactly once and then never changes. It is therefore
//     safe to share the same *Async across goroutines, renders and roots, which
//     is what makes [Cache] possible.
//   - A rejection is an ordinary Go error rather than a thrown value, and a
//     panic inside the producer becomes an [*AsyncPanic] error (see [Go]).
//
// The zero Async is not usable; always construct one with [Go], [Resolved] or
// [Failed].
type Async[T any] struct {
	// done is closed exactly once, by settle, when value/err are final. It is
	// the channel handed to [Suspend], so a Suspense boundary retries the
	// subtree the moment the work finishes.
	done chan struct{}

	// mu guards value and err. Strictly the close of done already orders the
	// producer's writes before any consumer's read, but the mutex keeps
	// [Async.Done] and [Async.TryResult] — which peek without waiting — free of
	// any subtlety, and costs nothing on a type that settles once.
	mu    sync.Mutex
	value T
	err   error
}

// AsyncPanic is the error an [Async] carries when its producer function
// panicked instead of returning. React has no analogue: in JavaScript a throw
// inside a promise executor is simply a rejection, but in Go a panic in a
// goroutine that nobody recovers takes the whole process down. [Go] therefore
// recovers, wraps the panic value here and settles the Async with it, so a
// crashing resource surfaces as a rendering error an error boundary can catch
// rather than as a silent process death.
//
// Value is the original panic value, preserved unchanged so callers can type
// assert on it, and Stack is the producer goroutine's stack captured at the
// moment of recovery — without it the panic's origin would be lost, since the
// consumer's stack is somewhere else entirely.
type AsyncPanic struct {
	// Value is the value passed to panic, exactly as recover returned it.
	Value any

	// Stack is the producer goroutine's stack trace at recovery time.
	Stack []byte
}

// Error renders the panic value the way a Go runtime panic message would.
func (p *AsyncPanic) Error() string {
	return fmt.Sprintf("react: async resource panicked: %v", p.Value)
}

// Unwrap exposes the panic value when it was itself an error, so that
// errors.Is and errors.As see through the wrapper to the original.
func (p *AsyncPanic) Unwrap() error {
	if err, ok := p.Value.(error); ok {
		return err
	}
	return nil
}

// Go starts fn on its own goroutine and returns a handle to the result
// immediately. It is the constructor that corresponds to `new Promise(…)` — or
// more precisely to an async function call, since the work begins right away
// rather than when someone subscribes.
//
// The returned Async settles when fn returns, whether or not anybody is
// waiting: the goroutine never sends on a channel and never blocks, it only
// writes two fields and closes done. That is what makes an ignored resource
// leak-free — dropping the *Async on the floor costs one goroutine that is
// already on its way to termination, not one parked forever on an unread
// send.
//
// A panic inside fn is recovered and becomes the resource's error as an
// [*AsyncPanic]; see that type for why. fn is called exactly once.
func Go[T any](fn func() (T, error)) *Async[T] {
	a := &Async[T]{done: make(chan struct{})}

	go func() {
		// settled guards the deferred recovery: on the normal path settle has
		// already run, and settling twice would panic on the second close.
		settled := false
		defer func() {
			if settled {
				return
			}
			// Reaching here means fn panicked (or called runtime.Goexit, in
			// which case recover returns nil and we still must settle rather
			// than leave every waiter blocked forever).
			r := recover()
			var zero T
			a.settle(zero, &AsyncPanic{Value: r, Stack: debug.Stack()})
		}()

		v, err := fn()
		settled = true
		a.settle(v, err)
	}()

	return a
}

// Resolved returns an Async that has already settled successfully, the
// equivalent of Promise.resolve. It allocates no goroutine: the done channel is
// created closed, so [Use] on it never suspends. Use it for cache warm-ups, for
// synchronous fallbacks and, most of all, in tests, where an already-settled
// resource makes the "does not suspend" path trivial to exercise.
func Resolved[T any](value T) *Async[T] {
	a := &Async[T]{done: make(chan struct{}), value: value}
	close(a.done)
	return a
}

// Failed returns an Async that has already settled with err, the equivalent of
// Promise.reject. [Use] on it panics err immediately, so it is the shortest way
// to drive an error boundary from a test.
//
// A nil err is accepted and makes the resource an ordinary successful one
// holding the zero value; refusing it would only push a nil check onto every
// caller that forwards an error it already has.
func Failed[T any](err error) *Async[T] {
	a := &Async[T]{done: make(chan struct{}), err: err}
	close(a.done)
	return a
}

// settle records the result and fires the completion signal, in that order —
// the write must be visible before the close, since the close is what every
// consumer uses to decide the fields are readable.
func (a *Async[T]) settle(value T, err error) {
	a.mu.Lock()
	a.value, a.err = value, err
	a.mu.Unlock()
	close(a.done)
}

// Ready returns the channel that closes when the resource settles. It is the
// channel [Use] hands to [Suspend], and the one a Suspense boundary waits on
// before retrying; callers can also select on it directly to combine a resource
// with a timeout or a cancellation channel.
//
// The channel is never sent on, only closed, so receiving from it more than
// once — from any number of goroutines — is fine.
func (a *Async[T]) Ready() <-chan struct{} { return a.done }

// Done reports whether the resource has settled, without blocking. It answers
// the same question as Ready-closed, in the form a plain `if` wants.
func (a *Async[T]) Done() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

// Result blocks until the resource settles and returns its value and error.
// This is the "await" of the port and belongs in ordinary Go code — a
// goroutine, a handler, a test — never in a component body, where blocking the
// render is exactly what [Use] and Suspense exist to avoid.
func (a *Async[T]) Result() (T, error) {
	<-a.done
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.value, a.err
}

// TryResult returns the settled value and error, reporting ok=false and the
// zero value when the resource is still pending. It never blocks, which is what
// makes it the primitive [Use] is built on: a render must decide "value or
// suspend" without ever waiting.
func (a *Async[T]) TryResult() (value T, err error, ok bool) {
	select {
	case <-a.done:
	default:
		var zero T
		return zero, nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.value, a.err, true
}

// Use reads a resource during a render — React 19's use(). It has three
// outcomes, matching use() exactly:
//
//   - The resource has settled successfully: its value is returned and the
//     render continues as if the data had always been there.
//   - The resource is still pending: Use calls [Suspend] with [Async.Ready],
//     which panics a [SuspendSignal]. The nearest Suspense boundary catches it,
//     shows its fallback, and re-renders the subtree when the channel closes.
//     Use does not return in this case.
//   - The resource failed: the error is panicked, so the nearest error boundary
//     renders its fallback. This mirrors use() on a rejected promise, which
//     throws the rejection reason into the render.
//
// Use is not an ordinary hook, and the difference is its defining property:
// **it may be called conditionally, inside an if, inside a loop, or after an
// early return**. Every other hook is illegal in those positions because
// nextHook identifies a hook's state by its call index, so a skipped call
// shifts every later slot. Use has no state to identify — everything it needs
// is in the *Async argument the caller already holds — and the implementation
// below genuinely never touches nextHook or the hook cursor. That is not an
// accident to be preserved by convention; it is the contract.
//
// Called outside a render — from a goroutine, a test, an event handler — Use
// cannot suspend, because there is no boundary above it and no render to
// restart. Rather than panic, it degrades to the obvious blocking read: it
// waits for the resource via [Async.Result] and then returns the value or
// panics the error, which is what a caller with no fallback to show wants
// anyway. The rule is therefore "Use blocks off-render, suspends on-render",
// and code that must not block should call [Async.Result] or [Async.TryResult]
// explicitly to say so.
//
// A nil *Async is a programming error and panics with a message saying so,
// rather than blocking forever on a nil channel.
func Use[T any](a *Async[T]) T {
	if a == nil {
		panic("react: Use called with a nil *Async; " +
			"construct resources with react.Go, react.Resolved or react.Failed")
	}

	value, err, ok := a.TryResult()
	if !ok {
		// currentlyRendering is the dispatcher fiber.go keeps for hooks. Reading
		// it here is the only way to tell a render apart from ordinary code, and
		// renders are serialized by renderMu, so during a render the value is
		// stable for the whole call.
		if currentlyRendering != nil {
			Suspend(a.Ready()) // never returns
		}
		value, err = a.Result()
	}
	if err != nil {
		panic(err)
	}
	return value
}
