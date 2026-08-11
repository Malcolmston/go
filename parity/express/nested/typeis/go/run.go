// Command run is the Go side of the express/typeis nested parity harness.
//
// Same JSON-Lines contract as node/run.js (see parity/HARNESS.md): one request
// per line on stdin, one reply per line on stdout, flushed per line, never
// exiting on a failing case.
//
// Representation note, applied identically by the Node runner: the Go API
// returns (value, ok) where upstream returns value-or-false; a false ok is
// emitted as JSON null.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/typeis"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func stringArg(req request, i int) (string, error) {
	if len(req.Args) <= i {
		return "", fmt.Errorf("missing argument %d", i)
	}
	var s string
	if err := json.Unmarshal(req.Args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d: %w", i, err)
	}
	return s, nil
}

func listArg(req request, i int) ([]string, error) {
	if len(req.Args) <= i || string(req.Args[i]) == "null" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(req.Args[i], &out); err != nil {
		return nil, fmt.Errorf("argument %d: %w", i, err)
	}
	return out, nil
}

func orNil(v string, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "is":
		value, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		types, err := listArg(req, 1)
		if err != nil {
			return nil, err
		}
		return orNil(typeis.Is(value, types...)), nil
	case "normalizeType":
		value, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		v := typeis.NormalizeType(value)
		return orNil(v, v != ""), nil
	case "normalize":
		t, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		return orNil(typeis.Normalize(t)), nil
	case "match":
		expected, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		actual, err := stringArg(req, 1)
		if err != nil {
			return nil, err
		}
		return typeis.Match(expected, actual), nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request line: " + err.Error()})
			_ = out.Flush()
			continue
		}
		_ = enc.Encode(answer(req))
		// Flush per line or the harness deadlocks waiting for the reply.
		_ = out.Flush()
	}
}

func answer(req request) (resp response) {
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	v, err := dispatch(req)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: v}
}
