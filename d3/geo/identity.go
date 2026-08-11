package geo

import "math"

// Identity is d3.geoIdentity: a "projection" that does no spherical
// mathematics at all, only scale, translate, reflect, rotate and clip.
//
// It exists because a lot of geographic data is already projected — a
// shapefile in state-plane coordinates, a TopoJSON of pixel positions, a
// floor plan — and running it through a real projection would be wrong. With an
// Identity, [Path] still gives you clipping, bounds, area, centroid and the
// Fit family over that data.
//
// Note the y axis: unlike [Projection], Identity does *not* flip it, because
// its input is already in screen-like coordinates. Use ReflectY if it is not.
type Identity struct {
	k      float64
	tx, ty float64
	sx, sy float64
	alpha  float64
	ca, sa float64

	clipX0, clipY0, clipX1, clipY1 float64
	hasClip                        bool
	postclip                       func(Stream) Stream

	kx, ky float64
}

// NewIdentity returns an identity transform: unit scale, no translation, no
// clipping.
func NewIdentity() *Identity {
	i := &Identity{k: 1, sx: 1, sy: 1}
	i.reset()
	return i
}

func (i *Identity) reset() *Identity {
	i.kx, i.ky = i.k*i.sx, i.k*i.sy
	i.ca, i.sa = math.Cos(i.alpha), math.Sin(i.alpha)
	if i.hasClip {
		i.postclip = clipRectangle(i.clipX0, i.clipY0, i.clipX1, i.clipY1)
	} else {
		i.postclip = nil
	}
	return i
}

// Project applies the transform.
func (i *Identity) Project(px, py float64) (x, y float64) {
	x, y = px*i.kx, py*i.ky
	if i.alpha != 0 {
		t := y*i.ca - x*i.sa
		x = x*i.ca + y*i.sa
		y = t
	}
	return x + i.tx, y + i.ty
}

// Invert undoes the transform.
func (i *Identity) Invert(px, py float64) (x, y float64) {
	x, y = px-i.tx, py-i.ty
	if i.alpha != 0 {
		t := y*i.ca + x*i.sa
		x = x*i.ca - y*i.sa
		y = t
	}
	return x / i.kx, y / i.ky
}

// Stream implements [Projector].
func (i *Identity) Stream(s Stream) Stream {
	down := s
	if i.postclip != nil {
		down = i.postclip(down)
	}
	return &funcStream{
		point: func(x, y, _ float64) {
			px, py := i.Project(x, y)
			down.Point(px, py, 0)
		},
		lineStart:    down.LineStart,
		lineEnd:      down.LineEnd,
		polygonStart: down.PolygonStart,
		polygonEnd:   down.PolygonEnd,
		sphere:       down.Sphere,
	}
}

// Scale sets the scale factor.
func (i *Identity) Scale(k float64) *Identity { i.k = k; return i.reset() }

// Translate sets the translation in output units.
func (i *Identity) Translate(x, y float64) *Identity { i.tx, i.ty = x, y; return i.reset() }

// Angle sets a rotation of the plane, in degrees clockwise.
func (i *Identity) Angle(a float64) *Identity {
	i.alpha = math.Mod(a, 360) * radians
	return i.reset()
}

// ReflectX mirrors horizontally.
func (i *Identity) ReflectX(v bool) *Identity {
	i.sx = 1
	if v {
		i.sx = -1
	}
	return i.reset()
}

// ReflectY mirrors vertically — the usual way to turn data whose y axis points
// up into SVG coordinates, whose y axis points down.
func (i *Identity) ReflectY(v bool) *Identity {
	i.sy = 1
	if v {
		i.sy = -1
	}
	return i.reset()
}

// ClipExtent clips output to an axis-aligned rectangle.
func (i *Identity) ClipExtent(x0, y0, x1, y1 float64) *Identity {
	i.clipX0, i.clipY0, i.clipX1, i.clipY1 = x0, y0, x1, y1
	i.hasClip = true
	return i.reset()
}

// ClearClipExtent removes any clipping.
func (i *Identity) ClearClipExtent() *Identity { i.hasClip = false; return i.reset() }

func (i *Identity) fitScaleTranslate(k, x, y float64) {
	i.k, i.tx, i.ty = k, x, y
	i.reset()
}

func (i *Identity) fitSuspendClip() func() {
	if !i.hasClip {
		return func() {}
	}
	x0, y0, x1, y1 := i.clipX0, i.clipY0, i.clipX1, i.clipY1
	i.ClearClipExtent()
	return func() { i.ClipExtent(x0, y0, x1, y1) }
}

// FitExtent sets scale and translate so the object fits the given rectangle.
func (i *Identity) FitExtent(x0, y0, x1, y1 float64, o Object) *Identity {
	fitExtent(i, x0, y0, x1, y1, o)
	return i
}

// FitSize is FitExtent anchored at the origin.
func (i *Identity) FitSize(width, height float64, o Object) *Identity {
	fitSize(i, width, height, o)
	return i
}

// FitWidth fits the object's width.
func (i *Identity) FitWidth(width float64, o Object) *Identity {
	fitWidth(i, width, o)
	return i
}

// FitHeight fits the object's height.
func (i *Identity) FitHeight(height float64, o Object) *Identity {
	fitHeight(i, height, o)
	return i
}
