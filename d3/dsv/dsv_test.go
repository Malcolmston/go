package dsv

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// TestParseRows covers the RFC 4180 grammar case by case. These are the
// behaviours the format actually specifies, so every one of them is asserted by
// value rather than by property.
func TestParseRows(t *testing.T) {
	tests := []struct {
		name  string
		delim rune
		in    string
		want  [][]string
	}{
		{"plain records", Comma, "a,b,c\n1,2,3", [][]string{{"a", "b", "c"}, {"1", "2", "3"}}},
		{"single field", Comma, "hello", [][]string{{"hello"}}},
		{"empty input", Comma, "", [][]string{}},

		// Quoting.
		{"quoted field containing the delimiter", Comma,
			"a,b\n\"x,y\",2", [][]string{{"a", "b"}, {"x,y", "2"}}},
		{"quoted field containing doubled quotes", Comma,
			"a\n\"say \"\"hi\"\"\"", [][]string{{"a"}, {`say "hi"`}}},
		{"a field that is only a quoted quote", Comma,
			"a\n\"\"\"\"", [][]string{{"a"}, {`"`}}},
		{"quoted field containing a newline", Comma,
			"a,b\n\"line1\nline2\",2", [][]string{{"a", "b"}, {"line1\nline2", "2"}}},
		{"quoted field containing CRLF", Comma,
			"a,b\r\n\"line1\r\nline2\",2", [][]string{{"a", "b"}, {"line1\nline2", "2"}}},
		{"quoted empty field", Comma,
			"a,b\n\"\",2", [][]string{{"a", "b"}, {"", "2"}}},

		// Line endings and blank fields.
		{"CRLF record separators", Comma,
			"a,b\r\n1,2\r\n3,4", [][]string{{"a", "b"}, {"1", "2"}, {"3", "4"}}},
		{"a single trailing newline is not a record", Comma,
			"a,b\n1,2\n", [][]string{{"a", "b"}, {"1", "2"}}},
		{"a single trailing CRLF is not a record", Comma,
			"a,b\r\n1,2\r\n", [][]string{{"a", "b"}, {"1", "2"}}},
		{"empty final field", Comma,
			"a,b\n1,", [][]string{{"a", "b"}, {"1", ""}}},
		{"empty leading field", Comma,
			"a,b\n,2", [][]string{{"a", "b"}, {"", "2"}}},
		{"all fields empty", Comma,
			"a,b\n,", [][]string{{"a", "b"}, {"", ""}}},
		{"whitespace is significant", Comma,
			"a,b\n  x , y ", [][]string{{"a", "b"}, {"  x ", " y "}}},

		// Documented divergence from d3: blank lines vanish rather than
		// becoming a row of one empty field.
		{"blank lines are skipped", Comma,
			"a\n1\n\n2", [][]string{{"a"}, {"1"}, {"2"}}},

		// A leading UTF-8 BOM is stripped; one anywhere else is data.
		{"leading BOM is stripped", Comma,
			"\uFEFFa,b\n1,2", [][]string{{"a", "b"}, {"1", "2"}}},
		{"BOM elsewhere is data", Comma,
			"a,b\n\uFEFF1,2", [][]string{{"a", "b"}, {"\uFEFF1", "2"}}},

		// Ragged records are allowed; it is Parse that pads or truncates them.
		{"ragged records", Comma,
			"a,b\n1\n2,3,4", [][]string{{"a", "b"}, {"1"}, {"2", "3", "4"}}},

		// Other delimiters.
		{"tabs", Tab, "a\tb\n1\t2", [][]string{{"a", "b"}, {"1", "2"}}},
		{"tab-separated field containing a comma", Tab,
			"a\tb\nx,y\t2", [][]string{{"a", "b"}, {"x,y", "2"}}},
		{"semicolons", ';', "a;b\n1;2", [][]string{{"a", "b"}, {"1", "2"}}},
		{"pipes", '|', "a|b\n1|2", [][]string{{"a", "b"}, {"1", "2"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRows(tc.delim, tc.in)
			if err != nil {
				t.Fatalf("ParseRows(%q) error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseRows(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseRowsErrors(t *testing.T) {
	tests := []struct {
		name  string
		delim rune
		in    string
	}{
		{"unterminated quoted field", Comma, "a\n\"unterminated"},
		{"junk after a closing quote", Comma, "a,b\n\"x\"y,2"},
		// A bare quote inside an unquoted field is malformed under RFC 4180;
		// d3 accepts it, encoding/csv does not, and this package follows Go.
		{"bare quote in an unquoted field", Comma, "a\nx\"y"},
		{"quote as delimiter", '"', "a\"b"},
		{"newline as delimiter", '\n', "a\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRows(tc.delim, tc.in); err == nil {
				t.Errorf("ParseRows(%q) succeeded, want an error", tc.in)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantColumns []string
		wantRows    []Row
	}{
		{"header and rows", "a,b\n1,2\n3,4",
			[]string{"a", "b"},
			[]Row{{"a": "1", "b": "2"}, {"a": "3", "b": "4"}}},
		{"header only", "a,b",
			[]string{"a", "b"}, []Row{}},
		{"empty input", "",
			[]string{}, []Row{}},
		{"short record pads with empty strings", "a,b,c\n1,2",
			[]string{"a", "b", "c"},
			[]Row{{"a": "1", "b": "2", "c": ""}}},
		{"long record drops the surplus", "a,b\n1,2,3",
			[]string{"a", "b"},
			[]Row{{"a": "1", "b": "2"}}},
		{"quoted header names", "\"first,name\",b\n1,2",
			[]string{"first,name", "b"},
			[]Row{{"first,name": "1", "b": "2"}}},
		{"duplicate column names collapse", "a,a\n1,2",
			[]string{"a", "a"},
			[]Row{{"a": "2"}}},
		{"BOM stripped from the first column name", "\uFEFFid,name\n1,x",
			[]string{"id", "name"},
			[]Row{{"id": "1", "name": "x"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCSV(tc.in)
			if err != nil {
				t.Fatalf("ParseCSV(%q) error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got.Columns, tc.wantColumns) {
				t.Errorf("columns = %q, want %q", got.Columns, tc.wantColumns)
			}
			if !reflect.DeepEqual(got.Rows, tc.wantRows) {
				t.Errorf("rows = %v, want %v", got.Rows, tc.wantRows)
			}
		})
	}
}

func TestParseTSV(t *testing.T) {
	got, err := ParseTSV("name\tvalue\nalpha\t1\nbeta\t2\n")
	if err != nil {
		t.Fatal(err)
	}
	wantCols := []string{"name", "value"}
	wantRows := []Row{{"name": "alpha", "value": "1"}, {"name": "beta", "value": "2"}}
	if !reflect.DeepEqual(got.Columns, wantCols) {
		t.Errorf("columns = %q, want %q", got.Columns, wantCols)
	}
	if !reflect.DeepEqual(got.Rows, wantRows) {
		t.Errorf("rows = %v, want %v", got.Rows, wantRows)
	}
}

// TestFormatValue pins d3's quoting rule, which is narrower than
// encoding/csv.Writer's — the "not quoted" cases are as much a part of the
// contract as the quoted ones.
func TestFormatValue(t *testing.T) {
	tests := []struct {
		name  string
		delim rune
		in    string
		want  string
	}{
		{"plain", Comma, "abc", "abc"},
		{"empty", Comma, "", ""},
		{"contains the delimiter", Comma, "a,b", `"a,b"`},
		{"contains a quote", Comma, `a"b`, `"a""b"`},
		{"contains a newline", Comma, "a\nb", "\"a\nb\""},
		{"contains a carriage return", Comma, "a\rb", "\"a\rb\""},
		{"leading space is not quoted", Comma, " a", " a"},
		{"trailing space is not quoted", Comma, "a ", "a "},
		{`the literal \. is not quoted`, Comma, `\.`, `\.`},
		{"a comma is data in a TSV", Tab, "a,b", "a,b"},
		{"a tab is quoted in a TSV", Tab, "a\tb", "\"a\tb\""},
		{"a tab is data in a CSV", Comma, "a\tb", "a\tb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatValue(tc.delim, tc.in); got != tc.want {
				t.Errorf("FormatValue(%q, %q) = %q, want %q", tc.delim, tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatRows(t *testing.T) {
	tests := []struct {
		name  string
		delim rune
		in    [][]string
		want  string
	}{
		{"plain", Comma, [][]string{{"a", "b"}, {"1", "2"}}, "a,b\n1,2"},
		{"no trailing newline", Comma, [][]string{{"x"}}, "x"},
		{"empty", Comma, nil, ""},
		{"quoting applied", Comma, [][]string{{"a,b", `c"d`}}, `"a,b","c""d"`},
		{"tabs", Tab, [][]string{{"a", "b"}}, "a\tb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatRows(tc.delim, tc.in); got != tc.want {
				t.Errorf("FormatRows = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	rows := []Row{
		{"name": "alpha", "value": "1"},
		{"name": "b,eta", "value": ""},
	}
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"explicit columns", FormatCSV(rows, []string{"name", "value"}),
			"name,value\nalpha,1\n\"b,eta\","},
		{"column order is honoured", FormatCSV(rows, []string{"value", "name"}),
			"value,name\n1,alpha\n,\"b,eta\""},
		{"inferred columns are sorted", FormatCSV(rows, nil),
			"name,value\nalpha,1\n\"b,eta\","},
		{"a column absent from the rows is empty", FormatCSV(rows, []string{"name", "missing"}),
			"name,missing\nalpha,\n\"b,eta\","},
		{"TSV", FormatTSV(rows, []string{"name", "value"}),
			"name\tvalue\nalpha\t1\nb,eta\t"},
		{"body only", FormatBody(Comma, rows, []string{"name", "value"}),
			"alpha,1\n\"b,eta\","},
		{"no rows still emits the header", FormatCSV(nil, []string{"a", "b"}), "a,b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// TestRoundTrip is the strongest single check on the parse/format pair: any
// table that survives formatting and re-parsing unchanged is one whose quoting
// rules agree in both directions.
func TestRoundTrip(t *testing.T) {
	inputs := []string{
		"a,b\n1,2\n3,4",
		"a,b\n\"x,y\",2",
		"a,b\n\"say \"\"hi\"\"\",2",
		"a,b\n\"line1\nline2\",2",
		"a,b\n,",
		"a\n  spaced  ",
		"\"weird\"\"name\",b\n1,2",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			first, err := ParseCSV(in)
			if err != nil {
				t.Fatal(err)
			}
			out := first.Format(Comma)
			second, err := ParseCSV(out)
			if err != nil {
				t.Fatalf("reparsing %q: %v", out, err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Errorf("round trip changed the table:\n in: %q\nout: %q", in, out)
			}
		})
	}
}

func TestInferColumns(t *testing.T) {
	rows := []Row{{"b": "1", "a": "2"}, {"c": "3", "a": "4"}}
	want := []string{"a", "b", "c"}
	if got := InferColumns(rows); !reflect.DeepEqual(got, want) {
		t.Errorf("InferColumns = %q, want %q", got, want)
	}
	// Determinism matters more than the particular order: repeated calls over
	// unordered maps must agree.
	for i := 0; i < 20; i++ {
		if got := InferColumns(rows); !reflect.DeepEqual(got, want) {
			t.Fatalf("InferColumns is not deterministic: %q", got)
		}
	}
}

func TestAutoTypeValue(t *testing.T) {
	utc := func(y int, mo time.Month, d, h, mi, s, ns int) time.Time {
		return time.Date(y, mo, d, h, mi, s, ns, time.UTC)
	}
	tests := []struct {
		name string
		in   string
		want any
	}{
		// Nulls.
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"tab only", "\t", nil},

		// Booleans, case-sensitively.
		{"true", "true", true},
		{"false", "false", false},
		{"TRUE stays a string", "TRUE", "TRUE"},
		{"True stays a string", "True", "True"},

		// Numbers.
		{"integer", "42", 42.0},
		{"negative", "-1.5", -1.5},
		{"explicit plus", "+3", 3.0},
		{"exponent", "1.5e3", 1500.0},
		{"negative exponent", "2E-2", 0.02},
		{"leading dot", ".5", 0.5},
		{"trailing dot", "5.", 5.0},
		{"zero", "0", 0.0},
		{"surrounding whitespace is ignored", "  42  ", 42.0},
		{"hexadecimal", "0x1f", 31.0},
		{"binary", "0b101", 5.0},
		{"octal", "0o17", 15.0},
		{"Infinity", "Infinity", math.Inf(1)},
		{"-Infinity", "-Infinity", math.Inf(-1)},

		// Things that look numeric to Go's strconv but not to JavaScript.
		{"lowercase inf stays a string", "inf", "inf"},
		{"lowercase nan stays a string", "nan", "nan"},
		{"underscored digits stay a string", "1_000", "1_000"},
		{"hex float stays a string", "0x1p-2", "0x1p-2"},
		{"thousands separators stay a string", "1,000", "1,000"},
		{"trailing unit stays a string", "42px", "42px"},

		// Dates. Note the number test runs first, so a bare year is a number.
		{"bare year is a number, not a date", "2024", 2024.0},
		{"year and month", "2024-05", utc(2024, 5, 1, 0, 0, 0, 0)},
		{"date", "2024-05-06", utc(2024, 5, 6, 0, 0, 0, 0)},
		{"zulu date-time", "2024-05-06T12:30Z", utc(2024, 5, 6, 12, 30, 0, 0)},
		{"zulu with seconds", "2024-05-06T12:30:45Z", utc(2024, 5, 6, 12, 30, 45, 0)},
		{"zulu with milliseconds", "2024-05-06T12:30:45.123Z",
			utc(2024, 5, 6, 12, 30, 45, 123000000)},
		{"explicit offset", "2024-05-06T12:30:45+02:00",
			utc(2024, 5, 6, 10, 30, 45, 0)},
		{"extended year stays a string", "+002024-05-06", "+002024-05-06"},
		{"impossible month stays a string", "2024-13-01", "2024-13-01"},
		{"loose date stays a string", "2024/05/06", "2024/05/06"},

		// Everything else.
		{"word", "hello", "hello"},
		{"a string keeps its surrounding whitespace", "  hello  ", "  hello  "},
		{"NaN is the float, not the string", "NaN", math.NaN()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AutoTypeValue(tc.in)
			// NaN is never equal to itself, so it needs its own comparison.
			if f, ok := tc.want.(float64); ok && math.IsNaN(f) {
				g, ok := got.(float64)
				if !ok || !math.IsNaN(g) {
					t.Fatalf("AutoTypeValue(%q) = %#v, want NaN", tc.in, got)
				}
				return
			}
			if wt, ok := tc.want.(time.Time); ok {
				gt, ok := got.(time.Time)
				if !ok || !gt.Equal(wt) {
					t.Fatalf("AutoTypeValue(%q) = %#v, want %v", tc.in, got, wt)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AutoTypeValue(%q) = %#v (%T), want %#v (%T)",
					tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}

// TestAutoTypeLocalDateTime covers the one inference whose result depends on the
// process's time zone: a date-time with no offset is local, a bare date is UTC.
func TestAutoTypeLocalDateTime(t *testing.T) {
	got, ok := AutoTypeValue("2024-05-06T12:30:45").(time.Time)
	if !ok {
		t.Fatalf("AutoTypeValue did not infer a date")
	}
	want := time.Date(2024, 5, 6, 12, 30, 45, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAutoType(t *testing.T) {
	table, err := ParseCSV("name,age,active,joined,note\nada,36,true,1852-11-27,\n")
	if err != nil {
		t.Fatal(err)
	}
	got := AutoTypeRows(table.Rows)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	want := map[string]any{
		"name":   "ada",
		"age":    36.0,
		"active": true,
		"joined": time.Date(1852, 11, 27, 0, 0, 0, 0, time.UTC),
		"note":   nil,
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("AutoType = %#v, want %#v", got[0], want)
	}

	// The source row must be untouched: d3 converts in place, this does not.
	if table.Rows[0]["age"] != "36" {
		t.Errorf("AutoType modified its input: %v", table.Rows[0])
	}
}
