// Command rrule-example exercises the published github.com/malcolmston/rrule
// module: building rules from Options, expanding occurrences, window queries,
// parsing and serializing RFC 5545 text, recurrence sets with RDATE/EXDATE/
// EXRULE, iCalendar encode/decode, and error handling.
//
// Every DTSTART, UNTIL and window bound is a fixed literal, so the output is
// deterministic. Expansions are compared against dates computed by hand (many
// of them taken straight from the RFC 5545 §3.8.5.3 examples).
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/rrule"
)

var (
	checks   int
	failures int
)

func check(label string, got, want any) {
	checks++
	status := "ok"
	if fmt.Sprint(got) != fmt.Sprint(want) {
		status = "MISMATCH"
		failures++
	}
	fmt.Printf("  [%-8s] %s\n", status, label)
	if status == "MISMATCH" {
		fmt.Printf("             got:  %v\n             want: %v\n", got, want)
	}
}

func section(name string) { fmt.Printf("\n=== %s ===\n", name) }

// utc builds a UTC instant.
func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

// dates renders occurrences as "YYYY-MM-DD Mon HH:MM ZZZ" for comparison.
func dates(ts []time.Time) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.Format("2006-01-02 Mon 15:04 MST")
	}
	return strings.Join(parts, " | ")
}

// days renders occurrences as bare dates.
func days(ts []time.Time) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.Format("2006-01-02")
	}
	return strings.Join(parts, " ")
}

// start is the DTSTART used by most examples: Tuesday 2 September 1997, 09:00 UTC.
var start = utc(1997, time.September, 2, 9, 0)

func main() {
	frequencies()
	byRuleParts()
	countUntil()
	windows()
	textRoundTrip()
	sets()
	icalendar()
	timezones()
	errorsAndEdges()

	fmt.Printf("\n=== summary ===\n  %d checks, %d mismatches\n", checks, failures)
	if failures > 0 {
		fmt.Fprintf(os.Stderr, "note: %d hand-computed expectations did not match\n", failures)
	}
}

// mustRule builds a rule and aborts on error: every rule in this program is a
// literal known to be valid.
func mustRule(o rrule.Options) *rrule.RRule {
	r, err := rrule.New(o)
	if err != nil {
		panic(err)
	}
	return r
}

func frequencies() {
	section("frequencies, INTERVAL and COUNT")

	daily := mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start, Count: 5})
	fmt.Printf("  FREQ=DAILY;COUNT=5              -> %s\n", days(daily.All()))
	check("daily x5 from 1997-09-02",
		days(daily.All()), "1997-09-02 1997-09-03 1997-09-04 1997-09-05 1997-09-06")

	every10 := mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start, Interval: 10, Count: 5})
	fmt.Printf("  FREQ=DAILY;INTERVAL=10;COUNT=5  -> %s\n", days(every10.All()))
	check("every 10 days x5",
		days(every10.All()), "1997-09-02 1997-09-12 1997-09-22 1997-10-02 1997-10-12")

	weekly := mustRule(rrule.Options{Freq: rrule.Weekly, DTStart: start, Count: 4})
	fmt.Printf("  FREQ=WEEKLY;COUNT=4             -> %s\n", days(weekly.All()))
	check("weekly x4", days(weekly.All()), "1997-09-02 1997-09-09 1997-09-16 1997-09-23")

	monthly := mustRule(rrule.Options{Freq: rrule.Monthly, DTStart: utc(1997, time.September, 2, 9, 0), Count: 4})
	fmt.Printf("  FREQ=MONTHLY;COUNT=4            -> %s\n", days(monthly.All()))
	check("monthly x4 keeps day 2", days(monthly.All()), "1997-09-02 1997-10-02 1997-11-02 1997-12-02")

	yearly := mustRule(rrule.Options{Freq: rrule.Yearly, DTStart: start, Count: 3})
	fmt.Printf("  FREQ=YEARLY;COUNT=3             -> %s\n", days(yearly.All()))
	check("yearly x3", days(yearly.All()), "1997-09-02 1998-09-02 1999-09-02")

	hourly := mustRule(rrule.Options{Freq: rrule.Hourly, DTStart: start, Interval: 3, Count: 4})
	fmt.Printf("  FREQ=HOURLY;INTERVAL=3;COUNT=4  -> %s\n", dates(hourly.All()))
	check("every 3 hours x4", dates(hourly.All()),
		"1997-09-02 Tue 09:00 UTC | 1997-09-02 Tue 12:00 UTC | 1997-09-02 Tue 15:00 UTC | 1997-09-02 Tue 18:00 UTC")

	minutely := mustRule(rrule.Options{Freq: rrule.Minutely, DTStart: start, Interval: 15, Count: 3})
	check("every 15 minutes x3", dates(minutely.All()),
		"1997-09-02 Tue 09:00 UTC | 1997-09-02 Tue 09:15 UTC | 1997-09-02 Tue 09:30 UTC")

	// Freq.String() gives the RFC spelling.
	check("Freq.String()", fmt.Sprint(rrule.Yearly, rrule.Monthly, rrule.Weekly, rrule.Daily,
		rrule.Hourly, rrule.Minutely, rrule.Secondly),
		"YEARLY MONTHLY WEEKLY DAILY HOURLY MINUTELY SECONDLY")

	// Read the compiled rule back.
	o := every10.Options()
	fmt.Printf("  Options() read-back             -> freq=%s interval=%d count=%d dtstart=%s\n",
		o.Freq, o.Interval, o.Count, every10.DTStart().Format(time.RFC3339))
	check("Freq() accessor", every10.Freq(), rrule.Daily)
	check("DTStart() accessor", every10.DTStart().Equal(start), true)
}

func byRuleParts() {
	section("BYDAY / BYMONTHDAY / BYMONTH / BYSETPOS / BYWEEKNO / BYHOUR")

	// BYDAY, plain weekdays.
	tuth := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: start, Count: 6,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.TH},
	})
	fmt.Printf("  WEEKLY;BYDAY=TU,TH;COUNT=6      -> %s\n", days(tuth.All()))
	check("Tue+Thu x6 from Tue 1997-09-02",
		days(tuth.All()), "1997-09-02 1997-09-04 1997-09-09 1997-09-11 1997-09-16 1997-09-18")

	// BYDAY with an ordinal: the first Friday of each month.
	firstFri := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 5, 9, 0), Count: 4,
		ByWeekday: []rrule.Weekday{rrule.FR.Nth(1)},
	})
	fmt.Printf("  MONTHLY;BYDAY=1FR;COUNT=4       -> %s\n", days(firstFri.All()))
	check("first Friday of Sep-Dec 1997",
		days(firstFri.All()), "1997-09-05 1997-10-03 1997-11-07 1997-12-05")

	// The last Sunday of each month.
	lastSun := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 28, 9, 0), Count: 3,
		ByWeekday: []rrule.Weekday{rrule.SU.Nth(-1)},
	})
	fmt.Printf("  MONTHLY;BYDAY=-1SU;COUNT=3      -> %s\n", days(lastSun.All()))
	check("last Sunday of Sep/Oct/Nov 1997", days(lastSun.All()), "1997-09-28 1997-10-26 1997-11-30")

	// BYMONTHDAY, negative counts back from the end of the month.
	lastDay := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 30, 9, 0), Count: 5,
		ByMonthDay: []int{-1},
	})
	fmt.Printf("  MONTHLY;BYMONTHDAY=-1;COUNT=5   -> %s\n", days(lastDay.All()))
	check("last day of Sep 1997 .. Jan 1998",
		days(lastDay.All()), "1997-09-30 1997-10-31 1997-11-30 1997-12-31 1998-01-31")

	// Two monthdays every other month.
	biMonthly := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 1, 9, 0), Interval: 2, Count: 6,
		ByMonthDay: []int{1, 15},
	})
	fmt.Printf("  MONTHLY;INTERVAL=2;BYMONTHDAY=1,15 -> %s\n", days(biMonthly.All()))
	check("1st and 15th of every other month",
		days(biMonthly.All()), "1997-09-01 1997-09-15 1997-11-01 1997-11-15 1998-01-01 1998-01-15")

	// BYMONTH limits a YEARLY rule.
	juneJuly := mustRule(rrule.Options{
		Freq: rrule.Yearly, DTStart: utc(1997, time.June, 10, 9, 0), Count: 4,
		ByMonth: []int{6, 7},
	})
	fmt.Printf("  YEARLY;BYMONTH=6,7;COUNT=4      -> %s\n", days(juneJuly.All()))
	check("June+July of 1997 and 1998",
		days(juneJuly.All()), "1997-06-10 1997-07-10 1998-06-10 1998-07-10")

	// BYSETPOS: the last weekday of each month.
	lastWeekday := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 29, 9, 0), Count: 3,
		ByWeekday: []rrule.Weekday{rrule.MO, rrule.TU, rrule.WE, rrule.TH, rrule.FR},
		BySetPos:  []int{-1},
	})
	fmt.Printf("  MONTHLY;BYDAY=MO..FR;BYSETPOS=-1 -> %s\n", dates(lastWeekday.All()))
	// Sep 30 1997 is a Tuesday, Oct 31 a Friday; Nov 30 is a Sunday so the last
	// weekday of November is Friday the 28th.
	check("last weekday of Sep/Oct/Nov 1997", days(lastWeekday.All()), "1997-09-30 1997-10-31 1997-11-28")

	// BYSETPOS=2 picks the second Tuesday-or-Thursday of each month.
	second := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 2, 9, 0), Count: 3,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.TH},
		BySetPos:  []int{2},
	})
	fmt.Printf("  MONTHLY;BYDAY=TU,TH;BYSETPOS=2  -> %s\n", days(second.All()))
	// September 1997: Tue 2, Thu 4 -> 2nd is Sep 4. October: Thu 2, Tue 7 -> Oct 7.
	// November: Tue 4, Thu 6 -> Nov 6.
	check("2nd Tue-or-Thu of Sep/Oct/Nov 1997", days(second.All()), "1997-09-04 1997-10-07 1997-11-06")

	// BYYEARDAY.
	yearDay := mustRule(rrule.Options{
		Freq: rrule.Yearly, DTStart: utc(1997, time.January, 1, 9, 0), Count: 4,
		ByYearDay: []int{1, -1},
	})
	fmt.Printf("  YEARLY;BYYEARDAY=1,-1;COUNT=4   -> %s\n", days(yearDay.All()))
	check("first and last day of 1997 and 1998",
		days(yearDay.All()), "1997-01-01 1997-12-31 1998-01-01 1998-12-31")

	// BYWEEKNO with BYDAY: the Monday of ISO week 20.
	weekNo := mustRule(rrule.Options{
		Freq: rrule.Yearly, DTStart: utc(1997, time.May, 12, 9, 0), Count: 3,
		ByWeekNo:  []int{20},
		ByWeekday: []rrule.Weekday{rrule.MO},
	})
	fmt.Printf("  YEARLY;BYWEEKNO=20;BYDAY=MO     -> %s\n", days(weekNo.All()))
	check("Monday of ISO week 20, 1997-1999", days(weekNo.All()), "1997-05-12 1998-05-11 1999-05-17")

	// BYHOUR expands a DAILY rule.
	byHour := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: start, Count: 4,
		ByHour: []int{9, 17},
	})
	fmt.Printf("  DAILY;BYHOUR=9,17;COUNT=4       -> %s\n", dates(byHour.All()))
	check("09:00 and 17:00 on two days", dates(byHour.All()),
		"1997-09-02 Tue 09:00 UTC | 1997-09-02 Tue 17:00 UTC | 1997-09-03 Wed 09:00 UTC | 1997-09-03 Wed 17:00 UTC")

	// WKST genuinely changes a FREQ=WEEKLY;INTERVAL=2 result. This is the
	// RFC 5545 §3.3.10 worked example.
	aug5 := utc(1997, time.August, 5, 9, 0) // a Tuesday
	wkstMO := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: aug5, Interval: 2, Count: 4,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.SU},
	})
	wkstSU := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: aug5, Interval: 2, Count: 4,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.SU}, Wkst: rrule.SundayStart,
	})
	fmt.Printf("  WKST=MO (zero value)            -> %s\n", days(wkstMO.All()))
	fmt.Printf("  WKST=SU (rrule.SundayStart)     -> %s\n", days(wkstSU.All()))
	check("RFC example, WKST=MO", days(wkstMO.All()), "1997-08-05 1997-08-10 1997-08-19 1997-08-24")
	check("RFC example, WKST=SU", days(wkstSU.All()), "1997-08-05 1997-08-17 1997-08-19 1997-08-31")

	// Weekday.String() spells BYDAY values.
	check("Weekday.String()", fmt.Sprint(rrule.MO, rrule.FR.Nth(-1), rrule.SU.Nth(2)), "MO -1FR 2SU")
}

func countUntil() {
	section("COUNT vs UNTIL")

	// UNTIL is inclusive.
	until := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: start, Until: utc(1997, time.October, 7, 9, 0),
	})
	fmt.Printf("  WEEKLY;UNTIL=19971007T090000Z   -> %s\n", days(until.All()))
	check("UNTIL includes the bound",
		days(until.All()), "1997-09-02 1997-09-09 1997-09-16 1997-09-23 1997-09-30 1997-10-07")

	// One second before the last occurrence excludes it.
	untilShort := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: start,
		Until: utc(1997, time.October, 7, 9, 0).Add(-time.Second),
	})
	check("UNTIL one second earlier drops the last", len(untilShort.All()), 5)

	// COUNT and UNTIL together: whichever bites first wins.
	both := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: start, Count: 10, Until: utc(1997, time.September, 4, 9, 0),
	})
	check("COUNT=10 but UNTIL after 3 days", days(both.All()), "1997-09-02 1997-09-03 1997-09-04")

	// An unbounded rule: All() is capped at MaxAllOccurrences.
	unbounded := mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start})
	all := unbounded.All()
	fmt.Printf("  unbounded DAILY All()           -> %d occurrences (MaxAllOccurrences=%d), last=%s\n",
		len(all), rrule.MaxAllOccurrences, all[len(all)-1].Format("2006-01-02"))
	check("All() capped on an unbounded rule", len(all), rrule.MaxAllOccurrences)
}

func windows() {
	section("window queries and Iterator")

	r := mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: start, Count: 10,
		ByWeekday: []rrule.Weekday{rrule.TU, rrule.TH},
	})
	fmt.Printf("  all                             -> %s\n", days(r.All()))

	win := r.Between(utc(1997, time.September, 9, 0, 0), utc(1997, time.September, 17, 0, 0), false)
	fmt.Printf("  Between(Sep 9, Sep 17)          -> %s\n", days(win))
	check("Between exclusive window", days(win), "1997-09-09 1997-09-11 1997-09-16")

	inc := r.Between(utc(1997, time.September, 9, 9, 0), utc(1997, time.September, 16, 9, 0), true)
	check("Between inclusive bounds", days(inc), "1997-09-09 1997-09-11 1997-09-16")

	exc := r.Between(utc(1997, time.September, 9, 9, 0), utc(1997, time.September, 16, 9, 0), false)
	check("Between exclusive bounds", days(exc), "1997-09-11")

	after := r.After(utc(1997, time.September, 9, 9, 0), false)
	before := r.Before(utc(1997, time.September, 9, 9, 0), false)
	fmt.Printf("  After/Before Sep 9 09:00        -> %s / %s\n",
		after.Format("2006-01-02"), before.Format("2006-01-02"))
	check("After(Sep 9 09:00, exclusive)", after.Format("2006-01-02"), "1997-09-11")
	check("Before(Sep 9 09:00, exclusive)", before.Format("2006-01-02"), "1997-09-04")
	check("After(Sep 9 09:00, inclusive)", r.After(utc(1997, time.September, 9, 9, 0), true).Format("2006-01-02"), "1997-09-09")
	check("After past the end is zero", r.After(utc(1999, time.January, 1, 0, 0), false).IsZero(), true)

	// The pull-based iterator; two iterators are independent.
	it1 := r.Iterator()
	it2 := r.Iterator()
	var seq []time.Time
	for i := 0; i < 3; i++ {
		t, ok := it1()
		if !ok {
			break
		}
		seq = append(seq, t)
	}
	first2, _ := it2()
	check("Iterator yields in order", days(seq), "1997-09-02 1997-09-04 1997-09-09")
	check("independent iterators", first2.Format("2006-01-02"), "1997-09-02")

	// Draining the iterator reports exhaustion, and keeps reporting it.
	itAll := mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start, Count: 3}).Iterator()
	n := 0
	for {
		if _, ok := itAll(); !ok {
			break
		}
		n++
	}
	_, ok := itAll()
	check("iterator exhausts after COUNT", n, 3)
	check("exhausted iterator stays false", ok, false)

	// NOTE: this published version has no AllLimit, so the bounded way to read
	// a rule from untrusted text is the iterator (or Between). The rule below is
	// finite but names ~2.5e11 occurrences: All() on it would try to
	// materialize every one.
	huge, err := rrule.StrToRRule("DTSTART:19970902T090000Z\nRRULE:FREQ=SECONDLY;UNTIL=99991231T235959Z")
	if err != nil {
		panic(err)
	}
	bounded := take(huge.Iterator(), 3)
	fmt.Printf("  bounded read of a 2.5e11 rule   -> %s\n", dates(bounded))
	check("iterator bounds a huge finite rule", dates(bounded),
		"1997-09-02 Tue 09:00 UTC | 1997-09-02 Tue 09:00 UTC | 1997-09-02 Tue 09:00 UTC")
}

// take pulls at most n occurrences from an iterator.
func take(next func() (time.Time, bool), n int) []time.Time {
	var out []time.Time
	for i := 0; i < n; i++ {
		t, ok := next()
		if !ok {
			break
		}
		out = append(out, t)
	}
	return out
}

func textRoundTrip() {
	section("RFC 5545 text: Parse / StrToRRule / String / RuleString")

	// A bare rule value, with and without the property name.
	for _, in := range []string{
		"FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6",
		"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=6",
	} {
		r, err := rrule.Parse(in)
		if err != nil {
			panic(err)
		}
		fmt.Printf("  Parse(%-38q) -> RuleString=%s\n", in, r.RuleString())
		check("RuleString round trip: "+in, r.RuleString(), "FREQ=WEEKLY;COUNT=6;BYDAY=MO,WE")
	}

	// DTSTART + RRULE, the form String() produces.
	block := "DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;INTERVAL=2;COUNT=4"
	r, err := rrule.StrToRRule(block)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  StrToRRule -> String():\n%s\n", indent(r.String()))
	check("StrToRRule occurrences", days(r.All()), "1997-09-02 1997-09-04 1997-09-06 1997-09-08")
	check("String() emits DTSTART + RRULE", r.String(),
		"DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;INTERVAL=2;COUNT=4")

	// String() -> Parse() -> String() is stable.
	again, err := rrule.StrToRRule(r.String())
	if err != nil {
		panic(err)
	}
	check("String round trips through StrToRRule", again.String(), r.String())

	// A zoned DTSTART serializes with TZID.
	ny := mustZone("America/New_York")
	zoned := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, ny), Count: 3,
	})
	fmt.Printf("  zoned String():\n%s\n", indent(zoned.String()))
	check("TZID on serialization", zoned.String(),
		"DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=DAILY;COUNT=3")

	// And it re-parses to the same instants.
	reparsed, err := rrule.StrToRRule(zoned.String())
	if err != nil {
		panic(err)
	}
	check("zoned round trip keeps instants", dates(reparsed.All()), dates(zoned.All()))

	// Every rule part survives serialization.
	full := "DTSTART:19970902T090000Z\nRRULE:FREQ=MONTHLY;INTERVAL=2;COUNT=5;WKST=SU;BYMONTH=1,9;BYMONTHDAY=1,-1;BYDAY=MO,-1FR;BYHOUR=9;BYSETPOS=1"
	fr, err := rrule.StrToRRule(full)
	if err != nil {
		panic(err)
	}
	fmt.Printf("  full rule RuleString            -> %s\n", fr.RuleString())
	rr2, err := rrule.StrToRRule(fr.String())
	if err != nil {
		panic(err)
	}
	check("all rule parts survive a round trip", rr2.RuleString(), fr.RuleString())

	// UNTIL in the text is a UTC instant.
	u, err := rrule.Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=WEEKLY;UNTIL=19970923T090000Z")
	if err != nil {
		panic(err)
	}
	check("parsed UNTIL is inclusive", days(u.All()), "1997-09-02 1997-09-09 1997-09-16 1997-09-23")

	// Folded content lines are unfolded first (RFC 5545 §3.1).
	folded := "DTSTART:19970902T090000Z\r\nRRULE:FREQ=WEEKLY;BYDAY=TU,\r\n TH;COUNT=4"
	f, err := rrule.StrToRRule(folded)
	if err != nil {
		panic(err)
	}
	check("folded line unfolds", days(f.All()), "1997-09-02 1997-09-04 1997-09-09 1997-09-11")
}

func sets() {
	section("recurrence sets: RRULE + RDATE + EXDATE + EXRULE")

	s := rrule.NewSet()
	s.RRule(mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start, Count: 5}))
	s.ExDate(utc(1997, time.September, 4, 9, 0))
	s.RDate(utc(1997, time.September, 7, 9, 0))
	fmt.Printf("  daily x5, EXDATE Sep 4, RDATE Sep 7 -> %s\n", days(s.All()))
	check("EXDATE removes, RDATE adds",
		days(s.All()), "1997-09-02 1997-09-03 1997-09-05 1997-09-06 1997-09-07")
	check("Set.DTStart()", s.DTStart().Format("2006-01-02"), "1997-09-02")
	check("Set.RDates()", days(s.RDates()), "1997-09-07")
	check("Set.ExDates()", days(s.ExDates()), "1997-09-04")
	check("Set.RRules() count", len(s.RRules()), 1)

	// An RDATE that duplicates a rule occurrence is not emitted twice.
	dup := rrule.NewSet()
	dup.RRule(mustRule(rrule.Options{Freq: rrule.Daily, DTStart: start, Count: 3}))
	dup.RDate(utc(1997, time.September, 3, 9, 0))
	check("duplicate RDATE deduplicated", days(dup.All()), "1997-09-02 1997-09-03 1997-09-04")

	// EXRULE subtracts a whole rule: weekdays minus Wednesdays.
	ex := rrule.NewSet()
	ex.RRule(mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: start, Count: 10,
	}))
	ex.ExRule(mustRule(rrule.Options{
		Freq: rrule.Weekly, DTStart: start, Count: 3,
		ByWeekday: []rrule.Weekday{rrule.SA, rrule.SU},
	}))
	fmt.Printf("  daily x10 minus weekends        -> %s\n", days(ex.All()))
	// 1997-09-02 is a Tuesday, so the ten days run to Sep 11; Sep 6 and 7 are
	// the weekend and are removed.
	check("EXRULE removes the weekend",
		days(ex.All()), "1997-09-02 1997-09-03 1997-09-04 1997-09-05 1997-09-08 1997-09-09 1997-09-10 1997-09-11")
	check("Set.ExRules() count", len(ex.ExRules()), 1)

	// Set window queries mirror the rule ones.
	check("Set.Between", days(s.Between(utc(1997, time.September, 3, 0, 0), utc(1997, time.September, 6, 0, 0), false)),
		"1997-09-03 1997-09-05")
	check("Set.After", s.After(utc(1997, time.September, 3, 9, 0), false).Format("2006-01-02"), "1997-09-05")
	check("Set.Before", s.Before(utc(1997, time.September, 5, 9, 0), false).Format("2006-01-02"), "1997-09-03")
	check("Set.Iterator", days(take(s.Iterator(), 2)), "1997-09-02 1997-09-03")

	// Two rules merge, in order and deduplicated.
	two := rrule.NewSet()
	two.RRule(mustRule(rrule.Options{Freq: rrule.Weekly, DTStart: start, Count: 2,
		ByWeekday: []rrule.Weekday{rrule.TU}}))
	two.RRule(mustRule(rrule.Options{Freq: rrule.Weekly, DTStart: start, Count: 2,
		ByWeekday: []rrule.Weekday{rrule.TH}}))
	check("two rules merge chronologically", days(two.All()),
		"1997-09-02 1997-09-04 1997-09-09 1997-09-11")

	// Serialization and re-parsing of a whole set.
	text := s.String()
	fmt.Printf("  Set.String():\n%s\n", indent(text))
	parsed, err := rrule.StrToSet(text)
	if err != nil {
		panic(err)
	}
	check("StrToSet round trip", days(parsed.All()), days(s.All()))

	// StrToSet straight from RFC 5545 text.
	fromText, err := rrule.StrToSet("DTSTART:19970902T090000Z\n" +
		"RRULE:FREQ=YEARLY;COUNT=6;BYDAY=TU,TH\n" +
		"EXDATE:19970904T090000Z")
	if err != nil {
		panic(err)
	}
	fmt.Printf("  StrToSet(text)                  -> %s\n", days(fromText.All()))
	check("EXDATE from text is applied", strings.Contains(days(fromText.All()), "1997-09-04"), false)

	// StrToRRule refuses a block that is really a set.
	_, err = rrule.StrToRRule("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=3\nEXDATE:19970903T090000Z")
	fmt.Printf("  StrToRRule on a set block       -> err=%v\n", err)
	check("StrToRRule rejects a set block", err != nil, true)

	// An empty set is empty rather than a panic.
	check("empty Set.All()", len(rrule.NewSet().All()), 0)
	check("empty Set.After() is zero", rrule.NewSet().After(start, true).IsZero(), true)
}

func icalendar() {
	section("iCalendar: Calendar.Encode / ParseCalendar")

	r := mustRule(rrule.Options{Freq: rrule.Weekly, DTStart: start, Count: 3,
		ByWeekday: []rrule.Weekday{rrule.TU}})
	cal := &rrule.Calendar{
		ProdID: "-//Example Corp.//Scheduler//EN",
		Events: []*rrule.Event{{
			UID:         "1@example.com",
			Summary:     "Stand-up; short",
			Description: "Daily sync\nsecond line",
			Location:    "Room 1",
			DTStart:     start,
			DTEnd:       start.Add(30 * time.Minute),
			RRule:       r,
		}},
	}
	var b strings.Builder
	if err := cal.Encode(&b); err != nil {
		panic(err)
	}
	ics := b.String()
	fmt.Println(indent(strings.ReplaceAll(ics, "\r\n", "\n")))
	check("CRLF line endings", strings.Contains(ics, "\r\n"), true)
	check("text escaping of ';'", strings.Contains(ics, `SUMMARY:Stand-up\; short`), true)
	check("newline escaped in DESCRIPTION", strings.Contains(ics, `\n`), true)
	check("RRULE emitted", strings.Contains(ics, "RRULE:FREQ=WEEKLY;COUNT=3;BYDAY=TU"), true)

	// Round trip.
	back, err := rrule.ParseCalendar(strings.NewReader(ics))
	if err != nil {
		panic(err)
	}
	ev := back.Events[0]
	fmt.Printf("  reparsed: uid=%s summary=%q loc=%q allDay=%v\n", ev.UID, ev.Summary, ev.Location, ev.AllDay)
	check("round-tripped UID", ev.UID, "1@example.com")
	check("round-tripped SUMMARY unescaped", ev.Summary, "Stand-up; short")
	check("round-tripped DESCRIPTION unescaped", ev.Description, "Daily sync\nsecond line")
	check("round-tripped DTEND", ev.DTEnd.Equal(start.Add(30*time.Minute)), true)
	check("round-tripped occurrences", days(ev.RRule.All()), days(r.All()))
	check("Event.Set is populated for a recurring event", ev.Set != nil, true)
	if ev.Set != nil {
		check("Event.Set expands like the rule", days(ev.Set.All()), days(r.All()))
	}
	check("raw Props kept", len(ev.Props["UID"]), 1)

	// A VEVENT carrying RDATE/EXDATE gives a Set.
	const withSet = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"UID:2@example.com\r\nDTSTART:19970902T090000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=4\r\nEXDATE:19970903T090000Z\r\n" +
		"RDATE:19970910T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	c2, err := rrule.ParseCalendar(strings.NewReader(withSet))
	if err != nil {
		panic(err)
	}
	fmt.Printf("  VEVENT with RDATE/EXDATE        -> %s\n", days(c2.Events[0].Set.All()))
	check("Set from a VEVENT", days(c2.Events[0].Set.All()),
		"1997-09-02 1997-09-04 1997-09-05 1997-09-10")

	// All-day events use VALUE=DATE.
	allDay := &rrule.Calendar{Events: []*rrule.Event{{
		UID: "3@example.com", Summary: "Holiday",
		DTStart: time.Date(1997, 12, 25, 0, 0, 0, 0, time.UTC), AllDay: true,
	}}}
	var ab strings.Builder
	if err := allDay.Encode(&ab); err != nil {
		panic(err)
	}
	check("all-day DTSTART is VALUE=DATE",
		strings.Contains(ab.String(), "DTSTART;VALUE=DATE:19971225"), true)
	adBack, err := rrule.ParseCalendar(strings.NewReader(ab.String()))
	if err != nil {
		panic(err)
	}
	check("all-day flag survives", adBack.Events[0].AllDay, true)

	// A malformed calendar is an error, not a panic.
	_, err = rrule.ParseCalendar(strings.NewReader("BEGIN:VCALENDAR\r\nnot a content line\r\nEND:VCALENDAR\r\n"))
	fmt.Printf("  malformed ICS                   -> err=%v\n", err)
	check("malformed ICS errors", err != nil, true)
}

func timezones() {
	section("time zones and DST")

	ny := mustZone("America/New_York")

	// Wall clock is preserved across the fall-back transition (2017-11-05).
	fall := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: time.Date(2017, 11, 4, 9, 0, 0, 0, ny), Count: 3})
	fmt.Printf("  daily 09:00 NY across fall-back -> %s\n", dates(fall.All()))
	check("wall clock kept, offset changes", dates(fall.All()),
		"2017-11-04 Sat 09:00 EDT | 2017-11-05 Sun 09:00 EST | 2017-11-06 Mon 09:00 EST")

	// The repeated hour resolves to the first (pre-transition) instant.
	repeated := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: time.Date(2017, 11, 4, 1, 30, 0, 0, ny), Count: 2})
	fmt.Printf("  daily 01:30 NY into the repeat  -> %s\n", dates(repeated.All()))
	check("repeated 01:30 uses EDT, the earlier offset",
		repeated.All()[1].Format("2006-01-02 15:04 MST"), "2017-11-05 01:30 EDT")

	// HOURLY across the spring-forward gap: 02:00 does not exist on 2017-03-12.
	hourly := mustRule(rrule.Options{
		Freq: rrule.Hourly, DTStart: time.Date(2017, 3, 12, 0, 0, 0, 0, ny), Count: 5})
	fmt.Printf("  hourly NY across spring-forward -> %s\n", dates(hourly.All()))
	check("no duplicate instant across the gap", dates(hourly.All()),
		"2017-03-12 Sun 00:00 EST | 2017-03-12 Sun 01:00 EST | 2017-03-12 Sun 03:00 EDT | "+
			"2017-03-12 Sun 04:00 EDT | 2017-03-12 Sun 05:00 EDT")
	check("occurrences strictly increasing", strictlyIncreasing(hourly.All()), true)

	// HOLE: a DAILY 02:30 rule on the day the gap removes 02:30 emits 01:30
	// instead of skipping the day. RFC 5545 §3.3.10 says a nonexistent local
	// time "MUST be ignored and MUST NOT be counted as part of the recurrence
	// set". The published version documents following time.Date instead.
	gap := mustRule(rrule.Options{
		Freq: rrule.Daily, DTStart: time.Date(2017, 3, 11, 2, 30, 0, 0, ny), Count: 3})
	fmt.Printf("  daily 02:30 NY over the gap     -> %s\n", dates(gap.All()))
	fmt.Printf("             RFC 5545 would give  -> 2017-03-11 02:30 EST | (2017-03-12 skipped) | 2017-03-13 02:30 EDT\n")

	// UNTIL is compared as an instant, so a UTC UNTIL cuts a zoned rule where
	// the instant says, not where the wall clock looks like it should.
	zonedUntil, err := rrule.StrToRRule(
		"DTSTART;TZID=America/New_York:19970902T090000\nRRULE:FREQ=HOURLY;INTERVAL=3;UNTIL=19970902T170000Z")
	if err != nil {
		panic(err)
	}
	fmt.Printf("  RFC 3.8.5.3 hourly example      -> %s\n", dates(zonedUntil.All()))
	// UNTIL 17:00Z is 13:00 in New York, so only 09:00 and 12:00 qualify. The
	// RFC's prose claims 09:00, 12:00 and 15:00; the normative reading (and
	// dateutil) give two. Documented in API-DEVIATIONS.md.
	check("UNTIL compared as an instant", dates(zonedUntil.All()),
		"1997-09-02 Tue 09:00 EDT | 1997-09-02 Tue 12:00 EDT")

	// HOLE: a time.FixedZone DTSTART has no resolvable zone name, and String()
	// emits an empty TZID -- "DTSTART;TZID=:19970902T090000". That line is not
	// valid iCalendar, and it re-parses without error in the reader's default
	// location, silently moving every occurrence (here by five hours).
	fixed := mustRule(rrule.Options{
		Freq: rrule.Daily, Count: 2,
		DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, time.FixedZone("", -5*3600))})
	fmt.Printf("  FixedZone String()              -> %s\n", strings.ReplaceAll(fixed.String(), "\n", " / "))
	check("FixedZone emits an empty TZID (bug)",
		strings.HasPrefix(fixed.String(), "DTSTART;TZID=:19970902T090000"), true)
	fixedBack, err := rrule.StrToRRule(fixed.String())
	if err != nil {
		panic(err)
	}
	fmt.Printf("  ... re-parsed                   -> %s (original: %s)\n",
		dates(fixedBack.All()), dates(fixed.All()))
	check("re-parsed FixedZone rule loses the offset (bug)",
		fixedBack.All()[0].Equal(fixed.All()[0]), false)

	// A fixed zone that does carry a name serializes it, even though the name
	// is not a tzdata identifier either.
	named := mustRule(rrule.Options{
		Freq: rrule.Daily, Count: 1,
		DTStart: time.Date(1997, 9, 2, 9, 0, 0, 0, time.FixedZone("EST", -5*3600))})
	fmt.Printf("  named FixedZone String()        -> %s\n", strings.ReplaceAll(named.String(), "\n", " / "))
}

func errorsAndEdges() {
	section("errors, degenerate values and edge cases")

	// Rejected rules.
	for _, tc := range []struct {
		name string
		opts rrule.Options
	}{
		{"Freq out of range", rrule.Options{Freq: rrule.Freq(99), DTStart: start}},
		{"BYMONTH=13", rrule.Options{Freq: rrule.Yearly, DTStart: start, ByMonth: []int{13}}},
		{"BYMONTHDAY=0", rrule.Options{Freq: rrule.Monthly, DTStart: start, ByMonthDay: []int{0}}},
		{"BYSETPOS=0", rrule.Options{Freq: rrule.Monthly, DTStart: start, BySetPos: []int{0}, ByMonthDay: []int{1}}},
		{"BYHOUR=24", rrule.Options{Freq: rrule.Daily, DTStart: start, ByHour: []int{24}}},
		{"negative INTERVAL", rrule.Options{Freq: rrule.Daily, DTStart: start, Interval: -1}},
	} {
		_, err := rrule.New(tc.opts)
		fmt.Printf("  New(%-22s) -> err=%v\n", tc.name, err)
		check("New rejects "+tc.name, err != nil, true)
	}

	// Parse errors, and the sentinel each one wraps.
	for _, tc := range []struct{ in, want string }{
		{"FREQ=NEVER", "ErrInvalidFreq"},
		{"FREQ=WEEKLY;BYDAY=XX", "ErrInvalidWeekday"},
		{"FREQ=WEEKLY;NOPE=1", "ErrParse"},
		{"FREQ=WEEKLY;COUNT=abc", "ErrParse"},
		{"FREQ=WEEKLY;UNTIL=nonsense", "ErrParse"},
	} {
		_, err := rrule.Parse(tc.in)
		fmt.Printf("  Parse(%-24q) -> %v\n", tc.in, err)
		check("Parse rejects "+tc.in+" ("+tc.want+")", err != nil, true)
	}

	// HOLE: a rule with no FREQ at all is accepted and silently becomes
	// FREQ=YEARLY (the zero value of Freq), even though Parse's own doc comment
	// says a missing FREQ yields ErrInvalidFreq. FREQ is the one mandatory rule
	// part in RFC 5545 §3.3.10.
	for _, in := range []string{"BYDAY=MO", "COUNT=3"} {
		nf, err := rrule.Parse(in)
		if err != nil {
			fmt.Printf("  Parse(%-12q) with no FREQ -> err=%v\n", in, err)
			continue
		}
		fmt.Printf("  Parse(%-12q) with no FREQ -> accepted as %s\n", in, nf.RuleString())
	}

	// HOLE: RFC 5545 spells COUNT and INTERVAL as positive integers and every
	// BYxxx value as a non-empty list. This published version accepts all three
	// degenerate forms, and each is read as the opposite of what it says:
	// COUNT=0 becomes an unbounded rule, INTERVAL=0 becomes INTERVAL=1, and an
	// empty BYxxx becomes an absent rule part.
	for _, in := range []string{
		"DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=0",
		"DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;INTERVAL=0;COUNT=3",
		"DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;COUNT=3;BYMONTH=",
	} {
		r, err := rrule.StrToRRule(in)
		line := strings.ReplaceAll(in, "\n", " / ")
		if err != nil {
			fmt.Printf("  %-62s -> err=%v\n", line, err)
			continue
		}
		fmt.Printf("  %-62s -> accepted, first 3: %s\n", line, days(take(r.Iterator(), 3)))
	}

	// HOLE: a very large INTERVAL panics with an out-of-range index during
	// iteration. Guarded here so the program still terminates.
	fmt.Printf("  huge INTERVAL -> %s\n", describePanic(func() string {
		r, err := rrule.Parse("DTSTART:19970902T090000Z\nRRULE:FREQ=DAILY;INTERVAL=9223372036854775807;COUNT=2")
		if err != nil {
			return fmt.Sprintf("rejected at parse time: %v", err)
		}
		return "accepted; occurrences: " + days(r.All())
	}))

	// An impossible rule terminates with an empty result instead of hanging.
	imp, err := rrule.StrToRRule("DTSTART:19970101T090000Z\nRRULE:FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2")
	if err != nil {
		panic(err)
	}
	check("FREQ=MONTHLY;BYMONTHDAY=31;BYMONTH=2 is empty", len(imp.All()), 0)

	// A legitimately sparse rule still works: 29 February.
	leap, err := rrule.StrToRRule("DTSTART:20200229T090000Z\nRRULE:FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=29;COUNT=3")
	if err != nil {
		panic(err)
	}
	fmt.Printf("  leap days                       -> %s\n", days(leap.All()))
	check("29 Feb every leap year", days(leap.All()), "2020-02-29 2024-02-29 2028-02-29")

	// A DTSTART that does not itself match the BYxxx parts is not emitted.
	skip := mustRule(rrule.Options{
		Freq: rrule.Monthly, DTStart: utc(1997, time.September, 3, 9, 0), Count: 2,
		ByMonthDay: []int{10},
	})
	check("non-matching DTSTART is skipped", days(skip.All()), "1997-09-10 1997-10-10")

	// Sub-second DTSTART fields: RFC 5545 has no sub-second precision.
	sub := mustRule(rrule.Options{
		Freq: rrule.Daily, Count: 1,
		DTStart: time.Date(1997, 9, 2, 9, 0, 0, 500*int(time.Millisecond), time.UTC)})
	fmt.Printf("  sub-second DTSTART              -> %s (ns=%d)\n",
		sub.All()[0].Format(time.RFC3339Nano), sub.All()[0].Nanosecond())
}

// describePanic runs f and reports either what it returned or the panic it
// raised, so a library panic does not take the whole program down.
func describePanic(f func() string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("PANIC: %v", r)
		}
	}()
	return f()
}

func strictlyIncreasing(ts []time.Time) bool {
	for i := 1; i < len(ts); i++ {
		if !ts[i].After(ts[i-1]) {
			return false
		}
	}
	return true
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	for i, l := range lines {
		lines[i] = "      " + strings.TrimRight(l, "\r")
	}
	return strings.Join(lines, "\n")
}

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
