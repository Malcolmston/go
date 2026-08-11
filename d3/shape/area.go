package shape

// Area generates the path data for the region between two lines: a topline
// (x1, y1) and a baseline (x0, y0).
//
// The two lines share one closed subpath. The topline is emitted in data
// order, then the baseline in reverse, and the curve closes the ring — which
// is why an area path ends in "Z" and why a gap in the data produces a
// separate closed ring per contiguous run rather than one ring with a notch.
//
// The usual configuration is the d3 one: set [Area.X] and [Area.Y1] to the
// data, leave [Area.Y0] at its default of 0, and the area fills down to the
// axis. Setting X0/X1 instead of X (or Y0/Y1 with a non-constant Y0) gives a
// band — a confidence interval or a candlestick range.
//
// As with [Line], the accessors have no meaningful default for an arbitrary
// datum type and start out returning 0 (except Y0, which is 0 by definition).
type Area[T any] struct {
	x0, y0  Accessor[T]
	x1, y1  Accessor[T] // nil means "use x0 / y0"
	defined DefinedFunc[T]
	curve   CurveFactory
	digits
}

// NewArea returns an Area with a linear curve, all points defined, a baseline
// pinned to y = 0, and x1/y1 tracking x0/y0 until configured.
func NewArea[T any]() *Area[T] {
	return &Area[T]{
		x0:      constAccessor[T](0),
		y0:      constAccessor[T](0),
		defined: func(T, int, []T) bool { return true },
		curve:   CurveLinear,
		digits:  noDigits(),
	}
}

// X sets both horizontal accessors, so the two lines share an x — the common
// case, where the area is a filled line chart.
func (a *Area[T]) X(f Accessor[T]) *Area[T] { a.x0, a.x1 = f, nil; return a }

// XConst pins both horizontal accessors to a constant.
func (a *Area[T]) XConst(v float64) *Area[T] { return a.X(constAccessor[T](v)) }

// X0 sets the baseline's horizontal accessor.
func (a *Area[T]) X0(f Accessor[T]) *Area[T] { a.x0 = f; return a }

// X0Const pins the baseline's horizontal coordinate to a constant.
func (a *Area[T]) X0Const(v float64) *Area[T] { a.x0 = constAccessor[T](v); return a }

// X1 sets the topline's horizontal accessor. Passing nil makes it track X0.
func (a *Area[T]) X1(f Accessor[T]) *Area[T] { a.x1 = f; return a }

// X1Const pins the topline's horizontal coordinate to a constant.
func (a *Area[T]) X1Const(v float64) *Area[T] { a.x1 = constAccessor[T](v); return a }

// Y sets both vertical accessors.
func (a *Area[T]) Y(f Accessor[T]) *Area[T] { a.y0, a.y1 = f, nil; return a }

// YConst pins both vertical accessors to a constant.
func (a *Area[T]) YConst(v float64) *Area[T] { return a.Y(constAccessor[T](v)) }

// Y0 sets the baseline's vertical accessor. The default is a constant 0.
func (a *Area[T]) Y0(f Accessor[T]) *Area[T] { a.y0 = f; return a }

// Y0Const pins the baseline to a constant height, typically the y coordinate
// of the zero line.
func (a *Area[T]) Y0Const(v float64) *Area[T] { a.y0 = constAccessor[T](v); return a }

// Y1 sets the topline's vertical accessor — for a filled line chart, the data.
func (a *Area[T]) Y1(f Accessor[T]) *Area[T] { a.y1 = f; return a }

// Y1Const pins the topline to a constant height.
func (a *Area[T]) Y1Const(v float64) *Area[T] { a.y1 = constAccessor[T](v); return a }

// Defined sets the predicate that decides which data are drawn.
func (a *Area[T]) Defined(f DefinedFunc[T]) *Area[T] { a.defined = f; return a }

// Curve sets the interpolation between points. The default is [CurveLinear].
func (a *Area[T]) Curve(f CurveFactory) *Area[T] { a.curve = f; return a }

// Digits rounds every emitted coordinate to n decimal places.
func (a *Area[T]) Digits(n int) *Area[T] { a.digits = digits{n: n}; return a }

// Generate returns the SVG path data for data, or "" when nothing is drawn.
func (a *Area[T]) Generate(data []T) string {
	p := a.newPath()
	out := a.curve(PathContext(p))
	n := len(data)

	// The baseline is replayed in reverse after the topline, so its points
	// have to be buffered as they are computed rather than recomputed — the
	// accessors are user code and need not be pure.
	x0z := make([]float64, n)
	y0z := make([]float64, n)

	defined0 := false
	j := 0 // index where the current run of defined points began
	for i := 0; i <= n; i++ {
		ok := i < n && a.defined(data[i], i, data)
		if !ok == defined0 {
			defined0 = !defined0
			if defined0 {
				j = i
				out.AreaStart()
				out.LineStart()
			} else {
				out.LineEnd()
				out.LineStart()
				for k := i - 1; k >= j; k-- {
					out.Point(x0z[k], y0z[k])
				}
				out.LineEnd()
				out.AreaEnd()
			}
		}
		if defined0 {
			x0z[i] = a.x0(data[i], i, data)
			y0z[i] = a.y0(data[i], i, data)
			x, y := x0z[i], y0z[i]
			if a.x1 != nil {
				x = a.x1(data[i], i, data)
			}
			if a.y1 != nil {
				y = a.y1(data[i], i, data)
			}
			out.Point(x, y)
		}
	}
	return p.String()
}

// arealine returns a Line sharing this area's defined predicate, curve and
// precision — the common part of the four edge helpers below.
func (a *Area[T]) arealine(x, y Accessor[T]) *Line[T] {
	l := NewLine[T]()
	l.x, l.y = x, y
	l.defined, l.curve, l.digits = a.defined, a.curve, a.digits
	return l
}

// LineY1 returns a [Line] along the area's topline. It is the idiomatic way to
// stroke the top edge of a fill in a different color: the same data, the same
// curve, one extra <path>.
func (a *Area[T]) LineY1() *Line[T] {
	y := a.y0
	if a.y1 != nil {
		y = a.y1
	}
	return a.arealine(a.x0, y)
}

// LineY0 returns a [Line] along the area's baseline.
func (a *Area[T]) LineY0() *Line[T] { return a.arealine(a.x0, a.y0) }

// LineX1 returns a [Line] along the right edge of a horizontal band.
func (a *Area[T]) LineX1() *Line[T] {
	x := a.x0
	if a.x1 != nil {
		x = a.x1
	}
	return a.arealine(x, a.y0)
}

// LineX0 returns a [Line] along the left edge of a horizontal band.
func (a *Area[T]) LineX0() *Line[T] { return a.arealine(a.x0, a.y0) }

// AreaRadial generates an area in polar coordinates, bounded by an inner and
// an outer radius over a range of angles — a radial stacked chart, or the band
// of a polar confidence interval. Angle 0 points up and increases clockwise,
// matching [Arc].
type AreaRadial[T any] struct{ area *Area[T] }

// NewAreaRadial returns a radial area with a linear curve and an inner radius
// of 0.
func NewAreaRadial[T any]() *AreaRadial[T] {
	a := NewArea[T]()
	a.curve = CurveRadialFactory(CurveLinear)
	return &AreaRadial[T]{area: a}
}

// Angle sets both angular accessors.
func (a *AreaRadial[T]) Angle(f Accessor[T]) *AreaRadial[T] { a.area.X(f); return a }

// AngleConst pins both angular accessors to a constant.
func (a *AreaRadial[T]) AngleConst(v float64) *AreaRadial[T] { a.area.XConst(v); return a }

// StartAngle sets the angular accessor of the inner edge.
func (a *AreaRadial[T]) StartAngle(f Accessor[T]) *AreaRadial[T] { a.area.X0(f); return a }

// EndAngle sets the angular accessor of the outer edge.
func (a *AreaRadial[T]) EndAngle(f Accessor[T]) *AreaRadial[T] { a.area.X1(f); return a }

// Radius sets both radial accessors.
func (a *AreaRadial[T]) Radius(f Accessor[T]) *AreaRadial[T] { a.area.Y(f); return a }

// RadiusConst pins both radial accessors to a constant.
func (a *AreaRadial[T]) RadiusConst(v float64) *AreaRadial[T] { a.area.YConst(v); return a }

// InnerRadius sets the accessor for the inner boundary. The default is 0.
func (a *AreaRadial[T]) InnerRadius(f Accessor[T]) *AreaRadial[T] { a.area.Y0(f); return a }

// InnerRadiusConst pins the inner boundary to a constant.
func (a *AreaRadial[T]) InnerRadiusConst(v float64) *AreaRadial[T] { a.area.Y0Const(v); return a }

// OuterRadius sets the accessor for the outer boundary — the data.
func (a *AreaRadial[T]) OuterRadius(f Accessor[T]) *AreaRadial[T] { a.area.Y1(f); return a }

// OuterRadiusConst pins the outer boundary to a constant.
func (a *AreaRadial[T]) OuterRadiusConst(v float64) *AreaRadial[T] { a.area.Y1Const(v); return a }

// Defined sets the predicate that decides which data are drawn.
func (a *AreaRadial[T]) Defined(f DefinedFunc[T]) *AreaRadial[T] { a.area.Defined(f); return a }

// Curve sets the interpolation, wrapped so the curve receives Cartesian points.
func (a *AreaRadial[T]) Curve(f CurveFactory) *AreaRadial[T] {
	a.area.curve = CurveRadialFactory(f)
	return a
}

// Digits rounds every emitted coordinate to n decimal places.
func (a *AreaRadial[T]) Digits(n int) *AreaRadial[T] { a.area.Digits(n); return a }

// Generate returns the SVG path data for data, or "" when nothing is drawn.
func (a *AreaRadial[T]) Generate(data []T) string { return a.area.Generate(data) }
