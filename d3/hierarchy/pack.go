package hierarchy

import "math"

// Pack is the circle-packing (enclosure) layout: every node becomes a circle,
// leaves are sized by Value, and each parent is the smallest circle enclosing
// its children. It writes X, Y and R on every node and touches nothing else.
//
// Call [Node.Sum] first, and sort if you care about arrangement — pack is
// sensitive to sibling order, and descending value gives the conventional look
// with the big circles in the middle.
//
//	root.Sum(size).Sort(func(a, b *hierarchy.Node[T]) int {
//		return cmp.Compare(b.Value, a.Value)
//	})
//	hierarchy.NewPack[T]().Size(900, 900).Padding(3).Layout(root)
//
// # How it works, and what it guarantees
//
// Packing runs bottom-up: a node's children are packed among themselves first,
// which fixes the node's own radius, and only then is the node placed among its
// own siblings. Siblings are packed with Wang et al.'s front-chain heuristic —
// place the first three circles mutually tangent, then keep a doubly linked
// "front" of the circles on the outer boundary and, for each new circle, place
// it tangent to the closest pair on that front, walking the chain outward to
// find and resolve the nearest collision. The bounding radius of the result is
// then the smallest circle enclosing the front, computed by [Enclose].
//
// The two guarantees the layout does make, and which the tests assert, are that
// no two circles at the same level overlap and that every child lies inside its
// parent. What it does *not* guarantee is optimality: front-chain packing is a
// heuristic, and there is generally a tighter arrangement. That is the accepted
// trade in D3 too, since optimal circle packing is intractable.
//
// # Sizing
//
// By default leaf radii are sqrt(Value), so a leaf's *area* is proportional to
// its value, and the whole packing is then scaled uniformly to fit the size.
// Radius overrides that with an explicit per-leaf radius, in which case no final
// scaling happens and the layout is centred at the middle of the size instead —
// the two modes are exclusive because scaling would defeat the point of naming
// exact radii.
//
// Padding is the gap left around each set of siblings. In the default (scaled)
// mode it is applied in the final coordinate space, so a padding of 3 really is
// about 3 units wide in the output. Padding may also be a function of the parent
// node, which is the way to get thicker separation between top-level groups.
type Pack[T any] struct {
	radius  func(*Node[T]) float64
	dx, dy  float64
	padding func(*Node[T]) float64
}

// NewPack returns a pack layout with D3's defaults: sqrt-of-value leaf radii, a
// size of 1×1 and no padding.
func NewPack[T any]() *Pack[T] {
	return &Pack[T]{dx: 1, dy: 1, padding: constantZero[T]}
}

// Size sets the extent of the layout. The packing is centred in the rectangle
// and scaled to fit the smaller dimension, so a non-square size leaves margins
// rather than producing ellipses.
func (p *Pack[T]) Size(width, height float64) *Pack[T] {
	p.dx, p.dy = width, height
	return p
}

// Radius sets an explicit leaf radius accessor, disabling the default
// sqrt(Value) sizing and the final scale-to-fit. Passing nil restores the
// default.
func (p *Pack[T]) Radius(f func(*Node[T]) float64) *Pack[T] {
	p.radius = f
	return p
}

// Padding sets a constant gap around each group of siblings.
func (p *Pack[T]) Padding(padding float64) *Pack[T] {
	p.padding = constant[T](padding)
	return p
}

// PaddingFunc sets a per-parent gap around each group of siblings.
func (p *Pack[T]) PaddingFunc(f func(*Node[T]) float64) *Pack[T] {
	p.padding = orZero(f)
	return p
}

// Layout packs the subtree rooted at root and returns it.
func (p *Pack[T]) Layout(root *Node[T]) *Node[T] {
	if root == nil {
		return nil
	}
	root.X, root.Y = p.dx/2, p.dy/2

	if p.radius != nil {
		root.EachBefore(radiusLeaf(p.radius))
		root.EachAfter(packChildren(p.padding, 0.5))
		root.EachBefore(translateChild[T](1))
		return root
	}

	// Default mode. The packing is done twice: once with no padding to
	// establish the natural radius of the whole tree, and again with the
	// padding scaled into that space, because the caller specifies padding in
	// *output* units and the intermediate packing is in arbitrary ones. The
	// final translate then scales everything to fit.
	root.EachBefore(radiusLeaf(func(n *Node[T]) float64 { return math.Sqrt(n.Value) }))
	root.EachAfter(packChildren(constantZero[T], 1))
	minSize := math.Min(p.dx, p.dy)
	k := 1.0
	if root.R > 0 {
		k = root.R / minSize
	}
	root.EachAfter(packChildren(p.padding, k))
	scale := 1.0
	if root.R > 0 {
		scale = minSize / (2 * root.R)
	}
	root.EachBefore(translateChild[T](scale))
	return root
}

// radiusLeaf assigns radii to leaves; internal nodes get theirs from packing.
func radiusLeaf[T any](radius func(*Node[T]) float64) func(*Node[T]) {
	return func(node *Node[T]) {
		if len(node.Children) > 0 {
			return
		}
		r := radius(node)
		if r < 0 || math.IsNaN(r) {
			r = 0
		}
		node.R = r
	}
}

// packChildren packs a node's children among themselves and sets the node's own
// radius to the enclosing radius.
//
// Padding is applied by temporarily inflating each child's radius, packing, then
// deflating again — which is exactly equivalent to leaving a gap of the padding
// between neighbours, and much simpler than teaching the packer about gaps.
func packChildren[T any](padding func(*Node[T]) float64, k float64) func(*Node[T]) {
	return func(node *Node[T]) {
		children := node.Children
		if len(children) == 0 {
			return
		}
		r := padding(node) * k
		if r < 0 || math.IsNaN(r) {
			r = 0
		}
		if r != 0 {
			for _, c := range children {
				c.R += r
			}
		}
		e := packSiblings(children)
		if r != 0 {
			for _, c := range children {
				c.R -= r
			}
		}
		node.R = e + r
	}
}

// translateChild converts the per-parent local coordinates produced by packing
// into absolute ones, scaling by k on the way down.
func translateChild[T any](k float64) func(*Node[T]) {
	return func(node *Node[T]) {
		node.R *= k
		if node.Parent != nil {
			node.X = node.Parent.X + k*node.X
			node.Y = node.Parent.Y + k*node.Y
		}
	}
}

// chainNode is a link in the front chain — the cyclic doubly linked list of
// circles currently on the boundary of the packed group.
type chainNode[T any] struct {
	node       *Node[T]
	next, prev *chainNode[T]
}

// packSiblings arranges the given nodes' circles around the origin without
// overlap and returns the radius of the smallest circle enclosing them. Each
// node's R is read; its X and Y are written.
func packSiblings[T any](nodes []*Node[T]) float64 {
	n := len(nodes)
	if n == 0 {
		return 0
	}

	// The first three circles are placed directly: one at the origin, the second
	// tangent to it on the x axis, the third tangent to both.
	a, b := nodes[0], nodes[0]
	a.X, a.Y = 0, 0
	if n == 1 {
		return a.R
	}
	b = nodes[1]
	a.X = -b.R
	b.X = a.R
	b.Y = 0
	if n == 2 {
		return a.R + b.R
	}
	placeTangent(b, a, nodes[2])

	an := &chainNode[T]{node: a}
	bn := &chainNode[T]{node: b}
	cn := &chainNode[T]{node: nodes[2]}
	// a → b → c → a, and the reverse for prev.
	an.next, cn.prev = bn, bn
	bn.next, an.prev = cn, cn
	cn.next, bn.prev = an, an

	// Place each remaining circle tangent to the current closest pair (an, bn),
	// then walk the front chain in both directions looking for the nearest
	// circle it collides with. If one is found, that circle becomes an endpoint
	// of the pair and the placement is retried; the circles between the old and
	// new endpoint are now interior and drop off the front.
placement:
	for i := 3; i < n; i++ {
		placeTangent(an.node, bn.node, nodes[i])
		newNode := &chainNode[T]{node: nodes[i]}

		j, k := bn.next, an.prev
		sj, sk := bn.node.R, an.node.R
		for {
			if sj <= sk {
				if intersects(j.node, newNode.node) {
					bn = j
					an.next = bn
					bn.prev = an
					i--
					continue placement
				}
				sj += j.node.R
				j = j.next
			} else {
				if intersects(k.node, newNode.node) {
					an = k
					an.next = bn
					bn.prev = an
					i--
					continue placement
				}
				sk += k.node.R
				k = k.prev
			}
			if j == k.next {
				break
			}
		}

		// No collision: splice the new circle into the front between a and b.
		newNode.prev, newNode.next = an, bn
		an.next, bn.prev = newNode, newNode

		// Choose the new insertion pair: the adjacent pair on the front whose
		// tangency point is closest to the origin, which is what keeps the
		// packing growing roughly radially instead of drifting off in one
		// direction.
		bn = newNode
		best := chainScore(an)
		for c := newNode.next; c != bn; c = c.next {
			if s := chainScore(c); s < best {
				an = c
				best = s
			}
		}
		bn = an.next
	}

	// The enclosing circle of the whole group is the enclosing circle of the
	// front chain: anything not on the front is by definition interior.
	var front []*Circle
	add := func(nd *Node[T]) {
		front = append(front, &Circle{X: nd.X, Y: nd.Y, R: nd.R})
	}
	add(bn.node)
	for c := bn.next; c != bn; c = c.next {
		add(c.node)
	}
	e := encloseAll(front)
	if e == nil {
		return 0
	}
	for _, nd := range nodes {
		nd.X -= e.X
		nd.Y -= e.Y
	}
	return e.R
}

// placeTangent positions c tangent to both a and b, on the far side from the
// existing pair. It is the standard two-circle Apollonius placement: intersect
// the circles of radius a.R+c.R about a and b.R+c.R about b, and take the
// solution on the outward side.
func placeTangent[T any](b, a, c *Node[T]) {
	dx := b.X - a.X
	dy := b.Y - a.Y
	d2 := dx*dx + dy*dy
	if d2 == 0 {
		// Coincident centres give no direction to work with; fall back to
		// placing c to the right of a.
		c.X = a.X + c.R
		c.Y = a.Y
		return
	}
	a2 := a.R + c.R
	a2 *= a2
	b2 := b.R + c.R
	b2 *= b2
	if a2 > b2 {
		x := (d2 + b2 - a2) / (2 * d2)
		y := math.Sqrt(math.Max(0, b2/d2-x*x))
		c.X = b.X - x*dx - y*dy
		c.Y = b.Y - x*dy + y*dx
	} else {
		x := (d2 + a2 - b2) / (2 * d2)
		y := math.Sqrt(math.Max(0, a2/d2-x*x))
		c.X = a.X + x*dx - y*dy
		c.Y = a.Y + x*dy + y*dx
	}
}

// intersects reports whether two circles overlap by more than a hair. The 1e-6
// slack matters: circles are routinely placed exactly tangent, and without it
// floating-point noise would report a tangency as a collision and send the
// packer into a retry loop.
func intersects[T any](a, b *Node[T]) bool {
	dr := a.R + b.R - 1e-6
	dx := b.X - a.X
	dy := b.Y - a.Y
	return dr > 0 && dr*dr > dx*dx+dy*dy
}

// chainScore is the squared distance from the origin to the radius-weighted
// midpoint of a front-chain pair — the heuristic for "which gap should the next
// circle go into".
func chainScore[T any](c *chainNode[T]) float64 {
	a, b := c.node, c.next.node
	ab := a.R + b.R
	if ab == 0 {
		return a.X*a.X + a.Y*a.Y
	}
	dx := (a.X*b.R + b.X*a.R) / ab
	dy := (a.Y*b.R + b.Y*a.R) / ab
	return dx*dx + dy*dy
}
