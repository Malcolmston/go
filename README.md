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

The **Parity** column comes from [`parity/<lib>/parity.json`](parity/), written by
`go test ./parity/<lib>/` — which installs the real upstream library, feeds it and
the Go port the same cases, and compares the answers. **Cases** is how many were
compared; a percentage without a case count is not worth much. **Upstream** is the
exact pinned version the score is against, because a parity figure is meaningless
without saying what it is parity *with*.

> **These numbers replaced an earlier, much rosier set, and they are lower on
> purpose.** The old scores came from hand-written expectations — a record of what
> someone believed upstream did on the day they wrote them. express, passport,
> socket.io and morgan were all listed at **100%** over 20–54 cases. Measured
> against the real libraries over 74–184 cases they are **66.8%, 66.2%, 81.8% and
> 67.3%**. Nothing regressed; the measurement got honest. See
> [`parity/HARNESS.md`](parity/HARNESS.md) for why running upstream is the only
> score that cannot quietly go stale.

Read the column with three caveats:

- **A high score is not a small surface, and a low one is not a bad port.**
  `markdown` is at **100% CommonMark conformance** (652/652 spec examples,
  byte-exact) while `puppeteer` is at 71.6% of what it implements — but only
  **2.9%** of puppeteer's actual API, because it has no CDP, no browser and no JS
  engine. `pdfkit` scores 6.7% strict and 43.7% structural, almost entirely
  because it lacks upstream's flow-based text model. The score measures agreement
  where the two overlap, not how much of upstream exists.
- **Some ports beat their upstream.** `yaml`'s parser is 308/308 on accept cases
  and 94/94 on must-fail against the official test suite, where the reference JS
  implementation is 307/308 and 92/94. All of yaml's defects are in its *emitter*.
- **The number is a floor where upstream has moved on.** `fastmcp`, `liveview` and
  `oban` are compared against upstreams considerably newer than the ports target,
  so part of the gap is upstream drift rather than port defect. Each
  `COVERAGE.md` says which is which.

Alongside each score, `parity/<lib>/COVERAGE.md` enumerates **every** upstream
symbol — derived mechanically by reflection, `javap`, `jq -n builtins`,
`COMMAND LIST` or `dir()`, with the command recorded — and marks it `match`,
`differs`, `missing`, `extra` or `untested`. A symbol with no case is `untested`,
never `match`.

### Measured parity, all 38 libraries

Sorted by score. Regenerate any row with `go test ./parity/<lib>/`; the harness
rewrites `parity/<lib>/parity.json`, which is what this table is built from.
Where a library declares deliberate deviations, they are excluded from the
denominator and counted separately, so **Cases** can exceed match + mismatch.

| Library | Parity | Cases | Measured against |
| ------- | -----: | ----: | ---------------- |
| [sled](sled) | 98.5% | 70 | `sled@0.34.7` |
| [markdown](markdown) | 97.4% | 701 | `commonmark@0.31.2` |
| [lodash](lodash) | 97.3% | 624 | `lodash@4.17.21` |
| [gltf](gltf) | 97.0% | 101 | `@gltf-transform/core@4.2.1+gl…` |
| [numpy](numpy) | 96.3% | 301 | `numpy@2.2.6` |
| [moment](moment) | 93.9% | 1731 | `moment@2.30.1` |
| [yaml](yaml) | 93.0% | 1071 | `yaml@2.6.1+yaml-test-suite@da…` |
| [jwt](jwt) | 92.8% | 155 | `jsonwebtoken@9.0.2` |
| [jose](jose) | 91.7% | 157 | `jose@5.9.6` |
| [jest](jest) | 90.9% | 369 | `` |
| [algebra](algebra) | 90.8% | 292 | `sympy@1.14.0` |
| [jq](jq) | 89.7% | 574 | `jq@1.7.1` |
| [handlebars](handlebars) | 87.7% | 236 | `handlebars@4.7.8` |
| [cheerio](cheerio) | 87.1% | 317 | `cheerio@1.0.0` |
| [rrule](rrule) | 85.8% | 261 | `python-dateutil@2.9.0.post0` |
| [chalk](chalk) | 85.8% | 680 | `chalk@5.3.0` |
| [oban](oban) | 82.4% | 165 | `sorentwo/oban` |
| [socket.io](socket.io) | 81.8% | 165 | `engine.io-parser@5.2.3; socke…` |
| [pandas](pandas) | 80.6% | 217 | `pandas==2.3.3` |
| [opencv](opencv) | 77.6% | 751 | `opencv-python@4.11.0` |
| [lucene](lucene) | 75.5% | 237 | `lucene@9.11.1` |
| [liveview](liveview) | 73.4% | 94 | `phoenixframework/phoenix_live…` |
| [prisma](prisma) | 72.3% | 112 | `prisma@5.22.0` |
| [puppeteer](puppeteer) | 71.6% | 197 | `cheerio@1.0.0 + tough-cookie@…` |
| [quartz](quartz) | 70.2% | 225 | `quartz-scheduler/quartz` |
| [sqlite](sqlite) | 69.2% | 428 | `sqlite3@3.45.3` |
| [streamlit](streamlit) | 69.1% | 55 | `streamlit==1.61.1` |
| [morgan](morgan) | 67.3% | 98 | `morgan@1.10.0` |
| [axios](axios) | 66.9% | 151 | `axios@1.7.9` |
| [express](express) | 66.8% | 184 | `express@4.21.2` |
| [redis](redis) | 66.7% | 93 | `redis-server@8.2.2` |
| [passport](passport) | 66.2% | 74 | `passport@0.7.0+passport-local…` |
| [sharp](sharp) | 65.1% | 327 | `sharp@0.33.5` |
| [matplotlib](matplotlib) | 59.2% | 103 | `matplotlib@3.10.0` |
| [migrate](migrate) | 51.2% | 86 | `rails/rails activerecord@8.0.…` |
| [fastmcp](fastmcp) | 50.0% | 110 | `fastmcp@3.4.6` |
| [pdfkit](pdfkit) | 6.7% | 119 | `pdfkit@0.15.1` |
| [nodemailer](nodemailer) | 1.2% | 86 | `nodemailer@6.9.16` |

`nodemailer` and `pdfkit` are the two scores that most need their footnote: both
are dominated by a single systematic difference — nodemailer always uses
quoted-printable and always emits an empty `Subject`, pdfkit emits no AFM pair
kerning — so their *structural* parity, with those masked, is 64.7% and 43.7%.
The strict number is the honest headline; the structural one is what is left to
fix after the systematic difference is addressed.

### Web & real-time

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [express](express) | expressjs/express (+ npm util ports) | 66.8% | 184 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/express) |
| [passport](passport) | jaredhanson/passport (+ strategies) | 66.2% | 74 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/passport) |
| [socket.io](socket.io) | socketio/socket.io | 81.8% | 165 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/socket.io) |
| [morgan](morgan) | expressjs/morgan | 67.3% | 98 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/morgan) |
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
| [chalk](chalk) | chalk/chalk (+ figlet, prompts) | 85.8% | 680 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/chalk) |
| [jose](jose) | panva/jose (RFC 7515/16/17/18) | 91.7% | 157 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jose) |
| [markdown](markdown) | commonmark/commonmark-spec | 97.4% | 701 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/markdown) |
| [yaml](yaml) | yaml/yaml-test-suite | 93.0% | 1071 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/yaml) |

### Data, math & ML (Python-inspired)

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [algebra](algebra) | sympy/sympy | 90.8% | 292 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/algebra) |
| [opencv](opencv) | opencv/opencv | 77.6% | 751 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/opencv) |
| [streamlit](streamlit) | streamlit/streamlit | 69.1% | 55 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/streamlit) |

### Tooling & data wrangling

| Library | Ports | Parity | Cases | Docs |
| ------- | ----- | :----: | ----: | ---- |
| [fastmcp](fastmcp) | jlowin/fastmcp (Python) | 50.0% | 110 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/fastmcp) |
| [jq](jq) | jqlang/jq (C) | 89.7% | 574 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/jq) |
| [rrule](rrule) | dateutil/dateutil (Python) | 85.8% | 261 | [pages](https://go-malcolms-projects-18e573c3.vercel.app/lib/rrule) |

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
