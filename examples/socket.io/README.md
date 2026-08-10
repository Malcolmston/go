# socket.io example — BLOCKED: the library is not consumable as a published module

> **Read this first.** This example cannot resolve its dependency from the module
> proxy. The `replace` directive in `go.mod` is a temporary local-checkout
> workaround, not the intended way to consume the library. Delete it once the
> module path below is fixed.

## The blocker

The library's `go.mod` declares one module path, but the GitHub repository lives
at a different one:

| | value |
| --- | --- |
| `module` line in `socket.io/go.mod` | `github.com/malcolmston/socketio` (no dot) |
| `git remote -v` | `https://github.com/malcolmston/socket.io.git` (with a dot) |

Because Go derives the repository URL from the module path, **neither path can be
fetched**. Verified in an empty scratch module (`GOWORK=off go mod init probe`):

```console
$ GOWORK=off go get github.com/malcolmston/socketio@latest
go: github.com/malcolmston/socketio@latest: module github.com/malcolmston/socketio:
	git ls-remote -q --end-of-options origin in .../cache/vcs/0383...: exit status 128:
	remote: Repository not found.
	fatal: repository 'https://github.com/malcolmston/socketio/' not found
```

```console
$ GOWORK=off go get github.com/malcolmston/socket.io@latest
go: github.com/malcolmston/socket.io@latest (v0.4.0) requires github.com/malcolmston/socket.io@v0.4.0:
	parsing go.mod:
	module declares its path as: github.com/malcolmston/socketio
	        but was required as: github.com/malcolmston/socket.io
```

The second error is the informative one. It proves three things:

1. A dot in a module path element is perfectly legal — Go did not reject
   `github.com/malcolmston/socket.io` as malformed. The `.io` suffix on a
   non-leading path element is not treated as a hostname.
2. `github.com/malcolmston/socket.io` **resolves**: Go reached the repo, listed
   its tags, picked `v0.4.0`, and downloaded and parsed that tag's `go.mod`.
3. The only remaining failure is the module-path mismatch *inside* that
   `go.mod`, which is fatal on its own.

The repo *is* tagged, so versioning is not the problem:

```console
$ git -C socket.io ls-remote --tags origin
refs/tags/stable
refs/tags/v0.1.0   refs/tags/v0.2.0   refs/tags/v0.3.0   refs/tags/v0.4.0
```

`v0.1.0` through `v0.4.0` are all present and well-formed semver. Every one of
them is unfetchable for the reason above.

### Which fix works

Both candidate fixes work, and they are mutually exclusive. Pick one.

**(a) Rename the GitHub repository to `socketio`.** Makes the existing
`module github.com/malcolmston/socketio` line correct. This is the smaller
change: no Go source is touched, the existing `v0.1.0`–`v0.4.0` tags become
fetchable retroactively, and GitHub keeps redirecting the old `socket.io` URL.
It is also the only fix that does **not** invalidate the already-published tags.
Not verifiable from here — it requires renaming the repo, which is the
maintainer's call.

**(b) Change the `module` line to `github.com/malcolmston/socket.io`.**
Empirically confirmed to be the only thing standing between the current repo and
a successful `go get`: the probe above already resolved the path, fetched the
tag, and parsed the `go.mod` — the mismatch was the sole error. Note it is
**not** a one-line change: 31 files in the library reference
`github.com/malcolmston/socketio` (all the internal imports of
`.../engineio`, `.../internal/ws`, `.../client`, plus the README badges and the
install instruction), and every one has to change with it. It also requires a
new tag, because `v0.4.0` will forever declare the old path.

Recommendation: **(a)**, because it fixes the four tags that already exist rather
than orphaning them.

Note that the library's own README currently tells users to run the command that
cannot work:

```sh
go get github.com/malcolmston/socketio   # README.md line 60 — always fails
```

## Running the example

```sh
cd examples/socket.io
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

It starts a Socket.IO server on a random loopback port via `httptest`, connects
three of the library's own Go clients to it, drives every feature, prints an
`ok`/`FAIL` line per assertion, shuts down, and exits — `0` if every check
passed, `1` otherwise. There is no outbound network access, no fixed port, and
every wait is bounded by a timeout, so it always terminates.

Status: **builds and runs clean.** All assertions pass, on repeated runs and
under `go run -race` (no data races reported).

## What it demonstrates

| Feature | API exercised |
| --- | --- |
| Events with arguments | `Socket.On` / `Socket.Emit`, `Client.On` / `Client.Emit` |
| Per-socket state | `Socket.Set` / `Get` / `GetString` / `Data` |
| Client → server ack | `Client.EmitWithAck`, handler returning `[]any` |
| Server → client ack | `Socket.EmitAck` (blocking), `Socket.EmitWithAck` (callback), `Socket.PendingAcks` |
| Binary payloads | `[]byte` args in both directions, as ACK payloads and mixed with text args |
| Catch-all | `Socket.OnAny`, `Socket.ListenersAny` |
| Namespaces | `Server.Of`, `Namespace.OnConnection`, `Namespace.Name`, `Namespace.FetchSockets` |
| Connection middleware | `Namespace.Use` gating on `Socket.Auth()`, rejection surfacing as a `Dial` error |
| Rooms | `Socket.Join`, `Socket.Rooms`, `Namespace.SocketsInRoom`, `Server.SocketsJoin` / `SocketsLeave` |
| Broadcasting | `Server.To(room).Emit` (all members), `Socket.To(room).Emit` and `Socket.Broadcast().Emit` (sender excluded — asserted negatively) |
| Server-to-server events | `Server.OnServerEvent`, `Server.ServerSideEmit` |
| Disconnect | `Socket.OnDisconnect`, `Client.Close`, `Server.DisconnectSockets`, client-side `"disconnect"` event |
| Shutdown | `Server.Close` |

Things that behaved exactly as documented and needed no workaround: the
polling→WebSocket transport stack, JSON and binary codecs, ack correlation in
both directions, namespace isolation, middleware rejection, sender exclusion on
both broadcast operators, and shutdown. `Server.Close` is genuinely clean — a
separate probe (5 clients, connect, ack, close) went from 1 goroutine to 22 at
peak and back to **1** after `Server.Close` + `srv.Close`, so there is no
goroutine or session leak, and no lingering ping loops.

## Holes found

### 1. Blocker: the module path does not match the repo (see above)

The library cannot be `go get`-ed at all, at any version. This is the only defect
that makes the library unusable rather than merely awkward.

### 2. Acks are sent for events that have no handler

Marked `// HOLE:` in `main.go`. Both `Client.dispatch` (`client/client.go`) and
`Socket.dispatch` (`socket.go`) send an ACK whenever the inbound packet carries
an ack id, substituting an empty `[]any{}` when no handler ran or every handler
returned `nil`:

```go
if pkt.ID != nil {
	if ack == nil {
		ack = []any{}   // acked even though nothing handled the event
	}
	_ = c.sendPacket(...)
}
```

In socket.io the ack fires only when the handler invokes its callback, so
"the peer has no handler for this" is observable. Here it is not. Two
consequences:

- `Socket.EmitAck` **cannot** return `ErrAckTimeout` when talking to this
  library's own Go client — the client always answers. The timeout path, and the
  exported `ErrAckTimeout` sentinel with it, is effectively unreachable in a
  Go-to-Go deployment. The example asserts the real behaviour (`err == nil`,
  empty reply) rather than the documented-looking one.
- A caller cannot distinguish "handled, nothing to report" from "unhandled".

### 3. `Client` has no catch-all, and no way to remove a handler

`Socket` (server side) has `OnAny` / `PrependAny` / `OffAny` / `ListenersAny`
and `Off`. `client.Client` has only `On`. There is no `OffAny`, no `OnAny`, and
no `Off` — a handler registered on a `Client` is permanent, and `On` appends, so
registering twice for the same event runs both. The example works around this by
registering each client handler exactly once up front.

### 4. Non-idiomatic API surface

- No `context.Context` anywhere. `Socket.EmitAck`, `Client.EmitWithAck`, and
  `client.Dial` all take a bare `time.Duration`, so a caller cannot cancel a
  wait or propagate a request context.
- `Socket.Emit` returns `error` but `BroadcastOperator.Emit`, `Server.Emit`,
  and `Namespace.Emit` return nothing, so a broadcast has no failure signal at
  all.
- Handler signature is `func(args []any) []any` — untyped in and untyped out,
  with the ACK expressed as "return non-nil". Numbers always arrive as
  `float64` (JSON), which every handler has to know; the example asserts
  `float64(42)`, not `42`.
- Server event handlers run **on the connection's read-loop goroutine**. Calling
  `Socket.EmitAck` from inside a handler therefore deadlocks: the loop that
  would deliver the ACK is the loop that is blocked waiting for it. The `client`
  package documents this hazard for its own handlers; the server-side `Socket`
  docs do not mention it. The example drives all server-initiated acks from the
  main goroutine for this reason.

### 5. README-vs-reality mismatches

- `README.md` line 60 documents `go get github.com/malcolmston/socketio`, which
  is precisely the command that fails. The `pkg.go.dev` and Go Report Card
  badges (lines 8–9) point at the same non-existent module.
- The "Status & scope" section understates binary support: "Binary attachments
  (BINARY_EVENT/BINARY_ACK) are parsed but the convenience API focuses on JSON
  payloads." In fact the convenience API handles `[]byte` end to end in both
  directions, including as ACK payloads and mixed with text args — this example
  exercises all of those and they work. The dedicated "Binary events" section
  earlier in the same README describes it correctly; the two contradict.

Not a hole, for the record: `Socket.EmitWithAck("event", cb, args...)` in the
README's Socket table matches the real signature (callback second), and
`io.Use(func(s *socketio.Socket, next func(error)))` matches
`Use(func(*Socket, func(err error)))`. Both check out.
