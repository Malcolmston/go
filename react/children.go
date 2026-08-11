package react

import (
	"fmt"
	"strconv"
)

// This file is React.Children — the small set of utilities for a component that
// receives an opaque `children` prop and needs to inspect, count or transform it
// without knowing whether the caller passed one child, ten, a slice, a nil, or a
// `cond && <x/>` that evaluated to false.
//
// In JavaScript these live on a namespace object (React.Children.map). Go has no
// nested namespaces, so they are package-level functions with a Children prefix:
// [ChildrenToSlice], [ChildrenCount], [ChildrenForEach], [ChildrenMap],
// [ChildrenOnly]. Each takes the raw [Node] a component read out of its props,
// not an already-normalized slice, because the whole reason these exist is that
// the raw value has no single shape.
//
// Every one of them normalizes first with [NormalizeChildren], so nil and
// boolean children are dropped and nested slices are spliced in place before any
// counting or indexing happens. That means the index a callback receives is the
// index in the *flattened, filtered* list — the same index React reports, and
// the one that makes generated keys line up with rendered positions.

// childrenKeyPrefix is the "." that marks a key as runtime-generated rather than
// author-supplied. React reserves the same character for exactly this purpose:
// user keys in practice never start with a dot, so prefixing generated keys puts
// them in a namespace that cannot collide with a key someone wrote by hand. It
// matters because a generated key and a real key end up in the same sibling
// namespace during reconciliation, and a collision there silently swaps two
// components' state.
const childrenKeyPrefix = "."

// childrenKeySeparator joins a source key to a key the mapper produced, so that
// one source child mapped to several outputs yields several distinct keys that
// all still trace back to the same input.
const childrenKeySeparator = "/"

// childrenSourceKey returns the reconciliation identity of the child at index i
// of a normalized children list. It is the anchor every generated key in this
// file is built from.
//
// A child that already carries a key keeps it verbatim: the author chose that
// identity deliberately, and rewriting it would break the "same key, same state"
// contract across an update. Everything else — keyless elements, and text or
// numeric leaves, which have nowhere to put a key at all — is identified by its
// position, rendered as [childrenKeyPrefix] plus the index: ".0", ".1", ".2".
//
// The scheme is stable in the only sense that matters: for a given children
// list, the same child gets the same key on every call, and a list rebuilt with
// the same shape produces the same keys. It is deliberately *not* stable across
// an insertion at the front of a keyless list — nothing can be, because a
// keyless list has no identity to track, which is precisely why React warns
// about missing keys.
func childrenSourceKey(child Node, i int) string {
	if el, ok := child.(*Element); ok && el != nil && el.Key != "" {
		return el.Key
	}
	return childrenKeyPrefix + strconv.Itoa(i)
}

// ChildrenToSlice flattens children into a slice with generated keys applied,
// mirroring React.Children.toArray. It is what a component calls when it must
// treat its children as an ordered, indexable collection — a tab bar reading
// child titles, a carousel slicing out one child at a time.
//
// Beyond what [NormalizeChildren] does (drop nil and booleans, splice nested
// slices), it assigns a key to every keyless element child, using the scheme
// documented on childrenSourceKey: keyless elements become ".0", ".1", … by
// their position in the flattened list, while already-keyed elements are
// returned untouched. React does this for the same reason: once children have
// been pulled apart and re-emitted somewhere else in the tree, position is no
// longer enough to reconcile them, so the array must carry explicit identity.
//
// Non-element children (strings, numbers) pass through unchanged — there is
// nowhere to hang a key on a bare value, and the reconciler matches text leaves
// by position anyway.
//
// Elements are never mutated. A keyless child is returned as a shallow copy with
// its Key filled in, so the caller's original element — which may be shared with
// another part of the tree, or being rendered right now — is untouched. The
// result is always non-nil.
func ChildrenToSlice(children Node) []Node {
	nodes := NormalizeChildren(children)
	out := make([]Node, 0, len(nodes))
	for i, n := range nodes {
		el, ok := n.(*Element)
		if !ok || el == nil || el.Key != "" {
			out = append(out, n)
			continue
		}
		out = append(out, childrenWithKey(el, childrenSourceKey(n, i)))
	}
	return out
}

// childrenWithKey returns a shallow copy of el carrying the given key. Elements
// are immutable by contract — the reconciler holds on to the props map of the
// element it last rendered — so re-keying must copy rather than assign, even
// though assigning would be cheaper and would usually appear to work.
//
// The props map itself is shared, not cloned: it is read-only for everyone, so
// copying it would allocate on every mapped child for no benefit.
func childrenWithKey(el *Element, key string) *Element {
	clone := *el
	clone.Key = key
	return &clone
}

// ChildrenCount reports how many children there are after normalization,
// mirroring React.Children.count. Nils, booleans and empty nested slices do not
// count; a nested slice contributes its flattened length, not one.
//
// This is the number a component should test against when its layout depends on
// child count ("render a divider only when there is more than one child"),
// because it counts what will actually render rather than what was passed.
func ChildrenCount(children Node) int {
	return len(NormalizeChildren(children))
}

// ChildrenForEach calls fn once per child, in render order, with the child's
// index in the flattened list. It is React.Children.forEach.
//
// The Go signature puts the index second, matching Go's range idiom rather than
// JavaScript's (child, index) — which happens to be the same order, but for the
// opposite reason. fn's return value is discarded; use [ChildrenMap] to build a
// new list.
func ChildrenForEach(children Node, fn func(child Node, i int)) {
	if fn == nil {
		return
	}
	for i, n := range NormalizeChildren(children) {
		fn(n, i)
	}
}

// ChildrenMap transforms each child and returns the results, mirroring
// React.Children.map. The classic use is a layout component wrapping each child:
//
//	ChildrenMap(props[ChildrenKey], func(c Node, i int) Node {
//	    return H("li", nil, c)
//	})
//
// Key handling is the subtle part, and the reason this is not simply a for loop.
// Wrapping a keyed child in a new element would normally throw the original
// key away, so a reordered list would reconcile by position and every item's
// state would land on the wrong row. To prevent that, an element returned by fn
// is re-keyed from the child that produced it:
//
//   - a result with no key of its own inherits the source key — the original
//     child's key when it had one, otherwise the positional ".N" form. This is
//     the wrapping case, and it is what carries identity through the wrapper.
//   - a result that carries a key of its own gets that key prefixed with the
//     source key and a "/" separator ("a/label"), so that one source child
//     expanded into several outputs yields several distinct, non-colliding keys
//     that all still track the source. The identity case is excluded: a mapper
//     that returns the child unchanged already carries the source key, and
//     prefixing it would turn "a" into "a/a" and remount the item.
//
// Results that are not elements (a string, a nil the mapper chose to return) are
// passed through untouched, and nils are *not* dropped here — the result slice
// is index-aligned with the input, and it is [NormalizeChildren] at render time
// that removes them. Elements are never mutated; re-keying copies.
//
// The result is always non-nil. fn must not be nil.
func ChildrenMap(children Node, fn func(child Node, i int) Node) []Node {
	nodes := NormalizeChildren(children)
	out := make([]Node, 0, len(nodes))
	if fn == nil {
		return out
	}

	for i, n := range nodes {
		res := fn(n, i)
		src := childrenSourceKey(n, i)

		el, ok := res.(*Element)
		if !ok || el == nil {
			out = append(out, res)
			continue
		}
		if el.Key == "" || el.Key == src {
			out = append(out, childrenWithKey(el, src))
			continue
		}
		out = append(out, childrenWithKey(el, src+childrenKeySeparator+el.Key))
	}
	return out
}

// ChildrenOnly asserts that children is exactly one element and returns it,
// mirroring React.Children.only. Components that must control their single child
// directly — a transition wrapper that clones it with extra props, a slot that
// forwards a ref onto it — use this to reject any other shape up front.
//
// React throws; Go returns an error, because a malformed children prop is a
// caller mistake that a library component can reasonably report rather than a
// runtime invariant violation. Two cases are errors: the flattened list does not
// hold exactly one node, and the one node it holds is not an element (a bare
// string child has no props to clone and no ref to forward).
func ChildrenOnly(children Node) (*Element, error) {
	nodes := NormalizeChildren(children)
	if len(nodes) != 1 {
		return nil, fmt.Errorf("react: ChildrenOnly expected exactly one child, got %d", len(nodes))
	}
	el, ok := nodes[0].(*Element)
	if !ok || el == nil {
		return nil, fmt.Errorf("react: ChildrenOnly expected the single child to be an element, got %T", nodes[0])
	}
	return el, nil
}
