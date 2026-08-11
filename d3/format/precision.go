package format

import "math"

// This file answers the question every axis has to answer: given that ticks are
// this far apart, how many digits should each label show?
//
// Show too few and neighbouring ticks print the same string; show too many and
// the axis is full of digits that carry no information. The three Precision
// functions each solve that for one family of format types, and they are meant
// to be fed straight into a [Specifier].

// exponent returns the decimal exponent of x, i.e. floor(log10(|x|)), computed
// from the decimal representation rather than from math.Log10 so that exact
// powers of ten land on the right side of the boundary. It returns 0 for zero,
// NaN and infinities, which keeps the Precision functions total.
func exponent(x float64) int {
	_, e, ok := decimalParts(math.Abs(x), 0)
	if !ok {
		return 0
	}
	return e
}

// PrecisionFixed returns the number of decimal places needed so that ticks
// spaced step apart are all distinguishable in fixed ("f") notation.
//
// A step of 0.01 needs two decimals, a step of 1 needs none, and a step of 100
// still needs none — the result is never negative.
//
//	p := format.PrecisionFixed(0.01)          // 2
//	f := format.MustNew(fmt.Sprintf(".%df", p))
func PrecisionFixed(step float64) int {
	return maxInt(0, -exponent(math.Abs(step)))
}

// PrecisionPrefix returns the number of decimal places needed for the SI-prefix
// ("s") notation of values near value, when ticks are step apart.
//
// It exists because the SI prefix is chosen from the *magnitude of the axis*,
// not from each individual tick: on an axis running to 1.3 gigawatts with a
// step of ten megawatts you want "1.30G", so the prefix comes from value and
// the digit count from step. Pair it with [FormatPrefix], which pins every
// label to the same prefix.
func PrecisionPrefix(step, value float64) int {
	return maxInt(0, clampInt(int(math.Floor(float64(exponent(value))/3)), -8, 8)*3-exponent(math.Abs(step)))
}

// PrecisionRound returns the number of significant digits needed so that ticks
// spaced step apart are distinguishable, on an axis whose largest value is max,
// in the significant-digit notations ("r", "g", "s", "p").
//
// The reasoning is that the leading digits shared by max and max-step carry no
// information, so the precision must cover the gap between their exponents,
// plus one for the digit that actually differs.
func PrecisionRound(step, max float64) int {
	step = math.Abs(step)
	max = math.Abs(max) - step
	return maxInt(0, exponent(max)-exponent(step)) + 1
}

// NewPrefix compiles specifier into a formatter that pins the SI prefix to the
// one appropriate for value, instead of letting each number choose its own.
//
// This is the difference between an axis labelled "900k, 1.0M, 1.1M" and one
// labelled "0.9M, 1.0M, 1.1M". The specifier's type is forced to "f", because
// the prefix is applied by dividing rather than by the "s" type's own logic, so
// the precision is a count of decimal places — use [PrecisionPrefix] to pick it.
func NewPrefix(specifier string, value float64) (func(float64) string, error) {
	return EnUS.NewPrefix(specifier, value)
}

// FormatPrefix is [NewPrefix] for specifiers known at compile time; it panics
// on a malformed specifier.
func FormatPrefix(specifier string, value float64) func(float64) string {
	f, err := NewPrefix(specifier, value)
	if err != nil {
		panic(err)
	}
	return f
}

// NewPrefix compiles a prefix-pinned formatter against this locale.
func (l *Locale) NewPrefix(specifier string, value float64) (func(float64) string, error) {
	s, err := ParseSpecifier(specifier)
	if err != nil {
		return nil, err
	}
	s.Type = 'f'
	f := l.Compile(s)
	e := clampInt(int(math.Floor(float64(exponent(value))/3)), -8, 8) * 3
	k := math.Pow(10, float64(-e))
	prefix := siPrefixes[8+e/3]
	return func(v float64) string { return f(k*v) + prefix }, nil
}

// FormatPrefix is [Locale.NewPrefix] but panics instead of returning an error.
func (l *Locale) FormatPrefix(specifier string, value float64) func(float64) string {
	f, err := l.NewPrefix(specifier, value)
	if err != nil {
		panic(err)
	}
	return f
}

// SIPrefix returns the SI prefix symbol for a decimal exponent, clamped to the
// yocto..yotta range that d3 supports. SIPrefix(6) is "M" and SIPrefix(-3) is
// "m"; exponents that are not multiples of three round down to the next lower
// prefix, which is what the "s" type does internally.
func SIPrefix(exp10 int) string {
	return siPrefixes[8+clampInt(int(math.Floor(float64(exp10)/3)), -8, 8)]
}
