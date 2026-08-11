package color

import (
	"math"
	"strings"
	"testing"
)

func TestParseNamed(t *testing.T) {
	tests := []struct {
		in         string
		r, g, b    float64
		opacity    float64
		wantString string
	}{
		{"black", 0, 0, 0, 1, "rgb(0, 0, 0)"},
		{"white", 255, 255, 255, 1, "rgb(255, 255, 255)"},
		{"red", 255, 0, 0, 1, "rgb(255, 0, 0)"},
		{"lime", 0, 255, 0, 1, "rgb(0, 255, 0)"},
		{"blue", 0, 0, 255, 1, "rgb(0, 0, 255)"},
		{"steelblue", 70, 130, 180, 1, "rgb(70, 130, 180)"},
		{"rebeccapurple", 102, 51, 153, 1, "rgb(102, 51, 153)"},
		{"transparent", 0, 0, 0, 0, "rgba(0, 0, 0, 0)"},
		// Case and whitespace insensitivity.
		{"  STEELBLUE  ", 70, 130, 180, 1, "rgb(70, 130, 180)"},
		{"SteelBlue", 70, 130, 180, 1, "rgb(70, 130, 180)"},
		// Aliases must agree with their partners.
		{"gray", 128, 128, 128, 1, "rgb(128, 128, 128)"},
		{"grey", 128, 128, 128, 1, "rgb(128, 128, 128)"},
		{"aqua", 0, 255, 255, 1, "rgb(0, 255, 255)"},
		{"cyan", 0, 255, 255, 1, "rgb(0, 255, 255)"},
		{"fuchsia", 255, 0, 255, 1, "rgb(255, 0, 255)"},
		{"magenta", 255, 0, 255, 1, "rgb(255, 0, 255)"},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.in), func(t *testing.T) {
			c, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.in, err)
			}
			got := c.RGBA()
			if got.R != tt.r || got.G != tt.g || got.B != tt.b || got.Opacity != tt.opacity {
				t.Errorf("Parse(%q).RGBA() = %+v, want {%v %v %v %v}",
					tt.in, got, tt.r, tt.g, tt.b, tt.opacity)
			}
			if s := c.String(); s != tt.wantString {
				t.Errorf("Parse(%q).String() = %q, want %q", tt.in, s, tt.wantString)
			}
		})
	}
}

// TestNamedColorRoundTrip runs every CSS keyword through Parse → String →
// Parse and requires the two parses to agree exactly. This is the check that
// catches a typo in the color table only if it makes the value unparseable, so
// it is paired with the count and the spot checks in TestParseNamed.
func TestNamedColorRoundTrip(t *testing.T) {
	names := NamedColors()
	if len(names) != 149 {
		t.Errorf("NamedColors() has %d entries, want 149 (148 CSS keywords + transparent)", len(names))
	}
	for _, name := range names {
		c1, err := Parse(name)
		if err != nil {
			t.Fatalf("Parse(%q): %v", name, err)
		}
		s := c1.String()
		c2, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) (from %q): %v", s, name, err)
		}
		if c1.RGBA() != c2.RGBA() {
			t.Errorf("%s: round trip %+v -> %q -> %+v", name, c1.RGBA(), s, c2.RGBA())
		}
		// Hex is the other CSS spelling; it must round-trip too.
		hex := c1.RGBA().Hex()
		c3, err := Parse(hex)
		if err != nil {
			t.Fatalf("Parse(%q) (hex of %q): %v", hex, name, err)
		}
		want := c1.RGBA()
		got := c3.RGBA()
		if got.R != want.R || got.G != want.G || got.B != want.B {
			t.Errorf("%s: hex round trip %+v -> %q -> %+v", name, want, hex, got)
		}
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		in   string
		want RGB
	}{
		{"#000", RGB{0, 0, 0, 1}},
		{"#fff", RGB{255, 255, 255, 1}},
		{"#f00", RGB{255, 0, 0, 1}},
		{"#abc", RGB{0xaa, 0xbb, 0xcc, 1}},
		{"#ABC", RGB{0xaa, 0xbb, 0xcc, 1}},
		{"#4682b4", RGB{70, 130, 180, 1}},
		{"#4682B4", RGB{70, 130, 180, 1}},
		{"  #4682b4\t", RGB{70, 130, 180, 1}},
		// The 4- and 8-digit forms carry alpha.
		{"#f00f", RGB{255, 0, 0, 1}},
		{"#f000", RGB{255, 0, 0, 0}},
		{"#ff000080", RGB{255, 0, 0, 128.0 / 255.0}},
		{"#ff0000ff", RGB{255, 0, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			if got := c.RGBA(); got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseFunctional(t *testing.T) {
	tests := []struct {
		in   string
		want RGB // expected sRGB, compared with tolerance
	}{
		{"rgb(255, 0, 0)", RGB{255, 0, 0, 1}},
		{"rgb(255,0,0)", RGB{255, 0, 0, 1}},
		{"RGB( 255 , 0 , 0 )", RGB{255, 0, 0, 1}},
		{"rgb(100%, 0%, 0%)", RGB{255, 0, 0, 1}},
		{"rgb(0%, 100%, 0%)", RGB{0, 255, 0, 1}},
		{"rgba(255, 0, 0, 0.5)", RGB{255, 0, 0, 0.5}},
		{"rgba(255, 0, 0, 50%)", RGB{255, 0, 0, 0.5}},
		// CSS Color 4 space-separated syntax.
		{"rgb(255 0 0)", RGB{255, 0, 0, 1}},
		{"rgb(255 0 0 / 0.25)", RGB{255, 0, 0, 0.25}},
		{"rgb(255 0 0 / 25%)", RGB{255, 0, 0, 0.25}},
		// hsl: primaries are exactly representable.
		{"hsl(0, 100%, 50%)", RGB{255, 0, 0, 1}},
		{"hsl(120, 100%, 50%)", RGB{0, 255, 0, 1}},
		{"hsl(240, 100%, 50%)", RGB{0, 0, 255, 1}},
		{"hsl(0, 0%, 0%)", RGB{0, 0, 0, 1}},
		{"hsl(0, 0%, 100%)", RGB{255, 255, 255, 1}},
		{"hsl(0, 0%, 50%)", RGB{127.5, 127.5, 127.5, 1}},
		{"hsla(120, 100%, 50%, 0.5)", RGB{0, 255, 0, 0.5}},
		{"HSL(120deg, 100%, 50%)", RGB{0, 255, 0, 1}},
		// Hue wraps.
		{"hsl(360, 100%, 50%)", RGB{255, 0, 0, 1}},
		{"hsl(-120, 100%, 50%)", RGB{0, 0, 255, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.in, err)
			}
			got := c.RGBA()
			if !closeRGB(got, tt.want, 1e-9) {
				t.Errorf("Parse(%q).RGBA() = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseSpaceIsPreserved checks that hsl() input stays HSL, which matters
// because interpolating a parsed hsl() color should not have to undo an sRGB
// round trip first.
func TestParseSpaceIsPreserved(t *testing.T) {
	if c := MustParse("hsl(120, 100%, 50%)"); func() bool { _, ok := c.(HSL); return !ok }() {
		t.Errorf("Parse(hsl(...)) returned %T, want HSL", c)
	}
	if c := MustParse("rgb(1, 2, 3)"); func() bool { _, ok := c.(RGB); return !ok }() {
		t.Errorf("Parse(rgb(...)) returned %T, want RGB", c)
	}
	if c := MustParse("#123456"); func() bool { _, ok := c.(RGB); return !ok }() {
		t.Errorf("Parse(hex) returned %T, want RGB", c)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"notacolor",
		"#",
		"#12",
		"#12345",
		"#1234567",
		"#gggggg",
		"rgb(1, 2)",
		"rgb(1, 2, 3, 4, 5)",
		"rgb(a, b, c)",
		"hsl(1, 2)",
		"cmyk(0,0,0,0)",
		"rgb(1, 2, 3",
		"rgb 1 2 3",
	}
	for _, in := range bad {
		t.Run(in, func(t *testing.T) {
			if c, err := Parse(in); err == nil {
				t.Errorf("Parse(%q) = %v, want error", in, c)
			}
		})
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParse(\"nope\") did not panic")
		}
	}()
	MustParse("nope")
}

func TestRGBString(t *testing.T) {
	tests := []struct {
		in   RGB
		want string
	}{
		{RGB{70, 130, 180, 1}, "rgb(70, 130, 180)"},
		{RGB{70, 130, 180, 0.5}, "rgba(70, 130, 180, 0.5)"},
		// Fractional channels round to the nearest integer.
		{RGB{70.4, 130.5, 180.6, 1}, "rgb(70, 131, 181)"},
		// Out-of-gamut channels clamp at format time, not before.
		{RGB{-10, 300, 128, 1}, "rgb(0, 255, 128)"},
		{RGB{0, 0, 0, 2}, "rgb(0, 0, 0)"},
		{RGB{0, 0, 0, -1}, "rgba(0, 0, 0, 0)"},
		// NaN never escapes into CSS.
		{RGB{math.NaN(), 0, 0, 1}, "rgb(0, 0, 0)"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("RGB%+v.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRGBHex(t *testing.T) {
	if got := NewRGB(70, 130, 180).Hex(); got != "#4682b4" {
		t.Errorf("Hex() = %q, want %#v", got, "#4682b4")
	}
	if got := NewRGBA(255, 0, 0, 0.5).HexA(); got != "#ff000080" {
		t.Errorf("HexA() = %q, want %q", got, "#ff000080")
	}
	if got := NewRGB(-10, 300, 128).Hex(); got != "#00ff80" {
		t.Errorf("Hex() clamping = %q, want %q", got, "#00ff80")
	}
}

func TestRGBDisplayableAndClamp(t *testing.T) {
	if !NewRGB(0, 128, 255).Displayable() {
		t.Error("in-gamut color reported not displayable")
	}
	if NewRGB(-1, 0, 0).Displayable() {
		t.Error("negative channel reported displayable")
	}
	if NewRGB(0, 0, 256).Displayable() {
		t.Error("channel over 255 reported displayable")
	}
	if (RGB{0, 0, 0, 1.5}).Displayable() {
		t.Error("opacity over 1 reported displayable")
	}
	if got, want := (RGB{-5, 300, 128, 2}).Clamp(), (RGB{0, 255, 128, 1}); got != want {
		t.Errorf("Clamp() = %+v, want %+v", got, want)
	}
}

func TestRGBNRGBA(t *testing.T) {
	r, g, b, a := NewRGBA(70, 130, 180, 0.5).NRGBA()
	if r != 70 || g != 130 || b != 180 || a != 128 {
		t.Errorf("NRGBA() = %d %d %d %d, want 70 130 180 128", r, g, b, a)
	}
}

// TestBrighterDarkerInverse relies on channels being unclamped: because the
// operations are pure multiplications and nothing is rounded, applying one and
// then the other must recover the input exactly (up to float error).
func TestBrighterDarkerInverse(t *testing.T) {
	c := NewRGB(70, 130, 180)
	for _, k := range []float64{0, 0.5, 1, 2, 3.7, -1} {
		got := c.Brighter(k).Darker(k)
		if !closeRGB(got, c, 1e-9) {
			t.Errorf("Brighter(%v).Darker(%v) = %+v, want %+v", k, k, got, c)
		}
		got = c.Darker(k).Brighter(k)
		if !closeRGB(got, c, 1e-9) {
			t.Errorf("Darker(%v).Brighter(%v) = %+v, want %+v", k, k, got, c)
		}
	}
	// Brighter(-k) == Darker(k).
	if !closeRGB(c.Brighter(-2), c.Darker(2), 1e-9) {
		t.Error("Brighter(-2) != Darker(2)")
	}
	// k = 0 is the identity.
	if c.Brighter(0) != c || c.Darker(0) != c {
		t.Error("k = 0 is not the identity")
	}
}

// TestBrighterMonotone checks the direction of the operations rather than
// specific values.
func TestBrighterMonotone(t *testing.T) {
	c := NewRGB(70, 130, 180)
	prev := c
	for k := 0.5; k <= 3; k += 0.5 {
		got := c.Brighter(k)
		if got.R <= prev.R || got.G <= prev.G || got.B <= prev.B {
			t.Fatalf("Brighter(%v) = %+v is not brighter than %+v", k, got, prev)
		}
		prev = got
	}
	// Multiplication cannot brighten black.
	if got := NewRGB(0, 0, 0).Brighter(5); got != NewRGB(0, 0, 0) {
		t.Errorf("Brighter on black = %+v, want black (multiplicative)", got)
	}
	// Opacity is untouched.
	if got := NewRGBA(1, 2, 3, 0.4).Brighter(1); got.Opacity != 0.4 {
		t.Errorf("Brighter changed opacity to %v", got.Opacity)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	// Every named color must survive RGB -> HSL -> RGB.
	for _, name := range NamedColors() {
		c := MustParse(name).RGBA()
		got := ToHSL(c).RGBA()
		if !closeRGB(got, c, 1e-9) {
			t.Errorf("%s: RGB->HSL->RGB = %+v, want %+v", name, got, c)
		}
	}
}

func TestToHSLAchromaticHueIsNaN(t *testing.T) {
	for _, name := range []string{"black", "white", "gray", "silver"} {
		h := ToHSL(MustParse(name))
		if !math.IsNaN(h.H) {
			t.Errorf("%s: hue = %v, want NaN (achromatic)", name, h.H)
		}
		if h.S != 0 {
			t.Errorf("%s: saturation = %v, want 0", name, h.S)
		}
	}
	// A NaN hue must not escape into output.
	if got := (HSL{H: math.NaN(), S: 0, L: 0.5, Opacity: 1}).String(); !strings.HasPrefix(got, "hsl(0,") {
		t.Errorf("NaN hue formatted as %q, want a hue of 0", got)
	}
	if got := (HSL{H: math.NaN(), S: 0, L: 0.5, Opacity: 1}).RGBA(); !closeRGB(got, RGB{127.5, 127.5, 127.5, 1}, 1e-9) {
		t.Errorf("NaN hue converted to %+v, want mid gray", got)
	}
}

func TestHSLString(t *testing.T) {
	tests := []struct {
		in   HSL
		want string
	}{
		{NewHSL(120, 1, 0.5), "hsl(120, 100%, 50%)"},
		{NewHSLA(120, 1, 0.5, 0.25), "hsla(120, 100%, 50%, 0.25)"},
		// Hue wraps into [0, 360); S and L clamp.
		{NewHSL(480, 1, 0.5), "hsl(120, 100%, 50%)"},
		{NewHSL(-120, 1, 0.5), "hsl(240, 100%, 50%)"},
		{NewHSL(0, 2, -1), "hsl(0, 100%, 0%)"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("HSL%+v.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestHSLUnclampedHue documents that hue is stored as given; only conversion
// and formatting wrap it. Interpolators depend on this.
func TestHSLUnclampedHue(t *testing.T) {
	c := HSL{H: 400, S: 1, L: 0.5, Opacity: 1}
	if c.H != 400 {
		t.Error("HSL stored a wrapped hue")
	}
	if !closeRGB(c.RGBA(), NewHSL(40, 1, 0.5).RGBA(), 1e-9) {
		t.Error("hue 400 did not convert as hue 40")
	}
}

func TestHSLBrighterDarker(t *testing.T) {
	c := NewHSL(120, 0.5, 0.4)
	if got := c.Brighter(1).Darker(1); math.Abs(got.L-c.L) > 1e-12 {
		t.Errorf("HSL Brighter/Darker not inverse: %v vs %v", got.L, c.L)
	}
	if c.Brighter(1).L <= c.L {
		t.Error("HSL.Brighter did not increase lightness")
	}
	if c.Darker(1).L >= c.L {
		t.Error("HSL.Darker did not decrease lightness")
	}
}

func TestToRGBNil(t *testing.T) {
	if got := ToRGB(nil); got != (RGB{}) {
		t.Errorf("ToRGB(nil) = %+v, want zero RGB", got)
	}
	if got := Format(nil); got != "none" {
		t.Errorf("Format(nil) = %q, want \"none\"", got)
	}
}

// closeRGB reports whether two colors agree channel-by-channel within tol.
func closeRGB(a, b RGB, tol float64) bool {
	return math.Abs(a.R-b.R) <= tol && math.Abs(a.G-b.G) <= tol &&
		math.Abs(a.B-b.B) <= tol && math.Abs(a.Opacity-b.Opacity) <= tol
}

// TestParsePermissiveForms covers the corners of the functional-notation
// grammar: uppercase hex, mixed units, alpha as a percentage, and the argument
// errors each branch can raise.
func TestParsePermissiveForms(t *testing.T) {
	ok := []struct {
		in   string
		want RGB
	}{
		{"#ABCDEF", RGB{0xab, 0xcd, 0xef, 1}},
		{"rgb(50%, 128, 0)", RGB{127.5, 128, 0, 1}},
		{"rgba(0, 0, 0, 100%)", RGB{0, 0, 0, 1}},
		{"hsla(0, 0%, 0%, 25%)", RGB{0, 0, 0, 0.25}},
		// A bare fraction is accepted for S and L, since that is how the
		// values are written in Go code.
		{"hsl(120, 1, 0.5)", RGB{0, 255, 0, 1}},
	}
	for _, tt := range ok {
		c, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got := c.RGBA(); !closeRGB(got, tt.want, 1e-9) {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
	bad := []string{
		"rgb(1, 2, x)",
		"rgb(1, 2, 3, x)",
		"hsl(x, 2%, 3%)",
		"hsl(1, x, 3%)",
		"hsl(1, 2%, x)",
		"hsl(1, 2%, 3%, x)",
		"rgb(1,, 3)",
		"rgb(,)",
	}
	for _, in := range bad {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", in)
		}
	}
}
