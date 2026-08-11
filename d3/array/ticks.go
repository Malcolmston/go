package array

import "math"

// The three thresholds that decide whether a "nice" step is 1, 2, 5 or 10 times
// a power of ten. They are the geometric midpoints of those choices — sqrt(1*2),
// sqrt(2*5) and sqrt(5*10) — so that the chosen factor is the one whose
// logarithm is nearest the ideal step. Computing them as square roots rather
// than writing 1.41, 3.16, 7.07 keeps the comparison exact at the boundaries.
var (
	e10 = math.Sqrt(50)
	e5  = math.Sqrt(10)
	e2  = math.Sqrt(2)
)

// jsRound rounds half away from +infinity, the way JavaScript's Math.round does.
//
// Go's math.Round rounds halves away from zero, so math.Round(-2.5) is -3 while
// Math.round(-2.5) is -2. tickSpec rounds domain endpoints scaled by the
// increment, and those endpoints are routinely negative and routinely land on a
// half, so using math.Round here would shift ticks by one whole step on negative
// domains. This is the only place the difference matters, and it is why the port
// does not simply call math.Round.
func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

// tickSpec computes the half-open integer range [i1, i2] and the increment that
// together describe the ticks of [start, stop] at approximately count steps.
// start must not exceed stop.
//
// The returned inc is signed with a twist that is easy to misread: a *negative*
// inc means "divide by -inc" rather than "multiply by inc". d3 does this so that
// sub-unit ticks are produced by exact division — the ticks of [0,1] at count 10
// are 0/10, 1/10, … 10/10, which are the correctly rounded doubles 0.1, 0.2,
// 0.3, instead of the visibly wrong 0.30000000000000004 that repeated addition
// or multiplication by 0.1 would give.
//
// The recursive tail handles a single-tick request that produced an empty range:
// with 0.5 <= count < 2 the span may be too narrow for any round value, so the
// count is doubled and the search retried, which is how d3 guarantees ticks with
// count 1 still returns something for most domains.
func tickSpec(start, stop, count float64) (i1, i2, inc float64) {
	step := (stop - start) / math.Max(0, count)
	power := math.Floor(math.Log10(step))
	err := step / math.Pow(10, power)

	factor := 1.0
	switch {
	case err >= e10:
		factor = 10
	case err >= e5:
		factor = 5
	case err >= e2:
		factor = 2
	}

	if power < 0 {
		inc = math.Pow(10, -power) / factor
		i1 = jsRound(start * inc)
		i2 = jsRound(stop * inc)
		if i1/inc < start {
			i1++
		}
		if i2/inc > stop {
			i2--
		}
		inc = -inc
	} else {
		inc = math.Pow(10, power) * factor
		i1 = jsRound(start / inc)
		i2 = jsRound(stop / inc)
		if i1*inc < start {
			i1++
		}
		if i2*inc > stop {
			i2--
		}
	}

	if i2 < i1 && 0.5 <= count && count < 2 {
		return tickSpec(start, stop, count*2)
	}
	return i1, i2, inc
}

// maxTicks caps the length of a generated tick slice. d3 has no such limit
// because JavaScript will happily allocate until it dies; a Go library that
// allocates a billion float64s because a caller passed a denormal domain is a
// denial of service. Domains this large are always a bug at the call site.
const maxTicks = 1 << 24

// Ticks returns approximately count representative values spanning the domain
// [start, stop], chosen so that they are "nice" round numbers — multiples of a
// power of ten, or of two or five times a power of ten. The values are
// inclusive of the endpoints when the endpoints themselves are round, so
// Ticks(0, 1, 10) is 0, 0.1, … 1 — eleven values for a count of ten. The count
// is a hint, not a guarantee.
//
// If stop is less than start the ticks are returned in descending order, so a
// reversed domain produces reversed ticks. If start equals stop the single value
// start is returned. If count is zero or negative the result is empty, since
// there is no such thing as a zero-tick axis.
//
// This is the function every scale's axis is built from; see also [TickStep] for
// just the spacing and [NiceTicks] for extending a domain outward to round
// values before ticking it.
func Ticks(start, stop float64, count int) []float64 {
	if count <= 0 || math.IsNaN(start) || math.IsNaN(stop) {
		return []float64{}
	}
	if start == stop {
		return []float64{start}
	}
	reverse := stop < start

	var i1, i2, inc float64
	if reverse {
		i1, i2, inc = tickSpec(stop, start, float64(count))
	} else {
		i1, i2, inc = tickSpec(start, stop, float64(count))
	}
	if !(i2 >= i1) {
		return []float64{}
	}

	nf := i2 - i1 + 1
	if !(nf >= 1) || nf > maxTicks {
		return []float64{}
	}
	n := int(nf)

	ticks := make([]float64, n)
	// Note the division when inc is negative: see tickSpec for why exact
	// division rather than multiplication is what keeps 0.3 spelled "0.3".
	switch {
	case reverse && inc < 0:
		for i := range ticks {
			ticks[i] = (i2 - float64(i)) / -inc
		}
	case reverse:
		for i := range ticks {
			ticks[i] = (i2 - float64(i)) * inc
		}
	case inc < 0:
		for i := range ticks {
			ticks[i] = (i1 + float64(i)) / -inc
		}
	default:
		for i := range ticks {
			ticks[i] = (i1 + float64(i)) * inc
		}
	}
	return ticks
}

// TickIncrement returns the raw increment [Ticks] would use for the domain
// [start, stop] at approximately count steps, in d3's signed encoding: a
// positive result is a step to multiply by, and a negative result -n means the
// step is 1/n. Most callers want [TickStep], which decodes this into a plain
// step size; TickIncrement exists for code that wants to reproduce d3's exact
// division and so needs the reciprocal form.
//
// start may exceed stop, in which case the increment describes the reversed
// domain and is not itself negated — again, use [TickStep] if you want a signed
// step that tracks the domain's direction.
func TickIncrement(start, stop float64, count int) float64 {
	_, _, inc := tickSpec(start, stop, float64(count))
	return inc
}

// TickStep returns the difference between adjacent values of Ticks(start, stop,
// count) — a "nice" step: a power of ten, optionally multiplied by two or five.
// The sign follows the domain, so TickStep is negative when stop is less than
// start, and the returned step is exact in the sense that adding it repeatedly
// from a niced start reproduces the ticks up to floating-point rounding.
//
// Because the count is only a hint, the actual number of ticks may differ; a
// scale that needs the values themselves should call [Ticks] rather than
// deriving them from the step.
func TickStep(start, stop float64, count int) float64 {
	reverse := stop < start
	var inc float64
	if reverse {
		inc = TickIncrement(stop, start, count)
	} else {
		inc = TickIncrement(start, stop, count)
	}
	sign := 1.0
	if reverse {
		sign = -1
	}
	if inc < 0 {
		return sign * (1 / -inc)
	}
	return sign * inc
}

// NiceTicks extends the domain [start, stop] outward to the nearest round
// values for the given tick count, and returns the extended domain. It is d3's
// d3.nice, renamed because "nice" alone says nothing about what it operates on;
// scales call it so that a domain derived from data ([0.37, 9.6], say) becomes
// one whose endpoints are themselves ticks ([0, 10]).
//
// The rounding is iterated rather than done once: widening the domain can change
// which step size [TickIncrement] would choose, so NiceTicks re-rounds until the
// step stops changing. It converges in two or three passes for any real domain;
// a degenerate one (start == stop, a non-finite endpoint, count <= 0) returns
// the input unchanged rather than looping.
//
// There is one deliberate deviation from d3 here. d3's nice loops until the step
// repeats, which for a small count over an asymmetric domain — nice(-3.2, 7.8, 1)
// — never happens: each widening pushes the chosen step up a size, which widens
// the domain further, forever. d3 hangs; this port stops as soon as the step
// grows instead of shrinking or holding, and returns the last domain it produced.
// Every domain on which d3 terminates gets d3's answer.
func NiceTicks(start, stop float64, count int) (float64, float64) {
	prestep, premag := math.NaN(), math.Inf(1)
	for {
		step := TickIncrement(start, stop, count)
		if step == prestep || step == 0 || math.IsInf(step, 0) || math.IsNaN(step) {
			return start, stop
		}
		// Decode the signed increment into an actual step size so successive
		// passes can be compared; see tickSpec for the encoding.
		mag := step
		if step < 0 {
			mag = 1 / -step
		}
		if mag > premag {
			return start, stop // diverging; see the doc comment
		}
		if step > 0 {
			start = math.Floor(start/step) * step
			stop = math.Ceil(stop/step) * step
		} else {
			start = math.Ceil(start*step) / step
			stop = math.Floor(stop*step) / step
		}
		prestep, premag = step, mag
	}
}
