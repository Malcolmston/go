package color

import (
	"math"
	"testing"
)

// labTolerance is the documented sRGB → Lab → sRGB round-trip bound. See the
// tolerance discussion at the top of lab.go: the error is not floating-point
// noise but the mismatch between d3's two independently rounded conversion
// matrices, and it is three orders of magnitude below the 8-bit quantum.
const labTolerance = 5e-4

func TestLabRoundTrip(t *testing.T) {
	worst := 0.0
	for _, name := range NamedColors() {
		c := MustParse(name).RGBA()
		got := ToLab(c).RGBA()
		for _, d := range []float64{got.R - c.R, got.G - c.G, got.B - c.B} {
			if math.Abs(d) > worst {
				worst = math.Abs(d)
			}
		}
		if !closeRGB(got, c, labTolerance) {
			t.Errorf("%s: RGB->Lab->RGB = %+v, want %+v (tolerance %v)", name, got, c, labTolerance)
		}
	}
	t.Logf("worst Lab round-trip error over %d named colors: %g", len(NamedColors()), worst)
	if worst > labTolerance {
		t.Errorf("worst error %g exceeds documented tolerance %g", worst, labTolerance)
	}
}

func TestHCLRoundTrip(t *testing.T) {
	for _, name := range NamedColors() {
		c := MustParse(name).RGBA()
		got := ToHCL(c).RGBA()
		if !closeRGB(got, c, labTolerance) {
			t.Errorf("%s: RGB->HCL->RGB = %+v, want %+v", name, got, c)
		}
	}
}

func TestCubehelixRoundTrip(t *testing.T) {
	// Cubehelix does not go through the Lab matrices, so it round-trips to
	// plain floating-point precision.
	for _, name := range NamedColors() {
		c := MustParse(name).RGBA()
		got := ToCubehelix(c).RGBA()
		if !closeRGB(got, c, 1e-9) {
			t.Errorf("%s: RGB->Cubehelix->RGB = %+v, want %+v", name, got, c)
		}
	}
}

// TestLabHCLEquivalence checks the polar relationship: HCL is Lab in
// cylindrical coordinates, so converting one to the other and back must be
// lossless beyond float error, with no trip through sRGB.
func TestLabHCLEquivalence(t *testing.T) {
	for _, name := range NamedColors() {
		c := MustParse(name)
		l := ToLab(c)
		back := ToHCL(l).ToLab()
		if math.Abs(back.L-l.L) > 1e-9 || math.Abs(back.A-l.A) > 1e-9 || math.Abs(back.B-l.B) > 1e-9 {
			t.Errorf("%s: Lab->HCL->Lab = %+v, want %+v", name, back, l)
		}
	}
}

// TestLabWhitePoint pins the two Lab values that are true by definition rather
// than by measurement: the reference white has L = 100 and no chroma, and black
// has L = 0. Everything in between is checked by round trip rather than by
// asserted constants.
func TestLabWhitePoint(t *testing.T) {
	w := ToLab(MustParse("white"))
	if math.Abs(w.L-100) > 1e-6 {
		t.Errorf("white L = %v, want 100", w.L)
	}
	if math.Abs(w.A) > 1e-6 || math.Abs(w.B) > 1e-6 {
		t.Errorf("white chroma = (%v, %v), want (0, 0)", w.A, w.B)
	}
	b := ToLab(MustParse("black"))
	if math.Abs(b.L) > 1e-6 {
		t.Errorf("black L = %v, want 0", b.L)
	}
	if math.Abs(b.A) > 1e-6 || math.Abs(b.B) > 1e-6 {
		t.Errorf("black chroma = (%v, %v), want (0, 0)", b.A, b.B)
	}
}

// TestLabGraysAreAchromatic checks the neutral shortcut in ToLab: a gray must
// have exactly zero chroma, and therefore an undefined (NaN) HCL hue, with no
// rounding dust turning it into a faint tint.
func TestLabGraysAreAchromatic(t *testing.T) {
	for v := 0.0; v <= 255; v += 17 {
		g := NewRGB(v, v, v)
		l := ToLab(g)
		if l.A != 0 || l.B != 0 {
			t.Errorf("gray %v: Lab chroma = (%v, %v), want (0, 0)", v, l.A, l.B)
		}
		h := ToHCL(g)
		if !math.IsNaN(h.H) {
			t.Errorf("gray %v: HCL hue = %v, want NaN", v, h.H)
		}
		if h.C != 0 {
			t.Errorf("gray %v: HCL chroma = %v, want 0", v, h.C)
		}
	}
}

// TestLabLightnessMonotone verifies the property that makes Lab useful: L
// increases strictly with luminance along the gray ramp, and grays are ordered
// the same way in Lab as they are in sRGB.
func TestLabLightnessMonotone(t *testing.T) {
	prev := math.Inf(-1)
	for v := 0.0; v <= 255; v += 5 {
		l := ToLab(NewRGB(v, v, v)).L
		if l <= prev {
			t.Fatalf("L not increasing at gray %v: %v after %v", v, l, prev)
		}
		prev = l
	}
}

// TestLabBrighterDarker checks that Lab lightness steps are additive and
// exactly reversible, unlike the multiplicative sRGB ones — including that they
// can brighten black, which RGB.Brighter cannot.
func TestLabBrighterDarker(t *testing.T) {
	c := ToLab(MustParse("steelblue"))
	for _, k := range []float64{0, 1, 2.5, -1} {
		if got := c.Brighter(k).Darker(k); math.Abs(got.L-c.L) > 1e-12 {
			t.Errorf("Lab Brighter(%v).Darker(%v) L = %v, want %v", k, k, got.L, c.L)
		}
	}
	if got := NewLab(0, 0, 0).Brighter(1); got.L != 18 {
		t.Errorf("Lab black brightened to L = %v, want 18", got.L)
	}
	if got := ToHCL(MustParse("steelblue")); math.Abs(got.Brighter(1).Darker(1).L-got.L) > 1e-12 {
		t.Error("HCL Brighter/Darker not inverse")
	}
}

// TestOutOfGamut documents the explicit handling of colors sRGB cannot show:
// the conversion produces out-of-range channels rather than garbage, reports
// itself as not displayable, and clamps only when asked.
func TestOutOfGamut(t *testing.T) {
	// A very saturated Lab color well outside sRGB.
	c := NewLab(50, 120, -100).RGBA()
	if c.Displayable() {
		t.Fatalf("expected %+v to be outside the sRGB gamut", c)
	}
	if math.IsNaN(c.R) || math.IsNaN(c.G) || math.IsNaN(c.B) {
		t.Errorf("out-of-gamut conversion produced NaN: %+v", c)
	}
	// At least one channel must actually be out of range, not merely flagged.
	if c.R >= 0 && c.R <= 255 && c.G >= 0 && c.G <= 255 && c.B >= 0 && c.B <= 255 {
		t.Errorf("Displayable() false but all channels in range: %+v", c)
	}
	// Clamping resolves it, and formatting clamps implicitly.
	if !c.Clamp().Displayable() {
		t.Error("Clamp() did not produce a displayable color")
	}
	if s := c.String(); s == "" {
		t.Error("String() of an out-of-gamut color produced nothing")
	}
	if _, err := Parse(c.String()); err != nil {
		t.Errorf("String() of an out-of-gamut color is not parseable: %v", err)
	}
	// High-chroma HCL is the common way to land outside the gamut.
	if NewHCL(120, 150, 50).RGBA().Displayable() {
		t.Error("expected chroma 150 at L=50 to be outside the sRGB gamut")
	}
}

// TestConversionsAreIdentityOnOwnType checks the fast paths: converting a value
// to the space it already occupies must return it untouched, not send it
// through sRGB and back.
func TestConversionsAreIdentityOnOwnType(t *testing.T) {
	lab := NewLab(41.2, 87.3, -12.5)
	if got := ToLab(lab); got != lab {
		t.Errorf("ToLab(Lab) = %+v, want %+v", got, lab)
	}
	hcl := NewHCL(210, 45, 52)
	if got := ToHCL(hcl); got != hcl {
		t.Errorf("ToHCL(HCL) = %+v, want %+v", got, hcl)
	}
	if got := ToLab(hcl); got != hcl.ToLab() {
		t.Errorf("ToLab(HCL) did not use the direct polar conversion")
	}
	hsl := NewHSL(207, 0.44, 0.49)
	if got := ToHSL(hsl); got != hsl {
		t.Errorf("ToHSL(HSL) = %+v, want %+v", got, hsl)
	}
	ch := NewCubehelix(300, 0.5, 0.5)
	if got := ToCubehelix(ch); got != ch {
		t.Errorf("ToCubehelix(Cubehelix) = %+v, want %+v", got, ch)
	}
	rgb := NewRGB(1, 2, 3)
	if got := ToRGB(rgb); got != rgb {
		t.Errorf("ToRGB(RGB) = %+v, want %+v", got, rgb)
	}
}

// TestOpacityPreserved checks that every conversion carries opacity through
// untouched — a fade must survive a change of color space.
func TestOpacityPreserved(t *testing.T) {
	base := NewRGBA(70, 130, 180, 0.375)
	for name, got := range map[string]float64{
		"HSL":       ToHSL(base).Opacity,
		"Lab":       ToLab(base).Opacity,
		"HCL":       ToHCL(base).Opacity,
		"Cubehelix": ToCubehelix(base).Opacity,
	} {
		if got != 0.375 {
			t.Errorf("%s dropped opacity: %v", name, got)
		}
	}
	// And back again.
	for name, got := range map[string]RGB{
		"HSL":       ToHSL(base).RGBA(),
		"Lab":       ToLab(base).RGBA(),
		"HCL":       ToHCL(base).RGBA(),
		"Cubehelix": ToCubehelix(base).RGBA(),
	} {
		if got.Opacity != 0.375 {
			t.Errorf("%s -> RGB dropped opacity: %v", name, got.Opacity)
		}
	}
}

// TestCubehelixLuminanceMonotone checks the property Cubehelix exists for: at
// fixed hue and saturation, relative luminance rises monotonically with L, so
// the ramp survives conversion to grayscale.
func TestCubehelixLuminanceMonotone(t *testing.T) {
	lum := func(c RGB) float64 {
		// Rec. 709 relative luminance on gamma-decoded channels.
		return 0.2126*rgb2lrgb(c.R) + 0.7152*rgb2lrgb(c.G) + 0.0722*rgb2lrgb(c.B)
	}
	for _, h := range []float64{0, 90, 180, 270} {
		prev := math.Inf(-1)
		for l := 0.0; l <= 1.0001; l += 0.05 {
			y := lum(NewCubehelix(h, 1, l).RGBA())
			if y <= prev {
				t.Fatalf("hue %v: luminance not increasing at L=%v (%v after %v)", h, l, y, prev)
			}
			prev = y
		}
	}
}

func TestCubehelixAchromatic(t *testing.T) {
	for _, name := range []string{"black", "white", "gray"} {
		c := ToCubehelix(MustParse(name))
		if c.S != 0 && !math.IsNaN(c.S) {
			t.Errorf("%s: cubehelix saturation = %v, want 0", name, c.S)
		}
		if !math.IsNaN(c.H) {
			t.Errorf("%s: cubehelix hue = %v, want NaN", name, c.H)
		}
	}
}

// TestLabStringGoesThroughRGB checks that the perceptual spaces format as CSS
// sRGB, since CSS has no lab()/lch() form this package commits to.
func TestLabStringGoesThroughRGB(t *testing.T) {
	c := MustParse("steelblue")
	want := c.String()
	for name, got := range map[string]string{
		"Lab":       ToLab(c).String(),
		"HCL":       ToHCL(c).String(),
		"Cubehelix": ToCubehelix(c).String(),
	} {
		if got != want {
			t.Errorf("%s.String() = %q, want %q", name, got, want)
		}
	}
}

// TestGammaTransferRoundTrip checks the sRGB transfer function and its inverse
// independently of everything else, including for out-of-range input, which
// must pass through rather than being clamped.
func TestGammaTransferRoundTrip(t *testing.T) {
	for v := -50.0; v <= 320; v += 3 {
		got := lrgb2rgb(rgb2lrgb(v))
		if math.Abs(got-v) > 1e-9 {
			t.Errorf("lrgb2rgb(rgb2lrgb(%v)) = %v", v, got)
		}
	}
	// The breakpoints of the piecewise definition must be continuous.
	if math.Abs(rgb2lrgb(0.04045*255)-0.0031308) > 1e-6 {
		t.Error("sRGB transfer function is discontinuous at its breakpoint")
	}
}

// TestAlphaConstructors covers the opacity-carrying constructors, which exist
// because the zero value of every color type is transparent rather than opaque.
func TestAlphaConstructors(t *testing.T) {
	if got := NewLabA(50, 10, -10, 0.25); got.Opacity != 0.25 || got.L != 50 {
		t.Errorf("NewLabA = %+v", got)
	}
	if got := NewHCLA(210, 45, 52, 0.25); got.Opacity != 0.25 || got.H != 210 {
		t.Errorf("NewHCLA = %+v", got)
	}
	if got := NewCubehelixA(300, 0.5, 0.5, 0.25); got.Opacity != 0.25 || got.H != 300 {
		t.Errorf("NewCubehelixA = %+v", got)
	}
	// The zero value really is transparent, which is why the constructors exist.
	if (RGB{R: 255}).String() != "rgba(255, 0, 0, 0)" {
		t.Error("zero-value RGB is not transparent")
	}
}

func TestCubehelixBrighterDarker(t *testing.T) {
	c := NewCubehelix(200, 0.8, 0.4)
	if got := c.Brighter(1).Darker(1); math.Abs(got.L-c.L) > 1e-12 {
		t.Errorf("Cubehelix Brighter/Darker not inverse: %v vs %v", got.L, c.L)
	}
	if c.Brighter(1).L <= c.L {
		t.Error("Cubehelix.Brighter did not increase lightness")
	}
	if c.Darker(1).L >= c.L {
		t.Error("Cubehelix.Darker did not decrease lightness")
	}
}

// TestNaNChannelsHandled checks that a NaN in a Lab or Cubehelix channel is
// treated as achromatic rather than propagating into the output, mirroring the
// NaN-hue convention.
func TestNaNChannelsHandled(t *testing.T) {
	c := Lab{L: 50, A: math.NaN(), B: math.NaN(), Opacity: 1}.RGBA()
	if math.IsNaN(c.R) || math.IsNaN(c.G) || math.IsNaN(c.B) {
		t.Errorf("Lab with NaN chroma converted to %+v", c)
	}
	ch := Cubehelix{H: math.NaN(), S: math.NaN(), L: 0.5, Opacity: 1}.RGBA()
	if math.IsNaN(ch.R) || math.IsNaN(ch.G) || math.IsNaN(ch.B) {
		t.Errorf("Cubehelix with NaN hue/saturation converted to %+v", ch)
	}
	if got := Format(NewRGB(1, 2, 3)); got != "rgb(1, 2, 3)" {
		t.Errorf("Format = %q", got)
	}
	// clampFloat maps NaN to the low bound so nothing NaN reaches CSS.
	if got := clampFloat(math.NaN(), 0, 1); got != 0 {
		t.Errorf("clampFloat(NaN) = %v, want 0", got)
	}
}
