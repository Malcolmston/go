// Command sled-example exercises the published github.com/malcolmston/sled
// module: an embedded, transactional, append-only-log key/value store that
// ports the Rust `sled` database to Go.
//
// It is consumed as an outside user would consume it: a plain `go get` of the
// published module, no replace directive, no reference to the local checkout.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/malcolmston/sled"
)

// dbPath is filled in by main and reused by the reopen section.
var dbPath string

func main() {
	dir, err := os.MkdirTemp("", "sled-example-*")
	must(err)
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println("cleanup failed:", err)
			return
		}
		fmt.Printf("\n[cleanup] removed temp dir %s\n", dir)
	}()

	// Open takes the path of the write-ahead LOG FILE, not a directory.
	dbPath = filepath.Join(dir, "example.sled")

	section("00. OPEN")
	db, err := sled.Open(dbPath)
	must(err)
	fmt.Printf("opened     : %s\n", db.Path())
	fmt.Printf("recovered  : %v (false on a fresh database)\n", db.WasRecovered())
	fmt.Printf("empty      : %v, len=%d\n", db.IsEmpty(), db.Len())

	basics(db)
	compareAndSwap(db)
	readModifyWrite(db)
	batches(db)
	iteration(db)
	ordering(db)
	popping(db)
	trees(db)
	transactions(db)
	subscribers(db)
	mergeOperator(db)
	ids(db)
	exportImport(db, dir)
	concurrency(db)
	durability(db)

	// Snapshot expectations, then close and reopen to prove durability.
	before := dump(db)
	beforeSum := db.Checksum()
	beforeTrees := db.TreeNames()
	must(db.Close())

	reopen(before, beforeSum, beforeTrees)
	compaction()

	fmt.Println("\n=== ALL SECTIONS COMPLETED ===")
}

// ---------------------------------------------------------------- 01. basics

func basics(db *sled.DB) {
	section("01. INSERT / GET / HAS / DELETE")

	must(db.Set([]byte("greeting"), []byte("hello")))
	must(db.Set([]byte("farewell"), []byte("goodbye")))

	v, ok, err := db.Get([]byte("greeting"))
	must(err)
	fmt.Printf("get greeting        : %q ok=%v\n", v, ok)

	has, err := db.Has([]byte("farewell"))
	must(err)
	fmt.Printf("has farewell        : %v\n", has)

	// ContainsKey is a sled-parity alias for Has.
	has, err = db.ContainsKey([]byte("nope"))
	must(err)
	fmt.Printf("containsKey nope    : %v\n", has)

	_, ok, err = db.Get([]byte("missing"))
	must(err)
	fmt.Printf("get missing         : ok=%v (miss is not an error)\n", ok)

	// GetAndSet / GetAndDelete return the previous value atomically.
	old, existed, err := db.GetAndSet([]byte("greeting"), []byte("hi"))
	must(err)
	fmt.Printf("getAndSet greeting  : old=%q existed=%v\n", old, existed)

	old, existed, err = db.GetAndDelete([]byte("farewell"))
	must(err)
	fmt.Printf("getAndDelete farewell: old=%q existed=%v\n", old, existed)

	must(db.Delete([]byte("greeting")))
	has, err = db.Has([]byte("greeting"))
	must(err)
	fmt.Printf("after delete has    : %v, len=%d\n", has, db.Len())

	// Empty keys are rejected with a sentinel error.
	err = db.Set(nil, []byte("x"))
	fmt.Printf("empty key rejected  : %v (errors.Is ErrEmptyKey=%v)\n", err, errors.Is(err, sled.ErrEmptyKey))
}

// -------------------------------------------------------- 02. compare-and-swap

func compareAndSwap(db *sled.DB) {
	section("02. COMPARE-AND-SWAP")

	// nil `old` means "expect the key to be absent" -> acts as insert-if-absent.
	swapped, err := db.CompareAndSwap([]byte("cas:k"), nil, []byte("v1"))
	must(err)
	fmt.Printf("cas absent->v1      : swapped=%v\n", swapped)

	// Same call again must fail: the key now exists.
	swapped, err = db.CompareAndSwap([]byte("cas:k"), nil, []byte("v1-again"))
	must(err)
	fmt.Printf("cas absent->v1 again: swapped=%v (expected false)\n", swapped)

	swapped, err = db.CompareAndSwap([]byte("cas:k"), []byte("v1"), []byte("v2"))
	must(err)
	fmt.Printf("cas v1->v2          : swapped=%v\n", swapped)

	swapped, err = db.CompareAndSwap([]byte("cas:k"), []byte("WRONG"), []byte("v3"))
	must(err)
	fmt.Printf("cas WRONG->v3       : swapped=%v (failed compare is not an error)\n", swapped)

	// CompareAndSwapErr is the error-reporting form: on mismatch it hands back
	// the current and proposed values so a retry needs no extra read.
	err = db.CompareAndSwapErr([]byte("cas:k"), []byte("WRONG"), []byte("v3"))
	var casErr *sled.CompareAndSwapError
	if errors.As(err, &casErr) {
		fmt.Printf("casErr mismatch     : current=%q proposed=%q\n", casErr.Current, casErr.Proposed)
	} else {
		fmt.Printf("casErr mismatch     : UNEXPECTED err=%v\n", err)
	}

	must(db.CompareAndSwapErr([]byte("cas:k"), []byte("v2"), []byte("v3")))
	v, _, err := db.Get([]byte("cas:k"))
	must(err)
	fmt.Printf("casErr success      : value now %q\n", v)

	// A nil newValue means "delete".
	swapped, err = db.CompareAndSwap([]byte("cas:k"), []byte("v3"), nil)
	must(err)
	has, err := db.Has([]byte("cas:k"))
	must(err)
	fmt.Printf("cas v3->nil (delete): swapped=%v stillPresent=%v\n", swapped, has)
}

// ------------------------------------------------ 03. read-modify-write helpers

func readModifyWrite(db *sled.DB) {
	section("03. FETCH-AND-UPDATE / UPDATE-AND-FETCH")

	oldV, newV, err := db.FetchAndUpdate([]byte("rmw:counter"), func(old []byte) []byte {
		if old == nil {
			return []byte("1")
		}
		return []byte(string(old) + "+1")
	})
	must(err)
	fmt.Printf("fetchAndUpdate  #1  : old=%q new=%q\n", oldV, newV)

	oldV, newV, err = db.FetchAndUpdate([]byte("rmw:counter"), func(old []byte) []byte {
		return []byte(string(old) + "+1")
	})
	must(err)
	fmt.Printf("fetchAndUpdate  #2  : old=%q new=%q\n", oldV, newV)

	newV, err = db.UpdateAndFetch([]byte("rmw:counter"), func(old []byte) []byte {
		return bytes.ToUpper(old)
	})
	must(err)
	fmt.Printf("updateAndFetch      : new=%q\n", newV)

	// Returning nil from the callback deletes the key.
	newV, err = db.UpdateAndFetch([]byte("rmw:counter"), func(old []byte) []byte { return nil })
	must(err)
	has, err := db.Has([]byte("rmw:counter"))
	must(err)
	fmt.Printf("updateAndFetch nil  : new=%v stillPresent=%v (nil result deletes)\n", newV, has)

	if _, e := db.UpdateAndFetch([]byte("rmw:x"), nil); e != nil {
		fmt.Printf("nil callback        : %v (errors.Is ErrNilFunc=%v)\n", e, errors.Is(e, sled.ErrNilFunc))
	}
}

// --------------------------------------------------------------- 04. batches

func batches(db *sled.DB) {
	section("04. BATCHES (ATOMIC MULTI-WRITE)")

	// Chained staging on an explicit batch, then one atomic Commit.
	b := db.NewBatch()
	b.Set([]byte("batch:a"), []byte("1")).
		Set([]byte("batch:b"), []byte("2")).
		Set([]byte("batch:c"), []byte("3"))
	fmt.Printf("staged ops          : %d\n", b.Len())
	must(b.Commit())
	fmt.Printf("after commit len    : %d\n", db.Len())

	// A committed batch cannot be reused.
	err := b.Commit()
	fmt.Printf("reuse rejected      : %v (errors.Is ErrTxClosed=%v)\n", err, errors.Is(err, sled.ErrTxClosed))

	// Closure form: an error from fn discards the whole batch.
	sentinel := errors.New("caller changed their mind")
	err = db.Batch(func(b *sled.Batch) error {
		b.Set([]byte("batch:doomed"), []byte("x"))
		b.Delete([]byte("batch:a"))
		return sentinel
	})
	has, gerr := db.Has([]byte("batch:doomed"))
	must(gerr)
	stillA, gerr := db.Has([]byte("batch:a"))
	must(gerr)
	fmt.Printf("aborted batch       : err=%v doomedWritten=%v batch:a survived=%v\n", err == sentinel, has, stillA)

	// Successful closure form, mixing sets and deletes.
	must(db.Batch(func(b *sled.Batch) error {
		b.Set([]byte("batch:d"), []byte("4"))
		b.Delete([]byte("batch:c"))
		return nil
	}))
	fmt.Printf("mixed batch         : %v\n", dumpPrefix(db, "batch:"))
}

// ------------------------------------------------------------- 05. iteration

func iteration(db *sled.DB) {
	section("05. RANGE / PREFIX ITERATION")

	must(db.Batch(func(b *sled.Batch) error {
		for _, k := range []string{"user:alice", "user:bob", "user:carol", "user:dave", "zzz:tail", "aaa:head"} {
			b.Set([]byte(k), []byte(strings.ToUpper(k)))
		}
		return nil
	}))

	fmt.Printf("prefix user:        : %v\n", collect(db.ScanPrefix([]byte("user:"))))
	fmt.Printf("Range{Prefix}       : %v\n", collect(db.Scan(sled.Range{Prefix: []byte("user:")})))

	// Half-open bounds: Lower inclusive, Upper exclusive.
	fmt.Printf("[user:b, user:d)    : %v\n", collect(db.Scan(sled.Range{
		Lower: []byte("user:b"), Upper: []byte("user:d"),
	})))

	// Reverse iteration over the same bounds.
	fmt.Printf("prefix user: reverse: %v\n", collect(db.Scan(sled.Range{
		Prefix: []byte("user:"), Reverse: true,
	})))

	// PrefixRange composes a prefix filter with other Range fields.
	r := sled.PrefixRange([]byte("user:"))
	r.Lower = []byte("user:c")
	fmt.Printf("PrefixRange+Lower   : %v\n", collect(db.Scan(r)))

	// An empty range yields nothing rather than erroring.
	fmt.Printf("no-match prefix     : %v (len=%d)\n",
		collect(db.ScanPrefix([]byte("nosuch:"))), len(collect(db.ScanPrefix([]byte("nosuch:")))))
}

// -------------------------------------------------------- 06. ordered scanning

func ordering(db *sled.DB) {
	section("06. ORDERED KEY SCANS")

	t, err := db.OpenTree("ordered")
	must(err)

	// Insert 200 keys in scrambled order; a treap-backed index must still
	// return them in lexicographic byte order.
	must(t.Batch(func(b *sled.Batch) error {
		for i := 0; i < 200; i++ {
			n := (i * 137) % 200 // pseudo-shuffled insertion order
			b.Set([]byte(fmt.Sprintf("k:%04d", n)), []byte(fmt.Sprintf("v%d", n)))
		}
		return nil
	}))

	var got []string
	for it := t.Scan(sled.Range{}); it.Valid(); it.Next() {
		got = append(got, string(it.Key()))
	}
	sorted := sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] })
	fmt.Printf("inserted 200 shuffled, scanned %d keys, ascending sorted=%v\n", len(got), sorted)
	fmt.Printf("first 3 / last 3    : %v ... %v\n", got[:3], got[len(got)-3:])

	// Reverse must be the exact mirror.
	var rev []string
	for it := t.Scan(sled.Range{Reverse: true}); it.Valid(); it.Next() {
		rev = append(rev, string(it.Key()))
	}
	mirrored := len(rev) == len(got)
	for i := range rev {
		if rev[i] != got[len(got)-1-i] {
			mirrored = false
			break
		}
	}
	fmt.Printf("reverse is mirror   : %v\n", mirrored)

	// Endpoint and neighbor lookups.
	k, v, ok := t.First()
	fmt.Printf("First()             : %q=%q ok=%v\n", k, v, ok)
	k, v, ok = t.Last()
	fmt.Printf("Last()              : %q=%q ok=%v\n", k, v, ok)
	k, _, ok = t.GetGt([]byte("k:0100"))
	fmt.Printf("GetGt  k:0100       : %q ok=%v\n", k, ok)
	k, _, ok = t.GetGte([]byte("k:0100"))
	fmt.Printf("GetGte k:0100       : %q ok=%v\n", k, ok)
	k, _, ok = t.GetLt([]byte("k:0100"))
	fmt.Printf("GetLt  k:0100       : %q ok=%v\n", k, ok)
	k, _, ok = t.GetLte([]byte("k:0100"))
	fmt.Printf("GetLte k:0100       : %q ok=%v\n", k, ok)
	_, _, ok = t.GetGt([]byte("k:9999"))
	fmt.Printf("GetGt past end      : ok=%v\n", ok)
}

// ---------------------------------------------------------------- 07. popping

func popping(db *sled.DB) {
	section("07. POP MIN / MAX (QUEUE-STYLE CONSUMPTION)")

	t, err := db.OpenTree("queue")
	must(err)
	must(t.Batch(func(b *sled.Batch) error {
		for i := 1; i <= 6; i++ {
			b.Set([]byte(fmt.Sprintf("job:%02d", i)), []byte(fmt.Sprintf("payload-%d", i)))
		}
		return nil
	}))

	k, v, ok, err := t.PopMin()
	must(err)
	fmt.Printf("PopMin              : %q=%q ok=%v\n", k, v, ok)
	k, v, ok, err = t.PopMax()
	must(err)
	fmt.Printf("PopMax              : %q=%q ok=%v\n", k, v, ok)

	k, v, ok, err = t.PopMinInRange(sled.Range{Lower: []byte("job:04")})
	must(err)
	fmt.Printf("PopMinInRange >=04  : %q=%q ok=%v\n", k, v, ok)
	k, v, ok, err = t.PopMaxInRange(sled.Range{Upper: []byte("job:04")})
	must(err)
	fmt.Printf("PopMaxInRange <04   : %q=%q ok=%v\n", k, v, ok)

	fmt.Printf("remaining queue     : %v\n", collect(t.Scan(sled.Range{})))

	// Popping an empty range reports ok=false rather than erroring.
	_, _, ok, err = t.PopMinInRange(sled.Range{Prefix: []byte("nosuch")})
	must(err)
	fmt.Printf("pop empty range     : ok=%v\n", ok)
}

// ----------------------------------------------------------------- 08. trees

func trees(db *sled.DB) {
	section("08. MULTIPLE TREES / KEYSPACES")

	users, err := db.OpenTree("users")
	must(err)
	sessions, err := db.OpenTree("sessions")
	must(err)

	must(users.Set([]byte("shared"), []byte("from-users")))
	must(sessions.Set([]byte("shared"), []byte("from-sessions")))
	must(db.Set([]byte("shared"), []byte("from-default")))

	uv, _, err := users.Get([]byte("shared"))
	must(err)
	sv, _, err := sessions.Get([]byte("shared"))
	must(err)
	dv, _, err := db.Get([]byte("shared"))
	must(err)
	fmt.Printf("same key, 3 trees   : users=%q sessions=%q default=%q\n", uv, sv, dv)
	fmt.Printf("tree names          : %v\n", db.TreeNames())
	fmt.Printf("users.Name()        : %q  isEmpty=%v len=%d\n", users.Name(), users.IsEmpty(), users.Len())

	// The default tree is exposed under a reserved sentinel name and cannot be
	// dropped.
	_, err = db.DropTree(sled.DefaultTreeName)
	fmt.Printf("drop default tree   : %v (errors.Is ErrDropDefaultTree=%v)\n", err, errors.Is(err, sled.ErrDropDefaultTree))

	_, err = db.OpenTree("")
	fmt.Printf("open empty name     : %v (errors.Is ErrEmptyTreeName=%v)\n", err, errors.Is(err, sled.ErrEmptyTreeName))

	// Create, clear and drop a scratch tree.
	scratch, err := db.OpenTree("scratch")
	must(err)
	must(scratch.Set([]byte("a"), []byte("1")))
	must(scratch.Set([]byte("b"), []byte("2")))
	fmt.Printf("scratch before clear: len=%d\n", scratch.Len())
	must(scratch.Clear())
	fmt.Printf("scratch after clear : len=%d isEmpty=%v\n", scratch.Len(), scratch.IsEmpty())

	dropped, err := db.DropTree("scratch")
	must(err)
	fmt.Printf("dropped scratch     : %v, names=%v\n", dropped, db.TreeNames())
	dropped, err = db.DropTree("never-existed")
	must(err)
	fmt.Printf("drop nonexistent    : %v (no error)\n", dropped)
}

// ---------------------------------------------------------- 09. transactions

func transactions(db *sled.DB) {
	section("09. TRANSACTIONS (UPDATE / VIEW, CROSS-TREE ATOMICITY)")

	accounts, err := db.OpenTree("accounts")
	must(err)
	ledger, err := db.OpenTree("ledger")
	must(err)
	must(accounts.Set([]byte("alice"), []byte("100")))
	must(accounts.Set([]byte("bob"), []byte("50")))

	// A committing write transaction spanning two trees.
	must(db.Update(func(tx *sled.Tx) error {
		if err := tx.SetTree(accounts, []byte("alice"), []byte("90")); err != nil {
			return err
		}
		if err := tx.SetTree(accounts, []byte("bob"), []byte("60")); err != nil {
			return err
		}
		if err := tx.SetTree(ledger, []byte("txn:0001"), []byte("alice->bob 10")); err != nil {
			return err
		}
		// Reads inside the transaction see its own buffered writes.
		v, _, err := tx.GetTree(accounts, []byte("alice"))
		if err != nil {
			return err
		}
		fmt.Printf("read-own-write      : alice=%q while still uncommitted\n", v)
		return nil
	}))
	a, _, _ := accounts.Get([]byte("alice"))
	bb, _, _ := accounts.Get([]byte("bob"))
	fmt.Printf("committed           : alice=%q bob=%q ledger=%v\n", a, bb, collect(ledger.Scan(sled.Range{})))

	// A rolled-back transaction leaves nothing behind, in either tree.
	rollback := errors.New("insufficient funds")
	err = db.Update(func(tx *sled.Tx) error {
		_ = tx.SetTree(accounts, []byte("alice"), []byte("0"))
		_ = tx.SetTree(ledger, []byte("txn:0002"), []byte("bogus"))
		_ = tx.DeleteTree(accounts, []byte("bob"))
		return rollback
	})
	a, _, _ = accounts.Get([]byte("alice"))
	bobStill, _ := accounts.Has([]byte("bob"))
	ghost, _ := ledger.Has([]byte("txn:0002"))
	fmt.Printf("rolled back         : err=%v alice=%q bobPresent=%v ledgerGhost=%v\n",
		err == rollback, a, bobStill, ghost)

	// Read-only transaction: a stable snapshot, and writes are refused.
	must(db.View(func(tx *sled.Tx) error {
		fmt.Printf("view writable       : %v\n", tx.Writable())
		v, ok, err := tx.GetTree(accounts, []byte("alice"))
		if err != nil {
			return err
		}
		fmt.Printf("view get alice      : %q ok=%v\n", v, ok)
		werr := tx.SetTree(accounts, []byte("alice"), []byte("999"))
		fmt.Printf("view write refused  : %v (errors.Is ErrTxNotWritable=%v)\n",
			werr, errors.Is(werr, sled.ErrTxNotWritable))
		// Scans inside a transaction are supported too.
		fmt.Printf("view scan accounts  : %v\n", collect(tx.ScanTree(accounts, sled.Range{})))
		return nil
	}))

	// Tx also has default-tree shorthands and a scan that sees buffered writes.
	must(db.Update(func(tx *sled.Tx) error {
		if err := tx.Set([]byte("tx:short"), []byte("yes")); err != nil {
			return err
		}
		has, err := tx.Has([]byte("tx:short"))
		if err != nil {
			return err
		}
		seen := collect(tx.Scan(sled.Range{Prefix: []byte("tx:")}))
		fmt.Printf("tx default-tree     : has=%v scanSeesBuffered=%v\n", has, seen)
		return nil
	}))

	// A transaction handle must not outlive its callback.
	var escaped *sled.Tx
	must(db.Update(func(tx *sled.Tx) error { escaped = tx; return nil }))
	_, _, err = escaped.Get([]byte("tx:short"))
	fmt.Printf("escaped tx handle   : %v (errors.Is ErrTxClosed=%v)\n", err, errors.Is(err, sled.ErrTxClosed))
}

// ---------------------------------------------------------- 10. subscribers

func subscribers(db *sled.DB) {
	section("10. SUBSCRIBERS (WATCH A PREFIX)")

	watched, err := db.OpenTree("watched")
	must(err)
	sub := watched.Watch([]byte("evt:"))
	defer sub.Close()

	must(watched.Set([]byte("evt:1"), []byte("a")))   // insert
	must(watched.Set([]byte("evt:1"), []byte("b")))   // update
	must(watched.Delete([]byte("evt:1")))             // delete
	must(watched.Set([]byte("other:1"), []byte("c"))) // filtered out by prefix

	for _, ev := range sub.Drain(0) {
		fmt.Printf("event               : type=%-6s tree=%q key=%q value=%q\n",
			ev.Type, ev.Tree, ev.Key, ev.Value)
	}

	// TryNext on a drained subscriber does not block.
	_, ok := sub.TryNext()
	fmt.Printf("TryNext when empty  : ok=%v (non-blocking)\n", ok)

	// Next() returns ok=false once the subscriber is closed and drained.
	sub2 := watched.Watch([]byte("evt:"))
	must(watched.Set([]byte("evt:2"), []byte("z")))
	ev, ok := sub2.Next()
	fmt.Printf("Next()              : key=%q ok=%v\n", ev.Key, ok)
	sub2.Close()
	sub2.Close() // Close is idempotent.
	_, ok = sub2.Next()
	fmt.Printf("Next after close    : ok=%v\n", ok)
}

// -------------------------------------------------------- 11. merge operator

func mergeOperator(db *sled.DB) {
	section("11. MERGE OPERATOR")

	counters, err := db.OpenTree("counters")
	must(err)

	// Merge without an operator installed is a clean error, not a panic.
	_, err = counters.Merge([]byte("hits"), []byte("1"))
	fmt.Printf("merge w/o operator  : %v (errors.Is ErrNoMergeOperator=%v)\n",
		err, errors.Is(err, sled.ErrNoMergeOperator))

	// Install an append-style operator.
	counters.SetMergeOperator(func(key, oldValue, mergeValue []byte) []byte {
		if oldValue == nil {
			return append([]byte{}, mergeValue...)
		}
		return append(append(append([]byte{}, oldValue...), ','), mergeValue...)
	})
	for _, s := range []string{"a", "b", "c"} {
		merged, err := counters.Merge([]byte("trail"), []byte(s))
		must(err)
		fmt.Printf("merge %q            : %q\n", s, merged)
	}

	// A merge operator returning nil deletes the key.
	counters.SetMergeOperator(func(key, oldValue, mergeValue []byte) []byte { return nil })
	merged, err := counters.Merge([]byte("trail"), []byte("x"))
	must(err)
	has, err := counters.Has([]byte("trail"))
	must(err)
	fmt.Printf("merge -> nil        : merged=%v stillPresent=%v\n", merged, has)
}

// ------------------------------------------------------------------- 12. ids

func ids(db *sled.DB) {
	section("12. MONOTONIC ID GENERATION")

	var first, last uint64
	monotonic := true
	prev := uint64(0)
	for i := 0; i < 2000; i++ { // crosses the 1024-ID durable reservation block
		id, err := db.GenerateID()
		must(err)
		if i == 0 {
			first = id
		} else if id <= prev {
			monotonic = false
		}
		prev = id
		last = id
	}
	fmt.Printf("2000 ids            : first=%d last=%d strictlyIncreasing=%v\n", first, last, monotonic)
}

// --------------------------------------------------------- 13. export/import

func exportImport(db *sled.DB, dir string) {
	section("13. EXPORT / IMPORT")

	var buf bytes.Buffer
	must(db.Export(&buf))
	fmt.Printf("exported            : %d bytes\n", buf.Len())

	// Import into a second, independent database.
	other, err := sled.Open(filepath.Join(dir, "imported.sled"))
	must(err)
	must(other.Import(bytes.NewReader(buf.Bytes())))
	fmt.Printf("imported db         : len(default)=%d trees=%d\n", other.Len(), len(other.TreeNames()))
	fmt.Printf("checksums match     : %v (src=%d dst=%d)\n",
		db.Checksum() == other.Checksum(), db.Checksum(), other.Checksum())
	must(other.Close())

	// A corrupt stream is rejected wholesale.
	corrupt := append([]byte{}, buf.Bytes()...)
	corrupt[len(corrupt)-1] ^= 0xFF
	third, err := sled.Open(filepath.Join(dir, "corrupt.sled"))
	must(err)
	err = third.Import(bytes.NewReader(corrupt))
	fmt.Printf("corrupt import      : %v (errors.Is ErrCorruptImport=%v, len=%d)\n",
		err != nil, errors.Is(err, sled.ErrCorruptImport), third.Len())
	must(third.Close())
}

// ---------------------------------------------------------- 14. concurrency

func concurrency(db *sled.DB) {
	section("14. CONCURRENT ACCESS FROM MANY GOROUTINES")

	conc, err := db.OpenTree("concurrent")
	must(err)

	const writers, perWriter, readers = 8, 250, 4

	var readerWG, writerWG sync.WaitGroup
	stop := make(chan struct{})

	// Readers scan snapshots continuously while writers mutate. A torn or
	// partially-visible value would show up as a mismatch.
	var scans, mismatches int64
	var mu sync.Mutex
	for r := 0; r < readers; r++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			local, bad := 0, 0
			for {
				select {
				case <-stop:
					mu.Lock()
					scans += int64(local)
					mismatches += int64(bad)
					mu.Unlock()
					return
				default:
				}
				for it := conc.Scan(sled.Range{Prefix: []byte("w:")}); it.Valid(); it.Next() {
					// Every value is the key with "val-" prepended.
					if !bytes.Equal(it.Value(), append([]byte("val-"), it.Key()...)) {
						bad++
					}
					local++
				}
			}
		}()
	}

	// Writers: plain sets on a shared tree.
	for w := 0; w < writers; w++ {
		writerWG.Add(1)
		go func(w int) {
			defer writerWG.Done()
			for i := 0; i < perWriter; i++ {
				k := []byte(fmt.Sprintf("w:%02d:%04d", w, i))
				must(conc.Set(k, append([]byte("val-"), k...)))
			}
		}(w)
	}

	// Contending compare-and-swap increments on one hot key: exactly
	// writers*perWriter successes must land, with no lost updates.
	must(conc.Set([]byte("hot"), []byte("0")))
	var casGoroutines sync.WaitGroup
	var casRetries int64
	var casMu sync.Mutex
	for w := 0; w < writers; w++ {
		casGoroutines.Add(1)
		go func() {
			defer casGoroutines.Done()
			retries := 0
			for i := 0; i < perWriter; i++ {
				for {
					cur, _, err := conc.Get([]byte("hot"))
					must(err)
					var n int
					fmt.Sscanf(string(cur), "%d", &n)
					swapped, err := conc.CompareAndSwap([]byte("hot"), cur, []byte(fmt.Sprintf("%d", n+1)))
					must(err)
					if swapped {
						break
					}
					retries++
				}
			}
			casMu.Lock()
			casRetries += int64(retries)
			casMu.Unlock()
		}()
	}

	// Concurrent transactions on their own keys, to exercise the writer slot.
	var txWG sync.WaitGroup
	for w := 0; w < 4; w++ {
		txWG.Add(1)
		go func(w int) {
			defer txWG.Done()
			for i := 0; i < 50; i++ {
				must(db.Update(func(tx *sled.Tx) error {
					return tx.SetTree(conc, []byte(fmt.Sprintf("tx:%d:%d", w, i)), []byte("t"))
				}))
			}
		}(w)
	}

	// Let every writer finish before telling the readers to stop, so the final
	// counts are taken against a quiesced database.
	writerWG.Wait()
	casGoroutines.Wait()
	txWG.Wait()
	close(stop)
	readerWG.Wait()

	got := len(collect(conc.Scan(sled.Range{Prefix: []byte("w:")})))
	hot, _, err := conc.Get([]byte("hot"))
	must(err)
	txCount := len(collect(conc.Scan(sled.Range{Prefix: []byte("tx:")})))

	fmt.Printf("writer goroutines   : %d x %d writes -> %d distinct keys (want %d) ok=%v\n",
		writers, perWriter, got, writers*perWriter, got == writers*perWriter)
	fmt.Printf("CAS counter         : %q (want %d) ok=%v, retries=%d\n",
		hot, writers*perWriter, string(hot) == fmt.Sprint(writers*perWriter), casRetries)
	fmt.Printf("concurrent txns     : %d keys (want 200) ok=%v\n", txCount, txCount == 200)
	fmt.Printf("reader scans        : %d entries read concurrently, value mismatches=%d\n", scans, mismatches)
}

// ----------------------------------------------------------- 15. durability

func durability(db *sled.DB) {
	section("15. FLUSH / DURABILITY")

	must(db.Set([]byte("durable:1"), []byte("v")))
	must(db.Flush())
	fmt.Println("Flush()             : ok (default mode already fsyncs each commit)")

	must(db.Set([]byte("durable:2"), []byte("v")))
	if err := <-db.FlushAsync(); err != nil {
		fmt.Println("FlushAsync()        :", err)
	} else {
		fmt.Println("FlushAsync()        : ok")
	}

	size, err := db.SizeOnDisk()
	must(err)
	fmt.Printf("SizeOnDisk          : %d bytes of append-only log\n", size)
	fmt.Printf("Checksum            : %d (content hash over all trees)\n", db.Checksum())
}

// -------------------------------------------------------------- 16. reopen

func reopen(before map[string]string, beforeSum uint32, beforeTrees []string) {
	section("16. REOPEN AND VERIFY SURVIVAL")

	db, err := sled.Open(dbPath)
	must(err)
	defer func() { must(db.Close()) }()

	fmt.Printf("WasRecovered        : %v (true after reopening an existing log)\n", db.WasRecovered())

	after := dump(db)
	fmt.Printf("default tree keys   : before=%d after=%d\n", len(before), len(after))

	lost, changed := 0, 0
	for k, v := range before {
		av, ok := after[k]
		switch {
		case !ok:
			lost++
			if lost <= 3 {
				fmt.Printf("  LOST key          : %q\n", k)
			}
		case av != v:
			changed++
			if changed <= 3 {
				fmt.Printf("  CHANGED key       : %q %q -> %q\n", k, v, av)
			}
		}
	}
	fmt.Printf("data survived       : lost=%d changed=%d extra=%d\n", lost, changed, len(after)-len(before)+lost)

	sort.Strings(beforeTrees)
	afterTrees := db.TreeNames()
	sort.Strings(afterTrees)
	fmt.Printf("trees survived      : %v (match=%v)\n", afterTrees, strings.Join(beforeTrees, ",") == strings.Join(afterTrees, ","))
	fmt.Printf("checksum stable     : %v (before=%d after=%d)\n", beforeSum == db.Checksum(), beforeSum, db.Checksum())

	// Named-tree data and ordering must also survive.
	ordered, err := db.OpenTree("ordered")
	must(err)
	keys := collect(ordered.Scan(sled.Range{}))
	stillSorted := sort.SliceIsSorted(keys, func(i, j int) bool { return keys[i] < keys[j] })
	fmt.Printf("tree 'ordered'      : %d keys, ascending=%v\n", len(keys), stillSorted)

	accounts, err := db.OpenTree("accounts")
	must(err)
	a, _, err := accounts.Get([]byte("alice"))
	must(err)
	fmt.Printf("tree 'accounts'     : alice=%q (committed txn value)\n", a)

	// Dropped trees must stay dropped.
	present := false
	for _, n := range afterTrees {
		if n == "scratch" {
			present = true
		}
	}
	fmt.Printf("dropped tree gone   : %v\n", !present)

	// GenerateID keeps advancing across the restart (no reuse).
	id, err := db.GenerateID()
	must(err)
	fmt.Printf("GenerateID after reopen: %d (>= pre-close high-water mark)\n", id)

	// Operations on a closed DB must fail cleanly.
	tmp, err := sled.Open(filepath.Join(filepath.Dir(dbPath), "closed.sled"))
	must(err)
	must(tmp.Close())
	err = tmp.Set([]byte("k"), []byte("v"))
	fmt.Printf("write after Close   : %v (errors.Is ErrClosed=%v)\n", err, errors.Is(err, sled.ErrClosed))
}

// ----------------------------------------------------------- 17. compaction

func compaction() {
	section("17. COMPACTION (LOG REWRITE)")

	db, err := sled.Open(dbPath)
	must(err)

	// Churn the same keys so the append-only log accumulates dead records.
	for round := 0; round < 40; round++ {
		must(db.Batch(func(b *sled.Batch) error {
			for i := 0; i < 50; i++ {
				b.Set([]byte(fmt.Sprintf("churn:%03d", i)), []byte(strings.Repeat("x", 64)))
			}
			return nil
		}))
	}
	for i := 0; i < 50; i++ {
		must(db.Delete([]byte(fmt.Sprintf("churn:%03d", i))))
	}

	// Deliberately leave one tree with zero live keys, to probe whether an empty
	// keyspace survives a compaction (see the HOLE below).
	empty, err := db.OpenTree("empty-after-churn")
	must(err)
	must(empty.Set([]byte("temp"), []byte("v")))
	must(empty.Delete([]byte("temp")))

	beforeSize, err := db.SizeOnDisk()
	must(err)
	beforeSum := db.Checksum()
	beforeLen := db.Len()
	beforeTrees := db.TreeNames()
	sort.Strings(beforeTrees)

	must(db.Compact())

	afterSize, err := db.SizeOnDisk()
	must(err)
	fmt.Printf("log size            : %d -> %d bytes (shrank=%v)\n", beforeSize, afterSize, afterSize < beforeSize)
	fmt.Printf("in-memory preserved : len %d -> %d, checksum stable=%v\n", beforeLen, db.Len(), beforeSum == db.Checksum())
	must(db.Close())

	// The compaction result must itself be recoverable.
	db, err = sled.Open(dbPath)
	must(err)
	afterTrees := db.TreeNames()
	sort.Strings(afterTrees)
	fmt.Printf("reopen post-compact : len=%d keyDataIntact=%v\n", db.Len(), db.Len() == beforeLen)

	// HOLE: Compact() writes only Set records for live keys, so a tree whose
	// live key set is empty leaves no trace in the rewritten log and its
	// existence is LOST on the next Open. TreeNames() shrinks and Checksum()
	// changes across compact+reopen, even though no key/value data was lost.
	// Without a compaction the same empty tree does survive a reopen (see
	// section 16, where the empty "counters" tree came back).
	fmt.Printf("trees before compact: %v\n", beforeTrees)
	fmt.Printf("trees after reopen  : %v\n", afterTrees)
	if len(afterTrees) != len(beforeTrees) {
		fmt.Printf("HOLE                : %d empty tree(s) lost by compact+reopen; checksum stable=%v\n",
			len(beforeTrees)-len(afterTrees), beforeSum == db.Checksum())
	} else {
		fmt.Printf("empty tree survived : yes, checksum stable=%v\n", beforeSum == db.Checksum())
	}
	must(db.Close())
}

// ------------------------------------------------------------------ helpers

func section(title string) {
	fmt.Printf("\n=== %s %s\n", title, strings.Repeat("=", max(0, 60-len(title))))
}

func collect(it *sled.Iterator) []string {
	var out []string
	for ; it.Valid(); it.Next() {
		out = append(out, string(it.Key()))
	}
	return out
}

func dumpPrefix(db *sled.DB, prefix string) []string {
	var out []string
	for it := db.ScanPrefix([]byte(prefix)); it.Valid(); it.Next() {
		out = append(out, fmt.Sprintf("%s=%s", it.Key(), it.Value()))
	}
	return out
}

func dump(db *sled.DB) map[string]string {
	m := make(map[string]string)
	for it := db.Scan(sled.Range{}); it.Valid(); it.Next() {
		m[string(it.Key())] = string(it.Value())
	}
	return m
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
