package react

import "strings"

// SVG and MathML in server rendering.
//
// An HTML document is not one language. Three grammars share the same byte
// stream, and which one you are in is decided by the elements you have walked
// through to get here: `<svg>` opens SVG, `<math>` opens MathML, and inside
// either of those an "integration point" — `<foreignObject>` most famously —
// hands control back to HTML for its subtree. This file models that stack, and
// carries the attribute-name knowledge that only shows up in the foreign
// grammars.
//
// # How this table was derived
//
// Not from memory, and not from the SVG spec. Every entry below was produced by
// rendering a probe element per candidate prop through the real
// react-dom@19.2.8 renderToStaticMarkup and recording the attribute name that
// came out, in five nesting contexts: a bare <div>, the <svg> root, a <path>
// inside <svg>, a <div> inside <foreignObject> inside <svg>, and an <mi> inside
// <math>. The candidate list was itself extracted mechanically from react-dom's
// own bundle rather than written by hand, and the resulting map was then
// cross-checked entry-for-entry against React's internal `aliases` Map read out
// of the same bundle. The two agree exactly.
//
// # The finding that shapes this file
//
// React's prop-to-attribute mapping is *namespace-blind*. All five contexts
// produced identical attribute names for all 471 probed props — not one
// divergence. React 19 serializes attributes through a single `pushAttribute`
// switch that never consults the insertion mode, so `strokeWidth` becomes
// `stroke-width` on a `<div>` exactly as it does on a `<path>`.
//
// That is worth stating plainly because the folk understanding is the opposite
// ("SVG has its own case-sensitive attribute table"), and a port that
// implemented the folk understanding would produce markup React does not. So
// the namespace tracked here does not switch tables. It exists because it is
// the honest model of the document — the transitions are real and a future
// namespace-sensitive rule has somewhere correct to live — and because
// [ElementNamespace] and [ContentNamespace] are useful on their own to anyone
// walking or validating this port's output.
//
// The second folk belief the probe killed: React does *not* self-close childless
// SVG elements. `<path d="…"/>` with no children renders as `<path d="…"></path>`.
// The self-closing set is the HTML void list, matched by tag name with no regard
// for namespace at all — `<svg><br/></svg>` really does come out self-closed.
// That is why nothing here touches the void-element logic in ssr_attr.go: the
// existing namespace-blind check is already what React does.

// Namespace identifies which of the three grammars an element belongs to. It is
// exported because "what namespace is this element in?" is a question a caller
// walking a tree, validating output, or writing a custom renderer has to answer
// too, and the answer depends on ancestry rather than on the element alone —
// which makes it exactly the kind of thing that should not be re-derived by
// hand at every call site.
type Namespace int

const (
	// NamespaceHTML is the default grammar, and the one restored inside an
	// integration point such as <foreignObject>.
	NamespaceHTML Namespace = iota
	// NamespaceSVG is entered by <svg> and inherited by every descendant that
	// is not below an integration point.
	NamespaceSVG
	// NamespaceMathML is entered by <math>, with the same inheritance rule.
	NamespaceMathML
)

// String names the namespace for error messages and test output.
func (n Namespace) String() string {
	switch n {
	case NamespaceSVG:
		return "svg"
	case NamespaceMathML:
		return "mathml"
	default:
		return "html"
	}
}

// ssrSVGIntegrationPoints are the SVG elements whose *children* are HTML rather
// than SVG. foreignObject is the one everybody knows; <desc> and <title> are
// integration points too, per the HTML parsing spec's "adjusted current node"
// rules, and leaving them out is a quiet way to get a subtree wrong.
//
// The values are the canonical spellings, kept as documentation; membership is
// tested case-insensitively (see ssrNamespaceKey).
var ssrSVGIntegrationPoints = map[string]string{
	"foreignobject": "foreignObject",
	"desc":          "desc",
	"title":         "title",
}

// ssrMathMLIntegrationPoints are the MathML elements whose children are HTML:
// the five token elements that hold text, plus <annotation-xml>.
//
// annotation-xml is an integration point only when its `encoding` attribute is
// text/html or application/xhtml+xml. This port treats it as one
// unconditionally, because the tag name is all a namespace transition should
// need to look at and text/html is overwhelmingly what the attribute says in
// practice. The choice is observable in nothing today — naming is
// namespace-blind — so the simpler rule wins.
var ssrMathMLIntegrationPoints = map[string]string{
	"mi":             "mi",
	"mo":             "mo",
	"mn":             "mn",
	"ms":             "ms",
	"mtext":          "mtext",
	"annotation-xml": "annotation-xml",
}

// ssrNamespaceKey normalizes a tag name for transition lookups.
//
// Transitions are matched case-insensitively even though the canonical
// spellings are camelCase, because that is what an HTML parser does: it
// lowercases the tag it read and then maps it back through a fixup table, so
// `<FOREIGNOBJECT>` and `<foreignObject>` both open the same element. Note that
// this affects *only* which namespace we decide we are in — the tag written to
// the output is always the caller's exact spelling, which is what keeps
// `linearGradient`, `clipPath`, `feGaussianBlur` and `textPath` intact.
func ssrNamespaceKey(tag string) string {
	return strings.ToLower(tag)
}

// ElementNamespace reports the namespace of an element with the given tag,
// appearing inside content of namespace parent.
//
// This is the element's *own* namespace, which is not always the namespace of
// its children: <foreignObject> is itself an SVG element whose children are
// HTML. Use [ContentNamespace] for the children. Keeping the two separate is
// the whole trick, and conflating them is how ports end up naming
// foreignObject's own attributes by HTML rules or its children's by SVG rules.
//
//	ElementNamespace(NamespaceHTML, "svg")            // NamespaceSVG
//	ElementNamespace(NamespaceSVG, "foreignObject")   // NamespaceSVG — still SVG
//	ElementNamespace(NamespaceHTML, "div")            // NamespaceHTML
func ElementNamespace(parent Namespace, tag string) Namespace {
	switch ssrNamespaceKey(tag) {
	case "svg":
		// <svg> opens SVG from anywhere, including from HTML that is itself
		// inside a foreignObject. That re-entry is what makes the nesting a
		// stack rather than a one-way switch.
		return NamespaceSVG
	case "math":
		return NamespaceMathML
	}
	// Everything else inherits: an unknown or custom tag inside <svg> is SVG,
	// which is the right default for the SVG elements this port has never heard
	// of and for future ones.
	return parent
}

// ContentNamespace reports the namespace that an element's children are in,
// given the namespace of the element itself (as returned by [ElementNamespace])
// and its tag.
//
// It differs from the element's own namespace at exactly the integration
// points: inside SVG, the children of <foreignObject>, <desc> and <title> are
// HTML; inside MathML, the children of <mi>, <mo>, <mn>, <ms>, <mtext> and
// <annotation-xml> are HTML. Leaving such an element restores the enclosing
// namespace automatically, because the caller holds the outer value on its own
// stack — see the emitter in ssr.go, which saves and restores around each host
// element.
//
//	ContentNamespace(NamespaceSVG, "g")              // NamespaceSVG
//	ContentNamespace(NamespaceSVG, "foreignObject")  // NamespaceHTML
//	ContentNamespace(NamespaceMathML, "mtext")       // NamespaceHTML
func ContentNamespace(own Namespace, tag string) Namespace {
	key := ssrNamespaceKey(tag)
	switch own {
	case NamespaceSVG:
		if _, ok := ssrSVGIntegrationPoints[key]; ok {
			return NamespaceHTML
		}
	case NamespaceMathML:
		if _, ok := ssrMathMLIntegrationPoints[key]; ok {
			return NamespaceHTML
		}
	}
	return own
}

// ssrResolveAttributeName is the namespace-aware seam the emitter calls instead
// of [AttributeName] directly.
//
// It is a straight delegation today, and that is the finding rather than an
// omission: the probe measured every prop in five nesting contexts and React's
// naming did not depend on the namespace once. So there is no second table to
// consult, and inventing one would put attribute names in SVG subtrees that
// React does not produce.
//
// Why the seam exists at all, given that it currently adds nothing:
// [ssrEmitter] genuinely tracks the namespace, correctly, across integration
// points — the hard part. Making the attribute path take that value as an
// argument costs one parameter and means a namespace-sensitive rule, if one is
// ever found, is a change to this function rather than a re-plumbing of the
// whole walk. The alternative is discovering the need and then having to thread
// state through a recursive emitter under pressure.
//
// The full measured oracle lives in ssr_namespace_test.go, which asserts that
// whatever table ssr_attr.go carries reproduces all 146 observed SVG and MathML
// attribute names. That is the guard that matters: the knowledge is checked
// against react-dom rather than against a second copy of itself.
func ssrResolveAttributeName(prop string, ns Namespace) (string, bool) {
	_ = ns
	return AttributeName(prop)
}
