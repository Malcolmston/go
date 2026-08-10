# COVERAGE — `opencv` (Python / `cv2`) vs `github.com/malcolmston/opencv` (Go)

| | |
| --- | --- |
| upstream | `opencv-python@4.11.0` — pinned by recording `cv2.__version__` from the system `python3` (3.13.5 at `/opt/anaconda3/bin/python3`). Nothing was installed and no virtualenv was created. |
| Go module | `github.com/malcolmston/opencv v0.8.0` — the published module, resolved with `GOWORK=off go get github.com/malcolmston/opencv@latest`, **no `replace` directive**. v0.8.0 lags the working tree: `aruco.ProjectPoints` and `aruco.GetBoardObjectAndImagePoints` are unreleased and therefore out of scope here. |
| runners | `python/run.py` (cv2), `go/run.go` (the port) — JSON Lines on stdio, one process each |
| cases | **751** across 16 case files |
| harness | `GOWORK=off go test ./parity/opencv/` — writes `parity.json` |
| result | **582 match, 168 differ, 1 deliberate deviation → parity 77.60 % over a denominator of 750 compared cases** |
| of which | 29 cases where *both* sides fail (agreed-upon rejections), and **38 cases where the port panics while cv2 returns a value** |

## How the upstream inventory was derived

Mechanically, from the installed package — not from the README and not from memory:

```
python3 -c "import cv2; print(len([n for n in dir(cv2) if not n.startswith('_') and callable(getattr(cv2,n))]))"   # 668
python3 -c "import cv2; print(len([n for n in dir(cv2) if not n.startswith('_')]))"                                # 2413
python3 -c "import cv2; print(len([n for n in dir(cv2) if not n.startswith('_') and type(getattr(cv2,n)).__name__=='module']))"  # 28
python3 -c "import cv2; print(len([n for n in dir(cv2) if n.startswith('COLOR_')]))"                               # 374
python3 -c "import numpy as np; print(len([n for n in dir(np.ndarray) if not n.startswith('_')]))"                  # 74
```

`dir(cv2)` yields **2413 public names**, of which **668 are callable** and the rest are
enum constants (374 of them are `COLOR_*` conversion codes alone). There are a further
**28 public submodules** (`cv2.aruco`, `cv2.ximgproc`, `cv2.dnn`, `cv2.quality`,
`cv2.cuda`, …) whose contents are not counted at all. The `Mat` type itself is a numpy
array on the Python side, so its surface is `dir(numpy.ndarray)` — 74 more public
attributes plus 91 callable dunders.

**This harness scores only over what it actually compared.** Of the 668 top-level cv2
callables, the cases reference **122**; the other 546 (`calibrateCamera`,
`calcOpticalFlowPyrLK`, `solvePnP`, `stereoRectify`, `seamlessClone`, `dnn_*`,
`aruco_*`, the whole `createBackgroundSubtractor*`/`createTracker*`/`createStitcher`
family, …) are **untested here** and are not in the denominator. The parity percentage
below is therefore a statement about the compared core — colour, threshold, filter,
edge, morphology, warp, pyramid, contour, connected-component, Hough, template,
histogram and `Mat` surface — and nothing more.

Symbol-level counts over the compared surface (one row per
`upstreamFn` × `goFn` pair, derived from the case files and `parity.json`):

| status | pairs |
| --- | --- |
| `match` (every case agreed) | 126 |
| `differs` (at least one case disagreed) | 66 |
| `differs (deliberate)` — documented deviation, counted separately | 1 |
| **compared total** | **193** |
| `untested` | 546 top-level cv2 callables + 28 submodules + the whole enum surface |

## Scale of what the port lacks (and what it adds)

The port is large in its own right — 93 packages and ~5 900 exported functions and
methods (`find … -name '*.go' | grep -c '^func [A-Z]'`) — but it is *not* a
transliteration of OpenCV, and the two surfaces line up only in the imgproc/core core:

- **`Mat` is one fixed type**: `struct{Rows, Cols, Channels int; Data []uint8}`. There
  is no depth/dtype system at all — no `CV_16U`, `CV_32F` or `CV_64F` image, no
  `convertTo`, no `ddepth` argument anywhere. Float results are a *separate*
  `FloatMat` type (`float64`, single channel), so a great deal of OpenCV's API shape
  simply cannot be expressed.
- **No ROI aliasing.** `Mat.Region` returns a deep copy where an OpenCV ROI shares
  memory. This is documented in the port and is the harness's single
  `deviation` (`mat-region-writes-through`).
- **Enums are tiny.** 13 `ColorConversionCode`s against cv2's 374 `COLOR_*` codes;
  2 `InterpolationFlag`s (nearest, linear — no cubic/area/Lanczos); 4
  `TemplateMatchMode`s (cv2 has 6; `TM_CCORR` and `TM_CCORR_NORMED` are absent, which
  is what `matchtemplate-ccorr*` records); 5 `ThresholdType`s; 3 `MorphShape`s.
- **No border argument anywhere.** Every filter hard-codes edge replication where
  cv2's default is `BORDER_REFLECT_101` (see the border section below).
- **Aperture limits.** `Sobel`, `SobelFloat` and `Laplacian` panic for `ksize` 5 or 7
  (`sobel-*-k5`, `laplacian-k5`, `harris-2-5`), which cv2 supports.
- **Go-only surface (`extra`).** `RGBToCMYK`/`CMYKToRGB`, `GammaCorrect`, `Entropy`,
  `Median`, `MSE`, `StackBlur`-style helpers, `plot.*` (21 colormaps, plot types,
  legends), `quality.*` (SSIM/FSIM/VIF/NIQE/PIQE/BRISQUE and ~30 more),
  `segmentation.*` (SLIC, RAG, IntelligentScissors, MultiOtsu, …) have no
  counterpart in base cv2 (several live in `opencv-contrib`). Where a defensible
  reference definition exists the harness supplies one and says so in the
  `upstreamFn` column (`numpy.median`, Shannon entropy, Tenengrad, Bresenham);
  `segmentation.MultiOtsu` and the rest of `quality`/`plot` are left **untested**
  rather than scored against a hand-written oracle.

## Comparison modes and tolerances

Two independent implementations of the same image operation can legitimately differ in
the last bit, so **every case declares its comparison mode** and the harness enforces
it (`parity_test.go`). Counts: **321 `exact`, 353 `tolerance`, 77 `structural`**.

### `exact` — integer-valued, well-defined operations

Buffers are compared byte-for-byte; numbers with a 1e-12 relative/absolute epsilon that
absorbs JSON formatting and nothing else. Used for: all five threshold types and Otsu,
bitwise logic, `inRange`, `LUT`, morphology with an explicit structuring element,
`getStructuringElement`, connected-component labels/counts, `calcHist`, `Mat`
shape/type/ROI semantics, `copyMakeBorder`, `borderInterpolate`, `flip`/`rotate`/
`transpose`, `medianBlur` (the median of integers is exactly defined), and the four
known-finding families.

### `tolerance` — filters, geometric transforms and float results

For **uint8 images** the shape must match exactly and then *four* things are checked:

1. per-sample `|Δ| <= tol`, with at most a fraction `frac` of samples allowed to
   exceed it;
2. the mean absolute difference over the whole image `<= meanTol`;
3. per-channel **summary statistics** — mean within `meanTol`, min and max within
   `tol`;
4. a per-channel **32-bin histogram** whose L1 distance (normalised to [0,1]) must be
   `<= histTol` (default 0.05).

For **float matrices** each element must satisfy `|Δ| <= tol` **or**
`|Δ| <= relTol·max(|a|,|b|)`, again with at most `frac` allowed to fail.
Non-finite values travel as the sentinel strings `"NaN"`, `"Infinity"`, `"-Infinity"`
and are compared exactly.

Budgets actually used, and why:

| family | budget | justification |
| --- | --- | --- |
| 8-bit colour conversions (forward) | `tol=1`, `meanTol=1.0` | cv2 uses fixed-point coefficients; a 1-LSB rounding difference per sample, and up to 1 LSB of systematic per-channel bias, is not a behavioural difference |
| 8-bit colour conversions (inverse), XYZ/YUV/HSV-full | `tol=2`, `meanTol=1.5` | two roundings compose |
| box / mean filters, `filter2D`, `sepFilter2D` | `tol=1`, `meanTol=0.5` | integer mean vs float accumulate-then-round |
| `GaussianBlur` | `tol=2`, `meanTol=0.6` | cv2 builds an **integer fixed-point** kernel for 8-bit input |
| `bilateralFilter` | `tol=4..6`, `meanTol=1.5..2.0` | cv2 quantises the colour weight into a 256-entry LUT |
| `stackBlur` | `tol=2` | different accumulator width |
| `Sobel`/`Scharr`/`Laplacian` to `CV_8U` | `tol=1`, `meanTol=0.5` | saturating cast rounds at .5 |
| `pyrDown`/`pyrUp` | `tol=1..2`, `meanTol=0.5..1.0` | 5-tap binomial kernel rounding |
| `warpAffine`/`warpPerspective`, linear | `tol=3`, `meanTol=1.0` | cv2 quantises warp coordinates to 1/32 px (`INTER_TAB_SIZE`) |
| `remap`, `resize` | `tol=2`, `meanTol=0.5..0.7` | cv2 quantises the fractional coordinate to 5 bits |
| `warpPolar`/`linearPolar`/`logPolar` | `tol=4`, `meanTol=2.0` | polar resampling compounds coordinate and interpolation rounding |
| `CLAHE` | `tol=8`, `meanTol=3.0`, `histTol=0.08` | clip redistribution plus bilinear tile interpolation |
| `Canny` (binary map) | `tol=0`, `frac=0.03`, `meanTol=10`, `histTol=0.03` | at most 3 % of pixels may flip, from hysteresis tie-breaks |
| `adaptiveThreshold` (binary map) | `tol=0`, `frac=0.02`, `meanTol=8`, `histTol=0.02` | the local mean rounds differently exactly at a tie |
| `matchTemplate` | `tol=2.0`, `relTol=1e-4` | cv2 accumulates in **float32**; SQDIFF over a 6×8 8-bit window reaches ~3e6, where one ulp is ~0.25 |
| contour geometry (`contourArea`, `arcLength`, `pointPolygonTest`) | `tol=1e-4`, `relTol=1e-6` | cv2 computes these in float32 |
| `cartToPolar` | `tol=2e-3`, `relTol=1e-3` | cv2's angle is a polynomial approximation |
| `distanceTransform` | `tol=1e-4`, `relTol=1e-6` | the **exact** transform is requested from cv2 (`DIST_MASK_PRECISE`) rather than its 3×3 chamfer approximation, so this compares distances, not kernels |
| `integral`, `sobelFloat`, `spatialGradient`, `sqrBoxFilter`, linear algebra, moments, `mean`/`norm` | `tol<=1e-6`, `relTol` 1e-9…1e-12 | double-precision identities; nothing is hidden |
| `plot.ColormapTable`/`plot.Table` | `tol=2` | lets the same palette pass through a different rounding while a genuinely different palette still fails |

### `structural` — detectors and contour finders

The runners emit a reduced, order-independent form and the harness then compares it
exactly. No float score is ever compared.

- `findContours`: the count, the hierarchy's top-level/with-parent split, and per
  contour the vertex count, the bounding rect and the **lexicographically sorted point
  set**. Contours are sorted by (vertex count, rect, points), so a different traversal
  start point or winding direction is not a divergence while a different boundary is.
- `connectedComponents*`: labels are **canonicalised by first appearance** in
  row-major order, so only the partition is compared, never the arbitrary numbering.
- `HoughLines`: sorted `(rho, theta)` rounded to 2/4 places. `HoughLinesP`,
  `HoughCircles`, `FASTCorners`, `goodFeaturesToTrack`: sorted integer coordinates and
  the count.
- `minAreaRect`: centre, **sorted** side lengths and area — equivalent
  parameterisations of the same rectangle differ freely in angle and side order.
- `fitLine`: the direction sign is normalised (`vx >= 0`).
- `matchTemplateLoc`: `minLoc`/`maxLoc` and the result shape, not the scores.

## Inputs and image encoding

No image is ever read from disk or compared as encoded bytes.

Every input is generated **analytically and identically on both sides** from the
generator spec carried in the case file: `gradient` (`v = (7x + 5y + 37c) mod 256`),
`checker`, `const`, `shapes` (four analytic discs plus a rectangle), `blobs` (six
rectangles, two of them corner-touching so 4- and 8-connectivity must disagree),
`lines`, `circles`, and `noise`/`gradnoise`. The noise generators use the glibc LCG
`s ← (1103515245·s + 12345) mod 2³¹`, `v = (s >> 16) & 255`, **seeded from the case
file** (`"seed": 20260810`, …) — neither runner ever touches its own RNG. Float inputs
(remap tables, small matrices) come from the same mechanism (`"fgen"`).

Images cross the wire as
`{"kind":"mat","rows":R,"cols":C,"channels":N,"hex":"<raw row-major buffer>"}` and float
matrices as `{"kind":"fmat","rows":R,"cols":C,"data":[…]}`. **No PNG/JPEG encoding is
involved anywhere.**

## Known port findings, confirmed

| finding | cases | outcome |
| --- | --- | --- |
| `plot.ApplyColorMap`/`plot.ColormapTable` panic on 13 of the package's own 21 `Colormap` constants | `applycolormap-shape-*`, `colormaptable-*` | **confirmed.** Exactly the 13 declared in the second `const` block (`ColormapAutumn` … `ColormapTurbo`, i.e. `iota + 8`) panic with `plot: ColormapTable unknown colormap`; the first 8 do not. The Go runner's `recover()` turns each into `ok:false` against a cv2 `ok:true`. **26 of the 168 mismatches are this one bug.** |
| the general entry points `plot.Table`/`plot.Colorize` are the ones that work | `plottable-*`, `colorize-*` | **confirmed.** All 21 constants return a table from `plot.Table` — no panic. 7 of those palettes match cv2 within the 2-level budget (`autumn`, `cool`, `grayscale`, `jet`, `spring`, `summer`, `winter`) and 14 do not (`hot`, `bone`, `hsv`, `viridis`, `plasma`, `ocean`, `rainbow`, `pink`, `parula`, `magma`, `inferno`, `cividis`, `twilight`, `turbo`) — a separate, cosmetic divergence. |
| `segmentation.GrabCut` treats a freshly allocated all-zero mask as "everything is background" and needs a literal `nil` | `grabcut-zero-mask`, `grabcut-nil-mask` | **confirmed.** With a zero mask the port returns `allBackground: true` (0 foreground pixels) where cv2 segments the rect; with `nil` it does segment, but still disagrees with cv2 on the foreground count (the port approximates the min-cut with ICM, which it documents). |
| `BoundingRect` and `MinAreaRect` disagree on inclusive vs exclusive extents | `rect-vs-minarea-square`, `-zig`, `-triangle` | **confirmed.** For the 8×6 point square cv2 reports `boundingRect` 9×7 and `minAreaRect` 8×6; the port reports `minAreaRect` height 6 where cv2 reports 8 for the zig-zag, i.e. the two entry points disagree with each other about whether the extent includes the far pixel. |
| `quality.Sharpness` and `quality.LaplacianVariance` return identical values | `sharpness-is-lapvar-*` | **confirmed.** The Go runner reports `equal: true` for all three images; the upstream runner, comparing two genuinely different measures (Laplacian variance and Tenengrad), reports `false`. The port's `LaplacianVariance` is also ~6× smaller than `cv2.Laplacian(CV_64F).var()` (`laplacian-variance-*`), so both names are wrong, not just one. |

## Every divergence found

Wrong pixel results and panics first.

### Panics where cv2 returns a value (38 cases)

| Go symbol | cases | panic |
| --- | --- | --- |
| `plot.ApplyColorMap`, `plot.ColormapTable` | 26 | `plot: ColormapTable unknown colormap` for 13 of its own 21 constants |
| `cv.Sobel`, `cv.CornerHarris` | 4 | `cv: Sobel supports ksize 1 or 3` — cv2 supports 5 and 7 |
| `cv.Laplacian` | 1 | `cv: Laplacian supports ksize 1 or 3` |
| `cv.MatchTemplate` | 4 | `unknown enum constant: "TmCcorr"` / `"TmCcorrNormed"` — the port has 4 of cv2's 6 modes |
| `cv.MatchTemplate` | 1 | `template larger than source`; OpenCV **swaps** image and template in that case and returns a result |
| `cv.Threshold` | 1 | `requires 1 channels, got 3`; cv2 thresholds multi-channel images sample-wise |
| `cv.GetAffineTransform` | 1 | `requires non-collinear source points`; cv2 returns an all-zero matrix |

### Wrong values

| what | cases | divergence |
| --- | --- | --- |
| **filter border convention** | `blur-cv2-default-border`, `gaussian-cv2-default-border`, `sobel-cv2-default-border`, `laplacian-cv2-default-border` | the port hard-codes edge replication; cv2's default is `BORDER_REFLECT_101` and there is no way to ask the port for it. *Every other filter case deliberately invokes cv2 with `BORDER_REPLICATE`* so this one root cause is isolated into four named cases instead of contaminating 40. |
| `GetStructuringElement(MorphEllipse, …)` | `strel-ellipse-5x5`, `-4x4`, `-1x5`, `-7x7`, and the 4 `erode`/`dilate-ellipse-5-*` cases that use them | the ellipse is rasterised differently — for 5×5 cv2 sets 4 more elements than the port |
| `Erode(…, iterations: 0)` | `erode-zero-iterations` | cv2 returns the source unchanged; the port erodes anyway |
| `MorphologyEx` on 3 channels | `morphex-3ch` | 90 of 6912 samples differ |
| `Canny` | 6 | 3 %+ of edge pixels differ; the 32-bin histogram distance reaches 0.23, i.e. the port finds a materially different number of edges |
| `CornerHarris`, `CornerMinEigenVal`, `PreCornerDetect` | 6 | different by 3–4 **orders of magnitude** (cv2 ~4.7e12 vs port ~-6.8e8) — the response is on a different scale entirely, and the sign differs |
| `DistanceTransform` | `distance-transform-shapes` | disagrees with the exact Euclidean transform |
| `GetGaussianKernel(5, -1)` | `gaussiankernel-5-m1p0` | cv2 derives `sigma = 0.3·((k-1)·0.5 - 1) + 0.8` from ksize when sigma <= 0; the port uses something else (0.0708 vs 0.0625 at tap 0) |
| `GetDerivKernels(0, 0, 3)` | `derivkernels-00-3` | cv2 asserts `dx+dy > 0`; the port returns kernels |
| `Remap` with fractional maps | 4 | mean `|Δ|` 19; the port's sampling/out-of-range rule differs (it returns 0 where cv2 samples) |
| `Resize` nearest-neighbour | `resize-half-nearest`, `resize-3ch-nearest` | mean `|Δ|` 44 — a different index convention, not rounding. Linear resize passes. |
| `WarpPolar`/`LinearPolar`/`LogPolar` | 4 | beyond a 4-LSB budget |
| `WarpAffine`/`WarpPerspective`, linear, non-trivial matrix | 2 | beyond a 3-LSB budget |
| `PyrDown`/`PyrUp` | 8 | 1-LSB budget exceeded at the borders (cv2 hard-codes `BORDER_REFLECT_101` inside the pyramid and the port replicates) |
| `CvtColor` HSV→RGB, HLS→RGB | 2 | max `|Δ|` 232, mean 11 — ~20 % of samples are badly wrong, not rounding |
| `Demosaic` (all 4 Bayer patterns) | 4 | max `|Δ|` 185–249; 12 % of samples exceed 2 LSB |
| `TriangleThreshold` | 5 | a different threshold entirely (`triangle-shapes`: cv2 2, port 0) |
| `CLAHE` | 4 | mean `|Δ|` 19 at every clip limit and tile size |
| `CalcBackProject` | 2 | the port does not perform a plain table lookup |
| `ApproxPolyDP` | 4 | returns one vertex fewer than cv2 on every closed polygon tested |
| `Ellipse2Poly` with an arc | `ellipse2poly-arc` | 11 points vs cv2's 12 |
| `ClipLine` | `clipline-crossing` | clipped endpoint off by one |
| `MatchShapes` | `matchshapes-blobs-shapes` | 0.109 vs cv2's 0.044 |
| `MatchTemplate` SQDIFF_NORMED | `matchtemplate-sqdiffnormed` | the port returns values **above 1** (3.16), which a normalised SQDIFF cannot produce |
| `MatchTemplate` CCOEFF / SQDIFF_NORMED peak location | 2 | different `maxLoc` |
| `HoughLines`, `HoughLinesP`, `HoughCircles` | 10 | different line counts (21 vs 10) and different circle counts |
| `FASTCorners` with non-max suppression | 3 | 67 corners vs cv2's 29 — the suppression is not equivalent. Without suppression it matches. |
| `goodFeaturesToTrack` | 1 | one differing corner |
| `ConnectedComponentsWithStats` background centroid | 3 | cv2 reports `NaN` when there are no background pixels; the port reports 0 |
| `Median` | `median-value-noise` | the port picks the lower of the two middle samples where `numpy.median` averages them (127 vs 127.5) |
| `quality.PSNR` of identical images | `psnr-identical` | the port returns `+Inf`; `cv2.PSNR` caps at ~361 dB |
| `Mat.CopyTo` out of range | `mat-copy-to-oob` | the port silently clips; a numpy slice assignment raises |
| `plot.Table` palettes | 14 | 14 of the 21 palettes differ from cv2's by more than 2 levels (`hot`, `bone`, `hsv`, `viridis`, `plasma`, `ocean`, `rainbow`, `pink`, `parula`, `magma`, `inferno`, `cividis`, `twilight`, `turbo`) |
| `plot.ColormapTable` palettes | 18 | 13 panics plus 5 palette differences (`hot`, `bone`, `hsv`, `viridis`, `plasma`) |

### Deliberate deviation (1 case, not counted as a bug)

| case | deviation |
| --- | --- |
| `mat-region-writes-through` | `cv.Mat.Region` returns a deep copy; an OpenCV/numpy ROI aliases the parent. The port documents this explicitly. |

## Per-group parity

| group | cases | parity |
| --- | --- | --- |
| arithmetic | 40 | 100.00 % |
| linalg | 29 | 100.00 % |
| mat | 79 | 98.72 % |
| threshold | 82 | 92.68 % |
| contours | 84 | 89.29 % |
| morphology | 75 | 86.67 % |
| filters | 42 | 85.71 % |
| histogram | 43 | 83.72 % |
| connected | 16 | 81.25 % |
| colour | 27 | 77.78 % |
| geometric | 44 | 70.45 % |
| edges | 54 | 61.11 % |
| template | 20 | 60.00 % |
| findings | 85 | 34.12 % |
| hough | 20 | 30.00 % |
| pyramids | 11 | 27.27 % |
| **total** | **751** | **77.60 %** (denominator 750; the 1 deviation is excluded) |

## Symbols with cases

One row per `upstreamFn` × `goFn` pair. `match` means every case for that pair agreed;
`differs` means at least one did not. A pair with no case would be `untested` and never
appears as `match` — see the untested discussion above for the 546 top-level cv2
callables and 28 submodules in that state.

| upstream symbol | Go symbol | status | mode | cases | failing cases |
| --- | --- | --- | --- | --- | --- |
| `cv2.add` | `cv.Add` | match | exact | 3 — `add-sat`, `add-3ch`, `add-shape-mismatch` |  |
| `cv2.subtract` | `cv.Subtract` | match | exact | 1 — `subtract-sat` |  |
| `cv2.absdiff` | `cv.AbsDiff` | match | exact | 2 — `absdiff`, `absdiff-3ch` |  |
| `cv2.multiply` | `cv.Multiply` | match | exact/tolerance | 2 — `multiply-scale-1`, `multiply-scale-small` |  |
| `cv2.divide` | `cv.Divide` | match | exact/tolerance | 2 — `divide-scale-255`, `divide-by-zero-region` |  |
| `cv2.addWeighted` | `cv.AddWeighted` | match | tolerance | 2 — `add-weighted`, `add-weighted-negative` |  |
| `cv2.bitwise_and` | `cv.BitwiseAnd` | match | exact | 1 — `bitwise-and` |  |
| `cv2.bitwise_or` | `cv.BitwiseOr` | match | exact | 1 — `bitwise-or` |  |
| `cv2.bitwise_xor` | `cv.BitwiseXor` | match | exact | 1 — `bitwise-xor` |  |
| `cv2.bitwise_not` | `cv.BitwiseNot` | match | exact | 1 — `bitwise-not` |  |
| `cv2.min` | `cv.Min` | match | exact | 1 — `min-mat` |  |
| `cv2.max` | `cv.Max` | match | exact | 1 — `max-mat` |  |
| `cv2.inRange` | `cv.InRange` | match | exact | 3 — `in-range-1ch`, `in-range-3ch`, `in-range-empty` |  |
| `cv2.convertScaleAbs` | `cv.ConvertScaleAbs` | match | exact/tolerance | 2 — `convert-scale-abs`, `convert-scale-abs-negate` |  |
| `cv2.normalize` | `cv.Normalize` | match | exact | 3 — `normalize-minmax`, `normalize-minmax-narrow`, `normalize-constant` |  |
| `cv2.countNonZero` | `cv.CountNonZero` | match | exact | 2 — `count-non-zero`, `count-non-zero-3ch` |  |
| `cv2.findNonZero` | `cv.FindNonZero` | match | exact | 1 — `find-non-zero` |  |
| `cv2.mean` | `cv.Mean` | match | tolerance | 2 — `mean-1ch`, `mean-3ch` |  |
| `cv2.meanStdDev` | `cv.MeanStdDev` | match | tolerance | 1 — `mean-stddev` |  |
| `cv2.sumElems` | `cv.SumElems` | match | tolerance | 1 — `sum-elems` |  |
| `cv2.norm` | `cv.NormL1Mat` | match | tolerance | 1 — `norm-l1` |  |
| `cv2.norm` | `cv.NormL2Mat` | match | tolerance | 1 — `norm-l2` |  |
| `cv2.norm` | `cv.NormInfMat` | match | tolerance | 1 — `norm-inf` |  |
| `cv2.LUT` | `cv.LUT` | match | exact | 3 — `lut-invert`, `lut-posterise`, `lut-short-table` |  |
| `cv2.LUT per plane` | `cv.LUTChannels` | match | exact | 1 — `lut-channels` |  |
| `cv2.cvtColor/ColorRGB2Gray` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-gray` |  |
| `cv2.cvtColor/ColorBGR2Gray` | `cv.CvtColor` | match | tolerance | 1 — `cvt-bgr-to-gray` |  |
| `cv2.cvtColor/ColorRGB2BGR` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-bgr` |  |
| `cv2.cvtColor/ColorBGR2RGB` | `cv.CvtColor` | match | tolerance | 1 — `cvt-bgr-to-rgb` |  |
| `cv2.cvtColor/ColorRGB2HSV` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-hsv` |  |
| `cv2.cvtColor/ColorRGB2Lab` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-lab` |  |
| `cv2.cvtColor/ColorRGB2YCrCb` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-ycrcb` |  |
| `cv2.cvtColor/ColorRGB2HLS` | `cv.CvtColor` | match | tolerance | 1 — `cvt-rgb-to-hls` |  |
| `cv2.cvtColor/ColorHSV2RGB` | `cv.CvtColor` | differs | tolerance | 1 — `cvt-hsv-to-rgb` | `cvt-hsv-to-rgb` |
| `cv2.cvtColor/ColorLab2RGB` | `cv.CvtColor` | match | tolerance | 1 — `cvt-lab-to-rgb` |  |
| `cv2.cvtColor/ColorYCrCb2RGB` | `cv.CvtColor` | match | tolerance | 1 — `cvt-ycrcb-to-rgb` |  |
| `cv2.cvtColor/ColorHLS2RGB` | `cv.CvtColor` | differs | tolerance | 1 — `cvt-hls-to-rgb` | `cvt-hls-to-rgb` |
| `cv2.cvtColor/COLOR_GRAY2RGB` | `cv.CvtColor` | match | exact | 1 — `cvt-gray-to-rgb` |  |
| `cv2.cvtColor` | `cv.CvtColor` | match | exact/tolerance | 3 — `cvt-checker-rgb-to-hsv`, `cvt-noise-rgb-to-lab`, `cvt-gray-input-to-hsv` |  |
| `cv2.cvtColor/COLOR_RGB2XYZ` | `cv.RGBToXYZ` | match | tolerance | 1 — `rgb-to-xyz` |  |
| `cv2.cvtColor/COLOR_XYZ2RGB` | `cv.XYZToRGB` | match | tolerance | 1 — `xyz-to-rgb` |  |
| `cv2.cvtColor/COLOR_RGB2YUV` | `cv.RGBToYUV` | match | tolerance | 1 — `rgb-to-yuv` |  |
| `cv2.cvtColor/COLOR_YUV2RGB` | `cv.YUVToRGB` | match | tolerance | 1 — `yuv-to-rgb` |  |
| `cv2.cvtColor/COLOR_RGB2HSV_FULL` | `cv.RGBToHSVFull` | match | tolerance | 1 — `rgb-to-hsv-full` |  |
| `cv2.cvtColor/COLOR_HSV2RGB_FULL` | `cv.HSVFullToRGB` | match | tolerance | 1 — `hsv-full-to-rgb` |  |
| `cv2.cvtColor/COLOR_RGB2GRAY` | `cv.RGBToGray601` | match | tolerance | 1 — `rgb-to-gray-601` |  |
| `cv2.cvtColor/COLOR_RG2RGB` | `cv.Demosaic` | differs | tolerance | 1 — `demosaic-bayerrg` | `demosaic-bayerrg` |
| `cv2.cvtColor/COLOR_GR2RGB` | `cv.Demosaic` | differs | tolerance | 1 — `demosaic-bayergr` | `demosaic-bayergr` |
| `cv2.cvtColor/COLOR_BG2RGB` | `cv.Demosaic` | differs | tolerance | 1 — `demosaic-bayerbg` | `demosaic-bayerbg` |
| `cv2.cvtColor/COLOR_GB2RGB` | `cv.Demosaic` | differs | tolerance | 1 — `demosaic-bayergb` | `demosaic-bayergb` |
| `cv2.connectedComponents` | `cv.ConnectedComponents` | match | exact | 9 — `cc-4-blobs`, `cc-4-checker`, `cc-4-circles` +6 more |  |
| `cv2.connectedComponentsWithStats` | `cv.ConnectedComponentsWithStats` | differs | tolerance | 7 — `ccstats-4-blobs`, `ccstats-4-checker`, `ccstats-4-circles` +4 more | `ccstats-4-checker`, `ccstats-8-checker`, `ccstats-8-all-set` |
| `cv2.findContours` | `cv.FindContours` | match | structural | 24 — `findcontours-external-none-blobs`, `findcontours-external-none-shapes`, `findcontours-external-none-circles` +21 more |  |
| `cv2.contourArea` | `cv.ContourArea` | match | tolerance | 3 — `contourarea-square`, `contourarea-triangle`, `contourarea-degenerate` |  |
| `cv2.arcLength` | `cv.ArcLength` | match | tolerance | 3 — `arclength-square-closed`, `arclength-square-open`, `arclength-triangle` |  |
| `cv2.approxPolyDP` | `cv.ApproxPolyDP` | differs | structural | 7 — `approxpoly-zig-0p5-closed`, `approxpoly-zig-0p5-open`, `approxpoly-zig-1p5-closed` +4 more | `approxpoly-zig-0p5-closed`, `approxpoly-zig-1p5-closed`, `approxpoly-zig-4p0-closed`, `approxpoly-square` |
| `cv2.convexHull` | `cv.ConvexHull` | match | structural | 3 — `convexhull-zig`, `convexhull-square`, `convexhull-collinear` |  |
| `cv2.isContourConvex` | `cv.IsContourConvex` | match | exact | 3 — `iscontourconvex-square`, `iscontourconvex-zig`, `iscontourconvex-triangle` |  |
| `cv2.boundingRect` | `cv.BoundingRect` | match | exact | 3 — `boundingrect-square`, `boundingrect-zig`, `boundingrect-single-point` |  |
| `cv2.minAreaRect` | `cv.MinAreaRect` | match | structural | 3 — `minarearect-square`, `minarearect-zig`, `minarearect-triangle` |  |
| `cv2.boundingRect vs cv2.minAreaRect` | `cv.BoundingRect vs cv.MinAreaRect` | differs | structural | 3 — `rect-vs-minarea-square`, `rect-vs-minarea-zig`, `rect-vs-minarea-triangle` | `rect-vs-minarea-square`, `rect-vs-minarea-zig` |
| `cv2.boxPoints` | `cv.BoxPoints` | match | structural | 2 — `boxpoints-axis-aligned`, `boxpoints-rotated` |  |
| `cv2.minEnclosingCircle` | `cv.MinEnclosingCircle` | match | structural | 2 — `minenclosingcircle-square`, `minenclosingcircle-zig` |  |
| `cv2.pointPolygonTest` | `cv.PointPolygonTest` | match | tolerance | 5 — `pointpolygontest-6-5-inside`, `pointpolygontest-6-5-dist`, `pointpolygontest-0-0-dist` +2 more |  |
| `cv2.fitLine` | `cv.FitLine` | match | tolerance | 2 — `fitline-diagonal`, `fitline-noisy` |  |
| `cv2.moments` | `cv.ImageMoments` | match | tolerance | 3 — `moments-blobs`, `moments-shapes`, `moments-gradient` |  |
| `cv2.HuMoments` | `cv.HuMoments` | match | tolerance | 3 — `humoments-blobs`, `humoments-shapes`, `humoments-gradient` |  |
| `cv2.moments centroid` | `cv.Moments.Centroid` | match | tolerance | 3 — `centroid-blobs`, `centroid-shapes`, `centroid-gradient` |  |
| `cv2.matchShapes` | `cv.MatchShapes` | differs | tolerance | 2 — `matchshapes-blobs-shapes`, `matchshapes-self` | `matchshapes-blobs-shapes` |
| `cv2.ellipse2Poly` | `cv.Ellipse2Poly` | differs | structural | 2 — `ellipse2poly-full`, `ellipse2poly-arc` | `ellipse2poly-arc` |
| `cv2.clipLine` | `cv.ClipLine` | differs | exact | 3 — `clipline-inside`, `clipline-crossing`, `clipline-outside` | `clipline-crossing` |
| `Bresenham (cv::LineIterator is not exposed to Python)` | `cv.NewLineIterator` | match | structural | 5 — `lineiterator-0-0-10-4`, `lineiterator-0-0-4-10`, `lineiterator-5-5-0-0` +2 more |  |
| `cv2.Sobel` | `cv.Sobel` | differs | exact/tolerance | 10 — `sobel-10-k3`, `sobel-01-k3`, `sobel-11-k3` +7 more | `sobel-10-k5`, `sobel-01-k5`, `sobel-20-k5` |
| `cv2.Sobel (default BORDER_REFLECT_101)` | `cv.Sobel` | differs | tolerance | 1 — `sobel-cv2-default-border` | `sobel-cv2-default-border` |
| `cv2.Sobel/CV_64F` | `cv.SobelFloat` | match | tolerance | 3 — `sobelfloat-10-k3`, `sobelfloat-01-k3`, `sobelfloat-11-k3` |  |
| `cv2.Scharr` | `cv.Scharr` | match | exact/tolerance | 3 — `scharr-10`, `scharr-01`, `scharr-invalid` |  |
| `cv2.Laplacian` | `cv.Laplacian` | differs | tolerance | 5 — `laplacian-k1`, `laplacian-k3`, `laplacian-k5` +2 more | `laplacian-k5` |
| `cv2.Laplacian (default BORDER_REFLECT_101)` | `cv.Laplacian` | differs | tolerance | 1 — `laplacian-cv2-default-border` | `laplacian-cv2-default-border` |
| `cv2.Canny` | `cv.Canny` | differs | tolerance | 6 — `canny-shapes-50-150`, `canny-shapes-high-100-200`, `canny-gradnoise-20-60` +3 more | `canny-shapes-50-150`, `canny-shapes-high-100-200`, `canny-gradnoise-20-60`, `canny-checker-80-160`, `canny-circles-30-90`, `canny-inverted-thresholds` |
| `cv2.spatialGradient` | `cv.SpatialGradient` | match | tolerance | 1 — `spatial-gradient` |  |
| `cv2.cornerHarris` | `cv.CornerHarris` | differs | tolerance | 3 — `harris-2-3`, `harris-3-3`, `harris-2-5` | `harris-2-3`, `harris-3-3`, `harris-2-5` |
| `cv2.cornerMinEigenVal` | `cv.CornerMinEigenVal` | differs | tolerance | 2 — `mineigenval-2-3`, `mineigenval-3-3` | `mineigenval-2-3`, `mineigenval-3-3` |
| `cv2.preCornerDetect` | `cv.PreCornerDetect` | differs | tolerance | 1 — `precorner-detect` | `precorner-detect` |
| `cv2.getDerivKernels` | `cv.GetDerivKernels` | differs | tolerance | 6 — `derivkernels-10-3`, `derivkernels-01-3`, `derivkernels-10-5` +3 more | `derivkernels-00-3` |
| `cv2.getGaussianKernel` | `cv.GetGaussianKernel` | differs | tolerance | 4 — `gaussiankernel-3-0p8`, `gaussiankernel-5-1p0`, `gaussiankernel-5-m1p0` +1 more | `gaussiankernel-5-m1p0` |
| `cv2.getGaborKernel` | `cv.GetGaborKernel` | match | tolerance | 1 — `gaborkernel` |  |
| `cv2.integral` | `cv.Integral` | match | tolerance | 1 — `integral` |  |
| `cv2.integral2` | `cv.IntegralSquared` | match | tolerance | 1 — `integral-squared` |  |
| `cv2.distanceTransform/DIST_L2 + DIST_MASK_PRECISE` | `cv.DistanceTransform` | differs | tolerance | 2 — `distance-transform-blobs`, `distance-transform-shapes` | `distance-transform-shapes` |
| `cv2.floodFill` | `cv.FloodFill` | match | exact | 3 — `floodfill-shapes`, `floodfill-background`, `floodfill-oob-seed` |  |
| `cv2.blur` | `cv.Blur` | match | tolerance | 6 — `blur-3`, `blur-3ch-3`, `blur-5` +3 more |  |
| `cv2.boxFilter` | `cv.BoxFilter` | match | tolerance | 4 — `boxfilter-3-norm`, `boxfilter-3-raw`, `boxfilter-5-norm` +1 more |  |
| `cv2.GaussianBlur` | `cv.GaussianBlur` | match | exact/tolerance | 7 — `gaussian-3-0p8`, `gaussian-3-1p5`, `gaussian-5-1p0` +4 more |  |
| `cv2.blur (default BORDER_REFLECT_101)` | `cv.Blur` | differs | tolerance | 1 — `blur-cv2-default-border` | `blur-cv2-default-border` |
| `cv2.GaussianBlur (default BORDER_REFLECT_101)` | `cv.GaussianBlur` | differs | tolerance | 1 — `gaussian-cv2-default-border` | `gaussian-cv2-default-border` |
| `cv2.medianBlur` | `cv.MedianBlur` | match | exact | 5 — `median-3`, `median-3ch-3`, `median-5` +2 more |  |
| `cv2.bilateralFilter` | `cv.BilateralFilter` | differs | tolerance | 4 — `bilateral-5-30-10`, `bilateral-5-75-75`, `bilateral-9-50-20` +1 more | `bilateral-5-30-10`, `bilateral-5-75-75`, `bilateral-9-50-20`, `bilateral-3ch` |
| `cv2.filter2D` | `cv.Filter2D` | match | exact/tolerance | 6 — `filter2d-identity`, `filter2d-mean`, `filter2d-sharpen` +3 more |  |
| `cv2.sepFilter2D` | `cv.Filter2DSep` | match | tolerance | 2 — `sepfilter-binomial`, `sepfilter-asymmetric` |  |
| `cv2.sqrBoxFilter` | `cv.SqrBoxFilter` | match | tolerance | 4 — `sqrbox-3-norm`, `sqrbox-3-raw`, `sqrbox-5-norm` +1 more |  |
| `cv2.stackBlur` | `cv.StackBlur` | match | tolerance | 2 — `stackblur-3`, `stackblur-5` |  |
| `cv2.applyColorMap` | `plot.ApplyColorMap` | differs | exact | 21 — `applycolormap-shape-jet`, `applycolormap-shape-hot`, `applycolormap-shape-cool` +18 more | `applycolormap-shape-autumn`, `applycolormap-shape-winter`, `applycolormap-shape-summer`, `applycolormap-shape-spring`, `applycolormap-shape-ocean`, `applycolormap-shape-rainbow` +7 more |
| `cv2.applyColorMap over a 0..255 ramp` | `plot.ColormapTable` | differs | tolerance | 21 — `colormaptable-jet`, `colormaptable-hot`, `colormaptable-cool` +18 more | `colormaptable-hot`, `colormaptable-bone`, `colormaptable-hsv`, `colormaptable-viridis`, `colormaptable-plasma`, `colormaptable-autumn` +12 more |
| `cv2.applyColorMap over a 0..255 ramp` | `plot.Table` | differs | tolerance | 21 — `plottable-jet`, `plottable-hot`, `plottable-cool` +18 more | `plottable-hot`, `plottable-bone`, `plottable-hsv`, `plottable-viridis`, `plottable-plasma`, `plottable-ocean` +8 more |
| `cv2.applyColorMap` | `plot.Colorize` | differs | exact/tolerance | 8 — `colorize-shape-jet`, `colorize-jet`, `colorize-shape-viridis` +5 more | `colorize-viridis`, `colorize-turbo` |
| `LUT over a caller-supplied table` | `plot.ApplyCustomColorMap` | match | exact | 1 — `applycustomcolormap` |  |
| `cv2.grabCut/GC_INIT_WITH_RECT` | `segmentation.GrabCut` | differs | exact | 2 — `grabcut-zero-mask`, `grabcut-nil-mask` | `grabcut-zero-mask`, `grabcut-nil-mask` |
| `cv2.grabCut` | `segmentation.GrabCut` | match | exact | 1 — `grabcut-nil-mask-1ch` |  |
| `Laplacian variance vs Tenengrad` | `quality.Sharpness vs quality.LaplacianVariance` | differs | exact | 3 — `sharpness-is-lapvar-gradnoise`, `sharpness-is-lapvar-shapes`, `sharpness-is-lapvar-noise` | `sharpness-is-lapvar-gradnoise`, `sharpness-is-lapvar-shapes`, `sharpness-is-lapvar-noise` |
| `cv2.Laplacian(CV_64F).var()` | `quality.LaplacianVariance` | differs | tolerance | 3 — `laplacian-variance-gradnoise`, `laplacian-variance-shapes`, `laplacian-variance-noise` | `laplacian-variance-gradnoise`, `laplacian-variance-shapes`, `laplacian-variance-noise` |
| `cv2.PSNR` | `quality.PSNR` | differs | tolerance | 2 — `psnr-gradient-noise`, `psnr-identical` | `psnr-identical` |
| `mean squared difference` | `cv.MSE` | match | tolerance | 2 — `mse-gradient-noise`, `mse-identical` |  |
| `cv2.getRotationMatrix2D` | `cv.GetRotationMatrix2D` | match | tolerance | 3 — `rotmat-90`, `rotmat-30-scaled`, `rotmat-negative` |  |
| `cv2.getAffineTransform` | `cv.GetAffineTransform` | differs | tolerance | 2 — `affine-from-points`, `affine-degenerate` | `affine-degenerate` |
| `cv2.invertAffineTransform` | `cv.InvertAffineTransform` | match | tolerance | 1 — `affine-invert` |  |
| `cv2.getPerspectiveTransform` | `cv.GetPerspectiveTransform` | match | tolerance | 1 — `perspective-from-points` |  |
| `cv2.perspectiveTransform` | `cv.PerspectiveTransform` | match | tolerance | 1 — `perspective-transform-points` |  |
| `cv2.warpAffine` | `cv.WarpAffine` | differs | exact/tolerance | 8 — `warpaffine-identity-nearest`, `warpaffine-shift-nearest`, `warpaffine-rotate-nearest` +5 more | `warpaffine-rotate-linear` |
| `cv2.warpPerspective` | `cv.WarpPerspective` | differs | exact/tolerance | 4 — `warpperspective-identity-nearest`, `warpperspective-skew-nearest`, `warpperspective-identity-linear` +1 more | `warpperspective-skew-linear` |
| `cv2.remap` | `cv.Remap` | differs | exact/tolerance | 8 — `remap-identity-nearest`, `remap-flip-nearest`, `remap-shift-nearest` +5 more | `remap-shift-nearest`, `remap-wobble-nearest`, `remap-shift-linear`, `remap-wobble-linear` |
| `cv2.resize` | `cv.Resize` | differs | exact/tolerance | 9 — `resize-half-nearest`, `resize-double-nearest`, `resize-same-nearest` +6 more | `resize-half-nearest`, `resize-3ch-nearest` |
| `cv2.getRectSubPix` | `cv.GetRectSubPix` | match | tolerance | 2 — `rectsubpix-centre`, `rectsubpix-integer-centre` |  |
| `cv2.warpPolar` | `cv.WarpPolar` | differs | tolerance | 2 — `warppolar-linear`, `warppolar-log` | `warppolar-linear`, `warppolar-log` |
| `cv2.warpPolar` | `cv.LinearPolar` | differs | tolerance | 1 — `linearpolar` | `linearpolar` |
| `cv2.warpPolar|WARP_POLAR_LOG` | `cv.LogPolar` | differs | tolerance | 1 — `logpolar` | `logpolar` |
| `cv2.transform` | `cv.Transform` | match | tolerance | 1 — `transform-3x3` |  |
| `cv2.calcHist` | `cv.CalcHist` | match | exact | 6 — `calchist-gradient`, `calchist-noise`, `calchist-shapes` +3 more |  |
| `cv2.compareHist` | `cv.CompareHist` | match | exact/tolerance | 17 — `comparehist-correl`, `comparehist-self-correl`, `comparehist-zero-correl` +14 more |  |
| `cv2.equalizeHist` | `cv.EqualizeHist` | match | exact | 5 — `equalizehist-gradient`, `equalizehist-noise`, `equalizehist-shapes` +2 more |  |
| `cv2.createCLAHE` | `cv.CLAHE` | differs | tolerance | 5 — `clahe-2-4`, `clahe-4-8`, `clahe-1-2` +2 more | `clahe-2-4`, `clahe-4-8`, `clahe-1-2`, `clahe-40-4` |
| `cv2.calcBackProject` | `cv.CalcBackProject` | differs | exact | 2 — `backproject-gradient`, `backproject-3ch-ch1` | `backproject-gradient`, `backproject-3ch-ch1` |
| `Shannon entropy of the 256-bin histogram` | `cv.Entropy` | match | tolerance | 4 — `entropy-gradient`, `entropy-noise`, `entropy-const` +1 more |  |
| `numpy.median` | `cv.Median` | differs | tolerance | 4 — `median-value-gradient`, `median-value-noise`, `median-value-const` +1 more | `median-value-noise` |
| `cv2.HoughLines` | `cv.HoughLines` | differs | structural | 5 — `houghlines-10`, `houghlines-20`, `houghlines-30` +2 more | `houghlines-10`, `houghlines-20`, `houghlines-30`, `houghlines-coarse` |
| `cv2.HoughLinesP` | `cv.HoughLinesP` | differs | structural | 3 — `houghlinesp-10-5-2`, `houghlinesp-20-10-3`, `houghlinesp-15-20-1` | `houghlinesp-10-5-2`, `houghlinesp-20-10-3`, `houghlinesp-15-20-1` |
| `cv2.HoughCircles` | `cv.HoughCircles` | differs | structural | 3 — `houghcircles-100-20`, `houghcircles-100-30`, `houghcircles-50-15` | `houghcircles-100-20`, `houghcircles-100-30`, `houghcircles-50-15` |
| `cv2.FastFeatureDetector` | `cv.FASTCorners` | differs | structural | 6 — `fast-10-nms`, `fast-10-raw`, `fast-30-nms` +3 more | `fast-10-nms`, `fast-30-nms`, `fast-60-nms` |
| `cv2.goodFeaturesToTrack` | `cv.GoodFeaturesToTrack` | differs | structural | 3 — `goodfeatures-10-0p01`, `goodfeatures-20-0p1`, `goodfeatures-5-0p05` | `goodfeatures-10-0p01` |
| `cv2.determinant` | `cv.Determinant` | match | tolerance | 2 — `determinant-3x3`, `determinant-singular` |  |
| `cv2.invert` | `cv.Invert` | match | tolerance | 2 — `invert-3x3`, `invert-singular` |  |
| `cv2.solve` | `cv.Solve` | match | tolerance | 2 — `solve-3x3`, `solve-singular` |  |
| `cv2.trace` | `cv.Trace` | match | tolerance | 1 — `trace-3x3` |  |
| `cv2.gemm` | `cv.Gemm` | match | tolerance | 3 — `gemm-2x3-3x2`, `gemm-alpha`, `gemm-shape-mismatch` |  |
| `cv2.dct` | `cv.DCT` | match | tolerance | 1 — `dct-4x4` |  |
| `cv2.dft + cv2.magnitude` | `cv.DFT + cv.Magnitude` | match | tolerance | 1 — `dft-magnitude-4x4` |  |
| `cv2.cartToPolar` | `cv.CartToPolar` | match | tolerance | 2 — `cart-to-polar-rad`, `cart-to-polar-deg` |  |
| `cv2.setIdentity` | `cv.SetIdentity` | match | tolerance | 2 — `set-identity-3x3`, `set-identity-nonsquare` |  |
| `cv2.reduce` | `cv.Reduce` | match | tolerance | 8 — `reduce-row-sum`, `reduce-row-avg`, `reduce-row-max` +5 more |  |
| `cv2.repeat` | `cv.Repeat` | match | tolerance | 1 — `repeat-2x3` |  |
| `cv2.sort` | `cv.Sort` | match | tolerance | 4 — `sort-row-desc`, `sort-row-asc`, `sort-col-desc` +1 more |  |
| `numpy.ndarray.shape` | `cv.Mat.Size/Total/Empty` | match | exact | 2 — `mat-info-1ch`, `mat-info-3ch` |  |
| `numpy.zeros` | `cv.NewMat` | match | exact | 4 — `mat-new-zeros`, `mat-new-zeros-3ch`, `mat-new-zero-dim` +1 more |  |
| `numpy.ndarray.__getitem__` | `cv.Mat.At` | match | exact | 3 — `mat-at`, `mat-at-oob-row`, `mat-at-oob-channel` |  |
| `numpy.ndarray.__setitem__` | `cv.Mat.Set` | match | exact | 2 — `mat-set`, `mat-set-oob` |  |
| `numpy.ndarray.__setitem__` | `cv.Mat.SetPixel` | match | exact | 2 — `mat-set-pixel`, `mat-set-pixel-wrong-arity` |  |
| `numpy.ndarray.__getitem__` | `cv.Mat.AtPixel` | match | exact | 1 — `mat-pixel` |  |
| `numpy slicing` | `cv.Mat.Region` | match | exact | 4 — `mat-region`, `mat-region-full`, `mat-region-oob` +1 more |  |
| `numpy slicing (a view)` | `cv.Mat.Region` | differs (deliberate) | exact | 1 — `mat-region-writes-through` | `mat-region-writes-through` |
| `numpy.ndarray.copy` | `cv.Mat.Clone` | match | exact | 2 — `mat-clone`, `mat-clone-isolated` |  |
| `numpy slice assign` | `cv.Mat.CopyTo` | differs | exact | 2 — `mat-copy-to`, `mat-copy-to-oob` | `mat-copy-to-oob` |
| `numpy.ndarray.fill` | `cv.Mat.SetTo` | match | exact | 1 — `mat-set-to` |  |
| `cv2.split` | `cv.Mat.Split` | match | exact | 2 — `mat-split`, `mat-split-1ch` |  |
| `cv2.merge` | `cv.Merge` | match | exact | 2 — `mat-split-merge`, `mat-merge-reversed` |  |
| `numpy slicing` | `cv.ExtractChannel` | match | exact | 3 — `extract-channel-1`, `extract-channel-oob`, `extract-channel-1ch` |  |
| `numpy slice assign` | `cv.InsertChannel` | match | exact | 2 — `insert-channel-0`, `insert-channel-2` |  |
| `cv2.mixChannels` | `cv.MixChannels` | match | exact | 2 — `mix-channels-reverse`, `mix-channels-partial` |  |
| `cv2.transpose` | `cv.Transpose` | match | exact | 2 — `transpose-1ch`, `transpose-3ch` |  |
| `cv2.flip` | `cv.Flip` | match | exact | 3 — `flip-flipvertical`, `flip-fliphorizontal`, `flip-flipboth` |  |
| `cv2.rotate` | `cv.Rotate` | match | exact | 3 — `rotate-rotate90cw`, `rotate-rotate180`, `rotate-rotate90ccw` |  |
| `cv2.copyMakeBorder` | `cv.CopyMakeBorder` | match | exact | 6 — `border-constant`, `border-replicate`, `border-reflect` +3 more |  |
| `cv2.borderInterpolate` | `cv.BorderInterpolate` | match | exact | 30 — `borderinterp-constant--3`, `borderinterp-constant--1`, `borderinterp-constant-0` +27 more |  |
| `cv2.getStructuringElement` | `cv.GetStructuringElement` | differs | exact | 19 — `strel-rect-3x3`, `strel-rect-5x5`, `strel-rect-3x5` +16 more | `strel-ellipse-5x5`, `strel-ellipse-4x4`, `strel-ellipse-1x5`, `strel-ellipse-7x7` |
| `cv2.erode` | `cv.Erode` | differs | exact | 14 — `erode-rect-3-i1`, `erode-rect-3-i2`, `erode-rect-5-i1` +11 more | `erode-ellipse-5-i1`, `erode-ellipse-5-i2`, `erode-zero-iterations` |
| `cv2.dilate` | `cv.Dilate` | differs | exact | 13 — `dilate-rect-3-i1`, `dilate-rect-3-i2`, `dilate-rect-5-i1` +10 more | `dilate-ellipse-5-i1`, `dilate-ellipse-5-i2` |
| `cv2.morphologyEx` | `cv.MorphologyEx` | differs | exact | 29 — `morphex-erode-rect`, `morphex-erode-cross`, `morphex-erode-ellipse` +26 more | `morphex-3ch` |
| `cv2.pyrDown` | `cv.PyrDown` | differs | tolerance | 5 — `pyrdown-gradnoise`, `pyrdown-shapes`, `pyrdown-checker` +2 more | `pyrdown-gradnoise`, `pyrdown-3ch`, `pyrdown-odd-size` |
| `cv2.pyrUp` | `cv.PyrUp` | differs | tolerance | 4 — `pyrup-gradnoise`, `pyrup-shapes`, `pyrup-checker` +1 more | `pyrup-gradnoise`, `pyrup-checker`, `pyrup-3ch` |
| `cv2.pyrDown x2` | `cv.PyrDown x2` | differs | tolerance | 1 — `pyrdown-twice` | `pyrdown-twice` |
| `cv2.pyrUp(cv2.pyrDown)` | `cv.PyrUp(cv.PyrDown)` | differs | tolerance | 1 — `pyrdown-up` | `pyrdown-up` |
| `cv2.matchTemplate/TmSqdiff` | `cv.MatchTemplate` | match | tolerance | 1 — `matchtemplate-sqdiff` |  |
| `cv2.matchTemplate + cv2.minMaxLoc` | `cv.MatchTemplate + cv.MinMaxLoc` | differs | structural | 6 — `matchtemplate-loc-sqdiff`, `matchtemplate-loc-sqdiffnormed`, `matchtemplate-loc-ccoeff` +3 more | `matchtemplate-loc-sqdiffnormed`, `matchtemplate-loc-ccoeff`, `matchtemplate-loc-ccorr`, `matchtemplate-loc-ccorrnormed` |
| `cv2.matchTemplate/TmSqdiffNormed` | `cv.MatchTemplate` | differs | tolerance | 1 — `matchtemplate-sqdiffnormed` | `matchtemplate-sqdiffnormed` |
| `cv2.matchTemplate/TmCcoeff` | `cv.MatchTemplate` | match | tolerance | 1 — `matchtemplate-ccoeff` |  |
| `cv2.matchTemplate/TmCcoeffNormed` | `cv.MatchTemplate` | match | tolerance | 1 — `matchtemplate-ccoeffnormed` |  |
| `cv2.matchTemplate/TmCcorr` | `cv.MatchTemplate` | differs | tolerance | 1 — `matchtemplate-ccorr` | `matchtemplate-ccorr` |
| `cv2.matchTemplate/TmCcorrNormed` | `cv.MatchTemplate` | differs | tolerance | 1 — `matchtemplate-ccorrnormed` | `matchtemplate-ccorrnormed` |
| `cv2.matchTemplate` | `cv.MatchTemplate` | differs | exact/tolerance | 4 — `matchtemplate-noise`, `matchtemplate-3ch`, `matchtemplate-same-size` +1 more | `matchtemplate-template-too-big` |
| `cv2.minMaxLoc` | `cv.MinMaxLoc` | match | tolerance | 1 — `minmaxloc-fmat` |  |
| `cv2.minMaxLoc` | `cv.MinMaxLocMat` | match | tolerance | 3 — `minmaxloc-mat-gradient`, `minmaxloc-mat-noise`, `minmaxloc-mat-const` |  |
| `cv2.threshold` | `cv.Threshold` | differs | exact | 28 — `thresh-binary-0`, `thresh-binary-64`, `thresh-binary-127` +25 more | `thresh-3ch` |
| `cv2.threshold|THRESH_OTSU` | `cv.Threshold|cv.ThreshOtsu` | match | exact | 10 — `otsu-binary-gradient`, `otsu-binary-noise`, `otsu-binary-checker` +7 more |  |
| `cv2.threshold|THRESH_TRIANGLE` | `cv.TriangleThreshold` | differs | exact | 5 — `triangle-gradient`, `triangle-noise`, `triangle-checker` +2 more | `triangle-gradient`, `triangle-noise`, `triangle-checker`, `triangle-shapes`, `triangle-gradnoise` |
| `cv2.adaptiveThreshold` | `cv.AdaptiveThreshold` | match | exact/tolerance | 39 — `adaptive-mea-3-0-binary`, `adaptive-mea-3-0-binaryinv`, `adaptive-mea-3-2-binary` +36 more |  |

**Totals over the compared surface: 193 pairs — 126 `match`, 66 `differs`, 1 `differs (deliberate)`.**

Case totals: **751 cases, 582 match, 168 differ, 1 deliberate deviation, 29 agreed
failures — parity 77.60 % over a denominator of 750.** Everything outside those 193
pairs (546 top-level `cv2` callables, 28 submodules, the 2413-name enum surface, and
the port's own ~5 900 exported functions across 93 packages) is `untested`.
