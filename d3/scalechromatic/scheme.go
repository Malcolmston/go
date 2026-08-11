package scalechromatic

import "github.com/malcolmston/d3/color"

// Family is a ColorBrewer scheme published at several class counts, indexed by
// the class count itself.
//
// ColorBrewer does not publish one list per family and then subset it. It
// publishes a separate, separately chosen list for each k, because the colors
// that keep five classes apart are not the first five of the colors that keep
// nine apart — with more classes each has to sit closer to its neighbors, so
// every list is rebalanced. Indexing by k rather than slicing is therefore not a
// convenience, it is the whole point.
//
//	SchemeBlues[5]        // the five-class Blues, as published
//	SchemeBlues[9][:5]    // five colors that are much too similar. Do not.
//
// The lower indices are nil, mirroring upstream's new Array(3).concat(…): a
// two-class ColorBrewer scheme does not exist, so Family[0], [1] and [2] are nil
// rather than approximated. Indexing past the end panics as any slice would;
// [Family.K] is the bounds-checked form and is what to use when k comes from
// outside the program.
type Family [][]color.Color

// K returns the family's list of k colors, or nil when the family is not
// published at that class count.
//
// Prefer this to direct indexing whenever k is data rather than a literal — a
// class count read from configuration or derived from a quantile scale's bucket
// count can easily be 2 or 12, and a nil result is a better outcome than a
// panic.
func (f Family) K(k int) []color.Color {
	if k < 0 || k >= len(f) {
		return nil
	}
	return f[k]
}

// Min returns the smallest class count the family is published at, or 0 when the
// family is empty.
func (f Family) Min() int {
	for k, cs := range f {
		if cs != nil {
			return k
		}
	}
	return 0
}

// Max returns the largest class count the family is published at, or 0 when the
// family is empty.
func (f Family) Max() int {
	for k := len(f) - 1; k >= 0; k-- {
		if f[k] != nil {
			return k
		}
	}
	return 0
}

// Largest returns the widest published list in the family.
//
// This is the list the family's continuous interpolator splines through, since
// it is the one that describes the family's shape in the most detail.
func (f Family) Largest() []color.Color { return f.K(f.Max()) }

// family builds a [Family] from upstream's per-k specifier strings, which begin
// at k = 3.
func family(specifiers ...string) Family {
	f := make(Family, 3, len(specifiers)+3)
	for _, s := range specifiers {
		f = append(f, colors(s))
	}
	return f
}

// colors splits a packed specifier — six hex digits per color, no separators —
// into opaque [color.RGB] values.
//
// This is d3's own storage format for these tables, kept so that data.go stays a
// character-for-character diff against the upstream bundle. It panics on a
// malformed specifier because the only specifiers it is ever handed are the
// generated literals in this package: a failure here is a corrupted source file,
// which should stop the program at init rather than yield a quietly wrong color.
func colors(specifier string) []color.Color {
	if len(specifier) == 0 || len(specifier)%6 != 0 {
		panic("scalechromatic: color specifier length is not a multiple of 6")
	}
	out := make([]color.Color, len(specifier)/6)
	for i := range out {
		s := specifier[i*6 : i*6+6]
		out[i] = color.RGB{
			R:       float64(hexPair(s[0], s[1])),
			G:       float64(hexPair(s[2], s[3])),
			B:       float64(hexPair(s[4], s[5])),
			Opacity: 1,
		}
	}
	return out
}

func hexPair(hi, lo byte) int { return hexDigit(hi)<<4 | hexDigit(lo) }

func hexDigit(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	}
	panic("scalechromatic: color specifier contains a non-hex digit")
}
