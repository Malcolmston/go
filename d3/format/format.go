// Package format is a Go port of d3-format — turning numbers into strings that
// humans read comfortably.
//
// The problem it solves is narrower than fmt's and much more specific: when you
// label an axis, or print a table of measurements, you want every label to agree
// on how many digits it shows, on where the thousands separators go, and on
// whether 1300000 is spelled "1,300,000" or "1.3M". Go's fmt verbs cannot say
// any of that in one token. d3's specifier can, and the specifier is a string,
// so it can be a configuration value, a URL parameter or a column definition:
//
//	f := format.MustNew(",.2f")
//	f(1234.5678) // "1,234.57"
//	format.MustNew("$,.2f")(1234.5678)  // "$1,234.57"
//	format.MustNew(".1%")(0.123)        // "12.3%"
//	format.MustNew(".3s")(1300000)      // "1.30M"
//
// The grammar is
//
//	[[fill]align][sign][symbol][0][width][,][.precision][~][type]
//
// and it is documented field by field on [Specifier]. The types are:
//
//	e  exponential notation, "1.234e+2"
//	f  fixed point, precision counts decimal places
//	g  either fixed or exponential, precision counts significant digits
//	r  fixed point, precision counts significant digits
//	s  fixed point with an SI prefix (k, M, G, µ, n, …)
//	%  multiply by 100 and append a percent sign, decimal places
//	p  multiply by 100 and append a percent sign, significant digits
//	b  binary, o octal, d decimal, x/X hexadecimal — all rounded to an integer
//	c  the number as-is, for pass-through
//	n  an alias for ",g"
//
// The distinction between decimal places and significant digits is the crux of
// the whole package: ".2f" of 3.14159 is "3.14", ".2r" is "3.1", and ".2r" of
// 12345 is "12000". Rounding is done on the exact binary value of the float64
// with halves rounded away from zero, matching JavaScript's toFixed rather than
// Go's strconv (which breaks ties to even); see decimal.go for why that matters.
//
// The "~" flag trims insignificant trailing zeros, which is what you usually
// want for ticks: ".3~s" prints 1300000 as "1.3M" rather than "1.30M", while
// still guaranteeing at most three significant digits.
//
// # Locales
//
// The decimal point, group separator, currency affixes, percent sign, minus
// sign and NaN spelling all live in a [Locale]. [NewLocale] fills in the
// missing pieces from the default en-US settings; package-level [New],
// [MustNew] and [FormatPrefix] use [EnUS].
//
// One deliberate divergence from d3: the default minus sign here is the ASCII
// hyphen-minus "-", whereas d3's built-in locale uses U+2212 MINUS SIGN. The
// typographic minus is the better character in rendered SVG, but the ASCII one
// round-trips through strconv.ParseFloat and through anything that later reads
// the output back. Set [Locale.Minus] to "−" for byte-identical parity.
//
// # Choosing a precision
//
// [PrecisionFixed], [PrecisionRound] and [PrecisionPrefix] compute the
// precision that a given tick step deserves, so that neighbouring labels differ
// without showing noise digits. They are what a scale's tickFormat is built
// from, and they pair with [FormatPrefix] when every label should share one SI
// prefix rather than each picking its own.
//
// The package is standard-library only and every returned formatting function
// is a pure value that is safe to share across goroutines.
package format

import (
	"math"
	"strconv"
	"strings"
)

// Locale carries the culture-specific parts of number formatting. The zero
// value is not usable directly; pass it through [NewLocale], which substitutes
// the en-US default for every empty field.
type Locale struct {
	// Decimal is the decimal point, "." by default.
	Decimal string
	// Thousands is the group separator, "," by default. Grouping is applied
	// only when the specifier carries the "," flag.
	Thousands string
	// Grouping lists the group sizes from the right, cycling on the last
	// entry: []int{3} gives 1,234,567 and []int{3, 2} gives 12,34,567 as
	// used in the Indian numbering system. Defaults to []int{3}.
	Grouping []int
	// Currency is the prefix and suffix applied by the "$" symbol, e.g.
	// {"$", ""} for dollars or {"", " €"} for euros.
	Currency [2]string
	// Numerals optionally maps the ten ASCII digits onto another digit set,
	// applied to the finished string. Must have length 10 when non-nil.
	Numerals []string
	// Percent is the suffix appended by the "%" and "p" types, "%" by default.
	Percent string
	// Minus is the negative sign, "-" by default (d3 uses U+2212).
	Minus string
	// NaN is the spelling of a not-a-number value, "NaN" by default.
	NaN string
}

// NewLocale returns a ready-to-use locale, filling in en-US defaults for the
// fields left empty. The returned value is independent of the argument.
func NewLocale(l Locale) *Locale {
	out := l
	if out.Decimal == "" {
		out.Decimal = "."
	}
	if out.Thousands == "" {
		out.Thousands = ","
	}
	if len(out.Grouping) == 0 {
		out.Grouping = []int{3}
	} else {
		out.Grouping = append([]int(nil), out.Grouping...)
	}
	if out.Currency[0] == "" && out.Currency[1] == "" {
		out.Currency = [2]string{"$", ""}
	}
	if out.Percent == "" {
		out.Percent = "%"
	}
	if out.Minus == "" {
		out.Minus = "-"
	}
	if out.NaN == "" {
		out.NaN = "NaN"
	}
	if out.Numerals != nil {
		out.Numerals = append([]string(nil), out.Numerals...)
	}
	return &out
}

// EnUS is the default locale: "." and ",", dollars, ASCII minus.
var EnUS = NewLocale(Locale{})

// New compiles a specifier into a formatting function. The error is non-nil
// only when the specifier does not match the grammar; unknown type letters are
// accepted and behave like the empty type (".12~g", i.e. up to twelve
// significant digits with trailing zeros trimmed).
func New(specifier string) (func(float64) string, error) { return EnUS.New(specifier) }

// MustNew is [New] for specifiers known at compile time; it panics on a
// malformed specifier.
func MustNew(specifier string) func(float64) string { return EnUS.MustNew(specifier) }

// New compiles a specifier against this locale.
func (l *Locale) New(specifier string) (func(float64) string, error) {
	s, err := ParseSpecifier(specifier)
	if err != nil {
		return nil, err
	}
	return l.Compile(s), nil
}

// MustNew is [Locale.New] but panics instead of returning an error.
func (l *Locale) MustNew(specifier string) func(float64) string {
	f, err := l.New(specifier)
	if err != nil {
		panic(err)
	}
	return f
}

// knownTypes lists the type letters that have a formatting function. Anything
// else — including the empty type — falls back to ".12~g".
const knownTypes = "efgrs%pbodxXc"

// Compile turns an already-parsed [Specifier] into a formatting function.
//
// All of the specifier-dependent work — resolving defaults, clamping the
// precision, choosing prefixes and suffixes — happens once, here; the returned
// closure only does per-value work. That is deliberate: a scale formats every
// tick on every redraw with the same specifier.
func (l *Locale) Compile(s Specifier) func(float64) string {
	fill, align, sign, symbol := s.Fill, s.Align, s.Sign, s.Symbol
	zero, width, comma, precision, trim, typ := s.Zero, s.Width, s.Comma, s.Precision, s.Trim, s.Type
	if fill == "" {
		fill = " "
	}
	if align == "" {
		align = ">"
	}
	if sign == "" {
		sign = "-"
	}

	switch {
	case typ == 'n':
		// "n" is an alias for ",g".
		comma, typ = true, 'g'
	case typ == 0 || strings.IndexByte(knownTypes, typ) < 0:
		// The empty (or unrecognized) type shows up to twelve significant
		// digits and trims the insignificant ones, which is the closest
		// thing to "just print the number sensibly".
		if precision == Unset {
			precision = 12
		}
		trim, typ = true, 'g'
	}

	// Zero fill implies that the padding sits between the sign and the digits,
	// so that -0001 rather than 000-1.
	if zero || (fill == "0" && align == "=") {
		zero, fill, align = true, "0", "="
	}

	prefix, suffix := "", ""
	switch {
	case symbol == "$":
		prefix, suffix = l.Currency[0], l.Currency[1]
	case symbol == "#" && strings.IndexByte("boxX", typ) >= 0:
		prefix = "0" + strings.ToLower(string(typ))
	}
	if symbol != "$" && (typ == '%' || typ == 'p') {
		suffix = l.Percent
	}

	// Only these types can produce a fractional or exponential tail that must
	// be held back from grouping: "1,234.5" groups the integer part only.
	maybeSuffix := strings.IndexByte("defgprs%", typ) >= 0

	switch {
	case precision == Unset:
		precision = 6
	case strings.IndexByte("gprs", typ) >= 0:
		precision = clampInt(precision, 1, 21) // significant digits
	default:
		precision = clampInt(precision, 0, 20) // decimal places
	}

	return func(value float64) string {
		valuePrefix, valueSuffix := prefix, suffix
		var v string

		if typ == 'c' {
			// The "c" type passes the value straight through; the whole
			// rendering becomes suffix, so alignment still applies but
			// grouping and sign handling do not.
			valueSuffix = jsString(value) + valueSuffix
			v = ""
		} else {
			// -0 is not less than 0, but it is still negative.
			negative := value < 0 || (value == 0 && math.Signbit(value))
			siExp := 0
			if math.IsNaN(value) {
				v = l.NaN
			} else {
				v, siExp = applyType(typ, math.Abs(value), precision)
			}
			if trim {
				v = formatTrim(v)
			}
			// A negative value that rounds to zero prints as zero, unless the
			// sign mode explicitly asks for a sign on everything.
			if negative && isZeroString(v) && sign != "+" {
				negative = false
			}

			switch {
			case negative && sign == "(":
				valuePrefix = "(" + valuePrefix
			case negative:
				valuePrefix = l.Minus + valuePrefix
			case sign == "-" || sign == "(":
				// no sign for non-negative values
			default:
				valuePrefix = sign + valuePrefix
			}
			if typ == 's' {
				valueSuffix = siPrefixes[8+siExp/3] + valueSuffix
			}
			if negative && sign == "(" {
				valueSuffix += ")"
			}

			// Split the rendering into the leading run of digits, which may be
			// grouped, and everything after it (a fraction, an exponent, the
			// letters of "Infinity"), which may not.
			if maybeSuffix {
				for i := 0; i < len(v); i++ {
					if c := v[i]; c < '0' || c > '9' {
						if c == '.' {
							valueSuffix = l.Decimal + v[i+1:] + valueSuffix
						} else {
							valueSuffix = v[i:] + valueSuffix
						}
						v = v[:i]
						break
					}
				}
			}
		}

		// With a non-zero fill, grouping happens before padding, so the padding
		// is not itself grouped.
		if comma && !zero {
			v = l.group(v, math.MaxInt)
		}

		length := runeLen(valuePrefix) + runeLen(v) + runeLen(valueSuffix)
		padding := ""
		if width != Unset && length < width {
			padding = strings.Repeat(fill, width-length)
		}

		// With a zero fill, padding happens first and is then grouped along
		// with the digits, so that "012,345" rather than "00012,345".
		if comma && zero {
			w := math.MaxInt
			if padding != "" {
				w = width - runeLen(valueSuffix)
			}
			v = l.group(padding+v, w)
			padding = ""
		}

		var out string
		switch align {
		case "<":
			out = valuePrefix + v + valueSuffix + padding
		case "=":
			out = valuePrefix + padding + v + valueSuffix
		case "^":
			runes := []rune(padding)
			half := len(runes) / 2
			out = string(runes[:half]) + valuePrefix + v + valueSuffix + string(runes[half:])
		default:
			out = padding + valuePrefix + v + valueSuffix
		}
		return l.numerals(out)
	}
}

// applyType renders the absolute value with the given type and precision. The
// second result is the SI exponent chosen by the "s" type, and zero otherwise.
func applyType(typ byte, x float64, precision int) (string, int) {
	switch typ {
	case 'e':
		return jsToExponential(x, precision), 0
	case 'f':
		return jsToFixed(x, precision), 0
	case 'g':
		return jsToPrecision(x, precision), 0
	case 'r':
		return formatRounded(x, precision), 0
	case 's':
		return formatPrefixAuto(x, precision)
	case '%':
		return jsToFixed(x*100, precision), 0
	case 'p':
		return formatRounded(x*100, precision), 0
	case 'b':
		return formatRadix(x, 2, false), 0
	case 'o':
		return formatRadix(x, 8, false), 0
	case 'x':
		return formatRadix(x, 16, false), 0
	case 'X':
		return formatRadix(x, 16, true), 0
	case 'd':
		return formatDecimal(x), 0
	default:
		return formatDecimal(x), 0
	}
}

// isZeroString reports whether a rendered value is numerically zero, which is
// how "-0.4" formatted as ".0f" becomes "0" rather than "-0".
func isZeroString(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	return err == nil && f == 0
}

// group inserts the thousands separator into a run of digits, honouring the
// locale's group sizes and cycling on the last one. width bounds the total
// length of the result, which the zero-fill path uses to keep a padded number
// exactly as wide as it asked to be; pass math.MaxInt for no bound.
func (l *Locale) group(value string, width int) string {
	if len(l.Grouping) == 0 || l.Thousands == "" || value == "" {
		return value
	}
	i := len(value)
	var t []string
	j := 0
	g := l.Grouping[0]
	length := 0
	for i > 0 && g > 0 {
		if width != math.MaxInt && length+g+1 > width {
			g = maxInt(1, width-length)
		}
		start := i - g
		if start < 0 {
			start = 0
		}
		t = append(t, value[start:i])
		i -= g
		length += g + 1
		if width != math.MaxInt && length > width {
			break
		}
		j = (j + 1) % len(l.Grouping)
		g = l.Grouping[j]
	}
	for a, b := 0, len(t)-1; a < b; a, b = a+1, b-1 {
		t[a], t[b] = t[b], t[a]
	}
	return strings.Join(t, l.Thousands)
}

// numerals substitutes the locale's digit set, if it has one.
func (l *Locale) numerals(s string) string {
	if len(l.Numerals) != 10 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= '0' && c <= '9' {
			b.WriteString(l.Numerals[c-'0'])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
