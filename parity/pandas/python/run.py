#!/usr/bin/env python3
"""Upstream (pandas) parity runner.

Speaks JSON Lines on stdio: one request object per line in, exactly one
response object per line out, in the same order. See ../../HARNESS.md.

Canonical result encoding (identical on the Go side):

  frame  {"kind":"frame","columns":[names],"dtypes":[canon],"index":[labels],
          "values":[[row], ...]}          -- row-major
  series {"kind":"series","name":str,"dtype":canon,"index":[labels],"values":[...]}
  mask   {"kind":"mask","values":[bool, ...]}
  list   {"kind":"list","values":[...]}
  scalar {"kind":"scalar","value":...}
  text   {"kind":"text","value":str}

Missing values are the string "__NA__"; non-finite floats are "__INF__" /
"__-INF__" / "__NAN__". dtype names are canonicalised to
float64/int64/string/bool/object so that pandas' nullable-extension spelling
("Int64") and the port's spelling ("int64") do not register as a difference,
while an int-vs-float difference still does.
"""

import json
import math
import sys

from io import StringIO

import pandas as pd

NA = "__NA__"

DTYPE_IN = {
    "int64": "Int64",
    "float64": "Float64",
    "string": "string",
    "bool": "boolean",
    "object": "object",
}

DTYPE_CANON = {
    "Int64": "int64", "int64": "int64", "int32": "int64", "int16": "int64",
    "int8": "int64", "uint64": "int64", "UInt64": "int64",
    "Float64": "float64", "float64": "float64", "float32": "float64",
    "string": "string", "str": "string",
    "boolean": "bool", "bool": "bool",
}


# --------------------------------------------------------------------------
# encoding
# --------------------------------------------------------------------------

def is_na(v):
    if v is None:
        return True
    try:
        r = pd.isna(v)
    except (TypeError, ValueError):
        return False
    if isinstance(r, bool):
        return r
    return False


def canon_dtype(series):
    s = str(series.dtype)
    if s in DTYPE_CANON:
        return DTYPE_CANON[s]
    if s == "object":
        vals = [v for v in series.tolist() if not is_na(v)]
        if vals and all(isinstance(v, str) for v in vals):
            return "string"
        if vals and all(isinstance(v, bool) for v in vals):
            return "bool"
        return "object"
    return s


def canon_scalar(v):
    if is_na(v):
        return NA
    if isinstance(v, bool):
        return bool(v)
    if hasattr(v, "item"):
        try:
            v = v.item()
        except (ValueError, AttributeError):
            pass
    if isinstance(v, bool):
        return bool(v)
    if isinstance(v, int):
        return int(v)
    if isinstance(v, float):
        if math.isnan(v):
            return "__NAN__"
        if math.isinf(v):
            return "__INF__" if v > 0 else "__-INF__"
        return float(v)
    if isinstance(v, str):
        return str(v)
    return str(v)


def canon_frame(df):
    cols = [str(c) for c in df.columns]
    dtypes = [canon_dtype(df[c]) for c in df.columns]
    index = [canon_scalar(i) for i in df.index]
    values = []
    for _, row in df.iterrows():
        values.append([canon_scalar(row[c]) for c in df.columns])
    return {"kind": "frame", "columns": cols, "dtypes": dtypes,
            "index": index, "values": values}


def canon_series(s):
    return {"kind": "series",
            "name": "" if s.name is None else str(s.name),
            "dtype": canon_dtype(s),
            "index": [canon_scalar(i) for i in s.index],
            "values": [canon_scalar(v) for v in s.tolist()]}


def mask(values):
    return {"kind": "mask", "values": [bool(v) for v in values]}


def lst(values):
    return {"kind": "list", "values": [canon_scalar(v) for v in values]}


def scalar(v):
    return {"kind": "scalar", "value": canon_scalar(v)}


def text(v):
    return {"kind": "text", "value": str(v)}


# --------------------------------------------------------------------------
# input construction
# --------------------------------------------------------------------------

def build_series(name, dtype, values):
    pdt = DTYPE_IN[dtype]
    if pdt == "Int64":
        vals = [None if v is None else int(v) for v in values]
    elif pdt == "Float64":
        vals = [None if v is None else float(v) for v in values]
    elif pdt == "string":
        vals = [None if v is None else str(v) for v in values]
    elif pdt == "boolean":
        vals = [None if v is None else bool(v) for v in values]
    else:
        vals = list(values)
    return pd.Series(vals, dtype=pdt, name=name)


def build_frame(table):
    data = {}
    order = []
    for c in table["columns"]:
        data[c["name"]] = build_series(c["name"], c["dtype"], c["values"])
        order.append(c["name"])
    if not order:
        return pd.DataFrame()
    return pd.DataFrame(data, columns=order)


def numeric_columns(df):
    return [c for c in df.columns if canon_dtype(df[c]) in ("int64", "float64")]


# --------------------------------------------------------------------------
# operations
# --------------------------------------------------------------------------

def op_series_new(a):
    return canon_series(build_series(a["name"], a["dtype"], a["values"]))


def op_series_infer(a):
    # "kinds" pins the intended Python/Go type of each element, because JSON
    # cannot distinguish 1 from 1.0 and the two runners must infer from the
    # same logical input.
    vals = list(a["values"])
    kinds = a.get("kinds")
    if kinds:
        cast = {"int": int, "float": float, "str": str, "bool": bool}
        vals = [None if v is None else cast[k](v) for v, k in zip(vals, kinds)]
    return canon_series(pd.Series(vals, name=a["name"]))


def op_series_astype(a):
    s = build_series(a["name"], a["dtype"], a["values"])
    return canon_series(s.astype(DTYPE_IN[a["to"]]))


def op_frame(a):
    return canon_frame(build_frame(a["table"]))


def op_frame_shape(a):
    r, c = build_frame(a["table"]).shape
    return lst([r, c])


def op_frame_names(a):
    return lst([str(c) for c in build_frame(a["table"]).columns])


def op_frame_index(a):
    return lst(list(build_frame(a["table"]).index))


def op_frame_dtypes(a):
    df = build_frame(a["table"])
    return lst([canon_dtype(df[c]) for c in df.columns])


def op_frame_from_map(a):
    # Mirrors pandas.FromMap: an explicit order slice decides column order and
    # any name missing from it is appended in sorted order.
    df = build_frame(a["table"])
    order = list(a["order"])
    rest = sorted(c for c in df.columns if c not in order)
    cols = [c for c in order if c in df.columns] + rest
    return canon_frame(df[cols])


def op_select(a):
    df = build_frame(a["table"])
    for c in a["columns"]:
        if c not in df.columns:
            raise KeyError(c)
    return canon_frame(df[list(a["columns"])])


def op_col(a):
    df = build_frame(a["table"])
    if a["column"] not in df.columns:
        raise KeyError(a["column"])
    return canon_series(df[a["column"]])


def op_iloc(a):
    return canon_frame(build_frame(a["table"]).iloc[a["start"]:a["end"]])


def op_loc(a):
    df = build_frame(a["table"])
    if a.get("set_index"):
        df = df.set_index(a["set_index"])
    labels = list(a["labels"])
    kind = a.get("label_kind", "int64")
    if kind in ("int64", "int"):
        labels = [int(v) for v in labels]
    return canon_frame(df.loc[labels])


def op_head(a):
    return canon_frame(build_frame(a["table"]).head(a["n"]))


def op_tail(a):
    return canon_frame(build_frame(a["table"]).tail(a["n"]))


def op_take(a):
    return canon_frame(build_frame(a["table"]).take(list(a["positions"])))


def op_reset_index(a):
    df = build_frame(a["table"])
    if a.get("sort_first"):
        df = df.sort_values(list(a["sort_first"]), kind="stable")
    out = df.reset_index()
    out = out.rename(columns={out.columns[0]: "index"})
    return canon_frame(out)


def op_set_index(a):
    df = build_frame(a["table"])
    if a["column"] not in df.columns:
        raise KeyError(a["column"])
    return canon_frame(df.set_index(a["column"]))


def op_filter_mask(a):
    df = build_frame(a["table"])
    m = list(a["mask"])
    if len(m) != len(df):
        raise ValueError("mask length %d != %d rows" % (len(m), len(df)))
    return canon_frame(df[pd.Series(m, index=df.index)])


CMP = {
    ">": lambda x, y: x > y,
    ">=": lambda x, y: x >= y,
    "<": lambda x, y: x < y,
    "<=": lambda x, y: x <= y,
    "==": lambda x, y: x == y,
    "!=": lambda x, y: x != y,
}


def op_filter_cmp(a):
    df = build_frame(a["table"])
    if a["column"] not in df.columns:
        raise KeyError(a["column"])
    col = df[a["column"]]
    m = CMP[a["op"]](col, a["value"])
    m = m.fillna(False).astype(bool)
    return canon_frame(df[m])


def op_with_column(a):
    df = build_frame(a["table"])
    vals = list(a["values"])
    if len(vals) != len(df):
        raise ValueError("column %r has length %d, want %d"
                         % (a["column"], len(vals), len(df)))
    s = build_series(a["column"], a["dtype"], vals)
    s.index = df.index
    out = df.copy()
    out[a["column"]] = s
    return canon_frame(out)


def op_drop(a):
    return canon_frame(build_frame(a["table"]).drop(columns=list(a["columns"])))


def op_rename(a):
    return canon_frame(build_frame(a["table"]).rename(columns=dict(a["mapping"])))


def op_sort_by(a):
    df = build_frame(a["table"])
    for k in a["keys"]:
        if k not in df.columns:
            raise KeyError(k)
    asc = a.get("ascending") or [True] * len(a["keys"])
    # na_position="last" on both keys and directions matches the port, which
    # always sorts missing values last regardless of direction.
    return canon_frame(df.sort_values(list(a["keys"]), ascending=list(asc),
                                      kind="stable", na_position="last"))


def op_drop_duplicates(a):
    return canon_frame(build_frame(a["table"]).drop_duplicates())


def op_concat(a):
    left = build_frame(a["table"])
    right = build_frame(a["right"])
    return canon_frame(pd.concat([left, right]))


def op_groupby_groups(a):
    df = build_frame(a["table"])
    for k in a["keys"]:
        if k not in df.columns:
            raise KeyError(k)
    return scalar(df.groupby(list(a["keys"]), sort=True, dropna=False).ngroups)


AGG_NAMES = {"sum": "sum", "mean": "mean", "min": "min", "max": "max",
             "count": "count", "std": "std"}


def op_groupby_agg(a):
    df = build_frame(a["table"])
    keys = list(a["keys"])
    for k in keys:
        if k not in df.columns:
            raise KeyError(k)
    spec = [(c, f) for c, f in a["spec"]]
    for c, _ in spec:
        if c not in df.columns:
            raise KeyError(c)
    gb = df.groupby(keys, sort=True, as_index=False, dropna=False,
                    observed=True)
    named = {}
    for c, f in spec:
        named["%s_%s" % (c, f)] = pd.NamedAgg(column=c, aggfunc=AGG_NAMES[f])
    out = gb.agg(**named)
    return canon_frame(out.reset_index(drop=True))


def op_groupby_plain(a):
    df = build_frame(a["table"])
    keys = list(a["keys"])
    gb = df.groupby(keys, sort=True, as_index=False, dropna=False)
    out = gb.agg({a["column"]: AGG_NAMES[a["agg"]]})
    return canon_frame(out.reset_index(drop=True))


def op_merge(a):
    left = build_frame(a["table"])
    right = build_frame(a["right"])
    how = a["how"]
    if how == "cross":
        return canon_frame(left.merge(right, how="cross"))
    on = a["on"]
    if on not in left.columns:
        raise KeyError("left frame has no column %r" % on)
    if on not in right.columns:
        raise KeyError("right frame has no column %r" % on)
    return canon_frame(left.merge(right, on=on, how=how,
                                  suffixes=("_left", "_right")))


def op_dropna(a):
    return canon_frame(build_frame(a["table"]).dropna())


def op_fillna(a):
    df = build_frame(a["table"])
    col = a.get("column") or ""
    out = df.copy()
    if col == "":
        for c in out.columns:
            out[c] = out[c].fillna(a["value"])
    else:
        if col not in out.columns:
            raise KeyError(col)
        out[col] = out[col].fillna(a["value"])
    return canon_frame(out)


def op_series_mask(a):
    df = build_frame(a["table"])
    s = df[a["column"]]
    if a["op"] == "isna":
        return mask(s.isna())
    if a["op"] == "notna":
        return mask(s.notna())
    raise ValueError("unknown mask op %r" % a["op"])


def op_series_mask_values(a):
    s = build_series(a["name"], a["dtype"], a["values"])
    if a["op"] == "isin":
        return mask(s.isin(list(a["arg"])).fillna(False))
    if a["op"] == "between":
        lo, hi = a["arg"]
        return mask(s.between(lo, hi).fillna(False))
    raise ValueError("unknown mask op %r" % a["op"])


def op_series_stat(a):
    s = build_series(a["name"], a["dtype"], a["values"])
    st = a["stat"]
    if st == "count":
        return scalar(int(s.count()))
    if st == "nunique":
        return scalar(int(s.nunique()))
    if st == "quantile":
        return scalar(s.quantile(a["q"]))
    if st not in ("sum", "mean", "min", "max", "std", "var", "median", "prod"):
        raise ValueError("unknown stat %r" % st)
    if canon_dtype(s) not in ("int64", "float64", "bool"):
        # pandas raises TypeError for mean/std/var/median of text; sum/min/max
        # of text do work, so only guard the ones that must fail.
        if st in ("mean", "std", "var", "median", "prod"):
            raise TypeError("cannot compute %s of dtype %s" % (st, s.dtype))
    return scalar(getattr(s, st)())


def op_series_op(a):
    s = build_series(a["name"], a["dtype"], a["values"])
    op = a["op"]
    arg = a.get("arg")
    if op == "dropna":
        return canon_series(s.dropna())
    if op == "fillna":
        return canon_series(s.fillna(arg))
    if op == "cumsum":
        return canon_series(s.cumsum())
    if op == "cummax":
        return canon_series(s.cummax())
    if op == "cummin":
        return canon_series(s.cummin())
    if op == "cumprod":
        return canon_series(s.cumprod())
    if op == "diff":
        return canon_series(s.diff())
    if op == "pct_change":
        return canon_series(s.pct_change())
    if op == "shift":
        return canon_series(s.shift(int(arg)))
    if op == "rank":
        return canon_series(s.rank(method=arg or "average"))
    if op == "round":
        return canon_series(s.round(int(arg)))
    if op == "abs":
        return canon_series(s.abs())
    if op == "clip":
        return canon_series(s.clip(arg[0], arg[1]))
    if op == "sort":
        return canon_series(s.sort_values(ascending=bool(arg),
                                          kind="stable", na_position="last"))
    if op == "unique":
        return lst(list(s.dropna().unique()))
    if op == "value_counts":
        vc = s.value_counts(dropna=True)
        vc = vc.sort_index(kind="stable").sort_values(ascending=False,
                                                      kind="stable")
        vc.name = "count"
        return canon_series(vc)
    if op == "mode":
        return lst(sorted(s.mode().tolist()))
    if op == "nlargest":
        return canon_series(s.dropna().nlargest(int(arg), keep="first"))
    if op == "nsmallest":
        return canon_series(s.dropna().nsmallest(int(arg), keep="first"))
    if op == "head":
        return canon_series(s.head(int(arg)))
    if op == "tail":
        return canon_series(s.tail(int(arg)))
    raise ValueError("unknown series op %r" % op)


ARITH = {"add": "add", "sub": "sub", "mul": "mul", "div": "truediv"}


def op_series_arith(a):
    dt = a["dtype"]
    left = build_series("l", dt, a["left"])
    right = build_series("r", dt, a["right"])
    out = getattr(left, ARITH[a["op"]])(right)
    out.name = ""
    return canon_series(out)


def op_series_pair_stat(a):
    dt = a["dtype"]
    left = build_series("l", dt, a["left"])
    right = build_series("r", dt, a["right"])
    if a["op"] == "corr":
        return scalar(left.corr(right))
    if a["op"] == "cov":
        return scalar(left.cov(right))
    raise ValueError("unknown pair stat %r" % a["op"])


def op_describe(a):
    return canon_frame(build_frame(a["table"]).describe())


FRAME_STATS = ("sum", "mean", "min", "max", "std", "var", "median", "nunique")


def op_frame_stat(a):
    df = build_frame(a["table"])
    st = a["stat"]
    if st not in FRAME_STATS:
        raise ValueError("unknown frame stat %r" % st)
    if st == "nunique":
        out = df.nunique()
        out.name = ""
        return canon_series(out)
    num = numeric_columns(df)
    if len(num) != len(df.columns):
        # pandas 2.x raises for a reduction over mixed dtypes unless
        # numeric_only is passed; the port silently drops non-numeric columns.
        raise TypeError("%s over non-numeric columns" % st)
    out = getattr(df[num], st)()
    out.name = ""
    return canon_series(out)


def op_frame_stat_name(a):
    """The name pandas gives a column-wise reduction result (None -> "")."""
    df = build_frame(a["table"])
    out = getattr(df[numeric_columns(df)], a["stat"])()
    return scalar("" if out.name is None else str(out.name))


def op_frame_corr(a):
    df = build_frame(a["table"])
    out = df[numeric_columns(df)].corr()
    return canon_frame(out)


def op_frame_op(a):
    df = build_frame(a["table"])
    op = a["op"]
    if op == "abs":
        out = df.copy()
        for c in numeric_columns(df):
            out[c] = out[c].abs()
        return canon_frame(out)
    if op == "round":
        out = df.copy()
        for c in numeric_columns(df):
            out[c] = out[c].round(int(a["arg"]))
        return canon_frame(out)
    if op == "transpose":
        out = df.T
        out.columns = [str(c) for c in out.columns]
        return canon_frame(out)
    raise ValueError("unknown frame op %r" % op)


def op_str_op(a):
    s = build_series(a["name"], a["dtype"], a["values"])
    op = a["op"]
    arg = a.get("arg")
    acc = s.str
    if op == "upper":
        return canon_series(acc.upper())
    if op == "lower":
        return canon_series(acc.lower())
    if op == "title":
        return canon_series(acc.title())
    if op == "strip":
        return canon_series(acc.strip())
    if op == "len":
        return canon_series(acc.len().astype("Int64"))
    if op == "replace":
        return canon_series(acc.replace(arg[0], arg[1], regex=False))
    if op == "contains":
        return mask(acc.contains(arg, regex=False).fillna(False))
    if op == "startswith":
        return mask(acc.startswith(arg).fillna(False))
    if op == "endswith":
        return mask(acc.endswith(arg).fillna(False))
    raise ValueError("unknown str op %r" % op)


def frame_to_csv(df):
    buf = StringIO()
    df.to_csv(buf, index=False, lineterminator="\n")
    return buf.getvalue()


def op_to_csv(a):
    return text(frame_to_csv(build_frame(a["table"])))


def read_csv_text(csv, delimiter=",", na_values=None, no_header=False):
    kw = {"sep": delimiter, "keep_default_na": False,
          "na_values": [""] + list(na_values or [])}
    if no_header:
        kw["header"] = None
    df = pd.read_csv(StringIO(csv), **kw)
    df.columns = [str(c) for c in df.columns]
    return df


def op_csv_roundtrip(a):
    df = build_frame(a["table"])
    return canon_frame(read_csv_text(frame_to_csv(df)))


def op_read_csv(a):
    return canon_frame(read_csv_text(a["csv"],
                                     delimiter=a.get("delimiter", ","),
                                     na_values=a.get("na_values"),
                                     no_header=bool(a.get("no_header"))))


OPS = {
    "series_new": op_series_new,
    "series_infer": op_series_infer,
    "series_astype": op_series_astype,
    "frame": op_frame,
    "frame_shape": op_frame_shape,
    "frame_names": op_frame_names,
    "frame_index": op_frame_index,
    "frame_dtypes": op_frame_dtypes,
    "frame_from_map": op_frame_from_map,
    "select": op_select,
    "col": op_col,
    "iloc": op_iloc,
    "loc": op_loc,
    "head": op_head,
    "tail": op_tail,
    "take": op_take,
    "reset_index": op_reset_index,
    "set_index": op_set_index,
    "filter_mask": op_filter_mask,
    "filter_cmp": op_filter_cmp,
    "with_column": op_with_column,
    "drop": op_drop,
    "rename": op_rename,
    "sort_by": op_sort_by,
    "drop_duplicates": op_drop_duplicates,
    "concat": op_concat,
    "groupby_groups": op_groupby_groups,
    "groupby_agg": op_groupby_agg,
    "groupby_plain": op_groupby_plain,
    "merge": op_merge,
    "dropna": op_dropna,
    "fillna": op_fillna,
    "series_mask": op_series_mask,
    "series_mask_values": op_series_mask_values,
    "series_stat": op_series_stat,
    "series_op": op_series_op,
    "series_arith": op_series_arith,
    "series_pair_stat": op_series_pair_stat,
    "describe": op_describe,
    "frame_stat": op_frame_stat,
    "frame_stat_name": op_frame_stat_name,
    "frame_corr": op_frame_corr,
    "frame_op": op_frame_op,
    "str_op": op_str_op,
    "to_csv": op_to_csv,
    "csv_roundtrip": op_csv_roundtrip,
    "read_csv": op_read_csv,
}


def main():
    if len(sys.argv) > 1 and sys.argv[1] == "--version":
        sys.stdout.write(pd.__version__ + "\n")
        return
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:  # malformed request: still answer
            sys.stdout.write(json.dumps({"id": "", "ok": False,
                                         "error": "bad request: %s" % exc}) + "\n")
            sys.stdout.flush()
            continue
        cid = req.get("id", "")
        try:
            fn = OPS.get(req.get("fn"))
            if fn is None:
                raise ValueError("unknown fn %r" % req.get("fn"))
            out = {"id": cid, "ok": True, "value": fn(req.get("args") or {})}
        except Exception as exc:
            print("case %s: %s: %s" % (cid, type(exc).__name__, exc),
                  file=sys.stderr)
            out = {"id": cid, "ok": False,
                   "error": "%s: %s" % (type(exc).__name__, exc)}
        sys.stdout.write(json.dumps(out, sort_keys=True, allow_nan=False) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
