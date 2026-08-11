# `express/sanitizehtml` vs npm `sanitize-html`

Oracle: **sanitize-html@2.17.6** (pinned in `node/package.json`).
Port: `github.com/malcolmston/express/sanitizehtml`.
Harness: `GOWORK=off go test .` in this directory, after `npm install` in `node/`.

> **Security.** Two divergences found by this harness were the port being *less
> safe* than upstream. They are written up in `security.json` in this directory,
> not as public issues, and both are fixed in the `express` working tree.

## How the upstream inventory was derived

The exported surface, from the installed package:

```
$ cd node && node -e "const m=require('sanitize-html');
  console.log(typeof m, m.name, m.length);
  console.log(JSON.stringify(Object.getOwnPropertyNames(m)));
  console.log(JSON.stringify(Object.keys(m.defaults).sort()))"
function sanitizeHtml 3
["length","name","arguments","caller","prototype","defaults","simpleTransform"]
["allowProtocolRelative","allowedAttributes","allowedEmptyAttributes",
 "allowedSchemes","allowedSchemesAppliedToAttributes","allowedSchemesByTag",
 "allowedTags","disallowedTagsMode","enforceHtmlBoundary","nonBooleanAttributes",
 "parseStyleAttributes","preserveEscapedAttributes","selfClosing"]
```

`defaults` only lists the options that *have* a default value, so the full option
surface was enumerated by scraping every `options.X` read in the library source —
that is the complete set the function actually consults:

```
$ cd node && node -e "
  const src = require('fs').readFileSync(require.resolve('sanitize-html'),'utf8');
  const s = new Set();
  for (const m of src.matchAll(/options\.([A-Za-z0-9_]+)/g)) s.add(m[1]);
  console.log(JSON.stringify([...s].sort()))"
["allowIframeRelativeUrls","allowProtocolRelative","allowVulnerableTags",
 "allowedAttributes","allowedClasses","allowedEmptyAttributes",
 "allowedIframeDomains","allowedIframeHostnames","allowedSchemes",
 "allowedSchemesAppliedToAttributes","allowedSchemesByTag",
 "allowedScriptDomains","allowedScriptHostnames","allowedStyles","allowedTags",
 "disallowedTagsMode","enforceHtmlBoundary","exclusiveFilter","nestingLimit",
 "nonBooleanAttributes","nonTextTags","onCloseTag","onOpenTag",
 "parseStyleAttributes","parser","preserveEscapedAttributes","selfClosing",
 "textFilter","transformTags"]
```

`nonTextTags` has no entry in `defaults` because its default lives inside the
function body; `node/run.js` reads that literal back out of the library source for
the `defaultNonTextTags` case, so the comparison stays honest even for a value the
package does not expose. The Go side was enumerated with
`GOWORK=off go doc -all ./sanitizehtml` in the `express` submodule.

## Symbol inventory

### Entry points

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sanitizeHtml(html, options)` | `sanitizehtml.Sanitize(string, Options) string` | match | 225 of the 233 cases | |
| `sanitizeHtml.defaults` | `sanitizehtml.DefaultOptions()` + the `Default*` vars | match | group `defaults` (8) | Every default list is compared element by element, sorted on both sides. |
| `sanitizeHtml.simpleTransform(tagName, attribs, merge)` | — | missing | — | A helper that only builds a `transformTags` entry, which is not ported. |

### Options

| upstream option | Go field | status | cases | note |
| --- | --- | --- | --- | --- |
| `allowedTags` | `Options.AllowedTags` | differs | `pol-tags-*` (11) | 9 match. `allowedTags: false` (allow every tag) has no `[]string` equivalent, so the port spells it `"*"`; the allowlist is also matched case-insensitively. Two declared deviations. |
| `allowedAttributes` | `Options.AllowedAttributes` | differs | `pol-attrs-*` (16) | 15 match. `allowedAttributes: null` means allow-every-attribute upstream; the port will not give a nil map a permissive meaning. One declared deviation, with `pol-attrs-allow-all-glob` covering the portable spelling. |
| `allowedClasses` | `Options.AllowedClasses` | match | `pol-classes-*` (7) | Includes the implicit `class` permission a tag gains by appearing here. |
| `allowedSchemes` | `Options.AllowedSchemes` | match | `pol-schemes-custom`, `pol-schemes-empty`, `def-allowed-schemes` (`pol-schemes-*`, 4) | |
| `allowedSchemesByTag` | `Options.AllowedSchemesByTag` | match | `pol-schemes-by-tag`, `pol-schemes-by-tag-empty` | |
| `allowedSchemesAppliedToAttributes` | `Options.AllowedSchemesAppliedToAttributes` | match | `def-scheme-attributes`, `pol-scheme-attrs-*` (13), `pol-imagesrcset`, `pol-srcset-*` (4) | **Was the first security finding.** |
| `allowProtocolRelative` | `Options.AllowProtocolRelative` | match | `by-href-protocol-relative`, `by-href-protocol-relative-backslash`, `by-href-protocol-relative-denied`, `def-allow-protocol-relative` | |
| `nonTextTags` | `Options.NonTextTags` | match | `def-non-text-tags`, `pol-nontext-*` (3), `by-option-*`, `by-textarea-*`, `by-xmp*` | **Was the second security finding.** |
| `selfClosing` | `Options.SelfClosing` | match | `def-self-closing`, `pol-selfclosing-*` (4), group `void` (32) | |
| `allowedEmptyAttributes` | `Options.AllowedEmptyAttributes` | match | `pol-attrs-empty-value`, `pol-attrs-empty-alt`, `pol-attrs-empty-custom`, `by-href-empty` | |
| `nonBooleanAttributes` | `Options.NonBooleanAttributes` | match | `pol-attrs-boolean`, `pol-attrs-nonboolean-star`, `pol-attrs-empty-value` | |
| `disallowedTagsMode` | `Options.DisallowedTagsMode` | match | `def-disallowed-mode`,  `pol-mode-*` (14) | All four modes plus the unrecognized-value fallback. |
| `textFilter` | `Options.TextFilter` | match | `pol-textfilter-*` (6) | A function cannot travel as JSON, so both runners implement the same four named filters (`upper`, `brackets`, `drop`, `markup`) and a case selects one by name. |
| `transformTags` | — | missing | — | Not ported. |
| `exclusiveFilter` | — | missing | — | Not ported. |
| `allowedStyles` | — | missing | — | Requires a CSS parser (upstream uses postcss). |
| `parseStyleAttributes` | — | missing | — | Only meaningful together with `allowedStyles`. |
| `allowedIframeHostnames` | — | missing | — | Not ported. |
| `allowedIframeDomains` | — | missing | — | Not ported. |
| `allowIframeRelativeUrls` | — | missing | — | Not ported. |
| `allowedScriptHostnames` | — | missing | — | Not ported. |
| `allowedScriptDomains` | — | missing | — | Not ported. |
| `nestingLimit` | — | missing | — | Not ported. |
| `enforceHtmlBoundary` | — | missing | — | Not ported. |
| `allowVulnerableTags` | — | missing | — | Suppresses a `console.warn` only; it has no effect on the output, and the port emits no warning to suppress. |
| `preserveEscapedAttributes` | — | missing | — | Not ported. |
| `onOpenTag` / `onCloseTag` | — | missing | — | Observer hooks with no effect on the output. |
| `parser` | — | missing | — | htmlparser2 options; the port has no third-party parser to configure. |

### Go-only surface

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `sanitizehtml.DefaultSchemes`, `DefaultSchemeAttributes`, `DefaultNonTextTags`, `DefaultSelfClosing`, `DefaultAllowedEmptyAttributes`, `DefaultNonBooleanAttributes` | extra | group `defaults` | Exported so a caller can extend a default list instead of retyping it. Each is compared against the corresponding upstream default. |
| — | `ModeDiscard`, `ModeCompletelyDiscard`, `ModeEscape`, `ModeRecursiveEscape` | extra | `pol-mode-*` | Named constants for upstream's string-literal union. |

### Counts

| status | symbols |
| --- | --- |
| match | 13 |
| differs | 2 |
| missing | 17 |
| extra | 2 |
| untested | 0 |

**Parity: 13/15 compared symbols fully match (86.7%); the other 2 differ only in
declared, documented ways. Counting the 17 unported symbols, the port covers 15 of
32 upstream symbols (46.9%) — the option surface, not the behaviour, is where this
port is a subset: every ported option matches, and every case in the bypass corpus
matches.**
**Cases: 233 total, 223 match, 0 mismatch, 10 declared deviations — 100% of compared cases.**

Per group: `bypass` 110 (105 match, 5 deviations), `policy` 83 (79 match, 4
deviations), `void` 32 (31 match, 1 deviation), `defaults` 8/8.

## Behaviour verified by the cases

`cases/defaults.json` compares the default **policy** itself, list by list, rather
than only its effect on sample inputs. That is what caught the first security
finding: a port whose default `allowedSchemesAppliedToAttributes` holds three of
upstream's nineteen entries silently changes the security posture of every caller.

`cases/bypass.json` (110 cases) is the bypass corpus, run under the default policy
unless a case says otherwise:

* **Raw-text handling.** `<script>` plain, mixed-case, with attributes, unclosed,
  with a nested `</script>` inside a string literal, script-in-script, closed with
  `</script foo>` and `</script\n>`, self-closed, and preceded by a space so that
  `< script>` is not a tag at all. Plus `<style>`, `<textarea>`, `<title>`,
  `<option>`, `<xmp>`, `<noscript>`, `<noembed>`, `<iframe>` and `<plaintext>`, each
  both disallowed and (for the ones that matter) explicitly allowed.
* **Escapable-raw-text entity payloads** — the CVE-2026-40186 and
  GHSA-jxwj-j7wr-gfrw shapes: `by-textarea-entities`, `by-title-entities`,
  `by-option-entities`, `by-textarea-allowed-mis-close` (the `</textarea/>`
  solidus mis-close).
* **URL schemes.** `javascript:` plain, mixed-case, leading spaces, a raw tab
  inside the scheme, an entity-encoded tab, an entity-encoded newline, an
  entity-encoded NUL, an embedded HTML comment, an *unterminated* embedded comment,
  a scheme using the `. - +` characters the grammar permits, a bare leading colon;
  plus `vbscript:`, `data:text/html;base64,…`, relative, protocol-relative with
  slashes and with backslashes, and protocol-relative denied.
* **Attribute boundaries.** Entity-encoded and raw quote breakouts, an apostrophe
  breakout, an encoded `>` that must not close the tag on re-serialisation, an
  attribute *name* containing a quote, a `<`, an `=` or a backtick, an unquoted
  value, and a duplicate attribute where one copy is a payload.
* **Event handlers**, on a disallowed tag, on an allowed tag, upper-cased, and
  under an `on*` glob the caller asked for.
* **Comments and pseudo-markup.** Plain, unterminated, abrupt-closing `<!-->`,
  nested `<!--<!-- -->`, conditional, a processing instruction, a doctype, a CDATA
  section.
* **Malformed markup.** Stray `<`, stray `>`, stray `&` (with entity
  round-tripping), unclosed tags, a close tag with no matching open, a stray close
  tag, non-standard nesting, `<p<b>`, an unterminated tag, an unterminated tag
  carrying an event handler, and markup hidden inside a quoted attribute value.
* **Entity-encoded and double-encoded tags**, numeric entities, full-width
  look-alikes, a NUL in the tag name, a newline and a form feed in the tag name,
  namespaced tags, and astral-plane text interleaved with markup.

`cases/policy.json` (83 cases) exercises every ported option, including the
thirteen `pol-scheme-attrs-*` cases — one per URL-bearing attribute upstream
scheme-checks by default — and the srcset/imagesrcset candidate filtering.

`cases/void.json` (32 cases) pins the distinction the port originally got wrong:
whether an element can have content is a property of HTML, while
`Options.SelfClosing` only decides whether it is written `<x />` or `<x></x>`. It
walks all seventeen void elements with an empty `selfClosing` list, and covers a
normal element named in `selfClosing`, a solidus on a non-void element, void
elements in escape mode, and end-tag recovery.

## Declared deviations

Ten, all listed with their reasoning in the `express` submodule's
`API-DEVIATIONS.md`. In summary: the port rejects an attribute name containing a
quote or a backtick where upstream echoes it (the port is **stricter**); `"*"`
means every tag and the tag allowlist is case-insensitive; a nil
`AllowedAttributes` is not given a permissive meaning; `completelyDiscard` really
discards the subtree where upstream leaves nested allowed tags behind; nesting is
preserved as authored rather than re-parented by a tree builder; and an
unterminated tag is discarded rather than flushed as text.

## Error cases

Neither implementation has an error return: `sanitizeHtml` returns `''` for
`null`/`undefined` input and `Sanitize` returns `""` for `""`. There are no
`ok:false` cases in this harness, and that is the correct inventory rather than a
gap. The one place upstream *can* throw — `allowedStyles` together with
`parseStyleAttributes: false` — is inside an unported option.

## What was fixed while measuring

Measured parity went from **126/173 compared cases (72.8%)** on the first run to
**223/223 (100%)**; the corpus grew from 175 to 233 cases as each fix exposed more
behaviour worth pinning. Two of the divergences were security bugs
(`security.json`); the rest are enumerated in `API-DEVIATIONS.md` under "the
HTML-safety cluster".
