#!/usr/bin/env python3
"""Upstream (OpenCV / cv2) parity runner.

Speaks JSON Lines on stdio: one request object per line in, one response object
per line out, in the same order. See ../../HARNESS.md.

Nothing is ever read from disk: every input image is generated *analytically*
from the generator spec carried in the case file, using the same closed-form
rules as go/run.go (including the LCG whose seed is supplied by the case, so
neither side ever touches its own RNG).

Value encoding (identical on the Go side):
  uint8 image  -> {"kind":"mat","rows":R,"cols":C,"channels":N,"hex":"<raw>"}
  float matrix -> {"kind":"fmat","rows":R,"cols":C,"data":[...]}
  everything else -> plain JSON scalars / arrays / objects
`hex` is the lowercase hex of the raw row-major interleaved buffer; no image is
ever PNG/JPEG encoded.  Non-finite floats become the sentinel strings "NaN",
"Infinity", "-Infinity".
"""

import json
import math
import sys

import cv2
import numpy as np

# --------------------------------------------------------------- enum tables

BORDER = {
    "BorderConstant": cv2.BORDER_CONSTANT,
    "BorderReplicate": cv2.BORDER_REPLICATE,
    "BorderReflect": cv2.BORDER_REFLECT,
    "BorderWrap": cv2.BORDER_WRAP,
    "BorderReflect101": cv2.BORDER_REFLECT101,
}

THRESH = {
    "ThreshBinary": cv2.THRESH_BINARY,
    "ThreshBinaryInv": cv2.THRESH_BINARY_INV,
    "ThreshTrunc": cv2.THRESH_TRUNC,
    "ThreshToZero": cv2.THRESH_TOZERO,
    "ThreshToZeroInv": cv2.THRESH_TOZERO_INV,
}

ADAPTIVE = {
    "AdaptiveThreshMeanC": cv2.ADAPTIVE_THRESH_MEAN_C,
    "AdaptiveThreshGaussianC": cv2.ADAPTIVE_THRESH_GAUSSIAN_C,
}

MORPH_SHAPE = {
    "MorphRect": cv2.MORPH_RECT,
    "MorphCross": cv2.MORPH_CROSS,
    "MorphEllipse": cv2.MORPH_ELLIPSE,
}

MORPH_OP = {
    "MorphErode": cv2.MORPH_ERODE,
    "MorphDilate": cv2.MORPH_DILATE,
    "MorphOpen": cv2.MORPH_OPEN,
    "MorphClose": cv2.MORPH_CLOSE,
    "MorphGradient": cv2.MORPH_GRADIENT,
    "MorphTophat": cv2.MORPH_TOPHAT,
    "MorphBlackhat": cv2.MORPH_BLACKHAT,
}

RETR = {
    "RetrExternal": cv2.RETR_EXTERNAL,
    "RetrList": cv2.RETR_LIST,
    "RetrTree": cv2.RETR_TREE,
}

CHAIN = {
    "ChainApproxNone": cv2.CHAIN_APPROX_NONE,
    "ChainApproxSimple": cv2.CHAIN_APPROX_SIMPLE,
}

INTERP = {
    "InterNearest": cv2.INTER_NEAREST,
    "InterLinear": cv2.INTER_LINEAR,
}

TM = {
    "TmSqdiff": cv2.TM_SQDIFF,
    "TmSqdiffNormed": cv2.TM_SQDIFF_NORMED,
    "TmCcoeff": cv2.TM_CCOEFF,
    "TmCcoeffNormed": cv2.TM_CCOEFF_NORMED,
    "TmCcorr": cv2.TM_CCORR,
    "TmCcorrNormed": cv2.TM_CCORR_NORMED,
}

HISTCMP = {
    "HistCmpCorrel": cv2.HISTCMP_CORREL,
    "HistCmpChiSqr": cv2.HISTCMP_CHISQR,
    "HistCmpIntersect": cv2.HISTCMP_INTERSECT,
    "HistCmpBhattacharyya": cv2.HISTCMP_BHATTACHARYYA,
}

COLOR = {
    "ColorRGB2Gray": cv2.COLOR_RGB2GRAY,
    "ColorGray2RGB": cv2.COLOR_GRAY2RGB,
    "ColorRGB2BGR": cv2.COLOR_RGB2BGR,
    "ColorBGR2RGB": cv2.COLOR_BGR2RGB,
    "ColorBGR2Gray": cv2.COLOR_BGR2GRAY,
    "ColorRGB2HSV": cv2.COLOR_RGB2HSV,
    "ColorHSV2RGB": cv2.COLOR_HSV2RGB,
    "ColorRGB2Lab": cv2.COLOR_RGB2Lab,
    "ColorLab2RGB": cv2.COLOR_Lab2RGB,
    "ColorRGB2YCrCb": cv2.COLOR_RGB2YCrCb,
    "ColorYCrCb2RGB": cv2.COLOR_YCrCb2RGB,
    "ColorRGB2HLS": cv2.COLOR_RGB2HLS,
    "ColorHLS2RGB": cv2.COLOR_HLS2RGB,
}

FLIP = {"FlipVertical": 0, "FlipHorizontal": 1, "FlipBoth": -1}

ROTATE = {
    "Rotate90CW": cv2.ROTATE_90_CLOCKWISE,
    "Rotate180": cv2.ROTATE_180,
    "Rotate90CCW": cv2.ROTATE_90_COUNTERCLOCKWISE,
}

REDUCE = {
    "ReduceSum": cv2.REDUCE_SUM,
    "ReduceAvg": cv2.REDUCE_AVG,
    "ReduceMax": cv2.REDUCE_MAX,
    "ReduceMin": cv2.REDUCE_MIN,
}

# plot.Colormap -> cv2 COLORMAP_*.  ColormapGrayscale has no cv2 counterpart;
# its documented definition (identity ramp in all three channels) is built here.
COLORMAP = {
    "ColormapJet": cv2.COLORMAP_JET,
    "ColormapHot": cv2.COLORMAP_HOT,
    "ColormapCool": cv2.COLORMAP_COOL,
    "ColormapBone": cv2.COLORMAP_BONE,
    "ColormapHSV": cv2.COLORMAP_HSV,
    "ColormapViridis": cv2.COLORMAP_VIRIDIS,
    "ColormapPlasma": cv2.COLORMAP_PLASMA,
    "ColormapGrayscale": None,
    "ColormapAutumn": cv2.COLORMAP_AUTUMN,
    "ColormapWinter": cv2.COLORMAP_WINTER,
    "ColormapSummer": cv2.COLORMAP_SUMMER,
    "ColormapSpring": cv2.COLORMAP_SPRING,
    "ColormapOcean": cv2.COLORMAP_OCEAN,
    "ColormapRainbow": cv2.COLORMAP_RAINBOW,
    "ColormapPink": cv2.COLORMAP_PINK,
    "ColormapParula": cv2.COLORMAP_PARULA,
    "ColormapMagma": cv2.COLORMAP_MAGMA,
    "ColormapInferno": cv2.COLORMAP_INFERNO,
    "ColormapCividis": cv2.COLORMAP_CIVIDIS,
    "ColormapTwilight": cv2.COLORMAP_TWILIGHT,
    "ColormapTurbo": cv2.COLORMAP_TURBO,
}


def pick(table, name):
    if name not in table:
        raise ValueError("unknown enum constant: %r" % (name,))
    return table[name]


# ------------------------------------------------------------- generators
#
# Every generator is closed-form integer arithmetic so the two runners agree
# bit-for-bit.  `noise` uses the classic glibc LCG with the seed taken from the
# case file, never from the language's own RNG.


def trunc_div(a, b):
    """Integer division truncating toward zero (Go's `/`, not Python's `//`)."""
    q = abs(a) // abs(b)
    return -q if (a < 0) != (b < 0) else q


def _lcg(seed, n):
    state = seed % (1 << 31)
    out = []
    for _ in range(n):
        state = (state * 1103515245 + 12345) % (1 << 31)
        out.append((state >> 16) & 255)
    return out


DISCS = [(8, 8, 5, 255), (24, 10, 6, 180), (14, 26, 4, 120), (36, 36, 7, 200)]
RECTS = [(28, 20, 40, 30, 220)]

BLOB_RECTS = [(2, 2, 6, 6), (10, 3, 16, 9), (20, 20, 22, 22), (4, 25, 9, 30),
              (25, 5, 27, 7), (28, 8, 30, 10)]

CIRCLES = [(12, 12, 6), (32, 14, 8), (20, 34, 5)]


def _spread(v, ch):
    """Replicate a single intensity across `ch` channels, deterministically."""
    if ch == 1:
        return [v]
    return [trunc_div(v * (10 - c), 10) for c in range(ch)]


def gen_image(spec):
    kind = spec["gen"]
    rows = int(spec.get("rows", 32))
    cols = int(spec.get("cols", 32))
    ch = int(spec.get("ch", 1))
    buf = np.zeros((rows, cols, ch), dtype=np.uint8)

    if kind == "gradient":
        for y in range(rows):
            for x in range(cols):
                for c in range(ch):
                    buf[y, x, c] = (x * 7 + y * 5 + c * 37) % 256
    elif kind == "const":
        v = int(spec["value"])
        for c in range(ch):
            buf[:, :, c] = (v + c * 20) % 256
    elif kind == "checker":
        b = int(spec.get("block", 4))
        for y in range(rows):
            for x in range(cols):
                on = ((x // b) + (y // b)) % 2 == 0
                for c in range(ch):
                    buf[y, x, c] = (200 + c * 15) % 256 if on else (30 + c * 5) % 256
    elif kind == "noise":
        vals = _lcg(int(spec["seed"]), rows * cols * ch)
        i = 0
        for y in range(rows):
            for x in range(cols):
                for c in range(ch):
                    buf[y, x, c] = vals[i]
                    i += 1
    elif kind == "gradnoise":
        amp = int(spec.get("amp", 40))
        vals = _lcg(int(spec["seed"]), rows * cols * ch)
        i = 0
        for y in range(rows):
            for x in range(cols):
                for c in range(ch):
                    g = (x * 7 + y * 5 + c * 37) % 256
                    d = trunc_div((vals[i] - 128) * amp, 128)
                    buf[y, x, c] = max(0, min(255, g + d))
                    i += 1
    elif kind == "shapes":
        for cx, cy, rad, val in DISCS:
            for y in range(rows):
                for x in range(cols):
                    if (x - cx) ** 2 + (y - cy) ** 2 <= rad * rad:
                        for c, cv_ in enumerate(_spread(val, ch)):
                            buf[y, x, c] = cv_
        for x0, y0, x1, y1, val in RECTS:
            for y in range(max(0, y0), min(rows, y1 + 1)):
                for x in range(max(0, x0), min(cols, x1 + 1)):
                    for c, cv_ in enumerate(_spread(val, ch)):
                        buf[y, x, c] = cv_
    elif kind == "blobs":
        for x0, y0, x1, y1 in BLOB_RECTS:
            for y in range(max(0, y0), min(rows, y1 + 1)):
                for x in range(max(0, x0), min(cols, x1 + 1)):
                    for c in range(ch):
                        buf[y, x, c] = 255
    elif kind == "lines":
        for x in range(cols):
            if 10 < rows:
                for c in range(ch):
                    buf[10, x, c] = 255
        for y in range(rows):
            if 20 < cols:
                for c in range(ch):
                    buf[y, 20, c] = 255
        for d in range(5, 36):
            if d < rows and d < cols:
                for c in range(ch):
                    buf[d, d, c] = 255
    elif kind == "circles":
        for cx, cy, rad in CIRCLES:
            for y in range(rows):
                for x in range(cols):
                    if (x - cx) ** 2 + (y - cy) ** 2 <= rad * rad:
                        for c in range(ch):
                            buf[y, x, c] = 255
    else:
        raise ValueError("unknown image generator: %r" % (kind,))

    if ch == 1:
        return buf[:, :, 0].copy()
    return buf


def gen_fmat(spec):
    """Deterministic float matrices: remap tables and small dense matrices."""
    kind = spec["fgen"]
    rows = int(spec.get("rows", 32))
    cols = int(spec.get("cols", 32))
    out = np.zeros((rows, cols), dtype=np.float64)
    if kind == "mapx-flip":
        for y in range(rows):
            for x in range(cols):
                out[y, x] = float(cols - 1 - x)
    elif kind == "mapy-id":
        for y in range(rows):
            for x in range(cols):
                out[y, x] = float(y)
    elif kind == "mapx-id":
        for y in range(rows):
            for x in range(cols):
                out[y, x] = float(x)
    elif kind == "mapx-shift":
        d = float(spec.get("d", 2.5))
        for y in range(rows):
            for x in range(cols):
                out[y, x] = x + d
    elif kind == "mapx-wobble":
        for y in range(rows):
            for x in range(cols):
                out[y, x] = x + ((y % 8) - 4) / 4.0
    elif kind == "mapy-wobble":
        for y in range(rows):
            for x in range(cols):
                out[y, x] = y + ((x % 8) - 4) / 4.0
    elif kind == "literal":
        data = spec["data"]
        for y in range(rows):
            for x in range(cols):
                out[y, x] = float(data[y * cols + x])
    else:
        raise ValueError("unknown float generator: %r" % (kind,))
    return out


def dec_arg(v):
    if isinstance(v, dict):
        if "gen" in v:
            return gen_image(v)
        if "fgen" in v:
            return gen_fmat(v)
    return v


# ----------------------------------------------------------------- encoding


def enc_num(x):
    x = float(x)
    if math.isnan(x):
        return "NaN"
    if x == math.inf:
        return "Infinity"
    if x == -math.inf:
        return "-Infinity"
    return x


def enc_mat(a):
    a = np.ascontiguousarray(a)
    if a.dtype != np.uint8:
        raise TypeError("enc_mat wants uint8, got %s" % a.dtype)
    if a.ndim == 2:
        rows, cols, ch = a.shape[0], a.shape[1], 1
    elif a.ndim == 3:
        rows, cols, ch = a.shape
    else:
        raise TypeError("enc_mat wants 2 or 3 dims, got %d" % a.ndim)
    return {"kind": "mat", "rows": int(rows), "cols": int(cols),
            "channels": int(ch), "hex": a.tobytes().hex()}


def enc_fmat(a):
    a = np.asarray(a, dtype=np.float64)
    if a.ndim == 1:
        a = a.reshape(1, -1)
    rows, cols = a.shape[0], a.shape[1]
    return {"kind": "fmat", "rows": int(rows), "cols": int(cols),
            "data": [enc_num(v) for v in a.reshape(-1)]}


def enc_floats(xs):
    return [enc_num(v) for v in xs]


def enc_ints(xs):
    return [int(v) for v in xs]


def pts_list(c):
    """cv2 contour/point array -> plain [[x,y], ...]."""
    return [[int(p[0]), int(p[1])] for p in np.asarray(c).reshape(-1, 2)]


def enc_contours(cnts):
    """Structural encoding: order-independent, no float scores.

    Contours are sorted by (vertex count, bounding rect) and each carries its
    own points sorted lexicographically, so a differing traversal start point
    or winding direction does not register as a divergence while a differing
    boundary does.
    """
    items = []
    for c in cnts:
        pts = pts_list(c)
        x, y, w, h = cv2.boundingRect(np.asarray(c))
        items.append({"n": len(pts), "rect": [int(x), int(y), int(w), int(h)],
                      "pts": sorted(pts)})
    items.sort(key=lambda d: (d["n"], d["rect"], d["pts"]))
    return items


# --------------------------------------------------------------------- ops

# The port documents edge replication for all of its filters, so the cv2 oracle
# is invoked with BORDER_REPLICATE and the *Reflect101 ops below re-run a few of
# them with cv2's own default border to isolate that divergence.
REPL = cv2.BORDER_REPLICATE

OPS = {}


def op(name):
    def deco(f):
        OPS[name] = f
        return f
    return deco


def gray(img):
    if img.ndim == 3:
        raise ValueError("expected single-channel image")
    return img


# ---- mat semantics -------------------------------------------------------


@op("matInfo")
def _(a):
    m = a[0]
    ch = 1 if m.ndim == 2 else m.shape[2]
    return {"rows": int(m.shape[0]), "cols": int(m.shape[1]), "channels": int(ch),
            "total": int(m.shape[0] * m.shape[1]), "empty": bool(m.size == 0)}


@op("newMat")
def _(a):
    rows, cols, ch = int(a[0]), int(a[1]), int(a[2])
    if rows <= 0 or cols <= 0 or ch <= 0:
        raise ValueError("non-positive dimension")
    if ch == 1:
        return enc_mat(np.zeros((rows, cols), np.uint8))
    return enc_mat(np.zeros((rows, cols, ch), np.uint8))


@op("matAt")
def _(a):
    m, y, x, c = a[0], int(a[1]), int(a[2]), int(a[3])
    if m.ndim == 2:
        if c != 0:
            raise IndexError("channel out of range")
        return int(m[y, x])
    return int(m[y, x, c])


@op("matSet")
def _(a):
    m, y, x, c, v = a[0].copy(), int(a[1]), int(a[2]), int(a[3]), int(a[4])
    if y < 0 or x < 0 or y >= m.shape[0] or x >= m.shape[1]:
        raise IndexError("out of range")
    if m.ndim == 2:
        if c != 0:
            raise IndexError("channel out of range")
        m[y, x] = v
    else:
        m[y, x, c] = v
    return enc_mat(m)


@op("matSetPixel")
def _(a):
    m, y, x, vals = a[0].copy(), int(a[1]), int(a[2]), a[3]
    if m.ndim == 2:
        if len(vals) != 1:
            raise ValueError("wrong number of channel values")
        m[y, x] = int(vals[0])
    else:
        if len(vals) != m.shape[2]:
            raise ValueError("wrong number of channel values")
        m[y, x] = [int(v) for v in vals]
    return enc_mat(m)


@op("matPixel")
def _(a):
    m, y, x = a[0], int(a[1]), int(a[2])
    if m.ndim == 2:
        return [int(m[y, x])]
    return [int(v) for v in m[y, x]]


@op("matRegion")
def _(a):
    m, y, x, h, w = a[0], int(a[1]), int(a[2]), int(a[3]), int(a[4])
    if h <= 0 or w <= 0 or y < 0 or x < 0 or y + h > m.shape[0] or x + w > m.shape[1]:
        raise IndexError("region out of range")
    return enc_mat(m[y:y + h, x:x + w].copy())


@op("matRegionWrites")
def _(a):
    """Does writing through a region change the parent?  In OpenCV/numpy the
    ROI aliases the parent; the port documents a deep copy instead."""
    m, y, x, h, w, v = a[0].copy(), int(a[1]), int(a[2]), int(a[3]), int(a[4]), int(a[5])
    roi = m[y:y + h, x:x + w]
    roi[...] = v
    return enc_mat(m)


@op("matClone")
def _(a):
    return enc_mat(a[0].copy())


@op("matCloneIsolated")
def _(a):
    m = a[0]
    c = m.copy()
    c[...] = 7
    return enc_mat(m)


@op("matCopyTo")
def _(a):
    src, dst, y, x = a[0], a[1].copy(), int(a[2]), int(a[3])
    if y < 0 or x < 0 or y + src.shape[0] > dst.shape[0] or x + src.shape[1] > dst.shape[1]:
        raise IndexError("copy out of range")
    dst[y:y + src.shape[0], x:x + src.shape[1]] = src
    return enc_mat(dst)


@op("matSetTo")
def _(a):
    m = a[0].copy()
    m[...] = int(a[1])
    return enc_mat(m)


@op("matSplit")
def _(a):
    return [enc_mat(p) for p in cv2.split(a[0])]


@op("matSplitMerge")
def _(a):
    return enc_mat(cv2.merge(cv2.split(a[0])))


@op("mergeReversed")
def _(a):
    return enc_mat(cv2.merge(list(reversed(cv2.split(a[0])))))


@op("extractChannel")
def _(a):
    m, coi = a[0], int(a[1])
    if m.ndim == 2:
        if coi != 0:
            raise IndexError("channel out of range")
        return enc_mat(m.copy())
    if coi < 0 or coi >= m.shape[2]:
        raise IndexError("channel out of range")
    return enc_mat(m[:, :, coi].copy())


@op("insertChannel")
def _(a):
    src, dst, coi = a[0], a[1].copy(), int(a[2])
    if src.ndim != 2:
        raise ValueError("source must be single channel")
    dst[:, :, coi] = src
    return enc_mat(dst)


@op("mixChannels")
def _(a):
    """fromTo pairs over a single 3-channel source and a single 3-channel dst."""
    src, dst, from_to = a[0], a[1].copy(), a[2]
    ft = []
    for pair in from_to:
        ft.extend([int(pair[0]), int(pair[1])])
    cv2.mixChannels([src], [dst], ft)
    return enc_mat(dst)


@op("transpose")
def _(a):
    return enc_mat(cv2.transpose(a[0]))


@op("flip")
def _(a):
    return enc_mat(cv2.flip(a[0], pick(FLIP, a[1])))


@op("rotate")
def _(a):
    return enc_mat(cv2.rotate(a[0], pick(ROTATE, a[1])))


@op("copyMakeBorder")
def _(a):
    m, t, b, l, r, bt, val = a[0], int(a[1]), int(a[2]), int(a[3]), int(a[4]), a[5], a[6]
    return enc_mat(cv2.copyMakeBorder(m, t, b, l, r, pick(BORDER, bt),
                                     value=[float(v) for v in val]))


@op("borderInterpolate")
def _(a):
    return int(cv2.borderInterpolate(int(a[0]), int(a[1]), pick(BORDER, a[2])))


# ---- arithmetic ----------------------------------------------------------


@op("add")
def _(a):
    return enc_mat(cv2.add(a[0], a[1]))


@op("subtract")
def _(a):
    return enc_mat(cv2.subtract(a[0], a[1]))


@op("absdiff")
def _(a):
    return enc_mat(cv2.absdiff(a[0], a[1]))


@op("multiply")
def _(a):
    return enc_mat(cv2.multiply(a[0], a[1], scale=float(a[2])))


@op("divide")
def _(a):
    return enc_mat(cv2.divide(a[0], a[1], scale=float(a[2])))


@op("addWeighted")
def _(a):
    return enc_mat(cv2.addWeighted(a[0], float(a[1]), a[2], float(a[3]), float(a[4])))


@op("bitwiseAnd")
def _(a):
    return enc_mat(cv2.bitwise_and(a[0], a[1]))


@op("bitwiseOr")
def _(a):
    return enc_mat(cv2.bitwise_or(a[0], a[1]))


@op("bitwiseXor")
def _(a):
    return enc_mat(cv2.bitwise_xor(a[0], a[1]))


@op("bitwiseNot")
def _(a):
    return enc_mat(cv2.bitwise_not(a[0]))


@op("minMat")
def _(a):
    return enc_mat(cv2.min(a[0], a[1]))


@op("maxMat")
def _(a):
    return enc_mat(cv2.max(a[0], a[1]))


@op("inRange")
def _(a):
    lo = np.array([int(v) for v in a[1]], np.uint8)
    hi = np.array([int(v) for v in a[2]], np.uint8)
    return enc_mat(cv2.inRange(a[0], lo, hi))


@op("convertScaleAbs")
def _(a):
    return enc_mat(cv2.convertScaleAbs(a[0], alpha=float(a[1]), beta=float(a[2])))


@op("normalizeMinMax")
def _(a):
    out = cv2.normalize(a[0], None, alpha=float(a[1]), beta=float(a[2]),
                        norm_type=cv2.NORM_MINMAX, dtype=cv2.CV_8U)
    return enc_mat(out)


@op("countNonZero")
def _(a):
    return int(cv2.countNonZero(gray(a[0])))


@op("findNonZero")
def _(a):
    nz = cv2.findNonZero(gray(a[0]))
    pts = [] if nz is None else pts_list(nz)
    return sorted(pts)


@op("mean")
def _(a):
    return enc_floats(cv2.mean(a[0]))


@op("meanStdDev")
def _(a):
    m, sd = cv2.meanStdDev(a[0])
    # cv2 returns one entry per channel; cv.MeanStdDev returns a 4-slot Scalar.
    def pad4(v):
        v = list(v.reshape(-1))
        return v + [0.0] * (4 - len(v))
    return {"mean": enc_floats(pad4(m)), "stddev": enc_floats(pad4(sd))}


@op("sumElems")
def _(a):
    return enc_floats(cv2.sumElems(a[0]))


@op("norm")
def _(a):
    kinds = {"L1": cv2.NORM_L1, "L2": cv2.NORM_L2, "Inf": cv2.NORM_INF}
    return enc_num(cv2.norm(a[0], kinds[a[1]]))


@op("lut")
def _(a):
    return enc_mat(cv2.LUT(a[0], np.array([int(v) for v in a[1]], np.uint8)))


@op("lutChannels")
def _(a):
    m, tables = a[0], a[1]
    planes = cv2.split(m)
    out = [cv2.LUT(p, np.array([int(v) for v in t], np.uint8))
           for p, t in zip(planes, tables)]
    return enc_mat(cv2.merge(out))


# ---- colour -------------------------------------------------------------


@op("cvtColor")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], pick(COLOR, a[1])))


@op("rgbToXYZ")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_RGB2XYZ))


@op("xyzToRGB")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_XYZ2RGB))


@op("rgbToYUV")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_RGB2YUV))


@op("yuvToRGB")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_YUV2RGB))


@op("rgbToHSVFull")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_RGB2HSV_FULL))


@op("hsvFullToRGB")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_HSV2RGB_FULL))


@op("rgbToGray601")
def _(a):
    return enc_mat(cv2.cvtColor(a[0], cv2.COLOR_RGB2GRAY))


@op("demosaic")
def _(a):
    codes = {"BayerRG": cv2.COLOR_BayerRG2RGB, "BayerGR": cv2.COLOR_BayerGR2RGB,
             "BayerBG": cv2.COLOR_BayerBG2RGB, "BayerGB": cv2.COLOR_BayerGB2RGB}
    return enc_mat(cv2.cvtColor(gray(a[0]), codes[a[1]]))


# ---- threshold ----------------------------------------------------------


@op("threshold")
def _(a):
    t, out = cv2.threshold(a[0], float(a[1]), float(a[2]), pick(THRESH, a[3]))
    return {"thresh": enc_num(t), "dst": enc_mat(out)}


@op("thresholdOtsu")
def _(a):
    t, out = cv2.threshold(gray(a[0]), 0, float(a[1]),
                           pick(THRESH, a[2]) | cv2.THRESH_OTSU)
    return {"thresh": enc_num(t), "dst": enc_mat(out)}


@op("thresholdTriangle")
def _(a):
    t, out = cv2.threshold(gray(a[0]), 0, 255,
                           cv2.THRESH_BINARY | cv2.THRESH_TRIANGLE)
    return {"thresh": enc_num(t), "dst": enc_mat(out)}


@op("adaptiveThreshold")
def _(a):
    out = cv2.adaptiveThreshold(gray(a[0]), float(a[1]), pick(ADAPTIVE, a[2]),
                                pick(THRESH, a[3]), int(a[4]), float(a[5]))
    return enc_mat(out)


# ---- filters ------------------------------------------------------------


@op("blur")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.blur(a[0], (k, k), borderType=REPL))


@op("boxFilter")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.boxFilter(a[0], -1, (k, k), normalize=bool(a[2]),
                                 borderType=REPL))


@op("gaussianBlur")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.GaussianBlur(a[0], (k, k), float(a[2]),
                                   borderType=REPL))


@op("medianBlur")
def _(a):
    return enc_mat(cv2.medianBlur(a[0], int(a[1])))


@op("bilateralFilter")
def _(a):
    return enc_mat(cv2.bilateralFilter(a[0], int(a[1]), float(a[2]), float(a[3]),
                                       borderType=REPL))


@op("filter2D")
def _(a):
    m, rows, cols, data, delta = a[0], int(a[1]), int(a[2]), a[3], float(a[4])
    k = np.array([float(v) for v in data], np.float64).reshape(rows, cols)
    return enc_mat(cv2.filter2D(m, -1, k, delta=delta,
                               borderType=REPL))


@op("sepFilter2D")
def _(a):
    m, kx, ky, delta = a[0], a[1], a[2], float(a[3])
    kxa = np.array([float(v) for v in kx], np.float64)
    kya = np.array([float(v) for v in ky], np.float64)
    return enc_mat(cv2.sepFilter2D(m, -1, kxa, kya, delta=delta,
                                  borderType=REPL))


@op("sqrBoxFilter")
def _(a):
    k = int(a[1])
    out = cv2.sqrBoxFilter(a[0], cv2.CV_64F, (k, k), normalize=bool(a[2]),
                           borderType=REPL)
    return enc_fmat(out)


@op("stackBlur")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.stackBlur(a[0], (k, k)))


@op("blurReflect101")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.blur(a[0], (k, k), borderType=cv2.BORDER_REFLECT101))


@op("gaussianBlurReflect101")
def _(a):
    k = int(a[1])
    return enc_mat(cv2.GaussianBlur(a[0], (k, k), float(a[2]),
                                   borderType=cv2.BORDER_REFLECT101))


@op("sobelReflect101")
def _(a):
    m, dx, dy, k, scale, delta = a[0], int(a[1]), int(a[2]), int(a[3]), float(a[4]), float(a[5])
    return enc_mat(cv2.Sobel(m, cv2.CV_8U, dx, dy, ksize=k, scale=scale, delta=delta,
                            borderType=cv2.BORDER_REFLECT101))


@op("laplacianReflect101")
def _(a):
    m, k, scale, delta = a[0], int(a[1]), float(a[2]), float(a[3])
    return enc_mat(cv2.Laplacian(m, cv2.CV_8U, ksize=k, scale=scale, delta=delta,
                                borderType=cv2.BORDER_REFLECT101))


@op("pyrDown")
def _(a):
    return enc_mat(cv2.pyrDown(a[0]))


@op("pyrUp")
def _(a):
    return enc_mat(cv2.pyrUp(a[0]))


@op("pyrDownTwice")
def _(a):
    return enc_mat(cv2.pyrDown(cv2.pyrDown(a[0])))


@op("pyrDownUp")
def _(a):
    return enc_mat(cv2.pyrUp(cv2.pyrDown(a[0])))


# ---- edges / derivatives ------------------------------------------------


@op("sobel")
def _(a):
    m, dx, dy, k, scale, delta = a[0], int(a[1]), int(a[2]), int(a[3]), float(a[4]), float(a[5])
    return enc_mat(cv2.Sobel(m, cv2.CV_8U, dx, dy, ksize=k, scale=scale, delta=delta,
                            borderType=REPL))


@op("sobelFloat")
def _(a):
    m, dx, dy, k = gray(a[0]), int(a[1]), int(a[2]), int(a[3])
    return enc_fmat(cv2.Sobel(m, cv2.CV_64F, dx, dy, ksize=k,
                              borderType=REPL))


@op("scharr")
def _(a):
    m, dx, dy, scale, delta = a[0], int(a[1]), int(a[2]), float(a[3]), float(a[4])
    return enc_mat(cv2.Scharr(m, cv2.CV_8U, dx, dy, scale=scale, delta=delta,
                             borderType=REPL))


@op("laplacian")
def _(a):
    m, k, scale, delta = a[0], int(a[1]), float(a[2]), float(a[3])
    return enc_mat(cv2.Laplacian(m, cv2.CV_8U, ksize=k, scale=scale, delta=delta,
                                borderType=REPL))


@op("canny")
def _(a):
    return enc_mat(cv2.Canny(gray(a[0]), float(a[1]), float(a[2])))


@op("spatialGradient")
def _(a):
    dx, dy = cv2.spatialGradient(gray(a[0]), borderType=cv2.BORDER_REPLICATE)
    return {"dx": enc_fmat(dx.astype(np.float64)), "dy": enc_fmat(dy.astype(np.float64))}


@op("cornerHarris")
def _(a):
    m, block, k, kk = gray(a[0]), int(a[1]), int(a[2]), float(a[3])
    return enc_fmat(cv2.cornerHarris(m.astype(np.float32), block, k, kk,
                                     borderType=REPL).astype(np.float64))


@op("cornerMinEigenVal")
def _(a):
    m, block, k = gray(a[0]), int(a[1]), int(a[2])
    return enc_fmat(cv2.cornerMinEigenVal(m.astype(np.float32), block, k,
                                          borderType=REPL).astype(np.float64))


@op("preCornerDetect")
def _(a):
    return enc_fmat(cv2.preCornerDetect(gray(a[0]).astype(np.float32), 3,
                                        borderType=REPL).astype(np.float64))


@op("getDerivKernels")
def _(a):
    kx, ky = cv2.getDerivKernels(int(a[0]), int(a[1]), int(a[2]))
    return {"kx": enc_floats(kx.reshape(-1)), "ky": enc_floats(ky.reshape(-1))}


@op("getGaussianKernel")
def _(a):
    k = cv2.getGaussianKernel(int(a[0]), float(a[1]))
    return enc_floats(k.reshape(-1))


@op("getGaborKernel")
def _(a):
    k = int(a[0])
    g = cv2.getGaborKernel((k, k), float(a[1]), float(a[2]), float(a[3]),
                           float(a[4]), float(a[5]), ktype=cv2.CV_64F)
    return enc_fmat(g)


@op("integral")
def _(a):
    return enc_fmat(cv2.integral(gray(a[0])).astype(np.float64))


@op("integralSquared")
def _(a):
    s, sq = cv2.integral2(gray(a[0]))
    return enc_fmat(sq.astype(np.float64))


@op("distanceTransform")
def _(a):
    d = cv2.distanceTransform(gray(a[0]), cv2.DIST_L2, cv2.DIST_MASK_PRECISE)
    return enc_fmat(d.astype(np.float64))


@op("floodFill")
def _(a):
    m, sx, sy, newval, lo, up = a[0].copy(), int(a[1]), int(a[2]), float(a[3]), float(a[4]), float(a[5])
    count, _, _, rect = cv2.floodFill(m, None, (sx, sy), newval, loDiff=lo, upDiff=up)
    return {"count": int(count), "dst": enc_mat(m)}


# ---- morphology ---------------------------------------------------------


@op("getStructuringElement")
def _(a):
    el = cv2.getStructuringElement(pick(MORPH_SHAPE, a[0]), (int(a[2]), int(a[1])))
    return enc_mat(el)


@op("erode")
def _(a):
    el = cv2.getStructuringElement(pick(MORPH_SHAPE, a[1]), (int(a[3]), int(a[2])))
    return enc_mat(cv2.erode(a[0], el, iterations=int(a[4]),
                            borderType=cv2.BORDER_REPLICATE))


@op("dilate")
def _(a):
    el = cv2.getStructuringElement(pick(MORPH_SHAPE, a[1]), (int(a[3]), int(a[2])))
    return enc_mat(cv2.dilate(a[0], el, iterations=int(a[4]),
                             borderType=cv2.BORDER_REPLICATE))


@op("morphologyEx")
def _(a):
    el = cv2.getStructuringElement(pick(MORPH_SHAPE, a[2]), (int(a[4]), int(a[3])))
    return enc_mat(cv2.morphologyEx(a[0], pick(MORPH_OP, a[1]), el,
                                   iterations=int(a[5]),
                                   borderType=cv2.BORDER_REPLICATE))


# ---- geometric ----------------------------------------------------------


def pts2f(v):
    return np.array([[float(p[0]), float(p[1])] for p in v], np.float32)


@op("getRotationMatrix2D")
def _(a):
    m = cv2.getRotationMatrix2D((float(a[0]), float(a[1])), float(a[2]), float(a[3]))
    return enc_floats(m.reshape(-1))


@op("getAffineTransform")
def _(a):
    m = cv2.getAffineTransform(pts2f(a[0]), pts2f(a[1]))
    return enc_floats(m.reshape(-1))


@op("invertAffineTransform")
def _(a):
    m = np.array([float(v) for v in a[0]], np.float64).reshape(2, 3)
    return enc_floats(cv2.invertAffineTransform(m).reshape(-1))


@op("getPerspectiveTransform")
def _(a):
    m = cv2.getPerspectiveTransform(pts2f(a[0]), pts2f(a[1]))
    return enc_floats(m.reshape(-1))


@op("perspectiveTransform")
def _(a):
    pts = pts2f(a[0]).reshape(-1, 1, 2).astype(np.float64)
    m = np.array([float(v) for v in a[1]], np.float64).reshape(3, 3)
    out = cv2.perspectiveTransform(pts, m).reshape(-1, 2)
    return [enc_floats(p) for p in out]


@op("warpAffine")
def _(a):
    m = np.array([float(v) for v in a[1]], np.float64).reshape(2, 3)
    out = cv2.warpAffine(a[0], m, (int(a[2]), int(a[3])), flags=pick(INTERP, a[4]),
                         borderMode=cv2.BORDER_CONSTANT, borderValue=0)
    return enc_mat(out)


@op("warpPerspective")
def _(a):
    m = np.array([float(v) for v in a[1]], np.float64).reshape(3, 3)
    out = cv2.warpPerspective(a[0], m, (int(a[2]), int(a[3])), flags=pick(INTERP, a[4]),
                              borderMode=cv2.BORDER_CONSTANT, borderValue=0)
    return enc_mat(out)


@op("remap")
def _(a):
    src, mx, my, interp = a[0], a[1], a[2], pick(INTERP, a[3])
    out = cv2.remap(src, mx.astype(np.float32), my.astype(np.float32), interp,
                    borderMode=cv2.BORDER_CONSTANT, borderValue=0)
    return enc_mat(out)


@op("resize")
def _(a):
    return enc_mat(cv2.resize(a[0], (int(a[1]), int(a[2])), interpolation=pick(INTERP, a[3])))


@op("getRectSubPix")
def _(a):
    out = cv2.getRectSubPix(a[0], (int(a[1]), int(a[2])), (float(a[3]), float(a[4])))
    return enc_mat(out)


@op("warpPolar")
def _(a):
    src, w, h, cx, cy, maxr, mode = a[0], int(a[1]), int(a[2]), float(a[3]), float(a[4]), float(a[5]), a[6]
    flags = cv2.INTER_LINEAR | cv2.WARP_FILL_OUTLIERS
    if mode == "WarpPolarLog":
        flags |= cv2.WARP_POLAR_LOG
    return enc_mat(cv2.warpPolar(src, (w, h), (cx, cy), maxr, flags))


@op("linearPolar")
def _(a):
    src, w, h, cx, cy, maxr = a[0], int(a[1]), int(a[2]), float(a[3]), float(a[4]), float(a[5])
    return enc_mat(cv2.warpPolar(src, (w, h), (cx, cy), maxr,
                                cv2.INTER_LINEAR | cv2.WARP_FILL_OUTLIERS))


@op("logPolar")
def _(a):
    src, w, h, cx, cy, maxr = a[0], int(a[1]), int(a[2]), float(a[3]), float(a[4]), float(a[5])
    return enc_mat(cv2.warpPolar(src, (w, h), (cx, cy), maxr,
                                cv2.INTER_LINEAR | cv2.WARP_FILL_OUTLIERS | cv2.WARP_POLAR_LOG))


# ---- contours -----------------------------------------------------------


def cnt(v):
    return np.array([[int(p[0]), int(p[1])] for p in v], np.int32).reshape(-1, 1, 2)


@op("findContours")
def _(a):
    img = gray(a[0])
    cnts, hier = cv2.findContours(img.copy(), pick(RETR, a[1]), pick(CHAIN, a[2]))
    h = [] if hier is None else hier.reshape(-1, 4).tolist()
    with_parent = sum(1 for row in h if row[3] >= 0)
    return {"count": len(cnts), "hierLen": len(h),
            "withParent": int(with_parent), "topLevel": len(h) - int(with_parent),
            "contours": enc_contours(cnts)}


@op("contourArea")
def _(a):
    return enc_num(cv2.contourArea(cnt(a[0])))


@op("arcLength")
def _(a):
    return enc_num(cv2.arcLength(cnt(a[0]), bool(a[1])))


@op("approxPolyDP")
def _(a):
    out = cv2.approxPolyDP(cnt(a[0]), float(a[1]), bool(a[2]))
    return pts_list(out)


@op("convexHull")
def _(a):
    out = cv2.convexHull(cnt(a[0]))
    return sorted(pts_list(out))


@op("isContourConvex")
def _(a):
    return bool(cv2.isContourConvex(cnt(a[0])))


@op("boundingRect")
def _(a):
    x, y, w, h = cv2.boundingRect(cnt(a[0]))
    return [int(x), int(y), int(w), int(h)]


@op("minAreaRect")
def _(a):
    (cx, cy), (w, h), ang = cv2.minAreaRect(cnt(a[0]))
    # Angle/side conventions differ freely between equivalent parameterisations
    # of the same rectangle, so only the size-independent invariants are
    # compared: centre, sorted side lengths and area.
    sides = sorted([round(float(w), 3), round(float(h), 3)])
    return {"cx": enc_num(round(float(cx), 3)), "cy": enc_num(round(float(cy), 3)),
            "sides": enc_floats(sides), "area": enc_num(round(float(w) * float(h), 3))}


@op("rectVsMinArea")
def _(a):
    """The port's BoundingRect and MinAreaRect are reported to disagree on
    inclusive vs exclusive extents.  Both extents are emitted side by side."""
    c = cnt(a[0])
    x, y, w, h = cv2.boundingRect(c)
    (_, _), (mw, mh), _ = cv2.minAreaRect(c)
    return {"rectW": int(w), "rectH": int(h),
            "minW": enc_num(round(float(mw), 3)), "minH": enc_num(round(float(mh), 3)),
            "sameW": bool(abs(float(mw) - w) < 1e-6 or abs(float(mh) - w) < 1e-6)}


@op("boxPoints")
def _(a):
    r = ((float(a[0]), float(a[1])), (float(a[2]), float(a[3])), float(a[4]))
    pts = cv2.boxPoints(r)
    out = sorted([[round(float(p[0]), 3), round(float(p[1]), 3)] for p in pts])
    return [enc_floats(p) for p in out]


@op("minEnclosingCircle")
def _(a):
    (cx, cy), r = cv2.minEnclosingCircle(cnt(a[0]))
    return {"cx": enc_num(round(float(cx), 2)), "cy": enc_num(round(float(cy), 2)),
            "r": enc_num(round(float(r), 2))}


@op("pointPolygonTest")
def _(a):
    return enc_num(cv2.pointPolygonTest(cnt(a[0]), (float(a[1]), float(a[2])), bool(a[3])))


@op("fitLine")
def _(a):
    v = cv2.fitLine(cnt(a[0]).astype(np.float32), cv2.DIST_L2, 0, 0.01, 0.01).reshape(-1)
    # Direction sign is arbitrary; normalise so vx >= 0 (then vy >= 0).
    vx, vy, x0, y0 = [float(t) for t in v]
    if vx < 0 or (vx == 0 and vy < 0):
        vx, vy = -vx, -vy
    return enc_floats([vx, vy, x0, y0])


@op("moments")
def _(a):
    m = cv2.moments(gray(a[0]))
    keys = ["m00", "m10", "m01", "m20", "m11", "m02", "m30", "m21", "m12", "m03",
            "mu20", "mu11", "mu02", "mu30", "mu21", "mu12", "mu03",
            "nu20", "nu11", "nu02", "nu30", "nu21", "nu12", "nu03"]
    return {k: enc_num(m[k]) for k in keys}


@op("huMoments")
def _(a):
    m = cv2.moments(gray(a[0]))
    return enc_floats(cv2.HuMoments(m).reshape(-1))


@op("centroid")
def _(a):
    m = cv2.moments(gray(a[0]))
    if m["m00"] == 0:
        return enc_floats([0.0, 0.0])
    return enc_floats([m["m10"] / m["m00"], m["m01"] / m["m00"]])


@op("matchShapes")
def _(a):
    return enc_num(cv2.matchShapes(gray(a[0]), gray(a[1]), cv2.CONTOURS_MATCH_I1, 0))


@op("ellipse2Poly")
def _(a):
    pts = cv2.ellipse2Poly((int(a[0]), int(a[1])), (int(a[2]), int(a[3])),
                           int(a[4]), int(a[5]), int(a[6]), int(a[7]))
    return pts_list(pts)


@op("clipLine")
def _(a):
    rect = (int(a[0]), int(a[1]), int(a[2]), int(a[3]))
    ok, p1, p2 = cv2.clipLine(rect, (int(a[4]), int(a[5])), (int(a[6]), int(a[7])))
    return {"ok": bool(ok), "p1": [int(p1[0]), int(p1[1])], "p2": [int(p2[0]), int(p2[1])]}


@op("lineIterator")
def _(a):
    x0, y0, x1, y1 = int(a[0]), int(a[1]), int(a[2]), int(a[3])
    # cv2.LineIterator is not exposed to Python; Bresenham (8-connected, the
    # connectivity cv::LineIterator defaults to) is spelled out instead.
    pts = []
    dx, dy = abs(x1 - x0), abs(y1 - y0)
    sx = 1 if x1 >= x0 else -1
    sy = 1 if y1 >= y0 else -1
    err = dx - dy
    x, y = x0, y0
    while True:
        pts.append([x, y])
        if x == x1 and y == y1:
            break
        e2 = 2 * err
        if e2 > -dy:
            err -= dy
            x += sx
        if e2 < dx:
            err += dx
            y += sy
    return {"count": len(pts), "points": pts}


# ---- connected components ----------------------------------------------


def _relabel(labels):
    """Component ids are arbitrary; canonicalise by first appearance in
    row-major order so only the partition is compared."""
    mapping = {0: 0}
    out = []
    nxt = 1
    for v in labels.reshape(-1):
        v = int(v)
        if v not in mapping:
            mapping[v] = nxt
            nxt += 1
        out.append(mapping[v])
    return out


@op("connectedComponents")
def _(a):
    conn = 4 if a[1] == "Connectivity4" else 8
    n, labels = cv2.connectedComponents(gray(a[0]), connectivity=conn)
    return {"count": int(n), "labels": _relabel(labels)}


@op("connectedComponentsWithStats")
def _(a):
    conn = 4 if a[1] == "Connectivity4" else 8
    n, labels, stats, cent = cv2.connectedComponentsWithStats(gray(a[0]), connectivity=conn)
    rows = []
    for i in range(n):
        rows.append({"area": int(stats[i, cv2.CC_STAT_AREA]),
                     "rect": [int(stats[i, cv2.CC_STAT_LEFT]), int(stats[i, cv2.CC_STAT_TOP]),
                              int(stats[i, cv2.CC_STAT_WIDTH]), int(stats[i, cv2.CC_STAT_HEIGHT])],
                     "cx": enc_num(round(float(cent[i][0]), 4)),
                     "cy": enc_num(round(float(cent[i][1]), 4))})
    body = sorted(rows[1:], key=lambda d: (d["area"], d["rect"]))
    return {"count": int(n), "labels": _relabel(labels),
            "background": rows[0], "components": body}


# ---- Hough --------------------------------------------------------------


@op("houghLines")
def _(a):
    lines = cv2.HoughLines(gray(a[0]), float(a[1]), float(a[2]), int(a[3]))
    out = []
    if lines is not None:
        for l in lines.reshape(-1, 2):
            out.append([round(float(l[0]), 2), round(float(l[1]), 4)])
    out.sort()
    return {"count": len(out), "lines": [enc_floats(p) for p in out]}


@op("houghLinesP")
def _(a):
    lines = cv2.HoughLinesP(gray(a[0]), float(a[1]), float(a[2]), int(a[3]),
                            minLineLength=int(a[4]), maxLineGap=int(a[5]))
    out = []
    if lines is not None:
        for l in lines.reshape(-1, 4):
            out.append([int(l[0]), int(l[1]), int(l[2]), int(l[3])])
    out.sort()
    return {"count": len(out), "segments": out}


@op("houghCircles")
def _(a):
    src, mindist, p1, p2, minr, maxr = gray(a[0]), float(a[1]), float(a[2]), float(a[3]), int(a[4]), int(a[5])
    circles = cv2.HoughCircles(src, cv2.HOUGH_GRADIENT, 1, mindist,
                               param1=p1, param2=p2, minRadius=minr, maxRadius=maxr)
    out = []
    if circles is not None:
        for c in circles.reshape(-1, 3):
            out.append([int(round(float(c[0]))), int(round(float(c[1]))), int(round(float(c[2])))])
    out.sort()
    return {"count": len(out), "circles": out}


@op("fastCorners")
def _(a):
    det = cv2.FastFeatureDetector_create(threshold=int(a[1]), nonmaxSuppression=bool(a[2]))
    kps = det.detect(gray(a[0]), None)
    pts = sorted([[int(round(k.pt[0])), int(round(k.pt[1]))] for k in kps])
    return {"count": len(pts), "points": pts}


@op("goodFeaturesToTrack")
def _(a):
    src, maxc, q, mind, block = gray(a[0]), int(a[1]), float(a[2]), float(a[3]), int(a[4])
    c = cv2.goodFeaturesToTrack(src, maxc, q, mind, blockSize=block)
    pts = [] if c is None else sorted([[int(round(p[0])), int(round(p[1]))] for p in c.reshape(-1, 2)])
    return {"count": len(pts), "points": pts}


# ---- template matching --------------------------------------------------


@op("matchTemplate")
def _(a):
    src, templ, mode = a[0], a[1], pick(TM, a[2])
    return enc_fmat(cv2.matchTemplate(src, templ, mode).astype(np.float64))


@op("matchTemplateLoc")
def _(a):
    src, templ, mode = a[0], a[1], pick(TM, a[2])
    res = cv2.matchTemplate(src, templ, mode)
    mn, mx, mnl, mxl = cv2.minMaxLoc(res)
    return {"minLoc": [int(mnl[0]), int(mnl[1])], "maxLoc": [int(mxl[0]), int(mxl[1])],
            "rows": int(res.shape[0]), "cols": int(res.shape[1])}


@op("minMaxLoc")
def _(a):
    f = a[0]
    mn, mx, mnl, mxl = cv2.minMaxLoc(f.astype(np.float32))
    return {"min": enc_num(mn), "max": enc_num(mx),
            "minLoc": [int(mnl[0]), int(mnl[1])], "maxLoc": [int(mxl[0]), int(mxl[1])]}


@op("minMaxLocMat")
def _(a):
    m = gray(a[0])
    mn, mx, mnl, mxl = cv2.minMaxLoc(m)
    return {"min": enc_num(mn), "max": enc_num(mx),
            "minLoc": [int(mnl[0]), int(mnl[1])], "maxLoc": [int(mxl[0]), int(mxl[1])]}


# ---- histograms ---------------------------------------------------------


@op("calcHist")
def _(a):
    m, chan = a[0], int(a[1])
    h = cv2.calcHist([m], [chan], None, [256], [0, 256])
    return enc_ints(h.reshape(-1).astype(np.int64))


@op("compareHist")
def _(a):
    h1 = np.array([float(v) for v in a[0]], np.float32)
    h2 = np.array([float(v) for v in a[1]], np.float32)
    return enc_num(cv2.compareHist(h1, h2, pick(HISTCMP, a[2])))


@op("compareHistOfImages")
def _(a):
    h1 = cv2.calcHist([a[0]], [0], None, [256], [0, 256])
    h2 = cv2.calcHist([a[1]], [0], None, [256], [0, 256])
    return enc_num(cv2.compareHist(h1, h2, pick(HISTCMP, a[2])))


@op("equalizeHist")
def _(a):
    return enc_mat(cv2.equalizeHist(gray(a[0])))


@op("clahe")
def _(a):
    c = cv2.createCLAHE(clipLimit=float(a[1]), tileGridSize=(int(a[2]), int(a[2])))
    return enc_mat(c.apply(gray(a[0])))


@op("calcBackProject")
def _(a):
    m, chan, hist = a[0], int(a[1]), a[2]
    h = np.array([float(v) for v in hist], np.float32).reshape(256, 1)
    return enc_mat(cv2.calcBackProject([m], [chan], h, [0, 256], 1))


@op("entropy")
def _(a):
    img = gray(a[0])
    hist = np.bincount(img.reshape(-1), minlength=256).astype(np.float64)
    p = hist / float(img.size)
    p = p[p > 0]
    return enc_num(float(-(p * np.log2(p)).sum()))


@op("medianValue")
def _(a):
    return enc_num(float(np.median(gray(a[0]).astype(np.float64))))


# ---- linear algebra / spectra ------------------------------------------


@op("determinant")
def _(a):
    m = np.array([float(v) for v in a[0]], np.float64).reshape(int(a[1]), int(a[1]))
    return enc_num(cv2.determinant(m))


@op("invert")
def _(a):
    n = int(a[1])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    ret, inv = cv2.invert(m, flags=cv2.DECOMP_LU)
    return {"ok": bool(ret != 0), "m": enc_floats(inv.reshape(-1))}


@op("solve")
def _(a):
    n = int(a[2])
    A = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    B = np.array([float(v) for v in a[1]], np.float64).reshape(n, 1)
    ok, x = cv2.solve(A, B, flags=cv2.DECOMP_LU)
    return {"ok": bool(ok), "x": enc_floats(np.asarray(x).reshape(-1))}


@op("trace")
def _(a):
    n = int(a[1])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    return enc_num(float(cv2.trace(m)[0]))


@op("gemm")
def _(a):
    ar, ac = int(a[1]), int(a[2])
    br, bc = int(a[4]), int(a[5])
    A = np.array([float(v) for v in a[0]], np.float64).reshape(ar, ac)
    B = np.array([float(v) for v in a[3]], np.float64).reshape(br, bc)
    out = cv2.gemm(A, B, float(a[6]), None, 0.0)
    return enc_fmat(out)


@op("dct")
def _(a):
    n = int(a[1])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    return enc_fmat(cv2.dct(m))


@op("dftMagnitude")
def _(a):
    n = int(a[1])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    sp = cv2.dft(m, flags=cv2.DFT_COMPLEX_OUTPUT)
    mag = cv2.magnitude(sp[:, :, 0], sp[:, :, 1])
    return enc_fmat(mag)


@op("cartToPolar")
def _(a):
    n = int(a[2])
    x = np.array([float(v) for v in a[0]], np.float64).reshape(n, n)
    y = np.array([float(v) for v in a[1]], np.float64).reshape(n, n)
    mag, ang = cv2.cartToPolar(x, y, angleInDegrees=bool(a[3]))
    return {"mag": enc_fmat(mag), "ang": enc_fmat(ang)}


@op("setIdentity")
def _(a):
    r, c, v = int(a[0]), int(a[1]), float(a[2])
    m = np.zeros((r, c), np.float64)
    cv2.setIdentity(m, v)
    return enc_fmat(m)


@op("reduce")
def _(a):
    r, c = int(a[1]), int(a[2])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(r, c)
    dim = 0 if bool(a[3]) else 1
    out = cv2.reduce(m, dim, pick(REDUCE, a[4]), dtype=cv2.CV_64F)
    return enc_fmat(out)


@op("repeatMat")
def _(a):
    r, c = int(a[1]), int(a[2])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(r, c)
    return enc_fmat(cv2.repeat(m, int(a[3]), int(a[4])))


@op("sortMat")
def _(a):
    r, c = int(a[1]), int(a[2])
    m = np.array([float(v) for v in a[0]], np.float64).reshape(r, c)
    flags = (cv2.SORT_EVERY_ROW if bool(a[3]) else cv2.SORT_EVERY_COLUMN)
    flags |= (cv2.SORT_DESCENDING if bool(a[4]) else cv2.SORT_ASCENDING)
    return enc_fmat(cv2.sort(m, flags))


@op("transformMat")
def _(a):
    src, rows, cols, data = a[0], int(a[1]), int(a[2]), a[3]
    m = np.array([float(v) for v in data], np.float64).reshape(rows, cols)
    return enc_mat(cv2.transform(src, m))


# ---- quality metrics ---------------------------------------------------


@op("psnr")
def _(a):
    return enc_num(cv2.PSNR(a[0], a[1]))


@op("mseMat")
def _(a):
    d = a[0].astype(np.float64) - a[1].astype(np.float64)
    return enc_num(float((d * d).mean()))


@op("laplacianVariance")
def _(a):
    lap = cv2.Laplacian(gray(a[0]).astype(np.float64), cv2.CV_64F, ksize=3,
                        borderType=REPL)
    return enc_num(float(lap.var()))


@op("sharpnessIsLaplacianVariance")
def _(a):
    """The port's quality.Sharpness and quality.LaplacianVariance return the
    identical number.  Upstream has two genuinely different measures:
    Laplacian variance and Tenengrad (mean squared Sobel magnitude), so the
    answer here is False."""
    g = gray(a[0]).astype(np.float64)
    lap = cv2.Laplacian(g, cv2.CV_64F, ksize=3, borderType=REPL)
    lapvar = float(lap.var())
    gx = cv2.Sobel(g, cv2.CV_64F, 1, 0, ksize=3, borderType=REPL)
    gy = cv2.Sobel(g, cv2.CV_64F, 0, 1, ksize=3, borderType=REPL)
    tenengrad = float((gx * gx + gy * gy).mean())
    return {"equal": bool(abs(lapvar - tenengrad) < 1e-9)}


# ---- colormaps (plot package) ------------------------------------------


def _cm_table(name):
    code = pick(COLORMAP, name)
    ramp = np.arange(256, dtype=np.uint8).reshape(256, 1)
    if code is None:  # ColormapGrayscale: the documented identity ramp.
        return [[int(i), int(i), int(i)] for i in range(256)]
    bgr = cv2.applyColorMap(ramp, code).reshape(256, 3)
    # cv2 emits BGR; the port's tables are RGB triples.
    return [[int(p[2]), int(p[1]), int(p[0])] for p in bgr]


@op("colormapTable")
def _(a):
    return _cm_table(a[0])


@op("applyColorMapShape")
def _(a):
    """Shape/type only: isolates the port's panic on unrecognised colormaps
    from any palette difference."""
    _cm_table(a[1])  # raises for an unknown name
    g = gray(a[0])
    return {"rows": int(g.shape[0]), "cols": int(g.shape[1]), "channels": 3}


@op("applyColorMap")
def _(a):
    g = gray(a[0])
    table = np.array(_cm_table(a[1]), np.uint8)
    out = np.zeros((g.shape[0], g.shape[1], 3), np.uint8)
    for c in range(3):
        out[:, :, c] = table[:, c][g]
    return enc_mat(out)


@op("applyCustomColorMap")
def _(a):
    g = gray(a[0])
    table = np.array([[int(v) for v in row] for row in a[1]], np.uint8)
    out = np.zeros((g.shape[0], g.shape[1], 3), np.uint8)
    for c in range(3):
        out[:, :, c] = table[:, c][g]
    return enc_mat(out)


# plot.Table/plot.Colorize are the port's general-purpose entry points and
# plot.ColormapTable/plot.ApplyColorMap the restricted ones; upstream has a
# single implementation, so these alias the same op.
OPS["plotTable"] = OPS["colormapTable"]
OPS["colorize"] = OPS["applyColorMap"]
OPS["colorizeShape"] = OPS["applyColorMapShape"]


# ---- segmentation ------------------------------------------------------


@op("grabCut")
def _(a):
    """`maskMode` selects how the mask argument is passed: "zeros" hands in a
    freshly allocated all-zero mask (the port reads that as "everything is
    background"), "none" hands in no mask at all."""
    img, rect, iters, mask_mode = a[0], a[1], int(a[2]), a[3]
    if img.ndim != 3:
        raise ValueError("grabCut wants a colour image")
    mask = np.zeros(img.shape[:2], np.uint8)
    bgd = np.zeros((1, 65), np.float64)
    fgd = np.zeros((1, 65), np.float64)
    r = (int(rect[0]), int(rect[1]), int(rect[2]), int(rect[3]))
    cv2.grabCut(img, mask, r, bgd, fgd, iters, cv2.GC_INIT_WITH_RECT)
    fg = int(((mask == cv2.GC_FGD) | (mask == cv2.GC_PR_FGD)).sum())
    return {"foreground": fg, "background": int(mask.size - fg),
            "allBackground": bool(fg == 0)}


@op("watershedRegions")
def _(a):
    img = a[0]
    if img.ndim != 3:
        raise ValueError("watershed wants a colour image")
    markers = np.asarray(a[1], np.int32)
    m = markers.copy()
    cv2.watershed(img, m)
    labels = sorted(set(int(v) for v in m.reshape(-1)))
    return {"labels": labels, "regions": len(labels)}


# ------------------------------------------------------------------ driver


def handle(req):
    fn = req.get("fn")
    if fn not in OPS:
        raise ValueError("unknown op: %r" % (fn,))
    args = [dec_arg(v) for v in req.get("args", [])]
    return OPS[fn](args)


def main():
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except Exception as exc:  # malformed request: still answer, in order
            out.write(json.dumps({"id": "", "ok": False,
                                  "error": "bad request: %s" % exc}) + "\n")
            out.flush()
            continue
        cid = req.get("id", "")
        try:
            val = handle(req)
            resp = {"id": cid, "ok": True, "value": val}
        except BaseException as exc:  # never exit on a failing case
            resp = {"id": cid, "ok": False,
                    "error": "%s: %s" % (type(exc).__name__, exc)}
        out.write(json.dumps(resp) + "\n")
        out.flush()


if __name__ == "__main__":
    main()
