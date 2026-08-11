package force

import (
	"math"

	"github.com/malcolmston/d3/random"
)

// Link is one edge of the graph: two nodes that a [LinkForce] pulls toward a
// target distance.
//
// Value is not used by this package. It is there for a strength or distance
// accessor to read, which is the usual way to say "this edge is heavier than
// that one" without a parallel array.
//
// Upstream lets a link name its endpoints by id and resolves them against the
// node set. Here they are *[Node] directly: resolving ids would mean either a
// constraint on the datum type or a reflective id accessor, and the caller who
// built the graph already has the nodes in hand.
type Link[T any] struct {
	// Source and Target are the endpoints. Both must be nodes of the
	// simulation the force is attached to.
	Source, Target *Node[T]

	// Value is the caller's own edge weight, free for accessors to read.
	Value float64

	// Index is the link's position in the slice, assigned when the force is
	// initialized.
	Index int
}

// LinkForce pulls linked nodes toward a fixed distance from each other, like a
// spring on every edge. It is the force that makes a force-directed graph a
// graph rather than a cloud of points.
//
//	links := []*force.Link[T]{{Source: a, Target: b}}
//	sim.SetForce("link", force.NewLink(links).SetDistance(60))
//
// # Default strength, and why it is not 1
//
// A naive spring force gives every edge the same strength, and the result is
// that a node with fifty edges is yanked around by all of them while a node with
// one barely moves — the layout becomes unstable exactly where the graph is
// densest. So the default strength of a link is 1/min(deg(source), deg(target)):
// the more edges compete for an endpoint, the weaker each of them pulls.
//
// The same reasoning splits the correction unevenly between the two endpoints.
// Each link's bias is deg(source)/(deg(source)+deg(target)), and the target
// absorbs that fraction of the displacement while the source absorbs the rest —
// so a leaf hanging off a hub moves and the hub mostly does not, which is both
// more stable and what the picture should look like.
//
// [LinkForce.SetIterations] runs the whole relaxation more than once per tick.
// Since the springs are resolved in sequence, one pass leaves the last edges
// better satisfied than the first; more passes converge toward satisfying all of
// them at once, which matters for rigid structures like grids and lattices.
type LinkForce[T any] struct {
	links []*Link[T]
	nodes map[*Node[T]]bool
	rnd   *random.Rand

	strength func(*Link[T]) float64
	distance func(*Link[T]) float64

	strengths []float64
	distances []float64
	bias      []float64

	// strengthSet records whether the caller overrode the degree-based default,
	// which cannot be decided until the degrees are known.
	strengthSet bool

	iterations int
}

// NewLink returns a link force over the given edges, with distance 30, one
// iteration and the degree-based default strength described on [LinkForce].
//
// The slice is retained, not copied; edges may be mutated between ticks, but the
// force must be re-registered (or the simulation's nodes reset) for a change in
// the endpoints to be picked up, since the degree counts are precomputed.
func NewLink[T any](links []*Link[T]) *LinkForce[T] {
	return &LinkForce[T]{
		links:      links,
		distance:   func(*Link[T]) float64 { return 30 },
		iterations: 1,
	}
}

// Links returns the edges this force acts on.
func (f *LinkForce[T]) Links() []*Link[T] { return f.links }

// SetLinks replaces the edges and returns the receiver, recomputing the degree
// counts and the per-link constants.
func (f *LinkForce[T]) SetLinks(links []*Link[T]) *LinkForce[T] {
	f.links = links
	f.init()
	return f
}

// SetDistance sets a constant target length for every edge and returns the
// receiver. This is a target, not a constraint: the springs are soft and other
// forces pull against them.
func (f *LinkForce[T]) SetDistance(d float64) *LinkForce[T] {
	return f.SetDistanceFunc(func(*Link[T]) float64 { return d })
}

// SetDistanceFunc sets a per-edge target length and returns the receiver. It is
// evaluated once per edge at initialization, not once per tick.
func (f *LinkForce[T]) SetDistanceFunc(fn func(*Link[T]) float64) *LinkForce[T] {
	f.distance = fn
	f.init()
	return f
}

// SetStrength sets a constant spring stiffness for every edge and returns the
// receiver, replacing the degree-based default. Values run from 0 (no spring) to
// 1 (rigid); anything above 1 overshoots and oscillates.
func (f *LinkForce[T]) SetStrength(s float64) *LinkForce[T] {
	return f.SetStrengthFunc(func(*Link[T]) float64 { return s })
}

// SetStrengthFunc sets a per-edge stiffness and returns the receiver, replacing
// the degree-based default. It is evaluated once per edge at initialization.
func (f *LinkForce[T]) SetStrengthFunc(fn func(*Link[T]) float64) *LinkForce[T] {
	f.strength = fn
	f.strengthSet = true
	f.init()
	return f
}

// SetIterations sets how many relaxation passes run per tick and returns the
// receiver. The default is 1. Raise it for rigid structures; see [LinkForce].
func (f *LinkForce[T]) SetIterations(n int) *LinkForce[T] {
	f.iterations = n
	return f
}

// Initialize implements [Force].
//
// It panics if a link references a node that is not in the simulation, or a nil
// one. That is a programmer error — the per-node arrays are indexed by
// [Node.Index], so a foreign node would silently corrupt another node's
// bookkeeping rather than merely produce a wrong picture.
func (f *LinkForce[T]) Initialize(nodes []*Node[T], rnd *random.Rand) {
	f.rnd = rnd
	f.nodes = make(map[*Node[T]]bool, len(nodes))
	for _, n := range nodes {
		f.nodes[n] = true
	}
	f.init()
}

func (f *LinkForce[T]) init() {
	n := len(f.links)
	f.strengths = make([]float64, n)
	f.distances = make([]float64, n)
	f.bias = make([]float64, n)
	if n == 0 {
		return
	}

	count := make(map[*Node[T]]float64, 2*n)
	for i, l := range f.links {
		l.Index = i
		if f.nodes != nil {
			if l.Source == nil || l.Target == nil {
				panic("force: Link has a nil endpoint")
			}
			if !f.nodes[l.Source] || !f.nodes[l.Target] {
				panic("force: Link references a node that is not in the simulation")
			}
		}
		count[l.Source]++
		count[l.Target]++
	}

	for i, l := range f.links {
		cs, ct := count[l.Source], count[l.Target]
		f.bias[i] = cs / (cs + ct)
		if f.strengthSet {
			f.strengths[i] = f.strength(l)
		} else {
			f.strengths[i] = 1 / math.Min(cs, ct)
		}
		f.distances[i] = f.distance(l)
	}
}

// Apply implements [Force].
func (f *LinkForce[T]) Apply(alpha float64) {
	for k := 0; k < f.iterations; k++ {
		for i, l := range f.links {
			source, target := l.Source, l.Target

			// Positions are read one integration step ahead — position plus
			// pending velocity — so that a spring reacts to where the nodes are
			// about to be rather than where they were. That is what lets
			// several iterations converge instead of ringing.
			x := target.X + target.VX - source.X - source.VX
			if x == 0 {
				x = jiggle(f.rnd)
			}
			y := target.Y + target.VY - source.Y - source.VY
			if y == 0 {
				y = jiggle(f.rnd)
			}
			d := math.Sqrt(x*x + y*y)
			k := (d - f.distances[i]) / d * alpha * f.strengths[i]
			x *= k
			y *= k

			b := f.bias[i]
			target.VX -= x * b
			target.VY -= y * b
			source.VX += x * (1 - b)
			source.VY += y * (1 - b)
		}
	}
}
