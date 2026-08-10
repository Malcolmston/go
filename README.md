# malcolmston/go

[![Library Tests](https://github.com/Malcolmston/go/actions/workflows/library-tests.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/library-tests.yml)
[![Go Workspace](https://github.com/Malcolmston/go/actions/workflows/go-workspace.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/go-workspace.yml)
[![Cross-compile](https://github.com/Malcolmston/go/actions/workflows/cross-compile.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/cross-compile.yml)
[![Web Unit](https://github.com/Malcolmston/go/actions/workflows/web-unit.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/web-unit.yml)
[![Web E2E](https://github.com/Malcolmston/go/actions/workflows/web-e2e.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/web-e2e.yml)
[![Pages](https://github.com/Malcolmston/go/actions/workflows/pages.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/pages.yml)
[![Release](https://img.shields.io/github/v/release/Malcolmston/go?sort=semver)](https://github.com/Malcolmston/go/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Malcolmston/go)](https://github.com/Malcolmston/go/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/Malcolmston/go)](https://github.com/Malcolmston/go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Docs](https://img.shields.io/badge/docs-vercel-2f9bff)](https://go-malcolms-projects-18e573c3.vercel.app)

**The Node.js — and Python, Rust, Elixir, Java — ecosystems, reimagined in Go.**

A unified home for Go libraries that recreate the most-loved building blocks of
other ecosystems with the same ergonomics, on top of Go's standard library.

**Where the project actually stands today:**

| | Count | What that means |
| --- | :---: | --- |
| **Shipped** | **38** | A real Go module: `go.mod` + source, buildable, a workspace member, covered by [Library Tests](.github/workflows/library-tests.yml). |
| **Total submodules** | 38 | Every entry in [`.gitmodules`](.gitmodules). |

All 38 submodules carry Go source and a measured `parity.json`.

> **If a checkout looks empty,** the submodule is pinned to an older commit and
> has not been fetched. `git submodule update --init --remote` brings every
> library up to its default branch. A submodule containing only a `README.md`
> is an un-fetched checkout, not a missing port — the code exists upstream.

🌐 **Unified site & docs:** **<https://go-malcolms-projects-18e573c3.vercel.app>** — a Home
grid, a tab per library with an inline API reference and Node-vs-Go comparisons,
plus How-to, FAQ, AI, and About.

## Shipped libraries

Each is an independent Go module (`github.com/malcolmston/<name>`), vendored
here as a git submodule and tied to the others through the committed
[`go.work`](go.work) workspace.

The **Parity** column is copied from that library's own `parity.json`, the
artifact its parity pipeline publishes (see [Upstream
parity](#upstream-parity--pipeline)). **Cases** is how many upstream cases that
score was computed over — a percentage without a case count is not worth much.
`—` means the port has no `parity.json` yet, i.e. it has never been measured.

### Web & real-time

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [express](express) | expressjs/express (+ npm util ports) | 100% | 33 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/express) |
| [passport](passport) | jaredhanson/passport (+ strategies) | 100% | 34 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/passport) |
| [socket.io](socket.io) | socketio/socket.io | 100% | 20 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/socket.io) |
| [morgan](morgan) | expressjs/morgan | 100% | 54 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/morgan) |
| [react](react) | facebook/react (React 19) | — | — | [readme](react/README.md) |
| [d3](d3) | d3/d3 (the computational modules) | — | — | [readme](d3/README.md) |

**d3** ports the computational half of d3 — `array`, `scale`, `shape`, `path`,
`color`, `interpolate`, `format`, `timefmt`, `hierarchy`, `ease`, `random`,
`dsv`. Selection, transition, drag, zoom and brush are deliberately absent:
they are DOM machinery, and there is no DOM. What remains pairs with **react**,
which renders what d3 computes — see
[`examples/d3-react-chart`](examples/d3-react-chart) for a chart built from both,
where the whole interface between the two is an SVG path string.

React also ships an RSC bridge, [`react/rsc`](react/rsc), which serializes a Go
tree into React's Flight wire format so a real React 19 client can render it —
Go server components travel as data, JS client components stay interactive. The
encoder is verified against React's own decoder.

React ships with a toolchain of its own, [`reactcli`](reactcli) — the `react`
command: project scaffolding, a rebuild-on-save dev server, component
generation, ecosystem integrations such as Tailwind wired in as pinned
standalone binaries, and `react doctor`, which finds the mistakes this port
makes easy to write and the compiler cannot catch. It is a separate module so
that the library itself stays dependency-free.

### CLI, docs & formats

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [chalk](chalk) | chalk/chalk (+ figlet, prompts) | 100% | 30 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/chalk) |
| [jose](jose) | panva/jose (RFC 7515/16/17/18) | 100% | 55 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jose) |
| [markdown](markdown) | commonmark/commonmark-spec | 100% | 652 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/markdown) |
| [yaml](yaml) | yaml/yaml-test-suite | 99.7% | 373 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/yaml) |

### Data, math & ML (Python-inspired)

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [algebra](algebra) | sympy/sympy | 100% | 56 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/algebra) |
| [opencv](opencv) | opencv/opencv | — | — | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/opencv) |
| [streamlit](streamlit) | streamlit/streamlit | 93% | 31 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/streamlit) |

### Tooling & data wrangling

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [fastmcp](fastmcp) | jlowin/fastmcp (Python) | 99% | 161 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/fastmcp) |
| [jq](jq) | jqlang/jq (C) | 92.5% | 782 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jq) |
| [rrule](rrule) | dateutil/dateutil (Python) | 99.6% | 256 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/rrule) |

> Module paths follow `github.com/malcolmston/<name>`, with one exception:
> Socket.IO is `github.com/malcolmston/socketio`.

**Dependencies.** All but one are stdlib-only — no cgo, no third-party
requires. The exception is **chalk**, whose `prompts` subpackage needs
`golang.org/x/term` for raw-mode terminal input; its `go.mod` records that
dependency explicitly.

## Upstream parity & pipeline

Where a port **is** measured, it is measured by running the original library.
Each repo's parity CI, driven by [`parity/`](parity):

1. **installs the upstream package it mirrors** — the real `chalk@5.3.0`,
   `sympy`, gem or crate — and starts it in its own runtime;
2. **asks it and the Go port the same questions**, comparing both answers (and
   both failures — returning a value where upstream throws is a gap);
3. **cross-compiles the port** for every target the family claims; and
4. **publishes `parity.json`**, the measured score with every failing case named.

Because the expectations come from upstream itself, they cannot drift: a new
upstream release re-scores the ports on its own. Where the upstream runtime
cannot be installed, the suite replays upstream's *recorded* answers and the
report says so (`"mode": "golden"`) rather than passing a replay off as a
measurement.

A library with **no `parity.json` has not been through this pipeline**, and the
tables above say `—` rather than inventing a number. As of this writing 13 of
the 14 shipped libraries have published a `parity.json`; **opencv** has not.

Every repo's pipeline routes through one **central reusable workflow**
([`.github/workflows/parity-reusable.yml`](.github/workflows/parity-reusable.yml)),
and the landing regenerates the scores on each deploy from whatever `parity.json`
files are present.

## API reference

The landing renders each documented library's **Go API reference inline** —
package-by-package types, functions, methods, constants and runnable examples —
generated from source by a stdlib-only `go/doc` tool ([`gendocs`](gendocs)) into
`public/docs/<lib>.json`.

Coverage is currently **partial**: of the 14 shipped libraries, 9 have a
generated `DocIndex` (algebra, chalk, express, fastmcp, morgan, opencv, passport,
socket.io, streamlit). **jose, jq, markdown, rrule and yaml do not yet** — they
were added after the last docs generation. Re-running the Pages workflow's
`gendocs` step regenerates the set.

## Clone (with submodules)

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

The 24 planned submodules will check out as directories containing only a
README — that is expected, not a failed clone.

## Use a single library

The libraries are independent — you do **not** need this repo to use them:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/algebra
go get github.com/malcolmston/socketio
# …any of the 14 SHIPPED libraries above, all github.com/malcolmston/<name>
```

## Develop across libraries (workspace)

[`go.work`](go.work) is **committed** and lists all 38 library submodules plus
the in-repo tooling modules (`examples/integration`, `gendocs`, `genparity`,
`parity`), so code in this repo resolves imports to the local checkouts — no
publishing required.

> A `use` directive pointing at a directory with no `go.mod` makes *every* `go`
> command in the repo fail, so an un-fetched submodule breaks the workspace
> rather than being skipped. Run `git submodule update --init --remote` before
> `go` anything.

The repo root is intentionally not itself a Go module, so build with explicit
paths rather than a bare `go build ./...`:

```sh
go build ./examples/integration          # express + socket.io + morgan on one server
go run   ./examples/integration
go build ./express/... ./passport/...    # any subset of members
(cd algebra && go test ./...)            # a member's own test suite
```

See [`examples/integration`](examples/integration) for a runnable server that
composes Express routing, a Socket.IO realtime endpoint, and morgan logging
through the standard `http.Handler` interface.

## The website

The Next.js 15 (App Router, React 19) site that documents the family lives at
the repo root and deploys to both Vercel and GitHub Pages.

```sh
pnpm install        # requires FONTAWESOME_PACKAGE_TOKEN — see below
pnpm dev
pnpm typecheck && pnpm test
pnpm build
```

**`FONTAWESOME_PACKAGE_TOKEN` is required to install.** [`.npmrc`](.npmrc)
points the `@awesome.me` and `@fortawesome` scopes at the Font Awesome Pro
registry, and `package.json` depends on `@awesome.me/kit-61a008692d`. Without
the token in your environment, pnpm prints

```
Failed to replace env in config: ${FONTAWESOME_PACKAGE_TOKEN}
```

on every command and `pnpm install` will 401 on a cold store (an existing
`node_modules`/store cache keeps working, which is why the warning often looks
harmless). Export it before installing:

```sh
export FONTAWESOME_PACKAGE_TOKEN=…      # Font Awesome account → Kits → package token
```

In CI it comes from the `FONTAWESOME_PACKAGE_TOKEN` Actions secret; on Vercel it
is a project environment variable. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Pipelines

CI is split into focused, independently-badged workflows so a failure points
straight at the suite that broke:

- **[Library Tests](.github/workflows/library-tests.yml)** — a matrix over the
  **14 shipped** submodules that checks out each at its latest commit, asserts
  it actually contains a Go module, then runs `go build ./... && go test -race
  ./...`.
- **[Go Workspace](.github/workflows/go-workspace.yml)** — verifies every
  `go.work` member is checked out and builds, then vets `examples/integration`.
- **[Cross-compile](.github/workflows/cross-compile.yml)** — builds the
  integration example across a GOOS/GOARCH matrix.
- **[Web Unit](.github/workflows/web-unit.yml)** — Vitest component tests plus a
  TypeScript type-check for the unified site.
- **[Web E2E](.github/workflows/web-e2e.yml)** — the Playwright device sweep
  against a production build of the site.
- **[Pages](.github/workflows/pages.yml)** — regenerates the per-library API
  docs + parity metrics and publishes the unified site.
- **[Parity (reusable)](.github/workflows/parity-reusable.yml)** — the central
  `workflow_call` every library's own `parity.yml` routes through.

Every Go workflow pins its toolchain with `go-version-file: go.work`, so the
`go` directive in the workspace is the single place the Go version is declared.

## Security

Vulnerability reporting and the automated-scanning inventory are in
[SECURITY.md](SECURITY.md).

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original projects.
