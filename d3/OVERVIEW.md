# d3 — Overview

`d3` is a single Go module that ports the **computational half of d3** — the
data wrangling, the math, the geometry and the formatting — onto Go's runtime.
It depends only on the standard library and compiles into whatever binary uses
it.

- `github.com/malcolmston/d3/array` — statistics, bisect, transforms, sets,
  bins, and the tick algorithm every scale depends on.
- `github.com/malcolmston/d3/color` — sRGB, HSL, Lab, HCL, and CSS parsing.
- `github.com/malcolmston/d3/interpolate` — numbers, strings, colors, basis
  splines, piecewise.
- `github.com/malcolmston/d3/scale` — the scale families.
- `github.com/malcolmston/d3/path` — the SVG path builder.
- `github.com/malcolmston/d3/shape` — line, area, arc, pie, stack, symbols, and
  the curve family.
- `github.com/malcolmston/d3/format` — d3's number-format specifier grammar.
- `github.com/malcolmston/d3/timefmt` — time intervals plus strftime-style
  parse and format.
- `github.com/malcolmston/d3/hierarchy` — tree, cluster, treemap, partition,
  pack.
- `github.com/malcolmston/d3/ease` — the easing family.
- `github.com/malcolmston/d3/random` — seeded distributions.
- `github.com/malcolmston/d3/dsv` — CSV/TSV with object parsing and type
  inference.

The thing it deliberately does *not* port is the browser. There is no DOM, so
`d3-selection`, `d3-transition`, `d3-drag`, `d3-zoom` and `d3-brush` are absent.
Everything else follows from that one decision.

---

## What this is

d3 is two libraries wearing one name, and almost every tutorial conflates them.

One half is **a document manipulation library**: `d3.select`, the enter/update/
exit join, transitions that tween an attribute over 750ms, drag and zoom
behaviours that translate pointer events into transforms. This half exists to
change a live document, and it is why people believe d3 is hard.

The other half is **a numerical library**. It answers questions like: given a
domain of `[0, 4718]` and 480 pixels of height, where does the value 2300 go?
What tick values would a human choose for that axis? What is the SVG path for a
monotone curve through these forty points? What is the arc geometry for a pie
slice of 17%? How do I lay out this tree so that no two nodes overlap? What does
`.2s` mean as a number format?

None of the second half needs a browser. It is arithmetic, geometry and string
building, and it is by far the more interesting and more reusable of the two.
That is what this module ports.

The consequence is a clean division of labour that JavaScript never quite
achieves:

> **d3 computes. `react` renders. The seam between them is a string of SVG path
> data and a set of scaled coordinates.**

That seam is the architectural story of this port, and it is worth dwelling on,
because it is narrower than anyone expects. A line chart's entire contract
between the two libraries is one string:

```go
d := line.Generate(data)                                 // d3 computes
react.H("path", react.Props{"d": d, "fill": "none"})     // react renders
```

There is no shared object, no callback registry, no lifecycle. The computing
side never learns that a renderer exists, and the rendering side never learns
that the string it is holding came from a scale. Either can be swapped for
something else — write the string into a template, into a file, into an HTTP
response — without the other noticing.

---

## How it works

### The layer cake

The packages form a strict dependency stack, and understanding it explains most
of the API.

```
              shape          hierarchy
                │                │
       ┌────────┼────────┐       │
     path     scale ─────┼───────┘
                │        │
        interpolate    format / timefmt
                │
              color
                │
              array
```

**`array` is the foundation**, and the reason is one function. `Ticks` — d3's
algorithm for choosing round numbers to label an axis with — is what every
quantitative scale calls for `Ticks()` and `Nice()`. Change it and every axis in
every chart changes. It is ported literally from d3-array's `tickSpec`,
including the trick of tracking a *negative* increment to mean `1/n` so that
ticks below 1 come out as exact divisions (`3/10 == 0.3`, not `0.1+0.1+0.1`).
It also carries a Go-specific correction: JavaScript's `Math.round` rounds
halves toward `+∞` while Go's `math.Round` rounds them away from zero, so the
port uses `floor(x+0.5)` internally. Without that, negative domains drift by one
tick.

**`color` and `interpolate` sit under `scale`** because a scale's range is not
necessarily numeric. A sequential color scale interpolates through Lab space,
so the same `Continuous` machinery that maps dollars to pixels maps dollars to
colors — which is why `Continuous` is generic in its range type, `Continuous[R]`,
with `NewLinear()` being the `Continuous[float64]` case.

**`path` sits under `shape`** because every shape generator is, at bottom, a
program that issues `MoveTo` / `LineTo` / `BezierCurveTo` calls and then asks
for the string. In JavaScript that sink is pluggable — `line.context(ctx)`
draws to a canvas instead — and this port keeps only the string sink, because
there is no canvas to draw to and the string is exactly what an SVG `d`
attribute wants.

### Scales: normalize, then interpolate

A continuous scale is two functions composed. First **normalize**: where does
`x` fall in the domain, as a number in `[0, 1]`? Then **interpolate**: what is
the range value at that position?

```
x  ──normalize(d0,d1)──▶  t ∈ [0,1]  ──interpolate(r0,r1)──▶  y
```

Every quantitative scale is this pair with a *transform* wrapped around the
normalize step: `Log` normalizes in log space, `Pow` in `x^k`, `Symlog` in
`sign(x)·log(1+|x|/C)`, `Time` in epoch milliseconds. That is genuinely all the
difference between them, which is why they share one `Continuous` type rather
than being five parallel implementations.

Two consequences that surprise people, both faithfully reproduced:

- **Scales extrapolate.** A value outside the domain produces a value outside
  the range, unless `SetClamp(true)`. This is a feature — it is how you draw a
  point that overflows the axis — and it is regularly mistaken for a bug.
- **Invert exists only when the range is numeric.** You cannot invert a color
  scale, because the mapping is not injective in any useful sense.

The discretizing scales — `Quantize`, `Quantile`, `Threshold` — are routinely
confused with each other, and the distinction is entirely about where the
breakpoints come from. Quantize cuts the *domain* into uniform slices. Quantile
cuts the *data*, so each bucket holds the same number of observations. Threshold
takes the cuts you hand it. Same input, three different pictures.

`Band` and `Point` are the positional specializations of `Ordinal`. A band scale
gives each category a slice of the range with configurable padding, and its
`Bandwidth()` is the width of a bar. Every bar chart ever written is a band
scale on one axis and a linear scale on the other.

### Shapes: a generator is a configured function

A shape generator is an object you configure with accessors and then call with
data. `Line` holds `x`, `y`, `defined` and `curve`; calling it walks the data,
asks each accessor for a number, feeds the points to the curve, and returns the
accumulated path string.

The **curve** is the interesting part, and it is a stream. `curveLinear` emits a
`LineTo` per point. `curveBasis` holds three points of lookahead and emits a
cubic segment whose control points are derived from them. `curveMonotoneX`
computes tangents that cannot overshoot, which is why it is the right choice for
data that must never appear to dip below a value it never had.

That state is why a generator is configured with a curve **factory** rather than
a curve: `Curve(shape.CurveMonotoneX)` stores the constructor, and each
`Generate` builds a fresh curve. The payoff is that a configured generator stays
a pure function from data to a string, and is safe to call from any goroutine.

`Area` is `Line` twice: a top edge forwards and a baseline backwards, joined
and closed. `Arc` is trigonometry plus the corner and pad-angle cases, which is
where most of its code lives. `Pie` computes *angles*, not paths — feeding those
angles to `Arc` is the second half of the operation, and keeping them separate
is what lets the same angles drive a donut, an exploded slice or a label
position.

`Stack` is a different shape of thing entirely: it takes a table and produces
`[y0, y1]` pairs per series per row, with a pluggable order and offset. The
offsets are what turn the same data into a stacked bar chart, a normalized
percentage chart or a streamgraph.

### Hierarchy: annotate, don't rebuild

`hierarchy.New` wraps arbitrary data in nodes carrying `Depth`, `Height`,
`Parent` and `Children`; `Sum`, `Sort`, `Count` and the traversals operate on
that. A layout — `Tree`, `Cluster`, `Treemap`, `Partition`, `Pack` — is then a
pass over the tree that **writes coordinates onto the nodes in place**.

That mutation is deliberate and matches upstream. A layout is not a
transformation into a new structure; it is an annotation of an existing one. It
means you can run `Treemap` and then read `node.X0/Y0/X1/Y1` off the same nodes
you already hold references to, and it means running two layouts over one tree
overwrites the first.

### Format: a grammar, not a printf

`format` implements d3's specifier grammar — `[[fill]align][sign][symbol][0]
[width][,][.precision][~][type]` — which is not `fmt`'s verb syntax and is not
trying to be. `.2f`, `,d`, `.1%`, `$,.2f` and `.3s` are the specifiers that
appear in real charts, and `s` (SI prefix) is the one with no Go equivalent at
all: it renders `1300` as `1.30k` and `0.000001` as `1.00µ`.

The reason this belongs in a charting library rather than being replaced by
`strconv` is axis labels. An axis with ticks at `0, 5000, 10000, 15000` should
be labelled `0, 5k, 10k, 15k`, and choosing that automatically from the tick
step is exactly the job.

One seam is worth flagging: `scale.TickFormat` does **not** currently route
through this package. It carries its own step-derived fixed-point formatter,
written before `format` existed, which covers the axis case correctly but means
an axis label and an explicit `format.New(".2f")` are produced by two different
code paths. Unifying them is a tracked item in [BACKLOG.md](BACKLOG.md).

### The seam, concretely

Nothing in this module knows how its output will be used. A scale returns a
`float64`. A shape generator returns a `string`. A format returns a `string`.
That is the entire interface to the outside world, and it is why the same
computation feeds a `react` tree, an `html/template`, a file on disk or an HTTP
response body without adaptation:

```go
path := line.Generate(series)

react.H("path", react.Props{"d": path})                 // a react tree
fmt.Fprintf(w, `<path d=%q/>`, path)                    // straight to a writer
os.WriteFile("chart.svg", []byte(svg(path)), 0o644)     // a file
```

---

## How to use it

### A line chart, end to end

The flagship use case: a scale positions the data, a shape generator turns the
positions into geometry, and a renderer puts the geometry on a page.

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

	x := scale.NewLinear().SetDomain(0, 5).SetRange(0, w)
	y := scale.NewLinear().SetDomain(0, maxV).SetRange(h, 0).Nice(10)

	line := shape.NewLine[Point]().
		X(func(p Point, _ int, _ []Point) float64 { return x.Scale(p.Day) }).
		Y(func(p Point, _ int, _ []Point) float64 { return y.Scale(p.Value) }).
		Curve(shape.CurveMonotoneX)

	fmt.Println(line.Generate(data))
	// a string of SVG path data, ready for a <path d="…">


	label := y.TickFormat(5)
	for _, t := range y.Ticks(5) {
		fmt.Printf("y=%.1f label=%s\n", y.Scale(t), label(t))
	}
}
```

Note the shape of it: the `y` range is `(h, 0)` and not `(0, h)`, because SVG's
origin is top-left and a chart's is bottom-left. Inverting the range is how
every d3 chart handles that, and it is one of the reasons scales are worth
having at all.

### Handing it to a renderer

```go
import (
	"github.com/malcolmston/d3/shape"
	"github.com/malcolmston/react"
)

func Chart(series []Point) react.Node {
	line := shape.NewLine[Point]().
		X(func(p Point, _ int, _ []Point) float64 { return x.Scale(p.Day) }).
		Y(func(p Point, _ int, _ []Point) float64 { return y.Scale(p.Value) })

	return react.H("svg", react.Props{"viewBox": "0 0 640 480"},
		react.H("path", react.Props{
			"d":      line.Generate(series),
			"fill":   "none",
			"stroke": "currentColor",
		}),
	)
}
```

That is the entire integration. The `d3` packages do not import `react`, and
`react` does not import `d3` — the coupling is a `string`.

### Color and interpolation

```go
c, err := color.Parse("#4682b4")
if err != nil {
	// an unparseable color is a programmer error, and is reported as one
}
fmt.Println(c.RGBA().Hex())            // #4682b4
fmt.Println(color.ToRGB(c).Darker(1))  // a darker steel blue

ramp := interpolate.Number(0, 100)
fmt.Println(ramp(0.5))                 // 50
```

### Statistics

```go
values := []float64{4, 8, 15, 16, 23, 42}

n, _ := array.MeanOf(values)             // 18
m, _ := array.MedianOf(values)           // 15.5
q, _ := array.QuantileOf(values, 0.25)   // 9.75
lo, hi, ok := array.ExtentOf(values)     // 4, 42, true
```

The second result is `ok`, and it is `false` when there was nothing to compute
from. `d3.mean([])` returns `undefined`; Go has no `undefined`, and a silent
zero would render as a real data point.

---

## Why it's better than its predecessor

The predecessor here is d3 in JavaScript, which is an excellent library. This
port is not better at what d3 is for; it is better at a specific subset of
situations:

- **The computation runs on the server, where it belongs.** A chart's data has
  to be fetched, filtered and aggregated somewhere. Doing the scale and shape
  math in the same process, in the same language, against the same types,
  removes an entire serialization boundary — and with it, the class of bug
  where the server and the client disagree about what a date is.
- **Types instead of duck typing.** d3 discovers at runtime whether your datum
  has an `.x`, a `[0]` or a `.length`. Here the datum is a type parameter and a
  missing accessor is a compile error rather than a chart full of `NaN`. The
  cost is one explicit accessor per channel; the benefit is that a renamed field
  breaks the build instead of the chart.
- **`undefined` has an answer.** Every statistic that can fail returns an `ok`
  bool, so an empty dataset is distinguishable from a dataset that legitimately
  measured zero. In JavaScript that distinction is `undefined` versus `0`, and
  it is lost the moment the value is arithmetic'd.
- **No dependency graph.** Standard library only. d3 in Node is thirty-odd
  packages with their own release cadences; here it is one module and one
  version.
- **The output is inert.** A path string is data. It can be cached, diffed,
  written to a file, sent over a wire, or asserted against in a test — none of
  which is comfortable when the output is a mutated DOM node.

**Honest tradeoffs.** This port is *smaller* than d3. `d3-geo` and its
projections, `d3-contour`, `d3-delaunay`, `d3-quadtree`, `d3-force` and
`d3-chord` are not written yet, and a map or a force-directed graph is
consequently not something this module can do today — see
[BACKLOG.md](BACKLOG.md). No parity percentage is published, because none has
been measured and inventing one would be worse than admitting the gap; the
corpus that would produce a real number is the top backlog item. And nothing
here is interactive: there is no hover, no zoom, no brush, and adding them is a
question for whatever renders the output, not for this module.
