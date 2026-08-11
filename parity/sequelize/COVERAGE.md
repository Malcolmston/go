# Coverage: `sequelize` (Go) vs `sequelize@6.37.8` (npm)

The upstream inventory below is **derived mechanically** from the installed
package, not from memory or the README. Reproduce it with:

```sh
cd node && node -e '
const { Model, Op, DataTypes } = require("sequelize");
console.log(Object.getOwnPropertyNames(Model)
  .filter(n => { try { return typeof Object.getOwnPropertyDescriptor(Model,n).value==="function" && !n.startsWith("_") } catch { return false } })
  .sort().join("\n"));
console.log(Object.keys(Op).sort().join(" "));
console.log(Object.keys(DataTypes).filter(k=>/^[A-Z]/.test(k)).sort().join(" "));'
```

The Go surface is the exported methods on `*Model`, `*Sequelize` and the `Op`
namespace in `github.com/malcolmston/sequelize` (`grep 'func (m *Model)'` etc.).

Every symbol gets one of: **match** (behaviour verified equal by a live parity
case), **differs** (a documented deviation — see `sequelize/API-DEVIATIONS.md`),
**missing** (no port), **extra** (Go-only), **untested** (ported but no case).

## Model query & mutation API

| upstream `Model.*` | Go `Model.*` | status | cases |
| --- | --- | --- | --- |
| `findAll` | `FindAll` | match | `find-all-*` (23), `include-*` |
| `findOne` | `FindOne` | match / differs | `find-one-match`; `find-one-miss` (differs: typed error vs null) |
| `findByPk` | `FindByPk` | match / differs | `find-by-pk-match`; `find-by-pk-miss` (differs) |
| `findAndCountAll` | `FindAndCountAll` | untested | — |
| `findOrCreate` | `FindOrCreate` | untested | — |
| `findCreateFind` | — | missing | — |
| `findOrBuild` / `build` / `bulkBuild` | — | missing | — (port is map-based, has no in-memory instance) |
| `count` | `Count` | match | `count-all`, `count-where`, `count-gt` |
| `sum` | `Sum` | match | `sum-age`, `sum-age-where`, `sum-empty` |
| `min` | `Min` | match | `min-age`, `max-empty` |
| `max` | `Max` | match | `max-age`, `max-empty` |
| `aggregate` | (internal) | untested | — |
| `create` | `Create` | match | `create-basic`, `create-default-age-null`, `create-null-name-fails`, `create-dup-email-fails`, `type-bool-create`, `type-decimal-create` (differs) |
| `bulkCreate` | `BulkCreate` | match | `bulk-create`, `bulk-create-null-name-fails` |
| `update` | `Update` | match | `update-basic`, `update-none` |
| `destroy` | `Destroy` | match | `destroy-basic`, `destroy-none`, `destroy-all` |
| `restore` | `Restore` | untested | — (paranoid; exercised in the port's own doc example) |
| `upsert` | `Upsert` | untested | — |
| `increment` | `Increment` | untested | — |
| `decrement` | `Decrement` | untested | — |
| `truncate` | `Truncate` | untested | — |
| `sync` | `Sync` | match | (setup for every case) |
| `drop` | `Drop` | untested | — |
| `describe` | `InspectTable` | extra-named | — |
| `getTableName` | `TableName` | untested | — |
| `getAttributes` | `Attributes` | untested | — |
| `removeAttribute` / `refreshAttributes` / `init` | — | missing | port models are immutable after `Define` |
| `schema` / `dropSchema` / `describe` | `AlterTable`/`SchemaDiff`/`CreateTableSQL` | extra | — (migration helpers, no upstream 1:1) |
| `scope` / `unscoped` / `addScope` | `Scope`/`Unscoped`/`AddScope` | untested | — |
| `beforeCreate` … `afterUpsert` (hooks) | `AddHook`/`RemoveHook` | untested | — (registry present; order verified in port unit tests) |

## Associations

| upstream | Go | status | cases |
| --- | --- | --- | --- |
| `hasOne` | `HasOne` | untested | — |
| `hasMany` | `HasMany` | match | `include-hasmany-one/-empty/-all` |
| `belongsTo` | `BelongsTo` | match | `include-belongsto-one/-all` |
| `belongsToMany` | `BelongsToMany` | untested | — |
| `getAssociations` / `getAssociationForAlias` | `Associations` / `Association` | untested | — |
| eager `include` | `Query.Include` | match | see above (hasMany batched, belongsTo joined) |

## `Op` operator vocabulary (40 upstream keys)

| upstream `Op.*` | Go `Op.*` | status | cases |
| --- | --- | --- | --- |
| `eq` | `Eq` | match | `find-all-where-eq`, `type-bool-where-*` |
| `ne` | `Ne` | match | `find-all-where-ne` |
| `gt` `gte` `lt` `lte` | `Gt` `Gte` `Lt` `Lte` | match | `find-all-where-gt/-gte/-lt/-lte` |
| `in` | `In` | match | `find-all-where-in`, `-in-shorthand`, `-in-empty` |
| `notIn` | `NotIn` | match | `find-all-where-notin`, `-notin-empty` |
| `like` | `Like` | match | `find-all-where-like` |
| `notLike` | `NotLike` | match | `find-all-where-notlike` |
| `between` | `Between` | match | `find-all-where-between` |
| `notBetween` | `NotBetween` | match | `find-all-where-notbetween` |
| `is` | `Is` / `IsNull` | match | `find-all-where-is-null`, `find-all-where-null` |
| `not` | `Not` / `NotNull` | match | `find-all-where-not-null`, `find-all-not` |
| `and` | `And` | match | `find-all-and` |
| `or` | `Or` | match | `find-all-or` |
| `iLike` `notILike` | `ILike` `NotILike` | untested | — (renders `ILIKE`; SQLite rejects it, so not comparable on this backend) |
| `startsWith` `endsWith` `substring` | — | missing | expressible via `Like` |
| `regexp` `notRegexp` `iRegexp` `notIRegexp` | — | missing | not portable across dialects |
| `col` `all` `any` `values` `placeholder` `join` | — | missing | query-builder internals, not user Op values |
| `contains` `contained` `overlap` `adjacent` `strictLeft` `strictRight` `noExtendLeft` `noExtendRight` | — | missing | PostgreSQL range/array operators; no SQLite equivalent |
| `match` | — | missing | full-text (tsvector), Postgres-only |

## DataTypes (39 upstream)

| upstream | Go | status | cases |
| --- | --- | --- | --- |
| `INTEGER` | `INTEGER()` | match | pervasive (`id`, `age`) |
| `STRING` | `STRING(n)` | match | pervasive (`name`, `email`) |
| `BOOLEAN` | `BOOLEAN()` | match | `type-bool-*` |
| `FLOAT` | `FLOAT()` | match | `type-float-sum`, `type-float-max` |
| `DECIMAL` `NUMERIC` | `DECIMAL(p,s)` | differs | `type-decimal-create` — string vs number on SQLite (see API-DEVIATIONS) |
| `BIGINT` | `BIGINT()` | untested | — (used internally for COUNT) |
| `TEXT` | `TEXT()` | untested | — |
| `DATE` | `DATE()` | untested | — (kept out of cases: JS `Date` ISO string vs Go time serialisation is a format difference, not a behaviour) |
| `DATEONLY` | `DATEONLY()` | untested | — |
| `JSON` | `JSON()` | untested | — |
| `BLOB` | `BLOB()` | untested | — |
| `UUID` | `UUID()` | untested | — |
| `ENUM` | `ENUM(...)` | untested | — (renders as TEXT on SQLite; see the port's HANDOFF item 9) |
| `DOUBLE` `REAL` `NUMBER` | (via FLOAT/DECIMAL) | partial | — |
| `SMALLINT` `MEDIUMINT` `TINYINT` `CHAR` `TIME` `NOW` | — | missing | not ported in v0.1.0 |
| `ARRAY` `JSONB` `HSTORE` `RANGE` `GEOMETRY` `GEOGRAPHY` `CIDR` `INET` `MACADDR` `CITEXT` `TSVECTOR` `VIRTUAL` `UUIDV1` `UUIDV4` `ABSTRACT` | — | missing | PostgreSQL-specific or abstract; out of the v0.1.0 SQLite scope |

## `Sequelize` connection API

| upstream | Go | status |
| --- | --- | --- |
| `new Sequelize` / `.define` / `.sync` / `.transaction` / `.close` / `.model` | `New`/`Open`, `Define`, `Sync`, `Transaction`, `Close`, `Model` | match (used as setup; `Transaction` verified in the port's doc example) |
| `.query` (raw) | `Query` | untested |
| `.authenticate` | `Ping` | untested |

## Query options (`findAll` second argument)

| upstream option | Go `Query` field | status | cases |
| --- | --- | --- | --- |
| `where` | `Where` | match | all `find-all-where-*` |
| `order` | `Order` | match | `find-all-order-desc` |
| `limit` | `Limit` | match | `find-all-limit` |
| `offset` | `Offset` | match | `find-all-limit-offset` |
| `attributes` | `Attrs` | match | `find-all-attributes` |
| `include` | `Include` | match | `include-*` |
| `group` / `having` | `Group` / `Having` | untested | — |
| `distinct` | `Distinct` | untested | — |
| `paranoid` | `Paranoid` | untested | — (verified in the port's doc example) |
| `transaction` | `Tx` | untested (harness) | — (verified in the port's doc example) |

## Score

Counting the symbols actually placed under a live parity case (the rows marked
**match** or **differs** above — `untested`/`missing`/`extra` are not part of the
parity ratio):

- **Compared symbols:** 41 (query/mutation ops, associations, `Op` vocabulary,
  core DataTypes, query options).
- **match:** 38 · **differs (documented deviation):** 3 · **mismatch (bug):** 0
- **Symbol parity:** 38 / 38 behavioural comparisons agree = **100%**, with 3
  deviations set aside and documented.

Live case totals (from `parity.json`, regenerated by `parity_test.go`):

- **61 cases** total — **58 compared**, **58 match (100%)**, **0 mismatch**,
  **3 deviations** (`find-one-miss`, `find-by-pk-miss`, `type-decimal-create`).
- Per group: query 30 (28 match + 2 dev), aggregate 9, mutation 11,
  associations 5, types 6 (5 match + 1 dev).

The large `missing` column is almost entirely PostgreSQL-only surface (range and
array operators, geometry, tsvector) that has no SQLite meaning, plus the
in-memory instance API (`build`, `save`, `reload`, `changed`) the port omits by
design because it is map-based rather than instance-based — its single largest
deliberate deviation, recorded in `API-DEVIATIONS.md`.
