# liveview example

A runnable, self-terminating program that exercises
[`github.com/malcolmston/liveview`](https://github.com/malcolmston/liveview)
(a Phoenix LiveView-style server-rendered stateful UI framework for Go) as an
**external consumer of the published module** — there is no `replace`
directive; the dependency is resolved from the module proxy.

**Resolved module version:** `github.com/malcolmston/liveview v0.0.0-20260719012639-59c5d74e34c6`
(pseudo-version; the repo has no semver tags.)

## Run

```sh
cd examples/liveview
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints labelled output plus `PASS`/`FAIL` for every assertion and
exits non-zero if any assertion fails. It never blocks: the HTTP and WebSocket
sections run against `httptest` servers with socket deadlines, and the program
returns from `main` on its own.

## What it demonstrates

A stateful `Dashboard` view (`Mount` / `HandleEvent` / `Render`) that also
implements the optional `ParamsHandler` and `InfoHandler` interfaces, plus a
stateful live `Toggle` component embedded inside it.

1. **Engine (no transport)** — `NewSession` → `Mount` → initial `Rendered`
   HTML → `InitialDiff` (with `"s"` statics and `"c"` components) → `Event`
   producing sparse diffs. Asserts:
   - `len(Statics) == len(Dynamics)+1`,
   - an `inc` event yields a 2-key diff with **no** statics,
   - a no-op event yields an empty diff,
   - switching the render to a *different* template re-sends the statics,
   - nested `*Rendered` (a sub-template) diffs recursively,
   - `FullDiff` / `DiffRendered` used standalone, including the nil cases.
2. **Templates & escaping** — `MustParse`, `Parse` error cases (unterminated
   `{{`, empty slot), `{{{{` brace escaping, missing assigns rendering empty,
   HTML escaping and the `Safe` opt-out (`&quot;`/`&#39;` Phoenix-style
   entities).
3. **Socket** — `Assign`, `AssignAll`, `Get`/`GetInt`/`GetString`, change
   tracking (`Changed`, `AnyChanged`, `ResetChanges`), and that `Assigns()`
   returns a copy.
4. **Live components** — cid allocation, `ComponentEvent(cid, …)` producing a
   diff *only* under `"c"` with the parent slot omitted, `EventByID`, `CID`.
5. **Flash** — `PutFlash` / `GetFlash` / `ClearFlash` and the `Flash` map API
   (`Put`, `Get`, `Has`, `Delete`, `Kinds`, `Merge`).
6. **Side channels** — `PushEvent` → `"e"`, `PushPatch`/`PushNavigate` → `"nav"`,
   live patch via `Session.Params` re-invoking `HandleParams`.
7. **PubSub / handle_info** — `NewPubSub`, `AttachPubSub`, `Socket.Subscribe`,
   `Broadcast`, `Session.Inbox()` → `Session.Info` → re-render; `Socket.Send`
   self-messaging; `Session.Close` unsubscribing.
8. **Streams** — `StreamInsert` / `StreamDelete` / `Stream.Reset` /
   `Append` / `Prepend` / `Pending`, and draining into the `"stream"` diff key.
9. **Uploads** — `AllowUpload`, `RegisterUploadEntry`, chunked `UploadChunk`
   with progress, `UploadEntry.Bytes`, `ConsumeUploadedEntries`, plus the
   max-file-size rejection and unknown-slot no-ops.
10. **Forms & HTML helpers** — `DecodeForm`/`DecodeFormString` (nested brackets,
    `tags[]` lists, last-value-wins), `Form` validation errors and `InputName`,
    `ClassList`, `AttrList`, `HiddenInputs`, `LivePatch`, `LiveNavigate`.
11. **JS commands** — chained `NewJS().AddClass(…).Toggle(…).Transition(…).Focus(…).Push(…)`,
    `Commands`, `String`, `Safe`, `Concat`.
12. **HTTP transport** — `httptest` server over `NewHandler`: the GET page
    (embedded `lv-root`, session id, inline JS client, query params reaching
    `Mount`), `POST /event` returning a JSON diff, 404 for an unknown session,
    400 for bad JSON, `Handler.Session`, `Handler.PubSub`.
13. **WebSocket transport** — a hand-rolled RFC 6455 client: real handshake
    (verified against `liveview.AcceptKey`), the `{"type":"mount"}` frame, an
    `event` frame, a `cid`-targeted component event, a `patch`, `upload_start`
    and `upload_chunk`, an ignored unknown message type, and ping/pong.
14. **README claims** — the "Driving the engine directly" snippet from the
    library README is asserted verbatim (initial HTML and the `{"1":"1"}` diff).

## Holes found

- **Component statics are re-sent on the first event over the HTTP route.**
  Only `Session.InitialDiff()` marks component renders as "sent". The
  `Handler`'s GET page path (`servePage`) renders the component into the HTML
  but never calls it, so the *first* `POST /event` diff (and the first
  `Session.Info` diff, if `InitialDiff` was skipped) ships a full component
  diff including its `"s"` statics even though the component did not change.
  The example asserts and prints this. The WebSocket path is fine because
  `socketLoop` sends `InitialDiff` first.
- **Flash is unreadable from `Render`.** `Flash` is stored under an unexported
  assign key (`__flash__`) and every exported accessor (`SocketFlash`,
  `GetFlash`) takes a `*Socket` — but `Render(assigns map[string]any)` never
  gets a socket. The example has to iterate the assigns map looking for a value
  of type `liveview.Flash`. There is no exported key constant or
  `FlashFromAssigns(assigns)` helper.
- **`Socket.LiveComponent` returns an unexported type** (`*componentRef`), so
  callers cannot name or store it in a typed variable/struct field; it only
  works because it is immediately handed to `Assign` as `any`. Same for
  `TryLiveComponent`. This trips the standard "exported func returns unexported
  type" lint.
- **Reserved diff keys are unexported string constants.** `"s"`, `"c"`, `"e"`,
  `"nav"`, `"stream"` are private (`staticsKey`, `componentsKey`, …), so any
  consumer inspecting a `Diff` in Go must hardcode the literals, as this
  example does.
- **`Handler` routes *any* POST to the event handler**, ignoring the path — the
  doc comment advertises `POST {prefix}/event`, but `POST /anything` is
  accepted. Conversely `prefix` is used only to build the client's WS URL, not
  to match request paths, so a handler mounted at `/dash` also serves `/`.
- **`Counter.HandleEvent("set")` only accepts an `int` payload**, but JSON
  decodes numbers as `float64`, so the documented `set` event silently does
  nothing when it arrives from the wire. The example asserts this. (`bump` in
  this example handles `float64` explicitly to work around the same trap for
  user views.)
- **README/`doc.go` "API surface" table is badly out of date** — it lists 9
  symbols (`View`, `Socket`, `Template`, `Rendered`, `Diff`, `Session`,
  `Handler`, `Counter`, `Safe`) and never mentions the majority of the package:
  `Component`/`ComponentManager`, `Stream`/`StreamOp`, uploads, `PubSub`,
  `Flash`, `Form`/`DecodeForm`, `JS` and its ~25 command builders, the
  `ParamsHandler`/`InfoHandler` hooks, the whole WebSocket implementation
  (`Upgrade`, `Conn`, `AcceptKey`), and the HTML helpers. Nothing documented is
  *wrong* — the README simply undersells the library by roughly 4x, and the
  README's "GET / … POST /event" description omits the WebSocket transport that
  is actually the primary path.
- Minor: `Diff` side-channel values are live Go values (`[]PushEvent`,
  `*Nav`, `map[string][]StreamOp`) rather than pre-marshalled JSON, so
  round-tripping a `Diff` through `encoding/json` and back does not yield the
  same Go types. Fine for the server→client direction, surprising in tests.

Everything else worked as documented; no panics, no compile failures, and no
features had to be commented out.
