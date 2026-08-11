package scale

import (
	"math"

	"github.com/malcolmston/d3/array"
	"github.com/malcolmston/d3/format"
)

// tickFormatFor is d3-array's tickFormat: it compiles a d3 format specifier and,
// when the specifier leaves the precision open, fills it in from the tick step.
//
// Deriving precision from the step rather than from each value is what stops an
// axis from reading "0.30000000000000004" next to "0.4". The step says how much
// resolution the reader was promised; anything beyond that is noise. Which
// precision function applies depends on what the specifier's type counts —
// decimal places for f and %, significant digits for e/g/p/r, and for s the
// digits are chosen from the step while the SI prefix is pinned to the
// magnitude of the whole axis, so a gigawatt axis says "1.30G" and not "900M".
func tickFormatFor(start, stop float64, count int, specifier string) (func(float64) string, error) {
	step := array.TickStep(start, stop, count)
	if specifier == "" {
		specifier = ",f"
	}
	s, err := format.ParseSpecifier(specifier)
	if err != nil {
		return nil, err
	}
	switch s.Type {
	case 's':
		value := math.Max(math.Abs(start), math.Abs(stop))
		if s.Precision == format.Unset {
			s.Precision = format.PrecisionPrefix(step, value)
		}
		return format.NewPrefix(s.String(), value)
	case 0, 'e', 'g', 'p', 'r':
		if s.Precision == format.Unset {
			p := format.PrecisionRound(step, math.Max(math.Abs(start), math.Abs(stop)))
			if s.Type == 'e' {
				p--
			}
			s.Precision = p
		}
	case 'f', '%':
		if s.Precision == format.Unset {
			p := format.PrecisionFixed(step)
			if s.Type == '%' {
				p -= 2
			}
			if p < 0 {
				p = 0
			}
			s.Precision = p
		}
	}
	return format.New(s.String())
}
