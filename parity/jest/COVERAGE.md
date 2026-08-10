# `jest` parity coverage

| | |
| --- | --- |
| Go module | `github.com/malcolmston/jest v0.0.0-20260810111534-fc19c60ec8b3` (consumed as a published module; no `replace`) |
| Upstream oracle | `expect@29.7.0`, plus `jest-mock@29.7.0` for the mock cases |
| Upstream repo | `jestjs/jest`, ecosystem **node** → `parity/jest/node/` |
| Case files | `cases/*.json` (9 groups) |
| Harness | `go test ./parity/jest/` (`GOWORK=off`) |
| Score file | `parity.json`, rewritten by the test |

## Why `expect` is the oracle, not the jest CLI

`github.com/malcolmston/jest` is a port of jest's *assertion engine*, not of its
test runner: the Go package exposes `Expect`/`Matcher` plus mocks and timers, and
its failures go to a `TestReporter` (i.e. `*testing.T`) rather than to a jest
reporter. The jest CLI would add everything that is *not* being ported —
transform pipeline, worker pool, module registry, config resolution, snapshot
files on disk, its own reporters — and it answers a different question
("did this test file pass?") at a much coarser granularity.

Jest publishes exactly the piece under test as a standalone package: `expect`.
`require('expect')` gives the same `expect()` object the CLI installs as a
global, with the same matcher registry and the same `expect.any`/`objectContaining`
asymmetric matchers, and it throws a `JestAssertionError` per assertion. That
makes the oracle a pure function of `(actual, expected, matcher, negated)`, which
is precisely the comparable artefact here: **whether the matcher passes**. It is
also the granularity at which a mismatch names a bug in one matcher rather than
"a test file failed".

`jest-mock@29.7.0` is added for the same reason: it is the standalone package
behind `jest.fn()`, and the call/return matchers need a mock whose recorded
history both sides can be given identically.

## What is compared

Each case is a matcher name, an actual value, zero or more expected arguments and
a `not` flag. Both runners answer `{"ok":true,"value":{"pass":<bool>}}`.

- A thrown `JestAssertionError` (upstream) or an `"assertion failed: expected
  value …"` report on the recording `TestReporter` (Go) means `pass:false`. It is
  not a runner error.
- A matcher *misused* — a length check on a value with no length, a comparison on
  non-numbers, an unknown matcher — is `ok:false` on both sides. The harness
  compares only *that* both refused, never the message text.
- Message-text quality is therefore explicitly out of scope; see
  "Message differences" below.

Values are described by a small tagged spec so that both languages build the same
thing: `{"$":"nan"}`, `{"$":"negzero"}`, `{"$":"undefined"}`, `{"$":"map"}`,
`{"$":"set"}`, `{"$":"date"}`, `{"$":"regexp"}`, `{"$":"sparse"}`,
`{"$":"cyclic"}`, `{"$":"throws"}`, `{"$":"mock"}`, and one tag per asymmetric
matcher. Plain JSON numbers decode to `float64` in Go, because every JavaScript
number is a float64.

## How the upstream inventory was derived

Mechanically, from the installed package — not from the docs. `node/enum.cjs`:

```
cd parity/jest/node && node enum.cjs
```

which is:

```js
const {expect} = require('expect');
Object.keys(expect(0)).sort()      // 46 matchers + modifiers on a matcher object
Object.keys(expect).sort()         // 15 statics
Object.keys(expect.not).sort()     // 5 negated asymmetric constructors
```

That yields **65 distinct symbols** (46 on `expect(0)`, 14 statics excluding
`expect.not` itself, and the 5 members of `expect.not`), every one of which
appears in the table below.

## Inventory

`expect().not.<m>` cases are folded into the row for `<m>`; the `expect().not`
row records the modifier itself.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `expect().lastCalledWith` | `jest.Matcher.LastCalledWith` | match | 1 (1 compared) |  |
| `expect().lastReturnedWith` | — | missing | — | no such alias; `ToHaveLastReturnedWith` exists — case `lastreturnedwith-alias-missing` |
| `expect().not` | `jest.Matcher.Not` | match | — | 25 negated cases spread across every group |
| `expect().nthCalledWith` | `jest.Matcher.NthCalledWith` | match | 1 (1 compared) |  |
| `expect().nthReturnedWith` | — | missing | — | no such alias; `ToHaveNthReturnedWith` exists — case `nthreturnedwith-alias-missing` |
| `expect().rejects` | — | missing | — | no promise modifier in the port |
| `expect().resolves` | — | missing | — | no promise modifier in the port |
| `expect().toBe` | `jest.Matcher.ToBe` | differs | 18 (17 compared) | disagrees on: `tobe-arr-structural`, `tobe-nan-vs-nan`, `tobe-obj-structural`, `tobe-zero-vs-negzero`; 1 deviation case(s) |
| `expect().toBeCalled` | `jest.Matcher.ToBeCalled` | match | 1 (1 compared) |  |
| `expect().toBeCalledTimes` | `jest.Matcher.ToBeCalledTimes` | match | 1 (1 compared) |  |
| `expect().toBeCalledWith` | `jest.Matcher.ToBeCalledWith` | match | 1 (1 compared) |  |
| `expect().toBeCloseTo` | `jest.Matcher.ToBeCloseTo` | differs | 13 (12 compared) | disagrees on: `closeto-inf-both`; 1 deviation case(s) |
| `expect().toBeDefined` | `jest.Matcher.ToBeDefined` | match | 6 (5 compared) | 1 deviation case(s) |
| `expect().toBeFalsy` | `jest.Matcher.ToBeFalsy` | match | 10 (10 compared) |  |
| `expect().toBeGreaterThan` | `jest.Matcher.ToBeGreaterThan` | match | 10 (10 compared) |  |
| `expect().toBeGreaterThanOrEqual` | `jest.Matcher.ToBeGreaterThanOrEqual` | match | 4 (4 compared) |  |
| `expect().toBeInstanceOf` | `jest.Matcher.ToBeInstanceOf` | differs | 16 (14 compared) | disagrees on: `instanceof-primitive-number`, `instanceof-primitive-string`; 2 deviation case(s) |
| `expect().toBeLessThan` | `jest.Matcher.ToBeLessThan` | match | 6 (6 compared) |  |
| `expect().toBeLessThanOrEqual` | `jest.Matcher.ToBeLessThanOrEqual` | match | 4 (4 compared) |  |
| `expect().toBeNaN` | `jest.Matcher.ToBeNaN` | match | 4 (4 compared) |  |
| `expect().toBeNull` | `jest.Matcher.ToBeNull` | match | 6 (5 compared) | 1 deviation case(s) |
| `expect().toBeTruthy` | `jest.Matcher.ToBeTruthy` | match | 14 (14 compared) |  |
| `expect().toBeUndefined` | `jest.Matcher.ToBeUndefined` | match | 4 (3 compared) | 1 deviation case(s) |
| `expect().toContain` | `jest.Matcher.ToContain` | differs | 16 (15 compared) | disagrees on: `contain-arr-reference`; 1 deviation case(s) |
| `expect().toContainEqual` | `jest.Matcher.ToContainEqual` | differs | 8 (8 compared) | disagrees on: `containequal-nan`, `containequal-undefined-prop` |
| `expect().toEqual` | `jest.Matcher.ToEqual` | differs | 34 (32 compared) | disagrees on: `toequal-absent-prop-vs-undefined`, `toequal-cyclic-diff`, `toequal-cyclic-same`, `toequal-nan-in-array`, `toequal-nan-vs-nan`, `toequal-undefined-prop-ignored`, `toequal-zero-vs-negzero`; 2 deviation case(s) |
| `expect().toHaveBeenCalled` | `jest.Matcher.ToHaveBeenCalled` | match | 4 (4 compared) |  |
| `expect().toHaveBeenCalledTimes` | `jest.Matcher.ToHaveBeenCalledTimes` | match | 3 (3 compared) |  |
| `expect().toHaveBeenCalledWith` | `jest.Matcher.ToHaveBeenCalledWith` | match | 8 (8 compared) |  |
| `expect().toHaveBeenLastCalledWith` | `jest.Matcher.ToHaveBeenLastCalledWith` | match | 3 (3 compared) |  |
| `expect().toHaveBeenNthCalledWith` | `jest.Matcher.ToHaveBeenNthCalledWith` | match | 4 (4 compared) |  |
| `expect().toHaveLastReturnedWith` | `jest.Matcher.ToHaveLastReturnedWith` | match | 2 (2 compared) |  |
| `expect().toHaveLength` | `jest.Matcher.ToHaveLength` | differs | 11 (11 compared) | disagrees on: `havelength-map`, `havelength-obj`, `havelength-str-astral`, `havelength-str-unicode` |
| `expect().toHaveNthReturnedWith` | `jest.Matcher.ToHaveNthReturnedWith` | match | 2 (2 compared) |  |
| `expect().toHaveProperty` | `jest.Matcher.ToHaveProperty` | match | 14 (14 compared) |  |
| `expect().toHaveReturned` | `jest.Matcher.ToHaveReturned` | match | 3 (3 compared) |  |
| `expect().toHaveReturnedTimes` | `jest.Matcher.ToHaveReturnedTimes` | match | 2 (2 compared) |  |
| `expect().toHaveReturnedWith` | `jest.Matcher.ToHaveReturnedWith` | differs | 4 (4 compared) | disagrees on: `returnedwith-undefined` |
| `expect().toMatch` | `jest.Matcher.ToMatch` | differs | 15 (14 compared) | disagrees on: `match-non-string-actual`, `match-string-invalid-regexp`, `match-string-metachars-only-regexp-would-match`; 1 deviation case(s) |
| `expect().toMatchObject` | `jest.Matcher.ToMatchObject` | match | 11 (11 compared) |  |
| `expect().toReturn` | `jest.Matcher.ToReturn` | match | 1 (1 compared) |  |
| `expect().toReturnTimes` | — | missing | — | no such alias; `ToHaveReturnedTimes` exists — case `returntimes-alias-missing` |
| `expect().toReturnWith` | `jest.Matcher.ToReturnWith` | match | 1 (1 compared) |  |
| `expect().toStrictEqual` | `jest.Matcher.ToStrictEqual` | differs | 14 (12 compared) | disagrees on: `tostrict-cyclic-same`, `tostrict-nan-vs-nan`, `tostrict-zero-vs-negzero`; 2 deviation case(s) |
| `expect().toThrow` | `jest.Matcher.ToThrow` | match | 22 (14 compared) | 8 deviation case(s) |
| `expect().toThrowError` | `jest.Matcher.ToThrow` | match | 3 (3 compared) |  |
| `expect.addEqualityTesters` | — | missing | — | no custom-equality-tester registry |
| `expect.any` | `jest.Any` | differs | 14 (13 compared) | disagrees on: `any-null-actual`, `any-object-on-array`; 1 deviation case(s) |
| `expect.anything` | `jest.Anything` | match | 6 (6 compared) |  |
| `expect.arrayContaining` | `jest.ArrayContaining` | match | 7 (7 compared) |  |
| `expect.assertions` | `jest.Assertions` | untested | — | assertion counting; needs a test lifecycle, not a matcher call |
| `expect.closeTo` | `jest.CloseTo` | match | 7 (7 compared) |  |
| `expect.extend` | `jest.Extend` | untested | — | custom matchers; no pass/fail case written |
| `expect.extractExpectedAssertionsErrors` | — | missing | — | internal to expect |
| `expect.getState` | — | missing | — | matcher state is not exposed by the port |
| `expect.hasAssertions` | `jest.HasAssertions` | untested | — | assertion counting; needs a test lifecycle |
| `expect.objectContaining` | `jest.ObjectContaining` | differs | 7 (7 compared) | disagrees on: `objectcontaining-on-array` |
| `expect.setState` | — | missing | — | matcher state is not exposed by the port |
| `expect.stringContaining` | `jest.StringContaining` | match | 5 (5 compared) |  |
| `expect.stringMatching` | `jest.StringMatching` | match | 5 (5 compared) |  |
| `expect.not.arrayContaining` | `jest.NotArrayContaining` | match | 2 (2 compared) |  |
| `expect.not.closeTo` | — | missing | — | no `NotCloseTo`, though the other four `Not*` constructors exist — cases `not-closeto-hit`, `not-closeto-miss` |
| `expect.not.objectContaining` | `jest.NotObjectContaining` | match | 2 (2 compared) |  |
| `expect.not.stringContaining` | `jest.NotStringContaining` | match | 2 (2 compared) |  |
| `expect.not.stringMatching` | `jest.NotStringMatching` | match | 2 (2 compared) |  |

## Counts

| status | symbols |
| --- | --- |
| `match` | 40 |
| `differs` | 12 |
| `missing` | 10 |
| `untested` | 3 |
| `extra` (Go-only, not part of the 65) | 7 (listed below) |
| **upstream total** | **65** |

**Parity over the symbols actually compared: 40 / 52 = 76.9 %.**
(`missing` and `untested` symbols are excluded from that ratio, as they were never
compared. Including them as failures gives 40 / 65 = 61.5 %.)

**Case totals** (from `parity.json`): 369 cases, 342 compared, **311 match, 31
mismatch → 90.9 % case-level parity**, plus 27 deviation cases counted separately
(4 of which happen to agree anyway).

Per group:

| group | cases | compared | match | mismatch | deviations |
| --- | --- | --- | --- | --- | --- |
| asymmetric | 61 | 58 | 55 | 3 | 3 |
| collections | 54 | 53 | 46 | 7 | 1 |
| equality | 66 | 61 | 47 | 14 | 5 |
| errors | 25 | 17 | 17 | 0 | 8 |
| instanceof | 16 | 14 | 12 | 2 | 2 |
| mocks | 45 | 42 | 41 | 1 | 3 |
| numbers | 37 | 36 | 35 | 1 | 1 |
| strings | 21 | 20 | 17 | 3 | 1 |
| truthiness | 44 | 41 | 41 | 0 | 3 |

## Every disagreement (these are real bugs in the port)

Ordered by matcher. "up" is `expect@29.7.0`, "go" is the port.

### `toBe`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `tobe-nan-vs-nan` | pass | fail | `toBe` is specified as `Object.is`, under which `NaN` *is* `NaN`. The port uses Go `==`, so `NaN != NaN`. |
| `tobe-zero-vs-negzero` | fail | pass | `Object.is(0, -0)` is false. Go's `==` says `0 == -0`. |
| `tobe-obj-structural` | fail | pass | `toBe` is reference identity for objects. `shallowEqual` falls back to `reflect.DeepEqual` for non-comparable values, so two distinct maps compare equal. |
| `tobe-arr-structural` | fail | pass | Same cause, for slices. |

The last two are arguably unavoidable in Go for map/slice actuals (there is no
usable reference identity for them at the `any` level), but `NaN` and `-0` are
straightforwardly fixable by routing `ToBe` through `Object.is` semantics.

### `toEqual`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `toequal-nan-vs-nan` | pass | fail | jest's structural equality treats `NaN` as equal to `NaN`. `asymEqual`/`DeepEqual` does not. |
| `toequal-nan-in-array` | pass | fail | Same, one level down. |
| `toequal-zero-vs-negzero` | fail | pass | jest's `toEqual` *does* distinguish `0` from `-0`; Go's `==` does not. Note this is the opposite direction from `toBe`, so one fix cannot serve both. |
| `toequal-undefined-prop-ignored` | pass | fail | `{a:1, b:undefined}` `toEqual` `{a:1}`. The port sees a present key with a `nil` value and reports inequality. |
| `toequal-absent-prop-vs-undefined` | pass | fail | The same case with the sides swapped. |
| `toequal-cyclic-same` | pass | **crash** | The port recurses without bound on a self-referential structure. |
| `toequal-cyclic-diff` | fail | **crash** | Same, and the port never gets far enough to report the inequality. |

The cyclic cases are the most serious finding: `jest.Expect(t, cyclic).ToEqual(...)`
**fatally stack-overflows**. It is not a recoverable panic — the overflow happens
inside `fmt` while building the failure message (`format(expected)`), so
`recover()` cannot catch it and the whole process dies. The harness detects the
dead runner, records the case as a crash, and restarts the runner
(`runnerRestarts: {"go": 3}` in `parity.json`).

### `toStrictEqual`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `tostrict-nan-vs-nan` | pass | fail | Same `NaN` cause as `toEqual`. |
| `tostrict-zero-vs-negzero` | fail | pass | Same `-0` cause. |
| `tostrict-cyclic-same` | pass | **crash** | Same unbounded recursion. |

### `toContain` / `toContainEqual`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `contain-arr-reference` | fail | pass | `toContain` is reference/`Object.is` membership; the port's `contains` uses `reflect.DeepEqual`, so `[{a:1}]` "contains" a structurally equal object. `toContainEqual` is the matcher that *should* do that. |
| `containequal-nan` | pass | fail | `NaN` again. |
| `containequal-undefined-prop` | pass | fail | The undefined-property rule again, inside an element. |

### `toHaveLength`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `havelength-str-unicode` | pass (1) | fail | `"é"` has `.length === 1` in JS (UTF-16 code units); Go's `len` is 2 bytes. |
| `havelength-str-astral` | pass (2) | fail | `"😀"` is 2 UTF-16 code units, 4 Go bytes. |
| `havelength-map` | usage error | pass | A JS `Map` has no `.length`, so upstream refuses; Go maps do have a length, so the port answers. |
| `havelength-obj` | usage error | pass | Same for a plain object. |

The two string cases are a real correctness bug for any non-ASCII assertion: the
port should count runes (still not identical to UTF-16 units for astral
characters, but far closer than bytes).

### `toMatch`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `match-string-metachars-only-regexp-would-match` | fail | pass | `toMatch("a.c")` against `"abc"`: JS does a *literal substring* check for a string argument. The port always compiles the argument as a regexp. |
| `match-string-invalid-regexp` | pass | usage error | `toMatch("a(b")` against `"a(b"` is a valid substring check upstream; the port fails to compile the pattern. |
| `match-non-string-actual` | usage error | pass | Upstream requires a string receiver; the port stringifies anything with `%v`. |

Fix shape: `ToMatch` needs the JS two-argument-kinds split (literal substring for
a plain string, pattern for an explicit regexp).

### `toBeInstanceOf`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `instanceof-primitive-string` | fail | pass | A primitive string is not `instanceof String` in JS. Go has no primitive/wrapper distinction. |
| `instanceof-primitive-number` | fail | pass | Same. |

### `toBeCloseTo`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `closeto-inf-both` | pass | fail | jest short-circuits `Infinity === Infinity` before the epsilon test. The port computes `Inf - Inf = NaN`, and `NaN <= eps` is false. |

### `toHaveReturnedWith`

| case | up | go | what it means |
| --- | --- | --- | --- |
| `returnedwith-undefined` | pass | fail | A `jest.fn()` with no configured return value returns `undefined`, and `toHaveReturnedWith(undefined)` passes. The port's `Mock.Call` records *no* result at all for such a call, and `ToHaveReturnedWith` skips calls with `len(Results) == 0`. |

### Asymmetric matchers

| case | up | go | what it means |
| --- | --- | --- | --- |
| `any-null-actual` | pass | fail | `expect(null).toEqual(expect.any(Object))` passes upstream (`null` is special-cased through `instanceof`-adjacent logic in `expect.any`'s `asymmetricMatch`). `jest.Any(sample)` rejects `nil` outright. |
| `any-object-on-array` | pass | fail | Every array is `instanceof Object`. `[]any` is not assignable to `map[string]any`. |
| `objectcontaining-on-array` | pass | fail | `expect.objectContaining({"0": 1})` matches `[1]` in JS because array indices are object keys. Go slices have no string-keyed properties. |

## Matchers the port lacks

| upstream | note |
| --- | --- |
| `expect().resolves`, `expect().rejects` | no promise modifiers; the port has no async assertion path at all |
| `expect().toReturnTimes` | alias missing (`ToHaveReturnedTimes` exists) |
| `expect().lastReturnedWith` | alias missing (`ToHaveLastReturnedWith` exists) |
| `expect().nthReturnedWith` | alias missing (`ToHaveNthReturnedWith` exists) |
| `expect.not.closeTo` | the only one of the five `expect.not.*` constructors with no `Not*` counterpart |
| `expect.addEqualityTesters` | no pluggable equality testers |
| `expect.getState`, `expect.setState`, `expect.extractExpectedAssertionsErrors` | matcher state is not exposed |

`toThrow`'s regexp, error-class and error-instance argument forms are also absent:
`Matcher.ToThrow(msg ...string)` accepts only an optional substring. Those cases
(`throw-regexp-*`, `throw-class-*`, `throw-instance-*`) are marked as deviations.

## The type gap — pairs that are not expressible in Go

These are marked `"deviation"` in the case files and are counted separately from
mismatches. They are *not* bugs; they are the cost of porting a dynamically typed
API to a generic one.

| what | cases | why |
| --- | --- | --- |
| `null` vs `undefined` | `tobe-null-vs-undefined`, `tostrict-null-vs-undefined`, `null-undefined`, `undefined-null`, `defined-null` | Go has exactly one `nil`. `ToBeNull`, `ToBeUndefined` and `ToBeDefined` are all `isNil` in the port, so the three matchers are mutually indistinguishable where JS distinguishes them. `defined-null` is the sharpest: in JS `null` *is* defined. |
| array holes | `toequal-sparse-vs-dense`, `tostrict-sparse-vs-dense` | a Go slice has no notion of a hole; the harness decodes a hole as a `nil` element, which is JS's `undefined`, which is exactly the distinction `toStrictEqual` exists to make |
| `Set` | `toequal-set-vs-array`, `contain-set-hit` | no Go `Set`; modelled as `map[any]struct{}` |
| mixed-type containers | not attempted | `Expect[T]` with `T = any` is the only way to hold `[1, "a", null]`, so the harness always uses `jest.Expect[any]`. A more natural `jest.Expect[[]int]` cannot express the JS pairs at all. This is the "`Expect[T]` forces `jest.Expect[any]`" issue: **confirmed** — every case in this harness goes through `Expect[any]`, and none of the matchers taking `any` arguments (`ToContain`, `ToMatchObject`, `ToHaveProperty`, the mock matchers) would type-check usefully otherwise. |
| `ToBeInstanceOf` cannot take an interface | `instanceof-typeerror-as-error`, `instanceof-array-as-object`, `any-error-on-typeerror` | **confirmed.** `ToBeInstanceOf(target any)` derives `reflect.TypeOf(target)` from a *sample value*, so there is no idiomatic way to say "an `error`" — you would have to reach for `reflect.TypeOf((*error)(nil)).Elem()`. Every JS subclass relation (`TypeError instanceof Error`, `Array instanceof Object`) therefore fails. |
| ordering matchers on non-ordered types | `gt-strings`, `lt-strings` | **confirmed.** `ToBeGreaterThan(n T)` with `T = string` *compiles*; it only fails at run time, via `compare`'s `toFloat` check, with `"ToBeGreaterThan requires numeric values"`. Upstream refuses too (`ensureNumbers`), so these two cases *agree* — the deviation is in the API's type safety, not in the answer. |
| `toBeCloseTo` argument shape | `closeto-default-loose` | JS takes a digit count (default 2 → epsilon 0.005); the port takes an absolute epsilon (default `1e-9`). For the explicit-precision cases the harness converts `p` to `10^-p / 2` so the *semantics* can be compared; the default-argument case shows the divergence directly. |
| `toMatch` regexp flags | `match-regexp-ignorecase` | `ToMatch(pattern string)` has no flags parameter, so `/hello/i` cannot be expressed (short of the caller writing `(?i)`). |

## Message differences (not compared, recorded here)

- Upstream produces a multi-line, coloured diff with `Expected:` / `Received:`
  blocks and matcher-specific hints. The port produces one line of the form
  `assertion failed: expected value to deeply equal …, but got …`, optionally
  followed by its own small `diff`.
- Upstream distinguishes an assertion failure (`JestAssertionError`, carrying
  `matcherResult`) from a misuse (`Error` with a `Matcher error:` preamble) by
  *type*. The port sends both through `TestReporter.Errorf`, and they can only be
  told apart by the message prefix (`"assertion failed: expected value"` vs
  `"assertion failed: <Matcher> requires"`). The Go runner relies on exactly that
  prefix; a wording change in the port would silently reclassify cases, which is
  worth a typed error in the port.
- Negated failures upstream read `expect(received).not.toBe(expected)`; the port
  reads `expected value NOT to be …`.

## Extra Go symbols (no upstream equivalent in `expect@29.7.0`)

| Go symbol | status | note |
| --- | --- | --- |
| `jest.Matcher.ToHaveLen` | extra | Go-spelled alias of `ToHaveLength` |
| `jest.Matcher.ToBeTrue` / `ToBeFalse` | extra | stricter than `ToBeTruthy`/`ToBeFalsy`: require an actual `bool` |
| `jest.Matcher.ToBeNil` | extra | Go spelling alongside `ToBeNull` |
| `jest.Matcher.ToPanic` | extra | Go-native spelling of `ToThrow` |
| `jest.Matcher.ToBeOneOf` | extra | exists in jest 30, not in `expect@29.7.0` |
| `jest.Matcher.To(name, args…)` | extra | dispatch for `Extend`-registered custom matchers |
| `jest.NotArrayContaining` / `NotObjectContaining` / `NotStringContaining` / `NotStringMatching` | extra as functions | upstream reaches these through the `expect.not` namespace rather than distinct constructors; scored above against `expect.not.*` |

## Reproducing

```
cd parity/jest/node && npm install          # pins expect@29.7.0, jest-mock@29.7.0
cd .. && GOWORK=off go test ./...           # rewrites parity.json
```

`go test` skips (never fails) when `node` is not on `PATH` or
`node/node_modules/expect` is absent.
