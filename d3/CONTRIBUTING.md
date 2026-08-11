# Contributing

Thanks for helping improve this project! It's a dependency-light Go port of the
computational half of [d3](https://github.com/d3/d3), so contributions that
improve fidelity, tests, or docs are especially welcome.

## Getting started
- Requires **Go 1.24+**.
- `go test ./...` — run the tests.
- `go test -race -covermode=atomic -coverprofile=coverage.out ./...` — race + coverage.
- `golangci-lint run` — lint (config in `.golangci.yml`).
- `gofmt -w .` — format.

## Pull requests
1. Branch from `main` and keep changes focused.
2. Add tests for any new behavior; keep them deterministic. Seed `d3/random`
   explicitly rather than relying on a default source.
3. Make sure `gofmt -l .` is empty, and `go vet ./...`, tests, and lint all pass —
   CI enforces all of these.
4. Preserve the **d3-mirroring API** (names and semantics are chosen to match
   the original library on purpose). If a Go constraint forces a different shape,
   add a row to [API-DEVIATIONS.md](API-DEVIATIONS.md) explaining why in the same
   PR. An undocumented deviation is the one thing that will get a change sent
   back.

## Working on the numerics

A few invariants hold the packages together. Breaking one produces bugs that
surface far away from the change, so they are worth stating:

- **Float output must match d3's, digit for digit, where it reasonably can.**
  d3's scales, ticks and interpolators are used to position pixels; a
  last-place difference in a tick value changes an axis label. Compare against
  upstream values in tests rather than against a re-derivation of the same
  formula.
- **`d3/array`'s `Ticks` is load-bearing.** Every quantitative scale's `Nice`
  and `Ticks` route through it. Changing its step selection changes every axis
  in every downstream chart.
- **Path output is a string, and its formatting is part of the API.** Number
  formatting in `d3/path` (how many digits, when a coordinate is elided) is
  observable in golden tests and in rendered SVG. Do not "tidy" it casually.
- **Empty input is not an error everywhere.** Where d3 returns `undefined` for
  an empty domain, this port has a documented Go answer — usually a zero value
  plus an `ok` bool, or a typed error. Pick the one the neighbouring functions
  in that package already use; do not introduce a third convention.
- **NaN propagates, it does not panic.** A scale handed `NaN` produces `NaN`,
  matching d3. Guarding it into a `0` silently produces a chart that is wrong
  rather than visibly broken.
- **Accessors are how generic input is read.** Add a `…With(accessor)` form
  rather than reflecting over struct fields.

## Fidelity claims

Do not put a number in `parity.json` that was not measured. If you build the
parity corpus described in [BACKLOG.md](BACKLOG.md), the figure and the method
that produced it both belong in the file's `notes`.

## Reporting issues
Open an issue with a minimal reproduction and the Go version you're using. For a
numeric difference, include the input, what this port produced, and what the
equivalent d3 call produces in Node — the actual values, not a description of
them.

By contributing, you agree that your contributions are licensed under the MIT License.
