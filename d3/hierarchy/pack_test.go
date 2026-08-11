package hierarchy

import (
	"cmp"
	"math"
	"testing"
)

// checkNoCircleOverlap asserts that no two siblings overlap. As with the
// rectangle layouts, siblings plus containment gives global disjointness.
//
// The tolerance is relative to the circle sizes rather than absolute, because
// packing is a long chain of square roots and tangencies; the meaningful claim
// is "no visible overlap", not "no overlap to the last bit".
func checkNoCircleOverlap[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		cs := n.Children
		for i := 0; i < len(cs); i++ {
			for j := i + 1; j < len(cs); j++ {
				a, b := cs[i], cs[j]
				d := math.Hypot(b.X-a.X, b.Y-a.Y)
				tol := 1e-6 * math.Max(1, a.R+b.R)
				if d < a.R+b.R-tol {
					t.Errorf("depth %d: circles r=%v and r=%v are %v apart and overlap by %v",
						a.Depth, a.R, b.R, d, a.R+b.R-d)
				}
			}
		}
	})
}

// checkCirclesNested asserts that every child circle lies wholly inside its
// parent's — the enclosure property the whole layout is named for.
func checkCirclesNested[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		p := n.Parent
		if p == nil {
			return
		}
		d := math.Hypot(n.X-p.X, n.Y-p.Y)
		tol := 1e-6 * math.Max(1, p.R)
		if d+n.R > p.R+tol {
			t.Errorf("depth %d: child (r=%v, %v from centre) sticks out of parent r=%v by %v",
				n.Depth, n.R, d, p.R, d+n.R-p.R)
		}
	})
}

func checkCirclesFinite[T any](t *testing.T, root *Node[T]) {
	t.Helper()
	root.Each(func(n *Node[T]) {
		for _, v := range []float64{n.X, n.Y, n.R} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("depth %d: non-finite coordinate %v", n.Depth, v)
			}
		}
		if n.R < 0 {
			t.Errorf("depth %d: negative radius %v", n.Depth, n.R)
		}
	})
}

func TestPackExactCoordinates(t *testing.T) {
	t.Run("single leaf fills the box", func(t *testing.T) {
		// Default sizing gives r = sqrt(4) = 2, which is then scaled by
		// min(100,100) / (2*2) = 25 to give a radius of 50 centred at (50,50).
		root := NewPack[*tdata]().Size(100, 100).
			Layout(Hierarchy(leaf("only", 4), kids).Sum(func(d *tdata) float64 { return d.weight }))
		if !closeTo(root.X, 50) || !closeTo(root.Y, 50) || !closeTo(root.R, 50) {
			t.Errorf("got (%v, %v) r=%v, want (50, 50) r=50", root.X, root.Y, root.R)
		}
	})

	t.Run("two equal children sit side by side", func(t *testing.T) {
		// Two unit circles pack to a bounding radius of 2 centred between them,
		// so the scale is 100/(2*2) = 25: two circles of radius 25 at
		// (25, 50) and (75, 50), inside a parent of radius 50 at (50, 50).
		root := NewPack[*tdata]().Size(100, 100).
			Layout(Hierarchy(branch("r", leaf("a", 1), leaf("b", 1)), kids).
				Sum(func(d *tdata) float64 { return d.weight }))
		want := []struct {
			name    string
			x, y, r float64
		}{
			{"r", 50, 50, 50},
			{"a", 25, 50, 25},
			{"b", 75, 50, 25},
		}
		for _, w := range want {
			n := find(t, root, w.name)
			if !closeTo(n.X, w.x) || !closeTo(n.Y, w.y) || !closeTo(n.R, w.r) {
				t.Errorf("%s = (%v, %v) r=%v, want (%v, %v) r=%v", w.name, n.X, n.Y, n.R, w.x, w.y, w.r)
			}
		}
	})

	t.Run("non-square size centres the packing", func(t *testing.T) {
		// Scaling is uniform and driven by the smaller dimension, so a lone
		// node in a 200x100 box gets radius 50 and is centred at (100, 50).
		root := NewPack[*tdata]().Size(200, 100).
			Layout(Hierarchy(leaf("only", 1), kids).Sum(func(d *tdata) float64 { return d.weight }))
		if !closeTo(root.X, 100) || !closeTo(root.Y, 50) || !closeTo(root.R, 50) {
			t.Errorf("got (%v, %v) r=%v, want (100, 50) r=50", root.X, root.Y, root.R)
		}
	})

	t.Run("explicit radius is used verbatim", func(t *testing.T) {
		// With Radius set there is no scale-to-fit; the packing is centred at
		// the middle of the size and leaves keep exactly the radius given.
		root := NewPack[*tdata]().Size(100, 100).
			Radius(func(n *Node[*tdata]) float64 { return 10 }).
			Layout(Hierarchy(branch("r", leaf("a", 1), leaf("b", 1)), kids).
				Sum(func(d *tdata) float64 { return d.weight }))
		a, b := find(t, root, "a"), find(t, root, "b")
		if !closeTo(a.R, 10) || !closeTo(b.R, 10) {
			t.Errorf("leaf radii = %v, %v, want 10", a.R, b.R)
		}
		if !closeTo(root.R, 20) {
			t.Errorf("root r = %v, want 20", root.R)
		}
		if !closeTo(a.X, 40) || !closeTo(b.X, 60) || !closeTo(a.Y, 50) || !closeTo(b.Y, 50) {
			t.Errorf("leaves at (%v,%v) and (%v,%v), want (40,50) and (60,50)", a.X, a.Y, b.X, b.Y)
		}
	})
}

func TestPackInvariants(t *testing.T) {
	build := func() *Node[*tdata] {
		return rectTree().Sort(func(a, b *Node[*tdata]) int { return cmp.Compare(b.Value, a.Value) })
	}

	tests := []struct {
		name string
		pack *Pack[*tdata]
	}{
		{"default", NewPack[*tdata]().Size(900, 900)},
		{"padded", NewPack[*tdata]().Size(900, 900).Padding(4)},
		{"heavily padded", NewPack[*tdata]().Size(900, 900).Padding(20)},
		{"padding by depth", NewPack[*tdata]().Size(900, 900).
			PaddingFunc(func(n *Node[*tdata]) float64 { return float64(12 - 4*n.Depth) })},
		{"explicit radius", NewPack[*tdata]().Size(900, 900).
			Radius(func(n *Node[*tdata]) float64 { return 5 + n.Value })},
		{"non-square", NewPack[*tdata]().Size(1200, 400)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.pack.Layout(build())
			checkCirclesFinite(t, root)
			checkNoCircleOverlap(t, root)
			checkCirclesNested(t, root)
		})
	}
}

func TestPackManySiblings(t *testing.T) {
	// The front-chain algorithm only becomes interesting past three circles,
	// and its retry path (a collision found while walking the chain) needs a
	// decent number of differently sized siblings to be exercised. A flat fan of
	// 60 leaves with a wide spread of values does that.
	var cs []*tdata
	for i := 1; i <= 60; i++ {
		cs = append(cs, leaf("l", float64((i*37)%23+1)))
	}
	root := Hierarchy(branch("r", cs...), kids).
		Sum(func(d *tdata) float64 { return d.weight }).
		Sort(func(a, b *Node[*tdata]) int { return cmp.Compare(b.Value, a.Value) })
	root = NewPack[*tdata]().Size(800, 800).Padding(2).Layout(root)

	checkCirclesFinite(t, root)
	checkNoCircleOverlap(t, root)
	checkCirclesNested(t, root)

	// Every leaf must be inside the root circle, which is the enclosing-circle
	// step doing its job.
	for _, l := range root.Leaves() {
		d := math.Hypot(l.X-root.X, l.Y-root.Y)
		if d+l.R > root.R+1e-6*root.R {
			t.Errorf("leaf sticks out of the root circle by %v", d+l.R-root.R)
		}
	}
	// And the root must fill the box: min(dx,dy) / 2.
	if !closeTo(root.R, 400) {
		t.Errorf("root r = %v, want 400", root.R)
	}
}

func TestPackIsDeterministic(t *testing.T) {
	// The enclosing-circle step shuffles its input, so determinism is a
	// deliberate property of the fixed-seed generator rather than an accident.
	build := func() *Node[*tdata] {
		return rectTree().Sort(func(a, b *Node[*tdata]) int { return cmp.Compare(b.Value, a.Value) })
	}
	a := NewPack[*tdata]().Size(500, 500).Padding(3).Layout(build())
	b := NewPack[*tdata]().Size(500, 500).Padding(3).Layout(build())
	an, bn := a.Descendants(), b.Descendants()
	for i := range an {
		if an[i].X != bn[i].X || an[i].Y != bn[i].Y || an[i].R != bn[i].R {
			t.Fatalf("node %d differs between runs: (%v,%v,%v) vs (%v,%v,%v)",
				i, an[i].X, an[i].Y, an[i].R, bn[i].X, bn[i].Y, bn[i].R)
		}
	}
}

func TestPackZeroValues(t *testing.T) {
	root := Hierarchy(branch("r", leaf("a", 0), leaf("b", 0)), kids).
		Sum(func(d *tdata) float64 { return d.weight })
	got := NewPack[*tdata]().Size(100, 100).Layout(root)
	checkCirclesFinite(t, got)
}

func TestEnclose(t *testing.T) {
	tests := []struct {
		name    string
		in      []Circle
		x, y, r float64
	}{
		{"empty", nil, 0, 0, 0},
		{"one circle is its own enclosure", []Circle{{1, 2, 3}}, 1, 2, 3},
		{
			// Two unit circles centred at ±1 on the x axis: the enclosure is
			// centred at the origin with radius 2.
			name: "two tangent circles",
			in:   []Circle{{-1, 0, 1}, {1, 0, 1}},
			x:    0, y: 0, r: 2,
		},
		{
			// A circle entirely inside another contributes nothing.
			name: "nested circles",
			in:   []Circle{{0, 0, 10}, {1, 1, 2}},
			x:    0, y: 0, r: 10,
		},
		{
			// Three points (radius 0) at the corners of a right triangle with
			// legs 6 and 8: the enclosing circle is the circumcircle of the
			// hypotenuse, centred at its midpoint with radius 5.
			name: "right triangle of points",
			in:   []Circle{{0, 0, 0}, {8, 0, 0}, {0, 6, 0}},
			x:    4, y: 3, r: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Enclose(tt.in)
			if math.Abs(got.X-tt.x) > 1e-6 || math.Abs(got.Y-tt.y) > 1e-6 || math.Abs(got.R-tt.r) > 1e-6 {
				t.Errorf("got (%v, %v) r=%v, want (%v, %v) r=%v", got.X, got.Y, got.R, tt.x, tt.y, tt.r)
			}
		})
	}
}

func TestEncloseContainsEverything(t *testing.T) {
	// The property that actually matters, over a spread of pseudo-random but
	// fixed inputs: the result contains every input circle, and is not
	// gratuitously larger than the obvious lower bound (the largest input).
	var circles []Circle
	s := uint32(12345)
	next := func() float64 {
		s = s*1664525 + 1013904223
		return float64(s) / 4294967296
	}
	for i := 0; i < 200; i++ {
		circles = append(circles, Circle{X: next()*200 - 100, Y: next()*200 - 100, R: next()*20 + 1})
	}
	e := Enclose(circles)
	maxR := 0.0
	for _, c := range circles {
		d := math.Hypot(c.X-e.X, c.Y-e.Y)
		if d+c.R > e.R+1e-6*e.R {
			t.Errorf("circle at (%v,%v) r=%v sticks out by %v", c.X, c.Y, c.R, d+c.R-e.R)
		}
		maxR = math.Max(maxR, c.R)
	}
	if e.R < maxR {
		t.Errorf("enclosing radius %v is smaller than the largest input radius %v", e.R, maxR)
	}
}
