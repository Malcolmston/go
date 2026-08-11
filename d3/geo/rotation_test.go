package geo

import "testing"

func TestRotationCentersOnItsNegation(t *testing.T) {
	// The identity every map depends on: rotating by the negated coordinates of
	// a place puts that place at (0, 0), which is where every projection looks.
	for _, p := range [][2]float64{{0, 0}, {30, 40}, {-120, -15}, {179, 89}, {-45, 0}} {
		r := NewRotation(-p[0], -p[1], 0)
		lon, lat := r.Rotate(p[0], p[1])
		near(t, lon, 0, 1e-9, "rotated lon for %v", p)
		near(t, lat, 0, 1e-9, "rotated lat for %v", p)
	}
}

func TestRotationIsInvertible(t *testing.T) {
	rots := [][3]float64{{0, 0, 0}, {90, 0, 0}, {0, 45, 0}, {0, 0, 30}, {-71, -42, 17}, {200, 0, 0}}
	pts := [][2]float64{{0, 0}, {45, 45}, {-90, -60}, {170, 10}, {-179.9, -80}, {12, 89}}
	for _, r := range rots {
		rot := NewRotation(r[0], r[1], r[2])
		for _, p := range pts {
			lon, lat := rot.Rotate(p[0], p[1])
			back0, back1 := rot.Invert(lon, lat)
			// Longitude is only defined modulo 360, and undefined at the poles.
			near(t, back1, p[1], 1e-9, "invert lat for rot %v point %v", r, p)
			d := Distance(back0, back1, p[0], p[1])
			near(t, d, 0, 1e-9, "invert round trip for rot %v point %v", r, p)
		}
	}
}

func TestRotationPreservesDistance(t *testing.T) {
	// A rotation is an isometry of the sphere. If it were not, every projection
	// built on it would distort before it even began.
	rot := NewRotation(-71, -42, 17)
	pts := [][2]float64{{0, 0}, {45, 45}, {-90, -60}, {170, 10}, {12, 89}}
	for i, a := range pts {
		for _, b := range pts[i+1:] {
			before := Distance(a[0], a[1], b[0], b[1])
			ax, ay := rot.Rotate(a[0], a[1])
			bx, by := rot.Rotate(b[0], b[1])
			near(t, Distance(ax, ay, bx, by), before, 1e-9, "distance %v→%v", a, b)
		}
	}
}

func TestRotationLongitudeOnlyIsAShift(t *testing.T) {
	r := NewRotation(30, 0, 0)
	lon, lat := r.Rotate(10, 25)
	near(t, lon, 40, 1e-9, "shifted lon")
	near(t, lat, 25, 1e-9, "unshifted lat")
	// And it wraps rather than running off the end of the world.
	lon, _ = r.Rotate(170, 0)
	near(t, lon, -160, 1e-9, "wrapped lon")
}
