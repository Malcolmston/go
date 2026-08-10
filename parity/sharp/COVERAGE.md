# sharp — cross-language parity coverage

| | |
| --- | --- |
| upstream oracle | `sharp@0.33.5` (npm, prebuilt libvips 8.15.3 — no system libvips needed) |
| Go port | `github.com/malcolmston/sharp v0.0.0-20260810111610-e874a7b0b3f8` |
| harness | `GOWORK=off go test ./parity/sharp/` |
| cases | 327 in 14 groups |
| case parity | **213 / 327 = 65.14 %** (0 deviations — no difference here is sanctioned) |
| symbol parity | **13 / 43 compared = 30.23 %** (13 / 62 of the whole upstream prototype = 20.97 %) |

## What is compared — and what deliberately is not

Two independent resamplers will never produce byte-identical pixels, and two
independent encoders will never produce byte-identical files. **Encoded image
bytes and exact pixel values are therefore not compared at all.** Comparing them
would score noise, and the noise would drown out the real divergences. Four
things are compared instead:

1. **Geometry — exact.** Output `width`, `height`, `channels` and `hasAlpha`
   after every resize / fit / crop / extend / rotate / extract combination.
   This is deterministic, has a single right answer, and is where the port
   genuinely diverges: aspect-ratio rounding, `withoutEnlargement`, and
   `fit: inside | outside | cover | contain | fill`. 212 of the 327 cases ask a
   geometry question.
2. **Metadata — exact.** `format`, `width`, `height`, `channels`, `hasAlpha`,
   `space` as each library reports them for the same file on disk.
3. **Coarse pixel statistics — with a tolerance.** Per-channel mean, min and max
   over every pixel, plus the mean Rec. 601 luma of each cell of a 4×4
   downsample grid. This catches gross errors — channel swaps, inverted or
   premultiplied alpha, wrong gamma, all-black output, aliasing collapse —
   without demanding resampler equality.
4. **Format acceptance — ok/error.** Which inputs each side decodes and which
   outputs each side encodes, probed by actually decoding and actually encoding
   each candidate rather than by introspection.

### The tolerance, and why those numbers

Pixel cases carry `"tol": 2.0` with `"tolByKey": {"min": 8, "max": 8, "width":
0, "height": 0}` — an absolute tolerance in 8-bit levels, i.e. 0.78 % of full
scale for means and grid luma, 3.1 % for the extremes, and none at all for
dimensions.

The numbers are measured, not guessed. Across operations where the two
implementations genuinely agree, the observed maximum deltas were:

| case | max Δgrid | max Δmean | max Δmin/max |
| --- | --- | --- | --- |
| `px-median` | 0.000 | 0.000 | 0 |
| `px-convolve-sharpen` | 0.000 | 0.000 | 0 |
| `px-modulate-hue` | 0.054 | 0.016 | 0 |
| `px-resize-cover` (lanczos3) | 0.142 | 0.653 | 1 |
| `px-blur` | 0.265 | 0.050 | 5 |
| `px-sharpen` | 0.302 | 0.086 | 0 |
| `px-resize-lanczos3-grad` | 0.394 | 0.896 | 3 |
| `px-linear` | 0.407 | 0.333 | 0 |
| `px-recomb` | 0.550 | 0.500 | 1 |

and across the operations that actually diverge the deltas start at 1.2 and run
to 255. The tolerance sits in that gap: **agreement never needs more than 0.9
mean / 0.6 grid / 5 extreme, and no divergence found here is smaller than 1.2.**
`min`/`max` get the looser 8 because they are single-pixel values and a windowed
kernel with negative lobes (lanczos, unsharp) legitimately overshoots by a few
levels at a hard edge.

Two consequences worth stating plainly:

- A coarse summary can let an algorithmic difference through. `px-sharpen`
  passes even though the two `sharpen` implementations are not the same
  algorithm and do not take the same parameters. That symbol is scored
  `differs` on its signature, not `match`, precisely because the pixel check
  cannot see the difference.
- The grid is what catches aliasing. `px-resize-linear-check` has *identical*
  mean/min/max on both sides and a grid delta of 33.3: libvips shrinks a
  checkerboard by box-averaging while the port point-samples bilinearly, so the
  global statistics agree perfectly and the local structure does not.

### Fixtures

All inputs are generated once by `node/gen-fixtures.mjs` into `fixtures/`, and
**both runners read the same bytes off the same files** — nothing is synthesised
twice. The PNG fixtures (gradient, 8-px checkerboard, semi-transparent disc,
fully-opaque RGBA, three awkward aspect ratios, a mask operand) come from a
dependency-free PNG writer in that script driven by closed-form pixel formulas,
so no image library has any say in what the canonical inputs look like; the BMP
and the three broken files are likewise written by hand. Only the JPEG/GIF/TIFF/
WebP/AVIF probe inputs are transcoded by upstream sharp, because it is the only
side that can encode all of them — and the question those cases ask is "can the
port read what sharp writes", which is the right question for a port.

Both runners are deterministic: libvips concurrency is pinned to 1 and its
operation cache disabled, no `Math.random`/`rand` appears anywhere, map keys are
emitted sorted, and neither runner exits on a failing case (a throw, a rejected
promise, a Go error or a Go panic all become `{"ok": false, "error": …}`).

### One harness-level accommodation, declared

The `resize` op maps `fit` and `kernel` names onto the port's nearest
equivalents, and when a case names *neither*, the Go runner passes
`Fit: FitCover, Interpolation: Lanczos3` explicitly so that an explicitly-pinned
case is not silently also testing the defaults. The defaults are compared on
their own by the `resizeDefault` op, which passes a zero-value
`sharp.ResizeOptions{}` against a bare `.resize(w, h)` — see `px-resize-default`.
Nothing else in either runner compensates for either library.

## How the upstream inventory was derived

Mechanically, from the installed package — not from the README and not from
memory.

The prototype method list (62 symbols) came from:

```sh
cd parity/sharp/node
node -e "const s=require('sharp');
  console.log(Object.getOwnPropertyNames(s.prototype)
    .filter(n => typeof s.prototype[n] === 'function'
                 && n !== 'constructor' && !n.startsWith('_')).sort().join('\n'))"
```

The option keys of each method came from the JSDoc of the real installed source:

```sh
cd parity/sharp/node/node_modules/sharp/lib
node -e "const fs=require('fs');
  for (const f of ['resize.js','operation.js','colour.js','channel.js','composite.js','output.js']) {
    const src = fs.readFileSync(f,'utf8');
    const re = /\/\*\*([\s\S]*?)\*\/\s*function\s+([A-Za-z0-9_]+)\s*\(/g;
    for (let m; (m = re.exec(src));) {
      const keys = [...new Set([...m[1].matchAll(/@param\s+\{[^}]*\}\s+\[?options\.([A-Za-z0-9]+)/g)].map(x => x[1]))];
      if (keys.length) console.log(m[2] + ': ' + keys.join(' '));
    }
  }"
```

The Go side came from `GOWORK=off go doc -all github.com/malcolmston/sharp`
against the resolved module version, so the "extra" rows are the port's real
exported surface and not a guess.

## Upstream inventory — all 62 public `Sharp.prototype` methods

`match` = compared and agrees everywhere it is exercised. `differs` = compared
and disagrees (or the signature cannot express what upstream does).
`missing` = not ported. `untested` = the port has something for it but no case
exercises it, so it is never counted as a match.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Sharp#affine` | `Pipeline.Affine` | differs | `affine-shear`, `affine-scale`, `affine-singular` | output bounding box 1 px wider (79 vs 78); sharp takes a 2×2 matrix + `interpolator`, the port a 2×3 matrix and no interpolator choice |
| `Sharp#avif` | — | **missing** | `enc-avif`, `decode-avif`, `meta-avif` | no AVIF at all: cannot encode, cannot decode |
| `Sharp#bandbool` | `Pipeline.Bandbool` | match | `ch-bandbool-and`, `px-bandbool-and`, `px-bandbool-or` | |
| `Sharp#blur` | `Pipeline.Blur` | differs | `px-blur`, `px-blur-checker` | agrees on a gradient (Δmean 0.05) but Δgrid 9.75 on a checkerboard: different Gaussian support/truncation |
| `Sharp#boolean` | `Pipeline.Boolean` | differs | `ch-boolean-eor`, `ch-boolean-and`, `ch-boolean-size-mismatch`, `ch-boolean-bogus-op`, `px-boolean-eor` | port XORs the alpha band (255^255 = 0) so `eor` wipes alpha; port rejects a differently-sized operand that sharp accepts |
| `Sharp#clahe` | `Pipeline.CLAHE` | differs | `px-clahe` | Δmean 64: different tile histogram/clip formulation |
| `Sharp#clone` | `Pipeline.Clone` | untested | — | present in both; no case |
| `Sharp#composite` | `Pipeline.Composite` | differs | `px-composite-over`, `px-composite-offset`, `px-composite-multiply`, `px-composite-gravity` | agrees for opaque overlays; Δmean 13 for a translucent overlay (the port's decode premultiplies — see divergence 7) |
| `Sharp#convolve` | `Pipeline.Convolve`, `sharp.NewKernel` | match | `px-convolve-sharpen`, `px-convolve-box` | exact to 0.000 |
| `Sharp#ensureAlpha` | `Pipeline.EnsureAlpha` | differs | `ch-ensure-alpha`, `ch-ensure-alpha-half` | with alpha 1 the port still reports 3 channels / `hasAlpha:false` (see divergence 6) |
| `Sharp#extend` | `Pipeline.Extend`, `Pipeline.ExtendWith` | differs | `extend-*` (13) | 12/13 agree including all four `extendWith` modes; a negative side throws upstream and is silently clamped to 0 by the port |
| `Sharp#extract` | `Pipeline.Extract`, `Pipeline.Crop` | match | `extract-*` (9) | including all four out-of-bounds/zero-size error cases |
| `Sharp#extractChannel` | `Pipeline.ExtractChannel` | differs | `ch-extract-r/g/b`, `ch-extract-a-present`, `ch-extract-a-missing` | upstream yields a 1-band image, the port a 3-band grey; extracting alpha from an image that has none throws upstream and succeeds in the port |
| `Sharp#flatten` | `Pipeline.Flatten` | differs | `ch-flatten`, `px-flatten` | geometry agrees; Δgrid 3.6 from the premultiplied decode |
| `Sharp#flip` | `Pipeline.FlipVertical`, `Pipeline.Flip` | match | `flip-grad`, `flip-flop-grad`, `px-flip` | |
| `Sharp#flop` | `Pipeline.FlipHorizontal`, `Pipeline.Flop` | match | `flop-grad`, `px-flop` | |
| `Sharp#gamma` | `Pipeline.Gamma` | differs | `px-gamma`, `px-gamma-low` | Δmean 49. sharp's `gamma(g)` decodes by 1/g pre-resize and re-encodes by g after, so with no resize it is near-identity; the port applies `out = 255·(in/255)^(1/g)` once and brightens |
| `Sharp#gif` | `Pipeline.ToGIF` | differs | `enc-gif`, `enc-gif-alpha` | opaque GIF agrees; the port's GIF drops alpha (3 bands) where sharp keeps it (4) |
| `Sharp#grayscale` | `Pipeline.Grayscale` | differs | `ch-grayscale`, `px-grayscale` | Δmean 12.2: sharp converts through linear-light B-W, the port applies Rec. 601 weights to sRGB values; also 1 band vs 3 |
| `Sharp#greyscale` | `Pipeline.Grayscale` | differs | `ch-grayscale` | same as above; the port has no British-spelled alias |
| `Sharp#heif` | — | **missing** | `enc-heif` | both sides error, but only because this prebuilt binary wants an explicit `compression` — not evidence of parity |
| `Sharp#joinChannel` | `sharp.JoinChannels` | untested | — | different shape (package function over `image.Image`, not a pipeline method) |
| `Sharp#jp2` | — | **missing** | `enc-jp2` | both error; upstream's is "requires libvips with OpenJPEG", so this pass is incidental |
| `Sharp#jpeg` | `Pipeline.ToJPEG`, `Pipeline.ToJPEGWithOptions` | match | `enc-jpeg`, `enc-jpeg-q40`, `enc-jpeg-q100`, `enc-jpeg-alpha`, `tofile-jpeg` | only `quality` of sharp's 13 JPEG keys exists in the port |
| `Sharp#jxl` | — | **missing** | `enc-jxl` | both error; upstream's is a missing libvips operation, so this pass is incidental |
| `Sharp#keepExif` | — | **missing** | — | no EXIF handling in the port at all |
| `Sharp#keepIccProfile` | — | **missing** | — | no ICC handling in the port at all |
| `Sharp#keepMetadata` | — | **missing** | — | |
| `Sharp#linear` | `Pipeline.Linear` | match | `px-linear`, `px-linear-per-channel` | Δmean 0.33 |
| `Sharp#median` | `Pipeline.Median` | differs | `px-median`, `px-median-5`, `px-median-even` | 3×3 exact; 5×5 Δmean 3.86; the port takes a radius so it cannot express an even window, which sharp accepts |
| `Sharp#metadata` | `Pipeline.Metadata` | differs | `meta-*` (14) | `channels`/`hasAlpha` are inferred from the pixels, not read from the file (see divergence 5); no WebP/AVIF/HEIF input, but BMP input that sharp lacks |
| `Sharp#modulate` | `Pipeline.Modulate` | match | `px-modulate-brightness`, `px-modulate-saturation`, `px-modulate-hue` | Δmean ≤ 0.02 — the port really does go through D65 CIELAB |
| `Sharp#negate` | `Pipeline.Negate`, `Pipeline.Invert` | differs | `px-negate`, `px-negate-alpha` | RGB agrees exactly; sharp's default `alpha: true` also negates alpha, the port always preserves it |
| `Sharp#normalise` | `Pipeline.Normalise` | differs | `px-normalise`, `px-normalise-full` | Δmean 9.6 at 1–99 %: different percentile/stretch basis |
| `Sharp#normalize` | `Pipeline.Normalize` | differs | `px-normalise` | alias of the above |
| `Sharp#pipelineColorspace` | — | **missing** | — | no control of the working colourspace |
| `Sharp#pipelineColourspace` | — | **missing** | — | |
| `Sharp#png` | `Pipeline.ToPNG`, `Pipeline.ToPNGWithOptions` | match | `enc-png`, `enc-png-alpha`, `tofile-png` | only `compressionLevel` of sharp's 11 PNG keys |
| `Sharp#raw` | `Pipeline.ToRaw`, `sharp.FromRaw` | differs | `surface-raw-roundtrip-grad/circle/orgba`, `tofile-raw` | round-trips cleanly, but the port always emits 4 bytes/px (12288 vs upstream's 9216 for a 3-band 64×48 image), and its doc comment says "non-premultiplied" while the buffer is premultiplied |
| `Sharp#recomb` | `Pipeline.Recomb` | match | `px-recomb`, `px-recomb-identity` | Δmean 0.5 |
| `Sharp#removeAlpha` | `Pipeline.RemoveAlpha` | match | `ch-remove-alpha`, `ch-remove-alpha-opaque` | |
| `Sharp#resize` | `Pipeline.Resize`, `ResizeTo`, `ResizeWidth`, `ResizeHeight` | differs | `fit-*` (100), `dim-*` (24), `opt-*` (14), `px-resize-*` (13) | 40 of 100 fit cases and 5 of 24 single-dimension cases disagree — see divergences 1–4 |
| `Sharp#rotate` | `Pipeline.Rotate`, `Rotate90/180/270` | differs | `rotate-*` (16), `px-rotate90/180/270` | exact multiples of 90° agree exactly; every other angle gets a bounding box 1 px too large (see divergence 2) |
| `Sharp#sharpen` | `Pipeline.Sharpen`, `Pipeline.Unsharp` | differs | `px-sharpen` | passes the coarse check (Δmean 0.09) but the signature cannot express sharp's `{sigma, m1, m2, x1, y2, y3}` — the port takes one unitless amount over a fixed 3×3 kernel |
| `Sharp#stats` | `Pipeline.Stats`, `Pipeline.ChannelStats` | untested | — | present but shaped differently (no `entropy`/`dominant`/`isOpaque` in one call); the coarse pixel summary is computed by the harness, not by either `stats()` |
| `Sharp#threshold` | `Pipeline.Threshold` | differs | `px-threshold` | Δmean 18.8, inherited from the greyscale difference the threshold is taken over |
| `Sharp#tiff` | `Pipeline.ToTIFF` | differs | `enc-tiff`, `enc-tiff-alpha`, `tofile-tiff` | opaque agrees; the port's TIFF keeps alpha (4 bands) where sharp drops it (3). Port writes baseline uncompressed only — none of sharp's 13 TIFF keys |
| `Sharp#tile` | — | **missing** | — | no tiled/deep-zoom output |
| `Sharp#timeout` | — | **missing** | — | |
| `Sharp#tint` | `Pipeline.Tint` | differs | `px-tint` | Δmean 80. sharp tints in LAB preserving lightness; the port multiplies each channel by the tint, which darkens |
| `Sharp#toBuffer` | `Pipeline.ToFormat` and friends | match | the whole `encode` group | `resolveWithObject` has no equivalent |
| `Sharp#toColorspace` | `Pipeline.ToColorspace` | differs | `ch-tocolourspace-bw`, `-srgb`, `-bogus` | alias of the below |
| `Sharp#toColourspace` | `Pipeline.ToColourspace` | differs | `ch-tocolourspace-bw`, `-srgb`, `-bogus` | `b-w` gives 1 band upstream, 3 in the port; an unknown space is ignored upstream and is a hard error in the port; only b-w/sRGB are recognised at all |
| `Sharp#toFile` | `Pipeline.ToFile` | differs | `tofile-*` (7) | **`ToFile(path, FormatRaw, …)` writes a PNG while `ToFormat(FormatRaw)` writes real raw bytes** (see divergence 8); also accepts BMP (extra) and rejects WebP (missing) |
| `Sharp#toFormat` | `Pipeline.ToFormat` | differs | `enc-*` (18), `surface-output-formats` | no WebP/AVIF/HEIF/JXL/JP2; BMP is Go-only |
| `Sharp#trim` | `Pipeline.Trim` | match | `trim-grad`, `trim-circle`, `px-trim` | `background` and `lineArt` keys have no equivalent |
| `Sharp#unflatten` | `Pipeline.Unflatten` | differs | `ch-unflatten`, `px-unflatten` | upstream produces a 4-band result, the port still reports 3; Δgrid 13.9 from the premultiplied decode |
| `Sharp#webp` | — | **missing** | `enc-webp`, `decode-webp`, `meta-webp`, `tofile-webp`, `surface-output-formats`, `surface-input-formats` | **no WebP at all: cannot encode, cannot decode** |
| `Sharp#withExif` | — | **missing** | — | |
| `Sharp#withExifMerge` | — | **missing** | — | |
| `Sharp#withIccProfile` | — | **missing** | — | |
| `Sharp#withMetadata` | `Pipeline.WithDensity` | untested | — | only the `density` half is ported; no case |

**Counts:** match 13 · differs 30 · missing 15 · untested 4 · total 62.
Parity over the symbols actually compared: **13 / (13 + 30) = 30.23 %**.
Over the whole prototype: 13 / 62 = 20.97 %.

## Option keys targeted

The keys below are the ones the case files actually drive. Every other key of
these methods is untested — and, where noted, has no Go counterpart to test.

| upstream method | keys upstream declares (from its JSDoc) | keys exercised here | Go counterpart |
| --- | --- | --- | --- |
| `resize` | `width height fit position background kernel withoutEnlargement withoutReduction fastShrinkOnLoad` | `width height fit position kernel withoutEnlargement withoutReduction` | `ResizeOptions{Width Height Fit Interpolation}` — no `position`, `background`, `withoutEnlargement`, `withoutReduction`, `fastShrinkOnLoad` |
| `resize.fit` | `cover contain fill inside outside` | all five | `FitCover FitContain FitExact` — `contain` (padding) and `outside` have no equivalent |
| `resize.kernel` | `nearest linear cubic mitchell lanczos2 lanczos3` | all six | `Nearest Bilinear Cubic Mitchell Lanczos3` — `lanczos2` missing |
| `extract` | `left top width height` | all four | `Rectangle{X Y Width Height}` |
| `extend` | `top right bottom left extendWith background` | all six | `ExtendOptions{Top Right Bottom Left Background Mode}`; all four `extendWith` values map |
| `rotate` | `background` (+ the positional angle) | both | `Rotate(degrees, fill)` |
| `flatten` | `background` | yes | `Flatten(bg)` |
| `negate` | `alpha` | default only | none — the port never negates alpha |
| `modulate` | `brightness saturation hue lightness` | all four | `ModulateOptions` — all four |
| `normalise` | `lower upper` | both | `NormaliseOptions{Lower Upper}` |
| `sharpen` | `sigma m1 m2 x1 y2 y3` | `sigma` | `Sharpen(amount)` / `UnsharpOptions{Sigma Amount Threshold}` — no `m1 m2 x1 y2 y3` |
| `blur` | `sigma precision minAmplitude` | `sigma` | `Blur(sigma)` only |
| `clahe` | `width height maxSlope` | all three | `CLAHEOptions` — all three |
| `affine` | `background idx idy odx ody interpolator` | `background` + matrix | `AffineOptions{Background}` only |
| `trim` | `background threshold lineArt` | `threshold` | `Trim(threshold)` only |
| `threshold` | `greyscale grayscale` | default only | `Threshold(level)` — cannot opt out of greyscaling |
| `jpeg` | 13 keys | `quality` | `JPEGOptions{Quality}` |
| `png` | 11 keys | defaults | `PNGOptions{Compression}` |
| `tiff` | 13 keys | defaults | none (baseline uncompressed only) |
| `gif` | 11 keys | defaults | none (256-colour default only) |
| `webp` / `avif` / `heif` / `jxl` / `jp2` | 6–12 keys each | format acceptance only | **none** |
| `composite` (layer) | `input blend gravity left top` (+ 8 more) | `input blend gravity left top` | `CompositeOptions{Left Top UseGravity Gravity Opacity Blend}` |

## Go-only surface (`extra`)

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `FormatBMP`, `Pipeline.ToBMP` | extra | `enc-bmp`, `decode-bmp`, `meta-bmp`, `tofile-bmp`, `surface-output-formats`, `surface-input-formats` | the port both reads and writes BMP; sharp does neither |
| `Pipeline.Rotate90/180/270` | extra | `rotate-grad-90/180/270`, `px-rotate90/180/270` | exact-multiple shortcuts; agree with `rotate(90|180|270)` |
| `Pipeline.Crop` | extra | `extract-crop-alias` | alias of `Extract` |
| `sharp.FromRaw` | extra | `surface-raw-roundtrip-*` | upstream expresses this as an input option, not a constructor |
| `Brightness`, `Contrast`, `Saturation`, `Sepia`, `Erode`, `Dilate`, `Morphology`, `Unsharp`, `Histogram`, `Entropy`, `Sharpness`, `DominantColor`, `MeanColor`, `IsOpaque`, `ChannelStats`, `RGBToLab`/`LabToRGB`/`RGBToXYZ`/`XYZToRGB`/`RGBToHSV`/`HSVToRGB`/`DeltaE76`/`Luma`, `WithDensity`, `JoinChannels`, `Invert`, `Flip`/`Flop` aliases | extra, untested | — | Go-only helpers with no upstream `Sharp.prototype` counterpart, so there is nothing to compare them against |

## Every real divergence found

Ordered by how much it would hurt someone porting code, geometry first.

1. **`fit: 'contain'` does not pad — 20 failing cases.** sharp's `contain`
   scales to fit *and* pads to exactly `width × height`. The port has no padding
   fit at all, so mapping it to the nearest available `FitContain` yields the
   unpadded size: `fit-grad-32x32-contain` gives 32×32 upstream and 32×24 in the
   port; `fit-wide-7x7-contain` gives 7×7 and 7×1.
2. **Non-right-angle rotation makes a canvas 1 px too large — 7 failing cases,
   plus `affine`.** The port computes the bounding box with
   `ceil(|w·cos| + |h·sin|)` where libvips rounds: 30° on 64×48 gives 79×74
   upstream and 80×74; 45° gives 79×79 and 80×80; 7° gives 69×55 and 70×56;
   270.5° gives 49×64 and 49×65. `affine-shear` is the same fault: 78 vs 79.
   Exact multiples of 90° (and 0/360) agree exactly.
3. **A single-dimension resize under a fit loses a pixel — 5 failing cases.**
   The port first derives the missing dimension from the aspect ratio and *then*
   applies the fit scale to the derived pair, so the fit shrinks the dimension
   that was explicitly asked for. `resize({width: 10, fit: 'inside'})` on 33×21
   gives 10×6 upstream and **9×6**; `width: 100` on 90×10 gives 100×11 and
   **99×11**; `height: 100` on 10×90 gives 11×100 and **11×99**.
4. **`fit: 'outside'` is not implemented — 20 failing cases.** The port has no
   "cover the box while preserving aspect" fit, so every `outside` case is an
   error against an upstream answer (`fit-grad-32x32-outside`: 43×32).
5. **`Metadata().Channels` is inferred from the pixels, not read from the file.**
   `meta-orgba` — a genuinely 4-channel RGBA PNG in which every pixel happens to
   be opaque — reports `channels: 4, hasAlpha: true` upstream and
   `channels: 3, hasAlpha: false` in the port. The same inference makes
   `ch-ensure-alpha` (`ensureAlpha(1)`) and `ch-unflatten` report 3 channels
   where upstream reports 4, and inverts on `ch-boolean-eor`, where the port
   reports 4 channels *because* it has just corrupted alpha to zero.
6. **`Boolean` with `BoolXor` wipes alpha.** The port applies the bitwise
   operator to all four bands, so `255 ^ 255 = 0`: `px-boolean-eor` has mean
   alpha 255 upstream and **0** in the port; min and max alpha are both 0. Every
   pixel of the result is fully transparent.
7. **The PNG decode path premultiplies, contradicting `ToRaw`'s own doc.**
   `toRGBA` reads pixels through `color.Color.RGBA()`, which premultiplies,
   into an `image.RGBA` — so `ToRaw`, whose comment promises "non-premultiplied",
   returns premultiplied bytes. On the semi-transparent disc (`px-identity-circle`,
   an untouched decode) the channel means are `[113.7, 53.5, 55.9]` upstream and
   `[65.6, 23.2, 20.7]` in the port — exactly the `α/255` factor. It propagates
   into `px-unflatten`, `px-flatten`, `px-composite-offset`,
   `px-composite-gravity` and `px-negate-alpha`.
8. **`ToFile(path, FormatRaw, …)` silently writes a PNG while
   `ToFormat(FormatRaw)` writes real raw bytes — two entry points disagreeing,
   with no error from either.** `tofile-raw` sniffs the two outputs by magic
   number: the port reports `{"buffer": "unknown", "file": "png"}`, i.e.
   `agree: false`. Upstream refuses the operation outright
   (*"Unsupported output format …/out.raw"*). Nothing in the port tells the
   caller that the file it just wrote is not the format it asked for.
   `tofile-png/jpeg/gif/tiff` all agree, so this is specific to `FormatRaw`.
9. **No WebP and no AVIF, in either direction — the most consequential gap.**
   `enc-webp`, `enc-avif`, `decode-webp`, `decode-avif`, `meta-webp`,
   `meta-avif`, `tofile-webp` and both `surface-*-formats` probes all fail.
   WebP is sharp's most-used output format. The port documents this as
   intentional (a codec would need cgo, and the port is standard-library-only),
   but it is still scored `missing`, not a sanctioned deviation, because a caller
   porting real sharp code cannot work around it.
10. **`raw()` always emits 4 bytes per pixel.** `surface-raw-roundtrip-grad`:
    9216 bytes / 3 channels upstream, 12288 / 4 in the port. Round-tripping
    through `FromRaw` is lossless on both sides, but the buffer layout differs
    for any image without alpha.
11. **Greyscale uses a different luma basis.** `px-grayscale` mean 139.9 vs
    127.7 (Δ12.2): sharp converts through linear-light B-W, the port applies
    Rec. 601 weights directly to sRGB values. `px-threshold` inherits it
    (157.9 vs 139.1), and `ch-grayscale`/`ch-tocolourspace-bw` additionally
    differ on band count (1 vs 3).
12. **`gamma` means something different.** sharp's `gamma(g)` decodes by `1/g`
    before a resize and re-encodes by `g` after, so with no resize it is close to
    a no-op (`px-gamma`: 125.2 vs the source's 127.5). The port applies
    `out = 255·(in/255)^(1/g)` once and brightens to 174.3 — Δ49.
13. **`tint` darkens instead of tinting.** `px-tint` with `#ff8000`:
    `[207.1, 112.4, 40.3]` upstream, `[127.5, 64.0, 0.0]` in the port. sharp
    tints in LAB preserving lightness; the port multiplies each channel by the
    tint colour, which can only ever reduce it.
14. **`normalise` and `clahe` use different formulations.** `px-normalise`
    Δmean 9.6; `px-clahe` Δmean 63.7 with the port's max at 220 where upstream
    reaches 255.
15. **Zero-value `ResizeOptions` defaults to Nearest + fit-exact where sharp
    defaults to Lanczos3 + `cover`.** Geometry happens to coincide when both
    dimensions are given (`resizeDefault` 16×32 is 16×32 either way), so this is
    only visible in the pixels: `px-resize-default` has upstream min
    `[85, 1, 0]` against the port's `[8, 0, 0]` — upstream cropped and
    Lanczos-filtered, the port stretched and point-sampled.
16. **Smaller semantic divergences, all with cases.**
    - `blur` agrees on smooth input and diverges on a checkerboard (Δgrid 9.75):
      different Gaussian support.
    - `linear` kernel downscaling: `px-resize-linear-check` has identical global
      statistics and Δgrid 33.3 — libvips box-averages on shrink, the port
      point-samples and aliases.
    - `median` is exact at 3×3, Δmean 3.86 at 5×5, and cannot express the even
      window sharp accepts (`px-median-even`).
    - `negate` never touches alpha; sharp's default does (`px-negate-alpha`).
    - GIF output drops alpha where sharp keeps it (`enc-gif-alpha`); TIFF output
      keeps alpha where sharp drops it (`enc-tiff-alpha`).
    - `extractChannel(3)` on an image with no alpha throws upstream and succeeds
      in the port (`ch-extract-a-missing`).
    - `resize()` with no dimensions is a no-op upstream and an error in the port
      (`opt-no-dimensions`); `width: 0` throws upstream and is treated as
      "derive from the aspect ratio" by the port (`opt-zero-width`).
    - `extend` with a negative side throws upstream, is clamped to 0 by the port
      (`extend-negative`).
    - `toColourspace` with an unrecognised name is ignored upstream, a hard error
      in the port (`ch-tocolourspace-bogus`).
    - `boolean` against a differently-sized operand works upstream, errors in the
      port (`ch-boolean-size-mismatch`).

### Cases that pass for a shallow reason

`enc-heif`, `enc-jp2`, `enc-jxl` and `enc-raw` score as matches because *both*
sides error — but upstream's errors are "this prebuilt libvips has no OpenJPEG /
no jxlsave / needs an explicit `compression`", not "this format does not exist".
They are counted honestly as both-failed parity, and the corresponding symbols
are still listed `missing` above.
