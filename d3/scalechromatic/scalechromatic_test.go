package scalechromatic

import (
	"math"
	"strings"
	"testing"

	"github.com/malcolmston/d3/color"
)

func hex(c color.Color) string { return color.ToRGB(c).Hex() }

// --- Discrete schemes ------------------------------------------------------

// TestCategoricalLengths pins the size of every categorical scheme. A
// categorical scheme's length is the only thing about it a caller can reason
// about without looking at it — "I have eleven series, Set2 will not do" — so a
// change here is a breaking change even when every color is still correct.
func TestCategoricalLengths(t *testing.T) {
	want := map[string]int{
		"Category10": 10, "Accent": 8, "Dark2": 8, "Paired": 12,
		"Pastel1": 9, "Pastel2": 8, "Set1": 9, "Set2": 8, "Set3": 12,
		"Tableau10": 10, "Observable10": 10,
	}
	for name, n := range want {
		cs, ok := Scheme(name)
		if !ok {
			t.Errorf("Scheme(%q): not registered", name)
			continue
		}
		if len(cs) != n {
			t.Errorf("Scheme(%q): len = %d, want %d", name, len(cs), n)
		}
	}
	if got := len(schemeTable); got != len(want) {
		t.Errorf("schemeTable has %d entries, want %d", got, len(want))
	}
}

// TestFamilyShapes is the strongest structural check available on the
// ColorBrewer tables: a family published for k classes must contain exactly k
// colors at index k. A transposed, truncated or duplicated line in data.go
// breaks this immediately, even though no test can catch a wrong hex digit.
func TestFamilyShapes(t *testing.T) {
	diverging := map[string]bool{
		"BrBG": true, "PRGn": true, "PiYG": true, "PuOr": true, "RdBu": true,
		"RdGy": true, "RdYlBu": true, "RdYlGn": true, "Spectral": true,
	}
	if len(familyTable) != 27 {
		t.Errorf("familyTable has %d families, want 27", len(familyTable))
	}
	for _, name := range SchemeFamilyNames() {
		f, ok := SchemeFamily(name)
		if !ok {
			t.Fatalf("SchemeFamily(%q): not registered", name)
		}
		wantMax := 9
		if diverging[name] {
			wantMax = 11
		}
		if f.Min() != 3 || f.Max() != wantMax {
			t.Errorf("%s: k range %d..%d, want 3..%d", name, f.Min(), f.Max(), wantMax)
		}
		for k := range f {
			switch {
			case k < 3:
				if f[k] != nil {
					t.Errorf("%s[%d] = %v, want nil", name, k, f[k])
				}
			default:
				if len(f[k]) != k {
					t.Errorf("%s[%d] has %d colors, want %d", name, k, len(f[k]), k)
				}
			}
		}
		if got := len(f.Largest()); got != wantMax {
			t.Errorf("%s.Largest() has %d colors, want %d", name, got, wantMax)
		}
	}
}

// TestFamilyK checks the bounds-safe accessor, which is the one to use when k is
// data rather than a literal.
func TestFamilyK(t *testing.T) {
	if got := len(SchemeBlues.K(5)); got != 5 {
		t.Errorf("SchemeBlues.K(5) has %d colors, want 5", got)
	}
	for _, k := range []int{-1, 0, 2, 10, 1000} {
		if cs := SchemeBlues.K(k); cs != nil {
			t.Errorf("SchemeBlues.K(%d) = %v, want nil", k, cs)
		}
	}
	var empty Family
	if empty.Min() != 0 || empty.Max() != 0 || empty.Largest() != nil {
		t.Error("the zero Family should report 0, 0, nil")
	}
}

// TestKnownEndpoints spot-checks only the values that are unambiguous and
// independently memorable — the ends of the greyscale, the ends of the
// single-hue sequential ramps, and the first color of the categorical schemes
// everyone recognizes. Interior ColorBrewer values are deliberately not asserted
// here: they were extracted mechanically rather than transcribed, and an
// assertion written from the same source would test nothing.
func TestKnownEndpoints(t *testing.T) {
	cases := []struct {
		name string
		got  color.Color
		want string
	}{
		// Greys runs pure white to pure black at k = 9. Anything else is wrong
		// by inspection, without consulting a table.
		{"SchemeGreys[9][0]", SchemeGreys[9][0], "#ffffff"},
		{"SchemeGreys[9][8]", SchemeGreys[9][8], "#000000"},

		// The single-hue sequential families all start at a near-white tint of
		// their hue and end at a dark shade of it.
		{"SchemeBlues[9][0]", SchemeBlues[9][0], "#f7fbff"},
		{"SchemeBlues[9][8]", SchemeBlues[9][8], "#08306b"},
		{"SchemeGreens[9][0]", SchemeGreens[9][0], "#f7fcf5"},
		{"SchemeReds[9][0]", SchemeReds[9][0], "#fff5f0"},

		// The first color of the two categorical schemes that appear in every
		// d3 example ever published.
		{"SchemeCategory10[0]", SchemeCategory10[0], "#1f77b4"},
		{"SchemeCategory10[1]", SchemeCategory10[1], "#ff7f0e"},
		{"SchemeSet1[0]", SchemeSet1[0], "#e41a1c"},
		{"SchemeTableau10[0]", SchemeTableau10[0], "#4e79a7"},

		// RdBu is symmetric about a neutral, and at odd k that neutral is
		// exactly the near-white #f7f7f7 shared by every ColorBrewer diverging
		// family that has one.
		{"SchemeRdBu[11][5]", SchemeRdBu[11][5], "#f7f7f7"},
		{"SchemePRGn[11][5]", SchemePRGn[11][5], "#f7f7f7"},
		{"SchemePuOr[11][5]", SchemePuOr[11][5], "#f7f7f7"},
	}
	for _, c := range cases {
		if got := hex(c.got); got != c.want {
			t.Errorf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

// TestSchemeColorsAreOpaqueAndDisplayable checks the decoded tables rather than
// the ramps: a specifier that decoded wrongly would most likely show up as a
// channel outside 0-255 or a zero opacity.
func TestSchemeColorsAreOpaqueAndDisplayable(t *testing.T) {
	check := func(label string, cs []color.Color) {
		for i, c := range cs {
			rgb := color.ToRGB(c)
			if !rgb.Displayable() {
				t.Errorf("%s[%d] = %v is not displayable", label, i, rgb)
			}
			if rgb.Opacity != 1 {
				t.Errorf("%s[%d] opacity = %v, want 1", label, i, rgb.Opacity)
			}
			if rgb.R != math.Trunc(rgb.R) || rgb.G != math.Trunc(rgb.G) || rgb.B != math.Trunc(rgb.B) {
				t.Errorf("%s[%d] = %v has a non-integer channel", label, i, rgb)
			}
		}
	}
	for name, cs := range Schemes() {
		check(name, cs)
	}
	for name, f := range SchemeFamilies() {
		for k, cs := range f {
			if cs != nil {
				check(name+"["+string(rune('0'+k/10))+string(rune('0'+k%10))+"]", cs)
			}
		}
	}
	for name, cs := range map[string][]color.Color{
		"Viridis": tableViridis, "Magma": tableMagma,
		"Inferno": tableInferno, "Plasma": tablePlasma,
	} {
		if len(cs) != 256 {
			t.Errorf("table%s has %d colors, want 256", name, len(cs))
		}
		check("table"+name, cs)
	}
}

// --- Continuous interpolators ---------------------------------------------

// allInterpolators is every ramp the package exports, by canonical name.
func allInterpolators(t *testing.T) map[string]func(float64) color.Color {
	t.Helper()
	m := Interpolators()
	if len(m) != 27+11 {
		t.Errorf("Interpolators() has %d entries, want %d", len(m), 27+11)
	}
	return m
}

// TestInterpolatorsDisplayable is the package's blanket contract: whatever t
// arrives, an interpolator returns an opaque color inside the sRGB gamut. It is
// sampled finely because the failure mode it guards against — a spline or a
// polynomial straying out of gamut somewhere in the middle — is invisible at the
// endpoints.
func TestInterpolatorsDisplayable(t *testing.T) {
	for name, f := range allInterpolators(t) {
		for i := 0; i <= 2000; i++ {
			x := float64(i) / 2000
			rgb := color.ToRGB(f(x))
			if !rgb.Displayable() {
				t.Fatalf("%s(%g) = %v is not displayable", name, x, rgb)
			}
			if rgb.Opacity != 1 {
				t.Fatalf("%s(%g) opacity = %v, want 1", name, x, rgb.Opacity)
			}
		}
		// Out-of-range and NaN inputs must not produce something unrenderable
		// either; each ramp clamps, wraps or is periodic, but none may return a
		// NaN channel.
		for _, x := range []float64{-1e6, -1.5, -0.25, 1.25, 2.5, 1e6, math.NaN()} {
			rgb := color.ToRGB(f(x))
			if !rgb.Displayable() {
				t.Errorf("%s(%g) = %v is not displayable", name, x, rgb)
			}
		}
	}
}

// TestBrewerRampEndpoints checks the property the B-spline is built to have: the
// ramp starts and ends exactly on its scheme, so a continuous legend and a
// discrete one agree at the extremes.
func TestBrewerRampEndpoints(t *testing.T) {
	for _, name := range SchemeFamilyNames() {
		f, _ := SchemeFamily(name)
		ramp, ok := Interpolator(name)
		if !ok {
			t.Fatalf("Interpolator(%q): not registered", name)
		}
		cs := f.Largest()
		if got, want := hex(ramp(0)), hex(cs[0]); got != want {
			t.Errorf("Interpolate%s(0) = %s, want %s", name, got, want)
		}
		if got, want := hex(ramp(1)), hex(cs[len(cs)-1]); got != want {
			t.Errorf("Interpolate%s(1) = %s, want %s", name, got, want)
		}
		// Below 0 and above 1 the spline is clamped, not extrapolated.
		if hex(ramp(-3)) != hex(cs[0]) || hex(ramp(3)) != hex(cs[len(cs)-1]) {
			t.Errorf("Interpolate%s does not clamp outside [0, 1]", name)
		}
	}
}

// TestTableRampsAreStepFunctions pins the deviation most likely to surprise
// someone: the matplotlib ramps are a 256-entry lookup, not a curve. t is
// mapped by floor(t*256), so the first and last 1/256th of the domain are
// constant and every sample lands exactly on a published color.
func TestTableRampsAreStepFunctions(t *testing.T) {
	for name, tbl := range map[string][]color.Color{
		"Viridis": tableViridis, "Magma": tableMagma,
		"Inferno": tableInferno, "Plasma": tablePlasma,
	} {
		f, _ := Interpolator(name)
		if got, want := hex(f(0)), hex(tbl[0]); got != want {
			t.Errorf("Interpolate%s(0) = %s, want %s", name, got, want)
		}
		if got, want := hex(f(1)), hex(tbl[255]); got != want {
			t.Errorf("Interpolate%s(1) = %s, want %s", name, got, want)
		}
		for i := 0; i < 256; i++ {
			mid := (float64(i) + 0.5) / 256
			if got, want := hex(f(mid)), hex(tbl[i]); got != want {
				t.Fatalf("Interpolate%s(%g) = %s, want table entry %d = %s", name, mid, got, i, want)
			}
		}
		// Every returned value is a table entry, never a blend of two.
		seen := make(map[string]bool, 256)
		for _, c := range tbl {
			seen[hex(c)] = true
		}
		for i := 0; i <= 5000; i++ {
			if got := hex(f(float64(i) / 5000)); !seen[got] {
				t.Fatalf("Interpolate%s produced %s, which is not in the table", name, got)
			}
		}
	}
}

// --- Computed ramps: checked against the published formula -----------------

// The formulas below are written out independently of the implementation —
// expanded polynomials rather than Horner's rule, and Dave Green's cubehelix
// equations written from the paper rather than reused from d3/color. That is the
// point: for the computed ramps there is a specification to check against, so
// this is the one part of the package where a test can catch a wrong constant.

func poly(t float64, c ...float64) float64 {
	sum, p := 0.0, 1.0
	for _, k := range c {
		sum += k * p
		p *= t
	}
	return sum
}

func TestTurboFormula(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		x := float64(i) / 1000
		want := [3]float64{
			poly(x, 34.61, 1172.33, -10793.56, 33300.12, -38394.49, 14825.05),
			poly(x, 23.31, 557.33, 1225.33, -3574.96, 1073.77, 707.56),
			poly(x, 27.2, 3211.1, -15327.97, 27814, -22569.18, 6838.66),
		}
		checkPolynomialRamp(t, "Turbo", InterpolateTurbo, x, want)
	}
}

func TestCividisFormula(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		x := float64(i) / 1000
		want := [3]float64{
			poly(x, -4.54, -35.34, 2381.73, -6402.7, 7024.72, -2710.57),
			poly(x, 32.49, 170.73, 52.82, -131.46, 176.58, -67.37),
			poly(x, 81.24, 442.36, -2482.43, 6167.24, -6614.94, 2475.67),
		}
		checkPolynomialRamp(t, "Cividis", InterpolateCividis, x, want)
	}
}

// checkPolynomialRamp compares against the unrounded polynomial with a half-unit
// tolerance. The implementation rounds to whole channels (upstream's behavior),
// and comparing rounded values would make the test hostage to a floating-point
// tie between two ways of evaluating the same polynomial; half a unit is tight
// enough that any wrong coefficient fails by orders of magnitude.
func checkPolynomialRamp(t *testing.T, name string, f func(float64) color.Color, x float64, want [3]float64) {
	t.Helper()
	got := color.ToRGB(f(x))
	for i, ch := range [3]float64{got.R, got.G, got.B} {
		w := math.Max(0, math.Min(255, want[i]))
		if ch != math.Trunc(ch) {
			t.Fatalf("Interpolate%s(%g) channel %d = %v, want a whole number", name, x, i, ch)
		}
		if math.Abs(ch-w) > 0.5+1e-9 {
			t.Fatalf("Interpolate%s(%g) channel %d = %v, want %v (clamped from %v)", name, x, i, ch, w, want[i])
		}
	}
}

// cubehelixRGB is Dave Green's cubehelix transform, written from the published
// equations rather than taken from d3/color, so that the rainbow test checks the
// ramp's definition and not merely its agreement with itself.
func cubehelixRGB(h, s, l float64) (r, g, b float64) {
	const (
		a1, b1 = -0.14861, +1.78277
		a2, b2 = -0.29227, -0.90649
		a3     = +1.97294
	)
	th := (h + 120) * math.Pi / 180
	amp := s * l * (1 - l)
	cos, sin := math.Cos(th), math.Sin(th)
	return 255 * (l + amp*(a1*cos+b1*sin)),
		255 * (l + amp*(a2*cos+b2*sin)),
		255 * (l + amp*(a3*cos))
}

func clampCh(v float64) float64 { return math.Max(0, math.Min(255, v)) }

func TestRainbowFormula(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		x := float64(i) / 1000
		ts := math.Abs(x - 0.5)
		wr, wg, wb := cubehelixRGB(360*x-100, 1.5-1.5*ts, 0.8-0.9*ts)
		got := color.ToRGB(InterpolateRainbow(x))
		for j, pair := range [3][2]float64{{got.R, clampCh(wr)}, {got.G, clampCh(wg)}, {got.B, clampCh(wb)}} {
			if math.Abs(pair[0]-pair[1]) > 1e-9 {
				t.Fatalf("InterpolateRainbow(%g) channel %d = %v, want %v", x, j, pair[0], pair[1])
			}
		}
	}
}

// TestRainbowIsCyclic checks the property that makes rainbow worth having at
// all: it closes on itself, so a wrapping domain has no seam, and t outside
// [0, 1] wraps rather than clamping.
func TestRainbowIsCyclic(t *testing.T) {
	if got, want := hex(InterpolateRainbow(0)), hex(InterpolateRainbow(1)); got != want {
		t.Errorf("InterpolateRainbow(0) = %s but (1) = %s; the ramp does not close", got, want)
	}
	for i := 0; i <= 100; i++ {
		x := float64(i) / 100
		for _, d := range []float64{-3, -1, 1, 2} {
			if got, want := hex(InterpolateRainbow(x+d)), hex(InterpolateRainbow(x)); got != want {
				t.Fatalf("InterpolateRainbow(%g) = %s, want %s (period 1)", x+d, got, want)
			}
		}
	}
}

func TestSinebowFormula(t *testing.T) {
	sq := func(x float64) float64 { return 255 * math.Pow(math.Sin(x), 2) }
	for i := -500; i <= 1500; i++ {
		x := float64(i) / 1000
		u := (0.5 - x) * math.Pi
		want := [3]float64{sq(u), sq(u + math.Pi/3), sq(u + 2*math.Pi/3)}
		got := color.ToRGB(InterpolateSinebow(x))
		for j, ch := range [3]float64{got.R, got.G, got.B} {
			if math.Abs(ch-want[j]) > 1e-9 {
				t.Fatalf("InterpolateSinebow(%g) channel %d = %v, want %v", x, j, ch, want[j])
			}
		}
	}
}

// TestSinebowConstantIntensity checks the property that squaring the sines buys:
// three sin-squared waves a third of a turn apart sum to 3/2 for every t, so
// total intensity is flat along the whole ramp. If a phase were wrong this would
// fail even though every individual channel still looked plausible.
func TestSinebowConstantIntensity(t *testing.T) {
	for i := 0; i <= 1000; i++ {
		x := float64(i) / 1000
		c := color.ToRGB(InterpolateSinebow(x))
		if got := c.R + c.G + c.B; math.Abs(got-255*1.5) > 1e-9 {
			t.Fatalf("InterpolateSinebow(%g) channels sum to %v, want %v", x, got, 255*1.5)
		}
	}
}

// TestSinebowIsPeriodic checks period 1 on the channels rather than on the hex
// string. sin is evaluated at a different argument for t and t+1, so the two
// agree to within a few ulps but can straddle a rounding boundary and render as
// adjacent hex values — which is a property of floating point, not of the ramp.
func TestSinebowIsPeriodic(t *testing.T) {
	for i := 0; i <= 100; i++ {
		x := float64(i) / 100
		a := color.ToRGB(InterpolateSinebow(x))
		b := color.ToRGB(InterpolateSinebow(x + 1))
		for j, pair := range [3][2]float64{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}} {
			if math.Abs(pair[0]-pair[1]) > 1e-9 {
				t.Errorf("InterpolateSinebow(%g) channel %d = %v, want %v (period 1)", x+1, j, pair[1], pair[0])
			}
		}
	}
}

// TestCubehelixEndpoints pins the fixed endpoints of the three cubehelix ramps,
// which are the part of their definition a typo could silently change.
func TestCubehelixEndpoints(t *testing.T) {
	// The default ramp is specified as running black to white.
	if got := hex(InterpolateCubehelixDefault(0)); got != "#000000" {
		t.Errorf("InterpolateCubehelixDefault(0) = %s, want #000000", got)
	}
	if got := hex(InterpolateCubehelixDefault(1)); got != "#ffffff" {
		t.Errorf("InterpolateCubehelixDefault(1) = %s, want #ffffff", got)
	}
	// Warm and cool share an endpoint at t = 1 by construction: both end at
	// cubehelix(80, 1.5, 0.8).
	if got, want := hex(InterpolateWarm(1)), hex(InterpolateCool(1)); got != want {
		t.Errorf("InterpolateWarm(1) = %s, InterpolateCool(1) = %s; want equal", got, want)
	}
	for _, tc := range []struct {
		name    string
		f       func(float64) color.Color
		h, s, l float64
	}{
		{"CubehelixDefault", InterpolateCubehelixDefault, 300, 0.5, 0.0},
		{"Warm", InterpolateWarm, -100, 0.75, 0.35},
		{"Cool", InterpolateCool, 260, 0.75, 0.35},
	} {
		r, g, b := cubehelixRGB(tc.h, tc.s, tc.l)
		got := color.ToRGB(tc.f(0))
		for j, pair := range [3][2]float64{{got.R, clampCh(r)}, {got.G, clampCh(g)}, {got.B, clampCh(b)}} {
			if math.Abs(pair[0]-pair[1]) > 1e-9 {
				t.Errorf("Interpolate%s(0) channel %d = %v, want %v", tc.name, j, pair[0], pair[1])
			}
		}
	}
}

// --- Registries ------------------------------------------------------------

// TestRegistryRoundTrips is what makes it safe to take a scheme name from
// configuration: every name a registry reports must resolve, in any casing, to
// the same value the registry holds.
func TestRegistryRoundTrips(t *testing.T) {
	for _, name := range SchemeNames() {
		cs, ok := Scheme(name)
		if !ok {
			t.Errorf("SchemeNames reported %q but Scheme could not find it", name)
			continue
		}
		for _, variant := range []string{strings.ToLower(name), strings.ToUpper(name), "  " + name + "  "} {
			got, ok := Scheme(variant)
			if !ok || len(got) != len(cs) {
				t.Errorf("Scheme(%q) did not resolve to the same scheme as %q", variant, name)
			}
		}
		if Schemes()[name] == nil {
			t.Errorf("Schemes() is missing %q", name)
		}
	}
	for _, name := range SchemeFamilyNames() {
		f, ok := SchemeFamily(strings.ToUpper(name))
		if !ok {
			t.Errorf("SchemeFamily(%q): not found", name)
			continue
		}
		if got := SchemeFamilies()[name]; len(got) != len(f) {
			t.Errorf("SchemeFamilies() disagrees with SchemeFamily for %q", name)
		}
	}
	for _, name := range InterpolatorNames() {
		f, ok := Interpolator(strings.ToLower(name))
		if !ok {
			t.Errorf("Interpolator(%q): not found", name)
			continue
		}
		g := Interpolators()[name]
		if g == nil {
			t.Errorf("Interpolators() is missing %q", name)
			continue
		}
		for _, x := range []float64{0, 0.25, 0.5, 0.75, 1} {
			if hex(f(x)) != hex(g(x)) {
				t.Errorf("Interpolator(%q) and Interpolators()[%q] disagree at t = %g", name, name, x)
			}
		}
	}
}

// TestRegistryNamesAreSortedAndCanonical checks the two things a caller building
// a dropdown depends on.
func TestRegistryNamesAreSortedAndCanonical(t *testing.T) {
	for _, list := range [][]string{SchemeNames(), SchemeFamilyNames(), InterpolatorNames()} {
		for i := 1; i < len(list); i++ {
			if list[i-1] >= list[i] {
				t.Errorf("names are not sorted and unique: %q then %q", list[i-1], list[i])
			}
		}
		for _, n := range list {
			if n == "" {
				t.Error("a registry produced an empty name; canonicalNames is missing a key")
			}
		}
	}
}

func TestUnknownNames(t *testing.T) {
	for _, n := range []string{"", "nope", "Viridis2", "scheme"} {
		if _, ok := Scheme(n); ok {
			t.Errorf("Scheme(%q) resolved", n)
		}
		if _, ok := SchemeFamily(n); ok {
			t.Errorf("SchemeFamily(%q) resolved", n)
		}
		if _, ok := Interpolator(n); ok {
			t.Errorf("Interpolator(%q) resolved", n)
		}
	}
	// A ColorBrewer family is in two registries and a categorical scheme in
	// one; that split is part of the API and worth pinning.
	if _, ok := Scheme("Blues"); ok {
		t.Error("Scheme(\"Blues\") resolved; a family is not a categorical scheme")
	}
	if _, ok := SchemeFamily("Category10"); ok {
		t.Error("SchemeFamily(\"Category10\") resolved; a categorical scheme is not a family")
	}
	if _, ok := Interpolator("Category10"); ok {
		t.Error("Interpolator(\"Category10\") resolved; a categorical scheme has no ramp")
	}
}

// TestRegistriesAreDefensive checks that mutating a returned map cannot affect
// the package.
func TestRegistriesAreDefensive(t *testing.T) {
	m := Interpolators()
	delete(m, "Viridis")
	if _, ok := Interpolator("Viridis"); !ok {
		t.Error("deleting from Interpolators() removed the entry from the package")
	}
	s := Schemes()
	s["Category10"] = nil
	if cs, ok := Scheme("Category10"); !ok || len(cs) != 10 {
		t.Error("mutating Schemes() affected the package")
	}
}

// --- Decoding --------------------------------------------------------------

func TestColorsRejectsMalformedSpecifiers(t *testing.T) {
	for _, s := range []string{"", "abc", "1234567", "12345g"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("colors(%q) did not panic", s)
				}
			}()
			colors(s)
		}()
	}
}

func TestColorsDecodesChannels(t *testing.T) {
	got := colors("0011ffFF8000")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if a := color.ToRGB(got[0]); a.R != 0 || a.G != 17 || a.B != 255 || a.Opacity != 1 {
		t.Errorf("first = %v, want RGB{0, 17, 255, 1}", a)
	}
	if b := color.ToRGB(got[1]); b.R != 255 || b.G != 128 || b.B != 0 {
		t.Errorf("second = %v, want RGB{255, 128, 0}", b)
	}
}
