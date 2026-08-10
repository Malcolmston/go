# `jq` parity coverage

- **Upstream oracle**: the real `jq` binary, **jq-1.7.1** (`jq --version` →
  `jq-1.7.1`), driven per case by [`c/run.py`](c/run.py). `parity_test.go` refuses
  any other version: jq 1.8 changed several of the behaviours scored below
  (`trim`, `reverse` on strings, `limit` with a negative count, `have_decnum`), so
  an unpinned oracle would silently move the score.
- **Go port**: `github.com/malcolmston/jq v0.0.0-20260810111538-69e9503f8dce`, consumed as a published module
  (no `replace` directive; added with `GOWORK=off go get github.com/malcolmston/jq@latest`).
- **Harness**: `GOWORK=off go test ./parity/jq/` — **574** cases in `cases/*.json`.
  A case is a *(filter program, input JSON, optional `$variables`)* triple and the
  compared answer is the **complete output stream** of the filter, as a JSON array.
  jq filters yield zero, one or many values, so collapsing a many-valued stream to
  one value is scored as a mismatch rather than hidden.
- **Determinism**: both runners install the same fixed environment
  (`TZ=UTC LANG=C LC_ALL=C PARITY=jq`) before evaluating anything, so `env`,
  `$ENV`, `localtime`, `strflocaltime` and `mktime` are comparable. Unbounded
  generators (`repeat`, `recurse`, `range(_;_;0)`) are always capped with `limit`.

## How the upstream symbol list was produced

jq has no headers to reflect over usefully — its builtin surface is what the
interpreter itself reports — so the inventory is jq's own answer:

```sh
jq -n -c 'builtins | sort'                 # 218 symbols, name/arity
jq --version                               # jq-1.7.1
```

and the port's surface comes from the same program run through the port's runner:

```sh
printf '{"id":"b","fn":"run","args":["builtins|sort",null]}\n' | go run ./go
#   -> 210 symbols
```

Two caveats about that list, both mechanical rather than editorial:

1. jq's `builtins` does **not** include the `@format` strings, but the port's
   does. `@text @json @csv @tsv @sh @uri @html @base64 @base64d` therefore appear
   as `extra` in the table below even though jq implements every one of them; they
   are exercised by the `strings` cases and they all match. `@base32`/`@base32d`
   are rejected by every jq binary on this machine (1.7.1 and 1.8.1 alike:
   `base32 is not a valid format`) and by the port too, so `at-base32` and
   `at-base32d` match on a shared failure rather than on a shared value.
2. Language *syntax* — operators, `if`, `reduce`, destructuring, `label`/`break` —
   is not in `builtins` either, so it is inventoried in its own table.

## Divergences found, worst first

### Wrong values / collapsed output streams

1. **`repeat/1` iterates when jq does not** (`repeat-capped`, `repeat-plus`). jq
   1.7.1 *and* 1.8.1 re-apply `f` to the **original** input on every step, so
   `1 | [limit(4; repeat(.*2))]` is `[2,2,2,2]`. The port threads the value
   through, giving `[1,2,4,8]` — i.e. it implements `repeat` as `recurse`, which
   also wrongly emits the input itself first.
2. **`last(empty)` loses jq's `null`** (`last-empty`). jq yields a one-element
   stream `[null]`; the port yields an empty stream. This is the collapsed-stream
   failure mode the harness exists to catch.
3. **`break` is not visible to `try`/`catch`** (`try-catch-break`,
   `catch-does-not-catch-break`). In jq a `break` is an error value, so
   `label $l | try (break $l) catch "caught"` produces `"caught"`, and
   `[label $o | 1, (try break $o catch "c"), 2]` is `[1,"c",2]`. The port lets
   the break unwind through `catch`, truncating both streams to `[]` and `[1]`.
4. **`//` swallows a left-hand error** (`alt-error`). jq 1.7 deliberately stopped
   doing this: `(error("x")) // 7` must fail. The port returns `7`.
5. **`@html` escapes `'` as `&#39;`** where jq emits `&apos;` (`at-html`).
6. **`limit` with a negative count** (`limit-neg`). jq 1.7.1 passes the whole
   stream through; the port errors. (jq 1.8.1 also errors, so this one is version
   skew rather than a bug — see below.)

### Over-permissive parsing / evaluation (the port accepts what jq rejects)

- `reverse` on a string (`reverse-string`) — jq 1.7.1 *and* 1.8.1 both fail with
  "Cannot index string with number"; the port returns `"cba"`.
- `from_entries` accepts a bare `k`/`v` entry shape (`from-entries-kv`).
- `with_entries` accepts an array input and stringifies the index into a key
  (`with-entries-nonobj-err`, `with-entries-array-err`).
- `leaf_paths` still exists (`leaf-paths`); jq removed it in 1.7.
- Port-only builtins that jq rejects at compile time: `ascii/0`, `trimstr/1`,
  `skip/2`, `toboolean/0`, `isvalid/1`, `date/0`, `recurse_down/0` — plus the jq-1.8
  arrivals `trim/0`, `ltrim/0`, `rtrim/0`, `toarray/0`, `add/1`, `have_decnum/0`,
  `have_literal_numbers/0`.

### Over-restrictive (the port errors where jq answers)

- `has(1)` on `null` → jq `false`, port a type error (`has-null`).
- `null | indices("a")` → jq `null`, port a type error (`indices-null`).
- `flatten` on an object → jq flattens the values, port errors (`flatten-nonarray`).
- `ltrimstr`/`rtrimstr` on a non-string input or with a non-string argument → jq
  passes the value through untouched, the port raises
  `startswith() requires string inputs` (`ltrimstr-nonstring`, `ltrimstr-num-arg`).
- `[-1] | implode` → jq substitutes U+FFFD, the port errors
  (`implode-invalid-codepoint`).

### Version skew, not a port defect

Four of the mismatches above are cases where the port matches **jq 1.8.1** (also
installed here as `/opt/homebrew/bin/jq`) and the pinned 1.7.1 oracle is the odd
one out: `limit-neg`, `last-empty`, `extra-trim`, `ltrimstr-nonstring`. They are
still counted as mismatches, because the pinned oracle is 1.7.1.

## Confirmed deviations (documented in the port's `API-DEVIATIONS.md`)

These are marked `"deviation"` in the case files and are scored separately.

1. **Object key order is lost.** Values are `map[string]any`, so `keys_unsorted`
   is identical to `keys` and `path(.[])` over an object walks sorted keys
   (`keys-unsorted`, `path-obj-iterate`).
2. **Numbers are float64 only.** jq 1.7 preserves the literal text of a number
   that passes through unchanged, so `100000000000000000000000|tojson` is exact
   upstream and `1e+23` in the port; `9007199254740993` rounds to
   `9007199254740992`; `1.0|tojson` is `1.0` upstream and `1` in the port
   (`number-precision-int`, `number-big-literal`, `number-large-int-arith`,
   `number-float-format`, `tostring-number`).
3. **Regex is RE2.** Lookaround and backreferences compile in oniguruma and fail
   in the port (`regex-lookahead`, `regex-backref`). Everything else in the
   `regex` group — `test match capture sub gsub scan splits`, named captures,
   the `g`/`i`/`x`/`n` flags, codepoint offsets — matches.
4. **No module system.** `import`/`include`/`modulemeta` are compile errors
   (`module-import`, `module-include`, `modulemeta`).
5. **`fromstream`/`truncate_stream` are absent** (`fromstream`,
   `truncate-stream`); `tostream` itself matches on every case.

Two further deviations are **not** yet in `API-DEVIATIONS.md` and should be added:

6. `input_filename` is `null` in the port and `"<stdin>"` upstream
   (`input-filename`) — the port has no file-input concept.
7. C `libm` and Go's `math` differ in the last bit for some transcendentals:
   `0.5|asin` is `0.5235987755982988` upstream and `…89` in the port
   (`math-asin-ulp`). Every other math case is either exact or rounded to 12
   decimals, and all of those match.

## Also confirmed (Go-API level, outside the JSON-Lines contract)

Checked directly against the published module with a throwaway `errors.Is`/
`errors.As` probe:

- **`ErrUndefined` is never wrapped.** Every undefined name — function
  (`no_such_function`), variable (`$nope`), wrong arity (`def f(a): a; f`), label
  (`break $x`) — surfaces as `*CompileError`, whose `Is` only answers
  `ErrCompile` and `ErrSyntax`. `errors.Is(err, jq.ErrUndefined)` is `false` in
  all of them; the sentinel is dead code.
- **`ErrBreak` is never wrapped either.** `breakErr` is only constructed by the
  evaluator and always consumed by its enclosing `label`; a `break` with no
  matching label is rejected at compile time, so `breakErr` never reaches a
  caller and `errors.Is(err, jq.ErrBreak)` is never `true`.
- **An escaped `break` leaks the internal symbol name**: `break $x` reports
  `$*label-x is not defined`. jq 1.7.1 leaks the same mangled name
  (`$*label-x is not defined`), so this is faithful rather than a divergence
  (`break-outside-label`, `break-outside-nested`).
- `input`/`inputs` are stubs, but the harness feeds exactly one input, so jq's
  `input` fails too and both sides agree (`input-stub`, `inputs-stub`).
- A runaway program is `ErrStepLimit` in the port and a crash/exit in jq; both
  are failures, so `runaway-recursion` matches.

## Builtin inventory (`jq -n 'builtins'`, 218 symbols)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `IN/1` | `IN/1` | match | `in-filter-1` |  |
| `IN/2` | `IN/2` | match | `in-filter-2` |  |
| `INDEX/1` | `INDEX/1` | match | `index-builtin-2` |  |
| `INDEX/2` | `INDEX/2` | match | `index-builtin-1` |  |
| `JOIN/2` | `JOIN/2` | match | `join-builtin-2` |  |
| `JOIN/3` | `JOIN/3` | match | `join-builtin-3` |  |
| `JOIN/4` | `JOIN/4` | match | `join-builtin-4` |  |
| `abs/0` | `abs/0` | match | `math-abs` |  |
| `acos/0` | `acos/0` | match | `b-acos` |  |
| `acosh/0` | `acosh/0` | match | `b-acosh` |  |
| `add/0` | `add/0` | match | `add-array`, `add-strings`, `add-empty`, `add-object`, `add-mixed-err` |  |
| `all/0` | `all/0` | match | `b-all-0` |  |
| `all/1` | `all/1` | match | `b-all-1` |  |
| `all/2` | `all/2` | match | `b-all-2` |  |
| `any/0` | `any/0` | match | `any-all`, `any-all-empty` |  |
| `any/1` | `any/1` | match | `any-all-cond` |  |
| `any/2` | `any/2` | match | `any-all-gen` |  |
| `arrays/0` | `arrays/0` | match | `b-arrays` |  |
| `ascii_downcase/0` | `ascii_downcase/0` | match | `ascii-downcase`, `ascii-downcase-nonstring-err` |  |
| `ascii_upcase/0` | `ascii_upcase/0` | match | `ascii-upcase` |  |
| `asin/0` | `asin/0` | differs | `b-asin`, `math-trig`, `math-asin-ulp` | C libm vs Go math: last-ULP difference (not yet in API-DEVIATIONS.md) |
| `asinh/0` | `asinh/0` | match | `b-asinh` |  |
| `atan/0` | `atan/0` | match | `b-atan` |  |
| `atan2/2` | `atan2/2` | match | `math-two-arg` |  |
| `atanh/0` | `atanh/0` | match | `b-atanh` |  |
| `booleans/0` | `booleans/0` | match | `b-booleans` |  |
| `bsearch/1` | `bsearch/1` | match | `bsearch-found`, `bsearch-insert` |  |
| `builtins/0` | `builtins/0` | match | `builtins-count`, `builtins-is-array` |  |
| `capture/1` | `capture/1` | match | `capture-named`, `capture-nomatch` |  |
| `capture/2` | `capture/2` | match | `capture-multi-flags` |  |
| `cbrt/0` | `cbrt/0` | match | `math-roots` |  |
| `ceil/0` | `ceil/0` | match | `b-ceil` |  |
| `combinations/0` | `combinations/0` | match | `combinations` |  |
| `combinations/1` | `combinations/1` | match | `combinations-n` |  |
| `contains/1` | `contains/1` | match | `contains-str`, `contains-arr`, `contains-obj`, `contains-type-err` |  |
| `copysign/2` | — | missing | `math-copysign` | not ported; not ported |
| `cos/0` | `cos/0` | match | `b-cos` |  |
| `cosh/0` | `cosh/0` | match | `b-cosh` |  |
| `debug/0` | `debug/0` | match | `debug-passthrough` |  |
| `debug/1` | `debug/1` | match | `debug-msg` |  |
| `del/1` | `del/1` | match | `del-field`, `del-iterate-sel`, `del-index`, `del-slice`, `del-nonexistent`, `del-multiple-nested` |  |
| `delpaths/1` | `delpaths/1` | match | `delpaths`, `delpaths-order`, `delpaths-empty-path` |  |
| `drem/2` | `drem/2` | match | `b-drem` |  |
| `empty/0` | `empty/0` | match | `empty-builtin` |  |
| `endswith/1` | `endswith/1` | match | `endswith` |  |
| `env/0` | `env/0` | match | `env-parity`, `env-tz`, `env-type` |  |
| `erf/0` | — | missing | `math-erf` | not ported; not ported |
| `erfc/0` | — | missing | `math-erfc` | not ported; not ported |
| `error/0` | `error/0` | match | `error-zero-arity` |  |
| `error/1` | `error/1` | match | `error-msg`, `error-null`, `error-catch-nonstring`, `error-obj-uncaught`, `error-in-map` |  |
| `exp/0` | `exp/0` | match | `b-exp`, `math-exp-log` |  |
| `exp10/0` | `exp10/0` | match | `b-exp10` |  |
| `exp2/0` | `exp2/0` | match | `b-exp2` |  |
| `explode/0` | `explode/0` | match | `explode`, `explode-nonstring-err` |  |
| `expm1/0` | `expm1/0` | match | `b-expm1` |  |
| `fabs/0` | `fabs/0` | match | `b-fabs` |  |
| `fdim/2` | — | missing | `math-fdim` | not ported; not ported |
| `finites/0` | `finites/0` | match | `typed-finites-normals` |  |
| `first/0` | `first/0` | match | `first-0` |  |
| `first/1` | `first/1` | match | `first-of-generator`, `first-empty-err` |  |
| `flatten/0` | `flatten/0` | differs | `flatten-default`, `flatten-nonarray` | jq flattens an object's values; the port errors |
| `flatten/1` | `flatten/1` | match | `flatten-depth`, `flatten-neg-err` |  |
| `floor/0` | `floor/0` | match | `math-rounding`, `math-floor-string-err` |  |
| `fma/3` | — | missing | `math-fma` | not ported; not ported |
| `fmax/2` | — | missing | `math-fmax` | not ported; not ported |
| `fmin/2` | — | missing | `math-fmin` | not ported; not ported |
| `fmod/2` | `fmod/2` | match | `b-fmod` |  |
| `format/1` | `format/1` | match | `format-fn` |  |
| `frexp/0` | — | missing | `math-frexp` | not ported; not ported |
| `from_entries/0` | `from_entries/0` | differs | `from-entries`, `from-entries-kv` | jq 1.7 does not accept a bare `k` as the key alias |
| `fromdate/0` | `fromdate/0` | match | `b-fromdate` |  |
| `fromdateiso8601/0` | `fromdateiso8601/0` | match | `b-fromdateiso8601` |  |
| `fromjson/0` | `fromjson/0` | match | `fromjson-bad-err` |  |
| `fromstream/1` | — | missing | `fromstream` | not ported; fromstream/truncate_stream are documented as not implemented |
| `gamma/0` | — | missing | `math-gamma` | not ported; not ported |
| `get_jq_origin/0` | — | missing | `origin-jq` | not ported; not ported |
| `get_prog_origin/0` | — | missing | `origin-prog` | not ported; not ported |
| `get_search_list/0` | — | missing | `origin-search-list` | not ported; not ported |
| `getpath/1` | `getpath/1` | match | `getpath`, `getpath-missing`, `getpath-through-num-err`, `getpath-nonpath-err`, `getpath-num-key-on-obj-err` |  |
| `gmtime/0` | `gmtime/0` | match | `gmtime-array` |  |
| `group_by/1` | `group_by/1` | match | `group-by`, `group-by-nonarray-err` |  |
| `gsub/2` | `gsub/2` | match | `gsub-basic`, `gsub-empty-match`, `gsub-named` |  |
| `gsub/3` | `gsub/3` | match | `gsub-flags` |  |
| `halt/0` | `halt/0` | match | `halt-plain` |  |
| `halt_error/0` | `halt_error/0` | match | `halt-error` |  |
| `halt_error/1` | `halt_error/1` | match | `halt-error-code` |  |
| `has/1` | `has/1` | differs | `has-obj`, `has-array`, `has-string-err`, `has-null` | jq answers false rather than erroring |
| `hypot/2` | `hypot/2` | match | `b-hypot` |  |
| `implode/0` | `implode/0` | differs | `implode`, `implode-invalid-codepoint`, `implode-surrogate-err`, `implode-nonarray-err` | jq substitutes U+FFFD; the port errors |
| `in/1` | `in/1` | match | `in-op`, `in-nonobj-err` |  |
| `index/1` | `index/1` | match | `index-str` |  |
| `indices/1` | `indices/1` | differs | `indices-str`, `indices-arr`, `indices-null`, `indices-empty-str`, `indices-arr-in-str-err` | jq propagates null instead of erroring |
| `infinite/0` | `infinite/0` | match | `infinite-output` |  |
| `input/0` | `input/0` | match | `input-stub` |  |
| `input_filename/0` | `input_filename/0` | differs | `input-filename` | the port has no file-input concept, so input_filename is null (not yet in API-DEVIATIONS.md) |
| `input_line_number/0` | `input_line_number/0` | match | `input-line-number`, `input-line-number-null` |  |
| `inputs/0` | `inputs/0` | match | `inputs-stub` |  |
| `inside/1` | `inside/1` | match | `inside` |  |
| `isempty/1` | `isempty/1` | match | `isempty` |  |
| `isfinite/0` | — | missing | `math-isfinite` | not ported; not ported |
| `isinfinite/0` | `isinfinite/0` | match | `b-isinfinite` |  |
| `isnan/0` | `isnan/0` | match | `nan-inf-predicates` |  |
| `isnormal/0` | `isnormal/0` | match | `b-isnormal` |  |
| `iterables/0` | `iterables/0` | match | `b-iterables` |  |
| `j0/0` | — | missing | `math-j0` | not ported; not ported |
| `j1/0` | — | missing | `math-j1` | not ported; not ported |
| `jn/2` | — | missing | `math-jn` | not ported; not ported |
| `join/1` | `join/1` | match | `join-strings`, `join-mixed`, `join-empty`, `join-obj-err`, `join-nonarray-err` |  |
| `keys/0` | `keys/0` | match | `keys`, `keys-array`, `keys-string-err` |  |
| `keys_unsorted/0` | `keys_unsorted/0` | differs | `keys-unsorted` | object key order is not preserved (map[string]any); keys_unsorted == keys |
| `last/0` | `last/0` | match | `last-0` |  |
| `last/1` | `last/1` | differs | `last-of-generator`, `last-empty` | jq 1.7 yields null for last(empty); collapsing to an empty stream is a bug |
| `ldexp/2` | `ldexp/2` | match | `b-ldexp` |  |
| `length/0` | `length/0` | match | `length-arity-err`, `length-all`, `length-bool-err` |  |
| `lgamma/0` | — | missing | `math-lgamma` | not ported; not ported |
| `lgamma_r/0` | — | missing | `math-lgamma-r` | not ported; not ported |
| `limit/2` | `limit/2` | differs | `limit-basic`, `limit-zero`, `limit-neg`, `limit-of-empty` | jq 1.7 passes everything through for n < 0 |
| `localtime/0` | `localtime/0` | match | `localtime-mktime` |  |
| `log/0` | `log/0` | match | `b-log` |  |
| `log10/0` | `log10/0` | match | `b-log10` |  |
| `log1p/0` | `log1p/0` | match | `b-log1p` |  |
| `log2/0` | `log2/0` | match | `b-log2` |  |
| `logb/0` | `logb/0` | match | `math-logb-significand` |  |
| `ltrimstr/1` | `ltrimstr/1` | differs | `ltrimstr`, `ltrimstr-empty`, `ltrimstr-nonstring`, `ltrimstr-num-arg` | jq 1.7 passes non-strings through unchanged |
| `map/1` | `map/1` | match | `map-basic`, `map-object` |  |
| `map_values/1` | `map_values/1` | match | `map-values-obj`, `map-values-empty`, `map-values-many` |  |
| `match/1` | `match/1` | match | `match-basic`, `match-named`, `match-unicode-offset` |  |
| `match/2` | `match/2` | match | `match-global`, `match-empty-regex`, `match-n-flag` |  |
| `max/0` | `max/0` | match | `b-max-0` |  |
| `max_by/1` | `max_by/1` | match | `max-by-only` |  |
| `min/0` | `min/0` | match | `min-max`, `min-max-empty` |  |
| `min_by/1` | `min_by/1` | match | `min-by-max-by`, `min-by-empty` |  |
| `mktime/0` | `mktime/0` | match | `gmtime-mktime` |  |
| `modf/0` | — | missing | `math-modf` | not ported; not ported |
| `modulemeta/0` | — | missing | `modulemeta` | not ported |
| `nan/0` | `nan/0` | match | `nan-output` |  |
| `nearbyint/0` | `nearbyint/0` | match | `b-nearbyint` |  |
| `nextafter/2` | — | missing | `math-nextafter` | not ported; not ported |
| `nexttoward/2` | — | missing | `math-nexttoward` | not ported; not ported |
| `normals/0` | `normals/0` | match | `b-normals` |  |
| `not/0` | `not/0` | match | `bool-not`, `not-arity-err` |  |
| `now/0` | `now/0` | match | `now-type` |  |
| `nth/1` | `nth/1` | match | `nth-1` |  |
| `nth/2` | `nth/2` | match | `nth-generator`, `nth-neg-err`, `nth-of-empty` |  |
| `nulls/0` | `nulls/0` | match | `b-nulls` |  |
| `numbers/0` | `numbers/0` | match | `typed-filters` |  |
| `objects/0` | `objects/0` | match | `b-objects` |  |
| `path/1` | `path/1` | differs | `path-simple`, `path-iterate`, `path-obj-iterate`, `path-slice`, `path-of-value-err`, `path-with-optional`, `path-alternative-err`, `path-getpath` | object key order is not preserved (map[string]any); keys_unsorted == keys |
| `paths/0` | `paths/0` | match | `paths-all`, `paths-empty` |  |
| `paths/1` | `paths/1` | match | `paths-filtered` |  |
| `pick/1` | `pick/1` | match | `pick-paths` |  |
| `pow/2` | `pow/2` | match | `math-pow-string-err` |  |
| `pow10/0` | — | missing | `math-pow10` | not ported |
| `range/1` | `range/1` | match | `range-1`, `range-nonnumber-err` |  |
| `range/2` | `range/2` | match | `range-2`, `range-stream-args` |  |
| `range/3` | `range/3` | match | `range-3`, `range-neg-step`, `range-zero-step` |  |
| `recurse/0` | `recurse/0` | match | `recurse-all` |  |
| `recurse/1` | `recurse/1` | match | `recurse-cond`, `recurse-f-cond-capped` |  |
| `recurse/2` | `recurse/2` | match | `recurse-2` |  |
| `remainder/2` | — | missing | `math-remainder` | not ported; not ported |
| `repeat/1` | `repeat/1` | differs | `repeat-capped`, `repeat-plus` | capped with limit; jq re-applies f to the ORIGINAL input each time |
| `reverse/0` | `reverse/0` | differs | `reverse-array`, `reverse-string`, `reverse-null`, `reverse-num-err` | jq 1.7 cannot reverse a string; the port can |
| `rindex/1` | `rindex/1` | match | `rindex-str` |  |
| `rint/0` | `rint/0` | match | `b-rint` |  |
| `round/0` | `round/0` | match | `b-round` |  |
| `rtrimstr/1` | `rtrimstr/1` | match | `rtrimstr` |  |
| `scalars/0` | `scalars/0` | match | `b-scalars` |  |
| `scalb/2` | — | missing | `math-scalb` | not ported; not ported |
| `scalbln/2` | — | missing | `math-scalbln` | not ported; not ported |
| `scan/1` | `scan/1` | match | `scan-basic`, `scan-groups`, `scan-no-match` |  |
| `scan/2` | `scan/2` | match | `scan-flags` |  |
| `select/1` | `select/1` | match | `select-basic`, `select-truthy` |  |
| `setpath/2` | `setpath/2` | match | `setpath`, `setpath-create`, `setpath-array-grow`, `setpath-empty-path`, `setpath-bad-key-err` |  |
| `significand/0` | `significand/0` | match | `b-significand` |  |
| `sin/0` | `sin/0` | match | `b-sin` |  |
| `sinh/0` | `sinh/0` | match | `b-sinh`, `math-hyper` |  |
| `sort/0` | `sort/0` | match | `sort-nums`, `sort-mixed-types`, `sort-objects`, `sort-nonarray-err`, `sort-string-err` |  |
| `sort_by/1` | `sort_by/1` | match | `sort-by-key`, `sort-by-multi`, `sort-by-stable`, `sort-by-nonarray-err` |  |
| `split/1` | `split/1` | match | `split-1`, `split-empty-sep`, `split-no-match`, `split-non-string-err` |  |
| `split/2` | `split/2` | match | `split-regex`, `split-regex-flags` |  |
| `splits/1` | `splits/1` | match | `splits-basic`, `splits-regex`, `splits-empty` |  |
| `splits/2` | `splits/2` | match | `splits-null-flags` |  |
| `sqrt/0` | `sqrt/0` | match | `math-sqrt-exact`, `math-sqrt-string-err`, `math-sqrt-neg-nan` |  |
| `startswith/1` | `startswith/1` | match | `startswith`, `startswith-nonstring-err` |  |
| `stderr/0` | `stderr/0` | match | `stderr-passthrough` |  |
| `strflocaltime/1` | `strflocaltime/1` | match | `strflocaltime` |  |
| `strftime/1` | `strftime/1` | match | `strftime` |  |
| `strings/0` | `strings/0` | match | `b-strings` |  |
| `strptime/1` | `strptime/1` | match | `strptime` |  |
| `sub/2` | `sub/2` | match | `sub-basic`, `sub-refs`, `sub-many-replacements` |  |
| `sub/3` | `sub/3` | match | `sub-global-flag`, `sub-flags-g-named` |  |
| `tan/0` | `tan/0` | match | `b-tan` |  |
| `tanh/0` | `tanh/0` | match | `b-tanh` |  |
| `test/1` | `test/1` | differs | `test-basic`, `test-array-arg`, `test-nonstring-err`, `test-bad-regex-err`, `regex-lookahead`, `regex-backref` | regexp is RE2, so lookaround and backreferences are unsupported |
| `test/2` | `test/2` | match | `test-flags-i`, `test-null-flags`, `regex-x-flag` |  |
| `tgamma/0` | — | missing | `math-tgamma` | not ported; not ported |
| `to_entries/0` | `to_entries/0` | match | `to-entries`, `to-entries-array-err` |  |
| `todate/0` | `todate/0` | match | `todate-fromdate` |  |
| `todateiso8601/0` | `todateiso8601/0` | match | `date-iso` |  |
| `tojson/0` | `tojson/0` | differs | `number-float-format`, `number-precision-int`, `number-big-literal`, `number-large-int-arith`, `number-neg-zero`, `tojson-nan`, `tojson-fromjson` | numbers are float64 only; jq preserves the literal text of untouched numbers |
| `tonumber/0` | `tonumber/0` | match | `tonumber-str`, `tonumber-num`, `tonumber-bad-err`, `tonumber-null-err` |  |
| `tostream/0` | `tostream/0` | match | `tostream`, `tostream-scalar`, `tostream-empty-obj`, `tostream-nested`, `tostream-limit` |  |
| `tostring/0` | `tostring/0` | differs | `tostring-number`, `tostring-all` | numbers are float64 only; jq preserves the literal text of untouched numbers |
| `transpose/0` | `transpose/0` | match | `transpose`, `transpose-uneven` |  |
| `trunc/0` | `trunc/0` | match | `b-trunc` |  |
| `truncate_stream/1` | — | missing | `truncate-stream` | not ported; fromstream/truncate_stream are documented as not implemented |
| `type/0` | `type/0` | match | `type-all` |  |
| `unique/0` | `unique/0` | match | `unique`, `unique-mixed`, `unique-nonarray-err` |  |
| `unique_by/1` | `unique_by/1` | match | `unique-by`, `unique-by-length` |  |
| `until/2` | `until/2` | match | `until-loop`, `until-immediate` |  |
| `utf8bytelength/0` | `utf8bytelength/0` | match | `utf8bytelength` |  |
| `values/0` | `values/0` | match | `values-builtin` |  |
| `walk/1` | `walk/1` | match | `walk-sort`, `walk-numbers` |  |
| `while/2` | `while/2` | match | `while-loop`, `while-none` |  |
| `with_entries/1` | `with_entries/1` | differs | `with-entries-array-err`, `with-entries`, `with-entries-nonobj-err` | jq rejects an array input; the port turns the index into a key |
| `y0/0` | — | missing | `math-y0` | not ported; not ported |
| `y1/0` | — | missing | `math-y1` | not ported; not ported |
| `yn/2` | — | missing | `math-yn` | not ported; not ported |

### Port-only symbols (`extra`, 24)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `@base64/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@base64d/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@csv/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@html/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@json/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@sh/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@text/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@tsv/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `@uri/0` | extra | — | jq exposes this as a format string, not as a `builtins` entry |
| — | `add/1` | extra | `extra-add-1` | port-only |
| — | `ascii/0` | extra | `extra-ascii` | port-only |
| — | `date/0` | extra | `extra-date` | port-only |
| — | `have_decnum/0` | extra | `extra-have-decnum` | port-only |
| — | `have_literal_numbers/0` | extra | — | port-only |
| — | `isvalid/1` | extra | `extra-isvalid` | port-only |
| — | `leaf_paths/0` | extra | `leaf-paths` | port-only |
| — | `ltrim/0` | extra | — | port-only |
| — | `recurse_down/0` | extra | `extra-recurse-down` | port-only |
| — | `rtrim/0` | extra | — | port-only |
| — | `skip/2` | extra | `extra-skip` | port-only |
| — | `toarray/0` | extra | `extra-toarray` | port-only |
| — | `toboolean/0` | extra | `extra-toboolean` | port-only |
| — | `trim/0` | extra | `extra-trim`, `extra-trim-nonstring-err` | port-only |
| — | `trimstr/1` | extra | `extra-trimstr` | port-only |

## Syntax and operator inventory (69 constructs)

Not reachable from `builtins`; enumerated from the jq 1.7.1 manual's grammar
sections and each given at least one case.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `!=` | `!=` | match | `cmp-ne` |  |
| `"..."` | `"..."` | match | `string-escapes` |  |
| `"\(f)"` | `"\(f)"` | match | `string-interp`, `string-interp-obj` |  |
| `#` | `#` | match | `comment-is-not-an-error` |  |
| `$ENV` | `$ENV` | match | `env-keys`, `var-env-named` |  |
| `$__loc__` | `$__loc__` | match | `loc-builtin` |  |
| `%` | `%` | match | `mod-nums`, `mod-neg`, `mod-zero`, `mod-float-trunc` |  |
| `*` | `*` | match | `mul-nums`, `mul-str-num`, `mul-str-zero`, `mul-obj-deep-merge` |  |
| `+` | `+` | match | `add-nums`, `add-str`, `add-arr`, `add-obj-merge`, `add-null-left`, `add-null-right`, `add-null-null`, `add-str-num-err`, `add-obj-arr-err`, `add-bool-err` |  |
| `+=` | `+=` | match | `assign-plus` |  |
| `,` | `,` | match | `comma` |  |
| `-` | `-` | match | `sub-nums`, `sub-arrays`, `sub-str-err` |  |
| `-(unary)` | `-(unary)` | match | `neg-unary`, `neg-string-err` |  |
| `--arg/--argjson` | `--arg/--argjson` | match | `var-scalar` |  |
| `--argjson` | `--argjson` | match | `var-object`, `var-in-filter`, `var-many`, `var-shadowed-by-def`, `var-unused`, `var-null`, `var-array-index`, `var-as-path` |  |
| `-=` | `-=` | match | `assign-arith-family` |  |
| `.` | `.` | match | `identity` |  |
| `..` | `..` | match | `recursive-descent`, `recursive-descent-select` |  |
| `.["foo"]` | `.["foo"]` | match | `field-bracket`, `index-array-with-string-err` |  |
| `.["foo"]?` | `.["foo"]?` | match | `field-bracket-opt` |  |
| `.[-n]` | `.[-n]` | match | `index-neg`, `index-neg-oob` |  |
| `.[]` | `.[]` | match | `iterate-array`, `iterate-object`, `iterate-empty`, `iterate-num-err`, `iterate-null-err`, `multi-index-iterate`, `iterate-string-err` |  |
| `.[]?` | `.[]?` | match | `iterate-opt-num` |  |
| `.[a:b]` | `.[a:b]` | match | `slice`, `slice-neg`, `slice-open-left`, `slice-rev-bounds`, `slice-string`, `slice-null`, `slice-oob`, `slice-noninteger`, `slice-object-err`, `slice-string-bound-err` |  |
| `.[n]` | `.[n]` | match | `index-int`, `index-oob`, `index-null`, `index-object-with-num-err`, `index-string-with-num-err` |  |
| `.foo` | `.foo` | match | `field`, `field-missing`, `field-on-array-err`, `field-on-bool-err` |  |
| `.foo.bar` | `.foo.bar` | match | `field-nested` |  |
| `.foo?` | `.foo?` | match | `field-opt-on-num`, `field-opt-on-null` |  |
| `/` | `/` | match | `div-nums`, `div-zero`, `div-zero-zero`, `div-strings-split` |  |
| `//` | `//` | differs | `alt-null`, `alt-false`, `alt-error`, `alt-empty`, `alt-first-of-many` | jq 1.7 no longer swallows a left-hand error |
| `//=` | `//=` | match | `assign-alt-update` |  |
| `<` | `<` | match | `cmp-lt-nums`, `cmp-sort-order-types`, `cmp-error-free` |  |
| `=` | `=` | match | `assign-eq`, `assign-multi-path`, `assign-rhs-many` |  |
| `==` | `==` | match | `cmp-eq-objects` |  |
| `>=` | `>=` | match | `cmp-ge-le` |  |
| `?` | `?` | match | `optional-suffix`, `optional-on-error-func` |  |
| `?//` | `?//` | match | `destructure-alt`, `question-alt-destructure` |  |
| `@base32` | `@base32` | match | `at-base32` |  |
| `@base32d` | `@base32d` | match | `at-base32d` |  |
| `@base64` | `@base64` | match | `at-base64`, `at-format-interp` |  |
| `@base64d` | `@base64d` | match | `at-base64d`, `at-base64d-bad` |  |
| `@csv` | `@csv` | match | `at-csv`, `at-csv-nested-err`, `at-csv-interp` |  |
| `@html` | `@html` | differs | `at-html` | jq escapes the apostrophe as &apos;, the port as &#39; |
| `@json` | `@json` | match | `at-json` |  |
| `@sh` | `@sh` | match | `at-sh`, `at-sh-obj-err` |  |
| `@text` | `@text` | match | `at-text` |  |
| `@tsv` | `@tsv` | match | `at-tsv` |  |
| `@uri` | `@uri` | match | `at-uri` |  |
| `[]` | `[]` | match | `arr-construct`, `arr-collect-stream`, `arr-empty` |  |
| `and` | `and` | match | `bool-and`, `bool-shortcircuit` |  |
| `as` | `as` | match | `as-binding`, `as-multi-values` |  |
| `as [$a]` | `as [$a]` | match | `destructure-array` |  |
| `as {$a}` | `as {$a}` | match | `destructure-object`, `destructure-shorthand`, `destructure-nested` |  |
| `def` | `def` | match | `def-simple`, `def-args`, `def-filter-arg`, `def-recursive`, `def-closure-scope`, `def-inner-shadow`, `def-multi-arity`, `def-arg-generator`, `runaway-recursion` |  |
| `elif` | `elif` | match | `if-elif` |  |
| `foreach` | `foreach` | match | `foreach-running`, `foreach-extract`, `foreach-3-args-empty` |  |
| `if` | `if` | match | `if-basic`, `if-no-else`, `if-no-else-passthrough`, `if-cond-stream` |  |
| `import` | `import` | match | `module-import` |  |
| `include` | `include` | match | `module-include` |  |
| `label` | `label` | match | `label-break`, `label-break-limit`, `nested-label-same-name`, `break-outside-label`, `break-outside-nested` |  |
| `or` | `or` | match | `bool-or` |  |
| `reduce` | `reduce` | match | `reduce-sum`, `reduce-empty`, `reduce-obj`, `reduce-multi-update`, `reduce-error-propagates` |  |
| `syntax error` | `syntax error` | match | `syntax-error-trailing-pipe`, `syntax-error-open-brace`, `syntax-error-unbalanced`, `syntax-error-empty`, `syntax-error-juxtaposed` |  |
| `try` | `try` | differs | `try-catch`, `try-catch-obj`, `try-no-catch`, `try-type-error`, `nested-try`, `try-catch-break`, `catch-does-not-catch-break` | jq's break is an error value that catch can see |
| `undefined function` | `undefined function` | match | `undefined-func`, `arity-mismatch-def` |  |
| `undefined variable` | `undefined variable` | match | `undefined-var`, `undefined-var-scoped`, `var-missing` |  |
| `{}` | `{}` | match | `nested-construct`, `obj-shorthand`, `obj-computed-key`, `obj-string-key-interp`, `obj-key-nonstring-err`, `obj-multi-values`, `obj-empty` |  |
| `|` | `|` | match | `pipe-chain` |  |
| `|=` | `|=` | match | `assign-update`, `assign-iterate`, `assign-update-empty` |  |

## Totals

### Builtins (denominator = symbols with at least one case)

| status | count |
| --- | --- |
| match | 168 |
| differs | 18 |
| missing | 32 |
| untested | 0 |
| **extra** | 24 |
| **total in `builtins`** | 218 |

Every one of the 218 symbols has at least one case, so the denominator is the
full list: **168/218 = 77.06%** parity over jq's builtin surface
(`differs` 18 + `missing` 32 = 50 symbols short).

### Syntax and operators

| status | count |
| --- | --- |
| match | 66 |
| differs | 3 |

**66/69 = 95.65%**.

### Combined symbol parity

**234/287 = 81.53%** over every compared symbol (218 builtins + 69 syntax constructs).

### Case totals (from `parity.json`, rewritten by the test)

| | |
| --- | --- |
| cases | 574 |
| match | 503 |
| mismatch | 58 |
| deviations | 13 |
| case parity | 89.66% (match / (cases − deviations)) |

Per group:

| group | cases | match | mismatch | deviations |
| --- | --- | --- | --- | --- |
| arith | 45 | 40 | 0 | 5 |
| builtins | 47 | 47 | 0 | 0 |
| collections | 57 | 55 | 2 | 0 |
| control | 88 | 81 | 7 | 0 |
| core | 52 | 52 | 0 | 0 |
| errors | 21 | 20 | 1 | 0 |
| math | 40 | 14 | 25 | 1 |
| misc | 41 | 26 | 14 | 1 |
| objects | 37 | 32 | 4 | 1 |
| paths | 48 | 44 | 1 | 3 |
| regex | 35 | 33 | 0 | 2 |
| strings | 52 | 48 | 4 | 0 |
| vars | 11 | 11 | 0 | 0 |
