# jest example

Runnable programs that exercise [`github.com/malcolmston/jest`](https://github.com/malcolmston/jest)
— a Jest-style assertion/mocking layer over Go's `testing` package — as an
outside consumer would: the dependency is a **published module**, there is no
`replace` directive.

Resolved module version: **`github.com/malcolmston/jest v0.0.0-20260719012634-e4d6de7846e6`**
(pseudo-version; the repo has no semver tags).

## Layout

* `main.go` — everything that works **without** a `*testing.T`. `jest.Expect`
  accepts any `jest.TestReporter`, so `main.go` plugs in a small recording
  reporter and prints `PASS` / `FAIL` per assertion. Mocks, spies, `SpyOn`, fake
  timers, asymmetric matchers and `Extend` custom matchers need no test at all.
* `main_test.go` — the parts that genuinely require `*testing.T`:
  `Describe`/`It`/`Test`, `BeforeEach`/`AfterEach`/`BeforeAll`/`AfterAll`,
  `ItSkip`/`ItTodo`, `Each`/`DescribeEach`, `Assertions`/`HasAssertions`, and the
  snapshot matchers.
* `__snapshots__/snapshots.snap` — committed snapshot store produced by
  `ToMatchSnapshot` / `ToThrowMatchingSnapshot`.

## Run

```sh
cd examples/jest
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .            # 12 labelled sections, terminates on its own
GOWORK=off go test -v ./...    # Describe/It/Each/snapshots
```

Refresh snapshots with `JEST_UPDATE_SNAPSHOTS=1 GOWORK=off go test ./...`.

## What it demonstrates

* **Core matchers**: `ToBe`, `ToEqual`, `ToBeNil`, `ToBeTrue/False`,
  `ToBeGreaterThan(OrEqual)`, `ToBeLessThan(OrEqual)`, `ToContain`,
  `ToHaveLen`/`ToHaveLength`, `ToMatch`, `ToBeCloseTo`, `ToBeTruthy/Falsy`,
  `ToBeDefined/Undefined/Null`, `ToBeNaN`, `ToBeOneOf`, `ToContainEqual`,
  `ToBeInstanceOf`.
* **Structural matchers**: `ToStrictEqual`, `ToMatchObject` (struct and nested
  map subsets), `ToHaveProperty` with dotted + indexed paths.
* **Panics**: `ToPanic` / `ToThrow` with message matching, and negated.
* **Negation**: `.Not()` across value, mock and custom matchers.
* **Asymmetric matchers**: `Any`, `Anything`, `StringContaining`,
  `StringMatching`, `ArrayContaining`, `ObjectContaining`, `CloseTo` and the
  `Not*` variants, nested inside `ToEqual` and `ToHaveProperty`.
* **Custom matchers**: `Extend` + `To(name, args...)`, including with arguments
  and negated; unknown names are reported as failures rather than panicking.
* **Mocks**: `NewMock`, `Return`, `ReturnValues`, `MockImplementation(Once)`,
  `MockReturnValueOnce`, `MockResolvedValue`, `MockRejectedValue`, `Call`,
  `CallCount`, `Called`, `CalledWith`, `Calls`, `LastCall`, `NthCall`,
  `Results`, `MockClear`, `MockReset`, `MockName`, `Reset`, `ClearAllMocks`,
  `ResetAllMocks`.
* **Typed helpers**: `Fn0`/`Fn1`/`Fn2`, `Spy0`/`Spy1`/`Spy2`.
* **`SpyOn`**: replaces a func-typed struct field in place, delegates to the
  original, can be stubbed, then `Restore()` / `RestoreAllMocks()`.
* **Mock matchers**: `ToHaveBeenCalled(Times|With)`, `ToHaveBeenNthCalledWith`,
  `ToHaveBeenLastCalledWith`, `ToHaveReturned(Times|With)`,
  `ToHaveLastReturnedWith`, `ToHaveNthReturnedWith`, plus the legacy aliases
  `ToBeCalled*`, `LastCalledWith`, `NthCalledWith`, `ToReturn`, `ToReturnWith`.
* **Fake timers**: `NewClock`, `NewClockAt`, `SetTimeout`, `SetInterval`,
  `ClearTimer`, `ClearAllTimers`, `After`, `PendingCount`, `GetTimerCount`,
  `AdvanceTimersByTime`, `AdvanceTimersToNextTimer`, `RunAllTimers`,
  `RunOnlyPendingTimers`, `SetSystemTime`, `Now`.
* **Failure output**: section 12 deliberately fails five assertions against the
  recording reporter and prints the messages (including `ToEqual`'s diff).

Everything above passes. One line is commented out with a `// HOLE:` marker
(see below).

## Holes / friction found

* **`ToBeInstanceOf` cannot be given an interface the Go way.**
  `ToBeInstanceOf((*error)(nil))` fails: the implementation calls
  `reflect.TypeOf(target)`, which yields `*error` (a pointer), not the interface.
  You must write `reflect.TypeOf((*error)(nil)).Elem()`. The README advertises an
  "interface-implementation check" without saying this, and neither the README
  nor the library's own tests show an interface example. The naive form is left
  commented out in `main.go`.
* **The generic `Expect[T]` fights the Jest API.** `Expect[T]` makes
  `ToBe(expected T)` / `ToEqual(expected T)` type-safe, but every matcher whose
  expected side is heterogeneous — `ToMatchObject`, `ToHaveProperty`,
  `ToBeTruthy/Falsy`, `ToBeDefined/Undefined/Null`, `ToBeInstanceOf`, and any
  use of an asymmetric matcher inside `ToEqual` — forces you to write
  `jest.Expect[any](t, x)` explicitly, because `T` is inferred from the actual
  value. So the same package requires two different call styles and the type
  parameter has to be spelled out by hand exactly where inference would be most
  welcome.
* **Ordering matchers are unconstrained.** `ToBeGreaterThan(n T)` etc. are
  methods on `Matcher[T any]`, so comparing strings or structs compiles fine and
  only fails at runtime. `cmp.Ordered` isn't usable here because the type
  parameter is shared with the non-ordered matchers.
* **`Assertions` / `HasAssertions` take an unexported interface.** They accept
  `cleanuper` (`Helper`, `Cleanup`, `Errorf`), not the documented public
  `TestReporter`. A custom reporter cannot be used with them, and because the
  interface is unexported you cannot name the parameter type in your own helper
  functions. Also, if the argument does not additionally satisfy `TestReporter`,
  `registerCounter` silently does nothing.
* **`ToMatchInlineSnapshot` is hand-maintenance only.** Unlike Jest it never
  writes the literal back into the source, and the serializer emits a **trailing
  comma after the last map entry** (`{\n  "a": 1,\n  "b": 2,\n}`), which is not
  JSON and is not documented — you have to run the assertion once, read the
  failure output, and paste it. `JEST_UPDATE_SNAPSHOTS` only affects on-disk
  snapshots.
* **Typed mock helpers stop at 2 arguments / 1 return value.** There is no
  `Fn3`+/`Spy3`+, and no typed helper for a function returning `(T, error)` —
  the single most common Go signature. Anything else must go through the untyped
  `Mock.Call(args ...any) []any`, losing all type safety (the `SpyOn` example in
  this repo has to write `[]any{"stubbed", nil}` by hand).
* **`SpyOn` only handles func *values*.** It panics unless given a pointer to a
  func variable or func-typed field; there is no way to spy on a method of a
  concrete type, which is how most Go code is structured.
* **`ReturnValues` repeats the last set forever** once the sequence is
  exhausted (4th call to a 3-element sequence returns the 3rd value). Jest's
  `mockReturnValueOnce` chain falls back to the default/`undefined` instead. Not
  documented either way.
* **`MockResolvedValue`/`MockRejectedValue` are promise vocabulary with no Go
  meaning** — they just make the mock return `[value, nil]` / `[nil, err]`.
  Harmless, but the names mislead: nothing async happens.
* **No mutex-free-ness guarantees documented for `Clock`**, and `Clock.After`
  only delivers when you advance the clock — fine, but if a test forgets to
  advance, a `<-ch` receive deadlocks with no timeout or diagnostic.
* **`ItOnly` focus is per-`Describe` block only** (as the README says), which
  means a top-level `ItOnly` does not focus the file the way Jest's `it.only`
  does. Not exercised in the example because it would suppress sibling cases.
