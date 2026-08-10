// Command matplotlib-example exercises the github.com/malcolmston/matplotlib
// port: the Figure/Axes model, every chart type it offers, axis labelling,
// titles, legends, grids and explicit limits, the extra plot kinds (step,
// fill-between, stem, error bars, reference lines, annotations), subplots, the
// pyplot-style global API, colors and colormaps, and both output backends
// (PNG and SVG).
//
// It writes its images into ./out next to this program, opens no window, makes
// no network calls, and terminates on its own.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	plt "github.com/malcolmston/matplotlib"
)

const outDir = "out"

var problems []string

func note(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	problems = append(problems, msg)
	fmt.Println("  !! " + msg)
}

func section(name string) {
	fmt.Printf("\n== %s %s\n", name, strings.Repeat("=", max(0, 60-len(name))))
}

func main() {
	fmt.Println("matplotlib port example — github.com/malcolmston/matplotlib")

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "cannot create out dir:", err)
		os.Exit(1)
	}

	linePlot()
	scatterPlot()
	barCharts()
	histogram()
	piechart()
	extraPlotKinds()
	subplots()
	pyplotAPI()
	colorsAndColormaps()
	numericHelpers()
	determinism()
	inMemoryRendering()
	histogramBinningCheck()
	fontAndLegendLimits()

	section("SUMMARY")
	listOutputs()
	fmt.Printf("\n  %d issues noted during the run\n", len(problems))
	for _, p := range problems {
		fmt.Println("   - " + p)
	}
}

// save writes both a PNG and an SVG for the figure and reports on the result.
func save(f *plt.Figure, base string) {
	pngPath := filepath.Join(outDir, base+".png")
	svgPath := filepath.Join(outDir, base+".svg")
	if err := f.SavePNG(pngPath); err != nil {
		note("SavePNG(%s) failed: %v", pngPath, err)
		return
	}
	if err := f.SaveSVG(svgPath); err != nil {
		note("SaveSVG(%s) failed: %v", svgPath, err)
		return
	}
	pi, perr := os.Stat(pngPath)
	si, serr := os.Stat(svgPath)
	if perr != nil || serr != nil {
		note("saved files missing: %v / %v", perr, serr)
		return
	}
	// Sanity-check that something was actually drawn rather than a blank page.
	inked, total := inkFraction(f.RenderImage())
	if inked == 0 {
		note("%s.png rendered a completely blank image", base)
	}
	fmt.Printf("  wrote %-22s png=%6d B  svg=%6d B  non-background pixels=%d/%d\n",
		base, pi.Size(), si.Size(), inked, total)
}

// inkFraction counts pixels that differ from the top-left (background) pixel.
func inkFraction(img *image.RGBA) (inked, total int) {
	b := img.Bounds()
	bg := img.RGBAAt(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			total++
			if img.RGBAAt(x, y) != bg {
				inked++
			}
		}
	}
	return inked, total
}

func linePlot() {
	section("1. Line plot: multiple series, markers, labels, legend, grid")

	x := plt.Linspace(0, 2*math.Pi, 60)
	sin := make([]float64, len(x))
	cos := make([]float64, len(x))
	damp := make([]float64, len(x))
	for i, v := range x {
		sin[i] = math.Sin(v)
		cos[i] = math.Cos(v)
		damp[i] = math.Exp(-v/4) * math.Sin(2*v)
	}

	fig := plt.NewFigure(800, 520)
	ax := fig.AddAxes()

	ax.Plot(x, sin).SetLabel("sin(x)").SetLineWidth(2)
	ax.Plot(x, cos).SetLabel("cos(x)").SetLineWidth(1).SetMarker(plt.MarkerNone)
	ax.Plot(x, damp).
		SetLabel("e^(-x/4)*sin(2x)").
		SetColor(plt.RGB(200, 60, 160)).
		SetLineWidth(1.5).
		SetMarker(plt.MarkerCircle)

	// A horizontal reference line at y=0 plus an annotation.
	ax.AxHLine(0).SetColor(plt.RGBA(0, 0, 0, 120)).SetLabel("y = 0")
	ax.Text(math.Pi, 0.9, "peak region")

	ax.SetTitle("Trigonometric series").
		SetXLabel("x (radians)").
		SetYLabel("amplitude").
		Legend().
		Grid(true)
	ax.SetXLim(0, 2*math.Pi)
	ax.SetYLim(-1.2, 1.2)

	save(fig, "01-line")

	// Chaining returns the same objects, so state is observable afterwards.
	if !ax.ShowLegend || !ax.ShowGrid {
		note("Legend()/Grid(true) did not set ShowLegend/ShowGrid")
	}
	if ax.Title != "Trigonometric series" || ax.XLabel != "x (radians)" || ax.YLabel != "amplitude" {
		note("SetTitle/SetXLabel/SetYLabel did not stick: %q %q %q", ax.Title, ax.XLabel, ax.YLabel)
	}
	fmt.Printf("  axes state: title=%q xlabel=%q ylabel=%q legend=%v grid=%v\n",
		ax.Title, ax.XLabel, ax.YLabel, ax.ShowLegend, ax.ShowGrid)
}

func scatterPlot() {
	section("2. Scatter: marker shapes, sizes, per-series colors")

	// Deterministic pseudo-random clusters (no math/rand, so output is stable).
	cluster := func(cx, cy float64, n int, seed uint64) (xs, ys []float64) {
		s := seed
		next := func() float64 {
			s = s*6364136223846793005 + 1442695040888963407
			return float64((s>>33)%2001)/1000 - 1 // in [-1, 1]
		}
		for i := 0; i < n; i++ {
			xs = append(xs, cx+next())
			ys = append(ys, cy+next())
		}
		return xs, ys
	}

	fig := plt.NewFigure(760, 560)
	ax := fig.AddAxes()

	ax1, ay1 := cluster(-2, 1, 70, 1)
	ax2, ay2 := cluster(2, -1, 70, 7)
	ax3, ay3 := cluster(0, 3, 40, 99)

	ax.Scatter(ax1, ay1).SetLabel("cluster A").SetMarker(plt.MarkerCircle).SetSize(6)
	ax.Scatter(ax2, ay2).SetLabel("cluster B").SetMarker(plt.MarkerSquare).SetSize(7)
	ax.Scatter(ax3, ay3).SetLabel("cluster C").SetMarker(plt.MarkerTriangle).SetSize(9).
		SetColor(plt.RGB(30, 140, 90))

	// Show off the remaining marker glyphs on a small diagonal.
	ax.Scatter([]float64{-3.5, -3.0}, []float64{-2.5, -2.5}).
		SetMarker(plt.MarkerCross).SetSize(10).SetLabel("cross")
	ax.Scatter([]float64{3.0, 3.5}, []float64{-2.5, -2.5}).
		SetMarker(plt.MarkerPlus).SetSize(10).SetLabel("plus")

	ax.AxVLine(0).SetColor(plt.RGBA(0, 0, 0, 80))
	ax.SetTitle("Three clusters (deterministic LCG)").
		SetXLabel("feature 1").SetYLabel("feature 2").Legend().Grid(true)

	save(fig, "02-scatter")
}

func barCharts() {
	section("3. Bar charts: vertical and horizontal, categorical axes")

	labels := []string{"go", "rust", "python", "c", "zig"}
	values := []float64{31, 24, 45, 18, 9}

	fig := plt.NewFigure(760, 460)
	ax := fig.AddAxes()
	ax.Bar(labels, values).SetLabel("mentions").SetColor(plt.RGB(60, 110, 200))
	ax.SetTitle("Vertical bars").SetXLabel("language").SetYLabel("count").
		Legend().Grid(true)
	save(fig, "03-bar-vertical")

	figH := plt.NewFigure(760, 460)
	axH := figH.AddAxes()
	axH.BarH(labels, values).SetLabel("mentions").SetColor(plt.RGB(220, 130, 40))
	axH.SetTitle("Horizontal bars").SetXLabel("count").SetYLabel("language").
		Legend().Grid(true)
	save(figH, "04-bar-horizontal")

	// HOLE: there is no grouped/stacked bar support. Adding two Bar series to
	// one Axes overlays them at identical x positions (each BarChart computes
	// its own full-width bars from its own Labels), so the second hides the
	// first instead of sitting beside it. NumPy-style width/offset arguments
	// (matplotlib's `width=` and `bottom=`) do not exist on BarChart — its only
	// setters are SetColor and SetLabel.
	overlay := plt.NewFigure(700, 420)
	oax := overlay.AddAxes()
	oax.Bar(labels, values).SetLabel("series 1")
	oax.Bar(labels, []float64{20, 30, 20, 30, 20}).SetLabel("series 2")
	oax.SetTitle("Two Bar series overlap (no grouping support)").Legend()
	save(overlay, "05-bar-overlap")
}

func histogram() {
	section("4. Histogram: automatic binning")

	// A deterministic, roughly bell-shaped sample: sum of 6 LCG draws.
	var data []float64
	s := uint64(20260810)
	draw := func() float64 {
		s = s*6364136223846793005 + 1442695040888963407
		return float64((s>>33)%1000) / 1000
	}
	for i := 0; i < 1500; i++ {
		sum := 0.0
		for j := 0; j < 6; j++ {
			sum += draw()
		}
		data = append(data, (sum-3)*4) // roughly centered on 0
	}

	fig := plt.NewFigure(780, 500)
	ax := fig.AddAxes()
	ax.Hist(data, 24).SetLabel("n=1500, 24 bins").SetColor(plt.RGB(90, 160, 120))
	ax.SetTitle("Histogram of a sum-of-uniforms sample").
		SetXLabel("value").SetYLabel("count").Legend().Grid(true)
	save(fig, "06-histogram")

	// bins <= 0 selects the documented default of 10.
	def := plt.NewFigure(600, 400)
	dax := def.AddAxes()
	dax.Hist(data, 0).SetLabel("default bins")
	dax.SetTitle("Hist with bins=0 -> 10 bins").Legend()
	save(def, "07-histogram-default-bins")

	// HOLE: Histogram exposes no accessor for the computed bin edges or counts
	// (the `edges`/`counts` fields are unexported and compute() is private), so
	// a caller cannot read back or reuse the binning the chart used, and there
	// is no standalone Histogram() numeric function like numpy.histogram.
	// There is also no density/normed option, no explicit bin-edge argument,
	// and no cumulative or step-filled variant.
}

func piechart() {
	section("5. Pie chart")

	fig := plt.NewFigure(560, 560)
	ax := fig.AddAxes()
	ax.Pie([]float64{35, 25, 20, 12, 8},
		[]string{"alpha", "beta", "gamma", "delta", "epsilon"})
	ax.SetTitle("Share by segment")
	save(fig, "08-pie")

	// A pie takes over the whole Axes: axis lines, ticks and any other series
	// are suppressed. Labels/limits set on a pie Axes are effectively ignored
	// except for the title.
	mixed := plt.NewFigure(560, 560)
	max_ := mixed.AddAxes()
	max_.Pie([]float64{1, 2, 3}, nil)
	max_.Plot([]float64{0, 1}, []float64{0, 1}).SetLabel("ignored line")
	max_.SetTitle("Pie + line: the line is not drawn").SetXLabel("x").Legend()
	save(mixed, "09-pie-plus-line")
	fmt.Println("  (a Pie on an Axes suppresses every other series on it)")
}

func extraPlotKinds() {
	section("6. Extra plot kinds: step, fill-between, stem, error bars")

	x := plt.Arange(0, 10, 1)
	y := make([]float64, len(x))
	lo := make([]float64, len(x))
	hi := make([]float64, len(x))
	errs := make([]float64, len(x))
	for i, v := range x {
		y[i] = math.Sqrt(v) * 3
		errs[i] = 0.4 + 0.05*v
		lo[i] = y[i] - errs[i]
		hi[i] = y[i] + errs[i]
	}

	fig := plt.NewFigure(860, 560)
	axs := fig.Subplots(2, 2)
	if len(axs) != 4 {
		note("Subplots(2,2) returned %d axes, want 4", len(axs))
		return
	}

	axs[0].Step(x, y).SetLabel("post (default)").SetWhere("post")
	axs[0].Step(x, plt.Linspace(0, 9, len(x))).SetLabel("pre").SetWhere("pre")
	axs[0].SetTitle("Step").SetXLabel("x").SetYLabel("y").Legend().Grid(true)

	axs[1].FillBetween(x, hi, lo).SetLabel("confidence band")
	axs[1].Plot(x, y).SetLabel("mean").SetLineWidth(2)
	axs[1].SetTitle("FillBetween").SetXLabel("x").SetYLabel("y").Legend()

	axs[2].Stem(x, y).SetLabel("stems")
	axs[2].SetTitle("Stem").SetXLabel("x").SetYLabel("y").Legend().Grid(true)

	axs[3].ErrorBar(x, y, errs).SetLabel("y +/- err")
	axs[3].AxHLine(5).SetColor(plt.RGB(200, 60, 60)).SetLabel("threshold")
	axs[3].SetTitle("ErrorBar").SetXLabel("x").SetYLabel("y").Legend()

	save(fig, "10-extra-kinds")

	// FillBetween with nil y2 fills down to zero (documented).
	f2 := plt.NewFigure(700, 420)
	a2 := f2.AddAxes()
	a2.FillBetween(x, y, nil).SetLabel("filled to zero")
	a2.SetTitle("FillBetween(x, y, nil)").Legend().Grid(true)
	save(f2, "11-fill-to-zero")
}

func subplots() {
	section("7. Subplots grid with independent axes")

	fig := plt.NewFigure(900, 640)
	axs := fig.Subplots(2, 3)
	titles := []string{"linear", "quadratic", "cubic", "sqrt", "log1p", "sine"}
	fns := []func(float64) float64{
		func(v float64) float64 { return v },
		func(v float64) float64 { return v * v },
		func(v float64) float64 { return v * v * v },
		math.Sqrt,
		math.Log1p,
		math.Sin,
	}
	x := plt.Linspace(0, 4, 40)
	for i, ax := range axs {
		y := make([]float64, len(x))
		for j, v := range x {
			y[j] = fns[i](v)
		}
		ax.Plot(x, y).SetLabel(titles[i]).SetLineWidth(1.5)
		ax.SetTitle(titles[i]).SetXLabel("x").SetYLabel("f(x)").Grid(true)
	}
	save(fig, "12-subplots")

	// Figure.Axes() returns (and lazily creates) the first Axes.
	f2 := plt.NewFigure(300, 200)
	if f2.Axes() == nil {
		note("Figure.Axes() returned nil on an empty figure")
	}
	if fig.Axes() != axs[0] {
		note("Figure.Axes() did not return the first Axes of a Subplots grid")
	}

	// HOLE: subplot layout is a rigid equal grid. There is no per-axes
	// positioning API (AddAxes always fills the whole figure and takes no
	// rectangle), no spacing/padding control, no shared x/y axes, no
	// suptitle, and no tight_layout equivalent — so subplot titles and tick
	// labels can collide with neighbouring cells.
}

func pyplotAPI() {
	section("8. pyplot-style global API")

	plt.Clf()
	plt.FigSize(720, 460)
	plt.Bar([]string{"q1", "q2", "q3", "q4"}, []float64{12, 19, 15, 24}).
		SetColor(plt.RGB(120, 80, 200)).SetLabel("revenue")
	plt.Title("Quarterly revenue (pyplot API)")
	plt.Xlabel("quarter")
	plt.Ylabel("M USD")
	plt.Legend()
	plt.Grid(true)
	plt.Ylim(0, 30)
	if err := plt.Save(filepath.Join(outDir, "13-pyplot-bar.png")); err != nil {
		note("pyplot Save(.png) failed: %v", err)
	}
	// Save picks the backend from the extension.
	if err := plt.Save(filepath.Join(outDir, "13-pyplot-bar.svg")); err != nil {
		note("pyplot Save(.svg) failed: %v", err)
	}
	fmt.Println("  wrote 13-pyplot-bar.{png,svg} via the global pyplot API")

	// Gcf/Gca expose the implicit figure/axes.
	if plt.Gcf() == nil || plt.Gca() == nil {
		note("Gcf()/Gca() returned nil after FigSize+Bar")
	}
	if plt.Gca().Title != "Quarterly revenue (pyplot API)" {
		note("plt.Title did not reach Gca(): %q", plt.Gca().Title)
	}

	// Clf resets the implicit state; a second chart starts clean.
	plt.Clf()
	plt.FigSize(600, 400)
	plt.Plot(plt.Linspace(0, 5, 30), plt.Linspace(0, 25, 30)).SetLabel("y=5x")
	plt.Scatter([]float64{1, 2, 3}, []float64{10, 8, 20}).SetLabel("samples")
	plt.Title("After Clf")
	plt.Xlim(0, 5)
	plt.Legend()
	if err := plt.Save(filepath.Join(outDir, "14-pyplot-after-clf.png")); err != nil {
		note("pyplot Save after Clf failed: %v", err)
	}
	if got := len(plt.Gcf().Axes().Title); got == 0 {
		note("title lost after Clf + FigSize")
	}
	fmt.Println("  wrote 14-pyplot-after-clf.png")

	// Save with an unrecognized extension: check what happens.
	badPath := filepath.Join(outDir, "15-unknown-extension.jpg")
	err := plt.Save(badPath)
	if err == nil {
		if _, statErr := os.Stat(badPath); statErr == nil {
			note("Save(%q) silently wrote a non-JPEG file for an unsupported extension (no error)",
				filepath.Base(badPath))
		}
	} else {
		fmt.Printf("  Save with .jpg extension correctly errors: %v\n", err)
	}
}

func colorsAndColormaps() {
	section("9. Colors, color ops and colormaps")

	// Hex parsing and the color-operation helpers.
	c, err := plt.Hex("#1f77b4")
	if err != nil {
		note("Hex(\"#1f77b4\") failed: %v", err)
	}
	if c != plt.Blue {
		note("Hex(\"#1f77b4\") = %v, want the tab10 blue %v", c, plt.Blue)
	}
	fmt.Printf("  Hex(\"#1f77b4\")      = %v (Hex() back: %s)\n", c, c.Hex())
	fmt.Printf("  Lighten(0.5)        = %s\n", c.Lighten(0.5).Hex())
	fmt.Printf("  Darken(0.5)         = %s\n", c.Darken(0.5).Hex())
	fmt.Printf("  Grayscale()         = %s (luminance %.4f)\n", c.Grayscale().Hex(), c.Luminance())
	fmt.Printf("  WithAlpha(128)      = %v\n", c.WithAlpha(128))
	fmt.Printf("  Blend(Red, Blue, .5)= %s\n", plt.Blend(plt.Red, plt.Blue, 0.5).Hex())
	h, s, v := c.HSV()
	fmt.Printf("  HSV                 = (%.1f, %.3f, %.3f); round-trip %s\n",
		h, s, v, plt.ColorFromHSV(h, s, v).Hex())
	if rt := plt.ColorFromHSV(h, s, v); absInt(int(rt.R)-int(c.R)) > 1 ||
		absInt(int(rt.G)-int(c.G)) > 1 || absInt(int(rt.B)-int(c.B)) > 1 {
		note("HSV round-trip lost more than 1/255 per channel: %v -> %v", c, rt)
	}

	// Draw a swatch strip for every named colormap using bar charts, so the
	// colormaps are actually exercised through the plotting path.
	maps := []struct {
		name string
		cm   plt.Colormap
	}{
		{"Viridis", plt.Viridis()},
		{"Plasma", plt.Plasma()},
		{"Grays", plt.Grays()},
		{"Jet", plt.Jet()},
		{"Coolwarm", plt.Coolwarm()},
		{"Viridis reversed", plt.Viridis().Reversed()},
		{"custom", plt.NewColormap([]plt.Color{plt.Black, plt.Red, plt.White})},
	}

	fig := plt.NewFigure(900, 900)
	axs := fig.Subplots(4, 2)
	norm := plt.NewNormalize(0, 19)
	for i, m := range maps {
		labels := make([]string, 20)
		values := make([]float64, 20)
		ax := axs[i]
		for j := 0; j < 20; j++ {
			labels[j] = ""
			values[j] = 1
		}
		// One BarChart per bar so each can carry its own colormap color.
		for j := 0; j < 20; j++ {
			single := make([]float64, 20)
			single[j] = 1
			ax.Bar(labels, single).SetColor(norm.Map(m.cm, float64(j)))
		}
		ax.SetTitle(m.name)
		_ = values
	}
	// The 8th cell shows a LogNorm-mapped ramp.
	ln := plt.NewLogNorm(1, 1000)
	labels := make([]string, 20)
	for j := 0; j < 20; j++ {
		single := make([]float64, 20)
		single[j] = 1
		axs[7].Bar(labels, single).SetColor(ln.Map(plt.Viridis(), 1+float64(j)*52.5))
	}
	axs[7].SetTitle("Viridis via LogNorm(1,1000)")
	save(fig, "16-colormaps")

	// Endpoint checks against the documented colormap definitions.
	if got := plt.Viridis().At(0); got != plt.RGB(68, 1, 84) {
		note("Viridis().At(0) = %v, expected the canonical dark purple #440154", got)
	}
	if got := plt.Grays().At(0); got != plt.Black {
		note("Grays().At(0) = %v, want black", got)
	}
	if got := plt.Grays().At(1); got != plt.White {
		note("Grays().At(1) = %v, want white", got)
	}
	if got := plt.Grays().Reversed().At(0); got != plt.White {
		note("Grays().Reversed().At(0) = %v, want white", got)
	}
	// Normalize clamps outside [min,max].
	n := plt.NewNormalize(0, 10)
	if n.Norm(-5) != 0 || n.Norm(15) != 1 || n.Norm(5) != 0.5 {
		note("Normalize did not clamp/scale as expected: %v %v %v",
			n.Norm(-5), n.Norm(15), n.Norm(5))
	}
	fmt.Printf("  Normalize(0,10): -5 -> %.1f, 5 -> %.1f, 15 -> %.1f\n",
		n.Norm(-5), n.Norm(5), n.Norm(15))
	fmt.Printf("  DefaultColors cycle has %d entries, first = %s\n",
		len(plt.DefaultColors), plt.DefaultColors[0].Hex())

	// HOLE: colormaps cannot be attached to a plot. There is no per-point
	// color array on Scatter (matplotlib's `c=` + `cmap=`), no colorbar, no
	// imshow/pcolormesh/contour, and no heatmap of any kind — so a Colormap can
	// only be used by manually mapping values to colors and drawing one series
	// per color, as done above. Hex() exists as both a package function
	// (parse) and a method (format), which reads confusingly.
	if _, err := plt.Hex("nonsense"); err == nil {
		note("Hex(\"nonsense\") returned no error")
	} else {
		fmt.Printf("  Hex(\"nonsense\") correctly errors: %v\n", err)
	}
}

func numericHelpers() {
	section("10. Numeric helpers")

	lin := plt.Linspace(0, 1, 5)
	fmt.Printf("  Linspace(0,1,5)        = %v\n", lin)
	if len(lin) != 5 || lin[0] != 0 || lin[4] != 1 {
		note("Linspace(0,1,5) = %v, want [0 0.25 0.5 0.75 1]", lin)
	}
	ar := plt.Arange(0, 5, 1)
	fmt.Printf("  Arange(0,5,1)          = %v\n", ar)
	if len(ar) != 5 || ar[4] != 4 {
		note("Arange(0,5,1) = %v, want [0 1 2 3 4] (stop exclusive)", ar)
	}
	ls := plt.Logspace(0, 3, 4, 10)
	fmt.Printf("  Logspace(0,3,4,10)     = %v\n", ls)
	if len(ls) != 4 || math.Abs(ls[0]-1) > 1e-9 || math.Abs(ls[3]-1000) > 1e-6 {
		note("Logspace(0,3,4,10) = %v, want [1 10 100 1000]", ls)
	}
	// Degenerate inputs should not panic.
	fmt.Printf("  Linspace(0,1,1)        = %v\n", plt.Linspace(0, 1, 1))
	fmt.Printf("  Linspace(0,1,0)        = %v\n", plt.Linspace(0, 1, 0))
	fmt.Printf("  Arange(0,1,0) (step 0) = %v\n", plt.Arange(0, 1, 0))
	fmt.Printf("  Arange(5,0,-1)         = %v\n", plt.Arange(5, 0, -1))
}

func determinism() {
	section("11. Determinism of both backends")

	build := func() *plt.Figure {
		f := plt.NewFigure(400, 300)
		a := f.AddAxes()
		a.Plot([]float64{0, 1, 2, 3}, []float64{0, 1, 4, 9}).
			SetLabel("x^2").SetMarker(plt.MarkerSquare)
		a.Hist([]float64{1, 1, 2, 3, 3, 3, 4}, 4).SetLabel("h")
		a.SetTitle("determinism").SetXLabel("x").SetYLabel("y").Legend().Grid(true)
		return f
	}

	var b1, b2 bytes.Buffer
	if err := build().WritePNG(&b1); err != nil {
		note("WritePNG failed: %v", err)
	}
	if err := build().WritePNG(&b2); err != nil {
		note("WritePNG failed: %v", err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		note("PNG output is NOT byte-identical across runs (%d vs %d bytes)", b1.Len(), b2.Len())
	} else {
		fmt.Printf("  PNG identical across two builds (%d bytes)\n", b1.Len())
	}

	s1, s2 := build().RenderSVG(), build().RenderSVG()
	if s1 != s2 {
		note("SVG output is NOT identical across runs")
	} else {
		fmt.Printf("  SVG identical across two builds (%d chars)\n", len(s1))
	}
	if !strings.HasPrefix(s1, "<?xml") && !strings.HasPrefix(s1, "<svg") {
		note("RenderSVG output does not start with <?xml or <svg: %.40q", s1)
	}
	// XML escaping of a title with special characters.
	f := plt.NewFigure(320, 240)
	f.AddAxes().Plot([]float64{0, 1}, []float64{0, 1}).SetLabel("a & b")
	f.Axes().SetTitle("<tricky> & \"quoted\"").Legend()
	svg := f.RenderSVG()
	if strings.Contains(svg, "<tricky>") {
		note("RenderSVG did not escape a title containing angle brackets")
	} else {
		fmt.Println("  SVG escapes special characters in text")
	}
	save(f, "17-escaping")
}

func inMemoryRendering() {
	section("12. In-memory rendering: RenderImage, PNGBytes, WriteSVG")

	f := plt.NewFigure(320, 240)
	a := f.AddAxes()
	a.Plot([]float64{0, 1, 2}, []float64{0, 4, 1}).SetLabel("series")
	a.SetTitle("in-memory").Legend()

	img := f.RenderImage()
	if img == nil {
		note("RenderImage returned nil")
		return
	}
	b := img.Bounds()
	fmt.Printf("  RenderImage bounds  = %v\n", b)
	if b.Dx() != 320 || b.Dy() != 240 {
		note("RenderImage size %dx%d does not match the figure's 320x240", b.Dx(), b.Dy())
	}
	if got := img.RGBAAt(0, 0); got != (color.RGBA{255, 255, 255, 255}) {
		note("default background pixel is %v, want opaque white", got)
	}

	raw, err := f.PNGBytes()
	if err != nil {
		note("PNGBytes failed: %v", err)
	} else {
		decoded, derr := png.Decode(bytes.NewReader(raw))
		if derr != nil {
			note("PNGBytes produced undecodable PNG: %v", derr)
		} else {
			fmt.Printf("  PNGBytes            = %d bytes, decodes to %v\n",
				len(raw), decoded.Bounds())
		}
	}

	var sb bytes.Buffer
	if err := f.WriteSVG(&sb); err != nil {
		note("WriteSVG failed: %v", err)
	} else {
		fmt.Printf("  WriteSVG            = %d bytes\n", sb.Len())
	}

	// Non-white background.
	dark := plt.NewFigure(320, 240)
	dark.Background = plt.RGB(24, 24, 32)
	da := dark.AddAxes()
	da.Plot([]float64{0, 1, 2}, []float64{2, 0, 3}).SetColor(plt.RGB(240, 200, 60)).SetLabel("light on dark")
	da.SetTitle("Custom background").Legend().Grid(true)
	save(dark, "18-dark-background")
	if got := dark.RenderImage().RGBAAt(0, 0); got != (color.RGBA{24, 24, 32, 255}) {
		note("Figure.Background was not honored: corner pixel %v", got)
	}
	fmt.Printf("  Figure DPI default  = %v\n", dark.DPI)

	// Empty axes must still render rather than panic.
	empty := plt.NewFigure(200, 150)
	empty.AddAxes().SetTitle("no data")
	save(empty, "19-empty-axes")
}

// histogramBinningCheck verifies the histogram's visual bin counts against
// hand-computed counts, using the only channel available: pixel inspection is
// impractical, so instead the check confirms the plot renders and documents
// that the counts are not readable programmatically.
func histogramBinningCheck() {
	section("13. Histogram binning: what a caller can and cannot verify")

	// 10 values in [0,10): with 5 bins over [0,10] the counts are
	// [2 2 2 2 2] for 0..9 stepping by 1 -> bins [0,2) [2,4) [4,6) [6,8) [8,10].
	data := plt.Arange(0, 10, 1)
	f := plt.NewFigure(400, 300)
	a := f.AddAxes()
	h := a.Hist(data, 5)
	a.SetTitle("Arange(0,10,1), 5 bins").Legend()
	save(f, "20-hist-known-counts")

	fmt.Printf("  Histogram struct exposes: Data(len=%d) Bins=%d Color=%v Label=%q\n",
		len(h.Data), h.Bins, h.Color, h.Label)
	fmt.Println("  computed edges/counts are unexported — a caller cannot assert on them")

	// What *is* checkable: the y axis must reach the tallest bin. With 5 bins
	// the tallest count is 2, so the rendered image must contain ink.
	if inked, _ := inkFraction(f.RenderImage()); inked == 0 {
		note("histogram with known data rendered blank")
	}
}

// fontAndLegendLimits probes the PNG backend's built-in bitmap font for missing
// glyphs and shows that the legend box has a hard-coded width.
func fontAndLegendLimits() {
	section("14. PNG bitmap-font coverage and legend box sizing")

	// Render each candidate rune as an annotation on an otherwise blank figure
	// and compare the ink to a plain space. Zero extra ink => no glyph.
	inkFor := func(s string) int {
		f := plt.NewFigure(120, 60)
		a := f.AddAxes()
		a.Text(0.5, 0.5, s)
		// Suppress axis decoration influence by comparing against the same
		// figure drawn with a space, so only the glyph differs.
		inked, _ := inkFraction(f.RenderImage())
		return inked
	}
	blank := inkFor(" ")
	var missing []string
	for _, r := range "^&$@|~{}`µ°–é" {
		if inkFor(string(r)) <= blank {
			missing = append(missing, string(r))
		}
	}
	if len(missing) > 0 {
		note("PNG font has no glyph for %s — these runes render as blank space with no error (SVG output is unaffected because it emits real <text>)",
			strings.Join(missing, " "))
	} else {
		fmt.Println("  every probed rune has a glyph")
	}

	// Legend width is fixed, so long labels overflow the box.
	f := plt.NewFigure(500, 300)
	a := f.AddAxes()
	a.Plot([]float64{0, 1, 2}, []float64{0, 1, 0}).
		SetLabel("a legend label that is far too long to fit")
	a.Plot([]float64{0, 1, 2}, []float64{0, 2, 1}).SetLabel("short")
	a.SetTitle("Legend box is a fixed 110px wide").Legend()
	save(f, "21-legend-overflow")
	note("the legend box width is hard-coded (110px) and labels are neither measured nor truncated, so long labels overflow it — see 21-legend-overflow.png")
}

func listOutputs() {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		note("cannot list %s: %v", outDir, err)
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	fmt.Printf("  %d files in ./%s:\n", len(names), outDir)
	for _, n := range names {
		fmt.Println("   ", n)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
