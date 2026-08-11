package array

import "math"

// Adder is a full-precision accumulator for float64 addition: however many
// values you add, and in whatever order, its [Adder.Sum] is the correctly
// rounded double nearest the exact mathematical total. Plain summation is not —
// 0.1 + 0.2 + 0.3 famously is not 0.6, and a long sum of values with mixed
// magnitudes can lose most of its significant digits to cancellation.
//
// The implementation is Shewchuk's expansion-sum, the same algorithm d3-array's
// Adder uses (and the one behind Python's math.fsum). The running total is held
// not as one float64 but as a list of non-overlapping partial sums whose exact
// mathematical total is the true sum; each new value is merged into that list
// using error-free two-sum transformations, so no bits are ever discarded.
// [Adder.Sum] collapses the partials at the end, rounding exactly once.
//
// Reach for an Adder — or for [FSum], which wraps one — when summing many
// values whose magnitudes differ wildly, when a sum must be reproducible
// regardless of input order, or when the result feeds a difference that would
// otherwise cancel. For ordinary sums [Sum] is faster and good enough.
//
// The zero Adder is ready to use. It is not safe for concurrent use.
type Adder struct {
	partials []float64
}

// Add accumulates x and returns the receiver so that calls can be chained.
//
// Each existing partial is combined with the incoming value using a two-sum:
// hi is their rounded sum and lo is the exact rounding error, which is either
// zero (the addition was exact, so the partial is consumed) or a value small
// enough not to overlap hi, so it is kept as a partial in its own right. The
// carried hi then continues up the list. The result is that the partials remain
// non-overlapping and their exact sum is unchanged, which is the invariant the
// whole algorithm rests on.
func (a *Adder) Add(x float64) *Adder {
	i := 0
	for _, y := range a.partials {
		hi := x + y
		var lo float64
		if math.Abs(x) < math.Abs(y) {
			lo = x - (hi - y)
		} else {
			lo = y - (hi - x)
		}
		if lo != 0 {
			a.partials[i] = lo
			i++
		}
		x = hi
	}
	a.partials = append(a.partials[:i], x)
	return a
}

// Sum collapses the partial sums into the single float64 nearest the exact
// total. It does not reset the Adder, so it may be called repeatedly as values
// continue to arrive.
//
// The final fixup — doubling the low word and checking whether the addition was
// exact — is the round-half-to-even tie-break: when the exact result sits
// precisely between two representable doubles, the sign of the next partial down
// decides which way it goes, and skipping this step would round the last bit
// wrongly in exactly the cases the algorithm exists to get right.
func (a *Adder) Sum() float64 {
	n := len(a.partials)
	if n == 0 {
		return 0
	}
	n--
	hi := a.partials[n]
	var lo float64
	for n > 0 {
		x := hi
		n--
		y := a.partials[n]
		hi = x + y
		lo = y - (hi - x)
		if lo != 0 {
			break
		}
	}
	if n > 0 && ((lo < 0 && a.partials[n-1] < 0) || (lo > 0 && a.partials[n-1] > 0)) {
		y := lo * 2
		x := hi + y
		if y == x-hi {
			hi = x
		}
	}
	return hi
}

// Reset discards all accumulated values, returning the Adder to its zero state
// while keeping its allocated storage for reuse.
func (a *Adder) Reset() { a.partials = a.partials[:0] }
