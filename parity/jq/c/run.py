#!/usr/bin/env python3
"""Upstream oracle runner for the jq parity harness.

Speaks the JSON-Lines contract from ../../HARNESS.md. jq is a C program with no
embeddable public API that is convenient to link against from a test, so this
long-lived wrapper process shells out to the real `jq` binary once per case:

    in   {"id": "...", "fn": "run", "args": [PROGRAM, INPUT, VARS?]}
    out  {"id": "...", "ok": true,  "value": [ ...the whole output stream... ]}
    out  {"id": "...", "ok": false, "error": "..."}

A jq filter yields a *stream* of zero, one or many values, so `value` is always
the full stream as a JSON array: a port that collapses a multi-value stream to a
single value must show up as a mismatch.

The jq binary is invoked with a fixed, minimal environment (argv[1] is its
absolute path, so PATH is not needed) so that `env` and time-zone dependent
builtins are comparable with the Go side.
"""

import json
import os
import subprocess
import sys

JQ = sys.argv[1] if len(sys.argv) > 1 else "jq"

# Fixed environment view: the Go runner installs exactly these, so `env` and
# `$ENV` compare. Nothing else is inherited.
FIXED_ENV = {
    "TZ": "UTC",
    "LANG": "C",
    "LC_ALL": "C",
    "PARITY": "jq",
}

CASE_TIMEOUT = 20


def handle(req):
    args = req.get("args") or []
    if not args:
        raise ValueError("case has no program argument")
    program = args[0]
    if not isinstance(program, str):
        raise ValueError("program argument must be a string")
    inp = args[1] if len(args) > 1 else None
    variables = args[2] if len(args) > 2 else None

    cmd = [JQ, "-c"]
    if variables:
        if not isinstance(variables, dict):
            raise ValueError("variables argument must be an object")
        for k in sorted(variables):
            cmd += ["--argjson", k, json.dumps(variables[k])]
    cmd += ["--", program]

    p = subprocess.run(
        cmd,
        input=json.dumps(inp),
        capture_output=True,
        text=True,
        env=FIXED_ENV,
        timeout=CASE_TIMEOUT,
    )
    if p.returncode != 0:
        msg = p.stderr.strip() or ("jq exited with status %d" % p.returncode)
        return {"ok": False, "error": msg}

    values = []
    for line in p.stdout.splitlines():
        if line.strip() == "":
            continue
        values.append(json.loads(line))
    return {"ok": True, "value": values}


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        rid = None
        try:
            req = json.loads(line)
            rid = req.get("id")
            out = handle(req)
        except Exception as exc:  # never exit on a failing case
            out = {"ok": False, "error": "%s: %s" % (type(exc).__name__, exc)}
        out["id"] = rid
        sys.stdout.write(json.dumps({"id": rid, **out}) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    # Keep our own environment quiet too; only the child's env matters, but this
    # guards against a stray TZ leaking into python's own json/number handling.
    os.environ.setdefault("TZ", "UTC")
    main()
