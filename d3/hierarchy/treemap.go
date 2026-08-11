package hierarchy

import "math"

// Treemap recursively subdivides a rectangle so that every node's area is
// proportional to its Value. It writes X0, Y0, X1, Y1 on every node and touches
// nothing else.
//
// Call [Node.Sum] first — a treemap with no values lays every rectangle out as
// degenerate. Sorting by descending value is also strongly recommended, and is
// required for [Squarify] to do a good job:
//
//	root.Sum(size).Sort(func(a, b *hierarchy.Node[T]) int {
//		return cmp.Compare(b.Value, a.Value)
//	})
//	hierarchy.NewTreemap[T]().Size(960, 500).PaddingInner(1).Layout(root)
//
// # Padding
//
// The padding family looks redundant until you know what each member is for.
// PaddingInner is the gap *between* siblings and is the one you want for a plain
// grid of cells. The outer paddings — PaddingTop, PaddingRight, PaddingBottom,
// PaddingLeft, with PaddingOuter setting all four — are the margin a parent
// keeps *inside* itself before laying out its children, which is what leaves
// room to draw the parent's own border or, with PaddingTop, a label bar above
// the children. Padding sets inner and all four outer paddings at once.
//
// Each of them also has a Func variant taking a func(*Node[T]) float64, because
// a useful treemap often wants padding to depend on depth: a thick frame around
// the top-level groups and hairline gaps further down.
//
// Padding is applied before tiling and is subtracted from the space available to
// the children, so it never causes overlap. When a rectangle is too small to
// absorb the padding it collapses to a zero-width or zero-height line at its
// centre rather than inverting — X0 <= X1 and Y0 <= Y1 always hold.
//
// Round snaps every coordinate to integers after the layout, which is what you
// want when the output goes straight to pixel-aligned SVG rects and you would
// otherwise get antialiased seams between cells. Rounding is applied to the
// finished rectangles, so it can make an area off by up to a pixel; do not
// enable it if you intend to measure the result.
type Treemap[T any] struct {
	tile     Tile[T]
	round    bool
	dx, dy   float64
	padInner func(*Node[T]) float64
	padTop   func(*Node[T]) float64
	padRight func(*Node[T]) float64
	padBotm  func(*Node[T]) float64
	padLeft  func(*Node[T]) float64
}

func constantZero[T any](*Node[T]) float64 { return 0 }

func constant[T any](v float64) func(*Node[T]) float64 {
	return func(*Node[T]) float64 { return v }
}

// NewTreemap returns a treemap layout with D3's defaults: [Squarify] tiling, a
// size of 1×1, no padding and no rounding.
func NewTreemap[T any]() *Treemap[T] {
	return &Treemap[T]{
		tile:     Squarify[T],
		dx:       1,
		dy:       1,
		padInner: constantZero[T],
		padTop:   constantZero[T],
		padRight: constantZero[T],
		padBotm:  constantZero[T],
		padLeft:  constantZero[T],
	}
}

// Size sets the extent of the layout: the root occupies (0, 0) to (width, height).
func (t *Treemap[T]) Size(width, height float64) *Treemap[T] {
	t.dx, t.dy = width, height
	return t
}

// Tile sets the tiling method. Passing nil restores [Squarify].
func (t *Treemap[T]) Tile(tile Tile[T]) *Treemap[T] {
	if tile == nil {
		tile = Squarify[T]
	}
	t.tile = tile
	return t
}

// Round enables or disables integer rounding of the finished rectangles.
func (t *Treemap[T]) Round(round bool) *Treemap[T] { t.round = round; return t }

// Padding sets the inner padding and all four outer paddings to the same value.
func (t *Treemap[T]) Padding(p float64) *Treemap[T] {
	return t.PaddingInner(p).PaddingOuter(p)
}

// PaddingFunc is [Treemap.Padding] with a per-node value.
func (t *Treemap[T]) PaddingFunc(f func(*Node[T]) float64) *Treemap[T] {
	return t.PaddingInnerFunc(f).PaddingOuterFunc(f)
}

// PaddingInner sets the gap between adjacent siblings.
func (t *Treemap[T]) PaddingInner(p float64) *Treemap[T] {
	t.padInner = constant[T](p)
	return t
}

// PaddingInnerFunc is [Treemap.PaddingInner] with a per-node value; the node
// passed is the parent whose children are being separated.
func (t *Treemap[T]) PaddingInnerFunc(f func(*Node[T]) float64) *Treemap[T] {
	t.padInner = orZero(f)
	return t
}

// PaddingOuter sets all four outer paddings.
func (t *Treemap[T]) PaddingOuter(p float64) *Treemap[T] {
	return t.PaddingTop(p).PaddingRight(p).PaddingBottom(p).PaddingLeft(p)
}

// PaddingOuterFunc is [Treemap.PaddingOuter] with a per-node value.
func (t *Treemap[T]) PaddingOuterFunc(f func(*Node[T]) float64) *Treemap[T] {
	return t.PaddingTopFunc(f).PaddingRightFunc(f).PaddingBottomFunc(f).PaddingLeftFunc(f)
}

// PaddingTop sets the margin a parent keeps above its children — the usual place
// to draw a group label.
func (t *Treemap[T]) PaddingTop(p float64) *Treemap[T] {
	t.padTop = constant[T](p)
	return t
}

// PaddingTopFunc is [Treemap.PaddingTop] with a per-node value.
func (t *Treemap[T]) PaddingTopFunc(f func(*Node[T]) float64) *Treemap[T] {
	t.padTop = orZero(f)
	return t
}

// PaddingRight sets the margin a parent keeps to the right of its children.
func (t *Treemap[T]) PaddingRight(p float64) *Treemap[T] {
	t.padRight = constant[T](p)
	return t
}

// PaddingRightFunc is [Treemap.PaddingRight] with a per-node value.
func (t *Treemap[T]) PaddingRightFunc(f func(*Node[T]) float64) *Treemap[T] {
	t.padRight = orZero(f)
	return t
}

// PaddingBottom sets the margin a parent keeps below its children.
func (t *Treemap[T]) PaddingBottom(p float64) *Treemap[T] {
	t.padBotm = constant[T](p)
	return t
}

// PaddingBottomFunc is [Treemap.PaddingBottom] with a per-node value.
func (t *Treemap[T]) PaddingBottomFunc(f func(*Node[T]) float64) *Treemap[T] {
	t.padBotm = orZero(f)
	return t
}

// PaddingLeft sets the margin a parent keeps to the left of its children.
func (t *Treemap[T]) PaddingLeft(p float64) *Treemap[T] {
	t.padLeft = constant[T](p)
	return t
}

// PaddingLeftFunc is [Treemap.PaddingLeft] with a per-node value.
func (t *Treemap[T]) PaddingLeftFunc(f func(*Node[T]) float64) *Treemap[T] {
	t.padLeft = orZero(f)
	return t
}

// orZero replaces a nil accessor with one that always returns 0, so a caller
// clearing a padding does not have to write a closure.
func orZero[T any](f func(*Node[T]) float64) func(*Node[T]) float64 {
	if f == nil {
		return constantZero[T]
	}
	return f
}

// Layout subdivides the rectangle among the subtree rooted at root and returns
// it.
//
// The walk is pre-order: a node's rectangle is always final before its children
// are placed inside it, which is what guarantees that no child can escape its
// parent no matter what a custom [Tile] does — the tile is only ever handed the
// parent's already-shrunk interior.
//
// Half of the inner padding is subtracted from each side of a child's share, so
// that adjacent siblings end up a full inner padding apart while the outermost
// children stay flush with the parent's interior; this is why the padding stack
// records half-values.
func (t *Treemap[T]) Layout(root *Node[T]) *Node[T] {
	if root == nil {
		return nil
	}
	tile := t.tile
	if tile == nil {
		tile = Squarify[T]
	}
	// paddingStack[d] is the half-inner-padding inherited by nodes at depth d.
	// The root inherits none.
	paddingStack := []float64{0}

	root.X0, root.Y0 = 0, 0
	root.X1, root.Y1 = t.dx, t.dy

	root.EachBefore(func(node *Node[T]) {
		d := node.Depth - root.Depth
		for len(paddingStack) <= d {
			paddingStack = append(paddingStack, 0)
		}
		p := paddingStack[d]
		x0, y0 := node.X0+p, node.Y0+p
		x1, y1 := node.X1-p, node.Y1-p
		x0, x1 = collapse(x0, x1)
		y0, y1 = collapse(y0, y1)
		node.X0, node.Y0, node.X1, node.Y1 = x0, y0, x1, y1

		if len(node.Children) == 0 {
			return
		}
		p = t.padInner(node) / 2
		for len(paddingStack) <= d+1 {
			paddingStack = append(paddingStack, 0)
		}
		paddingStack[d+1] = p
		x0 += t.padLeft(node) - p
		y0 += t.padTop(node) - p
		x1 -= t.padRight(node) - p
		y1 -= t.padBotm(node) - p
		x0, x1 = collapse(x0, x1)
		y0, y1 = collapse(y0, y1)
		tile(node, x0, y0, x1, y1)
	})

	if t.round {
		root.EachBefore(roundNode[T])
	}
	return root
}

// collapse keeps an interval non-inverting: if padding has eaten more than the
// available extent, the interval degenerates to its midpoint instead of turning
// inside out, which would produce negative-area rectangles a renderer cannot
// draw.
func collapse(lo, hi float64) (float64, float64) {
	if hi < lo {
		mid := (lo + hi) / 2
		return mid, mid
	}
	return lo, hi
}

// roundNode snaps a rectangle to integer coordinates.
func roundNode[T any](node *Node[T]) {
	node.X0 = math.Round(node.X0)
	node.Y0 = math.Round(node.Y0)
	node.X1 = math.Round(node.X1)
	node.Y1 = math.Round(node.Y1)
}
