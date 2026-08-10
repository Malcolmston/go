// Command run is the Go (github.com/malcolmston/liveview) side of the
// cross-language parity harness.
//
// It speaks JSON Lines on stdio: one request object per line in, one response
// object per line out, in the same order. It never exits on a failing case — a
// panic or error is reported as ok:false and the loop continues.
//
// Nothing here starts a server, a WebSocket or a goroutine that outlives a
// case: every answer comes from rendering programmatically through
// liveview.Template, liveview.FullDiff/DiffRendered and liveview.Session.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	liveview "github.com/malcolmston/liveview"
)

// ------------------------------------------------------------- templates ----
//
// Each of these is the Go port's spelling of the same *logical* template the
// Elixir runner writes in HEEx. Template syntax is out of scope for the
// comparison (see ../COVERAGE.md); only the emitted diff structure is compared.

var (
	tmplCounter = liveview.MustParse(`<div class="counter"><h1>{{ label }}</h1><span class="value">{{ count }}</span></div>`)
	tmplPair    = liveview.MustParse(`<p>{{ a }}{{ b }}</p>`)
	tmplLead    = liveview.MustParse(`{{ a }}<p>x</p>`)
	tmplTrail   = liveview.MustParse(`<p>x</p>{{ a }}`)
	tmplPlain   = liveview.MustParse(`<p>hi</p>`)
	tmplOne     = liveview.MustParse(`<i>{{ a }}</i>`)
	tmplNest    = liveview.MustParse(`<div><b>{{ a }}</b><section>{{ inner }}</section></div>`)
	tmplInner   = liveview.MustParse(`<i>{{ v }}</i>`)
	tmplNoRaw   = liveview.MustParse(`<div>{{ html }}</div>`)
	tmplRaw     = liveview.MustParse(`<div>{{ html }}</div>`)
	tmplMissing = liveview.MustParse(`<p>{{ nope }}</p>`)
	tmplLi      = liveview.MustParse(`<li>{{ i }}</li>`)
	tmplTr      = liveview.MustParse(`<tr><td>{{ r }}</td><td>{{ r }}</td></tr>`)
	tmplBadge   = liveview.MustParse(`<span class="badge">{{ text }}:{{ hits }}</span>`)
	tmplWith1   = liveview.MustParse(`<div><h1>{{ title }}</h1>{{ b1 }}</div>`)
	tmplWith2   = liveview.MustParse(`<div><h1>{{ title }}</h1>{{ b1 }}{{ b2 }}</div>`)
)

// render builds the *liveview.Rendered for one logical template name.
func render(name string, assigns map[string]any) (*liveview.Rendered, error) {
	switch name {
	case "counter":
		return tmplCounter.Render(assigns), nil
	case "pair":
		return tmplPair.Render(assigns), nil
	case "lead":
		return tmplLead.Render(assigns), nil
	case "trail":
		return tmplTrail.Render(assigns), nil
	case "plain":
		return tmplPlain.Render(assigns), nil
	case "one":
		return tmplOne.Render(assigns), nil
	case "nest":
		inner := tmplInner.Render(map[string]any{"v": assigns["v"]})
		return tmplNest.Render(map[string]any{"a": assigns["a"], "inner": inner}), nil
	case "noraw":
		return tmplNoRaw.Render(assigns), nil
	case "raw":
		// The port's raw/safe passthrough: wrap the value in liveview.Safe so it
		// is interpolated without escaping.
		s, _ := assigns["html"].(string)
		return tmplRaw.Render(map[string]any{"html": liveview.Safe(s)}), nil
	case "missing":
		return tmplMissing.Render(assigns), nil
	case "items":
		return comprehension(tmplLi, "i", assigns["items"], "<ul>", "</ul>"), nil
	case "rows":
		return comprehension(tmplTr, "r", assigns["items"], "<table>", "</table>"), nil
	default:
		return nil, fmt.Errorf("unknown template %q", name)
	}
}

// comprehension builds the closest thing the Go port can express for a list
// comprehension. The port has no comprehension construct at all: a Template has
// a fixed number of slots, so a per-item sub-render has to be spliced into a
// parent Rendered whose statics grow with the list. One nested *Rendered per
// entry is the most faithful encoding available, and it is what the resulting
// diff is compared against.
func comprehension(item *liveview.Template, field string, list any, open, close string) *liveview.Rendered {
	entries, _ := list.([]any)
	if len(entries) == 0 {
		return &liveview.Rendered{Statics: []string{open + close}}
	}
	statics := make([]string, 0, len(entries)+1)
	dynamics := make([]any, 0, len(entries))
	statics = append(statics, open)
	for i, e := range entries {
		if i > 0 {
			statics = append(statics, "")
		}
		dynamics = append(dynamics, item.Render(map[string]any{field: e}))
	}
	statics = append(statics, close)
	return &liveview.Rendered{Statics: statics, Dynamics: dynamics}
}

// ----------------------------------------------------- views & components ----

// badge is the Go counterpart of the Elixir LiveViewParity.Badge live
// component: its own "hits" counter plus a "text" handed down by the parent.
type badge struct {
	id   string
	text string
}

func (b *badge) ID() string { return b.id }

func (b *badge) Mount(s *liveview.Socket) error {
	s.Assign("text", b.text)
	s.Assign("hits", 0)
	return nil
}

func (b *badge) HandleEvent(event string, payload map[string]any, s *liveview.Socket) error {
	switch event {
	case "bump":
		s.Assign("hits", s.GetInt("hits")+1)
	case "set":
		s.Assign("hits", payload["value"])
	}
	return nil
}

func (b *badge) Render(assigns map[string]any) *liveview.Rendered {
	return tmplBadge.Render(assigns)
}

// compView is the Go counterpart of the "withcomp"/"twocomp" HEEx parents: a
// title plus one or two stateful components.
type compView struct{ n int }

func (v *compView) Mount(params map[string]any, s *liveview.Socket) error {
	title := "Title"
	if t, ok := params["title"].(string); ok && t != "" {
		title = t
	}
	text := "hi"
	if t, ok := params["text"].(string); ok && t != "" {
		text = t
	}
	s.Assign("title", title)
	s.Assign("text", text)
	s.Assign("b1", s.LiveComponent(&badge{id: "b1", text: text}))
	if v.n == 2 {
		s.Assign("b2", s.LiveComponent(&badge{id: "b2", text: text}))
	}
	return nil
}

func (v *compView) HandleEvent(event string, payload map[string]any, s *liveview.Socket) error {
	if event == "title" {
		if t, ok := payload["text"].(string); ok {
			s.Assign("title", t)
		}
	}
	return nil
}

func (v *compView) Render(assigns map[string]any) *liveview.Rendered {
	if v.n == 2 {
		return tmplWith2.Render(assigns)
	}
	return tmplWith1.Render(assigns)
}

func newView(name string) (liveview.View, error) {
	switch name {
	case "counter":
		return &liveview.Counter{}, nil
	case "withcomp":
		return &compView{n: 1}, nil
	case "twocomp":
		return &compView{n: 2}, nil
	default:
		return nil, fmt.Errorf("unknown view %q", name)
	}
}

// ---------------------------------------------------------------- dispatch ----

func dispatch(fn string, args []json.RawMessage) (any, error) {
	switch fn {
	case "render_full":
		name, assigns, err := tmplArgs(args)
		if err != nil {
			return nil, err
		}
		r, err := render(name, assigns)
		if err != nil {
			return nil, err
		}
		return canon(liveview.FullDiff(r)), nil

	case "render_html":
		name, assigns, err := tmplArgs(args)
		if err != nil {
			return nil, err
		}
		r, err := render(name, assigns)
		if err != nil {
			return nil, err
		}
		return r.HTML(), nil

	// render_diff and render_diff_reassign are the same operation for the port:
	// it has no assign-level change tracking, its minimality comes from
	// comparing the previous and next Rendered value by value.
	case "render_diff", "render_diff_reassign":
		if len(args) != 3 {
			return nil, fmt.Errorf("%s wants 3 args, got %d", fn, len(args))
		}
		var name string
		if err := json.Unmarshal(args[0], &name); err != nil {
			return nil, err
		}
		var before, later map[string]any
		if err := json.Unmarshal(args[1], &before); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(args[2], &later); err != nil {
			return nil, err
		}
		prev, err := render(name, before)
		if err != nil {
			return nil, err
		}
		next, err := render(name, later)
		if err != nil {
			return nil, err
		}
		return canon(liveview.DiffRendered(prev, next)), nil

	case "session_initial", "session_initial_wire":
		if len(args) != 2 {
			return nil, fmt.Errorf("%s wants 2 args, got %d", fn, len(args))
		}
		sess, err := mount(args[0], args[1])
		if err != nil {
			return nil, err
		}
		return canon(sess.InitialDiff()), nil

	case "session_event":
		view, params, event, payload, err := eventArgs(args)
		if err != nil {
			return nil, err
		}
		sess, err := mountValues(view, params)
		if err != nil {
			return nil, err
		}
		// The WebSocket transport ships InitialDiff() as its first frame, which
		// is what marks every component's statics as sent.
		_ = sess.InitialDiff()
		d, err := sess.Event(event, payload)
		if err != nil {
			return nil, err
		}
		return canon(d), nil

	// The HTTP event transport (Handler.serveEvent) never calls InitialDiff(),
	// even though the client already holds the document from the server-rendered
	// GET. This reproduces exactly that path.
	case "session_event_no_initial":
		view, params, event, payload, err := eventArgs(args)
		if err != nil {
			return nil, err
		}
		sess, err := mountValues(view, params)
		if err != nil {
			return nil, err
		}
		d, err := sess.Event(event, payload)
		if err != nil {
			return nil, err
		}
		return canon(d), nil

	case "component_event":
		if len(args) != 5 {
			return nil, fmt.Errorf("component_event wants 5 args, got %d", len(args))
		}
		var view string
		if err := json.Unmarshal(args[0], &view); err != nil {
			return nil, err
		}
		var params map[string]any
		if err := json.Unmarshal(args[1], &params); err != nil {
			return nil, err
		}
		var cid int
		if err := json.Unmarshal(args[2], &cid); err != nil {
			return nil, err
		}
		var event string
		if err := json.Unmarshal(args[3], &event); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(args[4], &payload); err != nil {
			return nil, err
		}
		sess, err := mountValues(view, params)
		if err != nil {
			return nil, err
		}
		_ = sess.InitialDiff()
		d, err := sess.ComponentEvent(cid, event, payload)
		if err != nil {
			return nil, err
		}
		return canon(d), nil

	default:
		return nil, fmt.Errorf("unsupported fn %q", fn)
	}
}

func tmplArgs(args []json.RawMessage) (string, map[string]any, error) {
	if len(args) != 2 {
		return "", nil, fmt.Errorf("wants 2 args, got %d", len(args))
	}
	var name string
	if err := json.Unmarshal(args[0], &name); err != nil {
		return "", nil, err
	}
	var assigns map[string]any
	if err := json.Unmarshal(args[1], &assigns); err != nil {
		return "", nil, err
	}
	return name, assigns, nil
}

func eventArgs(args []json.RawMessage) (string, map[string]any, string, map[string]any, error) {
	if len(args) != 4 {
		return "", nil, "", nil, fmt.Errorf("wants 4 args, got %d", len(args))
	}
	var view, event string
	var params, payload map[string]any
	if err := json.Unmarshal(args[0], &view); err != nil {
		return "", nil, "", nil, err
	}
	if err := json.Unmarshal(args[1], &params); err != nil {
		return "", nil, "", nil, err
	}
	if err := json.Unmarshal(args[2], &event); err != nil {
		return "", nil, "", nil, err
	}
	if err := json.Unmarshal(args[3], &payload); err != nil {
		return "", nil, "", nil, err
	}
	return view, params, event, payload, nil
}

func mount(rawView, rawParams json.RawMessage) (*liveview.Session, error) {
	var view string
	if err := json.Unmarshal(rawView, &view); err != nil {
		return nil, err
	}
	var params map[string]any
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}
	return mountValues(view, params)
}

func mountValues(view string, params map[string]any) (*liveview.Session, error) {
	v, err := newView(view)
	if err != nil {
		return nil, err
	}
	sess := liveview.NewSession(v)
	if _, err := sess.Mount(params); err != nil {
		return nil, err
	}
	return sess, nil
}

// ------------------------------------------------------------ normalising ----

// canon converts a liveview.Diff into the shape both runners agree to emit:
// plain JSON objects with string keys, "s" holding the literal statics list,
// "c" holding per-cid component diffs, and cids renumbered in order of first
// appearance starting at 1 (the port already numbers them that way, so the
// remap is the identity here; it exists so neither side is compared on its
// numbering scheme). Object keys are sorted by encoding/json on the way out.
func canon(d liveview.Diff) map[string]any {
	comps, _ := d[cKey].(map[string]any)
	cids := cidOrder(d, comps)

	out := walk(d, cids)
	if len(comps) > 0 {
		c := make(map[string]any, len(comps))
		for k, v := range comps {
			nk := k
			if n, ok := atoi(k); ok {
				if mapped, ok := cids[n]; ok {
					nk = itoa(mapped)
				}
			}
			c[nk] = walkValue(v, cids)
		}
		out[cKey] = c
	}
	return out
}

const cKey = "c"

func walk(d liveview.Diff, cids map[int]int) map[string]any {
	out := make(map[string]any, len(d))
	for k, v := range d {
		if k == cKey {
			continue
		}
		out[k] = walkValue(v, cids)
	}
	return out
}

func walkValue(v any, cids map[int]int) any {
	switch t := v.(type) {
	case liveview.Diff:
		return walk(t, cids)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = walkValue(vv, cids)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = walkValue(e, cids)
		}
		return out
	case int:
		if mapped, ok := cids[t]; ok {
			return mapped
		}
		return t
	default:
		return v
	}
}

// cidOrder assigns each cid a stable index in order of first appearance: the
// parent's numbered slots in ascending numeric order first, then any remaining
// cid present under "c".
func cidOrder(d liveview.Diff, comps map[string]any) map[int]int {
	known := map[int]bool{}
	for k := range comps {
		if n, ok := atoi(k); ok {
			known[n] = true
		}
	}

	slots := make([]string, 0, len(d))
	for k := range d {
		if _, ok := atoi(k); ok {
			slots = append(slots, k)
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		a, _ := atoi(slots[i])
		b, _ := atoi(slots[j])
		return a < b
	})

	order := []int{}
	seen := map[int]bool{}
	for _, k := range slots {
		if n, ok := d[k].(int); ok && known[n] && !seen[n] {
			seen[n] = true
			order = append(order, n)
		}
	}
	rest := make([]int, 0, len(known))
	for n := range known {
		if !seen[n] {
			rest = append(rest, n)
		}
	}
	sort.Ints(rest)
	order = append(order, rest...)

	out := make(map[int]int, len(order))
	for i, n := range order {
		out[n] = i + 1
	}
	return out
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// ------------------------------------------------------------------- main ----

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

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, response{OK: false, Error: "bad request line: " + err.Error()})
			continue
		}
		emit(out, answer(req))
	}
}

// answer runs one case, converting a panic into ok:false so a single bad case
// never kills the runner.
func answer(req request) (resp response) {
	resp = response{ID: req.ID}
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	v, err := dispatch(req.Fn, req.Args)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: v}
}

func emit(out *bufio.Writer, resp response) {
	raw, err := marshal(resp)
	if err != nil {
		raw, _ = marshal(response{ID: resp.ID, OK: false, Error: "unencodable value: " + err.Error()})
	}
	out.Write(raw)
	out.WriteByte('\n')
	out.Flush()
	os.Stdout.Sync()
}

// marshal encodes without encoding/json's HTML escaping, so the statics in a
// mismatch report read as the markup they are. encoding/json sorts map keys, so
// every emitted object is key-sorted as HARNESS.md requires.
func marshal(resp response) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
