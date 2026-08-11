package scale

import (
	"math"
	"time"

	"github.com/malcolmston/d3/timefmt"
)

// Time is a continuous scale whose domain is calendar time. Underneath it is an
// ordinary linear scale over epoch milliseconds; what makes it its own type is
// tick generation, which must produce round *calendar* values — the top of an
// hour, midnight, the first of a month — rather than round numbers of
// milliseconds. Nobody wants an axis labelled every 8,640,000 ms.
//
// The interval ladder, the boundary arithmetic and the label formats all come
// from [github.com/malcolmston/d3/timefmt], this port's d3-time and
// d3-time-format; this type is only the adapter between it and the continuous
// scale machinery.
//
// [NewTime] rounds and labels in the time zone carried by the values it is
// given, and [NewUTC] in UTC. The distinction is not cosmetic: it decides where
// "midnight" and "the first of the month" fall, and therefore where the ticks
// land. Use UTC for data that is genuinely timezone-free (server logs compared
// across regions) and local time when the axis is about someone's day.
//
// The range is float64 — pixels, the overwhelmingly common case. For a time
// domain with a non-numeric range (a color ramp over time, say), use
// [NewSequential] and convert to milliseconds yourself.
type Time struct {
	core *Continuous[float64]
	utc  bool
	loc  *time.Location // the zone Invert and Domain return values in
}

// NewTime returns a time scale in local time, with a domain of 2000-01-01 to
// 2000-01-02 and the unit range, matching d3's default.
func NewTime() *Time { return newTimeIn(false, time.Local) }

// NewUTC returns a time scale in UTC. See [Time] for why the choice matters.
func NewUTC() *Time { return newTimeIn(true, time.UTC) }

func newTimeIn(utc bool, loc *time.Location) *Time {
	t := &Time{core: NewLinear(), utc: utc, loc: loc}
	d0 := time.Date(2000, time.January, 1, 0, 0, 0, 0, loc)
	return t.SetDomain(d0, d0.AddDate(0, 0, 1))
}

// Location returns the time zone instants are returned in. It follows the
// domain: setting a domain of Tokyo timestamps makes a local time scale report
// its ticks in Tokyo.
func (t *Time) Location() *time.Location { return t.loc }

// UTC reports whether the scale rounds and formats in UTC.
func (t *Time) UTC() bool { return t.utc }

// msOf converts an instant to epoch milliseconds as a float64. Milliseconds are
// the unit d3 works in, and float64 represents every millisecond exactly for
// roughly ±285,000 years, so nothing is lost at the resolutions charts use.
func msOf(x time.Time) float64 { return float64(x.UnixNano()) / 1e6 }

func (t *Time) fromMs(ms float64) time.Time {
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return time.Time{}
	}
	return time.Unix(0, int64(math.Round(ms*1e6))).In(t.loc)
}

// Scale maps an instant to a range value.
func (t *Time) Scale(x time.Time) float64 { return t.core.Scale(msOf(x)) }

// Invert maps a range value back to an instant.
func (t *Time) Invert(y float64) time.Time { return t.fromMs(t.core.Invert(y)) }

// Domain returns a copy of the domain.
func (t *Time) Domain() []time.Time {
	ms := t.core.Domain()
	out := make([]time.Time, len(ms))
	for i, v := range ms {
		out[i] = t.fromMs(v)
	}
	return out
}

// SetDomain sets the domain. More than two values make the scale polylinear,
// exactly as for [Continuous.SetDomain]. For a local-time scale the zone of the
// first value becomes the scale's zone.
func (t *Time) SetDomain(d ...time.Time) *Time {
	if !t.utc && len(d) > 0 && d[0].Location() != nil {
		t.loc = d[0].Location()
	}
	ms := make([]float64, len(d))
	for i, v := range d {
		ms[i] = msOf(v)
	}
	t.core.SetDomain(ms...)
	return t
}

// Range returns a copy of the range.
func (t *Time) Range() []float64 { return t.core.Range() }

// SetRange sets the range.
func (t *Time) SetRange(r ...float64) *Time { t.core.SetRange(r...); return t }

// SetRangeRound sets the range and rounds outputs to whole pixels.
func (t *Time) SetRangeRound(r ...float64) *Time { t.core.SetRangeRound(r...); return t }

// Clamp reports whether clamping is on.
func (t *Time) Clamp() bool { return t.core.Clamp() }

// SetClamp turns clamping on or off.
func (t *Time) SetClamp(c bool) *Time { t.core.SetClamp(c); return t }

// Unknown returns the value produced for a zero instant.
func (t *Time) Unknown() float64 { return t.core.Unknown() }

// SetUnknown sets the value produced for a zero instant.
func (t *Time) SetUnknown(v float64) *Time { t.core.SetUnknown(v); return t }

// ends returns the domain endpoints as instants, low first.
func (t *Time) ends() (time.Time, time.Time, bool) {
	d := t.core.Domain()
	if len(d) < 2 {
		return time.Time{}, time.Time{}, false
	}
	lo, hi := d[0], d[len(d)-1]
	reversed := hi < lo
	if reversed {
		lo, hi = hi, lo
	}
	return t.fromMs(lo), t.fromMs(hi), reversed
}

// Interval returns the tick interval [Time.Ticks] would choose for count ticks.
// It is exported because an axis often wants to know the granularity — to pick
// a label format, or to decide whether to draw minor ticks — without generating
// the ticks themselves.
func (t *Time) Interval(count int) *timefmt.Interval {
	lo, hi, _ := t.ends()
	if count <= 0 {
		count = 10
	}
	if t.utc {
		return timefmt.UTCTickInterval(lo, hi, count)
	}
	return timefmt.TickInterval(lo, hi, count)
}

// Ticks returns calendar-aligned ticks spanning the domain: whole seconds,
// 5-minute marks, midnights, Sundays, first-of-months or whole years, whichever
// gives roughly count ticks. The choice and the alignment are timefmt's, which
// is d3-time's.
//
// A descending domain yields descending ticks.
func (t *Time) Ticks(count int) []time.Time {
	lo, hi, reversed := t.ends()
	if lo.IsZero() && hi.IsZero() {
		return nil
	}
	if count <= 0 {
		count = 10
	}
	var out []time.Time
	if t.utc {
		out = timefmt.UTCTicks(lo, hi, count)
	} else {
		out = timefmt.Ticks(lo, hi, count)
	}
	for i := range out {
		out[i] = out[i].In(t.loc)
	}
	if reversed {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// The d3 default time-axis label cascade. Each tick is labelled at the coarsest
// granularity that still distinguishes it from its neighbours, so a tick on the
// hour shows "03 PM", one at midnight shows "Mon 02" and one on 1 January shows
// the year. Repeating the full date on every tick would be noise; the reader
// recovers the missing context from the coarser labels further along the axis.
const (
	fmtMillisecond = ".%L"
	fmtSecond      = ":%S"
	fmtMinute      = "%I:%M"
	fmtHour        = "%I %p"
	fmtDay         = "%a %d"
	fmtWeek        = "%b %d"
	fmtMonth       = "%B"
	fmtYear        = "%Y"
)

// TickFormat returns a formatter that applies the cascade described above.
//
// The count argument is accepted for symmetry with the other scales; the
// cascade is driven by each tick's own alignment, so it is not used.
func (t *Time) TickFormat(count int) func(time.Time) string {
	compile := timefmt.Format
	if t.utc {
		compile = timefmt.UTCFormat
	}
	var (
		ms    = compile(fmtMillisecond)
		sec   = compile(fmtSecond)
		min   = compile(fmtMinute)
		hour  = compile(fmtHour)
		day   = compile(fmtDay)
		week  = compile(fmtWeek)
		month = compile(fmtMonth)
		year  = compile(fmtYear)
	)
	iSecond, iMinute, iHour, iDay, iWeek, iMonth, iYear := t.intervals()
	return func(x time.Time) string {
		switch {
		case iSecond.Floor(x).Before(x):
			return ms(x)
		case iMinute.Floor(x).Before(x):
			return sec(x)
		case iHour.Floor(x).Before(x):
			return min(x)
		case iDay.Floor(x).Before(x):
			return hour(x)
		case iMonth.Floor(x).Before(x):
			if iWeek.Floor(x).Before(x) {
				return day(x)
			}
			return week(x)
		case iYear.Floor(x).Before(x):
			return month(x)
		default:
			return year(x)
		}
	}
}

// intervals returns the interval family — local or UTC — this scale rounds by.
func (t *Time) intervals() (second, minute, hour, day, week, month, year *timefmt.Interval) {
	if t.utc {
		return timefmt.UTCSecond, timefmt.UTCMinute, timefmt.UTCHour,
			timefmt.UTCDay, timefmt.UTCSunday, timefmt.UTCMonth, timefmt.UTCYear
	}
	return timefmt.Second, timefmt.Minute, timefmt.Hour,
		timefmt.Day, timefmt.Sunday, timefmt.Month, timefmt.Year
}

// Nice extends the domain outward to the nearest boundaries of the interval
// that [Time.Ticks] would use, so an axis starts and ends on a labelled tick.
func (t *Time) Nice(count int) *Time {
	if count <= 0 {
		count = 10
	}
	return t.NiceInterval(t.Interval(count))
}

// NiceInterval extends the domain outward to the boundaries of a specific
// interval, for when the automatic choice is not the one you want (a weekly
// axis on data that happens to span three days, say).
func (t *Time) NiceInterval(iv *timefmt.Interval) *Time {
	d := t.core.Domain()
	if len(d) < 2 || iv == nil {
		return t
	}
	i0, i1 := 0, len(d)-1
	if d[i1] < d[i0] {
		i0, i1 = i1, i0
	}
	d[i0] = msOf(iv.Floor(t.fromMs(d[i0])))
	d[i1] = msOf(iv.Ceil(t.fromMs(d[i1])))
	t.core.SetDomain(d...)
	return t
}

// Copy returns an independent copy of the scale.
func (t *Time) Copy() *Time {
	c := *t
	c.core = t.core.Copy()
	return &c
}
