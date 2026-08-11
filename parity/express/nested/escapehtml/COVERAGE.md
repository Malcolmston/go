# `express/escapehtml` vs npm `escape-html`

Oracle: **escape-html@1.0.3** (pinned in `node/package.json`).
Port: `github.com/malcolmston/express/escapehtml`.
Harness: `GOWORK=off go test .` in this directory, after `npm install` in `node/`.

## How the upstream inventory was derived

`escape-html` is a single-function CommonJS module, so its whole exported surface
is the function object itself:

```
$ cd node && node -e "const m=require('escape-html');
  console.log(typeof m, m.name, m.length);
  console.log(JSON.stringify(Object.getOwnPropertyNames(m)))"
function escapeHtml 1
["length","name","prototype"]
```

There are no additional properties: `length`, `name` and `prototype` are the
intrinsics every JavaScript function has. The Go side was enumerated with
`GOWORK=off go doc -all ./escapehtml` in the `express` submodule.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `escapeHtml(string)` (module default export) | `escapehtml.Escape(string) string` | match | all 36 | Same five characters, same entity spellings, same pass-through for input with none of them. |
| — | — | — | — | Upstream exports nothing else. |

### Counts

| status | symbols |
| --- | --- |
| match | 1 |
| differs | 0 |
| missing | 0 |
| extra | 0 |
| untested | 0 |

**Parity: 1/1 compared symbols (100%).**
**Cases: 36 total, 36 match, 0 mismatch, 0 declared deviations — 100% of compared cases.**

## Behaviour verified by the cases

Every case is in `cases/escape.json`, group `escape`.

* The five characters and their exact entity spellings: `&amp;`, `&lt;`, `&gt;`,
  `&quot;` and the **numeric** `&#39;` for the apostrophe (`amp`, `lt`, `gt`,
  `dquote`, `squote`, `all-five`, `all-five-reversed`).
* The characters upstream deliberately does **not** escape: the backtick
  (`backtick`), `=` and `/` (`equals-slash`), whitespace (`newline-tab`), NUL
  (`nul-byte`) and the full-width look-alikes (`unicode-fullwidth-lt`).
* Attack shapes, so that "the payload comes out inert" is measured and not
  assumed: `script-tag`, `img-onerror`, `svg-payload`, `comment-payload`, the
  double-, single- and unquoted attribute breakouts
  (`attr-breakout-dquote`, `attr-breakout-squote`, `attr-breakout-unquoted`) and
  a JavaScript-string context (`js-string-context`) where *neither* side is
  sufficient — escape-html is an HTML encoder only, and the case records that
  both implementations agree on that limit.
* Double-escaping: an input that already contains an entity has its leading `&`
  escaped again on both sides (`already-escaped`, `amp-entity-name`).
* Position and fast-path coverage: a special character at index 0, at the end, in
  the middle, repeated, and after a long safe prefix (`leading-special`,
  `trailing-special`, `middle-special`, `repeated-amp`, `very-long-safe`,
  `long-with-late-special`) — the port has a no-match fast path and a prefix copy,
  and these exercise both.
* Non-ASCII survival: Latin-1, CJK, astral-plane emoji interleaved with escaped
  bytes, bidi controls and combining marks (`non-ascii-latin`, `non-ascii-cjk`,
  `emoji-and-lt`, `rtl-override`, `combining-marks`). Upstream iterates UTF-16
  code units and the port iterates bytes; these confirm the results are identical
  anyway.
* The empty string and plain ASCII (`empty`, `plain-ascii`).

## Differences that are not behaviour

`escapeHtml` coerces its argument with `'' + string`, so `escapeHtml(null)` is
`"null"` and `escapeHtml(42)` is `"42"`. `Escape` takes a Go `string`, so there is
no coercion to compare and no case can express one. This is a language property,
not a divergence, and it is the only thing separating the two APIs; nothing is
recorded in `API-DEVIATIONS.md` for this package.

Error behaviour: neither implementation can fail. `escapeHtml` throws only if
argument coercion throws (an object with a throwing `toString`), which a JSON
case cannot express, and `Escape` has no error path. There are therefore no
`ok:false` cases in this harness, and that is the correct inventory rather than a
gap.
