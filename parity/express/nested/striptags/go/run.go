// Command run is the Go-side runner for the express/striptags parity harness.
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
	"strings"

	"github.com/malcolmston/express/striptags"
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

// argString decodes args[i] as a JSON string. Upstream striptags throws
// TypeError for a non-string html argument, so a shape error here is the Go-side
// equivalent of that throw.
func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	if string(args[i]) == "null" {
		return "", fmt.Errorf("argument %d is null, not a string", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %w", i, err)
	}
	return s, nil
}

// argStrings decodes args[i] as a JSON array of strings. A missing argument is
// the empty list, matching upstream's `allowable_tags = allowable_tags || []`.
func argStrings(args []json.RawMessage, i int) ([]string, error) {
	if i >= len(args) || string(args[i]) == "null" {
		return nil, nil
	}
	var v []string
	if err := json.Unmarshal(args[i], &v); err != nil {
		return nil, fmt.Errorf("argument %d is not an array of strings: %w", i, err)
	}
	return v, nil
}

// argOptString decodes an optional string argument, defaulting to "" as upstream
// does with `tag_replacement = tag_replacement || ''`.
func argOptString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) || string(args[i]) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %w", i, err)
	}
	return s, nil
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "strip":
		html, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return striptags.StripTags(html), nil

	case "stripAllowedArray":
		html, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		allowed, err := argStrings(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return striptags.StripTagsWith(html, allowed, ""), nil

	case "stripAllowedString":
		html, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		allowed, err := argString(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return striptags.StripTags(html, allowed), nil

	case "stripWith":
		html, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		allowed, err := allowedArg(req.Args, 1)
		if err != nil {
			return nil, err
		}
		repl, err := argOptString(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return striptags.StripTagsWith(html, allowed, repl), nil

	case "stripStreaming":
		var chunks []string
		if len(req.Args) == 0 {
			return nil, fmt.Errorf("missing argument 0")
		}
		if err := json.Unmarshal(req.Args[0], &chunks); err != nil {
			return nil, fmt.Errorf("argument 0 is not an array of strings: %w", err)
		}
		allowed, err := allowedArg(req.Args, 1)
		if err != nil {
			return nil, err
		}
		repl, err := argOptString(req.Args, 2)
		if err != nil {
			return nil, err
		}
		stream := striptags.NewStreamer(allowed, repl)
		var b strings.Builder
		for _, c := range chunks {
			b.WriteString(stream.Write(c))
		}
		return b.String(), nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

// allowedArg accepts the allowable-tags argument in either upstream form: a JSON
// array of names, or a single JSON string listing bracketed names. The Go port
// takes a []string in both cases, treating a lone string as one entry.
func allowedArg(args []json.RawMessage, i int) ([]string, error) {
	if i >= len(args) || string(args[i]) == "null" {
		return nil, nil
	}
	raw := strings.TrimSpace(string(args[i]))
	if strings.HasPrefix(raw, "[") {
		return argStrings(args, i)
	}
	s, err := argString(args, i)
	if err != nil {
		return nil, err
	}
	return []string{s}, nil
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
