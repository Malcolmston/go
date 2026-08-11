package timefmt

import (
	"testing"
	"time"
)

func utcDate(y int, m time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, m, d, h, mi, s, 0, time.UTC)
}

// mustLoad returns a named location or skips the test. A machine without the
// tzdata database cannot exercise daylight saving, and that is not a failure of
// this package.
func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %q unavailable: %v", name, err)
	}
	return loc
}

// TestFloor pins the boundary each interval snaps to. Every case is a fact
// about the calendar, so these are value assertions.
func TestFloor(t *testing.T) {
	tm := time.Date(2024, 3, 9, 14, 5, 6, 123456789, time.UTC)
	tests := []struct {
		iv   *Interval
		want time.Time
	}{
		{Millisecond, time.Date(2024, 3, 9, 14, 5, 6, 123000000, time.UTC)},
		{Second, utcDate(2024, 3, 9, 14, 5, 6)},
		{Minute, utcDate(2024, 3, 9, 14, 5, 0)},
		{Hour, utcDate(2024, 3, 9, 14, 0, 0)},
		{Day, utcDate(2024, 3, 9, 0, 0, 0)},
		{Sunday, utcDate(2024, 3, 3, 0, 0, 0)}, // March 9, 2024 was a Saturday
		{Monday, utcDate(2024, 3, 4, 0, 0, 0)},
		{Saturday, utcDate(2024, 3, 9, 0, 0, 0)},
		{Friday, utcDate(2024, 3, 8, 0, 0, 0)},
		{Month, utcDate(2024, 3, 1, 0, 0, 0)},
		{Year, utcDate(2024, 1, 1, 0, 0, 0)},
	}
	for _, tt := range tests {
		if got := tt.iv.Floor(tm); !got.Equal(tt.want) {
			t.Errorf("%s.Floor = %v, want %v", tt.iv, got, tt.want)
		}
	}
}

// TestWeekdayIntervalsStartOnTheRightDay: each of the seven weekday intervals
// must floor to its own weekday, and to a day at most six days back.
func TestWeekdayIntervalsStartOnTheRightDay(t *testing.T) {
	intervals := []struct {
		iv  *Interval
		day time.Weekday
	}{
		{Sunday, time.Sunday},
		{Monday, time.Monday},
		{Tuesday, time.Tuesday},
		{Wednesday, time.Wednesday},
		{Thursday, time.Thursday},
		{Friday, time.Friday},
		{Saturday, time.Saturday},
	}
	for _, in := range intervals {
		for d := 0; d < 14; d++ {
			tm := utcDate(2024, 3, 1+d, 13, 0, 0)
			got := in.iv.Floor(tm)
			if got.Weekday() != in.day {
				t.Errorf("%s.Floor(%v) = %v (%v), want a %v", in.iv, tm, got, got.Weekday(), in.day)
			}
			if got.After(tm) || tm.Sub(got) >= 7*24*time.Hour {
				t.Errorf("%s.Floor(%v) = %v is not the most recent one", in.iv, tm, got)
			}
			if h, m, s := got.Clock(); h|m|s != 0 {
				t.Errorf("%s.Floor(%v) = %v is not midnight", in.iv, tm, got)
			}
		}
	}
	if Week != Sunday {
		t.Error("Week should be an alias for Sunday, matching d3's timeWeek")
	}
}

// TestCeilIsIdempotentOnBoundaries: Ceil of a boundary is that boundary, and
// Ceil of anything else is strictly later. This is the property that makes
// Range safe to call with an arbitrary start.
func TestCeilIsIdempotentOnBoundaries(t *testing.T) {
	intervals := []*Interval{Millisecond, Second, Minute, Hour, Day, Sunday, Monday, Month, Year}
	tm := time.Date(2024, 3, 9, 14, 5, 6, 123456789, time.UTC)
	for _, iv := range intervals {
		f := iv.Floor(tm)
		if got := iv.Ceil(f); !got.Equal(f) {
			t.Errorf("%s.Ceil(boundary %v) = %v, want the boundary itself", iv, f, got)
		}
		c := iv.Ceil(tm)
		if !c.After(tm) && !c.Equal(tm) {
			t.Errorf("%s.Ceil(%v) = %v, which is earlier", iv, tm, c)
		}
		if !iv.Floor(c).Equal(c) {
			t.Errorf("%s.Ceil(%v) = %v is not a boundary", iv, tm, c)
		}
	}
}

// TestMonthAndYearLengths: flooring and offsetting must respect the fact that
// months and years have different lengths, including February in a leap year
// and in a century that is not one.
func TestMonthAndYearLengths(t *testing.T) {
	tests := []struct {
		from time.Time
		want time.Time
	}{
		{utcDate(2024, 1, 31, 0, 0, 0), utcDate(2024, 3, 2, 0, 0, 0)}, // Jan 31 + 1 month overflows February
		{utcDate(2024, 2, 29, 0, 0, 0), utcDate(2024, 3, 29, 0, 0, 0)},
		{utcDate(2024, 12, 15, 0, 0, 0), utcDate(2025, 1, 15, 0, 0, 0)},
	}
	for _, tt := range tests {
		if got := Month.Offset(tt.from, 1); !got.Equal(tt.want) {
			t.Errorf("Month.Offset(%v, 1) = %v, want %v", tt.from, got, tt.want)
		}
	}
	// Day counts across a leap February and a non-leap one.
	if got := Day.Count(utcDate(2024, 2, 1, 0, 0, 0), utcDate(2024, 3, 1, 0, 0, 0)); got != 29 {
		t.Errorf("days in February 2024 = %d, want 29", got)
	}
	if got := Day.Count(utcDate(2023, 2, 1, 0, 0, 0), utcDate(2023, 3, 1, 0, 0, 0)); got != 28 {
		t.Errorf("days in February 2023 = %d, want 28", got)
	}
	if got := Day.Count(utcDate(1900, 1, 1, 0, 0, 0), utcDate(1901, 1, 1, 0, 0, 0)); got != 365 {
		t.Errorf("days in 1900 (not a leap year) = %d, want 365", got)
	}
	if got := Day.Count(utcDate(2000, 1, 1, 0, 0, 0), utcDate(2001, 1, 1, 0, 0, 0)); got != 366 {
		t.Errorf("days in 2000 (a leap year) = %d, want 366", got)
	}
	// Feb 29 + 1 year lands on March 1 of a non-leap year, as JavaScript and
	// Go both normalize it.
	if got := Year.Offset(utcDate(2024, 2, 29, 0, 0, 0), 1); !got.Equal(utcDate(2025, 3, 1, 0, 0, 0)) {
		t.Errorf("Year.Offset(Feb 29 2024, 1) = %v", got)
	}
}

// TestDaylightSaving is the reason Floor and Offset are defined the way they
// are. On a spring-forward day the local day is 23 hours long: Day.Offset must
// still land on the same clock time tomorrow, while Hour.Offset adds real time.
func TestDaylightSaving(t *testing.T) {
	ny := mustLoad(t, "America/New_York")
	// 2024-03-10: clocks jump from 02:00 EST to 03:00 EDT.
	afternoon := time.Date(2024, 3, 10, 13, 30, 0, 0, ny)

	if got := Day.Floor(afternoon); !got.Equal(time.Date(2024, 3, 10, 0, 0, 0, 0, ny)) {
		t.Errorf("Day.Floor on a spring-forward day = %v", got)
	}
	if got := Day.Ceil(afternoon); !got.Equal(time.Date(2024, 3, 11, 0, 0, 0, 0, ny)) {
		t.Errorf("Day.Ceil on a spring-forward day = %v", got)
	}
	// The short day really is 23 hours long.
	if d := Day.Ceil(afternoon).Sub(Day.Floor(afternoon)); d != 23*time.Hour {
		t.Errorf("spring-forward day lasted %v, want 23h", d)
	}
	// ...and the autumn day is 25.
	fall := time.Date(2024, 11, 3, 13, 30, 0, 0, ny)
	if d := Day.Ceil(fall).Sub(Day.Floor(fall)); d != 25*time.Hour {
		t.Errorf("fall-back day lasted %v, want 25h", d)
	}
	// Day.Offset is a calendar operation: same wall clock, next day.
	next := Day.Offset(afternoon, 1)
	if h, m, _ := next.Clock(); h != 13 || m != 30 {
		t.Errorf("Day.Offset kept clock %02d:%02d, want 13:30", h, m)
	}
	// Hour.Offset is a wall-clock one: 3 real hours from midnight lands at
	// 04:00 local, because one hour of the clock disappeared.
	if got := Hour.Offset(Day.Floor(afternoon), 3); got.Hour() != 4 {
		t.Errorf("Hour.Offset(midnight, 3) = %v, want 04:00 local", got)
	}
	// Day.Count still counts calendar days across the transition.
	if got := Day.Count(time.Date(2024, 3, 1, 0, 0, 0, 0, ny), time.Date(2024, 4, 1, 0, 0, 0, 0, ny)); got != 31 {
		t.Errorf("days in March 2024 (New York) = %d, want 31", got)
	}
	// Hour.Count does not: March 10 has 23 of them.
	if got := Hour.Count(time.Date(2024, 3, 10, 0, 0, 0, 0, ny), time.Date(2024, 3, 11, 0, 0, 0, 0, ny)); got != 23 {
		t.Errorf("hours in 2024-03-10 (New York) = %d, want 23", got)
	}
	// Day.Range over a DST week gives seven midnights, all at hour zero.
	days := Day.Range(time.Date(2024, 3, 8, 0, 0, 0, 0, ny), time.Date(2024, 3, 15, 0, 0, 0, 0, ny), 1)
	if len(days) != 7 {
		t.Fatalf("Day.Range across DST gave %d days, want 7", len(days))
	}
	for _, d := range days {
		if d.Hour() != 0 {
			t.Errorf("Day.Range produced %v, which is not local midnight", d)
		}
	}
}

// TestHalfHourZone: flooring by hour and minute must use the local clock, not
// the absolute instant, or zones offset by 30 or 45 minutes come out wrong.
func TestHalfHourZone(t *testing.T) {
	kolkata := mustLoad(t, "Asia/Kolkata") // UTC+05:30
	tm := time.Date(2024, 3, 9, 14, 45, 30, 0, kolkata)
	if got := Hour.Floor(tm); got.Minute() != 0 || got.Hour() != 14 {
		t.Errorf("Hour.Floor in a +05:30 zone = %v, want 14:00", got)
	}
	kathmandu := mustLoad(t, "Asia/Kathmandu") // UTC+05:45
	tm = time.Date(2024, 3, 9, 14, 45, 30, 0, kathmandu)
	if got := Hour.Floor(tm); got.Minute() != 0 || got.Hour() != 14 {
		t.Errorf("Hour.Floor in a +05:45 zone = %v, want 14:00", got)
	}
}

// TestLocalIntervalsUseTheValuesLocation is the design decision this port makes
// differently from d3: there is no ambient "local" zone.
func TestLocalIntervalsUseTheValuesLocation(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")
	tm := time.Date(2024, 3, 9, 22, 0, 0, 0, time.UTC) // already March 10 in Tokyo
	if got := Day.Floor(tm.In(tokyo)); got.Day() != 10 {
		t.Errorf("Day.Floor of a Tokyo value = %v, want March 10 Tokyo midnight", got)
	}
	if got := UTCDay.Floor(tm.In(tokyo)); got.Day() != 9 {
		t.Errorf("UTCDay.Floor = %v, want March 9 UTC midnight", got)
	}
	if got := Day.Floor(tm); got.Day() != 9 {
		t.Errorf("Day.Floor of a UTC value = %v, want March 9", got)
	}
}

// TestRound: ties go to the later boundary.
func TestRound(t *testing.T) {
	if got := Day.Round(utcDate(2024, 3, 9, 11, 59, 0)); !got.Equal(utcDate(2024, 3, 9, 0, 0, 0)) {
		t.Errorf("Day.Round(11:59) = %v", got)
	}
	if got := Day.Round(utcDate(2024, 3, 9, 12, 0, 0)); !got.Equal(utcDate(2024, 3, 10, 0, 0, 0)) {
		t.Errorf("Day.Round(noon) = %v, want the later boundary", got)
	}
	if got := Hour.Round(utcDate(2024, 3, 9, 14, 29, 0)); !got.Equal(utcDate(2024, 3, 9, 14, 0, 0)) {
		t.Errorf("Hour.Round(:29) = %v", got)
	}
}

// TestRange: boundaries only, half-open, and empty for a non-positive step.
func TestRange(t *testing.T) {
	got := Day.Range(utcDate(2024, 3, 1, 6, 0, 0), utcDate(2024, 3, 5, 0, 0, 0), 1)
	want := []time.Time{
		utcDate(2024, 3, 2, 0, 0, 0),
		utcDate(2024, 3, 3, 0, 0, 0),
		utcDate(2024, 3, 4, 0, 0, 0),
	}
	if len(got) != len(want) {
		t.Fatalf("Day.Range = %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("Day.Range[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if r := Day.Range(utcDate(2024, 3, 1, 0, 0, 0), utcDate(2024, 3, 5, 0, 0, 0), 0); len(r) != 0 {
		t.Errorf("Range with step 0 = %v, want empty", r)
	}
	if r := Day.Range(utcDate(2024, 3, 5, 0, 0, 0), utcDate(2024, 3, 1, 0, 0, 0), 1); len(r) != 0 {
		t.Errorf("Range with stop before start = %v, want empty", r)
	}
	// A step of two skips every other boundary.
	if r := Day.Range(utcDate(2024, 3, 1, 0, 0, 0), utcDate(2024, 3, 8, 0, 0, 0), 2); len(r) != 4 {
		t.Errorf("Day.Range step 2 = %v, want 4 entries", r)
	}
	// Months of a year.
	if r := Month.Range(utcDate(2024, 1, 1, 0, 0, 0), utcDate(2025, 1, 1, 0, 0, 0), 1); len(r) != 12 {
		t.Errorf("Month.Range over a year = %d entries, want 12", len(r))
	}
}

// TestEvery: the filtered boundaries are a subset of the originals, aligned to
// the natural start of the larger unit.
func TestEvery(t *testing.T) {
	tests := []struct {
		iv    *Interval
		step  int
		start time.Time
		stop  time.Time
		want  []int // the field value at each boundary
		field func(time.Time) int
	}{
		{Minute, 15, utcDate(2024, 3, 9, 14, 0, 0), utcDate(2024, 3, 9, 15, 0, 0),
			[]int{0, 15, 30, 45}, func(t time.Time) int { return t.Minute() }},
		{Hour, 6, utcDate(2024, 3, 9, 0, 0, 0), utcDate(2024, 3, 10, 0, 0, 0),
			[]int{0, 6, 12, 18}, func(t time.Time) int { return t.Hour() }},
		{Month, 3, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2025, 1, 1, 0, 0, 0),
			[]int{1, 4, 7, 10}, func(t time.Time) int { return int(t.Month()) }},
		{Second, 15, utcDate(2024, 3, 9, 14, 0, 0), utcDate(2024, 3, 9, 14, 1, 0),
			[]int{0, 15, 30, 45}, func(t time.Time) int { return t.Second() }},
	}
	for _, tt := range tests {
		iv := tt.iv.Every(tt.step)
		if iv == nil {
			t.Errorf("%s.Every(%d) = nil", tt.iv, tt.step)
			continue
		}
		got := iv.Range(tt.start, tt.stop, 1)
		if len(got) != len(tt.want) {
			t.Errorf("%s.Every(%d).Range = %v, want %d entries", tt.iv, tt.step, got, len(tt.want))
			continue
		}
		for i, w := range tt.want {
			if tt.field(got[i]) != w {
				t.Errorf("%s.Every(%d)[%d] = %v, want field %d", tt.iv, tt.step, i, got[i], w)
			}
		}
	}
	// Year.Every snaps to multiples, so decades start in 1980 rather than 1987.
	if got := Year.Every(10).Floor(utcDate(1987, 5, 1, 0, 0, 0)); !got.Equal(utcDate(1980, 1, 1, 0, 0, 0)) {
		t.Errorf("Year.Every(10).Floor(1987) = %v, want 1980", got)
	}
	if got := Year.Every(100).Floor(utcDate(1987, 5, 1, 0, 0, 0)); !got.Equal(utcDate(1900, 1, 1, 0, 0, 0)) {
		t.Errorf("Year.Every(100).Floor(1987) = %v, want 1900", got)
	}
	// Degenerate steps.
	if Year.Every(1) != Year {
		t.Error("Every(1) should return the interval itself")
	}
	if Day.Every(1) != Day {
		t.Error("Every(1) should return the interval itself")
	}
	for _, step := range []int{0, -1} {
		if got := Day.Every(step); got != nil {
			t.Errorf("Day.Every(%d) = %v, want nil", step, got)
		}
	}
}

// TestFilter builds a business-day interval, the canonical use of Filter.
func TestFilter(t *testing.T) {
	weekday := Day.Filter(func(t time.Time) bool {
		return t.Weekday() != time.Saturday && t.Weekday() != time.Sunday
	})
	// March 9-10, 2024 is a weekend; flooring inside it lands on the Friday.
	if got := weekday.Floor(utcDate(2024, 3, 10, 12, 0, 0)); !got.Equal(utcDate(2024, 3, 8, 0, 0, 0)) {
		t.Errorf("weekday.Floor(Sunday) = %v, want the previous Friday", got)
	}
	// A full week yields five business days.
	got := weekday.Range(utcDate(2024, 3, 4, 0, 0, 0), utcDate(2024, 3, 11, 0, 0, 0), 1)
	if len(got) != 5 {
		t.Errorf("business days in a week = %d (%v), want 5", len(got), got)
	}
	for _, d := range got {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			t.Errorf("filtered range contains a weekend day: %v", d)
		}
	}
	// Offsetting skips the weekend in both directions.
	if fri := weekday.Offset(utcDate(2024, 3, 8, 0, 0, 0), 1); !fri.Equal(utcDate(2024, 3, 11, 0, 0, 0)) {
		t.Errorf("weekday.Offset(Friday, 1) = %v, want Monday", fri)
	}
	if mon := weekday.Offset(utcDate(2024, 3, 11, 0, 0, 0), -1); !mon.Equal(utcDate(2024, 3, 8, 0, 0, 0)) {
		t.Errorf("weekday.Offset(Monday, -1) = %v, want Friday", mon)
	}
	// A filtered interval has no count, and therefore no Every.
	if weekday.Count(utcDate(2024, 3, 1, 0, 0, 0), utcDate(2024, 3, 8, 0, 0, 0)) != 0 {
		t.Error("filtered Count should be 0")
	}
	if weekday.Every(2) != nil {
		t.Error("filtered Every should be nil")
	}
}

// TestCount over each interval, against counts that follow from the calendar.
func TestCount(t *testing.T) {
	tests := []struct {
		iv          *Interval
		start, stop time.Time
		want        int
	}{
		{Millisecond, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 1, 1, 0, 0, 1), 1000},
		{Second, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 1, 1, 0, 1, 0), 60},
		{Minute, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 1, 1, 1, 0, 0), 60},
		{Hour, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 1, 2, 0, 0, 0), 24},
		{Day, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 1, 8, 0, 0, 0), 7},
		{Sunday, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2024, 12, 31, 0, 0, 0), 52},
		{Month, utcDate(2024, 1, 1, 0, 0, 0), utcDate(2025, 1, 1, 0, 0, 0), 12},
		{Year, utcDate(2000, 1, 1, 0, 0, 0), utcDate(2024, 1, 1, 0, 0, 0), 24},
	}
	for _, tt := range tests {
		if got := tt.iv.Count(tt.start, tt.stop); got != tt.want {
			t.Errorf("%s.Count = %d, want %d", tt.iv, got, tt.want)
		}
	}
}

// TestUTCFamilyMatchesLocalInUTC: for a value already in UTC the two families
// must agree, which is the sanity check that they were generated from the same
// code.
func TestUTCFamilyMatchesLocalInUTC(t *testing.T) {
	pairs := []struct{ a, b *Interval }{
		{Minute, UTCMinute}, {Hour, UTCHour}, {Day, UTCDay},
		{Sunday, UTCSunday}, {Monday, UTCMonday}, {Thursday, UTCThursday},
		{Month, UTCMonth}, {Year, UTCYear},
	}
	tm := time.Date(2024, 3, 9, 14, 5, 6, 0, time.UTC)
	for _, p := range pairs {
		if !p.a.Floor(tm).Equal(p.b.Floor(tm)) {
			t.Errorf("%s and %s disagree on a UTC value", p.a, p.b)
		}
		if !p.a.Offset(tm, 3).Equal(p.b.Offset(tm, 3)) {
			t.Errorf("%s and %s disagree on Offset", p.a, p.b)
		}
	}
}

// TestOffsetIsReversible for every interval and a range of steps.
func TestOffsetIsReversible(t *testing.T) {
	tm := utcDate(2024, 3, 9, 14, 5, 6)
	for _, iv := range []*Interval{Millisecond, Second, Minute, Hour, Day, Sunday, Month, Year} {
		for _, step := range []int{-13, -1, 0, 1, 7, 100} {
			if got := iv.Offset(iv.Offset(tm, step), -step); !got.Equal(tm) {
				t.Errorf("%s.Offset by %d then -%d = %v, want %v", iv, step, step, got, tm)
			}
		}
	}
}
