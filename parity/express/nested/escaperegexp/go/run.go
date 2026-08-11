// Command run is the Go-side runner for the express/escaperegexp parity harness.
//
// It speaks the same JSON Lines contract as node/run.js: one request object per
// line on stdin ({id, fn, args}), one reply object per line on stdout
// ({id, ok, value} or {id, ok:false, error}). A bad request or a bad argument
// shape is reported as ok:false rather than terminating the process, so a single
// losing case never costs the rest of the run.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/malcolmston/express/escaperegexp"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Value is never omitempty: an empty-string result must still serialize as
	// "value":"" so it compares equal to the upstream reply.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// argString decodes args[i] as a JSON string. A non-string argument is an error,
// matching upstream, which throws TypeError('Expected a string').
func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	// encoding/json quietly accepts a JSON null into a string, leaving it "".
	// Upstream throws for null, so reject it explicitly.
	if string(args[i]) == "null" {
		return "", fmt.Errorf("argument %d is null, not a string", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %w", i, err)
	}
	return s, nil
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "escapeRegExp":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return escaperegexp.EscapeRegExp(s), nil

	case "roundTrip":
		needle, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		haystack, err := argString(req.Args, 1)
		if err != nil {
			return nil, err
		}
		escaped := escaperegexp.EscapeRegExp(needle)
		anchored, err := regexp.Compile("^" + escaped + "$")
		if err != nil {
			return nil, fmt.Errorf("compile anchored: %w", err)
		}
		re, err := regexp.Compile(escaped)
		if err != nil {
			return nil, fmt.Errorf("compile: %w", err)
		}
		return map[string]any{
			"matchesSelf": anchored.MatchString(needle),
			// FindAllString with -1 counts non-overlapping matches, the same
			// count a JavaScript /g match returns.
			"count": len(re.FindAllString(haystack, -1)),
		}, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		resp := response{ID: req.ID}
		if v, err := dispatch(req); err != nil {
			resp.OK, resp.Error = false, err.Error()
		} else {
			resp.OK, resp.Value = true, v
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		// Flush per line or the harness deadlocks waiting for the reply.
		_ = out.Flush()
	}
}
