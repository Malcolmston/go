// Go runner for the socket.io/client nested parity harness.
//
// Same JSON-Lines stdio contract as the upstream runner (../node/run.mjs): one
// request per line in, exactly one reply per line out, in order, never exiting
// on a failing case. See ../../../HARNESS.md.
//
// Only the deterministic, socket-free surface of the client is exercised: the
// reconnection schedule (client.Backoff), endpoint addressing
// (client.ResolveEndpoint), option normalisation (Options.WithDefaults), the
// packets an emit builds (client.ConnectPacket / client.EventPacket) and the ack
// id allocator (client.AckIDs). Nothing here opens a connection.
//
// Binary event arguments are written {"$bin": "<lowercase hex>"} in the case
// files and become []byte here; encoded attachment frames come back as
// "bin:<lowercase hex>".
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"time"

	socketio "github.com/malcolmston/socketio"
	"github.com/malcolmston/socketio/client"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

// ---------------------------------------------------------------- backoff

type backoffSpec struct {
	Min        *float64  `json:"min"`
	Max        *float64  `json:"max"`
	Factor     *float64  `json:"factor"`
	Jitter     *float64  `json:"jitter"`
	Rands      []float64 `json:"rands"`
	Count      *int      `json:"count"`
	ResetAfter *int      `json:"resetAfter"`
}

func ms(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }

func opBackoffDurations(raw json.RawMessage) (any, error) {
	var spec backoffSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	opts := client.BackoffOptions{}
	if spec.Min != nil {
		opts.Min = ms(*spec.Min)
	}
	if spec.Max != nil {
		opts.Max = ms(*spec.Max)
	}
	if spec.Factor != nil {
		opts.Factor = *spec.Factor
	}
	if spec.Jitter != nil {
		opts.Jitter = *spec.Jitter
	}
	if len(spec.Rands) > 0 {
		// The same fixed draws the node runner feeds Math.random, cycled.
		i := 0
		rands := spec.Rands
		opts.Rand = func() float64 {
			v := rands[i%len(rands)]
			i++
			return v
		}
	}
	count := 1
	if spec.Count != nil {
		count = *spec.Count
	}
	b := client.NewBackoff(opts)
	durations := make([]any, 0, count)
	for i := 0; i < count; i++ {
		if spec.ResetAfter != nil && i == *spec.ResetAfter {
			b.Reset()
		}
		durations = append(durations, b.Duration().Milliseconds())
	}
	return map[string]any{"durations": durations, "attempts": b.Attempts()}, nil
}

// ---------------------------------------------------------------- endpoint

type endpointSpec struct {
	URI  string  `json:"uri"`
	Path *string `json:"path"`
}

func opEndpointResolve(raw json.RawMessage) (any, error) {
	var spec endpointSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	var opts client.Options
	if spec.Path != nil {
		opts.Path = *spec.Path
	}
	ep, err := client.ResolveEndpoint(spec.URI, opts)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(ep.URL)
	if err != nil {
		return nil, err
	}
	query := map[string]any{}
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			query[k] = vs[0]
		}
	}
	base := ep.URL
	if i := indexByte(base, '?'); i >= 0 {
		base = base[:i]
	}
	return map[string]any{"namespace": ep.Namespace, "base": base, "query": query}, nil
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------- options

type optionsSpec struct {
	Path                 *string  `json:"path"`
	Reconnection         *bool    `json:"reconnection"`
	ReconnectionAttempts *int     `json:"reconnectionAttempts"`
	ReconnectionDelay    *float64 `json:"reconnectionDelay"`
	ReconnectionDelayMax *float64 `json:"reconnectionDelayMax"`
	RandomizationFactor  *float64 `json:"randomizationFactor"`
	Timeout              *float64 `json:"timeout"`
}

func opOptionsDefaults(raw json.RawMessage) (any, error) {
	var spec optionsSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	var o client.Options
	if spec.Path != nil {
		o.Path = *spec.Path
	}
	if spec.Reconnection != nil {
		o.Reconnection = *spec.Reconnection
	}
	if spec.ReconnectionAttempts != nil {
		o.ReconnectionAttempts = *spec.ReconnectionAttempts
	}
	if spec.ReconnectionDelay != nil {
		o.ReconnectionDelay = ms(*spec.ReconnectionDelay)
	}
	if spec.ReconnectionDelayMax != nil {
		o.ReconnectionDelayMax = ms(*spec.ReconnectionDelayMax)
	}
	if spec.RandomizationFactor != nil {
		o.RandomizationFactor = *spec.RandomizationFactor
	}
	if spec.Timeout != nil {
		o.DialTimeout = ms(*spec.Timeout)
	}
	n := o.WithDefaults()
	var attempts any // nil == unlimited, upstream's Infinity
	if n.ReconnectionAttempts != 0 {
		attempts = n.ReconnectionAttempts
	}
	return map[string]any{
		"path":                 n.Path,
		"reconnection":         n.Reconnection,
		"reconnectionAttempts": attempts,
		"reconnectionDelay":    n.ReconnectionDelay.Milliseconds(),
		"reconnectionDelayMax": n.ReconnectionDelayMax.Milliseconds(),
		"randomizationFactor":  n.RandomizationFactor,
		"timeout":              n.DialTimeout.Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------- wire

// reviveArgs turns {"$bin":"<hex>"} markers into []byte, recursively.
func reviveArgs(v any) (any, error) {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			r, err := reviveArgs(e)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		if len(t) == 1 {
			if h, ok := t["$bin"].(string); ok {
				return hex.DecodeString(h)
			}
		}
		out := make(map[string]any, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			r, err := reviveArgs(t[k])
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

// frames renders an encoded packet the way both runners report it: the text
// frame first, then each binary attachment as "bin:<hex>".
func frames(p socketio.Packet) ([]any, error) {
	text, buffers, err := p.EncodeBinary()
	if err != nil {
		return nil, err
	}
	out := []any{text}
	for _, b := range buffers {
		out = append(out, "bin:"+hex.EncodeToString(b))
	}
	return out, nil
}

type wireSpec struct {
	Namespace *string          `json:"namespace"`
	Auth      *json.RawMessage `json:"auth"`
	Event     string           `json:"event"`
	Args      []any            `json:"args"`
	AckID     *uint64          `json:"ackId"`
	Count     int              `json:"count"`
}

func (w wireSpec) nsp() string {
	if w.Namespace == nil {
		return "/"
	}
	return *w.Namespace
}

func opWireConnect(raw json.RawMessage) (any, error) {
	var spec wireSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	var auth any
	if spec.Auth != nil {
		var v any
		if err := json.Unmarshal(*spec.Auth, &v); err != nil {
			return nil, err
		}
		r, err := reviveArgs(v)
		if err != nil {
			return nil, err
		}
		auth = r
	}
	f, err := frames(client.ConnectPacket(spec.nsp(), auth))
	if err != nil {
		return nil, err
	}
	return map[string]any{"frames": f}, nil
}

func opWireEmit(raw json.RawMessage) (any, error) {
	var spec wireSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	args, err := reviveArgs(spec.Args)
	if err != nil {
		return nil, err
	}
	list, _ := args.([]any)
	p, err := client.EventPacket(spec.nsp(), spec.Event, spec.AckID, list)
	if err != nil {
		return nil, err
	}
	f, err := frames(p)
	if err != nil {
		return nil, err
	}
	var ackID any
	if spec.AckID != nil {
		ackID = *spec.AckID
	}
	return map[string]any{"frames": f, "ackId": ackID}, nil
}

func opWireAckIDs(raw json.RawMessage) (any, error) {
	var spec wireSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	var ids client.AckIDs
	out := make([]any, 0, spec.Count)
	for i := 0; i < spec.Count; i++ {
		out = append(out, ids.Next())
	}
	return map[string]any{"ids": out}, nil
}

func opProtocol() (any, error) {
	return map[string]any{"protocol": socketio.ProtocolVersion}, nil
}

func dispatch(req request) (any, error) {
	var a0 json.RawMessage = json.RawMessage("{}")
	if len(req.Args) > 0 {
		a0 = req.Args[0]
	}
	switch req.Fn {
	case "backoff.durations":
		return opBackoffDurations(a0)
	case "endpoint.resolve":
		return opEndpointResolve(a0)
	case "options.defaults":
		return opOptionsDefaults(a0)
	case "wire.connect":
		return opWireConnect(a0)
	case "wire.emit":
		return opWireEmit(a0)
	case "wire.ackIds":
		return opWireAckIDs(a0)
	case "client.protocol":
		return opProtocol()
	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

// ---------------------------------------------------------------- stdio loop

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			write(out, reply{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		write(out, answer(req))
	}
}

// answer runs one case, converting a panic into a failed case so that a bug in
// the port cannot take the whole runner (and every remaining case) down.
func answer(req request) (rep reply) {
	rep = reply{ID: req.ID}
	defer func() {
		if r := recover(); r != nil {
			rep = reply{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	v, err := dispatch(req)
	if err != nil {
		return reply{ID: req.ID, OK: false, Error: err.Error()}
	}
	return reply{ID: req.ID, OK: true, Value: v}
}

func write(out *bufio.Writer, rep reply) {
	b, err := json.Marshal(rep)
	if err != nil {
		b, _ = json.Marshal(reply{ID: rep.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	out.Write(b)
	out.WriteByte('\n')
	// Flush every line: the harness writes one request and blocks on its reply.
	out.Flush()
}
