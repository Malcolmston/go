package force

import (
	"math"

	"github.com/malcolmston/d3/quadtree"
	"github.com/malcolmston/d3/random"
)

// ManyBody is the n-body force: every node attracts or repels every other. A
// negative strength repels, which is what spreads a graph out, and a positive
// one attracts, which clusters it. The default is −30, i.e. repulsion.
//
// A literal n-body force is O(n²) per tick. This one is not: it builds a
// quadtree over the nodes each tick, sums the total charge and the centre of
// charge of each subtree into that tree with a single
// [quadtree.Quadtree.VisitAfter] pass, and then — the Barnes–Hut approximation —
// treats a whole distant subtree as one point at its centre of charge instead of
// visiting the nodes inside it. Concretely, a quadrant of width w seen from
// distance d is collapsed when w/d < theta. That makes a tick O(n log n).
//
// The approximation is the only place this force is inexact, and [ManyBody.SetTheta]
// is the dial. It is a real trade, not a rounding error: theta = 0 disables the
// approximation entirely and computes the exact O(n²) force.
//
//	force.NewManyBody[T]().SetStrength(-50).SetDistanceMax(300)
//
// [ManyBody.SetDistanceMin] and [ManyBody.SetDistanceMax] bound the interaction.
// The minimum stops two nearly coincident nodes from firing each other across
// the screen; the maximum is a performance and stability lever that also makes
// disconnected components drift apart rather than repel forever.
type ManyBody[T any] struct {
	nodes     []*Node[T]
	rnd       *random.Rand
	strength  func(*Node[T]) float64
	strengths []float64

	theta2       float64
	distanceMin2 float64
	distanceMax2 float64
}

// NewManyBody returns an n-body force with d3's defaults: strength −30, theta
// 0.9, minimum distance 1 and no maximum.
func NewManyBody[T any]() *ManyBody[T] {
	return &ManyBody[T]{
		strength:     func(*Node[T]) float64 { return -30 },
		theta2:       0.81,
		distanceMin2: 1,
		distanceMax2: math.Inf(1),
	}
}

// SetStrength sets a constant charge for every node and returns the receiver.
// Negative repels, positive attracts.
func (f *ManyBody[T]) SetStrength(v float64) *ManyBody[T] {
	return f.SetStrengthFunc(func(*Node[T]) float64 { return v })
}

// SetStrengthFunc sets a per-node charge accessor and returns the receiver. It
// is evaluated once per node when the force is initialized, not once per tick,
// so it may be expensive. Upstream's (node, i, nodes) signature collapses to one
// argument here because the index is already on the node as [Node.Index].
func (f *ManyBody[T]) SetStrengthFunc(fn func(*Node[T]) float64) *ManyBody[T] {
	f.strength = fn
	f.initStrengths()
	return f
}

// SetTheta sets the Barnes–Hut accuracy parameter and returns the receiver.
//
// A subtree is approximated by its centre of charge when its width divided by
// its distance is below theta. Larger is faster and coarser; smaller is slower
// and more exact; 0 turns the approximation off and makes the force exact and
// quadratic. The default is 0.9.
func (f *ManyBody[T]) SetTheta(theta float64) *ManyBody[T] {
	f.theta2 = theta * theta
	return f
}

// SetDistanceMin sets the minimum interaction distance and returns the receiver.
//
// Below this distance the force stops growing. Without it the inverse-square law
// sends two coincident nodes to infinity, so this is a stability guard rather
// than a tuning knob; the default of 1 is usually right.
func (f *ManyBody[T]) SetDistanceMin(d float64) *ManyBody[T] {
	f.distanceMin2 = d * d
	return f
}

// SetDistanceMax sets the maximum interaction distance and returns the receiver.
// Beyond it nodes ignore each other entirely. The default is infinite.
func (f *ManyBody[T]) SetDistanceMax(d float64) *ManyBody[T] {
	f.distanceMax2 = d * d
	return f
}

// Initialize implements [Force].
func (f *ManyBody[T]) Initialize(nodes []*Node[T], rnd *random.Rand) {
	f.nodes = nodes
	f.rnd = rnd
	f.initStrengths()
}

func (f *ManyBody[T]) initStrengths() {
	f.strengths = make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		f.strengths[i] = f.strength(n)
	}
}

// Apply implements [Force].
func (f *ManyBody[T]) Apply(alpha float64) {
	if len(f.nodes) == 0 {
		return
	}
	tree := quadtree.New(
		func(n *Node[T]) float64 { return n.X },
		func(n *Node[T]) float64 { return n.Y },
	).AddAll(f.nodes)

	tree.VisitAfter(f.accumulate)

	for _, node := range f.nodes {
		tree.Visit(f.applyTo(node, alpha))
	}
}

// accumulate sums each subtree's total charge into Value and its charge-weighted
// centre into X and Y — the scratch fields d3/quadtree declares for exactly this.
// Running bottom-up means a node's children are always already summed.
func (f *ManyBody[T]) accumulate(q *quadtree.Node[*Node[T]], _, _, _, _ float64) {
	if !q.IsLeaf() {
		var strength, weight, x, y float64
		for _, c := range q.Children {
			if c == nil {
				continue
			}
			// Weighting by |value| rather than value keeps the centre
			// meaningful when a subtree mixes attraction and repulsion.
			cw := math.Abs(c.Value)
			if cw == 0 {
				continue
			}
			strength += c.Value
			weight += cw
			x += cw * c.X
			y += cw * c.Y
		}
		if weight != 0 {
			q.X, q.Y = x/weight, y/weight
		} else {
			// Every child had zero charge, so there is no centre to speak of.
			// Value is zero too, and the apply pass short-circuits on that
			// before ever reading X or Y.
			q.X, q.Y = 0, 0
		}
		q.Value = strength
		return
	}
	q.X, q.Y = q.Data.X, q.Data.Y
	var strength float64
	for l := q; l != nil; l = l.Next {
		strength += f.strengths[l.Data.Index]
	}
	q.Value = strength
}

// applyTo returns the visitor that accumulates the force on one node. It returns
// true — prune — whenever the subtree it was handed has been dealt with, either
// because it is empty of charge or because the Barnes–Hut test let it be treated
// as a single point.
func (f *ManyBody[T]) applyTo(node *Node[T], alpha float64) quadtree.VisitFunc[*Node[T]] {
	return func(q *quadtree.Node[*Node[T]], x1, _, x2, _ float64) bool {
		if q.Value == 0 {
			return true
		}
		x := q.X - node.X
		y := q.Y - node.Y
		w := x2 - x1
		l := x*x + y*y

		// The Barnes-Hut test: w/sqrt(l) < theta, squared to avoid the root.
		if w*w/f.theta2 < l {
			if l < f.distanceMax2 {
				if x == 0 {
					x = jiggle(f.rnd)
					l += x * x
				}
				if y == 0 {
					y = jiggle(f.rnd)
					l += y * y
				}
				if l < f.distanceMin2 {
					l = math.Sqrt(f.distanceMin2 * l)
				}
				node.VX += x * q.Value * alpha / l
				node.VY += y * q.Value * alpha / l
			}
			return true
		}

		// Too close to approximate: descend, unless this is a leaf we can
		// handle directly or one that is out of range entirely.
		if !q.IsLeaf() || l >= f.distanceMax2 {
			return false
		}

		// A leaf. Guard against the zero distance to self only when there is
		// somebody else here — a node does not push on itself.
		if q.Data != node || q.Next != nil {
			if x == 0 {
				x = jiggle(f.rnd)
				l += x * x
			}
			if y == 0 {
				y = jiggle(f.rnd)
				l += y * y
			}
			if l < f.distanceMin2 {
				l = math.Sqrt(f.distanceMin2 * l)
			}
		}
		for c := q; c != nil; c = c.Next {
			if c.Data == node {
				continue
			}
			k := f.strengths[c.Data.Index] * alpha / l
			node.VX += x * k
			node.VY += y * k
		}
		return false
	}
}
