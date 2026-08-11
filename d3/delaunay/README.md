# delaunay — Go port of d3-delaunay, and with it of the Delaunator triangulation kernel

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/delaunay.svg)](https://pkg.go.dev/github.com/malcolmston/d3/delaunay)

Package delaunay is a Go port of d3-delaunay, and with it of the Delaunator
triangulation kernel that d3-delaunay wraps.

A `Delaunay` is the Delaunay triangulation of a set of points: the unique
triangulation (for points in general position) in which no point lies inside
the circumcircle of any triangle. That property is not a detail — it is the
definition, it is what makes the triangulation the "right" one for
interpolation and nearest-neighbor work, and it is what this package's tests
verify over random inputs rather than against recorded output.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/delaunay"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Accessor` | Accessor reads one coordinate out of a datum, with the (datum, index, data) signature used throughout this port. |
| `Cell` | Cell pairs a cell polygon with the index of the point that generates it. |
| `Delaunay` | Delaunay is the Delaunay triangulation of a point set, plus the indices that make neighbor and nearest-point queries fast. |
| `Point` | Point is a position in the plane. |
| `Voronoi` | Voronoi is the Voronoi diagram dual to a `Delaunay` triangulation, clipped to a rectangle. |

<details>
<summary><code>Delaunay</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func From[T any](data []T, fx, fy Accessor[T]) *Delaunay` | From returns the triangulation of arbitrary data through coordinate accessors, the analogue of d3.Delaunay.from. |
| `func New(points []Point) *Delaunay` | New returns the Delaunay triangulation of points. |
| `func NewFromCoords(coords []float64) *Delaunay` | NewFromCoords returns the triangulation of an interleaved [x0, y0, x1, y1, …] coordinate slice, which is copied. |
| `func (d *Delaunay) Collinear() bool` | Collinear reports whether the input was collinear (or too small in extent to tell), in which case the coordinates were jittered before triangulating… |
| `func (d *Delaunay) Coords() []float64` | Coords returns the interleaved coordinates, a copy. |
| `func (d *Delaunay) Find(x, y float64) int` | Find returns the index of the input point closest to (x, y), or -1 if there are no points or the query is not finite. |
| `func (d *Delaunay) FindFrom(x, y float64, i int) int` | FindFrom is `Delaunay.Find` starting the search at point i. |
| `func (d *Delaunay) Halfedges() []int` | Halfedges returns the opposite half-edge of every half-edge, or -1 where the half-edge lies on the convex hull. |
| `func (d *Delaunay) Hull() []int` | Hull returns the point indices on the convex hull, counterclockwise. |
| `func (d *Delaunay) HullPolygon() []Point` | HullPolygon returns the closed ring of the convex hull, or nil when there is no hull. |
| `func (d *Delaunay) Inedges() []int` | Inedges returns, for each point, the index of an incoming half-edge, or -1 for a point that was dropped as a duplicate. |
| `func (d *Delaunay) Neighbors(i int) []int` | Neighbors returns the indices of the points sharing a triangulation edge with point i, in no particular order. |
| `func (d *Delaunay) Points() []Point` | Points returns the triangulated positions. |
| `func (d *Delaunay) Render() string` | Render returns SVG path data for every edge of the triangulation, hull included. |
| `func (d *Delaunay) RenderHull() string` | RenderHull returns SVG path data for the closed convex hull. |
| `func (d *Delaunay) RenderPoints(r float64) string` | RenderPoints returns SVG path data drawing a circle of radius r at every point — the scatterplot layer that usually sits on top of a mesh. |
| `func (d *Delaunay) RenderTriangle(i int) string` | RenderTriangle returns SVG path data for the closed outline of triangle i. |
| `func (d *Delaunay) TrianglePolygon(i int) []Point` | TrianglePolygon returns the closed ring of triangle i: four points, the last equal to the first. |
| `func (d *Delaunay) TrianglePolygons() [][]Point` | TrianglePolygons returns the closed ring of every triangle. |
| `func (d *Delaunay) Triangles() []int` | Triangles returns the vertex index of every half-edge: three consecutive entries are one triangle. |
| `func (d *Delaunay) Voronoi(xmin, ymin, xmax, ymax float64) *Voronoi` | Voronoi returns the Voronoi diagram dual to this triangulation, clipped to the given rectangle. |

</details>

<details>
<summary><code>Voronoi</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (v *Voronoi) CellPolygon(i int) []Point` | CellPolygon returns the closed ring of the clipped cell of point i, or nil if the cell is empty (a duplicate point, or a cell entirely outside the… |
| `func (v *Voronoi) CellPolygons() []Cell` | CellPolygons returns every non-empty cell. |
| `func (v *Voronoi) Contains(i int, x, y float64) bool` | Contains reports whether the cell of point i contains (x, y). |
| `func (v *Voronoi) Delaunay() *Delaunay` | Delaunay returns the triangulation this diagram is dual to. |
| `func (v *Voronoi) Extent() (xmin, ymin, xmax, ymax float64)` | Extent returns the clipping rectangle. |
| `func (v *Voronoi) Neighbors(i int) []int` | Neighbors returns the points whose clipped cells share a boundary segment with the cell of point i. |
| `func (v *Voronoi) Render() string` | Render returns SVG path data for the cell boundaries — the mesh of the diagram, with each edge drawn once and clipped to the extent. |
| `func (v *Voronoi) RenderBounds() string` | RenderBounds returns SVG path data for the clipping rectangle. |
| `func (v *Voronoi) RenderCell(i int) string` | RenderCell returns SVG path data for the closed clipped cell of point i, or "" if the cell is empty. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/delaunay).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
