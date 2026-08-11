// Command run is the Go-side runner for the socket.io parity harness.
//
// It speaks the JSON-Lines contract from parity/HARNESS.md: one request object
// per line on stdin, exactly one response object per line on stdout, in order.
// Every handler is wrapped so a panic or error becomes {"ok":false,"error":...}
// rather than killing the process.
//
// Groups it serves (identical `fn` names to node/run.mjs):
//
//	wire.*      socketio.Encoder / socketio.Decoder (exact wire bytes)
//	eio.*       engineio packet / polling-payload framing
//	scenario.*  end-to-end behaviour transcript (own server + own client)
//	serve.*     start a server for cross-implementation interop, return its URL
//	drive.*     connect a client to a given URL, return the client transcript
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	socketio "github.com/malcolmston/socketio"
	"github.com/malcolmston/socketio/client"
	"github.com/malcolmston/socketio/engineio"
)

const (
	ackWait         = 400 * time.Millisecond
	stepWait        = 300 * time.Millisecond
	connectSettle   = 80 * time.Millisecond
	dialTimeout     = 2 * time.Second
	scenarioTimeout = 8 * time.Second
)

// ---------------------------------------------------------------- protocol

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<22)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeJSON(out, response{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		val, err := safeHandle(req)
		if err != nil {
			writeJSON(out, response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			writeJSON(out, response{ID: req.ID, OK: true, Value: val})
		}
	}
	if live.srv != nil {
		stopServer(live.srv, live.ts)
	}
}

func writeJSON(w *bufio.Writer, r response) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		return
	}
	w.Write(buf.Bytes())
	w.Flush()
}

func safeHandle(req request) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			val, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return handle(req)
}

// ---------------------------------------------------------------- json helpers

// jstr marshals v the way the node runner's j() does: []byte values become
// {"$bin":"<hex>"} and map keys are emitted in sorted order (Go's encoder sorts
// them; the node side sorts explicitly).
func jstr(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fromNative(v)); err != nil {
		return "<unmarshalable>"
	}
	return strings.TrimRight(buf.String(), "\n")
}

// toNative turns {"$bin":"<hex>"} markers from the case JSON into []byte.
func toNative(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = toNative(x[i])
		}
		return out
	case map[string]any:
		if len(x) == 1 {
			if s, ok := x["$bin"].(string); ok {
				b, err := hex.DecodeString(s)
				if err == nil {
					return b
				}
			}
		}
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = toNative(vv)
		}
		return out
	default:
		return v
	}
}

// fromNative turns []byte back into a {"$bin":"<hex>"} marker.
func fromNative(v any) any {
	switch x := v.(type) {
	case []byte:
		return map[string]any{"$bin": hex.EncodeToString(x)}
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = fromNative(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = fromNative(vv)
		}
		return out
	default:
		return v
	}
}

func argObj(req request, i int) (map[string]any, error) {
	if len(req.Args) <= i {
		return nil, fmt.Errorf("missing args[%d]", i)
	}
	var m map[string]any
	if err := json.Unmarshal(req.Args[i], &m); err != nil {
		return nil, err
	}
	return m, nil
}

func argStr(req request, i int) (string, error) {
	if len(req.Args) <= i {
		return "", fmt.Errorf("missing args[%d]", i)
	}
	var s string
	err := json.Unmarshal(req.Args[i], &s)
	return s, err
}

// ---------------------------------------------------------------- transcript

type transcript struct {
	mu    sync.Mutex
	lines []string
	ids   map[string]string
	order []string
}

func newTranscript() *transcript {
	return &transcript{ids: map[string]string{}}
}

func (t *transcript) sid(id string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id == "" {
		return "<nil>"
	}
	if v, ok := t.ids[id]; ok {
		return v
	}
	v := fmt.Sprintf("<S%d>", len(t.ids)+1)
	t.ids[id] = v
	return v
}

func (t *transcript) push(parts ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, strings.Join(parts, " "))
}

func (t *transcript) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.lines))
	copy(out, t.lines)
	if out == nil {
		out = []string{}
	}
	return out
}

func (t *transcript) has(line string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, l := range t.lines {
		if l == line {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- wire group

func wireEncode(spec map[string]any) (any, error) {
	p := socketio.Packet{Namespace: "/"}
	tf, ok := spec["type"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing type")
	}
	p.Type = socketio.PacketType(int(tf))
	if n, ok := spec["nsp"].(string); ok {
		p.Namespace = n
	}
	if idv, ok := spec["id"].(float64); ok {
		id := uint64(idv)
		p.ID = &id
	}
	if d, ok := spec["data"]; ok && d != nil {
		p.Data = toNative(d)
	}
	frames, err := socketio.NewEncoder().Encode(p)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(frames))
	for _, f := range frames {
		switch v := f.(type) {
		case string:
			out = append(out, v)
		case []byte:
			out = append(out, map[string]any{"$bin": hex.EncodeToString(v)})
		default:
			return nil, fmt.Errorf("unexpected frame type %T", f)
		}
	}
	return map[string]any{"frames": out}, nil
}

func feedFrames(spec map[string]any) ([]socketio.Packet, bool, error) {
	raw, ok := spec["frames"].([]any)
	if !ok {
		return nil, false, fmt.Errorf("missing frames")
	}
	dec := socketio.NewDecoder()
	var packets []socketio.Packet
	for _, f := range raw {
		var frame any
		switch v := f.(type) {
		case string:
			frame = v
		case map[string]any:
			s, _ := v["$bin"].(string)
			b, err := hex.DecodeString(s)
			if err != nil {
				return nil, false, err
			}
			frame = b
		default:
			return nil, false, fmt.Errorf("unexpected frame %T", f)
		}
		p, err := dec.Add(frame)
		if err != nil {
			return nil, false, err
		}
		if p != nil {
			packets = append(packets, *p)
		}
	}
	return packets, dec.Pending(), nil
}

func wireDecode(spec map[string]any) (any, error) {
	packets, pending, err := feedFrames(spec)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(packets))
	for _, p := range packets {
		var id any
		if p.ID != nil {
			id = float64(*p.ID)
		}
		var data any
		if p.Data != nil {
			data = fromNative(p.Data)
		}
		out = append(out, map[string]any{
			"type": int(p.Type),
			"nsp":  p.Namespace,
			"id":   id,
			"data": data,
		})
	}
	return map[string]any{"packets": out, "pending": pending}, nil
}

func wireDecodeAttachments(spec map[string]any) (any, error) {
	packets, _, err := feedFrames(spec)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(packets))
	for _, p := range packets {
		// A fully reassembled (or non-binary) packet has no declared attachments.
		// Upstream deletes packet.attachments, so it reads back as undefined/null;
		// the port clears the count to 0. Report 0 as null so the two compare.
		if n := p.Attachments(); n == 0 {
			out = append(out, nil)
		} else {
			out = append(out, n)
		}
	}
	return map[string]any{"attachments": out}, nil
}

// ---------------------------------------------------------------- eio group

func eioSpecToPacket(spec map[string]any) (engineio.Packet, error) {
	tf, ok := spec["type"].(float64)
	if !ok {
		return engineio.Packet{}, fmt.Errorf("missing type")
	}
	p := engineio.Packet{Type: engineio.PacketType(int(tf))}
	if b, ok := spec["binary"].(string); ok {
		raw, err := hex.DecodeString(b)
		if err != nil {
			return engineio.Packet{}, err
		}
		p.Binary = raw
	} else if d, ok := spec["data"].(string); ok {
		p.Data = d
	}
	return p, nil
}

func normEioPacket(p engineio.Packet) map[string]any {
	m := map[string]any{"type": int(p.Type), "data": p.Data, "binary": nil}
	if p.Binary != nil {
		m["binary"] = hex.EncodeToString(p.Binary)
		m["data"] = ""
	}
	return m
}

func eioEncodePacket(spec map[string]any) (any, error) {
	p, err := eioSpecToPacket(spec)
	if err != nil {
		return nil, err
	}
	return map[string]any{"encoded": p.Encode()}, nil
}

func eioDecodePacket(spec map[string]any) (any, error) {
	s, _ := spec["encoded"].(string)
	p, err := engineio.Decode(s)
	if err != nil {
		return nil, err
	}
	return map[string]any{"packet": normEioPacket(p)}, nil
}

func eioEncodePayload(spec map[string]any) (any, error) {
	raw, ok := spec["packets"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing packets")
	}
	pkts := make([]engineio.Packet, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("bad packet spec")
		}
		p, err := eioSpecToPacket(m)
		if err != nil {
			return nil, err
		}
		pkts = append(pkts, p)
	}
	return map[string]any{"encoded": engineio.EncodePayload(pkts)}, nil
}

func eioDecodePayload(spec map[string]any) (any, error) {
	s, _ := spec["encoded"].(string)
	pkts, err := engineio.DecodePayload(s)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(pkts))
	for _, p := range pkts {
		out = append(out, normEioPacket(p))
	}
	return map[string]any{"packets": out}, nil
}

// ---------------------------------------------------------------- e2e plumbing

type serverBundle struct {
	srv *socketio.Server
	ts  *httptest.Server
	t   *transcript
	url string
}

func startServer(setup func(*socketio.Server, *transcript)) *serverBundle {
	srv := socketio.New(socketio.Options{
		// Long heartbeats keep the heartbeat out of the transcript entirely.
		PingInterval: 25 * time.Second,
		PingTimeout:  20 * time.Second,
	})
	t := newTranscript()
	setup(srv, t)
	mux := http.NewServeMux()
	srv.Attach(mux)
	ts := httptest.NewServer(mux)
	return &serverBundle{srv: srv, ts: ts, t: t, url: ts.URL}
}

func stopServer(srv *socketio.Server, ts *httptest.Server) {
	srv.Close()
	if ts != nil {
		ts.Close()
	}
}

func (b *serverBundle) stop() { stopServer(b.srv, b.ts) }

type connectResult struct {
	ok      bool
	message string
	client  *client.Client
}

func connect(url string, nsp string, auth any) connectResult {
	c, err := client.Dial(url, client.Options{
		Namespace:   nsp,
		Auth:        auth,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return connectResult{ok: false, message: normErr(err)}
	}
	return connectResult{ok: true, client: c}
}

// normErr strips the port's "client: " prefix so the message compares against
// the upstream Error.message.
func normErr(err error) string {
	s := err.Error()
	s = strings.TrimPrefix(s, "client: ")
	if s == "connect timeout" {
		// Upstream's socket.io-client surfaces a stalled handshake as a
		// "timeout" connect_error; the harness records "<no reply>" on both
		// sides so a hung handshake is comparable.
		return "<no reply>"
	}
	return s
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// emitAckClient mirrors the node runner's emitAck: full ack argument list or nil
// on timeout.
func emitAckClient(c *client.Client, ev string, args []any) []any {
	reply, err := c.EmitWithAck(ev, ackWait, args...)
	if err != nil {
		return nil
	}
	if reply == nil {
		return []any{}
	}
	return reply
}

func ackLine(prefix string, reply []any) string {
	if reply == nil {
		return prefix + "-timeout"
	}
	return prefix + " " + jstr(reply)
}

// ---------------------------------------------------------------- scenarios

var scenarios map[string]func() []string

func init() {
	scenarios = map[string]func() []string{
		"emit-scalar-args": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					t.push("srv:connection", t.sid(s.ID()))
					s.On("e", func(args []any) []any { t.push("srv:e", jstr(args)); return nil })
					s.On("done", func([]any) []any { t.push("srv:done"); return nil })
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			time.Sleep(connectSettle)
			b.t.push("cli:connect", boolStr(r.ok))
			if !r.ok {
				return b.t.snapshot()
			}
			defer r.client.Close()
			_ = r.client.Emit("e", "s", 1, 1.5, true, false, nil)
			_ = r.client.Emit("e")
			_ = r.client.Emit("e", map[string]any{"b": 1, "a": 2}, []any{1, []any{2, []any{3}}})
			_ = r.client.Emit("e", "", 0, -1, 1e21)
			_ = r.client.Emit("done")
			deadline := time.Now().Add(2 * time.Second)
			for !b.t.has("srv:done") && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			return b.t.snapshot()
		},

		"ack-values": func() []string {
			return ackScenario(func(s *socketio.Socket, t *transcript) {
				s.On("sum", func(args []any) []any {
					t.push("srv:sum", jstr(args))
					a, _ := args[0].(float64)
					bb, _ := args[1].(float64)
					return []any{a + bb}
				})
			}, "sum", []any{1, 2})
		},

		"ack-multi-values": func() []string {
			return ackScenario(func(s *socketio.Socket, t *transcript) {
				s.On("multi", func([]any) []any {
					t.push("srv:multi")
					return []any{1, "two", map[string]any{"k": 1}, nil}
				})
			}, "multi", nil)
		},

		"ack-empty-call": func() []string {
			return ackScenario(func(s *socketio.Socket, t *transcript) {
				s.On("empty", func([]any) []any {
					t.push("srv:empty")
					return []any{} // the port's equivalent of calling cb() with no args
				})
			}, "empty", nil)
		},

		// KNOWN PORT DIVERGENCE: the port acks an event with no handler.
		"ack-no-handler": func() []string {
			return ackScenario(func(*socketio.Socket, *transcript) {}, "ghost", []any{1})
		},

		// KNOWN PORT DIVERGENCE: a handler returning nil still produces an ack.
		"ack-handler-never-calls": func() []string {
			return ackScenario(func(s *socketio.Socket, t *transcript) {
				s.On("silent", func([]any) []any { t.push("srv:silent"); return nil })
			}, "silent", nil)
		},

		"ack-binary": func() []string {
			return ackScenario(func(s *socketio.Socket, t *transcript) {
				s.On("bin", func(args []any) []any {
					t.push("srv:bin", jstr(args))
					return []any{[]byte{9, 8, 7}}
				})
			}, "bin", []any{[]byte{0, 1, 255}})
		},

		"server-ack": func() []string {
			return serverAckScenario(func(c *client.Client, t *transcript) {
				c.On("q", func(args []any) []any {
					t.push("cli:q", jstr(args))
					return []any{"y"}
				})
			})
		},

		// KNOWN PORT DIVERGENCE (client side): the Go client also auto-acks.
		"server-ack-no-handler": func() []string {
			return serverAckScenario(func(*client.Client, *transcript) {})
		},

		"onany": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.OnAny(func(ev string, args []any) { t.push("srv:any", ev, jstr(args)) })
					s.On("b", func([]any) []any { return []any{"acked"} })
					s.On("z", func([]any) []any { t.push("srv:z"); return nil })
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			defer r.client.Close()
			_ = r.client.Emit("a", 1)
			time.Sleep(60 * time.Millisecond)
			reply := emitAckClient(r.client, "b", nil)
			if reply == nil {
				b.t.push("cli:ack-b", "<timeout>")
			} else {
				b.t.push("cli:ack-b", jstr(reply))
			}
			_ = r.client.Emit("z", map[string]any{"k": []any{1, 2}})
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"binary-server-to-client": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.On("ready", func([]any) []any {
						_ = s.Emit("blob", []byte{0, 1, 254, 255}, "tail",
							map[string]any{"nested": []byte("hi")})
						return nil
					})
				})
			})
			defer b.stop()
			c, err := client.Dial(b.url, client.Options{DialTimeout: dialTimeout})
			if err != nil {
				b.t.push("cli:connect", "false", normErr(err))
				return b.t.snapshot()
			}
			defer c.Close()
			c.On("blob", func(args []any) []any { b.t.push("cli:blob", jstr(args)); return nil })
			_ = c.Emit("ready")
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"binary-client-to-server": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.On("blob", func(args []any) []any { t.push("srv:blob", jstr(args)); return nil })
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			defer r.client.Close()
			_ = r.client.Emit("blob", []byte{0, 1, 254, 255}, []any{[]byte("ab"), 7})
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"namespace-connect": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.Of("/admin").OnConnection(func(s *socketio.Socket) {
					t.push("srv:connection", s.Namespace().Name(), t.sid(s.ID()))
				})
				srv.OnConnection(func(*socketio.Socket) { t.push("srv:root-connection") })
			})
			defer b.stop()
			r := connect(b.url, "/admin", nil)
			time.Sleep(connectSettle)
			b.t.push("cli:connect", boolStr(r.ok))
			if r.ok {
				defer r.client.Close()
			}
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"namespace-middleware-accept": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				ns := srv.Of("/admin")
				ns.Use(func(s *socketio.Socket, next func(error)) { t.push("srv:mw1"); next(nil) })
				ns.Use(func(s *socketio.Socket, next func(error)) { t.push("srv:mw2"); next(nil) })
				ns.OnConnection(func(*socketio.Socket) { t.push("srv:connection") })
			})
			defer b.stop()
			r := connect(b.url, "/admin", nil)
			time.Sleep(connectSettle)
			b.t.push("cli:connect", boolStr(r.ok))
			if r.ok {
				defer r.client.Close()
			}
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"namespace-middleware-reject": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				ns := srv.Of("/admin")
				ns.Use(func(s *socketio.Socket, next func(error)) {
					t.push("srv:mw1")
					next(fmt.Errorf("nope"))
				})
				ns.Use(func(s *socketio.Socket, next func(error)) {
					t.push("srv:mw2-should-not-run")
					next(nil)
				})
				ns.OnConnection(func(*socketio.Socket) { t.push("srv:connection-should-not-run") })
			})
			defer b.stop()
			r := connect(b.url, "/admin", nil)
			if r.ok {
				b.t.push("cli:connect", "true", "")
				r.client.Close()
			} else {
				b.t.push("cli:connect", "false", r.message)
			}
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		// A middleware that never calls next() must leave the connection hanging.
		"namespace-middleware-silent": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				ns := srv.Of("/admin")
				ns.Use(func(s *socketio.Socket, next func(error)) { t.push("srv:mw-no-next") })
				ns.OnConnection(func(*socketio.Socket) { t.push("srv:connection-should-not-run") })
			})
			defer b.stop()
			r := connect(b.url, "/admin", nil)
			if r.ok {
				b.t.push("cli:connect", "true", "")
				r.client.Close()
			} else {
				b.t.push("cli:connect", "false", r.message)
			}
			return b.t.snapshot()
		},

		"namespace-invalid": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(*socketio.Socket) {})
			})
			defer b.stop()
			r := connect(b.url, "/ghost", nil)
			if r.ok {
				b.t.push("cli:connect", "true", "")
				r.client.Close()
			} else {
				b.t.push("cli:connect", "false", r.message)
			}
			return b.t.snapshot()
		},

		"handshake-auth": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.Use(func(s *socketio.Socket, next func(error)) {
					t.push("srv:auth", jstr(s.Auth()))
					next(nil)
				})
				srv.OnConnection(func(*socketio.Socket) { t.push("srv:connection") })
			})
			defer b.stop()
			r := connect(b.url, "/", map[string]any{"token": "abc", "n": 1})
			time.Sleep(connectSettle)
			b.t.push("cli:connect", boolStr(r.ok))
			if r.ok {
				r.client.Close()
			}
			return b.t.snapshot()
		},

		"rooms-self-room": func() []string {
			show := func(s *socketio.Socket) string {
				rooms := s.Rooms()
				out := make([]any, 0, len(rooms))
				for _, r := range rooms {
					if r == s.ID() {
						out = append(out, "<self>")
					} else {
						out = append(out, r)
					}
				}
				sort.Slice(out, func(i, j int) bool { return out[i].(string) < out[j].(string) })
				return jstr(out)
			}
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					t.push("srv:rooms-initial", show(s))
					s.Join("a")
					s.Join("b")
					t.push("srv:rooms-joined", show(s))
					s.Leave("a")
					t.push("srv:rooms-left", show(s))
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if r.ok {
				defer r.client.Close()
			}
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"rooms-socket-to-excludes-sender": func() []string {
			return broadcastScenario(func(srv *socketio.Server, s *socketio.Socket, t *transcript) {
				s.On("shout", func(args []any) []any {
					t.push("srv:shout-from", t.sid(s.ID()))
					s.To("r").Emit("heard", args...)
					return nil
				})
			})
		},

		"rooms-namespace-to-includes-sender": func() []string {
			return broadcastScenario(func(srv *socketio.Server, s *socketio.Socket, t *transcript) {
				s.On("shout", func(args []any) []any {
					t.push("srv:shout-from", t.sid(s.ID()))
					srv.To("r").Emit("heard", args...)
					return nil
				})
			})
		},

		"rooms-except": func() []string {
			return broadcastScenario(func(srv *socketio.Server, s *socketio.Socket, t *transcript) {
				s.On("shout", func(args []any) []any {
					t.push("srv:shout-from", t.sid(s.ID()))
					srv.Except("r").Emit("heard", args...)
					return nil
				})
			})
		},

		"rooms-broadcast-all": func() []string {
			return broadcastScenario(func(srv *socketio.Server, s *socketio.Socket, t *transcript) {
				s.On("shout", func(args []any) []any {
					t.push("srv:shout-from", t.sid(s.ID()))
					srv.Emit("heard", args...)
					return nil
				})
			})
		},

		"rooms-socket-broadcast": func() []string {
			return broadcastScenario(func(srv *socketio.Server, s *socketio.Socket, t *transcript) {
				s.On("shout", func(args []any) []any {
					t.push("srv:shout-from", t.sid(s.ID()))
					s.Broadcast().Emit("heard", args...)
					return nil
				})
			})
		},

		"rooms-fetch-sockets": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) { s.Join("r") })
			})
			defer b.stop()
			var cs []*client.Client
			for i := 0; i < 3; i++ {
				r := connect(b.url, "/", nil)
				if !r.ok {
					b.t.push("cli:connect", "false", r.message)
					return b.t.snapshot()
				}
				cs = append(cs, r.client)
			}
			time.Sleep(stepWait)
			ns := b.srv.Of("/")
			b.t.push("srv:all", fmt.Sprint(len(b.srv.FetchSockets())))
			b.t.push("srv:in-room", fmt.Sprint(len(ns.SocketsInRoom("r"))))
			b.t.push("srv:in-missing", fmt.Sprint(len(ns.SocketsInRoom("nope"))))
			for _, c := range cs {
				c.Close()
			}
			return b.t.snapshot()
		},

		"rooms-sockets-join-leave": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {})
			defer b.stop()
			var cs []*client.Client
			for i := 0; i < 2; i++ {
				r := connect(b.url, "/", nil)
				if !r.ok {
					b.t.push("cli:connect", "false", r.message)
					return b.t.snapshot()
				}
				cs = append(cs, r.client)
			}
			time.Sleep(stepWait)
			ns := b.srv.Of("/")
			b.srv.SocketsJoin("bulk")
			time.Sleep(50 * time.Millisecond)
			b.t.push("srv:bulk", fmt.Sprint(len(ns.SocketsInRoom("bulk"))))
			b.srv.SocketsLeave("bulk")
			time.Sleep(50 * time.Millisecond)
			b.t.push("srv:bulk-after", fmt.Sprint(len(ns.SocketsInRoom("bulk"))))
			for _, c := range cs {
				c.Close()
			}
			return b.t.snapshot()
		},

		"disconnect-client-initiated": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.OnDisconnecting(func(reason string) { t.push("srv:disconnecting", reason) })
					s.OnDisconnect(func(reason string) { t.push("srv:disconnect", reason) })
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			r.client.Close()
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"disconnect-server-namespace": func() []string { return serverDisconnectScenario(false) },
		"disconnect-server-transport": func() []string { return serverDisconnectScenario(true) },

		"disconnect-namespace-then-emit": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.On("go", func([]any) []any {
						s.Disconnect(false)
						if err := s.Emit("after"); err != nil {
							t.push("srv:emit-after-threw")
						} else {
							t.push("srv:emit-after-ok")
						}
						return nil
					})
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			r.client.On("after", func([]any) []any { b.t.push("cli:after"); return nil })
			r.client.On("disconnect", func(args []any) []any {
				b.t.push("cli:disconnect", disconnectReason(args))
				return nil
			})
			_ = r.client.Emit("go")
			time.Sleep(stepWait)
			r.client.Close()
			return b.t.snapshot()
		},

		"reserved-event-server-emit": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					for _, name := range []string{"connect", "disconnect", "connect_error"} {
						if err := s.Emit(name, 1); err != nil {
							t.push("srv:emit-threw", name)
						} else {
							t.push("srv:emit-ok", name)
						}
					}
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			time.Sleep(stepWait)
			r.client.Close()
			return b.t.snapshot()
		},

		"reserved-event-client-emit": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(*socketio.Socket) {})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			for _, name := range []string{"connect", "disconnect", "connect_error"} {
				if err := r.client.Emit(name, 1); err != nil {
					b.t.push("cli:emit-threw", name)
				} else {
					b.t.push("cli:emit-ok", name)
				}
			}
			r.client.Close()
			return b.t.snapshot()
		},

		"handler-off": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.OnConnection(func(s *socketio.Socket) {
					s.On("x", func([]any) []any { t.push("srv:hit"); return nil })
					s.On("kill", func([]any) []any {
						s.Off("x")
						t.push("srv:killed")
						return nil
					})
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if !r.ok {
				b.t.push("cli:connect", "false", r.message)
				return b.t.snapshot()
			}
			defer r.client.Close()
			_ = r.client.Emit("x")
			time.Sleep(100 * time.Millisecond)
			_ = r.client.Emit("kill")
			time.Sleep(100 * time.Millisecond)
			_ = r.client.Emit("x")
			time.Sleep(stepWait)
			return b.t.snapshot()
		},

		"socket-data": func() []string {
			b := startServer(func(srv *socketio.Server, t *transcript) {
				srv.Use(func(s *socketio.Socket, next func(error)) {
					s.Set("who", "mw")
					next(nil)
				})
				srv.OnConnection(func(s *socketio.Socket) {
					t.push("srv:data", jstr(s.Data()))
					s.Set("n", 1)
					t.push("srv:data2", jstr(s.Data()))
				})
			})
			defer b.stop()
			r := connect(b.url, "/", nil)
			if r.ok {
				defer r.client.Close()
			}
			time.Sleep(stepWait)
			return b.t.snapshot()
		},
	}
}

func disconnectReason(args []any) string {
	if len(args) == 0 {
		// The port's client fires "disconnect" with no arguments; upstream
		// always supplies a reason string.
		return "<no-reason>"
	}
	if s, ok := args[0].(string); ok {
		return s
	}
	return jstr(args)
}

func ackScenario(setup func(*socketio.Socket, *transcript), ev string, args []any) []string {
	b := startServer(func(srv *socketio.Server, t *transcript) {
		srv.OnConnection(func(s *socketio.Socket) { setup(s, t) })
	})
	defer b.stop()
	r := connect(b.url, "/", nil)
	if !r.ok {
		b.t.push("cli:connect", "false", r.message)
		return b.t.snapshot()
	}
	defer r.client.Close()
	reply := emitAckClient(r.client, ev, args)
	time.Sleep(50 * time.Millisecond)
	b.t.push(ackLine("cli:ack", reply))
	return b.t.snapshot()
}

func serverAckScenario(clientSetup func(*client.Client, *transcript)) []string {
	done := make(chan struct{}, 1)
	b := startServer(func(srv *socketio.Server, t *transcript) {
		// Gated on a client "ready" event: the port's client cannot register
		// handlers before Dial returns, so a server that emits straight out of
		// the connection handler would race the client's listener.
		srv.OnConnection(func(s *socketio.Socket) {
			s.On("ready", func([]any) []any {
				// EmitAck must not run on the transport read loop: blocking
				// there would stop the very ACK it waits for from being read.
				go func() {
					reply, err := s.EmitAck("q", ackWait, "x")
					if err != nil {
						t.push("srv:ack-timeout")
					} else {
						if reply == nil {
							reply = []any{}
						}
						t.push("srv:ack", jstr(reply))
					}
					select {
					case done <- struct{}{}:
					default:
					}
				}()
				return nil
			})
		})
	})
	defer b.stop()
	r := connect(b.url, "/", nil)
	if !r.ok {
		b.t.push("cli:connect", "false", r.message)
		return b.t.snapshot()
	}
	defer r.client.Close()
	clientSetup(r.client, b.t)
	_ = r.client.Emit("ready")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		b.t.push("srv:never-finished")
	}
	time.Sleep(50 * time.Millisecond)
	return b.t.snapshot()
}

func serverDisconnectScenario(closeTransport bool) []string {
	b := startServer(func(srv *socketio.Server, t *transcript) {
		srv.OnConnection(func(s *socketio.Socket) {
			s.OnDisconnect(func(reason string) { t.push("srv:disconnect", reason) })
			s.On("go", func([]any) []any { s.Disconnect(closeTransport); return nil })
		})
	})
	defer b.stop()
	r := connect(b.url, "/", nil)
	if !r.ok {
		b.t.push("cli:connect", "false", r.message)
		return b.t.snapshot()
	}
	defer r.client.Close()
	r.client.On("disconnect", func(args []any) []any {
		b.t.push("cli:disconnect", disconnectReason(args))
		return nil
	})
	_ = r.client.Emit("go")
	time.Sleep(stepWait)
	return b.t.snapshot()
}

// broadcastScenario connects three clients in order; clients 0 and 1 join room
// "r"; client 0 emits "shout". Receiver lines are sorted before reporting
// because fan-out order across sockets is not specified.
func broadcastScenario(onShout func(*socketio.Server, *socketio.Socket, *transcript)) []string {
	var n int
	var mu sync.Mutex
	b := startServer(func(srv *socketio.Server, t *transcript) {
		srv.OnConnection(func(s *socketio.Socket) {
			mu.Lock()
			idx := n
			n++
			mu.Unlock()
			t.sid(s.ID())
			if idx < 2 {
				s.Join("r")
			}
			onShout(srv, s, t)
		})
	})
	defer b.stop()
	var cs []*client.Client
	for i := 0; i < 3; i++ {
		r := connect(b.url, "/", nil)
		if !r.ok {
			b.t.push("cli:connect", "false", r.message)
			return b.t.snapshot()
		}
		idx := i
		r.client.On("heard", func(args []any) []any {
			b.t.push("cli:heard", fmt.Sprintf("c%d", idx), jstr(args))
			return nil
		})
		cs = append(cs, r.client)
	}
	time.Sleep(stepWait)
	_ = cs[0].Emit("shout", "hey")
	time.Sleep(stepWait)
	for _, c := range cs {
		c.Close()
	}
	lines := b.t.snapshot()
	var head, heard []string
	for _, l := range lines {
		if strings.HasPrefix(l, "cli:heard") {
			heard = append(heard, l)
		} else {
			head = append(head, l)
		}
	}
	sort.Strings(heard)
	return append(head, heard...)
}

// ---------------------------------------------------------------- interop

var live struct {
	srv *socketio.Server
	ts  *httptest.Server
	t   *transcript
}

var interopServers = map[string]func(*socketio.Server, *transcript){
	"basic": func(srv *socketio.Server, t *transcript) {
		srv.OnConnection(func(s *socketio.Socket) {
			t.push("srv:connection", t.sid(s.ID()))
			s.On("echo", func(args []any) []any {
				t.push("srv:echo", jstr(args))
				return []any{"echoed", len(args)}
			})
			s.On("bin", func(args []any) []any {
				t.push("srv:bin", jstr(args))
				return []any{[]byte{1, 2, 3}}
			})
			s.On("ask", func([]any) []any {
				go func() {
					reply, err := s.EmitAck("question", ackWait, "q1")
					if err != nil {
						t.push("srv:answer-timeout")
					} else {
						if reply == nil {
							reply = []any{}
						}
						t.push("srv:answer", jstr(reply))
					}
				}()
				return nil
			})
			s.On("push-me", func([]any) []any {
				_ = s.Emit("pushed", "p", 2, map[string]any{"k": true})
				return nil
			})
			s.OnDisconnect(func(reason string) { t.push("srv:disconnect", reason) })
		})
	},
}

func driveInterop(url string, t *transcript) []string {
	c, err := client.Dial(url, client.Options{DialTimeout: dialTimeout})
	if err != nil {
		t.push("cli:connect", "false")
		t.push("cli:connect-error", normErr(err))
		return t.snapshot()
	}
	t.push("cli:connect", "true")
	c.On("pushed", func(args []any) []any { t.push("cli:pushed", jstr(args)); return nil })
	c.On("question", func(args []any) []any {
		t.push("cli:question", jstr(args))
		return []any{"a1"}
	})

	_ = c.Emit("echo", "s", 1, true, nil, map[string]any{"b": 1})
	time.Sleep(100 * time.Millisecond)

	t.push(ackLine("cli:ack", emitAckClient(c, "echo", []any{"with-ack"})))
	t.push(ackLine("cli:bin-ack", emitAckClient(c, "bin", []any{[]byte{0, 255}})))

	_ = c.Emit("push-me")
	time.Sleep(150 * time.Millisecond)
	_ = c.Emit("ask")
	time.Sleep(ackWait + 150*time.Millisecond)
	_ = c.Close()
	time.Sleep(100 * time.Millisecond)
	return t.snapshot()
}

// ---------------------------------------------------------------- dispatch

func handle(req request) (any, error) {
	switch req.Fn {
	case "wire.encode", "wire.decode", "wire.decodeAttachments",
		"eio.encodePacket", "eio.decodePacket", "eio.encodePayload", "eio.decodePayload":
		spec, err := argObj(req, 0)
		if err != nil {
			return nil, err
		}
		switch req.Fn {
		case "wire.encode":
			return wireEncode(spec)
		case "wire.decode":
			return wireDecode(spec)
		case "wire.decodeAttachments":
			return wireDecodeAttachments(spec)
		case "eio.encodePacket":
			return eioEncodePacket(spec)
		case "eio.decodePacket":
			return eioDecodePacket(spec)
		case "eio.encodePayload":
			return eioEncodePayload(spec)
		case "eio.decodePayload":
			return eioDecodePayload(spec)
		}
	case "meta.versions":
		return map[string]any{"protocol": socketio.ProtocolVersion, "engineio": engineio.Protocol}, nil
	case "serve.close":
		if live.srv == nil {
			return nil, fmt.Errorf("no live server")
		}
		lines := live.t.snapshot()
		stopServer(live.srv, live.ts)
		live.srv, live.ts, live.t = nil, nil, nil
		return map[string]any{"transcript": lines}, nil
	}

	switch {
	case strings.HasPrefix(req.Fn, "scenario."):
		name := strings.TrimPrefix(req.Fn, "scenario.")
		sc, ok := scenarios[name]
		if !ok {
			return nil, fmt.Errorf("unknown scenario: %s", name)
		}
		lines, err := runBounded(name, sc)
		if err != nil {
			return nil, err
		}
		return map[string]any{"transcript": lines}, nil

	case strings.HasPrefix(req.Fn, "serve."):
		name := strings.TrimPrefix(req.Fn, "serve.")
		setup, ok := interopServers[name]
		if !ok {
			return nil, fmt.Errorf("unknown interop server: %s", name)
		}
		if live.srv != nil {
			return nil, fmt.Errorf("server already running")
		}
		b := startServer(setup)
		live.srv, live.ts, live.t = b.srv, b.ts, b.t
		return map[string]any{"url": b.url}, nil

	case strings.HasPrefix(req.Fn, "drive."):
		name := strings.TrimPrefix(req.Fn, "drive.")
		if _, ok := interopServers[name]; !ok {
			return nil, fmt.Errorf("unknown interop script: %s", name)
		}
		url, err := argStr(req, 0)
		if err != nil {
			return nil, err
		}
		t := newTranscript()
		lines, err := runBounded(name, func() []string { return driveInterop(url, t) })
		if err != nil {
			t.push("cli:fatal", err.Error())
			return map[string]any{"transcript": t.snapshot()}, nil
		}
		return map[string]any{"transcript": lines}, nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

// runBounded runs fn with a hard timeout so a stuck scenario fails its own case
// instead of hanging the harness.
func runBounded(name string, fn func() []string) ([]string, error) {
	type result struct {
		lines []string
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- result{err: fmt.Errorf("panic: %v", r)}
			}
		}()
		ch <- result{lines: fn()}
	}()
	select {
	case r := <-ch:
		return r.lines, r.err
	case <-time.After(scenarioTimeout):
		return nil, fmt.Errorf("timeout: %s", name)
	}
}
