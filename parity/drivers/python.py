# Parity driver for CPython: imports the REAL PyPI distribution installed
# alongside it and answers parity cases over newline-delimited JSON.
#
# Each request is {id, src, input}. `src` is compiled as the body of a function
# receiving the case input; whatever it returns is the upstream answer, and
# whatever it raises is upstream's error behavior.
import base64
import datetime
import decimal
import inspect
import json
import math
import sys
import textwrap


def encode(v, seen=None):
    """Map Python values onto the canonical JSON the Go side also encodes to."""
    if v is None or isinstance(v, (bool, str)):
        return v
    if isinstance(v, int):
        return v
    if isinstance(v, float):
        if math.isnan(v):
            return "NaN"
        if math.isinf(v):
            return "Infinity" if v > 0 else "-Infinity"
        return v
    if isinstance(v, decimal.Decimal):
        return encode(float(v))
    if isinstance(v, (bytes, bytearray, memoryview)):
        return base64.b64encode(bytes(v)).decode()  # matches Go's []byte JSON
    if isinstance(v, (datetime.datetime, datetime.date, datetime.time)):
        return v.isoformat()
    if isinstance(v, complex):
        return {"real": encode(v.real), "imag": encode(v.imag)}

    seen = seen or set()
    if id(v) in seen:
        return {"$cycle": True}
    seen = seen | {id(v)}

    # numpy / pandas and friends: prefer their own lossless conversions.
    if hasattr(v, "tolist") and callable(v.tolist):
        return encode(v.tolist(), seen)
    if hasattr(v, "item") and callable(v.item) and getattr(v, "shape", None) == ():
        return encode(v.item(), seen)
    if hasattr(v, "to_dict") and callable(v.to_dict):
        return encode(v.to_dict(), seen)

    if isinstance(v, dict):
        return {str(k): encode(x, seen) for k, x in v.items()}
    if isinstance(v, (list, tuple, set, frozenset)):
        items = sorted(v, key=repr) if isinstance(v, (set, frozenset)) else v
        return [encode(x, seen) for x in items]
    if callable(v):
        return {"$fn": getattr(v, "__name__", "anonymous")}
    if hasattr(v, "__dict__"):
        return {k: encode(x, seen) for k, x in vars(v).items() if not k.startswith("_")}
    return str(v)


def run(src, value):
    scope = {}
    exec("def _parity_case(input):\n" + textwrap.indent(src, "    "), scope)
    out = scope["_parity_case"](value)
    if inspect.isawaitable(out):
        import asyncio

        out = asyncio.run(out)
    return out


def main():
    for line in sys.stdin:
        if not line.strip():
            continue
        try:
            req = json.loads(line)
        except Exception as err:  # noqa: BLE001 - report, never die
            reply = {"id": "", "ok": False, "error": f"bad request: {err}"}
        else:
            try:
                reply = {"id": req["id"], "ok": True, "value": encode(run(req["src"], req.get("input")))}
            except Exception as err:  # noqa: BLE001 - upstream raising is data
                reply = {"id": req["id"], "ok": False, "error": f"{type(err).__name__}: {err}"}
        sys.stdout.write(json.dumps(reply) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
