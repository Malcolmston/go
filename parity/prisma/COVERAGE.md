# `prisma` parity coverage

- **Upstream oracle**: npm `prisma` **5.22.0** and `@prisma/client` **5.22.0**,
  pinned exactly in [`node/package.json`](node/package.json) (`package-lock.json`
  in the same directory pins the full graph). Datasource provider **sqlite**, so
  the suite needs nothing external. Node v24.18.0.
- **Go port**: `github.com/malcolmston/prisma` **v0.3.0**, consumed as a
  published module (no `replace` directive), with `modernc.org/sqlite` v1.56.0 as
  the pure-Go `database/sql` driver.
- **Harness**: `cd parity/prisma && GOWORK=off go test ./` — **112 cases** in
  `cases/*.json`, each a *script* of steps against one SQLite database.

## What this harness can and cannot compare

**The Go port has no schema layer at all.** There is no `schema.prisma` parser,
no migration engine, no code generator and no introspection: a model is a Go
struct with `prisma:"col=...,pk"` tags, compiled by reflection at run time. The
entire schema/codegen/migrate surface — 19 CLI entry points plus
`$applyPendingMigrations` — is therefore inventoried below as **missing**. That
is the single largest finding in this document, and it is a property of the port,
not a gap in the harness.

What *is* comparable is **query semantics**, so that is what is compared:

1. The same two logical models are declared once as a Prisma datamodel
   ([`node/schema.prisma`](node/schema.prisma)) and once as tagged Go structs
   ([`go/run.go`](go/run.go)). Prisma field names are deliberately snake_case, so
   a Prisma field name, the SQL column name and the `col=` tag are always the
   same string and no difference can be an artefact of a naming convention.
2. **The DDL is written by upstream, once.** `parity_test.go` runs
   `prisma generate` and `prisma db push` into `node/.parity/template.db`; both
   runners then *copy that file*. Neither side hand-writes a `CREATE TABLE`, so
   the two execute against byte-identical tables, the `users_email_key` unique
   index and the `posts_author_id_fkey` foreign key. Foreign keys are enforced on
   both sides (Prisma's SQLite connector sets `PRAGMA foreign_keys=ON`; the Go
   runner opens with `_pragma=foreign_keys(1)`).
3. Every case **resets** (truncates both tables and clears `sqlite_sequence`, so
   ids restart at 1 no matter which cases ran before), **seeds** the same fixed
   8 users / 12 posts with explicit consecutive ids, runs one query, and emits
   rows as plain JSON objects keyed by column name. Writes always end with a
   `dump`, so what is scored is the effect on the data and not merely a return
   value. **No SQL text is ever compared.**
4. A step that fails is emitted as `{"ok":false,"code":"P2002"}` and the script
   continues. Only the **code** travels; messages go to stderr. Where one side
   classifies an error and the other does not, the code is `""` on that side and
   the difference is visible in the score rather than hidden.

Two smaller consequences of the port's design are absorbed by the runner rather
than smeared across every case, and are recorded as `differs` in the table
instead:

- `Select` narrows the SQL projection but still returns a fully-typed struct, so
  unselected fields come back as zero values. The Go runner re-applies the
  projection before emitting.
- `GroupQuery.Exec` returns `[]map[string]any` of driver-native values keyed
  `_sum_views`, with no type information, so a `BOOLEAN` group key arrives as
  `0`/`1`. The Go runner renames the keys onto Prisma's nested shape and restores
  the bool. No numeric value is touched.

The seed dataset is hard-coded identically in both runners rather than repeated
in 112 case files; `seed-dump` reads both tables straight back, so a divergence
in the seed itself would be scored like any other case.

**There are no `deviation` cases.** `parity/HARNESS.md` requires a deviation to
be listed in the library's `API-DEVIATIONS.md`, and v0.3.0 ships no such file, so
every difference below is counted as a mismatch.

## How the upstream symbol list was produced

Mechanically, from the **installed, generated** client — never from the docs.
[`node/inventory.js`](node/inventory.js) does three things:

```sh
cd parity/prisma/node && node inventory.js      # 219 raw symbols
```

1. **Reflection over the live client.** `Object.getOwnPropertyNames` walked up
   the prototype chain of `new PrismaClient()` and of `prisma.user`, keeping the
   function-valued keys. That is where the 13 public `$`-methods and the 18 model
   delegate methods come from. (`$executeRawInternal` and `$queryRawInternal` are
   dropped as internals.)
2. **The generated `node_modules/.prisma/client/index.d.ts`**, brace-matched to
   extract the top-level keys of each argument type: `UserWhereInput`,
   `StringFilter`, `StringNullableFilter`, `IntFilter`, `IntNullableFilter`,
   `FloatFilter`, `BoolFilter`, `PostListRelationFilter`, `UserRelationFilter`,
   `UserOrderByWithRelationInput`, `SortOrderInput`, `UserFindManyArgs`,
   `UserAggregateArgs`, `UserGroupByArgs`, `UserScalarWhereWithAggregatesInput`,
   `IntWithAggregatesFilter`, `UserUpdateInput`,
   `IntFieldUpdateOperationsInput`, `UserUpsertArgs`, `UserCreateInput`,
   `PostCreateNestedManyWithoutAuthorInput`,
   `PostUpdateManyWithoutAuthorNestedInput`, `UserCountAggregateOutputType`.
3. **`prisma --help`, `prisma db --help`, `prisma migrate --help`**, parsed for
   their command tables — the schema/migration surface, which lives in the CLI
   rather than the client.

Of the 219 raw symbols, **38 are model field names** (`where: city`,
`orderBy: age`, `create data: email`, …). Those are generated from *this*
datamodel rather than being part of the API vocabulary, so they are excluded;
the port addresses columns by string, which the `where-unknown-column`,
`orderby-unknown-column`, `select-unknown-column` and `distinct-unknown-column`
cases cover instead. **The denominator is the remaining 179 symbols.**

The Go side was enumerated with

```sh
GOWORK=off go doc -all github.com/malcolmston/prisma | grep -E '^(func|type) '   # 158
```

## Status table

`#` is how many upstream symbols the row accounts for; the column sums to 179
for the scored rows. `extra` rows have no upstream symbol and are **outside** the
denominator.

| # | upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- | --- |
| 1 | `PrismaClient#$connect` | — | missing | — | the port takes an already-open `*sql.DB`; there is nothing to connect |
| 1 | `PrismaClient#$disconnect` | — | missing | — | likewise; the caller owns the pool |
| 1 | `PrismaClient#$transaction` | `Client.Transaction` / `Client.Batch` | untested | — | present in both; not scored because a rollback race is not deterministic across processes |
| 1 | `PrismaClient#$executeRawUnsafe` | `Client.ExecuteRaw` | match | every case (`reset` step) | both runners truncate the tables through it, so every case exercises it |
| 1 | `PrismaClient#$executeRaw` | `Client.ExecuteRaw` | untested | — | the tagged-template form has no Go analogue; the port only takes a string |
| 2 | `PrismaClient#$queryRaw` / `PrismaClient#$queryRawUnsafe` | `Client.QueryRaw` | untested | — |  |
| 1 | `PrismaClient#$queryRawTyped` | `QueryRawInto[T]` | untested | — |  |
| 1 | `PrismaClient#$runCommandRaw` | — | missing | — | MongoDB-only upstream |
| 1 | `PrismaClient#$extends` | — | missing | — | no client extensions |
| 1 | `PrismaClient#$use` | — | missing | — | no middleware pipeline |
| 1 | `PrismaClient#$on` | — | missing | — | no query/error event hooks |
| 1 | `PrismaClient#$applyPendingMigrations` | — | missing | — | no migration engine |
| 1 | `<model>.findMany` | `Query[T].FindMany` | differs | many; see `include-to-many`, `distinct-take-projecting-id`, `skip-without-take` | the workhorse; the divergences below are all reached through it |
| 1 | `<model>.findFirst` | `Query[T].FindFirst` | differs | `find-first-missing` | upstream returns null for no match, the port returns `ErrNotFound` |
| 1 | `<model>.findUnique` | `Query[T].FindUnique` | differs | `find-unique-missing` | an alias of FindFirst: uniqueness is the caller's problem, and no match is an error |
| 1 | `<model>.findFirstOrThrow` | `Query[T].FindFirstOrThrow` | match | `find-first-or-throw-missing` | both raise P2025 |
| 1 | `<model>.findUniqueOrThrow` | `Query[T].FindUniqueOrThrow` | match | `find-unique-or-throw-missing` | both raise P2025 |
| 1 | `<model>.create` | `Query[T].Create` | differs | `create-user`, `create-null-optionals`, `create-post-with-fk`, `create-explicit-id` | agrees except that an explicit integer primary key is silently dropped |
| 1 | `<model>.createMany` | `Query[T].CreateMany` | match | `create-many`, `seed-dump`, `unique-violation-create-many` |  |
| 1 | `<model>.createManyAndReturn` | — | missing | — |  |
| 1 | `<model>.update` | `Query[T].Update` | differs | `update-one`, `update-set-null`, `update-missing-row` | returns a row count, not the record, and no match is 0 rather than P2025 |
| 1 | `<model>.updateMany` | `Query[T].UpdateMany` | differs | `update-increment`, `update-many-multi-column`, `update-many-with-take` | an alias of Update; take/skip/orderBy are accepted and ignored |
| 1 | `<model>.upsert` | `Query[T].Upsert` | differs | `upsert-updates-existing`, `upsert-inserts-new`, `upsert-where-not-matched-by-create`, `upsert-on-primary-key`, `upsert-atomic-operator` | a raw ON CONFLICT: the where is only a source of conflict column names |
| 1 | `<model>.delete` | `Query[T].Delete` | differs | `delete-one`, `delete-missing-row`, `delete-non-unique-where` | an alias of DeleteMany, so the singular form deletes every match |
| 1 | `<model>.deleteMany` | `Query[T].DeleteMany` | match | `delete-many`, `delete-many-all` |  |
| 1 | `<model>.count` | `Query[T].Count` | differs | `count-where`, `count-all`, `count-with-take` | agrees unless take/skip is set, which the port folds into the counted set |
| 1 | `<model>.aggregate` | `Query[T].Aggregate` | differs | `aggregate-all-functions`, `aggregate-mixed-columns`, `aggregate-empty-set`, `aggregate-string-min-max` | float64-only results, and NULL aggregates vanish |
| 1 | `<model>.groupBy` | `Query[T].GroupBy` | differs | `groupby-one-key`, `groupby-two-keys`, `groupby-having-sum`, `groupby-having-count`, `groupby-having-avg`, `groupby-string-min-max`, `groupby-where-then-having` | values agree; the port returns `[]map[string]any` of driver-native values keyed `_sum_views` rather than a typed `{ _sum: { views } }` |
| 2 | `<model>.findRaw` / `<model>.aggregateRaw` | — | missing | — | MongoDB-only upstream |
| 1 | `where: AND` | `prisma.And` | match | `and-or-nesting`, `deep-and-or-not` |  |
| 1 | `where: OR` | `prisma.Or` | match | `and-or-nesting`, `not-or-nesting` |  |
| 1 | `where: NOT` | `prisma.NotGroup` | match | `not-group`, `not-or-nesting`, `deep-and-or-not` |  |
| 6 | `where.<String>: equals` / `where.<String?>: equals` / `where.<Int>: equals` / `where.<Int?>: equals` / `where.<Float>: equals` / `where.<Boolean>: equals` | `prisma.Equals` / `prisma.IsNull` | differs | `equals-scalar`, `equals-explicit`, `equals-null`, `equals-null-literal` | `equals: null` has no analogue: `Equals(col, nil)` renders `col = ?` and matches nothing, so IsNull has to be chosen by hand |
| 6 | `where.<String>: not` / `where.<String?>: not` / `where.<Int>: not` / `where.<Int?>: not` / `where.<Float>: not` / `where.<Boolean>: not` | `prisma.Not` / `prisma.IsNotNull` | differs | `not-scalar`, `not-nested-operators`, `not-null`, `not-null-literal` | same story for `not: null` |
| 5 | `where.<String>: in` / `where.<String?>: in` / `where.<Int>: in` / `where.<Int?>: in` / `where.<Float>: in` | `prisma.In` | differs | `in-list`, `in-empty`, `in-with-null` | upstream rejects a null inside the list; the port binds it and matches nothing |
| 5 | `where.<String>: notIn` / `where.<String?>: notIn` / `where.<Int>: notIn` / `where.<Int?>: notIn` / `where.<Float>: notIn` | `prisma.NotIn` | match | `not-in-list`, `not-in-empty` |  |
| 5 | `where.<String>: lt` / `where.<String?>: lt` / `where.<Int>: lt` / `where.<Int?>: lt` / `where.<Float>: lt` | `prisma.Lt` | match | `range-gte-lt` |  |
| 5 | `where.<String>: lte` / `where.<String?>: lte` / `where.<Int>: lte` / `where.<Int?>: lte` / `where.<Float>: lte` | `prisma.Lte` | match | `range-gt-lte`, `deep-and-or-not` |  |
| 5 | `where.<String>: gt` / `where.<String?>: gt` / `where.<Int>: gt` / `where.<Int?>: gt` / `where.<Float>: gt` | `prisma.Gt` | match | `range-gt-lte`, `some-with-filter` |  |
| 5 | `where.<String>: gte` / `where.<String?>: gte` / `where.<Int>: gte` / `where.<Int?>: gte` / `where.<Float>: gte` | `prisma.Gte` | match | `range-gte-lt`, `is-to-one-nested-range` |  |
| 2 | `where.<String>: contains` / `where.<String?>: contains` | `prisma.Contains` | differs | `contains`, `contains-percent`, `contains-underscore` | the port escapes LIKE wildcards with a backslash but emits no `ESCAPE` clause, so SQLite matches the backslash literally |
| 2 | `where.<String>: startsWith` / `where.<String?>: startsWith` | `prisma.StartsWith` | match | `starts-and-ends-with` | the same unescaped-backslash issue applies in principle; only the plain form is scored |
| 2 | `where.<String>: endsWith` / `where.<String?>: endsWith` | `prisma.EndsWith` | match | `starts-and-ends-with` |  |
| 1 | `where.<to-many>: some` | `prisma.Some` | match | `some-with-filter`, `some-nested-and`, `some-any` |  |
| 1 | `where.<to-many>: every` | `prisma.Every` | match | `every-with-filter`, `every-any` |  |
| 1 | `where.<to-many>: none` | `prisma.None` | match | `none-with-filter`, `none-any` |  |
| 1 | `where.<to-one>: is` | `prisma.Is` | match | `is-to-one`, `is-to-one-nested-range` |  |
| 1 | `where.<to-one>: isNot` | — | missing | — | `NotGroup(Is(...))` composes to the same predicate, but there is no symbol |
| 1 | `findMany args: select` | `Query.Select` | differs | `select-projection`, `select-unknown-column` | the port narrows the SELECT list but still returns a full struct, so unselected fields come back as zero values rather than being absent |
| 1 | `findMany args: include` | `Query.Include` | differs | `include-to-one`, `include-to-one-ambiguous-filter`, `include-to-many`, `include-to-many-all` | to-one works; to-many emits invalid SQL, and a filter on a column both tables have is ambiguous |
| 1 | `findMany args: where` | `Query.Where` | differs | `where-unknown-column` | column names are unvalidated strings, so a typo becomes a database error rather than a build-time one |
| 1 | `findMany args: orderBy` | `Query.OrderBy` | differs | `orderby-asc-default-nulls`, `orderby-desc`, `orderby-multi-column`, `orderby-unknown-column` | values agree; the column name is again unvalidated |
| 1 | `orderBy.<nullable>: sort` | `prisma.Asc` / `prisma.Desc` | match | `orderby-desc` |  |
| 1 | `orderBy.<nullable>: nulls` | `Query.OrderByNulls` | match | `orderby-nulls-first`, `orderby-nulls-last`, `orderby-nulls-desc-last` | the port emulates placement with a leading `(col IS NULL)` term and agrees with upstream |
| 1 | `findMany args: take` | `Query.Take` | differs | `take-and-skip`, `take-negative` | a negative take is upstream's "last n"; the port passes it to LIMIT, where it means unlimited |
| 1 | `findMany args: skip` | `Query.Skip` | differs | `take-and-skip`, `skip-without-take` | the port only emits LIMIT when Take was called, and SQLite has no bare OFFSET |
| 1 | `findMany args: cursor` | `Query.Cursor` | differs | `cursor-forward`, `cursor-skip-one`, `cursor-descending` | the port's cursor is EXCLUSIVE; upstream includes the cursor row |
| 1 | `findMany args: distinct` | `Query.Distinct` | differs | `distinct-single-column`, `distinct-two-columns`, `distinct-descending-order`, `distinct-take-projecting-id`, `distinct-take-key-only`, `distinct-unknown-column` | three separate faults: LIMIT before the Go-side dedupe, pointer-valued columns keyed by address, and an unresolvable key silently ignored |
| 1 | `aggregate args: where` | `Query.Where` before `Aggregate` | match | `aggregate-empty-set` |  |
| 1 | `aggregate args: _count` | `AggregateQuery.Count` | match | `aggregate-all-functions`, `aggregate-mixed-columns` |  |
| 1 | `aggregate args: _sum` | `AggregateQuery.Sum` | differs | `aggregate-all-functions`, `aggregate-empty-set` | a NULL sum (no rows) is omitted from the result instead of reported as null |
| 1 | `aggregate args: _avg` | `AggregateQuery.Avg` | differs | `aggregate-all-functions`, `aggregate-mixed-columns`, `aggregate-empty-set` | same |
| 1 | `aggregate args: _min` | `AggregateQuery.Min` | differs | `aggregate-all-functions`, `aggregate-string-min-max` | AggregateResult is `map[string]float64`, so MIN over text or dates cannot be represented |
| 1 | `aggregate args: _max` | `AggregateQuery.Max` | differs | `aggregate-all-functions`, `aggregate-string-min-max` | same |
| 4 | `aggregate args: orderBy` / `aggregate args: take` / `aggregate args: skip` / `aggregate args: cursor` | — | missing | — | AggregateQuery.SQL emits no ORDER BY, LIMIT, OFFSET or keyset predicate |
| 1 | `aggregate _count output: _all` | `AggregateResult.Count` | match | `aggregate-all-functions` | the port only offers COUNT(*), which is upstream's `_all` |
| 1 | `groupBy args: by` | `Query.GroupBy` | match | `groupby-one-key`, `groupby-two-keys` |  |
| 1 | `groupBy args: where` | `Query.Where` before `GroupBy` | match | `groupby-where-then-having` |  |
| 1 | `groupBy args: orderBy` | `Query.OrderBy` before `GroupBy` | match | `groupby-one-key`, `groupby-two-keys` |  |
| 1 | `groupBy args: having` | `GroupQuery.Having` | match | `groupby-having-sum`, `groupby-having-count`, `groupby-having-avg` | the port filters on its own aggregate aliases (`_sum_views`), which SQLite resolves in HAVING |
| 1 | `groupBy args: _count` | `GroupQuery.Count` | match | `groupby-one-key`, `groupby-two-keys` |  |
| 1 | `groupBy args: _sum` | `GroupQuery.Sum` | match | `groupby-one-key`, `groupby-having-sum` |  |
| 1 | `groupBy args: _avg` | `GroupQuery.Avg` | match | `groupby-having-avg` |  |
| 1 | `groupBy args: _min` | `GroupQuery.Min` | match | `groupby-string-min-max` | unlike Aggregate this scans into `any`, so text MIN/MAX survives |
| 1 | `groupBy args: _max` | `GroupQuery.Max` | match | `groupby-string-min-max` |  |
| 2 | `groupBy args: take` / `groupBy args: skip` | — | missing | — | GroupQuery.SQL emits no LIMIT/OFFSET |
| 3 | `having: AND` / `having: OR` / `having: NOT` | `prisma.And` / `Or` / `NotGroup` in `Having` | untested | — | the conditions compose, but no case combines two HAVING predicates |
| 1 | `having.<Int>: _count` | `prisma.Gt("_count", ...)` | match | `groupby-having-count` |  |
| 1 | `having.<Int>: _sum` | `prisma.Gt("_sum_<col>", ...)` | match | `groupby-having-sum`, `groupby-where-then-having` |  |
| 1 | `having.<Int>: _avg` | `prisma.Gt("_avg_<col>", ...)` | match | `groupby-having-avg` |  |
| 2 | `having.<Int>: _min` / `having.<Int>: _max` | `prisma.Gt("_min_<col>", ...)` | untested | — |  |
| 1 | `having.<Int>: gt` | `prisma.Gt` on an alias | match | `groupby-having-sum`, `groupby-having-count`, `groupby-having-avg` |  |
| 1 | `having.<Int>: gte` | `prisma.Gte` on an alias | match | `groupby-where-then-having` |  |
| 6 | `having.<Int>: equals` / `having.<Int>: in` / `having.<Int>: notIn` / `having.<Int>: lt` / `having.<Int>: lte` / `having.<Int>: not` | `prisma.Equals` / `In` / ... on an alias | untested | — | the same alias trick applies; only gt/gte are scored |
| 1 | `update data.<Int>: set` | `prisma.SetTo` | match | `update-set-operator` |  |
| 1 | `update data.<Int>: increment` | `prisma.Increment` | match | `update-increment`, `update-many-multi-column` |  |
| 1 | `update data.<Int>: decrement` | `prisma.Decrement` | match | `update-decrement` |  |
| 1 | `update data.<Int>: multiply` | `prisma.Multiply` | match | `update-multiply` |  |
| 1 | `update data.<Int>: divide` | `prisma.Divide` | match | `update-divide-exact`, `update-divide-fractional` | including the fractional case, where both store the SQL division result |
| 1 | `upsert args: where` | the conflict-column argument of `Query.Upsert` | differs | `upsert-where-not-matched-by-create`, `upsert-on-primary-key` | only the KEY NAMES are used; the values are dropped, so the port updates whatever the INSERT collides with rather than the row the where selected |
| 1 | `upsert args: create` | the `create` argument of `Query.Upsert` | match | `upsert-inserts-new` |  |
| 1 | `upsert args: update` | the `update` argument of `Query.Upsert` | differs | `upsert-updates-existing`, `upsert-atomic-operator` | UpsertSQL binds each update value directly instead of letting an UpdateOp render itself, so `{ increment }` reaches the driver as a struct and the call fails |
| 2 | `upsert args: select` / `upsert args: include` | — | missing | — | Upsert returns a row count, so there is nothing to project |
| 1 | `create data.<to-many>: create` | `NestedCreate.Create` | match | `nested-create` |  |
| 1 | `create data.<to-many>: connect` | `NestedCreate.Connect` | untested | — |  |
| 1 | `create data.<to-many>: connectOrCreate` | `NestedCreate.ConnectOrCreate` | untested | — |  |
| 1 | `create data.<to-many>: createMany` | — | missing | — |  |
| 11 | `update data.<to-many>: create` / `update data.<to-many>: connect` / `update data.<to-many>: connectOrCreate` / `update data.<to-many>: update` / `update data.<to-many>: updateMany` / `update data.<to-many>: upsert` / `update data.<to-many>: createMany` / `update data.<to-many>: set` / `update data.<to-many>: disconnect` / `update data.<to-many>: delete` / `update data.<to-many>: deleteMany` | — | missing | — | nested writes exist only on the create path (`NestedCreate`); `Update` takes a flat column map, so none of the eleven nested-update operations has an analogue |
| 6 | `prisma CLI: init` / `prisma CLI: validate` / `prisma CLI: format` / `prisma CLI: version` / `prisma CLI: debug` / `prisma CLI: studio` | — | missing | — | no schema file exists to init, validate, format or browse |
| 1 | `prisma CLI: generate` | — | missing | — | there is no code generation: a model is a Go struct read by reflection at run time |
| 5 | `prisma CLI: db` / `prisma CLI: db push` / `prisma CLI: db pull` / `prisma CLI: db seed` / `prisma CLI: db execute` | — | missing | — | no schema push, no introspection, no seeding entry point; the harness has to create the tables with upstream and hand the port the resulting file |
| 7 | `prisma CLI: migrate` / `prisma CLI: migrate dev` / `prisma CLI: migrate deploy` / `prisma CLI: migrate reset` / `prisma CLI: migrate status` / `prisma CLI: migrate resolve` / `prisma CLI: migrate diff` | — | missing | — | no migration engine, no migration history, no drift detection |
| 12 | — | `FindManySQL`, `CountSQL`, `CreateSQL`, `CreateManySQL`, `UpdateSQL`, `DeleteSQL`, `UpsertSQL`, `ExistsSQL`, `CountDistinctSQL`, `AggregateQuery.SQL`, `GroupQuery.SQL`, `NestedCreate.Statements` | extra | — | every terminal operation has a dry-run twin returning the statement and args; upstream has no such API (only the `query` event / `--debug` log) |
| 11 | — | `Registry`, `NewRegistry`, `Registry.Register`, `Registry.ModelOf`, `Model`, `Model.Columns`, `Model.FieldByColumn`, `Model.RelationByName`, `Field`, `Relation`, `TableNamer` | extra | — | THE replacement for schema.prisma: a model is compiled from struct tags by reflection |
| 7 | — | `Dialect`, `Question`, `Dollar`, `MySQLDialect`, `UpsertStyle`, `OnConflict`, `OnDuplicateKey` | extra | — | the datasource provider is a runtime value here, not a schema block |
| 14 | — | `JSONPath`, `JSONEquals`, `JSONNotEquals`, `JSONGt`, `JSONGte`, `JSONLt`, `JSONLte`, `JSONContains`, `JSONNotContains`, `JSONStartsWith`, `JSONEndsWith`, `JSONIsNull`, `JSONIsNotNull`, `Dialect.JSONExtract` | extra | — | JSON path filters; this datamodel has no Json column (SQLite has no Json scalar in Prisma 5), so there is nothing to compare them against |
| 4 | — | `IEquals`, `IContains`, `IStartsWith`, `IEndsWith` | extra | — | case-insensitive matching; upstream's `mode: "insensitive"` does not exist in the SQLite client, so these have no counterpart to be scored against |
| 5 | — | `Between`, `NotBetween`, `NotContains`, `NotStartsWith`, `NotEndsWith` | extra | — | sugar for predicates upstream spells with `AND`/`not` |
| 3 | — | `WhereRaw`, `OrWhere`, `RawCondition` | extra | — | raw SQL fragments spliced into WHERE; upstream has only whole-query `$queryRaw` |
| 4 | — | `Exists`, `CountDistinct`, `Paginate`, `Clone` | extra | — | convenience terminals and a page-number pager with no upstream analogue |
| 2 | — | `SkipDuplicates`, `NestedCreate.Update` | extra | — | `skipDuplicates` is not offered by upstream on SQLite; `NestedCreate.Update` updates children during a parent insert |
| 7 | — | `ErrNotFound`, `Error`, `ErrUniqueViolation`, `ErrForeignKeyViolation`, `ErrNotNullViolation`, `ErrValueTooLong`, `ErrRecordNotFound` | extra | — | `ErrNotFound` is the port's own second not-found error, distinct from the P2025-carrying `ErrRecordNotFound`, and the two do not match with errors.Is |

## Score

| status | symbols |
| --- | --- |
| match | 67 |
| differs | 44 |
| missing | 50 |
| untested | 18 |
| **denominator** | **179** |
| extra (Go-only, outside the denominator) | 69 |

- **Symbol parity: 67/111 = 60.4 %** over the symbols actually compared
  (`match / (match + differs)`). Counting `missing` and `untested` against the
  port instead gives 67/179 = 37.4 %.
- **Case parity: 81/112 = 72.32 %**, 0 deviations — see
  [`parity.json`](parity.json), which `go test` rewrites.

Per group (from `parity.json`):

| group | cases | match | mismatch |
| --- | --- | --- | --- |
| basics | 10 | 10 | 0 |
| filters | 24 | 19 | 5 |
| ordering | 19 | 11 | 8 |
| relations | 13 | 10 | 3 |
| aggregates | 14 | 11 | 3 |
| writes | 22 | 14 | 8 |
| errors | 10 | 6 | 4 |

## The divergences, worst first

Wrong rows and silently-dropped data come first; error-shape differences last.

### 1. `include` cannot load a to-many relation (`include-to-many`, `include-to-many-all`)

`Query.Include` always joins `base.<ForeignKey> = target.<References>`. For a
to-many relation the foreign key is on the *target* table, so `User.Posts`
(`fk=author_id`) compiles to `LEFT JOIN posts ON users.author_id = posts.id` —
naming a column `users` does not have. SQLite rejects the statement, so a parent
and its children can never be loaded together.

| | upstream | Go port |
| --- | --- | --- |
| `user.findMany({ where: { id: 1 }, include: { posts: true } })` | Ada with her 2 posts | error: `no such column: users.author_id` |

Worth noting what this is *not*: it is not a silent empty list, it is an
unrunnable query. The relation *filters* (`Some`/`Every`/`None`) get the same
correlation right, so only loading is affected.

### 2. `include` makes any filter on a shared column ambiguous (`include-to-one-ambiguous-filter`)

The to-one join works, but `whereClause` and the cursor predicate never qualify
their columns with the base table while `orderClause` does. `posts.findMany({
where: { id: { in: [1,3] } }, include: { author: true } })` therefore emits
`WHERE id IN (?, ?)` over a join of two tables that both have `id`:
`ambiguous column name: id`. `include-to-one` (filtering on `title`) passes, so
the to-one path is usable only as long as no filtered column exists on both
sides.

### 3. `distinct` is wrong three separate ways

- **`distinct` + `take` is unsound** (`distinct-take-projecting-id`). `Take`
  becomes a SQL `LIMIT`, and the deduplication happens afterwards in Go, so the
  page is cut before duplicates are removed:

  | | upstream | Go port |
  | --- | --- | --- |
  | `distinct: ["city"], take: 3` | Paris(1), London(3), null(4) | Paris(1), **Paris(2)**, London(3) |

  `distinct-take-key-only` — the same query with `id` out of the projection —
  passes, because there `SELECT DISTINCT` can collapse rows in SQL. So the bug
  bites exactly when a unique column is selected, i.e. normally.

- **`distinct` over a pointer-valued column keys on the pointer**
  (`distinct-single-column`). `dedupeDistinct` builds its key with
  `fmt.Fprintf(&key, "%v\x00", ...)` on the raw field value. A nullable column
  must be modelled as `*string`, and `%v` of a non-nil pointer is its *address*,
  so no two rows ever collide:

  | | upstream | Go port |
  | --- | --- | --- |
  | `distinct: ["city"]` | 4 rows (Paris, London, null, Berlin) | **7 rows** — only the two `nil` cities collapse, because `%v` of a nil pointer is `<nil>` |

  `distinct-two-columns` and `distinct-descending-order` pass: they key on
  non-pointer columns.

- **An unresolvable distinct key is ignored** (`distinct-unknown-column`).
  `dedupeDistinct` returns the rows untouched when it cannot resolve any column,
  so `distinct: ["citty"]` returns all 8 rows where upstream refuses the query.

### 4. The cursor is exclusive, upstream's is inclusive (`cursor-forward`, `cursor-skip-one`, `cursor-descending`)

`Query.Cursor` compiles to `col > value` (or `<` for a descending order).
Prisma's cursor row is *included* in the page and `skip: 1` is what excludes it.
Every cursor page is therefore off by one:

| | upstream | Go port |
| --- | --- | --- |
| `cursor: { id: 3 }, take: 2` | 3, 4 | **4, 5** |
| `cursor: { id: 3 }, skip: 1, take: 2` | 4, 5 | **5, 6** |
| `cursor: { id: 5 }, take: 2, orderBy id desc` | 5, 4 | **4, 3** |

### 5. `update` / `delete` ignore `take`/`skip`/`orderBy`, and the singular forms are aliases

`Update`/`Delete` and `UpdateMany`/`DeleteMany` are literally the same methods,
and `UpdateSQL`/`DeleteSQL` emit no `LIMIT`. Prisma has no such arguments on a
write and rejects them outright, so a caller who asked to touch one row touches
all of them:

| case | upstream | Go port |
| --- | --- | --- |
| `updateMany({ where: { published: true }, data: …, take: 1 })` | rejected: unknown argument `orderBy` | **7 rows updated** |
| `deleteMany({ where: { published: true }, take: 1 })` | rejected | **7 rows deleted** |
| `delete({ where: { published: true } })` | rejected: `where` needs `id` | **7 rows deleted** |

### 6. `upsert` is a raw `ON CONFLICT`, not Prisma's find-then-write

`Query.Upsert(conflictColumns, create, update)` takes only the *names* of the
conflict columns. The `where` values are unrepresentable, so the row that gets
updated is whichever one the `INSERT` happens to collide with:

- `upsert-where-not-matched-by-create`: where `email = ada@x.io`, create
  `brand@x.io`. Upstream updates Ada; the port inserts a ninth user and leaves
  Ada alone.
- `upsert-on-primary-key`: `where: { id: 1 }`. The port drops an auto integer
  primary key from every `INSERT`, so it can never conflict on one — it inserts
  instead of updating.
- `upsert-atomic-operator`: `UpsertSQL` binds each update value with `w.bind(...)`
  instead of checking for the `updateExpr` interface the way `UpdateSQL` does, so
  `prisma.Increment(5)` reaches the driver as a Go struct:
  `sql: converting argument $6 type: unsupported type prisma.UpdateOp, a struct`.
  The atomic operators work in `Update` and are unusable in `Upsert`.

### 7. `take: -n` reads as "unlimited", not "last n" (`take-negative`)

Prisma's negative take means the last *n* rows. The port passes it straight to
`LIMIT`, where SQLite reads a negative value as no limit: upstream returns
`[7, 8]`, the port returns all eight rows.

### 8. `skip` without `take` is not a valid statement (`skip-without-take`)

`buildSelect` emits `OFFSET` only after a `LIMIT` it did not write, and SQLite
has no bare `OFFSET`. `skip: 6` works upstream and is a syntax error in the port.

### 9. `count` folds `take` into the counted set (`count-with-take`)

`CountSQL` deliberately counts a `LIMIT`ed subquery, with a comment asserting
that "Prisma applies take/skip to the set being counted". Upstream 5.22.0 does
not: `count({ where: { score: { gte: 30 } }, take: 2 })` returns **6**, the port
returns **2**.

### 10. LIKE wildcards are escaped with a backslash but no `ESCAPE` clause (`contains-percent`, `contains-underscore`)

`escapeLike` rewrites `%` to `\%` and `_` to `\_`, but the emitted SQL is
`col LIKE ?` with no `ESCAPE '\'`, and SQLite then treats the backslash as an
ordinary character. Upstream does not escape at all, so the two disagree in
opposite directions: `contains: "%"` matches all 12 posts upstream and none in
the port; `contains: "_two"` matches `beta_two` upstream and nothing in the port.

### 11. Aggregates are `float64`-only and drop NULLs (`aggregate-string-min-max`, `aggregate-empty-set`)

`AggregateResult` is four `map[string]float64`, and `Exec` scans into
`sql.NullFloat64`:

| | upstream | Go port |
| --- | --- | --- |
| `_min: { name }, _max: { name }` | `Ada` / `Linus` | error: cannot scan a text value into a float |
| `_sum: { views }` over no rows | `{ views: null }` | `{}` — the key is dropped |

The same `float64` funnel means large `BigInt` sums would be lossy; this
datamodel has no `BigInt` column, so that is recorded as untested rather than
demonstrated. `GroupQuery` does *not* share the fault — it scans into `any`, and
`groupby-string-min-max` passes.

### 12. There is no way to insert an explicit integer primary key (`create-explicit-id`)

`compileModel` sets `IsAuto` on any integer primary key unless the tag says
`auto`, and `parseTag` only ever *enables* `auto` — there is no `auto=false`. The
column is dropped from every `INSERT`:

| | upstream | Go port |
| --- | --- | --- |
| `create({ data: { id: 99, … } })` | id 99 | **id 9** |

This is why the seeds use consecutive ids from 1: it is the only id assignment
both sides can produce. It is also the root cause of `upsert-on-primary-key`.

### 13. Two incompatible not-found errors (`find-first-missing`, `find-unique-missing`)

`FindFirst`/`FindUnique` return the bare sentinel `ErrNotFound`, which carries no
code; the `*OrThrow` variants return `ErrRecordNotFound`, a `*Error` with P2025.
`errors.Is(ErrNotFound, ErrRecordNotFound)` is false, so a caller cannot handle
"not found" once. Upstream returns `null` from `findFirst`/`findUnique` and
raises P2025 only from the `*OrThrow` forms — which the port matches exactly, so
the divergence is confined to the plain forms:

| | upstream | Go port |
| --- | --- | --- |
| `findFirst` no match | `null` | error, code `""` |
| `findUnique` no match | `null` | error, code `""` |
| `findFirstOrThrow` no match | P2025 | P2025 ✓ |
| `findUniqueOrThrow` no match | P2025 | P2025 ✓ |

### 14. `field: null` has no shorthand (`equals-null-literal`, `not-null-literal`)

Prisma's `{ city: null }` means `IS NULL`. Translating it field-for-field to
`Equals("city", nil)` yields `city = ?` bound to nil, which matches nothing —
silently. The port does offer `IsNull`/`IsNotNull`, and the `equals-null` /
`not-null` cases (which use them) pass; the `-literal` cases exist to show what
the mechanical translation costs. A null inside an `in` list is a related trap
(`in-with-null`): upstream rejects it, the port binds it and matches nothing.

### 15. Constraint-violation codes: three of four agree

| case | upstream | Go port |
| --- | --- | --- |
| duplicate `email` on create | P2002 | P2002 ✓ |
| duplicate `email` inside `createMany` | P2002 | P2002 ✓ |
| `author_id: 999` on create | P2003 | P2003 ✓ |
| delete a user that still has posts | P2003 | P2003 ✓ |
| `name: null` on create | *validation error, no code* | **P2011** |

The last row is not really a wrong answer: both refuse the write and neither
table changes. The failure *mode* differs — upstream rejects the arguments before
any SQL runs, the port lets SQLite refuse — and it is scored as a mismatch
because the codes differ.

### 16. Unvalidated column names (`where-unknown-column`, `orderby-unknown-column`, `select-unknown-column`)

Where/orderBy/cursor/distinct columns are plain strings. `Select` and the
aggregate builders do check them against the model, but `Where`, `OrderBy` and
`Cursor` do not, so a typo becomes a database round-trip and a driver error
instead of a build-time one. All three cases still *pass*, because both sides
fail; only `distinct-unknown-column` (§3) turns the missing validation into a
wrong answer.

## What is untested, and why

| symbols | why |
| --- | --- |
| `$transaction`, `Client.Transaction`, `Client.Batch` | both sides have it; comparing rollback across two processes is not deterministic, and the harness rule is to test ordering, not races |
| `$queryRaw`, `$queryRawUnsafe`, `$queryRawTyped`, `$executeRaw` (tagged-template form) | raw SQL is by definition not a portable comparison; `$executeRawUnsafe` is exercised by every `reset` |
| `having` combined with `AND`/`OR`/`NOT`, and `equals`/`in`/`notIn`/`lt`/`lte`/`not` inside `having` | the alias trick that makes `gt`/`gte` work applies unchanged; only those two are scored |
| `having: { _min }`, `having: { _max }` | as above |
| nested `connect` / `connectOrCreate` on create | `NestedCreate.Connect` / `.ConnectOrCreate` exist; only the `create` branch is scored |

## Reproducing

```sh
cd parity/prisma/node && npm install          # prisma + @prisma/client 5.22.0
cd .. && GOWORK=off go test ./            # each sub-repo is its own module
cd parity/prisma/node && node inventory.js    # the 219-symbol upstream inventory
```

`go test` performs the `prisma generate` and `prisma db push` steps itself and
`t.Skip`s — never fails — if Node, the pinned install, the generated client or
either step is unavailable.
