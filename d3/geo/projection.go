package geo

import "math"

// Raw is a raw projection: a pair of maps between spherical coordinates in
// *radians* and an abstract plane, before any scaling, translation or rotation.
//
// Every projection in this package is a Raw wrapped by a [Projection], which is
// the same split d3 makes. It is the useful one: the mathematics of a
// projection is a dozen lines, and everything else — where the map sits on the
// screen, how large it is, which part of the globe faces you, where it is cut —
// is shared machinery.
//
// Inverse may be nil for a projection that cannot be inverted, in which case
// [Projection.Invert] reports ok == false. All the projections shipped here are
// invertible.
type Raw struct {
	Forward func(lambda, phi float64) (x, y float64)
	Inverse func(x, y float64) (lambda, phi float64)
}

func (r Raw) xform() xform { return xform{fwd: r.Forward, inv: r.Inverse} }

// Projector is anything that can wrap a [Stream] with its own transformation —
// a [Projection] or an [Identity]. It is what [Path] accepts.
type Projector interface {
	Stream(s Stream) Stream
}

// Projection turns spherical coordinates into planar ones, with the
// configuration a map needs around it.
//
// The default state matches d3: scale 150, translate (480, 250), no rotation,
// antimeridian pre-clipping, no post-clipping, and a precision of √0.5 pixels.
//
// Configuration is write-only and chains:
//
//	p := geo.NewMercator().Scale(200).Translate(320, 240).Rotate(-11, 0, 0)
//	x, y := p.Project(2.35, 48.85)  // Paris
//
// A Projection is mutable and is not safe for concurrent configuration.
// [Projection.Stream] and [Projection.Project] are read-only once configured.
type Projection struct {
	raw Raw

	// projectAt, phi0 and phi1 exist for the conic family, whose raw projection
	// depends on its two standard parallels and so must be rebuilt when they
	// change.
	projectAt func(phi0, phi1 float64) Raw
	phi0      float64
	phi1      float64

	k      float64 // scale
	x, y   float64 // translate
	lambda float64 // center, radians
	phi    float64

	deltaLambda, deltaPhi, deltaGamma float64 // pre-rotation, radians
	alpha                             float64 // post-rotation, radians
	sx, sy                            float64 // reflection

	theta    float64 // clip angle, radians
	hasTheta bool
	preclip  func(Stream) Stream

	clipX0, clipY0, clipX1, clipY1 float64
	hasClip                        bool
	postclip                       func(Stream) Stream

	delta2 float64 // squared precision

	// mercatorClip enables the extra clip that keeps a cylindrical projection
	// from repeating the world horizontally; transverse flips its axis.
	mercatorClip bool
	transverse   bool

	rotate                 xform
	projectTransform       xform
	projectRotateTransform xform
	projectResample        func(Stream) Stream
}

// NewProjection wraps a raw projection with the standard configuration.
func NewProjection(raw Raw) *Projection {
	p := &Projection{
		raw:     raw,
		k:       150,
		x:       480,
		y:       250,
		sx:      1,
		sy:      1,
		preclip: clipAntimeridian,
		delta2:  0.5,
		phi1:    pi / 3,
	}
	p.recenter()
	return p
}

// NewConicProjection wraps a family of raw projections parameterized by two
// standard parallels; see [Projection.Parallels]. The default parallels are 0°
// and 60°, matching d3.
func NewConicProjection(projectAt func(phi0, phi1 float64) Raw) *Projection {
	p := &Projection{
		projectAt: projectAt,
		k:         150,
		x:         480,
		y:         250,
		sx:        1,
		sy:        1,
		preclip:   clipAntimeridian,
		delta2:    0.5,
		phi0:      0,
		phi1:      pi / 3,
	}
	p.raw = projectAt(p.phi0, p.phi1)
	p.recenter()
	return p
}

// scaleTranslateRotate builds the affine map from the raw projection's plane to
// screen coordinates. The y axis is negated because SVG's origin is top-left
// and a map's is not.
func scaleTranslateRotate(k, dx, dy, sx, sy, alpha float64) xform {
	if alpha == 0 {
		return xform{
			fwd: func(x, y float64) (float64, float64) {
				x, y = x*sx, y*sy
				return dx + k*x, dy - k*y
			},
			inv: func(x, y float64) (float64, float64) {
				return (x - dx) / k * sx, (dy - y) / k * sy
			},
		}
	}
	cosAlpha, sinAlpha := math.Cos(alpha), math.Sin(alpha)
	a, b := cosAlpha*k, sinAlpha*k
	ai, bi := cosAlpha/k, sinAlpha/k
	ci := (sinAlpha*dy - cosAlpha*dx) / k
	fi := (sinAlpha*dx + cosAlpha*dy) / k
	return xform{
		fwd: func(x, y float64) (float64, float64) {
			x, y = x*sx, y*sy
			return a*x - b*y + dx, dy - b*x - a*y
		},
		inv: func(x, y float64) (float64, float64) {
			return sx * (ai*x - bi*y + ci), sy * (fi - bi*x - ai*y)
		},
	}
}

func (p *Projection) recenter() *Projection {
	center := scaleTranslateRotate(p.k, 0, 0, p.sx, p.sy, p.alpha)
	cx, cy := center.fwd(p.raw.Forward(p.lambda, p.phi))
	transform := scaleTranslateRotate(p.k, p.x-cx, p.y-cy, p.sx, p.sy, p.alpha)

	p.rotate = rotateRadians(p.deltaLambda, p.deltaPhi, p.deltaGamma)
	p.projectTransform = compose(p.raw.xform(), transform)
	p.projectRotateTransform = compose(p.rotate, p.projectTransform)
	p.projectResample = resample(p.projectTransform, p.delta2)
	p.applyClip()
	return p
}

func (p *Projection) applyClip() {
	switch {
	case p.mercatorClip:
		k := pi * p.k
		tx, ty := p.projectTransform.fwd(0, 0)
		var a0, b0, a1, b1 float64
		switch {
		case !p.hasClip:
			a0, b0, a1, b1 = tx-k, ty-k, tx+k, ty+k
		case !p.transverse:
			a0, b0 = math.Max(tx-k, p.clipX0), p.clipY0
			a1, b1 = math.Min(tx+k, p.clipX1), p.clipY1
		default:
			a0, b0 = p.clipX0, math.Max(ty-k, p.clipY0)
			a1, b1 = p.clipX1, math.Min(ty+k, p.clipY1)
		}
		p.postclip = clipRectangle(a0, b0, a1, b1)
	case p.hasClip:
		p.postclip = clipRectangle(p.clipX0, p.clipY0, p.clipX1, p.clipY1)
	default:
		p.postclip = nil
	}
}

// Project maps a point in degrees to the plane.
func (p *Projection) Project(lon, lat float64) (x, y float64) {
	return p.projectRotateTransform.fwd(lon*radians, lat*radians)
}

// Invert maps a planar point back to degrees. ok is false when the raw
// projection has no inverse.
func (p *Projection) Invert(x, y float64) (lon, lat float64, ok bool) {
	if p.projectRotateTransform.inv == nil {
		return 0, 0, false
	}
	lambda, phi := p.projectRotateTransform.inv(x, y)
	return lambda * degrees, phi * degrees, true
}

// Stream wraps a downstream Stream with the whole projection pipeline:
// degrees to radians, rotation, pre-clipping to the sphere, adaptive
// resampling and projection, then post-clipping to the viewport.
func (p *Projection) Stream(s Stream) Stream {
	down := s
	if p.postclip != nil {
		down = p.postclip(down)
	}
	down = p.projectResample(down)
	down = p.preclip(down)
	rot := p.rotate
	return &funcStream{
		point: func(x, y, _ float64) {
			lambda, phi := rot.fwd(x*radians, y*radians)
			down.Point(lambda, phi, 0)
		},
		lineStart:    down.LineStart,
		lineEnd:      down.LineEnd,
		polygonStart: down.PolygonStart,
		polygonEnd:   down.PolygonEnd,
		sphere:       down.Sphere,
	}
}

// Scale sets the scale factor: roughly, the number of pixels per radian.
// Doubling it doubles the size of the map.
func (p *Projection) Scale(k float64) *Projection { p.k = k; return p.recenter() }

// Translate sets the pixel coordinates of the projection's center. The default
// (480, 250) is the middle of a 960×500 viewport.
func (p *Projection) Translate(x, y float64) *Projection { p.x, p.y = x, y; return p.recenter() }

// Center sets the point in degrees that lands at the translate position.
//
// Center and [Projection.Rotate] both move the map, and the difference matters:
// centering slides the projected plane, so the shape of the map is unchanged
// and only which part of it is visible moves. Rotating turns the globe
// underneath the projection, which changes the shape.
//
// The center is applied *after* the rotation, which is d3's behavior and is
// worth knowing: on a rotated projection the point that lands at the translate
// position is the one that the rotation carries to the given coordinates, not
// the given coordinates themselves.
func (p *Projection) Center(lon, lat float64) *Projection {
	if p.transverse {
		// A transverse Mercator's raw projection has its axes swapped, so the
		// center it reports and the center it stores are not the same pair.
		p.lambda, p.phi = -lat*radians, lon*radians
	} else {
		p.lambda, p.phi = lon*radians, lat*radians
	}
	return p.recenter()
}

// Rotate sets the three-axis rotation of the globe, in degrees. To put a place
// in the middle of an azimuthal projection, rotate by its negated coordinates:
//
//	geo.NewOrthographic().Rotate(-lon, -lat, 0)
func (p *Projection) Rotate(deltaLambda, deltaPhi, deltaGamma float64) *Projection {
	if p.transverse {
		deltaGamma += 90
	}
	p.deltaLambda = deltaLambda * radians
	p.deltaPhi = deltaPhi * radians
	p.deltaGamma = deltaGamma * radians
	return p.recenter()
}

// Angle sets a rotation of the projected plane itself, in degrees clockwise.
func (p *Projection) Angle(a float64) *Projection {
	p.alpha = math.Mod(a, 360) * radians
	return p.recenter()
}

// ReflectX mirrors the projected plane horizontally, which is how you draw a
// map of the sky rather than of the ground.
func (p *Projection) ReflectX(v bool) *Projection {
	p.sx = 1
	if v {
		p.sx = -1
	}
	return p.recenter()
}

// ReflectY mirrors the projected plane vertically.
func (p *Projection) ReflectY(v bool) *Projection {
	p.sy = 1
	if v {
		p.sy = -1
	}
	return p.recenter()
}

// Precision sets the threshold, in pixels, for adaptive resampling: smaller
// values follow curves more closely and emit more points. Zero disables
// resampling entirely, which is what you want for a projection of already-dense
// data. The default is √0.5.
func (p *Projection) Precision(delta float64) *Projection {
	p.delta2 = delta * delta
	return p.recenter()
}

// ClipAngle sets the radius of the small circle, in degrees, outside which
// geometry is discarded before projection.
//
// Zero restores the default antimeridian clipping. A value of 90 is the visible
// hemisphere of an orthographic projection; a gnomonic projection needs
// something well under 90 because it diverges there.
func (p *Projection) ClipAngle(a float64) *Projection {
	if a == 0 {
		p.hasTheta = false
		p.preclip = clipAntimeridian
	} else {
		p.theta = a * radians
		p.hasTheta = true
		p.preclip = clipCircle(p.theta)
	}
	return p
}

// ClipExtent clips the *projected* output to an axis-aligned rectangle — the
// viewport. Unlike [Projection.ClipAngle] it operates after projection, so it
// removes what is off-screen rather than what is on the far side of the globe.
func (p *Projection) ClipExtent(x0, y0, x1, y1 float64) *Projection {
	p.clipX0, p.clipY0, p.clipX1, p.clipY1 = x0, y0, x1, y1
	p.hasClip = true
	p.applyClip()
	return p
}

// ClearClipExtent removes any viewport clipping. It is the counterpart of
// d3's clipExtent(null); Go has no null rectangle to pass.
func (p *Projection) ClearClipExtent() *Projection {
	p.hasClip = false
	p.applyClip()
	return p
}

// Parallels sets the two standard parallels of a conic projection, in degrees —
// the latitudes at which the cone touches the globe and distortion is zero. It
// has no effect on a projection that is not conic.
func (p *Projection) Parallels(phi0, phi1 float64) *Projection {
	if p.projectAt == nil {
		return p
	}
	p.phi0, p.phi1 = phi0*radians, phi1*radians
	p.raw = p.projectAt(p.phi0, p.phi1)
	return p.recenter()
}

// --- fitting -----------------------------------------------------------------

// fittable is the little that fit needs from a projection: set scale and
// translate, and temporarily suspend viewport clipping while measuring.
type fittable interface {
	Projector
	fitScaleTranslate(k, x, y float64)
	fitSuspendClip() (restore func())
}

func (p *Projection) fitScaleTranslate(k, x, y float64) {
	p.k, p.x, p.y = k, x, y
	p.recenter()
}

func (p *Projection) fitSuspendClip() func() {
	if !p.hasClip {
		return func() {}
	}
	x0, y0, x1, y1 := p.clipX0, p.clipY0, p.clipX1, p.clipY1
	p.ClearClipExtent()
	return func() { p.ClipExtent(x0, y0, x1, y1) }
}

// fitBounds measures the object at a known scale and hands the planar bounds to
// f, which chooses the final scale and translate.
func fitBounds(p fittable, o Object, f func(bx0, by0, bx1, by1 float64)) {
	restore := p.fitSuspendClip()
	p.fitScaleTranslate(150, 0, 0)
	b := &pathBounds{}
	b.reset()
	StreamObject(o, p.Stream(b))
	f(b.x0, b.y0, b.x1, b.y1)
	restore()
}

func fitExtent(p fittable, x0, y0, x1, y1 float64, o Object) {
	fitBounds(p, o, func(bx0, by0, bx1, by1 float64) {
		w, h := x1-x0, y1-y0
		k := math.Min(w/(bx1-bx0), h/(by1-by0))
		p.fitScaleTranslate(150*k, x0+(w-k*(bx1+bx0))/2, y0+(h-k*(by1+by0))/2)
	})
}

func fitSize(p fittable, width, height float64, o Object) {
	fitExtent(p, 0, 0, width, height, o)
}

func fitWidth(p fittable, width float64, o Object) {
	fitBounds(p, o, func(bx0, by0, bx1, by1 float64) {
		k := width / (bx1 - bx0)
		p.fitScaleTranslate(150*k, (width-k*(bx1+bx0))/2, -k*by0)
	})
}

func fitHeight(p fittable, height float64, o Object) {
	fitBounds(p, o, func(bx0, by0, bx1, by1 float64) {
		k := height / (by1 - by0)
		p.fitScaleTranslate(150*k, -k*bx0, (height-k*(by1+by0))/2)
	})
}

// FitExtent sets scale and translate so that the object fits inside the given
// rectangle, centered. This is how a map is sized to a viewport without anyone
// having to know what scale 150 means.
func (p *Projection) FitExtent(x0, y0, x1, y1 float64, o Object) *Projection {
	fitExtent(p, x0, y0, x1, y1, o)
	return p
}

// FitSize is FitExtent with the rectangle anchored at the origin.
func (p *Projection) FitSize(width, height float64, o Object) *Projection {
	fitSize(p, width, height, o)
	return p
}

// FitWidth fits the object's width, letting the height fall where it may.
func (p *Projection) FitWidth(width float64, o Object) *Projection {
	fitWidth(p, width, o)
	return p
}

// FitHeight fits the object's height, letting the width fall where it may.
func (p *Projection) FitHeight(height float64, o Object) *Projection {
	fitHeight(p, height, o)
	return p
}
