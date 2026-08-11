# shape — Go port of d3-shape: the generators that turn data into the SVG path strings a chart

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/shape.svg)](https://pkg.go.dev/github.com/malcolmston/d3/shape)

Package shape is a Go port of d3-shape: the generators that turn data into the
SVG path strings a chart is actually made of — lines, areas, pie and donut
arcs, stacked series and scatterplot symbols.

Every generator's output is a string of SVG path data produced by
`github.com/malcolmston/d3/path`, so rendering is a one-liner with
github.com/malcolmston/react:

```go
line := shape.NewLine[Point]().X(func(d Point, _ int, _ []Point) float64 { return xs(d.T) }).
	Y(func(d Point, _ int, _ []Point) float64 { return ys(d.V) })
react.H("path", react.Props{"d": line.Generate(data), "fill": "none", "stroke": "steelblue"})
```

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/shape"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func OffsetDiverging(values [][][2]float64, order []int)` | OffsetDiverging splits each column at zero: positive values stack upward from the baseline and negative values stack downward, so a chart of gains… |
| `func OffsetExpand(values [][][2]float64, order []int)` | OffsetExpand normalizes each column so the layers sum to 1, turning a stacked chart into a share-of-total chart. |
| `func OffsetNone(values [][][2]float64, order []int)` | OffsetNone stacks each layer on top of the one below, starting at zero. |
| `func OffsetSilhouette(values [][][2]float64, order []int)` | OffsetSilhouette centers the stack on zero: each column's baseline is minus half its total, so the band is symmetric about the axis. |
| `func OffsetWiggle(values [][][2]float64, order []int)` | OffsetWiggle shifts each column's baseline to minimize the total weighted slope of the layers — Byron & Wattenberg's streamgraph algorithm. |
| `func OrderAppearance(values [][][2]float64) []int` | OrderAppearance orders series by when they peak, earliest first. |
| `func OrderAscending(values [][][2]float64) []int` | OrderAscending puts the smallest series (by total value) at the bottom, which keeps the most volatile layers near the baseline where they are easiest… |
| `func OrderDescending(values [][][2]float64) []int` | OrderDescending puts the largest series at the bottom. |
| `func OrderInsideOut(values [][][2]float64) []int` | OrderInsideOut puts the series that peak earliest in the middle of the stack and alternates outward, balancing the two halves by total. |
| `func OrderNone(values [][][2]float64) []int` | OrderNone stacks in key order. |
| `func OrderReverse(values [][][2]float64) []int` | OrderReverse stacks in reverse key order. |

### Types

| Type | What it is |
| --- | --- |
| `Accessor` | Accessor reads one coordinate out of a datum. |
| `Arc` | Arc generates the path for a circular or annular sector — the wedge of a pie chart, the segment of a donut, the bar of a radial histogram. |
| `ArcAccessor` | ArcAccessor computes one property of an arc from its datum. |
| `ArcDatum` | ArcDatum describes one wedge for `Arc`. |
| `Area` | Area generates the path data for the region between two lines: a topline (x1, y1) and a baseline (x0, y0). |
| `AreaRadial` | AreaRadial generates an area in polar coordinates, bounded by an inner and an outer radius over a range of angles — a radial stacked chart, or the… |
| `Context` | Context is the drawing surface a `Curve` writes into. |
| `Curve` | Curve is the streaming interpolator a `Line` or `Area` pushes points through. |
| `CurveFactory` | CurveFactory binds a curve to the context it draws into. |
| `DefinedFunc` | DefinedFunc reports whether a datum should be drawn. |
| `Line` | Line generates the path data for a polyline (or a smooth curve, depending on the `Curve`) through a data slice. |
| `LineRadial` | LineRadial generates a line in polar coordinates: each datum contributes an angle and a radius, drawn about the origin with angle 0 pointing up and… |
| `Pie` | Pie turns a slice of data into the angles of a pie or donut chart. |
| `PieArc` | PieArc is one slice computed by `Pie`: the original datum, its value, and the angles `Arc` needs to draw it. |
| `Series` | Series is one layer of a stacked chart — one key's values across all the data. |
| `SeriesPoint` | SeriesPoint is one datum's contribution to one series: the baseline and topline the shape generator will draw. |
| `Stack` | Stack turns wide data — one row per x, one field per layer — into the baselines and toplines of a stacked chart. |
| `StackOffset` | StackOffset repositions the layers in place, given the values matrix (series × datum × [lower, upper]) and the stacking order. |
| `StackOrder` | StackOrder chooses the bottom-to-top order of the layers, returning series indices. |
| `Symbol` | Symbol generates the path data for a single marker. |
| `SymbolType` | SymbolType draws one scatterplot marker, centered on the origin, with the given area in square pixels. |

<details>
<summary><code>Arc</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewArc() *Arc` | NewArc returns an Arc whose radii and angles come from the `ArcDatum`, with no corner rounding. |
| `func (a *Arc) Centroid(d ArcDatum) (x, y float64)` | Centroid returns the midpoint of the arc: halfway between the two radii, at the bisecting angle. |
| `func (a *Arc) CornerRadius(v float64) *Arc` | CornerRadius sets a constant corner radius. |
| `func (a *Arc) CornerRadiusFunc(f ArcAccessor) *Arc` | CornerRadiusFunc sets a per-datum corner radius. |
| `func (a *Arc) Digits(n int) *Arc` | Digits rounds every emitted coordinate to n decimal places. |
| `func (a *Arc) EndAngle(v float64) *Arc` | EndAngle sets a constant end angle in radians. |
| `func (a *Arc) EndAngleFunc(f ArcAccessor) *Arc` | EndAngleFunc sets a per-datum end angle. |
| `func (a *Arc) Generate(d ArcDatum) string` | Generate returns the SVG path data for one arc. |
| `func (a *Arc) InnerRadius(v float64) *Arc` | InnerRadius sets a constant inner radius; 0 makes a pie wedge rather than a donut segment. |
| `func (a *Arc) InnerRadiusFunc(f ArcAccessor) *Arc` | InnerRadiusFunc sets a per-datum inner radius. |
| `func (a *Arc) OuterRadius(v float64) *Arc` | OuterRadius sets a constant outer radius. |
| `func (a *Arc) OuterRadiusFunc(f ArcAccessor) *Arc` | OuterRadiusFunc sets a per-datum outer radius, which is how a radial bar chart encodes its value. |
| `func (a *Arc) PadAngle(v float64) *Arc` | PadAngle sets a constant pad angle in radians. |
| `func (a *Arc) PadAngleFunc(f ArcAccessor) *Arc` | PadAngleFunc sets a per-datum pad angle. |
| `func (a *Arc) PadRadius(v float64) *Arc` | PadRadius sets the radius at which the pad angle is measured. |
| `func (a *Arc) PadRadiusFunc(f ArcAccessor) *Arc` | PadRadiusFunc sets a per-datum pad radius; nil restores the default. |
| `func (a *Arc) StartAngle(v float64) *Arc` | StartAngle sets a constant start angle in radians. |
| `func (a *Arc) StartAngleFunc(f ArcAccessor) *Arc` | StartAngleFunc sets a per-datum start angle. |

</details>

<details>
<summary><code>Area</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewArea[T any]() *Area[T]` | NewArea returns an Area with a linear curve, all points defined, a baseline pinned to y = 0, and x1/y1 tracking x0/y0 until configured. |
| `func (a *Area[T]) Curve(f CurveFactory) *Area[T]` | Curve sets the interpolation between points. |
| `func (a *Area[T]) Defined(f DefinedFunc[T]) *Area[T]` | Defined sets the predicate that decides which data are drawn. |
| `func (a *Area[T]) Digits(n int) *Area[T]` | Digits rounds every emitted coordinate to n decimal places. |
| `func (a *Area[T]) Generate(data []T) string` | Generate returns the SVG path data for data, or "" when nothing is drawn. |
| `func (a *Area[T]) LineX0() *Line[T]` | LineX0 returns a `Line` along the left edge of a horizontal band. |
| `func (a *Area[T]) LineX1() *Line[T]` | LineX1 returns a `Line` along the right edge of a horizontal band. |
| `func (a *Area[T]) LineY0() *Line[T]` | LineY0 returns a `Line` along the area's baseline. |
| `func (a *Area[T]) LineY1() *Line[T]` | LineY1 returns a `Line` along the area's topline. |
| `func (a *Area[T]) X(f Accessor[T]) *Area[T]` | X sets both horizontal accessors, so the two lines share an x — the common case, where the area is a filled line chart. |
| `func (a *Area[T]) X0(f Accessor[T]) *Area[T]` | X0 sets the baseline's horizontal accessor. |
| `func (a *Area[T]) X0Const(v float64) *Area[T]` | X0Const pins the baseline's horizontal coordinate to a constant. |
| `func (a *Area[T]) X1(f Accessor[T]) *Area[T]` | X1 sets the topline's horizontal accessor. |
| `func (a *Area[T]) X1Const(v float64) *Area[T]` | X1Const pins the topline's horizontal coordinate to a constant. |
| `func (a *Area[T]) XConst(v float64) *Area[T]` | XConst pins both horizontal accessors to a constant. |
| `func (a *Area[T]) Y(f Accessor[T]) *Area[T]` | Y sets both vertical accessors. |
| `func (a *Area[T]) Y0(f Accessor[T]) *Area[T]` | Y0 sets the baseline's vertical accessor. |
| `func (a *Area[T]) Y0Const(v float64) *Area[T]` | Y0Const pins the baseline to a constant height, typically the y coordinate of the zero line. |
| `func (a *Area[T]) Y1(f Accessor[T]) *Area[T]` | Y1 sets the topline's vertical accessor — for a filled line chart, the data. |
| `func (a *Area[T]) Y1Const(v float64) *Area[T]` | Y1Const pins the topline to a constant height. |
| `func (a *Area[T]) YConst(v float64) *Area[T]` | YConst pins both vertical accessors to a constant. |

</details>

<details>
<summary><code>AreaRadial</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewAreaRadial[T any]() *AreaRadial[T]` | NewAreaRadial returns a radial area with a linear curve and an inner radius of 0. |
| `func (a *AreaRadial[T]) Angle(f Accessor[T]) *AreaRadial[T]` | Angle sets both angular accessors. |
| `func (a *AreaRadial[T]) AngleConst(v float64) *AreaRadial[T]` | AngleConst pins both angular accessors to a constant. |
| `func (a *AreaRadial[T]) Curve(f CurveFactory) *AreaRadial[T]` | Curve sets the interpolation, wrapped so the curve receives Cartesian points. |
| `func (a *AreaRadial[T]) Defined(f DefinedFunc[T]) *AreaRadial[T]` | Defined sets the predicate that decides which data are drawn. |
| `func (a *AreaRadial[T]) Digits(n int) *AreaRadial[T]` | Digits rounds every emitted coordinate to n decimal places. |
| `func (a *AreaRadial[T]) EndAngle(f Accessor[T]) *AreaRadial[T]` | EndAngle sets the angular accessor of the outer edge. |
| `func (a *AreaRadial[T]) Generate(data []T) string` | Generate returns the SVG path data for data, or "" when nothing is drawn. |
| `func (a *AreaRadial[T]) InnerRadius(f Accessor[T]) *AreaRadial[T]` | InnerRadius sets the accessor for the inner boundary. |
| `func (a *AreaRadial[T]) InnerRadiusConst(v float64) *AreaRadial[T]` | InnerRadiusConst pins the inner boundary to a constant. |
| `func (a *AreaRadial[T]) OuterRadius(f Accessor[T]) *AreaRadial[T]` | OuterRadius sets the accessor for the outer boundary — the data. |
| `func (a *AreaRadial[T]) OuterRadiusConst(v float64) *AreaRadial[T]` | OuterRadiusConst pins the outer boundary to a constant. |
| `func (a *AreaRadial[T]) Radius(f Accessor[T]) *AreaRadial[T]` | Radius sets both radial accessors. |
| `func (a *AreaRadial[T]) RadiusConst(v float64) *AreaRadial[T]` | RadiusConst pins both radial accessors to a constant. |
| `func (a *AreaRadial[T]) StartAngle(f Accessor[T]) *AreaRadial[T]` | StartAngle sets the angular accessor of the inner edge. |

</details>

<details>
<summary><code>Context</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func PathContext(p *path.Path) Context` | PathContext returns a `Context` that draws into p. |

</details>

<details>
<summary><code>Curve</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func CurveBasis(ctx Context) Curve` | CurveBasis draws a cubic uniform B-spline through the points. |
| `func CurveBasisClosed(ctx Context) Curve` | CurveBasisClosed draws a closed cubic B-spline: the control polygon wraps around, so the first three points are replayed at the end to close the loop… |
| `func CurveBasisOpen(ctx Context) Curve` | CurveBasisOpen draws the same B-spline as `CurveBasis` but omits the straight run-in and run-out, so the curve starts and ends at the spline's… |
| `func CurveBundle(ctx Context) Curve` | CurveBundle is `CurveBundleFactory` with d3's default beta of 0.85. |
| `func CurveCardinal(ctx Context) Curve` | CurveCardinal is `CurveCardinalFactory` with d3's default tension of 0. |
| `func CurveCardinalClosed(ctx Context) Curve` | CurveCardinalClosed is `CurveCardinalClosedFactory` with tension 0. |
| `func CurveCardinalOpen(ctx Context) Curve` | CurveCardinalOpen is `CurveCardinalOpenFactory` with tension 0. |
| `func CurveCatmullRom(ctx Context) Curve` | CurveCatmullRom is `CurveCatmullRomFactory` with d3's default alpha of 0.5, the centripetal parameterization. |
| `func CurveCatmullRomClosed(ctx Context) Curve` | CurveCatmullRomClosed is `CurveCatmullRomClosedFactory` with alpha 0.5. |
| `func CurveCatmullRomOpen(ctx Context) Curve` | CurveCatmullRomOpen is `CurveCatmullRomOpenFactory` with alpha 0.5. |
| `func CurveLinear(ctx Context) Curve` | CurveLinear draws straight line segments between points. |
| `func CurveLinearClosed(ctx Context) Curve` | CurveLinearClosed draws straight segments and closes the loop back to the first point — a polygon rather than a polyline. |
| `func CurveMonotoneX(ctx Context) Curve` | CurveMonotoneX draws a cubic spline that preserves monotonicity in y as a function of x: where the data rises the curve rises, and it never… |
| `func CurveMonotoneY(ctx Context) Curve` | CurveMonotoneY is `CurveMonotoneX` with the axes exchanged: it preserves monotonicity in x as a function of y, for charts whose independent variable… |
| `func CurveNatural(ctx Context) Curve` | CurveNatural draws a natural cubic spline: the interpolating cubic with continuous second derivatives and zero curvature at both ends. |
| `func CurveStep(ctx Context) Curve` | CurveStep transitions midway between points (t = 0.5). |
| `func CurveStepAfter(ctx Context) Curve` | CurveStepAfter holds each value until the next x, so each point's value extends to the right (t = 1). |
| `func CurveStepBefore(ctx Context) Curve` | CurveStepBefore holds the previous value until the next x, so each point's value begins at that point (t = 0). |

</details>

<details>
<summary><code>CurveFactory</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func CurveBundleFactory(beta float64) CurveFactory` | CurveBundleFactory returns a straightened B-spline: every point is blended toward the straight line from the first point to the last, by 1-beta. |
| `func CurveCardinalClosedFactory(tension float64) CurveFactory` | CurveCardinalClosedFactory returns a closed cardinal spline: the first three points are replayed so the loop joins smoothly. |
| `func CurveCardinalFactory(tension float64) CurveFactory` | CurveCardinalFactory returns a cardinal spline with the given tension. |
| `func CurveCardinalOpenFactory(tension float64) CurveFactory` | CurveCardinalOpenFactory returns a cardinal spline that omits the first and last segments, so the curve spans only the interior points. |
| `func CurveCatmullRomClosedFactory(alpha float64) CurveFactory` | CurveCatmullRomClosedFactory returns a closed Catmull–Rom spline. |
| `func CurveCatmullRomFactory(alpha float64) CurveFactory` | CurveCatmullRomFactory returns a Catmull–Rom spline with the given alpha. |
| `func CurveCatmullRomOpenFactory(alpha float64) CurveFactory` | CurveCatmullRomOpenFactory returns a Catmull–Rom spline over the interior points only. |
| `func CurveRadialFactory(f CurveFactory) CurveFactory` | CurveRadialFactory wraps a curve so that points arrive as (angle, radius) and are drawn in Cartesian space. |
| `func CurveStepFactory(t float64) CurveFactory` | CurveStepFactory returns a piecewise-constant curve whose vertical transition happens a fraction t of the way between each pair of points. |

</details>

<details>
<summary><code>Line</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLine[T any]() *Line[T]` | NewLine returns a Line with a linear curve, all points defined, and X and Y returning 0 until configured. |
| `func (l *Line[T]) Curve(f CurveFactory) *Line[T]` | Curve sets the interpolation between points. |
| `func (l *Line[T]) Defined(f DefinedFunc[T]) *Line[T]` | Defined sets the predicate that decides which data are drawn. |
| `func (l *Line[T]) Digits(n int) *Line[T]` | Digits rounds every emitted coordinate to n decimal places. |
| `func (l *Line[T]) Generate(data []T) string` | Generate returns the SVG path data for data, or "" when nothing is drawn. |
| `func (l *Line[T]) X(f Accessor[T]) *Line[T]` | X sets the accessor for the horizontal coordinate. |
| `func (l *Line[T]) XConst(v float64) *Line[T]` | XConst pins the horizontal coordinate to a constant. |
| `func (l *Line[T]) Y(f Accessor[T]) *Line[T]` | Y sets the accessor for the vertical coordinate. |
| `func (l *Line[T]) YConst(v float64) *Line[T]` | YConst pins the vertical coordinate to a constant. |

</details>

<details>
<summary><code>LineRadial</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLineRadial[T any]() *LineRadial[T]` | NewLineRadial returns a radial line with a linear curve. |
| `func (l *LineRadial[T]) Angle(f Accessor[T]) *LineRadial[T]` | Angle sets the accessor for the angle in radians. |
| `func (l *LineRadial[T]) AngleConst(v float64) *LineRadial[T]` | AngleConst pins the angle to a constant. |
| `func (l *LineRadial[T]) Curve(f CurveFactory) *LineRadial[T]` | Curve sets the interpolation, automatically wrapped so the curve still receives Cartesian points. |
| `func (l *LineRadial[T]) Defined(f DefinedFunc[T]) *LineRadial[T]` | Defined sets the predicate that decides which data are drawn. |
| `func (l *LineRadial[T]) Digits(n int) *LineRadial[T]` | Digits rounds every emitted coordinate to n decimal places. |
| `func (l *LineRadial[T]) Generate(data []T) string` | Generate returns the SVG path data for data, or "" when nothing is drawn. |
| `func (l *LineRadial[T]) Radius(f Accessor[T]) *LineRadial[T]` | Radius sets the accessor for the distance from the origin. |
| `func (l *LineRadial[T]) RadiusConst(v float64) *LineRadial[T]` | RadiusConst pins the radius to a constant. |

</details>

<details>
<summary><code>Pie</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewPie[T any]() *Pie[T]` | NewPie returns a Pie covering a full turn clockwise from twelve o'clock, with slices sorted by descending value and no padding. |
| `func (p *Pie[T]) EndAngle(v float64) *Pie[T]` | EndAngle sets a constant end angle. |
| `func (p *Pie[T]) EndAngleFunc(f func(data []T) float64) *Pie[T]` | EndAngleFunc computes the end angle from the whole data slice. |
| `func (p *Pie[T]) Generate(data []T) []PieArc[T]` | Generate computes the slices. |
| `func (p *Pie[T]) PadAngle(v float64) *Pie[T]` | PadAngle sets a constant pad angle in radians. |
| `func (p *Pie[T]) PadAngleFunc(f func(data []T) float64) *Pie[T]` | PadAngleFunc computes the pad angle from the whole data slice. |
| `func (p *Pie[T]) Sort(f func(a, b T) int) *Pie[T]` | Sort orders the slices by comparing data, and disables `Pie.SortValues`. |
| `func (p *Pie[T]) SortValues(f func(a, b float64) int) *Pie[T]` | SortValues orders the slices by comparing computed values, and disables `Pie.Sort`. |
| `func (p *Pie[T]) StartAngle(v float64) *Pie[T]` | StartAngle sets a constant start angle in radians. |
| `func (p *Pie[T]) StartAngleFunc(f func(data []T) float64) *Pie[T]` | StartAngleFunc computes the start angle from the whole data slice. |
| `func (p *Pie[T]) Value(f Accessor[T]) *Pie[T]` | Value sets the accessor for each datum's magnitude. |

</details>

<details>
<summary><code>PieArc</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (a PieArc[T]) Arc() ArcDatum` | Arc converts the slice into the `ArcDatum` that `Arc.Generate` consumes. |

</details>

<details>
<summary><code>Stack</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewStack[T any]() *Stack[T]` | NewStack returns a Stack with no keys, input order and no offset — the conventional stacked chart, where each layer sits on the one below. |
| `func (s *Stack[T]) Generate(data []T) []Series[T]` | Generate computes the series, in key order. |
| `func (s *Stack[T]) Keys(keys ...string) *Stack[T]` | Keys sets the layers, bottom to top before any reordering. |
| `func (s *Stack[T]) Offset(o StackOffset) *Stack[T]` | Offset sets the baseline treatment. |
| `func (s *Stack[T]) Order(o StackOrder) *Stack[T]` | Order sets the layer order. |
| `func (s *Stack[T]) Value(f func(d T, key string, i int, data []T) float64) *Stack[T]` | Value sets the accessor for a datum's contribution to one key. |

</details>

<details>
<summary><code>Symbol</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewSymbol() *Symbol` | NewSymbol returns a circle of area 64, d3's defaults. |
| `func (s *Symbol) Digits(n int) *Symbol` | Digits rounds every emitted coordinate to n decimal places. |
| `func (s *Symbol) Generate() string` | Generate returns the SVG path data for the marker. |
| `func (s *Symbol) Size(v float64) *Symbol` | Size sets the marker's area in square pixels. |
| `func (s *Symbol) Type(t SymbolType) *Symbol` | Type sets the marker shape. |

</details>

### Variables

`SymbolsFill`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/shape).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
