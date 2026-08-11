// Command run is the Go side of the express/statuses nested parity harness.
//
// Same JSON-Lines contract as node/run.js (see parity/HARNESS.md): one request
// per line on stdin, one reply per line on stdout, flushed per line, never
// exiting on a failing case.
//
// Representation note, applied identically by the Node runner: Message reports
// an unknown code with its "" sentinel where the upstream callable throws, so ""
// is reported as ok:false rather than as the value "".
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/express/statuses"
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

func intArg(req request, i int) (int, error) {
	if len(req.Args) <= i {
		return 0, fmt.Errorf("missing argument %d", i)
	}
	var n int
	if err := json.Unmarshal(req.Args[i], &n); err != nil {
		return 0, fmt.Errorf("argument %d: %w", i, err)
	}
	return n, nil
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

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "message":
		code, err := intArg(req, 0)
		if err != nil {
			return nil, err
		}
		msg := statuses.Message(code)
		if msg == "" {
			return nil, fmt.Errorf("invalid status code: %d", code)
		}
		return msg, nil
	case "code":
		msg, err := stringArg(req, 0)
		if err != nil {
			return nil, err
		}
		return statuses.Code(msg)
	case "status":
		// The polymorphic upstream callable. Go splits it into two typed
		// functions, so the runner dispatches on the JSON type of the argument.
		if len(req.Args) == 0 {
			return nil, fmt.Errorf("missing argument 0")
		}
		var v any
		if err := json.Unmarshal(req.Args[0], &v); err != nil {
			return nil, err
		}
		switch t := v.(type) {
		case float64:
			msg := statuses.Message(int(t))
			if msg == "" {
				return nil, fmt.Errorf("invalid status code: %v", t)
			}
			return msg, nil
		case string:
			return statuses.Code(t)
		}
		return nil, fmt.Errorf("unsupported status argument %T", v)
	case "codes":
		return statuses.Codes(), nil
	case "isRedirect":
		code, err := intArg(req, 0)
		if err != nil {
			return nil, err
		}
		return statuses.IsRedirect(code), nil
	case "isEmpty":
		code, err := intArg(req, 0)
		if err != nil {
			return nil, err
		}
		return statuses.IsEmpty(code), nil
	case "isRetry":
		code, err := intArg(req, 0)
		if err != nil {
			return nil, err
		}
		return statuses.IsRetry(code), nil
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
