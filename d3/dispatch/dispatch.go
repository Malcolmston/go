// Package dispatch is a Go port of d3-dispatch: a small event bus with named
// event types and namespaced listener registration.
//
// # Why this exists in a language that has channels
//
// It is a fair question, and it deserves a straight answer rather than a
// reflex port. A channel already gives you fan-out-by-hand, backpressure and
// select. What it does not give you, and what this package is for, is:
//
//   - **Named types on one object.** A simulation or a layout emits "start",
//     "tick" and "end". With channels that is three fields, three lifetimes
//     and three shutdown paths; here it is one value and one string.
//   - **Namespaced registration and, crucially, de-registration.** A listener
//     registers as "tick.legend", and later "tick.legend" replaces or removes
//     exactly that listener without disturbing "tick.axis". There is no
//     channel equivalent — you would have to invent a registry, which is this
//     package.
//   - **Synchronous, ordered delivery.** Every listener runs to completion, in
//     registration order, on the caller's goroutine, before [Dispatch.Call]
//     returns. That is what a layout callback needs: "tick" means "the state
//     is consistent right now, read it". A channel hand-off makes the reader
//     concurrent with the next mutation, which is a different and usually
//     wrong contract.
//
// If what you want is asynchronous decoupling, a buffered channel is the
// better tool and you should use it. If what you want is "let several
// interested parties observe this state change, right here, and let them
// unsubscribe by name", this is the tool.
//
//	d := dispatch.MustNew[float64]("start", "tick", "end")
//	d.On("tick.legend", func(alpha float64) { redrawLegend(alpha) })
//	d.On("tick.axis", func(alpha float64) { redrawAxis(alpha) })
//	d.Call("tick", 0.3)
//	d.On("tick.legend", nil) // remove just that one
//
// # Typed payloads instead of variadic arguments
//
// d3's dispatch forwards `...args` and a `this`, both untyped. Go has no
// `this` and untyped variadics defeat the point of the language, so a
// [Dispatch] is generic over a single payload type. Pass a struct when an
// event carries several things; pass struct{}{} when it carries nothing.
//
// # Goroutine safety
//
// **This port is safe for concurrent use; upstream is not.** d3 runs in a
// single-threaded event loop and its dispatch does no locking at all. Go
// programmers will reasonably assume otherwise the moment a "tick" comes off a
// worker goroutine, so registration, removal, copying and dispatch are all
// guarded here.
//
// The guarantee is specific, and worth reading once:
//
//   - [Dispatch.On], [Dispatch.Copy], [Dispatch.Listener] and [Dispatch.Call]
//     may be called from any goroutine at any time.
//   - Call snapshots the listener list for the type and then releases the lock
//     *before* invoking anything. Listeners therefore run without holding the
//     bus, so a listener may safely register, remove or dispatch — no
//     deadlock, no reentrancy trap.
//   - The consequence, which is also d3's copy-on-write semantics: a listener
//     added or removed during a dispatch does not affect the dispatch already
//     in flight. It takes effect from the next Call.
//   - Nothing serializes the listeners themselves. Two goroutines calling the
//     same type concurrently will run the same listener concurrently; if that
//     listener touches shared state, it needs its own lock.
package dispatch

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrUnknownType is returned by [Dispatch.Apply] and wrapped by the errors
// from [New] and [Dispatch.On] when an event type was never declared.
//
// Types are fixed at construction on purpose: a typo in "tick" would otherwise
// register a listener that is simply never called, which is the single most
// common way to lose an afternoon to an event bus.
var ErrUnknownType = errors.New("dispatch: unknown event type")

// Listener is a callback. It runs synchronously on the goroutine that called
// [Dispatch.Call].
type Listener[T any] func(arg T)

// namedListener is one registration: the callback plus the namespace that can
// later address it. An empty name is a legitimate namespace — registering
// "tick" twice replaces the first, exactly as upstream.
type namedListener[T any] struct {
	name string
	fn   Listener[T]
}

// Dispatch is a bus over a fixed set of named event types.
//
// Construct one with [New] or [MustNew]; the zero value is not usable. It is
// safe for concurrent use — see the package documentation for exactly what
// that guarantees.
type Dispatch[T any] struct {
	mu sync.RWMutex
	// Listener slices are copy-on-write: a mutation replaces the slice rather
	// than editing it, so a dispatch in flight can hold the old one without a
	// lock and without racing.
	listeners map[string][]namedListener[T]
	// order preserves the declared type order for Copy and for error messages.
	order []string
}

// New returns a Dispatch for the given event types.
//
// An empty type name, a name containing a "." (which would be ambiguous with a
// namespace) and a duplicate are all errors. d3 throws on the same three
// conditions.
func New[T any](types ...string) (*Dispatch[T], error) {
	d := &Dispatch[T]{
		listeners: make(map[string][]namedListener[T], len(types)),
		order:     make([]string, 0, len(types)),
	}
	for _, t := range types {
		switch {
		case t == "":
			return nil, errors.New("dispatch: empty event type")
		case strings.Contains(t, "."):
			return nil, fmt.Errorf("dispatch: illegal event type %q: a type may not contain %q", t, ".")
		}
		if _, dup := d.listeners[t]; dup {
			return nil, fmt.Errorf("dispatch: duplicate event type %q", t)
		}
		d.listeners[t] = nil
		d.order = append(d.order, t)
	}
	return d, nil
}

// MustNew is [New] for the usual case, where the type names are literals in
// the source and a bad one is a programmer error rather than bad input. It
// panics instead of returning an error.
func MustNew[T any](types ...string) *Dispatch[T] {
	d, err := New[T](types...)
	if err != nil {
		panic(err)
	}
	return d
}

// Types returns the declared event types in declaration order.
func (d *Dispatch[T]) Types() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]string(nil), d.order...)
}

// typename is one parsed "type.name" specifier. An empty typ means "every
// declared type", which is only meaningful for removal.
type typename struct {
	typ  string
	name string
}

// parseTypenames splits a whitespace-separated list of "type.name" specifiers.
func (d *Dispatch[T]) parseTypenames(s string) ([]typename, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, errors.New("dispatch: no event type given")
	}
	out := make([]typename, 0, len(fields))
	for _, f := range fields {
		typ, name := f, ""
		if i := strings.Index(f, "."); i >= 0 {
			typ, name = f[:i], f[i+1:]
		}
		if typ != "" {
			if _, ok := d.listeners[typ]; !ok {
				return nil, fmt.Errorf("%w: %q", ErrUnknownType, typ)
			}
		}
		out = append(out, typename{typ: typ, name: name})
	}
	return out, nil
}

// On registers, replaces or removes listeners.
//
// typenames is a whitespace-separated list of specifiers, each "type" or
// "type.name":
//
//	d.On("start", fn)                 // the unnamed listener for "start"
//	d.On("tick.legend", fn)           // a named listener, replacing any with that name
//	d.On("start.a end.a", fn)         // the same listener on two types
//	d.On("tick.legend", nil)          // remove that one listener
//	d.On(".legend", nil)              // remove every listener named "legend", of any type
//
// A registration replaces any listener with the same type *and* name, so
// re-running setup code does not accumulate duplicates — that is the whole
// point of the namespace. A new listener is appended, so listeners run in
// first-registration order.
//
// Passing a nil callback removes. Removing something that was never registered
// is not an error. Naming an undeclared type is, and the error wraps
// [ErrUnknownType].
func (d *Dispatch[T]) On(typenames string, callback Listener[T]) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tns, err := d.parseTypenames(typenames)
	if err != nil {
		return err
	}
	// A bare "." prefix is only meaningful as a removal; registering the same
	// callback under every type is far more likely to be a typo than an
	// intention, and upstream rejects it too.
	if callback != nil {
		for _, tn := range tns {
			if tn.typ == "" {
				return fmt.Errorf("dispatch: %q has no event type; a wildcard namespace can only be used to remove listeners", typenames)
			}
		}
	}

	for _, tn := range tns {
		types := []string{tn.typ}
		if tn.typ == "" {
			types = d.order
		}
		for _, typ := range types {
			d.setLocked(typ, tn.name, callback)
		}
	}
	return nil
}

// setLocked replaces, appends or removes one listener. The slice is rebuilt
// rather than mutated so that any concurrent Call holding the old slice is
// unaffected.
func (d *Dispatch[T]) setLocked(typ, name string, callback Listener[T]) {
	old := d.listeners[typ]
	next := make([]namedListener[T], 0, len(old)+1)
	replaced := false
	for _, l := range old {
		if l.name != name {
			next = append(next, l)
			continue
		}
		replaced = true
		if callback != nil {
			// Replace in place, preserving the original position so that
			// re-registering does not reorder the dispatch.
			next = append(next, namedListener[T]{name: name, fn: callback})
		}
	}
	if !replaced && callback != nil {
		next = append(next, namedListener[T]{name: name, fn: callback})
	}
	if len(next) == 0 {
		d.listeners[typ] = nil
		return
	}
	d.listeners[typ] = next
}

// Listener returns the callback registered for a single "type.name"
// specifier, or nil if there is none.
//
// This is the read half of d3's overloaded .on(), which cannot be one method
// in Go. It is named for what it returns rather than being called On, because
// the two do genuinely different things and "does anything read this back?"
// is answered yes here — a caller sometimes needs to wrap an existing
// listener rather than replace it.
func (d *Dispatch[T]) Listener(spec string) Listener[T] {
	d.mu.RLock()
	defer d.mu.RUnlock()

	typ, name := spec, ""
	if i := strings.Index(spec, "."); i >= 0 {
		typ, name = spec[:i], spec[i+1:]
	}
	for _, l := range d.listeners[typ] {
		if l.name == name {
			return l.fn
		}
	}
	return nil
}

// Copy returns an independent Dispatch with the same types and the same
// listeners registered.
//
// Later changes to either bus do not affect the other, which is what makes it
// useful: a behaviour hands each new instance a copy of its prototype's
// dispatch, so per-instance listeners do not leak into the prototype.
func (d *Dispatch[T]) Copy() *Dispatch[T] {
	d.mu.RLock()
	defer d.mu.RUnlock()

	c := &Dispatch[T]{
		listeners: make(map[string][]namedListener[T], len(d.listeners)),
		order:     append([]string(nil), d.order...),
	}
	for typ, ls := range d.listeners {
		// The slices are copy-on-write, so sharing the backing array is safe:
		// neither bus will ever write into it.
		c.listeners[typ] = ls
	}
	return c
}

// Call invokes every listener registered for the type, in registration order,
// synchronously on the calling goroutine, and returns when the last one has
// returned.
//
// It panics if the type was never declared — the same class of error as
// indexing a map that must have a key, and upstream throws here too. Use
// [Dispatch.Apply] when the type name came from configuration rather than from
// a literal.
//
// A listener that panics propagates; nothing is recovered, so a broken
// listener is not silently swallowed and the remaining listeners do not run.
func (d *Dispatch[T]) Call(typ string, arg T) {
	if err := d.Apply(typ, arg); err != nil {
		panic(err)
	}
}

// Apply is [Dispatch.Call] with an unknown type reported as an error wrapping
// [ErrUnknownType] rather than as a panic.
//
// Upstream's .apply differs from .call only in how the *arguments* are passed
// — an array instead of a variadic list — which a single typed payload makes
// meaningless. Rather than ship an exact synonym, the port gives the name to
// the distinction Go code actually needs.
func (d *Dispatch[T]) Apply(typ string, arg T) error {
	d.mu.RLock()
	ls, ok := d.listeners[typ]
	d.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, typ)
	}
	// The lock is released before any listener runs, so a listener may freely
	// call On, Copy or Call. The slice it holds is immutable by construction.
	for _, l := range ls {
		l.fn(arg)
	}
	return nil
}
