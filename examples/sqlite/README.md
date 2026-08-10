# Example: `github.com/malcolmston/sqlite`

A self-checking example program that validates the published
`github.com/malcolmston/sqlite` module as an **outside consumer** would: the
dependency is fetched from the module proxy, with **no `replace` directive** and
no reference to the sibling `../../sqlite` working tree.

## What the library actually is

A **pure-Go, dependency-free, in-memory SQL engine** — tokenizer, parser,
tree-walking executor and value model — that registers a `database/sql` driver
under the name `mstsqlite` (exported as `sqlite.DriverName`). It is *not* a cgo
wrapper around real SQLite, and it has no on-disk file format; the "SQLite" in
the name refers to the SQL dialect and the dynamic-typing model it imitates.

It also exposes a lower-level API: `sqlite.NewDatabase()` / `(*Database).Exec` /
`(*Database).Query` for direct use, `sqlite.Parse` for AST inspection, and the
scalar-function registry (`ScalarNames`, `LookupScalar`, `CallScalar`, plus Go
functions such as `Abs`, `Substr`, `Coalesce`, `Glob`).

## Resolved module version

```
github.com/malcolmston/sqlite v0.0.0-20260719021424-60a28147f11e
```

(The repository has no semver tags, so `@latest` resolves to a pseudo-version.
The published `README.md` and `VERSION` claim `0.1.0`.)

## How to run

The parent repo uses a Go workspace, so `GOWORK=off` is required to consume the
published module rather than the local one:

```sh
cd examples/sqlite
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Each assertion prints `[PASS]` or `[FAIL]` with the observed and expected value.
The program exits `0` only when every check passes, and always terminates on its
own (no network, no timers, no goroutines).

## What it demonstrates

16 labelled sections:

1. Opening an in-memory database through `database/sql` (`:memory:` DSN, pool
   pinned to one connection), plus `Ping`.
2. `CREATE TABLE` with `INTEGER` / `TEXT` / `REAL` / `BLOB` columns,
   `PRIMARY KEY`, `NOT NULL`, and `CREATE TABLE IF NOT EXISTS`.
3. Multi-row literal `INSERT`, parameterized `INSERT` with `?` placeholders,
   `RowsAffected` / `LastInsertId`, and a `[]byte` BLOB round-trip.
4. `SELECT` with `WHERE`, `ORDER BY ASC/DESC`, `LIMIT`/`OFFSET`, `LIKE`,
   `NOT LIKE`, `IN (…)`, `IS NULL`, `IS NOT NULL`, `DISTINCT`, and `||`.
5. `COUNT(*)`, `COUNT(DISTINCT x)`, `SUM`, `AVG`, `MIN`, `MAX`, `GROUP BY`,
   `HAVING`.
6. Two-table `INNER JOIN` with aliases, combined with `WHERE` and `GROUP BY`,
   and scalar functions (`UPPER(SUBSTR(...))`) inside a query.
7. `UPDATE … SET col = col * 2 WHERE …` and `DELETE … WHERE …`, verifying
   `RowsAffected` and the resulting rows.
8. Prepared statements (`db.Prepare` + `stmt.Query`).
9. A transaction that **commits**.
10. A transaction that **rolls back**, checking read-own-writes inside the
    transaction and that both the `INSERT` and the `DELETE` are undone.
11. Indexes — see holes below; the section asserts the documented rejection of
    `CREATE INDEX` and that `PRIMARY KEY` uniqueness is still enforced.
12. Error handling: 18 bad or unsupported statements, an argument-count mismatch,
    the two silent-misparse bugs below, and proof the connection stays usable.
13. Named (shared) in-memory databases visible across two `sql.DB` handles, plus
    `DROP TABLE` and `DROP TABLE IF EXISTS`.
14. The direct `*sqlite.Database` API: `Exec`, `Query`, `ExecResult`,
    `ResultSet`, `Value` accessors (`Int64`/`Str`/`Float64`/`Bytes`/`GoValue`/
    `IsNull`/`Type`), constructors, and three-valued NULL logic.
15. `sqlite.Parse` and the exported AST (`SelectStmt`, `JoinClause`,
    `OrderTerm`, `CreateTableStmt`, `ColumnDef`, and the DML/transaction nodes).
16. The scalar function registry and its 22 Go-level helpers.

## Holes found

Two of these are genuine correctness bugs; the rest are missing features, some
documented and some not.

### Correctness bugs

1. **`LEFT JOIN` on an un-aliased table silently degrades to `INNER JOIN`.**
   `SELECT * FROM books LEFT JOIN authors ON …` parses successfully: the parser
   treats `LEFT` as the *alias* of `books` (`SelectStmt.FromAlias == "LEFT"`) and
   then parses `JOIN authors ON …` as a plain inner join. No error is returned
   and unmatched left rows are silently dropped — the worst possible failure mode
   for an unsupported feature. The same shape applies to `RIGHT`, `FULL`,
   `CROSS` and `OUTER`. When the left table *does* carry an explicit alias the
   query fails, but with the unhelpful message
   `unexpected trailing token "LEFT"` that never mentions outer joins.

2. **An omitted `INTEGER PRIMARY KEY` is not auto-assigned.** In real SQLite,
   `INSERT INTO t (name) VALUES ('x')` on a table with `id INTEGER PRIMARY KEY`
   assigns the next rowid. Here it fails, and the error misreports a constraint
   that was never declared: `NOT NULL constraint failed: t.id`. Combined with
   hole 5 (no `AUTOINCREMENT`, no `DEFAULT`), every row's primary key must be
   supplied by hand — the example has to hard-code every `id`.

3. **`Value.String()` loses REAL-vs-INTEGER distinction.** The REAL `2.0` renders
   as `"2"`, indistinguishable from the INTEGER `2` (SQLite renders `2.0`).
   `typeof()` and `Value.Type` are still correct, so this is limited to text
   rendering, but it makes `ResultSet` output ambiguous.

### Missing / unsupported

4. **No secondary indexes at all.** `CREATE INDEX` is not in the grammar
   (`expected "TABLE", got "INDEX"`), there is no `DROP INDEX`, no `UNIQUE`
   column constraint, and no query planner — every lookup is a full table scan.
   This is documented in the library README's "Limits", but it means the
   "indexes" part of a normal SQL exercise cannot be demonstrated at all; the
   example asserts the rejection instead.

5. **Column-definition grammar is minimal.** `UNIQUE`, `DEFAULT`,
   `AUTOINCREMENT`, `CHECK`, `COLLATE` and `REFERENCES` are all parse errors;
   only `PRIMARY KEY` and `NOT NULL` are accepted, and only one `PRIMARY KEY`
   column per table.

6. **Expression grammar gaps beyond what the README lists.** `CASE WHEN … END`,
   `BETWEEN … AND …`, `CAST(x AS T)` and `COLLATE` are all parse errors. `CASE`
   in particular produces a misleading `unexpected trailing token` error rather
   than "unsupported".

7. **Query-shape gaps.** No `UNION`/`INTERSECT`/`EXCEPT`, no subqueries, no
   comma cross-joins (`FROM a, b`), no more than one join per query, no
   qualified names (`main.books` → `unexpected trailing token "."`), no
   `INSERT … SELECT`, no `INSERT OR REPLACE`, no `ALTER TABLE`, no `PRAGMA`, no
   `EXPLAIN`, no views/triggers/CTEs/window functions/foreign keys.

8. **Aggregate set is small.** `group_concat` and `total` are missing, and
   `MAX`/`MIN` are aggregate-only — the two-argument scalar form `max(1,2)`
   fails with `MAX expects one argument`. There is no `random()` and no
   date/time function family (`date`, `time`, `strftime`, …), which makes the
   engine awkward for anything with timestamps.

9. **No on-disk persistence.** Documented, but worth restating: `":memory:"` and
   named in-memory databases are the only options, and named databases live for
   the process lifetime with no way to drop or enumerate them.

### API-shape / usability friction

10. **The direct `*sqlite.Database` API is not transaction-capable.** It has no
    `Begin`/`Commit`/`Rollback` methods, and `store.Exec("BEGIN")` is rejected
    with `statement *sqlite.BeginStmt cannot be executed here`. Transaction state
    lives on the unexported `conn` type, reachable only through `database/sql`.
    The library README advertises the direct API without mentioning this, so a
    caller who picks it must give up transactions entirely.

11. **`Exec` and `Query` return inconsistent shapes**: `Exec` returns
    `(ExecResult, error)` by value while `Query` returns `(*ResultSet, error)` by
    pointer.

12. **`Exec`/`Query` on `*Database` take `...interface{}` and no `context.Context`**,
    so the direct API has no cancellation story. The `database/sql` layer accepts
    a `context.Context` but ignores it (`ExecContext(_ context.Context, …)`),
    meaning `QueryContext` with a cancelled or deadline-bound context will still
    run the query to completion.

13. **Transactions serialize the entire database.** `Begin` takes a process-wide
    mutex on the `*Database` and holds it until `Commit`/`Rollback`, and
    rollback works by cloning every table up front (`snapshot`). Combined with
    the requirement to call `db.SetMaxOpenConns(1)` for `":memory:"`, concurrency
    is effectively one statement at a time. Forgetting `SetMaxOpenConns(1)` is an
    easy trap: each new pooled connection gets a *different* private anonymous
    database, so tables appear and disappear depending on which connection the
    pool hands out — this is documented but not enforced or detectable at runtime.

14. **Error values are unstructured.** Every engine error is a bare
    `fmt.Errorf("sqlite: …")` string; there are no sentinel errors and no error
    type carrying a code, so callers cannot distinguish "no such table" from a
    constraint violation without string matching. The one exception is
    `*sqlite.FuncError` for scalar-function problems, which does work with
    `errors.As`.

### Non-issues (verified working)

Placeholder binding, Go-value conversion and column type affinity
(`INSERT ... VALUES ('123')` into an `INTEGER` column stores an integer), the
`1/0 → NULL` and integer-vs-real division rules, three-valued NULL logic,
`ORDER BY` on multiple keys, `ORDER BY` on an output alias or on an aggregate,
`LIMIT -1` meaning "no limit", double-quoted identifiers, trailing semicolons,
`NOT LIKE`, `GLOB`, and `UNIQUE constraint failed: t.id` on a duplicate primary
key all behave as SQLite does. Rejection of trailing garbage after a statement is
correct in every case except the `LEFT`-as-alias bug above.
