// Package interpolate is a Go port of d3-interpolate — functions that blend
// between two values as a parameter t sweeps from 0 to 1.
//
// An interpolator is a closure: you give it the two endpoints once, and it
// gives you back a function of t.
//
//	f := interpolate.Number(10, 20)
//	f(0)    // 10
//	f(0.5)  // 15
//	f(1)    // 20
//
// That shape is the whole design. Building the closure is where the expensive
// work happens — parsing a string into number runs, converting colors into a
// perceptual space, precomputing spline coefficients — and it happens once,
// while calling it is cheap. An animation or a scale evaluates the same
// interpolator thousands of times, so the split matters.
//
// # t is not clamped
//
// This is the single most surprising property and it is deliberate, inherited
// from d3. Passing t = 1.5 does not return b; it extrapolates 50% past b.
// Passing t = -0.2 extrapolates before a. Every interpolator here follows that
// rule wherever the underlying operation is meaningfully defined outside
// [0, 1], which is most of them: [Number], [Round], [String], [RGB], [HSL],
// [Lab], [Basis].
//
// Extrapolation is a feature, not an oversight. d3 scales are built on it: a
// linear scale whose domain is [0, 100] must still produce a sensible answer
// for the value 120, and it does so by handing t = 1.2 to its interpolator.
// Clamping is a policy decision that belongs to the scale, not to the
// interpolation primitive, so it is applied there (or by the caller) rather
// than here. If you need clamped behavior, clamp t before calling, or wrap the
// interpolator.
//
// A few interpolators cannot extrapolate and say so on their own docs.
// [BasisClosed] wraps t into [0, 1] by construction, since a closed spline has
// no outside. [Piecewise] and [Value]'s slice/map dispatch inherit whatever the
// element interpolator does.
//
// # Endpoint exactness
//
// Every interpolator returns exactly a at t = 0 and exactly b at t = 1 — not a
// value within floating-point tolerance, but the endpoint itself where the type
// permits. The naive formula a + t*(b-a) does not guarantee that (it can drift
// at t = 1 for large magnitudes), so where it matters the implementation is
// arranged to be exact. This is what makes it safe to chain interpolators end
// to end, as [Piecewise] does, without visible seams at the joins.
//
// # Colors
//
// [RGB], [HSL], [Lab], [HCL] and [Cubehelix] interpolate in different spaces,
// and the choice is visible. Interpolating white to blue in sRGB passes through
// a dull gray-blue; in Lab it stays bright the whole way. Interpolating red to
// green in [HSL] sweeps through yellow (the short way round the wheel) while in
// [RGB] it passes through a muddy olive. Use [Lab] for sequential scales where
// perceptual evenness matters, [HSL] or [HCL] when you want a hue sweep, [RGB]
// when you need to match what CSS or canvas would do.
//
// # Known gaps
//
// This port covers the numeric, string, color, spline and generic dispatch
// parts of d3-interpolate. It does not include d3's transform (CSS/SVG matrix
// decomposition) interpolators, [Value] does not sniff color strings out of
// arbitrary text, and the discrete/quantize helpers live in the scale package
// rather than here.
package interpolate

import (
	"math"
	"strconv"
	"strings"
)

// Number returns an interpolator between two numbers.
//
// The result is a + t*(b-a). t is not clamped, so values outside [0, 1]
// extrapolate linearly — see the package documentation for why. Endpoints are
// exact: f(0) == a and f(1) == b bit for bit, because the expression is
// arranged as a*(1-t) + b*t at t == 1 rather than trusting a + 1*(b-a) to land
// on b for every magnitude.
//
// NaN endpoints propagate; infinite endpoints produce NaN at the t where the
// two infinities would cancel, which is the same behavior JavaScript has and
// not worth special-casing.
func Number(a, b float64) func(t float64) float64 {
	d := b - a
	return func(t float64) float64 {
		switch t {
		case 0:
			return a
		case 1:
			return b
		}
		return a + d*t
	}
}

// Round returns an interpolator between two numbers that rounds its result to
// the nearest integer.
//
// This is what you want for pixel positions and for any value that will index
// into something: interpolating without rounding produces subpixel jitter, and
// truncating instead of rounding biases everything toward zero. Rounding is
// half-away-from-zero (Go's math.Round), matching d3's Math.round only for
// positive values — JavaScript rounds -0.5 to -0 while this rounds it to -1.
// The difference shows up only exactly at .5 on negative numbers.
//
// As with [Number], t is not clamped, and the endpoints are exact after
// rounding: f(0) is round(a) and f(1) is round(b).
func Round(a, b float64) func(t float64) float64 {
	n := Number(a, b)
	return func(t float64) float64 { return math.Round(n(t)) }
}

// numberRun records one number found inside a string being interpolated: where
// its literal text began in the template, and the endpoint values to blend.
type numberRun struct {
	// index is the position in the output template's slice of literal chunks
	// after which this number is inserted.
	index int
	a, b  float64
}

// String returns an interpolator between two strings that blends the numbers
// embedded in them while keeping b's non-numeric text.
//
// This is how d3 animates structured strings without knowing anything about
// their syntax. Both strings are scanned for number runs; the runs are paired
// off in order, and each pair becomes a [Number] interpolator. Everything that
// is not a number is taken from b:
//
//	f := interpolate.String("translate(0,0)", "translate(10,20)")
//	f(0.5) // "translate(5,10)"
//
//	interpolate.String("1px solid red", "5px solid blue")(0.5)
//	// "3px solid blue"  — 1→5 is interpolated, "red"→"blue" is not
//
// Two rules follow from taking the literal text from b, and both are worth
// internalizing before relying on this.
//
// First, if b has more numbers than a, the extra ones are constant: they simply
// appear as written, at their final value, from t = 0 onward. If a has more
// numbers than b, the extras are dropped entirely, because there is no slot in
// b's template to put them in. Only the common prefix of number pairs animates.
//
// Second, when the two strings have different non-numeric text, f(0) does not
// equal a — it equals b's text with a's numbers substituted in. That is d3's
// behavior and it is the right trade: the alternative is refusing to interpolate
// whenever the templates differ at all, which would break the common case of
// "rgb(255, 0, 0)" to "rgb(0, 0, 255)" the moment one string has different
// spacing. When both strings share a template, f(0) reproduces a up to the
// canonical formatting of its numbers — "0.50" comes back as "0.5" — because
// the numbers are re-rendered rather than copied. The special case of a == b
// short-circuits to a constant function returning a, so the identity case is
// exact regardless.
//
// A number run is a decimal literal with optional sign, optional fraction and
// optional exponent: -1, 2.5, .5, 1e-3, +6.02E23 are all recognized. Numbers
// are formatted back with the shortest representation that round-trips, so
// interpolating 0 to 10 yields "5" rather than "5.000000". t is not clamped.
func String(a, b string) func(t float64) string {
	if a == b {
		return func(float64) string { return a }
	}
	// chunks holds the literal text of b split around its numbers; runs holds
	// the interpolators to splice back in between them. Building both up front
	// means evaluating at a given t is a walk over two slices and a
	// strings.Builder, with no re-scanning.
	var chunks []string
	var runs []numberRun

	aNums := findNumbers(a)
	bNums, bSpans := findNumbersWithSpans(b)

	prev := 0
	for i, span := range bSpans {
		chunks = append(chunks, b[prev:span[0]])
		prev = span[1]
		if i < len(aNums) {
			runs = append(runs, numberRun{index: len(chunks), a: aNums[i], b: bNums[i]})
		} else {
			// No counterpart in a: this number is constant, so fold its
			// literal text straight into the preceding chunk.
			chunks[len(chunks)-1] += b[span[0]:span[1]]
		}
	}
	chunks = append(chunks, b[prev:])

	if len(runs) == 0 {
		return func(float64) string { return b }
	}
	interps := make([]func(float64) float64, len(runs))
	for i, r := range runs {
		interps[i] = Number(r.a, r.b)
	}

	return func(t float64) string {
		var sb strings.Builder
		next := 0
		for i, chunk := range chunks {
			sb.WriteString(chunk)
			for next < len(runs) && runs[next].index == i+1 {
				sb.WriteString(strconv.FormatFloat(interps[next](t), 'g', -1, 64))
				next++
			}
		}
		return sb.String()
	}
}

// findNumbers returns the numeric values appearing in s, in order.
func findNumbers(s string) []float64 {
	v, _ := findNumbersWithSpans(s)
	return v
}

// findNumbersWithSpans returns the numeric values appearing in s, in order,
// along with the half-open byte ranges of their literal text.
//
// The scanner is hand-written rather than a regexp because it runs once per
// interpolator construction on short strings, and because the exact grammar
// matters: a sign is only part of a number when it directly precedes a digit or
// a leading dot, so "a-1" yields -1 but "1-1" yields 1 and -1 rather than 1 and
// a stray minus. That is the same grammar d3's /[-+]?(?:\d+\.?\d*|\.?\d+)(?:[eE][-+]?\d+)?/g
// implements.
func findNumbersWithSpans(s string) ([]float64, [][2]int) {
	var vals []float64
	var spans [][2]int
	i := 0
	for i < len(s) {
		start := i
		j := i
		if j < len(s) && (s[j] == '-' || s[j] == '+') {
			j++
		}
		digits := 0
		for j < len(s) && isDigit(s[j]) {
			j++
			digits++
		}
		if j < len(s) && s[j] == '.' {
			j++
			for j < len(s) && isDigit(s[j]) {
				j++
				digits++
			}
		}
		if digits == 0 {
			i = start + 1
			continue
		}
		// Optional exponent, accepted only when it is complete: "1e" is the
		// number 1 followed by the letter e, not a malformed literal.
		if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
			k := j + 1
			if k < len(s) && (s[k] == '-' || s[k] == '+') {
				k++
			}
			expDigits := 0
			for k < len(s) && isDigit(s[k]) {
				k++
				expDigits++
			}
			if expDigits > 0 {
				j = k
			}
		}
		v, err := strconv.ParseFloat(s[start:j], 64)
		if err != nil {
			i = start + 1
			continue
		}
		vals = append(vals, v)
		spans = append(spans, [2]int{start, j})
		i = j
	}
	return vals, spans
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// Numbers returns an interpolator between two slices of numbers, blending them
// element-wise.
//
// The output has len(b) elements, following the same "b is the template" rule
// as [String]: elements of b beyond len(a) are constant at their final value,
// and elements of a beyond len(b) are dropped. That asymmetry is what lets a
// scale's range grow or shrink without the interpolator having to error.
//
// The returned slice is freshly allocated on every call, so the caller may keep
// or mutate it. t is not clamped.
func Numbers(a, b []float64) func(t float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	interps := make([]func(float64) float64, n)
	for i := 0; i < n; i++ {
		interps[i] = Number(a[i], b[i])
	}
	tail := append([]float64(nil), b[n:]...)
	return func(t float64) []float64 {
		out := make([]float64, 0, len(b))
		for _, f := range interps {
			out = append(out, f(t))
		}
		return append(out, tail...)
	}
}

// Array returns an interpolator between two slices of arbitrary values, using
// interp for each element pair.
//
// It is the generic form of [Numbers] and follows the same length rule: the
// result has len(b) elements, extras in b are constant, extras in a are
// dropped. Pass [Value] as interp to get d3's behavior of dispatching on each
// element's own type.
//
// Extrapolation behavior is whatever interp does.
func Array[T any](interp func(a, b T) func(float64) T, a, b []T) func(t float64) []T {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	interps := make([]func(float64) T, n)
	for i := 0; i < n; i++ {
		interps[i] = interp(a[i], b[i])
	}
	tail := append([]T(nil), b[n:]...)
	return func(t float64) []T {
		out := make([]T, 0, len(b))
		for _, f := range interps {
			out = append(out, f(t))
		}
		return append(out, tail...)
	}
}

// Object returns an interpolator between two maps, using interp for each key
// present in both.
//
// Keys are treated the way [Array] treats indices: the result has exactly b's
// key set. A key only in b is constant at b's value; a key only in a is
// dropped. This is d3's object interpolation, and the reason it is keyed off b
// rather than off the union is that the result is meant to be a valid value of
// the same shape as b at every t, not a transient hybrid with keys the consumer
// does not expect.
//
// The returned map is freshly allocated on every call. Extrapolation behavior
// is whatever interp does.
func Object[K comparable, V any](interp func(a, b V) func(float64) V, a, b map[K]V) func(t float64) map[K]V {
	interps := make(map[K]func(float64) V, len(b))
	constants := make(map[K]V)
	for k, bv := range b {
		if av, ok := a[k]; ok {
			interps[k] = interp(av, bv)
		} else {
			constants[k] = bv
		}
	}
	return func(t float64) map[K]V {
		out := make(map[K]V, len(b))
		for k, f := range interps {
			out[k] = f(t)
		}
		for k, v := range constants {
			out[k] = v
		}
		return out
	}
}

// Piecewise chains a sequence of interpolators into one covering [0, 1].
//
// Given n values it builds n-1 interpolators and gives each an equal share of
// the parameter range, so with values [a, b, c] the result runs a→b over
// t in [0, 0.5] and b→c over [0.5, 1]. This is how a multi-stop color ramp is
// built out of two-endpoint primitives:
//
//	f := interpolate.Piecewise(interpolate.Lab, []color.Color{
//	    color.MustParse("#440154"),
//	    color.MustParse("#21918c"),
//	    color.MustParse("#fde725"),
//	})
//
// The internal joins are exact — at t = 0.5 above you get exactly b, not a
// value a rounding error away from it — because the segment index is computed
// by flooring and the local parameter is recomputed from it rather than
// accumulated.
//
// Extrapolation is delegated: t outside [0, 1] is clamped to the first or last
// *segment*, and that segment's interpolator is then called with a local
// parameter outside [0, 1]. So the ramp extrapolates along its end segments,
// which is the behavior a scale wants. A single value yields a constant
// function; zero values panics, since there is nothing to return.
func Piecewise[T any](interp func(a, b T) func(float64) T, values []T) func(t float64) T {
	if len(values) == 0 {
		panic("interpolate: Piecewise requires at least one value")
	}
	if len(values) == 1 {
		v := values[0]
		return func(float64) T { return v }
	}
	n := len(values) - 1
	interps := make([]func(float64) T, n)
	for i := 0; i < n; i++ {
		interps[i] = interp(values[i], values[i+1])
	}
	return func(t float64) T {
		f := math.Floor(t * float64(n))
		var i int
		switch {
		case math.IsNaN(f) || f < 0:
			i = 0
		case f >= float64(n):
			i = n - 1
		default:
			i = int(f)
		}
		return interps[i](t*float64(n) - float64(i))
	}
}
