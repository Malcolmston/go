# COVERAGE — `github.com/malcolmston/puppeteer` v0.2.0

## 0. The headline: puppeteer's browser-automation surface is entirely absent

**This port is not a CDP/browser-automation library.** Despite the name it
contains no websocket, no DevTools protocol, no `Evaluate`, no `Screenshot`, no
browser process and no JavaScript engine. It is `net/http` plus an original HTML
tokenizer/DOM builder and an original CSS selector engine. Verified
mechanically against the module cache:

```console
$ M=$(GOWORK=off go env GOMODCACHE)/github.com/malcolmston/puppeteer@v0.2.0
$ grep -rilE 'websocket|\bCDP\b|devtools|Evaluate|Screenshot' --include='*.go' $M
.../puppeteer@v0.2.0/doc.go     # the only hit, and it is the disclaimer:
                                # "no rendering, layout, or painting: … no screenshots"
$ GOWORK=off go list -m all | grep -v go-parity
github.com/malcolmston/puppeteer v0.2.0   # zero dependencies: stdlib only
```

The library's own README agrees: *"A dependency-free Go library cannot run a
browser or execute JavaScript … no script execution, no rendering/layout, no
live DOM."*

Every one of these upstream areas is therefore scored **`missing`** below —
there is nothing on the Go side to compare against:

| upstream area | representative upstream symbols | port |
| --- | --- | --- |
| launch / connect / browser process | `puppeteer.launch`, `puppeteer.connect`, `executablePath`, `defaultArgs`, `Browser.process`, `wsEndpoint`, `disconnect` | missing (`Launch` builds an `http.Client`) |
| JS evaluation | `Page.evaluate`, `evaluateHandle`, `$eval`, `$$eval`, `exposeFunction`, `waitForFunction`, `queryObjects`, `JSHandle.*` | missing |
| screenshot / PDF / screencast | `Page.screenshot`, `pdf`, `createPDFStream`, `screencast`, `ElementHandle.screenshot` | missing |
| input: click / type / keyboard / mouse / touch | `Page.click`, `type`, `tap`, `hover`, `focus`, `select`, `Keyboard.*`, `Mouse.*`, `Touchscreen.*`, `ElementHandle.click/type/press/drag*` | missing |
| viewport & emulation | `setViewport`, `viewport`, `emulate`, `emulateMediaType`, `emulateMediaFeatures`, `emulateCPUThrottling`, `emulateNetworkConditions`, `emulateTimezone`, `emulateIdleState`, `emulateVisionDeficiency`, `setGeolocation`, `setOfflineMode` | missing |
| request interception | `setRequestInterception`, `HTTPRequest.abort/continue/respond/*`, `authenticate`, `setBypassCSP`, `setCacheEnabled` | missing |
| frames | `Page.frames`, `mainFrame`, `waitForFrame`, all 30 `Frame.*` members | missing |
| dialogs | `Dialog.accept/dismiss/message/type/defaultValue` | missing |
| console / errors / events | the whole `EventEmitter` surface (`page.on('console'|'pageerror'|…)`) | missing |
| tracing & coverage | `Tracing.start/stop`, `Coverage.startJSCoverage/…` | missing |
| targets / contexts / workers | `Target.*`, `BrowserContext.*`, `Page.workers`, `createCDPSession` | missing |
| locators (auto-waiting) | all 16 `Locator.*` members | missing |
| DOM mutation | `Page.setContent`, `addScriptTag`, `addStyleTag`, `ElementHandle.uploadFile`, `autofill` | missing |

**The port is a scraper wearing puppeteer's name.** That is the single most
important finding of this harness.

## 1. Why the oracle is not puppeteer

Comparing against npm `puppeteer` would be comparing against a library that
shares only a name: 281 of its 307 members have no counterpart at all, and
exercising the ones that do (`goto`, `content`, `$`, `$$`) through real
puppeteer would require launching Chromium — explicitly out of bounds here (no
browser is launched or downloaded; every HTTP case runs on loopback).

So the oracle is chosen to match the port's **real** surface, ecosystem `node`,
in `parity/puppeteer/node/`:

| oracle | pinned version | what it is the oracle for |
| --- | --- | --- |
| `cheerio` | `1.0.0` (parse5 + css-select + dom-serializer) | HTML tree construction, serialisation, CSS selector matching, and form serialisation (`.serialize()` / `.serializeArray()` implement the HTML form-serialisation algorithm) |
| `tough-cookie` | `5.1.2` | the RFC 6265 cookie jar (the reference implementation Node HTTP clients use) |
| node built-ins | `node v24.18.0` (undici `fetch`, WHATWG `URL`) | status codes, redirect following, relative URL resolution |

`cheerio@1.0.0` is the same oracle version used by `parity/cheerio/`, so the two
reports are directly comparable — and the port's parser/selector engine is a
sibling of the one scored there.

The upstream inventory in §5 was produced from puppeteer's real type
declarations, not from memory or the README:

```console
$ npm install --no-audit --no-fund puppeteer-core@24.18.0   # no browser download
$ node enum.js Page Frame Browser BrowserContext ElementHandle JSHandle Locator \
      Keyboard Mouse Touchscreen HTTPRequest HTTPResponse Target Dialog \
      Coverage Tracing PuppeteerNode Puppeteer
# enum.js brace-matches each `declare class X {` body in
# node_modules/puppeteer-core/lib/types.d.ts and prints its depth-1 members
```

## 2. Usability findings (recorded, not scored)

1. **The selector engine is unreachable without a network round-trip.**
   `Parse()` returns a `*Node`, but nothing exported turns a `*Node` into an
   `*Element` (`wrapElement` is private), `*Node` has no query method, and there
   is no `Page.SetContent`. The only way to run a selector is
   `Browser.NewPage()` → `Page.Goto(url)` over real HTTP. The Go runner
   therefore stands up an `httptest` server and serves every fixture at
   `/f/<name>`; a caller who merely wants to query a string of HTML must do the
   same. This is the port's biggest ergonomic gap.
2. **Submit buttons can never participate in a submission.** `collectFields`
   hard-codes `included = false` for `type` in
   `{submit, button, reset, image, file}`, so the "which button was clicked"
   name/value pair that real submissions carry can only be re-added by hand with
   `Form.Set`. (For a *non-activated* submission this matches the spec, and
   `form-serialize-submit-only` passes.)
3. **`Form.EncType` is decorative.** It is parsed and exposed but
   `BuildRequest` always emits `application/x-www-form-urlencoded`; there is no
   multipart encoder in the module and no file upload is possible.
4. **`WaitForSelector` cannot wait for anything.** With no live DOM it re-fetches
   the URL on a timer, so it only helps when the *server* changes its answer.
5. **`Response.Body` is `[]byte`** while `Page.Content()` is `string`; the two
   can disagree about decoding.

## 3. Normalisation (identical on both sides)

Both runners pass every HTML string through the same `canon()` function — ported
line-for-line between `node/run.js` and `go/run.go` — before comparison:

* tag names and attribute names are lower-cased;
* attributes are sorted by name (so serialisation order is never a divergence);
* a valueless attribute becomes `k=""`; quoting is normalised to `"`;
* the self-closing `/` is dropped; `<!doctype …>` / `<! …>` declarations are lower-cased;
* raw-text element contents (`script`, `style`, `xmp`, `iframe`, `noembed`, `noframes`, `plaintext`) are copied verbatim.

Further rules:

* A fixture that declares itself a document (leading `<!doctype` or `<html`) is
  loaded by cheerio in **document mode** and compared against the port's
  `NormalizedHTML()` (its documented `page.content()` equivalent); every other
  fixture is loaded in **fragment mode** and compared against `HTML()`, because
  neither parser synthesises an `html`/`head`/`body` shell for a fragment.
* A missing attribute is `""` on both sides (the Go API is string-typed, so
  `undefined`/`null` is collapsed to `""` in the node runner).
* `innerTexts` collapses whitespace runs and trims on both sides.
* HTTP results have `http://127.0.0.1:<port>` replaced by `SRV`, since both
  runners bind port 0 on loopback.
* `form.fields` sorts by (name, value) on both sides, so it measures *which*
  controls are included independently of ordering; `form.serialize`,
  `form.request` and `form.submit` deliberately keep each side's natural order,
  which is where the ordering divergence surfaces.
* Both runners serve **identical routes** from their own in-process server
  (`/f/<name>`, `/status/<code>`, `/redirect/*`, `/deep/a/b/page`, `/cookie/*`,
  `/echo`), so an HTTP case compares client behaviour, not server behaviour.

## 4. Findings — every real divergence

### 4.1 HTML parsing and serialisation (22 match / 24 differ)

The port's tokenizer is competent on tags, attribute syntax, void elements and
the common implied end tags, but it implements **none of the HTML tree
construction algorithm's error recovery**:

| # | divergence | cases |
| --- | --- | --- |
| P1 | **No implied `<tbody>`.** `<table><tr>` keeps `<tr>` as a direct child of `<table>`. | `parse-table-implied-tbody`, `parse-table-nested`, `parse-table-unclosed-cells`, `parse-table-mixed`, `tree-table-implied` |
| P2 | **No foster parenting.** A stray `<b>` inside `<table>` stays inside the table instead of moving before it. | `parse-table-foster`, `tree-foster` |
| P3 | **No adoption agency algorithm.** `<b>1<i>2</b>3</i>` loses the reopened `<i>`, giving `…</b>3`; the same happens across a block boundary. | `parse-misnested-format`, `parse-misnested-deep`, `tree-misnested-deep` |
| P4 | **`<table>` does not close an open `<p>`**, and no empty `<p>` is reopened after the table. | `parse-select-in-p`, `tree-select-in-p` |
| P5 | **`<col>` outside a table is kept** as an element; the spec parser drops it. | `parse-void-elements`, `tree-void` |
| P6 | **`<span/>` is treated as self-closing.** Following text becomes a *sibling* of the span, not its child. | `parse-self-closing-nonvoid` |
| P7 | **Duplicate attributes are both kept** — the port serialises `id="first" id="second"`; the spec keeps only the first. | `parse-attr-dup` |
| P8 | **`<textarea>`/`<title>` content is not RCDATA on output.** The port emits `<textarea><b>hi</b>&</textarea>`, so the text of a `<textarea>` re-parses as markup. | `parse-raw-text` |
| P9 | **`&` is serialised raw instead of `&amp;`.** `<title>Doc &amp; Title</title>` round-trips as `<title>Doc & Title</title>`. This is a correctness *and* injection-safety bug: any `&` (or `<` in a text node coming from an entity) survives serialisation undoubled. | `parse-full-document`, `http-page-html` |
| P10 | **Entity handling differs three ways**: `&notanentity;` is not the legacy `¬` + `anentity;`, bare `&amp` (no semicolon) is not decoded, and a decoded U+00A0 is re-emitted literally rather than as `&nbsp;`. | `parse-entities`, `text-entities`, `parse-nbsp-text` |
| P11 | **Bogus comments keep their leading `!`**: `<![CDATA[x]]>` becomes `<!--![CDATA[x]]-->` instead of `<!--[CDATA[x]]-->`; same for `<!bogus>`. | `parse-cdata-bogus` |
| P12 | **A pre-`<html>` comment is moved into `<body>`** by `NormalizedHTML()`; the spec keeps it at document level. The raw tree also has no `html`/`head`/`body` at all for such a document. | `parse-comments-doctype`, `tree-comments-doctype` |

Matching, for the record: void element set (bar `<col>`), unquoted/single-quoted/
valueless/upper-case attributes, tag-name case folding, implied `</p>`/`</li>`/
`</dt>`/`</dd>`, stray end tags ignored, `script`/`style` raw text, comments and
doctype nodes, unclosed-at-EOF recovery, empty input, text-only input, a
complete document's structure, and `Page.Title`.

### 4.2 CSS selector semantics (84 match / 13 differ)

The selector engine is the strongest part of the port. **Everything below
matched**: all seven attribute operators (`[a]`, `=`, `^=`, `$=`, `*=`, `~=`,
`|=`), the Selectors 4 case-insensitivity flag `[title=hello i]`,
case-insensitive attribute *names*, the empty-operand rule (`[a*=""]` matches
nothing), all four combinators (descendant, `>`, `+`, `~`), selector lists,
`*`, `#id`, `.class`, compound selectors, arbitrary whitespace, all four
`:nth-*` families with the full An+B microsyntax (`odd`, `even`, `2n+1`,
`-n+3`, `3n`, `0`, out-of-range, inner whitespace), `:first-child`,
`:last-child`, `:only-child`, `:first-of-type`, `:last-of-type`,
`:only-of-type`, `:root`, `:not()` including selector lists inside it and
nesting (`:not(:first-child)`), and complex selectors passed to
`Element.Matches` / `Element.Closest`.

| # | divergence | cases |
| --- | --- | --- |
| S1 | **`:has()` is not implemented** — the port errors with `unsupported pseudo-class :has`. This is standard CSS and cheerio supports it. | `pseudo-has`, `pseudo-has-child` |
| S2 | **No form-state pseudo-classes**: `:checked`, `:disabled`, `:enabled`, `:selected` all error. They need no JS — they are attribute-derived — so this is a real gap. | `pseudo-checked`, `pseudo-disabled`, `pseudo-enabled`, `pseudo-selected` |
| S3 | **`:empty` uses the Selectors 4 relaxation**: a whitespace-only element matches in the port and not in css-select. Documented in the port as deliberate (`Element.IsEmpty`), but it is still a behavioural difference. | `pseudo-empty`, `pseudo-empty-whitespace` |
| S4 | **cheerio's jQuery extensions are absent**: `:contains()`, `:first`. Expected — they are not CSS — and listed only for completeness. | `pseudo-contains`, `pseudo-first-positional` |
| S5 | **The port is stricter than css-select on malformed selectors**: it rejects `""`, `"div >"` and `"[a!=b]"`, which cheerio accepts (returning an empty set, and treating `!=` as a non-standard operator). Rejecting these is arguably *more* correct; it is still a divergence. The port and cheerio agree in rejecting `:totally-unknown`, `"["`, `":nth-child(abc)"`, `"div,"` and `"div,,p"`. | `compile-empty`, `compile-dangling-combinator`, `compile-bad-attr-op` |

### 4.3 HTTP and navigation (22 match / 2 differ)

Almost everything here works. Matching: `200`/`201`/`404`/`500`/`204` (and a
4xx/5xx is correctly *not* an error), `301` and `302` following, a redirect
with an **absolute**, **root-relative** and **`../`-relative** `Location`, a
three-hop chain, `FollowRedirects: false` (status and `Location` preserved),
relative `Goto` resolution (`../other`, `sibling`, `/final`), `Page.URL` after
redirects, cookie jar round-trip, two `Set-Cookie` headers on one response,
`Path=` scoping in both directions, no cookie leakage before a `Set-Cookie`,
verbatim query-string preservation, `Page.Content` (raw body) and `Page.Title`.

| # | divergence | cases |
| --- | --- | --- |
| H1 | **`Page.Links()` omits `area[href]`.** `document.links` (and every scraper's expectation) includes `<area href>`; the port only walks `a[href]`. De-duplication and absolute resolution are otherwise correct. | `http-links-resolved` |
| H2 | `Page.HTML()` re-serialisation loses `&amp;` — the same bug as P9. | `http-page-html` |

### 4.4 Forms (13 match / 17 differ)

| # | divergence | cases |
| --- | --- | --- |
| F1 | **Field order is alphabetical, not document order.** `Form.Values()` is a `url.Values` map and `Encode()` sorts keys, so `zzz=3&aaa=1&mmm=2` is submitted as `aaa=1&mmm=2&zzz=3`. Real submissions are in document order; servers that care (signature verification, ordered lists) will see different bytes. Affects the body, the GET query string, and the bytes the server actually receives. | `form-serialize-{basic,get,order,novalue}`, `form-request-{basic,get,order,novalue}`, `form-submit-{basic,get,order,novalue}` |
| F2 | **`enctype="multipart/form-data"` is ignored.** The port sends `application/x-www-form-urlencoded` regardless of the declared enctype, and there is no multipart encoder anywhere in the module — so a file-upload form cannot be submitted at all. | `form-request-multipart` |
| F3 | **`<select multiple>` contributes only one pair.** The port's `selectedOptionValue` returns a single value, so two selected options serialise as `multi=m1`; the spec (and cheerio) produce one pair per selected option. *Note:* on the value axis the port is the correct one — it uses the `value` attribute (`m1`), whereas cheerio@1.0.0's `select.val()` returns the option's **text** (`M1`), a known cheerio quirk also recorded in `parity/cheerio/parity.json`. The multiplicity gap is the port's. | `form-serialize-select-multiple`, `form-fields-select-multiple`, `form-request-select-multiple`, `form-submit-select-multiple` |

Matching: which controls are included (text, hidden, checked checkbox,
unchecked checkbox excluded, checked radio only, `disabled` excluded, nameless
control excluded, `textarea` content, `select` selected option, `<button>`
excluded, `submit`/`reset`/`file` excluded), controls with no `value`
(`empty=`, `cb=on`, first-option fallback), `method` upper-casing and the
GET/POST split, merging pairs into an action that already has a query string,
`action`-less forms submitting to the document URL, the `Content-Type` for
urlencoded POSTs, and the end-to-end submission for every urlencoded fixture.

## 5. Upstream puppeteer API inventory (307 symbols)

Every member of every public class in `puppeteer-core@24.18.0`'s
`lib/types.d.ts`, produced by the command in §1.
| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Page.$` | `puppeteer.Page.QuerySelector` | differs | matches-*, closest-* | selector engine gaps, see Findings |
| `Page.$$` | `puppeteer.Page.QuerySelectorAll` | differs | all sel-*/attr-*/nth-*/pseudo-* | selector engine gaps, see Findings |
| `Page.$$eval` | — | missing | — |  |
| `Page.$eval` | — | missing | — |  |
| `Page.accessibility` | — | missing | — |  |
| `Page.addScriptTag` | — | missing | — |  |
| `Page.addStyleTag` | — | missing | — |  |
| `Page.authenticate` | — | missing | — |  |
| `Page.bringToFront` | — | missing | — |  |
| `Page.browser` | — | missing | — |  |
| `Page.browserContext` | — | missing | — |  |
| `Page.click` | — | missing | — |  |
| `Page.close` | — | missing | — |  |
| `Page.content` | `puppeteer.Page.Content` | match | http-page-content | Go returns the raw body; NormalizedHTML is the closer analogue of page.content() |
| `Page.cookies` | `puppeteer.Page.Cookies` | untested | — | jar behaviour is covered through Goto instead |
| `Page.coverage` | — | missing | — |  |
| `Page.createCDPSession` | — | missing | — |  |
| `Page.createPDFStream` | — | missing | — |  |
| `Page.deleteCookie` | — | missing | — |  |
| `Page.emulate` | — | missing | — |  |
| `Page.emulateCPUThrottling` | — | missing | — |  |
| `Page.emulateIdleState` | — | missing | — |  |
| `Page.emulateMediaFeatures` | — | missing | — |  |
| `Page.emulateMediaType` | — | missing | — |  |
| `Page.emulateNetworkConditions` | — | missing | — |  |
| `Page.emulateTimezone` | — | missing | — |  |
| `Page.emulateVisionDeficiency` | — | missing | — |  |
| `Page.evaluate` | — | missing | — |  |
| `Page.evaluateHandle` | — | missing | — |  |
| `Page.exposeFunction` | — | missing | — |  |
| `Page.focus` | — | missing | — |  |
| `Page.frames` | — | missing | — |  |
| `Page.getDefaultNavigationTimeout` | — | missing | — |  |
| `Page.getDefaultTimeout` | — | missing | — |  |
| `Page.goBack` | `puppeteer.Page.GoBack` | untested | — |  |
| `Page.goForward` | `puppeteer.Page.GoForward` | untested | — |  |
| `Page.goto` | `puppeteer.Page.Goto / GotoContext` | match | http-status-*, http-redirect-*, http-relative-goto-* | HTTP only: no lifecycle events, no subresources, no JS |
| `Page.hover` | — | missing | — |  |
| `Page.isClosed` | — | missing | — |  |
| `Page.isDragInterceptionEnabled` | — | missing | — |  |
| `Page.isJavaScriptEnabled` | — | missing | — |  |
| `Page.isServiceWorkerBypassed` | — | missing | — |  |
| `Page.keyboard` | — | missing | — |  |
| `Page.locator` | — | missing | — |  |
| `Page.mainFrame` | — | missing | — |  |
| `Page.metrics` | — | missing | — |  |
| `Page.mouse` | — | missing | — |  |
| `Page.pdf` | — | missing | — |  |
| `Page.queryObjects` | — | missing | — |  |
| `Page.reload` | `puppeteer.Page.Reload` | untested | — |  |
| `Page.removeExposedFunction` | — | missing | — |  |
| `Page.removeScriptToEvaluateOnNewDocument` | — | missing | — |  |
| `Page.screencast` | — | missing | — |  |
| `Page.screenshot` | — | missing | — |  |
| `Page.select` | — | missing | — |  |
| `Page.setBypassCSP` | — | missing | — |  |
| `Page.setBypassServiceWorker` | — | missing | — |  |
| `Page.setCacheEnabled` | — | missing | — |  |
| `Page.setContent` | — | missing | — |  |
| `Page.setCookie` | — | missing | — |  |
| `Page.setDefaultNavigationTimeout` | — | missing | — |  |
| `Page.setDefaultTimeout` | — | missing | — |  |
| `Page.setDragInterception` | — | missing | — |  |
| `Page.setExtraHTTPHeaders` | `puppeteer.Page.SetExtraHTTPHeaders` | untested | — |  |
| `Page.setGeolocation` | — | missing | — |  |
| `Page.setJavaScriptEnabled` | — | missing | — |  |
| `Page.setOfflineMode` | — | missing | — |  |
| `Page.setRequestInterception` | — | missing | — |  |
| `Page.setUserAgent` | `puppeteer.Page.SetUserAgent` | untested | — |  |
| `Page.setViewport` | — | missing | — |  |
| `Page.tap` | — | missing | — |  |
| `Page.target` | — | missing | — |  |
| `Page.title` | `puppeteer.Page.Title` | match | title-*, http-page-title |  |
| `Page.touchscreen` | — | missing | — |  |
| `Page.tracing` | — | missing | — |  |
| `Page.type` | — | missing | — |  |
| `Page.url` | `puppeteer.Page.URL` | match | http-redirect-*, http-relative-goto-* |  |
| `Page.viewport` | — | missing | — |  |
| `Page.waitForDevicePrompt` | — | missing | — |  |
| `Page.waitForFileChooser` | — | missing | — |  |
| `Page.waitForFrame` | — | missing | — |  |
| `Page.waitForFunction` | — | missing | — |  |
| `Page.waitForNavigation` | — | missing | — |  |
| `Page.waitForNetworkIdle` | — | missing | — |  |
| `Page.waitForRequest` | — | missing | — |  |
| `Page.waitForResponse` | — | missing | — |  |
| `Page.waitForSelector` | `puppeteer.Page.WaitForSelector` | untested | — | re-fetches the URL on a timer; it cannot observe a DOM change |
| `Page.workers` | — | missing | — |  |
| `Frame.$` | — | missing | — |  |
| `Frame.$$` | — | missing | — |  |
| `Frame.$$eval` | — | missing | — |  |
| `Frame.$eval` | — | missing | — |  |
| `Frame.addScriptTag` | — | missing | — |  |
| `Frame.addStyleTag` | — | missing | — |  |
| `Frame.childFrames` | — | missing | — |  |
| `Frame.click` | — | missing | — |  |
| `Frame.content` | — | missing | — |  |
| `Frame.detached` | — | missing | — |  |
| `Frame.evaluate` | — | missing | — |  |
| `Frame.evaluateHandle` | — | missing | — |  |
| `Frame.focus` | — | missing | — |  |
| `Frame.frameElement` | — | missing | — |  |
| `Frame.goto` | — | missing | — |  |
| `Frame.hover` | — | missing | — |  |
| `Frame.isDetached` | — | missing | — |  |
| `Frame.locator` | — | missing | — |  |
| `Frame.name` | — | missing | — |  |
| `Frame.page` | — | missing | — |  |
| `Frame.parentFrame` | — | missing | — |  |
| `Frame.select` | — | missing | — |  |
| `Frame.setContent` | — | missing | — |  |
| `Frame.tap` | — | missing | — |  |
| `Frame.title` | — | missing | — |  |
| `Frame.type` | — | missing | — |  |
| `Frame.url` | — | missing | — |  |
| `Frame.waitForFunction` | — | missing | — |  |
| `Frame.waitForNavigation` | — | missing | — |  |
| `Frame.waitForSelector` | — | missing | — |  |
| `Browser.browserContexts` | — | missing | — |  |
| `Browser.close` | `puppeteer.Browser.Close` | untested | — | closes idle connections only |
| `Browser.connected` | — | missing | — |  |
| `Browser.cookies` | `puppeteer.Browser.Cookies` | untested | — |  |
| `Browser.createBrowserContext` | — | missing | — |  |
| `Browser.debugInfo` | — | missing | — |  |
| `Browser.defaultBrowserContext` | — | missing | — |  |
| `Browser.deleteCookie` | — | missing | — |  |
| `Browser.disconnect` | — | missing | — |  |
| `Browser.installExtension` | — | missing | — |  |
| `Browser.isConnected` | — | missing | — |  |
| `Browser.newPage` | `puppeteer.Browser.NewPage` | match | every case |  |
| `Browser.pages` | — | missing | — |  |
| `Browser.process` | — | missing | — |  |
| `Browser.setCookie` | `puppeteer.Browser.SetCookies` | untested | — |  |
| `Browser.target` | — | missing | — |  |
| `Browser.targets` | — | missing | — |  |
| `Browser.uninstallExtension` | — | missing | — |  |
| `Browser.userAgent` | `puppeteer.Browser.UserAgent` | untested | — |  |
| `Browser.version` | — | missing | — |  |
| `Browser.waitForTarget` | — | missing | — |  |
| `Browser.wsEndpoint` | — | missing | — |  |
| `BrowserContext.browser` | — | missing | — |  |
| `BrowserContext.clearPermissionOverrides` | — | missing | — |  |
| `BrowserContext.close` | — | missing | — |  |
| `BrowserContext.closed` | — | missing | — |  |
| `BrowserContext.cookies` | — | missing | — |  |
| `BrowserContext.deleteCookie` | — | missing | — |  |
| `BrowserContext.id` | — | missing | — |  |
| `BrowserContext.newPage` | — | missing | — |  |
| `BrowserContext.overridePermissions` | — | missing | — |  |
| `BrowserContext.pages` | — | missing | — |  |
| `BrowserContext.setCookie` | — | missing | — |  |
| `BrowserContext.targets` | — | missing | — |  |
| `BrowserContext.waitForTarget` | — | missing | — |  |
| `ElementHandle.$` | `puppeteer.Element.QuerySelector` | differs | matches-*, closest-* | same selector engine |
| `ElementHandle.$$` | `puppeteer.Element.QuerySelectorAll` | differs | sel-* | same selector engine |
| `ElementHandle.$$eval` | — | missing | — |  |
| `ElementHandle.$eval` | — | missing | — |  |
| `ElementHandle.asLocator` | — | missing | — |  |
| `ElementHandle.autofill` | — | missing | — |  |
| `ElementHandle.backendNodeId` | — | missing | — |  |
| `ElementHandle.boundingBox` | — | missing | — |  |
| `ElementHandle.boxModel` | — | missing | — |  |
| `ElementHandle.click` | — | missing | — |  |
| `ElementHandle.clickablePoint` | — | missing | — |  |
| `ElementHandle.contentFrame` | — | missing | — |  |
| `ElementHandle.drag` | — | missing | — |  |
| `ElementHandle.dragAndDrop` | — | missing | — |  |
| `ElementHandle.dragEnter` | — | missing | — |  |
| `ElementHandle.dragOver` | — | missing | — |  |
| `ElementHandle.drop` | — | missing | — |  |
| `ElementHandle.focus` | — | missing | — |  |
| `ElementHandle.frame` | — | missing | — |  |
| `ElementHandle.hover` | — | missing | — |  |
| `ElementHandle.isHidden` | — | missing | — |  |
| `ElementHandle.isIntersectingViewport` | — | missing | — |  |
| `ElementHandle.isVisible` | — | missing | — |  |
| `ElementHandle.press` | — | missing | — |  |
| `ElementHandle.screenshot` | — | missing | — |  |
| `ElementHandle.scrollIntoView` | — | missing | — |  |
| `ElementHandle.select` | — | missing | — |  |
| `ElementHandle.tap` | — | missing | — |  |
| `ElementHandle.toElement` | — | missing | — |  |
| `ElementHandle.touchEnd` | — | missing | — |  |
| `ElementHandle.touchMove` | — | missing | — |  |
| `ElementHandle.touchStart` | — | missing | — |  |
| `ElementHandle.type` | — | missing | — |  |
| `ElementHandle.uploadFile` | — | missing | — |  |
| `ElementHandle.waitForSelector` | — | missing | — |  |
| `JSHandle.asElement` | — | missing | — |  |
| `JSHandle.dispose` | — | missing | — |  |
| `JSHandle.evaluate` | — | missing | — |  |
| `JSHandle.evaluateHandle` | — | missing | — |  |
| `JSHandle.getProperties` | — | missing | — |  |
| `JSHandle.getProperty` | — | missing | — |  |
| `JSHandle.jsonValue` | — | missing | — |  |
| `JSHandle.move` | — | missing | — |  |
| `JSHandle.remoteObject` | — | missing | — |  |
| `JSHandle.toString` | — | missing | — |  |
| `Locator.click` | — | missing | — |  |
| `Locator.clone` | — | missing | — |  |
| `Locator.fill` | — | missing | — |  |
| `Locator.filter` | — | missing | — |  |
| `Locator.hover` | — | missing | — |  |
| `Locator.map` | — | missing | — |  |
| `Locator.race` | — | missing | — |  |
| `Locator.scroll` | — | missing | — |  |
| `Locator.setEnsureElementIsInTheViewport` | — | missing | — |  |
| `Locator.setTimeout` | — | missing | — |  |
| `Locator.setVisibility` | — | missing | — |  |
| `Locator.setWaitForEnabled` | — | missing | — |  |
| `Locator.setWaitForStableBoundingBox` | — | missing | — |  |
| `Locator.timeout` | — | missing | — |  |
| `Locator.wait` | — | missing | — |  |
| `Locator.waitHandle` | — | missing | — |  |
| `Keyboard.down` | — | missing | — |  |
| `Keyboard.press` | — | missing | — |  |
| `Keyboard.sendCharacter` | — | missing | — |  |
| `Keyboard.type` | — | missing | — |  |
| `Keyboard.up` | — | missing | — |  |
| `Mouse.click` | — | missing | — |  |
| `Mouse.down` | — | missing | — |  |
| `Mouse.drag` | — | missing | — |  |
| `Mouse.dragAndDrop` | — | missing | — |  |
| `Mouse.dragEnter` | — | missing | — |  |
| `Mouse.dragOver` | — | missing | — |  |
| `Mouse.drop` | — | missing | — |  |
| `Mouse.move` | — | missing | — |  |
| `Mouse.reset` | — | missing | — |  |
| `Mouse.up` | — | missing | — |  |
| `Mouse.wheel` | — | missing | — |  |
| `Touchscreen.tap` | — | missing | — |  |
| `Touchscreen.touchEnd` | — | missing | — |  |
| `Touchscreen.touchMove` | — | missing | — |  |
| `Touchscreen.touchStart` | — | missing | — |  |
| `HTTPRequest.abort` | — | missing | — |  |
| `HTTPRequest.abortErrorReason` | — | missing | — |  |
| `HTTPRequest.client` | — | missing | — |  |
| `HTTPRequest.continue` | — | missing | — |  |
| `HTTPRequest.continueRequestOverrides` | — | missing | — |  |
| `HTTPRequest.enqueueInterceptAction` | — | missing | — |  |
| `HTTPRequest.failure` | — | missing | — |  |
| `HTTPRequest.fetchPostData` | — | missing | — |  |
| `HTTPRequest.finalizeInterceptions` | — | missing | — |  |
| `HTTPRequest.frame` | — | missing | — |  |
| `HTTPRequest.hasPostData` | — | missing | — |  |
| `HTTPRequest.headers` | — | missing | — |  |
| `HTTPRequest.initiator` | — | missing | — |  |
| `HTTPRequest.interceptResolutionState` | — | missing | — |  |
| `HTTPRequest.isInterceptResolutionHandled` | — | missing | — |  |
| `HTTPRequest.isNavigationRequest` | — | missing | — |  |
| `HTTPRequest.method` | — | missing | — |  |
| `HTTPRequest.postData` | — | missing | — |  |
| `HTTPRequest.redirectChain` | — | missing | — |  |
| `HTTPRequest.resourceType` | — | missing | — |  |
| `HTTPRequest.respond` | — | missing | — |  |
| `HTTPRequest.response` | — | missing | — |  |
| `HTTPRequest.responseForRequest` | — | missing | — |  |
| `HTTPRequest.url` | — | missing | — |  |
| `HTTPResponse.buffer` | — | missing | — |  |
| `HTTPResponse.content` | — | missing | — |  |
| `HTTPResponse.frame` | — | missing | — |  |
| `HTTPResponse.fromCache` | — | missing | — |  |
| `HTTPResponse.fromServiceWorker` | — | missing | — |  |
| `HTTPResponse.headers` | `puppeteer.Response.Header` | match | http-redirect-no-follow |  |
| `HTTPResponse.json` | — | missing | — |  |
| `HTTPResponse.ok` | `puppeteer.Response.OK` | match | http-status-* |  |
| `HTTPResponse.remoteAddress` | — | missing | — |  |
| `HTTPResponse.request` | — | missing | — |  |
| `HTTPResponse.securityDetails` | — | missing | — |  |
| `HTTPResponse.status` | `puppeteer.Response.StatusCode` | match | http-status-* |  |
| `HTTPResponse.statusText` | — | missing | — |  |
| `HTTPResponse.text` | `puppeteer.Response.Body` | untested | — | []byte, not a decoded string |
| `HTTPResponse.timing` | — | missing | — |  |
| `HTTPResponse.url` | `puppeteer.Response.URL` | match | http-redirect-* |  |
| `Target.asPage` | — | missing | — |  |
| `Target.browser` | — | missing | — |  |
| `Target.browserContext` | — | missing | — |  |
| `Target.createCDPSession` | — | missing | — |  |
| `Target.opener` | — | missing | — |  |
| `Target.page` | — | missing | — |  |
| `Target.type` | — | missing | — |  |
| `Target.url` | — | missing | — |  |
| `Target.worker` | — | missing | — |  |
| `Dialog.accept` | — | missing | — |  |
| `Dialog.defaultValue` | — | missing | — |  |
| `Dialog.dismiss` | — | missing | — |  |
| `Dialog.message` | — | missing | — |  |
| `Dialog.type` | — | missing | — |  |
| `Coverage.startCSSCoverage` | — | missing | — |  |
| `Coverage.startJSCoverage` | — | missing | — |  |
| `Coverage.stopCSSCoverage` | — | missing | — |  |
| `Coverage.stopJSCoverage` | — | missing | — |  |
| `Tracing.start` | — | missing | — |  |
| `Tracing.stop` | — | missing | — |  |
| `PuppeteerNode.connect` | — | missing | — |  |
| `PuppeteerNode.defaultArgs` | — | missing | — |  |
| `PuppeteerNode.defaultBrowser` | — | missing | — |  |
| `PuppeteerNode.executablePath` | — | missing | — |  |
| `PuppeteerNode.lastLaunchedBrowser` | — | missing | — |  |
| `PuppeteerNode.launch` | `puppeteer.Launch` | differs | every case | launches no browser process: returns an http.Client + cookie jar |
| `PuppeteerNode.product` | — | missing | — |  |
| `PuppeteerNode.trimCache` | — | missing | — |  |
| `Puppeteer.clearCustomQueryHandlers` | — | missing | — |  |
| `Puppeteer.connect` | — | missing | — |  |
| `Puppeteer.customQueryHandlerNames` | — | missing | — |  |
| `Puppeteer.registerCustomQueryHandler` | — | missing | — |  |
| `Puppeteer.unregisterCustomQueryHandler` | — | missing | — |  |

**Counts over puppeteer's real API:** 307 symbols — **9 match, 5 differs,
12 untested, 281 missing, 0 extra**.

* **Parity over puppeteer's real API (denominator 307): 9 / 307 = 2.93 %.**
* Parity over the puppeteer symbols that have any counterpart at all and were
  scored (denominator 14 = match + differs): 9 / 14 = 64.3 %.
* Only 26 / 307 = 8.5 % of puppeteer's API has a Go counterpart of any kind.

## 6. The port's own exported API (81 funcs/methods)

Produced with `GOWORK=off go doc -all github.com/malcolmston/puppeteer`.
"GO-ONLY" in the note means there is no puppeteer counterpart (`extra` in
harness terms); such a symbol is scored against the *oracle for what it does*
(cheerio/fetch/tough-cookie), not against puppeteer.
| Go symbol | upstream counterpart | status | cases | note |
| --- | --- | --- | --- | --- |
| `puppeteer.Launch` | `puppeteer.launch` | differs | every case | no browser process; builds an http.Client + cookiejar |
| `puppeteer.Browser.Close` | `browser.close` | untested | — |  |
| `puppeteer.Browser.Cookies` | `browser.cookies` | untested | — |  |
| `puppeteer.Browser.NewPage` | `browser.newPage` | match | every case |  |
| `puppeteer.Browser.SetCookies` | `browser.setCookie` | untested | — |  |
| `puppeteer.Browser.SetExtraHTTPHeaders` | `page.setExtraHTTPHeaders` | untested | — |  |
| `puppeteer.Browser.SetUserAgent` | `browser/page.setUserAgent` | untested | — |  |
| `puppeteer.Browser.UserAgent` | `browser.userAgent` | untested | — |  |
| `puppeteer.Element.Ancestors` | — | untested | — | GO-ONLY |
| `puppeteer.Element.Attr` | — | untested | — | GO-ONLY |
| `puppeteer.Element.AttrOr` | — | match | attr-*, closest-* | GO-ONLY accessor |
| `puppeteer.Element.AttributeNames` | — | untested | — | GO-ONLY |
| `puppeteer.Element.Attributes` | — | untested | — | GO-ONLY |
| `puppeteer.Element.Children` | — | untested | — | GO-ONLY |
| `puppeteer.Element.ClassList` | — | differs | sel-classes, pseudo-empty | GO-ONLY accessor |
| `puppeteer.Element.Closest` | — | match | closest-* |  |
| `puppeteer.Element.Dataset` | — | untested | — | GO-ONLY |
| `puppeteer.Element.HasAttribute` | — | untested | — | GO-ONLY |
| `puppeteer.Element.HasClass` | — | untested | — | GO-ONLY |
| `puppeteer.Element.ID` | — | untested | — | GO-ONLY |
| `puppeteer.Element.InnerHTML` | — | match | sel-inner | GO-ONLY accessor |
| `puppeteer.Element.InnerText` | — | match | sel-innertexts | GO-ONLY accessor |
| `puppeteer.Element.IsEmpty` | — | differs | pseudo-empty-whitespace | GO-ONLY; Selectors 4 :empty relaxation |
| `puppeteer.Element.Matches` | — | match | matches-* |  |
| `puppeteer.Element.Next` | — | untested | — | GO-ONLY |
| `puppeteer.Element.NextAll` | — | untested | — | GO-ONLY |
| `puppeteer.Element.Node` | — | differs | tree-* | GO-ONLY raw node access |
| `puppeteer.Element.OuterHTML` | — | match | sel-outer | GO-ONLY accessor |
| `puppeteer.Element.Parent` | — | untested | — | GO-ONLY |
| `puppeteer.Element.Prev` | — | untested | — | GO-ONLY |
| `puppeteer.Element.PrevAll` | — | untested | — | GO-ONLY |
| `puppeteer.Element.QuerySelector` | `elementHandle.$` | untested | — | reached only through Page in these cases |
| `puppeteer.Element.QuerySelectorAll` | `elementHandle.$$` | untested | — | reached only through Page in these cases |
| `puppeteer.Element.Siblings` | — | untested | — | GO-ONLY |
| `puppeteer.Element.TagName` | — | match | sel-*(tags) | GO-ONLY accessor |
| `puppeteer.Element.TextContent` | — | match | sel-*(texts) | GO-ONLY accessor |
| `puppeteer.Form.BuildRequest` | — | differs | form-request-* | GO-ONLY; always urlencoded |
| `puppeteer.Form.FieldNames` | — | untested | — | GO-ONLY |
| `puppeteer.Form.Get` | — | untested | — | GO-ONLY |
| `puppeteer.Form.Set` | — | untested | — | GO-ONLY |
| `puppeteer.Form.Submit` | — | differs | form-submit-* | GO-ONLY |
| `puppeteer.Form.SubmitContext` | — | untested | — | GO-ONLY |
| `puppeteer.Form.Values` | — | differs | form-serialize-*, form-fields-* | GO-ONLY; sorted, not document order |
| `puppeteer.Parse` | — | untested | — | GO-ONLY. Its *Node cannot be queried: no exported way to wrap it in an Element and no SetContent, so the parser is only reachable via a real navigation |
| `puppeteer.Node.AppendChild` | — | untested | — | GO-ONLY |
| `puppeteer.Page.CanGoBack` | — | untested | — | GO-ONLY |
| `puppeteer.Page.CanGoForward` | — | untested | — | GO-ONLY |
| `puppeteer.Page.Content` | `page.content` | match | http-page-content | raw body |
| `puppeteer.Page.Cookies` | `page.cookies` | untested | — |  |
| `puppeteer.Page.Count` | — | untested | — | GO-ONLY |
| `puppeteer.Page.Document` | — | differs | tree-* | GO-ONLY raw DOM access; tree construction differs |
| `puppeteer.Page.FillForm` | — | untested | — | GO-ONLY |
| `puppeteer.Page.FormBySelector` | — | match | form-* | GO-ONLY; exercised by every form case |
| `puppeteer.Page.Forms` | — | untested | — | GO-ONLY |
| `puppeteer.Page.GetAttribute` | — | untested | — | GO-ONLY |
| `puppeteer.Page.GoBack` | `page.goBack` | untested | — |  |
| `puppeteer.Page.GoBackContext` | `page.goBack` | untested | — |  |
| `puppeteer.Page.GoForward` | `page.goForward` | untested | — |  |
| `puppeteer.Page.GoForwardContext` | `page.goForward` | untested | — |  |
| `puppeteer.Page.Goto` | `page.goto` | match | http-* | HTTP axis only |
| `puppeteer.Page.GotoContext` | `page.goto` | untested | — |  |
| `puppeteer.Page.HTML` | `page.content` | differs | parse-*, http-page-html | re-serialised tree; entity re-escaping differs |
| `puppeteer.Page.History` | — | untested | — | GO-ONLY |
| `puppeteer.Page.Images` | — | untested | — | GO-ONLY |
| `puppeteer.Page.Links` | — | differs | http-links-resolved | GO-ONLY; omits area[href] |
| `puppeteer.Page.MetaContent` | — | untested | — | GO-ONLY |
| `puppeteer.Page.Metas` | — | untested | — | GO-ONLY |
| `puppeteer.Page.NormalizedHTML` | `page.content` | differs | parse-full-document, parse-comments-doctype | GO-ONLY name; comment placement differs |
| `puppeteer.Page.QuerySelector` | `page.$` | differs | matches-*, closest-* |  |
| `puppeteer.Page.QuerySelectorAll` | `page.$$` | differs | sel-*, attr-*, nth-*, pseudo-*, compile-* |  |
| `puppeteer.Page.Reload` | `page.reload` | untested | — |  |
| `puppeteer.Page.ReloadContext` | `page.reload` | untested | — |  |
| `puppeteer.Page.Scripts` | — | untested | — | GO-ONLY |
| `puppeteer.Page.SetExtraHTTPHeaders` | `page.setExtraHTTPHeaders` | untested | — |  |
| `puppeteer.Page.SetUserAgent` | `page.setUserAgent` | untested | — |  |
| `puppeteer.Page.Stylesheets` | — | untested | — | GO-ONLY |
| `puppeteer.Page.TextContent` | — | differs | text-* | GO-ONLY (Playwright-shaped); entity decoding differs |
| `puppeteer.Page.Title` | `page.title` | match | title-*, http-page-title |  |
| `puppeteer.Page.URL` | `page.url` | match | http-redirect-*, http-relative-goto-* |  |
| `puppeteer.Page.WaitForSelector` | `page.waitForSelector` | untested | — | polls by re-fetching; not comparable |
| `puppeteer.Response.OK` | `httpResponse.ok` | match | http-status-* |  |

**Counts over the port's own API:** 81 funcs/methods — **15 match, 14 differs,
52 untested**. Symbol-level parity over what was compared (denominator 29):
15 / 29 = 51.7 %.

Types are excluded from that count and listed here for completeness:
`Attribute`, `Browser`, `Element`, `Form`, `LaunchOptions`, `Node`, `NodeType`
(+ `DocumentNode`, `ElementNode`, `TextNode`, `CommentNode`, `DoctypeNode`),
`Page`, `Response`, `WaitForSelectorOptions`, and the constant
`DefaultUserAgent`.

## 7. Score

Regenerated by `go test ./parity/puppeteer/` into `parity.json`.

| group | cases | match | differ |
| --- | --- | --- | --- |
| parsing | 46 | 22 | 24 |
| selectors | 97 | 84 | 13 |
| http | 24 | 22 | 2 |
| forms | 30 | 13 | 17 |
| **total** | **197** | **141** | **56** |

**The two denominators, stated plainly:**

1. **Over what was compared** — the surface the port actually has (HTML parsing
   and serialisation, CSS selector semantics, HTTP navigation, form
   serialisation): **141 / 197 cases = 71.57 %**; at symbol level
   **15 / 29 = 51.7 %** of the port's compared exported API.
2. **Over puppeteer's real API** — all 307 members of `Page`, `Frame`,
   `Browser`, `BrowserContext`, `ElementHandle`, `JSHandle`, `Locator`,
   `Keyboard`, `Mouse`, `Touchscreen`, `HTTPRequest`, `HTTPResponse`, `Target`,
   `Dialog`, `Coverage`, `Tracing` and the top-level `Puppeteer`/`PuppeteerNode`:
   **9 / 307 = 2.93 %**.

0 cases are marked `deviation`: the port ships no `API-DEVIATIONS.md`, so every
difference above is counted as a divergence even where the port is arguably the
more correct of the two (S5, and the value axis of F3).

## 8. Reproducing

```console
$ cd parity/puppeteer/node && npm install     # cheerio@1.0.0, tough-cookie@5.1.2
$ cd .. && GOWORK=off go test ./              # skips (never fails) if node is absent
```

Both runners are deterministic: fixed fixtures, fixed routes, port 0 on
loopback, sorted map keys, no clocks, no network beyond loopback, and one
process per side for the whole run. A throw or panic in either implementation
becomes `ok:false` for that case only.
