# `socketio/client` vs `socket.io-client` — API coverage

Nested harness (see `../../../HARNESS.md`). `parity/socket.io/` scores the
**socket.io server** and merely uses this client as a driver; this directory
scores the *different* upstream package `socket.io/client` is a port of:
**`socket.io-client`**.

- **Upstream oracle:** `socket.io-client@4.8.1` with `socket.io-parser@4.2.7` and
  `engine.io-client@6.6.6` (pinned in `node/package.json` /
  `node/package-lock.json`), Node v24.18.0.
- **Port under test:** `github.com/malcolmston/socketio/client`
  (`replace` → `../../../../socket.io`).
- **Run:** `npm install` in `node/`, then `GOWORK=off go test .` from this
  directory. `GOWORK=off` is required: the harness is its own module, outside the
  aggregator workspace.
- **Score:** `parity.json`, rewritten by the test.

## Scope: what "deterministic" means for a client

A client is mostly I/O, and timing-dependent reconnection races are not parity
material. Every case here is **socket-free and deterministic**: upstream runs
with `autoConnect: false` throughout, so a real `Manager`/`Socket` is built and
interrogated but never dialled, and `Math.random` is replaced by a fixed draw
sequence for the jitter cases. Four surfaces are compared:

| group | what is compared | how upstream is driven |
| --- | --- | --- |
| `backoff` (31) | the reconnection schedule, in whole milliseconds | `Backoff` from `socket.io-client/build/esm/contrib/backo2.js` — the module `Manager` seeds from the reconnection options — with `Math.random` stubbed |
| `endpoint` (23) | the Engine.IO URL and namespace a dial would use | `url()` parses the URI, `new Manager(parsed.source, {autoConnect:false})`, then `new Engine(mgr.uri, mgr.opts)` and its websocket `Transport.uri()` — exactly the composition `lookup()` + `Manager.open()` perform |
| `options` (11) | normalised option defaults | a real `Manager`, read back through its own accessors (`reconnection()`, `reconnectionDelay()`, …); the mount path is read from the engine.io `Socket` the Manager would build, because that is where the trailing slash is added |
| `wire` (35) | the encoded frames an emit puts on the wire | a real `Socket` that is not connected: `emit` lands in `sendBuffer`, `_sendConnectPacket` is intercepted by stubbing `io._packet`, and the packets are encoded with `socket.io-parser`'s `Encoder` |
| `protocol` (1) | the protocol revision | `protocol` re-exported by `socket.io-client` |

Conventions: all durations are **milliseconds** in both requests and replies; a
binary event argument is written `{"$bin": "<lowercase hex>"}` and becomes a
`Buffer`/`[]byte`; encoded attachment frames come back as `"bin:<lowercase hex>"`;
`reconnectionAttempts` is `null` when unlimited, because upstream's default is
`Infinity`, which has no JSON form. Endpoint replies give `base` (the URL up to
the `?`) and `query` as a **map**, since upstream appends `EIO`/`transport` after
the caller's parameters while Go's `url.Values.Encode` sorts them and the
ordering carries no meaning.

## How the upstream inventory was produced

Mechanically, from the installed package:

```
$ cd node && node -e 'import("socket.io-client").then(m=>{
    console.log(Object.keys(m).sort());
    console.log(Object.getOwnPropertyNames(m.Manager.prototype).sort());
    console.log(Object.getOwnPropertyNames(m.Socket.prototype).sort())})'
exports:      [ Fetch, Manager, NodeWebSocket, NodeXHR, Socket, WebSocket,
                WebTransport, XHR, connect, default, io, protocol ]
Manager.proto:[ _close, _destroy, _packet, cleanup, connect, disconnect,
                maybeReconnectOnOpen, onclose, ondata, ondecoded, onerror,
                onopen, onping, onreconnect, open, randomizationFactor,
                reconnect, reconnection, reconnectionAttempts, reconnectionDelay,
                reconnectionDelayMax, socket, timeout ]
Socket.proto: [ _addToQueue, _clearAcks, _drainQueue, _registerAckCallback,
                _sendConnectPacket, ack, active, close, compress, connect,
                destroy, disconnect, disconnected, emit, emitBuffered, emitEvent,
                emitWithAck, listenersAny, listenersAnyOutgoing,
                notifyOutgoingListeners, offAny, offAnyOutgoing, onAny,
                onAnyOutgoing, onack, onclose, onconnect, ondisconnect, onerror,
                onevent, onopen, onpacket, open, packet, prependAny,
                prependAnyOutgoing, send, subEvents, timeout, volatile ]
```

Two internal modules the package does not re-export are imported by path from
the installed tree, because they *are* the upstream implementations of what the
port implements: `build/esm/url.js` and `build/esm/contrib/backo2.js`.

The Go side, likewise mechanically:

```
$ cd ../../../../socket.io && GOWORK=off go doc -all ./client
```

## Module exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `io` / `connect` / `default` (`lookup`) | `client.Dial` | differs | 23 `ep-*` (addressing) | Addressing is compared exactly; `lookup`'s manager **cache** (`forceNew`, `multiplex`) has no counterpart — one `Client` is one connection. |
| `Manager` | — (folded into `Client`) | differs | 11 `opt-*` | The port has no separate multiplexing layer: one `Client` is one namespace on one transport. `Manager`'s *option* surface is compared through `Options.WithDefaults`. |
| `Socket` | `client.Client` | differs | 35 `wire-*` | The packet-building surface is compared exactly; the event-emitter surface is smaller (see below). |
| `protocol` (= 5) | `socketio.ProtocolVersion` | match | `client-protocol` | |
| `NodeWebSocket` / `WebSocket` | — (internal `ws` transport) | missing | — | The port has exactly one transport, a websocket, and does not let it be swapped. |
| `NodeXHR` / `XHR` / `Fetch` | — | missing | — | No HTTP long-polling transport. |
| `WebTransport` | — | missing | — | Not ported. |

## `Manager` — every prototype member

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `reconnection` | `Options.Reconnection` | differs | `opt-*` | value matches once set; the **default** differs (upstream true) — declared deviation |
| `reconnectionAttempts` | `Options.ReconnectionAttempts` | match | `opt-defaults`, `opt-attempts-only` | 0 means unlimited, upstream's `Infinity` |
| `reconnectionDelay` | `Options.ReconnectionDelay` | match | `opt-defaults`, `opt-delay-only` | 1000 ms default on both sides |
| `reconnectionDelayMax` | `Options.ReconnectionDelayMax` | match | `opt-defaults`, `opt-delay-max-only` | 5000 ms default on both sides |
| `randomizationFactor` | `Options.RandomizationFactor` | differs | `opt-*`, all `bo-jitter-*` | the arithmetic matches `backo2` exactly; the **default** (0.5) and the out-of-range normalisation point differ — declared deviations |
| `timeout` | `Options.DialTimeout` | match | `opt-defaults`, `opt-timeout-only` | 20 000 ms default on both sides |
| `open` / `connect` | `client.Dial` → `establish` | untested here | — | opens a transport; scored live by `parity/socket.io/` (`behaviour`, `interop`) |
| `socket` | `Options.Namespace` / the URL path | differs | `ep-namespace*` | upstream returns a multiplexed `Socket` per namespace on one `Manager`; the port dials one `Client` per namespace |
| `reconnect` / `maybeReconnectOnOpen` / `onreconnect` | `Client.reconnectLoop` (unexported) | untested here | — | timing-dependent; the *schedule* it follows is what the `backoff` group pins |
| `_packet` | `Client.sendPacket` (unexported) | match (indirect) | 35 `wire-*` | the frames it would write are compared |
| `ondata` / `ondecoded` / `onping` / `onopen` / `onclose` / `onerror` | `Client.readLoop` (unexported) | untested here | — | inbound transport plumbing; scored live by `parity/socket.io/` |
| `disconnect` / `_close` / `_destroy` / `cleanup` | `Client.Close` | untested here | — | scored live by `parity/socket.io/` (`behaviour` disconnect cases) |

## `Socket` — every prototype member

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `emit` | `Client.Emit` / `client.EventPacket` | match | 27 `wire-emit-*` | argument shapes, unicode, binary attachments, ack ids, and all six reserved names |
| `emitWithAck` | `Client.EmitWithAck` | match | `wire-emit-ack-*` | the packet it builds; the awaiting half is scored live by `parity/socket.io/` |
| `_sendConnectPacket` | `client.ConnectPacket` | match | 8 `wire-connect-*` | namespace and auth payload |
| `_registerAckCallback` / `ack` / `onack` | `Client.pendingAcks` (unexported) | untested here | — | the *allocator* is compared (`wire-ack-ids-sequence`); resolution is scored live by `parity/socket.io/` |
| `ids` (ack id allocator) | `client.AckIDs` | match | `wire-ack-ids-sequence` | 0, 1, 2, … on both sides |
| `on` / `off` (from `Emitter`) | `Client.On` | untested here | — | dispatch is scored live by `parity/socket.io/`; the port has no `off` |
| `connect` / `open` | `client.Dial` | untested here | — | |
| `disconnect` / `close` | `Client.Close` | untested here | — | |
| `active` / `disconnected` | — | missing | — | no state predicates; `Client.ID()` returning "" is the nearest signal |
| `id` | `Client.ID` | untested here | — | assigned by the server; scored live by `parity/socket.io/` |
| `send` | — | missing | — | sugar for `emit("message", …)` |
| `compress` / `volatile` | — | missing | — | no per-packet flags; the port always sends |
| `timeout` | `Client.EmitWithAck`'s timeout argument | differs | — | upstream returns a flagged `Socket`; the port takes the timeout as an argument |
| `onAny` / `prependAny` / `offAny` / `listenersAny` | — | missing (4) | — | no catch-all listener on the client (the *server* port has `OnAny`) |
| `onAnyOutgoing` / `prependAnyOutgoing` / `offAnyOutgoing` / `listenersAnyOutgoing` / `notifyOutgoingListeners` | — | missing (5) | — | no outgoing interceptors |
| `_addToQueue` / `_drainQueue` / `emitBuffered` / `_clearAcks` | — | missing (4) | — | the port has no retry queue or offline buffer: `Emit` on a dead connection errors instead of buffering |
| `onpacket` / `onevent` / `onconnect` / `ondisconnect` / `onclose` / `onerror` / `emitEvent` / `packet` / `subEvents` / `destroy` | `Client` internals (unexported) | untested here | — | inbound dispatch; scored live by `parity/socket.io/` |

## Go-only surface (`extra`)

| Go symbol | cases | note |
| --- | --- | --- |
| `client.Backoff`, `BackoffOptions`, `NewBackoff`, `Duration`, `Reset`, `Attempts` (6) | 31 `bo-*` | upstream's equivalent is the *internal* `backo2` module, which the harness imports by path; `BackoffOptions.Rand` is Go-only, and exists so a `Backoff` can be deterministic in a test |
| `client.Endpoint`, `ResolveEndpoint`, `DefaultPath` (3) | 23 `ep-*` | upstream's equivalent is `url()` (internal) plus `Transport.uri()`; exported here so addressing can be inspected without dialling |
| `Options.WithDefaults` | 11 `opt-*` | upstream normalises inside the `Manager` constructor |
| `client.ConnectPacket`, `client.EventPacket` | 35 `wire-*` | the packet constructors `Dial`/`Emit` use, exported so the frames can be compared without a server |
| `client.AckIDs`, `AckIDs.Next` | `wire-ack-ids-sequence` | upstream's `Socket.ids` field |
| `client.Handler` | — | Go's callback shape; a handler returning a non-nil slice acknowledges |

## Declared deviations

Six, all `"deviation"` entries in the case files, counted separately from
mismatches in `parity.json`, and listed in `socket.io/API-DEVIATIONS.md`:

- **`opt-defaults`, `opt-reconnection-explicit`** — upstream defaults
  `reconnection` to **true** and `randomizationFactor` to **0.5**. A Go struct's
  zero value cannot express either default, and `Options` is a published API, so
  both must be set explicitly. The *arithmetic* driven by
  `randomizationFactor` matches `backo2` exactly (31/31 `backoff` cases).
- **`opt-jitter-out-of-range`** — `Manager` stores `randomizationFactor: 1.5`
  verbatim and lets `backo2` decide later that a value outside `(0,1]` means "no
  jitter"; the port normalises to 0 at the option layer. The resulting schedule
  is identical (`bo-jitter-over-one`).
- **`ep-uppercase-scheme`, `ep-unknown-scheme`, `ep-schemeless`** — `url()`
  recognises absolute URIs with a **case-sensitive** `/^(https?|wss?):\/\//` and
  otherwise resolves against `window.location`, which does not exist under Node:
  upstream answers `ws://undefined/socket.io/` with a namespace like
  `//HTTP://h:3000`. Upstream is the outlier (RFC 3986 §3.1 makes schemes
  case-insensitive, and relative addressing is browser-only).

## Port changes this harness drove

Measured before → after over the same 101 cases: **71/96 → 95/95 compared cases
matching** (74.0% → 100.0%).

- `Backoff.Duration` now computes what `backo2` computes: the deviation is
  **floored** before it is applied, the ceiling is enforced **after** the jitter
  rather than before, and the result is truncated to whole milliseconds. Six
  `bo-jitter-*` cases were wrong before.
- `NewBackoff` treats a jitter outside `(0,1]` as *no* jitter instead of clamping
  it to 1 (`bo-jitter-over-one`).
- Addressing was rebuilt as `ResolveEndpoint`: the URL's **path is the
  namespace** (as in `io("http://host/admin")`), the HTTP mount path is the new
  `Options.Path`, a port that is the scheme default is dropped, and userinfo
  never reaches the transport URL. Thirteen `ep-*` cases were wrong before, and
  `Dial(url + "/admin")` used to connect to `/` while asking for the HTTP path
  `/admin/`.
- `Options` gained `Path`, `ReconnectionDelayMax` and `RandomizationFactor`,
  `DialTimeout`'s default moved from 10 s to upstream's 20 s, and
  `reconnectLoop` now follows a real `Backoff` instead of an open-coded doubling.
  Six `opt-*` cases were wrong before.
- `ConnectPacket`, `EventPacket` and `AckIDs` were extracted from `Dial`,
  `Emit`/`EmitWithAck` and the ack counter, so the frames a client emits can be
  compared without a server. `Client` uses all three.

## Counts

Cases: **101** total — **95 compared, 95 match, 0 mismatch**, plus **6 declared
deviations**. Case parity over compared cases: **100.0%** (`parity.json`).

Symbols (module exports + `Manager` + `Socket` prototypes, 74 upstream symbols):

| status | count |
| --- | --- |
| match | 12 |
| differs | 8 |
| missing | 25 |
| untested (needs a live connection; scored by `parity/socket.io/`) | 29 |
| extra (Go-only) | 13 |

Symbol parity over the symbols actually compared: **12/20 = 60.0%** exact, and
20/20 if the eight `differs` rows — six of which are the declared deviations and
two the deliberate "one `Client` is one namespace" simplification — are counted
as intended divergences. The 25 `missing` symbols are the transports the port
does not implement (long-polling, WebTransport), the offline/retry queue, the
per-packet flags and the outgoing-interceptor API; the 29 `untested` ones are
inbound plumbing that cannot be exercised without a server and are covered live
by `parity/socket.io/`'s `behaviour` and `interop` groups.
