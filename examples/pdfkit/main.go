// Command pdfkit-example builds a multi-page PDF report exercising the
// github.com/malcolmston/pdfkit API: metadata, several page sizes, the standard
// fonts, text alignment/wrapping/measurement, vector paths, gradients,
// transparency, transforms, an in-memory PNG and JPEG image, links, outlines and
// form fields. It then re-parses the produced bytes and prints sanity checks.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/malcolmston/pdfkit"
)

const outName = "example.pdf"

// margin is applied by hand: pdfkit has no page-margin concept (see README HOLE).
type margins struct{ Top, Right, Bottom, Left float64 }

var pageMargins = margins{Top: 56, Right: 56, Bottom: 56, Left: 56}

func main() {
	fmt.Println("== pdfkit example ==")

	doc := pdfkit.New()

	// ---- document metadata (/Info) ----
	doc.SetTitle("pdfkit Feature Tour")
	doc.SetAuthor("go-examples")
	doc.SetSubject("Validating the Go port of PDFKit")
	doc.SetKeywords("pdf, go, vector, text, images")
	doc.SetCreator("examples/pdfkit/main.go")
	doc.SetProducer("github.com/malcolmston/pdfkit")
	fmt.Println("[metadata] Title/Author/Subject/Keywords/Creator/Producer set")

	cover := buildCoverPage(doc)
	textPage := buildTextPage(doc)
	vectorPage := buildVectorPage(doc)
	imagePage := buildImagePage(doc)
	sizePage := buildPageSizePage(doc)

	// ---- cross-page navigation: named destinations, links, outline ----
	doc.AddNamedDest("vectors", vectorPage, vectorPage.Size().Height-pageMargins.Top)
	cover.SetFont(pdfkit.HelveticaOblique, 11)
	cover.SetFillColor(pdfkit.RGB(0, 90, 200))
	cover.DrawText(pageMargins.Left, 120, "-> jump to the vector graphics page")
	cover.AddLinkToDest(pageMargins.Left, 116, 220, 16, "vectors")
	cover.AddLinkTo(pageMargins.Left, 96, 220, 16, textPage, textPage.Size().Height-pageMargins.Top)
	cover.DrawText(pageMargins.Left, 100, "-> jump to the typography page")
	cover.AddLinkURI(pageMargins.Left, 76, 220, 16, "https://example.com/")
	cover.DrawText(pageMargins.Left, 80, "-> external URI link")
	cover.AddTextAnnotation(pageMargins.Left, 56, "A sticky-note annotation.")

	outline := doc.Outline()
	bmCover := outline.Add("Cover", cover, cover.Size().Height)
	bmCover.Add("Typography", textPage, textPage.Size().Height)
	outline.Add("Vector graphics", vectorPage, vectorPage.Size().Height)
	outline.Add("Images", imagePage, imagePage.Size().Height)
	outline.Add("Page sizes", sizePage, sizePage.Size().Height)
	fmt.Println("[nav] 3 links, 1 text annotation, 1 named dest, 5 outline entries")

	fmt.Printf("[pages] %d pages added\n", doc.PageCount())

	// ---- serialize ----
	data, err := doc.Bytes()
	if err != nil {
		log.Fatalf("doc.Bytes: %v", err)
	}
	abs, err := filepath.Abs(outName)
	if err != nil {
		log.Fatalf("abs: %v", err)
	}
	if err := doc.Save(abs); err != nil {
		log.Fatalf("doc.Save: %v", err)
	}
	fmt.Printf("[save] wrote %s\n", abs)

	verify(abs, data, doc)
}

// ---------------------------------------------------------------- page 1: cover

func buildCoverPage(doc *pdfkit.Document) *pdfkit.Page {
	page := doc.AddPage(pdfkit.A4)
	w, h := page.Size().Width, page.Size().Height

	// Full-bleed gradient banner, bounded by a clip path.
	page.Save()
	page.Rect(0, h-220, w, 220)
	page.Clip()
	page.Shade(pdfkit.LinearGradient(0, h-220, 0, h,
		pdfkit.GradientStop{Offset: 0, Color: pdfkit.RGB(20, 30, 80)},
		pdfkit.GradientStop{Offset: 1, Color: pdfkit.RGB(90, 160, 230)},
	))
	page.Restore()

	page.SetFont(pdfkit.HelveticaBold, 34)
	page.SetFillColor(pdfkit.White)
	page.DrawText(pageMargins.Left, h-120, "pdfkit Feature Tour")

	page.SetFont(pdfkit.Helvetica, 13)
	page.SetFillColor(pdfkit.RGB(230, 240, 255))
	page.DrawText(pageMargins.Left, h-146, "Pure-Go PDF generation, no cgo, no dependencies")

	// Centered / right-aligned single lines inside the content box.
	boxX := pageMargins.Left
	boxW := w - pageMargins.Left - pageMargins.Right
	page.SetFont(pdfkit.TimesRoman, 12)
	page.SetFillColor(pdfkit.Gray(40))
	page.DrawTextAligned(boxX, h-280, boxW, "centered with DrawTextAligned", pdfkit.AlignCenter)
	page.DrawTextRight(boxX+boxW, h-300, "right-aligned with DrawTextRight")
	page.DrawTextCentered(w/2, h-320, "centered with DrawTextCentered")

	// A small metadata table drawn with rules + measured column widths.
	rows := [][2]string{
		{"Page size", fmt.Sprintf("A4 = %.2f x %.2f pt", w, h)},
		{"Margins", fmt.Sprintf("%.0f pt on all sides (manual)", pageMargins.Left)},
		{"Fonts", "14 standard Type1 fonts with AFM widths"},
		{"Colors", "RGB, Gray, CMYK, HSL, Hex, CSS names"},
	}
	y := h - 380
	page.SetLineWidth(0.6)
	page.SetStrokeColor(pdfkit.Gray(180))
	for _, r := range rows {
		page.SetFont(pdfkit.HelveticaBold, 10)
		page.SetFillColor(pdfkit.RGB(20, 30, 80))
		page.DrawText(boxX, y, r[0])
		page.SetFont(pdfkit.Helvetica, 10)
		page.SetFillColor(pdfkit.Gray(50))
		page.DrawText(boxX+130, y, r[1])
		page.DrawLine(boxX, y-6, boxX+boxW, y-6)
		y -= 22
	}

	// Color-model conversions, each swatch filled with a differently built color.
	hex, err := pdfkit.Hex("#e2725b")
	if err != nil {
		log.Fatalf("pdfkit.Hex: %v", err)
	}
	named, ok := pdfkit.NamedColor("mediumpurple")
	if !ok {
		log.Fatal("pdfkit.NamedColor(mediumpurple) not found")
	}
	swatches := []struct {
		label string
		fill  func()
	}{
		{"RGB", func() { page.SetFillColor(pdfkit.RGB(220, 60, 60)) }},
		{"Gray", func() { page.SetFillColor(pdfkit.Gray(120)) }},
		{"HSL", func() { page.SetFillColor(pdfkit.HSL(150, 0.7, 0.45)) }},
		{"Hex", func() { page.SetFillColor(hex) }},
		{"CSS", func() { page.SetFillColor(named) }},
		{"CMYK", func() { page.SetFillCMYK(pdfkit.CMYK(0.8, 0.2, 0, 0.1)) }},
	}
	sx := boxX
	for _, s := range swatches {
		s.fill()
		page.Rect(sx, y-46, 60, 40)
		page.Fill()
		page.SetFont(pdfkit.Helvetica, 8)
		page.SetFillColor(pdfkit.Gray(60))
		page.DrawTextAligned(sx, y-58, 60, s.label, pdfkit.AlignCenter)
		sx += 70
	}
	fmt.Printf("[cover] gradient banner, %d table rows, %d color swatches, hex=%s\n",
		len(rows), len(swatches), hex.Hex())
	return page
}

// ----------------------------------------------------------- page 2: typography

func buildTextPage(doc *pdfkit.Document) *pdfkit.Page {
	page := doc.AddPage(pdfkit.Letter)
	w, h := page.Size().Width, page.Size().Height
	x := pageMargins.Left
	boxW := w - pageMargins.Left - pageMargins.Right
	y := h - pageMargins.Top

	heading(page, x, y, "Typography")
	y -= 34

	// Every standard font family at a few sizes.
	fonts := []*pdfkit.Font{
		pdfkit.Helvetica, pdfkit.HelveticaBold, pdfkit.HelveticaOblique, pdfkit.HelveticaBoldOblique,
		pdfkit.TimesRoman, pdfkit.TimesBold, pdfkit.TimesItalic, pdfkit.TimesBoldItalic,
		pdfkit.Courier, pdfkit.CourierBold, pdfkit.CourierOblique, pdfkit.CourierBoldOblique,
		pdfkit.Symbol, pdfkit.ZapfDingbats,
	}
	for i, f := range fonts {
		size := 11.0 + float64(i%3)
		page.SetFont(f, size)
		page.SetFillColor(pdfkit.HSL(float64(i)*25, 0.55, 0.38))
		page.DrawText(x, y, fmt.Sprintf("%s - The quick brown fox 0123", f.BaseFont))
		y -= f.LineHeight(size) + 1
	}
	// StandardFont lookup by PDF base-font name.
	if got := pdfkit.StandardFont("Times-BoldItalic"); got != pdfkit.TimesBoldItalic {
		log.Fatal("StandardFont(Times-BoldItalic) mismatch")
	}

	y -= 12
	// Metrics + measurement helpers.
	page.SetFont(pdfkit.Helvetica, 10)
	page.SetFillColor(pdfkit.Gray(30))
	m := pdfkit.Helvetica
	page.DrawText(x, y, fmt.Sprintf(
		"Helvetica@10: ascent=%.2f descent=%.2f cap=%.2f xheight=%.2f lineheight=%.2f width(\"AV\")=%.2f",
		m.Ascent(10), m.Descent(10), m.CapHeight(10), m.XHeight(10), m.LineHeight(10), m.Width("AV", 10)))
	y -= 16
	page.DrawText(x, y, "TruncateToWidth: "+pdfkit.TruncateToWidth(m, 10, 140,
		"this sentence is far too long to fit in the given box", "..."))
	y -= 26

	// Four alignments through DrawTextBox.
	lorem := "PDFKit ported to Go: this paragraph is wrapped by the library's greedy " +
		"word-wrap routine so that alignment, underline and strike-through decorations " +
		"can all be exercised over multiple lines of real text."
	for _, a := range []struct {
		name  string
		align pdfkit.TextAlign
		opts  pdfkit.TextOptions
	}{
		{"AlignLeft", pdfkit.AlignLeft, pdfkit.TextOptions{}},
		{"AlignCenter", pdfkit.AlignCenter, pdfkit.TextOptions{}},
		{"AlignRight", pdfkit.AlignRight, pdfkit.TextOptions{Strike: true}},
		{"AlignJustify", pdfkit.AlignJustify, pdfkit.TextOptions{Underline: true}},
	} {
		page.SetFont(pdfkit.HelveticaBold, 9)
		page.SetFillColor(pdfkit.RGB(20, 30, 80))
		page.DrawText(x, y, a.name)
		y -= 14
		page.SetFont(pdfkit.TimesRoman, 10)
		page.SetFillColor(pdfkit.Gray(20))
		opts := a.opts
		opts.Align = a.align
		y = page.DrawTextBox(x, y, boxW, 13, lorem, opts) - 10
	}

	// Explicit wrapping / paragraph / multi-line APIs.
	page.SetFont(pdfkit.Courier, 9)
	page.SetFillColor(pdfkit.RGB(0, 100, 60))
	lines := pdfkit.WrapText(pdfkit.Courier, 9, boxW/2, lorem)
	page.DrawLines(x, y, 11, lines)
	y -= float64(len(lines))*11 + 8

	page.SetFont(pdfkit.TimesItalic, 10)
	page.SetFillColor(pdfkit.Gray(70))
	y = page.DrawParagraph(x, y, boxW, 12, lorem)

	fmt.Printf("[text] %d fonts sampled, WrapText -> %d lines, HeightOfString=%.2f\n",
		len(fonts), len(lines), pdfkit.Courier.HeightOfString(lorem, 9, boxW/2, 2))
	return page
}

// ------------------------------------------------------- page 3: vector drawing

func buildVectorPage(doc *pdfkit.Document) *pdfkit.Page {
	page := doc.AddPage(pdfkit.A4)
	h := page.Size().Height
	x := pageMargins.Left
	y := h - pageMargins.Top

	heading(page, x, y, "Vector graphics")

	// Lines with caps, joins, dashes.
	page.SetLineWidth(6)
	page.SetStrokeColor(pdfkit.RGB(200, 40, 40))
	for i, cap := range []pdfkit.LineCap{pdfkit.CapButt, pdfkit.CapRound, pdfkit.CapSquare} {
		page.SetLineCap(cap)
		page.DrawLine(x, y-40-float64(i)*18, x+140, y-40-float64(i)*18)
	}
	page.SetLineCap(pdfkit.CapButt)
	page.SetLineJoin(pdfkit.JoinRound)
	page.SetMiterLimit(4)
	page.SetStrokeColor(pdfkit.RGB(40, 90, 200))
	page.SetLineWidth(2)
	page.SetDash(0, 6, 3)
	page.Polyline(x+180, y-40, x+230, y-80, x+280, y-40, x+330, y-80)
	page.Stroke()
	page.SetDash(0) // reset to solid

	// Rect / rounded rect / circle / ellipse / polygon, filled and stroked.
	top := y - 130
	page.SetStrokeColor(pdfkit.Gray(40))
	page.SetLineWidth(1.5)

	page.SetFillColor(pdfkit.RGB(250, 210, 120))
	page.Rect(x, top-70, 90, 60)
	page.FillStroke()

	page.SetFillColor(pdfkit.RGB(140, 200, 160))
	page.RoundedRect(x+110, top-70, 90, 60, 14)
	page.FillStroke()

	page.SetFillColor(pdfkit.Blue)
	page.Circle(x+250, top-40, 30)
	page.Fill()

	page.SetFillColor(pdfkit.RGB(220, 120, 200))
	page.Ellipse(x+340, top-40, 45, 25)
	page.FillStroke()

	page.SetFillColor(pdfkit.RGB(90, 170, 220))
	page.RegularPolygon(x+440, top-40, 32, 6)
	page.FillStroke()

	// Bezier path built by hand, plus a closed polygon with even-odd fill.
	page.SetStrokeColor(pdfkit.RGB(230, 90, 30))
	page.SetLineWidth(3)
	page.MoveTo(x, top-130)
	page.CurveTo(x+60, top-60, x+140, top-200, x+200, top-130)
	page.LineTo(x+240, top-160)
	page.Stroke()

	page.SetFillColor(pdfkit.RGB(60, 60, 120))
	page.Polygon(x+300, top-190, x+380, top-190, x+420, top-120, x+340, top-95, x+260, top-120)
	page.FillEvenOdd()

	// SVG path data (arcs + quadratics) parsed by the library.
	page.Save()
	page.Translate(x, top-320)
	page.SetLineWidth(2)
	page.SetStrokeColor(pdfkit.RGB(0, 130, 110))
	if err := page.SVGPath("M 0 0 L 60 0 A 30 30 0 0 1 60 60 Q 30 90 0 60 Z"); err != nil {
		log.Fatalf("SVGPath: %v", err)
	}
	page.Stroke()
	page.Restore()

	// Transparency + blend modes over a shading-pattern fill.
	page.Save()
	pat := pdfkit.NewShadingPattern(pdfkit.RadialGradient(
		x+330, top-300, 5, x+330, top-300, 80,
		pdfkit.GradientStop{Offset: 0, Color: pdfkit.RGB(255, 240, 140)},
		pdfkit.GradientStop{Offset: 1, Color: pdfkit.RGB(200, 60, 20)},
	))
	page.SetFillPattern(pat)
	page.Circle(x+330, top-300, 70)
	page.Fill()
	page.ApplyExtGState(pdfkit.NewExtGState(0.55, 0.55, pdfkit.BlendMultiply))
	page.SetFillColor(pdfkit.RGB(40, 60, 200))
	page.Circle(x+390, top-300, 55)
	page.Fill()
	page.SetAlpha(0.35, 1)
	page.SetBlendMode(pdfkit.BlendScreen)
	page.SetFillColor(pdfkit.Green)
	page.Circle(x+360, top-260, 45)
	page.Fill()
	page.Restore()

	// Tiling pattern fill.
	tile := pdfkit.NewTilingPattern(14, 14, func(cell *pdfkit.Page) {
		cell.SetFillColor(pdfkit.RGB(235, 235, 245))
		cell.Rect(0, 0, 14, 14)
		cell.Fill()
		cell.SetFillColor(pdfkit.RGB(120, 140, 200))
		cell.Circle(7, 7, 3.5)
		cell.Fill()
	})
	page.Save()
	page.SetFillPattern(tile)
	page.Rect(x, pageMargins.Bottom, 200, 90)
	page.Fill()
	page.Restore()

	// Transform stack: scaled + translated star drawn repeatedly.
	for i := 0; i < 5; i++ {
		page.Save()
		page.Translate(x+260+float64(i)*60, pageMargins.Bottom+45)
		s := 0.5 + float64(i)*0.18
		page.Scale(s, s)
		// HOLE: there is no Page.Rotate(angle) helper (upstream PDFKit has
		// doc.rotate), so a rotation has to be spelled out as a raw Transform
		// matrix. Using a shear here to keep the example honest:
		//   page.Rotate(float64(i) * 10) // does not exist
		page.Transform(1, 0.15, 0, 1, 0, 0) // slight shear
		page.SetFillColor(pdfkit.HSL(float64(i)*60, 0.65, 0.5))
		star(page, 0, 0, 26, 11)
		page.FillEvenOdd()
		page.Restore()
	}

	fmt.Println("[vector] lines/caps/dashes, rect, rounded rect, circle, ellipse, polygon,")
	fmt.Println("         bezier path, SVG path, gradients, patterns, alpha/blend, transforms")
	return page
}

// star appends a closed 10-point star subpath (math from the standard library).
func star(p *pdfkit.Page, cx, cy, outer, inner float64) {
	for i := 0; i < 10; i++ {
		r := outer
		if i%2 == 1 {
			r = inner
		}
		a := -math.Pi/2 + float64(i)*math.Pi/5
		px := cx + r*math.Cos(a)
		py := cy + r*math.Sin(a)
		if i == 0 {
			p.MoveTo(px, py)
		} else {
			p.LineTo(px, py)
		}
	}
	p.ClosePath()
}

// ----------------------------------------------------------------- page 4: images

func buildImagePage(doc *pdfkit.Document) *pdfkit.Page {
	page := doc.AddPage(pdfkit.A4)
	w, h := page.Size().Width, page.Size().Height
	x := pageMargins.Left
	y := h - pageMargins.Top

	heading(page, x, y, "Images (generated in memory)")

	// RGBA PNG with a transparent corner -> exercises the soft-mask path.
	pngImg, err := pdfkit.LoadPNG(bytes.NewReader(makePNG(160, 120)))
	if err != nil {
		log.Fatalf("LoadPNG: %v", err)
	}
	// Same bytes through the format-sniffing entry point.
	sniffed, err := pdfkit.LoadImage(bytes.NewReader(makePNG(160, 120)))
	if err != nil {
		log.Fatalf("LoadImage(png): %v", err)
	}
	jpgImg, err := pdfkit.LoadJPEG(bytes.NewReader(makeJPEG(160, 120)))
	if err != nil {
		log.Fatalf("LoadJPEG: %v", err)
	}
	grayImg, err := pdfkit.LoadImage(bytes.NewReader(makeGrayPNG(120, 120)))
	if err != nil {
		log.Fatalf("LoadImage(gray png): %v", err)
	}

	label := func(lx, ly float64, s string) {
		page.SetFont(pdfkit.Helvetica, 9)
		page.SetFillColor(pdfkit.Gray(60))
		page.DrawText(lx, ly, s)
	}

	// Checkerboard behind the PNG so its transparency is visible.
	page.SetFillColor(pdfkit.Gray(215))
	for cx := 0; cx < 10; cx++ {
		for cy := 0; cy < 8; cy++ {
			if (cx+cy)%2 == 0 {
				continue
			}
			page.Rect(x+float64(cx)*20, y-190+float64(cy)*20, 20, 20)
			page.Fill()
		}
	}
	page.DrawImage(pngImg, x, y-190, 200, 150)
	label(x, y-204, fmt.Sprintf("PNG RGBA + SMask (%dx%d px)", pngImg.Width, pngImg.Height))

	page.DrawImage(jpgImg, x+230, y-190, 200, 150)
	label(x+230, y-204, fmt.Sprintf("JPEG / DCTDecode (%dx%d px)", jpgImg.Width, jpgImg.Height))

	page.DrawImage(grayImg, x, y-380, 150, 150)
	label(x, y-394, fmt.Sprintf("Grayscale PNG (%dx%d px)", grayImg.Width, grayImg.Height))

	// Same image scaled and clipped to a circle, plus one under a transform.
	page.Save()
	page.Circle(x+250, y-305, 75)
	page.Clip()
	page.DrawImage(sniffed, x+175, y-380, 150, 150)
	page.Restore()
	label(x+180, y-394, "LoadImage sniffing + circular clip")

	page.Save()
	page.Translate(x+400, y-360)
	page.Scale(1.1, 0.8)
	page.DrawImage(jpgImg, 0, 0, 120, 120)
	page.Restore()
	label(x+400, y-394, "Transformed placement")

	// Form fields (AcroForm) live on this page too.
	page.SetFont(pdfkit.HelveticaBold, 11)
	page.SetFillColor(pdfkit.RGB(20, 30, 80))
	page.DrawText(x, pageMargins.Bottom+150, "Interactive form fields")
	field := page.AddTextField("reviewer", x, pageMargins.Bottom+110, 200, 22, "type here")
	page.AddCheckbox("approved", x+230, pageMargins.Bottom+110, 20, true)
	page.AddPushButton("submit", x+280, pageMargins.Bottom+110, 90, 22, "Submit")
	// HOLE: *FormField has no exported fields or methods, so the returned value
	// cannot be inspected or adjusted (e.g. font size, border, read-only flag).
	// The fields are emitted with "/Helv 0 Tf" (auto-size) + /NeedAppearances,
	// so viewers render them at wildly different sizes.
	//   field.SetFontSize(9) // does not exist
	_ = field

	fmt.Printf("[images] 4 in-memory images embedded (PNG RGBA, PNG gray, JPEG, sniffed); page %.0fx%.0f\n", w, h)
	fmt.Println("[forms] text field, checkbox and push button added")
	return page
}

// makePNG builds an RGBA PNG in memory with a gradient and a transparent wedge.
func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			a := uint8(255)
			if px+py < w/3 { // transparent top-left wedge
				a = 0
			}
			img.SetRGBA(px, py, color.RGBA{
				R: uint8(255 * px / w),
				G: uint8(255 * py / h),
				B: uint8(200 - 150*px/w),
				A: a,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// makeGrayPNG builds an 8-bit grayscale PNG with concentric rings.
func makeGrayPNG(w, h int) []byte {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			dx := float64(px - w/2)
			dy := float64(py - h/2)
			d := math.Sqrt(dx*dx + dy*dy)
			img.SetGray(px, py, color.Gray{Y: uint8(127 + 120*math.Sin(d/6))})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// makeJPEG builds a JPEG in memory (checker + diagonal ramp).
func makeJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			c := color.RGBA{R: 30, G: uint8(60 + 150*py/h), B: uint8(220 - 120*px/w), A: 255}
			if (px/16+py/16)%2 == 0 {
				c.R = 240
			}
			img.SetRGBA(px, py, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		log.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

// ------------------------------------------------------------ page 5: page sizes

func buildPageSizePage(doc *pdfkit.Document) *pdfkit.Page {
	// Landscape A5 to show orientation handling, plus a custom-size page after it.
	page := doc.AddPage(pdfkit.A5.Landscape())
	w, h := page.Size().Width, page.Size().Height
	x := pageMargins.Left
	heading(page, x, h-pageMargins.Top, "Page sizes & margins")

	// Visualize the margin box.
	page.SetStrokeColor(pdfkit.RGB(200, 80, 80))
	page.SetLineWidth(0.8)
	page.SetDash(0, 4, 3)
	page.Rect(pageMargins.Left, pageMargins.Bottom,
		w-pageMargins.Left-pageMargins.Right, h-pageMargins.Top-pageMargins.Bottom)
	page.Stroke()
	page.SetDash(0)

	sizes := []struct {
		name string
		s    pdfkit.PageSize
	}{
		{"A3", pdfkit.A3}, {"A4", pdfkit.A4}, {"A5", pdfkit.A5},
		{"Letter", pdfkit.Letter}, {"Legal", pdfkit.Legal}, {"Tabloid", pdfkit.Tabloid},
		{"A4 landscape", pdfkit.A4.Landscape()}, {"A4 portrait", pdfkit.A4.Landscape().Portrait()},
		{"Custom 300x200", pdfkit.Custom(300, 200)},
	}
	y := h - pageMargins.Top - 34
	page.SetFont(pdfkit.Courier, 9)
	page.SetFillColor(pdfkit.Gray(30))
	for _, s := range sizes {
		page.DrawText(x, y, fmt.Sprintf("%-16s %8.2f x %8.2f pt", s.name, s.s.Width, s.s.Height))
		y -= 12
	}

	// SizeToPoint: CSS-ish size strings resolved against a context.
	ctx := pdfkit.SizeContext{}
	page.SetFont(pdfkit.Helvetica, 9)
	y -= 10
	for _, sz := range []string{"1in", "10mm", "2cm", "12pt", "3pc"} {
		v, ok := pdfkit.SizeToPoint(sz, 0, ctx)
		page.DrawText(x, y, fmt.Sprintf("SizeToPoint(%q) = %.3f pt (ok=%t)", sz, v, ok))
		y -= 12
	}

	// A final custom-size page so the document mixes sizes.
	small := doc.AddPage(pdfkit.Custom(300, 200))
	small.SetFont(pdfkit.HelveticaBold, 14)
	small.SetFillColor(pdfkit.RGB(20, 30, 80))
	small.DrawTextCentered(150, 120, "Custom 300x200 page")
	small.SetFont(pdfkit.Helvetica, 9)
	small.SetFillColor(pdfkit.Gray(80))
	small.DrawTextAligned(20, 100, 260, "last page of the tour", pdfkit.AlignCenter)
	small.SetStrokeColor(pdfkit.Gray(150))
	small.SetLineWidth(1)
	small.Rect(10, 10, 280, 180)
	small.Stroke()

	fmt.Printf("[sizes] A5 landscape %.2fx%.2f + custom 300x200 page\n", w, h)
	return page
}

func heading(p *pdfkit.Page, x, y float64, title string) {
	p.SetFont(pdfkit.HelveticaBold, 20)
	p.SetFillColor(pdfkit.RGB(20, 30, 80))
	p.DrawText(x, y, title)
	p.SetStrokeColor(pdfkit.RGB(20, 30, 80))
	p.SetLineWidth(1.2)
	p.DrawLine(x, y-8, x+260, y-8)
}

// ------------------------------------------------------------------- validation

func verify(path string, data []byte, doc *pdfkit.Document) {
	fmt.Println("\n== sanity checks ==")

	onDisk, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read back: %v", err)
	}
	check("Save() bytes match Bytes()", bytes.Equal(onDisk, data),
		fmt.Sprintf("%d vs %d bytes", len(onDisk), len(data)))
	check("non-trivial size", len(onDisk) > 20000, fmt.Sprintf("%d bytes", len(onDisk)))
	check("starts with %PDF-", bytes.HasPrefix(onDisk, []byte("%PDF-")), string(onDisk[:8]))
	check("ends with EOF marker", bytes.HasSuffix(bytes.TrimRight(onDisk, "\r\n"), []byte("%%EOF")),
		fmt.Sprintf("%q", string(onDisk[len(onDisk)-8:])))
	check("has xref table", bytes.Contains(onDisk, []byte("\nxref")), "")
	check("has trailer", bytes.Contains(onDisk, []byte("trailer")), "")
	check("page count", doc.PageCount() == 6, fmt.Sprintf("%d pages", doc.PageCount()))

	r, err := pdfkit.Parse(onDisk)
	if err != nil {
		log.Fatalf("pdfkit.Parse: %v", err)
	}
	check("re-parsed via pdfkit.Reader", r.ObjectCount() > 20,
		fmt.Sprintf("%d indirect objects", r.ObjectCount()))

	// Every xref entry must resolve to a readable object body.
	missing := 0
	for i := 1; i <= r.ObjectCount(); i++ {
		if len(r.Object(i)) == 0 {
			missing++
		}
	}
	check("all xref entries resolve", missing == 0, fmt.Sprintf("%d empty", missing))

	for _, want := range []string{"/Type /Catalog", "/Type /Pages", "/Type /Page", "/Font",
		"/XObject", "DCTDecode", "FlateDecode", "/SMask", "/Shading", "/Pattern",
		"/Annots", "/Outlines", "/AcroForm", "pdfkit Feature Tour"} {
		check("contains "+want, bytes.Contains(onDisk, []byte(want)), "")
	}
	fmt.Println("\nAll checks completed.")
}

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
	}
	if detail != "" {
		fmt.Printf("  [%s] %-32s %s\n", status, name, detail)
	} else {
		fmt.Printf("  [%s] %s\n", status, name)
	}
	if !ok {
		log.Fatalf("sanity check failed: %s", name)
	}
}
