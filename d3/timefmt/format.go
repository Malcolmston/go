// Package timefmt is a Go port of d3-time and d3-time-format: calendar
// intervals, and strftime-style formatting and parsing of instants.
//
// It exists so that a d3 specifier string works unchanged in Go. Go has
// [time.Time] and its own reference-time layouts ("2006-01-02"), which are
// lovely to read and completely incompatible with everything the d3 ecosystem
// writes down. A time axis is configured with strings like "%b %d" or
// "%Y-%m-%dT%H:%M:%S.%LZ", and those strings travel: they come from
// configuration files, from URL parameters, from a JavaScript sibling rendering
// the same chart. So this package implements d3's directive grammar directly
// rather than translating it into Go layouts — a translation that could not be
// faithful anyway, because Go has no layout for the ISO week-based year, the
// day of the year, or the quarter.
//
// Two things this package provides:
//
//   - [Interval] and its instances ([Second], [Minute], [Hour], [Day], [Week],
//     [Monday] … [Sunday], [Month], [Year], and the UTC counterparts): floor,
//     ceil, round, offset, range, filter, every and count.
//   - [Format] and [Parse], which compile a d3 specifier into a function.
//
// and one thing that joins them: [TickInterval], the algorithm a time scale
// uses to choose a sensible tick spacing for a domain.
//
//	f := timefmt.Format("%B %d, %Y")
//	f(time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)) // "March 09, 2024"
//
//	p := timefmt.Parse("%Y-%m-%d")
//	t, err := p("2024-03-09")
//
// # Directives
//
//	%a  abbreviated weekday name        %A  full weekday name
//	%b  abbreviated month name          %B  full month name
//	%c  the locale date and time        %x  the locale date
//	%X  the locale time
//	%d  zero-padded day of month        %e  space-padded day of month
//	%f  microseconds [000000,999999]    %L  milliseconds [000,999]
//	%g  ISO week-based year, 2 digits   %G  ISO week-based year
//	%H  hour, 24-hour clock             %I  hour, 12-hour clock
//	%j  day of the year [001,366]       %m  month [01,12]
//	%M  minute [00,59]                  %S  second [00,61]
//	%p  AM or PM                        %q  quarter [1,4]
//	%Q  milliseconds since the epoch    %s  seconds since the epoch
//	%u  ISO weekday [1,7], Monday first %w  weekday [0,6], Sunday first
//	%U  week of year, Sunday-based      %W  week of year, Monday-based
//	%V  ISO week of year [01,53]
//	%y  year without century            %Y  year with century
//	%Z  time zone offset                %%  a literal percent
//
// A directive may carry a padding modifier between the percent and the letter:
// "-" for no padding, "_" for spaces, "0" for zeros. "%-d" is the day of the
// month with no leading zero, which is what most date formats actually want.
// An unrecognized directive is emitted literally.
//
// # Where Go helps, and where it does not
//
// Go makes the ISO week trivial: [time.Time.ISOWeek] already implements the
// Thursday rule that %G, %g and %V need, where d3 has to construct it out of
// interval arithmetic. Years before 100 are also easier — [time.Date] takes the
// year as an int with no two-digit special case, whereas JavaScript's Date
// constructor maps 0..99 onto 1900..1999 and d3 has to work around it.
//
// Time zones are the hard part, and the difference is worth understanding. A
// JavaScript Date is an instant plus one ambient zone, so d3 needs two copies of
// everything: timeDay and utcDay, timeFormat and utcFormat. A [time.Time]
// carries its own [time.Location], so this port instead makes the local family
// operate *in the location of the value it is given* — [Day].Floor of a Tokyo
// timestamp is Tokyo midnight — and the UTC family convert to UTC first. That
// is strictly more useful than d3's version and it means nothing here reads
// time.Local implicitly. The one place Go is genuinely harder is a clock time
// that does not exist (the hour skipped by a spring-forward transition):
// [time.Date] normalizes it to a real instant, but which one is not part of
// Go's contract, so Floor on such a day lands at 01:00 rather than 00:00 —
// the same answer JavaScript gives, arrived at less deliberately.
//
// # Precision
//
// d3 works to the millisecond, JavaScript's Date resolution. This package keeps
// nanoseconds: %f formats and parses true microseconds rather than milliseconds
// multiplied by a thousand, so "%H:%M:%S.%f" round-trips exactly. %L is
// milliseconds either way.
package timefmt

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TimeLocale carries the names and the date/time patterns that make a format
// locale-specific. The zero value is not usable; pass it through
// [NewTimeLocale], which substitutes the en-US defaults for empty fields.
type TimeLocale struct {
	// DateTime is the pattern %c expands to, e.g. "%x, %X".
	DateTime string
	// Date is the pattern %x expands to, e.g. "%-m/%-d/%Y".
	Date string
	// Time is the pattern %X expands to, e.g. "%-I:%M:%S %p".
	Time string
	// Periods is the AM/PM pair, used by %p.
	Periods [2]string
	// Days and ShortDays are the weekday names starting at Sunday.
	Days      [7]string
	ShortDays [7]string
	// Months and ShortMonths are the month names starting at January.
	Months      [12]string
	ShortMonths [12]string
}

// EnUS is the default locale. It is a fixed table, not a lookup into the host
// system: the same specifier produces the same bytes on every machine, which is
// what a chart that is rendered on a server and compared in a test needs.
var EnUS = NewTimeLocale(TimeLocale{})

var enUSDefaults = TimeLocale{
	DateTime:    "%x, %X",
	Date:        "%-m/%-d/%Y",
	Time:        "%-I:%M:%S %p",
	Periods:     [2]string{"AM", "PM"},
	Days:        [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	ShortDays:   [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	Months:      [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	ShortMonths: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
}

// NewTimeLocale returns a ready-to-use locale, filling in en-US defaults for
// every field left empty.
func NewTimeLocale(l TimeLocale) *TimeLocale {
	out := l
	if out.DateTime == "" {
		out.DateTime = enUSDefaults.DateTime
	}
	if out.Date == "" {
		out.Date = enUSDefaults.Date
	}
	if out.Time == "" {
		out.Time = enUSDefaults.Time
	}
	for i := range out.Periods {
		if out.Periods[i] == "" {
			out.Periods[i] = enUSDefaults.Periods[i]
		}
	}
	for i := range out.Days {
		if out.Days[i] == "" {
			out.Days[i] = enUSDefaults.Days[i]
		}
		if out.ShortDays[i] == "" {
			out.ShortDays[i] = enUSDefaults.ShortDays[i]
		}
	}
	for i := range out.Months {
		if out.Months[i] == "" {
			out.Months[i] = enUSDefaults.Months[i]
		}
		if out.ShortMonths[i] == "" {
			out.ShortMonths[i] = enUSDefaults.ShortMonths[i]
		}
	}
	return &out
}

// ErrParse is the error class returned when a string does not match a parse
// specifier. Use errors.Is to test for it.
var ErrParse = errors.New("timefmt: parse")

// Format compiles a specifier into a formatting function using the default
// locale. The instant is rendered in its own [time.Location].
func Format(specifier string) func(time.Time) string { return EnUS.Format(specifier) }

// UTCFormat is [Format] but renders every instant in UTC, so %H is the UTC hour
// and %Z is always "+0000".
func UTCFormat(specifier string) func(time.Time) string { return EnUS.UTCFormat(specifier) }

// Parse compiles a specifier into a parsing function using the default locale.
// Fields the specifier does not mention default to January 1, 1900, 00:00:00,
// and the result is in [time.Local] unless the specifier reads a zone offset
// with %Z.
func Parse(specifier string) func(string) (time.Time, error) { return EnUS.Parse(specifier) }

// UTCParse is [Parse] but interprets unzoned fields as UTC.
func UTCParse(specifier string) func(string) (time.Time, error) { return EnUS.UTCParse(specifier) }

// Format compiles a specifier against this locale.
func (l *TimeLocale) Format(specifier string) func(time.Time) string {
	ops := l.compile(specifier, 0)
	return func(t time.Time) string { return runOps(ops, t) }
}

// UTCFormat compiles a specifier against this locale, rendering in UTC.
func (l *TimeLocale) UTCFormat(specifier string) func(time.Time) string {
	ops := l.compile(specifier, 0)
	return func(t time.Time) string { return runOps(ops, t.UTC()) }
}

// Parse compiles a parsing function that resolves unzoned fields in
// [time.Local].
func (l *TimeLocale) Parse(specifier string) func(string) (time.Time, error) {
	return l.ParseIn(specifier, time.Local)
}

// UTCParse compiles a parsing function that resolves unzoned fields in UTC.
func (l *TimeLocale) UTCParse(specifier string) func(string) (time.Time, error) {
	return l.ParseIn(specifier, time.UTC)
}

// ParseIn compiles a parsing function that resolves unzoned fields in loc. This
// has no d3 equivalent — JavaScript has only "local" and "UTC" — but it is the
// natural generalization once [time.Location] is a value you can pass around,
// and it is what you want when a data feed reports wall-clock times in a
// specific city.
func (l *TimeLocale) ParseIn(specifier string, loc *time.Location) func(string) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	return func(s string) (time.Time, error) {
		var d parts
		d.y, d.m, d.dd = 1900, 0, 1
		d.set = map[byte]bool{}
		i, err := l.parseSpec(&d, specifier, s, 0, 0)
		if err != nil {
			return time.Time{}, err
		}
		if i != len(s) {
			return time.Time{}, fmt.Errorf("%w: %q does not match %q (trailing %q)", ErrParse, s, specifier, s[i:])
		}
		return d.resolve(loc)
	}
}

// op is one compiled piece of a format specifier: either a literal run of text
// or a directive with its padding.
type op struct {
	lit string
	dir byte
	pad string
	// sub holds the compiled expansion of %c, %x and %X.
	sub []op
	// fn renders a locale-dependent directive (%a %A %b %B %p), which needs
	// the locale's name tables and so cannot be a plain switch on the byte.
	fn func(time.Time) string
}

func runOps(ops []op, t time.Time) string {
	var b strings.Builder
	for _, o := range ops {
		switch {
		case o.fn != nil:
			b.WriteString(o.fn(t))
		case o.sub != nil:
			b.WriteString(runOps(o.sub, t))
		case o.dir == 0:
			b.WriteString(o.lit)
		default:
			b.WriteString(directive(o.dir, t, o.pad))
		}
	}
	return b.String()
}

// maxDepth bounds the recursion through %c, %x and %X, so that a locale whose
// date pattern mentions %c cannot loop forever.
const maxDepth = 8

func (l *TimeLocale) compile(spec string, depth int) []op {
	var ops []op
	lit := strings.Builder{}
	flush := func() {
		if lit.Len() > 0 {
			ops = append(ops, op{lit: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(spec); i++ {
		if spec[i] != '%' {
			lit.WriteByte(spec[i])
			continue
		}
		i++
		if i >= len(spec) {
			lit.WriteByte('%')
			break
		}
		c := spec[i]
		pad, hasPad := pads[c]
		if hasPad {
			i++
			if i >= len(spec) {
				lit.WriteByte('%')
				lit.WriteByte(c)
				break
			}
			c = spec[i]
		} else if c == 'e' {
			pad = " "
		} else {
			pad = "0"
		}
		switch c {
		case 'c', 'x', 'X':
			if depth >= maxDepth {
				continue
			}
			flush()
			ops = append(ops, op{dir: c, pad: pad, sub: l.compile(l.subPattern(c), depth+1)})
			continue
		case 'a', 'A', 'b', 'B', 'p':
			flush()
			names := l.namesFor(c)
			index := nameIndexer(c)
			ops = append(ops, op{dir: c, pad: pad, fn: func(t time.Time) string {
				return names[index(t)]
			}})
			continue
		}
		if !isDirective(c) {
			// Unknown directive: emit the letter itself, as d3 does.
			lit.WriteByte(c)
			continue
		}
		flush()
		ops = append(ops, op{dir: c, pad: pad})
	}
	flush()
	return ops
}

func (l *TimeLocale) subPattern(c byte) string {
	switch c {
	case 'c':
		return l.DateTime
	case 'x':
		return l.Date
	default:
		return l.Time
	}
}

func (l *TimeLocale) namesFor(c byte) []string {
	switch c {
	case 'a':
		return l.ShortDays[:]
	case 'A':
		return l.Days[:]
	case 'b':
		return l.ShortMonths[:]
	case 'B':
		return l.Months[:]
	default:
		return l.Periods[:]
	}
}

// nameIndexer returns the function that picks which of the locale's names a
// directive wants: the weekday for %a and %A, the month for %b and %B, and the
// half of the day for %p.
func nameIndexer(c byte) func(time.Time) int {
	switch c {
	case 'a', 'A':
		return func(t time.Time) int { return int(t.Weekday()) }
	case 'b', 'B':
		return func(t time.Time) int { return int(t.Month()) - 1 }
	default:
		return func(t time.Time) int {
			if t.Hour() >= 12 {
				return 1
			}
			return 0
		}
	}
}

// isDirective reports whether c is a directive this package renders.
func isDirective(c byte) bool {
	return strings.IndexByte("aAbBcdefgGHIjLmMpqQsSuUVwWxXyYZ%", c) >= 0
}

var pads = map[byte]string{'-': "", '_': " ", '0': "0"}

// padNum renders v right-padded to width with fill, keeping any minus sign
// outside the padding. An empty fill means no padding at all.
func padNum(v int, fill string, width int) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	s := strconv.Itoa(v)
	if fill != "" {
		for len(s) < width {
			s = fill + s
		}
	}
	return sign + s
}

// directive renders one directive that does not depend on the locale.
func directive(c byte, t time.Time, pad string) string {
	switch c {
	case 'd':
		return padNum(t.Day(), pad, 2)
	case 'e':
		return padNum(t.Day(), pad, 2)
	case 'f':
		return padNum(t.Nanosecond()/1000, pad, 6)
	case 'L':
		return padNum(t.Nanosecond()/1e6, pad, 3)
	case 'H':
		return padNum(t.Hour(), pad, 2)
	case 'I':
		h := t.Hour() % 12
		if h == 0 {
			h = 12
		}
		return padNum(h, pad, 2)
	case 'j':
		return padNum(t.YearDay(), pad, 3)
	case 'm':
		return padNum(int(t.Month()), pad, 2)
	case 'M':
		return padNum(t.Minute(), pad, 2)
	case 'q':
		return strconv.Itoa((int(t.Month())-1)/3 + 1)
	case 'Q':
		return strconv.FormatInt(t.UnixMilli(), 10)
	case 's':
		return strconv.FormatInt(t.Unix(), 10)
	case 'S':
		return padNum(t.Second(), pad, 2)
	case 'u':
		w := int(t.Weekday())
		if w == 0 {
			w = 7
		}
		return strconv.Itoa(w)
	case 'w':
		return strconv.Itoa(int(t.Weekday()))
	case 'U':
		return padNum(sundayWeek(t), pad, 2)
	case 'W':
		return padNum(mondayWeek(t), pad, 2)
	case 'V':
		_, w := t.ISOWeek()
		return padNum(w, pad, 2)
	case 'g':
		y, _ := t.ISOWeek()
		return padNum(y%100, pad, 2)
	case 'G':
		y, _ := t.ISOWeek()
		return padNum(y%10000, pad, 4)
	case 'y':
		return padNum(t.Year()%100, pad, 2)
	case 'Y':
		return padNum(t.Year()%10000, pad, 4)
	case 'Z':
		return formatZone(t)
	case '%':
		return "%"
	}
	return string(c)
}

// sundayWeek and mondayWeek count the week-boundaries of the year so far. The
// year's first partial week is week zero, which is what %U and %W mean; %V is
// the ISO rule and comes from [time.Time.ISOWeek] instead.
func sundayWeek(t time.Time) int {
	return Sunday.Count(Year.Floor(t).Add(-tick), t)
}

func mondayWeek(t time.Time) int {
	return Monday.Count(Year.Floor(t).Add(-tick), t)
}

// formatZone renders the offset as ±HHMM, matching d3 (which always emits four
// digits and never a colon or "Z").
func formatZone(t time.Time) string {
	_, off := t.Zone()
	z := -off / 60 // minutes west of UTC, like JavaScript's getTimezoneOffset
	sign := "+"
	if z > 0 {
		sign = "-"
	} else {
		z = -z
	}
	return sign + padNum(z/60, "0", 2) + padNum(z%60, "0", 2)
}

// ISOSpecifier is the specifier d3's isoFormat uses.
const ISOSpecifier = "%Y-%m-%dT%H:%M:%S.%LZ"

var (
	isoFormat = EnUS.UTCFormat(ISOSpecifier)
	isoParse  = EnUS.UTCParse(ISOSpecifier)
)

// ISOFormat renders an instant as ISO 8601 in UTC, e.g.
// "2024-03-09T12:34:56.789Z".
func ISOFormat(t time.Time) string { return isoFormat(t) }

// ISOParse parses the format [ISOFormat] produces. It is strict: the whole
// string must match, milliseconds and the trailing Z included.
func ISOParse(s string) (time.Time, error) { return isoParse(s) }
