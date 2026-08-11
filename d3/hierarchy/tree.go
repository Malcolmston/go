package hierarchy

// DefaultSeparation is the separation function both [Tree] and [Cluster] use
// unless told otherwise: siblings are kept 1 apart, cousins 2 apart.
//
// A separation function is the layout's only knob for horizontal spacing, and it
// is expressed in the same units as the layout's own coordinates, which is what
// makes it composable: with a size-based layout the units are arbitrary and get
// normalised away at the end, while with a node-size-based layout they are the
// units you passed to NodeSize. Returning a larger number for nodes with
// different parents is what visually groups a family together.
//
// The two nodes are always adjacent at the same depth, and a is always to the
// left of b.
func DefaultSeparation[T any](a, b *Node[T]) float64 {
	if a.Parent == b.Parent {
		return 1
	}
	return 2
}

// Tree lays out a hierarchy with the Reingold–Tilford "tidy" algorithm, in the
// linear-time formulation of Buchheim, Jünger and Leipert. It writes X and Y on
// every node and touches nothing else.
//
// The tidy algorithm's guarantees are what make it worth the complexity over a
// naive layout: nodes at the same depth never overlap and are ordered
// left-to-right as in Children; a parent is centred exactly over its children;
// and isomorphic subtrees are drawn identically wherever they appear. It
// achieves that by giving every subtree a "contour" — its left and right
// extremes at each depth — and, whenever two sibling subtrees would collide,
// shifting the right-hand one and distributing the shift proportionally across
// the siblings in between so the parent stays centred. The threading of contours
// (the t and a fields on the internal node) is what keeps it linear instead of
// quadratic.
//
// Configure it either by Size, which scales the finished layout to fit a
// rectangle, or by NodeSize, which fixes the spacing per node and lets the
// drawing be as large as it needs to be. The two are mutually exclusive — the
// last one called wins — because they answer opposite questions ("how big is the
// canvas?" versus "how big is a node?"). Size is the default, at 1×1, so the
// output is normalised into the unit square and you can scale it yourself.
//
// The layout is depth-oriented in Y: Y is derived from Depth alone, so all nodes
// at a depth share a Y. To draw a tree top-to-bottom take (X, Y) as-is; to draw
// it left-to-right swap them at render time. Tree does not know about
// orientation.
//
// Tree does not read Value, so [Node.Sum] is not required.
type Tree[T any] struct {
	separation func(a, b *Node[T]) float64
	dx, dy     float64
	nodeSize   bool
}

// NewTree returns a tree layout with D3's defaults: [DefaultSeparation] and a
// size of 1×1.
func NewTree[T any]() *Tree[T] {
	return &Tree[T]{separation: DefaultSeparation[T], dx: 1, dy: 1}
}

// Size scales the finished layout to fit a width×height rectangle, and cancels
// any previous NodeSize. X spans [0, width] and Y spans [0, height].
func (t *Tree[T]) Size(width, height float64) *Tree[T] {
	t.nodeSize = false
	t.dx, t.dy = width, height
	return t
}

// NodeSize fixes the spacing per node — width multiplies the separation-derived
// X and height is the distance between consecutive depths — and cancels any
// previous Size. The extent of the result then depends on the tree, and the root
// is not necessarily at X 0; this is the mode to use when nodes are drawn at a
// fixed pixel size.
func (t *Tree[T]) NodeSize(width, height float64) *Tree[T] {
	t.nodeSize = true
	t.dx, t.dy = width, height
	return t
}

// Separation sets the function that decides the minimum gap between two adjacent
// nodes at the same depth. Passing nil restores [DefaultSeparation].
func (t *Tree[T]) Separation(f func(a, b *Node[T]) float64) *Tree[T] {
	if f == nil {
		f = DefaultSeparation[T]
	}
	t.separation = f
	return t
}

// treeNode is the scratch structure the algorithm runs on, mirroring the
// original paper's notation. It shadows the real tree so that the many
// intermediate quantities never touch [Node]:
//
//	z  the node's preliminary X coordinate
//	m  the modifier: a shift to be added to this node's whole subtree
//	c  and s, the "change" and "shift" accumulators used to spread a
//	   collision-avoiding shift evenly over the siblings between two subtrees
//	t  the thread: a pointer that lets a contour walk jump across a subtree it
//	   has already passed, which is what avoids re-walking and keeps the whole
//	   layout linear
//	a  the ancestor used when a shift must be attributed to a subtree
//	i  the node's index among its siblings
type treeNode[T any] struct {
	node     *Node[T]
	parent   *treeNode[T]
	children []*treeNode[T]
	ancestor *treeNode[T]
	thread   *treeNode[T]
	defaultA *treeNode[T]
	z, m     float64
	c, s     float64
	i        int
}

// Layout positions the subtree rooted at root and returns it.
func (t *Tree[T]) Layout(root *Node[T]) *Node[T] {
	if root == nil {
		return nil
	}
	sep := t.separation
	if sep == nil {
		sep = DefaultSeparation[T]
	}

	tr := newTreeRoot(root)
	tr.eachAfter(func(v *treeNode[T]) { firstWalk(v, sep) })
	tr.parent.m = -tr.z
	tr.eachBefore(secondWalk[T])

	if t.nodeSize {
		root.EachBefore(func(node *Node[T]) {
			node.X *= t.dx
			node.Y = float64(node.Depth) * t.dy
		})
		return root
	}

	// Size mode: normalise the preliminary coordinates into the requested
	// rectangle. The half-separation added at each end is what keeps the
	// outermost nodes from sitting exactly on the boundary, matching D3.
	left, right, bottom := root, root, root
	root.EachBefore(func(node *Node[T]) {
		if node.X < left.X {
			left = node
		}
		if node.X > right.X {
			right = node
		}
		if node.Depth > bottom.Depth {
			bottom = node
		}
	})
	s := 1.0
	if left != right {
		s = sep(left, right) / 2
	}
	tx := s - left.X
	kx := t.dx / (right.X + s + tx)
	depth := float64(bottom.Depth)
	if depth == 0 {
		depth = 1
	}
	ky := t.dy / depth
	root.EachBefore(func(node *Node[T]) {
		node.X = (node.X + tx) * kx
		node.Y = float64(node.Depth) * ky
	})
	return root
}

// newTreeRoot shadows the real tree with treeNodes and gives the root a
// synthetic parent, so firstWalk can read v.parent unconditionally.
func newTreeRoot[T any](root *Node[T]) *treeNode[T] {
	tr := &treeNode[T]{node: root}
	tr.ancestor = tr
	stack := []*treeNode[T]{tr}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kids := v.node.Children
		if len(kids) == 0 {
			continue
		}
		v.children = make([]*treeNode[T], len(kids))
		for i := len(kids) - 1; i >= 0; i-- {
			c := &treeNode[T]{node: kids[i], parent: v, i: i}
			c.ancestor = c
			v.children[i] = c
			stack = append(stack, c)
		}
	}
	parent := &treeNode[T]{}
	parent.ancestor = parent
	parent.children = []*treeNode[T]{tr}
	tr.parent = parent
	return tr
}

func (v *treeNode[T]) eachBefore(f func(*treeNode[T])) {
	stack := []*treeNode[T]{v}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		f(n)
		for i := len(n.children) - 1; i >= 0; i-- {
			stack = append(stack, n.children[i])
		}
	}
}

func (v *treeNode[T]) eachAfter(f func(*treeNode[T])) {
	stack := []*treeNode[T]{v}
	var order []*treeNode[T]
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		order = append(order, n)
		stack = append(stack, n.children...)
	}
	for i := len(order) - 1; i >= 0; i-- {
		f(order[i])
	}
}

// nextLeft and nextRight walk down the left and right contour of a subtree. When
// a node has no children the walk follows its thread instead, jumping to the
// next node on the contour of a subtree that was already traversed — the trick
// that makes the whole algorithm linear.
func nextLeft[T any](v *treeNode[T]) *treeNode[T] {
	if len(v.children) > 0 {
		return v.children[0]
	}
	return v.thread
}

func nextRight[T any](v *treeNode[T]) *treeNode[T] {
	if len(v.children) > 0 {
		return v.children[len(v.children)-1]
	}
	return v.thread
}

// moveSubtree records that wp's subtree must move right by shift, and stores the
// per-sibling portion in the c accumulators so executeShifts can later spread it
// linearly across everything between wm and wp.
func moveSubtree[T any](wm, wp *treeNode[T], shift float64) {
	change := shift / float64(wp.i-wm.i)
	wp.c -= change
	wp.s += shift
	wm.c += change
	wp.z += shift
	wp.m += shift
}

// executeShifts applies the accumulated shifts to a node's children, right to
// left, so that the gaps opened up by collision avoidance are distributed
// smoothly rather than dumped on a single sibling.
func executeShifts[T any](v *treeNode[T]) {
	shift, change := 0.0, 0.0
	for i := len(v.children) - 1; i >= 0; i-- {
		w := v.children[i]
		w.z += shift
		w.m += shift
		change += w.c
		shift += w.s + change
	}
}

// nextAncestor picks the subtree a shift should be charged to: the recorded
// ancestor if it is a sibling of v, otherwise the default one.
func nextAncestor[T any](vim, v, ancestor *treeNode[T]) *treeNode[T] {
	if vim.ancestor.parent == v.parent {
		return vim.ancestor
	}
	return ancestor
}

// firstWalk assigns each node a preliminary X in post-order: a leaf goes just
// right of its left sibling, and an internal node is centred over its children,
// with the difference recorded as a modifier so the subtree can be translated in
// one pass later.
func firstWalk[T any](v *treeNode[T], sep func(a, b *Node[T]) float64) {
	siblings := v.parent.children
	var w *treeNode[T]
	if v.i > 0 {
		w = siblings[v.i-1]
	}
	if len(v.children) > 0 {
		executeShifts(v)
		midpoint := (v.children[0].z + v.children[len(v.children)-1].z) / 2
		if w != nil {
			v.z = w.z + sep(v.node, w.node)
			v.m = v.z - midpoint
		} else {
			v.z = midpoint
		}
	} else if w != nil {
		v.z = w.z + sep(v.node, w.node)
	}
	def := v.parent.defaultA
	if def == nil {
		def = siblings[0]
	}
	v.parent.defaultA = apportion(v, w, def, sep)
}

// secondWalk turns preliminary coordinates plus accumulated modifiers into final
// X values, in pre-order so each node sees the total shift of its ancestors.
func secondWalk[T any](v *treeNode[T]) {
	v.node.X = v.z + v.parent.m
	v.m += v.parent.m
}

// apportion is the heart of the algorithm: it walks the right contour of
// everything to the left of v against the left contour of v, and whenever the
// two would touch, moves v's subtree right by exactly enough and charges the
// move to the correct intervening sibling. The four running sums (sip, sop, sim,
// som) carry the modifiers accumulated along the inner and outer contours on
// each side. The two trailing threads let a later contour walk skip whichever
// subtree ran out first.
func apportion[T any](v, w, ancestor *treeNode[T], sep func(a, b *Node[T]) float64) *treeNode[T] {
	if w == nil {
		return ancestor
	}
	vip, vop := v, v
	vim, vom := w, vip.parent.children[0]
	sip, sop := vip.m, vop.m
	sim, som := vim.m, vom.m

	for {
		vim = nextRight(vim)
		vip = nextLeft(vip)
		if vim == nil || vip == nil {
			break
		}
		// The outer contours are stepped in lockstep with the inner ones. In
		// the JavaScript original they are dereferenced unconditionally; here
		// they are nil-guarded, because a Go nil dereference is a panic rather
		// than the exception D3's callers never see in practice.
		vom = nextLeft(vom)
		vop = nextRight(vop)
		if vop != nil {
			vop.ancestor = v
		}
		shift := vim.z + sim - vip.z - sip + sep(vim.node, vip.node)
		if shift > 0 {
			moveSubtree(nextAncestor(vim, v, ancestor), v, shift)
			sip += shift
			sop += shift
		}
		sim += vim.m
		sip += vip.m
		if vom != nil {
			som += vom.m
		}
		if vop != nil {
			sop += vop.m
		}
	}

	if vim != nil && vop != nil && nextRight(vop) == nil {
		vop.thread = vim
		vop.m += sim - sop
	}
	if vip != nil && vom != nil && nextLeft(vom) == nil {
		vom.thread = vip
		vom.m += sip - som
		ancestor = v
	}
	return ancestor
}
