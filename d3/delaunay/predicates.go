package delaunay

import (
	"math"
	"math/big"
)

// The two geometric predicates below decide every branch the triangulation
// takes: which side of a line a point falls on, and whether a point lies inside
// a circle. Both are sign tests on a determinant, and both are catastrophically
// sensitive to rounding — a determinant whose true value is 1e-30 can come out
// of float64 arithmetic with the wrong sign, and a single wrong sign is enough
// to send the incremental hull walk into an infinite loop or to leave the mesh
// with crossing edges.
//
// Delaunator handles this by importing Shewchuk's adaptive-precision
// robust-predicates for the orientation test. There is no such package in the
// standard library, so this port uses a two-stage scheme with the same
// guarantee: evaluate in float64, compare the result against a bound on its own
// rounding error, and fall back to exact rational arithmetic ([math/big.Rat],
// which represents every float64 exactly) only when the fast answer is not
// trustworthy. The fallback is orders of magnitude slower and essentially never
// taken on data that is not degenerate, which is precisely the trade Shewchuk's
// adaptive predicates make.

// orientErrBound is a conservative relative bound on the rounding error of the
// float64 evaluation of orient2d. It is Delaunator's own constant (roughly
// 3 * 2^-52), applied to the magnitude of the two products.
const orientErrBound = 3.3306690738754716e-16

// orient2d reports the turn direction of a→b→c. The result is negative when the
// three points are in counterclockwise order, positive for clockwise and zero
// for collinear.
//
// The sign convention is the one the JavaScript robust-predicates package uses
// — negative for counterclockwise — rather than Shewchuk's original, because
// Delaunator is written against it and every comparison in [delaunator.update]
// is a transcription of Delaunator's.
func orient2d(ax, ay, bx, by, cx, cy float64) float64 {
	// The explicit float64 conversions matter: without them the compiler is
	// free to contract the two products and the subtraction into a fused
	// multiply-add. Fusion keeps more precision, which sounds harmless and is
	// not — it leaves a tiny non-zero residual where the true determinant is
	// exactly zero, and collinearity is detected by that exact zero.
	l := float64((ay - cy) * (bx - cx))
	r := float64((ax - cx) * (by - cy))
	det := l - r
	if math.Abs(det) >= orientErrBound*math.Abs(l+r) {
		return det
	}
	if !allFinite(ax, ay, bx, by, cx, cy) {
		return det
	}
	return orient2dExact(ax, ay, bx, by, cx, cy)
}

// orient2dExact evaluates the same determinant over the rationals, so its sign
// is the true sign. The magnitude it returns is meaningless beyond that sign;
// callers only ever compare against zero.
func orient2dExact(ax, ay, bx, by, cx, cy float64) float64 {
	var l, r, t1, t2 big.Rat
	t1.Sub(rat(ay), rat(cy))
	t2.Sub(rat(bx), rat(cx))
	l.Mul(&t1, &t2)
	t1.Sub(rat(ax), rat(cx))
	t2.Sub(rat(by), rat(cy))
	r.Mul(&t1, &t2)
	return float64(l.Cmp(&r))
}

// inCircle reports whether d lies strictly inside the circle through a, b and c
// — the Delaunay condition, and the only thing that decides whether an edge is
// flipped.
//
// The triangle a, b, c must be counterclockwise under [orient2d]'s convention;
// the determinant's sign is meaningless otherwise. That precondition is what
// the seed-triangle reordering in [delaunator.update] establishes and what edge
// flipping preserves.
func inCircle(ax, ay, bx, by, cx, cy, dx0, dy0 float64) bool {
	dx := ax - dx0
	dy := ay - dy0
	ex := bx - dx0
	ey := by - dy0
	fx := cx - dx0
	fy := cy - dy0

	ap := dx*dx + dy*dy
	bp := ex*ex + ey*ey
	cp := fx*fx + fy*fy

	t1 := dx * (float64(ey*cp) - float64(bp*fy))
	t2 := dy * (float64(ex*cp) - float64(bp*fx))
	t3 := ap * (float64(ex*fy) - float64(ey*fx))
	det := float64(t1-t2) + t3

	// The bound is deliberately loose: an unnecessary trip through the exact
	// path costs time, a missed one costs correctness.
	bound := 1e-14 * (math.Abs(t1) + math.Abs(t2) + math.Abs(t3))
	if math.Abs(det) > bound {
		return det < 0
	}
	if !allFinite(ax, ay, bx, by, cx, cy, dx0, dy0) {
		return det < 0
	}
	return inCircleExact(ax, ay, bx, by, cx, cy, dx0, dy0) < 0
}

// inCircleExact evaluates the in-circle determinant over the rationals.
func inCircleExact(ax, ay, bx, by, cx, cy, px, py float64) int {
	sub := func(u, v float64) *big.Rat {
		var r big.Rat
		return r.Sub(rat(u), rat(v))
	}
	dx, dy := sub(ax, px), sub(ay, py)
	ex, ey := sub(bx, px), sub(by, py)
	fx, fy := sub(cx, px), sub(cy, py)

	sq := func(u, v *big.Rat) *big.Rat {
		var a, b, s big.Rat
		a.Mul(u, u)
		b.Mul(v, v)
		return s.Add(&a, &b)
	}
	ap := sq(dx, dy)
	bp := sq(ex, ey)
	cp := sq(fx, fy)

	mul := func(u, v *big.Rat) *big.Rat { var r big.Rat; return r.Mul(u, v) }
	sub2 := func(u, v *big.Rat) *big.Rat { var r big.Rat; return r.Sub(u, v) }

	t1 := mul(dx, sub2(mul(ey, cp), mul(bp, fy)))
	t2 := mul(dy, sub2(mul(ex, cp), mul(bp, fx)))
	t3 := mul(ap, sub2(mul(ex, fy), mul(ey, fx)))

	var det big.Rat
	det.Add(sub2(t1, t2), t3)
	return det.Sign()
}

// rat converts a finite float64 to an exact rational.
func rat(v float64) *big.Rat {
	var r big.Rat
	r.SetFloat64(v)
	return &r
}

func allFinite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// dist2 is the squared distance between two points. Squared, because every use
// is a comparison and the square root would only cost time.
func dist2(ax, ay, bx, by float64) float64 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

// circumradius2 returns the squared circumradius of a triangle, or +Inf when
// the three points are collinear and no circumcircle exists. Returning infinity
// rather than an error is what lets the seed search in [delaunator.update]
// discover "every point is collinear" by simply never finding a finite radius.
func circumradius2(ax, ay, bx, by, cx, cy float64) float64 {
	dx := bx - ax
	dy := by - ay
	ex := cx - ax
	ey := cy - ay
	bl := dx*dx + dy*dy
	cl := ex*ex + ey*ey
	// Unfused, for the reason given in orient2d: a degenerate triangle must
	// produce exactly zero here, or it is mistaken for a real one.
	den := float64(dx*ey) - float64(dy*ex)
	if den == 0 {
		return math.Inf(1)
	}
	d := 0.5 / den
	x := (ey*bl - dy*cl) * d
	y := (dx*cl - ex*bl) * d
	r := x*x + y*y
	if math.IsNaN(r) {
		return math.Inf(1)
	}
	return r
}

// circumcenter returns the center of the circle through the three points.
func circumcenter(ax, ay, bx, by, cx, cy float64) (float64, float64) {
	dx := bx - ax
	dy := by - ay
	ex := cx - ax
	ey := cy - ay
	bl := dx*dx + dy*dy
	cl := ex*ex + ey*ey
	d := 0.5 / (float64(dx*ey) - float64(dy*ex))
	return ax + (float64(ey*bl)-float64(dy*cl))*d, ay + (float64(dx*cl)-float64(ex*bl))*d
}

// pseudoAngle maps a direction to [0, 1) monotonically in the real angle,
// without trigonometry. It is only ever used as a hash key for the hull, where
// monotonicity is all that matters and atan2 would dominate the running time.
func pseudoAngle(dx, dy float64) float64 {
	den := math.Abs(dx) + math.Abs(dy)
	if den == 0 {
		return 0
	}
	p := dx / den
	if dy > 0 {
		return (3 - p) / 4
	}
	return (1 + p) / 4
}
