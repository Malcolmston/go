# nodemailer example

A runnable validation program for the **published** module
`github.com/malcolmston/nodemailer`, consumed exactly as an outside user would
(no `replace` directive).

Resolved version: **`v0.0.0-20260719012648-105dba31a04d`** (pseudo-version — the
repo has no semver tags). The published Go sources are byte-identical to the
local working tree at the time of writing.

It composes real messages, renders and inspects the MIME output, re-parses it,
signs it with DKIM and delivers it through every transport the library ships —
**without any external server or internet access**.

## Run

```sh
cd examples/nodemailer
GOWORK=off go get github.com/malcolmston/nodemailer@latest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program terminates on its own. The only network activity is a listener on
`127.0.0.1:0` inside this same process (see `smtpserver.go`), used to drive
`SMTPTransport` and `Pool` for real.

## What it demonstrates

1. **Addresses** — `ParseAddress`, `ParseAddressList`, `NormalizeAddress`,
   `Address.Local/Domain/Equal`, `FormatAddressList`, `ParseAddresses`
   (including RFC 5322 groups), `ParseAddressGroup`, and rejection of invalid
   input.
2. **Composition** — a single message with From, two To recipients, Cc, Bcc,
   Reply-To, a named `To:` group and an empty `Cc:` group, a non-ASCII subject,
   text + HTML bodies, AMP and watch-HTML alternatives, an extra
   `text/x-markdown` alternative, a `text/calendar` REQUEST invite, priority
   headers, `In-Reply-To`/`References` threading, `List-ID` /
   `List-Unsubscribe` / `List-Unsubscribe-Post` (RFC 8058), custom `X-*`
   headers, an inline `cid:logo` PNG generated in memory, and three attachments
   sourced from bytes, a file path and an `io.Reader`.
   Date, Message-ID and boundary are pinned, and the example asserts that two
   `Build()` calls produce identical bytes.
3. **MIME inspection** — prints all top-level headers and an indented tree of
   the emitted boundaries and `Content-*` headers, showing the
   `multipart/mixed` → `multipart/related` → `multipart/alternative` nesting
   the library chooses automatically.
4. **`ParseMIME` round trip** — decodes the message it just built and checks
   the subject, address lists, date, custom headers and each attachment,
   including that the inline PNG is byte-identical after base64 round tripping.
5. **HTML→text and codecs** — `HTMLToText`, `GenerateTextFromHTML`, `QPEncode`,
   `QPWrap`, `Base64Encode`, `EncodeWord`, `DecodeHeaderWord`, `EncodeMimeWord`,
   `IsPlainText`, `HasLongerLines`, `GenerateMessageID`.
6. **DKIM** — generates a 2048-bit RSA key in process, round-trips it through
   `ParseRSAPrivateKey`, signs a message via `WithDKIM` (`Build` prepends the
   `DKIM-Signature`), calls `DKIM.Sign` directly, prints `DNSRecordName()` /
   `DNSRecord()`, and shows that an incomplete `DKIM` is rejected.
7. **Non-network transports** — `MemoryTransport`, `JSONTransport` (record is
   unmarshalled and checked), `StreamTransport` (two messages into one buffer).
8. **SMTP** — `SMTPTransport.Verify()`, a full send with RFC 3461 `DSNOptions`
   (the example prints the `MAIL FROM ... RET=/ENVID=` and
   `RCPT TO ... NOTIFY=/ORCPT=` lines the server actually received), then a
   `Pool` with `MaxConnections: 2` / `MaxMessages: 2` sending five messages
   (observably reusing connections), `Close()`, and `ErrPoolClosed` afterwards.
9. **Well-known services** — `WellKnownServiceNames`, `WellKnownService`,
   `NewServiceSMTP` (plus the unknown-service error), and `XOAuth2Auth` /
   `XOAuth2Token`. `SendmailTransport` is exercised against `/usr/bin/true`,
   and against a missing binary to show the error path.
10. **Deferred validation** — an invalid address passed to `AddTo` surfaces at
    `Build()` time.

## Holes found

### 1. Non-ASCII attachment filenames produce malformed MIME (data loss)

`mime.go:quoteParam` emits a **bare, unquoted** RFC 2047 encoded word for a
non-ASCII filename:

```
Content-Type: application/octet-stream; name==?utf-8?b?ZG9ubsOpZXMuYmlu?=
Content-Disposition: attachment; filename==?utf-8?b?ZG9ubsOpZXMuYmlu?=
```

`?` and `=` are MIME tspecials, so that parameter value is not a valid `token`
and is not a `quoted-string` either. `mime.ParseMediaType` rejects the header,
and the library's own `ParseMIME` then **silently drops the entire part**:
section `4b` of the example round-trips a message with a `données.bin`
attachment and recovers **0** attachments, while the identical message with an
ASCII filename recovers 1. The parse returns no error, so the loss is silent.
The README/doc.go both advertise "RFC 2047 encoded words for non-ASCII …
filenames" as a correctness feature. A conforming encoder must quote the
encoded word (`filename="=?utf-8?b?…?="`, the widely-tolerated hack) or use
RFC 2231 (`filename*=utf-8''...`), which is what Node's Nodemailer emits.

### 2. `SendmailTransport` always prepends `-i`, so most binaries can't stand in

`SendmailTransport.args` hardcodes `-i` as the first argument with no way to
suppress it (`Args` are inserted *after* it). That makes it impossible to point
the transport at an arbitrary local program for testing — `cat`, `wc`, `tee`
all fail on `-i`. The example has to use `/usr/bin/true`, which ignores its
arguments. A `Command []string` escape hatch, or making the `-i` opt-out, would
make the transport testable.

### 3. No validation that `From` is set

`New().AddTo("who@example.com").Err()` is nil and `Build()` succeeds, producing
a message with an empty `From:` and an empty SMTP envelope sender. Every real
SMTP server will reject it. Reported by the example in section 10.

### 4. Minor / cosmetic

- `HTMLToText` drops link targets: `<a href="https://example.com/details">Details</a>`
  becomes just `Details`, so the URL is unrecoverable in the text fallback.
- An empty address group renders as `Undisclosed: ;` (space before the
  semicolon). Legal per RFC 5322 CFWS, but unusual.
- `ParseMIME` flattens `To:` groups into the plain `To` slice — group structure
  present in the built message is not recoverable from the parsed one. There is
  no `ParsedMessage.ToGroups`.
- `Attachment.ContentType` sniffing gave `text/csv` for `report.csv` from
  `AttachFile`, which is correct; but `AttachReader` with an empty content type
  always yields `application/octet-stream` with no sniffing of the stream
  itself, despite the README's "with content-type sniffing" claim covering
  readers.

### Not holes (verified working)

- `Pool` really does reuse connections: 5 messages with `MaxMessages: 2` opened
  3 connections.
- Deterministic output: two `Build()` calls on the same message are
  byte-identical.
- Nested boundary derivation (`EXAMPLEBOUNDARY`, `_2`, `_3`) is distinct per
  level and the tree parses cleanly.
- DSN parameters reach the wire exactly as documented.
- The whole library is stdlib-only; `go mod tidy` added no dependencies.
