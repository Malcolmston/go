package react

import (
	"math"
	"testing"
)

// Every expectation below was read off real react-dom 19.2.7 (String(n) as both
// a text child and an attribute value) rather than from memory.
func TestJSNumberStringFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want string
	}{
		{"zero", 0, "0"},
		{"negative zero", math.Copysign(0, -1), "0"},
		{"small integer", 5, "5"},
		{"negative integer", -5, "-5"},
		{"simple decimal", 1.5, "1.5"},
		{"tenth", 0.1, "0.1"},
		{"one third", 1.0 / 3.0, "0.3333333333333333"},
		{"largest fixed integer form", 1e20, "100000000000000000000"},
		{"exponent threshold", 1e21, "1e+21"},
		{"exponent with fraction", 1.5e21, "1.5e+21"},
		{"large exponent", 1e100, "1e+100"},
		{"negative exponent form", -1e21, "-1e+21"},
		{"seventeen digit large", 1.2345678901234569e23, "1.2345678901234569e+23"},
		{"rounded integer under threshold", 123456789012345680000, "123456789012345680000"},
		{"smallest fixed fraction", 1e-6, "0.000001"},
		{"first exponential fraction", 1e-7, "1e-7"},
		{"small negative exponent", 1e-10, "1e-10"},
		{"negative small fraction", -1e-7, "-1e-7"},
		{"subnormal", 5e-324, "5e-324"},
		{"max float64", math.MaxFloat64, "1.7976931348623157e+308"},
		{"nan", math.NaN(), "NaN"},
		{"positive infinity", math.Inf(1), "Infinity"},
		{"negative infinity", math.Inf(-1), "-Infinity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jsNumberString(tt.in, 64); got != tt.want {
				t.Fatalf("jsNumberString(%v, 64) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A float32 keeps the shortest digits that round-trip at 32-bit precision, so
// the widening to float64 does not leak binary noise into the markup.
func TestJSNumberStringFloat32(t *testing.T) {
	tests := []struct {
		in   float32
		want string
	}{
		{0.1, "0.1"},
		{1.5, "1.5"},
		{-2.25, "-2.25"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{1e-7, "1e-7"},
		{1e-6, "0.000001"},
	}
	for _, tt := range tests {
		if got := jsNumberString(float64(tt.in), 32); got != tt.want {
			t.Fatalf("jsNumberString(float64(%v), 32) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// jsNumberString reaches markup through textOf, which is what numeric children,
// numeric attribute values and numeric style values all share.
func TestTextOfUsesJSNumberFormatting(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{1e21, "1e+21"},
		{1e20, "100000000000000000000"},
		{1e-7, "1e-7"},
		{float32(1e21), "1e+21"},
		{math.Copysign(0, -1), "0"},
		{7, "7"},
	}
	for _, tt := range tests {
		if got := textOf(tt.in); got != tt.want {
			t.Fatalf("textOf(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderNumericChildAndAttributeUseExponentForm(t *testing.T) {
	got, err := RenderToStaticMarkup(CreateElement("div", nil, 1e21))
	if err != nil {
		t.Fatalf("render numeric child: %v", err)
	}
	if want := "<div>1e+21</div>"; got != want {
		t.Fatalf("numeric child = %q, want %q", got, want)
	}
	got, err = RenderToStaticMarkup(CreateElement("div", Props{"data-n": 1e21}))
	if err != nil {
		t.Fatalf("render numeric attribute: %v", err)
	}
	if want := `<div data-n="1e+21"></div>`; got != want {
		t.Fatalf("numeric attribute = %q, want %q", got, want)
	}
}
