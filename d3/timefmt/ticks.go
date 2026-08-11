package timefmt

import (
	"math"
	"sort"
	"time"
)

// This file is the piece a time scale needs: given a domain and a desired
// number of ticks, which calendar unit should the ticks land on?
//
// The answer cannot come from dividing the span by the count, because time is
// not decimal. Ticks every 3.7 hours are useless; ticks every 3 hours, or every
// week, or every quarter, are what a reader can actually follow. So d3 keeps a
// hand-picked ladder of "reasonable" intervals and snaps to the rung whose
// spacing is closest, geometrically, to the ideal spacing. Above a year the
// ladder runs out and the step falls back to the 1/2/5/10 progression that
// linear scales use.

// tickIntervals is the ladder, from one second to one year. The duration is
// nominal — a month is treated as 30 days and a year as 365 for the purpose of
// choosing a rung; the interval itself still uses real calendar arithmetic.
type tickEntry struct {
	interval *Interval
	step     int
	dur      float64 // nominal spacing, in milliseconds
}

const (
	durSecondMs = 1000.0
	durMinuteMs = 60 * durSecondMs
	durHourMs   = 60 * durMinuteMs
	durDayMs    = 24 * durHourMs
	durWeekMs   = 7 * durDayMs
	durMonthMs  = 30 * durDayMs
	durYearMs   = 365 * durDayMs
)

func ladder(year, month, week, day, hour, minute *Interval) []tickEntry {
	return []tickEntry{
		{Second, 1, durSecondMs},
		{Second, 5, 5 * durSecondMs},
		{Second, 15, 15 * durSecondMs},
		{Second, 30, 30 * durSecondMs},
		{minute, 1, durMinuteMs},
		{minute, 5, 5 * durMinuteMs},
		{minute, 15, 15 * durMinuteMs},
		{minute, 30, 30 * durMinuteMs},
		{hour, 1, durHourMs},
		{hour, 3, 3 * durHourMs},
		{hour, 6, 6 * durHourMs},
		{hour, 12, 12 * durHourMs},
		{day, 1, durDayMs},
		{day, 2, 2 * durDayMs},
		{week, 1, durWeekMs},
		{month, 1, durMonthMs},
		{month, 3, 3 * durMonthMs},
		{year, 1, durYearMs},
	}
}

var (
	localLadder = ladder(Year, Month, Sunday, Day, Hour, Minute)
	utcLadder   = ladder(UTCYear, UTCMonth, UTCSunday, UTCDay, UTCHour, UTCMinute)
)

// epochMillis expresses an instant as a float64 count of milliseconds since the
// Unix epoch, which is the unit d3's tick arithmetic is written in.
func epochMillis(t time.Time) float64 { return float64(t.UnixNano()) / 1e6 }

// tickStep is d3-array's 1-2-5-10 step chooser, reproduced here so that the
// above-a-year fallback matches a linear scale's. The thresholds are geometric
// midpoints — sqrt(50), sqrt(10), sqrt(2) — so the chosen step is the one whose
// ratio to the ideal step is closest to one.
func tickStep(start, stop float64, count int) float64 {
	if count <= 0 {
		return math.NaN()
	}
	step0 := math.Abs(stop-start) / float64(count)
	if step0 == 0 || math.IsInf(step0, 0) || math.IsNaN(step0) {
		return math.NaN()
	}
	step1 := math.Pow(10, math.Floor(math.Log10(step0)))
	switch e := step0 / step1; {
	case e >= math.Sqrt(50):
		step1 *= 10
	case e >= math.Sqrt(10):
		step1 *= 5
	case e >= math.Sqrt(2):
		step1 *= 2
	}
	if stop < start {
		return -step1
	}
	return step1
}

// TickInterval returns the [Interval] a time scale should place its ticks on to
// get approximately count of them between start and stop, working in the local
// calendar (that is, in the location the values carry).
//
// This is the function d3's time scales call before generating ticks, and the
// one d3/scale should call. It returns nil when no sensible interval exists —
// a non-positive count, or a degenerate domain — and callers must handle that
// by producing no ticks.
//
//	iv := timefmt.TickInterval(start, stop, 10)
//	if iv != nil { ticks = iv.Range(iv.Ceil(start), stop, 1) }
//
// Or use [Ticks], which wraps exactly that and also handles a reversed domain.
func TickInterval(start, stop time.Time, count int) *Interval {
	return pickInterval(localLadder, Year, start, stop, count)
}

// UTCTickInterval is [TickInterval] in UTC, for scales whose domain should not
// follow daylight saving.
func UTCTickInterval(start, stop time.Time, count int) *Interval {
	return pickInterval(utcLadder, UTCYear, start, stop, count)
}

func pickInterval(tbl []tickEntry, year *Interval, start, stop time.Time, count int) *Interval {
	if count <= 0 {
		return nil
	}
	a, b := epochMillis(start), epochMillis(stop)
	target := math.Abs(b-a) / float64(count)
	if target == 0 || math.IsNaN(target) || math.IsInf(target, 0) {
		return nil
	}
	i := sort.Search(len(tbl), func(k int) bool { return tbl[k].dur > target })
	switch {
	case i == len(tbl):
		// Longer than a year: fall back to the 1-2-5-10 ladder, in years.
		step := tickStep(a/durYearMs, b/durYearMs, count)
		if math.IsNaN(step) {
			return nil
		}
		return year.Every(int(step))
	case i == 0:
		// Shorter than a second: fall back to whole milliseconds.
		step := tickStep(a, b, count)
		if math.IsNaN(step) {
			return nil
		}
		return Millisecond.Every(maxInt(int(step), 1))
	default:
		// Pick whichever neighbouring rung is the closer geometric fit.
		e := tbl[i]
		if target/tbl[i-1].dur < tbl[i].dur/target {
			e = tbl[i-1]
		}
		return e.interval.Every(e.step)
	}
}

// Ticks returns approximately count evenly spaced, calendar-aligned instants
// covering [start, stop] inclusive. A reversed domain yields reversed ticks.
func Ticks(start, stop time.Time, count int) []time.Time {
	return genTicks(localLadder, Year, start, stop, count)
}

// UTCTicks is [Ticks] in UTC.
func UTCTicks(start, stop time.Time, count int) []time.Time {
	return genTicks(utcLadder, UTCYear, start, stop, count)
}

func genTicks(tbl []tickEntry, year *Interval, start, stop time.Time, count int) []time.Time {
	reverse := stop.Before(start)
	if reverse {
		start, stop = stop, start
	}
	iv := pickInterval(tbl, year, start, stop, count)
	if iv == nil {
		return nil
	}
	// stop is inclusive, so nudge it past the last boundary.
	out := iv.Range(start, stop.Add(tick), 1)
	if reverse {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
