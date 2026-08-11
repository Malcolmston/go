package chord

import (
	"cmp"
	"math"
	"strconv"
	"strings"
	"testing"
)

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// A uniform 2x2 matrix splits the circle exactly in half, so every angle in
// the result is derivable on paper.
func TestUniform2x2SplitsTheCircleEvenly(t *testing.T) {
	res := New().Generate([][]float64{{1, 1}, {1, 1}})

	if len(res.Groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(res.Groups))
	}
	closeTo(t, res.Groups[0].StartAngle, 0, "group 0 start")
	closeTo(t, res.Groups[0].EndAngle, math.Pi, "group 0 end")
	closeTo(t, res.Groups[0].Value, 2, "group 0 value")
	closeTo(t, res.Groups[1].StartAngle, math.Pi, "group 1 start")
	closeTo(t, res.Groups[1].EndAngle, tau, "group 1 end")
	closeTo(t, res.Groups[1].Value, 2, "group 1 value")

	// Three chords: the two self-loops and the one pair.
	if len(res.Chords) != 3 {
		t.Fatalf("got %d chords, want 3", len(res.Chords))
	}
	// Each of the four unit cells occupies a quarter turn.
	for i, c := range res.Chords {
		closeTo(t, c.Source.EndAngle-c.Source.StartAngle, math.Pi/2, "chord source width")
		closeTo(t, c.Target.EndAngle-c.Target.StartAngle, math.Pi/2, "chord target width")
		if c.Source.Value != 1 || c.Target.Value != 1 {
			t.Errorf("chord %d values = %v/%v, want 1/1", i, c.Source.Value, c.Target.Value)
		}
	}

	// The self-loop of group 0 occupies the first quarter turn; the pair
	// occupies the second; the reverse end of the pair the third; group 1's
	// self-loop the fourth.
	closeTo(t, res.Chords[0].Source.StartAngle, 0, "chord (0,0) start")
	closeTo(t, res.Chords[0].Source.EndAngle, math.Pi/2, "chord (0,0) end")
	closeTo(t, res.Chords[1].Source.StartAngle, math.Pi/2, "chord (0,1) source start")
	closeTo(t, res.Chords[1].Target.StartAngle, math.Pi, "chord (0,1) target start")
	closeTo(t, res.Chords[2].Source.StartAngle, 3*math.Pi/2, "chord (1,1) start")
}

func TestUniform4x4SplitsTheCircleEvenly(t *testing.T) {
	m := make([][]float64, 4)
	for i := range m {
		m[i] = []float64{1, 1, 1, 1}
	}
	res := New().Generate(m)
	for i, g := range res.Groups {
		closeTo(t, g.StartAngle, float64(i)*math.Pi/2, "group start")
		closeTo(t, g.EndAngle, float64(i+1)*math.Pi/2, "group end")
	}
	// 4 self-loops + 6 unordered pairs.
	if len(res.Chords) != 10 {
		t.Errorf("got %d chords, want 10", len(res.Chords))
	}
}

// The invariant that holds for every matrix: the group arcs share out exactly
// the circle minus the total padding.
func TestGroupAnglesSumToTauMinusPadding(t *testing.T) {
	matrices := [][][]float64{
		{{1, 1}, {1, 1}},
		{{11975, 5871, 8916, 2868}, {1951, 10048, 2060, 6171}, {8010, 16145, 8090, 8045}, {1013, 990, 940, 6907}},
		{{0, 5, 0}, {3, 0, 1}, {0, 0, 7}},
		{{4}},
	}
	for _, pad := range []float64{0, 0.01, 0.05, 0.2} {
		for mi, m := range matrices {
			for name, l := range map[string]*Layout{
				"undirected": New().PadAngle(pad),
				"directed":   NewDirected().PadAngle(pad),
				"transpose":  NewTranspose().PadAngle(pad),
			} {
				res := l.Generate(m)
				n := float64(len(m))
				var sum float64
				for _, g := range res.Groups {
					if g.EndAngle < g.StartAngle {
						t.Errorf("%s matrix %d pad %v: group %d runs backwards", name, mi, pad, g.Index)
					}
					sum += g.EndAngle - g.StartAngle
				}
				want := math.Max(0, tau-pad*n)
				closeTo(t, sum, want, name+" group angle sum")

				// And the last group must finish exactly one turn minus its
				// own trailing gap from the start.
				last := res.Groups[len(res.Groups)-1]
				if pad*n < tau {
					closeTo(t, last.EndAngle, tau-pad, name+" final end angle")
				}
			}
		}
	}
}

// Every subgroup is contained in its group's arc, and the subgroups of a group
// tile that arc without gaps.
func TestSubgroupsTileTheirGroup(t *testing.T) {
	m := [][]float64{
		{11975, 5871, 8916, 2868},
		{1951, 10048, 2060, 6171},
		{8010, 16145, 8090, 8045},
		{1013, 990, 940, 6907},
	}
	for name, l := range map[string]*Layout{
		"undirected": New().PadAngle(0.03),
		"directed":   NewDirected().PadAngle(0.03),
	} {
		res := l.Generate(m)
		width := make([]float64, len(res.Groups))
		for _, c := range res.Chords {
			for _, s := range []Subgroup{c.Source, c.Target} {
				g := res.Groups[s.Index]
				if s.StartAngle < g.StartAngle-1e-9 || s.EndAngle > g.EndAngle+1e-9 {
					t.Errorf("%s: subgroup %v escapes group %v", name, s, g)
				}
				width[s.Index] += s.EndAngle - s.StartAngle
			}
		}
		for i, g := range res.Groups {
			// A self-loop in an undirected layout is one subgroup counted
			// twice (source and target are the same value), so compare only
			// where that cannot happen.
			if name == "undirected" && m[i][i] != 0 {
				continue
			}
			closeTo(t, width[i], g.EndAngle-g.StartAngle, name+" subgroup coverage")
		}
	}
}

func TestDirectedGroupSpansRowAndColumn(t *testing.T) {
	// A single directed flow from 0 to 1. Both groups have total 1, so each
	// takes half the circle.
	res := NewDirected().Generate([][]float64{{0, 1}, {0, 0}})
	closeTo(t, res.Groups[0].StartAngle, 0, "group 0 start")
	closeTo(t, res.Groups[0].EndAngle, math.Pi, "group 0 end")
	closeTo(t, res.Groups[1].StartAngle, math.Pi, "group 1 start")
	closeTo(t, res.Groups[1].EndAngle, tau, "group 1 end")

	if len(res.Chords) != 1 {
		t.Fatalf("got %d chords, want 1", len(res.Chords))
	}
	c := res.Chords[0]
	if c.Source.Index != 0 || c.Target.Index != 1 {
		t.Errorf("chord runs %d→%d, want 0→1", c.Source.Index, c.Target.Index)
	}
	closeTo(t, c.Source.StartAngle, 0, "source start")
	closeTo(t, c.Source.EndAngle, math.Pi, "source end")
	closeTo(t, c.Target.StartAngle, math.Pi, "target start")
	closeTo(t, c.Target.EndAngle, tau, "target end")
}

func TestUndirectedPutsTheLargerEndFirst(t *testing.T) {
	res := New().Generate([][]float64{{0, 2}, {9, 0}})
	if len(res.Chords) != 1 {
		t.Fatalf("got %d chords, want 1", len(res.Chords))
	}
	c := res.Chords[0]
	if c.Source.Value != 9 || c.Target.Value != 2 {
		t.Errorf("chord = %v/%v, want source 9 and target 2", c.Source.Value, c.Target.Value)
	}
}

func TestTransposeIsTheTransposedMatrix(t *testing.T) {
	m := [][]float64{{0, 5, 0}, {3, 0, 1}, {0, 0, 7}}
	mt := [][]float64{{0, 3, 0}, {5, 0, 0}, {0, 1, 7}}
	a := NewTranspose().Generate(m)
	b := New().Generate(mt)
	if len(a.Groups) != len(b.Groups) {
		t.Fatalf("group counts differ")
	}
	for i := range a.Groups {
		closeTo(t, a.Groups[i].StartAngle, b.Groups[i].StartAngle, "transposed group start")
		closeTo(t, a.Groups[i].EndAngle, b.Groups[i].EndAngle, "transposed group end")
		closeTo(t, a.Groups[i].Value, b.Groups[i].Value, "transposed group value")
	}
}

func TestEmptyCellsProduceNoChord(t *testing.T) {
	res := New().Generate([][]float64{{0, 0, 0}, {0, 0, 4}, {0, 4, 0}})
	if len(res.Chords) != 1 {
		t.Fatalf("got %d chords, want 1", len(res.Chords))
	}
	c := res.Chords[0]
	if c.Source.Index != 1 || c.Target.Index != 2 {
		t.Errorf("chord runs %d–%d, want 1–2", c.Source.Index, c.Target.Index)
	}
	// Group 0 has nothing, so it is a zero-width arc rather than absent.
	closeTo(t, res.Groups[0].EndAngle-res.Groups[0].StartAngle, 0, "empty group width")
}

func TestSortGroupsReordersTheCircle(t *testing.T) {
	m := [][]float64{{1, 0, 0}, {0, 5, 0}, {0, 0, 3}}
	// Descending totals: group 1 (5) first, then 2 (3), then 0 (1).
	res := New().SortGroups(func(a, b float64) int { return cmp.Compare(b, a) }).Generate(m)
	order := make([]int, 0, 3)
	// Read the groups back in angular order.
	byAngle := append([]Group(nil), res.Groups...)
	for len(order) < len(byAngle) {
		best := -1
		for i, g := range byAngle {
			if g.Index < 0 {
				continue
			}
			if best < 0 || g.StartAngle < byAngle[best].StartAngle {
				best = i
			}
		}
		order = append(order, byAngle[best].Index)
		byAngle[best].Index = -1
	}
	want := []int{1, 2, 0}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("angular order = %v, want %v", order, want)
		}
	}
	closeTo(t, res.Groups[1].StartAngle, 0, "largest group starts at zero")
}

func TestSortChordsOrdersByCombinedValue(t *testing.T) {
	m := [][]float64{
		{0, 1, 9},
		{1, 0, 4},
		{9, 4, 0},
	}
	res := New().SortChords(func(a, b float64) int { return cmp.Compare(a, b) }).Generate(m)
	var last float64
	for i, c := range res.Chords {
		v := c.Source.Value + c.Target.Value
		if i > 0 && v < last {
			t.Errorf("chords not ascending: %v after %v", v, last)
		}
		last = v
	}
	if len(res.Chords) != 3 {
		t.Fatalf("got %d chords, want 3", len(res.Chords))
	}
	closeTo(t, res.Chords[0].Source.Value+res.Chords[0].Target.Value, 2, "smallest chord")
	closeTo(t, res.Chords[2].Source.Value+res.Chords[2].Target.Value, 18, "largest chord")
}

func TestSortSubgroupsChangesOrderWithinAGroup(t *testing.T) {
	m := [][]float64{{0, 1, 5}, {1, 0, 0}, {5, 0, 0}}
	// Descending value within each group puts the fat chord first.
	res := New().SortSubgroups(func(a, b float64) int { return cmp.Compare(b, a) }).Generate(m)
	var first Subgroup
	for _, c := range res.Chords {
		for _, s := range []Subgroup{c.Source, c.Target} {
			if s.Index == 0 && (first.Value == 0 || s.StartAngle < first.StartAngle) {
				first = s
			}
		}
	}
	if first.Value != 5 {
		t.Errorf("first subgroup of group 0 has value %v, want 5", first.Value)
	}
	closeTo(t, first.StartAngle, 0, "first subgroup starts at zero")
}

func TestPaddingLargerThanTheCircleCollapsesGroups(t *testing.T) {
	res := New().PadAngle(4).Generate([][]float64{{1, 1}, {1, 1}})
	for _, g := range res.Groups {
		closeTo(t, g.EndAngle-g.StartAngle, 0, "collapsed group width")
	}
	closeTo(t, res.Groups[0].StartAngle, 0, "first collapsed group")
	closeTo(t, res.Groups[1].StartAngle, math.Pi, "second collapsed group")
}

func TestZeroMatrixDoesNotProduceNaN(t *testing.T) {
	res := New().Generate([][]float64{{0, 0}, {0, 0}})
	if len(res.Chords) != 0 {
		t.Errorf("got %d chords, want 0", len(res.Chords))
	}
	for _, g := range res.Groups {
		if math.IsNaN(g.StartAngle) || math.IsNaN(g.EndAngle) {
			t.Errorf("group %d has NaN angles: %v", g.Index, g)
		}
	}
}

func TestEmptyAndRaggedMatrices(t *testing.T) {
	if res := New().Generate(nil); len(res.Groups) != 0 || len(res.Chords) != 0 {
		t.Errorf("empty matrix produced %v", res)
	}
	// A short row is read as zero-padded rather than panicking.
	res := New().Generate([][]float64{{1, 1}, {1}})
	if len(res.Groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(res.Groups))
	}
	total := res.Groups[0].Value + res.Groups[1].Value
	closeTo(t, total, 3, "ragged matrix total")
}

func TestLayoutIsPure(t *testing.T) {
	m := [][]float64{{1, 2}, {3, 4}}
	l := New().PadAngle(0.05)
	a := l.Generate(m)
	b := l.Generate(m)
	for i := range a.Groups {
		if a.Groups[i] != b.Groups[i] {
			t.Errorf("repeat call differs at group %d: %v vs %v", i, a.Groups[i], b.Groups[i])
		}
	}
	// The input must be untouched.
	if m[0][0] != 1 || m[1][1] != 4 {
		t.Errorf("Generate modified its input: %v", m)
	}
}

// --- Ribbon -----------------------------------------------------------------

// A ribbon whose two ends are the same zero-width arc reduces to a single
// move, one degenerate curve and a close — derivable exactly.
func TestRibbonDegenerate(t *testing.T) {
	d := ChordDatum{
		Source: Subgroup{Index: 0, StartAngle: 0, EndAngle: 0},
		Target: Subgroup{Index: 0, StartAngle: 0, EndAngle: 0},
	}
	got := NewRibbon().Radius(100).Digits(6).Generate(d)
	// Angles are clockwise from twelve o'clock, so angle 0 at radius 100 is
	// the point (0, -100).
	want := "M0,-100Q0,0,0,-100Z"
	if got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

// The arrowhead is three straight segments at exactly computable points.
func TestRibbonArrowGeometry(t *testing.T) {
	d := ChordDatum{
		Source: Subgroup{Index: 0, StartAngle: 0, EndAngle: 0},
		Target: Subgroup{Index: 1, StartAngle: math.Pi / 2, EndAngle: math.Pi / 2},
	}
	got := NewRibbonArrow().Radius(100).HeadRadius(10).Digits(6).Generate(d)
	// Target angle pi/2 clockwise from twelve o'clock is three o'clock, i.e.
	// (r, 0). The head base sits at radius 90 and the tip at radius 100, and
	// the degenerate target arc puts all three at the same angle.
	want := "M0,-100Q0,0,90,0L100,0L90,0Q0,0,0,-100Z"
	if got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestRibbonStructure(t *testing.T) {
	res := New().Generate([][]float64{{1, 2}, {3, 4}})
	r := NewRibbon().Radius(240)
	for _, c := range res.Chords {
		d := r.Generate(c)
		if !strings.HasPrefix(d, "M") {
			t.Errorf("path %q does not start with a move", d)
		}
		if !strings.HasSuffix(d, "Z") {
			t.Errorf("path %q is not closed", d)
		}
		if c.Source == c.Target {
			// A self-loop has only one end: one arc, and one curve back to
			// where it started.
			if strings.Count(d, "A") != 1 || strings.Count(d, "Q") != 1 {
				t.Errorf("a self-loop should be one arc and one curve: %q", d)
			}
			continue
		}
		if strings.Count(d, "A") != 2 {
			t.Errorf("a two-ended ribbon should draw two arcs: %q", d)
		}
		if strings.Count(d, "Q") != 2 {
			t.Errorf("a ribbon should draw two curves through the centre: %q", d)
		}
	}
}

func TestRibbonArrowDrawsNoTargetArc(t *testing.T) {
	res := NewDirected().Generate([][]float64{{0, 1}, {2, 0}})
	arrow := NewRibbonArrow().Radius(240)
	for _, c := range res.Chords {
		d := arrow.Generate(c)
		if strings.Count(d, "L") != 2 {
			t.Errorf("an arrowhead is two line segments: %q", d)
		}
		// Only the source end is an arc.
		if strings.Count(d, "A") > 1 {
			t.Errorf("an arrow ribbon should draw at most one arc: %q", d)
		}
	}
}

// Every coordinate a ribbon emits lies within the configured radius, whatever
// the angles — the property that keeps a diagram inside its viewBox.
func TestRibbonStaysWithinItsRadius(t *testing.T) {
	const radius = 240
	res := New().PadAngle(0.05).Generate([][]float64{
		{11975, 5871, 8916, 2868},
		{1951, 10048, 2060, 6171},
		{8010, 16145, 8090, 8045},
		{1013, 990, 940, 6907},
	})
	r := NewRibbon().Radius(radius).PadAngle(0.02)
	for _, c := range res.Chords {
		for _, v := range coordinates(t, r.Generate(c)) {
			if math.Abs(v) > radius+1e-6 {
				t.Errorf("coordinate %v exceeds radius %v", v, radius)
			}
		}
	}
}

func TestRibbonPadAngleNarrowsBothEnds(t *testing.T) {
	d := ChordDatum{
		Source: Subgroup{Index: 0, StartAngle: 0, EndAngle: 1},
		Target: Subgroup{Index: 1, StartAngle: 2, EndAngle: 3},
	}
	// Padding wider than either end collapses it to its midpoint, which makes
	// the two curves meet at a point rather than crossing over.
	wide := NewRibbon().Radius(100).PadAngle(4).Generate(d)
	if wide == "" {
		t.Fatal("over-padded ribbon produced no path")
	}
	for _, v := range coordinates(t, wide) {
		if math.IsNaN(v) {
			t.Fatalf("over-padded ribbon produced NaN: %q", wide)
		}
	}
}

// coordinates pulls every number out of a path string, so a test can assert
// bounds without reimplementing the grammar.
func coordinates(t *testing.T, d string) []float64 {
	t.Helper()
	var out []float64
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		var v float64
		if _, err := fmtSscan(cur.String(), &v); err == nil {
			out = append(out, v)
		}
		cur.Reset()
	}
	for _, r := range d {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == 'e' || r == '+' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func fmtSscan(s string, v *float64) (int, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	*v = f
	return 1, nil
}

// Every accessor form is wired to the same place its constant form is, so a
// ribbon configured entirely through functions must produce exactly the path
// the constant configuration produces.
func TestRibbonAccessorFormsAgreeWithConstants(t *testing.T) {
	d := ChordDatum{
		Source: Subgroup{Index: 0, StartAngle: 0.2, EndAngle: 1.1},
		Target: Subgroup{Index: 1, StartAngle: 2.4, EndAngle: 3.3},
	}

	constants := NewRibbon().Radius(200).PadAngle(0.05).Digits(6).Generate(d)
	funcs := NewRibbon().
		Source(func(c ChordDatum) Subgroup { return c.Source }).
		Target(func(c ChordDatum) Subgroup { return c.Target }).
		RadiusFunc(func(Subgroup) float64 { return 200 }).
		StartAngleFunc(func(s Subgroup) float64 { return s.StartAngle }).
		EndAngleFunc(func(s Subgroup) float64 { return s.EndAngle }).
		PadAngleFunc(func(ChordDatum) float64 { return 0.05 }).
		Digits(6).
		Generate(d)
	if constants != funcs {
		t.Errorf("accessor form = %q, constant form = %q", funcs, constants)
	}

	// Split radii: SourceRadius and TargetRadius override Radius per end.
	split := NewRibbon().SourceRadius(200).TargetRadius(100).Digits(3).Generate(d)
	splitFn := NewRibbon().
		SourceRadiusFunc(func(Subgroup) float64 { return 200 }).
		TargetRadiusFunc(func(Subgroup) float64 { return 100 }).
		Digits(3).
		Generate(d)
	if split != splitFn {
		t.Errorf("split radius accessor form = %q, constant form = %q", splitFn, split)
	}
	if split == constants {
		t.Error("a different target radius should produce a different path")
	}

	// Constant angles override the ones on the datum, collapsing both ends.
	flat := NewRibbon().Radius(100).StartAngle(0).EndAngle(0).Digits(6).Generate(d)
	if flat != "M0,-100Q0,0,0,-100Z" {
		t.Errorf("constant-angle ribbon = %q", flat)
	}

	arrowA := NewRibbonArrow().Radius(200).HeadRadius(25).Digits(6).Generate(d)
	arrowB := NewRibbonArrow().Radius(200).
		HeadRadiusFunc(func(ChordDatum) float64 { return 25 }).
		Digits(6).Generate(d)
	if arrowA != arrowB {
		t.Errorf("head radius accessor form = %q, constant form = %q", arrowB, arrowA)
	}
}

func TestDirectedSortSubgroupsPutsArrivalsFirst(t *testing.T) {
	// Group 1 both receives (from 0) and sends (to 2). Sorting ascending puts
	// the incoming flows first, because incoming values sort as negatives.
	m := [][]float64{
		{0, 3, 0},
		{0, 0, 5},
		{0, 0, 0},
	}
	res := NewDirected().SortSubgroups(func(a, b float64) int { return cmp.Compare(a, b) }).Generate(m)
	var incoming, outgoing Subgroup
	for _, c := range res.Chords {
		if c.Target.Index == 1 {
			incoming = c.Target
		}
		if c.Source.Index == 1 {
			outgoing = c.Source
		}
	}
	if incoming.Value != 3 || outgoing.Value != 5 {
		t.Fatalf("incoming %v outgoing %v", incoming.Value, outgoing.Value)
	}
	if incoming.StartAngle >= outgoing.StartAngle {
		t.Errorf("incoming starts at %v, outgoing at %v: arrivals should come first",
			incoming.StartAngle, outgoing.StartAngle)
	}
	closeTo(t, incoming.EndAngle, outgoing.StartAngle, "subgroups abut")
}
