package react

// URL sanitization for server rendering.
//
// react-dom refuses to write a `javascript:` URL into the document. Rather than
// dropping the attribute, which would silently change the shape of the markup,
// it replaces the value with a URL that throws when a browser tries to run it,
// so the page still has an attribute in the same place and the failure is loud
// rather than invisible. This file reproduces that behaviour byte for byte.
//
// Two details matter for parity and are easy to get wrong:
//
//   - The rule is keyed by prop name, not by tag. React's sanitizeURL is called
//     from the writers for `src`, `href`, `action`, `formAction` and `xlinkHref`
//     regardless of which element carries them, so `<div href="javascript:...">`
//     is sanitized too even though it means nothing to a browser.
//   - The scheme match is deliberately loose, because HTML attribute parsing
//     and URL parsing both tolerate junk that a naive prefix test would let
//     through. Any run of control characters and spaces may precede the scheme,
//     the letters may be separated by tabs and newlines, and the whole thing is
//     case-insensitive. `java<tab>script:` is a live URL in a browser, so it has
//     to be caught here.
//
// Only `javascript:` is blocked. `data:` and `vbscript:` URLs pass through
// untouched: that is upstream's choice, verified against react-dom 19.2.7, and
// this port follows the oracle rather than improving on it.

// ssrBlockedJavaScriptURL is the replacement react-dom writes in place of a
// javascript: URL. It is a valid javascript: URL itself, which is the point:
// whatever would have triggered the original now raises instead.
//
// The value is written before HTML escaping, so the apostrophes here reach the
// output as character references like any other attribute value.
const ssrBlockedJavaScriptURL = "javascript:throw new Error('React has blocked a javascript: URL as a security precaution.')"

// ssrURLSanitizedProps is the set of prop names whose values are checked for a
// javascript: scheme, keyed by the React prop name rather than the emitted
// attribute name (`xlinkHref`, not `xlink:href`).
var ssrURLSanitizedProps = map[string]bool{
	"src":        true,
	"href":       true,
	"action":     true,
	"formAction": true,
	"xlinkHref":  true,
}

// ssrIsJavaScriptURL reports whether s would be treated as a javascript: URL by
// a browser, matching react-dom's isJavaScriptProtocol regexp:
//
//	/^[\u0000-\u001F ]*j[\r\n\t]*a[\r\n\t]*v[\r\n\t]*a[\r\n\t]*s[\r\n\t]*c[\r\n\t]*r[\r\n\t]*i[\r\n\t]*p[\r\n\t]*t[\r\n\t]*:/i
//
// It is written as a scanner rather than a regexp because it runs on every
// src/href in the tree, and the overwhelmingly common answer is "no" — a
// scanner reaches that answer on the first byte without touching the regexp
// engine.
//
// The comparison is byte-wise, which is safe here: every character the pattern
// can match is ASCII, and a multi-byte UTF-8 sequence has no ASCII bytes in it,
// so no interior byte of a rune can be mistaken for part of the scheme.
func ssrIsJavaScriptURL(s string) bool {
	i := 0
	// Leading control characters and spaces are skipped, exactly the
	// [\u0000-\u001F ] class. Note that this is the raw byte range: it does not
	// include other Unicode whitespace, and upstream does not either.
	for i < len(s) && (s[i] <= 0x1F || s[i] == ' ') {
		i++
	}
	const scheme = "javascript"
	for k := 0; k < len(scheme); k++ {
		if i >= len(s) {
			return false
		}
		c := s[i]
		// ASCII case folding only: the pattern's /i flag cannot match anything
		// outside a-z here.
		if c|0x20 != scheme[k] {
			return false
		}
		i++
		// Tabs, carriage returns and newlines may appear between any two
		// letters of the scheme, and after the last one before the colon.
		for i < len(s) && (s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
			i++
		}
	}
	return i < len(s) && s[i] == ':'
}

// ssrSanitizeURLValue applies the javascript: rule to one prop's already
// stringified value. It reports whether the value was replaced, so a caller can
// tell "left alone" from "happened to equal the blocked URL".
func ssrSanitizeURLValue(prop, value string) (string, bool) {
	if !ssrURLSanitizedProps[prop] {
		return value, false
	}
	if !ssrIsJavaScriptURL(value) {
		return value, false
	}
	return ssrBlockedJavaScriptURL, true
}
