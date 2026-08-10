// Command run is the Go-side parity runner for github.com/malcolmston/sharp.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. A returned error or a panic
// inside a case is reported as {"ok": false, "error": "..."} and the runner
// keeps going, so one bad case never costs the rest of the suite.
//
// Every value emitted here has an identically-shaped counterpart in
// ../node/run.mjs. The two runners share the pixel-summary arithmetic and the
// magic-number sniffer line for line, so a difference in a reply is a
// difference in the libraries, not in the harness.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malcolmston/sharp"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

var fixturesDir = filepath.Join("..", "fixtures")

func fixture(name string) string { return filepath.Join(fixturesDir, name) }

// ------------------------------------------------------------ format sniffing

// sniff names an encoded byte stream from its magic number only. It is a
// byte-for-byte twin of sniff() in the Node runner, so "what did this entry
// point actually write" is answered without trusting either library's own
// format detector.
func sniff(b []byte) string {
	has := func(off int, want ...byte) bool {
		if len(b) < off+len(want) {
			return false
		}
		for i, v := range want {
			if b[off+i] != v {
				return false
			}
		}
		return true
	}
	switch {
	case has(0, 0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A):
		return "png"
	case has(0, 0xFF, 0xD8, 0xFF):
		return "jpeg"
	case has(0, 'G', 'I', 'F', '8'):
		return "gif"
	case has(0, 'B', 'M'):
		return "bmp"
	case has(0, 'I', 'I', 0x2A, 0x00), has(0, 'M', 'M', 0x00, 0x2A):
		return "tiff"
	case has(0, 'R', 'I', 'F', 'F') && has(8, 'W', 'E', 'B', 'P'):
		return "webp"
	case has(4, 'f', 't', 'y', 'p'):
		return "isobmff"
	default:
		return "unknown"
	}
}

// ------------------------------------------------------------------ op mapping

// colour is the sharp-style colour object used in the case files.
type colour struct {
	R     uint8    `json:"r"`
	G     uint8    `json:"g"`
	B     uint8    `json:"b"`
	Alpha *float64 `json:"alpha"`
}

func (c colour) rgba() sharp.RGBA {
	a := uint8(255)
	if c.Alpha != nil {
		a = uint8(math.Round(clamp01(*c.Alpha) * 255))
	}
	return sharp.RGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// opOptions is the union of every option key any case file uses. Keys the Go
// port has no equivalent for are still parsed, so the runner can report a
// precise "unsupported" instead of silently ignoring them.
type opOptions struct {
	Width  *int `json:"width"`
	Height *int `json:"height"`
	Left   *int `json:"left"`
	Top    *int `json:"top"`
	Right  *int `json:"right"`
	Bottom *int `json:"bottom"`

	Fit                *string `json:"fit"`
	Kernel             *string `json:"kernel"`
	Position           *string `json:"position"`
	WithoutEnlargement *bool   `json:"withoutEnlargement"`
	WithoutReduction   *bool   `json:"withoutReduction"`

	Background *colour `json:"background"`
	Colour     *colour `json:"colour"`
	ExtendWith *string `json:"extendWith"`

	Angle     *float64 `json:"angle"`
	Sigma     *float64 `json:"sigma"`
	Gamma     *float64 `json:"gamma"`
	Threshold *float64 `json:"threshold"`
	Alpha     *float64 `json:"alpha"`
	Size      *int     `json:"size"`
	Channel   *int     `json:"channel"`
	Space     *string  `json:"space"`

	A []float64 `json:"a"`
	B []float64 `json:"b"`

	Matrix   [][]float64 `json:"matrix"`
	Flat     []float64   `json:"kernelWeights"`
	Scale    *float64    `json:"scale"`
	Offset   *float64    `json:"offset"`
	Lower    *float64    `json:"lower"`
	Upper    *float64    `json:"upper"`
	MaxSlope *int        `json:"maxSlope"`

	Brightness *float64 `json:"brightness"`
	Saturation *float64 `json:"saturation"`
	Hue        *float64 `json:"hue"`
	Lightness  *float64 `json:"lightness"`

	Operand  *string `json:"operand"`
	Operator *string `json:"operator"`
	Blend    *string `json:"blend"`
	Gravity  *string `json:"gravity"`
}

func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func floatOr(p *float64, def float64) float64 {
	if p == nil {
		return def
	}
	return *p
}

// unsupported marks an upstream capability the port does not expose. It is a
// case outcome (ok:false), never a crash.
func unsupported(what string) error {
	return fmt.Errorf("go port has no equivalent for %s", what)
}

var fitByName = map[string]sharp.Fit{
	// sharp's "fill" ignores aspect ratio, which is exactly FitExact.
	"fill": sharp.FitExact,
	// sharp's "cover" scales to cover then centre-crops, which is FitCover.
	"cover": sharp.FitCover,
	// sharp's "inside" scales to fit within the box with no padding, which is
	// what the port documents FitContain as doing.
	"inside": sharp.FitContain,
	// sharp's "contain" scales to fit *and pads* to exactly width x height. The
	// port has no padding fit, so FitContain is the nearest available mapping
	// and the geometry difference that follows is a real divergence.
	"contain": sharp.FitContain,
}

var kernelByName = map[string]sharp.Interpolation{
	"nearest":  sharp.Nearest,
	"linear":   sharp.Bilinear,
	"cubic":    sharp.Cubic,
	"mitchell": sharp.Mitchell,
	"lanczos3": sharp.Lanczos3,
}

var boolByName = map[string]sharp.BoolOp{
	"and": sharp.BoolAnd,
	"or":  sharp.BoolOr,
	"eor": sharp.BoolXor,
}

var extendByName = map[string]sharp.ExtendMode{
	"background": sharp.ExtendBackground,
	"copy":       sharp.ExtendCopy,
	"repeat":     sharp.ExtendRepeat,
	"mirror":     sharp.ExtendMirror,
}

var blendByName = map[string]sharp.BlendMode{
	"over":         sharp.BlendOver,
	"multiply":     sharp.BlendMultiply,
	"screen":       sharp.BlendScreen,
	"overlay":      sharp.BlendOverlay,
	"darken":       sharp.BlendDarken,
	"lighten":      sharp.BlendLighten,
	"colour-dodge": sharp.BlendColorDodge,
	"colour-burn":  sharp.BlendColorBurn,
	"hard-light":   sharp.BlendHardLight,
	"soft-light":   sharp.BlendSoftLight,
	"difference":   sharp.BlendDifference,
	"exclusion":    sharp.BlendExclusion,
	"add":          sharp.BlendAdd,
}

var gravityByName = map[string]sharp.Gravity{
	"northwest": sharp.GravityTopLeft,
	"north":     sharp.GravityTop,
	"northeast": sharp.GravityTopRight,
	"west":      sharp.GravityLeft,
	"centre":    sharp.GravityCenter,
	"center":    sharp.GravityCenter,
	"east":      sharp.GravityRight,
	"southwest": sharp.GravityBottomLeft,
	"south":     sharp.GravityBottom,
	"southeast": sharp.GravityBottomRight,
}

func applyOp(p *sharp.Pipeline, name string, o opOptions) (*sharp.Pipeline, error) {
	switch name {
	case "resize":
		if o.WithoutEnlargement != nil {
			return nil, unsupported("resize withoutEnlargement")
		}
		if o.WithoutReduction != nil {
			return nil, unsupported("resize withoutReduction")
		}
		if o.Position != nil {
			return nil, unsupported("resize position")
		}
		opts := sharp.ResizeOptions{
			Width:  intOr(o.Width, 0),
			Height: intOr(o.Height, 0),
			// The port's zero value is FitExact + Nearest; every explicit case
			// pins both so the defaults are only ever compared by the dedicated
			// resizeDefault op.
			Fit:           sharp.FitCover,
			Interpolation: sharp.Lanczos3,
		}
		if o.Fit != nil {
			fit, ok := fitByName[*o.Fit]
			if !ok {
				return nil, unsupported("resize fit " + *o.Fit)
			}
			opts.Fit = fit
		}
		if o.Kernel != nil {
			k, ok := kernelByName[*o.Kernel]
			if !ok {
				return nil, unsupported("resize kernel " + *o.Kernel)
			}
			opts.Interpolation = k
		}
		return p.Resize(opts), nil

	case "resizeDefault":
		// Deliberately the zero-value struct: this is the case that compares
		// the two libraries' *defaults*, not an explicit configuration.
		return p.Resize(sharp.ResizeOptions{Width: intOr(o.Width, 0), Height: intOr(o.Height, 0)}), nil

	case "extract":
		return p.Extract(sharp.Rectangle{
			X: intOr(o.Left, 0), Y: intOr(o.Top, 0),
			Width: intOr(o.Width, 0), Height: intOr(o.Height, 0),
		}), nil

	case "extend":
		top, right := intOr(o.Top, 0), intOr(o.Right, 0)
		bottom, left := intOr(o.Bottom, 0), intOr(o.Left, 0)
		bg := sharp.RGBA{A: 255}
		if o.Background != nil {
			bg = o.Background.rgba()
		}
		if o.ExtendWith != nil {
			mode, ok := extendByName[*o.ExtendWith]
			if !ok {
				return nil, unsupported("extendWith " + *o.ExtendWith)
			}
			return p.ExtendWith(sharp.ExtendOptions{
				Top: top, Right: right, Bottom: bottom, Left: left,
				Background: bg, Mode: mode,
			}), nil
		}
		return p.Extend(top, right, bottom, left, bg), nil

	case "rotate":
		bg := sharp.RGBA{A: 255}
		if o.Background != nil {
			bg = o.Background.rgba()
		}
		return p.Rotate(floatOr(o.Angle, 0), bg), nil

	case "flip":
		return p.FlipVertical(), nil
	case "flop":
		return p.FlipHorizontal(), nil
	case "grayscale":
		return p.Grayscale(), nil
	case "negate":
		return p.Negate(), nil
	case "removeAlpha":
		return p.RemoveAlpha(), nil
	case "ensureAlpha":
		return p.EnsureAlpha(floatOr(o.Alpha, 1)), nil
	case "unflatten":
		return p.Unflatten(), nil

	case "flatten":
		bg := sharp.RGBA{A: 255}
		if o.Background != nil {
			bg = o.Background.rgba()
		}
		return p.Flatten(bg), nil

	case "blur":
		return p.Blur(floatOr(o.Sigma, 1)), nil
	case "sharpen":
		// sharp takes {sigma}; the port takes a unitless amount for a fixed 3x3
		// kernel. Passing sigma through is the only available mapping.
		return p.Sharpen(floatOr(o.Sigma, 1)), nil
	case "gamma":
		return p.Gamma(floatOr(o.Gamma, 2.2)), nil
	case "threshold":
		return p.Threshold(uint8(clampInt(int(floatOr(o.Threshold, 128)), 0, 255))), nil
	case "linear":
		return p.Linear(o.A, o.B), nil
	case "tint":
		if o.Colour == nil {
			return nil, fmt.Errorf("tint requires a colour")
		}
		return p.Tint(o.Colour.rgba()), nil
	case "median":
		size := intOr(o.Size, 3)
		if size%2 == 0 {
			return nil, unsupported("even median window")
		}
		return p.Median((size - 1) / 2), nil
	case "recomb":
		return p.Recomb(o.Matrix), nil
	case "modulate":
		return p.Modulate(sharp.ModulateOptions{
			Brightness: floatOr(o.Brightness, 1),
			Saturation: floatOr(o.Saturation, 1),
			Hue:        floatOr(o.Hue, 0),
			Lightness:  floatOr(o.Lightness, 0),
		}), nil
	case "extractChannel":
		return p.ExtractChannel(sharp.Channel(intOr(o.Channel, 0))), nil
	case "toColourspace":
		return p.ToColourspace(strings.TrimSpace(derefString(o.Space))), nil
	case "trim":
		return p.Trim(floatOr(o.Threshold, 10)), nil
	case "normalise":
		return p.Normalise(sharp.NormaliseOptions{
			Lower: floatOr(o.Lower, 1), Upper: floatOr(o.Upper, 99),
		}), nil
	case "clahe":
		return p.CLAHE(sharp.CLAHEOptions{
			Width: intOr(o.Width, 0), Height: intOr(o.Height, 0),
			MaxSlope: intOr(o.MaxSlope, 3),
		}), nil

	case "boolean":
		op, ok := boolByName[derefString(o.Operator)]
		if !ok {
			return nil, unsupported("boolean operator " + derefString(o.Operator))
		}
		other, err := loadImage(derefString(o.Operand))
		if err != nil {
			return nil, err
		}
		return p.Boolean(other, op), nil

	case "bandbool":
		op, ok := boolByName[derefString(o.Operator)]
		if !ok {
			return nil, unsupported("bandbool operator " + derefString(o.Operator))
		}
		return p.Bandbool(op), nil

	case "composite":
		other, err := loadImage(derefString(o.Operand))
		if err != nil {
			return nil, err
		}
		opts := sharp.CompositeOptions{Left: intOr(o.Left, 0), Top: intOr(o.Top, 0)}
		if o.Blend != nil {
			mode, ok := blendByName[*o.Blend]
			if !ok {
				return nil, unsupported("blend mode " + *o.Blend)
			}
			opts.Blend = mode
		}
		if o.Gravity != nil {
			g, ok := gravityByName[*o.Gravity]
			if !ok {
				return nil, unsupported("gravity " + *o.Gravity)
			}
			opts.UseGravity = true
			opts.Gravity = g
		}
		return p.Composite(other, opts), nil

	case "convolve":
		k, err := sharp.NewKernel(o.Flat, floatOr(o.Scale, 1), floatOr(o.Offset, 0))
		if err != nil {
			return nil, err
		}
		return p.Convolve(k), nil

	case "affine":
		if len(o.Matrix) != 2 || len(o.Matrix[0]) != 2 || len(o.Matrix[1]) != 2 {
			return nil, fmt.Errorf("affine needs a 2x2 matrix")
		}
		bg := sharp.RGBA{A: 255}
		if o.Background != nil {
			bg = o.Background.rgba()
		}
		m := [6]float64{o.Matrix[0][0], o.Matrix[0][1], 0, o.Matrix[1][0], o.Matrix[1][1], 0}
		return p.Affine(m, sharp.AffineOptions{Background: bg}), nil

	default:
		return nil, fmt.Errorf("unknown op: %s", name)
	}
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func loadImage(name string) (image.Image, error) {
	if name == "" {
		return nil, fmt.Errorf("operand fixture not given")
	}
	return sharp.FromFile(fixture(name)).ToImage()
}

// opSpec is one entry of the op list in a case: ["name", {options}].
type opSpec struct {
	Name string
	Opts opOptions
}

func (s *opSpec) UnmarshalJSON(raw []byte) error {
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return err
	}
	if len(parts) == 0 {
		return fmt.Errorf("empty op")
	}
	if err := json.Unmarshal(parts[0], &s.Name); err != nil {
		return err
	}
	if len(parts) > 1 {
		if err := json.Unmarshal(parts[1], &s.Opts); err != nil {
			return err
		}
	}
	return nil
}

func applyOps(p *sharp.Pipeline, ops []opSpec) (*sharp.Pipeline, error) {
	cur := p
	for _, op := range ops {
		next, err := applyOp(cur, op.Name, op.Opts)
		if err != nil {
			return nil, err
		}
		cur = next
		if err := cur.Err(); err != nil {
			return nil, err
		}
	}
	return cur, cur.Err()
}

// ------------------------------------------------------------------ pixel view

type rawView struct {
	pix           []byte
	width, height int
	channels      int
}

// rawRGBA materialises the pipeline as a flat 4-channel RGBA view, the same
// normalisation the Node runner applies to whatever band count libvips returns.
// channels is reported from Metadata so the port's own idea of its channel
// count is what gets compared.
func rawRGBA(p *sharp.Pipeline) (rawView, error) {
	md, err := p.Metadata()
	if err != nil {
		return rawView{}, err
	}
	pix, err := p.ToRaw()
	if err != nil {
		return rawView{}, err
	}
	return rawView{pix: pix, width: md.Width, height: md.Height, channels: md.Channels}, nil
}

const gridN = 4

type summary struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Mean   []float64 `json:"mean"`
	Min    []int     `json:"min"`
	Max    []int     `json:"max"`
	Grid   []float64 `json:"grid"`
}

// pixelSummary is the coarse, tolerance-compared description of an image:
// per-channel mean/min/max plus the mean luma of each cell of a 4x4 grid.
func pixelSummary(v rawView) summary {
	sum := make([]float64, 4)
	min := []int{255, 255, 255, 255}
	max := []int{0, 0, 0, 0}
	n := v.width * v.height
	for i := 0; i < n; i++ {
		for c := 0; c < 4; c++ {
			p := int(v.pix[i*4+c])
			sum[c] += float64(p)
			if p < min[c] {
				min[c] = p
			}
			if p > max[c] {
				max[c] = p
			}
		}
	}
	mean := make([]float64, 4)
	for c := 0; c < 4; c++ {
		if n > 0 {
			mean[c] = sum[c] / float64(n)
		}
	}
	grid := make([]float64, 0, gridN*gridN)
	for gy := 0; gy < gridN; gy++ {
		y0 := gy * v.height / gridN
		y1 := (gy + 1) * v.height / gridN
		for gx := 0; gx < gridN; gx++ {
			x0 := gx * v.width / gridN
			x1 := (gx + 1) * v.width / gridN
			acc, count := 0.0, 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					i := (y*v.width + x) * 4
					acc += 0.299*float64(v.pix[i]) + 0.587*float64(v.pix[i+1]) + 0.114*float64(v.pix[i+2])
					count++
				}
			}
			if count == 0 {
				grid = append(grid, 0)
			} else {
				grid = append(grid, acc/float64(count))
			}
		}
	}
	return summary{Width: v.width, Height: v.height, Mean: mean, Min: min, Max: max, Grid: grid}
}

// ------------------------------------------------------------------- handlers

func handle(req request) (any, error) {
	arg := func(i int, dst any) error {
		if i >= len(req.Args) {
			return fmt.Errorf("missing arg %d", i)
		}
		return json.Unmarshal(req.Args[i], dst)
	}

	switch req.Fn {
	case "metadata", "decode":
		var name string
		if err := arg(0, &name); err != nil {
			return nil, err
		}
		md, err := sharp.FromFile(fixture(name)).Metadata()
		if err != nil {
			return nil, err
		}
		if req.Fn == "decode" {
			return map[string]any{"format": string(md.Format), "width": md.Width, "height": md.Height}, nil
		}
		return map[string]any{
			"format":   string(md.Format),
			"width":    md.Width,
			"height":   md.Height,
			"channels": md.Channels,
			"hasAlpha": md.HasAlpha,
			"space":    md.Space,
		}, nil

	case "geometry", "pixels":
		var name string
		if err := arg(0, &name); err != nil {
			return nil, err
		}
		var ops []opSpec
		if len(req.Args) > 1 {
			if err := arg(1, &ops); err != nil {
				return nil, err
			}
		}
		p, err := applyOps(sharp.FromFile(fixture(name)), ops)
		if err != nil {
			return nil, err
		}
		v, err := rawRGBA(p)
		if err != nil {
			return nil, err
		}
		if req.Fn == "geometry" {
			return map[string]any{
				"width": v.width, "height": v.height,
				"channels": v.channels, "hasAlpha": v.channels == 2 || v.channels == 4,
			}, nil
		}
		return pixelSummary(v), nil

	case "encode":
		var name, format string
		if err := arg(0, &name); err != nil {
			return nil, err
		}
		if err := arg(1, &format); err != nil {
			return nil, err
		}
		quality := 0
		if len(req.Args) > 2 {
			var opts struct {
				Quality *int `json:"quality"`
			}
			if err := arg(2, &opts); err != nil {
				return nil, err
			}
			if opts.Quality != nil {
				quality = *opts.Quality
			}
		}
		buf, err := sharp.FromFile(fixture(name)).ToFormat(sharp.Format(format), quality)
		if err != nil {
			return nil, err
		}
		md, err := sharp.FromBytes(buf).Metadata()
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"sniff":    sniff(buf),
			"format":   string(md.Format),
			"width":    md.Width,
			"height":   md.Height,
			"channels": md.Channels,
		}, nil

	case "tofile":
		var name, format string
		if err := arg(0, &name); err != nil {
			return nil, err
		}
		if err := arg(1, &format); err != nil {
			return nil, err
		}
		buf, err := sharp.FromFile(fixture(name)).ToFormat(sharp.Format(format), 0)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(os.TempDir(), "parity-sharp-go-out."+format)
		if err := sharp.FromFile(fixture(name)).ToFile(target, sharp.Format(format), 0); err != nil {
			return nil, err
		}
		onDisk, err := os.ReadFile(target)
		_ = os.Remove(target)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"buffer": sniff(buf),
			"file":   sniff(onDisk),
			"agree":  sniff(buf) == sniff(onDisk),
		}, nil

	case "outputFormats":
		var names []string
		if err := arg(0, &names); err != nil {
			return nil, err
		}
		out := []string{}
		for _, n := range names {
			if _, err := sharp.FromFile(fixture("gradient.png")).ToFormat(sharp.Format(n), 0); err == nil {
				out = append(out, n)
			}
		}
		sort.Strings(out)
		return out, nil

	case "inputFormats":
		var names []string
		if err := arg(0, &names); err != nil {
			return nil, err
		}
		out := []string{}
		for _, n := range names {
			if _, err := sharp.FromFile(fixture(n)).Metadata(); err == nil {
				out = append(out, n)
			}
		}
		sort.Strings(out)
		return out, nil

	case "rawRoundTrip":
		var name string
		if err := arg(0, &name); err != nil {
			return nil, err
		}
		p := sharp.FromFile(fixture(name))
		md, err := p.Metadata()
		if err != nil {
			return nil, err
		}
		data, err := p.ToRaw()
		if err != nil {
			return nil, err
		}
		back, err := sharp.FromRaw(data, md.Width, md.Height, 4).ToRaw()
		if err != nil {
			return nil, err
		}
		channels := 0
		if px := md.Width * md.Height; px > 0 {
			channels = len(data) / px
		}
		return map[string]any{
			"bytes":     len(data),
			"channels":  channels,
			"identical": bytes.Equal(data, back),
		}, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

// safeHandle turns a panic anywhere in the port into a failing case rather than
// a dead runner.
func safeHandle(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			value, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return handle(req)
}

func main() {
	if dir := os.Getenv("PARITY_FIXTURES"); dir != "" {
		fixturesDir = dir
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		resp := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp = response{OK: false, Error: "bad request: " + err.Error()}
		} else {
			resp.ID = req.ID
			value, err := safeHandle(req)
			if err != nil {
				resp.OK = false
				resp.Error = firstLine(err.Error())
			} else {
				resp.OK = true
				resp.Value = value
			}
		}
		enc, err := json.Marshal(resp)
		if err != nil {
			enc, _ = json.Marshal(response{ID: req.ID, OK: false, Error: "encode: " + err.Error()})
		}
		out.Write(enc)
		out.WriteByte('\n')
		if err := out.Flush(); err != nil {
			return
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
