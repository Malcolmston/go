package react

import "fmt"

// errorBoundaryTag is the element type of an [ErrorBoundary]. Like
// [suspenseTag] it is an empty struct so the renderer can be registered by Go
// type and so reconciliation keeps the boundary's fiber across renders.
type errorBoundaryTag struct{}

// errorBoundaryFallbackKey is the props key holding the boundary's fallback
// function. It is a function rather than a node because the fallback needs the
// recovered value, exactly like React's `<ErrorBoundary fallbackRender>`.
const errorBoundaryFallbackKey = "fallback"

func init() {
	RegisterElementRenderer(errorBoundaryTag{}, renderErrorBoundary)
}

// RecoveredError wraps a panic that the runtime caught on a component's behalf
// and can name a culprit. Go's panic values are untyped, so a raw recovered
// value tells you what went wrong but never where; this type carries both.
//
// It implements error, so it prints readably and can be compared with
// errors.Is/errors.As through [RecoveredError.Unwrap] whenever the underlying
// panic value was itself an error.
//
// [ErrorBoundary] does not wrap what it catches — its fallback receives the raw
// panic value, so `err.(string)` and `err.(error)` work as the author expects.
// The runtime produces a RecoveredError only where the panic originates inside
// the runtime itself and would otherwise be unattributable: a [Lazy] loader that
// fails, and a boundary fallback that panics while handling an error.
type RecoveredError struct {
	// Value is the value originally passed to panic.
	Value any

	// Component names the component or runtime feature that failed, such as
	// "react.Lazy". It is empty when the source could not be determined.
	Component string
}

// Error implements error.
func (e RecoveredError) Error() string {
	if e.Component != "" {
		return fmt.Sprintf("react: %s failed: %v", e.Component, e.Value)
	}
	return fmt.Sprintf("react: component failed: %v", e.Value)
}

// Unwrap exposes the underlying error when the panic value was one, so
// errors.Is and errors.As see through the wrapper. It returns nil for panics of
// any other type, which is what errors.Unwrap expects for a leaf.
func (e RecoveredError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// ErrorBoundary returns a boundary that renders children, and renders
// fallback(err) instead when rendering the children panics. It is the Go
// equivalent of a class component with componentDidCatch, which React 19 still
// has no hook-based replacement for.
//
// Scope, matching React's: a boundary catches panics raised *during rendering*
// of the subtree beneath it — the component function bodies, and everything the
// recursive descent through [reconcileChildren] runs. It does not catch:
//
//   - panics from effects, which run after the commit, off the boundary's call
//     stack entirely (they propagate out of [Root.Flush] to the caller);
//   - panics from event handlers or goroutines the subtree started later, for
//     the same reason;
//   - panics in the boundary's own fallback function — those are re-panicked,
//     wrapped in a [RecoveredError] naming the fallback, because a fallback that
//     fails while handling an error is a bug worth surfacing, not hiding.
//
// It also never catches a [SuspendSignal]: a suspension is a control-flow
// signal, not a failure, so it is re-panicked untouched and an outer [Suspense]
// boundary still sees it. This is what makes ErrorBoundary safe to place inside
// a Suspense boundary, which is the usual arrangement.
//
// Everything else is caught, including runtime errors such as nil map writes and
// out-of-range indexes. Those are genuine bugs, and swallowing them silently
// would hide them — so the recovered value is handed to fallback unchanged
// (a runtime.Error), letting the fallback re-panic or log it as it sees fit.
//
// The Go shape differs from React's in how a boundary resets. React keeps a
// boundary in its error state until something resets it, because the error lives
// in class state; here the boundary simply re-attempts its children on every
// render, so it recovers on its own as soon as they stop panicking. That costs
// one extra try/recover per render and never loops, since catching an error
// schedules no update. For React's sticky behavior, keep the failing subtree
// from re-rendering, or change the boundary element's Key to force a remount.
func ErrorBoundary(fallback func(err any) Node, children ...Node) *Element {
	return &Element{
		Type: errorBoundaryTag{},
		Props: Props{
			errorBoundaryFallbackKey: fallback,
			ChildrenKey:              children,
		},
	}
}

// errorBoundaryState is the boundary's memory, kept in the fiber's state slot.
// It is deliberately small: because the boundary retries on every render, the
// only thing it must remember between renders is which of the two trees —
// children or fallback — is currently mounted.
type errorBoundaryState struct {
	showFallback bool

	// recovered is the value from the most recent catch, kept for debugging and
	// so a future reset API has something to report. Nothing reads it today.
	recovered any
}

// renderErrorBoundary is the boundary's element renderer.
func renderErrorBoundary(f *fiber) {
	st, _ := f.state.(*errorBoundaryState)
	if st == nil {
		st = &errorBoundaryState{}
		f.state = st
	}

	value, caught := errorBoundaryAttempt(f, st)
	if !caught {
		st.showFallback, st.recovered = false, nil
		return
	}

	st.showFallback, st.recovered = true, value

	fallback, _ := f.props.Get(errorBoundaryFallbackKey).(func(err any) Node)
	reconcileChildren(f, []Node{errorBoundaryFallbackNode(fallback, value)})
}

// errorBoundaryAttempt renders the boundary's children and reports the panic
// value if one escaped them. As with [suspenseAttempt] it is a separate
// function so that recover has a frame of its own to run in.
func errorBoundaryAttempt(f *fiber, st *errorBoundaryState) (value any, caught bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, isSuspend := r.(SuspendSignal); isSuspend {
			// A suspension is not an error. Let it continue up to a Suspense
			// boundary — but drop the half-built subtree first, since that
			// boundary is above us and cannot see our children to clean them up
			// once it has unwound past this frame.
			boundaryDropChildren(f)
			st.showFallback = false
			panic(r)
		}

		boundaryDropChildren(f)
		value, caught = r, true
	}()

	// Coming back from the fallback: tear it down so reconciliation cannot hand
	// a content fiber the fallback's hook state.
	if st.showFallback {
		boundaryDropChildren(f)
		st.showFallback = false
	}

	reconcileChildren(f, f.props.Children())
	return nil, false
}

// errorBoundaryFallbackNode calls the fallback function and converts a panic
// inside it into a [RecoveredError] that names it, so a broken fallback reads as
// "the fallback failed" rather than as a mysterious second error from somewhere
// in the tree. A suspension raised by the fallback is passed through unwrapped,
// because an outer [Suspense] boundary is entitled to catch it.
//
// A nil fallback renders nothing, which is the least surprising reading of
// "no fallback was supplied".
func errorBoundaryFallbackNode(fallback func(err any) Node, value any) (node Node) {
	if fallback == nil {
		return nil
	}

	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, isSuspend := r.(SuspendSignal); isSuspend {
			panic(r)
		}
		panic(RecoveredError{Value: r, Component: "ErrorBoundary fallback"})
	}()

	return fallback(value)
}
