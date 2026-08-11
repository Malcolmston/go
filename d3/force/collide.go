package force

import (
	"math"

	"github.com/malcolmston/d3/quadtree"
	"github.com/malcolmston/d3/random"
)

// Collide treats each node as a circle and pushes overlapping circles apart. It
// is what stops labels from sitting on top of each other and what turns a
// many-body layout into a packed one.
//
//	force.NewCollide[T]().SetRadiusFunc(func(n *force.Node[T]) float64 {
//		return 4 + n.Data.Size
//	})
//
// Like [ManyBody] it uses a quadtree, but for a different reason and with a
// different trick. Each tick it stores in every internal node the largest radius
// anywhere below it — via [quadtree.Quadtree.VisitAfter], into the same scratch
// slot the tree declares for the purpose — and then, for each node, prunes any
// quadrant whose square is further away than that maximum radius plus the node's
// own. Nothing inside such a quadrant can possibly be overlapping, so the whole
// subtree is skipped. Unlike Barnes–Hut this is exact: pruning here discards
// only pairs that provably do not touch.
//
// The resolution is a soft one. Each overlapping pair is pushed apart by
// strength times the overlap, split between the two in inverse proportion to
// their areas so a small circle gives way to a large one. One pass therefore
// leaves some overlap behind — later pairs undo what earlier pairs fixed — and
// [Collide.SetIterations] is how to trade time for a tighter packing.
//
// Collision acts on the *projected* positions, position plus pending velocity,
// so it resolves where the nodes are about to be rather than where they were.
//
// One deliberate difference from upstream: when several nodes are at exactly the
// same coordinates the quadtree stacks them in one leaf, and d3's collide looks
// only at the first of them. This port walks the whole chain, so a pile of
// coincident nodes is pushed apart rather than left overlapping. It costs
// nothing when there are no coincident nodes, which is the normal case.
type Collide[T any] struct {
	nodes  []*Node[T]
	rnd    *random.Rand
	radius func(*Node[T]) float64
	radii  []float64

	strength   float64
	iterations int
}

// NewCollide returns a collision force with radius 1, strength 0.7 and one
// iteration.
func NewCollide[T any]() *Collide[T] {
	return &Collide[T]{
		radius:     func(*Node[T]) float64 { return 1 },
		strength:   0.7,
		iterations: 1,
	}
}

// SetRadius sets a constant collision radius and returns the receiver.
func (f *Collide[T]) SetRadius(r float64) *Collide[T] {
	return f.SetRadiusFunc(func(*Node[T]) float64 { return r })
}

// SetRadiusFunc sets a per-node collision radius and returns the receiver. It is
// evaluated once per node at initialization, not once per tick.
func (f *Collide[T]) SetRadiusFunc(fn func(*Node[T]) float64) *Collide[T] {
	f.radius = fn
	f.init()
	return f
}

// SetStrength sets how much of each overlap is corrected per pass and returns
// the receiver. The default is 0.7; 1 resolves overlaps in one step but
// overshoots and jitters, which is why the default is not 1.
func (f *Collide[T]) SetStrength(s float64) *Collide[T] { f.strength = s; return f }

// SetIterations sets how many resolution passes run per tick and returns the
// receiver. The default is 1. Raise it when circles must not overlap at all.
func (f *Collide[T]) SetIterations(n int) *Collide[T] { f.iterations = n; return f }

// Initialize implements [Force].
func (f *Collide[T]) Initialize(nodes []*Node[T], rnd *random.Rand) {
	f.nodes = nodes
	f.rnd = rnd
	f.init()
}

func (f *Collide[T]) init() {
	f.radii = make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		f.radii[i] = f.radius(n)
	}
}

// Apply implements [Force].
//
// Note that it ignores alpha: a collision is a hard geometric fact, and letting
// it cool with the rest of the simulation would leave circles overlapping in the
// final frame — which is the one thing this force exists to prevent.
func (f *Collide[T]) Apply(float64) {
	if len(f.nodes) == 0 {
		return
	}
	for k := 0; k < f.iterations; k++ {
		tree := quadtree.New(
			func(n *Node[T]) float64 { return n.X + n.VX },
			func(n *Node[T]) float64 { return n.Y + n.VY },
		).AddAll(f.nodes)

		// Push the largest radius in each subtree up into the tree, which is
		// what makes the pruning below sound.
		tree.VisitAfter(func(q *quadtree.Node[*Node[T]], _, _, _, _ float64) {
			if q.IsLeaf() {
				q.R = f.radii[q.Data.Index]
				for l := q.Next; l != nil; l = l.Next {
					if r := f.radii[l.Data.Index]; r > q.R {
						q.R = r
					}
				}
				return
			}
			q.R = 0
			for _, c := range q.Children {
				if c != nil && c.R > q.R {
					q.R = c.R
				}
			}
		})

		for _, node := range f.nodes {
			ri := f.radii[node.Index]
			ri2 := ri * ri
			xi := node.X + node.VX
			yi := node.Y + node.VY

			tree.Visit(func(q *quadtree.Node[*Node[T]], x0, y0, x1, y1 float64) bool {
				if !q.IsLeaf() {
					// Prune: nothing in this square can reach xi,yi.
					r := ri + q.R
					return x0 > xi+r || x1 < xi-r || y0 > yi+r || y1 < yi-r
				}
				for l := q; l != nil; l = l.Next {
					data := l.Data
					// Each unordered pair is resolved once, by the node with
					// the lower index; without this every collision would be
					// applied twice.
					if data.Index <= node.Index {
						continue
					}
					rj := f.radii[data.Index]
					r := ri + rj
					x := xi - data.X - data.VX
					y := yi - data.Y - data.VY
					l2 := x*x + y*y
					if l2 >= r*r {
						continue
					}
					if x == 0 {
						x = jiggle(f.rnd)
						l2 += x * x
					}
					if y == 0 {
						y = jiggle(f.rnd)
						l2 += y * y
					}
					d := math.Sqrt(l2)
					k := (r - d) / d * f.strength
					x *= k
					y *= k
					// Split the correction by area, so the bigger circle moves
					// less.
					rj2 := rj * rj
					share := rj2 / (ri2 + rj2)
					node.VX += x * share
					node.VY += y * share
					data.VX -= x * (1 - share)
					data.VY -= y * (1 - share)
				}
				return false
			})
		}
	}
}
