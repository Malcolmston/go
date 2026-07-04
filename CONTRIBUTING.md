# Contributing

Thanks for helping improve this project! It's a dependency-light Go port of a
Node.js library, so contributions that improve fidelity, tests, or docs are
especially welcome.

## Getting started
- Requires **Go 1.23+**.
- `go test ./...` — run the tests.
- `go test -race -covermode=atomic -coverprofile=coverage.out ./...` — race + coverage.
- `golangci-lint run` — lint (config in `.golangci.yml`).
- `gofmt -w .` — format.

## Pull requests
1. Branch from `main` and keep changes focused.
2. Add tests for any new behavior; keep them deterministic.
3. Make sure `gofmt -l .` is empty, and `go vet ./...`, tests, and lint all pass —
   CI enforces all of these on Go 1.23 and 1.24.
4. Preserve the **Node-mirroring API** (names and semantics are chosen to match
   the original library on purpose).

## Reporting issues
Open an issue with a minimal reproduction and the Go version you're using.

By contributing, you agree that your contributions are licensed under the MIT License.
