package hierarchy

// Partition is the icicle layout: each depth of the tree gets a band of equal
// thickness, and within a band each node's extent is proportional to its Value.
// It writes X0, Y0, X1, Y1 on every node and touches nothing else.
//
// Where a [Treemap] hides the tree's structure in favour of packing area
// efficiently, a partition keeps the structure completely legible — you can read
// depth straight off the Y axis and see every level at once — at the cost of
// wasting the space that deep, narrow branches leave empty. Mapping the result
// to polar coordinates at render time (X to angle, Y to radius) turns exactly
// the same numbers into a sunburst; the layout itself is orientation-agnostic
// and produces plain rectangles.
//
// Call [Node.Sum] first. Because each band is subdivided with [Dice], and
// [Dice] gives a parent's children exactly the parent's extent, the invariant
// "a parent's children exactly subdivide it" holds at every level.
//
// Padding insets every node by the same amount on all sides, which separates the
// bands visually; as with [Treemap] a rectangle too small to absorb it collapses
// to its midpoint rather than inverting. Round snaps the result to integers.
type Partition[T any] struct {
	dx, dy  float64
	padding float64
	round   bool
}

// NewPartition returns a partition layout with D3's defaults: size 1×1, no
// padding, no rounding.
func NewPartition[T any]() *Partition[T] {
	return &Partition[T]{dx: 1, dy: 1}
}

// Size sets the extent of the layout: the root spans (0, 0) to (width, height),
// with height divided evenly among the root's height+1 levels.
func (p *Partition[T]) Size(width, height float64) *Partition[T] {
	p.dx, p.dy = width, height
	return p
}

// Padding insets every node on all four sides.
func (p *Partition[T]) Padding(padding float64) *Partition[T] {
	p.padding = padding
	return p
}

// Round enables or disables integer rounding of the finished rectangles.
func (p *Partition[T]) Round(round bool) *Partition[T] { p.round = round; return p }

// Layout positions the subtree rooted at root and returns it.
func (p *Partition[T]) Layout(root *Node[T]) *Node[T] {
	if root == nil {
		return nil
	}
	n := float64(root.Height + 1)
	root.X0, root.Y0 = p.padding, p.padding
	root.X1 = p.dx
	root.Y1 = p.dy / n
	baseDepth := root.Depth

	root.EachBefore(func(node *Node[T]) {
		// Children are placed into the *next* band before this node's own
		// rectangle is trimmed by padding, so that the band boundaries stay on
		// the exact depth grid rather than drifting by the padding at each
		// level.
		if len(node.Children) > 0 {
			d := float64(node.Depth - baseDepth)
			Dice(node, node.X0, p.dy*(d+1)/n, node.X1, p.dy*(d+2)/n)
		}
		x0, y0 := node.X0, node.Y0
		x1, y1 := node.X1-p.padding, node.Y1-p.padding
		x0, x1 = collapse(x0, x1)
		y0, y1 = collapse(y0, y1)
		node.X0, node.Y0, node.X1, node.Y1 = x0, y0, x1, y1
	})

	if p.round {
		root.EachBefore(roundNode[T])
	}
	return root
}
