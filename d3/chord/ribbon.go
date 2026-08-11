package chord

import (
	"math"

	"github.com/malcolmston/d3/path"
)

// SubgroupAccessor computes one property of a chord end.
type SubgroupAccessor func(s Subgroup) float64

// ChordAccessor computes one property of a whole chord.
type ChordAccessor func(d ChordDatum) float64

// Ribbon generates the SVG path for one chord: the shape that leaves one
// group's arc, narrows through the centre of the circle and widens again onto
// the other group's arc.
//
// The geometry is four pieces — an arc along the source group, a quadratic
// Bézier whose single control point is the *origin* (which is what pulls every
// ribbon toward the centre and makes the bundle read as a bundle), an arc
// along the target group, and a second Bézier back. The path is centred on the
// origin, so position it with a transform on a parent group, exactly as with
// [github.com/malcolmston/d3/shape.Arc].
//
//	ribbon := chord.NewRibbon().Radius(240).PadAngle(0.02)
//	for _, c := range layout.Generate(matrix).Chords {
//		react.H("path", react.Props{"d": ribbon.Generate(c), "fill": fill(c.Source.Index)})
//	}
//
// Construct with [NewRibbon] or [NewRibbonArrow]; the zero value is not
// usable. A configured Ribbon is a pure function from a [ChordDatum] to a
// string and is safe to call from any goroutine.
//
// # Radius has no default
//
// Upstream's default radius accessor reads a `radius` property off the chord
// end, which the chord layout never sets — so in practice every d3 chord
// diagram calls .radius(r). This port makes that explicit: the default radius
// is 0, which draws a degenerate path. Set [Ribbon.Radius] to the inner radius
// of your group arcs.
type Ribbon struct {
	source, target       func(d ChordDatum) Subgroup
	sourceRadius         SubgroupAccessor
	targetRadius         SubgroupAccessor
	startAngle, endAngle SubgroupAccessor
	padAngle             ChordAccessor
	headRadius           ChordAccessor // nil for a plain ribbon
	precision            int
}

func constSub(v float64) SubgroupAccessor { return func(Subgroup) float64 { return v } }
func constChord(v float64) ChordAccessor  { return func(ChordDatum) float64 { return v } }

func newRibbon() *Ribbon {
	return &Ribbon{
		source:       func(d ChordDatum) Subgroup { return d.Source },
		target:       func(d ChordDatum) Subgroup { return d.Target },
		sourceRadius: constSub(0),
		targetRadius: constSub(0),
		startAngle:   func(s Subgroup) float64 { return s.StartAngle },
		endAngle:     func(s Subgroup) float64 { return s.EndAngle },
		padAngle:     constChord(0),
		precision:    -1,
	}
}

// NewRibbon returns a plain ribbon: both ends are arcs, and the two arcs are
// joined by curves through the centre.
func NewRibbon() *Ribbon { return newRibbon() }

// NewRibbonArrow returns a ribbon whose target end is a triangular arrowhead
// rather than an arc, with a head radius of 10.
//
// It only says something true when the layout distinguishes source from
// target, so pair it with [NewDirected].
func NewRibbonArrow() *Ribbon {
	r := newRibbon()
	r.headRadius = constChord(10)
	return r
}

// Source sets the accessor for the chord's source end. The default reads
// [ChordDatum.Source].
func (r *Ribbon) Source(f func(d ChordDatum) Subgroup) *Ribbon { r.source = f; return r }

// Target sets the accessor for the chord's target end. The default reads
// [ChordDatum.Target].
func (r *Ribbon) Target(f func(d ChordDatum) Subgroup) *Ribbon { r.target = f; return r }

// Radius sets a constant radius for both ends — the usual case, and normally
// the inner radius of the group arcs so the ribbons meet them flush.
func (r *Ribbon) Radius(v float64) *Ribbon {
	r.sourceRadius, r.targetRadius = constSub(v), constSub(v)
	return r
}

// RadiusFunc sets a per-end radius for both ends.
func (r *Ribbon) RadiusFunc(f SubgroupAccessor) *Ribbon {
	r.sourceRadius, r.targetRadius = f, f
	return r
}

// SourceRadius sets a constant radius for the source end only.
func (r *Ribbon) SourceRadius(v float64) *Ribbon { r.sourceRadius = constSub(v); return r }

// SourceRadiusFunc sets a per-end radius for the source end only.
func (r *Ribbon) SourceRadiusFunc(f SubgroupAccessor) *Ribbon { r.sourceRadius = f; return r }

// TargetRadius sets a constant radius for the target end only.
func (r *Ribbon) TargetRadius(v float64) *Ribbon { r.targetRadius = constSub(v); return r }

// TargetRadiusFunc sets a per-end radius for the target end only.
func (r *Ribbon) TargetRadiusFunc(f SubgroupAccessor) *Ribbon { r.targetRadius = f; return r }

// StartAngle sets a constant start angle in radians for both ends.
func (r *Ribbon) StartAngle(v float64) *Ribbon { r.startAngle = constSub(v); return r }

// StartAngleFunc sets a per-end start angle. The default reads
// [Subgroup.StartAngle], which is what the layout fills in.
func (r *Ribbon) StartAngleFunc(f SubgroupAccessor) *Ribbon { r.startAngle = f; return r }

// EndAngle sets a constant end angle in radians for both ends.
func (r *Ribbon) EndAngle(v float64) *Ribbon { r.endAngle = constSub(v); return r }

// EndAngleFunc sets a per-end end angle.
func (r *Ribbon) EndAngleFunc(f SubgroupAccessor) *Ribbon { r.endAngle = f; return r }

// PadAngle sets a constant angular gap, in radians, taken out of *each* end of
// the ribbon so that adjacent ribbons landing on the same group arc are
// visibly separate. Half of it is removed from each side.
//
// When an end is narrower than the padding it would lose, it collapses to a
// point at its midpoint rather than inverting.
func (r *Ribbon) PadAngle(v float64) *Ribbon { r.padAngle = constChord(v); return r }

// PadAngleFunc sets a per-chord pad angle.
func (r *Ribbon) PadAngleFunc(f ChordAccessor) *Ribbon { r.padAngle = f; return r }

// HeadRadius sets a constant arrowhead depth: the distance by which the two
// corners of the target end are pulled in from the radius, leaving the point
// at the radius itself.
//
// Setting it on a plain [NewRibbon] turns that ribbon into an arrow. Upstream
// exposes the setter only on ribbonArrow, which is the same statement made
// with two constructors instead of one flag.
func (r *Ribbon) HeadRadius(v float64) *Ribbon { r.headRadius = constChord(v); return r }

// HeadRadiusFunc sets a per-chord arrowhead depth.
func (r *Ribbon) HeadRadiusFunc(f ChordAccessor) *Ribbon { r.headRadius = f; return r }

// Digits rounds every emitted coordinate to n decimal places, the equivalent
// of d3.path(digits). A negative n restores full precision.
func (r *Ribbon) Digits(n int) *Ribbon { r.precision = n; return r }

// Generate returns the SVG path data for one chord, or "" if nothing was
// drawn. Angles are interpreted clockwise from twelve o'clock, matching the
// layout and d3/shape.
func (r *Ribbon) Generate(d ChordDatum) string {
	p := path.New()
	if r.precision >= 0 {
		p = path.NewWithPrecision(r.precision)
	}

	s := r.source(d)
	t := r.target(d)
	ap := r.padAngle(d) / 2

	sr := r.sourceRadius(s)
	// The half-turn offset converts "clockwise from twelve o'clock", which is
	// how every angle in this port is expressed, into the "counter-clockwise
	// from three o'clock" that trigonometry and path.Arc use.
	sa0 := r.startAngle(s) - halfPi
	sa1 := r.endAngle(s) - halfPi

	tr := r.targetRadius(t)
	ta0 := r.startAngle(t) - halfPi
	ta1 := r.endAngle(t) - halfPi

	if ap > epsilon {
		sa0, sa1 = inset(sa0, sa1, ap)
		ta0, ta1 = inset(ta0, ta1, ap)
	}

	p.MoveTo(sr*math.Cos(sa0), sr*math.Sin(sa0))
	p.Arc(0, 0, sr, sa0, sa1, false)

	// When both ends are the same arc there is no ribbon to cross the circle,
	// only a wedge — drawing the target half would double back over itself.
	if sa0 != ta0 || sa1 != ta1 {
		if r.headRadius != nil {
			hr := r.headRadius(d)
			base := tr - hr
			tip := (ta0 + ta1) / 2
			p.QuadraticCurveTo(0, 0, base*math.Cos(ta0), base*math.Sin(ta0))
			p.LineTo(tr*math.Cos(tip), tr*math.Sin(tip))
			p.LineTo(base*math.Cos(ta1), base*math.Sin(ta1))
		} else {
			p.QuadraticCurveTo(0, 0, tr*math.Cos(ta0), tr*math.Sin(ta0))
			p.Arc(0, 0, tr, ta0, ta1, false)
		}
	}

	p.QuadraticCurveTo(0, 0, sr*math.Cos(sa0), sr*math.Sin(sa0))
	p.ClosePath()
	return p.String()
}

// inset shrinks the arc [a0, a1] by ap at each end, in whichever direction the
// arc runs. An arc too narrow to survive collapses to its midpoint instead of
// turning inside out.
func inset(a0, a1, ap float64) (float64, float64) {
	if math.Abs(a1-a0) > ap*2+epsilon {
		if a1 > a0 {
			return a0 + ap, a1 - ap
		}
		return a0 - ap, a1 + ap
	}
	mid := (a0 + a1) / 2
	return mid, mid
}
