# API deviations

This port mirrors the API of [React 19](https://github.com/facebook/react) on
purpose: names, hook semantics, dependency rules and reconciliation behavior are
chosen to match the original. Where behavior or shape differs, it is listed here
with the reason.

Nothing in this file is a bug report. These are deliberate choices, and each one
is either forced by the language or by the absence of a browser.

## Authoring: no JSX

| Upstream | Here | Why |
| --- | --- | --- |
| `<div id="x">hi</div>` | `H("div", Props{"id": "x"}, "hi")` | Go has no JSX and no compiler hook that could add one. JSX is sugar over `createElement`; the desugared form is the authoring surface. |
| `<Comp {...props} />` | `CreateElement(Comp, props)` | Same reason. There is no spread syntax for a props map. |
| `<>…</>` | `Frag(a, b, c)` | Same reason. |
| children as nested syntax | children as variadic `...Node` | Go has no child syntax. Variadic arguments are the closest readable equivalent. |

Practical consequence: a Go tree becomes hard to read at a shallower nesting
depth than markup does. Factor sub-trees into helper functions earlier than you
would in JSX.

## Components

| Upstream | Here | Why |
| --- | --- | --- |
| A component is any function (or, historically, a class) | A component is the named type `Component func(Props) Node` | The runtime dispatches on element type with a Go type switch. A bare `func(Props) Node` is a *different* type and will not match, so components must be declared as `var Foo react.Component = …` or converted with `react.Component(foo)`. |
| Class components, `this.setState`, lifecycle methods, `componentDidCatch` | Not ported | React 19 removed class components from the recommended surface, and Go has no analogue for `this.setState`. Hooks are the single state mechanism; `ErrorBoundary` covers the one job classes were still required for. |
| `props` is an object; TypeScript types it per component | `Props` is `map[string]any` | A component's accepted props are open-ended and host elements take arbitrary HTML attributes. A per-component struct would need generics on the element type, which the renderer's type registry cannot express. Accessors (`Get`/`Has`/`String`/`Bool`/`Children`) recover some safety. |
| `forwardRef` | Not needed; refs are ordinary props | React 19 already passes `ref` as a prop to function components. `Element.Ref` is retained because host elements still need somewhere to put one. |
| `defaultProps` | Not ported | Deprecated upstream for function components. Default a missing prop in the component body. |
| `propTypes` | Not ported | A runtime type-checking layer for a language that has none. Go has one. |

## Elements and children

| Upstream | Here | Why |
| --- | --- | --- |
| `null` / `undefined` / `false` render as nothing; `true` also does | `nil`, `false` **and** `true` render as nothing | Go has no distinct `undefined`. Rendering the literal text `true` is never what a `cond && node` expression means. |
| A non-renderable child is coerced or throws in dev | Panics naming the offending Go type | A struct silently rendering as `{0xc000…}` is worse than a loud failure at the call site. |
| `key` may be any value | `key` is lifted to `Element.Key`, a string | Keys are compared, not stored typed. Numeric and `fmt.Stringer` keys are formatted with the same rules a text child uses, so an int key stringifies the way JavaScript would rather than through `%v`. A nil key means unkeyed. |
| `React.Children.only` throws | `ChildrenOnly` returns `(*Element, error)` | Go reports recoverable misuse with an error. Panicking would be the un-Go-like choice for a condition the caller can reasonably handle. |
| Number formatting via JS `String()` | `strconv.FormatFloat(v, 'f', -1, 64)` for floats | Produces the shortest round-tripping representation, matching JS output for the common cases rather than Go's default `%v`. |

## Hooks

| Upstream | Here | Why |
| --- | --- | --- |
| `const [n, setN] = useState(0)`; `setN(v)` **or** `setN(fn)` | `n, setN := UseState(0)`; `setN(v)` **or** `setN.Update(fn)` | Go cannot overload one parameter to accept both a `T` and a `func(T) T` — with `T = func(...)` the two are indistinguishable. `SetStateFunc[T]` is a named func type carrying an `Update` method, so both forms exist without ambiguity. |
| `useState(() => expensive())` — a function initializer is detected | `UseStateLazy(func() T)` is a separate hook | Same ambiguity: a `T` that happens to be a function cannot be told apart from an initializer. |
| `useReducer(r, init)` / `useReducer(r, arg, initFn)` | `UseReducer` / `UseReducerInit` | Same reason — the third-argument overload becomes a second function. |
| `useEffect(() => { … })` with an optional returned cleanup | `UseEffect(func() func(), deps)` and `UseEffectFn(func(), deps)` | JS lets an effect return `undefined`; Go would force `return nil` on every cleanup-free effect. The `Fn` variants remove that noise. `UseLayoutEffect`/`UseLayoutEffectFn` pair the same way. |
| `useRef(v).current` | `UseRef[T](v).Current` | Exported fields are capitalized in Go. |
| `useContext(Ctx)` returns whatever was provided | `UseContext(ctx)` returns `T` | The context is generic, so no assertion is needed at the call site. |
| `useId()` returns an opaque `:r0:`-style string | `UseId()` returns a string derived from the fiber path and hook index | The value is stable and unique per component instance, which is the contract. The exact format is not upstream's and should not be depended on. |
| `useDebugValue(v, fmt?)` | `UseDebugValue(label string)` and `UseDebugValueFn(func() string)` | There is no DevTools protocol to report into, so values are collected as strings and readable through `DebugValues(root)`. The lazy-formatter overload becomes a second function. |
| `useImperativeHandle(ref, create, deps)` | Same, with `ref *Ref[T]` and `create func() T` | Typed rather than dynamic; otherwise identical. |
| `useSyncExternalStore(sub, get, getServer?)` | `UseSyncExternalStore(sub, get)` | The server-snapshot argument exists upstream to resolve hydration mismatches. There is no client to hydrate against. |
| `useDeferredValue`, `useTransition`, `startTransition` | Present, same shape | See *Scheduling* below — the shape is preserved, the priority behavior is not. |

**Hook order is enforced, not merely documented.** A hook's identity is its slot
index within its fiber, exactly as upstream. Unlike upstream, each slot records
the *kind* of hook that created it, and a mismatch on a later render panics
naming the slot number, the previous hook and the new one. React's dev-mode
warning is easy to miss; a panic is not.

**Dependency comparison.** `ObjectsEqual` is the port's `Object.is`. Go's `==`
panics on slices, maps and functions, which a dependency list can easily contain,
so those are compared by pointer identity where possible and otherwise reported
as **unequal**. That is the conservative direction: a false "changed" costs one
re-render, a false "same" skips one that was needed. `NaN` is not special-cased
the way `Object.is` special-cases it.

## Scheduling, batching and concurrency

| Upstream | Here | Why |
| --- | --- | --- |
| Lane-based concurrent scheduler; renders can be interrupted, time-sliced and replayed at a different priority | No scheduler. A state update marks the tree dirty; a synchronous loop runs passes until it settles | Time slicing exists to keep a browser's main thread responsive. There is no main thread to yield to here, and a goroutine that blocks blocks only itself. |
| `useTransition` / `startTransition` mark an update low-priority | Present and API-compatible; the update is not actually deprioritised | Keeping the shape means code reads the same and can be ported back. Claiming the behavior would be a lie. `isPending` reflects the transition's own in-flight state. |
| `useDeferredValue` returns a lagging value while a render is in flight | Present and API-compatible; without priority lanes it cannot lag meaningfully | Same reason. |
| Automatic batching around events, effects and `setTimeout` | Explicit `Batch(fn)` | React batches automatically because it owns the event loop that dispatches your handlers. Go has no such loop. Outside a `Batch`, each update flushes; inside, updates coalesce into one render per affected root, and nested `Batch` calls flush only at the outermost. |
| Rendering is single-threaded by virtue of JavaScript | Rendering is serialized process-wide by a mutex | The hook dispatcher is a single ambient pointer, so two concurrent renders would interleave and hand hooks the wrong fiber. Independent roots therefore cannot render in parallel — a real cost, accepted in exchange for making an undiagnosable bug class impossible. |
| Render restarts at the fiber that changed | Every pass re-renders from the root | Reconciliation preserves fibers either way, so state and effects behave identically. `Memo` is where work is actually skipped. |
| "Too many re-renders" error | Same, as a panic, after a bounded number of passes | Identical intent; a panic carries the diagnostic naming the likely cause. |
| `act()` from `react-dom/test-utils` | `Act(root, fn)` | Same job — run work and drain the effects it scheduled — without a testing-library dependency. |

## Output: no DOM

| Upstream | Here | Why |
| --- | --- | --- |
| `createRoot(container).render(el)` mounts into a DOM node | `NewRoot(node)` returns a `*Root` holding a rendered tree | There is no container. A `Root` is the unit of state. |
| The rendered result is the DOM | The rendered result is HTML from `RenderToString` / `RenderToStaticMarkup` / `RenderToWriter` / `MustRenderToString` | Go has no DOM, so server rendering is the primary output path rather than an add-on. |
| `renderToString(element)` is separate from `createRoot` | Same: the render functions take a **`Node`**, not a `*Root`, and mount, walk and unmount their own `Root` | Matches upstream's shape, and means the common case — render a page and be done — never has to think about the `Root` lifecycle. Build a `Root` explicitly only when the tree must survive across renders. |
| `renderToString` embeds hydration markers; `renderToStaticMarkup` does not | The two produce **identical** output; `RenderToStaticMarkup` is an alias | There is no client to hydrate. Emitting markers nothing consumes would put noise in the output and imply a capability that does not exist. Both names are kept because they carry intent, and are the natural place for markers to land if hydration is ever added. |
| A render error throws | A render error is returned; a panic during rendering is recovered and returned as an error | A template rendering deep inside a request handler should fail that request, not the process. An I/O failure is the writer's own error, unwrappable; a tree problem is a `react:`-prefixed error naming the element at fault. |
| Tests query DOM nodes (`container.querySelector`) | `Root.Tree()` returns an `Instance` view, with `Find`, `FindAll`, `FindByTag`, `FindByProp`, `TextContent` | An `Instance` is this port's stand-in for the node a browser test would assert against. |
| `act(async () => …)` awaits | `Act(fn)` takes no root and settles synchronous work only | `Act` is `Batch` with a purpose-built name; it cannot know about a goroutine `fn` started. Wait for that goroutine with a channel, then call `Act` again. Note that the tree does not change *during* `fn` — read it after `Act` returns. |
| Synthetic event system: `onClick` and friends fire | Handler props are inert data | There is nothing to dispatch an event. Props named `onClick` are carried but never invoked by the runtime. |
| `hydrateRoot`, streaming SSR, selective hydration, `renderToPipeableStream` | Not ported | All are client/server coordination features. `RenderToWriter` streams bytes but does not implement React's streaming protocol or out-of-order shell flushing. |
| React Server Components, `use server`, server actions | Not ported | A build-and-transport protocol with no Go counterpart. `UseActionState` and `FormData` are ported as ordinary values, not as an RSC integration. |
| React DevTools | Not ported; `UseDebugValue` values are read through `DebugValues(root)` | There is no browser extension protocol to speak. |

## Context

| Upstream | Here | Why |
| --- | --- | --- |
| `createContext(v)` returns an object with `.Provider` / `.Consumer` **element types** | `CreateContext[T](v)` returns a `*Context[T]` whose `Provider` and `Consumer` are **methods that build elements** | An element type has to be storable in the renderer's `map[reflect.Type]` registry. A generic element type cannot be, and a per-instantiation type would fragment the registry. |
| The provider element type is per-context | A single non-generic sentinel type is shared by every context and every value type | Follows from the above. Context identity and the boxed value ride in the element's props under reserved, namespaced keys; `Context[T]` stays generic at the API surface, so `UseContext` still returns a `T`. |
| Consumers are matched by context object identity | Matched by a pointer allocated in `CreateContext` | Same semantics: two contexts with identical default values are still distinct contexts. Create a context once, into a package-level var — creating one inside a component body allocates a fresh identity every render and orphans every consumer. |
| `Context.Consumer` is legacy but supported | Same, and implemented by routing the render function through props | The consumer's element type is one package-level `Component`, not a per-call closure: the reconciler matches components by code pointer and does not overwrite a reused fiber's type, so a per-call closure would go stale. Passing the thunk as a prop refreshes it every render, as props always do. |
| `Context` also works as a value with `use(Context)` | Read contexts with `UseContext` | `Use` here is the async-resource reader; overloading it on an untyped argument would lose the generic return type. |

## Suspense, errors and async

| Upstream | Here | Why |
| --- | --- | --- |
| A suspending component **throws a promise** | `Suspend(ready)` **panics** a `SuspendSignal` carrying a `<-chan struct{}` | Go has no exceptions and no promises. Panic is the only construct that unwinds an arbitrary depth of calls, and a channel is the idiomatic "this will be ready later". Rendering is a recursive descent precisely so `recover` can catch it at a boundary. |
| Errors are caught by a class component's `componentDidCatch` | `ErrorBoundary(fallback func(err any) Node, children ...Node)` recovers a panic from its subtree | Same mechanism, same reason. The fallback is a function of the recovered value rather than a node, because Go has no `getDerivedStateFromError` to thread it through. |
| A boundary stays in its error state until something resets it | The boundary re-attempts its children on **every** render, and recovers on its own as soon as they stop panicking | Upstream's stickiness is a consequence of the error living in class state. With hooks there is no such state to latch, and re-attempting costs one extra recover per render and never loops (catching an error schedules no update). For sticky behavior, stop rendering the failing subtree or change the boundary's `Key` to force a remount. |
| A Suspense boundary also surfaces errors in some paths | `Suspense` re-panics anything that is not a `SuspendSignal` | Keeping the two boundaries strictly disjoint makes which one handles what a property of the type, not of ordering. |
| An uncaught thrown promise is a React error | An uncaught `SuspendSignal` is re-panicked | Matches "a component suspended while responding to synchronous input". |
| `use(promise)` | `Use(*Async[T])` — with `Go`, `Resolved` and `Failed` to build the value | Go has no promise type. `Async[T]` is the port's equivalent; `Resolved`/`Failed` build already-settled values, which makes suspense behavior testable without real concurrency. `Async` also exposes non-suspending readers (`Done`, `Result`, `TryResult`, `Ready`) for code outside a component. |
| `use(Context)` also reads a context | `Use` reads only an `*Async[T]` | Overloading it on an untyped argument would lose the generic return type. Read contexts with `UseContext`. |
| `cache(fn)` — one argument in, memoized | `Cache(fn func(K) (V, error)) func(K) *Async[V]`, plus `NewCache` returning a `*Cached[K, V]` | The `*Async` return is what makes the result usable with `Use` and shareable between components suspending on the same key. `Cache` matches upstream's shape and, like upstream, grows forever; `NewCache` keeps the handle so `Invalidate`, `Clear`, `Peek` and `Len` stay reachable — which in a long-running Go process is almost always what you want, whereas React can rely on a per-request cache lifetime. |
| `React.lazy(() => import(...))` | `Lazy(load func() (Component, error))` returns a `*LazyType` element type | Go has no dynamic import; the loader is an ordinary function. It is called at most once, shared across every `Root` and every Suspense retry, and a `(nil, nil)` return is reported as an error rather than rendering nothing. |
| Error boundaries do not catch errors in event handlers | There are no event handlers to catch errors in | Follows from having no event system. |

**Consequence worth calling out:** because both features ride on panic, a
component that recovers panics indiscriminately will swallow suspension *and*
error propagation. Recover selectively, or not at all, inside a component body.

## Memoization

| Upstream | Here | Why |
| --- | --- | --- |
| `React.memo(C, areEqual?)` | `Memo(C)` and `MemoWith(C, eq)`, both returning a `*MemoType` | The optional second argument becomes a second function; Go has no optional parameters. `ShallowEqualProps` is exported as the default comparison so a custom one can build on it. A `*MemoType` is an **element type**, not a `Component` — build it once at package level, because a fresh wrapper each render is a new element type and would remount the very subtree the memoization was protecting. |
| `useMemo` / `useCallback` are advisory — React may discard the cache | The cache is not discarded | There is no memory-pressure heuristic to drive eviction, and a render is cheap enough here that one is not warranted. Do not rely on either behavior for correctness, in this port or upstream. |
| `useCallback(fn, deps)` returns the same function type | `UseCallback[F any](fn F, deps []any) F` | Generic over the function type so the returned value is directly callable rather than needing an assertion. |
| `<Profiler onRender={...}>` reports commit timings | `Profiler` wraps a subtree and reports what the port can observe | Without a commit phase against a DOM, several of upstream's timing fields have no meaning here. |
| `StrictMode` double-invokes components and effects in development | `StrictMode` wraps a subtree | Present for structural parity. Do not assume the double-invocation side-effect detection unless the code says otherwise. |

## Additions with no upstream counterpart

Added where Go's lack of an ambient runtime left an obvious hole. Each is
additive — nothing in React's surface changes shape because of it.

| Addition | What it is for |
| --- | --- |
| `DangerousHTML` | Raw HTML is a wrapper type rather than a bare string in a props map, so it cannot be reached by accident. Upstream's `dangerouslySetInnerHTML: {__html: s}` object has the same intent; a Go type enforces it. |
| `AttributeName`, `FormatStyle` | The prop-name and style-value mappings the emitter uses, exported so a custom renderer can reuse them instead of re-deriving React's rules. |
| `Store[T]`, `NewStore` | A ready-made implementation of the `UseSyncExternalStore` contract. React leaves this to Redux/Zustand; there is no such ecosystem here yet. |
| `Cached[K, V]`, `NewCache` | `Cache` with the handle kept, so `Invalidate` / `Clear` / `Peek` / `Len` stay reachable in a process that outlives a request. |
| `UseDeferredValueInitial` | React's `useDeferredValue(value, initialValue)` second argument, as a separate function — Go has no optional parameters. |
| `UseDebugValueFn`, `DebugValues` | The lazy-formatter overload, plus a way to read collected values without DevTools. |
| `Instance` and its query methods | The DOM-node stand-in tests assert against. |
| `RegisterElementRenderer` | The extension point that keeps the core dispatch from growing a case per feature. Exported so a downstream package can add an element type; registering a type twice panics. |
| `ObjectsEqual`, `DepsChanged` | React's internal `Object.is` and dependency comparison, exported because a custom hook built on the same primitives needs the same semantics. |
| `Suspend`, `SuspendSignal` | The suspension mechanism itself, exported so a custom async primitive can suspend without going through `Async`. |
| `TextValue`, `IsText`, `IsFragment`, `HostTag`, `NormalizeChildren` | Element introspection a renderer needs and JSX users never do. |

## Things that behave the same, and are easy to assume otherwise

- **Keyless children match by index.** Inserting at the front of a keyless list
  shifts every item's state by one. This is upstream's behavior and is
  reproduced deliberately, not a limitation.
- **Keys only need to be unique among siblings**, not globally.
- **A change of element type remounts**, discarding hook state, even when the
  new type renders identical output.
- **Effect ordering**: layout effects run before any passive effect, both in
  tree order; each effect's previous cleanup runs immediately before its new
  body; unmount cleanups run leaves-first.
- **Deps semantics**: `nil` means every render, empty non-nil means mount only.
- **Providers render transparently** — wrapping a subtree in a provider never
  changes the shape of the output.
- **A render-phase state update** is not recursion; it settles by running
  another pass.
- **Props must not be mutated.** The runtime reuses props across bailouts, so a
  mutation is visible to the previous render and breaks memoization. Use
  `Props.Clone` to forward props with one key changed.
