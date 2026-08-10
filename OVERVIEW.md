# Overview

**Node.js, Python, Rust, Elixir and Java building blocks, reimagined in Go — one
family, one toolkit.**

## What this is

`malcolmston/go` is the umbrella repository for a family of Go ports of
well-known libraries from other ecosystems. It gathers the library modules, a
unified documentation site, and a shared React component library into one place
so they can be developed, tested, released, and documented as a coherent whole.

**Scope, stated plainly.** [`.gitmodules`](.gitmodules) lists **38** submodules,
and all 38 contain a Go module. A checkout that looks empty is pinned to an old
commit — `git submodule update --init --remote` fetches the real source.

The 38 shipped libraries each recreate the ergonomics of an original from
another ecosystem on top of Go's standard library:

| Library | Ports | Module | Docs |
| ------- | ----- | ------ | ---- |
| algebra | sympy/sympy (Python) | `github.com/malcolmston/algebra` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/algebra) |
| chalk | chalk/chalk (+ figlet, prompts) | `github.com/malcolmston/chalk` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/chalk) |
| express | expressjs/express | `github.com/malcolmston/express` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/express) |
| fastmcp | jlowin/fastmcp (Python) | `github.com/malcolmston/fastmcp` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/fastmcp) |
| jose | panva/jose | `github.com/malcolmston/jose` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jose) |
| jq | jqlang/jq (C) | `github.com/malcolmston/jq` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jq) |
| markdown | commonmark/commonmark-spec | `github.com/malcolmston/markdown` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/markdown) |
| morgan | expressjs/morgan | `github.com/malcolmston/morgan` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/morgan) |
| opencv | opencv/opencv (Python) | `github.com/malcolmston/opencv` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/opencv) |
| passport | jaredhanson/passport | `github.com/malcolmston/passport` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/passport) |
| rrule | dateutil/dateutil (Python) | `github.com/malcolmston/rrule` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/rrule) |
| socket.io | socketio/socket.io | `github.com/malcolmston/socketio` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/socket.io) |
| streamlit | streamlit/streamlit (Python) | `github.com/malcolmston/streamlit` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/streamlit) |
| yaml | yaml/yaml-test-suite | `github.com/malcolmston/yaml` | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/yaml) |

Alongside the libraries live two more pieces:

- **`ui/` — go-ui:** a shared Liquid-Glass React component library (package
  `go-ui`) that every documentation site is built from, so the docs share one
  look and feel.
- **the unified site:** a Next.js 15 App Router app **at the repository root**
  (`app/`, `src/`, `api/`) that presents every library under one roof, with
  per-library tabs, Node → Go code comparisons, how-to and FAQ pages, and live
  version/release data. It was previously under `web/`; that directory no longer
  exists.

Each library remains a standalone Go module with its own repository, versioning,
and docs — this repo is the connective tissue that ties them together.

## How it works

**Submodules + a Go workspace.** The libraries are vendored into this repo as
**git submodules** (see [`.gitmodules`](.gitmodules)), each pinned to its own
upstream repository. A **committed Go workspace** ([`go.work`](go.work)) lists
every submodule *that has a `go.mod`*, plus the in-repo tooling modules
(`examples/integration`, `gendocs`, `genparity`, `parity`), so cross-module code
resolves imports to the **local checkouts** instead of published versions. You
can change two libraries at once and build code that spans them with no
publishing round-trip.

Two rules keep the workspace honest:

- **Only directories with a `go.mod` may be `use`d.** Listing a placeholder
  submodule makes every `go` command fail with "directory … does not contain
  modules". This is why the 24 planned ports are absent.
- **The repo root is not itself a module.** A bare `go build ./...` at the root
  therefore reports "directory prefix . does not contain modules listed in
  go.work"; build with explicit paths instead (`go build ./examples/integration`,
  `go build ./express/...`, `cd algebra && go test ./...`).

Because `go.work` is committed, CI does not reconstruct it — every Go workflow
reads its member list and its toolchain version (`go-version-file: go.work`)
straight from the file, so there is exactly one definition of "what is in this
family".

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

`examples/integration` requires those three modules at the placeholder version
`v0.0.0`, which exists on no proxy. `go.work` therefore carries version-pinned
`replace` directives alongside the `use` block so the module *graph* closes
offline, not just the imports — without them, any command that has to load the
full graph (resolving chalk's `golang.org/x/term`, for instance) tries to fetch
`v0.0.0` and fails.

**Docs share one component library.** Each library's documentation site vendors
the shared **go-ui** component library as a submodule, so all the sites render
from the same Liquid-Glass components and stay visually consistent. The unified
site consumes go-ui the same way (`"go-ui": "file:./ui"`).

**Automated releases and live data.** Releases are VERSION-driven: a
[`VERSION`](VERSION) bump produces a tag and a GitHub Release, and a moving
`stable` tag tracks the latest. A submodule-sync pipeline keeps the vendored
libraries current, and the documentation site reads each library's tags and
release notes from the GitHub API **at load time**, so version badges and the
Releases page reflect live state rather than baked-in numbers. See
[`CHANGELOG.md`](CHANGELOG.md) and the [workflows](.github/workflows).

## How to use it

**Just want one library?** You don't need this repo at all. Each shipped library
is an independent module — reach for it with `go get`:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/passport
go get github.com/malcolmston/socketio
go get github.com/malcolmston/chalk
go get github.com/malcolmston/morgan
```

(Only the 14 listed above resolve. The planned ports have nothing to fetch.)

**Want to work across libraries, or run the integration example?** Clone with
submodules so the workspace has real checkouts to build against:

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

Then use the committed workspace — nothing needs to be published first:

```sh
go run   ./examples/integration   # express + socket.io + morgan on one server
go build ./examples/integration
```

The example server listens on `:3000`. Try it:

```sh
curl 'http://localhost:3000/api/hello?name=ada'   # {"msg":"hi","who":"ada"}
# and connect a Socket.IO client to /socket.io/ for the realtime chat room
```

**Want to work on the website?** It builds with pnpm from the repo root and
needs the Font Awesome Pro package token — see
[README → The website](README.md#the-website) and
[CONTRIBUTING.md](CONTRIBUTING.md).

## Why this beats reaching for the originals

This ecosystem is not trying to win a benchmark or claim the upstream projects
were wrong. It exists for teams that like the Express/Passport/Socket.IO/chalk/
morgan programming model but want to ship it on Go's runtime. The honest case:

- **One consistent, stdlib-first toolkit.** The libraries are built on
  `net/http`, `io`, `context`, and friends — not on a parallel universe of
  framework primitives. Learn the idioms once and they carry across the family.
- **Near-zero third-party dependencies.** 13 of the 14 shipped modules have an
  empty `require` block; chalk's `prompts` subpackage needs `golang.org/x/term`
  and nothing else. Your dependency tree stays small and auditable — far less
  transitive churn than a typical `node_modules` tree, and a smaller surface to
  keep patched.
- **Single static binaries.** `go build` produces one self-contained
  executable. No runtime to install on the target host, no lockfile to
  reconcile at deploy time — copy the binary and run it.
- **Wire-compatible behavior.** The ports aim to match the originals where it
  is observable: Socket.IO speaks the Socket.IO protocol, morgan emits the
  familiar log formats, Express keeps the `app.Get`/`app.Use`/middleware model.
  Existing clients and tooling keep working.
- **Type safety and tooling.** Compile-time checks, `gofmt`, the race detector,
  and Go's testing and profiling tools apply uniformly across every library.
- **Measured, not asserted, fidelity.** Where a port publishes a `parity.json`,
  the number behind it came from running the real upstream package and diffing
  the answers — and where it does not, the docs say `—` instead of guessing.

**Tradeoffs, honestly.** These are re-implementations, not the battle-tested
originals — they cover the common surface area, not every edge and plugin of a
decade-old package. The npm ecosystems around Express and Passport are vastly
larger, so a strategy or middleware you rely on may not have a port yet. Much of
the intended catalogue is still unwritten: two thirds of the submodules in this
repo are placeholders. And if your team lives in JavaScript end-to-end, sharing
code between client and server, staying on Node may simply be the lower-friction
choice. Pick this family when you want those ergonomics with Go's deployment
story and dependency discipline — not as a like-for-like drop-in for every
package you already use.

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original projects.
