package react

import "strings"

// Raw-text elements: <script> and <style>.
//
// Everywhere else in HTML, text content has to be entity-escaped so that a "<"
// in the text cannot start a tag. Inside <script> and <style> the HTML parser is
// not looking for tags at all — it scans for the element's own end tag and takes
// everything before it verbatim — so escaping there would be actively wrong: the
// browser would hand the script "1 &lt; 2" as source and the CSS parser would
// see "&amp;" inside a content string.
//
// React does exactly that: pushScriptImpl writes
// escapeEntireInlineScriptContent(children) and the style path writes
// escapeStyleTextContent(children), and neither function does any entity
// escaping. Each performs one substitution whose only purpose is to stop the
// text from closing the element early:
//
//	script: the six characters backslash-u-0-0-7-3 replace a lowercase "s",
//	        and backslash-u-0-0-5-3 an uppercase "S"
//	style:  backslash-7-3-space replaces a lowercase "s",
//	        and backslash-5-3-space an uppercase "S"
//
// Both replacements are literal text — a backslash, then the digits — not the
// character those digits denote. The first is a JavaScript string escape and the
// second a CSS identifier escape, so the byte sequence still means "s" to the
// script or style language while no longer reading as a tag to the HTML
// tokenizer. The case of the matched "s" is what selects between the two forms;
// the rest of the matched tag name is copied through unchanged, since React's
// regex captures only the "s".
//
// Verified against react-dom 19.2.7 by probe (outputs written with the escape
// sequences spelled out):
//
//	<script>{'var a = 1 < 2 && "x" + \'y\';'}</script> renders verbatim
//	<script>{'a</script>b'}</script>  -> a</ + bsl-u0073 + cript>b
//	<script>{'a</SCRIPT>b'}</script>  -> a</ + bsl-u0053 + CRIPT>b
//	<style>{"a{content:'&'}"}</style> renders verbatim
//	<style>{'a</style>b'}</style>     -> a</ + bsl-73-space + tyle>b
//
// The rule is exactly these two tags, in every namespace: an SVG <style> is raw
// too (probed). Escaping still applies to textarea, title, noscript and pre —
// all four were probed and all four escape — so nothing else may be added here.
//
// One documented divergence: React requires a raw-text element's children to be
// a single string, and drops anything else (a number, an array of strings, an
// element) with a development warning, emitting <script></script>. The port has
// no warning channel during SSR, so it concatenates the element's text children
// instead of dropping them, and falls back to ordinary escaped child rendering
// if the subtree contains a host element. No parity case covers either shape;
// see API-DEVIATIONS.md.

// ssrIsRawTextTag reports whether tag names an element whose text content is
// written verbatim rather than entity-escaped.
func ssrIsRawTextTag(tag string) bool {
	return tag == "script" || tag == "style"
}

// ssrRawTextContent returns the bytes to write inside a raw-text element for the
// text content text. tag selects the substitution; a tag that is not a raw-text
// element returns text unchanged, so a caller that forgot the
// [ssrIsRawTextTag] guard cannot silently un-escape ordinary content.
func ssrRawTextContent(tag, text string) string {
	switch tag {
	case "script":
		return ssrBreakUpRawText(text, "cript", "\\u0073", "\\u0053")
	case "style":
		return ssrBreakUpRawText(text, "tyle", "\\73 ", "\\53 ")
	}
	return text
}

// ssrBreakUpRawText rewrites every "<s"+rest and "</s"+rest occurrence in text,
// matched case-insensitively, replacing just the "s" with lower or upper
// depending on the case of the matched letter. It is the byte-scanning
// equivalent of React's two regex replacements.
func ssrBreakUpRawText(text, rest, lower, upper string) string {
	// The scan is over ASCII bytes only. Both needles are ASCII, and a multi-byte
	// UTF-8 sequence never contains an ASCII byte, so byte indexing cannot land
	// inside a rune.
	var b strings.Builder
	i := 0
	for {
		lt := strings.IndexByte(text[i:], '<')
		if lt < 0 {
			break
		}
		lt += i

		// The prefix React captures is "</" when a slash follows, else "<".
		j := lt + 1
		if j < len(text) && text[j] == '/' {
			j++
		}
		if j >= len(text) || (text[j] != 's' && text[j] != 'S') {
			// Not a candidate. Advance past this "<" only, so "<<script>" and the
			// "<" of "</ script" are both retried at the next byte.
			b.WriteString(text[i : lt+1])
			i = lt + 1
			continue
		}
		if !ssrHasPrefixFold(text[j+1:], rest) {
			b.WriteString(text[i : lt+1])
			i = lt + 1
			continue
		}

		b.WriteString(text[i:j]) // "<" or "</"
		if text[j] == 's' {
			b.WriteString(lower)
		} else {
			b.WriteString(upper)
		}
		// The rest of the tag name keeps the case it was written in: React's
		// capture group copies it through untouched.
		b.WriteString(text[j+1 : j+1+len(rest)])
		i = j + 1 + len(rest)
	}
	if b.Len() == 0 {
		return text
	}
	b.WriteString(text[i:])
	return b.String()
}

// ssrHasPrefixFold reports whether s starts with prefix, comparing ASCII
// letters without regard to case. prefix must be lowercase ASCII.
func ssrHasPrefixFold(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for k := 0; k < len(prefix); k++ {
		c := s[k]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[k] {
			return false
		}
	}
	return true
}

// ssrRawTextChildren collects the text content of a raw-text element's subtree
// and reports whether tag is a raw-text element whose content is all text.
//
// Components and fragments are transparent, exactly as they are for ordinary
// child emission, so a component that returns a script body works. A host
// element child makes the content non-textual: ok is false and the caller falls
// back to normal escaped rendering rather than inventing an interpretation.
func ssrRawTextChildren(f *fiber, tag string) (string, bool) {
	if !ssrIsRawTextTag(tag) {
		return "", false
	}
	var b strings.Builder
	if !ssrCollectRawText(f, &b) {
		return "", false
	}
	return b.String(), true
}

func ssrCollectRawText(f *fiber, b *strings.Builder) bool {
	for c := f.child; c != nil; c = c.sibling {
		switch c.typ.(type) {
		case textTag:
			s, _ := c.props[textValueKey].(string)
			b.WriteString(s)
		case string:
			// A host element inside <script> or <style>.
			return false
		default:
			if !ssrCollectRawText(c, b) {
				return false
			}
		}
	}
	return true
}
