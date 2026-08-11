package react

// This file is the element-construction layer: the Go stand-in for JSX. In
// React, `<div id="x">hi</div>` is sugar the compiler rewrites into
// `React.createElement("div", {id: "x"}, "hi")`. Go has no JSX and no compiler
// hook to add one, so the rewritten form *is* the authoring surface here, and
// the helpers below exist to make writing it by hand tolerable: [H] for host
// elements, [Frag] for fragments, [CreateElement] for the general case.
//
// Everything here produces plain, immutable [Element] values. Nothing in this
// file touches a fiber, schedules work, or reads runtime state — building an
// element is pure data construction and is safe from any goroutine, before or
// after a render.

// CreateElement builds an [Element], mirroring React.createElement.
//
// Two props are reserved and never reach a component, exactly as in React:
//
//   - [KeyProp] ("key") is lifted into [Element.Key]. JSX keys may be numbers,
//     so the value is formatted with the same rules a text child uses — an int
//     key, a float key or a [fmt.Stringer] key all work, and produce the string
//     React would have produced. A nil key is treated as absent.
//   - [RefProp] ("ref") is lifted into [Element.Ref].
//
// Both are removed from the resulting [Element.Props]. React 19 passes refs to
// function components as an ordinary prop, but this port keeps the field so host
// elements have somewhere to put one; read it from [Element.Ref].
//
// Variadic children follow React's rule: when at least one is supplied they
// replace whatever props[ChildrenKey] held, and when none are supplied the props
// entry is left alone. That is what makes both authoring styles work —
//
//	CreateElement("ul", nil, itemA, itemB)
//	CreateElement("ul", Props{ChildrenKey: items})
//
// — and it is why passing a single explicit nil child is meaningful: it is one
// child, so it clears the props entry. Children are stored raw; nils, booleans
// and nested slices are resolved later by [NormalizeChildren], so
// [Props.Children] round-trips what the caller wrote.
//
// The caller's props map is never mutated or retained: the result always holds a
// fresh, non-nil map, so passing the same Props value to several CreateElement
// calls is safe. props itself may be nil.
func CreateElement(typ any, props Props, children ...Node) *Element {
	el := &Element{Type: typ, Props: make(Props, len(props)+1)}

	for k, v := range props {
		switch k {
		case KeyProp:
			el.Key = elementKeyString(v)
		case RefProp:
			el.Ref = v
		default:
			el.Props[k] = v
		}
	}

	if len(children) > 0 {
		el.Props[ChildrenKey] = children
	}
	return el
}

// elementKeyString renders a reserved "key" prop as the string stored in
// [Element.Key]. It reuses the text-child formatting rules so that a numeric key
// — overwhelmingly the common case, since keys usually come from an ID or an
// index — stringifies the way JavaScript would, rather than through Go's %v.
//
// A nil key means "no key": React treats key={null} as unkeyed, and an empty
// [Element.Key] is this port's "match by position" marker.
func elementKeyString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return textOf(v)
}

// H builds a host element — the workhorse for writing trees by hand. It is
// nothing but [CreateElement] with the type fixed to a tag string; it exists
// because without JSX every line of a Go tree is a constructor call, and
// `H("div", nil, ...)` reads roughly like the markup it replaces where
// `CreateElement("div", nil, ...)` does not.
//
//	H("ul", Props{"class": "menu"},
//	    H("li", Props{KeyProp: 1}, "first"),
//	    H("li", Props{KeyProp: 2}, "second"),
//	)
func H(tag string, props Props, children ...Node) *Element {
	return CreateElement(tag, props, children...)
}

// Frag builds a [Fragment] element: several siblings grouped with no wrapper in
// the output, the equivalent of JSX's <>…</>. A Go component returns a single
// [Node], so this is how one returns a list.
//
// A fragment takes no props other than its children. Use [CreateElement] with
// [Fragment] directly if a keyed fragment is needed inside a list.
func Frag(children ...Node) *Element {
	return CreateElement(Fragment, nil, children...)
}

// CloneElement copies el with props shallow-merged over the original's,
// mirroring React.cloneElement. It is the mechanism behind "wrapper components
// that inject a prop": a component receives children it did not build and hands
// them back with one more prop set.
//
// The merge rules match React's:
//
//   - props overrides matching keys and adds new ones; every other original prop
//     survives. The merge is shallow — a nested map in a prop value is shared,
//     not copied.
//   - [Element.Key] and [Element.Ref] are inherited from el unless props carries
//     a "key" or "ref" of its own, in which case the new value wins.
//   - children are inherited unless at least one variadic child is supplied, in
//     which case they replace the original's entirely.
//
// Neither el nor the caller's props map is mutated; the result is a wholly new
// element with its own props map. Cloning a nil element panics, because there is
// no meaningful element to return and silently producing nil would surface as a
// missing subtree far from the mistake.
func CloneElement(el *Element, props Props, children ...Node) *Element {
	if el == nil {
		panic("react: CloneElement called with a nil element")
	}

	out := &Element{
		Type:  el.Type,
		Key:   el.Key,
		Ref:   el.Ref,
		Props: make(Props, len(el.Props)+len(props)),
	}
	for k, v := range el.Props {
		out.Props[k] = v
	}

	for k, v := range props {
		switch k {
		case KeyProp:
			out.Key = elementKeyString(v)
		case RefProp:
			out.Ref = v
		default:
			out.Props[k] = v
		}
	}

	if len(children) > 0 {
		out.Props[ChildrenKey] = children
	}
	return out
}

// IsValidElement reports whether n is an element rather than a text leaf, a nil
// or a slice. It is React.isValidElement, and it is what a component should call
// before reaching into a child's Props — [Props.Children] hands back the raw
// [Node] union, and only some members of that union are elements.
//
// Internal text elements count as valid, matching React, where a normalized text
// node is still an element; use [IsText] to tell the two apart.
func IsValidElement(n Node) bool {
	el, ok := n.(*Element)
	return ok && el != nil
}
