# fastmcp example

A single runnable program that exercises [`github.com/malcolmston/fastmcp`](https://github.com/Malcolmston/fastmcp)
— a standard-library-only Go port of Python's FastMCP — as an outside consumer
would: the dependency is the published module, with no `replace` directive.

**Module version under test: `github.com/malcolmston/fastmcp v0.4.0`**
(a real semver tag, not a pseudo-version).

## Run

```sh
cd examples/fastmcp
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program drives everything in-process and exits on its own; nothing blocks on
stdin or on a network listener. Runtime is well under a second.

## What it demonstrates

Server side (`buildServer`):

- `fastmcp.New` with `WithVersion` and `WithInstructions`.
- **Tools** in every supported handler shape:
  - `Server.Tool` with a struct argument (`AddArgs`) — the JSON input schema is
    reflected from the struct, including `jsonschema:"description=..."` tags and
    required/optional detection (non-pointer = required).
  - `Server.Tool` with `map[string]any` dynamic arguments.
  - `Server.ToolWithOutput` returning a struct — the reflected `outputSchema` is
    advertised in `tools/list` and each call also returns `structuredContent`.
  - A tool that reports progress and logs through `fastmcp.FromContext(ctx)`:
    `Context.Progress`, `Context.Info`, `Context.Debug`.
  - A tool that performs a **server-to-client** `sampling/createMessage` request
    via `Context.CreateMessage`.
  - A tool that queries the client's roots via `Context.ListRoots`.
  - A tool that returns an error, to show the MCP `isError` convention.
- **Resources**: `Server.Resource` (static text), `Server.ResourceTemplate`
  (`notes://{id}`, path variables extracted into `map[string]string`) and
  `Server.BinaryResource` (base64 `blob` contents).
- **Prompts**: `Server.Prompt` with declared `PromptArgument`s, built from
  `NewUserMessage` / `NewAssistantMessage`.
- **Completion**: `Server.CompletePrompt` and `Server.CompleteResourceTemplate`.
- **Broadcasts**: `NotifyToolsChanged`, `NotifyPromptsChanged`,
  `NotifyResourcesChanged`, `NotifyResourceUpdated` (the last one delivered only
  to subscribers).

Client side:

- `transport.InMemory` + `Transport.Client` (in-process pipes, bidirectional, so
  sampling / roots / notifications all work) with `client.WithClientInfo`,
  `WithRoots`, `WithSamplingHandler`, `WithNotificationHandler`.
- The full request surface: `Initialize`, `Ping`, `ListTools`, `CallTool`,
  `ListResources`, `ReadResource`, `Subscribe`/`Unsubscribe`, `ListPrompts`,
  `GetPrompt`, `CompletePrompt`, `CompleteResource`.
- `contrib.CallToolsBulk` (parallel, bounded concurrency, `ContinueOnError`) and
  `contrib.Timed`.
- `Server.HTTPHandler` behind `httptest.NewServer` driven by `client.NewHTTP`,
  including a demonstration that sampling correctly fails over a transport with
  no server-to-client channel.

## Holes found in v0.4.0

Marked in `main.go` with `// HOLE:` comments.

1. **`logging/setLevel` is advertised but not implemented.** `handleInitialize`
   puts `"logging": {}` in the server's capabilities, but `Server.Dispatch` has
   no `logging/setLevel` case, and `client.Client` has no `SetLoggingLevel`
   method. A client therefore cannot narrow the severity it receives, and
   `Context.Log`/`Debug`/`Info` always deliver every record (visible in the
   example output: the `debug` record arrives even though a real client would
   have asked for `info` and above). The `mcplog` subpackage models levels and
   thresholds, and the v0.4.0 CHANGELOG mentions `logging/setLevel`, but nothing
   wires it into the server or client. (The uncommitted working tree of the
   repo does add `Context.LogLevel`/`LogEnabled` and a `logging/setLevel`
   dispatch case — they are simply not in the published module.)
2. **Resource templates are undiscoverable from the client.** The server
   implements `resources/templates/list`, but `client.Client` exposes no
   `ListResourceTemplates` method in v0.4.0, and `Client.call` is unexported, so
   there is no escape hatch for issuing an arbitrary JSON-RPC method. A client
   built on this package can read `notes://beta` only if the URI template was
   communicated out of band. (Again present in the working tree, absent from
   v0.4.0.)

## Rough edges (not bugs, but worth knowing)

- **`Server.ServeHTTP(addr string) error`** shares its name with the
  `http.Handler` method but not its signature, so `*fastmcp.Server` does *not*
  satisfy `http.Handler` despite appearing to. You must call
  `HTTPHandler()`. The godoc flags this, but it is a real trap for anyone
  writing `http.ListenAndServe(addr, srv)`.
- **Registration panics instead of returning an error.** `Server.Tool` /
  `ToolWithOutput` panic on an unsupported handler shape. Fine for `main`, awkward
  for anything that registers tools from data.
- **Optional scalar tool arguments need pointers.** `Steps int` is emitted as
  `required`, so a handler that wants to default a numeric argument must declare
  `*int` (or accept the client sending an explicit zero). See `CountdownArgs` in
  the example, whose `steps` is required even though the handler defaults it.
- **Dynamic (`map[string]any`) tools advertise only `{"type":"object"}`** — no
  property names at all, so such a tool is not self-describing to a model.
- **`client.CallTool` always attaches a `progressToken`**, whether or not the
  caller wants progress notifications; there is no way to opt out.
- **Protocol version is pinned to `2024-11-05`** (`fastmcp.ProtocolVersion`),
  several MCP revisions behind. The server echoes back whatever version the
  client asks for without checking it supports that revision.
- Reading a resource whose handler fails surfaces as a JSON-RPC error, whereas a
  failing *tool* surfaces as a successful result with `isError: true`. Both match
  the spec, but the asymmetry is easy to trip over.
