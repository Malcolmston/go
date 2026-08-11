package contour

import (
	"math"
	"sort"

	"github.com/malcolmston/d3/array"
)

// Accessor reads one value out of a datum, with the (datum, index, data)
// signature used throughout this port.
type Accessor[T any] func(d T, i int, data []T) float64

// ContourDensity estimates a two-dimensional density from scattered points and
// contours it — the density heat map that replaces a scatterplot once there are
// too many points to see through.
//
// The estimate is a kernel density estimate computed the fast way: deposit each
// point's weight onto a coarse grid, blur the grid with a separable
// approximation of a Gaussian, and contour the blurred grid. Blurring a grid is
// O(cells) regardless of how many points there are, which is why this scales to
// millions of points where evaluating a kernel per point per pixel would not.
//
// The output is in the same coordinate space as the input, not in grid cells:
// [ContourDensity.Compute] scales the rings back by the cell size and removes
// the padding it added around the extent.
//
//	c := contour.NewContourDensity[P]().
//		X(func(p P, _ int, _ []P) float64 { return p.X }).
//		Y(func(p P, _ int, _ []P) float64 { return p.Y }).
//		Size(960, 500).
//		Bandwidth(30)
//	bands := c.Compute(data)
//
// The zero value is not usable; construct with [NewContourDensity].
type ContourDensity[T any] struct {
	x, y, weight Accessor[T]

	dx, dy int // extent, in input coordinates
	r      float64
	k      int // log2 of the grid cell size

	o    int // padding around the extent, in input coordinates
	n, m int // grid dimensions

	thresholds []float64
	count      int
}

// NewContourDensity returns a density estimator over a 960×500 extent with a
// bandwidth of about 20, a cell size of 4 and roughly 20 thresholds — d3's
// defaults. X and Y return 0 until configured.
func NewContourDensity[T any]() *ContourDensity[T] {
	c := &ContourDensity[T]{
		x:      func(T, int, []T) float64 { return 0 },
		y:      func(T, int, []T) float64 { return 0 },
		weight: func(T, int, []T) float64 { return 1 },
		dx:     960,
		dy:     500,
		r:      20,
		k:      2,
		count:  20,
	}
	c.resize()
	return c
}

func (c *ContourDensity[T]) resize() *ContourDensity[T] {
	c.o = int(c.r * 3)
	c.n = (c.dx + c.o*2) >> c.k
	c.m = (c.dy + c.o*2) >> c.k
	if c.n < 1 {
		c.n = 1
	}
	if c.m < 1 {
		c.m = 1
	}
	return c
}

// X sets the accessor for the horizontal coordinate.
func (c *ContourDensity[T]) X(f Accessor[T]) *ContourDensity[T] { c.x = f; return c }

// Y sets the accessor for the vertical coordinate.
func (c *ContourDensity[T]) Y(f Accessor[T]) *ContourDensity[T] { c.y = f; return c }

// Weight sets the accessor for a datum's weight. The default is 1, so density
// counts points; a weight makes it sum a quantity instead.
func (c *ContourDensity[T]) Weight(f Accessor[T]) *ContourDensity[T] { c.weight = f; return c }

// Size sets the extent of the estimate in input coordinates. Points outside it
// contribute nothing.
func (c *ContourDensity[T]) Size(dx, dy int) *ContourDensity[T] {
	if dx < 0 || dy < 0 {
		panic("contour: invalid size")
	}
	c.dx, c.dy = dx, dy
	return c.resize()
}

// CellSize sets the side of a grid cell in input coordinates. It is rounded
// down to a power of two, because the grid index is computed by shifting. A
// larger cell is faster and blockier; it must be at least 1.
func (c *ContourDensity[T]) CellSize(size float64) *ContourDensity[T] {
	if !(size >= 1) {
		panic("contour: invalid cell size")
	}
	c.k = int(math.Floor(math.Log2(size)))
	if c.k < 0 {
		c.k = 0
	}
	return c.resize()
}

// Bandwidth sets the standard deviation of the smoothing kernel, in input
// coordinates. It is the one knob that decides whether the result looks like a
// handful of spikes or a single blob.
//
// The value is converted to the box-blur radius whose triple application has
// that standard deviation, so the number set here is comparable across cell
// sizes.
func (c *ContourDensity[T]) Bandwidth(bw float64) *ContourDensity[T] {
	if !(bw >= 0) {
		panic("contour: invalid bandwidth")
	}
	c.r = (math.Sqrt(4*bw*bw+1) - 1) / 2
	return c.resize()
}

// Thresholds sets the explicit density levels to contour. The list is copied
// and sorted ascending.
func (c *ContourDensity[T]) Thresholds(t []float64) *ContourDensity[T] {
	c.thresholds = append([]float64(nil), t...)
	sortFloats(c.thresholds)
	return c
}

// ThresholdCount asks for approximately n uniformly spaced density levels.
func (c *ContourDensity[T]) ThresholdCount(n int) *ContourDensity[T] {
	c.thresholds = nil
	if n < 1 {
		n = 1
	}
	c.count = n
	return c
}

// Grid returns the estimated density grid together with its dimensions, in
// units of weight per square input unit. It is what [ContourDensity.Compute]
// contours, exposed because a caller may want to sample the density directly —
// to color a point by the local density it sits in, say — rather than draw
// bands.
func (c *ContourDensity[T]) Grid(data []T) (values []float64, n, m int) {
	values = make([]float64, c.n*c.m)
	pow2k := math.Pow(2, float64(-c.k))

	for i, d := range data {
		xi := (c.x(d, i, data) + float64(c.o)) * pow2k
		yi := (c.y(d, i, data) + float64(c.o)) * pow2k
		wi := c.weight(d, i, data)
		if wi == 0 || math.IsNaN(wi) || math.IsNaN(xi) || math.IsNaN(yi) {
			continue
		}
		if xi < 0 || xi >= float64(c.n) || yi < 0 || yi >= float64(c.m) {
			continue
		}
		// Bilinear deposit: splitting the weight across the four nearest cells
		// rather than dropping it all in one keeps the estimate from jittering
		// as a point moves across a cell boundary, which matters because the
		// grid is coarse relative to the data.
		x0 := int(math.Floor(xi - 0.5))
		y0 := int(math.Floor(yi - 0.5))
		xt := xi - 0.5 - float64(x0)
		yt := yi - 0.5 - float64(y0)
		add := func(x, y int, w float64) {
			if x >= 0 && x < c.n && y >= 0 && y < c.m {
				values[x+y*c.n] += w
			}
		}
		add(x0, y0, (1-xt)*(1-yt)*wi)
		add(x0+1, y0, xt*(1-yt)*wi)
		add(x0, y0+1, (1-xt)*yt*wi)
		add(x0+1, y0+1, xt*yt*wi)
	}

	blur2(values, c.n, c.m, c.r*pow2k)
	return values, c.n, c.m
}

// Compute returns one MultiPolygon per density level, in input coordinates.
//
// Values are densities — weight per square input unit — so they do not change
// when the cell size does, and a threshold chosen for one cell size stays
// meaningful at another.
func (c *ContourDensity[T]) Compute(data []T) []MultiPolygon {
	values, n, m := c.Grid(data)
	pow4k := math.Pow(2, float64(2*c.k))

	tz := c.thresholds
	if tz == nil {
		maxV := 0.0
		for _, v := range values {
			if v > maxV {
				maxV = v
			}
		}
		// The lower bound is the smallest positive value rather than zero: a
		// contour at exactly zero would enclose the entire padded grid.
		tz = array.Ticks(math.SmallestNonzeroFloat64, maxV/pow4k, c.count)
	}

	scaled := make([]float64, len(tz))
	for i, t := range tz {
		scaled[i] = t * pow4k
	}

	cs := NewContours().Size(n, m).Thresholds(scaled)
	out := cs.Compute(values)

	pow2k := math.Pow(2, float64(c.k))
	for i := range out {
		if i < len(tz) {
			out[i].Value = tz[i]
		}
		for _, poly := range out[i].Coordinates {
			for _, ring := range poly {
				for j := range ring {
					ring[j].X = ring[j].X*pow2k - float64(c.o)
					ring[j].Y = ring[j].Y*pow2k - float64(c.o)
				}
			}
		}
	}
	return out
}

func sortFloats(v []float64) { sort.Float64s(v) }

// blur2 applies a separable approximation of a Gaussian blur of standard
// deviation r to an n×m grid, in place.
//
// Three successive box blurs approximate a Gaussian closely enough that the
// difference is invisible in a density map, and each box blur is a running sum:
// the whole thing costs O(n*m) regardless of the radius. Fractional radii are
// handled by weighting the two cells just past the box, so that the bandwidth
// varies continuously rather than in integer jumps.
func blur2(values []float64, n, m int, r float64) {
	if r <= 0 || n == 0 || m == 0 {
		return
	}
	src := make([]float64, max(n, m))
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < m; y++ {
			line := values[y*n : (y+1)*n]
			copy(src, line)
			boxBlur(src[:n], line, r)
		}
		for x := 0; x < n; x++ {
			for y := 0; y < m; y++ {
				src[y] = values[x+y*n]
			}
			boxBlurStrided(src[:m], values, x, n, r)
		}
	}
}

// boxBlurStrided is boxBlur writing into column x of a grid of stride n.
func boxBlurStrided(src, dst []float64, x, n int, r float64) {
	out := make([]float64, len(src))
	boxBlur(src, out, r)
	for y, v := range out {
		dst[x+y*n] = v
	}
}

// boxBlur writes to dst the box average of src over a window of radius r,
// clamping at the ends. A fractional radius weights the two boundary cells by
// the fraction, so the kernel is continuous in r.
func boxBlur(src, dst []float64, r float64) {
	n := len(src)
	if n == 0 {
		return
	}
	r0 := int(math.Floor(r))
	t := r - float64(r0)
	w := 2*r + 1
	at := func(i int) float64 {
		if i < 0 {
			i = 0
		}
		if i >= n {
			i = n - 1
		}
		return src[i]
	}
	// Running sum over the integer part of the window.
	sum := 0.0
	for i := -r0; i <= r0; i++ {
		sum += at(i)
	}
	for i := 0; i < n; i++ {
		dst[i] = (sum + t*(at(i-r0-1)+at(i+r0+1))) / w
		sum += at(i+r0+1) - at(i-r0)
	}
}
