// Command run is the Go side of the express/keygrip parity harness. It speaks
// the same JSON Lines protocol as node/run.js and never exits on a failing
// case: a returned error or a panic (New panics on an empty key list, as the
// Node constructor throws) is reported as ok:false.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/keygrip"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// spec mirrors the Node constructor keygrip(keys, algorithm, encoding).
type spec struct {
	Keys      []string `json:"keys"`
	Algorithm string   `json:"algorithm"`
	Encoding  string   `json:"encoding"`
}

func parseSpec(raw json.RawMessage) (spec, error) {
	var s spec
	if len(raw) == 0 {
		return s, fmt.Errorf("missing keygrip spec")
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("bad keygrip spec %s: %v", raw, err)
	}
	return s, nil
}

func build(raw json.RawMessage) (*keygrip.Keygrip, error) {
	s, err := parseSpec(raw)
	if err != nil {
		return nil, err
	}
	return keygrip.NewWithOptions(s.Keys, keygrip.Options{
		Algorithm: s.Algorithm,
		Encoding:  s.Encoding,
	})
}

func str(raw json.RawMessage) (string, error) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("expected a string argument, got %s", raw)
	}
	return v, nil
}

func call(fn string, args []json.RawMessage) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if len(args) == 0 {
		return nil, fmt.Errorf("%s needs a keygrip spec", fn)
	}
	switch fn {
	case "new":
		if _, err := build(args[0]); err != nil {
			return nil, err
		}
		return true, nil
	case "sign":
		kg, err := build(args[0])
		if err != nil {
			return nil, err
		}
		data, err := str(args[1])
		if err != nil {
			return nil, err
		}
		return kg.Sign(data), nil
	case "verify":
		kg, err := build(args[0])
		if err != nil {
			return nil, err
		}
		data, err := str(args[1])
		if err != nil {
			return nil, err
		}
		digest, err := str(args[2])
		if err != nil {
			return nil, err
		}
		return kg.Verify(data, digest), nil
	case "index":
		kg, err := build(args[0])
		if err != nil {
			return nil, err
		}
		data, err := str(args[1])
		if err != nil {
			return nil, err
		}
		digest, err := str(args[2])
		if err != nil {
			return nil, err
		}
		return kg.Index(data, digest), nil
	case "signAll":
		s, err := parseSpec(args[0])
		if err != nil {
			return nil, err
		}
		data, err := str(args[1])
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(s.Keys))
		for _, key := range s.Keys {
			kg, err := keygrip.NewWithOptions([]string{key}, keygrip.Options{
				Algorithm: s.Algorithm,
				Encoding:  s.Encoding,
			})
			if err != nil {
				return nil, err
			}
			out = append(out, kg.Sign(data))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	enc := json.NewEncoder(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := call(req.Fn, req.Args)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		_ = enc.Encode(response{ID: req.ID, OK: true, Value: v})
	}
}
