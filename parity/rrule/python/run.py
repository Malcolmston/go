#!/usr/bin/env python3
"""Upstream (dateutil.rrule) parity runner.

Speaks JSON Lines on stdio: one request object per line in, exactly one
response object per line out, in the same order.  Every case is evaluated
inside a try/except so a raising case becomes {"ok": false, "error": "..."}
and the runner keeps reading -- a crashed runner would score every remaining
case as a loss.

No case may depend on "now": every request carries an explicit DTSTART, and
occurrences are emitted as fixed-format ISO-8601 strings so the harness can
compare them exactly.

Nothing but response lines goes to stdout; diagnostics go to stderr.
"""

import datetime
import json
import sys
import itertools

import dateutil
import dateutil.rrule as rr
from dateutil.tz import gettz

try:
    from zoneinfo import ZoneInfo
except ImportError:  # pragma: no cover - Python < 3.9
    ZoneInfo = None


# --------------------------------------------------------------- formatting

def fmt(dt):
    """Render one occurrence as a fixed-format ISO-8601 string.

    Aware instants carry a numeric +HH:MM offset (never "Z", so the Go side's
    -07:00 layout produces byte-identical output); naive instants carry none.
    """
    base = dt.strftime("%Y-%m-%dT%H:%M:%S")
    if dt.tzinfo is None:
        return base
    off = dt.utcoffset()
    total = int(off.total_seconds())
    sign = "+" if total >= 0 else "-"
    total = abs(total)
    return "%s%s%02d:%02d" % (base, sign, total // 3600, (total % 3600) // 60)


def zone(name):
    if name is None:
        return None
    # gettz is what dateutil's own rrulestr uses for DTSTART;TZID=, so using
    # it here keeps the two paths consistent.
    tz = gettz(name)
    if tz is None:
        raise ValueError("unknown timezone: %s" % name)
    return tz


def parse_naive(s):
    """Parse "YYYY-MM-DDTHH:MM:SS" strictly -- no locale, no "now" fallback."""
    return datetime.datetime.strptime(s, "%Y-%m-%dT%H:%M:%S")


def instant(s, tzname):
    dt = parse_naive(s)
    tz = zone(tzname)
    if tz is None:
        return dt
    return dt.replace(tzinfo=tz)


# ------------------------------------------------------------------ helpers

def build(spec):
    """Compile spec["rule"] (a bare RRULE value) anchored at spec["dtstart"]."""
    dt = instant(spec["dtstart"], spec.get("tz"))
    return rr.rrulestr(spec["rule"], dtstart=dt)


def take(it, limit):
    return [fmt(d) for d in itertools.islice(iter(it), limit)]


def rule_parts(text):
    """Normalise the RRULE line of a serialised rule for text comparison.

    Only the rule-part set is compared: upstream's __str__ drops the TZID from
    its DTSTART line while the port keeps it, and that difference is recorded
    in COVERAGE.md rather than spread across every case.
    """
    value = None
    for line in text.replace("\r\n", "\n").split("\n"):
        line = line.strip()
        if line.upper().startswith("RRULE:"):
            value = line.split(":", 1)[1]
    if value is None:
        raise ValueError("no RRULE line in serialised rule: %r" % text)
    parts = [p.strip().upper() for p in value.split(";") if p.strip()]
    return ";".join(sorted(parts))


# --------------------------------------------------------------------- ops

def op_expand(spec):
    return take(build(spec), spec["limit"])


def op_count(spec):
    return build(spec).count()


def op_between(spec):
    r = build(spec)
    after = instant(spec["after"], spec.get("tz"))
    before = instant(spec["before"], spec.get("tz"))
    out = r.between(after, before, inc=spec.get("inc", False))
    return [fmt(d) for d in out]


def op_after(spec):
    r = build(spec)
    d = r.after(instant(spec["after"], spec.get("tz")), inc=spec.get("inc", False))
    return None if d is None else fmt(d)


def op_before(spec):
    r = build(spec)
    d = r.before(instant(spec["before"], spec.get("tz")), inc=spec.get("inc", False))
    return None if d is None else fmt(d)


def op_rulestring(spec):
    return rule_parts(str(build(spec)))


def op_roundtrip(spec):
    """Serialise, re-parse the RRULE line, re-anchor, expand.

    The DTSTART line is deliberately dropped before re-parsing: upstream's
    serialisation is zone-blind, so re-anchoring both sides at the original
    DTSTART isolates what is being measured -- whether the rule parts survive
    a text round trip.
    """
    first = build(spec)
    value = None
    for line in str(first).split("\n"):
        if line.upper().startswith("RRULE:"):
            value = line.split(":", 1)[1]
    if value is None:
        raise ValueError("no RRULE line in serialised rule")
    again = rr.rrulestr(value, dtstart=instant(spec["dtstart"], spec.get("tz")))
    return take(again, spec["limit"])


def op_set(spec):
    s = rr.rrulestr(spec["text"], forceset=True)
    return take(s, spec["limit"])


def op_setbuild(spec):
    """Build an rruleset programmatically from rules/rdates/exdates/exrules."""
    s = rr.rruleset()
    tzname = spec.get("tz")
    dt = instant(spec["dtstart"], tzname)
    for value in spec.get("rrules", []):
        s.rrule(rr.rrulestr(value, dtstart=dt))
    for value in spec.get("exrules", []):
        s.exrule(rr.rrulestr(value, dtstart=dt))
    for d in spec.get("rdates", []):
        s.rdate(instant(d, tzname))
    for d in spec.get("exdates", []):
        s.exdate(instant(d, tzname))
    return take(s, spec["limit"])


OPS = {
    "expand": op_expand,
    "count": op_count,
    "between": op_between,
    "after": op_after,
    "before": op_before,
    "rulestring": op_rulestring,
    "roundtrip": op_roundtrip,
    "set": op_set,
    "setbuild": op_setbuild,
}


def handle(req):
    fn = req.get("fn")
    op = OPS.get(fn)
    if op is None:
        raise ValueError("unknown fn: %r" % fn)
    args = req.get("args") or []
    if len(args) != 1 or not isinstance(args[0], dict):
        raise ValueError("fn %s takes exactly one object argument" % fn)
    return op(args[0])


def main():
    if "--version" in sys.argv[1:]:
        sys.stdout.write(dateutil.__version__ + "\n")
        return
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:  # unparseable request: still answer something
            sys.stdout.write(json.dumps({"id": "", "ok": False,
                                         "error": "bad request: %s" % exc}) + "\n")
            sys.stdout.flush()
            continue
        cid = req.get("id", "")
        try:
            value = handle(req)
            out = {"id": cid, "ok": True, "value": value}
        except BaseException as exc:  # noqa: BLE001 - never die on a case
            out = {"id": cid, "ok": False,
                   "error": "%s: %s" % (type(exc).__name__, exc)}
        sys.stdout.write(json.dumps(out, sort_keys=True) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
