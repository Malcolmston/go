package quadtree

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/random"
)

// pt is the test datum. It carries an id so that two points at the same
// coordinates are still distinct values, which is what the coincident-point
// paths need.
type pt struct {
	X, Y float64
	ID   int
}

func newTree() *Quadtree[pt] {
	return New(
		func(p pt) float64 { return p.X },
		func(p pt) float64 { return p.Y },
	)
}

// randomPoints returns n points on a fixed seeded stream, so a failure is
// reproducible.
func randomPoints(seed float64, n int, scale float64) []pt {
	r := random.Source(seed)
	pts := make([]pt, n)
	for i := range pts {
		pts[i] = pt{X: (r.Float64() - 0.5) * scale, Y: (r.Float64() - 0.5) * scale, ID: i}
	}
	return pts
}

func bruteNearest(pts []pt, x, y float64) (pt, float64) {
	best := math.Inf(1)
	var found pt
	for _, p := range pts {
		dx, dy := x-p.X, y-p.Y
		if d2 := dx*dx + dy*dy; d2 < best {
			best, found = d2, p
		}
	}
	return found, best
}

func TestEmptyTree(t *testing.T) {
	q := newTree()
	if q.Root() != nil {
		t.Error("empty tree has a root")
	}
	if got := q.Size(); got != 0 {
		t.Errorf("Size() = %d, want 0", got)
	}
	if got := q.Data(); len(got) != 0 {
		t.Errorf("Data() = %v, want empty", got)
	}
	if _, _, _, _, ok := q.Extent(); ok {
		t.Error("empty tree reports an extent")
	}
	if _, ok := q.Find(0, 0); ok {
		t.Error("Find on an empty tree found something")
	}
}

func TestAddAllPointsArePresent(t *testing.T) {
	pts := randomPoints(1, 500, 1000)
	q := newTree().AddAll(pts)

	if got := q.Size(); got != len(pts) {
		t.Fatalf("Size() = %d, want %d", got, len(pts))
	}

	seen := make(map[pt]bool, len(pts))
	for _, d := range q.Data() {
		seen[d] = true
	}
	for _, p := range pts {
		if !seen[p] {
			t.Fatalf("point %+v missing from Data()", p)
		}
	}

	// Every point must be findable at its own coordinates, and the nearest
	// thing to a point is that point (distance zero).
	for _, p := range pts {
		got, ok := q.Find(p.X, p.Y)
		if !ok {
			t.Fatalf("Find(%v, %v) found nothing", p.X, p.Y)
		}
		if got.X != p.X || got.Y != p.Y {
			t.Fatalf("Find(%v, %v) = %+v, want a point at those exact coordinates", p.X, p.Y, got)
		}
	}
}

func TestAddOneAtATimeMatchesAddAll(t *testing.T) {
	pts := randomPoints(2, 200, 500)

	batch := newTree().AddAll(pts)
	incremental := newTree()
	for _, p := range pts {
		incremental.Add(p)
	}

	if batch.Size() != incremental.Size() {
		t.Fatalf("sizes differ: %d vs %d", batch.Size(), incremental.Size())
	}
	// The extents may differ — AddAll covers the whole bounding box up front
	// while Add grows one point at a time — but both must contain every point.
	assertCoversAll(t, batch, pts)
	assertCoversAll(t, incremental, pts)
}

func assertCoversAll(t *testing.T, q *Quadtree[pt], pts []pt) {
	t.Helper()
	x0, y0, x1, y1, ok := q.Extent()
	if !ok {
		t.Fatal("tree with data has no extent")
	}
	for _, p := range pts {
		if p.X < x0 || p.X > x1 || p.Y < y0 || p.Y > y1 {
			t.Fatalf("extent [%v %v %v %v] does not cover %+v", x0, y0, x1, y1, p)
		}
	}
}

// TestFindMatchesBruteForce is the load-bearing correctness test for Find: over
// many random trees and many random query points, the answer must be exactly as
// near as an exhaustive scan's answer. Comparing distances rather than identity
// lets genuine ties pass.
func TestFindMatchesBruteForce(t *testing.T) {
	for trial := 0; trial < 20; trial++ {
		n := 1 + trial*13
		pts := randomPoints(float64(trial+1), n, 400)
		q := newTree().AddAll(pts)

		r := random.Source(float64(1000 + trial))
		for k := 0; k < 200; k++ {
			// Query well outside the data too, not just inside it.
			x := (r.Float64() - 0.5) * 800
			y := (r.Float64() - 0.5) * 800

			got, ok := q.Find(x, y)
			want, wantD2 := bruteNearest(pts, x, y)
			if !ok {
				t.Fatalf("trial %d: Find(%v,%v) found nothing, want %+v", trial, x, y, want)
			}
			dx, dy := x-got.X, y-got.Y
			gotD2 := dx*dx + dy*dy
			if math.Abs(gotD2-wantD2) > 1e-9*(1+wantD2) {
				t.Fatalf("trial %d: Find(%v,%v) = %+v at d²=%v, brute force found %+v at d²=%v",
					trial, x, y, got, gotD2, want, wantD2)
			}
		}
	}
}

func TestFindWithinRespectsRadius(t *testing.T) {
	pts := randomPoints(7, 300, 400)
	q := newTree().AddAll(pts)
	r := random.Source(99)

	for k := 0; k < 300; k++ {
		x := (r.Float64() - 0.5) * 600
		y := (r.Float64() - 0.5) * 600
		radius := r.Float64() * 60

		got, ok := q.FindWithin(x, y, radius)
		want, wantD2 := bruteNearest(pts, x, y)
		inRange := wantD2 < radius*radius

		if ok != inRange {
			t.Fatalf("FindWithin(%v,%v,%v) ok = %v, want %v (nearest %+v at d=%v)",
				x, y, radius, ok, inRange, want, math.Sqrt(wantD2))
		}
		if ok {
			dx, dy := x-got.X, y-got.Y
			if gotD2 := dx*dx + dy*dy; math.Abs(gotD2-wantD2) > 1e-9*(1+wantD2) {
				t.Fatalf("FindWithin returned d²=%v, want %v", gotD2, wantD2)
			}
		}
	}
}

func TestCoincidentPoints(t *testing.T) {
	pts := []pt{
		{X: 5, Y: 5, ID: 0},
		{X: 5, Y: 5, ID: 1},
		{X: 5, Y: 5, ID: 2},
		{X: 9, Y: 1, ID: 3},
	}
	q := newTree().AddAll(pts)

	if got := q.Size(); got != 4 {
		t.Fatalf("Size() = %d, want 4", got)
	}
	if got := len(q.Data()); got != 4 {
		t.Fatalf("len(Data()) = %d, want 4", got)
	}

	// Removing two of the three coincident points must leave the third
	// findable, and removing all three must leave only the fourth.
	q.Remove(pts[1]).Remove(pts[0])
	if got := q.Size(); got != 2 {
		t.Fatalf("after removing 2 coincident points Size() = %d, want 2", got)
	}
	got, ok := q.Find(5, 5)
	if !ok || got != pts[2] {
		t.Fatalf("Find(5,5) = %+v (%v), want %+v", got, ok, pts[2])
	}
	q.Remove(pts[2])
	if _, ok := q.FindWithin(5, 5, 1); ok {
		t.Error("a removed coincident point is still findable")
	}
}

func TestRemoveMakesPointsUnfindable(t *testing.T) {
	pts := randomPoints(3, 250, 300)
	q := newTree().AddAll(pts)

	// Remove every other point, then check that exactly the survivors remain
	// and that Find never returns a removed one.
	removed := make(map[pt]bool)
	var kept []pt
	for i, p := range pts {
		if i%2 == 0 {
			q.Remove(p)
			removed[p] = true
		} else {
			kept = append(kept, p)
		}
	}

	if got := q.Size(); got != len(kept) {
		t.Fatalf("Size() = %d, want %d", got, len(kept))
	}
	for _, d := range q.Data() {
		if removed[d] {
			t.Fatalf("removed point %+v is still in the tree", d)
		}
	}

	// The remaining tree must still answer nearest-neighbour queries exactly,
	// which is the real test that Remove left the structure consistent rather
	// than merely unreachable.
	r := random.Source(77)
	for k := 0; k < 300; k++ {
		x := (r.Float64() - 0.5) * 600
		y := (r.Float64() - 0.5) * 600
		got, ok := q.Find(x, y)
		if !ok {
			t.Fatal("Find found nothing in a non-empty tree")
		}
		if removed[got] {
			t.Fatalf("Find returned removed point %+v", got)
		}
		_, wantD2 := bruteNearest(kept, x, y)
		dx, dy := x-got.X, y-got.Y
		if gotD2 := dx*dx + dy*dy; math.Abs(gotD2-wantD2) > 1e-9*(1+wantD2) {
			t.Fatalf("after Remove, Find(%v,%v) d²=%v, want %v", x, y, gotD2, wantD2)
		}
	}

	// Removing everything empties the tree.
	q.RemoveAll(kept)
	if got := q.Size(); got != 0 {
		t.Fatalf("after RemoveAll, Size() = %d, want 0", got)
	}
	if _, ok := q.Find(0, 0); ok {
		t.Error("emptied tree still finds something")
	}
}

func TestRemoveUnknownPointIsNoOp(t *testing.T) {
	pts := randomPoints(4, 50, 100)
	q := newTree().AddAll(pts)
	q.Remove(pt{X: 1e9, Y: 1e9, ID: -1})
	q.Remove(pt{X: pts[0].X, Y: pts[0].Y, ID: -2}) // same place, different datum
	if got := q.Size(); got != len(pts) {
		t.Fatalf("Size() = %d, want %d", got, len(pts))
	}
}

func TestVisitPruningIsCorrect(t *testing.T) {
	pts := randomPoints(5, 400, 500)
	q := newTree().AddAll(pts)

	// Pruning at the root must visit exactly one node.
	visited := 0
	q.Visit(func(*Node[pt], float64, float64, float64, float64) bool {
		visited++
		return true
	})
	if visited != 1 {
		t.Fatalf("pruning at the root visited %d nodes, want 1", visited)
	}

	// A window query that prunes on the node bounds must find exactly the
	// points a linear scan finds. This is the property that makes pruning
	// worth doing: it must discard only what it provably may.
	rx0, ry0, rx1, ry1 := -100.0, -50.0, 60.0, 180.0
	var inWindow []pt
	q.Visit(func(n *Node[pt], x0, y0, x1, y1 float64) bool {
		if x0 > rx1 || x1 < rx0 || y0 > ry1 || y1 < ry0 {
			return true // no overlap: prune
		}
		if n.IsLeaf() {
			for l := n; l != nil; l = l.Next {
				p := l.Data
				if p.X >= rx0 && p.X <= rx1 && p.Y >= ry0 && p.Y <= ry1 {
					inWindow = append(inWindow, p)
				}
			}
		}
		return false
	})

	want := make(map[pt]bool)
	for _, p := range pts {
		if p.X >= rx0 && p.X <= rx1 && p.Y >= ry0 && p.Y <= ry1 {
			want[p] = true
		}
	}
	if len(inWindow) != len(want) {
		t.Fatalf("pruned window query found %d points, linear scan found %d", len(inWindow), len(want))
	}
	for _, p := range inWindow {
		if !want[p] {
			t.Fatalf("pruned window query returned out-of-window point %+v", p)
		}
	}
}

func TestVisitCoversEveryNodeAndPoint(t *testing.T) {
	pts := randomPoints(6, 300, 400)
	q := newTree().AddAll(pts)

	count := 0
	q.Visit(func(n *Node[pt], x0, y0, x1, y1 float64) bool {
		if x1 <= x0 || y1 <= y0 {
			t.Fatalf("degenerate node square [%v %v %v %v]", x0, y0, x1, y1)
		}
		if n.IsLeaf() {
			for l := n; l != nil; l = l.Next {
				count++
				p := l.Data
				// Every leaf's point must lie inside the square it was
				// reported with.
				if p.X < x0 || p.X > x1 || p.Y < y0 || p.Y > y1 {
					t.Fatalf("point %+v is outside its own node square [%v %v %v %v]", p, x0, y0, x1, y1)
				}
			}
		}
		return false
	})
	if count != len(pts) {
		t.Fatalf("Visit reached %d points, want %d", count, len(pts))
	}
}

// TestVisitAfterIsPostOrder checks the guarantee the Barnes-Hut accumulation
// depends on: when a node is visited, every one of its children has been.
func TestVisitAfterIsPostOrder(t *testing.T) {
	pts := randomPoints(8, 300, 400)
	q := newTree().AddAll(pts)

	done := make(map[*Node[pt]]bool)
	internal, leaves := 0, 0
	q.VisitAfter(func(n *Node[pt], _, _, _, _ float64) {
		if n.IsLeaf() {
			leaves++
		} else {
			internal++
			for i, c := range n.Children {
				if c != nil && !done[c] {
					t.Fatalf("child %d visited after its parent", i)
				}
			}
		}
		done[n] = true
	})
	if leaves == 0 || internal == 0 {
		t.Fatalf("expected a mixed tree, got %d leaves and %d internal nodes", leaves, internal)
	}
}

// TestVisitAfterAccumulates exercises the scratch slots the way d3/force does:
// summing a count bottom-up must reproduce the tree's total size at the root.
func TestVisitAfterAccumulates(t *testing.T) {
	pts := randomPoints(9, 257, 400)
	q := newTree().AddAll(pts)

	q.VisitAfter(func(n *Node[pt], _, _, _, _ float64) {
		if n.IsLeaf() {
			n.Value = 0
			for l := n; l != nil; l = l.Next {
				n.Value++
			}
			return
		}
		n.Value = 0
		for _, c := range n.Children {
			if c != nil {
				n.Value += c.Value
			}
		}
	})
	if got := q.Root().Value; got != float64(len(pts)) {
		t.Fatalf("accumulated root value = %v, want %d", got, len(pts))
	}
}

func TestCoverGrowsWithoutLosingPoints(t *testing.T) {
	q := newTree().Add(pt{X: 0, Y: 0, ID: 0})
	q.Cover(1000, 1000).Cover(-1000, -1000)

	x0, y0, x1, y1, ok := q.Extent()
	if !ok {
		t.Fatal("no extent after Cover")
	}
	if x0 > -1000 || y0 > -1000 || x1 < 1000 || y1 < 1000 {
		t.Fatalf("extent [%v %v %v %v] does not cover the requested points", x0, y0, x1, y1)
	}
	// The extent is always square.
	if math.Abs((x1-x0)-(y1-y0)) > 1e-9 {
		t.Fatalf("extent is not square: %v by %v", x1-x0, y1-y0)
	}
	if got := q.Size(); got != 1 {
		t.Fatalf("Size() = %d after growing, want 1", got)
	}
	got, ok := q.Find(0, 0)
	if !ok || got.ID != 0 {
		t.Fatalf("original point lost after growing: %+v %v", got, ok)
	}
}

func TestSetExtent(t *testing.T) {
	q := newTree().SetExtent(0, 0, 100, 100)
	x0, y0, x1, y1, ok := q.Extent()
	if !ok || x0 > 0 || y0 > 0 || x1 < 100 || y1 < 100 {
		t.Fatalf("SetExtent gave [%v %v %v %v] (%v)", x0, y0, x1, y1, ok)
	}
}

func TestNaNIsIgnored(t *testing.T) {
	pts := []pt{
		{X: 1, Y: 2, ID: 0},
		{X: math.NaN(), Y: 2, ID: 1},
		{X: 3, Y: math.NaN(), ID: 2},
		{X: 4, Y: 5, ID: 3},
	}
	q := newTree().AddAll(pts)
	if got := q.Size(); got != 2 {
		t.Fatalf("Size() = %d, want 2 (NaN points ignored)", got)
	}
	q.Add(pt{X: math.NaN(), Y: math.NaN(), ID: 4})
	if got := q.Size(); got != 2 {
		t.Fatalf("Size() = %d after adding a NaN point, want 2", got)
	}
	q.Remove(pts[1]) // no-op, must not panic
	if got := q.Size(); got != 2 {
		t.Fatalf("Size() = %d after removing a NaN point, want 2", got)
	}

	// A tree of nothing but NaN never gets covered at all.
	only := newTree().AddAll([]pt{{X: math.NaN(), Y: math.NaN(), ID: 0}})
	if _, _, _, _, ok := only.Extent(); ok {
		t.Error("a tree of only NaN points has an extent")
	}
}

func TestCopyIsIndependent(t *testing.T) {
	pts := randomPoints(10, 200, 300)
	orig := newTree().AddAll(pts)
	dup := orig.Copy()

	if dup.Size() != orig.Size() {
		t.Fatalf("copy has %d points, original has %d", dup.Size(), orig.Size())
	}

	// Mutating the copy must not disturb the original, and vice versa.
	dup.RemoveAll(pts[:100])
	if got := orig.Size(); got != len(pts) {
		t.Fatalf("original changed to %d points after mutating the copy", got)
	}
	if got := dup.Size(); got != len(pts)-100 {
		t.Fatalf("copy has %d points after removing 100, want %d", got, len(pts)-100)
	}

	orig.Add(pt{X: 12345, Y: 12345, ID: 9999})
	if got := dup.Size(); got != len(pts)-100 {
		t.Fatalf("copy changed to %d points after mutating the original", got)
	}

	// A copy of a coincident chain must be a copy, not a shared chain.
	c := newTree().AddAll([]pt{{X: 1, Y: 1, ID: 0}, {X: 1, Y: 1, ID: 1}, {X: 1, Y: 1, ID: 2}})
	cc := c.Copy()
	c.Remove(pt{X: 1, Y: 1, ID: 1})
	if got := cc.Size(); got != 3 {
		t.Fatalf("copied coincident chain has %d points after mutating the original, want 3", got)
	}
}

func TestAccessorsAndRoot(t *testing.T) {
	q := newTree()
	if q.X() == nil || q.Y() == nil {
		t.Fatal("accessors are nil")
	}
	q.Add(pt{X: 3, Y: 4, ID: 0})
	root := q.Root()
	if root == nil || !root.IsLeaf() {
		t.Fatal("a one-point tree's root should be a leaf")
	}
	if root.Data.ID != 0 {
		t.Fatalf("root data = %+v", root.Data)
	}

	// Swapping the accessors changes how new points are read, not old ones.
	q.SetX(func(p pt) float64 { return p.Y }).SetY(func(p pt) float64 { return p.X })
	if q.X()(pt{X: 1, Y: 2}) != 2 {
		t.Error("SetX did not take effect")
	}
	if q.Y()(pt{X: 1, Y: 2}) != 1 {
		t.Error("SetY did not take effect")
	}
}

func TestPointerData(t *testing.T) {
	// The identity semantics the package documents: two structurally identical
	// but distinct *pt values are different data.
	a := &pt{X: 1, Y: 1}
	b := &pt{X: 1, Y: 1}
	q := New(
		func(p *pt) float64 { return p.X },
		func(p *pt) float64 { return p.Y },
	).AddAll([]*pt{a, b})

	if got := q.Size(); got != 2 {
		t.Fatalf("Size() = %d, want 2", got)
	}
	q.Remove(a)
	if got := q.Size(); got != 1 {
		t.Fatalf("Size() = %d after removing one, want 1", got)
	}
	got, ok := q.Find(1, 1)
	if !ok || got != b {
		t.Fatalf("Find = %p (%v), want %p", got, ok, b)
	}
}

func BenchmarkFind(b *testing.B) {
	pts := randomPoints(1, 10000, 1000)
	q := newTree().AddAll(pts)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Find(float64(i%1000)-500, float64(i%997)-500)
	}
}
