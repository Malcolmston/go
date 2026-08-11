# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Changed
- **`RenderToString` and `RenderToStaticMarkup` are no longer aliases**, and
  `RenderToString` output has changed. React's two functions differ by hydration
  metadata; the one piece of that metadata which reaches the bytes of a
  server-rendered document is the separator between two adjacent text nodes, and
  the port now reproduces it. `RenderToString(H("div", nil, "a", "b"))` is
  `<div>a<!-- -->b</div>` where it was `<div>ab</div>`;
  `RenderToStaticMarkup` of the same tree is unchanged at `<div>ab</div>`.
  `RenderToWriter` and `MustRenderToString` render in `RenderToString` mode and
  so gained the separator too; `RenderPortalsToString` renders in static-markup
  mode and did not. Only *adjacent text* separates — a tag ends the run — and an
  empty text node between two real ones is not a boundary. **If you were
  treating the two functions as interchangeable, switch the calls whose output is
  final to `RenderToStaticMarkup`.** See `API-DEVIATIONS.md`.
- **Attribute order for `input`, `button`, `form` and `option` has changed**, to
  match React's hand-written serializers for those four tags: a fixed set of
  props is now deferred to the end of the tag in React's own order rather than
  emitted in the general sorted pass. An `input` given `id`, `name`, `required`
  and `type` renders them `id required type name` where it rendered
  `id name required type`. Attribute order elsewhere is unchanged — sorted by
  prop name.
- **Document metadata is now hoisted**, so an existing page that renders a
  `<title>`, `<meta>`, `<link>` or async `<script src>` inside its body will see
  that element move to the front of the document — into the `<head>` when there
  is one, and into a synthesized `<head>` when an `<html>` lacks one. This
  matches React 19's Float. `<base>`, a `<link rel="stylesheet">` or `<style>`
  with no `precedence`, a `<link>` with no `href`, anything carrying `itemProp`,
  a `<title>` inside `<svg>` and anything inside a `<noscript>` all stay where
  they were written. One consequence for `RenderToWriter`: a tree that can hoist
  is buffered in memory and written once, because the last element of a document
  can still belong at its front. A tree with no document machinery in it still
  streams exactly as before.

### Documented
- The attribute-order deviation is now written down as the one difference that
  cannot be fixed from inside this package: React emits attributes in the props
  object's insertion order, `Props` is a Go map with no order, and the parity
  harness decodes each case's props through `json.Unmarshal` into a
  `map[string]any` (`parity/react/go/run.go`, `buildProps`), so the authored key
  order is gone before the port ever sees it. Sorted output remains the
  documented rule. See `API-DEVIATIONS.md`, *Attribute output order*.
- What "React 19 Float, in its static-markup shape" does and does not include:
  the hoisting that landed, the exceptions that pin an element in place, and the
  four limits — no streaming, no bootstrap scripts, no resource dedupe across
  requests, and image preloads decided but not yet flushed pending URL
  sanitizing. See `API-DEVIATIONS.md`, *Document metadata and resource hoisting*.
- Every byte-level claim the documents make is now pinned by
  `doc_claims_test.go`, so prose that quotes markup fails a test instead of
  rotting quietly. Each case names the document it defends, and the two
  deviations are asserted *as* deviations: the test fails if the port ever starts
  matching React, which is the signal to rewrite the section rather than to relax
  the assertion.

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
