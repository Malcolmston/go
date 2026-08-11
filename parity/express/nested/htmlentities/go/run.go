// Command run is the Go-side runner for the express/htmlentities parity harness.
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

	"github.com/malcolmston/express/htmlentities"
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

// options mirrors the shape of the single options object html-entities takes.
// The three encode keys and the two decode keys share one struct because a case
// only ever sets the ones its fn reads.
type options struct {
	Mode    string `json:"mode"`
	Level   string `json:"level"`
	Numeric string `json:"numeric"`
	Scope   string `json:"scope"`
}

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

// argOptions decodes the optional options object. An absent or null argument
// means "defaults", which the Go port spells as the zero EncodeOptions /
// DecodeOptions.
func argOptions(args []json.RawMessage, i int) (options, error) {
	var o options
	if i >= len(args) || string(args[i]) == "null" {
		return o, nil
	}
	if err := json.Unmarshal(args[i], &o); err != nil {
		return o, fmt.Errorf("argument %d is not an options object: %w", i, err)
	}
	return o, nil
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "encode":
		text, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return htmlentities.Encode(text, htmlentities.EncodeOptions{
			Mode: o.Mode, Level: o.Level, Numeric: o.Numeric,
		}), nil

	case "decode":
		text, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return htmlentities.Decode(text, htmlentities.DecodeOptions{
			Level: o.Level, Scope: o.Scope,
		}), nil

	case "decodeEntity":
		entity, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return htmlentities.DecodeEntity(entity, htmlentities.DecodeOptions{Level: o.Level}), nil

	case "roundTrip":
		text, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		encoded := htmlentities.Encode(text, htmlentities.EncodeOptions{
			Mode: o.Mode, Level: o.Level, Numeric: o.Numeric,
		})
		decoded := htmlentities.Decode(encoded)
		return map[string]any{
			"encoded":  encoded,
			"decoded":  decoded,
			"lossless": decoded == text,
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
