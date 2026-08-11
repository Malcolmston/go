package react

import (
	"io"
	"reflect"
	"strings"
)

// React 19's Float machinery does not render head-managed elements where the
// author wrote them. A <meta>, a <title>, most <link>s and an async <script>
// with a src are lifted out of the tree and flushed at the front of the
// document — into the start of the <head> when the tree has one, and ahead of
// everything otherwise. This file is that lift: the classifier that decides
// which elements are head-managed, the ordered buffer that collects them, and
// the assembly step that splices them back in.
//
// Buffering, and why it is confined to this path. RenderToWriter streams: bytes
// leave the emitter as they are produced. Hoisting cannot stream, because the
// last element of a document can still belong at its front. So the hoisting path
// accumulates the whole document in memory and writes it once at the end — and
// only takes over when the mounted tree actually contains an element that could
// hoist. A tree of divs, spans and text is walked and streamed exactly as it was
// before this file existed; see ssrHoistCandidateTree.
//
// Everything below was read off probe renders against the pinned oracle,
// react-dom 19.2.7, and cross-checked against pushStartInstance's meta, title,
// link, script and style cases plus flushPreamble in
// react-dom-server-legacy.browser.development.js:
//
//	<div><meta/></div>                     -> <meta/><div></div>
//	<div><title>T</title></div>            -> <title>T</title><div></div>
//	<div><script async src="/a.js"/></div> -> <script async="" src="/a.js"></script><div></div>
//	<div><link rel="icon" href="/f"/></div> -> <link rel="icon" href="/f"/><div></div>
//	<div><base href="/"/></div>            -> unchanged: base is not head-managed
//	<div><link rel="stylesheet" href="/a.css"/></div>
//	                                       -> unchanged without a precedence prop
//	<div><meta itemProp="x"/></div>        -> unchanged: itemProp pins an element in place
//	<svg><title>T</title></svg>            -> unchanged: an SVG title is not a document title
//	<noscript><meta name="a"/></noscript>  -> unchanged: nothing hoists out of a noscript
//	<div><meta name="a"/><span>x</span><meta name="b"/></div>
//	                                       -> <meta name="a"/><meta name="b"/><div><span>x</span></div>
//	<html><head><title>T</title></head><body><meta name="b"/></body></html>
//	                                       -> ...<head><title>T</title><meta name="b"/></head>...
//	<html><body><meta name="x"/></body></html>
//	                                       -> <html><head><meta name="x"/></head><body></body></html>
//
// The last one is not a typo: an <html> with no <head> gets one synthesized to
// hold the hoisted elements.

// ssrHoistKind is what an element is once Float has looked at it. Every kind but
// ssrHoistNone leaves the element's own position in the tree empty.
type ssrHoistKind int

const (
	// ssrHoistNone is an element that renders where it was written.
	ssrHoistNone ssrHoistKind = iota
	// ssrHoistCharset is a <meta charSet>, which flushes ahead of every other
	// hoisted element because a document's encoding has to be declared before
	// the bytes it applies to.
	ssrHoistCharset
	// ssrHoistViewport is a <meta name="viewport">, which flushes second for the
	// same reason: the layout viewport should be settled before content arrives.
	ssrHoistViewport
	// ssrHoistSheet is a <link rel="stylesheet"> carrying a precedence prop.
	// React manages those as resources: they are grouped by precedence,
	// deduplicated by href, and the prop is rewritten to data-precedence.
	ssrHoistSheet
	// ssrHoistStyle is a <style> carrying both precedence and href. Every style
	// sharing a precedence is merged into one <style> element whose data-href
	// lists the hrefs that went into it.
	ssrHoistStyle
	// ssrHoistScript is an async <script src>, deduplicated by src.
	ssrHoistScript
	// ssrHoistTitle is a document <title>. It shares the generic bucket with
	// metas and links and differs from them in one observable way: removing it
	// does not leave a hydration text separator behind.
	ssrHoistTitle
	// ssrHoistGeneric is every other head-managed element — a plain <meta>, a
	// <link> with a rel and an href — flushed last, in document order.
	ssrHoistGeneric
)

// ssrHoistClassify reports how Float treats an element with this tag and these
// props, assuming the element is not inside a <noscript> and not in the SVG
// namespace (both of which pin every element in place, and are the caller's
// tests to make because they are properties of the position, not the props).
//
// The rules, per tag, are React's in-place conditions read backwards:
//
//   - any of the five tags with a non-nil itemProp stays put. An itemProp means
//     the element is microdata describing its parent, so moving it would change
//     what it says. Note that itemProp={false} and itemProp="" both count as
//     present; only nil does not.
//   - meta hoists unconditionally, into the charset bucket for a string charSet,
//     the viewport bucket for name="viewport", and the generic bucket otherwise.
//   - title hoists unconditionally.
//   - link needs a string rel and a non-empty string href; without both it
//     stays put, which is why a bare <link/> renders in place. rel="stylesheet"
//     then needs a string precedence and no disabled, onLoad or onError to
//     become a managed sheet, and stays put otherwise — a plain stylesheet link
//     is not hoisted. Any other rel hoists unless onLoad or onError is set.
//   - script needs a non-empty string src and a truthy async, and no onLoad or
//     onError. A deferred script, an inline script and an async script without a
//     src all stay put.
//   - style needs both a string precedence and a non-empty string href.
func ssrHoistClassify(tag string, props Props) ssrHoistKind {
	switch tag {
	case "meta", "title", "link", "script", "style":
	default:
		return ssrHoistNone
	}
	if value, present := props["itemProp"]; present && value != nil {
		return ssrHoistNone
	}

	switch tag {
	case "meta":
		if _, ok := props["charSet"].(string); ok {
			return ssrHoistCharset
		}
		if name, _ := props["name"].(string); name == "viewport" {
			return ssrHoistViewport
		}
		return ssrHoistGeneric

	case "title":
		return ssrHoistTitle

	case "link":
		rel, relIsString := props["rel"].(string)
		href, _ := props["href"].(string)
		if !relIsString || href == "" {
			return ssrHoistNone
		}
		if rel != "stylesheet" {
			if ssrHoistTruthy(props["onLoad"]) || ssrHoistTruthy(props["onError"]) {
				return ssrHoistNone
			}
			return ssrHoistGeneric
		}
		if _, ok := props["precedence"].(string); !ok {
			return ssrHoistNone
		}
		if value, present := props["disabled"]; present && value != nil {
			return ssrHoistNone
		}
		if ssrHoistTruthy(props["onLoad"]) || ssrHoistTruthy(props["onError"]) {
			return ssrHoistNone
		}
		return ssrHoistSheet

	case "script":
		src, _ := props["src"].(string)
		if src == "" || !ssrHoistAsyncTruthy(props["async"]) {
			return ssrHoistNone
		}
		if ssrHoistTruthy(props["onLoad"]) || ssrHoistTruthy(props["onError"]) {
			return ssrHoistNone
		}
		return ssrHoistScript

	case "style":
		if _, ok := props["precedence"].(string); !ok {
			return ssrHoistNone
		}
		if href, _ := props["href"].(string); href == "" {
			return ssrHoistNone
		}
		return ssrHoistStyle
	}
	return ssrHoistNone
}

// ssrHoistAsyncTruthy is the async prop's test. It is JavaScript truthiness with
// one extra exclusion React writes out by hand: a function-valued or
// symbol-valued async does not make a script hoistable, even though a function
// is perfectly truthy in JavaScript. Go has no symbol.
func ssrHoistAsyncTruthy(value any) bool {
	if value != nil && reflect.ValueOf(value).Kind() == reflect.Func {
		return false
	}
	return ssrHoistTruthy(value)
}

// ssrHoistTruthy is JavaScript truthiness for a prop React tests with a bare
// `if`, which is how onLoad and onError pin an element in place. A handler is a
// func and a func is truthy, so unlike async there is nothing to exclude here.
func ssrHoistTruthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	}
	return true
}

// ssrHoistBuffer collects a render's hoisted elements in React's flush buckets.
//
// The buckets are not cosmetic: they are the order flushPreamble writes them in,
// which is charset, then viewport, then the image preloads promoted to the early
// bucket, then stylesheets by precedence, then async scripts, then the remaining
// image preloads, then everything else in document order. A page can therefore
// have its encoding, its stylesheet and its scripts all ahead of a <meta> that
// was written above them in the source.
type ssrHoistBuffer struct {
	charset  []string
	viewport []string
	scripts  []string
	generic  []string

	// precedences is the order the precedence groups were first seen in, which
	// is the order they flush in — not the alphabetical order of the names.
	precedences []string
	// sheets holds each group's <link> markup, and styleHrefs/styleRules hold
	// the pieces of the single merged <style> the group's style elements become.
	sheets     map[string][]string
	styleHrefs map[string][]string
	styleRules map[string][]string

	// styleSeen is React's styleResources: one map shared by managed stylesheet
	// links and managed style tags, so an href used by either is never emitted
	// twice even across the two element types.
	styleSeen map[string]bool
	// scriptSeen is React's scriptResources and moduleScriptResources. They are
	// separate maps in React, so the same src hoists once as a classic script
	// and once more as type="module"; the key here carries the distinction.
	scriptSeen map[string]bool
}

// addChunk records a hoisted element's finished markup in its bucket.
func (b *ssrHoistBuffer) addChunk(kind ssrHoistKind, props Props, chunk string) {
	switch kind {
	case ssrHoistCharset:
		b.charset = append(b.charset, chunk)
	case ssrHoistViewport:
		b.viewport = append(b.viewport, chunk)
	case ssrHoistScript:
		src, _ := props["src"].(string)
		key := src
		if t, _ := props["type"].(string); t == "module" {
			key = "module\n" + src
		}
		if b.scriptSeen == nil {
			b.scriptSeen = map[string]bool{}
		}
		if b.scriptSeen[key] {
			return
		}
		b.scriptSeen[key] = true
		b.scripts = append(b.scripts, chunk)
	case ssrHoistSheet:
		precedence, _ := props["precedence"].(string)
		href, _ := props["href"].(string)
		if !b.claimStyleHref(href) {
			return
		}
		b.group(precedence)
		b.sheets[precedence] = append(b.sheets[precedence], chunk)
	default:
		b.generic = append(b.generic, chunk)
	}
}

// addStyle records a managed <style>. Its markup is not the element as written:
// every style sharing a precedence is merged into one, so only the escaped href
// and the rules survive and every other prop is dropped.
func (b *ssrHoistBuffer) addStyle(precedence, href, rules string) {
	if !b.claimStyleHref(href) {
		return
	}
	b.group(precedence)
	b.styleHrefs[precedence] = append(b.styleHrefs[precedence], ssrEscapeText(href))
	b.styleRules[precedence] = append(b.styleRules[precedence], rules)
}

// claimStyleHref reports whether this href is new, recording it if so. It is the
// deduplication both managed stylesheet links and managed styles go through.
func (b *ssrHoistBuffer) claimStyleHref(href string) bool {
	if b.styleSeen == nil {
		b.styleSeen = map[string]bool{}
	}
	if b.styleSeen[href] {
		return false
	}
	b.styleSeen[href] = true
	return true
}

// group makes sure a precedence has a slot, remembering when it was first seen.
func (b *ssrHoistBuffer) group(precedence string) {
	if b.sheets == nil {
		b.sheets = map[string][]string{}
		b.styleHrefs = map[string][]string{}
		b.styleRules = map[string][]string{}
	}
	if _, ok := b.sheets[precedence]; ok {
		return
	}
	b.sheets[precedence] = nil
	b.styleHrefs[precedence] = nil
	b.styleRules[precedence] = nil
	b.precedences = append(b.precedences, precedence)
}

// styleChunks returns the stylesheet bucket: for each precedence in first-seen
// order, that group's links followed by the one <style> its style elements were
// merged into.
//
// A group holding only links contributes no style element at all — React's test
// is "no sheets, or some hrefs", which is what keeps a page that only links
// stylesheets from growing an empty <style> per precedence.
func (b *ssrHoistBuffer) styleChunks() []string {
	var out []string
	for _, precedence := range b.precedences {
		out = append(out, b.sheets[precedence]...)
		hrefs := b.styleHrefs[precedence]
		if len(b.sheets[precedence]) > 0 && len(hrefs) == 0 {
			continue
		}
		var s strings.Builder
		s.WriteString(`<style data-precedence="` + ssrEscapeText(precedence) + `"`)
		if len(hrefs) > 0 {
			s.WriteString(` data-href="` + strings.Join(hrefs, " ") + `"`)
		}
		s.WriteString(">")
		for _, rules := range b.styleRules[precedence] {
			s.WriteString(rules)
		}
		s.WriteString("</style>")
		out = append(out, s.String())
	}
	return out
}

// chunks returns every hoisted chunk in flush order.
//
// Two slots in that order are deliberately empty. React flushes the image
// preloads an <img src> contributes between the viewport metas and the
// stylesheets (the early bucket) and between the async scripts and the generic
// hoistables (the bulk bucket) — ssr_preload.go collects them into exactly those
// two buckets and this is where they belong. They are not flushed yet because
// ssrImagePreloadFor builds the link's href out of the img's src verbatim, and
// React sanitizes it: a javascript: src reaches the oracle's preload as the
// blocked-URL replacement, not as the URL. Flushing the buckets before that
// sanitizing lands would put a javascript: URL in the document that the img
// itself is careful not to emit. Once ssr_preload.go sanitizes, the two
// appends go here, in these positions.
func (b *ssrHoistBuffer) chunks() []string {
	var out []string
	out = append(out, b.charset...)
	out = append(out, b.viewport...)
	out = append(out, b.styleChunks()...)
	out = append(out, b.scripts...)
	out = append(out, b.generic...)
	return out
}

// ssrHoister is the per-render state the hoisting path needs: the buffer holding
// the document being assembled, the collected chunks, and where in the document
// they have to land.
type ssrHoister struct {
	// doc is the document buffer. It is also the emitter's identity test for
	// "am I writing the main stream": a portal's container buffer and the
	// scratch buffer a hoisted element is rendered into are both other writers,
	// and neither rearranges anything.
	doc *strings.Builder

	buf ssrHoistBuffer

	// suppress counts the <noscript> elements currently open. React pins every
	// head-managed element inside one in place.
	suppress int

	// hoisting an element re-enters host for the same fiber; skip lets that one
	// call through untouched.
	skip *fiber

	// headStart and htmlStart are byte offsets of the "<" of the document's head
	// and html open tags. The hoisted chunks go immediately after whichever open
	// tag ends first — see assemble.
	hasHead   bool
	headStart int
	hasHTML   bool
	htmlStart int
}

// hoistIntercept is the emitter's one hook into this file. It returns true when
// it has taken responsibility for the element, which for a hoisted element means
// the element produced no bytes where it was written.
//
// It also notices the document's <html> and <head> on the way past — they are
// emitted normally, but their positions are what assemble splices into — and
// tracks <noscript> so that nothing inside one hoists.
func (e *ssrEmitter) hoistIntercept(f *fiber, tag string) bool {
	h := e.hoist
	if h == nil {
		return false
	}
	if h.skip == f {
		h.skip = nil
		return false
	}
	// Only the main document stream is rearranged. Inside a portal container's
	// buffer, or inside the scratch buffer of an element already being hoisted,
	// an offset into doc would point at unrelated bytes.
	if e.w != h.doc {
		return false
	}

	switch tag {
	case "noscript":
		h.skip = f
		h.suppress++
		e.host(f, tag)
		h.suppress--
		return true
	case "html":
		if !h.hasHTML && ssrHoistDocumentLevel(f) {
			h.hasHTML, h.htmlStart = true, h.doc.Len()
		}
		return false
	case "head":
		if !h.hasHead && ssrHoistDocumentLevel(f) {
			h.hasHead, h.headStart = true, h.doc.Len()
		}
		return false
	}

	// An SVG <title> is a tooltip and an SVG <style> is a scoped stylesheet;
	// neither is a document-level anything. MathML is deliberately not excluded:
	// React tests only for the SVG insertion mode, so a <meta> inside <math>
	// really does hoist.
	if h.suppress > 0 || e.ns == NamespaceSVG {
		return false
	}
	kind := ssrHoistClassify(tag, f.props)
	if kind == ssrHoistNone {
		return false
	}

	// React writes the hydration text separator in place of the element it
	// removed, so that two text nodes that were not adjacent in the tree do not
	// become one run of characters in the output. The title case does not, which
	// is why <div>a<title>T</title>b</div> renders "ab" and the same tree with a
	// <meta> renders "a<!-- -->b". Either way the run of text has ended.
	if kind != ssrHoistTitle && !e.staticMarkup && e.lastText {
		e.write(ssrTextSeparator)
	}
	e.lastText = false

	if kind == ssrHoistStyle {
		e.hoistStyle(f)
		return true
	}

	// The element is rendered exactly as it would have been in place, into a
	// scratch buffer, and the bytes are filed in a bucket instead of written.
	var scratch strings.Builder
	outer := e.w
	e.w = &scratch
	saved := f.props
	if kind == ssrHoistSheet {
		// A managed stylesheet's precedence is not an attribute: React rewrites
		// it to data-precedence, which is what the client runtime reads to keep
		// the sheets in order.
		f.props = ssrHoistSheetProps(saved)
	}
	h.skip = f
	e.host(f, tag)
	f.props = saved
	e.w = outer
	if e.err == nil {
		h.buf.addChunk(kind, saved, scratch.String())
	}
	return true
}

// hoistStyle files a managed <style>. Only its href and its rules survive the
// merge into the group's single style element, so the element is never rendered.
func (e *ssrEmitter) hoistStyle(f *fiber) {
	precedence, _ := f.props["precedence"].(string)
	href, _ := f.props["href"].(string)

	raw, hasRaw, err := ssrDangerousContent(f.props)
	if err != nil {
		e.fail(err)
		return
	}
	rules := raw
	if !hasRaw {
		if text, ok := ssrRawTextChildren(f, "style"); ok {
			rules = ssrRawTextContent("style", text)
		}
	}
	e.hoist.buf.addStyle(precedence, href, rules)
}

// ssrHoistSheetProps copies props with precedence renamed to data-precedence.
// The copy is shallow and short-lived: it is installed on the fiber only for the
// one host call that renders the link, so nothing outside this file ever sees it.
func ssrHoistSheetProps(props Props) Props {
	out := make(Props, len(props))
	for k, v := range props {
		if k == "precedence" {
			continue
		}
		out[k] = v
	}
	out["data-precedence"] = props["precedence"]
	return out
}

// ssrHoistDocumentLevel reports whether f is where a document's own <html> or
// <head> would be: at the root of the render, or directly inside an <html> that
// is itself at the root. A <head> nested in a <div> is an ordinary element that
// happens to be spelled head, and React does not flush anything into it.
func ssrHoistDocumentLevel(f *fiber) bool {
	for p := f.parent; p != nil; p = p.parent {
		if tag, ok := p.typ.(string); ok && tag != "html" {
			return false
		}
	}
	return true
}

// assemble returns the finished document: the buffered markup with the hoisted
// chunks spliced in at the front of the head, or in a head synthesized for an
// <html> that has none, or ahead of everything when the tree is not a document.
func (h *ssrHoister) assemble() string {
	body := h.doc.String()
	chunks := h.buf.chunks()
	synthesizeHead := h.hasHTML && !h.hasHead
	if len(chunks) == 0 && !synthesizeHead {
		return body
	}
	hoisted := strings.Join(chunks, "")

	switch {
	case h.hasHead:
		at := ssrHoistAfterOpenTag(body, h.headStart)
		return body[:at] + hoisted + body[at:]
	case synthesizeHead:
		// React emits the head unconditionally for an <html> that lacks one,
		// even when there is nothing to put in it.
		at := ssrHoistAfterOpenTag(body, h.htmlStart)
		return body[:at] + "<head>" + hoisted + "</head>" + body[at:]
	default:
		return hoisted + body
	}
}

// ssrHoistAfterOpenTag returns the offset just past the open tag that starts at
// start. Scanning for the first ">" is exact rather than approximate: an
// attribute value is escaped before it is written, so the only unescaped ">" in
// the range is the one that ends the tag.
func ssrHoistAfterOpenTag(s string, start int) int {
	if i := strings.IndexByte(s[start:], '>'); i >= 0 {
		return start + i + 1
	}
	return len(s)
}

// ssrHoistCandidateTree reports whether a mounted tree contains any element that
// the hoisting path could move or synthesize around. It is the test that keeps
// RenderToWriter streaming for every tree that has no document machinery in it:
// a false answer means the walk writes straight to the caller's writer, exactly
// as it did before hoisting existed.
//
// It is deliberately generous. Answering yes for an element that turns out to
// stay in place costs one buffered render; answering no for one that would have
// hoisted would silently drop the hoist, so every tag any rule can fire on is
// listed, including <img>, whose Float preload is collected in ssr_preload.go
// and belongs in the buffer's flush order (see chunks).
func ssrHoistCandidateTree(f *fiber) bool {
	for c := f.child; c != nil; c = c.sibling {
		if tag, ok := c.typ.(string); ok {
			switch tag {
			case "html", "head", "meta", "title", "link", "script", "style", "img":
				return true
			}
		}
		if ssrHoistCandidateTree(c) {
			return true
		}
	}
	return false
}

// ssrHoistRender walks tree with e, assembling the main output through the
// hoisting path. The whole document is held in memory: see the file comment.
func ssrHoistRender(e *ssrEmitter, tree *fiber) (string, error) {
	var doc strings.Builder
	e.w = &doc
	e.hoist = &ssrHoister{doc: &doc}
	e.children(tree)
	if e.err != nil {
		return "", e.err
	}
	return e.hoist.assemble(), nil
}

// ssrHoistRenderToWriter walks tree with e and writes the result to w, streaming
// it when the tree cannot hoist anything and buffering it when it can.
func ssrHoistRenderToWriter(e *ssrEmitter, w io.Writer, tree *fiber) error {
	if !ssrHoistCandidateTree(tree) {
		e.w = w
		e.children(tree)
		return e.err
	}
	out, err := ssrHoistRender(e, tree)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, out); err != nil {
		return err
	}
	return nil
}
