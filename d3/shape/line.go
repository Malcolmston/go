package shape

// Accessor reads one coordinate out of a datum. The (datum, index, data)
// signature is d3's, and it is worth keeping: an accessor that needs the index
// (a bar chart's x position) or its neighbors (a delta) has them without
// closing over the slice.
type Accessor[T any] func(d T, i int, data []T) float64

// DefinedFunc reports whether a datum should be drawn. Returning false opens a
// gap: the points before and after it end up in separate subpaths of the same
// path string, which is how a line chart shows missing data as a break rather
// than a straight segment across it.
type DefinedFunc[T any] func(d T, i int, data []T) bool

// constAccessor is the accessor used when one has not been configured.
func constAccessor[T any](v float64) Accessor[T] {
	return func(T, int, []T) float64 { return v }
}

// Line generates the path data for a polyline (or a smooth curve, depending on
// the [Curve]) through a data slice.
//
// Because the datum type is a type parameter there is no useful default for
// [Line.X] and [Line.Y] — d3's defaults only make sense for [x, y] tuples — so
// both start out returning 0 and must be set:
//
//	l := shape.NewLine[Datum]().
//		X(func(d Datum, _ int, _ []Datum) float64 { return d.T }).
//		Y(func(d Datum, _ int, _ []Datum) float64 { return d.V })
//	react.H("path", react.Props{"d": l.Generate(data), "fill": "none"})
type Line[T any] struct {
	x, y    Accessor[T]
	defined DefinedFunc[T]
	curve   CurveFactory
	digits
}

// NewLine returns a Line with a linear curve, all points defined, and X and Y
// returning 0 until configured.
func NewLine[T any]() *Line[T] {
	return &Line[T]{
		x:       constAccessor[T](0),
		y:       constAccessor[T](0),
		defined: func(T, int, []T) bool { return true },
		curve:   CurveLinear,
		digits:  noDigits(),
	}
}

// X sets the accessor for the horizontal coordinate.
func (l *Line[T]) X(f Accessor[T]) *Line[T] { l.x = f; return l }

// XConst pins the horizontal coordinate to a constant.
func (l *Line[T]) XConst(v float64) *Line[T] { l.x = constAccessor[T](v); return l }

// Y sets the accessor for the vertical coordinate.
func (l *Line[T]) Y(f Accessor[T]) *Line[T] { l.y = f; return l }

// YConst pins the vertical coordinate to a constant.
func (l *Line[T]) YConst(v float64) *Line[T] { l.y = constAccessor[T](v); return l }

// Defined sets the predicate that decides which data are drawn. See
// [DefinedFunc] for the gap behavior.
func (l *Line[T]) Defined(f DefinedFunc[T]) *Line[T] { l.defined = f; return l }

// Curve sets the interpolation between points. The default is [CurveLinear].
func (l *Line[T]) Curve(f CurveFactory) *Line[T] { l.curve = f; return l }

// Digits rounds every emitted coordinate to n decimal places. Negative n
// restores full precision.
func (l *Line[T]) Digits(n int) *Line[T] { l.digits = digits{n: n}; return l }

// Generate returns the SVG path data for data, or "" when nothing is drawn.
func (l *Line[T]) Generate(data []T) string {
	p := l.newPath()
	out := l.curve(PathContext(p))
	n := len(data)
	defined0 := false
	// The loop deliberately runs one past the end: the extra iteration is what
	// terminates a run of defined points that reaches the end of the data.
	for i := 0; i <= n; i++ {
		ok := i < n && l.defined(data[i], i, data)
		if !ok == defined0 {
			defined0 = !defined0
			if defined0 {
				out.LineStart()
			} else {
				out.LineEnd()
			}
		}
		if defined0 {
			out.Point(l.x(data[i], i, data), l.y(data[i], i, data))
		}
	}
	return p.String()
}

// LineRadial generates a line in polar coordinates: each datum contributes an
// angle and a radius, drawn about the origin with angle 0 pointing up and
// increasing clockwise — the same convention as [Arc] and [Pie], so a radial
// line overlays a donut chart without conversion.
//
// Translate the whole path to place the origin, e.g. with a transform
// attribute on a parent <g>.
type LineRadial[T any] struct{ line *Line[T] }

// NewLineRadial returns a radial line with a linear curve. Angle and Radius
// return 0 until configured.
func NewLineRadial[T any]() *LineRadial[T] {
	l := NewLine[T]()
	l.curve = CurveRadialFactory(CurveLinear)
	return &LineRadial[T]{line: l}
}

// Angle sets the accessor for the angle in radians.
func (l *LineRadial[T]) Angle(f Accessor[T]) *LineRadial[T] { l.line.x = f; return l }

// AngleConst pins the angle to a constant.
func (l *LineRadial[T]) AngleConst(v float64) *LineRadial[T] {
	l.line.x = constAccessor[T](v)
	return l
}

// Radius sets the accessor for the distance from the origin.
func (l *LineRadial[T]) Radius(f Accessor[T]) *LineRadial[T] { l.line.y = f; return l }

// RadiusConst pins the radius to a constant.
func (l *LineRadial[T]) RadiusConst(v float64) *LineRadial[T] {
	l.line.y = constAccessor[T](v)
	return l
}

// Defined sets the predicate that decides which data are drawn.
func (l *LineRadial[T]) Defined(f DefinedFunc[T]) *LineRadial[T] { l.line.defined = f; return l }

// Curve sets the interpolation, automatically wrapped so the curve still
// receives Cartesian points. Closed curves such as [CurveLinearClosed] are the
// natural choice here, since a radial line usually wraps all the way around.
func (l *LineRadial[T]) Curve(f CurveFactory) *LineRadial[T] {
	l.line.curve = CurveRadialFactory(f)
	return l
}

// Digits rounds every emitted coordinate to n decimal places.
func (l *LineRadial[T]) Digits(n int) *LineRadial[T] { l.line.Digits(n); return l }

// Generate returns the SVG path data for data, or "" when nothing is drawn.
func (l *LineRadial[T]) Generate(data []T) string { return l.line.Generate(data) }
