// Command run is the Go side of the express/cookiesignature parity harness. It
// speaks the same JSON Lines protocol as node/run.js and never exits on a
// failing case.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/cookiesignature"
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

func str(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string argument, got %s", raw)
	}
	return s, nil
}

func call(fn string, args []json.RawMessage) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	switch fn {
	case "sign":
		if len(args) < 2 {
			return nil, fmt.Errorf("sign needs 2 args")
		}
		value, err := str(args[0])
		if err != nil {
			return nil, err
		}
		secret, err := str(args[1])
		if err != nil {
			return nil, err
		}
		return cookiesignature.Sign(value, secret), nil
	case "unsign":
		if len(args) < 2 {
			return nil, fmt.Errorf("unsign needs 2 args")
		}
		signed, err := str(args[0])
		if err != nil {
			return nil, err
		}
		secret, err := str(args[1])
		if err != nil {
			return nil, err
		}
		// Upstream returns the value or the boolean false; mirror that so the
		// two runners are directly comparable.
		out, ok := cookiesignature.Unsign(signed, secret)
		if !ok {
			return false, nil
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
