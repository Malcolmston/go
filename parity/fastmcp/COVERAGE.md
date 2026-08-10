# `fastmcp` parity coverage

- **Upstream oracle**: PyPI `fastmcp` **3.4.6** (with `mcp` **1.29.0**, pydantic
  2.13), installed into a project-local virtualenv at
  [`python/.venv`](python/) — never globally. The harness pins it as
  `fastmcp==3.4.6` and refuses to score a venv holding any other version.
- **Go port**: `github.com/malcolmston/fastmcp` **v0.4.0**, consumed as a
  published module (no `replace` directive); `go.mod` in this directory is the
  only place it is required.
- **Harness**: `GOWORK=off go test ./parity/fastmcp/` — **110 cases** in
  `cases/*.json`.
- **The compared artefact is Model Context Protocol wire behaviour.** Each case
  builds an equivalent server on both sides (same tool names, argument names and
  types, same resources, same prompts — see
  [`python/server_def.py`](python/server_def.py) and
  [`go/run.go`](go/run.go)), completes an `initialize` handshake, and issues one
  or more JSON-RPC requests **in-process**: Python over `anyio` memory object
  streams into `mcp.server.lowlevel.Server.run`, Go over an `io.Pipe` pair into
  `fastmcp.Server.ServeStdio`. No socket is opened and no model provider is
  contacted.
- Each case value is a list of outcomes, one per request:
  `{"kind":"result","result":…}` or `{"kind":"error","code":…}`. **Error cases
  compare the JSON-RPC code only** — message text is never compared, it is
  recorded here.
- An optional second argument to a case is a `/`-separated *path* into the
  result, so a single field (e.g. `tools/#name=greet/inputSchema/required`) can
  be compared in isolation instead of drowning in unrelated diffs. A path that
  cannot be followed yields JSON `null`, so "present on one side, absent on the
  other" scores as a mismatch rather than as two agreeing failures.

## Normalisation

Applied identically by both runners before comparison:

| what | why |
| --- | --- |
| JSON-RPC `id` dropped (only `result`/`error.code` is kept) | request ids are counters |
| `nextCursor` dropped | pagination cursor, opaque and implementation-specific |
| any key starting with `_` (e.g. `_meta`) dropped | framework-private annotations |
| `meta`, `lastModified`, `timestamp` dropped | clocks and framework metadata |
| `error.message` / `error.data` dropped | message text is not a parity signal |
| arrays named `required`, `values`, `tags` sorted | JSON Schema `required` order is not semantic |
| `tools`/`prompts`/`arguments` sorted by `name`, `resources` by `uri`, `resourceTemplates` by `uriTemplate` | registration order is not part of the spec |
| object keys unordered | both sides decode to maps before `DeepEqual` |
| all JSON numbers compared as `float64` | `1` and `1.0` are equal |
| `PYTHONHASHSEED=0`, fresh server per case | determinism |

Nothing else is stripped. In particular `additionalProperties`,
`x-fastmcp-wrap-result`, `capabilities.extensions` and `isError` are **kept**,
because a real client sees them.

## How the inventories were produced

Two inventories, both derived mechanically from the *installed* upstream, never
from the README:

```sh
cd python
# (1) the MCP wire-method inventory: every literal `method` value declared by a
#     Request/Notification model in the pinned mcp package
./.venv/bin/python - <<'PY'
import inspect, typing, mcp.types as t
methods = set()
for name, obj in vars(t).items():
    if inspect.isclass(obj) and (name.endswith("Request") or name.endswith("Notification")):
        f = getattr(obj, "model_fields", {}).get("method")
        if f is not None:
            args = typing.get_args(f.annotation)
            if args:
                methods.add(args[0])
print(len(methods)); print("\n".join(sorted(methods)))
PY

# (2) the FastMCP class inventory
./.venv/bin/python -c "from fastmcp import FastMCP; \
  n=[x for x in dir(FastMCP) if not x.startswith('_')]; print(len(n)); print('\n'.join(sorted(n)))"
```

That yields **31** MCP methods and **51** public `FastMCP` members. The Go side
was enumerated with:

```sh
M=$(GOWORK=off go list -m -f '{{.Dir}}' github.com/malcolmston/fastmcp)
grep -h '^func (s \*Server) [A-Z]\|^func [A-Z]\|^type [A-Z]' $M/*.go | grep -v '_test'
ls -d $M/*/          # sub-packages: auth client elicit jsonschema mcperror mcplog
                     # middleware mount openapi proxy transport uritemplate web
```

## Divergences that break real clients

### 1. `logging` is advertised but not implemented (interop bug)

Both servers advertise `capabilities.logging = {}`. Upstream then answers
`logging/setLevel` with `{}`; the port has no `logging/setLevel` case in
`Server.route` and returns **-32601 method not found**
(`cap-logging-setlevel`, `cap-logging-setlevel-bad-level`). A client that trusts
the advertised capability and calls `logging/setLevel` gets a hard JSON-RPC
error. This is the single clearest spec violation found.

### 2. `resources.subscribe: true` is advertised where upstream advertises `false`

The port advertises `resources: {"listChanged": true, "subscribe": true}`
unconditionally whenever any resource or template is registered, and *does*
answer `resources/subscribe`/`resources/unsubscribe` with `{}`. Upstream 3.4.6
advertises `subscribe: false` (it registers no subscribe handler) and answers
**-32601** (`init-cap-resources`, `resources-subscribe`,
`resources-unsubscribe`). Here the port is *ahead* of upstream, and its
advertisement is honest — but the two capability objects disagree, so a client
written against one will mis-plan against the other.

### 3. `completions: {}` is advertised where upstream advertises nothing

The port always advertises `capabilities.completions = {}` and implements
`completion/complete`, returning `{"completion":{"values":[],"total":0,"hasMore":false}}`
even for an unknown or malformed `ref` type. Upstream 3.4.6 registers no
completion handler: the capability is absent and the method answers **-32601**
(`init-cap-completions`, `complete-prompt-arg`, `complete-resource-template`,
`complete-bad-ref-type`). The port advertises `completions` *even when no
completer is registered*, which is the same class of over-advertisement as
`logging`.

### 4. `capabilities.experimental` / `capabilities.extensions` absent

Upstream emits `experimental: {}` and
`extensions: {"io.modelcontextprotocol/ui": {}}`; the port emits neither
(`init-cap-experimental`, `init-capabilities`, `init-full`).

### 5. `protocolVersion` is echoed, never negotiated

`fastmcp.ProtocolVersion` is pinned to `2024-11-05` (upstream's
`mcp.types.LATEST_PROTOCOL_VERSION` is `2025-11-25`), but the port's real
behaviour is worse than a stale pin: `handleInitialize` **echoes whatever the
client sent** and only falls back to its own constant when the field is absent.
Asked for `1999-01-01` it replies `1999-01-01`; asked for `not-a-version` it
replies `not-a-version`. Upstream negotiates down to a version it actually
supports (`init-protocol-unsupported`, `init-protocol-garbage`). Echoing an
unsupported version is a protocol violation — the client is told a revision is
in force that neither party implements.

## JSON Schema divergences (`tools/list`)

The generated argument schema is what every MCP client uses to build and
validate tool calls, so these are the highest-impact differences after the
capability bugs.

| # | case | upstream | port | consequence |
| --- | --- | --- | --- | --- |
| 1 | `schema-greet-required` | `required: ["name"]` | `required: ["greeting","name"]` | **a defaulted scalar is marked required.** `jsonschema:"default=Hello"` sets the `default` keyword but `isOptional` only looks at pointer-ness and `,omitempty`, so the field stays required. Clients refuse to call the tool without it. |
| 2 | `schema-search-required` | `required: ["query"]` | `required: ["limit","query","tags","verbose"]` | same bug across int, bool and slice defaults |
| 3 | `schema-echo-map`, `schema-echo-map-properties` | `{"type":"object","additionalProperties":false,"properties":{"payload":{…}},"required":["payload"]}` | `{"type":"object"}` | **a dynamic `map[string]any` tool publishes no argument information at all.** `buildToolEntry` short-circuits the whole schema to `{"type":"object"}`, so `properties` is absent. A client cannot discover that `payload` exists. |
| 4 | `schema-search-default-int` | `10` (number) | `"10"` (string) | `parseJSONSchemaTag` stores every tag value as a string, so `default` has the wrong JSON type |
| 5 | `schema-search-default-bool` | `false` | `"false"` | as above |
| 6 | `schema-search-default-array` | `[]` | `"[]"` | as above |
| 7 | `schema-nickname-nullable` | `{"anyOf":[{"type":"string"},{"type":"null"}],"default":null,…}` | `{"type":"string",…}` | a `*string` field is optional on both sides, but the port's schema forbids the explicit `null` its own handler accepts |
| 8 | `schema-add`, `schema-greet`, `schema-search`, `schema-stats-input`, `schema-echo-map` | `"additionalProperties": false` | absent | the port never closes an object, so unknown arguments are silently dropped instead of rejected (`call-extra-arg`) |
| 9 | `schema-stats-output` | carries the model docstring as `description` | no `description` | `reflectStructSchema` ignores Go doc comments (it has no access to them) |
| 10 | `schema-add-output`, `descriptor-add` | every tool gets an inferred `outputSchema` (scalar returns wrapped as `{"result":…}` with `x-fastmcp-wrap-result: true`) | only `ToolWithOutput` tools get one | structured-output clients see no schema for ordinary tools |
| 11 | `descriptor-add-title`, `descriptor-add-annotations` | absent on both (upstream omits when unset) | absent | **match** — the port simply has no `title`/`annotations` field to set |

## `tools/call` divergences

| case | upstream | port |
| --- | --- | --- |
| `call-greet-default` | `"Hello, Ada!"` | `", Ada!"` — **the declared default is never applied**; the zero value is used |
| `call-search-defaults` | `"mcp\|10\|false\|"` | `"mcp\|0\|false\|"` — same |
| `call-add-structured` | `structuredContent: {"result": 5}` | absent — the port only emits `structuredContent` for `ToolWithOutput` |
| `call-add-iserror` | `isError: false` on success | field omitted |
| `call-missing-required-arg` | `isError: true` (pydantic validation) | success — `json.Unmarshal` zero-fills the missing field |
| `call-extra-arg` | `isError: true` (`additionalProperties: false`) | success — the extra key is dropped |
| `call-no-arguments-key` | `isError: true` | success |
| `call-stats-content` | text `{"count":3,"total":6.0,"mean":2.0}` | text `{"count":3,"total":6,"mean":2}` — `encoding/json` drops the float `.0` |
| `tools-call-unknown-tool` | `isError: true` inside a successful result (per the MCP tool-error convention) | JSON-RPC **-32602** — a transport-level error where the spec wants a tool-level one |
| `tools-call-arguments-not-object` | JSON-RPC **-32602** | `isError: true` |
| `tools-call-params-not-object` | the upstream session raises and tears the connection down (reported `ok:false`) | **-32602** |

Matching: `call-add`(content), `call-add-negative`, `call-greet-both`,
`call-search-explicit`, `call-echo-map`, `call-nickname-set/unset/null`,
`call-stats-structured`, `call-stats-empty`, `call-fail-iserror`,
`call-fail-content-type`, `call-wrong-arg-type`, `tools-call-no-params`,
`tools-call-name-wrong-type`.

## Resource, prompt and error-code divergences

| case | upstream | port |
| --- | --- | --- |
| `resources-read-unknown`, `resources-read-template-slash` | **-32002** (`RESOURCE_NOT_FOUND`, an MCP-specific code) | **-32602** — the port has the constant in `mcperror.CodeResourceNotFound` but `handleResourcesRead` does not use it |
| `resources-read-template-encoded` | `res://user/a%20b` → `user:a b` (percent-decoded) | `user:a%20b` — `compileURITemplate` never decodes |
| `unknown-method`, `unknown-namespaced-method` | **-32602** (the method fails `ClientRequest` schema validation) | **-32601** — the port is arguably more correct here, but the codes differ |
| `prompts-get-unknown` | **code 0** (mcp's lowlevel handler wraps any handler exception as `ErrorData(code=0)`) | **-32602** |
| `prompts-get-missing-required` | error (code 0) — the required argument is enforced | success, rendering the prompt with an empty string |
| `prompt-arg-required-style` | `required: false` | field omitted — `PromptArgument.Required` is `,omitempty` |
| `prompt-args-summarize`, `prompt-descriptor-review`, `prompts-list-full` | each argument description is suffixed with `"\n\nProvide as a JSON string matching the following schema: {…}"` | plain description |

Matching: `resources-list-full`, `resources-list-uris`,
`resource-descriptor-greeting`, `resource-descriptor-logo`,
`templates-list-full`, `template-descriptor-user`, `resources-read-text`,
`resources-read-json`, `resources-read-blob` (base64 identical),
`resources-read-template`, `resources-read-missing-uri`,
`resources-read-uri-wrong-type`, `sequence-read-then-read`,
`prompts-get-summarize`, `prompts-get-summarize-default`, `prompts-get-review`,
`prompts-get-description`, `prompts-get-no-name`,
`prompts-get-arguments-not-object`, `ping`, `sequence-list-then-call`,
`tools-list-with-cursor`, `init-serverinfo`, `init-instructions`,
`init-cap-tools`, `init-cap-prompts`, `init-cap-logging`,
`init-protocol-latest`, `init-protocol-2025-06-18`,
`init-protocol-2024-11-05`.

### Directional note on templates

The task asked whether the port's *client* can list resource templates. It
cannot: `client/` in v0.4.0 exposes no `resources/templates/list` call, even
though the **server** implements the method correctly (`templates-list-full` and
`template-descriptor-user` both match). A port server is therefore reachable by
upstream clients, but a port client cannot discover templates on any server.

## Inventory A — MCP wire methods (31 symbols, from `mcp` 1.29.0)

| upstream method | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `initialize` | `Server.handleInitialize` | differs | `init-*` (15) | capabilities differ; version echoed not negotiated |
| `ping` | `Server.route("ping")` | match | `ping`, `sequence-list-then-call` | |
| `tools/list` | `Server.handleToolsList` | differs | `tools-list-*`, `schema-*`, `descriptor-*` (26) | see JSON Schema table |
| `tools/call` | `Server.handleToolsCall` | differs | `call-*`, `tools-call-*` (26) | defaults not applied; no `structuredContent`/`isError` |
| `resources/list` | `Server.handleResourcesList` | match | `resources-list-full`, `resources-list-uris`, `resource-descriptor-*` | |
| `resources/read` | `Server.handleResourcesRead` | differs | `resources-read-*` (7) | `-32602` instead of `-32002`; no percent-decoding |
| `resources/templates/list` | `Server.handleResourceTemplatesList` | match | `templates-list-full`, `template-descriptor-user` | server matches; the port's **client** cannot call it |
| `resources/subscribe` | `Server.handleSubscribe` | differs | `resources-subscribe` | port implements, upstream 3.4.6 answers `-32601` |
| `resources/unsubscribe` | `Server.handleUnsubscribe` | differs | `resources-unsubscribe` | as above |
| `prompts/list` | `Server.handlePromptsList` | differs | `prompts-list-full`, `prompt-*` (6) | `required:false` omitted; no schema hint in descriptions |
| `prompts/get` | `Server.handlePromptsGet` | differs | `prompts-get-*` (7) | missing required argument tolerated |
| `completion/complete` | `Server.handleComplete` | extra | `complete-prompt-arg`, `complete-resource-template`, `complete-bad-ref-type` | Go-only in 3.4.6; advertised even with no completer |
| `logging/setLevel` | — | missing | `cap-logging-setlevel`, `cap-logging-setlevel-bad-level` | **advertised but unimplemented** |
| `tasks/get` | — | missing | — | MCP tasks not ported |
| `tasks/list` | — | missing | — | |
| `tasks/cancel` | — | missing | — | |
| `tasks/result` | — | missing | — | |
| `notifications/initialized` | `Server.route` (no-op) | match | every `rpc` case (handshake) | both accept and return no reply |
| `notifications/cancelled` | `Server.route` (no-op) | untested | — | accepted and ignored by both |
| `notifications/progress` | `Context.Progress` / `Server.route` | untested | — | requires an out-of-band notification channel to score |
| `notifications/message` | `Context.Log` | untested | — | as above |
| `notifications/tools/list_changed` | `Server.NotifyToolsChanged` | untested | — | as above |
| `notifications/prompts/list_changed` | `Server.NotifyPromptsChanged` | untested | — | as above |
| `notifications/resources/list_changed` | `Server.NotifyResourcesChanged` | untested | — | as above |
| `notifications/resources/updated` | `Server.NotifyResourceUpdated` | untested | — | as above |
| `notifications/roots/list_changed` | — | missing | — | not ported |
| `notifications/tasks/status` | — | missing | — | not ported |
| `notifications/elicitation/complete` | — | missing | — | not ported |
| `sampling/createMessage` | `Context.CreateMessage` | untested | — | server→client; needs a scripted client |
| `roots/list` | `Context.ListRoots` | untested | — | as above |
| `elicitation/create` | `elicit` package | untested | — | as above |

**Inventory A counts** — match 4, differs 8, missing 8, extra 1, untested 10
(total 31). Compared = match + differs = **12**; parity = 4/12 = **33.33 %**.

## Inventory B — public `FastMCP` members (51 symbols, from `dir(FastMCP)`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `FastMCP.name` | `Server.Name` | match | `init-serverinfo` | |
| `FastMCP.version` | `WithVersion` / `Server.Version` | match | `init-serverinfo` | |
| `FastMCP.instructions` | `WithInstructions` | match | `init-instructions` | |
| `FastMCP.tool` | `Server.Tool` | differs | `schema-*` | schema generation |
| `FastMCP.add_tool` | `Server.Tool` | differs | `schema-*` | |
| `FastMCP.call_tool` | `Server.handleToolsCall` | differs | `call-*` | |
| `FastMCP.list_tools` | `Server.handleToolsList` | differs | `tools-list-*` | |
| `FastMCP.resource` | `Server.Resource` / `BinaryResource` | match | `resources-read-*` | |
| `FastMCP.add_resource` | `Server.Resource` | match | `resource-descriptor-*` | |
| `FastMCP.add_template` | `Server.ResourceTemplate` | match | `template-descriptor-user` | |
| `FastMCP.list_resources` | `Server.handleResourcesList` | match | `resources-list-*` | |
| `FastMCP.list_resource_templates` | `Server.handleResourceTemplatesList` | match | `templates-list-full` | |
| `FastMCP.read_resource` | `Server.handleResourcesRead` | differs | `resources-read-unknown` | error code, percent-decoding |
| `FastMCP.prompt` | `Server.Prompt` | differs | `prompt-args-summarize` | |
| `FastMCP.add_prompt` | `Server.Prompt` | differs | `prompt-descriptor-review` | |
| `FastMCP.list_prompts` | `Server.handlePromptsList` | differs | `prompts-list-full` | |
| `FastMCP.render_prompt` | `Server.handlePromptsGet` | differs | `prompts-get-missing-required` | |
| `FastMCP.get_prompt` | — | missing | — | no exported registry getter |
| `FastMCP.get_tool` | — | missing | — | |
| `FastMCP.get_tool_by_hash` | — | missing | — | |
| `FastMCP.get_resource` | — | missing | — | |
| `FastMCP.get_resource_template` | — | missing | — | |
| `FastMCP.remove_tool` | — | missing | — | registration is append-only in the port |
| `FastMCP.disable` / `.enable` | — | missing | — | no runtime enable/disable |
| `FastMCP.icons` | — | missing | — | no `Icon` type |
| `FastMCP.website_url` | — | missing | — | |
| `FastMCP.lifespan` | — | missing | — | no lifespan hook |
| `FastMCP.generate_name` | — | missing | — | |
| `FastMCP.add_provider` | — | missing | — | providers not ported |
| `FastMCP.local_provider` | — | missing | — | |
| `FastMCP.transforms` | — | missing | — | tool transformation not ported |
| `FastMCP.add_transform` | — | missing | — | |
| `FastMCP.wrap_transform` | — | missing | — | |
| `FastMCP.add_tool_transformation` | — | missing | — | |
| `FastMCP.remove_tool_transformation` | — | missing | — | |
| `FastMCP.docket` | — | missing | — | task queue not ported |
| `FastMCP.get_tasks` | — | missing | — | |
| `FastMCP.get_app_tool` | — | missing | — | apps not ported |
| `FastMCP.from_fastapi` | — | missing | — | no FastAPI analogue |
| `FastMCP.from_openapi` | `openapi` package | untested | — | out of scope for wire parity |
| `FastMCP.mount` | `mount` package | untested | — | |
| `FastMCP.import_server` | `mount` package | untested | — | |
| `FastMCP.as_proxy` | `proxy` package | untested | — | |
| `FastMCP.add_middleware` | `middleware` package | untested | — | |
| `FastMCP.custom_route` | `Server.HTTPHandler` | untested | — | HTTP transport not scored |
| `FastMCP.http_app` | `Server.HTTPHandler` | untested | — | |
| `FastMCP.run` | `Server.Run` | untested | — | transport, not wire behaviour |
| `FastMCP.run_async` | `Server.Run` | untested | — | |
| `FastMCP.run_stdio_async` | `Server.ServeStdio` | untested | — | used *by* the harness, not scored |
| `FastMCP.run_http_async` | `Server.ServeHTTP` | untested | — | |

**Inventory B counts** — match 8, differs 9, missing 23, extra 0, untested 11
(total 51). Compared = **17**; parity = 8/17 = **47.06 %**.

## Go-only surface (`extra`)

Not present in upstream 3.4.6 and therefore unscorable against it:
`Server.ToolWithOutput` (upstream infers an output schema for *every* tool),
`Server.BinaryResource` / `BinaryResourceTemplate` (upstream infers `bytes` from
the return annotation), `Server.CompletePrompt` /
`Server.CompleteResourceTemplate` and the whole `completion/complete` handler,
`Server.handleSubscribe` / `handleUnsubscribe`, `Server.NotifyResourceUpdated`,
and the `mcperror`, `uritemplate` and `jsonschema` packages.

## Score

| denominator | match | parity |
| --- | --- | --- |
| **cases** (`cases/*.json`) | 55 / 110 | **50.00 %** |
| **symbols actually compared** (A+B: match + differs) | 12 / 29 | **41.38 %** |
| symbols compared, MCP wire methods only (A) | 4 / 12 | 33.33 % |
| symbols compared, `FastMCP` API only (B) | 8 / 17 | 47.06 % |
| all enumerated symbols (A+B, 82) | 12 / 82 | 14.63 % |

Cases: **110** total, **55** match, **55** mismatch, **0** declared deviations.
Per-group totals live in [`parity.json`](parity.json), rewritten by every
complete `go test` run. Nothing in this directory hand-writes an expected value:
re-pinning `fastmcp` to a newer release re-scores the port automatically.
