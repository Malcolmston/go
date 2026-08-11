# `content-type` — API coverage

Nested harness: the oracle is the npm **`content-type`** package, not express.
Upstream pinned at `content-type@1.0.5` in [`node/package.json`](node/package.json);
Go port is `github.com/malcolmston/express/contenttype` in the `express`
submodule.

## How the upstream inventory was derived

```console
$ cd node && node -e "const c=require('content-type'); \
    console.log(typeof c); console.log(Object.keys(c).join(', '))"
object
format, parse
```

The Go side is `GOWORK=off go doc -short ./contenttype` in the submodule:

```
func Format(ct ContentType) (string, error)
type ContentType struct{ ... }
    func Parse(s string) (ContentType, error)
```

## Symbol inventory

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `contentType.parse(string)` | `contenttype.Parse(s)` | match | 42 in `parse` + 8 round trips | grammar, quoting, escapes, control characters, malformed parameters |
| `contentType.parse(req/res)` | — | missing | — | upstream also accepts a request/response-like object and reads `content-type` off it; the port takes the header value, leaving that plumbing to the framework |
| `contentType.format(obj)` | `contenttype.Format(ct)` | match | 26 in `format` | sorted parameter order, token vs quoted output, rejected values |
| `ContentType` (the returned shape) | `contenttype.ContentType` | match | every `parse` case | upstream returns `{type, parameters}`; the Go struct is `{Type, Parameters}` |

`Parse` errors are compared as errors: the harness matches on *whether* both
sides failed, not on message text. The Go port's messages are prefixed
`contenttype:` rather than reproducing Node's `TypeError` strings.

## Counts

| status | symbols |
| --- | --- |
| match | 3 |
| differs | 0 |
| missing | 1 |
| extra | 0 |
| untested | 0 |

Parity over the 3 symbols actually compared: **3/3 = 100%**.

**Cases: 76. Measured parity: 76/76 = 100.0%** (see
[`parity.json`](parity.json)), up from 68/76 = 89.5%.

## What the harness fixed in the port

All 8 mismatches were the port being **more permissive than upstream about
control characters and whitespace**, in both directions:

- `Parse` used `"(?:\\.|[^"\\])*"` for the quoted-string body, which admits
  anything at all between the quotes — including HTAB, CR, LF and NUL. Upstream's
  `PARAM_REGEXP` restricts it to `%x0b`, `%x20-21`, `%x23-5b`, `%x5d-7e` and
  obs-text `%x80-ff`, with escapes drawn from `%x0b` / `%x20-ff`. The port now
  uses that class verbatim, so `charset="utf-8\r\nX-Injected: 1"` is a parse
  error instead of a parameter value carrying a CRLF. **This one is filed as a
  security finding** — see [`security.json`](security.json).
- `Parse` allowed tabs as parameter padding (`text/html;\tcharset=utf-8`,
  `charset\t=\tutf-8`); upstream's grammar is `; *name *= *value *`, spaces only.
- `Format`'s output class had HTAB in it and was missing vertical tab, i.e. it
  was the wrong set on both ends. It now matches upstream's `TEXT_REGEXP`
  (`%x0b`, `%x20-7e`, `%x80-ff`), so a tab or CRLF in a parameter value is an
  error rather than something written into a header.
- `Format` rejected an empty parameter value; upstream's `qstring` only applies
  the text check to a non-empty string, so `{charset: ""}` legitimately formats
  as `charset=""` and the parse/format round trip of `title=""` succeeds.

The port's `edge_test.go` and `contenttype_parity_test.go` vectors were corrected
in the same change — several of them had pinned the too-permissive behaviour as
though it were intended.
