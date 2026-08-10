# pandas parity coverage

**Upstream oracle:** `{upstream}` (CPython 3.13, importable system install — no
virtualenv was needed; `python3 -c "import pandas"` already worked, so nothing
was installed for this harness).
**Go port under test:** `{gomodule}` (consumed as a published module; no
`replace` directive).
**Float tolerance:** absolute-or-relative `{tol}` — two numbers match when
`abs(a-b) <= tol` or `abs(a-b)/max(abs(a),abs(b)) <= tol`. Every JSON number is
decoded as float64 on both sides, so `1` and `1.0` compare equal *as values*;
dtypes are compared separately, so an int-vs-float dtype difference is still
caught.
**Missing-value sentinel:** the string `"__NA__"` on both sides.
Non-finite floats encode as `"__INF__"`, `"__-INF__"`, `"__NAN__"`.

## Case score

| | count |
| --- | --- |
| cases run | {cases} |
| match | {case_match} |
| differs | {case_differ} |
| deliberate deviation (excluded from the score) | {case_dev} |
| compared (match + differs) | {sym_denom} |
| **case parity** | **{case_parity}%** = {case_match} / {sym_denom}; the {case_dev} deviation is excluded from the denominator |

Regenerate with `GOWORK=off go test ./parity/pandas/`, which rewrites
`parity.json`; then `python3 python/coverage.py` from `parity/pandas/` rewrites
this file.

## Symbol score

| status | count |
| --- | --- |
| match | {n_match} |
| differs | {n_differs} |
| deviation | {n_deviation} |
| missing | {n_missing} |
| untested | {n_untested} |
| extra (Go-only) | {n_extra} |
| **upstream symbols inventoried** | **{total_symbols}** |

**Symbol parity: {sym_parity}%** = {n_match} match over the {sym_compared}
symbols actually compared (match + differs + deviation). Symbols with no case
are `untested`, never `match`. The one case-level deviation
(`gb-default-naming`) rolls up into a symbol that also has failing cases, so it
appears as `differs` in the symbol table rather than as its own `deviation` row.

### How the inventory was derived

Mechanically, from the *installed* pandas — not from memory or the README:

```python
import pandas as pd
from pandas.core.groupby import DataFrameGroupBy
[n for n in dir(pd.DataFrame)               if not n.startswith("_")]   # {n_df}
[n for n in dir(pd.Series)                  if not n.startswith("_")]   # {n_sr}
[n for n in dir(pd.Series.str)              if not n.startswith("_")]   # {n_str}
[n for n in dir(DataFrameGroupBy)           if not n.startswith("_")]   # {n_gb}
[n for n in dir(pd)                         if not n.startswith("_")]   # module level
```

`python/coverage.py` runs exactly that and joins it against `parity.json`.
Module-level names were filtered to lowercase functions plus `DataFrame` and
`Series`; the ~55 exported *type* names (`Timestamp`, `Categorical`,
`IntervalIndex`, the whole `arrays`/`api`/`errors` surface, …) have no analogue
in the port and are not individually listed.

### Be clear about the denominator

pandas' real surface is very much larger than this table: `dir()` on the two
core classes alone is {n_df} + {n_sr} names, and that excludes `pandas.Index`
and its ~20 subclasses, the `.dt`/`.cat`/`.sparse`/`.plot` accessors,
`pandas.api.*`, `pandas.io.*` (about 40 readers and writers), `pandas.testing`,
`ExtensionArray`, `MultiIndex`, `Timedelta`/`Period`/`Timestamp` arithmetic,
`pandas.eval`, resampling, rolling/expanding/EWM windows, `pivot`/`pivot_table`
/`melt`/`stack`/`unstack`/`explode`, `Styler`, and the whole
`DataFrame.plot` matplotlib bridge. The Go port implements roughly
{sym_compared} of those {total_symbols} inventoried names. **The port is a small
subset of pandas, and the percentage above is a percentage of the subset that
was compared, not of pandas.**

## Confirmed gaps and divergences

Each of these is backed by a failing case; none of the cases was shaped to avoid
the gap.

### Known gaps that were confirmed

| gap | evidence |
| --- | --- |
| CSV round-trip demotes `Float64` to `Int64` | `roundtrip-whole-floats`: a float column of whole values writes as `1,2,3` and reads back `int64`. pandas writes `1.0,2.0,3.0` and reads back `float64`. |
| `Loc` is type-exact and returns empty rather than erroring on a miss | `loc-untyped-int-label`: index labels are `int64`, so `Loc(int(1))` matches nothing and yields an empty frame. `loc-missing-label`: `Loc(0, 99)` silently returns just row 0 where pandas raises `KeyError`. |
| no index alignment for arithmetic | `series-add-length-mismatch`: pandas aligns on the index and pads to length 3 with NaN; `Series.Add` truncates to the shorter operand and returns length 1. |
| no pivot / melt / rolling / resample | `missing` rows for `pivot`, `pivot_table`, `melt`, `stack`, `unstack`, `explode`, `rolling`, `expanding`, `ewm`, `resample`, `asfreq`, `interpolate` below. Nothing to test against. |
| joins are inner and left only | `merge-right-unsupported`, `merge-outer-unsupported`, `merge-cross-unsupported` all succeed upstream and fail in the port. `DataFrame.join` (index-based) is `missing` entirely. |

### Further divergences the harness found

**dtype inference**

- `series-infer-mixed-numeric` — pandas promotes `[1, 2.5, 3]` to `float64`;
  `NewSeries` takes the dtype from the *first* element (`int64`) and silently
  truncates `2.5` to `2`. Data loss, not just a dtype label.
- `series-infer-mixed-types` — pandas gives `object` holding `1, "x", True`;
  the port gives `int64` `[1, NA, 1]`.

**astype**

- `astype-string-to-float-bad` — pandas raises `ValueError`; `Astype` silently
  produces NA.
- `astype-bool-to-int` — pandas gives `[1, 0, NA]`; `Astype(Int64)` on a Bool
  column gives `[NA, NA, NA]`. Bool-to-Int64 coercion is simply not implemented.

**CSV text**

- `to-csv-whole`, `to-csv-missing` — pandas writes float columns as `1.0`; the
  port writes `1`. Same root cause as the round-trip demotion.
- `to-csv-bool` — pandas writes `True`/`False`; the port writes `true`/`false`,
  so a port-written CSV of booleans does not round-trip through pandas as bool.
- `read-csv-no-header` — pandas names the columns `0`, `1`; the port names them
  `col0`, `col1`.
- `read-csv-ragged` — for `a,b` / `1,2,3` pandas silently promotes the extra
  leading field to the index; the port keeps the first two fields as data.
  Neither errors.

**Nullable integers vs pandas' NaN promotion**

`roundtrip-missing`, `roundtrip-sales`, `read-csv-blank-is-na`,
`read-csv-na-values` — an integer CSV column containing a blank is `int64` with
a validity mask in the port, but `float64` in pandas, because pandas' default
numpy path has no integer NA. This one is arguably the port being *better*
behaved, but it is still a difference a caller sees.

**groupby**

- `gb-sum`, `gb-multi-column-aggs`, `gb-all-na-group` — the sum of an all-NA
  group is `0` in pandas and NA in the port. Also the port's `_sum` output
  column is always `float64` where pandas keeps `int64`.
- `gb-bool-agg` — `sum` over an `int64` column: `int64` upstream, `float64` in
  the port.
- `gb-mean-of-string`, `gb-std-of-string` — the must-fail-on-both pair: pandas
  raises `TypeError: dtype 'string' does not support operation 'mean'`; the port
  returns an NA column. **The port does not fail where upstream does.**
- `gb-sum-of-string` — pandas concatenates (`"aa"+"cc" == "aacc"`); the port
  returns NA.
- `gb-default-naming` (marked `deviation`) — the port always names an
  aggregation output `<column>_<agg>`; pandas keeps the source column name. The
  other groupby cases use pandas *named aggregation* so this shows up exactly
  once instead of poisoning every case.

**merge**

- `merge-key-not-first-column`, `merge-key-not-first-column-left` — the port
  moves the join key to the front of the output; pandas preserves the left
  frame's column order and puts the key where it already was. Column *order*
  differs, values do not. (This is invisible when the key already happens to be
  the first left column, which is why a dedicated fixture exists for it.)
- Inner and left joins, right-key fan-out, and the `_left`/`_right` collision
  suffixes all match. pandas' default suffixes are `_x`/`_y`; the cases pass
  `suffixes=("_left","_right")` upstream so the *mechanism* is compared rather
  than the spelling. The spelling difference is real and recorded here.

**missing values**

- `fillna-float-into-int-column` — pandas raises for `1.5` into an `Int64`
  column; `FillNA` coerces it to `1`.
- `fillna-unknown-column` — pandas raises `KeyError`; `FillNA` silently returns
  the frame unchanged. `FillNA` has no error return, so this cannot be fixed
  without an API change.

**describe / reductions**

- `describe-nums`, `describe-sales` — the port emits only count/mean/std/min/max
  and puts the statistic name in a `stat` *column*; pandas also emits
  25%/50%/75% and uses the *index*. Different shape and different content.
- `describe-strings-only` — pandas falls back to count/unique/top/freq for a
  frame with no numeric columns; the port returns a frame with a lone `stat`
  column and no data columns.
- `frame-mean-mixed-dtypes` — pandas 2.x raises `TypeError` for a reduction over
  mixed dtypes unless `numeric_only=True`; the port silently drops non-numeric
  columns.
- `frame-reduction-result-name` — the port names a column-wise reduction result
  after the reduction (`"sum"`); the pandas result is unnamed. Compared once,
  by its own case, so the other eight `frame_stat` cases compare
  index/dtype/values alone.
- `frame-round` — `Round` promotes an `int64` column to `float64`; pandas leaves
  it `int64`.

**Series edge cases**

- `series-sum-empty` — pandas returns `0.0` for the sum of an empty Series (the
  additive identity); `Sum` reports "no value" and the harness encodes NA.
- `series-mean-of-strings` — the must-fail-on-both case for a non-numeric
  Series: pandas raises `TypeError`, the port returns NA.
- `series-div-by-zero` — pandas yields `+inf` / `-inf`; `Div` yields NA.
- `drop-unknown` — pandas raises `KeyError`; `Drop` ignores names it does not
  find (documented behaviour, but a divergence).
- `filter-unknown-column` — the must-fail-on-both case for boolean filtering:
  pandas raises `KeyError`, and `FilterFunc` cannot report anything, so an
  unknown column yields an empty frame.

### What matched, and is worth saying

Series and DataFrame construction and dtypes; label and positional selection
(`ILoc`, `Head`, `Tail`, `Take`, string-index `Loc`); boolean-mask and
predicate filtering including NA exclusion; `WithColumn` add and in-place
replace; `Drop`; `Rename`; single-, two- and three-key sorting with mixed
directions, stable ties and NA-last in both directions; `SetIndex`,
`ResetIndex`, `DropDuplicates`, `Concat` (including the column-union case);
every aggregation the port has (sum, mean, min, max, count, std) over one and
two group keys with NA in the data; inner and left merges including right-key
fan-out; `DropNA` and `FillNA`; `Quantile`, `Median`, `Var`, `Std`, `Prod`,
`Corr`, `Cov`, the full correlation matrix; `CumSum`/`CumMax`/`Diff`/
`PctChange`/`Shift`; `Rank` in average, min and dense modes; `Round`'s
round-half-to-even; `Clip`, `Abs`, `Sort`, `Unique`, `ValueCounts`, `Mode`,
`NLargest`, `NSmallest`, `IsIn`, `Between`; all nine `Str` accessor methods;
and `Transpose`. Index labels are preserved through filter, iloc, dropna and
sort exactly as pandas preserves them.

## Inventory

### `pandas.DataFrame` ({n_df} public names)

{df_table}

### `pandas.Series` ({n_sr} public names)

{sr_table}

### `pandas.Series.str` ({n_str} public names)

{str_table}

### `pandas.core.groupby.DataFrameGroupBy` ({n_gb} public names)

{gb_table}

### Module level

{mod_table}

### Dunder entry points (hidden by the `dir()` public filter, but tested)

{dunder_table}

### Go-only additions

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
{extra_table}
