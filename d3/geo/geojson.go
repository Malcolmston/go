package geo

import (
	"encoding/json"
	"fmt"
	"math"
)

// Position is a GeoJSON position: longitude, latitude, and optionally an
// elevation, in that order (RFC 7946 §3.1.1). Longitude comes first, which
// surprises people who expect "lat, lon" — it is x before y, and it is what
// every GeoJSON file on disk contains.
//
// It is a slice rather than a fixed-size array so that a two-element position
// round-trips through JSON as two elements and a three-element one as three;
// RFC 7946 forbids inventing or dropping a coordinate.
type Position []float64

// P builds a two-element Position.
func P(lon, lat float64) Position { return Position{lon, lat} }

// Lon returns the longitude in degrees, or NaN if the position is empty.
func (p Position) Lon() float64 { return p.at(0) }

// Lat returns the latitude in degrees, or NaN if the position has no latitude.
func (p Position) Lat() float64 { return p.at(1) }

// Alt returns the elevation, or NaN if the position carries none.
func (p Position) Alt() float64 { return p.at(2) }

func (p Position) at(i int) float64 {
	if i < len(p) {
		return p[i]
	}
	return math.NaN()
}

// Object is any GeoJSON object this package can stream: a [Geometry], a
// [Feature] or a [FeatureCollection].
type Object interface {
	// GeoJSONType returns the value of the object's "type" member.
	GeoJSONType() string
	// stream pushes the object through s. It is unexported because the entry
	// point is [StreamObject], which mirrors d3.geoStream.
	stream(s Stream)
}

// Geometry is a GeoJSON geometry object.
//
// The concrete types are [Point], [MultiPoint], [LineString],
// [MultiLineString], [Polygon], [MultiPolygon] and [GeometryCollection], plus
// [Sphere] — d3's extension for "the whole globe", which is not in RFC 7946 but
// is what draws the outline of an orthographic projection.
type Geometry interface {
	Object
	geometry()
}

// --- concrete geometries -----------------------------------------------------

// Point is a single position.
type Point struct{ Coordinates Position }

// MultiPoint is an unordered collection of positions.
type MultiPoint struct{ Coordinates []Position }

// LineString is an open path of two or more positions.
type LineString struct{ Coordinates []Position }

// MultiLineString is a collection of LineString coordinate arrays.
type MultiLineString struct{ Coordinates [][]Position }

// Polygon is a list of linear rings: the first is the exterior ring, the rest
// are holes. Each ring's first and last position must be identical.
//
// Winding order is load-bearing and, importantly, d3's convention is the
// *opposite* of RFC 7946's. On a sphere a closed ring divides the surface into
// two finite regions and nothing decides which is the inside except the
// direction you walk it. d3-geo — and therefore this package — puts the
// interior on the right: an exterior ring smaller than a hemisphere winds
// **clockwise**, holes wind counterclockwise. RFC 7946 §3.1.6 says the reverse.
// TopoJSON and most d3 map data already follow d3's convention; data straight
// from a spec-conforming GeoJSON writer may not, and a ring wound the other way
// describes the complement of the region you meant — [Area] will report
// 4π minus what you expected, and [Contains] will be inverted.
//
// Reverse the ring to fix it. Nothing here rewinds automatically, because
// guessing would silently corrupt the cases that are deliberately larger than a
// hemisphere.
type Polygon struct{ Coordinates [][]Position }

// MultiPolygon is a collection of Polygon coordinate arrays.
type MultiPolygon struct{ Coordinates [][][]Position }

// GeometryCollection is a heterogeneous collection of geometries.
type GeometryCollection struct{ Geometries []Geometry }

// Sphere is the whole globe. It is d3's extension, not part of RFC 7946: it has
// no coordinates, and streaming it emits a single Sphere() call, which a
// clipping stream turns into the outline of the visible hemisphere. It is how
// you draw the disc of an orthographic map or the frame of a world map.
type Sphere struct{}

func (*Point) geometry()              {}
func (*MultiPoint) geometry()         {}
func (*LineString) geometry()         {}
func (*MultiLineString) geometry()    {}
func (*Polygon) geometry()            {}
func (*MultiPolygon) geometry()       {}
func (*GeometryCollection) geometry() {}
func (*Sphere) geometry()             {}

func (*Point) GeoJSONType() string              { return "Point" }
func (*MultiPoint) GeoJSONType() string         { return "MultiPoint" }
func (*LineString) GeoJSONType() string         { return "LineString" }
func (*MultiLineString) GeoJSONType() string    { return "MultiLineString" }
func (*Polygon) GeoJSONType() string            { return "Polygon" }
func (*MultiPolygon) GeoJSONType() string       { return "MultiPolygon" }
func (*GeometryCollection) GeoJSONType() string { return "GeometryCollection" }
func (*Sphere) GeoJSONType() string             { return "Sphere" }

// Feature is a geometry with associated properties.
//
// ID is `any` because RFC 7946 allows a string or a number and says nothing
// more; it is nil when absent. Geometry may be nil, which the RFC explicitly
// permits ("unlocated feature") and which the stream treats as nothing to draw.
type Feature struct {
	ID         any
	Properties map[string]any
	Geometry   Geometry
	BBox       []float64
}

// GeoJSONType returns "Feature".
func (*Feature) GeoJSONType() string { return "Feature" }

// FeatureCollection is a list of features.
type FeatureCollection struct {
	Features []*Feature
	BBox     []float64
}

// GeoJSONType returns "FeatureCollection".
func (*FeatureCollection) GeoJSONType() string { return "FeatureCollection" }

// --- JSON --------------------------------------------------------------------

type rawGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates,omitempty"`
	Geometries  json.RawMessage `json:"geometries,omitempty"`
}

// MarshalJSON implements json.Marshaler.
func (g *Point) MarshalJSON() ([]byte, error) { return marshalCoords("Point", g.Coordinates) }

// MarshalJSON implements json.Marshaler.
func (g *MultiPoint) MarshalJSON() ([]byte, error) { return marshalCoords("MultiPoint", g.Coordinates) }

// MarshalJSON implements json.Marshaler.
func (g *LineString) MarshalJSON() ([]byte, error) { return marshalCoords("LineString", g.Coordinates) }

// MarshalJSON implements json.Marshaler.
func (g *MultiLineString) MarshalJSON() ([]byte, error) {
	return marshalCoords("MultiLineString", g.Coordinates)
}

// MarshalJSON implements json.Marshaler.
func (g *Polygon) MarshalJSON() ([]byte, error) { return marshalCoords("Polygon", g.Coordinates) }

// MarshalJSON implements json.Marshaler.
func (g *MultiPolygon) MarshalJSON() ([]byte, error) {
	return marshalCoords("MultiPolygon", g.Coordinates)
}

// MarshalJSON implements json.Marshaler.
func (g *Sphere) MarshalJSON() ([]byte, error) { return []byte(`{"type":"Sphere"}`), nil }

// MarshalJSON implements json.Marshaler.
func (g *GeometryCollection) MarshalJSON() ([]byte, error) {
	geoms := g.Geometries
	if geoms == nil {
		geoms = []Geometry{}
	}
	return json.Marshal(struct {
		Type       string     `json:"type"`
		Geometries []Geometry `json:"geometries"`
	}{"GeometryCollection", geoms})
}

// UnmarshalJSON implements json.Unmarshaler.
//
// The other geometries need no custom decoder — their coordinate fields are
// concrete — but a collection's members are the [Geometry] interface, which
// encoding/json cannot construct on its own.
func (g *GeometryCollection) UnmarshalJSON(data []byte) error {
	child, err := UnmarshalGeometry(data)
	if err != nil {
		return err
	}
	gc, ok := child.(*GeometryCollection)
	if !ok {
		return fmt.Errorf("geo: expected GeometryCollection, got %s", child.GeoJSONType())
	}
	*g = *gc
	return nil
}

func marshalCoords(typ string, coords any) ([]byte, error) {
	return json.Marshal(struct {
		Type        string `json:"type"`
		Coordinates any    `json:"coordinates"`
	}{typ, coords})
}

// UnmarshalGeometry decodes a single GeoJSON geometry, dispatching on its
// "type" member. An unrecognized type is an error rather than a silently
// dropped shape.
func UnmarshalGeometry(data []byte) (Geometry, error) {
	var raw rawGeometry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	switch raw.Type {
	case "Point":
		g := &Point{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "MultiPoint":
		g := &MultiPoint{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "LineString":
		g := &LineString{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "MultiLineString":
		g := &MultiLineString{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "Polygon":
		g := &Polygon{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "MultiPolygon":
		g := &MultiPolygon{}
		return g, unmarshalCoords(raw.Coordinates, &g.Coordinates)
	case "Sphere":
		return &Sphere{}, nil
	case "GeometryCollection":
		g := &GeometryCollection{}
		if len(raw.Geometries) == 0 {
			return g, nil
		}
		var parts []json.RawMessage
		if err := json.Unmarshal(raw.Geometries, &parts); err != nil {
			return nil, err
		}
		for _, p := range parts {
			child, err := UnmarshalGeometry(p)
			if err != nil {
				return nil, err
			}
			g.Geometries = append(g.Geometries, child)
		}
		return g, nil
	case "":
		return nil, fmt.Errorf("geo: geometry has no type")
	default:
		return nil, fmt.Errorf("geo: unknown geometry type %q", raw.Type)
	}
}

func unmarshalCoords(data json.RawMessage, dst any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dst)
}

// MarshalJSON implements json.Marshaler.
func (f *Feature) MarshalJSON() ([]byte, error) {
	out := map[string]any{"type": "Feature", "geometry": f.Geometry}
	if f.Properties != nil {
		out["properties"] = f.Properties
	} else {
		out["properties"] = nil
	}
	if f.ID != nil {
		out["id"] = f.ID
	}
	if f.BBox != nil {
		out["bbox"] = f.BBox
	}
	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *Feature) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type       string          `json:"type"`
		ID         any             `json:"id"`
		Properties map[string]any  `json:"properties"`
		Geometry   json.RawMessage `json:"geometry"`
		BBox       []float64       `json:"bbox"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Type != "" && raw.Type != "Feature" {
		return fmt.Errorf("geo: expected Feature, got %q", raw.Type)
	}
	f.ID, f.Properties, f.BBox = raw.ID, raw.Properties, raw.BBox
	f.Geometry = nil
	if len(raw.Geometry) > 0 && string(raw.Geometry) != "null" {
		g, err := UnmarshalGeometry(raw.Geometry)
		if err != nil {
			return err
		}
		f.Geometry = g
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (fc *FeatureCollection) MarshalJSON() ([]byte, error) {
	features := fc.Features
	if features == nil {
		features = []*Feature{}
	}
	out := map[string]any{"type": "FeatureCollection", "features": features}
	if fc.BBox != nil {
		out["bbox"] = fc.BBox
	}
	return json.Marshal(out)
}

// UnmarshalJSON implements json.Unmarshaler.
func (fc *FeatureCollection) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type     string     `json:"type"`
		Features []*Feature `json:"features"`
		BBox     []float64  `json:"bbox"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Type != "" && raw.Type != "FeatureCollection" {
		return fmt.Errorf("geo: expected FeatureCollection, got %q", raw.Type)
	}
	fc.Features, fc.BBox = raw.Features, raw.BBox
	return nil
}

// Unmarshal decodes any GeoJSON object — a geometry, a Feature or a
// FeatureCollection — into the corresponding Go value. It is the entry point
// when you do not know in advance which of the three a file holds.
func Unmarshal(data []byte) (Object, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}
	switch probe.Type {
	case "Feature":
		f := &Feature{}
		return f, json.Unmarshal(data, f)
	case "FeatureCollection":
		fc := &FeatureCollection{}
		return fc, json.Unmarshal(data, fc)
	default:
		return UnmarshalGeometry(data)
	}
}
