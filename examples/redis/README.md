# github.com/malcolmston/redis — example program

A single runnable program that exercises the `github.com/malcolmston/redis` port
end to end, consumed as a published Go module (no `replace` directive, no
reference to the sibling working tree).

## What this library actually is

**Both an embedded store and a server — but no client.**

- `redis.New()` returns a `*redis.Store`: a thread-safe, in-process Redis-style
  keyspace with a typed Go API (`Set`, `LPush`, `HGetAll`, `ZAdd`, …) plus a
  dynamic dispatcher, `Store.Do("SET", "k", "v")`.
- `redis.NewServer(store)` returns a RESP TCP server (`Serve`/`ListenAndServe`)
  that real Redis clients can talk to.
- There is **no client type**. The library does export a RESP codec
  (`redis.NewEncoder` / `redis.NewDecoder`), so this example builds a ~20-line
  client out of those and points it at the in-process server on `127.0.0.1:0`.

Nothing here needs an external `redis-server` or network access beyond the module
download.

## Resolved module version

```
github.com/malcolmston/redis v0.0.0-20260719021431-a0d55935a4e4
```

(This repo has no semver tags, so `@latest` resolves to a pseudo-version.)

## Run

```sh
cd examples/redis
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program is fully self-terminating: the subscription drain is bounded by a
2s timeout, the RESP client sets a 10s socket deadline, and the server is closed
with a 3s bounded wait on its `Serve` goroutine.

## What it demonstrates

| Section | Covered |
|---|---|
| Strings | `Set` (+`NX`/`XX`/`EX`), `Get`, `Append`, `GetSet`, `MSet`/`MGet`, `GetRange` |
| TTL / expiry | `TTL` + `TTLCode` (`TTLValue`/`TTLNoKey`/`TTLNoExpiry`), `Persist`, `Expire`, `ExpireAt`, `ExpireTime`, `ExpireWith` (`ExpireCondNX`/`GT`) |
| Deterministic expiry | `NewManualClock` + `NewWithClock` + `clock.Advance` to observe a key actually expiring, with no `time.Sleep` |
| Counters | `Incr`, `IncrBy`, `DecrBy`, `IncrByFloat`, and `ErrNotInteger` on a non-numeric value |
| Hashes | `HSet`, `HGet`, `HGetAll`, `HKeys`, `HLen`, `HExists`, `HIncrBy`, `HSetNX`, `HDel` |
| Lists | `RPush`/`LPush`, `LRange`, `LPop`/`RPop`, `LInsert`, `LSet`, `LIndex`, `LMove`, `LLen`, `LMPop` |
| Sets | `SAdd`, `SMembers`, `SIsMember`, `SCard`, `SInter`/`SUnion`/`SDiff`, `SMIsMember`, `SMove`, `SRem` |
| Sorted sets | `ZAdd`, `ZRange`/`ZRevRange`, `ZScore`, `ZRank`/`ZRevRank`, `ZRangeByScore` (incl. exclusive bounds and `math.Inf`), `ZIncrBy`, `ZCount`, `ZCard`, `ZPopMax`, `ZRem` |
| Iteration | `Keys` glob, cursor-driven `Scan` loop with `COUNT`, `HScan` |
| Transactions | `Multi().Queue(...).Exec()`, `Watch` abort on a racing writer, `Watch` success path, queue-time rejection of an unknown command, `Discard` |
| Pub/Sub | `Subscribe`, `PSubscribe`, `Publish` (receiver counts), `PubSubChannels`/`NumSub`/`NumPat`, `Close` semantics |
| Errors | `ErrWrongType` via `errors.Is` on `LPush`/`HGet`/`SAdd`/`ZAdd`/`Get` type mismatches, `ErrUnknownCommand`, `ErrWrongArgs` |
| Persistence | `MarshalSnapshot`, `NewFromSnapshot`, `DumpKey`, `RestoreKey` |
| Extras | `SetBit`/`GetBit`/`BitCount`, `PFAdd`/`PFCount`, `XAdd`/`XLen`/`XRange`, `GeoAdd`/`GeoDist` |
| RESP server | Server on a random loopback port; 14 real commands over the wire; **pipelining** (5 commands written before any reply is read); wrong-type error reply decoded as `redis.RESPError` |

## Holes found

Everything the example calls compiles and behaves correctly. The holes are gaps
in coverage and API ergonomics, not crashes.

1. **The RESP dispatch table is a small subset of the Go API.** `dispatch.go`
   registers roughly 50 commands. Hundreds of `Store` methods
   (`MSET`, `MGET`, `SETEX`, `GETDEL`, `GETEX`, `LINSERT`, `LSET`, `LMOVE`,
   `LMPOP`, `HINCRBY`, `HMGET`, `SPOP`, `SMOVE`, `ZINCRBY`, `ZCOUNT`, `ZPOPMIN`,
   `ZPOPMAX`, `SCAN`/`HSCAN`, `SETBIT`/`BITCOUNT`, `PFADD`/`PFCOUNT`, all `X*`
   stream commands, all `GEO*` commands, `OBJECT`, `SORT`, `RENAME`, `COPY`,
   `DUMP`/`RESTORE`, …) are unreachable from any RESP client, including
   `redis-cli`. The README's command table documents only the dispatchable
   subset, so it is not wrong — but the Go surface and the wire surface diverge
   sharply, which is surprising for a "server".
2. **MULTI/EXEC/DISCARD/WATCH are not available over RESP.** The example shows
   `MULTI` returning `-ERR unknown command 'MULTI'`. Transactions exist only as
   the Go `Store.Multi()` / `Tx` API.
3. **SUBSCRIBE/PSUBSCRIBE/PUBLISH are not available over RESP** either
   (`-ERR unknown command 'SUBSCRIBE'`). Pub/Sub is Go-API-only. This is
   arguably unavoidable — the `Server` has no per-connection push path — but it
   means the server cannot serve a pub/sub client at all.
4. **No client, and no `PING`/`SELECT`/`COMMAND`/`HELLO`/`INFO`/`QUIT`.** Most
   real Redis client libraries send a handshake or health-check command on
   connect, so pointing `go-redis` at this server would likely fail before the
   first real command. The example therefore has to hand-roll a client from the
   exported RESP codec.
5. **No pipelining API on the Go side.** Pipelining is only meaningful over the
   socket (which the example does by writing N commands before reading N
   replies). Note that `Encoder.Encode` flushes on every call, so a batch is not
   coalesced into a single write — the round-trip savings are there, but the
   syscall savings are not.
6. **`Server.Addr()` returns `nil` until `Serve` has stored the listener**, and
   there is no "ready" signal. `ListenAndServe` gives you no way to learn the
   bound address at all, so binding to port `0` requires calling `net.Listen`
   yourself and passing the listener to `Serve`. The example does that.
7. **`Tx.Exec()` after `Discard()` returns the WATCH error string.**
   `ErrTxAborted` is `"EXECABORT transaction discarded due to WATCH"`, so a
   discarded (never-watched) transaction reports a WATCH failure. It is
   documented, but the message is misleading and there is no distinct sentinel
   to tell "aborted by WATCH" from "discarded".
8. **`SetOptions.EX` and `SetOptions.PX` are both `time.Duration`.** The doc
   comments say "seconds" and "milliseconds", but since both are Durations the
   two fields are functionally identical (PX just wins when both are set). Same
   redundancy elsewhere: `PExpire` is literally `Expire`, `PTTL` is `TTL`, and
   `PExpireAt`/`PExpireTime` alias their second-precision twins. Idiomatic Go
   would expose one field/method.
9. **`redis.ZMember` is a type alias to an unexported struct** (`type ZMember =
   zmember`). It works from outside the package, but godoc for the alias target
   is not rendered, so the `Member`/`Score` field names are undiscoverable
   without reading the source.
10. **Inconsistent error conventions.** `Set` returns only `bool` (no error),
    `MGet`/`HMGet`/`ZMScore` return `[]*T` with no error, while sibling methods
    on the same types return `(T, error)`. Optional/absent values are variously
    signalled by a trailing `bool`, a `*string`, or a `TTLCode`.
**No behavioural divergence from real Redis was observed** in anything the
example exercises: every wrong-type path tested (`LPUSH`/`HGET`/`SADD`/`ZADD` on
a string, `GET` on a list) returned `ErrWrongType`; TTL/expiry, NX/XX, exclusive
score bounds, WATCH abort, pub/sub receiver counts and `GEODIST`
(343.7 km London–Paris) all matched expectations.

Nothing had to be commented out; there are no `// HOLE:` stubs for missing Go
APIs. The two `// HOLE:`-annotated lines in `main.go` are live assertions that
`MULTI` and `SUBSCRIBE` are rejected over the wire.

### Dependencies

Clean: the module has zero third-party dependencies and no cgo, and `go get
github.com/malcolmston/redis@latest` resolved without issue.
