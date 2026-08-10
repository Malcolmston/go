# chalk parity coverage

- upstream oracle: **chalk@5.3.0** (`parity/chalk/node/package.json`, pinned exactly, installed under `parity/chalk/node/node_modules/`)
- Go port under test: **github.com/malcolmston/chalk v0.4.0** (consumed as a published module; no `replace` directive)
- cases: **680** across 14 files in `cases/`; **578** match, **96** differ, **6** documented deviations
- case-level parity: **85.76%** (578/674 compared cases; deviations excluded from the denominator)
- every case is run at a *forced* colour level (0, 1, 2 and/or 3) on both sides -- upstream via `new Chalk({level})`, Go via `chalk.SetLevel` -- so no result depends on terminal detection.

## How the upstream inventory was derived

Mechanically, from the installed package -- not from the README. `parity/chalk/node/enum.mjs` is the script; it was run as:

```sh
cd parity/chalk/node && node enum.mjs
```

It prints three lists:

```js
import * as mod from 'chalk';
const c = mod.default;
Object.keys(mod).sort()                        // module exports
[...Object.keys(c), ...Object.getOwnPropertyNames(c)]  // instance own props
// plus Object.getOwnPropertyNames() walked up the prototype chain
```

Result: **13** module exports (`Chalk`, `backgroundColorNames`, `backgroundColors`, `chalkStderr`, `colorNames`, `colors`, `default`, `foregroundColorNames`, `foregroundColors`, `modifierNames`, `modifiers`, `supportsColor`, `supportsColorStderr`), **3** own properties on the default instance (`length`, `level`, `name`), and **53** prototype members (52 styles plus `constructor`).

The Go side was enumerated with:

```sh
GOWORK=off go doc -all github.com/malcolmston/chalk
```

## Upstream symbols

`differs` means at least one case that exercises the symbol diverged; the note says how. A symbol with no case is `untested`, never `match`.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `chalk.ansi256` | `(*chalk.Style).Ansi256` | differs | `ansi256-0-l0`, `ansi256-0-l1`, `ansi256-0-l2` (+57 more) | at level 1 the port degrades the palette index to ANSI-16; chalk emits `38;5;N` unchanged at every level >= 1 |
| `chalk.bgAnsi256` | `(*chalk.Style).BgAnsi256` | differs | `bgansi256-0-l0`, `bgansi256-0-l1`, `bgansi256-0-l2` (+49 more) | same level-1 degradation difference as `ansi256`; also out-of-range indices are wrapped mod 256 by the port and emitted raw by chalk |
| `chalk.bgBlack` | `(*chalk.Style).BgBlack` | match | `bgBlack-l0`, `bgBlack-l1`, `bgBlack-l3` |  |
| `chalk.bgBlackBright` | `(*chalk.Style).BgBrightBlack` | match | `bgBlackBright-l0`, `bgBlackBright-l1`, `bgBlackBright-l3` |  |
| `chalk.bgBlue` | `(*chalk.Style).BgBlue` | differs | `bgBlue-l0`, `bgBlue-l1`, `bgBlue-l3` (+11 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair |
| `chalk.bgBlueBright` | `(*chalk.Style).BgBrightBlue` | match | `bgBlueBright-l0`, `bgBlueBright-l1`, `bgBlueBright-l3` |  |
| `chalk.bgCyan` | `(*chalk.Style).BgCyan` | match | `bgCyan-l0`, `bgCyan-l1`, `bgCyan-l3` |  |
| `chalk.bgCyanBright` | `(*chalk.Style).BgBrightCyan` | match | `bgCyanBright-l0`, `bgCyanBright-l1`, `bgCyanBright-l3` |  |
| `chalk.bgGray` | `(*chalk.Style).BgGray` | match | `bgGray-l0`, `bgGray-l1`, `bgGray-l3` (+4 more) |  |
| `chalk.bgGreen` | `(*chalk.Style).BgGreen` | match | `bgGreen-l0`, `bgGreen-l1`, `bgGreen-l3` |  |
| `chalk.bgGreenBright` | `(*chalk.Style).BgBrightGreen` | match | `bgGreenBright-l0`, `bgGreenBright-l1`, `bgGreenBright-l3` |  |
| `chalk.bgGrey` | — | missing | `bgGrey-l0`, `bgGrey-l1`, `bgGrey-l3` (+1 more) | the port has `BgGray` but no `BgGrey` alias |
| `chalk.bgHex` | `(*chalk.Style).BgHex` | differs | `bghex-f80-l0`, `bghex-f80-l1`, `bghex-f80-l2` (+37 more) | same as `hex` |
| `chalk.bgMagenta` | `(*chalk.Style).BgMagenta` | match | `bgMagenta-l0`, `bgMagenta-l1`, `bgMagenta-l3` |  |
| `chalk.bgMagentaBright` | `(*chalk.Style).BgBrightMagenta` | match | `bgMagentaBright-l0`, `bgMagentaBright-l1`, `bgMagentaBright-l3` |  |
| `chalk.bgRed` | `(*chalk.Style).BgRed` | differs | `bgRed-l0`, `bgRed-l1`, `bgRed-l3` (+4 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair |
| `chalk.bgRedBright` | `(*chalk.Style).BgBrightRed` | match | `bgRedBright-l0`, `bgRedBright-l1`, `bgRedBright-l3` |  |
| `chalk.bgRgb` | `(*chalk.Style).BgRGB` | differs | `bgrgb-0-0-0-l0`, `bgrgb-0-0-0-l1`, `bgrgb-0-0-0-l2` (+41 more) | same as `rgb` |
| `chalk.bgWhite` | `(*chalk.Style).BgWhite` | match | `bgWhite-l0`, `bgWhite-l1`, `bgWhite-l3` (+4 more) |  |
| `chalk.bgWhiteBright` | `(*chalk.Style).BgBrightWhite` | match | `bgWhiteBright-l0`, `bgWhiteBright-l1`, `bgWhiteBright-l3` |  |
| `chalk.bgYellow` | `(*chalk.Style).BgYellow` | match | `bgYellow-l0`, `bgYellow-l1`, `bgYellow-l3` |  |
| `chalk.bgYellowBright` | `(*chalk.Style).BgBrightYellow` | match | `bgYellowBright-l0`, `bgYellowBright-l1`, `bgYellowBright-l3` |  |
| `chalk.black` | `(*chalk.Style).Black` | match | `fg-black-l0`, `fg-black-l1`, `fg-black-l2` (+1 more) |  |
| `chalk.blackBright` | `(*chalk.Style).BrightBlack` | match | `fg-blackBright-l0`, `fg-blackBright-l1`, `fg-blackBright-l3` |  |
| `chalk.blue` | `(*chalk.Style).Blue` | differs | `chain-04-l0`, `chain-04-l1`, `chain-04-l2` (+29 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair |
| `chalk.blueBright` | `(*chalk.Style).BrightBlue` | match | `fg-blueBright-l0`, `fg-blueBright-l1`, `fg-blueBright-l3` |  |
| `chalk.bold` | `(*chalk.Style).Bold` | differs | `chain-00-l0`, `chain-00-l1`, `chain-00-l2` (+31 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair |
| `chalk.cyan` | `(*chalk.Style).Cyan` | match | `fg-cyan-l0`, `fg-cyan-l1`, `fg-cyan-l2` (+1 more) |  |
| `chalk.cyanBright` | `(*chalk.Style).BrightCyan` | match | `fg-cyanBright-l0`, `fg-cyanBright-l1`, `fg-cyanBright-l3` |  |
| `chalk.dim` | `(*chalk.Style).Dim` | match | `chain-09-l0`, `chain-09-l1`, `chain-09-l2` (+8 more) |  |
| `chalk.gray` | `(*chalk.Style).Gray` | match | `chain-19-l0`, `chain-19-l1`, `chain-19-l2` (+4 more) |  |
| `chalk.green` | `(*chalk.Style).Green` | differs | `chain-03-l0`, `chain-03-l1`, `chain-03-l2` (+15 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair |
| `chalk.greenBright` | `(*chalk.Style).BrightGreen` | match | `fg-greenBright-l0`, `fg-greenBright-l1`, `fg-greenBright-l3` |  |
| `chalk.grey` | `(*chalk.Style).Grey` | match | `fg-grey-l0`, `fg-grey-l1`, `fg-grey-l3` |  |
| `chalk.hex` | `(*chalk.Style).Hex` | differs | `chain-10-l0`, `chain-10-l1`, `chain-10-l2` (+82 more) | malformed input is parsed leniently by chalk (`#ff880`, `#ff88000`, `#ff8800ff`, `0xff8800`, `" #ff8800"`, `#ff-800`) but falls back to black in the port; level-1 ANSI-16 rounding also differs. Neither side ever errors. |
| `chalk.hidden` | `(*chalk.Style).Hidden` | match | `mod-hidden-l0`, `mod-hidden-l1`, `mod-hidden-l2` (+1 more) |  |
| `chalk.inverse` | `(*chalk.Style).Inverse` | match | `chain-18-l0`, `chain-18-l1`, `chain-18-l2` (+5 more) |  |
| `chalk.italic` | `(*chalk.Style).Italic` | match | `chain-09-l0`, `chain-09-l1`, `chain-09-l2` (+5 more) |  |
| `chalk.magenta` | `(*chalk.Style).Magenta` | match | `fg-magenta-l0`, `fg-magenta-l1`, `fg-magenta-l2` (+1 more) |  |
| `chalk.magentaBright` | `(*chalk.Style).BrightMagenta` | match | `fg-magentaBright-l0`, `fg-magentaBright-l1`, `fg-magentaBright-l3` |  |
| `chalk.overline` | `(*chalk.Style).Overline` | match | `mod-overline-l0`, `mod-overline-l1`, `mod-overline-l2` (+1 more) |  |
| `chalk.red` | `(*chalk.Style).Red` | differs | `chain-00-l0`, `chain-00-l1`, `chain-00-l2` (+163 more) | repeating a style of the same category (`red.green`, `bold.bold`, `bgRed.bgBlue`) makes the port re-open the outer code before the final close; chalk emits only the nested pair; also `chalk.red('a','b')` joins operands with a space where `Sprint` concatenates and renders `nil` as `<nil>` |
| `chalk.redBright` | `(*chalk.Style).BrightRed` | match | `fg-redBright-l0`, `fg-redBright-l1`, `fg-redBright-l3` |  |
| `chalk.reset` | `(*chalk.Style).Reset` | match | `chain-07-l0`, `chain-07-l1`, `chain-07-l2` (+9 more) |  |
| `chalk.rgb` | `(*chalk.Style).RGB` | differs | `chain-12-l0`, `chain-12-l1`, `chain-12-l2` (+51 more) | level-1 ANSI-16 rounding differs (mid greys, saturated colours pick the bright variant); the port clamps channels to 0-255 where chalk emits them raw |
| `chalk.strikethrough` | `(*chalk.Style).Strikethrough` | match | `chain-09-l0`, `chain-09-l1`, `chain-09-l2` (+5 more) |  |
| `chalk.underline` | `(*chalk.Style).Underline` | match | `chain-02-l0`, `chain-02-l1`, `chain-02-l2` (+12 more) |  |
| `chalk.visible` | `(*chalk.Style).Visible` | match | `chain-16-l0`, `chain-16-l1`, `chain-16-l2` (+9 more) |  |
| `chalk.white` | `(*chalk.Style).White` | match | `fg-white-l0`, `fg-white-l1`, `fg-white-l2` (+1 more) |  |
| `chalk.whiteBright` | `(*chalk.Style).BrightWhite` | match | `fg-whiteBright-l0`, `fg-whiteBright-l1`, `fg-whiteBright-l3` |  |
| `chalk.yellow` | `(*chalk.Style).Yellow` | match | `fg-yellow-l0`, `fg-yellow-l1`, `fg-yellow-l2` (+4 more) |  |
| `chalk.yellowBright` | `(*chalk.Style).BrightYellow` | match | `fg-yellowBright-l0`, `fg-yellowBright-l1`, `fg-yellowBright-l3` |  |
| `Chalk` | `chalk.New + chalk.SetLevel` | differs | every case (level round-trip: `level-0` .. `level-3`, `level-invalid-*`) | every case constructs one; chalk rejects a level outside 0..3 while `SetLevel` accepts any int |
| `Chalk.prototype.constructor` | `chalk.New` | match | every case | the same class object as the `Chalk` row above |
| `chalk.level` | `chalk.GetLevel / chalk.SetLevel` | differs | `enabled-0`, `enabled-1`, `enabled-2` (+16 more) | value round-trips for 0..3; the port also accepts out-of-range levels |
| `chalk.length` | — | untested | — | `Function.prototype.length`, not part of the API |
| `chalk.name` | — | untested | — | `Function.prototype.name`, not part of the API |
| `backgroundColorNames (module export)` | — | missing | `names-backgroundColorNames` | no equivalent export (`names-backgroundColorNames`) |
| `backgroundColors (module export)` | — | missing | — | as `colors` |
| `chalkStderr (module export)` | — | missing | — | no stderr-specific instance in the port |
| `colorNames (module export)` | — | missing | `names-colorNames` | no equivalent export (`names-colorNames`) |
| `colors (module export)` | — | missing | — | raw open/close SGR pair table; the port keeps its pairs unexported |
| `default (module export)` | `chalk.Red / chalk.Bold / ... (package-level shortcuts)` | untested | — | the auto-detecting default instance; every case builds an explicit `new Chalk({level})` instead, so this is not compared |
| `foregroundColorNames (module export)` | — | missing | `names-foregroundColorNames` | no equivalent export (`names-foregroundColorNames`) |
| `foregroundColors (module export)` | — | missing | — | as `colors` |
| `modifierNames (module export)` | — | missing | `names-modifierNames` | no equivalent export (`names-modifierNames`) |
| `modifiers (module export)` | — | missing | — | as `colors` |
| `supportsColor (module export)` | `chalk.SupportsColor, chalk.HasBasic, chalk.Has256, chalk.HasTrueColor` | untested | — | result of environment sniffing; cannot be forced on the upstream side, so deliberately not compared |
| `supportsColorStderr (module export)` | — | missing | — | the port detects from stdout only |
| `a styler called as a template tag (tagged-template usage)` | — | missing | `template-chain`, `template-interp`, `template-plain` | chalk 5 dropped its template parser (now `chalk-template`) but a styler is still callable as a tag; the port has no equivalent -- deviation, see API-DEVIATIONS.md |

## Go-only symbols (extra)

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `chalk.Strip` | extra | `strip-basic`, `strip-nested`, `strip-plain` | deviation: chalk leaves this to `strip-ansi` |
| `chalk.VisibleLength` | extra | — | deviation: chalk leaves this to `string-width` |
| `chalk.Enabled, chalk.SetEnabled, chalk.ResetDetection` | extra | `enabled-0` .. `enabled-3` | `Enabled` compared against `chalk.level > 0`; the other two have no upstream analogue |
| `chalk.HasBasic, chalk.Has256, chalk.HasTrueColor, chalk.SupportsColor` | extra | — | finer-grained than upstream's single `supportsColor` object |
| `(*chalk.Style).Level` | extra | `level-pin-*` | per-style level pin; upstream needs a new `Chalk` instance instead |
| `(*chalk.Style).HSL, .HSV, .HWB, .BgHSL, .BgHSV, .BgHWB` | extra | — | colour models chalk does not offer |
| `chalk.HSL, chalk.HSV, chalk.HWB` | extra | — | package-level forms of the above |
| `chalk.HexToRGB, chalk.RGBToHex, chalk.RGBToAnsi16, chalk.RGBToAnsi256, chalk.Ansi256ToRGB, chalk.Ansi256ToAnsi16, chalk.HSLToRGB, chalk.RGBToHSL, chalk.HSVToRGB, chalk.RGBToHSV, chalk.HWBToRGB, chalk.RGBToHWB` | extra | — | colour-conversion helpers; chalk keeps the equivalents inside its vendored `ansi-styles`, unreachable through its `exports` map, so they are not compared |
| `(*chalk.Style).Sprint, .Sprintf, .Sprintln, .Print, .Printf, .Println` | extra | every `render` case goes through `Sprint` | upstream applies a style by calling the styler; `Sprint` is the port's spelling |
| `chalk.Red, chalk.Bold, ... (68 package-level shortcuts)` | extra | — | one-off forms of the `Style` methods; the parity cases exercise the `Style` methods, which share the same implementation |

## Totals

| status | upstream symbols |
| --- | --- |
| match | 40 |
| differs | 14 |
| missing | 12 |
| untested | 4 |
| **total upstream symbols** | **70** |
| extra (Go-only) | 10 rows |

**Symbol parity: 74.1%** -- 40 of the 54 symbols actually compared (`match` / (`match` + `differs`)). 12 symbols are missing from the port and 4 could not be compared.

**Case parity: 85.76%** -- 578 of 674 compared cases (680 cases in total, 6 of them documented deviations that are counted separately). Regenerate with:

```sh
GOWORK=off go test ./parity/chalk/
```

## Error-message differences

Cases are compared on *whether* a call failed, not on the message text. The differences observed:

- `new Chalk({level: 4})` -> `The \`level\` option should be an integer from 0 to 3`; `chalk.SetLevel(4)` accepts it and reports level 4 back (`level-invalid-4`, `level-invalid-99`, `level-invalid-neg1`).
- Neither side raises on malformed hex, an out-of-range RGB channel or an out-of-range 256-palette index: both fall back silently, to different values.
