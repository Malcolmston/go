# Backlog — missing features & gaps

Curated real work for the React port. Items are grouped by the part of React
they come from; the sections at the end cover things React does not have but a
Go port arguably should.

## Renderer & output

Escaping, void elements, the prop→attribute name map (`AttributeName`), style
serialization (`FormatStyle`) and `DangerousHTML` have landed. What is left is
the long tail:

- [ ] Audit the attribute name table against React's full `possibleStandardNames`
      list, and decide what to do with unknown props (upstream passes `data-*`
      and `aria-*` through and drops the rest).
- [ ] SVG and MathML namespaces, whose attribute names are case-sensitive
      (`viewBox`, `strokeWidth`) and do not follow the HTML table.
- [ ] `<select value>` → `selected` on the matching `<option>`, and the other
      controlled-input attribute rewrites React's SSR performs.
- [ ] Hoisting `<title>`, `<meta>` and `<link>` out of the body, as React 19
      does for document metadata rendered mid-tree.
- [ ] `<!DOCTYPE html>` and full-document rendering as a first-class mode rather
      than string concatenation at the call site.
- [ ] Real hydration markers, and the client that would consume them — the only
      thing that would make `RenderToString` and `RenderToStaticMarkup` differ.
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

- [ ] A parity corpus: a set of trees rendered by both real React
      (`renderToStaticMarkup`) and this port, diffed in CI. This is the only
      honest way to put a number in `parity.json`.
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
