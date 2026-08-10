// Command run is the Go-port side of the jq parity harness.
//
// It speaks the JSON-Lines contract from ../../HARNESS.md:
//
//	in   {"id": "...", "fn": "run", "args": [PROGRAM, INPUT, VARS?]}
//	out  {"id": "...", "ok": true,  "value": [ ...the whole output stream... ]}
//	out  {"id": "...", "ok": false, "error": "..."}
//
// A jq filter produces a *stream* of values, so `value` is the complete stream
// as a JSON array — collapsing a many-valued stream to one value is a real bug
// and must be visible to the comparison.
//
// Nothing here ever exits on a failing case: compile errors, runtime errors and
// panics all become {"ok": false}.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/malcolmston/jq"
)

// fixedEnv is the environment view both runners share so that `env` / `$ENV`
// and time-zone dependent builtins are comparable.
var fixedEnv = map[string]string{
	"TZ":     "UTC",
	"LANG":   "C",
	"LC_ALL": "C",
	"PARITY": "jq",
}

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
}

func main() {
	os.Clearenv()
	for k, v := range fixedEnv {
		_ = os.Setenv(k, v)
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<26)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		resp := response{}
		if err := json.Unmarshal(line, &req); err != nil {
			resp.Error = "bad request: " + err.Error()
		} else {
			resp = handle(req)
		}
		resp.ID = req.ID
		b, err := json.Marshal(resp)
		if err != nil {
			b, _ = json.Marshal(response{ID: req.ID, Error: "marshal reply: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
}

func handle(req request) (resp response) {
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	values, err := run(req)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	if values == nil {
		values = []any{}
	}
	// jq.Marshal knows how to render jq values (including NaN/Infinity, which
	// encoding/json refuses) the way jq itself renders them.
	b, merr := jq.Marshal(values)
	if merr != nil {
		return response{ID: req.ID, OK: false, Error: "marshal result: " + merr.Error()}
	}
	if !json.Valid(b) {
		return response{ID: req.ID, OK: false, Error: "result is not valid JSON: " + string(b)}
	}
	return response{ID: req.ID, OK: true, Value: json.RawMessage(b)}
}

func run(req request) ([]any, error) {
	if req.Fn != "" && req.Fn != "run" {
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
	if len(req.Args) == 0 {
		return nil, fmt.Errorf("case has no program argument")
	}
	var program string
	if err := json.Unmarshal(req.Args[0], &program); err != nil {
		return nil, fmt.Errorf("program argument must be a string: %v", err)
	}

	var input any
	if len(req.Args) > 1 && len(req.Args[1]) > 0 {
		v, err := jq.Unmarshal(req.Args[1])
		if err != nil {
			return nil, fmt.Errorf("input argument: %v", err)
		}
		input = v
	}

	q, err := jq.Compile(program)
	if err != nil {
		return nil, err
	}
	if len(req.Args) > 2 && len(req.Args[2]) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(req.Args[2], &raw); err != nil {
			return nil, fmt.Errorf("variables argument must be an object: %v", err)
		}
		if len(raw) > 0 {
			vars := make(map[string]any, len(raw))
			for k, rv := range raw {
				v, err := jq.Unmarshal(rv)
				if err != nil {
					return nil, fmt.Errorf("variable $%s: %v", k, err)
				}
				vars[k] = v
			}
			q = q.WithVariables(vars)
		}
	}
	return q.Run(input)
}
