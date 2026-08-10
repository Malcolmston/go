# examples/jq

A runnable program that exercises the **published** `github.com/malcolmston/jq`
module — the standard-library-only port of the jq JSON query language — against
an in-memory JSON document.

## Module version

The example consumes the module exactly as an outside user would: there is no
`replace` directive and no reference to the local working tree. The dependency
was added with `GOWORK=off go get github.com/malcolmston/jq@latest`, which
resolved to the pseudo-version

```
github.com/malcolmston/jq v0.0.0-20260725030912-6bc45a301980
```

The repository has no semver tags, hence the pseudo-version. Everything below
describes **that** published revision; the local `../../jq` working tree is
newer and behaves differently in places (for example it has `limits.go`,
`check.go` and `@base32`, none of which exist in the published module).

## Run

```sh
cd examples/jq
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program terminates on its own and prints twelve labelled sections.

## What it demonstrates

| Section | Feature |
| --- | --- |
| 1 | `jq.Unmarshal` / `jq.Marshal` round trip |
| 2 | Field access, nested fields, positive/negative indices, slices, optional `?`, `has` |
| 3 | `.[]` iteration, pipes, `select`, `map`, `map_values`, `to_entries`/`from_entries`/`with_entries`, object construction |
| 4 | Arithmetic (`+ - * / %`), object merge, array concat/subtract, `add`, `min`/`max`, `sqrt`/`floor`/`pow`, `reduce`, `foreach` |
| 5 | String builtins: `ascii_upcase`, `ltrimstr`/`rtrimstr`, `split`/`join`, `gsub`, interpolation, `test`/`capture`/`sub`, `@base64`/`@base64d`/`@csv`, `tostring`/`tonumber`, `explode`/`implode` |
| 6 | `sort`/`sort_by`, `group_by`, `unique`, `flatten`, `any`/`all`, `paths`, `getpath`, `del`, `recurse`, `walk`, `tostream` |
| 7 | `type`, `//`, `try`/`catch`, `error({...})`, `if/elif/else`, `label`/`break`, `limit`/`first`/`last`, `range`, `def` with and without arguments |
| 8 | `Query.WithVariables` binding `$name` values after compilation |
| 9 | `Query.RunFunc` streaming output and stopping early with a sentinel error |
| 10 | Error handling: `*CompileError` (offset + message), `*RuntimeError`, `RuntimeError.Value()`, undefined symbol, `WithMaxSteps` step budget, strict-JSON rejection of `NaN` |
| 11 | Number formatting: `nan`, `infinite`, float64 precision, `-0` |
| 12 | The parity gaps and holes below, each exercised so it is visible in the output |

## Holes found

Only one thing had to be commented out (`fromstream`, hole 1). Everything else
compiles and runs.

### 1. `fromstream/1` and `truncate_stream/2` are missing, but `tostream` is present

```
tostream/fromstream   RUNTIME ERROR: jq: syntax error at offset 10: fromstream/1 is not defined
```

The published `API-DEVIATIONS.md` does list the streaming builtins as not
implemented, so this is a documented gap — but the omission is asymmetric:
`tostream` works, so the canonical `fromstream(tostream)` idiom (and anything
built on `--stream`-style processing) cannot be expressed at all. The example
keeps this line commented out with a `// HOLE:` note.

### 2. `@base32` / `@base32d` are missing and *not* documented

```
@base32 missing: jq: error: @base32 is not a valid format
```

Upstream jq has both. The published `API-DEVIATIONS.md` "Not implemented" list
names only modules, the streaming builtins, and jq 1.8's value-depth limits —
it does not mention the base32 formats, and its general claim is that every
other builtin from upstream is present. `@base64`, `@base64d`, `@csv`, `@tsv`,
`@sh`, `@uri`, `@html` and `@json` all work.

### 3. `ErrUndefined` and `ErrBreak` are dead sentinels

`errors.go` declares and documents both, `doc.go` tells callers that "an escaped
break is `ErrBreak`", and `API-DEVIATIONS.md` lists them among the sentinels
added so that failures can be classified with `errors.Is`. Nothing in the
package ever wraps either one (`grep -rn ErrUndefined` in the module finds only
the declaration and the docs). The output shows both failing to match:

```
undefined at run: jq: syntax error at offset 0: no_such_builtin/0 is not defined
  (Is ErrCompile=true, Is ErrUndefined=false)
escaped break: jq: syntax error at offset 7: $*label-out is not defined (Is ErrBreak=false)
```

So there is no way to tell an undefined-symbol failure or an escaped `break`
apart from an ordinary syntax error other than by matching error strings.

### 4. An escaped `break` leaks a mangled internal symbol name

`def f: break $out; label $out | f` fails with
`$*label-out is not defined`. `$*label-out` is an internal mangling that no
caller wrote and cannot act on; upstream jq reports "break". The same shape
appears for `(label $out | .) | break $out`.

### 5. `Compile`'s documentation does not match `Compile`'s behaviour

The published doc comment says:

> Syntax errors **and references to undefined functions or variables** are
> reported as `*CompileError` […]

In practice `Compile` accepts all of these and the failure only appears from
`Run`:

```
Compile(`break $nope`) returned no error; failure is deferred to Run
Compile(`$undefinedVar`) ok; Run says: jq: syntax error at offset 0: $undefinedVar is not defined
```

(The library's own `README.md` acknowledges this in its upstream-parity table:
five `%%FAIL` programs that upstream rejects at compile time compile here.) The
newer local working tree rewrote this doc comment to describe the deferred
behaviour, so the published doc is simply stale.

### 6. Undefined-symbol errors always report `Offset: 0`

Related to 3–5: the `*CompileError` for an undefined name carries offset 0 (or
the offset of the enclosing definition), not the position of the offending
token, and renders as "syntax error". A caller cannot point at the bad name.

### 7. Documented deviations that change observable behaviour vs. real jq

All recorded in `API-DEVIATIONS.md`, and all visible in section 12 of the
output:

* **Key order is lost.** Values are `map[string]any`, so `keys_unsorted` is
  identical to `keys`: `{"b":1,"a":2} | [keys_unsorted, keys]` gives
  `[["a","b"],["a","b"]]` where real jq gives `[["b","a"],["a","b"]]`.
* **Numbers are `float64` only.** `9007199254740993` marshals back as
  `9007199254740992`; jq 1.7 preserves the literal text of untouched numbers.
* **Regexes are RE2.** `test("a(?=b)")` fails with
  `invalid or unsupported Perl syntax: (?=`; no lookaround, no backreferences.
* **No module system.** `import "foo" as f; .` is `unexpected keyword import`.
* **`input`/`inputs` are stubs.** `input` raises `No more inputs`, `[inputs]` is
  `[]`, so jq programs written around `inputs` cannot be ported as-is.
* **Strict JSON input.** `jq.Unmarshal([]byte(`{"x": NaN}`))` is rejected where
  jq's own parser accepts `NaN`/`nan`/`Infinity`.
* **No jq 1.8 value-depth limits** in this revision. JSON input nesting *is*
  bounded (`{20000-deep} -> "exceeded max depth"`), but the `Path too deep` /
  `Comparison too deep` family is absent; `API-DEVIATIONS.md` says so.

## What was easy

Builtin coverage was otherwise excellent, with real-jq-matching output first
try: `walk`, `with_entries`, `paths`, `del`, `recurse`, `group_by`, `unique`,
`sub` with named captures used in the replacement, `@sh`/`@uri`/`@html`/`@csv`,
`$ENV`/`env`, `IN`, `splits`, `strftime`/`fromdate`/`todate`, `$__loc__`,
`label`/`break` in the middle of a pipeline, and destructuring `def` arguments.
`try (1/0) catch` reproduces jq's own wording. The step budget
(`WithMaxSteps`) stops `def f: f; f` with `ErrStepLimit` instead of hanging or
overflowing the stack, and `RunFunc` returns a consumer's sentinel error
unchanged, which makes take-first-N straightforward. The module has zero
dependencies and downloads and builds cleanly.
