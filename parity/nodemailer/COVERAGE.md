# nodemailer — parity coverage

- **Upstream oracle:** `nodemailer@6.9.16` (pinned in `node/package.json`, installed under `node/node_modules`).
- **Go port under test:** `github.com/malcolmston/nodemailer v0.0.0-20260810111553-311143ed3171`
  (consumed as a published module, no `replace` directive; the repository publishes no
  semver tags, so `GOWORK=off go get github.com/malcolmston/nodemailer@latest` resolves
  to that pseudo-version).
- **Comparable artefact:** the generated MIME message. Upstream produces it with its own
  `streamTransport` (`buffer: true`, `newline: 'windows'`); the Go side produces it with
  `Message.Build`. **No SMTP connection is opened and nothing is sent anywhere.**
- **Cases:** 86 across `cases/addresses.json`, `cases/subject.json`, `cases/bodies.json`,
  `cases/attachments.json`, `cases/inline.json`, `cases/encoding.json`, `cases/headers.json`.
- **Score:** see `parity.json`, rewritten by `go test`.

## How determinism was obtained

MIME output has three sources of nondeterminism; all three are pinned.

| source | upstream | Go |
| --- | --- | --- |
| `Date` | `date: new Date(Date.UTC(2026,0,2,15,4,5))` | `SetDate(time.Date(2026,1,2,15,4,5,0,time.UTC))` |
| `Message-ID` | `messageId: '<parity-fixed@example.com>'` | `SetMessageID("parity-fixed@example.com")` |
| multipart boundaries | random per run | `SetBoundary("PARITYBOUNDARY")`, nested boundaries derived |

Boundaries cannot be made *equal* across implementations (upstream generates
`--_NmP-<hex>-Part_N`, the port derives `PARITYBOUNDARY`, `PARITYBOUNDARY_2`, …), so the
harness post-processes both messages: every distinct `boundary=` token is collected in
order of first appearance and rewritten to `B0`, `B1`, … everywhere it occurs (longest
token first, so `X` cannot corrupt `X_2`). Nesting order is therefore still asserted.

One further knob: upstream stamps an `X-Mailer:` banner that the port does not emit at
all. The node runner passes `xMailer: false` so the comparison is about MIME structure
rather than about a banner. This is the only upstream behaviour the harness suppresses,
and it is recorded as `missing` in the inventory below.

## Canonicalisation

Raw-string comparison is useless here — header order, fold points, encoded-word split
points and quoting style all differ without any semantic difference. Both messages are
therefore parsed into the same canonical tree (`canonical_test.go`) and the trees are
deep-compared. **One parser is used for both sides**, so the comparison can never inherit
a divergence between two different MIME parsers.

Each node is `{headers, contentType, transferEncoding, bodyKind, body, parts, malformed}`:

1. **Headers** are unfolded (continuation lines joined), split at the first colon, and the
   field name canonically cased (`textproto.CanonicalMIMEHeaderKey`). Values are then
   normalised by field grammar:
   - address fields (`From`, `To`, `Cc`, `Bcc`, `Reply-To`, `Sender`) are parsed with a
     tolerant splitter into mailboxes and RFC 5322 groups and re-rendered as
     `Display Name <local@domain>` / `Group:[m1, m2]`, with display names RFC 2047-decoded
     and unquoted and domains lower-cased. That neutralises `"Ada Lovelace" <a@b>` versus
     `Ada Lovelace <a@b>`.
   - `Content-Type` / `Content-Disposition` are parsed into a type plus parameters:
     parameter names lower-cased, quoted values unquoted, RFC 2231 `name*0*=charset''pct`
     continuations reassembled and percent-decoded, RFC 2047 encoded words decoded, and
     parameters sorted by name.
   - `Message-ID`, `In-Reply-To`, `References`, `Date`: whitespace runs collapsed.
   - everything else: RFC 2047-decoded, whitespace runs collapsed. Whitespace between two
     *adjacent* encoded words is dropped (RFC 2047 §6.2), which is what makes the two
     encoders' different encoded-word split points comparable.
   - the header list is then sorted by (name, value), so emission order is not compared.
2. **Bodies** are transfer-decoded (`base64`, `quoted-printable`, or passthrough). Text
   bodies are normalised to LF line endings with trailing newlines trimmed; anything not
   valid printable UTF-8 becomes `base64` of the decoded bytes. So the *decoded payload* is
   compared, not its encoded form — the encoding itself is compared separately as
   `transferEncoding`.
3. **`multipart/*`** nodes are split on their boundary and recursed into, up to depth 8.
4. **The parser never fails.** Anything invalid is appended to that node's `malformed`
   list, and `malformed` is part of the comparison. This is how the port's bare-encoded-word
   parameter defect surfaces as a *difference* instead of a parse error: the parser records
   `unquoted parameter filename contains tspecials: =?utf-8?b?…?=` on the Go side and
   nothing on the upstream side.

### Two metrics

`parity.json` reports two numbers over the same 85 compared cases (86 minus one declared
deviation):

- `parityPercent` — strict equality of the canonical trees. **1 / 85 = 1.18 %.**
- `structuralParityPercent` — the same comparison with `Content-Transfer-Encoding` and
  empty-valued headers masked out. **55 / 85 = 64.71 %.**

The gap is entirely two systematic, message-wide defects in the port (rows
`textEncoding` and `subject` below): it always quoted-printable-encodes text bodies, and
it always emits a `Subject:` header even when there is no subject. Between them they
fail almost every case, which is why the second metric exists — without it the first
number says nothing about the other 30 divergences.

## How the upstream inventory was enumerated

Upstream's *exported* surface is tiny; the real API of nodemailer is the mail-options
object. Both were enumerated mechanically against the installed 6.9.16 package, never from
the README:

```sh
cd parity/nodemailer/node

# exported module surface and the transporter instance surface
node -e "const nm=require('nodemailer');
         console.log(Object.keys(nm).sort());
         const t=nm.createTransport({streamTransport:true,buffer:true});
         console.log(Object.getOwnPropertyNames(Object.getPrototypeOf(t)).sort());
         console.log(Object.getOwnPropertyNames(require('nodemailer/lib/mail-composer').prototype).sort())"

# every mail-options key the composer and the mail-message reader actually read
grep -ohE "this\.mail\.[A-Za-z_]+|this\.data\.[A-Za-z_]+|mail\.data\.[A-Za-z_]+" \
  node_modules/nodemailer/lib/mail-composer/index.js \
  node_modules/nodemailer/lib/mailer/mail-message.js \
  node_modules/nodemailer/lib/mailer/index.js | sed -E 's/.*\.([A-Za-z_]+)$/\1/' | sort -u

# per-attachment and per-alternative keys
grep -ohE "attachment\.[A-Za-z_]+"  node_modules/nodemailer/lib/mail-composer/index.js | sort -u
grep -ohE "alternative\.[A-Za-z_]+" node_modules/nodemailer/lib/mail-composer/index.js | sort -u

# the address/threading keys, which the composer sets from a literal list
sed -n '61p' node_modules/nodemailer/lib/mail-composer/index.js
```

The Go side was enumerated with:

```sh
cd parity/nodemailer && GOWORK=off go doc -all github.com/malcolmston/nodemailer
```

Status is graded on the canonical fields the symbol governs, not on whether the whole
case matched — otherwise the two systematic defects would mark everything `differs`.
Those two defects are graded on their own rows (`subject`, `textEncoding`).

## Table A — upstream exported API

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `nodemailer.createTransport` | `nodemailer.NewTransporter` | untested | — | the harness composes, it does not send |
| `nodemailer.createTestAccount` | — | missing | — | provisions an Ethereal account over the network |
| `nodemailer.getTestMessageUrl` | — | missing | — | Ethereal-specific |
| `Mail#sendMail` | `Transporter.SendMail` | untested | — | the node runner calls it, but only its composed message is compared; the Go side is compared through `Message.Build` |
| `Mail#verify` | `SMTPTransport.Verify` / `Pool.Verify` | untested | — | needs a server |
| `Mail#close` | `Pool.Close` | untested | — | |
| `Mail#use` (plugins) | — | missing | — | no plugin pipeline in the port |
| `Mail#get` / `Mail#set` | — | missing | — | transport option accessors |
| `Mail#getVersionString` | — | missing | — | feeds the `X-Mailer` banner |
| `Mail#isIdle` | — | missing | — | pool introspection (`Pool` has no equivalent) |
| `Mail#dkim` | `Message.WithDKIM`, `DKIM.Sign` | untested | — | DKIM signing is not exercised by this harness |
| `Mail#_convertDataImages` (`attachDataUrls`) | — | missing | — | |
| `Mail#setupProxy` | — | missing | — | |
| `MailComposer#compile` | `Message.Build` | differs | all 86 | the compared symbol pair; see the divergence list |
| `MailComposer#getAlternatives` | `Message.Alternatives` handling | differs | `body-watch-html`, `body-amp`, `body-watch-and-amp`, `body-custom-alternative`, `body-two-alternatives`, `body-ical-event` | alternative ordering and charset differ |
| `MailComposer#getAttachments` | `Message.Attachments` handling | differs | `att-*`, `cid-*` | related/attached split differs |
| `MailComposer#_createMixed` | `Message.Build` (mixed branch) | differs | `att-only-no-body`, `att-explicit-disposition-inline`, `cid-no-html` | the port wraps where upstream collapses |
| `MailComposer#_createAlternative` | `Message.Build` (alternative branch) | differs | `body-text-and-html`, `body-watch-html`, `body-amp` | ordering |
| `MailComposer#_createRelated` | `Message.Build` (related branch) | differs | `cid-single-image`, `cid-image-with-text-alternative`, `cid-plus-regular-attachment`, `cid-two-images`, `cid-from-path`, `cid-non-image` | nesting and `type=` parameter |
| `MailComposer#_createContentNode` | `Message.Build` (single-part branch) | match | `body-text-only`, `body-html-only` | graded on tree shape |
| `MailComposer#_processDataUrl` | — | missing | — | `data:` URI attachments |

## Table B — upstream mail-options keys

| upstream option | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `from` | `Message.SetFrom` | match | `addr-from-bare`, `addr-from-display-name`, `addr-non-ascii-display-name`, `addr-quoted-display-name` | port quotes display names, upstream does not; equivalent after unquoting |
| `sender` | — | missing | `addr-sender` | no `Sender` field; the header is simply absent |
| `to` | `Message.AddTo`, `Message.AddToGroup` | differs | `addr-to-multiple`, `addr-group-to`, `addr-group-empty`, `addr-idn-domain`, `addr-invalid-no-at`, `addr-invalid-empty-domain` | IDN domains not punycoded; invalid addresses rejected where upstream accepts |
| `cc` | `Message.AddCc`, `Message.AddCcGroup` | match | `addr-to-cc-bcc`, `addr-group-cc` | |
| `bcc` | `Message.AddBcc` | differs | `addr-to-cc-bcc` | the port omits the `Bcc` header from the message entirely |
| `replyTo` | `Message.AddReplyTo` | match | `addr-reply-to`, `addr-reply-to-multiple` | |
| `subject` | `Message.SetSubject` | differs | `subject-ascii`, `subject-empty`, `subject-non-ascii`, `subject-cjk`, `subject-emoji`, `subject-long-ascii`, `subject-long-non-ascii`, `subject-with-newline` | RFC 2047 encoding and folding agree; but the port always emits `Subject:` (empty when unset) and does **not** strip CRLF, so a subject can inject headers |
| `messageId` | `Message.SetMessageID` | match | all | |
| `date` | `Message.SetDate` | match | all | identical RFC 5322 rendering |
| `inReplyTo` | `Message.SetInReplyTo` | match | `hdr-in-reply-to`, `hdr-references-multiple` | |
| `references` | `Message.AddReferences` | match | `hdr-references-single`, `hdr-references-multiple` | space-separated on both sides |
| `text` | `Message.SetText` | match | `body-text-only`, `body-text-multiline`, `body-text-and-html` | decoded payload identical |
| `html` | `Message.SetHTML` | match | `body-html-only`, `body-html-non-ascii`, `body-text-and-html` | |
| `watchHtml` | `Message.SetWatchHTML` | differs | `body-watch-html`, `body-watch-and-amp` | correct `text/watch-html` part, wrong position in `multipart/alternative` |
| `amp` | `Message.SetAMP` | differs | `body-amp`, `body-watch-and-amp` | correct `text/x-amp-html` part, wrong position |
| `icalEvent` | `Message.ICalEvent` | differs | `body-ical-event` | entirely different tree (see divergence 9) |
| `alternatives` | `Message.AddAlternative` | differs | `body-custom-alternative`, `body-two-alternatives` | port appends `charset=utf-8` to every `text/*` alternative; non-text alternatives handled differently |
| `attachments` | `Message.Attach*` / `Embed*` | differs | all `att-*`, `cid-*`, `enc-*attachment*` | see Table C |
| `headers` | `Message.AddHeader` | match | `hdr-custom-single`, `hdr-custom-multiple`, `hdr-custom-non-ascii`, `hdr-custom-long-value` | duplicates, folding and RFC 2047 all agree |
| `list` | `Message.SetListUnsubscribe`, `AddListHeader` | differs | `hdr-list-unsubscribe`, `hdr-list-id-and-help` | one comma-joined header vs one header per URI; upstream also coerces a bare `List-ID` to `http://…` |
| `priority` | `Message.SetPriority` | match | `hdr-priority-high`, `hdr-priority-low`, `hdr-priority-normal` | `X-Priority`, `X-MSMail-Priority`, `Importance` all agree |
| `textEncoding` | — | missing | `enc-forced-quoted-printable`, `enc-forced-base64`, `enc-7bit-short-ascii`, `enc-8bit-latin1`, `enc-mostly-non-ascii`, `enc-long-lines`, `enc-trailing-space`, `enc-equals-sign` | no option, and no per-body heuristic: text is always quoted-printable |
| `encoding` | — | missing | — | message-wide default transfer encoding |
| `envelope` | `Info.Envelope` (derived only) | missing | — | no per-message envelope override |
| `raw` | — | missing | — | pass a pre-built message through |
| `attachDataUrls` | — | missing | — | |
| `disableFileAccess` | — | missing | — | the port has no file/URL access switch (and `AttachURL` fetches unconditionally) |
| `disableUrlAccess` | — | missing | — | |
| `dkim` | `Message.WithDKIM` | untested | — | present in the port, not exercised here |
| `newline` | — | missing | — | the port always emits CRLF |
| `normalizeHeaderKey` | — | missing | — | |
| `xMailer` | — | missing | — | the port never emits `X-Mailer`; suppressed upstream for comparability |
| `baseBoundary` | `Message.SetBoundary` | match | all multipart cases | both accept a pinned base boundary |
| `boundaryPrefix` | — | missing | — | |

## Table C — upstream attachment keys

| upstream key | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `content` (string / Buffer / Stream) | `Attachment.Content`, `AttachBytes`, `AttachReader` | match | `att-string-explicit-type`, `att-buffer`, `att-from-stream` | payloads and base64 wrapping identical |
| `path` | `AttachFile`, `EmbedFile` | match | `att-from-path`, `att-from-path-binary`, `cid-from-path`, `att-missing-path` | both fail on a missing file |
| `filename` | `Attachment.Filename` | differs | `att-non-ascii-filename`, `att-non-ascii-filename-cjk`, `att-no-filename`, `att-long-ascii-filename`, `att-filename-with-quote` | **malformed MIME for non-ASCII names** (divergence 1); no synthesised filename when absent |
| `contentType` | `Attachment.ContentType` | differs | `att-string-inferred-type`, `att-inferred-unknown-extension`, `att-two-attachments`, `cid-plus-regular-attachment` | inference agrees, but the port adds `charset=utf-8` to inferred `text/*` attachment parts |
| `contentDisposition` | `Attachment.Inline` | differs | `att-explicit-disposition-inline`, `cid-non-image` | `inline` also flips the container to `multipart/related`; non-image `cid` parts are marked `inline` where upstream keeps `attachment` |
| `cid` | `Attachment.ContentID`, `Embed*` | differs | `cid-single-image`, `cid-image-with-text-alternative`, `cid-two-images`, `cid-plus-regular-attachment`, `cid-from-path`, `cid-no-html`, `cid-non-image` | `Content-ID` itself matches; container type/nesting differs |
| `encoding` | (decoded by the runner) | match | `att-base64-encoding-option` | |
| `contentTransferEncoding` | — | missing | `enc-attachment-explicit-cte-binary`, `enc-attachment-explicit-cte-7bit` | the port always base64s attachments |
| `headers` | — | missing | — | per-attachment extra headers |
| `href` | `AttachURL` (unconditional fetch) | untested | — | deliberately not exercised: no network access in this harness |
| `httpHeaders` | — | missing | — | |
| `raw` | — | missing | — | |

## Table D — upstream alternative keys

| upstream key | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `content` | `Alternative.Content` | match | `body-custom-alternative`, `body-two-alternatives` | |
| `contentType` | `Alternative.ContentType` | differs | `body-custom-alternative`, `body-two-alternatives` | port appends `charset=utf-8` to `text/*` |
| `contentTransferEncoding` | — | missing | `body-two-alternatives` | |
| `filename` | — | missing | — | |
| `headers` | — | missing | — | |
| `path` | — | missing | — | |
| `href` | — | missing | — | |
| `encoding` | — | missing | — | |
| `raw` | — | missing | — | |

## Table E — Go-only surface (`extra` / `untested`)

Derived from `GOWORK=off go doc -all github.com/malcolmston/nodemailer`. None of these has
an upstream counterpart in the message-composition path.

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `Message.SetListUnsubscribePost` | extra | `hdr-list-unsubscribe-post` | declared deviation: emits RFC 8058 `List-Unsubscribe-Post`, which 6.9.16 cannot. The library has no `API-DEVIATIONS.md` yet, so per `HARNESS.md` this deviation still needs to be recorded there. |
| `Message.GenerateTextFromHTML`, `HTMLToText` | untested | — | no upstream equivalent (`html-to-text` is a separate package upstream) |
| `Message.AttachURL` | untested | — | would require network access |
| `Message.EmbedReader`, `Message.AttachReader` | extra | `att-from-stream` | Go-shaped stream helpers |
| `ParseMIME`, `ParsedMessage`, `ParsedAddr`, `ParsedAttachment`, `ParseAddresses`, `ParseAddressesFlatten`, `ParsedMessage.Get` | untested | — | inbound parsing; upstream ships no parser (`mailparser` is separate) |
| `ParseAddress`, `ParseAddressList`, `ParseAddressGroup`, `Address.*`, `AddressGroup.String`, `FormatAddressList`, `NormalizeAddress` | untested | — | exercised indirectly through the address cases, not as standalone symbols |
| `EncodeWord`, `EncodeMimeWord`, `DecodeHeaderWord`, `QPEncode`, `QPWrap`, `Base64Encode`, `Base64Wrap`, `IsPlainText`, `HasLongerLines`, `GenerateMessageID` | untested | — | encoder internals exposed as public helpers; upstream keeps `mime-funcs` private |
| `SMTPTransport`, `Pool`, `SendmailTransport`, `MemoryTransport`, `StreamTransport`, `JSONTransport`, `Transport`, `Transporter`, `Info`, `Envelope`, `CapturedMessage` | untested | — | delivery layer; out of scope (no SMTP in this harness) |
| `DKIM`, `Canonicalization`, `DefaultDKIMHeaders`, `DKIM.Sign`, `DKIM.DNSRecord`, `DKIM.DNSRecordName`, `ParseRSAPrivateKey` | untested | — | |
| `DSNOptions`, `DSNNotify`, `DSNReturn` | untested | — | upstream exposes DSN through SMTP transport options |
| `Service`, `WellKnownService`, `WellKnownServiceNames`, `NewServiceSMTP`, `XOAuth2Auth`, `XOAuth2Token` | untested | — | |
| `ErrInvalidAddress`, `ErrDKIMConfig`, `ErrPoolClosed` | untested | — | |

## Counts

Over Tables A–D (the upstream inventory):

| status | Table A | Table B | Table C | Table D | total |
| --- | --- | --- | --- | --- | --- |
| match | 1 | 12 | 3 | 1 | 17 |
| differs | 6 | 9 | 4 | 1 | 20 |
| missing | 9 | 12 | 4 | 7 | 32 |
| untested | 5 | 1 | 1 | 0 | 7 |
| **total** | **21** | **34** | **12** | **9** | **76** |

- **Symbol parity (over the 37 symbols actually compared): 17 / 37 = 45.9 %.**
- **Case parity (strict canonical tree): 1 / 85 = 1.18 %.**
- **Case parity (structural, the two systematic defects masked): 55 / 85 = 64.71 %.**
- 86 cases total: 85 compared, 1 declared deviation. Of the 85, 1 is a both-sides-fail
  agreement (`att-missing-path`) and 3 are disagreements about *whether* composing
  succeeds at all.

Table E adds 2 `extra` and 11 grouped `untested` Go-only rows; they are excluded from the
percentages above, which score the port against upstream, not the reverse.

## Every real divergence found

Ordered roughly by severity.

1. **Malformed MIME for non-ASCII attachment filenames** (`att-non-ascii-filename`,
   `att-non-ascii-filename-cjk`). The port emits a bare RFC 2047 encoded word as an
   *unquoted* media-type parameter value:

   ```
   Content-Type: text/plain; charset=utf-8; name==?utf-8?b?cmFwcG9ydC1hbm7DqWUtw7wudHh0?=
   Content-Disposition: attachment; filename==?utf-8?b?cmFwcG9ydC1hbm7DqWUtw7wudHh0?=
   ```

   `?` and `=` are RFC 2045 *tspecials*, so this is not a valid parameter token and the
   value must be a quoted string; encoded words are not permitted in parameters at all.
   Upstream emits RFC 2231 continuations instead:
   `filename*0*=utf-8''%C3%BCn%C3%AFcode.txt`. The port's own `ParseMIME` drops the
   parameter when it reads this back. The harness records it as
   `malformed: unquoted parameter filename contains tspecials: …` on the Go side only.
2. **Header injection through `subject`** (`subject-with-newline`). A subject containing
   `\r\n` is emitted verbatim, so `"Innocent\r\nX-Injected: yes"` produces a real
   `X-Injected: yes` header. Upstream folds it into the subject value. This is a security
   defect, not a formatting one.
3. **`Bcc` is dropped** (`addr-to-cc-bcc`). Upstream writes a `Bcc:` header into the
   composed message; the port omits it (its recipients still reach the envelope via
   `Recipients()`).
4. **An empty `Subject:` header is always emitted** (every case without a subject). Upstream
   omits the field entirely.
5. **Text bodies are always `quoted-printable`** (`enc-7bit-short-ascii`,
   `enc-mostly-non-ascii`, `enc-forced-*`, and every text case). Upstream chooses per body:
   `7bit` for clean ASCII, `quoted-printable` for a little 8-bit, `base64` when non-ASCII
   dominates — and honours `textEncoding`. The port has no such heuristic and no option.
   The QP *escaping* itself agrees (`=`, trailing space/tab, 76-column soft wrapping).
6. **Alternative ordering** (`body-watch-html`, `body-amp`, `body-watch-and-amp`). Upstream
   orders `text/plain`, `text/watch-html`, `text/x-amp-html`, `text/html` — least to most
   preferred, with HTML last. The port emits `text/plain`, `text/html`, `text/watch-html`,
   `text/x-amp-html`, so a client picking the last renderable part gets the watch or AMP
   body instead of the HTML one.
7. **`multipart/related` lacks `type="text/html"`** (`cid-single-image`, `cid-two-images`,
   `cid-from-path`, `cid-non-image`).
8. **Related/alternative nesting is inverted when both a text alternative and a `cid`
   resource are present** (`cid-image-with-text-alternative`, `cid-plus-regular-attachment`).
   Upstream builds `alternative[ text/plain, related[ text/html, image ] ]`; the port builds
   `related[ alternative[ text/plain, text/html ], image ]`, which puts the inline image
   outside the alternative it belongs to.
9. **`icalEvent` produces a different tree** (`body-ical-event`). Upstream builds
   `mixed[ alternative[ text/plain, text/calendar; method=REQUEST ], application/ics
   attachment ]` — the invitation appears both as an alternative and as a downloadable
   attachment. The port builds only `alternative[ text/plain, text/calendar; method=REQUEST ]`.
10. **A lone attachment is wrapped instead of collapsed** (`att-only-no-body`). With an
    attachment and no body, upstream emits a single top-level `text/plain` attachment part;
    the port emits `multipart/mixed` containing an **empty** `text/plain` part plus the
    attachment.
11. **`contentDisposition: inline` changes the container** (`att-explicit-disposition-inline`).
    The port switches the root to `multipart/related`; upstream keeps `multipart/mixed`.
12. **A `cid` attachment with no HTML body stays `related`** (`cid-no-html`). Upstream demotes
    it to a regular attachment inside `multipart/mixed`.
13. **Non-image `cid` parts are marked `inline`** (`cid-non-image`). Upstream only defaults to
    `Content-Disposition: inline` for `image/*`.
14. **`charset=utf-8` is added to attachment and alternative `text/*` parts**
    (`att-two-attachments`, `att-non-ascii-filename`, `att-long-ascii-filename`,
    `att-filename-with-quote`, `body-custom-alternative`, `body-two-alternatives`,
    `cid-plus-regular-attachment`). Upstream emits `text/plain; name="one.txt"` with no
    charset for attachment parts and no charset on custom alternatives.
15. **No filename is synthesised** (`att-no-filename`). Upstream invents
    `attachment-1.txt` and emits both `name=` and `filename=`; the port emits a bare
    `Content-Disposition: attachment`.
16. **`List-Unsubscribe` is comma-joined** (`hdr-list-unsubscribe`). Upstream emits one
    `List-Unsubscribe` header per URI; the port emits
    `<mailto:…>, <https://…>` in a single header. (Both forms are legal per RFC 2369; they
    are not the same bytes.) In the same case upstream coerces a bare `List-ID` value into
    `<http://…>`, which the port does not — an upstream quirk rather than a port defect.
17. **IDN domains are not punycoded** (`addr-idn-domain`). Upstream emits
    `user@xn--mller-kva.example`; the port emits raw UTF-8 `user@müller.example`.
18. **Address validation is stricter** (`addr-invalid-no-at`, `addr-invalid-empty-domain`).
    The port returns `nodemailer: invalid email address` where upstream happily composes a
    message with `To: not-an-address`. Arguably an improvement, but not parity.
19. **An empty message is an error** (`body-empty`). The port returns
    `nodemailer: message has no content`; upstream composes an empty `text/plain` body.
20. **`sender` is unsupported** (`addr-sender`) — no `Sender:` header.
21. **Per-attachment `contentTransferEncoding` is ignored**
    (`enc-attachment-explicit-cte-binary`, `enc-attachment-explicit-cte-7bit`). The port
    always base64s attachments.

Things that *do* agree, and are worth recording because they are the hard parts: RFC 2047
subject encoding for Latin-1, CJK and astral-plane emoji; long-header folding for both
plain and encoded-word subjects and for custom headers; duplicate custom header fields;
`References` / `In-Reply-To` rendering; priority header triples; address group syntax
(including the empty `Undisclosed recipients:;` group); base64 attachment payloads and
76-column wrapping; quoted-printable escaping of `=`, trailing whitespace and long lines;
content-type inference from a filename extension, including the `application/octet-stream`
fallback; `Content-ID` values; and boundary-lookalike body text.

## Running it

```sh
cd parity/nodemailer/node && npm install     # once, pins nodemailer@6.9.16
cd .. && GOWORK=off go test ./...            # rewrites parity.json
```

`go test` skips (never fails) when `node` is not on `PATH` or when
`node/node_modules/nodemailer` is absent.
