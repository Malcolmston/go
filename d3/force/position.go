package force

import (
	"math"

	"github.com/malcolmston/d3/random"
)

// Center keeps the layout's centre of mass at a point.
//
// It is not a force in the same sense as the others: it does not touch
// velocities and it does not scale with alpha. It measures the centroid of the
// nodes and translates every one of them so the centroid lands on the target.
// The layout's shape is completely unaffected — only where it sits.
//
// That makes it the right tool for "keep the drawing on the canvas" and the
// wrong one for "pull everything together", which is [XForce] and [YForce].
// Because it translates rather than pushes, it can also fight a pinned node: the
// pin is reasserted at the end of the tick, so with one node pinned the centring
// translation is undone for that node and applied to everything else.
type Center[T any] struct {
	nodes    []*Node[T]
	x, y     float64
	strength float64
}

// NewCenter returns a centring force targeting (x, y) with strength 1 — full
// correction every tick.
func NewCenter[T any](x, y float64) *Center[T] {
	return &Center[T]{x: x, y: y, strength: 1}
}

// SetX sets the target x and returns the receiver.
func (f *Center[T]) SetX(x float64) *Center[T] { f.x = x; return f }

// SetY sets the target y and returns the receiver.
func (f *Center[T]) SetY(y float64) *Center[T] { f.y = y; return f }

// SetStrength sets the fraction of the centring error corrected each tick and
// returns the receiver. 1 snaps the centroid to the target immediately; smaller
// values ease it there, which looks better when the layout is being animated and
// makes no difference to where it ends up.
func (f *Center[T]) SetStrength(s float64) *Center[T] { f.strength = s; return f }

// Initialize implements [Force].
func (f *Center[T]) Initialize(nodes []*Node[T], _ *random.Rand) { f.nodes = nodes }

// Apply implements [Force].
func (f *Center[T]) Apply(float64) {
	n := len(f.nodes)
	if n == 0 || f.strength == 0 {
		return
	}
	var sx, sy float64
	for _, node := range f.nodes {
		sx += node.X
		sy += node.Y
	}
	sx = (sx/float64(n) - f.x) * f.strength
	sy = (sy/float64(n) - f.y) * f.strength
	for _, node := range f.nodes {
		node.X -= sx
		node.Y -= sy
	}
}

// XForce pulls every node toward a target x coordinate, leaving y alone.
//
// It is the one-dimensional positioning force, and the way to build layouts
// where an axis means something: give each node the x of its category or its
// date, and the graph arranges itself vertically within columns that the force
// keeps horizontally honest. Pair it with a [YForce] for a weak pull toward a
// point, which is a gentler alternative to [Center] because it does scale with
// alpha and does deform the layout.
type XForce[T any] struct {
	nodes     []*Node[T]
	x         func(*Node[T]) float64
	strength  func(*Node[T]) float64
	xs        []float64
	strengths []float64
}

// NewX returns a force pulling every node toward the given x, with strength 0.1.
func NewX[T any](x float64) *XForce[T] {
	return &XForce[T]{
		x:        func(*Node[T]) float64 { return x },
		strength: func(*Node[T]) float64 { return 0.1 },
	}
}

// SetX sets a constant target and returns the receiver.
func (f *XForce[T]) SetX(x float64) *XForce[T] {
	return f.SetXFunc(func(*Node[T]) float64 { return x })
}

// SetXFunc sets a per-node target and returns the receiver, evaluated once per
// node at initialization.
func (f *XForce[T]) SetXFunc(fn func(*Node[T]) float64) *XForce[T] {
	f.x = fn
	f.init()
	return f
}

// SetStrength sets a constant pull and returns the receiver. It is the fraction
// of the remaining distance covered per tick at alpha 1; values above about 1
// overshoot.
func (f *XForce[T]) SetStrength(s float64) *XForce[T] {
	return f.SetStrengthFunc(func(*Node[T]) float64 { return s })
}

// SetStrengthFunc sets a per-node pull and returns the receiver, evaluated once
// per node at initialization.
func (f *XForce[T]) SetStrengthFunc(fn func(*Node[T]) float64) *XForce[T] {
	f.strength = fn
	f.init()
	return f
}

// Initialize implements [Force].
func (f *XForce[T]) Initialize(nodes []*Node[T], _ *random.Rand) {
	f.nodes = nodes
	f.init()
}

func (f *XForce[T]) init() {
	f.xs = make([]float64, len(f.nodes))
	f.strengths = make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		f.xs[i] = f.x(n)
		f.strengths[i] = f.strength(n)
	}
}

// Apply implements [Force].
func (f *XForce[T]) Apply(alpha float64) {
	for i, n := range f.nodes {
		n.VX += (f.xs[i] - n.X) * f.strengths[i] * alpha
	}
}

// YForce pulls every node toward a target y coordinate, leaving x alone. It is
// [XForce] on the other axis; see there for what it is for.
type YForce[T any] struct {
	nodes     []*Node[T]
	y         func(*Node[T]) float64
	strength  func(*Node[T]) float64
	ys        []float64
	strengths []float64
}

// NewY returns a force pulling every node toward the given y, with strength 0.1.
func NewY[T any](y float64) *YForce[T] {
	return &YForce[T]{
		y:        func(*Node[T]) float64 { return y },
		strength: func(*Node[T]) float64 { return 0.1 },
	}
}

// SetY sets a constant target and returns the receiver.
func (f *YForce[T]) SetY(y float64) *YForce[T] {
	return f.SetYFunc(func(*Node[T]) float64 { return y })
}

// SetYFunc sets a per-node target and returns the receiver, evaluated once per
// node at initialization.
func (f *YForce[T]) SetYFunc(fn func(*Node[T]) float64) *YForce[T] {
	f.y = fn
	f.init()
	return f
}

// SetStrength sets a constant pull and returns the receiver.
func (f *YForce[T]) SetStrength(s float64) *YForce[T] {
	return f.SetStrengthFunc(func(*Node[T]) float64 { return s })
}

// SetStrengthFunc sets a per-node pull and returns the receiver.
func (f *YForce[T]) SetStrengthFunc(fn func(*Node[T]) float64) *YForce[T] {
	f.strength = fn
	f.init()
	return f
}

// Initialize implements [Force].
func (f *YForce[T]) Initialize(nodes []*Node[T], _ *random.Rand) {
	f.nodes = nodes
	f.init()
}

func (f *YForce[T]) init() {
	f.ys = make([]float64, len(f.nodes))
	f.strengths = make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		f.ys[i] = f.y(n)
		f.strengths[i] = f.strength(n)
	}
}

// Apply implements [Force].
func (f *YForce[T]) Apply(alpha float64) {
	for i, n := range f.nodes {
		n.VY += (f.ys[i] - n.Y) * f.strengths[i] * alpha
	}
}

// Radial pulls every node toward a circle of a given radius around a centre.
//
// It is [XForce] and [YForce] in polar coordinates: the pull is along the ray
// through the node, so a node's angle is left entirely to the other forces while
// its distance from the centre is governed here. Give each node a radius derived
// from its data and the layout arranges itself into concentric rings — the usual
// use, and one no combination of the Cartesian forces reproduces.
type Radial[T any] struct {
	nodes     []*Node[T]
	x, y      float64
	radius    func(*Node[T]) float64
	strength  func(*Node[T]) float64
	radii     []float64
	strengths []float64
}

// NewRadial returns a force pulling every node toward the circle of the given
// radius centred at (x, y), with strength 0.1.
func NewRadial[T any](radius, x, y float64) *Radial[T] {
	return &Radial[T]{
		x: x, y: y,
		radius:   func(*Node[T]) float64 { return radius },
		strength: func(*Node[T]) float64 { return 0.1 },
	}
}

// SetX sets the centre's x and returns the receiver.
func (f *Radial[T]) SetX(x float64) *Radial[T] { f.x = x; return f }

// SetY sets the centre's y and returns the receiver.
func (f *Radial[T]) SetY(y float64) *Radial[T] { f.y = y; return f }

// SetRadius sets a constant target radius and returns the receiver.
func (f *Radial[T]) SetRadius(r float64) *Radial[T] {
	return f.SetRadiusFunc(func(*Node[T]) float64 { return r })
}

// SetRadiusFunc sets a per-node target radius and returns the receiver,
// evaluated once per node at initialization.
func (f *Radial[T]) SetRadiusFunc(fn func(*Node[T]) float64) *Radial[T] {
	f.radius = fn
	f.init()
	return f
}

// SetStrength sets a constant pull and returns the receiver.
func (f *Radial[T]) SetStrength(s float64) *Radial[T] {
	return f.SetStrengthFunc(func(*Node[T]) float64 { return s })
}

// SetStrengthFunc sets a per-node pull and returns the receiver.
func (f *Radial[T]) SetStrengthFunc(fn func(*Node[T]) float64) *Radial[T] {
	f.strength = fn
	f.init()
	return f
}

// Initialize implements [Force].
func (f *Radial[T]) Initialize(nodes []*Node[T], _ *random.Rand) {
	f.nodes = nodes
	f.init()
}

func (f *Radial[T]) init() {
	f.radii = make([]float64, len(f.nodes))
	f.strengths = make([]float64, len(f.nodes))
	for i, n := range f.nodes {
		f.radii[i] = f.radius(n)
		f.strengths[i] = f.strength(n)
	}
}

// Apply implements [Force].
func (f *Radial[T]) Apply(alpha float64) {
	for i, n := range f.nodes {
		dx := n.X - f.x
		if dx == 0 {
			dx = 1e-6
		}
		dy := n.Y - f.y
		if dy == 0 {
			dy = 1e-6
		}
		r := math.Sqrt(dx*dx + dy*dy)
		k := (f.radii[i] - r) * f.strengths[i] * alpha / r
		n.VX += dx * k
		n.VY += dy * k
	}
}
