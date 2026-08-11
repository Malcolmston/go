# figlet — upstream API inventory vs the Go port

- **Upstream oracle:** `figlet@1.11.4` (npm, patorjk/figlet.js), pinned in
  `node/package.json`.
- **Port:** `github.com/malcolmston/chalk/figlet` — a nested package inside the
  `chalk` sub-repo that ports a *different* npm package, so it is scored here and
  not by `parity/chalk/`.
- **Score:** see `parity.json`, rewritten by `GOWORK=off go test .`

## How the upstream list was derived

Mechanically, from the real installed package — not from the README:

```console
$ cd node && node -e "import('figlet').then(m => { \
    for (const k of Object.keys(m.default).sort()) console.log(k, typeof m.default[k]); })"
```

and, for the option keys each entry point accepts, from the installed sources
`node_modules/figlet/dist/figlet-*.js` (`_reworkFontOpts`,
`getHorizontalFittingRules`, `getVerticalFittingRules`) and
`node_modules/figlet/dist/node-figlet.mjs`.

The Go side comes from `go doc -all ./figlet`.

## Which fonts are compared, and why those

This is the one thing that needs stating plainly, because the two sides do not
ship the same fonts.

Upstream bundles 328 real `.flf` FIGfonts (`node_modules/figlet/fonts/`). The Go
port bundles **no upstream `.flf` files at all**: its registry (`Fonts()`) holds
its own hand-authored art — `standard`, `block`, `dark`, `medium`, `light`,
`dots`, `stars`, `plus`, `at`, `small`, `mini`, `banner`, `outline` — plus roughly
a thousand programmatically generated variants of those three base shapes. Five of
those names (`Standard`, `Small`, `Mini`, `Banner`, `Block`) collide with upstream
font names while the glyphs differ completely.

So the harness measures the two things separately:

- **The ported engine** — the actual porting work — is measured over twenty real
  upstream FIGfonts copied into `fonts/`, which *both* runners read as bytes:
  upstream through `figlet.parseFont`/`loadFontSync`, the port through
  `figlet.LoadFontFile`. Same font file, same input, exact comparison of the
  rendered art. The twenty were chosen to span the format: heights 1 to 11,
  hardblanks `$` and `0x7f`, headers with and without the extended `fullLayout`
  field, every horizontal layout a font can declare, fonts with vertical rules
  (`Standard` 24463, `Bubble` 10127, `Slant` 18319, `Mini` 1920, `Shadow` 4992),
  a right-to-left font (`Ivrit`, `printDirection 1`), CRLF-terminated files
  (`Bubble`, `Digital`, `Term`), and fonts with code-tagged glyph tables
  (`Standard` 229 tags, `Term`/`Bubble` 242).
- **The name collision** is measured too, in the `registry` group, and recorded
  as a deviation rather than a bug: the port's own art under an upstream name is
  a deliberate design choice, documented in `chalk/API-DEVIATIONS.md`.

Fonts tested (all 20 present on both sides as identical files): `3x5`,
`ANSI Shadow`, `Banner`, `Big`, `Block`, `Bubble`, `Dancing Font`, `Digital`,
`Doom`, `Graffiti`, `Isometric1`, `Ivrit`, `Mini`, `Ogre`, `Puffy`, `Shadow`,
`Slant`, `Small`, `Standard`, `Term`.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `figlet.textSync(txt, options)` | `figlet.Font.Render` | match | `font-*`, `hlayout-*`, `vlayout-*`, `width-*`, `text-*`, `hardblank-*`, `direction-*` (155 cases) | the whole rendering engine |
| `figlet.text(txt, options)` | — | missing | — | the async form of `textSync`; Go has no promises and the port exposes only the synchronous call. Same code path upstream, so nothing is left unmeasured |
| `figlet.parseFont(name, data)` | `figlet.ParseFont` / `figlet.LoadFont` | match | `err-parse-*`, `ok-parse-*`, `font-by-file` | header validation, glyph table, code tags |
| `figlet.loadFontSync(name)` | `figlet.LoadFontFile` | match | `meta-*`, `err-unknown-font*` | resolving a name to a file on disk |
| `figlet.loadFont(name)` | — | missing | — | async form of `loadFontSync` |
| `figlet.metadata(name)` | `figlet.Font.Metadata` + `figlet.Font.FittingRules` | match | `meta-*` (20 cases) | header fields and the resolved fitting rules |
| `figlet.fontsSync()` | `os.ReadDir` + `figlet.LoadFontDir` | match | `meta-font-list` | the FIGfonts in a directory |
| `figlet.fonts(cb)` | — | missing | — | async form of `fontsSync` |
| `figlet.defaults({font, fontPath})` | — | missing | — | there is no ambient default font path; the port takes an explicit path per call (`LoadFontFile`, `LoadFontDir`) |
| `figlet.figFonts` | `figlet.Fonts` / `figlet.GetFont` / `figlet.Register` | differs | `registry-*` | both are a name→font registry, but the port pre-populates it with its own art rather than with upstream's `.flf` files |
| `figlet.loadedFonts()` | `figlet.Fonts()` | differs | `registry-*` | upstream lists only what has been loaded; the port's registry is never empty |
| `figlet.clearLoadedFonts()` | — | missing | — | the port has no unregister call |
| `figlet.preloadFonts(names)` | `figlet.LoadFontDir` | differs | — | untested: upstream preloads by name list, the port by directory |

### Option keys of `textSync` / `text`

| upstream option | Go field | status | cases |
| --- | --- | --- | --- |
| `font` | the `*Font` receiver / `figlet.RenderFont` name | match | `font-*`, `registry-*` |
| `horizontalLayout` (`default`, `full`, `fitted`, `controlled smushing`, `universal smushing`) | `Options.Layout` (`LayoutDefault`, `LayoutFull`, `LayoutFitted`, `LayoutControlledSmush`, `LayoutUniversalSmush`) | match | `hlayout-*` (31) |
| `verticalLayout` (same five values) | `Options.VerticalLayout` | match | `vlayout-*` (28) |
| `width` | `Options.Width` | match | `width-*` (24) |
| `whitespaceBreak` | `Options.WhitespaceBreak` | match | `width-*-words` |
| `showHardBlanks` | `Options.ShowHardBlanks` | match | `hardblank-*` (8) |
| `printDirection` | `Options.PrintDirection` (`*int`, nil = the font's own) | match | `direction-*` (7) |

### Go-only surface

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `figlet.Render`, `figlet.RenderFont`, `figlet.BuiltinFont`, `figlet.FontFromGlyphs`, `figlet.Register`, `figlet.Fonts`, `figlet.GetFont` | extra | `registry-*` | the bundled-font registry; no upstream counterpart because upstream has no built-in art |
| `figlet.Rainbow`, `figlet.Gradient`, `figlet.RenderRainbow`, `figlet.RenderGradient` | extra | — | untested: colorizing a finished banner with the sibling chalk package. Upstream figlet emits no colour at all, so there is no oracle |
| `figlet.MaxFontHeight`, `figlet.MaxFontBytes` | extra | — | resource caps on parsing; upstream has none. See `chalk/API-DEVIATIONS.md` |
| `figlet.Font.Height` | extra | `meta-*` | a narrower accessor than `Metadata` |
| `figlet.LoadFontDir` | extra | `meta-font-list` | closest to `fontsSync` + `preloadFonts` |

## Counts

| status | symbols |
| --- | --- |
| match | 13 (6 entry points + 7 option keys) |
| differs | 3 |
| missing | 5 |
| extra | 5 |
| untested | 2 (`preloadFonts`; the colour helpers) |

**Parity over the symbols actually compared: 13 / 16 = 81.3 %.** The three
`differs` rows are all the same underlying fact — the port's font registry holds
its own art — and are recorded as deviations, not bugs.

**Case-level parity: see `parity.json`.** Every one of the five `missing` rows is
an async wrapper around a synchronous call that *is* compared, or an ambient
default-path setter, so no upstream behaviour is left unmeasured by them.

## Behaviour differences that do not change a value

Recorded here rather than as mismatches, because the harness compares *whether*
a call failed and not the message text:

- Error messages differ throughout. Upstream says
  `FIGlet header contains invalid values.`; the port says
  `figlet: header contains invalid values`.
- Upstream's `verticallySmushLines` concatenates the JavaScript boolean `false`
  into the row when controlled vertical smushing finds no applicable rule. The
  path is unreachable (the overlap distance is only ever set to a validated
  value), and the port falls back to universal smushing there instead.
- An unrecognised `horizontalLayout`/`verticalLayout` string is silently ignored
  by both sides, leaving the font's own rules in force (`hlayout-unknown-name`).
- `textSync(txt, {font: ""})` falls back to upstream's default font `Standard`;
  the port has no ambient default, so an empty font name is simply a font it
  cannot find. Not compared.
