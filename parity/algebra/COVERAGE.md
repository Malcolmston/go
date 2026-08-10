# `algebra` vs sympy — API inventory and parity score

- **Go module under test:** `github.com/malcolmston/algebra v0.8.0` (published module, resolved with
  `GOWORK=off go get github.com/malcolmston/algebra@latest`; no `replace` directive).
  Packages exercised: `algebra`, `algebra/matrix`, `algebra/ntheory`.
- **Upstream oracle:** sympy **1.14.0** (`python3 -c "import sympy; print(sympy.__version__)"`),
  the system interpreter; nothing was installed and no virtualenv was needed.
- **Harness:** `GOWORK=off go test ./parity/algebra/` — both runners are started once and
  every case is streamed through them as JSON Lines. Machine-readable score in `parity.json`.

## How the upstream surface was enumerated

```
python3 -c "import sympy; print(len([n for n in dir(sympy) if not n.startswith('_')]))"     # 928
python3 -c "import sympy; print(len([n for n in dir(sympy.Matrix) if not n.startswith('_')]))"  # 206
```

and for the port:

```
cd algebra && GOWORK=off go doc -all .        | grep -c '^func '   # 162
cd algebra && GOWORK=off go doc -all ./matrix | grep -c '^func '   # 111
cd algebra && GOWORK=off go doc -all ./ntheory| grep -c '^func '   # 93
```

**sympy's surface is vastly larger than this port's, and this document does not pretend
otherwise.** sympy exports **928** public top-level names (874 of them callable) plus 26
subpackages, and `sympy.Matrix` alone has **206** public members. The port exposes **366**
exported functions/methods across the three packages compared here. The parity percentage below
is therefore scored **only over the symbols actually compared**; every sympy symbol outside that
set is listed as `missing` (no counterpart in the port) or `untested`, and it is not counted in
the score. A symbol with no case is never `match`.

## Comparison method — never compare expression strings

Two correct symbolic answers can be spelled differently (`2*x` vs `x*2`, `exp(x^2+C)` vs
`C*exp(x^2)`), so each case declares one of three comparison modes and both runners emit an
already-normalised, JSON-comparable value:

| mode | what is emitted | used for |
| --- | --- | --- |
| `numeric` | the answer evaluated at fixed sample points, as a list of `[re, im]` pairs | derivatives, integrals, simplification, expansion, limits, series, sums, ODE solutions, Cholesky, Frobenius norm, symbolic determinants |
| `set` | the solution set evaluated numerically, deduplicated and sorted by `(re, im)`, as `[re, im]` pairs | `solve`, eigenvalues |
| `exact` | canonical strings — rationals always as `p/q`, integers as plain decimals | determinants, inverses, transpose, rank, char-poly coefficients, RREF, polynomial gcd/discriminant/resultant, factor structure, all of number theory and combinatorics |

**Tolerance.** `numeric` and `set` comparisons accept `|a - b| <= 1e-9 + 1e-9*|b|` on each
component (absolute and relative tolerance both `1e-9`). `exact` comparisons are byte equality of
the canonical strings. Both are defined in `parity_test.go` (`absTol`, `relTol`).

**Sampling points.** The default table is

```
0.3170, 0.7391, 1.4142, 2.2360, 3.1415
```

and for a case with several variables, variable *i* takes sample *j* from index `(j + 2*i) mod 5`,
so the points are deterministic and identical on both sides. Cases that would land on a pole or a
branch cut override the table explicitly (`"samples"` in the case JSON) — e.g. `tan`/`sec` cases
use `0.2 … 1.0`, `asin`-shaped cases use `-0.8 … 0.9`, and cases with `log(x-1)` use `1.3 … 4.5`.

**Antiderivatives** are only defined up to an additive constant, so every integration case sets
`"upToConstant": true` and the runners emit `F(x_j) - F(x_0)` instead of `F(x_j)`. The constant
cancels and a legitimate difference of constants is not scored as a failure.

**Errors are cases too.** 12 cases are expected to fail on *both* sides (singular inverse,
non-square determinant, dimension mismatch, non-positive-definite Cholesky, non-coprime modular
inverse, incompatible CRT moduli, quadratic non-residue, non-unit multiplicative order,
underdetermined and inconsistent linear systems, …). Failing identically counts as parity; only
*whether* the call failed is compared, never the message text.

**String rendering is deliberately excluded** from the general comparison. Three explicit `str-*`
cases exist solely to document the port's printer.

## Score

| | |
| --- | --- |
| cases | **292** |
| cases in agreement | **265** (12 of them agreed failures) |
| cases diverging | **27** |
| **case-level parity** | **90.8 %** |
| upstream/Go symbol pairs compared | **85** |
| pairs fully in agreement (`match`) | **73** |
| pairs with at least one divergence (`differs`) | **12** |
| **symbol-level parity over the compared surface** | **85.9 %** |
| cases by mode | 133 `numeric`, 27 `set`, 132 `exact` |

Group breakdown (from `parity.json`): combinatorics 18/18, differentiation 18/19,
integration 19/28, limits-series 24/26, matrix 42/44, numbertheory 60/62, polynomials 21/21,
rewriting 36/40, solving 27/34.

## The divergences that matter

1. **A genuinely inaccurate answer — `Limit((1+1/x)^x, x -> oo)`.** The port returns
   `2.7182817863957975`; the answer is exactly *e* = `2.718281828459045`. Only 7 digits are
   correct, so the limit is being extrapolated numerically rather than recognised. Case
   `lim-euler-definition`.
2. **`Solution.Result` reports only one root of a multi-root quadratic** (confirmed).
   `SolveQuadraticSteps(x^2-5x+6, x).Result` is `2`; the `Steps` narrate both `2` and `3`.
   `algebra.Solve` itself returns both roots correctly, so this is a defect of the worked-solution
   API only. Cases `solve-quadratic-stepper-result`, `solve-quadratic-stepper-result-irrational`.
3. **`SolveODE1` returns `exp(x^2+C1)` instead of `C1*exp(x^2)`** (confirmed). Both describe the
   same solution family, but the arbitrary constant enters non-linearly, so for a fixed `C1` the
   two answers are different functions (at `C1 = 1/2`: `1.823·exp(x^2)` vs `0.5·exp(x^2)`), and the
   family excludes the `C1 = 0` solution `y = 0`. Only the separable branch is affected — the
   linear branch (`ode1-linear-nonhomogeneous`) agrees with sympy exactly. Cases `ode1-separable`,
   `ode1-exponential-growth`.
4. **`String()` renders division as a negative power** (confirmed): `1/x` prints as `x^(-1)` and
   `(x+1)/(x+2)` as `(x+2)^(-1)*(x+1)`. Cases `str-division`, `str-rational-quotient`.
5. **`LegendreSymbol` accepts a composite modulus and answers anyway.** `LegendreSymbol(3, 4)`
   returns `-1`, a meaningless value; sympy raises `ValueError: p should be an odd prime integer`.
   Case `legendre-nonprime-modulus`.
6. **Characteristic-polynomial sign convention.** `matrix.Matrix.CharPoly` computes
   `det(A - lambda*I)`, sympy's `charpoly` computes `det(lambda*I - A)`. They differ by `(-1)^n`,
   so 2x2 and 4x4 agree and 3x3 does not. Case `charpoly-3x3`.
7. **`Bernoulli(1) = -1/2`** where sympy 1.14 gives `+1/2`; sympy switched convention in 1.12.
   Both conventions exist in the literature. Case `bernoulli-one`.
8. **`digamma` accuracy.** `Diff(Gamma(x), x)` produces the correct `gamma(x)*digamma(x)`, but at
   `x = 1.4142` the port evaluates it to `-0.041601393438867` against sympy's
   `-0.041601392202979` — a relative error of ~3e-8, far outside the 1e-9 tolerance. `Gamma`
   itself is accurate to double precision. Case `diff-gamma`.
9. **`ceil` vs `ceiling`.** Beyond the naming difference, the port's `Parse` does not reject the
   unknown name `ceiling`: it reads `ceiling(7/2)` as an implicit product `7/2*ceiling` with a free
   symbol. Cases `evalf-ceiling-sympy-name`, `evalf-ceil-port-name`.
10. **`DetLU` is inexact.** It agrees within tolerance but pivots in `float64`, returning
    `-305.99999999999994` where the determinant is exactly `-306` and where `Det` and sympy both
    stay exact. Cases `det-via-lu-3x3`, `det-via-lu-4x4`.
11. **Capability gaps that surface as clean "I cannot do this" errors** (the port returns an
    explicit unevaluated node or an error — no wrong answer is ever produced):
    - integration: `log(x)`, `x*log(x)`, `exp(x)*cos(x)` (by parts), `1/((x-1)^2*(x+1))`,
      `(x^2+1)/(x^3+x)`, `1/(x^3+1)` (partial fractions with repeated or irreducible-quadratic
      factors), `exp(-x^2)`, `sin(x)/x`, `1/(x*log(x))` — 9 of 28 integrands;
    - `Limit(x*log(x), x -> 0)` (the `0 * (-oo)` form);
    - `Solve` on transcendental equations (`sin(x)`, `exp(x)-2`), and it errors on a non-zero
      constant equation where sympy returns the empty set;
    - `Eigenvalues` for 4x4 and larger.

Everything else agrees: all 8 series expansions, all 6 summations and 2 products, all 17
polynomial `Solve` cases (linear through quintic, rational, irrational, double, pure-imaginary,
complex-conjugate and casus-irreducibilis roots), all 5 linear systems, all 4 second-order
constant-coefficient ODEs, the full matrix suite except the two items above, and all 80 number
theory / combinatorics cases except `bernoulli-one` and `legendre-nonprime-modulus`.

## Inventory — symbols compared

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sympy.factorial` | `ntheory.Factorial` | match | `factorial-0`, `factorial-20`, `factorial-50` |  |
| `sympy.binomial` | `ntheory.Binomial` | match | `binomial-5-2`, `binomial-50-25`, `binomial-k-greater-than-n`, `binomial-k-zero` |  |
| `sympy.ff` | `ntheory.Permutations` | match | `permutations-10-3`, `permutations-n-equals-k`, `permutations-k-zero`, `permutations-k-greater-than-n` | nPk is sympy's falling factorial ff(n,k) |
| `sympy.factorial` | `ntheory.Multinomial` | match | `multinomial-2-3-4`, `multinomial-binomial-equivalent` | sympy has no multinomial-coefficient function of this shape; oracle is composed from factorials |
| `sympy.catalan` | `ntheory.CatalanNumber` | match | `catalan-0`, `catalan-15` |  |
| `sympy.functions.combinatorial.numbers.stirling` | `ntheory.StirlingSecond` | match | `stirling2-6-3`, `stirling2-n-equals-k`, `stirling2-k-zero` |  |
| `sympy.diff` | `algebra.Diff` | differs | `diff-poly`, `diff-product-rule`, `diff-quotient-rule`, `diff-chain-exp`, `diff-chain-log`, `diff-sqrt`, `diff-tan`, `diff-sec`, `diff-atan`, `diff-asin`, `diff-hyperbolic`, `diff-tanh`, `diff-erf`, `diff-gamma`, `diff-power-tower`, `diff-partial-x`, `diff-second-sin`, `diff-third-poly` | `diff-gamma`: d/dx gamma(x) is the right formula, gamma(x)*digamma(x), but the port's digamma is only accurate to ~3e-8 (-0.041601393438867 vs -0.041601392202979) |
| `sympy.Expr.diff` | `algebra.Expr.Diff` | match | `diff-method-expr` |  |
| `sympy.integrate` | `algebra.Integrate` | differs | `int-poly`, `int-negative-power`, `int-one-over-x`, `int-exp-linear`, `int-sin-linear`, `int-arctan-form`, `int-arctan-form-scaled`, `int-arcsin-form`, `int-byparts-x-exp`, `int-byparts-x2-exp`, `int-byparts-x-sin`, `int-byparts-x-cos2x`, `int-byparts-log`, `int-byparts-x-log`, `int-byparts-exp-cos`, `int-rational-distinct-linear`, `int-rational-x2-minus-1`, `int-rational-repeated-root`, `int-rational-irreducible-quadratic`, `int-rational-cubic-denominator`, `int-tan`, `int-sec`, `int-sinh`, `int-tanh`, `int-gaussian`, `int-sinc`, `int-log-log`, `int-nonsense-symbolic-var` | 9 of 28 integrands are returned as an unevaluated `Integral(...)` node: log(x), x*log(x), exp(x)*cos(x), 1/((x-1)^2*(x+1)), (x^2+1)/(x^3+x), 1/(x^3+1), exp(-x^2), sin(x)/x, 1/(x*log(x)). No wrong antiderivative was produced |
| `sympy.limit` | `algebra.Limit` | differs | `lim-sin-over-x`, `lim-tan-over-x`, `lim-one-minus-cos`, `lim-removable-hole`, `lim-lhopital-exp`, `lim-lhopital-sqrt`, `lim-rational-at-infinity`, `lim-exp-decay-at-infinity`, `lim-euler-definition`, `lim-x-log-x` | `lim-euler-definition`: the port extrapolates numerically and returns 2.7182817863957975 for a limit that is exactly E (7 correct digits). `lim-x-log-x`: 0*(-oo) is left as an unevaluated `Limit(...)` |
| `sympy.series` | `algebra.Series` | match | `series-exp`, `series-sin`, `series-cos`, `series-log1p`, `series-geometric`, `series-sqrt1p`, `series-tan`, `series-taylor-at-one` |  |
| `sympy.summation` | `algebra.Summation` | match | `sum-k`, `sum-k-squared`, `sum-k-cubed`, `sum-constant`, `sum-geometric-numeric`, `sum-geometric-symbolic` |  |
| `sympy.product` | `algebra.Product` | match | `product-factorial`, `product-k-squared` |  |
| `sympy.Matrix.det` | `matrix.Matrix.Det` | match | `det-2x2`, `det-3x3`, `det-4x4-singular`, `det-rational-entries`, `det-nonsquare`, `det-symbolic`, `det-symbolic-3x3` |  |
| `sympy.Matrix.det(method='lu')` | `matrix.Matrix.DetLU` | match | `det-via-lu-3x3`, `det-via-lu-4x4` | agrees within tolerance but the port's DetLU pivots in float64 and returns an inexact Float (-305.99999999999994 for -306) where sympy stays exact |
| `sympy.Matrix.inv` | `matrix.Matrix.Inverse` | match | `inverse-2x2`, `inverse-3x3-tridiagonal`, `inverse-singular`, `inverse-nonsquare` |  |
| `sympy.Matrix.T` | `matrix.Matrix.Transpose` | match | `transpose-2x3`, `transpose-symmetric` |  |
| `sympy.Matrix.rank` | `matrix.Matrix.Rank` | match | `rank-full-2x2`, `rank-deficient-3x3`, `rank-zero-matrix`, `rank-wide-2x4` |  |
| `sympy.Matrix.charpoly` | `matrix.Matrix.CharPoly` | differs | `charpoly-2x2`, `charpoly-3x3`, `charpoly-4x4` | sign convention: the port returns det(A-lambda*I), sympy returns det(lambda*I-A); the two differ by an overall factor of (-1)^n, so odd dimensions disagree |
| `sympy.Matrix.eigenvals` | `matrix.Matrix.Eigenvalues` | differs | `eigenvalues-diagonal-2x2`, `eigenvalues-irrational-2x2`, `eigenvalues-complex-2x2`, `eigenvalues-symmetric-3x3`, `eigenvalues-4x4-diagonal` | `eigenvalues-4x4-diagonal`: the port refuses 4x4 and larger (documented); sympy handles it |
| `sympy.Matrix.cholesky` | `matrix.Matrix.Cholesky` | match | `cholesky-3x3`, `cholesky-2x2-irrational`, `cholesky-not-positive-definite` |  |
| `sympy.Matrix.__mul__` | `matrix.Matrix.Mul` | match | `matmul-2x2`, `matmul-rectangular`, `matmul-dimension-mismatch` |  |
| `sympy.Matrix.__pow__` | `matrix.Matrix.Pow` | match | `matpow-cube`, `matpow-zero` |  |
| `sympy.physics.quantum.TensorProduct` | `matrix.Matrix.Kron` | match | `kron-2x2` |  |
| `sympy.Matrix.solve` | `matrix.Solve` | match | `mat-solve-2x2`, `mat-solve-3x3`, `mat-solve-singular` |  |
| `sympy.Matrix.adjugate` | `matrix.Matrix.Adjugate` | match | `adjugate-3x3` |  |
| `sympy.Matrix.norm` | `matrix.Matrix.NormFro` | match | `norm-frobenius-2x2`, `norm-frobenius-3x3` |  |
| `sympy.Matrix.rref` | `matrix.Matrix.RREF` | match | `rref-3x3`, `rref-rank-deficient` |  |
| `sympy.igcd` | `ntheory.GCD` | match | `gcd-basic`, `gcd-with-zero`, `gcd-negative` |  |
| `sympy.ilcm` | `ntheory.LCM` | match | `lcm-basic`, `lcm-large` |  |
| `sympy.core.intfunc.igcdex` | `ntheory.ExtendedGCD` | match | `extended-gcd-240-46`, `extended-gcd-99-78` |  |
| `sympy.totient` | `ntheory.EulerPhi` | match | `totient-360`, `totient-prime`, `totient-one` |  |
| `sympy.mobius` | `ntheory.MobiusMu` | match | `mobius-squarefree`, `mobius-square-factor`, `mobius-one` |  |
| `sympy.factorint` | `ntheory.FactorList` | match | `factorint-5040`, `factorint-prime`, `factorint-semiprime-large`, `factorint-prime-power` |  |
| `sympy.isprime` | `ntheory.IsPrime` | match | `isprime-carmichael-561`, `isprime-large-prime`, `isprime-one`, `isprime-negative`, `isprime-strong-pseudoprime` |  |
| `sympy.nextprime` | `ntheory.NextPrime` | match | `nextprime-100`, `nextprime-prime-input` |  |
| `sympy.primepi` | `ntheory.PrimePi` | match | `primepi-1000`, `primepi-100000` |  |
| `sympy.divisors` | `ntheory.Divisors` | match | `divisors-60`, `divisors-prime` |  |
| `sympy.divisor_sigma` | `ntheory.DivisorSigma` | match | `divisor-sigma-1-of-12`, `divisor-sigma-2-of-12` |  |
| `sympy.divisor_count` | `ntheory.CountDivisors` | match | `divisor-count-360` |  |
| `sympy.primefactors` | `ntheory.Radical` | match | `radical-360` | sympy has no radical(); oracle is the product of primefactors() |
| `builtins.pow` | `ntheory.ModPow` | match | `modpow-fermat`, `modpow-large-exponent` | sympy exposes no modular-exponentiation function; oracle is Python's 3-argument pow |
| `sympy.mod_inverse` | `ntheory.ModInverse` | match | `modinverse-coprime`, `modinverse-noncoprime` |  |
| `sympy.ntheory.modular.crt` | `ntheory.CRT` | match | `crt-classic`, `crt-noncoprime-moduli` |  |
| `sympy.jacobi_symbol` | `ntheory.JacobiSymbol` | match | `jacobi-1001-9907`, `jacobi-composite-modulus` |  |
| `sympy.legendre_symbol` | `ntheory.LegendreSymbol` | differs | `legendre-10-13`, `legendre-nonprime-modulus` | `legendre-nonprime-modulus`: the port silently returns -1 for LegendreSymbol(3,4), where the symbol is undefined; sympy raises |
| `sympy.discrete_log` | `ntheory.DiscreteLog` | match | `discrete-log-mod-17`, `discrete-log-mod-1009` |  |
| `sympy.ntheory.primetest.is_square` | `ntheory.IsSquare` | match | `is-square-144`, `is-square-145` |  |
| `sympy.sqrt_mod` | `ntheory.SqrtMod` | match | `sqrt-mod-13`, `sqrt-mod-nonresidue` |  |
| `sympy.n_order` | `ntheory.Order` | match | `multiplicative-order-3-mod-7`, `multiplicative-order-noncoprime` |  |
| `sympy.fibonacci` | `ntheory.Fibonacci` | match | `fibonacci-10`, `fibonacci-200`, `fibonacci-zero` |  |
| `sympy.lucas` | `ntheory.Lucas` | match | `lucas-50` |  |
| `sympy.tribonacci` | `ntheory.Tribonacci` | match | `tribonacci-20` |  |
| `sympy.npartitions` | `ntheory.Partition` | match | `partition-5`, `partition-100`, `partition-zero` |  |
| `sympy.bernoulli` | `ntheory.Bernoulli` | differs | `bernoulli-10`, `bernoulli-zero`, `bernoulli-one`, `bernoulli-odd` | `bernoulli-one`: B1 = -1/2 in the port, +1/2 in sympy >= 1.12 (competing conventions) |
| `sympy.factor_list` | `algebra.Poly.Factor` | match | `factor-degrees-quadratic`, `factor-degrees-irreducible-quadratic`, `factor-degrees-x3-minus-1`, `factor-degrees-x4-minus-1`, `factor-degrees-repeated-quadratic`, `factor-degrees-double-roots`, `factor-degrees-quintic`, `factor-degrees-non-monic` |  |
| `sympy.factor` | `algebra.Factor` | match | `factor-numeric-cubic`, `factor-numeric-quartic` |  |
| `sympy.gcd` | `algebra.PolyGCD` | match | `polygcd-x4-x2`, `polygcd-linear-factor`, `polygcd-coprime`, `polygcd-non-monic-inputs` |  |
| `sympy.discriminant` | `algebra.Poly.Discriminant` | match | `discriminant-quadratic`, `discriminant-quadratic-negative`, `discriminant-cubic`, `discriminant-quartic` |  |
| `sympy.resultant` | `algebra.Poly.Resultant` | match | `resultant-common-factor`, `resultant-coprime`, `resultant-linear-pair` |  |
| `sympy.simplify` | `algebra.Simplify` | match | `simplify-pythagorean`, `simplify-double-angle`, `simplify-log-of-exp`, `simplify-exp-sum`, `simplify-cancel-factor`, `simplify-tan-quotient`, `simplify-sqrt-of-square`, `simplify-pi-multiple`, `simplify-euler-identity` |  |
| `sympy.expand` | `algebra.Expand` | match | `expand-binomial-fourth`, `expand-triple-product`, `expand-two-variable-cube` |  |
| `sympy.collect` | `algebra.Collect` | match | `collect-linear-in-x`, `collect-quadratic-in-x` |  |
| `sympy.apart` | `algebra.ApartExpr` | match | `apart-distinct-linear`, `apart-repeated-linear`, `apart-improper-fraction` |  |
| `sympy.Expr.subs` | `algebra.Subs` | match | `subs-integer`, `subs-expression` |  |
| `sympy.N` | `algebra.Evalf` | match | `evalf-pi`, `evalf-mixed-constants` |  |
| `sympy.gamma` | `algebra.Gamma` | match | `evalf-gamma-integer`, `evalf-gamma-half` |  |
| `sympy.beta` | `algebra.Beta` | match | `evalf-beta` |  |
| `sympy.erf` | `algebra.Erf` | match | `evalf-erf` |  |
| `sympy.erfc` | `algebra.Erfc` | match | `evalf-erfc` |  |
| `sympy.factorial` | `algebra.Factorial` | match | `evalf-factorial` |  |
| `sympy.Abs` | `algebra.Abs` | match | `evalf-abs-negative`, `evalf-complex-modulus` |  |
| `sympy.floor` | `algebra.Floor` | match | `evalf-floor` |  |
| `sympy.ceiling` | `algebra.Ceil` | differs | `evalf-ceiling-sympy-name`, `evalf-ceil-port-name` | naming: the port spells it ceil(); worse, its parser reads the unknown name `ceiling` as an implicit product with a symbol instead of rejecting it |
| `sympy.sign` | `algebra.Sign` | match | `evalf-sign` |  |
| `sympy.atan2` | `algebra.Atan2` | match | `evalf-atan2` |  |
| `sympy.conjugate` | `algebra.Conjugate` | match | `evalf-conjugate` |  |
| `sympy.re` | `algebra.Re` | match | `evalf-re-im` |  |
| `sympy.arg` | `algebra.Arg` | match | `evalf-arg` |  |
| `sympy.sstr` | `algebra.Expr.String` | differs | `str-plain-sum`, `str-division`, `str-rational-quotient` | known port issue: division is printed as a negative power - `x^(-1)` for 1/x, `(x+2)^(-1)*(x+1)` for (x+1)/(x+2) |
| `sympy.solve` | `algebra.Solve` | differs | `solve-linear`, `solve-linear-rational`, `solve-quadratic-integer-roots`, `solve-quadratic-rational-roots`, `solve-quadratic-irrational-roots`, `solve-quadratic-double-root`, `solve-quadratic-pure-imaginary`, `solve-quadratic-complex-conjugates`, `solve-cubic-integer-roots`, `solve-cubic-one-real-root`, `solve-cubic-mixed-roots`, `solve-cubic-three-irrational`, `solve-quartic-biquadratic`, `solve-quartic-roots-of-unity`, `solve-quartic-mixed`, `solve-quartic-general`, `solve-quintic-roots-of-unity`, `solve-nonpolynomial-sin`, `solve-nonpolynomial-exp`, `solve-no-solution-constant` | every polynomial case from linear to quintic agrees, including complex conjugate pairs and the casus irreducibilis. Transcendental equations (sin(x), exp(x)-2) are rejected; a constant non-zero equation errors where sympy returns the empty set |
| `sympy.solve` | `algebra.SolveQuadraticSteps().Result` | differs | `solve-quadratic-stepper-result`, `solve-quadratic-stepper-result-irrational` | known port issue: Solution.Result holds only ONE of the two roots although Steps narrates both |
| `sympy.solve` | `algebra.SolveSystem` | match | `system-2x2`, `system-2x2-rational`, `system-3x3`, `system-underdetermined`, `system-inconsistent` | includes the underdetermined and inconsistent systems, which fail on both sides |
| `sympy.dsolve` | `algebra.SolveODE1` | differs | `ode1-separable`, `ode1-exponential-growth`, `ode1-linear-nonhomogeneous` | known port issue: the separable branch returns exp(x^2+C1) rather than C1*exp(x^2) - the same solution family, but the constant enters non-linearly, so for any fixed C1 the two answers are different functions. The linear branch (`ode1-linear-nonhomogeneous`) agrees |
| `sympy.dsolve` | `algebra.SolveODE2Const` | match | `ode2-distinct-real-roots`, `ode2-repeated-root`, `ode2-complex-roots`, `ode2-forced-polynomial` |  |
| `sympy.im` | `algebra.Im` | match | `evalf-re-im` | exercised jointly with `sympy.re` in one expression |

## Inventory — sympy areas with no counterpart in the port (`missing`)

These are the sympy capabilities a CAS user would reach for next; none of them exists in
`algebra`, `algebra/matrix` or `algebra/ntheory`, so they carry no cases.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sympy.integrate(f, (x, a, b))` (definite integrals) | — | missing | — | the port's `Integrate` is indefinite only |
| `sympy.solveset`, `sympy.linsolve`, `sympy.nonlinsolve`, `sympy.nsolve` | — | missing | — | only `Solve` (univariate polynomial) and `SolveSystem` (linear) exist |
| `sympy.roots` (roots with multiplicities) | — | missing | — | `Solve` returns distinct roots without multiplicity |
| `sympy.RootOf` / `sympy.CRootOf` | — | missing | — | no indexed-root representation |
| `sympy.trigsimp`, `powsimp`, `radsimp`, `logcombine`, `expand_trig`, `expand_log`, `cancel`, `together`, `nsimplify`, `Expr.rewrite` | — | missing | — | the port has a single `Simplify` |
| `sympy.dsolve` for order > 2, non-constant coefficients, systems, ICs | — | missing | — | only `SolveODE1` and `SolveODE2Const` |
| `sympy.laplace_transform`, `fourier_transform`, `inverse_laplace_transform`, `mellin_transform` | — | missing | — | no integral transforms |
| `sympy.Matrix.eigenvects`, `diagonalize`, `jordan_form` | — | missing | — | eigenvalues only |
| `sympy.Matrix.LUdecomposition` (exact, no pivoting) | `matrix.Matrix.LU` | differs | — | untested on purpose: the port pivots in `float64`, so `L`, `U` and `P` are not comparable entry-by-entry; only `DetLU` was compared |
| `sympy.Matrix.nullspace`, `columnspace`, `rowspace`, `pinv`, `QRdecomposition`, `singular_values` | `matrix.Matrix.NullspaceExact`, `ColumnSpaceExact`, `RowSpaceExact`, `Pinv`, `QR`, `SingularValues` | untested | — | present on both sides but not compared: basis vectors are only defined up to scaling and ordering, which needs a canonicalisation this harness does not implement |
| `sympy.groebner`, `sympy.GF`, `sympy.minimal_polynomial`, `sympy.sqf`, `sympy.Poly` domain machinery | — | missing | — | the port's `Poly` is univariate over the rationals only |
| `sympy.lambdify`, `sympy.Sum`/`sympy.Product` as objects, assumptions (`Symbol(..., positive=True)`), `sympy.Interval`/set algebra, `sympy.plot`, `sympy.physics.*`, `sympy.stats`, `sympy.geometry`, `sympy.combinatorics.*` | — | missing | — | out of scope for the port |
| `sympy.primerange`, `sympy.prime`, `sympy.primorial`, `sympy.multiplicity`, `sympy.perfect_power`, `sympy.harmonic`, `sympy.euler`, `sympy.genocchi`, `sympy.bell`, `sympy.motzkin`, `sympy.partitions` | `ntheory.PrimesInRange`, `NthPrime`, `Radical`, … (partial) | untested | — | close counterparts exist for some; not compared |

## Inventory — Go-only surface (`extra`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `algebra.SimplifySteps`, `ExpandSteps`, `FactorSteps`, `DifferentiateSteps`, `IntegrateSteps`, `LimitSteps`, `SeriesSteps`, `PartialFractionSteps`, `CompleteSquareSteps`, `SolveLinearSteps`, `SolveQuadraticSteps`, `SolveCubicSteps`, `SolveSystemSteps`, `Solution`, `Step` | extra | `solve-quadratic-stepper-result`, `solve-quadratic-stepper-result-irrational` | worked-solution ("show your work") API with no sympy equivalent; only `SolveQuadraticSteps().Result` was compared, and it diverges |
| — | `algebra.VerifyODE1` | extra | — | untested; checks an ODE solution by substitution |
| — | `algebra.Pretty`, `MathML`, `LaTeXEq`, `GenerateLaTeX`, `SolutionLaTeX` | extra | — | untested rendering helpers (`sympy.pretty`, `sympy.mathml`, `sympy.latex` are the loose analogues) |
| — | `algebra.Evalc`, `EvalComplex`, `Eval` | extra | used by both runners internally | the numeric-mode evaluator; `sympy.N` is the analogue |
| — | `matrix.Dense`, `matrix.Matrix.ToDense`, `Floats`, `EigSym`, `EigSymValues`, `EigenvaluesNumeric`, `RankNumeric`, `Cond2`, `CondP`, `Norm1`, `NormInf`, `NormMax`, `ExpScaled`, `SolveLU`, `LeastSquares` | extra | — | untested float64 fast paths and numeric-analysis helpers |
| — | `ntheory.Montgomery`, `Barrett`, `MulModU64`/`AddModU64`/`SubModU64`/`ModPowU64`, `MillerRabinU64`, `PollardBrentU64`, `PollardRhoBig`, `SegmentedSieve`, `PrimeSieve`, `MobiusSieve`, `TotientSieve`, `PrimePiRange` | extra | — | untested performance-oriented machinery with no sympy counterpart |
| — | `ntheory.PellFundamental`, `ContinuedFraction`, `ContinuedFractionRat`, `Convergents`, `RatFromContinuedFraction`, `SqrtContinuedFraction`, `MertensFunction`, `Carmichael`, `IsPerfect`, `DoubleFactorial`, `SumDivisors`, `AllSqrtModComposite`, `SqrtModPrimePower`, `PrimitiveRoot(s)`, `IsPrimitiveRoot`, `IsQuadraticResidue`, `PrevPrime`, `IsqrtBig` | extra | — | untested; sympy has partial analogues (`continued_fraction`, `mertens`, `is_perfect`, `factorial2`, `primitive_root`, `is_quadratic_residue`, `prevprime`, `integer_nthroot`) that were not compared |
| — | ~140 further subpackages of the `algebra` repo (`stats`, `physics`, `graph`, `crypto`, `fem`, `optimize`, `groebner`, `galois`, …) | extra | — | outside the three packages this harness compares |

## Untested symbols, counted

- Of sympy's **928** public top-level names, **90** appear (directly or as the oracle for a
  composed answer) in this harness. The remaining **838** are `missing` or `untested` and are
  listed verbatim below so the inventory is auditable and reproducible with the enumeration
  command above.
- Of the port's **366** exported functions/methods in the three compared packages, **≈66** are
  covered by cases; the rest are `untested` (see the `extra` table for the notable ones).

<details>
<summary>sympy top-level public names not covered by this harness (838)</summary>

```
AccumBounds, Adjoint, AlgebraicField, AlgebraicNumber, And, AppliedPredicate, Array,
AssumptionsContext, Atom, AtomicExpr, BasePolynomialError, Basic, BlockDiagMatrix, BlockMatrix,
CC, CRootOf, Catalan, Chi, Ci, Circle, CoercionFailed, Complement, ComplexField, ComplexRegion,
ComplexRootOf, Complexes, ComputationFailed, ConditionSet, Contains, CosineTransform, Curve,
DeferredVector, DenseNDimArray, Determinant, DiagMatrix, DiagonalMatrix, DiagonalOf, Dict,
DiracDelta, DisjointUnion, Domain, DomainError, DotProduct, Dummy, E1, EPath, EX, EXRAW, Ei,
Eijk, Ellipse, EmptySequence, EmptySet, Equality, Equivalent, EulerGamma, EvaluationFailed,
ExactQuotientFailed, Expr, ExpressionDomain, ExtraneousFactors, FF, FF_gmpy, FF_python, FU,
FallingFactorial, FiniteField, FiniteSet, FlagError, FourierTransform, FractionField,
FunctionClass, FunctionMatrix, GF, GMPYFiniteField, GMPYIntegerRing, GMPYRationalField, Ge,
GeneratorsError, GeneratorsNeeded, GeometryError, GoldenRatio, GramSchmidt, GreaterThan,
GroebnerBasis, Gt, HadamardPower, HadamardProduct, HankelTransform, Heaviside,
HeuristicGCDFailed, HomomorphismFailed, ITE, Id, Identity, Idx, ImageSet, ImmutableDenseMatrix,
ImmutableDenseNDimArray, ImmutableMatrix, ImmutableSparseMatrix, ImmutableSparseNDimArray,
Implies, Indexed, IndexedBase, IntegerRing, Integers, Integral, Intersection, Interval, Inverse,
InverseCosineTransform, InverseFourierTransform, InverseHankelTransform,
InverseLaplaceTransform, InverseMellinTransform, InverseSineTransform, IsomorphismFailed,
KroneckerDelta, KroneckerProduct, LC, LM, LT, Lambda, LambertW, LaplaceTransform, Le, LessThan,
LeviCivita, Li, Limit, Line, Line2D, Line3D, Lt, MatAdd, MatMul, MatPow, MatrixBase, MatrixExpr,
MatrixPermute, MatrixSlice, MatrixSymbol, Max, MellinTransform, Min, Mod, Monomial,
MultivariatePolynomialError, MutableDenseMatrix, MutableDenseNDimArray, MutableMatrix,
MutableSparseMatrix, MutableSparseNDimArray, NDimArray, Nand, Naturals, Naturals0, Ne,
NonSquareMatrixError, Nor, Not, NotAlgebraic, NotInvertible, NotReversible, Number,
NumberSymbol, O, OmegaPower, OneMatrix, OperationNotSupported, OptionError, Options, Or, Order,
Ordinal, POSform, Parabola, Permanent, PermutationMatrix, Piecewise, Plane, Point, Point2D,
Point3D, PoleError, PolificationFailed, Polygon, PolynomialDivisionFailed, PolynomialError,
PolynomialRing, PowerSet, PrecisionExhausted, Predicate, Product, ProductSet, PurePoly,
PythonFiniteField, PythonIntegerRing, PythonRational, Q, QQ, QQ_I, QQ_gmpy, QQ_python,
Quaternion, RR, Range, RationalField, Rationals, Ray, Ray2D, Ray3D, RealField, RealNumber,
Reals, RefinementFailed, RegularPolygon, Rel, Rem, RisingFactorial, RootOf, RootSum, S, SOPform,
SYMPY_DEBUG, Segment, Segment2D, Segment3D, SeqAdd, SeqFormula, SeqMul, SeqPer, Set, ShapeError,
Shi, Si, Sieve, SineTransform, SingularityFunction, SparseMatrix, SparseNDimArray, StrPrinter,
StrictGreaterThan, StrictLessThan, Subs, Sum, SymmetricDifference, SympifyError, TableForm,
Trace, Transpose, Triangle, TribonacciConstant, Tuple, Unequality, UnevaluatedExpr,
UnificationFailed, Union, UnivariatePolynomialError, UniversalSet, Wild, WildFunction, Xor, Ynm,
Ynm_c, ZZ, ZZ_I, ZZ_gmpy, ZZ_python, ZeroMatrix, Znm, abundance, acosh, acot, acoth, acsc,
acsch, adjoint, airyai, airyaiprime, airybi, airybiprime, algebras, all_roots, andre,
apart_list, appellf1, apply_finite_diff, approximants, are_similar, arity, asec, asech, asinh,
ask, assemble_partfrac_list, assoc_laguerre, assoc_legendre, assuming, assumptions, atanh,
banded, bell, besseli, besselj, besselk, besselsimp, bessely, betainc, betainc_regularized,
binomial_coefficients, binomial_coefficients_list, block_collapse, blockcut, bool_map,
bottom_up, bspline_basis, bspline_basis_set, cacheit, calculus, cancel, capture, carmichael,
cartes, casoratian, cbrt, ccode, centroid, chebyshevt, chebyshevt_poly, chebyshevt_root,
chebyshevu, chebyshevu_poly, chebyshevu_root, check_assumptions, checkodesol, checkpdesol,
checksol, classify_ode, classify_pde, closest_points, cofactors, collect_const, combsimp, comp,
compose, composite, compositepi, concrete, construct_domain, content, continued_fraction,
continued_fraction_convergents, continued_fraction_iterator, continued_fraction_periodic,
continued_fraction_reduce, convex_hull, convolution, core, cosine_transform, coth, count_ops,
count_roots, covering_product, csch, cse, cxxcode, cycle_length, cyclotomic_poly, decompogen,
decompose, default_sort_key, deg, degree, degree_list, denom, derive_by_array, det, det_quick,
diag, diagonalize_vector, dict_merge, difference_delta, differentiate_finite, digamma,
diophantine, dirichlet_eta, discrete, div, doctest, dotprint, egyptian_fraction, elliptic_e,
elliptic_f, elliptic_k, elliptic_pi, epath, erf2, erf2inv, erfcinv, erfi, erfinv, euler,
euler_equations, evalf, evaluate, exp_polar, expand_complex, expand_func, expand_log,
expand_mul, expand_multinomial, expand_power_base, expand_power_exp, expand_trig, expint,
exptrigsimp, exquo, external, eye, factor_cache, factor_nc, factor_system, factor_terms,
factorial2, factorrat, failing_assumptions, false, farthest_points, fcode, fft, field,
field_isomorphism, filldedent, finite_diff_weights, flatten, fourier_series, fourier_transform,
fps, frac, fraction, fresnelc, fresnels, fu, functions, fwht, galois_group, gammasimp, gcd_list,
gcd_terms, gcdex, gegenbauer, genocchi, geometry, get_contraction_structure, get_indices, gff,
gff_list, glsl_code, grevlex, grlex, groebner, ground_roots, group, gruntz, hadamard_product,
half_gcdex, hankel1, hankel2, hankel_transform, harmonic, has_dups, has_variety, hermite,
hermite_poly, hermite_prob, hermite_prob_poly, hessian, hn1, hn2, homogeneous_order, horner,
hyper, hyperexpand, hypersimilar, hypersimp, idiff, ifft, ifwht, igrevlex, igrlex, ilex,
imageset, init_printing, init_session, integer_log, integer_nthroot, integrals, interactive,
interactive_traversal, interpolate, interpolating_poly, interpolating_spline,
intersecting_product, intersection, intervals, intt, inv_quick, inverse_cosine_transform,
inverse_fourier_transform, inverse_hankel_transform, inverse_laplace_transform,
inverse_mellin_transform, inverse_mobius_transform, inverse_sine_transform, invert, is_abundant,
is_amicable, is_carmichael, is_convex, is_decreasing, is_deficient, is_increasing,
is_mersenne_prime, is_monotonic, is_nthpow_residue, is_perfect, is_primitive_root,
is_quad_residue, is_strictly_decreasing, is_strictly_increasing, is_zero_dimensional, isolate,
itermonomials, jacobi, jacobi_normalized, jacobi_poly, jn, jn_zeros, jordan_cell, jscode,
julia_code, kronecker_product, kronecker_symbol, kroneckersimp, laguerre, laguerre_poly,
lambdify, laplace_correspondence, laplace_initial_conds, laplace_transform, latex,
lazy_function, lcm, lcm_list, legendre, legendre_poly, lerchphi, lex, li, limit_seq,
line_integrate, linear_eq_to_matrix, linsolve, list2numpy, ln, logcombine, loggamma, logic,
lowergamma, maple_code, marcumq, mathematica_code, mathieuc, mathieucprime, mathieus,
mathieusprime, mathml, matrices, matrix2numpy, matrix_multiply_elementwise, matrix_symbols,
maximum, meijerg, mellin_transform, memoize_property, mersenne_prime_exponent,
minimal_polynomial, minimum, minpoly, mobius_transform, monic, motzkin, multigamma,
multiline_latex, multinomial_coefficients, multipledispatch, multiplicity, nan, nfloat,
nonlinsolve, not_empty_in, nroots, nsimplify, nsolve, nth_power_roots_poly, ntheory,
nthroot_mod, ntt, num_digits, numbered_symbols, numer, octave_code, ode_order, ones, ord0,
ordered, pager_print, parallel_poly_from_expr, parse_expr, parsing, partition, pde_separate,
pde_separate_add, pde_separate_mul, pdiv, pdsolve, per, perfect_power, periodic_argument,
periodicity, permutedims, pexquo, piecewise_exclusive, piecewise_fold, plot, plot_backends,
plot_implicit, plot_parametric, plotting, polar_lift, polarify, pollard_pm1, pollard_rho, poly,
poly_from_expr, polygamma, polylog, polys, posify, postfixes, postorder_traversal, powdenest,
powsimp, pprint, pprint_try_use_unicode, pprint_use_unicode, pquo, prefixes, prem,
preorder_traversal, pretty, pretty_print, preview, prevprime, prime, prime_decomp,
prime_valuation, primenu, primeomega, primerange, primitive, primitive_element, primitive_root,
primorial, principal_branch, print_ccode, print_fcode, print_glsl, print_gtk, print_jscode,
print_latex, print_maple_code, print_mathml, print_python, print_rcode, print_tree, printing,
prod, proper_divisor_count, proper_divisors, public, pycode, python, quadratic_congruence,
quadratic_residues, quo, rad, radsimp, randMatrix, random_poly, randprime, rational_interpolate,
ratsimp, ratsimpmodprime, rcode, rcollect, real_root, real_roots, reduce_abs_inequalities,
reduce_abs_inequality, reduce_inequalities, reduced, reduced_totient, refine, refine_root,
register_handler, release, rem, remove_handler, reshape, residue, rf, riemann_xi, ring, root,
rootof, roots, rot_axis1, rot_axis2, rot_axis3, rot_ccw_axis1, rot_ccw_axis2, rot_ccw_axis3,
rot_givens, rotations, round_two, rsolve, rsolve_hyper, rsolve_poly, rsolve_ratio, rust_code,
satisfiable, sech, separatevars, sequence, seterr, sets, sfield, shape, sieve, sift, signsimp,
simplify_logic, sinc, sine_transform, singularities, singularityintegrate, smtlib_code,
solve_linear, solve_linear_system, solve_linear_system_LU, solve_poly_inequality,
solve_poly_system, solve_rational_inequalities, solve_triangulated, solve_undetermined_coeffs,
solve_univariate_inequality, solvers, solveset, sqf, sqf_list, sqf_norm, sqf_part,
sqrt_mod_iter, sqrtdenest, srepr, sring, sstrrepr, stationary_points, stieltjes, strategies,
sturm, subfactorial, subresultants, subsets, substitution, swinnerton_dyer_poly, symarray,
symbols, symmetric_poly, symmetrize, sympify, take, tensor, tensorcontraction, tensordiagonal,
tensorproduct, terms_gcd, test, textplot, threaded, timed, to_cnf, to_dnf, to_nnf,
to_number_field, together, topological_sort, total_degree, trace, trailing, transpose, trigamma,
trigsimp, true, trunc, unbranched_argument, unflatten, unpolarify, uppergamma, use, utilities,
var, variations, vectorize, vfield, viete, vring, wronskian, xfield, xring, xthreaded, yn,
zeros, zeta, zoo
```

</details>

## Reproducing

```
cd <repo root>
GOWORK=off go test ./parity/algebra/          # rewrites parity/algebra/parity.json
GOWORK=off go test ./parity/algebra/ -run 'TestParity/int-'   # one group
```

`go test` **fails** while any divergence remains: each of the 27 diverging cases is reported with
`t.Errorf` under its own subtest name. That is the intended behaviour of the harness — the score in
`parity.json` is the deliverable, and a green run would mean full parity over the compared surface.
If `python3` is absent, or sympy is not importable, or the installed sympy is not 1.14.0, the test
`t.Skip`s instead of failing.
