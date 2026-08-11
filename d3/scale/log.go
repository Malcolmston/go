package scale

import (
	"math"

	"github.com/malcolmston/d3/array"
	"github.com/malcolmston/d3/format"
)

// logTicks reproduces d3's log tick algorithm. The shape of the problem: on a
// log axis the visually even positions are the powers of the base, but between
// 1 and 10 there is a lot of room, so d3 also emits the intermediate "digits"
// (2, 3, … 9) whenever the domain spans few enough powers that they will fit.
// When the base is not an integer those digits are meaningless, so it falls back
// to evenly spaced ticks in log space, mapped back through pow.
func (s *Continuous[R]) logTicks(d0, d1 float64, count int) []float64 {
	u, v := d0, d1
	reversed := v < u
	if reversed {
		u, v = v, u
	}
	i := s.logs(u)
	j := s.logs(v)
	n := float64(count)
	var z []float64

	if s.base == math.Trunc(s.base) && j-i < n {
		i = math.Floor(i)
		j = math.Ceil(j)
		if u > 0 {
		outerPos:
			for ; i <= j; i++ {
				for k := 1.0; k < s.base; k++ {
					var t float64
					if i < 0 {
						t = k / s.pows(-i)
					} else {
						t = k * s.pows(i)
					}
					if t < u {
						continue
					}
					if t > v {
						continue outerPos
					}
					z = append(z, t)
				}
			}
		} else {
		outerNeg:
			for ; i <= j; i++ {
				for k := s.base - 1; k >= 1; k-- {
					var t float64
					if i > 0 {
						t = k / s.pows(-i)
					} else {
						t = k * s.pows(i)
					}
					if t < u {
						continue
					}
					if t > v {
						continue outerNeg
					}
					z = append(z, t)
				}
			}
		}
		if float64(len(z))*2 < n {
			z = array.Ticks(u, v, count)
		}
	} else {
		m := count
		if int(j-i) < m {
			m = int(j - i)
		}
		if m < 1 {
			m = 1
		}
		raw := array.Ticks(i, j, m)
		z = make([]float64, len(raw))
		for idx, r := range raw {
			z[idx] = s.pows(r)
		}
	}

	if reversed {
		out := append([]float64(nil), z...)
		reverseFloats(out)
		return out
	}
	return z
}

// logTickFormat filters as well as formats. A log axis over [1, 1000] produces
// roughly 27 ticks; labelling all of them is unreadable, so d3 labels only the
// ones whose mantissa is small enough that the requested count is respected,
// returning "" for the rest. The tick is still drawn — only the text is
// suppressed — which is why log axes have many small unlabelled ticks and a few
// labelled ones at 1, 2, 5, 10, 20, 50, 100.
func (s *Continuous[R]) logTickFormat(count int, specifier string) (func(float64) string, error) {
	if specifier == "" {
		// SI prefixes read well on a base-10 axis ("1k", "10k"); for any other
		// base the powers are not round decimal numbers, so plain grouped
		// digits are clearer. This is d3's choice too.
		if s.base == 10 {
			specifier = "s"
		} else {
			specifier = ","
		}
	}
	spec, err := format.ParseSpecifier(specifier)
	if err != nil {
		return nil, err
	}
	// With an integer base every tick is a whole multiple of a power, so
	// trailing zeros carry no information and are trimmed.
	if s.base == math.Trunc(s.base) && spec.Precision == format.Unset {
		spec.Trim = true
	}
	f, err := format.New(spec.String())
	if err != nil {
		return nil, err
	}

	// k is the largest mantissa that still gets a label. It falls as the axis
	// spans more decades, which is what thins the labels out.
	nt := len(s.Ticks(count))
	if nt == 0 {
		nt = 1
	}
	k := math.Max(1, s.base*float64(count)/float64(nt))
	return func(d float64) string {
		i := d / s.pows(math.Round(s.logs(d)))
		if i*s.base < s.base-0.5 {
			i *= s.base
		}
		if i <= k {
			return f(d)
		}
		return ""
	}, nil
}
