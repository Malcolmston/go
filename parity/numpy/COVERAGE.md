# COVERAGE — `numpy` (Python) vs `github.com/malcolmston/numpy` (Go)

| | |
| --- | --- |
| upstream | `numpy@2.2.6` (pinned by recording `numpy.__version__`; resolved via `python3` 3.13.5 at `/opt/anaconda3/bin/python3`. Nothing was installed and no venv was created; `python/requirements.txt` records the pin. Note that `/usr/bin/python3` has no numpy — the harness `t.Skip`s rather than failing when `import numpy` fails.) |
| Go module | `github.com/malcolmston/numpy v0.0.0-20260810111555-4476c82e3a95` (published module, no `replace` directive) |
| runners | `python/run.py`, `go/run.go` — JSON Lines on stdio |
| cases | 301 across 10 case files |
| harness | `GOWORK=off go test ./parity/numpy/` |

## How the upstream inventory was derived

Mechanically, from the installed package — not from the README and not from memory:

```
python3 -c "import numpy as np; print([n for n in dir(np) if not n.startswith('_') and callable(getattr(np, n))])"   # 462 public callables
python3 -c "import numpy as np; print([n for n in dir(np.ndarray) if not n.startswith('_')])"                        # 74 public ndarray attributes
```

That is **536 public symbols** in the two surfaces, and it still undercounts numpy:
it excludes the submodules (`numpy.linalg`, `numpy.random`, `numpy.fft`, `numpy.ma`,
`numpy.polynomial`, `numpy.strings`, `numpy.rec`, `numpy.char`, `numpy.testing`, …),
every dunder operator on `ndarray` (91 callable dunders, of which this harness exercises
only `__getitem__`), and the whole `dtype`/casting system. **numpy's real surface is
far larger than this port's**: the port is one dense `float64` array type with ~125
exported functions and methods. The honest summary is that the port implements a
small, well-chosen core and the rest of numpy is simply absent.

## Numeric comparison policy (tolerance)

Floats are **not** compared for exact equality. Two JSON numbers are equal when

```
|a-b| <= 1e-12   (absolute)   OR   |a-b| <= 1e-12 * max(|a|,|b|)   (relative)
```

with `epsilon = 1e-12` declared in `parity_test.go`. This absorbs the difference
between numpy's pairwise summation and the port's naive accumulation loops (see
`mean-precision`, which sums `[1e16, 1, -1e16]`) without hiding real numerical
error — every divergence found below is a behavioural difference, not a rounding one.

Two further encoding rules make numeric parity meaningful:

- **Arrays are emitted as nested JSON lists *plus* an explicit `shape`**
  (`{"kind":"array","shape":[2,3],"data":[[…],[…]]}`), so a shape divergence is
  always distinguishable from a value divergence. `matmul-1d-2d` and `dot-1d`
  below are shape divergences found exactly this way.
- **JSON has no literal for NaN/Inf**, so both runners encode non-finite float64
  values as the sentinel strings `"NaN"`, `"Infinity"`, `"-Infinity"` (in inputs
  as well as outputs) and the harness compares those strings exactly. This is what
  surfaced the port's NaN-ordering bugs. The harness also splices case `args` into
  the request line verbatim rather than re-encoding them, because Go's encoder
  writes `-0.0` as `-0` and Python then reads that back as the integer `0`,
  silently losing the sign of the zero.

## Symbols with cases

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `numpy.absolute` | `numpy.NDArray.Abs` | match | `abs` |  |
| `numpy.add` | `numpy.NDArray.Add`, `numpy.NDArray.AddScalar` | match | `add-2d`, `add-scalar`, `inf-arithmetic`, `bcast-3x1-1x4`, `bcast-2x3-row`, `bcast-scalarish`, … (9 total) |  |
| `numpy.all` | `numpy.NDArray.All` | match | `all-true`, `all-false` |  |
| `numpy.allclose` | `numpy.NDArray.AllClose` | match | `allclose-true`, `allclose-false` |  |
| `numpy.any` | `numpy.NDArray.Any` | match | `any-true`, `any-false` |  |
| `numpy.arange` | `numpy.Arange` | match | `arange-0-10-1`, `arange-frac`, `arange-neg-step`, `arange-empty`, `arange-wrong-dir`, `arange-zero-step` |  |
| `numpy.arccos` | `numpy.NDArray.Arccos` | match | `arccos` |  |
| `numpy.arcsin` | `numpy.NDArray.Arcsin` | match | `arcsin`, `arcsin-domain` |  |
| `numpy.arctan` | `numpy.NDArray.Arctan` | match | `arctan` |  |
| `numpy.arctan2` | `numpy.NDArray.Arctan2` | match | `arctan2-quadrants` |  |
| `numpy.argmax` | `numpy.NDArray.Argmax` | differs | `argmax-1d`, `argmax-ties`, `argmax-2d-flat`, `argmax-empty`, `argmax-with-nan` | port skips NaN; numpy returns the NaN's index (`argmax-with-nan`) |
| `numpy.argmin` | `numpy.NDArray.Argmin` | match | `argmin-1d`, `argmin-ties`, `argmin-empty` |  |
| `numpy.argsort` | `numpy.NDArray.Argsort` | differs | `argsort-1d`, `argsort-stable-dupes`, `argsort-2d-flat`, `argsort-with-nan` | port's comparator collapses on NaN and returns the identity permutation (`argsort-with-nan`) |
| `numpy.array` | `numpy.FromNested` | match | `from-nested-2d`, `from-nested-3d` |  |
| `numpy.array_equal` | `numpy.NDArray.Equal` | match | `array-equal-true`, `array-equal-shape` |  |
| `numpy.broadcast_to` | `numpy.NDArray.BroadcastTo` | differs | `broadcast-to-3x4`, `broadcast-to-1d`, `broadcast-to-add-axis`, `broadcast-to-bad`, `broadcast-to-shrink` | port accepts a shape with *fewer* dims than the input and silently returns the wrong array; numpy raises |
| `numpy.cbrt` | `numpy.NDArray.Cbrt` | match | `cbrt` |  |
| `numpy.ceil` | `numpy.NDArray.Ceil` | match | `ceil` |  |
| `numpy.clip` | `numpy.NDArray.Clip` | differs | `clip`, `clip-inverted` | port panics when min > max; numpy silently returns max |
| `numpy.concatenate` | `numpy.Concatenate` | match | `concat-axis0`, `concat-axis1`, `concat-1d`, `concat-bad-shape`, `concat-bad-axis` |  |
| `numpy.cos` | `numpy.NDArray.Cos` | match | `cos` |  |
| `numpy.cosh` | `numpy.NDArray.Cosh` | match | `cosh` |  |
| `numpy.cross` | `numpy.NDArray.Cross` | match | `cross-3x3`, `cross-bad-len` |  |
| `numpy.cumprod` | `numpy.NDArray.Cumprod` | match | `cumprod-1d`, `cumprod-2d-flat`, `cumprod-zero` |  |
| `numpy.cumsum` | `numpy.NDArray.Cumsum` | match | `cumsum-1d`, `cumsum-2d-flat`, `cumsum-negatives`, `cumsum-empty` |  |
| `numpy.diag` | `numpy.Diag` | match | `diag-from-1d`, `diag-from-2d`, `diag-3d-error` |  |
| `numpy.diagonal` | `numpy.NDArray.Diagonal` | match | `diagonal-k0`, `diagonal-k1`, `diagonal-km1`, `diagonal-1d-error` |  |
| `numpy.diff` | `numpy.NDArray.Diff` | match | `diff-1d`, `diff-2d-flat`, `diff-single`, `diff-empty` |  |
| `numpy.divide` | `numpy.NDArray.Div`, `numpy.NDArray.DivScalar` | match | `div-2d`, `div-by-zero`, `div-scalar`, `div-scalar-zero` |  |
| `numpy.dot` | `numpy.NDArray.Dot` | differs | `dot-1d`, `dot-2d`, `dot-1d-mismatch`, `dot-1d-2d` | port requires matching ndim and returns a shape (1) array for the 1-D/1-D case; numpy returns a 0-d scalar and supports mixed 1-D/2-D |
| `numpy.equal` | `numpy.NDArray.EqualMask`, `numpy.NDArray.EqualScalar` | match | `equal-mask`, `equal-scalar`, `equal-mask-nan` |  |
| `numpy.exp` | `numpy.NDArray.Exp` | match | `exp`, `exp-overflow` |  |
| `numpy.expand_dims` | `numpy.NDArray.ExpandDims` | match | `expand-dims-0`, `expand-dims-1`, `expand-dims-neg1`, `expand-dims-bad` |  |
| `numpy.expm1` | `numpy.NDArray.Expm1` | match | `expm1` |  |
| `numpy.eye` | `numpy.Eye` | match | `eye-3`, `eye-3x4-k1`, `eye-4x3-km1` |  |
| `numpy.flip` | `numpy.NDArray.Flip` | match | `flip-axis0`, `flip-axis1`, `flip-axis-neg`, `flip-bad-axis` |  |
| `numpy.floor` | `numpy.NDArray.Floor` | match | `floor` |  |
| `numpy.fmod` | `numpy.NDArray.Mod` | match | `mod-signs`, `mod-zero` |  |
| `numpy.full` | `numpy.Full` | match | `full-2x3`, `full-neg` |  |
| `numpy.greater` | `numpy.NDArray.Greater`, `numpy.NDArray.GreaterScalar` | match | `greater`, `greater-scalar` |  |
| `numpy.greater_equal` | `numpy.NDArray.GreaterEqual` | match | `greater-equal` |  |
| `numpy.hypot` | `numpy.NDArray.Hypot` | match | `hypot` |  |
| `numpy.identity` | `numpy.Identity` | match | `identity-4` |  |
| `numpy.less` | `numpy.NDArray.Less`, `numpy.NDArray.LessScalar` | match | `less`, `less-scalar` |  |
| `numpy.less_equal` | `numpy.NDArray.LessEqual` | match | `less-equal` |  |
| `numpy.linalg.norm` | `numpy.NDArray.Norm` | match | `norm-1d`, `norm-2d-frobenius` |  |
| `numpy.linspace` | `numpy.Linspace` | match | `linspace-endpoint`, `linspace-open`, `linspace-one`, `linspace-zero`, `linspace-negative-num` |  |
| `numpy.log` | `numpy.NDArray.Log` | match | `log`, `log-zero-neg` |  |
| `numpy.log10` | `numpy.NDArray.Log10` | match | `log10` |  |
| `numpy.log1p` | `numpy.NDArray.Log1p` | match | `log1p` |  |
| `numpy.log2` | `numpy.NDArray.Log2` | match | `log2` |  |
| `numpy.matmul` | `numpy.NDArray.MatMul` | differs | `matmul-2x3-3x2`, `matmul-3x3`, `matmul-identity`, `matmul-mismatch`, `matmul-1d-2d`, `matmul-3d-error` | port requires both operands to be exactly 2-D; numpy promotes 1-D operands and batches >2-D |
| `numpy.max` | `numpy.NDArray.Max`, `numpy.NDArray.MaxAxis` | differs | `max-all`, `max-axis0`, `max-axis1-keepdims`, `max-axis-bad`, `max-with-nan`, `max-axis-with-nan` | port skips NaN; numpy propagates it (`max-with-nan`) |
| `numpy.maximum` | `numpy.NDArray.Maximum` | match | `maximum`, `maximum-nan` |  |
| `numpy.mean` | `numpy.NDArray.Mean`, `numpy.NDArray.MeanAxis` | match | `mean-all`, `mean-axis0`, `mean-axis1-keepdims`, `mean-axis-3d`, `mean-precision` |  |
| `numpy.median` | `numpy.NDArray.Median` | differs | `median-odd`, `median-even`, `median-2d`, `median-empty`, `median-with-nan` | port skips NaN and panics on an empty array; numpy yields NaN in both cases |
| `numpy.min` | `numpy.NDArray.Min`, `numpy.NDArray.MinAxis` | differs | `min-all`, `min-axis0`, `min-axis1`, `min-with-nan` | port skips NaN; numpy propagates it (`min-with-nan`) |
| `numpy.minimum` | `numpy.NDArray.Minimum` | match | `minimum`, `minimum-bcast` |  |
| `numpy.multiply` | `numpy.NDArray.Mul`, `numpy.NDArray.MulScalar` | match | `mul-2d`, `mul-scalar`, `bcast-2x3-col` |  |
| `numpy.ndarray.T` | `numpy.NDArray.T` | match | `transpose-2d` |  |
| `numpy.ndarray.__getitem__` | `numpy.NDArray.Slice`, `numpy.NDArray.At`, `numpy.NDArray.MaskSelect` | match | `slice-2d-block`, `slice-full-axis`, `slice-1d`, `slice-neg-start`, `slice-neg-stop`, `slice-neg-both`, … (21 total) |  |
| `numpy.ndarray.flatten` | `numpy.NDArray.Flatten` | match | `flatten-2d` |  |
| `numpy.ndarray.ndim` | `numpy.NDArray.Ndim` | match | `ndim-3d` |  |
| `numpy.ndarray.ravel` | `numpy.NDArray.Ravel` | match | `ravel-3d`, `ravel-transposed` |  |
| `numpy.ndarray.reshape` | `numpy.NDArray.Reshape` | match | `reshape-6-to-3x2`, `reshape-infer`, `reshape-infer-first`, `reshape-to-3d`, `reshape-bad-size`, `reshape-two-infer` |  |
| `numpy.ndarray.shape` | `numpy.NDArray.Shape` | match | `shape-3d` |  |
| `numpy.ndarray.size` | `numpy.NDArray.Size` | match | `size-3d` |  |
| `numpy.ndarray.transpose` | `numpy.NDArray.Transpose` | match | `transpose-3d` |  |
| `numpy.negative` | `numpy.NDArray.Neg` | match | `neg` |  |
| `numpy.not_equal` | `numpy.NDArray.NotEqualMask` | match | `not-equal-mask` |  |
| `numpy.ones` | `numpy.Ones` | match | `ones-3d`, `ones-1d` |  |
| `numpy.ones_like` | `numpy.OnesLike` | match | `ones-like` |  |
| `numpy.outer` | `numpy.NDArray.Outer` | match | `outer-2x3`, `outer-flattens` |  |
| `numpy.percentile` | `numpy.NDArray.Percentile` | match | `percentile-0`, `percentile-25`, `percentile-50`, `percentile-90`, `percentile-100`, `percentile-2d`, … (8 total) |  |
| `numpy.power` | `numpy.NDArray.Pow`, `numpy.NDArray.PowScalar` | match | `pow-2d`, `pow-neg-base-frac`, `pow-scalar` |  |
| `numpy.prod` | `numpy.NDArray.Prod`, `numpy.NDArray.ProdAxis` | match | `prod-all`, `prod-with-zero`, `prod-axis0`, `prod-axis1-keepdims` |  |
| `numpy.ptp` | `numpy.NDArray.Ptp` | match | `ptp` |  |
| `numpy.quantile` | `numpy.NDArray.Quantile` | match | `quantile-quarter`, `quantile-half`, `quantile-out-of-range` |  |
| `numpy.reciprocal` | `numpy.NDArray.Reciprocal` | match | `reciprocal`, `reciprocal-zero` |  |
| `numpy.roll` | `numpy.NDArray.Roll` | match | `roll-pos`, `roll-neg`, `roll-2d-flat` |  |
| `numpy.round` | `numpy.NDArray.Round` | match | `round-half-even` |  |
| `numpy.searchsorted` | `numpy.NDArray.SearchSorted` | match | `searchsorted-mid`, `searchsorted-exact`, `searchsorted-below`, `searchsorted-above` |  |
| `numpy.sign` | `numpy.NDArray.Sign` | match | `sign`, `sign-nan` |  |
| `numpy.sin` | `numpy.NDArray.Sin` | match | `sin` |  |
| `numpy.sinh` | `numpy.NDArray.Sinh` | match | `sinh` |  |
| `numpy.sort` | `numpy.NDArray.Sort` | differs | `sort-1d`, `sort-dupes`, `sort-2d-flat`, `sort-with-nan`, `sort-neg-zero` | port sorts NaN first, numpy sorts NaN last (`sort-with-nan`) |
| `numpy.sqrt` | `numpy.NDArray.Sqrt` | match | `sqrt`, `sqrt-negative` |  |
| `numpy.square` | `numpy.NDArray.Square` | match | `square` |  |
| `numpy.squeeze` | `numpy.NDArray.Squeeze` | differs | `squeeze-mid`, `squeeze-col`, `squeeze-all-ones` | port returns a 1-D length-1 array where numpy returns a 0-d array |
| `numpy.stack` | `numpy.Stack` | match | `stack-axis0`, `stack-axis1`, `stack-2d-axis1`, `stack-mismatch` |  |
| `numpy.std` | `numpy.NDArray.StdAxis`, `numpy.NDArray.Std`, `numpy.NDArray.StdDDof` | match | `std-axis0`, `std-axis1-keepdims`, `std-axis-3d`, `std-axis-bad`, `std-pop`, `std-2d`, … (8 total) |  |
| `numpy.subtract` | `numpy.NDArray.Sub`, `numpy.NDArray.SubScalar` | match | `sub-2d`, `sub-scalar`, `inf-minus-inf`, `bcast-3d-row` |  |
| `numpy.sum` | `numpy.NDArray.Sum`, `numpy.NDArray.SumAxis` | match | `sum-all`, `sum-3d`, `sum-axis0`, `sum-axis1`, `sum-axis0-keepdims`, `sum-axis1-keepdims`, … (12 total) |  |
| `numpy.tan` | `numpy.NDArray.Tan` | match | `tan` |  |
| `numpy.tanh` | `numpy.NDArray.Tanh` | match | `tanh` |  |
| `numpy.trace` | `numpy.NDArray.Trace` | match | `trace-3x3`, `trace-2x3`, `trace-1d-error` |  |
| `numpy.transpose` | `numpy.NDArray.Transpose` | match | `transpose-perm`, `transpose-perm-2`, `transpose-bad-perm`, `transpose-short-perm` |  |
| `numpy.trunc` | `numpy.NDArray.Trunc` | match | `trunc` |  |
| `numpy.unique` | `numpy.NDArray.Unique` | differs | `unique-1d`, `unique-2d`, `unique-negzero`, `unique-with-nan` | port keeps every NaN and sorts them first; numpy collapses NaN to one trailing entry (`unique-with-nan`) |
| `numpy.var` | `numpy.NDArray.Var`, `numpy.NDArray.VarDDof` | differs | `var-pop`, `var-2d`, `var-ddof1`, `var-ddof-too-big` | `VarDDof` panics when N-ddof <= 0; numpy warns and yields inf |
| `numpy.where` | `numpy.Where` | match | `bcast-where`, `where-basic`, `where-2d`, `where-mismatch` |  |
| `numpy.zeros` | `numpy.Zeros` | match | `zeros-2x3`, `zeros-1d` |  |
| `numpy.zeros_like` | `numpy.ZerosLike` | match | `zeros-like` |  |

### Aliases of symbols above — status `match` (no separate cases)

`abs` (=`absolute`), `amax` (=`max`), `amin` (=`min`), `around` (=`round`),
`concat` (=`concatenate`), `cumulative_sum` (=`cumsum`), `cumulative_prod` (=`cumprod`),
`matrix_transpose` / `permute_dims` (=`transpose`), `pow` (=`power`),
`true_divide` (=`divide`).

## Go symbols with no upstream counterpart, or no case — `extra` / `untested`

| Go symbol | nearest upstream | status | note |
| --- | --- | --- | --- |
| `numpy.Range`, `numpy.R` | — | extra | the port's slice descriptor; numpy uses `slice` syntax |
| `numpy.FromData`, `numpy.FromSlice` | `numpy.array` | extra | Go-specific constructors from a flat `[]float64` + shape (`FromNested` is the one under test) |
| `numpy.NDArray.Copy` | `numpy.ndarray.copy` | untested | no case |
| `numpy.NDArray.Data` | `numpy.ndarray.tolist` | untested | used internally by the Go runner's encoder |
| `numpy.NDArray.Strides` | `numpy.ndarray.strides` | untested | strides are in elements, not bytes |
| `numpy.NDArray.Set` | `numpy.ndarray.__setitem__` | untested | no case (mutation is out of scope for a stateless runner) |
| `numpy.NDArray.String` | `repr(ndarray)` | untested | debug formatting, deliberately not compared |

## Upstream surface the port does not have at all — status `missing`

### Areas absent wholesale

- **The entire dtype / casting system.** The port is `float64`-only: no `dtype`,
  `astype`, `can_cast`, `result_type`, `promote_types`, `finfo`/`iinfo`, and none of
  the 30-odd scalar types (`int8`…`uint64`, `float16`/`32`, `complex64`/`128`,
  `bool_`, `datetime64`, `timedelta64`, `str_`, `object_`, `void`, …). There are no
  integer arrays, no booleans-as-dtype (masks are `1.0`/`0.0` float64), and no
  complex numbers, so `angle`, `conj`, `real`, `imag`, `iscomplexobj` and friends
  have nothing to operate on.
- **`numpy.random`** — no generators, no seeding, no distributions.
- **`numpy.linalg`** beyond a Frobenius norm — no `solve`, `inv`, `det`, `eig`,
  `svd`, `qr`, `cholesky`, `lstsq`, `pinv`, `matrix_rank`, `slogdet`.
- **`numpy.fft`**, **`numpy.polynomial`** (and the legacy `poly1d`/`polyfit`/`polyval`
  family), **`numpy.ma`** (masked arrays), **`numpy.strings`/`char`**, **`numpy.rec`**,
  **`numpy.testing`**, `matrix`/`memmap`/`recarray`.
- **All I/O**: `load`, `save`, `savez`, `loadtxt`, `savetxt`, `genfromtxt`,
  `fromfile`, `tofile`, `frombuffer`, `tobytes`, `dump`/`dumps`.
- **NaN-aware reductions**: `nansum`, `nanmean`, `nanmedian`, `nanstd`, `nanvar`,
  `nanmax`, `nanmin`, `nanargmax`, `nanargmin`, `nanpercentile`, `nanquantile`,
  `nancumsum`, `nancumprod` — notable because the port's *ordinary* reductions
  already behave like the `nan*` variants (see the divergences below).
- **`einsum`/`tensordot`/`inner`/`vdot`/`vecdot`/`kron`/`matvec`/`vecmat`**, and
  batched (>2-D) `matmul`.
- **Bitwise and logical ufuncs** (`bitwise_and`, `left_shift`, `logical_and`,
  `logical_not`, …), `isnan`/`isinf`/`isfinite`/`isclose`/`nan_to_num`,
  `floor_divide`/`remainder`/`divmod`/`modf`/`frexp`/`ldexp`/`copysign`/`signbit`/
  `nextafter`/`spacing`.
- **Axis-aware forms of what the port only does flat**: `sort`/`argsort`/`cumsum`/
  `cumprod`/`diff`/`unique`/`argmax`/`argmin`/`median`/`percentile`/`quantile`/`var`
  all take an `axis=` (and `unique` takes `return_counts`/`return_index`) upstream;
  the port has only whole-array or, for six reductions, single-axis forms — and no
  `VarAxis`, `MedianAxis`, `ArgmaxAxis`, `SortAxis` or `argpartition`/`partition`/
  `lexsort`/`take_along_axis`/`put_along_axis` at all.
- **Stepped and open-ended slicing.** `numpy.NDArray.Slice` takes one
  `Range{Start, Stop}` per axis, with no step field and no way to say "to the end"
  other than passing the axis length (a zero `Range` means the whole axis). So
  `a[::2]`, `a[::-1]`, `a[1::3]`, `a[..., 2:]`, `a[np.newaxis]`, `a[[0,2,1]]`
  (fancy indexing), `a[mask]` as an lvalue, and ellipsis indexing are all
  **inexpressible in the port** — no cases were written for them, because
  contorting them would hide the gap rather than record it. Related missing
  helpers: `take`, `put`, `compress`, `choose`, `select`, `extract`, `place`,
  `putmask`, `indices`, `ix_`, `ravel_multi_index`, `unravel_index`, `nonzero`,
  `argwhere`, `flatnonzero`, `diag_indices`, `tril_indices`, `triu_indices`,
  `mask_indices`, `fill_diagonal`, `moveaxis`, `rollaxis`, `swapaxes`, `rot90`,
  `fliplr`, `flipud`, `resize`, `repeat`, `tile`, `pad`, `insert`, `delete`,
  `append`, `split`/`hsplit`/`vsplit`/`dsplit`/`array_split`, `hstack`/`vstack`/
  `dstack`/`column_stack`/`block`/`unstack`, `atleast_1d`/`2d`/`3d`,
  `broadcast_arrays`, `broadcast_shapes`, `meshgrid`.
- **Statistics and signal helpers**: `average` (weights), `cov`, `corrcoef`,
  `histogram`/`histogram2d`/`histogramdd`/`bincount`/`digitize`, `gradient`,
  `interp`, `convolve`, `correlate`, `trapezoid`, `unwrap`, `sinc`, `i0`,
  `hamming`/`hanning`/`bartlett`/`blackman`/`kaiser`, `piecewise`, `vectorize`,
  `apply_along_axis`, `apply_over_axes`, `frompyfunc`, `nditer`/`ndenumerate`/
  `ndindex`, `set_intersect`-style set ops (`intersect1d`, `union1d`, `setdiff1d`,
  `setxor1d`, `in1d`, `isin`, `unique_all`/`unique_counts`/`unique_inverse`/
  `unique_values`, `ediff1d`, `trim_zeros`, `sort_complex`), `geomspace`,
  `logspace`, `empty`/`empty_like`/`full_like`/`tri`/`tril`/`triu`/`diagflat`/
  `vander`, printing/config (`set_printoptions`, `errstate`, `seterr`, `info`,
  `show_config`, …).

### Exhaustive list: 353 public top-level callables with no Go counterpart and no case

`acos`, `acosh`, `angle`, `append`, `apply_along_axis`, `apply_over_axes`, `arccosh`, `arcsinh`,
`arctanh`, `argpartition`, `argwhere`, `array2string`, `array_equiv`, `array_repr`, `array_split`,
`array_str`, `asanyarray`, `asarray`, `asarray_chkfinite`, `ascontiguousarray`, `asfortranarray`,
`asin`, `asinh`, `asmatrix`, `astype`, `atan`, `atan2`, `atanh`, `atleast_1d`, `atleast_2d`,
`atleast_3d`, `average`, `bartlett`, `base_repr`, `binary_repr`, `bincount`, `bitwise_and`,
`bitwise_count`, `bitwise_invert`, `bitwise_left_shift`, `bitwise_not`, `bitwise_or`,
`bitwise_right_shift`, `bitwise_xor`, `blackman`, `block`, `bmat`, `bool`, `bool_`, `broadcast`,
`broadcast_arrays`, `broadcast_shapes`, `busday_count`, `busday_offset`, `busdaycalendar`, `byte`,
`bytes_`, `can_cast`, `cdouble`, `character`, `choose`, `clongdouble`, `column_stack`,
`common_type`, `complex128`, `complex64`, `complexfloating`, `compress`, `conj`, `conjugate`,
`convolve`, `copy`, `copysign`, `copyto`, `corrcoef`, `correlate`, `count_nonzero`, `cov`,
`csingle`, `datetime64`, `datetime_as_string`, `datetime_data`, `deg2rad`, `degrees`, `delete`,
`diag_indices`, `diag_indices_from`, `diagflat`, `digitize`, `divmod`, `double`, `dsplit`, `dstack`,
`dtype`, `ediff1d`, `einsum`, `einsum_path`, `empty`, `empty_like`, `errstate`, `exp2`, `extract`,
`fabs`, `fill_diagonal`, `finfo`, `fix`, `flatiter`, `flatnonzero`, `flexible`, `fliplr`, `flipud`,
`float16`, `float32`, `float64`, `float_power`, `floating`, `floor_divide`, `fmax`, `fmin`,
`format_float_positional`, `format_float_scientific`, `frexp`, `from_dlpack`, `frombuffer`,
`fromfile`, `fromfunction`, `fromiter`, `frompyfunc`, `fromregex`, `fromstring`, `full_like`, `gcd`,
`generic`, `genfromtxt`, `geomspace`, `get_include`, `get_printoptions`, `getbufsize`, `geterr`,
`geterrcall`, `gradient`, `half`, `hamming`, `hanning`, `heaviside`, `histogram`, `histogram2d`,
`histogram_bin_edges`, `histogramdd`, `hsplit`, `hstack`, `i0`, `iinfo`, `imag`, `in1d`, `indices`,
`inexact`, `info`, `inner`, `insert`, `int16`, `int32`, `int64`, `int8`, `int_`, `intc`, `integer`,
`interp`, `intersect1d`, `intp`, `invert`, `is_busday`, `isclose`, `iscomplex`, `iscomplexobj`,
`isdtype`, `isfinite`, `isfortran`, `isin`, `isinf`, `isnan`, `isnat`, `isneginf`, `isposinf`,
`isreal`, `isrealobj`, `isscalar`, `issubdtype`, `iterable`, `ix_`, `kaiser`, `kron`, `lcm`,
`ldexp`, `left_shift`, `lexsort`, `load`, `loadtxt`, `logaddexp`, `logaddexp2`, `logical_and`,
`logical_not`, `logical_or`, `logical_xor`, `logspace`, `long`, `longdouble`, `longlong`,
`mask_indices`, `matrix`, `matvec`, `may_share_memory`, `memmap`, `meshgrid`, `min_scalar_type`,
`mintypecode`, `mod`, `modf`, `moveaxis`, `nan_to_num`, `nanargmax`, `nanargmin`, `nancumprod`,
`nancumsum`, `nanmax`, `nanmean`, `nanmedian`, `nanmin`, `nanpercentile`, `nanprod`, `nanquantile`,
`nanstd`, `nansum`, `nanvar`, `ndarray`, `ndenumerate`, `ndindex`, `nditer`, `nested_iters`,
`nextafter`, `nonzero`, `number`, `object_`, `packbits`, `pad`, `partition`, `piecewise`, `place`,
`poly`, `poly1d`, `polyadd`, `polyder`, `polydiv`, `polyfit`, `polyint`, `polymul`, `polysub`,
`polyval`, `positive`, `printoptions`, `promote_types`, `put`, `put_along_axis`, `putmask`,
`rad2deg`, `radians`, `ravel_multi_index`, `real`, `real_if_close`, `recarray`, `record`,
`remainder`, `repeat`, `require`, `resize`, `result_type`, `right_shift`, `rint`, `rollaxis`,
`roots`, `rot90`, `row_stack`, `save`, `savetxt`, `savez`, `savez_compressed`, `select`,
`set_printoptions`, `setbufsize`, `setdiff1d`, `seterr`, `seterrcall`, `setxor1d`, `shares_memory`,
`short`, `show_config`, `show_runtime`, `signbit`, `signedinteger`, `sinc`, `single`,
`sort_complex`, `spacing`, `split`, `str_`, `swapaxes`, `take`, `take_along_axis`, `tensordot`,
`test`, `tile`, `timedelta64`, `trapezoid`, `trapz`, `tri`, `tril`, `tril_indices`,
`tril_indices_from`, `trim_zeros`, `triu`, `triu_indices`, `triu_indices_from`, `typename`, `ubyte`,
`ufunc`, `uint`, `uint16`, `uint32`, `uint64`, `uint8`, `uintc`, `uintp`, `ulong`, `ulonglong`,
`union1d`, `unique_all`, `unique_counts`, `unique_inverse`, `unique_values`, `unpackbits`,
`unravel_index`, `unsignedinteger`, `unstack`, `unwrap`, `ushort`, `vander`, `vdot`, `vecdot`,
`vecmat`, `vectorize`, `void`, `vsplit`, `vstack`

### Exhaustive list: 43 public `ndarray` attributes with no Go counterpart and no case

`argpartition`, `astype`, `base`, `byteswap`, `choose`, `compress`, `conj`, `conjugate`, `copy`,
`ctypes`, `data`, `device`, `dtype`, `dump`, `dumps`, `fill`, `flags`, `flat`, `getfield`, `imag`,
`item`, `itemset`, `itemsize`, `mT`, `nbytes`, `newbyteorder`, `nonzero`, `partition`, `put`,
`real`, `repeat`, `resize`, `setfield`, `setflags`, `strides`, `swapaxes`, `take`, `to_device`,
`tobytes`, `tofile`, `tolist`, `tostring`, `view`

(The remaining 23 `ndarray` attributes — `sum`, `mean`, `max`, `min`, `prod`, `std`,
`var`, `sort`, `argsort`, `argmax`, `argmin`, `clip`, `round`, `cumsum`, `cumprod`,
`ptp`, `all`, `any`, `trace`, `dot`, `squeeze`, `copy`, `searchsorted` — are covered
by the top-level function form in the table above.)

## Counts

| | |
| --- | --- |
| upstream symbols with at least one case | **103** (93 top-level callables + 8 public `ndarray` attributes + `ndarray.__getitem__` + `linalg.norm`) |
| `match` | **90** |
| `differs` | **13** |
| `match` by alias (no separate case) | 11 |
| `extra` (Go-only) | 4 (`Range`, `R`, `FromData`, `FromSlice`) |
| `untested` (Go symbol, no case) | 5 (`Copy`, `Data`, `Strides`, `Set`, `String`) |
| `missing` | **396** (353 top-level callables + 43 `ndarray` attributes), plus every submodule and the dtype system |
| **symbol parity** | **90 / 103 = 87.4 %** of the symbols actually compared |
| cases | 301 total — 286 agree, 11 diverge, 4 are recorded deviations; 33 of the agreements are cases where **both sides fail** (bad reshape, incompatible broadcast/matmul, out-of-range axis, empty argmax, out-of-range percentile, …) |
| **case parity** | **286 / 297 = 96.3 %** (`parity.json`, deviations excluded from the denominator) |

A symbol with no case is `untested`, never `match`. The `match` column above never
counts a symbol whose only evidence is that it exists.

## The divergences, in full

All eleven are real; none is a tolerance artefact. `go test` fails on them.

1. **NaN ordering in `Sort`** (`sort-with-nan`) — the port puts NaN **first**
   (`[NaN,1,2,3]`); numpy puts it last (`[1,2,3,NaN]`). The port's own doc comment
   claims "NaN values sort to the end", so this is a bug against its own contract.
2. **`Argsort` collapses on NaN** (`argsort-with-nan`) — for `[3,NaN,1,2]` the port
   returns the identity permutation `[0,1,2,3]`; numpy returns `[2,3,0,1]`. A
   NaN makes every comparison false, and the port's comparator gives up.
3. **`Unique` and NaN** (`unique-with-nan`) — for `[2,NaN,1,NaN]` the port returns
   `[NaN,NaN,1,2]` (4 elements, NaN duplicated and sorted first); numpy returns
   `[1,2,NaN]`.
4. **`Max` does not propagate NaN** (`max-with-nan`) — `[1,NaN,3]` gives `3` in the
   port, `NaN` in numpy.
5. **`Min` does not propagate NaN** (`min-with-nan`) — gives `1`, numpy gives `NaN`.
6. **`Median` does not propagate NaN** (`median-with-nan`) — gives `1`, numpy gives
   `NaN`. (It falls out of divergence 1: the median is read from a sort that put
   NaN at the front.)
7. **`Argmax` and NaN** (`argmax-with-nan`) — the port returns `2` (the largest
   non-NaN); numpy returns `1` (the NaN's index).
8. **`Median` of an empty array** (`median-empty`) — the port panics; numpy warns
   and returns `NaN`.
9. **`VarDDof` when `N-ddof <= 0`** (`var-ddof-too-big`) — the port panics; numpy
   warns and returns `inf`.
10. **`Clip` with `min > max`** (`clip-inverted`) — the port panics; numpy applies
    the bounds in order and silently returns `max` for every element.
11. **`BroadcastTo` a *lower*-rank shape** (`broadcast-to-shrink`) — the port
    accepts `(2,3) -> (3)` and returns `[1,2,3]`, silently dropping data; numpy
    raises `ValueError`. This is the one divergence where the port is *more*
    permissive than upstream and returns a wrong answer rather than an error.

Taken together, 1–7 are one finding: **the port's comparison-based operations have
no NaN policy**, so `Sort`, `Argsort`, `Unique`, `Max`, `Min`, `Median` and `Argmax`
all behave like numpy's `nan*` family instead of like numpy. `MaxAxis` is the
exception — `max-axis-with-nan` shows the axis reduction *does* propagate NaN,
so the port is not even self-consistent.

## Deliberate deviations (recorded, not counted as failures)

These four are consequences of the port's design (float64-only, no 0-d arrays,
strictly-2-D linear algebra) rather than bugs. They are kept as cases and marked
`"deviation"` in the case files; the harness logs them instead of failing.

| case | what happens |
| --- | --- |
| `dot-1d` | numpy returns a 0-d scalar for `dot(1-D, 1-D)`; the port returns a shape `(1)` array |
| `dot-1d-2d` | numpy supports mixed 1-D/2-D `dot`; the port panics |
| `matmul-1d-2d` | numpy promotes a 1-D operand to a matrix; the port requires both operands to be exactly 2-D |
| `squeeze-all-ones` | numpy squeezes `(1,1,1)` to a 0-d array; the port returns a 1-D array of length 1 |

The port has no 0-dimensional array concept at all, which is what the first and
last rows really record. Per `HARNESS.md` these should also be listed in the
library's own `API-DEVIATIONS.md`.
