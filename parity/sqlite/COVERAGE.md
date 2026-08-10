# `sqlite` parity coverage

| | |
| --- | --- |
| upstream oracle | **`sqlite3` 3.45.3** (`sqlite3 --version` -> `3.45.3 2024-04-15 13:34:05 8653b758870e6ef0c98d46b3ace27849054af85da891eb121e9aaa537f1e8355 (64-bit)`) |
| Go port | **`github.com/malcolmston/sqlite v0.2.0`** (published module, no `replace` directive) |
| upstream runner | `c/run.py` -> `sqlite3 -batch -init /dev/null :memory:` |
| Go runner | `go/run.go` -> `database/sql` driver `mstsqlite`, DSN `:memory:`, `SetMaxOpenConns(1)` |
| cases | **428** across 14 groups |
| case parity | **296 / 428 = 69.16%** |

## How each case works

A case is a **script**: a list of statements run in order against a brand-new
in-memory database, then one final `SELECT` whose result *is* the case value.
Every final `SELECT` has an explicit `ORDER BY` (or returns one row), so row
order is defined on both sides.

The value is `{"columns": [...], "rows": [[[tag, payload], ...], ...]}` with
`tag` in `null | int | real | text | blob`. The tag is carried explicitly so the
port's INTEGER-vs-REAL typing is compared rather than flattened by string
coercion: `int` payloads are exact decimal strings, `real` payloads are JSON
numbers (non-finite ones become `"inf"`/`"-inf"`/`"nan"`), `blob` payloads are
lowercase hex.

### Exactly how the oracle is invoked

One `sqlite3` process per case, so each case gets a fresh private database:

```
sqlite3 -batch -init /dev/null :memory:
```

fed a generated script that always starts with

```
.bail on
.mode quote
.headers on
.separator "\t"
```

`.mode quote` is the load-bearing choice: it is the only CLI output mode that
does **not** coerce types, rendering each value as the SQL literal that
reproduces it -- `NULL`, `1`, `1.0`, `'a''b'`, `X'6162'`. REAL always carries a
`.` or an exponent, so REAL and INTEGER can never be confused, and SQLite 3.45
prints REAL with enough digits to round-trip exactly through `float`. Those
literals are parsed back into `[tag, payload]` pairs. `.bail on` makes the first
error abort the script with a non-zero exit, which becomes `ok:false`.

`.print @@BEGIN` / `.print @@END` fence the final `SELECT`'s output so that
anything a setup statement prints (`PRAGMA`, `EXPLAIN`, ...) is discarded, and a
missing `@@END` is a reliable signal that the script aborted.

Two adaptations, applied symmetrically and deliberately:

* **`?` parameters.** The CLI cannot bind bare `?`, so the k-th bare `?`
  (string-literal aware) is rewritten to `?k` and bound with
  `.parameter set ?k <literal>`. The Go runner passes the same values through
  `database/sql` arguments.
* **Empty results have no column names.** No `sqlite3` output mode prints a
  header row for a zero-row result, so both runners report `columns: null` when
  there are no rows. Column names are compared for every case that returns at
  least one row.

`BEGIN`/`COMMIT`/`ROLLBACK` appear in cases in two forms on purpose: as
`{"txn": "..."}` steps, which the Go runner drives through `database/sql`'s
transaction API (the port's idiomatic path) and the upstream runner emits as
plain SQL; and as literal SQL strings, which both sides simply execute.

## Score by case group

| group | cases | match | silent wrong answer | port rejects | port-only accepts |
| --- | --- | --- | --- | --- | --- |
| `aggregates` | 26 | 22 | 0 | 4 | 0 |
| `bugs` | 7 | 0 | 3 | 4 | 0 |
| `ddl` | 37 | 13 | 0 | 24 | 0 |
| `dml` | 22 | 15 | 0 | 7 | 0 |
| `errors` | 20 | 20 | 0 | 0 | 0 |
| `functions` | 85 | 60 | 0 | 25 | 0 |
| `joins` | 15 | 6 | 1 | 8 | 0 |
| `null-logic` | 21 | 21 | 0 | 0 | 0 |
| `operators` | 30 | 22 | 6 | 2 | 0 |
| `params` | 17 | 17 | 0 | 0 | 0 |
| `select-basic` | 37 | 37 | 0 | 0 | 0 |
| `txn` | 12 | 10 | 0 | 2 | 0 |
| `types` | 47 | 47 | 0 | 0 | 0 |
| `unsupported` | 52 | 6 | 0 | 46 | 0 |
| **total** | **428** | **296** | **10** | **122** | **0** |

## Silent wrong answers (highest-value divergences)

These are the cases where **both engines succeeded and the rows differ**: the
port answered the query without complaining and the answer was wrong.

| case | SQL surface | upstream | port |
| --- | --- | --- | --- |
| `bug-left-join-unaliased` | LEFT JOIN on an un-aliased table | 5 rows `[[["int", "1"], ["int", "100"]], [["int", "1"], ["int", "2...` | 3 rows `[[["int", "1"], ["int", "100"]], [["int", "1"], ["int", "2...` |
| `bug-left-join-unaliased-count` | LEFT JOIN row count on an un-aliased table | 1 row `[[["int", "5"]]]` | 1 row `[[["int", "3"]]]` |
| `bug-left-join-unaliased-nulls` | LEFT JOIN unmatched rows | 1 row `[[["int", "2"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-01` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-02` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-03` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-05` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-06` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `glob-07` | GLOB | 1 row `[[["int", "1"]]]` | 1 row `[[["int", "0"]]]` |
| `join-right` | RIGHT JOIN | 4 rows `[[["int", "10"]], [["int", "11"]], [["int", "12"]], [["int...` | 3 rows `[[["int", "10"]], [["int", "11"]], [["int", "12"]]]` |

Root causes, three distinct bugs:

1. **`LEFT JOIN` / `RIGHT JOIN` on an un-aliased table degrades to `INNER JOIN`.**
   The parser consumes `LEFT` (or `RIGHT`) as the *alias* of the preceding
   table, then finds a bare `JOIN`, so unmatched rows are silently dropped with
   no error. `t AS a LEFT JOIN o AS b` is instead rejected outright
   (`unexpected trailing token "LEFT"`), so the only way to write a `LEFT JOIN`
   the port accepts is the way that gives the wrong answer.
2. **`GLOB` is parsed as a synonym for `LIKE`.** `parser.go` maps both keywords
   onto the same `LikeExpr` node, so `'abc' GLOB 'a*'` is evaluated with `LIKE`
   semantics and `*`/`?`/`[...]` are treated as literal characters. The port does
   ship a correct `Glob()` implementation, but the operator never reaches it,
   and the `glob(pattern, text)` *function* form is unparseable because `GLOB`
   is a reserved keyword.
3. (Reported as a silent bug, observed as a loud one.) **An omitted
   `INTEGER PRIMARY KEY` is not auto-assigned a rowid.** In v0.2.0 this is not
   silent: `INSERT INTO p (name) VALUES ('a')` fails with
   `NOT NULL constraint failed: p.id` (cases `bug-rowid-omitted-pk`,
   `bug-rowid-omitted-pk-multi`, `bug-rowid-explicit-null`). There is also no
   implicit `rowid` pseudo-column at all (`bug-rowid-pseudo-column`).

## SQL the oracle accepts and the port rejects

122 cases. Grouped by what is missing:

| bucket | cases | examples of the port's error |
| --- | --- | --- |
| subqueries and set operations | 16 | `sqlite: expected "VALUES", got "SELECT"`; `sqlite: expected identifier, got "("`; `sqlite: expected statement keyword, got "WITH"`; `sqlite: unexpected keyword "EXISTS" in expression` |
| `CASE` / `BETWEEN` / `CAST` / bitwise operators | 18 | `sqlite: expected ")", got "AS"`; `sqlite: expected "NULL", got "1"`; `sqlite: expected "NULL", got "2"`; `sqlite: unexpected character "&" at 9` |
| joins beyond one un-aliased INNER join | 8 | `sqlite: expected "ON", got "("`; `sqlite: expected "ON", got "ORDER"`; `sqlite: expected "ON", got "WHERE"`; `sqlite: unexpected trailing token ","` |
| DDL constraints and objects | 33 | `sqlite: expected "(", got "AS"`; `sqlite: expected ")", got "AUTOINCREMENT"`; `sqlite: expected ")", got "CHECK"`; `sqlite: expected ")", got "COLLATE"` |
| DML beyond plain INSERT/UPDATE/DELETE | 5 | `sqlite: expected "INTO", got "OR"`; `sqlite: expected "VALUES", got "DEFAULT"`; `sqlite: expected statement keyword, got "REPLACE"`; `sqlite: unexpected trailing token "ON"` |
| built-in functions | 20 | `sqlite: MAX expects one argument`; `sqlite: MIN expects one argument`; `sqlite: unknown function CHANGES()`; `sqlite: unknown function DATE()` |
| aggregates | 3 | `sqlite: unknown function GROUP_CONCAT()`; `sqlite: unknown function TOTAL()` |
| clause-level extras | 19 | `sqlite: NOT NULL constraint failed: p.id`; `sqlite: expected statement keyword, got "SAVEPOINT"`; `sqlite: no such column: q`; `sqlite: no such column: rowid` |

Full list: `agg-group-concat`, `agg-group-concat-sep`, `agg-total`, `bug-rowid-explicit-null`, `bug-rowid-omitted-pk`, `bug-rowid-omitted-pk-multi`, `bug-rowid-pseudo-column`, `ddl-alter-add-column`, `ddl-alter-drop-column`, `ddl-alter-rename-column`, `ddl-alter-rename-table`, `ddl-autoincrement`, `ddl-backtick-ident`, `ddl-bracket-ident`, `ddl-check-ok`, `ddl-collate`, `ddl-create-index`, `ddl-create-table-as`, `ddl-create-trigger`, `ddl-create-unique-index`, `ddl-create-view`, `ddl-default`, `ddl-drop-index`, `ddl-foreign-key-table`, `ddl-generated-column`, `ddl-pk-composite`, `ddl-pk-table-constraint`, `ddl-references`, `ddl-temp-table`, `ddl-unique`, `ddl-without-rowid`, `fn-cast-blob`, `fn-cast-int`, `fn-cast-numeric`, `fn-cast-real`, `fn-cast-text`, `fn-changes`, `fn-date`, `fn-date-modifier`, `fn-datetime`, `fn-datetime-unixepoch`, `fn-format`, `fn-iif`, `fn-julianday`, `fn-last-insert-rowid`, `fn-likely`, `fn-max-scalar`, `fn-min-scalar`, `fn-printf`, `fn-randomblob-len`, `fn-strftime`, `fn-time`, `fn-total-changes`, `fn-unixepoch`, `fn-unlikely`, `fn-zeroblob`, `glob-func`, `group-by-null`, `insert-default-values`, `insert-or-ignore`, `insert-or-replace`, `insert-select`, `insert-select-where`, `insert-upsert`, `join-cross-comma`, `join-cross-keyword`, `join-full`, `join-left-aliased`, `join-left-outer-aliased`, `join-natural`, `join-three`, `join-using`, `like-func`, `replace-into`, `txn-rollback-to-savepoint`, `txn-savepoint`, `u-agg-over-window`, `u-analyze`, `u-attach`, `u-between`, `u-bitand`, `u-bitnot`, `u-bitor`, `u-case-no-else`, `u-case-operand`, `u-case-when`, `u-cast-expr`, `u-collate-expr`, `u-cte`, `u-cte-recursive`, `u-except`, `u-exists-expr`, `u-explain`, `u-explain-qp`, `u-filter-clause`, `u-intersect`, `u-is-not-value`, `u-is-value`, `u-like-escape`, `u-multi-values-select`, `u-not-between`, `u-order-collate`, `u-order-nulls-last`, `u-pragma-foreign-keys`, `u-pragma-table-info`, `u-pragma-user-version`, `u-regexp`, `u-select-all-kw`, `u-shift-left`, `u-shift-right`, `u-sqlite-master`, `u-subquery-correlated`, `u-subquery-exists`, `u-subquery-from`, `u-subquery-in`, `u-subquery-not-exists`, `u-subquery-scalar`, `u-union`, `u-union-all`, `u-vacuum`, `u-values-stmt`, `u-window-row-number`

## SQL the port accepts and the oracle rejects

**None.** Across all 428 cases there is no script the port accepts and real
SQLite rejects: the port's grammar is a strict subset, never a superset.

## Inventory 1 -- the SQL surface these cases target

Every case declares the surface it exercises in its `upstreamFn` field, so the
distinct labels are the targeted inventory with exact per-item attribution --
no keyword-in-the-text guessing. 409 items over 428 cases.

Status folding: `match` = every case for the item agrees; `differs` = at least
one case where both engines ran and the rows differ; `missing` = every
disagreement is the port rejecting a script the oracle executed.

| SQL surface | status | cases |
| --- | --- | --- |
| `!= operator` | match | `u-ne-bang` |
| `-- line comment` | match | `u-comment-line` |
| `/* block comment */` | match | `u-comment-block` |
| `1.0/0 -> NULL` | match | `div-zero-real` |
| `1/0 -> NULL` | match | `div-zero-int` |
| `? as a LIKE pattern` | match | `param-like` |
| `? as LIMIT` | match | `param-limit` |
| `? bound to a BLOB` | match | `param-blob` |
| `? bound to NULL` | match | `param-null` |
| `? bound to NULL in a comparison` | match | `param-null-compare` |
| `? in a multi-row INSERT` | match | `param-insert-multi` |
| `? in DELETE` | match | `param-delete` |
| `? in HAVING` | match | `param-having` |
| `? in INSERT` | match | `param-insert` |
| `? in the SELECT list` | match | `param-select-list` |
| `? in UPDATE` | match | `param-update` |
| `? in WHERE (INTEGER)` | match | `param-where-int` |
| `? in WHERE (REAL)` | match | `param-where-real` |
| `? in WHERE (TEXT)` | match | `param-where-text` |
| `? inside an expression` | match | `param-expr` |
| `? inside IN (...)` | match | `param-in-list` |
| `abs()` | match | `fn-abs-int` |
| `abs() near INTEGER min` | match | `fn-abs-overflow` |
| `abs() on TEXT` | match | `fn-abs-text` |
| `abs() REAL` | match | `fn-abs-real` |
| `abs(NULL)` | match | `fn-abs-null` |
| `addition` | match | `add-int` |
| `aggregate FILTER clause` | missing | `u-filter-clause` |
| `aggregate in WHERE` | match | `err-agg-in-where` |
| `aggregate over an expression` | match | `agg-expr` |
| `aggregate used as a window function` | missing | `u-agg-over-window` |
| `ALTER TABLE ... ADD COLUMN` | missing | `ddl-alter-add-column` |
| `ALTER TABLE ... DROP COLUMN` | missing | `ddl-alter-drop-column` |
| `ALTER TABLE ... RENAME COLUMN` | missing | `ddl-alter-rename-column` |
| `ALTER TABLE ... RENAME TO` | missing | `ddl-alter-rename-table` |
| `ANALYZE` | missing | `u-analyze` |
| `ATTACH DATABASE` | missing | `u-attach` |
| `AUTOINCREMENT` | missing | `ddl-autoincrement` |
| `avg()` | match | `agg-avg` |
| `avg() of integers is REAL` | match | `agg-avg-int-exact` |
| `avg() over only NULLs` | match | `agg-avg-null-only` |
| `backtick-quoted identifiers` | missing | `ddl-backtick-ident` |
| `bare VALUES statement` | missing | `u-values-stmt` |
| `BEGIN ... COMMIT persists` | match | `txn-commit-visible` |
| `BEGIN ... ROLLBACK discards` | match | `txn-rollback-discards` |
| `BEGIN TRANSACTION / COMMIT TRANSACTION as plain SQL` | match | `txn-raw-begin-transaction` |
| `BEGIN/COMMIT as plain SQL statements` | match | `txn-raw-begin-sql` |
| `BETWEEN ... AND` | missing | `u-between` |
| `bitwise &` | missing | `u-bitand` |
| `bitwise <<` | missing | `u-shift-left` |
| `bitwise >>` | missing | `u-shift-right` |
| `bitwise \|` | missing | `u-bitor` |
| `bitwise ~` | missing | `u-bitnot` |
| `BLOB column round-trip` | match | `column-affinity-blob` |
| `BLOB literal x'...'` | match | `type-blob-literal` |
| `bracket-quoted identifiers` | missing | `ddl-bracket-ident` |
| `CASE <expr> WHEN` | missing | `u-case-operand` |
| `CASE WHEN ... THEN ... ELSE ... END` | missing | `u-case-when` |
| `CASE with no ELSE` | missing | `u-case-no-else` |
| `CAST in a SELECT list` | missing | `u-cast-expr` |
| `CAST(... AS BLOB)` | missing | `fn-cast-blob` |
| `CAST(... AS INTEGER)` | missing | `fn-cast-int` |
| `CAST(... AS NUMERIC)` | missing | `fn-cast-numeric` |
| `CAST(... AS REAL)` | missing | `fn-cast-real` |
| `CAST(... AS TEXT)` | missing | `fn-cast-text` |
| `changes()` | missing | `fn-changes` |
| `char()` | match | `fn-char-one` |
| `char() variadic` | match | `fn-char-many` |
| `char() with no arguments` | match | `fn-char-none` |
| `CHECK column constraint` | missing | `ddl-check-ok` |
| `CHECK violated (must fail)` | match | `ddl-check-violation` |
| `coalesce()` | match | `coalesce-first`, `fn-coalesce-fn` |
| `coalesce() all NULL` | match | `coalesce-all-null` |
| `COLLATE column constraint` | missing | `ddl-collate` |
| `COLLATE in an expression` | missing | `u-collate-expr` |
| `composite PRIMARY KEY` | missing | `ddl-pk-composite` |
| `correlated scalar subquery` | missing | `u-subquery-correlated` |
| `count(*)` | match | `agg-count-star` |
| `count(*) over no rows -> 0` | match | `agg-count-empty` |
| `count(column) skips NULL` | match | `agg-count-col` |
| `count(DISTINCT text)` | match | `agg-count-distinct-text` |
| `count(DISTINCT x)` | match | `agg-count-distinct` |
| `CREATE INDEX` | missing | `ddl-create-index` |
| `CREATE TABLE ... AS SELECT` | missing | `ddl-create-table-as` |
| `CREATE TABLE duplicate (must fail)` | match | `ddl-duplicate` |
| `CREATE TABLE IF NOT EXISTS` | match | `ddl-if-not-exists` |
| `CREATE TABLE with typed and untyped columns` | match | `ddl-types` |
| `CREATE TEMP TABLE` | missing | `ddl-temp-table` |
| `CREATE TRIGGER` | missing | `ddl-create-trigger` |
| `CREATE UNIQUE INDEX` | missing | `ddl-create-unique-index` |
| `CREATE VIEW` | missing | `ddl-create-view` |
| `CROSS JOIN` | missing | `join-cross-keyword` |
| `date()` | missing | `fn-date` |
| `date() with a modifier` | missing | `fn-date-modifier` |
| `datetime()` | missing | `fn-datetime` |
| `datetime(..., 'unixepoch')` | missing | `fn-datetime-unixepoch` |
| `DEFAULT column constraint` | missing | `ddl-default` |
| `DELETE ... LIMIT (needs SQLITE_ENABLE_UPDATE_DELETE_LIMIT)` | match | `delete-limit` |
| `DELETE ... WHERE` | match | `delete-where` |
| `DELETE from an unknown table` | match | `err-delete-no-table` |
| `DELETE on IS NULL` | match | `delete-null-pred` |
| `DELETE without WHERE` | match | `delete-all` |
| `derived table FROM (SELECT ...)` | missing | `u-subquery-from` |
| `double-quoted identifiers` | match | `ddl-quoted-ident` |
| `DROP INDEX` | missing | `ddl-drop-index` |
| `DROP TABLE` | match | `ddl-drop` |
| `DROP TABLE IF EXISTS` | match | `ddl-drop-if-exists` |
| `DROP TABLE missing (must fail)` | match | `ddl-drop-missing` |
| `empty BLOB literal` | match | `type-blob-empty` |
| `EXCEPT` | missing | `u-except` |
| `EXISTS (subquery)` | missing | `u-subquery-exists` |
| `EXISTS in a SELECT list` | missing | `u-exists-expr` |
| `EXPLAIN` | missing | `u-explain` |
| `EXPLAIN QUERY PLAN` | missing | `u-explain-qp` |
| `explicit NULL into INTEGER PRIMARY KEY auto-assigns a rowid` | missing | `bug-rowid-explicit-null` |
| `false is 0` | match | `bool-false` |
| `format()` | missing | `fn-format` |
| `FROM (VALUES ...)` | missing | `u-multi-values-select` |
| `FROM a, b (comma join)` | missing | `join-cross-comma` |
| `FROM t alias` | match | `table-alias-bare` |
| `FROM t AS alias` | match | `table-alias` |
| `FROM with no table` | match | `err-syntax-trailing` |
| `FULL OUTER JOIN` | missing | `join-full` |
| `GENERATED ALWAYS AS column` | missing | `ddl-generated-column` |
| `GLOB` | differs | `glob-00`, `glob-01`, `glob-02`, `glob-03`, `glob-04` |
| `glob() function` | missing | `glob-func` |
| `GROUP BY` | match | `group-by` |
| `GROUP BY ... HAVING` | match | `group-by-having` |
| `GROUP BY a, b` | match | `group-by-two` |
| `GROUP BY an expression` | match | `u-group-by-expr` |
| `GROUP BY an unknown column` | match | `err-group-by-unknown` |
| `GROUP BY groups NULLs together` | missing | `group-by-null` |
| `group_concat()` | missing | `agg-group-concat` |
| `group_concat(x, sep)` | missing | `agg-group-concat-sep` |
| `HAVING over sum()` | match | `group-by-having-sum` |
| `hex() on BLOB` | match | `fn-hex-blob` |
| `hex() on INTEGER` | match | `fn-hex-int` |
| `hex() on TEXT` | match | `fn-hex-text` |
| `ifnull()` | match | `fn-ifnull-fn`, `ifnull-null` |
| `ifnull() non-NULL` | match | `ifnull-set` |
| `iif()` | missing | `fn-iif` |
| `implicit rowid pseudo-column` | missing | `bug-rowid-pseudo-column` |
| `IN (...)` | match | `where-in` |
| `IN (...) no match` | match | `where-in-empty-match` |
| `IN (subquery)` | missing | `u-subquery-in` |
| `IN comparing INTEGER to REAL` | match | `in-real` |
| `IN with mixed types` | match | `in-mixed` |
| `INNER JOIN ... ON` | match | `join-inner-on` |
| `INSERT ... DEFAULT VALUES` | missing | `insert-default-values` |
| `INSERT ... ON CONFLICT DO UPDATE (upsert)` | missing | `insert-upsert` |
| `INSERT ... SELECT` | missing | `insert-select` |
| `INSERT ... SELECT ... WHERE` | missing | `insert-select-where` |
| `INSERT into an unknown column` | match | `err-insert-unknown-column` |
| `INSERT into an unknown table` | match | `err-insert-no-table` |
| `INSERT INTO t (cols) VALUES (...)` | match | `insert-cols` |
| `INSERT omitting a nullable column` | match | `insert-partial-cols` |
| `INSERT omitting INTEGER PRIMARY KEY auto-assigns a rowid` | missing | `bug-rowid-omitted-pk` |
| `INSERT OR IGNORE` | missing | `insert-or-ignore` |
| `INSERT OR REPLACE` | missing | `insert-or-replace` |
| `INSERT value/column count mismatch` | match | `err-insert-count-mismatch` |
| `INSERT with an expression` | match | `insert-expr` |
| `INSERT without a column list` | match | `insert-no-collist` |
| `instr()` | match | `fn-instr-hit` |
| `instr() no match` | match | `fn-instr-miss` |
| `instr(NULL, ...)` | match | `fn-instr-null` |
| `INTEGER = REAL` | match | `cmp-int-real` |
| `INTEGER column affinity coerces '123'` | match | `column-affinity-int-text` |
| `integer division` | match | `div-int` |
| `INTEGER literal` | match | `type-int-literal` |
| `INTEGER max` | match | `type-maxint` |
| `INTEGER near-min` | match | `type-minint` |
| `INTEGER PRIMARY KEY auto-numbering sequence` | missing | `bug-rowid-omitted-pk-multi` |
| `INTEGER vs TEXT ordering` | match | `cmp-int-text` |
| `INTERSECT` | missing | `u-intersect` |
| `IS NOT NULL` | match | `where-is-not-null` |
| `IS NOT with a non-NULL operand` | missing | `u-is-not-value` |
| `IS NULL` | match | `where-is-null` |
| `IS with a non-NULL operand` | missing | `u-is-value` |
| `JOIN + GROUP BY` | match | `join-agg` |
| `JOIN + WHERE` | match | `join-where` |
| `JOIN ... ON (INNER implied)` | match | `join-bare-join` |
| `JOIN ... USING (col)` | missing | `join-using` |
| `JOIN ON a constant` | match | `join-on-const` |
| `JOIN with aliases` | match | `join-aliased` |
| `julianday()` | missing | `fn-julianday` |
| `last_insert_rowid()` | missing | `fn-last-insert-rowid` |
| `LEFT JOIN on an un-aliased table` | differs | `bug-left-join-unaliased` |
| `LEFT JOIN row count on an un-aliased table` | differs | `bug-left-join-unaliased-count` |
| `LEFT JOIN unmatched rows` | differs | `bug-left-join-unaliased-nulls` |
| `LEFT JOIN with aliases` | missing | `join-left-aliased` |
| `LEFT OUTER JOIN` | missing | `join-left-outer-aliased` |
| `length()` | match | `fn-length-text` |
| `length() multi-byte` | match | `fn-length-utf8` |
| `length() on BLOB` | match | `fn-length-blob` |
| `length() on INTEGER` | match | `fn-length-int` |
| `length(NULL)` | match | `fn-length-null` |
| `LIKE` | match | `like-00`, `like-01`, `like-02`, `like-03`, `like-04` |
| `LIKE ... ESCAPE` | missing | `u-like-escape` |
| `like() function` | missing | `like-func` |
| `likely()` | missing | `fn-likely` |
| `LIMIT` | match | `limit` |
| `LIMIT ... OFFSET` | match | `limit-offset` |
| `LIMIT 0` | match | `limit-zero` |
| `LIMIT beyond row count` | match | `limit-over` |
| `lower()` | match | `fn-lower-basic` |
| `lower() non-ASCII` | match | `fn-lower-mixed` |
| `ltrim()` | match | `fn-ltrim-basic` |
| `ltrim(x, chars)` | match | `fn-ltrim-chars` |
| `malformed BLOB literal` | match | `err-bad-blob-literal` |
| `malformed statement` | match | `err-syntax-garbage` |
| `max() aggregate` | match | `agg-max` |
| `max() over TEXT` | match | `agg-max-text` |
| `max() scalar (multi-arg)` | missing | `fn-max-scalar` |
| `min() aggregate` | match | `agg-min` |
| `min() over TEXT` | match | `agg-min-text` |
| `min() scalar (multi-arg)` | missing | `fn-min-scalar` |
| `mixed addition` | match | `add-mixed` |
| `mixed INTEGER/REAL division` | match | `div-mixed` |
| `modulo %` | match | `mod-basic` |
| `modulo by zero -> NULL` | match | `mod-zero` |
| `multi-row INSERT` | match | `insert-multi-row` |
| `multiplication` | match | `mul-int` |
| `NATURAL JOIN` | missing | `join-natural` |
| `negative integer division` | match | `div-int-neg` |
| `negative INTEGER literal` | match | `type-neg-int` |
| `negative modulo` | match | `mod-neg` |
| `NOT BETWEEN` | missing | `u-not-between` |
| `NOT EXISTS (subquery)` | missing | `u-subquery-not-exists` |
| `NOT GLOB` | match | `not-glob` |
| `NOT IN (...)` | match | `where-not-in` |
| `NOT IN drops NULL rows` | match | `where-null-not-in` |
| `NOT LIKE` | match | `not-like` |
| `NOT NULL -> NULL` | match | `not-null` |
| `NOT NULL constraint satisfied` | match | `ddl-not-null-ok` |
| `NOT NULL constraint violated (must fail)` | match | `ddl-not-null-violation` |
| `NULL <> NULL -> NULL` | match | `null-ne-null` |
| `NULL = NULL -> NULL` | match | `null-eq-null` |
| `NULL AND false -> false` | match | `null-and-false` |
| `NULL AND true -> NULL` | match | `null-and-true` |
| `NULL arithmetic` | match | `null-plus` |
| `NULL comparison -> NULL` | match | `cmp-null-lt` |
| `NULL IN (...)` | match | `null-in` |
| `NULL IS NOT NULL -> 0` | match | `null-isnotnull` |
| `NULL IS NULL -> 1` | match | `null-isnull` |
| `NULL LIKE` | match | `null-like` |
| `NULL literal` | match | `type-null-literal` |
| `NULL OR false -> NULL` | match | `null-or-false` |
| `NULL OR true -> true` | match | `null-or-true` |
| `nullif()` | match | `fn-nullif-fn` |
| `nullif() different` | match | `nullif-diff` |
| `nullif() equal` | match | `nullif-equal` |
| `OFFSET beyond row count` | match | `offset-over` |
| `ORDER BY ... COLLATE` | missing | `u-order-collate` |
| `ORDER BY ... NULLS LAST` | missing | `u-order-nulls-last` |
| `ORDER BY a, b` | match | `order-multi` |
| `ORDER BY alias` | match | `order-by-alias` |
| `ORDER BY ASC` | match | `order-asc` |
| `ORDER BY DESC` | match | `order-desc` |
| `ORDER BY DESC with NULLs` | match | `order-nulls-desc` |
| `ORDER BY expression` | match | `order-by-expr` |
| `ORDER BY ordinal` | match | `u-order-ordinal` |
| `ORDER BY with no term` | match | `err-order-by-missing` |
| `ORDER BY with NULLs` | match | `order-nulls-asc` |
| `PRAGMA foreign_keys = ON` | missing | `u-pragma-foreign-keys` |
| `PRAGMA table_info` | missing | `u-pragma-table-info` |
| `PRAGMA user_version` | missing | `u-pragma-user-version` |
| `precedence: * over +` | match | `prec-mul-add` |
| `precedence: AND over OR` | match | `prec-and-or` |
| `precedence: \|\| over =` | match | `prec-concat-cmp` |
| `PRIMARY KEY uniqueness (must fail)` | match | `ddl-pk-dup` |
| `printf()` | missing | `fn-printf` |
| `quote() on BLOB` | match | `fn-quote-blob` |
| `quote() on INTEGER` | match | `fn-quote-int` |
| `quote() on TEXT` | match | `fn-quote-text` |
| `quote(NULL)` | match | `fn-quote-null` |
| `randomblob() (length only)` | missing | `fn-randomblob-len` |
| `REAL column affinity coerces 3 -> 3.0` | match | `column-affinity-real` |
| `REAL division` | match | `div-real` |
| `REAL exponent literal` | match | `type-exp-literal` |
| `REAL literal` | match | `type-real-literal` |
| `REAL literal with integral value` | match | `type-real-integral` |
| `REAL multiplication` | match | `mul-real` |
| `REFERENCES column constraint` | missing | `ddl-references` |
| `REGEXP operator` | missing | `u-regexp` |
| `REPLACE INTO` | missing | `replace-into` |
| `replace()` | match | `fn-replace-basic` |
| `replace() empty needle` | match | `fn-replace-empty` |
| `RIGHT JOIN` | differs | `join-right` |
| `ROLLBACK of a CREATE TABLE` | match | `txn-rollback-ddl` |
| `ROLLBACK of a DELETE` | match | `txn-rollback-delete` |
| `ROLLBACK of an UPDATE` | match | `txn-rollback-update` |
| `ROLLBACK TO SAVEPOINT` | missing | `txn-rollback-to-savepoint` |
| `round() half-up` | match | `fn-round-half` |
| `round() negative` | match | `fn-round-neg` |
| `round() on INTEGER` | match | `fn-round-int` |
| `round() wrong arity` | match | `fn-round3` |
| `round(x)` | match | `fn-round-1` |
| `round(x, n)` | match | `fn-round-2` |
| `rtrim()` | match | `fn-rtrim-basic` |
| `rtrim(x, chars)` | match | `fn-rtrim-chars` |
| `SAVEPOINT / RELEASE` | missing | `txn-savepoint` |
| `scalar subquery` | missing | `u-subquery-scalar` |
| `SELECT *` | match | `select-star` |
| `SELECT * with no FROM` | match | `err-select-star-no-from` |
| `SELECT ... AS alias` | match | `select-alias-as` |
| `SELECT <column list>` | match | `select-cols` |
| `SELECT ALL` | missing | `u-select-all-kw` |
| `SELECT DISTINCT` | match | `select-distinct` |
| `SELECT DISTINCT over two columns` | match | `u-distinct-multi` |
| `SELECT returning no rows` | match | `empty-table` |
| `SELECT t.*` | match | `select-qualified-star` |
| `SELECT t.column` | match | `select-qualified-col` |
| `SELECT with an empty column list` | match | `err-select-no-list` |
| `SELECT without FROM` | match | `select-no-from` |
| `several aggregates in one SELECT` | match | `agg-multi` |
| `sign() negative` | match | `fn-sign-neg` |
| `sign() on TEXT` | match | `fn-sign-text` |
| `sign() positive` | match | `fn-sign-pos` |
| `sign() zero` | match | `fn-sign-zero` |
| `soundex()` | match | `fn-soundex` |
| `sqlite_master catalogue` | missing | `u-sqlite-master` |
| `strftime()` | missing | `fn-strftime` |
| `substr() negative length` | match | `fn-substr-neg-len` |
| `substr() negative start` | match | `fn-substr-neg` |
| `substr() start 0` | match | `fn-substr-zero` |
| `substr(x, start)` | match | `fn-substr-2` |
| `substr(x, start, len)` | match | `fn-substr-3` |
| `substring()` | match | `fn-substring-alias` |
| `subtraction` | match | `sub-int` |
| `sum()` | match | `agg-sum` |
| `sum() over no rows -> NULL` | match | `agg-sum-empty` |
| `sum() over REAL` | match | `agg-sum-real` |
| `sum(DISTINCT x)` | match | `agg-distinct-sum` |
| `table-level FOREIGN KEY` | missing | `ddl-foreign-key-table` |
| `table-level PRIMARY KEY (a)` | missing | `ddl-pk-table-constraint` |
| `TEXT column affinity coerces 5 -> '5'` | match | `column-affinity-text` |
| `TEXT literal` | match | `type-text-literal` |
| `TEXT literal with '' escape` | match | `type-text-escaped` |
| `TEXT PRIMARY KEY` | match | `ddl-pk-text` |
| `TEXT vs BLOB ordering` | match | `cmp-text-blob` |
| `three-table JOIN` | missing | `join-three` |
| `time()` | missing | `fn-time` |
| `total()` | missing | `agg-total` |
| `total_changes()` | missing | `fn-total-changes` |
| `trim()` | match | `fn-trim-basic` |
| `trim(x, chars)` | match | `fn-trim-chars` |
| `true is 1` | match | `bool-true` |
| `two ? placeholders` | match | `param-two` |
| `two sequential transactions` | match | `txn-commit-multi` |
| `typeof of integer division` | match | `typeof-div` |
| `typeof of REAL division` | match | `typeof-divreal` |
| `typeof over declared columns` | match | `typeof-columns` |
| `typeof()` | match | `fn-typeof-fn` |
| `typeof(BLOB)` | match | `typeof-blob` |
| `typeof(INTEGER)` | match | `typeof-int` |
| `typeof(NULL)` | match | `typeof-null` |
| `typeof(REAL)` | match | `typeof-real` |
| `typeof(TEXT)` | match | `typeof-text` |
| `unary minus` | match | `unary-minus` |
| `unary plus` | match | `unary-plus` |
| `unbalanced parenthesis` | match | `err-unclosed-paren` |
| `uncommitted rows visible to the writer` | match | `txn-visible-inside` |
| `unicode()` | match | `fn-unicode-a` |
| `unicode() non-ASCII` | match | `fn-unicode-multi` |
| `UNION` | missing | `u-union` |
| `UNION ALL` | missing | `u-union-all` |
| `UNIQUE column constraint` | missing | `ddl-unique` |
| `UNIQUE violated (must fail)` | match | `ddl-unique-violation` |
| `unixepoch()` | missing | `fn-unixepoch` |
| `unknown column` | match | `err-no-such-column` |
| `unknown column in ORDER BY` | match | `err-no-such-column-order` |
| `unknown column in WHERE` | match | `err-no-such-column-where` |
| `unknown function` | match | `fn-unknown-fn` |
| `unknown table` | match | `err-no-such-table` |
| `unknown table qualifier` | match | `err-unknown-table-qualifier` |
| `unlikely()` | missing | `fn-unlikely` |
| `unterminated string literal` | match | `err-unclosed-string` |
| `UPDATE ... SET c = NULL` | match | `update-to-null` |
| `UPDATE ... WHERE` | match | `update-where` |
| `UPDATE an unknown table` | match | `err-update-no-table` |
| `UPDATE matching no rows` | match | `update-no-match` |
| `UPDATE multiple columns` | match | `update-multi-col` |
| `UPDATE with an expression` | match | `update-expr` |
| `UPDATE without WHERE` | match | `update-all` |
| `upper()` | match | `fn-upper-basic` |
| `upper() non-ASCII` | match | `fn-upper-mixed` |
| `upper() wrong arity` | match | `fn-upper-arity` |
| `VACUUM` | missing | `u-vacuum` |
| `value IN list containing NULL` | match | `null-in-null` |
| `WHERE <` | match | `where-lt` |
| `WHERE <=` | match | `where-le` |
| `WHERE <>` | match | `where-ne` |
| `WHERE =` | match | `where-eq` |
| `WHERE >` | match | `where-gt` |
| `WHERE >=` | match | `where-ge` |
| `WHERE AND` | match | `where-and` |
| `WHERE drops NULL rows` | match | `where-null-filtered` |
| `WHERE NOT` | match | `where-not` |
| `WHERE OR` | match | `where-or` |
| `WHERE parentheses` | match | `where-paren` |
| `window function OVER ()` | missing | `u-window-row-number` |
| `WITH ... (common table expression)` | missing | `u-cte` |
| `WITH RECURSIVE` | missing | `u-cte-recursive` |
| `WITHOUT ROWID` | missing | `ddl-without-rowid` |
| `work after a ROLLBACK` | match | `txn-rollback-then-insert` |
| `zeroblob()` | missing | `fn-zeroblob` |
| `\|\| concatenation` | match | `concat-text` |
| `\|\| on numbers` | match | `concat-num` |
| `\|\| with NULL` | match | `concat-null` |

**Targeted surface:** 409 items -- match 282, differs 5, missing 122. Parity over the 287 compared = **98.26%**.

## Inventory 2 -- built-in functions

Derived mechanically from the installed oracle:

```sh
sqlite3 -batch -init /dev/null :memory: \
  "SELECT DISTINCT upper(name) FROM pragma_function_list ORDER BY 1;"   # 193 names
```

and from the port by reflection over the real package:

```go
sqlite.ScalarNames()   // 22 names, plus the 5 aggregates COUNT/SUM/AVG/MIN/MAX
```

A function with no case is `untested` when the port has it and `missing` when it
does not. `differs` means a case exists and the two sides disagree.

| upstream function | port | status | cases |
| --- | --- | --- | --- |
| `->()` | `--` | missing | -- |
| `->>()` | `--` | missing | -- |
| `ABS()` | `ABS` | match | `fn-abs-int`, `fn-abs-null`, `fn-abs-overflow`, `fn-abs-real` |
| `ACOS()` | `--` | missing | -- |
| `ACOSH()` | `--` | missing | -- |
| `ASIN()` | `--` | missing | -- |
| `ASINH()` | `--` | missing | -- |
| `ATAN()` | `--` | missing | -- |
| `ATAN2()` | `--` | missing | -- |
| `ATANH()` | `--` | missing | -- |
| `AVG()` | `AVG` | match | `agg-avg`, `agg-avg-int-exact`, `agg-avg-null-only` |
| `BASE64()` | `--` | missing | -- |
| `BASE85()` | `--` | missing | -- |
| `BM25()` | `--` | missing | -- |
| `CEIL()` | `--` | missing | -- |
| `CEILING()` | `--` | missing | -- |
| `CHANGES()` | `--` | missing | `fn-changes` |
| `CHAR()` | `CHAR` | match | `fn-char-many`, `fn-char-none`, `fn-char-one` |
| `COALESCE()` | `COALESCE` | match | `coalesce-all-null`, `coalesce-first`, `fn-coalesce-fn` |
| `CONCAT()` | `--` | missing | -- |
| `CONCAT_WS()` | `--` | missing | -- |
| `COS()` | `--` | missing | -- |
| `COSH()` | `--` | missing | -- |
| `COUNT()` | `COUNT` | match | `agg-count-col`, `agg-count-distinct`, `agg-count-distinct-text`, `agg-count-empty` |
| `CUME_DIST()` | `--` | missing | -- |
| `CURRENT_DATE()` | `--` | missing | -- |
| `CURRENT_TIME()` | `--` | missing | -- |
| `CURRENT_TIMESTAMP()` | `--` | missing | -- |
| `DATE()` | `--` | missing | `fn-date`, `fn-date-modifier` |
| `DATETIME()` | `--` | missing | `fn-datetime`, `fn-datetime-unixepoch` |
| `DECIMAL()` | `--` | missing | -- |
| `DECIMAL_ADD()` | `--` | missing | -- |
| `DECIMAL_CMP()` | `--` | missing | -- |
| `DECIMAL_EXP()` | `--` | missing | -- |
| `DECIMAL_MUL()` | `--` | missing | -- |
| `DECIMAL_POW2()` | `--` | missing | -- |
| `DECIMAL_SUB()` | `--` | missing | -- |
| `DECIMAL_SUM()` | `--` | missing | -- |
| `DEGREES()` | `--` | missing | -- |
| `DENSE_RANK()` | `--` | missing | -- |
| `DTOSTR()` | `--` | missing | -- |
| `EDIT()` | `--` | missing | -- |
| `EXP()` | `--` | missing | -- |
| `FIRST_VALUE()` | `--` | missing | -- |
| `FLOOR()` | `--` | missing | -- |
| `FORMAT()` | `--` | missing | `fn-format` |
| `FTS3_TOKENIZER()` | `--` | missing | -- |
| `FTS5()` | `--` | missing | -- |
| `FTS5_SOURCE_ID()` | `--` | missing | -- |
| `GEOPOLY_AREA()` | `--` | missing | -- |
| `GEOPOLY_BBOX()` | `--` | missing | -- |
| `GEOPOLY_BLOB()` | `--` | missing | -- |
| `GEOPOLY_CCW()` | `--` | missing | -- |
| `GEOPOLY_CONTAINS_POINT()` | `--` | missing | -- |
| `GEOPOLY_DEBUG()` | `--` | missing | -- |
| `GEOPOLY_GROUP_BBOX()` | `--` | missing | -- |
| `GEOPOLY_JSON()` | `--` | missing | -- |
| `GEOPOLY_OVERLAP()` | `--` | missing | -- |
| `GEOPOLY_REGULAR()` | `--` | missing | -- |
| `GEOPOLY_SVG()` | `--` | missing | -- |
| `GEOPOLY_WITHIN()` | `--` | missing | -- |
| `GEOPOLY_XFORM()` | `--` | missing | -- |
| `GLOB()` | `GLOB` | missing | `glob-func` |
| `GROUP_CONCAT()` | `--` | missing | `agg-group-concat`, `agg-group-concat-sep` |
| `HEX()` | `HEX` | match | `fn-hex-blob`, `fn-hex-int`, `fn-hex-text` |
| `HIGHLIGHT()` | `--` | missing | -- |
| `IEEE754()` | `--` | missing | -- |
| `IEEE754_EXPONENT()` | `--` | missing | -- |
| `IEEE754_FROM_BLOB()` | `--` | missing | -- |
| `IEEE754_INC()` | `--` | missing | -- |
| `IEEE754_MANTISSA()` | `--` | missing | -- |
| `IEEE754_TO_BLOB()` | `--` | missing | -- |
| `IFNULL()` | `IFNULL` | match | `fn-ifnull-fn`, `ifnull-null`, `ifnull-set` |
| `IIF()` | `--` | missing | `fn-iif` |
| `INSTR()` | `INSTR` | match | `fn-instr-hit`, `fn-instr-miss`, `fn-instr-null` |
| `JSON()` | `--` | missing | -- |
| `JSONB()` | `--` | missing | -- |
| `JSONB_ARRAY()` | `--` | missing | -- |
| `JSONB_EXTRACT()` | `--` | missing | -- |
| `JSONB_GROUP_ARRAY()` | `--` | missing | -- |
| `JSONB_GROUP_OBJECT()` | `--` | missing | -- |
| `JSONB_INSERT()` | `--` | missing | -- |
| `JSONB_OBJECT()` | `--` | missing | -- |
| `JSONB_PATCH()` | `--` | missing | -- |
| `JSONB_REMOVE()` | `--` | missing | -- |
| `JSONB_REPLACE()` | `--` | missing | -- |
| `JSONB_SET()` | `--` | missing | -- |
| `JSON_ARRAY()` | `--` | missing | -- |
| `JSON_ARRAY_LENGTH()` | `--` | missing | -- |
| `JSON_ERROR_POSITION()` | `--` | missing | -- |
| `JSON_EXTRACT()` | `--` | missing | -- |
| `JSON_GROUP_ARRAY()` | `--` | missing | -- |
| `JSON_GROUP_OBJECT()` | `--` | missing | -- |
| `JSON_INSERT()` | `--` | missing | -- |
| `JSON_OBJECT()` | `--` | missing | -- |
| `JSON_PATCH()` | `--` | missing | -- |
| `JSON_QUOTE()` | `--` | missing | -- |
| `JSON_REMOVE()` | `--` | missing | -- |
| `JSON_REPLACE()` | `--` | missing | -- |
| `JSON_SET()` | `--` | missing | -- |
| `JSON_TYPE()` | `--` | missing | -- |
| `JSON_VALID()` | `--` | missing | -- |
| `JULIANDAY()` | `--` | missing | `fn-julianday` |
| `LAG()` | `--` | missing | -- |
| `LAST_INSERT_ROWID()` | `--` | missing | `fn-last-insert-rowid` |
| `LAST_VALUE()` | `--` | missing | -- |
| `LEAD()` | `--` | missing | -- |
| `LENGTH()` | `LENGTH` | match | `fn-length-blob`, `fn-length-int`, `fn-length-null`, `fn-length-text` |
| `LIKE()` | `--` | missing | `like-func` |
| `LIKELIHOOD()` | `--` | missing | -- |
| `LIKELY()` | `--` | missing | `fn-likely` |
| `LN()` | `--` | missing | -- |
| `LOAD_EXTENSION()` | `--` | missing | -- |
| `LOG()` | `--` | missing | -- |
| `LOG10()` | `--` | missing | -- |
| `LOG2()` | `--` | missing | -- |
| `LOWER()` | `LOWER` | match | `fn-lower-basic`, `fn-lower-mixed` |
| `LSMODE()` | `--` | missing | -- |
| `LTRIM()` | `LTRIM` | match | `fn-ltrim-basic`, `fn-ltrim-chars` |
| `MATCH()` | `--` | missing | -- |
| `MATCHINFO()` | `--` | missing | -- |
| `MAX()` | `MAX` | missing | `agg-max`, `agg-max-text`, `fn-max-scalar` |
| `MIN()` | `MIN` | missing | `agg-min`, `agg-min-text`, `fn-min-scalar` |
| `MOD()` | `--` | missing | -- |
| `NTH_VALUE()` | `--` | missing | -- |
| `NTILE()` | `--` | missing | -- |
| `NULLIF()` | `NULLIF` | match | `fn-nullif-fn`, `nullif-diff`, `nullif-equal` |
| `OCTET_LENGTH()` | `--` | missing | -- |
| `OFFSETS()` | `--` | missing | -- |
| `OPTIMIZE()` | `--` | missing | -- |
| `PERCENT_RANK()` | `--` | missing | -- |
| `PI()` | `--` | missing | -- |
| `POW()` | `--` | missing | -- |
| `POWER()` | `--` | missing | -- |
| `PRINTF()` | `--` | missing | `fn-printf` |
| `QUOTE()` | `QUOTE` | match | `fn-quote-blob`, `fn-quote-int`, `fn-quote-null`, `fn-quote-text` |
| `RADIANS()` | `--` | missing | -- |
| `RANDOM()` | `--` | missing | -- |
| `RANDOMBLOB()` | `--` | missing | `fn-randomblob-len` |
| `RANK()` | `--` | missing | -- |
| `READFILE()` | `--` | missing | -- |
| `REGEXP()` | `--` | missing | -- |
| `REGEXPI()` | `--` | missing | -- |
| `REPLACE()` | `REPLACE` | match | `fn-replace-basic`, `fn-replace-empty` |
| `ROUND()` | `ROUND` | match | `fn-round-1`, `fn-round-2`, `fn-round-half`, `fn-round-int` |
| `ROW_NUMBER()` | `--` | missing | -- |
| `RTREECHECK()` | `--` | missing | -- |
| `RTREEDEPTH()` | `--` | missing | -- |
| `RTREENODE()` | `--` | missing | -- |
| `RTRIM()` | `RTRIM` | match | `fn-rtrim-basic`, `fn-rtrim-chars` |
| `SHA3()` | `--` | missing | -- |
| `SHA3_QUERY()` | `--` | missing | -- |
| `SHELL_ADD_SCHEMA()` | `--` | missing | -- |
| `SHELL_MODULE_SCHEMA()` | `--` | missing | -- |
| `SHELL_PUTSNL()` | `--` | missing | -- |
| `SIGN()` | `SIGN` | match | `fn-sign-neg`, `fn-sign-pos`, `fn-sign-text`, `fn-sign-zero` |
| `SIN()` | `--` | missing | -- |
| `SINH()` | `--` | missing | -- |
| `SNIPPET()` | `--` | missing | -- |
| `SQLAR_COMPRESS()` | `--` | missing | -- |
| `SQLAR_UNCOMPRESS()` | `--` | missing | -- |
| `SQLITE_COMPILEOPTION_GET()` | `--` | missing | -- |
| `SQLITE_COMPILEOPTION_USED()` | `--` | missing | -- |
| `SQLITE_LOG()` | `--` | missing | -- |
| `SQLITE_SOURCE_ID()` | `--` | missing | -- |
| `SQLITE_VERSION()` | `--` | missing | -- |
| `SQRT()` | `--` | missing | -- |
| `STRFTIME()` | `--` | missing | `fn-strftime` |
| `STRING_AGG()` | `--` | missing | -- |
| `STRTOD()` | `--` | missing | -- |
| `SUBSTR()` | `SUBSTR` | match | `fn-substr-2`, `fn-substr-3`, `fn-substr-neg`, `fn-substr-neg-len` |
| `SUBSTRING()` | `SUBSTRING` | match | `fn-substring-alias` |
| `SUBTYPE()` | `--` | missing | -- |
| `SUM()` | `SUM` | match | `agg-distinct-sum`, `agg-sum`, `agg-sum-empty`, `agg-sum-real` |
| `TAN()` | `--` | missing | -- |
| `TANH()` | `--` | missing | -- |
| `TIME()` | `--` | missing | `fn-time` |
| `TIMEDIFF()` | `--` | missing | -- |
| `TOTAL()` | `--` | missing | `agg-total` |
| `TOTAL_CHANGES()` | `--` | missing | `fn-total-changes` |
| `TRIM()` | `TRIM` | match | `fn-trim-basic`, `fn-trim-chars` |
| `TRUNC()` | `--` | missing | -- |
| `TYPEOF()` | `TYPEOF` | match | `fn-typeof-fn`, `typeof-blob`, `typeof-int`, `typeof-null` |
| `UNHEX()` | `--` | missing | -- |
| `UNICODE()` | `UNICODE` | match | `fn-unicode-a`, `fn-unicode-multi` |
| `UNIXEPOCH()` | `--` | missing | `fn-unixepoch` |
| `UNLIKELY()` | `--` | missing | `fn-unlikely` |
| `UPPER()` | `UPPER` | match | `fn-upper-arity`, `fn-upper-basic`, `fn-upper-mixed` |
| `USLEEP()` | `--` | missing | -- |
| `WRITEFILE()` | `--` | missing | -- |
| `ZEROBLOB()` | `--` | missing | `fn-zeroblob` |
| `ZIPFILE()` | `--` | missing | -- |
| `ZIPFILE_CDS()` | `--` | missing | -- |

**Functions:** 193 upstream, of which match 24, differs 0, missing 169, untested 0. Function parity over the 24 compared = **100.00%**.

**Extra (port-only) functions:** none

## Inventory 3 -- SQL keywords (statements, clauses, operators)

Derived mechanically from the oracle's own keyword table -- the `completion`
virtual table compiled into the `sqlite3` shell:

```sh
sqlite3 -batch -init /dev/null :memory: \
  "SELECT DISTINCT candidate FROM completion WHERE phase=1 ORDER BY 1;"   # 147 keywords
```

and from the port's own reserved-word table, read out of the installed module
(`$(go env GOMODCACHE)/github.com/malcolmston/sqlite@v0.2.0/token.go`, the
`keywords` map -- 54 words).

A keyword's status is folded from every case whose SQL contains it: `match` when
all such cases match, `differs` when some disagree, `missing` when the only
disagreements are the port rejecting the statement *and* the port's tokenizer
does not know the word, `untested` when no case uses it.

| upstream keyword | in port grammar | status | cases |
| --- | --- | --- | --- |
| `ABORT` | -- | missing | -- |
| `ACTION` | -- | missing | -- |
| `ADD` | -- | missing | 1 case |
| `AFTER` | -- | missing | -- |
| `ALL` | -- | missing | 2 cases |
| `ALTER` | -- | missing | 4 cases |
| `ALWAYS` | -- | missing | 1 case |
| `ANALYZE` | -- | missing | 1 case |
| `AND` | yes | missing | 5 cases |
| `AS` | yes | missing | 9 cases |
| `ASC` | yes | match | 1 case |
| `ATTACH` | -- | missing | 1 case |
| `AUTOINCREMENT` | -- | missing | 1 case |
| `BEFORE` | -- | missing | -- |
| `BEGIN` | yes | match | 4 cases |
| `BETWEEN` | -- | missing | 2 cases |
| `BY` | yes | missing | 19 cases |
| `CASCADE` | -- | missing | -- |
| `CASE` | -- | missing | 3 cases |
| `CAST` | -- | missing | 6 cases |
| `CHECK` | -- | missing | 2 cases |
| `COLLATE` | yes | missing | 3 cases |
| `COLUMN` | -- | missing | 3 cases |
| `COMMIT` | yes | match | 3 cases |
| `CONFLICT` | -- | missing | 1 case |
| `CONSTRAINT` | -- | missing | -- |
| `CREATE` | yes | missing | 10 cases |
| `CROSS` | -- | missing | 1 case |
| `CURRENT` | -- | missing | -- |
| `CURRENT_DATE` | -- | missing | -- |
| `CURRENT_TIME` | -- | missing | -- |
| `CURRENT_TIMESTAMP` | -- | missing | -- |
| `DATABASE` | -- | missing | 1 case |
| `DEFAULT` | -- | missing | 2 cases |
| `DEFERRABLE` | -- | missing | -- |
| `DEFERRED` | -- | missing | -- |
| `DELETE` | yes | match | 7 cases |
| `DESC` | yes | match | 2 cases |
| `DETACH` | -- | missing | -- |
| `DISTINCT` | yes | match | 5 cases |
| `DO` | -- | missing | 1 case |
| `DROP` | yes | missing | 5 cases |
| `EACH` | -- | missing | -- |
| `ELSE` | -- | missing | 2 cases |
| `END` | -- | missing | 1 case |
| `ESCAPE` | -- | missing | 1 case |
| `EXCEPT` | -- | missing | 1 case |
| `EXCLUDE` | -- | missing | -- |
| `EXCLUSIVE` | -- | missing | -- |
| `EXISTS` | yes | missing | 5 cases |
| `EXPLAIN` | -- | missing | 2 cases |
| `FAIL` | -- | missing | -- |
| `FILTER` | -- | missing | 1 case |
| `FIRST` | -- | missing | -- |
| `FOLLOWING` | -- | missing | -- |
| `FOR` | -- | missing | -- |
| `FOREIGN` | -- | missing | 1 case |
| `FROM` | yes | missing | 8 cases |
| `FULL` | -- | missing | 1 case |
| `GENERATED` | -- | missing | 1 case |
| `GLOB` | yes | differs | 9 cases |
| `GROUP` | yes | missing | 7 cases |
| `GROUPS` | -- | missing | -- |
| `HAVING` | yes | match | 3 cases |
| `IF` | yes | match | 2 cases |
| `IGNORE` | -- | missing | 1 case |
| `IMMEDIATE` | -- | missing | -- |
| `IN` | yes | missing | 10 cases |
| `INDEX` | -- | missing | 3 cases |
| `INDEXED` | -- | missing | -- |
| `INITIALLY` | -- | missing | -- |
| `INNER` | yes | match | 2 cases |
| `INSERT` | yes | missing | 17 cases |
| `INSTEAD` | -- | missing | -- |
| `INTERSECT` | -- | missing | 1 case |
| `INTO` | yes | missing | 2 cases |
| `IS` | yes | missing | 7 cases |
| `ISNULL` | -- | missing | -- |
| `JOIN` | yes | differs | 17 cases |
| `KEY` | yes | missing | 8 cases |
| `LAST` | -- | missing | 1 case |
| `LEFT` | -- | differs | 5 cases |
| `LIKE` | yes | missing | 15 cases |
| `LIMIT` | yes | match | 6 cases |
| `MATCH` | -- | missing | -- |
| `MATERIALIZED` | -- | missing | -- |
| `NATURAL` | -- | missing | 1 case |
| `NO` | -- | missing | -- |
| `NOT` | yes | missing | 14 cases |
| `NOTHING` | -- | missing | -- |
| `NOTNULL` | -- | missing | -- |
| `NULL` | yes | missing | 41 cases |
| `NULLS` | -- | missing | 1 case |
| `OF` | -- | missing | -- |
| `OFFSET` | yes | match | 2 cases |
| `ON` | yes | missing | 5 cases |
| `OR` | yes | missing | 6 cases |
| `ORDER` | yes | missing | 12 cases |
| `OTHERS` | -- | missing | -- |
| `OUTER` | -- | missing | 2 cases |
| `OVER` | -- | missing | 1 case |
| `PARTITION` | -- | missing | -- |
| `PLAN` | -- | missing | 1 case |
| `PRAGMA` | -- | missing | 3 cases |
| `PRECEDING` | -- | missing | -- |
| `PRIMARY` | yes | missing | 7 cases |
| `QUERY` | -- | missing | 1 case |
| `RAISE` | -- | missing | -- |
| `RANGE` | -- | missing | -- |
| `RECURSIVE` | -- | missing | 1 case |
| `REFERENCES` | -- | missing | 1 case |
| `REGEXP` | -- | missing | 1 case |
| `REINDEX` | -- | missing | -- |
| `RELEASE` | -- | missing | 1 case |
| `RENAME` | -- | missing | 2 cases |
| `REPLACE` | -- | missing | 2 cases |
| `RESTRICT` | -- | missing | -- |
| `RETURNING` | -- | missing | -- |
| `RIGHT` | -- | differs | 1 case |
| `ROLLBACK` | yes | missing | 6 cases |
| `ROW` | -- | missing | -- |
| `ROWS` | -- | missing | -- |
| `SAVEPOINT` | -- | missing | 2 cases |
| `SELECT` | yes | missing | 20 cases |
| `SET` | yes | match | 1 case |
| `TABLE` | yes | missing | 13 cases |
| `TEMP` | -- | missing | 1 case |
| `TEMPORARY` | -- | missing | -- |
| `THEN` | -- | missing | 1 case |
| `TIES` | -- | missing | -- |
| `TO` | -- | missing | 2 cases |
| `TRANSACTION` | yes | match | 1 case |
| `TRIGGER` | -- | missing | 1 case |
| `UNBOUNDED` | -- | missing | -- |
| `UNION` | -- | missing | 2 cases |
| `UNIQUE` | -- | missing | 3 cases |
| `UPDATE` | yes | missing | 10 cases |
| `USING` | -- | missing | 1 case |
| `VACUUM` | -- | missing | 1 case |
| `VALUES` | yes | missing | 4 cases |
| `VIEW` | -- | missing | 1 case |
| `VIRTUAL` | -- | missing | -- |
| `WHEN` | -- | missing | 2 cases |
| `WHERE` | yes | missing | 22 cases |
| `WINDOW` | -- | missing | -- |
| `WITH` | -- | missing | 2 cases |
| `WITHOUT` | -- | missing | 1 case |

**Keywords:** 147 upstream, of which match 13, differs 4, missing 130, untested 0. Keyword parity over the 17 compared = **76.47%**.

**Extra (port-only) keywords:** `AVG`, `BLOB`, `COUNT`, `FALSE`, `INT`, `INTEGER`, `MAX`, `MIN`, `REAL`, `SUM`, `TEXT`, `TRUE`

## Inventory 4 -- the Go API surface

The port also exposes a direct, non-`database/sql` API. It is not part of
SQLite's surface and is therefore `extra` by construction; only the
`database/sql` path is exercised by these cases.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sqlite3_open` (driver) | `sql.Open("mstsqlite", ...)` | match | all 428 | the anonymous `:memory:` database is private per connection, so the runner pins `SetMaxOpenConns(1)` |
| -- | `sqlite.NewDatabase` | extra | -- | direct store handle |
| -- | `sqlite.Database.Exec` / `.Query` | extra | -- | bypasses `database/sql` |
| -- | `sqlite.Parse` | extra | -- | exposes the AST |
| -- | `sqlite.ScalarNames` / `LookupScalar` / `CallScalar` | extra | -- | used above to derive the port's function inventory |

## Totals

| axis | denominator | match | differs | missing | untested | parity |
| --- | --- | --- | --- | --- | --- | --- |
| cases | 428 | 296 | 132 (10 silent wrong answer, 122 port-rejected) | -- | -- | **69.16%** |
| targeted SQL surface | 287 compared (of 409) | 282 | 5 | 122 | 0 | **98.26%** |
| built-in functions | 24 compared (of 193) | 24 | 0 | 169 | 0 | **100.00%** |
| SQL keywords | 17 compared (of 147) | 13 | 4 | 130 | 0 | **76.47%** |

**Headline parity: 296 / 428 cases = 69.16%.**

Read the symbol rows carefully. Following HARNESS.md, their parity column is
`match / (match + differs)` -- over the symbols *actually compared* -- which
excludes everything the port has no grammar for. Counting `missing` as a failure
instead gives the fuller picture:

| axis | match / (match + differs) | match / (match + differs + missing) |
| --- | --- | --- |
| targeted SQL surface | 98.26% (282/287) | 68.95% (282/409) |
| built-in functions | 100.00% (24/24) | 12.44% (24/193) |
| SQL keywords | 76.47% (13/17) | 8.84% (13/147) |


The shape of the gap matters more than the number. The port gets the core
relational engine right -- projection, `WHERE`, `ORDER BY`, `LIMIT`/`OFFSET`,
`DISTINCT`, a single un-aliased `INNER JOIN`, `GROUP BY`/`HAVING`, the five
aggregates, three-valued `NULL` logic, SQLite's dynamic typing including
INTEGER-vs-REAL division and `1/0 -> NULL`, column affinity, `?` parameters,
`LIKE`, and commit/rollback -- and then simply has no grammar for most of the
rest of SQLite. Of 193 upstream built-in functions it implements 27 (14.0%);
of 147 reserved keywords its tokenizer knows 42, and fewer than that are actually
usable in the grammar. Missing wholesale: subqueries of every kind, `UNION`/
`INTERSECT`/`EXCEPT`, CTEs, window functions, `CASE`, `BETWEEN`, `CAST`, the
bitwise operators, `!=`, `LEFT`/`RIGHT`/`FULL`/`CROSS`/`NATURAL`/`USING` joins,
more than two tables in a `FROM`, `INSERT ... SELECT`, `INSERT OR ...`/`REPLACE`/
upsert, every `ALTER TABLE` form, indexes, views, triggers, `WITHOUT ROWID`,
generated columns, the `UNIQUE`/`DEFAULT`/`AUTOINCREMENT`/`CHECK`/`COLLATE`/
`REFERENCES` column constraints, table-level `PRIMARY KEY`/`FOREIGN KEY`,
`PRAGMA`, `EXPLAIN`, `ANALYZE`, `VACUUM`, `ATTACH`, `SAVEPOINT`, the
`sqlite_master` catalogue, the implicit `rowid`, `GLOB` (accepted but wrong),
`group_concat`/`total`, all date and time functions, `printf`/`format`, `iif`,
scalar `min`/`max`, and quoted identifiers other than `"..."`.

## Notes on error messages

Comparison matches on *whether* a script failed, never on the message text. The
wording differs systematically: real SQLite reports
`Parse error near line N: no such column: x`, the port reports
`sqlite: no such column: x`; for unsupported grammar the port reports a
tokenizer-level complaint (`unexpected trailing token "BETWEEN"`,
`expected ")", got "CHECK"`) where SQLite would have executed the statement.

