# Contributing

Thanks for helping improve this project! It's a dependency-light Go port of
React 19, so contributions that improve fidelity, tests, or docs are especially
welcome.

## Getting started
- Requires **Go 1.24+**.
- `go test ./...` — run the tests.
- `go test -race -covermode=atomic -coverprofile=coverage.out ./...` — race + coverage.
- `golangci-lint run` — lint (config in `.golangci.yml`).
- `gofmt -w .` — format.

## Pull requests
1. Branch from `main` and keep changes focused.
2. Add tests for any new behavior; keep them deterministic. Drive renders and
   effects through `Act` or an explicit `Root.Flush` rather than sleeping.
3. Make sure `gofmt -l .` is empty, and `go vet ./...`, tests, and lint all pass —
   CI enforces all of these.
4. Preserve the **React-mirroring API** (names and semantics are chosen to match
   the original library on purpose). If a Go constraint forces a different shape,
   add a row to [API-DEVIATIONS.md](API-DEVIATIONS.md) explaining why in the same
   PR. An undocumented deviation is the one thing that will get a change sent
   back.

## Working on the runtime

A few invariants hold the reconciler together. Breaking one produces bugs that
are very hard to trace, so they are worth stating:

- **Hooks are identified by slot index.** Anything that changes how many hooks a
  component calls, or in what order, is a breaking change to that component's
  state. New hooks must be built on the existing slot primitive so the
  kind-mismatch diagnostic keeps working.
- **A fiber's type and key never change.** A change in either forces a remount;
  code that mutates them in place will silently strand state.
- **Props are read-only.** The runtime reuses props across bailouts, so mutating
  a props map is visible to the previous render. Use `Props.Clone`.
- **Effects never run during render.** They are queued and drained after the tree
  commits, layout effects first.
- **New element types register a renderer** rather than adding a case to the core
  type switch, from an `init` in their own file.
- **Suspense and error boundaries ride on `panic`/`recover`.** Do not add a
  blanket `recover` anywhere on the render path.

## Fidelity claims

Do not put a number in `parity.json` that was not measured. If you build the
parity corpus described in [BACKLOG.md](BACKLOG.md), the figure and the method
that produced it both belong in the file's `notes`.

## Reporting issues
Open an issue with a minimal reproduction and the Go version you're using. For a
rendering difference, include the tree you built, the HTML this port produced,
and the HTML React produces for the equivalent JSX.

By contributing, you agree that your contributions are licensed under the MIT License.
