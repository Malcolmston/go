# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-08-05
### Added
- Initial public release — a Go port of the **computational half of d3**: data,
  math, geometry and formatting, with no DOM and no rendering.
- `d3/array` — statistics (`Min`, `Max`, `Extent`, `Sum`, `FSum`, `Cumsum`,
  `Mean`, `Median`, `Mode`, `Quantile`, `Variance`, `Deviation`, `Rank`), binary
  search (`Bisect`, `BisectLeft/Right`, `Bisector`, `Quickselect`), transforms
  (`Group`, `Groups`, `Rollup`, `Index`, `Merge`, `Pairs`, `Permute`, `Shuffle`,
  `Range`, `Transpose`, `Zip`), set operations (`Union`, `Intersection`,
  `Difference`, `Disjoint`, `Superset`, `Subset`), histogram binning with the
  Sturges, Scott and Freedman–Diaconis threshold estimators, and the `Ticks` /
  `TickStep` / `TickIncrement` / `NiceTicks` algorithm every quantitative scale
  is built on. Each statistic comes in a generic accessor form and an `…Of` form
  specialized to `[]float64`.
- `d3/color` — sRGB, HSL, Lab and HCL color values, CSS color parsing
  (`Parse` / `MustParse`), the named-color table, `Brighter` / `Darker`,
  `Displayable` / `Clamp`, and hex and CSS serialization.
- `d3/interpolate` — `Number`, `Round`, `String`, `Numbers`, `Array`, `Object`,
  `Basis` / `BasisClosed`, `Piecewise`, `Zoom`, the color interpolators (`RGB`,
  `HSL`, `Lab`, `HCL`, `Cubehelix` and their `…Long` variants), and `Value` as
  the port of d3's runtime type-sniffing `d3.interpolate`.
- `d3/scale` — continuous (`NewLinear`, `NewPow`, `NewSqrt`, `NewLog`,
  `NewSymlog`, `NewIdentity`, `NewRadial`, `NewTime`, `NewUTC`), sequential and
  diverging (including the `…Log` / `…Pow` / `…Sqrt` / `…Symlog` variants and
  `NewSequentialQuantile`), discretizing (`NewQuantize`, `NewQuantile`,
  `NewThreshold`) and discrete (`NewOrdinal`, `NewBand`, `NewPoint`) scales,
  with `Nice`, `Ticks`, `TickFormat`, `Invert`, `SetClamp` and `Copy`.
- `d3/path` — the SVG path builder: `MoveTo`, `LineTo`, `QuadraticCurveTo`,
  `BezierCurveTo`, `ArcTo`, `Arc`, `Rect`, `ClosePath`, and `String`, with
  `NewWithPrecision` for coordinate rounding.
- `d3/shape` — `Line` and `Area` with their radial counterparts, `Arc`, `Pie`,
  `Stack` with the full offset and order sets, `Symbol` and `SymbolsFill`, and
  the curve family (linear, basis, bundle, cardinal, Catmull–Rom, monotone,
  natural, step, and the closed/open variants) — all emitting SVG path strings
  through `d3/path`.
- `d3/format` — d3's number-format specifier grammar (`ParseSpecifier`, `New`,
  `MustNew`), SI prefixes, rounding, grouping and the locale scaffolding.
- `d3/timefmt` — the time intervals (`Millisecond` … `Year`, the weekday
  intervals, and their `UTC…` twins) with `Floor`, `Ceil`, `Round`, `Offset`,
  `Range`, `Filter`, `Count` and `Every`; `TickInterval` and `Ticks` for time
  axes; and strftime-style `Format` / `Parse` (with `UTCFormat` / `UTCParse`,
  `ISOFormat` / `ISOParse` and the locale scaffolding).
- `d3/hierarchy` — `Hierarchy`, `Stratify`, the traversals (`Each`,
  `EachBefore`, `EachAfter`, `Ancestors`, `Descendants`, `Leaves`, `Find`,
  `Path`, `Links`, `Copy`) and the `Tree`, `Cluster`, `Treemap`, `Partition`
  and `Pack` layouts with the tiling methods.
- `d3/ease` — the easing family (linear, poly, quad, cubic, sin, exp, circle,
  elastic, back, bounce) in `In` / `Out` / `InOut` forms, with the `…With`
  parameterized variants.
- `d3/random` — the distributions (uniform, normal, log-normal, Bates,
  Irwin–Hall, exponential, Pareto, Bernoulli, geometric, binomial, gamma, beta,
  Weibull, Cauchy, logistic, Poisson), over a package default source or an
  explicit one from `Source` (d3's `randomLcg`) or `SourceFunc`.
- `d3/dsv` — CSV/TSV reading and writing (`Parse`, `ParseRows`, `Format`,
  `FormatRows` and their CSV/TSV shorthands) and `AutoType` type inference.
- `API-DEVIATIONS.md` itemising every deliberate departure from d3's JavaScript
  API, and `parity.json` published with no invented figure.

### Notes
- Standard library only; requires **Go 1.24+** (the packages use generics).
- `d3-selection`, `d3-transition`, `d3-drag`, `d3-zoom` and `d3-brush` are
  deliberately out of scope: they exist to manipulate a live document, and Go
  has none. This port computes; [`react`](https://github.com/malcolmston/react)
  renders what it computes.
- `d3-geo`, `d3-contour`, `d3-delaunay`, `d3-quadtree`, `d3-force`, `d3-chord`
  and `d3-polygon` are **not yet ported**. They are genuinely missing rather
  than excluded, and are tracked in [BACKLOG.md](BACKLOG.md).
- No parity percentage is published. Nothing has been diffed against d3's own
  test suite yet, so any number would be invented; see `parity.json`.

[0.1.0]: https://github.com/malcolmston/d3/releases/tag/v0.1.0
