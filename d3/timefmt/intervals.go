package timefmt

import (
	"strconv"
	"time"
)

// This file defines the concrete intervals. Each one is a pair of a floor
// function (snap to a boundary) and an offset function (step by whole units),
// plus a count and, where one exists, the calendar field that [Interval.Every]
// filters on.
//
// The local and UTC families are generated from the same code, parameterized by
// a normalizing function: local leaves the value in whatever location it
// arrived with, UTC converts it first. See the [Interval] doc comment for why.

// zoneOf normalizes an instant into the location the interval works in.
func local(t time.Time) time.Time { return t }
func utc(t time.Time) time.Time   { return t.UTC() }

// wallSeconds returns the number of seconds of *local wall-clock time* since
// the epoch. Subtracting two of these counts calendar days across a daylight
// saving transition correctly, where subtracting the instants themselves would
// be off by an hour. It is the Go form of d3's getTimezoneOffset correction.
func wallSeconds(t time.Time) int64 {
	_, off := t.Zone()
	return t.Unix() + int64(off)
}

// Millisecond is the finest interval: it floors to a whole millisecond and
// steps by one. Its boundaries do not depend on the time zone, so there is no
// separate UTC form (UTCMillisecond is an alias).
var Millisecond = &Interval{
	name:   "millisecond",
	floor:  func(t time.Time) time.Time { return t.Truncate(time.Millisecond) },
	offset: func(t time.Time, step int) time.Time { return t.Add(time.Duration(step) * time.Millisecond) },
	count:  func(a, b time.Time) int { return int(b.Sub(a) / time.Millisecond) },
}

// UTCMillisecond is [Millisecond]; millisecond boundaries are zone-independent.
var UTCMillisecond = Millisecond

// Second floors to a whole second. Like [Millisecond] it is zone-independent,
// because every time zone offset is a whole number of seconds.
var Second = &Interval{
	name:   "second",
	floor:  func(t time.Time) time.Time { return t.Truncate(time.Second) },
	offset: func(t time.Time, step int) time.Time { return t.Add(time.Duration(step) * time.Second) },
	count:  func(a, b time.Time) int { return int(b.Sub(a) / time.Second) },
	field:  func(t time.Time) int { return t.UTC().Second() },
}

// UTCSecond is [Second]; second boundaries are zone-independent.
var UTCSecond = Second

// newMinute and newHour subtract the sub-unit parts of the *local* clock rather
// than truncating the absolute instant, which is what makes them correct in
// zones offset by 30 or 45 minutes: 05:45 in Kathmandu floors to 05:00 local,
// not to 05:15.
func newMinute(name string, zone func(time.Time) time.Time) *Interval {
	return &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			return t.Add(-(time.Duration(t.Second())*time.Second + time.Duration(t.Nanosecond())))
		},
		offset: func(t time.Time, step int) time.Time {
			return zone(t).Add(time.Duration(step) * time.Minute)
		},
		count: func(a, b time.Time) int { return int(b.Sub(a) / time.Minute) },
		field: func(t time.Time) int { return zone(t).Minute() },
	}
}

func newHour(name string, zone func(time.Time) time.Time) *Interval {
	return &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			return t.Add(-(time.Duration(t.Minute())*time.Minute +
				time.Duration(t.Second())*time.Second +
				time.Duration(t.Nanosecond())))
		},
		offset: func(t time.Time, step int) time.Time {
			return zone(t).Add(time.Duration(step) * time.Hour)
		},
		count: func(a, b time.Time) int { return int(b.Sub(a) / time.Hour) },
		field: func(t time.Time) int { return zone(t).Hour() },
	}
}

func newDay(name string, zone func(time.Time) time.Time) *Interval {
	return &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			y, m, d := t.Date()
			return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
		},
		offset: func(t time.Time, step int) time.Time {
			t = zone(t)
			y, m, d := t.Date()
			return time.Date(y, m, d+step, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		},
		count: func(a, b time.Time) int { return int((wallSeconds(b) - wallSeconds(a)) / 86400) },
		field: func(t time.Time) int { return zone(t).Day() - 1 },
	}
}

// newWeekday builds the interval whose boundaries are the midnights of one
// particular weekday. start is the weekday the week begins on, so
// newWeekday("sunday", time.Sunday, …).Floor gives the most recent Sunday.
func newWeekday(name string, start time.Weekday, zone func(time.Time) time.Time) *Interval {
	return &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			back := (int(t.Weekday()) + 7 - int(start)) % 7
			y, m, d := t.Date()
			return time.Date(y, m, d-back, 0, 0, 0, 0, t.Location())
		},
		offset: func(t time.Time, step int) time.Time {
			t = zone(t)
			y, m, d := t.Date()
			return time.Date(y, m, d+step*7, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		},
		count: func(a, b time.Time) int { return int((wallSeconds(b) - wallSeconds(a)) / (7 * 86400)) },
	}
}

func newMonth(name string, zone func(time.Time) time.Time) *Interval {
	return &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			y, m, _ := t.Date()
			return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
		},
		offset: func(t time.Time, step int) time.Time {
			t = zone(t)
			y, m, d := t.Date()
			return time.Date(y, m+time.Month(step), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		},
		count: func(a, b time.Time) int {
			a, b = zone(a), zone(b)
			return int(b.Month()) - int(a.Month()) + (b.Year()-a.Year())*12
		},
		field: func(t time.Time) int { return int(zone(t).Month()) - 1 },
	}
}

func newYear(name string, zone func(time.Time) time.Time) *Interval {
	y := &Interval{
		name: name,
		floor: func(t time.Time) time.Time {
			t = zone(t)
			return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
		},
		offset: func(t time.Time, step int) time.Time {
			t = zone(t)
			yy, m, d := t.Date()
			return time.Date(yy+step, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
		},
		count: func(a, b time.Time) int { return zone(b).Year() - zone(a).Year() },
		field: func(t time.Time) int { return zone(t).Year() },
	}
	// Year.Every must snap to multiples of the step — decades start in 1980,
	// not "ten years after whenever". The generic Every would filter on
	// year % step == 0, which happens to give the same boundaries, but this
	// form also floors correctly for a partial step and is what d3 does.
	y.every = func(step int) *Interval {
		if step < 1 {
			return nil
		}
		if step == 1 {
			return y
		}
		return &Interval{
			name: name + ".every(" + strconv.Itoa(step) + ")",
			floor: func(t time.Time) time.Time {
				t = zone(t)
				return time.Date(floorDiv(t.Year(), step)*step, time.January, 1, 0, 0, 0, 0, t.Location())
			},
			offset: func(t time.Time, s int) time.Time {
				t = zone(t)
				yy, m, d := t.Date()
				return time.Date(yy+s*step, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
			},
		}
	}
	return y
}

// floorDiv divides rounding towards negative infinity, so that year -5 with a
// step of 10 floors to -10 rather than to 0.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// The local-time intervals. Each operates in the location of the value it is
// given; see the [Interval] doc comment.
var (
	Minute = newMinute("minute", local)
	Hour   = newHour("hour", local)
	Day    = newDay("day", local)
	Month  = newMonth("month", local)
	Year   = newYear("year", local)

	// Sunday through Saturday are the seven weekday intervals; Week is an
	// alias for Sunday, matching d3's timeWeek.
	Sunday    = newWeekday("sunday", time.Sunday, local)
	Monday    = newWeekday("monday", time.Monday, local)
	Tuesday   = newWeekday("tuesday", time.Tuesday, local)
	Wednesday = newWeekday("wednesday", time.Wednesday, local)
	Thursday  = newWeekday("thursday", time.Thursday, local)
	Friday    = newWeekday("friday", time.Friday, local)
	Saturday  = newWeekday("saturday", time.Saturday, local)
	Week      = Sunday
)

// The UTC intervals, which normalize their input to UTC before doing anything.
var (
	UTCMinute = newMinute("utcMinute", utc)
	UTCHour   = newHour("utcHour", utc)
	UTCDay    = newDay("utcDay", utc)
	UTCMonth  = newMonth("utcMonth", utc)
	UTCYear   = newYear("utcYear", utc)

	UTCSunday    = newWeekday("utcSunday", time.Sunday, utc)
	UTCMonday    = newWeekday("utcMonday", time.Monday, utc)
	UTCTuesday   = newWeekday("utcTuesday", time.Tuesday, utc)
	UTCWednesday = newWeekday("utcWednesday", time.Wednesday, utc)
	UTCThursday  = newWeekday("utcThursday", time.Thursday, utc)
	UTCFriday    = newWeekday("utcFriday", time.Friday, utc)
	UTCSaturday  = newWeekday("utcSaturday", time.Saturday, utc)
	UTCWeek      = UTCSunday
)
