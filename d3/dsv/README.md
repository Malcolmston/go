# dsv — Go port of d3-dsv — parsing and formatting delimiter-separated values (CSV, TSV

[![Go Reference](https://pkg.go.dev/badge/github.com/malcolmston/d3/dsv.svg)](https://pkg.go.dev/github.com/malcolmston/d3/dsv)

Package dsv is a Go port of d3-dsv — parsing and formatting
delimiter-separated values (CSV, TSV, and anything else with a single-rune
delimiter).

- Object parsing. d3's central idea is that a CSV is a table with named
columns, so `Parse` treats the first row as a header and returns []Row — one
map per record, keyed by column name — together with the column names in
file order. encoding/csv returns [][]string and leaves the header to you. -
Type inference. `AutoType` turns the strings in a row into numbers, booleans,
dates and nulls using d3's exact rules; see its documentation for what it will
and will not convert. - TSV, and arbitrary delimiters, as first-class entry
points rather than a field to set on a reader. - d3's formatting and quoting
rules, which differ from `encoding/csv.Writer`'s (see `FormatValue`).

## Install

`d3` lives inside the [aggregator repo](../..) as a plain
directory rather than its own GitHub repository, so it is **not fetchable with
`go get`** yet. Use it through the committed `go.work`, or add a `replace`:

```
require github.com/malcolmston/d3 v0.0.0
replace github.com/malcolmston/d3 => ../path/to/go/d3
```

```go
import "github.com/malcolmston/d3/dsv"
```

Local version: `0.1.0` (see [`VERSION`](../VERSION)).

## Exported surface

### Functions

| Function | What it does |
| --- | --- |
| `func AutoType(row Row) map[string]any` | AutoType applies `AutoTypeValue` to every field of a row, returning a new map. |
| `func AutoTypeRows(rows []Row) []map[string]any` | AutoTypeRows applies `AutoType` to every row of a table. |
| `func AutoTypeValue(s string) any` | AutoTypeValue infers a Go value from a single field, following d3's autoType rules exactly. |
| `func Format(delim rune, rows []Row, columns []string) string` | Format renders rows as a delimited file with a header row, joined by "\n" and with no trailing newline. |
| `func FormatBody(delim rune, rows []Row, columns []string) string` | FormatBody is `Format` without the header row — useful for appending to an existing file. |
| `func FormatCSV(rows []Row, columns []string) string` | FormatCSV renders rows as a comma-separated file with a header. |
| `func FormatCSVRows(rows [][]string) string` | FormatCSVRows renders records as comma-separated lines. |
| `func FormatRow(delim rune, row []string) string` | FormatRow renders one record as a delimited line, with no trailing newline. |
| `func FormatRows(delim rune, rows [][]string) string` | FormatRows renders records as delimited lines joined by "\n", with no header and no trailing newline. |
| `func FormatTSV(rows []Row, columns []string) string` | FormatTSV renders rows as a tab-separated file with a header. |
| `func FormatTSVRows(rows [][]string) string` | FormatTSVRows renders records as tab-separated lines. |
| `func FormatValue(delim rune, value string) string` | FormatValue renders a single field, quoting it only if it has to be. |
| `func InferColumns(rows []Row) []string` | InferColumns returns the union of the keys of every row. |
| `func ParseCSVRows(text string) ([][]string, error)` | ParseCSVRows parses comma-separated rows. |
| `func ParseRows(delim rune, text string) ([][]string, error)` | ParseRows parses text as delimiter-separated values and returns the raw records, with no header handling. |
| `func ParseTSVRows(text string) ([][]string, error)` | ParseTSVRows parses tab-separated rows. |

### Types

| Type | What it is |
| --- | --- |
| `Row` | Row is one parsed record, keyed by column name. |
| `Table` | Table is the result of parsing a DSV file with a header row: the column names in the order they appear in the file, and one `Row` per record. |

<details>
<summary><code>Table</code> — constructors and methods</summary>

| Signature | What it does |
| --- | --- |
| `func Parse(delim rune, text string) (*Table, error)` | Parse parses text as delimiter-separated values with a header row, returning one `Row` per record keyed by the header's column names. |
| `func ParseCSV(text string) (*Table, error)` | ParseCSV parses a comma-separated file with a header row. |
| `func ParseTSV(text string) (*Table, error)` | ParseTSV parses a tab-separated file with a header row. |
| `func (t *Table) Format(delim rune) string` | Format renders the table with the given delimiter, header row included. |

</details>

### Constants

`Comma`, `Tab`

Full signatures, doc comments and every runnable example are on
[pkg.go.dev](https://pkg.go.dev/github.com/malcolmston/d3/dsv).

## Deviations from upstream

Deliberate differences for this package, where any exist, are recorded in the
module-wide [`API-DEVIATIONS.md`](../API-DEVIATIONS.md).

## License

MIT, as part of [`github.com/malcolmston/d3`](..).
An independent re-implementation, not affiliated with or endorsed by the
original project.
