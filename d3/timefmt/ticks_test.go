package timefmt

import (
	"testing"
	"time"
)

// TestTickIntervalLadder pins which rung of the ladder each span lands on. The
// expectations follow from the table in ticks.go and from the geometric
// tie-break, so they are value assertions on the algorithm rather than guesses
// about output text.
func TestTickIntervalLadder(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		span  time.Duration
		count int
		want  string
	}{
		{time.Second, 10, "millisecond.every(100)"},
		{time.Minute, 10, "second.every(5)"},
		{time.Hour, 10, "utcMinute.every(5)"},
		{24 * time.Hour, 10, "utcHour.every(3)"},
		{7 * 24 * time.Hour, 10, "utcHour.every(12)"},
		{30 * 24 * time.Hour, 10, "utcDay.every(2)"},
		{365 * 24 * time.Hour, 10, "utcMonth"},
		{10 * 365 * 24 * time.Hour, 10, "utcYear"},
		{100 * 365 * 24 * time.Hour, 10, "utcYear.every(10)"},
	}
	for _, tt := range tests {
		iv := UTCTickInterval(start, start.Add(tt.span), tt.count)
		if iv == nil {
			t.Errorf("span %v: got nil", tt.span)
			continue
		}
		if iv.String() != tt.want {
			t.Errorf("span %v count %d: got %s, want %s", tt.span, tt.count, iv, tt.want)
		}
	}
}

// TestTicksCountIsReasonable is the property that actually matters to an axis:
// the number of ticks is close to what was asked for, never wildly off, and
// every tick lies inside the domain.
func TestTicksCountIsReasonable(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	spans := []time.Duration{
		10 * time.Millisecond, time.Second, 45 * time.Second, time.Minute,
		17 * time.Minute, time.Hour, 5 * time.Hour, 24 * time.Hour,
		3 * 24 * time.Hour, 14 * 24 * time.Hour, 90 * 24 * time.Hour,
		365 * 24 * time.Hour, 3 * 365 * 24 * time.Hour, 40 * 365 * 24 * time.Hour,
	}
	for _, span := range spans {
		for _, count := range []int{2, 5, 10, 20} {
			stop := start.Add(span)
			ticks := UTCTicks(start, stop, count)
			if len(ticks) == 0 {
				t.Errorf("span %v count %d: no ticks", span, count)
				continue
			}
			// The ladder is coarse, so allow a generous factor either way; the
			// point is that it never produces one tick or a thousand.
			if len(ticks) < count/4 || len(ticks) > count*5+2 {
				t.Errorf("span %v count %d: got %d ticks", span, count, len(ticks))
			}
			for _, tk := range ticks {
				if tk.Before(start) || tk.After(stop) {
					t.Errorf("span %v count %d: tick %v outside [%v, %v]", span, count, tk, start, stop)
				}
			}
			// Ticks are strictly increasing.
			for i := 1; i < len(ticks); i++ {
				if !ticks[i].After(ticks[i-1]) {
					t.Errorf("span %v: ticks not increasing at %d: %v, %v", span, i, ticks[i-1], ticks[i])
				}
			}
		}
	}
}

// TestTicksAreAligned: every tick must be a boundary of the interval that was
// chosen, which is what makes the labels readable.
func TestTicksAreAligned(t *testing.T) {
	start := time.Date(2024, 3, 9, 7, 23, 41, 0, time.UTC)
	for _, span := range []time.Duration{time.Hour, 24 * time.Hour, 30 * 24 * time.Hour, 365 * 24 * time.Hour} {
		stop := start.Add(span)
		iv := UTCTickInterval(start, stop, 10)
		if iv == nil {
			t.Fatalf("span %v: nil interval", span)
		}
		for _, tk := range UTCTicks(start, stop, 10) {
			if !iv.Floor(tk).Equal(tk) {
				t.Errorf("span %v: tick %v is not a %s boundary", span, tk, iv)
			}
		}
	}
}

// TestTicksReversedDomain: a descending domain gives descending ticks.
func TestTicksReversedDomain(t *testing.T) {
	a := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	b := a.Add(24 * time.Hour)
	fwd := UTCTicks(a, b, 5)
	rev := UTCTicks(b, a, 5)
	if len(fwd) != len(rev) {
		t.Fatalf("forward %d ticks, reverse %d", len(fwd), len(rev))
	}
	for i := range fwd {
		if !fwd[i].Equal(rev[len(rev)-1-i]) {
			t.Errorf("reversed ticks differ at %d: %v vs %v", i, fwd[i], rev[len(rev)-1-i])
		}
	}
}

// TestTickIntervalDegenerate: no ticks for an empty domain or a bad count.
func TestTickIntervalDegenerate(t *testing.T) {
	a := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		start, stop time.Time
		count       int
	}{
		{a, a, 10},
		{a, a.Add(time.Hour), 0},
		{a, a.Add(time.Hour), -1},
	} {
		if iv := UTCTickInterval(tc.start, tc.stop, tc.count); iv != nil {
			t.Errorf("TickInterval(%v, %v, %d) = %s, want nil", tc.start, tc.stop, tc.count, iv)
		}
		if ticks := UTCTicks(tc.start, tc.stop, tc.count); len(ticks) != 0 {
			t.Errorf("Ticks(%v, %v, %d) = %v, want none", tc.start, tc.stop, tc.count, ticks)
		}
	}
}

// TestLocalTickIntervalUsesValueLocation: the local family picks calendar
// boundaries in the domain's own zone, so a day tick is local midnight.
func TestLocalTickIntervalUsesValueLocation(t *testing.T) {
	tokyo := mustLoad(t, "Asia/Tokyo")
	start := time.Date(2024, 3, 1, 0, 0, 0, 0, tokyo)
	stop := start.AddDate(0, 0, 20)
	for _, tk := range Ticks(start, stop, 10) {
		if tk.Location() != tokyo {
			t.Errorf("tick %v lost its location", tk)
		}
		if tk.Hour() != 0 {
			t.Errorf("tick %v is not Tokyo midnight", tk)
		}
	}
}

// TestTickStep is the 1-2-5-10 progression the above-a-year fallback relies on.
func TestTickStep(t *testing.T) {
	tests := []struct {
		start, stop float64
		count       int
		want        float64
	}{
		{0, 10, 10, 1},
		{0, 100, 10, 10},
		{0, 1, 10, 0.1},
		{0, 10, 5, 2},
		{0, 10, 2, 5},
		{0, 50, 10, 5},
		{10, 0, 10, -1},
	}
	for _, tt := range tests {
		if got := tickStep(tt.start, tt.stop, tt.count); got != tt.want {
			t.Errorf("tickStep(%v, %v, %d) = %v, want %v", tt.start, tt.stop, tt.count, got, tt.want)
		}
	}
}

// TestTicksThenFormat is the end-to-end shape a time axis has: choose an
// interval, generate ticks, label them. It is here to make sure the two halves
// of the package compose.
func TestTicksThenFormat(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stop := start.AddDate(0, 6, 0)
	iv := UTCTickInterval(start, stop, 6)
	if iv == nil {
		t.Fatal("nil interval")
	}
	label := UTCFormat("%b %Y")
	var labels []string
	for _, tk := range UTCTicks(start, stop, 6) {
		labels = append(labels, label(tk))
	}
	if len(labels) < 5 || labels[0] != "Jan 2024" {
		t.Errorf("labels = %v", labels)
	}
	for i := 1; i < len(labels); i++ {
		if labels[i] == labels[i-1] {
			t.Errorf("duplicate labels at %d: %v", i, labels)
		}
	}
}
