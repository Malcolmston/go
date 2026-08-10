// Command run is the Go-side parity runner for github.com/malcolmston/pdfkit.
//
// It reads one JSON object per line on stdin and writes exactly one JSON object
// per line on stdout, in the same order. It never exits on a failing case: an
// error, an unsupported op or a panic inside the library is reported as
// {"ok": false, "error": ...}. Several of the port's entry points panic (a
// tiling-pattern callback receives a page with no document behind it), so the
// recover() is load-bearing rather than defensive.
//
// The comparable artefact is the finished PDF, returned as lowercase hex; the
// harness parses it and derives the canonical structural description, so exactly
// one parser sees both sides' output.
//
// # Coordinate convention
//
// The neutral op vocabulary uses pdfkit's user space: origin at the top-left of
// the page, y increasing downwards. pdfkit implements that by emitting
// "1 0 0 -1 0 <height> cm" as the first operator of every page, so this runner
// does the same via Transform. Path construction then needs no conversion at
// all. Text and images are anchored to the bottom-left in the Go API, so — again
// exactly as pdfkit does — those calls are wrapped in q / flip / Q, which makes
// the effective CTM the identity, and the y coordinate is converted to
// bottom-left space explicitly.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/malcolmston/pdfkit"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// the neutral document spec

type stopSpec struct {
	Offset  float64   `json:"offset"`
	Color   []float64 `json:"color"`
	Opacity *float64  `json:"opacity"`
}

type gradientSpec struct {
	Op    string     `json:"op"`
	Args  []float64  `json:"args"`
	Stops []stopSpec `json:"stops"`
}

type opSpec struct {
	Op        string            `json:"op"`
	Args      []json.RawMessage `json:"args"`
	Gradient  *gradientSpec     `json:"gradient"`
	Cell      []opSpec          `json:"cell"`
	Underline bool              `json:"underline"`
	Strike    bool              `json:"strike"`
	Origin    []float64         `json:"origin"`
	Color     []float64         `json:"color"`
}

type pageSpec struct {
	Size    json.RawMessage    `json:"size"`
	Layout  string             `json:"layout"`
	Margin  *float64           `json:"margin"`
	Margins map[string]float64 `json:"margins"`
	Rotate  *float64           `json:"rotate"`
	Ops     []opSpec           `json:"ops"`
}

type encryptSpec struct {
	UserPassword     string `json:"userPassword"`
	OwnerPassword    string `json:"ownerPassword"`
	UseAES           bool   `json:"useAES"`
	AllowPrinting    bool   `json:"allowPrinting"`
	AllowModify      bool   `json:"allowModify"`
	AllowCopying     bool   `json:"allowCopying"`
	AllowAnnotations bool   `json:"allowAnnotations"`
}

type docSpec struct {
	Title    *string      `json:"title"`
	Author   *string      `json:"author"`
	Subject  *string      `json:"subject"`
	Keywords *string      `json:"keywords"`
	Producer *string      `json:"producer"`
	Creator  *string      `json:"creator"`
	Encrypt  *encryptSpec `json:"encrypt"`
}

type outlineSpec struct {
	Title    string        `json:"title"`
	Page     *int          `json:"page"`
	Y        float64       `json:"y"`
	Expanded bool          `json:"expanded"`
	Children []outlineSpec `json:"children"`
}

type buildSpec struct {
	Doc     *docSpec      `json:"doc"`
	Pages   []pageSpec    `json:"pages"`
	Outline []outlineSpec `json:"outline"`
}

// ---------------------------------------------------------------------------

var root = func() string {
	if r := os.Getenv("PARITY_ROOT"); r != "" {
		return r
	}
	wd, _ := os.Getwd()
	return wd
}()

var schemeRe = regexp.MustCompile(`^[a-zA-Z]+://`)

func fixturePath(rel string) (string, error) {
	if schemeRe.MatchString(rel) {
		return "", fmt.Errorf("remote paths are not allowed in the parity harness")
	}
	p := rel
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, rel)
	}
	if !strings.HasPrefix(p, root+string(filepath.Separator)) && p != root {
		return "", fmt.Errorf("fixture path escapes the harness root: %s", rel)
	}
	return p, nil
}

var pageSizes = map[string]pdfkit.PageSize{
	"A3": pdfkit.A3, "A4": pdfkit.A4, "A5": pdfkit.A5,
	"LETTER": pdfkit.Letter, "LEGAL": pdfkit.Legal, "TABLOID": pdfkit.Tabloid,
}

func resolveSize(raw json.RawMessage) (pdfkit.PageSize, error) {
	if len(raw) == 0 {
		return pdfkit.A4, nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		s, ok := pageSizes[strings.ToUpper(name)]
		if !ok {
			return pdfkit.PageSize{}, fmt.Errorf("unknown page size: %s", name)
		}
		return s, nil
	}
	var dims []float64
	if err := json.Unmarshal(raw, &dims); err == nil && len(dims) == 2 {
		return pdfkit.Custom(dims[0], dims[1]), nil
	}
	return pdfkit.PageSize{}, fmt.Errorf("bad page size: %s", raw)
}

// argument decoding -------------------------------------------------------

type args struct {
	raw []json.RawMessage
}

func (a args) n(i int) (float64, error) {
	if i >= len(a.raw) {
		return 0, fmt.Errorf("missing numeric argument %d", i)
	}
	var f float64
	if err := json.Unmarshal(a.raw[i], &f); err != nil {
		return 0, fmt.Errorf("argument %d is not a number: %s", i, a.raw[i])
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("argument %d is not finite", i)
	}
	return f, nil
}

func (a args) s(i int) (string, error) {
	if i >= len(a.raw) {
		return "", fmt.Errorf("missing string argument %d", i)
	}
	var s string
	if err := json.Unmarshal(a.raw[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %s", i, a.raw[i])
	}
	return s, nil
}

func (a args) b(i int) (bool, error) {
	if i >= len(a.raw) {
		return false, fmt.Errorf("missing boolean argument %d", i)
	}
	var b bool
	if err := json.Unmarshal(a.raw[i], &b); err != nil {
		return false, fmt.Errorf("argument %d is not a boolean: %s", i, a.raw[i])
	}
	return b, nil
}

func (a args) floats(i int) ([]float64, error) {
	if i >= len(a.raw) {
		return nil, fmt.Errorf("missing array argument %d", i)
	}
	var fs []float64
	if err := json.Unmarshal(a.raw[i], &fs); err != nil {
		return nil, fmt.Errorf("argument %d is not a number array: %s", i, a.raw[i])
	}
	return fs, nil
}

func (a args) nums() ([]float64, error) {
	out := make([]float64, 0, len(a.raw))
	for i := range a.raw {
		f, err := a.n(i)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// rgbColor takes 0..255 components, matching the neutral vocabulary.
func rgbColor(c []float64) (pdfkit.Color, error) {
	if len(c) != 3 {
		return pdfkit.Color{}, fmt.Errorf("rgb expects [r,g,b] 0..255")
	}
	return pdfkit.Color{R: c[0] / 255, G: c[1] / 255, B: c[2] / 255}, nil
}

// cmykColor takes 0..100 components, matching the neutral vocabulary (and
// pdfkit's own interpretation of a 4-element colour array).
func cmykColor(c []float64) (pdfkit.CMYKColor, error) {
	if len(c) != 4 {
		return pdfkit.CMYKColor{}, fmt.Errorf("cmyk expects [c,m,y,k] 0..100")
	}
	return pdfkit.CMYK(c[0]/100, c[1]/100, c[2]/100, c[3]/100), nil
}

var aligns = map[string]pdfkit.TextAlign{
	"left": pdfkit.AlignLeft, "center": pdfkit.AlignCenter,
	"right": pdfkit.AlignRight, "justify": pdfkit.AlignJustify,
}

// ---------------------------------------------------------------------------

type builder struct {
	doc    *pdfkit.Document
	pages  []*pdfkit.Page
	page   *pdfkit.Page
	height float64

	font     *pdfkit.Font
	fontSize float64
}

// flip pushes the inverse of the page-level coordinate flip, making the
// effective CTM the identity, exactly as pdfkit does around text and images.
func (b *builder) flip() {
	b.page.Save()
	b.page.Transform(1, 0, 0, -1, 0, b.height)
}

func (b *builder) unflip() { b.page.Restore() }

// baseline converts a top-down "top of the line box" y to the bottom-left
// baseline y the Go API expects. pdfkit's default text baseline is the font
// ascender below the given y.
func (b *builder) baseline(y float64) (float64, error) {
	if b.font == nil {
		// Mirror pdfkit's placement arithmetic anyway so the emitted trace is
		// comparable; DrawText will simply draw nothing.
		return b.height - y, nil
	}
	return b.height - (y + b.font.Ascent(b.fontSize)), nil
}

func (b *builder) applyOps(ops []opSpec) error {
	for _, op := range ops {
		if err := b.applyOp(op); err != nil {
			return fmt.Errorf("op %q: %w", op.Op, err)
		}
	}
	return nil
}

func (b *builder) applyOp(op opSpec) error {
	a := args{op.Args}
	p := b.page
	switch op.Op {

	// --- text -------------------------------------------------------------
	case "setFont":
		name, err := a.s(0)
		if err != nil {
			return err
		}
		size, err := a.n(1)
		if err != nil {
			return err
		}
		f := pdfkit.StandardFont(name)
		if f == nil {
			return fmt.Errorf("not one of the standard 14 fonts: %s", name)
		}
		b.font, b.fontSize = f, size
		p.SetFont(f, size)
		return nil

	case "text":
		x, err := a.n(0)
		if err != nil {
			return err
		}
		y, err := a.n(1)
		if err != nil {
			return err
		}
		s, err := a.s(2)
		if err != nil {
			return err
		}
		by, err := b.baseline(y)
		if err != nil {
			return err
		}
		b.flip()
		p.DrawText(x, by, s)
		b.unflip()
		return nil

	case "textAligned":
		x, err := a.n(0)
		if err != nil {
			return err
		}
		y, err := a.n(1)
		if err != nil {
			return err
		}
		w, err := a.n(2)
		if err != nil {
			return err
		}
		s, err := a.s(3)
		if err != nil {
			return err
		}
		an, err := a.s(4)
		if err != nil {
			return err
		}
		al, ok := aligns[an]
		if !ok {
			return fmt.Errorf("unknown alignment: %s", an)
		}
		by, err := b.baseline(y)
		if err != nil {
			return err
		}
		b.flip()
		p.DrawTextAligned(x, by, w, s, al)
		b.unflip()
		return nil

	case "textBox":
		x, err := a.n(0)
		if err != nil {
			return err
		}
		y, err := a.n(1)
		if err != nil {
			return err
		}
		w, err := a.n(2)
		if err != nil {
			return err
		}
		gap, err := a.n(3)
		if err != nil {
			return err
		}
		s, err := a.s(4)
		if err != nil {
			return err
		}
		an, err := a.s(5)
		if err != nil {
			return err
		}
		al, ok := aligns[an]
		if !ok {
			return fmt.Errorf("unknown alignment: %s", an)
		}
		by, err := b.baseline(y)
		if err != nil {
			return err
		}
		b.flip()
		p.DrawTextBox(x, by, w, 0, s, pdfkit.TextOptions{Align: al, LineGap: gap})
		b.unflip()
		return nil

	case "textDecorated":
		x, err := a.n(0)
		if err != nil {
			return err
		}
		y, err := a.n(1)
		if err != nil {
			return err
		}
		w, err := a.n(2)
		if err != nil {
			return err
		}
		s, err := a.s(3)
		if err != nil {
			return err
		}
		by, err := b.baseline(y)
		if err != nil {
			return err
		}
		b.flip()
		p.DrawTextBox(x, by, w, 0, s, pdfkit.TextOptions{Underline: op.Underline, Strike: op.Strike})
		b.unflip()
		return nil

	case "textCharSpacing":
		return fmt.Errorf("the port has no character-spacing option (no Tc operator is ever emitted)")

	case "textFlow":
		return fmt.Errorf("the port has no layout cursor: text always takes explicit coordinates")
	case "moveDown", "moveUp":
		return fmt.Errorf("the port has no layout cursor, so there is nothing to move")
	case "list":
		return fmt.Errorf("the port has no list API")
	case "lineGap":
		return fmt.Errorf("the port has no document-level line gap; it is a per-call TextOptions field")

	// --- vector -----------------------------------------------------------
	case "moveTo", "lineTo":
		x, err := a.n(0)
		if err != nil {
			return err
		}
		y, err := a.n(1)
		if err != nil {
			return err
		}
		if op.Op == "moveTo" {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
		return nil

	case "curveTo":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 6 {
			return fmt.Errorf("curveTo takes 6 numbers")
		}
		p.CurveTo(v[0], v[1], v[2], v[3], v[4], v[5])
		return nil

	case "quadraticCurveTo":
		return fmt.Errorf("the port has no quadratic curve operator (only cubic CurveTo)")

	case "rect":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 4 {
			return fmt.Errorf("rect takes 4 numbers")
		}
		p.Rect(v[0], v[1], v[2], v[3])
		return nil

	case "roundedRect":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 5 {
			return fmt.Errorf("roundedRect takes 5 numbers")
		}
		p.RoundedRect(v[0], v[1], v[2], v[3], v[4])
		return nil

	case "circle":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 3 {
			return fmt.Errorf("circle takes 3 numbers")
		}
		p.Circle(v[0], v[1], v[2])
		return nil

	case "ellipse":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 4 {
			return fmt.Errorf("ellipse takes 4 numbers")
		}
		p.Ellipse(v[0], v[1], v[2], v[3])
		return nil

	case "polygon":
		v, err := a.nums()
		if err != nil {
			return err
		}
		p.Polygon(v...)
		return nil

	case "closePath":
		p.ClosePath()
		return nil

	case "svgPath":
		d, err := a.s(0)
		if err != nil {
			return err
		}
		return p.SVGPath(d)

	case "fill":
		p.Fill()
		return nil
	case "fillEvenOdd":
		p.FillEvenOdd()
		return nil
	case "stroke":
		p.Stroke()
		return nil
	case "fillStroke":
		p.FillStroke()
		return nil

	case "fillColor", "strokeColor":
		c, err := a.floats(0)
		if err != nil {
			return err
		}
		col, err := rgbColor(c)
		if err != nil {
			return err
		}
		if op.Op == "fillColor" {
			p.SetFillColor(col)
		} else {
			p.SetStrokeColor(col)
		}
		return nil

	case "fillCMYK", "strokeCMYK":
		c, err := a.floats(0)
		if err != nil {
			return err
		}
		col, err := cmykColor(c)
		if err != nil {
			return err
		}
		if op.Op == "fillCMYK" {
			p.SetFillCMYK(col)
		} else {
			p.SetStrokeCMYK(col)
		}
		return nil

	case "lineWidth":
		w, err := a.n(0)
		if err != nil {
			return err
		}
		p.SetLineWidth(w)
		return nil

	case "lineCap":
		v, err := a.n(0)
		if err != nil {
			return err
		}
		if v != 0 && v != 1 && v != 2 {
			return fmt.Errorf("line cap must be 0, 1 or 2")
		}
		p.SetLineCap(pdfkit.LineCap(int(v)))
		return nil

	case "lineJoin":
		v, err := a.n(0)
		if err != nil {
			return err
		}
		if v != 0 && v != 1 && v != 2 {
			return fmt.Errorf("line join must be 0, 1 or 2")
		}
		p.SetLineJoin(pdfkit.LineJoin(int(v)))
		return nil

	case "miterLimit":
		m, err := a.n(0)
		if err != nil {
			return err
		}
		p.SetMiterLimit(m)
		return nil

	case "dash":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) < 2 {
			return fmt.Errorf("dash takes a phase and at least one length")
		}
		// pdfkit rejects non-positive dash lengths; the port does not, so the
		// runner does not either: the divergence is the point of the case.
		p.SetDash(v[0], v[1:]...)
		return nil

	case "undash":
		p.SetDash(0)
		return nil

	// --- state / transforms -----------------------------------------------
	case "save":
		p.Save()
		return nil
	case "restore":
		p.Restore()
		return nil

	case "translate":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 2 {
			return fmt.Errorf("translate takes 2 numbers")
		}
		p.Translate(v[0], v[1])
		return nil

	case "scale":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 2 {
			return fmt.Errorf("scale takes 2 numbers")
		}
		p.Scale(v[0], v[1])
		return nil

	case "rotate":
		return fmt.Errorf("the port has no rotate transform (only Translate, Scale and a raw Transform)")

	case "transform":
		v, err := a.nums()
		if err != nil {
			return err
		}
		if len(v) != 6 {
			return fmt.Errorf("transform takes 6 numbers")
		}
		p.Transform(v[0], v[1], v[2], v[3], v[4], v[5])
		return nil

	case "clip":
		p.Clip()
		return nil
	case "clipEvenOdd":
		p.ClipEvenOdd()
		return nil

	// --- images -----------------------------------------------------------
	case "image":
		rel, err := a.s(0)
		if err != nil {
			return err
		}
		v, err := args{op.Args[1:]}.nums()
		if err != nil {
			return err
		}
		if len(v) != 4 {
			return fmt.Errorf("image takes a path then x, y, width, height")
		}
		path, err := fixturePath(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		img, err := pdfkit.LoadImage(f)
		if err != nil {
			return err
		}
		b.flip()
		p.DrawImage(img, v[0], b.height-(v[1]+v[3]), v[2], v[3])
		b.unflip()
		return nil

	// --- gradients / patterns ---------------------------------------------
	case "fillGradient", "strokeGradient":
		sh, err := b.gradient(op.Gradient)
		if err != nil {
			return err
		}
		pat := pdfkit.NewShadingPattern(sh)
		if op.Op == "fillGradient" {
			p.SetFillPattern(pat)
		} else {
			p.SetStrokePattern(pat)
		}
		return nil

	case "shade":
		sh, err := b.gradient(op.Gradient)
		if err != nil {
			return err
		}
		p.Shade(sh)
		return nil

	case "fillTilingPattern":
		v, err := args{op.Args[:2]}.nums()
		if err != nil {
			return err
		}
		// The port's tiling patterns are *coloured* (PaintType 1): there is no
		// way to leave the cell uncoloured and supply the colour at selection
		// time, so the pattern colour is painted inside the cell instead.
		col, err := rgbColor(op.Color)
		if err != nil {
			return err
		}
		cell := op.Cell
		pat := pdfkit.NewTilingPattern(v[0], v[1], func(cp *pdfkit.Page) {
			sub := &builder{doc: b.doc, page: cp, height: v[1]}
			cp.SetFillColor(col)
			if err := sub.applyOps(cell); err != nil {
				panic(err)
			}
		})
		p.SetFillPattern(pat)
		return nil

	// --- transparency -----------------------------------------------------
	case "fillOpacity":
		v, err := a.n(0)
		if err != nil {
			return err
		}
		p.SetAlpha(v, -1)
		return nil
	case "strokeOpacity":
		v, err := a.n(0)
		if err != nil {
			return err
		}
		p.SetAlpha(-1, v)
		return nil
	case "opacity":
		v, err := a.n(0)
		if err != nil {
			return err
		}
		p.SetAlpha(v, v)
		return nil
	case "blendMode":
		s, err := a.s(0)
		if err != nil {
			return err
		}
		p.SetBlendMode(pdfkit.BlendMode(s))
		return nil

	// --- annotations / links ----------------------------------------------
	case "linkURI":
		v, err := args{op.Args[:4]}.nums()
		if err != nil {
			return err
		}
		uri, err := a.s(4)
		if err != nil {
			return err
		}
		p.AddLinkURI(v[0], b.height-(v[1]+v[3]), v[2], v[3], uri)
		return nil

	case "linkToDest":
		v, err := args{op.Args[:4]}.nums()
		if err != nil {
			return err
		}
		name, err := a.s(4)
		if err != nil {
			return err
		}
		p.AddLinkToDest(v[0], b.height-(v[1]+v[3]), v[2], v[3], name)
		return nil

	case "linkToPage":
		v, err := args{op.Args[:4]}.nums()
		if err != nil {
			return err
		}
		idx, err := a.n(4)
		if err != nil {
			return err
		}
		atY, err := a.n(5)
		if err != nil {
			return err
		}
		i := int(idx)
		if i < 0 || i >= len(b.pages) {
			return fmt.Errorf("page index %d out of range", i)
		}
		p.AddLinkTo(v[0], b.height-(v[1]+v[3]), v[2], v[3], b.pages[i], b.height-atY)
		return nil

	case "namedDest":
		name, err := a.s(0)
		if err != nil {
			return err
		}
		y, err := a.n(2)
		if err != nil {
			return err
		}
		b.doc.AddNamedDest(name, p, b.height-y)
		return nil

	case "stickyNoteAnnotation":
		v, err := args{op.Args[:4]}.nums()
		if err != nil {
			return err
		}
		s, err := a.s(4)
		if err != nil {
			return err
		}
		p.AddTextAnnotation(v[0], b.height-v[1], s)
		return nil

	case "freeTextAnnotation", "highlightAnnotation", "underlineAnnotation",
		"strikeAnnotation", "lineAnnotation", "rectAnnotation", "ellipseAnnotation":
		return fmt.Errorf("the port has no %s: its only annotations are links and the /Text sticky note", op.Op)

	// --- form fields ------------------------------------------------------
	case "textField":
		name, err := a.s(0)
		if err != nil {
			return err
		}
		v, err := args{op.Args[1:5]}.nums()
		if err != nil {
			return err
		}
		val, err := a.s(5)
		if err != nil {
			return err
		}
		p.AddTextField(name, v[0], b.height-(v[1]+v[3]), v[2], v[3], val)
		return nil

	case "checkbox":
		name, err := a.s(0)
		if err != nil {
			return err
		}
		v, err := args{op.Args[1:4]}.nums()
		if err != nil {
			return err
		}
		checked, err := a.b(4)
		if err != nil {
			return err
		}
		p.AddCheckbox(name, v[0], b.height-(v[1]+v[2]), v[2], checked)
		return nil

	case "textFieldConfigured":
		return fmt.Errorf("*FormField is opaque — it has no exported fields and no setters — so a field's " +
			"appearance, font, alignment, border, background, flags and multiline behaviour cannot be configured")

	case "pushButton":
		name, err := a.s(0)
		if err != nil {
			return err
		}
		v, err := args{op.Args[1:5]}.nums()
		if err != nil {
			return err
		}
		caption, err := a.s(5)
		if err != nil {
			return err
		}
		p.AddPushButton(name, v[0], b.height-(v[1]+v[3]), v[2], v[3], caption)
		return nil
	}
	return fmt.Errorf("unknown op: %s", op.Op)
}

func (b *builder) gradient(g *gradientSpec) (*pdfkit.Shading, error) {
	if g == nil {
		return nil, fmt.Errorf("missing gradient spec")
	}
	if len(g.Stops) < 2 {
		return nil, fmt.Errorf("a gradient needs at least two stops")
	}
	stops := make([]pdfkit.GradientStop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c, err := rgbColor(s.Color)
		if err != nil {
			return nil, err
		}
		if s.Opacity != nil {
			return nil, fmt.Errorf("the port has no per-stop gradient opacity")
		}
		stops = append(stops, pdfkit.GradientStop{Offset: s.Offset, Color: c})
	}
	switch g.Op {
	case "linearGradient":
		if len(g.Args) != 4 {
			return nil, fmt.Errorf("linearGradient takes 4 numbers")
		}
		return pdfkit.LinearGradient(g.Args[0], b.height-g.Args[1], g.Args[2], b.height-g.Args[3], stops...), nil
	case "radialGradient":
		if len(g.Args) != 6 {
			return nil, fmt.Errorf("radialGradient takes 6 numbers")
		}
		return pdfkit.RadialGradient(g.Args[0], b.height-g.Args[1], g.Args[2], g.Args[3], b.height-g.Args[4], g.Args[5], stops...), nil
	}
	return nil, fmt.Errorf("unknown gradient: %s", g.Op)
}

func (b *builder) addOutline(items []outlineSpec, parent *pdfkit.Bookmark) error {
	for _, it := range items {
		target := b.pages[len(b.pages)-1]
		if it.Page != nil {
			if *it.Page < 0 || *it.Page >= len(b.pages) {
				return fmt.Errorf("outline page index %d out of range", *it.Page)
			}
			target = b.pages[*it.Page]
		}
		if it.Expanded {
			return fmt.Errorf("the port has no expanded/open outline option")
		}
		var node *pdfkit.Bookmark
		if parent == nil {
			node = b.doc.Outline().Add(it.Title, target, it.Y)
		} else {
			node = parent.Add(it.Title, target, it.Y)
		}
		if len(it.Children) > 0 {
			if err := b.addOutline(it.Children, node); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildPDF(spec buildSpec) (string, error) {
	if len(spec.Pages) == 0 {
		return "", fmt.Errorf("a case must declare at least one page")
	}
	doc := pdfkit.New()
	// Determinism: the port stamps no CreationDate/ModDate at all, and its
	// Producer/Creator default to "pdfkit"; both sides pin them to "parity".
	doc.SetProducer("parity")
	doc.SetCreator("parity")
	if d := spec.Doc; d != nil {
		if d.Title != nil {
			doc.SetTitle(*d.Title)
		}
		if d.Author != nil {
			doc.SetAuthor(*d.Author)
		}
		if d.Subject != nil {
			doc.SetSubject(*d.Subject)
		}
		if d.Keywords != nil {
			doc.SetKeywords(*d.Keywords)
		}
		if d.Producer != nil {
			doc.SetProducer(*d.Producer)
		}
		if d.Creator != nil {
			doc.SetCreator(*d.Creator)
		}
		if d.Encrypt != nil {
			e := d.Encrypt
			doc.SetEncryption(pdfkit.EncryptOptions{
				UserPassword:     e.UserPassword,
				OwnerPassword:    e.OwnerPassword,
				UseAES:           e.UseAES,
				AllowPrinting:    e.AllowPrinting,
				AllowModify:      e.AllowModify,
				AllowCopying:     e.AllowCopying,
				AllowAnnotations: e.AllowAnnotations,
			})
		}
	}

	b := &builder{doc: doc}
	// Create every page first, so link-to-page and outline targets resolve
	// regardless of declaration order.
	for _, ps := range spec.Pages {
		size, err := resolveSize(ps.Size)
		if err != nil {
			return "", err
		}
		switch ps.Layout {
		case "", "portrait":
		case "landscape":
			size = size.Landscape()
		default:
			return "", fmt.Errorf("unknown layout: %s", ps.Layout)
		}
		if ps.Margin != nil || len(ps.Margins) > 0 {
			return "", fmt.Errorf("the port has no page margin option: AddPage takes only a size")
		}
		if ps.Rotate != nil {
			return "", fmt.Errorf("the port has no page rotation")
		}
		b.pages = append(b.pages, doc.AddPage(size))
	}

	for i, ps := range spec.Pages {
		b.page, b.height = b.pages[i], b.pages[i].Size().Height
		b.font, b.fontSize = nil, 0
		// Mirror pdfkit's page-level flip to a top-left origin.
		b.page.Transform(1, 0, 0, -1, 0, b.height)
		if err := b.applyOps(ps.Ops); err != nil {
			return "", err
		}
	}

	if len(spec.Outline) > 0 {
		b.page = b.pages[0]
		if err := b.addOutline(spec.Outline, nil); err != nil {
			return "", err
		}
	}

	data, err := doc.Bytes()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func handle(req request) (value string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if req.Fn != "buildPDF" {
		return "", fmt.Errorf("unknown fn: %q", req.Fn)
	}
	if len(req.Args) == 0 {
		return "", fmt.Errorf("missing document spec argument")
	}
	var spec buildSpec
	if err := json.Unmarshal(req.Args[0], &spec); err != nil {
		return "", fmt.Errorf("bad document spec: %w", err)
	}
	return buildPDF(spec)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<25)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{ID: "<unparseable>", OK: false, Error: "bad request line: " + err.Error()})
			out.Flush()
			continue
		}
		value, err := handle(req)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: value})
		}
		out.Flush()
	}
	out.Flush()
}
