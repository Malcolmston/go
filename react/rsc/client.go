// Package rsc serializes a Go react tree into React's Flight wire format — the
// payload a real React 19 client deserializes with
// `createFromFetch`/`createFromReadableStream` from `react-server-dom-*/client`.
//
// This is the port's interoperability story. A Go server component is *rendered
// here* and travels as data, so it needs no JavaScript counterpart; a client
// component stays a real JS module, is referenced by module id, and keeps its
// hooks, its event handlers and its interactivity. The split is React's own:
// server components produce the tree, client components make it interactive.
//
// # What this can and cannot do
//
// It can produce a payload that real React renders — the encoder is written
// against React's own decoder and verified by round-tripping through the real
// `react-server-dom-webpack` client, not against a written spec.
//
// It cannot make a Go component interactive. A Go component never reaches the
// browser; only its output does. Anything needing state in the browser must be
// a JS client component referenced with [Client]. That is not a limitation of
// this package — it is what "server component" means.
//
// # Stability
//
// Flight is an internal, undocumented, version-coupled format. React does not
// promise it across releases, so this encoder targets a specific React version
// (see [TargetVersion]) and must be re-verified against each upgrade. The
// round-trip tests exist precisely so that a mismatch fails loudly rather than
// showing up as a blank page in production.
package rsc

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/malcolmston/react"
)

// TargetVersion is the React version this encoder was verified against by
// round-tripping payloads through that release's real client decoder.
const TargetVersion = "19.2.8"

func init() {
	// A client reference renders its children — they are server-rendered and
	// handed to the client component as props.children, exactly as in React —
	// while the element itself stays identifiable by type so the encoder can
	// emit a module reference instead of markup.
	react.RegisterTransparentType((*ClientRef)(nil))
}

// ClientRef is a reference to a JavaScript client component.
//
// It is an [react.Element] type: build elements with [ClientRef.El] and they
// render into the tree like any other component, except that the encoder emits
// a module reference rather than the component's output. The client resolves
// that reference through its bundler manifest, loads the real module and
// renders it with the props sent from here.
//
// The three fields correspond exactly to the entries React's webpack client
// manifest holds for a module, which is why they are what an `I` row carries.
type ClientRef struct {
	// ID is the bundler's module identifier — the `id` field of the client
	// manifest entry, e.g. "./src/Button.js". It must match the manifest the
	// client was configured with, or the client throws "Could not find the
	// module … in the React Client Manifest".
	ID string

	// Chunks are the bundler chunk ids the client must load before the module
	// is available. Empty is valid when the module needs no extra chunk.
	Chunks []string

	// Name is the export to use from that module — "default" for a default
	// export, otherwise the named export.
	Name string

	// Async marks a module the client must await before rendering. It maps to
	// the manifest's `async` flag and is rarely needed.
	Async bool
}

// Client declares a client component. id is the bundler module id, name the
// export within it, and chunks the chunk ids to load first.
//
//	Button := rsc.Client("./src/Button.js", "Button", "static/chunks/button.js")
//	tree := react.H("main", nil, Button.El(react.Props{"label": "Save"}))
//
// The returned value must be created once and reused: like every element type,
// its identity is what reconciliation matches on, so building a fresh one per
// render remounts the subtree.
func Client(id, name string, chunks ...string) *ClientRef {
	if id == "" {
		panic("rsc: Client needs a non-empty module id")
	}
	if name == "" {
		panic("rsc: Client needs a non-empty export name; use \"default\" for a default export")
	}
	return &ClientRef{ID: id, Name: name, Chunks: chunks}
}

// propNamesKey is reserved on a client element's props. Nothing sets it today;
// it is skipped when serializing so that the name stays available for future
// encoder metadata without becoming a prop the client sees.
const propNamesKey = "$rsc:propNames"

// El builds an element that renders this client component on the client.
//
// Children are rendered here, on the server, and delivered as props.children —
// so a Go-authored subtree can be passed into a JS client component and appear
// inside it. That is the composition rule React server components are built
// around, and it works in both directions.
//
// Element-valued props get the same treatment. A prop holding elements is moved
// into the child list under a reserved key so the runtime renders it in place,
// with the surrounding context intact, and the encoder restores it to its own
// prop name afterwards. Rendering it anywhere else would be simpler and wrong:
// it would sever the element from the context it was written inside.
func (c *ClientRef) El(props react.Props, children ...react.Node) *react.Element {
	if c == nil {
		panic("rsc: El called on a nil *ClientRef")
	}

	kept := react.Props{}
	var elementProps []string
	for name, value := range props {
		switch name {
		case react.KeyProp, react.RefProp, react.ChildrenKey:
			kept[name] = value
			continue
		}
		if holdsElement(value) {
			elementProps = append(elementProps, name)
			continue
		}
		kept[name] = value
	}

	if len(elementProps) == 0 {
		return react.CreateElement(c, kept, children...)
	}
	// Deterministic order keeps the rendered child list — and therefore the
	// payload — stable across runs.
	sort.Strings(elementProps)

	// Declared children come first so that moving a prop never reorders them.
	kids := make([]react.Node, 0, len(children)+len(elementProps)+1)
	if declared, ok := kept[react.ChildrenKey]; ok && len(children) == 0 {
		kids = append(kids, declared)
	}
	delete(kept, react.ChildrenKey)
	kids = append(kids, children...)

	for _, name := range elementProps {
		kids = append(kids, react.CreateElement(react.Fragment,
			react.Props{react.KeyProp: propChildKeyPrefix + name}, props[name]))
	}
	return react.CreateElement(c, kept, kids...)
}

// holdsElement reports whether a prop value contains elements and therefore has
// to be rendered rather than serialized directly.
func holdsElement(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case *react.Element:
		return true
	case []react.Node:
		for _, n := range t {
			if holdsElement(n) {
				return true
			}
		}
		return false
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		for i := range rv.Len() {
			if holdsElement(rv.Index(i).Interface()) {
				return true
			}
		}
	}
	return false
}

// String renders the reference the way React's manifest keys it, which is the
// form its error messages quote — so a mismatch is greppable.
func (c *ClientRef) String() string {
	if c == nil {
		return "<nil client reference>"
	}
	return fmt.Sprintf("%s#%s", c.ID, c.Name)
}

// manifestRow is the payload of the reference's `I` row: React's client decoder
// reads it as [moduleID, chunks, exportName] and, when async, a fourth element.
func (c *ClientRef) manifestRow() []any {
	chunks := c.Chunks
	if chunks == nil {
		// A nil slice marshals as JSON null; the decoder expects an array.
		chunks = []string{}
	}
	row := []any{c.ID, chunks, c.Name}
	if c.Async {
		row = append(row, true)
	}
	return row
}
