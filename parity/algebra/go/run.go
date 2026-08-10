// Command run is the Go-side parity runner for github.com/malcolmston/algebra.
//
// It speaks JSON Lines on stdio: one case object in per line, one result object
// out per line, in the same order.  See ../../HARNESS.md and ../COVERAGE.md.
//
// Never exits on a failing case: every error and panic is reported as
// {"ok": false}.  Fully deterministic: no randomness, no timestamps.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	a "github.com/malcolmston/algebra"
	mx "github.com/malcolmston/algebra/matrix"
	nt "github.com/malcolmston/algebra/ntheory"
)

// DefaultSamples mirrors DEFAULT_SAMPLES in ../python/run.py.
var DefaultSamples = []float64{0.3170, 0.7391, 1.4142, 2.2360, 3.1415}

type Case struct {
	ID           string               `json:"id"`
	Fn           string               `json:"fn"`
	Args         []json.RawMessage    `json:"args"`
	Mode         string               `json:"mode"`
	Vars         []string             `json:"vars"`
	Samples      []map[string]float64 `json:"samples"`
	UpToConstant bool                 `json:"upToConstant"`
}

type Result struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// ---------------------------------------------------------------- arg helpers

func str(r json.RawMessage) string {
	var s string
	if err := json.Unmarshal(r, &s); err == nil {
		return s
	}
	var f float64
	if err := json.Unmarshal(r, &f); err == nil {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	panic(fmt.Sprintf("arg is not a string: %s", r))
}

func i64(r json.RawMessage) int64 {
	var f float64
	if err := json.Unmarshal(r, &f); err != nil {
		panic(fmt.Sprintf("arg is not a number: %s", r))
	}
	return int64(f)
}

func i64s(r json.RawMessage) []int64 {
	var fs []float64
	if err := json.Unmarshal(r, &fs); err != nil {
		panic(fmt.Sprintf("arg is not a number list: %s", r))
	}
	out := make([]int64, len(fs))
	for i, f := range fs {
		out[i] = int64(f)
	}
	return out
}

func strs(r json.RawMessage) []string {
	var ss []string
	if err := json.Unmarshal(r, &ss); err != nil {
		panic(fmt.Sprintf("arg is not a string list: %s", r))
	}
	return ss
}

// P parses a case-file expression string ('^' for power, as the port's Parse wants).
func P(s string) a.Expr {
	e, err := a.Parse(s)
	if err != nil {
		panic(err)
	}
	return e
}

func pexpr(r json.RawMessage) a.Expr { return P(str(r)) }

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("bad rational literal " + s)
	}
	return r
}

// ------------------------------------------------------------- value encoding

// exactStr renders an Expr that must be an exact rational as canonical "p/q".
func exactStr(e a.Expr) string {
	switch v := a.Simplify(e).(type) {
	case *a.Integer:
		return new(big.Rat).SetInt(v.Val).String()
	case *a.Rational:
		return v.Val.String()
	}
	panic(fmt.Sprintf("not an exact rational: %v", e))
}

func pair(c complex128) []float64 {
	if math.IsNaN(real(c)) || math.IsNaN(imag(c)) ||
		math.IsInf(real(c), 0) || math.IsInf(imag(c), 0) {
		panic(fmt.Sprintf("non-finite value: %v", c))
	}
	return []float64{real(c), imag(c)}
}

func cnum(e a.Expr, env map[string]complex128) complex128 {
	c, err := a.EvalComplex(e, env)
	if err != nil {
		panic(err)
	}
	if math.IsNaN(real(c)) || math.IsNaN(imag(c)) {
		panic(fmt.Sprintf("nan value from %v", e))
	}
	return c
}

func samplePoints(c *Case) []map[string]float64 {
	if len(c.Vars) == 0 {
		return []map[string]float64{{}}
	}
	if len(c.Samples) > 0 {
		return c.Samples
	}
	pts := make([]map[string]float64, 0, len(DefaultSamples))
	for j := range DefaultSamples {
		pt := map[string]float64{}
		for i, v := range c.Vars {
			pt[v] = DefaultSamples[(j+2*i)%len(DefaultSamples)]
		}
		pts = append(pts, pt)
	}
	return pts
}

func sampleExpr(c *Case, e a.Expr) [][]float64 {
	pts := samplePoints(c)
	vals := make([]complex128, 0, len(pts))
	for _, pt := range pts {
		env := map[string]complex128{}
		for k, v := range pt {
			env[k] = complex(v, 0)
		}
		vals = append(vals, cnum(e, env))
	}
	if c.UpToConstant {
		base := vals[0]
		shifted := make([]complex128, 0, len(vals)-1)
		for _, v := range vals[1:] {
			shifted = append(shifted, v-base)
		}
		vals = shifted
	}
	out := make([][]float64, 0, len(vals))
	for _, v := range vals {
		out = append(out, pair(v))
	}
	return out
}

func asSet(vals []complex128) [][]float64 {
	round := func(f float64) float64 { return math.Round(f*1e9) / 1e9 }
	seen := map[[2]float64]complex128{}
	keys := make([][2]float64, 0, len(vals))
	for _, v := range vals {
		if math.IsNaN(real(v)) || math.IsNaN(imag(v)) {
			panic("nan in solution set")
		}
		k := [2]float64{round(real(v)), round(imag(v))}
		if _, dup := seen[k]; !dup {
			seen[k] = v
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	out := make([][]float64, 0, len(keys))
	for _, k := range keys {
		out = append(out, pair(seen[k]))
	}
	return out
}

func evalSet(es []a.Expr) [][]float64 {
	vals := make([]complex128, 0, len(es))
	for _, e := range es {
		vals = append(vals, cnum(e, map[string]complex128{}))
	}
	return asSet(vals)
}

// ------------------------------------------------------------------- matrices

func mat(r json.RawMessage) *mx.Matrix {
	var rows []([]json.RawMessage)
	if err := json.Unmarshal(r, &rows); err != nil {
		panic(fmt.Sprintf("arg is not a matrix: %s", r))
	}
	vals := make([][]a.Expr, len(rows))
	for i, row := range rows {
		vals[i] = make([]a.Expr, len(row))
		for j, cell := range row {
			var s string
			if err := json.Unmarshal(cell, &s); err == nil {
				vals[i][j] = P(s)
				continue
			}
			var f float64
			if err := json.Unmarshal(cell, &f); err != nil {
				panic("bad matrix cell")
			}
			if f == math.Trunc(f) {
				vals[i][j] = a.Int(int64(f))
			} else {
				vals[i][j] = a.Flt(f)
			}
		}
	}
	return mx.FromExpr(vals)
}

func vec(r json.RawMessage) *mx.Vector {
	var cells []json.RawMessage
	if err := json.Unmarshal(r, &cells); err != nil {
		panic("arg is not a vector")
	}
	comps := make([]a.Expr, len(cells))
	for i, cell := range cells {
		var s string
		if err := json.Unmarshal(cell, &s); err == nil {
			comps[i] = P(s)
			continue
		}
		comps[i] = a.Int(i64(cell))
	}
	return mx.NewVector(comps...)
}

func flatExact(m *mx.Matrix) []string {
	out := make([]string, 0, m.Rows()*m.Cols())
	for i := 0; i < m.Rows(); i++ {
		for j := 0; j < m.Cols(); j++ {
			out = append(out, exactStr(m.At(i, j)))
		}
	}
	return out
}

func flatNumeric(m *mx.Matrix) [][]float64 {
	out := make([][]float64, 0, m.Rows()*m.Cols())
	for i := 0; i < m.Rows(); i++ {
		for j := 0; j < m.Cols(); j++ {
			out = append(out, pair(cnum(m.At(i, j), map[string]complex128{})))
		}
	}
	return out
}

func vecExact(v *mx.Vector) []string {
	out := make([]string, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		out = append(out, exactStr(v.At(i)))
	}
	return out
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// bigStr renders a *big.Int as a plain decimal string.
func bigStr(v *big.Int) string { return v.String() }

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ------------------------------------------------------------------- dispatch

func run(c *Case) interface{} {
	args := c.Args
	switch c.Fn {

	// ---- calculus -------------------------------------------------------
	case "diff":
		return sampleExpr(c, a.Diff(pexpr(args[0]), a.Sym(str(args[1]))))
	case "diff_n":
		e := pexpr(args[0])
		v := a.Sym(str(args[1]))
		for k := int64(0); k < i64(args[2]); k++ {
			e = a.Diff(e, v)
		}
		return sampleExpr(c, e)
	case "integrate":
		return sampleExpr(c, a.Integrate(pexpr(args[0]), a.Sym(str(args[1]))))
	case "limit":
		return sampleExpr(c, a.Limit(pexpr(args[0]), a.Sym(str(args[1])), pexpr(args[2])))
	case "series":
		return sampleExpr(c, a.Series(pexpr(args[0]), a.Sym(str(args[1])), pexpr(args[2]), int(i64(args[3]))))
	case "summation":
		return sampleExpr(c, a.Summation(pexpr(args[0]), a.Sym(str(args[1])), pexpr(args[2]), pexpr(args[3])))
	case "product":
		return sampleExpr(c, a.Product(pexpr(args[0]), a.Sym(str(args[1])), pexpr(args[2]), pexpr(args[3])))

	// ---- rewriting ------------------------------------------------------
	case "simplify":
		return sampleExpr(c, a.Simplify(pexpr(args[0])))
	case "expand":
		return sampleExpr(c, a.Expand(pexpr(args[0])))
	case "factor":
		return sampleExpr(c, a.Factor(pexpr(args[0]), a.Sym(str(args[1]))))
	case "collect":
		return sampleExpr(c, a.Collect(a.Expand(pexpr(args[0])), a.Sym(str(args[1]))))
	case "apart":
		return sampleExpr(c, must(a.ApartExpr(pexpr(args[0]), a.Sym(str(args[1])))))
	case "subs":
		return sampleExpr(c, a.Subs(pexpr(args[0]), a.Sym(str(args[1])), pexpr(args[2])))
	case "evalf":
		return [][]float64{pair(cnum(pexpr(args[0]), map[string]complex128{}))}
	case "str":
		return []string{strings.Join(strings.Fields(pexpr(args[0]).String()), "")}

	// ---- factor structure ----------------------------------------------
	case "factor_degrees":
		p := must(a.PolyFrom(pexpr(args[0]), a.Sym(str(args[1]))))
		facs := p.Factor()
		type dm struct{ deg, mult int }
		pairs := make([]dm, 0, len(facs))
		for _, f := range facs {
			if f.Base.Degree() <= 0 {
				continue
			}
			pairs = append(pairs, dm{f.Base.Degree(), f.Mult})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].deg != pairs[j].deg {
				return pairs[i].deg < pairs[j].deg
			}
			return pairs[i].mult < pairs[j].mult
		})
		out := make([]string, 0, len(pairs))
		for _, q := range pairs {
			out = append(out, fmt.Sprintf("%d:%d", q.deg, q.mult))
		}
		return out

	// ---- solving --------------------------------------------------------
	case "solve":
		return evalSet(must(a.Solve(pexpr(args[0]), a.Sym(str(args[1])))))
	case "solve_result_single":
		sol := a.SolveQuadraticSteps(pexpr(args[0]), a.Sym(str(args[1])))
		if sol == nil || sol.Result == nil {
			panic("no solution")
		}
		return evalSet([]a.Expr{sol.Result})
	case "solve_system":
		eqs := make([]a.Expr, 0)
		for _, s := range strs(args[0]) {
			eqs = append(eqs, P(s))
		}
		syms := make([]a.Expr, 0)
		for _, s := range strs(args[1]) {
			syms = append(syms, a.Sym(s))
		}
		vals := must(a.SolveSystem(eqs, syms))
		out := make([][]float64, 0, len(vals))
		for _, v := range vals {
			out = append(out, pair(cnum(v, map[string]complex128{})))
		}
		return out
	case "ode1":
		sol := must(a.SolveODE1(pexpr(args[0]), a.Sym(str(args[1])), a.Sym(str(args[2]))))
		if sol.Kind != "" && strings.HasSuffix(sol.Kind, "-implicit") {
			panic("implicit solution: " + sol.General.String())
		}
		g := sol.General
		for _, k := range sol.Constants {
			g = a.Subs(g, k, ratExpr(str(args[3])))
		}
		return sampleExpr(c, g)
	case "ode2const":
		sol := must(a.SolveODE2Const(pexpr(args[0]), pexpr(args[1]), pexpr(args[2]), pexpr(args[3]),
			a.Sym(str(args[4])), a.Sym(str(args[5]))))
		g := sol.General
		consts := []string{str(args[6]), str(args[7])}
		for i, k := range sol.Constants {
			if i < len(consts) {
				g = a.Subs(g, k, ratExpr(consts[i]))
			}
		}
		return sampleExpr(c, g)

	// ---- polynomials ----------------------------------------------------
	case "poly_gcd":
		v := a.Sym(str(args[2]))
		p := must(a.PolyFrom(pexpr(args[0]), v))
		q := must(a.PolyFrom(pexpr(args[1]), v))
		g := a.PolyGCD(p, q).Monic()
		out := make([]string, 0, g.Degree()+1)
		for i := 0; i <= g.Degree(); i++ {
			out = append(out, exactStr(g.Coeff(i)))
		}
		return out
	case "discriminant":
		p := must(a.PolyFrom(pexpr(args[0]), a.Sym(str(args[1]))))
		return []string{exactStr(p.Discriminant())}
	case "resultant":
		v := a.Sym(str(args[2]))
		p := must(a.PolyFrom(pexpr(args[0]), v))
		q := must(a.PolyFrom(pexpr(args[1]), v))
		return []string{exactStr(p.Resultant(q))}

	// ---- matrices -------------------------------------------------------
	case "det":
		return []string{exactStr(must(mat(args[0]).Det()))}
	case "det_numeric":
		return sampleExpr(c, must(mat(args[0]).Det()))
	case "det_lu":
		return [][]float64{pair(cnum(must(mat(args[0]).DetLU()), map[string]complex128{}))}
	case "inverse":
		return flatExact(must(mat(args[0]).Inverse()))
	case "transpose":
		return flatExact(mat(args[0]).Transpose())
	case "rank":
		return []string{strconv.Itoa(mat(args[0]).Rank())}
	case "charpoly":
		cp := must(mat(args[0]).CharPoly())
		p := must(a.PolyFrom(cp, mx.Lambda))
		out := make([]string, 0, p.Degree()+1)
		for i := p.Degree(); i >= 0; i-- { // high power first, like sympy all_coeffs
			out = append(out, exactStr(p.Coeff(i)))
		}
		return out
	case "eigenvalues":
		return evalSet(must(mat(args[0]).Eigenvalues()))
	case "cholesky":
		return flatNumeric(must(mat(args[0]).Cholesky()))
	case "matmul":
		return flatExact(must(mat(args[0]).Mul(mat(args[1]))))
	case "matpow":
		return flatExact(must(mat(args[0]).Pow(int(i64(args[1])))))
	case "kron":
		return flatExact(mat(args[0]).Kron(mat(args[1])))
	case "mat_solve":
		return vecExact(must(mx.Solve(mat(args[0]), vec(args[1]))))
	case "adjugate":
		return flatExact(must(mat(args[0]).Adjugate()))
	case "norm_fro":
		return [][]float64{pair(cnum(mat(args[0]).NormFro(), map[string]complex128{}))}
	case "rref":
		r, _ := mat(args[0]).RREF()
		return flatExact(r)

	// ---- number theory --------------------------------------------------
	case "gcd":
		return []string{strconv.FormatInt(nt.GCD(i64(args[0]), i64(args[1])), 10)}
	case "lcm":
		return []string{strconv.FormatInt(nt.LCM(i64(args[0]), i64(args[1])), 10)}
	case "extended_gcd":
		g, x, y := nt.ExtendedGCD(i64(args[0]), i64(args[1]))
		return []string{strconv.FormatInt(g, 10), strconv.FormatInt(x, 10), strconv.FormatInt(y, 10)}
	case "totient":
		return []string{strconv.FormatInt(nt.EulerPhi(i64(args[0])), 10)}
	case "mobius":
		return []string{strconv.Itoa(nt.MobiusMu(i64(args[0])))}
	case "factorial":
		return []string{bigStr(nt.Factorial(i64(args[0])))}
	case "binomial":
		return []string{bigStr(nt.Binomial(i64(args[0]), i64(args[1])))}
	case "permutations":
		return []string{bigStr(nt.Permutations(i64(args[0]), i64(args[1])))}
	case "multinomial":
		ks := make([]int64, 0, len(args))
		for _, r := range args {
			ks = append(ks, i64(r))
		}
		return []string{bigStr(nt.Multinomial(ks...))}
	case "catalan":
		return []string{bigStr(nt.CatalanNumber(i64(args[0])))}
	case "bernoulli":
		return []string{nt.Bernoulli(i64(args[0])).String()}
	case "fibonacci":
		return []string{bigStr(nt.Fibonacci(i64(args[0])))}
	case "lucas":
		return []string{bigStr(nt.Lucas(i64(args[0])))}
	case "tribonacci":
		return []string{bigStr(nt.Tribonacci(i64(args[0])))}
	case "partition":
		return []string{bigStr(nt.Partition(i64(args[0])))}
	case "factorint":
		fl := nt.FactorList(i64(args[0]))
		sort.Slice(fl, func(i, j int) bool { return fl[i].Prime < fl[j].Prime })
		out := make([]string, 0, len(fl))
		for _, pp := range fl {
			out = append(out, fmt.Sprintf("%d^%d", pp.Prime, pp.Exponent))
		}
		return out
	case "isprime":
		return []string{boolStr(nt.IsPrime(i64(args[0])))}
	case "nextprime":
		return []string{strconv.FormatInt(nt.NextPrime(i64(args[0])), 10)}
	case "primepi":
		return []string{strconv.FormatInt(nt.PrimePi(i64(args[0])), 10)}
	case "divisors":
		ds := nt.Divisors(i64(args[0]))
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		out := make([]string, 0, len(ds))
		for _, d := range ds {
			out = append(out, strconv.FormatInt(d, 10))
		}
		return out
	case "divisor_sigma":
		return []string{strconv.FormatInt(nt.DivisorSigma(i64(args[1]), i64(args[0])), 10)}
	case "divisor_count":
		return []string{strconv.FormatInt(nt.CountDivisors(i64(args[0])), 10)}
	case "radical":
		return []string{strconv.FormatInt(nt.Radical(i64(args[0])), 10)}
	case "stirling2":
		return []string{bigStr(nt.StirlingSecond(i64(args[0]), i64(args[1])))}
	case "modpow":
		return []string{strconv.FormatInt(nt.ModPow(i64(args[0]), i64(args[1]), i64(args[2])), 10)}
	case "modinverse":
		inv, ok := nt.ModInverse(i64(args[0]), i64(args[1]))
		if !ok {
			panic("no modular inverse")
		}
		return []string{strconv.FormatInt(inv, 10)}
	case "crt":
		x, m, ok := nt.CRT(i64s(args[0]), i64s(args[1]))
		if !ok {
			panic("no CRT solution")
		}
		return []string{strconv.FormatInt(x, 10), strconv.FormatInt(m, 10)}
	case "jacobi":
		return []string{strconv.Itoa(nt.JacobiSymbol(i64(args[0]), i64(args[1])))}
	case "legendre":
		return []string{strconv.Itoa(nt.LegendreSymbol(i64(args[0]), i64(args[1])))}
	case "discrete_log":
		x, ok := nt.DiscreteLog(i64(args[0]), i64(args[1]), i64(args[2]))
		if !ok {
			panic("no discrete logarithm")
		}
		return []string{strconv.FormatInt(x, 10)}
	case "is_square":
		return []string{boolStr(nt.IsSquare(i64(args[0])))}
	case "sqrt_mod":
		x, ok := nt.SqrtMod(i64(args[0]), i64(args[1]))
		if !ok {
			panic("no square root")
		}
		// both roots are correct; report the canonical smaller one
		m := i64(args[1])
		x = ((x % m) + m) % m
		if m-x < x {
			x = m - x
		}
		return []string{strconv.FormatInt(x, 10)}
	case "multiplicative_order":
		ord, ok := nt.Order(i64(args[0]), i64(args[1]))
		if !ok {
			panic("no multiplicative order")
		}
		return []string{strconv.FormatInt(ord, 10)}
	}
	panic("unknown fn " + c.Fn)
}

// ratExpr turns a decimal/fraction literal from a case file into an exact Expr.
func ratExpr(s string) a.Expr {
	r := mustRat(s)
	if r.IsInt() {
		return a.IntBig(r.Num())
	}
	return a.Rat(r.Num().Int64(), r.Denom().Int64())
}

func execute(c *Case) (res Result) {
	res.ID = c.ID
	defer func() {
		if r := recover(); r != nil {
			res = Result{ID: c.ID, OK: false, Error: fmt.Sprint(r)}
		}
	}()
	v := run(c)
	return Result{ID: c.ID, OK: true, Value: v}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<22)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var c Case
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			fmt.Fprintln(os.Stderr, "bad case line:", err)
			continue
		}
		if err := enc.Encode(execute(&c)); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
		}
	}
}
