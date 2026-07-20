# malcolmston/go

[![Library Tests](https://github.com/Malcolmston/go/actions/workflows/library-tests.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/library-tests.yml)
[![Go Workspace](https://github.com/Malcolmston/go/actions/workflows/go-workspace.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/go-workspace.yml)
[![Cross-compile](https://github.com/Malcolmston/go/actions/workflows/cross-compile.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/cross-compile.yml)
[![Web Unit](https://github.com/Malcolmston/go/actions/workflows/web-unit.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/web-unit.yml)
[![Web E2E](https://github.com/Malcolmston/go/actions/workflows/web-e2e.yml/badge.svg)](https://github.com/Malcolmston/go/actions/workflows/web-e2e.yml)
[![Release](https://img.shields.io/github/v/release/Malcolmston/go?sort=semver)](https://github.com/Malcolmston/go/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Malcolmston/go)](https://github.com/Malcolmston/go/commits)
[![Code Size](https://img.shields.io/github/languages/code-size/Malcolmston/go)](https://github.com/Malcolmston/go)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

**The Node.js — and Python, Rust, Elixir, Java — ecosystems, reimagined in Go.**

A unified home for **33** independent Go libraries that recreate the most-loved
building blocks of other ecosystems — the same ergonomics, on top of Go's
standard library. Every port is **dependency-free** (stdlib only: no cgo, no
third-party `require`s), individually versioned, and **verified against the
original library's own test suite**.

🌐 **Unified site & docs:** a Next.js app (deployed on Vercel) — a Home grid and
a page per library with its own routes for Overview, Examples, an inline API
reference and Node-vs-Go comparisons, plus How-to, FAQ, AI, and About. Symbol
search is Elasticsearch-backed (with an in-memory BM25 fallback).

## The libraries

Each library is an independent Go module (`github.com/malcolmston/<name>`). This
repo vendors them as **git submodules** and ties them together with a **Go
workspace** (`go.work`) so cross-module code builds against the local checkouts.
The **parity** column is the port's measured fidelity to the original, generated
live from each repo's `parity.json` (see [Upstream parity](#upstream-parity--pipeline)).

### Web & real-time

| Library | Ports | Parity | Docs |
| ------- | ----- | :----: | ---- |
| [express](express) | expressjs/express (+90 npm util ports) | 100% | [pages](https://malcolmston.github.io/express/) |
| [passport](passport) | jaredhanson/passport (+ strategies) | 100% | [pages](https://malcolmston.github.io/passport/) |
| [socket.io](socket.io) | socketio/socket.io | 100% | [pages](https://malcolmston.github.io/socket.io/) |
| [morgan](morgan) | expressjs/morgan | 100% | [pages](https://malcolmston.github.io/morgan/) |
| [liveview](liveview) | phoenixframework/phoenix_live_view | 100% | [pages](https://malcolmston.github.io/liveview/) |
| [cheerio](cheerio) | cheeriojs/cheerio | 100% | [pages](https://malcolmston.github.io/cheerio/) |
| [handlebars](handlebars) | handlebars-lang/handlebars.js | 97% | [pages](https://malcolmston.github.io/handlebars/) |
| [axios](axios) | axios/axios | 100% | [pages](https://malcolmston.github.io/axios/) |
| [puppeteer](puppeteer) | puppeteer/puppeteer | 94% | [pages](https://malcolmston.github.io/puppeteer/) |

### CLI, docs & formats

| Library | Ports | Parity | Docs |
| ------- | ----- | :----: | ---- |
| [chalk](chalk) | chalk/chalk (+ figlet, prompts) | 100% | [pages](https://malcolmston.github.io/chalk/) |
| [jest](jest) | jestjs/jest | 100% | [pages](https://malcolmston.github.io/jest/) |
| [jwt](jwt) | auth0/node-jsonwebtoken | 100% | [pages](https://malcolmston.github.io/jwt/) |
| [nodemailer](nodemailer) | nodemailer/nodemailer | 100% | [pages](https://malcolmston.github.io/nodemailer/) |
| [pdfkit](pdfkit) | foliojs/pdfkit | 100% | [pages](https://malcolmston.github.io/pdfkit/) |
| [gltf](gltf) | KhronosGroup/glTF | 100% | [pages](https://malcolmston.github.io/gltf/) |
| [sharp](sharp) | lovell/sharp | 100% | [pages](https://malcolmston.github.io/sharp/) |

### Data, math & ML (Python-inspired)

| Library | Ports | Parity | Docs |
| ------- | ----- | :----: | ---- |
| [algebra](algebra) | sympy/sympy (91 subpackages) | 100% | [pages](https://malcolmston.github.io/algebra/) |
| [opencv](opencv) | opencv/opencv | — | [pages](https://malcolmston.github.io/opencv/) |
| [numpy](numpy) | numpy/numpy | 100% | [pages](https://malcolmston.github.io/numpy/) |
| [pandas](pandas) | pandas-dev/pandas | 100% | [pages](https://malcolmston.github.io/pandas/) |
| [matplotlib](matplotlib) | matplotlib/matplotlib | 100% | [pages](https://malcolmston.github.io/matplotlib/) |
| [streamlit](streamlit) | streamlit/streamlit | 93% | [pages](https://malcolmston.github.io/streamlit/) |

### Stores, infra & tooling

| Library | Ports | Parity | Docs |
| ------- | ----- | :----: | ---- |
| [sqlite](sqlite) | sqlite/sqlite | 100% | [pages](https://malcolmston.github.io/sqlite/) |
| [redis](redis) | redis/redis | 100% | [pages](https://malcolmston.github.io/redis/) |
| [sled](sled) | spacejam/sled (Rust) | 100% | [pages](https://malcolmston.github.io/sled/) |
| [prisma](prisma) | prisma/prisma | 100% | [pages](https://malcolmston.github.io/prisma/) |
| [migrate](migrate) | rails/rails (ActiveRecord) | 100% | [pages](https://malcolmston.github.io/migrate/) |
| [quartz](quartz) | quartz-scheduler/quartz (Java) | 100% | [pages](https://malcolmston.github.io/quartz/) |
| [oban](oban) | sorentwo/oban (Elixir) | 100% | [pages](https://malcolmston.github.io/oban/) |
| [lucene](lucene) | apache/lucene (Java) | 100% | [pages](https://malcolmston.github.io/lucene/) |
| [fastmcp](fastmcp) | jlowin/fastmcp (Python) | 99% | [pages](https://malcolmston.github.io/fastmcp/) |

### General utilities

| Library | Ports | Parity | Docs |
| ------- | ----- | :----: | ---- |
| [lodash](lodash) | lodash/lodash | 98% | [pages](https://malcolmston.github.io/lodash/) |
| [moment](moment) | moment/moment | 98% | [pages](https://malcolmston.github.io/moment/) |

> Module paths follow `github.com/malcolmston/<name>` (Socket.IO is
> `github.com/malcolmston/socketio`).

## Upstream parity & pipeline

Every port is measured against the **original** library, not by eyeball. Each
repo's parity CI:

1. **syncs the upstream project's own test suite** — the real vectors from
   `expressjs/express`, `lodash/lodash`, the RFC appendices, the official
   JSON-Schema / URI-Template conformance suites, etc. — into Go
   `TestParity*` tests;
2. **closes the behavior gaps** those vectors expose; and
3. **publishes `parity.json`**, the machine-readable score.

Every repo's pipeline routes through one **central reusable workflow**
([`.github/workflows/parity-reusable.yml`](.github/workflows/parity-reusable.yml)),
and the landing regenerates the scores live on each deploy. Open any library tab
on the site to see its score broken down (cases synced, gaps closed) and a
node-graph of the **actual pipeline stages** with live status.

Multi-package libraries are additionally audited **per subpackage**: express's
~60 npm-utility ports, passport's strategy/RFC subpackages, and chalk / fastmcp /
socket.io subpackages are each verified against their own upstream suites.

## Full API reference

The landing renders every library's **complete Go API reference inline** —
package-by-package types, functions, methods, constants and runnable examples —
generated from source by a stdlib-only `go/doc` tool (`gendocs`). Each library
also ships the same reference on its own docs site (the **Docs** tab), so the
family maintains **100% exported-symbol doc coverage**.

## Clone (with submodules)

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

## Use a single library

The libraries are independent — you do **not** need this repo to use them:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/algebra
go get github.com/malcolmston/socketio
# …any of the 33, all github.com/malcolmston/<name>
```

## Develop across libraries (workspace)

`go.work` lists every submodule plus the example, so code in this repo resolves
imports to the local checkouts — no publishing required:

```sh
go run ./examples/integration   # express + socket.io + morgan on one server
go build ./examples/integration
```

See [`examples/integration`](examples/integration) for a runnable server that
composes Express routing, a Socket.IO realtime endpoint, and morgan logging
through the standard `http.Handler` interface.

## Pipelines

CI is split into focused, independently-badged workflows so a failure points
straight at the suite that broke:

- **[Library Tests](.github/workflows/library-tests.yml)** — a matrix that
  checks out each submodule at its latest commit and runs
  `go build ./... && go test -race ./...` per library.
- **[Go Workspace](.github/workflows/go-workspace.yml)** — builds and vets the
  cross-module `examples/integration` server through the shared `go.work`.
- **[Cross-compile](.github/workflows/cross-compile.yml)** — builds the
  integration example across a GOOS/GOARCH matrix.
- **[Web Unit](.github/workflows/web-unit.yml)** — Vitest component tests plus a
  TypeScript type-check for the unified site.
- **[Web E2E](.github/workflows/web-e2e.yml)** — the Playwright device sweep
  against a production build of the site.
- **[Pages](.github/workflows/pages.yml)** — regenerates the per-library API
  docs + live parity metrics and publishes the unified site.
- **[Parity (reusable)](.github/workflows/parity-reusable.yml)** — the central
  `workflow_call` every library's own `parity.yml` routes through.

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original projects.
