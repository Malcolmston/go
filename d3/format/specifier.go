package format

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// specifierRe is a direct transcription of d3-format's grammar:
//
//	[[fill]align][sign][symbol][0][width][,][.precision][~][type]
//
// The optional fill character may only appear together with an alignment, which
// is why the first two groups are wrapped in a non-capturing group. (?s) lets
// the fill be a newline, matching JavaScript's "." over a single UTF-16 unit;
// here it is a single rune. The type is matched case-insensitively so that "X"
// (upper-case hexadecimal) is accepted alongside "x".
var specifierRe = regexp.MustCompile(`^(?s)(?:(.)?([<>=^]))?([-+( ])?([$#])?(0)?(\d+)?(,)?(\.\d+)?(~)?([a-zA-Z%])?$`)

// ErrInvalidSpecifier is returned by [ParseSpecifier] (and wrapped by [New])
// when a string is not a well-formed format specifier.
var ErrInvalidSpecifier = errors.New("format: invalid specifier")

// Unset is the sentinel stored in [Specifier.Width] and [Specifier.Precision]
// when the specifier omits them. JavaScript can say "undefined"; Go cannot, and
// zero is a meaningful precision (".0f" rounds to whole numbers), so the absent
// case needs its own value.
//
// This makes the zero [Specifier] value slightly awkward: it means "precision
// 0", not "no precision". Build specifiers with [ParseSpecifier] or start from
// [DefaultSpecifier] rather than from a bare struct literal.
const Unset = -1

// Specifier is a parsed d3 format specifier.
//
// The fields correspond one-for-one to the pieces of the grammar
// [[fill]align][sign][symbol][0][width][,][.precision][~][type]:
//
//	Fill      the padding character (default " ")
//	Align     "<" left, ">" right (default), "=" pad after the sign, "^" centered
//	Sign      "-" negative only (default), "+" always, "(" parentheses for
//	          negatives, " " a space for positives
//	Symbol    "$" currency, "#" a base prefix (0b/0o/0x) for b/o/x/X
//	Zero      zero padding, which also implies Fill "0" and Align "="
//	Width     the minimum field width, or [Unset]
//	Comma     group thousands
//	Precision significant digits or decimal places depending on Type, or [Unset]
//	Trim      the "~" flag: remove insignificant trailing zeros
//	Type      one of e f g r s % p b o d x X c n, or 0 for none
//
// Whether Precision counts significant digits or decimal places is the single
// most important thing about a specifier. For e, f and % it is the number of
// digits after the decimal point; for g, p, r and s it is the number of
// significant digits. ".2f" of 3.14159 is "3.14" (two decimals) while ".2r" is
// "3.1" (two significant digits) and ".2r" of 12345 is "12000".
type Specifier struct {
	Fill      string
	Align     string
	Sign      string
	Symbol    string
	Zero      bool
	Width     int
	Comma     bool
	Precision int
	Trim      bool
	Type      byte
}

// DefaultSpecifier returns the specifier that an empty format string denotes:
// space fill, right alignment, minus-only sign, no width and no precision.
func DefaultSpecifier() Specifier {
	return Specifier{Fill: " ", Align: ">", Sign: "-", Width: Unset, Precision: Unset}
}

// ParseSpecifier parses a d3 format specifier such as "$,.2f" or ">10.3~s".
//
// Every field is filled in, with the documented defaults substituted for the
// parts the string omits, so the result is directly usable. An unrecognized
// type letter is *not* an error here — d3 accepts it and treats it as the empty
// type, which behaves like ".12~g" — but a string that does not match the
// grammar at all returns [ErrInvalidSpecifier].
func ParseSpecifier(s string) (Specifier, error) {
	m := specifierRe.FindStringSubmatch(s)
	if m == nil {
		return Specifier{}, fmt.Errorf("%w: %q", ErrInvalidSpecifier, s)
	}
	spec := DefaultSpecifier()
	if m[1] != "" {
		spec.Fill = m[1]
	}
	if m[2] != "" {
		spec.Align = m[2]
	}
	if m[3] != "" {
		spec.Sign = m[3]
	}
	spec.Symbol = m[4]
	spec.Zero = m[5] != ""
	if m[6] != "" {
		w, err := strconv.Atoi(m[6])
		if err != nil {
			return Specifier{}, fmt.Errorf("%w: %q", ErrInvalidSpecifier, s)
		}
		spec.Width = w
	}
	spec.Comma = m[7] != ""
	if m[8] != "" {
		p, err := strconv.Atoi(m[8][1:])
		if err != nil {
			return Specifier{}, fmt.Errorf("%w: %q", ErrInvalidSpecifier, s)
		}
		spec.Precision = p
	}
	spec.Trim = m[9] != ""
	if m[10] != "" {
		spec.Type = m[10][0]
	}
	return spec, nil
}

// String renders the specifier back into the grammar, so that
//
//	s, _ := ParseSpecifier(x); ParseSpecifier(s.String())
//
// yields the same [Specifier] again. Fill and alignment are omitted when they
// hold their defaults (" " and ">"), which keeps ".2f" round-tripping as ".2f".
// d3's FormatSpecifier.toString always emits them, so the JavaScript output for
// that case is " >.2f"; both strings parse identically, and the shorter one is
// what a human wrote.
func (s Specifier) String() string {
	var b strings.Builder
	if (s.Fill != "" && s.Fill != " ") || (s.Align != "" && s.Align != ">") {
		if s.Fill != "" && s.Fill != " " {
			b.WriteString(s.Fill)
		}
		if s.Align != "" {
			b.WriteString(s.Align)
		} else {
			b.WriteString(">")
		}
	}
	if s.Sign != "" && s.Sign != "-" {
		b.WriteString(s.Sign)
	}
	b.WriteString(s.Symbol)
	if s.Zero {
		b.WriteByte('0')
	}
	if s.Width != Unset && s.Width > 0 {
		b.WriteString(strconv.Itoa(maxInt(1, s.Width)))
	}
	if s.Comma {
		b.WriteByte(',')
	}
	if s.Precision != Unset && s.Precision >= 0 {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(s.Precision))
	}
	if s.Trim {
		b.WriteByte('~')
	}
	if s.Type != 0 {
		b.WriteByte(s.Type)
	}
	return b.String()
}

// runeLen counts characters rather than bytes, so that a multi-byte fill or a
// non-ASCII currency symbol pads to the width a reader would expect.
func runeLen(s string) int { return utf8.RuneCountInString(s) }
