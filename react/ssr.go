package react

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Server rendering is the port's primary output path. React's renderer talks to
// a DOM; Go has no DOM, so a tree only becomes something you can look at by
// being serialized to HTML. Everything in this file is built on one idea: mount
// the tree with [NewRoot], walk the fibers it produced, and emit markup for the
// host and text fibers while treating every other fiber kind — components,
// fragments, providers, memo wrappers — as transparent.

// RenderToWriter renders node to HTML and writes it to w. It is the primitive
// [RenderToString] and [RenderToStaticMarkup] are built on, and the one to
// reach for when the destination is an http.ResponseWriter: markup is written
// as it is produced rather than accumulated, so a large page starts reaching
// the client before the whole tree has been serialized.
//
// The tree is mounted, walked, and then unmounted before the function returns,
// so effect cleanups run exactly as they would for any other Root lifecycle. A
// caller who wants the tree to stay alive should build a [Root] themselves.
//
// Errors, and why this returns one at all: a caller needs to tell "your tree is
// wrong" from "the socket died". Both arrive as an error here, but they are
// distinguishable — an I/O failure is the error w returned, unwrapped and
// unwrapped-into, while a tree problem is a react-prefixed error describing the
// element at fault. The tree problems are:
//
//   - a void element ("br", "img", …) given children or raw HTML;
//   - an element with both children and "dangerouslySetInnerHTML";
//   - a "dangerouslySetInnerHTML" prop that does not hold a [DangerousHTML];
//   - a style prop, or a style value, of an unsupported type ([FormatStyle]).
//
// A panic anywhere in rendering — an unrenderable child type, a panicking
// component, a hook-order violation — is recovered and returned as an error
// naming the panic value, because a template rendering deep inside a request
// handler should fail that request rather than the process.
func RenderToWriter(w io.Writer, node Node) (err error) {
	// Registered last, so it runs after the Unmount below: cleanups get to run
	// even when the render panicked, and a panic raised *by* a cleanup is
	// caught here too.
	defer func() {
		if r := recover(); r != nil {
			err = ssrPanicError(r)
		}
	}()

	root := NewRoot(node)
	defer root.Unmount()

	e := &ssrEmitter{w: w}
	e.children(root.tree())
	return e.err
}

// renderSeparatingPortals renders node once, emitting everything except portal
// subtrees into the returned markup and collecting each portal's output under
// its container id.
//
// One render, one tree, one set of hooks: the separation happens as the bytes
// are produced, not by scanning them afterwards. That distinction is the whole
// reason this exists — the emitter knows which output came from which fiber,
// where a text scan can only pattern-match, and raw [DangerousHTML] content can
// contain anything at all including something that looks exactly like a portal
// boundary.
//
// It backs [RenderPortalsToString]; see that function and portal.go for what a
// portal means in a runtime with no DOM.
func renderSeparatingPortals(node Node) (main string, portals map[string]string, err error) {
	defer func() {
		if r := recover(); r != nil {
			main, portals, err = "", nil, ssrPanicError(r)
		}
	}()

	root := NewRoot(node)
	defer root.Unmount()

	var b strings.Builder
	e := &ssrEmitter{w: &b, portals: map[string]*strings.Builder{}}
	e.children(root.tree())
	if e.err != nil {
		return "", nil, e.err
	}

	portals = make(map[string]string, len(e.portals))
	for id, buf := range e.portals {
		portals[id] = buf.String()
	}
	return b.String(), portals, nil
}

// RenderToString renders node to a complete HTML string.
//
// In React, renderToString and renderToStaticMarkup differ by hydration
// metadata: renderToString embeds the comment markers and attributes that let
// hydrateRoot match the client tree to the server output. This port has no
// client, no hydration and therefore no markers, and inventing markers nothing
// consumes would be worse than useless — it would put noise in the output and
// imply a capability that does not exist. So the two functions produce byte-
// identical output today, and [RenderToStaticMarkup] is documented as the alias.
// The pair is kept because the names carry intent: code that would hydrate uses
// RenderToString, and if hydration markers ever land they land here, not in
// RenderToStaticMarkup.
func RenderToString(node Node) (string, error) {
	var b strings.Builder
	if err := RenderToWriter(&b, node); err != nil {
		return "", err
	}
	return b.String(), nil
}

// RenderToStaticMarkup renders node to HTML that is never going to be
// hydrated — an email body, a static site page, a PDF source document.
//
// It is currently an exact alias for [RenderToString]: see that function for
// why, in a port with no client runtime, the two cannot honestly differ. Prefer
// this name when the output is genuinely final, so the intent survives if the
// two ever diverge.
func RenderToStaticMarkup(node Node) (string, error) {
	return RenderToString(node)
}

// MustRenderToString is [RenderToString] with the error turned into a panic. It
// exists for tests, examples and package-level initialization, where an error
// return is pure noise because there is no caller who could handle it. Never
// use it on a request path.
func MustRenderToString(node Node) string {
	s, err := RenderToString(node)
	if err != nil {
		panic(err)
	}
	return s
}

// ssrPanicError converts a recovered panic value into an error with enough
// context to find the failure. An error panic is wrapped so errors.Is/As still
// see through it; anything else is formatted.
func ssrPanicError(r any) error {
	if e, ok := r.(error); ok {
		return fmt.Errorf("react: panic while rendering to HTML: %w", e)
	}
	return fmt.Errorf("react: panic while rendering to HTML: %v", r)
}

// ssrEmitter carries the destination and the first error seen down the
// traversal. Threading an error return through a recursive walk would obscure
// the shape of the walk itself; sticking the error on the emitter and making
// every write a no-op once it is set keeps the traversal readable and still
// stops at the first failure.
type ssrEmitter struct {
	w   io.Writer
	err error

	// portals, when non-nil, diverts each portal subtree's output into a buffer
	// named by its container instead of emitting it inline.
	//
	// Separating portals during emission rather than lifting them out of the
	// finished markup afterwards is what makes the separation exact: the
	// emitter knows which bytes came from which fiber, where a later text scan
	// can only guess. The case that breaks a text scan is real — a
	// [DangerousHTML] value containing the literal portal tag is
	// indistinguishable from a boundary once it is bytes.
	portals map[string]*strings.Builder

	// ns is the namespace — HTML, SVG or MathML — that the fiber currently
	// being emitted lives in. It is a plain field rather than a parameter
	// because it is inherited by a whole subtree: host saves it, sets it for the
	// element and again for the element's children, and restores it on the way
	// out, which is what makes <foreignObject> hand control back to HTML and
	// give it up again at its closing tag. See ssr_namespace.go.
	ns Namespace
}

// portalBuffer returns the buffer collecting output for a container, creating
// it on first use so that a container with no portal is absent rather than
// present and empty.
func (e *ssrEmitter) portalBuffer(id string) *strings.Builder {
	if b, ok := e.portals[id]; ok {
		return b
	}
	b := &strings.Builder{}
	e.portals[id] = b
	return b
}

// portal emits a portal's children into its container's buffer.
//
// The wrapper element itself is not emitted at all: it exists to mark the
// subtree in the fiber tree, not to appear in the output. Nested portals fall
// out for free, because the recursive walk re-enters this method and swaps the
// destination again.
func (e *ssrEmitter) portal(f *fiber, id string) {
	previous := e.w
	e.w = e.portalBuffer(id)
	e.children(f)
	e.w = previous
}

// fail records the first error and makes every later write a no-op.
func (e *ssrEmitter) fail(err error) {
	if e.err == nil {
		e.err = err
	}
}

// write emits a literal chunk of markup. The string must already be escaped;
// callers pass tag names, punctuation and pre-escaped values.
func (e *ssrEmitter) write(s string) {
	if e.err != nil || s == "" {
		return
	}
	if _, err := io.WriteString(e.w, s); err != nil {
		e.fail(err)
	}
}

// children emits every fiber below f, in render order.
func (e *ssrEmitter) children(f *fiber) {
	for c := f.child; c != nil; c = c.sibling {
		e.fiber(c)
	}
}

// fiber emits one fiber. Only two fiber kinds produce output of their own:
// host fibers, whose typ is a tag string, and text fibers. Every other kind —
// [Fragment], a [Component], and anything registered through
// [RegisterElementRenderer] such as a context provider or a memo boundary — is
// transparent, contributing nothing but its children. That is exactly the
// browser's view of a React tree, and it means new element types need no change
// here.
func (e *ssrEmitter) fiber(f *fiber) {
	if e.err != nil {
		return
	}
	// A portal is a host element with a reserved tag, so this test has to come
	// before the host case that would otherwise emit it inline.
	if e.portals != nil {
		if id, isPortal := portalContainerOfFiber(f); isPortal {
			if id == "" {
				// The reserved tag used as an ordinary host element. Reporting
				// it is worth more than rendering it: the output would look
				// right and the portal would silently never be retrievable.
				e.fail(fmt.Errorf("react: <%s> element with no %s attribute; "+
					"the %s tag is reserved for CreatePortal and must not be used as an "+
					"ordinary host element", PortalTag, PortalContainerProp, PortalTag))
				return
			}
			e.portal(f, id)
			return
		}
	}

	switch t := f.typ.(type) {
	case string:
		e.host(f, t)
	case textTag:
		s, _ := f.props[textValueKey].(string)
		e.write(ssrEscapeHTML(s))
	default:
		e.children(f)
	}
}

// host emits a host element: its open tag with attributes, its content, and —
// unless it is a void element — its closing tag.
func (e *ssrEmitter) host(f *fiber, tag string) {
	// The element's own namespace governs its attributes; its children may be in
	// a different one (foreignObject). Restoring on the way out is what makes
	// leaving an <svg> or a <foreignObject> put the enclosing grammar back.
	outerNS := e.ns
	e.ns = ElementNamespace(outerNS, tag)
	defer func() { e.ns = outerNS }()

	raw, hasRaw, err := ssrDangerousContent(f.props)
	if err != nil {
		e.fail(fmt.Errorf("<%s>: %w", tag, err))
		return
	}
	hasChildren := f.child != nil

	// Both content sources at once is always a mistake: React throws here, and
	// silently preferring one would make the dropped half invisible.
	if hasRaw && hasChildren {
		e.fail(fmt.Errorf("react: <%s> has both children and %s; "+
			"an element may set its content one way or the other, not both",
			tag, ssrDangerousProp))
		return
	}

	void := ssrIsVoidElement(tag)
	if void && (hasChildren || hasRaw) {
		e.fail(fmt.Errorf("react: <%s> is a void element and cannot have content; "+
			"void elements (area, base, br, col, embed, hr, img, input, link, meta, "+
			"source, track, wbr) have no closing tag", tag))
		return
	}

	e.write("<" + tag)
	e.attributes(f, tag)
	if e.err != nil {
		return
	}

	if void {
		// The trailing slash is not required by HTML5, but React emits it and it
		// keeps the output parseable as XHTML/XML, which matters for the email
		// and feed use cases this renderer serves.
		e.write("/>")
		return
	}

	e.write(">")
	// Content, not the element itself: an integration point's children are HTML
	// even though the element carrying them is SVG or MathML.
	e.ns = ContentNamespace(e.ns, tag)
	if hasRaw {
		// Deliberately unescaped: that is the entire contract of DangerousHTML.
		e.write(raw)
	} else {
		e.children(f)
	}
	e.write("</" + tag + ">")
}

// attributes emits the element's props as HTML attributes.
//
// Props are emitted in sorted order. React preserves the JavaScript object's
// insertion order, which Go maps simply do not have; sorting is the only choice
// that makes the same tree render to the same bytes twice, and byte stability
// is worth far more here than imitating an order the source language chose
// arbitrarily.
func (e *ssrEmitter) attributes(f *fiber, tag string) {
	if len(f.props) == 0 {
		return
	}
	names := make([]string, 0, len(f.props))
	for k := range f.props {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, prop := range names {
		attr, ok := ssrResolveAttributeName(prop, e.ns)
		if !ok {
			continue
		}
		value, mode, err := ssrAttrValue(prop, attr, f.props[prop])
		if err != nil {
			e.fail(fmt.Errorf("<%s>: %w", tag, err))
			return
		}
		switch mode {
		case ssrAttrOmit:
			continue
		case ssrAttrBare:
			// A flag attribute carries no information in its value, but React
			// still writes an empty one, and the parity oracle compares bytes.
			// See ssrAttrBare.
			e.write(" " + attr + `=""`)
		case ssrAttrValued:
			e.write(" " + attr + `="` + ssrEscapeHTML(value) + `"`)
		}
	}
}
