# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
