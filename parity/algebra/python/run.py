#!/usr/bin/env python3
"""Upstream (sympy) parity runner for github.com/malcolmston/algebra.

Speaks JSON Lines on stdio: one case object in per line, one result object out
per line, in the same order.  See ../../HARNESS.md and ../COVERAGE.md.

Never exits on a failing case: every exception is reported as {"ok": false}.
Fully deterministic: no randomness, no timestamps, all containers sorted.
"""

import json
import sys

import sympy as sp
from sympy.core.intfunc import igcdex
from sympy.functions.combinatorial.numbers import partition as sp_partition, stirling
from sympy.ntheory.modular import crt as sp_crt
from sympy.ntheory.primetest import is_square as sp_is_square
from sympy.parsing.sympy_parser import (
    convert_xor,
    parse_expr,
    standard_transformations,
)

TRANSFORMS = standard_transformations + (convert_xor,)

# Fixed sample table used when a case does not name its own samples.
# Documented in COVERAGE.md.  Variable i, sample j -> DEFAULT[(j + 2*i) % 5].
DEFAULT_SAMPLES = [0.3170, 0.7391, 1.4142, 2.2360, 3.1415]

PREC = 30


def P(s):
    """Parse a case-file expression string (uses ^ for power, like the Go port)."""
    if isinstance(s, (int, float)):
        return sp.sympify(s)
    return parse_expr(s, transformations=TRANSFORMS)


def sym(name):
    return sp.Symbol(name)


def ratstr(v):
    """Canonical 'p/q' rendering of an exact rational, always with a denominator."""
    r = sp.Rational(sp.nsimplify(sp.simplify(v)))
    return "%d/%d" % (r.p, r.q)


def intstr(v):
    return str(int(v))


def cnum(e):
    """Evaluate a sympy expression to a python complex, or raise."""
    v = sp.N(e, PREC)
    if v.has(sp.zoo) or v.has(sp.nan) or v.has(sp.oo) or v.has(-sp.oo):
        raise ValueError("non-finite value: %s" % v)
    c = complex(v)
    if c.real != c.real or c.imag != c.imag:
        raise ValueError("nan value")
    if abs(c.real) == float("inf") or abs(c.imag) == float("inf"):
        raise ValueError("infinite value")
    return c


def pair(c):
    c = complex(c)
    return [c.real, c.imag]


def sample_points(case):
    variables = case.get("vars") or []
    if not variables:
        return [{}]
    if case.get("samples"):
        return case["samples"]
    pts = []
    for j in range(len(DEFAULT_SAMPLES)):
        pt = {}
        for i, v in enumerate(variables):
            pt[v] = DEFAULT_SAMPLES[(j + 2 * i) % len(DEFAULT_SAMPLES)]
        pts.append(pt)
    return pts


def sample_expr(case, expr):
    """Numeric-mode value: expr evaluated at every sample point, as [re,im] pairs."""
    pts = sample_points(case)
    vals = []
    for pt in pts:
        sub = {sym(k): sp.Float(v, PREC) for k, v in pt.items()}
        vals.append(cnum(expr.subs(sub)))
    if case.get("upToConstant"):
        base = vals[0]
        vals = [v - base for v in vals[1:]]
    return [pair(v) for v in vals]


def as_set(values):
    """Set-mode value: dedup + sort a list of numbers as [re,im] pairs."""
    seen = {}
    for v in values:
        c = complex(v)
        key = (round(c.real, 9), round(c.imag, 9))
        if key not in seen:
            seen[key] = c
    return [pair(seen[k]) for k in sorted(seen)]


def matrix(arg):
    return sp.Matrix([[P(v) if isinstance(v, str) else sp.sympify(v) for v in row] for row in arg])


def flat_exact(m):
    return [ratstr(v) for v in m]


def flat_numeric(m):
    return [pair(cnum(v)) for v in m]


# --------------------------------------------------------------------------
# dispatch
# --------------------------------------------------------------------------


def run(case):
    fn = case["fn"]
    a = case.get("args", [])
    mode = case.get("mode", "exact")

    # ---- calculus -------------------------------------------------------
    if fn == "diff":
        return sample_expr(case, sp.diff(P(a[0]), sym(a[1])))
    if fn == "diff_n":
        return sample_expr(case, sp.diff(P(a[0]), sym(a[1]), int(a[2])))
    if fn == "integrate":
        return sample_expr(case, sp.integrate(P(a[0]), sym(a[1])))
    if fn == "limit":
        return sample_expr(case, sp.limit(P(a[0]), sym(a[1]), P(a[2])))
    if fn == "series":
        e = sp.series(P(a[0]), sym(a[1]), P(a[2]), int(a[3])).removeO()
        return sample_expr(case, e)
    if fn == "summation":
        return sample_expr(case, sp.simplify(sp.summation(P(a[0]), (sym(a[1]), P(a[2]), P(a[3])))))
    if fn == "product":
        return sample_expr(case, sp.simplify(sp.product(P(a[0]), (sym(a[1]), P(a[2]), P(a[3])))))

    # ---- rewriting ------------------------------------------------------
    if fn == "simplify":
        return sample_expr(case, sp.simplify(P(a[0])))
    if fn == "expand":
        return sample_expr(case, sp.expand(P(a[0])))
    if fn == "factor":
        return sample_expr(case, sp.factor(P(a[0]), sym(a[1])))
    if fn == "collect":
        return sample_expr(case, sp.collect(sp.expand(P(a[0])), sym(a[1])))
    if fn == "apart":
        return sample_expr(case, sp.apart(P(a[0]), sym(a[1])))
    if fn == "subs":
        return sample_expr(case, P(a[0]).subs(sym(a[1]), P(a[2])))
    if fn == "evalf":
        return [pair(cnum(P(a[0])))]
    if fn == "str":
        return ["".join(sp.sstr(P(a[0])).split())]

    # ---- factor structure ----------------------------------------------
    if fn == "factor_degrees":
        x = sym(a[1])
        _, facs = sp.factor_list(P(a[0]), x)
        out = sorted([sp.Poly(f, x).degree(), int(m)] for f, m in facs)
        return ["%d:%d" % (d, m) for d, m in out]

    # ---- solving --------------------------------------------------------
    if fn == "solve":
        roots = sp.solve(sp.Eq(P(a[0]), 0), sym(a[1]))
        return as_set(cnum(r) for r in roots)
    if fn == "solve_result_single":
        # Go's Solution.Result for the quadratic stepper; upstream has the full set.
        roots = sp.solve(sp.Eq(P(a[0]), 0), sym(a[1]))
        return as_set(cnum(r) for r in roots)
    if fn == "solve_system":
        eqs = [sp.Eq(P(e), 0) for e in a[0]]
        syms = [sym(s) for s in a[1]]
        sol = sp.solve(eqs, syms, dict=True)
        if not sol:
            raise ValueError("no solution")
        s0 = sol[0]
        return [pair(cnum(s0[s])) for s in syms]
    if fn == "ode1":
        # y' = rhs(x, y); compare the general solution with C1 fixed.
        x = sym(a[1])
        y = sp.Function(a[2])
        rhs = P(a[0]).subs(sym(a[2]), y(x))
        sol = sp.dsolve(sp.Eq(sp.Derivative(y(x), x), rhs), y(x))
        if isinstance(sol, list):
            sol = sol[0]
        expr = sol.rhs.subs(sp.Symbol("C1"), sp.Rational(a[3]))
        return sample_expr(case, expr)
    if fn == "ode2const":
        # a*y'' + b*y' + c*y = g
        x = sym(a[4])
        y = sp.Function(a[5])
        A, B, C, G = (P(a[0]), P(a[1]), P(a[2]), P(a[3]))
        eq = sp.Eq(A * sp.Derivative(y(x), x, 2) + B * sp.Derivative(y(x), x) + C * y(x), G)
        sol = sp.dsolve(eq, y(x))
        expr = sol.rhs.subs({sp.Symbol("C1"): sp.Rational(a[6]), sp.Symbol("C2"): sp.Rational(a[7])})
        return sample_expr(case, expr)

    # ---- polynomials ----------------------------------------------------
    if fn == "poly_gcd":
        x = sym(a[2])
        g = sp.Poly(sp.gcd(P(a[0]), P(a[1])), x).monic()
        return [ratstr(c) for c in reversed(g.all_coeffs())]
    if fn == "discriminant":
        return [ratstr(sp.discriminant(P(a[0]), sym(a[1])))]
    if fn == "resultant":
        return [ratstr(sp.resultant(P(a[0]), P(a[1]), sym(a[1 + 1])))]

    # ---- matrices -------------------------------------------------------
    if fn == "det":
        return [ratstr(matrix(a[0]).det())]
    if fn == "det_numeric":
        return sample_expr(case, matrix(a[0]).det())
    if fn == "det_lu":
        return [pair(cnum(matrix(a[0]).det(method="lu")))]
    if fn == "inverse":
        return flat_exact(matrix(a[0]).inv())
    if fn == "transpose":
        return flat_exact(matrix(a[0]).T)
    if fn == "rank":
        return [intstr(matrix(a[0]).rank())]
    if fn == "charpoly":
        return [ratstr(c) for c in matrix(a[0]).charpoly().all_coeffs()]
    if fn == "eigenvalues":
        vals = matrix(a[0]).eigenvals()
        return as_set(cnum(v) for v in vals)
    if fn == "cholesky":
        return flat_numeric(matrix(a[0]).cholesky())
    if fn == "matmul":
        return flat_exact(matrix(a[0]) * matrix(a[1]))
    if fn == "matpow":
        return flat_exact(matrix(a[0]) ** int(a[1]))
    if fn == "kron":
        from sympy.physics.quantum import TensorProduct

        return flat_exact(TensorProduct(matrix(a[0]), matrix(a[1])))
    if fn == "mat_solve":
        m = matrix(a[0])
        b = sp.Matrix([[P(v) if isinstance(v, str) else sp.sympify(v)] for v in a[1]])
        return flat_exact(m.solve(b))
    if fn == "adjugate":
        return flat_exact(matrix(a[0]).adjugate())
    if fn == "norm_fro":
        return [pair(cnum(matrix(a[0]).norm()))]
    if fn == "rref":
        return flat_exact(matrix(a[0]).rref()[0])

    # ---- number theory --------------------------------------------------
    if fn == "gcd":
        return [intstr(sp.igcd(int(a[0]), int(a[1])))]
    if fn == "lcm":
        return [intstr(sp.ilcm(int(a[0]), int(a[1])))]
    if fn == "extended_gcd":
        p, q, g = igcdex(int(a[0]), int(a[1]))
        return [intstr(g), intstr(p), intstr(q)]
    if fn == "totient":
        return [intstr(sp.totient(int(a[0])))]
    if fn == "mobius":
        return [intstr(sp.mobius(int(a[0])))]
    if fn == "factorial":
        return [intstr(sp.factorial(int(a[0])))]
    if fn == "binomial":
        return [intstr(sp.binomial(int(a[0]), int(a[1])))]
    if fn == "permutations":
        return [intstr(sp.ff(int(a[0]), int(a[1])))]
    if fn == "multinomial":
        ks = [int(k) for k in a]
        v = sp.factorial(sum(ks))
        for k in ks:
            v /= sp.factorial(k)
        return [intstr(v)]
    if fn == "catalan":
        return [intstr(sp.catalan(int(a[0])))]
    if fn == "bernoulli":
        return [ratstr(sp.bernoulli(int(a[0])))]
    if fn == "fibonacci":
        return [intstr(sp.fibonacci(int(a[0])))]
    if fn == "lucas":
        return [intstr(sp.lucas(int(a[0])))]
    if fn == "tribonacci":
        return [intstr(sp.tribonacci(int(a[0])))]
    if fn == "partition":
        return [intstr(sp_partition(int(a[0])))]
    if fn == "factorint":
        d = sp.factorint(int(a[0]))
        return ["%d^%d" % (p, d[p]) for p in sorted(d)]
    if fn == "isprime":
        return ["true" if sp.isprime(int(a[0])) else "false"]
    if fn == "nextprime":
        return [intstr(sp.nextprime(int(a[0])))]
    if fn == "primepi":
        return [intstr(sp.primepi(int(a[0])))]
    if fn == "divisors":
        return [intstr(d) for d in sorted(sp.divisors(int(a[0])))]
    if fn == "divisor_sigma":
        return [intstr(sp.divisor_sigma(int(a[0]), int(a[1])))]
    if fn == "divisor_count":
        return [intstr(sp.divisor_count(int(a[0])))]
    if fn == "radical":
        v = 1
        for p in sp.primefactors(int(a[0])):
            v *= p
        return [intstr(v)]
    if fn == "stirling2":
        return [intstr(stirling(int(a[0]), int(a[1])))]
    if fn == "modpow":
        return [intstr(pow(int(a[0]), int(a[1]), int(a[2])))]
    if fn == "modinverse":
        return [intstr(sp.mod_inverse(int(a[0]), int(a[1])))]
    if fn == "crt":
        res = sp_crt([int(m) for m in a[1]], [int(r) for r in a[0]])
        if res is None:
            raise ValueError("no CRT solution")
        return [intstr(res[0]), intstr(res[1])]
    if fn == "jacobi":
        return [intstr(sp.jacobi_symbol(int(a[0]), int(a[1])))]
    if fn == "legendre":
        return [intstr(sp.legendre_symbol(int(a[0]), int(a[1])))]
    if fn == "discrete_log":
        return [intstr(sp.discrete_log(int(a[2]), int(a[1]), int(a[0])))]
    if fn == "is_square":
        return ["true" if sp_is_square(int(a[0])) else "false"]
    if fn == "sqrt_mod":
        v = sp.sqrt_mod(int(a[0]), int(a[1]))
        if v is None:
            raise ValueError("no square root")
        # both roots are correct; report the canonical smaller one
        v, m = int(v), int(a[1])
        return [intstr(min(v % m, (m - v) % m))]
    if fn == "multiplicative_order":
        return [intstr(sp.n_order(int(a[0]), int(a[1])))]

    raise ValueError("unknown fn %r" % fn)


def main():
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        case = json.loads(line)
        try:
            value = run(case)
            res = {"id": case["id"], "ok": True, "value": value}
        except Exception as exc:  # noqa: BLE001 - never die on a case
            res = {"id": case["id"], "ok": False, "error": "%s: %s" % (type(exc).__name__, exc)}
        out.write(json.dumps(res, sort_keys=True) + "\n")
        out.flush()


if __name__ == "__main__":
    main()
