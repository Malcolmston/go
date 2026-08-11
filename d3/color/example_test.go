package color_test

import (
	"fmt"

	"github.com/malcolmston/d3/color"
)

func ExampleParse() {
	for _, s := range []string{"steelblue", "#4682b4", "rgb(70, 130, 180)", "hsl(207.3, 44%, 49%)"} {
		c, err := color.Parse(s)
		if err != nil {
			panic(err)
		}
		fmt.Println(c.RGBA().Hex())
	}
	// Output:
	// #4682b4
	// #4682b4
	// #4682b4
	// #4682b4
}

func ExampleRGB_String() {
	fmt.Println(color.NewRGB(70, 130, 180))
	fmt.Println(color.NewRGBA(70, 130, 180, 0.5))
	// Out-of-gamut channels clamp only at format time.
	fmt.Println(color.NewRGB(-20, 300, 180))
	// Output:
	// rgb(70, 130, 180)
	// rgba(70, 130, 180, 0.5)
	// rgb(0, 255, 180)
}

func ExampleRGB_Brighter() {
	c := color.MustParse("steelblue").RGBA()
	fmt.Println(c.Brighter(1))
	fmt.Println(c.Darker(1))
	// Output:
	// rgb(100, 186, 255)
	// rgb(49, 91, 126)
}

func ExampleToLab() {
	c := color.MustParse("steelblue")
	lab := color.ToLab(c)
	fmt.Printf("L=%.1f A=%.1f B=%.1f\n", lab.L, lab.A, lab.B)

	// Lab lightness steps are additive and perceptually uniform, so unlike the
	// multiplicative sRGB version they can brighten black.
	fmt.Println(color.NewLab(0, 0, 0).Brighter(1).L)
	// Output:
	// L=52.0 A=-8.4 B=-32.8
	// 18
}

func ExampleRGB_Displayable() {
	// A high-chroma HCL color has no sRGB equivalent.
	c := color.NewHCL(120, 150, 50).RGBA()
	fmt.Println(c.Displayable())
	fmt.Println(c.String()) // clamped to the nearest displayable color
	// Output:
	// false
	// rgb(0, 145, 0)
}
