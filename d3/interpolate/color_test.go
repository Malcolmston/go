package interpolate

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/color"
)

// colorInterps is the table of every color-space interpolator, used by the
// shared property tests below. Properties, not values, are what these check:
// endpoint exactness, opacity handling and behavior with achromatic endpoints
// are true regardless of the space, while the intermediate colors themselves
// are the whole point of having different spaces and are not asserted.
var colorInterps = []struct {
	name string
	f    func(a, b color.Color) func(float64) color.Color
}{
	{"RGB", RGB},
	{"HSL", HSL},
	{"HSLLong", HSLLong},
	{"Lab", Lab},
	{"HCL", HCL},
	{"HCLLong", HCLLong},
	{"Cubehelix", Cubehelix},
	{"CubehelixLong", CubehelixLong},
}

// TestColorEndpoints is the contract a sibling scale package depends on: every
// color interpolator reproduces its endpoints in sRGB at t = 0 and t = 1.
//
// The tolerance is not zero because most of these convert into another space
// and back, and the conversions are not exact inverses (see the tolerance note
// in the color package). It is well below the 8-bit quantum, so no rendered
// color ever differs.
func TestColorEndpoints(t *testing.T) {
	const tol = 1e-3
	pairs := [][2]string{
		{"white", "black"},
		{"red", "blue"},
		{"steelblue", "orange"},
		{"#123456", "#fedcba"},
		{"rebeccapurple", "lime"},
		{"gray", "red"}, // achromatic endpoint
		{"red", "gray"},
		{"gray", "silver"}, // both achromatic
		{"red", "red"},     // degenerate
	}
	for _, ci := range colorInterps {
		for _, p := range pairs {
			a := color.MustParse(p[0])
			b := color.MustParse(p[1])
			f := ci.f(a, b)
			if got, want := f(0).RGBA(), a.RGBA(); !closeRGB(got, want, tol) {
				t.Errorf("%s(%s, %s)(0) = %+v, want %+v", ci.name, p[0], p[1], got, want)
			}
			if got, want := f(1).RGBA(), b.RGBA(); !closeRGB(got, want, tol) {
				t.Errorf("%s(%s, %s)(1) = %+v, want %+v", ci.name, p[0], p[1], got, want)
			}
		}
	}
}

// TestColorOpacity checks that opacity is interpolated linearly in every space,
// so that fading and shifting hue at once behaves as expected.
func TestColorOpacity(t *testing.T) {
	a := color.NewRGBA(255, 0, 0, 0)
	b := color.NewRGBA(0, 0, 255, 1)
	for _, ci := range colorInterps {
		f := ci.f(a, b)
		if got := f(0.5).RGBA().Opacity; math.Abs(got-0.5) > 1e-9 {
			t.Errorf("%s: opacity at t=0.5 = %v, want 0.5", ci.name, got)
		}
		if got := f(0).RGBA().Opacity; got != 0 {
			t.Errorf("%s: opacity at t=0 = %v, want 0", ci.name, got)
		}
		if got := f(1).RGBA().Opacity; got != 1 {
			t.Errorf("%s: opacity at t=1 = %v, want 1", ci.name, got)
		}
	}
}

// TestColorNoNaN checks that no interpolator emits NaN channels anywhere in or
// near the range, including for achromatic endpoints where hue is undefined.
func TestColorNoNaN(t *testing.T) {
	pairs := [][2]string{
		{"gray", "red"},
		{"red", "gray"},
		{"white", "black"},
		{"black", "white"},
		{"gray", "gray"},
	}
	for _, ci := range colorInterps {
		for _, p := range pairs {
			f := ci.f(color.MustParse(p[0]), color.MustParse(p[1]))
			for x := -0.5; x <= 1.5; x += 0.05 {
				c := f(x).RGBA()
				if math.IsNaN(c.R) || math.IsNaN(c.G) || math.IsNaN(c.B) || math.IsNaN(c.Opacity) {
					t.Fatalf("%s(%s, %s)(%v) = %+v has NaN", ci.name, p[0], p[1], x, c)
				}
			}
		}
	}
}

// TestColorExtrapolates checks that t outside [0, 1] keeps moving in the same
// direction rather than being clamped — the package's documented behavior.
func TestColorExtrapolates(t *testing.T) {
	f := RGB(color.NewRGB(0, 0, 0), color.NewRGB(100, 100, 100))
	if got := f(2).RGBA(); got.R != 200 {
		t.Errorf("RGB extrapolation at t=2 gave R=%v, want 200 (unclamped)", got.R)
	}
	if got := f(-1).RGBA(); got.R != -100 {
		t.Errorf("RGB extrapolation at t=-1 gave R=%v, want -100 (unclamped)", got.R)
	}
	// The unclamped value still formats as a legal CSS color.
	if got := f(2).String(); got != "rgb(200, 200, 200)" {
		t.Errorf("f(2).String() = %q", got)
	}
	if got := f(-1).String(); got != "rgb(0, 0, 0)" {
		t.Errorf("f(-1).String() = %q, want the clamped rgb(0, 0, 0)", got)
	}
}

// TestRGBIsLinearPerChannel pins the one interpolator whose intermediate values
// are fully determined: sRGB blends each channel linearly, so the midpoint of
// red and blue is exactly (127.5, 0, 127.5).
func TestRGBIsLinearPerChannel(t *testing.T) {
	f := RGB(color.MustParse("red"), color.MustParse("blue"))
	got := f(0.5).RGBA()
	want := color.RGB{R: 127.5, G: 0, B: 127.5, Opacity: 1}
	if !closeRGB(got, want, 1e-9) {
		t.Errorf("RGB(red, blue)(0.5) = %+v, want %+v", got, want)
	}
	// And it is monotone per channel.
	prev := math.Inf(-1)
	for x := 0.0; x <= 1; x += 0.02 {
		v := f(x).RGBA().B
		if v < prev {
			t.Fatalf("blue channel not monotone at t=%v", x)
		}
		prev = v
	}
}

// TestHSLTakesShortestHuePath checks the shortest-path rule. Red (hue 0) to
// magenta (hue 300) is 60 degrees the short way (through hue 330, pink), not
// 300 degrees the long way (through green and blue).
func TestHSLTakesShortestHuePath(t *testing.T) {
	f := HSL(color.MustParse("red"), color.MustParse("magenta"))
	mid := color.ToHSL(f(0.5))
	if got := math.Mod(mid.H+360, 360); math.Abs(got-330) > 1e-6 {
		t.Errorf("HSL(red, magenta)(0.5) hue = %v, want 330 (the short way)", got)
	}
	// The long variant goes the other way, through 150.
	long := color.ToHSL(HSLLong(color.MustParse("red"), color.MustParse("magenta"))(0.5))
	if got := math.Mod(long.H+360, 360); math.Abs(got-150) > 1e-6 {
		t.Errorf("HSLLong(red, magenta)(0.5) hue = %v, want 150 (the long way)", got)
	}
}

// TestHueWrapAcrossZero is the case shortest-path exists for: 350 to 10 must be
// a 20-degree step through red, not a 340-degree sweep backwards.
func TestHueWrapAcrossZero(t *testing.T) {
	a := color.NewHSL(350, 1, 0.5)
	b := color.NewHSL(10, 1, 0.5)
	mid := color.ToHSL(HSL(a, b)(0.5))
	if got := math.Mod(mid.H+720, 360); math.Abs(got) > 1e-6 && math.Abs(got-360) > 1e-6 {
		t.Errorf("HSL(350, 10)(0.5) hue = %v (normalized %v), want 0/360", mid.H, got)
	}
	// The sweep must stay within 20 degrees of the endpoints the whole way.
	f := HSL(a, b)
	for x := 0.0; x <= 1; x += 0.05 {
		h := math.Mod(color.ToHSL(f(x)).H+720, 360)
		if h > 20 && h < 340 {
			t.Fatalf("hue left the short arc at t=%v: %v", x, h)
		}
	}
}

// TestAchromaticEndpointHoldsHue checks that fading gray to red keeps red's hue
// throughout instead of sweeping the spectrum, which is the whole reason the
// color types report NaN for an undefined hue.
func TestAchromaticEndpointHoldsHue(t *testing.T) {
	f := HSL(color.MustParse("gray"), color.MustParse("red"))
	for x := 0.1; x <= 1; x += 0.1 {
		h := color.ToHSL(f(x)).H
		if math.IsNaN(h) {
			continue
		}
		if hh := math.Mod(h+360, 360); hh > 1e-6 && hh < 360-1e-6 {
			t.Fatalf("HSL(gray, red)(%v) hue = %v, want red's hue of 0", x, h)
		}
	}
	// Same for HCL, whose achromatic hue is also NaN.
	g := HCL(color.MustParse("gray"), color.MustParse("red"))
	target := color.ToHCL(color.MustParse("red")).H
	for x := 0.1; x <= 1; x += 0.1 {
		if h := color.ToHCL(g(x)).H; !math.IsNaN(h) && math.Abs(h-target) > 1 {
			t.Fatalf("HCL(gray, red)(%v) hue = %v, want %v", x, h, target)
		}
	}
}

// TestLabLightnessMonotone checks the property that makes Lab the right default
// for a sequential scale: interpolating white to black passes through
// monotonically decreasing perceptual lightness with no bright or dark bands.
func TestLabLightnessMonotone(t *testing.T) {
	f := Lab(color.MustParse("white"), color.MustParse("black"))
	prev := math.Inf(1)
	for x := 0.0; x <= 1.0001; x += 0.01 {
		l := color.ToLab(f(x)).L
		if l > prev {
			t.Fatalf("Lab lightness not decreasing at t=%v: %v after %v", x, l, prev)
		}
		prev = l
	}
	// And it is uniform: equal steps in t give equal steps in L, which is not
	// true of the same interpolation in sRGB.
	step := color.ToLab(f(0.1)).L - color.ToLab(f(0)).L
	for x := 0.0; x < 0.9; x += 0.1 {
		got := color.ToLab(f(x+0.1)).L - color.ToLab(f(x)).L
		if math.Abs(got-step) > 1e-6 {
			t.Fatalf("Lab step at t=%v is %v, want the uniform %v", x, got, step)
		}
	}
}

// TestLabMidpointStaysBright is the qualitative reason to prefer Lab over sRGB:
// the midpoint of white and blue is far lighter in Lab than the muddy sRGB one.
func TestLabMidpointStaysBright(t *testing.T) {
	white := color.MustParse("white")
	blue := color.MustParse("blue")
	labMid := color.ToLab(Lab(white, blue)(0.5)).L
	rgbMid := color.ToLab(RGB(white, blue)(0.5)).L
	if labMid <= rgbMid {
		t.Errorf("Lab midpoint L = %v, not brighter than the sRGB midpoint L = %v", labMid, rgbMid)
	}
}

// TestCubehelixLuminanceMonotone checks the grayscale-safety property that
// motivates cubehelix: luminance rises monotonically along the ramp.
func TestCubehelixLuminanceMonotone(t *testing.T) {
	lum := func(c color.RGB) float64 {
		// Approximate relative luminance on the gamma-encoded channels; the
		// ordering is all this test needs.
		return 0.2126*c.R + 0.7152*c.G + 0.0722*c.B
	}
	f := CubehelixLong(color.NewCubehelix(-100, 0.75, 0), color.NewCubehelix(80, 1.5, 1))
	prev := math.Inf(-1)
	for x := 0.0; x <= 1.0001; x += 0.02 {
		y := lum(f(x).RGBA().Clamp())
		if y < prev-1e-9 {
			t.Fatalf("cubehelix luminance dropped at t=%v: %v after %v", x, y, prev)
		}
		prev = y
	}
}

// TestPiecewiseWithColors exercises the combination a multi-stop color scale
// actually uses.
func TestPiecewiseWithColors(t *testing.T) {
	stops := []color.Color{
		color.MustParse("#440154"),
		color.MustParse("#21918c"),
		color.MustParse("#fde725"),
	}
	f := Piecewise(Lab, stops)
	for i, want := range stops {
		x := float64(i) / 2
		if got := f(x).RGBA(); !closeRGB(got, want.RGBA(), 1e-3) {
			t.Errorf("Piecewise(Lab)(%v) = %+v, want %+v", x, got, want.RGBA())
		}
	}
}

func closeRGB(a, b color.RGB, tol float64) bool {
	return math.Abs(a.R-b.R) <= tol && math.Abs(a.G-b.G) <= tol &&
		math.Abs(a.B-b.B) <= tol && math.Abs(a.Opacity-b.Opacity) <= tol
}
