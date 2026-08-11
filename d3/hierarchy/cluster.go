package hierarchy

// Cluster is the dendrogram layout: every leaf sits on the same line at the far
// end of the drawing, and every internal node is centred over its children one
// step back. It writes X and Y and touches nothing else.
//
// The difference from [Tree] is what Y means. In a tidy tree Y comes from depth,
// so a leaf near the root is drawn near the root. In a cluster Y comes from
// height, so all leaves line up regardless of how deep they are — which is the
// point of a dendrogram, where the leaves are the observations and the interior
// is the merge structure. Cluster is also far simpler: because leaves are placed
// left to right in order and parents are the mean of their children, one
// post-order pass does the whole job, with no contour tracking and no collision
// avoidance. That also means the separation function only ever applies between
// adjacent leaves.
//
// As with [Tree], Size scales the result into a rectangle and NodeSize fixes the
// per-node spacing; the last one called wins. In NodeSize mode the root sits at
// X 0 and coordinates are measured relative to it, so descendants take negative
// X on the left — this matches D3 and is what makes a node-sized cluster easy to
// centre.
//
// Cluster does not read Value.
type Cluster[T any] struct {
	separation func(a, b *Node[T]) float64
	dx, dy     float64
	nodeSize   bool
}

// NewCluster returns a cluster layout with D3's defaults: [DefaultSeparation]
// and a size of 1×1.
func NewCluster[T any]() *Cluster[T] {
	return &Cluster[T]{separation: DefaultSeparation[T], dx: 1, dy: 1}
}

// Size scales the finished layout to fit a width×height rectangle and cancels
// any previous NodeSize.
func (c *Cluster[T]) Size(width, height float64) *Cluster[T] {
	c.nodeSize = false
	c.dx, c.dy = width, height
	return c
}

// NodeSize fixes the per-node spacing and cancels any previous Size. Coordinates
// come out relative to the root, which stays at (0, 0).
func (c *Cluster[T]) NodeSize(width, height float64) *Cluster[T] {
	c.nodeSize = true
	c.dx, c.dy = width, height
	return c
}

// Separation sets the gap between adjacent leaves. Passing nil restores
// [DefaultSeparation].
func (c *Cluster[T]) Separation(f func(a, b *Node[T]) float64) *Cluster[T] {
	if f == nil {
		f = DefaultSeparation[T]
	}
	c.separation = f
	return c
}

// Layout positions the subtree rooted at root and returns it.
func (c *Cluster[T]) Layout(root *Node[T]) *Node[T] {
	if root == nil {
		return nil
	}
	sep := c.separation
	if sep == nil {
		sep = DefaultSeparation[T]
	}

	// Pass one: leaves get consecutive X in traversal order, internal nodes the
	// mean X and one-more-than-max Y of their children.
	var previous *Node[T]
	x := 0.0
	root.EachAfter(func(node *Node[T]) {
		if len(node.Children) > 0 {
			sum, maxY := 0.0, 0.0
			for _, kid := range node.Children {
				sum += kid.X
				if kid.Y > maxY {
					maxY = kid.Y
				}
			}
			node.X = sum / float64(len(node.Children))
			node.Y = maxY + 1
			return
		}
		if previous != nil {
			x += sep(node, previous)
			node.X = x
		} else {
			node.X = 0
		}
		node.Y = 0
		previous = node
	})

	if c.nodeSize {
		rootX, rootY := root.X, root.Y
		return root.EachAfter(func(node *Node[T]) {
			node.X = (node.X - rootX) * c.dx
			node.Y = (rootY - node.Y) * c.dy
		})
	}

	// Size mode: map the leaf span onto [0, dx], padding each end by half a
	// separation so the outermost leaves are inset rather than flush, and invert
	// Y so the root is at 0 and the leaves at dy.
	left, right := leafLeft(root), leafRight(root)
	x0 := left.X - sep(left, right)/2
	x1 := right.X + sep(right, left)/2
	rootY := root.Y
	span := x1 - x0
	return root.EachAfter(func(node *Node[T]) {
		if span != 0 {
			node.X = (node.X - x0) / span * c.dx
		} else {
			node.X = c.dx / 2
		}
		if rootY != 0 {
			node.Y = (1 - node.Y/rootY) * c.dy
		} else {
			node.Y = 0
		}
	})
}

// leafLeft returns the first leaf reached by always taking the first child.
func leafLeft[T any](node *Node[T]) *Node[T] {
	for len(node.Children) > 0 {
		node = node.Children[0]
	}
	return node
}

// leafRight returns the last leaf reached by always taking the last child.
func leafRight[T any](node *Node[T]) *Node[T] {
	for len(node.Children) > 0 {
		node = node.Children[len(node.Children)-1]
	}
	return node
}
