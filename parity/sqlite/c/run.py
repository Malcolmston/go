#!/usr/bin/env python3
"""Upstream parity runner for the real SQLite (the `sqlite3` CLI, ecosystem C).

Speaks the harness JSON-Lines contract on stdio: one request object per line in,
one reply object per line out, same order, never exiting on a failing case.

    in   {"id":"...","fn":"script","args":[{"setup":[...],"query":"...","params":[...]}]}
    out  {"id":"...","ok":true,"value":{"columns":[...],"rows":[[[tag,payload],...]]}}
    out  {"id":"...","ok":false,"error":"..."}

How the oracle is invoked
-------------------------
Each case gets its own **fresh** private in-memory database, so the CLI is
spawned once per case:

    sqlite3 -batch -init /dev/null :memory:

  * `-init /dev/null` stops a user's ~/.sqliterc from changing the output.
  * `-batch` forces non-interactive mode.

The generated script always begins with

    .bail on          -- stop and exit non-zero at the first error
    .mode quote       -- render every value as a SQL literal
    .headers on       -- emit the column-name row
    .separator "<TAB>"

`.mode quote` is the whole trick: it is the only CLI mode that does **not**
coerce types on the way out. NULL prints as `NULL`, INTEGER as `1`, REAL as
`1.0` (always with a `.` or an exponent), TEXT as `'a''b'`, BLOB as `X'6162'`.
That makes the port's known REAL-vs-INTEGER rendering difference visible instead
of being flattened into a string. Values are parsed back out of those literals
into an explicit `[tag, payload]` pair, tag in
{null, int, real, text, blob}.

`.print @@BEGIN` / `.print @@END` fence the final SELECT's output, so anything a
setup statement happens to print (PRAGMA, EXPLAIN, …) is ignored, and a missing
`@@END` is a reliable signal that `.bail on` aborted the script.

`?` placeholders cannot be bound positionally by the CLI, so the k-th bare `?`
of a parameterised statement is rewritten to `?k` and bound with
`.parameter set ?k <literal>`. The rewrite is string-literal aware.

Caveat, applied identically by the Go runner: a zero-row result set emits no
header line in any CLI output mode, so `columns` is reported as null when there
are no rows.
"""

import json
import os
import re
import subprocess
import sys

SQLITE3 = os.environ.get("SQLITE3_BIN", "sqlite3")
SEP = "\t"
BEGIN = "@@BEGIN"
END = "@@END"
TIMEOUT = 20.0

INT_RE = re.compile(r"^-?\d+$")


# --------------------------------------------------------------- SQL emission

def sql_literal(v):
    """Render a JSON case value as a SQL literal for `.parameter set`."""
    if v is None:
        return "NULL"
    if v is True:
        return "1"
    if v is False:
        return "0"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, float):
        return repr(v)
    if isinstance(v, str):
        return "'" + v.replace("'", "''") + "'"
    if isinstance(v, dict) and "blob" in v:
        return "X'" + v["blob"] + "'"
    raise ValueError("unsupported parameter %r" % (v,))


def number_params(sql):
    """Rewrite the k-th bare `?` outside string literals to `?k`.

    Returns (rewritten_sql, count).
    """
    out = []
    n = 0
    i = 0
    while i < len(sql):
        c = sql[i]
        if c == "'":
            out.append(c)
            i += 1
            while i < len(sql):
                out.append(sql[i])
                if sql[i] == "'":
                    if i + 1 < len(sql) and sql[i + 1] == "'":
                        out.append("'")
                        i += 2
                        continue
                    i += 1
                    break
                i += 1
            continue
        if c == "?":
            n += 1
            out.append("?%d" % n)
            i += 1
            continue
        out.append(c)
        i += 1
    return "".join(out), n


def build_script(spec):
    lines = [
        ".bail on",
        ".mode quote",
        ".headers on",
        '.separator "\\t"',
    ]

    def emit(sql, params):
        sql = sql.strip().rstrip(";")
        if params:
            sql, n = number_params(sql)
            if n != len(params):
                raise ValueError(
                    "statement has %d placeholders but %d params" % (n, len(params))
                )
            for k, p in enumerate(params, 1):
                lines.append(".parameter set ?%d %s" % (k, sql_literal(p)))
        # The terminator goes on its own line so a trailing `--` comment cannot
        # swallow it (and cannot swallow the following dot-command either).
        lines.append(sql)
        lines.append(";")
        if params:
            lines.append(".parameter clear")

    for step in spec.get("setup", []):
        if isinstance(step, str):
            emit(step, None)
        elif "txn" in step:
            # The Go runner drives these through database/sql's transaction API;
            # upstream has no such split, so plain SQL is the equivalent.
            lines.append({"begin": "BEGIN;", "commit": "COMMIT;",
                          "rollback": "ROLLBACK;"}[step["txn"]])
        else:
            emit(step["sql"], step.get("params"))

    lines.append(".print " + BEGIN)
    emit(spec["query"], spec.get("params"))
    lines.append(".print " + END)
    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------- value parsing

def split_literals(payload):
    """Split the quote-mode payload into rows of raw SQL literals.

    Rows are separated by newlines and fields by TAB, but only outside single
    quotes -- a TEXT literal may legitimately contain either.
    """
    rows = []
    row = []
    cur = []
    i = 0
    n = len(payload)
    while i < n:
        c = payload[i]
        if c == "'":
            cur.append(c)
            i += 1
            while i < n:
                cur.append(payload[i])
                if payload[i] == "'":
                    if i + 1 < n and payload[i + 1] == "'":
                        cur.append("'")
                        i += 2
                        continue
                    i += 1
                    break
                i += 1
            continue
        if c == SEP:
            row.append("".join(cur))
            cur = []
            i += 1
            continue
        if c == "\n":
            row.append("".join(cur))
            cur = []
            rows.append(row)
            row = []
            i += 1
            continue
        cur.append(c)
        i += 1
    tail = "".join(cur)
    if tail or row:
        row.append(tail)
        rows.append(row)
    return [r for r in rows if r != [""]]


def parse_literal(lit):
    """Turn one quote-mode SQL literal into [tag, payload]."""
    s = lit.strip()
    if s == "NULL":
        return ["null", None]
    if len(s) >= 2 and s[0] == "'" and s[-1] == "'":
        return ["text", s[1:-1].replace("''", "'")]
    if len(s) >= 3 and s[0] in "Xx" and s[1] == "'" and s[-1] == "'":
        return ["blob", s[2:-1].lower()]
    if s in ("Inf", "inf"):
        return ["real", "inf"]
    if s in ("-Inf", "-inf"):
        return ["real", "-inf"]
    if s in ("NaN", "nan", "-NaN"):
        return ["real", "nan"]
    if INT_RE.match(s):
        return ["int", s]
    # Anything left is a REAL: quote mode always gives it a '.' or exponent.
    return ["real", float(s)]


def parse_header(lit):
    s = lit.strip()
    if len(s) >= 2 and s[0] == "'" and s[-1] == "'":
        return s[1:-1].replace("''", "'")
    return s


# --------------------------------------------------------------------- driving

def run_case(spec):
    script = build_script(spec)
    proc = subprocess.run(
        [SQLITE3, "-batch", "-init", os.devnull, ":memory:"],
        input=script,
        capture_output=True,
        text=True,
        timeout=TIMEOUT,
    )
    out = proc.stdout

    marker = BEGIN + "\n"
    idx = out.find(marker)
    if idx < 0:
        err = (proc.stderr or out).strip() or "sqlite3 exited %d" % proc.returncode
        raise RuntimeError(err.splitlines()[0] if err else "sqlite3 failed")
    body = out[idx + len(marker):]
    endidx = body.find(END + "\n")
    if endidx < 0 and body.rstrip("\n").endswith(END):
        endidx = body.rstrip("\n").rfind(END)
    if endidx < 0:
        err = (proc.stderr or "").strip() or "sqlite3 exited %d" % proc.returncode
        raise RuntimeError(err.splitlines()[0] if err else "sqlite3 failed")
    body = body[:endidx]

    rows = split_literals(body)
    if not rows:
        return {"columns": None, "rows": []}
    columns = [parse_header(x) for x in rows[0]]
    values = [[parse_literal(x) for x in r] for r in rows[1:]]
    return {"columns": columns, "rows": values}


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as e:  # unparseable request: no id to answer with
            print(json.dumps({"id": "", "ok": False, "error": "bad request: %s" % e}),
                  flush=True)
            continue
        rid = req.get("id", "")
        try:
            if req.get("fn") != "script":
                raise ValueError("unknown fn %r" % req.get("fn"))
            spec = req["args"][0]
            value = run_case(spec)
            print(json.dumps({"id": rid, "ok": True, "value": value}), flush=True)
        except Exception as e:
            print(json.dumps({"id": rid, "ok": False,
                              "error": "%s: %s" % (type(e).__name__, e)}), flush=True)


if __name__ == "__main__":
    main()
