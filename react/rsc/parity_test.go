package rsc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/malcolmston/react"
)

// The fixtures in testdata were produced by React 19.2.8 itself — by rendering
// an equivalent JSX tree through react-server-dom-webpack's own
// renderToPipeableStream — and captured verbatim. Each test here builds the same
// tree in Go and asserts this encoder produces the same payload.
//
// The comparison is on the decoded structure, not on bytes, for one reason:
// JavaScript objects keep insertion order while Go maps have none, so this
// encoder sorts prop names. Every other difference — a missing marker, a number
// sent as a string, a wrong chunk id, a collapsed single child — still fails.
//
// Regenerate the fixtures with the script documented in testdata/README.md after
// a React upgrade. A diff there is the signal that the wire format moved.

// row is one line of a Flight payload.
type row struct {
	// id is the chunk id, decoded from its hexadecimal form on the wire.
	id int64

	// tag is the row type: 0 for a model row, 'I' for a module row, and so on.
	tag byte

	// model is the row's payload, decoded from JSON.
	model any
}

// parsePayload splits a Flight payload into rows and decodes each one.
func parsePayload(t *testing.T, payload string) map[int64]row {
	t.Helper()

	rows := map[int64]row{}
	for _, line := range strings.Split(strings.TrimRight(payload, "\n"), "\n") {
		if line == "" {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			t.Fatalf("row has no id separator: %q", line)
		}
		id, err := strconv.ParseInt(line[:colon], 16, 64)
		if err != nil {
			t.Fatalf("row id %q is not hexadecimal: %v", line[:colon], err)
		}

		rest := line[colon+1:]
		var tag byte
		// A row whose payload does not start with a JSON value carries a tag
		// byte first. Model rows have no tag.
		if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' {
			tag = rest[0]
			rest = rest[1:]
		}

		var model any
		if err := json.Unmarshal([]byte(rest), &model); err != nil {
			t.Fatalf("row %d payload is not JSON: %v\n%s", id, err, rest)
		}
		rows[id] = row{id: id, tag: tag, model: model}
	}
	return rows
}

// loadFixture reads a captured React payload.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".flight"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(data)
}

// assertMatchesReact renders node and compares the payload to the captured one.
func assertMatchesReact(t *testing.T, name string, node react.Node) {
	t.Helper()

	got, err := RenderToString(node)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	want := loadFixture(t, name)
	gotRows, wantRows := parsePayload(t, got), parsePayload(t, want)

	if len(gotRows) != len(wantRows) {
		t.Fatalf("row count = %d, want %d\n  go:    %s\n  react: %s",
			len(gotRows), len(wantRows), got, want)
	}

	ids := make([]int64, 0, len(wantRows))
	for id := range wantRows {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })

	for _, id := range ids {
		w := wantRows[id]
		g, ok := gotRows[id]
		if !ok {
			t.Errorf("missing row %d", id)
			continue
		}
		if g.tag != w.tag {
			t.Errorf("row %d tag = %q, want %q", id, g.tag, w.tag)
		}
		if !reflect.DeepEqual(g.model, w.model) {
			t.Errorf("row %d differs\n  go:    %s\n  react: %s", id, mustJSON(g.model), mustJSON(w.model))
		}
	}

	if t.Failed() {
		t.Logf("full payloads:\n  go:    %s\n  react: %s", got, want)
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<unencodable: %v>", err)
	}
	return string(b)
}

// greeting is the Go counterpart of the fixture's JS server component. React
// evaluates a server component before serializing; so does this port, so the
// two payloads should contain the component's output and no trace of the
// component itself.
var greeting react.Component = func(props react.Props) react.Node {
	return react.H("p", nil, "hello ", props.String("who"), "!")
}

func TestParityBasicTree(t *testing.T) {
	var items []react.Node
	for _, i := range []int{1, 2} {
		items = append(items, react.H("li", react.Props{"key": "k" + strconv.Itoa(i)}, "item", i))
	}

	assertMatchesReact(t, "basic",
		react.H("div", react.Props{"id": "root"},
			react.H("h1", nil, "Title"),
			react.CreateElement(greeting, react.Props{"who": "world"}),
			nil, false, 7,
			react.H("ul", nil, items),
		))
}

func TestParityProps(t *testing.T) {
	assertMatchesReact(t, "props",
		react.H("div", react.Props{
			"className": "a b",
			"data-n":    3,
			"hidden":    true,
			"tabIndex":  0,
			"style":     map[string]any{"color": "red", "marginTop": 4},
			"title":     Undefined,
		},
			react.H("input", react.Props{"defaultValue": "v", "disabled": false}),
		))
}

func TestParitySpecialValues(t *testing.T) {
	assertMatchesReact(t, "specials",
		react.H("div", react.Props{
			"inf":    mathInf(1),
			"ninf":   mathInf(-1),
			"nan":    mathNaN(),
			"nzero":  negZero(),
			"when":   time.Date(2020, 1, 2, 3, 4, 5, 678_000_000, time.UTC),
			"dollar": "$literal",
			"empty":  "",
			"deep": map[string]any{
				"a": []any{1, "x", nil},
				"b": map[string]any{"c": true},
			},
		}))
}

func TestParityClientReferences(t *testing.T) {
	// One reference used twice must be transmitted once and share a chunk id —
	// the behaviour that keeps a payload from growing with every repeat of a
	// component.
	button := Client("./src/Button.js", "Button", "static/button.js")

	assertMatchesReact(t, "client",
		react.H("main", nil,
			button.El(react.Props{"label": "Save", "count": 2},
				react.H("span", nil, "inner")),
			button.El(react.Props{"label": "Again"}),
		))
}

func TestParityFragmentRoot(t *testing.T) {
	assertMatchesReact(t, "fragment",
		react.Frag(
			react.H("a", nil, "one"),
			react.H("b", nil, "two"),
		))
}

// mathInf, mathNaN and negZero build the IEEE values without importing math
// into the test's readable surface.
func mathInf(sign int) float64 {
	if sign > 0 {
		return 1.0 / zero()
	}
	return -1.0 / zero()
}

func mathNaN() float64 { return zero() / zero() }
func zero() float64    { z := 0.0; return z }
func negZero() float64 { return zero() * -1 }

// TestParityEmptyAndNestedChildren is the case that forced the encoder to walk
// the authored tree rather than the rendered one.
//
// React keeps null, false and true in the payload even though it renders
// nothing for any of them; it keeps 0 and "" rather than treating them as
// empty; and it does not flatten a nested array into its parent. The rendered
// tree has none of that information left.
func TestParityEmptyAndNestedChildren(t *testing.T) {
	assertMatchesReact(t, "empties",
		react.H("div", nil,
			nil, false, true, 0, "", "x",
			[]react.Node{nil, "y"},
			react.H("i", nil, "e"),
		))
}
