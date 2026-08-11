# color — Go port of d3-color — parsing

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/color.svg)](https://pkg.go.dev/github.com/malcolmston/d3/color)

Package color is a Go port of d3-color — parsing, representing and
converting between the color spaces that data visualization actually needs. It
is the computational half of the problem: there is no DOM here and nothing is
rendered, so a color is just a value you can read channels off of, convert,
and format back to a CSS string for whatever does the drawing.

Why more than one color space? Because sRGB is a terrible space to compute in.
Averaging two sRGB colors, or walking a hue ramp in sRGB, produces muddy
midpoints and bands of uneven apparent brightness, because the sRGB channels
are not perceptually uniform — a step of 10 in the green channel is a much
bigger visual change than a step of 10 in blue. `Lab` and `HCL` are designed
so that equal numeric distances look like equal perceptual distances, which is
exactly what you want when interpolating a color scale or generating a
categorical palette. `HSL` is not perceptual at all, but it is the space most
people reason about ("same color, lighter"), and CSS speaks it natively.
`Cubehelix` trades perceptual accuracy for a guarantee sequential ramps care
about: monotonically increasing luminance along the ramp, so the scale still
reads correctly in grayscale.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/color"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Usage

This is the package's own `ExampleParse`, so it compiles and its output is
asserted on every `go test ./color/`.

```go
for _, s := range []string{"steelblue", "#4682b4", "rgb(70, 130, 180)", "hsl(207.3, 44%, 49%)"} {
		c, err := color.Parse(s)
		if err != nil {
			panic(err)
		}
		fmt.Println(c.RGBA().Hex())
	}
```

```
#4682b4
#4682b4
#4682b4
#4682b4
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func Format(c Color) string` | Format renders any `Color` as CSS. |
| `func NamedColors() []string` | NamedColors returns the sorted list of CSS color keywords `Parse` accepts. |

### Types

| Type | What it is |
| --- | --- |
| `Color` | Color is the behavior every color space in this package shares: it can project itself into sRGB and it can render itself as a CSS string. |
| `Cubehelix` | Cubehelix is a color in Dave Green's cubehelix space: hue in degrees, saturation, and lightness in 0–1. |
| `HCL` | HCL is the cylindrical form of `Lab`: hue in degrees, chroma, and the same lightness L in 0–100. |
| `HSL` | HSL is a color in the CSS HSL space: hue in degrees, saturation and lightness in 0–1, opacity in 0–1. |
| `Lab` | Lab is a color in the CIELAB space with a D65 white point. |
| `RGB` | RGB is a color in the sRGB space: red, green and blue channels nominally in 0–255 and an opacity (alpha) nominally in 0–1. |

<details>
<summary><code>Color</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func MustParse(s string) Color` | MustParse is `Parse` but panics instead of returning an error. |
| `func Parse(s string) (Color, error)` | Parse parses a CSS color string. |

</details>

<details>
<summary><code>Cubehelix</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewCubehelix(h, s, l float64) Cubehelix` | NewCubehelix returns an opaque Cubehelix color. |
| `func NewCubehelixA(h, s, l, opacity float64) Cubehelix` | NewCubehelixA returns a Cubehelix color with the given opacity. |
| `func ToCubehelix(col Color) Cubehelix` | ToCubehelix converts any `Color` to `Cubehelix`, returning it unchanged when it already is one. |
| `func (c Cubehelix) Brighter(k float64) Cubehelix` | Brighter returns the color with L scaled by (1/0.7)^k; see `RGB.Brighter`. |
| `func (c Cubehelix) Darker(k float64) Cubehelix` | Darker returns the color with L scaled by 0.7^k. |
| `func (c Cubehelix) RGBA() RGB` | RGBA converts to sRGB. |
| `func (c Cubehelix) String() string` | String formats the color as sRGB; see `Lab.String`. |

</details>

<details>
<summary><code>HCL</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewHCL(h, c, l float64) HCL` | NewHCL returns an opaque HCL color. |
| `func NewHCLA(h, c, l, opacity float64) HCL` | NewHCLA returns an HCL color with the given opacity. |
| `func ToHCL(c Color) HCL` | ToHCL converts any `Color` to `HCL`. |
| `func (c HCL) Brighter(k float64) HCL` | Brighter returns the color with L increased by 18*k; see `Lab.Brighter`. |
| `func (c HCL) Darker(k float64) HCL` | Darker returns the color with L decreased by 18*k. |
| `func (c HCL) RGBA() RGB` | RGBA converts to sRGB by way of `Lab`. |
| `func (c HCL) String() string` | String formats the color as sRGB; see `Lab.String`. |
| `func (c HCL) ToLab() Lab` | ToLab converts an HCL color to `Lab`. |

</details>

<details>
<summary><code>HSL</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewHSL(h, s, l float64) HSL` | NewHSL returns an opaque HSL. |
| `func NewHSLA(h, s, l, opacity float64) HSL` | NewHSLA returns an HSL with the given channels and opacity. |
| `func ToHSL(c Color) HSL` | ToHSL converts any `Color` to `HSL`, returning it unchanged when it is already an HSL (which avoids a needless lossy round trip through sRGB). |
| `func (c HSL) Brighter(k float64) HSL` | Brighter returns the color with lightness scaled by (1/0.7)^k. |
| `func (c HSL) Darker(k float64) HSL` | Darker returns the color with lightness scaled by 0.7^k, the inverse of `HSL.Brighter`. |
| `func (c HSL) RGBA() RGB` | RGBA converts to sRGB using the standard CSS HSL algorithm. |
| `func (c HSL) String() string` | String formats the color as CSS: "hsl(h, s%, l%)" when opaque, otherwise "hsla(h, s%, l%, a)". |

</details>

<details>
<summary><code>Lab</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLab(l, a, b float64) Lab` | NewLab returns an opaque Lab color. |
| `func NewLabA(l, a, b, opacity float64) Lab` | NewLabA returns a Lab color with the given opacity. |
| `func ToLab(c Color) Lab` | ToLab converts any `Color` to `Lab`, short-circuiting when it is already a `Lab` or an `HCL` so that no needless sRGB round trip is taken. |
| `func (c Lab) Brighter(k float64) Lab` | Brighter returns the color with L increased by 18*k, the step d3 uses. |
| `func (c Lab) Darker(k float64) Lab` | Darker returns the color with L decreased by 18*k, the inverse of `Lab.Brighter`. |
| `func (c Lab) RGBA() RGB` | RGBA converts to sRGB. |
| `func (c Lab) String() string` | String formats the color by converting to sRGB first, because CSS has no widely supported Lab notation this package commits to. |

</details>

<details>
<summary><code>RGB</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewRGB(r, g, b float64) RGB` | NewRGB returns an opaque RGB with the given channels. |
| `func NewRGBA(r, g, b, opacity float64) RGB` | NewRGBA returns an RGB with the given channels and opacity. |
| `func ToRGB(c Color) RGB` | ToRGB converts any `Color` to sRGB. |
| `func (c RGB) Brighter(k float64) RGB` | Brighter returns the color with each channel scaled by (1/0.7)^k, leaving opacity untouched. |
| `func (c RGB) Clamp() RGB` | Clamp returns the color with every channel forced into the sRGB gamut, without rounding to integers. |
| `func (c RGB) Darker(k float64) RGB` | Darker returns the color with each channel scaled by 0.7^k, leaving opacity untouched. |
| `func (c RGB) Displayable() bool` | Displayable reports whether every channel is already inside the sRGB gamut (0–255, opacity 0–1), meaning `RGB.String` would not have to clamp… |
| `func (c RGB) Hex() string` | Hex formats the color as a lowercase "#rrggbb" string, clamping and rounding as `RGB.String` does. |
| `func (c RGB) HexA() string` | HexA formats the color as a lowercase "#rrggbbaa" string, encoding opacity in the fourth byte. |
| `func (c RGB) NRGBA() (r, g, b, a uint8)` | NRGBA converts to the standard library's non-alpha-premultiplied 8-bit color type, clamping and rounding as `RGB.String` does. |
| `func (c RGB) RGBA() RGB` | RGBA returns c unchanged, satisfying `Color`. |
| `func (c RGB) String() string` | String formats the color as CSS: "rgb(r, g, b)" when fully opaque, otherwise "rgba(r, g, b, a)". |

</details>

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/color).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
