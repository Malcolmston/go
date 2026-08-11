# interpolate — Go port of d3-interpolate — functions that blend between two values as a parameter t

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/interpolate.svg)](https://pkg.go.dev/github.com/malcolmston/d3/interpolate)

Package interpolate is a Go port of d3-interpolate — functions that blend
between two values as a parameter t sweeps from 0 to 1.

An interpolator is a closure: you give it the two endpoints once, and it gives
you back a function of t.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/interpolate"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Usage

This is the package's own `ExampleLab`, so it compiles and its output is
asserted on every `go test ./interpolate/`.

```go
white := color.MustParse("white")
	blue := color.MustParse("blue")
	fmt.Println(interpolate.Lab(white, blue)(0.5).RGBA().Hex())
	fmt.Println(interpolate.RGB(white, blue)(0.5).RGBA().Hex())
```

```
#af89ff
#8080ff
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Array[T any](interp func(a, b T) func(float64) T, a, b []T) func(t float64) []T` | Array returns an interpolator between two slices of arbitrary values, using interp for each element pair. |
| `func Basis(values []float64) func(t float64) float64` | Basis returns a uniform cubic B-spline through the given values, evaluated over t in [0, 1]. |
| `func BasisClosed(values []float64) func(t float64) float64` | BasisClosed returns a closed uniform cubic B-spline through the given values. |
| `func Cubehelix(a, b color.Color) func(t float64) color.Color` | Cubehelix returns an interpolator between two colors in cubehelix space, taking the shortest path around the hue circle. |
| `func CubehelixLong(a, b color.Color) func(t float64) color.Color` | CubehelixLong returns a cubehelix interpolator that takes the long way around the hue circle. |
| `func HCL(a, b color.Color) func(t float64) color.Color` | HCL returns an interpolator between two colors in CIE LCh, taking the shortest path around the hue circle. |
| `func HCLLong(a, b color.Color) func(t float64) color.Color` | HCLLong returns an interpolator in CIE LCh that takes the long way around the hue circle, the perceptual analogue of `HSLLong`. |
| `func HSL(a, b color.Color) func(t float64) color.Color` | HSL returns an interpolator between two colors in the CSS HSL space, taking the shortest path around the hue circle. |
| `func HSLLong(a, b color.Color) func(t float64) color.Color` | HSLLong returns an interpolator between two colors in HSL that takes the long way around the hue circle rather than the short one. |
| `func Lab(a, b color.Color) func(t float64) color.Color` | Lab returns an interpolator between two colors in CIELAB. |
| `func Number(a, b float64) func(t float64) float64` | Number returns an interpolator between two numbers. |
| `func Numbers(a, b []float64) func(t float64) []float64` | Numbers returns an interpolator between two slices of numbers, blending them element-wise. |
| `func Object[K comparable, V any](interp func(a, b V) func(float64) V, a, b map[K]V) func(t float64) map[K]V` | Object returns an interpolator between two maps, using interp for each key present in both. |
| `func Piecewise[T any](interp func(a, b T) func(float64) T, values []T) func(t float64) T` | Piecewise chains a sequence of interpolators into one covering [0, 1]. |
| `func RGB(a, b color.Color) func(t float64) color.Color` | RGB returns an interpolator between two colors in the sRGB space. |
| `func Round(a, b float64) func(t float64) float64` | Round returns an interpolator between two numbers that rounds its result to the nearest integer. |
| `func String(a, b string) func(t float64) string` | String returns an interpolator between two strings that blends the numbers embedded in them while keeping b's non-numeric text. |
| `func Value(a, b any) func(t float64) any` | Value returns an interpolator between two arbitrary values, choosing the strategy from b's dynamic type. |
| `func Zoom(a, b ZoomView) func(t float64) ZoomView` | Zoom returns an interpolator between two viewports that follows the smooth zoom path of Jarke van Wijk and Wim Nuij, "Smooth and efficient zooming… |
| `func ZoomDuration(a, b ZoomView) float64` | ZoomDuration returns the recommended transition duration in milliseconds for the path `Zoom` takes between two viewports. |

### Types

| Type | What it is |
| --- | --- |
| `ZoomView` | ZoomView is a viewport for `Zoom`: the x and y coordinates of its center and its width, all in the same units. |

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/interpolate).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
