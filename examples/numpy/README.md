# numpy example

A runnable validation program for the Go port `github.com/malcolmston/numpy`,
consumed as a published module (no `replace` directive).

**Resolved module version:** `github.com/malcolmston/numpy
v0.0.0-20260719012649-e8b2190ec447` — the repo has no semver tags, so
`go get ...@latest` yields a pseudo-version. (Its `VERSION` file claims `0.2.0`,
but no `v0.2.0` tag is published, so `go get github.com/malcolmston/numpy@v0.2.0`
does not work.) The published tree is byte-identical to the local `numpy/`
working copy for all `.go` files.

Every non-trivial result is compared against a hand-computed expectation. The
program prints one line per check, prints a summary, and exits non-zero if any
check mismatches. It makes no network calls and opens no windows.

## Run

```sh
cd examples/numpy
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Current status: builds clean, `go vet` clean, **182 checks, 0 mismatches**.

## What it demonstrates

1. **Creation** — `Arange`, `Linspace` (both `endpoint` modes), `Zeros`, `Ones`,
   `Full`, `ZerosLike`/`OnesLike`, `FromSlice`, `FromData`, `FromNested`
   (2-D and 3-D nesting), `Eye(n,m,k)` with an offset diagonal, `Identity`,
   `String()`.
2. **Shapes** — `Reshape` (including the inferred `-1` axis), `Ndim`, `Size`,
   `Shape`, `Strides`, `At`/`Set` with negative indices, `Copy`, `T` and
   `Transpose(axes...)` on a 3-D array, `Squeeze`, `ExpandDims`, `Flip`, `Roll`,
   `Concatenate` on both axes, `Stack`.
3. **Slicing and views** — `Slice`/`R`, whole-axis `Range{}`, negative indices,
   and a demonstration that `Slice` aliases the parent while `Ravel` of a
   non-contiguous view copies.
4. **Broadcasting** — `(2,3)+(3,)`, `(3,1)*(1,4)` → `(3,4)`, explicit
   `BroadcastTo`, and the panic on incompatible shapes.
5. **Element-wise / matrix ops** — all arithmetic and `*Scalar` variants,
   `Maximum`/`Minimum`/`Clip`, transcendentals (`Sin`, `Cos`, `Exp`/`Log`
   round-trip, `Log2`, `Log10`, `Cbrt`, `Hypot`, `Arctan2`), rounding
   (`Floor`, `Ceil`, `Trunc`, `Round`, `Sign`, `Mod`), `MatMul` for 2×2 and
   (2,3)×(3,2), `MatMul` through a transposed (non-contiguous) operand, and
   `Dot` in both its 1-D and 2-D forms.
6. **Reductions** — `Sum`, `Prod`, `Mean`, `Max`, `Min`, `Var`, `Std`, `Ptp`,
   `Norm`; `SumAxis`/`MeanAxis`/`MaxAxis`/`MinAxis`/`ProdAxis`/`StdAxis` with
   and without `keepdims`, on 2-D and 3-D arrays; mean-centering via keepdims
   broadcasting; `Cumsum`, `Cumprod`, `Diff`.
7. **Sorting / statistics** — `Sort`, `Argsort`, `Argmax`, `Argmin`, `Unique`,
   `SearchSorted`, `Median` (even and odd n), `Percentile`, `Quantile`,
   `VarDDof`, `StdDDof`.
8. **Masking** — all comparison operators and scalar variants, `MaskSelect`,
   `Where`, `Any`, `All`, `Equal`, `AllClose`.
9. **Linear algebra** — `Trace`, `Diagonal(k)` for k = 0/±1, overloaded `Diag`,
   `Outer`, `Cross` (3-vector, 2-vector 0-d result), `Norm`, identity check.
10. **Dtypes** — documents that there is only one.
11. **Panic behavior** — six invalid-usage cases, confirming each panics with a
    `numpy:`-prefixed message.

## Holes found

Nothing was numerically wrong: after correcting the example's own expectations
to the documented behavior, all 182 checks pass. The gaps are missing API and
API-design issues.

### Missing API

- **No `numpy.linalg` core.** There is no `Inv`, `Det`, `Solve`, `Eig`,
  `EigVals`, `SVD`, `QR`, `Cholesky`, `Pinv`, `MatrixRank` or `LstSq` anywhere
  in the package. `linalg.go` is only `MatMul`/`Dot`; `linalgextra.go` adds
  `Trace`, `Diagonal`, `Outer`, `Cross`, `Norm`, `Diag`. Nothing that requires
  a factorization exists, so the whole "solve a linear system" use case is out
  of reach. See the `// HOLE:` block in `linalg()`.
- **No dtypes at all.** `NDArray` is hard-wired to `float64`: there is no
  `Dtype` type, no `AsType`/`Astype`, and no integer, bool or complex array.
  Consequences visible in the example: boolean masks are `1.0`/`0.0` float
  arrays, and `Argsort` returns *float64* indices that must be converted by
  hand before they can index anything. See the `// HOLE:` block in `dtypes()`.
- **No `Norm` variants.** Only the Frobenius/L2 norm; no `ord` parameter
  (L1, L-inf, nuclear) and no axis-wise norm.
- **No axis argument on most functions that have one in NumPy.** `Sort`,
  `Argsort`, `Argmax`, `Argmin`, `Cumsum`, `Cumprod`, `Diff`, `Unique`,
  `Median`, `Percentile`, `Quantile`, `Ptp` and `Roll` all operate on the
  flattened array only (`Flip` is the one exception — it takes an axis). There
  is no `ArgmaxAxis`, `MedianAxis` or `VarAxis`; note `VarAxis` is missing even
  though `StdAxis` exists, an odd asymmetry.
- **No random module, no I/O.** No `Random`/`RandN`/`Seed`, no `Save`/`Load`,
  no `Repeat`, `Tile`, `Split`, `VStack`/`HStack`, `Insert`, `Delete`,
  `Take`/`Put`, `Meshgrid`, `Histogram`, `Interp`, `Einsum`, `NanSum`-family,
  `Isnan`/`Isinf`, `Dstack`, `Pad`, or `Corrcoef`/`Cov`.
- **No fancy/integer-array indexing.** `MaskSelect` covers boolean selection,
  but there is no `a[[0,2,5]]` equivalent and no masked *assignment*.

### API-design problems

- **`Slice` cannot express an open-ended range.** `Range`'s zero value means
  "whole axis", which collides with `Stop == 0`: `R(1, 0)` resolves `Stop` to 0,
  clamps it up to `Start`, and yields an **empty** axis instead of NumPy's
  `a[1:]`. There is no sentinel for "to the end", so every open-ended slice must
  hard-code the axis length. `Range` also has no `Step` field, so `a[::2]` and
  reversed slices are inexpressible. The example checks this behavior explicitly
  in `slicingAndViews()`.
- **`Slice` demands exactly one `Range` per axis** (it panics otherwise), so
  slicing only the first axis of a 4-D array needs three filler `Range{}`
  values.
- **`BroadcastTo` copies.** NumPy's `broadcast_to` returns a read-only view;
  this one returns a contiguous copy (its strides are ordinary row-major, not
  zero-stride), so it cannot be used to avoid an allocation.
- **View-vs-copy is easy to get wrong.** `Slice` and `Transpose` alias the
  parent's buffer while `Reshape`, `Ravel`, `Data` and all arithmetic copy.
  Nothing in the type or method names marks the difference, so whether a write
  through the result is visible in the parent has to be looked up per method.
  (`Data()` is correct on non-contiguous views — it walks strides and returns a
  fresh row-major copy — but that also means it allocates on every call, so it
  is a trap in loops.)
- **`Panic`-only error reporting.** Documented and consistent (all messages are
  `numpy:`-prefixed), but it means shape bugs are runtime crashes with no
  `error`-returning alternative anywhere.
- **Doc contradicts itself on `Reshape`.** `doc.go` says "Reshape re-views the
  same data under a new shape" and, a few lines earlier, lists `Reshape` among
  the operations that "always return a fresh contiguous array". The
  implementation copies (`newArray(a.Data(), resolved)`), so the "re-views"
  wording — repeated in `manipulation.go`'s own first sentence — is wrong.

### Things that made it hard to use

- The float64-only design forces manual `int(...)` conversions around every
  index-returning call, and mixes `int` returns (`Argmax`) with `*NDArray`
  returns of floats (`Argsort`) for the same conceptual operation.
- `Eye(n, m, k)` requires all three arguments — there is no `Eye(n)` shorthand
  despite `Identity(n)` existing.
- `Linspace` requires the `endpoint bool` positionally, so the common call is
  `Linspace(0, 1, 5, true)` with an unexplained trailing literal.
- `Cross` returns a 0-dimensional array for two 2-vectors, which is faithful to
  NumPy but means callers must special-case `Ndim() == 0` and read `Data()[0]`.
