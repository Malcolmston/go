package geo

// Stream is the sink that every geographic operation in this package writes
// into, and the interface that makes projection, clipping and drawing compose.
//
// A GeoJSON object is pushed through a stream as a sequence of events rather
// than converted into a new object:
//
//	PolygonStart LineStart Point… LineEnd LineStart Point… LineEnd PolygonEnd
//
// The rules are d3's:
//
//   - Point outside any line is a standalone point (a marker).
//   - LineStart/LineEnd bracket an open path.
//   - PolygonStart/PolygonEnd bracket one or more rings, each of which arrives
//     as a LineStart/LineEnd pair with its closing point omitted — a consumer
//     that needs a closed ring closes it itself.
//   - Sphere means "the entire globe", d3's extension used to draw the outline
//     of a clipped world.
//
// The z argument to Point is the optional third coordinate of a GeoJSON
// position (usually elevation). Most consumers ignore it; the clipping streams
// reuse the slot to mark points they synthesized at a clip boundary, which is
// why it is part of the interface rather than dropped.
//
// A Stream is stateful and single-threaded. Wrap, do not share.
type Stream interface {
	Point(x, y, z float64)
	LineStart()
	LineEnd()
	PolygonStart()
	PolygonEnd()
	Sphere()
}

// funcStream adapts a set of optional closures to a Stream, which is how d3
// writes every one of its streams (an object literal whose members are
// reassigned to switch state). A nil field is a no-op, matching the fact that
// d3's streams simply omit the methods they do not need.
type funcStream struct {
	point        func(x, y, z float64)
	lineStart    func()
	lineEnd      func()
	polygonStart func()
	polygonEnd   func()
	sphere       func()
}

func (s *funcStream) Point(x, y, z float64) {
	if s.point != nil {
		s.point(x, y, z)
	}
}
func (s *funcStream) LineStart() {
	if s.lineStart != nil {
		s.lineStart()
	}
}
func (s *funcStream) LineEnd() {
	if s.lineEnd != nil {
		s.lineEnd()
	}
}
func (s *funcStream) PolygonStart() {
	if s.polygonStart != nil {
		s.polygonStart()
	}
}
func (s *funcStream) PolygonEnd() {
	if s.polygonEnd != nil {
		s.polygonEnd()
	}
}
func (s *funcStream) Sphere() {
	if s.sphere != nil {
		s.sphere()
	}
}

// TransformFuncs describes a stream that intercepts some events and forwards
// the rest unchanged. It is the Go form of d3.geoTransform: supply only the
// handlers you want to override, and call the corresponding method on the
// downstream Stream (passed as the first argument) to forward.
//
//	// Round every projected coordinate to whole pixels.
//	round := geo.NewTransform(geo.TransformFuncs{
//		Point: func(s geo.Stream, x, y, z float64) {
//			s.Point(math.Round(x), math.Round(y), z)
//		},
//	})
//
// Go has no prototype to override, so the downstream stream is an explicit
// parameter where d3 writes `this.stream`.
type TransformFuncs struct {
	Point        func(s Stream, x, y, z float64)
	LineStart    func(s Stream)
	LineEnd      func(s Stream)
	PolygonStart func(s Stream)
	PolygonEnd   func(s Stream)
	Sphere       func(s Stream)
}

// NewTransform returns a stream wrapper: given a downstream Stream it produces
// a Stream that applies t's overrides and passes everything else through.
func NewTransform(t TransformFuncs) func(Stream) Stream {
	return func(down Stream) Stream {
		s := &funcStream{
			point:        func(x, y, z float64) { down.Point(x, y, z) },
			lineStart:    down.LineStart,
			lineEnd:      down.LineEnd,
			polygonStart: down.PolygonStart,
			polygonEnd:   down.PolygonEnd,
			sphere:       down.Sphere,
		}
		if t.Point != nil {
			s.point = func(x, y, z float64) { t.Point(down, x, y, z) }
		}
		if t.LineStart != nil {
			s.lineStart = func() { t.LineStart(down) }
		}
		if t.LineEnd != nil {
			s.lineEnd = func() { t.LineEnd(down) }
		}
		if t.PolygonStart != nil {
			s.polygonStart = func() { t.PolygonStart(down) }
		}
		if t.PolygonEnd != nil {
			s.polygonEnd = func() { t.PolygonEnd(down) }
		}
		if t.Sphere != nil {
			s.sphere = func() { t.Sphere(down) }
		}
		return s
	}
}

// StreamObject pushes a GeoJSON object through a stream. It is d3.geoStream,
// and it is how every consumer in this package — area, bounds, centroid,
// [Path] — is fed.
//
// A nil object emits nothing.
func StreamObject(o Object, s Stream) {
	if o == nil {
		return
	}
	o.stream(s)
}

// --- streaming of each object type -------------------------------------------

func streamLine(coords []Position, s Stream, closed int) {
	n := len(coords) - closed
	s.LineStart()
	for i := 0; i < n; i++ {
		c := coords[i]
		s.Point(c.Lon(), c.Lat(), c.Alt())
	}
	s.LineEnd()
}

func streamPolygon(rings [][]Position, s Stream) {
	s.PolygonStart()
	for _, ring := range rings {
		// closed=1: the repeated final position of a GeoJSON ring is dropped,
		// because a stream consumer closes rings itself.
		streamLine(ring, s, 1)
	}
	s.PolygonEnd()
}

func (g *Point) stream(s Stream) {
	c := g.Coordinates
	s.Point(c.Lon(), c.Lat(), c.Alt())
}

func (g *MultiPoint) stream(s Stream) {
	for _, c := range g.Coordinates {
		s.Point(c.Lon(), c.Lat(), c.Alt())
	}
}

func (g *LineString) stream(s Stream) { streamLine(g.Coordinates, s, 0) }

func (g *MultiLineString) stream(s Stream) {
	for _, l := range g.Coordinates {
		streamLine(l, s, 0)
	}
}

func (g *Polygon) stream(s Stream) { streamPolygon(g.Coordinates, s) }

func (g *MultiPolygon) stream(s Stream) {
	for _, p := range g.Coordinates {
		streamPolygon(p, s)
	}
}

func (g *GeometryCollection) stream(s Stream) {
	for _, child := range g.Geometries {
		if child != nil {
			child.stream(s)
		}
	}
}

func (g *Sphere) stream(s Stream) { s.Sphere() }

func (f *Feature) stream(s Stream) {
	if f.Geometry != nil {
		f.Geometry.stream(s)
	}
}

func (fc *FeatureCollection) stream(s Stream) {
	for _, f := range fc.Features {
		if f != nil {
			f.stream(s)
		}
	}
}
