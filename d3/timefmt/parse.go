package timefmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parsing is the mirror image of formatting, and it is the harder half, because
// a specifier does not have to mention every field and the fields it does
// mention may over-determine the date. "%Y-%W-%w" gives a year, a week number
// and a weekday, from which a calendar date has to be reconstructed; "%s" gives
// an instant directly and everything else is irrelevant.
//
// So parsing happens in two phases, exactly as in d3. First the specifier is
// walked and each directive deposits what it read into a [parts] value, which
// records not just the numbers but *which* directives appeared. Then
// [parts.resolve] turns that bag of fields into a [time.Time], applying the
// precedence rules: a Unix timestamp wins outright, an am/pm flag folds into
// the hour, a week number plus a weekday becomes a day of the year, and a zone
// offset reinterprets everything as UTC.
//
// Unmentioned fields default to 1900-01-01 00:00:00, which is d3's default and
// is deliberately not "now": a parsed value must be a pure function of its
// input.

// parts accumulates the fields a parse has seen so far. set records which
// directives appeared, standing in for JavaScript's "in" operator on a
// dynamically-grown object.
type parts struct {
	y  int // full year
	m  int // month, 0-11
	dd int // day of month
	H  int
	M  int
	S  int
	ns int // nanoseconds

	p int // 0 for AM, 1 for PM
	q int // quarter, expressed as its first month (0, 3, 6, 9)
	U int // week of year, Sunday-based
	W int // week of year, Monday-based
	V int // ISO week of year
	w int // weekday, 0 = Sunday
	u int // ISO weekday, 1 = Monday
	Z int // zone offset as -(hhmm), i.e. positive west of UTC

	Q   int64 // milliseconds since the epoch
	sec int64 // seconds since the epoch

	set map[byte]bool
}

func (d *parts) has(c byte) bool { return d.set[c] }
func (d *parts) mark(c byte)     { d.set[c] = true }

// resolve converts the accumulated fields into an instant. loc is the location
// unzoned fields are interpreted in; a %Z in the specifier overrides it.
func (d *parts) resolve(loc *time.Location) (time.Time, error) {
	// An explicit timestamp short-circuits everything else.
	if d.has('Q') {
		return time.UnixMilli(d.Q).In(loc), nil
	}
	if d.has('s') {
		ns := 0
		if d.has('L') || d.has('f') {
			ns = d.ns
		}
		return time.Unix(d.sec, int64(ns)).In(loc), nil
	}

	if d.has('p') {
		d.H = d.H%12 + d.p*12
	}
	if !d.has('m') {
		if d.has('q') {
			d.m = d.q
		} else {
			d.m = 0
		}
	}

	// Week numbers are resolved in the same calendar the final value uses, so
	// that "week 1 of 2024" means the same thing the formatter would print.
	wl := loc
	if d.has('Z') {
		wl = time.UTC
	}

	switch {
	case d.has('V'):
		if d.V < 1 || d.V > 53 {
			return time.Time{}, fmt.Errorf("%w: ISO week %d out of range [1,53]", ErrParse, d.V)
		}
		if !d.has('w') {
			// d3 defaults the weekday to Monday here and ignores %u, which
			// makes "%G-W%V-%u" — the canonical ISO 8601 week date — fail to
			// round trip. Honouring %u costs nothing and is plainly the
			// intent, so this port does.
			if d.has('u') {
				d.w = d.u % 7
			} else {
				d.w = 1
			}
		}
		week := time.Date(d.y, time.January, 1, 0, 0, 0, 0, wl)
		// ISO week 1 is the week containing the first Thursday, i.e. the
		// Monday-week that January 4th falls in.
		if day := int(week.Weekday()); day > 4 || day == 0 {
			week = Monday.Ceil(week)
		} else {
			week = Monday.Floor(week)
		}
		week = Day.Offset(week, (d.V-1)*7)
		d.y, d.m, d.dd = week.Year(), int(week.Month())-1, week.Day()+(d.w+6)%7
	case d.has('W') || d.has('U'):
		if !d.has('w') {
			switch {
			case d.has('u'):
				d.w = d.u % 7
			case d.has('W'):
				d.w = 1
			default:
				d.w = 0
			}
		}
		day := int(time.Date(d.y, time.January, 1, 0, 0, 0, 0, wl).Weekday())
		d.m = 0
		if d.has('W') {
			d.dd = (d.w+6)%7 + d.W*7 - (day+5)%7
		} else {
			d.dd = d.w + d.U*7 - (day+6)%7
		}
	}

	if d.has('Z') {
		// The fields describe a wall clock at the parsed offset. Shifting by
		// the offset makes them UTC; re-expressing the result in a fixed zone
		// gives back the wall clock the input actually wrote, so that a
		// format/parse round trip through %Z is the identity.
		offsetSeconds := -(d.Z/100*3600 + d.Z%100*60)
		t := time.Date(d.y, time.Month(d.m+1), d.dd, d.H+d.Z/100, d.M+d.Z%100, d.S, d.ns, time.UTC)
		if offsetSeconds == 0 {
			return t, nil
		}
		return t.In(time.FixedZone("", offsetSeconds)), nil
	}
	return time.Date(d.y, time.Month(d.m+1), d.dd, d.H, d.M, d.S, d.ns, loc), nil
}

// parseSpec walks the specifier and the input in lockstep. Literal characters
// must match exactly; directives consume as much as they are allowed to. It
// returns the index one past the last consumed byte of s.
func (l *TimeLocale) parseSpec(d *parts, spec, s string, i, depth int) (int, error) {
	for j := 0; j < len(spec); {
		if i >= len(s) {
			return 0, fmt.Errorf("%w: %q ended before %q was satisfied", ErrParse, s, spec)
		}
		c := spec[j]
		j++
		if c != '%' {
			if s[i] != c {
				return 0, fmt.Errorf("%w: expected %q at offset %d of %q", ErrParse, string(c), i, s)
			}
			i++
			continue
		}
		if j >= len(spec) {
			return 0, fmt.Errorf("%w: specifier %q ends with a bare %%", ErrParse, spec)
		}
		c = spec[j]
		j++
		if _, isPad := pads[c]; isPad {
			if j >= len(spec) {
				return 0, fmt.Errorf("%w: specifier %q ends with a bare padding flag", ErrParse, spec)
			}
			c = spec[j]
			j++
		}
		ni, err := l.parseDirective(d, c, s, i, depth)
		if err != nil {
			return 0, err
		}
		i = ni
	}
	return i, nil
}

func (l *TimeLocale) parseDirective(d *parts, c byte, s string, i, depth int) (int, error) {
	fail := func() (int, error) {
		return 0, fmt.Errorf("%w: %%%c does not match %q at offset %d", ErrParse, c, s, i)
	}
	switch c {
	case 'a':
		return l.parseName(d, c, l.ShortDays[:], s, i)
	case 'A':
		return l.parseName(d, c, l.Days[:], s, i)
	case 'b':
		return l.parseName(d, c, l.ShortMonths[:], s, i)
	case 'B':
		return l.parseName(d, c, l.Months[:], s, i)
	case 'p':
		return l.parseName(d, c, l.Periods[:], s, i)

	case 'c', 'x', 'X':
		if depth >= maxDepth {
			return fail()
		}
		return l.parseSpec(d, l.subPattern(c), s, i, depth+1)

	case 'd', 'e':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.dd = v
		d.mark('d')
		return ni, nil
	case 'f':
		v, ni, ok := scanInt(s, i, 6)
		if !ok {
			return fail()
		}
		d.ns = v * 1000
		d.mark('f')
		return ni, nil
	case 'L':
		v, ni, ok := scanInt(s, i, 3)
		if !ok {
			return fail()
		}
		d.ns = v * 1e6
		d.mark('L')
		return ni, nil
	case 'H', 'I':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.H = v
		d.mark('H')
		return ni, nil
	case 'j':
		v, ni, ok := scanInt(s, i, 3)
		if !ok {
			return fail()
		}
		d.m, d.dd = 0, v
		d.mark('m')
		d.mark('d')
		return ni, nil
	case 'm':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.m = v - 1
		d.mark('m')
		return ni, nil
	case 'M':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.M = v
		d.mark('M')
		return ni, nil
	case 'q':
		v, ni, ok := scanInt(s, i, 1)
		if !ok {
			return fail()
		}
		d.q = (v - 1) * 3
		d.mark('q')
		return ni, nil
	case 'Q':
		v, ni, ok := scanInt64(s, i, len(s)-i)
		if !ok {
			return fail()
		}
		d.Q = v
		d.mark('Q')
		return ni, nil
	case 's':
		v, ni, ok := scanInt64(s, i, len(s)-i)
		if !ok {
			return fail()
		}
		d.sec = v
		d.mark('s')
		return ni, nil
	case 'S':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.S = v
		d.mark('S')
		return ni, nil
	case 'u':
		v, ni, ok := scanInt(s, i, 1)
		if !ok {
			return fail()
		}
		d.u = v
		d.mark('u')
		return ni, nil
	case 'w':
		v, ni, ok := scanInt(s, i, 1)
		if !ok {
			return fail()
		}
		d.w = v
		d.mark('w')
		return ni, nil
	case 'U':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.U = v
		d.mark('U')
		return ni, nil
	case 'V':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.V = v
		d.mark('V')
		return ni, nil
	case 'W':
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		d.W = v
		d.mark('W')
		return ni, nil
	case 'y', 'g':
		// A two-digit year is windowed the way d3 does it: 69-99 mean the
		// twentieth century, 00-68 the twenty-first.
		v, ni, ok := scanInt(s, i, 2)
		if !ok {
			return fail()
		}
		if v > 68 {
			d.y = v + 1900
		} else {
			d.y = v + 2000
		}
		d.mark('y')
		return ni, nil
	case 'Y', 'G':
		v, ni, ok := scanInt(s, i, 4)
		if !ok {
			return fail()
		}
		d.y = v
		d.mark('Y')
		return ni, nil
	case 'Z':
		ni, ok := scanZone(d, s, i)
		if !ok {
			return fail()
		}
		d.mark('Z')
		return ni, nil
	case '%':
		if i < len(s) && s[i] == '%' {
			return i + 1, nil
		}
		return fail()
	}
	return 0, fmt.Errorf("%w: unknown directive %%%c", ErrParse, c)
}

// parseName matches the longest of the locale's names that prefixes the input,
// case-insensitively, and records its index in the field the directive owns.
// Longest-match matters for locales whose names share a prefix.
func (l *TimeLocale) parseName(d *parts, c byte, names []string, s string, i int) (int, error) {
	best, bestLen := -1, 0
	rest := s[i:]
	for k, n := range names {
		if len(n) > bestLen && len(rest) >= len(n) && strings.EqualFold(rest[:len(n)], n) {
			best, bestLen = k, len(n)
		}
	}
	if best < 0 {
		return 0, fmt.Errorf("%w: %%%c does not match %q at offset %d", ErrParse, c, s, i)
	}
	switch c {
	case 'a', 'A':
		d.w = best
		d.mark('w')
	case 'b', 'B':
		d.m = best
		d.mark('m')
	case 'p':
		d.p = best
		d.mark('p')
	}
	return i + bestLen, nil
}

// scanInt reads up to maxLen bytes of leading whitespace and digits, mirroring
// d3's /^\s*\d+/ applied to a fixed-width slice. The width limit is what stops
// "%Y%m%d" from swallowing the whole string with %Y.
func scanInt(s string, i, maxLen int) (int, int, bool) {
	v, ni, ok := scanInt64(s, i, maxLen)
	return int(v), ni, ok
}

func scanInt64(s string, i, maxLen int) (int64, int, bool) {
	if maxLen <= 0 {
		return 0, i, false
	}
	end := i + maxLen
	if end > len(s) {
		end = len(s)
	}
	j := i
	for j < end && isSpace(s[j]) {
		j++
	}
	k := j
	for k < end && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == j {
		return 0, i, false
	}
	v, err := strconv.ParseInt(s[j:k], 10, 64)
	if err != nil {
		return 0, i, false
	}
	return v, k, true
}

func isSpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	}
	return false
}

// scanZone accepts "Z", "-07", "-0700" and "-07:00", storing the offset the way
// JavaScript's getTimezoneOffset reports it — positive west of UTC — because
// that is the sign convention the resolution step expects.
func scanZone(d *parts, s string, i int) (int, bool) {
	if i < len(s) && s[i] == 'Z' {
		d.Z = 0
		return i + 1, true
	}
	if i+3 > len(s) {
		return 0, false
	}
	sign := s[i]
	if sign != '+' && sign != '-' {
		return 0, false
	}
	if !isDigit(s[i+1]) || !isDigit(s[i+2]) {
		return 0, false
	}
	hh := int(s[i+1]-'0')*10 + int(s[i+2]-'0')
	j := i + 3
	mm := 0
	k := j
	if k < len(s) && s[k] == ':' {
		k++
	}
	if k+1 < len(s) && isDigit(s[k]) && isDigit(s[k+1]) {
		mm = int(s[k]-'0')*10 + int(s[k+1]-'0')
		j = k + 2
	}
	v := hh*100 + mm
	if sign == '-' {
		d.Z = v
	} else {
		d.Z = -v
	}
	return j, true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
