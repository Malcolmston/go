# `sled` parity coverage

- **Upstream oracle**: crates.io `sled` **0.34.7**, pinned as `sled = "=0.34.7"` in
  [`rust/Cargo.toml`](rust/Cargo.toml) (`Cargo.lock` in the same directory pins the
  full graph). Built with `rustc 1.91.1` / `cargo 1.91.1`.
- **Go port**: `github.com/malcolmston/sled v0.0.0-20260810111611-e90aa95364ed`,
  consumed as a published module (no `replace` directive).
- **Harness**: `GOWORK=off go test ./parity/sled/` — 70 cases in `cases/*.json`,
  each a *script* of operations against a fresh database in a temp directory. The
  compared answer is the result of every step plus a canonical dump of every tree
  (all key/value pairs in byte order, hex-encoded), so semantic divergence — not
  just a single return value — is what is scored.

## How the upstream symbol list was produced

`cargo doc --json` needs a nightly toolchain, which is not installed here
(`rustup toolchain list` → `stable`, `1.88`), so the inventory was extracted
mechanically from the *pinned, vendored* crate source that cargo downloaded:

```sh
S=~/.cargo/registry/src/index.crates.io-*/sled-0.34.7/src
# every public function on the public types
for f in db tree batch config iter ivec result subscriber transaction; do
  echo "-- $f.rs"
  grep -n '^\s*pub \(async \)\?fn \|^pub struct \|^pub enum \|^pub trait ' "$S/$f.rs"
done
# the public re-export surface of the crate root
sed -n '/^pub use self::{/,/^};/p' "$S/lib.rs"
# the Config builder knobs, which are macro-generated
sed -n '/^    builder!(/,/^    );/p' "$S/config.rs"
```

`#[doc(hidden)]` re-exports (the `pagecache`, `Lazy`, `Serialize`,
`crossbeam_epoch` block in `lib.rs`) are internal test hooks and are excluded, as
are the `fail` and `event_log` test-only modules. Everything else the crate
exports is in the table below.

Note that `sled::Db` is `Deref<Target = Tree>`, so every `Tree` method is also
reachable on `Db`; those are listed once, on `Tree`.

## API-level divergences worth stating up front

1. **`Open` takes a file, not a directory.** Upstream `sled::open`/`Config::path`
   take a *directory* that sled fills with its log, blobs and config. The Go port
   is an in-memory treap fronted by a single append-only write-ahead **log file**,
   so `sled.Open` takes the path *of that file*. Each runner therefore derives its
   own path from the temp directory the harness case is given.
2. **Empty keys.** sled accepts a zero-length key. The Go port rejects it with
   `sled.ErrEmptyKey` (`empty-key`, `batch-empty-key`).
3. **Range bounds.** sled's `range` accepts any `RangeBounds` (`Bound::Excluded`
   lower, `Bound::Included` upper). `sled.Range` is inclusive-lower /
   exclusive-upper only (`range-exclusive-lower`, `range-inclusive-upper`).
4. **Previous-value conventions.** sled's `insert`/`remove` *return the previous
   value*. The port splits this: `Set`/`Delete` return only an error, and the
   value-returning forms are `GetAndSet`/`GetAndDelete`. The scripts use the
   `GetAnd*` forms, which match upstream exactly.
5. **Tree persistence.** sled persists a tree the moment `open_tree` returns; the
   port only persists a tree once it receives a write
   (`opened-but-unwritten-tree-survives-reopen`).
6. **`DB.Compact()` is a port extension** (sled 0.34.7 has no explicit compaction
   entry point — its log is GC'd continuously). It carries a **bug**: see
   `compact-destroys-empty-tree` below.
7. **No event/subscriber comparison.** `watch_prefix`/`Watch` is deliberately not
   scored: comparing asynchronous event streams is not deterministic, and the
   harness rule is to test ordering, not races.

## Known port bug reproduced by the harness

`compact-destroys-empty-tree` is the single **mismatch** in the suite. Script:
create tree `t1`, write a key into it, remove that key (so `t1` is registered and
durable but currently empty), write a key into the default tree, then
`Compact()`, then reopen.

| step | upstream | Go port |
| --- | --- | --- |
| `tree_names` before compact | `[__sled__default, t1]` | `[__sled__default, t1]` |
| `tree_names` after compact | `[__sled__default, t1]` | `[__sled__default, t1]` |
| `tree_names` after reopen | `[__sled__default, t1]` | **`[__sled__default]`** |

The companion case `emptied-tree-survives-reopen` runs the identical script
*without* `Compact()` and passes on both sides, which isolates the fault to
compaction: `Compact()` rewrites the log with only live key/value records and so
drops the registration record of any tree that happens to be empty at that
moment. It is silent — `Compact()` returns `nil` and the tree is still listed
until the process restarts.

## Inventory

Status legend: `match` (compared and equal), `differs` (compared and divergent),
`missing` (upstream symbol not ported), `extra` (Go-only), `untested` (no case
compares it).

### Crate root / `Config`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sled::open` | `sled.Open` | differs | all 70 | upstream takes a directory, the port takes the log file path |
| `sled::Db::open` | `sled.Open` | differs | all 70 | same divergence |
| `sled::Config::new` | `sled.DefaultConfig` | untested | — | the Rust runner opens through `Config`, the Go runner through `Open`; the builder itself is not compared |
| `sled::Config::path` | `Config.Path` | untested | — | directory vs file path (see divergence 1) |
| `sled::Config::open` | `Config.Open` | untested | — | |
| `sled::Config::get_path` | `DB.Path` | untested | — | |
| `sled::Config::temporary` | `Config.Temporary` / `WithTemporary` | untested | — | port deletes the log file on clean `Close` |
| `sled::Config::cache_capacity` | — | missing | — | port has no page cache (whole index is in memory) |
| `sled::Config::mode` (`Mode::LowSpace`/`HighThroughput`) | — | missing | — | no space/throughput trade-off knob |
| `sled::Config::use_compression` | — | missing | — | port does not compress |
| `sled::Config::compression_factor` | — | missing | — | |
| `sled::Config::create_new` | — | missing | — | port always creates-or-opens |
| `sled::Config::flush_every_ms` | — | missing | — | port has `WithSyncWrites` (per-commit fsync) instead |
| `sled::Config::idgen_persist_interval` | — | missing | — | port's id block size is not configurable |
| `sled::Config::print_profile_on_drop` | — | missing | — | |
| `sled::Config::segment_size` (`#[doc(hidden)]`) | — | missing | — | log-segment tuning has no analogue |
| `sled::Config::snapshot_path` | — | missing | — | deprecated no-op upstream |
| `sled::Mode` | — | missing | — | |

### `sled::Db`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Db::open_tree` | `DB.OpenTree` | differs | `multiple-trees-isolation`, `tree-name-and-default`, `opened-but-unwritten-tree-survives-reopen`, `emptied-tree-survives-reopen`, `cross-tree-batch-and-prefix` | a tree that never received a write does not survive a restart in the port |
| `Db::drop_tree` | `DB.DropTree` | match | `drop-tree`, `drop-missing-tree`, `drop-default-tree`, `drop-tree-then-reopen` | both report `false` for a missing tree and both fail on the default tree |
| `Db::tree_names` | `DB.TreeNames` | match | `tree-names-sorted`, `drop-tree`, `compact-preserves-nonempty`, … | port returns `[]string`, upstream `Vec<IVec>`; both hex-encoded by the runners |
| `Db::was_recovered` | `DB.WasRecovered` | match | `was-recovered-fresh`, `reopen-twice` | |
| `Db::generate_id` | `DB.GenerateID` | match | `generate-id-monotonic`, `generate-id-monotonic-across-reopen` | strictly increasing, including across restarts; absolute values are implementation-defined so only monotonicity is compared |
| `Db::export` | `DB.Export` | untested | — | byte format is implementation-specific and not comparable across languages |
| `Db::import` | `DB.Import` | untested | — | same |
| `Db::checksum` | `DB.Checksum` | untested | — | different digest construction; not comparable |
| `Db::size_on_disk` | `DB.SizeOnDisk` | untested | — | storage layouts are unrelated |
| `Db::space_amplification` | — | missing | — | |
| `Db: Deref<Target = Tree>` | `DB.Set`/`Get`/`Delete`/`Scan`/… | match | all `core`, `cas`, `iteration` cases | the default keyspace is reachable directly off the handle on both sides |

### `sled::Tree`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Tree::insert` | `Tree.GetAndSet` | match | `insert-get-remove`, `insert-returns-previous-empty`, `non-utf8-keys`, `non-utf8-values`, `empty-value-roundtrip` | both return the previous value, `null` when absent, and an empty string for a previously-empty value |
| `Tree::set` (deprecated alias of `insert`) | `Tree.Set` | differs | — | port's `Set` returns only an error and discards the previous value |
| `Tree::get` | `Tree.Get` | match | `insert-get-remove`, `empty-value-roundtrip`, … | present-with-empty-value is distinguished from absent on both sides |
| `Tree::remove` | `Tree.GetAndDelete` | match | `insert-get-remove`, `remove-absent`, `removes-do-not-resurrect` | returns the removed value; removing an absent key is not an error |
| `Tree::del` (deprecated alias of `remove`) | `Tree.Delete` | differs | — | port's `Delete` discards the previous value |
| `Tree::compare_and_swap` | `Tree.CompareAndSwapErr` | match | `cas-sequence`, `cas-delete`, `cas-empty-vs-absent`, `cas-non-utf8`, `cas-survives-reopen` | on failure both report `current` and `proposed`; `old=None` means expect-absent, `new=None` means delete |
| `Tree::update_and_fetch` | `Tree.UpdateAndFetch` | match | `update-and-fetch`, `update-and-fetch-delete` | returns the new value; `None`/`nil` deletes |
| `Tree::fetch_and_update` | `Tree.FetchAndUpdate` | match | `fetch-and-update`, `update-and-fetch-delete` | returns the previous value |
| `Tree::apply_batch` | `Tree.ApplyBatch` | match | `batch-mixed`, `batch-same-key-twice`, `batch-atomic-across-reopen`, `batch-named-tree`, `batch-remove-absent`, `batch-empty` | applied atomically; later ops in a batch win; survives a reopen |
| `Tree::merge` | `Tree.Merge` | match | `merge-without-operator`, `merge-concat`, `merge-sum-u64`, `merge-delete`, `merge-per-tree-operator`, `merge-survives-reopen` | both fail when no operator is installed; both delete on a `nil`/`None` result |
| `Tree::set_merge_operator` | `Tree.SetMergeOperator` | match | `merge-*` | per-tree, effective immediately, lost on close (both runners reinstall after reopen) |
| `Tree::iter` | `Tree.Scan(sled.Range{})` | match | `iter-forward-reverse`, `iter-empty-tree`, `iter-after-removes`, `iter-survives-reopen` | ascending unsigned-byte key order |
| `Tree::range` | `Tree.Scan(sled.Range{Lower,Upper})` | differs | `range-lower-inclusive-upper-exclusive`, `range-non-utf8-bounds`, `range-exclusive-lower`, `range-inclusive-upper` | inclusive-lower/exclusive-upper agrees exactly; the port cannot express an exclusive lower or inclusive upper bound |
| `Tree::scan_prefix` | `Tree.ScanPrefix` / `sled.PrefixRange` | match | `prefix-forward-reverse`, `cross-tree-batch-and-prefix`, `iter-survives-reopen` | empty prefix scans everything; `0xff` prefix behaves identically |
| `Iter: DoubleEndedIterator` (`.rev()`) | `sled.Range.Reverse` | match | `iter-forward-reverse`, `prefix-forward-reverse`, `range-lower-inclusive-upper-exclusive` | reverse iteration visits the same set in the opposite order; bounds are unchanged |
| `Tree::first` | `Tree.First` | match | `first-last`, `pop-then-reopen` | |
| `Tree::last` | `Tree.Last` | match | `first-last`, `pop-then-reopen` | |
| `Tree::pop_min` | `Tree.PopMin` | match | `pop-min-max-drain`, `pop-empty-tree`, `pop-non-utf8`, `pop-then-reopen` | returns `null` and leaves the tree untouched when empty |
| `Tree::pop_max` | `Tree.PopMax` | match | `pop-min-max-drain`, `pop-empty-tree`, `pop-non-utf8` | |
| `Tree::get_gt` | `Tree.GetGt` | match | `get-gt-lt` | |
| `Tree::get_lt` | `Tree.GetLt` | match | `get-gt-lt` | |
| `Tree::contains_key` | `Tree.ContainsKey` | match | `contains-key`, `empty-value-roundtrip`, `cas-delete` | |
| `Tree::len` | `Tree.Len` | match | `len-clear-is-empty`, `insert-get-remove`, … | |
| `Tree::is_empty` | `Tree.IsEmpty` | match | `len-clear-is-empty`, `remove-absent`, `empty-db-reopen` | |
| `Tree::clear` | `Tree.Clear` | match | `len-clear-is-empty`, `clear-then-reopen` | a cleared tree stays cleared across a reopen |
| `Tree::name` | `Tree.Name` | differs | `tree-name-and-default`, `tree-names-sorted` | upstream returns `IVec` (bytes), the port a `string`; the runners hex-encode, and the *values* agree |
| `Tree::flush` | `DB.Flush` | match | `flush-then-reopen`, `batch-atomic-across-reopen`, `iter-survives-reopen`, … | upstream returns the number of bytes flushed, the port only an error; the flush *effect* is what is compared |
| `Tree::flush_async` | `DB.FlushAsync` | untested | — | `async fn` vs a `<-chan error`; not scriptable deterministically |
| `Tree::watch_prefix` | `Tree.Watch` | untested | — | event streams are excluded by design (nondeterministic to compare) |
| `Tree::checksum` | `Tree.Checksum` | untested | — | different digest construction |
| `Tree::verify_integrity` | — | missing | — | no analogue |
| `Tree::transaction` | `DB.Update` / `DB.View` | untested | — | closure-shaped transaction APIs with different conflict models; not encodable as a language-agnostic script here |
| `sled::CompareAndSwapError{current, proposed}` | `sled.CompareAndSwapError{Current, Proposed}` | match | `cas-sequence`, `cas-non-utf8` | field-for-field |

### `sled::Batch`, `sled::Iter`, `sled::IVec`

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Batch::default` | `DB.NewBatch` | match | all `batch-*` | |
| `Batch::insert` | `Batch.Set` | match | all `batch-*` | |
| `Batch::remove` | `Batch.Delete` | match | `batch-mixed`, `batch-same-key-twice`, `batch-remove-absent` | |
| `Iter: Iterator` | `sled.Iterator` (`Valid`/`Key`/`Value`/`Next`) | match | all `iteration` cases | |
| `Iter::keys` | — | missing | — | no keys-only iterator |
| `Iter::values` | — | missing | — | no values-only iterator |
| `sled::IVec` | `[]byte` | differs | all | the port uses plain byte slices; no inline/`Arc` value type |
| `IVec::subslice` | — | missing | — | |

### Errors, events, transactions

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `sled::Error` (`CollectionNotFound`, `Unsupported`, `ReportableBug`, `Io`, `Corruption`) | `sled.Err*` sentinels | untested | — | the harness compares *whether* a call failed, never message text (HARNESS rule); the sentinel sets do not correspond one-to-one |
| `sled::Event` (`Insert`/`Remove`) + `Event::key` | `sled.Event` / `sled.EventType` | untested | — | subscriber surface excluded |
| `Subscriber: Iterator` | `Subscriber.Next` / `Subscriber.Events` | untested | — | |
| `Subscriber::next_timeout` | — | missing | — | port has `TryNext`/`Drain` instead |
| `Subscriber::complete` | — | missing | — | no event-completion handshake |
| `sled::Transactional` (trait) | — | missing | — | port spans trees with a single `DB.Update` + `Tx.SetTree` instead of a tuple-of-trees trait |
| `transaction::TransactionalTree::insert` | `Tx.Set` / `Tx.SetTree` | untested | — | |
| `transaction::TransactionalTree::remove` | `Tx.Delete` / `Tx.DeleteTree` | untested | — | |
| `transaction::TransactionalTree::get` | `Tx.Get` / `Tx.GetTree` | untested | — | |
| `transaction::TransactionalTree::apply_batch` | — | missing | — | no batch-inside-transaction |
| `transaction::TransactionalTree::flush` | — | missing | — | port always commits durably |
| `transaction::TransactionalTree::generate_id` | — | missing | — | not available on `Tx` |
| `transaction::abort` | — | missing | — | port aborts by returning an error from the closure |
| `transaction::TransactionError` | — | missing | — | |
| `transaction::ConflictableTransactionError` | — | missing | — | |
| `transaction::UnabortableTransactionError` | — | missing | — | |

### Go-only surface (`extra`)

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| — | `DB.Compact` | extra | `compact-preserves-nonempty`, `compact-destroys-empty-tree`, `compact-after-many-overwrites` | compared against a no-op upstream (`Db::flush`), since compaction must be semantically invisible; **one case fails — see the bug section** |
| — | `Tree.GetGte` / `Tree.GetLte` | extra | `get-gte-lte` | compared against a `range()`-based emulation in the Rust runner |
| — | `Tree.PopMinInRange` / `Tree.PopMaxInRange` | extra | `pop-in-range` | compared against a `range()+remove()` emulation in the Rust runner |
| — | `Tree.CompareAndSwap` (bool form) | extra | — | `CompareAndSwapErr` is the upstream-shaped form and is the one scored |
| — | `DB.GetAndSet` / `DB.GetAndDelete` | extra | all `core` | value-returning aliases that restore the upstream `insert`/`remove` contract |
| — | `Tree.Batch` / `DB.Batch` / `Batch.Len` | extra | — | closure-scoped batch helpers |
| — | `sled.Range` / `sled.PrefixRange` | extra | all `iteration` | struct-shaped replacement for Rust `RangeBounds`; see divergence 3 |
| — | `sled.Option`, `WithSyncWrites`, `WithFileMode`, `WithTemporary` | extra | — | functional options replacing part of `Config` |
| — | `sled.DefaultTreeName` | extra | `tree-name-and-default` | exported constant for the `__sled__default` keyspace (upstream keeps it private) |
| — | `DB.Update` / `DB.View` / `sled.Tx` | extra | — | see the transaction rows above |
| — | `Subscriber.Drain` / `TryNext` / `Events` / `Close` | extra | — | subscriber surface excluded |
| — | `DB.Clear`, `DB.IsEmpty`, `DB.Path`, `DB.Checksum`, `DB.SizeOnDisk` | extra | `len-clear-is-empty`, `empty-db-reopen` | `Db`-level conveniences for the default tree (upstream reaches them through `Deref`) |
| — | `sled.ErrClosed`, `ErrEmptyKey`, `ErrTxClosed`, `ErrTxNotWritable`, `ErrEmptyTreeName`, `ErrDropDefaultTree`, `ErrNoMergeOperator`, `ErrNilFunc`, `ErrCorruptImport`, `ErrNoPath` | extra | `empty-key`, `drop-default-tree`, `open-tree-empty-name`, `merge-without-operator` | error identity is not compared, only failure vs success |

## Score

### Symbols

| status | count |
| --- | --- |
| `match` | 33 |
| `differs` | 8 |
| `missing` | 26 |
| `extra` | 13 |
| `untested` | 19 |
| **total rows** | **99** |

**Symbol parity = 33 / (33 + 8) = 80.5 %** over the 41 symbols actually compared.
The 26 `missing` symbols are almost entirely storage-engine configuration
(`Config` knobs for the page cache, compression, log segments, space
amplification) and the `Transactional`/`transaction::*` tuple-transaction surface,
neither of which the port models. The 19 `untested` symbols are the ones whose
outputs are inherently implementation-specific (`export`/`import` byte streams,
`checksum`, `size_on_disk`) or nondeterministic to compare (`watch_prefix`,
`Subscriber`, `flush_async`, `transaction`).

### Cases

| | |
| --- | --- |
| cases | **70** |
| match | **64** |
| mismatch | **1** (`compact-destroys-empty-tree`) |
| deviations | **5** (`empty-key`, `batch-empty-key`, `range-exclusive-lower`, `range-inclusive-upper`, `opened-but-unwritten-tree-survives-reopen`) |
| **case parity** | **98.46 %** (64 / 65, deviations excluded from the denominator) |

Per-group totals are regenerated into [`parity.json`](parity.json) by the test.
Every deviation is a genuine, documented API-shape difference — it is kept as a
case so that a future release which closes the gap is detected automatically. The
single mismatch is the `Compact()` bug and is a real defect, so
`GOWORK=off go test ./parity/sled/` fails today, by design.

