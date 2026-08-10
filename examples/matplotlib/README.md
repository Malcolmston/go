# matplotlib example

A runnable exercise of the Go port `github.com/malcolmston/matplotlib`, consumed
as a published module (no `replace` directive).

**Resolved module version:** `github.com/malcolmston/matplotlib
v0.0.0-20260719012643-a56acf6c9b5f` — the repo publishes no semver tags, so
`go get ...@latest` yields a pseudo-version (`go list -m -versions` lists none,
and `@v0.1.0` / `@v0.2.0` fail with "unknown revision" even though the repo's
`VERSION` file says `0.2.0`). The published tree is byte-identical to the local
`matplotlib/` working copy for every `.go` file.

The program writes 40+ PNG/SVG files into `./out`, prints a labelled report of
what it produced and verified, and exits on its own. It opens no window and
makes no network calls.

## Run

```sh
cd examples/matplotlib
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Builds clean, `go vet` clean, runs to completion with exit code 0.

## What it demonstrates

1. **Line plot** — three series on one `Axes` with per-series color, line width
   and markers, plus `AxHLine`, `Text` annotation, `SetTitle`/`SetXLabel`/
   `SetYLabel`, `Legend()`, `Grid(true)`, `SetXLim`/`SetYLim`.
2. **Scatter** — three deterministic LCG-generated clusters plus a strip
   exercising all five marker glyphs (`MarkerCircle`, `MarkerSquare`,
   `MarkerTriangle`, `MarkerCross`, `MarkerPlus`), `SetSize`, `AxVLine`.
3. **Bar charts** — vertical `Bar` and horizontal `BarH` on categorical axes,
   plus a figure showing what happens when two `Bar` series share an `Axes`.
4. **Histogram** — 1500-sample sum-of-uniforms with 24 explicit bins and a
   second figure using `bins=0` to hit the documented 10-bin default.
5. **Pie** — a labelled pie, and a pie sharing an `Axes` with a line series to
   show that the pie takes over the whole cell.
6. **Extra plot kinds** — `Step` (both `pre` and `post`), `FillBetween` (with a
   real band and with `nil` y2 filling to zero), `Stem`, `ErrorBar`, `AxHLine`.
7. **Subplots** — a 2×3 grid of independent `Axes`, plus checks that
   `Figure.Axes()` returns the first cell and lazily creates one.
8. **pyplot global API** — `Clf`, `FigSize`, `Bar`, `Plot`, `Scatter`, `Title`,
   `Xlabel`, `Ylabel`, `Legend`, `Grid`, `Xlim`, `Ylim`, `Save`, `Gcf`, `Gca`,
   including a `Clf` reset and a save with an unsupported extension.
9. **Colors and colormaps** — `Hex` parse (and its error path), `Color.Hex`,
   `Lighten`, `Darken`, `Grayscale`, `Luminance`, `WithAlpha`, `Blend`, `HSV`
   round-trip via `ColorFromHSV`, all five named colormaps plus
   `NewColormap`/`Reversed`, `Normalize` clamping, `LogNorm`, `DefaultColors`.
10. **Numeric helpers** — `Linspace`, `Arange`, `Logspace` and their degenerate
    inputs (`num<=1`, `step==0`, negative step).
11. **Determinism** — the same figure built twice produces byte-identical PNG
    and identical SVG; SVG special-character escaping is checked.
12. **In-memory rendering** — `RenderImage` (bounds and background pixel),
    `PNGBytes` (re-decoded with `image/png`), `WriteSVG`, a custom
    `Figure.Background`, and an empty `Axes` that must render without panicking.
13. **Histogram introspection** — documents that computed edges/counts are
    unreachable.
14. **Font/legend limits** — probes the PNG bitmap font for missing glyphs and
    renders a legend with an over-long label.

Every saved figure is also checked for non-background ink, so a silently blank
render would be reported.

## Holes found

Nothing panicked and nothing rendered blank. All the findings are missing
features, silent-failure behavior, or layout limitations.

### Real bugs / silent failures

- **`Save` with an unsupported extension silently writes a PNG under the wrong
  name and returns `nil`.** `plt.Save("x.jpg")` produced a file that `file(1)`
  identifies as `PNG image data`, with no error. `Figure.Save` only special-cases
  `.svg` and falls through to PNG for everything else, so a typo'd or
  unsupported format is indistinguishable from success. Demonstrated in
  `pyplotAPI()`; the resulting `out/15-unknown-extension.jpg` is a PNG.
- **The PNG backend silently drops runes it has no glyph for.** `glyphFor`
  returns a blank glyph and its "known" boolean is discarded at both call sites
  in `png.go`. The example probes and confirms that `^ & $ @ | ~ { } ` µ ° – é`
  all render as whitespace with no error or warning — so `"e^(-x/4)*sin(2x)"`
  loses its caret in PNG while the SVG of the same figure is correct. Only
  ASCII letters/digits and a short punctuation list are covered; there is no
  way to supply a font.
- **The legend box has a hard-coded width of 110px and never measures or
  truncates its labels.** `Axes.renderLegend` sets `boxW := 110.0` and draws
  each label at `bx+32` with `AnchorStart`, so any label longer than ~13
  characters overflows the box and runs off the plot (and off the figure). See
  `out/21-legend-overflow.png`. Legend placement is also fixed at the top-right
  — there is no `loc`/position option and no way to turn off the legend frame.

### Missing API (relative to what the README/matplotlib imply)

- **No grouped or stacked bars.** `BarChart`'s only setters are `SetColor` and
  `SetLabel`; there is no bar width, offset or `bottom`, and each `BarChart`
  computes full-width bars from its own `Labels`. Two `Bar` series on one `Axes`
  therefore draw on top of each other (`out/05-bar-overlap.png`), which reads as
  a broken stacked chart rather than an error.
- **Colormaps cannot be attached to any plot.** There is no per-point color
  array on `Scatter` (matplotlib's `c=` / `cmap=`), no colorbar, and no
  `imshow`/`pcolormesh`/`contour`/heatmap of any kind. `Colormap`, `Normalize`
  and `LogNorm` are usable only by mapping values to `Color` by hand and drawing
  one series per color — which is what `colorsAndColormaps()` has to do, at one
  `BarChart` per bar.
- **No histogram introspection and no numeric histogram.** `Histogram.edges`
  and `.counts` are unexported and `compute()` is private, so a caller can never
  read back the binning the chart used, and there is no `Histogram()` function
  returning counts. There is also no density/normed option, no explicit
  bin-edges argument, and no cumulative or step-filled variant.
- **No log-scale axes.** `Logspace` exists as a numeric helper, but `Axes` has
  no `SetXScale`/`SetYScale` and no log tick generation, so log data can only be
  plotted by pre-transforming it (and then the tick labels are wrong).
- **No custom tick control.** `Axes` has no `SetXTicks`/`SetYTicks`/tick-label
  or tick-formatter API — `xticks`/`xtickLabels` are unexported and filled only
  by the internal `niceTicks`. Rotating the categorical labels of a busy bar
  chart is impossible.
- **No layout control.** `AddAxes` always fills the entire figure and takes no
  rectangle, `Subplots` only makes an equal grid, and there is no spacing/pad
  setting, `suptitle`, shared axes, or `tight_layout`. In
  `out/12-subplots.png` the bottom row's x-labels sit hard against the figure
  edge. `Figure.DPI` is a dead field: it is set to 100 by `NewFigure` and never
  read anywhere in the package (`grep DPI` finds only the declaration, the
  initializer and two doc comments), so sizes are pixels only.
- **No per-series line style.** `LinePlot` has `SetLineWidth` and `SetMarker`
  but no dash/linestyle option, no marker-size option, and no alpha (alpha only
  via building a `Color` with `RGBA`).
- **No other chart types.** No boxplot, violin, hexbin, quiver, stackplot,
  polar, twin axes, `Axes.Bar` with error bars, `text` with arrows/`annotate`,
  spans (`axhspan`/`axvspan`), or 3-D anything.
- **No JPEG/PDF output** despite `Save` accepting arbitrary paths (see the
  silent-PNG bug above).

### Non-idiomatic / confusing API

- **`Hex` is both a package function and a method with opposite meanings**:
  `plt.Hex("#1f77b4") (Color, error)` parses, while `Color.Hex() string`
  formats. Reading `c.Hex()` next to `plt.Hex(s)` is genuinely ambiguous.
- **The pyplot layer is package-level mutable global state** (`currentFig`,
  `currentAx`) with no mutex, so it is not safe for concurrent use and there is
  no documentation saying so.
- **`Axes.Legend()` is a setter that reads like a getter** and returns `*Axes`
  for chaining, while the pyplot `Legend()` returns nothing — the two layers
  have different shapes for the same operation.
- **A `PieChart`'s `render` is a no-op**; pies are drawn out-of-band by
  `Axes.renderPie`, driven by a private `isPie` flag and a `firstPie` lookup, so
  only the *first* pie on an `Axes` is ever drawn and every other series on that
  `Axes` is silently discarded (`out/09-pie-plus-line.png` shows the line
  missing, with no warning).
- **`SetXLim`/`SetYLim` are silently ignored on categorical and pie axes.**
  `Axes.finalize` returns early for a pie, and for a vertical bar chart it hard-
  codes `xmin/xmax` to `-0.5 … n-0.5` without consulting `xlimSet` (mirrored for
  `SetYLim` on `BarH`). The setter still reports success by returning `*Axes`.
- **Marker/anchor types are unexported-value `int` enums with no `String()`**,
  so debug printing a `Marker` gives a bare number.
- **The 5×7 bitmap font has no descender room**, so `g`, `p`, `q` and `y` are
  squashed into the x-height band and read as `9`, `P`, and so on at the default
  11–15px sizes (visible in every title in `./out`). Text quality in PNG output
  is noticeably worse than in SVG, which uses a real `font-family: monospace`.

### Things that made it hard to use

- Discovering the API requires reading the source: the README's method table
  lists only six chart types and omits the whole `plots_extra.go` set (`Step`,
  `FillBetween`, `Stem`, `ErrorBar`, `AxHLine`, `AxVLine`, `Text`) and the
  colormap/`Normalize`/`LogNorm` surface entirely.
- Because there is no colormap→plot integration and no bar width, the two most
  natural "make a nice chart" tasks (a color-mapped scatter and a grouped bar
  chart) both require dropping down to one-series-per-element loops.
- There is no way to assert on anything numeric that the chart computed, so
  validating a plot means inspecting pixels — which is what this example ends up
  doing via `inkFraction`.
