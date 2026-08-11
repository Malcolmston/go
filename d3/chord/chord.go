// Package chord is a Go port of d3-chord: the layout that turns a square
// matrix of flows into the angles of a chord diagram, and the ribbon generator
// that draws one flow as an SVG path.
//
// A chord diagram shows a square matrix — trade between countries, transitions
// between states, migration between regions — as a circle. Each row/column
// index becomes a *group*, an arc around the circumference whose angular size
// is the group's total. Each matrix cell becomes a *chord*, a ribbon joining
// the two groups it relates, whose width at each end is that cell's value.
//
// The layout is pure arithmetic over a matrix and draws nothing:
//
//	matrix := [][]float64{
//		{11975, 5871, 8916, 2868},
//		{1951, 10048, 2060, 6171},
//		{8010, 16145, 8090, 8045},
//		{1013, 990, 940, 6907},
//	}
//	res := chord.New().PadAngle(0.05).Generate(matrix)
//
//	arc := shape.NewArc().InnerRadius(240).OuterRadius(250)
//	for _, g := range res.Groups {
//		fmt.Println(arc.Generate(shape.ArcDatum{StartAngle: g.StartAngle, EndAngle: g.EndAngle}))
//	}
//
//	ribbon := chord.NewRibbon().Radius(240)
//	for _, c := range res.Chords {
//		fmt.Println(ribbon.Generate(c))
//	}
//
// Groups feed [github.com/malcolmston/d3/shape.Arc] directly — that is the
// whole reason the layout returns angles rather than paths, exactly as
// [github.com/malcolmston/d3/shape.Pie] does. The ribbons are this package's
// own job, because their geometry (two arcs joined by two quadratic curves
// through the centre) has no counterpart in d3/shape.
//
// # Angles
//
// Angles are in radians, measured clockwise from twelve o'clock, matching
// d3/shape's arcs. Together the groups span exactly 2π: the padding is taken
// *out* of the circle rather than added to it, so with n groups and a pad
// angle p the group arcs sum to 2π − np and the gaps sum to np. That identity
// holds for any matrix and is the invariant to assert if you are testing a
// layout of your own.
//
// When the matrix sums to zero there is no value to distribute, and the groups
// are spread evenly at 2π/n each with zero angular width — the degenerate case
// upstream produces, kept rather than special-cased.
//
// # The three layouts
//
// [New] treats the matrix as undirected: cells (i,j) and (j,i) describe the two
// ends of a single ribbon, and one [ChordDatum] is produced per unordered pair,
// with the larger end as Source.
//
// [NewDirected] treats every cell as its own directed flow. A group's arc then
// covers both its outgoing row and its incoming column, so each pair can yield
// a ribbon in each direction, and the ribbon is drawn narrow-to-wide rather
// than symmetrically. This is the layout to use with [NewRibbonArrow].
//
// [NewTranspose] is [New] over the transposed matrix. Upstream calls it
// chordTranspose; there is no "chordTransitive" in d3-chord and none is
// invented here.
//
// # Configuration
//
// The layout and both ribbon generators follow the port's fluent convention:
// configuration methods are named after the property, take the new value,
// return the receiver so calls chain, and have no getters. A configured value
// is invoked with Generate.
package chord

import (
	"math"
	"sort"
)

const (
	tau     = 2 * math.Pi
	halfPi  = math.Pi / 2
	epsilon = 1e-12
)

// Subgroup is one end of a chord: the slice of a group's arc that this
// particular flow occupies.
//
// Index is the group it belongs to, Value the matrix cell it came from, and
// the angles are the sub-arc it spans — always contained within that group's
// [Group] arc.
type Subgroup struct {
	Index      int
	StartAngle float64
	EndAngle   float64
	Value      float64
}

// ChordDatum is one ribbon: the two ends it joins.
//
// For an undirected layout Source is the larger of the two ends. For a
// directed layout Source is the origin of the flow and Target its destination,
// which is what makes an arrowhead meaningful.
type ChordDatum struct {
	Source Subgroup
	Target Subgroup
}

// Group is one index's arc around the circumference. Value is the group's
// total — its row sum, plus its column sum for a directed layout.
type Group struct {
	Index      int
	StartAngle float64
	EndAngle   float64
	Value      float64
}

// Chords is what a layout produces. Upstream returns an array of chords with
// the groups hung off it as a `.groups` property; Go cannot attach a property
// to a slice, so both are fields of a struct — the same resolution
// d3/dsv uses for csvParse's `.columns`.
type Chords struct {
	Chords []ChordDatum
	Groups []Group
}

// Layout computes the angles of a chord diagram from a square matrix.
//
// Construct one with [New], [NewDirected] or [NewTranspose]; the zero value is
// not usable. A configured Layout is a pure function from a matrix to a
// [Chords] and is safe to call from any goroutine.
type Layout struct {
	directed  bool
	transpose bool

	padAngle      float64
	sortGroups    func(a, b float64) int
	sortSubgroups func(a, b float64) int
	sortChords    func(a, b float64) int
}

// New returns an undirected chord layout: no padding, and groups, subgroups
// and chords all left in matrix order.
func New() *Layout { return &Layout{} }

// NewDirected returns a directed chord layout, in which each cell is its own
// flow and a group's arc spans both its row and its column. Pair it with
// [NewRibbonArrow] to show direction.
func NewDirected() *Layout { return &Layout{directed: true} }

// NewTranspose returns an undirected chord layout over the transpose of the
// matrix — the same diagram with rows and columns exchanged. Upstream's name
// for it is chordTranspose.
func NewTranspose() *Layout { return &Layout{transpose: true} }

// PadAngle sets the angular gap between adjacent groups, in radians. It is
// subtracted from the circle rather than added to it, so the diagram still
// spans exactly one turn. Negative values are clamped to zero.
func (l *Layout) PadAngle(v float64) *Layout {
	l.padAngle = math.Max(0, v)
	return l
}

// SortGroups orders the groups around the circle by comparing their totals.
// The comparator returns negative when a should come first, in the shape
// [sort] and [slices] use. Passing nil restores matrix order.
//
// Sorting by descending total is the usual choice, because it puts the
// diagram's dominant groups together instead of scattering them.
func (l *Layout) SortGroups(f func(a, b float64) int) *Layout { l.sortGroups = f; return l }

// SortSubgroups orders the chord ends within each group by comparing their
// values. Passing nil restores matrix order.
func (l *Layout) SortSubgroups(f func(a, b float64) int) *Layout { l.sortSubgroups = f; return l }

// SortChords orders the returned chords by comparing the sum of each chord's
// two end values. Passing nil leaves them in matrix order.
//
// This is a *drawing* order, and it matters: ribbons overlap near the centre,
// so drawing the largest last (an ascending sort) puts them on top. Upstream
// wraps the comparator the same way, comparing source.value + target.value
// rather than exposing the chords themselves.
func (l *Layout) SortChords(f func(a, b float64) int) *Layout { l.sortChords = f; return l }

// Generate computes the layout.
//
// The matrix is read as square with n = len(matrix). A short row is padded
// with zeros rather than panicking, so a ragged matrix degrades into a sparse
// one; extra columns beyond n are ignored. An empty matrix yields empty
// results.
func (l *Layout) Generate(matrix [][]float64) Chords {
	n := len(matrix)
	if n == 0 {
		return Chords{}
	}

	// Flatten once, applying the transpose here so nothing downstream has to
	// think about it. at() tolerates ragged input.
	m := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if l.transpose {
				m[i*n+j] = at(matrix, j, i, n)
			} else {
				m[i*n+j] = at(matrix, i, j, n)
			}
		}
	}

	directed := 0.0
	if l.directed {
		directed = 1
	}

	// The scaling factor from value to angle. A group's total is its row sum,
	// plus its column sum when the layout is directed (because then the group
	// arc has to accommodate incoming flows as well as outgoing ones).
	groupSums := make([]float64, n)
	total := 0.0
	for i := 0; i < n; i++ {
		x := 0.0
		for j := 0; j < n; j++ {
			x += m[i*n+j] + directed*m[j*n+i]
		}
		groupSums[i] = x
		total += x
	}
	// The angle left for the groups themselves, once the gaps are taken out.
	available := math.Max(0, tau-l.padAngle*float64(n))
	k := 0.0
	if total != 0 && !math.IsNaN(total) && !math.IsInf(total, 0) {
		k = available / total
	}
	dx := l.padAngle
	if available == 0 {
		// Padding has consumed the whole circle. Rather than invert, the
		// groups collapse to n evenly spaced points of zero width — upstream's
		// behavior, and the only degenerate answer that stays on the circle.
		dx = tau / float64(n)
	}

	groupIndex := make([]int, n)
	for i := range groupIndex {
		groupIndex[i] = i
	}
	if l.sortGroups != nil {
		sort.SliceStable(groupIndex, func(a, b int) bool {
			return l.sortGroups(groupSums[groupIndex[a]], groupSums[groupIndex[b]]) < 0
		})
	}

	// chords is keyed by i*n+j, the cell that owns the ribbon, so that the two
	// ends of one ribbon — discovered while walking two different groups —
	// find each other. Insertion order is deliberately not used: iterating the
	// keys in ascending order reproduces upstream's Object.values() over a
	// sparse array.
	chords := make(map[int]*entry, n)
	groups := make([]Group, n)

	x := 0.0
	for _, i := range groupIndex {
		x0 := x
		if l.directed {
			x = l.layoutDirected(m, n, i, k, x, chords)
		} else {
			x = l.layoutUndirected(m, n, i, k, x, chords)
		}
		groups[i] = Group{Index: i, StartAngle: x0, EndAngle: x, Value: groupSums[i]}
		x += dx
	}

	keys := make([]int, 0, len(chords))
	for key := range chords {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	out := make([]ChordDatum, len(keys))
	for i, key := range keys {
		out[i] = chords[key].d
	}
	if l.sortChords != nil {
		sort.SliceStable(out, func(a, b int) bool {
			return l.sortChords(
				out[a].Source.Value+out[a].Target.Value,
				out[b].Source.Value+out[b].Target.Value,
			) < 0
		})
	}
	return Chords{Chords: out, Groups: groups}
}

// layoutUndirected walks group i's subgroups and returns the angle it ends at.
//
// A cell is visited from both of its groups: (i,j) contributes one end while
// laying out group i and the other while laying out group j, and both write
// into the chord keyed by the lower-index-first cell.
func (l *Layout) layoutUndirected(m []float64, n, i int, k, x float64, chords map[int]*entry) float64 {
	sub := make([]int, 0, n)
	for j := 0; j < n; j++ {
		// A pair with no flow in either direction gets no ribbon at all.
		if truthy(m[i*n+j]) || truthy(m[j*n+i]) {
			sub = append(sub, j)
		}
	}
	if l.sortSubgroups != nil {
		sort.SliceStable(sub, func(a, b int) bool {
			return l.sortSubgroups(m[i*n+sub[a]], m[i*n+sub[b]]) < 0
		})
	}
	for _, j := range sub {
		v := m[i*n+j]
		var c *entry
		if i < j {
			c = chordAt(chords, i*n+j)
			c.d.Source = Subgroup{Index: i, StartAngle: x, EndAngle: x + v*k, Value: v}
			c.hasSource = true
		} else {
			c = chordAt(chords, j*n+i)
			c.d.Target = Subgroup{Index: i, StartAngle: x, EndAngle: x + v*k, Value: v}
			c.hasTarget = true
			// A self-loop has only one end; both sides of the ribbon are it.
			if i == j {
				c.d.Source = c.d.Target
				c.hasSource = true
			}
		}
		x += v * k
		// Source is the larger end, so a ribbon always reads widest-first.
		if c.hasSource && c.hasTarget && c.d.Source.Value < c.d.Target.Value {
			c.d.Source, c.d.Target = c.d.Target, c.d.Source
		}
	}
	return x
}

// layoutDirected walks group i's subgroups, which for a directed layout
// include both outgoing cells (row i) and incoming ones (column i).
//
// Upstream encodes the two in a single index range by using the bitwise
// complement for incoming edges; here they are an explicit pair, which costs
// nothing and is legible.
func (l *Layout) layoutDirected(m []float64, n, i int, k, x float64, chords map[int]*entry) float64 {
	type edge struct {
		other    int
		incoming bool
	}
	// Incoming edges first, in descending order of the other index, then
	// outgoing in ascending order — upstream's range(-n, n) walked in order,
	// which decides the default arrangement within a group's arc.
	sub := make([]edge, 0, 2*n)
	for j := n - 1; j >= 0; j-- {
		if truthy(m[j*n+i]) {
			sub = append(sub, edge{other: j, incoming: true})
		}
	}
	for j := 0; j < n; j++ {
		if truthy(m[i*n+j]) {
			sub = append(sub, edge{other: j, incoming: false})
		}
	}
	// Incoming values sort as negatives so that, under an ascending
	// comparator, everything arriving precedes everything leaving.
	value := func(e edge) float64 {
		if e.incoming {
			return -m[e.other*n+i]
		}
		return m[i*n+e.other]
	}
	if l.sortSubgroups != nil {
		sort.SliceStable(sub, func(a, b int) bool {
			return l.sortSubgroups(value(sub[a]), value(sub[b])) < 0
		})
	}
	for _, e := range sub {
		if e.incoming {
			v := m[e.other*n+i]
			c := chordAt(chords, e.other*n+i)
			c.d.Target = Subgroup{Index: i, StartAngle: x, EndAngle: x + v*k, Value: v}
			c.hasTarget = true
			x += v * k
		} else {
			v := m[i*n+e.other]
			c := chordAt(chords, i*n+e.other)
			c.d.Source = Subgroup{Index: i, StartAngle: x, EndAngle: x + v*k, Value: v}
			c.hasSource = true
			x += v * k
		}
	}
	return x
}

// entry is a chord under construction. The two ends are discovered while
// walking two different groups, and the flags record which have arrived —
// upstream distinguishes them with null, which a Go struct field cannot.
type entry struct {
	d                    ChordDatum
	hasSource, hasTarget bool
}

func chordAt(chords map[int]*entry, key int) *entry {
	if c, ok := chords[key]; ok {
		return c
	}
	c := &entry{}
	chords[key] = c
	return c
}

// truthy mirrors JavaScript's test on a number: zero and NaN are falsy, which
// is what keeps empty cells (and NaN ones) from producing ribbons.
func truthy(v float64) bool { return v != 0 && !math.IsNaN(v) }

// at reads matrix[i][j], treating a missing row or column as zero.
func at(matrix [][]float64, i, j, n int) float64 {
	if i >= len(matrix) || j >= n {
		return 0
	}
	row := matrix[i]
	if j >= len(row) {
		return 0
	}
	return row[j]
}
