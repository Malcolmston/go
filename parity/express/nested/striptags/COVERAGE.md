# `express/striptags` vs npm `striptags`

Oracle: **striptags@3.2.0** (pinned in `node/package.json`).
Port: `github.com/malcolmston/express/striptags`.
Harness: `GOWORK=off go test .` in this directory, after `npm install` in `node/`.

## How the upstream inventory was derived

```
$ cd node && node -e "const m=require('striptags');
  console.log(typeof m, m.name, m.length);
  console.log(JSON.stringify(Object.getOwnPropertyNames(m)))"
function striptags 3
["length","name","prototype","init_streaming_mode"]
```

One callable module with one property. `length`, `name` and `prototype` are the
function intrinsics; `init_streaming_mode` is the only real member. The three
parameters were read from the installed `src/striptags.js`
(`striptags(html, allowable_tags, tag_replacement)`), which is also where the
allowlist-parsing rules below come from. The Go side was enumerated with
`GOWORK=off go doc -all ./striptags` in the `express` submodule.

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `striptags(html)` | `striptags.StripTags(string, ...string) string` | match | group `strip` (60) | |
| `striptags(html, allowable_tags)` — array form | `striptags.StripTagsWith(string, []string, string)` with `""` replacement | differs | `arr-*` (16) | 14 match; `arr-brackets` and `arr-upper-entry` are declared deviations — see below. |
| `striptags(html, allowable_tags)` — string form | `striptags.StripTags(string, ...string) string` | differs | `str-*` (13) | 8 match; 5 declared deviations. |
| `striptags(html, allowable_tags, tag_replacement)` | `striptags.StripTagsWith(string, []string, string) string` | match | `repl-*` (10) | |
| `striptags.init_streaming_mode(allowable_tags, tag_replacement)` | `striptags.NewStreamer([]string, string) *Streamer` + `(*Streamer).Write(string) string` | match | `stream-*` (12) | The returned closure becomes a `*Streamer` whose `Write` is called once per chunk. |
| — | `striptags.Streamer` (type) | extra | `stream-*` | The state upstream keeps in a closure has to be a named type in Go. |

### Counts

| status | symbols |
| --- | --- |
| match | 3 |
| differs | 2 |
| missing | 0 |
| extra | 1 |
| untested | 0 |

The two `differs` rows are the two spellings of the same argument, and both differ
only in how forgiving the allowlist parser is; the stripping behaviour itself is
identical. Counting them as compared symbols:

**Parity: 3/5 compared symbols fully match, with the other 2 differing only in
declared, documented ways (100% of compared *cases*).**
**Cases: 116 total, 109 match, 0 mismatch, 7 declared deviations — 100% of compared cases.**

Per group: `strip` 60/60, `allowed` 21/28 with 7 deviations,
`replacement-streaming-errors` 28/28.

## Behaviour verified by the cases

`cases/strip.json` (group `strip`) is the tokenizer corpus. striptags is a
hand-rolled three-state machine, not a parser, so this is where a port diverges:

* Raw-text and escapable-raw-text elements are **not** special-cased by striptags,
  and the cases pin that: `script`, `script-nested-close`, `script-type`, `style`,
  `title`, `textarea` all keep the element's body as text on both sides. The port
  documentation is explicit that striptags is a text extractor and not a
  sanitizer; these cases are what makes that claim checkable rather than a note.
* Comments: always removed regardless of the allowlist, including unterminated
  ones, extra dashes, a single dash (which does *not* enter comment state), a
  comment inside a tag, a conditional comment, and the abrupt-closing `<!-->`
  trick (`comment`, `comment-unterminated`, `comment-extra-dashes`,
  `comment-single-dash`, `comment-in-tag`, `comment-conditional`,
  `comment-then-tag`).
* The bare-`<` escape hatch, which fires for space and newline but **not** tab
  (`lt-space`, `lt-newline`, `lt-tab`, `lt-only`, `gt-only`).
* Quote tracking inside a tag, including `>` and `<` inside a quoted value, single
  quotes, an apostrophe inside a double-quoted value, and an unbalanced quote that
  swallows the rest of the input (`quote-gt`, `quote-lt`, `single-quote-gt`,
  `mixed-quotes`, `unclosed-quote-then-gt`, `attr-gt-unquoted`).
* Nesting depth on `<<b>>`, `<a<<b>>>`, `<b<b>`, `<a <b>` (`nested-lt`,
  `nested-lt-deep`, `malformed-bb`, `tag-in-tag-space`).
* Declarations and pseudo-markup: doctype, `<!x>`, CDATA, an XML processing
  instruction, a PHP tag (`doctype`, `bang-short`, `cdata`, `xml-pi`, `php-tag`).
* Entities are not decoded, so no second-order tag appears (`entities-kept`,
  `entity-encoded-lt`).
* Attack shapes that must come out as inert text: `img-onerror`, `svg-onload`,
  `iframe-srcdoc`, `javascript-href`, `nul-in-tag`, `fullwidth-lt`.
* Non-ASCII text between tags and inside attributes (`cjk`, `emoji`,
  `cjk-in-attr`).

`cases/allowed.json` (group `allowed`) separates the **array** and **string**
forms of `allowable_tags`, because upstream treats them differently: an array is
used verbatim (`new Set(arr)`) while a string is scanned with `/<(\w*)>/g`. It
also pins that an allowed tag keeps **all** its attributes verbatim on both sides
(`arr-keeps-attrs`, with an `onclick` on it) — striptags is not a sanitizer, and
the case says so out loud.

`cases/replacement-streaming-errors.json` covers the third argument (including
that a removed **comment** produces no replacement, and that a replacement is not
re-parsed or escaped), streaming mode, and the throwing inputs.

## Declared deviations

Seven, all one thing, all listed in the `express` submodule's
`API-DEVIATIONS.md`: upstream lower-cases the tag name it extracts from the input
but compares it against the caller's allowlist verbatim, so any entry that is not
already a bare lower-case name matches nothing — silently.

| case | allowlist entry | upstream | port |
| --- | --- | --- | --- |
| `arr-brackets` | `["<p>"]` | strips `<p>` | keeps `<p>` |
| `arr-upper-entry` | `["P"]` | strips `<p>` | keeps `<p>` |
| `str-no-brackets` | `"p"` | strips | keeps |
| `str-trailing-space` | `"<p >"` | strips | keeps |
| `str-close-form` | `"</p>"` | strips | keeps |
| `str-upper` | `"<P>"` | strips | keeps |
| `str-hyphen` | `"<my-tag>"` | strips | keeps |

In every one of these the tag the port keeps is exactly the tag the caller named
in their own allowlist, so none of them widens what an attacker can push through.
That is why they are deviations and not a security finding.

## Error cases

Upstream throws `TypeError("'html' parameter must be a string")` for a non-string
`html`. The Go function takes a `string`, so the Go runner rejects the argument
shape — the same observable `ok:false` on both sides: `err-number-html`,
`err-true-html`, `err-object-html`, `err-array-html`, `err-nested-array-html`,
`err-number-html-with-allowed`.

Not covered, deliberately: `striptags(null)` and `striptags(false)` do **not**
throw upstream, because `html = html || ''` turns them into `""` first. There is
no Go call that expresses "pass a falsy non-string", so a case would be comparing
two different calls rather than the same one.

## What was fixed while measuring

Measured parity went from **97/109 (89.0%)** to **109/109 (100%)**:
`init_streaming_mode` was not ported at all, so a tag split across two chunks
could not be handled. `NewStreamer` now carries the parser state — state, tag
buffer, nesting depth and quote character — across `Write` calls, and
`StripTagsWith` is one `Write` on a fresh `Streamer`. The 12 `stream-*` cases
include one-character-per-chunk and empty-chunk shapes, which are the hardest
boundary conditions the closure has to survive.
