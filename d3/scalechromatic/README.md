# scalechromatic — Go port of d3-scale-chromatic: the color schemes and the continuous color ramps

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/scalechromatic.svg)](https://pkg.go.dev/github.com/malcolmston/d3/scalechromatic)

Package scalechromatic is a Go port of d3-scale-chromatic: the color schemes
and the continuous color ramps that d3's sequential and diverging scales are
almost always given as their range.

The package is two things that happen to ship together. One is data — the
ColorBrewer tables, chosen by Cynthia Brewer from cartographic research into
which color sequences people actually read correctly, plus the matplotlib
ramps. The other is a handful of formulas — Cubehelix, Turbo, Cividis,
Rainbow, Sinebow — that compute a color from t directly. The distinction
matters more here than anywhere else in this port, and it is why the two
halves are documented and tested differently: a formula can be checked, and a
table can only be transcribed carefully. See "Provenance" below.

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/scalechromatic"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Usage

This is the package's own `ExampleFamily`, so it compiles and its output is
asserted on every `go test ./scalechromatic/`.

```go
var out []string
	for _, c := range scalechromatic.SchemeBlues[5] {
		out = append(out, hex(c))
	}
	fmt.Println(strings.Join(out, " "))
	fmt.Println(scalechromatic.SchemeBlues.Min(), scalechromatic.SchemeBlues.Max())
	fmt.Println(scalechromatic.SchemeBlues.K(2) == nil)
```

```
#eff3ff #bdd7e7 #6baed6 #3182bd #08519c
3 9
true
```

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func InterpolateBlues(t float64) color.Color` | InterpolateBlues is the continuous form of the ColorBrewer "Blues" sequential family, running white to dark blue. |
| `func InterpolateBrBG(t float64) color.Color` | InterpolateBrBG is the continuous form of the ColorBrewer "BrBG" diverging family, running brown to blue-green. |
| `func InterpolateBuGn(t float64) color.Color` | InterpolateBuGn is the continuous form of the ColorBrewer "BuGn" sequential family, running blue to green. |
| `func InterpolateBuPu(t float64) color.Color` | InterpolateBuPu is the continuous form of the ColorBrewer "BuPu" sequential family, running blue to purple. |
| `func InterpolateCividis(t float64) color.Color` | InterpolateCividis is a sequential ramp derived from viridis and optimized so that a viewer with deuteranomaly or protanomaly sees very nearly the… |
| `func InterpolateCool(t float64) color.Color` | InterpolateCool is a cyclical cubehelix ramp through the cool half of the hue circle — green, blue, purple — at high saturation. |
| `func InterpolateCubehelixDefault(t float64) color.Color` | InterpolateCubehelixDefault is Dave Green's cubehelix ramp as d3 configures it: black at t = 0, white at t = 1, sweeping a full turn and a half of… |
| `func InterpolateGnBu(t float64) color.Color` | InterpolateGnBu is the continuous form of the ColorBrewer "GnBu" sequential family, running green to blue. |
| `func InterpolateGreens(t float64) color.Color` | InterpolateGreens is the continuous form of the ColorBrewer "Greens" sequential family, running white to dark green. |
| `func InterpolateGreys(t float64) color.Color` | InterpolateGreys is the continuous form of the ColorBrewer "Greys" sequential family, running white to black. |
| `func InterpolateInferno(t float64) color.Color` | InterpolateInferno is one of matplotlib's perceptually uniform ramps, running black through red and orange to pale yellow — the most saturated of… |
| `func InterpolateMagma(t float64) color.Color` | InterpolateMagma is one of matplotlib's perceptually uniform ramps, running black through purple and magenta to pale yellow. |
| `func InterpolateOrRd(t float64) color.Color` | InterpolateOrRd is the continuous form of the ColorBrewer "OrRd" sequential family, running orange to red. |
| `func InterpolateOranges(t float64) color.Color` | InterpolateOranges is the continuous form of the ColorBrewer "Oranges" sequential family, running white to dark orange. |
| `func InterpolatePRGn(t float64) color.Color` | InterpolatePRGn is the continuous form of the ColorBrewer "PRGn" diverging family, running purple to green. |
| `func InterpolatePiYG(t float64) color.Color` | InterpolatePiYG is the continuous form of the ColorBrewer "PiYG" diverging family, running pink to yellow-green. |
| `func InterpolatePlasma(t float64) color.Color` | InterpolatePlasma is one of matplotlib's perceptually uniform ramps, running dark blue through magenta to yellow. |
| `func InterpolatePuBu(t float64) color.Color` | InterpolatePuBu is the continuous form of the ColorBrewer "PuBu" sequential family, running purple to blue. |
| `func InterpolatePuBuGn(t float64) color.Color` | InterpolatePuBuGn is the continuous form of the ColorBrewer "PuBuGn" sequential family, running purple to blue to green. |
| `func InterpolatePuOr(t float64) color.Color` | InterpolatePuOr is the continuous form of the ColorBrewer "PuOr" diverging family, running orange to purple. |
| `func InterpolatePuRd(t float64) color.Color` | InterpolatePuRd is the continuous form of the ColorBrewer "PuRd" sequential family, running purple to red. |
| `func InterpolatePurples(t float64) color.Color` | InterpolatePurples is the continuous form of the ColorBrewer "Purples" sequential family, running white to dark purple. |
| `func InterpolateRainbow(t float64) color.Color` | InterpolateRainbow is d3's cyclical rainbow: a full turn of cubehelix hue with saturation and lightness peaked at t = 0.5, so that f(0) and f(1) are… |
| `func InterpolateRdBu(t float64) color.Color` | InterpolateRdBu is the continuous form of the ColorBrewer "RdBu" diverging family, running red to blue. |
| `func InterpolateRdGy(t float64) color.Color` | InterpolateRdGy is the continuous form of the ColorBrewer "RdGy" diverging family, running red to grey. |
| `func InterpolateRdPu(t float64) color.Color` | InterpolateRdPu is the continuous form of the ColorBrewer "RdPu" sequential family, running red to purple. |
| `func InterpolateRdYlBu(t float64) color.Color` | InterpolateRdYlBu is the continuous form of the ColorBrewer "RdYlBu" diverging family, running red to yellow to blue. |
| `func InterpolateRdYlGn(t float64) color.Color` | InterpolateRdYlGn is the continuous form of the ColorBrewer "RdYlGn" diverging family, running red to yellow to green. |
| `func InterpolateReds(t float64) color.Color` | InterpolateReds is the continuous form of the ColorBrewer "Reds" sequential family, running white to dark red. |
| `func InterpolateSinebow(t float64) color.Color` | InterpolateSinebow is Charlie Loyd's sinebow: three sine waves a third of a turn apart, one per channel, squared. |
| `func InterpolateSpectral(t float64) color.Color` | InterpolateSpectral is the continuous form of the ColorBrewer "Spectral" diverging family, running the full red-to-blue spectral sweep. |
| `func InterpolateTurbo(t float64) color.Color` | InterpolateTurbo is Google's replacement for the jet rainbow ramp: it keeps the high color contrast that makes rainbow ramps popular for spotting… |
| `func InterpolateViridis(t float64) color.Color` | InterpolateViridis is matplotlib's default sequential ramp, and the safe default here too: it is monotone in lightness, so it survives being printed… |
| `func InterpolateWarm(t float64) color.Color` | InterpolateWarm is a cyclical cubehelix ramp through the warm half of the hue circle — purple, red, orange, yellow — at high saturation. |
| `func InterpolateYlGn(t float64) color.Color` | InterpolateYlGn is the continuous form of the ColorBrewer "YlGn" sequential family, running yellow to green. |
| `func InterpolateYlGnBu(t float64) color.Color` | InterpolateYlGnBu is the continuous form of the ColorBrewer "YlGnBu" sequential family, running yellow to green to blue. |
| `func InterpolateYlOrBr(t float64) color.Color` | InterpolateYlOrBr is the continuous form of the ColorBrewer "YlOrBr" sequential family, running yellow to orange to brown. |
| `func InterpolateYlOrRd(t float64) color.Color` | InterpolateYlOrRd is the continuous form of the ColorBrewer "YlOrRd" sequential family, running yellow to orange to red. |
| `func Interpolator(name string) (func(float64) color.Color, bool)` | Interpolator returns the continuous ramp with the given name. |
| `func InterpolatorNames() []string` | InterpolatorNames returns the canonical names of the continuous ramps, sorted. |
| `func Interpolators() map[string]func(float64) color.Color` | Interpolators returns every continuous ramp, keyed by its canonical name. |
| `func Scheme(name string) ([]color.Color, bool)` | Scheme returns the categorical scheme with the given name. |
| `func SchemeFamilies() map[string]Family` | SchemeFamilies returns every ColorBrewer family, keyed by its canonical name. |
| `func SchemeFamilyNames() []string` | SchemeFamilyNames returns the canonical names of the ColorBrewer families, sorted. |
| `func SchemeNames() []string` | SchemeNames returns the canonical names of the categorical schemes, sorted. |
| `func Schemes() map[string][]color.Color` | Schemes returns every categorical scheme, keyed by its canonical name. |

### Types

| Type | What it is |
| --- | --- |
| `Family` | Family is a ColorBrewer scheme published at several class counts, indexed by the class count itself. |

<details>
<summary><code>Family</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func SchemeFamily(name string) (Family, bool)` | SchemeFamily returns the ColorBrewer family with the given name, indexable by class count. |
| `func (f Family) K(k int) []color.Color` | K returns the family's list of k colors, or nil when the family is not published at that class count. |
| `func (f Family) Largest() []color.Color` | Largest returns the widest published list in the family. |
| `func (f Family) Max() int` | Max returns the largest class count the family is published at, or 0 when the family is empty. |
| `func (f Family) Min() int` | Min returns the smallest class count the family is published at, or 0 when the family is empty. |

</details>

### Variables

`SchemeAccent`, `SchemeBlues`, `SchemeBrBG`, `SchemeBuGn`, `SchemeBuPu`, `SchemeCategory10`, `SchemeDark2`, `SchemeGnBu`, `SchemeGreens`, `SchemeGreys`, `SchemeObservable10`, `SchemeOrRd`, `SchemeOranges`, `SchemePRGn`, `SchemePaired`, `SchemePastel1`, `SchemePastel2`, `SchemePiYG`, `SchemePuBu`, `SchemePuBuGn`, `SchemePuOr`, `SchemePuRd`, `SchemePurples`, `SchemeRdBu`, `SchemeRdGy`, `SchemeRdPu`, `SchemeRdYlBu`, `SchemeRdYlGn`, `SchemeReds`, `SchemeSet1`, `SchemeSet2`, `SchemeSet3`, `SchemeSpectral`, `SchemeTableau10`, `SchemeYlGn`, `SchemeYlGnBu`, `SchemeYlOrBr`, `SchemeYlOrRd`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/scalechromatic).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
