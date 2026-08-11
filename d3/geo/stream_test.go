package geo

import (
	"fmt"
	"strings"
	"testing"
)

// recorder logs the stream events it receives, so the event sequence itself can
// be asserted rather than only its numeric consequences.
type recorder struct{ events []string }

func (r *recorder) Point(x, y, z float64) {
	r.events = append(r.events, fmt.Sprintf("point(%g,%g,%g)", x, y, z))
}
func (r *recorder) LineStart()    { r.events = append(r.events, "lineStart") }
func (r *recorder) LineEnd()      { r.events = append(r.events, "lineEnd") }
func (r *recorder) PolygonStart() { r.events = append(r.events, "polygonStart") }
func (r *recorder) PolygonEnd()   { r.events = append(r.events, "polygonEnd") }
func (r *recorder) Sphere()       { r.events = append(r.events, "sphere") }
func (r *recorder) String() string {
	return strings.Join(r.events, " ")
}

func record(o Object) string {
	r := &recorder{}
	StreamObject(o, r)
	return r.String()
}

func TestStreamEventSequences(t *testing.T) {
	cases := []struct {
		name string
		o    Object
		want string
	}{
		{
			"Point",
			&Point{Coordinates: P(1, 2)},
			"point(1,2,NaN)",
		},
		{
			"Point with elevation",
			&Point{Coordinates: Position{1, 2, 3}},
			"point(1,2,3)",
		},
		{
			"MultiPoint",
			&MultiPoint{Coordinates: []Position{P(1, 2), P(3, 4)}},
			"point(1,2,NaN) point(3,4,NaN)",
		},
		{
			"LineString",
			&LineString{Coordinates: []Position{P(0, 0), P(1, 1)}},
			"lineStart point(0,0,NaN) point(1,1,NaN) lineEnd",
		},
		{
			"MultiLineString",
			&MultiLineString{Coordinates: [][]Position{{P(0, 0)}, {P(1, 1)}}},
			"lineStart point(0,0,NaN) lineEnd lineStart point(1,1,NaN) lineEnd",
		},
		{
			// The closing position of each ring is dropped: a stream consumer
			// closes rings itself.
			"Polygon",
			&Polygon{Coordinates: [][]Position{{P(0, 0), P(0, 1), P(1, 1), P(0, 0)}}},
			"polygonStart lineStart point(0,0,NaN) point(0,1,NaN) point(1,1,NaN) lineEnd polygonEnd",
		},
		{
			"MultiPolygon",
			&MultiPolygon{Coordinates: [][][]Position{{{P(0, 0), P(0, 0)}}, {{P(1, 1), P(1, 1)}}}},
			"polygonStart lineStart point(0,0,NaN) lineEnd polygonEnd " +
				"polygonStart lineStart point(1,1,NaN) lineEnd polygonEnd",
		},
		{
			"GeometryCollection",
			&GeometryCollection{Geometries: []Geometry{
				&Point{Coordinates: P(1, 2)},
				&Sphere{},
			}},
			"point(1,2,NaN) sphere",
		},
		{
			"Sphere",
			&Sphere{},
			"sphere",
		},
		{
			"Feature",
			&Feature{Geometry: &Point{Coordinates: P(1, 2)}},
			"point(1,2,NaN)",
		},
		{
			"FeatureCollection",
			&FeatureCollection{Features: []*Feature{
				{Geometry: &Point{Coordinates: P(1, 2)}},
				{Geometry: nil},
			}},
			"point(1,2,NaN)",
		},
	}
	for _, c := range cases {
		if got := record(c.o); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestStreamNilObject(t *testing.T) {
	r := &recorder{}
	StreamObject(nil, r)
	if len(r.events) != 0 {
		t.Errorf("nil object emitted %v", r.events)
	}
}

func TestTransformPassesEverythingThrough(t *testing.T) {
	r := &recorder{}
	s := NewTransform(TransformFuncs{})(r)
	StreamObject(&Polygon{Coordinates: [][]Position{{P(0, 0), P(1, 1), P(2, 2), P(0, 0)}}}, s)
	want := record(&Polygon{Coordinates: [][]Position{{P(0, 0), P(1, 1), P(2, 2), P(0, 0)}}})
	if got := r.String(); got != want {
		t.Errorf("empty transform changed the stream:\n got %s\nwant %s", got, want)
	}
}

func TestTransformOverridesOneEvent(t *testing.T) {
	r := &recorder{}
	swap := NewTransform(TransformFuncs{
		Point: func(s Stream, x, y, z float64) { s.Point(y, x, z) },
	})(r)
	StreamObject(&LineString{Coordinates: []Position{P(1, 2), P(3, 4)}}, swap)
	want := "lineStart point(2,1,NaN) point(4,3,NaN) lineEnd"
	if got := r.String(); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

func TestTransformsCompose(t *testing.T) {
	// Two transforms wrapping one another must apply outside-in, which is what
	// lets a projection be assembled from a chain of them.
	r := &recorder{}
	double := NewTransform(TransformFuncs{
		Point: func(s Stream, x, y, z float64) { s.Point(x*2, y*2, z) },
	})
	shift := NewTransform(TransformFuncs{
		Point: func(s Stream, x, y, z float64) { s.Point(x+1, y+1, z) },
	})
	StreamObject(&Point{Coordinates: P(1, 1)}, shift(double(r)))
	if got := r.String(); got != "point(4,4,NaN)" {
		t.Errorf("got %s, want point(4,4,NaN) — (1+1)*2", got)
	}
}
