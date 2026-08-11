package react

import (
	"math"
	"strconv"
	"strings"
)

// jsNumberString formats a float the way JavaScript's String(number) —
// Number.prototype.toString with radix 10 — formats it. React never formats
// numbers itself: it interpolates them into markup as strings, so every numeric
// text child, numeric attribute value and numeric style value goes through the
// ECMAScript Number-to-String algorithm. Go's strconv comes close but differs
// in three visible ways, all of which show up in rendered HTML:
//
//   - FormatFloat(v, 'f', -1, 64) never switches to exponent form, so 1e21
//     renders as "1000000000000000000000" where JS renders "1e+21".
//   - FormatFloat(v, 'g', -1, 64) switches at the wrong thresholds and pads the
//     exponent to two digits, so 1e-7 renders as "1e-07" where JS renders "1e-7".
//   - Go prints negative zero as "-0"; JS prints "0".
//
// The implementation follows ECMA-262 Number::toString step by step: take the
// shortest round-tripping decimal digits, then decide between fixed and
// exponential notation from the position of the decimal point.
//
// bitSize is 64 for a float64 and 32 for a float32, so a float32 keeps the
// shortest digit string that round-trips at 32-bit precision (float32(0.1)
// stays "0.1" instead of becoming "0.10000000149011612").
func jsNumberString(v float64, bitSize int) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v == 0:
		// Covers negative zero, which JS renders without the sign.
		return "0"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}

	if v < 0 {
		return "-" + jsNumberString(-v, bitSize)
	}

	// FormatFloat with 'e' gives the shortest round-tripping digits in the form
	// d[.ddd]e±dd. Split it into the digit string and the decimal exponent.
	sci := strconv.FormatFloat(v, 'e', -1, bitSize)
	mantissa, expPart, _ := strings.Cut(sci, "e")
	exp, err := strconv.Atoi(expPart)
	if err != nil {
		// Unreachable for finite values; fall back to something readable rather
		// than emitting a broken document.
		return sci
	}
	digits := strings.Replace(mantissa, ".", "", 1)

	// In spec terms: the value is digits * 10^(n-k), where k is the digit count
	// and n is the position of the decimal point relative to the digit string.
	k := len(digits)
	n := exp + 1

	switch {
	case n >= k && n <= 21:
		// Integer with trailing zeros: 1e20 -> "100000000000000000000".
		return digits + strings.Repeat("0", n-k)
	case n > 0 && n <= 21:
		// Decimal point falls inside the digits: 1.5 -> "1.5".
		return digits[:n] + "." + digits[n:]
	case n > -6 && n <= 0:
		// Leading zeros after the point: 1e-6 -> "0.000001".
		return "0." + strings.Repeat("0", -n) + digits
	}

	// Exponential form. JS writes an explicit sign and no zero padding, so the
	// exponent is "e+21" and "e-7", never Go's "e+21"/"e-07".
	e := n - 1
	sign := "+"
	if e < 0 {
		sign = "-"
		e = -e
	}
	out := digits[:1]
	if k > 1 {
		out += "." + digits[1:]
	}
	return out + "e" + sign + strconv.Itoa(e)
}
