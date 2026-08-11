package rsc

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/malcolmston/react"
)

// propChildKeyPrefix marks a child that is really an element-valued prop.
//
// An element passed as a prop — a Suspense fallback, a slot, an icon — has to
// be rendered inside the tree that declared it, or it loses the context it was
// written in. The runtime only renders children, so [ClientRef.El] moves such
// props into the child list under a reserved key and the encoder puts them back
// afterwards. The prefix is chosen to be one no author would write.
const propChildKeyPrefix = "$rsc:prop:"

// Encoder writes a react tree as a Flight payload.
//
// One Encoder produces one payload: chunk ids are allocated per payload, and
// the client resolves them within a single response.
type Encoder struct {
	w io.Writer

	// nextID is the next chunk id to hand out. Row 0 is reserved for the root
	// model, so allocation starts at 1.
	nextID int

	// refs remembers the chunk assigned to each client reference so that a
	// component used twice is transmitted once and shares one module row.
	refs map[*ClientRef]int

	// rows holds the dependency rows — currently the `I` module rows — in
	// allocation order. They are written before the root model, matching what
	// React emits, so a client never has to block on a later row for a type it
	// needs immediately.
	rows []string
}

// NewEncoder returns an Encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w, nextID: 1, refs: map[*ClientRef]int{}}
}

// Render renders node and writes its Flight payload to w.
//
// The tree is mounted, rendered and unmounted here, so effects run and their
// cleanups run, exactly as [react.RenderToString] does. Server components are
// evaluated; client references are not, and travel as module references.
func Render(w io.Writer, node react.Node) error {
	return NewEncoder(w).Encode(node)
}

// RenderToString is [Render] into a string.
func RenderToString(node react.Node) (string, error) {
	var buf bytes.Buffer
	if err := Render(&buf, node); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Encode renders node and writes the payload.
func (e *Encoder) Encode(node react.Node) (err error) {
	// A panic anywhere in a component surfaces as an error, so a broken tree
	// fails the request rather than the process — the same contract
	// react.RenderToString offers.
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok {
				err = fmt.Errorf("rsc: render panicked: %w", rerr)
				return
			}
			err = fmt.Errorf("rsc: render panicked: %v", r)
		}
	}()

	root := react.NewRoot(node)
	defer root.Unmount()

	model, err := e.children(root.Tree())
	if err != nil {
		return err
	}

	encoded, err := marshal(model)
	if err != nil {
		return fmt.Errorf("rsc: encoding the root model: %w", err)
	}

	// Dependency rows first, then the root. The client tolerates either order,
	// but emitting types before the model that uses them means it never has to
	// park a chunk it could have resolved.
	var out bytes.Buffer
	for _, row := range e.rows {
		out.WriteString(row)
	}
	out.WriteString("0:")
	out.Write(encoded)
	out.WriteByte('\n')

	_, err = e.w.Write(out.Bytes())
	return err
}

// allocate returns the chunk id for a client reference, emitting its module row
// the first time it is seen.
func (e *Encoder) allocate(ref *ClientRef) (int, error) {
	if id, ok := e.refs[ref]; ok {
		return id, nil
	}

	id := e.nextID
	e.nextID++
	e.refs[ref] = id

	payload, err := marshal(ref.manifestRow())
	if err != nil {
		return 0, fmt.Errorf("rsc: encoding the module row for %s: %w", ref, err)
	}
	// Row ids are hexadecimal on the wire: the client accumulates them nibble
	// by nibble as it scans, so a decimal id above 9 would be misread.
	e.rows = append(e.rows, strconv.FormatInt(int64(id), 16)+":I"+string(payload)+"\n")
	return id, nil
}

// children converts an instance's children into a model value.
//
// The shape follows the *authored* tree, not the rendered one, because that is
// what React sends. React serializes props.children as the author wrote it:
// nulls, booleans and undefined survive, 0 and "" survive, and a nested array
// stays nested rather than being flattened into its parent. The rendered tree
// has had all of that normalized away, so reading it alone would produce a
// payload that renders identically but is not the same value — visible to any
// client component that counts or indexes its children.
//
// The rendered children are still needed, for the elements: each one supplies
// the model for the next element encountered in the authored walk. The two line
// up because reconciliation visits authored children depth-first and skips
// exactly the entries that produce no fiber.
func (e *Encoder) children(inst react.Instance) (any, error) {
	authored, ok := authoredChildren(inst)
	if !ok {
		return nil, nil
	}

	rendered := make([]react.Instance, 0, len(inst.Children()))
	for _, kid := range inst.Children() {
		// Children standing in for element-valued props belong to the parent's
		// props, not its child list; clientModel pulls them out.
		if strings.HasPrefix(kid.Key(), propChildKeyPrefix) {
			continue
		}
		rendered = append(rendered, kid)
	}

	cursor := 0
	// React's createElement collapses a lone variadic child to the value
	// itself, so `children` is an element rather than an array of one. Nested
	// arrays are never collapsed — only this outermost variadic list is.
	if list, isList := asNodeList(authored); isList && len(list) == 1 {
		return e.authoredValue(list[0], rendered, &cursor)
	}
	return e.authoredValue(authored, rendered, &cursor)
}

// authoredChildren returns the children value as written, and whether there is
// one at all. A component's authored children are whatever it returned; for
// everything else they are the declared children prop.
func authoredChildren(inst react.Instance) (any, bool) {
	if out, ok := inst.Output(); ok {
		return out, true
	}
	props := inst.Props()
	if !props.Has(react.ChildrenKey) {
		return nil, false
	}
	return props[react.ChildrenKey], true
}

// asNodeList reports whether v is a list of nodes, and returns its entries.
func asNodeList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []react.Node:
		out := make([]any, len(t))
		for i, n := range t {
			out[i] = n
		}
		return out, true
	case []any:
		return t, true
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// authoredValue converts one authored child into its model form, drawing on the
// rendered instances for anything that had to be evaluated.
//
// cursor walks the rendered children in step with the authored walk. Entries
// that produce no fiber — nil, true, false — do not advance it; everything else
// consumes exactly one, which is the invariant reconciliation guarantees.
func (e *Encoder) authoredValue(v any, rendered []react.Instance, cursor *int) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case bool:
		// React keeps booleans in the payload even though it renders nothing
		// for them. Go cannot distinguish false from undefined here, which is
		// the one asymmetry this reproduction cannot close.
		return t, nil
	case *react.Element:
		inst, err := takeRendered(rendered, cursor, v)
		if err != nil {
			return nil, err
		}
		return e.node(inst)
	case string:
		if _, err := takeRendered(rendered, cursor, v); err != nil {
			return nil, err
		}
		return encodeString(t), nil
	}

	if list, ok := asNodeList(v); ok {
		out := make([]any, len(list))
		for i, entry := range list {
			model, err := e.authoredValue(entry, rendered, cursor)
			if err != nil {
				return nil, err
			}
			out[i] = model
		}
		return out, nil
	}

	// A scalar leaf: a number, a Stringer. It rendered as a text node, so it
	// consumes a slot, but its value is sent as authored.
	if _, err := takeRendered(rendered, cursor, v); err != nil {
		return nil, err
	}
	return e.encodeValue(v)
}

// takeRendered consumes the next rendered child.
func takeRendered(rendered []react.Instance, cursor *int, authored any) (react.Instance, error) {
	if *cursor >= len(rendered) {
		return react.Instance{}, fmt.Errorf("rsc: the rendered tree has fewer children than the "+
			"tree that was written (ran out at a %T); this happens when a boundary replaced part "+
			"of the subtree — a suspended component under a Suspense fallback, or a caught error",
			authored)
	}
	inst := rendered[*cursor]
	*cursor++
	return inst, nil
}

// node converts one rendered instance into its model value.
func (e *Encoder) node(inst react.Instance) (any, error) {
	if raw, ok := inst.TextRaw(); ok {
		// TextRaw, not Text: React keeps a numeric child a number on the wire,
		// and a client component reading props.children would otherwise see a
		// string where React would have given it a number.
		return e.encodeValue(raw)
	}
	if tag, ok := inst.Tag(); ok {
		return e.hostModel(inst, tag)
	}
	if ref, ok := inst.Type().(*ClientRef); ok {
		return e.clientModel(inst, ref)
	}

	// Everything else — fragments, evaluated server components, Memo, context
	// providers, StrictMode, Profiler — contributes no node of its own and
	// renders through to its children.
	return e.children(inst)
}

// hostModel serializes a host element as ["$", tag, key, props].
func (e *Encoder) hostModel(inst react.Instance, tag string) (any, error) {
	props, err := e.props(inst)
	if err != nil {
		return nil, fmt.Errorf("in <%s>: %w", tag, err)
	}
	return []any{markerElement, tag, keyOf(inst), props}, nil
}

// clientModel serializes a client component as ["$", "$L<id>", key, props],
// where the type is a lazy reference to the module row.
func (e *Encoder) clientModel(inst react.Instance, ref *ClientRef) (any, error) {
	id, err := e.allocate(ref)
	if err != nil {
		return nil, err
	}

	props, err := e.props(inst)
	if err != nil {
		return nil, fmt.Errorf("in client component %s: %w", ref, err)
	}

	// Reattach the element-valued props that were rendered as keyed children.
	for _, kid := range inst.Children() {
		key := kid.Key()
		if !strings.HasPrefix(key, propChildKeyPrefix) {
			continue
		}
		name := strings.TrimPrefix(key, propChildKeyPrefix)
		value, err := e.children(kid)
		if err != nil {
			return nil, fmt.Errorf("in prop %q of %s: %w", name, ref, err)
		}
		props[name] = value
	}

	return []any{markerElement, prefixLazy + strconv.FormatInt(int64(id), 16), keyOf(inst), props}, nil
}

// props builds the props object for an instance: every declared prop except the
// reserved ones, plus the rendered children under "children".
func (e *Encoder) props(inst react.Instance) (map[string]any, error) {
	declared := inst.Props()
	out := make(map[string]any, len(declared)+1)

	names := make([]string, 0, len(declared))
	for name := range declared {
		names = append(names, name)
	}
	// Sorted for determinism. Go maps have no order, and a payload that differs
	// run to run cannot be diffed, cached or compared in a test.
	sort.Strings(names)

	for _, name := range names {
		switch name {
		case react.ChildrenKey, react.KeyProp, react.RefProp, propNamesKey:
			// children come from the rendered tree; key and ref are lifted out
			// of props by the element constructor and are not sent.
			continue
		}
		value, err := e.encodeValue(declared[name])
		if err != nil {
			return nil, err
		}
		out[name] = value
	}

	children, err := e.children(inst)
	if err != nil {
		return nil, err
	}
	if children != nil {
		out[react.ChildrenKey] = children
	}
	return out, nil
}

// keyOf returns the element key as the client expects it: a string, or null.
func keyOf(inst react.Instance) any {
	if key := inst.Key(); key != "" {
		return key
	}
	return nil
}
