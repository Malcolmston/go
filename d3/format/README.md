# format — Go port of d3-format — turning numbers into strings that humans read comfortably

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/format.svg)](https://pkg.go.dev/github.com/malcolmston/d3/format)

Package format is a Go port of d3-format — turning numbers into strings that
humans read comfortably.

The problem it solves is narrower than fmt's and much more specific: when you
label an axis, or print a table of measurements, you want every label to agree
on how many digits it shows, on where the thousands separators go, and on
whether 1300000 is spelled "1,300,000" or "1.3M". Go's fmt verbs cannot say
any of that in one token. d3's specifier can, and the specifier is a string,
so it can be a configuration value, a URL parameter or a column definition:

```go
f := format.MustNew(",.2f")
f(1234.5678) // "1,234.57"
format.MustNew("$,.2f")(1234.5678)  // "$1,234.57"
format.MustNew(".1%")(0.123)        // "12.3%"
format.MustNew(".3s")(1300000)      // "1.30M"
```

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/format"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func FormatPrefix(specifier string, value float64) func(float64) string` | FormatPrefix is `NewPrefix` for specifiers known at compile time; it panics on a malformed specifier. |
| `func MustNew(specifier string) func(float64) string` | MustNew is `New` for specifiers known at compile time; it panics on a malformed specifier. |
| `func New(specifier string) (func(float64) string, error)` | New compiles a specifier into a formatting function. |
| `func NewPrefix(specifier string, value float64) (func(float64) string, error)` | NewPrefix compiles specifier into a formatter that pins the SI prefix to the one appropriate for value, instead of letting each number choose its own. |
| `func PrecisionFixed(step float64) int` | PrecisionFixed returns the number of decimal places needed so that ticks spaced step apart are all distinguishable in fixed ("f") notation. |
| `func PrecisionPrefix(step, value float64) int` | PrecisionPrefix returns the number of decimal places needed for the SI-prefix ("s") notation of values near value, when ticks are step apart. |
| `func PrecisionRound(step, max float64) int` | PrecisionRound returns the number of significant digits needed so that ticks spaced step apart are distinguishable, on an axis whose largest value is… |
| `func SIPrefix(exp10 int) string` | SIPrefix returns the SI prefix symbol for a decimal exponent, clamped to the yocto..yotta range that d3 supports. |

### Types

| Type | What it is |
| --- | --- |
| `Locale` | Locale carries the culture-specific parts of number formatting. |
| `Specifier` | Specifier is a parsed d3 format specifier. |

<details>
<summary><code>Locale</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func NewLocale(l Locale) *Locale` | NewLocale returns a ready-to-use locale, filling in en-US defaults for the fields left empty. |
| `func (l *Locale) Compile(s Specifier) func(float64) string` | Compile turns an already-parsed `Specifier` into a formatting function. |
| `func (l *Locale) FormatPrefix(specifier string, value float64) func(float64) string` | FormatPrefix is `Locale.NewPrefix` but panics instead of returning an error. |
| `func (l *Locale) MustNew(specifier string) func(float64) string` | MustNew is `Locale.New` but panics instead of returning an error. |
| `func (l *Locale) New(specifier string) (func(float64) string, error)` | New compiles a specifier against this locale. |
| `func (l *Locale) NewPrefix(specifier string, value float64) (func(float64) string, error)` | NewPrefix compiles a prefix-pinned formatter against this locale. |

</details>

<details>
<summary><code>Specifier</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func DefaultSpecifier() Specifier` | DefaultSpecifier returns the specifier that an empty format string denotes: space fill, right alignment, minus-only sign, no width and no precision. |
| `func ParseSpecifier(s string) (Specifier, error)` | ParseSpecifier parses a d3 format specifier such as "$,.2f" or ">10.3~s". |
| `func (s Specifier) String() string` | String renders the specifier back into the grammar, so that |

</details>

### Constants

`Unset`

### Variables

`EnUS`, `ErrInvalidSpecifier`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/format).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
