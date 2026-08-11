// Command run is the Go side of the express/jwtdecode parity harness. It speaks
// the same JSON Lines protocol as node/run.js and never exits on a failing
// case.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/jwtdecode"
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
	if len(args) == 0 {
		return nil, fmt.Errorf("%s needs a token", fn)
	}
	token, err := str(args[0])
	if err != nil {
		return nil, err
	}
	switch fn {
	case "decode":
		claims, err := jwtdecode.Decode(token)
		if err != nil {
			return nil, err
		}
		// A nil map is JSON null, which is what JSON.parse("null") yields
		// upstream; encoding/json would otherwise render it as null too, so no
		// special-casing is needed.
		return claims, nil
	case "decodeHeader":
		header, err := jwtdecode.DecodeHeader(token)
		if err != nil {
			return nil, err
		}
		return header, nil
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
