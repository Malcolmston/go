# migrate example

A single runnable program that exercises
[`github.com/malcolmston/migrate`](https://github.com/malcolmston/migrate)
against a real, throwaway SQLite database created in an `os.MkdirTemp`
directory. **No database server and no network access are needed at run time.**

The SQLite driver is `modernc.org/sqlite` (pure Go, cgo-free), used purely as
the `database/sql` backend — the library itself has no driver dependency.

## Resolved module version

The example consumes the published module, with no `replace` directive:

```
github.com/malcolmston/migrate v0.0.0-20260719021422-a57fafd75ccd
```

(`@latest` resolves to that pseudo-version; the repository has no semver tags,
even though `VERSION` in it says `0.2.0`.)

## Run

```sh
cd examples/migrate
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program prints nine labeled sections and terminates on its own.

## What it demonstrates

1. **Schema DSL / SQL generation** — the same `CreateTable` body rendered for
   all four dialects (`ANSI`, `Postgres`, `MySQL`, `SQLite`) covering `String`,
   `Text`, `Integer`, `Decimal`, `Boolean`, `Timestamp(Precision, WithTimezone)`,
   `UUID`, `JSONB`, `Binary`, `Enum`, `Array`, `References`, `Timestamps`; plus
   `DialectByName`, unique/partial/functional/`USING` indexes, `AddForeignKey`
   with `OnDelete`/`OnUpdate`, `AddReference`, bulk `ChangeTable`, check and
   unique constraints, views (incl. materialized + refresh), join tables,
   sequences, Postgres extensions/enums, `TruncateTable`,
   `ChangeColumnNull`, `ChangeColumnDefaultRaw`, `RenameIndex`.
2. **Reversible `Change` migrations** built with `ChangeWith(migrate.SQLite, …)`,
   alongside a plain `Migration` with DSL-rendered `UpSQL`/`DownSQL`.
3. **Loaders** — `LoadFS` over an `embed.FS` and `LoadDir` over a real
   directory, including the "non-matching files are ignored" behaviour.
4. **Migrate up** — `Up(ctx, 2)`, then `Migrate`, `Status`, idempotent re-run,
   `Version`, `Migrations`.
5. **Real DML on the migrated schema** plus idempotent `Seeder` (`Run`,
   `RunSQL`, `Applied`, `EnsureTable`, `Execute`, `ExecuteAll`), and a query
   through the view a Go-function migration created.
6. **Rollbacks** — `Rollback`, `Down(n)`, `MigrateTo(version)`, `MigrateTo(0)`,
   `Redo`, then back up again.
7. **Error / dirty-state handling** — `ErrDuplicateVersion`,
   `ErrInvalidMigration`, `ErrInvalidTableName`, a migration whose second
   statement fails (proving the per-migration transaction rolls back, the
   version is not recorded and the run halts), and `ErrMissingMigration` for an
   applied-but-unregistered version (also surfaced by `Status` as `(unknown)`).
8. **Irreversible `Change` migrations** — rollback fails with
   `ErrIrreversibleMigration` and the version row survives.
9. **`SchemaDump`** — a version-stamped, reconstructable Postgres DDL dump.

## Holes found

Nothing was missing badly enough to require commenting code out; the example
compiles and runs in full. The problems found are API/design gaps:

- **No dirty-state concept at all.** golang-migrate's `dirty` flag has no
  counterpart here: the bookkeeping table is only
  `(version, applied_at)`, `MigrationStatus` has no dirty/failed field, and
  there is no `Force`. In practice this is defensible — a failed migration is
  rolled back in its own transaction and never recorded — but it silently
  breaks for the DDL-non-transactional case (MySQL), where a half-applied
  migration would leave the database inconsistent with **no** marker at all.
  The README's transaction-per-migration claim is only true on engines with
  transactional DDL.
- **`Change`/`ChangeWith` produce an opaque migration.** The returned
  `Migration` carries closures in `Up`/`Down` and leaves `UpSQL`/`DownSQL`
  empty, and `ChangeRecorder` has no exported constructor and no exported
  accessor for the recorded statements. You therefore cannot print, review,
  diff or unit-test the SQL a reversible migration will run without actually
  executing it against a database. `ChangeRecorder.Reversible()` is exported
  but unreachable for the same reason, so reversibility can only be discovered
  by attempting the rollback at run time.
- **`Enum` values are silently dropped** on the ANSI and SQLite dialects: the
  column degrades to `VARCHAR(255)` with no `CHECK` constraint, so the value
  set is lost with no error or warning. On Postgres the column is typed
  `role`, which only works if you separately emit `CreateEnum` — the DSL does
  not do it for you.
- **`Array()` is not portable and fails silently**: ANSI renders
  `VARCHAR(255) ARRAY` and MySQL/SQLite fold the column to `JSON`, i.e. the
  same DSL produces three semantically different columns with no diagnostic.
- **Auto-generated constraint/index names are mangled** rather than rejected:
  `AddCheckConstraint("users", "login_count >= 0")` yields
  `chk_users_login_count____0` and a functional index on `lower(email)` yields
  `index_users_on_lower_email_`. They are stable but unreadable, and long
  expressions will collide.
- **`Migration.UpSQL` statement splitting is naive** (acknowledged in a code
  comment, but not in the README's "multiple `;`-separated statements" claim).
  `splitStatements` splits
  on `;`, so a semicolon inside a string literal, a `CREATE TRIGGER … BEGIN …
  END;` body, or a PL/pgSQL `$$ … $$` function body will be broken apart.
  Real DDL of that kind has to go through a Go `Up` function.
- **No CLI.** The package is library-only; there is no command to run
  migrations from a shell, and no `Create`/`New` helper to scaffold a
  timestamped migration file pair, both of which the ActiveRecord/golang-migrate
  workflows the README invokes rely on heavily.
- **Bookkeeping bind style is hardcoded to `?`** (documented, but it means the
  package cannot be used as-is against PostgreSQL through `lib/pq`/`pgx` in
  their default `$n` mode — `Dialect.Placeholder` exists for *your* SQL but is
  not used for the migrator's own inserts). This is the one real portability
  bug rather than a design choice.
- Minor: `example_test.go` and `memdb_test.go` show the in-memory driver, but
  it is test-only, so an example like this one must bring its own third-party
  driver even though the library advertises "no third-party dependencies".
