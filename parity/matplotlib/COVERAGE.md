# matplotlib parity coverage

- Upstream oracle: **matplotlib 3.10.0** (CPython 3.13.5, `Agg` backend forced),
  already importable in the environment; no venv was needed and nothing was
  installed globally. Verify with
  `python3 -c "import matplotlib; print(matplotlib.__version__)"`.
  numpy 2.2.6 is used only to reproduce `Axes.hist`'s binning
  (`numpy.histogram`, which is what `hist` delegates to).
- Go port: **`github.com/malcolmston/matplotlib@v0.0.0-20260810111548-1b42add03764`**,
  consumed as a published module (no `replace` directive).
- Run with `GOWORK=off go test ./parity/matplotlib/`. Machine-readable totals
  land in `parity.json`.

## What is in scope, and what is not

**Pixel-level rendering parity is explicitly out of scope.** These are two
independently written rasterisers: matplotlib draws antialiased paths through
Agg with FreeType-hinted DejaVu Sans text at configurable DPI, and the port
draws Bresenham lines with a 5x7 bitmap font into an `image.RGBA`. Their plot
rectangles are not even placed by the same rule — matplotlib positions a subplot
by figure fraction, the port by fixed pixel padding — so no two corresponding
pixels are ever expected to agree. An image diff would report 100% failure for
every case and would tell us nothing about whether the port *computes* the same
answers. It was not attempted.

What *is* exactly comparable is everything upstream computes before it draws:

| surface | how it is compared |
| --- | --- |
| axis limits and tick locations | `Axes.get_xlim`/`get_xticks` (AutoLocator/MaxNLocator) vs the port's limits and ticks |
| tick label formatting | `ScalarFormatter` output vs the port's tick label strings |
| histogram binning | `numpy.histogram` edges and counts vs `Axes.Hist`'s bars |
| colour parsing/formatting | `colors.to_rgba` / `to_hex` vs `Hex` / `Color.Hex` |
| named colours and the property cycle | `to_rgba('red')`, `rcParams['axes.prop_cycle']` vs `Red`, `DefaultColors` |
| colormap sampling | `get_cmap(name)(t)` vs `Viridis()/Plasma()/Jet()/Coolwarm()/Grays().At(t)` |
| colour-space conversion | `colors.rgb_to_hsv` / `hsv_to_rgb` vs `RGBToHSV` / `HSVToRGB` |
| value normalisation | `colors.Normalize` / `LogNorm` vs `Normalize` / `LogNorm` |
| data -> axes coordinates | `Axes.transLimits` vs the port's internal data->pixel map |
| legend and label plumbing | `get_title`/`get_xlabel`/`get_ylabel`, `Legend.get_texts` vs the port's rendered label text |
| output format selection | `Figure.savefig` vs `Figure.Save`, comparing the file's magic bytes only |
| text glyph coverage | `FT2Font.get_char_index` vs "did the port actually draw this character" |

### How the Go side is observed

The port exposes almost none of this: axis limits, tick values, tick labels and
histogram bins all live in unexported fields of `Axes` and `Histogram`, and
there are no getters. Its only inspectable output is `Figure.RenderSVG`, so
`go/run.go` renders the figure and reads the values back out of the SVG — the
axis frame gives the plot rectangle, the tick stubs give tick pixel positions,
the 11px text elements give tick labels, and the port's own linear data->pixel
map is inverted from two ticks to recover the limits. This is measurement of the
port's real computed output, not a re-implementation of it.

SVG coordinates are written with two decimals, so anything recovered this way
carries roughly 2e-5 relative error.

### Float tolerance

- **Default: 1e-9**, applied as `|a-b| <= tol * max(1, |a|, |b|)`. Used for every
  pure computation: colours, colormaps, `Normalize`/`LogNorm`, HSV conversion,
  tick *values* parsed from labels, and all string/bool results.
- **1e-4** for values recovered from SVG geometry — the `xlim`/`ylim`/`catlim`
  fields of the `ticks` group, the `edges` of the `hist` group, and the
  `axesfrac` of the `transforms` group. Declared per case as `"tol": 0.0001` in
  `cases/*.json`.
- Histogram counts are integers and are rounded after inversion.
- JSON has no NaN/Infinity literal; both runners encode non-finite floats as the
  sentinel strings `"NaN"`, `"Infinity"`, `"-Infinity"`.

## How the upstream inventory was enumerated

Mechanically, against the installed package — not from the docs or from memory:

```
python3 -c 'import matplotlib; matplotlib.use("Agg")
import matplotlib.axes, matplotlib.figure, matplotlib.colors, matplotlib.ticker, matplotlib.pyplot as plt
for name, obj in [("Axes", matplotlib.axes.Axes), ("Figure", matplotlib.figure.Figure),
                  ("colors", matplotlib.colors), ("ticker", matplotlib.ticker), ("pyplot", plt)]:
    print(name, len([n for n in dir(obj) if not n.startswith("_")]))
print("colormaps", len(plt.colormaps()))
print("named colors", len(matplotlib.colors.get_named_colors_mapping()))'
```

which reports, for 3.10.0:

| upstream namespace | public names |
| --- | --- |
| `matplotlib.axes.Axes` | 293 |
| `matplotlib.figure.Figure` | 136 |
| `matplotlib.colors` | 57 |
| `matplotlib.ticker` | 39 |
| `matplotlib.pyplot` | 243 |
| `matplotlib` (top level) | 103 |
| registered colormaps | 180 |
| named colours | 1163 |

The Go side was enumerated with
`GOWORK=off go doc -all github.com/malcolmston/matplotlib`: **105 exported
functions/methods** across 20 exported types.

**matplotlib's surface vastly exceeds the port's** — on the order of 900 public
names across the namespaces above versus about 105 in the port, before counting
the 180 colormaps and 1163 named colours. Enumerating all of them as `missing`
would be noise, so the table below lists (a) every symbol actually compared,
(b) every port symbol that exists but has no case, and (c) the upstream *areas*
the port does not address at all, aggregated. The parity percentage is computed
only over the symbols actually compared.

## Symbol table

### Compared — agreed (`match`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Axes.set_title` / `get_title` | `Axes.SetTitle` | match | `labels-all-three`, `labels-empty`, `labels-xml-special-chars`, `labels-unicode` | XML escaping round-trips |
| `Axes.set_xlabel` / `get_xlabel` | `Axes.SetXLabel` | match | same four | |
| `Axes.set_ylabel` / `get_ylabel` | `Axes.SetYLabel` | match | same four | |
| `Axes.transLimits` | internal `Axes.tx`/`ty`, observed via SVG | match | `d2a-center`, `d2a-origin`, `d2a-corner`, `d2a-quarter`, `d2a-outside-range`, `d2a-inverted-xlim`, `d2a-nonround-limits` | agrees for fixed limits, including reversed axes and points outside the view |
| `colors.to_hex` | `Color.Hex` | match | `hexfmt-tab-blue`, `hexfmt-black`, `hexfmt-white`, `hexfmt-low` | |
| `colors.rgb_to_hsv` | `RGBToHSV` | match | `rgb2hsv-red`, `rgb2hsv-green`, `rgb2hsv-gray`, `rgb2hsv-black`, `rgb2hsv-tab-blue` | bit-for-bit, including hue 0 for unsaturated input |
| `colors.hsv_to_rgb` | `HSVToRGB` | match | `hsv2rgb-magenta`, `hsv2rgb-unsaturated`, `hsv2rgb-teal`, `hsv2rgb-wrap-hue-1` | |
| `rcParams['axes.prop_cycle']` | `DefaultColors` | match | `cycle-0`, `cycle-3`, `cycle-9`, `cycle-wrap-12` | exact tab10, same order, same wraparound |
| `Colormap.__call__` clamping | `Colormap.At` | match | `cmap-clamped-out-of-range` | out-of-range `t` clamps to the endpoint colours on both sides |
| `Figure.savefig` (`.png`) | `Figure.Save` / `SavePNG` | match | `save-png` | |
| `Figure.savefig` (`.svg`) | `Figure.Save` / `SaveSVG` | match | `save-svg` | |
| `Figure.add_subplot` | `Figure.AddAxes` | match | `labels-all-three`, `ticks-explicit-0-10-0-100` | both yield a working single axes filling the figure |

### Compared — disagreed (`differs`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `ticker.AutoLocator` / `MaxNLocator` (`get_xticks`/`get_yticks`) | internal `niceTicks`/`ticksWithin` | differs | `ticks-explicit-0-3-0-10`, `ticks-explicit-negative`, `ticks-explicit-tiny-range`, `ticks-explicit-nonround`, `ticks-plot-auto`, `ticks-scatter-auto`, `ticks-hist-auto` | the port always aims for 5 ticks; matplotlib picks its bin count from the axes size in points, and typically produces more. See "Divergences" |
| `ticker.ScalarFormatter` (`get_xticklabels`) | internal `fmtTick` | differs | `labelfmt-0-1`, `labelfmt-0-100000` | `"0"` vs `"0.0"`; `"1.0e+05"` vs `"100000"` |
| `Axes.set_xlim` | `Axes.SetXLim` | differs | `ticks-explicit-*`, `ticks-bar-xlim-ignored` | honoured on numeric axes; silently ignored on a bar axes |
| `Axes.set_ylim` | `Axes.SetYLim` | differs | `ticks-explicit-*`, `ticks-barh-ylim-ignored` | ditto for `barh` |
| `Axes.plot` (+ autoscale) | `Axes.Plot` | differs | `ticks-plot-auto`, `ticks-plot-auto-wide` | the port has no data margins and snaps the limits to the outermost tick |
| `Axes.scatter` (+ autoscale) | `Axes.Scatter` | differs | `ticks-scatter-auto` | same cause |
| `Axes.hist` / `numpy.histogram` | `Axes.Hist` | differs | `hist-4-bins`, `hist-10-bins`, `hist-1-bin`, `hist-negative-data`, `hist-fractional-edges`, `hist-outlier-tail`, `hist-single-value`, `hist-empty-1-bin`, `hist-empty-3-bins`, `hist-bins-zero`, `hist-bins-negative` | ordinary binning agrees exactly; degenerate and invalid inputs do not |
| `Axes.bar` | `Axes.Bar` | differs | `cat-bar-3`, `cat-bar-5`, `ticks-bar-xlim-ignored` | categorical tick positions and labels agree; the view limits do not (no margins) |
| `Axes.barh` | `Axes.BarH` | differs | `cat-barh-4`, `ticks-barh-ylim-ignored` | ditto |
| `Axes.pie` | `Axes.Pie` | differs | `ticks-pie-lims-ignored` | matplotlib keeps a real (equal-aspect) axes; the port's pie axes computes no limits or ticks at all |
| `Axes.legend` / `Legend.get_texts` | `Axes.Legend` + `*.SetLabel` | differs | `legend-two-series`, `legend-three-series`, `legend-unlabelled-series`, `legend-underscore-label` | entry order and text agree; the port has no `_`-prefix exclusion rule |
| `colors.to_rgba` (hex forms) | `Hex` | differs | `hex-6-digit`, `hex-3-digit`, `hex-uppercase`, `hex-white`, `hex-8-digit-alpha`, `hex-no-hash`, `hex-bad-digits`, `hex-bad-length` | 3- and 6-digit forms agree; `#rrggbbaa` and a missing `#` do not |
| `colors.to_rgba` (CSS colour names) | `Black`, `White`, `Red`, `Green`, `Blue` | differs | `named-black`, `named-white`, `named-red`, `named-green`, `named-blue` | the port's `Red`/`Green`/`Blue` are tab10 colours, not CSS ones |
| `colors.Normalize` | `Normalize` | differs | `norm-clip-in-range`, `norm-clip-out-of-range`, `norm-noclip-out-of-range`, `norm-negative-range`, `norm-degenerate` | matches `clip=True`; the port has no unclipped mode |
| `colors.LogNorm` | `LogNorm` | differs | `lognorm-decades`, `lognorm-fractional`, `lognorm-clip-out-of-range`, `lognorm-nonpositive-value` | matches for positive in-range input; non-positive input is masked upstream, 0 in the port |
| `get_cmap('viridis')` | `Viridis()` | differs | `cmap-viridis` | 5-anchor approximation (documented as such) |
| `get_cmap('plasma')` | `Plasma()` | differs | `cmap-plasma` | approximation |
| `get_cmap('jet')` | `Jet()` | differs | `cmap-jet` | approximation |
| `get_cmap('coolwarm')` | `Coolwarm()` | differs | `cmap-coolwarm` | approximation |
| `get_cmap('gray')` | `Grays()` | differs | `cmap-gray` | only a 1/255 rounding difference at t=0.75 |
| `Figure.savefig` (format from extension) | `Figure.Save` | differs | `save-pdf`, `save-jpg`, `save-unknown-ext` | the port writes PNG bytes under whatever name it was given |
| `Figure.set_dpi` / `Figure.dpi` | `Figure.DPI` | differs | `dpi-unchanged-100`, `dpi-doubled-200`, `dpi-lowered-72` | the field is stored and never read |
| `ft2font.FT2Font.get_char_index` / `font_manager` | built-in 5x7 bitmap font (PNG backend) | differs | `glyph-ascii-A`, `glyph-ascii-hash`, `glyph-latin1-eacute`, `glyph-greek-alpha`, `glyph-micro-sign`, `glyph-arrow` | ASCII agrees; every non-ASCII character is silently blank |

### Present in the port but not covered by a case (`untested`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Axes.errorbar` | `Axes.ErrorBar` | untested | — | only pixel output would distinguish it |
| `Axes.fill_between` | `Axes.FillBetween` | untested | — | ditto |
| `Axes.step` | `Axes.Step` / `StepPlot.SetWhere` | untested | — | ditto |
| `Axes.stem` | `Axes.Stem` | untested | — | ditto |
| `Axes.axhline` / `axvline` | `Axes.AxHLine` / `AxVLine` | untested | — | ditto |
| `Axes.text` / `annotate` | `Axes.Text`, `Annotation` | untested | — | ditto |
| `Axes.grid` | `Axes.Grid` | untested | — | affects grid lines only |
| `Figure.subplots` | `Figure.Subplots` | untested | — | multi-axes layout is pixel geometry |
| `Figure.set_facecolor` | `Figure.Background` | untested | — | |
| `Figure.canvas.buffer_rgba` | `Figure.RenderImage`, `PNGBytes`, `WritePNG`, `WriteSVG` | untested | — | used internally by the glyph probe, not asserted |
| `LinearSegmentedColormap.from_list` | `NewColormap` | untested | — | |
| `Colormap.reversed` | `Colormap.Reversed` | untested | — | |
| `colors.Normalize.__call__` -> `Colormap` | `Normalize.Map`, `LogNorm.Map` | untested | — | composition of two symbols that are each covered |
| `Line2D.set_linewidth` / `set_marker` / `set_color` | `LinePlot.SetLineWidth`/`SetMarker`/`SetColor`, `ScatterPlot.SetSize`/`SetMarker`, `BarChart.SetColor`, `Histogram.SetColor`, `RefLine.*`, `StemPlot.*`, `FillBetween.*`, `ErrorBar.*` | untested | — | style setters with pixel-only effect |
| `pyplot.plot`/`bar`/`hist`/`pie`/`scatter`/`title`/`xlabel`/`ylabel`/`xlim`/`ylim`/`legend`/`grid`/`savefig`/`gca`/`gcf`/`clf`/`figure` | `Plot`, `Bar`, `BarH`, `Hist`, `Pie`, `Scatter`, `Title`, `Xlabel`, `Ylabel`, `Xlim`, `Ylim`, `Legend`, `Grid`, `Save`, `Gca`, `Gcf`, `Clf`, `FigSize` | untested | — | the stateful wrapper delegates to the object API that *is* covered; global state makes it a poor fit for a shared runner |

### Not ported (`missing`)

Aggregated by area, because listing ~900 individual names would be noise. Each
row was confirmed absent from `go doc -all github.com/malcolmston/matplotlib`.

| upstream symbol / area | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Axes.get_xlim`/`get_ylim`/`get_xticks`/`get_yticks`/`get_xticklabels` | — | missing | — | **no getters at all**; this is why the port's computed values had to be read out of its SVG |
| `Axes.set_xticks`/`set_yticks`/`set_xticklabels`/`set_yticklabels`/`tick_params`/`minorticks_on` | — | missing | — | no tick API |
| `matplotlib.ticker` (39 public names: `MaxNLocator`, `LogLocator`, `FixedLocator`, `ScalarFormatter`, `FuncFormatter`, `PercentFormatter`, …) | — | missing | — | no locator or formatter is exposed or replaceable |
| `Axes.set_xscale`/`set_yscale`/`semilogx`/`semilogy`/`loglog`, `scale.LogScale`/`SymmetricalLogScale`/`LogitScale` | — | missing | — | **no log or non-linear axes**; the port's axes are always linear (`LogNorm` covers only colour mapping) |
| `Figure.colorbar` / `Axes.colorbar` / `ScalarMappable` | — | missing | — | **no colorbar**; `Normalize`+`Colormap` exist but nothing draws a scale |
| `Axes.imshow`/`pcolormesh`/`pcolor`/`contour`/`contourf`/`matshow`/`spy` | — | missing | — | no 2-D array/image plotting |
| `Axes.boxplot`/`violinplot`/`hexbin`/`hist2d`/`eventplot`/`stackplot`/`quiver`/`streamplot`/`broken_barh`/`stairs` | — | missing | — | no statistical or vector-field plots |
| `Axes.twinx`/`twiny`/`secondary_xaxis`/`inset_axes`/`sharex`/`sharey` | — | missing | — | no shared or twinned axes |
| `Axes.set_aspect`/`Axes.axis('equal')`/`Figure.tight_layout`/`constrained_layout`/`GridSpec` | — | missing | — | no layout engine; padding is fixed pixel constants |
| `Axes.annotate` arrows, `patches.*` (Rectangle, Circle, Polygon, FancyArrow, …), `Path`, `PathEffects` | — | missing | — | no artist/patch layer |
| `mathtext` (`$\alpha$`), `TeX` rendering, `font_manager`, `FontProperties`, `rcParams` font selection | — | missing | — | one built-in 5x7 bitmap font, ASCII only |
| `matplotlib.rcParams` / `rc_context` / `style.use` (103 top-level names) | — | missing | — | no configuration system |
| `animation.*`, `widgets.*`, interactive backends, `pyplot.show` | — | missing | — | static output only |
| `dates.*` (`DateFormatter`, `AutoDateLocator`, date units), `units`/`category` unit framework | — | missing | — | no date or unit-aware axes |
| `colors.ListedColormap`, `BoundaryNorm`, `TwoSlopeNorm`, `PowerNorm`, `CenteredNorm`, `SymLogNorm`, `colors.CSS4_COLORS`/`XKCD_COLORS` (1163 named colours), `colors.to_rgba_array`, `colors.LightSource` | — | missing | — | of the 57 public names in `matplotlib.colors`, 5 have counterparts; 175 of the 180 registered colormaps are absent |
| `Figure.savefig` to `pdf`/`ps`/`eps`/`pgf`/`webp`/`tif`/`raw`, `backend_pdf`, `backend_ps` | — | missing | — | PNG and SVG only |
| `Axes3D` / `mpl_toolkits.mplot3d`, `mpl_toolkits.axes_grid1` | — | missing | — | no 3-D or toolkit axes |

### Go-only (`extra`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `Color.Lighten`, `Color.Darken`, `Color.Luminance`, `Color.Grayscale`, `Color.WithAlpha`, `Blend` | extra | — | no matplotlib equivalent |
| — | `Color.HSV`, `ColorFromHSV` | extra | — | 8-bit convenience wrappers over the covered float conversions |
| — | `RGB`, `RGBA`, `Color.RGBA` (`image/color.Color`) | extra | — | Go-idiomatic constructors |
| — | `Linspace`, `Arange`, `Logspace` | extra | — | numpy helpers, not matplotlib API; deliberately not scored here |
| — | `Marker` constants, `TextAnchor` constants, `Point` | extra | — | port-specific enums |
| — | `Figure.RenderSVG` returning a string | extra | — | matplotlib has no equivalent in-memory SVG accessor; it is what makes this harness possible |

## Divergences found

Ordered by value. All were observed, not inferred.

1. **Autoscale limits have no data margins.** matplotlib expands the data range
   by `rcParams['axes.xmargin']` (5%) and *then* places ticks; the port snaps the
   view to the outermost "nice" tick. For `x=[0,1,2,3]` matplotlib gives
   `xlim=(-0.15, 3.15)`, the port `(0, 3)` (`ticks-plot-auto`). The port's
   endpoints therefore always sit exactly on the frame — data at the extremes is
   drawn on the spine.
2. **Tick density is fixed at ~5 instead of size-dependent.** matplotlib's
   `AutoLocator` chooses its bin count from the axes size in points; the port
   always calls its `niceTicks(lo, hi, 5)`. At 640x480 with `xlim=(0,3)`
   matplotlib emits `0, 0.5, 1, 1.5, 2, 2.5, 3` and the port `0, 1, 2, 3`
   (`ticks-explicit-0-3-0-10`). Same for `ylim=(-1,1)`: 9 ticks vs 5
   (`ticks-explicit-negative`). This is the single most visible numeric
   difference and it affects every plot.
3. **`SetXLim` is silently ignored on a bar axes.** `Bar()` marks the x axis
   categorical, and `finalize` then overwrites the user limits with
   `(-0.5, n-0.5)`. With `SetXLim(0, 1)` on three categories the port still
   reports `xlim≈(-0.5, 2.5)` while matplotlib reports `(0, 1)`
   (`ticks-bar-xlim-ignored`). `SetYLim` on `BarH` behaves the same way
   (`ticks-barh-ylim-ignored`). Confirmed.
4. **A pie axes computes nothing.** `Axes.finalize` returns immediately when
   `isPie`, so there are no limits, no ticks and no frame: `SetXLim`/`SetYLim`
   are dead and nothing is inspectable. matplotlib keeps a real axes with
   `xlim=(-1.25, 1.25)` (and honours `set_xlim`). Confirmed
   (`ticks-pie-lims-ignored`).
5. **Categorical view limits omit margins.** For three bars matplotlib gives
   `(-0.54, 2.54)` (bar width 0.8 plus 5% margins), the port `(-0.5, 2.5)`
   (`cat-bar-3`, `cat-bar-5`, `cat-barh-4`). Tick positions and labels agree.
6. **`Figure.DPI` is never read.** Confirmed: setting `DPI = 200` on a
   640x480 figure leaves the rendered output 640x480, where matplotlib's
   `set_dpi(200)` makes it 1280x960 (`dpi-doubled-200`, `dpi-lowered-72`).
   The field is documented and settable but has no effect anywhere.
7. **`Save` writes PNG bytes under any name.** Confirmed: `Save("out.pdf")`
   produces a file starting `\x89PNG` (`save-pdf`), likewise `.jpg`
   (`save-jpg`), and `Save("out.xyz")` succeeds where matplotlib raises
   `ValueError: Format 'xyz' is not supported` (`save-unknown-ext`). Only
   `.svg` is special-cased; every other extension is a silent lie.
8. **The PNG backend silently drops glyphs it lacks.** Confirmed: `glyphFor`
   substitutes a blank space for anything outside its 5x7 table, with no error
   and no replacement box. `é`, `α`, `µ` and `→` all render as nothing while
   matplotlib's DejaVu Sans has all four (`glyph-latin1-eacute`,
   `glyph-greek-alpha`, `glyph-micro-sign`, `glyph-arrow`). Note that the SVG
   backend passes the text through untouched, so the same figure loses text in
   PNG and keeps it in SVG (`labels-unicode` passes).
9. **Tick label formatting differs.** The port prints `0` and `1` where
   matplotlib prints `0.0` and `1.0`, and switches to `1.0e+05` at 1e5 where
   matplotlib still prints `100000` (`labelfmt-0-1`, `labelfmt-0-100000`).
10. **`Red`, `Green`, `Blue` are not CSS colours.** `to_rgba('red')` is
    `(1,0,0,1)`; the port's `Red` is tab10's `(0.839,0.153,0.157,1)`
    (`named-red`, `named-green`, `named-blue`). `Black`/`White` agree. A user
    porting code that passed `"red"` gets a different colour with no warning.
11. **`Normalize` has no unclipped mode.** matplotlib's default `clip=False`
    returns `-0.5` and `1.5` for values outside `[vmin,vmax]`; the port always
    clamps to `[0,1]` (`norm-noclip-out-of-range`). Fine for colour lookup,
    wrong if the normalised value is used for anything else.
12. **`LogNorm` returns 0 for non-positive input** where matplotlib masks it
    (`lognorm-nonpositive-value`). 0 is also the legitimate result for `vmin`,
    so invalid input is indistinguishable from the bottom of the range.
13. **Degenerate histogram ranges widen differently.** For `[3,3,3,3]` with 2
    bins, numpy uses `(2.5, 3.5)` and puts all four samples in the upper bin;
    the port uses `(3, 4)` and puts them in the lower one (`hist-single-value`).
14. **Empty data ignores the bin count.** `Hist([], 3)` produces a single
    `[0,1]` bin; `numpy.histogram([], bins=3)` produces three
    (`hist-empty-3-bins`). With `bins=1` they agree (`hist-empty-1-bin`).
15. **A non-positive bin count is silently replaced by 10.** `Hist(data, 0)`
    and `Hist(data, -3)` both bin into 10; numpy raises
    ``ValueError: `bins` must be positive`` (`hist-bins-zero`,
    `hist-bins-negative`).
16. **`Hex` accepts and rejects the wrong things.** It accepts `1f77b4`
    without the leading `#` (matplotlib raises) and rejects `#1f77b480`
    (matplotlib reads the alpha suffix) — `hex-no-hash`, `hex-8-digit-alpha`.
    The 3- and 6-digit forms and both malformed inputs agree.
17. **Legend has no `_`-prefix exclusion.** matplotlib omits artists whose
    label starts with `_`; the port shows `_hidden` in the legend
    (`legend-underscore-label`).
18. **Named colormaps are visibly approximate.** `Viridis`, `Plasma`, `Jet`
    and `Coolwarm` are 5-anchor piecewise-linear approximations (the port
    documents this). Worst observed error is `jet` at t=0.5, where blue is
    0.502 against matplotlib's 0.478. `Grays` is off only by a 1/255 rounding
    at t=0.75.

Positive findings worth recording: ordinary histogram binning agrees exactly
(edges and counts, including negative data, fractional edges, a single bin and a
heavy outlier tail); `rgb_to_hsv`/`hsv_to_rgb` are bit-for-bit identical to
matplotlib's; `DefaultColors` is exactly tab10 in the right order; the
data->axes-fraction transform agrees for fixed limits including reversed axes
and out-of-range points; and title/label round-tripping survives SVG escaping
and non-ASCII text.

## Score

Case level, from `parity.json`:

| group | cases | match | differs |
| --- | --- | --- | --- |
| ticks | 18 | 2 | 16 |
| hist | 11 | 7 | 4 |
| colors | 45 | 33 | 12 |
| transforms | 7 | 7 | 0 |
| labels | 8 | 7 | 1 |
| bugs | 14 | 5 | 9 |
| **total** | **103** | **61** | **42** |

**Case parity: 61/103 = 59.22%** (0 deviations, 0 runner errors).

Symbol level, counting only symbols with at least one case:

| status | count |
| --- | --- |
| match | 12 |
| differs | 23 |
| **compared (denominator)** | **35** |
| untested (port symbols with no case) | 15 rows |
| missing (upstream areas absent from the port) | 18 rows |
| extra (Go-only) | 6 rows |

**Symbol parity: 12/35 = 34.3% of the compared symbols agree on every case.**

Both numbers are scored only over what was compared. They say nothing about the
~900 public upstream names outside that set, the vast majority of which the port
does not implement.
