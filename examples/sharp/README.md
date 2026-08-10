# sharp example

A runnable example program that exercises
[`github.com/malcolmston/sharp`](https://github.com/malcolmston/sharp) — a pure-Go
port of the Node.js [sharp](https://sharp.pixelplumbing.com/) image processing
library — as an ordinary outside consumer of the published module.

The module is consumed straight from the proxy: there is **no `replace`
directive** and no reference to the sibling `../../sharp` working tree.

## Resolved module version

```
github.com/malcolmston/sharp v0.0.0-20260719012951-f1ec4ac1cf25
```

The repository publishes no semver tags, so `go get ...@latest` resolves to a
pseudo-version. The library has **zero third-party dependencies** and is pure Go
(no cgo, no libvips) — `go build` works out of the box on any Go toolchain.

## How to run

```sh
cd examples/sharp
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

`GOWORK=off` is required because the repository root contains a `go.work` that
would otherwise pull in the local library copy.

Every input image is generated programmatically in memory (gradients, a
checkerboard, a semi-transparent circular badge, a bordered rectangle, a noisy
plate). Nothing is read from disk and nothing is fetched from the network beyond
the module download itself. Outputs are written to a fresh
`os.MkdirTemp` directory whose path is printed at startup, then each output is
decoded again and its dimensions/format verified, with a `PASS`/`FAIL` line per
check. The program always terminates.

## What it demonstrates

| Section | Coverage |
| --- | --- |
| 1. Loading & metadata | `New`, `FromBytes`, `FromReader`, `FromFile`, `Metadata`, `WithDensity` |
| 2. Resize | `Resize` with `FitExact`/`FitContain`/`FitCover`, aspect-ratio derivation, all five kernels (`Nearest`, `Bilinear`, `Cubic`, `Mitchell`, `Lanczos3`), `ResizeTo`/`ResizeWidth`/`ResizeHeight` |
| 3. Extract / extend | `Extract`, `Crop`, `Extend`, `ExtendWith` in all four `ExtendMode`s |
| 4. Rotate & flip | `Rotate90/180/270`, arbitrary-angle `Rotate` (bounding-box growth verified), `FlipVertical`/`FlipHorizontal` and the `Flip`/`Flop` aliases |
| 5. Formats | `ToPNG`, `ToJPEG`, `ToGIF`, `ToBMP`, `ToTIFF`, generic `ToFormat`, `ToFile` |
| 6. Quality | JPEG quality sweep (size monotonicity asserted), `ToJPEGWithOptions`, `ToPNGWithOptions` across all `png.CompressionLevel`s, quality clamping |
| 7. Compositing | `Composite` by offset and by all `Gravity` anchors, `Opacity`, all 13 `BlendMode`s, `Flatten`, `IsOpaque` |
| 8. Filters | `Blur`, `Sharpen`, `Unsharp`, `Convolve` with a custom `NewKernel` (edge detect and box blur), `Median`, `Morphology`/`Erode`/`Dilate`. Blur/sharpen are validated numerically against the library's own `Sharpness()` metric |
| 9. Colour & tone | `Grayscale`, `Negate`/`Invert`, `Sepia`, `Tint`, `Brightness`, `Contrast`, `Gamma`, `Saturation`, `Threshold`, `Modulate`, `Linear`, `Normalise`/`Normalize`, `CLAHE`, `ToColourspace`, `Unflatten`, `Luma`, and the standalone `RGBToHSV`/`HSVToRGB`/`RGBToXYZ`/`XYZToRGB`/`RGBToLab`/`LabToRGB`/`DeltaE76` helpers |
| 10. Channels | `ExtractChannel`, `RemoveAlpha`, `EnsureAlpha`, `JoinChannels`, `Recomb`, `Boolean`, `Bandbool` |
| 11. Statistics | `Stats`, `ChannelStats`, `Histogram` (pixel count asserted), `Entropy` (gradient vs flat asserted), `Sharpness`, `IsOpaque`, `DominantColor`/`DominantColour`, `MeanColor` |
| 12. Raw buffers | `ToRaw`, `FromRaw` for 1/3/4 channels, byte-exact round trip, length validation |
| 13. Affine & trim | `Affine` (shear, singular-matrix rejection), `Trim` |
| 14. Errors | Deferred-error propagation through a chain, `Err()`, `Clone` independence |

## Holes found

Everything the example exercises produced correct dimensions and plausible
pixels; the run is entirely `PASS`. The gaps below are API-shape and
behaviour issues rather than crashes.

### 1. No WebP or AVIF encoder/decoder

The brief asked for WebP output. There is no `ToWebP`, no `FormatWebP`, and
`FromBytes` cannot decode WebP. `ToFormat(sharp.Format("webp"), 80)` compiles —
`Format` is a bare `string` type — but returns
`sharp: unsupported format "webp"`. This is *documented as intentional* in the
library README (a WebP codec would require a cgo/native dependency, which the
port deliberately avoids), so it is a scope decision rather than a defect, but it
does mean the port cannot cover sharp's most commonly used output format.
Supported: read PNG/JPEG/GIF/BMP/uncompressed-TIFF/raw, write the same set.

### 2. `ToFile(path, FormatRaw, …)` silently writes a PNG

`ToFormat` routes `FormatRaw` to `ToRaw`, but `ToFile`'s switch lumps it in with
PNG:

```go
case FormatPNG, FormatUnknown, FormatRaw:
    data, err = p.ToPNG()
```

So `ToFile(p, sharp.FormatRaw, 0)` on a 10x10 image writes a 273-byte PNG rather
than the 400 raw bytes `ToFormat(sharp.FormatRaw, 0)` returns for the same
image. Two "write in format X" entry points disagree for the same `Format`
value, with no error reported. The example prints this discrepancy in section 5.

### 3. `Boolean` with `BoolXor` destroys the alpha channel

`Boolean` applies the operator to alpha as well, so XOR-ing two fully opaque
images gives `255 ^ 255 == 0` — a completely transparent result. The doc comment
says alpha is combined with the same operator, so it is intentional, but it makes
`BoolXor` unusable on opaque images without a follow-up `EnsureAlpha` or
`Flatten`. Section 10 prints the resulting alpha of 0.

### 4. `Metadata().Channels` is inferred, not read from the source

`Channels` is computed as `3` or `4` purely from whether any pixel has
`alpha != 255`, not from the decoded image's actual channel count. A 4-channel
RGBA PNG that happens to be fully opaque reports `Channels: 3`. Node sharp
reports the true channel count.

### 5. Poor zero-value defaults on `ResizeOptions`

`ResizeOptions{Width: 100}` silently uses `Nearest` (the `Interpolation` zero
value) — the lowest-quality kernel — and `FitExact` (the `Fit` zero value).
Node sharp defaults to Lanczos3 and `fit: "cover"`. The idiomatic-Go zero value
here is the worst possible choice for image quality, and it is easy to write a
resize that looks bad without realising why. `ResizeTo` at least documents that
it picks bilinear.

### 6. `Interpolation` constants are split across two `iota` blocks

`Nearest`/`Bilinear` are declared in `resize.go` as `iota` from 0, and
`Cubic`/`Mitchell`/`Lanczos3` in `kernels.go` as `iota + 2`. The two blocks must
be kept manually in sync; inserting a constant into the first block would
silently renumber the enum and alias two kernels. Not a user-facing bug today,
just fragile.

### 7. Missing sharp features (not attempted, no API exists)

No EXIF/ICC/orientation metadata at all (`Metadata` has no `Orientation` or
`EXIF` field, so there is no equivalent of sharp's EXIF auto-rotate or
`withMetadata`), no animated GIF/WebP frame handling, no SVG or text/`create`
input, no progressive/interlaced JPEG or PNG palette options, no `toBuffer`
`{info}` return, and no streaming interface (`FromReader` reads the whole input
into memory before decoding).

## Notes on ergonomics

Positives worth recording: the deferred-error pipeline (`p.Err()`, errors
short-circuiting the rest of the chain and surfacing at the terminal call) works
correctly and makes chains readable; `Clone()` genuinely deep-copies, verified in
section 14; `New()` copies the source image so the caller's `image.RGBA` is never
mutated; every error message is prefixed `sharp:` and includes the offending
values; and both US and UK spellings are provided as aliases
(`Normalise`/`Normalize`, `DominantColor`/`DominantColour`,
`ToColourspace`/`ToColorspace`). Zero dependencies and no cgo make it trivially
buildable and cross-compilable, which is the main trade the port makes against
codec coverage.
