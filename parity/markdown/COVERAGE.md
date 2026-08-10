# `markdown` parity coverage

| | |
| --- | --- |
| Go module under test | `github.com/malcolmston/markdown v0.1.0` (published module, no `replace`) |
| Upstream implementation oracle | `commonmark@0.31.2` (npm, `parity/markdown/node/`) |
| Upstream specification oracle | CommonMark **0.31.2** `spec.json`, 652 examples, vendored at `cases/spec.json` |
| Cases | 701 (652 generated from `spec.json`, 49 hand-written) |
| **CommonMark 0.31.2 conformance (Go port vs `spec.json`)** | **652 / 652 = 100.00 %** |
| Harness cross-check (`commonmark.js` vs `spec.json`) | 652 / 652 = 100.00 % |
| Parity (Go port vs `commonmark.js`, all 701 cases) | 683 / 701 = 97.43 % |

Reproduce with:

```sh
GOWORK=off go test ./parity/markdown/          # runs both runners, rewrites parity.json
node parity/markdown/gen-cases.mjs             # regenerates cases/spec-*.json from cases/spec.json
```

## Why this library has two oracles

Every other directory in `parity/` has exactly one oracle: the upstream library.
CommonMark ships something better — `spec.json`, the specification's own
machine-readable conformance suite of `(markdown, html, section, example)`
tuples, fetched verbatim from <https://spec.commonmark.org/0.31.2/spec.json>.
The expected HTML is therefore still not hand-written; it is normative.

Every generated case is compared **three ways**:

| comparison | what it means | how a failure is reported |
| --- | --- | --- |
| Go vs `spec.json` | **conformance** — the headline number | `t.Errorf` |
| Go vs `commonmark.js` | parity, the number the rest of this harness reports | `t.Errorf` |
| `commonmark.js` vs `spec.json` | a self-check that the harness is wired up correctly | `t.Logf` with `HARNESS WARNING`, never a failure — a difference here indicts the harness or the reference implementation, not the port |

The cross-check reads 652/652, so the plumbing (JSON-Lines transport, argument
encoding, normalisation) is confirmed not to be losing or mangling anything.

## HTML normalisation — exactly what is and is not done

`normaliseHTML` in `parity_test.go` is applied **identically** to the spec HTML,
the upstream HTML and the Go HTML, and does exactly two things:

1. `\r\n` and lone `\r` become `\n` — so a checkout with CRLF line endings in
   `cases/spec.json` cannot invent differences.
2. A single trailing `\n` is removed.

Nothing else. Specifically **not** normalised, because in HTML these are exactly
where real CommonMark rendering bugs live:

- interior whitespace and newlines between block elements (tight vs loose lists,
  `<br />` placement, blank lines inside `<pre>`);
- indentation;
- attribute order and attribute quoting;
- the text inside `<pre><code>` (tab expansion, trailing newlines);
- entity vs literal character choices (`&amp;` vs `&`, `&quot;` vs `"`).

Measured claim: on this module version the normalisation is **the identity
function** for all 652 spec examples on both sides — the raw bytes already
match. It is there to stop a future line-ending accident, not to buy the score.

## Per-spec-section conformance

Derived mechanically: the `section` field of each `spec.json` entry becomes the
case-file `group`, and the harness scores each group separately into
`parity.json` (`groups.<section>.specMatch / specCases`). Sections are listed in
spec order.

| spec section | Go port vs spec | % | `commonmark.js` vs spec (self-check failures) |
| --- | --- | --- | --- |
| `Tabs` | 11/11 | 100% | 0 |
| `Backslash escapes` | 13/13 | 100% | 0 |
| `Entity and numeric character references` | 17/17 | 100% | 0 |
| `Precedence` | 1/1 | 100% | 0 |
| `Thematic breaks` | 19/19 | 100% | 0 |
| `ATX headings` | 18/18 | 100% | 0 |
| `Setext headings` | 27/27 | 100% | 0 |
| `Indented code blocks` | 12/12 | 100% | 0 |
| `Fenced code blocks` | 29/29 | 100% | 0 |
| `HTML blocks` | 44/44 | 100% | 0 |
| `Link reference definitions` | 27/27 | 100% | 0 |
| `Paragraphs` | 8/8 | 100% | 0 |
| `Blank lines` | 1/1 | 100% | 0 |
| `Block quotes` | 25/25 | 100% | 0 |
| `List items` | 48/48 | 100% | 0 |
| `Lists` | 26/26 | 100% | 0 |
| `Inlines` | 1/1 | 100% | 0 |
| `Code spans` | 22/22 | 100% | 0 |
| `Emphasis and strong emphasis` | 132/132 | 100% | 0 |
| `Links` | 90/90 | 100% | 0 |
| `Images` | 22/22 | 100% | 0 |
| `Autolinks` | 19/19 | 100% | 0 |
| `Raw HTML` | 20/20 | 100% | 0 |
| `Hard line breaks` | 15/15 | 100% | 0 |
| `Soft line breaks` | 2/2 | 100% | 0 |
| `Textual content` | 3/3 | 100% | 0 |
| **total** | **652/652** | **100%** | **0** |

There is no worst-performing section: the port renders every example in the
CommonMark 0.31.2 suite byte for byte. The 18 remaining failures all come from
the three hand-written groups below, which ask questions `spec.json` does not.

### Not covered by `spec.json` at all

`spec.json` is **pure CommonMark**. It contains no GFM extensions, so the port's
complete absence of tables, strikethrough, task lists, autolink literals and
footnotes costs it nothing here and is not visible in the table above. Stated
plainly: `github.com/malcolmston/markdown v0.1.0` implements **no GFM**. There
is no `Options` field, node type or renderer hook for any of it, and no
extension mechanism to add one, so a caller needing GFM cannot get there from
the public API. `commonmark.js` has no GFM either, so this is not a parity gap —
it is a capability gap against GitHub-flavoured expectations.

## Security findings — the most important result here

Case group `security` (19 cases, `cases/security.json`). This is the part of the
port's behaviour that a conformance score cannot see.

### 1. No link sanitising whatsoever, and no way to turn any on

`DefaultOptions()` is `Options{HTML: true, Typographer: false, LineBreaks: false,
LinkTarget: ""}` — verified by case `opt-default-options`, which reads the real
struct. There is no allowlist of URL schemes anywhere in the exported API. Every
one of these produces a live, clickable, script-executing href:

| input | `markdown.Render` output |
| --- | --- |
| `[x](javascript:alert(1))` | `<p><a href="javascript:alert(1)">x</a></p>` |
| `[x](JaVaScRiPt:alert(1))` | `<p><a href="JaVaScRiPt:alert(1)">x</a></p>` |
| `[x](java&#115;cript:alert(1))` | `<p><a href="javascript:alert(1)">x</a></p>` (the character reference is decoded first) |
| `[x](vbscript:msgbox(1))` | `<p><a href="vbscript:msgbox(1)">x</a></p>` |
| `[x](data:text/html;base64,…)` | `<p><a href="data:text/html;base64,…">x</a></p>` |
| `![i](javascript:alert(1))` | `<p><img src="javascript:alert(1)" alt="i" /></p>` |
| `[ref]` + `[ref]: javascript:alert(1)` | `<p><a href="javascript:alert(1)">ref</a></p>` |

`commonmark.js` with its default renderer does **exactly the same thing**, so
these 11 cases are at parity — being unsafe by default is inherited from the
reference implementation, not invented by the port.

The difference is what happens next. `commonmark.js` offers
`new HtmlRenderer({safe: true})`, which blanks unsafe destinations
(`<p><a>x</a></p>`, `<img src="" alt="i" />`), replaces raw HTML with
`<!-- raw HTML omitted -->`, and leaves benign links untouched
(`<a href="https://example.com/a?b=1&amp;c=2">`). The Go port has **no
equivalent** — no `Options` field, no renderer hook, no exported node rewriting
step. All 7 `render_safe` cases are therefore scored as real mismatches, not
deviations: `sec-safe-js-scheme-link`, `sec-safe-vbscript-scheme-link`,
`sec-safe-data-scheme-link`, `sec-safe-js-scheme-image`,
`sec-safe-raw-script-block`, `sec-safe-raw-inline-event-handler`,
`sec-safe-http-link-untouched`.

**Consequence:** rendering untrusted Markdown with this module requires an
external HTML sanitiser on the output. There is no in-library configuration that
makes it safe.

### 2. `Options{HTML: false}` is not a sanitiser

It is the closest thing the port has to a safety switch, and it does not do the
job. It escapes raw HTML (`&lt;script&gt;…`, case `opt-nohtml-block` /
`opt-nohtml-inline`) but **leaves link destinations completely untouched**:
`opt-nohtml-does-not-touch-links` shows `[x](javascript:alert(1))` still
rendering `<a href="javascript:alert(1)">`. Anyone reaching for `HTML: false`
expecting safety gets XSS through links.

### 3. Tabs in a link destination: the port is more permissive than `commonmark.js`

`sec-js-scheme-tab-obfuscated` — the only *rendering* difference found outside
the option groups:

| | `[x](\tjavascript:alert(1))` |
| --- | --- |
| Go port | `<p><a href="javascript:alert(1)">x</a></p>` — a live link |
| `commonmark.js` | `<p>[x](\tjavascript:alert(1))</p>` — literal text, no link |

The same holds for a trailing tab (`[x](/url\t)`): the port links, upstream does
not. A leading *space* or *newline* is accepted by both. `spec.json` contains no
example for a tab in that position, so the vendored oracle cannot adjudicate it,
and this is recorded as an observed behavioural difference rather than a
conformance failure either way. It matters operationally: any filter that
decides "does this Markdown contain a link?" by running `commonmark.js` will
disagree with the Go renderer on this input.

## Upstream API inventory

Derived mechanically, not from the README. The exact command, run against the
installed `node_modules/commonmark`:

```sh
node -e "import('commonmark').then(cm=>{for(const k of Object.keys(cm).sort()){
  console.log('EXPORT',k,typeof cm[k]);
  if(typeof cm[k]==='function')for(const p of Object.getOwnPropertyNames(cm[k].prototype||{}).sort()){
    const d=Object.getOwnPropertyDescriptor(cm[k].prototype,p);
    console.log('  ',k+'.prototype.'+p,d.get?'accessor':typeof d.value);}}})"
```

`Object.keys` on the module gives 5 exports: `Parser`, `HtmlRenderer`,
`XmlRenderer`, `Renderer`, `Node`. `Object.getOwnPropertyNames` on each
prototype gives the members below. Instance-only own properties
(`Object.getOwnPropertyNames(new cm.Parser())` etc.) are internal parser state
and `options`, and are not public API.

Status legend: `match` (compared and equal), `differs` (compared and unequal),
`missing` (upstream has it, port does not), `extra` (port has it, upstream does
not), `untested` (no case).

### Module exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Parser` (constructor) | `markdown.New` / `markdown.Parse` | match | `api-ast-*`, `api-node-*` | |
| `new Parser({smart})` | `Options.Typographer` | match | `opt-smart-quotes`, `opt-smart-dashes-ellipsis`, `opt-smart-apostrophe`, `opt-smart-nested-quotes`, `opt-smart-in-code-span` | all 5 agree byte for byte |
| `Parser#parse` | `markdown.Parse`, `(*Markdown).Parse` | match | `api-ast-headings-and-text`, `api-ast-lists`, `api-ast-code-blocks`, `api-ast-links-and-images`, `api-ast-blockquote-and-breaks`, `api-ast-raw-html`, `api-ast-code-span`, `api-ast-empty` | AST field subset, see below |
| `HtmlRenderer` (constructor) | `markdown.New` | match | all 652 spec cases | |
| `HtmlRenderer#render` | `markdown.Render`, `(*Markdown).Render` | match | all 652 spec cases + 12 `sec-*` | |
| `new HtmlRenderer({safe})` | — | missing | `sec-safe-*` (7) | **no sanitising mode in the port at all** |
| `new HtmlRenderer({softbreak})` | `Options.LineBreaks` | differs | `opt-softbreak-br`, `opt-softbreak-multi-line`, `opt-softbreak-in-heading` | port emits `<br />\n`, upstream emits `<br />` (markdown-it vs cmark style). Not listed in the module's `API-DEVIATIONS.md`, so scored as a mismatch |
| `new HtmlRenderer({softbreak})` on a hard break | `Options.LineBreaks` | match | `opt-softbreak-hardbreak-still-br` | |
| `new HtmlRenderer({sourcepos})` | — | missing | — | untested: the port records no source positions, so there is nothing to compare |
| `new HtmlRenderer({esc})` custom escaper | — | missing | — | untested: no renderer hooks in the port |
| `XmlRenderer` (+ `#render`, `#cr`, `#esc`, `#out`, `#tag`) | — | missing | — | port renders HTML only |
| `Renderer` (base class: `#render`, `#cr`, `#esc`, `#lit`, `#out`) | — | missing | — | the port's renderer is unexported and not subclassable |
| `HtmlRenderer#` per-node methods (`text`, `softbreak`, `linebreak`, `link`, `image`, `emph`, `strong`, `paragraph`, `heading`, `code`, `code_block`, `thematic_break`, `block_quote`, `list`, `item`, `html_block`, `html_inline`, `custom_block`, `custom_inline`, `attrs`, `esc`, `out`, `tag`) | — | missing | — | 23 overridable render hooks; the port exposes none. Their *effects* are covered by the 652 spec cases |
| `Node` (constructor) | `markdown.Node` struct literal | match | `api-append-child` | |

### `Node` members

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Node#type` | `Node.Type` + `NodeType.String` | match | `api-ast-*`, `api-node-types-all-constructs` | all 18 type names are spelled identically, including `softbreak`/`linebreak` |
| `Node#literal` | `Node.Literal` | match | `api-ast-code-blocks`, `api-ast-raw-html`, `api-ast-code-span` | compared for `text`, `code`, `code_block`, `html_block`, `html_inline` only |
| `Node#level` | `Node.Level` | match | `api-ast-headings-and-text` | compared on `heading` only |
| `Node#destination` | `Node.Destination` | match | `api-ast-links-and-images` | |
| `Node#title` | `Node.Title` | match | `api-ast-links-and-images` | |
| `Node#info` | `Node.Info` | match | `api-ast-code-blocks` | compared on `code_block` only |
| `Node#listType` | `Node.Info` on a `list` | match | `api-ast-lists` | `"bullet"` / `"ordered"` in both |
| `Node#listTight` | `Node.Tight` | match | `api-ast-lists` | |
| `Node#listStart` | `Node.Level` on a `list` | match | `api-ast-lists` | |
| `Node#listDelimiter` | `Node.Literal` on a `list` | differs | — | untested by design: upstream exposes only `.` / `)`, the port also stores the bullet character `-`/`*`/`+` and copies both onto every `item`. Masked out of the AST comparison |
| `Node#firstChild` | `(*Node).FirstChild` | match | `api-node-nav-document`, `api-node-nav-empty` | |
| `Node#lastChild` | `(*Node).LastChild` | match | `api-node-nav-document`, `api-node-nav-empty` | |
| `Node#parent` | `(*Node).Parent` | match | `api-node-nav-document`, `api-node-nav-empty` | both report `null`/`nil` for the document root |
| `Node#appendChild` | `(*Node).AppendChild` | match | `api-append-child` | |
| `Node#walker` | `(*Node).Walk` | match | `api-node-types-all-constructs`, `api-node-nav-document` | node counts and type sets agree |
| `Node#next` | `Node.Children` slice | match | every `api-ast-*` case | sibling links vs a slice; the traversal order is what is compared |
| `Node#prev` | — | missing | — | untested: no backward sibling link |
| `Node#insertAfter` | — | missing | — | port has no sibling insertion |
| `Node#insertBefore` | — | missing | — | port has no sibling insertion |
| `Node#prependChild` | — | missing | — | |
| `Node#unlink` | — | missing | — | no way to detach a node |
| `Node#isContainer` | `NodeType.IsBlock` | differs | `api-block-types` | different questions: `isContainer` is per-node ("may have children", true for `link`/`emph`), `IsBlock` is per-type ("block-level"). No oracle, so this is scored as a mismatch and is really *extra* |
| `Node#sourcepos` | — | missing | — | the port records no source positions |
| `Node#onEnter` / `#onExit` | — | missing | — | custom-block hooks; the port has no custom node types |

### Go-only surface (`extra`)

Derived from `GOWORK=off go doc -all github.com/malcolmston/markdown`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `markdown.RenderTo` | extra | `api-render-to-matches-render`, `api-render-to-write-error` | output identical to upstream `render()`; the error path wraps `ErrWrite`, `ErrMarkdown` and the writer's own error, all three verified with `errors.Is` |
| — | `(*Markdown).RenderTo` | extra | — | untested: same code path as `markdown.RenderTo` |
| — | `markdown.DefaultOptions` | extra | `opt-default-options` | no upstream accessor; the case exists to pin the shipped default `{HTML:true}` |
| — | `Options.HTML` | extra | `opt-nohtml-block`, `opt-nohtml-inline`, `opt-nohtml-does-not-touch-links` | no upstream counterpart (`safe:true` also rewrites links); escapes rather than omits raw HTML |
| — | `Options.LinkTarget` | extra | `opt-linktarget-blank`, `opt-linktarget-autolink` | no upstream counterpart |
| — | `markdown.ErrMarkdown` | extra | `api-render-to-write-error` | |
| — | `markdown.ErrWrite` | extra | `api-render-to-write-error` | |
| — | `NodeType` + its 18 constants | extra | `api-node-types-all-constructs` | upstream uses bare strings |
| — | `NodeType.String` | extra | `api-node-types-all-constructs`, every `api-ast-*` | names match upstream's strings exactly |
| — | `NodeType.IsBlock` | extra | `api-block-types` | no upstream equivalent |
| — | `Node.Tight`, `Node.Info`, `Node.Level` reuse for list metadata | extra | `api-ast-lists` | documented in the module's `API-DEVIATIONS.md` §1 |

## Counts

Upstream symbols enumerated: **62** (5 module exports + 8 constructor-option
keys/behaviours + 23 `HtmlRenderer` render hooks + 26 `Node` members, counting
`Node`'s constructor once).

| status | count |
| --- | --- |
| `match` | 22 |
| `differs` | 3 |
| `missing` | 33 |
| `untested` | 4 |
| `extra` (Go-only, listed separately) | 11 |

Parity over upstream symbols actually compared (`match` + `differs`):
**22 / 25 = 88.0 %**.

The `missing` count is dominated by two things that are not rendering
behaviour: the 23 overridable `HtmlRenderer` per-node hooks and the
`XmlRenderer`/`Renderer` classes. Their *output* is fully covered by the 652
spec cases; what the port lacks is the extensibility, not the behaviour.

## Everything the port lacks (summary)

1. **No sanitising mode.** No scheme allowlist, no raw-HTML stripping. Untrusted
   input requires an external HTML sanitiser. `commonmark.js` has `safe: true`.
2. **No GFM.** No tables, strikethrough, task lists, autolink literals or
   footnotes, and no extension point to add them.
3. **No renderer extensibility.** The renderer is unexported; none of upstream's
   23 per-node hooks, `Renderer` subclassing, or custom `esc` is available.
4. **No XML/AST serialiser** (upstream `XmlRenderer`).
5. **No source positions** (upstream `Node#sourcepos`, `{sourcepos: true}`).
6. **No AST mutation beyond append**: no `unlink`, `insertBefore`,
   `insertAfter`, `prependChild`, and no `prev` sibling link.
7. **`LineBreaks` emits `<br />\n`** where `commonmark.js`'s `softbreak` option
   emits `<br />` — undocumented in the module's `API-DEVIATIONS.md`.
8. **Tabs are accepted as padding inside `(...)` link destinations** where
   `commonmark.js` rejects them; `spec.json` has no example either way.

## Case files

| file | group | cases | source |
| --- | --- | --- | --- |
| `cases/spec.json` | — | — | verbatim CommonMark 0.31.2 conformance suite; **not** a case file, read only by the generator and by the harness's corpus assertion |
| `cases/spec-01-tabs.json` … `cases/spec-26-textual-content.json` | the 26 spec section names | 652 | generated by `gen-cases.mjs` from `cases/spec.json`; ids are the spec's own example numbers (`ex-001`…`ex-652`) and each case carries `specHTML` and `specLines` |
| `cases/security.json` | `security` | 19 | hand-written hostile inputs; the specification says nothing about sanitising |
| `cases/options.json` | `options` | 15 | hand-written; the port's `Options` fields mapped onto the nearest `commonmark.js` option |
| `cases/api.json` | `api` | 15 | hand-written; the exported API that rendering HTML does not reach (`Parse`, `Node`, `NodeType`, `RenderTo`, `ErrWrite`) |

## Determinism

Neither runner reads a clock, a random source, an environment variable or a
locale. Both construct every parser/renderer from an explicit option set (no TTY
or `process.env` sniffing). Map iteration is sorted before emission
(`node_types`, `block_types`). Both runners catch every throw/panic into
`{"ok": false, "error": …}` and keep reading, so no single case can end a run;
the harness additionally enforces a 20 s per-case timeout.
