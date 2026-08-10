// Example program exercising github.com/malcolmston/sqlite, a pure-Go
// in-memory SQL engine that registers a database/sql driver named "mstsqlite".
//
// It runs two suites:
//   - the database/sql path (open, DDL, DML, queries, transactions, errors)
//   - the direct *sqlite.Database API and the exported AST / scalar helpers
//
// Every check prints PASS or FAIL with the observed value, and the process exits
// non-zero if anything failed.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	mst "github.com/malcolmston/sqlite"
)

var failures int

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		failures++
	}
	fmt.Printf("  [%s] %-46s %s\n", status, name, detail)
}

func checkEq[T comparable](name string, got, want T) {
	check(name, got == want, fmt.Sprintf("got=%v want=%v", got, want))
}

func fatal(where string, err error) {
	fmt.Printf("  [FAIL] %s: unexpected error: %v\n", where, err)
	failures++
}

func section(title string) {
	fmt.Printf("\n== %s ==\n", title)
}

func main() {
	fmt.Printf("library: github.com/malcolmston/sqlite\n")
	fmt.Printf("driver name constant: %q\n", mst.DriverName)

	databaseSQLSuite()
	directAPISuite()
	astAndFuncSuite()

	fmt.Printf("\n== SUMMARY ==\n")
	if failures == 0 {
		fmt.Println("all checks passed")
		return
	}
	fmt.Printf("%d check(s) FAILED\n", failures)
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// database/sql suite
// ---------------------------------------------------------------------------

func databaseSQLSuite() {
	section("1. open in-memory database via database/sql")

	db, err := sql.Open(mst.DriverName, ":memory:")
	if err != nil {
		fatal("sql.Open", err)
		return
	}
	defer db.Close()
	// The engine's anonymous ":memory:" DSN is private per connection, and
	// transactions take the whole-database lock, so the pool must be pinned to
	// one connection.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		fatal("Ping", err)
	} else {
		check("Ping", true, "ok")
	}

	// -- DDL -----------------------------------------------------------------
	section("2. CREATE TABLE / schema constraints")

	ddl := []string{
		`CREATE TABLE authors (
			id     INTEGER PRIMARY KEY,
			name   TEXT NOT NULL,
			country TEXT
		)`,
		`CREATE TABLE books (
			id        INTEGER PRIMARY KEY,
			author_id INTEGER NOT NULL,
			title     TEXT NOT NULL,
			year      INTEGER,
			price     REAL,
			cover     BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS books (id INTEGER)`, // no-op, must not error
	}
	for i, stmt := range ddl {
		if _, err := db.Exec(stmt); err != nil {
			fatal(fmt.Sprintf("DDL #%d", i+1), err)
		}
	}
	check("CREATE TABLE x2 + IF NOT EXISTS", true, "3 statements executed")

	// -- INSERT --------------------------------------------------------------
	section("3. INSERT (multi-row literal + parameterized)")

	res, err := db.Exec(`INSERT INTO authors (id, name, country) VALUES
		(1, 'Ursula K. Le Guin', 'US'),
		(2, 'Stanislaw Lem',     'PL'),
		(3, 'Italo Calvino',     'IT'),
		(4, 'Nobody',            NULL)`)
	if err != nil {
		fatal("multi-row INSERT", err)
	} else {
		n, _ := res.RowsAffected()
		checkEq("authors RowsAffected", n, int64(4))
		id, _ := res.LastInsertId()
		check("authors LastInsertId > 0", id > 0, fmt.Sprintf("got=%d", id))
	}

	type book struct {
		id, authorID, year int
		title              string
		price              float64
	}
	books := []book{
		{1, 1, 1969, "The Left Hand of Darkness", 12.50},
		{2, 1, 1974, "The Dispossessed", 11.00},
		{3, 1, 1971, "The Lathe of Heaven", 9.75},
		{4, 2, 1961, "Solaris", 10.25},
		{5, 2, 1967, "The Cyberiad", 14.00},
		{6, 3, 1972, "Invisible Cities", 13.50},
	}
	for _, b := range books {
		if _, err := db.Exec(
			`INSERT INTO books (id, author_id, title, year, price) VALUES (?, ?, ?, ?, ?)`,
			b.id, b.authorID, b.title, b.year, b.price,
		); err != nil {
			fatal("parameterized INSERT", err)
		}
	}
	var nBooks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM books`).Scan(&nBooks); err != nil {
		fatal("COUNT(*)", err)
	} else {
		checkEq("parameterized INSERT row count", nBooks, 6)
	}

	// BLOB round-trip.
	if _, err := db.Exec(
		`INSERT INTO books (id, author_id, title, year, price, cover) VALUES (?, ?, ?, ?, ?, ?)`,
		7, 3, "If on a winter's night", 1979, 15.00, []byte{0xDE, 0xAD, 0xBE, 0xEF},
	); err != nil {
		fatal("BLOB INSERT", err)
	} else {
		var cover []byte
		if err := db.QueryRow(`SELECT cover FROM books WHERE id = ?`, 7).Scan(&cover); err != nil {
			fatal("BLOB SELECT", err)
		} else {
			checkEq("BLOB round-trip", fmt.Sprintf("%X", cover), "DEADBEEF")
		}
	}

	// -- SELECT: WHERE / ORDER BY / LIMIT ------------------------------------
	section("4. SELECT with WHERE, ORDER BY, LIMIT/OFFSET")

	titles, err := queryStrings(db,
		`SELECT title FROM books WHERE year >= ? AND price < ? ORDER BY year DESC`,
		1969, 14.00)
	if err != nil {
		fatal("WHERE+ORDER BY", err)
	} else {
		checkEq("WHERE/ORDER BY DESC",
			strings.Join(titles, "|"),
			"The Dispossessed|Invisible Cities|The Lathe of Heaven|The Left Hand of Darkness")
	}

	titles, err = queryStrings(db, `SELECT title FROM books ORDER BY title ASC LIMIT 2 OFFSET 1`)
	if err != nil {
		fatal("LIMIT/OFFSET", err)
	} else {
		checkEq("ORDER BY ASC + LIMIT 2 OFFSET 1",
			strings.Join(titles, "|"), "Invisible Cities|Solaris")
	}

	// LIKE / IN / IS NULL / NOT.
	titles, err = queryStrings(db, `SELECT title FROM books WHERE title LIKE 'The %' ORDER BY title`)
	if err != nil {
		fatal("LIKE", err)
	} else {
		checkEq("LIKE 'The %' count", len(titles), 4)
	}
	names, err := queryStrings(db, `SELECT name FROM authors WHERE country IN ('PL','IT') ORDER BY name`)
	if err != nil {
		fatal("IN", err)
	} else {
		checkEq("IN (...)", strings.Join(names, "|"), "Italo Calvino|Stanislaw Lem")
	}
	names, err = queryStrings(db, `SELECT name FROM authors WHERE country IS NULL`)
	if err != nil {
		fatal("IS NULL", err)
	} else {
		checkEq("IS NULL", strings.Join(names, "|"), "Nobody")
	}

	// DISTINCT + expression + concatenation.
	vals, err := queryStrings(db, `SELECT DISTINCT country FROM authors WHERE country IS NOT NULL ORDER BY country`)
	if err != nil {
		fatal("DISTINCT", err)
	} else {
		checkEq("DISTINCT country", strings.Join(vals, "|"), "IT|PL|US")
	}
	var label string
	if err := db.QueryRow(`SELECT name || ' (' || country || ')' FROM authors WHERE id = 2`).Scan(&label); err != nil {
		fatal("|| concat", err)
	} else {
		checkEq("string concatenation ||", label, "Stanislaw Lem (PL)")
	}

	// -- Aggregates / GROUP BY / HAVING --------------------------------------
	section("5. aggregates, GROUP BY, HAVING")

	var cnt int
	var sum, avg, min, max float64
	err = db.QueryRow(`SELECT COUNT(*), SUM(price), AVG(price), MIN(price), MAX(price) FROM books`).
		Scan(&cnt, &sum, &avg, &min, &max)
	if err != nil {
		fatal("aggregate row", err)
	} else {
		checkEq("COUNT(*)", cnt, 7)
		checkEq("SUM(price)", fmt.Sprintf("%.2f", sum), "86.00")
		checkEq("AVG(price)", fmt.Sprintf("%.4f", avg), "12.2857")
		checkEq("MIN(price)", fmt.Sprintf("%.2f", min), "9.75")
		checkEq("MAX(price)", fmt.Sprintf("%.2f", max), "15.00")
	}

	var distinctAuthors int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT author_id) FROM books`).Scan(&distinctAuthors); err != nil {
		fatal("COUNT(DISTINCT)", err)
	} else {
		checkEq("COUNT(DISTINCT author_id)", distinctAuthors, 3)
	}

	grouped, err := queryStrings(db,
		`SELECT author_id, COUNT(*) FROM books GROUP BY author_id HAVING COUNT(*) > 1 ORDER BY author_id`)
	if err != nil {
		fatal("GROUP BY/HAVING", err)
	} else {
		checkEq("GROUP BY + HAVING", strings.Join(grouped, "|"), "1,3|2,2|3,2")
	}

	// -- JOIN ----------------------------------------------------------------
	section("6. INNER JOIN")

	joined, err := queryStrings(db,
		`SELECT a.name, b.title FROM books AS b
		 INNER JOIN authors AS a ON a.id = b.author_id
		 WHERE a.country = ? ORDER BY b.title`, "PL")
	if err != nil {
		fatal("INNER JOIN", err)
	} else {
		checkEq("JOIN with alias + WHERE",
			strings.Join(joined, "|"),
			"Stanislaw Lem,Solaris|Stanislaw Lem,The Cyberiad")
	}

	joinedAgg, err := queryStrings(db,
		`SELECT a.name, COUNT(*) FROM authors AS a
		 INNER JOIN books AS b ON b.author_id = a.id
		 GROUP BY a.name ORDER BY a.name`)
	if err != nil {
		fatal("JOIN + GROUP BY", err)
	} else {
		checkEq("JOIN + GROUP BY aggregate",
			strings.Join(joinedAgg, "|"),
			"Italo Calvino,2|Stanislaw Lem,2|Ursula K. Le Guin,3")
	}

	// Scalar functions inside a query.
	var up string
	if err := db.QueryRow(`SELECT UPPER(SUBSTR(title, 1, 7)) FROM books WHERE id = 4`).Scan(&up); err != nil {
		fatal("scalar funcs in SQL", err)
	} else {
		checkEq("UPPER(SUBSTR(...))", up, "SOLARIS")
	}

	// -- UPDATE / DELETE -----------------------------------------------------
	section("7. UPDATE and DELETE")

	upRes, err := db.Exec(`UPDATE books SET price = price * 2, year = ? WHERE author_id = ?`, 2000, 2)
	if err != nil {
		fatal("UPDATE", err)
	} else {
		n, _ := upRes.RowsAffected()
		checkEq("UPDATE RowsAffected", n, int64(2))
		var p float64
		if err := db.QueryRow(`SELECT price FROM books WHERE id = 4`).Scan(&p); err != nil {
			fatal("UPDATE verify", err)
		} else {
			checkEq("UPDATE applied arithmetic", fmt.Sprintf("%.2f", p), "20.50")
		}
	}

	delRes, err := db.Exec(`DELETE FROM books WHERE year = ?`, 2000)
	if err != nil {
		fatal("DELETE", err)
	} else {
		n, _ := delRes.RowsAffected()
		checkEq("DELETE RowsAffected", n, int64(2))
		var left int
		_ = db.QueryRow(`SELECT COUNT(*) FROM books`).Scan(&left)
		checkEq("rows remaining after DELETE", left, 5)
	}

	// -- Prepared statements -------------------------------------------------
	section("8. prepared statements")

	stmt, err := db.Prepare(`SELECT title FROM books WHERE author_id = ? ORDER BY year`)
	if err != nil {
		fatal("Prepare", err)
	} else {
		defer stmt.Close()
		got := []string{}
		rows, err := stmt.Query(1)
		if err != nil {
			fatal("stmt.Query", err)
		} else {
			for rows.Next() {
				var t string
				if err := rows.Scan(&t); err != nil {
					fatal("stmt scan", err)
					break
				}
				got = append(got, t)
			}
			rows.Close()
			checkEq("prepared stmt reuse",
				strings.Join(got, "|"),
				"The Left Hand of Darkness|The Lathe of Heaven|The Dispossessed")
		}
	}

	// -- Transactions --------------------------------------------------------
	section("9. transactions (commit)")

	tx, err := db.Begin()
	if err != nil {
		fatal("Begin", err)
	} else {
		if _, err := tx.Exec(`INSERT INTO authors (id, name, country) VALUES (?, ?, ?)`,
			10, "Committed Author", "SE"); err != nil {
			fatal("tx Exec", err)
			_ = tx.Rollback()
		} else if err := tx.Commit(); err != nil {
			fatal("Commit", err)
		} else {
			var n int
			_ = db.QueryRow(`SELECT COUNT(*) FROM authors WHERE id = 10`).Scan(&n)
			checkEq("committed row visible", n, 1)
		}
	}

	section("10. transactions (rollback)")

	tx, err = db.Begin()
	if err != nil {
		fatal("Begin #2", err)
	} else {
		if _, err := tx.Exec(`INSERT INTO authors (id, name) VALUES (?, ?)`, 11, "Doomed Author"); err != nil {
			fatal("tx Exec #2", err)
		}
		if _, err := tx.Exec(`DELETE FROM books WHERE author_id = 1`); err != nil {
			fatal("tx DELETE", err)
		}
		// Changes are visible inside the transaction...
		var insideBooks int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM books WHERE author_id = 1`).Scan(&insideBooks); err != nil {
			fatal("tx read-own-writes", err)
		} else {
			checkEq("read-own-writes inside tx", insideBooks, 0)
		}
		if err := tx.Rollback(); err != nil {
			fatal("Rollback", err)
		}
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM authors WHERE id = 11`).Scan(&n)
		checkEq("rolled-back INSERT discarded", n, 0)
		var afterBooks int
		_ = db.QueryRow(`SELECT COUNT(*) FROM books WHERE author_id = 1`).Scan(&afterBooks)
		checkEq("rolled-back DELETE restored", afterBooks, 3)
	}

	// -- Indexes -------------------------------------------------------------
	section("11. indexes")

	if _, err := db.Exec(`CREATE INDEX idx_books_year ON books (year)`); err != nil {
		check("CREATE INDEX rejected (documented limitation)", true,
			"err="+firstLine(err.Error()))
	} else {
		check("CREATE INDEX rejected (documented limitation)", false,
			"unexpectedly succeeded")
	}
	// HOLE: the engine has no secondary indexes at all -- no CREATE INDEX /
	// DROP INDEX statement in the parser, no UNIQUE column constraint, and no
	// query planner. All lookups are full scans. The only index-like feature is
	// the single INTEGER PRIMARY KEY uniqueness check below.
	if _, err := db.Exec(`INSERT INTO authors (id, name) VALUES (1, 'Duplicate PK')`); err != nil {
		check("PRIMARY KEY uniqueness enforced", true, "err="+firstLine(err.Error()))
	} else {
		check("PRIMARY KEY uniqueness enforced", false, "duplicate PK accepted")
	}

	// -- Error handling ------------------------------------------------------
	section("12. error handling")

	errCases := []struct {
		name string
		sql  string
		args []any
	}{
		{"syntax error", `SELCT * FROM books`, nil},
		{"unterminated string", `SELECT 'abc FROM books`, nil},
		{"unknown table", `SELECT * FROM no_such_table`, nil},
		{"unknown column", `SELECT nope FROM books`, nil},
		{"unknown function", `SELECT bogus_fn(1)`, nil},
		{"NOT NULL violation", `INSERT INTO authors (id, name) VALUES (99, NULL)`, nil},
		{"wrong arg count for function", `SELECT UPPER()`, nil},
		{"unsupported: ALTER TABLE", `ALTER TABLE books ADD COLUMN isbn TEXT`, nil},
		{"unsupported: subquery", `SELECT * FROM books WHERE author_id IN (SELECT id FROM authors)`, nil},
		{"unsupported: CASE WHEN", `SELECT CASE WHEN year > 1970 THEN 'new' ELSE 'old' END FROM books`, nil},
		{"unsupported: BETWEEN", `SELECT * FROM books WHERE year BETWEEN 1960 AND 1970`, nil},
		{"unsupported: CAST", `SELECT CAST(year AS TEXT) FROM books`, nil},
		{"unsupported: UNION", `SELECT id FROM books UNION SELECT id FROM authors`, nil},
		{"unsupported: comma cross join", `SELECT * FROM books, authors`, nil},
		{"unsupported: INSERT..SELECT", `INSERT INTO authors (id, name) SELECT id, title FROM books`, nil},
		{"unsupported: UNIQUE constraint", `CREATE TABLE u1 (a INTEGER UNIQUE)`, nil},
		{"unsupported: DEFAULT clause", `CREATE TABLE u2 (a INTEGER DEFAULT 5)`, nil},
		{"unsupported: AUTOINCREMENT", `CREATE TABLE u3 (a INTEGER PRIMARY KEY AUTOINCREMENT)`, nil},
		{"unsupported: qualified table name", `SELECT * FROM main.books`, nil},
		{"unsupported: PRAGMA", `PRAGMA table_info(books)`, nil},
	}
	for _, tc := range errCases {
		_, err := db.Exec(tc.sql, tc.args...)
		check(tc.name+" returns error", err != nil, describeErr(err))
	}

	// Too few placeholder args must not silently succeed.
	_, err = db.Query(`SELECT * FROM books WHERE id = ? AND year = ?`, 1)
	check("arg-count mismatch returns error", err != nil, describeErr(err))

	// HOLE (real bug): "LEFT JOIN" on an un-aliased table is not rejected. The
	// parser consumes LEFT as the *alias* of the preceding table and then parses
	// "JOIN authors ON ..." as a plain INNER JOIN, so the query silently returns
	// INNER JOIN results instead of erroring. Same shape for RIGHT/FULL/CROSS.
	leftRows, err := queryStrings(db,
		`SELECT books.title FROM books
		 LEFT JOIN authors ON authors.id = books.author_id
		 WHERE books.author_id = 999`)
	if err != nil {
		// If a future version rejects it outright, that is the correct behaviour.
		check("un-aliased LEFT JOIN silently becomes INNER JOIN (bug)", false,
			"now errors instead: "+describeErr(err))
	} else {
		check("un-aliased LEFT JOIN silently becomes INNER JOIN (bug)", len(leftRows) == 0,
			fmt.Sprintf("no error, %d rows (SQLite would keep the unmatched left rows)", len(leftRows)))
	}
	// With an explicit alias the same query fails, but with a confusing message
	// that never mentions outer joins.
	_, err = db.Query(`SELECT b.title FROM books AS b LEFT JOIN authors AS a ON a.id = b.author_id`)
	check("aliased LEFT JOIN errors with a confusing message", err != nil, describeErr(err))

	// HOLE (real bug): an omitted INTEGER PRIMARY KEY is not auto-assigned a
	// rowid the way SQLite does, and the resulting error misreports a NOT NULL
	// constraint on a column that was never declared NOT NULL.
	if _, err := db.Exec(`INSERT INTO authors (name) VALUES ('No Explicit Id')`); err != nil {
		check("omitted INTEGER PRIMARY KEY is not auto-assigned (bug)", true,
			"err="+firstLine(err.Error()))
	} else {
		check("omitted INTEGER PRIMARY KEY is not auto-assigned (bug)", false,
			"unexpectedly succeeded (autoincrement now works)")
	}

	// A failing statement must leave the connection usable.
	var stillWorks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM books`).Scan(&stillWorks); err != nil {
		fatal("connection usable after errors", err)
	} else {
		checkEq("connection usable after errors", stillWorks, 3+2)
	}

	// -- Named shared database + DROP TABLE ----------------------------------
	section("13. named (shared) database and DROP TABLE")

	sharedA, err := sql.Open(mst.DriverName, "shared_example_db")
	if err != nil {
		fatal("open shared A", err)
		return
	}
	defer sharedA.Close()
	sharedB, err := sql.Open(mst.DriverName, "shared_example_db")
	if err != nil {
		fatal("open shared B", err)
		return
	}
	defer sharedB.Close()

	if _, err := sharedA.Exec(`CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		fatal("shared CREATE", err)
	}
	if _, err := sharedA.Exec(`INSERT INTO kv (k, v) VALUES (?, ?)`, "hello", "world"); err != nil {
		fatal("shared INSERT", err)
	}
	var v string
	if err := sharedB.QueryRow(`SELECT v FROM kv WHERE k = ?`, "hello").Scan(&v); err != nil {
		fatal("shared SELECT from second handle", err)
	} else {
		checkEq("named DB shared across sql.DB handles", v, "world")
	}

	if _, err := sharedA.Exec(`DROP TABLE kv`); err != nil {
		fatal("DROP TABLE", err)
	} else {
		_, err := sharedB.Query(`SELECT * FROM kv`)
		check("DROP TABLE removed the table", err != nil, describeErr(err))
	}
	if _, err := sharedA.Exec(`DROP TABLE IF EXISTS kv`); err != nil {
		fatal("DROP TABLE IF EXISTS", err)
	} else {
		check("DROP TABLE IF EXISTS is a no-op", true, "ok")
	}
}

// ---------------------------------------------------------------------------
// direct API suite
// ---------------------------------------------------------------------------

func directAPISuite() {
	section("14. direct *sqlite.Database API")

	store := mst.NewDatabase()

	if _, err := store.Exec(`CREATE TABLE metrics (id INTEGER PRIMARY KEY, host TEXT NOT NULL, load REAL)`); err != nil {
		fatal("store CREATE", err)
		return
	}
	rows := [][]any{
		{1, "web-1", 0.75},
		{2, "web-2", 1.25},
		{3, "web-1", 0.50},
		{4, "db-1", 2.00},
	}
	for _, r := range rows {
		if _, err := store.Exec(`INSERT INTO metrics (id, host, load) VALUES (?, ?, ?)`, r...); err != nil {
			fatal("store INSERT", err)
		}
	}

	er, err := store.Exec(`UPDATE metrics SET load = load + 1 WHERE host = ?`, "web-1")
	if err != nil {
		fatal("store UPDATE", err)
	} else {
		checkEq("ExecResult.RowsAffected", er.RowsAffected, int64(2))
	}

	rs, err := store.Query(
		`SELECT host, COUNT(*), ROUND(AVG(load), 3) FROM metrics GROUP BY host ORDER BY host`)
	if err != nil {
		fatal("store Query", err)
	} else {
		checkEq("ResultSet.Columns length", len(rs.Columns), 3)
		checkEq("ResultSet.Rows length", len(rs.Rows), 3)
		var b strings.Builder
		for i, row := range rs.Rows {
			if i > 0 {
				b.WriteString("|")
			}
			for j, v := range row {
				if j > 0 {
					b.WriteString(",")
				}
				b.WriteString(v.String())
			}
		}
		// HOLE: Value.String() renders the REAL 2.0 as "2", making it
		// indistinguishable from the INTEGER 2. SQLite's own text rendering of a
		// REAL always keeps a decimal point ("2.0").
		checkEq("GROUP BY + ROUND(AVG) via direct API", b.String(),
			"db-1,1,2|web-1,2,1.625|web-2,1,1.25")
	}

	// Value accessors and typing.
	rs, err = store.Query(`SELECT id, host, load FROM metrics WHERE id = ?`, 4)
	if err != nil {
		fatal("store typed Query", err)
	} else if len(rs.Rows) != 1 {
		check("typed row fetched", false, fmt.Sprintf("got %d rows", len(rs.Rows)))
	} else {
		row := rs.Rows[0]
		checkEq("Value.Int64", row[0].Int64(), int64(4))
		checkEq("Value.Str", row[1].Str(), "db-1")
		checkEq("Value.Float64", row[2].Float64(), 2.0)
		checkEq("Value.Type on INTEGER", row[0].Type.String(), mst.Int(0).Type.String())
		checkEq("Value.GoValue", fmt.Sprintf("%v", row[1].GoValue()), "db-1")
		checkEq("Value.IsNull on non-null", row[0].IsNull(), false)
		checkEq("Null().IsNull", mst.Null().IsNull(), true)
	}

	// Three-valued logic: NULL comparisons are neither true nor false.
	rs, err = store.Query(`SELECT id FROM metrics WHERE load > NULL`)
	if err != nil {
		fatal("NULL comparison", err)
	} else {
		checkEq("NULL comparison yields no rows", len(rs.Rows), 0)
	}

	// Constructors.
	checkEq("mst.Text", mst.Text("hi").Str(), "hi")
	checkEq("mst.Real", mst.Real(1.5).Float64(), 1.5)
	checkEq("mst.Blob", fmt.Sprintf("%X", mst.Blob([]byte{1, 2}).Bytes()), "0102")

	// The direct API is not transaction-aware.
	// HOLE: *sqlite.Database exposes no Begin/Commit/Rollback methods, and
	// Exec("BEGIN") on it is accepted but is a no-op for rollback purposes --
	// transaction state lives on the unexported conn type, reachable only
	// through database/sql. See check below.
	if _, err := store.Exec(`BEGIN`); err != nil {
		check("direct API BEGIN is rejected", true, "err="+firstLine(err.Error()))
	} else {
		_, _ = store.Exec(`INSERT INTO metrics (id, host, load) VALUES (98, 'ghost', 0)`)
		_, rbErr := store.Exec(`ROLLBACK`)
		rs, _ := store.Query(`SELECT COUNT(*) FROM metrics WHERE id = 98`)
		ghostStillThere := len(rs.Rows) == 1 && rs.Rows[0][0].Int64() == 1
		check("direct API BEGIN/ROLLBACK does NOT roll back (known hole)",
			ghostStillThere,
			fmt.Sprintf("row survived rollback=%v rollbackErr=%v", ghostStillThere, rbErr))
	}
}

// ---------------------------------------------------------------------------
// AST + scalar function suite
// ---------------------------------------------------------------------------

func astAndFuncSuite() {
	section("15. exported AST via sqlite.Parse")

	stmt, err := mst.Parse(`SELECT DISTINCT a.x AS ex, COUNT(*) FROM t AS a
		INNER JOIN u AS b ON b.id = a.id
		WHERE a.x > ? AND a.y LIKE 'q%'
		GROUP BY a.x HAVING COUNT(*) > 1
		ORDER BY ex DESC LIMIT 5 OFFSET 2`)
	if err != nil {
		fatal("Parse SELECT", err)
	} else {
		sel, ok := stmt.(*mst.SelectStmt)
		check("Parse returns *SelectStmt", ok, fmt.Sprintf("%T", stmt))
		if ok {
			checkEq("SelectStmt.Distinct", sel.Distinct, true)
			checkEq("SelectStmt.From", sel.From, "t")
			checkEq("SelectStmt.FromAlias", sel.FromAlias, "a")
			check("SelectStmt.Join populated", sel.Join != nil, fmt.Sprintf("%v", sel.Join != nil))
			if sel.Join != nil {
				checkEq("JoinClause.Table", sel.Join.Table, "u")
				checkEq("JoinClause.Alias", sel.Join.Alias, "b")
			}
			checkEq("len(Columns)", len(sel.Columns), 2)
			checkEq("Columns[0].Alias", sel.Columns[0].Alias, "ex")
			check("Where set", sel.Where != nil, "ok")
			checkEq("len(GroupBy)", len(sel.GroupBy), 1)
			check("Having set", sel.Having != nil, "ok")
			checkEq("len(OrderBy)", len(sel.OrderBy), 1)
			checkEq("OrderBy[0].Desc", sel.OrderBy[0].Desc, true)
			check("Limit set", sel.Limit != nil, "ok")
			check("Offset set", sel.Offset != nil, "ok")
		}
	}

	ct, err := mst.Parse(`CREATE TABLE z (id INTEGER PRIMARY KEY, nm TEXT NOT NULL)`)
	if err != nil {
		fatal("Parse CREATE TABLE", err)
	} else if c, ok := ct.(*mst.CreateTableStmt); !ok {
		check("Parse returns *CreateTableStmt", false, fmt.Sprintf("%T", ct))
	} else {
		checkEq("CreateTableStmt column count", len(c.Columns), 2)
		checkEq("ColumnDef.PrimaryKey", c.Columns[0].PrimaryKey, true)
		checkEq("ColumnDef.NotNull", c.Columns[1].NotNull, true)
	}

	for _, tc := range []struct{ sql, want string }{
		{`INSERT INTO t (a) VALUES (1)`, "*sqlite.InsertStmt"},
		{`UPDATE t SET a = 1 WHERE b = 2`, "*sqlite.UpdateStmt"},
		{`DELETE FROM t WHERE a = 1`, "*sqlite.DeleteStmt"},
		{`DROP TABLE t`, "*sqlite.DropTableStmt"},
		{`BEGIN`, "*sqlite.BeginStmt"},
		{`COMMIT`, "*sqlite.CommitStmt"},
		{`ROLLBACK`, "*sqlite.RollbackStmt"},
	} {
		s, err := mst.Parse(tc.sql)
		if err != nil {
			fatal("Parse "+tc.sql, err)
			continue
		}
		checkEq("Parse "+strings.Fields(tc.sql)[0]+" node type", fmt.Sprintf("%T", s), tc.want)
	}

	if _, err := mst.Parse(`SELECT FROM`); err != nil {
		check("Parse rejects invalid SQL", true, "err="+firstLine(err.Error()))
	} else {
		check("Parse rejects invalid SQL", false, "no error")
	}

	section("16. exported scalar function registry")

	names := mst.ScalarNames()
	check("ScalarNames non-empty", len(names) > 0, fmt.Sprintf("%d functions: %s", len(names), strings.Join(names, ",")))
	checkEq("IsScalarFunc(\"upper\")", mst.IsScalarFunc("upper"), true)
	checkEq("IsScalarFunc(\"nope\")", mst.IsScalarFunc("nope"), false)
	if _, ok := mst.LookupScalar("LENGTH"); !ok {
		check("LookupScalar(\"LENGTH\")", false, "not found")
	} else {
		check("LookupScalar(\"LENGTH\")", true, "found")
	}

	if v, err := mst.CallScalar("replace", []mst.Value{mst.Text("a-b-c"), mst.Text("-"), mst.Text("+")}); err != nil {
		fatal("CallScalar(replace)", err)
	} else {
		checkEq("CallScalar(replace)", v.Str(), "a+b+c")
	}
	if _, err := mst.CallScalar("does_not_exist", nil); err == nil {
		check("CallScalar unknown name errors", false, "no error")
	} else {
		var fe *mst.FuncError
		check("CallScalar unknown name -> *FuncError", errors.As(err, &fe), describeErr(err))
	}

	// Direct Go-level helpers.
	checkEq("Abs", mst.Abs(mst.Int(-7)).Int64(), int64(7))
	checkEq("Length", mst.Length(mst.Text("héllo")).Int64(), int64(5))
	checkEq("Upper/Lower", mst.Upper(mst.Text("aB")).Str()+mst.Lower(mst.Text("Cd")).Str(), "ABcd")
	checkEq("Trim", mst.Trim(mst.Text("  x  ")).Str(), "x")
	checkEq("LTrim", mst.LTrim(mst.Text("xxay"), mst.Text("x")).Str(), "ay")
	checkEq("RTrim", mst.RTrim(mst.Text("axyy"), mst.Text("y")).Str(), "ax")
	checkEq("Substr", mst.Substr(mst.Text("abcdef"), mst.Int(2), mst.Int(3)).Str(), "bcd")
	checkEq("Replace", mst.Replace(mst.Text("aaa"), mst.Text("a"), mst.Text("b")).Str(), "bbb")
	checkEq("Round", mst.Round(mst.Real(2.34567), mst.Int(2)).Float64(), 2.35)
	checkEq("Coalesce", mst.Coalesce(mst.Null(), mst.Null(), mst.Text("z")).Str(), "z")
	checkEq("IfNull", mst.IfNull(mst.Null(), mst.Int(9)).Int64(), int64(9))
	checkEq("NullIf equal -> NULL", mst.NullIf(mst.Int(1), mst.Int(1)).IsNull(), true)
	checkEq("TypeOf", mst.TypeOf(mst.Text("x")).Str(), "text")
	checkEq("Hex", mst.Hex(mst.Blob([]byte{0xAB})).Str(), "AB")
	checkEq("Quote", mst.Quote(mst.Text("it's")).Str(), "'it''s'")
	checkEq("Instr", mst.Instr(mst.Text("hello"), mst.Text("ll")).Int64(), int64(3))
	checkEq("Unicode", mst.Unicode(mst.Text("A")).Int64(), int64(65))
	checkEq("Char", mst.Char(mst.Int(72), mst.Int(105)).Str(), "Hi")
	checkEq("Sign", mst.Sign(mst.Real(-3.2)).Int64(), int64(-1))
	checkEq("Glob match", mst.Glob("h*o", "hello"), true)
	checkEq("Glob non-match", mst.Glob("h?o", "hello"), false)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// queryStrings runs a query and joins each row's columns with commas.
func queryStrings(db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []string{}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		parts := make([]string, len(cells))
		for i, c := range cells {
			switch v := c.(type) {
			case nil:
				parts[i] = "NULL"
			case []byte:
				parts[i] = string(v)
			case float64:
				parts[i] = trimFloat(v)
			default:
				parts[i] = fmt.Sprintf("%v", v)
			}
		}
		out = append(out, strings.Join(parts, ","))
	}
	return out, rows.Err()
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%.4f", f)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func describeErr(err error) string {
	if err == nil {
		return "err=<nil>"
	}
	return "err=" + firstLine(err.Error())
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 90 {
		s = s[:90] + "..."
	}
	return s
}
