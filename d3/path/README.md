# path — Go port of d3-path: a serializer that speaks the CanvasRenderingContext2D drawing

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/path.svg)](https://pkg.go.dev/github.com/malcolmston/d3/path)

Package path is a Go port of d3-path: a serializer that speaks the
CanvasRenderingContext2D drawing vocabulary (moveTo, lineTo, arc, …) but
emits an SVG path data string instead of painting pixels.

It exists because d3's shape generators are written against the canvas API,
yet the overwhelmingly common use is to feed the `d` attribute of an SVG
<path>. d3-path bridges the two, and so does this package. There is no DOM
here and none is needed: a `Path` accumulates commands into a string, and that
string is handed straight to a renderer. With github.com/malcolmston/react
that is the whole integration story —

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/path"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Path` | Path builds an SVG path data string from canvas-style drawing calls. |

<details>
<summary><code>Path</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func New() *Path` | New returns an empty Path that emits full-precision coordinates. |
| `func NewWithPrecision(digits int) *Path` | NewWithPrecision returns an empty Path that rounds every coordinate to digits decimal places, the equivalent of d3.path(digits). |
| `func (p *Path) Arc(x, y, r, startAngle, endAngle float64, counterclockwise bool) *Path` | Arc draws a circular arc centered at (x, y) with radius r, from startAngle to endAngle in radians, sweeping counterclockwise when counterclockwise is… |
| `func (p *Path) ArcTo(x1, y1, x2, y2, r float64) *Path` | ArcTo draws a circular arc of radius r tangent to the line from the current point to (x1, y1) and to the line from (x1, y1) to (x2, y2) — the… |
| `func (p *Path) BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) *Path` | BezierCurveTo draws a cubic Bézier to (x, y) with control points (cp1x, cp1y) and (cp2x, cp2y). |
| `func (p *Path) ClosePath() *Path` | ClosePath closes the current subpath with a straight line back to its start. |
| `func (p *Path) LineTo(x, y float64) *Path` | LineTo draws a straight line from the current point to (x, y). |
| `func (p *Path) MoveTo(x, y float64) *Path` | MoveTo starts a new subpath at (x, y). |
| `func (p *Path) QuadraticCurveTo(cpx, cpy, x, y float64) *Path` | QuadraticCurveTo draws a quadratic Bézier to (x, y) with control point (cpx, cpy). |
| `func (p *Path) Rect(x, y, w, h float64) *Path` | Rect draws an axis-aligned rectangle as its own closed subpath. |
| `func (p *Path) String() string` | String returns the accumulated SVG path data. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/path).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
