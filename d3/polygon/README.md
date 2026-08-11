# polygon — Go port of d3-polygon: the small set of geometric primitives that operate on a closed

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/polygon.svg)](https://pkg.go.dev/github.com/malcolmston/d3/polygon)

Package polygon is a Go port of d3-polygon: the small set of geometric
primitives that operate on a closed ring of two-dimensional points — signed
area, centroid, convex hull, point containment and perimeter length.

It is deliberately tiny and has no configuration, no generators and no output
format. It computes numbers about a ring of points, and something else draws
them. Everything here is exact arithmetic on float64s, so the results are
testable by value rather than by eyeball.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/polygon"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Point` | Point is a vertex. |
| `Polygon` | Polygon is a closed ring of vertices in order. |

<details>
<summary><code>Polygon</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Hull(points []Point) Polygon` | Hull returns the convex hull of a cloud of points, computed with Andrew's monotone chain algorithm: sort lexicographically by x then y, sweep once to… |
| `func (p Polygon) Area() float64` | Area returns the signed area of the ring. |
| `func (p Polygon) Centroid() Point` | Centroid returns the centroid of the ring's *enclosed area* — the balance point of a uniform lamina with this outline, not the mean of the vertices. |
| `func (p Polygon) Contains(q Point) bool` | Contains reports whether the point lies inside the ring, by the even-odd (crossing-number) rule: a ray is cast from the point and the number of edges… |
| `func (p Polygon) Length() float64` | Length returns the perimeter: the total length of every edge, including the implied closing edge from the last vertex back to the first. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/polygon).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
