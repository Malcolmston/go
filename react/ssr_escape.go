package react

import "strings"

// ssrEscapeText is the port's escapeTextForBrowser: the single function every
// piece of untrusted text passes through on its way into markup, whether it
// lands in element content or inside a double-quoted attribute value. React
// uses one escaper for both positions, and so does this port, because the two
// contexts have the same hazards once the quote character is escaped.
//
// Five characters are replaced and nothing else. Escaping only what changes
// meaning keeps output byte-identical to react-dom, which matters because the
// parity harness compares bytes and because a renderer that escaped more (say,
// every non-ASCII rune) would silently produce different HTML from the same
// tree.
//
// The apostrophe is the reason this file exists. React writes the HEX numeric
// reference &#x27;, not the decimal &#39; and not the named &apos; (which is
// undefined in HTML 4). Verified against the pinned oracle, react-dom 19.2.7:
//
//	renderToStaticMarkup(<div>{`&<>"'`}</div>)      -> <div>&amp;&lt;&gt;&quot;&#x27;</div>
//	renderToStaticMarkup(<div title={`&<>"'`} />)   -> <div title="&amp;&lt;&gt;&quot;&#x27;"></div>
//
// Both forms are the same six-byte-per-quote sequence, which is why one
// function serves both call sites in ssr.go.
//
// Note on the duplicate: ssrEscapeHTML in ssr_attr.go is the legacy spelling of
// this function and differs only in emitting the decimal &#39;. It is kept
// alive solely because a test in ssr_attr_test.go asserts the decimal form, and
// that file is owned elsewhere and off-limits right now. Once its owner
// releases it, ssrEscapeHTML and its test case should be deleted and every
// caller pointed here; there should be exactly one escaper in this package.
func ssrEscapeText(s string) string {
	// Fast path: most strings need no escaping at all, and allocating a Builder
	// for every text node and attribute value in a tree would dominate the cost
	// of rendering. strings.ContainsAny scans once and returns the original
	// string untouched, so the common case costs no allocation.
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/8 + 8)
	// Indexing bytes rather than ranging over runes is deliberate: all five
	// escaped characters are ASCII, so no multi-byte rune can contain one of
	// them, and copying the remaining bytes verbatim passes invalid UTF-8
	// through unchanged instead of rewriting it to U+FFFD the way WriteRune
	// would.
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#x27;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
