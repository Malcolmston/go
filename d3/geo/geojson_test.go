package geo

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGeoJSONRoundTrip(t *testing.T) {
	cases := []string{
		`{"type":"Point","coordinates":[1,2]}`,
		`{"type":"Point","coordinates":[1,2,3]}`,
		`{"type":"MultiPoint","coordinates":[[1,2],[3,4]]}`,
		`{"type":"LineString","coordinates":[[1,2],[3,4],[5,6]]}`,
		`{"type":"MultiLineString","coordinates":[[[1,2],[3,4]],[[5,6],[7,8]]]}`,
		`{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}`,
		`{"type":"MultiPolygon","coordinates":[[[[0,0],[0,1],[1,1],[0,0]]]]}`,
		`{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[1,2]},{"type":"LineString","coordinates":[[0,0],[1,1]]}]}`,
		`{"type":"Sphere"}`,
	}
	for _, in := range cases {
		g, err := UnmarshalGeometry([]byte(in))
		if err != nil {
			t.Fatalf("UnmarshalGeometry(%s): %v", in, err)
		}
		out, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", in, err)
		}
		if string(out) != in {
			t.Errorf("round trip\n got %s\nwant %s", out, in)
		}
	}
}

func TestGeoJSONThirdCoordinatePreserved(t *testing.T) {
	g, err := UnmarshalGeometry([]byte(`{"type":"Point","coordinates":[1,2,3]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := g.(*Point)
	if len(p.Coordinates) != 3 || p.Coordinates.Alt() != 3 {
		t.Fatalf("elevation lost: %v", p.Coordinates)
	}
	// A two-element position must not gain a third on the way back out.
	two, _ := json.Marshal(&Point{Coordinates: P(1, 2)})
	if string(two) != `{"type":"Point","coordinates":[1,2]}` {
		t.Errorf("two-element position marshalled as %s", two)
	}
}

func TestGeoJSONUnknownType(t *testing.T) {
	if _, err := UnmarshalGeometry([]byte(`{"type":"Triangle","coordinates":[]}`)); err == nil {
		t.Fatal("want an error for an unknown geometry type")
	}
	if _, err := UnmarshalGeometry([]byte(`{"coordinates":[]}`)); err == nil {
		t.Fatal("want an error for a missing type")
	}
}

func TestFeatureRoundTrip(t *testing.T) {
	in := `{"type":"Feature","id":"x","properties":{"name":"a"},"geometry":{"type":"Point","coordinates":[1,2]}}`
	o, err := Unmarshal([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	f, ok := o.(*Feature)
	if !ok {
		t.Fatalf("got %T, want *Feature", o)
	}
	if f.ID != "x" || f.Properties["name"] != "a" {
		t.Errorf("id/properties lost: %v %v", f.ID, f.Properties)
	}
	if _, ok := f.Geometry.(*Point); !ok {
		t.Errorf("geometry is %T", f.Geometry)
	}
	out, _ := json.Marshal(f)
	var a, b map[string]any
	_ = json.Unmarshal([]byte(in), &a)
	_ = json.Unmarshal(out, &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("round trip\n got %v\nwant %v", b, a)
	}
}

func TestFeatureNullGeometry(t *testing.T) {
	// RFC 7946 permits an unlocated feature; it must decode and stream nothing.
	f := &Feature{}
	if err := json.Unmarshal([]byte(`{"type":"Feature","properties":null,"geometry":null}`), f); err != nil {
		t.Fatal(err)
	}
	if f.Geometry != nil {
		t.Errorf("geometry = %v, want nil", f.Geometry)
	}
	if got := NewPath().Generate(f); got != "" {
		t.Errorf("Generate = %q, want empty", got)
	}
}

func TestFeatureCollectionRoundTrip(t *testing.T) {
	in := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":null,"geometry":{"type":"Point","coordinates":[1,2]}}]}`
	o, err := Unmarshal([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	fc, ok := o.(*FeatureCollection)
	if !ok {
		t.Fatalf("got %T", o)
	}
	if len(fc.Features) != 1 {
		t.Fatalf("features = %d", len(fc.Features))
	}
	out, _ := json.Marshal(fc)
	if !strings.Contains(string(out), `"type":"FeatureCollection"`) {
		t.Errorf("marshalled as %s", out)
	}
}

func TestFeatureWrongType(t *testing.T) {
	f := &Feature{}
	if err := json.Unmarshal([]byte(`{"type":"Point","coordinates":[1,2]}`), f); err == nil {
		t.Fatal("want an error decoding a Point as a Feature")
	}
}

func TestGeometryCollectionDecodesDirectly(t *testing.T) {
	// A collection's members are an interface, so it needs its own decoder;
	// this checks that plain json.Unmarshal into the concrete type works too.
	gc := &GeometryCollection{}
	in := `{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[1,2]},{"type":"Sphere"}]}`
	if err := json.Unmarshal([]byte(in), gc); err != nil {
		t.Fatal(err)
	}
	if len(gc.Geometries) != 2 {
		t.Fatalf("got %d geometries", len(gc.Geometries))
	}
	if _, ok := gc.Geometries[0].(*Point); !ok {
		t.Errorf("first member is %T", gc.Geometries[0])
	}
	if _, ok := gc.Geometries[1].(*Sphere); !ok {
		t.Errorf("second member is %T", gc.Geometries[1])
	}
	out, _ := json.Marshal(gc)
	if string(out) != in {
		t.Errorf("round trip\n got %s\nwant %s", out, in)
	}
	if err := json.Unmarshal([]byte(`{"type":"Point","coordinates":[1,2]}`), &GeometryCollection{}); err == nil {
		t.Error("want an error decoding a Point as a GeometryCollection")
	}
}

func TestConcreteGeometriesDecodeDirectly(t *testing.T) {
	p := &Polygon{}
	if err := json.Unmarshal([]byte(`{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[0,0]]]}`), p); err != nil {
		t.Fatal(err)
	}
	if len(p.Coordinates) != 1 || len(p.Coordinates[0]) != 4 {
		t.Errorf("decoded %v", p.Coordinates)
	}
}
