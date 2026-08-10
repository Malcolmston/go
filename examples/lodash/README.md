# lodash example

A runnable, self-terminating program that exercises
[`github.com/malcolmston/lodash`](https://github.com/malcolmston/lodash)
(a generics-based port of lodash) as an **external consumer of the published
module** — there is no `replace` directive; the dependency is resolved from the
module proxy.

**Resolved module version:** `github.com/malcolmston/lodash v0.0.0-20260719133131-96017314187c`
(pseudo-version; the repo has no semver tags.)

## Run

```sh
cd examples/lodash
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Output is grouped into labelled sections; the program prints a final
"All sections completed." line and exits 0.

## What it demonstrates

Roughly 200 distinct exported functions across every category the library
advertises:

- **Collections** — `Map`, `MapI`, `Filter`, `Reject`, `Reduce`, `ReduceRight`,
  `ForEach*`, `Find*`, `Every`/`Some`/`None`, `Includes`, `IndexOf*`,
  `GroupBy`, `KeyBy`, `CountBy`, `Partition`, `SortBy`, `OrderBy` (multi-key,
  `Asc`/`Desc`), plus the map-flavoured set: `EveryMap`, `SomeMap`, `NoneMap`,
  `FindMap`, `FilterMap`, `RejectMap`, `MapToSlice`, `ReduceMap`,
  `PartitionMap`, `GroupByMap`, `IncludesValue`, `MinByMap`, `MaxByMap`.
- **Arrays** — `Chunk`, `Take*`/`Drop*` (+ `While` variants), `Head`/`First`/
  `Last`/`Nth`/`Initial`/`Tail`/`Slice` (incl. negative indices), `Join`,
  `Reverse`, `Uniq`/`UniqBy`/`UniqWith`/`SortedUniq*`, `Compact`, `Flatten`/
  `FlattenDeep`/`FlattenDepth`/`FlatMap`/`FlatMapDeep`/`FlatMapDepth`, `Zip`/
  `Unzip`/`ZipWith`/`UnzipWith`/`ZipObject`/`ZipObjectDeep`, `Fill`/`FillRange`,
  `Concat`, `Pull*`, `PullAt`, `Remove`, `Without`, the `Sorted*Index*` binary
  searches, and seeded `Sample`/`SampleN`/`Shuffle`.
- **Set ops** — `Difference`/`Intersection`/`Union`/`Xor` and all their `By`/
  `With` variants.
- **Math & numbers** — `Sum*`, `Mean*`, `Min*`/`Max*`, `Add`/`Subtract`/
  `Multiply`/`Divide`, `Clamp`, `Round`/`Ceil`/`Floor` with precision,
  `Range`/`RangeStep`/`RangeRight`, `InRange`, `Random`/`RandomFloat`.
- **Objects** — `Keys`/`SortedKeys`/`Values`/`Entries`/`FromEntries`/`ToPairs`/
  `FromPairs`, `Pick*`/`Omit*`, `MapKeys`/`MapValues`, `Invert`/`InvertBy`,
  `Merge*`/`Assign*`/`Defaults`/`DefaultsDeep`, `Transform`, `ForOwn`,
  `FindKey`, `Size`.
- **Deep paths** — `ToPath` (dotted, bracket, quoted), `Get`/`GetOr`/`Has`/`At`/
  `Result`/`Set`/`Unset`/`Update`/`SetWith`/`UpdateWith`, `Property`,
  `PropertyOf`, `Matches`, `MatchesProperty`.
- **Strings** — `Words`, all six case converters, `Capitalize`/`UpperFirst`/
  `LowerFirst`, `Pad*`, `Truncate`, `Repeat`, `Deburr`, `Trim*`, `Escape`/
  `Unescape`/`EscapeRegExp`, `Replace`, `Split`, `StartsWith`/`EndsWith`,
  `Template` (default and custom delimiters), `ParseInt`/`ParseFloat`.
- **Lang** — `Clone` vs `CloneDeep` (shows `Clone` sharing a slice backing
  array), `IsEqual`/`Eq`/`IsEmpty`/`IsNil`/`IsMatch`, the full `Is*` predicate
  family incl. `IsFunction`/`IsRegExp`/`IsDate`/`IsBuffer`/`IsSafeInteger`/
  `IsLength`/`IsNull`/`IsUndefined`/`IsObject`/`IsArrayLike*`, the `To*`
  coercions, `Conforms`/`ConformsTo`, `IsEqualWith`/`IsMatchWith`.
- **Combinators** — `Curry`/`Curry3`/`Curry4`/`CurryRight*`, `Partial*`, `Flip`,
  `Flow`/`FlowRight`/`Compose`, `Cond`+`CondPair`, `Over`/`OverEvery`/
  `OverSome`/`OverArgs`, `Negate`, `Ary`, `Unary`, `Rearg`, `Spread`, `NthArg`,
  `Wrap`, `Rest`.
- **Function utilities** — `Once`, `Memoize`/`MemoizeBy` (verifying the
  underlying call count), `After`/`Before`, `NewDebouncer` (coalescing a burst
  and `Cancel`), `NewThrottler`, `Delay`, `Defer`, `Times`, `UniqueID`,
  `Identity`/`Constant`/`Noop`, the `Stub*` family, `Attempt` (incl. recovering
  a panic).
- **Chaining** — `Chain`/`Seq` (`Thru`, `Tap`, package-level `Thru`/`Tap` for
  type changes), `ChainSlice`/`SliceSeq`, `ChainSet`/`SetSeq`.
- **Generics & edge cases** — an entire section on empty/nil inputs: `nil`
  slices and maps through ~40 helpers, out-of-range `Slice`/`Nth`,
  zero-argument variadic `Union()`/`Intersection()`/`Flow()`/`Over()`/
  `CastArray()`, `Get`/`Set` on a `nil` map, empty-string paths and templates,
  `IsString(nil)`, `ToNumber("abc")`, negative counts.

## Holes found

- **`Set` (and `Update`/`SetWith`) cannot write into a slice.** `setPath` has no
  `[]any` case: writing `Set(obj, "user.tags[2]", "cs")` where `user.tags` is
  an existing `[]any` **replaces the whole slice with `map[string]any{"2": …}`**,
  silently destroying the existing elements. `Get` *does* traverse `[]any` by
  integer index, so the read and write paths are asymmetric — a real
  round-tripping bug, not just a limitation. The example prints the broken
  result (labelled `Set into slice (BROKEN)`).
- **`Chunk` panics on `size < 1`** and **`RangeStep` panics on `step == 0`**.
  Both are documented, but they diverge from lodash (JS returns `[]` and a
  filled array respectively) and are surprising for a library whose other
  helpers return zero values instead of panicking. The example catches both with
  `recover`.
- **`Sample`/`SampleN`/`Shuffle` dereference their `*rand.Rand` unconditionally**,
  so passing `nil` is a nil-pointer panic rather than falling back to the global
  source. The example notes this in a `// HOLE:` comment rather than crashing.
- **Inconsistent argument order in the set-op family.** `Difference(s, others…)`
  puts the subject first, but `DifferenceBy(fn, s, others…)` /
  `DifferenceWith(eq, s, others…)` / `IntersectionBy(fn, slices…)` /
  `UnionBy(fn, slices…)` / `XorBy(fn, slices…)` put the iteratee first (forced
  by Go's variadics, but still a footgun when switching between them).
- **`Template` custom delimiters silently swallow interpolation.** `Open`/`Close`
  replace only `<%`/`%>`, so the `=` marker is still required: with
  `TemplateOptions{Open: "{{", Close: "}}"}`, `{{ n }}` is parsed as an
  (ignored) evaluate block and renders as `""` — you must write `{{= n }}`.
  Neither the README nor the `TemplateOptions` doc mentions this. The example
  shows both forms.
- **`Round` parity divergence:** `Round(-4.5, 0)` returns `-5` (Go's
  `math.Round`, half-away-from-zero) where JS `_.round(-4.5)` returns `-4`
  (half-up). The doc comment explicitly claims "round-half-away-from-zero", so
  this is a deliberate but undeclared parity break for a library that ships
  `*_parity_test.go` files.
- **`Fill` does not match lodash.** `Fill(value, n)` builds a *new* slice of
  length `n`; lodash's `_.fill(array, value, [start], [end])` overwrites an
  existing array. The actual analogue is the separately named `FillRange`.
- Minor, non-idiomatic: `Property`/`PropertyOf`/`Get` are hard-wired to
  `map[string]any` only, so no deep-path helper works on structs or on
  `map[string]string`; `OrderBy` takes `[]func(a, b T) int` comparators rather
  than iteratees, which is much noisier than lodash's `_.orderBy(list, ['a'], ['desc'])`;
  `Clone`/`CloneDeep` of a `nil` slice or map return an empty non-nil value.
- README/`doc.go` claim "over 250 functions … roughly 80% of lodash's API" and
  the listed inventories are accurate as far as they go, but they **omit** a
  number of exported helpers that do exist: `UniqWith`, `PullAllWith`,
  `ZipObjectDeep`, `FlatMapDeep`, `FlatMapDepth`, `FillRange`, `SetWith`,
  `UpdateWith`, `MatchesProperty`, `CurryRight`, `CurryRight3`, `Curry4`,
  `Rest`, `Delay`, `Defer`, `ParseInt`, `ParseFloat`, the whole
  `predicates.go` set (`IsFunction`, `IsRegExp`, `IsDate`, `IsBuffer`,
  `IsSafeInteger`, `IsLength`, `IsNull`, `IsUndefined`, `IsObject`,
  `IsArrayLike`, `IsArrayLikeObject`, `ToLength`, `IsEqualWith`,
  `IsMatchWith`), the `*Map` collection family in `collectionmap.go`, and the
  `ChainSlice`/`ChainSet` wrappers (only `Chain` is mentioned).

No compile failures and no unexpected runtime panics; nothing had to be
commented out except the deliberate `nil`-`*rand.Rand` probe.
