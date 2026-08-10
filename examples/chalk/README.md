# chalk example

A runnable program that exercises the **published** module
`github.com/malcolmston/chalk` (Go port of Node [chalk](https://github.com/chalk/chalk)).

- **Resolved version: `v0.4.0`** (a real semver tag, not a pseudo-version — `go get
  github.com/malcolmston/chalk@latest` reported `v0.0.0 => v0.4.0`).
- No `replace` directive: the module is consumed exactly as an outside user would.

## What it demonstrates

1. **Chained immutable styles** — `chalk.New().Bold().Red()`, deriving two styles
   from one shared base and proving the base is unmodified; `Sprint`, `Sprintf`,
   `Println`; modifiers `Bold/Dim/Italic/Underline/Inverse/Strikethrough/Overline`.
2. **Package-level shortcuts** — `chalk.Red`, `chalk.BgGreen`, `chalk.Hex`,
   `chalk.Ansi256`, `chalk.RGB`, `chalk.HSL/HSV/HWB`, bright variants.
3. **Level degradation** — a `Hex("#ff8800")` style rendered at `LevelTrueColor` /
   `Level256` / `LevelBasic` / `LevelNone`, showing that degradation works only when
   `Level` is applied *before* the color is chained, and that a stored style does
   **not** re-degrade afterwards (see holes).
4. **Global level control and probes** — `SetLevel`, `SetEnabled`, `GetLevel`,
   `ResetDetection`, `Enabled`, `SupportsColor`, `HasBasic`, `Has256`,
   `HasTrueColor`, and `Visible()` (output suppressed entirely while color is off).
5. **Nesting and line breaks** — outer style re-opened after an inner close code;
   empty input produces no escapes; LF and CRLF each get their own open/close pair.
6. **Measurement utilities** — `Strip`, `VisibleLength`, and a colored status table
   aligned with them (which visibly mis-aligns for CJK — see holes).
7. **Color-space conversions** — `HexToRGB`, `RGBToHex`, `RGBToAnsi256`,
   `Ansi256ToRGB`, `RGBToAnsi16`, `Ansi256ToAnsi16`, and HSL/HSV/HWB round trips.
8. `Reset` and `Hidden` shortcuts.

The program calls `chalk.SetLevel(chalk.LevelTrueColor)` up front so output is
deterministic when piped, and prints escape-heavy results with `\x1b` escaped so
they can be verified without a terminal.

## Run

```sh
cd examples/chalk
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Builds and runs cleanly, exits 0.

## Holes found in v0.4.0

Nothing panics and nothing had to be deleted, but three of these are real
functional gaps, marked `// HOLE:` in `main.go`.

1. **`Style.Level()` does not re-target a color that was already chained.** The
   escape sequence for `Ansi256`/`RGB`/`Hex`/`HSL`/`HSV`/`HWB` is resolved at
   *build* time from the level in force at that moment (`s.with(fgRGB(r,g,b,
   s.effectiveLevel()), "39")`). So:
   ```go
   warn := chalk.New().Hex("#ff8800")     // frozen as 38;2;255;136;0
   warn.Level(chalk.Level256).Sprint("x") // STILL emits 38;2;255;136;0
   ```
   Only `LevelNone` still takes effect (it short-circuits in `render`). This
   contradicts the doc comment on `Level`, which says it "pins this style to a
   specific color level regardless of the global setting" — you have to write
   `chalk.New().Level(l).Hex(...)` instead. It also means a style stored in a
   package variable ignores any later `chalk.SetLevel`, so a lazily-detected or
   runtime-changed terminal capability is silently missed.
2. **No display-width measurement.** There is no `VisibleWidth` (terminal cells)
   and no `RuneWidth`. `VisibleLength` counts runes, so a CJK ideograph or emoji
   counts 1 where it occupies 2 cells, and any column layout built on it is
   misaligned — the example prints a table that demonstrates this. The package doc
   nonetheless recommends `VisibleLength` for "laying out tables or padding colored
   columns", which is the wrong tool for that job.
3. **`Strip` only removes SGR sequences.** It is `regexp.MustCompile("\x1b\\[[0-9;]*m")`,
   so non-SGR CSI sequences (`\x1b[2J`, cursor movement) and OSC sequences
   (`\x1b]0;title\x07`, window titles and hyperlinks) survive. The doc comment says
   "removes ANSI escape sequences", which overstates it, and the leftovers corrupt
   any width computed from the "stripped" string.
4. **Asymmetric package-level shortcuts.** `Style` has `BgBrightRed` …
   `BgBrightWhite`, `BgHSL`, `BgHSV`, `BgHWB`, but there are no package-level
   counterparts (only `BgBlack…BgWhite`, `BgGray`, `BgRGB`, `BgHex`, `BgAnsi256`).
   Foreground `HSL/HSV/HWB` *do* have shortcuts, so the gap is easy to trip over.
5. **`Print`/`Printf`/`Println` are hard-wired to stdout** (`fmt.Print` internally).
   There is no `Fprint(w, …)`, so styling to `os.Stderr` requires `Sprint` plus your
   own `fmt.Fprint`.
6. **Color level is global mutable process state.** `SetLevel`/`SetEnabled` mutate a
   package-level variable and detect from `os.Stdout` specifically, regardless of
   where output actually goes; there is no `New(Options{Level: …})` constructor as
   in modern Node chalk. Combined with hole 1, the level in force when a style is
   *constructed* is what matters, which makes initialisation order significant.
7. **`RGBToAnsi16` returns an SGR code (30–37/90–97), not a 0–15 palette index** —
   `RGBToAnsi16(255,136,0) == 93`. Matches Node `color-convert` and the doc comment
   says so, but the name misleads.

### Note on the local working tree

The local `chalk/` directory contains uncommitted work that fixes holes 1, 2 and 3
(a `width.go` with `VisibleWidth`/`RuneWidth`, a hand-written `Strip` that handles
CSI/OSC, and render-time color resolution via a `dyn` function). None of that is in
the published `v0.4.0`, which is what this example is written against.
