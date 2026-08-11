package scale

import (
	"testing"
	"time"

	"github.com/malcolmston/d3/timefmt"
)

func utc(y int, mo time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC)
}

func TestTimeScaleMapping(t *testing.T) {
	d0 := utc(2000, time.January, 1, 0, 0, 0)
	d1 := utc(2000, time.January, 2, 0, 0, 0)
	s := NewUTC().SetDomain(d0, d1).SetRange(0, 960)

	closeTo(t, s.Scale(d0), 0, "time low endpoint")
	closeTo(t, s.Scale(d1), 960, "time high endpoint")
	closeTo(t, s.Scale(utc(2000, time.January, 1, 12, 0, 0)), 480, "time midpoint")
	// Like every continuous scale, it extrapolates.
	closeTo(t, s.Scale(utc(2000, time.January, 3, 0, 0, 0)), 1920, "time extrapolates")

	// Invert round-trips to the millisecond.
	for _, x := range []time.Time{d0, d1, utc(2000, time.January, 1, 7, 23, 11)} {
		got := s.Invert(s.Scale(x))
		if !got.Equal(x) {
			t.Errorf("Invert(Scale(%v)) = %v", x, got)
		}
	}

	// Clamping applies as usual.
	s.SetClamp(true)
	closeTo(t, s.Scale(utc(2000, time.January, 3, 0, 0, 0)), 960, "clamped time scale")
}

// TestTimeTickIntervals is the heart of a time axis: the interval chosen must be
// a unit people recognize on a clock or calendar, and it must scale with the
// span of the domain. The ladder itself lives in timefmt; what is tested here is
// that the scale hands it the right span and count.
func TestTimeTickIntervals(t *testing.T) {
	tests := []struct {
		name         string
		start, end   time.Time
		count        int
		wantInterval string
		wantTicks    int
	}{
		{
			name:  "half a minute picks 5-second ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2000, time.January, 1, 0, 0, 30),
			count: 10, wantInterval: "second.every(5)", wantTicks: 7,
		},
		{
			name:  "ten minutes picks minute ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2000, time.January, 1, 0, 10, 0),
			count: 10, wantInterval: "utcMinute", wantTicks: 11,
		},
		{
			name:  "a day picks 3-hour ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2000, time.January, 2, 0, 0, 0),
			count: 10, wantInterval: "utcHour.every(3)", wantTicks: 9,
		},
		{
			name:  "ten days picks day ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2000, time.January, 11, 0, 0, 0),
			count: 10, wantInterval: "utcDay", wantTicks: 11,
		},
		{
			name:  "ten weeks picks week ticks",
			start: utc(2000, time.January, 2, 0, 0, 0),
			end:   utc(2000, time.March, 12, 0, 0, 0),
			count: 10, wantInterval: "utcSunday", wantTicks: 11,
		},
		{
			name:  "a year picks month ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2001, time.January, 1, 0, 0, 0),
			count: 12, wantInterval: "utcMonth", wantTicks: 13,
		},
		{
			name:  "a decade picks year ticks",
			start: utc(2000, time.January, 1, 0, 0, 0),
			end:   utc(2010, time.January, 1, 0, 0, 0),
			count: 10, wantInterval: "utcYear", wantTicks: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewUTC().SetDomain(tt.start, tt.end)
			iv := s.Interval(tt.count)
			if got := iv.String(); got != tt.wantInterval {
				t.Errorf("Interval(%d) = %q, want %q", tt.count, got, tt.wantInterval)
			}

			ticks := s.Ticks(tt.count)
			if len(ticks) != tt.wantTicks {
				t.Errorf("Ticks(%d) returned %d values, want %d: %v", tt.count, len(ticks), tt.wantTicks, ticks)
			}
			for i, x := range ticks {
				if x.Before(tt.start) || x.After(tt.end) {
					t.Errorf("tick %v outside the domain", x)
				}
				if i > 0 && !x.After(ticks[i-1]) {
					t.Errorf("ticks not ascending at %d: %v", i, ticks)
				}
				if !iv.Floor(x).Equal(x) {
					t.Errorf("tick %v is not aligned to the interval", x)
				}
			}
		})
	}
}

func TestTimeTicksExactVector(t *testing.T) {
	s := NewUTC().
		SetDomain(utc(2000, time.January, 1, 0, 0, 0), utc(2000, time.January, 2, 0, 0, 0))
	got := s.Ticks(10)

	// A one-day domain at ten ticks resolves to every three hours, inclusive of
	// both endpoints — nine ticks.
	want := []time.Time{
		utc(2000, time.January, 1, 0, 0, 0),
		utc(2000, time.January, 1, 3, 0, 0),
		utc(2000, time.January, 1, 6, 0, 0),
		utc(2000, time.January, 1, 9, 0, 0),
		utc(2000, time.January, 1, 12, 0, 0),
		utc(2000, time.January, 1, 15, 0, 0),
		utc(2000, time.January, 1, 18, 0, 0),
		utc(2000, time.January, 1, 21, 0, 0),
		utc(2000, time.January, 2, 0, 0, 0),
	}
	if len(got) != len(want) {
		t.Fatalf("Ticks(10) = %v (%d), want %d", got, len(got), len(want))
	}
	for i := range got {
		if !got[i].Equal(want[i]) {
			t.Errorf("Ticks(10)[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestTimeTicksReversedDomain(t *testing.T) {
	s := NewUTC().
		SetDomain(utc(2000, time.January, 2, 0, 0, 0), utc(2000, time.January, 1, 0, 0, 0))
	got := s.Ticks(10)
	if len(got) < 2 {
		t.Fatalf("Ticks = %v", got)
	}
	for i := 1; i < len(got); i++ {
		if !got[i].Before(got[i-1]) {
			t.Fatalf("a descending domain should give descending ticks: %v", got)
		}
	}
	// A reversed domain must still scale correctly.
	r := NewUTC().
		SetDomain(utc(2000, time.January, 2, 0, 0, 0), utc(2000, time.January, 1, 0, 0, 0)).
		SetRange(0, 100)
	closeTo(t, r.Scale(utc(2000, time.January, 2, 0, 0, 0)), 0, "reversed time low")
	closeTo(t, r.Scale(utc(2000, time.January, 1, 0, 0, 0)), 100, "reversed time high")
}

func TestTimeNice(t *testing.T) {
	tests := []struct {
		name       string
		start, end time.Time
		count      int
		wantStart  time.Time
		wantEnd    time.Time
	}{
		{
			name:  "a day rounds to 3-hour boundaries",
			start: utc(2000, time.January, 1, 0, 17, 0),
			end:   utc(2000, time.January, 1, 23, 42, 0),
			count: 10,
			// 3-hour ticks: 00:17 floors to 00:00, 23:42 ceils to the next day.
			wantStart: utc(2000, time.January, 1, 0, 0, 0),
			wantEnd:   utc(2000, time.January, 2, 0, 0, 0),
		},
		{
			name:      "already aligned is left alone",
			start:     utc(2000, time.January, 1, 0, 0, 0),
			end:       utc(2000, time.January, 2, 0, 0, 0),
			count:     10,
			wantStart: utc(2000, time.January, 1, 0, 0, 0),
			wantEnd:   utc(2000, time.January, 2, 0, 0, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewUTC().SetDomain(tt.start, tt.end).Nice(tt.count).Domain()
			if !d[0].Equal(tt.wantStart) || !d[1].Equal(tt.wantEnd) {
				t.Errorf("Nice = [%v %v], want [%v %v]", d[0], d[1], tt.wantStart, tt.wantEnd)
			}
			// Nice must never narrow the domain.
			if d[0].After(tt.start) || d[1].Before(tt.end) {
				t.Error("Nice narrowed the domain")
			}
		})
	}
}

func TestTimeNiceInterval(t *testing.T) {
	s := NewUTC().
		SetDomain(utc(2000, time.January, 5, 13, 0, 0), utc(2000, time.January, 9, 4, 0, 0)).
		NiceInterval(timefmt.UTCMonth)
	d := s.Domain()
	want0 := utc(2000, time.January, 1, 0, 0, 0)
	want1 := utc(2000, time.February, 1, 0, 0, 0)
	if !d[0].Equal(want0) || !d[1].Equal(want1) {
		t.Errorf("NiceInterval(month) = [%v %v], want [%v %v]", d[0], d[1], want0, want1)
	}
}

// TestTimeTickFormatCascade pins the default axis labels: each tick is labelled
// at the coarsest granularity that still distinguishes it.
func TestTimeTickFormatCascade(t *testing.T) {
	f := NewUTC().TickFormat(10)
	tests := []struct {
		in   time.Time
		want string
	}{
		{utc(2000, time.January, 1, 0, 0, 0), "2000"},   // the start of a year
		{utc(2000, time.April, 1, 0, 0, 0), "April"},    // the start of a month
		{utc(2000, time.January, 5, 0, 0, 0), "Wed 05"}, // a day
		{utc(2000, time.January, 2, 0, 0, 0), "Jan 02"}, // a Sunday: a week
		{utc(2000, time.January, 1, 15, 0, 0), "03 PM"}, // an hour
		{utc(2000, time.January, 1, 15, 30, 0), "03:30"},
		{utc(2000, time.January, 1, 15, 30, 20), ":20"},
	}
	for _, tt := range tests {
		if got := f(tt.in); got != tt.want {
			t.Errorf("TickFormat(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestTimeLocalIsCalendarAware checks that a local scale rounds in the zone of
// its domain rather than in UTC — the reason [NewTime] and [NewUTC] are
// different constructors.
func TestTimeLocalIsCalendarAware(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	start := time.Date(2021, time.March, 1, 0, 0, 0, 0, loc)
	stop := time.Date(2021, time.March, 11, 0, 0, 0, 0, loc)

	local := NewTime().SetDomain(start, stop)
	if local.Location() != loc {
		t.Fatalf("Location() = %v, want %v", local.Location(), loc)
	}
	for _, x := range local.Ticks(10) {
		if h, m, s := x.Clock(); h != 0 || m != 0 || s != 0 {
			t.Errorf("local tick %v is not local midnight", x)
		}
	}

	// The same domain on a UTC scale rounds to UTC midnight, which is 09:00 in
	// Tokyo — a different set of instants.
	u := NewUTC().SetDomain(start, stop)
	for _, x := range u.Ticks(10) {
		if h, _, _ := x.In(time.UTC).Clock(); h != 0 {
			t.Errorf("UTC tick %v is not UTC midnight", x)
		}
	}
}

func TestTimeCopyAndLocation(t *testing.T) {
	a := NewUTC().
		SetDomain(utc(2000, time.January, 1, 0, 0, 0), utc(2000, time.January, 2, 0, 0, 0)).
		SetRange(0, 100)
	b := a.Copy().SetRange(0, 200)
	closeTo(t, a.Scale(utc(2000, time.January, 2, 0, 0, 0)), 100, "original unaffected")
	closeTo(t, b.Scale(utc(2000, time.January, 2, 0, 0, 0)), 200, "copy independent")

	if NewUTC().Location() != time.UTC || !NewUTC().UTC() {
		t.Error("NewUTC should work in UTC")
	}
	if NewTime().UTC() {
		t.Error("NewTime should not be a UTC scale")
	}
}
