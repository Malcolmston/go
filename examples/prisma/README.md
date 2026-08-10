# prisma example

A single runnable program that exercises
[`github.com/malcolmston/prisma`](https://github.com/malcolmston/prisma)
against a real, throwaway SQLite database created in an `os.MkdirTemp`
directory. **No database server and no network access are needed at run time.**

The SQLite driver is `modernc.org/sqlite` (pure Go, cgo-free), used purely as
the `database/sql` backend — the library itself is driver-agnostic.

## Resolved module version

The example consumes the published module, with no `replace` directive:

```
github.com/malcolmston/prisma v0.0.0-20260719012940-8e518830dc03
```

(`@latest` resolves to that pseudo-version; the repository has no semver tags,
even though `VERSION` in it says `0.2.0`.)

## Run

```sh
cd examples/prisma
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints eleven labeled sections and terminates on its own.

## What it demonstrates

1. **Model definition & introspection** — three models via `prisma:"…"` struct
   tags (`col=`, `pk`, `auto`, `-`, `relation`, `fk=`, `references=`), a
   `TableNamer` override, `Model.Table/Columns/PK/Relations/FieldByColumn`, a
   standalone `Registry` with `WithRegistry` + `WithDialect`, and the two model
   compilation errors (non-struct, `relation` without `fk=`).
2. **Query building / SQL generation without a database** — `FindManySQL`,
   `CountSQL`, `ExistsSQL`, `CountDistinctSQL`, `CreateSQL`, `CreateManySQL`
   (+`SkipDuplicates`), `UpdateSQL`, `DeleteSQL`, `UpsertSQL`,
   `Aggregate().SQL`, `GroupBy().SQL`, relation filters, JSON path filters,
   `WhereRaw`/`OrWhere`, `Include` joins, and the identical query rendered under
   `Question`, `Dollar` and `MySQLDialect` (placeholder style, `DISTINCT ON`,
   native `NULLS LAST`, `ON CONFLICT` vs `ON DUPLICATE KEY UPDATE`).
3. **Create** — `Create` (generated id written back), `CreateMany`, and
   `NestedCreate(...).Create/Connect/ConnectOrCreate/Update` executed in one
   transaction plus `Statements(parentID)` for dry-run inspection.
4. **Read** — `FindMany`, `FindFirst`, `FindUnique`, `Count`, `Exists`,
   `CountDistinct`, `Select`.
5. **Filtering** — every operator, run against real data: `Equals`, `Not`,
   `In`, `NotIn`, `Lt/Lte/Gt/Gte`, `Contains`, `StartsWith`, `EndsWith`, the
   `Not*` and case-insensitive `I*` variants, `IsNull`/`IsNotNull`,
   `Between`/`NotBetween`, `And`/`Or`/`NotGroup`, `RawCondition`, and the JSON
   filters (`JSONEquals`, `JSONNotEquals`, `JSONContains`, `JSONStartsWith`,
   `JSONIsNull`, `JSONIsNotNull`, `JSONPath`).
6. **Relations** — `Include("Author")` eager-loading a to-one relation through a
   `LEFT JOIN`, and `Some`/`Every`/`None`/`Is` compiling to correlated `EXISTS`
   subqueries, including composed inside `And`/`Or`.
7. **Update & Delete** — `Update`, `UpdateMany`, `Delete`, `DeleteMany`, and the
   arithmetic update operators `Increment`, `Decrement`, `Multiply`, `Divide`,
   `SetTo`.
8. **Aggregations** — `Aggregate().Count/Sum/Avg/Min/Max`, `GroupBy` with
   aggregates, and `Having(prisma.Gt("_count", 1))`.
9. **Distinct, cursor pagination, `Paginate`, `OrderByNulls`, `Clone`** — a
   full keyset-pagination loop over the table.
10. **Upsert, transactions and raw SQL** — `Upsert` (insert then conflict),
    `Client.Transaction` committing and rolling back, `Client.Batch`,
    `QueryRaw`, `QueryRawInto[T]`, `ExecuteRaw`.
11. **Typed errors** — P2002 unique violation, P2003 foreign-key violation,
    P2011 NOT NULL violation, `ErrNotFound` vs P2025 `ErrRecordNotFound`,
    `FindFirstOrThrow`/`FindUniqueOrThrow`, and the builder validation errors.

## Holes found

Nothing had to be commented out — the example compiles and runs every feature
listed above. The gaps are these:

- **No schema layer at all — this is the biggest gap versus Prisma.** There is
  no `schema.prisma` parser, no DSL, no DDL generation, no migrations and no
  code generation. The "schema" is only Go struct tags compiled by reflection at
  runtime, so the example has to hand-write `CREATE TABLE` and keep it manually
  in sync with the structs. Prisma's defining workflow (`schema.prisma` +
  `prisma generate` + `prisma migrate`) has no counterpart here. The README's
  claim of being a "type-safe ORM … inspired by Prisma" is accurate only for
  the query-builder half.
- **`Where` column names are not validated.** `Select`, `Update`, `Upsert` and
  the aggregate helpers all check the column against the model, but conditions
  do not: `Where(prisma.Equals("nope", 1))` builds
  `… WHERE nope = ?` and only fails at the database. Same for `OrderBy`,
  `Cursor` and `Distinct`. Typos become runtime SQL errors rather than
  build-time Go errors, which undercuts the "type-safe" claim — column names are
  untyped strings throughout, with no generated constants.
- **`Include` cannot load to-many relations.** It only emits a to-one
  `LEFT JOIN`, and calling `Include("Posts")` for a slice relation produces
  invalid SQL (`no such column: users.author_id`) instead of an error. There is
  no second-query loader or row-grouping, so `User.Posts` is always `nil` after
  any read — the relation exists purely to power `Some`/`Every`/`None`.
- **`Distinct(columns…)` is unsound when combined with `Take`.** On the
  `Question`/`MySQL` dialects it emits a bare `SELECT DISTINCT` over *all*
  selected columns and then de-duplicates by the requested column in Go, after
  the SQL `LIMIT` has already been applied. The example shows
  `Distinct("city").Take(3)` returning 2 rows. Only `Dollar` (`DISTINCT ON`)
  gets this right.
- **`ErrNotFound` and P2025 are two different not-found errors.** `FindFirst`
  and `FindUnique` return the plain sentinel `ErrNotFound`, which does *not*
  match `ErrRecordNotFound`/`CodeRecordNotFound`, while
  `FindFirstOrThrow`/`FindUniqueOrThrow` return the typed P2025 `*Error`. A
  caller branching on `prisma.ErrRecordNotFound` silently misses the ordinary
  find methods.
- **Error-code mapping is substring matching on driver messages**
  (`mapError` greps for `"unique constraint"`, `"duplicate entry"`, …). It works
  on SQLite/PostgreSQL/MySQL English messages but is locale- and driver-fragile,
  and it cannot populate the fields real Prisma errors carry (`meta.target`,
  i.e. *which* constraint or column failed).
- **`GroupQuery` has no `OrderBy`** even though its `SQL()` renders the base
  query's `ORDER BY`; ordering must be set on the `Query` *before* `GroupBy`,
  which reads backwards. `GroupQuery` also has no `Take`/`Skip`.
- **`Update`/`Delete` silently ignore `Take`, `Skip` and `OrderBy`.** There is
  no `LIMIT` on the generated `UPDATE`/`DELETE`, so
  `…Take(1).Delete(ctx)` deletes every matching row. `Delete`/`DeleteMany` and
  `Update`/`UpdateMany` are documented aliases of each other, which means the
  singular-sounding methods offer no safety at all — a `Delete` with a
  forgotten `Where` clears the whole table.
- **`Aggregate` results are `float64`-only.** `AggregateResult.Sum/Avg/Min/Max`
  are `map[string]float64`, so `Min`/`Max` over a string, date or big-integer
  column cannot be represented, and integer sums lose exactness past 2^53.
- **`NewQuery[T]` auto-registers unknown types**, so a query against a struct
  you forgot to `Register` silently succeeds with a reflection-derived table
  name instead of reporting the mistake.
- **`FindUnique` is just an alias for `FindFirst`** (documented, but it means
  there is no unique-constraint awareness and no "more than one row" detection).
- Minor: the README's quick start uses `sql.Open("sqlite3", …)`, implying
  `mattn/go-sqlite3`, while advertising "no third-party dependencies, no cgo" —
  that driver requires cgo. Any real use needs a third-party driver regardless.
- Minor: `Query` is mutable and chaining mutates the receiver
  (`FindFirst` even overwrites `take`), so a shared base query must be `Clone`d
  before reuse; this is easy to get wrong and is not called out in the README.
