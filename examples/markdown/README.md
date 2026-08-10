# markdown example

A single runnable program that exercises `github.com/malcolmston/markdown` — a
CommonMark 0.31.2 parser and HTML renderer — over a document that uses every
block and inline construct, plus a conformance spot-check table, AST walking,
options, escaping probes and edge cases.

The library is consumed as a **published Go module**: there is no `replace`
directive and no reference to the local checkout.

Resolved module version (the repo has no semver tags, so `@latest` yields a
pseudo-version):

```
github.com/malcolmston/markdown v0.0.0-20260725030040-36a3ae1bcadc
```

```sh
GOWORK=off go get github.com/malcolmston/markdown@latest
# go: downloading github.com/malcolmston/markdown v0.0.0-20260725030040-36a3ae1bcadc
```

> **Important:** the published module is *older and smaller* than the local
> `../../markdown` working tree. The entire link-sanitizing layer
> (`ValidateLink`, `Options.ValidateLink`, `CommonMarkOptions`, `security.go`)
> exists only in the local checkout and is **not** in the published module, and
> `DefaultOptions()` means the opposite thing in each (`HTML: true` published vs
> `HTML: false` locally). This example codes against the published API only.
> See hole 1.

## Run

```sh
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Eight labelled sections are printed and the program exits on its own.

## What it demonstrates

1. **Full-document render** with the package defaults: ATX and setext headings,
   emphasis/strong (including nesting and the intraword-underscore rule), tight
   nested bullet lists, a loose ordered list with `start="3"`, inline/reference/
   collapsed/shortcut links, autolinks and email autolinks, images with titles,
   inline code, fenced code with an info string, indented code, nested block
   quotes containing a list, a thematic break, entities, backslash escapes and a
   hard break.
2. **`Options{HTML:false}` vs the package default** — how raw HTML is escaped
   versus passed through.
3. **32 conformance spot-checks** against the expected CommonMark 0.31.2 output,
   compared byte for byte. All 32 pass, including tab expansion, `&#0;` and
   out-of-range numeric references collapsing to U+FFFD, the 8-digit
   non-reference case, and `&copy` without a semicolon.
4. **Escaping and link destinations** — raw HTML in both modes, a 13-case table
   of hostile destinations with each result flagged safe/UNSAFE, and
   attribute-value escaping for titles, `alt` text and the info-string class.
5. **Options** — `Typographer`, `LineBreaks`, `LinkTarget` (including that
   `LinkTarget` is correctly escaped when it contains a quote).
6. **AST walking** — `Parse`, `Node.Walk` (including subtree pruning by
   returning `false`), `FirstChild`/`LastChild`/`Parent`, a per-`NodeType`
   histogram with `IsBlock`, the list metadata encoding
   (`Info`/`Literal`/`Level`/`Tight`), and `AppendChild`.
7. **`RenderTo`** — streaming to an `io.Writer` and the error path, verifying
   that the returned error satisfies `errors.Is` for `ErrWrite`, `ErrMarkdown`
   *and* the underlying writer error.
8. **Edge cases** — empty input, whitespace-only, CRLF, no trailing newline, a
   NUL byte, an unterminated fence, unclosed emphasis, eight levels of block
   quote nesting, invalid UTF-8, a lone surrogate reference.

## Holes found

Nothing panicked, and the CommonMark output was correct in every case I checked
(32/32 spot-checks byte-exact). The problems are all about the published
module's *surface*.

### 1. The published module has no link sanitizing at all — `javascript:` URLs render verbatim (security)

There is no `ValidateLink`, no `Options.ValidateLink`, and no `security.go` in
`v0.0.0-20260725030040-36a3ae1bcadc`. Section 4 of the example shows the
consequences; these are actual outputs from the published module:

```
[x](javascript:alert(1))                  -> <p><a href="javascript:alert(1)">x</a></p>
[x](JaVaScRiPt:alert(1))                  -> <p><a href="JaVaScRiPt:alert(1)">x</a></p>
[x](&#x6a;avascript:alert&lpar;1&rpar;)    -> <p><a href="javascript:alert(1)">x</a></p>
[x](vbscript:x)                           -> <p><a href="vbscript:x">x</a></p>
<javascript:alert(1)>                     -> <p><a href="javascript:alert(1)">javascript:alert(1)</a></p>
![x](data:text/html,<script>…)            -> <img src="data:text/html,%3Cscript%3E…" alt="x" />
```

Note that the character-reference form is *decoded into* a live
`javascript:` URL, so even a caller who tries to pre-filter the source string
cannot defend itself. `Parse` also leaves `Destination` unsanitized, so walking
the AST does not help. There is **no supported way** to reject a scheme: the
only escape hatch would be to post-process the HTML with an external sanitizer.

This is the highest-severity finding, and it is made worse by hole 2.

### 2. Documentation gaps and one README-vs-API mismatch

- The published `README.md` and `doc.go` contain **no security section at all** —
  no mention of `javascript:`, no warning that `Render` is unsafe for untrusted
  input, and no guidance to sanitize downstream. For an HTML renderer whose
  package default is `HTML: true` (raw HTML, including `<script>`, passed
  through verbatim), the omission is itself the problem: nothing tells a caller
  that `markdown.Render(userInput)` is an XSS vector. The README's
  "**Correct escaping** of text, attribute values, and URL destinations" bullet
  actively suggests otherwise; it refers only to percent-encoding, not to scheme
  filtering.
- **`<br>` vs `<br />`**: the README documents `LineBreaks` as producing `<br>`
  ("a newline inside a paragraph becomes `<br>`", and the sample output shows
  `<p>line one<br>`). The renderer emits `<br />`, as the example's section 5
  shows. The renderer is right (CommonMark's reference output uses `<br />`);
  the README is wrong.
- The local working tree's `README.md` and `API-DEVIATIONS.md` describe a
  `ValidateLink` / `Options.ValidateLink` / `CommonMarkOptions()` API and a
  `DefaultOptions()` of `Options{HTML: false}`. None of that exists in the
  published module, so anyone reading the repo on GitHub's default branch and
  then `go get`-ting the module writes code that does not compile — and believes
  the renderer defends against `javascript:` when the published one does not.
  Publishing a tagged release would fix this; today `@latest` is a
  pseudo-version pointing at a stale commit.

### 3. No exported way to render a `*Node`

`Parse`, `AppendChild`, `Walk` and the whole AST are exported, but the renderer
(`htmlRenderer`, `render`) is not, and `Render`/`RenderTo` only accept a source
*string*. So the AST is read-only in practice: you can inspect a document but you
cannot transform it and render the result, and a tree built with `AppendChild`
(the example builds one) can never be turned into HTML. That makes `AppendChild`
and the mutable exported `Node` fields close to useless. A `RenderNode(*Node)`
or `(*Markdown).RenderNode` is the obvious missing entry point. Marked
`// HOLE:` in `main.go`.

This directly contradicts the published README, which offers the AST as the
substitute for a plugin system — "`markdown-it`'s rule chain, `md.use(...)`, and
custom renderer-rule overrides have no equivalent; the AST from `Parse` is the
extension point instead". It cannot be an extension point while it is
render-only-in-one-direction: you can read the tree but you can never render a
modified one.

### 4. No GFM extensions

Tables, strikethrough, task lists, footnotes, autolink-literals and heading
anchors/IDs are all absent. This is defensible — the package advertises
CommonMark, not GFM — but it means the library cannot render a typical README,
and a pipe table degrades silently into a paragraph of literal `|` characters
(the example checks that this is the CommonMark-correct behaviour, and it is).
The published README does disclose this under "No GFM or markdown-it
extensions". There is also no plugin or renderer-rule hook, so — combined with
hole 3 — an application cannot add tables itself either.

### 5. Minor / non-idiomatic

- `Options` has no `Extensions` or `Renderer` field of any kind, so the only
  configuration is four booleans/strings; combined with hole 3 there is no
  extension point at all.
- `Node`'s list metadata is smuggled through `Info` (`"bullet"`/`"ordered"`),
  `Literal` (the bullet character) and `Level` (the ordered-list start number).
  `Level` therefore means "heading level" on one node type and "list start
  number" on another, which is easy to misuse and needs the deviations doc to
  decode.
- `Node` exposes ~20 unexported parser-bookkeeping fields alongside 8 exported
  ones, so `&markdown.Node{...}` composite literals are legal but produce trees
  the renderer was never designed to accept.
- `Walk`'s callback returns `bool` to mean "descend into children", which is not
  obvious from the name; there is no entering/exiting distinction, so a walker
  cannot emit a closing tag.
- There is no `Version()` or exported spec-version constant, even though a
  `VERSION` file and a 652/652 parity claim ship in the repo.
