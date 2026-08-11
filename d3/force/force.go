// Package force is a Go port of d3-force — a velocity Verlet particle
// simulation for laying out graphs and packing points, built on the sibling
// d3/quadtree package for its Barnes–Hut approximation and on d3/random for
// reproducibility.
//
// A simulation is a set of [Node] values carrying a position and a velocity, and
// a set of [Force] values that adjust those velocities each step. Nothing here
// draws anything: the output is the X and Y written onto the nodes.
//
//	sim := force.New(people).
//		SetForce("charge", force.NewManyBody[Person]()).
//		SetForce("link", force.NewLink(links)).
//		SetForce("center", force.NewCenter[Person](480, 270))
//
//	sim.RunUntilStable()
//	for _, n := range sim.Nodes() {
//		fmt.Println(n.Data.Name, n.X, n.Y)
//	}
//
// # The simulation is stepped, not scheduled
//
// This is the one deep difference from upstream and the reason to read this
// paragraph before any code. JavaScript d3-force is driven by a
// requestAnimationFrame timer: `d3.forceSimulation(nodes)` starts running
// immediately, fires a "tick" event sixty times a second, and stops itself when
// alpha falls below alphaMin. The layout is a side effect of the browser's frame
// clock.
//
// There is no frame clock here, and rather than inventing one out of
// time.Ticker this port makes the stepping explicit. [Simulation.Tick] advances
// the simulation exactly one step and returns. [Simulation.RunUntilStable] ticks
// until alpha has decayed past alphaMin and returns how many steps that took.
// Nothing runs unless the caller asks.
//
// For the job this port exists to do — computing a layout on a server and
// handing the coordinates to a renderer — that is strictly better than a timer.
// The caller decides when the answer is final, gets it synchronously, and can
// serialize the result knowing no goroutine is still moving the nodes. Code that
// genuinely wants animation can call Tick from its own clock and read positions
// between steps, which is what the tick event was for anyway.
//
// The consequences are worth stating plainly. [Simulation.Stop] and
// [Simulation.Restart] control a flag rather than a timer, there are no "tick"
// or "end" events to listen for (a caller who wants per-step output writes the
// loop), and a freshly constructed simulation has done no work — nodes sit at
// their initial phyllotaxis positions until the first Tick.
//
// # Determinism
//
// Given the same nodes in the same order, the same forces added in the same
// order and the same random seed, this package produces bit-identical output on
// every run. That is a guarantee, not an accident, and three things buy it:
//
//   - Randomness comes from an explicit [random.Rand], defaulting to
//     random.Source(1) — the same LCG constants and the same initial state as
//     d3-force's own default. Randomness enters only through "jiggle", the
//     1e-6 nudge applied to coincident points so that a division by zero becomes
//     a tiny push in an arbitrary direction.
//   - Forces are applied in the order they were added, held in a slice. A map
//     would have made the output depend on Go's randomized map iteration, which
//     would have been a fine way to make a layout irreproducible.
//   - The quadtree traversals underneath ManyBody and Collide are deterministic
//     in quadrant order.
//
// Floating-point summation is order-dependent, so all three matter. Reordering
// the SetForce calls is a real change to the result, not a stylistic one.
//
// # Initial positions
//
// Nodes with no position are placed on a phyllotaxis spiral — the sunflower-seed
// arrangement, radius 10·sqrt(i+0.5) at angle i·π(3−√5) — which spreads them
// evenly and deterministically instead of piling them on the origin where every
// repulsion would be a division by zero.
//
// "No position" means NaN, matching upstream's undefined, because zero is a
// perfectly good coordinate and cannot be used as a sentinel. [NewNode] and
// [New] produce nodes marked that way; a Node built as a bare struct literal
// starts at the origin and stays there. Prefer [New], which builds the nodes
// from your data for you.
//
// # Concurrency
//
// A Simulation and its nodes are mutable throughout. Nothing here is safe for
// concurrent use, and reading positions from another goroutine while a Tick is
// in flight will read torn state. Finish the run, then publish the nodes.
package force

import (
	"math"

	"github.com/malcolmston/d3/random"
)

// Node is one particle: the caller's datum plus the state the simulation reads
// and writes.
//
// The state lives here rather than in a side table for the same reason
// d3/hierarchy puts layout coordinates on its nodes — the alternative is a
// map[*Node]state that every force, every callback and every renderer has to
// thread through, that allocates on the hot path, and that can go stale relative
// to the node set it describes. The cost is six float64s and an int per node.
//
// Create nodes with [NewNode] or let [New] do it. A zero Node is at the origin
// with no velocity, which is legal but means every node built that way starts on
// top of every other; see the package doc on initial positions.
type Node[T any] struct {
	// Data is the caller's value for this node.
	Data T

	// Index is the node's position in the simulation's node slice, assigned by
	// [Simulation.SetNodes]. Forces use it to index their per-node arrays.
	Index int

	// X and Y are the position, written by the simulation every tick. This is
	// the output.
	X, Y float64

	// VX and VY are the velocity. Forces add to it; the simulation applies it
	// to the position and then damps it by the velocity decay.
	VX, VY float64

	// FX and FY pin the node. When non-nil the corresponding coordinate is held
	// exactly at that value and its velocity is zeroed every tick, so the node
	// does not move and cannot be pushed. Pinning one axis and not the other is
	// allowed. Use [Node.Fix] and [Node.Unfix] rather than taking addresses by
	// hand.
	FX, FY *float64
}

// NewNode returns a node holding d, marked for phyllotaxis placement: its
// position is NaN until the simulation assigns one.
func NewNode[T any](d T) *Node[T] {
	return &Node[T]{Data: d, X: math.NaN(), Y: math.NaN()}
}

// Fix pins the node at (x, y). It will be held exactly there — and its velocity
// zeroed — on every tick until [Node.Unfix].
func (n *Node[T]) Fix(x, y float64) *Node[T] {
	n.FX, n.FY = &x, &y
	return n
}

// FixX pins only the node's x coordinate, leaving y free.
func (n *Node[T]) FixX(x float64) *Node[T] { n.FX = &x; return n }

// FixY pins only the node's y coordinate, leaving x free.
func (n *Node[T]) FixY(y float64) *Node[T] { n.FY = &y; return n }

// Unfix releases both pins. The node keeps the position it was pinned at and is
// free to move from there.
func (n *Node[T]) Unfix() *Node[T] {
	n.FX, n.FY = nil, nil
	return n
}

// Force is something that adjusts node velocities once per tick.
//
// Initialize is called whenever the simulation's node set changes, and is where
// a force precomputes its per-node arrays — link counts, charge strengths,
// collision radii — indexed by [Node.Index]. It also receives the simulation's
// random source, which a force must use for any randomness it needs if the
// simulation is to stay reproducible.
//
// Apply is called once per tick with the current alpha, the "temperature" that
// scales every force down as the layout settles.
type Force[T any] interface {
	Initialize(nodes []*Node[T], rnd *random.Rand)
	Apply(alpha float64)
}

// namedForce keeps forces in insertion order. See the package doc on
// determinism for why this is a slice and not a map.
type namedForce[T any] struct {
	name  string
	force Force[T]
}

const (
	// initialRadius and initialAngle define the phyllotaxis spiral used to
	// place nodes that have no position. The angle is the golden angle, which
	// is what makes the spiral fill space evenly rather than forming spokes.
	initialRadius = 10.0

	defaultAlpha         = 1.0
	defaultAlphaMin      = 0.001
	defaultVelocityDecay = 0.6
)

var initialAngle = math.Pi * (3 - math.Sqrt(5))

// Simulation is a set of nodes and the forces acting on them.
//
// Build one with [New] or [NewWithNodes], configure it with the chainable
// setters, and drive it with [Simulation.Tick] or [Simulation.RunUntilStable].
type Simulation[T any] struct {
	nodes  []*Node[T]
	forces []namedForce[T]

	alpha         float64
	alphaMin      float64
	alphaDecay    float64
	alphaTarget   float64
	velocityDecay float64

	rnd *random.Rand

	stopped bool
}

// New returns a simulation over one node per datum, each placed on the
// phyllotaxis spiral. This is the usual entry point.
//
// Reach the nodes — and therefore the computed positions — through
// [Simulation.Nodes]; they are created in the order of the data.
func New[T any](data []T) *Simulation[T] {
	nodes := make([]*Node[T], len(data))
	for i, d := range data {
		nodes[i] = NewNode(d)
	}
	return NewWithNodes(nodes)
}

// NewWithNodes returns a simulation over nodes the caller built, which is how to
// start from known positions — reusing the result of an earlier run, or seeding
// a layout from a previous frame so it does not jump.
//
// A node whose X or Y is NaN is placed on the phyllotaxis spiral; every other
// node keeps the position it arrived with. The slice is retained, not copied.
func NewWithNodes[T any](nodes []*Node[T]) *Simulation[T] {
	s := &Simulation[T]{
		alpha:         defaultAlpha,
		alphaMin:      defaultAlphaMin,
		alphaTarget:   0,
		velocityDecay: defaultVelocityDecay,
		rnd:           random.Source(1),
	}
	// d3 derives the default decay from alphaMin so that a default simulation
	// takes about 300 ticks to converge, whatever alphaMin is set to.
	s.alphaDecay = 1 - math.Pow(s.alphaMin, 1.0/300)
	s.SetNodes(nodes)
	return s
}

// Nodes returns the simulation's nodes — the same slice it is working on, not a
// copy, because the nodes are the output and copying the slice would only hide
// that its elements are shared anyway.
func (s *Simulation[T]) Nodes() []*Node[T] { return s.nodes }

// SetNodes replaces the node set and returns the receiver.
//
// Indices are reassigned, missing positions are filled in, and every force is
// re-initialized — forces keep per-node arrays indexed by [Node.Index], so
// changing the nodes without telling the forces would be a memory-safety
// problem, not just a wrong layout.
func (s *Simulation[T]) SetNodes(nodes []*Node[T]) *Simulation[T] {
	s.nodes = nodes
	s.initializeNodes()
	for _, nf := range s.forces {
		nf.force.Initialize(s.nodes, s.rnd)
	}
	return s
}

func (s *Simulation[T]) initializeNodes() {
	for i, n := range s.nodes {
		n.Index = i
		if n.FX != nil {
			n.X = *n.FX
		}
		if n.FY != nil {
			n.Y = *n.FY
		}
		if math.IsNaN(n.X) || math.IsNaN(n.Y) {
			radius := initialRadius * math.Sqrt(0.5+float64(i))
			angle := float64(i) * initialAngle
			n.X = radius * math.Cos(angle)
			n.Y = radius * math.Sin(angle)
		}
		if math.IsNaN(n.VX) || math.IsNaN(n.VY) {
			n.VX, n.VY = 0, 0
		}
	}
}

// Force returns the force registered under name, or nil.
func (s *Simulation[T]) Force(name string) Force[T] {
	for _, nf := range s.forces {
		if nf.name == name {
			return nf.force
		}
	}
	return nil
}

// SetForce registers a force under a name and returns the receiver. Registering
// over an existing name replaces it in place, keeping its position in the
// application order; a new name is appended.
//
// The force is initialized immediately with the current nodes.
func (s *Simulation[T]) SetForce(name string, f Force[T]) *Simulation[T] {
	f.Initialize(s.nodes, s.rnd)
	for i := range s.forces {
		if s.forces[i].name == name {
			s.forces[i].force = f
			return s
		}
	}
	s.forces = append(s.forces, namedForce[T]{name, f})
	return s
}

// RemoveForce unregisters a force and returns the receiver. Removing a name that
// was never registered is a no-op.
func (s *Simulation[T]) RemoveForce(name string) *Simulation[T] {
	for i := range s.forces {
		if s.forces[i].name == name {
			s.forces = append(s.forces[:i], s.forces[i+1:]...)
			return s
		}
	}
	return s
}

// Alpha returns the current alpha: the simulation's temperature, which scales
// every force and decays toward the alpha target each tick.
func (s *Simulation[T]) Alpha() float64 { return s.alpha }

// SetAlpha sets alpha and returns the receiver. Raising it reheats a settled
// layout, which is what to do after moving a node by hand.
func (s *Simulation[T]) SetAlpha(a float64) *Simulation[T] { s.alpha = a; return s }

// AlphaMin returns the alpha below which the simulation is considered converged
// and [Simulation.Tick] becomes a no-op.
func (s *Simulation[T]) AlphaMin() float64 { return s.alphaMin }

// SetAlphaMin sets the convergence threshold and returns the receiver. It does
// not recompute the alpha decay, so lowering it makes a run longer in the
// obvious way.
func (s *Simulation[T]) SetAlphaMin(m float64) *Simulation[T] { s.alphaMin = m; return s }

// AlphaDecay returns the per-tick decay rate: alpha moves this fraction of the
// way from its current value to the alpha target each tick.
func (s *Simulation[T]) AlphaDecay() float64 { return s.alphaDecay }

// SetAlphaDecay sets the decay rate and returns the receiver. The default is
// 1 − alphaMin^(1/300), which is the rate that reaches alphaMin in about 300
// ticks. Smaller values cool more slowly and generally find a better layout;
// zero means the simulation never converges on its own.
func (s *Simulation[T]) SetAlphaDecay(d float64) *Simulation[T] { s.alphaDecay = d; return s }

// AlphaTarget returns the value alpha decays toward, normally zero.
func (s *Simulation[T]) AlphaTarget() float64 { return s.alphaTarget }

// SetAlphaTarget sets the value alpha decays toward and returns the receiver.
//
// A target above alphaMin keeps the simulation permanently warm, which is what
// upstream uses to hold a layout live while a node is being dragged. With
// explicit stepping there is no timer to keep warm, so this mostly matters as a
// way to stop a layout from freezing between interactive updates — and it means
// [Simulation.RunUntilStable] can never converge, which that method documents.
func (s *Simulation[T]) SetAlphaTarget(t float64) *Simulation[T] { s.alphaTarget = t; return s }

// VelocityDecay returns the per-tick velocity damping, the simulation's
// friction.
func (s *Simulation[T]) VelocityDecay() float64 { return 1 - s.velocityDecay }

// SetVelocityDecay sets the friction and returns the receiver. The argument is
// the fraction of velocity *lost* per tick, so 0 is frictionless and 1 stops
// everything dead; the default is 0.4. Too low and the layout oscillates,
// too high and it gets stuck in a local minimum.
func (s *Simulation[T]) SetVelocityDecay(d float64) *Simulation[T] {
	s.velocityDecay = 1 - d
	return s
}

// Random returns the simulation's random source.
func (s *Simulation[T]) Random() *random.Rand { return s.rnd }

// SetRandom replaces the random source and returns the receiver, re-initializing
// every force so they pick it up.
//
// The default is random.Source(1), which reproduces d3-force's own default LCG
// exactly — same multiplier, same increment, same initial state. Pass a
// different [random.Source] seed to get a different but equally reproducible
// run; see the package doc on determinism.
func (s *Simulation[T]) SetRandom(r *random.Rand) *Simulation[T] {
	s.rnd = r
	for _, nf := range s.forces {
		nf.force.Initialize(s.nodes, s.rnd)
	}
	return s
}

// Converged reports whether alpha has fallen below alphaMin, which is when
// [Simulation.Tick] stops doing anything.
func (s *Simulation[T]) Converged() bool { return s.alpha < s.alphaMin }

// Stop halts the simulation and returns the receiver: [Simulation.Tick] becomes
// a no-op until [Simulation.Restart].
//
// Upstream this cancels a timer. Here there is no timer, so it is a flag — kept
// because code ported from d3 calls it, and because "this layout is finished,
// do not let anything move it" is worth being able to say.
func (s *Simulation[T]) Stop() *Simulation[T] { s.stopped = true; return s }

// Restart reheats the simulation and returns the receiver: alpha goes back to 1
// and a previous [Simulation.Stop] is cleared.
//
// This differs from upstream, where restart only resumes the timer and the idiom
// is `simulation.alpha(1).restart()`. With no timer to resume, a restart that
// did not reheat would do nothing at all, so it reheats. Call
// [Simulation.SetAlpha] afterwards for a gentler nudge.
func (s *Simulation[T]) Restart() *Simulation[T] {
	s.stopped = false
	s.alpha = defaultAlpha
	return s
}

// Tick advances the simulation one step and returns the receiver.
//
// The step is: decay alpha toward its target, let every force adjust
// velocities in registration order, then integrate — damp each velocity by the
// friction and add it to the position. A pinned node has its coordinate
// reasserted and its velocity zeroed instead.
//
// Tick is a no-op once the simulation has converged ([Simulation.Converged]) or
// been stopped. Reheat with [Simulation.Restart] or [Simulation.SetAlpha].
func (s *Simulation[T]) Tick() *Simulation[T] {
	if s.stopped || s.Converged() {
		return s
	}
	s.alpha += (s.alphaTarget - s.alpha) * s.alphaDecay

	for _, nf := range s.forces {
		nf.force.Apply(s.alpha)
	}

	for _, n := range s.nodes {
		if n.FX == nil {
			n.VX *= s.velocityDecay
			n.X += n.VX
		} else {
			n.X = *n.FX
			n.VX = 0
		}
		if n.FY == nil {
			n.VY *= s.velocityDecay
			n.Y += n.VY
		} else {
			n.Y = *n.FY
			n.VY = 0
		}
	}
	return s
}

// TickN advances the simulation n steps, stopping early if it converges, and
// returns the number of steps actually taken.
func (s *Simulation[T]) TickN(n int) int {
	for i := 0; i < n; i++ {
		if s.stopped || s.Converged() {
			return i
		}
		s.Tick()
	}
	return n
}

// RunUntilStable ticks until the simulation converges and returns how many ticks
// that took. This is the whole point of an explicitly stepped simulation: one
// call, and the layout in the nodes is final.
//
// Convergence is alpha falling below alphaMin. Two configurations prevent it,
// and both are capped rather than allowed to spin forever: an alpha target at or
// above alphaMin (alpha decays toward the target, so it never gets past it) and
// an alpha decay of zero. In either case this returns after the number of ticks
// a default configuration would have needed, which is a bounded amount of work
// and not a hang.
func (s *Simulation[T]) RunUntilStable() int {
	max := s.maxTicks()
	n := 0
	for n < max && !s.stopped && !s.Converged() {
		s.Tick()
		n++
	}
	return n
}

// maxTicks is the analytic number of ticks needed to decay from the current
// alpha to alphaMin, ignoring the alpha target, plus slack. It exists purely as
// the loop bound that keeps RunUntilStable total.
func (s *Simulation[T]) maxTicks() int {
	const fallback = 300
	if s.alphaDecay <= 0 || s.alphaDecay >= 1 || s.alphaMin <= 0 || s.alpha <= 0 {
		return fallback
	}
	n := math.Log(s.alphaMin/s.alpha) / math.Log(1-s.alphaDecay)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return fallback
	}
	return int(math.Ceil(n)) + 2
}

// Find returns the node closest to (x, y), and whether there was one.
//
// This is a linear scan, as upstream: the nodes move every tick, so a spatial
// index built for one query would be stale by the next and would cost more to
// build than the scan costs to run.
func (s *Simulation[T]) Find(x, y float64) (*Node[T], bool) {
	return s.findWithin(x, y, math.Inf(1))
}

// FindWithin returns the node closest to (x, y) within radius, and false if
// there is none that close.
func (s *Simulation[T]) FindWithin(x, y, radius float64) (*Node[T], bool) {
	return s.findWithin(x, y, radius)
}

func (s *Simulation[T]) findWithin(x, y, radius float64) (*Node[T], bool) {
	best := radius * radius
	var found *Node[T]
	for _, n := range s.nodes {
		dx, dy := x-n.X, y-n.Y
		if d2 := dx*dx + dy*dy; d2 < best {
			best, found = d2, n
		}
	}
	return found, found != nil
}

// jiggle is the 1e-6 nudge applied when two points are exactly coincident, so
// that a zero distance becomes a tiny arbitrary direction instead of a division
// by zero. It is the only place randomness enters the simulation.
func jiggle(r *random.Rand) float64 {
	return (r.Float64() - 0.5) * 1e-6
}
