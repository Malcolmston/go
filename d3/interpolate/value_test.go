package interpolate

import (
	"math"
	"testing"

	"github.com/malcolmston/d3/color"
)

func TestValueNumbers(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		t    float64
		want float64
	}{
		{"float64", 0.0, 10.0, 0.5, 5},
		{"int", 0, 10, 0.5, 5},
		{"int64", int64(0), int64(100), 0.25, 25},
		{"float32", float32(0), float32(1), 0.5, 0.5},
		{"uint8", uint8(0), uint8(200), 0.5, 100},
		{"mixed kinds", 0, 10.0, 0.5, 5},
		{"mixed the other way", 0.0, 10, 0.5, 5},
		// Endpoints and extrapolation carry through from Number.
		{"start", 3.0, 9.0, 0, 3},
		{"end", 3.0, 9.0, 1, 9},
		{"extrapolate", 0.0, 10.0, 2, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Value(tt.a, tt.b)(tt.t).(float64)
			if !ok {
				t.Fatalf("Value(%v, %v) did not return a float64", tt.a, tt.b)
			}
			if got != tt.want {
				t.Errorf("Value(%v, %v)(%v) = %v, want %v", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

func TestValueStrings(t *testing.T) {
	got := Value("translate(0,0)", "translate(10,20)")(0.5)
	if got != "translate(5,10)" {
		t.Errorf("Value on strings = %v, want translate(5,10)", got)
	}
	// A string with no numbers is a constant b.
	if got := Value("red", "blue")(0.5); got != "blue" {
		t.Errorf("Value(red, blue) = %v, want blue", got)
	}
	if got := Value("red", "blue")(0); got != "blue" {
		t.Errorf("Value(red, blue)(0) = %v, want blue", got)
	}
}

func TestValueColors(t *testing.T) {
	a := color.MustParse("red")
	b := color.MustParse("blue")
	got, ok := Value(a, b)(0.5).(color.Color)
	if !ok {
		t.Fatalf("Value on colors returned %T, want color.Color", got)
	}
	want := color.RGB{R: 127.5, G: 0, B: 127.5, Opacity: 1}
	if !closeRGB(got.RGBA(), want, 1e-9) {
		t.Errorf("Value(red, blue)(0.5) = %+v, want %+v (sRGB by default)", got.RGBA(), want)
	}
	// Endpoints.
	if v := Value(a, b)(0).(color.Color).RGBA(); !closeRGB(v, a.RGBA(), 1e-9) {
		t.Errorf("Value(a, b)(0) = %+v, want %+v", v, a.RGBA())
	}
	if v := Value(a, b)(1).(color.Color).RGBA(); !closeRGB(v, b.RGBA(), 1e-9) {
		t.Errorf("Value(a, b)(1) = %+v, want %+v", v, b.RGBA())
	}
	// A color in a different space still dispatches to the color path.
	if _, ok := Value(color.ToLab(a), color.ToHSL(b))(0.5).(color.Color); !ok {
		t.Error("Value on Lab/HSL did not dispatch to the color path")
	}
}

func TestValueUninterpolatable(t *testing.T) {
	tests := []struct {
		name string
		a, b any
	}{
		{"bools", true, false},
		{"nil b", 1.0, nil},
		{"structs", struct{ X int }{1}, struct{ X int }{2}},
		{"mismatched", "abc", 5.0},
		{"number to string", 5.0, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Value(tt.a, tt.b)
			for _, x := range []float64{0, 0.5, 1} {
				if got := f(x); got != tt.b {
					t.Errorf("Value(%v, %v)(%v) = %v, want the constant %v", tt.a, tt.b, x, got, tt.b)
				}
			}
		})
	}
}

// TestValueNumberToStringIsConstant documents the mismatch rule: a string b
// with a non-string a cannot be interpolated, so it snaps to b.
func TestValueNumberToStringIsConstant(t *testing.T) {
	if got := Value(1.0, "5px")(0.5); got != "5px" {
		t.Errorf("Value(1.0, \"5px\") = %v, want the constant \"5px\"", got)
	}
}

func TestToFloat(t *testing.T) {
	vals := []any{
		float64(1), float32(1), int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1),
	}
	for _, v := range vals {
		got, ok := toFloat(v)
		if !ok || got != 1 {
			t.Errorf("toFloat(%T %v) = %v, %v; want 1, true", v, v, got, ok)
		}
	}
	for _, v := range []any{"1", true, nil, []int{1}} {
		if _, ok := toFloat(v); ok {
			t.Errorf("toFloat(%T) reported numeric", v)
		}
	}
}

func TestValueNaN(t *testing.T) {
	got := Value(math.NaN(), 1.0)(0.5).(float64)
	if !math.IsNaN(got) {
		t.Errorf("Value with a NaN endpoint = %v, want NaN to propagate", got)
	}
}
