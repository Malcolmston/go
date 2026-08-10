# sled example — validating `github.com/malcolmston/sled`

A single runnable program that exercises the published `sled` Go module (a port of the
Rust `sled` embedded key/value database) the way an outside consumer would: plain
`go get` of the published module, **no `replace` directive**, no reference to the local
`../../sled` checkout.

## Resolved module version

```
github.com/malcolmston/sled v0.0.0-20260719021433-e796e0b0b783
```

The repo carries no semver tags, so `@latest` resolves to a pseudo-version.
The library has **zero third-party dependencies** (`go.mod` declares only `go 1.24.7`),
so `go.sum` contains just the one module.

## How to run

```sh
cd examples/sled
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .

# recommended: the program is heavily concurrent
GOWORK=off go run -race .
```

`GOWORK=off` is required so the repo-root `go.work` does not shadow the published
module with the local working tree. The program creates everything under
`os.MkdirTemp`, removes it on exit, terminates on its own, and does no network I/O
beyond the initial module download.

## What it demonstrates

Seventeen labeled sections, in order:

| # | Section | API exercised |
|---|---------|---------------|
| 00 | Open | `Open`, `DB.Path`, `WasRecovered`, `IsEmpty`, `Len` |
| 01 | Insert / get / delete | `Set`, `Get`, `Has`, `ContainsKey`, `Delete`, `GetAndSet`, `GetAndDelete`, `ErrEmptyKey` |
| 02 | Compare-and-swap | `CompareAndSwap`, `CompareAndSwapErr`, `*CompareAndSwapError{Current,Proposed}`, nil-old = insert-if-absent, nil-new = delete |
| 03 | Read-modify-write | `FetchAndUpdate`, `UpdateAndFetch`, `ErrNilFunc` |
| 04 | Batches | `NewBatch`, chained `Set`/`Delete`, `Len`, `Commit`, `DB.Batch` closure form, abort-discards-everything, reuse rejected |
| 05 | Range / prefix iteration | `Scan`, `Range{Lower,Upper,Prefix,Reverse}`, `ScanPrefix`, `PrefixRange` |
| 06 | Ordered key scans | 200 keys inserted shuffled, verified ascending + reverse-is-exact-mirror; `First`, `Last`, `GetGt`, `GetGte`, `GetLt`, `GetLte` |
| 07 | Pop min/max | `PopMin`, `PopMax`, `PopMinInRange`, `PopMaxInRange` |
| 08 | Multiple trees | `OpenTree`, `TreeNames`, `Tree.Name`, same key isolated across 3 trees, `Clear`, `DropTree`, `ErrDropDefaultTree`, `ErrEmptyTreeName` |
| 09 | Transactions | `Update`, `View`, `Tx.Set/Get/Has/Delete/Scan`, `SetTree/GetTree/DeleteTree/ScanTree`, cross-tree atomic commit, rollback, read-own-writes, `ErrTxNotWritable`, `ErrTxClosed` |
| 10 | Subscribers | `Watch`, `Events`/`Next`/`TryNext`/`Drain`, `EventInsert`/`EventUpdate`/`EventDelete`, prefix filtering, idempotent `Close` |
| 11 | Merge operator | `SetMergeOperator`, `Merge`, `ErrNoMergeOperator`, nil-result deletes |
| 12 | ID generation | `GenerateID` across 2000 IDs (crosses the 1024-ID durable reservation block) |
| 13 | Export / import | `Export`, `Import`, checksum equality against a second DB, `ErrCorruptImport` on a flipped byte |
| 14 | Concurrency | 8 writer goroutines x 250 writes, 8 goroutines doing contended CAS increments on one hot key, 4 goroutines running `Update` transactions, 4 reader goroutines scanning snapshots throughout |
| 15 | Flush / durability | `Flush`, `FlushAsync`, `SizeOnDisk`, `Checksum` |
| 16 | Reopen | close, reopen, diff every key/value, tree list, checksum, dropped-tree absence, ID monotonicity, `ErrClosed` |
| 17 | Compaction | `Compact`, log shrink, data preservation, recoverability of the compacted log |

### Verified results

Everything above passes. Notably:

- **Durability**: after `Close` + `Open`, 0 keys lost, 0 changed, 0 extra; all 10 trees
  returned; `Checksum()` byte-identical; a dropped tree stayed dropped; `GenerateID`
  resumed past its pre-close high-water mark.
- **Ordering**: 200 keys inserted in pseudo-shuffled order scan back in exact
  lexicographic byte order, and reverse iteration is the precise mirror. Ordering also
  survives a reopen.
- **Atomicity**: an aborted batch and a rolled-back cross-tree transaction both left
  zero trace, in memory and on disk.
- **Concurrency**: no lost updates. 2000 writes from 8 goroutines produced exactly 2000
  keys; ~6.8k contended CAS retries converged on exactly 2000; 200 concurrent
  transactions all landed. Reader goroutines performed >100M snapshot reads during the
  writes with **zero** torn or inconsistent values, and `go run -race` reports **no data
  races**.
- **Compaction**: log shrank 314 KB -> 81 KB with all key/value data intact.

## Holes found

### 1. Empty trees are silently destroyed by `Compact()` + reopen (real bug)

`Compact()` rewrites the log as nothing but `Set` records for currently-live keys
(`compact.go`). A tree whose live key set happens to be empty therefore leaves **no
trace at all** in the rewritten log, and its existence is lost on the next `Open()`.

Observed in section 17: `TreeNames()` went from 11 trees to 9 across compact+reopen,
and `Checksum()` — which incorporates tree names — changed. No key/value data is lost,
but keyspace *existence* is. Without a compaction the same empty tree does survive a
reopen (section 16 correctly recovers the empty `counters` tree), so `Compact()` is not
durability-neutral, contradicting its doc comment's framing as a pure space reclamation.

Minimal reproduction:

```go
db, _ := sled.Open(p)
db.Set([]byte("keep"), []byte("v"))
e, _ := db.OpenTree("emptytree")
e.Set([]byte("a"), []byte("1"))
e.Delete([]byte("a"))          // tree now exists but is empty
db.Close()

db, _ = sled.Open(p)
db.Compact()
db.Close()

db, _ = sled.Open(p)
fmt.Println(db.TreeNames())    // "emptytree" is GONE
```

A fix would need `Compact()` to emit a tree-registration record for every known tree,
not just for trees that contribute key records. This is flagged with a `// HOLE:` comment
in `main.go`; the example still exercises `Compact()` and reports the discrepancy at
runtime rather than commenting the call out.

Everything else below is API friction rather than incorrect behavior — nothing else had
to be commented out, and no API named in the library's own README was missing.

### 2. `Open` takes a log *file* path, not a directory

Rust `sled::open` takes a directory. Here `Open("data.sled")` creates a single
append-only WAL file (plus a transient `.compact` sibling). Passing a path you intended
as a directory silently creates a file instead. Worth calling out prominently; it is the
first thing an outside user gets wrong.

### 3. The whole dataset must fit in RAM

The index is an in-memory treap and `Open` replays the entire log from byte zero to
rebuild it, so both memory use and startup time are O(total live data). This is a real
architectural divergence from `sled`, which is a disk-resident B-tree with a page cache.
`doc.go` describes the in-memory index honestly, but neither it nor the README states
the resulting capacity ceiling.

### 4. `DefaultTreeName` sentinel leaks into the public API

`TreeNames()` returns the internal sentinel `"__sled__default"` alongside real user tree
names, so callers must filter a magic double-underscore string out of what looks like a
list of their own keyspaces.

### 5. Inconsistent error returns on ordered lookups

`Get` returns `(value []byte, ok bool, err error)`, but `First`, `Last`, `GetGt`,
`GetGte`, `GetLt`, `GetLte` return `(key, value []byte, ok bool)` with **no error**.
They cannot report `ErrClosed`, so on a closed DB they silently return `ok=false` —
indistinguishable from an empty tree.

### 6. `Batch` defers errors to `Commit`

`b.Set(...)` returns `*Batch` for chaining and stashes any error internally, so an
invalid key is not visible until `Commit()`. Idiomatic Go would return the error at the
call site. Also, `Batch` is created from a `DB` (`db.NewBatch()`) but is applied to a
keyspace (`tree.ApplyBatch(b)`), and a single batch cannot span multiple trees — for
cross-tree atomicity you must use `Update`. The split between `DB.Batch`, `Tree.Batch`,
`Batch.Commit` and `Tree.ApplyBatch` takes a while to untangle.

### 7. Callback-under-writer-lock footgun

`FetchAndUpdate`, `UpdateAndFetch` and `MergeOperator` callbacks all run while the single
writer slot is held. Calling back into the DB from inside one self-deadlocks. This is
documented but not detectable or enforced.

### 8. `Iterator` has no `Close` and no `Error`

An `Iterator` pins the snapshot root it was created from, so a long-lived iterator
retains the entire index version it observed with no way to release it early. There is
also no way for an iterator to surface an error.

### 9. Silent event loss for slow subscribers

`Subscriber` delivery is non-blocking over a fixed 1024-entry buffer; a subscriber that
falls behind loses events with no signal that a gap occurred. Documented, but it makes
`Watch` unusable for anything requiring a complete change log.

### 10. `Checksum()` returns no error

`db.Checksum() uint32` has no error return and no closed-DB check, unlike its neighbor
`SizeOnDisk() (uint64, error)`. On a closed DB it returns a stale value rather than
`ErrClosed`.
