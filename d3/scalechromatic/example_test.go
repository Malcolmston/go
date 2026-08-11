package scalechromatic_test

import (
	"fmt"
	"strings"

	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/scale"
	"github.com/malcolmston/d3/scalechromatic"
)

func hex(c color.Color) string { return color.ToRGB(c).Hex() }

// A categorical scheme is a range for an ordinal scale: unordered data in,
// visually distinct colors out.
func ExampleSchemeCategory10() {
	c := scale.NewOrdinal[string, color.Color](scalechromatic.SchemeCategory10...).
		SetDomain("apples", "pears", "plums")

	for _, fruit := range []string{"apples", "pears", "plums"} {
		fmt.Printf("%s %s\n", fruit, hex(c.Scale(fruit)))
	}
	// Output:
	// apples #1f77b4
	// pears #ff7f0e
	// plums #2ca02c
}

// An interpolator is a range for a sequential scale. The scale normalizes the
// domain to [0, 1] and hands that straight to the ramp, which is exactly the
// interface these functions have.
func ExampleInterpolateViridis() {
	heat := scale.NewSequential(scalechromatic.InterpolateViridis).SetDomain(0, 100)

	for _, v := range []float64{0, 50, 100} {
		fmt.Printf("%v %s\n", v, hex(heat.Scale(v)))
	}
	// Output:
	// 0 #440154
	// 50 #21918c
	// 100 #fde725
}

// A diverging scale takes three domain values, so the scheme's pale midpoint
// lands on the value that means "neither" rather than halfway along the axis.
//
// Note that the midpoint here is #f2efee rather than SchemeRdBu[11][5]'s
// #f7f7f7. That is the B-spline doing what a B-spline does: it passes through
// its end colors exactly and is only pulled toward the interior ones. If a
// legend has to show the scheme's own colors, use the discrete form.
func ExampleInterpolateRdBu() {
	swing := scale.NewDiverging(scalechromatic.InterpolateRdBu).SetDomain(-20, 0, 20)

	for _, v := range []float64{-20, -5, 0, 5, 20} {
		fmt.Printf("%v %s\n", v, hex(swing.Scale(v)))
	}
	// Output:
	// -20 #67001f
	// -5 #faccb4
	// 0 #f2efee
	// 5 #bfdceb
	// 20 #053061
}

// Discrete and continuous are the same scheme twice. Use the k-indexed form when
// the data is classed, because ColorBrewer chose those five colors as a set of
// five rather than as five samples of a ramp.
func ExampleFamily() {
	var out []string
	for _, c := range scalechromatic.SchemeBlues[5] {
		out = append(out, hex(c))
	}
	fmt.Println(strings.Join(out, " "))
	fmt.Println(scalechromatic.SchemeBlues.Min(), scalechromatic.SchemeBlues.Max())
	fmt.Println(scalechromatic.SchemeBlues.K(2) == nil)
	// Output:
	// #eff3ff #bdd7e7 #6baed6 #3182bd #08519c
	// 3 9
	// true
}

// The registries exist so that a scheme name can come from configuration
// instead of from a switch statement. Lookup is case-insensitive.
func ExampleInterpolator() {
	ramp, ok := scalechromatic.Interpolator("spectral")
	if !ok {
		ramp = scalechromatic.InterpolateViridis
	}
	fmt.Println(ok, hex(ramp(0)), hex(ramp(1)))

	_, ok = scalechromatic.Interpolator("chartreuse")
	fmt.Println(ok)
	// Output:
	// true #9e0142 #5e4fa2
	// false
}
