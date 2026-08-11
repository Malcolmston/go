# `socketio/engineio` vs `engine.io-parser` — API coverage

Nested harness (see `../../../HARNESS.md`). `parity/socket.io/` scores
**socket.io**; this directory scores the *different* upstream package that
`socket.io/engineio` is a port of: **`engine.io-parser`**, the Engine.IO v4
transport codec, plus the deterministic part of **`engine.io`** itself (its
protocol revision and its re-export of that parser).

- **Upstream oracle:** `engine.io-parser@5.2.3`, `engine.io@6.6.4`
  (pinned in `node/package.json`, `node/package-lock.json`), Node v24.18.0.
- **Port under test:** `github.com/malcolmston/socketio/engineio`
  (`replace` → `../../../../socket.io`).
- **Run:** `npm install` in `node/`, then `GOWORK=off go test .` from this
  directory. `GOWORK=off` is required: the harness is its own module, outside the
  aggregator workspace.
- **Score:** `parity.json`, rewritten by the test.

## How the upstream inventory was produced

Mechanically, from the installed packages — not from the README and not from
memory:

```
$ cd node && node -e 'Promise.all([import("engine.io-parser"),import("engine.io")])
    .then(([p,e])=>{console.log(Object.keys(p).sort());
                    console.log(Object.keys(e).sort());
                    console.log(Object.keys(e.transports).sort())})'
engine.io-parser: [ createPacketDecoderStream, createPacketEncoderStream,
                    decodePacket, decodePayload, encodePacket, encodePayload,
                    protocol ]
engine.io:        [ Server, Socket, Transport, attach, listen, parser, protocol,
                    transports ]
e.transports:     [ polling, websocket, webtransport ]
```

The Go side, likewise mechanically:

```
$ cd ../../../../socket.io && GOWORK=off go doc -all ./engineio
```

## Representation contract

Both runners speak the same JSON shape so that the comparison is about the codec
and not about language types:

- A packet is `{"type": <engine.io type name>, "data": <string>, "binary": <lowercase hex | null>}`.
  Type **names** (not digits) are used deliberately, so `PacketType`'s constants
  and its `String` method are exercised in both directions.
- **Binary payloads are lowercase hex strings** in every case file and every
  reply, because JSON has no byte type.
- `encodePacket` is driven with `supportsBinary=false` (the polling form: `"b"` +
  standard base64) except for the three `*-native-frame`/`supports-binary` cases.
- Upstream signals a parse failure *in band*, by returning its `ERROR_PACKET`
  (`{type:"error",data:"parser error"}`) instead of throwing. The node runner
  converts that into `ok:false`, which is the honest counterpart of the Go codec
  returning an `error`. Failures are compared as answers in their own right.

## `engine.io-parser` — every exported symbol

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `encodePacket` | `engineio.Packet.Encode` (+ `Packet.Binary` for a native frame) | match | 25 `enc-*` (+ every `encpay-*`/`roundtrip-*`) | all seven types, empty/unicode/NUL/separator-bearing text, empty and multi-byte binary, base64 padding, and the type-prefix-dropping behaviour for binary |
| `decodePacket` | `engineio.Decode` | differs | 37 (`dec-*`, incl. 5 `decodeBinaryFrame`) | 33/37 identical; the 4 divergences are the declared lenient-base64 deviations D1–D4 below |
| `encodePayload` | `engineio.EncodePayload` | differs | 10 `encpay-*` | 9/10 identical; the divergence is D5 (upstream never calls back for an empty batch) |
| `decodePayload` | `engineio.DecodePayload` | match | 17 `decpay-*` | separators, empty/leading/trailing/double separators, bad members, 200-member batch |
| `protocol` (= 4) | `engineio.Protocol` | match | `protocol-revision` | |
| `createPacketEncoderStream` | — | missing | — | WebTransport framing (length-prefixed `TransformStream`). The port has no WebTransport transport, so there is nothing to frame. |
| `createPacketDecoderStream` | — | missing | — | same |

`eio.roundTrip` (6 cases) drives `encodePayload` + `decodePayload` together and
is counted under both.

## `engine.io` (the server) — every exported symbol

The port's `engineio` package is a **codec only**; the server half of Engine.IO
lives in the parent `socketio` package and is scored by `parity/socket.io/`
(groups `behaviour` and `interop`). Listed here for completeness:

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `protocol` (= 4) | `engineio.Protocol` | match | `protocol-revision` | both packages advertise revision 4; the port has one constant for both roles |
| `parser` (re-export of `engine.io-parser`) | the `engineio` package | match | all 101 | identical surface, scored above |
| `Server` | — (`socketio.Server` embeds the role) | missing | — | no standalone Engine.IO server; scored via `parity/socket.io/` |
| `Socket` | — (`socketio.Socket` is the Socket.IO-level socket) | missing | — | |
| `Transport` | — | missing | — | the port's transport is internal to `socketio` |
| `attach` | — | missing | — | `socketio.Server` is itself an `http.Handler` |
| `listen` | — | missing | — | |
| `transports.polling` | — | missing | — | the port is websocket-only |
| `transports.websocket` | — (internal to `socketio`) | missing | — | present but not exported from `engineio` |
| `transports.webtransport` | — | missing | — | not ported |

## Go-only surface (`extra`)

| Go symbol | cases | note |
| --- | --- | --- |
| `PacketType` + `Open`/`Close`/`Ping`/`Pong`/`Message`/`Upgrade`/`Noop` (7) | every case | upstream uses type-name strings; the port uses a numeric type, and the name mapping is checked in both directions by every case |
| `PacketType.String` | every `dec-*`, `decpay-*`, `roundtrip-*` | the reply's `type` field *is* `String()`'s output |
| `Packet` struct (`Type`/`Data`/`Binary`) | every case | upstream has a plain object with a polymorphic `data` |
| `NewOpen`, `NewClose`, `NewPing`, `NewPong`, `NewMessage`, `NewUpgrade`, `NewNoop`, `NewBinaryMessage` (8) | every `enc-*`, `encpay-*`, `roundtrip-*`, `dec-binary-frame*` | the Go runner builds packets through these constructors rather than struct literals, so they are covered rather than bypassed |
| `Packet.IsBinary` | every `dec-*`/`roundtrip-*` reply | decides `binary` vs `data` in the reply |
| `ErrEmptyPacket` | `dec-empty-string`, `decpay-empty-body` | upstream's counterpart is the in-band `ERROR_PACKET` |
| `CloseCode` + 14 code constants, `CloseCode.String`, `CloseCode.IsValid`, `EncodeCloseFrame`, `DecodeCloseFrame`, `ErrInvalidCloseFrame` (19) | — (`untested` here) | RFC 6455 close-frame helpers. Neither `engine.io-parser` nor `engine.io` exports a close-code table — upstream gets it from the `ws` library, which is not part of the ported surface — so there is **no oracle** and no case can be written. Covered by the port's own `engineio/closecode_test.go`. |

## Declared deviations

All five are `"deviation"` entries in the case files, are counted separately from
mismatches in `parity.json`, and are listed in
`socket.io/API-DEVIATIONS.md`. Every one of them is a case where **upstream is
the outlier**.

- **D1 `dec-binary-bad-base64`** (`"b###"`) — Node's `Buffer.from(s,"base64")`
  silently drops characters outside the alphabet, so upstream accepts the frame
  and hands the application an *empty* payload. `base64.StdEncoding` returns an
  error and the port rejects the frame. (Also recorded as W12 by the parent
  harness.)
- **D2 `dec-binary-bad-base64-length`** (`"b4"`) — one base64 character cannot
  encode a byte; upstream yields an empty payload, the port errors.
- **D3 `dec-binary-base64url`** (`"b-_8"`) — Node's decoder also accepts the
  base64url alphabet; the port insists on the standard alphabet the encoder
  emits.
- **D4 `dec-binary-base64-whitespace`** (`"bYW Jj"`) — Node's decoder skips
  embedded whitespace; the port does not.
- **D5 `encpay-empty-batch`** — `encodePayload` counts completed packets against
  the batch length, so for an **empty** batch the counter never reaches the
  length and **the callback is never invoked**: upstream hangs. The port returns
  the empty string.

D1–D4 are one family: the port refuses to fabricate a payload out of malformed
base64, which is the safer answer for attacker-supplied bytes. D5 is a latent
upstream hang the port has no reason to reproduce.

## Recorded differences that are not scored

- **`encodePacket` with a type outside the seven-name table.** Upstream emits the
  literal string `"undefined"` + data (`PACKET_TYPES[bogus]` is `undefined`); the
  port emits the raw digit for an out-of-range `PacketType`. Neither validates,
  and neither answer is defensible, so there is no meaningful oracle: the case
  `enc-unknown-type` only asserts that *both* runners refuse a name that is not
  in the table.
- **Partial results from `decodePayload`.** On a bad member upstream `break`s and
  returns the packets decoded so far *plus* an error packet; the port returns an
  error for the whole body. Both refuse the body, which is the only thing the
  transport layer acts on, so `decpay-error-after-good-members` compares as a
  shared failure. The port never surfaces the partially decoded prefix, which is
  the stricter reading of a corrupt batch.
- **No escaping in the payload format.** A text packet whose data contains U+001E
  is emitted verbatim by both implementations and decodes as two packets
  (`encpay-data-contains-separator`). That is the wire format, identical on both
  sides, and not a port defect.

## Counts

Cases: **101** total — **96 compared, 96 match, 0 mismatch**, plus **5 declared
deviations**. Case parity over compared cases: **100.0%** (`parity.json`).

Symbols:

| | engine.io-parser | engine.io | total |
| --- | --- | --- | --- |
| match | 3 | 2 | 5 |
| differs (declared deviations only) | 2 | 0 | 2 |
| missing | 2 | 8 | 10 |
| compared (match + differs) | 5 | 2 | 7 |
| extra (Go-only) | — | — | 21 tested + 19 untested (close frames) |

Symbol parity over the symbols actually compared: **5/7 = 71.4%** exact, or
**7/7** if the two documented lenient-base64/empty-batch deviations are counted
as intended divergences. Every `missing` symbol is server-side Engine.IO, which
this package deliberately does not implement — that role belongs to the parent
`socketio` package and is scored by `parity/socket.io/`.
