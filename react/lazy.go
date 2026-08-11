package react

import (
	"errors"
	"fmt"
	"sync"
)

func init() {
	// The sample is a typed nil pointer: RegisterElementRenderer keys on the Go
	// type of the value, and *LazyType is the type every lazy element carries.
	RegisterElementRenderer((*LazyType)(nil), renderLazy)
}

// LazyType is the element type produced by [Lazy]: a component that is not
// loaded yet. Use the *LazyType returned by [Lazy] as an [Element.Type], the
// same way a [Component] value is used:
//
//	var Chart = react.Lazy(func() (react.Component, error) { … })
//	el := &react.Element{Type: Chart, Props: react.Props{"data": data}}
//
// The first render of any element with this type starts the loader on its own
// goroutine and calls [Suspend], so the nearest [Suspense] boundary shows its
// fallback; when loading finishes the boundary retries and the real component
// renders with the element's props. A loader that fails panics, so an
// [ErrorBoundary] can show the failure.
//
// A LazyType is the unit of sharing, exactly like React's `React.lazy` result:
// the loader runs at most once per LazyType no matter how many elements use it,
// how many [Root]s they are mounted in, or how many times a boundary retries.
// Store one in a package-level variable and reuse it; constructing a new one per
// render would reload every time and also force a remount, since reconciliation
// compares element types by pointer.
//
// A *LazyType is safe for concurrent use.
type LazyType struct {
	// once guarantees the load-exactly-once contract. It is the whole reason
	// LazyType is a pointer type: a copy would carry a fresh Once and reload.
	once sync.Once

	load func() (Component, error)

	// ready is closed when loading has finished, successfully or not. It is the
	// channel handed to [Suspend], and closing it is what publishes comp and err
	// to every goroutine that later observes the close.
	ready chan struct{}

	// comp and err are written once by the loader goroutine before ready is
	// closed, and only ever read after a receive on ready. The channel close is
	// the happens-before edge, so no additional lock is needed.
	comp Component
	err  error
}

// Lazy returns a [LazyType] that defers loading a component until it is first
// rendered. It mirrors React.lazy, with two differences that Go forces.
//
// React's loader returns a promise; here it is an ordinary blocking function
// returning (Component, error), and the runtime moves it off the render
// goroutine itself. Whatever it does — read a file, build a component from a
// template, wait on a network fetch — it may block for as long as it likes.
//
// And React's loader signals failure by rejecting; here a non-nil error is
// panicked as a [RecoveredError] on the next render attempt, since panicking is
// how this port moves a render-time failure to an [ErrorBoundary]. A loader that
// panics is treated the same way, and a loader that returns (nil, nil) is
// reported as an error rather than rendering nothing, because a nil component is
// always a bug in the loader.
//
// load is never called more than once. Every element using the returned
// *LazyType — across every [Root], across every Suspense retry — shares the one
// result.
func Lazy(load func() (Component, error)) *LazyType {
	if load == nil {
		panic("react: Lazy needs a non-nil loader function")
	}
	return &LazyType{load: load, ready: make(chan struct{})}
}

// start kicks off the loader on its first call and does nothing on every later
// one. The goroutine is what keeps a blocking loader from stalling the render:
// the render that called start suspends immediately afterwards rather than
// waiting.
func (l *LazyType) start() {
	l.once.Do(func() {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// A panicking loader is a failed load, not a crashed
					// program: turn it into the error the next render reports.
					l.comp, l.err = nil, fmt.Errorf("loader panicked: %v", r)
				}
				close(l.ready)
			}()

			comp, err := l.load()
			if err == nil && comp == nil {
				err = errors.New("loader returned a nil Component")
			}
			l.comp, l.err = comp, err
		}()
	})
}

// renderLazy renders a lazy element: start the load, suspend while it is
// outstanding, then render the loaded component with this element's props.
//
// The props are forwarded verbatim, children included, so a lazy element is a
// drop-in replacement for the component it loads. The loaded component becomes
// the fiber's only child rather than replacing the lazy fiber itself, which
// keeps the lazy fiber — and the identity the reconciler matches on — stable
// across the suspend/retry cycle.
func renderLazy(f *fiber) {
	l, ok := f.typ.(*LazyType)
	if !ok {
		panic(fmt.Sprintf("react: lazy renderer got element type %T", f.typ))
	}

	l.start()

	// Non-blocking: a render must never block the render goroutine, and while
	// the load is outstanding the right answer is to hand control to the nearest
	// Suspense boundary.
	select {
	case <-l.ready:
	default:
		Suspend(l.ready)
	}

	if l.err != nil {
		panic(RecoveredError{Value: l.err, Component: "react.Lazy"})
	}

	reconcileChildren(f, []Node{&Element{Type: l.comp, Props: f.props}})
}
