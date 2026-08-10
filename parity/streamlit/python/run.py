#!/usr/bin/env python3
"""Upstream (Python/streamlit) half of the streamlit parity harness.

Reads one JSON request per line on stdin, writes exactly one JSON reply per
line on stdout, and never exits on a failing case.

Request::

    {"id": "...", "fn": "<app name>", "args": [[<step>, ...], {"profile": "core"}]}

Reply::

    {"id": "...", "ok": true, "value": {"snapshots": [<snapshot>, ...]}}
    {"id": "...", "ok": false, "error": "..."}

The oracle is ``streamlit.testing.v1.AppTest``, which executes the script
headlessly and exposes the resulting element tree — the same tree the
``ForwardMsg``/``Delta`` stream would carry to a browser. No browser and no
``streamlit run`` is involved.
"""

from __future__ import annotations

import datetime
import json
import sys
import traceback

import streamlit as st
from streamlit.testing.v1 import AppTest

from apps import APPS

# ------------------------------------------------------------- compared properties
#
# Every case compares the core properties both implementations model: identity,
# label, current value, options and numeric bounds. A case may opt in to extra
# properties with ``{"extra": [...]}``; that keeps a property the port does not
# model at all (``help``, ``disabled``, a button's clicked state, a number
# input's step) from being scored on every case that merely happens to contain
# such a widget, and gives it exactly one case of its own instead.

CORE_WIDGET_PROPS = ("label", "value", "options", "min", "max", "step", "index")
EXTRA_PROPS = ("format", "help", "disabled", "placeholder", "maxChars")

# Element .type values Streamlit uses for the four alert kinds.
ALERT_KINDS = {"success", "info", "warning", "error"}

PASSTHROUGH_TEXT = {
    "title",
    "header",
    "subheader",
    "markdown",
    "caption",
    "text",
}

WIDGET_TYPES = {
    "button",
    "checkbox",
    "toggle",
    "radio",
    "selectbox",
    "multiselect",
    "slider",
    "text_input",
    "text_area",
    "number_input",
    "date_input",
    "time_input",
    "file_uploader",
    "color_picker",
    "select_slider",
}


def jsonable(v):
    """Coerce a Python value into something JSON-comparable and deterministic."""
    if v is None or isinstance(v, (bool, int, float, str)):
        return v
    if isinstance(v, datetime.datetime):
        return v.isoformat()
    if isinstance(v, datetime.date):
        return v.isoformat()
    if isinstance(v, datetime.time):
        return v.strftime("%H:%M")
    if isinstance(v, (list, tuple, set)):
        return [jsonable(x) for x in v]
    if isinstance(v, dict):
        return {str(k): jsonable(v[k]) for k in sorted(v, key=str)}
    return str(v)


def canon_json(text: str):
    """Re-emit a JSON document with sorted keys so both sides agree on layout."""
    try:
        return json.dumps(json.loads(text), sort_keys=True, separators=(",", ":"))
    except Exception:
        return text


def field(proto, name, default=None):
    try:
        return getattr(proto, name)
    except Exception:
        return default


def enum_name(proto, name):
    """Return a protobuf enum value as a lowercase name, or None."""
    try:
        desc = proto.DESCRIPTOR.fields_by_name[name]
        return desc.enum_type.values_by_number[getattr(proto, name)].name.lower()
    except Exception:
        return None


def df_props(el):
    """Normalise a Dataframe/Table element to header + stringified rows."""
    try:
        df = el.value
        header = [str(c) for c in df.columns]
        rows = [[str(c) for c in row] for row in df.values.tolist()]
        return {"header": header, "rows": rows}
    except Exception as exc:  # pragma: no cover - defensive
        return {"header": None, "rows": None, "error": str(exc)}


def chart_props(el):
    """Describe a vega-lite chart by its mark kind and the data it encodes."""
    out = {"kind": None, "encoding": "vega-lite", "series": None}
    try:
        spec = json.loads(el.proto.spec)
    except Exception:
        return out
    mark = spec.get("mark")
    if mark is None and isinstance(spec.get("layer"), list) and spec["layer"]:
        mark = spec["layer"][0].get("mark")
    if isinstance(mark, dict):
        mark = mark.get("type")
    out["kind"] = {"circle": "scatter", "point": "scatter"}.get(mark, mark)
    # The values live in an arrow-encoded dataset; decode it if pyarrow is able.
    try:
        import io

        import pyarrow as pa

        blobs = [d.data.data for d in el.proto.datasets] or [el.proto.data.data]
        series = []
        for blob in blobs:
            if not blob:
                continue
            table = pa.ipc.open_stream(io.BytesIO(blob)).read_all().to_pydict()
            for name in sorted(table):
                if name.endswith("-- streamlit-generated"):
                    continue
                series.append(
                    [float(x) for x in table[name] if isinstance(x, (int, float))]
                )
        out["series"] = series or None
    except Exception:
        pass
    return out


def widget_key(el, positional: dict) -> str:
    """Widget identity, normalised.

    An explicitly-keyed widget reports its key. An unkeyed widget reports
    ``#<n>``, its zero-based position among same-typed widgets in document
    order — Streamlit's real ids are content hashes and the Go port's are
    ``auto-<type>-<n>``, so neither is comparable verbatim.
    """
    if getattr(el, "key", None):
        return el.key
    n = positional.get(el.type, 0)
    positional[el.type] = n + 1
    return "#%d" % n


def widget_value(el):
    try:
        return jsonable(el.value)
    except Exception:
        return None


def canon_element(el, extra: set, positional: dict):
    """Map one Streamlit element to the harness's shared vocabulary."""
    t = el.type
    p = getattr(el, "proto", None)

    if t in ALERT_KINDS:
        return {"type": "alert", "props": {"kind": t, "text": el.value}}
    if t == "divider":
        return {"type": "divider", "props": {}}
    if t == "latex":
        text = el.value or ""
        if text.startswith("$$\n") and text.endswith("\n$$"):
            text = text[3:-3]
        return {"type": "latex", "props": {"text": text}}
    if t in PASSTHROUGH_TEXT:
        return {"type": t, "props": {"text": el.value}}
    if t == "code":
        return {
            "type": "code",
            "props": {"text": field(p, "code_text", ""), "language": field(p, "language", "")},
        }
    if t == "json":
        return {"type": "json", "props": {"body": canon_json(el.value or "")}}
    if t == "metric":
        return {
            "type": "metric",
            "props": {
                "label": field(p, "label", ""),
                "value": field(p, "body", ""),
                "delta": field(p, "delta", ""),
                "direction": (enum_name(p, "direction") or "none"),
                "color": {"gray": "grey"}.get(
                    enum_name(p, "color") or "", enum_name(p, "color")
                ),
            },
        }
    if t == "dataframe":
        return {"type": "dataframe", "props": df_props(el)}
    if t == "table":
        return {"type": "table", "props": df_props(el)}
    if t == "vega_lite_chart":
        return {"type": "chart", "props": chart_props(el)}

    if t in WIDGET_TYPES:
        return canon_widget(el, p, extra, positional)

    # Containers -------------------------------------------------------------
    if t == "column":
        return {"type": "column", "props": {}, "children": canon_children(el, extra, positional)}
    if t == "tab":
        return {
            "type": "tab",
            "props": {"label": field(p, "label", "")},
            "children": canon_children(el, extra, positional),
        }
    if t == "expander":
        return {
            "type": "expander",
            "props": {"label": field(p, "label", ""), "expanded": bool(field(p, "expanded", False))},
            "children": canon_children(el, extra, positional),
        }
    if t == "tab_container":
        kids = canon_children(el, extra, positional)
        return {
            "type": "tabs",
            "props": {"labels": [k["props"].get("label") for k in kids]},
            "children": kids,
        }
    if t == "form":
        return {
            "type": "form",
            "props": {"key": field(field(p, "form"), "form_id", "")},
            "children": canon_children(el, extra, positional),
        }
    if t in ("flex_container", "vertical_block", "horizontal_block"):
        kids = canon_children(el, extra, positional)
        if kids and all(k["type"] == "column" for k in kids):
            return {"type": "columns", "props": {"n": len(kids)}, "children": kids}
        return {"type": "container", "props": {}, "children": kids}

    return {
        "type": t,
        "props": {"unmapped": True},
        "children": canon_children(el, extra, positional),
    }


def canon_widget(el, p, extra: set, positional: dict):
    t = el.type
    props = {"key": widget_key(el, positional), "label": field(p, "label", "")}
    val = widget_value(el)

    if t == "file_uploader":
        files = []
        if val is not None:
            raw = el.value
            raw = raw if isinstance(raw, list) else [raw]
            files = [f.name for f in raw if f is not None]
        props["files"] = files
        props["value"] = files
    elif t == "button":
        # Opt-in: see go/run.go for why a button's value is not core.
        if "value" in extra:
            props["value"] = bool(val)
    else:
        props["value"] = val

    if t in ("radio", "selectbox", "multiselect", "select_slider"):
        props["options"] = list(field(p, "options", []))
    if t in ("slider", "select_slider"):
        props["min"] = field(p, "min")
        props["max"] = field(p, "max")
        props["step"] = field(p, "step")
    if t == "number_input":
        props["min"] = field(p, "min") if field(p, "has_min", False) else None
        props["max"] = field(p, "max") if field(p, "has_max", False) else None
        if "step" in extra:
            props["step"] = field(p, "step")

    if t in ("radio", "selectbox"):
        opts = props.get("options") or []
        props["index"] = opts.index(val) if val in opts else -1

    if "format" in extra:
        props["format"] = field(p, "format") or None
    if "help" in extra:
        props["help"] = field(p, "help") or None
    if "disabled" in extra:
        props["disabled"] = bool(field(p, "disabled", False))
    if "placeholder" in extra:
        props["placeholder"] = field(p, "placeholder") or None
    if "maxChars" in extra:
        props["maxChars"] = field(p, "max_chars") or None

    if t == "button" and field(p, "is_form_submitter", False):
        t = "form_submit"
        # Neither implementation lets the app name a submit button, so its
        # identity is positional on both sides.
        props["key"] = "#%d" % positional.get("form_submit", 0)
        positional["form_submit"] = positional.get("form_submit", 0) + 1

    # Keep only the properties this profile compares, so a prop one side cannot
    # model never masquerades as a wrong value.
    allowed = set(CORE_WIDGET_PROPS) | {"key", "files"} | set(extra)
    props = {k: v for k, v in props.items() if k in allowed}
    return {"type": t, "props": props}


def canon_children(node, extra: set, positional: dict):
    kids = getattr(node, "children", None)
    if not isinstance(kids, dict):
        return []
    out = []
    for idx in sorted(kids):
        out.append(canon_element(kids[idx], extra, positional))
    return out


def canon_tree(at, extra: set):
    positional: dict = {}
    main = canon_children(at.main, extra, positional)
    side = canon_children(at.sidebar, extra, positional)
    return main, side


def app_state(at):
    out = {}
    for k in at.session_state.filtered_state:
        if str(k).startswith("ss_"):
            out[str(k)] = jsonable(at.session_state[k])
    return {k: out[k] for k in sorted(out)}


def snapshot(at, extra: set):
    main, side = canon_tree(at, extra)
    return {"main": main, "sidebar": side, "state": app_state(at)}


# ------------------------------------------------------------------- driving it


def all_widgets(at):
    """Every widget in document order, main body then sidebar."""
    out = []

    def walk(node):
        kids = getattr(node, "children", None)
        if not isinstance(kids, dict):
            return
        for idx in sorted(kids):
            child = kids[idx]
            if child.type in WIDGET_TYPES:
                out.append(child)
            walk(child)

    walk(at.main)
    walk(at.sidebar)
    return out


def locate(at, step):
    """Resolve the widget a step addresses, by key or by (type, index)."""
    widgets = all_widgets(at)
    if step.get("key"):
        for w in widgets:
            if getattr(w, "key", None) == step["key"]:
                return w
        raise KeyError("no widget with key %r" % step["key"])
    typ, idx = step.get("type"), step.get("index", 0)
    same = [w for w in widgets if w.type == typ]
    if idx >= len(same):
        raise KeyError("no %s widget at index %d" % (typ, idx))
    return same[idx]


def coerce(widget, value):
    t = widget.type
    if t == "date_input":
        return datetime.date.fromisoformat(value)
    if t == "time_input":
        h, m = str(value).split(":")[:2]
        return datetime.time(int(h), int(m))
    if t in ("multiselect",):
        return list(value)
    if t in ("slider", "number_input"):
        return float(value)
    return value


def find_submit(at, form_key):
    def walk(node):
        kids = getattr(node, "children", None)
        if not isinstance(kids, dict):
            return None
        for idx in sorted(kids):
            child = kids[idx]
            if child.type == "button" and field(child.proto, "is_form_submitter", False):
                if field(child.proto, "form_id", "") == form_key:
                    return child
            found = walk(child)
            if found is not None:
                return found
        return None

    return walk(at.main) or walk(at.sidebar)


def run_case(fn, steps, extra):
    script = APPS[fn]
    # Both cache tables are process-wide, exactly as in a deployed app, so they
    # are cleared per case to keep the runner deterministic and order-free.
    st.cache_data.clear()
    st.cache_resource.clear()

    at = AppTest.from_string(script, default_timeout=30)
    at.run()
    snaps = [snapshot(at, extra)]

    staged: dict = {}
    for step in steps:
        op = step.get("op")
        if op == "run":
            at.run()
        elif op == "set":
            locate(at, step).set_value(coerce(locate(at, step), step["value"]))
            at.run()
        elif op == "stage":
            # A form widget's change never reaches the server until submission,
            # so staged values are held here and applied by the submit step.
            staged[json.dumps({k: step[k] for k in ("key", "type", "index") if k in step})] = step["value"]
            # No request reaches the server, so there is no new tree to snapshot.
            continue
        elif op == "submit":
            for addr, value in sorted(staged.items()):
                target = locate(at, json.loads(addr))
                target.set_value(coerce(target, value))
            staged = {}
            btn = find_submit(at, step["form"])
            if btn is None:
                raise KeyError("no submit button for form %r" % step["form"])
            btn.click()
            at.run()
        elif op == "click":
            locate(at, step).click()
            at.run()
        elif op == "upload":
            raise NotImplementedError("upload steps are not modelled")
        else:
            raise ValueError("unknown step op %r" % op)
        snaps.append(snapshot(at, extra))

    if at.exception:
        msgs = [e.value for e in at.exception]
        raise RuntimeError("app raised: " + "; ".join(msgs))
    return {"snapshots": snaps}


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:
            print(json.dumps({"id": "", "ok": False, "error": "bad request: %s" % exc}), flush=True)
            continue
        cid = req.get("id", "")
        try:
            args = req.get("args") or []
            steps = args[0] if len(args) > 0 and args[0] is not None else []
            opts = args[1] if len(args) > 1 and isinstance(args[1], dict) else {}
            value = run_case(req["fn"], steps, set(opts.get("extra") or ()))
            out = {"id": cid, "ok": True, "value": value}
        except Exception as exc:
            print(traceback.format_exc(), file=sys.stderr)
            out = {"id": cid, "ok": False, "error": "%s: %s" % (type(exc).__name__, exc)}
        print(json.dumps(out, sort_keys=True), flush=True)


if __name__ == "__main__":
    main()
