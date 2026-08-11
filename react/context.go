package react

import (
	"fmt"
	"sync/atomic"
)

// This file ports React's context API: [CreateContext], the provider element it
// hands back, the render-prop consumer, and [UseContext]. Context is the escape
// hatch from prop drilling — a value published at one point in the tree and read
// anywhere beneath it, without every intermediate component forwarding it.
//
// # Why the provider is not a generic type
//
// This is the single most surprising part of the port, and it is worth reading
// before changing anything here.
//
// The natural Go shape would be a generic provider element type, something like
// `type providerTag[T any] struct{}`, so the value could travel with its static
// type. That shape cannot work. [RegisterElementRenderer] keys handlers by the
// Go type of [Element.Type] and panics when the same type registers twice, and
// registration happens from an init function — code that runs once, before any
// user type argument is known. A generic tag instantiates a fresh, distinct Go
// type for every T a program ever uses (contextProviderTag[int],
// contextProviderTag[User], …), and there is no place to register any of them:
// init cannot enumerate the instantiations, and registering lazily from
// [Context.Provider] would either race or panic on the second context of the
// same T.
//
// So the element type is one plain, non-generic struct — [contextProviderTag] —
// registered exactly once from this file's init. Everything that varies per
// context travels through [Props] instead, which is already an untyped bag:
//
//   - contextIdentityKey holds the context's identity (a *contextID allocated
//     in [CreateContext]), so a provider fiber can be matched to the context it
//     provides for.
//   - contextValueKey holds the provided value boxed as `any`.
//   - [ChildrenKey] holds the children, as for any other element.
//
// The generics therefore live entirely at the API surface: [Context] is
// generic, [Context.Provider] and [UseContext] are generic, and the single
// unchecked type assertion in [UseContext] is where the static type is handed
// back. That assertion cannot fail unless someone hand-builds a provider
// element with a mismatched value, which is a programming error and panics
// naming both types.
//
// # Why identity is a pointer and not the *Context itself
//
// A *Context[T] pointer would work as identity, but the props value would then
// be a generic type again, forcing reflection to compare it. A dedicated,
// non-generic *contextID keeps the comparison a plain interface `==`. The ID
// struct carries a sequence number rather than being empty on purpose: Go is
// permitted to give every allocation of a zero-size type the same address, so
// `new(struct{})` is not a reliable identity.

// contextProviderTag is the [Element.Type] of every context provider element,
// for every context and every value type. It is deliberately not generic; see
// the file comment for why. It carries no data — the context identity and the
// provided value live in the element's props.
type contextProviderTag struct{}

// The reserved props keys a provider element uses. They are namespaced with a
// "react:" prefix so they cannot collide with a user's own props, and they are
// unexported because nothing outside this file has any business reading them:
// [UseContext] is the supported way to get at a provided value.
const (
	// contextIdentityKey holds the *contextID of the context being provided.
	contextIdentityKey = "react:context.id"
	// contextValueKey holds the provided value, boxed as any.
	contextValueKey = "react:context.value"
	// contextRenderKey holds the render-prop thunk of a [Context.Consumer]
	// element. It is a func() Node that already closes over the typed render
	// function, so the non-generic consumer component can invoke it blindly.
	contextRenderKey = "react:context.render"
)

// contextSeq numbers contexts so every [contextID] is a distinct, non-zero-size
// allocation with a distinct address. It is atomic only because contexts may be
// created from package-level var initializers in different files concurrently
// under a parallel test binary; creation is otherwise not a hot path.
var contextSeq atomic.Uint64

// contextID is a context's runtime identity — the thing a provider fiber stores
// and [UseContext] compares against while walking up the tree. It survives
// across renders because it is allocated once, in [CreateContext], and the same
// pointer is written into every provider element that context ever builds.
//
// The seq field exists solely to make the struct non-empty; see the file
// comment on zero-size allocations.
type contextID struct{ seq uint64 }

// Context is a value published to a subtree, the Go port of React's
// `React.Context`. Unlike React's, it is generic: the value's type is part of
// the context's type, so [UseContext] returns a T rather than an `any` the
// caller must assert. Create one with [CreateContext] — the zero Context is not
// usable, because a context's identity is allocated at creation time.
//
// A Context is immutable and safe to share; it holds no per-render state. The
// values it carries live on provider fibers in the tree, which is what makes
// two [Root]s rendering the same context completely independent.
type Context[T any] struct {
	// id is the identity a provider stores in its props and [UseContext]
	// matches against. Two contexts created with identical default values are
	// still distinct contexts, exactly as in React, because identity is this
	// pointer and never the value.
	id *contextID

	// defaultValue is returned by [UseContext] when no matching provider is
	// found above the reading component. Note React's rule, which this follows:
	// the default applies only when there is *no* provider, not when a provider
	// supplies the zero value.
	defaultValue T
}

// CreateContext returns a new context whose [UseContext] result is
// defaultValue wherever no provider is present above the reader.
//
// It differs from React's `createContext` in shape rather than behavior: React
// returns an object with `.Provider` and `.Consumer` element types, while here
// they are methods that build elements, because Go has no JSX and an element
// type would have to be generic — which the runtime cannot support (see the
// file comment). Call it once, typically into a package-level var; creating a
// context inside a component body would allocate a fresh identity every render
// and silently orphan every consumer.
func CreateContext[T any](defaultValue T) *Context[T] {
	return &Context[T]{
		id:           &contextID{seq: contextSeq.Add(1)},
		defaultValue: defaultValue,
	}
}

// Provider returns an element that publishes value to everything rendered
// beneath it. It is the equivalent of `<Ctx.Provider value={v}>…</Ctx.Provider>`,
// with children passed variadically because Go has no child syntax.
//
// The provider renders transparently: it contributes no output of its own, only
// its children, so wrapping a subtree in a provider never changes the shape of
// the rendered tree. Providers nest, and the nearest one above a reader wins.
//
// Calling it with no children is legal and renders nothing.
func (c *Context[T]) Provider(value T, children ...Node) *Element {
	return &Element{
		Type: contextProviderTag{},
		Props: Props{
			contextIdentityKey: c.id,
			// Boxed as any deliberately: the props map is untyped, and this is
			// the value [UseContext] type-asserts back to T.
			contextValueKey: any(value),
			ChildrenKey:     children,
		},
	}
}

// Consumer returns an element that calls render with the current context value
// and renders whatever it returns — React's render-prop consumer,
// `<Ctx.Consumer>{v => …}</Ctx.Consumer>`.
//
// In React 19 this form is legacy: `useContext` is the recommended way to read
// a context. It is kept here for the same reason React keeps it — it lets a
// caller read a context at a point in the tree where there is no component to
// hang a hook on. The value it receives is by construction exactly what
// [UseContext] would return at the same position, because it calls it.
//
// Note the Go-specific detail: the consumer's element type is a single
// package-level [Component] value, not a per-call closure. A per-call closure
// would be a new function value each render, and while the reconciler matches
// components by code pointer (so the fiber would still be reused), it does not
// overwrite a reused fiber's type — the stale closure, capturing the stale
// render function, would keep being called. Routing the render function through
// props instead means it is refreshed on every render, as props always are.
func (c *Context[T]) Consumer(render func(value T) Node) *Element {
	return &Element{
		Type: contextConsumerComponent,
		Props: Props{
			// The thunk is created per call and captures the typed render
			// function and this context; it is invoked from the consumer
			// component's body, so [UseContext] resolves against the consumer's
			// own fiber.
			contextRenderKey: func() Node { return render(UseContext(c)) },
		},
	}
}

// contextConsumerComponent is the one and only component behind every
// [Context.Consumer] element, for every context and value type. Keeping it a
// single package-level value means every consumer element has the same
// [Element.Type] identity, so consumers reconcile against each other by
// position like any other component — and the varying part, the render thunk,
// travels in props where a re-render can replace it.
var contextConsumerComponent Component = func(p Props) Node {
	fn, ok := p[contextRenderKey].(func() Node)
	if !ok {
		panic(fmt.Sprintf("react: context consumer element is missing its render "+
			"function (props[%q] = %T); build consumers with Context.Consumer, "+
			"never by hand", contextRenderKey, p[contextRenderKey]))
	}
	return fn()
}

// UseContext returns the value provided by the nearest [Context.Provider] for c
// above the calling component, or c's default value when there is none. It is
// React's `useContext`, and like every hook it may only be called from a
// component body.
//
// It is not really a hook in the [nextHook] sense — it consumes no hook slot,
// so unlike UseState it may legally be called conditionally. It is named
// UseContext anyway, because it shares the constraint that matters: it needs a
// component fiber to read from, and calling it outside a render panics with the
// runtime's standard hook diagnostic.
//
// Re-rendering is not this function's job. The runtime re-renders from the root
// on every pass, so a changed provider value reaches every reader below it
// automatically; there is no per-context subscription list as in React's
// implementation.
func UseContext[T any](c *Context[T]) T {
	// Ask for the fiber first, so that calling this outside a render produces
	// the runtime's hook diagnostic rather than a nil-context complaint.
	f := currentFiber()

	if c == nil {
		panic("react: UseContext was given a nil context; contexts must come " +
			"from CreateContext")
	}

	provider := findAncestor(f, func(a *fiber) bool {
		if _, ok := a.typ.(contextProviderTag); !ok {
			return false
		}
		// Identity comparison, not value comparison: two contexts of the same T
		// are distinct, and the same context provided twice at different depths
		// both match — findAncestor stops at the first, which is the nearest.
		return a.props[contextIdentityKey] == c.id
	})
	if provider == nil {
		return c.defaultValue
	}

	raw := provider.props[contextValueKey]
	if raw == nil {
		// Only reachable when T is an interface type and a nil interface was
		// provided: a nil pointer or nil map stored in an `any` is still a
		// non-nil interface. The zero T is the right answer, and a type
		// assertion would wrongly report a mismatch.
		var zero T
		return zero
	}

	v, ok := raw.(T)
	if !ok {
		var zero T
		panic(fmt.Sprintf("react: context value has Go type %T but this context "+
			"carries %T; a provider element was built with a mismatched value",
			raw, zero))
	}
	return v
}

// contextRenderProvider renders a provider fiber. There is nothing to compute:
// a provider is a pure carrier, so it reconciles its children and produces no
// output of its own. The value it publishes is already sitting in its props,
// where [UseContext] finds it by walking up from the reader.
//
// Reconciling with the children unconditionally — even an empty list — is what
// the [RegisterElementRenderer] contract asks for, so that a provider whose
// children disappear tears the old subtree down.
func contextRenderProvider(f *fiber) {
	reconcileChildren(f, f.props.Children())
}

func init() {
	// Registered exactly once, for the one non-generic tag type. See the file
	// comment for why there is no per-T registration.
	RegisterElementRenderer(contextProviderTag{}, contextRenderProvider)
}
