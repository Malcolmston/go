# geo — Go port of d3-geo: spherical geometry, GeoJSON streaming, map projections, clipping

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/geo.svg)](https://pkg.go.dev/github.com/malcolmston/d3/geo)

Package geo is a Go port of d3-geo: spherical geometry, GeoJSON streaming, map
projections, clipping, and SVG path generation for geographic data.

The module has two halves that are useful independently.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/geo"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Area(o Object) float64` | Area returns the spherical area of a GeoJSON object in steradians. |
| `func Bounds(o Object) (lon0, lat0, lon1, lat1 float64)` | Bounds returns the spherical bounding box of a GeoJSON object as (lon0, lat0, lon1, lat1) in degrees: the southwest corner followed by the northeast… |
| `func Centroid(o Object) (lon, lat float64)` | Centroid returns the spherical centroid of a GeoJSON object, in degrees. |
| `func Contains(o Object, lon, lat float64) bool` | Contains reports whether a GeoJSON object contains the given point, in degrees. |
| `func Distance(lon0, lat0, lon1, lat1 float64) float64` | Distance returns the great-circle distance in radians between two points given in degrees. |
| `func Interpolate(lon0, lat0, lon1, lat1 float64) (func(t float64) (lon, lat float64), float64)` | Interpolate returns a function that walks the great-circle arc — the shortest path over the sphere — from one point to another, in degrees. |
| `func Length(o Object) float64` | Length returns the great-circle length of a GeoJSON object in radians: the length of its lines, and the perimeter of each of a polygon's rings —… |
| `func NewTransform(t TransformFuncs) func(Stream) Stream` | NewTransform returns a stream wrapper: given a downstream Stream it produces a Stream that applies t's overrides and passes everything else through. |
| `func StreamObject(o Object, s Stream)` | StreamObject pushes a GeoJSON object through a stream. |

### Types

| Type | What it is |
| --- | --- |
| `Circle` | Circle generates a GeoJSON polygon approximating a circle on the sphere — the set of points a fixed angular distance from a center. |
| `Feature` | Feature is a geometry with associated properties. |
| `FeatureCollection` | FeatureCollection is a list of features. |
| `Geometry` | Geometry is a GeoJSON geometry object. |
| `GeometryCollection` | GeometryCollection is a heterogeneous collection of geometries. |
| `Identity` | Identity is d3.geoIdentity: a "projection" that does no spherical mathematics at all, only scale, translate, reflect, rotate and clip. |
| `LineString` | LineString is an open path of two or more positions. |
| `MultiLineString` | MultiLineString is a collection of LineString coordinate arrays. |
| `MultiPoint` | MultiPoint is an unordered collection of positions. |
| `MultiPolygon` | MultiPolygon is a collection of Polygon coordinate arrays. |
| `Object` | Object is any GeoJSON object this package can stream: a `Geometry`, a `Feature` or a `FeatureCollection`. |
| `Path` | Path renders GeoJSON through a projection into SVG path data, and measures the projected result. |
| `Point` | Point is a single position. |
| `Polygon` | Polygon is a list of linear rings: the first is the exterior ring, the rest are holes. |
| `Position` | Position is a GeoJSON position: longitude, latitude, and optionally an elevation, in that order (RFC 7946 §3.1.1). |
| `Projection` | Projection turns spherical coordinates into planar ones, with the configuration a map needs around it. |
| `Projector` | Projector is anything that can wrap a `Stream` with its own transformation — a `Projection` or an `Identity`. |
| `Raw` | Raw is a raw projection: a pair of maps between spherical coordinates in *radians* and an abstract plane, before any scaling, translation or rotation. |
| `Rotation` | Rotation is d3.geoRotation: a rotation of the sphere expressed as three angles in degrees, applied in the order λ (about the poles), φ (about the… |
| `Sphere` | Sphere is the whole globe. |
| `Stream` | Stream is the sink that every geographic operation in this package writes into, and the interface that makes projection, clipping and drawing compose. |
| `TransformFuncs` | TransformFuncs describes a stream that intercepts some events and forwards the rest unchanged. |

<details>
<summary><code>Circle</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCircle() *Circle` | NewCircle returns a circle centered at (0, 0) with a radius of 90° and a precision of 6°, matching d3's defaults. |
| `func (c *Circle) Center(lon, lat float64) *Circle` | Center sets the circle's center in degrees. |
| `func (c *Circle) Generate() *Polygon` | Generate returns the circle as a single-ring `Polygon` in degrees. |
| `func (c *Circle) Precision(p float64) *Circle` | Precision sets the angular step between generated vertices, in degrees. |
| `func (c *Circle) Radius(r float64) *Circle` | Radius sets the angular radius in degrees. |

</details>

<details>
<summary><code>Feature</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*Feature) GeoJSONType() string` | GeoJSONType returns "Feature". |
| `func (f *Feature) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |
| `func (f *Feature) UnmarshalJSON(data []byte) error` | UnmarshalJSON implements json.Unmarshaler. |

</details>

<details>
<summary><code>FeatureCollection</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*FeatureCollection) GeoJSONType() string` | GeoJSONType returns "FeatureCollection". |
| `func (fc *FeatureCollection) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |
| `func (fc *FeatureCollection) UnmarshalJSON(data []byte) error` | UnmarshalJSON implements json.Unmarshaler. |

</details>

<details>
<summary><code>Geometry</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func UnmarshalGeometry(data []byte) (Geometry, error)` | UnmarshalGeometry decodes a single GeoJSON geometry, dispatching on its "type" member. |

</details>

<details>
<summary><code>GeometryCollection</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*GeometryCollection) GeoJSONType() string` | — |
| `func (g *GeometryCollection) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |
| `func (g *GeometryCollection) UnmarshalJSON(data []byte) error` | UnmarshalJSON implements json.Unmarshaler. |

</details>

<details>
<summary><code>Identity</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewIdentity() *Identity` | NewIdentity returns an identity transform: unit scale, no translation, no clipping. |
| `func (i *Identity) Angle(a float64) *Identity` | Angle sets a rotation of the plane, in degrees clockwise. |
| `func (i *Identity) ClearClipExtent() *Identity` | ClearClipExtent removes any clipping. |
| `func (i *Identity) ClipExtent(x0, y0, x1, y1 float64) *Identity` | ClipExtent clips output to an axis-aligned rectangle. |
| `func (i *Identity) FitExtent(x0, y0, x1, y1 float64, o Object) *Identity` | FitExtent sets scale and translate so the object fits the given rectangle. |
| `func (i *Identity) FitHeight(height float64, o Object) *Identity` | FitHeight fits the object's height. |
| `func (i *Identity) FitSize(width, height float64, o Object) *Identity` | FitSize is FitExtent anchored at the origin. |
| `func (i *Identity) FitWidth(width float64, o Object) *Identity` | FitWidth fits the object's width. |
| `func (i *Identity) Invert(px, py float64) (x, y float64)` | Invert undoes the transform. |
| `func (i *Identity) Project(px, py float64) (x, y float64)` | Project applies the transform. |
| `func (i *Identity) ReflectX(v bool) *Identity` | ReflectX mirrors horizontally. |
| `func (i *Identity) ReflectY(v bool) *Identity` | ReflectY mirrors vertically — the usual way to turn data whose y axis points up into SVG coordinates, whose y axis points down. |
| `func (i *Identity) Scale(k float64) *Identity` | Scale sets the scale factor. |
| `func (i *Identity) Stream(s Stream) Stream` | Stream implements `Projector`. |
| `func (i *Identity) Translate(x, y float64) *Identity` | Translate sets the translation in output units. |

</details>

<details>
<summary><code>LineString</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*LineString) GeoJSONType() string` | — |
| `func (g *LineString) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>MultiLineString</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*MultiLineString) GeoJSONType() string` | — |
| `func (g *MultiLineString) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>MultiPoint</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*MultiPoint) GeoJSONType() string` | — |
| `func (g *MultiPoint) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>MultiPolygon</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*MultiPolygon) GeoJSONType() string` | — |
| `func (g *MultiPolygon) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>Object</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Unmarshal(data []byte) (Object, error)` | Unmarshal decodes any GeoJSON object — a geometry, a Feature or a FeatureCollection — into the corresponding Go value. |

</details>

<details>
<summary><code>Path</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewPath() *Path` | NewPath returns a Path with no projection and d3's default point radius of 4.5. |
| `func (p *Path) Area(o Object) float64` | Area returns the projected area of an object in square output units. |
| `func (p *Path) Bounds(o Object) (x0, y0, x1, y1 float64)` | Bounds returns the projected bounding box as (x0, y0, x1, y1). |
| `func (p *Path) Centroid(o Object) (x, y float64)` | Centroid returns the projected centroid — area-weighted for polygons, length-weighted for lines, the mean position for points. |
| `func (p *Path) Digits(n int) *Path` | Digits rounds every emitted coordinate to n decimal places, trading sub-pixel accuracy for markedly smaller path strings. |
| `func (p *Path) Generate(o Object) string` | Generate returns the SVG path data for an object, or "" when nothing is drawn — check for the empty string before emitting a <path>, exactly where… |
| `func (p *Path) Measure(o Object) float64` | Measure returns the projected length of an object's lines, or the perimeter of its rings, in output units. |
| `func (p *Path) PointRadius(r float64) *Path` | PointRadius sets the radius, in output units, of the circle drawn for a standalone Point or MultiPoint. |
| `func (p *Path) Projection(proj Projector) *Path` | Projection sets the projection to render through. |

</details>

<details>
<summary><code>Point</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*Point) GeoJSONType() string` | — |
| `func (g *Point) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>Polygon</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*Polygon) GeoJSONType() string` | — |
| `func (g *Polygon) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

<details>
<summary><code>Position</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func P(lon, lat float64) Position` | P builds a two-element Position. |
| `func (p Position) Alt() float64` | Alt returns the elevation, or NaN if the position carries none. |
| `func (p Position) Lat() float64` | Lat returns the latitude in degrees, or NaN if the position has no latitude. |
| `func (p Position) Lon() float64` | Lon returns the longitude in degrees, or NaN if the position is empty. |

</details>

<details>
<summary><code>Projection</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewAzimuthalEqualArea() *Projection` | NewAzimuthalEqualArea returns Lambert's azimuthal equal-area projection. |
| `func NewAzimuthalEquidistant() *Projection` | NewAzimuthalEquidistant returns the azimuthal equidistant projection. |
| `func NewConicConformal() *Projection` | NewConicConformal returns Lambert's conformal conic projection, with both standard parallels at 30°. |
| `func NewConicEqualArea() *Projection` | NewConicEqualArea returns Albers' conic equal-area projection. |
| `func NewConicEquidistant() *Projection` | NewConicEquidistant returns the conic equidistant projection. |
| `func NewConicProjection(projectAt func(phi0, phi1 float64) Raw) *Projection` | NewConicProjection wraps a family of raw projections parameterized by two standard parallels; see `Projection.Parallels`. |
| `func NewEqualEarth() *Projection` | NewEqualEarth returns the Equal Earth projection. |
| `func NewEquirectangular() *Projection` | NewEquirectangular returns the equirectangular (plate carrée) projection. |
| `func NewGnomonic() *Projection` | NewGnomonic returns the gnomonic projection, clipped to 60°. |
| `func NewMercator() *Projection` | NewMercator returns the Mercator projection. |
| `func NewNaturalEarth1() *Projection` | NewNaturalEarth1 returns the Natural Earth projection. |
| `func NewOrthographic() *Projection` | NewOrthographic returns the orthographic projection, clipped to the visible hemisphere. |
| `func NewProjection(raw Raw) *Projection` | NewProjection wraps a raw projection with the standard configuration. |
| `func NewStereographic() *Projection` | NewStereographic returns the stereographic projection. |
| `func NewTransverseMercator() *Projection` | NewTransverseMercator returns the transverse Mercator projection. |
| `func (p *Projection) Angle(a float64) *Projection` | Angle sets a rotation of the projected plane itself, in degrees clockwise. |
| `func (p *Projection) Center(lon, lat float64) *Projection` | Center sets the point in degrees that lands at the translate position. |
| `func (p *Projection) ClearClipExtent() *Projection` | ClearClipExtent removes any viewport clipping. |
| `func (p *Projection) ClipAngle(a float64) *Projection` | ClipAngle sets the radius of the small circle, in degrees, outside which geometry is discarded before projection. |
| `func (p *Projection) ClipExtent(x0, y0, x1, y1 float64) *Projection` | ClipExtent clips the *projected* output to an axis-aligned rectangle — the viewport. |
| `func (p *Projection) FitExtent(x0, y0, x1, y1 float64, o Object) *Projection` | FitExtent sets scale and translate so that the object fits inside the given rectangle, centered. |
| `func (p *Projection) FitHeight(height float64, o Object) *Projection` | FitHeight fits the object's height, letting the width fall where it may. |
| `func (p *Projection) FitSize(width, height float64, o Object) *Projection` | FitSize is FitExtent with the rectangle anchored at the origin. |
| `func (p *Projection) FitWidth(width float64, o Object) *Projection` | FitWidth fits the object's width, letting the height fall where it may. |
| `func (p *Projection) Invert(x, y float64) (lon, lat float64, ok bool)` | Invert maps a planar point back to degrees. |
| `func (p *Projection) Parallels(phi0, phi1 float64) *Projection` | Parallels sets the two standard parallels of a conic projection, in degrees — the latitudes at which the cone touches the globe and distortion is… |
| `func (p *Projection) Precision(delta float64) *Projection` | Precision sets the threshold, in pixels, for adaptive resampling: smaller values follow curves more closely and emit more points. |
| `func (p *Projection) Project(lon, lat float64) (x, y float64)` | Project maps a point in degrees to the plane. |
| `func (p *Projection) ReflectX(v bool) *Projection` | ReflectX mirrors the projected plane horizontally, which is how you draw a map of the sky rather than of the ground. |
| `func (p *Projection) ReflectY(v bool) *Projection` | ReflectY mirrors the projected plane vertically. |
| `func (p *Projection) Rotate(deltaLambda, deltaPhi, deltaGamma float64) *Projection` | Rotate sets the three-axis rotation of the globe, in degrees. |
| `func (p *Projection) Scale(k float64) *Projection` | Scale sets the scale factor: roughly, the number of pixels per radian. |
| `func (p *Projection) Stream(s Stream) Stream` | Stream wraps a downstream Stream with the whole projection pipeline: degrees to radians, rotation, pre-clipping to the sphere, adaptive resampling… |
| `func (p *Projection) Translate(x, y float64) *Projection` | Translate sets the pixel coordinates of the projection's center. |

</details>

<details>
<summary><code>Raw</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func ConicConformalRaw(y0, y1 float64) Raw` | ConicConformalRaw is Lambert's conformal conic: the standard for aeronautical charts and for mid-latitude countries wider than they are tall. |
| `func ConicEqualAreaRaw(y0, y1 float64) Raw` | ConicEqualAreaRaw is Albers' conic equal-area projection: the usual choice for a thematic map of a country, because a choropleth whose areas are… |
| `func ConicEquidistantRaw(y0, y1 float64) Raw` | ConicEquidistantRaw preserves distance along meridians and along the two standard parallels. |

</details>

<details>
<summary><code>Rotation</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRotation(deltaLambda, deltaPhi, deltaGamma float64) *Rotation` | NewRotation returns the rotation by the given angles in degrees. |
| `func (r *Rotation) Invert(lon, lat float64) (float64, float64)` | Invert undoes the rotation. |
| `func (r *Rotation) Rotate(lon, lat float64) (float64, float64)` | Rotate applies the rotation to a point in degrees. |

</details>

<details>
<summary><code>Sphere</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func (*Sphere) GeoJSONType() string` | — |
| `func (g *Sphere) MarshalJSON() ([]byte, error)` | MarshalJSON implements json.Marshaler. |

</details>

### Variables

`AzimuthalEqualAreaRaw`, `AzimuthalEquidistantRaw`, `EqualEarthRaw`, `EquirectangularRaw`, `GnomonicRaw`, `MercatorRaw`, `NaturalEarth1Raw`, `OrthographicRaw`, `StereographicRaw`, `TransverseMercatorRaw`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/geo).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
