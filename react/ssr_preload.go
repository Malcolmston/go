package react

import (
	"reflect"
	"strings"
)

// React 19's Float machinery turns an ordinary <img src> into two things in the
// output: the img itself, where the author wrote it, and a hoisted
//
//	<link rel="preload" as="image" href="..."/>
//
// emitted ahead of the document body so the browser can start fetching the
// image before it has parsed down to the tag. This file is the decision and the
// serialization for that link. It deliberately does not decide *where* the
// chunk lands in the output: that is the hoistable buffer's job (see the
// float-hoisting work item), and this file only hands it ordered chunks in
// React's two priority buckets.
//
// Everything below was read off probe renders against the pinned oracle,
// react-dom 19.2.7, renderToStaticMarkup, and cross-checked against
// pushImg/pushLinkImpl in react-dom-server.browser.development.js:
//
//	<img alt="a" src="/a.png"/>
//	  -> <link rel="preload" as="image" href="/a.png"/><img alt="a" src="/a.png"/>
//	<img src="/a.png" srcSet="/a.png 1x"/>
//	  -> <link rel="preload" as="image" imageSrcSet="/a.png 1x"/><img .../>
//	<img src="/a.png" srcSet="/a.png 1x" sizes="100vw"/>
//	  -> <link rel="preload" as="image" imageSrcSet="/a.png 1x" imageSizes="100vw"/><img .../>
//	<img src="/a.png" loading="lazy"/>              -> no preload
//	<img src="/a.png" fetchPriority="low"/>         -> no preload
//	<img alt="a"/>                                  -> no preload
//	<picture><img src="/a.png"/></picture>          -> no preload
//	<noscript><img src="/a.png"/></noscript>        -> no preload
//	<img src="data:image/png;base64,AAA"/>          -> no preload
//
// Two spellings look like bugs and are not. The link's srcset/sizes attributes
// are written imageSrcSet and imageSizes, camelCase, because React pushes those
// property names straight through for rel=preload links; crossOrigin, by
// contrast, becomes lowercase crossorigin and its value is normalized to either
// "use-credentials" or the empty string. Copy the bytes; do not tidy them.

// ssrImagePreloadHighBucketLimit is how many image preloads React will put in
// the early ("high") bucket before the rest fall into the bulk bucket that
// flushes after stylesheets and scripts. React's test is
// `10 > renderState.highImagePreloads.size`, so the tenth image preload is the
// last one promoted by size alone; fetchPriority="high" ignores the limit.
const ssrImagePreloadHighBucketLimit = 10

// ssrImagePreload is one hoisted preload link: the markup to emit, the key it
// deduplicates on, and whether the author demanded the early bucket.
type ssrImagePreload struct {
	// key is React's image resource key: the srcSet plus "\n" plus the sizes
	// when a srcSet is present, and the src otherwise. Two imgs sharing a key
	// produce one preload; note that an img with a srcSet and an img with only
	// the matching src have *different* keys and so produce two.
	key string

	// chunk is the complete <link .../> markup, already escaped.
	chunk string

	// high records fetchPriority="high", which puts the preload in the early
	// bucket regardless of how many are already there.
	high bool
}

// ssrPreloadSuppressingTag reports whether an element suppresses the img
// preload for everything inside it. React tracks <picture> and <noscript> in
// its tag scope: inside a picture the img is only a fallback for the source
// list, and inside a noscript nothing should be fetched eagerly at all. The
// flag applies to the whole subtree, not just direct children.
func ssrPreloadSuppressingTag(tag string) bool {
	return tag == "picture" || tag == "noscript"
}

// ssrImagePreloadFor returns the preload an <img> with these props contributes,
// or ok == false when React emits none. suppressed is true when the img is
// anywhere inside a <picture> or <noscript>.
//
// The suppression rules, in React's order:
//
//   - loading="lazy" — the author asked the browser not to fetch it yet;
//   - neither src nor srcSet (a missing or empty one counts as absent, since
//     React tests JS truthiness);
//   - src or srcSet present but not a string — React refuses to preload what it
//     cannot serialize, even though the img attribute itself still renders;
//   - fetchPriority="low";
//   - inside <picture> or <noscript>;
//   - a data: URI in src or srcSet — the bytes are already in the document, so
//     a preload would only duplicate them. The prefix test is case-insensitive
//     on the four letters and requires the colon in the fifth byte, exactly as
//     React's hand-unrolled comparison does.
func ssrImagePreloadFor(props Props, suppressed bool) (ssrImagePreload, bool) {
	if suppressed {
		return ssrImagePreload{}, false
	}

	src, srcOK := ssrPreloadStringProp(props, "src")
	if !srcOK {
		return ssrImagePreload{}, false
	}
	srcSet, srcSetOK := ssrPreloadStringProp(props, "srcSet")
	if !srcSetOK {
		return ssrImagePreload{}, false
	}
	if src == "" && srcSet == "" {
		return ssrImagePreload{}, false
	}
	if loading, _ := props["loading"].(string); loading == "lazy" {
		return ssrImagePreload{}, false
	}
	fetchPriority, _ := props["fetchPriority"].(string)
	if fetchPriority == "low" {
		return ssrImagePreload{}, false
	}
	if ssrIsDataURL(src) || ssrIsDataURL(srcSet) {
		return ssrImagePreload{}, false
	}

	// sizes reaches the link only as a string. sizes={10} renders on the img as
	// sizes="10" and contributes no imageSizes, because React reads it with a
	// typeof check before building the link props.
	sizes, hasSizes := props["sizes"].(string)

	var b strings.Builder
	b.WriteString(`<link rel="preload" as="image"`)
	// href and imageSrcSet are mutually exclusive: React passes
	// `href: srcSet ? undefined : src`, so a non-empty srcSet takes over even
	// when src is also set. An *empty* srcSet is falsy, so href comes back and
	// imageSrcSet="" is still written — that asymmetry is React's, verified.
	if srcSet == "" {
		ssrPreloadAttr(&b, "href", src)
	}
	if _, present := props["srcSet"]; present {
		ssrPreloadAttr(&b, "imageSrcSet", srcSet)
	}
	if hasSizes {
		ssrPreloadAttr(&b, "imageSizes", sizes)
	}
	if crossOrigin, ok := props["crossOrigin"].(string); ok {
		// React collapses every value but "use-credentials" — including
		// "anonymous" — to the empty string on the preload link, while the img
		// keeps whatever the author wrote.
		if crossOrigin != "use-credentials" {
			crossOrigin = ""
		}
		ssrPreloadAttr(&b, "crossorigin", crossOrigin)
	}
	// integrity, type, fetchPriority and referrerPolicy are copied through in
	// this order, which is the order of the keys in React's link props object.
	for _, prop := range [...]string{"integrity", "type", "fetchPriority", "referrerPolicy"} {
		if value, ok := ssrPreloadPassthrough(props[prop]); ok {
			ssrPreloadAttr(&b, prop, value)
		}
	}
	b.WriteString("/>")

	key := src
	if srcSet != "" {
		key = srcSet + "\n" + sizes
	}
	return ssrImagePreload{key: key, chunk: b.String(), high: fetchPriority == "high"}, true
}

// ssrPreloadStringProp reads a prop React requires to be a string. It returns
// ok == false only for a value that is present, non-nil and not a string —
// which is React's `"string" !== typeof v && null != v` test, the one that
// makes <img src={5}/> render its src and still preload nothing.
func ssrPreloadStringProp(props Props, prop string) (string, bool) {
	value, present := props[prop]
	if !present || value == nil {
		return "", true
	}
	s, isString := value.(string)
	return s, isString
}

// ssrPreloadPassthrough formats a prop copied verbatim onto the preload link.
// A nil is absent; a bool or a func is dropped the way every string-valued
// attribute drops them; anything else is stringified, so integrity={5} reaches
// the link as integrity="5" just as it does on the img.
func ssrPreloadPassthrough(value any) (string, bool) {
	switch value.(type) {
	case nil:
		return "", false
	case bool:
		return "", false
	case DangerousHTML:
		return "", false
	}
	if reflect.ValueOf(value).Kind() == reflect.Func {
		return "", false
	}
	return textOf(value), true
}

// ssrPreloadAttr appends one already-decided attribute, escaping the value the
// same way every other attribute value is escaped.
func ssrPreloadAttr(b *strings.Builder, name, value string) {
	b.WriteString(" " + name + `="` + ssrEscapeText(value) + `"`)
}

// ssrIsDataURL reports whether s starts with a case-insensitive "data:".
func ssrIsDataURL(s string) bool {
	if len(s) < 5 || s[4] != ':' {
		return false
	}
	return strings.EqualFold(s[:4], "data")
}

// ssrImagePreloads collects a render's image preloads: it deduplicates them by
// key, keeps them in emission order, and splits them into React's two buckets.
//
// The buckets are not cosmetic. The early bucket flushes ahead of hoisted
// titles, metas, stylesheets and scripts; the bulk bucket flushes after them.
// React fills the early bucket with the first ten image preloads and sends the
// rest to bulk, so a page with twelve images puts ten preloads before its
// stylesheet link and two after it — verified against the oracle.
//
// One further wrinkle is reproduced here because it is observable: a preload
// already sitting in the bulk bucket is *promoted* when a later img repeats its
// key and the early bucket still has room. Promotion moves the chunk to the end
// of the early bucket rather than to its original position.
type ssrImagePreloads struct {
	high []*ssrImagePreload
	bulk []*ssrImagePreload

	// emitted is React's resumableState.imageResources: a key seen once never
	// produces a second link.
	emitted map[string]bool

	// promotable indexes the entries currently in the bulk bucket by key, which
	// is the only reason a repeated img does any work at all.
	promotable map[string]*ssrImagePreload
}

// add records a preload, deduplicating and bucketing it.
func (p *ssrImagePreloads) add(preload ssrImagePreload) {
	if p.emitted == nil {
		p.emitted = map[string]bool{}
		p.promotable = map[string]*ssrImagePreload{}
	}

	if existing, ok := p.promotable[preload.key]; ok {
		if preload.high || len(p.high) < ssrImagePreloadHighBucketLimit {
			delete(p.promotable, preload.key)
			p.bulk = ssrRemovePreload(p.bulk, existing)
			p.high = append(p.high, existing)
		}
		return
	}
	if p.emitted[preload.key] {
		return
	}
	p.emitted[preload.key] = true

	entry := &preload
	if preload.high || len(p.high) < ssrImagePreloadHighBucketLimit {
		p.high = append(p.high, entry)
		return
	}
	p.bulk = append(p.bulk, entry)
	p.promotable[preload.key] = entry
}

// highChunks returns the markup for the early bucket, in flush order.
func (p *ssrImagePreloads) highChunks() []string { return ssrPreloadChunks(p.high) }

// bulkChunks returns the markup for the late bucket, in flush order.
func (p *ssrImagePreloads) bulkChunks() []string { return ssrPreloadChunks(p.bulk) }

func ssrPreloadChunks(entries []*ssrImagePreload) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.chunk)
	}
	return out
}

// ssrRemovePreload drops one entry from a bucket, preserving the order of the
// rest.
func ssrRemovePreload(entries []*ssrImagePreload, drop *ssrImagePreload) []*ssrImagePreload {
	for i, entry := range entries {
		if entry == drop {
			return append(entries[:i:i], entries[i+1:]...)
		}
	}
	return entries
}
