# `cheerio` parity coverage

- **Upstream oracle:** `cheerio@1.0.0` (pinned in `node/package.json`, installed in `node/node_modules`).
- **Go port:** `github.com/malcolmston/cheerio v0.0.0-20260810111529-e3282199363b`
  (added with `GOWORK=off go get github.com/malcolmston/cheerio@latest`; consumed as a
  published module — there is no `replace` directive in `go.mod`).
- **Harness:** `GOWORK=off go test ./parity/cheerio/` — 317 cases across 7 case files.
- **Case score:** 306 match / 2 differ / 9 declared deviations → **99.35 %** of the
  308 compared cases (see `parity.json`). The 9 deviations are listed in
  `cheerio/API-DEVIATIONS.md`; the 2 remaining differs are the adoption-agency
  parsing gap.
- **Symbol score:** 60 match / 1 differs over 61 compared symbols → **98.36 %**
  (3 symbols are now pure deviations — `serialize`, `serializeArray`, `val`;
  17 untested, 9 missing, 90 upstream symbols total).

## How the upstream inventory was derived

Run from `parity/cheerio/node`:

```sh
node -e "
const c = require('cheerio');
console.log('MODULE:', Object.keys(c).sort().join(' '));
const \$ = c.load('<div/>');
console.log('DOLLAR:', Object.getOwnPropertyNames(\$).sort().join(' '));
let p = \$.fn, out = new Set();
while (p && p !== Object.prototype) { Object.getOwnPropertyNames(p).forEach(n => out.add(n)); p = Object.getPrototypeOf(p); }
console.log('PROTO(' + out.size + '):', [...out].sort().join(' '));
"
```

That yields exactly 7 module exports, 15 own properties on a loaded `$`, and 71 names on
the `Cheerio.prototype` chain (walking `$.fn` and its prototypes; walking
`Object.getPrototypeOf($('div'))` gives the identical 71). 4 module exports are not also
`$` properties, so the unique upstream surface is **90 symbols**.

The Go side was enumerated with:

```sh
GOWORK=off go doc -all github.com/malcolmston/cheerio | grep -E '^func '
```

## Normalisation (what is compared, and what is deliberately not)

1. **Parsing mode.** The Node runner calls `cheerio.load(html, null, false)`, i.e. parse5
   in *fragment* mode. The Go port has no document/fragment switch and never synthesises
   `<html>/<head>/<body>`, so fragment mode is the only apples-to-apples comparison.
   One consequence is visible in the score: parse5's fragment mode **drops the doctype**,
   while the Go port preserves it (`parse-comments-doctype`, `ser-doctype-preserved`).
   In upstream document mode the doctype *is* preserved, so this pair of failures is a
   mode artefact rather than a port bug — recorded here rather than silently normalised.
2. **Attribute order and tag spelling.** Both runners pass every serialised HTML string
   through an identical canonicaliser (`canon()` in `node/run.js` and `go/run.go`): tag and
   attribute names are lower-cased, attributes are sorted lexicographically by name and
   re-emitted as `name="value"` with double quotes, a valueless attribute becomes
   `name=""`, the self-closing `/` is dropped, and `<!...>` declarations are lower-cased.
   Comment bodies and raw-text element bodies (`script`, `style`, `xmp`, `iframe`,
   `noembed`, `noframes`, `plaintext`) are copied verbatim.
   **Attribute *values* are re-emitted byte-for-byte**, so entity-escaping differences are
   still reported (and several are — see below).
3. **Value typing.** cheerio returns rich JS values from `.data()` and `.prop()`; the Go
   port's signatures are string-only. The Node runner therefore stringifies scalars
   (`String()` for number/boolean, `JSON.stringify` for objects) before emitting. This is
   the only place a difference is normalised away, and it only affects representation, not
   semantics.
4. **Absent values.** `undefined` is emitted as JSON `null` on the Node side. The Go port
   returns `("", false)` for absent attributes/data (also emitted as `null`), but
   `Css`/`Val` return a bare `""`; those cases are reported as divergences, not normalised.
5. **Fixtures.** Both runners read the *same* `fixtures/fixtures.json` (path passed as
   `argv[1]` by the harness) and cases reference fixtures by name only, so the two sides
   cannot drift.

## `Cheerio.prototype` (71 symbols)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `add` | `Selection.Add` / `Selection.AddSelection` | match | `tr-add`, `tr-add-selection` | |
| `addBack` | `Selection.AddBack` | differs | `tr-add-back`, `tr-add-back-filtered` | Go drops the previous stage: returns only the current set |
| `addClass` | `Selection.AddClass` | differs | `man-addclass`, `man-addclass-dup`, `man-addclass-no-attr` | Go rewrites the whole `class` attribute, collapsing the original inter-token whitespace |
| `after` | `Selection.After` / `AfterNodes` / `AfterSelection` | match | `man-after` | |
| `append` | `Selection.Append` / `AppendNodes` / `AppendSelection` | match | `man-append`, `man-append-multi-args`, `man-append-multi-target`, `man-append-text`, `man-append-into-table`, `man-chain-order`, `ser-comment-preserved`, `ser-void-no-close-tag` | |
| `appendTo` | `Selection.AppendTo` | match | `man-appendto` | |
| `attr` | `Selection.Attr` / `Attrs` / `SetAttr` | differs | `acc-attr-*`, `man-setattr`, `man-setattr-overwrite-order`, `man-setattr-escaping`, `ser-escape-attr-*` | getter matches; the setter's serialisation escapes `<`/`>` inside attribute values, upstream leaves them raw |
| `before` | `Selection.Before` / `BeforeNodes` / `BeforeSelection` | match | `man-before` | |
| `cheerio` | — | untested | — | internal marker property (`"[cheerio object]"`) |
| `children` | `Selection.Children` | match | `tr-children`, `tr-children-filtered`, `tree-void-children`* | |
| `clone` | `Selection.Clone` | match | `tr-clone-detached`, `tr-clone-independent` | |
| `closest` | `Selection.Closest` | match | `tr-closest-self`, `tr-closest-ancestor`, `tr-closest-miss` | |
| `constructor` | — | untested | — | internal |
| `contents` | `Selection.Contents` | match | `tr-contents`, `tree-comment-contents` | |
| `css` | `Selection.Css` / `SetCss` / `SetCssMap` | differs | `acc-css-present`, `acc-css-hyphenated`, `acc-css-no-space`, `acc-css-absent`, `man-setcss`, `man-setcss-new` | absent property: upstream `undefined`, Go `""` |
| `data` | `Selection.Data` / `DataOr` / `DataMap` / `SetData` | differs | `acc-data-*`, `man-setdata` | getters match after stringification; the **setter** diverges: cheerio caches the value in memory and never touches the DOM, the Go port writes a real `data-*` attribute |
| `each` | `Selection.Each` | untested | — | callback-based; not expressible over the JSON-Lines protocol |
| `empty` | `Selection.Empty` | match | `man-empty` | |
| `end` | `Selection.End` | differs | `tr-end` | Go returns the current set instead of the previous traversal stage |
| `eq` | `Selection.Eq` | match | `tr-eq`, `tr-eq-negative`, `tr-eq-out-of-range` | |
| `extract` | — | missing | — | cheerio 1.0's declarative scraping helper is not ported |
| `filter` | `Selection.Filter` / `FilterMatcher` | match | `tr-filter` | |
| `filterArray` | — | untested | — | internal |
| `find` | `Selection.Find` / `FindMatcher` | match | `tr-find-nested`, `tr-find-dedup`, `comb-scoped-child`, `comb-scoped-adjacent`, all `sel-*` | |
| `first` | `Selection.First` | match | `tr-first`, `tr-first-empty` | |
| `get` | `Selection.Get` | untested | — | index accessor; exercised only indirectly |
| `has` | `Selection.Has` | match | `tr-has` | |
| `hasClass` | `Selection.HasClass` | match | `acc-hasclass-*` | |
| `html` | `Selection.Html` / `SetHtml` | match | `acc-html-*`, `man-sethtml*`, `ser-script-not-escaped`, `ser-style-not-escaped`, `tree-textarea-html` | |
| `index` | `Selection.Index` / `IndexSelector` / `IndexOfNode` | match | `acc-index-zero-arg`, `acc-index-selector`, `acc-index-selector-miss` | |
| `insertAfter` | `Selection.InsertAfter` | match | `man-insertafter` | |
| `insertBefore` | `Selection.InsertBefore` | match | `man-insertbefore` | |
| `is` | `Selection.Is` / `IsMatcher` | match | `acc-is-true`, `acc-is-false` | |
| `last` | `Selection.Last` | match | `tr-last` | |
| `map` | `Selection.Map` / `MapNodes` | untested | — | callback-based |
| `next` | `Selection.Next` | match | `tr-next`, `tr-next-filtered` | |
| `nextAll` | `Selection.NextAll` | match | `tr-next-all`, `tr-next-all-filtered` | |
| `nextUntil` | `Selection.NextUntil` | match | `tr-next-until`, `tr-next-until-filtered` | |
| `not` | `Selection.Not` | match | `tr-not` | |
| `parent` | `Selection.Parent` | match | `tr-parent`, `tr-parent-filtered-miss` | |
| `parents` | `Selection.Parents` | match | `tr-parents`, `tr-parents-filtered`, `tr-parents-multi-origin` | |
| `parentsUntil` | `Selection.ParentsUntil` | match | `tr-parents-until`, `tr-parents-until-filtered` | |
| `prepend` | `Selection.Prepend` / `PrependNodes` / `PrependSelection` | match | `man-prepend` | |
| `prependTo` | `Selection.PrependTo` | match | `man-prependto` | |
| `prev` | `Selection.Prev` | match | `tr-prev` | |
| `prevAll` | `Selection.PrevAll` | match | `tr-prev-all` | nearest-first order agrees |
| `prevUntil` | `Selection.PrevUntil` | match | `tr-prev-until` | |
| `prop` | `Selection.Prop` / `SetProp` | match | `acc-prop-*`, `man-setprop-bool-on`, `man-setprop-bool-off` | equal only after the Node side stringifies booleans (see Normalisation 3) |
| `remove` | `Selection.Remove` | match | `man-remove`, `man-remove-filtered` | |
| `removeAttr` | `Selection.RemoveAttr` | match | `man-removeattr`, `man-removeattr-absent` | |
| `removeClass` | `Selection.RemoveClass` | differs | `man-removeclass`, `man-removeclass-all-tokens`, `man-removeclass-zero-arg` | the Go port has no zero-argument "remove every class" form |
| `replaceWith` | `Selection.ReplaceWith` / `ReplaceWithNodes` / `ReplaceWithSelection` | match | `man-replacewith`, `man-replacewith-multi` | |
| `serialize` | `Selection.Serialize` | differs | `form-serialize`, `form-serialize-encoding` | see `serializeArray` |
| `serializeArray` | `Selection.SerializeArray` | differs | `form-serialize-array`, `form-serialize-inputs-only`, `form-serialize-select-multiple`, `form-serialize-empty` | for a `multiple` select upstream submits each selected option's *text* (`M1`), the Go port submits its `value` (`m1`); upstream's own single-select path uses `value`, so this looks like an upstream inconsistency |
| `siblings` | `Selection.Siblings` | match | `tr-siblings`, `tr-siblings-filtered` | |
| `slice` | `Selection.Slice` | match | `tr-slice`, `tr-slice-negative`, `tr-slice-clamped` | Go requires both bounds (upstream's one-arg form has no counterpart) |
| `splice` | — | missing | — | Array-mutation method inherited from the array-like base; no Go analogue (`Nodes`/`ToArray` are read-only views) |
| `text` | `Selection.Text` / `SetText` | differs | `acc-text-*`, `man-settext`, `ser-escape-text-*`, `tree-*-text` | getter matches; the setter's serialisation emits U+00A0 literally where upstream emits `&nbsp;` |
| `toArray` | `Selection.ToArray` | untested | — | used internally by the Node runner only |
| `toString` | `Selection.ToString` | match | `acc-tostring-multi` | |
| `toggleClass` | `Selection.ToggleClass` | match | `man-toggleclass` | |
| `unwrap` | `Selection.Unwrap` | match | `man-unwrap` | Go takes no selector argument |
| `val` | `Selection.Val` / `SetVal` | differs | `acc-val-*`, `man-setval-*` | control with no `value` attribute: upstream `undefined`, Go `""` |
| `wrap` | `Selection.Wrap` | match | `man-wrap`, `man-wrap-nested` | |
| `wrapAll` | `Selection.WrapAll` | match | `man-wrapall` | |
| `wrapInner` | `Selection.WrapInner` | match | `man-wrapinner` | |
| `_findBySelector` | — | untested | — | internal |
| `_make` | — | untested | — | internal |
| `_makeDomArray` | — | untested | — | internal |
| `_parse` | — | untested | — | internal |
| `_render` | — | untested | — | internal |

\* `tree-void-children` fails, but the cause is parsing (`<col>` handling), not `children`.

## Loaded `$` own properties (15 symbols)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `load` (`cheerio.load`) | `cheerio.Load` | differs | all 25 `parse-*`, all `tree-*` | tree-construction differences — see "Divergences" |
| `$.html` | `Document.Html` / `Node.OuterHTML` / `cheerio.RenderNodes` | differs | `ser-*`, `static-html-*`, `acc-outerhtml-escapes` | `<`/`>` inside attribute values, doctype, entity re-escaping |
| `$.text` | `cheerio.TextOf` / `Document.Text` | match | `static-text-nodes`, `static-text-empty` | |
| `$.root` | `Document.Root` | match | `ser-doc-text`, every `chain` case (the initial selection) | Go's `Document.Root()` returns the document node, matching `$.root()` (its doc comment claims it returns the top-level *elements*, which is wrong) |
| `$.contains` | `cheerio.Contains` | match | `static-contains-true`, `static-contains-false`, `static-contains-self` | |
| `$.merge` | `cheerio.Merge` | differs | `static-merge`, `static-merge-overlap` | upstream concatenates, the Go port de-duplicates |
| `$.parseHTML` | `cheerio.ParseHTML` | differs | `static-parsehtml-*` | upstream returns `null` for `""`; Go returns an empty slice |
| `$.extract` | — | missing | — | not ported |
| `$.xml` | — | missing | — | the Go port has no XML/XHTML serialisation mode |
| `$.fn` | — | missing | — | no plugin/extension point in the Go port |
| `$.length` | `Selection.Length` | match | `acc-length`, every `count` case | instance property, tested as a selection accessor |
| `$.load` | `cheerio.Load` | untested | — | re-exported `load` on the loaded instance |
| `$.prototype` | — | untested | — | internal |
| `$._options` | — | untested | — | internal |
| `$._root` | — | untested | — | internal |
| `$.name` | — | untested | — | `Function.name` artefact |

## Module exports not present on `$` (4 symbols)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `cheerio.fromURL` | — | missing | — | network loader; no Go equivalent |
| `cheerio.loadBuffer` | — | missing | — | encoding sniffing from a Buffer |
| `cheerio.decodeStream` | — | missing | — | streaming parser |
| `cheerio.stringStream` | — | missing | — | streaming parser |

## Go-only symbols (`extra`)

Untested unless a case is listed. None of these has an upstream counterpart to compare against.

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `cheerio.Compile`, `cheerio.MustCompile`, `Matcher.Match`, `Matcher.String` | extra | `compile-ok`, `compile-bad-bracket`, `compile-bad-pseudo-paren`, `compile-trailing-combinator`, `compile-empty`, `compile-unknown-pseudo` | pre-compiled selectors; upstream has no equivalent, so these cases compare `$(selector)` throwing against `Compile` returning an error |
| `Document.FindMatcher`, `Selection.FindMatcher`, `FilterMatcher`, `IsMatcher` | extra | — | pre-compiled variants |
| `Document.Render`, `Node.Render`, `Selection.Render` | extra | — | streaming serialisation to an `io.Writer` |
| `Selection.Attrs`, `AttrOr`, `DataOr`, `DataMap` | extra/mapped | `acc-attr-all`, `acc-data-all` | `Attrs`/`DataMap` stand in for upstream's no-argument `attr()`/`data()` |
| `Selection.Even`, `Selection.Odd` | extra | — | no cheerio counterpart |
| `Selection.FilterFunc`, `Selection.MapNodes`, `Selection.Nodes` | extra | — | Go-idiomatic callback/slice forms |
| `Selection.IndexOfNode` | extra | — | corresponds to jQuery `.index(element)` |
| `Selection.OuterHtml` | extra/mapped | `acc-outerhtml-first`, `acc-outerhtml-escapes` | upstream uses `$.html(node)` |
| `Selection.RemoveData` | extra | `man-removedata` | not on `Cheerio.prototype` in cheerio@1.0.0 |
| `Selection.SetAttrs`, `SetCssMap` | extra | — | map-taking bulk setters |
| `Selection.TagName`, `Node.TagName` | extra/mapped | `acc-tagname`, `tree-case-folded-tag` | upstream uses `.prop('tagName')` |
| `Selection.AddNodes`, `AfterNodes`, `AppendNodes`, `BeforeNodes`, `PrependNodes`, `ReplaceWithNodes`, `*Selection` variants | extra/mapped | as for their string forms | Go splits cheerio's polymorphic argument into typed methods |
| `Node.*` (`AppendChild`, `AttrValue`, `Clone`, `FirstChild`, `HasClass`, `InnerHTML`, `LastChild`, `Matches`, `NextSibling`, `NextElementSibling`, `OuterHTML`, `PrevSibling`, `PrevElementSibling`, `RemoveAttr`, `SetAttr`, `TagName`, `Text`) | extra | `ser-inner-all`, `ser-outer-all` | node-level DOM API; cheerio exposes the raw domhandler node instead |
| `cheerio.RenderNodes`, `cheerio.TextOf` | extra/mapped | `static-html-*`, `static-text-*` | stand in for `$.html(nodes)` / `$.text(nodes)` |
| `NodeType` and its constants, `Attribute`, `FormField` | extra | — | exported types |

## Status update (this round)

Most divergences catalogued below have since been fixed in the port and now
match upstream; the descriptions are kept for the record. Concretely:

- **Serialisation / escaping (all fixed):** attribute values no longer escape
  `<`/`>`, and both text and attribute serialisation emit `&nbsp;` for U+00A0
  — matching cheerio's dom-serializer (`parse-escape-out`, `acc-outerhtml-escapes`,
  `man-setattr-escaping`, `static-html-escapes`, `ser-escape-out-roundtrip`,
  `ser-escape-text-nbsp`).
- **Entity decoding (fixed):** the decoder now does longest-prefix matching and
  honours the legacy (semicolon-optional) entities, so `&notanentity;` → `¬anentity;`
  and `&amp` → `&` (`parse-entities`, `text-entities`, `tree-entities-text`,
  `ser-entities-roundtrip`).
- **Parser (fixed):** foster parenting for stray content in `<table>`; the
  self-closing slash is ignored on non-void elements; `<col>` outside a table is
  dropped; a stray `</p>` synthesises an empty `<p></p>` under the same conditions
  parse5 does (`parse-table-foster`, `tree-foster-siblings`,
  `parse-self-closing-nonvoid`, `parse-void-elements`, `tree-void-children`,
  `parse-select-in-p`, `tree-select-in-p-children`).
- **Selectors (fixed):** `:empty`/`:parent` treat whitespace text as content;
  `:enabled` matches any element without a `disabled` attribute; an unknown
  pseudo-class is a `Compile` error; the empty selector compiles to a
  match-nothing matcher (`pseudo-empty`, `pseudo-parent`, `pseudo-enabled`,
  `compile-unknown-pseudo`, `compile-empty`).
- **Traversal (fixed):** `End`/`AddBack` now track the correct previous stage
  through a filtered traversal step (`tr-end`, `tr-add-back`, `tr-add-back-filtered`).
- **Accessors/manipulation/statics (fixed):** `AddClass` preserves the original
  class-attribute whitespace; `RemoveClass()` has a zero-argument remove-all form;
  `SetData` caches in memory instead of writing an attribute; `Merge` concatenates
  without de-duplicating; `ParseHTML("")` returns nil (→ `null`).

**Remaining real difference:** the adoption agency algorithm is still not
implemented (`parse-misnested-format`, `parse-misnested-deep`).

The nine deliberate deviations are documented in `cheerio/API-DEVIATIONS.md`
(absent `Css`/`Val` → `""`, `multiple`-select serialisation, `RemoveData`,
namespace-qualified selectors, and the fragment-mode doctype artefact).

## Divergences found (original catalogue; see status update above)

### HTML parsing and error recovery (highest-value area)

1. **Foster parenting is not implemented** (`parse-table-foster`, `tree-foster-siblings`).
   `<table><b>stray</b><tr>…` — upstream moves `<b>` out *before* the table
   (`<b>stray</b><table>…`); the Go port leaves it inside the `<table>` element.
2. **No adoption agency algorithm** (`parse-misnested-format`, `parse-misnested-deep`).
   `<b>1<i>2</b>3</i>` — upstream reconstructs the active formatting elements and yields
   `<b>1<i>2</i></b><i>3</i>`; the Go port yields `<b>1<i>2</i></b>3`, dropping the
   reopened `<i>`.
3. **`<col>` is not table-scoped** (`parse-void-elements`, `tree-void-children`).
   A `<col>` outside a table is dropped by the HTML5 tree builder; the Go port keeps it.
4. **Self-closing syntax is honoured on non-void elements** (`parse-self-closing-nonvoid`).
   `<div><span/>after</div>` — upstream parses `<span>after</span>`; the Go port closes the
   span immediately: `<span></span>after`.
5. **A stray `</p>` does not synthesise an empty paragraph** (`parse-select-in-p`,
   `tree-select-in-p-children`). `<p>a<table>…</table>c</p>` — upstream ends with an extra
   empty `<p></p>`, the Go port does not.
6. **Entity decoding is stricter than the HTML5 spec** (`parse-entities`, `text-entities`,
   `tree-entities-text`, `ser-entities-roundtrip`):
   - `&notanentity;` — upstream applies the longest-prefix rule and decodes `&not` to `¬`,
     leaving `¬anentity;`; the Go port leaves the whole run literal.
   - `&amp` (no trailing semicolon) — upstream decodes to `&`; the Go port keeps `&amp`.
7. **Doctype in fragment mode** (`parse-comments-doctype`, `ser-doctype-preserved`): the Go
   port preserves `<!DOCTYPE html>`, parse5's fragment mode drops it. Mode artefact — see
   Normalisation 1.

Correctly recovered (all matching): implied `<tbody>`, unclosed `<p>`/`<li>`/`<dt>`/`<dd>`,
unclosed `<td>`/`<tr>`, nested tables, stray end tags, raw-text elements
(`script`/`style`/`textarea`/`title`), duplicate attributes, unquoted and single-quoted
attribute values, tag/attribute case folding, comments, empty and text-only input.

### Serialisation / escaping

8. `<` and `>` inside an attribute **value** are escaped by the Go port and left raw by
   upstream (`parse-escape-out`, `acc-outerhtml-escapes`, `man-setattr-escaping`,
   `static-html-escapes`, `ser-escape-out-roundtrip`).
9. U+00A0 in text is emitted as `&nbsp;` by upstream and as a literal byte sequence by the
   Go port (`ser-escape-text-nbsp`).

### Selectors

10. **`:empty` / `:parent` ignore whitespace-only text nodes** (`pseudo-empty`,
    `pseudo-parent`). `<div class="ws"> </div>` counts as empty in the Go port; per CSS it
    is not empty because it has a text child.
11. **`:enabled` excludes `<form>`** (`pseudo-enabled`): upstream matches 20 elements
    including the `<form>` itself, the Go port matches 19.
12. **Namespace-qualified type selectors** (`sel-namespace-qualified`): the Go port
    implements `*|p`; css-select throws *"Namespaced tag names are not yet supported"*.
    The port is a superset here.
13. **Unknown pseudo-classes are silently accepted** (`compile-unknown-pseudo`):
    `:totally-unknown` throws upstream, compiles (and matches nothing) in Go.
14. **The empty selector** (`compile-empty`): `$("")` returns an empty selection upstream;
    `cheerio.Compile("")` returns an error. This is a documented Go design choice.

Every other selector feature matched: all seven attribute operators plus the `i` flag, all
four combinators, `:scope`-relative `find` members, `:nth-child`/`:nth-last-child`/
`:nth-of-type`/`:nth-last-of-type` with integers, `odd`/`even`, `an+b`, `an-b`, `-n+b`,
bare `n`, `0` and internal whitespace, `:first/last/only-child`,
`:first/last/only-of-type`, `:root`, `:not` (including selector lists and chaining),
`:has` (including `> h2`), `:contains` (quoted and bare), `:header`, `:input`, `:checked`,
`:selected`, `:disabled`, and nested `:not(:has(...))`.

### Accessors / API shape

15. `Css` and `Val` return `""` where cheerio returns `undefined` (`acc-css-absent`,
    `acc-val-no-value-attr`).
16. `Selection.SetData` writes a real `data-*` attribute; cheerio's `.data(k, v)` only
    populates an in-memory cache and leaves the DOM untouched (`man-setdata`).
17. `Selection.AddBack` and `Selection.End` do not track the previous traversal stage
    (`tr-add-back`, `tr-add-back-filtered`, `tr-end`) — both behave as no-ops on the
    current set.
18. `Selection.RemoveClass` has no zero-argument "remove all" form
    (`man-removeclass-zero-arg`).
19. `AddClass` normalises the whitespace of the existing `class` attribute
    (`man-addclass-dup`).
20. `SerializeArray`/`Serialize`: for `<option value="m1" selected>M1</option>` inside a
    `multiple` select, upstream submits the option **text** (`M1`) while the Go port
    submits the `value` (`m1`) (`form-serialize-array`, `form-serialize`,
    `form-serialize-select-multiple`). Upstream's single-select path *does* use `value`
    (`acc-val-select` matches), so this is most likely an upstream bug in
    `Cheerio.prototype.val` for `select[multiple]`, not a port defect.
21. `cheerio.Merge` de-duplicates; `cheerio.merge` does not (`static-merge-overlap`).
22. `cheerio.ParseHTML("")` returns an empty slice; `$.parseHTML("")` returns `null`
    (`static-parsehtml-empty`).

## Upstream API the port lacks

`extract`, `splice`, `$.xml`, `$.fn` (plugin surface), `cheerio.fromURL`,
`cheerio.loadBuffer`, `cheerio.decodeStream`, `cheerio.stringStream`, the zero-argument
`removeClass()` form, and the one-argument `slice(start)` form.

## Score summary

| scope | match | differs | missing | untested | extra | total | parity |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **upstream total** | **60** | **1** | **9** | **17** | — | **90** | **98.36 % of 61 compared** |
| cases | 306 | 2 | — | — | — | 317 | **99.35 % of 308 compared** |

Symbol parity = `match / (match + differs)` = 60 / 61 = 98.36 % (3 symbols —
`serialize`, `serializeArray`, `val` — are now pure deviations, excluded).
Case parity = `match / (match + differ)` = 306 / 308 = 99.35 % (9 deviations
excluded).
A symbol with no case is `untested`, never `match`.
