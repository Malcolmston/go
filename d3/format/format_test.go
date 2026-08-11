package format

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestKnownValues asserts the exact strings that d3's specifier grammar pins
// down unambiguously. Each case is a fact about the grammar rather than an
// observation of a particular implementation: ".2f" means two decimal places,
// ".2r" means two significant digits, "," groups by three, "%" multiplies by a
// hundred, "$" wraps in the locale currency.
func TestKnownValues(t *testing.T) {
	tests := []struct {
		spec  string
		value float64
		want  string
	}{
		// Fixed point: precision is decimal places.
		{".2f", 3.14159, "3.14"},
		{".0f", 3.14159, "3"},
		{".4f", 3.14159, "3.1416"},
		{".2f", 0, "0.00"},
		{".2f", -1.005, "-1.00"}, // 1.005 is below the half in binary
		{"f", 1, "1.000000"},     // default precision is 6

		// Significant digits: the "r" type keeps positional notation.
		{".2r", 3.14159, "3.1"},
		{".3r", 3.14159, "3.14"},
		{".2r", 12345.0, "12000"},
		{".1r", 0.0123, "0.01"},

		// Grouping.
		{",d", 1234567, "1,234,567"},
		{",d", 123, "123"},
		{",d", 1000, "1,000"},
		{",.2f", 1234.5678, "1,234.57"},

		// Percent.
		{".1%", 0.123, "12.3%"},
		{".0%", 0.5, "50%"},
		{".2%", 1, "100.00%"},

		// Currency.
		{"$,.2f", 1234.5678, "$1,234.57"},
		{"$.2f", -12.5, "-$12.50"},

		// SI prefixes.
		{".3s", 1300000, "1.30M"},
		{".3~s", 1300000, "1.3M"},
		{".3s", 1500, "1.50k"},
		{".3s", 0.00042, "420µ"},
		{".3s", 1, "1.00"},

		// Exponential.
		{".2e", 42, "4.20e+1"},
		{".0e", 42, "4e+1"},
		{".2e", 0.000123, "1.23e-4"},

		// Integer types.
		{"d", 1234.6, "1235"},
		{"b", 5, "101"},
		{"o", 8, "10"},
		{"x", 255, "ff"},
		{"X", 255, "FF"},
		{"#x", 255, "0xff"},
		{"#b", 5, "0b101"},

		// Sign handling.
		{"+.2f", 1, "+1.00"},
		{"+.2f", -1, "-1.00"},
		{"(.2f", -1, "(1.00)"},
		{"(.2f", 1, "1.00"},
		{" .2f", 1, " 1.00"},

		// A negative value that rounds to zero loses its sign.
		{".0f", -0.4, "0"},
		// ...but only when the sign mode is not "+", which asks for an
		// explicit sign on everything and so keeps the negative zero.
		{"+.0f", -0.4, "-0"},

		// Width, fill and alignment.
		{"10.2f", 3.14159, "      3.14"},
		{"<10.2f", 3.14159, "3.14      "},
		{"^10.2f", 3.14159, "   3.14   "},
		{"010.2f", -3.14159, "-000003.14"},
		{"*>8.2f", 3.14159, "****3.14"},

		// "n" is an alias for ",g".
		{",.6g", 12345.0, "12,345.0"},
		{".6n", 12345.0, "12,345.0"},
		// "g" escapes to exponential once the exponent reaches the precision.
		{".4g", 12345.0, "1.235e+4"},

		// The empty type is ".12~g".
		{"", 0.1, "0.1"},
		{"", 1234567, "1234567"},

		// Trimming.
		{".4~f", 1.5, "1.5"},
		{".4~f", 1, "1"},
		{"~e", 1000, "1e+3"},
	}
	for _, tt := range tests {
		t.Run(tt.spec+"/"+strconv.FormatFloat(tt.value, 'g', -1, 64), func(t *testing.T) {
			got := MustNew(tt.spec)(tt.value)
			if got != tt.want {
				t.Errorf("MustNew(%q)(%v) = %q, want %q", tt.spec, tt.value, got, tt.want)
			}
		})
	}
}

// TestRoundingTiesAwayFromZero pins the one place where d3 and Go's strconv
// disagree. ECMA-262 rounds a value that is exactly halfway up; Go rounds it to
// even. Both 2.5 and 0.5 are exactly representable, so the difference is real
// and observable, and matching d3 is the point of the port.
func TestRoundingTiesAwayFromZero(t *testing.T) {
	tests := []struct {
		spec  string
		value float64
		want  string
	}{
		{".0f", 0.5, "1"},
		{".0f", 1.5, "2"},
		{".0f", 2.5, "3"},
		{".0f", 3.5, "4"},
		{"d", 2.5, "3"},
		{".1f", 0.25, "0.3"},
	}
	for _, tt := range tests {
		if got := MustNew(tt.spec)(tt.value); got != tt.want {
			t.Errorf("MustNew(%q)(%v) = %q, want %q (Go's strconv would round to even)", tt.spec, tt.value, got, tt.want)
		}
	}
	// The complement: 1.005 is *not* a tie, because its float64 value is
	// slightly below 1.005, so it must round down however the ties are broken.
	if got := MustNew(".2f")(1.005); got != "1.00" {
		t.Errorf(".2f of 1.005 = %q, want %q", got, "1.00")
	}
}

// TestPrecisionMeaning is the property that distinguishes the two families of
// types: for f/e/% the precision is a count of characters after the decimal
// point, for r/g/s/p it is a count of significant digits.
func TestPrecisionMeaning(t *testing.T) {
	for p := 1; p <= 8; p++ {
		fixed := MustNew("." + strconv.Itoa(p) + "f")(math.Pi)
		if got := len(fixed) - strings.IndexByte(fixed, '.') - 1; got != p {
			t.Errorf(".%df of pi = %q: %d decimals, want %d", p, fixed, got, p)
		}
		for _, v := range []float64{math.Pi, 12345.6789, 0.000123456789, 98765432.1} {
			r := MustNew("." + strconv.Itoa(p) + "r")(v)
			// Positional notation cannot say how many trailing zeros are
			// significant, so the assertion is two-sided: no more than p
			// digits are meaningful, and the value is accurate to within half
			// of the last retained digit.
			if got := significantDigits(r); got > p {
				t.Errorf(".%dr of %v = %q: %d significant digits, want at most %d", p, v, r, got, p)
			}
			f, err := strconv.ParseFloat(r, 64)
			if err != nil {
				t.Fatalf(".%dr of %v = %q: %v", p, v, r, err)
			}
			if rel := math.Abs(f-v) / math.Abs(v); rel > 5*math.Pow(10, float64(-p)) {
				t.Errorf(".%dr of %v = %q: relative error %g too large", p, v, r, rel)
			}
		}
	}
}

// significantDigits counts the digits of a positional-notation rendering,
// ignoring the leading zeros that only place the decimal point.
func significantDigits(s string) int {
	s = strings.TrimPrefix(s, "-")
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	s = strings.TrimRight(s, "0")
	return len(s)
}

// TestSIPrefixBoundaries checks the property that matters for axis labels: the
// prefix changes exactly at each power of a thousand, and the mantissa always
// lands in [1, 1000).
func TestSIPrefixBoundaries(t *testing.T) {
	f := MustNew(".3s")
	tests := []struct {
		value  float64
		prefix string
	}{
		{1, ""},
		{999, ""},
		{1000, "k"},
		{999000, "k"},
		{1e6, "M"},
		{1e9, "G"},
		{1e12, "T"},
		{1e15, "P"},
		{1e18, "E"},
		{1e21, "Z"},
		{1e24, "Y"},
		{1e-3, "m"},
		{1e-6, "µ"},
		{1e-9, "n"},
		{1e-12, "p"},
		{1e-15, "f"},
		{1e-18, "a"},
		{1e-21, "z"},
		{1e-24, "y"},
	}
	for _, tt := range tests {
		got := f(tt.value)
		if !strings.HasSuffix(got, tt.prefix) {
			t.Errorf(".3s of %v = %q, want suffix %q", tt.value, got, tt.prefix)
			continue
		}
		mantissa, err := strconv.ParseFloat(strings.TrimSuffix(got, tt.prefix), 64)
		if err != nil {
			t.Fatalf(".3s of %v = %q: %v", tt.value, got, err)
		}
		if mantissa < 1 || mantissa >= 1000 {
			t.Errorf(".3s of %v = %q: mantissa %v out of [1,1000)", tt.value, got, mantissa)
		}
	}
	// The prefix is clamped, so values beyond yotta keep growing the mantissa.
	if got := f(1e27); !strings.HasSuffix(got, "Y") {
		t.Errorf(".3s of 1e27 = %q, want a Y suffix", got)
	}
}

// TestWidthProperty: padding brings the rendering up to the requested width and
// never truncates a value that is already wider.
func TestWidthProperty(t *testing.T) {
	for _, align := range []string{"", "<", ">", "^", "="} {
		for w := 1; w <= 15; w++ {
			spec := align + strconv.Itoa(w) + ".2f"
			f := MustNew(spec)
			for _, v := range []float64{0, 1, -1, 1234.5, -98765.4321} {
				got := f(v)
				if runeLen(got) < w {
					t.Errorf("%q of %v = %q: width %d < %d", spec, v, got, runeLen(got), w)
				}
				bare := MustNew(".2f")(v)
				if runeLen(bare) >= w && got != bare {
					t.Errorf("%q of %v = %q, want unpadded %q", spec, v, got, bare)
				}
			}
		}
	}
}

// TestZeroFillKeepsSign: with "=" alignment (which zero fill implies) the sign
// stays outside the padding.
func TestZeroFillKeepsSign(t *testing.T) {
	if got := MustNew("08.2f")(-1); got != "-0001.00" {
		t.Errorf("08.2f of -1 = %q, want %q", got, "-0001.00")
	}
	if got := MustNew("08.2f")(1); got != "00001.00" {
		t.Errorf("08.2f of 1 = %q, want %q", got, "00001.00")
	}
}

// TestNaNAndInfinity: NaN prints the locale's spelling and infinities pass
// through as words rather than as digits.
func TestNaNAndInfinity(t *testing.T) {
	f := MustNew(".2f")
	if got := f(math.NaN()); got != "NaN" {
		t.Errorf(".2f of NaN = %q", got)
	}
	if got := f(math.Inf(1)); got != "Infinity" {
		t.Errorf(".2f of +Inf = %q", got)
	}
	if got := f(math.Inf(-1)); got != "-Infinity" {
		t.Errorf(".2f of -Inf = %q", got)
	}
	l := NewLocale(Locale{NaN: "n/a"})
	if got := l.MustNew(".2f")(math.NaN()); got != "n/a" {
		t.Errorf("locale NaN = %q, want %q", got, "n/a")
	}
}

// TestLocale exercises the culture-specific hooks against a locale that differs
// from en-US in every field, so a hard-coded separator anywhere would show up.
func TestLocale(t *testing.T) {
	de := NewLocale(Locale{
		Decimal:   ",",
		Thousands: ".",
		Grouping:  []int{3},
		Currency:  [2]string{"", " €"},
	})
	if got := de.MustNew("$,.2f")(1234.5678); got != "1.234,57 €" {
		t.Errorf("de $,.2f = %q", got)
	}
	// Indian grouping cycles on the last group size: 3, then 2, 2, 2, ...
	// Indian grouping: three digits, then twos. The last entry cycles, so the
	// list must be long enough to cover the number's width.
	in := NewLocale(Locale{Grouping: []int{3, 2, 2, 2, 2}})
	if got := in.MustNew(",d")(12345678); got != "1,23,45,678" {
		t.Errorf("indian ,d = %q, want %q", got, "1,23,45,678")
	}
	// Alternate numerals are applied to the finished string.
	ar := NewLocale(Locale{Numerals: []string{"٠", "١", "٢", "٣", "٤", "٥", "٦", "٧", "٨", "٩"}})
	if got := ar.MustNew("d")(1234); got != "١٢٣٤" {
		t.Errorf("arabic numerals = %q", got)
	}
	// The default locale is ASCII-only for the minus sign, unlike d3.
	if got := MustNew("d")(-1); got != "-1" {
		t.Errorf("default minus = %q", got)
	}
	u2212 := NewLocale(Locale{Minus: "−"})
	if got := u2212.MustNew("d")(-1); got != "−1" {
		t.Errorf("U+2212 minus = %q", got)
	}
}

// TestDefaultLocaleIsIndependentOfEnvironment: the package must never consult
// the host locale, so the same specifier gives the same bytes everywhere.
func TestDefaultLocaleIsIndependentOfEnvironment(t *testing.T) {
	for _, spec := range []string{",.2f", "$,.2f", ".3s", ".1%"} {
		got := MustNew(spec)(1234.5)
		for _, r := range got {
			if r > 127 && r != 'µ' {
				t.Errorf("%q produced non-ASCII %q in %q", spec, r, got)
			}
		}
	}
}

// TestFormatPrefix pins every label to one prefix, which is the whole reason
// the function exists: 900k next to 1.0M reads worse than 0.9M next to 1.0M.
func TestFormatPrefix(t *testing.T) {
	f := FormatPrefix(",.0", 1e6)
	if got := f(1.3e6); got != "1M" {
		t.Errorf("FormatPrefix(1e6)(1.3e6) = %q, want %q", got, "1M")
	}
	g := FormatPrefix(".1f", 1e6)
	for _, tt := range []struct {
		v    float64
		want string
	}{
		{9e5, "0.9M"},
		{1e6, "1.0M"},
		{1.1e6, "1.1M"},
	} {
		if got := g(tt.v); got != tt.want {
			t.Errorf("FormatPrefix(\".1f\", 1e6)(%v) = %q, want %q", tt.v, got, tt.want)
		}
	}
}

// TestInvalidSpecifier: only a genuine grammar violation is an error; an
// unrecognized type letter is accepted and behaves like the empty type.
func TestInvalidSpecifier(t *testing.T) {
	for _, bad := range []string{"foo", ".f2", "1.2.3f", "$$d", "..2f", "d,"} {
		if _, err := New(bad); err == nil {
			t.Errorf("New(%q) = nil error, want ErrInvalidSpecifier", bad)
		}
	}
	f, err := New("z")
	if err != nil {
		t.Fatalf("New(%q): %v", "z", err)
	}
	if got := f(0.1); got != "0.1" {
		t.Errorf("unknown type z: %q, want %q (the .12~g fallback)", got, "0.1")
	}
}

func TestMustNewPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustNew did not panic on an invalid specifier")
		}
	}()
	MustNew("nope!")
}
