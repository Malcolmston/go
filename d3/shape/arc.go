package shape

import (
	"math"

	"github.com/malcolmston/d3/path"
)

// ArcDatum describes one wedge for [Arc]. It is what [Pie] produces, so the
// two compose directly: pie.Generate(data) yields []PieArc, and each
// PieArc.Arc() is the ArcDatum for arc.Generate.
//
// Angles are in radians, measured clockwise from twelve o'clock. InnerRadius
// and OuterRadius are read only by the default radius accessors; a chart that
// sets a constant radius on the generator can leave them zero.
type ArcDatum struct {
	StartAngle  float64
	EndAngle    float64
	PadAngle    float64
	InnerRadius float64
	OuterRadius float64
}

// ArcAccessor computes one property of an arc from its datum.
type ArcAccessor func(d ArcDatum) float64

func constArc(v float64) ArcAccessor { return func(ArcDatum) float64 { return v } }

// Arc generates the path for a circular or annular sector — the wedge of a pie
// chart, the segment of a donut, the bar of a radial histogram. It is the most
// configurable generator here because the visual polish of a donut chart lives
// entirely in two features: rounded corners and the padding between wedges.
//
//	arc := shape.NewArc().InnerRadius(60).OuterRadius(100).CornerRadius(4).PadAngle(0.02)
//	for _, a := range pie.Generate(data) {
//		react.H("path", react.Props{"d": arc.Generate(a.Arc()), "fill": color(a.Data)})
//	}
//
// The path is centered on the origin; position it with a transform on a parent
// group. Inner and outer radius are swapped if given the wrong way round, so
// an annulus is never inside out.
//
// # Corner radius and padding interact
//
// [Arc.CornerRadius] rounds all four corners of the sector, clamped to half
// the ring's thickness so opposite corners cannot cross. For a sector narrower
// than half a turn there is a second clamp: the corner circles must still fit
// inside the wedge, so their radius is reduced toward the apex and can reach
// zero. When both corners of an edge would merge, a single arc replaces the
// two corner arcs and the ring between them.
//
// [Arc.PadAngle] separates adjacent wedges. The gap is a constant *distance*,
// not a constant angle: the padding angle is applied at [Arc.PadRadius] (by
// default the quadratic mean of the two radii) and converted to a different
// angular inset at each radius, so the visual gap has parallel sides instead
// of fanning out. When padding would consume the whole wedge, the sector
// collapses to a line at its bisector rather than inverting.
type Arc struct {
	innerRadius, outerRadius ArcAccessor
	cornerRadius             ArcAccessor
	padRadius                ArcAccessor // nil means the quadratic mean of the radii
	startAngle, endAngle     ArcAccessor
	padAngle                 ArcAccessor
	digits
}

// NewArc returns an Arc whose radii and angles come from the [ArcDatum], with
// no corner rounding.
func NewArc() *Arc {
	return &Arc{
		innerRadius:  func(d ArcDatum) float64 { return d.InnerRadius },
		outerRadius:  func(d ArcDatum) float64 { return d.OuterRadius },
		cornerRadius: constArc(0),
		startAngle:   func(d ArcDatum) float64 { return d.StartAngle },
		endAngle:     func(d ArcDatum) float64 { return d.EndAngle },
		padAngle:     func(d ArcDatum) float64 { return d.PadAngle },
		digits:       noDigits(),
	}
}

// InnerRadius sets a constant inner radius; 0 makes a pie wedge rather than a
// donut segment.
func (a *Arc) InnerRadius(v float64) *Arc { a.innerRadius = constArc(v); return a }

// InnerRadiusFunc sets a per-datum inner radius.
func (a *Arc) InnerRadiusFunc(f ArcAccessor) *Arc { a.innerRadius = f; return a }

// OuterRadius sets a constant outer radius.
func (a *Arc) OuterRadius(v float64) *Arc { a.outerRadius = constArc(v); return a }

// OuterRadiusFunc sets a per-datum outer radius, which is how a radial bar
// chart encodes its value.
func (a *Arc) OuterRadiusFunc(f ArcAccessor) *Arc { a.outerRadius = f; return a }

// CornerRadius sets a constant corner radius. It is clamped as described on
// [Arc].
func (a *Arc) CornerRadius(v float64) *Arc { a.cornerRadius = constArc(v); return a }

// CornerRadiusFunc sets a per-datum corner radius.
func (a *Arc) CornerRadiusFunc(f ArcAccessor) *Arc { a.cornerRadius = f; return a }

// PadRadius sets the radius at which the pad angle is measured. Passing a
// negative value restores the default, the quadratic mean of the two radii.
func (a *Arc) PadRadius(v float64) *Arc {
	if v < 0 {
		a.padRadius = nil
	} else {
		a.padRadius = constArc(v)
	}
	return a
}

// PadRadiusFunc sets a per-datum pad radius; nil restores the default.
func (a *Arc) PadRadiusFunc(f ArcAccessor) *Arc { a.padRadius = f; return a }

// StartAngle sets a constant start angle in radians.
func (a *Arc) StartAngle(v float64) *Arc { a.startAngle = constArc(v); return a }

// StartAngleFunc sets a per-datum start angle. The default reads
// [ArcDatum.StartAngle], which is what [Pie] fills in.
func (a *Arc) StartAngleFunc(f ArcAccessor) *Arc { a.startAngle = f; return a }

// EndAngle sets a constant end angle in radians.
func (a *Arc) EndAngle(v float64) *Arc { a.endAngle = constArc(v); return a }

// EndAngleFunc sets a per-datum end angle.
func (a *Arc) EndAngleFunc(f ArcAccessor) *Arc { a.endAngle = f; return a }

// PadAngle sets a constant pad angle in radians.
func (a *Arc) PadAngle(v float64) *Arc { a.padAngle = constArc(v); return a }

// PadAngleFunc sets a per-datum pad angle.
func (a *Arc) PadAngleFunc(f ArcAccessor) *Arc { a.padAngle = f; return a }

// Digits rounds every emitted coordinate to n decimal places.
func (a *Arc) Digits(n int) *Arc { a.digits = digits{n: n}; return a }

// Centroid returns the midpoint of the arc: halfway between the two radii, at
// the bisecting angle. It is where a wedge label goes. Padding and corner
// radius are deliberately ignored — the label should sit in the middle of the
// nominal wedge, not of the drawn one.
func (a *Arc) Centroid(d ArcDatum) (x, y float64) {
	r := (a.innerRadius(d) + a.outerRadius(d)) / 2
	ang := (a.startAngle(d)+a.endAngle(d))/2 - halfPi
	return math.Cos(ang) * r, math.Sin(ang) * r
}

// Generate returns the SVG path data for one arc.
func (a *Arc) Generate(d ArcDatum) string {
	p := a.newPath()

	r0 := a.innerRadius(d)
	r1 := a.outerRadius(d)
	// Angles are rotated a quarter turn so that 0 points up, then everything
	// below works in standard math orientation.
	a0 := a.startAngle(d) - halfPi
	a1 := a.endAngle(d) - halfPi
	da := math.Abs(a1 - a0)
	cw := a1 > a0

	if r1 < r0 {
		r0, r1 = r1, r0
	}

	switch {
	case !(r1 > epsilon):
		// Degenerate: no outer radius at all.
		p.MoveTo(0, 0)

	case da > tau-epsilon:
		// A whole circle (or annulus). SVG cannot express it as one sector, so
		// the outer ring is drawn as a full circle and, if there is one, the
		// inner ring is drawn as a second, counter-wound circle — the even-odd
		// winding is what punches out the hole.
		p.MoveTo(r1*math.Cos(a0), r1*math.Sin(a0))
		p.Arc(0, 0, r1, a0, a1, !cw)
		if r0 > epsilon {
			p.MoveTo(r0*math.Cos(a1), r0*math.Sin(a1))
			p.Arc(0, 0, r0, a1, a0, cw)
		}

	default:
		a.sector(p, r0, r1, a0, a1, da, cw, d)
	}

	p.ClosePath()
	return p.String()
}

// sector draws a proper circular or annular sector: the general case, with
// padding and corner rounding.
func (a *Arc) sector(p *path.Path, r0, r1, a0, a1, da float64, cw bool, d ArcDatum) {
	// a01/a11 bound the outer edge, a00/a10 the inner edge. They start equal
	// and are pulled apart by padding, which insets the inner edge more than
	// the outer one because the same distance subtends a larger angle there.
	a01, a11 := a0, a1
	a00, a10 := a0, a1
	da0, da1 := da, da

	ap := a.padAngle(d) / 2
	var rp float64
	if ap > epsilon {
		if a.padRadius != nil {
			rp = a.padRadius(d)
		} else {
			rp = math.Sqrt(r0*r0 + r1*r1)
		}
	}
	rc := math.Min(math.Abs(r1-r0)/2, a.cornerRadius(d))
	rc0, rc1 := rc, rc

	dir := 1.0
	if !cw {
		dir = -1
	}

	if rp > epsilon {
		// asin converts the padding arc length at rp into an angular inset at
		// each radius; the inner radius gets the larger inset, which is what
		// keeps the gap's sides parallel.
		p0 := math.Asin(rp / r0 * math.Sin(ap))
		p1 := math.Asin(rp / r1 * math.Sin(ap))
		if da0 -= p0 * 2; da0 > epsilon {
			p0 *= dir
			a00 += p0
			a10 -= p0
		} else {
			da0 = 0
			a00 = (a0 + a1) / 2
			a10 = a00
		}
		if da1 -= p1 * 2; da1 > epsilon {
			p1 *= dir
			a01 += p1
			a11 -= p1
		} else {
			da1 = 0
			a01 = (a0 + a1) / 2
			a11 = a01
		}
	}

	x01, y01 := r1*math.Cos(a01), r1*math.Sin(a01)
	x10, y10 := r0*math.Cos(a10), r0*math.Sin(a10)

	if rc > epsilon {
		x11, y11 := r1*math.Cos(a11), r1*math.Sin(a11)
		x00, y00 := r0*math.Cos(a00), r0*math.Sin(a00)

		// For a sector narrower than half a turn the two side edges converge,
		// so the corner circles have to shrink to keep fitting. The apex is
		// where those edges meet; if they are parallel the intersection fails
		// and rounding is abandoned entirely rather than drawn wrong.
		if da < pi {
			if ox, oy, ok := intersect(x01, y01, x00, y00, x11, y11, x10, y10); ok {
				ax, ay := x01-ox, y01-oy
				bx, by := x11-ox, y11-oy
				kc := 1 / math.Sin(math.Acos((ax*bx+ay*by)/(math.Hypot(ax, ay)*math.Hypot(bx, by)))/2)
				lc := math.Hypot(ox, oy)
				rc0 = math.Min(rc, (r0-lc)/(kc-1))
				rc1 = math.Min(rc, (r1-lc)/(kc+1))
			} else {
				rc0, rc1 = 0, 0
			}
		}
	}

	switch {
	case !(da1 > epsilon):
		// Padding ate the whole sector: it collapses to a line at the bisector.
		p.MoveTo(x01, y01)

	case rc1 > epsilon:
		x11, y11 := r1*math.Cos(a11), r1*math.Sin(a11)
		x00, y00 := r0*math.Cos(a00), r0*math.Sin(a00)
		t0 := cornerTangents(x00, y00, x01, y01, r1, rc1, cw)
		t1 := cornerTangents(x11, y11, x10, y10, r1, rc1, cw)

		p.MoveTo(t0.cx+t0.x01, t0.cy+t0.y01)
		if rc1 < rc {
			// The clamp bit: the two corners have merged into one arc.
			p.Arc(t0.cx, t0.cy, rc1, math.Atan2(t0.y01, t0.x01), math.Atan2(t1.y01, t1.x01), !cw)
		} else {
			p.Arc(t0.cx, t0.cy, rc1, math.Atan2(t0.y01, t0.x01), math.Atan2(t0.y11, t0.x11), !cw)
			p.Arc(0, 0, r1, math.Atan2(t0.cy+t0.y11, t0.cx+t0.x11), math.Atan2(t1.cy+t1.y11, t1.cx+t1.x11), !cw)
			p.Arc(t1.cx, t1.cy, rc1, math.Atan2(t1.y11, t1.x11), math.Atan2(t1.y01, t1.x01), !cw)
		}

	default:
		p.MoveTo(x01, y01)
		p.Arc(0, 0, r1, a01, a11, !cw)
	}

	switch {
	case !(r0 > epsilon) || !(da0 > epsilon):
		// A pie wedge, or an annular sector whose inner edge padding collapsed:
		// straight in to the apex.
		p.LineTo(x10, y10)

	case rc0 > epsilon:
		x11, y11 := r1*math.Cos(a11), r1*math.Sin(a11)
		x00, y00 := r0*math.Cos(a00), r0*math.Sin(a00)
		// Negative corner radius: the inner corners bulge the other way.
		t0 := cornerTangents(x10, y10, x11, y11, r0, -rc0, cw)
		t1 := cornerTangents(x01, y01, x00, y00, r0, -rc0, cw)

		p.LineTo(t0.cx+t0.x01, t0.cy+t0.y01)
		if rc0 < rc {
			p.Arc(t0.cx, t0.cy, rc0, math.Atan2(t0.y01, t0.x01), math.Atan2(t1.y01, t1.x01), !cw)
		} else {
			p.Arc(t0.cx, t0.cy, rc0, math.Atan2(t0.y01, t0.x01), math.Atan2(t0.y11, t0.x11), !cw)
			p.Arc(0, 0, r0, math.Atan2(t0.cy+t0.y11, t0.cx+t0.x11), math.Atan2(t1.cy+t1.y11, t1.cx+t1.x11), cw)
			p.Arc(t1.cx, t1.cy, rc0, math.Atan2(t1.y11, t1.x11), math.Atan2(t1.y01, t1.x01), !cw)
		}

	default:
		p.Arc(0, 0, r0, a10, a00, cw)
	}
}

// intersect returns the crossing point of the lines through (x0,y0)-(x1,y1)
// and (x2,y2)-(x3,y3), or ok == false when they are parallel.
func intersect(x0, y0, x1, y1, x2, y2, x3, y3 float64) (x, y float64, ok bool) {
	x10, y10 := x1-x0, y1-y0
	x32, y32 := x3-x2, y3-y2
	t := y32*x10 - x32*y10
	if t*t < epsilon {
		return 0, 0, false
	}
	t = (x32*(y0-y2) - y32*(x0-x2)) / t
	return x0 + t*x10, y0 + t*y10, true
}

// tangents locates a corner circle: cx/cy is its center, (x01,y01) the offset
// from the center to where the corner meets the straight edge, and (x11,y11)
// the offset to where it meets the ring of radius r1.
type tangents struct {
	cx, cy   float64
	x01, y01 float64
	x11, y11 float64
}

// cornerTangents finds the circle of radius rc tangent to both the segment
// (x0,y0)-(x1,y1) and the circle of radius r1 about the origin.
//
// It offsets the segment perpendicularly by rc, then intersects that offset
// line with the circle of radius r1-rc; the two intersections are the two
// possible corner centers and the nearer one is the corner we want. A negative
// rc offsets the other way, which is how the inner corners of an annular
// sector are found with the same code.
func cornerTangents(x0, y0, x1, y1, r1, rc float64, cw bool) tangents {
	x01, y01 := x0-x1, y0-y1
	signedRC := rc
	if !cw {
		signedRC = -rc
	}
	lo := signedRC / math.Hypot(x01, y01)
	ox, oy := lo*y01, -lo*x01
	x11, y11 := x0+ox, y0+oy
	x10, y10 := x1+ox, y1+oy
	x00, y00 := (x11+x10)/2, (y11+y10)/2
	dx, dy := x10-x11, y10-y11
	d2 := dx*dx + dy*dy
	r := r1 - rc
	D := x11*y10 - x10*y11
	sgn := 1.0
	if dy < 0 {
		sgn = -1
	}
	dd := sgn * math.Sqrt(math.Max(0, r*r*d2-D*D))
	cx0 := (D*dy - dx*dd) / d2
	cy0 := (-D*dx - dy*dd) / d2
	cx1 := (D*dy + dx*dd) / d2
	cy1 := (-D*dx + dy*dd) / d2

	dx0, dy0 := cx0-x00, cy0-y00
	dx1, dy1 := cx1-x00, cy1-y00
	if dx0*dx0+dy0*dy0 > dx1*dx1+dy1*dy1 {
		cx0, cy0 = cx1, cy1
	}

	return tangents{
		cx: cx0, cy: cy0,
		x01: -ox, y01: -oy,
		x11: cx0 * (r1/r - 1), y11: cy0 * (r1/r - 1),
	}
}
