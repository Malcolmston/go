# prompts — upstream API inventory vs the Go port

- **Upstream oracle:** `prompts@2.4.2` (npm, terkelg/prompts), pinned in
  `node/package.json`.
- **Port:** `github.com/malcolmston/chalk/prompts` — a nested package inside the
  `chalk` sub-repo that ports a *different* npm package, so it is scored here and
  not by `parity/chalk/`.
- **Score:** see `parity.json`, rewritten by `GOWORK=off go test .`

## How the upstream list was derived

Mechanically, from the real installed package:

```console
$ cd node && node -e "const p = require('prompts'); \
    console.log('top-level:', Object.keys(p).sort().join(' ')); \
    console.log('types:', Object.keys(p.prompts).sort().join(' '))"
top-level: inject override prompt prompts
types: autocomplete autocompleteMultiselect confirm date invisible list multiselect number password select text toggle
```

Per-type options come from the installed sources
`node_modules/prompts/lib/prompts.js` (the JSDoc `@param` list on each type) and
the element constructors under `node_modules/prompts/lib/elements/`.

The Go side comes from `go doc -all ./prompts`.

## How an interactive library is compared at all

prompts renders to a terminal and reads keystrokes, so there is no pure function
to call. The harness drives **both** sides with a scripted keyboard: each case
carries a list of key tokens (`"bob"`, `"<enter>"`, `"<down>"`, `"<space>"`,
`"<backspace>"`, `"<tab>"`, `"<ctrl-c>"`), the runners turn them into the same raw
bytes, and feed them to the prompt through its injectable input — upstream's
`opts.stdin`, the port's `cfg.In`. Prompt drawing goes to a sink on both sides
(`stdout.columns` pinned to 80 upstream), so no case depends on a terminal, a
clock or a random value.

**prompts' own `inject()` API is deliberately not used.** `inject` short-circuits
the prompt element entirely — `lib/index.js` returns the injected value with
`skipValidation = true` and no type coercion — so an injected `select` answer
never touches the choice list and an injected `number` answer is never parsed.
Comparing against that would compare the harness with itself. Scripted stdin
exercises the real element.

Validators and formatters cannot travel in language-agnostic JSON, so they are
referenced by name (`nonEmpty`, `min3`, `reject`, `even`, `upper`, `trim`) and
defined identically in both runners.

## Symbol inventory

### Prompt types (`prompts.prompts.*`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `text` | `prompts.Input` | match | `text-*` (17), `validate-*` (7), `cancel-text*` (3) | |
| `password` | `prompts.Password` | match | `password-*` (6), `validate-password-*`, `cancel-password` | upstream always echoes `*`; the port echoes nothing unless `Mask` is set, which does not change the value |
| `number` | `prompts.Number` | differs | `number-*` (14 match, 6 deviations) | bounds, keystroke filtering and float rounding differ; see below |
| `confirm` | `prompts.Confirm` | differs | `confirm-*` (10 match, 1 deviation) | upstream submits on the first y/n keypress, the port reads a line |
| `select` | `prompts.Select` | match | `select-*` (15), `cancel-select` | |
| `multiselect` | `prompts.MultiSelect` | match | `ms-*` (13), `cancel-multiselect` | |
| `invisible` | `prompts.Password` with `Mask` unset | differs | — | untested: upstream's `invisible` is `text` with a third render style; the port folds it into `Password` (hidden is its default), so there is no separate symbol to drive |
| `toggle` | — | missing | — | not ported |
| `list` | — | missing | — | not ported (`text` + a delimiter split) |
| `date` | — | missing | — | not ported |
| `autocomplete` | — | missing | — | not ported |
| `autocompleteMultiselect` | — | missing | — | not ported |

### Top-level API

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `prompts(questions)` / `prompts.prompt` | — | missing | — | there is no question-list runner: the port's prompts are called one at a time as ordinary functions. Everything the runner adds (`format`, `onSubmit`, `onCancel`) is covered by the per-prompt equivalents |
| `prompts.inject(values)` | — | missing | — | scripting is done with `cfg.In` instead, which drives the real prompt rather than bypassing it |
| `prompts.override(values)` | — | missing | — | no CLI-argument override layer |
| `question.format` | `InputConfig.Transform` | match | `text-format-*`, `validate-with-format` | |
| `question.validate` | `*Config.Validate` | match | `validate-*` (8) | |
| `question.initial` | `*Config.Default` (`SelectConfig.Default` is an index) | match | `text-initial-*`, `number-initial-*`, `confirm-enter-default-*`, `select-initial-*` | |
| `question.choices[].title` / `.value` / `.selected` / `.disabled` | `Choice.Name` / `.Value` / `.Checked` / `.Disabled` | match | `select-explicit-*`, `ms-preselected*`, `ms-explicit-values` | `Choice.ResolvedValue` reproduces upstream's "value defaults to the label" rule |
| `question.onState` | — | missing | — | no per-keystroke state callback |
| `question.onRender` | — | missing | — | styling is fixed rather than themeable |
| `question.stdin` / `question.stdout` | `*Config.In` / `*Config.Out` | match | every case | this is what makes the harness possible on both sides |
| Ctrl-C / Ctrl-D abort → `onCancel` | `prompts.ErrCanceled` | match | `cancel-*` (8) | |

### Go-only surface

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `prompts.Slides` / `prompts.SlidesConfig` | extra | — | untested: a paged text viewer. Upstream `prompts` has nothing like it — there is no oracle to compare against, so it is out of scope for this harness and covered by the port's own tests |
| `SelectConfig.MaxVisible` / `MultiSelectConfig.MaxVisible` | extra | — | untested: upstream's `limit`. It changes only what is *drawn*, and drawing is not compared (see below) |
| `NumberConfig.Integer` | extra | `number-*` | the inverse of upstream's `float`, and the two are driven together |
| `Choice.ResolvedValue` | extra | `select-*`, `ms-*` | makes the documented "Value defaults to Name" rule real |

## Counts

| status | symbols |
| --- | --- |
| match | 13 |
| differs | 3 |
| missing | 10 |
| extra | 4 |
| untested | 4 (`invisible`, `Slides`, `MaxVisible`, and rendering — see below) |

**Parity over the symbols actually compared: 13 / 16 = 81.3 %.**
**Case-level parity: see `parity.json`.**

## Recorded as `untested`, and why

Each of these is a surface the two libraries genuinely cannot be driven through
equivalently. No case was invented for them.

- **Everything that is drawn.** The prompt frame — the prefix symbol, the
  question, the pointer, the checkbox glyphs, the hint text, the inline error, the
  answer summary, the redraw escape sequences — is written by each library in its
  own style with its own colour library (`kleur` upstream, chalk here). There is no
  sense in which one is "wrong", so the harness compares only the *returned value*.
  `MaxVisible`/`limit` paging falls entirely inside this boundary.
- **Escape as cancel.** Both libraries treat Esc as an abort, but they
  disambiguate a bare `0x1b` differently: upstream's `readline` waits out a 50 ms
  `escapeCodeTimeout`, the port's key reader looks ahead exactly one byte. A
  scripted Esc is therefore not the same event on the two sides, and a case built
  on it would measure the decoders' timing, not the prompts. Cancellation is
  covered through Ctrl-C instead (`cancel-*`), which needs no lookahead.
- **`number`'s arrow-key increment.** Upstream's `up`/`down` step the value by
  `increment`, starting from `min ± increment` when the field is empty — with no
  bounds set that means starting from `±Infinity`, which is not even JSON. The port
  has no increment behaviour at all, so there is nothing to compare rather than
  something that disagrees.
- **`multiselect`'s `a` (toggle all), `left`/`right` (deselect/select) and
  `max`.** The port maps `left`/`right` to cursor movement and has no
  toggle-all or maximum-selection key, so these are absent rather than different.
- **`prompts.Slides`.** Go-only; upstream has no paged viewer, so there is no
  oracle.
- **`invisible`.** The port has no separate symbol: hidden input is
  `Password` with no `Mask`, which is already compared.

## Behaviour differences that do not change a value

- Error and hint text differs throughout (`Please Enter A Valid Value` upstream
  vs the validator's own message here). The harness compares *whether* a call
  failed, not the message.
- After a validation failure upstream leaves the rejected text in the buffer and
  parks the cursor at its end; the port clears the line and re-prompts. The
  `validate-*` cases neutralise this by clearing the line with backspaces, so both
  sides retry from empty; where it cannot be neutralised (`number`, where upstream
  also accumulates a `typed` string) it is recorded as a deviation.
- Reaching end of input is Enter-on-the-current-buffer in the port, *including*
  running the validator. Upstream simply never resolves. Every case script
  therefore ends in a submitting key or `<ctrl-c>`.
