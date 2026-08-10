# `redis` parity coverage

| | |
| --- | --- |
| upstream oracle | `redis-server 8.2.2` (`redis-server --version` → `v=8.2.2 sha=00000000:1 malloc=libc bits=64 build=b8c0c59a9db96eb1`) |
| upstream ecosystem | `c` (`parity/redis/c/run.py` boots the real server) |
| Go module under test | `github.com/malcolmston/redis v0.3.0` (`GOWORK=off go get github.com/malcolmston/redis@latest`, no `replace`) |
| Go surface compared | `Store.Do(args ...string)` — the RESP dispatcher, i.e. what a client of `redis.NewServer(store)` reaches |
| harness | `GOWORK=off go test ./parity/redis/` |
| cases | 93 scripts across 12 groups |

## How the two sides are driven

Every case is a **script of commands against a fresh, empty database**. The Go
runner builds a new `redis.New()` store per case; the upstream runner opens a new
connection and issues `FLUSHALL` per case (a new connection also guarantees no
`MULTI`/`WATCH` state leaks between cases). The compared value is

```jsonc
{"steps": [<one normalised reply per command>], "dump": [<canonical key dump>]}
```

so a divergence in **state** is caught even when every return value agreed. The
dump is `KEYS *` sorted, and for each key its `TYPE`, a TTL bucket
(`none`/`set`/`gone`) and its value (`GET`, `LRANGE 0 -1`, `HGETALL`, `SMEMBERS`,
`ZRANGE 0 -1 WITHSCORES`). The dump deliberately uses **only commands the port
can dispatch**, so building the dump never biases the comparison.

The oracle is the real server, never an already-running one: `c/run.py` picks a
free ephemeral port with `bind(0)`, starts `redis-server` on `127.0.0.1` with a
`mkdtemp` dir and `--save '' --appendonly no --appendfsync no`, waits for `PING`,
and on exit sends `SHUTDOWN NOSAVE` and removes the temp dir. Nothing is
persisted and port 6379 is never touched.

### Command input

Commands are sent as arrays of argument strings in the case JSON, so both runners
receive byte-identical input. The Go side calls `Store.Do(args...)`; the upstream
side writes the same array as a RESP2 command (`*N\r\n$len\r\n…`) on a TCP socket.
The upstream runner speaks RESP directly rather than shelling out to `redis-cli`
per command because raw RESP preserves the reply *type* that `redis-cli`'s
human-readable output flattens; `redis-cli` is still the source of the command
inventory below.

### Reply normalisation (identical on both sides)

| RESP2 reply | `Store.Do` result | JSON |
| --- | --- | --- |
| integer `:5` | `int64` | `5` (number) |
| bulk string `$3\r\nfoo` | `string` | `"foo"` |
| null bulk / null array | `nil` | `null` |
| simple string `+OK` | `redis.SimpleString` | `{"status":"OK"}` |
| array `*2` | `[]any` | `[…]` (recursive) |
| error `-ERR …` | non-nil `error` (or a panic) | `{"error":true}` |

Only *whether* a command errored is compared — never the message text. A reply
that is not valid UTF-8 is reported as `{"error":true}` by both runners, so
implementation-defined binary payloads can never masquerade as agreement.

Commands whose reply order Redis does not define are sorted on both sides before
comparison: `KEYS`, `SMEMBERS`, `SINTER`, `SUNION`, `SDIFF`, `SRANDMEMBER`,
`SPOP`, `HKEYS`, `HVALS`; `HGETALL` is sorted as field/value pairs; `SCAN`-family
replies have their key array sorted. The dump sorts keys, set members and hash
pairs the same way.

### Determinism

* No `*` stream IDs, no `SRANDMEMBER`/`SPOP` on multi-member sets, no `INFO`,
  `COMMAND`, `OBJECT REFCOUNT`-style volatile output in a compared position.
* **Expiry uses a real, short sleep, not an injected clock.** The port does offer
  `redis.NewWithClock(*ManualClock)`, but the oracle has no such hook, so the
  runners share the only mechanism both can honour: `SET … PX 120` / `PEXPIRE …
  120` followed by a `{"op":"sleep","ms":400}` step. `PTTL` replies are rounded
  up to whole seconds (`"round": 1000` on the step) so the millisecond of
  in-flight time cannot flip a comparison.
* `BITOP XOR`/`NOT` and all HyperLogLog cases end with `FLUSHALL` so that
  implementation-defined binary representations stay out of the dump; those cases
  compare `BITCOUNT`/`STRLEN`/`PFCOUNT` instead.
* Two consecutive full runs produce a byte-identical `parity.json`.

**Pub/sub is `untested`.** `SUBSCRIBE`/`PSUBSCRIBE`/`PUBLISH` cannot be scripted
in this request/reply harness: `SUBSCRIBE` puts the upstream connection into
push mode where the number and timing of out-of-band messages is not a function
of the command stream, and the port's pub/sub (`pubsub.go`) is not in the RESP
dispatch table at all, so there is nothing to compare it against. It is recorded
as `missing` in the table below on the strength of the dispatch probe, and as
`untested` in the sense that no case exercises it.

## How the upstream command surface was derived

Mechanically, from the running oracle — not from memory and not from the README:

```
$ redis-cli -p <ephemeral> COMMAND COUNT
(integer) 267
$ redis-cli -p <ephemeral> COMMAND LIST | grep -v '|' | sort   # 267 names
```

`COMMAND LIST` returns 397 rows on this server; 130 of them are container
subcommands (`acl|cat`, `object|encoding`, `xinfo|stream`, …) which are folded
into their parent, leaving exactly the 267 top-level names that `COMMAND COUNT`
reports. The harness re-derives the same list at run time (`fn:"commands"` in
`c/run.py`) and stores the totals in `parity.json`.

**Denominator: 267.** That is every top-level command this `redis-server 8.2.2`
advertises, which is more than the ~240 of a bare Redis because the 8.x build
bundles the vector-set module (`VADD`, `VCARD`, `VDIM`, `VEMB`, `VGETATTR`,
`VINFO`, `VISMEMBER`, `VLINKS`, `VRANDMEMBER`, `VREM`, `VSETATTR`, `VSIM` — 12
commands). Excluding those leaves 255 core commands. All percentages below use
267; subtracting the 12 module commands from both the denominator and the
`missing` count changes the parity figure by less than half a point.

## How each command's status was derived

The set of commands the port can actually dispatch is **probed, not read off the
source**: `fn:"probe"` in `go/run.go` calls `Store.Do(name)` for all 267 upstream
names and counts a name as dispatchable iff the error is not
`errors.Is(err, redis.ErrUnknownCommand)`. This matters, because the dispatch
table in `dispatch.go` is not the whole story — `strbitmap.go`, `hll.go` and
`streams.go` each register more entries from their own `init()`.

Then, mechanically:

* `missing` — not dispatchable.
* `differs` — dispatchable and named in the `upstreamFn` of at least one case
  that **mismatched**.
* `match` — dispatchable, not the above, and present in at least one case that
  matched.
* `untested` — dispatchable but in no case. (There are none.)
* `extra` — a `Store.Do` command that upstream does not have. (There are none;
  all 64 dispatchable names appear in `COMMAND LIST`.)

## Score

| | |
| --- | --- |
| upstream commands (denominator) | **267** |
| reachable through `Store.Do` | **64 (23.97%)** |
| `match` | **51** |
| `differs` | **13** |
| `missing` (not dispatchable) | **203** |
| `extra` | 0 |
| `untested` (dispatchable, no case) | 0 |
| **parity over the commands actually compared** | **51 / 64 = 79.69%** |
| **parity over the whole upstream surface** | **51 / 267 = 19.10%** |
| cases | 93 (62 match, 31 mismatch, 0 deviations) → **66.67%** |

Per-group case scores (from `parity.json`):

| group | cases | match | mismatch |
| --- | --- | --- | --- |
| strings | 12 | 10 | 2 |
| keyspace | 5 | 5 | 0 |
| expiry | 7 | 7 | 0 |
| hashes | 5 | 5 | 0 |
| lists | 6 | 5 | 1 |
| sets | 4 | 4 | 0 |
| sortedsets | 10 | 7 | 3 |
| bitmaps | 5 | 4 | 1 |
| hyperloglog | 3 | 3 | 0 |
| streams | 4 | 3 | 1 |
| errors (WRONGTYPE + arity) | 9 | 9 | 0 |
| gaps (undispatchable commands) | 23 | 0 | 23 |

## Semantic divergences found

Wrong replies or wrong state, in order of severity:

1. **`SET key value KEEPTTL` is rejected** (`set-keepttl`). The port's `SET`
   option parser knows only `NX`, `XX`, `EX`, `PX`; anything else is a syntax
   error, so the value is never written. Upstream rewrites the value and keeps
   the TTL.
2. **`SET key value EX 0` and `EX -1` are accepted** (`set-ex-invalid`).
   Upstream rejects a non-positive expire with `ERR invalid expire time in
   'set' command` and does *not* create the key; the port creates the key with
   **no** TTL. This one is over-permissive *and* leaves divergent state.
3. **All `ZADD` flags are unusable** (`zadd-flags`). `cmdZAdd` parses argument 1
   as a float, so `ZADD z NX 1 a`, `XX`, `GT`, `LT`, `CH` and `INCR` all fail
   with "not a valid float". Upstream supports all six.
4. **Sorted-set score formatting** (`zset-infinite-scores`,
   `zset-large-scores`). The port formats scores with `strconv.FormatFloat(f,
   'g', -1, 64)`, producing `+Inf`, `-Inf`, `1e+17`, `1e-07` where Redis prints
   `inf`, `-inf`, `100000000000000000`, `1e-7`. Affects `ZSCORE` and every
   `WITHSCORES` reply. (`formatFloatHuman` in the port already does the right
   thing for `INCRBYFLOAT`; the zset path does not use it.)
5. **Streams are stored outside the keyspace** (`stream-keyspace-invisible`).
   `XADD` writes into a package-level `streamReg` map keyed by `*Store`, so
   `TYPE` replies `none`, `EXISTS`/`DBSIZE` reply 0, `KEYS *` omits the key and
   `DEL` cannot remove it, while `XLEN` still sees the entries. `FLUSHALL` does
   not clear it either, and the registry is never pruned when a `*Store` is
   dropped. `XADD`/`XLEN`/`XRANGE`/`XREAD` themselves are at parity.
6. **`BITPOS key 0` on a missing key** (`bitmap-bitpos`). Upstream treats an
   absent key as an infinite run of zero bits and replies `0`; the port replies
   `-1`.
7. **`LPOP`/`RPOP` reject the `count` argument** (`list-pop-with-count`). Arity
   is fixed at 1, so `LPOP key 2` errors instead of returning an array.
8. **`EXPIRE` rejects the conditional forms** (`gap-expire-variants`).
   `EXPIRE key seconds NX|XX|GT|LT` errors on arity.

Over-permissive errors (the port succeeds where upstream fails): item 2 above is
the only one found. Every WRONGTYPE case and every arity case in the `errors`
group agrees with upstream, including `INCR` on `"abc"`, `"3.0"`, `""` and
`" 1"`, 64-bit overflow on `INCR`, `DECRBY LLONG_MIN`, `SET … NX XX`, and
running each type's commands against all four other types.

No case is marked `deviation`: none of the divergences above is documented as
deliberate.

## What the port lacks (beyond individual commands)

* **`MULTI`/`EXEC`/`DISCARD`/`WATCH`/`UNWATCH` are not dispatchable at all.**
  `transactions.go` implements them as typed Go API, and even consults
  `dispatchTable` for queue-time validation, but registers nothing, so a RESP
  client cannot open a transaction. `gap-transactions`,
  `gap-transaction-discard`, `gap-transaction-watch` and
  `gap-transaction-queue-error` show the consequence: the "queued" commands
  execute immediately, `DISCARD` cannot roll anything back (the port ends with
  `k = "after"` where upstream ends with `k = "before"`), and a queue-time error
  does not abort anything.
* **No connection or server protocol commands**: no `PING`, `ECHO`, `HELLO`,
  `SELECT`, `AUTH`, `QUIT`, `RESET`, `CLIENT`, `COMMAND`, `INFO`, `CONFIG`.
  There is exactly one logical database (`Store`) and `SELECT` cannot be spoken.
* **No `SCAN`/`HSCAN`/`SSCAN`/`ZSCAN`** over RESP, although `scan.go` implements
  cursored iteration in the typed API.
* **No `MSET`/`MGET`/`MSETNX`, `SETEX`/`PSETEX`/`SETNX`, `GETDEL`/`GETEX`,
  `SETRANGE`/`GETRANGE`, `INCRBYFLOAT`**.
* **No list mutators**: `LSET`, `LINSERT`, `LTRIM`, `LREM`, `LPOS`, `LMOVE`,
  `LMPOP`, `RPOPLPUSH`, and none of the blocking `B*` forms.
* **No `HINCRBY`/`HINCRBYFLOAT`/`HMGET`/`HMSET`/`HSETNX`/`HSTRLEN`/`HRANDFIELD`**.
* **No `SPOP`/`SMOVE`/`SRANDMEMBER`/`SMISMEMBER`** and no `*STORE`/`SINTERCARD`
  set forms.
* **No `ZINCRBY`/`ZCOUNT`/`ZPOPMIN`/`ZPOPMAX`/`ZREMRANGEBY*`, no lexical range
  commands** (`ZRANGEBYLEX`, `ZREVRANGEBYLEX`, `ZLEXCOUNT`), no
  `ZREVRANGEBYSCORE`, no `ZRANGESTORE`, and no `BY*`/`REV`/`LIMIT` arguments on
  `ZRANGE`.
* **No `RENAME`/`RENAMENX`/`COPY`/`UNLINK`/`TOUCH`/`RANDOMKEY`/`SORT`/`OBJECT`/
  `DUMP`/`RESTORE`/`MIGRATE`**, no `EXPIREAT`/`PEXPIREAT`/`EXPIRETIME`/
  `PEXPIRETIME`.
* **No `GEO*`** (`geo.go` exists as typed API only), no stream consumer-group
  commands (`XGROUP`, `XACK`, `XREADGROUP`, `XAUTOCLAIM`, `XDEL`, `XTRIM`,
  `XINFO`), no scripting (`EVAL`, `FUNCTION`), no cluster/replication/ACL/
  persistence administration.

## Full inventory — all 267 upstream commands

`match` = compared and agreed · `differs` = compared and diverged ·
`missing` = not reachable through `Store.Do` · `extra` = Go-only (none) ·
`untested` = reachable but uncased (none)

| upstream command | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `ACL` | — | missing | — | not in the RESP dispatch table |
| `APPEND` | `Store.Do("APPEND", …)` | match | `set-get-basic` |  |
| `ASKING` | — | missing | — | not in the RESP dispatch table |
| `AUTH` | — | missing | — | not in the RESP dispatch table |
| `BGREWRITEAOF` | — | missing | — | not in the RESP dispatch table |
| `BGSAVE` | — | missing | — | not in the RESP dispatch table |
| `BITCOUNT` | `Store.Do("BITCOUNT", …)` | match | `bitmap-bitcount`, `bitmap-errors` |  |
| `BITFIELD` | — | missing | — | not in the RESP dispatch table |
| `BITFIELD_RO` | — | missing | — | not in the RESP dispatch table |
| `BITOP` | `Store.Do("BITOP", …)` | match | `bitmap-bitop`, `bitmap-errors` |  |
| `BITPOS` | `Store.Do("BITPOS", …)` | differs | `bitmap-bitpos`, `bitmap-errors` | `BITPOS nokey 0` replies 0 upstream (a missing key is an infinite run of zero bits) and -1 in the port |
| `BLMOVE` | — | missing | — | not in the RESP dispatch table |
| `BLMPOP` | — | missing | — | not in the RESP dispatch table |
| `BLPOP` | — | missing | — | not in the RESP dispatch table |
| `BRPOP` | — | missing | — | not in the RESP dispatch table |
| `BRPOPLPUSH` | — | missing | — | not in the RESP dispatch table |
| `BZMPOP` | — | missing | — | not in the RESP dispatch table |
| `BZPOPMAX` | — | missing | — | not in the RESP dispatch table |
| `BZPOPMIN` | — | missing | — | not in the RESP dispatch table |
| `CLIENT` | — | missing | — | not in the RESP dispatch table |
| `CLUSTER` | — | missing | — | not in the RESP dispatch table |
| `COMMAND` | — | missing | — | not in the RESP dispatch table |
| `CONFIG` | — | missing | — | not in the RESP dispatch table |
| `COPY` | — | missing | `gap-rename-copy` | not in the RESP dispatch table |
| `DBSIZE` | `Store.Do("DBSIZE", …)` | differs | `stream-keyspace-invisible`, `flushall-dbsize` | correct for all five stored types; does not count stream keys |
| `DEBUG` | — | missing | — | not in the RESP dispatch table |
| `DECR` | `Store.Do("DECR", …)` | match | `incr-decr` |  |
| `DECRBY` | `Store.Do("DECRBY", …)` | match | `incr-decr`, `decrby-llong-min` |  |
| `DEL` | `Store.Do("DEL", …)` | differs | `stream-keyspace-invisible`, `exists-del`, `del-mixed-types` | correct for all five stored types; cannot delete a stream key |
| `DISCARD` | — | missing | `gap-transaction-discard` | not in the RESP dispatch table |
| `DUMP` | — | missing | `gap-dump-restore` | not in the RESP dispatch table |
| `ECHO` | — | missing | `gap-connection-commands` | not in the RESP dispatch table |
| `EVAL` | — | missing | — | not in the RESP dispatch table |
| `EVALSHA` | — | missing | — | not in the RESP dispatch table |
| `EVALSHA_RO` | — | missing | — | not in the RESP dispatch table |
| `EVAL_RO` | — | missing | — | not in the RESP dispatch table |
| `EXEC` | — | missing | `gap-transactions`, `gap-transaction-queue-error` | not in the RESP dispatch table |
| `EXISTS` | `Store.Do("EXISTS", …)` | differs | `stream-keyspace-invisible`, `exists-del` | correct for all five stored types; replies 0 for a stream key |
| `EXPIRE` | `Store.Do("EXPIRE", …)` | differs | `gap-expire-variants`, `expire-persist`, `expire-negative-deletes`, `expire-non-integer` | the `NX|XX|GT|LT` conditional forms are rejected (arity is fixed at 2) |
| `EXPIREAT` | — | missing | `gap-expire-variants` | not in the RESP dispatch table |
| `EXPIRETIME` | — | missing | `gap-expire-variants` | not in the RESP dispatch table |
| `FAILOVER` | — | missing | — | not in the RESP dispatch table |
| `FCALL` | — | missing | — | not in the RESP dispatch table |
| `FCALL_RO` | — | missing | — | not in the RESP dispatch table |
| `FLUSHALL` | `Store.Do("FLUSHALL", …)` | match | `flushall-dbsize` |  |
| `FLUSHDB` | — | missing | — | not in the RESP dispatch table |
| `FUNCTION` | — | missing | — | not in the RESP dispatch table |
| `GEOADD` | — | missing | `gap-geo` | not in the RESP dispatch table |
| `GEODIST` | — | missing | `gap-geo` | not in the RESP dispatch table |
| `GEOHASH` | — | missing | `gap-geo` | not in the RESP dispatch table |
| `GEOPOS` | — | missing | `gap-geo` | not in the RESP dispatch table |
| `GEORADIUS` | — | missing | — | not in the RESP dispatch table |
| `GEORADIUSBYMEMBER` | — | missing | — | not in the RESP dispatch table |
| `GEORADIUSBYMEMBER_RO` | — | missing | — | not in the RESP dispatch table |
| `GEORADIUS_RO` | — | missing | — | not in the RESP dispatch table |
| `GEOSEARCH` | — | missing | — | not in the RESP dispatch table |
| `GEOSEARCHSTORE` | — | missing | — | not in the RESP dispatch table |
| `GET` | `Store.Do("GET", …)` | match | `set-get-basic`, `set-empty-value` |  |
| `GETBIT` | `Store.Do("GETBIT", …)` | match | `bitmap-setbit-getbit`, `bitmap-errors` |  |
| `GETDEL` | — | missing | `gap-getdel-getex` | not in the RESP dispatch table |
| `GETEX` | — | missing | `gap-getdel-getex` | not in the RESP dispatch table |
| `GETRANGE` | — | missing | `gap-setrange-getrange` | not in the RESP dispatch table |
| `GETSET` | `Store.Do("GETSET", …)` | match | `getset` |  |
| `HDEL` | `Store.Do("HDEL", …)` | match | `hash-delete-empties-key`, `hash-arity` |  |
| `HELLO` | — | missing | — | not in the RESP dispatch table |
| `HEXISTS` | `Store.Do("HEXISTS", …)` | match | `hash-basics` |  |
| `HEXPIRE` | — | missing | — | not in the RESP dispatch table |
| `HEXPIREAT` | — | missing | — | not in the RESP dispatch table |
| `HEXPIRETIME` | — | missing | — | not in the RESP dispatch table |
| `HGET` | `Store.Do("HGET", …)` | match | `hash-basics`, `hash-arity` |  |
| `HGETALL` | `Store.Do("HGETALL", …)` | match | `hash-basics`, `hash-missing-key-replies` |  |
| `HGETDEL` | — | missing | — | not in the RESP dispatch table |
| `HGETEX` | — | missing | — | not in the RESP dispatch table |
| `HINCRBY` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HINCRBYFLOAT` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HKEYS` | `Store.Do("HKEYS", …)` | match | `hash-basics`, `hash-missing-key-replies` |  |
| `HLEN` | `Store.Do("HLEN", …)` | match | `hash-basics`, `hash-missing-key-replies` |  |
| `HMGET` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HMSET` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HPERSIST` | — | missing | — | not in the RESP dispatch table |
| `HPEXPIRE` | — | missing | — | not in the RESP dispatch table |
| `HPEXPIREAT` | — | missing | — | not in the RESP dispatch table |
| `HPEXPIRETIME` | — | missing | — | not in the RESP dispatch table |
| `HPTTL` | — | missing | — | not in the RESP dispatch table |
| `HRANDFIELD` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HSCAN` | — | missing | `gap-scan` | not in the RESP dispatch table |
| `HSET` | `Store.Do("HSET", …)` | match | `hash-basics`, `hash-arity`, `hash-empty-field-and-value` |  |
| `HSETEX` | — | missing | — | not in the RESP dispatch table |
| `HSETNX` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HSTRLEN` | — | missing | `gap-hash-extras` | not in the RESP dispatch table |
| `HTTL` | — | missing | — | not in the RESP dispatch table |
| `HVALS` | `Store.Do("HVALS", …)` | match | `hash-basics`, `hash-missing-key-replies` |  |
| `INCR` | `Store.Do("INCR", …)` | match | `incr-decr`, `incr-errors` |  |
| `INCRBY` | `Store.Do("INCRBY", …)` | match | `incr-decr`, `decrby-llong-min` |  |
| `INCRBYFLOAT` | — | missing | `gap-incrbyfloat` | not in the RESP dispatch table |
| `INFO` | — | missing | — | not in the RESP dispatch table |
| `KEYS` | `Store.Do("KEYS", …)` | differs | `stream-keyspace-invisible`, `keys-glob` | correct for all five stored types; never lists a stream key |
| `LASTSAVE` | — | missing | — | not in the RESP dispatch table |
| `LATENCY` | — | missing | — | not in the RESP dispatch table |
| `LCS` | — | missing | — | not in the RESP dispatch table |
| `LINDEX` | `Store.Do("LINDEX", …)` | match | `list-push-pop`, `list-arity-and-non-integer` |  |
| `LINSERT` | — | missing | `gap-list-mutators` | not in the RESP dispatch table |
| `LLEN` | `Store.Do("LLEN", …)` | match | `list-push-pop`, `list-arity-and-non-integer` |  |
| `LMOVE` | — | missing | `gap-lmove-lmpop` | not in the RESP dispatch table |
| `LMPOP` | — | missing | `gap-lmove-lmpop` | not in the RESP dispatch table |
| `LOLWUT` | — | missing | — | not in the RESP dispatch table |
| `LPOP` | `Store.Do("LPOP", …)` | differs | `list-pop-with-count`, `list-push-pop`, `list-pop-empties-key` | the optional `count` argument is rejected (arity is fixed at 1) |
| `LPOS` | — | missing | `gap-list-mutators` | not in the RESP dispatch table |
| `LPUSH` | `Store.Do("LPUSH", …)` | match | `list-push-pop`, `list-arity-and-non-integer`, `list-duplicates-and-order` |  |
| `LPUSHX` | — | missing | — | not in the RESP dispatch table |
| `LRANGE` | `Store.Do("LRANGE", …)` | match | `list-push-pop`, `lrange-bounds`, `list-arity-and-non-integer` |  |
| `LREM` | — | missing | `gap-list-mutators` | not in the RESP dispatch table |
| `LSET` | — | missing | `gap-list-mutators` | not in the RESP dispatch table |
| `LTRIM` | — | missing | `gap-list-mutators` | not in the RESP dispatch table |
| `MEMORY` | — | missing | — | not in the RESP dispatch table |
| `MGET` | — | missing | `gap-mset-mget` | not in the RESP dispatch table |
| `MIGRATE` | — | missing | — | not in the RESP dispatch table |
| `MODULE` | — | missing | — | not in the RESP dispatch table |
| `MONITOR` | — | missing | — | not in the RESP dispatch table |
| `MOVE` | — | missing | — | not in the RESP dispatch table |
| `MSET` | — | missing | `gap-mset-mget` | not in the RESP dispatch table |
| `MSETNX` | — | missing | `gap-mset-mget` | not in the RESP dispatch table |
| `MULTI` | — | missing | `gap-transactions`, `gap-transaction-discard`, `gap-transaction-queue-error` | not in the RESP dispatch table |
| `OBJECT` | — | missing | `gap-keyspace-extras`, `gap-dump-restore` | not in the RESP dispatch table |
| `PERSIST` | `Store.Do("PERSIST", …)` | match | `expire-persist` |  |
| `PEXPIRE` | `Store.Do("PEXPIRE", …)` | match | `pexpire`, `pexpire-elapses` |  |
| `PEXPIREAT` | — | missing | — | not in the RESP dispatch table |
| `PEXPIRETIME` | — | missing | — | not in the RESP dispatch table |
| `PFADD` | `Store.Do("PFADD", …)` | match | `hll-basic`, `hll-empty-and-errors` |  |
| `PFCOUNT` | `Store.Do("PFCOUNT", …)` | match | `hll-basic`, `hll-cardinality-20`, `hll-empty-and-errors` |  |
| `PFDEBUG` | — | missing | — | not in the RESP dispatch table |
| `PFMERGE` | `Store.Do("PFMERGE", …)` | match | `hll-basic`, `hll-empty-and-errors` |  |
| `PFSELFTEST` | — | missing | — | not in the RESP dispatch table |
| `PING` | — | missing | `gap-connection-commands` | not in the RESP dispatch table |
| `PSETEX` | — | missing | `gap-setex-setnx` | not in the RESP dispatch table |
| `PSUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `PSYNC` | — | missing | — | not in the RESP dispatch table |
| `PTTL` | `Store.Do("PTTL", …)` | match | `expire-persist` |  |
| `PUBLISH` | — | missing | — | not in the RESP dispatch table |
| `PUBSUB` | — | missing | — | not in the RESP dispatch table |
| `PUNSUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `QUIT` | — | missing | — | not in the RESP dispatch table |
| `RANDOMKEY` | — | missing | `gap-keyspace-extras` | not in the RESP dispatch table |
| `READONLY` | — | missing | — | not in the RESP dispatch table |
| `READWRITE` | — | missing | — | not in the RESP dispatch table |
| `RENAME` | — | missing | `gap-rename-copy` | not in the RESP dispatch table |
| `RENAMENX` | — | missing | `gap-rename-copy` | not in the RESP dispatch table |
| `REPLCONF` | — | missing | — | not in the RESP dispatch table |
| `REPLICAOF` | — | missing | — | not in the RESP dispatch table |
| `RESET` | — | missing | — | not in the RESP dispatch table |
| `RESTORE` | — | missing | — | not in the RESP dispatch table |
| `RESTORE-ASKING` | — | missing | — | not in the RESP dispatch table |
| `ROLE` | — | missing | — | not in the RESP dispatch table |
| `RPOP` | `Store.Do("RPOP", …)` | match | `list-push-pop`, `list-pop-empties-key` |  |
| `RPOPLPUSH` | — | missing | `gap-lmove-lmpop` | not in the RESP dispatch table |
| `RPUSH` | `Store.Do("RPUSH", …)` | match | `list-push-pop`, `list-arity-and-non-integer`, `list-duplicates-and-order` |  |
| `RPUSHX` | — | missing | — | not in the RESP dispatch table |
| `SADD` | `Store.Do("SADD", …)` | match | `set-basics`, `set-arity` |  |
| `SAVE` | — | missing | — | not in the RESP dispatch table |
| `SCAN` | — | missing | `gap-scan` | not in the RESP dispatch table |
| `SCARD` | `Store.Do("SCARD", …)` | match | `set-basics`, `set-arity` |  |
| `SCRIPT` | — | missing | — | not in the RESP dispatch table |
| `SDIFF` | `Store.Do("SDIFF", …)` | match | `set-algebra` |  |
| `SDIFFSTORE` | — | missing | `gap-set-store` | not in the RESP dispatch table |
| `SELECT` | — | missing | `gap-connection-commands` | not in the RESP dispatch table |
| `SET` | `Store.Do("SET", …)` | differs | `set-keepttl`, `set-ex-invalid`, `expire-elapses`, `set-get-basic` | `KEEPTTL` is a syntax error in the port; `EX 0` / `EX -1` are accepted instead of rejected |
| `SETBIT` | `Store.Do("SETBIT", …)` | match | `bitmap-setbit-getbit`, `bitmap-errors` |  |
| `SETEX` | — | missing | `gap-setex-setnx` | not in the RESP dispatch table |
| `SETNX` | — | missing | `gap-setex-setnx` | not in the RESP dispatch table |
| `SETRANGE` | — | missing | `gap-setrange-getrange` | not in the RESP dispatch table |
| `SHUTDOWN` | — | missing | — | not in the RESP dispatch table |
| `SINTER` | `Store.Do("SINTER", …)` | match | `set-algebra`, `set-arity` |  |
| `SINTERCARD` | — | missing | `gap-set-store` | not in the RESP dispatch table |
| `SINTERSTORE` | — | missing | `gap-set-store` | not in the RESP dispatch table |
| `SISMEMBER` | `Store.Do("SISMEMBER", …)` | match | `set-basics`, `set-arity` |  |
| `SLAVEOF` | — | missing | — | not in the RESP dispatch table |
| `SLOWLOG` | — | missing | — | not in the RESP dispatch table |
| `SMEMBERS` | `Store.Do("SMEMBERS", …)` | match | `set-basics`, `set-arity` |  |
| `SMISMEMBER` | — | missing | `gap-set-extras` | not in the RESP dispatch table |
| `SMOVE` | — | missing | `gap-set-extras` | not in the RESP dispatch table |
| `SORT` | — | missing | `gap-keyspace-extras` | not in the RESP dispatch table |
| `SORT_RO` | — | missing | — | not in the RESP dispatch table |
| `SPOP` | — | missing | `gap-set-extras` | not in the RESP dispatch table |
| `SPUBLISH` | — | missing | — | not in the RESP dispatch table |
| `SRANDMEMBER` | — | missing | `gap-set-extras` | not in the RESP dispatch table |
| `SREM` | `Store.Do("SREM", …)` | match | `set-basics`, `set-empty-removes-key`, `set-arity` |  |
| `SSCAN` | — | missing | `gap-scan` | not in the RESP dispatch table |
| `SSUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `STRLEN` | `Store.Do("STRLEN", …)` | match | `set-get-basic` |  |
| `SUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `SUBSTR` | — | missing | — | not in the RESP dispatch table |
| `SUNION` | `Store.Do("SUNION", …)` | match | `set-algebra` |  |
| `SUNIONSTORE` | — | missing | `gap-set-store` | not in the RESP dispatch table |
| `SUNSUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `SWAPDB` | — | missing | — | not in the RESP dispatch table |
| `SYNC` | — | missing | — | not in the RESP dispatch table |
| `TIME` | — | missing | — | not in the RESP dispatch table |
| `TOUCH` | — | missing | `gap-keyspace-extras` | not in the RESP dispatch table |
| `TTL` | `Store.Do("TTL", …)` | match | `expire-persist` |  |
| `TYPE` | `Store.Do("TYPE", …)` | differs | `stream-keyspace-invisible`, `type-of-every-kind` | correct for all five stored types; replies `none` for a stream key created by XADD |
| `UNLINK` | — | missing | `gap-keyspace-extras` | not in the RESP dispatch table |
| `UNSUBSCRIBE` | — | missing | — | not in the RESP dispatch table |
| `UNWATCH` | — | missing | `gap-transaction-watch` | not in the RESP dispatch table |
| `VADD` | — | missing | — | not in the RESP dispatch table |
| `VCARD` | — | missing | — | not in the RESP dispatch table |
| `VDIM` | — | missing | — | not in the RESP dispatch table |
| `VEMB` | — | missing | — | not in the RESP dispatch table |
| `VGETATTR` | — | missing | — | not in the RESP dispatch table |
| `VINFO` | — | missing | — | not in the RESP dispatch table |
| `VISMEMBER` | — | missing | — | not in the RESP dispatch table |
| `VLINKS` | — | missing | — | not in the RESP dispatch table |
| `VRANDMEMBER` | — | missing | — | not in the RESP dispatch table |
| `VREM` | — | missing | — | not in the RESP dispatch table |
| `VSETATTR` | — | missing | — | not in the RESP dispatch table |
| `VSIM` | — | missing | — | not in the RESP dispatch table |
| `WAIT` | — | missing | — | not in the RESP dispatch table |
| `WAITAOF` | — | missing | — | not in the RESP dispatch table |
| `WATCH` | — | missing | `gap-transaction-watch` | not in the RESP dispatch table |
| `XACK` | — | missing | — | not in the RESP dispatch table |
| `XACKDEL` | — | missing | — | not in the RESP dispatch table |
| `XADD` | `Store.Do("XADD", …)` | differs | `stream-keyspace-invisible`, `stream-xadd-xlen-xrange`, `stream-xadd-id-rules` | stores the stream in a package-level side registry, so the key is invisible to the keyspace |
| `XAUTOCLAIM` | — | missing | — | not in the RESP dispatch table |
| `XCLAIM` | — | missing | — | not in the RESP dispatch table |
| `XDEL` | — | missing | — | not in the RESP dispatch table |
| `XDELEX` | — | missing | — | not in the RESP dispatch table |
| `XGROUP` | — | missing | — | not in the RESP dispatch table |
| `XINFO` | — | missing | — | not in the RESP dispatch table |
| `XLEN` | `Store.Do("XLEN", …)` | match | `stream-xadd-xlen-xrange` |  |
| `XPENDING` | — | missing | — | not in the RESP dispatch table |
| `XRANGE` | `Store.Do("XRANGE", …)` | match | `stream-xadd-xlen-xrange` |  |
| `XREAD` | `Store.Do("XREAD", …)` | match | `stream-xread` |  |
| `XREADGROUP` | — | missing | — | not in the RESP dispatch table |
| `XREVRANGE` | — | missing | — | not in the RESP dispatch table |
| `XSETID` | — | missing | — | not in the RESP dispatch table |
| `XTRIM` | — | missing | — | not in the RESP dispatch table |
| `ZADD` | `Store.Do("ZADD", …)` | differs | `zset-infinite-scores`, `zset-large-scores`, `zadd-flags`, `zset-basics` | the `NX|XX|GT|LT|CH|INCR` flags are parsed as a score, so every flagged form is an error |
| `ZCARD` | `Store.Do("ZCARD", …)` | match | `zset-basics` |  |
| `ZCOUNT` | — | missing | `gap-zset-extras` | not in the RESP dispatch table |
| `ZDIFF` | — | missing | — | not in the RESP dispatch table |
| `ZDIFFSTORE` | — | missing | — | not in the RESP dispatch table |
| `ZINCRBY` | — | missing | `gap-zset-extras` | not in the RESP dispatch table |
| `ZINTER` | — | missing | — | not in the RESP dispatch table |
| `ZINTERCARD` | — | missing | — | not in the RESP dispatch table |
| `ZINTERSTORE` | — | missing | — | not in the RESP dispatch table |
| `ZLEXCOUNT` | — | missing | `gap-zset-lex` | not in the RESP dispatch table |
| `ZMPOP` | — | missing | — | not in the RESP dispatch table |
| `ZMSCORE` | — | missing | — | not in the RESP dispatch table |
| `ZPOPMAX` | — | missing | `gap-zset-extras` | not in the RESP dispatch table |
| `ZPOPMIN` | — | missing | `gap-zset-extras` | not in the RESP dispatch table |
| `ZRANDMEMBER` | — | missing | — | not in the RESP dispatch table |
| `ZRANGE` | `Store.Do("ZRANGE", …)` | differs | `gap-zset-lex`, `zset-basics`, `zset-ties-sort-lexically`, `zrange-bounds` | same score formatting divergence in `WITHSCORES` replies |
| `ZRANGEBYLEX` | — | missing | `gap-zset-lex` | not in the RESP dispatch table |
| `ZRANGEBYSCORE` | `Store.Do("ZRANGEBYSCORE", …)` | match | `zrangebyscore` |  |
| `ZRANGESTORE` | — | missing | `gap-zset-lex` | not in the RESP dispatch table |
| `ZRANK` | `Store.Do("ZRANK", …)` | match | `zset-basics` |  |
| `ZREM` | `Store.Do("ZREM", …)` | match | `zset-basics`, `zset-arity-and-errors`, `zset-empty-removes-key` |  |
| `ZREMRANGEBYLEX` | — | missing | — | not in the RESP dispatch table |
| `ZREMRANGEBYRANK` | — | missing | — | not in the RESP dispatch table |
| `ZREMRANGEBYSCORE` | — | missing | — | not in the RESP dispatch table |
| `ZREVRANGE` | `Store.Do("ZREVRANGE", …)` | match | `zset-basics`, `zrange-bounds` |  |
| `ZREVRANGEBYLEX` | — | missing | — | not in the RESP dispatch table |
| `ZREVRANGEBYSCORE` | — | missing | `gap-zset-lex` | not in the RESP dispatch table |
| `ZREVRANK` | `Store.Do("ZREVRANK", …)` | match | `zset-basics` |  |
| `ZSCAN` | — | missing | `gap-scan` | not in the RESP dispatch table |
| `ZSCORE` | `Store.Do("ZSCORE", …)` | differs | `zset-large-scores`, `zset-basics`, `zset-fractional-scores`, `zset-arity-and-errors` | score formatting: `%g` yields `+Inf`/`-Inf`/`1e+17`/`1e-07` where Redis prints `inf`/`-inf`/`100000000000000000`/`1e-7` |
| `ZUNION` | — | missing | — | not in the RESP dispatch table |
| `ZUNIONSTORE` | — | missing | — | not in the RESP dispatch table |
