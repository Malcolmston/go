package shape

import (
	"math"
	"sort"
)

// PieArc is one slice computed by [Pie]: the original datum, its value, and
// the angles [Arc] needs to draw it.
//
// Index is the slice's position in the *drawing* order, which differs from its
// position in the input data when a sort is configured — the input order is
// preserved in the returned slice so a color scale keyed by position stays
// stable while the wedges move.
type PieArc[T any] struct {
	Data       T
	Index      int
	Value      float64
	StartAngle float64
	EndAngle   float64
	PadAngle   float64
}

// Arc converts the slice into the [ArcDatum] that [Arc.Generate] consumes.
func (a PieArc[T]) Arc() ArcDatum {
	return ArcDatum{StartAngle: a.StartAngle, EndAngle: a.EndAngle, PadAngle: a.PadAngle}
}

// Pie turns a slice of data into the angles of a pie or donut chart. It draws
// nothing itself — that is [Arc]'s job — which is what lets the same angles
// feed a wedge, a label position and a legend.
//
//	pie := shape.NewPie[Row]().Value(func(r Row, _ int, _ []Row) float64 { return r.Count })
//	arcs := pie.Generate(rows)
//
// Negative and zero values contribute no angle at all (they cannot: a negative
// wedge is meaningless), but they still occupy a slot, so a padded pie
// containing zeros shows the padding.
type Pie[T any] struct {
	value      Accessor[T]
	sortValues func(a, b float64) int
	sort       func(a, b T) int
	startAngle func(data []T) float64
	endAngle   func(data []T) float64
	padAngle   func(data []T) float64
}

// NewPie returns a Pie covering a full turn clockwise from twelve o'clock,
// with slices sorted by descending value and no padding. Value returns 0 until
// configured.
func NewPie[T any]() *Pie[T] {
	return &Pie[T]{
		value: constAccessor[T](0),
		// Descending value is d3's default and the conventional presentation:
		// the largest slice starts at twelve o'clock.
		sortValues: func(a, b float64) int {
			switch {
			case b < a:
				return -1
			case b > a:
				return 1
			default:
				return 0
			}
		},
		startAngle: func([]T) float64 { return 0 },
		endAngle:   func([]T) float64 { return tau },
		padAngle:   func([]T) float64 { return 0 },
	}
}

// Value sets the accessor for each datum's magnitude.
func (p *Pie[T]) Value(f Accessor[T]) *Pie[T] { p.value = f; return p }

// Sort orders the slices by comparing data, and disables [Pie.SortValues].
// Passing nil leaves the slices in input order.
func (p *Pie[T]) Sort(f func(a, b T) int) *Pie[T] { p.sort, p.sortValues = f, nil; return p }

// SortValues orders the slices by comparing computed values, and disables
// [Pie.Sort]. Passing nil leaves the slices in input order.
func (p *Pie[T]) SortValues(f func(a, b float64) int) *Pie[T] {
	p.sortValues, p.sort = f, nil
	return p
}

// StartAngle sets a constant start angle in radians.
func (p *Pie[T]) StartAngle(v float64) *Pie[T] {
	p.startAngle = func([]T) float64 { return v }
	return p
}

// StartAngleFunc computes the start angle from the whole data slice.
func (p *Pie[T]) StartAngleFunc(f func(data []T) float64) *Pie[T] { p.startAngle = f; return p }

// EndAngle sets a constant end angle. Combined with StartAngle it makes a
// partial pie — a gauge, or a semicircular donut.
func (p *Pie[T]) EndAngle(v float64) *Pie[T] { p.endAngle = func([]T) float64 { return v }; return p }

// EndAngleFunc computes the end angle from the whole data slice.
func (p *Pie[T]) EndAngleFunc(f func(data []T) float64) *Pie[T] { p.endAngle = f; return p }

// PadAngle sets a constant pad angle in radians. It is taken out of the total
// available angle rather than added to it, so the pie still spans exactly the
// configured range; it is also clamped so the padding can never exceed the
// available angle per slice.
func (p *Pie[T]) PadAngle(v float64) *Pie[T] { p.padAngle = func([]T) float64 { return v }; return p }

// PadAngleFunc computes the pad angle from the whole data slice.
func (p *Pie[T]) PadAngleFunc(f func(data []T) float64) *Pie[T] { p.padAngle = f; return p }

// Generate computes the slices. The result is in input order; each slice's
// Index gives its position in drawing order.
func (p *Pie[T]) Generate(data []T) []PieArc[T] {
	n := len(data)
	arcs := make([]PieArc[T], n)
	index := make([]int, n)
	values := make([]float64, n)

	sum := 0.0
	for i := range data {
		index[i] = i
		v := p.value(data[i], i, data)
		values[i] = v
		if v > 0 {
			sum += v
		}
	}

	a0 := p.startAngle(data)
	// A range beyond a full turn would overlap itself, so it is clamped.
	da := math.Min(tau, math.Max(-tau, p.endAngle(data)-a0))
	pad := 0.0
	if n > 0 {
		pad = math.Min(math.Abs(da)/float64(n), p.padAngle(data))
	}
	pa := pad
	if da < 0 {
		pa = -pad
	}

	switch {
	case p.sortValues != nil:
		sort.SliceStable(index, func(a, b int) bool {
			return p.sortValues(values[index[a]], values[index[b]]) < 0
		})
	case p.sort != nil:
		sort.SliceStable(index, func(a, b int) bool {
			return p.sort(data[index[a]], data[index[b]]) < 0
		})
	}

	// The padding is subtracted from the total before scaling, so the slices
	// share out only the angle that is left.
	k := 0.0
	if sum != 0 {
		k = (da - float64(n)*pa) / sum
	}
	for i := 0; i < n; i++ {
		j := index[i]
		v := values[j]
		w := 0.0
		if v > 0 {
			w = v * k
		}
		a1 := a0 + w + pa
		arcs[j] = PieArc[T]{
			Data:       data[j],
			Index:      i,
			Value:      v,
			StartAngle: a0,
			EndAngle:   a1,
			PadAngle:   pad,
		}
		a0 = a1
	}
	return arcs
}
