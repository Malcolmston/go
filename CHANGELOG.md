# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-07-20
### Changed
- **The Next.js landing now lives at the repository root** (moved up from `web/`),
  so Vercel zero-config detects and builds it from the normal root — no
  `vercel.json` and no dashboard Root Directory override. `vercel.json` removed.
- **Migrated the aggregator from Vite to Next.js 15 (App Router, React 19).** Each
  tab is a real route (`/`, `/parity`, `/pipeline`, `/explore`, `/releases`,
  `/howto`, `/faq`, `/ai`, `/about`, and SSG `/lib/[id]` per library); the
  search + package-graph API runs as Next Route Handlers.
- **Package manager is pnpm end to end** (app + CI); `next build` now
  type-checks and lints (ESLint via `eslint-config-next`) as part of the
  production/Vercel build.
### Added
- **Elasticsearch-backed search + a GraphQL package-connection graph** (Vercel
  serverless functions) powering the Explore tab, with a bundled graph/search
  fallback for the static GitHub Pages export.
- Per-library **Overview / Examples / API / Parity** sub-tabs and a dedicated
  Parity + Pipeline presentation.
### Fixed
- Playwright E2E made hermetic and reliable after the migration: real
  path-routing, blocked-external-host aborts (no render-blocking font/API stalls),
  atomic link sweeps, client-side tab navigation, and a right-sized CI worker
  count. PR runs use a representative device subset; the full 200+ device matrix
  runs on `main` and nightly.

## [0.3.0] - 2026-07-19
### Added
- **Inline API reference** on every library tab — the complete Go API docs
  (package-by-package types, functions, methods, examples) rendered from source by
  a new stdlib-only `gendocs` tool.
- **Live upstream-parity score** per library, regenerated from each repo's
  `parity.json` on deploy, with a per-library breakdown (cases synced, gaps closed)
  and an **interactive React-Flow-style pipeline diagram** (draggable, pannable /
  zoomable canvas, live GitHub Actions status, Replay).
- Full multi-section **How-to guide** covering the whole family.
### Changed
- README lists all 33 libraries grouped by domain with live parity scores.
- Pages CI regenerates per-library API docs + live parity metrics on each deploy.

## [0.1.0] - 2026-07-04
### Added
- Initial public release of the unified `malcolmston/go` aggregator.
- Liquid-Glass landing + documentation site (per-library tabs, Node → Go code
  comparisons, how-to, FAQ) published to GitHub Pages.
- Live version badges and a Releases page that read each library's tags and
  release notes from the GitHub API at load time.
- Sibling libraries wired in as git submodules with a `go.work` workspace and a
  runnable `examples/integration` server (Express + Socket.IO + morgan).
- Submodule-sync pipeline and automated releases (VERSION-driven tags + GitHub
  Releases, moving `stable` tag).

[Unreleased]: https://github.com/malcolmston/go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/malcolmston/go/releases/tag/v0.1.0
