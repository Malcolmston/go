# `mime-types` — API coverage

Nested harness: the oracle is the npm **`mime-types`** package, not express.
Upstream pinned at `mime-types@2.1.35` in [`node/package.json`](node/package.json)
— the version express@4.21.2 resolves, so this score is consistent with the
top-level express harness. Go port is
`github.com/malcolmston/express/mimetypes` in the `express` submodule.

## How the upstream inventory was derived

```console
$ cd node && node -e "const m=require('mime-types'); \
    console.log(typeof m); console.log(Object.keys(m).join(', '))"
object
charset, charsets, contentType, extension, extensions, lookup, types
```

The type data itself was enumerated from `mime-db`, which is what `mime-types`
is a view over:

```console
$ cd node && node -e "const db=require('mime-db'); \
    console.log(Object.entries(db).filter(([,v])=>v.charset).length)"
39
```

The Go side is `GOWORK=off go doc -short ./mimetypes` in the submodule:

```
func Charset(mimeType string) (string, bool)
func ContentType(typeOrExt string) (string, bool)
func Extension(mimeType string) (string, bool)
func Lookup(pathOrExt string) (string, bool)
```

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `mime.lookup(path)` | `mimetypes.Lookup(pathOrExt)` | match | 83 in `lookup` | paths, filenames, bare and dotted extensions, dotfiles, Windows-looking paths, query strings, unknowns |
| `mime.charset(type)` | `mimetypes.Charset(mimeType)` | match | 25 in `charset` | the declared-charset table plus the `text/*` fallback |
| `mime.contentType(str)` | `mimetypes.ContentType(typeOrExt)` | match | 22 in `content-type` | |
| `mime.extension(type)` | `mimetypes.Extension(mimeType)` | match | 58 in `extension` | |
| `mime.types` (ext → type map) | — | missing | — | the port keeps the same data in an unexported map; every entry is exercised indirectly through `Lookup` |
| `mime.extensions` (type → ext[] map) | — | missing | — | the port stores only the first extension per type (which is all `extension()` reports), so the full list is not available |
| `mime.charsets.lookup` | — | missing | — | a deprecated alias of `charset` in upstream itself |

## Scope of the data table

The port ships a **subset** of `mime-db` — 65 extensions and 54 types — not the
full ~1000-type database, and that is deliberate (see the package doc). What the
harness pins is that every entry the subset *does* contain resolves exactly as
`mime-types@2.1.35` resolves it, in both directions, and that unknown inputs
report unknown rather than guessing. All 83 `lookup` cases and all 58 `extension`
cases are drawn from that subset plus deliberate misses.

## Representation mapping

Upstream returns `false` where the Go port returns `("", false)`; both runners
emit JSON `null`. Applied symmetrically.

## Counts

| status | symbols |
| --- | --- |
| match | 4 |
| differs | 0 |
| missing | 3 |
| extra | 0 |
| untested | 0 |

Parity over the 4 symbols actually compared: **4/4 = 100%**.

**Cases: 188. Measured parity: 188/188 = 100.0%** (see
[`parity.json`](parity.json)), up from 152/188 = 80.9%.

## What the harness fixed in the port

The tables were hand-curated and had drifted from `mime-db`'s actual rankings;
they are now generated from the installed `mime-types@2.1.35` itself. 36 closed
mismatches, in three families:

- **forward lookups with the wrong winner.** `mime-db` ranks candidate types per
  extension, and the port had guessed: `.js`/`.mjs` are
  `application/javascript` (not `text/javascript`) at this version, `.ico` is
  `image/vnd.microsoft.icon`, `.wav` is `audio/wave`, `.opus` is `audio/ogg`,
  `.flac` is `audio/x-flac`, `.m4v` is `video/x-m4v`, `.exe` is
  `application/x-msdos-program`, `.dll` is `application/x-msdownload`, and
  `.tgz` is not in `mime-db` at all.
- **reverse lookups.** `extension()` returns the **first** extension `mime-db`
  lists, which is often not the familiar one: `image/jpeg` → `jpeg`,
  `audio/mpeg` → `mpga`, `audio/ogg` → `oga`, `video/quicktime` → `qt`,
  `image/tiff` → `tif`, `text/markdown` → `markdown`. Conversely
  `text/javascript`, `audio/opus` and `audio/flac` have *no* extensions and must
  report nothing.
- **charset rules.** The port applied a `+json`/`+xml` structured-suffix rule
  that upstream does not have, so `application/xml`, `application/ld+json` and
  `image/svg+xml` were all claimed as UTF-8 — which then leaked into
  `contentType()` output as a spurious `; charset=utf-8`. `Charset` is now
  `mime-db`'s 39 declared-charset entries (including the non-UTF-8 ones:
  `US-ASCII` for the news types, `7-BIT` for `application/prs.cyn`) plus the
  blanket `text/*` fallback. `contentType()` also no longer trims its input,
  matching upstream's byte-for-byte passthrough.

The port's own unit tests, example tests and its `mimetypes_parity_test.go`
vectors were updated to the corrected expectations in the same change.
