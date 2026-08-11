package shape

import "math"

// Curve is the streaming interpolator a [Line] or [Area] pushes points
// through. It is the extension point of this package: the generator decides
// which points exist and in what order, the curve decides what is drawn
// between them.
//
// The protocol is d3's, and the generators obey it exactly:
//
//	AreaStart, (LineStart, Point…, LineEnd)×2, AreaEnd   — for an area
//	LineStart, Point…, LineEnd                            — for a line
//
// An area is two lines: the topline in data order, then the baseline in
// reverse. Curves use AreaStart/AreaEnd to learn that they are inside an area,
// which is why a curve must close the ring after the second line rather than
// after each one. Every gap produced by a defined accessor starts a fresh
// LineStart/LineEnd (or AreaStart/AreaEnd) pair, which is how one path string
// ends up holding several disjoint segments.
type Curve interface {
	AreaStart()
	AreaEnd()
	LineStart()
	LineEnd()
	Point(x, y float64)
}

// CurveFactory binds a curve to the context it draws into. Generators take a
// factory, not a curve, because each Generate call needs its own curve state.
type CurveFactory func(ctx Context) Curve

// truthy reproduces JavaScript's notion of a truthy number, which the curve
// state machines rely on: the "line" flag is 0 inside the first line of an
// area, 1 inside the second, and NaN outside an area entirely.
func truthy(v float64) bool { return v != 0 && !math.IsNaN(v) }

// lineState is the shared "am I inside an area, and which half?" flag.
//
// d3 encodes three states in one number: NaN for a standalone line, 0 for the
// topline of an area, 1 for the baseline. LineEnd flips 0↔1 (and leaves NaN
// alone), so the baseline knows to LineTo rather than MoveTo — that is what
// joins the two halves into one closed ring — and closes the subpath.
type lineState struct{ line float64 }

// AreaStart and AreaEnd are promoted onto every curve that embeds lineState,
// which is how they satisfy [Curve] without repeating the flag bookkeeping.
func (l *lineState) AreaStart() { l.line = 0 }
func (l *lineState) AreaEnd()   { l.line = math.NaN() }
func (l *lineState) flip()      { l.line = 1 - l.line }

// newLineState starts outside an area.
func newLineState() lineState { return lineState{line: math.NaN()} }

// moveOrLine emits the first point of a line: a MoveTo normally, but a LineTo
// when this is the baseline of an area, so the ring stays connected.
func (l *lineState) moveOrLine(ctx Context, x, y float64) {
	if truthy(l.line) {
		ctx.LineTo(x, y)
	} else {
		ctx.MoveTo(x, y)
	}
}

// shouldClose reports whether LineEnd must close the subpath: always on the
// baseline of an area, and for a standalone line only when it degenerated to a
// single point (so a round line cap still renders a dot).
func (l *lineState) shouldClose(point int, single int) bool {
	return truthy(l.line) || (l.line != 0 && point == single)
}

// ---------------------------------------------------------------------------
// Linear
// ---------------------------------------------------------------------------

type curveLinear struct {
	ctx   Context
	point int
	lineState
}

// CurveLinear draws straight line segments between points. It is the default
// for every generator and the only curve that passes exactly through the data
// with no interpolation error.
func CurveLinear(ctx Context) Curve {
	return &curveLinear{ctx: ctx, lineState: newLineState()}
}

func (c *curveLinear) LineStart() { c.point = 0 }
func (c *curveLinear) LineEnd() {
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveLinear) Point(x, y float64) {
	if c.point == 0 {
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
		return
	}
	if c.point == 1 {
		c.point = 2
	}
	c.ctx.LineTo(x, y)
}

type curveLinearClosed struct {
	ctx   Context
	point int
}

// CurveLinearClosed draws straight segments and closes the loop back to the
// first point — a polygon rather than a polyline. It ignores the area
// protocol, so it is a line-only curve.
func CurveLinearClosed(ctx Context) Curve { return &curveLinearClosed{ctx: ctx} }

func (c *curveLinearClosed) AreaStart() {}
func (c *curveLinearClosed) AreaEnd()   {}
func (c *curveLinearClosed) LineStart() { c.point = 0 }
func (c *curveLinearClosed) LineEnd() {
	if c.point != 0 {
		c.ctx.ClosePath()
	}
}
func (c *curveLinearClosed) Point(x, y float64) {
	if c.point != 0 {
		c.ctx.LineTo(x, y)
	} else {
		c.point = 1
		c.ctx.MoveTo(x, y)
	}
}

// ---------------------------------------------------------------------------
// Basis
// ---------------------------------------------------------------------------

// basisPoint emits the cubic B-spline segment for the window (x0,y0), (x1,y1),
// (x,y). A uniform B-spline of degree 3 does not interpolate its control
// points, which is why the curve looks smooth but does not touch the data.
func basisPoint(ctx Context, x0, y0, x1, y1, x, y float64) {
	ctx.BezierCurveTo(
		(2*x0+x1)/3, (2*y0+y1)/3,
		(x0+2*x1)/3, (y0+2*y1)/3,
		(x0+4*x1+x)/6, (y0+4*y1+y)/6,
	)
}

type curveBasis struct {
	ctx            Context
	x0, y0, x1, y1 float64
	point          int
	lineState
}

// CurveBasis draws a cubic uniform B-spline through the points. The spline
// does not pass through the data — it is pulled toward it — which is the price
// of C² continuity.
func CurveBasis(ctx Context) Curve { return &curveBasis{ctx: ctx, lineState: newLineState()} }

func (c *curveBasis) LineStart() {
	c.x0, c.x1, c.y0, c.y1 = nan4()
	c.point = 0
}
func (c *curveBasis) LineEnd() {
	switch c.point {
	case 3:
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x1, c.y1)
		fallthrough
	case 2:
		c.ctx.LineTo(c.x1, c.y1)
	}
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveBasis) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		// Straight run-in to the spline's true start, one sixth of the way.
		c.ctx.LineTo((5*c.x0+c.x1)/6, (5*c.y0+c.y1)/6)
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, x, y)
	default:
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, x, y)
	}
	c.x0, c.x1 = c.x1, x
	c.y0, c.y1 = c.y1, y
}

type curveBasisClosed struct {
	ctx                    Context
	x0, y0, x1, y1         float64
	x2, y2, x3, y3, x4, y4 float64
	point                  int
}

// CurveBasisClosed draws a closed cubic B-spline: the control polygon wraps
// around, so the first three points are replayed at the end to close the loop
// seamlessly. Line-only, like every closed curve.
func CurveBasisClosed(ctx Context) Curve { return &curveBasisClosed{ctx: ctx} }

func (c *curveBasisClosed) AreaStart() {}
func (c *curveBasisClosed) AreaEnd()   {}
func (c *curveBasisClosed) LineStart() {
	c.x0, c.x1, c.y0, c.y1 = nan4()
	c.x2, c.y2, c.x3, c.y3, c.x4, c.y4 = nan6()
	c.point = 0
}
func (c *curveBasisClosed) LineEnd() {
	switch c.point {
	case 1:
		c.ctx.MoveTo(c.x2, c.y2)
		c.ctx.ClosePath()
	case 2:
		c.ctx.MoveTo((c.x2+2*c.x3)/3, (c.y2+2*c.y3)/3)
		c.ctx.LineTo((c.x3+2*c.x2)/3, (c.y3+2*c.y2)/3)
		c.ctx.ClosePath()
	case 3:
		// Replay the first three points to wrap the spline around.
		c.Point(c.x2, c.y2)
		c.Point(c.x3, c.y3)
		c.Point(c.x4, c.y4)
	}
}
func (c *curveBasisClosed) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
		c.x2, c.y2 = x, y
	case 1:
		c.point = 2
		c.x3, c.y3 = x, y
	case 2:
		c.point = 3
		c.x4, c.y4 = x, y
		c.ctx.MoveTo((c.x0+4*c.x1+x)/6, (c.y0+4*c.y1+y)/6)
	default:
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, x, y)
	}
	c.x0, c.x1 = c.x1, x
	c.y0, c.y1 = c.y1, y
}

type curveBasisOpen struct {
	ctx            Context
	x0, y0, x1, y1 float64
	point          int
	lineState
}

// CurveBasisOpen draws the same B-spline as [CurveBasis] but omits the
// straight run-in and run-out, so the curve starts and ends at the spline's
// natural endpoints — inside the data rather than at it.
func CurveBasisOpen(ctx Context) Curve { return &curveBasisOpen{ctx: ctx, lineState: newLineState()} }

func (c *curveBasisOpen) LineStart() {
	c.x0, c.x1, c.y0, c.y1 = nan4()
	c.point = 0
}
func (c *curveBasisOpen) LineEnd() {
	if c.shouldClose(c.point, 3) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveBasisOpen) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		c.moveOrLine(c.ctx, (c.x0+4*c.x1+x)/6, (c.y0+4*c.y1+y)/6)
	case 3:
		c.point = 4
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, x, y)
	default:
		basisPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, x, y)
	}
	c.x0, c.x1 = c.x1, x
	c.y0, c.y1 = c.y1, y
}

// ---------------------------------------------------------------------------
// Bundle
// ---------------------------------------------------------------------------

type curveBundle struct {
	basis *curveBasis
	beta  float64
	x, y  []float64
}

// CurveBundleFactory returns a straightened B-spline: every point is blended
// toward the straight line from the first point to the last, by 1-beta. beta 1
// is [CurveBasis]; beta 0 collapses to that straight line. It is the curve
// used for hierarchical edge bundling, hence the name, and it is line-only.
func CurveBundleFactory(beta float64) CurveFactory {
	return func(ctx Context) Curve {
		return &curveBundle{basis: &curveBasis{ctx: ctx, lineState: newLineState()}, beta: beta}
	}
}

// CurveBundle is [CurveBundleFactory] with d3's default beta of 0.85.
func CurveBundle(ctx Context) Curve { return CurveBundleFactory(0.85)(ctx) }

func (c *curveBundle) AreaStart() {}
func (c *curveBundle) AreaEnd()   {}
func (c *curveBundle) LineStart() {
	c.x, c.y = c.x[:0], c.y[:0]
	c.basis.LineStart()
}
func (c *curveBundle) LineEnd() {
	if j := len(c.x) - 1; j > 0 {
		x0, y0 := c.x[0], c.y[0]
		dx, dy := c.x[j]-x0, c.y[j]-y0
		for i := 0; i <= j; i++ {
			t := float64(i) / float64(j)
			c.basis.Point(
				c.beta*c.x[i]+(1-c.beta)*(x0+t*dx),
				c.beta*c.y[i]+(1-c.beta)*(y0+t*dy),
			)
		}
	}
	c.x, c.y = c.x[:0], c.y[:0]
	c.basis.LineEnd()
}
func (c *curveBundle) Point(x, y float64) {
	c.x = append(c.x, x)
	c.y = append(c.y, y)
}

// ---------------------------------------------------------------------------
// Cardinal
// ---------------------------------------------------------------------------

// cardinalPoint emits the Bézier for a cardinal spline segment: the tangent at
// each interior point is k times the vector between its neighbors, so the
// curve interpolates the data (unlike a B-spline) with a controllable
// overshoot.
func cardinalPoint(ctx Context, k, x0, y0, x1, y1, x2, y2, x, y float64) {
	ctx.BezierCurveTo(
		x1+k*(x2-x0), y1+k*(y2-y0),
		x2+k*(x1-x), y2+k*(y1-y),
		x2, y2,
	)
}

type curveCardinal struct {
	ctx                    Context
	k                      float64
	x0, y0, x1, y1, x2, y2 float64
	point                  int
	lineState
}

// CurveCardinalFactory returns a cardinal spline with the given tension.
// Tension 0 is the loosest (and equals a Catmull–Rom spline with uniform
// parameterization); tension 1 produces straight segments.
func CurveCardinalFactory(tension float64) CurveFactory {
	k := (1 - tension) / 6
	return func(ctx Context) Curve {
		return &curveCardinal{ctx: ctx, k: k, lineState: newLineState()}
	}
}

// CurveCardinal is [CurveCardinalFactory] with d3's default tension of 0.
func CurveCardinal(ctx Context) Curve { return CurveCardinalFactory(0)(ctx) }

func (c *curveCardinal) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.point = 0
}
func (c *curveCardinal) LineEnd() {
	switch c.point {
	case 2:
		c.ctx.LineTo(c.x2, c.y2)
	case 3:
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, c.x1, c.y1)
	}
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveCardinal) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
	case 1:
		c.point = 2
		c.x1, c.y1 = x, y
	case 2:
		c.point = 3
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	default:
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

type curveCardinalClosed struct {
	ctx                    Context
	k                      float64
	x0, y0, x1, y1, x2, y2 float64
	x3, y3, x4, y4, x5, y5 float64
	point                  int
}

// CurveCardinalClosedFactory returns a closed cardinal spline: the first three
// points are replayed so the loop joins smoothly. Line-only.
func CurveCardinalClosedFactory(tension float64) CurveFactory {
	k := (1 - tension) / 6
	return func(ctx Context) Curve { return &curveCardinalClosed{ctx: ctx, k: k} }
}

// CurveCardinalClosed is [CurveCardinalClosedFactory] with tension 0.
func CurveCardinalClosed(ctx Context) Curve { return CurveCardinalClosedFactory(0)(ctx) }

func (c *curveCardinalClosed) AreaStart() {}
func (c *curveCardinalClosed) AreaEnd()   {}
func (c *curveCardinalClosed) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.x3, c.y3, c.x4, c.y4, c.x5, c.y5 = nan6()
	c.point = 0
}
func (c *curveCardinalClosed) LineEnd() {
	switch c.point {
	case 1:
		c.ctx.MoveTo(c.x3, c.y3)
		c.ctx.ClosePath()
	case 2:
		c.ctx.LineTo(c.x3, c.y3)
		c.ctx.ClosePath()
	case 3:
		c.Point(c.x3, c.y3)
		c.Point(c.x4, c.y4)
		c.Point(c.x5, c.y5)
	}
}
func (c *curveCardinalClosed) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
		c.x3, c.y3 = x, y
	case 1:
		c.point = 2
		c.x4, c.y4 = x, y
		c.ctx.MoveTo(x, y)
	case 2:
		c.point = 3
		c.x5, c.y5 = x, y
	default:
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

type curveCardinalOpen struct {
	ctx                    Context
	k                      float64
	x0, y0, x1, y1, x2, y2 float64
	point                  int
	lineState
}

// CurveCardinalOpenFactory returns a cardinal spline that omits the first and
// last segments, so the curve spans only the interior points.
func CurveCardinalOpenFactory(tension float64) CurveFactory {
	k := (1 - tension) / 6
	return func(ctx Context) Curve {
		return &curveCardinalOpen{ctx: ctx, k: k, lineState: newLineState()}
	}
}

// CurveCardinalOpen is [CurveCardinalOpenFactory] with tension 0.
func CurveCardinalOpen(ctx Context) Curve { return CurveCardinalOpenFactory(0)(ctx) }

func (c *curveCardinalOpen) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.point = 0
}
func (c *curveCardinalOpen) LineEnd() {
	if c.shouldClose(c.point, 3) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveCardinalOpen) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		c.moveOrLine(c.ctx, c.x2, c.y2)
	case 3:
		c.point = 4
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	default:
		cardinalPoint(c.ctx, c.k, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

// ---------------------------------------------------------------------------
// Catmull–Rom
// ---------------------------------------------------------------------------

// catmullRomState holds the chord lengths raised to the alpha power that
// distinguish a Catmull–Rom spline from a plain cardinal one. Weighting each
// tangent by chord length is what removes the cusps and self-intersections a
// uniform spline develops when the points are unevenly spaced.
type catmullRomState struct {
	l01a, l12a, l23a    float64
	l012a, l122a, l232a float64
	alpha               float64
}

func (s *catmullRomState) reset() {
	s.l01a, s.l12a, s.l23a = 0, 0, 0
	s.l012a, s.l122a, s.l232a = 0, 0, 0
}

// measure records the chord length from (x2,y2) to the incoming point.
func (s *catmullRomState) measure(x2, y2, x, y float64) {
	x23, y23 := x2-x, y2-y
	s.l232a = math.Pow(x23*x23+y23*y23, s.alpha)
	s.l23a = math.Sqrt(s.l232a)
}

func (s *catmullRomState) shift() {
	s.l01a, s.l12a = s.l12a, s.l23a
	s.l012a, s.l122a = s.l122a, s.l232a
}

// catmullRomPoint emits one segment, nudging the cardinal control points by
// the chord-length weights. When a chord is degenerate the corresponding
// adjustment is skipped, which keeps repeated points from producing NaNs.
func (s *catmullRomState) catmullRomPoint(ctx Context, x0, y0, x1p, y1p, x2, y2, x, y float64) {
	x1, y1 := x1p, y1p
	x2c, y2c := x2, y2
	if s.l01a > epsilon {
		a := 2*s.l012a + 3*s.l01a*s.l12a + s.l122a
		n := 3 * s.l01a * (s.l01a + s.l12a)
		x1 = (x1p*a - x0*s.l122a + x2*s.l012a) / n
		y1 = (y1p*a - y0*s.l122a + y2*s.l012a) / n
	}
	if s.l23a > epsilon {
		b := 2*s.l232a + 3*s.l23a*s.l12a + s.l122a
		m := 3 * s.l23a * (s.l23a + s.l12a)
		x2c = (x2*b + x1p*s.l232a - x*s.l122a) / m
		y2c = (y2*b + y1p*s.l232a - y*s.l122a) / m
	}
	ctx.BezierCurveTo(x1, y1, x2c, y2c, x2, y2)
}

type curveCatmullRom struct {
	ctx                    Context
	x0, y0, x1, y1, x2, y2 float64
	point                  int
	catmullRomState
	lineState
}

// CurveCatmullRomFactory returns a Catmull–Rom spline with the given alpha.
// alpha 0 is uniform (equivalent to a cardinal spline with tension 0), 0.5 is
// centripetal — the value that provably avoids cusps and self-intersections —
// and 1 is chordal. alpha 0 is delegated to [CurveCardinal] exactly as d3 does.
func CurveCatmullRomFactory(alpha float64) CurveFactory {
	if alpha == 0 {
		return CurveCardinalFactory(0)
	}
	return func(ctx Context) Curve {
		return &curveCatmullRom{
			ctx:             ctx,
			catmullRomState: catmullRomState{alpha: alpha},
			lineState:       newLineState(),
		}
	}
}

// CurveCatmullRom is [CurveCatmullRomFactory] with d3's default alpha of 0.5,
// the centripetal parameterization.
func CurveCatmullRom(ctx Context) Curve { return CurveCatmullRomFactory(0.5)(ctx) }

func (c *curveCatmullRom) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.reset()
	c.point = 0
}
func (c *curveCatmullRom) LineEnd() {
	switch c.point {
	case 2:
		c.ctx.LineTo(c.x2, c.y2)
	case 3:
		c.Point(c.x2, c.y2)
	}
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveCatmullRom) Point(x, y float64) {
	if c.point != 0 {
		c.measure(c.x2, c.y2, x, y)
	}
	switch c.point {
	case 0:
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		c.catmullRomPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	default:
		c.catmullRomPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.shift()
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

type curveCatmullRomClosed struct {
	ctx                    Context
	x0, y0, x1, y1, x2, y2 float64
	x3, y3, x4, y4, x5, y5 float64
	point                  int
	catmullRomState
}

// CurveCatmullRomClosedFactory returns a closed Catmull–Rom spline. Line-only.
func CurveCatmullRomClosedFactory(alpha float64) CurveFactory {
	if alpha == 0 {
		return CurveCardinalClosedFactory(0)
	}
	return func(ctx Context) Curve {
		return &curveCatmullRomClosed{ctx: ctx, catmullRomState: catmullRomState{alpha: alpha}}
	}
}

// CurveCatmullRomClosed is [CurveCatmullRomClosedFactory] with alpha 0.5.
func CurveCatmullRomClosed(ctx Context) Curve { return CurveCatmullRomClosedFactory(0.5)(ctx) }

func (c *curveCatmullRomClosed) AreaStart() {}
func (c *curveCatmullRomClosed) AreaEnd()   {}
func (c *curveCatmullRomClosed) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.x3, c.y3, c.x4, c.y4, c.x5, c.y5 = nan6()
	c.reset()
	c.point = 0
}
func (c *curveCatmullRomClosed) LineEnd() {
	switch c.point {
	case 1:
		c.ctx.MoveTo(c.x3, c.y3)
		c.ctx.ClosePath()
	case 2:
		c.ctx.LineTo(c.x3, c.y3)
		c.ctx.ClosePath()
	case 3:
		c.Point(c.x3, c.y3)
		c.Point(c.x4, c.y4)
		c.Point(c.x5, c.y5)
	}
}
func (c *curveCatmullRomClosed) Point(x, y float64) {
	if c.point != 0 {
		c.measure(c.x2, c.y2, x, y)
	}
	switch c.point {
	case 0:
		c.point = 1
		c.x3, c.y3 = x, y
	case 1:
		c.point = 2
		c.x4, c.y4 = x, y
		c.ctx.MoveTo(x, y)
	case 2:
		c.point = 3
		c.x5, c.y5 = x, y
	default:
		c.catmullRomPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.shift()
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

type curveCatmullRomOpen struct {
	ctx                    Context
	x0, y0, x1, y1, x2, y2 float64
	point                  int
	catmullRomState
	lineState
}

// CurveCatmullRomOpenFactory returns a Catmull–Rom spline over the interior
// points only.
func CurveCatmullRomOpenFactory(alpha float64) CurveFactory {
	if alpha == 0 {
		return CurveCardinalOpenFactory(0)
	}
	return func(ctx Context) Curve {
		return &curveCatmullRomOpen{
			ctx:             ctx,
			catmullRomState: catmullRomState{alpha: alpha},
			lineState:       newLineState(),
		}
	}
}

// CurveCatmullRomOpen is [CurveCatmullRomOpenFactory] with alpha 0.5.
func CurveCatmullRomOpen(ctx Context) Curve { return CurveCatmullRomOpenFactory(0.5)(ctx) }

func (c *curveCatmullRomOpen) LineStart() {
	c.x0, c.y0, c.x1, c.y1, c.x2, c.y2 = nan6()
	c.reset()
	c.point = 0
}
func (c *curveCatmullRomOpen) LineEnd() {
	if c.shouldClose(c.point, 3) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveCatmullRomOpen) Point(x, y float64) {
	if c.point != 0 {
		c.measure(c.x2, c.y2, x, y)
	}
	switch c.point {
	case 0:
		c.point = 1
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		c.moveOrLine(c.ctx, c.x2, c.y2)
	case 3:
		c.point = 4
		c.catmullRomPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	default:
		c.catmullRomPoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.x2, c.y2, x, y)
	}
	c.shift()
	c.x0, c.x1, c.x2 = c.x1, c.x2, x
	c.y0, c.y1, c.y2 = c.y1, c.y2, y
}

// ---------------------------------------------------------------------------
// Monotone
// ---------------------------------------------------------------------------

// sign matches JavaScript's convention in the Fritsch–Carlson filter: anything
// not negative (including NaN) counts as positive.
func sign(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}

// slope3 computes the tangent at the middle of three points using the
// Fritsch–Carlson limiter, which is what makes this curve monotone: the slope
// is clamped to at most three times the smaller adjacent secant, and forced to
// zero whenever the two secants disagree in sign (a local extremum). Without
// that clamp a cubic through the same points overshoots and invents maxima the
// data does not have.
func slope3(x0, y0, x1, y1, x2, y2 float64) float64 {
	h0 := x1 - x0
	h1 := x2 - x1
	// Dividing by a signed zero is deliberate: a repeated x must yield an
	// infinite secant of the correct sign so the limiter still picks the finite
	// neighbor, which is d3's `h0 || h1 < 0 && -0` trick.
	s0 := (y1 - y0) / signedZero(h0, h1)
	s1 := (y2 - y1) / signedZero(h1, h0)
	p := (s0*h1 + s1*h0) / (h0 + h1)
	r := (sign(s0) + sign(s1)) * math.Min(math.Min(math.Abs(s0), math.Abs(s1)), 0.5*math.Abs(p))
	if math.IsNaN(r) {
		return 0
	}
	return r
}

// signedZero returns h when it is non-zero, otherwise a zero whose sign is
// chosen so that division by it yields an infinity pointing the right way.
func signedZero(h, other float64) float64 {
	if h != 0 {
		return h
	}
	if other < 0 {
		return math.Copysign(0, -1)
	}
	return 0
}

// slope2 computes the one-sided tangent at an endpoint from the interior
// tangent t, so the end segment blends into the rest.
func slope2(x0, y0, x1, y1, t float64) float64 {
	h := x1 - x0
	if h != 0 {
		return (3*(y1-y0)/h - t) / 2
	}
	return t
}

// monotonePoint emits the Hermite segment for tangents t0 and t1.
func monotonePoint(ctx Context, x0, y0, x1, y1, t0, t1 float64) {
	dx := (x1 - x0) / 3
	ctx.BezierCurveTo(x0+dx, y0+dx*t0, x1-dx, y1-dx*t1, x1, y1)
}

type curveMonotoneX struct {
	ctx            Context
	x0, y0, x1, y1 float64
	t0             float64
	point          int
	lineState
}

// CurveMonotoneX draws a cubic spline that preserves monotonicity in y as a
// function of x: where the data rises the curve rises, and it never overshoots
// a local extremum. That makes it the right default for time series, where an
// invented dip below zero would be a lie about the data.
//
// x must be monotonically increasing; the curve is undefined otherwise.
// Coincident consecutive points are ignored rather than producing a NaN slope.
func CurveMonotoneX(ctx Context) Curve {
	return &curveMonotoneX{ctx: ctx, lineState: newLineState()}
}

func (c *curveMonotoneX) LineStart() {
	c.x0, c.y0, c.x1, c.y1 = nan4()
	c.t0 = math.NaN()
	c.point = 0
}
func (c *curveMonotoneX) LineEnd() {
	switch c.point {
	case 2:
		c.ctx.LineTo(c.x1, c.y1)
	case 3:
		monotonePoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.t0, slope2(c.x0, c.y0, c.x1, c.y1, c.t0))
	}
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	c.flip()
}
func (c *curveMonotoneX) Point(x, y float64) {
	if x == c.x1 && y == c.y1 {
		return // Coincident point: no segment, and no zero-length secant.
	}
	t1 := math.NaN()
	switch c.point {
	case 0:
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
	case 1:
		c.point = 2
	case 2:
		c.point = 3
		t1 = slope3(c.x0, c.y0, c.x1, c.y1, x, y)
		monotonePoint(c.ctx, c.x0, c.y0, c.x1, c.y1, slope2(c.x0, c.y0, c.x1, c.y1, t1), t1)
	default:
		t1 = slope3(c.x0, c.y0, c.x1, c.y1, x, y)
		monotonePoint(c.ctx, c.x0, c.y0, c.x1, c.y1, c.t0, t1)
	}
	c.x0, c.x1 = c.x1, x
	c.y0, c.y1 = c.y1, y
	c.t0 = t1
}

// reflectContext swaps the two axes of every command, which is the whole
// implementation of CurveMonotoneY: run the x-monotone algorithm on
// transposed input and transpose the output back.
type reflectContext struct{ ctx Context }

func (r reflectContext) MoveTo(x, y float64) { r.ctx.MoveTo(y, x) }
func (r reflectContext) LineTo(x, y float64) { r.ctx.LineTo(y, x) }
func (r reflectContext) BezierCurveTo(a, b, c, d, x, y float64) {
	r.ctx.BezierCurveTo(b, a, d, c, y, x)
}
func (r reflectContext) ClosePath() { r.ctx.ClosePath() }

type curveMonotoneY struct{ inner *curveMonotoneX }

// CurveMonotoneY is [CurveMonotoneX] with the axes exchanged: it preserves
// monotonicity in x as a function of y, for charts whose independent variable
// runs down the page.
func CurveMonotoneY(ctx Context) Curve {
	return &curveMonotoneY{inner: &curveMonotoneX{
		ctx:       reflectContext{ctx},
		lineState: newLineState(),
	}}
}

func (c *curveMonotoneY) AreaStart()         { c.inner.AreaStart() }
func (c *curveMonotoneY) AreaEnd()           { c.inner.AreaEnd() }
func (c *curveMonotoneY) LineStart()         { c.inner.LineStart() }
func (c *curveMonotoneY) LineEnd()           { c.inner.LineEnd() }
func (c *curveMonotoneY) Point(x, y float64) { c.inner.Point(y, x) }

// ---------------------------------------------------------------------------
// Natural
// ---------------------------------------------------------------------------

type curveNatural struct {
	ctx  Context
	x, y []float64
	lineState
}

// CurveNatural draws a natural cubic spline: the interpolating cubic with
// continuous second derivatives and zero curvature at both ends. Unlike every
// other curve here it is not local — each point influences the whole curve —
// so it can only be solved once all the points have arrived, which is why it
// buffers them until LineEnd.
func CurveNatural(ctx Context) Curve { return &curveNatural{ctx: ctx, lineState: newLineState()} }

func (c *curveNatural) LineStart() { c.x, c.y = c.x[:0], c.y[:0] }
func (c *curveNatural) LineEnd() {
	n := len(c.x)
	if n > 0 {
		c.moveOrLine(c.ctx, c.x[0], c.y[0])
		if n == 2 {
			c.ctx.LineTo(c.x[1], c.y[1])
		} else if n > 2 {
			pxa, pxb := naturalControlPoints(c.x)
			pya, pyb := naturalControlPoints(c.y)
			for i := 0; i+1 < n; i++ {
				c.ctx.BezierCurveTo(pxa[i], pya[i], pxb[i], pyb[i], c.x[i+1], c.y[i+1])
			}
		}
	}
	if truthy(c.line) || (c.line != 0 && n == 1) {
		c.ctx.ClosePath()
	}
	c.flip()
	c.x, c.y = c.x[:0], c.y[:0]
}
func (c *curveNatural) Point(x, y float64) {
	c.x = append(c.x, x)
	c.y = append(c.y, y)
}

// naturalControlPoints solves the tridiagonal system for a natural cubic
// spline's Bézier control points along one axis. The system is diagonally
// dominant, so the Thomas algorithm (forward elimination, back substitution)
// is stable without pivoting.
func naturalControlPoints(x []float64) (a, b []float64) {
	n := len(x) - 1
	a = make([]float64, n)
	b = make([]float64, n)
	r := make([]float64, n)

	a[0], b[0], r[0] = 0, 2, x[0]+2*x[1]
	for i := 1; i < n-1; i++ {
		a[i], b[i], r[i] = 1, 4, 4*x[i]+2*x[i+1]
	}
	a[n-1], b[n-1], r[n-1] = 2, 7, 8*x[n-1]+x[n]

	for i := 1; i < n; i++ {
		m := a[i] / b[i-1]
		b[i] -= m
		r[i] -= m * r[i-1]
	}
	a[n-1] = r[n-1] / b[n-1]
	for i := n - 2; i >= 0; i-- {
		a[i] = (r[i] - a[i+1]) / b[i]
	}
	b[n-1] = (x[n] + a[n-1]) / 2
	for i := 0; i < n-1; i++ {
		b[i] = 2*x[i+1] - a[i+1]
	}
	return a, b
}

// ---------------------------------------------------------------------------
// Step
// ---------------------------------------------------------------------------

type curveStep struct {
	ctx   Context
	t     float64
	x, y  float64
	point int
	lineState
}

// CurveStepFactory returns a piecewise-constant curve whose vertical
// transition happens a fraction t of the way between each pair of points.
func CurveStepFactory(t float64) CurveFactory {
	return func(ctx Context) Curve { return &curveStep{ctx: ctx, t: t, lineState: newLineState()} }
}

// CurveStep transitions midway between points (t = 0.5).
func CurveStep(ctx Context) Curve { return CurveStepFactory(0.5)(ctx) }

// CurveStepBefore holds the previous value until the next x, so each point's
// value begins at that point (t = 0).
func CurveStepBefore(ctx Context) Curve { return CurveStepFactory(0)(ctx) }

// CurveStepAfter holds each value until the next x, so each point's value
// extends to the right (t = 1).
func CurveStepAfter(ctx Context) Curve { return CurveStepFactory(1)(ctx) }

func (c *curveStep) LineStart() {
	c.x, c.y = math.NaN(), math.NaN()
	c.point = 0
}
func (c *curveStep) LineEnd() {
	if 0 < c.t && c.t < 1 && c.point == 2 {
		c.ctx.LineTo(c.x, c.y)
	}
	if c.shouldClose(c.point, 1) {
		c.ctx.ClosePath()
	}
	// The baseline of an area is traversed right-to-left, so the step must be
	// mirrored to land on the same x positions as the topline.
	if c.line >= 0 {
		c.t = 1 - c.t
		c.flip()
	}
}
func (c *curveStep) Point(x, y float64) {
	switch c.point {
	case 0:
		c.point = 1
		c.moveOrLine(c.ctx, x, y)
	default:
		if c.point == 1 {
			c.point = 2
		}
		if c.t <= 0 {
			c.ctx.LineTo(c.x, y)
			c.ctx.LineTo(x, y)
		} else {
			x1 := c.x*(1-c.t) + x*c.t
			c.ctx.LineTo(x1, c.y)
			c.ctx.LineTo(x1, y)
		}
	}
	c.x, c.y = x, y
}

// ---------------------------------------------------------------------------
// Radial
// ---------------------------------------------------------------------------

type curveRadial struct{ inner Curve }

// CurveRadialFactory wraps a curve so that points arrive as (angle, radius)
// and are drawn in Cartesian space. Angle 0 points up and increases clockwise,
// which is the convention the whole package shares with [Arc] and [Pie].
//
// The transform happens per point rather than per segment, so a "straight"
// radial line between two angles is genuinely straight in Cartesian space, not
// an arc — the same approximation d3 makes.
func CurveRadialFactory(f CurveFactory) CurveFactory {
	return func(ctx Context) Curve { return &curveRadial{inner: f(ctx)} }
}

func (c *curveRadial) AreaStart() { c.inner.AreaStart() }
func (c *curveRadial) AreaEnd()   { c.inner.AreaEnd() }
func (c *curveRadial) LineStart() { c.inner.LineStart() }
func (c *curveRadial) LineEnd()   { c.inner.LineEnd() }
func (c *curveRadial) Point(a, r float64) {
	c.inner.Point(r*math.Sin(a), r*-math.Cos(a))
}

// nan4 and nan6 spell the "no points seen yet" reset that every stateful curve
// performs in LineStart.
func nan4() (float64, float64, float64, float64) {
	n := math.NaN()
	return n, n, n, n
}

func nan6() (float64, float64, float64, float64, float64, float64) {
	n := math.NaN()
	return n, n, n, n, n, n
}
