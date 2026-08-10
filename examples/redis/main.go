// Command redis-example exercises github.com/malcolmston/redis, an embeddable
// in-memory Redis-style store that also ships a RESP TCP server.
//
// It needs no external redis-server: the embedded Store is used directly for the
// typed Go API, and the library's own Server is started on a random loopback
// port and driven with the library's own RESP Encoder/Decoder acting as a client.
package main

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/malcolmston/redis"
)

func section(name string) {
	fmt.Printf("\n=== %s ===\n", name)
}

func check(label string, err error) {
	if err != nil {
		fmt.Printf("  !! %s: %v\n", label, err)
	}
}

func main() {
	s := redis.New()

	stringsAndTTL(s)
	counters(s)
	hashes(s)
	lists(s)
	sets(s)
	sortedSets(s)
	expiryWithManualClock()
	scanning(s)
	transactions(s)
	pubsub(s)
	wrongType(s)
	snapshots(s)
	extras(s)

	if err := serverRoundTrip(); err != nil {
		fmt.Printf("\n!! RESP server demo failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nAll sections completed.")
}

// ---------------------------------------------------------------- strings/TTL

func stringsAndTTL(s *redis.Store) {
	section("strings: SET / GET / SET with TTL / NX / XX")

	fmt.Println("  SET greeting:", s.Set("greeting", "hello", redis.SetOptions{}))
	v, ok, err := s.Get("greeting")
	check("GET", err)
	fmt.Printf("  GET greeting = %q (found=%v)\n", v, ok)

	// NX must fail on an existing key, XX must fail on a missing key.
	fmt.Println("  SET greeting NX (expect false):", s.Set("greeting", "other", redis.SetOptions{NX: true}))
	fmt.Println("  SET missing XX  (expect false):", s.Set("missing", "x", redis.SetOptions{XX: true}))

	// SET with an expiration.
	s.Set("session", "token-abc", redis.SetOptions{EX: 30 * time.Second})
	ttl, code := s.TTL("session")
	fmt.Printf("  SET session EX 30s -> TTL %v (code=%d, TTLValue=%d)\n", ttl.Round(time.Second), code, redis.TTLValue)

	_, noKey := s.TTL("nope")
	fmt.Printf("  TTL of absent key -> code=%d (TTLNoKey=%d)\n", noKey, redis.TTLNoKey)
	_, noExp := s.TTL("greeting")
	fmt.Printf("  TTL of persistent key -> code=%d (TTLNoExpiry=%d)\n", noExp, redis.TTLNoExpiry)

	fmt.Println("  PERSIST session:", s.Persist("session"))
	_, code = s.TTL("session")
	fmt.Printf("  TTL after PERSIST -> code=%d\n", code)

	n, err := s.Append("greeting", " world")
	check("APPEND", err)
	v, _, _ = s.Get("greeting")
	fmt.Printf("  APPEND -> len=%d value=%q\n", n, v)

	old, had, err := s.GetSet("greeting", "replaced")
	check("GETSET", err)
	fmt.Printf("  GETSET returned old=%q (had=%v)\n", old, had)

	check("MSET", s.MSet("a", "1", "b", "2", "c", "3"))
	mget := s.MGet("a", "b", "zzz")
	fmt.Print("  MGET a b zzz = [")
	for i, p := range mget {
		if i > 0 {
			fmt.Print(" ")
		}
		if p == nil {
			fmt.Print("<nil>")
		} else {
			fmt.Printf("%q", *p)
		}
	}
	fmt.Println("]")

	sub, err := s.GetRange("greeting", 0, 3)
	check("GETRANGE", err)
	fmt.Printf("  GETRANGE greeting 0 3 = %q\n", sub)
}

// ------------------------------------------------------------------- counters

func counters(s *redis.Store) {
	section("counters: INCR / INCRBY / DECR / INCRBYFLOAT")

	s.Set("hits", "10", redis.SetOptions{})
	n, err := s.Incr("hits")
	check("INCR", err)
	fmt.Println("  INCR hits =", n)

	n, err = s.IncrBy("hits", 25)
	check("INCRBY", err)
	fmt.Println("  INCRBY hits 25 =", n)

	n, err = s.DecrBy("hits", 6)
	check("DECRBY", err)
	fmt.Println("  DECRBY hits 6 =", n)

	f, err := s.IncrByFloat("ratio", 1.5)
	check("INCRBYFLOAT", err)
	fmt.Println("  INCRBYFLOAT ratio 1.5 =", f)

	// Non-integer value must produce ErrNotInteger.
	s.Set("word", "abc", redis.SetOptions{})
	_, err = s.Incr("word")
	fmt.Printf("  INCR on non-numeric -> errors.Is(ErrNotInteger)=%v (%v)\n",
		errors.Is(err, redis.ErrNotInteger), err)
}

// --------------------------------------------------------------------- hashes

func hashes(s *redis.Store) {
	section("hashes: HSET / HGET / HGETALL / HINCRBY")

	added, err := s.HSet("user:1", "name", "alice", "email", "a@example.com", "logins", "3")
	check("HSET", err)
	fmt.Println("  HSET added fields:", added)

	name, ok, err := s.HGet("user:1", "name")
	check("HGET", err)
	fmt.Printf("  HGET user:1 name = %q (found=%v)\n", name, ok)

	all, err := s.HGetAll("user:1")
	check("HGETALL", err)
	fmt.Println("  HGETALL user:1 =", sortedMap(all))

	logins, err := s.HIncrBy("user:1", "logins", 2)
	check("HINCRBY", err)
	fmt.Println("  HINCRBY logins 2 =", logins)

	set, err := s.HSetNX("user:1", "name", "bob")
	check("HSETNX", err)
	fmt.Println("  HSETNX existing field (expect false):", set)

	keys, err := s.HKeys("user:1")
	check("HKEYS", err)
	sort.Strings(keys)
	fmt.Println("  HKEYS =", keys)

	exists, err := s.HExists("user:1", "email")
	check("HEXISTS", err)
	n, err := s.HLen("user:1")
	check("HLEN", err)
	fmt.Printf("  HEXISTS email=%v  HLEN=%d\n", exists, n)

	removed, err := s.HDel("user:1", "email")
	check("HDEL", err)
	fmt.Println("  HDEL email removed:", removed)
}

// ---------------------------------------------------------------------- lists

func lists(s *redis.Store) {
	section("lists: RPUSH / LPUSH / LRANGE / LPOP / LINSERT / LMOVE")

	n, err := s.RPush("queue", "b", "c", "d")
	check("RPUSH", err)
	fmt.Println("  RPUSH -> len", n)
	n, err = s.LPush("queue", "a")
	check("LPUSH", err)
	fmt.Println("  LPUSH -> len", n)

	items, err := s.LRange("queue", 0, -1)
	check("LRANGE", err)
	fmt.Println("  LRANGE queue 0 -1 =", items)

	head, ok, err := s.LPop("queue")
	check("LPOP", err)
	tail, _, _ := s.RPop("queue")
	fmt.Printf("  LPOP=%q (ok=%v)  RPOP=%q\n", head, ok, tail)

	_, err = s.LInsert("queue", true, "c", "b2")
	check("LINSERT", err)
	items, _ = s.LRange("queue", 0, -1)
	fmt.Println("  after LINSERT BEFORE c:", items)

	check("LSET", s.LSet("queue", 0, "B"))
	idx, ok, err := s.LIndex("queue", 0)
	check("LINDEX", err)
	fmt.Printf("  LSET/LINDEX 0 = %q (ok=%v)\n", idx, ok)

	moved, ok, err := s.LMove("queue", "done", redis.ListLeft, redis.ListRight)
	check("LMOVE", err)
	done, _ := s.LRange("done", 0, -1)
	fmt.Printf("  LMOVE queue->done moved=%q (ok=%v) done=%v\n", moved, ok, done)

	llen, err := s.LLen("queue")
	check("LLEN", err)
	fmt.Println("  LLEN queue =", llen)

	key, popped, ok, err := s.LMPop(redis.DirLeft, 2, "queue", "done")
	check("LMPOP", err)
	fmt.Printf("  LMPOP LEFT 2 -> key=%q values=%v (ok=%v)\n", key, popped, ok)
}

// ----------------------------------------------------------------------- sets

func sets(s *redis.Store) {
	section("sets: SADD / SMEMBERS / SINTER / SUNION / SDIFF")

	added, err := s.SAdd("tags:a", "go", "redis", "db")
	check("SADD", err)
	_, err = s.SAdd("tags:b", "redis", "cache")
	check("SADD", err)
	fmt.Println("  SADD tags:a added:", added)

	members, err := s.SMembers("tags:a")
	check("SMEMBERS", err)
	sort.Strings(members)
	fmt.Println("  SMEMBERS tags:a =", members)

	isMember, err := s.SIsMember("tags:a", "go")
	check("SISMEMBER", err)
	card, err := s.SCard("tags:a")
	check("SCARD", err)
	fmt.Printf("  SISMEMBER go=%v  SCARD=%d\n", isMember, card)

	inter, err := s.SInter("tags:a", "tags:b")
	check("SINTER", err)
	union, err := s.SUnion("tags:a", "tags:b")
	check("SUNION", err)
	diff, err := s.SDiff("tags:a", "tags:b")
	check("SDIFF", err)
	sort.Strings(inter)
	sort.Strings(union)
	sort.Strings(diff)
	fmt.Println("  SINTER =", inter)
	fmt.Println("  SUNION =", union)
	fmt.Println("  SDIFF  =", diff)

	flags, err := s.SMIsMember("tags:a", "go", "nope")
	check("SMISMEMBER", err)
	fmt.Println("  SMISMEMBER [go nope] =", flags)

	ok, err := s.SMove("tags:a", "tags:b", "db")
	check("SMOVE", err)
	fmt.Println("  SMOVE db tags:a->tags:b =", ok)

	removed, err := s.SRem("tags:b", "cache")
	check("SREM", err)
	fmt.Println("  SREM cache from tags:b =", removed)
}

// ---------------------------------------------------------------- sorted sets

func sortedSets(s *redis.Store) {
	section("sorted sets: ZADD / ZRANGE / ZRANGEBYSCORE / ZINCRBY / ZPOPMAX")

	added, err := s.ZAdd("board",
		redis.ZMember{Member: "alice", Score: 30},
		redis.ZMember{Member: "bob", Score: 25},
		redis.ZMember{Member: "carol", Score: 42},
	)
	check("ZADD", err)
	fmt.Println("  ZADD board added:", added)

	asc, err := s.ZRange("board", 0, -1)
	check("ZRANGE", err)
	fmt.Println("  ZRANGE  0 -1 =", fmtZ(asc))

	desc, err := s.ZRevRange("board", 0, -1)
	check("ZREVRANGE", err)
	fmt.Println("  ZREVRANGE 0 -1 =", fmtZ(desc))

	score, ok, err := s.ZScore("board", "bob")
	check("ZSCORE", err)
	rank, _, err := s.ZRank("board", "bob")
	check("ZRANK", err)
	revRank, _, err := s.ZRevRank("board", "bob")
	check("ZREVRANK", err)
	fmt.Printf("  bob: score=%v (ok=%v) rank=%d revrank=%d\n", score, ok, rank, revRank)

	byScore, err := s.ZRangeByScore("board", redis.ScoreRange{Min: 26, Max: math.Inf(1)})
	check("ZRANGEBYSCORE", err)
	fmt.Println("  ZRANGEBYSCORE (26,+inf] =", fmtZ(byScore))

	excl, err := s.ZRangeByScore("board", redis.ScoreRange{
		Min: 30, Max: math.Inf(1), MinExclusive: true,
	})
	check("ZRANGEBYSCORE excl", err)
	fmt.Println("  ZRANGEBYSCORE (30..+inf exclusive min) =", fmtZ(excl))

	ns, err := s.ZIncrBy("board", 5, "bob")
	check("ZINCRBY", err)
	fmt.Println("  ZINCRBY bob +5 =", ns)

	count, err := s.ZCount("board", redis.ScoreRange{Min: 0, Max: 100})
	check("ZCOUNT", err)
	card, err := s.ZCard("board")
	check("ZCARD", err)
	fmt.Printf("  ZCOUNT [0,100]=%d  ZCARD=%d\n", count, card)

	top, err := s.ZPopMax("board", 1)
	check("ZPOPMAX", err)
	fmt.Println("  ZPOPMAX 1 =", fmtZ(top))

	removed, err := s.ZRem("board", "bob")
	check("ZREM", err)
	fmt.Println("  ZREM bob =", removed)
}

// --------------------------------------------------------------- key expiry

func expiryWithManualClock() {
	section("key expiry (deterministic, via ManualClock)")

	clock := redis.NewManualClock(time.Now())
	s := redis.NewWithClock(clock)

	s.Set("short", "value", redis.SetOptions{})
	fmt.Println("  EXPIRE short 2s:", s.Expire("short", 2*time.Second))

	ttl, code := s.TTL("short")
	fmt.Printf("  TTL before advance = %v (code=%d)\n", ttl, code)
	fmt.Println("  EXISTS short:", s.Exists("short"))

	clock.Advance(3 * time.Second)

	_, ok, err := s.Get("short")
	check("GET after expiry", err)
	_, code = s.TTL("short")
	fmt.Printf("  after clock.Advance(3s): GET found=%v  TTL code=%d (TTLNoKey=%d)\n",
		ok, code, redis.TTLNoKey)
	fmt.Println("  EXISTS short:", s.Exists("short"), " DBSIZE:", s.DBSize())

	// ExpireAt / ExpireTime / ExpireWith conditions.
	s.Set("k", "v", redis.SetOptions{})
	deadline := clock.Now().Add(time.Minute)
	fmt.Println("  EXPIREAT k +60s:", s.ExpireAt("k", deadline))
	at, code := s.ExpireTime("k")
	fmt.Printf("  EXPIRETIME k = %v (code=%d)\n", at.Sub(clock.Now()).Round(time.Second), code)
	fmt.Println("  EXPIRE k NX (already has TTL, expect false):",
		s.ExpireWith("k", time.Hour, redis.ExpireCondNX))
	fmt.Println("  EXPIRE k GT 2h (expect true):",
		s.ExpireWith("k", 2*time.Hour, redis.ExpireCondGT))
}

// -------------------------------------------------------------------- SCAN

func scanning(s *redis.Store) {
	section("iteration: KEYS pattern / SCAN / HSCAN")

	scanStore := redis.New()
	for i := 0; i < 7; i++ {
		scanStore.Set(fmt.Sprintf("item:%d", i), "v", redis.SetOptions{})
	}
	scanStore.Set("other", "v", redis.SetOptions{})

	matched := scanStore.Keys("item:*")
	sort.Strings(matched)
	fmt.Println("  KEYS item:* =", matched)

	var seen []string
	cursor := uint64(0)
	for {
		res := scanStore.Scan(cursor, "item:*", 3)
		seen = append(seen, res.Keys...)
		cursor = res.Cursor
		if cursor == 0 {
			break
		}
	}
	sort.Strings(seen)
	fmt.Printf("  SCAN MATCH item:* COUNT 3 -> %d keys %v\n", len(seen), seen)

	_, err := s.HSet("scan:hash", "f1", "1", "f2", "2", "f3", "3")
	check("HSET", err)
	pairs, err := s.HScan("scan:hash", 0, "*", 10)
	check("HSCAN", err)
	fmt.Printf("  HSCAN scan:hash -> cursor=%d pairs=%v\n", pairs.Cursor, pairs.Pairs)
}

// -------------------------------------------------------------- transactions

func transactions(s *redis.Store) {
	section("transactions: MULTI / QUEUE / EXEC / WATCH")

	s.Set("balance", "100", redis.SetOptions{})

	res, err := s.Multi().
		Queue("INCRBY", "balance", "50").
		Queue("SET", "flag", "on").
		Queue("GET", "balance").
		Exec()
	check("EXEC", err)
	fmt.Printf("  EXEC results = %v\n", res)

	// WATCH detects a concurrent modification and aborts.
	tx := s.Multi().Watch("balance").Queue("INCRBY", "balance", "1")
	s.Set("balance", "999", redis.SetOptions{}) // racing writer
	_, err = tx.Exec()
	fmt.Printf("  EXEC after watched key changed -> errors.Is(ErrTxAborted)=%v (%v)\n",
		errors.Is(err, redis.ErrTxAborted), err)
	bal, _, _ := s.Get("balance")
	fmt.Printf("  balance untouched by aborted tx = %q\n", bal)

	// WATCH that still matches lets the transaction through.
	tx2 := s.Multi().Watch("balance").Queue("INCRBY", "balance", "1")
	res, err = tx2.Exec()
	check("EXEC watched-ok", err)
	fmt.Printf("  EXEC with unchanged watch = %v\n", res)

	// An unknown command poisons the whole transaction at queue time.
	_, err = s.Multi().Queue("SET", "x", "1").Queue("NOSUCHCMD").Exec()
	fmt.Printf("  EXEC with bad command -> errors.Is(ErrUnknownCommand)=%v (%v)\n",
		errors.Is(err, redis.ErrUnknownCommand), err)
	fmt.Println("  EXISTS x after aborted queue (expect 0):", s.Exists("x"))

	// DISCARD.
	tx3 := s.Multi().Queue("SET", "y", "1")
	tx3.Discard()
	_, err = tx3.Exec()
	fmt.Printf("  EXEC after DISCARD -> %v\n", err)
}

// -------------------------------------------------------------------- pub/sub

func pubsub(s *redis.Store) {
	section("pub/sub: SUBSCRIBE / PSUBSCRIBE / PUBLISH")

	sub := s.Subscribe("news", "sports")
	defer sub.Close()
	psub := s.PSubscribe("news.*")
	defer psub.Close()

	fmt.Println("  subscription channels:", sub.Channels())
	fmt.Println("  PUBSUB CHANNELS:", sortedCopy(s.PubSubChannels("*")))
	fmt.Println("  PUBSUB NUMSUB news:", s.PubSubNumSub("news")["news"])
	fmt.Println("  PUBSUB NUMPAT:", s.PubSubNumPat())

	fmt.Println("  PUBLISH news 'hello' -> receivers:", s.Publish("news", "hello"))
	fmt.Println("  PUBLISH news.tech 'go' -> receivers:", s.Publish("news.tech", "go"))
	fmt.Println("  PUBLISH nobody 'x' -> receivers:", s.Publish("nobody", "x"))

	// Drain with a timeout so this can never block forever.
	drain(sub, 1, "exact")
	drain(psub, 1, "pattern")

	sub.Close()
	fmt.Println("  after Close, PUBLISH news -> receivers:", s.Publish("news", "again"))
	if _, open := <-sub.Messages(); !open {
		fmt.Println("  Messages() channel closed after Close: true")
	}
}

func drain(sub *redis.Subscription, want int, label string) {
	deadline := time.After(2 * time.Second)
	for i := 0; i < want; i++ {
		select {
		case m, open := <-sub.Messages():
			if !open {
				fmt.Printf("  [%s] channel closed early\n", label)
				return
			}
			fmt.Printf("  [%s] channel=%q pattern=%q payload=%q\n", label, m.Channel, m.Pattern, m.Payload)
		case <-deadline:
			fmt.Printf("  [%s] TIMEOUT waiting for message\n", label)
			return
		}
	}
}

// ----------------------------------------------------------- error handling

func wrongType(s *redis.Store) {
	section("error handling: wrong-type operations")

	s.Set("str", "not-a-collection", redis.SetOptions{})

	_, err := s.LPush("str", "x")
	fmt.Printf("  LPUSH on string -> WRONGTYPE=%v (%v)\n", errors.Is(err, redis.ErrWrongType), err)

	_, _, err = s.HGet("str", "f")
	fmt.Printf("  HGET  on string -> WRONGTYPE=%v\n", errors.Is(err, redis.ErrWrongType))

	_, err = s.SAdd("str", "m")
	fmt.Printf("  SADD  on string -> WRONGTYPE=%v\n", errors.Is(err, redis.ErrWrongType))

	_, err = s.ZAdd("str", redis.ZMember{Member: "m", Score: 1})
	fmt.Printf("  ZADD  on string -> WRONGTYPE=%v\n", errors.Is(err, redis.ErrWrongType))

	_, _ = s.RPush("alist", "a")
	_, _, err = s.Get("alist")
	fmt.Printf("  GET   on list   -> WRONGTYPE=%v\n", errors.Is(err, redis.ErrWrongType))

	_, err = s.Do("NOSUCHCOMMAND")
	fmt.Printf("  Do(unknown)     -> ErrUnknownCommand=%v\n", errors.Is(err, redis.ErrUnknownCommand))

	_, err = s.Do("GET")
	fmt.Printf("  Do(\"GET\") no args -> ErrWrongArgs=%v\n", errors.Is(err, redis.ErrWrongArgs))

	fmt.Println("  TYPE str =", s.TypeOf("str"), " TYPE absent =", s.TypeOf("absent"))
}

// ------------------------------------------------------------------ snapshots

func snapshots(s *redis.Store) {
	section("persistence: MarshalSnapshot / NewFromSnapshot / DUMP+RESTORE")

	src := redis.New()
	src.Set("s", "v", redis.SetOptions{EX: time.Hour})
	_, _ = src.RPush("l", "1", "2")
	_, _ = src.HSet("h", "f", "v")
	_, _ = src.SAdd("set", "m")
	_, _ = src.ZAdd("z", redis.ZMember{Member: "m", Score: 1})

	blob, err := src.MarshalSnapshot()
	check("MarshalSnapshot", err)
	fmt.Printf("  snapshot size = %d bytes, DBSIZE(src)=%d\n", len(blob), src.DBSize())

	restored, err := redis.NewFromSnapshot(blob)
	if err != nil {
		check("NewFromSnapshot", err)
		return
	}
	keys := restored.Keys("*")
	sort.Strings(keys)
	fmt.Println("  restored keys =", keys)
	l, _ := restored.LRange("l", 0, -1)
	fmt.Println("  restored list l =", l)
	_, code := restored.TTL("s")
	fmt.Printf("  restored TTL code for s = %d (TTLValue=%d)\n", code, redis.TTLValue)

	dumped, ok, err := src.DumpKey("h")
	check("DumpKey", err)
	fmt.Printf("  DUMP h -> %d bytes (ok=%v)\n", len(dumped), ok)
	check("RestoreKey", s.RestoreKey("h:copy", dumped, 0, true))
	copied, err := s.HGetAll("h:copy")
	check("HGETALL copy", err)
	fmt.Println("  RESTORE h:copy =", sortedMap(copied))
}

// --------------------------------------------------------------------- extras

func extras(s *redis.Store) {
	section("extras: bitmaps / HyperLogLog / streams / geo")

	_, err := s.SetBit("bits", 7, 1)
	check("SETBIT", err)
	_, _ = s.SetBit("bits", 0, 1)
	bit, err := s.GetBit("bits", 7)
	check("GETBIT", err)
	pop, err := s.BitCount("bits", 0, -1, false)
	check("BITCOUNT", err)
	fmt.Printf("  SETBIT/GETBIT bit7=%d BITCOUNT=%d\n", bit, pop)

	_, err = s.PFAdd("hll", "a", "b", "c", "a")
	check("PFADD", err)
	est, err := s.PFCount("hll")
	check("PFCOUNT", err)
	fmt.Printf("  PFADD a,b,c,a -> PFCOUNT ~= %d\n", est)

	id, err := s.XAdd("events", "*", "kind", "click", "page", "/home")
	check("XADD", err)
	_, err = s.XAdd("events", "*", "kind", "scroll")
	check("XADD", err)
	entries, err := s.XRange("events", "-", "+", 10)
	check("XRANGE", err)
	fmt.Printf("  XADD first id=%s XLEN=%d XRANGE entries=%d\n", id, s.XLen("events"), len(entries))

	_, err = s.GeoAdd("cities",
		redis.GeoMember{Member: "london", Longitude: -0.1278, Latitude: 51.5074},
		redis.GeoMember{Member: "paris", Longitude: 2.3522, Latitude: 48.8566},
	)
	check("GEOADD", err)
	dist, ok, err := s.GeoDist("cities", "london", "paris", redis.GeoKilometers)
	check("GEODIST", err)
	fmt.Printf("  GEODIST london..paris = %.1f km (ok=%v)\n", dist, ok)

	fmt.Println("  DBSIZE =", s.DBSize())
}

// ------------------------------------------------- RESP server + RESP client

// respClient is a tiny client built from the library's own Encoder/Decoder.
type respClient struct {
	conn net.Conn
	enc  *redis.Encoder
	dec  *redis.Decoder
}

func dial(addr string) (*respClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, err
	}
	return &respClient{conn: conn, enc: redis.NewEncoder(conn), dec: redis.NewDecoder(conn)}, nil
}

func (c *respClient) send(args ...string) error { return c.enc.Encode(args) }

func (c *respClient) read() (any, error) { return c.dec.Decode() }

func (c *respClient) do(args ...string) (any, error) {
	if err := c.send(args...); err != nil {
		return nil, err
	}
	return c.read()
}

func (c *respClient) close() { _ = c.conn.Close() }

func serverRoundTrip() error {
	section("RESP server: in-process TCP server on 127.0.0.1:0 + RESP client")

	store := redis.New()
	srv := redis.NewServer(store)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	serveDone := make(chan struct{})
	go func() {
		defer close(serveDone)
		_ = srv.Serve(ln) // returns net.ErrClosed after Close
	}()
	defer func() {
		_ = srv.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			fmt.Println("  !! server goroutine did not exit within 3s")
		}
	}()

	addr := srv.Addr()
	if addr == nil {
		// Addr is set inside Serve; give the goroutine a moment.
		time.Sleep(50 * time.Millisecond)
		addr = srv.Addr()
	}
	if addr == nil {
		return errors.New("server Addr() is nil")
	}
	fmt.Println("  serving on", addr.String())

	c, err := dial(addr.String())
	if err != nil {
		return err
	}
	defer c.close()

	for _, cmd := range [][]string{
		{"SET", "k", "v", "EX", "60"},
		{"GET", "k"},
		{"TTL", "k"},
		{"INCRBY", "counter", "7"},
		{"RPUSH", "list", "a", "b"},
		{"LRANGE", "list", "0", "-1"},
		{"HSET", "h", "f", "1"},
		{"HGETALL", "h"},
		{"SADD", "s", "x", "y"},
		{"SCARD", "s"},
		{"ZADD", "z", "1", "one", "2", "two"},
		{"ZRANGE", "z", "0", "-1", "WITHSCORES"},
		{"TYPE", "k"},
		{"DBSIZE"},
	} {
		reply, err := c.do(cmd...)
		if err != nil {
			return fmt.Errorf("%v: %w", cmd, err)
		}
		fmt.Printf("  > %-38s %s\n", strings.Join(cmd, " "), fmtReply(reply))
	}

	// Pipelining: write several commands before reading any reply.
	section("RESP server: pipelining")
	pipeline := [][]string{
		{"SET", "p1", "one"},
		{"SET", "p2", "two"},
		{"APPEND", "p1", "!"},
		{"GET", "p1"},
		{"GET", "p2"},
	}
	for _, cmd := range pipeline {
		if err := c.send(cmd...); err != nil {
			return err
		}
	}
	for _, cmd := range pipeline {
		reply, err := c.read()
		if err != nil {
			return err
		}
		fmt.Printf("  pipelined %-22s -> %s\n", strings.Join(cmd, " "), fmtReply(reply))
	}

	// Wrong-type over the wire comes back as a RESP error reply.
	section("RESP server: error replies")
	reply, err := c.do("LPUSH", "k", "x")
	if err != nil {
		return err
	}
	respErr, isErr := reply.(redis.RESPError)
	fmt.Printf("  LPUSH on string key -> RESPError=%v: %v\n", isErr, respErr)

	reply, err = c.do("MULTI")
	if err != nil {
		return err
	}
	// HOLE: MULTI/EXEC, SUBSCRIBE and PUBLISH are absent from the dispatch table,
	// so they are unavailable to any RESP client (including redis-cli). They exist
	// only as Go APIs (Store.Multi / Store.Subscribe / Store.Publish).
	fmt.Printf("  MULTI over RESP -> %s   (HOLE: not in dispatch table)\n", fmtReply(reply))

	reply, err = c.do("SUBSCRIBE", "news")
	if err != nil {
		return err
	}
	fmt.Printf("  SUBSCRIBE over RESP -> %s   (HOLE: not in dispatch table)\n", fmtReply(reply))

	return nil
}

// ------------------------------------------------------------------- helpers

func fmtReply(v any) string {
	switch val := v.(type) {
	case nil:
		return "(nil)"
	case redis.SimpleString:
		return "+" + string(val)
	case redis.RESPError:
		return "-" + string(val)
	case int64:
		return fmt.Sprintf("(integer) %d", val)
	case string:
		return fmt.Sprintf("%q", val)
	case []any:
		parts := make([]string, len(val))
		for i, e := range val {
			parts[i] = fmtReply(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func fmtZ(ms []redis.ZMember) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = fmt.Sprintf("%s=%g", m.Member, m.Score)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func sortedMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
