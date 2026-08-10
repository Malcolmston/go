// Command sharp-example exercises the github.com/malcolmston/sharp image
// processing library end to end. Every input image is generated in memory, so
// the program needs no external asset and no network access.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/malcolmston/sharp"
)

var outDir string

func main() {
	dir, err := os.MkdirTemp("", "sharp-example-*")
	if err != nil {
		log.Fatal(err)
	}
	outDir = dir
	fmt.Println("sharp example — output directory:", outDir)
	fmt.Println()

	src := makeSourceImage(320, 200)

	sectionMetadata(src)
	sectionResize(src)
	sectionExtract(src)
	sectionRotateFlip(src)
	sectionFormats(src)
	sectionQuality(src)
	sectionComposite(src)
	sectionFilters(src)
	sectionTone(src)
	sectionChannels(src)
	sectionStats(src)
	sectionRawRoundTrip(src)
	sectionGeometryExtras(src)
	sectionErrors()

	fmt.Println("== Done ==")
	fmt.Println("all outputs written under", outDir)
}

// ---------------------------------------------------------------- generators

// makeSourceImage builds a colourful test image: a horizontal/vertical gradient
// with a checkerboard overlay in the lower-right quadrant and a solid bar so
// crop/extract results are visually verifiable.
func makeSourceImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(255 * x / (w - 1))
			g := uint8(255 * y / (h - 1))
			b := uint8(128 + 127*math.Sin(float64(x+y)/18.0))
			if x > w/2 && y > h/2 && ((x/8)+(y/8))%2 == 0 {
				r, g, b = 250, 250, 250
			}
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	// Solid magenta bar across the top 12 rows.
	for y := 0; y < 12; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{255, 0, 200, 255})
		}
	}
	return img
}

// makeOverlay builds a semi-transparent circular badge used for compositing.
func makeOverlay(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size-1) / 2
	rad := c
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			if math.Hypot(dx, dy) <= rad {
				img.Set(x, y, color.RGBA{0, 90, 255, 200})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	return img
}

// makeBorderedImage builds an image with a uniform border, for Trim.
func makeBorderedImage(w, h, border int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < border || y < border || x >= w-border || y >= h-border {
				img.Set(x, y, color.RGBA{20, 20, 20, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 180, 40, 255})
			}
		}
	}
	return img
}

// ------------------------------------------------------------------- helpers

func header(s string) {
	fmt.Printf("== %s ==\n", s)
}

// save writes encoded bytes to the output dir and reports the size.
func save(name string, data []byte) string {
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	return path
}

// check decodes the bytes with the stdlib and verifies the dimensions, printing
// a PASS/FAIL line. Only PNG and JPEG are registered with image.Decode, so
// other formats are verified with sharp itself via checkWithSharp.
func check(label string, data []byte, wantW, wantH int) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		fmt.Printf("   FAIL %-28s decode error: %v\n", label, err)
		return
	}
	status := "PASS"
	if cfg.Width != wantW || cfg.Height != wantH {
		status = "FAIL"
	}
	fmt.Printf("   %s %-28s %s %dx%d (want %dx%d) %d bytes\n",
		status, label, format, cfg.Width, cfg.Height, wantW, wantH, len(data))
}

// checkWithSharp round-trips bytes back through sharp.FromBytes and verifies
// the reported metadata.
func checkWithSharp(label string, data []byte, wantW, wantH int, wantFormat sharp.Format) {
	md, err := sharp.FromBytes(data).Metadata()
	if err != nil {
		fmt.Printf("   FAIL %-28s sharp decode error: %v\n", label, err)
		return
	}
	status := "PASS"
	if md.Width != wantW || md.Height != wantH || md.Format != wantFormat {
		status = "FAIL"
	}
	fmt.Printf("   %s %-28s %s %dx%d (want %s %dx%d) %d bytes\n",
		status, label, md.Format, md.Width, md.Height, wantFormat, wantW, wantH, len(data))
}

// dims reports the pipeline's current dimensions, verifying expectations.
func dims(label string, p *sharp.Pipeline, wantW, wantH int) {
	md, err := p.Metadata()
	if err != nil {
		fmt.Printf("   FAIL %-28s %v\n", label, err)
		return
	}
	status := "PASS"
	if md.Width != wantW || md.Height != wantH {
		status = "FAIL"
	}
	fmt.Printf("   %s %-28s %dx%d (want %dx%d)\n", status, label, md.Width, md.Height, wantW, wantH)
}

// px reads a single pixel out of a pipeline.
func px(p *sharp.Pipeline, x, y int) color.RGBA {
	img, err := p.ToImage()
	if err != nil {
		log.Fatalf("ToImage: %v", err)
	}
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// ------------------------------------------------------------------ sections

func sectionMetadata(src image.Image) {
	header("1. Loading and metadata")

	// From an in-memory image.Image.
	md, err := sharp.New(src).Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   from image.Image : %dx%d format=%q channels=%d alpha=%v density=%d space=%s\n",
		md.Width, md.Height, md.Format, md.Channels, md.HasAlpha, md.Density, md.Space)

	// Round-trip via encoded bytes so the source format is detected.
	pngBytes, err := sharp.New(src).ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	md2, err := sharp.FromBytes(pngBytes).Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   from PNG bytes   : %dx%d format=%q channels=%d alpha=%v\n",
		md2.Width, md2.Height, md2.Format, md2.Channels, md2.HasAlpha)

	// From an io.Reader.
	md3, err := sharp.FromReader(bytes.NewReader(pngBytes)).Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   from io.Reader   : %dx%d format=%q\n", md3.Width, md3.Height, md3.Format)

	// From a file on disk.
	path := save("source.png", pngBytes)
	md4, err := sharp.FromFile(path).Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   from file        : %dx%d format=%q\n", md4.Width, md4.Height, md4.Format)

	// Density is settable and reflected in metadata.
	md5, err := sharp.New(src).WithDensity(300).Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   WithDensity(300) : density=%d\n", md5.Density)
	fmt.Println()
}

func sectionResize(src image.Image) {
	header("2. Resize: fit modes and interpolation kernels")

	// FitExact stretches to the exact box, ignoring aspect ratio.
	exact := sharp.New(src).Resize(sharp.ResizeOptions{
		Width: 100, Height: 100, Fit: sharp.FitExact, Interpolation: sharp.Bilinear,
	})
	dims("FitExact 100x100", exact, 100, 100)

	// FitContain fits inside the box (320x200 -> 100x63).
	contain := sharp.New(src).Resize(sharp.ResizeOptions{
		Width: 100, Height: 100, Fit: sharp.FitContain, Interpolation: sharp.Lanczos3,
	})
	dims("FitContain 100x100 box", contain, 100, 63)

	// FitCover covers the box then centre-crops to it exactly.
	cover := sharp.New(src).Resize(sharp.ResizeOptions{
		Width: 100, Height: 100, Fit: sharp.FitCover, Interpolation: sharp.Mitchell,
	})
	dims("FitCover 100x100 box", cover, 100, 100)

	// Omitting a dimension derives it from the aspect ratio.
	auto := sharp.New(src).Resize(sharp.ResizeOptions{Width: 160, Interpolation: sharp.Cubic})
	dims("Resize width-only 160", auto, 160, 100)

	// Convenience helpers.
	dims("ResizeTo(64,48)", sharp.New(src).ResizeTo(64, 48), 64, 48)
	dims("ResizeWidth(80)", sharp.New(src).ResizeWidth(80), 80, 50)
	dims("ResizeHeight(50)", sharp.New(src).ResizeHeight(50), 50*320/200, 50)

	// All five interpolation kernels, each encoded and verified.
	kernels := []struct {
		name   string
		interp sharp.Interpolation
	}{
		{"nearest", sharp.Nearest},
		{"bilinear", sharp.Bilinear},
		{"cubic", sharp.Cubic},
		{"mitchell", sharp.Mitchell},
		{"lanczos3", sharp.Lanczos3},
	}
	for _, k := range kernels {
		data, err := sharp.New(src).
			Resize(sharp.ResizeOptions{Width: 128, Height: 80, Interpolation: k.interp}).
			ToPNG()
		if err != nil {
			log.Fatalf("resize %s: %v", k.name, err)
		}
		save("resize-"+k.name+".png", data)
		check("resize-"+k.name+".png", data, 128, 80)
	}
	fmt.Println()
}

func sectionExtract(src image.Image) {
	header("3. Extract / crop / extend")

	// Extract the magenta bar from the top of the source.
	bar := sharp.New(src).Extract(sharp.Rectangle{X: 0, Y: 0, Width: 320, Height: 12})
	dims("Extract top bar", bar, 320, 12)
	fmt.Printf("        pixel(5,5) = %v (expect magenta 255,0,200)\n", px(bar, 5, 5))

	// Crop is an alias for Extract.
	crop := sharp.New(src).Crop(sharp.Rectangle{X: 200, Y: 120, Width: 100, Height: 60})
	dims("Crop 100x60 @(200,120)", crop, 100, 60)
	data, err := crop.ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("crop.png", data)
	check("crop.png", data, 100, 60)

	// Extend pads with a fill colour.
	ext := sharp.New(src).Extend(10, 20, 30, 40, sharp.RGBA{R: 0, G: 0, B: 0, A: 255})
	dims("Extend t10 r20 b30 l40", ext, 320+60, 200+40)
	fmt.Printf("        corner(0,0) = %v (expect black fill)\n", px(ext, 0, 0))

	// ExtendWith supports edge-replication modes.
	for _, m := range []struct {
		name string
		mode sharp.ExtendMode
	}{
		{"background", sharp.ExtendBackground},
		{"copy", sharp.ExtendCopy},
		{"repeat", sharp.ExtendRepeat},
		{"mirror", sharp.ExtendMirror},
	} {
		p := sharp.New(src).ExtendWith(sharp.ExtendOptions{
			Top: 8, Right: 8, Bottom: 8, Left: 8,
			Background: sharp.White, Mode: m.mode,
		})
		dims("ExtendWith "+m.name, p, 336, 216)
	}
	fmt.Println()
}

func sectionRotateFlip(src image.Image) {
	header("4. Rotate and flip")

	dims("Rotate90", sharp.New(src).Rotate90(), 200, 320)
	dims("Rotate180", sharp.New(src).Rotate180(), 320, 200)
	dims("Rotate270", sharp.New(src).Rotate270(), 200, 320)

	// Arbitrary-angle rotation grows the canvas to the bounding box.
	rot := sharp.New(src).Rotate(30, sharp.RGBA{R: 255, G: 255, B: 255, A: 255})
	md, err := rot.Metadata()
	if err != nil {
		log.Fatal(err)
	}
	// Expected bounding box for a 320x200 image rotated 30 degrees.
	wantW := int(math.Ceil(320*math.Cos(math.Pi/6) + 200*math.Sin(math.Pi/6)))
	wantH := int(math.Ceil(320*math.Sin(math.Pi/6) + 200*math.Cos(math.Pi/6)))
	fmt.Printf("   Rotate(30) -> %dx%d (bounding box ~%dx%d)\n", md.Width, md.Height, wantW, wantH)
	if md.Width < 320 || md.Height < 200 {
		fmt.Println("   FAIL Rotate(30) did not grow the canvas")
	} else {
		fmt.Println("   PASS Rotate(30) canvas grew to contain the rotation")
	}
	rotData, err := rot.ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("rotate30.png", rotData)
	check("rotate30.png", rotData, md.Width, md.Height)

	// Flips. FlipVertical mirrors top<->bottom, FlipHorizontal left<->right.
	// Flip/Flop are the sharp-style aliases.
	origTop := px(sharp.New(src), 5, 5)
	flipped := sharp.New(src).FlipVertical()
	fmt.Printf("   FlipVertical   : (5,5) was %v, bottom row now %v (magenta bar moved down)\n",
		origTop, px(flipped, 5, 194))
	flopped := sharp.New(src).FlipHorizontal()
	fmt.Printf("   FlipHorizontal : (0,100)=%v mirrors original (319,100)=%v\n",
		px(flopped, 0, 100), px(sharp.New(src), 319, 100))
	dims("Flip alias", sharp.New(src).Flip(), 320, 200)
	dims("Flop alias", sharp.New(src).Flop(), 320, 200)
	fmt.Println()
}

func sectionFormats(src image.Image) {
	header("5. Format conversion")

	// PNG and JPEG are checkable with the stdlib decoders.
	pngData, err := sharp.New(src).ResizeTo(120, 90).ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("out.png", pngData)
	check("out.png", pngData, 120, 90)

	jpgData, err := sharp.New(src).ResizeTo(120, 90).ToJPEG(85)
	if err != nil {
		log.Fatal(err)
	}
	save("out.jpg", jpgData)
	check("out.jpg", jpgData, 120, 90)

	// GIF, BMP and uncompressed TIFF are implemented in-package; verify them by
	// decoding back through sharp.
	gifData, err := sharp.New(src).ResizeTo(120, 90).ToGIF()
	if err != nil {
		log.Fatal(err)
	}
	save("out.gif", gifData)
	checkWithSharp("out.gif", gifData, 120, 90, sharp.FormatGIF)

	bmpData, err := sharp.New(src).ResizeTo(120, 90).ToBMP()
	if err != nil {
		log.Fatal(err)
	}
	save("out.bmp", bmpData)
	checkWithSharp("out.bmp", bmpData, 120, 90, sharp.FormatBMP)

	tiffData, err := sharp.New(src).ResizeTo(120, 90).ToTIFF()
	if err != nil {
		log.Fatal(err)
	}
	save("out.tiff", tiffData)
	checkWithSharp("out.tiff", tiffData, 120, 90, sharp.FormatTIFF)

	// ToFormat dispatches generically.
	for _, f := range []sharp.Format{sharp.FormatPNG, sharp.FormatJPEG, sharp.FormatGIF, sharp.FormatBMP, sharp.FormatTIFF} {
		data, err := sharp.New(src).ResizeTo(48, 32).ToFormat(f, 70)
		if err != nil {
			fmt.Printf("   FAIL ToFormat(%s): %v\n", f, err)
			continue
		}
		fmt.Printf("   PASS ToFormat(%-4s) -> %d bytes\n", f, len(data))
	}

	// ToFile writes straight to disk.
	fp := filepath.Join(outDir, "tofile.jpg")
	if err := sharp.New(src).ResizeTo(64, 40).ToFile(fp, sharp.FormatJPEG, 60); err != nil {
		log.Fatal(err)
	}
	onDisk, err := os.ReadFile(fp)
	if err != nil {
		log.Fatal(err)
	}
	check("tofile.jpg (ToFile)", onDisk, 64, 40)

	// HOLE: ToFile(FormatRaw) does not write raw pixel bytes. ToFormat routes
	// FormatRaw to ToRaw, but ToFile lumps FormatRaw into its PNG branch
	// (`case FormatPNG, FormatUnknown, FormatRaw: data, err = p.ToPNG()`), so
	// the file silently contains a PNG. The two "write in format X" entry
	// points therefore disagree for the same Format value.
	rawPath := filepath.Join(outDir, "tofile.raw")
	if err := sharp.New(src).ResizeTo(10, 10).ToFile(rawPath, sharp.FormatRaw, 0); err != nil {
		log.Fatal(err)
	}
	rawOnDisk, err := os.ReadFile(rawPath)
	if err != nil {
		log.Fatal(err)
	}
	viaToFormat, err := sharp.New(src).ResizeTo(10, 10).ToFormat(sharp.FormatRaw, 0)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   NOTE ToFile(FormatRaw) wrote %d bytes starting %q; ToFormat(FormatRaw) gave %d bytes (10*10*4=400)\n",
		len(rawOnDisk), rawOnDisk[:4], len(viaToFormat))
	if bytes.HasPrefix(rawOnDisk, []byte("\x89PNG")) {
		fmt.Println("   HOLE ToFile(FormatRaw) silently wrote a PNG, not raw pixels")
	}

	// HOLE: WebP and AVIF are not supported by this port. There is no ToWebP /
	// FormatWebP in the published API (the README states they are intentionally
	// out of scope because they would require a cgo/native encoder), so the
	// WebP part of the brief cannot be exercised:
	//
	//   webpData, err := sharp.New(src).ToWebP(80)   // does not exist
	//
	// sharp.FormatWebP is also absent; asking ToFormat for it is not even
	// expressible since Format is a string type — passing sharp.Format("webp")
	// compiles but returns `sharp: unsupported format "webp"`.
	if _, err := sharp.New(src).ToFormat(sharp.Format("webp"), 80); err != nil {
		fmt.Printf("   NOTE WebP unsupported (expected): %v\n", err)
	}
	fmt.Println()
}

func sectionQuality(src image.Image) {
	header("6. Quality and encoder options")

	// JPEG quality changes the encoded size monotonically.
	var sizes []int
	for _, q := range []int{20, 50, 80, 95} {
		data, err := sharp.New(src).ToJPEG(q)
		if err != nil {
			log.Fatal(err)
		}
		sizes = append(sizes, len(data))
		save(fmt.Sprintf("quality-q%d.jpg", q), data)
		fmt.Printf("   JPEG quality %2d -> %6d bytes\n", q, len(data))
	}
	increasing := true
	for i := 1; i < len(sizes); i++ {
		if sizes[i] <= sizes[i-1] {
			increasing = false
		}
	}
	if increasing {
		fmt.Println("   PASS encoded size increases with quality")
	} else {
		fmt.Println("   FAIL encoded size is not monotonic in quality")
	}

	// JPEGOptions is the struct form.
	optData, err := sharp.New(src).ToJPEGWithOptions(sharp.JPEGOptions{Quality: 40})
	if err != nil {
		log.Fatal(err)
	}
	check("ToJPEGWithOptions q40", optData, 320, 200)

	// PNG compression levels.
	for _, lvl := range []struct {
		name string
		c    png.CompressionLevel
	}{
		{"default", png.DefaultCompression},
		{"no-compression", png.NoCompression},
		{"best-speed", png.BestSpeed},
		{"best-compression", png.BestCompression},
	} {
		data, err := sharp.New(src).ToPNGWithOptions(sharp.PNGOptions{Compression: lvl.c})
		if err != nil {
			fmt.Printf("   FAIL PNG %s: %v\n", lvl.name, err)
			continue
		}
		fmt.Printf("   PNG %-17s -> %6d bytes\n", lvl.name, len(data))
	}

	// Out-of-range JPEG quality is clamped rather than rejected.
	if _, err := sharp.New(src).ToJPEG(500); err != nil {
		fmt.Printf("   NOTE ToJPEG(500) errored: %v\n", err)
	} else {
		fmt.Println("   PASS ToJPEG(500) clamped instead of erroring")
	}
	fmt.Println()
}

func sectionComposite(src image.Image) {
	header("7. Compositing and blend modes")

	overlay := makeOverlay(80)

	// Explicit offset placement.
	off := sharp.New(src).Composite(overlay, sharp.CompositeOptions{Left: 20, Top: 30, Opacity: 1})
	dims("Composite at (20,30)", off, 320, 200)
	fmt.Printf("        pixel(60,70) = %v (inside blue badge)\n", px(off, 60, 70))

	// Gravity placement — the base image keeps its size.
	for _, g := range []struct {
		name string
		g    sharp.Gravity
	}{
		{"top-left", sharp.GravityTopLeft},
		{"center", sharp.GravityCenter},
		{"bottom-right", sharp.GravityBottomRight},
	} {
		p := sharp.New(src).Composite(overlay, sharp.CompositeOptions{
			UseGravity: true, Gravity: g.g, Opacity: 1,
		})
		dims("Composite gravity "+g.name, p, 320, 200)
	}

	// Partial opacity.
	half := sharp.New(src).Composite(overlay, sharp.CompositeOptions{
		UseGravity: true, Gravity: sharp.GravityCenter, Opacity: 0.4,
	})
	fullOp := sharp.New(src).Composite(overlay, sharp.CompositeOptions{
		UseGravity: true, Gravity: sharp.GravityCenter, Opacity: 1,
	})
	fmt.Printf("   Opacity 0.4 centre pixel %v vs opacity 1.0 %v\n", px(half, 160, 100), px(fullOp, 160, 100))

	// Every blend mode, each encoded and dimension-checked.
	modes := []struct {
		name string
		m    sharp.BlendMode
	}{
		{"over", sharp.BlendOver}, {"multiply", sharp.BlendMultiply},
		{"screen", sharp.BlendScreen}, {"overlay", sharp.BlendOverlay},
		{"darken", sharp.BlendDarken}, {"lighten", sharp.BlendLighten},
		{"color-dodge", sharp.BlendColorDodge}, {"color-burn", sharp.BlendColorBurn},
		{"hard-light", sharp.BlendHardLight}, {"soft-light", sharp.BlendSoftLight},
		{"difference", sharp.BlendDifference}, {"exclusion", sharp.BlendExclusion},
		{"add", sharp.BlendAdd},
	}
	ok := 0
	for _, m := range modes {
		p := sharp.New(src).ResizeTo(120, 80).Composite(
			makeOverlay(60),
			sharp.CompositeOptions{UseGravity: true, Gravity: sharp.GravityCenter, Opacity: 1, Blend: m.m},
		)
		data, err := p.ToPNG()
		if err != nil {
			fmt.Printf("   FAIL blend %s: %v\n", m.name, err)
			continue
		}
		save("blend-"+m.name+".png", data)
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || cfg.Width != 120 || cfg.Height != 80 {
			fmt.Printf("   FAIL blend %s produced bad output\n", m.name)
			continue
		}
		ok++
	}
	fmt.Printf("   PASS %d/%d blend modes produced valid 120x80 PNGs\n", ok, len(modes))

	// Flatten composites the image over an opaque background, dropping alpha.
	transparent := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			transparent.Set(x, y, color.RGBA{255, 0, 0, 128})
		}
	}
	flat := sharp.New(transparent).Flatten(sharp.White)
	fmt.Printf("   Flatten(red@50%% over white) -> %v\n", px(flat, 10, 10))
	opaque, err := flat.IsOpaque()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   IsOpaque after Flatten = %v (expect true)\n", opaque)
	fmt.Println()
}

func sectionFilters(src image.Image) {
	header("8. Blur, sharpen and convolution filters")

	// Blur reduces high-frequency detail: Sharpness() should drop.
	base := sharp.New(src).ResizeTo(160, 100)
	baseSharp, err := base.Clone().Sharpness()
	if err != nil {
		log.Fatal(err)
	}
	blurred := base.Clone().Blur(3.0)
	blurSharp, err := blurred.Clone().Sharpness()
	if err != nil {
		log.Fatal(err)
	}
	sharpened := base.Clone().Sharpen(1.5)
	shSharp, err := sharpened.Clone().Sharpness()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Sharpness: original %.2f, Blur(3.0) %.2f, Sharpen(1.5) %.2f\n", baseSharp, blurSharp, shSharp)
	if blurSharp < baseSharp {
		fmt.Println("   PASS Blur lowered the sharpness metric")
	} else {
		fmt.Println("   FAIL Blur did not lower the sharpness metric")
	}
	if shSharp > baseSharp {
		fmt.Println("   PASS Sharpen raised the sharpness metric")
	} else {
		fmt.Println("   FAIL Sharpen did not raise the sharpness metric")
	}
	blurData, err := blurred.Clone().ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("blur.png", blurData)
	check("blur.png", blurData, 160, 100)

	// Unsharp mask with explicit sigma/amount/threshold.
	un := base.Clone().Unsharp(sharp.UnsharpOptions{Sigma: 1.2, Amount: 1.0, Threshold: 4})
	dims("Unsharp", un, 160, 100)

	// A custom 3x3 edge-detect kernel.
	k, err := sharp.NewKernel([]float64{
		-1, -1, -1,
		-1, 8, -1,
		-1, -1, -1,
	}, 1, 0)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   NewKernel 3x3 edge-detect: size=%d divisor=%.1f offset=%.1f\n", k.Size, k.Divisor, k.Offset)
	edges := base.Clone().Convolve(k)
	edgeData, err := edges.ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("convolve-edges.png", edgeData)
	check("convolve-edges.png", edgeData, 160, 100)

	// A box blur kernel via divisor normalisation.
	box, err := sharp.NewKernel([]float64{1, 1, 1, 1, 1, 1, 1, 1, 1}, 9, 0)
	if err != nil {
		log.Fatal(err)
	}
	dims("Convolve box blur", base.Clone().Convolve(box), 160, 100)

	// Non-square kernels are rejected.
	if _, err := sharp.NewKernel([]float64{1, 2, 3, 4, 5, 6}, 1, 0); err != nil {
		fmt.Printf("   PASS NewKernel rejects non-square weights: %v\n", err)
	}

	// Median and morphology.
	noisy := makeNoisyImage(120, 90)
	dims("Median(2)", sharp.New(noisy).Median(2), 120, 90)
	for _, m := range []struct {
		name string
		op   sharp.MorphOp
	}{
		{"erode", sharp.MorphErode}, {"dilate", sharp.MorphDilate},
		{"open", sharp.MorphOpen}, {"close", sharp.MorphClose},
	} {
		dims("Morphology "+m.name, sharp.New(noisy).Morphology(m.op, 1), 120, 90)
	}
	dims("Erode helper", sharp.New(noisy).Erode(1), 120, 90)
	dims("Dilate helper", sharp.New(noisy).Dilate(1), 120, 90)
	fmt.Println()
}

func makeNoisyImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	next := func() uint32 { seed = seed*1664525 + 1013904223; return seed }
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			base := uint8(180)
			if (x/10+y/10)%2 == 0 {
				base = 60
			}
			n := int(next()>>24) % 60
			v := int(base) + n - 30
			if v < 0 {
				v = 0
			}
			if v > 255 {
				v = 255
			}
			img.Set(x, y, color.RGBA{uint8(v), uint8(v), uint8(v), 255})
		}
	}
	return img
}

func sectionTone(src image.Image) {
	header("9. Colour and tone adjustments")

	p := func() *sharp.Pipeline { return sharp.New(src).ResizeTo(80, 50) }

	fmt.Printf("   original(20,40)   = %v\n", px(p(), 20, 40))
	fmt.Printf("   Grayscale         = %v\n", px(p().Grayscale(), 20, 40))
	fmt.Printf("   Negate            = %v\n", px(p().Negate(), 20, 40))
	fmt.Printf("   Invert (alias)    = %v\n", px(p().Invert(), 20, 40))
	fmt.Printf("   Sepia             = %v\n", px(p().Sepia(), 20, 40))
	fmt.Printf("   Tint(orange)      = %v\n", px(p().Tint(sharp.RGBA{R: 255, G: 140, B: 0, A: 255}), 20, 40))
	fmt.Printf("   Brightness(1.4)   = %v\n", px(p().Brightness(1.4), 20, 40))
	fmt.Printf("   Contrast(1.6)     = %v\n", px(p().Contrast(1.6), 20, 40))
	fmt.Printf("   Gamma(2.2)        = %v\n", px(p().Gamma(2.2), 20, 40))
	fmt.Printf("   Saturation(0.2)   = %v\n", px(p().Saturation(0.2), 20, 40))
	fmt.Printf("   Threshold(128)    = %v\n", px(p().Threshold(128), 20, 40))
	fmt.Printf("   Luma helper       = %d\n", sharp.Luma(120, 200, 40))

	// Modulate applies brightness/saturation/hue/lightness in HSL space.
	mod := p().Modulate(sharp.ModulateOptions{Brightness: 1.1, Saturation: 1.3, Hue: 45, Lightness: 5})
	fmt.Printf("   Modulate          = %v\n", px(mod, 20, 40))

	// Linear applies per-channel a*x+b.
	lin := p().Linear([]float64{1.2, 1.0, 0.8}, []float64{10, 0, -10})
	fmt.Printf("   Linear            = %v\n", px(lin, 20, 40))

	// Normalise stretches the histogram to the full range.
	lowContrast := makeLowContrastImage(80, 50)
	before, err := sharp.New(lowContrast).ChannelStats()
	if err != nil {
		log.Fatal(err)
	}
	norm := sharp.New(lowContrast).Normalise(sharp.NormaliseOptions{Lower: 1, Upper: 99})
	after, err := norm.Clone().ChannelStats()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Normalise: R range %d-%d -> %d-%d\n",
		before[0].Min, before[0].Max, after[0].Min, after[0].Max)
	if after[0].Max-after[0].Min > before[0].Max-before[0].Min {
		fmt.Println("   PASS Normalise widened the dynamic range")
	} else {
		fmt.Println("   FAIL Normalise did not widen the dynamic range")
	}
	dims("Normalize (alias)", sharp.New(lowContrast).Normalize(sharp.NormaliseOptions{}), 80, 50)

	// CLAHE — local contrast enhancement.
	clahe := sharp.New(lowContrast).CLAHE(sharp.CLAHEOptions{Width: 16, Height: 16, MaxSlope: 3})
	dims("CLAHE 16x16 slope 3", clahe, 80, 50)

	// Colourspace conversions.
	dims("ToColourspace(b-w)", p().ToColourspace("b-w"), 80, 50)
	dims("ToColorspace(srgb)", p().ToColorspace("srgb"), 80, 50)

	// Unflatten turns white pixels transparent.
	white := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := range white.Pix {
		white.Pix[i] = 255
	}
	fmt.Printf("   Unflatten(white)  = %v (alpha should be 0)\n", px(sharp.New(white).Unflatten(), 5, 5))

	// Standalone colour-space helper functions.
	h, s, v := sharp.RGBToHSV(255, 100, 50)
	rr, gg, bb := sharp.HSVToRGB(h, s, v)
	fmt.Printf("   RGBToHSV(255,100,50) = (%.1f, %.2f, %.2f) -> back to (%d,%d,%d)\n", h, s, v, rr, gg, bb)
	x, y, z := sharp.RGBToXYZ(255, 255, 255)
	fmt.Printf("   RGBToXYZ(white)      = (%.3f, %.3f, %.3f)\n", x, y, z)
	xr, xg, xb := sharp.XYZToRGB(x, y, z)
	fmt.Printf("   XYZToRGB round-trip  = (%d,%d,%d)\n", xr, xg, xb)
	l, la, lb := sharp.RGBToLab(120, 60, 200)
	fmt.Printf("   RGBToLab(120,60,200) = (%.2f, %.2f, %.2f)\n", l, la, lb)
	lr, lg, lbl := sharp.LabToRGB(l, la, lb)
	fmt.Printf("   LabToRGB round-trip  = (%d,%d,%d)\n", lr, lg, lbl)
	l2, a2, b2 := sharp.RGBToLab(125, 60, 200)
	fmt.Printf("   DeltaE76             = %.3f\n", sharp.DeltaE76(l, la, lb, l2, a2, b2))
	fmt.Println()
}

func makeLowContrastImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Everything squeezed into the 100-150 band.
			v := uint8(100 + 50*x/(w-1))
			img.Set(x, y, color.RGBA{v, v, uint8(int(v) - 10), 255})
		}
	}
	return img
}

func sectionChannels(src image.Image) {
	header("10. Channel operations")

	p := func() *sharp.Pipeline { return sharp.New(src).ResizeTo(60, 40) }

	for _, c := range []struct {
		name string
		ch   sharp.Channel
	}{
		{"R", sharp.ChannelR}, {"G", sharp.ChannelG}, {"B", sharp.ChannelB}, {"A", sharp.ChannelA},
	} {
		fmt.Printf("   ExtractChannel(%s) pixel(15,30) = %v\n", c.name, px(p().ExtractChannel(c.ch), 15, 30))
	}

	// ExtractChannel(A) is only meaningful when alpha actually varies, so use
	// the semi-transparent badge overlay as the source.
	badge := sharp.New(makeOverlay(40))
	fmt.Printf("   ExtractChannel(A) on badge: centre=%v edge=%v (alpha 200 vs 0)\n",
		px(badge.Clone().ExtractChannel(sharp.ChannelA), 20, 20),
		px(badge.Clone().ExtractChannel(sharp.ChannelA), 0, 0))

	// HOLE: Boolean with BoolXor also XORs the alpha channel, so combining two
	// fully opaque images (255^255 == 0) yields a completely transparent result.
	// It is documented ("Alpha is combined with the same operator") but makes
	// BoolXor unusable on opaque images without a follow-up EnsureAlpha/Flatten.
	xorAlpha := px(p().Boolean(solidGray(60, 40, 0x0F), sharp.BoolXor), 15, 30)
	fmt.Printf("   NOTE Boolean(xor) alpha = %d (opaque^opaque wipes alpha)\n", xorAlpha.A)

	// RemoveAlpha / EnsureAlpha.
	noAlpha := p().RemoveAlpha()
	mdNo, err := noAlpha.Clone().Metadata()
	if err != nil {
		log.Fatal(err)
	}
	withAlpha := p().EnsureAlpha(0.5)
	mdWith, err := withAlpha.Clone().Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   RemoveAlpha  -> channels=%d hasAlpha=%v\n", mdNo.Channels, mdNo.HasAlpha)
	fmt.Printf("   EnsureAlpha(0.5) -> channels=%d hasAlpha=%v pixel=%v\n", mdWith.Channels, mdWith.HasAlpha, px(withAlpha, 15, 30))

	// JoinChannels builds an RGB image from three grayscale planes.
	r := solidGray(40, 30, 220)
	g := solidGray(40, 30, 120)
	b := solidGray(40, 30, 40)
	joined := sharp.JoinChannels(r, g, b)
	dims("JoinChannels(3 planes)", joined, 40, 30)
	fmt.Printf("        pixel = %v (expect 220,120,40)\n", px(joined, 10, 10))

	// Recomb applies a 3x3 (or 4x4) colour matrix — here a sepia matrix.
	recomb := p().Recomb([][]float64{
		{0.393, 0.769, 0.189},
		{0.349, 0.686, 0.168},
		{0.272, 0.534, 0.131},
	})
	fmt.Printf("   Recomb(sepia matrix) = %v\n", px(recomb, 15, 30))

	// Boolean combines two images bitwise; Bandbool folds the channels.
	other := solidGray(60, 40, 0x0F)
	for _, op := range []struct {
		name string
		op   sharp.BoolOp
	}{
		{"and", sharp.BoolAnd}, {"or", sharp.BoolOr}, {"xor", sharp.BoolXor},
	} {
		fmt.Printf("   Boolean %-3s pixel = %v\n", op.name, px(p().Boolean(other, op.op), 15, 30))
	}
	fmt.Printf("   Bandbool(and) pixel = %v\n", px(p().Bandbool(sharp.BoolAnd), 15, 30))
	fmt.Println()
}

func solidGray(w, h int, v uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4] = v
		img.Pix[i*4+1] = v
		img.Pix[i*4+2] = v
		img.Pix[i*4+3] = 255
	}
	return img
}

func sectionStats(src image.Image) {
	header("11. Statistics and analysis")

	p := sharp.New(src)

	st, err := p.Clone().Stats()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Stats means      : R=%.1f G=%.1f B=%.1f A=%.1f\n", st.MeanR, st.MeanG, st.MeanB, st.MeanA)

	cs, err := p.Clone().ChannelStats()
	if err != nil {
		log.Fatal(err)
	}
	names := []string{"R", "G", "B", "A"}
	for i, c := range cs {
		fmt.Printf("   Channel %s        : min=%3d max=%3d mean=%7.2f stddev=%6.2f sum=%.0f\n",
			names[i], c.Min, c.Max, c.Mean, c.StdDev, c.Sum)
	}

	hist, err := p.Clone().Histogram()
	if err != nil {
		log.Fatal(err)
	}
	var total uint64
	for _, n := range hist.Luminance {
		total += n
	}
	fmt.Printf("   Histogram        : luminance bins sum to %d (expect %d pixels)\n", total, 320*200)
	if total == uint64(320*200) {
		fmt.Println("   PASS histogram counts every pixel")
	} else {
		fmt.Println("   FAIL histogram count mismatch")
	}

	ent, err := p.Clone().Entropy()
	if err != nil {
		log.Fatal(err)
	}
	flatEnt, err := sharp.New(solidGray(50, 50, 128)).Entropy()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Entropy          : gradient %.4f vs flat grey %.4f\n", ent, flatEnt)
	if ent > flatEnt {
		fmt.Println("   PASS gradient has higher entropy than a flat image")
	} else {
		fmt.Println("   FAIL entropy ordering unexpected")
	}

	sh, err := p.Clone().Sharpness()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Sharpness        : %.4f\n", sh)

	op, err := p.Clone().IsOpaque()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   IsOpaque         : %v\n", op)

	dom, err := p.Clone().DominantColor()
	if err != nil {
		log.Fatal(err)
	}
	domUK, err := p.Clone().DominantColour()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   DominantColor    : %+v (Colour alias %+v)\n", dom, domUK)

	mean, err := p.Clone().MeanColor()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   MeanColor        : %+v\n", mean)
	fmt.Println()
}

func sectionRawRoundTrip(src image.Image) {
	header("12. Raw pixel buffers")

	raw, err := sharp.New(src).ResizeTo(40, 30).ToRaw()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   ToRaw            : %d bytes (expect 40*30*4 = %d)\n", len(raw), 40*30*4)

	back := sharp.FromRaw(raw, 40, 30, 4)
	dims("FromRaw(RGBA)", back, 40, 30)

	// Verify the round-trip is byte-exact.
	raw2, err := back.Clone().ToRaw()
	if err != nil {
		log.Fatal(err)
	}
	if bytes.Equal(raw, raw2) {
		fmt.Println("   PASS raw round-trip is byte-identical")
	} else {
		fmt.Println("   FAIL raw round-trip differs")
	}

	// Grayscale and RGB raw inputs.
	gray := make([]byte, 20*10)
	for i := range gray {
		gray[i] = uint8(i % 256)
	}
	dims("FromRaw(1 channel)", sharp.FromRaw(gray, 20, 10, 1), 20, 10)
	rgb := make([]byte, 20*10*3)
	for i := range rgb {
		rgb[i] = uint8(i % 256)
	}
	dims("FromRaw(3 channels)", sharp.FromRaw(rgb, 20, 10, 3), 20, 10)

	// Wrong length is reported through Err().
	if err := sharp.FromRaw(gray, 99, 99, 1).Err(); err != nil {
		fmt.Printf("   PASS FromRaw rejects a bad length: %v\n", err)
	}
	fmt.Println()
}

func sectionGeometryExtras(src image.Image) {
	header("13. Affine transforms and Trim")

	// A shear + scale affine matrix [a b c d e f].
	aff := sharp.New(src).ResizeTo(120, 80).Affine(
		[6]float64{1.0, 0.3, 0, 0.0, 1.0, 0},
		sharp.AffineOptions{Background: sharp.White},
	)
	md, err := aff.Clone().Metadata()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Affine shear     : 120x80 -> %dx%d (canvas grew as expected: %v)\n",
		md.Width, md.Height, md.Width > 120)
	affData, err := aff.ToPNG()
	if err != nil {
		log.Fatal(err)
	}
	save("affine-shear.png", affData)
	check("affine-shear.png", affData, md.Width, md.Height)

	// A singular matrix is rejected.
	if err := sharp.New(src).Affine([6]float64{0, 0, 0, 0, 0, 0}, sharp.AffineOptions{}).Err(); err != nil {
		fmt.Printf("   PASS Affine rejects a singular matrix: %v\n", err)
	}

	// Trim removes the uniform 20px border from a 140x100 image.
	bordered := makeBorderedImage(140, 100, 20)
	trimmed := sharp.New(bordered).Trim(10)
	dims("Trim(border 20px)", trimmed, 100, 60)
	fmt.Println()
}

func sectionErrors() {
	header("14. Error propagation")

	// Errors are deferred and surface at the terminal call.
	p := sharp.New(nil)
	if err := p.Err(); err != nil {
		fmt.Printf("   New(nil)             -> %v\n", err)
	}

	if err := sharp.FromBytes([]byte("not an image")).Err(); err != nil {
		fmt.Printf("   FromBytes(garbage)   -> %v\n", err)
	}

	if err := sharp.FromFile("/nonexistent/nope.png").Err(); err != nil {
		fmt.Printf("   FromFile(missing)    -> %v\n", err)
	}

	// An error early in a chain short-circuits everything after it.
	bad := sharp.New(makeSourceImage(20, 20)).
		Resize(sharp.ResizeOptions{Width: -5}).
		Blur(2).
		Rotate90()
	if _, err := bad.ToPNG(); err != nil {
		fmt.Printf("   chained after error  -> %v\n", err)
	}

	// Extract outside the bounds.
	if err := sharp.New(makeSourceImage(20, 20)).
		Extract(sharp.Rectangle{X: 15, Y: 15, Width: 50, Height: 50}).Err(); err != nil {
		fmt.Printf("   Extract out of range -> %v\n", err)
	}

	// Clone gives an independent pipeline.
	orig := sharp.New(makeSourceImage(40, 40))
	clone := orig.Clone().Grayscale()
	fmt.Printf("   Clone independence   : original %v, clone %v\n", px(orig, 20, 20), px(clone, 20, 20))
	fmt.Println()
}
