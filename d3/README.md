# d3

[![Go Test](https://github.com/Malcolmston/d3/actions/workflows/go-test.yml/badge.svg)](https://github.com/Malcolmston/d3/actions/workflows/go-test.yml)
[![Go Lint](https://github.com/Malcolmston/d3/actions/workflows/go-lint.yml/badge.svg)](https://github.com/Malcolmston/d3/actions/workflows/go-lint.yml)
[![Go Vuln](https://github.com/Malcolmston/d3/actions/workflows/go-vuln.yml/badge.svg)](https://github.com/Malcolmston/d3/actions/workflows/go-vuln.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3.svg)](https://pkg.go.dev/github.com/malcolmston/d3)
[![Go Report Card](https://goreportcard.com/badge/github.com/malcolmston/d3)](https://goreportcard.com/report/github.com/malcolmston/d3)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Malcolmston/d3)](go.mod)
[![Release](https://img.shields.io/github/v/release/Malcolmston/d3?sort=semver)](https://github.com/Malcolmston/d3/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Malcolmston/d3)](https://github.com/Malcolmston/d3/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/Malcolmston/d3)](https://github.com/Malcolmston/d3)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Docs](https://img.shields.io/badge/docs-pages-2f9bff)](https://malcolmston.github.io/d3/)

**The computational half of d3 — for Go.**

d3 is two libraries wearing one name. One half manipulates a live document:
selections, transitions, drag, zoom, brush. The other half is arithmetic —
scales, shapes, interpolators, hierarchies, formats, statistics — and needs no
browser at all.

This is the second half. It computes; something else renders.

- **`d3/array`** — statistics, bisect, transforms, sets, bins, and the tick
  algorithm every scale is built on.
- **`d3/color`** — sRGB, HSL, Lab, HCL, CSS parsing.
- **`d3/interpolate`** — numbers, strings, colors, basis splines, piecewise.
- **`d3/scale`** — linear, pow, log, symlog, time, sequential, diverging,
  quantize, quantile, threshold, ordinal, band, point.
- **`d3/path`** — the SVG path builder.
- **`d3/shape`** — line, area, arc, pie, stack, symbols, and the curve family.
- **`d3/format`** — d3's number-format specifier grammar and SI prefixes.
- **`d3/timefmt`** — time intervals, time-axis ticks, and strftime-style parse
  and format.
- **`d3/hierarchy`** — tree, cluster, treemap, partition, pack.
- **`d3/ease`** — the easing family.
- **`d3/random`** — seeded distributions.
- **`d3/dsv`** — CSV/TSV with object parsing and type inference.

Standard library only.

## Install

```sh
go get github.com/malcolmston/d3
```

Requires **Go 1.24+** (the packages use generics throughout).

## A line chart

The flagship use case, and the shortest complete statement of what this library
is for: a **scale** positions the data, a **shape generator** turns positions
into geometry, and the geometry comes out as a string an SVG `<path>` can wear.

```go
package main

import (
	"fmt"

	"github.com/malcolmston/d3/array"
	"github.com/malcolmston/d3/scale"
	"github.com/malcolmston/d3/shape"
)

type Point struct {
	Day   float64
	Value float64
}

func main() {
	data := []Point{{0, 120}, {1, 180}, {2, 140}, {3, 260}, {4, 230}, {5, 310}}

	const w, h = 640.0, 480.0

	maxV, _ := array.Max(data, func(p Point) float64 { return p.Value })

	// Note the inverted y range: SVG's origin is top-left, a chart's is
	// bottom-left. Flipping the range is how every d3 chart handles that.
	x := scale.NewLinear().SetDomain(0, 5).SetRange(0, w)
	y := scale.NewLinear().SetDomain(0, maxV).SetRange(h, 0).Nice(10)

	line := shape.NewLine[Point]().
		X(func(p Point, _ int, _ []Point) float64 { return x.Scale(p.Day) }).
		Y(func(p Point, _ int, _ []Point) float64 { return y.Scale(p.Value) }).
		Curve(shape.CurveMonotoneX)

	d := line.Generate(data)
	fmt.Printf(`<path d=%q fill="none" stroke="steelblue"/>`+"\n", d)

	// Axis labels, chosen and formatted the way d3 would.
	label := y.TickFormat(5)
	for _, t := range y.Ticks(5) {
		fmt.Printf("y=%.1f  %s\n", y.Scale(t), label(t))
	}
}
```

Handed to a renderer, the whole integration is one prop:

```go
import "github.com/malcolmston/react"

react.H("svg", react.Props{"viewBox": "0 0 640 480"},
	react.H("path", react.Props{
		"d":      line.Generate(data),
		"fill":   "none",
		"stroke": "currentColor",
	}),
)
```

The `d3` packages do not import `react`, and `react` does not import `d3`. The
coupling between them is a `string` of path data and a set of scaled
coordinates — see [OVERVIEW.md](OVERVIEW.md) for why that seam is the whole
architectural story.

## The packages

### `d3/array` — data before it is a picture

Statistics, binary search, reshaping, and the tick algorithm.

```go
values := []float64{4, 8, 15, 16, 23, 42}

mean, ok := array.MeanOf(values)            // 18, true
med, _   := array.MedianOf(values)          // 15.5
q1, _    := array.QuantileOf(values, 0.25)  // 9.75
lo, hi, _ := array.ExtentOf(values)         // 4, 42

// The generic form takes an accessor:
top, _ := array.Max(rows, func(r Row) float64 { return r.Revenue })

i := array.BisectLeft(sorted, 17)           // insertion point
ticks := array.Ticks(0, 100, 5)             // [0 20 40 60 80 100]
```

Every statistic returns a second `ok` bool, and it is `false` when there was
nothing to compute from — the Go answer to `d3.mean([])` returning `undefined`.
`NaN` is treated the way d3 treats `null`/`undefined`: **skipped** by the
statistics, and sorted **last** by the ordering functions.

`Ticks` / `TickStep` / `TickIncrement` are load-bearing: every quantitative
scale's `Nice` and `Ticks` route through them.

### `d3/scale` — data units to display units

```go
x := scale.NewLinear().SetDomain(0, 100).SetRange(0, 640)
x.Scale(50)          // 320
x.Invert(320)        // 50
x.SetClamp(true)     // stop extrapolating outside the domain

scale.NewLog().SetBase(10).SetDomain(1, 1e6).SetRange(0, 400)
scale.NewPow().SetExponent(0.5).SetDomain(0, 1).SetRange(0, 100)
scale.NewSymlog().SetConstant(1)                     // handles zero and negatives
scale.NewBand[string]().SetDomain(cats...).SetRange(0, 640).SetPadding(0.1)

// A color ramp: the range is an interpolator rather than two endpoints.
warm := interpolate.Lab(color.MustParse("#fee"), color.MustParse("#900"))
heat := scale.NewSequential(warm).SetDomain(0, maxV)
heat.Scale(v)                                        // a color.Color
```

Continuous scales **normalize** the input to `[0, 1]` within the domain and then
**interpolate** the range at that position; every quantitative scale is that
pair with a transform wrapped around the normalize step, which is why they share
one generic `Continuous[R]` type. `R` is the range type, so the same machinery
maps dollars to pixels (`Continuous[float64]`) or dollars to colors.

Quantize, quantile and threshold are the three that get confused with each
other. The difference is only where the breakpoints come from: quantize cuts the
*domain* uniformly, quantile cuts the *data* so each bucket holds equally many
observations, and threshold takes the cuts you supply.

`Band` and `Point` are `Ordinal` specialized for position. `Bandwidth()` is the
width of a bar.

### `d3/shape` and `d3/path` — geometry as a string

```go
p := path.New()
p.MoveTo(0, 0).LineTo(100, 50).BezierCurveTo(120, 60, 140, 40, 160, 50)
p.String()  // "M0,0L100,50C120,60,140,40,160,50"

area := shape.NewArea[Point]().
	X(func(p Point, _ int, _ []Point) float64 { return x.Scale(p.Day) }).
	Y0Const(y.Scale(0)).
	Y1(func(p Point, _ int, _ []Point) float64 { return y.Scale(p.Value) })

area.Generate(data)         // the filled band
area.Line().Generate(data)  // just its top edge, as a Line

arc := shape.NewArc().InnerRadius(60).OuterRadius(100).CornerRadius(4)
pie := shape.NewPie[Row]().Value(func(r Row, _ int, _ []Row) float64 { return r.Count })

for _, w := range pie.Generate(rows) {
	fmt.Println(arc.Generate(w.Arc()))
	cx, cy := arc.Centroid(w.Arc())   // two float64s, not a two-element slice
	_, _ = cx, cy
}

stack := shape.NewStack[Month]().Keys("apples", "pears").
	Value(func(m Month, key string, _ int, _ []Month) float64 { return m.Sales[key] })
series := stack.Generate(months)  // one Series per key, each holding [y0, y1] pairs

shape.NewSymbol().Type(shape.SymbolStar).Size(64).Generate()
shape.SymbolsFill   // the seven-shape categorical scale, in assignment order
```

Where d3 overloads a channel to take either a constant or an accessor, this
port gives you two methods, and the short name goes to whichever form dominates:
`Line.X(fn)` / `Line.XConst(v)` for a positional channel, but
`Arc.InnerRadius(v)` / `Arc.InnerRadiusFunc(fn)` for a radius.

`Pie` computes *angles*, not paths — feeding them to `Arc` is the second half of
the operation, which is what lets the same angles drive a donut, an exploded
slice and a label position. `Area` is a top edge forwards and a baseline
backwards, joined and closed. `Stack` turns a table into `[y0, y1]` pairs, and
its offsets are what make the same data a stacked bar chart, a percentage chart
or a streamgraph.

The **curve** decides how consecutive points are joined, and it is a stream with
lookahead state — `CurveBasis` holds three points before it can emit a segment,
`CurveMonotoneX` computes tangents that cannot overshoot (which is why it is the
right choice for data that must never appear to dip below a value it never had).
Because of that state, a generator is configured with a curve **factory**
(`Curve(shape.CurveMonotoneX)`), not with a curve, and builds a fresh one per
call — so a configured generator stays a pure function from data to a string and
is safe to call from any goroutine. Parameterized curves come as
`…Factory` functions: `Curve(shape.CurveCardinalFactory(0.5))`.

An empty input produces `""`, where d3 produces `null`. Check for the empty
string before emitting a `<path>`.

### `d3/color` and `d3/interpolate`

```go
c, err := color.Parse("#4682b4")           // hex, rgb(), hsl(), named
fmt.Println(color.ToRGB(c).Hex())          // #4682b4
fmt.Println(color.ToRGB(c).Darker(1))

ramp := interpolate.Number(0, 100)         // func(t float64) float64
ramp(0.5)                                  // 50

interpolate.String("0px", "24px")          // interpolates embedded numbers
interpolate.Basis([]float64{0, 5, 2, 9})   // a B-spline through the values
interpolate.Piecewise(interpolate.Number, []float64{0, 10, 100})
```

Colors are value types, not mutable objects: `Brighter` and `Darker` return new
values. Out-of-gamut channels are allowed (Lab and HCL produce them
legitimately); `Displayable()` asks, and `Clamp()` fixes.

`d3.interpolate`'s runtime type sniffing is available as `interpolate.Value`,
but the named constructors are typed, faster and checked at compile time — use
those unless you are translating d3 code line for line.

### `d3/format` and `d3/timefmt`

```go
f, _ := format.New(",.2f")
f(1234.5678)                    // 1,234.57

format.MustNew(".1%")(0.1234)   // 12.3%
format.MustNew(".3s")(1300)     // 1.30k
format.MustNew("$,.2f")(1e6)    // $1,000,000.00
```

`format` implements d3's specifier grammar
(`[[fill]align][sign][symbol][0][width][,][.precision][~][type]`), which is not
`fmt`'s verb syntax and is not trying to be. The `s` type — SI prefixes — is the
one with no Go equivalent at all, and it is why axis labels can read `0, 5k,
10k` instead of `0, 5000, 10000`.

Note that `scale.TickFormat` does not yet route through this package: it carries
its own step-derived formatter, so an axis label and an explicit
`format.New(".2f")` come from two different code paths. Unifying them is tracked
in [BACKLOG.md](BACKLOG.md).

```go
timefmt.Day.Floor(t)
timefmt.Month.Range(start, stop, 1)     // the first of each month in between
timefmt.Monday.Count(yearStart, t)      // week number
timefmt.Ticks(start, stop, 10)          // the values a time axis should label

timefmt.Format("%Y-%m-%d")(t)           // strftime directives, not Go layouts
timefmt.Parse("%Y-%m-%d")("2026-08-05") // (time.Time, error)
timefmt.ISOFormat(t)
```

`timefmt` supplies the time intervals — `Millisecond`, `Second`, `Minute`,
`Hour`, `Day`, the weekday intervals (`Sunday` … `Saturday`, with `Week` as an
alias for `Sunday`), `Month`, `Year` and their `UTC…` twins — each with `Floor`,
`Ceil`, `Round`, `Offset`, `Range`, `Filter`, `Count` and `Every`, plus
`TickInterval` and `Ticks` for choosing what a time axis should be labelled at,
and the strftime-style `Format` / `Parse` pair (with `UTCFormat` / `UTCParse`
and the ISO helpers).

Use Go's own reference layouts when the format is yours; use this package when
the format came from d3.

### `d3/hierarchy`

```go
root := hierarchy.Hierarchy(data, func(d Datum) []Datum { return d.Children })
root.Sum(func(d Datum) float64 { return d.Size })
root.Sort(func(a, b *hierarchy.Node[Datum]) int { return cmp.Compare(b.Value, a.Value) })

hierarchy.NewTreemap[Datum]().Size(960, 600).Padding(2).Layout(root)
for _, n := range root.Descendants() {
	fmt.Println(n.X0, n.Y0, n.X1, n.Y1)
}
```

Traversals (`Each`, `EachBefore`, `EachAfter`, `Ancestors`, `Descendants`,
`Leaves`, `Find`, `Path`, `Links`, `Copy`) and the layouts — `NewTree`,
`NewCluster`, `NewTreemap`, `NewPartition`, `NewPack` — all follow d3's shapes.

A layout **annotates the tree in place** rather than returning a new one, which
matches upstream: run the treemap and then read the coordinates off the nodes
you already hold (`Layout` returns the same root for chaining). Running a second
layout over the same tree overwrites the first.

`Stratify` builds a hierarchy from flat parent/child rows, and reports cycles,
multiple roots and missing parents as errors.

### `d3/ease`, `d3/random`, `d3/dsv`

```go
ease.CubicInOut(0.5)                    // 0.5
ease.ElasticOut(0.3)

src := random.Source(42)                // d3's randomLcg — reproducible
gauss := src.Normal(0, 1)
gauss()

random.Normal(0, 1)()                   // the default source, seeded at startup
random.SourceFunc(myRNG.Float64)        // bring your own uniform generator

tbl, err := dsv.ParseCSV(text)          // *Table: Rows plus the column order
typed := dsv.AutoTypeRows(tbl.Rows)     // d3.autoType-style inference
fmt.Println(dsv.FormatCSV(tbl.Rows, tbl.Columns))
```

`random` mirrors d3's split: package-level functions draw from a default source
(`math/rand/v2`'s global generator, seeded unpredictably at startup, so **not**
reproducible), and `Source(seed)` gives you d3's `randomLcg` for when a test or
a rendering has to reproduce exactly. `Source` reproduces d3's seed handling
faithfully, including the idiosyncrasy that a seed in `[0, 1)` is scaled by
2³² while any other seed has its absolute value taken.

## How this differs from d3 in JavaScript

The algorithms are faithful; the API shape is not always, and the reasons are
worth knowing before you start translating.

- **No DOM, therefore no selection, transition, drag, zoom or brush.** Those
  five modules exist to manipulate a live document, and Go has none. This is a
  scope decision, not a gap: the work they do is `react`'s. `d3/interpolate` and
  `d3/ease` — the actual math a transition runs — *are* here, so code with its
  own clock has everything it needs.
- **Generics and accessors instead of dynamic property access.** d3 discovers at
  runtime whether your datum has an `.x`, a `[0]` or a `.length`. Here the datum
  is a type parameter and the channels are explicit accessor functions, so a
  renamed field breaks the build rather than the chart. `array` drops the `(d,
  i, data)` index arguments because they are almost never used there; `shape`
  keeps them, because positioning a point by its index is ordinary charting
  code.
- **Explicit getters and setters instead of d3's overloaded method.**
  `scale.domain()` cannot be both a read and a write in Go, so `scale` splits it
  into `Domain()` and `SetDomain(…)`. `shape` resolves the same problem the
  other way — configuration is write-only, so `line.X(fn)` keeps the short name
  and there is no getter at all. Setters return the receiver, so chaining
  survives in both. A scale value is therefore cheap to configure but is not
  safe for concurrent *mutation*: finish configuring before publishing the
  pointer, or hand each goroutine a `Copy()`.
- **The scale itself is not callable.** `scale(x)` becomes `scale.Scale(x)`, and
  `line(data)` becomes `line.Generate(data)`. Go has no callable value that also
  carries methods.
- **Typed errors instead of silent coercion.** An unparseable color, a malformed
  format specifier or a broken CSV row is an `error`, not a `null` that
  propagates into a chart as a blank. Where a value legitimately might not
  exist, the answer is an `ok` bool: `array.MinOf(nil)` returns `(0, false)`,
  because `undefined` and `0` are different facts and Go has no way to say the
  first.
- **`NaN` still propagates.** A scale handed `NaN` returns `NaN`, exactly as
  upstream. Coercing it to `0` would produce a chart that is quietly wrong
  rather than visibly broken.
- **SVG output is a string, not a canvas context.** `line.context(ctx)` has no
  counterpart; generators return path data. The internal `shape.Context`
  interface survives because curves write into it and coordinate transforms wrap
  it, which is a better reason for it than canvas support was.
- **Layouts mutate, generators do not.** A hierarchy layout writes coordinates
  onto the nodes it is given (upstream behavior, deliberately kept). A shape
  generator is a pure function from data to a string and is safe to call from
  any goroutine once configured.

Every deviation, including the smaller ones, is itemised with its reason in
[API-DEVIATIONS.md](API-DEVIATIONS.md).

### Not ported

Only the DOM-bound modules — `d3-selection`, `d3-transition`, `d3-drag`,
`d3-zoom`, `d3-brush`, `d3-axis` — and they are out of scope rather than
outstanding: each exists to mutate a live document, and there is none. See
[API-DEVIATIONS.md](API-DEVIATIONS.md) for what replaces them.

**The computational surface is complete.** `d3-geo` and its projections,
`d3-quadtree`, `d3-force`, `d3-chord`, `d3-polygon`, `d3-dispatch`, `d3-timer`,
`d3-delaunay`/Voronoi, `d3-contour` and `d3-scale-chromatic` have all landed — force-directed graphs, chord
diagrams and the full ColorBrewer/matplotlib palette set are available. See
[API-DEVIATIONS.md](API-DEVIATIONS.md) for the three that differ from upstream
in ways worth knowing: `polygon`'s winding sign, `force` being explicitly
stepped rather than frame-driven, and `timer` being a clock rather than a
`requestAnimationFrame` imitation.

No parity percentage is published in `parity.json`. None has been measured —
d3's own tests live in thirty-odd repositories and many of them assert against a
jsdom document — and inventing a number would be worse than admitting the gap.

## License

[MIT](LICENSE). This is an independent re-implementation and is **not**
affiliated with or endorsed by Mike Bostock or the d3 project.
