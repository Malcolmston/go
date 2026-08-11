package react

import (
	"fmt"
	"reflect"
	"strconv"
)

// Node is anything that may appear in a component's output or in a props
// "children" list. It mirrors React's ReactNode union, which has no direct Go
// equivalent, so the port models it as an empty interface constrained by
// convention. The values the renderer understands are:
//
//   - nil, false — rendered as nothing (React drops null/undefined/false)
//   - true — also rendered as nothing, matching JSX's `{cond && <x/>}` idiom
//   - string, all integer and float kinds — rendered as a text node
//   - *Element — an element produced by [CreateElement] or a helper
//   - []Node, and any other slice — flattened in place, like a JSX array child
//
// Anything else is a programming error and panics at render time with a message
// naming the offending Go type. Note that unlike React, a bare `false` and a
// bare `true` are both dropped: Go has no distinct undefined, and rendering the
// literal text "true" is never what a `cond && node` expression means.
type Node any

// Props is the property bag handed to a component. It is React's props object:
// an untyped string-keyed map rather than a struct, because a component's
// accepted props are open-ended and host elements ("div", "input") take
// arbitrary HTML attributes.
//
// Two keys are special and reserved by the runtime, exactly as in React:
// "children" holds the child nodes (see [Props.Children]) and "key" is lifted
// out of the map into [Element.Key] by [CreateElement]. A "ref" prop is lifted
// into [Element.Ref].
//
// Props values are read-only. A component must never mutate the map it is
// given — the runtime reuses props across bailouts, so a mutation is visible to
// the previous render and breaks memoization.
type Props map[string]any

// ChildrenKey is the reserved props key under which child nodes are stored.
const ChildrenKey = "children"

// KeyProp and RefProp are the reserved props keys that [CreateElement] lifts
// out of the props map into [Element.Key] and [Element.Ref]. They never reach a
// component's Props.
const (
	KeyProp = "key"
	RefProp = "ref"
)

// Children returns the element's child nodes, flattened into a single slice
// with nils, booleans and nested slices resolved the way the renderer resolves
// them. It returns an empty slice — never nil — when there are no children, so
// callers can range over the result unconditionally.
//
// This is the accessor components should use to read their children; reading
// p[ChildrenKey] directly exposes the un-normalized value, which may be a bare
// Node rather than a slice.
func (p Props) Children() []Node {
	if p == nil {
		return []Node{}
	}
	return NormalizeChildren(p[ChildrenKey])
}

// Get returns the value stored under key, or nil when the key is absent. It is
// a nil-safe map read, so it works on a nil Props.
func (p Props) Get(key string) any {
	if p == nil {
		return nil
	}
	return p[key]
}

// Has reports whether key is present, distinguishing an explicit nil value from
// an absent key — a distinction that matters for HTML attributes, where
// present-and-nil and absent render differently.
func (p Props) Has(key string) bool {
	if p == nil {
		return false
	}
	_, ok := p[key]
	return ok
}

// String returns the value at key rendered as a string, or "" when the key is
// absent or holds nil. Strings are returned as-is; every other type is
// formatted the way a text child would be.
func (p Props) String(key string) string {
	v := p.Get(key)
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return textOf(v)
}

// Bool returns the boolean at key. It reports ok=false when the key is absent
// or does not hold a bool, which lets callers distinguish "false" from "unset"
// — the difference between rendering disabled={false} and omitting it.
func (p Props) Bool(key string) (value, ok bool) {
	b, ok := p.Get(key).(bool)
	return b, ok
}

// Clone returns a shallow copy of the props. Use it when a component needs to
// forward its props with one key changed: mutating the received map in place is
// never allowed.
func (p Props) Clone() Props {
	out := make(Props, len(p)+1)
	for k, v := range p {
		out[k] = v
	}
	return out
}

// Component is a function component: it receives props and returns the nodes to
// render. This is the only kind of component the port supports — React 19
// removed class components from the recommended surface, and Go has no natural
// analogue for `this.setState`, so hooks are the single state mechanism.
//
// A component must be a pure function of its props, its hook state and the
// contexts it reads. The runtime calls it more than once per visible update
// (during a re-render triggered by any ancestor, and again when a state update
// is replayed), so side effects belong in UseEffect, never in the body.
type Component func(props Props) Node

// Element is one node of the element tree — the Go equivalent of a JSX element
// or a `React.createElement` result. Elements are immutable, plain data
// descriptions of what to render; they are not the rendered output and hold no
// state. The runtime turns them into fibers, which is where state lives.
type Element struct {
	// Type identifies what to render. The runtime understands:
	//
	//   string      — a host element; the string is the tag name ("div")
	//   Component   — a function component
	//   Fragment    — the fragment sentinel; children render with no wrapper
	//   textTag     — an internal text node (created by the runtime, not users)
	//
	// plus any type registered by [RegisterElementRenderer], which is how
	// context providers, Memo, Lazy, Suspense and the other built-in wrappers
	// plug in without this switch growing a case per feature.
	Type any

	// Key is the reconciliation identity of this element among its siblings,
	// lifted out of props by [CreateElement]. An empty Key means "match by
	// position", React's default. Keys only need to be unique among siblings.
	Key string

	// Props are the element's properties, including ChildrenKey. Never nil for
	// an element built by [CreateElement].
	Props Props

	// Ref is the value passed as the reserved "ref" prop. In React 19 refs are
	// ordinary props for function components; the field is kept because host
	// elements still need somewhere to put one.
	Ref any
}

// fragmentTag is the type of the [Fragment] sentinel. It is a distinct named
// type so the renderer can match it by Go type rather than by value.
type fragmentTag struct{}

// Fragment is the element type that groups children without rendering a wrapper
// element, the equivalent of React's <>…</>. Use it as the Type argument to
// [CreateElement] when a component must return several siblings.
var Fragment any = fragmentTag{}

// textTag is the type of the internal text elements the runtime creates when it
// normalizes a string or number child. User code never constructs one; it reads
// text through [TextValue].
type textTag struct{}

// textValueKey is the props key under which a text element stores its content.
// textRawKey holds the value it was created from, before formatting.
const (
	textValueKey = "value"
	textRawKey   = "raw"
)

// newTextElement wraps a rendered leaf in the internal text element type, so
// that every child in the tree is a *Element and the reconciler has exactly one
// shape to diff.
//
// raw is the value the text was formatted from. Rendering only ever needs the
// string, but a serializer that has to reproduce JavaScript's distinction
// between the child 7 and the child "7" needs the original — see [TextRaw].
func newTextElement(s string, raw any) *Element {
	return &Element{Type: textTag{}, Props: Props{textValueKey: s, textRawKey: raw}}
}

// TextRaw returns the value a text element was created from — a string, an int,
// a float — and reports whether el is a text element.
//
// [TextValue] is what a renderer wants; this is what a serializer wants. The two
// differ only in that this one has not yet collapsed numbers into their string
// form, which matters when the output format distinguishes them.
func TextRaw(el *Element) (any, bool) {
	if el == nil {
		return nil, false
	}
	if _, ok := el.Type.(textTag); !ok {
		return nil, false
	}
	return el.Props[textRawKey], true
}

// TextValue returns the string content of a text element and reports whether el
// is one. Renderers use it to distinguish the text leaves of the tree from host
// elements; it is the only supported way to read a text node's content.
func TextValue(el *Element) (string, bool) {
	if el == nil {
		return "", false
	}
	if _, ok := el.Type.(textTag); !ok {
		return "", false
	}
	s, _ := el.Props[textValueKey].(string)
	return s, true
}

// IsText reports whether el is an internal text node.
func IsText(el *Element) bool {
	_, ok := TextValue(el)
	return ok
}

// IsFragment reports whether el is a fragment.
func IsFragment(el *Element) bool {
	if el == nil {
		return false
	}
	_, ok := el.Type.(fragmentTag)
	return ok
}

// HostTag returns the tag name of a host element and reports whether el is one.
func HostTag(el *Element) (string, bool) {
	if el == nil {
		return "", false
	}
	tag, ok := el.Type.(string)
	return tag, ok
}

// NormalizeChildren flattens a raw children value into the flat slice of nodes
// the renderer walks. It drops nil and booleans, splices nested slices in
// place, and leaves everything else untouched — text conversion happens later,
// at reconciliation, so that Children() round-trips the values a caller passed.
//
// The result is always non-nil, so it is safe to range over directly.
func NormalizeChildren(raw any) []Node {
	out := []Node{}
	appendNode(&out, raw)
	return out
}

// appendNode is NormalizeChildren's recursive worker. It is separate so the
// slice is allocated once rather than once per nesting level.
func appendNode(out *[]Node, n Node) {
	switch v := n.(type) {
	case nil:
		return
	case bool:
		// Both true and false render as nothing: `cond && node` yields a bare
		// false in JSX, and Go's lack of undefined makes true the same case.
		return
	case []Node:
		for _, c := range v {
			appendNode(out, c)
		}
		return
	case *Element:
		*out = append(*out, v)
		return
	}

	// Any other slice kind — []*Element, []string, []any from a Map helper — is
	// spliced in like a JSX array child. Reflection is the only way to accept
	// them all without asking callers to convert.
	rv := reflect.ValueOf(n)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		for i := range rv.Len() {
			appendNode(out, rv.Index(i).Interface())
		}
		return
	}

	*out = append(*out, n)
}

// textOf renders a leaf value as the text React would show for it. Strings pass
// through; numbers format the way JavaScript's String() formats them, which for
// floats means the shortest round-tripping representation rather than Go's
// default %v.
func textOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return jsNumberString(t, 64)
	case float32:
		return jsNumberString(float64(t), 32)
	case int:
		return strconv.Itoa(t)
	case int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", t)
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

// nodeToElement converts a normalized node into the *Element the reconciler
// diffs, wrapping leaf values in a text element. It returns nil for nodes that
// render as nothing. A node of an unsupported Go type panics here rather than
// silently rendering as a formatted struct, because that is nearly always a bug
// at the call site.
func nodeToElement(n Node) *Element {
	switch v := n.(type) {
	case nil:
		return nil
	case bool:
		return nil
	case *Element:
		return v
	case string:
		return newTextElement(v, v)
	}

	switch reflect.ValueOf(n).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return newTextElement(textOf(n), n)
	}

	if _, ok := n.(fmt.Stringer); ok {
		return newTextElement(textOf(n), n)
	}

	panic(fmt.Sprintf("react: cannot render value of type %T; "+
		"valid children are nil, bool, string, numbers, *react.Element and slices of those", n))
}
