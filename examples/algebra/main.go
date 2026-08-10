// Command algebra-example exercises the github.com/malcolmston/algebra
// computer-algebra system and a representative slice of its subpackages.
//
// It is deliberately breadth-first: each section drives one area of the
// library with real inputs and prints labelled results so that the output can
// be eyeballed against textbook answers.
package main

import (
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"strings"

	"github.com/malcolmston/algebra"
	"github.com/malcolmston/algebra/autodiff"
	"github.com/malcolmston/algebra/combin"
	"github.com/malcolmston/algebra/crypto"
	"github.com/malcolmston/algebra/graph"
	"github.com/malcolmston/algebra/matrix"
	"github.com/malcolmston/algebra/ntheory"
	"github.com/malcolmston/algebra/physics"
	"github.com/malcolmston/algebra/stats"
)

func section(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("=", len(title)))
}

func main() {
	coreCAS()
	calculus()
	polynomials()
	equationSolving()
	linearAlgebra()
	numberTheory()
	combinatorics()
	cryptography()
	statistics()
	physicsDemo()
	automaticDifferentiation()
	graphs()
	printing()

	fmt.Println("\nDone.")
}

// ---------------------------------------------------------------------------

func coreCAS() {
	section("1. Expression building, parsing and simplification")

	x := algebra.Sym("x")
	y := algebra.Sym("y")

	// Two routes to the same tree: fluent builders and the infix parser.
	built := x.Pow(algebra.Int(2)).Add(x.Mul(algebra.Int(2)), algebra.Int(1))
	parsed := algebra.MustParse("x^2 + 2*x + 1")
	fmt.Printf("built            : %v\n", built)
	fmt.Printf("parsed           : %v\n", parsed)
	fmt.Printf("structurally equal: %v\n", built.Equal(parsed))

	// Implicit multiplication is accepted by the parser.
	implicit := algebra.MustParse("2x + 3(x+1) - x y")
	fmt.Printf("implicit mult    : %v\n", implicit)

	fmt.Printf("Expand((x+y)^3)  : %v\n", algebra.Expand(algebra.Pow(algebra.Add(x, y), algebra.Int(3))))
	fmt.Printf("Simplify trig    : %v\n", algebra.Simplify(algebra.MustParse("sin(x)^2 + cos(x)^2")))
	fmt.Printf("Simplify sin(pi/6): %v\n", algebra.Simplify(algebra.Sin(algebra.Mul(algebra.Rat(1, 6), algebra.Pi))))
	fmt.Printf("Simplify log(exp(x)): %v\n", algebra.Simplify(algebra.Log(algebra.Exp(x))))

	// Exact rational arithmetic via math/big.
	third := algebra.Rat(1, 3)
	fmt.Printf("1/3 + 1/6        : %v\n", algebra.Add(third, algebra.Rat(1, 6)))

	// Substitution then numeric evaluation.
	sub := algebra.Subs(parsed, x, algebra.Add(y, algebra.Int(1)))
	fmt.Printf("Subs x -> y+1    : %v\n", algebra.Expand(sub))

	v, err := algebra.Eval(parsed, map[string]float64{"x": 3})
	fmt.Printf("Eval at x=3      : %v (err=%v)\n", v, err)

	f, err := algebra.Evalf(algebra.MustParse("sqrt(2) + log(E) + pi"))
	fmt.Printf("Evalf sqrt2+1+pi : %.10f (err=%v)\n", f, err)

	// Complex arithmetic: I participates in ordinary simplification.
	fmt.Printf("I^2              : %v\n", algebra.Pow(algebra.I, algebra.Int(2)))
	fmt.Printf("exp(I*pi)        : %v\n", algebra.Simplify(algebra.Exp(algebra.Mul(algebra.I, algebra.Pi))))
	z := algebra.Add(algebra.Int(3), algebra.Mul(algebra.Int(4), algebra.I))
	fmt.Printf("conj(3+4i)       : %v\n", algebra.Conjugate(z))
	fmt.Printf("|3+4i|           : %v\n", algebra.Simplify(algebra.Abs(z)))
	c, err := algebra.Evalc(algebra.Exp(algebra.Mul(algebra.I, algebra.Pi)))
	fmt.Printf("Evalc exp(i*pi)  : %v (err=%v)\n", c, err)
}

// ---------------------------------------------------------------------------

func calculus() {
	section("2. Differentiation, integration, limits and series")

	x := algebra.Sym("x")

	for _, s := range []string{"x^2 + 2*x + 1", "sin(x)*exp(x)", "log(x)/x", "atan(x)"} {
		e := algebra.MustParse(s)
		fmt.Printf("d/dx %-14s = %v\n", s, algebra.Simplify(algebra.Diff(e, x)))
	}

	for _, s := range []string{"x^3", "1/x", "exp(2*x)", "cos(3*x + 1)", "x*exp(x)", "1/(x^2+1)", "(x+3)/(x^2-x-2)", "log(log(x))"} {
		e := algebra.MustParse(s)
		fmt.Printf("int %-16s dx = %v\n", s, algebra.Integrate(e, x))
	}

	// Round trip: integrate the derivative.
	e := algebra.MustParse("x^4 - 3*x^2 + 7")
	back := algebra.Simplify(algebra.Integrate(algebra.Diff(e, x), x))
	fmt.Printf("int(d/dx(x^4-3x^2+7)) = %v\n", back)

	fmt.Printf("lim x->0 sin(x)/x      = %v\n", algebra.Limit(algebra.MustParse("sin(x)/x"), x, algebra.Int(0)))
	fmt.Printf("lim x->0 (1-cos(x))/x^2= %v\n", algebra.Limit(algebra.MustParse("(1 - cos(x))/x^2"), x, algebra.Int(0)))
	fmt.Printf("lim x->inf (2x+1)/(x-3)= %v\n", algebra.Limit(algebra.MustParse("(2*x+1)/(x-3)"), x, algebra.Inf))

	fmt.Printf("Series exp(x) @0 n=5   = %v\n", algebra.Series(algebra.Exp(x), x, algebra.Int(0), 5))
	fmt.Printf("Series sin(x) @0 n=6   = %v\n", algebra.Series(algebra.Sin(x), x, algebra.Int(0), 6))

	k := algebra.Sym("k")
	n := algebra.Sym("n")
	fmt.Printf("sum k, k=1..n          = %v\n", algebra.Summation(k, k, algebra.Int(1), n))
	fmt.Printf("sum k^2, k=1..n        = %v\n", algebra.Summation(algebra.Pow(k, algebra.Int(2)), k, algebra.Int(1), n))
	fmt.Printf("sum 2^k, k=0..10       = %v\n", algebra.Summation(algebra.Pow(algebra.Int(2), k), k, algebra.Int(0), algebra.Int(10)))
	fmt.Printf("prod k, k=1..6         = %v\n", algebra.Product(k, k, algebra.Int(1), algebra.Int(6)))
}

// ---------------------------------------------------------------------------

func polynomials() {
	section("3. Polynomial arithmetic, factoring and partial fractions")

	x := algebra.Sym("x")

	// x^3 - 6x^2 + 11x - 6 = (x-1)(x-2)(x-3)
	p, err := algebra.PolyFrom(algebra.MustParse("x^3 - 6*x^2 + 11*x - 6"), x)
	if err != nil {
		panic(err)
	}
	fmt.Printf("p                = %v (degree %d)\n", p, p.Degree())
	fmt.Printf("p'               = %v\n", p.Derivative())
	fmt.Printf("p(4)             = %v\n", p.Eval(algebra.Int(4)))
	fmt.Printf("discriminant     = %v\n", p.Discriminant())

	for _, fac := range p.Factor() {
		fmt.Printf("  factor         : (%v)^%d\n", fac.Base, fac.Mult)
	}

	q := algebra.NewPoly("x", algebra.Int(-1), algebra.Int(1)) // x - 1
	quo, rem, err := p.DivMod(q)
	fmt.Printf("p / (x-1)        = %v  rem %v (err=%v)\n", quo, rem, err)

	r := algebra.NewPoly("x", algebra.Int(-4), algebra.Int(0), algebra.Int(1)) // x^2 - 4
	fmt.Printf("gcd(p, x^2-4)    = %v\n", algebra.PolyGCD(p, r))
	fmt.Printf("lcm(x-1, x^2-4)  = %v\n", algebra.PolyLCM(q, r))
	fmt.Printf("resultant(p, r)  = %v\n", p.Resultant(r))

	fmt.Printf("Factor symbolic  = %v\n", algebra.Factor(algebra.MustParse("x^2 - 5*x + 6"), x))
	fmt.Printf("Collect          = %v\n", algebra.Collect(algebra.MustParse("3*x^2 + 2*x - x^2 + 5"), x))

	// Partial fractions: (3x + 5) / ((x-1)(x+2))
	num := algebra.NewPoly("x", algebra.Int(5), algebra.Int(3))
	den := algebra.NewPoly("x", algebra.Int(-2), algebra.Int(1), algebra.Int(1))
	poly, terms, err := algebra.PartialFractions(num, den)
	fmt.Printf("PartialFractions polynomial part = %v (err=%v)\n", poly, err)
	for _, t := range terms {
		fmt.Printf("  (%v) / (%v)^%d\n", t.Coeff, t.Denom, t.Power)
	}

	ap, err := algebra.ApartExpr(algebra.MustParse("(3*x + 5)/((x-1)*(x+2))"), x)
	fmt.Printf("ApartExpr        = %v (err=%v)\n", ap, err)
}

// ---------------------------------------------------------------------------

func equationSolving() {
	section("4. Equation, system and ODE solving")

	x := algebra.Sym("x")
	y := algebra.Sym("y")

	for _, s := range []string{"2*x - 8", "x^2 - 5*x + 6", "x^2 + 1", "x^3 - 6*x^2 + 11*x - 6", "x^4 - 1"} {
		roots, err := algebra.Solve(algebra.MustParse(s), x)
		if err != nil {
			fmt.Printf("solve %-24s -> error: %v\n", s, err)
			continue
		}
		strs := make([]string, len(roots))
		for i, r := range roots {
			strs[i] = r.String()
		}
		fmt.Printf("solve %-24s -> [%s]\n", s, strings.Join(strs, ", "))
	}

	// Linear system: 2x + y = 5, x - y = 1  ->  x = 2, y = 1.
	sol, err := algebra.SolveSystem(
		[]algebra.Expr{
			algebra.MustParse("2*x + y - 5"),
			algebra.MustParse("x - y - 1"),
		},
		[]algebra.Expr{x, y},
	)
	fmt.Printf("SolveSystem      -> x=%v, y=%v (err=%v)\n", sol[0], sol[1], err)

	// First-order ODE: y' = 2*x*y (separable) -> y = C1*exp(x^2).
	ode, err := algebra.SolveODE1(algebra.MustParse("2*x*y"), x, y)
	if err != nil {
		fmt.Printf("SolveODE1        -> error: %v\n", err)
	} else {
		fmt.Printf("SolveODE1 y'=2xy -> y = %v [kind=%s, verified=%v]\n",
			ode.General, ode.Kind, algebra.VerifyODE1(ode, algebra.MustParse("2*x*y"), x, y))
	}

	// Second-order constant coefficient: y'' - 3y' + 2y = 0.
	ode2, err := algebra.SolveODE2Const(algebra.Int(1), algebra.Int(-3), algebra.Int(2), algebra.Int(0), x, y)
	if err != nil {
		fmt.Printf("SolveODE2Const   -> error: %v\n", err)
	} else {
		fmt.Printf("SolveODE2Const   -> y = %v [kind=%s]\n", ode2.General, ode2.Kind)
	}
}

// ---------------------------------------------------------------------------

// mapMatrix applies f entrywise, standing in for the *Matrix.Map method that
// the published module does not have.
func mapMatrix(m *matrix.Matrix, f func(algebra.Expr) algebra.Expr) *matrix.Matrix {
	out := matrix.New(m.Rows(), m.Cols())
	for i := 0; i < m.Rows(); i++ {
		for j := 0; j < m.Cols(); j++ {
			out.Set(i, j, f(m.At(i, j)))
		}
	}
	return out
}

func linearAlgebra() {
	section("5. Linear algebra (symbolic + numeric)")

	a := matrix.FromInts([][]int64{
		{4, 12, -16},
		{12, 37, -43},
		{-16, -43, 98},
	})
	fmt.Printf("A =\n%v\n", a)

	det, err := a.Det()
	fmt.Printf("det(A)           = %v (err=%v)\n", det, err)
	tr, _ := a.Trace()
	fmt.Printf("trace(A)         = %v\n", tr)
	fmt.Printf("rank(A)          = %d\n", a.Rank())
	// HOLE: v0.8.0 has no shape/predicate helpers on *Matrix — no IsSymmetric,
	// IsIdentity, IsDiagonal, IsUpperTriangular, Diagonal, SubMatrix,
	// DeleteRowCol, HStack/VStack, SwapRows/SwapCols, Map or Subs. They exist in
	// the repository's working tree but are not in the published module, so the
	// predicates below are open-coded.
	fmt.Printf("symmetric        = %v\n", a.Equal(a.Transpose()))

	inv, err := a.Inverse()
	if err != nil {
		fmt.Printf("inverse error: %v\n", err)
	} else {
		prod, _ := a.Mul(inv)
		fmt.Printf("A*A^-1 identity  = %v\n", mapMatrix(prod, algebra.Simplify).Equal(matrix.Identity(3)))
	}

	cp, err := a.CharPoly()
	fmt.Printf("charpoly(A)      = %v (err=%v)\n", cp, err)

	vals, err := a.EigSymValues()
	fmt.Printf("EigSymValues     = %.6f (err=%v)\n", vals, err)

	l, err := a.Cholesky()
	if err != nil {
		fmt.Printf("Cholesky error: %v\n", err)
	} else {
		fmt.Printf("Cholesky L =\n%v\n", l)
	}

	// Solve Ax = b exactly.
	b := matrix.VectorFromInts(1, 2, 3)
	xs, err := matrix.Solve(a, b)
	fmt.Printf("solve A x = b    = %v (err=%v)\n", xs, err)

	// LU with partial pivoting.
	lu, uu, pp, sign, err := a.LU()
	if err != nil {
		fmt.Printf("LU error: %v\n", err)
	} else {
		fmt.Printf("LU sign=%d  L[0][0]=%v U[0][0]=%v P rows=%d\n", sign, lu.At(0, 0), uu.At(0, 0), pp.Rows())
	}

	// SVD / pseudo-inverse of a non-square matrix.
	m := matrix.FromFloats([][]float64{{1, 2}, {3, 4}, {5, 6}})
	sv, err := m.SingularValues()
	fmt.Printf("singular values  = %.6f (err=%v)\n", sv, err)
	pinv, err := m.Pinv(1e-12)
	fmt.Printf("pinv 2x3 shape   = %dx%d (err=%v)\n", pinv.Rows(), pinv.Cols(), err)

	// Least squares fit of y = a + b t through slightly noisy data.
	design := matrix.FromFloats([][]float64{{1, 0}, {1, 1}, {1, 2}, {1, 3}})
	rhs := matrix.NewVector(algebra.Flt(1.0), algebra.Flt(3.1), algebra.Flt(4.9), algebra.Flt(7.2))
	ls, err := matrix.LeastSquares(design, rhs)
	fmt.Printf("least squares     = %v (err=%v)\n", ls, err)

	// Symbolic matrix: entries are expressions.
	t := algebra.Sym("t")
	rot := matrix.FromExpr([][]algebra.Expr{
		{algebra.Cos(t), algebra.Mul(algebra.Int(-1), algebra.Sin(t))},
		{algebra.Sin(t), algebra.Cos(t)},
	})
	rdet, _ := rot.Det()
	fmt.Printf("det(rotation(t)) = %v\n", algebra.Simplify(rdet))
	at0 := mapMatrix(rot, func(e algebra.Expr) algebra.Expr {
		return algebra.Simplify(algebra.Subs(e, t, algebra.Int(0)))
	})
	fmt.Printf("rotation at t=0  =\n%v\n", at0)

	// Vector operations.
	v1 := matrix.VectorFromInts(1, 2, 3)
	v2 := matrix.VectorFromInts(4, 5, 6)
	dot, _ := v1.Dot(v2)
	cross, _ := v1.Cross(v2)
	ang, _ := v1.Angle(v2)
	fmt.Printf("v1.v2=%v  v1xv2=%v  |v1|=%v  angle=%.6f rad\n", dot, cross, v1.Norm(), ang)

	// Kronecker product and matrix exponential.
	kron := matrix.Identity(2).Kron(matrix.FromInts([][]int64{{1, 2}, {3, 4}}))
	fmt.Printf("I2 (x) B is %dx%d\n", kron.Rows(), kron.Cols())
	ex, err := matrix.FromFloats([][]float64{{0, 1}, {0, 0}}).Exp()
	fmt.Printf("exp([[0,1],[0,0]]) =\n%v\n(err=%v)\n", ex, err)
}

// ---------------------------------------------------------------------------

func numberTheory() {
	section("6. Number theory")

	fmt.Printf("gcd(462, 1071)   = %d\n", ntheory.GCD(462, 1071))
	fmt.Printf("lcm(4, 6)        = %d\n", ntheory.LCM(4, 6))
	g, u, v := ntheory.ExtendedGCD(240, 46)
	fmt.Printf("extgcd(240,46)   = g=%d u=%d v=%d  (check %d)\n", g, u, v, 240*u+46*v)

	fmt.Printf("primes <= 50     = %v\n", ntheory.PrimesUpTo(50))
	fmt.Printf("pi(1000)         = %d\n", ntheory.PrimePi(1000))
	fmt.Printf("IsPrime(1000003) = %v\n", ntheory.IsPrime(1000003))
	fmt.Printf("NextPrime(1e6)   = %d\n", ntheory.NextPrime(1000000))
	fmt.Printf("NthPrime(100)    = %d\n", ntheory.NthPrime(100))
	fmt.Printf("factorize(360)   = %v\n", ntheory.Factorize(360))
	fmt.Printf("factor list 360  = %v\n", ntheory.FactorList(360))

	fmt.Printf("phi(36)          = %d\n", ntheory.EulerPhi(36))
	fmt.Printf("mu(30)           = %d\n", ntheory.MobiusMu(30))
	fmt.Printf("divisors(28)     = %v perfect=%v\n", ntheory.Divisors(28), ntheory.IsPerfect(28))
	fmt.Printf("sigma_1(28)      = %d\n", ntheory.SumDivisors(28))

	fmt.Printf("3^200 mod 1000   = %d\n", ntheory.ModPow(3, 200, 1000))
	inv, ok := ntheory.ModInverse(17, 3120)
	fmt.Printf("17^-1 mod 3120   = %d (ok=%v)\n", inv, ok)
	crt, mod, ok := ntheory.CRT([]int64{2, 3, 2}, []int64{3, 5, 7})
	fmt.Printf("CRT              = %d mod %d (ok=%v)\n", crt, mod, ok)
	fmt.Printf("legendre(3, 11)  = %d\n", ntheory.LegendreSymbol(3, 11))
	fmt.Printf("jacobi(1001,9907)= %d\n", ntheory.JacobiSymbol(1001, 9907))
	dl, ok := ntheory.DiscreteLog(2, 22, 29)
	fmt.Printf("log_2(22) mod 29 = %d (ok=%v)\n", dl, ok)

	fmt.Printf("F(100)           = %v\n", ntheory.Fibonacci(100))
	fmt.Printf("L(20)            = %v\n", ntheory.Lucas(20))
	fmt.Printf("Catalan(15)      = %v\n", ntheory.CatalanNumber(15))
	fmt.Printf("p(100)           = %v\n", ntheory.Partition(100))
	fmt.Printf("B_10             = %v\n", ntheory.Bernoulli(10))
	fmt.Printf("C(50, 25)        = %v\n", ntheory.Binomial(50, 25))

	// Continued fractions and Pell.
	cf := ntheory.ContinuedFractionRat(big.NewRat(415, 93))
	fmt.Printf("cf(415/93)       = %v -> %v\n", cf, ntheory.RatFromContinuedFraction(cf))
	a0, period := ntheory.SqrtContinuedFraction(7)
	fmt.Printf("cf(sqrt 7)       = [%d; %v]\n", a0, period)
	px, py := ntheory.PellFundamental(61)
	fmt.Printf("Pell x^2-61y^2=1 : x=%v y=%v\n", px, py)
}

// ---------------------------------------------------------------------------

func combinatorics() {
	section("7. Combinatorics")

	fmt.Printf("20!              = %v\n", combin.Factorial(20))
	fmt.Printf("C(52, 5)         = %v\n", combin.Binomial(52, 5))
	fmt.Printf("P(10, 3)         = %v\n", combin.Permutations(10, 3))
	fmt.Printf("multinomial(3,2,1)=%v\n", combin.Multinomial(3, 2, 1))
	fmt.Printf("Bell(10)         = %v\n", combin.Bell(10))
	fmt.Printf("Catalan(10)      = %v\n", combin.Catalan(10))
	fmt.Printf("Stirling2(10, 3) = %v\n", combin.StirlingSecond(10, 3))
	fmt.Printf("Stirling1(10, 3) = %v\n", combin.StirlingFirst(10, 3))
	fmt.Printf("derangements(10) = %v (prob %.6f)\n", combin.Derangement(10), combin.DerangementProbability(10))
	fmt.Printf("Motzkin(10)      = %v\n", combin.MotzkinNumber(10))
	fmt.Printf("Pascal row 6     = %v\n", combin.PascalRow(6))
	fmt.Printf("Bernoulli B_12   = %v\n", combin.BernoulliNumber(12))
	fmt.Printf("Euler E_10       = %v\n", combin.EulerNumber(10))
	fmt.Printf("H_10             = %.10f\n", combin.HarmonicNumber(10))
	fmt.Printf("partitions of 5  = %v\n", combin.Partitions(5))
	fmt.Printf("p(5) count       = %d\n", combin.PartitionNumber(5))
	fmt.Printf("compositions(4)  = %v\n", combin.CompositionList(4))
	fmt.Printf("C(4,2) subsets   = %v\n", combin.CombinationList(4, 2))
	fmt.Printf("perms of [1 2 3] = %v\n", combin.AllPermutations([]int{1, 2, 3}))
	fmt.Printf("gray 3 bits      = %v\n", combin.GrayCodeSequence(3))

	// Rank / unrank round trip over permutations of 5 elements.
	perm := []int{2, 0, 4, 1, 3}
	rank := combin.PermutationRank(perm)
	fmt.Printf("rank %v = %v, unrank -> %v, parity=%d\n",
		perm, rank, combin.PermutationUnrank(5, rank), combin.PermutationParity(perm))
}

// ---------------------------------------------------------------------------

func cryptography() {
	section("8. Cryptography and modular arithmetic")

	rng := rand.New(rand.NewSource(20260810))

	// RSA round trip with a deterministic RNG so the run is reproducible.
	priv, err := crypto.GenerateRSAKey(256, big.NewInt(65537), rng)
	if err != nil {
		fmt.Printf("GenerateRSAKey error: %v\n", err)
	} else {
		msg := big.NewInt(424242)
		ct, err1 := crypto.RSAEncrypt(priv.Public, msg)
		pt, err2 := crypto.RSADecrypt(priv, ct)
		ptCRT, err3 := crypto.RSADecryptCRT(priv, ct)
		sig, err4 := crypto.RSASign(priv, msg)
		fmt.Printf("RSA modulus bits = %d\n", priv.Public.N.BitLen())
		fmt.Printf("RSA decrypt ok   = %v (CRT ok=%v)\n", pt.Cmp(msg) == 0, ptCRT.Cmp(msg) == 0)
		fmt.Printf("RSA verify sig   = %v\n", crypto.RSAVerify(priv.Public, msg, sig))
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			fmt.Printf("  (errs: %v %v %v %v)\n", err1, err2, err3, err4)
		}
	}

	// ElGamal encryption and signing.
	eg := crypto.GenerateElGamalKey(160, rng)
	m := big.NewInt(12345)
	c1, c2, err := crypto.ElGamalEncrypt(eg.Public, m, rng)
	if err != nil {
		fmt.Printf("ElGamalEncrypt error: %v\n", err)
	} else {
		dec, err := crypto.ElGamalDecrypt(eg, c1, c2)
		fmt.Printf("ElGamal decrypt ok = %v (err=%v)\n", dec != nil && dec.Cmp(m) == 0, err)
	}
	// HOLE: ElGamal *signing* is absent from the published module — there is no
	// crypto.ElGamalSign / crypto.ElGamalVerify / crypto.ElGamalSignature in
	// v0.8.0 (only encryption/decryption). The repository working tree has
	// crypto/elgamal_sign.go, but it was never released.
	//
	//	sig, err := crypto.ElGamalSign(eg, m, rng)
	//	fmt.Printf("ElGamal verify = %v\n", crypto.ElGamalVerify(eg.Public, m, sig))
	fmt.Println("ElGamal signing    = unavailable in v0.8.0 (see HOLE in source)")

	// Shamir secret sharing: 3-of-5, reconstruct from any 3 shares.
	prime := big.NewInt(2147483647) // 2^31 - 1, a Mersenne prime
	secret := big.NewInt(987654321)
	shares, err := crypto.SplitSecret(secret, 3, 5, prime, rng)
	if err != nil {
		fmt.Printf("SplitSecret error: %v\n", err)
	} else {
		got, err := crypto.CombineShares(shares[1:4], prime)
		fmt.Printf("Shamir 3-of-5 ok = %v (err=%v)\n", got != nil && got.Cmp(secret) == 0, err)
	}

	// Factoring / primality helpers on big.Int.
	n := big.NewInt(1000003 * 1000033)
	fmt.Printf("Factorization    = %v\n", crypto.Factorization(n))
	fmt.Printf("PollardRho       = %v\n", crypto.PollardRho(n))
	fmt.Printf("EulerTotient(n)  = %v\n", crypto.EulerTotient(n))
	fmt.Printf("MillerRabinDet   = %v\n", crypto.MillerRabinDeterministic(big.NewInt(1000003)))
	fmt.Printf("PrimitiveRoot(41)= %v\n", crypto.PrimitiveRoot(big.NewInt(41)))
	ms, err := crypto.ModSqrt(big.NewInt(10), big.NewInt(13))
	fmt.Printf("sqrt(10) mod 13  = %v (err=%v)\n", ms, err)
	ord, err := crypto.MultiplicativeOrder(big.NewInt(3), big.NewInt(41))
	fmt.Printf("ord_41(3)        = %v (err=%v)\n", ord, err)
	if dl, err := crypto.BabyStepGiantStep(big.NewInt(3), big.NewInt(27), big.NewInt(41), big.NewInt(40)); err == nil {
		fmt.Printf("BSGS log_3(27)   = %v\n", dl)
	} else {
		fmt.Printf("BSGS error: %v\n", err)
	}
}

// ---------------------------------------------------------------------------

func statistics() {
	section("9. Statistics")

	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	fmt.Printf("data             = %v\n", xs)
	fmt.Printf("mean=%.4f median=%.4f mode=%v\n", stats.Mean(xs), stats.Median(xs), stats.Mode(xs))
	fmt.Printf("var=%.4f popvar=%.4f sd=%.4f popsd=%.4f\n",
		stats.Variance(xs), stats.PopVariance(xs), stats.StdDev(xs), stats.PopStdDev(xs))
	fmt.Printf("Q1=%.4f Q3=%.4f IQR=%.4f skew=%.4f kurt=%.4f\n",
		stats.Quantile(xs, 0.25), stats.Quantile(xs, 0.75), stats.IQR(xs), stats.Skewness(xs), stats.Kurtosis(xs))
	fmt.Printf("geo mean=%.4f harm mean=%.4f\n", stats.GeometricMean(xs), stats.HarmonicMean(xs))

	ys := []float64{1, 3, 3, 5, 6, 5, 8, 10}
	fmt.Printf("cov=%.4f corr=%.4f\n", stats.Covariance(xs, ys), stats.Correlation(xs, ys))
	slope, intercept, r2 := stats.LinearRegression(xs, ys)
	fmt.Printf("regression       : y = %.4f x + %.4f  (r2=%.4f)\n", slope, intercept, r2)

	// Distributions.
	nd := stats.Normal{Mu: 0, Sigma: 1}
	fmt.Printf("N(0,1) pdf(0)=%.6f cdf(1.96)=%.6f q(0.975)=%.6f\n", nd.PDF(0), nd.CDF(1.96), nd.Quantile(0.975))
	bd := stats.Binomial{N: 10, P: 0.3}
	fmt.Printf("Bin(10,0.3) pmf(3)=%.6f cdf(3)=%.6f mean=%.4f var=%.4f\n", bd.PMF(3), bd.CDF(3), bd.Mean(), bd.Variance())
	pd := stats.Poisson{Lambda: 4}
	fmt.Printf("Poisson(4) pmf(4)=%.6f cdf(4)=%.6f\n", pd.PMF(4), pd.CDF(4))
	td := stats.StudentT{Nu: 10}
	fmt.Printf("t(10) cdf(2.228)=%.6f\n", td.CDF(2.228))
	cs := stats.ChiSquared{K: 3}
	fmt.Printf("chi2(3) cdf(7.815)=%.6f\n", cs.CDF(7.815))

	// Hypothesis tests.
	tr := stats.OneSampleTTest(xs, 4)
	fmt.Printf("1-sample t vs 4  : t=%.4f df=%.1f p=%.6f\n", tr.Statistic, tr.DF, tr.PValue)
	tr2 := stats.TwoSampleTTest(xs, ys, true)
	fmt.Printf("2-sample t       : t=%.4f df=%.1f p=%.6f\n", tr2.Statistic, tr2.DF, tr2.PValue)
	an := stats.OneWayANOVA([]float64{1, 2, 3}, []float64{4, 5, 6}, []float64{7, 8, 9})
	fmt.Printf("one-way ANOVA    : F=%.4f p=%.8f\n", an.Statistic, an.PValue)
	gof := stats.ChiSquareGoodnessOfFit([]float64{30, 14, 34, 45, 57, 20}, []float64{33, 33, 33, 33, 33, 33})
	fmt.Printf("chi2 GOF         : X2=%.4f df=%.1f p=%.6f\n", gof.Statistic, gof.DF, gof.PValue)

	lo, hi := stats.MeanConfidenceInterval(xs, 0.95)
	fmt.Printf("95%% CI for mean  : [%.4f, %.4f]\n", lo, hi)

	// Multiple regression on an exact plane z = 1 + 2u + 3v.
	X := [][]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}, {2, 1}}
	z := []float64{1, 3, 4, 6, 8}
	coeffs, r2m := stats.MultipleLinearRegression(X, z, true)
	fmt.Printf("multi regression : coeffs=%.4f r2=%.6f\n", coeffs, r2m)

	// Bootstrap with a seeded RNG.
	br := stats.BootstrapMean(xs, 2000, 0.95, rand.New(rand.NewSource(7)))
	fmt.Printf("bootstrap mean   : %.4f (se %.4f) CI [%.4f, %.4f]\n", br.Estimate, br.StdErr, br.Lo, br.Hi)
}

// ---------------------------------------------------------------------------

func physicsDemo() {
	section("10. Physics constants, units and formulas")

	fmt.Printf("core constants: %d, extended CODATA table: %d\n",
		len(physics.Constants()), len(physics.ExtendedConstants()))
	// NOTE: Lookup matches Symbol *exactly*, so Boltzmann's constant is "k_B",
	// not "k". There is no fuzzy/alias lookup in the core table.
	for _, sym := range []string{"c", "h", "G", "k_B", "N_A", "R"} {
		if k, ok := physics.Lookup(sym); ok {
			fmt.Printf("  %-4s %-28s = %-14g %s\n", k.Symbol, k.Name, k.Value, k.Unit)
		} else {
			fmt.Printf("  %-4s not found\n", sym)
		}
	}
	if k, ok := physics.LookupByName("Boltzmann constant"); ok {
		fmt.Printf("  LookupByName -> %s = %g %s\n", k.Symbol, k.Value, k.Unit)
	}

	fmt.Printf("KE(2 kg, 3 m/s)      = %g J\n", physics.KineticEnergy(2, 3))
	fmt.Printf("projectile range     = %.4f m (v=20, 45 deg)\n", physics.ProjectileRange(20, math.Pi/4))
	fmt.Printf("escape velocity Earth= %.2f m/s\n", physics.EscapeVelocity(5.972e24, 6.371e6))
	fmt.Printf("orbital period LEO   = %.2f s\n", physics.OrbitalPeriod(5.972e24, 6.771e6))
	fmt.Printf("Lorentz gamma @0.9c  = %.6f\n", physics.LorentzFactor(0.9*physics.SpeedOfLight))
	fmt.Printf("E = mc^2 for 1 g     = %.6e J\n", physics.MassEnergy(1e-3))
	fmt.Printf("photon 500 nm        = %.6e J\n", physics.PhotonEnergy(physics.SpeedOfLight/500e-9))
	fmt.Printf("Carnot eff 300/500 K = %.4f\n", physics.CarnotEfficiency(300, 500))
	fmt.Printf("Wien peak at 5778 K  = %.4e m\n", physics.WienPeakWavelength(5778))
	fmt.Printf("RMS speed N2 @300K   = %.2f m/s\n", physics.RMSSpeed(300, 0.028))
	fmt.Printf("resistors 100||200   = %g ohm\n", physics.ResistorsParallel(100, 200))

	// Unit conversion and temperature scales.
	km, err := physics.Convert(1, "mi", "km")
	fmt.Printf("1 mi                 = %v km (err=%v)\n", km, err)
	fmt.Printf("25 C                 = %.2f K = %.2f F\n", physics.CelsiusToKelvin(25), physics.CelsiusToFahrenheit(25))
	fmt.Printf("1 eV                 = %.6e J\n", physics.EVToJoules(1))

	// Uncertainty propagation through the area of a rectangle.
	area, err := physics.Propagate(func(v []float64) float64 { return v[0] * v[1] },
		[]float64{2.0, 3.0}, []float64{0.01, 0.02})
	fmt.Printf("area 2x3 +- prop     = %.4f +- %.4f (err=%v)\n", area.Value, area.Sigma, err)

	// Vector calculus on a scalar/vector field, numerically.
	p := physics.NewVec3(1, 2, 3)
	grad := physics.Gradient(func(q physics.Vec3) float64 { return q.X*q.X + q.Y*q.Y + q.Z*q.Z }, p, 1e-5)
	div := physics.Divergence(func(q physics.Vec3) physics.Vec3 { return q }, p, 1e-5)
	fmt.Printf("grad r^2 at (1,2,3)  = (%.4f, %.4f, %.4f); div(r) = %.4f\n", grad.X, grad.Y, grad.Z, div)
}

// ---------------------------------------------------------------------------

func automaticDifferentiation() {
	section("11. Forward-mode automatic differentiation")

	// f(x) = x^2 * sin(x); f'(x) = 2x sin x + x^2 cos x.
	f := func(d autodiff.Dual) autodiff.Dual {
		return d.Mul(d).Mul(autodiff.Sin(d))
	}
	at := 1.3
	val, der := autodiff.ValueAndDerivative(f, at)
	want := 2*at*math.Sin(at) + at*at*math.Cos(at)
	fmt.Printf("f(1.3)=%.10f  f'(1.3)=%.10f (analytic %.10f)\n", val, der, want)

	// Gradient of g(x,y,z) = x*y + sin(z).
	g := func(v []autodiff.Dual) autodiff.Dual {
		return v[0].Mul(v[1]).Add(autodiff.Sin(v[2]))
	}
	fmt.Printf("grad g at (2,3,0)    = %.6f\n", autodiff.Gradient(g, []float64{2, 3, 0}))

	// Jacobian of a 2->2 map.
	jf := func(v []autodiff.Dual) []autodiff.Dual {
		return []autodiff.Dual{v[0].Mul(v[0]).Add(v[1]), v[0].Mul(v[1])}
	}
	fmt.Printf("jacobian at (2,3)    = %.6f\n", autodiff.Jacobian(jf, []float64{2, 3}))

	// Second derivatives via hyper-duals: h(x) = exp(x)*cos(x).
	h := func(d autodiff.HyperDual) autodiff.HyperDual {
		return d.Exp().Mul(d.Cos())
	}
	v0, d1, d2 := autodiff.Derivatives2(h, 0.7)
	fmt.Printf("h(0.7)=%.10f h'=%.10f h''=%.10f (h''analytic %.10f)\n",
		v0, d1, d2, -2*math.Exp(0.7)*math.Sin(0.7))

	// Hessian of q(x,y) = x^2*y + y^3.
	q := func(v []autodiff.HyperDual) autodiff.HyperDual {
		return v[0].Mul(v[0]).Mul(v[1]).Add(v[1].Mul(v[1]).Mul(v[1]))
	}
	fmt.Printf("hessian at (1,2)     = %.6f\n", autodiff.Hessian(q, []float64{1, 2}))
}

// ---------------------------------------------------------------------------

func graphs() {
	section("12. Graph algorithms")

	// Weighted undirected graph.
	ug := graph.New()
	edges := [][3]float64{
		{0, 1, 4}, {0, 2, 1}, {2, 1, 2}, {1, 3, 5}, {2, 3, 8}, {3, 4, 3},
	}
	for _, e := range edges {
		ug.AddWeightedEdge(int(e[0]), int(e[1]), e[2])
	}
	fmt.Printf("vertices=%d edges=%d connected=%v tree=%v bipartite=%v\n",
		ug.NumVertices(), ug.NumEdges(), ug.IsConnected(), ug.IsTree(), ug.IsBipartite())

	order, err := ug.BFS(0)
	fmt.Printf("BFS from 0       = %v (err=%v)\n", order, err)
	dorder, _ := ug.DFS(0)
	fmt.Printf("DFS from 0       = %v\n", dorder)

	path, w, err := ug.DijkstraPath(0, 4)
	fmt.Printf("Dijkstra 0->4    = %v weight %.1f (err=%v)\n", path, w, err)

	mst, total, err := ug.Kruskal()
	fmt.Printf("Kruskal MST cost = %.1f over %d edges (err=%v)\n", total, len(mst), err)
	pmst, ptotal, _ := ug.Prim(0)
	fmt.Printf("Prim MST cost    = %.1f over %d edges\n", ptotal, len(pmst))

	fmt.Printf("greedy coloring  = %v (colors=%d, proper=%v)\n",
		ug.GreedyColoring(), graph.NumColors(ug.GreedyColoring()), ug.IsProperColoring(ug.GreedyColoring()))

	// Directed graph: topological sort and strongly connected components.
	dg := graph.NewDirected()
	for _, e := range [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}, {3, 4}} {
		dg.AddEdge(e[0], e[1])
	}
	topo, err := dg.TopologicalSort()
	fmt.Printf("DAG=%v topo order = %v (err=%v)\n", dg.IsDAG(), topo, err)

	cyc := graph.NewDirected()
	for _, e := range [][2]int{{0, 1}, {1, 2}, {2, 0}, {2, 3}} {
		cyc.AddEdge(e[0], e[1])
	}
	fmt.Printf("Tarjan SCC       = %v\n", cyc.TarjanSCC())

	// Max flow on a classic network.
	flowG := graph.NewDirected()
	for _, e := range [][3]float64{{0, 1, 16}, {0, 2, 13}, {1, 2, 10}, {2, 1, 4}, {1, 3, 12}, {2, 4, 14}, {3, 2, 9}, {4, 3, 7}, {3, 5, 20}, {4, 5, 4}} {
		flowG.AddWeightedEdge(int(e[0]), int(e[1]), e[2])
	}
	maxflow, err := flowG.EdmondsKarp(0, 5)
	fmt.Printf("max flow 0->5    = %.1f (expected 23, err=%v)\n", maxflow, err)
	cut, side, err := flowG.MinCut(0, 5)
	fmt.Printf("min cut          = %.1f source side %v (err=%v)\n", cut, side, err)
}

// ---------------------------------------------------------------------------

func printing() {
	section("13. Pretty printing, LaTeX, MathML and step-by-step solutions")

	e := algebra.MustParse("(x^2 + 1)/(sqrt(x) + 3)")
	fmt.Printf("String  : %v\n", e)
	fmt.Printf("LaTeX   : %s\n", algebra.LaTeX(e))
	fmt.Printf("LaTeXEq : %s\n", algebra.LaTeXEq(algebra.Sym("y"), e))
	fmt.Printf("MathML  : %s\n", algebra.MathML(algebra.MustParse("x^2 + 1")))
	fmt.Println("Pretty  :")
	fmt.Println(algebra.Pretty(e))

	x := algebra.Sym("x")

	fmt.Println("--- DifferentiateSteps(x^2*sin(x)) ---")
	fmt.Println(algebra.DifferentiateSteps(algebra.MustParse("x^2*sin(x)"), x))

	fmt.Println("--- SolveQuadraticSteps(x^2 - 5x + 6) ---")
	fmt.Println(algebra.SolveQuadraticSteps(algebra.MustParse("x^2 - 5*x + 6"), x))

	fmt.Println("--- IntegrateSteps(x*exp(x)) ---")
	sol := algebra.IntegrateSteps(algebra.MustParse("x*exp(x)"), x)
	fmt.Printf("title=%q steps=%d result=%v\n", sol.Title, len(sol.Steps), sol.Result)
	for i, st := range sol.Steps {
		fmt.Printf("  step %d: %s\n", i+1, st)
	}
	fmt.Printf("LaTeX of solution (first 120 chars): %.120s...\n", algebra.SolutionLaTeX(sol))
}
