<div align="center">

# malcolmston/go

**The Node.js — and Python, Java, Ruby, Rust, Elixir and C — ecosystems, rebuilt in Go.**

42 Go libraries that recreate the most-loved building blocks of other
ecosystems, each one **scored against the real original** rather than against
someone's memory of it.

[![Library Tests](https://github.com/malcolmston/go/actions/workflows/library-tests.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/library-tests.yml)
[![Go Workspace](https://github.com/malcolmston/go/actions/workflows/go-workspace.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/go-workspace.yml)
[![Cross-compile](https://github.com/malcolmston/go/actions/workflows/cross-compile.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/cross-compile.yml)
[![Web Unit](https://github.com/malcolmston/go/actions/workflows/web-unit.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/web-unit.yml)
[![Web E2E](https://github.com/malcolmston/go/actions/workflows/web-e2e.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/web-e2e.yml)
[![Pages](https://github.com/malcolmston/go/actions/workflows/pages.yml/badge.svg)](https://github.com/malcolmston/go/actions/workflows/pages.yml)

[![Release](https://img.shields.io/github/v/tag/malcolmston/go?sort=semver&label=release)](https://github.com/malcolmston/go/releases)
[![Libraries](https://img.shields.io/badge/libraries-42-blue)](#the-libraries)
[![Parity harnesses](https://img.shields.io/badge/parity%20harnesses-76-blue)](#measured-parity)
[![License: MIT](https://img.shields.io/github/license/malcolmston/go)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Docs](https://img.shields.io/badge/docs-vercel-2f9bff)](https://go-malcolms-projects-18e573c3.vercel.app)

**[Website & full API docs →](https://go-malcolms-projects-18e573c3.vercel.app)**

</div>

---

## What this is

Each library here is an independent, from-scratch Go module that reproduces the
API of a well-known library from another language: `express` for
`expressjs/express`, `algebra` for `sympy`, `jq` for `jqlang/jq`, and 39 more.
They are ordinary Go modules — `go get` one and use it; you do not need this
repo.

What makes the collection unusual is the second half of it. Every port is
paired with a **parity harness** that installs the *real* upstream library,
starts it in its own runtime, feeds it and the Go port the same cases, and
compares both answers — and both failures, because returning a value where
upstream throws is a gap too. The score in every table below is that
comparison's output, read straight out of a committed `parity.json`. Nothing
here is hand-asserted.

| | |
| --- | --- |
| **Libraries** | 42 Go modules — 39 git submodules plus 3 that live here as directories |
| **Top-level parity harnesses** | 40, each pinned to a named upstream release |
| **Nested parity harnesses** | 36, measuring subpackages that port a *different* upstream |
| **Cases compared, last run** | 17,029 of 17,399 checked-in cases, across all 76 harnesses |
| **Dependencies** | standard library only, with three recorded exceptions |

---

## Quick start

Use one library. This is the normal case, and this repo is not involved:

```sh
go get github.com/malcolmston/express
go get github.com/malcolmston/algebra
go get github.com/malcolmston/socketio   # note: no dot — see the naming note below
```

```go
import (
	"github.com/malcolmston/express"
	"github.com/malcolmston/express/middleware/cors"
)

app := express.New()
app.Use(cors.New(cors.Options{AllowedOrigins: []string{"https://app.example.com"}}))
app.Get("/hello", func(req *express.Request, res *express.Response, next express.Next) {
	res.JSON(map[string]string{"hello": "world"})
})
http.ListenAndServe(":3000", app)
```

Clone the whole collection, submodules and all:

```sh
git clone --recurse-submodules https://github.com/malcolmston/go
# already cloned?
git submodule update --init --recursive
```

> **Naming.** Every module path is `github.com/malcolmston/<name>`, matching the
> directory — with one exception: the `socket.io/` directory is
> **`github.com/malcolmston/socketio`**, because a module path cannot carry the
> dot. `go.mod` is always the authority.

---

## The libraries

Grouped by the ecosystem they port from. Each row links to the library, names
its upstream, and carries its released tag, its pkg.go.dev reference, its CI
status and its license.

### Node.js & npm (25)

| Library | Ports | |
| --- | --- | --- |
| [`express`](express) | **expressjs/express** — the framework, plus ~95 npm utility ports | [![release](https://img.shields.io/github/v/tag/malcolmston/express?sort=semver&label=)](https://github.com/malcolmston/express/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/express.svg)](https://pkg.go.dev/github.com/malcolmston/express) [![ci](https://github.com/malcolmston/express/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/express/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/express)](express/LICENSE) |
| [`socket.io`](socket.io) | **socketio/socket.io** — server, client and the engine.io transport | [![release](https://img.shields.io/github/v/tag/malcolmston/socketio?sort=semver&label=)](https://github.com/malcolmston/socketio/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/socketio.svg)](https://pkg.go.dev/github.com/malcolmston/socketio) [![ci](https://github.com/malcolmston/socketio/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/socketio/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/socketio)](socket.io/LICENSE) |
| [`passport`](passport) | **jaredhanson/passport** — plus 154 strategies | [![release](https://img.shields.io/github/v/tag/malcolmston/passport?sort=semver&label=)](https://github.com/malcolmston/passport/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/passport.svg)](https://pkg.go.dev/github.com/malcolmston/passport) [![ci](https://github.com/malcolmston/passport/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/passport/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/passport)](passport/LICENSE) |
| [`morgan`](morgan) | **expressjs/morgan** — HTTP request logging middleware | [![release](https://img.shields.io/github/v/tag/malcolmston/morgan?sort=semver&label=)](https://github.com/malcolmston/morgan/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/morgan.svg)](https://pkg.go.dev/github.com/malcolmston/morgan) [![ci](https://github.com/malcolmston/morgan/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/morgan/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/morgan)](morgan/LICENSE) |
| [`axios`](axios) | **axios/axios** — promise-shaped HTTP client | [![release](https://img.shields.io/github/v/tag/malcolmston/axios?sort=semver&label=)](https://github.com/malcolmston/axios/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/axios.svg)](https://pkg.go.dev/github.com/malcolmston/axios) [![ci](https://github.com/malcolmston/axios/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/axios/actions/workflows/parity.yml) |
| [`lodash`](lodash) | **lodash/lodash** — the utility belt | [![release](https://img.shields.io/github/v/tag/malcolmston/lodash?sort=semver&label=)](https://github.com/malcolmston/lodash/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/lodash.svg)](https://pkg.go.dev/github.com/malcolmston/lodash) [![ci](https://github.com/malcolmston/lodash/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/lodash/actions/workflows/parity.yml) |
| [`moment`](moment) | **moment/moment** — date parsing, formatting and arithmetic | [![release](https://img.shields.io/github/v/tag/malcolmston/moment?sort=semver&label=)](https://github.com/malcolmston/moment/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/moment.svg)](https://pkg.go.dev/github.com/malcolmston/moment) [![ci](https://github.com/malcolmston/moment/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/moment/actions/workflows/parity.yml) |
| [`cheerio`](cheerio) | **cheeriojs/cheerio** — jQuery-style server-side HTML | [![release](https://img.shields.io/github/v/tag/malcolmston/cheerio?sort=semver&label=)](https://github.com/malcolmston/cheerio/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/cheerio.svg)](https://pkg.go.dev/github.com/malcolmston/cheerio) [![ci](https://github.com/malcolmston/cheerio/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/cheerio/actions/workflows/parity.yml) |
| [`handlebars`](handlebars) | **handlebars-lang/handlebars** — logic-less templates | [![release](https://img.shields.io/github/v/tag/malcolmston/handlebars?sort=semver&label=)](https://github.com/malcolmston/handlebars/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/handlebars.svg)](https://pkg.go.dev/github.com/malcolmston/handlebars) [![ci](https://github.com/malcolmston/handlebars/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/handlebars/actions/workflows/parity.yml) |
| [`chalk`](chalk) | **chalk/chalk** — terminal styling, plus figlet and prompts | [![release](https://img.shields.io/github/v/tag/malcolmston/chalk?sort=semver&label=)](https://github.com/malcolmston/chalk/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/chalk.svg)](https://pkg.go.dev/github.com/malcolmston/chalk) [![ci](https://github.com/malcolmston/chalk/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/chalk/actions/workflows/go-test.yml) |
| [`jest`](jest) | **jestjs/jest** — the `expect` matcher and mock surface | [![release](https://img.shields.io/github/v/tag/malcolmston/jest?sort=semver&label=)](https://github.com/malcolmston/jest/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/jest.svg)](https://pkg.go.dev/github.com/malcolmston/jest) [![ci](https://github.com/malcolmston/jest/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/jest/actions/workflows/parity.yml) |
| [`jwt`](jwt) | **auth0/node-jsonwebtoken** — JWT sign and verify | [![release](https://img.shields.io/github/v/tag/malcolmston/jwt?sort=semver&label=)](https://github.com/malcolmston/jwt/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/jwt.svg)](https://pkg.go.dev/github.com/malcolmston/jwt) [![ci](https://github.com/malcolmston/jwt/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/jwt/actions/workflows/parity.yml) |
| [`jose`](jose) | **panva/jose** — JWS/JWE/JWK/JWT, RFC 7515-7518 | [![release](https://img.shields.io/github/v/tag/malcolmston/jose?sort=semver&label=)](https://github.com/malcolmston/jose/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/jose.svg)](https://pkg.go.dev/github.com/malcolmston/jose) [![ci](https://github.com/malcolmston/jose/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/jose/actions/workflows/parity.yml) [![license](https://img.shields.io/github/license/malcolmston/jose)](jose/LICENSE) |
| [`yaml`](yaml) | **eemeli/yaml** — YAML 1.2 parser and emitter | [![release](https://img.shields.io/github/v/tag/malcolmston/yaml?sort=semver&label=)](https://github.com/malcolmston/yaml/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/yaml.svg)](https://pkg.go.dev/github.com/malcolmston/yaml) [![ci](https://github.com/malcolmston/yaml/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/yaml/actions/workflows/parity.yml) [![license](https://img.shields.io/github/license/malcolmston/yaml)](yaml/LICENSE) |
| [`prisma`](prisma) | **prisma/prisma** — schema language and query builder | [![release](https://img.shields.io/github/v/tag/malcolmston/prisma?sort=semver&label=)](https://github.com/malcolmston/prisma/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/prisma.svg)](https://pkg.go.dev/github.com/malcolmston/prisma) [![ci](https://github.com/malcolmston/prisma/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/prisma/actions/workflows/parity.yml) |
| [`sequelize`](sequelize) | **sequelize/sequelize** — ORM over `database/sql` | [![release](https://img.shields.io/github/v/tag/malcolmston/sequelize?sort=semver&label=)](https://github.com/malcolmston/sequelize/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/sequelize.svg)](https://pkg.go.dev/github.com/malcolmston/sequelize) [![license](https://img.shields.io/github/license/malcolmston/sequelize)](sequelize/LICENSE) |
| [`sqlite`](sqlite) | **TryGhost/node-sqlite3** — the node-sqlite3 API surface | [![release](https://img.shields.io/github/v/tag/malcolmston/sqlite?sort=semver&label=)](https://github.com/malcolmston/sqlite/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/sqlite.svg)](https://pkg.go.dev/github.com/malcolmston/sqlite) [![ci](https://github.com/malcolmston/sqlite/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/sqlite/actions/workflows/parity.yml) |
| [`nodemailer`](nodemailer) | **nodemailer/nodemailer** — MIME composition and SMTP | [![release](https://img.shields.io/github/v/tag/malcolmston/nodemailer?sort=semver&label=)](https://github.com/malcolmston/nodemailer/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/nodemailer.svg)](https://pkg.go.dev/github.com/malcolmston/nodemailer) [![ci](https://github.com/malcolmston/nodemailer/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/nodemailer/actions/workflows/parity.yml) |
| [`pdfkit`](pdfkit) | **foliojs/pdfkit** — PDF generation | [![release](https://img.shields.io/github/v/tag/malcolmston/pdfkit?sort=semver&label=)](https://github.com/malcolmston/pdfkit/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/pdfkit.svg)](https://pkg.go.dev/github.com/malcolmston/pdfkit) [![ci](https://github.com/malcolmston/pdfkit/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/pdfkit/actions/workflows/parity.yml) |
| [`sharp`](sharp) | **lovell/sharp** — image resize and convert | [![release](https://img.shields.io/github/v/tag/malcolmston/sharp?sort=semver&label=)](https://github.com/malcolmston/sharp/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/sharp.svg)](https://pkg.go.dev/github.com/malcolmston/sharp) [![ci](https://github.com/malcolmston/sharp/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/sharp/actions/workflows/parity.yml) |
| [`puppeteer`](puppeteer) | **puppeteer/puppeteer** — the DOM-and-cookies half, no CDP | [![release](https://img.shields.io/github/v/tag/malcolmston/puppeteer?sort=semver&label=)](https://github.com/malcolmston/puppeteer/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/puppeteer.svg)](https://pkg.go.dev/github.com/malcolmston/puppeteer) [![ci](https://github.com/malcolmston/puppeteer/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/puppeteer/actions/workflows/parity.yml) |
| [`gltf`](gltf) | **donmccurdy/glTF-Transform** — glTF read, write and transform | [![release](https://img.shields.io/github/v/tag/malcolmston/gltf?sort=semver&label=)](https://github.com/malcolmston/gltf/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/gltf.svg)](https://pkg.go.dev/github.com/malcolmston/gltf) [![ci](https://github.com/malcolmston/gltf/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/gltf/actions/workflows/parity.yml) |
| [`react`](react) | **facebook/react** — React 19, plus an RSC Flight encoder | ![in-repo](https://img.shields.io/badge/v0.1.0-in--repo-lightgrey) |
| [`d3`](d3) | **d3/d3** — the computational modules only | ![in-repo](https://img.shields.io/badge/v0.1.0-in--repo-lightgrey) |
| [`reactcli`](reactcli) | *(first-party tool)* — the `react` toolchain: scaffold, dev server, doctor | ![in-repo](https://img.shields.io/badge/v0.1.0-in--repo-lightgrey) |

### Python (8)

| Library | Ports | |
| --- | --- | --- |
| [`algebra`](algebra) | **sympy/sympy** — computer algebra, across ~100 subpackages | [![release](https://img.shields.io/github/v/tag/malcolmston/algebra?sort=semver&label=)](https://github.com/malcolmston/algebra/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/algebra.svg)](https://pkg.go.dev/github.com/malcolmston/algebra) [![ci](https://github.com/malcolmston/algebra/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/algebra/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/algebra)](algebra/LICENSE) |
| [`numpy`](numpy) | **numpy/numpy** — n-dimensional arrays and ufuncs | [![release](https://img.shields.io/github/v/tag/malcolmston/numpy?sort=semver&label=)](https://github.com/malcolmston/numpy/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/numpy.svg)](https://pkg.go.dev/github.com/malcolmston/numpy) [![ci](https://github.com/malcolmston/numpy/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/numpy/actions/workflows/parity.yml) |
| [`pandas`](pandas) | **pandas-dev/pandas** — Series and DataFrame | [![release](https://img.shields.io/github/v/tag/malcolmston/pandas?sort=semver&label=)](https://github.com/malcolmston/pandas/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/pandas.svg)](https://pkg.go.dev/github.com/malcolmston/pandas) [![ci](https://github.com/malcolmston/pandas/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/pandas/actions/workflows/parity.yml) |
| [`matplotlib`](matplotlib) | **matplotlib/matplotlib** — figure and axes plotting | [![release](https://img.shields.io/github/v/tag/malcolmston/matplotlib?sort=semver&label=)](https://github.com/malcolmston/matplotlib/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/matplotlib.svg)](https://pkg.go.dev/github.com/malcolmston/matplotlib) [![ci](https://github.com/malcolmston/matplotlib/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/matplotlib/actions/workflows/parity.yml) |
| [`opencv`](opencv) | **opencv/opencv-python** — computer vision, across ~90 subpackages | [![release](https://img.shields.io/github/v/tag/malcolmston/opencv?sort=semver&label=)](https://github.com/malcolmston/opencv/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/opencv.svg)](https://pkg.go.dev/github.com/malcolmston/opencv) [![ci](https://github.com/malcolmston/opencv/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/opencv/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/opencv)](opencv/LICENSE) |
| [`streamlit`](streamlit) | **streamlit/streamlit** — script-shaped web apps | [![release](https://img.shields.io/github/v/tag/malcolmston/streamlit?sort=semver&label=)](https://github.com/malcolmston/streamlit/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/streamlit.svg)](https://pkg.go.dev/github.com/malcolmston/streamlit) [![ci](https://github.com/malcolmston/streamlit/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/streamlit/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/streamlit)](streamlit/LICENSE) |
| [`fastmcp`](fastmcp) | **jlowin/fastmcp** — Model Context Protocol server and client | [![release](https://img.shields.io/github/v/tag/malcolmston/fastmcp?sort=semver&label=)](https://github.com/malcolmston/fastmcp/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/fastmcp.svg)](https://pkg.go.dev/github.com/malcolmston/fastmcp) [![ci](https://github.com/malcolmston/fastmcp/actions/workflows/go-test.yml/badge.svg)](https://github.com/malcolmston/fastmcp/actions/workflows/go-test.yml) [![license](https://img.shields.io/github/license/malcolmston/fastmcp)](fastmcp/LICENSE) |
| [`rrule`](rrule) | **dateutil/dateutil** — RFC 5545 recurrence rules | [![release](https://img.shields.io/github/v/tag/malcolmston/rrule?sort=semver&label=)](https://github.com/malcolmston/rrule/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/rrule.svg)](https://pkg.go.dev/github.com/malcolmston/rrule) [![ci](https://github.com/malcolmston/rrule/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/rrule/actions/workflows/parity.yml) [![license](https://img.shields.io/github/license/malcolmston/rrule)](rrule/LICENSE) |

### Java (2)

| Library | Ports | |
| --- | --- | --- |
| [`lucene`](lucene) | **apache/lucene** — the index, analysis and query surface | [![release](https://img.shields.io/github/v/tag/malcolmston/lucene?sort=semver&label=)](https://github.com/malcolmston/lucene/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/lucene.svg)](https://pkg.go.dev/github.com/malcolmston/lucene) [![ci](https://github.com/malcolmston/lucene/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/lucene/actions/workflows/parity.yml) |
| [`quartz`](quartz) | **quartz-scheduler/quartz** — job scheduling and cron triggers | [![release](https://img.shields.io/github/v/tag/malcolmston/quartz?sort=semver&label=)](https://github.com/malcolmston/quartz/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/quartz.svg)](https://pkg.go.dev/github.com/malcolmston/quartz) [![ci](https://github.com/malcolmston/quartz/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/quartz/actions/workflows/parity.yml) |

### Elixir (2)

| Library | Ports | |
| --- | --- | --- |
| [`liveview`](liveview) | **phoenixframework/phoenix_live_view** — the diff format | [![release](https://img.shields.io/github/v/tag/malcolmston/liveview?sort=semver&label=)](https://github.com/malcolmston/liveview/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/liveview.svg)](https://pkg.go.dev/github.com/malcolmston/liveview) [![ci](https://github.com/malcolmston/liveview/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/liveview/actions/workflows/parity.yml) |
| [`oban`](oban) | **sorentwo/oban** — the job and queue surface | [![release](https://img.shields.io/github/v/tag/malcolmston/oban?sort=semver&label=)](https://github.com/malcolmston/oban/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/oban.svg)](https://pkg.go.dev/github.com/malcolmston/oban) [![ci](https://github.com/malcolmston/oban/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/oban/actions/workflows/parity.yml) |

### Ruby, Rust & C (5)

| Library | Ports | |
| --- | --- | --- |
| [`migrate`](migrate) | **rails/rails (ActiveRecord)** — migrations and schema DSL | [![release](https://img.shields.io/github/v/tag/malcolmston/migrate?sort=semver&label=)](https://github.com/malcolmston/migrate/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/migrate.svg)](https://pkg.go.dev/github.com/malcolmston/migrate) [![ci](https://github.com/malcolmston/migrate/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/migrate/actions/workflows/parity.yml) |
| [`sled`](sled) | **spacejam/sled** — embedded ordered key/value store | [![release](https://img.shields.io/github/v/tag/malcolmston/sled?sort=semver&label=)](https://github.com/malcolmston/sled/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/sled.svg)](https://pkg.go.dev/github.com/malcolmston/sled) [![ci](https://github.com/malcolmston/sled/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/sled/actions/workflows/parity.yml) |
| [`redis`](redis) | **redis/redis** — the command surface, wire-compatible | [![release](https://img.shields.io/github/v/tag/malcolmston/redis?sort=semver&label=)](https://github.com/malcolmston/redis/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/redis.svg)](https://pkg.go.dev/github.com/malcolmston/redis) [![ci](https://github.com/malcolmston/redis/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/redis/actions/workflows/parity.yml) |
| [`jq`](jq) | **jqlang/jq** — the jq language and its builtins | [![release](https://img.shields.io/github/v/tag/malcolmston/jq?sort=semver&label=)](https://github.com/malcolmston/jq/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/jq.svg)](https://pkg.go.dev/github.com/malcolmston/jq) [![ci](https://github.com/malcolmston/jq/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/jq/actions/workflows/parity.yml) [![license](https://img.shields.io/github/license/malcolmston/jq)](jq/LICENSE) |
| [`markdown`](markdown) | **commonmark/commonmark-spec** — CommonMark 0.31.2 | [![release](https://img.shields.io/github/v/tag/malcolmston/markdown?sort=semver&label=)](https://github.com/malcolmston/markdown/releases) [![reference](https://pkg.go.dev/badge/github.com/malcolmston/markdown.svg)](https://pkg.go.dev/github.com/malcolmston/markdown) [![ci](https://github.com/malcolmston/markdown/actions/workflows/parity.yml/badge.svg)](https://github.com/malcolmston/markdown/actions/workflows/parity.yml) [![license](https://img.shields.io/github/license/malcolmston/markdown)](markdown/LICENSE) |

### The three that are not submodules

`react`, `reactcli` and `d3` are full Go modules with their own `go.mod`,
`VERSION` and test suites, but they live in **this** repository as plain
directories rather than as submodules with GitHub repos of their own. That has
one practical consequence, and it is why their rows carry a grey badge instead
of a release tag: **`go get github.com/malcolmston/react` does not resolve.**
There is no repository behind that path and no module-proxy entry, so they are
consumed through the committed [`go.work`](go.work), or through a `replace`:

```
require github.com/malcolmston/react v0.0.0
replace github.com/malcolmston/react => ../go/react
```

The other 39 are ordinary published modules and `go get` works normally.

**d3** ports the computational half of d3 — `array`, `scale`, `shape`, `path`,
`color`, `interpolate`, `format`, `timefmt`, `hierarchy`, `ease`, `random`,
`dsv`, `contour`, `delaunay`, `force`, `geo`, `polygon`, `quadtree` and the rest
(22 packages). Selection, transition, drag, zoom and brush are deliberately
absent: they are DOM machinery, and there is no DOM. What remains pairs with
**react**, which renders what d3 computes.

**react** also ships an RSC bridge, [`react/rsc`](react/rsc), which serializes a
Go tree into React's Flight wire format so a real React 19 client can render it
— Go server components travel as data, JS client components stay interactive.
The encoder is verified against React's own decoder, not only against itself.

**reactcli** is the `react` command: project scaffolding, a rebuild-on-save dev
server, component generation, ecosystem integrations wired in as pinned
standalone binaries, and `react doctor`, which finds the mistakes this port
makes easy to write and the compiler cannot catch. It is a separate module so
the library itself stays dependency-free.

### Dependencies

Every library is **standard-library only**, with exactly three recorded
exceptions, each visible in its own `go.mod`:

| Library | Requires | Why |
| --- | --- | --- |
| `chalk` | `golang.org/x/term`, `golang.org/x/sys` | the `prompts` subpackage needs raw-mode terminal input |
| `sequelize` | `modernc.org/sqlite` | **tests only** — the package itself ships no driver |
| `reactcli` | `github.com/spf13/cobra`, `github.com/fsnotify/fsnotify`, `chalk` | it is a CLI, not a library |

No cgo anywhere.

---

## Measured parity

Where a port is scored, it is scored by **running the original**. Each harness
under [`parity/`](parity):

1. **installs the upstream package it mirrors** — the real `chalk@5.3.0`,
   `sympy@1.14.0`, gem, crate or C binary — and starts it in its own runtime;
2. **asks it and the Go port the same questions**, comparing both answers and
   both failures;
3. **cross-compiles the port** for every target the family claims; and
4. **publishes `parity.json`**, the measured score with every failing case named.

Because the expectations come from upstream itself, they cannot drift: a new
upstream release re-scores the port on its own. Where the upstream runtime
cannot be installed, the suite replays upstream's *recorded* answers and the
report says so (`"mode": "golden"`) rather than passing a replay off as a
measurement. See [`parity/HARNESS.md`](parity/HARNESS.md).

Alongside each score, `parity/<lib>/COVERAGE.md` enumerates **every** upstream
symbol — derived mechanically by reflection, `javap`, `jq -n builtins`,
`COMMAND LIST` or `dir()`, with the command recorded — and marks it `match`,
`differs`, `missing`, `extra` or `untested`. A symbol with no case is
`untested`, never `match`.

### How to read these tables

- **Parity** is the figure the harness itself published in `parity.json`. It is
  reproduced here, never recomputed or rounded up.
- **Compared** is how many cases actually reached a comparison — the
  denominator. **Case files** is how many are checked in. Where the two differ,
  the gap is declared deviations and cases the harness could not run; the note
  under the table says which.
- **Declared deviations** are documented, deliberate differences. They are
  excluded from the denominator and named individually in the harness report, so
  they are visible rather than quietly counted as passes.
- **A high score is not a large surface, and a low one is not a bad port.**
  `markdown` is at **100% CommonMark conformance** (652/652 spec examples,
  byte-exact) while `puppeteer` scores 97.4% of *what it implements* — which is
  the DOM-and-cookies half only, with no CDP, no browser and no JS engine.
- **Some ports beat their upstream.** `yaml`'s parser is 308/308 on accept cases
  and 94/94 on must-fail against the official test suite, where the reference JS
  implementation is 307/308 and 92/94. All of yaml's defects are in its
  *emitter*.
- **The number is a floor where upstream has moved on.** `fastmcp`, `liveview`
  and `oban` are compared against upstreams considerably newer than the ports
  target, so part of the gap is upstream drift rather than port defect. Each
  `COVERAGE.md` says which is which.

### All 40 top-level harnesses

Regenerate any row with `go test ./parity/<lib>/`; the harness rewrites
`parity/<lib>/parity.json`, which is what this table is built from.

| Library | Measured against | Parity | Compared | Case files | Declared deviations |
| --- | --- | ---: | ---: | ---: | ---: |
| [axios](axios) | `axios@1.7.9` | **100%** | 149 | 151 | 2 |
| [passport](passport) | `passport@0.7.0+passport-local@1.0.0+p…` | **100%** | 72 | 74 | 2 |
| [sequelize](sequelize) | `sequelize@6.37.8` | **100%** | 58 | 61 | 3 |
| [react](react) | `react@19.2.7 react-dom@19.2.7` | **99.8%** | 475 | 483 | 8 |
| [cheerio](cheerio) | `cheerio@1.0.0` | **99.3%** | 308 | 317 | 9 |
| [express](express) | `express@4.21.2` | **98.9%** | 184 | 184 | 1 |
| [sled](sled) | `sled@0.34.7` | **98.5%** | 65 | 70 | 5 |
| [markdown](markdown) | `commonmark@0.31.2` | **97.4%** | 701 | 701 | — |
| [puppeteer](puppeteer) | `cheerio@1.0.0 + tough-cookie@5.1.2 + …` | **97.4%** | 190 | 197 | 7 |
| [lodash](lodash) | `lodash@4.17.21` | **97.3%** | 602 | 624 | 22 |
| [gltf](gltf) | `@gltf-transform/core@4.2.1+gltf-valid…` | **97%** | 101 | 101 | — |
| [socket.io](socket.io) | `engine.io-parser@5.2.3; socket.io-par…` | **97%** | 165 | 165 | 5 |
| [numpy](numpy) | `numpy@2.2.6` | **96.3%** | 297 | 301 | 4 |
| [jose](jose) | `jose@5.9.6` | **96.2%** | 157 | 157 | — |
| [jwt](jwt) | `jsonwebtoken@9.0.2` | **94.1%** | 152 | 155 | 3 |
| [moment](moment) | `moment@2.30.1` | **93.9%** | 1726 | 1731 | 5 |
| [yaml](yaml) | `yaml@2.6.1+yaml-test-suite@da267a5c47…` | **93%** | 1064 | 1071 | 7 |
| [jest](jest) | `expect@29.7.0; expect@29.7.0 + jest-m…` | **90.9%** | 342 | 369 | 27 |
| [algebra](algebra) | `sympy@1.14.0` | **90.8%** | 292 | 292 | — |
| [jq](jq) | `jq@1.7.1` | **89.7%** | 561 | 574 | 13 |
| [handlebars](handlebars) | `handlebars@4.7.8` | **87.7%** | 236 | 236 | — |
| [chalk](chalk) | `chalk@5.3.0` | **85.8%** | 674 | 680 | 6 |
| [rrule](rrule) | `python-dateutil@2.9.0.post0` | **85.8%** | 261 | 261 | — |
| [oban](oban) | `oban@2.23.1` | **82.4%** | 165 | 165 | — |
| [pandas](pandas) | `pandas==2.3.3` | **80.6%** | 216 | 217 | 1 |
| [opencv](opencv) | `opencv-python@4.11.0` | **77.6%** | 750 | 751 | 1 |
| [lucene](lucene) | `lucene@9.11.1` | **75.5%** | 237 | 237 | — |
| [liveview](liveview) | `phoenix_live_view@1.2.9` | **73.4%** | 94 | 94 | — |
| [streamlit](streamlit) | `streamlit==1.61.1` | **72.7%** | 55 | 55 | — |
| [prisma](prisma) | `prisma@5.22.0` | **72.3%** | 112 | 112 | — |
| [quartz](quartz) | `org.quartz-scheduler:quartz@2.5.0` | **70.2%** | 225 | 225 | — |
| [sqlite](sqlite) | `sqlite3@3.45.3` | **69.2%** | 428 | 428 | — |
| [morgan](morgan) | `morgan@1.10.0` | **67.3%** | 98 | 98 | — |
| [redis](redis) | `redis-server@8.2.2` | **66.7%** | 93 | 93 | — |
| [sharp](sharp) | `sharp@0.33.5` | **65.1%** | 327 | 327 | — |
| [matplotlib](matplotlib) | `matplotlib@3.10.0` | **59.2%** | 103 | 103 | — |
| [migrate](migrate) | `rails/rails activerecord@8.0.2 (sqlit…` | **51.2%** | 86 | 86 | — |
| [fastmcp](fastmcp) | `fastmcp@3.4.6` | **50%** | 110 | 110 | — |
| [pdfkit](pdfkit) | `pdfkit@0.15.1` | **6.7%** | 119 | 119 | — |
| [nodemailer](nodemailer) | `nodemailer@6.9.16` | **0%** | 8 | 86 | — |

Two rows need their footnote, and both are cases where a *single* systematic
difference dominates the strict score:

- **`nodemailer` — 0%, and the denominator is 8, not 86.** The port always uses
  quoted-printable and always emits an empty `Subject`, which fails every
  compared case on the canonical-MIME-tree comparison. With those two
  divergences masked, *structural* parity is **87.5%** (7 of 8). Note the
  counters: the last run compared only **8** of the 86 checked-in case files, so
  read this as thin coverage with a known systematic bug, not as 86 failures.
- **`pdfkit` — 6.7% strict, 43.7% structural.** It emits no AFM pair kerning and
  differs on six systematic details (header version, `/Info` dates, trailer
  `/ID`, the `/ProcSet` array, `cs`+`scn` vs `rg`/`g`/`k` colour spelling, and
  `q`/`Q`/`cm` bookkeeping). Masking those six leaves 52 of 119 matching. The
  strict number is the honest headline; the structural one is what is left to fix
  once the systematic difference is addressed.

Three more reading notes, so the columns are not over-read:

- **`markdown`** publishes two figures. The 97.4% above is parity against the
  `commonmark` reference implementation across all 701 cases; its **CommonMark
  0.31.2 spec conformance is 652/652 — 100%, byte-exact**, and so is upstream's.
- **`cheerio`, `axios`, `passport`, `sequelize`, `puppeteer`, `lodash`, `jwt`**
  and others show **Compared** below **Case files** because their declared
  deviations are subtracted from the denominator. The three at 100% are 100% *of
  what was compared* — 72 of 74, 149 of 151, 58 of 61 — not of everything
  checked in.
- **`jest`** is measured against `expect@29.7.0` and `jest-mock@29.7.0` rather
  than the whole test runner: the matcher and mock surface is the part that has a
  meaningful Go analogue.

### The 36 nested harnesses

Several libraries carry subpackages that are ports of a **different upstream
package** and are separately importable, so they get their own harness and their
own score. `express/qs` is measured against npm `qs`, not against express;
`socket.io/engineio` against `engine.io`; `chalk/figlet` against `figlet`. This
is a real and easily-missed part of the coverage story: it is 5,138 checked-in
cases, 4,971 of them compared, on top of the top-level numbers.

**Every one of the 36 is at 100% of its compared cases**, with 167 declared
deviations across them.

<details>
<summary>All 36 nested harnesses, with the upstream each is measured against</summary>

| Parent | Package | Ports | Parity | Compared | Devs |
| --- | --- | --- | ---: | ---: | ---: |
| [chalk](chalk) | [`figlet`](chalk/figlet) | `figlet@1.11.4` | 100% | 191 | 7 |
| [chalk](chalk) | [`prompts`](chalk/prompts) | `prompts@2.4.2` | 100% | 91 | 7 |
| [express](express) | [`accepts`](express/accepts) | `accepts@1.3.8` | 100% | 117 | — |
| [express](express) | [`bytes`](express/bytes) | `bytes@3.1.2` | 100% | 218 | 12 |
| [express](express) | [`camelcase`](express/camelcase) | `camelcase@9.0.0` | 100% | 260 | — |
| [express](express) | [`contenttype`](express/contenttype) | `content-type@1.0.5` | 100% | 76 | — |
| [express](express) | [`cookiesignature`](express/cookiesignature) | `cookie-signature@1.2.2` | 100% | 54 | — |
| [express](express) | [`deburr`](express/deburr) | `lodash@4.17.21` | 100% | 81 | — |
| [express](express) | [`escapehtml`](express/escapehtml) | `escape-html@1.0.3` | 100% | 36 | — |
| [express](express) | [`escaperegexp`](express/escaperegexp) | `escape-string-regexp@5.0.0` | 100% | 61 | — |
| [express](express) | [`filesize`](express/filesize) | `filesize@11.0.22` | 100% | 188 | — |
| [express](express) | [`htmlentities`](express/htmlentities) | `html-entities@2.6.0` | 100% | 110 | 3 |
| [express](express) | [`jsonwebtoken`](express/jsonwebtoken) | `jsonwebtoken@9.0.3` | 100% | 120 | 9 |
| [express](express) | [`jwtdecode`](express/jwtdecode) | `jwt-decode@4.0.0` | 100% | 45 | 4 |
| [express](express) | [`kebabcase`](express/kebabcase) | `change-case@5.4.4` | 100% | 102 | — |
| [express](express) | [`keygrip`](express/keygrip) | `keygrip@1.1.0` | 100% | 63 | 2 |
| [express](express) | [`mimetypes`](express/mimetypes) | `mime-types@2.1.35` | 100% | 188 | — |
| [express](express) | [`ms`](express/ms) | `ms@4.0.0-nightly.202508271359` | 100% | 178 | 2 |
| [express](express) | [`nanoid`](express/nanoid) | `nanoid@6.0.1` | 100% | 123 | 3 |
| [express](express) | [`negotiator`](express/negotiator) | `negotiator@0.6.3` | 100% | 142 | — |
| [express](express) | [`otpauth`](express/otpauth) | `otpauth@9.5.1` | 100% | 158 | 6 |
| [express](express) | [`pluralize`](express/pluralize) | `pluralize@8.0.0` | 100% | 450 | 14 |
| [express](express) | [`prettybytes`](express/prettybytes) | `pretty-bytes@7.1.1` | 100% | 187 | 3 |
| [express](express) | [`qs`](express/qs) | `qs@6.15.3` | 100% | 268 | 3 |
| [express](express) | [`sanitizehtml`](express/sanitizehtml) | `sanitize-html@2.17.6` | 100% | 223 | 10 |
| [express](express) | [`semver`](express/semver) | `semver@7.8.5` | 100% | 313 | 7 |
| [express](express) | [`slugify`](express/slugify) | `slugify@1.6.9` | 100% | 70 | 5 |
| [express](express) | [`statuses`](express/statuses) | `statuses@2.0.1` | 100% | 141 | 1 |
| [express](express) | [`striptags`](express/striptags) | `striptags@3.2.0` | 100% | 109 | 7 |
| [express](express) | [`typeis`](express/typeis) | `type-is@1.6.18` | 100% | 105 | — |
| [passport](passport) | [`httpauth`](passport/httpauth) | `http-auth-utils@7.0.1` | 100% | 114 | 31 |
| [passport](passport) | [`otpauth`](passport/otpauth) | `otpauth@9.5.1` | 100% | 99 | 8 |
| [passport](passport) | [`pkce`](passport/pkce) | `pkce-challenge@6.0.0` | 100% | 42 | 7 |
| [passport](passport) | [`pwhash`](passport/pwhash) | `pbkdf2@3.1.6` | 100% | 57 | 5 |
| [socket.io](socket.io) | [`client`](socket.io/client) | `socket.io-client@4.8.1; socket.…` | 100% | 95 | 6 |
| [socket.io](socket.io) | [`engineio`](socket.io/engineio) | `engine.io-parser@5.2.3; engine.…` | 100% | 96 | 5 |

</details>

`d3` and `reactcli` are the two libraries with **no** harness at all, and the
tables say so rather than inventing a figure: d3's output is geometry that would
need a rendering comparison, and `reactcli` is a CLI, not an API surface.

---

## Documentation

Three places, in increasing depth:

1. **[The website](https://go-malcolms-projects-18e573c3.vercel.app)** — a home
   grid, then a tab per library with an inline API reference and
   upstream-vs-Go comparisons, plus How-to, FAQ, AI and About pages.
2. **pkg.go.dev** — every published library's real godoc, linked from its badge
   in the tables above.
3. **The repository itself** — each library root has a `README.md`, every
   nested package has one, and each library that declares deliberate differences
   from upstream has an `API-DEVIATIONS.md` beside it. `parity/<lib>/COVERAGE.md`
   is the symbol-by-symbol audit.

The site renders each library's Go API reference **inline** — package-by-package
types, functions, methods, constants and runnable examples — generated from
source by a stdlib-only `go/doc` tool ([`gendocs`](gendocs)) into
`public/docs/<lib>.json`. Coverage there is currently **38 of the 42
libraries**; `sequelize`, `react`, `reactcli` and `d3` were added after the last
docs generation, and re-running the Pages workflow's `gendocs` step picks them
up.

---

## Examples

[`examples/`](examples) holds **38 standalone, runnable programs — one per
library** — and each is a module of its own that consumes the library **from the
proxy, with no `replace` directive**, exactly as an outside user would. That is
the point: they prove the *published* module works, not just the working tree.

```sh
cd examples/algebra
GOWORK=off go mod tidy
GOWORK=off go run .
```

Each example's README records the module version it resolved to, so a run that
silently picked up a pseudo-version instead of a real tag is visible.

---

## Developing across libraries

[`go.work`](go.work) is **committed** and lists every library plus the in-repo
tooling modules (`gendocs`, `genparity`, `parity`), so code here resolves imports
to the local checkouts — no publishing round-trip when you change two libraries
at once.

> A `use` directive pointing at a directory with no `go.mod` makes *every* `go`
> command in the repo fail, so an un-fetched submodule breaks the workspace
> rather than being skipped. Run `git submodule update --init --recursive`
> before `go` anything.

The repo root is intentionally **not** itself a Go module, so build with explicit
paths rather than a bare `go build ./...`:

```sh
go build ./express/... ./passport/...     # any subset of members
(cd algebra && go test ./...)             # a member's own suite
go test ./parity/express/                 # re-measure one library
go test ./parity/express/nested/qs/       # re-measure one nested package
go run  ./gendocs                         # regenerate public/docs/<lib>.json
```

---

## Pipelines

CI is split into focused, independently-badged workflows so a failure points
straight at the suite that broke:

| Workflow | What it does |
| --- | --- |
| [Library Tests](.github/workflows/library-tests.yml) | a matrix over the submodules, each checked out at its latest commit, asserted to contain a Go module, then `go build ./... && go test -race ./...` |
| [Go Workspace](.github/workflows/go-workspace.yml) | verifies every `go.work` member is checked out and builds |
| [Cross-compile](.github/workflows/cross-compile.yml) | builds across a GOOS/GOARCH matrix |
| [Web Unit](.github/workflows/web-unit.yml) | Vitest component tests plus a TypeScript type-check for the site |
| [Web E2E](.github/workflows/web-e2e.yml) | the Playwright device sweep against a production build |
| [Pages](.github/workflows/pages.yml) | regenerates the per-library API docs and parity metrics, then publishes the site |
| [Parity (reusable)](.github/workflows/parity-reusable.yml) | the central `workflow_call` every library's own `parity.yml` routes through |
| [Parity advisories](.github/workflows/parity-advisories.yml) | turns declared security findings in a parity report into draft advisories |
| [Sync submodules](.github/workflows/sync-submodules.yml) | keeps the pinned submodule commits moving with their default branches |

Every Go workflow pins its toolchain with `go-version-file: go.work`, so the `go`
directive in the workspace is the single place the Go version is declared.

Each library repo runs its own `go-test.yml` / `parity.yml` — that is the badge
in its row above.

---

## The website

The Next.js 15 (App Router, React 19) site that documents the family lives at the
repo root and deploys to Vercel.

```sh
pnpm install        # requires FONTAWESOME_PACKAGE_TOKEN — see below
pnpm dev
pnpm typecheck && pnpm test
pnpm build
```

**`FONTAWESOME_PACKAGE_TOKEN` is required to install.** [`.npmrc`](.npmrc)
points the `@awesome.me` and `@fortawesome` scopes at the Font Awesome Pro
registry, and `package.json` depends on `@awesome.me/kit-61a008692d`. Without the
token in your environment, pnpm prints

```
Failed to replace env in config: ${FONTAWESOME_PACKAGE_TOKEN}
```

on every command and `pnpm install` will 401 on a cold store — an existing
`node_modules`/store cache keeps working, which is why the warning often looks
harmless. Export it before installing:

```sh
export FONTAWESOME_PACKAGE_TOKEN=…      # Font Awesome account → Kits → package token
```

In CI it comes from the `FONTAWESOME_PACKAGE_TOKEN` Actions secret; on Vercel it
is a project environment variable. See [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) has the workflow. The two rules that matter
most here:

- **Do not hand-write a parity expectation.** If you cannot get the case from
  upstream, the case does not go in. A recorded upstream answer is fine and is
  labelled `"mode": "golden"`; a guess is not.
- **A difference from upstream is either a bug or an entry in
  `API-DEVIATIONS.md`.** There is no third option.

## Security

Vulnerability reporting and the automated-scanning inventory are in
[SECURITY.md](SECURITY.md).

## License

MIT. Each library is an independent re-implementation and is **not** affiliated
with or endorsed by the original projects. Upstream names are used only to say
what is being ported and what the score is measured against.
