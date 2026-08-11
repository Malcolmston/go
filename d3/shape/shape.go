// Package shape is a Go port of d3-shape: the generators that turn data into
// the SVG path strings a chart is actually made of — lines, areas, pie and
// donut arcs, stacked series and scatterplot symbols.
//
// Every generator's output is a string of SVG path data produced by
// [github.com/malcolmston/d3/path], so rendering is a one-liner with
// github.com/malcolmston/react:
//
//	line := shape.NewLine[Point]().X(func(d Point, _ int, _ []Point) float64 { return xs(d.T) }).
//		Y(func(d Point, _ int, _ []Point) float64 { return ys(d.V) })
//	react.H("path", react.Props{"d": line.Generate(data), "fill": "none", "stroke": "steelblue"})
//
// There is no DOM and no canvas anywhere in this package. A generator is a
// pure function from data to a string, which makes it trivially testable and
// safe to call from any goroutine once configured.
//
// # The fluent convention
//
// d3 overloads one method name for both getting and setting a property, which
// Go cannot express. This port uses a single rule instead, applied everywhere:
//
//   - A configuration method is named after the property, takes the new value,
//     and returns the receiver, so configuration chains:
//     NewArc().InnerRadius(40).OuterRadius(80).CornerRadius(4).
//   - The plain name takes the value form that is most useful in practice.
//     For a radius or an angle that is a constant (Arc.InnerRadius(40)); for a
//     per-datum coordinate that is an accessor function (Line.X(fn)).
//   - Where d3 also allows the other form, a Func suffix supplies it:
//     Arc.InnerRadiusFunc(func(d ArcDatum) float64 { … }).
//   - There are no getters. Configuration is write-only; hold on to the values
//     you set if you need them.
//   - The generator is finally invoked with Generate, which returns the path
//     string. An empty input yields "", where d3 returns null — check for the
//     empty string before emitting a <path> element.
//
// Accessor signatures mirror d3's (datum, index, data), which is what makes
// index-dependent and neighbor-dependent accessors possible. Generators are
// generic over the datum type, so no type assertions are needed.
//
// # What lives here
//
// [Line] and [Area] with their radial counterparts [LineRadial] and
// [AreaRadial]; [Arc] (the donut-chart workhorse, with corner radius and
// padding); [Pie], which turns values into the angles [Arc] consumes; [Stack]
// with the full set of offsets and orders; [Symbol] with the seven built-in
// symbol types; and the [Curve] family — [CurveLinear], [CurveBasis],
// [CurveCatmullRom], [CurveMonotoneX] and the rest — which decides how
// consecutive points are interpolated.
package shape

import (
	"math"

	"github.com/malcolmston/d3/path"
)

const (
	pi      = math.Pi
	tau     = 2 * math.Pi
	halfPi  = pi / 2
	epsilon = 1e-12
)

// Context is the drawing surface a [Curve] writes into.
//
// It is deliberately narrower than [path.Path]: curves only ever need these
// four commands, and the interface lets a curve be wrapped by a transforming
// context. Both of the transforms in this package are exactly that —
// [CurveRadial] converts (angle, radius) to (x, y), and CurveMonotoneY reuses
// CurveMonotoneX through a context that swaps the two axes.
type Context interface {
	MoveTo(x, y float64)
	LineTo(x, y float64)
	BezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y float64)
	ClosePath()
}

// pathContext adapts a *path.Path (whose methods return the receiver for
// chaining) to the void-returning [Context] interface.
type pathContext struct{ p *path.Path }

func (c pathContext) MoveTo(x, y float64) { c.p.MoveTo(x, y) }
func (c pathContext) LineTo(x, y float64) { c.p.LineTo(x, y) }
func (c pathContext) BezierCurveTo(a, b, cx, cy, x, y float64) {
	c.p.BezierCurveTo(a, b, cx, cy, x, y)
}
func (c pathContext) ClosePath() { c.p.ClosePath() }

// PathContext returns a [Context] that draws into p. Use it when driving a
// [Curve] directly rather than through a generator.
func PathContext(p *path.Path) Context { return pathContext{p} }

// digits carries the optional fixed-precision setting shared by every
// generator, so they all construct their scratch path the same way.
type digits struct{ n int }

// newPath builds the scratch path a generator serializes into.
func (d digits) newPath() *path.Path {
	if d.n < 0 {
		return path.New()
	}
	return path.NewWithPrecision(d.n)
}

// noDigits is the "full precision" sentinel; -1 rather than 0 because 0 digits
// is a legitimate (integer-rounding) setting.
func noDigits() digits { return digits{n: -1} }
