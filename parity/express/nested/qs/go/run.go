// Command run is the Go side of the express/qs nested parity harness. It speaks
// the same JSON Lines protocol as node/run.js (see parity/HARNESS.md) against
// github.com/malcolmston/express/qs.
//
// Case options arrive as the JSON object upstream qs would take (`{"depth":1}`,
// `{"arrayFormat":"brackets"}`, …) and are mapped onto qs.ParseOptions /
// qs.StringifyOptions here. Two conventions of the Go API show up in that
// mapping:
//
//   - a zero int means "the documented default" and a negative int means "no
//     limit", where upstream spells no-limit as Infinity. Case files write the
//     string "Infinity" so the same JSON feeds both runners.
//   - an option the port does not implement is reported as ok:false rather than
//     silently ignored. Silently ignoring it would score a parity match the port
//     has not earned.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/malcolmston/express/qs"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Not omitempty: a legitimate zero value ("", {}, false) must still be
	// reported, or the harness cannot tell it from an absent answer.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// ------------------------------------------------------------------ arguments

func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string", i)
	}
	return s, nil
}

func argObject(args []json.RawMessage, i int) (map[string]any, error) {
	if i >= len(args) {
		return nil, fmt.Errorf("missing argument %d", i)
	}
	var m map[string]any
	if err := json.Unmarshal(args[i], &m); err != nil {
		return nil, fmt.Errorf("argument %d is not an object", i)
	}
	if m == nil {
		return nil, fmt.Errorf("argument %d is null, not an object", i)
	}
	return m, nil
}

// argOpts reads an optional trailing options object.
func argOpts(args []json.RawMessage, i int) (map[string]any, error) {
	if i >= len(args) {
		return nil, nil
	}
	if string(args[i]) == "null" {
		return nil, nil
	}
	return argObject(args, i)
}

// optInt reads an integer option, translating upstream's Infinity into the
// port's negative "no limit".
func optInt(o map[string]any, key string, out *int) error {
	v, ok := o[key]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case string:
		switch t {
		case "Infinity":
			*out = -1
			return nil
		case "-Infinity":
			*out = -1
			return nil
		}
		return fmt.Errorf("option %s: unexpected string %q", key, t)
	case float64:
		*out = int(t)
		return nil
	}
	return fmt.Errorf("option %s: expected a number", key)
}

func optBool(o map[string]any, key string, out *bool) error {
	v, ok := o[key]
	if !ok {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("option %s: expected a boolean", key)
	}
	*out = b
	return nil
}

func optString(o map[string]any, key string, out *string) error {
	v, ok := o[key]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("option %s: expected a string", key)
	}
	*out = s
	return nil
}

// unsupported reports the options present in o that this port has no field for.
func unsupported(o map[string]any, known ...string) error {
	set := map[string]bool{}
	for _, k := range known {
		set[k] = true
	}
	var bad []string
	for k := range o {
		if !set[k] {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("option not implemented by the port: %s", strings.Join(bad, ", "))
}

// ------------------------------------------------------------------- options

func parseOptions(o map[string]any) (qs.ParseOptions, error) {
	var po qs.ParseOptions
	if o == nil {
		return po, nil
	}
	if err := unsupported(o,
		"depth", "parameterLimit", "arrayLimit", "delimiter", "allowDots",
		"ignoreQueryPrefix", "plainObjects", "allowPrototypes",
		"strictNullHandling", "comma", "duplicates", "allowEmptyArrays",
	); err != nil {
		return po, err
	}
	for _, f := range []func() error{
		func() error { return optInt(o, "depth", &po.Depth) },
		func() error { return optInt(o, "parameterLimit", &po.ParameterLimit) },
		func() error { return optInt(o, "arrayLimit", &po.ArrayLimit) },
		func() error { return optString(o, "delimiter", &po.Delimiter) },
		func() error { return optBool(o, "allowDots", &po.AllowDots) },
		func() error { return optBool(o, "ignoreQueryPrefix", &po.IgnoreQueryPrefix) },
		func() error { return optBool(o, "plainObjects", &po.PlainObjects) },
		func() error { return optBool(o, "allowPrototypes", &po.AllowPrototypes) },
		func() error { return optBool(o, "strictNullHandling", &po.StrictNullHandling) },
		func() error { return optBool(o, "comma", &po.Comma) },
		func() error { return optString(o, "duplicates", &po.Duplicates) },
		func() error { return optBool(o, "allowEmptyArrays", &po.AllowEmptyArrays) },
	} {
		if err := f(); err != nil {
			return po, err
		}
	}
	return po, nil
}

func stringifyOptions(o map[string]any) (qs.StringifyOptions, error) {
	var so qs.StringifyOptions
	if o == nil {
		return so, nil
	}
	// "sort" is the harness's own knob: upstream is given a code-unit key
	// comparator so its output order matches this port, which always sorts.
	// It carries no meaning on the Go side.
	if err := unsupported(o,
		"arrayFormat", "format", "delimiter", "addQueryPrefix", "allowDots",
		"encode", "encodeValuesOnly", "skipNulls", "strictNullHandling",
		"allowEmptyArrays", "sort",
	); err != nil {
		return so, err
	}
	encode := true
	for _, f := range []func() error{
		func() error { return optString(o, "arrayFormat", &so.ArrayFormat) },
		func() error { return optString(o, "format", &so.Format) },
		func() error { return optString(o, "delimiter", &so.Delimiter) },
		func() error { return optBool(o, "addQueryPrefix", &so.AddQueryPrefix) },
		func() error { return optBool(o, "allowDots", &so.AllowDots) },
		func() error { return optBool(o, "encode", &encode) },
		func() error { return optBool(o, "encodeValuesOnly", &so.EncodeValuesOnly) },
		func() error { return optBool(o, "skipNulls", &so.SkipNulls) },
		func() error { return optBool(o, "strictNullHandling", &so.StrictNullHandling) },
		func() error { return optBool(o, "allowEmptyArrays", &so.AllowEmptyArrays) },
	} {
		if err := f(); err != nil {
			return so, err
		}
	}
	so.SkipEncoding = !encode
	return so, nil
}

// ------------------------------------------------------------------ dispatch

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "parse":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOpts(req.Args, 1)
		if err != nil {
			return nil, err
		}
		po, err := parseOptions(o)
		if err != nil {
			return nil, err
		}
		return qs.ParseWith(s, po), nil

	case "stringify":
		m, err := argObject(req.Args, 0)
		if err != nil {
			return nil, err
		}
		o, err := argOpts(req.Args, 1)
		if err != nil {
			return nil, err
		}
		so, err := stringifyOptions(o)
		if err != nil {
			return nil, err
		}
		return qs.StringifyWith(m, so), nil

	case "parseStringify":
		s, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		po, err := optsAt(req.Args, 1, parseOptions)
		if err != nil {
			return nil, err
		}
		so, err := optsAt(req.Args, 2, stringifyOptions)
		if err != nil {
			return nil, err
		}
		return qs.StringifyWith(qs.ParseWith(s, po), so), nil

	case "stringifyParse":
		m, err := argObject(req.Args, 0)
		if err != nil {
			return nil, err
		}
		so, err := optsAt(req.Args, 1, stringifyOptions)
		if err != nil {
			return nil, err
		}
		po, err := optsAt(req.Args, 2, parseOptions)
		if err != nil {
			return nil, err
		}
		return qs.ParseWith(qs.StringifyWith(m, so), po), nil

	case "formats":
		return map[string]any{
			"default": qs.FormatRFC3986,
			"RFC1738": qs.FormatRFC1738,
			"RFC3986": qs.FormatRFC3986,
		}, nil

	case "exports":
		// The three names upstream qs exports, as this port spells the same
		// capabilities. Compared against Object.keys(require('qs')).
		return []any{"formats", "parse", "stringify"}, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func optsAt[T any](args []json.RawMessage, i int, conv func(map[string]any) (T, error)) (T, error) {
	o, err := argOpts(args, i)
	if err != nil {
		var zero T
		return zero, err
	}
	return conv(o)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "malformed request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		resp := response{ID: req.ID}
		if v, err := dispatch(req); err != nil {
			resp.OK = false
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Value = v
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		// Flush after every line or the harness deadlocks waiting for a reply.
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}
}
