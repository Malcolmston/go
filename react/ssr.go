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
func RenderToWriter(w io.Writer, node Node) error {
	return ssrRenderToWriter(w, node, false)
}

// ssrRenderToWriter is [RenderToWriter] with the choice of text mode made
// explicit: staticMarkup selects React's renderToStaticMarkup text rules rather
// than renderToString's. The only difference is the hydration separator between
// adjacent text nodes; see ssr_textsep.go.
func ssrRenderToWriter(w io.Writer, node Node, staticMarkup bool) (err error) {
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

	e := &ssrEmitter{w: w, staticMarkup: staticMarkup}
	// Head-managed elements do not render where they were written, so the walk
	// goes through the hoisting path; it streams straight to w for any tree that
	// cannot hoist. See ssr_hoist.go.
	return ssrHoistRenderToWriter(e, w, root.tree())
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

	// staticMarkup: splitting a document across containers produces markup no
	// client can hydrate as one tree, so the hydration text separator would be
	// noise here. See [RenderPortalsToString] and ssr_textsep.go.
	e := &ssrEmitter{portals: map[string]*strings.Builder{}, staticMarkup: true}
	// The main output goes through the hoisting path, which owns the buffer it
	// is assembled in; a container's output is its own stream and is not
	// rearranged. See ssr_hoist.go.
	main, err = ssrHoistRender(e, root.tree())
	if err != nil {
		return "", nil, err
	}

	portals = make(map[string]string, len(e.portals))
	for id, buf := range e.portals {
		portals[id] = buf.String()
	}
	return main, portals, nil
}

// RenderToString renders node to a complete HTML string.
//
// In React, renderToString and renderToStaticMarkup differ by hydration
// metadata, and the difference that reaches the bytes of a server-rendered
// document is the text separator: renderToString writes a "<!-- -->" comment
// between two adjacent text nodes so that the client can tell where one ends and
// the next begins, while renderToStaticMarkup, whose output is never hydrated,
// writes the two runs of text straight up against each other. This port
// reproduces that difference exactly — see ssr_textsep.go — so
// RenderToString(div "a" "b") is "<div>a<!-- -->b</div>" and
// [RenderToStaticMarkup] of the same tree is "<div>ab</div>".
//
// Reach for this name when the markup is going to a client that may hydrate it,
// and for RenderToStaticMarkup when the output is final.
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
// It differs from [RenderToString] exactly as React's renderToStaticMarkup
// differs from its renderToString: no "<!-- -->" separator is written between
// adjacent text nodes, because nothing will ever need to find the boundary. See
// [RenderToString] and ssr_textsep.go.
func RenderToStaticMarkup(node Node) (string, error) {
	var b strings.Builder
	if err := ssrRenderToWriter(&b, node, true); err != nil {
		return "", err
	}
	return b.String(), nil
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

	// hoist, when non-nil, is the Float buffer that head-managed elements are
	// diverted into instead of being written where they appear. It also holds
	// the document buffer they are eventually spliced back into. See
	// ssr_hoist.go.
	hoist *ssrHoister

	// preloads collects the hoisted <link rel="preload" as="image"> chunks the
	// img elements in this render contributed, in React's two flush buckets.
	// Nothing here emits them: the hoistable buffer flushes highChunks ahead of
	// the other hoistables and bulkChunks after them, both still ahead of the
	// document body. See ssr_preload.go.
	preloads ssrImagePreloads

	// noPreload is set while emitting the subtree of a <picture> or <noscript>,
	// where React suppresses img preloads entirely.
	noPreload bool

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

	// staticMarkup selects React's renderToStaticMarkup text rules over
	// renderToString's, and lastText records whether the bytes written most
	// recently were a text node. Together they decide the "<!-- -->" hydration
	// separator between adjacent text nodes; both belong to ssr_textsep.go,
	// which is where every rule that reads them lives.
	staticMarkup bool
	lastText     bool
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
	previous, previousText := e.w, e.lastText
	// A container's buffer is its own document order, so the run of text in the
	// stream being interrupted must not leak into it or back out of it.
	e.w, e.lastText = e.portalBuffer(id), false
	e.children(f)
	e.lastText = previousText
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
		// Not a plain write: adjacent text nodes are separated for hydration
		// unless this is static markup. See ssr_textsep.go.
		e.text(s)
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

	// React's Float machinery renders a <meta>, a <title>, a hoistable <link>
	// and an async <script src> at the front of the document rather than here.
	// The classifier, the buffer and the assembly all live in ssr_hoist.go; this
	// is the one place the walk has to ask.
	if e.hoistIntercept(f, tag) {
		return
	}

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

	// React's Float machinery hoists a <link rel="preload" as="image"> for an
	// <img src>. Collecting it here, before the img is written, is the only
	// wiring this file needs: the rules and the markup live in ssr_preload.go,
	// and the buckets are handed to the hoistable buffer to flush.
	if tag == "img" {
		if preload, ok := ssrImagePreloadFor(f.props, e.noPreload); ok {
			e.preloads.add(preload)
		}
	}
	// picture and noscript suppress preloads for their whole subtree, so the
	// flag is set for the children and restored on the way out.
	if ssrPreloadSuppressingTag(tag) {
		outerNoPreload := e.noPreload
		e.noPreload = true
		defer func() { e.noPreload = outerNoPreload }()
	}

	void := ssrIsVoidElement(tag)
	if void && (hasChildren || hasRaw) {
		e.fail(fmt.Errorf("react: <%s> is a void element and cannot have content; "+
			"void elements (area, base, br, col, embed, hr, img, input, link, meta, "+
			"source, track, wbr) have no closing tag", tag))
		return
	}

	// Controlled form values: a <select>'s and a <textarea>'s value props are
	// subtree context and text content rather than attributes, and an
	// <option>'s selected is decided by the <select> above it. See
	// ssr_control.go; the rewritten props feed the attribute pass below.
	ctlText, ctlHasText, ctlErr := ssrControlContent(f, tag, hasRaw)
	if ctlErr != nil {
		e.fail(ctlErr)
		return
	}

	e.write("<" + tag)
	e.attributes(ssrControlFiber(f, tag), tag)
	if e.err != nil {
		return
	}

	if void {
		// The trailing slash is not required by HTML5, but React emits it and it
		// keeps the output parseable as XHTML/XML, which matters for the email
		// and feed use cases this renderer serves.
		e.write("/>")
		// A tag ends the run of text, so the next text node needs no separator.
		e.lastText = false
		return
	}

	e.write(">")
	e.lastText = false
	// Content, not the element itself: an integration point's children are HTML
	// even though the element carrying them is SVG or MathML.
	e.ns = ContentNamespace(e.ns, tag)
	if ctlHasText {
		// A <textarea>'s content is its value prop, already escaped by the
		// helper that derived it. See ssr_control.go.
		e.write(ctlText)
	} else if hasRaw {
		// Deliberately unescaped: that is the entire contract of DangerousHTML.
		e.write(raw)
	} else if text, ok := ssrRawTextChildren(f, tag); ok {
		// script and style hold raw text, not markup: entity-escaping their
		// content would corrupt the program or the stylesheet. See ssr_rawtext.go.
		e.write(ssrRawTextContent(tag, text))
	} else {
		e.children(f)
	}
	e.write("</" + tag + ">")
	e.lastText = false
}

// attributes emits the element's props as HTML attributes.
//
// Props are emitted in sorted order. React preserves the JavaScript object's
// insertion order, which Go maps simply do not have; sorting is the only choice
// that makes the same tree render to the same bytes twice, and byte stability
// is worth far more here than imitating an order the source language chose
// arbitrarily.
//
// Sorting is not the whole story for input, button, form and option: React's
// hand-written serializers for those tags defer a fixed set of props to the end
// of the start tag, and that is visible in the bytes, so ssrAttributeOrder
// reorders the sorted names to match. See ssr_order.go.
func (e *ssrEmitter) attributes(f *fiber, tag string) {
	if len(f.props) == 0 {
		return
	}
	names := make([]string, 0, len(f.props))
	for k := range f.props {
		names = append(names, k)
	}
	sort.Strings(names)
	names = ssrAttributeOrder(tag, f.props, names)

	for _, prop := range names {
		attr, ok := ssrResolveAttributeName(prop, e.ns)
		if !ok {
			continue
		}
		var (
			value string
			mode  ssrAttrMode
			err   error
		)
		if prop == ssrStyleProp {
			// The style prop has its own React-exact serializer; see ssr_style.go.
			value, mode, err = ssrStyleAttrValue(f.props[prop])
		} else {
			value, mode, err = ssrAttrValue(prop, attr, f.props[prop])
		}
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
			// A javascript: URL never reaches the page; see ssr_url.go.
			if sanitized, blocked := ssrSanitizeURLValue(prop, value); blocked {
				value = sanitized
			}
			e.write(" " + attr + `="` + ssrEscapeText(value) + `"`)
		}
	}
}
