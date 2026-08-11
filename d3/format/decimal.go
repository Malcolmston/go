package format

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// This file holds the numeric primitives that everything else in the package is
// built on. It exists because d3-format is defined in terms of JavaScript's
// Number.prototype.toFixed, toExponential, toPrecision and toString, and those
// four functions do not have exact equivalents in Go's strconv.
//
// The differences are small but they are exactly the differences that show up in
// axis tick labels, so they are worth spelling out:
//
//   - Rounding ties. ECMA-262 defines toFixed/toExponential/toPrecision as
//     "pick the integer n such that n/10^f - x is closest to zero; if two are
//     equally close, pick the larger n" — that is, round half up (and since
//     d3 always formats math.Abs(value), half away from zero). Go's strconv
//     correctly rounds the exact binary value but breaks ties to even, so
//     strconv.FormatFloat(2.5, 'f', 0, 64) is "2" while (2.5).toFixed(0) is
//     "3". Ties only occur when the float64 is *exactly* a half at the target
//     digit, which is rare but perfectly reachable (0.5, 2.5, 1.005 is not one
//     — its binary value is below the half). To match d3 exactly we do the
//     rounding ourselves on the exact value of the float64, obtained as a
//     [math/big.Rat]. That is slower than strconv but exact, and tick labels
//     are not a hot path.
//
//   - Significant digits versus decimal places. Go's %g trims trailing zeros
//     and picks its own exponent threshold; JavaScript's toPrecision keeps
//     trailing zeros ((1).toPrecision(3) is "1.00") and switches to exponential
//     notation only when the decimal exponent is < -6 or >= the precision.
//     d3's "g" type is toPrecision, and its "r" type is a *fixed-notation*
//     rounding to significant digits with no exponential escape at all. Both
//     are built here from [sigDigits].
//
//   - Number.toString. The "c" and "d" types stringify through JavaScript's
//     default number-to-string algorithm, which uses the shortest digit string
//     that round-trips and only switches to exponential notation outside
//     10^-7 .. 10^21. [jsString] reproduces it.
//
// Everything in this file assumes a non-negative input, because [Locale.Compile]
// takes math.Abs of the value before dispatching and carries the sign itself.

// pow10Rat returns 10^k as an exact rational, for positive or negative k.
func pow10Rat(k int) *big.Rat {
	n := k
	if n < 0 {
		n = -n
	}
	p := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
	if k >= 0 {
		return new(big.Rat).SetInt(p)
	}
	return new(big.Rat).SetFrac(big.NewInt(1), p)
}

// ratRoundHalfUp rounds a non-negative rational to the nearest integer, with
// halves rounded up. This is the tie rule ECMA-262 specifies for toFixed and
// friends, applied to the exact value of the float64 rather than to its
// shortest decimal representation.
func ratRoundHalfUp(r *big.Rat) *big.Int {
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(r.Num(), r.Denom(), rem)
	rem.Lsh(rem, 1) // 2*rem >= denom  <=>  fractional part >= 1/2
	if rem.Cmp(r.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	return q
}

// sigDigits returns p significant decimal digits of x (which must be finite and
// non-negative) together with the decimal exponent e, such that
//
//	x ≈ 0.D × 10^(e+1) = D × 10^(e-p+1)
//
// where D is the returned digit string. It is the engine behind toExponential,
// toPrecision and d3's "r" and "s" types: 1234.5 with p=3 yields ("123", 3).
//
// The exponent is found by an estimate from math.Log10 and then corrected by
// exact comparison, because Log10 is off by one near powers of ten.
func sigDigits(x float64, p int) (string, int) {
	if p < 1 {
		p = 1
	}
	if x == 0 {
		return strings.Repeat("0", p), 0
	}
	r := new(big.Rat).SetFloat64(x)
	e := int(math.Floor(math.Log10(x)))
	for r.Cmp(pow10Rat(e)) < 0 {
		e--
	}
	for r.Cmp(pow10Rat(e+1)) >= 0 {
		e++
	}
	scaled := new(big.Rat).Mul(r, pow10Rat(p-1-e))
	s := ratRoundHalfUp(scaled).String()
	if len(s) > p {
		// Rounding carried into a new decade, e.g. 9.99 to 2 digits is 10.0.
		s = s[:p]
		e++
	}
	return s, e
}

// shortestDigits returns the shortest digit string that round-trips to x
// together with its decimal exponent — the digits JavaScript's Number.toString
// would produce. It is what d3's formatDecimalParts computes when no precision
// is given.
func shortestDigits(x float64) (string, int) {
	s := strconv.FormatFloat(x, 'e', -1, 64) // e.g. "1.2345e+20", "0e+00"
	i := strings.IndexByte(s, 'e')
	mant, exp := s[:i], s[i+1:]
	if j := strings.IndexByte(mant, '.'); j >= 0 {
		mant = mant[:j] + mant[j+1:]
	}
	e, _ := strconv.Atoi(exp)
	return mant, e
}

// decimalParts is the Go form of d3's formatDecimalParts. A p of zero or less
// means "as many digits as needed"; ok is false for NaN and ±Inf, which the
// callers stringify instead.
func decimalParts(x float64, p int) (digits string, exp int, ok bool) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "", 0, false
	}
	if p > 0 {
		d, e := sigDigits(x, p)
		return d, e, true
	}
	d, e := shortestDigits(x)
	return d, e, true
}

// fixedFromDigits renders a digit string and decimal exponent in plain
// positional notation, never using an exponent. This is d3's formatRounded
// tail, and also the non-exponential branch of toPrecision.
func fixedFromDigits(digits string, e int) string {
	switch {
	case e < 0:
		return "0." + strings.Repeat("0", -e-1) + digits
	case len(digits) > e+1:
		return digits[:e+1] + "." + digits[e+1:]
	default:
		return digits + strings.Repeat("0", e-len(digits)+1)
	}
}

// jsString reproduces JavaScript's Number-to-String conversion: the shortest
// round-tripping digits, in positional notation while the decimal exponent is
// in (-7, 21], and in exponential notation ("1e+21") outside that window.
func jsString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	sign := ""
	if x < 0 || (x == 0 && math.Signbit(x)) {
		// JavaScript prints "0" for -0, but "-1" for -1.
		if x != 0 {
			sign = "-"
		}
		x = math.Abs(x)
	}
	s, e := shortestDigits(x)
	k := len(s)
	n := e + 1 // value = 0.s × 10^n
	switch {
	case k <= n && n <= 21:
		return sign + s + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + s[:n] + "." + s[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + s
	}
	esign := "+"
	if e < 0 {
		esign = "-"
	}
	ea := e
	if ea < 0 {
		ea = -ea
	}
	mant := s
	if k > 1 {
		mant = s[:1] + "." + s[1:]
	}
	return sign + mant + "e" + esign + strconv.Itoa(ea)
}

// jsToFixed is Number.prototype.toFixed: p digits after the decimal point, with
// halves rounded away from zero on the exact binary value. Above 10^21
// JavaScript falls back to Number.toString, and so do we.
func jsToFixed(x float64, p int) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return jsString(x)
	}
	if math.Abs(x) >= 1e21 {
		return jsString(x)
	}
	return new(big.Rat).SetFloat64(x).FloatString(p)
}

// jsToExponential is Number.prototype.toExponential: one digit before the point
// and p after, with the exponent written as "e+3"/"e-3" and never zero-padded.
func jsToExponential(x float64, p int) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return jsString(x)
	}
	d, e := sigDigits(math.Abs(x), p+1)
	sign := ""
	if x < 0 {
		sign = "-"
	}
	mant := d
	if len(d) > 1 {
		mant = d[:1] + "." + d[1:]
	}
	esign := "+"
	if e < 0 {
		esign = "-"
		e = -e
	}
	return sign + mant + "e" + esign + strconv.Itoa(e)
}

// jsToPrecision is Number.prototype.toPrecision: p significant digits, keeping
// trailing zeros, switching to exponential notation when the decimal exponent
// is below -6 or at least p.
func jsToPrecision(x float64, p int) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return jsString(x)
	}
	if p < 1 {
		p = 1
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	d, e := sigDigits(x, p)
	if e < -6 || e >= p {
		mant := d
		if len(d) > 1 {
			mant = d[:1] + "." + d[1:]
		}
		esign := "+"
		if e < 0 {
			esign = "-"
			e = -e
		}
		return sign + mant + "e" + esign + strconv.Itoa(e)
	}
	return sign + fixedFromDigits(d, e)
}

// formatRounded is d3's "r" type: p significant digits in positional notation,
// with no exponential escape however large or small the value. .3r of 12345 is
// "12300" and .3r of 0.000012345 is "0.0000123".
func formatRounded(x float64, p int) string {
	d, e, ok := decimalParts(x, p)
	if !ok {
		return jsString(x)
	}
	return fixedFromDigits(d, e)
}

// siPrefixes are the SI unit prefixes from yocto (10^-24) to yotta (10^24),
// indexed by 8 + exponent/3. The empty string in the middle is 10^0.
var siPrefixes = [...]string{"y", "z", "a", "f", "p", "n", "µ", "m", "", "k", "M", "G", "T", "P", "E", "Z", "Y"}

// formatPrefixAuto is d3's "s" type. It rounds x to p significant digits and
// then shifts the decimal point to the nearest SI prefix, returning both the
// digit string and the exponent of the prefix that was chosen (a multiple of 3
// clamped to ±24). The caller appends siPrefixes[8+exp/3].
//
// The last branch handles values smaller than one yocto, where the prefix
// cannot absorb the whole exponent and the result needs leading zeros; d3
// re-rounds there with fewer significant digits so the digit count still
// reflects the requested precision.
func formatPrefixAuto(x float64, p int) (string, int) {
	d, e, ok := decimalParts(x, p)
	if !ok {
		return jsString(x), 0
	}
	pe := clampInt(int(math.Floor(float64(e)/3)), -8, 8) * 3
	i := e - pe + 1
	n := len(d)
	switch {
	case i == n:
		return d, pe
	case i > n:
		return d + strings.Repeat("0", i-n), pe
	case i > 0:
		return d[:i] + "." + d[i:], pe
	default:
		d2, _, _ := decimalParts(x, maxInt(0, p+i-1))
		return "0." + strings.Repeat("0", -i) + d2, pe
	}
}

// formatTrim removes insignificant trailing zeros — the "~" flag. It turns
// "1.2000k" into "1.2k" and "1.0000e+3" into "1e+3", but leaves "1200" alone,
// because those zeros are significant. It scans for the decimal point and the
// last run of zeros and splices them out, stopping at the first character that
// is neither a digit nor a point (the "e" of an exponent, or an SI prefix).
func formatTrim(s string) string {
	i0, i1 := -1, -1
	n := len(s)
loop:
	for i := 1; i < n; i++ {
		switch c := s[i]; {
		case c == '.':
			i0, i1 = i, i
		case c == '0':
			if i0 == 0 {
				i0 = i
			}
			i1 = i
		default:
			if c < '1' || c > '9' {
				break loop
			}
			if i0 > 0 {
				i0 = 0
			}
		}
	}
	if i0 > 0 {
		return s[:i0] + s[i1+1:]
	}
	return s
}

// formatRadix is the "b", "o", "x" and "X" types: round to an integer and
// render in the given base. big.Int keeps this honest for values beyond the
// range of int64, which JavaScript's toString(radix) also handles.
func formatRadix(x float64, base int, upper bool) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return jsString(x)
	}
	bi, _ := new(big.Float).SetFloat64(math.Round(x)).Int(nil)
	s := bi.Text(base)
	if upper {
		s = strings.ToUpper(s)
	}
	return s
}

// formatDecimal is the "d" type: round to an integer and render every digit,
// never in exponential notation.
func formatDecimal(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return jsString(x)
	}
	return strconv.FormatFloat(math.Round(x), 'f', -1, 64)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
