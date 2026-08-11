// Package react is a Go port of React 19 — the component, props and hooks
// programming model, running on Go's runtime and rendering to a string rather
// than to a browser. Components are ordinary functions, state lives in hooks,
// and a reconciler diffs the tree between renders so that state survives where
// the shape of the output does not change:
//
//	var Greeting react.Component = func(p react.Props) react.Node {
//		name, setName := react.UseState(p.String("name"))
//		_ = setName
//		return react.H("h1", nil, "Hello, ", name, "!")
//	}
//
//	el := react.CreateElement(Greeting, react.Props{"name": "Ada"})
//	html, _ := react.RenderToString(el) // <h1>Hello, Ada!</h1>
//
// Reach for it when you want React's mental model — declarative trees,
// composition by function call, local state that outlives a re-render — for
// server-side HTML generation, static site output, email rendering, or any
// program that repeatedly derives a tree from state. The package is
// standard-library only.
//
// # Building trees without JSX
//
// This is the single biggest difference from React, so it comes first. In
// JavaScript, `<div id="x">hi</div>` is sugar that a compiler rewrites into
// `React.createElement("div", {id: "x"}, "hi")`. Go has no JSX and no compiler
// hook that could add one, so the rewritten form *is* the authoring surface
// here. [CreateElement] is the general constructor; [H] fixes the type to a tag
// string and is the workhorse for markup; [Frag] groups siblings with no
// wrapper. Trees are therefore nested function calls, and read best when the
// indentation is allowed to carry the structure:
//
//	react.H("ul", react.Props{"class": "menu"},
//		react.H("li", react.Props{react.KeyProp: 1}, "first"),
//		react.H("li", react.Props{react.KeyProp: 2}, "second"),
//	)
//
// Props are a [Props] map rather than a struct, because a component's accepted
// props are open-ended and host elements take arbitrary HTML attributes. Two
// keys are reserved exactly as in React: "children" holds the child nodes, and
// "key" and "ref" are lifted out of the map into [Element.Key] and
// [Element.Ref] by [CreateElement] and never reach a component. Read props
// through the accessors — [Props.Get], [Props.Has], [Props.String],
// [Props.Bool], [Props.Children] — which are nil-safe, and never mutate the map
// a component was handed: the runtime reuses props across bailouts, so a
// mutation is visible to the previous render and breaks memoization.
//
// Children are a [Node], the port's stand-in for React's ReactNode union: nil,
// booleans (both true and false render as nothing, which is what
// `cond && node` means), strings, any integer or float kind, an [*Element], and
// slices of any of those, spliced in place. A value of any other type panics at
// render time naming the offending Go type, rather than silently rendering a
// formatted struct.
//
// # Components
//
// A [Component] is a plain func(Props) Node. That is the only kind of component
// the port supports — React 19 removed class components from the recommended
// surface, and Go has no natural analogue for `this.setState`, so hooks are the
// single state mechanism. One Go detail bites here: [Component] is a *named*
// function type, and the runtime dispatches on it by type switch, so a bare
// func literal must be given that type before it can be used as an element
// type. Declare components as `var Foo react.Component = func(p react.Props)
// react.Node { … }`, or convert at the call site with `react.Component(foo)`.
//
// A component must be a pure function of its props, its hook state and the
// contexts it reads. The runtime calls it more than once per visible update, so
// side effects belong in [UseEffect], never in the body.
//
// # Hooks and why order matters
//
// Hooks are package-level functions with no receiver, so they find "the
// component being rendered right now" through an ambient pointer that the
// runtime sets before it calls a component — React calls this the dispatcher.
// Each fiber keeps a slice of hook slots, and each hook call takes the next slot
// by index. The index, and nothing else, is a hook's identity. That is why the
// Rules of Hooks are a real constraint rather than a style rule: a hook behind
// an `if` shifts every later hook onto the wrong slot. The port checks for this
// rather than letting it corrupt state silently — each slot records which kind
// of hook created it, and a mismatch panics with a message naming both hooks and
// the slot number.
//
// The state hooks are [UseState], [UseStateLazy], [UseReducer] and
// [UseReducerInit]. [UseState] returns a [SetStateFunc], which is callable like
// React's setter and additionally carries an [SetStateFunc.Update] method for
// the functional form — Go cannot overload one parameter to accept both a value
// and an updater the way JavaScript does, so the two forms are two entry points.
//
// Effects are [UseEffect] and [UseEffectFn] (passive), [UseLayoutEffect] and
// [UseLayoutEffectFn] (synchronous, before any passive effect) and
// [UseInsertionEffect]. The `Fn` variants take a function with no cleanup, which
// is the common case and avoids writing `return nil`. Dependency lists follow
// React's rules exactly: a nil list means "run after every render", an empty
// non-nil list means "run once on mount", and a populated list re-runs when any
// entry changes under [ObjectsEqual], the port's Object.is. Effects never run
// during render; they are queued and drained after the tree commits.
//
// Memoization and identity are [UseMemo], [UseCallback], [UseRef] (returning a
// [Ref] with a .Current field), [UseImperativeHandle], [UseDebugValue] and
// [UseId]. Context is [CreateContext], [Context.Provider], [Context.Consumer]
// and [UseContext]; the context type is generic, so [UseContext] hands back a T
// rather than an `any` to assert. [Memo] and [MemoWith] wrap a whole component
// so that a re-render with equal props is skipped — note that they return a
// [MemoType] to use as an element type, not a [Component].
//
// # Rendering, updates and batching
//
// [NewRoot] mounts a tree and renders it; [Root.Render] replaces the element and
// re-renders; [Root.Unmount] tears it down, running every effect cleanup from
// the leaves upward. A [Root] is the unit of state — the same element rendered
// into two Roots has two independent sets of hooks.
//
// Every render pass walks from the root down. The port does not restart work at
// the fiber that changed, because reconciliation preserves fibers either way and
// the simpler traversal is far easier to reason about; [Memo] is where a subtree
// re-render is actually skipped. Reconciliation matches a new child to an old
// fiber — and therefore to its hooks and its state — when both element type and
// key agree. Keyed children are matched by key across the whole sibling list, so
// reordering a keyed list moves state with the items; keyless children match by
// position, which is why inserting at the front of a keyless list shifts
// everyone's state by one.
//
// There is no scheduler. React 19 assigns lanes and priorities and can interrupt
// a render; here a state update marks the tree dirty and a render loop runs
// passes until it settles, synchronously, on the calling goroutine. A component
// that updates state unconditionally in its own body would loop forever, so the
// loop panics after a bounded number of passes with a diagnostic rather than
// hanging. [UseTransition], [StartTransition] and [UseDeferredValue] are present
// and keep their API shape, but without a scheduler behind them they cannot
// deprioritise work the way React does — see API-DEVIATIONS.md.
//
// Batching is likewise explicit. React 19 batches automatically around events
// and effects; Go has no event loop to hang that off, so several updates are
// coalesced into one render by wrapping them in [Batch]. [Batch] calls nest and
// only the outermost one flushes. [Root.Flush] settles pending work and effects
// on demand, and [Act] is the test helper that runs a function and then settles
// every tree it touched, the way React's `act` does.
//
// Rendering is serialized process-wide by a package mutex. The dispatcher hooks
// read is a single pointer, so concurrent renders would interleave and hand
// hooks the wrong fiber; the mutex makes that impossible rather than merely
// undocumented, at the cost of disallowing genuinely parallel renders of
// independent roots. Root methods are safe to call from any goroutine — a state
// update from a background goroutine blocks until any in-flight render finishes.
//
// # Suspense, errors and async
//
// Rendering is recursive rather than a work loop, which makes Go's panic/recover
// the natural mechanism for the two features that need to abort a subtree.
// [Suspend] panics a [SuspendSignal] carrying a channel; the nearest [Suspense]
// boundary above recovers it, renders its fallback, and re-renders the subtree
// once the channel closes. [ErrorBoundary] recovers an ordinary panic from its
// subtree and renders a fallback from it. A [SuspendSignal] that reaches the
// root with no boundary to catch it is re-panicked, matching React's "a
// component suspended while responding to synchronous input" failure.
//
// The resource helpers sit on top of that: [Async] is a value being computed,
// started with [Go] or created already-settled with [Resolved] and [Failed], and
// [Use] reads one — returning the value if it is ready, suspending if it is not,
// and panicking its error if it failed. [Cache] and [NewCache] turn a
// `func(K) (V, error)` into a keyed, single-flight loader returning the same
// [Async] for the same key, so a component that renders repeatedly does not
// restart the work every pass. [Lazy] is the same idea applied to a component.
//
// Because Suspense is implemented with panic, a component that recovers panics
// indiscriminately will swallow suspension and error propagation. Recover
// selectively, or not at all, inside a component body.
//
// # Output
//
// There is no DOM here, so a rendered tree is not an end in itself. The primary
// output path is server rendering. [RenderToWriter] mounts a [Node], streams its
// HTML to an io.Writer and unmounts it again; [RenderToString] is the same thing
// accumulated into a string, [RenderToStaticMarkup] renders output that will
// never be hydrated, and [MustRenderToString] is the panicking convenience for
// templates and tests. All four take a [Node] and manage the [Root] themselves —
// build a Root by hand only when the tree needs to outlive one render.
//
// RenderToString and RenderToStaticMarkup are not aliases. They differ exactly
// as React's two functions differ where the difference reaches the bytes: the
// hydration separator between two adjacent text nodes. RenderToString of
// H("div", nil, "a", "b") is "<div>a<!-- -->b</div>", because "ab" could have
// come from one text node or from two and a hydrating client cannot guess;
// RenderToStaticMarkup of the same tree is "<div>ab</div>". RenderToWriter
// renders in RenderToString mode. Nothing else about hydration exists here —
// no boundary comments, no bootstrap scripts, no client to consume them.
//
// Document metadata is hoisted, as React 19's Float does it: a title, meta,
// link or async script src written inside the body is lifted to the front of the
// document, into the head when the tree has one and into a synthesized head when
// an html element lacks one. A tree that can hoist is buffered rather than
// streamed, because the last element of a document can still belong at its
// front; a tree with no document machinery in it streams as before. Several
// things that look hoistable are not — see API-DEVIATIONS.md.
//
// Attributes are emitted in sorted prop-name order. React follows the props
// object's insertion order, which a Go map has none of; sorting is what makes
// one tree render to the same bytes twice. React's own hand-written orderings
// for input, button, form and option are reproduced on top of that pass. See
// API-DEVIATIONS.md.
//
// Text and attribute values are HTML-escaped on the way out, void elements are
// emitted without a closing tag, React's prop-to-attribute names are mapped
// ([AttributeName]), style maps are serialised with React's camelCase and
// unitless-property rules ([FormatStyle]), and raw HTML is opt-in through the
// [DangerousHTML] wrapper. A structurally invalid tree — children on a void
// element, raw HTML alongside children — is an error rather than malformed
// output, and a panic during rendering is recovered and returned as an error, so
// a bad template fails a request rather than the process.
//
// For inspecting a tree rather than serialising it, [Root.Tree] exposes an
// [Instance] view — the port's stand-in for the DOM node a test would otherwise
// query, with Find/FindAll/FindByTag/TextContent — which is what assertions in
// tests are written against.
//
// # Parity
//
// The element model, the hook set, keyed reconciliation, context, memoization,
// Suspense and error boundaries, and the server renderers are ported. JSX, class
// components, the DOM renderer and its synthetic event system, the concurrent
// scheduler and lane-based priority, streaming and selective hydration, React
// Server Components and the DevTools protocol are not. Every deliberate
// difference in the surface that *is* ported — generics instead of dynamic
// typing, the setter's Update method, explicit [Batch], panic/recover instead of
// throw, [Instance] instead of a DOM node — is itemised with its reason in
// API-DEVIATIONS.md.
package react
