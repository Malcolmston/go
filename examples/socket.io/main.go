// Command socketio-example exercises github.com/malcolmston/socketio end to end:
// it starts a Socket.IO server on a random loopback port, connects to it with
// the library's own Go client, and drives every headline feature — events with
// arguments, acknowledgements in both directions, namespaces with connection
// middleware, rooms and broadcasts, a catch-all handler, binary payloads, and
// disconnect notification — then shuts everything down and exits.
//
// It is entirely self-contained: no outbound network access, no fixed ports, and
// every wait is bounded by a timeout so the program always terminates.
//
// Run it with:
//
//	GOWORK=off go run .
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"time"

	socketio "github.com/malcolmston/socketio"
	"github.com/malcolmston/socketio/client"
)

const (
	// ackTimeout bounds every request/response round trip.
	ackTimeout = 3 * time.Second
	// waitTimeout bounds every "did this event arrive?" wait.
	waitTimeout = 5 * time.Second
	// quietWindow is how long we wait to prove an event was NOT delivered.
	quietWindow = 400 * time.Millisecond
)

// failures counts checks that did not hold, so the process can exit non-zero
// without aborting the remaining demonstrations.
var failures atomic.Int32

func main() {
	log.SetFlags(0)

	// ------------------------------------------------------------------
	// Server setup. httptest gives us a real *http.Server on a random
	// loopback port; *socketio.Server is an http.Handler, so it mounts
	// directly. (socketio.DefaultPath is "/socket.io/", which is also the
	// path the Go client appends, so serving the handler at the root works.)
	// ------------------------------------------------------------------
	io := socketio.New()

	// Captured server-side sockets, so the main goroutine can drive them.
	rootSockets := make(chan *socketio.Socket, 8)
	// Events seen by the catch-all handler.
	anyEvents := make(chan string, 32)
	// Disconnect reasons reported by the server.
	disconnects := make(chan string, 8)

	io.OnConnection(func(s *socketio.Socket) {
		// Per-socket state, the equivalent of socket.data in Node.
		s.Set("joinedAt", time.Now().Format(time.RFC3339Nano))

		// Catch-all: fires for every inbound event on this socket, before the
		// named handlers.
		s.OnAny(func(event string, args []any) {
			select {
			case anyEvents <- event:
			default:
			}
		})

		// Plain event with arguments, echoed straight back to the sender.
		s.On("echo", func(args []any) []any {
			if err := s.Emit("echo:reply", args...); err != nil {
				log.Printf("echo reply failed: %v", err)
			}
			return nil
		})

		// Acknowledgement: a non-nil return value becomes the ACK payload,
		// delivered only if the client asked for one.
		s.On("sum", func(args []any) []any {
			total := 0.0
			for _, a := range args {
				if f, ok := a.(float64); ok {
					total += f
				}
			}
			return []any{total, len(args)}
		})

		// Binary payload in, binary payload out (as an ACK).
		s.On("upload", func(args []any) []any {
			raw, _ := args[0].([]byte)
			return []any{append([]byte("sha:"), raw...), len(raw)}
		})

		// Binary payload pushed from server to client, unsolicited.
		s.On("download", func(args []any) []any {
			if err := s.Emit("blob", []byte{0x00, 0x7f, 0x80, 0xff}, "trailer"); err != nil {
				log.Printf("blob emit failed: %v", err)
			}
			return nil
		})

		// Rooms: the client asks to be put in a room.
		s.On("join", func(args []any) []any {
			room, _ := args[0].(string)
			s.Join(room)
			return []any{s.Rooms()}
		})

		// Broadcast excluding the sender.
		s.On("gossip", func(args []any) []any {
			s.Broadcast().Emit("gossip", args...)
			return nil
		})

		// Room-scoped broadcast excluding the sender.
		s.On("shout", func(args []any) []any {
			room, _ := args[0].(string)
			s.To(room).Emit("shout", args[1:]...)
			return nil
		})

		s.OnDisconnect(func(reason string) {
			select {
			case disconnects <- reason:
			default:
			}
		})

		select {
		case rootSockets <- s:
		default:
		}
	})

	// A second namespace, gated by connection middleware that inspects the
	// CONNECT auth payload.
	adminSockets := make(chan *socketio.Socket, 4)
	admin := io.Of("/admin")
	admin.Use(func(s *socketio.Socket, next func(err error)) {
		auth, _ := s.Auth().(map[string]any)
		if auth == nil || auth["token"] != "s3cret" {
			next(errors.New("unauthorized"))
			return
		}
		next(nil)
	})
	admin.OnConnection(func(s *socketio.Socket) {
		s.On("whoami", func(args []any) []any {
			return []any{"admin", s.Namespace().Name()}
		})
		select {
		case adminSockets <- s:
		default:
		}
	})

	srv := httptest.NewServer(http.HandlerFunc(io.ServeHTTP))
	defer srv.Close()
	defer io.Close()
	log.Printf("socket.io server listening on %s%s", srv.URL, socketio.DefaultPath)

	// ------------------------------------------------------------------
	// 1. Connect, and exchange an event with arguments.
	// ------------------------------------------------------------------
	section("events with arguments")
	c, err := client.Dial(srv.URL)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer c.Close()
	log.Printf("connected, client sid = %q", c.ID())

	echoed := make(chan []any, 1)
	c.On("echo:reply", func(args []any) []any { push(echoed, args); return nil })
	if err := c.Emit("echo", "hello", float64(42), true); err != nil {
		log.Fatalf("emit echo: %v", err)
	}
	if args, ok := wait(echoed, "echo:reply"); ok {
		check("echo arg count", len(args) == 3, fmt.Sprintf("%v", args))
		check("echo string arg", args[0] == "hello", fmt.Sprintf("%v", args[0]))
		// JSON numbers always decode to float64 — there is no int on the wire.
		check("echo number arg", args[1] == float64(42), fmt.Sprintf("%T %v", args[1], args[1]))
		check("echo bool arg", args[2] == true, fmt.Sprintf("%v", args[2]))
	}

	// Wait for the server-side socket so later steps can drive it.
	var serverSock *socketio.Socket
	select {
	case serverSock = <-rootSockets:
	case <-time.After(waitTimeout):
		log.Fatal("server never reported the connection")
	}
	if v, ok := serverSock.Get("joinedAt"); ok {
		log.Printf("server-side socket %s, joinedAt=%v", serverSock.ID(), v)
	}

	// ------------------------------------------------------------------
	// 2. Client -> server acknowledgement.
	// ------------------------------------------------------------------
	section("client -> server acknowledgement")
	reply, err := c.EmitWithAck("sum", ackTimeout, float64(1), float64(2), float64(4))
	if err != nil {
		check("client EmitWithAck", false, err.Error())
	} else {
		check("ack payload", len(reply) == 2 && reply[0] == float64(7) && reply[1] == float64(3),
			fmt.Sprintf("%v", reply))
		log.Printf("sum(1,2,4) acked with %v", reply)
	}

	// ------------------------------------------------------------------
	// 3. Server -> client acknowledgement, both the blocking and the
	//    callback flavour. Note these must NOT be called from inside a
	//    server event handler: handlers run on the connection's read loop,
	//    so blocking there would deadlock the very loop that delivers the
	//    ACK. Driving them from the main goroutine is the correct pattern.
	// ------------------------------------------------------------------
	section("server -> client acknowledgement")
	c.On("ping", func(args []any) []any {
		// A non-nil return value from a client handler becomes the ACK.
		return []any{"pong", args[0]}
	})
	got, err := serverSock.EmitAck("ping", ackTimeout, "seq-1")
	if err != nil {
		check("Socket.EmitAck", false, err.Error())
	} else {
		check("EmitAck payload", len(got) == 2 && got[0] == "pong" && got[1] == "seq-1",
			fmt.Sprintf("%v", got))
		log.Printf("EmitAck round trip: %v", got)
	}

	cbAck := make(chan []any, 1)
	if err := serverSock.EmitWithAck("ping", func(args []any) { push(cbAck, args) }, "seq-2"); err != nil {
		check("Socket.EmitWithAck", false, err.Error())
	} else if args, ok := wait(cbAck, "EmitWithAck callback"); ok {
		check("EmitWithAck callback payload", len(args) == 2 && args[1] == "seq-2", fmt.Sprintf("%v", args))
	}
	// HOLE: Socket.PendingAcks() is not in the published v0.4.0 — the field is
	// unexported (pendingAcks) and there is no accessor, so a consumer cannot
	// assert that ack bookkeeping was released. The method exists only in the
	// repository's uncommitted working tree.

	// HOLE: an event the peer has no handler for is still acknowledged, with an
	// empty payload. Client.dispatch (and Socket.dispatch on the server) send an
	// ACK whenever the inbound packet carries an ack id, without checking that a
	// handler actually ran or that it returned anything. In socket.io the ack is
	// sent only when the handler invokes the callback, so "the peer did not
	// handle this" is observable there and is not here. Consequences:
	//   - Socket.EmitAck can never time out against this Go client, so the
	//     timeout path is unreachable in practice; the caller gets a successful
	//     empty []any instead. (v0.4.0 exports no ErrAckTimeout sentinel at all
	//     — it exists only in the uncommitted working tree — so a caller cannot
	//     even name the error it would have to match.)
	//   - a caller cannot distinguish "handled, nothing to report" from
	//     "no handler registered".
	// The assertion below records the behaviour as it actually is.
	unhandled, err := serverSock.EmitAck("nobody-listens", 300*time.Millisecond)
	check("HOLE: unhandled event is auto-acked instead of timing out",
		err == nil && len(unhandled) == 0,
		fmt.Sprintf("reply=%v err=%v", unhandled, err))

	// ------------------------------------------------------------------
	// 4. Binary payloads, in both directions.
	// ------------------------------------------------------------------
	section("binary payloads")
	payload := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x0a}
	bin, err := c.EmitWithAck("upload", ackTimeout, payload)
	if err != nil {
		check("binary ack", false, err.Error())
	} else {
		blob, _ := bin[0].([]byte)
		check("binary ack round trip", bytes.Equal(blob, append([]byte("sha:"), payload...)),
			fmt.Sprintf("%v", bin[0]))
		check("binary ack length arg", bin[1] == float64(len(payload)), fmt.Sprintf("%v", bin[1]))
		log.Printf("uploaded %d bytes, server acked %q + %v", len(payload), blob, bin[1])
	}

	blobs := make(chan []any, 1)
	c.On("blob", func(args []any) []any { push(blobs, args); return nil })
	if err := c.Emit("download"); err != nil {
		check("emit download", false, err.Error())
	} else if args, ok := wait(blobs, "blob"); ok {
		b, _ := args[0].([]byte)
		check("server->client binary", bytes.Equal(b, []byte{0x00, 0x7f, 0x80, 0xff}), fmt.Sprintf("%v", args[0]))
		check("mixed binary+text args", len(args) == 2 && args[1] == "trailer", fmt.Sprintf("%v", args))
		log.Printf("received %d binary bytes alongside %v", len(b), args[1])
	}

	// ------------------------------------------------------------------
	// 5. Catch-all handler. Every event emitted above should have passed
	//    through Socket.OnAny.
	// ------------------------------------------------------------------
	section("catch-all handler")
	seen := drain(anyEvents)
	log.Printf("OnAny observed: %v", seen)
	for _, want := range []string{"echo", "sum", "upload", "download"} {
		check("OnAny saw "+want, contains(seen, want), fmt.Sprintf("%v", seen))
	}
	check("OnAny listener registered", len(serverSock.ListenersAny()) == 1,
		fmt.Sprintf("%d", len(serverSock.ListenersAny())))

	// ------------------------------------------------------------------
	// 6. Namespaces + connection middleware.
	// ------------------------------------------------------------------
	section("namespaces and middleware")
	if _, err := client.Dial(srv.URL, client.Options{
		Namespace:   "/admin",
		Auth:        map[string]any{"token": "wrong"},
		DialTimeout: ackTimeout,
	}); err == nil {
		check("middleware rejects bad auth", false, "Dial unexpectedly succeeded")
	} else {
		check("middleware rejects bad auth", true, "")
		log.Printf("rejected as expected: %v", err)
	}

	adminClient, err := client.Dial(srv.URL, client.Options{
		Namespace:   "/admin",
		Auth:        map[string]any{"token": "s3cret"},
		DialTimeout: ackTimeout,
	})
	if err != nil {
		check("middleware accepts good auth", false, err.Error())
	} else {
		defer adminClient.Close()
		check("middleware accepts good auth", true, "")
		who, err := adminClient.EmitWithAck("whoami", ackTimeout)
		if err != nil {
			check("/admin ack", false, err.Error())
		} else {
			check("/admin ack payload", len(who) == 2 && who[0] == "admin" && who[1] == "/admin",
				fmt.Sprintf("%v", who))
			log.Printf("/admin whoami -> %v", who)
		}
		// Namespaces are isolated: the root namespace's sockets are not the
		// /admin namespace's sockets.
		check("namespace isolation", len(admin.FetchSockets()) == 1 && len(io.FetchSockets()) == 1,
			fmt.Sprintf("admin=%d root=%d", len(admin.FetchSockets()), len(io.FetchSockets())))
	}

	// ------------------------------------------------------------------
	// 7. Rooms and broadcasting, with three participants.
	// ------------------------------------------------------------------
	section("rooms and broadcasting")
	b, err := client.Dial(srv.URL, client.Options{DialTimeout: ackTimeout})
	if err != nil {
		log.Fatalf("dial b: %v", err)
	}
	defer b.Close()
	d, err := client.Dial(srv.URL, client.Options{DialTimeout: ackTimeout})
	if err != nil {
		log.Fatalf("dial d: %v", err)
	}
	defer d.Close()

	announceA := make(chan []any, 4)
	announceB := make(chan []any, 4)
	announceD := make(chan []any, 4)
	c.On("announce", func(args []any) []any { push(announceA, args); return nil })
	b.On("announce", func(args []any) []any { push(announceB, args); return nil })
	d.On("announce", func(args []any) []any { push(announceD, args); return nil })

	shoutA := make(chan []any, 4)
	shoutB := make(chan []any, 4)
	shoutD := make(chan []any, 4)
	c.On("shout", func(args []any) []any { push(shoutA, args); return nil })
	b.On("shout", func(args []any) []any { push(shoutB, args); return nil })
	d.On("shout", func(args []any) []any { push(shoutD, args); return nil })

	gossipA := make(chan []any, 4)
	gossipB := make(chan []any, 4)
	gossipD := make(chan []any, 4)
	c.On("gossip", func(args []any) []any { push(gossipA, args); return nil })
	b.On("gossip", func(args []any) []any { push(gossipB, args); return nil })
	d.On("gossip", func(args []any) []any { push(gossipD, args); return nil })

	// A, B and D all join "lobby"; the ACK returns the socket's room list, so
	// a successful ack also proves the join happened server-side.
	for name, cl := range map[string]*client.Client{"a": c, "b": b, "d": d} {
		rooms, err := cl.EmitWithAck("join", ackTimeout, "lobby")
		if err != nil {
			check("join lobby ("+name+")", false, err.Error())
			continue
		}
		list, _ := rooms[0].([]any)
		// Every socket is implicitly in a room named after its own id, so
		// "lobby" is the second entry.
		check("join lobby ("+name+")", len(list) == 2 && contains(anyStrings(list), "lobby"),
			fmt.Sprintf("%v", rooms[0]))
	}
	check("server sees 3 root sockets", len(io.FetchSockets()) == 3,
		fmt.Sprintf("%d", len(io.FetchSockets())))
	check("lobby has 3 members", len(io.Of("/").SocketsInRoom("lobby")) == 3,
		fmt.Sprintf("%d", len(io.Of("/").SocketsInRoom("lobby"))))

	// Server-initiated room broadcast reaches every member.
	io.To("lobby").Emit("announce", "maintenance at 5pm", float64(17))
	for name, ch := range map[string]chan []any{"a": announceA, "b": announceB, "d": announceD} {
		if args, ok := wait(ch, "announce ("+name+")"); ok {
			check("announce payload ("+name+")",
				len(args) == 2 && args[0] == "maintenance at 5pm" && args[1] == float64(17),
				fmt.Sprintf("%v", args))
		}
	}

	// Socket.To(room) broadcasts to the room but excludes the sender.
	if err := c.Emit("shout", "lobby", "anyone there?"); err != nil {
		check("emit shout", false, err.Error())
	}
	if args, ok := wait(shoutB, "shout (b)"); ok {
		check("shout payload (b)", len(args) == 1 && args[0] == "anyone there?", fmt.Sprintf("%v", args))
	}
	if _, ok := wait(shoutD, "shout (d)"); !ok {
		check("shout reached d", false, "not delivered")
	}
	check("Socket.To excludes the sender", quiet(shoutA), "sender received its own room broadcast")

	// Socket.Broadcast() targets the whole namespace except the sender.
	if err := b.Emit("gossip", "psst"); err != nil {
		check("emit gossip", false, err.Error())
	}
	if args, ok := wait(gossipA, "gossip (a)"); ok {
		check("gossip payload (a)", len(args) == 1 && args[0] == "psst", fmt.Sprintf("%v", args))
	}
	if _, ok := wait(gossipD, "gossip (d)"); !ok {
		check("gossip reached d", false, "not delivered")
	}
	check("Socket.Broadcast excludes the sender", quiet(gossipB), "sender received its own broadcast")

	// Server-wide room management.
	io.SocketsJoin("vip")
	check("SocketsJoin put everyone in vip", len(io.Of("/").SocketsInRoom("vip")) == 3,
		fmt.Sprintf("%d", len(io.Of("/").SocketsInRoom("vip"))))
	io.SocketsLeave("vip")
	check("SocketsLeave emptied vip", len(io.Of("/").SocketsInRoom("vip")) == 0,
		fmt.Sprintf("%d", len(io.Of("/").SocketsInRoom("vip"))))

	// Server-to-server events (local-only without a cluster Broadcaster).
	serverEvents := make(chan []any, 1)
	io.OnServerEvent("cluster-ping", func(args []any) { push(serverEvents, args) })
	io.ServerSideEmit("cluster-ping", "node-1")
	if args, ok := wait(serverEvents, "cluster-ping"); ok {
		check("ServerSideEmit payload", len(args) == 1 && args[0] == "node-1", fmt.Sprintf("%v", args))
	}

	// ------------------------------------------------------------------
	// 8. Disconnect handling, from both ends.
	// ------------------------------------------------------------------
	section("disconnect handling")
	drain(disconnects)

	// Client-initiated close: the server's OnDisconnect must fire.
	clientGone := make(chan []any, 1)
	d.On("disconnect", func(args []any) []any { push(clientGone, args); return nil })
	if err := d.Close(); err != nil {
		check("client Close", false, err.Error())
	}
	if reason, ok := waitString(disconnects, "server OnDisconnect after client close"); ok {
		log.Printf("server saw disconnect: %q", reason)
	}

	// Server-initiated disconnect: the client's local "disconnect" event fires.
	serverGone := make(chan []any, 1)
	b.On("disconnect", func(args []any) []any { push(serverGone, args); return nil })
	io.DisconnectSockets(true)
	if _, ok := wait(serverGone, "client disconnect event after DisconnectSockets"); ok {
		log.Print("client observed the server-side disconnect")
	}
	if reason, ok := waitString(disconnects, "server OnDisconnect after DisconnectSockets"); ok {
		log.Printf("server-side disconnect reason: %q", reason)
	}

	// Emitting on a closed client must error rather than panic or block.
	if err := d.Emit("echo", "after close"); err == nil {
		check("emit after Close errors", false, "returned nil error")
	} else {
		check("emit after Close errors", true, "")
	}

	// ------------------------------------------------------------------
	// 9. Shutdown. Server.Close disconnects every session; the deferred
	//    srv.Close then tears down the HTTP listener.
	// ------------------------------------------------------------------
	section("shutdown")
	io.Close()
	check("all sockets gone after Server.Close", len(io.FetchSockets()) == 0,
		fmt.Sprintf("%d", len(io.FetchSockets())))

	if n := failures.Load(); n > 0 {
		log.Printf("\n%d check(s) FAILED", n)
		os.Exit(1)
	}
	log.Print("\nall checks passed")
}

// ---------------------------------------------------------------------------
// Small helpers. Everything that waits does so with a timeout, so the program
// cannot hang.
// ---------------------------------------------------------------------------

func section(name string) { log.Printf("\n=== %s ===", name) }

func check(what string, ok bool, detail string) {
	if ok {
		log.Printf("  ok   %s", what)
		return
	}
	failures.Add(1)
	if detail != "" {
		log.Printf("  FAIL %s: %s", what, detail)
		return
	}
	log.Printf("  FAIL %s", what)
}

// push is a non-blocking send, so a handler running on the read loop is never
// stalled by a full channel.
func push(ch chan []any, args []any) {
	select {
	case ch <- args:
	default:
	}
}

// wait receives from ch or fails the named check on timeout.
func wait(ch chan []any, what string) ([]any, bool) {
	select {
	case v := <-ch:
		return v, true
	case <-time.After(waitTimeout):
		check(what+" delivered", false, "timed out after "+waitTimeout.String())
		return nil, false
	}
}

func waitString(ch chan string, what string) (string, bool) {
	select {
	case v := <-ch:
		check(what, true, "")
		return v, true
	case <-time.After(waitTimeout):
		check(what, false, "timed out after "+waitTimeout.String())
		return "", false
	}
}

// quiet reports whether nothing arrived on ch within quietWindow — used to
// prove an event was correctly NOT delivered.
func quiet(ch chan []any) bool {
	select {
	case v := <-ch:
		log.Printf("  (unexpected delivery: %v)", v)
		return false
	case <-time.After(quietWindow):
		return true
	}
}

func drain(ch chan string) []string {
	var out []string
	for {
		select {
		case v := <-ch:
			out = append(out, v)
		default:
			return out
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func anyStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
