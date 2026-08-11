package hierarchy

import (
	"math"
	"sort"
	"testing"
)

const eps = 1e-9

func closeTo(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

// bigTree is a deliberately lopsided fixture: a wide shallow branch next to a
// deep narrow one, plus a subtree that is duplicated so isomorphic-subtree
// behaviour is exercised. Irregularity is what forces the tidy layout's
// collision handling to actually do something.
func bigTree() *Node[*tdata] {
	twin := func(p string) *tdata {
		return branch(p,
			branch(p+"x", leaf(p+"x1", 1), leaf(p+"x2", 1)),
			leaf(p+"y", 1),
		)
	}
	return Hierarchy(branch("root",
		branch("wide", leaf("w1", 1), leaf("w2", 1), leaf("w3", 1), leaf("w4", 1), leaf("w5", 1)),
		branch("deep", branch("d1", branch("d2", branch("d3", leaf("d4", 1))))),
		twin("t"),
		twin("u"),
		leaf("solo", 1),
	), kids)
}

// byDepth groups a laid-out tree's nodes by depth, each group sorted by X.
func byDepth[T any](root *Node[T]) [][]*Node[T] {
	var levels [][]*Node[T]
	root.Each(func(n *Node[T]) {
		for len(levels) <= n.Depth {
			levels = append(levels, nil)
		}
		levels[n.Depth] = append(levels[n.Depth], n)
	})
	for _, l := range levels {
		sort.Slice(l, func(i, j int) bool { return l[i].X < l[j].X })
	}
	return levels
}

// checkNoOverlapByDepth asserts that adjacent nodes at the same depth are at
// least minGap apart. This is the tidy tree's central promise: nothing at a
// level ever collides, no matter how the subtrees above it are shaped.
func checkNoOverlapByDepth[T any](t *testing.T, root *Node[T], minGap float64) {
	t.Helper()
	for d, level := range byDepth(root) {
		for i := 1; i < len(level); i++ {
			gap := level[i].X - level[i-1].X
			if gap < minGap-eps {
				t.Errorf("depth %d: nodes %d and %d are %v apart, want at least %v",
					d, i-1, i, gap, minGap)
			}
		}
	}
}

// checkParentsCentred asserts that every parent sits exactly midway between its
// first and last child, which is the other half of the tidy tree's contract and
// the property that makes the drawing read as a tree rather than a graph.
func checkParentsCentred[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		if len(n.Children) == 0 {
			return
		}
		mid := (n.Children[0].X + n.Children[len(n.Children)-1].X) / 2
		if !closeTo(n.X, mid) {
			t.Errorf("node at depth %d has X %v, want %v (midpoint of its children)", n.Depth, n.X, mid)
		}
	})
}

// checkYByDepth asserts that Y depends only on depth and increases with it.
func checkYByDepth[T any](t *testing.T, root *Node[T], step float64) {
	t.Helper()
	for d, level := range byDepth(root) {
		want := float64(d) * step
		for _, n := range level {
			if !closeTo(n.Y, want) {
				t.Errorf("depth %d node has Y %v, want %v", d, n.Y, want)
			}
		}
	}
}

func TestTreeExactCoordinates(t *testing.T) {
	type want struct {
		name string
		x, y float64
	}
	tests := []struct {
		name   string
		layout func() *Tree[*tdata]
		data   *tdata
		want   []want
	}{
		{
			// A single node has nothing to separate from, so node-size mode
			// leaves it at the origin.
			name:   "single node, nodeSize",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().NodeSize(1, 1) },
			data:   leaf("only", 1),
			want:   []want{{"only", 0, 0}},
		},
		{
			// In size mode a lone node is centred: with left == right the
			// half-separation is 1, so the span is 2 and the node lands at 1/2.
			name:   "single node, size 1x1",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().Size(1, 1) },
			data:   leaf("only", 1),
			want:   []want{{"only", 0.5, 0}},
		},
		{
			// Two leaves 1 apart, parent centred between them at 0.
			name:   "two children, nodeSize 1x1",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().NodeSize(1, 1) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 0, 0}, {"a", -0.5, 1}, {"b", 0.5, 1}},
		},
		{
			// Three leaves at -1, 0, 1; the middle child lands exactly under
			// the parent.
			name:   "three children, nodeSize 1x1",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().NodeSize(1, 1) },
			data:   branch("r", leaf("a", 1), leaf("b", 1), leaf("c", 1)),
			want:   []want{{"r", 0, 0}, {"a", -1, 1}, {"b", 0, 1}, {"c", 1, 1}},
		},
		{
			// nodeSize scales X by width and spaces depths by height.
			name:   "two children, nodeSize 10x20",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().NodeSize(10, 20) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 0, 0}, {"a", -5, 20}, {"b", 5, 20}},
		},
		{
			// Size mode: leaves at -0.5 and 0.5 with a half-separation of 0.5
			// at each end gives a span of 2, mapped onto [0,1].
			name:   "two children, size 1x1",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().Size(1, 1) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 0.5, 0}, {"a", 0.25, 1}, {"b", 0.75, 1}},
		},
		{
			name:   "two children, size 100x50",
			layout: func() *Tree[*tdata] { return NewTree[*tdata]().Size(100, 50) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 50, 0}, {"a", 25, 50}, {"b", 75, 50}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.layout().Layout(Hierarchy(tt.data, kids))
			for _, w := range tt.want {
				n := find(t, root, w.name)
				if !closeTo(n.X, w.x) || !closeTo(n.Y, w.y) {
					t.Errorf("%s = (%v, %v), want (%v, %v)", w.name, n.X, n.Y, w.x, w.y)
				}
			}
		})
	}
}

func TestTreeSeparationIsHonoured(t *testing.T) {
	// A constant separation of 3 must put the two leaves 3 apart in node-size
	// mode with unit width.
	root := NewTree[*tdata]().
		NodeSize(1, 1).
		Separation(func(a, b *Node[*tdata]) float64 { return 3 }).
		Layout(Hierarchy(branch("r", leaf("a", 1), leaf("b", 1)), kids))
	if got := find(t, root, "b").X - find(t, root, "a").X; !closeTo(got, 3) {
		t.Errorf("leaf gap = %v, want 3", got)
	}

	// Cousins get twice the gap of siblings under the default separation, so
	// the two inner leaves of adjacent pairs sit 2 apart while siblings sit 1.
	root = NewTree[*tdata]().NodeSize(1, 1).Layout(Hierarchy(
		branch("r", branch("p", leaf("a", 1), leaf("b", 1)), branch("q", leaf("c", 1), leaf("d", 1))), kids))
	sib := find(t, root, "b").X - find(t, root, "a").X
	cousin := find(t, root, "c").X - find(t, root, "b").X
	if !closeTo(sib, 1) {
		t.Errorf("sibling gap = %v, want 1", sib)
	}
	if !closeTo(cousin, 2) {
		t.Errorf("cousin gap = %v, want 2", cousin)
	}
}

func TestTreeInvariants(t *testing.T) {
	tests := []struct {
		name   string
		layout *Tree[*tdata]
		minGap float64
		yStep  float64
	}{
		{"nodeSize 1x1", NewTree[*tdata]().NodeSize(1, 1), 1, 1},
		{"nodeSize 20x40", NewTree[*tdata]().NodeSize(20, 40), 20, 40},
		{"wide separation", NewTree[*tdata]().NodeSize(1, 1).
			Separation(func(a, b *Node[*tdata]) float64 { return 5 }), 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.layout.Layout(bigTree())
			checkNoOverlapByDepth(t, root, tt.minGap)
			checkParentsCentred(t, root)
			checkYByDepth(t, root, tt.yStep)
		})
	}

	t.Run("size mode fits the box", func(t *testing.T) {
		root := NewTree[*tdata]().Size(960, 500).Layout(bigTree())
		checkParentsCentred(t, root)
		minX, maxX := math.Inf(1), math.Inf(-1)
		root.Each(func(n *Node[*tdata]) {
			minX = math.Min(minX, n.X)
			maxX = math.Max(maxX, n.X)
			if n.Y < -eps || n.Y > 500+eps {
				t.Errorf("Y = %v outside [0, 500]", n.Y)
			}
		})
		if minX < -eps || maxX > 960+eps {
			t.Errorf("X range = [%v, %v], outside [0, 960]", minX, maxX)
		}
		// Scaling is uniform, so the ordering guarantee survives even though
		// the absolute gap is no longer 1.
		for d, level := range byDepth(root) {
			for i := 1; i < len(level); i++ {
				if level[i].X-level[i-1].X <= 0 {
					t.Errorf("depth %d: nodes are not strictly ordered in X", d)
				}
			}
		}
	})
}

func TestTreeIsomorphicSubtreesMatch(t *testing.T) {
	// The tidy algorithm draws identical subtrees identically wherever they
	// appear; the "t" and "u" subtrees of bigTree are isomorphic, so their
	// internal shapes must be translations of one another.
	root := NewTree[*tdata]().NodeSize(1, 1).Layout(bigTree())
	tn, un := find(t, root, "t"), find(t, root, "u")
	offset := un.X - tn.X
	pairs := [][2]string{{"tx", "ux"}, {"ty", "uy"}, {"tx1", "ux1"}, {"tx2", "ux2"}}
	for _, p := range pairs {
		a, b := find(t, root, p[0]), find(t, root, p[1])
		if !closeTo(b.X-a.X, offset) {
			t.Errorf("%s/%s offset = %v, want %v", p[0], p[1], b.X-a.X, offset)
		}
	}
}

func TestClusterExactCoordinates(t *testing.T) {
	type want struct {
		name string
		x, y float64
	}
	tests := []struct {
		name   string
		layout func() *Cluster[*tdata]
		data   *tdata
		want   []want
	}{
		{
			name:   "single node, size 1x1",
			layout: func() *Cluster[*tdata] { return NewCluster[*tdata]().Size(1, 1) },
			data:   leaf("only", 1),
			want:   []want{{"only", 0.5, 0}},
		},
		{
			// Leaves at 0 and 1, half-separation padding at each end gives a
			// span of 2 mapped onto [0,1]; the root is the mean, and lands at
			// Y 0 while both leaves land at Y 1.
			name:   "two children, size 1x1",
			layout: func() *Cluster[*tdata] { return NewCluster[*tdata]().Size(1, 1) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 0.5, 0}, {"a", 0.25, 1}, {"b", 0.75, 1}},
		},
		{
			name:   "two children, nodeSize 1x1",
			layout: func() *Cluster[*tdata] { return NewCluster[*tdata]().NodeSize(1, 1) },
			data:   branch("r", leaf("a", 1), leaf("b", 1)),
			want:   []want{{"r", 0, 0}, {"a", -0.5, 1}, {"b", 0.5, 1}},
		},
		{
			// The defining property of a dendrogram: the shallow leaf "b" is
			// drawn on the same line as the deep leaf "c2", not one step from
			// the root.
			name:   "leaves align regardless of depth",
			layout: func() *Cluster[*tdata] { return NewCluster[*tdata]().Size(1, 60) },
			data:   branch("r", leaf("b", 1), branch("c", branch("c1", leaf("c2", 1)))),
			want:   []want{{"r", 0.5, 0}, {"b", 0.25, 60}, {"c2", 0.75, 60}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.layout().Layout(Hierarchy(tt.data, kids))
			for _, w := range tt.want {
				n := find(t, root, w.name)
				if !closeTo(n.X, w.x) || !closeTo(n.Y, w.y) {
					t.Errorf("%s = (%v, %v), want (%v, %v)", w.name, n.X, n.Y, w.x, w.y)
				}
			}
		})
	}
}

func TestClusterInvariants(t *testing.T) {
	root := NewCluster[*tdata]().NodeSize(1, 1).Layout(bigTree())

	t.Run("all leaves share a line", func(t *testing.T) {
		leaves := root.Leaves()
		for _, l := range leaves {
			if !closeTo(l.Y, leaves[0].Y) {
				t.Errorf("leaf Y = %v, want %v (all leaves on one line)", l.Y, leaves[0].Y)
			}
		}
	})

	t.Run("adjacent leaves are separated", func(t *testing.T) {
		leaves := root.Leaves()
		sort.Slice(leaves, func(i, j int) bool { return leaves[i].X < leaves[j].X })
		for i := 1; i < len(leaves); i++ {
			if gap := leaves[i].X - leaves[i-1].X; gap < 1-eps {
				t.Errorf("leaf gap = %v, want at least 1", gap)
			}
		}
	})

	t.Run("parents are the mean of their children", func(t *testing.T) {
		root.Each(func(n *Node[*tdata]) {
			if len(n.Children) == 0 {
				return
			}
			sum := 0.0
			for _, c := range n.Children {
				sum += c.X
			}
			if want := sum / float64(len(n.Children)); !closeTo(n.X, want) {
				t.Errorf("node at depth %d has X %v, want %v", n.Depth, n.X, want)
			}
		})
	})

	t.Run("same depth is strictly ordered", func(t *testing.T) {
		for d, level := range byDepth(root) {
			for i := 1; i < len(level); i++ {
				if level[i].X-level[i-1].X <= 0 {
					t.Errorf("depth %d: nodes share an X or are out of order", d)
				}
			}
		}
	})

	t.Run("size mode fits the box", func(t *testing.T) {
		r := NewCluster[*tdata]().Size(400, 300).Layout(bigTree())
		r.Each(func(n *Node[*tdata]) {
			if n.X < -eps || n.X > 400+eps || n.Y < -eps || n.Y > 300+eps {
				t.Errorf("node at (%v, %v) is outside the 400x300 box", n.X, n.Y)
			}
		})
	})
}
