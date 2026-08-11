// Go runner for the socket.io/engineio nested parity harness.
//
// Same JSON-Lines stdio contract as the upstream runner (../node/run.mjs): one
// request per line in, exactly one reply per line out, in order, never exiting
// on a failing case. See ../../../HARNESS.md.
//
// Packets cross the wire to the harness as
// {"type": <engine.io type name>, "data": <string>, "binary": <hex|null>};
// names (not digits) are used so that engineio.PacketType's constants and its
// String method are exercised in both directions.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/socketio/engineio"
)

// ---------------------------------------------------------------- marshalling

var typeByName = map[string]engineio.PacketType{
	"open":    engineio.Open,
	"close":   engineio.Close,
	"ping":    engineio.Ping,
	"pong":    engineio.Pong,
	"message": engineio.Message,
	"upgrade": engineio.Upgrade,
	"noop":    engineio.Noop,
}

type packetSpec struct {
	Type   string  `json:"type"`
	Data   *string `json:"data"`
	Binary *string `json:"binary"`
}

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

func specToPacket(spec packetSpec) (engineio.Packet, error) {
	t, ok := typeByName[spec.Type]
	if !ok {
		return engineio.Packet{}, fmt.Errorf("unknown engine.io packet type: %s", spec.Type)
	}
	// Build via the package's own constructors where one exists, so that they
	// are covered rather than bypassed.
	switch {
	case spec.Binary != nil:
		raw, err := hex.DecodeString(*spec.Binary)
		if err != nil {
			return engineio.Packet{}, err
		}
		if t == engineio.Message {
			return engineio.NewBinaryMessage(raw), nil
		}
		return engineio.Packet{Type: t, Binary: raw}, nil
	case spec.Data != nil:
		switch t {
		case engineio.Message:
			return engineio.NewMessage(*spec.Data), nil
		case engineio.Open:
			return engineio.NewOpen(*spec.Data), nil
		case engineio.Ping:
			return engineio.NewPing(*spec.Data), nil
		case engineio.Pong:
			return engineio.NewPong(*spec.Data), nil
		}
		return engineio.Packet{Type: t, Data: *spec.Data}, nil
	}
	switch t {
	case engineio.Close:
		return engineio.NewClose(), nil
	case engineio.Upgrade:
		return engineio.NewUpgrade(), nil
	case engineio.Noop:
		return engineio.NewNoop(), nil
	case engineio.Ping:
		return engineio.NewPing(""), nil
	case engineio.Pong:
		return engineio.NewPong(""), nil
	}
	return engineio.Packet{Type: t}, nil
}

func normPacket(p engineio.Packet) map[string]any {
	m := map[string]any{"type": p.Type.String(), "data": p.Data, "binary": nil}
	if p.IsBinary() {
		m["binary"] = hex.EncodeToString(p.Binary)
		m["data"] = ""
	}
	return m
}

// ---------------------------------------------------------------- operations

// argSpec decodes the single object argument every operation here takes.
func argSpec(args []json.RawMessage) (map[string]json.RawMessage, error) {
	if len(args) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args[0], &m); err != nil {
		return nil, err
	}
	return m, nil
}

func onePacketSpec(m map[string]json.RawMessage) (packetSpec, error) {
	var spec packetSpec
	b, err := json.Marshal(m)
	if err != nil {
		return spec, err
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		return spec, err
	}
	return spec, nil
}

func opEncodePacket(m map[string]json.RawMessage) (any, error) {
	spec, err := onePacketSpec(m)
	if err != nil {
		return nil, err
	}
	supportsBinary := false
	if raw, ok := m["supportsBinary"]; ok {
		_ = json.Unmarshal(raw, &supportsBinary)
	}
	p, err := specToPacket(spec)
	if err != nil {
		return nil, err
	}
	if supportsBinary && p.IsBinary() {
		// The websocket transport hands Packet.Binary to the frame writer
		// verbatim; there is no encoder step for a native binary frame.
		return map[string]any{"kind": "binary", "encoded": hex.EncodeToString(p.Binary)}, nil
	}
	return map[string]any{"kind": "string", "encoded": p.Encode()}, nil
}

func opDecodePacket(m map[string]json.RawMessage) (any, error) {
	var encoded string
	if err := json.Unmarshal(m["encoded"], &encoded); err != nil {
		return nil, err
	}
	p, err := engineio.Decode(encoded)
	if err != nil {
		return nil, err
	}
	return map[string]any{"packet": normPacket(p)}, nil
}

func opDecodeBinaryFrame(m map[string]json.RawMessage) (any, error) {
	var h string
	if err := json.Unmarshal(m["binary"], &h); err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	return map[string]any{"packet": normPacket(engineio.NewBinaryMessage(raw))}, nil
}

func packetList(raw json.RawMessage) ([]engineio.Packet, error) {
	var specs []packetSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		return nil, err
	}
	out := make([]engineio.Packet, 0, len(specs))
	for _, s := range specs {
		p, err := specToPacket(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func opEncodePayload(m map[string]json.RawMessage) (any, error) {
	pkts, err := packetList(m["packets"])
	if err != nil {
		return nil, err
	}
	return map[string]any{"encoded": engineio.EncodePayload(pkts)}, nil
}

func opDecodePayload(m map[string]json.RawMessage) (any, error) {
	var encoded string
	if err := json.Unmarshal(m["encoded"], &encoded); err != nil {
		return nil, err
	}
	pkts, err := engineio.DecodePayload(encoded)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(pkts))
	for _, p := range pkts {
		out = append(out, normPacket(p))
	}
	return map[string]any{"packets": out}, nil
}

func opRoundTrip(m map[string]json.RawMessage) (any, error) {
	pkts, err := packetList(m["packets"])
	if err != nil {
		return nil, err
	}
	encoded := engineio.EncodePayload(pkts)
	back, err := engineio.DecodePayload(encoded)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(back))
	for _, p := range back {
		out = append(out, normPacket(p))
	}
	return map[string]any{"encoded": encoded, "packets": out}, nil
}

func opProtocol() (any, error) {
	return map[string]any{
		"parser":    engineio.Protocol,
		"engine":    engineio.Protocol,
		"separator": hex.EncodeToString([]byte("\x1e")),
	}, nil
}

func dispatch(req request) (any, error) {
	m, err := argSpec(req.Args)
	if err != nil {
		return nil, err
	}
	switch req.Fn {
	case "eio.encodePacket":
		return opEncodePacket(m)
	case "eio.decodePacket":
		return opDecodePacket(m)
	case "eio.decodeBinaryFrame":
		return opDecodeBinaryFrame(m)
	case "eio.encodePayload":
		return opEncodePayload(m)
	case "eio.decodePayload":
		return opDecodePayload(m)
	case "eio.roundTrip":
		return opRoundTrip(m)
	case "eio.protocol":
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
