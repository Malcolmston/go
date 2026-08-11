package rsc

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/malcolmston/react"
)

// The round trip: a payload this encoder produced, handed to React's own client
// decoder, rendered to HTML by react-dom, and compared to what React produces
// for the equivalent tree.
//
// It is the only test here that proves compatibility rather than resemblance —
// [TestParityBasicTree] and its neighbours compare against captured payloads,
// which would agree with each other even if both were wrong about what the
// decoder accepts.
//
// It needs Node and an npm install, so it is opt-in:
//
//	cd testdata/roundtrip && npm install
//	RSC_ROUNDTRIP=1 go test ./rsc/...
//
// Everything else in this package runs with no toolchain at all.

// roundtrip encodes node, decodes it with the real React client and returns the
// HTML that came out.
func roundtrip(t *testing.T, node react.Node) string {
	t.Helper()

	harness := filepath.Join("testdata", "roundtrip")
	if _, err := os.Stat(filepath.Join(harness, "node_modules")); err != nil {
		t.Skipf("round-trip harness is not installed; run `cd %s && npm install`", harness)
	}

	payload, err := RenderToString(node)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	file := filepath.Join(t.TempDir(), "payload.flight")
	if err := os.WriteFile(file, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("node", "decode.mjs", file)
	cmd.Dir = harness
	cmd.Env = append(os.Environ(), "NODE_ENV=production")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("React's decoder rejected the payload: %v\n\npayload:\n%s\n\noutput:\n%s",
			err, payload, out)
	}
	return string(out)
}

func TestRoundTripThroughRealReact(t *testing.T) {
	if os.Getenv("RSC_ROUNDTRIP") == "" {
		t.Skip("set RSC_ROUNDTRIP=1 to run the round trip against the real React client")
	}

	var greet react.Component = func(p react.Props) react.Node {
		return react.H("p", nil, "hello ", p.String("who"), "!")
	}

	cases := []struct {
		name string
		node react.Node
		want string
	}{
		{
			name: "host tree with an evaluated server component",
			node: react.H("div", react.Props{"id": "root"},
				react.H("h1", nil, "Title"),
				react.CreateElement(greet, react.Props{"who": "world"}),
				nil, false, 7,
			),
			want: `<div id="root"><h1>Title</h1><p>hello world!</p>7</div>`,
		},
		{
			name: "keyed list keeps its order",
			node: react.H("ul", nil, []react.Node{
				react.H("li", react.Props{"key": "a"}, "one"),
				react.H("li", react.Props{"key": "b"}, "two"),
			}),
			want: `<ul><li>one</li><li>two</li></ul>`,
		},
		{
			name: "boolean and nil children render as nothing",
			node: react.H("p", nil, nil, false, true, "only"),
			want: `<p>only</p>`,
		},
		{
			name: "props survive the trip",
			node: react.H("input", react.Props{"defaultValue": "v", "disabled": false}),
			want: `<input value="v"/>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := roundtrip(t, c.node); got != c.want {
				t.Errorf("HTML after the round trip =\n  %s\nwant\n  %s", got, c.want)
			}
		})
	}
}

// TestRoundTripResolvesClientReferences is the interoperability claim in one
// test: a Go-authored tree containing a reference to a JavaScript component is
// decoded by React, the reference is resolved through a bundler manifest to a
// real JS module, and that component renders with the props Go sent it and the
// children Go rendered for it.
func TestRoundTripResolvesClientReferences(t *testing.T) {
	if os.Getenv("RSC_ROUNDTRIP") == "" {
		t.Skip("set RSC_ROUNDTRIP=1 to run the round trip against the real React client")
	}

	// Matches the manifest the harness is configured with.
	button := Client("./src/Button.js", "Button", "static/button.js")

	node := react.H("main", nil,
		button.El(react.Props{"label": "Save", "count": 2},
			react.H("span", nil, "server-rendered child")),
		button.El(react.Props{"label": "Again"}),
	)

	want := `<main><button data-count="2">Save<span>server-rendered child</span></button>` +
		`<button>Again</button></main>`

	if got := roundtrip(t, node); got != want {
		t.Errorf("HTML after the round trip =\n  %s\nwant\n  %s", got, want)
	}
}
