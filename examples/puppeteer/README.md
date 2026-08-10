# puppeteer example

A single runnable program that exercises the public API of
[`github.com/malcolmston/puppeteer`](https://github.com/malcolmston/puppeteer)
end to end.

- **Module under test:** `github.com/malcolmston/puppeteer`
- **Resolved version:** `v0.0.0-20260719012943-3386099480be` (pseudo-version;
  the repo has no semver tags). Consumed as a published module — this example
  has **no** `replace` directive and does not reference the sibling source tree.

## Run it

```sh
cd examples/puppeteer
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program is hermetic and self-terminating: it needs **no browser** and **no
outbound network** (only the one-time module download). Every page it visits is
served by an in-process `net/http/httptest` server on loopback, every navigation
is bounded by `LaunchOptions.Timeout`, and every wait is bounded by
`WaitForSelectorOptions.Timeout`. Typical runtime is well under a second.

## What it demonstrates

| Section | API surface |
| --- | --- |
| 0 | Browser/CDP capability probe (see "What needs a real browser" below) |
| 1 | `Launch`, `LaunchOptions{UserAgent, Timeout, Headers, FollowRedirects, Jar, Transport}`, `DefaultUserAgent`, `Browser.UserAgent/SetUserAgent/SetExtraHTTPHeaders` |
| 2 | `Browser.NewPage`, `Page.Goto`, `Page.GotoContext` (incl. relative-URL resolution and a pre-cancelled context), `Response{URL,Status,StatusCode,Header,Body}`, `Response.OK`, `Page.URL/Title/Content` |
| 3 | The whole selector engine: type, `*`, `#id`, `.class`, all seven attribute operators, `>` `+` `~` and descendant combinators, selector lists, and every structural pseudo-class it actually implements (see holes) — plus the pseudo-classes it correctly rejects |
| 4 | `Element`: `TagName`, `ID`, `ClassList`, `HasClass`, `TextContent`, `InnerText`, `InnerHTML`, `OuterHTML`, `Attr`, `AttrOr`, `Attributes`, `AttributeNames`, `HasAttribute`, `Dataset`, `Node`, `Parent`, `Prev`, `Next`, `Siblings`, `PrevAll`, `NextAll`, `Ancestors`, `Children`, `Closest`, `Matches`, `IsEmpty`, element-scoped `QuerySelector`/`QuerySelectorAll` |
| 5 | `Page.Links`, `Images`, `Scripts`, `Stylesheets`, `Metas`, `MetaContent`, `TextContent`, `GetAttribute`, `Count` |
| 6 | `Page.Document`, `Page.HTML` vs `Page.Content`, `Page.NormalizedHTML` on a `<head>`/`<body>`-less document, implicit-`</li>` parser recovery |
| 7 | `Browser.Cookies`, `Browser.SetCookies`, `Page.Cookies` (server `Set-Cookie` captured by the jar and replayed on submit) |
| 8 | `Page.History`, `CanGoBack`, `CanGoForward`, `GoBack`, `GoForward`, `Reload` and all four `*Context` variants |
| 9 | `Page.Forms`, `FormBySelector`, `FillForm`, `Form.{Action,Method,EncType}`, `FieldNames`, `Get`, `Set`, `Values`, `BuildRequest`, `Submit`, `SubmitContext` — for both a POST form and a GET form, with an echo handler proving the exact wire encoding, headers, UA and cookies sent |
| 10 | `Page.WaitForSelector` + `WaitForSelectorOptions` against an endpoint whose response changes across identical requests, plus the bounded timeout path |
| 11 | Redirect policy: `FollowRedirects` default (followed) vs `false` (raw 302 + `Location`) |
| 12 | Guard errors before `Goto`, empty-page accessors, a deliberately hung server bounded by `Timeout`, a refused connection, an unparseable URL |
| 13 | `Parse`, the raw `Node`/`Attribute`/`NodeType` DOM, `Node.AppendChild`, tokenizer behaviour on unquoted/boolean attributes and `<script>` raw text |

## What needs a real browser (and was skipped)

Nothing — and that is the headline finding. **This port contains no browser
automation whatsoever.** Despite the name, it is not a Chrome DevTools Protocol
client: grepping the published source for `websocket`, `CDP`, `devtools`,
`Evaluate`, or `Screenshot` returns zero hits. It is an `net/http` client, an
original HTML tokenizer/parser, and a CSS selector engine. The README and
`doc.go` are upfront about this.

So the following Puppeteer concepts have **no API at all** and are skipped;
section 0 prints a `skipped: no browser available` line and explains why a
browser would not help even if installed:

- launching/connecting to a browser process, `browserWSEndpoint`, CDP sessions,
  targets, browser contexts;
- `page.evaluate` / `evaluateHandle` / exposed functions — no JS engine;
- `page.screenshot`, `page.pdf`, viewport/`setViewport`, device emulation;
- input: `click`, `type`, `hover`, `focus`, `select`, keyboard/mouse/touchscreen;
- layout: bounding boxes, computed styles, visibility (`:visible`, `waitFor`
  with `visible`/`hidden` state);
- live-DOM events: `page.on('request'/'response'/'console')`, request
  interception, frames, dialogs, downloads, workers, tracing, coverage.

## Holes found

Blocking / API-shape problems:

1. **`Parse()`'s output cannot be queried.** `Parse(string) *Node` is exported,
   but the selector engine is reachable only through `Page.QuerySelector*`, and
   a `Page` only ever gets a document from a completed HTTP navigation.
   `compileSelector`, `wrapElement` and `(*selector).queryAll` are unexported,
   `Element` has no exported constructor, and there is no `Page.SetContent`.
   Result: you cannot run a selector over HTML you already have in memory —
   the single most common offline use — without standing up an HTTP server.
   Marked `// HOLE:` in `section13RawParse`.
2. **No `Page.SetContent` / `Page.SetDocument`.** Same root cause as above; also
   makes unit-testing user code against fixture HTML awkward.
3. **`WaitForSelector` takes no `context.Context`.** Every other blocking call
   has a `*Context` twin (`GotoContext`, `ReloadContext`, `GoBackContext`,
   `GoForwardContext`, `SubmitContext`); `WaitForSelector` does not, so it can
   only be bounded by its own `Timeout` field and cannot participate in caller
   cancellation. It also calls `time.Sleep` directly rather than selecting on a
   channel. It is at least never unbounded, so nothing hangs.
4. **README/`doc.go` understate the selector engine.** Both list only
   `:first-child`, `:last-child` and `:nth-child()`. The published code also
   implements `:first-of-type`, `:last-of-type`, `:nth-of-type()`,
   `:nth-last-child()`, `:nth-last-of-type()`, `:only-child`, `:only-of-type`,
   `:empty`, `:root` and `:not()` (including nesting, e.g.
   `li:not(:nth-child(2))`). Section 3 exercises all of them. A user reading the
   docs would needlessly hand-roll these.
5. **No cookie removal.** `Browser.SetCookies` only adds; there is no
   `DeleteCookie`, `ClearCookies`, or jar reset, and the jar is not exposed for
   direct manipulation. `LaunchOptions.Jar` is the only escape hatch (create a
   new browser).
6. **`Element` is entirely read-only.** No `SetAttribute`/`RemoveAttribute`/
   text mutation, and `Node` exposes only `AppendChild` (no `RemoveChild`,
   `InsertBefore`, `SetAttr`). Fine for scraping; blocks DOM-rewriting uses.
7. **No multipart form support.** `Form.EncType` is parsed and reported, but
   `BuildRequest` always emits `application/x-www-form-urlencoded`, silently
   ignoring `multipart/form-data`; there is no file-field concept. A form
   declaring multipart will be submitted incorrectly with no warning.
8. **No way to pick a submit button.** `<input type=submit name=...>` fields are
   collected but always excluded, so multi-submit forms (`name=action`) require
   a manual `Set("action", ...)`.
9. **`Page` history is per-`Page` and cannot be pruned.** `WaitForSelector`'s
   internal re-fetch goes through `Goto`, so each poll appends a new history
   entry — polling 40 times leaves 40 identical history records. (`Reload` is
   correctly exempt via an internal lock; `WaitForSelector` is not.)

Non-blocking observations:

10. `Element.Dataset()` keys are derived from already-lower-cased attribute
    names, so `data-testId` becomes `testid`, not `testId`. Correct per HTML
    (attribute names are case-insensitive), but worth knowing.
11. `Page.Title()` trims surrounding whitespace but does not collapse internal
    runs, unlike `document.title`.
12. `Page.QuerySelector` returns `(nil, nil)` on a miss — the Puppeteer/JS
    convention, but it means every call site needs a nil check that the compiler
    will not enforce. Same for `GoBack`/`GoForward` at the ends of history.
13. `GotoContext`'s error wraps the raw `*url.Error`, so messages leak the full
    URL — fine, but noisy in logs.
14. `LaunchOptions.Timeout` uses `0` for "30s default" and a negative value for
    "no timeout", which is an unusual convention worth a doc comment on every
    call site.

No compile failures, no runtime panics, no hangs, no dependency problems: the
module has zero dependencies (standard library only), fetched cleanly from the
proxy, and the published source is byte-identical to the repo working tree.
