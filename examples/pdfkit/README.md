# pdfkit example — feature tour

A single runnable program that builds a 6-page PDF with
[`github.com/malcolmston/pdfkit`](https://github.com/malcolmston/pdfkit), then
re-parses the bytes it produced and prints structural sanity checks.

Consumed as a published module (no `replace` directive, no local path):

```
github.com/malcolmston/pdfkit v0.0.0-20260719012937-9781bf83a2b1
```

(The repo has no semver tags, so `@latest` resolves to that pseudo-version.
It is byte-identical to the local working tree at the time of writing.)

## Run

```sh
cd examples/pdfkit
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

No network access is needed beyond the module download, no external font or
image files are read, and the program terminates on its own. It writes
`example.pdf` next to `main.go`.

## What it demonstrates

**Document / output**
- `New`, `AddPage`, `PageCount`, `Bytes`, `Save`
- All six `/Info` metadata setters (`SetTitle`/`SetAuthor`/`SetSubject`/
  `SetKeywords`/`SetCreator`/`SetProducer`)
- `Parse` + `Reader.ObjectCount`/`Reader.Object` to read the result back

**Page sizes** — `A3`/`A4`/`A5`/`Letter`/`Legal`/`Tabloid`, `Landscape`,
`Portrait`, `Custom`, `Page.Size`, plus `SizeToPoint` for CSS-ish size strings
(`1in`, `10mm`, `2cm`, `12pt`, `3pc`). The document deliberately mixes A4,
Letter, landscape A5 and a 300x200 custom page. Margins are drawn as a dashed
guide box (see holes: the library has no margin concept).

**Text** — all 14 standard fonts at several sizes, `StandardFont` lookup,
`SetFont`, `DrawText`, `DrawLines`, `DrawParagraph`, `DrawTextBox` with all four
`TextAlign` values plus `Underline`/`Strike`, `DrawTextAligned`,
`DrawTextCentered`, `DrawTextRight`, `WrapText`, `TruncateToWidth`,
`Font.Width`/`Ascent`/`Descent`/`CapHeight`/`XHeight`/`LineHeight`/
`HeightOfString`, `Page.TextWidth`.

**Color** — `RGB`, `Gray`, `HSL`, `Hex`/`Color.Hex`, `NamedColor`, `CMYK` +
`SetFillCMYK`, `SetFillColor`, `SetStrokeColor`.

**Vector graphics** — `MoveTo`/`LineTo`/`CurveTo`/`ClosePath`, `DrawLine`,
`Rect`, `RoundedRect`, `Circle`, `Ellipse`, `Polyline`, `Polygon`,
`RegularPolygon`, `SVGPath` (arc + quadratic segments), `Stroke`, `Fill`,
`FillStroke`, `FillEvenOdd`, `Clip`, `SetLineWidth`, `SetLineCap`, `SetLineJoin`,
`SetMiterLimit`, `SetDash`, `Save`/`Restore`, `Translate`/`Scale`/`Transform`,
`LinearGradient`/`RadialGradient` + `Shade`, `NewShadingPattern`,
`NewTilingPattern`, `SetFillPattern`, `NewExtGState`/`ApplyExtGState`,
`SetAlpha`, `SetBlendMode`.

**Images** — four images generated in memory with `image/png` and `image/jpeg`:
an RGBA PNG with a transparent wedge (exercises the `/SMask` path), an 8-bit
grayscale PNG, a JPEG (embedded as `DCTDecode`), and the same PNG bytes loaded
through the sniffing `LoadImage`. Placed with `DrawImage`, including one scaled
under a transform and one clipped to a circle.

**Interactive bits** — `AddLinkURI`, `AddLinkTo`, `AddNamedDest` +
`AddLinkToDest`, `AddTextAnnotation`, `Outline`/`Outline.Add`/`Bookmark.Add`,
`AddTextField`, `AddCheckbox`, `AddPushButton`.

## Verification

Beyond the in-program checks (size > 20 KB, `%PDF-` prefix, `%%EOF` suffix,
`Save()` bytes equal `Bytes()`, xref/trailer present, every xref entry resolves
to a non-empty object, expected dictionary keys present), the output was checked
externally: `qpdf --check` reports "no syntax or stream encoding errors",
`pdfinfo` reads back all six metadata fields and 6 pages, and `pdftoppm`
rasterizes all six pages without warnings. Every drawing feature above renders
correctly in the rasterized pages.

## Holes / rough edges found

Real problems (nothing here was worked around with fake code; only the two
`// HOLE:` comments in `main.go` mark APIs that do not exist):

1. **No margin support at all.** Despite margins being a first-class PDFKit
   concept, there is no margin API and no layout cursor: `AddPage` only takes a
   `PageSize`, and every drawing call takes absolute coordinates. The example
   defines its own `margins` struct and does all layout arithmetic by hand.
   Related upstream-PDFKit conveniences are also absent: no text flow with
   automatic page breaks, no `moveDown`, no lists, no tables, no column layout.
2. **No `Page.Rotate`.** `Translate`, `Scale` and raw `Transform` exist, but
   rotation must be written out as a hand-computed matrix, even though the
   package already carries internal `sinApprox`/`cosApprox` helpers. Marked
   `// HOLE:` in `buildVectorPage`.
3. **`*FormField` is an opaque, useless return value.** `AddTextField`,
   `AddCheckbox` and `AddPushButton` return `*FormField`, but the type has no
   exported fields and no methods, so the caller can do nothing with it — no
   font size, border style, read-only/required flags, or export value. Fields
   are emitted with `/Helv 0 Tf` (auto-size) plus `/NeedAppearances true` and no
   appearance streams, so viewers synthesize wildly oversized field text
   (visible when rasterizing page 4). Marked `// HOLE:` in `buildImagePage`.
4. **`NewTilingPattern`'s callback page panics on text or images.** The cell is
   constructed as a `&Page{...}` with a nil `doc`, so `cell.SetFont(...)`,
   `cell.DrawImage(...)` or `cell.Shade(...)` nil-panic instead of failing
   gracefully. The doc comment says text and images "are not supported", but a
   nil-pointer dereference in a user callback is a harsh way to enforce that.
5. **Non-idiomatic error handling on the drawing surface.** Almost every method
   silently no-ops on misuse: `DrawText`/`DrawTextBox` do nothing (and
   `DrawTextBox` returns `y` unchanged) if no font was set, and `Font.Width`
   returns 0 for a nil font. Only `SVGPath` returns an `error`. This makes a
   forgotten `SetFont` produce a blank page with no signal.
6. **Symbol / ZapfDingbats text is effectively unusable from Go strings.** The
   fonts are exposed and their widths are correct, but the bytes written are the
   Go string's, so `DrawText` with ASCII against `Symbol` renders unrelated
   glyphs. There is no encoding/glyph-name helper to select a dingbat.
7. **Minor: `NamedColor` covers only the 147 CSS Level-2 keywords** (matching
   upstream PDFKit), so CSS Color 4 additions like `rebeccapurple` are missing
   and the lookup is case-sensitive with no normalization.
8. **Minor: `LoadTrueType`/`LoadTrueTypeFile` exist and would embed and subset a
   TTF, but the package bundles no font file**, so the whole embedded-font code
   path (a large chunk of the library) cannot be exercised without an external
   asset. Not used here, per the no-external-files constraint.
9. **Minor README/doc gaps.** The README documents only a small fraction of the
   API — gradients, patterns, transparency, blend modes, clipping, dashes, SVG
   paths, annotations, outlines, form fields, encryption, XMP, TrueType
   embedding and `SizeToPoint` are all undocumented there (some are covered in
   `doc.go`, some nowhere). The README's `LoadImage` snippet also reads from a
   file, which is the only image entry point shown.

Things that worked exactly as documented: deterministic output (`Save` bytes ==
`Bytes` bytes), the xref/trailer structure, AFM text widths, PNG soft masks,
JPEG pass-through, gradients, patterns, alpha/blend, clipping, SVG path parsing,
outlines, annotations, and RC4-128/AES-128 encryption plus XMP metadata (tried
separately; `qpdf --password=... --check` passes — note that a zero-value
`EncryptOptions` denies every permission including printing, which is
documented).
