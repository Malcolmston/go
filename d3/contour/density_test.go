package contour

import (
	"math"
	"math/rand"
	"testing"
)

type pt struct{ X, Y, W float64 }

func px(p pt, _ int, _ []pt) float64 { return p.X }
func py(p pt, _ int, _ []pt) float64 { return p.Y }
func pw(p pt, _ int, _ []pt) float64 { return p.W }

func newDensity() *ContourDensity[pt] {
	return NewContourDensity[pt]().X(px).Y(py)
}

func TestDensityOfNoPointsIsEmpty(t *testing.T) {
	got := newDensity().Size(100, 100).Compute(nil)
	for _, mp := range got {
		if len(mp.Coordinates) != 0 {
			t.Errorf("empty data produced %v", mp.Coordinates)
		}
	}
}

func TestDensityGridPreservesTotalWeight(t *testing.T) {
	// Blurring redistributes weight; it must not create or destroy it. The
	// check is what makes the output a density rather than an arbitrary
	// smoothing artifact.
	rng := rand.New(rand.NewSource(1))
	var data []pt
	for i := 0; i < 500; i++ {
		// Well inside the extent, so no weight is clamped at the border.
		data = append(data, pt{200 + rng.NormFloat64()*30, 200 + rng.NormFloat64()*30, 1})
	}
	c := newDensity().Size(400, 400).Bandwidth(20)
	values, _, _ := c.Grid(data)
	total := 0.0
	for _, v := range values {
		total += v
	}
	if math.Abs(total-500) > 500*0.01 {
		t.Errorf("grid total = %v, want 500 within 1%%", total)
	}
}

func TestDensityWeightScalesLinearly(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	var data []pt
	for i := 0; i < 100; i++ {
		data = append(data, pt{100 + rng.NormFloat64()*10, 100 + rng.NormFloat64()*10, 3})
	}
	plain, _, _ := newDensity().Size(200, 200).Grid(data)
	weighted, _, _ := newDensity().Size(200, 200).Weight(pw).Grid(data)
	for i := range plain {
		if math.Abs(weighted[i]-3*plain[i]) > 1e-9*math.Max(1, math.Abs(weighted[i])) {
			t.Fatalf("cell %d: weighted %v, want 3x %v", i, weighted[i], plain[i])
		}
	}
}

func TestDensityContoursSurroundTheirCluster(t *testing.T) {
	// One tight cluster: every contour must enclose it, higher thresholds must
	// enclose less, and the geometry must come back in input coordinates rather
	// than grid cells.
	rng := rand.New(rand.NewSource(3))
	var data []pt
	for i := 0; i < 400; i++ {
		data = append(data, pt{300 + rng.NormFloat64()*15, 200 + rng.NormFloat64()*15, 1})
	}
	got := newDensity().Size(600, 400).Bandwidth(20).ThresholdCount(6).Compute(data)
	if len(got) == 0 {
		t.Fatal("no contours")
	}

	prev := math.Inf(1)
	for k, mp := range got {
		if len(mp.Coordinates) == 0 {
			continue
		}
		total := 0.0
		enclosed := false
		for _, poly := range mp.Coordinates {
			for _, ring := range poly {
				if ring[0] != ring[len(ring)-1] {
					t.Fatalf("contour %d: ring is not closed", k)
				}
				total += area(ring)
			}
			if inRing(poly[0], Point{300, 200}) {
				enclosed = true
			}
		}
		if !enclosed {
			t.Errorf("contour %d (value %v) does not enclose the cluster center", k, mp.Value)
		}
		if total > prev+1e-9 {
			t.Errorf("contour %d (value %v) covers %v, more than %v at the lower level", k, mp.Value, total, prev)
		}
		prev = total
		if mp.Value <= 0 {
			t.Errorf("contour %d has a non-positive density %v", k, mp.Value)
		}
	}
}

func TestDensitySeparatesTwoClusters(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	var data []pt
	for i := 0; i < 300; i++ {
		data = append(data, pt{100 + rng.NormFloat64()*8, 150 + rng.NormFloat64()*8, 1})
		data = append(data, pt{500 + rng.NormFloat64()*8, 150 + rng.NormFloat64()*8, 1})
	}
	got := newDensity().Size(600, 300).Bandwidth(15).ThresholdCount(8).Compute(data)

	// At the highest level that has any geometry, the two clusters are two
	// separate polygons, each around its own center.
	var top MultiPolygon
	for _, mp := range got {
		if len(mp.Coordinates) > 0 {
			top = mp
		}
	}
	if len(top.Coordinates) != 2 {
		t.Fatalf("the densest level has %d polygons, want 2 (one per cluster)", len(top.Coordinates))
	}
	left, right := false, false
	for _, poly := range top.Coordinates {
		if inRing(poly[0], Point{100, 150}) {
			left = true
		}
		if inRing(poly[0], Point{500, 150}) {
			right = true
		}
	}
	if !left || !right {
		t.Errorf("the two polygons do not enclose the two cluster centers (left=%v right=%v)", left, right)
	}
}

func TestDensityBandwidthWidensTheEstimate(t *testing.T) {
	// A bigger bandwidth spreads the same mass over more area, so the peak
	// density is lower. This is the one knob whose direction is easy to get
	// backwards.
	rng := rand.New(rand.NewSource(5))
	var data []pt
	for i := 0; i < 200; i++ {
		data = append(data, pt{150 + rng.NormFloat64()*5, 150 + rng.NormFloat64()*5, 1})
	}
	peak := func(bw float64) float64 {
		values, _, _ := newDensity().Size(300, 300).Bandwidth(bw).Grid(data)
		m := 0.0
		for _, v := range values {
			m = math.Max(m, v)
		}
		return m
	}
	narrow, wide := peak(5), peak(40)
	if !(narrow > wide) {
		t.Errorf("peak density with bandwidth 5 = %v, with 40 = %v; want the narrow one higher", narrow, wide)
	}
}

func TestDensityExplicitThresholds(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	var data []pt
	for i := 0; i < 200; i++ {
		data = append(data, pt{100 + rng.NormFloat64()*10, 100 + rng.NormFloat64()*10, 1})
	}
	want := []float64{0.0001, 0.001, 0.01}
	got := newDensity().Size(200, 200).Thresholds([]float64{0.01, 0.0001, 0.001}).Compute(data)
	if len(got) != 3 {
		t.Fatalf("got %d contours, want 3", len(got))
	}
	for i, mp := range got {
		if mp.Value != want[i] {
			t.Errorf("contour %d value = %v, want %v (sorted ascending)", i, mp.Value, want[i])
		}
	}
}

func TestDensityCellSizeChangesResolutionNotScale(t *testing.T) {
	// The output is in input coordinates, so a coarser grid must not move the
	// contours, only roughen them.
	rng := rand.New(rand.NewSource(7))
	var data []pt
	for i := 0; i < 500; i++ {
		data = append(data, pt{200 + rng.NormFloat64()*20, 200 + rng.NormFloat64()*20, 1})
	}
	center := func(cell float64) (float64, float64) {
		got := newDensity().Size(400, 400).CellSize(cell).Bandwidth(20).ThresholdCount(4).Compute(data)
		for _, mp := range got {
			if len(mp.Coordinates) == 0 {
				continue
			}
			x0, y0, x1, y1 := bbox(mp.Coordinates[0][0])
			return (x0 + x1) / 2, (y0 + y1) / 2
		}
		return math.NaN(), math.NaN()
	}
	fx, fy := center(2)
	cx, cy := center(8)
	if math.Abs(fx-cx) > 15 || math.Abs(fy-cy) > 15 {
		t.Errorf("contour center moved from (%v,%v) to (%v,%v) when the cell size changed", fx, fy, cx, cy)
	}
	if math.Abs(fx-200) > 20 || math.Abs(fy-200) > 20 {
		t.Errorf("contour center (%v,%v) is not near the cluster at (200,200)", fx, fy)
	}
}

func TestDensityInvalidConfigurationPanics(t *testing.T) {
	mustPanic := func(name string, f func()) {
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		f()
	}
	mustPanic("Size(-1, 1)", func() { newDensity().Size(-1, 1) })
	mustPanic("CellSize(0.5)", func() { newDensity().CellSize(0.5) })
	mustPanic("CellSize(NaN)", func() { newDensity().CellSize(math.NaN()) })
	mustPanic("Bandwidth(-1)", func() { newDensity().Bandwidth(-1) })
}

func TestDensityIgnoresPointsOutsideTheExtent(t *testing.T) {
	inside := []pt{{100, 100, 1}}
	withStrays := []pt{{100, 100, 1}, {-5000, -5000, 1}, {9000, 9000, 1}, {math.NaN(), 5, 1}}
	a, _, _ := newDensity().Size(200, 200).Grid(inside)
	b, _, _ := newDensity().Size(200, 200).Grid(withStrays)
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			t.Fatalf("cell %d differs: %v vs %v — a point outside the extent leaked in", i, a[i], b[i])
		}
	}
}

func BenchmarkDensity(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	var data []pt
	for i := 0; i < 20000; i++ {
		data = append(data, pt{rng.Float64() * 960, rng.Float64() * 500, 1})
	}
	c := newDensity()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Compute(data)
	}
}
