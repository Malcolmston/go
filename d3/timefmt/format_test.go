package timefmt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// reference is a Saturday in a leap year, in the afternoon, with a fractional
// second — chosen so that every directive has something interesting to say:
// %p is PM, %I differs from %H, %q is the first quarter, %V and %U and %W all
// differ from one another, and %f exercises sub-millisecond precision.
var reference = time.Date(2024, time.March, 9, 14, 5, 6, 123456789, time.UTC)

// TestFormatDirectives asserts the exact rendering of every directive. These
// are value assertions because each directive has a single documented meaning
// and the reference instant is fully specified.
func TestFormatDirectives(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		{"%a", "Sat"},
		{"%A", "Saturday"},
		{"%b", "Mar"},
		{"%B", "March"},
		{"%c", "3/9/2024, 2:05:06 PM"},
		{"%d", "09"},
		{"%e", " 9"},
		{"%f", "123456"},
		{"%g", "24"},
		{"%G", "2024"},
		{"%H", "14"},
		{"%I", "02"},
		{"%j", "069"}, // 31 + 29 (leap February) + 9
		{"%L", "123"},
		{"%m", "03"},
		{"%M", "05"},
		{"%p", "PM"},
		{"%q", "1"},
		{"%Q", "1709993106123"},
		{"%s", "1709993106"},
		{"%S", "06"},
		{"%u", "6"}, // Saturday is 6 with Monday first
		{"%U", "09"},
		{"%V", "10"},
		{"%w", "6"}, // Saturday is 6 with Sunday first
		{"%W", "10"},
		{"%x", "3/9/2024"},
		{"%X", "2:05:06 PM"},
		{"%y", "24"},
		{"%Y", "2024"},
		{"%Z", "+0000"},
		{"%%", "%"},

		// Padding modifiers.
		{"%-d", "9"},
		{"%_d", " 9"},
		{"%0d", "09"},
		{"%-m/%-d/%Y", "3/9/2024"},
		{"%-H", "14"},
		{"%_j", " 69"},

		// Literal text passes through, and an unknown directive emits itself.
		{"on %A at %-I%p", "on Saturday at 2PM"},
		{"%K", "K"},
	}
	for _, tt := range tests {
		if got := Format(tt.spec)(reference); got != tt.want {
			t.Errorf("Format(%q) = %q, want %q", tt.spec, got, tt.want)
		}
	}
}

// TestFormatAMPM checks the half-day boundary, which %p and %I both hinge on
// and which is easy to get wrong at exactly noon and midnight.
func TestFormatAMPM(t *testing.T) {
	f := Format("%I %p")
	tests := []struct {
		hour int
		want string
	}{
		{0, "12 AM"},
		{1, "01 AM"},
		{11, "11 AM"},
		{12, "12 PM"},
		{13, "01 PM"},
		{23, "11 PM"},
	}
	for _, tt := range tests {
		tm := time.Date(2024, 1, 1, tt.hour, 0, 0, 0, time.UTC)
		if got := f(tm); got != tt.want {
			t.Errorf("hour %d = %q, want %q", tt.hour, got, tt.want)
		}
	}
}

// TestISOWeekEdgeCases: the ISO week-based year is the directive most likely to
// be wrong, because at the turn of the year it disagrees with the calendar
// year. January 1, 2023 was a Sunday, so it belongs to ISO week 52 of 2022.
func TestISOWeekEdgeCases(t *testing.T) {
	f := Format("%G-W%V-%u")
	tests := []struct {
		date time.Time
		want string
	}{
		{time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), "2022-W52-7"},
		{time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC), "2023-W01-1"},
		{time.Date(2019, 12, 30, 0, 0, 0, 0, time.UTC), "2020-W01-1"},
		{time.Date(2021, 1, 4, 0, 0, 0, 0, time.UTC), "2021-W01-1"},
		{time.Date(2020, 12, 31, 0, 0, 0, 0, time.UTC), "2020-W53-4"},
	}
	for _, tt := range tests {
		if got := f(tt.date); got != tt.want {
			t.Errorf("%v = %q, want %q", tt.date.Format("2006-01-02"), got, tt.want)
		}
	}
}

// TestWeekNumbering pins %U and %W, which count from the first Sunday and the
// first Monday of the year respectively and so start at zero for the partial
// week before it.
func TestWeekNumbering(t *testing.T) {
	// January 1, 2024 was a Monday: %W is already week 1, %U is still week 0.
	tests := []struct {
		date time.Time
		u, w string
	}{
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "00", "01"},
		{time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC), "00", "01"},
		{time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC), "01", "01"},
		{time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC), "01", "02"},
		{time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), "52", "53"},
	}
	fu, fw := Format("%U"), Format("%W")
	for _, tt := range tests {
		if got := fu(tt.date); got != tt.u {
			t.Errorf("%%U of %v = %q, want %q", tt.date.Format("2006-01-02"), got, tt.u)
		}
		if got := fw(tt.date); got != tt.w {
			t.Errorf("%%W of %v = %q, want %q", tt.date.Format("2006-01-02"), got, tt.w)
		}
	}
}

// TestZoneDirective: %Z reports the offset of the value's own location, and
// UTCFormat forces it to zero.
func TestZoneDirective(t *testing.T) {
	f := Format("%Z")
	tests := []struct {
		offset int // seconds east of UTC
		want   string
	}{
		{0, "+0000"},
		{-7 * 3600, "-0700"},
		{5*3600 + 30*60, "+0530"},
		{-3*3600 - 30*60, "-0330"},
		{5*3600 + 45*60, "+0545"},
	}
	for _, tt := range tests {
		tm := reference.In(time.FixedZone("x", tt.offset))
		if got := f(tm); got != tt.want {
			t.Errorf("offset %d = %q, want %q", tt.offset, got, tt.want)
		}
		if got := UTCFormat("%Z")(tm); got != "+0000" {
			t.Errorf("UTCFormat %%Z of offset %d = %q, want %q", tt.offset, got, "+0000")
		}
	}
}

// TestRoundTripEveryDirective is the property test the task calls for: for each
// directive that carries enough information to be reconstructed, formatting,
// parsing and formatting again must reach a fixed point.
func TestRoundTripEveryDirective(t *testing.T) {
	specs := []string{
		"%Y-%m-%d",
		"%Y-%m-%dT%H:%M:%S",
		"%Y-%m-%dT%H:%M:%S.%L",
		"%Y-%m-%dT%H:%M:%S.%f",
		"%d/%m/%Y",
		"%e/%m/%Y",
		"%-m/%-d/%Y",
		"%B %d, %Y",
		"%b %d %Y",
		"%A, %B %-d, %Y",
		"%a %b %d %Y",
		"%Y %j",
		"%Y-%m-%d %I:%M:%S %p",
		"%Y-%m-%d %H:%M",
		"%y-%m-%d",
		"%G-W%V-%u",
		"%Y %U %w",
		"%Y %W %w",
		"%c",
		"%x %X",
		"%s",
		"%Q",
		"%Y-%m-%dT%H:%M:%S%Z",
		"%Y-%m-%d [%q]",
		"100%% on %Y-%m-%d",
	}
	instants := []time.Time{
		reference,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1999, 12, 31, 23, 59, 59, 999000000, time.UTC),
		time.Date(2000, 2, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2023, 1, 1, 6, 30, 0, 0, time.UTC),
		time.Date(2020, 7, 4, 18, 45, 30, 0, time.UTC),
	}
	for _, spec := range specs {
		format, parse := UTCFormat(spec), UTCParse(spec)
		for _, tm := range instants {
			s := format(tm)
			back, err := parse(s)
			if err != nil {
				t.Errorf("%q: parse(%q): %v", spec, s, err)
				continue
			}
			if again := format(back); again != s {
				t.Errorf("%q: %q -> %v -> %q", spec, s, back, again)
			}
		}
	}
}

// TestParseExactness: for specifiers that fully determine an instant, the parse
// must recover it exactly, not merely to a fixed point.
func TestParseExactness(t *testing.T) {
	tests := []struct {
		spec, in string
		want     time.Time
	}{
		{"%Y-%m-%d", "2024-03-09", time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%Y-%m-%dT%H:%M:%S", "2024-03-09T14:05:06", time.Date(2024, 3, 9, 14, 5, 6, 0, time.UTC)},
		{"%Y-%m-%dT%H:%M:%S.%L", "2024-03-09T14:05:06.123", time.Date(2024, 3, 9, 14, 5, 6, 123000000, time.UTC)},
		{"%Y-%m-%dT%H:%M:%S.%f", "2024-03-09T14:05:06.123456", time.Date(2024, 3, 9, 14, 5, 6, 123456000, time.UTC)},
		{"%B %-d, %Y", "March 9, 2024", time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%d-%b-%y", "09-Mar-24", time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%d-%b-%y", "09-Mar-69", time.Date(1969, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%d-%b-%y", "09-Mar-68", time.Date(2068, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%Y %j", "2024 069", time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)},
		{"%I:%M %p", "02:05 PM", time.Date(1900, 1, 1, 14, 5, 0, 0, time.UTC)},
		{"%I:%M %p", "12:00 AM", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%I:%M %p", "12:00 PM", time.Date(1900, 1, 1, 12, 0, 0, 0, time.UTC)},
		{"%Q", "1709993106123", time.Date(2024, 3, 9, 14, 5, 6, 123000000, time.UTC)},
		{"%s", "1709993106", time.Date(2024, 3, 9, 14, 5, 6, 0, time.UTC)},
		{"%G-W%V-%u", "2022-W52-7", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"%G-W%V-%u", "2023-W01-1", time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
		// Case-insensitive name matching, as in d3.
		{"%B %-d, %Y", "MARCH 9, 2024", time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		got, err := UTCParse(tt.spec)(tt.in)
		if err != nil {
			t.Errorf("Parse(%q)(%q): %v", tt.spec, tt.in, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("Parse(%q)(%q) = %v, want %v", tt.spec, tt.in, got, tt.want)
		}
	}
}

// TestParseZone: an offset in the input wins over the parser's location, and
// the result keeps the wall clock the input wrote.
func TestParseZone(t *testing.T) {
	p := UTCParse("%Y-%m-%dT%H:%M:%S%Z")
	tests := []struct {
		in   string
		want time.Time
	}{
		{"2024-03-09T14:05:06Z", time.Date(2024, 3, 9, 14, 5, 6, 0, time.UTC)},
		{"2024-03-09T14:05:06+0000", time.Date(2024, 3, 9, 14, 5, 6, 0, time.UTC)},
		{"2024-03-09T14:05:06-0700", time.Date(2024, 3, 9, 21, 5, 6, 0, time.UTC)},
		{"2024-03-09T14:05:06-07:00", time.Date(2024, 3, 9, 21, 5, 6, 0, time.UTC)},
		{"2024-03-09T14:05:06+0530", time.Date(2024, 3, 9, 8, 35, 6, 0, time.UTC)},
		{"2024-03-09T14:05:06-07", time.Date(2024, 3, 9, 21, 5, 6, 0, time.UTC)},
	}
	for _, tt := range tests {
		got, err := p(tt.in)
		if err != nil {
			t.Errorf("parse(%q): %v", tt.in, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("parse(%q) = %v, want %v", tt.in, got.UTC(), tt.want)
		}
		// The wall clock survives, so re-formatting gives the input back.
		if again := Format("%Y-%m-%dT%H:%M:%S%Z")(got); !strings.HasPrefix(again, "2024-03-09T14:05:06") {
			t.Errorf("parse(%q) lost its wall clock: %q", tt.in, again)
		}
	}
}

// TestParseErrors: parsing is strict — the whole string must be consumed and
// literals must match.
func TestParseErrors(t *testing.T) {
	tests := []struct{ spec, in string }{
		{"%Y-%m-%d", "2024-03"},          // input ends early
		{"%Y-%m-%d", "2024-03-09extra"},  // trailing input
		{"%Y-%m-%d", "2024/03/09"},       // literal mismatch
		{"%Y-%m-%d", "not a date"},       // no digits
		{"%B %d, %Y", "Marbles 9, 2024"}, // unknown month name
		{"%G-W%V-%u", "2024-W99-1"},      // ISO week out of range
		{"%p", "XM"},                     // unknown period
	}
	for _, tt := range tests {
		if _, err := UTCParse(tt.spec)(tt.in); !errors.Is(err, ErrParse) {
			t.Errorf("Parse(%q)(%q) error = %v, want ErrParse", tt.spec, tt.in, err)
		}
	}
}

// TestISOHelpers: the ISO round trip is the one every data feed relies on.
func TestISOHelpers(t *testing.T) {
	tm := time.Date(2024, 3, 9, 14, 5, 6, 123000000, time.UTC)
	const want = "2024-03-09T14:05:06.123Z"
	if got := ISOFormat(tm); got != want {
		t.Errorf("ISOFormat = %q, want %q", got, want)
	}
	back, err := ISOParse(want)
	if err != nil {
		t.Fatalf("ISOParse: %v", err)
	}
	if !back.Equal(tm) {
		t.Errorf("ISOParse = %v, want %v", back, tm)
	}
	// An instant in another zone is normalized to UTC first.
	if got := ISOFormat(tm.In(time.FixedZone("x", -7*3600))); got != want {
		t.Errorf("ISOFormat of a zoned value = %q, want %q", got, want)
	}
}

// TestLocaleIndependence: the default locale is a fixed table, so the output
// never depends on the host's LANG or on time.Local.
func TestLocaleIndependence(t *testing.T) {
	f := Format("%A %B %p")
	if got := f(reference); got != "Saturday March PM" {
		t.Errorf("default locale = %q", got)
	}
	// The same instant in another zone still uses English names.
	if got := f(reference.In(time.FixedZone("x", 3600))); !strings.Contains(got, "March") {
		t.Errorf("zoned = %q", got)
	}
}

// TestCustomLocale exercises the locale hooks with names that share prefixes,
// which is where a naive first-match lookup would break.
func TestCustomLocale(t *testing.T) {
	fr := NewTimeLocale(TimeLocale{
		DateTime:    "%A %e %B %Y à %X",
		Date:        "%d/%m/%Y",
		Time:        "%H:%M:%S",
		Periods:     [2]string{"AM", "PM"},
		Days:        [7]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"},
		ShortDays:   [7]string{"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam."},
		Months:      [12]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		ShortMonths: [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
	})
	if got := fr.Format("%A %-d %B %Y")(reference); got != "samedi 9 mars 2024" {
		t.Errorf("fr = %q", got)
	}
	if got := fr.Format("%x")(reference); got != "09/03/2024" {
		t.Errorf("fr %%x = %q", got)
	}
	back, err := fr.UTCParse("%A %-d %B %Y")("samedi 9 mars 2024")
	if err != nil {
		t.Fatalf("fr parse: %v", err)
	}
	if want := time.Date(2024, 3, 9, 0, 0, 0, 0, time.UTC); !back.Equal(want) {
		t.Errorf("fr parse = %v, want %v", back, want)
	}
	// "mai" and "mars" share no prefix, but "juin"/"juillet" do in the short
	// list ("juin" vs "juil."); longest-match must pick the right one.
	for m := 1; m <= 12; m++ {
		tm := time.Date(2024, time.Month(m), 15, 0, 0, 0, 0, time.UTC)
		s := fr.Format("%b %-d %Y")(tm)
		back, err := fr.UTCParse("%b %-d %Y")(s)
		if err != nil {
			t.Errorf("fr short month %d (%q): %v", m, s, err)
			continue
		}
		if back.Month() != tm.Month() {
			t.Errorf("fr short month %q parsed as %v", s, back.Month())
		}
	}
}

// TestFormatInValueLocation: the formatter reads the instant in whatever
// location it carries, which is the Go-native version of d3's local/UTC split.
func TestFormatInValueLocation(t *testing.T) {
	tm := time.Date(2024, 3, 9, 14, 0, 0, 0, time.UTC)
	if got := Format("%H")(tm.In(time.FixedZone("x", 3*3600))); got != "17" {
		t.Errorf("local format = %q, want %q", got, "17")
	}
	if got := UTCFormat("%H")(tm.In(time.FixedZone("x", 3*3600))); got != "14" {
		t.Errorf("utc format = %q, want %q", got, "14")
	}
}
