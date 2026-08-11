package hierarchy

import "math"

// Circle is a circle in layout space, used by [Enclose] and internally by
// [Pack].
type Circle struct {
	X, Y, R float64
}

// Enclose returns the smallest circle that contains all of the given circles, or
// the zero Circle if there are none.
//
// This is Welzl's algorithm in the incremental "move-to-front" form D3 uses. The
// idea is that the smallest enclosing circle is always determined by at most
// three of the input circles — its *basis* — so rather than search over all
// triples, we scan the circles and, whenever one falls outside the current
// candidate, rebuild the basis to include it and restart the scan. Each rebuild
// strictly improves the candidate, so the process terminates, and because the
// input is shuffled first the expected number of rebuilds is linear rather than
// the quadratic worst case an adversarial order would produce.
//
// The shuffle uses a fixed-seed linear congruential generator, not
// [math/rand], so the result is deterministic: the same input always produces
// the same circle, which matters because a layout that shifted slightly between
// runs would make snapshot tests and animations useless.
//
// Note the basis members are circles, not points, so "encloses" means one circle
// contains another, and the two- and three-circle basis solutions are the
// Apollonius constructions rather than the simpler point versions.
func Enclose(circles []Circle) Circle {
	if len(circles) == 0 {
		return Circle{}
	}
	work := make([]*Circle, len(circles))
	for i := range circles {
		c := circles[i]
		work[i] = &c
	}
	e := encloseAll(work)
	if e == nil {
		return Circle{}
	}
	return *e
}

// encloseAll is the shared implementation, operating on pointers so that [Pack]
// can hand it the very circles it is positioning without copying.
func encloseAll(circles []*Circle) *Circle {
	n := len(circles)
	if n == 0 {
		return nil
	}
	shuffled := make([]*Circle, n)
	copy(shuffled, circles)
	shuffle(shuffled, newLCG())

	var basis []*Circle
	var e *Circle
	for i := 0; i < n; {
		p := shuffled[i]
		if e != nil && enclosesWeak(e, p) {
			i++
			continue
		}
		basis = extendBasis(basis, p)
		e = encloseBasis(basis)
		i = 0
	}
	return e
}

// newLCG returns D3's own linear congruential generator, seeded at 1. It is
// reimplemented here rather than pulled from math/rand precisely because its
// determinism is a feature; see [Enclose].
func newLCG() func() float64 {
	const (
		a = 1664525
		c = 1013904223
		m = 4294967296 // 2^32
	)
	s := uint32(1)
	return func() float64 {
		s = uint32((a*uint64(s) + c) % m)
		return float64(s) / m
	}
}

// shuffle performs a Fisher–Yates shuffle driven by the supplied generator.
func shuffle[T any](a []T, random func() float64) {
	for m := len(a); m > 0; {
		i := int(random() * float64(m))
		m--
		if i >= len(a) {
			i = len(a) - 1
		}
		a[m], a[i] = a[i], a[m]
	}
}

// extendBasis returns a new basis that includes p, trying the cheapest
// possibilities first: p alone, then p paired with an existing basis member,
// then p with two of them. One of these must work, because a smallest enclosing
// circle in the plane is determined by at most three circles.
func extendBasis(basis []*Circle, p *Circle) []*Circle {
	if enclosesWeakAll(p, basis) {
		return []*Circle{p}
	}
	for _, b := range basis {
		if enclosesNot(p, b) && enclosesWeakAll(encloseBasis2(b, p), basis) {
			return []*Circle{b, p}
		}
	}
	for i := 0; i < len(basis)-1; i++ {
		for j := i + 1; j < len(basis); j++ {
			if enclosesNot(encloseBasis2(basis[i], basis[j]), p) &&
				enclosesNot(encloseBasis2(basis[i], p), basis[j]) &&
				enclosesNot(encloseBasis2(basis[j], p), basis[i]) &&
				enclosesWeakAll(encloseBasis3(basis[i], basis[j], p), basis) {
				return []*Circle{basis[i], basis[j], p}
			}
		}
	}
	// Unreachable for well-formed input; degrading to the single-circle basis
	// beats panicking inside a layout, and the outer loop will correct it.
	return []*Circle{p}
}

// enclosesNot reports that a does *not* contain b.
func enclosesNot(a, b *Circle) bool {
	dr := a.R - b.R
	dx := b.X - a.X
	dy := b.Y - a.Y
	return dr < 0 || dr*dr < dx*dx+dy*dy
}

// enclosesWeak reports that a contains b, with a relative tolerance so that a
// circle sitting exactly on the boundary — which is the normal case for a basis
// member — counts as enclosed despite rounding.
func enclosesWeak(a, b *Circle) bool {
	dr := a.R - b.R + math.Max(math.Max(a.R, b.R), 1)*1e-9
	dx := b.X - a.X
	dy := b.Y - a.Y
	return dr > 0 && dr*dr > dx*dx+dy*dy
}

func enclosesWeakAll(a *Circle, basis []*Circle) bool {
	for _, b := range basis {
		if !enclosesWeak(a, b) {
			return false
		}
	}
	return true
}

func encloseBasis(basis []*Circle) *Circle {
	switch len(basis) {
	case 1:
		c := *basis[0]
		return &c
	case 2:
		return encloseBasis2(basis[0], basis[1])
	case 3:
		return encloseBasis3(basis[0], basis[1], basis[2])
	}
	return nil
}

// encloseBasis2 returns the smallest circle tangent internally to both a and b:
// its diameter is the distance between centres plus both radii, and its centre
// lies on the line through them.
func encloseBasis2(a, b *Circle) *Circle {
	x1, y1, r1 := a.X, a.Y, a.R
	x2, y2, r2 := b.X, b.Y, b.R
	x21, y21, r21 := x2-x1, y2-y1, r2-r1
	l := math.Sqrt(x21*x21 + y21*y21)
	if l == 0 {
		// Concentric: the larger circle already encloses the smaller.
		if r1 >= r2 {
			c := *a
			return &c
		}
		c := *b
		return &c
	}
	return &Circle{
		X: (x1 + x2 + x21/l*r21) / 2,
		Y: (y1 + y2 + y21/l*r21) / 2,
		R: (l + r1 + r2) / 2,
	}
}

// encloseBasis3 solves for the circle internally tangent to all three, which
// reduces to a quadratic in the unknown radius once the centre is expressed as
// an affine function of it.
func encloseBasis3(a, b, c *Circle) *Circle {
	x1, y1, r1 := a.X, a.Y, a.R
	x2, y2, r2 := b.X, b.Y, b.R
	x3, y3, r3 := c.X, c.Y, c.R
	a2, a3 := x1-x2, x1-x3
	b2, b3 := y1-y2, y1-y3
	c2, c3 := r2-r1, r3-r1
	d1 := x1*x1 + y1*y1 - r1*r1
	d2 := d1 - x2*x2 - y2*y2 + r2*r2
	d3 := d1 - x3*x3 - y3*y3 + r3*r3
	ab := a3*b2 - a2*b3
	xa := (b2*d3-b3*d2)/(ab*2) - x1
	xb := (b3*c2 - b2*c3) / ab
	ya := (a3*d2-a2*d3)/(ab*2) - y1
	yb := (a2*c3 - a3*c2) / ab
	A := xb*xb + yb*yb - 1
	B := 2 * (r1 + xa*xb + ya*yb)
	C := xa*xa + ya*ya - r1*r1
	var r float64
	if math.Abs(A) > 1e-6 {
		r = -((B + math.Sqrt(B*B-4*A*C)) / (2 * A))
	} else {
		r = -(C / B)
	}
	return &Circle{X: x1 + xa + xb*r, Y: y1 + ya + yb*r, R: r}
}
