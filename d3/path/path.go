// Package path is a Go port of d3-path: a serializer that speaks the
// CanvasRenderingContext2D drawing vocabulary (moveTo, lineTo, arc, …) but
// emits an SVG path data string instead of painting pixels.
//
// It exists because d3's shape generators are written against the canvas API,
// yet the overwhelmingly common use is to feed the `d` attribute of an SVG
// <path>. d3-path bridges the two, and so does this package. There is no DOM
// here and none is needed: a [Path] accumulates commands into a string, and
// that string is handed straight to a renderer. With
// github.com/malcolmston/react that is the whole integration story —
//
//	react.H("path", react.Props{"d": p.String()})
//
// — and it is why this package and its sibling d3/shape are the seam where the
// d3 port meets the view layer.
//
// A [Path] is a builder: every method returns the receiver so calls chain.
//
//	d := path.New().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).ClosePath().String()
//	// "M0,0L10,0L10,10Z"
//
// # Number formatting
//
// Path data is mostly numbers, so how they are printed matters for both size
// and diffability. d3 relies on JavaScript's shortest-round-trip number
// formatting, which never emits trailing zeros: 1 is "1", not "1.0000000".
// This package matches that with strconv shortest formatting in non-exponent
// form, so output is byte-comparable with d3's for the values charts actually
// produce. Two deliberate normalizations: negative zero prints as "0" (as
// String(-0) does in JavaScript), and non-finite values print as "NaN",
// "Infinity" and "-Infinity" rather than Go's "+Inf" spelling — such a path is
// invalid SVG either way, but it matches d3 and makes the bug legible.
//
// [NewWithPrecision] is the analogue of d3.path.digits: it rounds every
// coordinate to a fixed number of decimal places before formatting, which
// trades sub-pixel accuracy for markedly smaller strings. Rounding is
// half-up (toward +∞), matching JavaScript's Math.round rather than Go's
// math.Round, which rounds half away from zero; the two differ only on exact
// ties of negative numbers.
//
// # Arcs
//
// [Path.Arc] and [Path.ArcTo] are the only methods that do real geometry: the
// canvas API describes arcs by center/radius/angles or by a tangent fillet,
// while SVG has only the endpoint-parameterized "A" command. Both ports follow
// d3's decomposition exactly, including its edge cases — a zero radius, a
// degenerate or collinear tangent triangle, a sweep of more than a full turn
// (which must be emitted as two "A" commands because a single one cannot
// express a closed circle). See the method docs for the specific behaviors.
package path

import (
	"math"
	"strconv"
	"strings"
)

const (
	pi         = math.Pi
	tau        = 2 * math.Pi
	epsilon    = 1e-6
	tauEpsilon = tau - epsilon
)

// Path builds an SVG path data string from canvas-style drawing calls.
//
// The zero value is not usable; construct with [New] or [NewWithPrecision].
// A Path is not safe for concurrent use.
type Path struct {
	buf strings.Builder

	// Start of the current subpath, restored by ClosePath.
	x0, y0 float64
	// Current point. hasCurrent is false until the first MoveTo/Rect, which is
	// the "is this path empty?" test that Arc and ArcTo branch on.
	x1, y1     float64
	hasCurrent bool

	// round is nil at full precision, otherwise quantizes each coordinate.
	round func(float64) float64
}

// New returns an empty Path that emits full-precision coordinates.
func New() *Path { return &Path{} }

// NewWithPrecision returns an empty Path that rounds every coordinate to
// digits decimal places, the equivalent of d3.path(digits).
//
// Fewer digits means shorter path strings, which is usually the point: at
// typical chart scales three digits is visually indistinguishable from full
// precision and can halve the size of the markup. digits is clamped at zero;
// values above 15 exceed float64's decimal resolution and are treated as full
// precision, matching d3.
func NewWithPrecision(digits int) *Path {
	if digits < 0 {
		digits = 0
	}
	if digits > 15 {
		return New()
	}
	k := math.Pow(10, float64(digits))
	return &Path{round: func(v float64) float64 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return v
		}
		// math.Floor(v*k+0.5) is JavaScript's Math.round (half up), not Go's
		// math.Round (half away from zero).
		return math.Floor(v*k+0.5) / k
	}}
}

// num formats one coordinate the way d3 does: shortest round-tripping decimal,
// no trailing zeros, no exponent.
func (p *Path) num(v float64) string {
	if p.round != nil {
		v = p.round(v)
	}
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	case v == 0:
		// Normalizes -0 to "0", as JavaScript string conversion does.
		return "0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (p *Path) write(s string)     { p.buf.WriteString(s) }
func (p *Path) writeNum(v float64) { p.buf.WriteString(p.num(v)) }

// pt appends "<x>,<y>".
func (p *Path) pt(x, y float64) {
	p.writeNum(x)
	p.buf.WriteByte(',')
	p.writeNum(y)
}

// MoveTo starts a new subpath at (x, y).
func (p *Path) MoveTo(x, y float64) *Path {
	p.x0, p.x1 = x, x
	p.y0, p.y1 = y, y
	p.hasCurrent = true
	p.write("M")
	p.pt(x, y)
	return p
}

// ClosePath closes the current subpath with a straight line back to its start.
//
// It is a no-op on an empty path, so it is always safe to call — the shape
// generators lean on that when a series turns out to have no defined points.
func (p *Path) ClosePath() *Path {
	if p.hasCurrent {
		p.x1, p.y1 = p.x0, p.y0
		p.write("Z")
	}
	return p
}

// LineTo draws a straight line from the current point to (x, y).
func (p *Path) LineTo(x, y float64) *Path {
	p.x1, p.y1 = x, y
	p.hasCurrent = true
	p.write("L")
	p.pt(x, y)
	return p
}

// QuadraticCurveTo draws a quadratic Bézier to (x, y) with control point
// (cpx, cpy).
func (p *Path) QuadraticCurveTo(cpx, cpy, x, y float64) *Path {
	p.write("Q")
	p.pt(cpx, cpy)
	p.buf.WriteByte(',')
	p.x1, p.y1 = x, y
	p.hasCurrent = true
	p.pt(x, y)
	return p
}

// BezierCurveTo draws a cubic Bézier to (x, y) with control points
// (cp1x, cp1y) and (cp2x, cp2y). This is the workhorse of every smooth curve
// in d3/shape.
func (p *Path) BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64) *Path {
	p.write("C")
	p.pt(cp1x, cp1y)
	p.buf.WriteByte(',')
	p.pt(cp2x, cp2y)
	p.buf.WriteByte(',')
	p.x1, p.y1 = x, y
	p.hasCurrent = true
	p.pt(x, y)
	return p
}

// ArcTo draws a circular arc of radius r tangent to the line from the current
// point to (x1, y1) and to the line from (x1, y1) to (x2, y2) — the canvas
// "fillet the corner" primitive.
//
// The degenerate cases all have to be handled explicitly, because the tangent
// construction divides by lengths that can be zero:
//
//   - Empty path: there is no current point to be tangent from, so this
//     behaves as MoveTo(x1, y1).
//   - (x1, y1) coincident with the current point: nothing is drawn.
//   - The three points collinear, or r == 0: no arc exists, so a straight
//     LineTo(x1, y1) is emitted.
//
// Otherwise a "L" to the first tangent point (omitted when it already is the
// current point) is followed by a single "A". A negative radius has no meaning
// and is treated as zero rather than panicking; d3 throws there.
func (p *Path) ArcTo(x1, y1, x2, y2, r float64) *Path {
	if r < 0 || math.IsNaN(r) {
		r = 0
	}

	// Empty path? There is nothing to be tangent to; start one.
	if !p.hasCurrent {
		p.x1, p.y1 = x1, y1
		p.hasCurrent = true
		p.write("M")
		p.pt(x1, y1)
		return p
	}

	x0, y0 := p.x1, p.y1
	x21, y21 := x2-x1, y2-y1
	x01, y01 := x0-x1, y0-y1
	l01_2 := x01*x01 + y01*y01

	// (x1,y1) coincident with the current point: no corner to round.
	if !(l01_2 > epsilon) {
		return p
	}

	// Collinear (equivalently: (x1,y1) coincident with (x2,y2)) or zero
	// radius: the "arc" degenerates to the corner point itself.
	if !(math.Abs(y01*x21-y21*x01) > epsilon) || r == 0 {
		p.x1, p.y1 = x1, y1
		p.write("L")
		p.pt(x1, y1)
		return p
	}

	x20, y20 := x2-x0, y2-y0
	l21_2 := x21*x21 + y21*y21
	l20_2 := x20*x20 + y20*y20
	l21 := math.Sqrt(l21_2)
	l01 := math.Sqrt(l01_2)
	// Half-angle tangent length: how far back along each leg the tangent
	// points sit for a circle of radius r inscribed in the corner.
	l := r * math.Tan((pi-math.Acos((l21_2+l01_2-l20_2)/(2*l21*l01)))/2)
	t01 := l / l01
	t21 := l / l21

	// Line to the start tangent point unless we are already there.
	if math.Abs(t01-1) > epsilon {
		p.write("L")
		p.pt(x1+t01*x01, y1+t01*y01)
	}

	sweep := 0
	if y01*x20 > x01*y20 {
		sweep = 1
	}
	p.write("A")
	p.writeNum(r)
	p.buf.WriteByte(',')
	p.writeNum(r)
	p.write(",0,0,")
	p.buf.WriteString(strconv.Itoa(sweep))
	p.buf.WriteByte(',')
	p.x1 = x1 + t21*x21
	p.y1 = y1 + t21*y21
	p.pt(p.x1, p.y1)
	return p
}

// Arc draws a circular arc centered at (x, y) with radius r, from startAngle
// to endAngle in radians, sweeping counterclockwise when counterclockwise is
// true. Angles are measured from the positive x-axis, and because SVG's y-axis
// points down, positive angles progress clockwise on screen.
//
// The subtleties, all of which d3 handles and this port reproduces:
//
//   - The arc's start point is joined to the existing path with an implicit
//     MoveTo (empty path) or LineTo (path in progress, start point elsewhere).
//     Nothing is emitted when the current point already is the start.
//   - r == 0 draws only that join: an arc of no radius is a point.
//   - A sweep that wraps the wrong way is normalized into [0, 2π).
//   - A sweep of a full turn or more cannot be written as one "A" command,
//     because an arc whose endpoints coincide is ambiguous. It is emitted as
//     two half-circle "A" commands through the antipode, exactly as d3 does.
//   - A sweep smaller than epsilon draws nothing beyond the join.
//
// A negative radius is treated as zero rather than panicking; d3 throws there.
func (p *Path) Arc(x, y, r, startAngle, endAngle float64, counterclockwise bool) *Path {
	if r < 0 || math.IsNaN(r) {
		r = 0
	}
	dx := r * math.Cos(startAngle)
	dy := r * math.Sin(startAngle)
	sx, sy := x+dx, y+dy

	// SVG's sweep flag is 1 for the clockwise (increasing-angle) direction.
	cw := "1"
	if counterclockwise {
		cw = "0"
	}
	da := endAngle - startAngle
	if counterclockwise {
		da = startAngle - endAngle
	}

	switch {
	case !p.hasCurrent:
		p.write("M")
		p.pt(sx, sy)
	case math.Abs(p.x1-sx) > epsilon || math.Abs(p.y1-sy) > epsilon:
		p.write("L")
		p.pt(sx, sy)
	}
	p.x1, p.y1 = sx, sy
	p.hasCurrent = true

	if r == 0 {
		return p
	}

	// Wrong way round? Normalize into [0, 2π).
	if da < 0 {
		da = math.Mod(da, tau) + tau
	}

	switch {
	case da > tauEpsilon:
		// Full circle: two arcs through the antipode.
		p.arcCmd(r, "1", cw, x-dx, y-dy)
		p.arcCmd(r, "1", cw, sx, sy)
		p.x1, p.y1 = sx, sy
	case da > epsilon:
		large := "0"
		if da >= pi {
			large = "1"
		}
		p.x1 = x + r*math.Cos(endAngle)
		p.y1 = y + r*math.Sin(endAngle)
		p.arcCmd(r, large, cw, p.x1, p.y1)
	}
	return p
}

// arcCmd emits one "A rx,ry,0,large,sweep,x,y".
func (p *Path) arcCmd(r float64, large, sweep string, x, y float64) {
	p.write("A")
	p.writeNum(r)
	p.buf.WriteByte(',')
	p.writeNum(r)
	p.write(",0,")
	p.write(large)
	p.buf.WriteByte(',')
	p.write(sweep)
	p.buf.WriteByte(',')
	p.pt(x, y)
}

// Rect draws an axis-aligned rectangle as its own closed subpath.
//
// It uses relative "h"/"v" commands after the initial "M", which is both what
// d3 emits and meaningfully shorter than four absolute "L" commands.
func (p *Path) Rect(x, y, w, h float64) *Path {
	p.x0, p.x1 = x, x
	p.y0, p.y1 = y, y
	p.hasCurrent = true
	p.write("M")
	p.pt(x, y)
	p.write("h")
	p.writeNum(w)
	p.write("v")
	p.writeNum(h)
	p.write("h")
	p.writeNum(-w)
	p.write("Z")
	return p
}

// String returns the accumulated SVG path data. An untouched Path returns "",
// which is the value to test for before emitting a <path> element.
func (p *Path) String() string { return p.buf.String() }
