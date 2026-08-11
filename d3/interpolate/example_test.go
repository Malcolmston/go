package interpolate_test

import (
	"fmt"

	"github.com/malcolmston/d3/color"
	"github.com/malcolmston/d3/interpolate"
)

func ExampleNumber() {
	f := interpolate.Number(10, 20)
	fmt.Println(f(0), f(0.5), f(1))
	// t is not clamped, so out-of-range values extrapolate.
	fmt.Println(f(-0.5), f(1.5))
	// Output:
	// 10 15 20
	// 5 25
}

func ExampleString() {
	f := interpolate.String("translate(0,0)", "translate(10,20)")
	fmt.Println(f(0))
	fmt.Println(f(0.5))
	fmt.Println(f(1))

	// Non-numeric text always comes from b.
	fmt.Println(interpolate.String("1px solid red", "5px solid blue")(0.5))
	// Output:
	// translate(0,0)
	// translate(5,10)
	// translate(10,20)
	// 3px solid blue
}

func ExampleLab() {
	// Lab keeps the midpoint perceptually halfway; sRGB does not.
	white := color.MustParse("white")
	blue := color.MustParse("blue")
	fmt.Println(interpolate.Lab(white, blue)(0.5).RGBA().Hex())
	fmt.Println(interpolate.RGB(white, blue)(0.5).RGBA().Hex())
	// Output:
	// #af89ff
	// #8080ff
}

func ExamplePiecewise() {
	f := interpolate.Piecewise(interpolate.Lab, []color.Color{
		color.MustParse("#440154"),
		color.MustParse("#21918c"),
		color.MustParse("#fde725"),
	})
	for _, t := range []float64{0, 0.5, 1} {
		fmt.Println(f(t).RGBA().Hex())
	}
	// Output:
	// #440154
	// #21918c
	// #fde725
}

func ExampleValue() {
	fmt.Println(interpolate.Value(0, 10)(0.5))
	fmt.Println(interpolate.Value("a0", "a10")(0.5))
	// Values that cannot be blended snap to b.
	fmt.Println(interpolate.Value(true, false)(0.5))
	// Output:
	// 5
	// a5
	// false
}
