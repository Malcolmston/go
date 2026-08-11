# quadtree — Go port of d3-quadtree — a two-dimensional recursive spatial subdivision

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/quadtree.svg)](https://pkg.go.dev/github.com/malcolmston/d3/quadtree)

Package quadtree is a Go port of d3-quadtree — a two-dimensional recursive
spatial subdivision. It is the index that makes "which of these ten thousand
points is nearest to my cursor?" and "what is the net repulsion from every
other point?" cheap enough to ask in a loop.

A quadtree partitions a square region into four equal quadrants, and each
quadrant that ends up holding more than one point is subdivided again. The
result is a tree whose leaves hold the data and whose internal nodes stand for
regions of space, so a query that can rule out a region rules out everything
inside it in one comparison.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/quadtree"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Types

| Type | What it is |
| --- | --- |
| `Node` | Node is one vertex of a quadtree: either an internal node standing for a square region, or a leaf holding one or more coincident data points. |
| `Quadtree` | Quadtree indexes values of type T by a point derived from each of them. |
| `VisitAfterFunc` | VisitAfterFunc is the callback for `Quadtree.VisitAfter`. |
| `VisitFunc` | VisitFunc is the callback for `Quadtree.Visit`. |

<details>
<summary><code>Node</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (n *Node[T]) IsLeaf() bool` | IsLeaf reports whether this node holds data rather than quadrants. |

</details>

<details>
<summary><code>Quadtree</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func New[T comparable](x, y func(T) float64) *Quadtree[T]` | New returns an empty quadtree that reads each datum's coordinates with the given accessors. |
| `func (q *Quadtree[T]) Add(d T) *Quadtree[T]` | Add inserts one datum and returns the receiver. |
| `func (q *Quadtree[T]) AddAll(data []T) *Quadtree[T]` | AddAll inserts every datum and returns the receiver. |
| `func (q *Quadtree[T]) Copy() *Quadtree[T]` | Copy returns a deep copy of the tree: the same data in the same structure, with every node newly allocated, so adding to or removing from the copy… |
| `func (q *Quadtree[T]) Cover(x, y float64) *Quadtree[T]` | Cover expands the tree so that the given point lies inside its extent, and returns the receiver. |
| `func (q *Quadtree[T]) Data() []T` | Data returns every datum in the tree. |
| `func (q *Quadtree[T]) Extent() (x0, y0, x1, y1 float64, ok bool)` | Extent returns the square the tree currently covers, and whether it covers anything at all. |
| `func (q *Quadtree[T]) Find(x, y float64) (T, bool)` | Find returns the datum closest to (x, y), and whether the tree had one. |
| `func (q *Quadtree[T]) FindWithin(x, y, radius float64) (T, bool)` | FindWithin is `Quadtree.Find` limited to a search radius: it returns the closest datum within radius of (x, y), and false if there is none. |
| `func (q *Quadtree[T]) Remove(d T) *Quadtree[T]` | Remove deletes one datum from the tree and returns the receiver. |
| `func (q *Quadtree[T]) RemoveAll(data []T) *Quadtree[T]` | RemoveAll deletes every listed datum and returns the receiver. |
| `func (q *Quadtree[T]) Root() *Node[T]` | Root returns the root node, or nil for an empty tree. |
| `func (q *Quadtree[T]) SetExtent(x0, y0, x1, y1 float64) *Quadtree[T]` | SetExtent expands the tree to cover the given rectangle's corners and returns the receiver. |
| `func (q *Quadtree[T]) SetX(x func(T) float64) *Quadtree[T]` | SetX replaces the x accessor and returns the receiver. |
| `func (q *Quadtree[T]) SetY(y func(T) float64) *Quadtree[T]` | SetY replaces the y accessor and returns the receiver. |
| `func (q *Quadtree[T]) Size() int` | Size returns the number of data in the tree, counting every point of a coincident chain. |
| `func (q *Quadtree[T]) Visit(visit VisitFunc[T]) *Quadtree[T]` | Visit traverses the tree top-down in pre-order, calling visit for each node before its children, and returns the receiver. |
| `func (q *Quadtree[T]) VisitAfter(visit VisitAfterFunc[T]) *Quadtree[T]` | VisitAfter traverses the tree bottom-up in post-order, calling visit for each node only after every one of its children, and returns the receiver. |
| `func (q *Quadtree[T]) X() func(T) float64` | X returns the x accessor. |
| `func (q *Quadtree[T]) Y() func(T) float64` | Y returns the y accessor. |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/quadtree).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
