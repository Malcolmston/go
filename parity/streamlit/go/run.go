// Command run is the Go half of the streamlit parity harness.
//
// It reads one JSON request per line on stdin and writes exactly one JSON reply
// per line on stdout, never exiting on a failing case:
//
//	{"id": "...", "fn": "<app name>", "args": [[<step>, ...], {"profile": "core"}]}
//	{"id": "...", "ok": true,  "value": {"snapshots": [<snapshot>, ...]}}
//	{"id": "...", "ok": false, "error": "..."}
//
// The port's element tree is only reachable through its HTTP surface, so each
// case stands up [st.Handler] on an httptest server bound to a random loopback
// port, drives it with POST /api/run, and shuts it down again. Nothing here
// blocks on a long-lived server.
//
// The request and response bodies of POST /api/run are unexported in the
// library (st.runRequest / st.runResponse), so they are re-declared below —
// recorded in COVERAGE.md as a usability finding: an in-process driver cannot
// reuse the port's own wire types.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/malcolmston/streamlit/st"
)

// ------------------------------------------------------------------- protocol

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// step is one interaction in a case's script.
type step struct {
	Op    string `json:"op"`
	Key   string `json:"key,omitempty"`
	Type  string `json:"type,omitempty"`
	Index int    `json:"index,omitempty"`
	Value any    `json:"value,omitempty"`
	Form  string `json:"form,omitempty"`
}

// ---- re-declared copies of the library's unexported /api/run wire types ----

type wireEvent struct {
	Key    string         `json:"key"`
	Value  any            `json:"value"`
	Button bool           `json:"button"`
	Form   string         `json:"form,omitempty"`
	Values map[string]any `json:"values,omitempty"`
}

type wireRequest struct {
	SessionID string     `json:"sessionId"`
	Event     *wireEvent `json:"event"`
}

// element mirrors st.Element as it arrives over the wire.
type element struct {
	Type     string         `json:"type"`
	Key      string         `json:"key"`
	Props    map[string]any `json:"props"`
	Children []*element     `json:"children"`
}

type wireResponse struct {
	SessionID string   `json:"sessionId"`
	Tree      *element `json:"tree"`
}

// ------------------------------------------------------------- canonical tree

// canonType maps the port's element type names onto the harness's shared
// vocabulary, which is named after Streamlit's own element types.
var canonType = map[string]string{
	"title": "title", "header": "header", "subheader": "subheader",
	"text": "text", "caption": "caption", "markdown": "markdown",
	"divider": "divider", "code": "code", "json": "json", "metric": "metric",
	"latex": "latex", "chart": "chart", "alert": "alert",
	"columns": "columns", "column": "column", "tabs": "tabs", "tab": "tab",
	"expander": "expander", "container": "container", "form": "form",
	"button": "button", "checkbox": "checkbox", "toggle": "toggle",
	"radio": "radio", "selectbox": "selectbox", "multiselect": "multiselect",
	"slider": "slider", "text_input": "text_input", "text_area": "text_area",
	"number": "number_input", "date_input": "date_input",
	"time_input": "time_input", "file_uploader": "file_uploader",
	"form_submit": "form_submit", "color_picker": "color_picker",
	"select_slider": "select_slider",
}

// widgetTypes lists the canonical types that carry a widget identity.
var widgetTypes = map[string]bool{
	"button": true, "checkbox": true, "toggle": true, "radio": true,
	"selectbox": true, "multiselect": true, "slider": true,
	"text_input": true, "text_area": true, "number_input": true,
	"date_input": true, "time_input": true, "file_uploader": true,
	"form_submit": true, "color_picker": true, "select_slider": true,
}

// extraProps mirrors python/run.py:EXTRA_PROPS. Every case compares the core
// properties both implementations model; a case opts in to these with
// {"extra": [...]} so a property the port does not model at all gets exactly
// one case of its own instead of failing every case that contains a widget.
var extraProps = []string{"format", "help", "disabled", "placeholder", "maxChars"}

type canon struct {
	Type     string         `json:"type"`
	Props    map[string]any `json:"props"`
	Children []canon        `json:"children,omitempty"`
}

type snapshot struct {
	Main    []canon        `json:"main"`
	Sidebar []canon        `json:"sidebar"`
	State   map[string]any `json:"state"`
}

// positional counts unkeyed widgets per canonical type so their identity can be
// reported as "#<n>" — the port's auto keys ("auto-slider-3") and Streamlit's
// content-hash ids are both unstable and neither is comparable verbatim.
type positional map[string]int

func (p positional) next(t string) string {
	n := p[t]
	p[t] = n + 1
	return "#" + strconv.Itoa(n)
}

func prop(el *element, name string) any {
	if el.Props == nil {
		return nil
	}
	return el.Props[name]
}

func propStr(el *element, name string) string {
	if s, ok := prop(el, name).(string); ok {
		return s
	}
	return ""
}

func propBool(el *element, name string) bool {
	b, _ := prop(el, name).(bool)
	return b
}

// canonJSON re-emits a JSON document with sorted keys so both sides agree on
// layout, matching python/run.py:canon_json.
func canonJSON(text string) string {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return text
	}
	out, err := json.Marshal(v)
	if err != nil {
		return text
	}
	return string(out)
}

func canonElement(el *element, extra map[string]bool, pos positional) canon {
	t, known := canonType[el.Type]
	if !known {
		return canon{Type: el.Type, Props: map[string]any{"unmapped": true},
			Children: canonChildren(el, extra, pos)}
	}

	switch t {
	case "title", "header", "subheader", "text", "caption", "markdown":
		return canon{Type: t, Props: map[string]any{"text": propStr(el, "text")}}
	case "latex":
		return canon{Type: t, Props: map[string]any{"text": propStr(el, "expr")}}
	case "alert":
		return canon{Type: t, Props: map[string]any{
			"kind": propStr(el, "kind"), "text": propStr(el, "text")}}
	case "divider":
		return canon{Type: t, Props: map[string]any{}}
	case "code":
		return canon{Type: t, Props: map[string]any{
			"text": propStr(el, "code"), "language": propStr(el, "lang")}}
	case "json":
		return canon{Type: t, Props: map[string]any{"body": canonJSON(propStr(el, "json"))}}
	case "metric":
		return canon{Type: t, Props: map[string]any{
			"label": propStr(el, "label"), "value": propStr(el, "value"),
			"delta": propStr(el, "delta"), "direction": propStr(el, "direction"),
			"color": propStr(el, "color")}}
	case "chart":
		// The port renders charts to inline SVG on the server, so the element
		// carries no mark kind and no data — see COVERAGE.md.
		return canon{Type: t, Props: map[string]any{
			"kind": nil, "encoding": "svg", "series": nil}}
	case "columns":
		return canon{Type: t, Props: map[string]any{"n": prop(el, "n")},
			Children: canonChildren(el, extra, pos)}
	case "column":
		return canon{Type: t, Props: map[string]any{},
			Children: canonChildren(el, extra, pos)}
	case "tabs":
		return canon{Type: t, Props: map[string]any{"labels": prop(el, "labels")},
			Children: canonChildren(el, extra, pos)}
	case "tab":
		return canon{Type: t, Props: map[string]any{"label": propStr(el, "label")},
			Children: canonChildren(el, extra, pos)}
	case "expander":
		return canon{Type: t, Props: map[string]any{
			"label": propStr(el, "label"), "expanded": propBool(el, "expanded")},
			Children: canonChildren(el, extra, pos)}
	case "container":
		return canon{Type: t, Props: map[string]any{},
			Children: canonChildren(el, extra, pos)}
	case "form":
		return canon{Type: t, Props: map[string]any{"key": propStr(el, "key")},
			Children: canonChildren(el, extra, pos)}
	}

	if widgetTypes[t] {
		return canonWidget(el, t, extra, pos)
	}
	return canon{Type: t, Props: map[string]any{}, Children: canonChildren(el, extra, pos)}
}

func canonWidget(el *element, t string, extra map[string]bool, pos positional) canon {
	key := el.Key
	if key == "" || strings.HasPrefix(key, "auto-") {
		key = pos.next(t)
	}
	props := map[string]any{"key": key, "label": propStr(el, "label")}

	switch t {
	case "file_uploader":
		files := prop(el, "files")
		if files == nil {
			files = []any{}
		}
		props["files"] = files
		props["value"] = files
	case "button", "form_submit":
		// The port's button elements carry no clicked/value property at all;
		// the transient value is only visible to app code. Compared in the full
		// Opt-in, so the many cases that merely happen to contain a button
		// still score their own subject.
		if extra["value"] {
			props["value"] = nil
		}
	default:
		props["value"] = prop(el, "value")
	}

	switch t {
	case "radio", "selectbox", "multiselect", "select_slider":
		opts := prop(el, "options")
		if opts == nil {
			opts = []any{}
		}
		props["options"] = opts
	}
	switch t {
	case "slider", "select_slider":
		props["min"] = prop(el, "min")
		props["max"] = prop(el, "max")
		props["step"] = prop(el, "step")
	case "number_input":
		props["min"] = prop(el, "min")
		props["max"] = prop(el, "max")
		// The port models no step for a number input; opt-in.
		if extra["step"] {
			props["step"] = prop(el, "step")
		}
	}
	switch t {
	case "radio", "selectbox":
		props["index"] = indexOf(prop(el, "options"), prop(el, "value"))
	}

	// The port models none of these except maxChars; reporting the absence
	// (null) rather than inventing a default is the finding.
	for _, name := range extraProps {
		if extra[name] {
			props[name] = prop(el, name)
		}
	}
	if t == "form_submit" {
		// Neither implementation lets the app name a submit button, so its
		// identity is positional on both sides.
		props["key"] = pos.next("form_submit")
	}
	return canon{Type: t, Props: props}
}

func indexOf(options, value any) int {
	opts, ok := options.([]any)
	if !ok {
		return -1
	}
	for i, o := range opts {
		if fmt.Sprint(o) == fmt.Sprint(value) {
			return i
		}
	}
	return -1
}

// canonTable handles the port's single "table" element type, which carries a
// sortable flag where Streamlit has two distinct element types.
func canonTable(el *element) canon {
	t := "table"
	if propBool(el, "sortable") {
		t = "dataframe"
	}
	header := prop(el, "header")
	if header == nil {
		header = []any{}
	}
	rows := prop(el, "rows")
	if rows == nil {
		rows = []any{}
	}
	return canon{Type: t, Props: map[string]any{"header": header, "rows": rows}}
}

func canonChildren(el *element, extra map[string]bool, pos positional) []canon {
	out := []canon{}
	for _, child := range el.Children {
		if child == nil {
			continue
		}
		if child.Type == "table" {
			out = append(out, canonTable(child))
			continue
		}
		out = append(out, canonElement(child, extra, pos))
	}
	return out
}

// canonTree splits the port's root element into the main body and the sidebar.
func canonTree(root *element, extra map[string]bool) ([]canon, []canon) {
	pos := positional{}
	var main, side []canon
	for _, child := range root.Children {
		switch child.Type {
		case "main":
			main = canonChildren(child, extra, pos)
		case "sidebar":
			side = canonChildren(child, extra, pos)
		}
	}
	if main == nil {
		main = []canon{}
	}
	if side == nil {
		side = []canon{}
	}
	return main, side
}

// --------------------------------------------------------------- driving a case

// driver owns one httptest server and one session for the duration of a case.
type driver struct {
	srv       *httptest.Server
	sessionID string
	tree      *element
}

func newDriver(app func(*st.Session)) *driver {
	return &driver{srv: httptest.NewServer(st.Handler(app))}
}

func (d *driver) close() { d.srv.Close() }

func (d *driver) post(ev *wireEvent) error {
	body, err := json.Marshal(wireRequest{SessionID: d.sessionID, Event: ev})
	if err != nil {
		return err
	}
	resp, err := http.Post(d.srv.URL+"/api/run", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /api/run: %s", resp.Status)
	}
	var out wireResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Tree == nil {
		return fmt.Errorf("POST /api/run returned no tree")
	}
	d.sessionID, d.tree = out.SessionID, out.Tree
	return nil
}

// findWidget resolves the widget key a step addresses: an explicit key is used
// verbatim, otherwise the index-th widget of the given canonical type in
// document order.
func (d *driver) findWidget(s step) (string, error) {
	if s.Key != "" {
		return s.Key, nil
	}
	var keys []string
	var walk func(el *element)
	walk = func(el *element) {
		for _, child := range el.Children {
			if child == nil {
				continue
			}
			if canonType[child.Type] == s.Type && child.Key != "" {
				keys = append(keys, child.Key)
			}
			walk(child)
		}
	}
	walk(d.tree)
	if s.Index >= len(keys) {
		return "", fmt.Errorf("no %s widget at index %d", s.Type, s.Index)
	}
	return keys[s.Index], nil
}

func runCase(fn string, steps []step, extra map[string]bool) (any, error) {
	app, ok := apps[fn]
	if !ok {
		return nil, fmt.Errorf("unknown app %q", fn)
	}
	// The port's cache is process-wide, exactly as in a deployed app, so it is
	// cleared per case to keep the runner order-independent.
	st.CacheClear()

	ctx := newAppCtx()
	d := newDriver(func(s *st.Session) { ctx.run(fn, app, s) })
	defer d.close()

	ctx.reset()
	if err := d.post(nil); err != nil {
		return nil, err
	}
	snaps := []snapshot{d.snapshot(ctx, extra)}

	staged := map[string]any{}
	for _, s := range steps {
		switch s.Op {
		case "run":
			ctx.reset()
			if err := d.post(nil); err != nil {
				return nil, err
			}
		case "set":
			key, err := d.findWidget(s)
			if err != nil {
				return nil, err
			}
			ctx.reset()
			if err := d.post(&wireEvent{Key: key, Value: s.Value}); err != nil {
				return nil, err
			}
		case "click":
			key, err := d.findWidget(s)
			if err != nil {
				return nil, err
			}
			ctx.reset()
			if err := d.post(&wireEvent{Key: key, Button: true}); err != nil {
				return nil, err
			}
		case "stage":
			// A form widget's change is staged in the browser and never
			// reaches the server until the form is submitted.
			key, err := d.findWidget(s)
			if err != nil {
				return nil, err
			}
			staged[key] = s.Value
			continue
		case "submit":
			ctx.reset()
			ev := &wireEvent{Key: s.Form, Button: true, Form: s.Form, Values: staged}
			staged = map[string]any{}
			if err := d.post(ev); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unknown step op %q", s.Op)
		}
		snaps = append(snaps, d.snapshot(ctx, extra))
	}
	return map[string]any{"snapshots": snaps}, nil
}

func (d *driver) snapshot(ctx *appCtx, extra map[string]bool) snapshot {
	main, side := canonTree(d.tree, extra)
	return snapshot{Main: main, Sidebar: side, State: ctx.published()}
}

// reset clears the per-run publication buffer before a request drives the app.
func (a *appCtx) reset() { a.state = map[string]any{} }

// run invokes an app. It deliberately installs no recover(): the port halts a
// run by panicking an unexported sentinel that only its own run loop may
// recover, so a wrapper here would change the very control flow under test.
func (a *appCtx) run(name string, fn func(*appCtx, *st.Session), s *st.Session) {
	fn(a, s)
}

// published returns the app's own state keys, sorted, normalised for JSON.
func (a *appCtx) published() map[string]any {
	out := make(map[string]any, len(a.state))
	keys := make([]string, 0, len(a.state))
	for k := range a.state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = a.state[k]
	}
	return out
}

// trimFloat renders a float without a trailing ".0", so a cache key derived
// from a whole number is stable.
func trimFloat(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// ----------------------------------------------------------------------- main

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
			emit(out, reply{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		emit(out, handle(req))
	}
	_ = out.Flush()
}

// handle runs one case, turning any panic into ok:false rather than exiting.
func handle(req request) (rep reply) {
	rep = reply{ID: req.ID}
	defer func() {
		if r := recover(); r != nil {
			rep = reply{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	var steps []step
	if len(req.Args) > 0 && string(req.Args[0]) != "null" {
		if err := json.Unmarshal(req.Args[0], &steps); err != nil {
			return reply{ID: req.ID, OK: false, Error: "bad steps: " + err.Error()}
		}
	}
	extra := map[string]bool{}
	if len(req.Args) > 1 {
		var opts struct {
			Extra []string `json:"extra"`
		}
		if err := json.Unmarshal(req.Args[1], &opts); err == nil {
			for _, name := range opts.Extra {
				extra[name] = true
			}
		}
	}

	value, err := runCase(req.Fn, steps, extra)
	if err != nil {
		return reply{ID: req.ID, OK: false, Error: err.Error()}
	}
	return reply{ID: req.ID, OK: true, Value: value}
}

func emit(w *bufio.Writer, rep reply) {
	enc, err := json.Marshal(rep)
	if err != nil {
		enc, _ = json.Marshal(reply{ID: rep.ID, OK: false, Error: "encode: " + err.Error()})
	}
	_, _ = w.Write(enc)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}
