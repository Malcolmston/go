# Backlog — missing features & gaps

Curated real work for the React port. Items are grouped by the part of React
they come from; the sections at the end cover things React does not have but a
Go port arguably should.

## Renderer & output

Escaping, void elements, the prop→attribute name map (`AttributeName`), style
serialization (`FormatStyle`) and `DangerousHTML` have landed. So have the SVG
and MathML namespaces with their case-sensitive attribute names
(`ssr_namespace.go`), the controlled-form rewrites — `<select value>` deciding
`selected` on the matching `<option>`, `<textarea value>` as text content
(`ssr_control.go`) — React's per-tag attribute orderings for `input`, `button`,
`form` and `option` (`ssr_order.go`), React 19 Float's document-metadata hoisting
in its static-markup shape (`ssr_hoist.go`), and the parity corpus that measures
all of it. What is left is the long tail:

- [ ] Audit the attribute name table against React's full `possibleStandardNames`
      list, and decide what to do with unknown props (upstream passes `data-*`
      and `aria-*` through and drops the rest).
- [ ] Sanitize the image preload's `href` the way React sanitizes it, then flush
      the two preload buckets the emitter already collects. `ssr_preload.go`
      decides every preload correctly and `ssr_hoist.go` knows where the buckets
      belong in the flush order; the link's `href` is built from the `img`'s `src`
      verbatim, so flushing now would put a `javascript:` URL in the document that
      the `img` itself refuses to emit. This is the last step of Float's
      static-markup shape and the last thing standing between `void-img-src` and
      parity.
- [ ] Stream a tree that hoists. Hoisting currently buffers the whole document,
      because the last element can still belong at its front. React solves this
      with `renderToPipeableStream` and a shell/resource protocol; short of that,
      a two-pass walk that discovers hoistables before emitting the body would
      restore streaming for the common case.
- [ ] `<!DOCTYPE html>` and full-document rendering as a first-class mode rather
      than string concatenation at the call site.
- [ ] The rest of hydration: boundary comments, bootstrap scripts, and the client
      that would consume them. The adjacent-text separator has landed, so
      `RenderToString` and `RenderToStaticMarkup` already differ; everything
      above is what would make them differ *usefully*.
- [ ] Streaming render with a real shell/fallback protocol
      (`renderToPipeableStream`-equivalent), including out-of-order flushing of
      resolved Suspense boundaries.
- [ ] Alternative render targets behind one traversal: plain text, a terminal
      layout backend, an `io.Writer`-based document generator.

## Reconciliation & scheduling

- [ ] Restart work at the dirty fiber instead of re-rendering from the root, for
      trees where a root pass is measurably expensive.
- [ ] Real priority lanes so `UseTransition` and `UseDeferredValue` do more than
      preserve their API shape.
- [ ] Interruptible rendering / time slicing with a yield point, and the
      cooperative cancellation that requires.
- [ ] Per-root render serialization instead of the process-wide mutex (needs the
      hook dispatcher moved off a package-level pointer).
- [ ] Move-aware keyed reconciliation that reports placement, so a future
      stateful backend can reorder rather than rebuild.
- [ ] Bailout when a component returns a structurally identical tree.

## Hooks

- [ ] `UseSyncExternalStore` server-snapshot argument, once there is anything to
      hydrate.
- [ ] `UseEffectEvent` (React's `useEffectEvent`) for reading latest props from
      an effect without adding a dependency.
- [ ] Dependency-list linting: a `go vet`-style analyzer that flags a captured
      variable missing from a deps list. This is where most React bugs live and
      Go has no ESLint plugin equivalent.
- [ ] `UseDebugValue` values exposed through a structured inspection API rather
      than a flat `[]string`.
- [ ] `NaN` handling in `ObjectsEqual` to match `Object.is` exactly.
- [ ] A hook-order violation that reports the *source position* of both hooks,
      not just the slot index.

## Components & boundaries

- [ ] `StrictMode` double-invocation of components and effects, to surface impure
      renders and missing cleanups the way React's dev mode does.
- [ ] `Profiler` timing fields with real meaning: actual vs base duration,
      commit phase, interaction tracing.
- [ ] Error boundary: an opt-in *sticky* mode. The boundary currently re-attempts
      its children on every render, which is the right default but gives no way
      to express React's "stay down until reset".
- [ ] Error boundary logging hook (`componentDidCatch`'s side-effecting half),
      separate from the fallback.
- [ ] Suspense: `SuspenseList`-style ordering, and throttling of fallback
      appearance to avoid a flash on fast resolves.
- [ ] `Lazy` with an error path distinct from a suspended path.
- [ ] Portals, or an explicit statement that they cannot exist without a DOM.

## Not ported, and probably shouldn't be

Listed so the boundary is explicit rather than implied:

- [ ] Class components, `this.setState`, the legacy lifecycle methods.
- [ ] The DOM renderer, the synthetic event system, `hydrateRoot`.
- [ ] React Server Components, `use server`, server actions as a transport.
- [ ] The DevTools protocol.
- [ ] JSX. (A code generator that emits `H(...)` calls from a template file is a
      *different* idea, and a plausible one — see below.)

## Ergonomics — the JSX-shaped hole

Writing trees as nested constructor calls is the port's biggest cost. Real
options, none of them free:

- [ ] A `go:generate` template compiler: an `.html`-ish file compiled to a Go
      file of `H(...)` calls, keeping the output ordinary Go with ordinary
      tooling.
- [ ] A builder DSL (`react.Div().Class("x").Text("hi")`) as an alternative
      surface over `CreateElement`, for the host-element-heavy case.
- [ ] Typed props: generic component wrappers that take a struct and marshal it
      into `Props`, trading a little indirection for compile-time checking.
- [ ] `gofmt`-friendly formatting guidance, and examples that demonstrate where
      to break a deep tree into functions.

## Tooling & confidence

- [x] A parity corpus: trees rendered by both real React and this port and diffed
      in CI. Lives in `parity/react` — a vitest project renders each case with
      the pinned `react-dom`, `go/run.go` renders the same cases with the port,
      and the comparison writes `parity/react/parity.json`. What is left is
      growing it, not building it.
- [ ] Fuzz the reconciler: random keyed/keyless child-list mutations, asserting
      that state follows keys and that every dropped fiber is cleaned up exactly
      once.
- [ ] Fuzz the HTML escaper against `html/template`.
- [ ] Benchmarks for a deep tree, a wide keyed list, and a memoized subtree, so
      the "re-render from the root" choice can be revisited with data.
- [ ] Race-detector coverage for updates arriving from background goroutines
      during a render.
- [ ] Examples: an HTTP server rendering a page, a static-site generator, a
      Suspense-driven data-fetching page.
- [ ] `golangci-lint` config and a documentation site matching the rest of the
      family.

---

### A note on scope

This lists real, actionable gaps rather than padding to a round number. The
richest genuine work here is the renderer's HTML fidelity (React's attribute
handling has a long tail) and the parity corpus that would let `parity.json`
carry a measured figure instead of a dash.
