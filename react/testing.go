package react

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Instance is a read-only handle on one node of a rendered tree — the port's
// equivalent of React's test instance, and the only way code outside this
// package can look at what a [Root] actually rendered. Without it a Root is a
// black box: state goes in, effects fire, and nothing observable comes back out
// until a renderer turns the tree into a string.
//
// An Instance wraps a fiber, so it is a live view rather than a copy: read it
// again after a re-render and it reflects the new props. That is deliberate —
// tests hold an Instance across an update and re-read it. Fibers survive
// re-renders as long as their type and key keep matching (see [reconcileChildren]),
// so a handle stays meaningful across updates to the same component; when the
// node is unmounted, or its whole [Root] is, the handle goes stale and
// [Instance.Valid] reports false.
//
// Every accessor is safe on an invalid or zero Instance: it returns the zero
// value for its type rather than panicking, and the navigation and lookup
// methods return invalid Instances. Chains may therefore be written without
// intermediate checks —
//
//	item := root.Tree().FindByTag("ul").FindByTag("li")
//	if !item.Valid() { t.Fatal("no list item rendered") }
//
// — and a missing "ul" produces an invalid result at the end instead of a panic
// in the middle.
//
// An Instance is a value; copying it copies the handle, not the node. It is not
// safe to use one from a goroutine while another goroutine renders the same
// Root, for the same reason reading any mutable tree during mutation is unsafe.
type Instance struct {
	f *fiber
}

// Tree returns a handle on the root of the rendered tree.
//
// The node it returns is the synthetic container fiber the Root renders into,
// not the element the caller passed: it is a fragment with no type, no key and
// no props of its own, and the mounted element is its first child. Exposing the
// container rather than the top-level element means [Instance.Children] on the
// result is the natural "what did this Root render" list even when the root
// element is a fragment or a component returning several siblings.
func (r *Root) Tree() Instance {
	if r == nil {
		return Instance{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return Instance{f: r.container}
}

// Valid reports whether the handle still points at a live node. It is false for
// the zero Instance, for a node that was removed from the tree by a re-render,
// and for every node of an unmounted [Root].
//
// Check it before trusting a zero value from an accessor: an empty [Instance.Key]
// means "this node has no key" on a valid handle and "there is no node" on an
// invalid one, and only Valid tells the two apart.
func (i Instance) Valid() bool {
	if i.f == nil || i.f.unmounted {
		return false
	}
	// Root.Unmount tears down the container's children but leaves the container
	// itself intact, so the flag on the Root is the authority for the top node.
	if r := i.f.root; r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		return !r.unmounted
	}
	return true
}

// Type returns the node's element type: a tag string for a host element, the
// [Component] value for a component, [Fragment] for a fragment, or whatever type
// was registered with [RegisterElementRenderer]. It returns nil for an invalid
// handle.
//
// Comparing against Fragment works as expected; comparing against a Component
// requires reflect, because Go functions are not comparable — prefer matching on
// [Instance.Tag] or a prop where you can.
func (i Instance) Type() any {
	if !i.Valid() {
		return nil
	}
	return i.f.typ
}

// Tag returns the HTML tag name when this node is a host element. The boolean
// distinguishes a host element from a component, a fragment or a text node, the
// same way [HostTag] does for an [Element].
func (i Instance) Tag() (string, bool) {
	if !i.Valid() {
		return "", false
	}
	tag, ok := i.f.typ.(string)
	return tag, ok
}

// Text returns the content of a text node. Text nodes are created by the
// runtime when it normalizes a string or numeric child, so a component that
// returns 42 produces a node whose Text is "42". The boolean reports whether
// this node is one; use [Instance.TextContent] to collect the text of a whole
// subtree.
func (i Instance) Text() (string, bool) {
	if !i.Valid() {
		return "", false
	}
	if _, ok := i.f.typ.(textTag); !ok {
		return "", false
	}
	s, _ := i.f.props[textValueKey].(string)
	return s, true
}

// Output returns the node a component returned from its most recent render, and
// reports whether this instance is a function component at all.
//
// [Instance.Children] gives the *rendered* children: normalized, flattened, with
// nils and booleans dropped. This gives the value the component actually
// returned, before any of that. A renderer never needs it; a serializer whose
// output format preserves the authored shape — React's own wire format does —
// cannot reconstruct it from the rendered tree.
func (i Instance) Output() (Node, bool) {
	if !i.Valid() {
		return nil, false
	}
	if _, ok := i.f.typ.(Component); !ok {
		return nil, false
	}
	return i.f.rendered, true
}

// TextRaw returns the value this text node was created from — a string, an int,
// a float — and reports whether it is a text node.
//
// [Instance.Text] is what a renderer wants; this is what a serializer wants. A
// format that distinguishes the child 7 from the child "7", as React's own wire
// format does, needs the value before it was formatted for display.
func (i Instance) TextRaw() (any, bool) {
	if !i.Valid() {
		return nil, false
	}
	if _, ok := i.f.typ.(textTag); !ok {
		return nil, false
	}
	return i.f.props[textRawKey], true
}

// Key returns the reconciliation key this node was rendered with, or "" when it
// has none. It is the key that decides whether the node kept its state across
// the last render, which makes it worth asserting on in tests about list
// reordering.
func (i Instance) Key() string {
	if !i.Valid() {
		return ""
	}
	return i.f.key
}

// Props returns the node's properties. The result is a fresh map, so writing to
// it cannot reach the rendered tree — a handle that let a caller mutate live
// props would corrupt the memoization the runtime does against them.
//
// The copy is shallow, as [Props.Clone] is: the values inside are the live ones,
// including the children slice, and mutating those is still forbidden. Props of
// an invalid handle are nil, which is a legal Props that every accessor on it
// handles.
func (i Instance) Props() Props {
	if !i.Valid() {
		return nil
	}
	return i.f.props.Clone()
}

// Children returns the node's children in render order.
//
// These are fiber children, not host children: components and fragments appear
// as nodes of their own, so a <div> containing one component shows the component
// as its child and the component's host output as the grandchild. That is the
// tree the runtime actually keeps, and hiding the intermediate nodes would make
// [Instance.Parent] lie. Use [Instance.FindAllByTag] when only host structure
// matters.
//
// The result is nil for a leaf or an invalid handle; ranging over it is safe
// either way.
func (i Instance) Children() []Instance {
	if !i.Valid() {
		return nil
	}
	var out []Instance
	for c := i.f.child; c != nil; c = c.sibling {
		out = append(out, Instance{f: c})
	}
	return out
}

// Parent returns the node's parent, or an invalid Instance at the top of the
// tree. The parent of the node [Root.Tree] returns is always invalid, since the
// container has nothing above it.
func (i Instance) Parent() Instance {
	if !i.Valid() {
		return Instance{}
	}
	return Instance{f: i.f.parent}
}

// Find returns the first descendant for which pred is true, searching
// depth-first in render order, or an invalid Instance when nothing matches.
//
// The receiver itself is never tested. That is React's rule for find, and it is
// what makes repeated narrowing work: FindByTag("div").FindByTag("div") reaches
// a nested div instead of returning the outer one twice.
func (i Instance) Find(pred func(Instance) bool) Instance {
	var found Instance
	i.eachDescendant(func(c Instance) bool {
		if pred != nil && pred(c) {
			found = c
			return false
		}
		return true
	})
	return found
}

// FindAll returns every descendant for which pred is true, depth-first in render
// order. As with [Instance.Find] the receiver is not tested. The result is nil
// when nothing matches.
func (i Instance) FindAll(pred func(Instance) bool) []Instance {
	var out []Instance
	i.eachDescendant(func(c Instance) bool {
		if pred != nil && pred(c) {
			out = append(out, c)
		}
		return true
	})
	return out
}

// FindByTag returns the first descendant host element with the given tag name.
// It is the query tests reach for most, so it is spelled out rather than left to
// [Instance.Find] with a hand-written predicate.
func (i Instance) FindByTag(tag string) Instance {
	return i.Find(func(c Instance) bool {
		t, ok := c.Tag()
		return ok && t == tag
	})
}

// FindAllByTag returns every descendant host element with the given tag name, in
// render order — the query behind assertions like "this list rendered three
// items".
func (i Instance) FindAllByTag(tag string) []Instance {
	return i.FindAll(func(c Instance) bool {
		t, ok := c.Tag()
		return ok && t == tag
	})
}

// FindByProp returns the first descendant carrying a prop named name whose value
// equals value. Equality is [ObjectsEqual]'s, extended so that comparing
// function-valued props (an onClick handler, say) reports pointer identity
// instead of panicking.
//
// It is how a test addresses a node that has no distinguishing tag: match on the
// id, the class, or a test-only prop the component forwards.
func (i Instance) FindByProp(name string, value any) Instance {
	return i.Find(func(c Instance) bool {
		if !c.f.props.Has(name) {
			return false
		}
		return storeObjectsEqual(c.f.props[name], value)
	})
}

// TextContent returns every text node in the subtree, this node included,
// concatenated in render order and with nothing inserted between them — so a
// heading rendered as {"Hello, ", name, "!"} reads back as "Hello, world!".
//
// It is the assertion to write when a test cares what the user would see rather
// than how the tree is shaped.
func (i Instance) TextContent() string {
	if !i.Valid() {
		return ""
	}
	var b strings.Builder
	walk(i.f, func(f *fiber) {
		if _, ok := f.typ.(textTag); ok {
			s, _ := f.props[textValueKey].(string)
			b.WriteString(s)
		}
	})
	return b.String()
}

// String renders the subtree as a compact S-expression, for test failure
// messages and for printing a tree while debugging. The shapes are:
//
//	(div id="main" (span "hi"))   host element, props sorted by name
//	(li#a "first")                a keyed node, key after the tag
//	"42"                          a text node
//	(<> …)                        a fragment
//	(MyComponent …)               a component, named from its Go function
//	<invalid>                     a handle whose node is gone
//
// The node [Root.Tree] returns prints as its children alone, with no wrapper,
// since the container is an artifact of the runtime and not something the caller
// rendered.
//
// The format is meant to be read, not parsed: it is deliberately lossy about
// prop values, and it may change.
func (i Instance) String() string {
	if !i.Valid() {
		return "<invalid>"
	}
	var b strings.Builder
	// The container has no parent and no meaning of its own; print through it.
	if i.f.parent == nil {
		instanceWriteChildren(&b, i.f)
	} else {
		instanceWrite(&b, i.f)
	}
	return b.String()
}

// eachDescendant visits the subtree below i — excluding i — in render order,
// stopping early when visit returns false. Find and FindAll are both this
// traversal; keeping it in one place is what keeps their ordering identical.
func (i Instance) eachDescendant(visit func(Instance) bool) {
	if !i.Valid() {
		return
	}
	var descend func(f *fiber) bool
	descend = func(f *fiber) bool {
		for c := f.child; c != nil; c = c.sibling {
			if !visit(Instance{f: c}) {
				return false
			}
			if !descend(c) {
				return false
			}
		}
		return true
	}
	descend(i.f)
}

// instanceWrite renders one fiber and its subtree into b. It is [Instance.String]'s
// recursive worker, split out so the container special case stays at the top.
func instanceWrite(b *strings.Builder, f *fiber) {
	if _, isText := f.typ.(textTag); isText {
		s, _ := f.props[textValueKey].(string)
		b.WriteString(strconv.Quote(s))
		return
	}

	b.WriteString("(")
	b.WriteString(instanceTypeName(f.typ))
	if f.key != "" {
		b.WriteString("#" + f.key)
	}
	for _, name := range instancePropNames(f.props) {
		b.WriteString(" " + name + "=" + instanceFormatValue(f.props[name]))
	}
	if f.child != nil {
		b.WriteString(" ")
		instanceWriteChildren(b, f)
	}
	b.WriteString(")")
}

// instanceWriteChildren renders f's children space-separated, with no wrapper.
func instanceWriteChildren(b *strings.Builder, f *fiber) {
	for c := f.child; c != nil; c = c.sibling {
		if c != f.child {
			b.WriteString(" ")
		}
		instanceWrite(b, c)
	}
}

// instanceTypeName names an element type for display: the tag for a host
// element, "<>" for a fragment, the Go function's own name for a component, and
// the Go type for anything registered through [RegisterElementRenderer].
func instanceTypeName(typ any) string {
	switch t := typ.(type) {
	case string:
		return t
	case fragmentTag:
		return "<>"
	case textTag:
		return "#text"
	}

	v := reflect.ValueOf(typ)
	if v.Kind() == reflect.Func {
		if fn := runtime.FuncForPC(v.Pointer()); fn != nil {
			name := fn.Name()
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			return name
		}
	}
	return fmt.Sprintf("%T", typ)
}

// instancePropNames returns the prop names worth printing, sorted so that the
// rendering of a tree is stable enough to compare in a test. Children are left
// out because they are printed as nodes; every other prop is shown, including
// one named "value" — text nodes, which store their content under that name,
// never reach here.
func instancePropNames(p Props) []string {
	if len(p) == 0 {
		return nil
	}
	names := make([]string, 0, len(p))
	for k := range p {
		if k == ChildrenKey {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// instanceFormatValue renders a prop value compactly: strings quoted so an empty
// one is visible, functions reduced to a marker because printing a code pointer
// would make every dump differ between runs, everything else through %v.
func instanceFormatValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(t)
	}
	if reflect.ValueOf(v).Kind() == reflect.Func {
		return "func"
	}
	return fmt.Sprintf("%v", v)
}

// Act runs fn and then settles every tree it touched: the updates fn scheduled
// are rendered, the effects of those renders fire, updates those effects
// scheduled are rendered in turn, and so on until nothing is left pending. It is
// React's act() test helper, and the reason a test can assert on a tree
// immediately after triggering something instead of guessing when the runtime
// caught up.
//
// Because updates are held until fn returns, a sequence of them costs one render
// rather than one each — Act is [Batch] with a name that says what it is for.
// Two consequences follow from that and are worth knowing:
//
//   - The tree does not change while fn is running. Read it after Act returns,
//     not inside fn.
//   - A [Root] created inside fn does not render until Act returns, so hold the
//     assertions about it until then too.
//
// Act settles synchronous work only. It cannot know about a goroutine fn started
// that has not finished; wait for that goroutine — with a channel, never a sleep
// — and then call Act, or call it inside Act before returning.
func Act(fn func()) {
	if fn == nil {
		return
	}
	Batch(fn)
}
