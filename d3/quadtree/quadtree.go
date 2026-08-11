// Package quadtree is a Go port of d3-quadtree — a two-dimensional recursive
// spatial subdivision. It is the index that makes "which of these ten thousand
// points is nearest to my cursor?" and "what is the net repulsion from every
// other point?" cheap enough to ask in a loop.
//
// A quadtree partitions a square region into four equal quadrants, and each
// quadrant that ends up holding more than one point is subdivided again. The
// result is a tree whose leaves hold the data and whose internal nodes stand for
// regions of space, so a query that can rule out a region rules out everything
// inside it in one comparison.
//
//	q := quadtree.New(
//		func(p Point) float64 { return p.X },
//		func(p Point) float64 { return p.Y },
//	).AddAll(points)
//
//	near, ok := q.FindWithin(mouseX, mouseY, 20)
//
// The two headline uses are the two this repository needs. [Quadtree.Find] is
// nearest-neighbour search, used for hit testing. [Quadtree.VisitAfter] is a
// bottom-up traversal that lets a caller accumulate an aggregate into each
// internal node — the mass and centre of charge of everything below it — which
// is exactly the precomputation the Barnes–Hut approximation in the sibling
// d3/force package runs before every tick.
//
// # The node representation
//
// The tree is built from one [Node] type playing two roles, faithfully to
// upstream. An *internal* node is a region: it has Children and no data. A
// *leaf* is a point: it has Data, no children, and a Next pointer chaining any
// further points at exactly the same coordinates. Coincident points are the
// reason for that chain — no amount of subdivision separates two points at the
// same position, so the tree stops subdividing and stacks them. Ask
// [Node.IsLeaf] rather than inspecting Children.
//
// # The aggregate slots
//
// [Node] carries four float64 fields — X, Y, Value and R — that this package
// never reads or writes. They exist for callers of [Quadtree.VisitAfter] to
// accumulate into.
//
// This mirrors the decision the sibling d3/hierarchy package documents at
// length. JavaScript d3-force writes `quad.x`, `quad.y` and `quad.value` onto
// quadtree nodes as dynamic properties during its accumulate pass, and
// d3-force's collide writes `quad.r`. Go has no dynamic properties, so the
// options were a side table keyed by *Node — allocated and thrown away on every
// tick of a simulation, which is the hot loop this package exists to make fast —
// or a handful of declared fields. The fields won. They are documented as
// scratch space rather than as state of the tree: they are not copied by
// [Quadtree.Copy], not maintained across [Quadtree.Add], and mean nothing until
// a VisitAfter pass has filled them in.
//
// # Why T must be comparable
//
// [Quadtree.Remove] deletes a specific datum, and d3 identifies it with `===`.
// Go's equivalent is ==, which needs T comparable. Any pointer type qualifies,
// and a pointer is what JavaScript's identity comparison meant anyway — so a
// datum that is not comparable (one containing a slice or a map) should be
// stored as a *T.
//
// # Concurrency
//
// A Quadtree is mutable and is not safe for concurrent modification. Concurrent
// [Quadtree.Find] and [Quadtree.Visit] calls on a tree nobody is mutating are
// fine, with the caveat that the aggregate slots described above are shared
// mutable state and a VisitAfter pass is therefore a mutation.
package quadtree

import "math"

// Node is one vertex of a quadtree: either an internal node standing for a
// square region, or a leaf holding one or more coincident data points.
//
// Which role a node is playing is reported by [Node.IsLeaf]. On a leaf, Data is
// the first datum at that position and Next chains any others at exactly the
// same coordinates; Children is zero. On an internal node, Children holds the
// four quadrants in the order (top-left, top-right, bottom-left, bottom-right)
// — index (bottom<<1)|right, matching upstream — with a nil entry for a quadrant
// that is empty; Data and Next are zero.
//
// X, Y, Value and R are scratch space for [Quadtree.VisitAfter]; see the package
// doc. Nothing in this package reads them.
type Node[T comparable] struct {
	// Data is this leaf's datum. Meaningless on an internal node.
	Data T

	// Next is the next datum at exactly the same coordinates, or nil.
	// Meaningless on an internal node.
	Next *Node[T]

	// Children are the four quadrants of an internal node, indexed
	// (bottom<<1)|right, with nil for an empty quadrant. All nil on a leaf.
	Children [4]*Node[T]

	// X, Y, Value and R are unused by this package and free for a
	// [Quadtree.VisitAfter] callback to accumulate into. See the package doc.
	X, Y, Value, R float64

	leaf bool
}

// IsLeaf reports whether this node holds data rather than quadrants.
func (n *Node[T]) IsLeaf() bool { return n.leaf }

// Quadtree indexes values of type T by a point derived from each of them.
//
// Create one with [New], which takes the two accessors that read a datum's
// coordinates, then fill it with [Quadtree.Add] or [Quadtree.AddAll].
type Quadtree[T comparable] struct {
	x, y func(T) float64

	// The covered square. Valid only when covered is true; an empty tree that
	// has never been covered has no extent at all.
	x0, y0, x1, y1 float64
	covered        bool

	root *Node[T]
}

// New returns an empty quadtree that reads each datum's coordinates with the
// given accessors. Both are required; there is no default, because there is no
// dynamic property access to default to.
func New[T comparable](x, y func(T) float64) *Quadtree[T] {
	return &Quadtree[T]{x: x, y: y}
}

// X returns the x accessor.
func (q *Quadtree[T]) X() func(T) float64 { return q.x }

// Y returns the y accessor.
func (q *Quadtree[T]) Y() func(T) float64 { return q.y }

// SetX replaces the x accessor and returns the receiver.
//
// As upstream, this does not re-index data already in the tree: points added
// before the change keep the positions they were filed under, and the tree is
// wrong until it is rebuilt. Set the accessors before adding anything.
func (q *Quadtree[T]) SetX(x func(T) float64) *Quadtree[T] { q.x = x; return q }

// SetY replaces the y accessor and returns the receiver. See [Quadtree.SetX]
// for the caveat about data already added.
func (q *Quadtree[T]) SetY(y func(T) float64) *Quadtree[T] { q.y = y; return q }

// Root returns the root node, or nil for an empty tree.
//
// The root of a one-point tree is that point's leaf, not an internal node.
func (q *Quadtree[T]) Root() *Node[T] { return q.root }

// Extent returns the square the tree currently covers, and whether it covers
// anything at all. A tree that has had no point added and no explicit
// [Quadtree.Cover] has no extent, and ok is false.
//
// The extent is always a square whose side is a power of two times the side of
// the initial unit square, because that is what lets [Quadtree.Cover] grow the
// tree by wrapping the existing root in a new parent rather than rebuilding it.
// It is therefore usually larger than the bounding box of the data.
func (q *Quadtree[T]) Extent() (x0, y0, x1, y1 float64, ok bool) {
	if !q.covered {
		return 0, 0, 0, 0, false
	}
	return q.x0, q.y0, q.x1, q.y1, true
}

// SetExtent expands the tree to cover the given rectangle's corners and returns
// the receiver. It is [Quadtree.Cover] applied twice; the covered region ends up
// square and at least this large.
func (q *Quadtree[T]) SetExtent(x0, y0, x1, y1 float64) *Quadtree[T] {
	return q.Cover(x0, y0).Cover(x1, y1)
}

// Cover expands the tree so that the given point lies inside its extent, and
// returns the receiver.
//
// Growing is done by doubling: the existing root becomes one quadrant of a new
// internal node, repeatedly, until the point is inside. No existing point moves
// and nothing is re-inserted, which is why the extent is always a power-of-two
// square. A point with a NaN coordinate is ignored.
func (q *Quadtree[T]) Cover(x, y float64) *Quadtree[T] {
	if math.IsNaN(x) || math.IsNaN(y) {
		return q
	}
	if !q.covered {
		q.x0 = math.Floor(x)
		q.y0 = math.Floor(y)
		q.x1 = q.x0 + 1
		q.y1 = q.y0 + 1
		q.covered = true
		return q
	}

	z := q.x1 - q.x0
	if z == 0 {
		z = 1
	}
	node := q.root
	for q.x0 > x || x >= q.x1 || q.y0 > y || y >= q.y1 {
		i := 0
		if y < q.y0 {
			i |= 2
		}
		if x < q.x0 {
			i |= 1
		}
		parent := &Node[T]{}
		parent.Children[i] = node
		node = parent
		z *= 2
		switch i {
		case 0:
			q.x1 = q.x0 + z
			q.y1 = q.y0 + z
		case 1:
			q.x0 = q.x1 - z
			q.y1 = q.y0 + z
		case 2:
			q.x1 = q.x0 + z
			q.y0 = q.y1 - z
		case 3:
			q.x0 = q.x1 - z
			q.y0 = q.y1 - z
		}
	}
	// Only adopt the wrapper chain if there was a tree to wrap. Wrapping a
	// single leaf would turn it into an internal node holding a leaf, which is
	// a different (and wrong) shape: a leaf is always re-filed by Add.
	if q.root != nil && !q.root.leaf {
		q.root = node
	}
	return q
}

// Add inserts one datum and returns the receiver. The tree is covered first, so
// adding a point outside the current extent grows it. A datum whose coordinates
// are NaN is ignored.
func (q *Quadtree[T]) Add(d T) *Quadtree[T] {
	x, y := q.x(d), q.y(d)
	return q.Cover(x, y).add(x, y, d)
}

// add inserts a datum already known to be inside the covered extent. This is
// the port of d3-quadtree's internal add(), split out so that AddAll can cover
// the whole batch once instead of once per point.
func (q *Quadtree[T]) add(x, y float64, d T) *Quadtree[T] {
	if math.IsNaN(x) || math.IsNaN(y) {
		return q
	}
	leaf := &Node[T]{Data: d, leaf: true}
	if q.root == nil {
		q.root = leaf
		return q
	}

	x0, y0, x1, y1 := q.x0, q.y0, q.x1, q.y1
	var parent *Node[T]
	var i int
	node := q.root

	// Descend to a leaf, splitting the region as we go.
	for !node.leaf {
		xm, ym := (x0+x1)/2, (y0+y1)/2
		right := x >= xm
		if right {
			x0 = xm
		} else {
			x1 = xm
		}
		bottom := y >= ym
		if bottom {
			y0 = ym
		} else {
			y1 = ym
		}
		parent = node
		i = 0
		if bottom {
			i |= 2
		}
		if right {
			i |= 1
		}
		node = parent.Children[i]
		if node == nil {
			parent.Children[i] = leaf
			return q
		}
	}

	// node is a leaf. If the new point is coincident with it, push onto the
	// chain rather than subdividing forever.
	xp, yp := q.x(node.Data), q.y(node.Data)
	if x == xp && y == yp {
		leaf.Next = node
		if parent != nil {
			parent.Children[i] = leaf
		} else {
			q.root = leaf
		}
		return q
	}

	// Otherwise split until the two points land in different quadrants.
	var j int
	for {
		if parent != nil {
			n := &Node[T]{}
			parent.Children[i] = n
			parent = n
		} else {
			n := &Node[T]{}
			q.root = n
			parent = n
		}
		xm, ym := (x0+x1)/2, (y0+y1)/2
		right := x >= xm
		if right {
			x0 = xm
		} else {
			x1 = xm
		}
		bottom := y >= ym
		if bottom {
			y0 = ym
		} else {
			y1 = ym
		}
		i = 0
		if bottom {
			i |= 2
		}
		if right {
			i |= 1
		}
		j = 0
		if yp >= ym {
			j |= 2
		}
		if xp >= xm {
			j |= 1
		}
		if i != j {
			break
		}
	}
	parent.Children[j] = node
	parent.Children[i] = leaf
	return q
}

// AddAll inserts every datum and returns the receiver.
//
// This is not a loop over [Quadtree.Add]: the bounding box of the whole batch is
// computed first and covered once, so the tree doubles at most as many times as
// it has to instead of once per out-of-range point. Data with NaN coordinates
// are skipped, and skipping them here also keeps them out of the bounding box.
func (q *Quadtree[T]) AddAll(data []T) *Quadtree[T] {
	n := len(data)
	xs := make([]float64, n)
	ys := make([]float64, n)
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)

	for i, d := range data {
		x, y := q.x(d), q.y(d)
		if math.IsNaN(x) || math.IsNaN(y) {
			xs[i], ys[i] = math.NaN(), math.NaN()
			continue
		}
		xs[i], ys[i] = x, y
		if x < x0 {
			x0 = x
		}
		if x > x1 {
			x1 = x
		}
		if y < y0 {
			y0 = y
		}
		if y > y1 {
			y1 = y
		}
	}

	// Every point was invalid; there is nothing to cover and nothing to add.
	if x0 > x1 || y0 > y1 {
		return q
	}

	q.Cover(x0, y0).Cover(x1, y1)
	for i, d := range data {
		q.add(xs[i], ys[i], d)
	}
	return q
}

// Remove deletes one datum from the tree and returns the receiver. The datum is
// identified by ==, matching d3's identity comparison; see the package doc.
// Removing something that is not in the tree is a no-op.
func (q *Quadtree[T]) Remove(d T) *Quadtree[T] {
	x, y := q.x(d), q.y(d)
	if math.IsNaN(x) || math.IsNaN(y) {
		return q
	}
	node := q.root
	if node == nil {
		return q
	}

	var parent, retainer, previous *Node[T]
	var i, j int
	x0, y0, x1, y1 := q.x0, q.y0, q.x1, q.y1

	if !node.leaf {
		for {
			xm, ym := (x0+x1)/2, (y0+y1)/2
			right := x >= xm
			if right {
				x0 = xm
			} else {
				x1 = xm
			}
			bottom := y >= ym
			if bottom {
				y0 = ym
			} else {
				y1 = ym
			}
			parent = node
			i = 0
			if bottom {
				i |= 2
			}
			if right {
				i |= 1
			}
			node = parent.Children[i]
			if node == nil {
				return q
			}
			if node.leaf {
				break
			}
			// Remember the deepest ancestor with a second occupied quadrant:
			// that is where a collapse has to stop.
			if parent.Children[(i+1)&3] != nil || parent.Children[(i+2)&3] != nil || parent.Children[(i+3)&3] != nil {
				retainer, j = parent, i
			}
		}
	}

	// Walk the coincident chain for the exact datum.
	for node.Data != d {
		previous = node
		node = node.Next
		if node == nil {
			return q
		}
	}
	next := node.Next
	node.Next = nil

	// It was in the middle of a coincident chain: unlink and stop.
	if previous != nil {
		previous.Next = next
		return q
	}

	// It was the root leaf.
	if parent == nil {
		q.root = next
		return q
	}

	if next != nil {
		parent.Children[i] = next
	} else {
		parent.Children[i] = nil
	}

	// If the parent is now a single leaf, that leaf can take the parent's place
	// — but only up to the nearest ancestor that still has a sibling.
	only := parent.Children[0]
	if only == nil {
		only = parent.Children[1]
	}
	if only == nil {
		only = parent.Children[2]
	}
	if only == nil {
		only = parent.Children[3]
	}
	last := parent.Children[3]
	if last == nil {
		last = parent.Children[2]
	}
	if last == nil {
		last = parent.Children[1]
	}
	if last == nil {
		last = parent.Children[0]
	}
	if only != nil && only == last && only.leaf {
		if retainer != nil {
			retainer.Children[j] = only
		} else {
			q.root = only
		}
	}
	return q
}

// RemoveAll deletes every listed datum and returns the receiver.
func (q *Quadtree[T]) RemoveAll(data []T) *Quadtree[T] {
	for _, d := range data {
		q.Remove(d)
	}
	return q
}

// quad is one entry on the explicit traversal stack: a node and the square it
// occupies. The traversals are iterative rather than recursive because upstream
// is, and because Find's correctness depends on the order the stack is popped
// in.
type quad[T comparable] struct {
	node           *Node[T]
	x0, y0, x1, y1 float64
}

// Find returns the datum closest to (x, y), and whether the tree had one.
//
// The search is exact, not approximate: it maintains the best distance found so
// far and prunes any quadrant whose square cannot contain anything closer, so
// the answer is the same one a brute-force scan would give. Ties go to whichever
// point the traversal reaches first.
func (q *Quadtree[T]) Find(x, y float64) (T, bool) {
	return q.find(x, y, math.Inf(1))
}

// FindWithin is [Quadtree.Find] limited to a search radius: it returns the
// closest datum within radius of (x, y), and false if there is none.
//
// This is the form to use for hit testing, where "nothing is near enough"
// is a real answer and scanning the whole tree to discover it is waste. Go has
// no optional arguments, which is why upstream's single `find(x, y[, radius])`
// is two methods here.
func (q *Quadtree[T]) FindWithin(x, y, radius float64) (T, bool) {
	return q.find(x, y, radius)
}

func (q *Quadtree[T]) find(x, y, radius float64) (T, bool) {
	var found T
	var ok bool

	if q.root == nil {
		return found, false
	}

	// The search window, shrunk every time a closer point is found.
	sx0, sy0, sx1, sy1 := q.x0, q.y0, q.x1, q.y1
	stack := []quad[T]{{q.root, q.x0, q.y0, q.x1, q.y1}}

	r2 := math.Inf(1)
	if !math.IsInf(radius, 1) {
		sx0, sy0 = x-radius, y-radius
		sx1, sy1 = x+radius, y+radius
		r2 = radius * radius
	}

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		node := cur.node
		if node == nil || cur.x0 > sx1 || cur.y0 > sy1 || cur.x1 < sx0 || cur.y1 < sy0 {
			continue
		}

		if !node.leaf {
			xm, ym := (cur.x0+cur.x1)/2, (cur.y0+cur.y1)/2
			stack = append(stack,
				quad[T]{node.Children[3], xm, ym, cur.x1, cur.y1},
				quad[T]{node.Children[2], cur.x0, ym, xm, cur.y1},
				quad[T]{node.Children[1], xm, cur.y0, cur.x1, ym},
				quad[T]{node.Children[0], cur.x0, cur.y0, xm, ym},
			)
			// Visit the quadrant containing the search point first, by swapping
			// it to the top of the stack. This is the whole trick: the first
			// point found is then usually very close, so the radius collapses
			// immediately and almost everything else is pruned.
			i := 0
			if y >= ym {
				i |= 2
			}
			if x >= xm {
				i |= 1
			}
			if i != 0 {
				top := len(stack) - 1
				stack[top], stack[top-i] = stack[top-i], stack[top]
			}
			continue
		}

		dx := x - q.x(node.Data)
		dy := y - q.y(node.Data)
		d2 := dx*dx + dy*dy
		if d2 < r2 {
			r2 = d2
			d := math.Sqrt(d2)
			sx0, sy0 = x-d, y-d
			sx1, sy1 = x+d, y+d
			found, ok = node.Data, true
		}
	}
	return found, ok
}

// VisitFunc is the callback for [Quadtree.Visit]. It receives a node and the
// square it occupies, and returns true to prune — to skip that node's children
// entirely. Returning false descends.
type VisitFunc[T comparable] func(node *Node[T], x0, y0, x1, y1 float64) bool

// VisitAfterFunc is the callback for [Quadtree.VisitAfter]. There is no return
// value because there is nothing left to prune: the children have already been
// visited.
type VisitAfterFunc[T comparable] func(node *Node[T], x0, y0, x1, y1 float64)

// Visit traverses the tree top-down in pre-order, calling visit for each node
// before its children, and returns the receiver.
//
// Returning true from the callback prunes: that node's children are not visited.
// This is what makes a quadtree worth building — a spatial query decides from a
// node's square alone that nothing inside it can matter, and skips the whole
// subtree. Children are visited in quadrant order, so a traversal that prunes
// nothing is deterministic.
func (q *Quadtree[T]) Visit(visit VisitFunc[T]) *Quadtree[T] {
	if q.root == nil {
		return q
	}
	stack := []quad[T]{{q.root, q.x0, q.y0, q.x1, q.y1}}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := cur.node
		if visit(node, cur.x0, cur.y0, cur.x1, cur.y1) || node.leaf {
			continue
		}
		xm, ym := (cur.x0+cur.x1)/2, (cur.y0+cur.y1)/2
		// Pushed in reverse so quadrant 0 is popped first.
		if c := node.Children[3]; c != nil {
			stack = append(stack, quad[T]{c, xm, ym, cur.x1, cur.y1})
		}
		if c := node.Children[2]; c != nil {
			stack = append(stack, quad[T]{c, cur.x0, ym, xm, cur.y1})
		}
		if c := node.Children[1]; c != nil {
			stack = append(stack, quad[T]{c, xm, cur.y0, cur.x1, ym})
		}
		if c := node.Children[0]; c != nil {
			stack = append(stack, quad[T]{c, cur.x0, cur.y0, xm, ym})
		}
	}
	return q
}

// VisitAfter traverses the tree bottom-up in post-order, calling visit for each
// node only after every one of its children, and returns the receiver.
//
// There is no pruning, because a post-order traversal has already done the work
// pruning would save. The point of this traversal is aggregation: a callback
// that reads its children's accumulated values and writes its own is guaranteed
// to find them already computed. That is how d3/force's ManyBody force turns a
// quadtree into a Barnes–Hut tree, summing charge and centre of mass into the
// scratch fields described in the package doc.
func (q *Quadtree[T]) VisitAfter(visit VisitAfterFunc[T]) *Quadtree[T] {
	if q.root == nil {
		return q
	}
	stack := []quad[T]{{q.root, q.x0, q.y0, q.x1, q.y1}}
	var order []quad[T]
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := cur.node
		if !node.leaf {
			xm, ym := (cur.x0+cur.x1)/2, (cur.y0+cur.y1)/2
			if c := node.Children[0]; c != nil {
				stack = append(stack, quad[T]{c, cur.x0, cur.y0, xm, ym})
			}
			if c := node.Children[1]; c != nil {
				stack = append(stack, quad[T]{c, xm, cur.y0, cur.x1, ym})
			}
			if c := node.Children[2]; c != nil {
				stack = append(stack, quad[T]{c, cur.x0, ym, xm, cur.y1})
			}
			if c := node.Children[3]; c != nil {
				stack = append(stack, quad[T]{c, xm, ym, cur.x1, cur.y1})
			}
		}
		order = append(order, cur)
	}
	for i := len(order) - 1; i >= 0; i-- {
		c := order[i]
		visit(c.node, c.x0, c.y0, c.x1, c.y1)
	}
	return q
}

// Size returns the number of data in the tree, counting every point of a
// coincident chain.
func (q *Quadtree[T]) Size() int {
	n := 0
	q.Visit(func(node *Node[T], _, _, _, _ float64) bool {
		if node.leaf {
			for l := node; l != nil; l = l.Next {
				n++
			}
		}
		return false
	})
	return n
}

// Data returns every datum in the tree.
//
// The order is the tree's, not the insertion order: quadrant order from the
// root down, and within a coincident chain, most-recently-added first. It is
// deterministic for a given tree, which is what a test needs, but it is not
// something to depend on the meaning of.
func (q *Quadtree[T]) Data() []T {
	var out []T
	q.Visit(func(node *Node[T], _, _, _, _ float64) bool {
		if node.leaf {
			for l := node; l != nil; l = l.Next {
				out = append(out, l.Data)
			}
		}
		return false
	})
	return out
}

// Copy returns a deep copy of the tree: the same data in the same structure,
// with every node newly allocated, so adding to or removing from the copy leaves
// the original alone. The data themselves are copied by value and are shared if
// T is a pointer, exactly as upstream.
//
// The aggregate scratch fields (X, Y, Value, R) are not copied — they belong to
// whatever [Quadtree.VisitAfter] pass wrote them, not to the tree.
func (q *Quadtree[T]) Copy() *Quadtree[T] {
	c := &Quadtree[T]{
		x: q.x, y: q.y,
		x0: q.x0, y0: q.y0, x1: q.x1, y1: q.y1,
		covered: q.covered,
	}
	c.root = copyNode(q.root)
	return c
}

func copyNode[T comparable](n *Node[T]) *Node[T] {
	if n == nil {
		return nil
	}
	c := &Node[T]{leaf: n.leaf}
	if n.leaf {
		c.Data = n.Data
		dst := c
		for src := n.Next; src != nil; src = src.Next {
			dst.Next = &Node[T]{Data: src.Data, leaf: true}
			dst = dst.Next
		}
		return c
	}
	for i, child := range n.Children {
		c.Children[i] = copyNode(child)
	}
	return c
}
