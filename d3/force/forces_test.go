package force

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/random"
)

// TestManyBodyRepelsAndAttracts checks the sign of the force rather than its
// magnitude: a negative strength must spread nodes out, a positive one must draw
// them together.
func TestManyBodyRepelsAndAttracts(t *testing.T) {
	spread := func(s *Simulation[datum]) float64 {
		var cx, cy float64
		for _, n := range s.Nodes() {
			cx += n.X
			cy += n.Y
		}
		cx /= float64(len(s.Nodes()))
		cy /= float64(len(s.Nodes()))
		var sum float64
		for _, n := range s.Nodes() {
			sum += math.Hypot(n.X-cx, n.Y-cy)
		}
		return sum / float64(len(s.Nodes()))
	}

	repel := New(data(60)).SetForce("charge", NewManyBody[datum]().SetStrength(-60))
	before := spread(repel)
	repel.RunUntilStable()
	if after := spread(repel); after <= before {
		t.Errorf("repulsion did not spread the nodes: %v then %v", before, after)
	}
	assertFinite(t, repel, "many-body repulsion")

	// Attraction is checked on a single pair over a single tick. A settled
	// many-body attraction is not a meaningful measurement: an inverse-distance
	// pull with no opposing force collapses the cloud, overshoots through the
	// centre and scatters it again, so the end state says nothing about the
	// sign of the force. One tick says it unambiguously.
	a := &Node[datum]{Data: datum{"a"}, X: -100, Y: 0}
	b := &Node[datum]{Data: datum{"b"}, X: 100, Y: 0}
	NewWithNodes([]*Node[datum]{a, b}).
		SetForce("charge", NewManyBody[datum]().SetStrength(+60)).Tick()
	if d := dist(a, b); d >= 200 {
		t.Errorf("positive strength did not attract: separation %v, was 200", d)
	}

	c := &Node[datum]{Data: datum{"c"}, X: -100, Y: 0}
	d := &Node[datum]{Data: datum{"d"}, X: 100, Y: 0}
	NewWithNodes([]*Node[datum]{c, d}).
		SetForce("charge", NewManyBody[datum]().SetStrength(-60)).Tick()
	if sep := dist(c, d); sep <= 200 {
		t.Errorf("negative strength did not repel: separation %v, was 200", sep)
	}
}

// TestBarnesHutApproximatesTheExactForce is how the approximation is verified:
// theta = 0 disables it and computes the exact O(n²) sum, so the same
// configuration run both ways must agree to within the accuracy theta buys.
// Nothing is asserted about the absolute values — only that the fast path
// tracks the slow one.
func TestBarnesHutApproximatesTheExactForce(t *testing.T) {
	// Distinct positions, so jiggle never fires and the two runs are otherwise
	// identical.
	makeNodes := func() []*Node[datum] {
		r := random.Source(4242)
		nodes := make([]*Node[datum], 400)
		for i := range nodes {
			nodes[i] = &Node[datum]{
				Data: datum{Name: "n"},
				X:    (r.Float64() - 0.5) * 1000,
				Y:    (r.Float64() - 0.5) * 1000,
			}
		}
		return nodes
	}

	velocities := func(theta float64) []state {
		nodes := makeNodes()
		s := NewWithNodes(nodes).
			SetForce("charge", NewManyBody[datum]().SetStrength(-30).SetTheta(theta))
		s.Tick()
		out := make([]state, len(nodes))
		for i, n := range nodes {
			out[i] = state{n.X, n.Y, n.VX, n.VY}
		}
		return out
	}

	exact := velocities(0)

	// The exact run is the reference. Check it is not degenerate.
	var refMag float64
	for _, v := range exact {
		refMag += math.Hypot(v.vx, v.vy)
	}
	refMag /= float64(len(exact))
	if refMag == 0 || math.IsNaN(refMag) {
		t.Fatalf("exact many-body produced no usable force (mean |v| = %v)", refMag)
	}

	// Accuracy must improve monotonically as theta shrinks, and even the
	// default must be close. The tolerances are generous multiples of the
	// reference magnitude, not tuned constants: the claim is "tracks", not
	// "matches".
	prev := math.Inf(1)
	for _, tc := range []struct {
		theta   float64
		maxMean float64
	}{{0.9, 0.15}, {0.5, 0.06}, {0.2, 0.02}} {
		approx := velocities(tc.theta)
		var sum, worst float64
		for i := range exact {
			e := math.Hypot(exact[i].vx-approx[i].vx, exact[i].vy-approx[i].vy)
			sum += e
			if e > worst {
				worst = e
			}
			if math.IsNaN(approx[i].vx) || math.IsInf(approx[i].vx, 0) {
				t.Fatalf("theta=%v produced a non-finite velocity", tc.theta)
			}
		}
		mean := sum / float64(len(exact)) / refMag
		if mean > tc.maxMean {
			t.Errorf("theta=%v: mean relative error %v exceeds %v (worst absolute %v, reference %v)",
				tc.theta, mean, tc.maxMean, worst, refMag)
		}
		if mean > prev {
			t.Errorf("theta=%v: error %v is worse than the coarser setting's %v", tc.theta, mean, prev)
		}
		prev = mean
	}

	// theta = 0 must reproduce itself exactly — it is the same code path with
	// no approximation, so this is a real determinism check on the traversal.
	if !sameAs(exact, velocities(0)) {
		t.Error("the exact many-body force is not deterministic")
	}
}

func TestManyBodyDistanceMaxLimitsRange(t *testing.T) {
	// Two nodes far apart. With no maximum they repel; with a maximum below
	// their separation they must not interact at all.
	build := func(max float64) (*Node[datum], *Node[datum]) {
		a := &Node[datum]{Data: datum{"a"}, X: -500, Y: 0}
		b := &Node[datum]{Data: datum{"b"}, X: 500, Y: 0}
		f := NewManyBody[datum]().SetStrength(-1000)
		if max > 0 {
			f.SetDistanceMax(max)
		}
		NewWithNodes([]*Node[datum]{a, b}).SetForce("charge", f).Tick()
		return a, b
	}

	a, _ := build(0)
	if math.Hypot(a.X+500, a.Y) == 0 {
		t.Error("unbounded many-body did not move a distant node")
	}

	a2, b2 := build(100)
	if a2.X != -500 || a2.Y != 0 || b2.X != 500 || b2.Y != 0 {
		t.Errorf("bounded many-body moved nodes beyond distanceMax: %v %v", a2.X, b2.X)
	}
}

// TestLinkBringsConnectedNodesCloser is the property that defines a link force.
// Two components of identical shape, one wired together and one not, must end up
// with the wired one measurably tighter.
func TestLinkBringsConnectedNodesCloser(t *testing.T) {
	const half = 10

	nodes := make([]*Node[datum], 2*half)
	for i := range nodes {
		nodes[i] = NewNode(datum{Name: "n"})
	}
	// Link the first half into a path; leave the second half unconnected.
	var links []*Link[datum]
	for i := 0; i+1 < half; i++ {
		links = append(links, &Link[datum]{Source: nodes[i], Target: nodes[i+1]})
	}

	s := NewWithNodes(nodes).
		SetForce("charge", NewManyBody[datum]().SetStrength(-30)).
		SetForce("link", NewLink(links).SetDistance(30))
	s.RunUntilStable()
	assertFinite(t, s, "link force")

	meanPairwise := func(group []*Node[datum]) float64 {
		var sum float64
		var count int
		for i, a := range group {
			for _, b := range group[i+1:] {
				sum += dist(a, b)
				count++
			}
		}
		return sum / float64(count)
	}

	linked := meanPairwise(nodes[:half])
	loose := meanPairwise(nodes[half:])
	if linked >= loose {
		t.Errorf("linked nodes (mean separation %v) are not closer than unlinked ones (%v)", linked, loose)
	}

	// Adjacent linked nodes should sit near the requested link distance.
	for i := 0; i+1 < half; i++ {
		if d := dist(nodes[i], nodes[i+1]); d < 5 || d > 120 {
			t.Errorf("link %d-%d rest length %v is nowhere near the requested 30", i, i+1, d)
		}
	}
}

// The degree-based default is a formula over the graph, not over the physics,
// so it is checked directly against degrees the test knows by construction.
func TestLinkDefaultStrengthIsDegreeBased(t *testing.T) {
	// hub is joined to three leaves; leafA is additionally joined to leafB.
	// Degrees: hub 3, leafA 2, leafB 2, leafC 1.
	nodes := make([]*Node[datum], 4)
	for i := range nodes {
		nodes[i] = NewNode(datum{Name: "n"})
	}
	hub, a, b, c := nodes[0], nodes[1], nodes[2], nodes[3]
	links := []*Link[datum]{
		{Source: hub, Target: a},
		{Source: hub, Target: b},
		{Source: hub, Target: c},
		{Source: a, Target: b},
	}
	deg := map[*Node[datum]]float64{hub: 3, a: 2, b: 2, c: 1}

	f := NewLink(links)
	f.Initialize(nodes, random.Source(1))

	for i, l := range links {
		ds, dt := deg[l.Source], deg[l.Target]
		if want := 1 / math.Min(ds, dt); f.strengths[i] != want {
			t.Errorf("link %d strength = %v, want 1/min(%v,%v) = %v", i, f.strengths[i], ds, dt, want)
		}
		if want := ds / (ds + dt); f.bias[i] != want {
			t.Errorf("link %d bias = %v, want %v", i, f.bias[i], want)
		}
		if f.distances[i] != 30 {
			t.Errorf("link %d distance = %v, want the default 30", i, f.distances[i])
		}
		if l.Index != i {
			t.Errorf("link %d has Index %d", i, l.Index)
		}
	}

	// An explicit strength overrides the degree-based default.
	g := NewLink(links).SetStrength(0.05)
	g.Initialize(nodes, random.Source(1))
	for i := range g.strengths {
		if g.strengths[i] != 0.05 {
			t.Fatalf("explicit strength was overridden: %v", g.strengths[i])
		}
	}

	// And the bias is what splits the correction between the endpoints. Set it
	// by hand on an isolated pair — a second link would couple the two axes and
	// muddy the measurement — and check that a bias of 2/3 moves the target
	// exactly twice as far as the source.
	src := &Node[datum]{Data: datum{"src"}, X: 0, Y: 0}
	tgt := &Node[datum]{Data: datum{"tgt"}, X: 300, Y: 0}
	one := NewLink([]*Link[datum]{{Source: src, Target: tgt}}).SetDistance(30)
	sim := NewWithNodes([]*Node[datum]{src, tgt}).SetForce("link", one)
	one.bias[0] = 2.0 / 3.0
	sim.Tick()

	srcMoved := math.Abs(src.X)
	tgtMoved := math.Abs(tgt.X - 300)
	if ratio := tgtMoved / srcMoved; math.Abs(ratio-2) > 1e-9 {
		t.Errorf("target moved %v and source %v (ratio %v), want a 2:1 split from bias 2/3",
			tgtMoved, srcMoved, ratio)
	}
}

func TestLinkIterationsAreApplied(t *testing.T) {
	build := func(iters int) float64 {
		a := &Node[datum]{Data: datum{"a"}, X: 0, Y: 0}
		b := &Node[datum]{Data: datum{"b"}, X: 300, Y: 0}
		links := []*Link[datum]{{Source: a, Target: b}}
		NewWithNodes([]*Node[datum]{a, b}).
			SetForce("link", NewLink(links).SetDistance(30).SetIterations(iters)).
			Tick()
		return dist(a, b)
	}
	one, many := build(1), build(10)
	if many >= one {
		t.Errorf("10 iterations left the spring at %v, no better than 1 iteration's %v", many, one)
	}
}

func TestLinkRejectsForeignNodes(t *testing.T) {
	inside := NewNode(datum{"in"})
	outside := NewNode(datum{"out"})
	defer func() {
		if recover() == nil {
			t.Error("a link to a node outside the simulation did not panic")
		}
	}()
	NewWithNodes([]*Node[datum]{inside}).
		SetForce("link", NewLink([]*Link[datum]{{Source: inside, Target: outside}}))
}

// TestCenterMovesCentroidTowardTarget checks the defining property, and the
// second half checks the other one: centring translates, so it must not change
// the layout's shape.
func TestCenterMovesCentroidTowardTarget(t *testing.T) {
	const tx, ty = 400.0, 250.0

	s := New(data(30)).
		SetForce("charge", NewManyBody[datum]()).
		SetForce("center", NewCenter[datum](tx, ty))

	centroid := func() (float64, float64) {
		var cx, cy float64
		for _, n := range s.Nodes() {
			cx += n.X
			cy += n.Y
		}
		return cx / float64(len(s.Nodes())), cy / float64(len(s.Nodes()))
	}

	cx, cy := centroid()
	before := math.Hypot(cx-tx, cy-ty)
	if before < 100 {
		t.Fatalf("test setup: centroid already at the target (distance %v)", before)
	}

	s.Tick()
	cx, cy = centroid()
	if after := math.Hypot(cx-tx, cy-ty); after >= before {
		t.Errorf("centroid distance to target went from %v to %v", before, after)
	}

	s.RunUntilStable()
	cx, cy = centroid()
	// Not exactly zero: the centring runs before the integration step, so the
	// centroid drifts back by whatever mean velocity is left over.
	if d := math.Hypot(cx-tx, cy-ty); d > 1e-3 {
		t.Errorf("settled centroid is %v from the target", d)
	}
	assertFinite(t, s, "center force")

	// A weaker centring still converges on the target, just more slowly.
	weak := New(data(30)).SetForce("center", NewCenter[datum](tx, ty).SetStrength(0.1))
	weak.RunUntilStable()
	var wx, wy float64
	for _, n := range weak.Nodes() {
		wx += n.X
		wy += n.Y
	}
	wx /= 30
	wy /= 30
	if d := math.Hypot(wx-tx, wy-ty); d > 1 {
		t.Errorf("weak centring settled %v from the target", d)
	}
}

func TestCenterPreservesShape(t *testing.T) {
	nodes := []*Node[datum]{
		{Data: datum{"a"}, X: 0, Y: 0},
		{Data: datum{"b"}, X: 10, Y: 0},
		{Data: datum{"c"}, X: 0, Y: 10},
	}
	before := [3]float64{dist(nodes[0], nodes[1]), dist(nodes[0], nodes[2]), dist(nodes[1], nodes[2])}
	NewWithNodes(nodes).SetForce("center", NewCenter[datum](1000, -500)).Tick()
	after := [3]float64{dist(nodes[0], nodes[1]), dist(nodes[0], nodes[2]), dist(nodes[1], nodes[2])}
	for i := range before {
		if math.Abs(before[i]-after[i]) > 1e-9 {
			t.Errorf("centring changed pairwise distance %d from %v to %v", i, before[i], after[i])
		}
	}
}

// TestCollideResolvesOverlaps is the property test for collision: after the
// simulation settles, no two circles may overlap by more than a tolerance.
func TestCollideResolvesOverlaps(t *testing.T) {
	const radius = 12.0

	// Start everything piled in a small area so there is a lot to resolve.
	r := random.Source(31337)
	nodes := make([]*Node[datum], 120)
	for i := range nodes {
		nodes[i] = &Node[datum]{
			Data: datum{Name: "n"},
			X:    (r.Float64() - 0.5) * 40,
			Y:    (r.Float64() - 0.5) * 40,
		}
	}

	s := NewWithNodes(nodes).
		SetForce("collide", NewCollide[datum]().SetRadius(radius).SetIterations(4))

	worstBefore := worstOverlap(nodes, radius)
	if worstBefore < radius {
		t.Fatalf("test setup: nodes were not overlapping enough (worst %v)", worstBefore)
	}

	s.RunUntilStable()
	assertFinite(t, s, "collide force")

	worst := worstOverlap(nodes, radius)
	// Tolerance is a fraction of the radius: the resolution is soft, so exact
	// zero is not a promise the force makes.
	if worst > 0.05*radius {
		t.Errorf("worst overlap after settling is %v, more than 5%% of the radius %v", worst, radius)
	}
	if worst >= worstBefore {
		t.Errorf("overlap did not improve: %v then %v", worstBefore, worst)
	}
}

func worstOverlap(nodes []*Node[datum], radius float64) float64 {
	worst := 0.0
	for i, a := range nodes {
		for _, b := range nodes[i+1:] {
			if o := 2*radius - dist(a, b); o > worst {
				worst = o
			}
		}
	}
	return worst
}

func TestCollideRespectsPerNodeRadius(t *testing.T) {
	// A big circle and a small one, coincident. They must separate to the sum
	// of their radii, and the small one must move further.
	big := &Node[datum]{Data: datum{"big"}, X: 0, Y: 0}
	small := &Node[datum]{Data: datum{"small"}, X: 0, Y: 0}
	radii := map[*Node[datum]]float64{big: 40, small: 5}

	s := NewWithNodes([]*Node[datum]{big, small}).
		SetForce("collide", NewCollide[datum]().
			SetRadiusFunc(func(n *Node[datum]) float64 { return radii[n] }).
			SetIterations(8))
	s.RunUntilStable()
	assertFinite(t, s, "per-node collide radius")

	// The guarantee is non-overlap, not exact tangency: two coincident circles
	// are separated by a jiggle-sized nudge amplified by the 1/d term, which
	// overshoots, and nothing ever pulls them back.
	if d := dist(big, small); d < 45 {
		t.Errorf("separation %v, want at least 45 (40 + 5)", d)
	}
	if math.Hypot(small.X, small.Y) <= math.Hypot(big.X, big.Y) {
		t.Error("the small circle should have given way to the big one")
	}
}

func TestXAndYForcesPullToTarget(t *testing.T) {
	s := New(data(20)).
		SetForce("x", NewX[datum](300)).
		SetForce("y", NewY[datum](-150))
	s.RunUntilStable()
	assertFinite(t, s, "x/y forces")

	for _, n := range s.Nodes() {
		if math.Abs(n.X-300) > 5 || math.Abs(n.Y+150) > 5 {
			t.Errorf("node %d settled at (%v, %v), want near (300, -150)", n.Index, n.X, n.Y)
		}
	}

	// The per-node accessor form: each node gets its own column.
	s2 := New(data(9)).
		SetForce("x", NewX[datum](0).SetXFunc(func(n *Node[datum]) float64 {
			return float64(n.Index) * 100
		}).SetStrength(0.5))
	s2.RunUntilStable()
	for _, n := range s2.Nodes() {
		if math.Abs(n.X-float64(n.Index)*100) > 5 {
			t.Errorf("node %d settled at x=%v, want near %v", n.Index, n.X, n.Index*100)
		}
	}

	// A y force must not touch x, and vice versa.
	only := &Node[datum]{Data: datum{"o"}, X: 77, Y: 0}
	NewWithNodes([]*Node[datum]{only}).SetForce("y", NewY[datum](500)).RunUntilStable()
	if only.X != 77 {
		t.Errorf("the y force moved x to %v", only.X)
	}
	if math.Abs(only.Y-500) > 5 {
		t.Errorf("the y force settled at y=%v, want near 500", only.Y)
	}
}

func TestRadialPullsToCircle(t *testing.T) {
	const cx, cy, radius = 100.0, -60.0, 250.0

	s := New(data(40)).
		SetForce("charge", NewManyBody[datum]().SetStrength(-20)).
		SetForce("radial", NewRadial[datum](radius, cx, cy).SetStrength(0.8))
	s.RunUntilStable()
	assertFinite(t, s, "radial force")

	for _, n := range s.Nodes() {
		r := math.Hypot(n.X-cx, n.Y-cy)
		if math.Abs(r-radius) > 15 {
			t.Errorf("node %d settled at radius %v, want near %v", n.Index, r, radius)
		}
	}

	// Per-node radii give concentric rings.
	s2 := New(data(12)).
		SetForce("radial", NewRadial[datum](0, 0, 0).
			SetRadiusFunc(func(n *Node[datum]) float64 { return 50 * float64(1+n.Index%3) }).
			SetStrength(0.9))
	s2.RunUntilStable()
	for _, n := range s2.Nodes() {
		want := 50 * float64(1+n.Index%3)
		if r := math.Hypot(n.X, n.Y); math.Abs(r-want) > 10 {
			t.Errorf("node %d settled at radius %v, want near %v", n.Index, r, want)
		}
	}
}

func TestRadialHandlesNodeAtTheCentre(t *testing.T) {
	// A node exactly at the centre has no direction to be pushed in; the force
	// must produce a finite result anyway.
	n := &Node[datum]{Data: datum{"c"}, X: 0, Y: 0}
	s := NewWithNodes([]*Node[datum]{n}).SetForce("radial", NewRadial[datum](100, 0, 0))
	s.RunUntilStable()
	assertFinite(t, s, "radial from the centre")
}

// TestNoNaNUnderPathologicalInput is the blanket guarantee: whatever the input
// does, a position or velocity must never become NaN or Inf.
func TestNoNaNUnderPathologicalInput(t *testing.T) {
	cases := map[string]func() *Simulation[datum]{
		"all coincident at the origin": func() *Simulation[datum] {
			nodes := make([]*Node[datum], 50)
			for i := range nodes {
				nodes[i] = &Node[datum]{Data: datum{"n"}}
			}
			return NewWithNodes(nodes)
		},
		"two nodes, everything on": func() *Simulation[datum] {
			a := &Node[datum]{Data: datum{"a"}}
			b := &Node[datum]{Data: datum{"b"}}
			return NewWithNodes([]*Node[datum]{a, b}).
				SetForce("link", NewLink([]*Link[datum]{{Source: a, Target: b}}))
		},
		"single node": func() *Simulation[datum] {
			return New(data(1))
		},
		"no nodes": func() *Simulation[datum] {
			return New([]datum{})
		},
		"huge coordinates": func() *Simulation[datum] {
			nodes := make([]*Node[datum], 20)
			for i := range nodes {
				nodes[i] = &Node[datum]{Data: datum{"n"}, X: 1e12 * float64(i), Y: -1e12 * float64(i)}
			}
			return NewWithNodes(nodes)
		},
		"tiny coordinates": func() *Simulation[datum] {
			nodes := make([]*Node[datum], 20)
			for i := range nodes {
				nodes[i] = &Node[datum]{Data: datum{"n"}, X: 1e-14 * float64(i), Y: 1e-14 * float64(i)}
			}
			return NewWithNodes(nodes)
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			s := build()
			s.SetForce("charge", NewManyBody[datum]()).
				SetForce("collide", NewCollide[datum]().SetRadius(6).SetIterations(2)).
				SetForce("center", NewCenter[datum](0, 0)).
				SetForce("x", NewX[datum](10)).
				SetForce("y", NewY[datum](10)).
				SetForce("radial", NewRadial[datum](100, 0, 0))
			s.RunUntilStable()
			assertFinite(t, s, name)
		})
	}
}

func BenchmarkManyBodyTick(b *testing.B) {
	s := New(data(1000)).SetForce("charge", NewManyBody[datum]())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SetAlpha(1)
		s.Tick()
	}
}
