# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-08-05
### Added
- Initial public release — React 19's component, props and hooks model for Go,
  rendering to HTML rather than to a DOM. Standard library only, Go 1.24+.
- **Elements.** `CreateElement` with React's reserved `key`/`ref` lifting, the
  `H` host shorthand, `Frag`, `CloneElement`, `IsValidElement`, the `Fragment`
  sentinel, and the `React.Children` helpers `ChildrenToSlice`, `ChildrenCount`,
  `ChildrenForEach`, `ChildrenMap` and `ChildrenOnly`.
- **Core types.** `Node` (React's ReactNode union: nil, booleans, strings,
  numbers, `*Element` and slices thereof), `Props` with nil-safe `Get`, `Has`,
  `String`, `Bool`, `Children` and `Clone`, and `Component` as a plain
  `func(Props) Node`.
- **Runtime.** `NewRoot`, `Root.Render`, `Root.Flush`, `Root.Unmount` and
  `Root.Tree`; a fiber reconciler with React's keyed and positional matching
  rules, leaves-first unmount cleanups, and a bounded render loop that panics
  with a diagnostic instead of spinning. `Batch` coalesces updates; `Act` runs a
  function and then settles every tree it touched; `Root.Tree` returns an
  `Instance` — a queryable view of the rendered tree with `Find`, `FindAll`,
  `FindByTag`, `FindByProp` and `TextContent`.
- **State hooks.** `UseState` (returning a `SetStateFunc[T]` with an `.Update`
  method for the functional form), `UseStateLazy`, `UseReducer`,
  `UseReducerInit`.
- **Effect hooks.** `UseEffect`, `UseEffectFn`, `UseLayoutEffect`,
  `UseLayoutEffectFn` and `UseInsertionEffect`, with React's dependency rules
  (nil deps re-run every render, empty deps run on mount only) and React's
  ordering guarantee that layout effects precede passive effects.
- **Memo and identity hooks.** `UseMemo`, `UseCallback`, `UseRef` (a `*Ref[T]`
  with `.Current`), `UseImperativeHandle`, `UseDebugValue` and `UseId`.
- **Context.** Generic `CreateContext[T]` with `Provider` / `Consumer` element
  builders and `UseContext`, so a context read is typed rather than an `any` to
  assert.
- **Components.** `Memo` and `MemoWith` (returning a `*MemoType` element type),
  `ShallowEqualProps`, `StrictMode`, and `Profiler` with `ProfilerEvent` /
  `ProfilerPhase`. Memo boundaries track descendant work, so memoizing can never
  swallow a child's state update.
- **Boundaries.** `Suspense`, `ErrorBoundary` and `Lazy`, built on Go's
  `panic`/`recover` — `Suspend` panics a `SuspendSignal` carrying a readiness
  channel, and the nearest boundary recovers it and renders its fallback. The
  two boundaries are disjoint: `Suspense` re-panics anything that is not a
  suspension, and `ErrorBoundary` re-attempts its children each render rather
  than latching into an error state.
- **Concurrency surface.** `UseTransition`, `StartTransition`,
  `UseDeferredValue`, `UseDeferredValueInitial`, `UseOptimistic`,
  `UseActionState`, `UseFormStatus`, `FormStatus` and `FormData`. The API shape
  matches React 19; there is no scheduler behind it (see `API-DEVIATIONS.md`).
- **Async resources.** `Async[T]` with both suspending (`Use`) and
  non-suspending (`Done`, `Result`, `TryResult`, `Ready`) readers, `Go`,
  `Resolved`, `Failed`, and the keyed single-flight loaders `Cache` and
  `NewCache` / `Cached[K, V]`.
- **External stores.** `UseSyncExternalStore`, plus a ready-made `Store[T]`
  (`NewStore`, `Get`, `Set`, `Update`, `Subscribe`) implementing its contract.
- **Server rendering.** `RenderToWriter`, `RenderToString`,
  `RenderToStaticMarkup` and `MustRenderToString` — the port's primary output
  path. All take a `Node` and manage the `Root` themselves. HTML-escaped text
  and attributes, void elements without a closing tag, React's prop-to-attribute
  name mapping (`AttributeName`), style-map serialization with camelCase and
  unitless-property rules (`FormatStyle`), and opt-in raw HTML via
  `DangerousHTML`. A structurally invalid tree is an error, and a panic during
  rendering is recovered and returned as one, so a bad template fails a request
  rather than the process. `RenderToString` and `RenderToStaticMarkup` produce
  identical output: with no client to hydrate, hydration markers would imply a
  capability that does not exist.
- **Diagnostics.** A changed hook order panics naming both hooks and the slot; a
  hook called outside a render panics with React's own message; an unrenderable
  child panics naming its Go type; a duplicate element renderer panics at init.
- `parity.json`, `API-DEVIATIONS.md` and `BACKLOG.md` published alongside the
  package docs.

[0.1.0]: https://github.com/malcolmston/react/releases/tag/v0.1.0
