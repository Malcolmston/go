// Command opencv-example exercises the pure-Go OpenCV port
// (github.com/malcolmston/opencv) end to end without touching the network or
// requiring any input image: every input is synthesised in memory.
//
// Run with:
//
//	GOWORK=off go run .
//
// Output images are written into an "out" directory next to this program.
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	cv "github.com/malcolmston/opencv"
	"github.com/malcolmston/opencv/aruco"
	"github.com/malcolmston/opencv/barcode"
	"github.com/malcolmston/opencv/features2d"
	"github.com/malcolmston/opencv/imghash"
	"github.com/malcolmston/opencv/ml"
	"github.com/malcolmston/opencv/objdetect"
	"github.com/malcolmston/opencv/photo"
	"github.com/malcolmston/opencv/plot"
	"github.com/malcolmston/opencv/quality"
	"github.com/malcolmston/opencv/segmentation"
	"github.com/malcolmston/opencv/ximgproc"
)

var outDir string

func main() {
	var err error
	outDir, err = filepath.Abs("out")
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}
	fmt.Printf("output directory: %s\n", outDir)

	scene := buildScene(240, 320)
	save("01-scene.png", scene)

	sectionMatCore(scene)
	sectionIO(scene)
	sectionColor(scene)
	sectionFilters(scene)
	sectionThreshold(scene)
	sectionMorphology(scene)
	sectionGeometry(scene)
	sectionContours(scene)
	sectionConnected(scene)
	sectionFeatures(scene)
	sectionHistogram(scene)

	sectionQuality(scene)
	sectionImgHash(scene)
	sectionPhoto(scene)
	sectionXImgProc(scene)
	sectionSegmentation(scene)
	sectionFeatures2D()
	sectionObjDetect(scene)
	sectionAruco()
	sectionBarcode()
	sectionPlot()
	sectionML()

	fmt.Println("\n== done ==")
}

// ---------------------------------------------------------------- helpers ----

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fatal:", err)
	os.Exit(1)
}

func header(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func save(name string, m *cv.Mat) {
	if err := cv.ImWrite(filepath.Join(outDir, name), m); err != nil {
		fatal(fmt.Errorf("ImWrite %s: %w", name, err))
	}
}

func describe(label string, m *cv.Mat) {
	fmt.Printf("  %-28s %dx%d, %d ch, %d bytes\n", label, m.Cols, m.Rows, m.Channels, len(m.Data))
}

// buildScene paints a deterministic RGB test image: a vertical gradient
// background, three solid shapes, a diagonal line and a text label. Shapes have
// distinct colours so colour conversion, thresholding, contour finding and
// connected components all have something real to chew on.
func buildScene(rows, cols int) *cv.Mat {
	m := cv.NewMat(rows, cols, 3)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			g := uint8(30 + 120*y/rows)
			m.SetPixel(y, x, []uint8{g, g / 2, uint8(200 - 120*x/cols)})
		}
	}
	// Bright square, bright disc, bright triangle.
	cv.Rectangle(m, cv.Point{X: 30, Y: 30}, cv.Point{X: 100, Y: 100}, cv.NewScalar(240, 240, 240), cv.Filled)
	// Kept deliberately light as well as saturated: a pure (250,40,40) red grays
	// to ~103, which is almost exactly the background luma here, so it would be
	// invisible to every luminance-based operator.
	cv.Circle(m, cv.Point{X: 200, Y: 70}, 38, cv.NewScalar(255, 110, 110), cv.Filled)
	cv.FillPoly(m, [][]cv.Point{{
		{X: 120, Y: 200}, {X: 190, Y: 200}, {X: 155, Y: 140},
	}}, cv.NewScalar(40, 250, 90))
	cv.Line(m, cv.Point{X: 0, Y: 235}, cv.Point{X: 319, Y: 180}, cv.NewScalar(255, 255, 0), 2)
	cv.PutText(m, "CV PORT", cv.Point{X: 210, Y: 210}, 1, cv.NewScalar(255, 255, 255))
	return m
}

// ------------------------------------------------------------- mat / core ----

func sectionMatCore(scene *cv.Mat) {
	header("Mat core")
	describe("scene", scene)
	rows, cols := scene.Size()
	fmt.Printf("  Size()=%dx%d Total()=%d Empty()=%v\n", rows, cols, scene.Total(), scene.Empty())
	fmt.Printf("  AtPixel(70,200)=%v At(70,200,0)=%d\n", scene.AtPixel(70, 200), scene.At(70, 200, 0))

	// Split into planes, zero the green plane, merge back.
	planes := scene.Split()
	fmt.Printf("  Split() -> %d planes, each %dx%d/%dch\n",
		len(planes), planes[0].Cols, planes[0].Rows, planes[0].Channels)
	planes[1] = cv.NewMat(scene.Rows, scene.Cols, 1)
	noGreen := cv.Merge(planes)
	describe("Merge(R,0,B)", noGreen)
	save("02-no-green.png", noGreen)

	// Region view + CopyTo compositing.
	roi := scene.Region(30, 30, 71, 71)
	describe("Region(30,30,71,71)", roi)
	canvas := cv.NewMat(120, 240, 3)
	canvas.SetTo(20)
	roi.CopyTo(canvas, 25, 10)
	roi.CopyTo(canvas, 25, 150)
	describe("two ROI pastes", canvas)
	save("03-roi-paste.png", canvas)

	// Standard-library bridge.
	img := scene.ToImage()
	back := cv.FromImage(img)
	fmt.Printf("  ToImage bounds=%v -> FromImage %dx%d/%dch (round-trip equal: %v)\n",
		img.Bounds(), back.Cols, back.Rows, back.Channels, equalMat(scene, back))
}

func equalMat(a, b *cv.Mat) bool {
	if a.Rows != b.Rows || a.Cols != b.Cols || a.Channels != b.Channels {
		return false
	}
	for i := range a.Data {
		if a.Data[i] != b.Data[i] {
			return false
		}
	}
	return true
}

// -------------------------------------------------------------------- IO ----

func sectionIO(scene *cv.Mat) {
	header("I/O (PNG / JPEG, files and buffers)")
	png, err := cv.IMEncode("png", scene)
	if err != nil {
		fatal(err)
	}
	jpg, err := cv.IMEncode("jpeg", scene)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  IMEncode png=%d bytes, jpeg=%d bytes\n", len(png), len(jpg))

	dec, err := cv.IMDecode(png)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  IMDecode(png) -> %dx%d/%dch, lossless round-trip: %v\n",
		dec.Cols, dec.Rows, dec.Channels, equalMat(scene, dec))

	path := filepath.Join(outDir, "04-roundtrip.png")
	if err := cv.ImWrite(path, scene); err != nil {
		fatal(err)
	}
	read, err := cv.ImRead(path)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("  ImWrite+ImRead round-trip equal: %v\n", equalMat(scene, read))
}

// ----------------------------------------------------------------- colour ----

func sectionColor(scene *cv.Mat) {
	header("Colour conversions + InRange")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	describe("ColorRGB2Gray", gray)
	save("05-gray.png", gray)

	for _, tc := range []struct {
		name     string
		fwd, inv cv.ColorConversionCode
	}{
		{"HSV", cv.ColorRGB2HSV, cv.ColorHSV2RGB},
		{"Lab", cv.ColorRGB2Lab, cv.ColorLab2RGB},
		{"YCrCb", cv.ColorRGB2YCrCb, cv.ColorYCrCb2RGB},
		{"HLS", cv.ColorRGB2HLS, cv.ColorHLS2RGB},
	} {
		conv := cv.CvtColor(scene, tc.fwd)
		back := cv.CvtColor(conv, tc.inv)
		fmt.Printf("  RGB->%-5s->RGB max abs round-trip error: %d\n", tc.name, maxAbsDiff(scene, back))
	}

	// Isolate the red disc in HSV space.
	hsv := cv.CvtColor(scene, cv.ColorRGB2HSV)
	redMask := cv.BitwiseOr(
		cv.InRange(hsv, []uint8{0, 120, 80}, []uint8{10, 255, 255}),
		cv.InRange(hsv, []uint8{170, 120, 80}, []uint8{179, 255, 255}),
	)
	fmt.Printf("  InRange red mask: %d of %d pixels set (disc area ~%d)\n",
		countNonZero(redMask), redMask.Total(), int(math.Round(math.Pi*38*38)))
	save("06-red-mask.png", redMask)
}

func maxAbsDiff(a, b *cv.Mat) int {
	d := cv.AbsDiff(a, b)
	worst := 0
	for _, v := range d.Data {
		if int(v) > worst {
			worst = int(v)
		}
	}
	return worst
}

func countNonZero(m *cv.Mat) int {
	n := 0
	for _, v := range m.Data {
		if v != 0 {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- filters ----

func sectionFilters(scene *cv.Mat) {
	header("Filtering / convolution")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	noisy := addSaltPepper(gray, 0.06)
	save("07-noisy.png", noisy)

	blur := cv.Blur(noisy, 5)
	box := cv.BoxFilter(noisy, 5, true)
	gauss := cv.GaussianBlur(noisy, 5, 1.4)
	med := cv.MedianBlur(noisy, 5)
	bil := cv.BilateralFilter(noisy, 5, 40, 5)
	fmt.Printf("  variance: noisy=%.1f blur=%.1f box=%.1f gauss=%.1f median=%.1f bilateral=%.1f\n",
		variance(noisy), variance(blur), variance(box), variance(gauss), variance(med), variance(bil))
	save("08-median.png", med)
	save("09-bilateral.png", bil)

	fmt.Printf("  GaussianKernel1D(5,1.4) = %v\n", round3(cv.GaussianKernel1D(5, 1.4)))

	// Custom 3x3 sharpening kernel through the generic Filter2D path, and the
	// same box blur expressed separably.
	sharp := cv.Filter2D(gray, cv.NewKernel(3, 3, []float64{
		0, -1, 0,
		-1, 5, -1,
		0, -1, 0,
	}), 0)
	describe("Filter2D sharpen", sharp)
	save("10-sharpen.png", sharp)

	k := []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	sep := cv.Filter2DSep(gray, k, k, 0)
	fmt.Printf("  Filter2DSep(3x3 box) vs BoxFilter(3): max abs diff %d\n",
		maxAbsDiff(sep, cv.BoxFilter(gray, 3, true)))

	sx := cv.Sobel(gray, 1, 0, 3, 1, 0)
	sy := cv.Sobel(gray, 0, 1, 3, 1, 0)
	mag := cv.AddWeighted(sx, 0.5, sy, 0.5, 0)
	save("11-sobel.png", mag)
	describe("Sobel dx+dy", mag)
	describe("Scharr dx", cv.Scharr(gray, 1, 0, 1, 0))
	describe("Laplacian", cv.Laplacian(gray, 3, 1, 0))

	fmt.Printf("  SobelFloat -> %d channel plane(s) of raw (unsaturated) gradients\n",
		len(cv.SobelFloat(gray, 1, 0, 3)))
}

func addSaltPepper(gray *cv.Mat, frac float64) *cv.Mat {
	out := gray.Clone()
	// Deterministic LCG so runs are reproducible.
	var state uint64 = 0x2545F4914F6CDD1D
	n := int(float64(out.Total()) * frac)
	for i := 0; i < n; i++ {
		state = state*6364136223846793005 + 1442695040888963407
		idx := int((state >> 33) % uint64(len(out.Data)))
		if state&1 == 0 {
			out.Data[idx] = 0
		} else {
			out.Data[idx] = 255
		}
	}
	return out
}

func variance(m *cv.Mat) float64 {
	var sum, sumSq float64
	for _, v := range m.Data {
		sum += float64(v)
		sumSq += float64(v) * float64(v)
	}
	n := float64(len(m.Data))
	mean := sum / n
	return sumSq/n - mean*mean
}

func round3(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = math.Round(x*1000) / 1000
	}
	return out
}

// ------------------------------------------------------------- threshold -----

func sectionThreshold(scene *cv.Mat) {
	header("Thresholding")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)

	fixed, used := cv.Threshold(gray, 128, 255, cv.ThreshBinary)
	fmt.Printf("  fixed  thresh=%.0f -> %d set\n", used, countNonZero(fixed))

	otsu, level := cv.Threshold(gray, 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	fmt.Printf("  Otsu   thresh=%.0f -> %d set\n", level, countNonZero(otsu))
	save("12-otsu.png", otsu)

	for _, tc := range []struct {
		name string
		typ  cv.ThresholdType
	}{
		{"ThreshBinaryInv", cv.ThreshBinaryInv},
		{"ThreshTrunc", cv.ThreshTrunc},
		{"ThreshToZero", cv.ThreshToZero},
		{"ThreshToZeroInv", cv.ThreshToZeroInv},
	} {
		out, _ := cv.Threshold(gray, 128, 255, tc.typ)
		fmt.Printf("  %-16s nonzero=%d\n", tc.name, countNonZero(out))
	}

	adMean := cv.AdaptiveThreshold(gray, 255, cv.AdaptiveThreshMeanC, cv.ThreshBinary, 11, 2)
	adGauss := cv.AdaptiveThreshold(gray, 255, cv.AdaptiveThreshGaussianC, cv.ThreshBinary, 11, 2)
	fmt.Printf("  adaptive mean nonzero=%d, gaussian nonzero=%d\n",
		countNonZero(adMean), countNonZero(adGauss))
	save("13-adaptive.png", adGauss)
}

// ------------------------------------------------------------ morphology -----

func sectionMorphology(scene *cv.Mat) {
	header("Morphology")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	bin, _ := cv.Threshold(gray, 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	speckled := addSaltPepper(bin, 0.04)

	for _, sh := range []struct {
		name  string
		shape cv.MorphShape
	}{
		{"MorphRect", cv.MorphRect},
		{"MorphCross", cv.MorphCross},
		{"MorphEllipse", cv.MorphEllipse},
	} {
		el := cv.GetStructuringElement(sh.shape, 5, 5)
		fmt.Printf("  %-13s 5x5 element, %d/%d cells set\n", sh.name, countNonZero(el), el.Total())
	}

	el := cv.GetStructuringElement(cv.MorphEllipse, 5, 5)
	fmt.Printf("  nonzero: speckled=%d erode=%d dilate=%d\n",
		countNonZero(speckled), countNonZero(cv.Erode(speckled, el, 1)), countNonZero(cv.Dilate(speckled, el, 1)))
	for _, op := range []struct {
		name string
		op   cv.MorphType
	}{
		{"MorphOpen", cv.MorphOpen},
		{"MorphClose", cv.MorphClose},
		{"MorphGradient", cv.MorphGradient},
		{"MorphTophat", cv.MorphTophat},
		{"MorphBlackhat", cv.MorphBlackhat},
	} {
		out := cv.MorphologyEx(speckled, el, op.op, 1)
		fmt.Printf("  %-14s nonzero=%d\n", op.name, countNonZero(out))
	}
	save("14-morph-open.png", cv.MorphologyEx(speckled, el, cv.MorphOpen, 1))
	save("15-morph-gradient.png", cv.MorphologyEx(bin, el, cv.MorphGradient, 1))
}

// -------------------------------------------------------------- geometry -----

func sectionGeometry(scene *cv.Mat) {
	header("Geometric transforms")
	describe("Resize nearest 160x120", cv.Resize(scene, 160, 120, cv.InterNearest))
	half := cv.Resize(scene, 160, 120, cv.InterLinear)
	describe("Resize bilinear 160x120", half)
	save("16-resize.png", half)

	describe("Flip horizontal", cv.Flip(scene, cv.FlipHorizontal))
	describe("Flip both", cv.Flip(scene, cv.FlipBoth))
	describe("Rotate 90CW", cv.Rotate(scene, cv.Rotate90CW))
	describe("Transpose", cv.Transpose(scene))

	m := cv.GetRotationMatrix2D(160, 120, 30, 0.9)
	fmt.Printf("  GetRotationMatrix2D(160,120,30deg,0.9) = %v\n", round3(m[:]))
	warped := cv.WarpAffine(scene, m, 320, 240, cv.InterLinear)
	save("17-warp-affine.png", warped)
	describe("WarpAffine", warped)

	src := [4]cv.Point{{X: 0, Y: 0}, {X: 319, Y: 0}, {X: 319, Y: 239}, {X: 0, Y: 239}}
	dst := [4]cv.Point{{X: 40, Y: 20}, {X: 300, Y: 0}, {X: 319, Y: 220}, {X: 10, Y: 239}}
	pm := cv.GetPerspectiveTransform(src, dst)
	persp := cv.WarpPerspective(scene, pm, 320, 240, cv.InterLinear)
	describe("WarpPerspective", persp)
	save("18-warp-perspective.png", persp)

	// Remap: a barrel-ish swirl built from explicit float maps.
	mapX := cv.NewFloatMat(240, 320)
	mapY := cv.NewFloatMat(240, 320)
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			mapX.Data[y*320+x] = float64(x) + 8*math.Sin(float64(y)/18)
			mapY.Data[y*320+x] = float64(y)
		}
	}
	remapped := cv.Remap(scene, mapX, mapY, cv.InterLinear)
	describe("Remap (wave)", remapped)
	save("19-remap.png", remapped)

	down := cv.PyrDown(scene)
	up := cv.PyrUp(down)
	describe("PyrDown", down)
	describe("PyrUp(PyrDown)", up)

	bin, _ := cv.Threshold(cv.CvtColor(scene, cv.ColorRGB2Gray), 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	dt := cv.DistanceTransform(bin)
	_, maxDist, _, _, mx, my := cv.MinMaxLoc(dt)
	fmt.Printf("  DistanceTransform: max=%.2f at (%d,%d)\n", maxDist, mx, my)
}

// -------------------------------------------------------------- contours -----

func sectionContours(scene *cv.Mat) {
	header("Contours & shape")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	bin, _ := cv.Threshold(gray, 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	clean := cv.MorphologyEx(bin, cv.GetStructuringElement(cv.MorphEllipse, 5, 5), cv.MorphOpen, 1)

	for _, mode := range []struct {
		name string
		m    cv.RetrievalMode
	}{
		{"RetrExternal", cv.RetrExternal},
		{"RetrList", cv.RetrList},
		{"RetrTree", cv.RetrTree},
	} {
		cs, hier := cv.FindContours(clean, mode.m, cv.ChainApproxSimple)
		fmt.Printf("  %-13s -> %d contours, %d hierarchy nodes\n", mode.name, len(cs), len(hier))
	}

	contours, _ := cv.FindContours(clean, cv.RetrExternal, cv.ChainApproxSimple)
	none, _ := cv.FindContours(clean, cv.RetrExternal, cv.ChainApproxNone)
	fmt.Printf("  ChainApproxSimple vs None total points: %d vs %d\n",
		totalPoints(contours), totalPoints(none))

	sort.Slice(contours, func(i, j int) bool {
		return cv.ContourArea(contours[i]) > cv.ContourArea(contours[j])
	})
	overlay := scene.Clone()
	for i, c := range contours {
		area := cv.ContourArea(c)
		if area < 200 {
			continue
		}
		r := cv.BoundingRect(c)
		rr := cv.MinAreaRect(c)
		hull := cv.ConvexHull(c)
		approx := cv.ApproxPolyDP(c, 0.02*cv.ArcLength(c, true), true)
		fmt.Printf("  #%d area=%7.1f perim=%6.1f bbox=%v minAreaRect=(%.0fx%.0f @%.1fdeg) hull=%d approx=%d verts\n",
			i, area, cv.ArcLength(c, true), r, rr.Width, rr.Height, rr.Angle, len(hull), len(approx))

		cv.Rectangle(overlay, cv.Point{X: r.X, Y: r.Y},
			cv.Point{X: r.X + r.Width - 1, Y: r.Y + r.Height - 1}, cv.NewScalar(0, 255, 255), 1)
		pts := rr.Points()
		cv.Polylines(overlay, [][]cv.Point{pts[:]}, true, cv.NewScalar(255, 0, 255), 1)
	}
	cv.DrawContours(overlay, contours, -1, cv.NewScalar(0, 0, 255), 1)
	save("20-contours.png", overlay)

	mom := cv.ImageMoments(clean)
	cx, cy := mom.Centroid()
	fmt.Printf("  ImageMoments: M00=%.0f centroid=(%.1f,%.1f) Nu20=%.5f Nu02=%.5f\n",
		mom.M00, cx, cy, mom.Nu20, mom.Nu02)
}

func totalPoints(cs []cv.Contour) int {
	n := 0
	for _, c := range cs {
		n += len(c)
	}
	return n
}

// --------------------------------------------------- connected components ----

func sectionConnected(scene *cv.Mat) {
	header("Connected components")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	bin, _ := cv.Threshold(gray, 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	clean := cv.MorphologyEx(bin, cv.GetStructuringElement(cv.MorphEllipse, 5, 5), cv.MorphOpen, 1)

	_, n4 := cv.ConnectedComponents(clean, cv.Connectivity4)
	labels, n8, stats := cv.ConnectedComponentsWithStats(clean, cv.Connectivity8)
	fmt.Printf("  labels: 4-connected=%d, 8-connected=%d (len(labels)=%d)\n", n4, n8, len(labels))

	sort.Slice(stats, func(i, j int) bool { return stats[i].Area > stats[j].Area })
	shown := 0
	for _, s := range stats {
		if s.Label == 0 || s.Area < 100 {
			continue
		}
		fmt.Printf("  label %2d area=%6d bbox=%v centroid=(%.1f,%.1f)\n",
			s.Label, s.Area, s.Rect, s.CentroidX, s.CentroidY)
		if shown++; shown >= 6 {
			break
		}
	}
}

// -------------------------------------------------------------- features -----

func sectionFeatures(scene *cv.Mat) {
	header("Edges, Hough, corners, template matching")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	blur := cv.GaussianBlur(gray, 5, 1.4)
	edges := cv.Canny(blur, 50, 150)
	fmt.Printf("  Canny(50,150): %d edge pixels\n", countNonZero(edges))
	save("21-canny.png", edges)

	harris := cv.CornerHarris(gray, 2, 3, 0.04)
	_, hMax, _, _, hx, hy := cv.MinMaxLoc(harris)
	fmt.Printf("  CornerHarris peak response=%.4g at (%d,%d)\n", hMax, hx, hy)

	corners := cv.GoodFeaturesToTrack(gray, 25, 0.01, 10, 3)
	fmt.Printf("  GoodFeaturesToTrack -> %d corners, first=%v\n", len(corners), firstPoints(corners, 4))

	fast := cv.FASTCorners(gray, 30, true)
	fmt.Printf("  FASTCorners(t=30, nonmax) -> %d corners\n", len(fast))

	lines := cv.HoughLines(edges, 1, math.Pi/180, 60)
	fmt.Printf("  HoughLines -> %d peaks", len(lines))
	if len(lines) > 0 {
		fmt.Printf(", strongest rho=%.1f theta=%.3frad", lines[0].Rho, lines[0].Theta)
	}
	fmt.Println()

	segs := cv.HoughLinesP(edges, 1, math.Pi/180, 60, 40, 6)
	fmt.Printf("  HoughLinesP -> %d segments\n", len(segs))
	hough := scene.Clone()
	for _, s := range segs {
		cv.Line(hough, s.Pt1, s.Pt2, cv.NewScalar(255, 0, 255), 1)
	}
	for _, p := range corners {
		cv.Circle(hough, p, 3, cv.NewScalar(0, 255, 255), 1)
	}
	save("22-hough-corners.png", hough)

	// param2 (the accumulator vote floor) has to be set fairly high or a textured
	// scene yields dozens of spurious circles; there is no automatic scaling.
	for _, p2 := range []float64{26.0, 45.0} {
		circles := cv.HoughCircles(gray, 20, 150, p2, 20, 60)
		hit := "none"
		for _, c := range circles {
			if absInt(c.X-200) <= 8 && absInt(c.Y-70) <= 8 && absInt(c.Radius-38) <= 6 {
				hit = fmt.Sprintf("(%d,%d) r=%d", c.X, c.Y, c.Radius)
				break
			}
		}
		fmt.Printf("  HoughCircles(param2=%.0f) -> %2d circles; detection matching the true disc (200,70,r38): %s\n",
			p2, len(circles), hit)
	}

	// Template matching: cut the disc out of the scene and find it again.
	templ := scene.Region(40, 170, 60, 60)
	for _, mode := range []struct {
		name string
		m    cv.TemplateMatchMode
		best string
	}{
		{"TmSqdiff", cv.TmSqdiff, "min"},
		{"TmSqdiffNormed", cv.TmSqdiffNormed, "min"},
		{"TmCcoeff", cv.TmCcoeff, "max"},
		{"TmCcoeffNormed", cv.TmCcoeffNormed, "max"},
	} {
		res := cv.MatchTemplate(scene, templ, mode.m)
		lo, hi, lx, ly, hx, hy := cv.MinMaxLoc(res)
		x, y, v := hx, hy, hi
		if mode.best == "min" {
			x, y, v = lx, ly, lo
		}
		fmt.Printf("  %-15s best(%s)=%.4g at (%d,%d) [expected (170,40)]\n", mode.name, mode.best, v, x, y)
	}

	// LUT-based inversion + Normalize / ConvertScaleAbs.
	table := make([]uint8, 256)
	for i := range table {
		table[i] = uint8(255 - i)
	}
	fmt.Printf("  LUT invert: gray(0,0)=%d -> %d\n", gray.At(0, 0, 0), cv.LUT(gray, table).At(0, 0, 0))
	norm := cv.Normalize(gray, 0, 255)
	fmt.Printf("  Normalize(0,255) span: %d..%d\n", minVal(norm), maxVal(norm))
	fmt.Printf("  ConvertScaleAbs(1.5,-20) span: %d..%d\n",
		minVal(cv.ConvertScaleAbs(gray, 1.5, -20)), maxVal(cv.ConvertScaleAbs(gray, 1.5, -20)))
}

func firstPoints(pts []cv.Point, n int) []cv.Point {
	if len(pts) < n {
		return pts
	}
	return pts[:n]
}

func minVal(m *cv.Mat) int {
	v := 255
	for _, x := range m.Data {
		if int(x) < v {
			v = int(x)
		}
	}
	return v
}

func maxVal(m *cv.Mat) int {
	v := 0
	for _, x := range m.Data {
		if int(x) > v {
			v = int(x)
		}
	}
	return v
}

// ------------------------------------------------------------- histogram -----

func sectionHistogram(scene *cv.Mat) {
	header("Histograms")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	hist := cv.CalcHist(gray, 0)
	fmt.Printf("  CalcHist: %d bins, sum=%d, peak bin=%d\n", len(hist), sumInts(hist), argMax(hist))

	eq := cv.EqualizeHist(gray)
	cl := cv.CLAHE(gray, 2.0, 8)
	fmt.Printf("  spans: gray=%d..%d equalized=%d..%d clahe=%d..%d\n",
		minVal(gray), maxVal(gray), minVal(eq), maxVal(eq), minVal(cl), maxVal(cl))
	save("23-equalized.png", eq)
	save("24-clahe.png", cl)

	eqHist := cv.CalcHist(eq, 0)
	for _, cm := range []struct {
		name string
		m    cv.HistCompMethod
	}{
		{"Correl", cv.HistCmpCorrel},
		{"ChiSqr", cv.HistCmpChiSqr},
		{"Intersect", cv.HistCmpIntersect},
		{"Bhattacharyya", cv.HistCmpBhattacharyya},
	} {
		fmt.Printf("  CompareHist %-14s self=%.4f vs-equalized=%.4f\n",
			cm.name, cv.CompareHist(hist, hist, cm.m), cv.CompareHist(hist, eqHist, cm.m))
	}

	bp := cv.CalcBackProject(gray, 0, hist)
	fmt.Printf("  CalcBackProject -> %dx%d span %d..%d\n", bp.Cols, bp.Rows, minVal(bp), maxVal(bp))
}

func sumInts(xs []int) int {
	s := 0
	for _, x := range xs {
		s += x
	}
	return s
}

func argMax(xs []int) int {
	best, bi := -1, 0
	for i, x := range xs {
		if x > best {
			best, bi = x, i
		}
	}
	return bi
}

// --------------------------------------------------------- quality module ----

func sectionQuality(scene *cv.Mat) {
	header("quality: full-reference & no-reference metrics")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)
	noisy := addSaltPepper(gray, 0.03)
	denoised := cv.MedianBlur(noisy, 3)

	for _, tc := range []struct {
		name string
		b    *cv.Mat
	}{{"self", gray}, {"noisy", noisy}, {"denoised", denoised}} {
		ssim, _ := quality.SSIM(gray, tc.b)
		gmsd, _ := quality.GMSD(gray, tc.b)
		fmt.Printf("  vs %-9s PSNR=%7.2f SSIM=%.4f GMSD=%.4f MSE=%v RMSE=%v\n",
			tc.name, quality.PSNR(gray, tc.b), ssim, gmsd,
			round2s(quality.MSE(gray, tc.b)), round2s(quality.RMSE(gray, tc.b)))
	}
	fmt.Printf("  FSIM(noisy)=%.4f VIFP(noisy)=%.4f SNR=%.2f\n",
		quality.FSIM(gray, noisy), quality.VIFP(gray, noisy), quality.SNR(gray, noisy))
	fmt.Printf("  no-reference: Sharpness=%.2f LaplacianVariance=%.2f Tenengrad=%.2f Entropy=%.3f BRISQUE=%.2f\n",
		quality.Sharpness(gray), quality.LaplacianVariance(gray),
		quality.Tenengrad(gray), quality.Entropy(gray), quality.BrisqueScore(gray))

	// Stateful, OpenCV-style QualityBase objects.
	q := quality.NewQualitySSIM(gray)
	fmt.Printf("  NewQualitySSIM.Compute(noisy)=%v\n", round2s(q.Compute(noisy)))
}

func round2s(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = math.Round(x*100) / 100
	}
	return out
}

// -------------------------------------------------------- imghash module -----

func sectionImgHash(scene *cv.Mat) {
	header("imghash: perceptual hashing")
	a := scene
	b := cv.GaussianBlur(scene, 5, 1.2)
	c := cv.Flip(scene, cv.FlipHorizontal)

	type hasher struct {
		name string
		fn   func(*cv.Mat) []byte
	}
	for _, h := range []hasher{
		{"Average", imghash.Average},
		{"Perceptual", imghash.Perceptual},
		{"Difference", imghash.Difference},
		{"Median", imghash.Median},
		{"BlockMean", imghash.BlockMean},
		{"MarrHildreth", imghash.MarrHildreth},
		{"RadialVariance", imghash.RadialVariance},
		{"ColorMoment", imghash.ColorMoment},
	} {
		ha, hb, hc := h.fn(a), h.fn(b), h.fn(c)
		fmt.Printf("  %-15s %2dB hash=%s… sim(blur)=%.3f sim(flip)=%.3f dup(blur,0.1)=%v\n",
			h.name, len(ha), truncate(imghash.HexEncode(ha), 16),
			imghash.Similarity(ha, hb), imghash.Similarity(ha, hc),
			imghash.IsDuplicate(ha, hb, 0.1))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ----------------------------------------------------------- photo module ----

func sectionPhoto(scene *cv.Mat) {
	header("photo: denoise, inpaint, stylisation")
	small := cv.Resize(scene, 120, 90, cv.InterLinear)
	noisy := addSaltPepper(small, 0.05)

	den := photo.FastNlMeansDenoisingColored(noisy, 10, 10, 3, 7)
	fmt.Printf("  FastNlMeansDenoisingColored: variance %.1f -> %.1f\n", variance(noisy), variance(den))
	save("25-denoised.png", den)

	// Inpaint a hole punched through the middle of the scene.
	holed := small.Clone()
	mask := cv.NewMat(small.Rows, small.Cols, 1)
	cv.Rectangle(holed, cv.Point{X: 40, Y: 30}, cv.Point{X: 70, Y: 55}, cv.NewScalar(0, 0, 0), cv.Filled)
	cv.Rectangle(mask, cv.Point{X: 40, Y: 30}, cv.Point{X: 70, Y: 55}, cv.NewScalar(255), cv.Filled)
	inp := photo.Inpaint(holed, mask, 3, 0)
	fmt.Printf("  Inpaint: PSNR(holed)=%.2f -> PSNR(inpainted)=%.2f\n",
		quality.PSNR(small, holed), quality.PSNR(small, inp))
	save("26-inpainted.png", inp)

	describe("EdgePreservingFilter", photo.EdgePreservingFilter(small, 0, 30, 0.4))
	describe("DetailEnhance", photo.DetailEnhance(small, 20, 0.15))
	describe("Stylization", photo.Stylization(small, 30, 0.4))
	describe("GammaCorrection(0.5)", photo.GammaCorrection(small, 0.5))
	describe("UnsharpMask", photo.UnsharpMask(small, 5, 1.2, 1.0))
	describe("HistogramStretch", photo.HistogramStretch(small))
	describe("GrayWorldWhiteBalance", photo.GrayWorldWhiteBalance(small))
	dgray, boost := photo.Decolor(small)
	fmt.Printf("  Decolor -> gray %dch, boost %dch\n", dgray.Channels, boost.Channels)
	save("27-stylized.png", photo.Stylization(small, 30, 0.4))
}

// -------------------------------------------------------- ximgproc module ----

func sectionXImgProc(scene *cv.Mat) {
	header("ximgproc: edge-aware filters, thinning, superpixels")
	small := cv.Resize(scene, 120, 90, cv.InterLinear)
	gray := cv.CvtColor(small, cv.ColorRGB2Gray)

	describe("GuidedFilter", ximgproc.GuidedFilter(gray, gray, 4, 0.02))
	describe("JointBilateralFilter", ximgproc.JointBilateralFilter(small, small, 5, 30, 5))
	describe("RollingGuidanceFilter", ximgproc.RollingGuidanceFilter(gray, 5, 30, 5, 2))
	describe("AnisotropicDiffusion", ximgproc.AnisotropicDiffusion(small, 0.15, 20, 5))
	describe("BilateralTextureFilter", ximgproc.BilateralTextureFilter(small, 3, 1))
	describe("NiBlackThreshold", ximgproc.NiBlackThreshold(gray, -0.2, 11, 0))

	bin, _ := cv.Threshold(gray, 0, 255, cv.ThreshBinary|cv.ThreshOtsu)
	thin := ximgproc.Thinning(bin)
	fmt.Printf("  Thinning: %d -> %d foreground pixels\n", countNonZero(bin), countNonZero(thin))
	save("28-thinning.png", thin)

	labels, n := ximgproc.SuperpixelSLIC(small, 12, 10)
	fmt.Printf("  SuperpixelSLIC -> %d superpixels (label map %dx%d)\n", n, labels.Cols, labels.Rows)
	lscLabels, lscN := ximgproc.SuperpixelLSC(small, 12, 0.075)
	fmt.Printf("  SuperpixelLSC  -> %d superpixels (label map %dx%d)\n", lscN, lscLabels.Cols, lscLabels.Rows)

	segs := ximgproc.FastLineDetector(gray)
	fmt.Printf("  FastLineDetector -> %d segments\n", len(segs))
	edges := ximgproc.StructuredEdgeDetectionLite(gray)
	_, eMax, _, _, _, _ := cv.MinMaxLoc(edges)
	fmt.Printf("  StructuredEdgeDetectionLite -> float map, max=%.4f\n", eMax)
	boxes := ximgproc.EdgeBoxes(edges, 5)
	fmt.Printf("  EdgeBoxes(top 5) -> %d proposals\n", len(boxes))
}

// ---------------------------------------------------- segmentation module ----

func sectionSegmentation(scene *cv.Mat) {
	header("segmentation: k-means, graph, SLIC, GrabCut, flood fill")
	small := cv.Resize(scene, 120, 90, cv.InterLinear)

	lm, centers := segmentation.KMeansSegmentation(small, 4, 10)
	fmt.Printf("  KMeansSegmentation k=4 -> %d regions, centres=%v\n", lm.Count, roundCenters(centers))

	eg := segmentation.EfficientGraphSegmentation(small, 0.8, 300, 50)
	fmt.Printf("  EfficientGraphSegmentation -> %d regions\n", eg.Count)

	slic := segmentation.SLIC(small, 12, 10, 8)
	fmt.Printf("  SLIC -> %d regions\n", slic.Count)

	msLm := segmentation.MeanShiftSegmentation(small, 8, 24, 30)
	fmt.Printf("  MeanShiftSegmentation -> %d regions\n", msLm.Count)
	describe("PyrMeanShiftFiltering", segmentation.PyrMeanShiftFiltering(small, 8, 24, 1))

	gray := cv.CvtColor(small, cv.ColorRGB2Gray)
	levels := segmentation.MultiOtsu(gray, 3)
	quant, lv2 := segmentation.MultiOtsuThreshold(gray, 3)
	fmt.Printf("  MultiOtsu(3 classes) levels=%v / %v -> quantised %dx%d\n",
		levels, lv2, quant.Cols, quant.Rows)

	count, rect := segmentation.FloodFill(small.Clone(), cv.Point{X: 20, Y: 15},
		cv.NewScalar(0, 255, 0), cv.NewScalar(12, 12, 12), cv.NewScalar(12, 12, 12), 8)
	fmt.Printf("  FloodFill from (20,15): %d pixels, bbox=%v\n", count, rect)

	// NOTE: pass a nil mask for rect initialisation. Handing GrabCut a freshly
	// allocated (all-zero) mask means "every pixel is definite background", so it
	// silently returns an empty segmentation instead of using rect.
	gc := segmentation.GrabCut(small, nil, cv.Rect{X: 60, Y: 10, Width: 45, Height: 45}, 3)
	fg := 0
	for _, v := range gc.Data {
		if v&1 == 1 {
			fg++
		}
	}
	fmt.Printf("  GrabCut(rect init) -> mask %dx%d, %d foreground pixels (mask&1)\n", gc.Cols, gc.Rows, fg)

	rag := segmentation.BuildRAG(slic, small)
	fmt.Printf("  BuildRAG over SLIC labels -> %T built\n", rag)

	seeds := []cv.Point{{X: 20, Y: 15}, {X: 75, Y: 25}, {X: 60, Y: 70}}
	rg := segmentation.RegionGrowing(small, seeds, 25)
	fmt.Printf("  RegionGrowing(3 seeds, t=25) -> %d regions\n", rg.Count)

	// Colourise a label map for a visual check.
	save("29-slic-labels.png", labelsToImage(slic))
}

func roundCenters(cs [][]float64) [][]float64 {
	out := make([][]float64, len(cs))
	for i, c := range cs {
		out[i] = round2s(c)
	}
	return out
}

func labelsToImage(lm *segmentation.LabelMap) *cv.Mat {
	m := cv.NewMat(lm.Rows, lm.Cols, 1)
	if lm.Count <= 1 {
		return m
	}
	for i, l := range lm.Labels {
		m.Data[i] = uint8(l * 255 / (lm.Count - 1))
	}
	return plot.ApplyColorMap(m, plot.ColormapViridis)
}

// --------------------------------------------------- features2d module -------

func sectionFeatures2D() {
	header("features2d: ORB keypoints + descriptor matching")
	// A textured synthetic scene, plus the same scene shifted, so matching has
	// a ground truth.
	base := buildTextured(160, 160, 0)
	shifted := buildTextured(160, 160, 6)

	orb := features2d.NewORB(200)
	kpsA := orb.Detect(base)
	fmt.Printf("  ORB.Detect -> %d keypoints\n", len(kpsA))
	if len(kpsA) > 0 {
		k := kpsA[0]
		fmt.Printf("    kp[0] pt=%v size=%.1f angle=%.1f response=%.4g octave=%d\n",
			k.Pt, k.Size, k.Angle, k.Response, k.Octave)
	}

	kpA, descA := orb.DetectAndCompute(base)
	kpB, descB := orb.DetectAndCompute(shifted)
	fmt.Printf("  DetectAndCompute: A=%d kps/%d desc, B=%d kps/%d desc, %d bytes each\n",
		len(kpA), len(descA), len(kpB), len(descB), descLen(descA))

	if len(descA) > 0 && len(descB) > 0 {
		q := features2d.NewBinaryDescriptors(descA)
		t := features2d.NewBinaryDescriptors(descB)
		matcher := features2d.NewBFMatcher(features2d.NormHamming)
		matches := matcher.Match(q, t)
		fmt.Printf("  BFMatcher(NormHamming).Match -> %d matches\n", len(matches))

		knn := matcher.KnnMatch(q, t, 2)
		kept := features2d.RatioTest(knn, 0.8)
		fmt.Printf("  KnnMatch(k=2) + RatioTest(0.8) -> %d/%d kept\n", len(kept), len(knn))

		// How many surviving matches respect the true +6px x-shift?
		good := 0
		for _, m := range kept {
			dx := kpB[m.TrainIdx].Pt.X - kpA[m.QueryIdx].Pt.X
			dy := kpB[m.TrainIdx].Pt.Y - kpA[m.QueryIdx].Pt.Y
			if dx >= 4 && dx <= 8 && dy >= -2 && dy <= 2 {
				good++
			}
		}
		fmt.Printf("  matches consistent with the true (+6,0) shift: %d/%d\n", good, len(kept))
		fmt.Printf("  HammingDistance([0xAA],[0x55]) = %d\n",
			features2d.HammingDistance([]byte{0xAA}, []byte{0x55}))
	}
}

func descLen(d [][]byte) int {
	if len(d) == 0 {
		return 0
	}
	return len(d[0])
}

// buildTextured draws a deterministic checker/blob texture, offset by dx pixels.
func buildTextured(rows, cols, dx int) *cv.Mat {
	m := cv.NewMat(rows, cols, 1)
	m.SetTo(40)
	// Nine deliberately dissimilar motifs (varying size, shape and interior) so
	// descriptors are distinctive rather than repetitive.
	for i := 0; i < 9; i++ {
		cx := 25 + (i%3)*55 + dx
		cy := 25 + (i/3)*55
		r := 8 + i
		bright := uint8(150 + 10*i)
		switch i % 3 {
		case 0:
			cv.Rectangle(m, cv.Point{X: cx - r, Y: cy - r}, cv.Point{X: cx + r, Y: cy + r},
				cv.NewScalar(float64(bright)), cv.Filled)
			cv.Line(m, cv.Point{X: cx - r, Y: cy}, cv.Point{X: cx + r, Y: cy}, cv.NewScalar(20), 1+i%2)
		case 1:
			cv.Circle(m, cv.Point{X: cx, Y: cy}, r, cv.NewScalar(float64(bright)), cv.Filled)
			cv.Rectangle(m, cv.Point{X: cx - 3, Y: cy - r}, cv.Point{X: cx + 3, Y: cy}, cv.NewScalar(20), cv.Filled)
		default:
			cv.FillPoly(m, [][]cv.Point{{
				{X: cx, Y: cy - r}, {X: cx + r, Y: cy + r}, {X: cx - r, Y: cy + r},
			}}, cv.NewScalar(float64(bright)))
			cv.Circle(m, cv.Point{X: cx, Y: cy + r/2}, 1+i/3, cv.NewScalar(30), cv.Filled)
		}
	}
	return m
}

// ----------------------------------------------------- objdetect module ------

func sectionObjDetect(scene *cv.Mat) {
	header("objdetect: HOG, LBP, integral image, NMS")
	gray := cv.CvtColor(scene, cv.ColorRGB2Gray)

	hog := objdetect.NewHOGDescriptor()
	fmt.Printf("  HOGDescriptor.DescriptorSize() = %d\n", hog.DescriptorSize())
	window := cv.Resize(gray, 64, 128, cv.InterLinear)
	desc := hog.Compute(window)
	fmt.Printf("  HOG.Compute(64x128) -> %d floats, L2 norm=%.4f\n", len(desc), l2(desc))
	mag, ang, w, h := hog.ComputeGradient(window)
	fmt.Printf("  HOG.ComputeGradient -> %dx%d, %d mags / %d angles\n", w, h, len(mag), len(ang))
	weights := hog.DefaultPeopleDetector()
	hits := hog.DetectMultiScale(gray, weights, 0.5, 1.2)
	fmt.Printf("  DetectMultiScale(default people detector) -> %d raw hits\n", len(hits))

	lbp := objdetect.ComputeLBP(gray)
	describe("ComputeLBP", lbp)

	ii := objdetect.NewIntegralImage(gray)
	fmt.Printf("  IntegralImage built: %T\n", ii)

	boxes := []cv.Rect{
		{X: 10, Y: 10, Width: 50, Height: 50},
		{X: 14, Y: 12, Width: 50, Height: 50},
		{X: 200, Y: 100, Width: 40, Height: 40},
	}
	scores := []float64{0.9, 0.85, 0.7}
	fmt.Printf("  RectIoU(box0,box1)=%.3f RectIoU(box0,box2)=%.3f\n",
		objdetect.RectIoU(boxes[0], boxes[1]), objdetect.RectIoU(boxes[0], boxes[2]))
	fmt.Printf("  NMSBoxes(score>0.5, iou>0.3) keeps indices %v\n",
		objdetect.NMSBoxes(boxes, scores, 0.5, 0.3))
	softIdx, softScores := objdetect.SoftNMSBoxes(boxes, scores, 0.5, 0.5)
	fmt.Printf("  SoftNMSBoxes -> indices %v, decayed scores %v\n", softIdx, round2s(softScores))
	fmt.Printf("  GroupRectangles(minNeighbors=1) -> %v\n", objdetect.GroupRectangles(boxes, 1, 0.4))
}

func l2(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x * x
	}
	return math.Sqrt(s)
}

// --------------------------------------------------------- aruco module ------

func sectionAruco() {
	header("aruco: marker generation, detection, pose, boards")
	dict := aruco.GetPredefinedDictionary(aruco.Dict4x4)
	fmt.Printf("  dictionary %q: %d markers, %d bits/side, tolerance=%d\n",
		dict.Name, dict.Size(), dict.BitsPerSide(), dict.Tolerance())

	// Paste two markers onto a light background and detect them.
	canvas := cv.NewMat(240, 320, 1)
	canvas.SetTo(210)
	aruco.GenerateMarker(dict, 3, 72).CopyTo(canvas, 30, 30)
	aruco.GenerateMarker(dict, 7, 72).CopyTo(canvas, 120, 180)
	save("30-aruco-scene.png", canvas)

	corners, ids := aruco.DetectMarkers(canvas, dict)
	fmt.Printf("  DetectMarkers -> ids %v (expected [3 7])\n", ids)
	for i, id := range ids {
		fmt.Printf("    id %d corners=%v\n", id, corners[i])
	}
	save("31-aruco-detected.png", aruco.DrawDetectedMarkers(cv.CvtColor(canvas, cv.ColorGray2RGB), corners, ids))

	// Rotation invariance: 90-degree rotation must yield the same ids.
	rot := cv.Rotate(canvas, cv.Rotate90CW)
	_, rotIDs := aruco.DetectMarkers(rot, dict)
	fmt.Printf("  DetectMarkers on 90deg-rotated canvas -> ids %v\n", rotIDs)

	// Grid board render + detect.
	board := aruco.NewGridBoard(2, 2, 40, 10, dict, 0)
	boardImg := aruco.DrawPlanarBoard(board, 300, 300, 15)
	describe("DrawPlanarBoard 2x2", boardImg)
	save("32-aruco-gridboard.png", boardImg)
	bCorners, bIDs := aruco.DetectMarkers(boardImg, dict)
	fmt.Printf("  grid board detection -> %d markers, ids %v\n", len(bIDs), bIDs)
	// HOLE: aruco.GetBoardObjectAndImagePoints exists in the working tree but not
	// in the published v0.8.0 module, so the board object/image point pairing is
	// unavailable to outside users.
	// objPts, imgPts := aruco.GetBoardObjectAndImagePoints(board, bCorners, bIDs)

	// Drop one marker and let RefineDetectedMarkers recover it from the board layout.
	if len(bIDs) > 1 {
		rc, rid, recovered := aruco.RefineDetectedMarkers(boardImg, board, bCorners[1:], bIDs[1:])
		fmt.Printf("  RefineDetectedMarkers(dropped 1) -> %d markers, ids %v, recovered=%d\n",
			len(rc), rid, recovered)
	}

	camK := [3][3]float64{{500, 0, 150}, {0, 500, 150}, {0, 0, 1}}
	brvec, btvec, used := aruco.EstimatePoseBoard(bCorners, bIDs, board, camK, nil)
	fmt.Printf("  EstimatePoseBoard -> markersUsed=%d rvec=%v tvec=%v\n",
		used, round2s(brvec[:]), round2s(btvec[:]))

	// ChArUco board render + detect + corner interpolation.
	ch := aruco.NewCharucoBoard(4, 4, 40, 24, dict)
	chImg := aruco.DrawCharucoBoard(ch, 320, 320, 10)
	save("33-charuco.png", chImg)
	chMarkerCorners, chMarkerIDs := aruco.DetectMarkers(chImg, dict)
	chCorners, chIDs := aruco.InterpolateCornersCharuco(chMarkerCorners, chMarkerIDs, chImg, ch)
	fmt.Printf("  ChArUco: %d markers detected, %d interpolated corners (ids %v)\n",
		len(chMarkerIDs), len(chCorners), chIDs)

	// Diamond.
	dia := aruco.DrawCharucoDiamond(dict, [4]int{0, 1, 2, 3}, 90, 60)
	describe("DrawCharucoDiamond", dia)
	diaMarkerCorners, diaMarkerIDs := aruco.DetectMarkers(dia, dict)
	diaCorners, diaIDs := aruco.DetectCharucoDiamond(dia, diaMarkerCorners, diaMarkerIDs, dict, 90, 60)
	fmt.Printf("  DetectCharucoDiamond -> %d diamonds, ids %v\n", len(diaCorners), diaIDs)

	// Single-marker pose estimation with a synthetic pinhole camera.
	// HOLE: aruco.ProjectPoints exists in the working tree but not in published
	// v0.8.0, so there is no way to forward-project object points for a
	// reprojection check from outside the module.
	k := [3][3]float64{{500, 0, 160}, {0, 500, 120}, {0, 0, 1}}
	rvecs, tvecs := aruco.EstimatePoseSingleMarkers(corners, 40, k, nil)
	for i := range rvecs {
		fmt.Printf("  EstimatePoseSingleMarkers id=%d rvec=%v tvec=%v\n",
			ids[i], round2s(rvecs[i][:]), round2s(tvecs[i][:]))
	}
	dRvecs, dTvecs := aruco.EstimatePoseSingleMarkersWithDistortion(corners, 40, k, []float64{-0.05, 0.01, 0, 0})
	fmt.Printf("  ...WithDistortion -> %d poses, first tvec=%v (rvec=%v)\n",
		len(dRvecs), round2s(dTvecs[0][:]), round2s(dRvecs[0][:]))

	// Sub-pixel refinement and distortion undo on raw point lists.
	rough := [][2]float64{{30.4, 30.6}, {101.3, 30.2}}
	fmt.Printf("  CornerSubPix %v -> %v\n", rough, aruco.CornerSubPix(canvas, rough, 5, 20, 0.01))
	fmt.Printf("  UndistortImagePoints %v -> %v\n", rough,
		aruco.UndistortImagePoints(rough, k, []float64{-0.05, 0.01, 0, 0}))

	// Detection with explicit parameters.
	params := aruco.DefaultDetectorParameters()
	pCorners, pIDs := aruco.DetectMarkersWithParams(canvas, dict, params)
	fmt.Printf("  DetectMarkersWithParams(defaults) -> %d markers, ids %v\n", len(pCorners), pIDs)

	// 6x6 dictionary + custom dictionary.
	d6 := aruco.GetPredefinedDictionary6x6(aruco.Dict6x6)
	fmt.Printf("  6x6 dictionary %q: %d markers, %d bits/side\n", d6.Name, d6.Size(), d6.BitsPerSide())
	custom := aruco.GenerateCustomDictionary(5, 20, 6, 42)
	fmt.Printf("  GenerateCustomDictionary(5 bits, 20 markers, minDist 6) -> %d markers\n", custom.Size())
}

// ------------------------------------------------------- barcode module ------

func sectionBarcode() {
	header("barcode: QR + 1-D symbologies, encode & decode")
	qr := barcode.QREncode("HELLO CV PORT", 3)
	describe("QREncode v3", qr)
	save("34-qr.png", qr)
	text, ok := barcode.QRDetectAndDecode(qr)
	fmt.Printf("  QRDetectAndDecode -> %q ok=%v\n", text, ok)
	fmt.Printf("  QRCapacity(v3)=%d bytes, QRModuleCount(v3)=%d, QRSizeForVersion(3)=%d\n",
		barcode.QRCapacity(3), barcode.QRModuleCount(3), barcode.QRSizeForVersion(3))

	adv, err := barcode.QREncodeAdvanced("Numeric 12345 and text", 4, 1)
	if err != nil {
		fmt.Printf("  QREncodeAdvanced error: %v\n", err)
	} else {
		got, ok := barcode.QRDetectAndDecodeAdvanced(adv)
		fmt.Printf("  QREncodeAdvanced v4 (%dx%d) -> decode %q ok=%v\n", adv.Cols, adv.Rows, got, ok)
	}
	fmt.Printf("  FindFinderPatterns(qr) -> %d patterns\n", len(barcode.FindFinderPatterns(qr)))

	type sym struct {
		name    string
		payload string
		enc     func(string) (*cv.Mat, error)
		dec     func(*cv.Mat) (string, bool)
	}
	for _, s := range []sym{
		{"EAN-13", "590123412345", barcode.EncodeEAN13, barcode.DecodeEAN13},
		{"EAN-8", "9638507", barcode.EncodeEAN8, barcode.DecodeEAN8},
		{"Code128", "CV-PORT-128", barcode.EncodeCode128, barcode.DecodeCode128},
		{"Code39", "CV39", barcode.EncodeCode39, barcode.DecodeCode39},
		{"Code93", "CV93", barcode.EncodeCode93, barcode.DecodeCode93},
		{"Codabar", "A12345B", barcode.EncodeCodabar, barcode.DecodeCodabar},
		{"ITF", "1234567890", barcode.EncodeITF, barcode.DecodeITF},
		{"MSI", "12345", barcode.EncodeMSI, barcode.DecodeMSI},
		{"Code11", "123-45", barcode.EncodeCode11, barcode.DecodeCode11},
	} {
		img, err := s.enc(s.payload)
		if err != nil {
			fmt.Printf("  %-8s encode error: %v\n", s.name, err)
			continue
		}
		got, ok := s.dec(img)
		fmt.Printf("  %-8s %3dx%-3d encode(%q) -> decode(%q) ok=%v\n",
			s.name, img.Cols, img.Rows, s.payload, got, ok)
		if s.name == "Code128" {
			save("35-code128.png", img)
		}
	}

	cd, _ := barcode.EAN13CheckDigit("590123412345")
	fmt.Printf("  EAN13CheckDigit(590123412345)=%d ValidateEAN13=%v ValidateLuhn(4539578763621486)=%v\n",
		cd, barcode.ValidateEAN13("5901234123457"), barcode.ValidateLuhn("4539578763621486"))
	const upcA = "042100005264"
	upce, err := barcode.UPCAToUPCE(upcA)
	if err != nil {
		fmt.Printf("  UPCAToUPCE(%s) error: %v\n", upcA, err)
	} else {
		back, err := barcode.UPCEToUPCA(upce)
		fmt.Printf("  UPCAToUPCE(%s)=%s -> UPCEToUPCA=%s (err=%v, round-trip=%v)\n",
			upcA, upce, back, err, back == upcA)
	}
	ean13, err := barcode.UPCAToEAN13(upcA)
	fmt.Printf("  UPCAToEAN13(%s)=%s err=%v\n", upcA, ean13, err)

	// Multi-symbol detection on a stacked canvas.
	c128, _ := barcode.EncodeCode128("ONE")
	e13, _ := barcode.EncodeEAN13("590123412345")
	stack := cv.NewMat(c128.Rows+e13.Rows+30, maxInt(c128.Cols, e13.Cols)+20, 1)
	stack.SetTo(255)
	c128.CopyTo(stack, 10, 10)
	e13.CopyTo(stack, c128.Rows+20, 10)
	found := barcode.DetectAndDecodeMulti(stack)
	fmt.Printf("  DetectAndDecodeMulti(stacked) -> %d symbols\n", len(found))
	for _, b := range found {
		fmt.Printf("    %+v\n", b)
	}
	save("36-barcode-stack.png", stack)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------- plot module ------

func sectionPlot() {
	header("plot: charts and colormaps")
	xs := make([]float64, 64)
	ys := make([]float64, 64)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = math.Sin(float64(i)/6) * 40
	}
	line := plot.CreatePlot(xs, ys).
		SetSize(480, 280).
		SetShowGrid(true).
		SetLineColor(cv.NewScalar(255, 80, 80)).
		SetBackgroundColor(cv.NewScalar(18, 18, 24)).
		Render()
	describe("CreatePlot line chart", line)
	save("37-plot-line.png", line)

	describe("ScatterPlot", plot.ScatterPlot(xs, ys).SetSize(320, 200).Render())
	describe("BarPlot", plot.BarPlot([]float64{3, 7, 2, 9, 5}).SetSize(320, 200).Render())
	hist := plot.HistogramPlot(ys, 12).SetSize(400, 240).Render()
	describe("HistogramPlot", hist)
	save("38-plot-hist.png", hist)

	pie := plot.NewPiePlot([]float64{4, 3, 2, 1})
	fmt.Printf("  NewPiePlot built: %T\n", pie)
	fmt.Printf("  NewBoxPlot built: %T\n", plot.NewBoxPlot([][]float64{{1, 2, 3, 4}, {2, 3, 8}}))
	fmt.Printf("  NewHeatmapPlot built: %T\n", plot.NewHeatmapPlot([][]float64{{1, 2}, {3, 4}}))
	fmt.Printf("  TextSize(\"CV\",1) = %v\n", []int{textW("CV"), plot.TextHeight(1)})

	// Colormaps over a grayscale ramp.
	ramp := cv.NewMat(40, 256, 1)
	for y := 0; y < 40; y++ {
		for x := 0; x < 256; x++ {
			ramp.Data[y*256+x] = uint8(x)
		}
	}
	// HOLE: ApplyColorMap/ColormapTable only know the original eight colormaps and
	// PANIC ("plot: ColormapTable unknown colormap") on the thirteen extra ones
	// declared in the same package (Autumn..Turbo). The general entry points are
	// plot.Colorize / plot.Table, so which function you must call depends on which
	// Colormap constant you picked — an easy, unchecked trap.
	for _, cm := range []struct {
		name string
		cm   plot.Colormap
	}{
		{"Jet", plot.ColormapJet},
		{"Viridis", plot.ColormapViridis},
	} {
		c := plot.ApplyColorMap(ramp, cm.cm)
		fmt.Printf("  ApplyColorMap %-8s 0->%v 255->%v (table %d entries)\n",
			cm.name, c.AtPixel(0, 0), c.AtPixel(0, 255), len(plot.ColormapTable(cm.cm)))
	}
	for _, cm := range []struct {
		name string
		cm   plot.Colormap
	}{
		{"Turbo", plot.ColormapTurbo},
		{"Inferno", plot.ColormapInferno},
		{"Parula", plot.ColormapParula},
	} {
		c := plot.Colorize(ramp, cm.cm)
		fmt.Printf("  Colorize      %-8s 0->%v 255->%v (table %d entries)\n",
			cm.name, c.AtPixel(0, 0), c.AtPixel(0, 255), len(plot.Table(cm.cm)))
	}
	save("39-colormap-jet.png", plot.ApplyColorMap(ramp, plot.ColormapJet))

	legend := cv.NewMat(120, 220, 3)
	legend.SetTo(30)
	plot.DrawLegend(legend, []plot.LegendEntry{
		{Label: "alpha", Color: cv.NewScalar(255, 90, 90)},
		{Label: "beta", Color: cv.NewScalar(90, 255, 140)},
	}, 10, 10, 1, cv.NewScalar(255, 255, 255), cv.NewScalar(60, 60, 60))
	save("40-legend.png", legend)
	describe("DrawLegend canvas", legend)
}

func textW(s string) int {
	w, _ := plot.TextSize(s, 1)
	return w
}

// ------------------------------------------------------------ ml module ------

func sectionML() {
	header("ml: classifiers, clustering, metrics")
	// Two well-separated 2-D Gaussian-ish blobs, deterministic.
	var samples [][]float64
	var labels []int
	var state uint64 = 12345
	next := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64((state>>33)%1000)/1000 - 0.5
	}
	for i := 0; i < 60; i++ {
		samples = append(samples, []float64{1 + next(), 1 + next()})
		labels = append(labels, 0)
		samples = append(samples, []float64{4 + next(), 4.5 + next()})
		labels = append(labels, 1)
	}
	td := ml.NewTrainData(samples, labels)
	fmt.Printf("  TrainData: %d samples, %d labels\n", len(td.Samples), len(td.Labels))

	models := []struct {
		name string
		m    ml.Classifier
	}{
		{"KNearest(3)", ml.NewKNearest(3)},
		{"SVM", ml.NewSVM()},
		{"RTrees(20)", ml.NewRTrees(20)},
		{"Boost(20)", ml.NewBoost(20)},
		{"NormalBayes", ml.NewNormalBayesClassifier()},
		{"LogisticRegression", ml.NewLogisticRegression()},
		{"ANNMLP(4)", ml.NewANNMLP(4)},
	}
	for _, mdl := range models {
		if err := mdl.m.Train(samples, labels); err != nil {
			fmt.Printf("  %-19s train error: %v\n", mdl.name, err)
			continue
		}
		pred := mdl.m.PredictBatch(samples)
		fmt.Printf("  %-19s train accuracy=%.3f macroF1=%.3f\n",
			mdl.name, ml.Accuracy(pred, labels), ml.MacroF1(pred, labels))
	}

	scores := ml.CrossValScore(ml.NewKNearest(3), td, 5, 7)
	fmt.Printf("  CrossValScore(KNearest, k=5) = %v mean=%.3f\n", round2s(scores), ml.MeanScore(scores))

	knn := ml.NewKNearest(3)
	if err := knn.Train(samples, labels); err != nil {
		fatal(err)
	}
	nl, nd := knn.FindNearest([]float64{4, 4.5}, 3)
	fmt.Printf("  KNearest.FindNearest([4,4.5],3) -> labels %v dists %v; Predict=%d\n",
		nl, round2s(nd), knn.Predict([]float64{4, 4.5}))

	svm := ml.NewSVM()
	if err := svm.Train(samples, labels); err != nil {
		fatal(err)
	}
	fmt.Printf("  SVM classes=%v decision scores at [1,1]=%v\n",
		svm.Classes(), round2s(svm.DecisionScores([]float64{1, 1})))

	pred := knn.PredictBatch(samples)
	fmt.Printf("  ConfusionMatrix=%v precision(1)=%.3f recall(1)=%.3f F1(1)=%.3f\n",
		ml.ConfusionMatrix(pred, labels, 2), ml.Precision(pred, labels, 1),
		ml.Recall(pred, labels, 1), ml.F1Score(pred, labels, 1))

	lab, centers := ml.KMeans(samples, 2, 25, 3)
	fmt.Printf("  KMeans(k=2) -> %d assignments, centres=%v\n", len(lab), roundCenters(centers))

	// Persist and reload a model. Only the five models that implement
	// gob.GobEncoder survive the trip.
	//
	// HOLE: ml.Save/SaveFile is plain encoding/gob, and only RTrees, Boost,
	// ANNMLP, GaussianMixture and KernelSVM implement GobEncoder/GobDecoder.
	// Saving a KNearest / SVM / NormalBayes / LogisticRegression writes only its
	// exported fields, Load returns a nil error, and the "restored" model then
	// panics with "ml: model has not been trained" on first use. Silent data loss
	// with no error and no documented warning on Save/SaveFile:
	//   _ = ml.SaveFile(p, knn); _ = ml.LoadFile(p, ml.NewKNearest(3))
	//   reloaded.PredictBatch(samples) // panics
	rt := ml.NewRTrees(20)
	if err := rt.Train(samples, labels); err != nil {
		fatal(err)
	}
	modelPath := filepath.Join(outDir, "rtrees.gob")
	if err := ml.SaveFile(modelPath, rt); err != nil {
		fmt.Printf("  ml.SaveFile error: %v\n", err)
	} else {
		reloaded := ml.NewRTrees(20)
		if err := ml.LoadFile(modelPath, reloaded); err != nil {
			fmt.Printf("  ml.LoadFile error: %v\n", err)
		} else {
			fmt.Printf("  SaveFile/LoadFile(RTrees) round-trip: reloaded accuracy=%.3f\n",
				ml.Accuracy(reloaded.PredictBatch(samples), labels))
		}
	}

	// Regression-side metrics.
	truth := []float64{1, 2, 3, 4, 5}
	guess := []float64{1.1, 1.9, 3.2, 3.7, 5.4}
	fmt.Printf("  regression: MSE=%.4f RMSE=%.4f MAE=%.4f R2=%.4f\n",
		ml.MSE(guess, truth), ml.RMSE(guess, truth), ml.MAE(guess, truth), ml.R2Score(guess, truth))
	fpr, tpr := ml.ROCCurve([]float64{0.1, 0.4, 0.35, 0.8}, []int{0, 0, 1, 1}, 1)
	fmt.Printf("  ROCCurve points=%d AUC=%.3f\n", len(fpr), ml.AUCFromCurve(fpr, tpr))
}
