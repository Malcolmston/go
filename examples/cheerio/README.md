# cheerio example

A single runnable program that exercises `github.com/malcolmston/cheerio` — parsing,
CSS selectors, accessors, traversal, manipulation, forms and serialization — entirely on
string literals embedded in the program. No network access, no I/O beyond stdout; the
program terminates on its own.

## Module version

The library is consumed as a published Go module — there is **no `replace` directive**.

| Module | Resolved version |
|---|---|
| `github.com/malcolmston/cheerio` | `v0.0.0-20260719012630-7b40c4063eb0` |

The repository carries no semver tags, so `@latest` resolves to that pseudo-version
(commit `7b40c4063eb0`, 2026-07-19). The published tree is byte-identical to the local
`cheerio/` working copy apart from an untracked `web/vendor` directory, so nothing in
this example depends on uncommitted local changes.

## Run

```sh
cd examples/cheerio
GOWORK=off go get github.com/malcolmston/cheerio@latest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

## What it demonstrates

| Section | API surface |
|---|---|
| Parsing | `Load` on a full document: entity decoding in text and attributes, void elements, raw-text `<script>`, preserved comments and doctype, recovery of unclosed `<p>`/`<tr>`/`</table>` |
| Selectors | 27 live selectors covering type/id/class, all attribute operators (`^= *= ~= \|=`), all four combinators, selector lists, `:first-child`, `:nth-child(odd)`, `:nth-of-type(n)`, `:last-of-type`, `:empty`, `:only-child`, `:root`, `:not()`, `:has()`, `:contains()`, `:header`, `:input`, `:checked`, `:selected`, `:enabled`, `:disabled` |
| Precompiled matchers | `Compile`, `MustCompile`, `Matcher.String`, `Document.FindMatcher`, `Selection.IsMatcher`, and the error path for an invalid selector |
| Accessors | `Attr`, `AttrOr`, `Attrs`, `TagName`, `HasClass`, `Data`/`DataOr`/`DataMap` (dash↔camel), `Prop`, `Text`, `Html`, `OuterHtml`, `Css`, `Val` (input/checkbox/select/textarea) |
| Traversal | `First`, `Last`, `Eq`, `Even`, `Odd`, `Slice`, `Children`, `Contents`, `Parent`, `Parents`, `ParentsUntil`, `Closest`, `Next`, `Prev`, `NextAll`, `PrevAll`, `NextUntil`, `Siblings`, `Filter`, `FilterFunc`, `Not`, `Is`, `Has`, `Index`, `IndexSelector`, `IndexOfNode`, `Add`, `AddSelection`, `AddNodes`, `AddBack`, `End`, `Each`, `Map`, `Get`, `Nodes`, `ToArray` |
| Node API | `Node.TagName/Text/InnerHTML/OuterHTML/FirstChild/LastChild/NextElementSibling/PrevElementSibling/Matches/HasClass/AttrValue` |
| Static helpers | `ParseHTML`, `RenderNodes`, `TextOf`, `Contains`, `Merge` |
| Forms | `SerializeArray`, `Serialize` — correctly skips unnamed/disabled/submit controls, unchecked boxes, and picks the selected option |
| Manipulation | `Append`, `Prepend`, `Before`, `After`, `AppendNodes`, `PrependSelection`, `AppendTo`, `PrependTo`, `InsertBefore`, `InsertAfter`, `ReplaceWith`, `Remove`, `Empty`, `Clone`, `Wrap`, `WrapAll`, `WrapInner`, `Unwrap`, `SetText`, `SetHtml`, `SetAttr`, `SetAttrs`, `AddClass`, `RemoveClass`, `ToggleClass`, `SetCss`, `SetCssMap`, `SetData`, `RemoveData` |
| Output | `Document.Html/Text/Render`, `Selection.ToString/Render/MapNodes` |

Everything listed above compiles and runs; nothing in the example is commented out. Form
serialization, `:nth-*` with `an+b`/`odd`/`even`/negative coefficients, namespace
selectors (`svg|rect`, `*|title`), case-insensitive attribute matching (`[a=v i]`),
`ToggleClass`, `Prop`/`SetProp`, clone independence, and attribute/text escaping on
output were all verified against separate probes and behave as documented.

## Holes found

1. **No table foster parenting, contradicting "recovered the way a browser would".**
   `Load("<table><tr><td>a</td></tr><p>stray</p></table>")` leaves the `<p>` *inside*
   `<tbody>`. Every browser and the HTML5 spec foster-parent non-table content out to
   just before the `<table>`. The example's document omits `</table>`, which is why
   `Children of table` reports `[tr tr tr tr p p div div form script]` — the paragraphs,
   divs, form and script all end up as `<tbody>` children.

2. **Implied `<tbody>` insertion is undocumented and silently breaks child-combinator
   selectors.** `Load("<table><tr><td>a</td></tr></table>")` yields
   `table > tbody > tr`, so `doc.Find("table > tr")` matches **0** elements. This is the
   correct HTML5 behavior, but the README's parsing section lists only implicit *closes*
   and never mentions tag *insertion*, so it is a real trap: the obvious selector for a
   simple hand-written table silently returns nothing.

3. **`Find` and `Node.Matches` silently swallow selector syntax errors, and one
   malformed form returns a *wrong* result rather than nothing.** `Compile("p[")`
   correctly returns `cheerio: invalid selector "p["`, but `Find` and `Matches` call the
   error-free internal `parseSelector`, so:
   - `Find("p[")`, `Find(":::bogus(")` and `Find("p:nosuchpseudo")` all return 0 matches
     with no error — a typo becomes a silent empty result;
   - **`Find(".a >")` returns 1**: the dangling child combinator is silently discarded
     and the selector degrades to `.a`, matching an element that the written selector
     could never legally match.

   There is no error-returning `Find` variant, so `Compile`/`FindMatcher` is the only
   way to validate a selector — and that is not discoverable from the README's Selection
   API list, which presents `Find` as the primary entry point.

4. **`Selection` carries no error channel and no panic guard on out-of-range access, but
   `Get` returns a raw `*Node`.** `Get(99)` returns `nil` (good), yet the returned
   `*Node` is then dereferenced by every `Node` method (`n.TagName()` on a nil node
   panics). `Eq`/`First`/`Last` on an empty selection all safely return length-0
   selections, so the nil-returning `Get` is the odd one out.

5. **Non-idiomatic getter/setter naming.** Go convention is `Attr`/`SetAttr` (which the
   library follows) but also `Html()`/`SetHtml()` alongside `OuterHtml()` with *no*
   setter, plus a parallel `Node.InnerHTML`/`OuterHTML` using the opposite
   capitalization (`HTML` vs `Html`) for the same concept. Two spellings of the same
   initialism across `Selection` and `Node` is a persistent source of compile errors.

6. **`Selection.Render` is not actually streaming** despite its doc comment ("the
   streaming form of `ToString`, avoiding an intermediate string"): the implementation
   serializes into a `strings.Builder` and then does a single `io.WriteString`, so the
   full document is still materialized in memory.

Non-issues checked and deliberately *not* reported: `IndexSelector` returning `-1` when
the receiver's first node does not match the selector is documented and matches jQuery;
entity references are correctly *not* decoded inside `<script>`/`<style>`; `Load` never
panics on garbage input (`"<<>><p unclosed=\""` parses cleanly).
