# `github.com/malcolmston/opencv` — worked example

A single runnable program that exercises the pure-Go OpenCV port as an outside
consumer would: the dependency is resolved from the module proxy, there is **no
`replace` directive**, no network access at run time, and no input image on disk.
Every input is synthesised in memory.

## Module version under test

```
github.com/malcolmston/opencv v0.8.0
```

Resolved with `GOWORK=off go get github.com/malcolmston/opencv@latest`. The repo
does carry semver tags, so this is a real release tag, not a
`v0.0.0-<date>-<sha>` pseudo-version.

> The local working tree at `../../opencv` is **ahead** of v0.8.0 and contains
> exported functions that the published module does not have. This example is
> written against v0.8.0 only; see the holes below.

## Running it

```sh
cd examples/opencv
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

It prints a labelled report to stdout, writes ~41 PNGs plus one serialised model
into `./out/`, and exits 0 on its own. Runtime is a few seconds.

## What it demonstrates

**Root `cv` package**

- **Mat core** — `NewMat`, `Size`/`Total`/`Empty`, `At`/`AtPixel`/`Set`/`SetPixel`,
  `Clone`, `SetTo`, `Split`/`Merge` (green plane zeroed), `Region` + `CopyTo`
  compositing, and the `FromImage` / `ToImage` standard-library bridge (verified
  byte-for-byte lossless).
- **I/O** — `IMEncode`/`IMDecode` for PNG and JPEG buffers, `ImWrite`/`ImRead`
  for files, both round-trip-checked.
- **Colour** — `CvtColor` round-trips through HSV, Lab, YCrCb and HLS with the
  worst-case per-sample error reported for each; `InRange` (two hue bands OR-ed)
  to isolate the red disc.
- **Filtering** — `Blur`, `BoxFilter`, `GaussianBlur` (+ `GaussianKernel1D`),
  `MedianBlur`, `BilateralFilter` compared by output variance on a salt-and-pepper
  image; a custom `Filter2D`/`NewKernel` sharpener; `Filter2DSep` checked to be
  bit-identical to the equivalent `BoxFilter`; `Sobel`, `SobelFloat`, `Scharr`,
  `Laplacian`.
- **Arithmetic** — `Add`/`Subtract`/`AbsDiff`/`AddWeighted`/`BitwiseOr`,
  `Normalize`, `ConvertScaleAbs`, `LUT`.
- **Thresholding** — all five `ThresholdType`s, `ThreshOtsu` (recovered level
  printed), both `AdaptiveThreshold` methods.
- **Morphology** — all three `GetStructuringElement` shapes (cell counts printed),
  `Erode`/`Dilate`, and all five `MorphologyEx` ops.
- **Geometry** — `Resize` (both interpolations), `Flip`, `Rotate`, `Transpose`,
  `GetRotationMatrix2D` + `WarpAffine`, `GetPerspectiveTransform` +
  `WarpPerspective`, `Remap` driven by hand-built `FloatMat` maps, `PyrDown`/
  `PyrUp`, `DistanceTransform` + `MinMaxLoc`.
- **Contours & shape** — `FindContours` across all three retrieval modes and both
  chain approximations (point counts compared), `ContourArea`, `ArcLength`,
  `BoundingRect`, `MinAreaRect` (+ `RotatedRect.Points`), `ConvexHull`,
  `ApproxPolyDP`, `DrawContours`, `Polylines`, `ImageMoments`.
- **Connected components** — 4- vs 8-connectivity counts and
  `ConnectedComponentsWithStats` per-label area / bbox / centroid (centroids land
  exactly on the drawn shape centres).
- **Features** — `Canny`, `CornerHarris`, `GoodFeaturesToTrack`, `FASTCorners`,
  `HoughLines`, `HoughLinesP`, `HoughCircles` (checked against the known disc),
  `MatchTemplate` in all four modes (all four recover the exact ground-truth
  offset).
- **Histograms** — `CalcHist`, `EqualizeHist`, `CLAHE`, all four `CompareHist`
  methods, `CalcBackProject`.
- **Drawing** — `Line`, `Rectangle`, `Circle`, `FillPoly`, `Polylines`, `PutText`.

**Subpackages** (11 of them)

`quality` (PSNR/SSIM/GMSD/MSE/RMSE/FSIM/VIFP/SNR + no-reference Sharpness,
Tenengrad, Entropy, BRISQUE, and the stateful `QualityBase` objects) ·
`imghash` (8 hashers, `Similarity`, `IsDuplicate`, `HexEncode`) ·
`photo` (NL-means denoise, `Inpaint` with a before/after PSNR, edge-preserving
filter, detail enhance, stylisation, gamma, unsharp, white balance, `Decolor`) ·
`ximgproc` (guided/joint-bilateral/rolling-guidance/anisotropic/texture filters,
NiBlack, `Thinning`, SLIC + LSC superpixels, `FastLineDetector`,
`StructuredEdgeDetectionLite` + `EdgeBoxes`) ·
`segmentation` (k-means, Felzenszwalb graph, SLIC, mean-shift, `MultiOtsu`,
`FloodFill`, `GrabCut`, `BuildRAG`, `RegionGrowing`) ·
`features2d` (ORB detect + describe, `BFMatcher` Hamming match, `KnnMatch` +
`RatioTest`, with matches validated against a known +6 px shift — 55/56 survive) ·
`objdetect` (HOG descriptor + gradients + multi-scale detect, LBP, integral
image, `RectIoU`, `NMSBoxes`, `SoftNMSBoxes`, `GroupRectangles`) ·
`aruco` (marker generate/detect incl. rotation invariance, `DrawDetectedMarkers`,
grid board render/detect/`RefineDetectedMarkers`/`EstimatePoseBoard`, ChArUco
board + `InterpolateCornersCharuco`, ChArUco diamond, single-marker pose with and
without distortion, `CornerSubPix`, `UndistortImagePoints`,
`DetectMarkersWithParams`, 6x6 and custom dictionaries) ·
`barcode` (QR encode/decode basic + advanced, `FindFinderPatterns`, and
encode→decode round trips for EAN-13, EAN-8, Code128, Code39, Code93, Codabar,
ITF, MSI, Code11, plus check digits, UPC-E↔UPC-A↔EAN-13 conversion and
`DetectAndDecodeMulti` on a stacked canvas) ·
`plot` (line/scatter/bar/histogram charts, pie/box/heatmap constructors,
colormaps, `DrawLegend`) ·
`ml` (7 classifiers, `CrossValScore`, `KMeans`, confusion matrix / precision /
recall / F1 / ROC / AUC, regression metrics, gob persistence).

## Holes and rough edges found

Verified against **v0.8.0 in the module cache**, not the local working tree.

### Missing from the published module (present in the working tree)

1. **`aruco.ProjectPoints` does not exist in v0.8.0.** The whole
   `aruco/project.go` file (and `aruco/draw_more.go`, which holds `DrawAxis`,
   `DrawDetectedCornersCharuco`, `DrawDetectedDiamonds`) is unreleased. There is
   therefore **no way to forward-project object points** from outside the module,
   so a pose estimate cannot be reprojection-checked and detected ChArUco corners
   / diamonds cannot be visualised. Commented out in `main.go` with a
   `// HOLE:` note; substituted `EstimatePoseSingleMarkers` /
   `EstimatePoseBoard`.
2. **`aruco.GetBoardObjectAndImagePoints` does not exist in v0.8.0** — same
   cause. Board object↔image point pairing is unavailable to published-module
   users. Commented out.

### Runtime panic caused by an inconsistent API split

3. **`plot.ApplyColorMap` / `plot.ColormapTable` PANIC on 13 of the package's own
   21 colormap constants.** `ColormapAutumn`, `Winter`, `Summer`, `Spring`,
   `Ocean`, `Rainbow`, `Pink`, `Parula`, `Magma`, `Inferno`, `Cividis`,
   `Twilight` and `Turbo` are all exported `plot.Colormap` values, but
   `ColormapTable` `switch`es only over the original eight and `panic`s with
   `"plot: ColormapTable unknown colormap"` on the rest. The general versions are
   the differently-named `plot.Table` / `plot.Colorize`. Nothing in the type
   system or in `ApplyColorMap`'s signature warns you; the correct function to
   call depends on which constant you happened to pick. This crashed the example
   on `plot.ApplyColorMap(ramp, plot.ColormapTurbo)`; worked around by calling
   `Colorize` for the extended maps.

### Silent data loss with a nil error

4. **`ml.SaveFile` / `ml.LoadFile` silently produce an untrained model for 4 of
   the 9 classifiers.** Persistence is plain `encoding/gob`, and only `RTrees`,
   `Boost`, `ANNMLP`, `GaussianMixture` and `KernelSVM` implement
   `GobEncoder`/`GobDecoder`. Saving a `KNearest`, `SVM`, `NormalBayesClassifier`
   or `LogisticRegression` writes only its exported fields (`K`, `Weighted`, …),
   **`LoadFile` returns `nil`**, and the "restored" model then panics with
   `"ml: model has not been trained"` on the first `Predict`/`PredictBatch`.
   Reproduced exactly this way while writing the example. `Save`/`SaveFile` should
   reject types that cannot round-trip; the doc comment on `persistence.go`
   mentions which types are supported but the *public* `Save`/`SaveFile` godoc
   says nothing. Worked around by persisting an `RTrees` instead.

### Non-idiomatic / surprising API

5. **`segmentation.GrabCut(img, mask, rect, iters)` with a freshly allocated
   mask returns an empty segmentation.** A zeroed `*cv.Mat` is a valid
   `GC_INIT_WITH_MASK` labelling in which *every* pixel is `GcBgd` (a hard
   constraint), so `rect` is ignored and the result has 0 foreground pixels with
   no error or panic. You must pass a literal `nil` mask to get rect
   initialisation. The behaviour is documented but is a natural first-use trap —
   `nil` and "zero value" mean opposite things.
6. **`barcode` encode/decode payloads are asymmetric.** `EncodeEAN13` takes 12
   digits and `DecodeEAN13` returns 13 (check digit appended); likewise EAN-8
   (7 → 8) and MSI (`"12345"` → `"123455"`). A naive `decode(encode(x)) == x`
   assertion fails for half the symbologies and succeeds for the other half
   (Code128/39/93, Codabar, ITF, Code11).
7. **`MinAreaRect` and `BoundingRect` disagree by one pixel on the same
   contour.** The 71×71 drawn square yields `BoundingRect` `71×71` (documented as
   inclusive) but `MinAreaRect` `70×70`. Consistent with the docs, but mixing the
   two in one pipeline needs care.
8. **`quality.Sharpness` and `quality.LaplacianVariance` return byte-identical
   values** (1401.44 for the test scene) — two exported names for one metric.
9. **`SobelFloat` returns `[][]float64` indexed by channel, not by row**, which
   reads as a row-major image at the call site. Easy to misuse.
10. **`Threshold` returns `(*Mat, float64)`** where the second value is only
    meaningful for `ThreshOtsu`; and the Otsu flag is OR-ed into the same
    `ThresholdType` enum (`cv.ThreshBinary|cv.ThreshOtsu`, where `ThreshOtsu = 8`
    sits in the same type as the iota-based modes). Faithful to OpenCV, but not
    Go-idiomatic and not type-safe.
11. **Approximation quality is parameter-sensitive with no guardrails.**
    `HoughCircles` on a textured scene returns 19 circles at `param2=26` and 6 at
    `param2=45`; `objdetect.HOGDescriptor.DetectMultiScale` with
    `DefaultPeopleDetector()` reports 342 hits on a scene with no people. Both
    are documented as weight-free approximations, but there is no confidence
    output to filter on.

### Not holes (verified correct)

`ApproxPolyDP` on a clean closed square returns exactly 4 vertices;
`HoughCircles` on a clean synthetic circle returns exactly `{100 100 40}`;
`MatchTemplate` recovers the exact ground-truth offset in all four modes; the
`FromImage`/`ToImage` and PNG round trips are byte-exact; `Filter2DSep` matches
`BoxFilter` bit-for-bit; `ConnectedComponentsWithStats` centroids are exact.
Reading library README claims against the code found no mismatches — the
documented surface (Mat/CV_8U only, RGB not BGR, weight-free approximations,
CPU-backed `cuda*` shims) is what is actually shipped, and no cgo or third-party
dependency is pulled in (`go.sum` has exactly one module).
