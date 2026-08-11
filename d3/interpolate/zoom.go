package interpolate

import "math"

// ZoomView is a viewport for [Zoom]: the x and y coordinates of its center and
// its width, all in the same units.
//
// Only the width is carried, not the height, because the interpolation assumes
// the aspect ratio is fixed — which it is, for a viewport that is being panned
// and zoomed rather than resized. Scale factor is width0/width1.
type ZoomView struct {
	CX, CY, Width float64
}

// rho is the curvature of the zoom path: how much the interpolation zooms out
// on its way between two viewports. Van Wijk and Nuij derived sqrt(2) as the
// value that minimizes perceived travel time for a typical viewer, and d3 uses
// it as the default. Larger values zoom out further and move faster; smaller
// values keep more context on screen but take a more literal path.
const (
	rho     = math.Sqrt2
	rho2    = 2.0 // rho^2
	rho4    = 4.0 // rho^4
	epsilon = 1e-12
)

// Zoom returns an interpolator between two viewports that follows the smooth
// zoom path of Jarke van Wijk and Wim Nuij, "Smooth and efficient zooming and
// panning" (2003).
//
// The problem it solves is that naively interpolating center and width between
// two distant, tightly zoomed viewports produces a horrible result: the content
// streaks past at enormous apparent speed while the zoom level barely changes,
// and the viewer loses all sense of where they went. The van Wijk solution is
// to zoom out as you travel and back in as you arrive, along a hyperbolic path
// chosen so that the *apparent* velocity of the content is constant. The result
// is the recognizable "swoop" of a map flying between two cities.
//
// The two special cases are handled as the paper prescribes. When the centers
// coincide (a pure zoom, no pan) the path degenerates to an exponential
// interpolation of width, which is the right answer — zooming should be
// geometric, not linear, so each unit of t doubles or halves rather than adding
// a fixed amount. When the widths are also equal, the result is constant.
//
// The endpoints are exact: f(0) is a and f(1) is b, returned directly rather
// than computed, since the closed form only lands on them to within
// floating-point error. t is not clamped, and extrapolating is well behaved —
// the path simply continues along the same hyperbola.
//
// The one limitation worth knowing: the closed form evaluates cosh of a term
// proportional to t, so extrapolating far past the endpoints of a path that
// already spans an extreme scale change (many orders of magnitude in width) can
// overflow to infinity and then to NaN. Inside [0, 1] the term is bounded by
// the path's own arc length and this cannot happen.
//
// Use [ZoomDuration] to get the transition length this path implies.
func Zoom(a, b ZoomView) func(t float64) ZoomView {
	ux0, uy0, w0 := a.CX, a.CY, a.Width
	ux1, uy1, w1 := b.CX, b.CY, b.Width
	dx := ux1 - ux0
	dy := uy1 - uy0
	d2 := dx*dx + dy*dy

	var inner func(t float64) ZoomView
	if d2 < epsilon {
		// Pure zoom: interpolate width geometrically.
		s := math.Log(w1/w0) / rho
		inner = func(t float64) ZoomView {
			return ZoomView{
				CX:    ux0 + t*dx,
				CY:    uy0 + t*dy,
				Width: w0 * math.Exp(rho*t*s),
			}
		}
	} else {
		d1 := math.Sqrt(d2)
		b0 := (w1*w1 - w0*w0 + rho4*d2) / (2 * w0 * rho2 * d1)
		b1 := (w1*w1 - w0*w0 - rho4*d2) / (2 * w1 * rho2 * d1)
		// r = log(sqrt(b^2+1) - b) is exactly -asinh(b), and asinh is the form
		// to use: written literally, sqrt(b^2+1) equals b in float64 once b
		// exceeds about 1e8, so the subtraction cancels to zero and the log
		// returns -Inf, poisoning the whole path with NaN. That happens for
		// real inputs — a pan across a scale change of several orders of
		// magnitude — so this is a correctness fix, not a micro-optimization.
		// (d3 evaluates the literal form and has the bug.)
		r0 := -math.Asinh(b0)
		r1 := -math.Asinh(b1)
		s := (r1 - r0) / rho
		coshr0 := math.Cosh(r0)
		inner = func(t float64) ZoomView {
			ts := t * s
			u := w0 / (rho2 * d1) * (coshr0*math.Tanh(rho*ts+r0) - math.Sinh(r0))
			return ZoomView{
				CX:    ux0 + u*dx,
				CY:    uy0 + u*dy,
				Width: w0 * coshr0 / math.Cosh(rho*ts+r0),
			}
		}
	}
	return func(t float64) ZoomView {
		switch t {
		case 0:
			return a
		case 1:
			return b
		}
		return inner(t)
	}
}

// ZoomDuration returns the recommended transition duration in milliseconds for
// the path [Zoom] takes between two viewports.
//
// The point of a derived duration is that a fixed one is always wrong: a short
// hop and a flight across the world should not take the same time. The van Wijk
// path has a natural arc length S in its own parameterization, and traversing
// it at constant apparent velocity takes time proportional to S. The constant
// of proportionality here is d3's — 1000 * rho / sqrt(2) milliseconds per unit,
// which with the default rho is simply 1000 — chosen so that a typical
// same-scale pan across one screen width lands near a second.
//
// Identical viewports give a duration of 0.
func ZoomDuration(a, b ZoomView) float64 {
	dx := b.CX - a.CX
	dy := b.CY - a.CY
	d2 := dx*dx + dy*dy
	var s float64
	if d2 < epsilon {
		s = math.Log(b.Width/a.Width) / rho
	} else {
		d1 := math.Sqrt(d2)
		w0, w1 := a.Width, b.Width
		b0 := (w1*w1 - w0*w0 + rho4*d2) / (2 * w0 * rho2 * d1)
		b1 := (w1*w1 - w0*w0 - rho4*d2) / (2 * w1 * rho2 * d1)
		r0 := -math.Asinh(b0)
		r1 := -math.Asinh(b1)
		s = (r1 - r0) / rho
	}
	return math.Abs(s) * 1000 * rho / math.Sqrt2
}
