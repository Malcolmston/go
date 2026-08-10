# liveview — upstream API inventory vs the Go port

- **Upstream**: [`phoenixframework/phoenix_live_view`](https://github.com/phoenixframework/phoenix_live_view),
  Elixir, declared `{:phoenix_live_view, "~> 1.0"}` in `elixir/mix.exs` and
  **locked to `phoenix_live_view 1.2.9`** by `elixir/mix.lock` (with
  `phoenix 1.8.9`, `phoenix_html 4.3.0`, `phoenix_template 1.0.4`, `plug 1.20.3`).
  The runner is an escript built from `elixir/`.
- **Port**: `github.com/malcolmston/liveview@v0.3.0`, consumed as a published
  module (no `replace` directive).
- **Toolchain used**: Elixir 1.20.3 / Erlang OTP 29, Go 1.24 (`GOWORK=off`).
- **Score**: see `parity.json`, rewritten by `go test`.

## What is being compared, and what is not

LiveView is mostly a *distributed system*: a WebSocket channel, a per-view
GenServer under a supervision tree, a router, a dead-render plug, an upload
channel with its own writer processes, and a JavaScript client. None of that has
a repo-free entry point, and this harness deliberately starts **no endpoint, no
router, no PubSub, no channel and no socket**.

What LiveView *does* have that is spec-like — the one thing any LiveView client,
in any language, must agree with byte for byte — is **the rendered diff format**.
That is the entire comparable surface here:

| area | upstream entry point | comparable? | cases |
| --- | --- | --- | --- |
| static/dynamic split (`"s"` + numbered slots) | `Phoenix.LiveView.Diff.render/4` | yes | 21 (`static-split`) |
| HTML escaping and raw/safe passthrough | `MACRO-Phoenix.LiveView.Engine.to_safe/2`, `Phoenix.HTML.raw/1` | yes | 12 (`escaping`) |
| diff minimality on update | `Phoenix.LiveView.Diff.render/4` with threaded fingerprints | yes | 15 (`diff-minimality`) |
| comprehension / for-loop encoding | `Phoenix.LiveView.Comprehension` | yes | 14 (`comprehension`) |
| nested component encoding under `"c"`, cid scheme | `Phoenix.LiveView.Diff.render/4`, `Diff.write_component/4` | yes | 14 (`components`) |
| mount → event → diff cycle | `Phoenix.LiveView.Diff.render/4` over a hand-written mirror of the port's `Counter` | yes | 18 (`events`) |
| the materialised HTML for the same render | `Phoenix.LiveView.Diff.to_iodata/1` | yes | 7 (inside the groups above) |
| **a live WebSocket session** | `Phoenix.LiveView.Channel` | **no** — a GenServer joined to a `Phoenix.Socket`; there is nothing to call without an endpoint |
| **the supervision tree, `send_update`, async assigns** | `Phoenix.LiveView.Async`, `Phoenix.LiveView.Application` | **no** — requires live processes |
| **the dead render / HTTP mount** | `Phoenix.LiveView.Static` | **no** — requires a `Plug.Conn` and an endpoint with a signing salt |
| **uploads** | `Phoenix.LiveView.Upload`, `UploadChannel`, `UploadTmpFileWriter` | **no** — a separate channel plus a writer process per entry |
| **navigation, flash, tokens** | `Phoenix.LiveView.Utils`, `Phoenix.LiveView.Session`, `Phoenix.LiveView.Router` | **no** — signing needs `Endpoint.config(:secret_key_base)` |
| **forms and the built-in components** | `Phoenix.Component.form/1`, `inputs_for/1`, `to_form/1` | **no** — need an `Ecto`/`Plug` form source and a router for the action |
| **JS commands** | `Phoenix.LiveView.JS` | **no** — a `%JS{}` struct is serialised into an attribute and *interpreted by the browser*; nothing on the server produces a comparable answer |
| **the test client** | `Phoenix.LiveViewTest` (86 exports) | **no** — an `Endpoint` plus a `ClientProxy` process |

Everything in the "no" rows is marked `untested` below with that reason. It is
not guessed at, and the parity percentage is not allowed to imply anything about
it.

### Template syntax is explicitly out of scope

The two projects do not share a template language: upstream is HEEx (a full HTML
parser with `:for`/`:if`, slots, function components and compile-time attribute
validation), the port is a `{{ name }}` substitution engine over an assigns map
with no expressions, no conditionals and no loops. Comparing the syntaxes would
answer nothing.

Instead, `elixir/lib/runner.ex` and `go/run.go` each define **the same logical
template** under the same name (`counter`, `pair`, `lead`, `trail`, `plain`,
`one`, `nest`, `noraw`, `raw`, `missing`, `items`, `rows`, `withcomp`,
`twocomp`), and only the **emitted diff structure** is compared. The HEEx is
written on a single line with no leading or trailing whitespace so that HEEx's
whitespace trimming does not change the statics; that is the only concession the
upstream side makes to the port.

Where the port cannot express a construct at all, the runner uses the closest
thing the port *can* do and says so — see the comprehension section.

### Normalisations, and exactly what each one hides

Both runners emit JSON objects with **string keys, sorted**. Beyond that the
Elixir side normalises three upstream wire optimisations. Each is a real
difference, and each is either compensated for by a dedicated case or recorded
here as untested:

1. **Template dedup (`"p"` + integer `"s"`).** Since 1.0, upstream hoists
   repeated static lists into a root-level `"p"` map and replaces `"s"` with an
   index into it. The runner expands the index back into the literal statics list
   and drops `"p"`. Without this, *every* case would fail on the same artefact
   instead of on the structure under test. **The port has no template dedup**;
   that is a real (unmeasured) bandwidth difference, listed as untested.
2. **`"r"` (root marker).** Dropped on the upstream side; it carries no
   structure. The port emits nothing equivalent.
3. **Component static cross-references.** When two components of the same module
   are rendered, upstream ships the statics once and points the second at the
   first with an integer `"s"` inside `"c"`. `session_initial` resolves those
   references the way the JS client resolves them (`Diff.to_iodata/2`'s
   `resolve_components_xrefs`) so the *content* can be compared; the
   `cmp-initial-two-wire` case compares the **un-resolved wire form** head-on, so
   the optimisation is scored rather than hidden.
4. **cids are renumbered to first-appearance order starting at 1** on both sides,
   so neither implementation is judged on its numbering scheme. In practice this
   is the identity map: upstream's `Diff.new_components/0` starts at 1 and the
   port's `NewComponentManager` starts at 1, and both allocate in the order the
   components are first encountered. That agreement is itself confirmed by
   `cmp-initial-two` and `cmp-event-bump-cid2`.

### How the two sides obtain a *minimal* diff at all

This is the mechanism difference that most of the divergences below come from,
so it is worth stating plainly:

- **Upstream tracks assigns.** `Phoenix.Component.assign/3` records a key in
  `assigns.__changed__` (and does *not* record it when the new value is `===` the
  old one). The compiled HEEx closure then skips any dynamic whose assigns did
  not change, `traverse/6` sees `nil` for that slot, and the slot is absent from
  the diff. Statics are omitted whenever the fingerprint matches.
- **The port compares rendered values.** `liveview.DiffRendered(prev, next)`
  re-renders everything and emits a slot only when the new string differs from
  the old one, and re-sends the whole subtree (statics included) when the statics
  arrays are not equal.

The Elixir runner mirrors `assign/3` faithfully, including the "identical value
is not a change" rule (pinned by `ev-label-same-value`). The
`min-reassign-*` cases bypass `assign/3` and mark every assign changed, which
probes `Phoenix.LiveView.Engine`'s change-tracking contract directly.

## How the upstream inventory was produced

Mechanically, by reflection over the *installed* 1.2.9 package — not from the
docs and not from memory:

```elixir
# run from parity/liveview/elixir with: mix run --no-start <this file>
Application.load(:phoenix_live_view)

skip = [:module_info, :__info__, :__struct__, :__impl__, :behaviour_info,
        :__protocol__, :__using__, :__after_compile__, :__opts__,
        :__components__, :__mix_recompile__?, :__phoenix_verify_routes__,
        :__live__, :__templates__, :__mix_recompile__]

for m <- Enum.sort(Application.spec(:phoenix_live_view, :modules)) do
  exports =
    m.module_info(:exports)
    |> Enum.reject(fn {f, _a} -> f in skip end)
    |> Enum.reject(fn {f, _a} -> String.starts_with?(Atom.to_string(f), "-") end)
    |> Enum.sort()
    |> Enum.map(fn {f, a} -> "#{f}/#{a}" end)

  hidden? =
    match?({:docs_v1, _, _, _, h, _, _} when h in [:hidden, :none], Code.fetch_docs(m))

  IO.puts("#{inspect(m)}#{if hidden?, do: " [hidden]"}\t#{length(exports)}\t#{Enum.join(exports, ", ")}")
end
```

That yields **79 modules and 657 exported functions/macros**. 49 of the modules
carry `@moduledoc false` and are marked *(hidden)* below; compiler-generated
noise and anonymous-function stubs are filtered out.

The Go side was enumerated with
`GOWORK=off go doc -all github.com/malcolmston/liveview` over the same module
version `go.mod` pins: **161 exported symbols** (136 functions/methods, 25
types).

A note on hidden modules: `Phoenix.LiveView.Diff` is `@moduledoc false`, i.e.
private by convention. It is nevertheless the comparison target, because it is
the only place the wire format is produced, and the wire format is the one part
of LiveView that is a public contract in practice — every non-Elixir LiveView
client implements it.

## Module-by-module inventory

<!-- generated from the reflection script above -->

| upstream module | exports | Go counterpart | status | note |
| --- | --- | --- | --- | --- |
| `Enumerable.Phoenix.LiveView.LiveStream` *(hidden)* | 4 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveView.Socket` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveView.Socket.AssignsNotInSocket` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveView.UploadConfig` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveViewTest.Element` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveViewTest.Upload` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `Inspect.Phoenix.LiveViewTest.View` *(hidden)* | 1 | — | untested | needs a running Phoenix endpoint, a compile-time macro context, or a live process |
| `JSON.Encoder.Phoenix.LiveView.JS` *(hidden)* | 1 | `(*liveview.JS).MarshalJSON` | untested | the encodable form of a JS command; only the browser consumes it |
| `Jason.Encoder.Phoenix.LiveView.JS` *(hidden)* | 1 | `(*liveview.JS).MarshalJSON` | untested | as above |
| `Mix.Tasks.Compile.PhoenixLiveView` | 1 | — | untested | a Mix compiler task |
| `Mix.Tasks.PhoenixLiveView.Upgrade` *(hidden)* | 1 | — | untested | a Mix upgrade task |
| `Phoenix.Component` | 42 | `liveview.View`, `liveview.Template`, `liveview.ClassList`, `liveview.AttrList`, `liveview.LivePatch`, `liveview.LiveNavigate` | untested | `MACRO-sigil_H/3` is exercised transitively by every case (it produces the `Rendered` being diffed) but is not named in any `upstreamFn`. `assign/2,3`, `assign_new/3`, `update/3`, `changed?/2` are mirrored by `(*liveview.Socket).Assign` and friends, whose only observable output is the diff. The ~20 built-in components (`form/1`, `link/1`, `inputs_for/1`, `focus_wrap/1`, `portal/1`, `live_file_input/1`, …) need an endpoint or a `%Phoenix.HTML.Form{}` |
| `Phoenix.Component.Declarative` *(hidden)* | 12 | — | untested | compile-time `attr`/`slot` validation |
| `Phoenix.Component.MacroComponent` *(hidden)* | 5 | — | untested | compile-time macro components |
| `Phoenix.HTML.Safe.Phoenix.LiveComponent.CID` *(hidden)* | 1 | — | untested | protocol impl |
| `Phoenix.HTML.Safe.Phoenix.LiveView.Component` *(hidden)* | 1 | — | untested | protocol impl |
| `Phoenix.HTML.Safe.Phoenix.LiveView.Comprehension` *(hidden)* | 1 | — | untested | protocol impl; the comprehension *encoding* is compared separately |
| `Phoenix.HTML.Safe.Phoenix.LiveView.JS` *(hidden)* | 1 | — | untested | protocol impl |
| `Phoenix.HTML.Safe.Phoenix.LiveView.Rendered` *(hidden)* | 1 | `(*liveview.Rendered).HTML` | untested | the `Safe` protocol impl; the port's HTML materialisation is compared through `Diff.to_iodata/1` instead |
| `Phoenix.LiveComponent` | 2 | `liveview.Component`, `liveview.ComponentManager` | untested | only `__using__` macros; the behaviour itself is compared through `Diff.write_component/4` |
| `Phoenix.LiveComponent.CID` | 0 | the bare `int` a component slot carries | untested | 0 exports (a struct); the cid *scheme* is compared by the `components` cases |
| `Phoenix.LiveView` | 47 | `liveview.View`, `liveview.Socket`, `liveview.Stream`, `liveview.UploadConfig`, `liveview.PushEvent`, `liveview.Nav` | untested | every export needs a live socket: uploads, streams, async, flash, navigation, hooks, `send_update/2,3`, `transport_pid/1` |
| `Phoenix.LiveView.Application` *(hidden)* | 2 | — | untested | the OTP application callback |
| `Phoenix.LiveView.Async` *(hidden)* | 11 | — | missing | `assign_async`/`start_async` spawn tasks against a live process |
| `Phoenix.LiveView.AsyncResult` | 6 | — | missing | the port has no async-assign story |
| `Phoenix.LiveView.Channel` *(hidden)* | 19 | `liveview.Handler`, `liveview.Conn`, `liveview.Upgrade` | untested | the per-view WebSocket channel GenServer |
| `Phoenix.LiveView.ColocatedAssets` *(hidden)* | 2 | — | untested | build-time asset extraction |
| `Phoenix.LiveView.ColocatedAssets.Entry` *(hidden)* | 0 | — | untested | struct |
| `Phoenix.LiveView.ColocatedCSS` | 3 | — | missing | build-time asset extraction |
| `Phoenix.LiveView.ColocatedHook` | 1 | — | missing | build-time asset extraction |
| `Phoenix.LiveView.ColocatedJS` | 3 | — | missing | build-time asset extraction |
| `Phoenix.LiveView.Component` | 0 | — | untested | 0 exports (a struct): the placeholder a `live_component/1` call leaves in the dynamics |
| `Phoenix.LiveView.Comprehension` | 0 | — | differs | 0 exports (a struct), but its *encoding* is what the 14 `comprehension` cases compare; the port has no comprehension construct at all |
| `Phoenix.LiveView.Controller` | 2 | — | missing | `live_render/2,3` from a Phoenix controller |
| `Phoenix.LiveView.Debug` | 4 | — | missing | live-process introspection |
| `Phoenix.LiveView.Diff` *(hidden)* | 14 | `liveview.FullDiff`, `liveview.DiffRendered`, `(*liveview.Session).InitialDiff`, `(*liveview.Session).Event`, `(*liveview.Session).ComponentEvent`, `(*liveview.Rendered).HTML` | differs | **the comparison target.** `render/4`, `write_component/4` and `to_iodata/1` are cased; the other 11 exports are fingerprint/component bookkeeping with no port counterpart |
| `Phoenix.LiveView.Engine` | 17 | `liveview.Parse`, `liveview.MustParse`, `(*liveview.Template).Render`, `liveview.Safe` | differs | the change-tracking template compiler. `MACRO-to_safe/2` is cased (escaping); `changed_assign?/2` and `nested_changed_assign?/4` have no counterpart — the port compares rendered values instead of tracking assigns |
| `Phoenix.LiveView.HTMLAlgebra` *(hidden)* | 3 | — | untested | the formatter's layout algebra |
| `Phoenix.LiveView.HTMLEngine` | 12 | `liveview.AttrList`, `liveview.htmlEscape` (unexported) | untested | attribute-level escaping (`attributes_escape/1`, `class_attribute_encode/1`, `void?/1`); the port's `AttrList`/`ClassList` are sorted-map helpers with no upstream analogue, so there is no like-for-like call |
| `Phoenix.LiveView.HTMLFormatter` | 2 | — | missing | `mix format` plugin |
| `Phoenix.LiveView.HTMLFormatter.TagFormatter` | 0 | — | untested | struct |
| `Phoenix.LiveView.Helpers` *(hidden)* | 7 | `liveview.LivePatch`, `liveview.LiveNavigate` | untested | deprecated `~L` / `live_patch` helpers |
| `Phoenix.LiveView.JS` | 61 | `liveview.JS` and its 21 methods | untested | the port has a close analogue (`AddClass`, `Toggle`, `Transition`, …), but upstream's answer is a `%JS{}` struct serialised into an HTML attribute and interpreted by the JS client; there is no server-side result to compare that is not simply the port's own JSON |
| `Phoenix.LiveView.Lifecycle` *(hidden)* | 12 | `liveview.ParamsHandler`, `liveview.InfoHandler` | untested | hook staging around a live process |
| `Phoenix.LiveView.LiveStream` *(hidden)* | 8 | `liveview.Stream`, `liveview.StreamOp` | untested | stream inserts land in the diff under `"stream"`; upstream's shape needs `stream_configure/3` on a live socket |
| `Phoenix.LiveView.Logger` | 9 | — | missing | telemetry logging |
| `Phoenix.LiveView.Plug` *(hidden)* | 2 | — | untested | the router plug |
| `Phoenix.LiveView.ReloadError` *(hidden)* | 2 | — | untested | exception |
| `Phoenix.LiveView.Rendered` | 0 | `liveview.Rendered` | untested | 0 exports (a struct); its `static`/`dynamic` fields are compared indirectly, through the diff |
| `Phoenix.LiveView.Renderer` *(hidden)* | 2 | — | untested | `render/1` resolution at compile time |
| `Phoenix.LiveView.Route` *(hidden)* | 3 | — | untested | route metadata |
| `Phoenix.LiveView.Router` | 8 | `liveview.NewHandler` | untested | `live/3` and `live_session/3` are router macros |
| `Phoenix.LiveView.Session` *(hidden)* | 3 | — | untested | the signed session struct |
| `Phoenix.LiveView.Socket` | 12 | `liveview.Socket` | untested | a `Phoenix.Socket` transport, not the assigns holder the port's `Socket` is |
| `Phoenix.LiveView.Socket.AssignsNotInSocket` *(hidden)* | 0 | — | untested | struct |
| `Phoenix.LiveView.Static` *(hidden)* | 7 | `(*liveview.Handler).ServeHTTP` | untested | the dead render: needs a `Plug.Conn` and an endpoint |
| `Phoenix.LiveView.TagEngine` | 11 | `liveview.Parse` | untested | the HEEx tag engine; the port's template parser has a different grammar (see "template syntax is out of scope") |
| `Phoenix.LiveView.TagEngine.Compiler` *(hidden)* | 1 | — | untested | HEEx compilation |
| `Phoenix.LiveView.TagEngine.Parser` *(hidden)* | 5 | — | untested | HEEx parsing |
| `Phoenix.LiveView.TagEngine.Tokenizer` *(hidden)* | 3 | — | untested | HEEx tokenising |
| `Phoenix.LiveView.TagEngine.Tokenizer.ParseError` *(hidden)* | 4 | — | untested | exception |
| `Phoenix.LiveView.Upload` *(hidden)* | 19 | `liveview.UploadConfig`, `(*liveview.Session).UploadChunk` | untested | needs the upload channel and a writer process |
| `Phoenix.LiveView.UploadChannel` *(hidden)* | 11 | — | untested | a separate channel process per entry |
| `Phoenix.LiveView.UploadConfig` | 19 | `liveview.UploadConfig`, `liveview.UploadOptions` | untested | needs a live socket |
| `Phoenix.LiveView.UploadEntry` | 1 | `liveview.UploadEntry` | untested | needs a live socket |
| `Phoenix.LiveView.UploadTmpFileWriter` *(hidden)* | 4 | — | untested | writer process |
| `Phoenix.LiveView.UploadWriter` | 0 | — | untested | behaviour |
| `Phoenix.LiveView.Utils` *(hidden)* | 38 | `(*liveview.Socket).Assign`, `.Changed`, `.ResetChanges`, `liveview.PutFlash`, `liveview.GetFlash` | untested | `assign/3`'s rule that an identical value is *not* marked changed is mirrored by the Elixir runner and pinned by `ev-label-same-value`; the flash and token halves need an endpoint secret |
| `Phoenix.LiveViewTest` | 86 | — | untested | the whole test client — `live/2`, `render_click/2`, `element/3` — requires an `Endpoint` and a `ClientProxy` process |
| `Phoenix.LiveViewTest.ClientProxy` *(hidden)* | 12 | — | untested | the simulated client process |
| `Phoenix.LiveViewTest.DOM` *(hidden)* | 26 | — | untested | test-only DOM helpers |
| `Phoenix.LiveViewTest.Diff` *(hidden)* | 4 | — | untested | test-only diff merging (the client-side half of the wire format) |
| `Phoenix.LiveViewTest.Element` | 0 | — | untested | struct |
| `Phoenix.LiveViewTest.TreeDOM` *(hidden)* | 30 | — | untested | test-only DOM helpers |
| `Phoenix.LiveViewTest.Upload` | 1 | — | untested | test-only upload client |
| `Phoenix.LiveViewTest.UploadClient` *(hidden)* | 14 | — | untested | test-only upload client |
| `Phoenix.LiveViewTest.Utils` *(hidden)* | 3 | — | untested | test helpers |
| `Phoenix.LiveViewTest.View` | 0 | — | untested | struct |
| `String.Chars.Phoenix.LiveComponent.CID` *(hidden)* | 1 | — | untested | protocol impl |

Sum of the `exports` column: **657**, i.e. the full export count.

## The symbols that actually carry cases

A symbol is `match` only when **every** case attributed to it agrees.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Phoenix.LiveView.Diff.render/4` | `liveview.FullDiff`, `liveview.DiffRendered`, `(*liveview.Session).InitialDiff`, `(*liveview.Session).Event` | differs | 66 (`static-split`, `diff-minimality`, `components`, `events`, and the `render_diff` half of `comprehension`) — 48/66 agree | see the divergence list |
| `Phoenix.LiveView.Diff.to_iodata/1` | `(*liveview.Rendered).HTML` | match | `split-html-counter`, `split-html-nested`, `split-html-plain`, `esc-html-string`, `esc-raw-html`, `comp-html-two`, `comp-html-empty` — 7/7 agree | the *materialised* HTML agrees even where the encoding does not |
| `MACRO-Phoenix.LiveView.Engine.to_safe/2` | `(*liveview.Template).Render` (via the unexported `htmlEscape`) | match | `esc-all-five`, `esc-none`, `esc-double-quote`, `esc-apostrophe`, `esc-existing-entity`, `esc-whitespace`, `esc-unicode`, `esc-script-tag` — 8/8 agree | the port emits `&quot;` and `&#39;`, matching Phoenix rather than Go's `html.EscapeString` |
| `Phoenix.HTML.raw/1` *(from `phoenix_html 4.3.0`, a hard dependency)* | `liveview.Safe` | match | `esc-raw-passthrough`, `esc-raw-quotes` — 2/2 agree | |
| `Phoenix.LiveView.Comprehension` *(struct)* | — | differs | `comp-two-entries`, `comp-one-entry`, `comp-empty`, `comp-five-entries`, `comp-escape-entry`, `comp-multi-slot` — 0/6 agree | the port has no comprehension construct |
| `Phoenix.LiveView.Diff.write_component/4` | `(*liveview.Session).ComponentEvent` | differs | `cmp-event-bump`, `cmp-event-bump-cid2`, `cmp-event-set-number`, `cmp-event-unhandled`, `cmp-event-unknown-cid` — 4/5 agree | only the unknown-cid case diverges |

## Real divergences in the diff format

Ordered as the harness brief asks: non-minimal diffs first, then the wrong
comprehension encoding, then the rest.

### 1. Component statics are re-sent on the first HTTP event (`cmp-parent-change-no-initial`, `cmp-parent-change-no-initial-two`)

Only `Session.InitialDiff()` marks a component's render as sent
(`componentState.sent = st.current` inside `ComponentManager.fullComponents`).
`Handler.ServeHTTP` renders the page server-side on `GET` (`handler.go:100`,
`sess.Mount`) and then handles `POST /event` through `sess.Event`
(`handler.go:272`) — **`InitialDiff()` is never called on the HTTP path**. So
`diffComponents` sees `st.sent == nil`, falls into `DiffRendered(nil, next)` =
`FullDiff(next)`, and re-sends every component's statics and every slot even
though the client already has the document. Changing only the parent's title:

| | diff |
| --- | --- |
| upstream | `{"0":"T2"}` |
| Go port | `{"0":"T2","c":{"1":{"0":"hi","1":"0","s":["<span class=\"badge\">",":","</span>"]}}}` |

With two components the port ships both static lists again. The WebSocket path
(`handler.go:150`) *does* call `InitialDiff()` first, which is why
`cmp-parent-change-only` passes — the bug is specific to the HTTP event
transport. `ev-inc-no-initial` is the control: with no components in the view,
the no-initial path is identical on both sides.

Upstream has no transport on which a client receives an event diff without first
having received the document, so the Elixir runner answers
`session_event_no_initial` exactly as `session_event`. The case therefore asks
the only question that has an answer: *given a client that already holds the
statics, does the event diff re-send them?*

### 2. The comprehension encoding is entirely different (11 of the 14 `comprehension` cases diverge)

Upstream 1.2.9 encodes a comprehension as **one shared statics list plus a keyed
map of per-entry dynamics**: `{"s": [...], "k": {"0": {...}, "1": {...}, "kc": 2}}`.
(This is the successor to the `"d"` dynamics *array* of LiveView ≤ 1.0 — worth
recording, because the array form is what most third-party client
implementations still expect. The `"d"` form does not appear anywhere in 1.2.9.)

The port has no comprehension construct at all: `Template` has a fixed slot
count and no loop syntax. The runner therefore splices one nested `*Rendered`
per entry into a parent whose statics grow with the list — the most faithful
encoding available to it. For `["a","b"]`:

| | diff |
| --- | --- |
| upstream | `{"0":{"k":{"0":{"0":"a"},"1":{"0":"b"},"kc":2},"s":["<li>","</li>"]},"s":["<ul>","</ul>"]}` |
| Go port | `{"0":{"0":"a","s":["<li>","</li>"]},"1":{"0":"b","s":["<li>","</li>"]},"s":["<ul>","","</ul>"]}` |

Two consequences, both costing bandwidth:

- **Per-entry statics are repeated.** `comp-five-entries`: upstream ships
  `["<li>","</li>"]` once, the port ships it five times.
- **Appending one entry invalidates the entire list.** Because the parent's
  statics array grows with the list, `staticsEqual` fails and the whole subtree
  is re-sent as a full diff. `comp-append-one` (`["a"]` → `["a","b"]`):

| | diff |
| --- | --- |
| upstream | `{"0":{"k":{"1":{"0":"b"},"kc":2}}}` |
| Go port | `{"0":{"0":"a","s":["<li>","</li>"]},"1":{"0":"b","s":["<li>","</li>"]},"s":["<ul>","","</ul>"]}` |

  The same happens on removal (`comp-remove-one`) and on growth from empty
  (`comp-grow-from-empty`). Upstream's answer for a removal is
  `{"0":{"k":{"kc":1}}}` — just the new count.

An empty list also differs in kind: upstream renders the comprehension slot as
`""` and keeps the parent's two statics (`{"0":"","s":["<ul>","</ul>"]}`), while
the port has no slot to keep and collapses to a single static
(`{"s":["<ul></ul>"]}`).

Only three of the fourteen agree: the two `render_html` cases (the
materialised HTML is identical even though the encoding is not) and
`comp-unchanged` (both sides emit `{}`). Even changing a single entry's value
without changing the length disagrees — upstream nests the delta under the
comprehension's `"k"` map (`{"0":{"k":{"1":{"0":"z"},"kc":2}}}`) while the port
addresses the entry as a top-level slot (`{"1":{"0":"z"}}`) — so
`comp-change-one` and `comp-rows-change-one` are divergences too.

### 3. Cross-component static sharing is missing (`cmp-initial-two-wire`)

Two components of the same module, initial diff, un-normalised wire form:

| | `"c"` |
| --- | --- |
| upstream | `{"1":{"0":"hi","1":"0","s":["<span class=\"badge\">",":","</span>"]},"2":{"0":"hi","1":"0","s":1}}` |
| Go port | both cids carry the full statics list |

Upstream's `maybe_reuse_static` points cid 2 at cid 1; the port's
`ComponentManager.fullComponents` calls `FullDiff` per component with no sharing.
For a list of N identical components this is N copies of the statics instead of
one. `cmp-initial-two` (the normalised form) passes, which confirms the
*content* is right and only the sharing is absent.

### 4. Minimality is value-compared, not assign-tracked (`min-reassign-same-values`, `min-reassign-one-changed`, `min-reassign-nested`)

When the runtime declares an assign changed, upstream re-sends its slot even if
the rendered value is byte-identical; the port omits it because it compares
values. Upstream `{"0":"A","1":"0"}` vs Go `{}` for two assigns re-assigned to
their existing values.

Here the port is *more* minimal than upstream, so this is a difference in
mechanism rather than a bug — and it is not reachable through
`Phoenix.Component.assign/3`, which refuses to mark a key changed when it is
assigned the value it already holds (`ev-label-same-value` pins that: both sides
answer `{}`). These three cases probe
`Phoenix.LiveView.Engine`'s change-tracking contract directly. They are counted
as divergences rather than marked `"deviation"`, because `HARNESS.md` requires a
deviation to be listed in the library's own `API-DEVIATIONS.md` and the
`liveview` repo has no such file.

### 5. `Counter.HandleEvent("set")` only accepts a Go `int`, so a JSON number is a silent no-op (`ev-set-int`, `ev-set-float`, `ev-set-string`)

`counter.go` does:

```go
case "set":
    if v, ok := payload["value"].(int); ok {
        socket.Assign("count", v)
    }
```

The payload reaches `HandleEvent` after `encoding/json` decoding
(`handler.go` decodes the request body into `map[string]any`), so a JSON number
is **always** a `float64` and **never** an `int`. The assertion can therefore
never succeed for a value that came off the wire, and the event is dropped
without any error:

| case | payload | upstream diff | Go diff |
| --- | --- | --- | --- |
| `ev-set-int` | `{"value": 5}` | `{"1":"5"}` | `{}` |
| `ev-set-float` | `{"value": 5.5}` | `{"1":"5.5"}` | `{}` |
| `ev-set-string` | `{"value": "5"}` | `{"1":"5"}` | `{}` |
| `ev-set-zero` | `{"value": 0}` | `{}` | `{}` |
| `ev-set-missing-value` | `{}` | `{}` | `{}` |

`ev-set-zero` agrees only by accident: upstream assigns `0` over an existing `0`,
which `assign/3` does not mark as changed, so upstream also emits nothing.

`ev-mount-start` is the same bug in `Counter.Mount`
(`params["start"].(int)`): with `{"start": 3}` upstream mounts at `3`, the port
mounts at `0`. And `(*liveview.Socket).GetInt` has the same shape, so the
`"inc"`/`"dec"` events would silently reset a float-typed count to 0 — those
cases pass here only because `Mount` seeds `count` with a real Go `int`.

The upstream mirror does no type check at all, which is faithful: LiveView
assigns whatever the client sent and lets the template stringify it.

### 6. A missing assign renders as empty instead of raising (`split-missing-assign`)

| | |
| --- | --- |
| upstream | raises `KeyError: key :nope not found` — `ok:false` |
| Go port | renders the slot as `""` — `ok:true`, `{"0":"","s":["<p>","</p>"]}` |

`(*Template).Render` does `escape(assigns[f])` on a plain map lookup. HEEx
resolves `@nope` through `assigns.nope`, which raises. A typo'd assign is a
crash upstream and silence in the port.

### 7. An unknown cid is silently ignored (`cmp-event-unknown-cid`)

`Diff.write_component/4` returns `:error` for a cid that was never rendered (the
runner turns that into `ok:false`). `ComponentManager.event` returns `nil` when
the cid is absent, so `Session.ComponentEvent` re-renders and returns an empty
diff — a client addressing a stale cid gets a success reply.

### 8. Scalar stringification: whole floats and lists (`split-float-whole`, `split-list-interp`)

| value | upstream | Go port |
| --- | --- | --- |
| JSON `1.0` | `1.0` | `1` |
| JSON `1.5` | `1.5` | `1.5` (agrees) |
| JSON `["a","b"]` | `ab` — `Phoenix.HTML.Safe` for a list concatenates its members | `[a b]` — `fmt.Sprint` on a `[]any` |
| JSON `true` / `false` / `null` | `true` / `false` / `` | same (agrees) |

The float case is `strconv.FormatFloat(t, 'g', -1, 64)` dropping the `.0` that
Elixir's `Float.to_string/1` keeps. The list case matters more than it looks:
interpolating a slice into a template produces Go's debug formatting in the
middle of a page, where Phoenix produces the concatenated members.

## Behaviour that does agree

Verified case by case, not asserted:

- **The static/dynamic split itself.** Statics count is always
  `len(dynamics) + 1`; a slot in leading, trailing or adjacent position produces
  the empty static in exactly the same place; a template with no slots produces
  a one-element statics list and no numbered keys
  (`split-lead`, `split-trail`, `split-pair-adjacent`, `split-plain-no-slots`).
- **Nested sub-templates.** A nested render's own statics live inside its slot
  under `"s"`, and the parent's statics are unaffected (`split-nested-subtemplate`).
- **All five HTML entities**, including `&quot;` rather than Go's default
  `&#34;`, plus double-escaping of an existing entity, untouched newlines/tabs,
  and non-ASCII left as UTF-8 (8 cases).
- **Raw/safe passthrough** (2 cases).
- **Diff minimality for scalar slots**: only the changed slot travels, statics
  are never re-sent on the second render, an unchanged render produces `{}`, and
  a nested sub-diff is scoped to its own slot with the sibling slot omitted
  entirely (11 of the 15 `diff-minimality` cases).
- **The component encoding**: the parent slot carries the bare integer cid, the
  component's diff lives under `"c"` keyed by that cid, cids start at 1 in
  first-appearance order, an unchanged component contributes nothing at all to a
  parent-only diff (no cid re-send, no `"c"` entry), and a component event
  produces `{"c":{"<cid>":{"<slot>":"<value>"}}}` and nothing else
  (`cmp-initial-one`, `cmp-initial-two`, `cmp-initial-titled`,
  `cmp-parent-change-only`, `cmp-parent-change-only-two`, `cmp-parent-no-change`,
  `cmp-event-bump`, `cmp-event-bump-cid2`, `cmp-event-set-number`,
  `cmp-event-unhandled`).
- **The materialised HTML** for every template compared, comprehensions
  included (7 cases).
- **The mount → event → diff cycle** for `inc`, `dec`, `label` (including an
  escaped label and the `assign/3` identical-value rule), unhandled events, and
  the label/param fallbacks (11 of the 18 `events` cases).

## Counts

Symbol-level, over the whole upstream surface (657 exports across 79 modules):

| status | symbols | note |
| --- | --- | --- |
| `match` | 2 | `Phoenix.LiveView.Diff.to_iodata/1`, `MACRO-Phoenix.LiveView.Engine.to_safe/2` |
| `differs` | 2 | `Phoenix.LiveView.Diff.render/4`, `Phoenix.LiveView.Diff.write_component/4` |
| `missing` | 41 | no Go counterpart at all: all of `Phoenix.LiveView.Async` (11), `AsyncResult` (6), `Logger` (9), `Debug` (4), `Controller` (2), `HTMLFormatter` (2), `ColocatedCSS`/`ColocatedHook`/`ColocatedJS` (7) — the async, telemetry, formatter and colocated-asset stories are simply absent |
| `untested` | 612 | needs a running Phoenix endpoint, a live process, a `Plug.Conn`, a form source, a compile-time macro context, or the JS client. Enumerated module by module above. |

2 + 2 + 41 + 612 = 657.

Outside the 657: `Phoenix.HTML.raw/1` (from `phoenix_html`, a hard dependency of
LiveView) is `match`, and `Phoenix.LiveView.Comprehension` — a zero-export
struct, so not a "symbol" by the reflection above — is `differs`.

Go-only surface (`extra`), from the 161 exported Go symbols: `liveview.Form`
and its 8 methods, `liveview.Flash` and its 8 methods, `liveview.DecodeForm`,
`liveview.DecodeFormString`, `liveview.ClassList`, `liveview.AttrList`,
`liveview.HiddenInputs`, `liveview.AcceptKey`, `liveview.Counter`,
`liveview.PubSub` and its 5 methods, `liveview.Conn`/`liveview.Upgrade` (a
hand-rolled WebSocket implementation), `liveview.NewComponentManager`,
`(*liveview.ComponentManager).EventByID`/`CID`. These are `untested`: they either
have no upstream counterpart (`Counter`, `HiddenInputs`, `AcceptKey`,
`EventByID`) or their upstream counterpart lives outside `phoenix_live_view`
(`Phoenix.PubSub`, `Phoenix.HTML.Form`, `Phoenix.Flash`).

### Parity percentages, with their denominators stated

- **Case pass rate: 69 / 94 = 73.4 %.** The denominator is every case in
  `cases/*.json` that both runners answered. Per group: `static-split` 18/21,
  `escaping` 12/12, `diff-minimality` 12/15, `comprehension` 3/14,
  `components` 10/14, `events` 14/18.
- **Symbol match rate: 3 / 6 = 50 %.** The denominator is the six upstream
  symbols named in `upstreamFn` across the case files, i.e. the ones that have at
  least one case: `Diff.render/4`, `Diff.to_iodata/1`,
  `MACRO-Engine.to_safe/2`, `Phoenix.HTML.raw/1`, `Diff.write_component/4`,
  `Phoenix.LiveView.Comprehension`.
- **Fraction of the upstream surface compared at all: 4 / 657 = 0.6 %** — two
  `match` plus two `differs`; everything else is `missing` or `untested`.

Read them together. `escaping` is at 100 % and the HTML materialisation agrees
everywhere, so the port's escaping and interleaving are genuinely right. The
scalar diff protocol is right too. But the comprehension encoding is a different
format, component statics leak on the HTTP event path and are duplicated across
sibling components, and 614 of LiveView's 657 exports were never exercised
because they require a running Phoenix endpoint or a live process. None of these
numbers should be
read as "73 % of LiveView is ported".
