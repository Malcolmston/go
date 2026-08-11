package hierarchy

import (
	"cmp"
	"math"
	"testing"
)

// rectTree is the fixture for the area layouts: enough branching and enough
// spread in the values that a tiling method has real choices to make.
func rectTree() *Node[*tdata] {
	return Hierarchy(branch("root",
		branch("a", leaf("a1", 6), leaf("a2", 6), leaf("a3", 4), leaf("a4", 3), leaf("a5", 2), leaf("a6", 2), leaf("a7", 1)),
		branch("b",
			branch("b1", leaf("b1a", 8), leaf("b1b", 5)),
			leaf("b2", 11),
			leaf("b3", 1),
		),
		leaf("c", 20),
		branch("d", leaf("d1", 3), leaf("d2", 3)),
	), kids).Sum(func(d *tdata) float64 { return d.weight })
}

func sortedRectTree() *Node[*tdata] {
	return rectTree().Sort(func(a, b *Node[*tdata]) int { return cmp.Compare(b.Value, a.Value) })
}

func area[T any](n *Node[T]) float64 { return (n.X1 - n.X0) * (n.Y1 - n.Y0) }

// checkRectSanity asserts that every rectangle is non-inverted and that every
// child is contained in its parent.
func checkRectSanity[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		if n.X1 < n.X0-eps || n.Y1 < n.Y0-eps {
			t.Errorf("depth %d: inverted rect [%v,%v]x[%v,%v]", n.Depth, n.X0, n.X1, n.Y0, n.Y1)
		}
		if p := n.Parent; p != nil {
			if n.X0 < p.X0-eps || n.X1 > p.X1+eps || n.Y0 < p.Y0-eps || n.Y1 > p.Y1+eps {
				t.Errorf("depth %d: child [%v,%v]x[%v,%v] escapes parent [%v,%v]x[%v,%v]",
					n.Depth, n.X0, n.X1, n.Y0, n.Y1, p.X0, p.X1, p.Y0, p.Y1)
			}
		}
	})
}

// checkNoRectOverlap asserts that no two siblings overlap. Only siblings need
// checking: containment (above) plus sibling disjointness inductively gives
// global disjointness of the leaves.
func checkNoRectOverlap[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		cs := n.Children
		for i := 0; i < len(cs); i++ {
			for j := i + 1; j < len(cs); j++ {
				a, b := cs[i], cs[j]
				ox := math.Min(a.X1, b.X1) - math.Max(a.X0, b.X0)
				oy := math.Min(a.Y1, b.Y1) - math.Max(a.Y0, b.Y0)
				if ox > eps && oy > eps {
					t.Errorf("siblings at depth %d overlap by %vx%v", a.Depth, ox, oy)
				}
			}
		}
	})
}

// checkExactTiling asserts that a parent's children cover it completely: the
// areas add up and nothing is left over. Combined with the containment and
// overlap checks this is a full "exactly tiles" proof — cover the same area,
// stay inside, and never double up.
func checkExactTiling[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		if len(n.Children) == 0 {
			return
		}
		sum := 0.0
		for _, c := range n.Children {
			sum += area(c)
		}
		want := area(n)
		if math.Abs(sum-want) > 1e-9*math.Max(1, want) {
			t.Errorf("depth %d: children cover %v of a %v parent", n.Depth, sum, want)
		}
	})
}

// checkAreaProportional asserts the headline treemap property: a node's area is
// its share of the root's value.
func checkAreaProportional[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	total := area(root)
	root.Each(func(n *Node[T]) {
		want := total * n.Value / root.Value
		if math.Abs(area(n)-want) > 1e-9*math.Max(1, total) {
			t.Errorf("depth %d: area %v for value %v, want %v", n.Depth, area(n), n.Value, want)
		}
	})
}

func tilings() []struct {
	name string
	tile Tile[*tdata]
} {
	return []struct {
		name string
		tile Tile[*tdata]
	}{
		{"Binary", Binary[*tdata]},
		{"Dice", Dice[*tdata]},
		{"Slice", Slice[*tdata]},
		{"SliceDice", SliceDice[*tdata]},
		{"Squarify", Squarify[*tdata]},
		{"SquarifyRatio(1)", SquarifyRatio[*tdata](1)},
		{"Resquarify", Resquarify[*tdata]},
	}
}

func TestTreemapExactCoordinates(t *testing.T) {
	// Two children of values 1 and 3 in a 4x2 box. With Dice the split is
	// horizontal at x=1; with Slice it is vertical at y=0.5.
	data := branch("r", leaf("a", 1), leaf("b", 3))

	type rect struct {
		name           string
		x0, y0, x1, y1 float64
	}
	tests := []struct {
		name string
		tile Tile[*tdata]
		want []rect
	}{
		{"Dice", Dice[*tdata], []rect{
			{"r", 0, 0, 4, 2},
			{"a", 0, 0, 1, 2},
			{"b", 1, 0, 4, 2},
		}},
		{"Slice", Slice[*tdata], []rect{
			{"r", 0, 0, 4, 2},
			{"a", 0, 0, 4, 0.5},
			{"b", 0, 0.5, 4, 2},
		}},
		{"SliceDice at depth 0 dices", SliceDice[*tdata], []rect{
			{"a", 0, 0, 1, 2},
			{"b", 1, 0, 4, 2},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewTreemap[*tdata]().Size(4, 2).Tile(tt.tile).
				Layout(Hierarchy(data, kids).Sum(func(d *tdata) float64 { return d.weight }))
			for _, w := range tt.want {
				n := find(t, root, w.name)
				if !closeTo(n.X0, w.x0) || !closeTo(n.Y0, w.y0) || !closeTo(n.X1, w.x1) || !closeTo(n.Y1, w.y1) {
					t.Errorf("%s = [%v,%v]x[%v,%v], want [%v,%v]x[%v,%v]",
						w.name, n.X0, n.X1, n.Y0, n.Y1, w.x0, w.x1, w.y0, w.y1)
				}
			}
		})
	}

	t.Run("single leaf fills the box", func(t *testing.T) {
		root := NewTreemap[*tdata]().Size(10, 5).
			Layout(Hierarchy(leaf("only", 1), kids).Sum(func(d *tdata) float64 { return d.weight }))
		if root.X0 != 0 || root.Y0 != 0 || root.X1 != 10 || root.Y1 != 5 {
			t.Errorf("got [%v,%v]x[%v,%v], want [0,10]x[0,5]", root.X0, root.X1, root.Y0, root.Y1)
		}
	})

	t.Run("inner padding of 2 in a 10x2 box with Dice", func(t *testing.T) {
		// Inner padding works by expanding the area handed to the tiling by
		// half the padding on every side, then shrinking each child back by the
		// same half. The outer children therefore stay flush with the box while
		// the two of them end up a full padding apart: the tiling runs over
		// [-1,11], putting a at [-1,2] and b at [2,11], and shrinking each side
		// by 1 gives a = [0,1] and b = [3,10].
		root := NewTreemap[*tdata]().Size(10, 2).Tile(Dice[*tdata]).PaddingInner(2).
			Layout(Hierarchy(data, kids).Sum(func(d *tdata) float64 { return d.weight }))
		a, b := find(t, root, "a"), find(t, root, "b")
		if !closeTo(a.X0, 0) || !closeTo(a.X1, 1) {
			t.Errorf("a x = [%v,%v], want [0,1]", a.X0, a.X1)
		}
		if !closeTo(b.X0, 3) || !closeTo(b.X1, 10) {
			t.Errorf("b x = [%v,%v], want [3,10]", b.X0, b.X1)
		}
		if got := b.X0 - a.X1; !closeTo(got, 2) {
			t.Errorf("gap = %v, want the full inner padding of 2", got)
		}
	})
}

func TestTreemapInvariants(t *testing.T) {
	for _, tc := range tilings() {
		t.Run(tc.name, func(t *testing.T) {
			root := NewTreemap[*tdata]().Size(960, 500).Tile(tc.tile).Layout(sortedRectTree())
			checkRectSanity(t, root)
			checkNoRectOverlap(t, root)
			checkExactTiling(t, root)
			checkAreaProportional(t, root)
			if root.X0 != 0 || root.Y0 != 0 || root.X1 != 960 || root.Y1 != 500 {
				t.Errorf("root = [%v,%v]x[%v,%v], want the full 960x500 box",
					root.X0, root.X1, root.Y0, root.Y1)
			}
		})
	}
}

func TestTreemapPaddingKeepsRectsValid(t *testing.T) {
	// Padding removes area, so exact tiling and proportionality no longer hold,
	// but containment and non-overlap must survive — that is the whole point of
	// subtracting padding before tiling rather than after.
	tests := []struct {
		name  string
		build func() *Treemap[*tdata]
	}{
		{"inner", func() *Treemap[*tdata] { return NewTreemap[*tdata]().PaddingInner(3) }},
		{"outer", func() *Treemap[*tdata] { return NewTreemap[*tdata]().PaddingOuter(5) }},
		{"top only", func() *Treemap[*tdata] { return NewTreemap[*tdata]().PaddingTop(14) }},
		{"all sides", func() *Treemap[*tdata] { return NewTreemap[*tdata]().Padding(4) }},
		{"asymmetric", func() *Treemap[*tdata] {
			return NewTreemap[*tdata]().PaddingTop(12).PaddingLeft(3).PaddingRight(1).PaddingBottom(7).PaddingInner(2)
		}},
		{"by depth", func() *Treemap[*tdata] {
			return NewTreemap[*tdata]().PaddingInnerFunc(func(n *Node[*tdata]) float64 {
				return float64(6 - 2*n.Depth)
			})
		}},
		{"absurdly large", func() *Treemap[*tdata] { return NewTreemap[*tdata]().Padding(400) }},
	}

	for _, tt := range tests {
		for _, tc := range tilings() {
			t.Run(tt.name+"/"+tc.name, func(t *testing.T) {
				root := tt.build().Size(960, 500).Tile(tc.tile).Layout(sortedRectTree())
				checkRectSanity(t, root)
				checkNoRectOverlap(t, root)
			})
		}
	}
}

func TestTreemapPaddingSeparatesSiblings(t *testing.T) {
	// With inner padding p, adjacent siblings must be at least p apart along
	// whichever axis separates them.
	const p = 6
	root := NewTreemap[*tdata]().Size(960, 500).PaddingInner(p).Layout(sortedRectTree())
	root.Each(func(n *Node[*tdata]) {
		cs := n.Children
		for i := 0; i < len(cs); i++ {
			for j := i + 1; j < len(cs); j++ {
				a, b := cs[i], cs[j]
				gapX := math.Max(a.X0, b.X0) - math.Min(a.X1, b.X1)
				gapY := math.Max(a.Y0, b.Y0) - math.Min(a.Y1, b.Y1)
				if math.Max(gapX, gapY) < p-1e-6 {
					t.Errorf("siblings at depth %d are only %v apart, want %v",
						a.Depth, math.Max(gapX, gapY), p)
				}
			}
		}
	})
}

func TestTreemapRound(t *testing.T) {
	root := NewTreemap[*tdata]().Size(960, 500).Round(true).Layout(sortedRectTree())
	root.Each(func(n *Node[*tdata]) {
		for _, v := range []float64{n.X0, n.Y0, n.X1, n.Y1} {
			if v != math.Trunc(v) {
				t.Errorf("rounded layout produced %v", v)
			}
		}
	})
	checkRectSanity(t, root)
	checkNoRectOverlap(t, root)
}

func TestTreemapZeroValues(t *testing.T) {
	// Zero-valued children are legal input and must produce degenerate but
	// valid rectangles rather than NaNs.
	root := Hierarchy(branch("r", leaf("a", 0), leaf("b", 0), leaf("c", 0)), kids).
		Sum(func(d *tdata) float64 { return d.weight })
	for _, tc := range tilings() {
		t.Run(tc.name, func(t *testing.T) {
			r := NewTreemap[*tdata]().Size(100, 100).Tile(tc.tile).Layout(root)
			r.Each(func(n *Node[*tdata]) {
				for _, v := range []float64{n.X0, n.Y0, n.X1, n.Y1} {
					if math.IsNaN(v) || math.IsInf(v, 0) {
						t.Fatalf("non-finite coordinate %v at depth %d", v, n.Depth)
					}
				}
			})
			checkRectSanity(t, r)
		})
	}
}

func TestResquarifyReusesRows(t *testing.T) {
	// Laying the same tree out twice with Resquarify must keep the row
	// structure: after halving one value, cells resize but no cell jumps to a
	// different row. We detect a row change by checking that every child which
	// shared a Y band with another still does.
	root := sortedRectTree()
	tm := NewTreemap[*tdata]().Size(600, 400).Tile(Resquarify[*tdata])
	tm.Layout(root)

	a := find(t, root, "a")
	bands := map[string][2]float64{}
	for _, c := range a.Children {
		bands[c.Data.name] = [2]float64{c.Y0, c.Y1}
	}
	sameRow := func(m map[string][2]float64, x, y string) bool {
		return m[x][0] == m[y][0] && m[x][1] == m[y][1]
	}

	// Perturb the data and re-run.
	find(t, root, "a1").Data.weight = 3
	root.Sum(func(d *tdata) float64 { return d.weight })
	tm.Layout(root)

	after := map[string][2]float64{}
	for _, c := range a.Children {
		after[c.Data.name] = [2]float64{c.Y0, c.Y1}
	}
	for i := range a.Children {
		for j := i + 1; j < len(a.Children); j++ {
			x, y := a.Children[i].Data.name, a.Children[j].Data.name
			if sameRow(bands, x, y) != sameRow(after, x, y) {
				t.Errorf("%s and %s changed rows between layouts", x, y)
			}
		}
	}
	checkRectSanity(t, root)
	checkNoRectOverlap(t, root)
	checkExactTiling(t, root)
}

func TestPartitionExactCoordinates(t *testing.T) {
	// Root of value 4 with children 1 and 3 in a 4x2 box. Height is 1, so there
	// are 2 bands of height 1: the root occupies y [0,1] and its children
	// y [1,2], split at x=1.
	root := NewPartition[*tdata]().Size(4, 2).Layout(
		Hierarchy(branch("r", leaf("a", 1), leaf("b", 3)), kids).
			Sum(func(d *tdata) float64 { return d.weight }))

	tests := []struct {
		name           string
		x0, y0, x1, y1 float64
	}{
		{"r", 0, 0, 4, 1},
		{"a", 0, 1, 1, 2},
		{"b", 1, 1, 4, 2},
	}
	for _, tt := range tests {
		n := find(t, root, tt.name)
		if !closeTo(n.X0, tt.x0) || !closeTo(n.Y0, tt.y0) || !closeTo(n.X1, tt.x1) || !closeTo(n.Y1, tt.y1) {
			t.Errorf("%s = [%v,%v]x[%v,%v], want [%v,%v]x[%v,%v]",
				tt.name, n.X0, n.X1, n.Y0, n.Y1, tt.x0, tt.x1, tt.y0, tt.y1)
		}
	}
}

// checkPartitionSanity is the partition analogue of [checkRectSanity]. A
// partition deliberately places children in the *next* band rather than inside
// the parent's own rectangle, so vertical containment does not apply; what must
// hold is that no rectangle is inverted and that a child's horizontal extent
// stays within its parent's.
func checkPartitionSanity[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		if n.X1 < n.X0-eps || n.Y1 < n.Y0-eps {
			t.Errorf("depth %d: inverted rect [%v,%v]x[%v,%v]", n.Depth, n.X0, n.X1, n.Y0, n.Y1)
		}
		if p := n.Parent; p != nil {
			if n.X0 < p.X0-eps || n.X1 > p.X1+eps {
				t.Errorf("depth %d: child x [%v,%v] escapes parent x [%v,%v]", n.Depth, n.X0, n.X1, p.X0, p.X1)
			}
			if n.Y0 < p.Y1-eps {
				t.Errorf("depth %d: child band starts at %v, before the parent's ends at %v", n.Depth, n.Y0, p.Y1)
			}
		}
	})
}

func TestPartitionInvariants(t *testing.T) {
	root := NewPartition[*tdata]().Size(900, 400).Layout(sortedRectTree())

	checkPartitionSanity(t, root)
	checkNoRectOverlap(t, root)

	t.Run("children exactly subdivide the parent's width", func(t *testing.T) {
		root.Each(func(n *Node[*tdata]) {
			if len(n.Children) == 0 {
				return
			}
			// Contiguous, in order, spanning exactly the parent.
			if !closeTo(n.Children[0].X0, n.X0) {
				t.Errorf("depth %d: first child starts at %v, want %v", n.Depth, n.Children[0].X0, n.X0)
			}
			last := n.Children[len(n.Children)-1]
			if !closeTo(last.X1, n.X1) {
				t.Errorf("depth %d: last child ends at %v, want %v", n.Depth, last.X1, n.X1)
			}
			for i := 1; i < len(n.Children); i++ {
				if !closeTo(n.Children[i].X0, n.Children[i-1].X1) {
					t.Errorf("depth %d: gap between children at %v/%v",
						n.Depth, n.Children[i-1].X1, n.Children[i].X0)
				}
			}
		})
	})

	t.Run("bands follow depth", func(t *testing.T) {
		n := float64(root.Height + 1)
		bandHeight := 400 / n
		root.Each(func(nd *Node[*tdata]) {
			wantY0 := float64(nd.Depth) * bandHeight
			if !closeTo(nd.Y0, wantY0) || !closeTo(nd.Y1, wantY0+bandHeight) {
				t.Errorf("depth %d band = [%v,%v], want [%v,%v]",
					nd.Depth, nd.Y0, nd.Y1, wantY0, wantY0+bandHeight)
			}
		})
	})

	t.Run("width is proportional to value", func(t *testing.T) {
		root.Each(func(nd *Node[*tdata]) {
			want := 900 * nd.Value / root.Value
			if math.Abs((nd.X1-nd.X0)-want) > 1e-9*900 {
				t.Errorf("depth %d: width %v for value %v, want %v", nd.Depth, nd.X1-nd.X0, nd.Value, want)
			}
		})
	})
}

func TestPartitionPaddingAndRound(t *testing.T) {
	root := NewPartition[*tdata]().Size(900, 400).Padding(2).Layout(sortedRectTree())
	checkPartitionSanity(t, root)
	checkNoRectOverlap(t, root)

	t.Run("padding cannot invert a rect", func(t *testing.T) {
		// Padding far larger than the box: every rectangle collapses to a point
		// but none may turn inside out.
		r := NewPartition[*tdata]().Size(20, 20).Padding(100).Layout(sortedRectTree())
		r.Each(func(n *Node[*tdata]) {
			if n.X1 < n.X0-eps || n.Y1 < n.Y0-eps {
				t.Errorf("depth %d: inverted rect [%v,%v]x[%v,%v]", n.Depth, n.X0, n.X1, n.Y0, n.Y1)
			}
		})
	})

	t.Run("round", func(t *testing.T) {
		r := NewPartition[*tdata]().Size(900, 400).Round(true).Layout(sortedRectTree())
		r.Each(func(n *Node[*tdata]) {
			for _, v := range []float64{n.X0, n.Y0, n.X1, n.Y1} {
				if v != math.Trunc(v) {
					t.Errorf("rounded layout produced %v", v)
				}
			}
		})
	})
}
