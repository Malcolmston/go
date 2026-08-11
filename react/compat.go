package react

// This file is the last few names from React 19's public surface that have no
// home elsewhere: the ones that exist for source compatibility, the ones that
// were renamed upstream, and the ones that report on values rather than doing
// anything. Each carries its status in React 19 in its own doc comment, because
// "why does this exist and should I use it?" is the only real question any of
// them raises.
//
// What is deliberately *not* here matters as much as what is. React exports a
// number of names whose entire meaning is a DOM or a client runtime —
// hydrateRoot, findDOMNode, preload/preinit/preconnect, requestFormReset,
// captureOwnerStack, the class Component/PureComponent pair. A Go function with
// one of those names could only be a stub that lies about a capability, and a
// documented absence is worth more than that. See API-DEVIATIONS.md for the full
// list and the reasoning.

import "reflect"

// Version is the React version whose semantics this package targets, mirroring
// React.version. Code that branches on React's behaviour — a serializer, a
// devtools bridge, a compatibility shim — reads it for the same reason it would
// read React.version.
//
// It is *not* the version of this Go module; that is what the module's own tags
// and its VERSION file record. The two move independently: a bug fix here does
// not change which React the port is modelled on, and tracking a new React minor
// changes this constant without necessarily changing the module's major.
const Version = "19.2.8"

// ForwardRef exists for source compatibility and returns c unchanged.
//
// In React 18 and earlier, a function component could not receive a ref: JSX
// lifted `ref` out of props, so a component that wanted to pass one down to a
// host element had to be wrapped in React.forwardRef, which handed it back as a
// second argument. **React 19 removed that restriction** — `ref` is an ordinary
// prop for function components, and forwardRef is deprecated and slated for
// removal. This port follows React 19: [CreateElement] lifts `ref` into
// [Element.Ref] for host elements, and a component that wants a ref should simply
// declare a prop and read it.
//
// So this is a documented no-op wrapper, kept so that a React port of an existing
// codebase compiles without deleting every ForwardRef call site.
//
// It returns c *itself* rather than a wrapper closure, and that is a correctness
// requirement, not an optimisation. Reconciliation compares component types by
// code pointer (see sameType), so a wrapper built on each call would be a new
// element type on every render and would remount the subtree, destroying exactly
// the state the component was written to keep. Returning c also means
// ForwardRef(c) and c are interchangeable as element types, which is what makes
// removing the calls later a safe mechanical edit.
//
// A nil component panics, matching [Memo] and [Lazy]: the wrapper is normally
// built at package initialisation, where a stack trace still points at the
// mistake.
func ForwardRef(c Component) Component {
	if c == nil {
		panic("react: ForwardRef called with a nil component")
	}
	return c
}

// UseFormState is the pre-19 name for [UseActionState] and is an exact alias.
//
// React renamed useFormState to useActionState in 19 — the hook was never
// specific to <form>, and the new name says so — and kept the old one as a
// deprecated re-export for one release. The signature did not change, so the
// alias here is genuine rather than approximate: same arguments, same three
// return values, same hook slot.
//
// Prefer [UseActionState] in new code. This name is here so that ported code
// compiles, and so that a reader who learned the hook by its old name finds it.
func UseFormState[S any](action func(prev S, data FormData) S, initial S) (state S, submit func(FormData), isPending bool) {
	return UseActionState(action, initial)
}

// CreateRef returns an empty [Ref], mirroring React.createRef.
//
// It is the non-hook way to make a ref: no fiber is involved, nothing is
// remembered between renders, and the caller owns the lifetime. React documents
// it as "mostly for class components", which this port does not have, but the one
// use that survives into function components survives here too — a parent that
// wants to own the handle a child publishes through [UseImperativeHandle] builds
// the box itself and passes it down.
//
// Never call it from a component body expecting it to persist: a fresh ref every
// render is a fresh empty box every render. That is [UseRef]'s job.
//
// The Go shape differs from React's in the usual way — the ref is generic, so
// [Ref.Current] is typed and needs no assertion. `&Ref[T]{}` is exactly
// equivalent; this spelling exists so that the React name is findable.
func CreateRef[T any]() *Ref[T] {
	return &Ref[T]{}
}

// IsValidElementType reports whether t may be used as an [Element.Type] — the
// question React's isValidElementType (from react-is) answers, and the one a
// wrapper component has to ask before building an element around a type it was
// handed.
//
// Note the distinction from [IsValidElement], which is about a *value in the tree*
// being an element. This is about a value being usable in the type *position*:
//
//	IsValidElement(H("div", nil))   // true  — that is an element
//	IsValidElementType("div")       // true  — that is a tag name
//
// It returns true for exactly what [renderFiber] can dispatch on:
//
//   - a non-empty tag string. The empty string is rejected because it would emit
//     "<>" and produce malformed HTML rather than an error.
//   - a non-nil [Component].
//   - [Fragment], and the runtime's internal text type.
//   - any type registered through [RegisterElementRenderer] or
//     [RegisterTransparentType] — which covers *[MemoType], *[LazyType], the
//     Suspense, StrictMode, Profiler and ErrorBoundary markers, every context
//     provider, and marker types declared by other packages such as
//     [github.com/malcolmston/react/rsc].
//
// Typed nil pointers are rejected even when their type is registered: a
// (*MemoType)(nil) passes the type test but its handler panics on the nil, so
// reporting it valid would only move the failure further from the mistake.
//
// Everything else — nil, an int, an unregistered struct — is false. Rendering
// such a type panics with a message naming it; this is how to find out first.
func IsValidElementType(t any) bool {
	switch v := t.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case Component:
		return v != nil
	case fragmentTag, textTag:
		return true
	}

	rt := reflect.TypeOf(t)
	if _, ok := elementRenderers[rt]; !ok {
		return false
	}

	// A registered type still has to hold something. Only the nil-able kinds can
	// fail this, and reflect.Value.IsNil panics on any other kind, so the switch
	// is a guard rather than a convenience.
	switch rv := reflect.ValueOf(t); rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		return !rv.IsNil()
	}
	return true
}
