# Overview

**The Node.js ecosystem, reimagined in Go — one family, one toolkit.**

## What this is

`malcolmston/go` is the umbrella repository for a family of Go ports of the
most-loved building blocks of the Node.js ecosystem. It gathers five
independent Go libraries, a unified documentation site, and a shared React
component library into one place so they can be developed, tested, released,
and documented as a coherent whole.

The five libraries each recreate the ergonomics of a Node original on top of
Go's standard library:

| Library | Ports | Module | Docs |
| ------- | ----- | ------ | ---- |
| express | expressjs/express | `github.com/malcolmston/express` | [pages](https://malcolmston.github.io/express/) |
| passport | jaredhanson/passport | `github.com/malcolmston/passport` | [pages](https://malcolmston.github.io/passport/) |
| socket.io | socketio/socket.io | `github.com/malcolmston/socketio` | [pages](https://malcolmston.github.io/socket.io/) |
| chalk | chalk/chalk (+ inquirer, figlet) | `github.com/malcolmston/chalk` | [pages](https://malcolmston.github.io/chalk/) |
| morgan | expressjs/morgan | `github.com/malcolmston/morgan` | [pages](https://malcolmston.github.io/morgan/) |

Alongside the libraries live two more pieces:

- **`ui/` — go-ui:** a shared Liquid-Glass React component library (package
  `go-ui`) that every documentation site is built from, so the docs share one
  look and feel.
- **`web/` — the unified site:** the home and documentation portal that presents
  every library under one roof, with per-library tabs, Node → Go code
  comparisons, how-to and FAQ pages, and live version/release data.

Each library remains a standalone Go module with its own repository, versioning,
and docs — this repo is the connective tissue that ties them together.

## How it works

**Submodules + a Go workspace.** The five libraries are vendored into this repo
as **git submodules** (see [`.gitmodules`](.gitmodules)), each pinned to its own
upstream repository. A **Go workspace** ([`go.work`](go.work)) lists every
submodule plus the integration example, so cross-module code in this repo
resolves imports to the **local checkouts** instead of published versions. That
means you can change two libraries at once and build code that spans them with
no publishing round-trip.

**One server composing three libraries.** The
[`examples/integration`](examples/integration) command is the proof that the
libraries compose. It wires **express** (JSON routing), **socket.io** (realtime
chat over WebSocket/polling), and **morgan** (request logging) onto a single
`net/http` server. Everything meets at the standard `http.Handler` interface:
Socket.IO's handler intercepts `/socket.io/` and delegates the rest to Express,
and morgan wraps the whole thing:

```go
handler := io.Handler(app)                          // socket.io in front of express
logged  := morgan.New(handler, morgan.Dev, morgan.Config{})
log.Fatal(http.ListenAndServe(":3000", logged))
```

Because the pieces are all plain `http.Handler`s, they layer in any order and
interoperate with the rest of the Go HTTP ecosystem.

**Docs share one component library.** Each library's documentation site vendors
the shared **go-ui** component library as a submodule, so all the sites render
from the same Liquid-Glass components and stay visually consistent. The unified
`web/` site consumes go-ui the same way (`"go-ui": "file:../ui"`).

**Automated releases and live data.** Releases are VERSION-driven: a
[`VERSION`](VERSION) bump produces a tag and a GitHub Release, and a moving
`stable` tag tracks the latest. A submodule-sync pipeline keeps the vendored
libraries current, and the documentation site reads each library's tags and
release notes from the GitHub API **at load time**, so version badges and the
Releases page reflect live state rather than baked-in numbers. See
[`CHANGELOG.md`](CHANGELOG.md) and the [workflows](.github/workflows).

## How to use it

**Just want one library?** You don't need this repo at all. Each library is an
independent module — reach for it with `go get`:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/passport
go get github.com/malcolmston/socketio
go get github.com/malcolmston/chalk
go get github.com/malcolmston/morgan
```

**Want to work across libraries, or run the integration example?** Clone with
submodules so the workspace has real checkouts to build against:

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

Then use the workspace. `go.work` resolves every import to the local submodule,
so nothing needs to be published first:

```sh
go run   ./examples/integration   # express + socket.io + morgan on one server
go build ./examples/integration
```

The example server listens on `:3000`. Try it:

```sh
curl 'http://localhost:3000/api/hello?name=ada'   # {"msg":"hi","who":"ada"}
# and connect a Socket.IO client to /socket.io/ for the realtime chat room
```

## Why this beats reaching for the Node originals

This ecosystem is not trying to win a benchmark or claim the Node projects were
wrong. It exists for teams that like the Express/Passport/Socket.IO/chalk/morgan
programming model but want to ship it on Go's runtime. The honest case:

- **One consistent, stdlib-first toolkit.** All five libraries are built on
  `net/http`, `io`, `context`, and friends — not on a parallel universe of
  framework primitives. Learn the idioms once and they carry across the whole
  family.
- **Near-zero third-party dependencies.** Because the ports lean on the standard
  library, your dependency tree stays small and auditable — far less transitive
  churn than a typical `node_modules` tree, and a smaller surface to keep
  patched.
- **Single static binaries.** `go build` produces one self-contained
  executable. No runtime to install on the target host, no lockfile to
  reconcile at deploy time — copy the binary and run it.
- **Wire-compatible behavior.** The ports aim to match the originals where it
  is observable: Socket.IO speaks the Socket.IO protocol, morgan emits the
  familiar log formats, Express keeps the `app.Get`/`app.Use`/middleware model.
  Existing clients and tooling keep working.
- **Type safety and tooling.** Compile-time checks, `gofmt`, the race detector,
  and Go's testing and profiling tools apply uniformly across every library.
- **Cohesive, auto-generated docs.** One shared component library and one site
  mean the docs for all five libraries look and behave the same, with Node → Go
  comparisons and live version data instead of five divergent sites.

**Tradeoffs, honestly.** These are re-implementations, not the battle-tested
originals — they cover the common surface area, not every edge and plugin of a
decade-old Node package. The npm ecosystems around Express and Passport are
vastly larger, so a strategy or middleware you rely on may not have a port yet.
And if your team lives in JavaScript end-to-end, sharing code between client and
server, staying on Node may simply be the lower-friction choice. Pick this
family when you want Node-shaped ergonomics with Go's deployment story and
dependency discipline — not as a like-for-like drop-in for every Node package
you already use.

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original Node.js projects.
