package timefmt

import (
	"strconv"
	"time"
)

// An Interval is a unit of calendar time — a second, a day, a month, "every
// third hour", "every Monday" — that knows how to snap an instant to its
// boundaries, step from one boundary to the next, and enumerate the boundaries
// in a span.
//
// That small vocabulary is enough to build a time axis: [TickInterval] picks an
// interval, [Interval.Range] produces the tick positions, and a [Format]
// produces their labels. It is also enough to answer everyday calendar
// questions without date arithmetic by hand — "the start of this week", "the
// first of next month", "how many days between these two dates".
//
// # Time zones
//
// d3-time has two parallel families of intervals: local ones that use the
// browser's zone, and UTC ones. Go carries the zone inside each [time.Time], so
// this port takes the more natural route: the local-time intervals operate in
// *the location of the value handed to them*, and the UTC intervals convert to
// UTC first. [Day].Floor of a Berlin timestamp is Berlin midnight; [UTCDay]
// .Floor of the same timestamp is UTC midnight. Nothing reads time.Local unless
// the caller's value does, which makes the package testable without changing
// the process time zone.
//
// This distinction only matters for intervals whose boundaries depend on the
// calendar: day, week, month, year, and (for zones offset by fractions of an
// hour) hour and minute. [Second] and [Millisecond] are the same in every zone,
// so UTCSecond and UTCMillisecond are aliases rather than separate values.
//
// # Daylight saving
//
// Floor and Ceil are calendar operations and Offset is a wall-clock one, which
// is exactly d3's split and exactly what a reader expects: [Day].Offset moves
// to the same clock time the next day even if that day was 23 or 25 hours long,
// while [Hour].Offset adds 3600 real seconds and may land on a different clock
// hour across a transition. [Day].Count likewise counts calendar days, not
// multiples of 86400 seconds.
//
// # Precision
//
// d3 works to the millisecond because that is JavaScript's Date resolution.
// [Millisecond] here floors to a whole millisecond so that it remains a genuine
// floor, but Ceil steps back by one nanosecond rather than one millisecond, so
// intervals behave correctly for values with sub-millisecond components.
type Interval struct {
	name   string
	floor  func(time.Time) time.Time
	offset func(time.Time, int) time.Time
	// count is nil for intervals produced by Filter, which have no natural
	// notion of "how many of these are there between two instants".
	count func(start, end time.Time) int
	// field extracts the calendar field an Every filter tests, e.g. the minute
	// of the hour. Nil when the interval has no such field, in which case Every
	// counts from the Unix epoch instead.
	field func(time.Time) int
	// every, when set, replaces the generic Every implementation. Only Year
	// uses it, because "every 10 years" must snap to multiples of ten rather
	// than to every tenth year counted from the epoch.
	every func(int) *Interval
}

// String returns the interval's name, e.g. "day" or "utcMonth". Filtered
// intervals report the interval they were derived from.
func (i *Interval) String() string { return i.name }

// tick is the smallest step back from an instant that is guaranteed to land in
// the previous interval. d3 uses one millisecond; Go's nanosecond resolution
// lets us be exact.
const tick = time.Nanosecond

// Floor returns the latest boundary of this interval at or before t.
func (i *Interval) Floor(t time.Time) time.Time { return i.floor(t) }

// Ceil returns the earliest boundary of this interval at or after t. An instant
// that is already a boundary is returned unchanged.
func (i *Interval) Ceil(t time.Time) time.Time {
	return i.floor(i.offset(i.floor(t.Add(-tick)), 1))
}

// Round returns the boundary nearest to t, breaking a tie towards the later
// one — the same rule d3 uses, and the one that makes "round to the nearest
// day" put noon on tomorrow.
func (i *Interval) Round(t time.Time) time.Time {
	d0, d1 := i.Floor(t), i.Ceil(t)
	if t.Sub(d0) < d1.Sub(t) {
		return d0
	}
	return d1
}

// Offset advances t by step intervals without flooring, preserving the parts of
// the value smaller than the interval. Offsetting a Tuesday afternoon by two
// days gives a Thursday afternoon. step may be negative or zero.
func (i *Interval) Offset(t time.Time, step int) time.Time { return i.offset(t, step) }

// Range returns every boundary in [start, stop), advancing step intervals at a
// time. It is the tick generator: Day.Range over a fortnight gives fourteen
// midnights.
//
// The result is empty when step is not positive or when the ceiling of start is
// not before stop. Each returned value is a boundary, so Range is safe to use
// even when start is midway through an interval.
func (i *Interval) Range(start, stop time.Time, step int) []time.Time {
	var out []time.Time
	if step <= 0 {
		return out
	}
	cur := i.Ceil(start)
	if !cur.Before(stop) {
		return out
	}
	for {
		prev := cur
		out = append(out, prev)
		cur = i.floor(i.offset(cur, step))
		// The first condition guards against an offset that fails to advance,
		// which a filtered interval can do at the edges of its domain.
		if !prev.Before(cur) || !cur.Before(stop) {
			break
		}
	}
	return out
}

// Filter returns a new interval containing only the boundaries of this one for
// which test reports true — every third hour, or only weekdays:
//
//	weekday := timefmt.Day.Filter(func(t time.Time) bool {
//		return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday
//	})
//
// The filtered interval has no Count and therefore no Every; test must be
// deterministic and must accept infinitely many instants in both directions, or
// Floor and Offset will not terminate.
func (i *Interval) Filter(test func(time.Time) bool) *Interval {
	return &Interval{
		name: i.name + "(filtered)",
		floor: func(t time.Time) time.Time {
			for {
				t = i.floor(t)
				if test(t) {
					return t
				}
				t = t.Add(-tick)
			}
		},
		offset: func(t time.Time, step int) time.Time {
			if step < 0 {
				for s := step; ; {
					s++
					if s > 0 {
						break
					}
					for {
						t = i.offset(t, -1)
						if test(t) {
							break
						}
					}
				}
			} else {
				for s := step; ; {
					s--
					if s < 0 {
						break
					}
					for {
						t = i.offset(t, 1)
						if test(t) {
							break
						}
					}
				}
			}
			return t
		},
	}
}

// Count returns the number of interval boundaries strictly after start and at
// or before end, after flooring both — d3's semantics. Day.Count(jan1, jan2) is
// 1, and Count is what makes "the 42nd week of the year" computable.
//
// It returns 0 for intervals produced by [Interval.Filter], which do not define
// a count.
func (i *Interval) Count(start, end time.Time) int {
	if i.count == nil {
		return 0
	}
	return i.count(i.floor(start), i.floor(end))
}

// Every returns an interval containing every step-th boundary of this one,
// aligned to the natural start of the larger unit rather than to an arbitrary
// origin: Minute.Every(15) gives :00, :15, :30 and :45, not fifteen minutes
// after whenever you happened to start.
//
// It returns the receiver for a step of 1 and nil for a step below 1 or for an
// interval that has no Count — callers that thread the result into
// [Interval.Range] should check for nil. Year.Every(10) snaps to decades.
func (i *Interval) Every(step int) *Interval {
	if i.every != nil {
		return i.every(step)
	}
	if step < 1 || i.count == nil {
		return nil
	}
	if step == 1 {
		return i
	}
	var out *Interval
	if i.field != nil {
		f := i.field
		out = i.Filter(func(t time.Time) bool { return f(t)%step == 0 })
	} else {
		// No calendar field to test — weeks, for instance — so count from the
		// Unix epoch instead. That is d3's fallback and it is why Week.Every(2)
		// aligns to the epoch rather than to the start of the year.
		out = i.Filter(func(t time.Time) bool {
			return i.Count(time.UnixMilli(0).In(t.Location()), t)%step == 0
		})
	}
	out.name = i.name + ".every(" + strconv.Itoa(step) + ")"
	return out
}
