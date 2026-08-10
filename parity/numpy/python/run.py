#!/usr/bin/env python3
"""Upstream (numpy) parity runner.

Speaks JSON Lines on stdio: one request object per line in, one response
object per line out, in the same order. See ../../HARNESS.md.

Value encoding (identical on the Go side):
  ndarray -> {"kind":"array","shape":[...],"data":<nested lists>}
  float   -> {"kind":"scalar","value":<number|sentinel>}
  int     -> {"kind":"int","value":<int>}
  bool    -> {"kind":"bool","value":<bool>}
Non-finite float64 values are encoded as the sentinel strings "NaN",
"Infinity" and "-Infinity" because JSON has no literal for them.

Argument encoding: an array argument is the object {"arr": <nested lists>};
everything else is a plain JSON scalar / list of ints.
"""

import json
import math
import sys

import numpy as np

SENT = {"NaN": math.nan, "Infinity": math.inf, "-Infinity": -math.inf}


# ---------------------------------------------------------------- decoding
def dec_num(v):
    if isinstance(v, str):
        if v not in SENT:
            raise ValueError("unknown numeric sentinel: %r" % (v,))
        return SENT[v]
    return float(v)


def dec_nested(v):
    if isinstance(v, list):
        return [dec_nested(x) for x in v]
    return dec_num(v)


def dec_arg(v):
    if isinstance(v, dict) and "arr" in v:
        return np.array(dec_nested(v["arr"]), dtype=np.float64)
    return v


# ---------------------------------------------------------------- encoding
def enc_num(x):
    x = float(x)
    if math.isnan(x):
        return "NaN"
    if x == math.inf:
        return "Infinity"
    if x == -math.inf:
        return "-Infinity"
    return x


def enc_nested(a):
    if a.ndim == 0:
        return enc_num(a.item())
    if a.ndim == 1:
        return [enc_num(x) for x in a.tolist()]
    return [enc_nested(a[i]) for i in range(a.shape[0])]


def enc(v):
    if isinstance(v, np.ndarray):
        return {"kind": "array", "shape": list(v.shape), "data": enc_nested(v)}
    if isinstance(v, (bool, np.bool_)):
        return {"kind": "bool", "value": bool(v)}
    if isinstance(v, (int, np.integer)):
        return {"kind": "int", "value": int(v)}
    if isinstance(v, (float, np.floating)):
        return {"kind": "scalar", "value": enc_num(v)}
    raise TypeError("cannot encode %r" % (type(v),))


# ---------------------------------------------------------------- helpers
def mk_slices(pairs, ndim):
    """A [0,0] pair means "the whole axis" (matching the Go port's zero Range)."""
    if len(pairs) != ndim:
        raise ValueError("slice: expected %d ranges, got %d" % (ndim, len(pairs)))
    out = []
    for s, e in pairs:
        out.append(slice(None) if (s == 0 and e == 0) else slice(s, e))
    return tuple(out)


def mask(b):
    return b.astype(np.float64)


def flat_sort(a):
    return np.sort(a, axis=None)


# ---------------------------------------------------------------- dispatch
OPS = {
    # ---- creation
    "zeros": lambda sh: np.zeros(tuple(sh), dtype=np.float64),
    "ones": lambda sh: np.ones(tuple(sh), dtype=np.float64),
    "full": lambda sh, v: np.full(tuple(sh), dec_num(v), dtype=np.float64),
    "arange": lambda a, b, c: np.arange(dec_num(a), dec_num(b), dec_num(c), dtype=np.float64),
    "linspace": lambda a, b, n, ep: np.linspace(dec_num(a), dec_num(b), n, endpoint=ep, dtype=np.float64),
    "eye": lambda n, m, k: np.eye(n, m, k, dtype=np.float64),
    "identity": lambda n: np.identity(n, dtype=np.float64),
    "zeros_like": lambda a: np.zeros_like(a),
    "ones_like": lambda a: np.ones_like(a),
    "from_nested": lambda a: a,
    "full_scalar": lambda sh, v: np.full(tuple(sh), dec_num(v), dtype=np.float64),
    # ---- shape
    "reshape": lambda a, sh: a.reshape(tuple(sh)),
    "transpose": lambda a: a.transpose(),
    "transpose_axes": lambda a, ax: a.transpose(tuple(ax)),
    "ravel": lambda a: a.ravel(),
    "flatten": lambda a: a.flatten(),
    "squeeze": lambda a: np.squeeze(a),
    "expand_dims": lambda a, ax: np.expand_dims(a, ax),
    "concatenate": lambda ax, *xs: np.concatenate(xs, axis=ax),
    "stack": lambda ax, *xs: np.stack(xs, axis=ax),
    "flip": lambda a, ax: np.flip(a, axis=ax),
    "roll": lambda a, n: np.roll(a, n),
    "diag": lambda a: np.diag(a),
    "diagonal": lambda a, k: np.diagonal(a, offset=k).copy(),
    "shape": lambda a: np.array([float(x) for x in a.shape], dtype=np.float64),
    "ndim": lambda a: int(a.ndim),
    "size": lambda a: int(a.size),
    # ---- indexing / slicing
    "slice": lambda a, pairs: np.ascontiguousarray(a[mk_slices(pairs, a.ndim)]),
    "at": lambda a, idx: float(a[tuple(idx)]),
    # ---- broadcasting
    "broadcast_to": lambda a, sh: np.ascontiguousarray(np.broadcast_to(a, tuple(sh))),
    # ---- elementwise arithmetic
    "add": lambda a, b: a + b,
    "sub": lambda a, b: a - b,
    "mul": lambda a, b: a * b,
    "div": lambda a, b: a / b,
    "pow": lambda a, b: a ** b,
    "mod": lambda a, b: np.fmod(a, b),
    "add_scalar": lambda a, s: a + dec_num(s),
    "sub_scalar": lambda a, s: a - dec_num(s),
    "mul_scalar": lambda a, s: a * dec_num(s),
    "div_scalar": lambda a, s: a / dec_num(s),
    "pow_scalar": lambda a, s: a ** dec_num(s),
    # ---- unary ufuncs
    "neg": lambda a: -a,
    "abs": lambda a: np.abs(a),
    "sqrt": lambda a: np.sqrt(a),
    "cbrt": lambda a: np.cbrt(a),
    "square": lambda a: np.square(a),
    "reciprocal": lambda a: 1.0 / a,
    "exp": lambda a: np.exp(a),
    "expm1": lambda a: np.expm1(a),
    "log": lambda a: np.log(a),
    "log1p": lambda a: np.log1p(a),
    "log2": lambda a: np.log2(a),
    "log10": lambda a: np.log10(a),
    "sin": lambda a: np.sin(a),
    "cos": lambda a: np.cos(a),
    "tan": lambda a: np.tan(a),
    "arcsin": lambda a: np.arcsin(a),
    "arccos": lambda a: np.arccos(a),
    "arctan": lambda a: np.arctan(a),
    "sinh": lambda a: np.sinh(a),
    "cosh": lambda a: np.cosh(a),
    "tanh": lambda a: np.tanh(a),
    "floor": lambda a: np.floor(a),
    "ceil": lambda a: np.ceil(a),
    "trunc": lambda a: np.trunc(a),
    "round": lambda a: np.round(a),
    "sign": lambda a: np.sign(a),
    "clip": lambda a, lo, hi: np.clip(a, dec_num(lo), dec_num(hi)),
    # ---- binary ufuncs
    "maximum": lambda a, b: np.maximum(a, b),
    "minimum": lambda a, b: np.minimum(a, b),
    "hypot": lambda a, b: np.hypot(a, b),
    "arctan2": lambda a, b: np.arctan2(a, b),
    # ---- linalg
    "matmul": lambda a, b: a @ b,
    "dot": lambda a, b: np.dot(a, b),
    "outer": lambda a, b: np.outer(a, b),
    "trace": lambda a: float(np.trace(a)),
    "norm": lambda a: float(np.sqrt(np.sum(a * a))),
    "cross": lambda a, b: np.cross(a, b),
    # ---- reductions
    "sum": lambda a: float(np.sum(a)),
    "mean": lambda a: float(np.mean(a)),
    "max": lambda a: float(np.max(a)),
    "min": lambda a: float(np.min(a)),
    "prod": lambda a: float(np.prod(a)),
    "ptp": lambda a: float(np.ptp(a)),
    "all": lambda a: bool(np.all(a)),
    "any": lambda a: bool(np.any(a)),
    "sum_axis": lambda a, ax, kd: np.sum(a, axis=ax, keepdims=kd),
    "mean_axis": lambda a, ax, kd: np.mean(a, axis=ax, keepdims=kd),
    "max_axis": lambda a, ax, kd: np.max(a, axis=ax, keepdims=kd),
    "min_axis": lambda a, ax, kd: np.min(a, axis=ax, keepdims=kd),
    "prod_axis": lambda a, ax, kd: np.prod(a, axis=ax, keepdims=kd),
    "std_axis": lambda a, ax, kd: np.std(a, axis=ax, keepdims=kd),
    # ---- sorting / searching
    "sort": lambda a: flat_sort(a),
    "argsort": lambda a: np.argsort(a, axis=None, kind="stable").astype(np.float64),
    "argmax": lambda a: int(np.argmax(a)),
    "argmin": lambda a: int(np.argmin(a)),
    "unique": lambda a: np.unique(a),
    "searchsorted": lambda a, v: int(np.searchsorted(a, dec_num(v), side="left")),
    "where": lambda m, x, y: np.where(m != 0, x, y),
    "mask_select": lambda a, m: a[np.broadcast_to(m, a.shape) != 0],
    "greater": lambda a, b: mask(a > b),
    "greater_equal": lambda a, b: mask(a >= b),
    "less": lambda a, b: mask(a < b),
    "less_equal": lambda a, b: mask(a <= b),
    "equal_mask": lambda a, b: mask(a == b),
    "not_equal_mask": lambda a, b: mask(a != b),
    "greater_scalar": lambda a, s: mask(a > dec_num(s)),
    "less_scalar": lambda a, s: mask(a < dec_num(s)),
    "equal_scalar": lambda a, s: mask(a == dec_num(s)),
    "array_equal": lambda a, b: bool(a.shape == b.shape and np.array_equal(a, b)),
    "allclose": lambda a, b, tol: bool(
        a.shape == b.shape and np.all(np.abs(a - b) <= dec_num(tol))
    ),
    # ---- statistics
    "std": lambda a: float(np.std(a)),
    "var": lambda a: float(np.var(a)),
    "std_ddof": lambda a, d: float(np.std(a, ddof=d)),
    "var_ddof": lambda a, d: float(np.var(a, ddof=d)),
    "median": lambda a: float(np.median(a)),
    "percentile": lambda a, q: float(np.percentile(a, dec_num(q), method="linear")),
    "quantile": lambda a, q: float(np.quantile(a, dec_num(q), method="linear")),
    # ---- cumulative
    "cumsum": lambda a: np.cumsum(a),
    "cumprod": lambda a: np.cumprod(a),
    "diff": lambda a: np.diff(a.ravel()),
}


def handle(req):
    fn = req.get("fn")
    if fn not in OPS:
        raise KeyError("unknown fn: %s" % (fn,))
    args = [dec_arg(a) for a in req.get("args", [])]
    return enc(OPS[fn](*args))


def main():
    np.seterr(all="ignore")
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:  # unparseable line: nothing sensible to echo
            out.write(json.dumps({"id": "?", "ok": False, "error": str(exc)}) + "\n")
            out.flush()
            continue
        cid = req.get("id", "?")
        try:
            out.write(json.dumps({"id": cid, "ok": True, "value": handle(req)},
                                 allow_nan=False, sort_keys=True) + "\n")
        except BaseException as exc:  # never exit on a failing case
            out.write(json.dumps({"id": cid, "ok": False,
                                  "error": "%s: %s" % (type(exc).__name__, exc)},
                                 sort_keys=True) + "\n")
        out.flush()


if __name__ == "__main__":
    main()
