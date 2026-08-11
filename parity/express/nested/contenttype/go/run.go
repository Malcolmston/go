// Command run is the Go side of the express/contenttype nested parity harness.
//
// Same JSON-Lines contract as node/run.js (see parity/HARNESS.md): one request
// per line on stdin, one reply per line on stdout, flushed per line, never
// exiting on a failing case. Where upstream throws a TypeError the Go port
// returns an error, and both are reported as ok:false.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/contenttype"
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

// parsed is the wire shape both runners use for a ContentType.
type parsed struct {
	Type       string            `json:"type"`
	Parameters map[string]string `json:"parameters"`
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

func toWire(ct contenttype.ContentType) parsed {
	params := ct.Parameters
	if params == nil {
		params = map[string]string{}
	}
	return parsed{Type: ct.Type, Parameters: params}
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "parse":
		s, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		ct, err := contenttype.Parse(s)
		if err != nil {
			return nil, err
		}
		return toWire(ct), nil
	case "format":
		if len(req.Args) == 0 {
			return nil, fmt.Errorf("missing argument 0")
		}
		var in parsed
		if err := json.Unmarshal(req.Args[0], &in); err != nil {
			return nil, fmt.Errorf("argument 0: %w", err)
		}
		return contenttype.Format(contenttype.ContentType{Type: in.Type, Parameters: in.Parameters})
	case "roundTrip":
		s, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		ct, err := contenttype.Parse(s)
		if err != nil {
			return nil, err
		}
		return contenttype.Format(ct)
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
