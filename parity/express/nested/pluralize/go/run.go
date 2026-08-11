// Go runner for the express/pluralize nested parity harness.
//
// Same JSON Lines protocol as ../node/run.js: one request per line in, one reply
// per line out, flushed each time, never exiting on a failing case.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/express/pluralize"
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

func argString(raw json.RawMessage) (string, error) {
	if string(raw) == "null" {
		return "", fmt.Errorf("string argument expected, got null")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("string argument expected: %w", err)
	}
	return s, nil
}

func dispatch(fn string, args []json.RawMessage) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s: missing argument", fn)
	}
	s, err := argString(args[0])
	if err != nil {
		return nil, err
	}
	switch fn {
	case "plural":
		return pluralize.Plural(s), nil
	case "singular":
		return pluralize.Singular(s), nil
	case "isPlural":
		return pluralize.IsPlural(s), nil
	case "isSingular":
		return pluralize.IsSingular(s), nil
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "bad request line: %v\n", err)
			continue
		}
		resp := response{ID: req.ID}
		func() {
			defer func() {
				if r := recover(); r != nil {
					resp.OK = false
					resp.Error = fmt.Sprintf("panic: %v", r)
				}
			}()
			v, err := dispatch(req.Fn, req.Args)
			if err != nil {
				resp.OK = false
				resp.Error = err.Error()
				return
			}
			resp.OK = true
			resp.Value = v
		}()
		b, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal reply: %v\n", err)
			continue
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}
