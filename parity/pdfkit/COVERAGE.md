# COVERAGE — `foliojs/pdfkit` vs `github.com/malcolmston/pdfkit`

| | |
| --- | --- |
| upstream (oracle) | `pdfkit@0.15.1` (`parity/pdfkit/node/package.json`, installed under `node/node_modules`) |
| Go module | `github.com/malcolmston/pdfkit v0.0.0-20260810111600-857c462c84f5` |
| harness | `go test ./parity/pdfkit/` — 119 cases, regenerating `parity.json` |
| case parity | **6.72%** (8/119 compared cases) strict; **43.70%** (52/119) with the six systematic divergences masked |
| symbol parity | **45.45%** (30 match / 66 compared) — see the counts at the bottom |

## How the upstream inventory was produced

Mechanically, from the installed package — never from the README or from memory.

**Prototype methods** (107 public entries, run in `parity/pdfkit/node/`):

```sh
node -e "
const P = require('pdfkit').prototype;
const skip = new Set(['constructor','toString']);
console.log(Object.getOwnPropertyNames(P)
  .filter(n => !n.startsWith('_') && !skip.has(n)).sort().join('\n'));
"
```

`PDFDocument` extends `stream.Readable`, so its prototype chain also carries
Node's stream and `EventEmitter` methods (`pipe`, `on`, `read`, `destroy`, …).
Those are Node stream plumbing, not the PDF API, and are excluded — walking the
chain with `Object.getPrototypeOf` shows them at depths 1-3, all owned by
`Readable`/`Stream`/`EventEmitter`.

**Constructor options**, from the compiled bundle:

```sh
node -e "
const src = require('fs').readFileSync(require.resolve('pdfkit/js/pdfkit.js'), 'utf8');
const s = new Set(); let m;
const re = /this\.options\.([A-Za-z_][A-Za-z0-9_]*)/g;
while ((m = re.exec(src))) s.add(m[1]);
console.log([...s].sort().join(' '));
"
# autoFirstPage bufferPages compress displayTitle expanded ignoreOrientation info lang
```

That misses the options read as a bare `options.x` inside the constructor and the
security handler, so the same grep was repeated for `options\.` and the results
intersected with the constructor and `PDFSecurity.create` bodies, which adds
`pdfVersion`, `size`, `layout`, `margin`, `margins`, `font`, `subset`, `tagged`,
`userPassword`, `ownerPassword` and `permissions`. The seven `permissions` keys
come from the body of `getPermissionsR3` in the same file.

**Per-call option bags** (text, image, annotation, list, outline) come from the
same `/options\.([A-Za-z_][A-Za-z0-9_]*)/g` sweep over the bundle, which yields
97 distinct names; the PDF dictionary keys it also picks up (`Rect`, `Subtype`,
`QuadPoints`, …) are grouped into the single annotation-options row below rather
than listed one by one.

## The headline: pdfkit's text model is a flow layout, and the port has none

pdfkit's whole text API is **flow-based**. A document has margins, a layout
cursor (`doc.x`, `doc.y`), and `doc.text(str)` with no coordinates at all lays
the string out from the cursor, wraps it inside the margins, advances the cursor,
and **starts a new page automatically** when it runs out of room. `moveDown`,
`moveUp`, `continued`, `columns`, `paragraphGap`, `indent` and `list` all build
on that cursor.

The port has **no part of it**:

* `Document.AddPage` takes a `PageSize` and nothing else — there is no margin
  parameter, field or option anywhere in the package;
* there is no layout cursor, so every text call takes explicit coordinates;
* there is no automatic page break: text that runs off the bottom of the page is
  simply lost;
* there are no lists and no tables;
* `DrawText` is a **silent no-op** if `SetFont` was never called, where upstream
  starts with Helvetica selected.

What the port does offer is fixed-coordinate wrapping: `DrawParagraph`,
`DrawTextBox`, `DrawTextAligned`, `DrawLines`, plus `WrapText` and
`TruncateToWidth`. Those cover wrapping to a width, alignment and leading, which
is why `text-wrapped-box` and `text-aligned-in-width` can be compared at all. But
every case that needs the cursor (`text-flow-cursor`, `text-move-down`,
`text-list`, `text-doc-line-gap`) has no port counterpart and is `missing` below.

In inventory terms: of the option bag behind `doc.text` — 20+ documented keys —
the port answers five (`align`, `width`, `lineGap`, `underline`, `strike`) and
nothing else.

## Real divergences found

Ordered worst-first. None of these is a formatting artefact; each was found by
parsing both files back out and comparing the canonical structure.

1. **The tiling-pattern callback nil-panics.** `NewTilingPattern`'s `draw
   func(*Page)` receives a `Page` with no document behind it, so `SetFont`,
   `DrawText`, `DrawImage` and `Shade` dereference nil and panic
   (`pat-tiling-text-in-cell`). The Go runner wraps every call in `recover()`, so
   the panic is reported as `ok:false` rather than killing the runner; it is
   listed in `parity.json` under `goPanicCases`.
2. **No pair kerning.** Upstream applies the AFM kern pairs and emits `TJ`
   adjustments (e.g. `adj=40` for "Hello, PDF!"); the port emits a bare `Tj`.
   Glyph positions inside a run therefore differ, and because `widthOfString`
   inherits the same gap, right- and centre-aligned text lands in a different
   place (`text-aligned-in-width`: 274.456 upstream vs 274.636).
3. **Encryption permission bits are wrong.** For the same requested permissions
   the port writes `/P -4` where upstream writes `/P -1796`: it leaves the
   form-filling, accessibility and document-assembly bits set unconditionally and
   does not apply upstream's reserved-bit base mask `0xfffff0c0`. A document the
   caller locked down is not locked down (`enc-all-permissions`).
4. **Circles and ellipses are traced in the opposite winding direction.** The
   four-Bézier geometry is identical to 1e-4, but the control points run the other
   way round (`vec-circle`, `vec-ellipse`, `clip-circle`, `clip-then-image`).
   Harmless on its own; it changes the result of a nonzero-winding fill that
   combines a circle with another subpath.
5. **Link and widget annotations omit `/F 4`.** Upstream sets the Print flag on
   every annotation it creates; the port sets no flags, so links and form fields
   may not print (`annot-link-uri`, `form-text-field`).
6. **Widget annotations omit `/Border`**, and checkbox `/V` is written as a name
   (`/Yes`) where upstream writes a string (`(Yes)`). The port is arguably right
   per the spec here, but they are not the same file. Neither side emits
   appearance streams; both rely on `/NeedAppearances true`.
7. **`*FormField` is opaque.** It has no exported fields and no setters, so none
   of upstream's field options — alignment, font size, border colour, background
   colour, required, multiline, flags — can be set at all
   (`form-text-field-configured`).
8. **Tiling patterns are a different kind of pattern.** Upstream's are uncoloured
   (`PaintType 2`, `TilingType 2`) with the colour supplied when the pattern is
   selected; the port's are coloured (`PaintType 1`, `TilingType 1`). The port
   also leaves `/Matrix` at identity, so the tiling phase is anchored to the page
   bottom-left instead of the top-left: the tiles land offset (`pat-tiling-fill`).
9. **PNG images are re-encoded.** Upstream passes the original IDAT through with
   `/DecodeParms /Predictor 15`; the port inflates and re-deflates raw RGB
   samples. Same pixels, bigger file, different XObject (`img-png-rgb`,
   `img-png-gray`). JPEG placement and `DCTDecode` agree exactly, soft masks
   included.
10. **Outline destinations use `/XYZ` instead of `/Fit`** (`outline-flat`), and
    named destinations write `null` where upstream writes `0` for the `/XYZ` left
    coordinate (`annot-named-dest-and-link`).
11. **`ExtGState` objects are never reused.** Two `SetAlpha(0.5)` calls emit two
    identical `/ExtGState` dictionaries where upstream emits one
    (`alpha-reused-state`).
12. **Sticky notes are placed differently.** `AddTextAnnotation` takes a point,
    not a rectangle, and anchors it 20 pt above upstream's rectangle
    (`annot-sticky-note`).
13. **Input validation is looser.** `SetDash` accepts zero and negative lengths
    that upstream rejects (`vec-dash-zero-length`), and `SVGPath` accepts
    malformed path data that upstream rejects (`vec-svg-path-malformed`).
14. **Wrapped lines are trimmed differently.** `DrawTextBox` drops the trailing
    space from each wrapped line; upstream shows it. The break points and the
    11.1 pt line advance are identical (`text-wrapped-box`).
15. **Empty metadata values are dropped.** `SetTitle("")` writes no `/Title` at
    all; upstream writes an empty string (`meta-empty-strings`).
16. **No XMP for encrypted documents.** Upstream emits an XMP `/Metadata` stream
    whenever encryption is on; the port emits none (`enc-*`).

One place the port is *more* correct than upstream: `SVGPath` elevates SVG's
quadratic `Q`/`T` to a proper cubic, where upstream emits a `v` operator using
the quadratic control point as the second cubic control point — a visibly
different curve (`vec-svg-path-curves`). It also correctly omits
`/Encoding /WinAnsiEncoding` from Symbol and ZapfDingbats, which upstream stamps
on unconditionally (`text-standard-14`).

## The six systematic divergences (masked in the structural score only)

These are uniform across every case, so they are reported once here and masked
when computing the structural parity percentage. The strict percentage does not
mask them, which is why it is so much lower.

1. **Header version.** Upstream writes `%PDF-1.3` by default (and 1.4/1.5/1.6/1.7
   on request); the port always writes `%PDF-1.7` and has no option.
2. **`/Info` `CreationDate` and `ModDate`.** The port has no API for either and
   writes neither; upstream always writes `CreationDate`.
3. **The trailer `/ID`.** Upstream always writes one, derived from `/Info`; the
   port writes one only when encrypting, derived from a digest of the document.
4. **The `/ProcSet` resource array.** Upstream writes
   `[/PDF /Text /ImageB /ImageC /ImageI]` on every page; the port writes none.
5. **Colour-operator spelling.** Upstream selects a device colour space and then
   sets the colour (`/DeviceRGB cs` + `1 0 0 scn`); the port uses the single-shot
   form (`1 0 0 rg`). Identical instructions.
6. **`q` / `Q` / `cm` bookkeeping.** Neither marks the page — their only effect is
   on later coordinates, which the trace already reports in device space (and the
   device matrix is attached to `Do` and `sh` directly). They differ
   systematically because the port has no flipped user space, so the runner
   re-applies pdfkit's page flip around every text and image call, adding one
   `q`/`cm`/`Q` nesting level upstream does not need.

## Determinism: what is pinned and what is normalised

**Pinned** (both runners produce byte-stable output):

* upstream `info.CreationDate` is fixed to `2026-01-02T15:04:05Z`. That also
  fixes pdfkit's file id, which is an MD5 over the `/Info` dictionary — so
  nothing random is left in the upstream output.
* `info.Producer` and `info.Creator` are pinned to `"parity"` on both sides
  (upstream defaults to `"PDFKit"`, the port to `"pdfkit"`), so the metadata
  comparison is about the mechanism, not the brand.
* upstream `compress: false`, because the port never compresses page content
  streams; this makes the `/Filter` of page contents comparable instead of
  guaranteed-different.
* upstream `autoFirstPage: false` and `bufferPages: true`, so page creation is
  explicit on both sides and every page stays addressable.
* the port stamps no date and has no random id, so nothing needs pinning there.
* every image input is a committed fixture generated by
  `GOWORK=off go run ./fixtures/gen` (8x8 RGB PNG, 8x8 RGBA PNG, 8x8 grey PNG,
  16x16 quality-90 JPEG). Both runners read the same bytes off disk.

**Normalised** in the canonical form:

* the trailer `/ID` is reduced to presence-only — the two derive it from
  different inputs by design.
* `/Info` `CreationDate`/`ModDate` values become `<date>`; only presence is
  compared.
* once a document is encrypted every string and stream in it is ciphertext, so
  `/Info` values become `<encrypted>` and the page content trace becomes
  `<encrypted: content stream not comparable>`. Structure — MediaBox, resources,
  the encryption dictionary's `Filter`/`V`/`R`/`Length`/`P`/`CFM` — is still
  plaintext and still compared.
* resource names (`/F1` vs `/F0`, `/I1` vs `/Im0`) are replaced by a description
  of the resource they name, because the names themselves are arbitrary.
* pattern `/Matrix` is composed into the geometry it governs (shading `/Coords`
  and radii, tiling `/BBox` and `/XStep`/`/YStep`), because upstream folds the
  page flip into the matrix while the port pre-flips the coordinates.

## Numeric precision

Every number in the canonical form is **snapped to the 1e-4 grid and then rounded
to 3 decimal places**.

The snap comes first because the port serialises every coordinate with exactly
four decimal places (`strconv.FormatFloat(v, 'f', 4, 64)`), while upstream writes
full double precision. Without it the two sides can be split by the port's *own*
rounding: upstream's `103.4314575` rounds to `103.431` and the port's printed
`103.4315` rounds to `103.432`, which is not a difference in geometry. Snapping
to 1e-4 first puts both on the same value whenever they agree to within the
port's output resolution.

Three decimals is then the comparison tolerance. 0.001 pt is 1/72000 inch — about
0.35 µm, roughly 1/4000 of a pixel at 72 dpi — so it is far below anything that
could affect rendering. It is also far above the noise floor: the two
implementations reach the same coordinate through different arithmetic (the same
kappa constant applied in a different order, CTM composition, AFM metrics divided
by 1000), which introduces error around 1e-9 to 1e-12. Colours live in [0,1]
where 0.001 still separates adjacent 8-bit levels (1/255 = 0.0039), so the same
precision serves them.

## Inventory: `PDFDocument.prototype` (107 public methods)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `addContent` | — | missing | — | raw operator injection; the port has no escape hatch into the content stream |
| `addNamedDestination` | `Document.AddNamedDest` | differs | `annot-named-dest-and-link` | /XYZ left is 0 upstream, null in the port |
| `addNamedEmbeddedFile` | — | missing | — | named embedded-file tree |
| `addNamedJavaScript` | — | missing | — | document-level JavaScript |
| `addPage` | `Document.AddPage` | match | `page-single-a4`, `page-letter`, `page-sizes-all`, `page-custom-size`, `page-landscape`, `page-many` | geometry agrees; the margin/margins options are separately missing |
| `addStructure` | — | missing | — | tagged-PDF structure tree |
| `annotate` | — | missing | — | the generic annotation writer every annotation helper builds on |
| `appendXML` | — | missing | — | appends to the XMP packet; Document.SetXMP replaces it wholesale (listed as extra) |
| `arc` | — | missing | — | no arc primitive; only SVGPath's A command flattens arcs |
| `bezierCurveTo` | `Page.CurveTo` | match | `vec-bezier` |  |
| `bufferedPageRange` | — | missing | — | the port has no page buffering to interrogate |
| `circle` | `Page.Circle` | differs | `vec-circle`, `clip-circle`, `grad-radial-fill` | same four-Bezier geometry traced in the opposite winding direction |
| `clip` | `Page.Clip, Page.ClipEvenOdd` | match | `clip-rect`, `clip-even-odd` |  |
| `closePath` | `Page.ClosePath` | match | `vec-close-path` |  |
| `continueOnNewPage` | — | missing | — | tagged-PDF marked-content continuation |
| `createStructParentTreeNextKey` | — | missing | — | tagged PDF |
| `currentLineHeight` | `Font.LineHeight` | untested | — | measurement only; never reaches the content stream, so no case can compare it |
| `dash` | `Page.SetDash` | differs | `vec-dash-simple`, `vec-dash-phase`, `vec-dash-zero-length` | the port accepts zero and negative dash lengths that upstream rejects |
| `ellipse` | `Page.Ellipse` | differs | `vec-ellipse` | winding direction reversed, as circle |
| `ellipseAnnotation` | — | missing | `annot-ellipse` |  |
| `end` | `Document.Bytes, Document.Write` | match | `every case` | serialisation; exercised by all 119 cases |
| `endAcroForm` | `(inside Document.Write)` | differs | `form-text-field`, `form-mixed` | the port omits /Border on widgets and writes /V as a name for buttons |
| `endMarkedContent` | — | missing | — | tagged PDF |
| `endMarkings` | — | missing | — | tagged PDF |
| `endMetadata` | — | missing | `enc-rc4-128-user-password` | upstream emits an XMP /Metadata stream for encrypted documents; the port emits none |
| `endOutline` | `(inside Document.Write)` | differs | `outline-flat`, `outline-nested` | destination mode /Fit upstream vs /XYZ in the port |
| `endPageMarkings` | — | missing | — | tagged PDF |
| `file` | — | missing | — | embedded file streams |
| `fileAnnotation` | — | missing | — | file-attachment annotation |
| `fill` | `Page.Fill, Page.FillEvenOdd` | match | `vec-rect-fill`, `vec-even-odd-fill` |  |
| `fillAndStroke` | `Page.FillStroke` | match | `vec-rect-fill-stroke` |  |
| `fillColor` | `Page.SetFillColor, Page.SetFillCMYK, Page.SetFillPattern` | match | `vec-rect-fill`, `vec-cmyk-colours`, `text-fill-colour` | plain colours agree once the cs+scn vs rg/k spelling is folded; gradients and patterns are scored on their own rows |
| `fillOpacity` | `Page.SetAlpha` | differs | `alpha-fill-opacity`, `alpha-reused-state` | the port allocates a fresh ExtGState per call instead of reusing an identical one |
| `flushPages` | — | missing | — | the port has no streaming page flush |
| `font` | `Page.SetFont, pdfkit.StandardFont` | differs | `text-standard-14` | upstream stamps /Encoding /WinAnsiEncoding on Symbol and ZapfDingbats too; the port correctly omits it |
| `fontSize` | `Page.SetFont (size argument)` | match | `text-sizes` | the Tf size operand agrees; the kerning difference is scored on text/widthOfString |
| `formAnnotation` | — | missing | — | the generic widget-annotation writer |
| `formCheckbox` | `Page.AddCheckbox` | differs | `form-checkbox`, `form-mixed` | /V is a name in the port and a string upstream; no /Border |
| `formCombo` | — | missing | — | combo box |
| `formField` | — | missing | — | field groups and hierarchies; *FormField is opaque |
| `formList` | — | missing | — | list box |
| `formPushButton` | `Page.AddPushButton` | differs | `form-push-button`, `form-mixed` | no /Border; caption is not otherwise configurable |
| `formRadioButton` | — | missing | — | radio button |
| `formText` | `Page.AddTextField` | differs | `form-text-field`, `form-text-field-configured` | no /Border; none of upstream's options (align, fontSize, multiline, required, borderColor, backgroundColor, …) can be expressed |
| `getMarkInfoDictionary` | — | missing | — | tagged PDF |
| `getStructParentTree` | — | missing | — | tagged PDF |
| `getStructTreeRoot` | — | missing | — | tagged PDF |
| `goTo` | `Page.AddLinkToDest` | differs | `annot-named-dest-and-link` | the port omits the /F 4 (Print) annotation flag |
| `heightOfString` | `Font.HeightOfString` | untested | — | measurement only |
| `highlight` | — | missing | `annot-highlight` |  |
| `image` | `Page.DrawImage` | differs | `img-png-rgb`, `img-png-rgba`, `img-png-gray`, `img-jpeg`, `img-under-transform` | JPEG placement and DCTDecode agree exactly; PNG is re-encoded as raw FlateDecode RGB instead of passing the IDAT through with /DecodeParms /Predictor 15 |
| `initColor` | — | missing | — | mixin initialiser; the port has no explicit initialisation step |
| `initFonts` | — | missing | — | mixin initialiser |
| `initForm` | — | missing | `form-text-field` | upstream requires it before any field; the port creates the AcroForm implicitly |
| `initImages` | — | missing | — | mixin initialiser |
| `initMarkings` | — | missing | — | tagged PDF |
| `initMetadata` | — | missing | — | XMP |
| `initOutline` | — | missing | — | the port creates the outline lazily in Document.Outline |
| `initPageMarkings` | — | missing | — | tagged PDF |
| `initSubset` | — | missing | — | font subsetting options |
| `initText` | — | missing | — | mixin initialiser |
| `initVector` | — | missing | — | mixin initialiser |
| `lineAnnotation` | — | missing | `annot-line` |  |
| `lineCap` | `Page.SetLineCap` | match | `vec-line-caps` |  |
| `lineGap` | — | missing | `text-doc-line-gap` | document-level leading; the port only has a per-call TextOptions.LineGap |
| `lineJoin` | `Page.SetLineJoin` | match | `vec-line-joins` |  |
| `lineTo` | `Page.LineTo` | match | `vec-line-stroke` |  |
| `lineWidth` | `Page.SetLineWidth` | match | `vec-line-width` |  |
| `linearGradient` | `pdfkit.LinearGradient` | differs | `grad-linear-fill`, `grad-linear-three-stops`, `grad-stroke`, `grad-stop-opacity` | two-stop gradients use a bare Type 2 function where upstream always wraps in a Type 3 stitching function; no per-stop opacity |
| `link` | `Page.AddLinkURI` | differs | `annot-link-uri`, `annot-link-two` | the port omits the /F 4 (Print) annotation flag |
| `list` | — | missing | `text-list` | no list API of any kind |
| `markContent` | — | missing | — | tagged PDF |
| `markStructureContent` | — | missing | — | tagged PDF |
| `miterLimit` | `Page.SetMiterLimit` | match | `vec-miter-limit` |  |
| `moveDown` | — | missing | `text-move-down` | no layout cursor to move |
| `moveTo` | `Page.MoveTo` | match | `vec-line-stroke` |  |
| `moveUp` | — | missing | `text-move-down` | no layout cursor to move |
| `note` | `Page.AddTextAnnotation` | differs | `annot-sticky-note` | same /Subtype /Text, but the port takes a point rather than a rectangle (its rect sits 20 pt higher), omits /Border and omits /F |
| `opacity` | `Page.SetAlpha` | differs | `alpha-both` | as fillOpacity: no ExtGState reuse |
| `openImage` | `pdfkit.LoadImage, LoadPNG, LoadJPEG` | differs | `img-png-rgb`, `img-jpeg` | see image: the PNG embedding strategy differs |
| `path` | `Page.SVGPath` | differs | `vec-svg-path`, `vec-svg-path-curves`, `vec-svg-path-arc`, `vec-svg-path-malformed` | the port elevates SVG Q/T correctly to a cubic while upstream emits a v operator with the quadratic control point (a different curve); the port also accepts malformed path data that upstream rejects |
| `pattern` | `pdfkit.NewTilingPattern` | differs | `pat-tiling-fill`, `pat-tiling-text-in-cell` | upstream patterns are uncoloured (PaintType 2, TilingType 2) with the colour supplied at selection; the port's are coloured (PaintType 1, TilingType 1). The port leaves /Matrix at identity so the tiling phase is anchored to the page bottom-left rather than the top-left. The cell callback receives a Page with no document behind it and nil-panics on SetFont, DrawText, DrawImage and Shade. |
| `polygon` | `Page.Polygon` | match | `vec-polygon` |  |
| `quadraticCurveTo` | — | missing | `vec-quadratic` | only cubic CurveTo |
| `radialGradient` | `pdfkit.RadialGradient` | differs | `grad-radial-fill` | as linearGradient: bare Type 2 function instead of a Type 3 stitching wrapper |
| `rect` | `Page.Rect` | match | `vec-rect-fill` |  |
| `rectAnnotation` | — | missing | `annot-rect` |  |
| `ref` | — | missing | — | indirect-object allocation is not exposed by the port |
| `registerFont` | `pdfkit.LoadTrueType, LoadTrueTypeFile` | untested | — | embedded fonts need a real font-file fixture; out of scope for this harness |
| `restore` | `Page.Restore` | match | `vec-save-restore`, `alpha-scoped` |  |
| `rotate` | — | missing | `xf-rotate`, `xf-rotate-origin` | no rotation transform at all, with or without an origin |
| `roundedRect` | `Page.RoundedRect` | match | `vec-rounded-rect` |  |
| `save` | `Page.Save` | match | `vec-save-restore`, `alpha-scoped` |  |
| `scale` | `Page.Scale` | match | `xf-scale`, `xf-nested` |  |
| `strike` | — | missing | `annot-strike` | the strike-out *annotation*; TextOptions.Strike draws a line instead |
| `stroke` | `Page.Stroke` | match | `vec-line-stroke` |  |
| `strokeColor` | `Page.SetStrokeColor, Page.SetStrokeCMYK, Page.SetStrokePattern` | match | `vec-rect-fill-stroke`, `vec-cmyk-colours` |  |
| `strokeOpacity` | `Page.SetAlpha` | differs | `alpha-stroke-opacity` | as fillOpacity |
| `struct` | — | missing | — | tagged PDF |
| `switchToPage` | — | missing | — | the port hands back a *Page per page, so there is no current-page cursor to switch |
| `text` | `Page.DrawText, DrawTextAligned, DrawTextBox, DrawLines, DrawParagraph` | differs | `text-helvetica-basic`, `text-standard-14`, `text-aligned-in-width`, `text-wrapped-box`, `text-wrapped-justify`, `text-wrapped-linegap`, `text-underline-strike`, `text-no-font-set`, `text-winansi-high-bytes`, `text-char-spacing`, `text-flow-cursor` | glyph placement agrees to the baseline for left/centre alignment, but the port applies no AFM pair kerning (upstream emits TJ adjustments), silently draws nothing when no font has been set, has no layout cursor, no automatic page breaks and no characterSpacing. Wrapped lines agree on break points and line advance but the port trims each line's trailing space, which upstream shows. |
| `textAnnotation` | — | missing | `annot-free-text` | /FreeText annotation (the name is misleading); the port has no equivalent |
| `transform` | `Page.Transform` | match | `xf-matrix` |  |
| `translate` | `Page.Translate` | match | `xf-translate`, `xf-nested` |  |
| `undash` | `Page.SetDash (empty pattern)` | match | `vec-undash` |  |
| `underline` | — | missing | `annot-underline` | the underline *annotation*; TextOptions.Underline draws a line instead |
| `widthOfString` | `Page.TextWidth, Font.Width` | differs | `text-aligned-in-width` | no pair kerning, so measured widths differ by up to a few tenths of a point and right/centre alignment lands elsewhere |

## Inventory: constructor / `addPage` options and encryption options

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `options.autoFirstPage` | — | missing | — | the port never creates an implicit first page, which is the same end state as autoFirstPage:false; there is no way to ask for the implicit page |
| `options.bufferPages` | — | missing | — | every page stays addressable in the port, so buffering has no analogue |
| `options.compress` | — | missing | — | the port never compresses page content streams and offers no switch |
| `options.displayTitle` | — | missing | — | /ViewerPreferences /DisplayDocTitle |
| `options.ignoreOrientation` | — | missing | — | JPEG EXIF orientation handling |
| `options.info` | `Document.SetTitle/SetAuthor/SetSubject/SetKeywords/SetProducer/SetCreator` | differs | `meta-all-fields`, `meta-producer-creator`, `meta-unicode-title`, `meta-empty-strings` | values agree, including UTF-16BE for non-Latin text, but the port drops empty-string values entirely and has no API for CreationDate, ModDate or arbitrary custom /Info keys |
| `options.lang` | — | missing | — | /Lang in the catalog |
| `options.layout` | `PageSize.Landscape` | match | `page-landscape` |  |
| `options.margin` | — | missing | `page-margin-uniform` | no margin concept anywhere in the port |
| `options.margins` | — | missing | `page-margins-per-side` | no per-side margins |
| `options.size` | `Document.AddPage(PageSize), pdfkit.Custom` | match | `page-sizes-all`, `page-custom-size` |  |
| `options.font` | — | missing | `text-no-font-set` | the default document font; the port starts with no font and DrawText is a silent no-op until SetFont |
| `options.pdfVersion` | — | missing | `page-single-a4` | the port always writes %PDF-1.7; upstream defaults to 1.3 and selects 1.4/1.5/1.6/1.7 |
| `options.userPassword` | `EncryptOptions.UserPassword` | match | `enc-rc4-128-user-password` |  |
| `options.ownerPassword` | `EncryptOptions.OwnerPassword` | match | `enc-owner-only` |  |
| `options.permissions` | `EncryptOptions.Allow* flags` | differs | `enc-all-permissions`, `enc-aes-128` | the port computes a different /P: it leaves the form-filling, accessibility and document-assembly bits set unconditionally and does not clear upstream's reserved-bit base mask (0xfffff0c0), so /P is -4 where upstream writes -1796 |
| `options.permissions.printing (lowResolution/highResolution)` | `EncryptOptions.AllowPrinting` | differs | `enc-all-permissions` | boolean only: no low/high-resolution distinction |
| `options.permissions.modifying` | `EncryptOptions.AllowModify` | differs | `enc-all-permissions` | see options.permissions |
| `options.permissions.copying` | `EncryptOptions.AllowCopying` | differs | `enc-all-permissions` | see options.permissions |
| `options.permissions.annotating` | `EncryptOptions.AllowAnnotations` | differs | `enc-all-permissions` | see options.permissions |
| `options.permissions.fillingForms` | — | missing | `enc-all-permissions` | not expressible; the bit is always set |
| `options.permissions.contentAccessibility` | — | missing | `enc-all-permissions` | not expressible; the bit is always set |
| `options.permissions.documentAssembly` | — | missing | `enc-all-permissions` | not expressible; the bit is always set |
| `options.subset` | — | missing | — | PDF/A and PDF/UA subset conformance |
| `options.tagged` | — | missing | — | tagged PDF |

## Inventory: per-call option bags (text, image, list, annotation, outline)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `text options.align` | `TextOptions.Align, Page.DrawTextAligned` | differs | `text-aligned-in-width`, `text-wrapped-justify` | left and centre land identically; right and justify differ because the port measures without kerning |
| `text options.width` | `Page.DrawTextBox width` | differs | `text-wrapped-box` | the wrap points and the 11.1 pt line advance agree exactly; the port trims the trailing space off each wrapped line where upstream shows it, and applies no kerning |
| `text options.lineGap` | `TextOptions.LineGap` | differs | `text-wrapped-linegap` |  |
| `text options.underline` | `TextOptions.Underline` | differs | `text-underline-strike` | different line thickness and offset rules |
| `text options.strike` | `TextOptions.Strike` | differs | `text-underline-strike` | different line offset rules |
| `text options.lineBreak` | — | missing | `text-helvetica-basic` | the port has no wrapping switch: DrawText never wraps and DrawTextBox always does |
| `text options.characterSpacing` | — | missing | `text-char-spacing` | no Tc operator is ever emitted |
| `text options.wordSpacing` | — | missing | — | no Tw operator except as part of justification |
| `text options.baseline` | — | missing | — | only the alphabetic baseline |
| `text options.oblique` | — | missing | — | no synthetic italic skew |
| `text options.continued` | — | missing | `text-flow-cursor` | paragraph continuation needs the layout cursor |
| `text options.indent, textIndent, paragraphGap, columns, columnGap, ellipsis, height, fill, stroke, link, goTo, destination, features, rotation` | — | missing | `text-flow-cursor` | the rest of the flow-text option bag has no counterpart |
| `list options.bulletRadius, bulletIndent, textIndent, listType` | — | missing | `text-list` | no list API |
| `image options.width, height` | `Page.DrawImage width/height` | differs | `img-jpeg-non-square` | see image |
| `image options.fit, cover, align, valign, scale` | — | missing | — | no fitting, cropping or alignment helpers |
| `image options.link, goTo, destination` | — | missing | — | no link-from-image helpers |
| `annotation options (Border, C, Contents, F, Name, Q, DA, …)` | — | missing | `annot-link-uri` | annotation dictionaries are not configurable in the port |
| `outline options.expanded` | — | missing | `outline-expanded` | no initially-open outline entries |

## Inventory: Go-only surface (`extra`)

| Go symbol | what it is | cases | note |
| --- | --- | --- | --- |
| `Page.Shade` | paints a shading directly with the sh operator | `grad-sh-operator` | upstream always routes gradients through a shading pattern |
| `Page.SetBlendMode, pdfkit.BlendMode constants, pdfkit.NewExtGState` | the 16 PDF blend modes and explicit ExtGState construction | `alpha-blend-multiply` | upstream exposes no blend-mode API |
| `Page.AddLinkTo` | an internal link straight to another page object | `annot-link-to-page` | upstream internal links always go through a named destination |
| `Page.Polyline, Page.RegularPolygon` | open polyline and regular-polygon primitives | — | untested |
| `Page.DrawUnderline, Page.DrawStrikethrough` | standalone decoration strokes | — | untested |
| `Page.DrawLines, Page.DrawParagraph, Page.DrawTextCentered, Page.DrawTextRight` | fixed-coordinate multi-line and aligned text helpers | `text-wrapped-box` | the port's substitute for the flow model |
| `Document.EnableXMP, Document.SetXMP` | XMP packet generation from /Info, or a verbatim packet | — | untested; upstream builds XMP through initMetadata/appendXML/endMetadata |
| `Document.PageCount` | page count accessor | — | untested |
| `pdfkit.Reader, pdfkit.Parse` | a minimal xref reader for round-tripping | — | untested; no upstream counterpart |
| `pdfkit.NormalizeColor, pdfkit.SizeToPoint` | direct ports of upstream's internal _normalizeColor and sizeToPoint | — | exported by the port, private upstream |
| `pdfkit.WrapText, pdfkit.TruncateToWidth, Font.Width/Ascent/Descent/CapHeight/XHeight/LineHeight/HeightOfString` | measurement and wrapping helpers | — | untested |
| `pdfkit.Hex, NamedColor, HSL, Gray, RGB, CMYK, Color.Hex/CMYK/RGB` | colour construction and conversion helpers | `vec-cmyk-colours` |  |
| `Page.SetFillCMYK, Page.SetStrokeCMYK` | explicit DeviceCMYK setters | `vec-cmyk-colours` | upstream infers CMYK from a 4-element array |

## Counts

| status | count |
| --- | --- |
| match | 30 |
| differs | 36 |
| missing | 81 |
| untested | 3 |
| extra (Go-only, not in the denominator) | 13 |
| **total upstream symbols inventoried** | **150** |

**Symbol parity: 45.45%** — 30 `match` over a denominator of **66** symbols
actually compared (`match` + `differs`). `missing` (81) and `untested` (3) are
excluded from the denominator, as HARNESS.md requires: a symbol with no case is
never `match`, and a symbol with no Go counterpart cannot be compared. Over the
full inventory of 150 upstream symbols the port answers 66 of them at all
(44.0% coverage), and of those it agrees on 45.5%.

A symbol is `match` here when every case covering it agrees **after** the six
systematic divergences above are masked; strictly, almost nothing matches,
because the header version, `/Info` dates, `/ID` and `/ProcSet` differ in every
single file. That is what the two percentages in the summary table measure.

## Case-level score

From `parity.json`, regenerated by `GOWORK=off go test ./parity/pdfkit/`:

| | |
| --- | --- |
| cases | 119 |
| strict match | 8 |
| strict differs | 111 |
| strict parity % | 6.72 |
| structural match | 52 |
| structural parity % | 43.7 |
| both sides failed (parity on failure) | 8 |
| only the port failed (missing API) | 20 |
| only upstream failed (Go-only feature or looser validation) | 5 |
| harness errors | 0 |
| Go panics recovered | `pat-tiling-text-in-cell` |
| structurally invalid PDFs | none — every file produced by either side parses as a well-formed PDF |

The strict denominator is `match + differs` = 119: the 25 cases where one side
raised and the other did not are counted as `differs` (a port that cannot do
something upstream can do is not at parity), and the 8 cases where both raised
are counted as `match` (parity on failure, message text not compared).

