// Go runner for the express/slugify nested parity harness.
//
// Speaks the same JSON Lines protocol as ../node/run.js: one request per line
// in, exactly one reply per line out, flushed each time, never exiting on a
// failing case. Logs go to stderr only.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/malcolmston/express/slugify"
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

// jsOptions is the presence-aware decode of a case's options object, mirroring
// slugify's `options.x === undefined` checks.
type jsOptions struct {
	Replacement *string `json:"replacement"`
	Lower       *bool   `json:"lower"`
	Strict      *bool   `json:"strict"`
	Trim        *bool   `json:"trim"`
	Remove      *string `json:"remove"`
}

func argString(raw json.RawMessage) (string, error) {
	var s string
	// JSON null unmarshals into a string without error, so reject it explicitly:
	// upstream's typeof guard rejects null as firmly as it rejects a number.
	if string(raw) == "null" {
		return "", fmt.Errorf("slugify: string argument expected")
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		// Mirror upstream's `typeof string !== 'string'` guard.
		return "", fmt.Errorf("slugify: string argument expected")
	}
	return s, nil
}

// toGoOptions translates the case options into slugify.Options.
//
// Two upstream defaults have to be supplied explicitly because the Go API takes
// a struct whose bool zero values cannot express "unset": trim defaults to true
// and replacement defaults to "-". Both are documented behaviours of the Go
// package's no-Options form, so the runner reproduces them here rather than
// letting a struct zero value masquerade as an upstream default. Cases that
// pass trim/replacement explicitly still measure the real behaviour.
func toGoOptions(raw json.RawMessage) (slugify.Options, error) {
	o := slugify.Options{Trim: true}
	if len(raw) == 0 || string(raw) == "null" {
		return o, nil
	}
	// String shorthand: slugify(str, "_") === slugify(str, {replacement: "_"}).
	var shorthand string
	if err := json.Unmarshal(raw, &shorthand); err == nil {
		o.Separator = shorthand
		return o, nil
	}
	var js jsOptions
	if err := json.Unmarshal(raw, &js); err != nil {
		return o, fmt.Errorf("bad options: %w", err)
	}
	if js.Replacement != nil {
		o.Separator = *js.Replacement
	}
	if js.Lower != nil {
		o.Lower = *js.Lower
	}
	if js.Strict != nil {
		o.Strict = *js.Strict
	}
	if js.Trim != nil {
		o.Trim = *js.Trim
	}
	if js.Remove != nil {
		re, err := regexp.Compile(*js.Remove)
		if err != nil {
			return o, fmt.Errorf("bad remove pattern: %w", err)
		}
		o.Remove = re
	}
	return o, nil
}

func dispatch(fn string, args []json.RawMessage) (any, error) {
	switch fn {
	case "slugify":
		s, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		return slugify.Slugify(s), nil
	case "slugifyOpts":
		s, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if len(args) > 1 {
			raw = args[1]
		}
		o, err := toGoOptions(raw)
		if err != nil {
			return nil, err
		}
		return slugify.Slugify(s, o), nil
	case "charmap":
		s, err := argString(args[0])
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(s))
		for _, ch := range s {
			parts = append(parts, slugify.Slugify(string(ch), slugify.Options{Trim: false}))
		}
		return strings.Join(parts, "|"), nil
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
