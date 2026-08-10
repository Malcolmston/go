# algebra example

A single runnable program that exercises the published
[`github.com/malcolmston/algebra`](https://pkg.go.dev/github.com/malcolmston/algebra)
computer-algebra system and a representative slice of its subpackages.

**Resolved module version: `github.com/malcolmston/algebra v0.8.0`**
(`go get github.com/malcolmston/algebra@latest` resolved to the real semver tag
`v0.8.0`, not a pseudo-version.) There is no `replace` directive; the example
consumes the module from the proxy exactly as an outside user would.

## Run it

```sh
cd examples/algebra
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program is fully deterministic (all RNGs are seeded), prints ~310 lines of
labelled output, and terminates on its own with exit status 0.

## What it demonstrates

| Section | Package | Covered |
| --- | --- | --- |
| 1 | `algebra` | fluent builders vs. `Parse`/`MustParse`, implicit multiplication, `Expand`, `Simplify` (trig identities, exact `sin(pi/6)`, `log(exp(x))`), exact `big.Rat` arithmetic, `Subs`, `Eval`/`Evalf`, complex arithmetic with `I`, `Conjugate`, `Abs`, `Evalc` |
| 2 | `algebra` | `Diff` (product/quotient/chain rules), `Integrate` (powers, `1/x`, linear substitution, by parts, `atan` form, rational functions by partial fractions, unevaluated `Integral` fallback), `Limit` (including at infinity and L'Hôpital cases), `Series`, `Summation`, `Product` |
| 3 | `algebra` | `PolyFrom`/`NewPoly`, `Degree`, `Derivative`, `Eval`, `Discriminant`, `Factor`, `DivMod`, `PolyGCD`, `PolyLCM`, `Resultant`, `Collect`, `PartialFractions`, `ApartExpr` |
| 4 | `algebra` | `Solve` for degrees 1–4 (rational, complex and irrational roots), `SolveSystem`, `SolveODE1` + `VerifyODE1`, `SolveODE2Const` |
| 5 | `algebra/matrix` | `Det`, `Trace`, `Rank`, `Inverse`, `CharPoly`, `EigSymValues`, `Cholesky`, `Solve` (exact rational), `LU`, `SingularValues`, `Pinv`, `LeastSquares`, symbolic entries (rotation matrix), `Dot`/`Cross`/`Norm`/`Angle`, `Kron`, matrix `Exp` |
| 6 | `algebra/ntheory` | `GCD`/`LCM`/`ExtendedGCD`, sieves, `PrimePi`, `NthPrime`, `Factorize`/`FactorList`, `EulerPhi`, `MobiusMu`, divisor functions, `ModPow`/`ModInverse`/`CRT`, Legendre/Jacobi, `DiscreteLog`, `Fibonacci`/`Lucas`/`Catalan`/`Partition`/`Bernoulli`, continued fractions, `PellFundamental` |
| 7 | `algebra/combin` | factorials, binomials, `Bell`, `Catalan`, both Stirling kinds, derangements, `Motzkin`, Pascal rows, Bernoulli/Euler numbers, harmonic numbers, partition/composition/combination/permutation enumeration, Gray codes, permutation rank/unrank/parity |
| 8 | `algebra/crypto` | RSA keygen + encrypt/decrypt/CRT-decrypt/sign/verify, ElGamal encryption, Shamir 3-of-5 secret sharing, `Factorization`, `PollardRho`, `EulerTotient`, deterministic Miller–Rabin, `PrimitiveRoot`, `ModSqrt`, `MultiplicativeOrder`, baby-step giant-step |
| 9 | `algebra/stats` | descriptive statistics, correlation, `LinearRegression`, Normal/Binomial/Poisson/StudentT/ChiSquared distributions, t-tests, one-way ANOVA, chi-square GOF, confidence intervals, `MultipleLinearRegression`, `BootstrapMean` |
| 10 | `algebra/physics` | constant tables + `Lookup`/`LookupByName`, kinematics, orbital mechanics, relativity, thermodynamics, blackbody, circuits, unit/temperature conversion, `Propagate` (uncertainty), numeric `Gradient`/`Divergence` |
| 11 | `algebra/autodiff` | forward-mode `Dual` derivatives, `Gradient`, `Jacobian`, hyper-dual `Derivatives2`, `Hessian` (all checked against closed forms) |
| 12 | `algebra/graph` | BFS/DFS, `DijkstraPath`, `Kruskal`, `Prim`, greedy colouring, `TopologicalSort`, `TarjanSCC`, `EdmondsKarp` max-flow, `MinCut` |
| 13 | `algebra` | `String`, `LaTeX`, `LaTeXEq`, `MathML`, `Pretty` (Unicode 2-D layout), and the step-by-step `Solution` API (`DifferentiateSteps`, `SolveQuadraticSteps`, `IntegrateSteps`, `SolutionLaTeX`) |

Numerical spot checks that the program verifies against known answers: max flow
= 23 on the CLRS network, `F(100) = 354224848179261915075`, `p(100) =
190569292`, Pell `x² − 61y² = 1 → (1766319049, 226153980)`, Cholesky of the
classic `[[4,12,−16],[12,37,−43],[−16,−43,98]]`, and all autodiff derivatives
against analytic formulas.

## Holes found

These are recorded as `// HOLE:` comments in `main.go` where they affect the
code.

1. **`matrix.Matrix` has no shape/predicate/mapping helpers in the published
   module.** `IsSymmetric`, `IsIdentity`, `IsDiagonal`, `IsUpper/LowerTriangular`,
   `IsAntiSymmetric`, `IsZero`, `Diagonal`, `SubMatrix`, `DeleteRowCol`,
   `HStack`, `VStack`, `SwapRows`, `SwapCols`, `Map` and `Subs` are all absent
   from `v0.8.0`. The repository working tree contains them (`matrix/shape.go`),
   and `matrix/example_test.go` in the *working tree* has runnable examples for
   them, but they were never released. The example open-codes a `mapMatrix`
   helper and uses `a.Equal(a.Transpose())` for symmetry instead.
2. **ElGamal signing is missing.** `v0.8.0`'s `crypto` package has
   `GenerateElGamalKey`/`ElGamalEncrypt`/`ElGamalDecrypt` but no
   `ElGamalSign`, `ElGamalVerify` or `ElGamalSignature` type. The working tree
   has `crypto/elgamal_sign.go`; it is unreleased. The example prints a notice
   in place of the signature round trip.
3. **`Solution.Result` is a single `Expr`, so multi-root step-by-step solutions
   silently lose roots.** `SolveQuadraticSteps(x² − 5x + 6)` correctly narrates
   both roots in its `Steps` (`x -> 2`, `x -> 3`) but reports
   `Result: 2`. Any caller reading `sol.Result` gets half the answer. The type
   needs a `Results []Expr` or similar.
4. **README is substantially out of date relative to the code it ships with.**
   The `v0.8.0` README says solving "covers polynomials of degree 1 (linear) and
   2 (quadratic)… higher-degree and complex roots are deferred" and that
   integration only covers `exp`, `sin`, `cos` and linear substitutions. In fact
   `v0.8.0` solves cubics and quartics, returns complex roots (`x² + 1 → [I,
   -I]`), integrates rational functions by partial fractions, does integration by
   parts, and handles the `atan` form. The README also claims `Solve` returns
   "the real roots", which is wrong. `doc.go` is accurate; the README is not.
5. **The README documents 4 subpackages; the module ships around 100.** Only
   `matrix`, `ntheory`, `stats` and `physics` are mentioned anywhere. There is no
   index of `autodiff`, `combin`, `crypto`, `graph`, `signal`, `wavelet`,
   `tensor`, `linprog`, `queueing`, `voronoi`, … so discovering the API means
   grepping the source tree. This was the single biggest obstacle to writing
   this example.
6. **Division is printed as a negative power, not a fraction.**
   `MustParse("(x^2+1)/(sqrt(x)+3)").String()` gives
   `(sqrt(x) + 3)^(-1)*(x^2 + 1)`. The README promises "a readable infix string
   with correct precedence… and textbook term ordering". `LaTeX` and `Pretty`
   both render a proper fraction, so only `String` is affected — but `String` is
   what `fmt.Println` uses, so it is what users see first.
7. **`physics.Lookup` is exact-match only, with no aliases.** Boltzmann's
   constant is keyed `"k_B"`, the reduced Planck constant `"ħ"`, permittivity
   `"ε0"` — so `Lookup("k")` and `Lookup("hbar")` both fail silently
   (`ok == false`). Non-ASCII keys are awkward to type from calling code, and
   `LookupAny` is only an alias for the *extended* table, not a fuzzier
   matcher.
8. **`SolveODE1` puts the arbitrary constant inside the exponential.**
   `y' = 2xy` returns `y = exp(x² + C1)` rather than the conventional
   `y = C1·exp(x²)`. It is a valid general solution (and `VerifyODE1` confirms
   it), but it is not the textbook form and it cannot express the `y ≡ 0`
   solution for any finite `C1`.
9. **Minor API inconsistencies across sibling packages.** Number-theoretic
   functions are split three ways with different types and different names for
   the same idea: `ntheory` (`int64`/`uint64`/`big.Int` variants with suffixes
   like `U64`/`Big`), `crypto` (all `*big.Int`), and `combin` (`int`). E.g.
   GCD exists as `ntheory.GCD(int64,int64)` and `crypto.GCD(*big.Int,*big.Int)`;
   `Factorize` (map), `FactorList` (slice of `PrimePower`) and
   `crypto.Factorization` (slice of `Factor`) all factor an integer with three
   different return shapes. `combin.AllPermutations` returns Heap's-algorithm
   order (`…[3 2 1] [3 1 2]`) while `combin.NextPermutation` is lexicographic,
   which is easy to trip over.

Nothing panicked, no numerical result was wrong, and no dependency problems were
found — the module is pure standard library and fetched cleanly from the proxy.
