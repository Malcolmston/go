# Backlog — missing features & gaps

Curated real work for the d3 port. The port covers d3's **computational** half —
data, math, geometry, formatting. The sections below are grouped by the upstream
module they come from, and the first one is the honest headline: several whole
d3 modules are not here yet.

## Whole modules not ported yet

These are genuinely missing, not deliberately excluded. Each is a real body of
work and each is wanted:

- [ ] **`d3-geo`** — spherical math, the projection family (Mercator,
      orthographic, conic, equal-earth, …), `geoPath`, clipping, rotation,
      graticules, and the GeoJSON plumbing underneath all of it. The largest
      single gap, and the one most often asked for. `d3/path` already provides
      the output sink a `geoPath` would render into.
- [ ] **`d3-contour`** — marching squares over a value grid, `contourDensity`.
      Depends on nothing this port lacks; it emits multipolygons that
      `d3/path` can already serialize.
- [ ] **`d3-delaunay` / Voronoi** — Delaunator's incremental hull algorithm plus
      the dual Voronoi diagram, cell clipping and neighbour queries.
- [ ] **`d3-quadtree`** — the spatial index. Wanted in its own right for hit
      testing and nearest-neighbour queries, and a prerequisite for a usable
      force simulation.
- [ ] **`d3-force`** — the velocity-Verlet simulation and the standard forces
      (link, many-body, center, collide, x/y/radial). Needs the quadtree for
      Barnes–Hut approximation of many-body.
- [ ] **`d3-chord`** — chord layout and the ribbon generator.
- [ ] **`d3-polygon`** — hull, centroid, area, containment. Small, self-contained,
      a good first contribution.
- [ ] **`d3-fetch` / `d3-dsv` request helpers** — `d3.csv(url)` and friends. The
      parsing half lives in `d3/dsv`; the fetching half is `net/http` and does
      not obviously belong in a charting library.

## Deliberately out of scope

Listed so the boundary is explicit rather than implied. These need a live
document, which Go does not have:

- **`d3-selection`**, **`d3-transition`** — data binding and tweening against
  DOM nodes.
- **`d3-drag`**, **`d3-zoom`**, **`d3-brush`** — pointer-event behaviours.
- **`d3-dispatch`** — an event bus whose reason to exist is those behaviours.
  Go channels cover the same ground where it is needed.
- **`d3-timer`** — a `requestAnimationFrame` scheduler. `time.Ticker` is the Go
  answer.

If a client-side story ever appears (WebAssembly, or a Go-driven canvas), these
become interesting again. Until then, this port computes and
[`react`](https://github.com/malcolmston/react) renders.

## `d3/array`

- [ ] Multi-key nesting for `Group` / `Groups` / `Rollup` / `Index`, which
      upstream supports by passing several key functions.
- [ ] `d3.cross`, and `flatGroup` / `flatRollup`.
- [ ] `d3.blur`, `blur2`, `blurImage`.
- [ ] Go 1.23 iterator (`iter.Seq`) forms alongside the slice-taking ones, so
      the statistics can consume a stream without materializing it.
- [ ] Set operations over a *key function* rather than over directly comparable
      elements — upstream's `InternMap` trick, which is how `union` works on
      objects and dates.

## `d3/scale`

- [ ] The `d3-scale-chromatic` palettes — the categorical schemes (Category10,
      Tableau10, Set3, …) and the continuous ones (Viridis, Magma, Turbo, the
      ColorBrewer ramps). A large table with no algorithm behind it; worth
      generating rather than typing.
- [ ] UTC/local parity for `Time` across every method (see the cross-package
      note about the duplicated intervals).
- [ ] `interpolateRound` as a first-class scale option everywhere it applies.

## `d3/shape`

- [ ] `arc` corner-radius edge cases: the full inner/outer corner geometry when
      the corner radius exceeds the available angular span.
- [ ] `link` / `linkHorizontal` / `linkVertical` / `linkRadial`.
- [ ] The remaining curves' rarely-exercised branches — `curveBundle`'s beta,
      `curveCardinalClosed`, `curveCatmullRomOpen`, `curveBumpRadial`.
- [ ] `stack` offsets `wiggle` and `diverging` verified against upstream on
      inputs with negative and missing values.
- [ ] `symbol`'s full set including the `symbolsFill` / `symbolsStroke` split
      introduced in d3 7.7.

## `d3/hierarchy`

- [ ] `treemapResquarify`, and the `paddingInner`/`paddingOuter` interaction
      with `tile` for every tiling method.
- [ ] `pack`'s enclosing-circle solver on degenerate inputs (one node, zero
      radius, collinear centres).
- [ ] `hierarchy.copy`, `path`, `find`, and the full traversal set.
- [ ] `stratify` error messages matching upstream's for cycles, multiple roots
      and missing parents.

## `d3/format` and `d3/timefmt`

- [ ] `formatLocale` / `timeFormatLocale` — the full locale object (currency,
      decimal, grouping, thousands, day and month names) rather than the
      built-in English defaults.
- [ ] `formatPrefix` for a fixed SI prefix independent of the value.
- [ ] `scaleTime`'s multi-scale conditional tick format — the cascade that
      labels a tick with the year, the month or the hour depending on which
      boundary it falls on. `timefmt.Format` now exists to express it; `scale`
      does not use it yet.
- [ ] The `%j` / `%U` / `%W` week-number directives round-tripping through
      `Format` and `Parse` correctly.
- [ ] `timefmt.Parse` on ambiguous or DST-straddling local times, and a
      documented rule for which instant it picks.

## `d3/color` and `d3/interpolate`

- [ ] The `gamma` variants of the interpolators. (`Cubehelix` and
      `CubehelixLong` have landed alongside the RGB/HSL/Lab/HCL pairs.)
- [ ] CSS Color 4 syntax: `color()`, `lab()`/`lch()`/`oklab()`/`oklch()` function
      forms and space-separated `rgb()` with slash alpha.
- [ ] `interpolateTransformCss` / `interpolateTransformSvg` — the transform-list
      parser and its per-component interpolation. (`Zoom` has landed.)
- [ ] Field-wise interpolation for structs. `Object` covers `map[K]V`; a struct
      equivalent needs either reflection or code generation, and it is not
      obvious which is the right trade.
- [ ] Extend `Value`'s type sniffing to cover every case `d3.interpolate`
      handles, and document precisely which ones it does not.

## Cross-package consistency

Real seams left by building the packages in parallel. Neither remaining item is
a bug today; each is an inconsistency a reader meets inside a single chart.

Two duplications that were listed here have since been closed, and are recorded
because the reason they existed is worth remembering: both were written so a
package would not block on a sibling that had not landed yet.

- [x] **`scale.TickFormat` routes through `d3/format`.** The step-derived
      `formatAuto` stand-in is gone. `TickFormatSpecifier(count, specifier)`
      now accepts real d3 specifiers, with precision filled from the tick step
      via `PrecisionFixed`/`PrecisionRound`/`PrecisionPrefix`, so an axis label
      and an explicit `format.New(".2f")` cannot disagree.
- [x] **Time intervals have one definition.** `scale/timeinterval.go` was
      deleted; `Time` delegates to `timefmt.Ticks`/`TickInterval`/`Format`, and
      `Time.Interval(count)` returns a `*timefmt.Interval`. `NewTime` follows
      the domain's own location rather than `time.Local`, matching `timefmt`'s
      zone model.
- [ ] **Accessor signatures differ between `array` and `shape`.** `array` takes
      `func(T) float64`; `shape` takes `func(d T, i int, data []T) float64`.
      Both choices are defensible in isolation (see API-DEVIATIONS.md) but
      composing them needs a wrapper. Decide whether an adapter helper is worth
      shipping.
- [ ] **Configuration convention differs between `scale` and everything else.**
      `scale` uses `Domain()` / `SetDomain(…)`; `shape` and `hierarchy` are
      write-only with the bare name. The rule ("does anything read it back?") is
      documented, but a reader coming from d3 meets both within one chart.
- [ ] A top-level `d3` package that re-exports the common entry points, so a
      simple chart needs one import rather than four.

## Tooling & confidence

- [ ] **A parity corpus.** Run the same inputs through real d3 in Node and
      through this port, and diff the outputs in CI: tick arrays, formatted
      numbers, path strings, scale evaluations, interpolated colors. This is the
      only honest way to put a number in `parity.json`, and it should come
      before any further feature work.
- [ ] Golden-file tests for path output, so a formatting change to `d3/path` is
      visible in a diff rather than in a rendered chart.
- [ ] Fuzz the DSV reader against `encoding/csv`, and the format-specifier
      parser against a grammar of its own.
- [ ] Property tests: a scale composed with its own `Invert` is the identity
      within tolerance; `Ticks` never returns a value outside the domain; `Bin`
      never loses or duplicates a datum.
- [ ] Benchmarks — scale evaluation in a hot loop, path building for a
      100k-point line, hierarchy layout on a deep tree — so the allocation
      choices can be revisited with data.
- [ ] Worked examples paired with `react`: an axis component, a line chart, a
      treemap, served over `net/http`.
- [ ] `golangci-lint` config and a documentation site matching the rest of the
      family.

---

### A note on scope

This lists real, actionable gaps rather than padding to a round number. The
largest genuine work is `d3-geo` (a module the size of several of the ported
ones combined) and the parity corpus that would let `parity.json` carry a
measured figure instead of a dash.
