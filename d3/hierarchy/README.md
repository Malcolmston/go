# hierarchy — Go port of d3-hierarchy — the part of D3 that turns tree-shaped data into coordinates

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/hierarchy.svg)](https://pkg.go.dev/github.com/malcolmston/d3/hierarchy)

Package hierarchy is a Go port of d3-hierarchy — the part of D3 that turns
tree-shaped data into coordinates a renderer can draw. It contains no drawing
code and no DOM: every layout in here reads a tree of `Node` values and writes
numbers back onto those nodes, which something else (in this repo, the React
port emitting SVG) is free to turn into pixels.

The starting point is always a `Node`. You build one either from nested data
with `Hierarchy`, which walks a children accessor you supply:

```go
root := hierarchy.Hierarchy(data, func(d *Dir) []*Dir { return d.Subdirs })
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
import "github.com/malcolmston/d3/hierarchy"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Binary[T any](parent *Node[T], x0, y0, x1, y1 float64)` | Binary recursively splits the children into two groups of as near as possible equal value and divides the rectangle along its longer axis between… |
| `func DefaultSeparation[T any](a, b *Node[T]) float64` | DefaultSeparation is the separation function both `Tree` and `Cluster` use unless told otherwise: siblings are kept 1 apart, cousins 2 apart. |
| `func Dice[T any](parent *Node[T], x0, y0, x1, y1 float64)` | Dice lays the children out side by side across the full height of the parent. |
| `func Resquarify[T any](parent *Node[T], x0, y0, x1, y1 float64)` | Resquarify is `Squarify` with the row structure remembered between runs. |
| `func Slice[T any](parent *Node[T], x0, y0, x1, y1 float64)` | Slice stacks the children top to bottom across the full width of the parent — the transpose of `Dice`. |
| `func SliceDice[T any](parent *Node[T], x0, y0, x1, y1 float64)` | SliceDice alternates `Slice` at even depths with `Dice` at odd ones. |
| `func Squarify[T any](parent *Node[T], x0, y0, x1, y1 float64)` | Squarify tiles with the squarified treemap algorithm at the golden ratio, producing rectangles as close to square as the algorithm can manage. |

### Types

| Type | What it is |
| --- | --- |
| `Circle` | Circle is a circle in layout space, used by `Enclose` and internally by `Pack`. |
| `Cluster` | Cluster is the dendrogram layout: every leaf sits on the same line at the far end of the drawing, and every internal node is centred over its… |
| `Link` | Link is one parent→child edge, as produced by `Node.Links`. |
| `Node` | Node is one vertex of a hierarchy, generic in the caller's data type. |
| `Pack` | Pack is the circle-packing (enclosure) layout: every node becomes a circle, leaves are sized by Value, and each parent is the smallest circle… |
| `Partition` | Partition is the icicle layout: each depth of the tree gets a band of equal thickness, and within a band each node's extent is proportional to its… |
| `Tile` | Tile is a treemap tiling method: given a parent whose rectangle has already been decided (and shrunk by any padding), it must set X0, Y0, X1, Y1 on… |
| `Tree` | Tree lays out a hierarchy with the Reingold–Tilford "tidy" algorithm, in the linear-time formulation of Buchheim, Jünger and Leipert. |
| `Treemap` | Treemap recursively subdivides a rectangle so that every node's area is proportional to its Value. |

<details>
<summary><code>Circle</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Enclose(circles []Circle) Circle` | Enclose returns the smallest circle that contains all of the given circles, or the zero Circle if there are none. |

</details>

<details>
<summary><code>Cluster</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCluster[T any]() *Cluster[T]` | NewCluster returns a cluster layout with D3's defaults: `DefaultSeparation` and a size of 1×1. |
| `func (c *Cluster[T]) Layout(root *Node[T]) *Node[T]` | Layout positions the subtree rooted at root and returns it. |
| `func (c *Cluster[T]) NodeSize(width, height float64) *Cluster[T]` | NodeSize fixes the per-node spacing and cancels any previous Size. |
| `func (c *Cluster[T]) Separation(f func(a, b *Node[T]) float64) *Cluster[T]` | Separation sets the gap between adjacent leaves. |
| `func (c *Cluster[T]) Size(width, height float64) *Cluster[T]` | Size scales the finished layout to fit a width×height rectangle and cancels any previous NodeSize. |

</details>

<details>
<summary><code>Node</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Hierarchy[T any](data T, children func(T) []T) *Node[T]` | Hierarchy builds a tree from nested data by repeatedly applying a children accessor, then computes Depth and Height for every node. |
| `func Stratify[T any](rows []T, id func(T) string, parentID func(T) string) (*Node[T], error)` | Stratify builds a tree from flat rows joined on id and parent id — the shape you get from a CSV export, a database query, or an org chart. |
| `func (n *Node[T]) Ancestors() []*Node[T]` | Ancestors returns this node and every proper ancestor, starting here and ending at the root. |
| `func (n *Node[T]) Copy() *Node[T]` | Copy returns a new tree with the same shape and the same Data values, rooted at a copy of this node. |
| `func (n *Node[T]) Count() *Node[T]` | Count sets every leaf's Value to 1 and every internal node's Value to the number of leaves beneath it. |
| `func (n *Node[T]) Descendants() []*Node[T]` | Descendants returns this node and every node beneath it, in breadth-first order (the same order as `Node.Each`). |
| `func (n *Node[T]) Each(f func(*Node[T])) *Node[T]` | Each invokes f for every node in the subtree in breadth-first order: the node itself, then its children, then its grandchildren. |
| `func (n *Node[T]) EachAfter(f func(*Node[T])) *Node[T]` | EachAfter invokes f for every node in post-order: a node after all of its descendants. |
| `func (n *Node[T]) EachBefore(f func(*Node[T])) *Node[T]` | EachBefore invokes f for every node in pre-order: a node before any of its descendants. |
| `func (n *Node[T]) Find(match func(*Node[T]) bool) *Node[T]` | Find returns the first node in the subtree for which match reports true, searching breadth-first, or nil if there is none. |
| `func (n *Node[T]) Leaves() []*Node[T]` | Leaves returns every node in the subtree with no children, in breadth-first order. |
| `func (n *Node[T]) Links() []Link[T]` | Links returns one `Link` per edge in the subtree — that is, one per node other than this one, pairing it with its parent. |
| `func (n *Node[T]) Path(end *Node[T]) []*Node[T]` | Path returns the shortest path from this node to end through their least common ancestor: up from here to the ancestor, then down to end. |
| `func (n *Node[T]) Sort(cmp func(a, b *Node[T]) int) *Node[T]` | Sort sorts every node's children in place using cmp, which follows the slices.SortStableFunc convention: negative if a sorts before b. |
| `func (n *Node[T]) Sum(value func(T) float64) *Node[T]` | Sum sets every node's Value to value(node.Data) plus the sum of its children's values, working leaves-first so a parent always sees final child… |

</details>

<details>
<summary><code>Pack</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewPack[T any]() *Pack[T]` | NewPack returns a pack layout with D3's defaults: sqrt-of-value leaf radii, a size of 1×1 and no padding. |
| `func (p *Pack[T]) Layout(root *Node[T]) *Node[T]` | Layout packs the subtree rooted at root and returns it. |
| `func (p *Pack[T]) Padding(padding float64) *Pack[T]` | Padding sets a constant gap around each group of siblings. |
| `func (p *Pack[T]) PaddingFunc(f func(*Node[T]) float64) *Pack[T]` | PaddingFunc sets a per-parent gap around each group of siblings. |
| `func (p *Pack[T]) Radius(f func(*Node[T]) float64) *Pack[T]` | Radius sets an explicit leaf radius accessor, disabling the default sqrt(Value) sizing and the final scale-to-fit. |
| `func (p *Pack[T]) Size(width, height float64) *Pack[T]` | Size sets the extent of the layout. |

</details>

<details>
<summary><code>Partition</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewPartition[T any]() *Partition[T]` | NewPartition returns a partition layout with D3's defaults: size 1×1, no padding, no rounding. |
| `func (p *Partition[T]) Layout(root *Node[T]) *Node[T]` | Layout positions the subtree rooted at root and returns it. |
| `func (p *Partition[T]) Padding(padding float64) *Partition[T]` | Padding insets every node on all four sides. |
| `func (p *Partition[T]) Round(round bool) *Partition[T]` | Round enables or disables integer rounding of the finished rectangles. |
| `func (p *Partition[T]) Size(width, height float64) *Partition[T]` | Size sets the extent of the layout: the root spans (0, 0) to (width, height), with height divided evenly among the root's height+1 levels. |

</details>

<details>
<summary><code>Tile</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func ResquarifyRatio[T any](ratio float64) Tile[T]` | ResquarifyRatio returns a `Tile` like `Resquarify` at a chosen aspect ratio. |
| `func SquarifyRatio[T any](ratio float64) Tile[T]` | SquarifyRatio returns a `Tile` like `Squarify` but targeting a different aspect ratio. |

</details>

<details>
<summary><code>Tree</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewTree[T any]() *Tree[T]` | NewTree returns a tree layout with D3's defaults: `DefaultSeparation` and a size of 1×1. |
| `func (t *Tree[T]) Layout(root *Node[T]) *Node[T]` | Layout positions the subtree rooted at root and returns it. |
| `func (t *Tree[T]) NodeSize(width, height float64) *Tree[T]` | NodeSize fixes the spacing per node — width multiplies the separation-derived X and height is the distance between consecutive depths — and… |
| `func (t *Tree[T]) Separation(f func(a, b *Node[T]) float64) *Tree[T]` | Separation sets the function that decides the minimum gap between two adjacent nodes at the same depth. |
| `func (t *Tree[T]) Size(width, height float64) *Tree[T]` | Size scales the finished layout to fit a width×height rectangle, and cancels any previous NodeSize. |

</details>

<details>
<summary><code>Treemap</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewTreemap[T any]() *Treemap[T]` | NewTreemap returns a treemap layout with D3's defaults: `Squarify` tiling, a size of 1×1, no padding and no rounding. |
| `func (t *Treemap[T]) Layout(root *Node[T]) *Node[T]` | Layout subdivides the rectangle among the subtree rooted at root and returns it. |
| `func (t *Treemap[T]) Padding(p float64) *Treemap[T]` | Padding sets the inner padding and all four outer paddings to the same value. |
| `func (t *Treemap[T]) PaddingBottom(p float64) *Treemap[T]` | PaddingBottom sets the margin a parent keeps below its children. |
| `func (t *Treemap[T]) PaddingBottomFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingBottomFunc is `Treemap.PaddingBottom` with a per-node value. |
| `func (t *Treemap[T]) PaddingFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingFunc is `Treemap.Padding` with a per-node value. |
| `func (t *Treemap[T]) PaddingInner(p float64) *Treemap[T]` | PaddingInner sets the gap between adjacent siblings. |
| `func (t *Treemap[T]) PaddingInnerFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingInnerFunc is `Treemap.PaddingInner` with a per-node value; the node passed is the parent whose children are being separated. |
| `func (t *Treemap[T]) PaddingLeft(p float64) *Treemap[T]` | PaddingLeft sets the margin a parent keeps to the left of its children. |
| `func (t *Treemap[T]) PaddingLeftFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingLeftFunc is `Treemap.PaddingLeft` with a per-node value. |
| `func (t *Treemap[T]) PaddingOuter(p float64) *Treemap[T]` | PaddingOuter sets all four outer paddings. |
| `func (t *Treemap[T]) PaddingOuterFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingOuterFunc is `Treemap.PaddingOuter` with a per-node value. |
| `func (t *Treemap[T]) PaddingRight(p float64) *Treemap[T]` | PaddingRight sets the margin a parent keeps to the right of its children. |
| `func (t *Treemap[T]) PaddingRightFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingRightFunc is `Treemap.PaddingRight` with a per-node value. |
| `func (t *Treemap[T]) PaddingTop(p float64) *Treemap[T]` | PaddingTop sets the margin a parent keeps above its children — the usual place to draw a group label. |
| `func (t *Treemap[T]) PaddingTopFunc(f func(*Node[T]) float64) *Treemap[T]` | PaddingTopFunc is `Treemap.PaddingTop` with a per-node value. |
| `func (t *Treemap[T]) Round(round bool) *Treemap[T]` | Round enables or disables integer rounding of the finished rectangles. |
| `func (t *Treemap[T]) Size(width, height float64) *Treemap[T]` | Size sets the extent of the layout: the root occupies (0, 0) to (width, height). |
| `func (t *Treemap[T]) Tile(tile Tile[T]) *Treemap[T]` | Tile sets the tiling method. |

</details>

### Variables

`ErrNoRoot`, `ErrMultipleRoots`, `ErrAmbiguousID`, `ErrMissingParent`, `ErrCycle`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/hierarchy).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
