package geo

import "math"

// xform is a pair of inverse maps on the plane or the sphere. Every projection,
// rotation and affine transform in this package is one, and composing two of
// them composes their inverses in the opposite order — which is the whole
// reason [Projection.Invert] exists at all.
type xform struct {
	fwd func(x, y float64) (float64, float64)
	inv func(x, y float64) (float64, float64)
}

func compose(a, b xform) xform {
	t := xform{fwd: func(x, y float64) (float64, float64) { return b.fwd(a.fwd(x, y)) }}
	if a.inv != nil && b.inv != nil {
		t.inv = func(x, y float64) (float64, float64) { return a.inv(b.inv(x, y)) }
	}
	return t
}

// rotationIdentity wraps longitude into (-π, π] and leaves latitude alone. It
// is not a no-op: an unrotated projection still has to normalize longitudes
// that arithmetic pushed past the antimeridian.
func rotationIdentity(lambda, phi float64) (float64, float64) {
	if math.Abs(lambda) > pi {
		lambda -= math.Round(lambda/tau) * tau
	}
	return lambda, phi
}

func forwardRotationLambda(deltaLambda float64) func(float64, float64) (float64, float64) {
	return func(lambda, phi float64) (float64, float64) {
		lambda += deltaLambda
		if math.Abs(lambda) > pi {
			lambda -= math.Round(lambda/tau) * tau
		}
		return lambda, phi
	}
}

func rotationLambda(deltaLambda float64) xform {
	return xform{fwd: forwardRotationLambda(deltaLambda), inv: forwardRotationLambda(-deltaLambda)}
}

// rotationPhiGamma rotates about the y and x axes. Longitude rotation is a
// shift; these two are not, so they go through Cartesian coordinates.
func rotationPhiGamma(deltaPhi, deltaGamma float64) xform {
	cosDeltaPhi, sinDeltaPhi := math.Cos(deltaPhi), math.Sin(deltaPhi)
	cosDeltaGamma, sinDeltaGamma := math.Cos(deltaGamma), math.Sin(deltaGamma)
	return xform{
		fwd: func(lambda, phi float64) (float64, float64) {
			cosPhi := math.Cos(phi)
			x := math.Cos(lambda) * cosPhi
			y := math.Sin(lambda) * cosPhi
			z := math.Sin(phi)
			k := z*cosDeltaPhi + x*sinDeltaPhi
			return math.Atan2(y*cosDeltaGamma-k*sinDeltaGamma, x*cosDeltaPhi-z*sinDeltaPhi),
				asin(k*cosDeltaGamma + y*sinDeltaGamma)
		},
		inv: func(lambda, phi float64) (float64, float64) {
			cosPhi := math.Cos(phi)
			x := math.Cos(lambda) * cosPhi
			y := math.Sin(lambda) * cosPhi
			z := math.Sin(phi)
			k := z*cosDeltaGamma - y*sinDeltaGamma
			return math.Atan2(y*cosDeltaGamma+z*sinDeltaGamma, x*cosDeltaPhi+k*sinDeltaPhi),
				asin(k*cosDeltaPhi - x*sinDeltaPhi)
		},
	}
}

// rotateRadians builds the three-axis rotation used by every projection. The
// branching is d3's and is worth keeping: the common cases (no rotation, or
// longitude only) skip the Cartesian round trip entirely.
func rotateRadians(deltaLambda, deltaPhi, deltaGamma float64) xform {
	deltaLambda = math.Mod(deltaLambda, tau)
	switch {
	case deltaLambda != 0 && (deltaPhi != 0 || deltaGamma != 0):
		return compose(rotationLambda(deltaLambda), rotationPhiGamma(deltaPhi, deltaGamma))
	case deltaLambda != 0:
		return rotationLambda(deltaLambda)
	case deltaPhi != 0 || deltaGamma != 0:
		return rotationPhiGamma(deltaPhi, deltaGamma)
	}
	return xform{fwd: rotationIdentity, inv: rotationIdentity}
}

// Rotation is d3.geoRotation: a rotation of the sphere expressed as three
// angles in degrees, applied in the order λ (about the poles), φ (about the
// y-axis) and γ (about the x-axis).
//
// This is what "recentering" a map actually is. A projection is fixed — an
// orthographic projection always looks at (0°, 0°) — so to look at Tokyo you
// rotate the globe under it, which is why [Projection.Rotate] takes the
// *negated* coordinates of the place you want in the middle.
type Rotation struct{ t xform }

// NewRotation returns the rotation by the given angles in degrees.
func NewRotation(deltaLambda, deltaPhi, deltaGamma float64) *Rotation {
	return &Rotation{t: rotateRadians(deltaLambda*radians, deltaPhi*radians, deltaGamma*radians)}
}

// Rotate applies the rotation to a point in degrees.
func (r *Rotation) Rotate(lon, lat float64) (float64, float64) {
	x, y := r.t.fwd(lon*radians, lat*radians)
	return x * degrees, y * degrees
}

// Invert undoes the rotation.
func (r *Rotation) Invert(lon, lat float64) (float64, float64) {
	x, y := r.t.inv(lon*radians, lat*radians)
	return x * degrees, y * degrees
}
