// Command run is the Go-side runner for the handlebars parity harness.
//
// It reads one JSON object per line from stdin and writes exactly one JSON
// object per line to stdout, in the same order. A failing case is reported as
// {"ok":false,"error":...}; the process never exits early, and panics inside the
// library are recovered so they become case failures rather than a dead runner.
// Everything non-protocol goes to stderr.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	hb "github.com/malcolmston/handlebars"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Value interface{} `json:"value"`
	Error string      `json:"error,omitempty"`
}

// spec is the render/compile option bundle carried by a case's third argument.
type spec struct {
	NoEscape         bool              `json:"noEscape"`
	Strict           bool              `json:"strict"`
	Compat           bool              `json:"compat"`
	NoData           bool              `json:"noData"`
	KnownHelpers     []string          `json:"knownHelpers"`
	KnownHelpersOnly bool              `json:"knownHelpersOnly"`
	Partials         map[string]string `json:"partials"`
	Helpers          []string          `json:"helpers"`
	Decorators       []string          `json:"decorators"`
}

// ---------------------------------------------------------------------------
// value helpers

func str(v interface{}) string { return hb.Stringify(v) }

// jsTruthy mirrors JavaScript's `if (v)` for the values JSON can carry, so the
// shared test helpers behave identically on both sides.
func jsTruthy(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// Shared custom-helper registry — same names and same intent as node/run.js.

var helpers = map[string]hb.Helper{
	"shout": func(o *hb.Options) interface{} {
		return strings.ToUpper(str(o.Arg(0)))
	},
	"rawHtml": func(o *hb.Options) interface{} {
		return hb.SafeString("<i>" + str(o.Arg(0)) + "</i>")
	},
	"sum": func(o *hb.Options) interface{} {
		a, _ := o.Arg(0).(float64)
		b, _ := o.Arg(1).(float64)
		return a + b
	},
	"join": func(o *hb.Options) interface{} {
		sep := o.HashStr("sep", ",")
		parts := make([]string, 0, len(o.Args))
		for _, a := range o.Args {
			parts = append(parts, str(a))
		}
		return strings.Join(parts, sep)
	},
	"bold": func(o *hb.Options) interface{} {
		return "<b>" + o.Fn(nil) + "</b>"
	},
	"hasIt": func(o *hb.Options) interface{} {
		if jsTruthy(o.Arg(0)) {
			return o.Fn(nil)
		}
		return o.Inverse(nil)
	},
	"pair": func(o *hb.Options) interface{} {
		return o.FnWithBlockParams(nil, o.Arg(0), o.Arg(1))
	},
	"helperMissing": func(o *hb.Options) interface{} {
		return "MISSING:" + o.Name
	},
	"blockHelperMissing": func(o *hb.Options) interface{} {
		return "BHM:" + o.Name + ":" + str(o.Arg(0))
	},
}

var decorators = map[string]hb.Decorator{
	"noop": func(o *hb.DecoratorOptions) {},
}

// ---------------------------------------------------------------------------

func compileOptions(s spec) ([]hb.Option, error) {
	var opts []hb.Option
	if s.NoEscape {
		opts = append(opts, hb.NoEscape())
	}
	if s.Strict {
		opts = append(opts, hb.Strict())
	}
	if s.NoData {
		opts = append(opts, hb.NoData())
	}
	if len(s.KnownHelpers) > 0 {
		opts = append(opts, hb.KnownHelpers(s.KnownHelpers...))
	}
	if s.KnownHelpersOnly {
		opts = append(opts, hb.KnownHelpersOnly())
	}
	if s.Compat {
		// The port has no equivalent of Handlebars' compat option; report the
		// gap instead of silently ignoring it.
		return nil, fmt.Errorf("handlebars: compile option \"compat\" is not implemented")
	}
	return opts, nil
}

func build(source string, s spec) (*hb.Template, error) {
	opts, err := compileOptions(s)
	if err != nil {
		return nil, err
	}
	t, err := hb.Compile(source, opts...)
	if err != nil {
		return nil, err
	}
	// {{log}} must never touch stdout.
	t.SetLogger(log.New(io.Discard, "", 0))
	for _, n := range s.Helpers {
		h, ok := helpers[n]
		if !ok {
			return nil, fmt.Errorf("unknown test helper: %s", n)
		}
		t.RegisterHelper(n, h)
	}
	for _, n := range s.Decorators {
		d, ok := decorators[n]
		if !ok {
			return nil, fmt.Errorf("unknown test decorator: %s", n)
		}
		t.RegisterDecorator(n, d)
	}
	names := make([]string, 0, len(s.Partials))
	for k := range s.Partials {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if err := t.RegisterPartial(k, s.Partials[k]); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func decodeArg(raw json.RawMessage, dst interface{}) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func argAny(args []json.RawMessage, i int) (interface{}, error) {
	if i >= len(args) {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(args[i], &v); err != nil {
		return nil, err
	}
	return v, nil
}

func argSpec(args []json.RawMessage, i int) (spec, error) {
	var s spec
	if i >= len(args) {
		return s, nil
	}
	var probe interface{}
	if err := json.Unmarshal(args[i], &probe); err != nil {
		return s, err
	}
	if probe == nil {
		return s, nil
	}
	err := json.Unmarshal(args[i], &s)
	return s, err
}

func dispatch(req request) (val interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	switch req.Fn {
	case "render":
		var source string
		if err := decodeArg(argAt(req.Args, 0), &source); err != nil {
			return nil, err
		}
		ctx, err := argAny(req.Args, 1)
		if err != nil {
			return nil, err
		}
		s, err := argSpec(req.Args, 2)
		if err != nil {
			return nil, err
		}
		t, err := build(source, s)
		if err != nil {
			return nil, err
		}
		return t.Render(ctx)

	case "compile":
		var source string
		if err := decodeArg(argAt(req.Args, 0), &source); err != nil {
			return nil, err
		}
		s, err := argSpec(req.Args, 1)
		if err != nil {
			return nil, err
		}
		if _, err := build(source, s); err != nil {
			return nil, err
		}
		return true, nil

	case "escapeExpression":
		v, err := argAny(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return hb.EscapeExpression(v), nil

	case "isEmpty":
		v, err := argAny(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return hb.IsEmpty(v), nil

	case "isArray":
		v, err := argAny(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return hb.IsArray(v), nil

	case "extend":
		var dst map[string]interface{}
		if err := decodeArg(argAt(req.Args, 0), &dst); err != nil {
			return nil, err
		}
		out := map[string]interface{}{}
		for k, v := range dst {
			out[k] = v
		}
		var sources []map[string]interface{}
		for i := 1; i < len(req.Args); i++ {
			var m map[string]interface{}
			if err := json.Unmarshal(req.Args[i], &m); err != nil {
				return nil, err
			}
			sources = append(sources, m)
		}
		return hb.Extend(out, sources...), nil

	case "createFrame":
		var data map[string]interface{}
		if err := decodeArg(argAt(req.Args, 0), &data); err != nil {
			return nil, err
		}
		return hb.CreateFrame(data), nil

	case "safeString":
		var s string
		if err := decodeArg(argAt(req.Args, 0), &s); err != nil {
			return nil, err
		}
		return string(hb.SafeString(s)), nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

func argAt(args []json.RawMessage, i int) json.RawMessage {
	if i >= len(args) {
		return nil
	}
	return args[i]
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
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
			out.Flush()
			continue
		}
		v, err := dispatch(req)
		resp := response{ID: req.ID}
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Value = v
		}
		_ = enc.Encode(resp)
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			return
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}
}
