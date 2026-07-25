# parity — measure a Go port by running the library it ports

`go get github.com/malcolmston/go/parity`

Every malcolmston/\* library is a port of something that already exists: chalk,
express, numpy, activerecord. The question a port has to answer is not "do my
tests pass" but "do I do what the original does". This package answers it the
only way that cannot go stale — by **installing the original library and running
it**, then asking it and the Go port the same questions and comparing.

```go
func TestParityChalk(t *testing.T) {
	s := parity.New(t, parity.Config{
		Repo:     "chalk",
		Upstream: "chalk/chalk",
		Oracle:   parity.Node("chalk@5.3.0"), // the real npm package
	})

	s.Case("bold", parity.Case{
		Upstream: `const c = (await imp('chalk')).default; return c.bold(input)`,
		Input:    "hello",
		Go:       func() (any, error) { return chalk.Bold("hello"), nil },
	})

	s.Case("throws on bad hex", parity.Case{
		Upstream: `const c = (await imp('chalk')).default; return c.hex('nope')('x')`,
		Go:       func() (any, error) { return chalk.Hex("nope")("x") },
	})

	s.Run()
}
```

`go test -run TestParity ./...` installs `chalk@5.3.0`, starts Node once, asks
it every case, compares each answer to the port's, cross-compiles the package
for every supported GOOS/GOARCH, and writes `parity.json`.

## Why run the original

Hand-written expectations record what someone believed upstream did on the day
they wrote them. Running upstream records what it does today: a new release
re-scores the port for free, and no expectation can quietly drift out of date.
Errors count too — a port that returns a value where upstream throws is not at
parity, so both sides' failures are compared, not just their successes.

## Runtimes

| constructor | ecosystem | case source is |
| --- | --- | --- |
| `parity.Node("chalk@5.3.0")` | npm / Node.js | an async function body with `input`, `require`, `imp` |
| `parity.Python("numpy==2.0.1")` | PyPI / CPython | a function body with `input` |
| `parity.Ruby("activesupport@7.1")` | RubyGems / CRuby | a block body with `input` |

Any other ecosystem is one `parity.Runtime` literal away — an interpreter, a
driver program speaking the newline-delimited JSON protocol in `oracle.go`, and
an install command. Elixir, Rust and Java ports plug in the same way.

Values cross the boundary as canonical JSON: numbers as numbers (with `NaN` and
`±Infinity` spelled out, since JSON cannot), bytes as base64, time as RFC 3339.
Both sides are normalized identically, so a difference is a difference in
behavior and never in how two ecosystems spell the same value. Floats compare
within `Config.Tolerance` (default `1e-9`, relative at large magnitudes).

## When the original cannot run

A laptop offline, a CI job without Ruby, an upstream release that no longer
installs. Then the suite replays **goldens** — upstream's own answers, recorded
on a run that did have it — and the report says `"mode": "golden"` so nobody
mistakes a replay for a measurement.

```
PARITY_RECORD=1 go test -run TestParity ./...   # record upstream's answers
```

Commit `testdata/parity/*.json` alongside the suite. Re-record when the pinned
upstream version changes; the diff shows exactly which upstream behaviors moved.

## What lands in parity.json

```jsonc
{
  "schema": 2,
  "repo": "chalk",
  "upstream": "chalk/chalk",
  "mode": "live",
  "oracle": { "runtime": "node", "package": "chalk@5.3.0", "version": "v22.14.0" },
  "cases": { "total": 30, "matched": 29, "mismatched": 1, "skipped": 0, "errored": 0 },
  "parityBefore": "83%",   // the same case set, scored with the previous run's failures
  "parityAfter": "96%",    // measured now
  "gapsClosed": 4,
  "failing": ["TestParityChalk/rgb"],
  "regressions": [],
  "platforms": [{ "goos": "linux", "goarch": "amd64", "ok": true }, "…"]
}
```

`before → after` is movement on today's cases, not on a remembered denominator,
so the number cannot be improved by dropping a case. Failing cases are **named**:
a gap is published, never rounded away. `Case.Skip` records a deliberate
divergence — it still appears in the report, as a gap rather than a pass.

The landing aggregates every repo's `parity.json` via `go run ./genparity`.

## Cross-compiling

A behavior score for code that will not build on `windows/amd64` is a score for
something nobody can run there, so every parity run also builds the package for
`DefaultPlatforms` (linux, darwin, windows × amd64/arm64) with `CGO_ENABLED=0`.
Set `PARITY_CROSS=0`, or run `go test -short`, to skip it.

## CI

Point the repo's `parity.yml` at the family's reusable workflow, which installs
the upstream runtime and gates on regressions:

```yaml
jobs:
  parity:
    uses: Malcolmston/go/.github/workflows/parity-reusable.yml@main
    with:
      runtimes: node
```

## Environment

| variable | effect |
| --- | --- |
| `PARITY_RECORD=1` | record upstream's answers into the golden files |
| `PARITY_GOLDEN=1` | replay goldens even when upstream could run |
| `PARITY_OFFLINE=1` | never install upstream packages |
| `PARITY_CROSS=0` | skip the cross-compile check |
| `PARITY_<RUNTIME>_DIR` | use a preinstalled upstream tree instead of installing |
| `PARITY_DIR` / `PARITY_OUT` | suite fragment directory / report path |
