package random

import (
	"math"
	"testing"
)

// sampleN is the sample size for the distribution moment tests. It is large
// enough that the sampling error on a mean is small relative to the tolerances
// below, and small enough that the whole suite stays under a second.
const sampleN = 200000

// stats draws n values and returns their mean and (unbiased) variance.
func stats(g Gen, n int) (mean, variance float64) {
	// Welford's online algorithm: numerically stable, and avoids holding the
	// sample in memory.
	var m, m2 float64
	for i := 1; i <= n; i++ {
		x := g()
		d := x - m
		m += d / float64(i)
		m2 += d * (x - m)
	}
	return m, m2 / float64(n-1)
}

// TestLCGFirstDraw pins the one value in this package that is derivable by hand
// rather than statistically. From a zero seed the LCG's first state is
// 0·multiplier + increment = 0x3C6EF35F, and the returned deviate is that
// divided by 2^32. If the multiplier, increment, state width or scaling were
// wrong, this would catch it; nothing else in the suite would.
func TestLCGFirstDraw(t *testing.T) {
	got := Source(0).Float64()
	want := 1013904223.0 / 4294967296.0 // 0x3C6EF35F / 2^32
	if got != want {
		t.Errorf("Source(0).Float64() = %v, want %v", got, want)
	}
}

// TestSeedNormalization covers d3's two seed conventions without duplicating
// the implementation: a seed in [0, 1) is scaled by 2^32, and any other seed has
// its absolute value taken. Both are expressed as equalities between sources,
// so the test cannot pass by accidentally agreeing with a wrong formula.
func TestSeedNormalization(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
	}{
		// 0.5 * 2^32 == 2^31, which is also what |2147483648| truncates to.
		{"fractional scaled by 2^32", 0.5, 2147483648},
		{"quarter", 0.25, 1073741824},
		{"negative seeds use absolute value", -12345, 12345},
		{"truncated toward zero", 7.9, 7},
		// Non-finite seeds map to state 0, the same state seed 0 produces.
		{"NaN maps to zero", math.NaN(), 0},
		{"infinity maps to zero", math.Inf(1), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ra, rb := Source(tc.a), Source(tc.b)
			for i := 0; i < 10; i++ {
				if x, y := ra.Float64(), rb.Float64(); x != y {
					t.Fatalf("draw %d: Source(%v) = %v, Source(%v) = %v", i, tc.a, x, tc.b, y)
				}
			}
		})
	}
}

func TestFloat64Range(t *testing.T) {
	r := Source(1)
	for i := 0; i < 100000; i++ {
		v := r.Float64()
		if v < 0 || v >= 1 {
			t.Fatalf("draw %d = %v, outside [0,1)", i, v)
		}
	}
}

// TestDeterminism is the reproducibility guarantee: the same seed yields the
// same sequence, for every distribution, including the ones that consume a
// variable number of uniforms per draw (gamma's rejection loop, binomial's
// recursion, poisson's waiting times). Those are exactly the generators where a
// stray shared-state bug would show up as a divergence.
func TestDeterminism(t *testing.T) {
	build := map[string]func(*Rand) Gen{
		"Float64":     func(r *Rand) Gen { return r.Float64 },
		"Uniform":     func(r *Rand) Gen { return r.Uniform(-2, 7) },
		"Normal":      func(r *Rand) Gen { return r.Normal(3, 2) },
		"LogNormal":   func(r *Rand) Gen { return r.LogNormal(0, 0.5) },
		"IrwinHall":   func(r *Rand) Gen { return r.IrwinHall(7) },
		"Bates":       func(r *Rand) Gen { return r.Bates(10) },
		"Exponential": func(r *Rand) Gen { return r.Exponential(2) },
		"Pareto":      func(r *Rand) Gen { return r.Pareto(6) },
		"Bernoulli":   func(r *Rand) Gen { return r.Bernoulli(0.3) },
		"Geometric":   func(r *Rand) Gen { return r.Geometric(0.2) },
		"Binomial":    func(r *Rand) Gen { return r.Binomial(100, 0.3) },
		"Gamma":       func(r *Rand) Gen { return r.Gamma(3, 2) },
		"Beta":        func(r *Rand) Gen { return r.Beta(2, 5) },
		"Weibull":     func(r *Rand) Gen { return r.Weibull(2, 0, 1) },
		"Cauchy":      func(r *Rand) Gen { return r.Cauchy(0, 1) },
		"Logistic":    func(r *Rand) Gen { return r.Logistic(0, 1) },
		"Poisson":     func(r *Rand) Gen { return r.Poisson(40) },
	}
	for name, mk := range build {
		t.Run(name, func(t *testing.T) {
			a, b := mk(Source(42)), mk(Source(42))
			for i := 0; i < 1000; i++ {
				x, y := a(), b()
				if x != y {
					t.Fatalf("draw %d diverged: %v vs %v", i, x, y)
				}
			}
			// A different seed must produce a different stream; otherwise the
			// generator is ignoring its source entirely.
			c := mk(Source(7))
			d := mk(Source(42))
			same := true
			for i := 0; i < 100; i++ {
				if c() != d() {
					same = false
				}
			}
			if same {
				t.Fatal("different seeds produced identical streams")
			}
		})
	}
}

// TestMoments checks each distribution statistically: the sample mean and
// variance of a large seeded draw must sit close to the theoretical values.
// This is the right kind of assertion for a random generator — pinning specific
// draws would test the LCG (already covered above) rather than the transform,
// and would be invented rather than derived.
//
// Tolerances are absolute and deliberately loose. The sample is seeded, so
// these tests are deterministic: a passing run stays passing.
func TestMoments(t *testing.T) {
	tests := []struct {
		name     string
		gen      func(*Rand) Gen
		mean     float64
		variance float64
		meanTol  float64
		varTol   float64
	}{
		{"Uniform(2,5)", func(r *Rand) Gen { return r.Uniform(2, 5) },
			3.5, 0.75, 0.02, 0.02},
		{"Normal(3,2)", func(r *Rand) Gen { return r.Normal(3, 2) },
			3, 4, 0.05, 0.1},
		{"LogNormal(0,0.5)", func(r *Rand) Gen { return r.LogNormal(0, 0.5) },
			math.Exp(0.125), (math.Exp(0.25) - 1) * math.Exp(0.25), 0.02, 0.03},
		{"IrwinHall(7)", func(r *Rand) Gen { return r.IrwinHall(7) },
			3.5, 7.0 / 12, 0.02, 0.02},
		{"Bates(10)", func(r *Rand) Gen { return r.Bates(10) },
			0.5, 1.0 / 120, 0.005, 0.002},
		{"Exponential(2)", func(r *Rand) Gen { return r.Exponential(2) },
			0.5, 0.25, 0.02, 0.02},
		{"Pareto(6)", func(r *Rand) Gen { return r.Pareto(6) },
			1.2, 6.0 / (25 * 4), 0.01, 0.02},
		{"Bernoulli(0.3)", func(r *Rand) Gen { return r.Bernoulli(0.3) },
			0.3, 0.21, 0.01, 0.01},
		{"Geometric(0.2)", func(r *Rand) Gen { return r.Geometric(0.2) },
			5, 20, 0.1, 1.5},
		{"Binomial(100,0.3)", func(r *Rand) Gen { return r.Binomial(100, 0.3) },
			30, 21, 0.15, 0.5},
		{"Binomial(10,0.5)", func(r *Rand) Gen { return r.Binomial(10, 0.5) },
			5, 2.5, 0.05, 0.1},
		{"Gamma(3,2)", func(r *Rand) Gen { return r.Gamma(3, 2) },
			6, 12, 0.1, 0.4},
		{"Gamma(0.5,1)", func(r *Rand) Gen { return r.Gamma(0.5, 1) },
			0.5, 0.5, 0.02, 0.06},
		{"Beta(2,5)", func(r *Rand) Gen { return r.Beta(2, 5) },
			2.0 / 7, 10.0 / (49 * 8), 0.005, 0.002},
		// Weibull(2): mean Γ(1.5) = √π/2, variance 1 - π/4.
		{"Weibull(2,0,1)", func(r *Rand) Gen { return r.Weibull(2, 0, 1) },
			math.Sqrt(math.Pi) / 2, 1 - math.Pi/4, 0.01, 0.01},
		{"Weibull(1,0,2)", func(r *Rand) Gen { return r.Weibull(1, 0, 2) },
			2, 4, 0.05, 0.1},
		{"Logistic(0,1)", func(r *Rand) Gen { return r.Logistic(0, 1) },
			0, math.Pi * math.Pi / 3, 0.05, 0.15},
		{"Poisson(9)", func(r *Rand) Gen { return r.Poisson(9) },
			9, 9, 0.05, 0.2},
		{"Poisson(40)", func(r *Rand) Gen { return r.Poisson(40) },
			40, 40, 0.15, 1.0},
		{"Poisson(2)", func(r *Rand) Gen { return r.Poisson(2) },
			2, 2, 0.03, 0.06},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mean, variance := stats(tc.gen(Source(0x5eed)), sampleN)
			if math.Abs(mean-tc.mean) > tc.meanTol {
				t.Errorf("mean = %v, want %v ± %v", mean, tc.mean, tc.meanTol)
			}
			if math.Abs(variance-tc.variance) > tc.varTol {
				t.Errorf("variance = %v, want %v ± %v", variance, tc.variance, tc.varTol)
			}
		})
	}
}

// TestCauchyQuantiles checks the one distribution whose mean and variance do not
// exist. The median and quartiles do, and are exactly a and a ± b.
func TestCauchyQuantiles(t *testing.T) {
	const n = 100000
	g := Source(11).Cauchy(4, 2)
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = g()
	}
	// A full sort is unnecessary; counting is enough and cannot be fooled by
	// the extreme tails.
	frac := func(below float64) float64 {
		c := 0
		for _, x := range xs {
			if x < below {
				c++
			}
		}
		return float64(c) / n
	}
	tests := []struct {
		name string
		at   float64
		want float64
	}{
		{"median at a", 4, 0.5},
		{"lower quartile at a-b", 2, 0.25},
		{"upper quartile at a+b", 6, 0.75},
	}
	for _, tc := range tests {
		if got := frac(tc.at); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("%s: P(X < %v) = %v, want %v", tc.name, tc.at, got, tc.want)
		}
	}
}

// TestDiscreteSupport checks that the counting distributions return whole
// numbers in their documented support. A transform bug that produced 4.5
// successes would sail past the moment tests.
func TestDiscreteSupport(t *testing.T) {
	tests := []struct {
		name     string
		gen      Gen
		min, max float64 // inclusive; max is +Inf where unbounded
	}{
		{"Bernoulli", Source(1).Bernoulli(0.4), 0, 1},
		{"Geometric", Source(2).Geometric(0.3), 1, math.Inf(1)},
		{"Binomial", Source(3).Binomial(20, 0.5), 0, 20},
		{"Binomial large", Source(4).Binomial(500, 0.4), 0, 500},
		{"Poisson", Source(5).Poisson(3), 0, math.Inf(1)},
		{"Poisson large", Source(6).Poisson(50), 0, math.Inf(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20000; i++ {
				v := tc.gen()
				if v != math.Trunc(v) {
					t.Fatalf("draw %d = %v, not an integer", i, v)
				}
				if v < tc.min || v > tc.max {
					t.Fatalf("draw %d = %v, outside [%v, %v]", i, v, tc.min, tc.max)
				}
			}
		})
	}
}

// TestBoundedSupport checks the continuous distributions with a bounded range.
func TestBoundedSupport(t *testing.T) {
	tests := []struct {
		name     string
		gen      Gen
		min, max float64
	}{
		{"Uniform", Source(1).Uniform(-3, 8), -3, 8},
		{"Bates", Source(2).Bates(5), 0, 1},
		{"IrwinHall", Source(3).IrwinHall(4), 0, 4},
		{"Beta", Source(4).Beta(0.5, 0.5), 0, 1},
		{"Pareto", Source(5).Pareto(3), 1, math.Inf(1)},
		{"Exponential", Source(6).Exponential(1), 0, math.Inf(1)},
		{"Gamma", Source(7).Gamma(2, 3), 0, math.Inf(1)},
		{"LogNormal", Source(8).LogNormal(0, 1), 0, math.Inf(1)},
		{"Weibull", Source(9).Weibull(1.5, 0, 1), 0, math.Inf(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 20000; i++ {
				v := tc.gen()
				if v < tc.min || v > tc.max {
					t.Fatalf("draw %d = %v, outside [%v, %v]", i, v, tc.min, tc.max)
				}
			}
		})
	}
}

// TestDegenerateCases covers the parameter values where d3 short-circuits to a
// constant or to the limiting distribution.
func TestDegenerateCases(t *testing.T) {
	r := Source(1)
	tests := []struct {
		name string
		gen  Gen
		want float64
	}{
		{"Geometric(1) always 1", r.Geometric(1), 1},
		{"Geometric(0) never succeeds", r.Geometric(0), math.Inf(1)},
		{"Bernoulli(0)", r.Bernoulli(0), 0},
		{"Bernoulli(1)", r.Bernoulli(1), 1},
		{"Binomial(n,0)", r.Binomial(10, 0), 0},
		{"Binomial(n,1)", r.Binomial(10, 1), 10},
		{"Gamma(0,theta)", r.Gamma(0, 5), 0},
		{"IrwinHall(0)", r.IrwinHall(0), 0},
		{"IrwinHall(-1)", r.IrwinHall(-1), 0},
	}
	for _, tc := range tests {
		for i := 0; i < 100; i++ {
			if got := tc.gen(); got != tc.want {
				t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}

	// Bates(0) falls back to the raw uniform source.
	b := Source(3).Bates(0)
	u := Source(3).Float64
	for i := 0; i < 10; i++ {
		if x, y := b(), u(); x != y {
			t.Fatalf("Bates(0) draw %d = %v, want the raw uniform %v", i, x, y)
		}
	}

	// Gamma(1, theta) is exactly an exponential with rate 1/theta; both should
	// consume the source identically.
	g := Source(4).Gamma(1, 2)
	e := Source(4).Exponential(0.5)
	for i := 0; i < 10; i++ {
		if x, y := g(), e(); math.Abs(x-y) > 1e-12 {
			t.Fatalf("Gamma(1,2) draw %d = %v, want exponential %v", i, x, y)
		}
	}
}

func TestInvalidParametersPanic(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{"Bernoulli p<0", func() { Source(1).Bernoulli(-0.1) }},
		{"Bernoulli p>1", func() { Source(1).Bernoulli(1.1) }},
		{"Geometric p<0", func() { Source(1).Geometric(-1) }},
		{"Geometric p>1", func() { Source(1).Geometric(2) }},
		{"Pareto alpha<0", func() { Source(1).Pareto(-1) }},
		{"Gamma k<0", func() { Source(1).Gamma(-1, 1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected a panic, got none")
				}
			}()
			tc.fn()
		})
	}
}

// TestSourceFunc checks that an arbitrary uniform source drives the transforms,
// using a constant source so the expected values are exact rather than
// statistical.
func TestSourceFunc(t *testing.T) {
	const u = 0.5
	r := SourceFunc(func() float64 { return u })
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"Uniform", r.Uniform(0, 10)(), 5},
		{"Exponential", r.Exponential(1)(), -math.Log1p(-u)},
		{"Cauchy", r.Cauchy(0, 1)(), math.Tan(math.Pi * u)},
		{"Logistic", r.Logistic(0, 1)(), 0}, // log(0.5/0.5) == 0
		{"Bernoulli", r.Bernoulli(0.5)(), 1},
		{"Weibull(1)", r.Weibull(1, 0, 1)(), -math.Log1p(-u)},
	}
	for _, tc := range tests {
		if math.Abs(tc.got-tc.want) > 1e-12 {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// TestPackageLevelGenerators smoke-tests the default-source wrappers. They are
// non-deterministic by design, so all that can be asserted is that they are
// wired to the right distribution.
func TestPackageLevelGenerators(t *testing.T) {
	tests := []struct {
		name     string
		gen      Gen
		min, max float64
	}{
		{"Uniform", Uniform(1, 2), 1, 2},
		{"Bates", Bates(4), 0, 1},
		{"IrwinHall", IrwinHall(3), 0, 3},
		{"Beta", Beta(2, 2), 0, 1},
		{"Bernoulli", Bernoulli(0.5), 0, 1},
		{"Pareto", Pareto(2), 1, math.Inf(1)},
		{"Exponential", Exponential(1), 0, math.Inf(1)},
		{"Geometric", Geometric(0.5), 1, math.Inf(1)},
		{"Binomial", Binomial(10, 0.5), 0, 10},
		{"Gamma", Gamma(2, 1), 0, math.Inf(1)},
		{"Weibull", Weibull(2, 0, 1), 0, math.Inf(1)},
		{"LogNormal", LogNormal(0, 1), 0, math.Inf(1)},
		{"Poisson", Poisson(4), 0, math.Inf(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 1000; i++ {
				if v := tc.gen(); v < tc.min || v > tc.max {
					t.Fatalf("draw %v outside [%v, %v]", v, tc.min, tc.max)
				}
			}
		})
	}
	// Unbounded ones: just check they produce finite numbers.
	for _, g := range []Gen{Float64, Normal(0, 1), Cauchy(0, 1), Logistic(0, 1)} {
		for i := 0; i < 1000; i++ {
			if v := g(); math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("non-finite draw %v", v)
			}
		}
	}
}

// TestNormalCaching pins the polar method's pairing behaviour: the generator
// returns two deviates per pair of accepted uniforms, so consuming an even
// number of draws leaves the source at a predictable point. This is the
// structural detail that makes the Go and JavaScript streams line up.
func TestNormalCaching(t *testing.T) {
	r := Source(99)
	n := r.Normal(0, 1)
	n()
	n() // second of the pair: served from the cache, must not touch the source
	after := r.Float64()

	r2 := Source(99)
	n2 := r2.Normal(0, 1)
	n2()
	after2 := r2.Float64()

	if after != after2 {
		t.Fatalf("the second deviate consumed from the source: %v vs %v", after, after2)
	}

	// ...and the third draw must go back to the source, so two normals from the
	// same seed diverge from a source that was advanced in between.
	r3 := Source(99)
	n3 := r3.Normal(0, 1)
	n3()
	n3()
	third := n3()
	r4 := Source(99)
	n4 := r4.Normal(0, 1)
	n4()
	n4()
	r4.Float64() // steal a uniform
	if third == n4() {
		t.Fatal("the third deviate did not draw from the source")
	}
}
