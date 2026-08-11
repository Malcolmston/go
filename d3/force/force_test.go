package force

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/random"
)

// The tests in this file assert *properties*, never literal coordinates. Node
// positions after N ticks of a velocity Verlet integrator are not derivable by
// hand, so a table of expected numbers would only be a recording of whatever the
// code did the day it was written. What can be checked, and is, is that the
// simulation converges, that each force does the thing it is named for, and that
// a seeded run is reproducible.

type datum struct{ Name string }

func data(n int) []datum {
	d := make([]datum, n)
	for i := range d {
		d[i] = datum{Name: string(rune('a' + i%26))}
	}
	return d
}

func dist(a, b *Node[datum]) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}

func assertFinite(t *testing.T, s *Simulation[datum], when string) {
	t.Helper()
	for _, n := range s.Nodes() {
		for name, v := range map[string]float64{"X": n.X, "Y": n.Y, "VX": n.VX, "VY": n.VY} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("%s: node %d %s = %v", when, n.Index, name, v)
			}
		}
	}
}

func TestInitialPlacementIsSpreadOut(t *testing.T) {
	s := New(data(50))
	assertFinite(t, s, "after New")

	// Phyllotaxis: no two nodes coincide, and the radius grows.
	for i, a := range s.Nodes() {
		if a.Index != i {
			t.Fatalf("node %d has Index %d", i, a.Index)
		}
		for _, b := range s.Nodes()[i+1:] {
			if dist(a, b) == 0 {
				t.Fatalf("nodes %d and %d were placed at the same point", a.Index, b.Index)
			}
		}
	}
	first := math.Hypot(s.Nodes()[0].X, s.Nodes()[0].Y)
	last := math.Hypot(s.Nodes()[49].X, s.Nodes()[49].Y)
	if last <= first {
		t.Errorf("phyllotaxis radius did not grow: %v then %v", first, last)
	}
}

func TestExplicitPositionsAreKept(t *testing.T) {
	nodes := []*Node[datum]{
		{Data: datum{"a"}, X: 3, Y: 4},
		{Data: datum{"b"}, X: math.NaN(), Y: math.NaN()},
	}
	NewWithNodes(nodes)
	if nodes[0].X != 3 || nodes[0].Y != 4 {
		t.Errorf("explicit position was overwritten: %v %v", nodes[0].X, nodes[0].Y)
	}
	if math.IsNaN(nodes[1].X) || math.IsNaN(nodes[1].Y) {
		t.Error("NaN position was not replaced")
	}
}

// TestConverges is the headline property: alpha decays monotonically toward the
// target, the run terminates, and once it has, Tick does nothing at all.
func TestConverges(t *testing.T) {
	s := New(data(40)).
		SetForce("charge", NewManyBody[datum]()).
		SetForce("center", NewCenter[datum](0, 0))

	prev := s.Alpha()
	if prev != 1 {
		t.Fatalf("initial alpha = %v, want 1", prev)
	}

	ticks := 0
	for !s.Converged() {
		s.Tick()
		ticks++
		if a := s.Alpha(); a >= prev {
			t.Fatalf("alpha did not decrease at tick %d: %v then %v", ticks, prev, a)
		} else if a <= s.AlphaTarget() {
			t.Fatalf("alpha %v went past the target %v", a, s.AlphaTarget())
		}
		prev = s.Alpha()
		if ticks > 10000 {
			t.Fatal("simulation did not converge")
		}
	}
	if ticks < 100 || ticks > 500 {
		t.Errorf("converged in %d ticks; the default decay should take roughly 300", ticks)
	}
	assertFinite(t, s, "after convergence")

	// Tick is now a no-op.
	before := snapshot(s)
	s.Tick().Tick().Tick()
	if !sameAs(before, snapshot(s)) {
		t.Error("Tick moved nodes after convergence")
	}
	if n := s.TickN(50); n != 0 {
		t.Errorf("TickN after convergence ran %d ticks, want 0", n)
	}
}

func TestRunUntilStable(t *testing.T) {
	s := New(data(30)).SetForce("charge", NewManyBody[datum]())
	n := s.RunUntilStable()
	if !s.Converged() {
		t.Errorf("RunUntilStable returned after %d ticks without converging (alpha %v)", n, s.Alpha())
	}
	if n == 0 {
		t.Error("RunUntilStable did nothing")
	}
	// Idempotent: a second call has nothing left to do.
	if again := s.RunUntilStable(); again != 0 {
		t.Errorf("second RunUntilStable ran %d ticks, want 0", again)
	}
	assertFinite(t, s, "after RunUntilStable")
}

// A configuration that can never converge must still terminate.
func TestRunUntilStableTerminatesWithAlphaTarget(t *testing.T) {
	s := New(data(10)).
		SetForce("charge", NewManyBody[datum]()).
		SetAlphaTarget(0.5) // far above alphaMin: alpha can never reach it
	n := s.RunUntilStable()
	if n == 0 {
		t.Error("RunUntilStable with a high alpha target did nothing")
	}
	if s.Converged() {
		t.Error("simulation converged despite an alpha target above alphaMin")
	}
	if s.Alpha() <= s.AlphaTarget() {
		t.Errorf("alpha %v decayed past its target %v", s.Alpha(), s.AlphaTarget())
	}
	assertFinite(t, s, "after a capped run")

	// Zero decay is the other non-converging configuration.
	z := New(data(10)).SetForce("charge", NewManyBody[datum]()).SetAlphaDecay(0)
	if z.RunUntilStable() == 0 {
		t.Error("RunUntilStable with zero decay did nothing")
	}
	if z.Alpha() != 1 {
		t.Errorf("alpha = %v with zero decay, want 1", z.Alpha())
	}
}

func TestStopAndRestart(t *testing.T) {
	s := New(data(20)).SetForce("charge", NewManyBody[datum]())
	s.TickN(10)
	s.Stop()

	before := snapshot(s)
	alpha := s.Alpha()
	s.Tick()
	if !sameAs(before, snapshot(s)) {
		t.Error("Tick moved nodes while stopped")
	}
	if s.Alpha() != alpha {
		t.Error("alpha decayed while stopped")
	}

	s.Restart()
	if s.Alpha() != 1 {
		t.Errorf("Restart left alpha at %v, want 1", s.Alpha())
	}
	s.Tick()
	if sameAs(before, snapshot(s)) {
		t.Error("Tick did nothing after Restart")
	}
}

// TestSeedIsReproducible is the determinism guarantee. Two independently
// constructed simulations with the same seed must agree bit for bit — including
// through the coincident-node paths where jiggle actually draws from the source.
func TestSeedIsReproducible(t *testing.T) {
	build := func(seed float64) *Simulation[datum] {
		// Deliberately start every node at the origin so that jiggle fires.
		nodes := make([]*Node[datum], 30)
		for i := range nodes {
			nodes[i] = &Node[datum]{Data: datum{Name: "n"}}
		}
		links := []*Link[datum]{
			{Source: nodes[0], Target: nodes[1]},
			{Source: nodes[1], Target: nodes[2]},
			{Source: nodes[2], Target: nodes[0]},
		}
		s := NewWithNodes(nodes).
			SetForce("charge", NewManyBody[datum]()).
			SetForce("link", NewLink(links)).
			SetForce("collide", NewCollide[datum]().SetRadius(5)).
			SetForce("center", NewCenter[datum](100, 100))
		if seed != 0 {
			s.SetRandom(random.Source(seed))
		}
		return s
	}

	a, b := build(0), build(0)
	a.RunUntilStable()
	b.RunUntilStable()
	sa, sb := snapshot(a), snapshot(b)
	if !sameAs(sa, sb) {
		t.Fatal("two runs with the default seed produced different output")
	}
	assertFinite(t, a, "after a jiggle-heavy run")

	// A different seed must produce a different result, or the seed is not
	// actually reaching the jiggle.
	c := build(12345)
	c.RunUntilStable()
	if sameAs(sa, snapshot(c)) {
		t.Error("a different seed produced identical output")
	}
}

// Force application order is part of the determinism guarantee, so it has to be
// observable and stable.
func TestForceOrderIsStable(t *testing.T) {
	var order []string
	rec := func(name string, log *[]string) *recorder[datum] {
		return &recorder[datum]{name: name, log: log}
	}
	s := New(data(3)).
		SetForce("a", rec("a", &order)).
		SetForce("b", rec("b", &order)).
		SetForce("c", rec("c", &order))

	s.Tick()
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("forces applied in order %v, want [a b c]", order)
	}

	// Replacing a force keeps its slot; removing one drops it.
	order = nil
	s.SetForce("b", rec("B", &order))
	s.Tick()
	if len(order) != 3 || order[1] != "B" {
		t.Fatalf("after replacement, order %v, want b's slot held by B", order)
	}

	order = nil
	s.RemoveForce("a")
	s.Tick()
	if len(order) != 2 || order[0] != "B" || order[1] != "c" {
		t.Fatalf("after removal, order %v, want [B c]", order)
	}
	if s.Force("a") != nil {
		t.Error("removed force is still registered")
	}
	if s.Force("c") == nil {
		t.Error("surviving force is not registered")
	}
	s.RemoveForce("nonexistent") // must not panic
}

type recorder[T any] struct {
	name string
	log  *[]string
}

func (r *recorder[T]) Initialize([]*Node[T], *random.Rand) {}
func (r *recorder[T]) Apply(float64)                       { *r.log = append(*r.log, r.name) }

func TestFixedNodesArePinnedExactly(t *testing.T) {
	s := New(data(25)).
		SetForce("charge", NewManyBody[datum]().SetStrength(-200)).
		SetForce("center", NewCenter[datum](500, 500))

	pinned := s.Nodes()[0]
	pinned.Fix(-17.5, 42.25)
	halfPinned := s.Nodes()[1]
	halfPinned.FixX(3)

	s.RunUntilStable()

	if pinned.X != -17.5 || pinned.Y != 42.25 {
		t.Errorf("pinned node moved to (%v, %v), want (-17.5, 42.25)", pinned.X, pinned.Y)
	}
	if pinned.VX != 0 || pinned.VY != 0 {
		t.Errorf("pinned node has velocity (%v, %v), want zero", pinned.VX, pinned.VY)
	}
	if halfPinned.X != 3 {
		t.Errorf("x-pinned node moved to x=%v, want 3", halfPinned.X)
	}
	if halfPinned.Y == 0 {
		t.Error("x-pinned node should still be free in y")
	}

	// Unpinning lets it move again.
	pinned.Unfix()
	s.Restart().TickN(20)
	if pinned.X == -17.5 && pinned.Y == 42.25 {
		t.Error("unpinned node did not move")
	}
	assertFinite(t, s, "with pinned nodes")
}

func TestFindAndFindWithin(t *testing.T) {
	s := New(data(40)).SetForce("charge", NewManyBody[datum]())
	s.RunUntilStable()

	target := s.Nodes()[7]
	got, ok := s.Find(target.X, target.Y)
	if !ok || got != target {
		t.Fatalf("Find at a node's own position returned %v (%v)", got, ok)
	}

	// FindWithin agrees with a linear scan, including when nothing is near.
	far := 1e6
	if _, ok := s.FindWithin(far, far, 10); ok {
		t.Error("FindWithin found something a million units away")
	}
	if _, ok := s.FindWithin(target.X+1, target.Y+1, 5); !ok {
		t.Error("FindWithin missed a node 1.4 units away")
	}

	empty := New([]datum{})
	if _, ok := empty.Find(0, 0); ok {
		t.Error("Find on an empty simulation found something")
	}
}

func TestConfigurationRoundTrips(t *testing.T) {
	s := New(data(3)).
		SetAlpha(0.5).
		SetAlphaMin(0.01).
		SetAlphaDecay(0.05).
		SetAlphaTarget(0.002).
		SetVelocityDecay(0.25)

	if s.Alpha() != 0.5 || s.AlphaMin() != 0.01 || s.AlphaDecay() != 0.05 || s.AlphaTarget() != 0.002 {
		t.Errorf("alpha configuration did not round-trip: %v %v %v %v",
			s.Alpha(), s.AlphaMin(), s.AlphaDecay(), s.AlphaTarget())
	}
	if math.Abs(s.VelocityDecay()-0.25) > 1e-12 {
		t.Errorf("VelocityDecay() = %v, want 0.25", s.VelocityDecay())
	}
	if s.Random() == nil {
		t.Error("Random() is nil")
	}
}

func TestSetNodesReinitializesForces(t *testing.T) {
	s := New(data(5)).SetForce("charge", NewManyBody[datum]().SetStrength(-40))
	// Growing the node set must not leave the force indexing a shorter array.
	s.SetNodes(append(s.Nodes(), NewNode(datum{"z"}), NewNode(datum{"y"})))
	if len(s.Nodes()) != 7 {
		t.Fatalf("SetNodes left %d nodes", len(s.Nodes()))
	}
	s.RunUntilStable()
	assertFinite(t, s, "after growing the node set")
	for i, n := range s.Nodes() {
		if n.Index != i {
			t.Fatalf("node %d has Index %d after SetNodes", i, n.Index)
		}
	}
}

// snapshot/sameAs compare full simulation state exactly — no tolerance, because
// the determinism claim is bit-for-bit.

type state struct{ x, y, vx, vy float64 }

func snapshot(s *Simulation[datum]) []state {
	out := make([]state, len(s.Nodes()))
	for i, n := range s.Nodes() {
		out[i] = state{n.X, n.Y, n.VX, n.VY}
	}
	return out
}

func sameAs(a, b []state) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
