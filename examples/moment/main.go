// Command moment-example exercises the github.com/malcolmston/moment port:
// parsing, token formatting, immutable manipulation, diffing, start/end of
// unit, relative-time humanization, durations, time zones and invalid input.
//
// Every instant is a fixed literal, and the relative-time helpers run against
// an injected FixedClock, so the output is byte-for-byte deterministic.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/malcolmston/moment"
)

var (
	checks   int
	failures int
)

// check compares a value against a hand-computed expectation.
func check(label string, got, want any) {
	checks++
	status := "ok"
	if fmt.Sprint(got) != fmt.Sprint(want) {
		status = "MISMATCH"
		failures++
	}
	fmt.Printf("  [%-8s] %-46s got=%v want=%v\n", status, label, got, want)
}

func section(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

// ref is the reference instant used throughout: Friday 14 July 2017,
// 02:40:09.123 UTC.
var ref = moment.DateTime(2017, time.July, 14, 2, 40, 9, 123*int(time.Millisecond), time.UTC)

func main() {
	parsing()
	formatting()
	manipulation()
	startEndOf()
	diffing()
	relative()
	durations()
	zones()
	invalidInput()

	fmt.Printf("\n=== summary ===\n  %d checks, %d mismatches\n", checks, failures)
	if failures > 0 {
		// Report but still exit 0 so the example remains a demo, not a test.
		fmt.Fprintf(os.Stderr, "note: %d hand-computed expectations did not match\n", failures)
	}
}

func parsing() {
	section("parsing")

	// ISO 8601 via the auto-detecting Parse.
	m, err := moment.Parse("2017-07-14T02:40:09.123Z")
	fmt.Printf("  Parse(ISO)              -> %s (err=%v)\n", m.ISO(), err)
	check("Parse ISO instant", m.UnixMilli(), ref.UnixMilli())

	// A date-only ISO value.
	d, err := moment.ParseISO("2017-07-14")
	fmt.Printf("  ParseISO(date only)     -> %s (err=%v)\n", d.ISO(), err)
	check("ParseISO date-only midnight", d.Format("YYYY-MM-DD HH:mm:ss"), "2017-07-14 00:00:00")

	// RFC 2822.
	r, err := moment.ParseRFC2822("Fri, 14 Jul 2017 02:40:09 +0000")
	fmt.Printf("  ParseRFC2822            -> %s (err=%v)\n", r.ISO(), err)
	check("RFC2822 weekday", r.Format("dddd"), "Friday")

	// moment-style token parsing.
	t1, err := moment.ParseFormat("14/07/2017 02:40", "DD/MM/YYYY HH:mm")
	fmt.Printf("  ParseFormat(DD/MM/YYYY) -> %s (err=%v)\n", t1.ISO(), err)
	check("token parse", t1.Format("YYYY-MM-DD HH:mm"), "2017-07-14 02:40")

	// 12-hour clock with a meridiem and a month name.
	t2, err := moment.ParseFormat("July 4 2017 2:05 pm", "MMMM D YYYY h:mm a")
	fmt.Printf("  ParseFormat(meridiem)   -> %s (err=%v)\n", t2.ISO(), err)
	check("meridiem parse -> 14:05", t2.Format("HH:mm"), "14:05")

	// Multi-format parsing: first layout that fits wins.
	layouts := []string{"YYYY-MM-DD", "DD.MM.YYYY", "MMM D, YYYY"}
	for _, in := range []string{"2017-07-14", "14.07.2017", "Jul 14, 2017"} {
		p, err := moment.ParseFormats(in, layouts)
		fmt.Printf("  ParseFormats(%-12q) -> %s (err=%v)\n", in, p.Format("YYYY-MM-DD"), err)
		check("ParseFormats "+in, p.Format("YYYY-MM-DD"), "2017-07-14")
	}

	// Parsing into an explicit location.
	nyc, err := moment.ParseInLocation("2017-07-14 02:40", "YYYY-MM-DD HH:mm", mustZone("America/New_York"))
	fmt.Printf("  ParseInLocation(NY)     -> %s (err=%v)\n", nyc.ISO(), err)
	check("wall clock kept, offset -04:00", nyc.Format("YYYY-MM-DDTHH:mmZ"), "2017-07-14T02:40-04:00")

	// Raw Go layouts are available too.
	gl, err := moment.ParseLayout("2017-07-14 02:40:09", "2006-01-02 15:04:05")
	fmt.Printf("  ParseLayout(Go layout)  -> %s (err=%v)\n", gl.ISO(), err)

	// Unix seconds / millis tokens.
	ux, err := moment.ParseFormat("1500000000", "X")
	fmt.Printf("  ParseFormat(X)          -> %s (err=%v)\n", ux.UTC().ISO(), err)
	check("unix seconds token", ux.Unix(), int64(1500000000))

	// CreationData records how a value was produced.
	if cd := t1.CreationData(); cd != nil {
		fmt.Printf("  CreationData            -> input=%q format=%q valid=%v\n", cd.Input, cd.Format, cd.Valid)
	}
}

func formatting() {
	section("formatting")

	for _, f := range []string{
		"YYYY-MM-DD HH:mm:ss.SSS",
		"dddd, MMMM Do YYYY",
		"ddd MMM D, YY [at] h:mm A",
		"Qo [quarter], DDDo [day of year], wo [week]",
		"E e GGGG-[W]WW gggg-ww",
		"k kk H HH h hh a A",
		"X x",
		"Z ZZ z",
		"LT | LTS | L | LL | LLL | LLLL",
	} {
		fmt.Printf("  %-38s -> %s\n", f, ref.Format(f))
	}

	check("ISO()", ref.ISO(), "2017-07-14T02:40:09.123Z")
	check("dddd", ref.Format("dddd"), "Friday")
	check("Do ordinal", ref.Format("Do"), "14th")
	check("Qo quarter", ref.Format("Qo"), "3rd")
	check("DDD day-of-year", ref.Format("DDD"), "195")
	check("bracketed literal", ref.Format("[Y] YYYY"), "Y 2017")

	// Locale-aware formatting.
	fmt.Printf("  fr LLLL                                -> %s\n", ref.Locale("fr").Format("LLLL"))
	fmt.Printf("  de LLLL                                -> %s\n", ref.Locale("de").Format("LLLL"))
	fmt.Printf("  available locales                      -> %v\n", moment.AvailableLocales())

	// Queries used by the checks above.
	y, w := ref.ISOWeek()
	fmt.Printf("  ISOWeek=%d-W%02d quarter=%d dayOfYear=%d daysInMonth=%d leap=%v\n",
		y, w, ref.Quarter(), ref.DayOfYear(), ref.DaysInMonth(), ref.IsLeapYear())
	check("ISO week of 2017-07-14", fmt.Sprintf("%d-W%d", y, w), "2017-W28")
	check("days in July", ref.DaysInMonth(), 31)
	check("2017 is not a leap year", ref.IsLeapYear(), false)
}

func manipulation() {
	section("manipulation (immutable)")

	plus3d := ref.Add(3, moment.Day)
	fmt.Printf("  ref                     -> %s\n", ref.ISO())
	fmt.Printf("  ref.Add(3, Day)         -> %s\n", plus3d.ISO())
	check("receiver unchanged", ref.ISO(), "2017-07-14T02:40:09.123Z")
	check("Add 3 days", plus3d.Format("YYYY-MM-DD"), "2017-07-17")

	// End-of-month clamping, the classic moment.js behaviour.
	jan31 := moment.DateTime(2017, time.January, 31, 12, 0, 0, 0, time.UTC)
	check("2017-01-31 + 1 month -> Feb 28", jan31.Add(1, moment.Month).Format("YYYY-MM-DD"), "2017-02-28")
	check("2017-01-31 + 1 month + 1 month", jan31.Add(1, moment.Month).Add(1, moment.Month).Format("YYYY-MM-DD"), "2017-03-28")
	check("2016-02-29 + 1 year -> Feb 28", moment.DateTime(2016, time.February, 29, 0, 0, 0, 0, time.UTC).
		Add(1, moment.Year).Format("YYYY-MM-DD"), "2017-02-28")

	// Aliases for units.
	check("alias \"days\"", ref.Add(1, moment.Unit("days")).Format("YYYY-MM-DD"), "2017-07-15")
	check("alias \"Q\"", ref.Add(1, moment.Unit("Q")).Format("YYYY-MM-DD"), "2017-10-14")

	check("Subtract 3 weeks", ref.Subtract(3, moment.Week).Format("YYYY-MM-DD"), "2017-06-23")
	check("Subtract 90 minutes", ref.Subtract(90, moment.Minute).Format("HH:mm"), "01:10")
	check("AddDuration(36h)", ref.AddDuration(36*time.Hour).Format("YYYY-MM-DD HH:mm"), "2017-07-15 14:40")

	// Setters.
	set := ref.SetYear(2020).SetMonth(time.December).SetDate(25).SetHour(9).SetMinute(30).SetSecond(0).SetMillisecond(0)
	// ISO() is RFC3339Nano, so it drops an all-zero fraction; ToISOString() is
	// the moment.js toISOString with fixed millisecond precision.
	check("chained setters (ISO)", set.ISO(), "2020-12-25T09:30:00Z")
	check("chained setters (ToISOString)", set.ToISOString(), "2020-12-25T09:30:00.000Z")
	check("SetAll", ref.SetAll(moment.DateSpec{Year: iptr(1999), Month: iptr(int(time.December)), Day: iptr(31)}).
		Format("YYYY-MM-DD"), "1999-12-31")
	check("SetWeekday(Monday)", ref.SetWeekday(time.Monday).Format("YYYY-MM-DD ddd"), "2017-07-10 Mon")
	check("SetDayOfYear(1)", ref.SetDayOfYear(1).Format("YYYY-MM-DD"), "2017-01-01")

	// Max / Min and comparison helpers.
	a := moment.DateTime(2017, time.January, 1, 0, 0, 0, 0, time.UTC)
	b := moment.DateTime(2018, time.January, 1, 0, 0, 0, 0, time.UTC)
	check("Max", moment.Max(a, ref, b).Format("YYYY"), "2018")
	check("Min", moment.Min(a, ref, b).Format("YYYY"), "2017")
	check("IsBetween(a,b)", ref.IsBetween(a, b, false), true)
	check("IsSameUnit(month)", ref.IsSameUnit(ref.Add(3, moment.Day), moment.Month), true)
	check("ToArray", fmt.Sprint(ref.ToArray()), "[2017 6 14 2 40 9 123]")
}

func startEndOf() {
	section("start/end of unit")

	for _, u := range []moment.Unit{
		moment.Year, moment.Quarter, moment.Month, moment.Week,
		moment.ISOWeek, moment.Day, moment.Hour, moment.Minute,
	} {
		fmt.Printf("  %-12s start=%-28s end=%s\n", u,
			ref.StartOf(u).Format("YYYY-MM-DD HH:mm:ss.SSS"),
			ref.EndOf(u).Format("YYYY-MM-DD HH:mm:ss.SSS"))
	}

	check("StartOf(Year)", ref.StartOf(moment.Year).ToISOString(), "2017-01-01T00:00:00.000Z")
	check("StartOf(Quarter) -> Jul 1", ref.StartOf(moment.Quarter).Format("YYYY-MM-DD"), "2017-07-01")
	// NOTE: EndOf lands on the last *nanosecond*, not the last millisecond as
	// moment.js does. Documented, but it means EndOf(u).Millisecond() == 999
	// while the instant is .999999999.
	check("EndOf(Month) is last nanosecond", ref.EndOf(moment.Month).ISO(), "2017-07-31T23:59:59.999999999Z")
	check("EndOf(Month).Millisecond()", ref.EndOf(moment.Month).Millisecond(), 999)
	// 2017-07-14 is a Friday: the ISO week starts Monday the 10th, the
	// en-locale week starts Sunday the 9th.
	check("StartOf(ISOWeek) -> Mon Jul 10", ref.StartOf(moment.ISOWeek).Format("YYYY-MM-DD ddd"), "2017-07-10 Mon")
	check("StartOf(Week) -> Sun Jul 9", ref.StartOf(moment.Week).Format("YYYY-MM-DD ddd"), "2017-07-09 Sun")
	check("EndOf(Day)", ref.EndOf(moment.Day).ISO(), "2017-07-14T23:59:59.999999999Z")
}

func diffing() {
	section("diffing")

	from := moment.DateTime(2017, time.January, 31, 0, 0, 0, 0, time.UTC)
	to := moment.DateTime(2017, time.July, 14, 12, 0, 0, 0, time.UTC)
	fmt.Printf("  from=%s to=%s\n", from.Format("YYYY-MM-DD HH:mm"), to.Format("YYYY-MM-DD HH:mm"))
	for _, u := range []moment.Unit{moment.Year, moment.Month, moment.Week, moment.Day, moment.Hour, moment.Minute} {
		fmt.Printf("  %-12s Diff=%12.6f DiffInt=%d\n", u, to.Diff(from, u), to.DiffInt(from, u))
	}

	// 31 Jan -> 14 Jul 2017 is 164 days and 12 hours.
	check("DiffInt days", to.DiffInt(from, moment.Day), 164)
	check("DiffInt hours", to.DiffInt(from, moment.Hour), 164*24+12)
	check("DiffInt months (truncating)", to.DiffInt(from, moment.Month), 5)
	check("DiffInt years", to.DiffInt(from, moment.Year), 0)
	check("sign flips", from.DiffInt(to, moment.Day), -164)
	check("DiffDuration", to.DiffDuration(from), 164*24*time.Hour+12*time.Hour)
	check("whole-month diff is exact", moment.DateTime(2017, time.March, 1, 0, 0, 0, 0, time.UTC).
		Diff(moment.DateTime(2017, time.January, 1, 0, 0, 0, 0, time.UTC), moment.Month), float64(2))
}

func relative() {
	section("relative time (injected fixed clock)")

	clock := moment.FixedClock(ref.Time())
	at := func(d time.Duration) moment.Moment { return ref.AddDuration(d).WithClock(clock) }

	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-20 * time.Second, "a few seconds ago"},
		{-5 * time.Minute, "5 minutes ago"},
		{-3 * time.Hour, "3 hours ago"},
		{-30 * 24 * time.Hour, "a month ago"},
		{45 * time.Second, "in a minute"},
		{2 * time.Hour, "in 2 hours"},
		{3 * 24 * time.Hour, "in 3 days"},
		{400 * 24 * time.Hour, "in a year"},
	} {
		got := at(tc.d).FromNow()
		check(fmt.Sprintf("FromNow(%v)", tc.d), got, tc.want)
	}

	// From / To / ToNow against explicit references.
	other := ref.Add(2, moment.Day)
	check("From(other)", ref.From(other), "2 days ago")
	check("To(other)", ref.To(other), "in 2 days")
	check("ToNow", at(-3*time.Hour).ToNow(), "in 3 hours")

	// Localized relative time.
	fmt.Printf("  fr in 3 days            -> %s\n", at(3*24*time.Hour).Locale("fr").FromNow())
	fmt.Printf("  de in 3 days            -> %s\n", at(3*24*time.Hour).Locale("de").FromNow())
	fmt.Printf("  ru in 3 days            -> %s\n", at(3*24*time.Hour).Locale("ru").FromNow())

	// Calendar strings relative to a fixed reference.
	for _, d := range []int{-8, -1, 0, 1, 4} {
		c := ref.Add(d, moment.Day).Calendar(ref)
		fmt.Printf("  Calendar(ref%+d day)     -> %s\n", d, c)
	}
	check("Calendar today", ref.Calendar(ref), "Today at 2:40 AM")
	check("Calendar yesterday", ref.Subtract(1, moment.Day).Calendar(ref), "Yesterday at 2:40 AM")

	// Package-level humanizer, and threshold tuning.
	check("Humanize(90m)", moment.Humanize(90*time.Minute), "2 hours")
	prev := moment.RelativeTimeThreshold("m")
	if moment.SetRelativeTimeThreshold("m", 55) {
		check("Humanize(56m) with m=55", moment.Humanize(56*time.Minute), "an hour")
		moment.SetRelativeTimeThreshold("m", prev)
	}
	check("threshold restored", moment.RelativeTimeThreshold("m"), prev)
}

func durations() {
	section("durations")

	d := moment.NewDuration(1, moment.Year).Add(moment.NewDuration(2, moment.Month))
	check("ISOString", d.ISOString(), "P1Y2M")
	check("AsMonths", d.AsMonths(), float64(14))

	rt, err := moment.ParseDuration("P1Y2M10DT2H30M")
	fmt.Printf("  ParseDuration           -> %s (err=%v)\n", rt.ISOString(), err)
	check("ISO duration round trip", rt.ISOString(), "P1Y2M10DT2H30M")
	fmt.Printf("  components              -> y=%d mo=%d d=%d h=%d mi=%d\n",
		rt.Years(), rt.Months(), rt.Days(), rt.Hours(), rt.Minutes())

	between := moment.DurationBetween(ref, ref.Add(1, moment.Year).Add(2, moment.Day))
	fmt.Printf("  DurationBetween         -> %s (%.2f days) humanize=%q\n",
		between.ISOString(), between.AsDays(), between.Humanize(false))

	check("DurationFromTime(2h30m)", moment.DurationFromTime(150*time.Minute).ISOString(), "PT2H30M")
	check("Abs of negative", moment.NewDuration(-3, moment.Hour).Abs().ISOString(), "PT3H")
	check("Subtract", moment.NewDuration(3, moment.Hour).Subtract(moment.NewDuration(30, moment.Minute)).ISOString(), "PT2H30M")
	check("Humanize with suffix", moment.NewDuration(3, moment.Day).Humanize(true), "in 3 days")
	fmt.Printf("  de humanize             -> %s\n", moment.NewDuration(3, moment.Day).Locale("de").Humanize(true))
}

func zones() {
	section("time zones and UTC")

	nyc := mustZone("America/New_York")
	tokyo := mustZone("Asia/Tokyo")

	fmt.Printf("  ref (UTC)               -> %s  zone=%s offset=%d\n", ref.Format("YYYY-MM-DD HH:mm Z"), ref.ZoneAbbr(), ref.UTCOffset())
	inNY := ref.In(nyc)
	inTokyo := ref.In(tokyo)
	fmt.Printf("  In(New_York)            -> %s  zone=%s offset=%d dst=%v\n",
		inNY.Format("YYYY-MM-DD HH:mm Z"), inNY.ZoneAbbr(), inNY.UTCOffset(), inNY.IsDST())
	fmt.Printf("  In(Tokyo)               -> %s  zone=%s offset=%d dst=%v\n",
		inTokyo.Format("YYYY-MM-DD HH:mm Z"), inTokyo.ZoneAbbr(), inTokyo.UTCOffset(), inTokyo.IsDST())

	// 02:40 UTC on 14 July is 22:40 EDT on 13 July (UTC-4, DST in effect).
	check("UTC -> New York wall clock", inNY.Format("YYYY-MM-DD HH:mm"), "2017-07-13 22:40")
	check("New York offset in July", inNY.UTCOffset(), -240)
	check("New York DST in July", inNY.IsDST(), true)
	check("Tokyo offset", inTokyo.UTCOffset(), 540)
	check("Tokyo has no DST", inTokyo.IsDST(), false)
	check("instant preserved across zones", inNY.UnixMilli(), ref.UnixMilli())
	check("IsUTC", ref.IsUTC(), true)
	check("round trip back to UTC", inTokyo.UTC().ISO(), "2017-07-14T02:40:09.123Z")

	// StartOf(Day) is local to the value's zone.
	check("StartOf(Day) in New York", inNY.StartOf(moment.Day).Format("YYYY-MM-DDTHH:mmZ"), "2017-07-13T00:00-04:00")

	// A parsed offset is kept by ParseZone.
	pz, err := moment.ParseZone("2017-07-14T02:40:09+05:30")
	fmt.Printf("  ParseZone(+05:30)       -> %s (err=%v) offset=%d\n", pz.Format("YYYY-MM-DDTHH:mmZ"), err, pz.UTCOffset())
	check("ParseZone keeps offset", pz.UTCOffset(), 330)

	// SetUTCOffset shifts the rendering zone without moving the instant.
	off := ref.SetUTCOffset(-330)
	check("SetUTCOffset(-330) instant", off.UnixMilli(), ref.UnixMilli())
	check("SetUTCOffset(-330) render", off.Format("YYYY-MM-DDTHH:mmZ"), "2017-07-13T21:10-05:30")

	// A DST spring-forward boundary in New York: 2017-03-12 02:30 does not exist.
	dst := moment.DateTime(2017, time.March, 12, 1, 30, 0, 0, nyc)
	fmt.Printf("  NY 2017-03-12 01:30     -> %s offset=%d\n", dst.Format("HH:mm Z"), dst.UTCOffset())
	fmt.Printf("  + 1 hour                -> %s offset=%d\n", dst.Add(1, moment.Hour).Format("HH:mm Z"), dst.Add(1, moment.Hour).UTCOffset())
	check("crossing spring forward skips 02:00", dst.Add(1, moment.Hour).Format("HH:mm"), "03:30")
}

func invalidInput() {
	section("invalid input")

	for _, tc := range []struct{ value, format string }{
		{"not a date", "YYYY-MM-DD"},
		{"14/07/2017", "YYYY-MM-DD"},
		{"", "YYYY-MM-DD"},
	} {
		m, err := moment.ParseFormat(tc.value, tc.format)
		fmt.Printf("  ParseFormat(%-12q, %q) -> valid=%v err=%v\n", tc.value, tc.format, m.IsValid(), err != nil)
		check("invalid parse is !IsValid: "+tc.value, m.IsValid(), false)
	}

	// HOLE: out-of-range components are silently normalized instead of being
	// rejected. moment.js reports month 13 / day 45 as an invalid date even in
	// non-strict mode; here time.Date rolls them over into the next year, and
	// even ParseFormatStrict accepts the value.
	for _, fn := range []struct {
		name string
		f    func(string, string) (moment.Moment, error)
	}{
		{"ParseFormat", moment.ParseFormat},
		{"ParseFormatStrict", moment.ParseFormatStrict},
	} {
		for _, in := range []string{"2017-13-45", "2017-07-32", "2017-02-30"} {
			bad, err := fn.f(in, "YYYY-MM-DD")
			fmt.Printf("  %-17s(%-12q) -> valid=%v err=%v normalized=%s\n",
				fn.name, in, bad.IsValid(), err, bad.Format("YYYY-MM-DD"))
		}
	}

	if _, err := moment.Parse("total nonsense"); err != nil {
		fmt.Printf("  Parse(nonsense)         -> err=%v\n", err)
	} else {
		fmt.Println("  Parse(nonsense)         -> UNEXPECTEDLY SUCCEEDED")
	}

	// Strict parsing rejects trailing garbage that the lenient parser accepts.
	lenient, lerr := moment.ParseFormat("2017-07-14 trailing", "YYYY-MM-DD")
	strict, serr := moment.ParseFormatStrict("2017-07-14 trailing", "YYYY-MM-DD")
	fmt.Printf("  lenient trailing garbage-> valid=%v err=%v\n", lenient.IsValid(), lerr)
	fmt.Printf("  strict  trailing garbage-> valid=%v err=%v\n", strict.IsValid(), serr != nil)
	check("strict rejects trailing garbage", strict.IsValid(), false)

	// The explicitly invalid Moment.
	inv := moment.Invalid()
	check("Invalid().IsValid()", inv.IsValid(), false)
	check("Invalid().Format()", inv.Format("YYYY-MM-DD"), "Invalid date")
	if cd := inv.CreationData(); cd != nil {
		check("Invalid CreationData.Valid", cd.Valid, false)
	}

	// The zero value is invalid but does not panic. NOTE: unlike Invalid(), it
	// formats as a real date rather than "Invalid date".
	var zero moment.Moment
	fmt.Printf("  zero value              -> IsZero=%v IsValid=%v Format=%q\n",
		zero.IsZero(), zero.IsValid(), zero.Format("YYYY-MM-DD"))
	check("zero value is !IsValid", zero.IsValid(), false)
	check("zero value ToJSON is empty", zero.ToJSON(), "")

	// A bad ISO duration is reported too.
	if _, err := moment.ParseDuration("P1X"); err != nil {
		fmt.Printf("  ParseDuration(\"P1X\")     -> err=%v\n", err)
	} else {
		fmt.Println("  ParseDuration(\"P1X\")     -> UNEXPECTEDLY SUCCEEDED")
	}
}

func iptr(v int) *int { return &v }

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
