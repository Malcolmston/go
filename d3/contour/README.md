# contour — Go port of d3-contour: isolines and isobands over a rectangular grid of values

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/contour.svg)](https://pkg.go.dev/github.com/malcolmston/d3/contour)

Package contour is a Go port of d3-contour: isolines and isobands over a
rectangular grid of values, and two-dimensional kernel density estimation that
produces such a grid from scattered points.

`Contours` runs marching squares. Given a grid of values and a threshold, it
returns the region where the value is at or above the threshold, as a
GeoJSON-shaped `MultiPolygon` whose exterior rings are wound one way and whose
holes are wound the other. Given several thresholds it returns one
MultiPolygon per threshold, and because the regions are nested — everything
above 20 is inside everything above 10 — the result stacks into the familiar
filled contour map.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/contour"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Accessor` | Accessor reads one value out of a datum, with the (datum, index, data) signature used throughout this port. |
| `ContourDensity` | ContourDensity estimates a two-dimensional density from scattered points and contours it — the density heat map that replaces a scatterplot once… |
| `Contours` | Contours computes the contours of a grid of values. |
| `MultiPolygon` | MultiPolygon is the region at or above one threshold, shaped like the GeoJSON geometry of the same name. |
| `Point` | Point is a position in grid coordinates. |
| `Polygon` | Polygon is an exterior ring followed by its holes. |
| `Ring` | Ring is a closed sequence of points: the last equals the first. |

<details>
<summary><code>ContourDensity</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewContourDensity[T any]() *ContourDensity[T]` | NewContourDensity returns a density estimator over a 960×500 extent with a bandwidth of about 20, a cell size of 4 and roughly 20 thresholds —… |
| `func (c *ContourDensity[T]) Bandwidth(bw float64) *ContourDensity[T]` | Bandwidth sets the standard deviation of the smoothing kernel, in input coordinates. |
| `func (c *ContourDensity[T]) CellSize(size float64) *ContourDensity[T]` | CellSize sets the side of a grid cell in input coordinates. |
| `func (c *ContourDensity[T]) Compute(data []T) []MultiPolygon` | Compute returns one MultiPolygon per density level, in input coordinates. |
| `func (c *ContourDensity[T]) Grid(data []T) (values []float64, n, m int)` | Grid returns the estimated density grid together with its dimensions, in units of weight per square input unit. |
| `func (c *ContourDensity[T]) Size(dx, dy int) *ContourDensity[T]` | Size sets the extent of the estimate in input coordinates. |
| `func (c *ContourDensity[T]) ThresholdCount(n int) *ContourDensity[T]` | ThresholdCount asks for approximately n uniformly spaced density levels. |
| `func (c *ContourDensity[T]) Thresholds(t []float64) *ContourDensity[T]` | Thresholds sets the explicit density levels to contour. |
| `func (c *ContourDensity[T]) Weight(f Accessor[T]) *ContourDensity[T]` | Weight sets the accessor for a datum's weight. |
| `func (c *ContourDensity[T]) X(f Accessor[T]) *ContourDensity[T]` | X sets the accessor for the horizontal coordinate. |
| `func (c *ContourDensity[T]) Y(f Accessor[T]) *ContourDensity[T]` | Y sets the accessor for the vertical coordinate. |

</details>

<details>
<summary><code>Contours</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewContours() *Contours` | NewContours returns a Contours over a 1×1 grid, with smoothing on and the threshold count chosen by Sturges' formula. |
| `func (c *Contours) Compute(values []float64) []MultiPolygon` | Compute returns one MultiPolygon per threshold, in ascending threshold order. |
| `func (c *Contours) ComputeOne(values []float64, threshold float64) MultiPolygon` | ComputeOne returns the single contour at the given threshold, ignoring the configured thresholds. |
| `func (c *Contours) Size(dx, dy int) *Contours` | Size sets the grid dimensions in cells. |
| `func (c *Contours) Smooth(on bool) *Contours` | Smooth turns linear interpolation of ring vertices along cell edges on or off. |
| `func (c *Contours) ThresholdCount(n int) *Contours` | ThresholdCount asks for approximately n uniformly spaced thresholds, chosen with the same nice-round-number algorithm the scales use (`array.Ticks`),… |
| `func (c *Contours) Thresholds(t []float64) *Contours` | Thresholds sets the explicit list of thresholds, one contour per entry. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/contour).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
