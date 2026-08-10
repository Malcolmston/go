#!/usr/bin/env python3
"""Upstream (matplotlib) parity runner.

Speaks JSON Lines on stdio: one request object per line in, one response
object per line out, in the same order. See ../../HARNESS.md.

Scope: this runner deliberately never compares rendered pixels. Every `fn`
below returns *computed numeric or semantic* output that precedes rasterising:
axis limits, tick locations, tick label strings, histogram bin edges/counts,
colour conversions, colormap samples, normalisations, the data->axes
coordinate transform, legend/label plumbing, and the file format actually
written by savefig. See ../COVERAGE.md.

Determinism: the Agg backend is forced, rcParams are reset to the library
defaults for every case, no random numbers are used and no timestamps enter
the output. Figures are always closed.
"""

import base64
import io
import json
import math
import os
import sys
import tempfile

import matplotlib

matplotlib.use("Agg")  # headless, deterministic raster backend

import matplotlib.colors as mcolors  # noqa: E402
import matplotlib.pyplot as plt  # noqa: E402
import numpy as np  # noqa: E402

EPS = 1e-9


def enc_num(x):
    """JSON has no NaN/Inf literal; use the same sentinels as the Go runner."""
    x = float(x)
    if math.isnan(x):
        return "NaN"
    if x == math.inf:
        return "Infinity"
    if x == -math.inf:
        return "-Infinity"
    return x


def nums(seq):
    return [enc_num(v) for v in seq]


def rgba(c):
    """A colour as a 4-element list of floats in [0,1]."""
    return nums(mcolors.to_rgba(c))


def new_fig(a):
    figsize = a.get("figsize", [6.4, 4.8])
    dpi = a.get("dpi", 100)
    fig = plt.figure(figsize=(figsize[0], figsize[1]), dpi=dpi)
    ax = fig.add_subplot(111)
    return fig, ax


def draw_series(ax, a):
    kind = a.get("kind", "plot")
    if kind == "plot":
        ax.plot(a["x"], a["y"])
    elif kind == "scatter":
        ax.scatter(a["x"], a["y"])
    elif kind == "hist":
        ax.hist(a["x"], bins=a.get("bins", 10))
    elif kind == "bar":
        ax.bar(a["labels"], a["y"])
    elif kind == "barh":
        ax.barh(a["labels"], a["y"])
    elif kind == "pie":
        ax.pie(a["y"], labels=a.get("labels"))
    else:
        raise ValueError("unknown kind: %r" % (kind,))


def visible(ticks, lo, hi):
    """Locator ticks clipped to the axis limits.

    matplotlib's get_xticks() reports every location the locator produced,
    including ones outside the view; only the in-range ones are drawn, and the
    Go port only ever emits the drawn ones. Compare the drawn set.
    """
    lo, hi = min(lo, hi), max(lo, hi)
    span = hi - lo
    tol = 1e-9 * (abs(span) if span else 1.0)
    return [float(t) for t in ticks if lo - tol <= t <= hi + tol]


# ------------------------------------------------------------------ handlers
def fn_axis_ticks(a):
    fig, ax = new_fig(a)
    try:
        draw_series(ax, a)
        if a.get("xlim") is not None:
            ax.set_xlim(a["xlim"][0], a["xlim"][1])
        if a.get("ylim") is not None:
            ax.set_ylim(a["ylim"][0], a["ylim"][1])
        fig.canvas.draw()
        xlo, xhi = ax.get_xlim()
        ylo, yhi = ax.get_ylim()
        return {
            "xlim": nums([xlo, xhi]),
            "ylim": nums([ylo, yhi]),
            "xticks": nums(visible(ax.get_xticks(), xlo, xhi)),
            "yticks": nums(visible(ax.get_yticks(), ylo, yhi)),
        }
    finally:
        plt.close(fig)


def fn_tick_labels(a):
    fig, ax = new_fig(a)
    try:
        draw_series(ax, a)
        if a.get("xlim") is not None:
            ax.set_xlim(a["xlim"][0], a["xlim"][1])
        fig.canvas.draw()
        xlo, xhi = ax.get_xlim()
        lo, hi = min(xlo, xhi), max(xlo, xhi)
        tol = 1e-9 * (abs(hi - lo) or 1.0)
        out = []
        for t, lab in zip(ax.get_xticks(), ax.get_xticklabels()):
            if lo - tol <= t <= hi + tol:
                out.append(lab.get_text())
        return {"xticklabels": out}
    finally:
        plt.close(fig)


def fn_cat_ticks(a):
    fig, ax = new_fig(a)
    try:
        draw_series(ax, a)
        fig.canvas.draw()
        horiz = a.get("kind") == "barh"
        if horiz:
            lo, hi = ax.get_ylim()
            ticks, labels = ax.get_yticks(), ax.get_yticklabels()
        else:
            lo, hi = ax.get_xlim()
            ticks, labels = ax.get_xticks(), ax.get_xticklabels()
        lo, hi = min(lo, hi), max(lo, hi)
        keep = [(float(t), l.get_text()) for t, l in zip(ticks, labels) if lo <= t <= hi]
        return {
            "catticks": nums([t for t, _ in keep]),
            "catlabels": [l for _, l in keep],
            "catlim": nums([lo, hi]),
        }
    finally:
        plt.close(fig)


def fn_hist_bins(a):
    counts, edges = np.histogram(np.asarray(a["data"], dtype=float), bins=a.get("bins", 10))
    return {"counts": [int(c) for c in counts], "edges": nums(edges)}


def fn_data_to_axes(a):
    fig, ax = new_fig(a)
    try:
        ax.plot(a["x"], a["y"])
        ax.set_xlim(a["xlim"][0], a["xlim"][1])
        ax.set_ylim(a["ylim"][0], a["ylim"][1])
        fig.canvas.draw()
        fx, fy = ax.transLimits.transform((a["point"][0], a["point"][1]))
        return {"axesfrac": nums([fx, fy])}
    finally:
        plt.close(fig)


def fn_labels(a):
    fig, ax = new_fig(a)
    try:
        for s in a.get("series", []):
            ax.plot(s["x"], s["y"], label=s.get("label"))
        ax.set_title(a.get("title", ""))
        ax.set_xlabel(a.get("xlabel", ""))
        ax.set_ylabel(a.get("ylabel", ""))
        legend = []
        if a.get("legend"):
            leg = ax.legend()
            fig.canvas.draw()
            if leg is not None:
                legend = [t.get_text() for t in leg.get_texts()]
        else:
            fig.canvas.draw()
        return {
            "title": ax.get_title(),
            "xlabel": ax.get_xlabel(),
            "ylabel": ax.get_ylabel(),
            "legend": legend,
        }
    finally:
        plt.close(fig)


MAGIC = [
    (b"\x89PNG\r\n\x1a\n", "png"),
    (b"%PDF", "pdf"),
    (b"\xff\xd8\xff", "jpeg"),
    (b"<?xml", "svg"),
    (b"<svg", "svg"),
    (b"%!PS", "ps"),
]


def sniff(head):
    for magic, name in MAGIC:
        if head.startswith(magic):
            return name
    return "unknown:" + base64.b16encode(head[:8]).decode("ascii").lower()


def fn_save_ext(a):
    fig, ax = new_fig(a)
    try:
        ax.plot([0, 1, 2], [0, 1, 4])
        d = tempfile.mkdtemp(prefix="parity-mpl-")
        path = os.path.join(d, "out" + a["ext"])
        fig.savefig(path)
        with open(path, "rb") as fh:
            head = fh.read(16)
        size = os.path.getsize(path)
        os.remove(path)
        os.rmdir(d)
        return {"exists": True, "format": sniff(head), "nonempty": size > 0}
    finally:
        plt.close(fig)


def fn_glyph_supported(a):
    """Does the default font actually contain a glyph for this character?

    Asked of the real font table, not of pixels. The Go port answers the same
    question by rendering the character and checking whether anything was
    drawn.
    """
    from matplotlib import font_manager, ft2font

    path = font_manager.findfont(font_manager.FontProperties())
    face = ft2font.FT2Font(path)
    ch = a["char"]
    if len(ch) != 1:
        raise ValueError("char must be exactly one character")
    return {"supported": face.get_char_index(ord(ch)) != 0}


def fn_dpi_effect(a):
    """Build the figure at the library-default 100 dpi, then change its DPI.

    Both sides start from a figure whose pixel size is figsize*100 and then
    assign a new DPI; the question is whether the rendered output changes size.
    """
    figsize = a["figsize"]
    fig = plt.figure(figsize=(figsize[0], figsize[1]), dpi=100)
    try:
        fig.add_subplot(111).plot([0, 1], [0, 1])
        fig.set_dpi(a["dpi"])
        fig.canvas.draw()
        w, h = fig.canvas.get_width_height()
        return {"width": int(w), "height": int(h)}
    finally:
        plt.close(fig)


def fn_hex_parse(a):
    return {"rgba": rgba(a["s"])}


def fn_hex_format(a):
    r, g, b = a["rgb"]
    return {"hex": mcolors.to_hex((r / 255.0, g / 255.0, b / 255.0))}


def fn_named_color(a):
    return {"rgba": rgba(a["name"])}


def fn_cycle_color(a):
    cyc = plt.rcParams["axes.prop_cycle"].by_key()["color"]
    i = int(a["index"]) % len(cyc)
    return {"rgba": rgba(cyc[i]), "cyclelen": len(cyc)}


def fn_cmap_sample(a):
    cm = plt.get_cmap(a["name"])
    return {"rgba": [nums(cm(float(t))) for t in a["fractions"]]}


def unmask(r):
    """A Normalize result as a plain float; a masked result becomes NaN."""
    arr = np.ma.asarray(r)
    if np.ma.getmaskarray(arr).any():
        return math.nan
    return float(np.ravel(np.ma.getdata(arr))[0])


def fn_normalize(a):
    n = mcolors.Normalize(vmin=a["min"], vmax=a["max"], clip=a.get("clip", False))
    out = []
    for v in a["values"]:
        with np.errstate(all="ignore"):
            out.append(enc_num(unmask(n(float(v)))))
    return {"norm": out}


def fn_lognorm(a):
    n = mcolors.LogNorm(vmin=a["min"], vmax=a["max"], clip=a.get("clip", False))
    out = []
    for v in a["values"]:
        with np.errstate(all="ignore"):
            out.append(enc_num(unmask(n(float(v)))))
    return {"norm": out}


def fn_rgb_to_hsv(a):
    h, s, v = mcolors.rgb_to_hsv(np.asarray(a["rgb"], dtype=float))
    return {"hsv": nums([h, s, v])}


def fn_hsv_to_rgb(a):
    r, g, b = mcolors.hsv_to_rgb(np.asarray(a["hsv"], dtype=float))
    return {"rgb": nums([r, g, b])}


HANDLERS = {
    "axis_ticks": fn_axis_ticks,
    "tick_labels": fn_tick_labels,
    "cat_ticks": fn_cat_ticks,
    "hist_bins": fn_hist_bins,
    "data_to_axes": fn_data_to_axes,
    "labels": fn_labels,
    "save_ext": fn_save_ext,
    "glyph_supported": fn_glyph_supported,
    "dpi_effect": fn_dpi_effect,
    "hex_parse": fn_hex_parse,
    "hex_format": fn_hex_format,
    "named_color": fn_named_color,
    "cmap_sample": fn_cmap_sample,
    "cycle_color": fn_cycle_color,
    "normalize": fn_normalize,
    "lognorm": fn_lognorm,
    "rgb_to_hsv": fn_rgb_to_hsv,
    "hsv_to_rgb": fn_hsv_to_rgb,
}


def main():
    if "--version" in sys.argv:
        sys.stdout.write(matplotlib.__version__ + "\n")
        return
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        cid = None
        try:
            req = json.loads(line)
            cid = req.get("id")
            fn = req["fn"]
            if fn not in HANDLERS:
                raise KeyError("no handler for fn %r" % (fn,))
            # Swallow anything a handler prints; stdout is the wire.
            buf = io.StringIO()
            saved, sys.stdout = sys.stdout, buf
            try:
                matplotlib.rcdefaults()
                matplotlib.use("Agg")
                value = HANDLERS[fn](req.get("args") or {})
            finally:
                sys.stdout = saved
            if buf.getvalue():
                sys.stderr.write(buf.getvalue())
            out = {"id": cid, "ok": True, "value": value}
        except Exception as exc:  # never exit on a failing case
            out = {"id": cid, "ok": False, "error": "%s: %s" % (type(exc).__name__, exc)}
        sys.stdout.write(json.dumps(out, sort_keys=True) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
