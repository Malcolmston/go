package hierarchy

import "math"

// Tile is a treemap tiling method: given a parent whose rectangle has already
// been decided (and shrunk by any padding), it must set X0, Y0, X1, Y1 on each
// of the parent's children so that they exactly cover the rectangle
// (x0, y0)–(x1, y1) with areas proportional to Value.
//
// The tiling methods shipped here — [Binary], [Dice], [Slice], [SliceDice],
// [Squarify] and [Resquarify] — all satisfy that contract exactly, up to
// floating-point rounding. Custom tilings are supported and only need to honour
// the same invariant; the [Treemap] layout supplies the rectangle, applies
// padding, and recurses on its own.
type Tile[T any] func(parent *Node[T], x0, y0, x1, y1 float64)

// dice divides a rectangle into horizontally adjacent slabs — one column per
// child, full height — with widths proportional to value.
//
// It is factored out of [Dice] to take the value and children explicitly,
// because [Squarify] needs to dice a *row* of children that has no [Node] of its
// own.
func dice[T any](value float64, children []*Node[T], x0, y0, x1, y1 float64) {
	if len(children) == 0 {
		return
	}
	k, total := 0.0, 0.0
	for _, c := range children {
		total += c.Value
	}
	if value != 0 {
		k = (x1 - x0) / value
	}
	for _, c := range children {
		c.Y0, c.Y1 = y0, y1
		c.X0 = x0
		x0 += c.Value * k
		c.X1 = x0
	}
	if exhausts(total, value) {
		// The children account for the whole of the parent's value, so they are
		// meant to cover it exactly; snap the last edge to absorb the rounding
		// that accumulated across the loop. When the parent holds value of its
		// own (total < value) the shortfall is deliberate and is left alone.
		children[len(children)-1].X1 = x1
	}
}

// exhausts reports whether the children's values account for the parent's, up to
// floating-point slop.
func exhausts(total, value float64) bool {
	if value == 0 {
		return total == 0
	}
	return math.Abs(total-value) <= 1e-12*math.Abs(value)
}

// slice divides a rectangle into vertically stacked bands — one row per child,
// full width — with heights proportional to value.
func slice[T any](value float64, children []*Node[T], x0, y0, x1, y1 float64) {
	if len(children) == 0 {
		return
	}
	k, total := 0.0, 0.0
	for _, c := range children {
		total += c.Value
	}
	if value != 0 {
		k = (y1 - y0) / value
	}
	for _, c := range children {
		c.X0, c.X1 = x0, x1
		c.Y0 = y0
		y0 += c.Value * k
		c.Y1 = y0
	}
	if exhausts(total, value) {
		children[len(children)-1].Y1 = y1
	}
}

// Dice lays the children out side by side across the full height of the parent.
// Used on its own it produces very wide, thin rectangles at depth; it is mostly
// useful as a building block, and it is what [Partition] uses for its icicle
// bands.
func Dice[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	dice(parent.Value, parent.Children, x0, y0, x1, y1)
}

// Slice stacks the children top to bottom across the full width of the parent —
// the transpose of [Dice].
func Slice[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	slice(parent.Value, parent.Children, x0, y0, x1, y1)
}

// SliceDice alternates [Slice] at even depths with [Dice] at odd ones. The
// alternation keeps rectangles from degenerating as fast as pure slicing does
// while preserving the input order exactly, which [Squarify] does not — so it is
// the tiling to use when reading order matters more than shape.
func SliceDice[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	if parent.Depth&1 == 1 {
		Slice(parent, x0, y0, x1, y1)
	} else {
		Dice(parent, x0, y0, x1, y1)
	}
}

// Binary recursively splits the children into two groups of as near as possible
// equal value and divides the rectangle along its longer axis between them.
//
// Because the split point is chosen by a binary search over a prefix-sum array
// and the children are never reordered, Binary preserves input order while still
// producing reasonably square rectangles. That combination is what makes it the
// right choice for data with a meaningful order (time, rank, alphabet) where
// [Squarify]'s reshuffling into rows would destroy the reading.
func Binary[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	nodes := parent.Children
	n := len(nodes)
	if n == 0 {
		return
	}
	sums := make([]float64, n+1)
	for i, c := range nodes {
		sums[i+1] = sums[i] + c.Value
	}

	var partition func(i, j int, value, x0, y0, x1, y1 float64)
	partition = func(i, j int, value, x0, y0, x1, y1 float64) {
		if i >= j-1 {
			node := nodes[i]
			node.X0, node.Y0, node.X1, node.Y1 = x0, y0, x1, y1
			return
		}
		valueOffset := sums[i]
		valueTarget := value/2 + valueOffset
		k, hi := i+1, j-1
		for k < hi {
			mid := int(uint(k+hi) >> 1)
			if sums[mid] < valueTarget {
				k = mid + 1
			} else {
				hi = mid
			}
		}
		// Prefer the split that lands closer to the target, as long as the left
		// group keeps at least one child.
		if (valueTarget-sums[k-1]) < (sums[k]-valueTarget) && i+1 < k {
			k--
		}
		valueLeft := sums[k] - valueOffset
		valueRight := value - valueLeft
		if (x1 - x0) > (y1 - y0) {
			xk := x1
			if value != 0 {
				xk = (x0*valueRight + x1*valueLeft) / value
			}
			partition(i, k, valueLeft, x0, y0, xk, y1)
			partition(k, j, valueRight, xk, y0, x1, y1)
		} else {
			yk := y1
			if value != 0 {
				yk = (y0*valueRight + y1*valueLeft) / value
			}
			partition(i, k, valueLeft, x0, y0, x1, yk)
			partition(k, j, valueRight, x0, yk, x1, y1)
		}
	}
	partition(0, n, parent.Value, x0, y0, x1, y1)
}

// phi is the golden ratio, the default target aspect ratio for [Squarify]. It is
// D3's default (from Bruls, Huizing and van Wijk's paper) because rectangles
// near the golden ratio are the easiest to compare by area at a glance — a 1:1
// target sounds better but produces more extreme outliers among the small cells.
const phi = 1.618033988749895 // (1 + sqrt(5)) / 2

// squarifyRow is one row of a squarified layout: a run of consecutive children
// laid out together across the short axis of the remaining space.
type squarifyRow[T any] struct {
	value    float64
	dice     bool
	children []*Node[T]
}

// Squarify tiles with the squarified treemap algorithm at the golden ratio,
// producing rectangles as close to square as the algorithm can manage.
//
// It greedily accumulates children into a row, extending the row while doing so
// improves the worst aspect ratio in it and closing the row as soon as it would
// get worse; each row is then diced or sliced across whichever axis of the
// remaining rectangle is shorter. Squaring is worth wanting because area is much
// easier to judge on a near-square than on a sliver — but the price is that a
// small change in one value can move a child into a different row and visibly
// reorganise the picture, which is exactly what [Resquarify] exists to avoid.
//
// This tiling assumes the children are sorted by descending value; that is what
// the algorithm was designed for and what makes the greedy row-building near
// optimal. Call root.Sort with a descending-Value comparator first.
func Squarify[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	squarifyRatio(phi, parent, x0, y0, x1, y1)
}

// SquarifyRatio returns a [Tile] like [Squarify] but targeting a different
// aspect ratio. Values below 1 are meaningless (a ratio and its reciprocal
// describe the same shape) and are treated as 1.
func SquarifyRatio[T any](ratio float64) Tile[T] {
	if !(ratio > 1) {
		ratio = 1
	}
	return func(parent *Node[T], x0, y0, x1, y1 float64) {
		squarifyRatio(ratio, parent, x0, y0, x1, y1)
	}
}

// squarifyRatio performs the row accumulation and returns the rows, which
// [Resquarify] caches.
func squarifyRatio[T any](ratio float64, parent *Node[T], x0, y0, x1, y1 float64) []squarifyRow[T] {
	nodes := parent.Children
	n := len(nodes)
	if n == 0 {
		return nil
	}
	var rows []squarifyRow[T]
	value := parent.Value
	i0, i1 := 0, 0

	for i0 < n {
		dx, dy := x1-x0, y1-y0

		// Skip leading zero-value children: they contribute no area and would
		// make the aspect-ratio arithmetic degenerate, but they still have to
		// land in a row so they get a (degenerate) rectangle.
		var sumValue float64
		for {
			sumValue = nodes[i1].Value
			i1++
			if sumValue != 0 || i1 >= n {
				break
			}
		}
		minValue, maxValue := sumValue, sumValue
		alpha := math.Max(dy/dx, dx/dy) / (value * ratio)
		beta := sumValue * sumValue * alpha
		minRatio := math.Max(maxValue/beta, beta/minValue)

		// Keep adding children while the worst aspect ratio in the row improves.
		for ; i1 < n; i1++ {
			nodeValue := nodes[i1].Value
			sumValue += nodeValue
			if nodeValue < minValue {
				minValue = nodeValue
			}
			if nodeValue > maxValue {
				maxValue = nodeValue
			}
			beta = sumValue * sumValue * alpha
			newRatio := math.Max(maxValue/beta, beta/minValue)
			if newRatio > minRatio {
				sumValue -= nodeValue
				break
			}
			minRatio = newRatio
		}

		// The children are copied rather than aliased so that a later
		// [Node.Sort] cannot silently rewrite the cached rows [Resquarify]
		// depends on.
		row := squarifyRow[T]{
			value:    sumValue,
			dice:     dx < dy,
			children: append([]*Node[T](nil), nodes[i0:i1]...),
		}
		rows = append(rows, row)
		if row.dice {
			top, bottom := y0, y1
			if value != 0 {
				y0 += dy * sumValue / value
				bottom = y0
			}
			dice(row.value, row.children, x0, top, x1, bottom)
		} else {
			left, right := x0, x1
			if value != 0 {
				x0 += dx * sumValue / value
				right = x0
			}
			slice(row.value, row.children, left, y0, right, y1)
		}
		value -= sumValue
		i0 = i1
	}
	return rows
}

// Resquarify is [Squarify] with the row structure remembered between runs.
//
// On the first layout it behaves exactly like [Squarify]. On any later layout of
// the same tree it reuses the rows it chose the first time, only resizing them
// to the new values and rectangle. That makes it the tiling to use for animation
// or for a treemap whose values update over time: cells grow and shrink smoothly
// instead of jumping into a different row every time the greedy row-builder
// changes its mind. The cost is that the rows drift away from optimal as the
// data moves; re-running with [Squarify] resets them.
//
// The cached rows live in an unexported field on the parent [Node], so a
// [Node.Copy] starts fresh.
func Resquarify[T any](parent *Node[T], x0, y0, x1, y1 float64) {
	resquarifyRatio(phi, parent, x0, y0, x1, y1)
}

// ResquarifyRatio returns a [Tile] like [Resquarify] at a chosen aspect ratio.
// Changing the ratio invalidates any cached rows.
func ResquarifyRatio[T any](ratio float64) Tile[T] {
	if !(ratio > 1) {
		ratio = 1
	}
	return func(parent *Node[T], x0, y0, x1, y1 float64) {
		resquarifyRatio(ratio, parent, x0, y0, x1, y1)
	}
}

func resquarifyRatio[T any](ratio float64, parent *Node[T], x0, y0, x1, y1 float64) {
	if !parent.hasSquarify || parent.squarifyRatio != ratio {
		parent.squarifyRows = squarifyRatio(ratio, parent, x0, y0, x1, y1)
		parent.squarifyRatio = ratio
		parent.hasSquarify = true
		return
	}
	value := parent.Value
	for j := range parent.squarifyRows {
		row := &parent.squarifyRows[j]
		row.value = 0
		for _, c := range row.children {
			row.value += c.Value
		}
		if row.dice {
			top := y0
			bottom := y1
			if value != 0 {
				y0 += (y1 - y0) * row.value / value
				bottom = y0
			}
			dice(row.value, row.children, x0, top, x1, bottom)
		} else {
			left := x0
			right := x1
			if value != 0 {
				x0 += (x1 - x0) * row.value / value
				right = x0
			}
			slice(row.value, row.children, left, y0, right, y1)
		}
		value -= row.value
	}
}
