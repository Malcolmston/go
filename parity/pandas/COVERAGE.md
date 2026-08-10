# pandas parity coverage

**Upstream oracle:** `pandas==2.3.3` (CPython 3.13, importable system install — no
virtualenv was needed; `python3 -c "import pandas"` already worked, so nothing
was installed for this harness).
**Go port under test:** `github.com/malcolmston/pandas v0.2.0` (consumed as a published module; no
`replace` directive).
**Float tolerance:** absolute-or-relative `1e-09` — two numbers match when
`abs(a-b) <= tol` or `abs(a-b)/max(abs(a),abs(b)) <= tol`. Every JSON number is
decoded as float64 on both sides, so `1` and `1.0` compare equal *as values*;
dtypes are compared separately, so an int-vs-float dtype difference is still
caught.
**Missing-value sentinel:** the string `"__NA__"` on both sides.
Non-finite floats encode as `"__INF__"`, `"__-INF__"`, `"__NAN__"`.

## Case score

| | count |
| --- | --- |
| cases run | 217 |
| match | 174 |
| differs | 42 |
| deliberate deviation (excluded from the score) | 1 |
| compared (match + differs) | 216 |
| **case parity** | **80.56%** = 174 / 216; the 1 deviation is excluded from the denominator |

Regenerate with `GOWORK=off go test ./parity/pandas/`, which rewrites
`parity.json`; then `python3 python/coverage.py` from `parity/pandas/` rewrites
this file.

## Symbol score

| status | count |
| --- | --- |
| match | 74 |
| differs | 19 |
| deviation | 0 |
| missing | 492 |
| untested | 35 |
| extra (Go-only) | 19 |
| **upstream symbols inventoried** | **620** |

**Symbol parity: 79.57%** = 74 match over the 93
symbols actually compared (match + differs + deviation). Symbols with no case
are `untested`, never `match`. The one case-level deviation
(`gb-default-naming`) rolls up into a symbol that also has failing cases, so it
appears as `differs` in the symbol table rather than as its own `deviation` row.

### How the inventory was derived

Mechanically, from the *installed* pandas — not from memory or the README:

```python
import pandas as pd
from pandas.core.groupby import DataFrameGroupBy
[n for n in dir(pd.DataFrame)               if not n.startswith("_")]   # 209
[n for n in dir(pd.Series)                  if not n.startswith("_")]   # 210
[n for n in dir(pd.Series.str)              if not n.startswith("_")]   # 56
[n for n in dir(DataFrameGroupBy)           if not n.startswith("_")]   # 66
[n for n in dir(pd)                         if not n.startswith("_")]   # module level
```

`python/coverage.py` runs exactly that and joins it against `parity.json`.
Module-level names were filtered to lowercase functions plus `DataFrame` and
`Series`; the ~55 exported *type* names (`Timestamp`, `Categorical`,
`IntervalIndex`, the whole `arrays`/`api`/`errors` surface, …) have no analogue
in the port and are not individually listed.

### Be clear about the denominator

pandas' real surface is very much larger than this table: `dir()` on the two
core classes alone is 209 + 210 names, and that excludes `pandas.Index`
and its ~20 subclasses, the `.dt`/`.cat`/`.sparse`/`.plot` accessors,
`pandas.api.*`, `pandas.io.*` (about 40 readers and writers), `pandas.testing`,
`ExtensionArray`, `MultiIndex`, `Timedelta`/`Period`/`Timestamp` arithmetic,
`pandas.eval`, resampling, rolling/expanding/EWM windows, `pivot`/`pivot_table`
/`melt`/`stack`/`unstack`/`explode`, `Styler`, and the whole
`DataFrame.plot` matplotlib bridge. The Go port implements roughly
93 of those 620 inventoried names. **The port is a small
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

### `pandas.DataFrame` (209 public names)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.DataFrame.T` | `DataFrame.Transpose` | untested | — |  |
| `pandas.DataFrame.abs` | `DataFrame.Abs` | match | `frame-abs` |  |
| `pandas.DataFrame.add` | — | missing | — |  |
| `pandas.DataFrame.add_prefix` | — | missing | — |  |
| `pandas.DataFrame.add_suffix` | — | missing | — |  |
| `pandas.DataFrame.agg` | — | missing | — |  |
| `pandas.DataFrame.aggregate` | — | missing | — |  |
| `pandas.DataFrame.align` | — | missing | — |  |
| `pandas.DataFrame.all` | — | missing | — |  |
| `pandas.DataFrame.any` | — | missing | — |  |
| `pandas.DataFrame.apply` | — | missing | — |  |
| `pandas.DataFrame.applymap` | — | missing | — |  |
| `pandas.DataFrame.asfreq` | — | missing | — |  |
| `pandas.DataFrame.asof` | — | missing | — |  |
| `pandas.DataFrame.assign` | `DataFrame.WithColumn` | match | `with-column-bad-length`, `with-column-new-float`, `with-column-new-string`, `with-column-replace` |  |
| `pandas.DataFrame.astype` | — | missing | — |  |
| `pandas.DataFrame.at` | — | missing | — |  |
| `pandas.DataFrame.at_time` | — | missing | — |  |
| `pandas.DataFrame.attrs` | — | missing | — |  |
| `pandas.DataFrame.axes` | — | missing | — |  |
| `pandas.DataFrame.backfill` | — | missing | — |  |
| `pandas.DataFrame.between_time` | — | missing | — |  |
| `pandas.DataFrame.bfill` | — | missing | — |  |
| `pandas.DataFrame.bool` | — | missing | — |  |
| `pandas.DataFrame.boxplot` | — | missing | — |  |
| `pandas.DataFrame.clip` | — | missing | — |  |
| `pandas.DataFrame.columns` | `DataFrame.Names` | match | `frame-names-sales` |  |
| `pandas.DataFrame.combine` | — | missing | — |  |
| `pandas.DataFrame.combine_first` | — | missing | — |  |
| `pandas.DataFrame.compare` | — | missing | — |  |
| `pandas.DataFrame.convert_dtypes` | — | missing | — |  |
| `pandas.DataFrame.copy` | `DataFrame.Copy` | untested | — |  |
| `pandas.DataFrame.corr` | `DataFrame.Corr` | match | `corr-matrix` |  |
| `pandas.DataFrame.corrwith` | — | missing | — |  |
| `pandas.DataFrame.count` | — | missing | — |  |
| `pandas.DataFrame.cov` | — | missing | — |  |
| `pandas.DataFrame.cummax` | — | missing | — |  |
| `pandas.DataFrame.cummin` | — | missing | — |  |
| `pandas.DataFrame.cumprod` | — | missing | — |  |
| `pandas.DataFrame.cumsum` | — | missing | — |  |
| `pandas.DataFrame.describe` | `DataFrame.Describe` | differs | `describe-nums`, `describe-sales`, `describe-strings-only` |  |
| `pandas.DataFrame.diff` | — | missing | — |  |
| `pandas.DataFrame.div` | — | missing | — |  |
| `pandas.DataFrame.divide` | — | missing | — |  |
| `pandas.DataFrame.dot` | — | missing | — |  |
| `pandas.DataFrame.drop` | `DataFrame.Drop` | differs | `drop-one`, `drop-two`, `drop-unknown` |  |
| `pandas.DataFrame.drop_duplicates` | `DataFrame.DropDuplicates` | match | `drop-duplicates` |  |
| `pandas.DataFrame.droplevel` | — | missing | — |  |
| `pandas.DataFrame.dropna` | `DataFrame.DropNA` | match | `dropna-gaps`, `dropna-nothing-missing`, `dropna-sales` |  |
| `pandas.DataFrame.dtypes` | `Series.DType` | match | `frame-dtypes-nums`, `frame-dtypes-sales` |  |
| `pandas.DataFrame.duplicated` | — | missing | — |  |
| `pandas.DataFrame.empty` | — | missing | — |  |
| `pandas.DataFrame.eq` | — | missing | — |  |
| `pandas.DataFrame.equals` | — | missing | — |  |
| `pandas.DataFrame.eval` | — | missing | — |  |
| `pandas.DataFrame.ewm` | — | missing | — |  |
| `pandas.DataFrame.expanding` | — | missing | — |  |
| `pandas.DataFrame.explode` | — | missing | — |  |
| `pandas.DataFrame.ffill` | — | missing | — |  |
| `pandas.DataFrame.fillna` | `DataFrame.FillNA` | differs | `fillna-all-columns-zero`, `fillna-bool-column`, `fillna-float-into-int-column`, `fillna-one-column-float`, `fillna-one-column-int`, `fillna-unknown-column` |  |
| `pandas.DataFrame.filter` | — | missing | — |  |
| `pandas.DataFrame.first` | — | missing | — |  |
| `pandas.DataFrame.first_valid_index` | — | missing | — |  |
| `pandas.DataFrame.flags` | — | missing | — |  |
| `pandas.DataFrame.floordiv` | — | missing | — |  |
| `pandas.DataFrame.from_dict` | `FromMap` | untested | — |  |
| `pandas.DataFrame.from_records` | `FromRecords` | untested | — |  |
| `pandas.DataFrame.ge` | — | missing | — |  |
| `pandas.DataFrame.get` | — | missing | — |  |
| `pandas.DataFrame.groupby` | `DataFrame.GroupBy` | match | `gb-groups-count`, `gb-groups-two-keys`, `gb-unknown-key` |  |
| `pandas.DataFrame.gt` | — | missing | — |  |
| `pandas.DataFrame.head` | `DataFrame.Head` | match | `head-3`, `head-over`, `head-zero` |  |
| `pandas.DataFrame.hist` | — | missing | — |  |
| `pandas.DataFrame.iat` | — | missing | — |  |
| `pandas.DataFrame.idxmax` | — | missing | — |  |
| `pandas.DataFrame.idxmin` | — | missing | — |  |
| `pandas.DataFrame.iloc` | `DataFrame.ILoc` | match | `iloc-clamped-high`, `iloc-empty-range`, `iloc-from-zero`, `iloc-middle` |  |
| `pandas.DataFrame.index` | `DataFrame.Index` | match | `frame-index-sales` |  |
| `pandas.DataFrame.infer_objects` | — | missing | — |  |
| `pandas.DataFrame.info` | — | missing | — |  |
| `pandas.DataFrame.insert` | — | missing | — |  |
| `pandas.DataFrame.interpolate` | — | missing | — |  |
| `pandas.DataFrame.isetitem` | — | missing | — |  |
| `pandas.DataFrame.isin` | — | missing | — |  |
| `pandas.DataFrame.isna` | — | missing | — |  |
| `pandas.DataFrame.isnull` | — | missing | — |  |
| `pandas.DataFrame.items` | — | missing | — |  |
| `pandas.DataFrame.iterrows` | — | missing | — |  |
| `pandas.DataFrame.itertuples` | — | missing | — |  |
| `pandas.DataFrame.join` | — | missing | — |  |
| `pandas.DataFrame.keys` | — | missing | — |  |
| `pandas.DataFrame.kurt` | — | missing | — |  |
| `pandas.DataFrame.kurtosis` | — | missing | — |  |
| `pandas.DataFrame.last` | — | missing | — |  |
| `pandas.DataFrame.last_valid_index` | — | missing | — |  |
| `pandas.DataFrame.le` | — | missing | — |  |
| `pandas.DataFrame.loc` | `DataFrame.Loc` | differs | `loc-int-labels`, `loc-missing-label`, `loc-reordered-labels`, `loc-string-index`, `loc-untyped-int-label` |  |
| `pandas.DataFrame.lt` | — | missing | — |  |
| `pandas.DataFrame.map` | — | missing | — |  |
| `pandas.DataFrame.mask` | — | missing | — |  |
| `pandas.DataFrame.max` | `DataFrame.Max` | match | `frame-max` |  |
| `pandas.DataFrame.mean` | `DataFrame.Mean` | differs | `frame-mean`, `frame-mean-mixed-dtypes` |  |
| `pandas.DataFrame.median` | `DataFrame.Median` | match | `frame-median` |  |
| `pandas.DataFrame.melt` | — | missing | — |  |
| `pandas.DataFrame.memory_usage` | — | missing | — |  |
| `pandas.DataFrame.merge` | `DataFrame.Merge` | differs | `merge-bad-key`, `merge-colliding-columns`, `merge-cross-unsupported`, `merge-inner`, `merge-inner-duplicate-right-keys`, `merge-key-not-first-column`, `merge-key-not-first-column-left`, `merge-left`, `merge-left-duplicate-right-keys`, `merge-no-overlap`, `merge-outer-unsupported`, `merge-right-unsupported` |  |
| `pandas.DataFrame.min` | `DataFrame.Min` | match | `frame-min` |  |
| `pandas.DataFrame.mod` | — | missing | — |  |
| `pandas.DataFrame.mode` | — | missing | — |  |
| `pandas.DataFrame.mul` | — | missing | — |  |
| `pandas.DataFrame.multiply` | — | missing | — |  |
| `pandas.DataFrame.ndim` | — | missing | — |  |
| `pandas.DataFrame.ne` | — | missing | — |  |
| `pandas.DataFrame.nlargest` | — | missing | — |  |
| `pandas.DataFrame.notna` | — | missing | — |  |
| `pandas.DataFrame.notnull` | — | missing | — |  |
| `pandas.DataFrame.nsmallest` | — | missing | — |  |
| `pandas.DataFrame.nunique` | `DataFrame.Nunique` | match | `frame-nunique` |  |
| `pandas.DataFrame.pad` | — | missing | — |  |
| `pandas.DataFrame.pct_change` | — | missing | — |  |
| `pandas.DataFrame.pipe` | — | missing | — |  |
| `pandas.DataFrame.pivot` | — | missing | — |  |
| `pandas.DataFrame.pivot_table` | — | missing | — |  |
| `pandas.DataFrame.plot` | — | missing | — |  |
| `pandas.DataFrame.pop` | — | missing | — |  |
| `pandas.DataFrame.pow` | — | missing | — |  |
| `pandas.DataFrame.prod` | — | missing | — |  |
| `pandas.DataFrame.product` | — | missing | — |  |
| `pandas.DataFrame.quantile` | — | missing | — |  |
| `pandas.DataFrame.query` | — | missing | — |  |
| `pandas.DataFrame.radd` | — | missing | — |  |
| `pandas.DataFrame.rank` | — | missing | — |  |
| `pandas.DataFrame.rdiv` | — | missing | — |  |
| `pandas.DataFrame.reindex` | — | missing | — |  |
| `pandas.DataFrame.reindex_like` | — | missing | — |  |
| `pandas.DataFrame.rename` | `DataFrame.Rename` | match | `rename-one`, `rename-two`, `rename-unknown` |  |
| `pandas.DataFrame.rename_axis` | — | missing | — |  |
| `pandas.DataFrame.reorder_levels` | — | missing | — |  |
| `pandas.DataFrame.replace` | — | missing | — |  |
| `pandas.DataFrame.resample` | — | missing | — |  |
| `pandas.DataFrame.reset_index` | `DataFrame.ResetIndex` | match | `reset-index-sorted` |  |
| `pandas.DataFrame.rfloordiv` | — | missing | — |  |
| `pandas.DataFrame.rmod` | — | missing | — |  |
| `pandas.DataFrame.rmul` | — | missing | — |  |
| `pandas.DataFrame.rolling` | — | missing | — |  |
| `pandas.DataFrame.round` | `DataFrame.Round` | differs | `frame-round` |  |
| `pandas.DataFrame.rpow` | — | missing | — |  |
| `pandas.DataFrame.rsub` | — | missing | — |  |
| `pandas.DataFrame.rtruediv` | — | missing | — |  |
| `pandas.DataFrame.sample` | — | missing | — |  |
| `pandas.DataFrame.select_dtypes` | — | missing | — |  |
| `pandas.DataFrame.sem` | — | missing | — |  |
| `pandas.DataFrame.set_axis` | — | missing | — |  |
| `pandas.DataFrame.set_flags` | — | missing | — |  |
| `pandas.DataFrame.set_index` | `DataFrame.SetIndex` | match | `set-index-region`, `set-index-unknown` |  |
| `pandas.DataFrame.shape` | `DataFrame.Shape` | match | `frame-shape-sales` |  |
| `pandas.DataFrame.shift` | — | missing | — |  |
| `pandas.DataFrame.size` | — | missing | — |  |
| `pandas.DataFrame.skew` | — | missing | — |  |
| `pandas.DataFrame.sort_index` | — | missing | — |  |
| `pandas.DataFrame.sort_values` | `DataFrame.SortBy` | match | `sort-bool-key`, `sort-one-asc`, `sort-one-desc`, `sort-stable-ties`, `sort-string-key`, `sort-three-keys`, `sort-two-keys`, `sort-two-keys-mixed`, `sort-unknown-key` |  |
| `pandas.DataFrame.sparse` | — | missing | — |  |
| `pandas.DataFrame.squeeze` | — | missing | — |  |
| `pandas.DataFrame.stack` | — | missing | — |  |
| `pandas.DataFrame.std` | `DataFrame.Std` | match | `frame-std` |  |
| `pandas.DataFrame.style` | — | missing | — |  |
| `pandas.DataFrame.sub` | — | missing | — |  |
| `pandas.DataFrame.subtract` | — | missing | — |  |
| `pandas.DataFrame.sum` | `DataFrame.Sum` | differs | `frame-reduction-result-name`, `frame-sum` |  |
| `pandas.DataFrame.swapaxes` | — | missing | — |  |
| `pandas.DataFrame.swaplevel` | — | missing | — |  |
| `pandas.DataFrame.tail` | `DataFrame.Tail` | match | `tail-2`, `tail-over` |  |
| `pandas.DataFrame.take` | `DataFrame.Take` | match | `take-positions`, `take-repeat` |  |
| `pandas.DataFrame.to_clipboard` | — | missing | — |  |
| `pandas.DataFrame.to_csv` | `DataFrame.WriteCSV` | differs | `to-csv-bool`, `to-csv-fractional`, `to-csv-missing`, `to-csv-whole` |  |
| `pandas.DataFrame.to_dict` | — | missing | — |  |
| `pandas.DataFrame.to_excel` | — | missing | — |  |
| `pandas.DataFrame.to_feather` | — | missing | — |  |
| `pandas.DataFrame.to_gbq` | — | missing | — |  |
| `pandas.DataFrame.to_hdf` | — | missing | — |  |
| `pandas.DataFrame.to_html` | — | missing | — |  |
| `pandas.DataFrame.to_json` | — | missing | — |  |
| `pandas.DataFrame.to_latex` | — | missing | — |  |
| `pandas.DataFrame.to_markdown` | — | missing | — |  |
| `pandas.DataFrame.to_numpy` | — | missing | — |  |
| `pandas.DataFrame.to_orc` | — | missing | — |  |
| `pandas.DataFrame.to_parquet` | — | missing | — |  |
| `pandas.DataFrame.to_period` | — | missing | — |  |
| `pandas.DataFrame.to_pickle` | — | missing | — |  |
| `pandas.DataFrame.to_records` | — | missing | — |  |
| `pandas.DataFrame.to_sql` | — | missing | — |  |
| `pandas.DataFrame.to_stata` | — | missing | — |  |
| `pandas.DataFrame.to_string` | `DataFrame.String` | untested | — |  |
| `pandas.DataFrame.to_timestamp` | — | missing | — |  |
| `pandas.DataFrame.to_xarray` | — | missing | — |  |
| `pandas.DataFrame.to_xml` | — | missing | — |  |
| `pandas.DataFrame.transform` | — | missing | — |  |
| `pandas.DataFrame.transpose` | `DataFrame.Transpose` | match | `frame-transpose` |  |
| `pandas.DataFrame.truediv` | — | missing | — |  |
| `pandas.DataFrame.truncate` | — | missing | — |  |
| `pandas.DataFrame.tz_convert` | — | missing | — |  |
| `pandas.DataFrame.tz_localize` | — | missing | — |  |
| `pandas.DataFrame.unstack` | — | missing | — |  |
| `pandas.DataFrame.update` | — | missing | — |  |
| `pandas.DataFrame.value_counts` | — | missing | — |  |
| `pandas.DataFrame.values` | — | missing | — |  |
| `pandas.DataFrame.var` | `DataFrame.Var` | match | `frame-var` |  |
| `pandas.DataFrame.where` | — | missing | — |  |
| `pandas.DataFrame.xs` | — | missing | — |  |

### `pandas.Series` (210 public names)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.Series.T` | — | missing | — |  |
| `pandas.Series.abs` | `Series.Abs` | match | `series-abs` |  |
| `pandas.Series.add` | `Series.Add` | differs | `series-add`, `series-add-length-mismatch` |  |
| `pandas.Series.add_prefix` | — | missing | — |  |
| `pandas.Series.add_suffix` | — | missing | — |  |
| `pandas.Series.agg` | — | missing | — |  |
| `pandas.Series.aggregate` | — | missing | — |  |
| `pandas.Series.align` | — | missing | — |  |
| `pandas.Series.all` | — | missing | — |  |
| `pandas.Series.any` | — | missing | — |  |
| `pandas.Series.apply` | `Series.Apply` | untested | — |  |
| `pandas.Series.argmax` | `Series.ArgMax` | untested | — |  |
| `pandas.Series.argmin` | `Series.ArgMin` | untested | — |  |
| `pandas.Series.argsort` | — | missing | — |  |
| `pandas.Series.array` | — | missing | — |  |
| `pandas.Series.asfreq` | — | missing | — |  |
| `pandas.Series.asof` | — | missing | — |  |
| `pandas.Series.astype` | `Series.Astype` | differs | `astype-bool-to-int`, `astype-float-to-int`, `astype-int-to-float`, `astype-int-to-string`, `astype-string-to-float-bad` |  |
| `pandas.Series.at` | — | missing | — |  |
| `pandas.Series.at_time` | — | missing | — |  |
| `pandas.Series.attrs` | — | missing | — |  |
| `pandas.Series.autocorr` | — | missing | — |  |
| `pandas.Series.axes` | — | missing | — |  |
| `pandas.Series.backfill` | — | missing | — |  |
| `pandas.Series.between` | `Series.Between` | match | `series-between` |  |
| `pandas.Series.between_time` | — | missing | — |  |
| `pandas.Series.bfill` | — | missing | — |  |
| `pandas.Series.bool` | — | missing | — |  |
| `pandas.Series.case_when` | — | missing | — |  |
| `pandas.Series.cat` | — | missing | — |  |
| `pandas.Series.clip` | `Series.Clip` | match | `series-clip` |  |
| `pandas.Series.combine` | — | missing | — |  |
| `pandas.Series.combine_first` | — | missing | — |  |
| `pandas.Series.compare` | — | missing | — |  |
| `pandas.Series.convert_dtypes` | — | missing | — |  |
| `pandas.Series.copy` | `Series.Copy` | untested | — |  |
| `pandas.Series.corr` | `Series.Corr` | match | `series-corr` |  |
| `pandas.Series.count` | `Series.Count` | match | `series-count` |  |
| `pandas.Series.cov` | `Series.Cov` | match | `series-cov` |  |
| `pandas.Series.cummax` | `Series.CumMax` | match | `series-cummax` |  |
| `pandas.Series.cummin` | `Series.CumMin` | untested | — |  |
| `pandas.Series.cumprod` | `Series.CumProd` | untested | — |  |
| `pandas.Series.cumsum` | `Series.CumSum` | match | `series-cumsum` |  |
| `pandas.Series.describe` | — | missing | — |  |
| `pandas.Series.diff` | `Series.Diff` | match | `series-diff` |  |
| `pandas.Series.div` | `Series.Div` | differs | `series-div`, `series-div-by-zero` |  |
| `pandas.Series.divide` | `Series.Div` | untested | — |  |
| `pandas.Series.divmod` | — | missing | — |  |
| `pandas.Series.dot` | — | missing | — |  |
| `pandas.Series.drop` | — | missing | — |  |
| `pandas.Series.drop_duplicates` | — | missing | — |  |
| `pandas.Series.droplevel` | — | missing | — |  |
| `pandas.Series.dropna` | `Series.DropNA` | match | `series-dropna` |  |
| `pandas.Series.dt` | — | missing | — |  |
| `pandas.Series.dtype` | `Series.DType` | untested | — |  |
| `pandas.Series.dtypes` | `Series.DType` | untested | — |  |
| `pandas.Series.duplicated` | — | missing | — |  |
| `pandas.Series.empty` | — | missing | — |  |
| `pandas.Series.eq` | — | missing | — |  |
| `pandas.Series.equals` | — | missing | — |  |
| `pandas.Series.ewm` | — | missing | — |  |
| `pandas.Series.expanding` | — | missing | — |  |
| `pandas.Series.explode` | — | missing | — |  |
| `pandas.Series.factorize` | — | missing | — |  |
| `pandas.Series.ffill` | — | missing | — |  |
| `pandas.Series.fillna` | `Series.FillNA` | match | `series-fillna`, `series-fillna-string` |  |
| `pandas.Series.filter` | — | missing | — |  |
| `pandas.Series.first` | — | missing | — |  |
| `pandas.Series.first_valid_index` | — | missing | — |  |
| `pandas.Series.flags` | — | missing | — |  |
| `pandas.Series.floordiv` | — | missing | — |  |
| `pandas.Series.ge` | — | missing | — |  |
| `pandas.Series.get` | — | missing | — |  |
| `pandas.Series.groupby` | — | missing | — |  |
| `pandas.Series.gt` | — | missing | — |  |
| `pandas.Series.hasnans` | — | missing | — |  |
| `pandas.Series.head` | `Series.Head` | untested | — |  |
| `pandas.Series.hist` | — | missing | — |  |
| `pandas.Series.iat` | — | missing | — |  |
| `pandas.Series.idxmax` | — | missing | — |  |
| `pandas.Series.idxmin` | — | missing | — |  |
| `pandas.Series.iloc` | — | missing | — |  |
| `pandas.Series.index` | `Series.Index` | untested | — |  |
| `pandas.Series.infer_objects` | — | missing | — |  |
| `pandas.Series.info` | — | missing | — |  |
| `pandas.Series.interpolate` | — | missing | — |  |
| `pandas.Series.is_monotonic_decreasing` | — | missing | — |  |
| `pandas.Series.is_monotonic_increasing` | — | missing | — |  |
| `pandas.Series.is_unique` | — | missing | — |  |
| `pandas.Series.isin` | `Series.IsIn` | match | `series-isin` |  |
| `pandas.Series.isna` | `Series.IsNA` | match | `isna-mask` |  |
| `pandas.Series.isnull` | `Series.IsNA` | untested | — |  |
| `pandas.Series.item` | — | missing | — |  |
| `pandas.Series.items` | — | missing | — |  |
| `pandas.Series.keys` | — | missing | — |  |
| `pandas.Series.kurt` | — | missing | — |  |
| `pandas.Series.kurtosis` | — | missing | — |  |
| `pandas.Series.last` | — | missing | — |  |
| `pandas.Series.last_valid_index` | — | missing | — |  |
| `pandas.Series.le` | — | missing | — |  |
| `pandas.Series.list` | — | missing | — |  |
| `pandas.Series.loc` | — | missing | — |  |
| `pandas.Series.lt` | — | missing | — |  |
| `pandas.Series.map` | `Series.Map` | untested | — |  |
| `pandas.Series.mask` | — | missing | — |  |
| `pandas.Series.max` | `Series.Max` | match | `series-max` |  |
| `pandas.Series.mean` | `Series.Mean` | differs | `series-mean`, `series-mean-empty`, `series-mean-of-strings` |  |
| `pandas.Series.median` | `Series.Median` | match | `series-median-even` |  |
| `pandas.Series.memory_usage` | — | missing | — |  |
| `pandas.Series.min` | `Series.Min` | match | `series-min` |  |
| `pandas.Series.mod` | — | missing | — |  |
| `pandas.Series.mode` | `Series.Mode` | match | `series-mode` |  |
| `pandas.Series.mul` | `Series.Mul` | match | `series-mul` |  |
| `pandas.Series.multiply` | `Series.Mul` | untested | — |  |
| `pandas.Series.name` | `Series.Name` | untested | — |  |
| `pandas.Series.nbytes` | — | missing | — |  |
| `pandas.Series.ndim` | — | missing | — |  |
| `pandas.Series.ne` | — | missing | — |  |
| `pandas.Series.nlargest` | `Series.NLargest` | match | `series-nlargest` |  |
| `pandas.Series.notna` | — | missing | — |  |
| `pandas.Series.notnull` | — | missing | — |  |
| `pandas.Series.nsmallest` | `Series.NSmallest` | match | `series-nsmallest` |  |
| `pandas.Series.nunique` | `Series.Unique (len)` | match | `series-nunique` |  |
| `pandas.Series.pad` | — | missing | — |  |
| `pandas.Series.pct_change` | `Series.PctChange` | match | `series-pct-change` |  |
| `pandas.Series.pipe` | — | missing | — |  |
| `pandas.Series.plot` | — | missing | — |  |
| `pandas.Series.pop` | — | missing | — |  |
| `pandas.Series.pow` | — | missing | — |  |
| `pandas.Series.prod` | `Series.Prod` | match | `series-prod` |  |
| `pandas.Series.product` | `Series.Prod` | untested | — |  |
| `pandas.Series.quantile` | `Series.Quantile` | match | `series-quantile-25`, `series-quantile-90` |  |
| `pandas.Series.radd` | — | missing | — |  |
| `pandas.Series.rank` | `Series.Rank / Series.RankBy` | match | `series-rank-average`, `series-rank-dense`, `series-rank-min` |  |
| `pandas.Series.ravel` | — | missing | — |  |
| `pandas.Series.rdiv` | — | missing | — |  |
| `pandas.Series.rdivmod` | — | missing | — |  |
| `pandas.Series.reindex` | — | missing | — |  |
| `pandas.Series.reindex_like` | — | missing | — |  |
| `pandas.Series.rename` | `Series.Rename` | untested | — |  |
| `pandas.Series.rename_axis` | — | missing | — |  |
| `pandas.Series.reorder_levels` | — | missing | — |  |
| `pandas.Series.repeat` | — | missing | — |  |
| `pandas.Series.replace` | — | missing | — |  |
| `pandas.Series.resample` | — | missing | — |  |
| `pandas.Series.reset_index` | — | missing | — |  |
| `pandas.Series.rfloordiv` | — | missing | — |  |
| `pandas.Series.rmod` | — | missing | — |  |
| `pandas.Series.rmul` | — | missing | — |  |
| `pandas.Series.rolling` | — | missing | — |  |
| `pandas.Series.round` | `Series.Round` | match | `series-round-half-even` |  |
| `pandas.Series.rpow` | — | missing | — |  |
| `pandas.Series.rsub` | — | missing | — |  |
| `pandas.Series.rtruediv` | — | missing | — |  |
| `pandas.Series.sample` | — | missing | — |  |
| `pandas.Series.searchsorted` | — | missing | — |  |
| `pandas.Series.sem` | — | missing | — |  |
| `pandas.Series.set_axis` | — | missing | — |  |
| `pandas.Series.set_flags` | — | missing | — |  |
| `pandas.Series.shape` | — | missing | — |  |
| `pandas.Series.shift` | `Series.Shift` | match | `series-shift-down`, `series-shift-up` |  |
| `pandas.Series.size` | `Series.Len` | untested | — |  |
| `pandas.Series.skew` | — | missing | — |  |
| `pandas.Series.sort_index` | — | missing | — |  |
| `pandas.Series.sort_values` | `Series.Sort` | match | `series-sort-asc`, `series-sort-desc` |  |
| `pandas.Series.sparse` | — | missing | — |  |
| `pandas.Series.squeeze` | — | missing | — |  |
| `pandas.Series.std` | `Series.Std` | match | `series-std`, `series-std-single` |  |
| `pandas.Series.str` | `Series.Str` | untested | — |  |
| `pandas.Series.struct` | — | missing | — |  |
| `pandas.Series.sub` | `Series.Sub` | match | `series-sub` |  |
| `pandas.Series.subtract` | `Series.Sub` | untested | — |  |
| `pandas.Series.sum` | `Series.Sum` | differs | `series-sum`, `series-sum-empty` |  |
| `pandas.Series.swapaxes` | — | missing | — |  |
| `pandas.Series.swaplevel` | — | missing | — |  |
| `pandas.Series.tail` | `Series.Tail` | untested | — |  |
| `pandas.Series.take` | `Series.Take` | untested | — |  |
| `pandas.Series.to_clipboard` | — | missing | — |  |
| `pandas.Series.to_csv` | — | missing | — |  |
| `pandas.Series.to_dict` | — | missing | — |  |
| `pandas.Series.to_excel` | — | missing | — |  |
| `pandas.Series.to_frame` | — | missing | — |  |
| `pandas.Series.to_hdf` | — | missing | — |  |
| `pandas.Series.to_json` | — | missing | — |  |
| `pandas.Series.to_latex` | — | missing | — |  |
| `pandas.Series.to_list` | `Series.Values` | untested | — |  |
| `pandas.Series.to_markdown` | — | missing | — |  |
| `pandas.Series.to_numpy` | — | missing | — |  |
| `pandas.Series.to_period` | — | missing | — |  |
| `pandas.Series.to_pickle` | — | missing | — |  |
| `pandas.Series.to_sql` | — | missing | — |  |
| `pandas.Series.to_string` | `Series.String` | untested | — |  |
| `pandas.Series.to_timestamp` | — | missing | — |  |
| `pandas.Series.to_xarray` | — | missing | — |  |
| `pandas.Series.tolist` | `Series.Values` | untested | — |  |
| `pandas.Series.transform` | — | missing | — |  |
| `pandas.Series.transpose` | — | missing | — |  |
| `pandas.Series.truediv` | `Series.Div` | untested | — |  |
| `pandas.Series.truncate` | — | missing | — |  |
| `pandas.Series.tz_convert` | — | missing | — |  |
| `pandas.Series.tz_localize` | — | missing | — |  |
| `pandas.Series.unique` | `Series.Unique` | match | `series-unique` |  |
| `pandas.Series.unstack` | — | missing | — |  |
| `pandas.Series.update` | — | missing | — |  |
| `pandas.Series.value_counts` | `Series.ValueCounts` | match | `series-value-counts` |  |
| `pandas.Series.values` | `Series.Values` | untested | — |  |
| `pandas.Series.var` | `Series.Var` | match | `series-var` |  |
| `pandas.Series.view` | — | missing | — |  |
| `pandas.Series.where` | — | missing | — |  |
| `pandas.Series.xs` | — | missing | — |  |

### `pandas.Series.str` (56 public names)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.Series.str.capitalize` | — | missing | — |  |
| `pandas.Series.str.casefold` | — | missing | — |  |
| `pandas.Series.str.cat` | — | missing | — |  |
| `pandas.Series.str.center` | — | missing | — |  |
| `pandas.Series.str.contains` | `StrAccessor.Contains` | match | `str-contains` |  |
| `pandas.Series.str.count` | — | missing | — |  |
| `pandas.Series.str.decode` | — | missing | — |  |
| `pandas.Series.str.encode` | — | missing | — |  |
| `pandas.Series.str.endswith` | `StrAccessor.EndsWith` | match | `str-endswith` |  |
| `pandas.Series.str.extract` | — | missing | — |  |
| `pandas.Series.str.extractall` | — | missing | — |  |
| `pandas.Series.str.find` | — | missing | — |  |
| `pandas.Series.str.findall` | — | missing | — |  |
| `pandas.Series.str.fullmatch` | — | missing | — |  |
| `pandas.Series.str.get` | — | missing | — |  |
| `pandas.Series.str.get_dummies` | — | missing | — |  |
| `pandas.Series.str.index` | — | missing | — |  |
| `pandas.Series.str.isalnum` | — | missing | — |  |
| `pandas.Series.str.isalpha` | — | missing | — |  |
| `pandas.Series.str.isdecimal` | — | missing | — |  |
| `pandas.Series.str.isdigit` | — | missing | — |  |
| `pandas.Series.str.islower` | — | missing | — |  |
| `pandas.Series.str.isnumeric` | — | missing | — |  |
| `pandas.Series.str.isspace` | — | missing | — |  |
| `pandas.Series.str.istitle` | — | missing | — |  |
| `pandas.Series.str.isupper` | — | missing | — |  |
| `pandas.Series.str.join` | — | missing | — |  |
| `pandas.Series.str.len` | `StrAccessor.Len` | match | `str-len` |  |
| `pandas.Series.str.ljust` | — | missing | — |  |
| `pandas.Series.str.lower` | `StrAccessor.Lower` | match | `str-lower` |  |
| `pandas.Series.str.lstrip` | — | missing | — |  |
| `pandas.Series.str.match` | — | missing | — |  |
| `pandas.Series.str.normalize` | — | missing | — |  |
| `pandas.Series.str.pad` | — | missing | — |  |
| `pandas.Series.str.partition` | — | missing | — |  |
| `pandas.Series.str.removeprefix` | — | missing | — |  |
| `pandas.Series.str.removesuffix` | — | missing | — |  |
| `pandas.Series.str.repeat` | — | missing | — |  |
| `pandas.Series.str.replace` | `StrAccessor.Replace` | match | `str-replace` |  |
| `pandas.Series.str.rfind` | — | missing | — |  |
| `pandas.Series.str.rindex` | — | missing | — |  |
| `pandas.Series.str.rjust` | — | missing | — |  |
| `pandas.Series.str.rpartition` | — | missing | — |  |
| `pandas.Series.str.rsplit` | — | missing | — |  |
| `pandas.Series.str.rstrip` | — | missing | — |  |
| `pandas.Series.str.slice` | — | missing | — |  |
| `pandas.Series.str.slice_replace` | — | missing | — |  |
| `pandas.Series.str.split` | — | missing | — |  |
| `pandas.Series.str.startswith` | `StrAccessor.StartsWith` | match | `str-startswith` |  |
| `pandas.Series.str.strip` | `StrAccessor.Strip` | match | `str-strip` |  |
| `pandas.Series.str.swapcase` | — | missing | — |  |
| `pandas.Series.str.title` | `StrAccessor.Title` | match | `str-title` |  |
| `pandas.Series.str.translate` | — | missing | — |  |
| `pandas.Series.str.upper` | `StrAccessor.Upper` | match | `str-upper` |  |
| `pandas.Series.str.wrap` | — | missing | — |  |
| `pandas.Series.str.zfill` | — | missing | — |  |

### `pandas.core.groupby.DataFrameGroupBy` (66 public names)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.core.groupby.DataFrameGroupBy.agg` | `GroupBy.Agg` | differs | `gb-all-aggs`, `gb-all-na-group`, `gb-bool-agg`, `gb-mean-of-string`, `gb-multi-column-aggs`, `gb-std-of-string`, `gb-sum-of-string`, `gb-two-keys-mean`, `gb-two-keys-sum-count`, `gb-unknown-agg-column` |  |
| `pandas.core.groupby.DataFrameGroupBy.aggregate` | `GroupBy.Agg` | untested | — |  |
| `pandas.core.groupby.DataFrameGroupBy.all` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.any` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.apply` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.bfill` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.boxplot` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.corr` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.corrwith` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.count` | `GroupBy.Count` | match | `gb-count` |  |
| `pandas.core.groupby.DataFrameGroupBy.cov` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.cumcount` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.cummax` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.cummin` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.cumprod` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.cumsum` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.describe` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.diff` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.dtypes` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.ewm` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.expanding` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.ffill` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.fillna` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.filter` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.first` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.get_group` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.grouper` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.groups` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.head` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.hist` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.idxmax` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.idxmin` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.indices` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.keys` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.last` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.level` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.max` | `GroupBy.Max` | match | `gb-max` |  |
| `pandas.core.groupby.DataFrameGroupBy.mean` | `GroupBy.Mean` | match | `gb-mean` |  |
| `pandas.core.groupby.DataFrameGroupBy.median` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.min` | `GroupBy.Min` | match | `gb-min` |  |
| `pandas.core.groupby.DataFrameGroupBy.ndim` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.ngroup` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.ngroups` | `GroupBy.Groups` | untested | — |  |
| `pandas.core.groupby.DataFrameGroupBy.nth` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.nunique` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.ohlc` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.pct_change` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.pipe` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.plot` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.prod` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.quantile` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.rank` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.resample` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.rolling` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.sample` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.sem` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.shift` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.size` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.skew` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.std` | `GroupBy.Std` | match | `gb-std` |  |
| `pandas.core.groupby.DataFrameGroupBy.sum` | `GroupBy.Sum` | differs | `gb-default-naming`, `gb-sum` |  |
| `pandas.core.groupby.DataFrameGroupBy.tail` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.take` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.transform` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.value_counts` | — | missing | — |  |
| `pandas.core.groupby.DataFrameGroupBy.var` | — | missing | — |  |

### Module level

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.DataFrame` | `NewDataFrame` | match | `frame-from-map-order`, `frame-from-map-partial-order`, `frame-gaps`, `frame-nums`, `frame-sales` |  |
| `pandas.Series` | `NewSeries / NewSeriesTyped` | differs | `series-allnull`, `series-bool`, `series-empty`, `series-float64`, `series-infer-floats`, `series-infer-ints`, `series-infer-mixed-numeric`, `series-infer-mixed-types`, `series-infer-strings`, `series-int64`, `series-string` |  |
| `pandas.annotations` | — | missing | — |  |
| `pandas.api` | — | missing | — |  |
| `pandas.array` | — | missing | — |  |
| `pandas.arrays` | — | missing | — |  |
| `pandas.bdate_range` | — | missing | — |  |
| `pandas.compat` | — | missing | — |  |
| `pandas.concat` | `Concat` | match | `concat-different-columns`, `concat-self` |  |
| `pandas.core` | — | missing | — |  |
| `pandas.crosstab` | — | missing | — |  |
| `pandas.cut` | — | missing | — |  |
| `pandas.date_range` | — | missing | — |  |
| `pandas.describe_option` | — | missing | — |  |
| `pandas.errors` | — | missing | — |  |
| `pandas.eval` | — | missing | — |  |
| `pandas.factorize` | — | missing | — |  |
| `pandas.from_dummies` | — | missing | — |  |
| `pandas.get_dummies` | — | missing | — |  |
| `pandas.get_option` | — | missing | — |  |
| `pandas.infer_freq` | — | missing | — |  |
| `pandas.interval_range` | — | missing | — |  |
| `pandas.io` | — | missing | — |  |
| `pandas.isna` | — | missing | — |  |
| `pandas.isnull` | — | missing | — |  |
| `pandas.json_normalize` | — | missing | — |  |
| `pandas.lreshape` | — | missing | — |  |
| `pandas.melt` | — | missing | — |  |
| `pandas.merge` | `DataFrame.Merge` | untested | — |  |
| `pandas.merge_asof` | — | missing | — |  |
| `pandas.merge_ordered` | — | missing | — |  |
| `pandas.notna` | — | missing | — |  |
| `pandas.notnull` | — | missing | — |  |
| `pandas.offsets` | — | missing | — |  |
| `pandas.option_context` | — | missing | — |  |
| `pandas.options` | — | missing | — |  |
| `pandas.pandas` | — | missing | — |  |
| `pandas.period_range` | — | missing | — |  |
| `pandas.pivot` | — | missing | — |  |
| `pandas.pivot_table` | — | missing | — |  |
| `pandas.plotting` | — | missing | — |  |
| `pandas.qcut` | — | missing | — |  |
| `pandas.read_clipboard` | — | missing | — |  |
| `pandas.read_csv` | `ReadCSV / ReadCSVFile` | differs | `read-csv-blank-is-na`, `read-csv-inference`, `read-csv-mixed-column`, `read-csv-na-values`, `read-csv-no-header`, `read-csv-quoted`, `read-csv-ragged`, `read-csv-semicolon`, `roundtrip-bool`, `roundtrip-fractional`, `roundtrip-missing`, `roundtrip-sales`, `roundtrip-whole-floats` |  |
| `pandas.read_excel` | — | missing | — |  |
| `pandas.read_feather` | — | missing | — |  |
| `pandas.read_fwf` | — | missing | — |  |
| `pandas.read_gbq` | — | missing | — |  |
| `pandas.read_hdf` | — | missing | — |  |
| `pandas.read_html` | — | missing | — |  |
| `pandas.read_json` | — | missing | — |  |
| `pandas.read_orc` | — | missing | — |  |
| `pandas.read_parquet` | — | missing | — |  |
| `pandas.read_pickle` | — | missing | — |  |
| `pandas.read_sas` | — | missing | — |  |
| `pandas.read_spss` | — | missing | — |  |
| `pandas.read_sql` | — | missing | — |  |
| `pandas.read_sql_query` | — | missing | — |  |
| `pandas.read_sql_table` | — | missing | — |  |
| `pandas.read_stata` | — | missing | — |  |
| `pandas.read_table` | — | missing | — |  |
| `pandas.read_xml` | — | missing | — |  |
| `pandas.reset_option` | — | missing | — |  |
| `pandas.set_eng_float_format` | — | missing | — |  |
| `pandas.set_option` | — | missing | — |  |
| `pandas.show_versions` | — | missing | — |  |
| `pandas.test` | — | missing | — |  |
| `pandas.testing` | — | missing | — |  |
| `pandas.timedelta_range` | — | missing | — |  |
| `pandas.to_datetime` | — | missing | — |  |
| `pandas.to_numeric` | — | missing | — |  |
| `pandas.to_pickle` | — | missing | — |  |
| `pandas.to_timedelta` | — | missing | — |  |
| `pandas.tseries` | — | missing | — |  |
| `pandas.unique` | — | missing | — |  |
| `pandas.util` | — | missing | — |  |
| `pandas.value_counts` | — | missing | — |  |
| `pandas.wide_to_long` | — | missing | — |  |

### Dunder entry points (hidden by the `dir()` public filter, but tested)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `pandas.DataFrame.__getitem__` | `DataFrame.Select / Col / Filter / FilterFunc` | differs | `col-numeric`, `col-string`, `col-unknown`, `filter-eq-bool`, `filter-eq-string`, `filter-gt-int`, `filter-le-float`, `filter-mask`, `filter-mask-all`, `filter-mask-none`, `filter-mask-wrong-length`, `filter-ne-string`, `filter-unknown-column`, `select-all`, `select-reordered`, `select-two`, `select-unknown` |  |

### Go-only additions

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `pandas.DataFrame.Col` | extra | — | typed column lookup returning (col, ok) |
| — | `pandas.DataFrame.MustCol` | extra | — | panicking column lookup |
| — | `pandas.DataFrame.HasColumn` | extra | — | column-existence predicate |
| — | `pandas.DataFrame.NumRows` | extra | — | row count (pandas: len(df)) |
| — | `pandas.DataFrame.NumCols` | extra | — | column count (pandas: len(df.columns)) |
| — | `pandas.DataFrame.Filter` | extra | — | boolean-mask row filter (pandas: df[mask]) |
| — | `pandas.DataFrame.FilterFunc` | extra | — | row-predicate filter, no pandas analogue |
| — | `pandas.DataFrame.Row` | extra | — | keyed read-only row view |
| — | `pandas.DataFrame.WithColumn` | extra | — | add-or-replace one column |
| — | `pandas.DataFrame.Select` | extra | — | ordered multi-column projection |
| — | `pandas.DataFrame.WriteCSVFile` | extra | — | path-taking CSV writer |
| — | `pandas.ReadCSVFile` | extra | — | path-taking CSV reader |
| — | `pandas.Series.At` | extra | — | positional element access returning (v, present) |
| — | `pandas.Series.Filter` | extra | — | boolean-mask element filter |
| — | `pandas.Series.RankBy` | extra | — | rank with an explicit tie method |
| — | `pandas.Series.NewSeriesTyped` | extra | — | construction with an explicit dtype |
| — | `pandas.Row.Get / Row.Float / Row.Label` | extra | — | row accessors |
| — | `pandas.DType / pandas.AggFunc / pandas.JoinType` | extra | — | enum types |
| — | `pandas.ReadCSVOptions` | extra | — | CSV parse options struct |
