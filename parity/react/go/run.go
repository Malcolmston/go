// Command run is the Go side of the react parity harness: the same JSON-Lines
// stdio contract as ../node/run.mjs (see parity/HARNESS.md), answering each case
// with github.com/malcolmston/react's server renderer instead of
// react-dom/server.
//
// The tree encoding it accepts is defined in ../COVERAGE.md and mirrored by
// ../node/tree.mjs. Nothing here hand-writes an expectation; the process only
// renders and reports, and a render that fails is reported as ok:false rather
// than killing the runner — a dead runner would score every remaining case as a
// loss.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	react "github.com/malcolmston/react"
)

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, reply{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		emit(out, answer(req))
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
	_ = out.Flush()
}

// emit writes exactly one reply line and flushes, because the harness writes one
// case and then blocks on the answer.
func emit(w *bufio.Writer, r reply) {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(reply{ID: r.ID, OK: false, Error: "marshal reply: " + err.Error()})
	}
	w.Write(b)
	w.WriteByte('\n')
	_ = w.Flush()
}

// answer renders one case. Panics are recovered so that a port bug that panics
// scores as a failed case (comparable with an upstream throw) instead of taking
// the runner down.
func answer(req request) (rep reply) {
	rep = reply{ID: req.ID}
	defer func() {
		if r := recover(); r != nil {
			rep = reply{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	if len(req.Args) != 1 {
		return reply{ID: req.ID, OK: false, Error: "expected exactly one argument (the node)"}
	}
	node, err := buildNode(req.Args[0])
	if err != nil {
		return reply{ID: req.ID, OK: false, Error: err.Error()}
	}

	var html string
	switch req.Fn {
	case "renderToStaticMarkup":
		html, err = react.RenderToStaticMarkup(node)
	case "renderToString":
		html, err = react.RenderToString(node)
	default:
		return reply{ID: req.ID, OK: false, Error: "unknown fn " + req.Fn}
	}
	if err != nil {
		return reply{ID: req.ID, OK: false, Error: err.Error()}
	}
	return reply{ID: req.ID, OK: true, Value: html}
}

// ------------------------------------------------------------ tree decoding

// passThrough is the "#component" pseudo-type: a function component that renders
// exactly its children, so a case can assert that components are transparent to
// the serializer.
var passThrough react.Component = func(props react.Props) react.Node {
	return props.Children()
}

func buildNode(spec any) (react.Node, error) {
	switch v := spec.(type) {
	case nil:
		return nil, nil
	case bool:
		return v, nil
	case string:
		return v, nil
	case float64:
		return v, nil
	case []any:
		out := make([]react.Node, 0, len(v))
		for _, child := range v {
			n, err := buildNode(child)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, nil
	case map[string]any:
		return buildElement(v)
	}
	return nil, fmt.Errorf("unrepresentable node: %T", spec)
}

func buildElement(spec map[string]any) (react.Node, error) {
	if u, ok := spec["$undefined"].(bool); ok && u {
		return nil, nil
	}
	rawType, ok := spec["type"].(string)
	if !ok {
		return nil, fmt.Errorf(`element spec needs a string "type"`)
	}

	var typ any
	switch rawType {
	case "#fragment":
		typ = react.Fragment
	case "#component":
		typ = passThrough
	default:
		typ = rawType
	}

	props, err := buildProps(spec["props"])
	if err != nil {
		return nil, err
	}

	var kids []react.Node
	if raw, present := spec["children"]; present {
		list, isList := raw.([]any)
		if !isList {
			return nil, fmt.Errorf(`"children" must be an array`)
		}
		for _, child := range list {
			n, err := buildNode(child)
			if err != nil {
				return nil, err
			}
			kids = append(kids, n)
		}
	}
	return react.CreateElement(typ, props, kids...), nil
}

func buildProps(raw any) (react.Props, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf(`"props" must be an object`)
	}
	props := make(react.Props, len(m))
	// Sorted purely so that an error message is deterministic; the renderer sorts
	// its own output anyway.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, err := buildPropValue(m[k])
		if err != nil {
			return nil, fmt.Errorf("prop %q: %w", k, err)
		}
		props[k] = v
	}
	return props, nil
}

// buildPropValue decodes one prop value. The two wrappers mirror tree.mjs:
// {"$undefined":true} is JS undefined (nil here) and {"$html":"…"} is the
// dangerouslySetInnerHTML payload; any other object is a nested map, which in
// practice means a style map.
func buildPropValue(v any) (any, error) {
	switch t := v.(type) {
	case nil, bool, string, float64:
		return t, nil
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			ev, err := buildPropValue(e)
			if err != nil {
				return nil, err
			}
			out = append(out, ev)
		}
		return out, nil
	case map[string]any:
		if u, ok := t["$undefined"].(bool); ok && u {
			return nil, nil
		}
		if html, ok := t["$html"].(string); ok {
			return react.DangerousHTML{HTML: html}, nil
		}
		out := make(map[string]any, len(t))
		for k, e := range t {
			ev, err := buildPropValue(e)
			if err != nil {
				return nil, err
			}
			out[k] = ev
		}
		return out, nil
	}
	return nil, fmt.Errorf("unrepresentable value: %T", v)
}
