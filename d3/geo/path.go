package geo

import (
	"math"

	"github.com/malcolmston/d3/path"
)

// --- planar consumers --------------------------------------------------------

// pathBounds accumulates the axis-aligned bounding box of projected geometry.
type pathBounds struct {
	x0, y0, x1, y1 float64
}

func (b *pathBounds) reset() {
	b.x0, b.y0 = math.Inf(1), math.Inf(1)
	b.x1, b.y1 = math.Inf(-1), math.Inf(-1)
}

func (b *pathBounds) Point(x, y, _ float64) {
	if x < b.x0 {
		b.x0 = x
	}
	if x > b.x1 {
		b.x1 = x
	}
	if y < b.y0 {
		b.y0 = y
	}
	if y > b.y1 {
		b.y1 = y
	}
}
func (b *pathBounds) LineStart()    {}
func (b *pathBounds) LineEnd()      {}
func (b *pathBounds) PolygonStart() {}
func (b *pathBounds) PolygonEnd()   {}
func (b *pathBounds) Sphere()       {}

// pathArea is the shoelace formula. The signed sums of a polygon's rings
// accumulate together — so a hole, wound the other way, subtracts — and the
// absolute value is taken once per polygon, so a whole polygon wound backwards
// still reports a positive area. Each member of a MultiPolygon is its own
// polygon and contributes its own absolute value.
type pathArea struct {
	sum, ringSum adder
	x00, y00     float64
	x0, y0       float64
	inRing       bool
	first        bool
}

func (a *pathArea) PolygonStart() { a.inRing = true }
func (a *pathArea) PolygonEnd() {
	a.sum.Add(math.Abs(a.ringSum.Value()))
	a.ringSum.Reset()
	a.inRing = false
}
func (a *pathArea) LineStart() {
	if a.inRing {
		a.first = true
	}
}
func (a *pathArea) LineEnd() {
	if a.inRing {
		a.point(a.x00, a.y00)
	}
}
func (a *pathArea) Point(x, y, _ float64) {
	if !a.inRing {
		return
	}
	if a.first {
		a.first = false
		a.x00, a.x0 = x, x
		a.y00, a.y0 = y, y
		return
	}
	a.point(x, y)
}
func (a *pathArea) point(x, y float64) {
	a.ringSum.Add(a.y0*x - a.x0*y)
	a.x0, a.y0 = x, y
}
func (a *pathArea) Sphere() {}

func (a *pathArea) result() float64 { return a.sum.Value() / 2 }

// pathCentroid weights by area where there is area, by length where there is
// only a line, and by count where there are only points — the same three-level
// fallback as the spherical [Centroid], for the same reason.
type pathCentroid struct {
	x0s, y0s, z0s float64 // point-weighted
	x1s, y1s, z1s float64 // length-weighted
	x2s, y2s, z2s float64 // area-weighted
	x00, y00      float64
	xp, yp        float64
	mode          int // 0 points, 1 line, 2 ring
	first         bool
}

func (c *pathCentroid) Sphere()       {}
func (c *pathCentroid) PolygonStart() { c.mode = 2 }
func (c *pathCentroid) PolygonEnd()   { c.mode = 0 }
func (c *pathCentroid) LineStart() {
	if c.mode != 2 {
		c.mode = 1
	}
	c.first = true
}
func (c *pathCentroid) LineEnd() {
	if c.mode == 2 {
		c.ringPoint(c.x00, c.y00)
		c.first = true
		return
	}
	c.mode = 0
}

func (c *pathCentroid) Point(x, y, _ float64) {
	switch c.mode {
	case 2:
		if c.first {
			c.first = false
			c.x00, c.y00 = x, y
			c.xp, c.yp = x, y
			c.addPoint(x, y)
			return
		}
		c.ringPoint(x, y)
	case 1:
		if c.first {
			c.first = false
			c.xp, c.yp = x, y
			c.addPoint(x, y)
			return
		}
		c.linePoint(x, y)
	default:
		c.addPoint(x, y)
	}
}

func (c *pathCentroid) addPoint(x, y float64) {
	c.x0s += x
	c.y0s += y
	c.z0s++
}

func (c *pathCentroid) linePoint(x, y float64) {
	dx, dy := x-c.xp, y-c.yp
	z := math.Sqrt(dx*dx + dy*dy)
	c.x1s += z * (c.xp + x) / 2
	c.y1s += z * (c.yp + y) / 2
	c.z1s += z
	c.xp, c.yp = x, y
	c.addPoint(x, y)
}

func (c *pathCentroid) ringPoint(x, y float64) {
	dx, dy := x-c.xp, y-c.yp
	z := math.Sqrt(dx*dx + dy*dy)
	c.x1s += z * (c.xp + x) / 2
	c.y1s += z * (c.yp + y) / 2
	c.z1s += z
	z = c.yp*x - c.xp*y
	c.x2s += z * (c.xp + x)
	c.y2s += z * (c.yp + y)
	c.z2s += z * 3
	c.xp, c.yp = x, y
	c.addPoint(x, y)
}

func (c *pathCentroid) result() (float64, float64) {
	switch {
	case c.z2s != 0:
		return c.x2s / c.z2s, c.y2s / c.z2s
	case c.z1s != 0:
		return c.x1s / c.z1s, c.y1s / c.z1s
	case c.z0s != 0:
		return c.x0s / c.z0s, c.y0s / c.z0s
	}
	return math.NaN(), math.NaN()
}

// pathMeasure is the perimeter of projected geometry: line length for lines,
// ring perimeter for polygons.
type pathMeasure struct {
	sum      adder
	x00, y00 float64
	xp, yp   float64
	inRing   bool
	inLine   bool
	first    bool
}

func (m *pathMeasure) PolygonStart() { m.inRing = true }
func (m *pathMeasure) PolygonEnd()   { m.inRing = false }
func (m *pathMeasure) Sphere()       {}
func (m *pathMeasure) LineStart()    { m.inLine, m.first = true, true }
func (m *pathMeasure) LineEnd() {
	if m.inRing && !m.first {
		m.point(m.x00, m.y00)
	}
	m.inLine = false
}
func (m *pathMeasure) Point(x, y, _ float64) {
	if !m.inLine {
		return
	}
	if m.first {
		m.first = false
		m.x00, m.xp = x, x
		m.y00, m.yp = y, y
		return
	}
	m.point(x, y)
}
func (m *pathMeasure) point(x, y float64) {
	dx, dy := x-m.xp, y-m.yp
	m.sum.Add(math.Sqrt(dx*dx + dy*dy))
	m.xp, m.yp = x, y
}

// pathContext writes stream events into a d3/path.Path.
type pathContext struct {
	p      *path.Path
	radius float64
	line   int // 0 inside a polygon, -1 otherwise
	point  int // 0 expect move, 1 expect line, -1 standalone point
}

func newPathContext(p *path.Path, radius float64) *pathContext {
	return &pathContext{p: p, radius: radius, line: -1, point: -1}
}

func (c *pathContext) PolygonStart() { c.line = 0 }
func (c *pathContext) PolygonEnd()   { c.line = -1 }
func (c *pathContext) LineStart()    { c.point = 0 }
func (c *pathContext) LineEnd() {
	if c.line == 0 {
		c.p.ClosePath()
	}
	c.point = -1
}
func (c *pathContext) Sphere() {}

func (c *pathContext) Point(x, y, _ float64) {
	switch c.point {
	case 0:
		c.p.MoveTo(x, y)
		c.point = 1
	case 1:
		c.p.LineTo(x, y)
	default:
		// A standalone point is drawn as a circle of the configured radius,
		// which is what makes a scatter of cities visible at all.
		c.p.MoveTo(x+c.radius, y)
		c.p.Arc(x, y, c.radius, 0, tau, false)
	}
}

// --- Path --------------------------------------------------------------------

// Path renders GeoJSON through a projection into SVG path data, and measures
// the projected result.
//
// It is d3.geoPath minus the canvas: [Path.Generate] returns a string, as every
// generator in this port does.
//
//	p := geo.NewPath().Projection(geo.NewEqualEarth().FitSize(960, 500, land))
//	d := p.Generate(land)
//
// With no projection set, coordinates pass through unchanged, which is the
// right choice for data that is already projected (see also [Identity], which
// adds scale, translate and clipping to that).
//
// A configured Path is a pure function from an object to a string: each call
// builds its own path and its own stream, so it is safe to call concurrently.
type Path struct {
	projection  Projector
	pointRadius float64
	digits      int
}

// NewPath returns a Path with no projection and d3's default point radius
// of 4.5.
func NewPath() *Path { return &Path{pointRadius: 4.5, digits: -1} }

// Projection sets the projection to render through. A nil projection passes
// coordinates through unchanged.
func (p *Path) Projection(proj Projector) *Path { p.projection = proj; return p }

// PointRadius sets the radius, in output units, of the circle drawn for a
// standalone Point or MultiPoint.
func (p *Path) PointRadius(r float64) *Path { p.pointRadius = r; return p }

// Digits rounds every emitted coordinate to n decimal places, trading
// sub-pixel accuracy for markedly smaller path strings. Negative n restores
// full precision.
func (p *Path) Digits(n int) *Path { p.digits = n; return p }

func (p *Path) newPath() *path.Path {
	if p.digits < 0 {
		return path.New()
	}
	return path.NewWithPrecision(p.digits)
}

func (p *Path) sink(s Stream) Stream {
	if p.projection == nil {
		return s
	}
	return p.projection.Stream(s)
}

// Generate returns the SVG path data for an object, or "" when nothing is
// drawn — check for the empty string before emitting a <path>, exactly where
// you would have checked for null in d3.
func (p *Path) Generate(o Object) string {
	pb := p.newPath()
	ctx := newPathContext(pb, p.pointRadius)
	StreamObject(o, p.sink(ctx))
	return pb.String()
}

// Area returns the projected area of an object in square output units.
//
// Holes subtract, because a polygon's rings share one signed accumulator; the
// result is then made positive per polygon, so the winding of the whole polygon
// does not matter. Lines and points have no area.
func (p *Path) Area(o Object) float64 {
	var a pathArea
	StreamObject(o, p.sink(&a))
	return a.result()
}

// Bounds returns the projected bounding box as (x0, y0, x1, y1). An object that
// projects to nothing gives (+Inf, +Inf, -Inf, -Inf), the identity of the
// min/max fold, which is the value to test if you need to know.
func (p *Path) Bounds(o Object) (x0, y0, x1, y1 float64) {
	var b pathBounds
	b.reset()
	StreamObject(o, p.sink(&b))
	return b.x0, b.y0, b.x1, b.y1
}

// Centroid returns the projected centroid — area-weighted for polygons,
// length-weighted for lines, the mean position for points. This is where a
// label goes.
func (p *Path) Centroid(o Object) (x, y float64) {
	var c pathCentroid
	StreamObject(o, p.sink(&c))
	return c.result()
}

// Measure returns the projected length of an object's lines, or the perimeter
// of its rings, in output units.
func (p *Path) Measure(o Object) float64 {
	var m pathMeasure
	StreamObject(o, p.sink(&m))
	return m.sum.Value()
}
