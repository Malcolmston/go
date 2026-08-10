// Command run is the Go-side parity runner for github.com/malcolmston/opencv.
//
// It speaks JSON Lines on stdio: one request object per line in, one response
// object per line out, in the same order. See ../../HARNESS.md.
//
// Every op is invoked inside a recover() so a panic from the library becomes
// {"ok":false,...} rather than killing the runner — several of the port's
// entry points panic where cv2 returns a value, and those are exactly the
// divergences this harness exists to find.
//
// Inputs are never read from disk: each image is generated analytically from
// the generator spec carried in the case file, with the same closed-form rules
// (and the same case-supplied LCG seed) as python/run.py.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	cv "github.com/malcolmston/opencv"
	"github.com/malcolmston/opencv/plot"
	"github.com/malcolmston/opencv/quality"
	"github.com/malcolmston/opencv/segmentation"
)

// ------------------------------------------------------------- enum tables

var borderTypes = map[string]cv.BorderType{
	"BorderConstant":   cv.BorderConstant,
	"BorderReplicate":  cv.BorderReplicate,
	"BorderReflect":    cv.BorderReflect,
	"BorderWrap":       cv.BorderWrap,
	"BorderReflect101": cv.BorderReflect101,
}

var threshTypes = map[string]cv.ThresholdType{
	"ThreshBinary":    cv.ThreshBinary,
	"ThreshBinaryInv": cv.ThreshBinaryInv,
	"ThreshTrunc":     cv.ThreshTrunc,
	"ThreshToZero":    cv.ThreshToZero,
	"ThreshToZeroInv": cv.ThreshToZeroInv,
}

var adaptiveMethods = map[string]cv.AdaptiveMethod{
	"AdaptiveThreshMeanC":     cv.AdaptiveThreshMeanC,
	"AdaptiveThreshGaussianC": cv.AdaptiveThreshGaussianC,
}

var morphShapes = map[string]cv.MorphShape{
	"MorphRect":    cv.MorphRect,
	"MorphCross":   cv.MorphCross,
	"MorphEllipse": cv.MorphEllipse,
}

var morphOps = map[string]cv.MorphType{
	"MorphErode":    cv.MorphErode,
	"MorphDilate":   cv.MorphDilate,
	"MorphOpen":     cv.MorphOpen,
	"MorphClose":    cv.MorphClose,
	"MorphGradient": cv.MorphGradient,
	"MorphTophat":   cv.MorphTophat,
	"MorphBlackhat": cv.MorphBlackhat,
}

var retrModes = map[string]cv.RetrievalMode{
	"RetrExternal": cv.RetrExternal,
	"RetrList":     cv.RetrList,
	"RetrTree":     cv.RetrTree,
}

var chainModes = map[string]cv.ContourApproxMode{
	"ChainApproxNone":   cv.ChainApproxNone,
	"ChainApproxSimple": cv.ChainApproxSimple,
}

var interps = map[string]cv.InterpolationFlag{
	"InterNearest": cv.InterNearest,
	"InterLinear":  cv.InterLinear,
}

var tmModes = map[string]cv.TemplateMatchMode{
	"TmSqdiff":       cv.TmSqdiff,
	"TmSqdiffNormed": cv.TmSqdiffNormed,
	"TmCcoeff":       cv.TmCcoeff,
	"TmCcoeffNormed": cv.TmCcoeffNormed,
}

var histCmp = map[string]cv.HistCompMethod{
	"HistCmpCorrel":        cv.HistCmpCorrel,
	"HistCmpChiSqr":        cv.HistCmpChiSqr,
	"HistCmpIntersect":     cv.HistCmpIntersect,
	"HistCmpBhattacharyya": cv.HistCmpBhattacharyya,
}

var colorCodes = map[string]cv.ColorConversionCode{
	"ColorRGB2Gray":  cv.ColorRGB2Gray,
	"ColorGray2RGB":  cv.ColorGray2RGB,
	"ColorRGB2BGR":   cv.ColorRGB2BGR,
	"ColorBGR2RGB":   cv.ColorBGR2RGB,
	"ColorBGR2Gray":  cv.ColorBGR2Gray,
	"ColorRGB2HSV":   cv.ColorRGB2HSV,
	"ColorHSV2RGB":   cv.ColorHSV2RGB,
	"ColorRGB2Lab":   cv.ColorRGB2Lab,
	"ColorLab2RGB":   cv.ColorLab2RGB,
	"ColorRGB2YCrCb": cv.ColorRGB2YCrCb,
	"ColorYCrCb2RGB": cv.ColorYCrCb2RGB,
	"ColorRGB2HLS":   cv.ColorRGB2HLS,
	"ColorHLS2RGB":   cv.ColorHLS2RGB,
}

var flipCodes = map[string]cv.FlipCode{
	"FlipVertical":   cv.FlipVertical,
	"FlipHorizontal": cv.FlipHorizontal,
	"FlipBoth":       cv.FlipBoth,
}

var rotateCodes = map[string]cv.RotateCode{
	"Rotate90CW":  cv.Rotate90CW,
	"Rotate180":   cv.Rotate180,
	"Rotate90CCW": cv.Rotate90CCW,
}

var reduceOps = map[string]cv.ReduceOp{
	"ReduceSum": cv.ReduceSum,
	"ReduceAvg": cv.ReduceAvg,
	"ReduceMax": cv.ReduceMax,
	"ReduceMin": cv.ReduceMin,
}

var bayer = map[string]cv.BayerPattern{
	"BayerRG": cv.BayerRG, "BayerGR": cv.BayerGR,
	"BayerBG": cv.BayerBG, "BayerGB": cv.BayerGB,
}

var warpPolarModes = map[string]cv.WarpPolarMode{
	"WarpPolarLinear": cv.WarpPolarLinear,
	"WarpPolarLog":    cv.WarpPolarLog,
}

var colormaps = map[string]plot.Colormap{
	"ColormapJet": plot.ColormapJet, "ColormapHot": plot.ColormapHot,
	"ColormapCool": plot.ColormapCool, "ColormapBone": plot.ColormapBone,
	"ColormapHSV": plot.ColormapHSV, "ColormapViridis": plot.ColormapViridis,
	"ColormapPlasma": plot.ColormapPlasma, "ColormapGrayscale": plot.ColormapGrayscale,
	"ColormapAutumn": plot.ColormapAutumn, "ColormapWinter": plot.ColormapWinter,
	"ColormapSummer": plot.ColormapSummer, "ColormapSpring": plot.ColormapSpring,
	"ColormapOcean": plot.ColormapOcean, "ColormapRainbow": plot.ColormapRainbow,
	"ColormapPink": plot.ColormapPink, "ColormapParula": plot.ColormapParula,
	"ColormapMagma": plot.ColormapMagma, "ColormapInferno": plot.ColormapInferno,
	"ColormapCividis": plot.ColormapCividis, "ColormapTwilight": plot.ColormapTwilight,
	"ColormapTurbo": plot.ColormapTurbo,
}

func lookup[T any](table map[string]T, name string) T {
	v, ok := table[name]
	if !ok {
		panic(fmt.Sprintf("unknown enum constant: %q", name))
	}
	return v
}

// ------------------------------------------------------------- generators

// truncDiv is integer division truncating toward zero, matching Python's
// trunc_div helper in python/run.py.
func truncDiv(a, b int) int { return a / b }

func lcg(seed, n int) []int {
	const m = 1 << 31
	state := seed % m
	out := make([]int, n)
	for i := range out {
		state = (state*1103515245 + 12345) % m
		out[i] = (state >> 16) & 255
	}
	return out
}

type disc struct{ cx, cy, r, val int }

var discs = []disc{{8, 8, 5, 255}, {24, 10, 6, 180}, {14, 26, 4, 120}, {36, 36, 7, 200}}

type rectSpec struct{ x0, y0, x1, y1, val int }

var rects = []rectSpec{{28, 20, 40, 30, 220}}

var blobRects = [][4]int{{2, 2, 6, 6}, {10, 3, 16, 9}, {20, 20, 22, 22},
	{4, 25, 9, 30}, {25, 5, 27, 7}, {28, 8, 30, 10}}

var circleSpecs = [][3]int{{12, 12, 6}, {32, 14, 8}, {20, 34, 5}}

func spread(v, ch int) []int {
	if ch == 1 {
		return []int{v}
	}
	out := make([]int, ch)
	for c := range out {
		out[c] = truncDiv(v*(10-c), 10)
	}
	return out
}

type genSpec struct {
	Gen   string    `json:"gen"`
	FGen  string    `json:"fgen"`
	Rows  *int      `json:"rows"`
	Cols  *int      `json:"cols"`
	Ch    *int      `json:"ch"`
	Block *int      `json:"block"`
	Seed  *int      `json:"seed"`
	Amp   *int      `json:"amp"`
	Value *int      `json:"value"`
	D     *float64  `json:"d"`
	Data  []float64 `json:"data"`
}

func ival(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func genImage(s genSpec) *cv.Mat {
	rows, cols, ch := ival(s.Rows, 32), ival(s.Cols, 32), ival(s.Ch, 1)
	m := cv.NewMat(rows, cols, ch)
	set := func(y, x, c, v int) { m.Set(y, x, c, uint8(v)) }

	switch s.Gen {
	case "gradient":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				for c := 0; c < ch; c++ {
					set(y, x, c, (x*7+y*5+c*37)%256)
				}
			}
		}
	case "const":
		v := ival(s.Value, 0)
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				for c := 0; c < ch; c++ {
					set(y, x, c, (v+c*20)%256)
				}
			}
		}
	case "checker":
		b := ival(s.Block, 4)
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				on := ((x/b)+(y/b))%2 == 0
				for c := 0; c < ch; c++ {
					if on {
						set(y, x, c, (200+c*15)%256)
					} else {
						set(y, x, c, (30+c*5)%256)
					}
				}
			}
		}
	case "noise":
		vals := lcg(ival(s.Seed, 1), rows*cols*ch)
		i := 0
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				for c := 0; c < ch; c++ {
					set(y, x, c, vals[i])
					i++
				}
			}
		}
	case "gradnoise":
		amp := ival(s.Amp, 40)
		vals := lcg(ival(s.Seed, 1), rows*cols*ch)
		i := 0
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				for c := 0; c < ch; c++ {
					g := (x*7 + y*5 + c*37) % 256
					d := truncDiv((vals[i]-128)*amp, 128)
					v := g + d
					if v < 0 {
						v = 0
					}
					if v > 255 {
						v = 255
					}
					set(y, x, c, v)
					i++
				}
			}
		}
	case "shapes":
		for _, d := range discs {
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					if (x-d.cx)*(x-d.cx)+(y-d.cy)*(y-d.cy) <= d.r*d.r {
						for c, v := range spread(d.val, ch) {
							set(y, x, c, v)
						}
					}
				}
			}
		}
		for _, r := range rects {
			for y := max(0, r.y0); y < min(rows, r.y1+1); y++ {
				for x := max(0, r.x0); x < min(cols, r.x1+1); x++ {
					for c, v := range spread(r.val, ch) {
						set(y, x, c, v)
					}
				}
			}
		}
	case "blobs":
		for _, r := range blobRects {
			for y := max(0, r[1]); y < min(rows, r[3]+1); y++ {
				for x := max(0, r[0]); x < min(cols, r[2]+1); x++ {
					for c := 0; c < ch; c++ {
						set(y, x, c, 255)
					}
				}
			}
		}
	case "lines":
		if 10 < rows {
			for x := 0; x < cols; x++ {
				for c := 0; c < ch; c++ {
					set(10, x, c, 255)
				}
			}
		}
		if 20 < cols {
			for y := 0; y < rows; y++ {
				for c := 0; c < ch; c++ {
					set(y, 20, c, 255)
				}
			}
		}
		for d := 5; d < 36; d++ {
			if d < rows && d < cols {
				for c := 0; c < ch; c++ {
					set(d, d, c, 255)
				}
			}
		}
	case "circles":
		for _, cc := range circleSpecs {
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					if (x-cc[0])*(x-cc[0])+(y-cc[1])*(y-cc[1]) <= cc[2]*cc[2] {
						for c := 0; c < ch; c++ {
							set(y, x, c, 255)
						}
					}
				}
			}
		}
	default:
		panic("unknown image generator: " + s.Gen)
	}
	return m
}

func genFMat(s genSpec) *cv.FloatMat {
	rows, cols := ival(s.Rows, 32), ival(s.Cols, 32)
	f := cv.NewFloatMat(rows, cols)
	put := func(y, x int, v float64) { f.Data[y*cols+x] = v }
	switch s.FGen {
	case "mapx-flip":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(cols-1-x))
			}
		}
	case "mapy-id":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(y))
			}
		}
	case "mapx-id":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(x))
			}
		}
	case "mapx-shift":
		d := 2.5
		if s.D != nil {
			d = *s.D
		}
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(x)+d)
			}
		}
	case "mapx-wobble":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(x)+float64((y%8)-4)/4.0)
			}
		}
	case "mapy-wobble":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, float64(y)+float64((x%8)-4)/4.0)
			}
		}
	case "literal":
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				put(y, x, s.Data[y*cols+x])
			}
		}
	default:
		panic("unknown float generator: " + s.FGen)
	}
	return f
}

// ---------------------------------------------------------------- encoding

func encNum(x float64) any {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	return x
}

func encMat(m *cv.Mat) any {
	return map[string]any{"kind": "mat", "rows": m.Rows, "cols": m.Cols,
		"channels": m.Channels, "hex": hex.EncodeToString(m.Data)}
}

func encFMat(f *cv.FloatMat) any {
	data := make([]any, len(f.Data))
	for i, v := range f.Data {
		data[i] = encNum(v)
	}
	return map[string]any{"kind": "fmat", "rows": f.Rows, "cols": f.Cols, "data": data}
}

func encFloats(xs []float64) []any {
	out := make([]any, len(xs))
	for i, v := range xs {
		out[i] = encNum(v)
	}
	return out
}

func encGrid(rows, cols int, data []float64) any {
	return encFMat(&cv.FloatMat{Rows: rows, Cols: cols, Data: data})
}

func encPoints(pts []cv.Point) [][]int {
	out := make([][]int, len(pts))
	for i, p := range pts {
		out[i] = []int{p.X, p.Y}
	}
	return out
}

func lessIntSlice(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func sortPoints(pts [][]int) [][]int {
	sort.Slice(pts, func(i, j int) bool { return lessIntSlice(pts[i], pts[j]) })
	return pts
}

func lessFloatSlice(a, b []float64) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// -------------------------------------------------------------- arg access

type ctx struct{ args []json.RawMessage }

func (c *ctx) raw(i int) json.RawMessage {
	if i >= len(c.args) {
		panic(fmt.Sprintf("missing argument %d", i))
	}
	return c.args[i]
}

func (c *ctx) into(i int, dst any) {
	if err := json.Unmarshal(c.raw(i), dst); err != nil {
		panic(fmt.Sprintf("argument %d: %v", i, err))
	}
}

func (c *ctx) mat(i int) *cv.Mat {
	var s genSpec
	c.into(i, &s)
	if s.Gen == "" {
		panic(fmt.Sprintf("argument %d is not an image generator", i))
	}
	return genImage(s)
}

func (c *ctx) fmat(i int) *cv.FloatMat {
	var s genSpec
	c.into(i, &s)
	if s.FGen == "" {
		panic(fmt.Sprintf("argument %d is not a float generator", i))
	}
	return genFMat(s)
}

func (c *ctx) i(idx int) int        { var v int; c.into(idx, &v); return v }
func (c *ctx) f(idx int) float64    { var v float64; c.into(idx, &v); return v }
func (c *ctx) s(idx int) string     { var v string; c.into(idx, &v); return v }
func (c *ctx) b(idx int) bool       { var v bool; c.into(idx, &v); return v }
func (c *ctx) ints(idx int) []int   { var v []int; c.into(idx, &v); return v }
func (c *ctx) fs(idx int) []float64 { var v []float64; c.into(idx, &v); return v }

func (c *ctx) bytes(idx int) []uint8 {
	var v []uint8
	c.into(idx, &v)
	return v
}

func (c *ctx) points(idx int) []cv.Point {
	var raw [][]int
	c.into(idx, &raw)
	out := make([]cv.Point, len(raw))
	for i, p := range raw {
		out[i] = cv.Point{X: p[0], Y: p[1]}
	}
	return out
}

func (c *ctx) points2f(idx int) []cv.Point2f {
	var raw [][]float64
	c.into(idx, &raw)
	out := make([]cv.Point2f, len(raw))
	for i, p := range raw {
		out[i] = cv.Point2f{X: p[0], Y: p[1]}
	}
	return out
}

func (c *ctx) scalar(idx int) cv.Scalar {
	vals := c.fs(idx)
	return cv.NewScalar(vals...)
}

func matFromFloats(rows, cols int, data []float64) *cv.FloatMat {
	if len(data) != rows*cols {
		panic("matrix data length does not match shape")
	}
	f := cv.NewFloatMat(rows, cols)
	copy(f.Data, data)
	return f
}

// ------------------------------------------------------------------- ops

type opFunc func(*ctx) any

var ops = map[string]opFunc{}

func reg(name string, f opFunc) { ops[name] = f }

func gray(m *cv.Mat) *cv.Mat {
	if m.Channels != 1 {
		panic("expected single-channel image")
	}
	return m
}

func init() {
	// ---- mat semantics ------------------------------------------------
	reg("matInfo", func(c *ctx) any {
		m := c.mat(0)
		r, cc := m.Size()
		return map[string]any{"rows": r, "cols": cc, "channels": m.Channels,
			"total": m.Total(), "empty": m.Empty()}
	})
	reg("newMat", func(c *ctx) any { return encMat(cv.NewMat(c.i(0), c.i(1), c.i(2))) })
	reg("matAt", func(c *ctx) any { return c.mat(0).At(c.i(1), c.i(2), c.i(3)) })
	reg("matSet", func(c *ctx) any {
		m := c.mat(0)
		m.Set(c.i(1), c.i(2), c.i(3), uint8(c.i(4)))
		return encMat(m)
	})
	reg("matSetPixel", func(c *ctx) any {
		m := c.mat(0)
		m.SetPixel(c.i(1), c.i(2), c.bytes(3))
		return encMat(m)
	})
	reg("matPixel", func(c *ctx) any {
		px := c.mat(0).AtPixel(c.i(1), c.i(2))
		out := make([]int, len(px))
		for i, v := range px {
			out[i] = int(v)
		}
		return out
	})
	reg("matRegion", func(c *ctx) any {
		return encMat(c.mat(0).Region(c.i(1), c.i(2), c.i(3), c.i(4)))
	})
	reg("matRegionWrites", func(c *ctx) any {
		m := c.mat(0)
		roi := m.Region(c.i(1), c.i(2), c.i(3), c.i(4))
		roi.SetTo(uint8(c.i(5)))
		return encMat(m)
	})
	reg("matClone", func(c *ctx) any { return encMat(c.mat(0).Clone()) })
	reg("matCloneIsolated", func(c *ctx) any {
		m := c.mat(0)
		cl := m.Clone()
		cl.SetTo(7)
		return encMat(m)
	})
	reg("matCopyTo", func(c *ctx) any {
		src, dst := c.mat(0), c.mat(1)
		src.CopyTo(dst, c.i(2), c.i(3))
		return encMat(dst)
	})
	reg("matSetTo", func(c *ctx) any {
		m := c.mat(0)
		m.SetTo(uint8(c.i(1)))
		return encMat(m)
	})
	reg("matSplit", func(c *ctx) any {
		planes := c.mat(0).Split()
		out := make([]any, len(planes))
		for i, p := range planes {
			out[i] = encMat(p)
		}
		return out
	})
	reg("matSplitMerge", func(c *ctx) any { return encMat(cv.Merge(c.mat(0).Split())) })
	reg("mergeReversed", func(c *ctx) any {
		p := c.mat(0).Split()
		for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
			p[i], p[j] = p[j], p[i]
		}
		return encMat(cv.Merge(p))
	})
	reg("extractChannel", func(c *ctx) any { return encMat(cv.ExtractChannel(c.mat(0), c.i(1))) })
	reg("insertChannel", func(c *ctx) any {
		src, dst := c.mat(0), c.mat(1)
		cv.InsertChannel(src, dst, c.i(2))
		return encMat(dst)
	})
	reg("mixChannels", func(c *ctx) any {
		src, dst := c.mat(0), c.mat(1)
		var pairs [][2]int
		c.into(2, &pairs)
		cv.MixChannels([]*cv.Mat{src}, []*cv.Mat{dst}, pairs)
		return encMat(dst)
	})
	reg("transpose", func(c *ctx) any { return encMat(cv.Transpose(c.mat(0))) })
	reg("flip", func(c *ctx) any { return encMat(cv.Flip(c.mat(0), lookup(flipCodes, c.s(1)))) })
	reg("rotate", func(c *ctx) any { return encMat(cv.Rotate(c.mat(0), lookup(rotateCodes, c.s(1)))) })
	reg("copyMakeBorder", func(c *ctx) any {
		return encMat(cv.CopyMakeBorder(c.mat(0), c.i(1), c.i(2), c.i(3), c.i(4),
			lookup(borderTypes, c.s(5)), c.scalar(6)))
	})
	reg("borderInterpolate", func(c *ctx) any {
		return cv.BorderInterpolate(c.i(0), c.i(1), lookup(borderTypes, c.s(2)))
	})

	// ---- arithmetic ---------------------------------------------------
	reg("add", func(c *ctx) any { return encMat(cv.Add(c.mat(0), c.mat(1))) })
	reg("subtract", func(c *ctx) any { return encMat(cv.Subtract(c.mat(0), c.mat(1))) })
	reg("absdiff", func(c *ctx) any { return encMat(cv.AbsDiff(c.mat(0), c.mat(1))) })
	reg("multiply", func(c *ctx) any { return encMat(cv.Multiply(c.mat(0), c.mat(1), c.f(2))) })
	reg("divide", func(c *ctx) any { return encMat(cv.Divide(c.mat(0), c.mat(1), c.f(2))) })
	reg("addWeighted", func(c *ctx) any {
		return encMat(cv.AddWeighted(c.mat(0), c.f(1), c.mat(2), c.f(3), c.f(4)))
	})
	reg("bitwiseAnd", func(c *ctx) any { return encMat(cv.BitwiseAnd(c.mat(0), c.mat(1))) })
	reg("bitwiseOr", func(c *ctx) any { return encMat(cv.BitwiseOr(c.mat(0), c.mat(1))) })
	reg("bitwiseXor", func(c *ctx) any { return encMat(cv.BitwiseXor(c.mat(0), c.mat(1))) })
	reg("bitwiseNot", func(c *ctx) any { return encMat(cv.BitwiseNot(c.mat(0))) })
	reg("minMat", func(c *ctx) any { return encMat(cv.Min(c.mat(0), c.mat(1))) })
	reg("maxMat", func(c *ctx) any { return encMat(cv.Max(c.mat(0), c.mat(1))) })
	reg("inRange", func(c *ctx) any { return encMat(cv.InRange(c.mat(0), c.bytes(1), c.bytes(2))) })
	reg("convertScaleAbs", func(c *ctx) any {
		return encMat(cv.ConvertScaleAbs(c.mat(0), c.f(1), c.f(2)))
	})
	reg("normalizeMinMax", func(c *ctx) any { return encMat(cv.Normalize(c.mat(0), c.f(1), c.f(2))) })
	reg("countNonZero", func(c *ctx) any { return cv.CountNonZero(gray(c.mat(0))) })
	reg("findNonZero", func(c *ctx) any { return sortPoints(encPoints(cv.FindNonZero(gray(c.mat(0))))) })
	reg("mean", func(c *ctx) any { s := cv.Mean(c.mat(0)); return encFloats(s[:]) })
	reg("meanStdDev", func(c *ctx) any {
		m, s := cv.MeanStdDev(c.mat(0))
		return map[string]any{"mean": encFloats(m[:]), "stddev": encFloats(s[:])}
	})
	reg("sumElems", func(c *ctx) any { s := cv.SumElems(c.mat(0)); return encFloats(s[:]) })
	reg("norm", func(c *ctx) any {
		m := c.mat(0)
		switch c.s(1) {
		case "L1":
			return encNum(cv.NormL1Mat(m))
		case "L2":
			return encNum(cv.NormL2Mat(m))
		case "Inf":
			return encNum(cv.NormInfMat(m))
		}
		panic("unknown norm kind")
	})
	reg("lut", func(c *ctx) any { return encMat(cv.LUT(c.mat(0), c.bytes(1))) })
	reg("lutChannels", func(c *ctx) any {
		var tables [][]uint8
		c.into(1, &tables)
		return encMat(cv.LUTChannels(c.mat(0), tables))
	})

	// ---- colour -------------------------------------------------------
	reg("cvtColor", func(c *ctx) any {
		return encMat(cv.CvtColor(c.mat(0), lookup(colorCodes, c.s(1))))
	})
	reg("rgbToXYZ", func(c *ctx) any { return encMat(cv.RGBToXYZ(c.mat(0))) })
	reg("xyzToRGB", func(c *ctx) any { return encMat(cv.XYZToRGB(c.mat(0))) })
	reg("rgbToYUV", func(c *ctx) any { return encMat(cv.RGBToYUV(c.mat(0))) })
	reg("yuvToRGB", func(c *ctx) any { return encMat(cv.YUVToRGB(c.mat(0))) })
	reg("rgbToHSVFull", func(c *ctx) any { return encMat(cv.RGBToHSVFull(c.mat(0))) })
	reg("hsvFullToRGB", func(c *ctx) any { return encMat(cv.HSVFullToRGB(c.mat(0))) })
	reg("rgbToGray601", func(c *ctx) any { return encMat(cv.RGBToGray601(c.mat(0))) })
	reg("demosaic", func(c *ctx) any {
		return encMat(cv.Demosaic(gray(c.mat(0)), lookup(bayer, c.s(1))))
	})

	// ---- threshold ----------------------------------------------------
	reg("threshold", func(c *ctx) any {
		dst, t := cv.Threshold(c.mat(0), c.f(1), c.f(2), lookup(threshTypes, c.s(3)))
		return map[string]any{"thresh": encNum(t), "dst": encMat(dst)}
	})
	reg("thresholdOtsu", func(c *ctx) any {
		dst, t := cv.Threshold(gray(c.mat(0)), 0, c.f(1),
			lookup(threshTypes, c.s(2))|cv.ThreshOtsu)
		return map[string]any{"thresh": encNum(t), "dst": encMat(dst)}
	})
	reg("thresholdTriangle", func(c *ctx) any {
		dst, t := cv.TriangleThreshold(gray(c.mat(0)))
		return map[string]any{"thresh": encNum(t), "dst": encMat(dst)}
	})
	reg("adaptiveThreshold", func(c *ctx) any {
		return encMat(cv.AdaptiveThreshold(gray(c.mat(0)), c.f(1),
			lookup(adaptiveMethods, c.s(2)), lookup(threshTypes, c.s(3)), c.i(4), c.f(5)))
	})

	// ---- filters ------------------------------------------------------
	reg("blur", func(c *ctx) any { return encMat(cv.Blur(c.mat(0), c.i(1))) })
	reg("boxFilter", func(c *ctx) any { return encMat(cv.BoxFilter(c.mat(0), c.i(1), c.b(2))) })
	reg("gaussianBlur", func(c *ctx) any { return encMat(cv.GaussianBlur(c.mat(0), c.i(1), c.f(2))) })
	reg("medianBlur", func(c *ctx) any { return encMat(cv.MedianBlur(c.mat(0), c.i(1))) })
	reg("bilateralFilter", func(c *ctx) any {
		return encMat(cv.BilateralFilter(c.mat(0), c.i(1), c.f(2), c.f(3)))
	})
	reg("filter2D", func(c *ctx) any {
		k := cv.NewKernel(c.i(1), c.i(2), c.fs(3))
		return encMat(cv.Filter2D(c.mat(0), k, c.f(4)))
	})
	reg("sepFilter2D", func(c *ctx) any {
		return encMat(cv.Filter2DSep(c.mat(0), c.fs(1), c.fs(2), c.f(3)))
	})
	reg("sqrBoxFilter", func(c *ctx) any {
		return encFMat(cv.SqrBoxFilter(c.mat(0), c.i(1), c.b(2)))
	})
	reg("stackBlur", func(c *ctx) any { return encMat(cv.StackBlur(c.mat(0), c.i(1))) })
	// The *Reflect101 ops exist only so the upstream runner can be asked for
	// cv2's default border; the Go side is the same port call as above.
	reg("blurReflect101", func(c *ctx) any { return encMat(cv.Blur(c.mat(0), c.i(1))) })
	reg("gaussianBlurReflect101", func(c *ctx) any {
		return encMat(cv.GaussianBlur(c.mat(0), c.i(1), c.f(2)))
	})
	reg("sobelReflect101", func(c *ctx) any {
		return encMat(cv.Sobel(c.mat(0), c.i(1), c.i(2), c.i(3), c.f(4), c.f(5)))
	})
	reg("laplacianReflect101", func(c *ctx) any {
		return encMat(cv.Laplacian(c.mat(0), c.i(1), c.f(2), c.f(3)))
	})
	reg("pyrDown", func(c *ctx) any { return encMat(cv.PyrDown(c.mat(0))) })
	reg("pyrUp", func(c *ctx) any { return encMat(cv.PyrUp(c.mat(0))) })
	reg("pyrDownTwice", func(c *ctx) any { return encMat(cv.PyrDown(cv.PyrDown(c.mat(0)))) })
	reg("pyrDownUp", func(c *ctx) any { return encMat(cv.PyrUp(cv.PyrDown(c.mat(0)))) })

	// ---- edges --------------------------------------------------------
	reg("sobel", func(c *ctx) any {
		return encMat(cv.Sobel(c.mat(0), c.i(1), c.i(2), c.i(3), c.f(4), c.f(5)))
	})
	reg("sobelFloat", func(c *ctx) any {
		src := gray(c.mat(0))
		// cv.SobelFloat returns one flat plane per channel, not one slice per
		// row, so channel 0 is reshaped back to the image geometry here.
		g := cv.SobelFloat(src, c.i(1), c.i(2), c.i(3))
		if len(g) == 0 {
			panic("cv.SobelFloat returned no planes")
		}
		return encGrid(src.Rows, src.Cols, g[0])
	})
	reg("scharr", func(c *ctx) any {
		return encMat(cv.Scharr(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4)))
	})
	reg("laplacian", func(c *ctx) any {
		return encMat(cv.Laplacian(c.mat(0), c.i(1), c.f(2), c.f(3)))
	})
	reg("canny", func(c *ctx) any { return encMat(cv.Canny(gray(c.mat(0)), c.f(1), c.f(2))) })
	reg("spatialGradient", func(c *ctx) any {
		dx, dy := cv.SpatialGradient(gray(c.mat(0)))
		return map[string]any{"dx": encFMat(dx), "dy": encFMat(dy)}
	})
	reg("cornerHarris", func(c *ctx) any {
		return encFMat(cv.CornerHarris(gray(c.mat(0)), c.i(1), c.i(2), c.f(3)))
	})
	reg("cornerMinEigenVal", func(c *ctx) any {
		return encFMat(cv.CornerMinEigenVal(gray(c.mat(0)), c.i(1), c.i(2)))
	})
	reg("preCornerDetect", func(c *ctx) any { return encFMat(cv.PreCornerDetect(gray(c.mat(0)))) })
	reg("getDerivKernels", func(c *ctx) any {
		kx, ky := cv.GetDerivKernels(c.i(0), c.i(1), c.i(2))
		return map[string]any{"kx": encFloats(kx), "ky": encFloats(ky)}
	})
	reg("getGaussianKernel", func(c *ctx) any {
		return encFloats(cv.GetGaussianKernel(c.i(0), c.f(1)))
	})
	reg("getGaborKernel", func(c *ctx) any {
		return encFMat(cv.GetGaborKernel(c.i(0), c.f(1), c.f(2), c.f(3), c.f(4), c.f(5)))
	})
	reg("integral", func(c *ctx) any { return encFMat(cv.Integral(gray(c.mat(0)))) })
	reg("integralSquared", func(c *ctx) any { return encFMat(cv.IntegralSquared(gray(c.mat(0)))) })
	reg("distanceTransform", func(c *ctx) any {
		return encFMat(cv.DistanceTransform(gray(c.mat(0))))
	})
	reg("floodFill", func(c *ctx) any {
		dst, n := cv.FloodFill(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4), c.f(5))
		return map[string]any{"count": n, "dst": encMat(dst)}
	})

	// ---- morphology ---------------------------------------------------
	reg("getStructuringElement", func(c *ctx) any {
		return encMat(cv.GetStructuringElement(lookup(morphShapes, c.s(0)), c.i(1), c.i(2)))
	})
	reg("erode", func(c *ctx) any {
		k := cv.GetStructuringElement(lookup(morphShapes, c.s(1)), c.i(2), c.i(3))
		return encMat(cv.Erode(c.mat(0), k, c.i(4)))
	})
	reg("dilate", func(c *ctx) any {
		k := cv.GetStructuringElement(lookup(morphShapes, c.s(1)), c.i(2), c.i(3))
		return encMat(cv.Dilate(c.mat(0), k, c.i(4)))
	})
	reg("morphologyEx", func(c *ctx) any {
		k := cv.GetStructuringElement(lookup(morphShapes, c.s(2)), c.i(3), c.i(4))
		return encMat(cv.MorphologyEx(c.mat(0), k, lookup(morphOps, c.s(1)), c.i(5)))
	})

	// ---- geometric ----------------------------------------------------
	reg("getRotationMatrix2D", func(c *ctx) any {
		m := cv.GetRotationMatrix2D(c.f(0), c.f(1), c.f(2), c.f(3))
		return encFloats(m[:])
	})
	reg("getAffineTransform", func(c *ctx) any {
		src, dst := c.points2f(0), c.points2f(1)
		if len(src) != 3 || len(dst) != 3 {
			panic("getAffineTransform needs three point pairs")
		}
		m := cv.GetAffineTransform([3]cv.Point2f{src[0], src[1], src[2]},
			[3]cv.Point2f{dst[0], dst[1], dst[2]})
		return encFloats(m[:])
	})
	reg("invertAffineTransform", func(c *ctx) any {
		vals := c.fs(0)
		var m cv.AffineMatrix
		copy(m[:], vals)
		out := cv.InvertAffineTransform(m)
		return encFloats(out[:])
	})
	reg("getPerspectiveTransform", func(c *ctx) any {
		src, dst := c.points(0), c.points(1)
		if len(src) != 4 || len(dst) != 4 {
			panic("getPerspectiveTransform needs four point pairs")
		}
		m := cv.GetPerspectiveTransform([4]cv.Point{src[0], src[1], src[2], src[3]},
			[4]cv.Point{dst[0], dst[1], dst[2], dst[3]})
		return encFloats(m[:])
	})
	reg("perspectiveTransform", func(c *ctx) any {
		pts := c.points2f(0)
		var m cv.PerspectiveMatrix
		copy(m[:], c.fs(1))
		out := cv.PerspectiveTransform(pts, m)
		res := make([]any, len(out))
		for i, p := range out {
			res[i] = encFloats([]float64{p.X, p.Y})
		}
		return res
	})
	reg("warpAffine", func(c *ctx) any {
		var m cv.AffineMatrix
		copy(m[:], c.fs(1))
		return encMat(cv.WarpAffine(c.mat(0), m, c.i(2), c.i(3), lookup(interps, c.s(4))))
	})
	reg("warpPerspective", func(c *ctx) any {
		var m cv.PerspectiveMatrix
		copy(m[:], c.fs(1))
		return encMat(cv.WarpPerspective(c.mat(0), m, c.i(2), c.i(3), lookup(interps, c.s(4))))
	})
	reg("remap", func(c *ctx) any {
		return encMat(cv.Remap(c.mat(0), c.fmat(1), c.fmat(2), lookup(interps, c.s(3))))
	})
	reg("resize", func(c *ctx) any {
		return encMat(cv.Resize(c.mat(0), c.i(1), c.i(2), lookup(interps, c.s(3))))
	})
	reg("getRectSubPix", func(c *ctx) any {
		return encMat(cv.GetRectSubPix(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4)))
	})
	reg("warpPolar", func(c *ctx) any {
		return encMat(cv.WarpPolar(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4), c.f(5),
			lookup(warpPolarModes, c.s(6))))
	})
	reg("linearPolar", func(c *ctx) any {
		return encMat(cv.LinearPolar(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4), c.f(5)))
	})
	reg("logPolar", func(c *ctx) any {
		return encMat(cv.LogPolar(c.mat(0), c.i(1), c.i(2), c.f(3), c.f(4), c.f(5)))
	})

	// ---- contours -----------------------------------------------------
	reg("findContours", func(c *ctx) any {
		cnts, hier := cv.FindContours(gray(c.mat(0)), lookup(retrModes, c.s(1)),
			lookup(chainModes, c.s(2)))
		withParent := 0
		for _, h := range hier {
			if h.Parent >= 0 {
				withParent++
			}
		}
		items := make([]map[string]any, 0, len(cnts))
		for _, cc := range cnts {
			r := cv.BoundingRect(cc)
			items = append(items, map[string]any{
				"n":    len(cc),
				"rect": []int{r.X, r.Y, r.Width, r.Height},
				"pts":  sortPoints(encPoints(cc)),
			})
		}
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i], items[j]
			if a["n"].(int) != b["n"].(int) {
				return a["n"].(int) < b["n"].(int)
			}
			if !equalInts(a["rect"].([]int), b["rect"].([]int)) {
				return lessIntSlice(a["rect"].([]int), b["rect"].([]int))
			}
			ap, bp := a["pts"].([][]int), b["pts"].([][]int)
			for k := 0; k < len(ap) && k < len(bp); k++ {
				if !equalInts(ap[k], bp[k]) {
					return lessIntSlice(ap[k], bp[k])
				}
			}
			return len(ap) < len(bp)
		})
		out := make([]any, len(items))
		for i, it := range items {
			out[i] = it
		}
		return map[string]any{"count": len(cnts), "hierLen": len(hier),
			"withParent": withParent, "topLevel": len(hier) - withParent,
			"contours": out}
	})
	reg("contourArea", func(c *ctx) any { return encNum(cv.ContourArea(cv.Contour(c.points(0)))) })
	reg("arcLength", func(c *ctx) any { return encNum(cv.ArcLength(c.points(0), c.b(1))) })
	reg("approxPolyDP", func(c *ctx) any {
		return encPoints(cv.ApproxPolyDP(c.points(0), c.f(1), c.b(2)))
	})
	reg("convexHull", func(c *ctx) any { return sortPoints(encPoints(cv.ConvexHull(c.points(0)))) })
	reg("isContourConvex", func(c *ctx) any { return cv.IsContourConvex(c.points(0)) })
	reg("boundingRect", func(c *ctx) any {
		r := cv.BoundingRect(c.points(0))
		return []int{r.X, r.Y, r.Width, r.Height}
	})
	reg("minAreaRect", func(c *ctx) any {
		r := cv.MinAreaRect(c.points(0))
		sides := []float64{round(r.Width, 3), round(r.Height, 3)}
		sort.Float64s(sides)
		return map[string]any{"cx": encNum(round(r.CenterX, 3)), "cy": encNum(round(r.CenterY, 3)),
			"sides": encFloats(sides), "area": encNum(round(r.Width*r.Height, 3))}
	})
	reg("rectVsMinArea", func(c *ctx) any {
		pts := c.points(0)
		br := cv.BoundingRect(pts)
		mr := cv.MinAreaRect(pts)
		return map[string]any{"rectW": br.Width, "rectH": br.Height,
			"minW": encNum(round(mr.Width, 3)), "minH": encNum(round(mr.Height, 3)),
			"sameW": math.Abs(mr.Width-float64(br.Width)) < 1e-6 ||
				math.Abs(mr.Height-float64(br.Width)) < 1e-6}
	})
	reg("boxPoints", func(c *ctx) any {
		r := cv.RotatedRect{CenterX: c.f(0), CenterY: c.f(1), Width: c.f(2),
			Height: c.f(3), Angle: c.f(4)}
		pts := cv.BoxPoints(r)
		rows := make([][]float64, 4)
		for i, p := range pts {
			rows[i] = []float64{round(p.X, 3), round(p.Y, 3)}
		}
		sort.Slice(rows, func(i, j int) bool { return lessFloatSlice(rows[i], rows[j]) })
		out := make([]any, 4)
		for i, r := range rows {
			out[i] = encFloats(r)
		}
		return out
	})
	reg("minEnclosingCircle", func(c *ctx) any {
		ctr, r := cv.MinEnclosingCircle(c.points(0))
		return map[string]any{"cx": encNum(round(ctr.X, 2)), "cy": encNum(round(ctr.Y, 2)),
			"r": encNum(round(r, 2))}
	})
	reg("pointPolygonTest", func(c *ctx) any {
		return encNum(cv.PointPolygonTest(c.points(0), cv.Point{X: int(c.f(1)), Y: int(c.f(2))}, c.b(3)))
	})
	reg("fitLine", func(c *ctx) any {
		vx, vy, x0, y0 := cv.FitLine(c.points(0))
		if vx < 0 || (vx == 0 && vy < 0) {
			vx, vy = -vx, -vy
		}
		return encFloats([]float64{vx, vy, x0, y0})
	})
	reg("moments", func(c *ctx) any {
		m := cv.ImageMoments(gray(c.mat(0)))
		return map[string]any{
			"m00": encNum(m.M00), "m10": encNum(m.M10), "m01": encNum(m.M01),
			"m20": encNum(m.M20), "m11": encNum(m.M11), "m02": encNum(m.M02),
			"m30": encNum(m.M30), "m21": encNum(m.M21), "m12": encNum(m.M12),
			"m03":  encNum(m.M03),
			"mu20": encNum(m.Mu20), "mu11": encNum(m.Mu11), "mu02": encNum(m.Mu02),
			"mu30": encNum(m.Mu30), "mu21": encNum(m.Mu21), "mu12": encNum(m.Mu12),
			"mu03": encNum(m.Mu03),
			"nu20": encNum(m.Nu20), "nu11": encNum(m.Nu11), "nu02": encNum(m.Nu02),
			"nu30": encNum(m.Nu30), "nu21": encNum(m.Nu21), "nu12": encNum(m.Nu12),
			"nu03": encNum(m.Nu03),
		}
	})
	reg("huMoments", func(c *ctx) any {
		h := cv.HuMoments(cv.ImageMoments(gray(c.mat(0))))
		return encFloats(h[:])
	})
	reg("centroid", func(c *ctx) any {
		x, y := cv.ImageMoments(gray(c.mat(0))).Centroid()
		return encFloats([]float64{x, y})
	})
	reg("matchShapes", func(c *ctx) any {
		return encNum(cv.MatchShapes(cv.ImageMoments(gray(c.mat(0))),
			cv.ImageMoments(gray(c.mat(1)))))
	})
	reg("ellipse2Poly", func(c *ctx) any {
		pts := cv.Ellipse2Poly(cv.Point{X: c.i(0), Y: c.i(1)}, c.i(2), c.i(3),
			c.f(4), c.f(5), c.f(6), c.i(7))
		return encPoints(pts)
	})
	reg("clipLine", func(c *ctx) any {
		p1, p2, ok := cv.ClipLine(cv.Rect{X: c.i(0), Y: c.i(1), Width: c.i(2), Height: c.i(3)},
			cv.Point{X: c.i(4), Y: c.i(5)}, cv.Point{X: c.i(6), Y: c.i(7)})
		return map[string]any{"ok": ok, "p1": []int{p1.X, p1.Y}, "p2": []int{p2.X, p2.Y}}
	})
	reg("lineIterator", func(c *ctx) any {
		it := cv.NewLineIterator(c.i(0), c.i(1), c.i(2), c.i(3))
		pts := it.Points()
		return map[string]any{"count": len(pts), "points": encPoints(pts)}
	})

	// ---- connected components ----------------------------------------
	reg("connectedComponents", func(c *ctx) any {
		conn := cv.Connectivity8
		if c.s(1) == "Connectivity4" {
			conn = cv.Connectivity4
		}
		labels, n := cv.ConnectedComponents(gray(c.mat(0)), conn)
		return map[string]any{"count": n, "labels": relabel(labels)}
	})
	reg("connectedComponentsWithStats", func(c *ctx) any {
		conn := cv.Connectivity8
		if c.s(1) == "Connectivity4" {
			conn = cv.Connectivity4
		}
		labels, n, stats := cv.ConnectedComponentsWithStats(gray(c.mat(0)), conn)
		rows := make([]map[string]any, 0, len(stats))
		for _, s := range stats {
			rows = append(rows, map[string]any{"area": s.Area,
				"rect": []int{s.Rect.X, s.Rect.Y, s.Rect.Width, s.Rect.Height},
				"cx":   encNum(round(s.CentroidX, 4)), "cy": encNum(round(s.CentroidY, 4))})
		}
		var bg any = nil
		body := []any{}
		if len(rows) > 0 {
			bg = rows[0]
			rest := rows[1:]
			sort.SliceStable(rest, func(i, j int) bool {
				if rest[i]["area"].(int) != rest[j]["area"].(int) {
					return rest[i]["area"].(int) < rest[j]["area"].(int)
				}
				return lessIntSlice(rest[i]["rect"].([]int), rest[j]["rect"].([]int))
			})
			for _, r := range rest {
				body = append(body, r)
			}
		}
		return map[string]any{"count": n, "labels": relabel(labels),
			"background": bg, "components": body}
	})

	// ---- Hough / detectors -------------------------------------------
	reg("houghLines", func(c *ctx) any {
		lines := cv.HoughLines(gray(c.mat(0)), c.f(1), c.f(2), c.i(3))
		rows := make([][]float64, len(lines))
		for i, l := range lines {
			rows[i] = []float64{round(l.Rho, 2), round(l.Theta, 4)}
		}
		sort.Slice(rows, func(i, j int) bool { return lessFloatSlice(rows[i], rows[j]) })
		out := make([]any, len(rows))
		for i, r := range rows {
			out[i] = encFloats(r)
		}
		return map[string]any{"count": len(lines), "lines": out}
	})
	reg("houghLinesP", func(c *ctx) any {
		segs := cv.HoughLinesP(gray(c.mat(0)), c.f(1), c.f(2), c.i(3), c.i(4), c.i(5))
		rows := make([][]int, len(segs))
		for i, s := range segs {
			rows[i] = []int{s.Pt1.X, s.Pt1.Y, s.Pt2.X, s.Pt2.Y}
		}
		sortPoints(rows)
		return map[string]any{"count": len(segs), "segments": rows}
	})
	reg("houghCircles", func(c *ctx) any {
		cs := cv.HoughCircles(gray(c.mat(0)), c.f(1), c.f(2), c.f(3), c.i(4), c.i(5))
		rows := make([][]int, len(cs))
		for i, cc := range cs {
			rows[i] = []int{cc.X, cc.Y, cc.Radius}
		}
		sortPoints(rows)
		return map[string]any{"count": len(cs), "circles": rows}
	})
	reg("fastCorners", func(c *ctx) any {
		pts := cv.FASTCorners(gray(c.mat(0)), c.i(1), c.b(2))
		return map[string]any{"count": len(pts), "points": sortPoints(encPoints(pts))}
	})
	reg("goodFeaturesToTrack", func(c *ctx) any {
		pts := cv.GoodFeaturesToTrack(gray(c.mat(0)), c.i(1), c.f(2), c.f(3), c.i(4))
		return map[string]any{"count": len(pts), "points": sortPoints(encPoints(pts))}
	})

	// ---- template matching -------------------------------------------
	reg("matchTemplate", func(c *ctx) any {
		return encFMat(cv.MatchTemplate(c.mat(0), c.mat(1), lookup(tmModes, c.s(2))))
	})
	reg("matchTemplateLoc", func(c *ctx) any {
		res := cv.MatchTemplate(c.mat(0), c.mat(1), lookup(tmModes, c.s(2)))
		_, _, minX, minY, maxX, maxY := cv.MinMaxLoc(res)
		return map[string]any{"minLoc": []int{minX, minY}, "maxLoc": []int{maxX, maxY},
			"rows": res.Rows, "cols": res.Cols}
	})
	reg("minMaxLoc", func(c *ctx) any {
		mn, mx, minX, minY, maxX, maxY := cv.MinMaxLoc(c.fmat(0))
		return map[string]any{"min": encNum(mn), "max": encNum(mx),
			"minLoc": []int{minX, minY}, "maxLoc": []int{maxX, maxY}}
	})
	reg("minMaxLocMat", func(c *ctx) any {
		mn, mx, minX, minY, maxX, maxY := cv.MinMaxLocMat(gray(c.mat(0)))
		return map[string]any{"min": encNum(mn), "max": encNum(mx),
			"minLoc": []int{minX, minY}, "maxLoc": []int{maxX, maxY}}
	})

	// ---- histograms ---------------------------------------------------
	reg("calcHist", func(c *ctx) any { return cv.CalcHist(c.mat(0), c.i(1)) })
	reg("compareHist", func(c *ctx) any {
		return encNum(cv.CompareHist(c.ints(0), c.ints(1), lookup(histCmp, c.s(2))))
	})
	reg("compareHistOfImages", func(c *ctx) any {
		h1 := cv.CalcHist(c.mat(0), 0)
		h2 := cv.CalcHist(c.mat(1), 0)
		return encNum(cv.CompareHist(h1, h2, lookup(histCmp, c.s(2))))
	})
	reg("equalizeHist", func(c *ctx) any { return encMat(cv.EqualizeHist(gray(c.mat(0)))) })
	reg("clahe", func(c *ctx) any { return encMat(cv.CLAHE(gray(c.mat(0)), c.f(1), c.i(2))) })
	reg("calcBackProject", func(c *ctx) any {
		return encMat(cv.CalcBackProject(c.mat(0), c.i(1), c.ints(2)))
	})
	reg("entropy", func(c *ctx) any { return encNum(cv.Entropy(gray(c.mat(0)))) })
	reg("medianValue", func(c *ctx) any { return encNum(cv.Median(gray(c.mat(0)))) })

	// ---- linear algebra ----------------------------------------------
	reg("determinant", func(c *ctx) any {
		n := c.i(1)
		return encNum(cv.Determinant(matFromFloats(n, n, c.fs(0))))
	})
	reg("invert", func(c *ctx) any {
		n := c.i(1)
		inv, ok := cv.Invert(matFromFloats(n, n, c.fs(0)))
		data := []float64{}
		if inv != nil {
			data = inv.Data
		}
		return map[string]any{"ok": ok, "m": encFloats(data)}
	})
	reg("solve", func(c *ctx) any {
		n := c.i(2)
		x, ok := cv.Solve(matFromFloats(n, n, c.fs(0)), matFromFloats(n, 1, c.fs(1)))
		data := []float64{}
		if x != nil {
			data = x.Data
		}
		return map[string]any{"ok": ok, "x": encFloats(data)}
	})
	reg("trace", func(c *ctx) any {
		n := c.i(1)
		return encNum(cv.Trace(matFromFloats(n, n, c.fs(0))))
	})
	reg("gemm", func(c *ctx) any {
		a := matFromFloats(c.i(1), c.i(2), c.fs(0))
		b := matFromFloats(c.i(4), c.i(5), c.fs(3))
		return encFMat(cv.Gemm(a, b, c.f(6), nil, 0, false, false))
	})
	reg("dct", func(c *ctx) any {
		n := c.i(1)
		return encFMat(cv.DCT(matFromFloats(n, n, c.fs(0))))
	})
	reg("dftMagnitude", func(c *ctx) any {
		n := c.i(1)
		re := matFromFloats(n, n, c.fs(0))
		im := cv.NewFloatMat(n, n)
		outRe, outIm := cv.DFT(re, im)
		return encFMat(cv.Magnitude(outRe, outIm))
	})
	reg("cartToPolar", func(c *ctx) any {
		n := c.i(2)
		mag, ang := cv.CartToPolar(matFromFloats(n, n, c.fs(0)),
			matFromFloats(n, n, c.fs(1)), c.b(3))
		return map[string]any{"mag": encFMat(mag), "ang": encFMat(ang)}
	})
	reg("setIdentity", func(c *ctx) any {
		m := cv.NewFloatMat(c.i(0), c.i(1))
		cv.SetIdentity(m, c.f(2))
		return encFMat(m)
	})
	reg("reduce", func(c *ctx) any {
		m := matFromFloats(c.i(1), c.i(2), c.fs(0))
		return encFMat(cv.Reduce(m, c.b(3), lookup(reduceOps, c.s(4))))
	})
	reg("repeatMat", func(c *ctx) any {
		m := matFromFloats(c.i(1), c.i(2), c.fs(0))
		return encFMat(cv.Repeat(m, c.i(3), c.i(4)))
	})
	reg("sortMat", func(c *ctx) any {
		m := matFromFloats(c.i(1), c.i(2), c.fs(0))
		return encFMat(cv.Sort(m, c.b(3), c.b(4)))
	})
	reg("transformMat", func(c *ctx) any {
		m := matFromFloats(c.i(1), c.i(2), c.fs(3))
		return encMat(cv.Transform(c.mat(0), m))
	})

	// ---- quality ------------------------------------------------------
	reg("psnr", func(c *ctx) any { return encNum(quality.PSNR(c.mat(0), c.mat(1))) })
	reg("mseMat", func(c *ctx) any { return encNum(cv.MSE(c.mat(0), c.mat(1))) })
	reg("laplacianVariance", func(c *ctx) any {
		return encNum(quality.LaplacianVariance(gray(c.mat(0))))
	})
	reg("sharpnessIsLaplacianVariance", func(c *ctx) any {
		g := gray(c.mat(0))
		return map[string]any{"equal": math.Abs(quality.Sharpness(g)-
			quality.LaplacianVariance(g)) < 1e-9}
	})

	// ---- colormaps (plot) --------------------------------------------
	reg("colormapTable", func(c *ctx) any { return encTable(plot.ColormapTable(lookup(colormaps, c.s(0)))) })
	reg("plotTable", func(c *ctx) any { return encTable(plot.Table(lookup(colormaps, c.s(0)))) })
	reg("applyColorMapShape", func(c *ctx) any {
		out := plot.ApplyColorMap(gray(c.mat(0)), lookup(colormaps, c.s(1)))
		return map[string]any{"rows": out.Rows, "cols": out.Cols, "channels": out.Channels}
	})
	reg("colorizeShape", func(c *ctx) any {
		out := plot.Colorize(gray(c.mat(0)), lookup(colormaps, c.s(1)))
		return map[string]any{"rows": out.Rows, "cols": out.Cols, "channels": out.Channels}
	})
	reg("applyColorMap", func(c *ctx) any {
		return encMat(plot.ApplyColorMap(gray(c.mat(0)), lookup(colormaps, c.s(1))))
	})
	reg("colorize", func(c *ctx) any {
		return encMat(plot.Colorize(gray(c.mat(0)), lookup(colormaps, c.s(1))))
	})
	reg("applyCustomColorMap", func(c *ctx) any {
		var rows [][3]uint8
		c.into(1, &rows)
		return encMat(plot.ApplyCustomColorMap(gray(c.mat(0)), rows))
	})

	// ---- segmentation -------------------------------------------------
	reg("grabCut", func(c *ctx) any {
		img := c.mat(0)
		rect := c.ints(1)
		iters := c.i(2)
		var mask *cv.Mat
		switch c.s(3) {
		case "zeros":
			mask = cv.NewMat(img.Rows, img.Cols, 1)
		case "none":
			mask = nil
		default:
			panic("unknown grabCut mask mode")
		}
		out := segmentation.GrabCut(img, mask,
			cv.Rect{X: rect[0], Y: rect[1], Width: rect[2], Height: rect[3]}, iters)
		fg := 0
		for _, v := range out.Data {
			if v == segmentation.GcFgd || v == segmentation.GcPrFgd {
				fg++
			}
		}
		return map[string]any{"foreground": fg, "background": len(out.Data) - fg,
			"allBackground": fg == 0}
	})
}

func encTable(t [][3]uint8) any {
	out := make([]any, len(t))
	for i, row := range t {
		out[i] = []int{int(row[0]), int(row[1]), int(row[2])}
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// relabel canonicalises component ids by first appearance in row-major order,
// so only the partition is compared and not the arbitrary numbering.
func relabel(labels []int) []int {
	m := map[int]int{0: 0}
	next := 1
	out := make([]int, len(labels))
	for i, v := range labels {
		nv, ok := m[v]
		if !ok {
			nv = next
			next++
			m[v] = nv
		}
		out[i] = nv
	}
	return out
}

// ------------------------------------------------------------------ driver

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// call runs one op, turning any panic — including the library's own — into an
// error rather than letting it take the runner down.
func call(fn string, args []json.RawMessage) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	f, ok := ops[fn]
	if !ok {
		return nil, fmt.Errorf("unknown op: %q", fn)
	}
	return f(&ctx{args: args}), nil
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<26)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		val, err := call(req.Fn, req.Args)
		resp := response{ID: req.ID, OK: err == nil, Value: val}
		if err != nil {
			resp.Value = nil
			resp.Error = err.Error()
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		_ = out.Flush()
	}
}
