# `migrate` parity coverage

- **Port under test:** `github.com/malcolmston/migrate v0.3.0` (published module, no `replace`).
- **Oracle:** `rails/rails` ActiveRecord **8.0.2** (`activerecord` 8.0.2 + `sqlite3` 2.7.4), pinned in
  `ruby/Gemfile` with a committed `ruby/Gemfile.lock`.
- **Backend:** SQLite on both sides, one fresh database per case in its own temp directory.
- **Run:** `GOWORK=off go test ./parity/migrate/`
- **Score:** see `parity.json`; the per-case SQL each side emitted is dumped to `sql.json`.

## What is compared

The comparable artefact is **the schema each side actually produces**, not the SQL text — formatting
differs harmlessly, so comparing strings would be noise. Each case is a script of operations; the
compared value is:

- `steps` — per operation, `{i, op, ok}` (and, for migration cases, `version`, `applied`, the
  `{version, applied}` status projection, and a table/column outline after every step);
- `schema` — a canonical read-back of the SQLite catalogue: for every table its columns
  (name, declared type, nullability, default, primary-key position), its index list (uniqueness,
  partiality, origin, column order), its foreign keys and its CHECK constraints;
- `bookkeeping` — the shape of `schema_migrations`, in the one case that asks for it.

Both runners additionally return `sql` (every statement they emitted) and `raw` (un-normalised index
and constraint names, error texts, the verbatim `CREATE TABLE` of each table). **The harness strips
those two keys before comparing** and writes them to `sql.json`, so type-mapping and naming
differences stay visible without being scored twice.

Error *text* is never compared; only whether an operation failed.

### Normalisations (applied identically by both runners)

1. **Declared types** are whitespace-collapsed and upper-cased (`varchar(40)` → `VARCHAR(40)`).
   The *content* of the type is still compared — `varchar` vs `VARCHAR(255)` is a reported
   divergence, not a normalisation.
2. **An `INTEGER PRIMARY KEY` is reported `NOT NULL`.** SQLite forbids a null rowid whatever the DDL
   said, so the port's omission of `NOT NULL` on the auto id column is not scored. (A `TEXT PRIMARY
   KEY` gets no such exemption, which is why `table-without-id` diverges.)
3. **Index names** collapse to `<default>` when they equal the ActiveRecord convention
   `index_<table>_on_<col>_and_<col>`, and to `<autoindex>` for `sqlite_autoindex_%`. Any other name
   is compared verbatim, because it was either requested explicitly by the case or invented by one
   side only. The raw name always travels in `raw.names`.
4. **CHECK-constraint names** collapse to `<auto>` unconditionally — both sides invent opaque names
   (`chk_rails_<hash>` vs `chk_users_login_count____0`). The raw names are reported side by side in
   `raw.names`; the port's mangled spelling is a finding, not a normalisation.
5. **`applied_at`/timestamps** are never compared, only the version bookkeeping they accompany.

### Fairness adjustments

- The Ruby runner provisions `schema_migrations` and `ar_internal_metadata` up front, mirroring the
  port's `Migrator.EnsureSchemaTable`, so a status query before the first migration is comparable.
- `Schema.AddColumn` / `Schema.ChangeColumn` / `AlterTable.Change` take a **raw SQL type string**;
  the port has no abstract-typed form. Where a case needs one it supplies `sql_type` for the Go side
  only, and the case notes say so. Cases that measure *type mapping* never do this — they go through
  `create_table`, where the port's own typed DSL renders the type.
- A migration whose spec omits `down` gets no down direction on either side; an explicitly empty
  `"down": []` gets an empty no-op rollback on both.

## How the upstream inventory was derived

Mechanically, by reflecting over the installed gem — not from the README or from memory:

```ruby
# parity/migrate/ruby $ bundle exec ruby -e '<the following>'
require "active_record"
ActiveRecord::Base.establish_connection(adapter: "sqlite3", database: "/tmp/x.sqlite3")
c = ActiveRecord::Base.lease_connection
puts ActiveRecord::ConnectionAdapters::SchemaStatements.public_instance_methods(false).sort.inspect
puts ActiveRecord::ConnectionAdapters::SQLite3Adapter.public_instance_methods(false).sort.inspect
puts c.native_database_types.keys.sort.inspect
puts ActiveRecord::ConnectionAdapters::TableDefinition.public_instance_methods(false).sort.inspect
puts ActiveRecord::MigrationContext.public_instance_methods(false).sort.inspect
puts ActiveRecord::Migrator.public_instance_methods(false).sort.inspect
```

That yields 78 `SchemaStatements` public instance methods, 56 `SQLite3Adapter` public overrides,
12 `native_database_types` keys, 40 `TableDefinition` public methods, 19 `MigrationContext` public
methods and 10 `Migrator` public methods. The Go side of each row was derived from
`GOWORK=off go doc -all github.com/malcolmston/migrate`.

---

## 1. `SchemaStatements` — schema-mutating methods (31)

`truncate` lives in `DatabaseStatements`, not `SchemaStatements`; it is included because it mutates.

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `add_belongs_to` (alias of `add_reference`) | `Schema.AddReference` | differs | `add-reference-plain` | see `add_reference` |
| `add_check_constraint` | `Schema.AddCheckConstraint` | differs | `add-check-constraint` | AR's SQLite3 adapter emits invalid `ALTER TABLE … ADD CHECK` and fails; the port emits `ALTER TABLE … ADD CONSTRAINT` which SQLite also rejects — but the *port* succeeded here because SQLite accepted its spelling, producing a constraint AR never made, named `chk_users_login_count____0` |
| `add_column` | `Schema.AddColumn`, `AlterTable.*` | differs | `add-column-text`, `add-column-not-null-default`, `add-column-decimal` | abstract types only exist for string/text/integer/bigint/boolean/timestamp on `AlterTable`; anything else needs a raw SQL type |
| `add_columns` | — | missing | — | no plural form |
| `add_foreign_key` | `Schema.AddForeignKey` | differs | `add-foreign-key`, `add-foreign-key-on-delete-cascade`, `remove-foreign-key` | AR rebuilds the table; the port emits `ALTER TABLE … ADD CONSTRAINT`, which SQLite rejects — the FK is silently never created |
| `add_index` | `Schema.AddIndex` | match | `add-index-single`, `add-index-unique`, `add-index-composite`, `add-index-named`, `add-index-partial`, `add-index-expression` | including partial and expression indexes and the `index_<t>_on_<c>` naming convention |
| `add_reference` | `Schema.AddReference` | differs | `add-reference-plain`, `add-reference-index-and-fk`, `add-reference-polymorphic-index` | AR indexes the reference by default and uses `integer`; the port uses `BIGINT`, adds no index unless asked, cannot add the FK on SQLite, and names the polymorphic index differently |
| `add_timestamps` | `Schema.AddTimestamps` | differs | `add-timestamps`, `change-table-timestamps-and-unique-index` | AR `datetime(6)`; the port `TIMESTAMP` here but `DATETIME` from `Table.Timestamps` — inconsistent with itself |
| `change_column` | `Schema.ChangeColumn` | differs | `change-column-type` | AR rebuilds the table; the port emits `ALTER COLUMN … TYPE`, which SQLite rejects |
| `change_column_comment` | `Schema.SetColumnComment` | untested | — | SQLite has no `COMMENT ON` |
| `change_column_default` | `Schema.ChangeColumnDefault`, `Schema.DropColumnDefault` | differs | `change-column-default`, `drop-column-default` | AR rebuilds the table; the port emits `ALTER COLUMN … SET/DROP DEFAULT`, which SQLite rejects |
| `change_column_null` | `Schema.ChangeColumnNull` | differs | `change-column-null-false` | AR rebuilds the table; the port emits `ALTER COLUMN … SET NOT NULL`, which SQLite rejects |
| `change_table` | `Schema.ChangeTable` + `AlterTable` | differs | `change-table-bulk`, `change-table-timestamps-and-unique-index` | the block itself matches; only the timestamp type diverges |
| `change_table_comment` | `Schema.SetTableComment` | untested | — | SQLite has no `COMMENT ON` |
| `create_join_table` | `Schema.CreateJoinTable` | differs | `create-join-table` | the port's naive de-pluralisation yields `assemblie_id` where AR yields `assembly_id`, and `BIGINT` where AR uses `integer` |
| `create_table` | `Schema.CreateTable` | differs | `table-default-primary-key`, `table-if-not-exists`, `table-without-id`, `table-named-primary-key`, `table-check-constraint-inline` | default and `id: false` shapes; the port has no `primary_key:` option and no in-table check constraint |
| `drop_join_table` | `Schema.DropJoinTable` | match | `drop-join-table` | |
| `drop_table` | `Schema.DropTable`, `Schema.DropTableIfExists` | match | `drop-table` | |
| `remove_belongs_to` (alias) | `Schema.RemoveReference` | match | `remove-reference` | |
| `remove_check_constraint` | `Schema.RemoveCheckConstraint` | match | `remove-check-constraint` | end state agrees |
| `remove_column` | `Schema.DropColumn` | match | `remove-column` | |
| `remove_columns` | — | missing | — | no plural form |
| `remove_constraint` | — | missing | — | no generic constraint drop |
| `remove_foreign_key` | `Schema.RemoveForeignKey` | differs | `remove-foreign-key` | unusable on SQLite for the same reason as `add_foreign_key` |
| `remove_index` | `Schema.DropIndex` | match | `remove-index-by-columns` | the port takes a name only; the harness derives the conventional name |
| `remove_reference` | `Schema.RemoveReference` | match | `remove-reference` | |
| `remove_timestamps` | `Schema.RemoveTimestamps` | match | `remove-timestamps` | |
| `rename_column` | `Schema.RenameColumn` | match | `rename-column`, `sequence-of-changes` | |
| `rename_index` | `Schema.RenameIndex` | differs | `rename-index` | AR drops and recreates; the port emits `ALTER INDEX`, which SQLite has no syntax for |
| `rename_table` | `Schema.RenameTable` | match | `rename-table` | |
| `truncate` (`DatabaseStatements`) | `Schema.TruncateTable` | differs | `truncate-table` | AR issues `DELETE FROM`; the port emits `TRUNCATE TABLE`, which SQLite lacks |

**Subtotal:** match 11, differs 15, missing 3, untested 2.

## 2. `SchemaStatements` — introspection and internal builders (48)

The port is **write-only DDL**: it renders and executes statements but exposes no schema
introspection whatsoever. Everything in this section is therefore `missing` unless a Go counterpart
exists. Grouped rather than repeated row-by-row, with every name listed.

| upstream symbols | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `tables`, `table_exists?`, `table_comment`, `table_options`, `columns`, `column_exists?`, `primary_key`, `indexes`, `index_exists?`, `index_name_exists?`, `foreign_keys`, `foreign_key_exists?`, `check_constraints`, `check_constraint_exists?`, `views`, `view_exists?`, `data_sources`, `data_source_exists?`, `native_database_types`, `max_index_name_size`, `assume_migrated_upto_version` (21) | — | missing | — | no introspection API in the port at all; the harness reads `sqlite_master`/pragmas itself |
| `add_index_options`, `build_add_column_definition`, `build_change_column_default_definition`, `build_create_index_definition`, `build_create_join_table_definition`, `build_create_table_definition`, `check_constraint_options`, `columns_for_distinct`, `distinct_relation_for_primary_key`, `foreign_key_column_for`, `foreign_key_options`, `index_algorithm`, `internal_string_options_for_primary_key`, `options_include_default?`, `quoted_columns_for_index`, `schema_creation`, `table_alias_for`, `use_foreign_keys?`, `valid_column_definition_options`, `valid_primary_key_options`, `valid_table_definition_options` (21) | — | missing | — | builder plumbing; nothing a port is expected to reproduce, listed for completeness |
| `create_schema_dumper`, `dump_schema_information` | `SchemaDump`, `SchemaDump.String` | untested | — | the port's dump is a hand-recorded script, not derived from a live schema |
| `bulk_change_table`, `update_table_definition` | `Schema.ChangeTable` | untested | — | reached indirectly by the `change_table` cases |
| `index_name` | (implicit in `Schema.AddIndex`) | untested | — | naming is asserted through `add_index` instead |
| `type_to_sql` | `Dialect.columnType` (unexported) | untested | — | not reachable from the port's public API |

**Subtotal:** match 0, differs 0, missing 42, untested 6.

## 3. Column type vocabulary (21)

Types are measured through `create_table`, so each side's own DSL renders the type. Cases live in
`cases/types.json`.

| upstream type | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `primary_key` (implicit `id`) | `Schema.CreateTable` auto id | differs | `table-without-id`, `table-named-primary-key` | AR: `integer PRIMARY KEY AUTOINCREMENT NOT NULL`; the port omits `NOT NULL`, which is harmless for an `INTEGER` rowid alias but loses the constraint for a `TEXT` primary key |
| `string` | `Table.String` | differs | `type-string`, `type-string-limit`, `type-string-default` | AR emits an unlimited `varchar`; the port imposes `VARCHAR(255)` the caller never asked for. `limit:` matches |
| `text` | `Table.Text` | match | `type-text` | |
| `integer` | `Table.Integer` | match | `type-integer` | |
| `bigint` | `Table.BigInteger` | match | `type-bigint` | |
| `float` | `Table.Float` | differs | `type-float` | AR `float`, port `REAL` |
| `decimal` | `Table.Decimal` | match | `type-decimal` | `DECIMAL(10,2)` on both |
| `numeric` (alias of `decimal`) | `Table.Decimal` | untested | — | alias not separately exercised |
| `datetime` | `Table.Timestamp` | differs | `type-datetime`, `type-datetime-precision`, `type-datetime-default-raw` | AR defaults to `datetime(6)`; the port emits bare `DATETIME` and **silently drops `Precision`** on SQLite |
| `timestamp` (alias of `datetime`) | `Table.Timestamps`, `AlterTable.Timestamps` | differs | `type-timestamps`, `add-timestamps` | as above, and the two port spellings disagree with each other (`DATETIME` vs `TIMESTAMP`) |
| `time` | `Table.Time` | match | `type-time` | |
| `date` | `Table.Date` | match | `type-date` | |
| `binary` | `Table.Binary` | match | `type-binary` | |
| `blob` (alias of `binary`) | `Table.Binary` | untested | — | alias not separately exercised |
| `boolean` | `Table.Boolean` | match | `type-boolean` | |
| `json` | `Table.JSON` | match | `type-json` | |
| `jsonb` (not native on SQLite) | `Table.JSONB` | differs | `type-jsonb` | AR passes `jsonb` through; the port folds it to `JSON` |
| `uuid` (not native on SQLite) | `Table.UUID` | differs | `type-uuid` | AR passes `uuid` through; the port emits `VARCHAR(36)` |
| `enum` (PostgreSQL/MySQL only) | `Table.Enum` | differs | `type-enum` | **AR refuses** (`ArgumentError: Unknown key: :values`); the port silently produces `VARCHAR(255)` with no `CHECK`, so the enumeration is lost with no diagnostic |
| `array: true` (PostgreSQL only) | `Array()` | differs | `type-array-of-string` | **AR refuses** (`ArgumentError: Unknown key: :array`); the port silently produces `JSON` on SQLite (and `JSON` on MySQL, `base[]` on PostgreSQL, `base ARRAY` on ANSI) with no diagnostic |
| `virtual` / `t.as` (generated columns) | — | missing | — | not ported |

**Subtotal:** match 9, differs 9, missing 1, untested 2.

## 4. Column options (9)

| upstream option | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `limit:` | `Limit` | match | `type-string-limit` | |
| `precision:` | `Precision` | differs | `type-datetime-precision` | dropped by the port's SQLite dialect |
| `scale:` | `Scale` | match | `type-decimal` | |
| `null:` | `NotNull` | match | `type-not-null-and-default` | |
| `default:` | `Default`, `DefaultRaw` | differs | `type-boolean-default-false`, `type-string-default`, `type-datetime-default-raw` | booleans: AR stores `0`/`1`, the port stores `FALSE`/`TRUE` — SQLite has no boolean literal, so the port's default is the string `FALSE` |
| `primary_key:` (on a column) | `PrimaryKey` | differs | `table-without-id` | the port does not imply `NOT NULL` |
| `collation:` | — | missing | — | not ported |
| `comment:` | `SetColumnComment` (statement only) | missing | — | no per-column comment inside `create_table` |
| `if_not_exists:` | `IfNotExists` | match | `table-if-not-exists` | |

**Subtotal:** match 4, differs 3, missing 2, untested 0.

## 5. `MigrationContext` (19)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `migrate` | `Migrator.Migrate`, `Migrator.MigrateTo` | match | `up-all-then-version`, `up-idempotent`, `rollback-all`, `migrate-to-middle-then-forward` | |
| `up` | `Migrator.MigrateTo` (upward) | match | `migrate-to-middle-then-forward` | |
| `down` | `Migrator.MigrateTo` (downward), `Migrator.Down` | match | `rollback-all` | |
| `forward` | `Migrator.Up(n)` | differs | `up-one-step-at-a-time` | AR counts steps from the *current migration's index*, so `forward(1)` from version 0 applies **two** migrations; the port's `Up(1)` applies exactly one pending migration |
| `rollback` | `Migrator.Rollback` | differs | `rollback-one`, `rollback-empty-down`, `rollback-irreversible-no-down` | a migration with no `down` method: AR silently un-records the version and leaves the table behind; the port refuses with `ErrMissingMigration` and keeps the version |
| `current_version` | `Migrator.Version` | match | `up-all-then-version` | |
| `get_all_versions` | `Migrator.Status` (applied set) | match | every mechanics case | |
| `migrations` | `Migrator.Migrations` | match | every mechanics case | |
| `migrations_status` | `Migrator.Status` | match | `status-across-a-partial-migration` | the `{version, applied}` projection agrees; **neither** side carries a failed/dirty flag |
| `schema_migration` | `Migrator.EnsureSchemaTable` table | differs | `bookkeeping-table-shape` | AR: `schema_migrations(version VARCHAR PRIMARY KEY)`; the port: `schema_migrations(version BIGINT PRIMARY KEY, applied_at TIMESTAMP NOT NULL)` |
| `internal_metadata` | — | missing | — | the port has no metadata table, so no environment or schema-format record |
| `current_environment` | — | missing | — | |
| `last_stored_environment` | — | missing | — | |
| `protected_environment?` | — | missing | — | no production guard |
| `needs_migration?` | — | missing | — | derivable from `Status`, not exposed |
| `pending_migration_versions` | — | missing | — | derivable from `Status`, not exposed |
| `open` | — | missing | — | |
| `run` | — | missing | — | no "run exactly this one version" entry point |
| `migrations_paths` | `LoadDir`, `LoadFS` | untested | — | the file loader is not exercised; cases register migrations programmatically |

**Subtotal:** match 7, differs 3, missing 8, untested 1.

## 6. `Migrator` (10)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `migrate` | `Migrator.Migrate` | match | `up-all-then-version` | |
| `current_version` | `Migrator.Version` | match | `up-all-then-version` | |
| `migrations` | `Migrator.Migrations` | match | mechanics | |
| `migrated` | `Migrator.Status` | match | mechanics | |
| `load_migrated` | `Migrator.Status` | match | mechanics | |
| `current` | — | missing | — | |
| `current_migration` | — | missing | — | |
| `pending_migrations` | — | missing | — | |
| `run` | — | missing | — | |
| `runnable` | — | missing | — | |

**Subtotal:** match 5, differs 0, missing 5, untested 0.

## 7. `ActiveRecord::Migration` class API (7)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `up` | `Migration.Up` / `Migration.UpSQL` | match | every mechanics case | but see the raw-SQL findings below |
| `down` | `Migration.Down` / `Migration.DownSQL` | match | `rollback-one`, `rollback-empty-down` | |
| `change` | `Change`, `ChangeWith`, `ChangeRecorder` | untested | — | the harness drives explicit up/down so both sides are compared on the same footing |
| `revert` | `ChangeRecorder` inverse | untested | — | |
| `ActiveRecord::IrreversibleMigration` | `ErrIrreversibleMigration` | untested | — | raised only by `change`-style migrations, which are untested |
| `disable_ddl_transaction!` | — | missing | — | the port always wraps a migration in a transaction, with no opt-out |
| `check_all_pending!` / `check_pending!` | — | missing | — | no boot-time guard against an out-of-date schema |

**Subtotal:** match 2, differs 0, missing 2, untested 3.

## 8. Go-only surface (14)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `Migrator.Redo` | extra | `rollback-then-reapply` | AR has no single call for it (rake task only) |
| — | `Migrator.Validate` | extra | — | |
| — | `Migrator.EnsureSchemaTable` | extra | mechanics (all) | |
| — | `WithTable` | extra | — | AR's table name is not configurable |
| — | `NewSeeder` / `Seeder` | extra | — | tracked, idempotent seeds; Rails' `db/seeds.rb` is untracked |
| — | `DialectByName` / `NewSchema` / `ANSI` / `Postgres` / `MySQL` / `SQLite` | extra | all | the port renders for a dialect chosen by the caller rather than by a live connection |
| — | `Schema.AddUniqueConstraint` / `RemoveUniqueConstraint` | extra | `add-unique-constraint` | AR has these on PostgreSQL only; on SQLite both sides refuse, so the case scores as a match |
| — | `Schema.CreateEnum` / `DropEnum` / `AddEnumValue` | extra | — | PostgreSQL-only in AR |
| — | `Schema.CreateSequence` / `DropSequence` | extra | — | |
| — | `Schema.CreateView` / `DropView` / `RefreshMaterializedView` | extra | — | AR can only read views |
| — | `Schema.EnableExtension` / `DisableExtension` | extra | — | PostgreSQL-only in AR |
| — | `JoinTableName` | extra | `create-join-table` | |
| — | `Table.EnumType` | extra | — | |
| — | `Deferrable` | extra | — | |

---

## Totals

| status | symbols |
| --- | --- |
| match | 38 |
| differs | 30 |
| missing | 63 |
| untested | 14 |
| extra | 14 |
| **inventory total** | **159** (145 upstream + 14 Go-only) |

- **Symbol parity: 38 / 68 = 55.9 %** — denominator is the symbols actually compared
  (`match + differs`); `missing`, `untested` and `extra` are excluded, as `HARNESS.md` requires.
- **Case parity: 44 / 86 = 51.2 %** — denominator is every case minus deliberate deviations
  (there are none). Per group: `ddl` 22/45, `types` 12/24, `mechanics` 10/17.
- Cases total 86 across 3 files; 0 deviations.

## Findings, worst first

Schema differences that **silently lose intent** (both sides report success, the port's schema is
weaker):

1. **`Enum` values are dropped.** `Table.Enum(name, values)` on SQLite renders `VARCHAR(255)` with
   no `CHECK` and no error. ActiveRecord refuses the operation outright rather than pretending.
   (`type-enum`)
2. **`Array()` degrades by dialect with no diagnostic** — `JSON` on SQLite and MySQL, `base[]` on
   PostgreSQL, `base ARRAY` on ANSI. ActiveRecord refuses `array:` on a backend that cannot do it.
   (`type-array-of-string`)
3. **`Precision` is discarded** for `Timestamp`/`Time` on SQLite; ActiveRecord honours it, and its
   default is `datetime(6)`. Sub-second precision requested by the caller is lost.
   (`type-datetime-precision`, `type-datetime`, `type-timestamps`)
4. **A `PrimaryKey()` column is not `NOT NULL`.** `CREATE TABLE users ("k" TEXT PRIMARY KEY)` accepts
   NULL keys in SQLite; ActiveRecord emits `text NOT NULL PRIMARY KEY`. (`table-without-id`)
5. **`Table.References` adds no index**, uses `BIGINT` where ActiveRecord uses `integer`, and its
   polymorphic index is named `index_t_on_taggable_type_and_taggable_id` where ActiveRecord uses the
   composite name `index_t_on_taggable`. A caller porting a Rails migration silently loses the index.
   (`table-references-plain`, `add-reference-*`)
6. **`CreateJoinTable` mis-singularises**: `assemblies` → `assemblie_id`, where ActiveRecord produces
   `assembly_id`. The column name is wrong, not merely differently typed. (`create-join-table`)
7. **`Default(false)` renders `FALSE`**, which SQLite stores as the *string* `FALSE`; ActiveRecord
   stores `0`. (`type-boolean-default-false`)
8. **`Table.String` imposes `VARCHAR(255)`** where ActiveRecord emits an unlimited `varchar`.
   (`type-string`, `type-string-default`, `sequence-of-changes`)
9. **`Timestamps()` is internally inconsistent**: `DATETIME` from `Table`, `TIMESTAMP` from
   `AlterTable`. (`add-timestamps` vs `type-timestamps`)
10. `UUID` → `VARCHAR(36)` and `JSONB` → `JSON` on SQLite, where ActiveRecord passes the declared
    type through. (`type-uuid`, `type-jsonb`)

Operations the port **cannot perform on SQLite at all**, because it emits standard SQL that SQLite
does not implement while ActiveRecord rebuilds the table instead:

11. `ChangeColumn` (`ALTER COLUMN … TYPE`), `ChangeColumnNull` (`SET NOT NULL`),
    `ChangeColumnDefault` / `DropColumnDefault` (`SET/DROP DEFAULT`),
    `AddForeignKey` / `RemoveForeignKey` (`ADD/DROP CONSTRAINT`),
    `RenameIndex` (`ALTER INDEX`), `TruncateTable` (`TRUNCATE`). Six of the ActiveRecord
    `SchemaStatements` core fail at the database. The port has one `SQLite` dialect but no
    per-dialect *strategies*, so it cannot do the table-rebuild dance SQLite requires.

Migration-state handling:

12. **There is no dirty/failed state at all**, confirmed: the bookkeeping table is exactly
    `(version BIGINT PRIMARY KEY, applied_at TIMESTAMP NOT NULL)`, there is no `Force`, and
    `MigrationStatus` has only `Version/Name/Applied/AppliedAt`. ActiveRecord has no dirty column
    either — but it does have `check_all_pending!`, `internal_metadata` and
    `protected_environment?`, all of which the port lacks, so nothing stops the port running against
    a schema it does not understand. (`bookkeeping-table-shape`, `status-across-a-partial-migration`)
13. **Transactional integrity is sound.** A migration that fails at its second operation leaves no
    partial schema and no recorded version on either side, and a later retry behaves the same.
    (`fail-midway-second-op`, `fail-midway-then-rollback-and-retry`, `raw-sql-failing-second-statement`)
14. **`Up(n)` and `forward(n)` count differently.** From version 0, ActiveRecord's `forward(1)`
    applies two migrations (it indexes from the current migration); the port applies one.
    (`up-one-step-at-a-time`)
15. **A migration with no down direction diverges in the dangerous direction — for ActiveRecord.**
    AR's `rollback` on a plain `up`/`down` migration with no `down` method silently deletes the
    version row and leaves the table; the port refuses with `ErrMissingMigration`. The port is
    stricter here. (`rollback-irreversible-no-down`)
16. **Statement splitting is naive on `;`**, confirmed: a semicolon inside a string literal
    (`INSERT … VALUES ('first; second')`) and a trigger body (`BEGIN … ; END`) are both split into
    fragments and fail. ActiveRecord's `execute` hands the whole script to SQLite. Conversely the
    sqlite3 gem executes only the *first* statement of a multi-statement string, so multi-statement
    raw SQL is not portable in either direction and the port is the more useful of the two here.
    (`raw-sql-semicolon-in-string-literal`, `raw-sql-trigger-body`, `raw-sql-two-statements`)
17. **Bookkeeping binds are hardcoded `?`**, confirmed by reading `migrator.go`
    (`INSERT INTO %s (version, applied_at) VALUES (?, ?)`, `DELETE FROM %s WHERE version = ?`).
    The `Postgres` dialect's `Placeholder` returns `$n`, so the `Postgres` dialect and the migrator's
    own bookkeeping cannot be used together. Not case-covered: this harness runs SQLite only, where
    `?` is correct.

Structural gaps:

18. **No schema introspection at all** (42 `missing` rows in section 2) — no `tables`, `columns`,
    `indexes`, `foreign_keys`, `column_exists?`. A port that cannot read the schema cannot implement
    the SQLite table-rebuild strategy that finding 11 requires.
19. **No abstract-typed `add_column` / `change_column`.** `Schema.AddColumn` and `ChangeColumn` take
    a raw SQL type string; `AlterTable` offers abstract types for only 6 of the 21 type vocabulary
    entries. (`add-column-decimal`)
20. **No `primary_key:` option, no composite primary keys, no in-table check constraints, no
    generated/virtual columns, no `collation:`, no plural `add_columns`/`remove_columns`, no
    `disable_ddl_transaction!`.**
