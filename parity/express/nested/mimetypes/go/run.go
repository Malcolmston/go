// Command run is the Go side of the express/mimetypes nested parity harness.
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

	"github.com/malcolmston/express/mimetypes"
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

// orNil folds the Go (value, ok) pair into the shared JSON shape.
func orNil(v string, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

func dispatch(req request) (any, error) {
	s, err := stringArg(req, 0)
	if err != nil {
		return nil, err
	}
	switch req.Fn {
	case "lookup":
		return orNil(mimetypes.Lookup(s)), nil
	case "charset":
		return orNil(mimetypes.Charset(s)), nil
	case "contentType":
		return orNil(mimetypes.ContentType(s)), nil
	case "extension":
		return orNil(mimetypes.Extension(s)), nil
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
