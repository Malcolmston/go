// Package dsv is a Go port of d3-dsv — parsing and formatting
// delimiter-separated values (CSV, TSV, and anything else with a single-rune
// delimiter).
//
// Go already has encoding/csv in the standard library, and it is a correct,
// well-tested RFC 4180 reader. Reimplementing one here would be pointless, so
// this package does not: [ParseRows] and [Parse] delegate the character-level
// work to encoding/csv. What this package adds is the part of d3-dsv that
// encoding/csv has no equivalent for:
//
//   - Object parsing. d3's central idea is that a CSV is a table with named
//     columns, so [Parse] treats the first row as a header and returns
//     []Row — one map per record, keyed by column name — together with the
//     column names in file order. encoding/csv returns [][]string and leaves
//     the header to you.
//   - Type inference. [AutoType] turns the strings in a row into numbers,
//     booleans, dates and nulls using d3's exact rules; see its documentation
//     for what it will and will not convert.
//   - TSV, and arbitrary delimiters, as first-class entry points rather than a
//     field to set on a reader.
//   - d3's formatting and quoting rules, which differ from
//     [encoding/csv.Writer]'s (see [FormatValue]).
//
// # Where this deliberately differs from d3
//
// Two behaviours follow encoding/csv rather than d3, because the Go behaviour is
// the better one and a caller who wanted the JavaScript quirk is unlikely to be
// well served by it:
//
//   - Malformed input is an error. d3's parser is unfailingly lenient: a bare
//     quote in the middle of an unquoted field, or a quoted field with trailing
//     junk, is silently absorbed. Here it produces an error, so a truncated
//     download or a mis-detected delimiter is reported rather than turned into
//     plausible-looking garbage.
//   - Blank lines are skipped. d3 yields a one-element row containing the empty
//     string for each blank line, which then becomes a row of empty fields.
//     encoding/csv skips them, and so does this package.
//
// One behaviour deliberately differs from encoding/csv: a leading UTF-8 byte
// order mark is stripped before parsing. Neither d3 nor encoding/csv does this,
// and both consequently produce a first column name of "\uFEFFid" instead of
// "id" for any file exported by Excel. That silent corruption of one column name
// is a much worse default than dropping a mark that carries no information in a
// Go string.
//
// Output is formatted d3's way rather than encoding/csv's: rows are joined with
// "\n" (never "\r\n"), there is no trailing newline, and a value is quoted only
// when it actually contains the delimiter, a double quote, or a line break. See
// [FormatValue].
package dsv

import (
	"encoding/csv"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Common delimiters, for use with the generic [Parse], [ParseRows] and [Format].
const (
	Comma = ','
	Tab   = '\t'
)

// bom is the UTF-8 byte order mark, stripped from the head of any input. See the
// package doc for why.
const bom = "\uFEFF"

// Row is one parsed record, keyed by column name.
//
// This is d3's row object. Fields absent from a short record are present as the
// empty string rather than missing from the map, so a lookup never needs a
// comma-ok check to distinguish "empty" from "not there" — which matches d3,
// where every row object carries every column.
type Row map[string]string

// Table is the result of parsing a DSV file with a header row: the column names
// in the order they appear in the file, and one [Row] per record.
//
// Columns exists because a Go map has no order and a table does. Every
// formatting entry point takes the column list separately for the same reason:
// round-tripping a file without reordering its columns is only possible if that
// order is carried alongside the data.
type Table struct {
	Columns []string
	Rows    []Row
}

// Format renders the table with the given delimiter, header row included. It is
// shorthand for Format(delim, t.Rows, t.Columns).
func (t *Table) Format(delim rune) string { return Format(delim, t.Rows, t.Columns) }

// validDelim reports whether r can be used as a delimiter. The three exclusions
// are structural: a quote or a line break as a delimiter would make the grammar
// ambiguous.
func validDelim(r rune) bool {
	return r != '"' && r != '\r' && r != '\n' && r != 0 && r != 0xFFFD
}

// newReader builds a csv.Reader configured to behave like d3's parser as far as
// it can: ragged records are allowed (FieldsPerRecord = -1, since d3 pads or
// truncates rather than rejecting), leading whitespace is significant, and
// nothing is treated as a comment.
func newReader(delim rune, text string) (*csv.Reader, error) {
	if !validDelim(delim) {
		return nil, fmt.Errorf("dsv: invalid delimiter %q", delim)
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, bom)))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = false
	r.TrimLeadingSpace = false
	r.ReuseRecord = false
	return r, nil
}

// ParseRows parses text as delimiter-separated values and returns the raw
// records, with no header handling. Use it when the file has no header row, or
// when the header needs custom treatment.
//
// Quoted fields may contain the delimiter, line breaks, and doubled quotes;
// records may be terminated with LF or CRLF; a single trailing newline is not
// treated as an extra record. Blank lines are skipped and a leading BOM is
// stripped — see the package doc.
func ParseRows(delim rune, text string) ([][]string, error) {
	r, err := newReader(delim, text)
	if err != nil {
		return nil, err
	}
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("dsv: %w", err)
	}
	if rows == nil {
		rows = [][]string{}
	}
	return rows, nil
}

// ParseCSVRows parses comma-separated rows. See [ParseRows].
func ParseCSVRows(text string) ([][]string, error) { return ParseRows(Comma, text) }

// ParseTSVRows parses tab-separated rows. See [ParseRows].
func ParseTSVRows(text string) ([][]string, error) { return ParseRows(Tab, text) }

// Parse parses text as delimiter-separated values with a header row, returning
// one [Row] per record keyed by the header's column names.
//
// Records shorter than the header get the empty string for the missing columns;
// records longer than the header have the surplus fields dropped, since there is
// no name to file them under. Both match d3. Duplicate column names collapse
// onto one key, again as in d3 — the last field with that name wins.
//
// Empty input yields a table with no columns and no rows rather than an error.
func Parse(delim rune, text string) (*Table, error) {
	rows, err := ParseRows(delim, text)
	if err != nil {
		return nil, err
	}
	t := &Table{Columns: []string{}, Rows: []Row{}}
	if len(rows) == 0 {
		return t, nil
	}
	t.Columns = rows[0]
	for _, rec := range rows[1:] {
		row := make(Row, len(t.Columns))
		for i, name := range t.Columns {
			if i < len(rec) {
				row[name] = rec[i]
			} else {
				row[name] = ""
			}
		}
		t.Rows = append(t.Rows, row)
	}
	return t, nil
}

// ParseCSV parses a comma-separated file with a header row. See [Parse].
func ParseCSV(text string) (*Table, error) { return Parse(Comma, text) }

// ParseTSV parses a tab-separated file with a header row. See [Parse].
func ParseTSV(text string) (*Table, error) { return Parse(Tab, text) }

// FormatValue renders a single field, quoting it only if it has to be.
//
// d3's rule — quote when the value contains the delimiter, a double quote, CR or
// LF, and never otherwise — is narrower than [encoding/csv.Writer]'s, which also
// quotes fields with leading whitespace and the literal `\.`. Both are valid
// RFC 4180; this package follows d3 so that its output is byte-identical to the
// JavaScript original, which matters when the two are compared in a test or a
// diff. Embedded quotes are escaped by doubling.
func FormatValue(delim rune, value string) string {
	if !strings.ContainsRune(value, delim) &&
		!strings.ContainsAny(value, "\"\n\r") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// FormatRow renders one record as a delimited line, with no trailing newline.
func FormatRow(delim rune, row []string) string {
	var b strings.Builder
	for i, v := range row {
		if i > 0 {
			b.WriteRune(delim)
		}
		b.WriteString(FormatValue(delim, v))
	}
	return b.String()
}

// FormatRows renders records as delimited lines joined by "\n", with no header
// and no trailing newline.
func FormatRows(delim rune, rows [][]string) string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = FormatRow(delim, row)
	}
	return strings.Join(lines, "\n")
}

// FormatCSVRows renders records as comma-separated lines. See [FormatRows].
func FormatCSVRows(rows [][]string) string { return FormatRows(Comma, rows) }

// FormatTSVRows renders records as tab-separated lines. See [FormatRows].
func FormatTSVRows(rows [][]string) string { return FormatRows(Tab, rows) }

// InferColumns returns the union of the keys of every row.
//
// d3 returns them in order of discovery, which it can do because JavaScript
// objects remember insertion order. A Go map does not, so the names are sorted
// lexicographically instead — arbitrary, but deterministic, which is the
// property that actually matters for a file you intend to diff. Pass an explicit
// column list to [Format] whenever the order is meaningful.
func InferColumns(rows []Row) []string {
	seen := map[string]bool{}
	var cols []string
	for _, row := range rows {
		for name := range row {
			if !seen[name] {
				seen[name] = true
				cols = append(cols, name)
			}
		}
	}
	sort.Strings(cols)
	return cols
}

// Format renders rows as a delimited file with a header row, joined by "\n" and
// with no trailing newline.
//
// If columns is nil the column names are inferred with [InferColumns]. Values
// missing from a row are written as empty fields.
func Format(delim rune, rows []Row, columns []string) string {
	if columns == nil {
		columns = InferColumns(rows)
	}
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, FormatRow(delim, columns))
	lines = append(lines, formatBody(delim, rows, columns)...)
	return strings.Join(lines, "\n")
}

// FormatBody is [Format] without the header row — useful for appending to an
// existing file.
func FormatBody(delim rune, rows []Row, columns []string) string {
	if columns == nil {
		columns = InferColumns(rows)
	}
	return strings.Join(formatBody(delim, rows, columns), "\n")
}

func formatBody(delim rune, rows []Row, columns []string) []string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		rec := make([]string, len(columns))
		for j, name := range columns {
			rec[j] = row[name]
		}
		lines[i] = FormatRow(delim, rec)
	}
	return lines
}

// FormatCSV renders rows as a comma-separated file with a header. See [Format].
func FormatCSV(rows []Row, columns []string) string { return Format(Comma, rows, columns) }

// FormatTSV renders rows as a tab-separated file with a header. See [Format].
func FormatTSV(rows []Row, columns []string) string { return Format(Tab, rows, columns) }

// jsNumber matches the decimal numeric literals that JavaScript's unary + would
// accept, and nothing else.
//
// Go's [strconv.ParseFloat] is more permissive than JavaScript in ways that
// matter here: it accepts "inf", "nan", "1_000" and hexadecimal floats such as
// "0x1p-2", none of which JavaScript treats as numbers. Screening with this
// pattern first keeps [AutoType] from turning a column of the strings "nan" or
// "inf" — perfectly ordinary words — into floating-point specials.
var jsNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// isoDate is d3's date pattern, matched exactly: an optional extended-year sign
// and century, a four-digit year, optional month and day, and an optional time
// with optional seconds, milliseconds and zone.
var isoDate = regexp.MustCompile(`^([-+]\d{2})?\d{4}(-\d{2}(-\d{2})?)?(T\d{2}:\d{2}(:\d{2}(\.\d{3})?)?(Z|[-+]\d{2}:\d{2})?)?$`)

// dateLayouts are tried in order against a string that already matched
// [isoDate]. The zoned layouts come first so that an explicit offset wins over
// the zoneless interpretation.
var dateLayouts = []struct {
	layout string
	// zoned reports whether the layout consumes an explicit zone. Layouts that
	// do not are interpreted in local time when they carry a time of day, and
	// in UTC when they are a bare date — which is what JavaScript's Date
	// constructor does with the same strings.
	zoned bool
	// dateOnly reports whether the layout has no time component.
	dateOnly bool
}{
	{"2006-01-02T15:04:05.000Z07:00", true, false},
	{"2006-01-02T15:04:05Z07:00", true, false},
	{"2006-01-02T15:04Z07:00", true, false},
	{"2006-01-02T15:04:05.000", false, false},
	{"2006-01-02T15:04:05", false, false},
	{"2006-01-02T15:04", false, false},
	{"2006-01-02", false, true},
	{"2006-01", false, true},
}

// AutoTypeValue infers a Go value from a single field, following d3's autoType
// rules exactly. It returns one of:
//
//   - nil, for a field that is empty or all whitespace;
//   - bool, for exactly "true" or "false";
//   - float64, for anything JavaScript's unary + would convert, plus NaN for
//     exactly "NaN";
//   - [time.Time], for an ISO 8601 date or date-time;
//   - string, the original unmodified field, for everything else.
//
// The order is significant and has one famous consequence: the number test comes
// before the date test, so a bare year like "2024" infers as the number 2024,
// not as a date. Only strings with a month ("2024-05") or more reach the date
// branch. That is d3's behaviour and this package keeps it, because a column of
// years really is more useful as numbers.
//
// The comparisons for "true", "false" and "NaN" are case-sensitive, again as in
// d3: "TRUE" stays a string. Leading and trailing whitespace is ignored when
// deciding, but a value that ends up staying a string is returned untrimmed.
//
// Two details are Go-specific. A date with no zone but a time of day is
// interpreted in [time.Local] and a bare date in UTC, matching how modern
// JavaScript engines read the same strings. Extended years ("+002024-01-01")
// match d3's pattern but are not parsed here and stay strings.
func AutoTypeValue(s string) any {
	v := strings.TrimSpace(s)
	switch {
	case v == "":
		return nil
	case v == "true":
		return true
	case v == "false":
		return false
	case v == "NaN":
		return math.NaN()
	}
	if n, ok := parseJSNumber(v); ok {
		return n
	}
	if isoDate.MatchString(v) {
		if t, ok := parseISODate(v); ok {
			return t
		}
	}
	return s
}

// parseJSNumber converts the strings JavaScript's unary + accepts, and only
// those: decimal literals, the three spellings of Infinity, and the 0x/0o/0b
// integer literals.
func parseJSNumber(v string) (float64, bool) {
	switch v {
	case "Infinity", "+Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	}
	if len(v) > 2 && v[0] == '0' {
		var base int
		switch v[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			n, err := strconv.ParseUint(v[2:], base, 64)
			if err != nil {
				return 0, false
			}
			return float64(n), true
		}
	}
	if !jsNumber.MatchString(v) {
		return 0, false
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseISODate(v string) (time.Time, bool) {
	for _, l := range dateLayouts {
		var t time.Time
		var err error
		switch {
		case l.zoned:
			t, err = time.Parse(l.layout, v)
		case l.dateOnly:
			t, err = time.ParseInLocation(l.layout, v, time.UTC)
		default:
			t, err = time.ParseInLocation(l.layout, v, time.Local)
		}
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// AutoType applies [AutoTypeValue] to every field of a row, returning a new map.
// The input row is not modified — unlike d3's autoType, which converts in place;
// Go's type system makes that impossible for a map[string]string anyway.
func AutoType(row Row) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = AutoTypeValue(v)
	}
	return out
}

// AutoTypeRows applies [AutoType] to every row of a table.
func AutoTypeRows(rows []Row) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = AutoType(row)
	}
	return out
}
