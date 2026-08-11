package shape

import (
	"math"
	"sort"
)

// SeriesPoint is one datum's contribution to one series: the baseline and
// topline the shape generator will draw. Upper-Lower is the datum's value
// (except under [OffsetExpand], which normalizes it).
type SeriesPoint[T any] struct {
	Lower float64
	Upper float64
	Data  T
}

// Series is one layer of a stacked chart — one key's values across all the
// data. Index is its position in the stacking order, which is what a color
// scale should key off when an order is applied.
type Series[T any] struct {
	Key    string
	Index  int
	Points []SeriesPoint[T]
}

// StackOrder chooses the bottom-to-top order of the layers, returning series
// indices. It sees the raw values as [lower, upper] pairs, before any offset
// has been applied, so lower is always 0 and upper is the datum's value.
//
// Orders and offsets are deliberately not generic: they only ever look at
// numbers, so keeping the datum type out of their signatures means
// shape.OrderInsideOut is a plain value rather than something that has to be
// instantiated at every use.
type StackOrder func(values [][][2]float64) []int

// StackOffset repositions the layers in place, given the values matrix
// (series × datum × [lower, upper]) and the stacking order.
type StackOffset func(values [][][2]float64, order []int)

// Stack turns wide data — one row per x, one field per layer — into the
// baselines and toplines of a stacked chart. Each layer becomes a [Series] to
// be drawn with an [Area] or a set of rects.
//
//	st := shape.NewStack[Row]().Keys("apples", "bananas").
//		Value(func(r Row, key string, _ int, _ []Row) float64 { return r.Fruit[key] })
//	for _, s := range st.Generate(rows) { … }
//
// Value has no meaningful default for an arbitrary datum type (d3 indexes the
// datum by key, which Go cannot do generically), so it returns 0 until set.
type Stack[T any] struct {
	keys   []string
	value  func(d T, key string, i int, data []T) float64
	order  StackOrder
	offset StackOffset
}

// NewStack returns a Stack with no keys, input order and no offset — the
// conventional stacked chart, where each layer sits on the one below.
func NewStack[T any]() *Stack[T] {
	return &Stack[T]{
		value:  func(T, string, int, []T) float64 { return 0 },
		order:  OrderNone,
		offset: OffsetNone,
	}
}

// Keys sets the layers, bottom to top before any reordering.
func (s *Stack[T]) Keys(keys ...string) *Stack[T] { s.keys = keys; return s }

// Value sets the accessor for a datum's contribution to one key.
func (s *Stack[T]) Value(f func(d T, key string, i int, data []T) float64) *Stack[T] {
	s.value = f
	return s
}

// Order sets the layer order. The default is [OrderNone].
func (s *Stack[T]) Order(o StackOrder) *Stack[T] { s.order = o; return s }

// Offset sets the baseline treatment. The default is [OffsetNone].
func (s *Stack[T]) Offset(o StackOffset) *Stack[T] { s.offset = o; return s }

// Generate computes the series, in key order. Each series' Index reports its
// position in the stacking order.
func (s *Stack[T]) Generate(data []T) []Series[T] {
	n := len(s.keys)
	values := make([][][2]float64, n)
	for i, key := range s.keys {
		values[i] = make([][2]float64, len(data))
		for j, d := range data {
			values[i][j] = [2]float64{0, s.value(d, key, j, data)}
		}
	}

	// Order first, then offset: an offset such as wiggle needs to know which
	// layer is at the bottom before it can decide where the bottom goes.
	order := s.order(values)
	s.offset(values, order)

	series := make([]Series[T], n)
	for i := range series {
		series[i] = Series[T]{Key: s.keys[i], Points: make([]SeriesPoint[T], len(data))}
		for j := range data {
			series[i].Points[j] = SeriesPoint[T]{
				Lower: values[i][j][0],
				Upper: values[i][j][1],
				Data:  data[j],
			}
		}
	}
	for i, oi := range order {
		if oi >= 0 && oi < n {
			series[oi].Index = i
		}
	}
	return series
}

// ---------------------------------------------------------------------------
// Offsets
// ---------------------------------------------------------------------------

// OffsetNone stacks each layer on top of the one below, starting at zero. This
// is the plain stacked chart, and every other offset ends by applying it.
//
// A NaN value is treated as a zero-height layer rather than poisoning
// everything above it: the layer above inherits the baseline unchanged.
func OffsetNone(values [][][2]float64, order []int) {
	n := len(values)
	if n <= 1 {
		return
	}
	s1 := values[order[0]]
	m := len(s1)
	for i := 1; i < n; i++ {
		s0 := s1
		s1 = values[order[i]]
		for j := 0; j < m && j < len(s1); j++ {
			base := s0[j][1]
			if math.IsNaN(base) {
				base = s0[j][0]
			}
			s1[j][0] = base
			s1[j][1] += base
		}
	}
}

// OffsetExpand normalizes each column so the layers sum to 1, turning a
// stacked chart into a share-of-total chart. Columns that sum to zero are left
// alone rather than producing infinities.
func OffsetExpand(values [][][2]float64, order []int) {
	n := len(values)
	if n == 0 {
		return
	}
	m := len(values[0])
	for j := 0; j < m; j++ {
		y := 0.0
		for i := 0; i < n; i++ {
			if v := values[i][j][1]; !math.IsNaN(v) {
				y += v
			}
		}
		if y != 0 {
			for i := 0; i < n; i++ {
				values[i][j][1] /= y
			}
		}
	}
	OffsetNone(values, order)
}

// OffsetDiverging splits each column at zero: positive values stack upward
// from the baseline and negative values stack downward, so a chart of gains
// and losses reads correctly instead of cancelling out.
//
// It is the one offset that does not end with [OffsetNone] — it computes both
// baselines itself.
func OffsetDiverging(values [][][2]float64, order []int) {
	n := len(values)
	if n == 0 {
		return
	}
	m := len(values[order[0]])
	for j := 0; j < m; j++ {
		yp, yn := 0.0, 0.0
		for i := 0; i < n; i++ {
			d := &values[order[i]][j]
			dy := d[1] - d[0]
			switch {
			case dy > 0:
				d[0] = yp
				yp += dy
				d[1] = yp
			case dy < 0:
				d[1] = yn
				yn += dy
				d[0] = yn
			default:
				d[0] = 0
				d[1] = dy
			}
		}
	}
}

// OffsetSilhouette centers the stack on zero: each column's baseline is minus
// half its total, so the band is symmetric about the axis. It is the classic
// "streamgraph without the wiggle" layout.
func OffsetSilhouette(values [][][2]float64, order []int) {
	n := len(values)
	if n == 0 {
		return
	}
	s0 := values[order[0]]
	for j := 0; j < len(s0); j++ {
		y := 0.0
		for i := 0; i < n; i++ {
			if v := values[i][j][1]; !math.IsNaN(v) {
				y += v
			}
		}
		s0[j][0] = -y / 2
		s0[j][1] += s0[j][0]
	}
	OffsetNone(values, order)
}

// OffsetWiggle shifts each column's baseline to minimize the total weighted
// slope of the layers — Byron & Wattenberg's streamgraph algorithm. The
// baseline wanders, which is the point: it keeps the individual bands as flat
// as possible so their thicknesses stay readable, at the cost of a
// non-horizontal zero line.
//
// It assumes the layers are already in a sensible order; pair it with
// [OrderInsideOut], which is what the original paper recommends.
func OffsetWiggle(values [][][2]float64, order []int) {
	n := len(values)
	if n == 0 {
		return
	}
	s0 := values[order[0]]
	m := len(s0)
	if m == 0 {
		return
	}
	y := 0.0
	j := 1
	for ; j < m; j++ {
		var s1, s2 float64
		for i := 0; i < n; i++ {
			si := values[order[i]]
			sij0 := nz(si[j][1])
			sij1 := nz(si[j-1][1])
			s3 := (sij0 - sij1) / 2
			for k := 0; k < i; k++ {
				sk := values[order[k]]
				s3 += nz(sk[j][1]) - nz(sk[j-1][1])
			}
			s1 += sij0
			s2 += s3 * sij0
		}
		s0[j-1][0] = y
		s0[j-1][1] += y
		if s1 != 0 {
			y -= s2 / s1
		}
	}
	s0[j-1][0] = y
	s0[j-1][1] += y
	OffsetNone(values, order)
}

// nz treats NaN as zero, the way JavaScript's `x || 0` does in d3's offsets.
func nz(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return v
}

// ---------------------------------------------------------------------------
// Orders
// ---------------------------------------------------------------------------

// OrderNone stacks in key order.
func OrderNone(values [][][2]float64) []int {
	o := make([]int, len(values))
	for i := range o {
		o[i] = i
	}
	return o
}

// OrderReverse stacks in reverse key order.
func OrderReverse(values [][][2]float64) []int {
	return reverseInts(OrderNone(values))
}

// OrderAscending puts the smallest series (by total value) at the bottom,
// which keeps the most volatile layers near the baseline where they are
// easiest to read.
func OrderAscending(values [][][2]float64) []int {
	sums := seriesSums(values)
	o := OrderNone(values)
	sort.SliceStable(o, func(a, b int) bool { return sums[o[a]] < sums[o[b]] })
	return o
}

// OrderDescending puts the largest series at the bottom.
func OrderDescending(values [][][2]float64) []int {
	return reverseInts(OrderAscending(values))
}

// OrderAppearance orders series by when they peak, earliest first.
func OrderAppearance(values [][][2]float64) []int {
	peaks := make([]int, len(values))
	for i, s := range values {
		peaks[i] = peakIndex(s)
	}
	o := OrderNone(values)
	sort.SliceStable(o, func(a, b int) bool { return peaks[o[a]] < peaks[o[b]] })
	return o
}

// OrderInsideOut puts the series that peak earliest in the middle of the stack
// and alternates outward, balancing the two halves by total. It is the
// companion to [OffsetWiggle]: a streamgraph looks right when the big, early
// layers are in the middle and the small, late ones at the edges.
func OrderInsideOut(values [][][2]float64) []int {
	n := len(values)
	sums := seriesSums(values)
	order := OrderAppearance(values)
	var top, bottom float64
	tops := make([]int, 0, n)
	bottoms := make([]int, 0, n)
	for i := 0; i < n; i++ {
		j := order[i]
		if top < bottom {
			top += sums[j]
			tops = append(tops, j)
		} else {
			bottom += sums[j]
			bottoms = append(bottoms, j)
		}
	}
	return append(reverseInts(bottoms), tops...)
}

func seriesSums(values [][][2]float64) []float64 {
	sums := make([]float64, len(values))
	for i, s := range values {
		total := 0.0
		for _, p := range s {
			if !math.IsNaN(p[1]) {
				total += p[1]
			}
		}
		sums[i] = total
	}
	return sums
}

func peakIndex(s [][2]float64) int {
	j := 0
	vj := math.Inf(-1)
	for i, p := range s {
		if p[1] > vj {
			vj, j = p[1], i
		}
	}
	return j
}

func reverseInts(o []int) []int {
	for i, j := 0, len(o)-1; i < j; i, j = i+1, j-1 {
		o[i], o[j] = o[j], o[i]
	}
	return o
}
