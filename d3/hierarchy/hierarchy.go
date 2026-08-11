// Package hierarchy is a Go port of d3-hierarchy — the part of D3 that turns
// tree-shaped data into coordinates a renderer can draw. It contains no drawing
// code and no DOM: every layout in here reads a tree of [Node] values and writes
// numbers back onto those nodes, which something else (in this repo, the React
// port emitting SVG) is free to turn into pixels.
//
// The starting point is always a [Node]. You build one either from nested data
// with [Hierarchy], which walks a children accessor you supply:
//
//	root := hierarchy.Hierarchy(data, func(d *Dir) []*Dir { return d.Subdirs })
//
// or from flat rows with [Stratify], which joins each row to its parent by id:
//
//	root, err := hierarchy.Stratify(rows,
//		func(r Row) string { return r.ID },
//		func(r Row) string { return r.ParentID })
//
// Either way you get a root [Node] whose Depth, Height, Parent and Children are
// filled in, and whose Data field holds your own value — [Node] is generic in
// that type, so nothing is lost to `any` and no type assertion is needed in the
// accessors you write.
//
// Before most layouts will do anything useful the tree needs values: call
// [Node.Sum] to derive each node's Value from its data (the classic "size of a
// file, summed up the directory tree"), or [Node.Count] to make every leaf worth
// 1. [Node.Sort] then fixes sibling order, which for the area-based layouts is
// what makes the output stable and readable.
//
// # Where the coordinates live
//
// This is the one place the port deliberately looks different from the original,
// and it is worth understanding before reading any of the layout code.
//
// JavaScript D3 writes layout output onto the node objects as dynamic
// properties: tree and cluster set node.x and node.y, treemap and partition set
// node.x0/x1/y0/y1, pack sets node.x/y/r. Go has no dynamic properties, so there
// were two options — return a parallel structure keyed by node, or declare the
// fields up front on [Node]. This package declares them up front: [Node] carries
// X, Y, R and X0, Y0, X1, Y1 alongside Value, and each layout writes the subset
// it owns.
//
// The alternative — a `map[*Node[T]]Rect` returned from each layout — was
// rejected because it makes the common operations awkward for no real gain. A
// renderer walking the tree with [Node.Descendants] would have to thread the map
// through every call site; [Node.Links] would return node pairs whose
// coordinates you then have to look up twice per edge; and nothing would stop
// the map from going stale relative to the tree it describes. Fields on the node
// keep the D3 idiom intact (`for _, n := range root.Descendants() { draw(n.X, n.Y) }`),
// cost a handful of float64s per node, and make the port readable next to the
// original source.
//
// The price is that [Node] has fields that are meaningless for the layout you
// happen to be running: X and Y are zero after a [Treemap], X0..Y1 are zero
// after a [Tree]. Each layout's doc comment states exactly which fields it
// writes; treat the others as undefined rather than as zero. Running two layouts
// over the same tree is allowed and simply overwrites the overlapping fields —
// running [Tree] then [Cluster] leaves you with cluster coordinates.
//
// # Layouts
//
//   - [Tree] — the Reingold–Tilford "tidy" algorithm in Buchheim et al.'s linear
//     formulation. Writes X and Y.
//   - [Cluster] — dendrogram; all leaves on one line. Writes X and Y.
//   - [Treemap] — nested rectangles, with the [Binary], [Dice], [Slice],
//     [SliceDice], [Squarify] and [Resquarify] tilings and the full padding
//     family. Writes X0, Y0, X1, Y1.
//   - [Partition] — icicle/sunburst; also rectangles. Writes X0, Y0, X1, Y1.
//   - [Pack] — enclosure diagram via the front-chain algorithm and Welzl's
//     smallest-enclosing-circle. Writes X, Y and R.
//
// Every layout is a struct with chainable setters and a Layout method, so the
// D3 shape survives:
//
//	hierarchy.NewTreemap[Row]().Size(960, 500).PaddingInner(1).Layout(root)
//
// The layouts are pure functions of the tree plus their configuration and hold
// no global state, but they mutate the nodes they are given, so a single tree
// must not be laid out from two goroutines at once.
package hierarchy

import "slices"

// Node is one vertex of a hierarchy, generic in the caller's data type.
//
// The structural fields (Data, Depth, Height, Parent, Children) are filled in by
// [Hierarchy] or [Stratify] and by [Node.Copy]. Value is filled in by
// [Node.Sum] or [Node.Count]. The geometry fields are filled in by whichever
// layout you run; see the package doc for why they live here rather than in a
// returned side table, and each layout's doc for which of them it writes.
//
// A Node is not safe for concurrent mutation. Reading a laid-out tree from
// several goroutines is fine.
type Node[T any] struct {
	// Data is the caller's value for this node — whatever was passed to
	// [Hierarchy] or the row that [Stratify] consumed.
	Data T

	// Depth is the number of edges from the root: 0 at the root.
	Depth int

	// Height is the number of edges on the longest path to a descendant leaf:
	// 0 at every leaf.
	Height int

	// Parent is nil at the root.
	Parent *Node[T]

	// Children is nil (not empty) for a leaf, matching D3, so `if n.Children
	// != nil` and `if len(n.Children) > 0` agree.
	Children []*Node[T]

	// Value is the aggregate set by [Node.Sum] or [Node.Count]. Treemap,
	// partition and pack all size nodes by it, so it must be non-negative and
	// a parent's value should be at least the sum of its children's.
	Value float64

	// X and Y are the node position written by [Tree] and [Cluster]; X, Y and
	// R together are the circle written by [Pack].
	X, Y, R float64

	// X0, Y0, X1, Y1 are the rectangle written by [Treemap] and [Partition],
	// with X0 <= X1 and Y0 <= Y1.
	X0, Y0, X1, Y1 float64

	// squarifyRows caches the row structure computed by [Squarify] so that
	// [Resquarify] can reuse it on a later pass; see resquarify's doc for what
	// that buys. Unexported because it is an implementation detail of one
	// tiling method, and cleared whenever the ratio changes.
	squarifyRows  []squarifyRow[T]
	squarifyRatio float64
	hasSquarify   bool
}

// Link is one parent→child edge, as produced by [Node.Links]. Renderers draw an
// edge per link; the coordinates come from the endpoints' own fields.
type Link[T any] struct {
	Source *Node[T]
	Target *Node[T]
}

// Hierarchy builds a tree from nested data by repeatedly applying a children
// accessor, then computes Depth and Height for every node.
//
// children is called exactly once per node and may return nil for a leaf. It
// must not produce a cycle — Hierarchy walks eagerly and would not terminate.
// Because the accessor is a plain function the data does not have to be a
// literal tree of structs; it can index into a map, filter, or unwrap an
// interface, as long as the result is acyclic.
//
// The returned root has Value 0 everywhere; call [Node.Sum] or [Node.Count]
// before running an area-based layout.
func Hierarchy[T any](data T, children func(T) []T) *Node[T] {
	root := &Node[T]{Data: data}
	stack := []*Node[T]{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var kids []T
		if children != nil {
			kids = children(node.Data)
		}
		if len(kids) == 0 {
			continue
		}
		node.Children = make([]*Node[T], len(kids))
		for i := len(kids) - 1; i >= 0; i-- {
			child := &Node[T]{Data: kids[i], Parent: node, Depth: node.Depth + 1}
			node.Children[i] = child
			stack = append(stack, child)
		}
	}
	return root.computeHeights()
}

// computeHeights fills in Height for every node in the subtree.
//
// It uses D3's trick of walking in pre-order and pushing each node's height up
// the ancestor chain, stopping as soon as an ancestor already claims a height at
// least as large. That is O(n) overall rather than the O(n·depth) a naive
// "recompute from every leaf" would cost, because each ancestor is only revisited
// while its recorded height is still too small.
func (n *Node[T]) computeHeights() *Node[T] {
	n.EachBefore(func(node *Node[T]) {
		height := 0
		for {
			node.Height = height
			node = node.Parent
			height++
			if node == nil || node.Height >= height {
				break
			}
		}
	})
	return n
}

// Each invokes f for every node in the subtree in breadth-first order: the node
// itself, then its children, then its grandchildren.
//
// Breadth-first is the D3 default for `each` and is the right traversal when the
// callback cares about depth — for example assigning a color per level, or
// stopping once a depth budget is spent. For value aggregation you want
// [Node.EachAfter] instead, and for anything that pushes information downward
// (inherited scales, cumulative angles) you want [Node.EachBefore].
//
// Unlike D3 the callback receives no index; range over [Node.Descendants] if you
// need one.
func (n *Node[T]) Each(f func(*Node[T])) *Node[T] {
	queue := []*Node[T]{n}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		f(node)
		queue = append(queue, node.Children...)
	}
	return n
}

// EachBefore invokes f for every node in pre-order: a node before any of its
// descendants. This is the traversal for propagating information down the tree,
// and it is what the treemap and partition layouts use to subdivide a parent's
// rectangle before recursing into it.
func (n *Node[T]) EachBefore(f func(*Node[T])) *Node[T] {
	stack := []*Node[T]{n}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		f(node)
		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
	}
	return n
}

// EachAfter invokes f for every node in post-order: a node after all of its
// descendants. This is the traversal for aggregating information up the tree —
// [Node.Sum] and [Node.Count] are both a single EachAfter — and for the packing
// layout, which must know how big a subtree is before it can place it.
func (n *Node[T]) EachAfter(f func(*Node[T])) *Node[T] {
	stack := []*Node[T]{n}
	var order []*Node[T]
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		order = append(order, node)
		stack = append(stack, node.Children...)
	}
	for i := len(order) - 1; i >= 0; i-- {
		f(order[i])
	}
	return n
}

// Sum sets every node's Value to value(node.Data) plus the sum of its children's
// values, working leaves-first so a parent always sees final child values.
//
// The accessor is applied to internal nodes too, which is how D3 behaves and is
// occasionally useful (a directory that has its own size), but the common case
// returns 0 for anything with children. Negative results are clamped to 0:
// treemap and pack divide by Value and interpret it as an area, and a negative
// area has no meaning — silently clamping matches D3 and keeps a stray -1 in the
// data from producing inverted rectangles.
func (n *Node[T]) Sum(value func(T) float64) *Node[T] {
	return n.EachAfter(func(node *Node[T]) {
		sum := value(node.Data)
		if sum < 0 {
			sum = 0
		}
		for _, c := range node.Children {
			sum += c.Value
		}
		node.Value = sum
	})
}

// Count sets every leaf's Value to 1 and every internal node's Value to the
// number of leaves beneath it. It is the standard way to lay out a tree whose
// data carries no natural weight.
func (n *Node[T]) Count() *Node[T] {
	return n.EachAfter(func(node *Node[T]) {
		if len(node.Children) == 0 {
			node.Value = 1
			return
		}
		sum := 0.0
		for _, c := range node.Children {
			sum += c.Value
		}
		node.Value = sum
	})
}

// Sort sorts every node's children in place using cmp, which follows the
// [slices.SortStableFunc] convention: negative if a sorts before b.
//
// Sorting is stable, so equal siblings keep their input order and the layout of
// unchanged data does not jitter between runs. Sort by descending Value for a
// conventional treemap — but note that Sort reads Value, so call [Node.Sum]
// first.
func (n *Node[T]) Sort(cmp func(a, b *Node[T]) int) *Node[T] {
	return n.EachBefore(func(node *Node[T]) {
		if node.Children != nil {
			slices.SortStableFunc(node.Children, cmp)
		}
	})
}

// Ancestors returns this node and every proper ancestor, starting here and
// ending at the root.
func (n *Node[T]) Ancestors() []*Node[T] {
	var out []*Node[T]
	for node := n; node != nil; node = node.Parent {
		out = append(out, node)
	}
	return out
}

// Descendants returns this node and every node beneath it, in breadth-first
// order (the same order as [Node.Each]).
func (n *Node[T]) Descendants() []*Node[T] {
	var out []*Node[T]
	n.Each(func(node *Node[T]) { out = append(out, node) })
	return out
}

// Leaves returns every node in the subtree with no children, in breadth-first
// order.
func (n *Node[T]) Leaves() []*Node[T] {
	var out []*Node[T]
	n.Each(func(node *Node[T]) {
		if len(node.Children) == 0 {
			out = append(out, node)
		}
	})
	return out
}

// Find returns the first node in the subtree for which match reports true,
// searching breadth-first, or nil if there is none. Breadth-first means the
// shallowest match wins, which is usually what "find the node for this id"
// wants.
func (n *Node[T]) Find(match func(*Node[T]) bool) *Node[T] {
	queue := []*Node[T]{n}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if match(node) {
			return node
		}
		queue = append(queue, node.Children...)
	}
	return nil
}

// Path returns the shortest path from this node to end through their least
// common ancestor: up from here to the ancestor, then down to end. Both
// endpoints are included, and the ancestor appears exactly once.
//
// If the two nodes are in different trees there is no common ancestor and Path
// returns nil, rather than the ragged half-path D3 would produce.
func (n *Node[T]) Path(end *Node[T]) []*Node[T] {
	if end == nil {
		return nil
	}
	ancestor := leastCommonAncestor(n, end)
	if ancestor == nil {
		return nil
	}
	var up []*Node[T]
	for node := n; node != ancestor; node = node.Parent {
		up = append(up, node)
	}
	var down []*Node[T]
	for node := end; node != ancestor; node = node.Parent {
		down = append(down, node)
	}
	out := append(up, ancestor)
	for i := len(down) - 1; i >= 0; i-- {
		out = append(out, down[i])
	}
	return out
}

// leastCommonAncestor returns the deepest node that is an ancestor-or-self of
// both a and b, or nil when they belong to different trees.
func leastCommonAncestor[T any](a, b *Node[T]) *Node[T] {
	if a == b {
		return a
	}
	seen := make(map[*Node[T]]bool)
	for node := a; node != nil; node = node.Parent {
		seen[node] = true
	}
	for node := b; node != nil; node = node.Parent {
		if seen[node] {
			return node
		}
	}
	return nil
}

// Links returns one [Link] per edge in the subtree — that is, one per node other
// than this one, pairing it with its parent. The order matches [Node.Each].
func (n *Node[T]) Links() []Link[T] {
	var out []Link[T]
	n.Each(func(node *Node[T]) {
		if node != n && node.Parent != nil {
			out = append(out, Link[T]{Source: node.Parent, Target: node})
		}
	})
	return out
}

// Copy returns a new tree with the same shape and the same Data values, rooted
// at a copy of this node. The copy is deep in structure and shallow in data: the
// two trees share whatever T points at.
//
// Following D3, only structure and data survive the copy. Value is not carried
// over and neither are any layout coordinates, because a copy is normally taken
// so that a *different* value accessor or layout can be applied to the same
// data; carrying stale numbers across would be a trap. Depth is recomputed
// relative to the new root, so copying a subtree gives you a tree whose root is
// at depth 0.
func (n *Node[T]) Copy() *Node[T] {
	root := &Node[T]{Data: n.Data}
	type pair struct{ src, dst *Node[T] }
	stack := []pair{{n, root}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if p.src.Children == nil {
			continue
		}
		p.dst.Children = make([]*Node[T], len(p.src.Children))
		for i, c := range p.src.Children {
			dc := &Node[T]{Data: c.Data, Parent: p.dst, Depth: p.dst.Depth + 1}
			p.dst.Children[i] = dc
			stack = append(stack, pair{c, dc})
		}
	}
	return root.computeHeights()
}
