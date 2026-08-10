// Command run is the Go-side parity runner for github.com/malcolmston/matplotlib.
//
// It speaks JSON Lines on stdio: one request object per line in, one response
// object per line out, in the same order. See ../../HARNESS.md.
//
// # How the port's computed values are observed
//
// The port exposes almost none of its computed geometry: axis limits, tick
// locations, tick labels and histogram bins all live in unexported fields. Its
// only inspectable output is the SVG document produced by Figure.RenderSVG, so
// this runner renders the figure and reads the values back out of the SVG:
//
//   - the axis frame is the pair of #333333 stroke-width-1.5 lines, giving the
//     plot rectangle in pixels (px0,py0,px1,py1);
//   - each tick is a 5px #333333 stroke-width-1 stub, paired in emission order
//     with the 11px tick label text that follows it;
//   - axis limits are recovered by solving the port's own linear data->pixel
//     map from two ticks with known values plus the frame edges;
//   - histogram bins are the series-coloured <rect> elements, inverted through
//     that same map.
//
// SVG coordinates are written with two decimals, so geometry-derived values
// carry a relative error of roughly 2e-5; those cases declare a looser
// tolerance. Purely numeric operations (colours, normalisation, colour-space
// conversion) are compared exactly.
//
// Nothing here compares pixels between the two libraries; see ../COVERAGE.md.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	mpl "github.com/malcolmston/matplotlib"
)

// ---------------------------------------------------------------- wire types

type request struct {
	ID   string          `json:"id"`
	Fn   string          `json:"fn"`
	Args json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// args is the decoded argument object; every field is optional.
type args struct {
	Kind      string      `json:"kind"`
	X         []float64   `json:"x"`
	Y         []float64   `json:"y"`
	Data      []float64   `json:"data"`
	Labels    []string    `json:"labels"`
	Bins      *int        `json:"bins"`
	XLim      []float64   `json:"xlim"`
	YLim      []float64   `json:"ylim"`
	FigSize   []float64   `json:"figsize"`
	DPI       *float64    `json:"dpi"`
	Point     []float64   `json:"point"`
	Series    []series    `json:"series"`
	Legend    bool        `json:"legend"`
	Title     string      `json:"title"`
	XLabel    string      `json:"xlabel"`
	YLabel    string      `json:"ylabel"`
	Ext       string      `json:"ext"`
	Char      string      `json:"char"`
	S         string      `json:"s"`
	RGB       []float64   `json:"rgb"`
	HSV       []float64   `json:"hsv"`
	Name      string      `json:"name"`
	Fractions []float64   `json:"fractions"`
	Min       *float64    `json:"min"`
	Max       *float64    `json:"max"`
	Values    []float64   `json:"values"`
	Clip      bool        `json:"clip"`
	Index     *int        `json:"index"`
	_         interface{} `json:"-"`
}

type series struct {
	Label string    `json:"label"`
	X     []float64 `json:"x"`
	Y     []float64 `json:"y"`
}

// ------------------------------------------------------------ number sentinels

// encNum encodes a float for JSON, using the same non-finite sentinels as the
// Python runner because JSON has no NaN/Infinity literal.
func encNum(v float64) any {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	return v
}

func encNums(vs []float64) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = encNum(v)
	}
	return out
}

// ----------------------------------------------------------------- SVG model

type svgLine struct {
	x1, y1, x2, y2 float64
	stroke         string
	width          float64
}

type svgRect struct {
	x, y, w, h float64
	fill       string
}

type svgCircle struct {
	cx, cy, r float64
	fill      string
}

type svgText struct {
	x, y    float64
	size    float64
	fill    string
	anchor  string
	rotated bool
	content string
}

type svgDoc struct {
	width, height int
	lines         []svgLine
	rects         []svgRect
	circles       []svgCircle
	texts         []svgText
}

var (
	reSVG = regexp.MustCompile(`<svg [^>]*width="(\d+)" height="(\d+)"`)
	reLn  = regexp.MustCompile(`<line x1="([^"]*)" y1="([^"]*)" x2="([^"]*)" y2="([^"]*)" stroke="([^"]*)" stroke-width="([^"]*)"/>`)
	reRc  = regexp.MustCompile(`<rect x="([^"]*)" y="([^"]*)" width="([^"]*)" height="([^"]*)" fill="([^"]*)"/>`)
	reCi  = regexp.MustCompile(`<circle cx="([^"]*)" cy="([^"]*)" r="([^"]*)" fill="([^"]*)"/>`)
	reTx  = regexp.MustCompile(`<text x="([^"]*)" y="([^"]*)" font-family="monospace" font-size="([^"]*)" fill="([^"]*)" text-anchor="([^"]*)"( transform="[^"]*")?>([^<]*)</text>`)
)

func mustF(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

func unescapeXML(s string) string {
	return strings.NewReplacer(
		"&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&",
	).Replace(s)
}

func parseSVG(doc string) *svgDoc {
	d := &svgDoc{}
	if m := reSVG.FindStringSubmatch(doc); m != nil {
		d.width, _ = strconv.Atoi(m[1])
		d.height, _ = strconv.Atoi(m[2])
	}
	for _, m := range reLn.FindAllStringSubmatch(doc, -1) {
		d.lines = append(d.lines, svgLine{mustF(m[1]), mustF(m[2]), mustF(m[3]), mustF(m[4]), m[5], mustF(m[6])})
	}
	for _, m := range reRc.FindAllStringSubmatch(doc, -1) {
		d.rects = append(d.rects, svgRect{mustF(m[1]), mustF(m[2]), mustF(m[3]), mustF(m[4]), m[5]})
	}
	for _, m := range reCi.FindAllStringSubmatch(doc, -1) {
		d.circles = append(d.circles, svgCircle{mustF(m[1]), mustF(m[2]), mustF(m[3]), m[4]})
	}
	for _, m := range reTx.FindAllStringSubmatch(doc, -1) {
		d.texts = append(d.texts, svgText{
			x: mustF(m[1]), y: mustF(m[2]), size: mustF(m[3]), fill: m[4],
			anchor: m[5], rotated: m[6] != "", content: unescapeXML(m[7]),
		})
	}
	return d
}

const (
	axisStroke = "#333333"
	cycle0     = "#1f77b4" // DefaultColors[0], the first series colour
)

func near(a, b float64) bool { return math.Abs(a-b) <= 0.02 }

// frame is the port's plot rectangle in pixels.
type frame struct{ px0, py0, px1, py1 float64 }

func (d *svgDoc) frame() (frame, error) {
	var f frame
	var haveH, haveV bool
	for _, l := range d.lines {
		if l.stroke != axisStroke || l.width != 1.5 {
			continue
		}
		if near(l.y1, l.y2) && !haveH { // bottom spine
			f.px0, f.px1, f.py1 = l.x1, l.x2, l.y1
			haveH = true
		} else if near(l.x1, l.x2) && !haveV { // left spine
			f.py0 = l.y1
			haveV = true
		}
	}
	if !haveH || !haveV {
		return f, fmt.Errorf("port drew no numeric axis frame (axes exposes no limits or ticks)")
	}
	return f, nil
}

// xTicks returns the drawn x tick pixel positions paired with their labels, in
// ascending order.
func (d *svgDoc) xTicks(f frame) (px []float64, labels []string) {
	for _, l := range d.lines {
		if l.stroke == axisStroke && l.width == 1 && near(l.x1, l.x2) && near(l.y1, f.py1) && near(l.y2, f.py1+5) {
			px = append(px, l.x1)
		}
	}
	for _, t := range d.texts {
		if t.size == 11 && t.anchor == "middle" && !t.rotated && t.y > f.py1 {
			labels = append(labels, t.content)
		}
	}
	return px, labels
}

func (d *svgDoc) yTicks(f frame) (py []float64, labels []string) {
	for _, l := range d.lines {
		if l.stroke == axisStroke && l.width == 1 && near(l.y1, l.y2) && near(l.x1, f.px0-5) && near(l.x2, f.px0) {
			py = append(py, l.y1)
		}
	}
	for _, t := range d.texts {
		if t.size == 11 && t.anchor == "end" && !t.rotated && t.x < f.px0 {
			labels = append(labels, t.content)
		}
	}
	return py, labels
}

// legendLabels returns the legend entry texts (11px, left-anchored, inside the
// plot rectangle).
func (d *svgDoc) legendLabels(f frame) []string {
	var out []string
	for _, t := range d.texts {
		if t.size == 11 && t.anchor == "start" && !t.rotated && t.x > f.px0 && t.y < f.py1 {
			out = append(out, t.content)
		}
	}
	return out
}

// axisMap is the recovered affine data<->pixel map for one axis.
type axisMap struct {
	lo, hi float64 // data limits
}

// solveX recovers the x data limits from two ticks and the frame edges.
func solveX(f frame, px []float64, vals []float64) (axisMap, error) {
	if len(px) != len(vals) {
		return axisMap{}, fmt.Errorf("x tick/label count mismatch: %d marks, %d labels", len(px), len(vals))
	}
	if len(px) < 2 {
		return axisMap{}, fmt.Errorf("need >=2 x ticks to recover limits, got %d", len(px))
	}
	a, b := 0, len(px)-1
	if vals[a] == vals[b] {
		return axisMap{}, fmt.Errorf("degenerate x ticks")
	}
	// px = px0 + (v-lo)*m
	m := (px[b] - px[a]) / (vals[b] - vals[a])
	if m == 0 || math.IsInf(m, 0) || math.IsNaN(m) {
		return axisMap{}, fmt.Errorf("degenerate x scale")
	}
	lo := vals[a] - (px[a]-f.px0)/m
	return axisMap{lo: lo, hi: lo + (f.px1-f.px0)/m}, nil
}

// solveY recovers the y data limits; pixel y grows downward.
func solveY(f frame, py []float64, vals []float64) (axisMap, error) {
	if len(py) != len(vals) {
		return axisMap{}, fmt.Errorf("y tick/label count mismatch: %d marks, %d labels", len(py), len(vals))
	}
	if len(py) < 2 {
		return axisMap{}, fmt.Errorf("need >=2 y ticks to recover limits, got %d", len(py))
	}
	a, b := 0, len(py)-1
	if vals[a] == vals[b] {
		return axisMap{}, fmt.Errorf("degenerate y ticks")
	}
	// py = py1 - (v-lo)*s
	s := (py[a] - py[b]) / (vals[b] - vals[a])
	if s == 0 || math.IsInf(s, 0) || math.IsNaN(s) {
		return axisMap{}, fmt.Errorf("degenerate y scale")
	}
	lo := vals[a] - (f.py1-py[a])/s
	return axisMap{lo: lo, hi: lo + (f.py1-f.py0)/s}, nil
}

func parseLabels(labels []string) ([]float64, error) {
	out := make([]float64, len(labels))
	for i, l := range labels {
		v, err := strconv.ParseFloat(strings.TrimSpace(l), 64)
		if err != nil {
			return nil, fmt.Errorf("tick label %q is not numeric", l)
		}
		out[i] = v
	}
	return out, nil
}

// ------------------------------------------------------------ figure building

// newFigure builds a figure of figsize inches * dpi pixels, mirroring what the
// Python side gets from plt.figure(figsize=..., dpi=...).
func newFigure(a *args) *mpl.Figure {
	fs := a.FigSize
	if len(fs) != 2 {
		fs = []float64{6.4, 4.8}
	}
	dpi := 100.0
	if a.DPI != nil {
		dpi = *a.DPI
	}
	f := mpl.NewFigure(int(math.Round(fs[0]*dpi)), int(math.Round(fs[1]*dpi)))
	f.DPI = dpi
	return f
}

func drawSeries(ax *mpl.Axes, a *args) error {
	kind := a.Kind
	if kind == "" {
		kind = "plot"
	}
	switch kind {
	case "plot":
		ax.Plot(a.X, a.Y)
	case "scatter":
		ax.Scatter(a.X, a.Y)
	case "hist":
		bins := 10
		if a.Bins != nil {
			bins = *a.Bins
		}
		ax.Hist(a.X, bins)
	case "bar":
		ax.Bar(a.Labels, a.Y)
	case "barh":
		ax.BarH(a.Labels, a.Y)
	case "pie":
		ax.Pie(a.Y, a.Labels)
	default:
		return fmt.Errorf("unknown kind: %q", kind)
	}
	return nil
}

// ----------------------------------------------------------------- handlers

func fnAxisTicks(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	if err := drawSeries(ax, a); err != nil {
		return nil, err
	}
	if len(a.XLim) == 2 {
		ax.SetXLim(a.XLim[0], a.XLim[1])
	}
	if len(a.YLim) == 2 {
		ax.SetYLim(a.YLim[0], a.YLim[1])
	}
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	xpx, xlab := d.xTicks(f)
	ypx, ylab := d.yTicks(f)
	// A categorical axis carries category names instead of numbers; its ticks
	// sit at successive integer data positions, exactly as matplotlib's do.
	xv, err := tickValues(xlab)
	if err != nil {
		return nil, fmt.Errorf("x axis: %w", err)
	}
	yv, err := tickValues(ylab)
	if err != nil {
		return nil, fmt.Errorf("y axis: %w", err)
	}
	xm, err := solveX(f, xpx, xv)
	if err != nil {
		return nil, err
	}
	ym, err := solveY(f, ypx, yv)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"xlim":   encNums([]float64{xm.lo, xm.hi}),
		"ylim":   encNums([]float64{ym.lo, ym.hi}),
		"xticks": encNums(xv),
		"yticks": encNums(yv),
	}, nil
}

// tickValues turns tick labels into data positions: numeric labels are parsed,
// and non-numeric (categorical) labels take the successive integer positions
// matplotlib also uses for categories.
func tickValues(labels []string) ([]float64, error) {
	if v, err := parseLabels(labels); err == nil {
		return v, nil
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("no ticks drawn")
	}
	out := make([]float64, len(labels))
	for i := range out {
		out[i] = float64(i)
	}
	return out, nil
}

func fnTickLabels(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	if err := drawSeries(ax, a); err != nil {
		return nil, err
	}
	if len(a.XLim) == 2 {
		ax.SetXLim(a.XLim[0], a.XLim[1])
	}
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	_, xlab := d.xTicks(f)
	if xlab == nil {
		xlab = []string{}
	}
	return map[string]any{"xticklabels": xlab}, nil
}

func fnCatTicks(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	if err := drawSeries(ax, a); err != nil {
		return nil, err
	}
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	horiz := a.Kind == "barh"
	var pix []float64
	var labels []string
	if horiz {
		pix, labels = d.yTicks(f)
	} else {
		pix, labels = d.xTicks(f)
	}
	if len(pix) != len(labels) {
		return nil, fmt.Errorf("categorical tick/label count mismatch: %d vs %d", len(pix), len(labels))
	}
	if len(pix) < 2 {
		return nil, fmt.Errorf("need >=2 categorical ticks, got %d", len(pix))
	}
	// Categories are expected at successive integer data positions; verify the
	// pixel spacing really is uniform before assuming it.
	step := pix[1] - pix[0]
	for i := 2; i < len(pix); i++ {
		if math.Abs((pix[i]-pix[i-1])-step) > 0.05 {
			return nil, fmt.Errorf("categorical ticks are not evenly spaced")
		}
	}
	idx := make([]float64, len(pix))
	for i := range idx {
		idx[i] = float64(i)
	}
	var m axisMap
	if horiz {
		m, err = solveY(f, pix, idx)
	} else {
		m, err = solveX(f, pix, idx)
	}
	if err != nil {
		return nil, err
	}
	lo, hi := math.Min(m.lo, m.hi), math.Max(m.lo, m.hi)
	return map[string]any{
		"catticks":  encNums(idx),
		"catlabels": labels,
		"catlim":    encNums([]float64{lo, hi}),
	}, nil
}

func fnHistBins(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	bins := 10
	if a.Bins != nil {
		bins = *a.Bins
	}
	ax.Hist(a.Data, bins)
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	xpx, xlab := d.xTicks(f)
	ypx, ylab := d.yTicks(f)
	xv, err := parseLabels(xlab)
	if err != nil {
		return nil, fmt.Errorf("x axis: %w", err)
	}
	yv, err := parseLabels(ylab)
	if err != nil {
		return nil, fmt.Errorf("y axis: %w", err)
	}
	xm, err := solveX(f, xpx, xv)
	if err != nil {
		return nil, err
	}
	ym, err := solveY(f, ypx, yv)
	if err != nil {
		return nil, err
	}
	var bars []svgRect
	for _, r := range d.rects {
		if r.fill == cycle0 {
			bars = append(bars, r)
		}
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("port drew no histogram bars")
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].x < bars[j].x })
	invX := func(px float64) float64 {
		return xm.lo + (px-f.px0)*(xm.hi-xm.lo)/(f.px1-f.px0)
	}
	invY := func(py float64) float64 {
		return ym.lo + (f.py1-py)*(ym.hi-ym.lo)/(f.py1-f.py0)
	}
	edges := make([]float64, 0, len(bars)+1)
	counts := make([]int, 0, len(bars))
	for _, r := range bars {
		edges = append(edges, invX(r.x))
		// A bar's top edge is the count; zero-count bars have zero height and
		// sit on the baseline. Counts are integers, so round.
		counts = append(counts, int(math.Round(invY(r.y))))
	}
	edges = append(edges, invX(bars[len(bars)-1].x+bars[len(bars)-1].w))
	return map[string]any{"counts": counts, "edges": encNums(edges)}, nil
}

func fnDataToAxes(a *args) (any, error) {
	if len(a.Point) != 2 || len(a.XLim) != 2 || len(a.YLim) != 2 {
		return nil, fmt.Errorf("data_to_axes needs point, xlim and ylim")
	}
	fig := newFigure(a)
	ax := fig.AddAxes()
	ax.Scatter([]float64{a.Point[0]}, []float64{a.Point[1]})
	ax.SetXLim(a.XLim[0], a.XLim[1])
	ax.SetYLim(a.YLim[0], a.YLim[1])
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	var found *svgCircle
	for i := range d.circles {
		if d.circles[i].fill == cycle0 {
			found = &d.circles[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("port drew no marker for the query point")
	}
	fx := (found.cx - f.px0) / (f.px1 - f.px0)
	fy := (f.py1 - found.cy) / (f.py1 - f.py0)
	return map[string]any{"axesfrac": encNums([]float64{fx, fy})}, nil
}

func fnLabels(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	for _, s := range a.Series {
		ax.Plot(s.X, s.Y).SetLabel(s.Label)
	}
	ax.SetTitle(a.Title).SetXLabel(a.XLabel).SetYLabel(a.YLabel)
	if a.Legend {
		ax.Legend()
	}
	d := parseSVG(fig.RenderSVG())
	f, err := d.frame()
	if err != nil {
		return nil, err
	}
	title, xlabel, ylabel := "", "", ""
	for _, t := range d.texts {
		switch {
		case t.size == 15:
			title = t.content
		case t.size == 12 && t.rotated:
			ylabel = t.content
		case t.size == 12 && !t.rotated:
			xlabel = t.content
		}
	}
	legend := d.legendLabels(f)
	if legend == nil {
		legend = []string{}
	}
	return map[string]any{
		"title": title, "xlabel": xlabel, "ylabel": ylabel, "legend": legend,
	}, nil
}

var magics = []struct {
	prefix []byte
	name   string
}{
	{[]byte("\x89PNG\r\n\x1a\n"), "png"},
	{[]byte("%PDF"), "pdf"},
	{[]byte("\xff\xd8\xff"), "jpeg"},
	{[]byte("<?xml"), "svg"},
	{[]byte("<svg"), "svg"},
	{[]byte("%!PS"), "ps"},
}

func sniff(head []byte) string {
	for _, m := range magics {
		if len(head) >= len(m.prefix) && string(head[:len(m.prefix)]) == string(m.prefix) {
			return m.name
		}
	}
	n := len(head)
	if n > 8 {
		n = 8
	}
	return "unknown:" + fmt.Sprintf("%x", head[:n])
}

func fnSaveExt(a *args) (any, error) {
	fig := newFigure(a)
	ax := fig.AddAxes()
	ax.Plot([]float64{0, 1, 2}, []float64{0, 1, 4})
	dir, err := os.MkdirTemp("", "parity-mpl-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "out"+a.Ext)
	if err := fig.Save(path); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	head := b
	if len(head) > 16 {
		head = head[:16]
	}
	return map[string]any{
		"exists": true, "format": sniff(head), "nonempty": len(b) > 0,
	}, nil
}

// fnGlyphSupported answers "can the backend actually draw this character?" by
// rendering it and checking whether any pixel changed relative to rendering a
// space in the same place. The port's PNG backend substitutes a blank glyph for
// anything missing from its 5x7 bitmap font.
func fnGlyphSupported(a *args) (any, error) {
	runes := []rune(a.Char)
	if len(runes) != 1 {
		return nil, fmt.Errorf("char must be exactly one character")
	}
	render := func(title string) []uint8 {
		fig := newFigure(a)
		ax := fig.AddAxes()
		ax.Plot([]float64{0, 1}, []float64{0, 1})
		ax.SetTitle(title)
		return fig.RenderImage().Pix
	}
	got := render(a.Char)
	blank := render(" ")
	if len(got) != len(blank) {
		return nil, fmt.Errorf("render size changed between probes")
	}
	for i := range got {
		if got[i] != blank[i] {
			return map[string]any{"supported": true}, nil
		}
	}
	return map[string]any{"supported": false}, nil
}

func fnDPIEffect(a *args) (any, error) {
	if len(a.FigSize) != 2 || a.DPI == nil {
		return nil, fmt.Errorf("dpi_effect needs figsize and dpi")
	}
	// Build at the library default of 100 dpi, then change the DPI.
	fig := mpl.NewFigure(int(math.Round(a.FigSize[0]*100)), int(math.Round(a.FigSize[1]*100)))
	fig.AddAxes().Plot([]float64{0, 1}, []float64{0, 1})
	fig.DPI = *a.DPI
	d := parseSVG(fig.RenderSVG())
	return map[string]any{"width": d.width, "height": d.height}, nil
}

func colorRGBA(c mpl.Color) []any {
	return encNums([]float64{
		float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255,
	})
}

func fnHexParse(a *args) (any, error) {
	c, err := mpl.Hex(a.S)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rgba": colorRGBA(c)}, nil
}

func fnHexFormat(a *args) (any, error) {
	if len(a.RGB) != 3 {
		return nil, fmt.Errorf("hex_format needs a 3-element rgb")
	}
	c := mpl.RGB(uint8(a.RGB[0]), uint8(a.RGB[1]), uint8(a.RGB[2]))
	return map[string]any{"hex": c.Hex()}, nil
}

var namedColors = map[string]mpl.Color{
	"black": mpl.Black,
	"white": mpl.White,
	"red":   mpl.Red,
	"green": mpl.Green,
	"blue":  mpl.Blue,
}

func fnNamedColor(a *args) (any, error) {
	c, ok := namedColors[a.Name]
	if !ok {
		return nil, fmt.Errorf("matplotlib: unknown colour name %q", a.Name)
	}
	return map[string]any{"rgba": colorRGBA(c)}, nil
}

func fnCycleColor(a *args) (any, error) {
	if a.Index == nil {
		return nil, fmt.Errorf("cycle_color needs index")
	}
	n := len(mpl.DefaultColors)
	i := ((*a.Index % n) + n) % n
	return map[string]any{"rgba": colorRGBA(mpl.DefaultColors[i]), "cyclelen": n}, nil
}

func fnCmapSample(a *args) (any, error) {
	var cm mpl.Colormap
	switch a.Name {
	case "viridis":
		cm = mpl.Viridis()
	case "plasma":
		cm = mpl.Plasma()
	case "jet":
		cm = mpl.Jet()
	case "coolwarm":
		cm = mpl.Coolwarm()
	case "gray", "Greys":
		cm = mpl.Grays()
	default:
		return nil, fmt.Errorf("matplotlib: no such colormap %q", a.Name)
	}
	out := make([]any, 0, len(a.Fractions))
	for _, t := range a.Fractions {
		out = append(out, colorRGBA(cm.At(t)))
	}
	return map[string]any{"rgba": out}, nil
}

func fnNormalize(a *args) (any, error) {
	if a.Min == nil || a.Max == nil {
		return nil, fmt.Errorf("normalize needs min and max")
	}
	n := mpl.NewNormalize(*a.Min, *a.Max)
	out := make([]any, 0, len(a.Values))
	for _, v := range a.Values {
		out = append(out, encNum(n.Norm(v)))
	}
	return map[string]any{"norm": out}, nil
}

func fnLogNorm(a *args) (any, error) {
	if a.Min == nil || a.Max == nil {
		return nil, fmt.Errorf("lognorm needs min and max")
	}
	n := mpl.NewLogNorm(*a.Min, *a.Max)
	out := make([]any, 0, len(a.Values))
	for _, v := range a.Values {
		out = append(out, encNum(n.Norm(v)))
	}
	return map[string]any{"norm": out}, nil
}

func fnRGBToHSV(a *args) (any, error) {
	if len(a.RGB) != 3 {
		return nil, fmt.Errorf("rgb_to_hsv needs a 3-element rgb")
	}
	h, s, v := mpl.RGBToHSV(a.RGB[0], a.RGB[1], a.RGB[2])
	return map[string]any{"hsv": encNums([]float64{h, s, v})}, nil
}

func fnHSVToRGB(a *args) (any, error) {
	if len(a.HSV) != 3 {
		return nil, fmt.Errorf("hsv_to_rgb needs a 3-element hsv")
	}
	r, g, b := mpl.HSVToRGB(a.HSV[0], a.HSV[1], a.HSV[2])
	return map[string]any{"rgb": encNums([]float64{r, g, b})}, nil
}

var handlers = map[string]func(*args) (any, error){
	"axis_ticks":      fnAxisTicks,
	"tick_labels":     fnTickLabels,
	"cat_ticks":       fnCatTicks,
	"hist_bins":       fnHistBins,
	"data_to_axes":    fnDataToAxes,
	"labels":          fnLabels,
	"save_ext":        fnSaveExt,
	"glyph_supported": fnGlyphSupported,
	"dpi_effect":      fnDPIEffect,
	"hex_parse":       fnHexParse,
	"hex_format":      fnHexFormat,
	"named_color":     fnNamedColor,
	"cmap_sample":     fnCmapSample,
	"cycle_color":     fnCycleColor,
	"normalize":       fnNormalize,
	"lognorm":         fnLogNorm,
	"rgb_to_hsv":      fnRGBToHSV,
	"hsv_to_rgb":      fnHSVToRGB,
}

// dispatch runs one case, converting a panic into an ok:false result so a bad
// case never takes the runner down.
func dispatch(req *request) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	h, ok := handlers[req.Fn]
	if !ok {
		return nil, fmt.Errorf("no handler for fn %q", req.Fn)
	}
	var a args
	if len(req.Args) > 0 {
		if e := json.Unmarshal(req.Args, &a); e != nil {
			return nil, fmt.Errorf("bad args: %v", e)
		}
	}
	return h(&a)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		res := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			res.OK, res.Error = false, "bad request: "+err.Error()
		} else {
			res.ID = req.ID
			v, err := dispatch(&req)
			if err != nil {
				res.OK, res.Error = false, err.Error()
			} else {
				res.OK, res.Value = true, v
			}
		}
		b, err := json.Marshal(res)
		if err != nil {
			b, _ = json.Marshal(response{ID: req.ID, OK: false, Error: "unencodable value: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}
