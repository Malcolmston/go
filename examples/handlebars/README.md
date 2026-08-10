# handlebars example

A runnable program that exercises [`github.com/malcolmston/handlebars`](https://github.com/malcolmston/handlebars)
— a dependency-free Handlebars/Mustache templating engine for Go — as an
outside consumer would: the dependency is a **published module**, there is no
`replace` directive.

Resolved module version: **`github.com/malcolmston/handlebars v0.0.0-20260719133134-9a6c0576bd42`**
(pseudo-version; the repo has no semver tags).

## Run

```sh
cd examples/handlebars
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints ten labelled sections and terminates on its own.

## What it demonstrates

1. **Compilation & rendering** — `Render` (one-shot), `Parse`, `MustParse`,
   `ParseString`, `Render`/`MustRender`, path expressions (`a.b.c`, `items.0`,
   `../parent`, `@root`, bracketed `[odd key]`), slice `.length`.
2. **Escaping** — `{{x}}` (escaped) vs `{{{x}}}` / `{{& x}}` (raw), the exported
   `EscapeHTML` / `EscapeExpression` helpers, `SafeString`, and both comment forms.
3. **Conditionals** — `{{#if}}` / `{{else if}}` / `{{else}}`, `{{#unless}}`,
   inverse sections `{{^x}}`, and the built-in comparison helpers
   (`eq ne not and or gt lt gte lte`) used inside subexpressions.
4. **Loops** — `{{#each}}` over slices *and* maps (sorted keys), `@index`,
   `@key`, `@first`, `@last`, `this`, nested `@../index`, block parameters
   (`as |item i|`), `{{#with x as |p|}}`, and the `lookup` helper.
5. **Partials** — `RegisterPartial`/`RegisterPartials`, `{{> name}}`, explicit
   context `{{> name ctx}}`, hash overlays `{{> name k=v}}`, dynamic
   `{{> (helper)}}`, partial blocks `{{#> layout}}…{{/layout}}` with
   `{{> @partial-block}}` and its fallback body, plus `HasPartial`/`PartialNames`.
6. **Custom helpers** — inline helpers, `SafeString` to opt out of escaping,
   hash arguments via `HashStr`, a block helper using `Fn`/`Inverse`/
   `FnWithBlockParams`, the `helperMissing` hook, `UnregisterHelper`, `Clone`.
7. **Decorators & inline partials** — `{{#*inline "n"}}…{{/inline}}` and a
   custom decorator using `DecoratorOptions.Program` + `RegisterPartial`.
8. **Compile options** — `NoEscape`, `Strict`, `NoData`, `KnownHelpers`,
   `KnownHelpersOnly`, `SetLogger` with the `log` helper.
9. **Exported utilities** — `Stringify`, `IsEmpty`, `IsArray`, `Extend`,
   `CreateFrame`.
10. **Error handling** — parse errors (unclosed block, stray `{{/if}}`,
    unterminated mustache), `MustParse` panic, partial parse errors.

Everything above works. Nothing had to be commented out.

## Holes / friction found

* **Helpers cannot return an error.** `type Helper func(*Options) interface{}`
  and `Options.Fn`/`Inverse` return a bare `string`. A custom helper that hits a
  real failure (bad argument type, failed lookup) has no way to abort rendering
  with an error — its only options are to return a string or `panic`, and a
  panic is not recovered into the `Render` error. `Render` returning
  `(string, error)` therefore only ever surfaces *engine* errors, never helper
  errors. This is the single biggest API gap.
* **Undocumented `.length`.** `{{items.length}}` resolves to `reflect.Len` for
  slices/arrays/maps (see `lookupIndex`), but the README's path list does not
  mention it, so it looks unsupported.
* **Structs stringify with Go syntax.** `{{lookup items 2}}` on a struct prints
  `{Cable 7.25 [] 12}` (Go `%v`). Handlebars.js prints `[object Object]`. Not
  wrong, just a parity difference worth knowing if you port templates.
* **`interface{}` instead of `any`** throughout the public API (`Helper`,
  `Options.Args`, `Render(data interface{})`) on a `go 1.24.7` module. Cosmetic
  but dated.
* **`Options.Hash` accessors are thin.** Only `HashStr(key, def)` exists; there
  is no `HashBool`/`HashInt`/typed accessor, so numeric or boolean hash
  arguments must be dug out of the raw `map[string]interface{}` and type-asserted
  by hand.
* **`Template` mutation vs. rendering is unsynchronised.** `RegisterHelper`,
  `RegisterPartial` etc. write to plain maps with no mutex and no documented
  concurrency contract, while `Render` reads them. Registering a helper *after*
  `ParseString` works (resolution happens at render time — the example relies on
  this), but concurrent register + render is a data race and the docs are silent.
* **No `KnownHelpers` misuse feedback.** `Compile(src, KnownHelpersOnly())`
  without `KnownHelpers(...)` fails only at *render* time
  (`missing helper "x"`), not at compile time, which is where Handlebars.js
  reports it.
