package react

import (
	"strings"
	"testing"
	"unsafe"
)

// TestSSREscapeText pins every rule the escaper implements, one case per rule.
// The expectations are not invented: each was produced by rendering the same
// input through react-dom 19.2.7 (the pinned oracle in parity/react/node) and
// reading the bytes back out.
func TestSSREscapeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty string is returned as is", "", ""},
		{"plain text is untouched", "hello world", "hello world"},
		{"ampersand", "a & b", "a &amp; b"},
		{"less than", "a < b", "a &lt; b"},
		{"greater than", "a > b", "a &gt; b"},
		{"double quote", `say "hi"`, "say &quot;hi&quot;"},
		{"apostrophe uses the hex numeric form", "it's", "it&#x27;s"},
		{"all five at once", `&<>"'`, "&amp;&lt;&gt;&quot;&#x27;"},

		// Escaping is a single pass over the input, never over its own output:
		// an ampersand that already begins an entity is escaped once, so text
		// round-trips through a browser back to the string the caller wrote.
		{"an already escaped entity is escaped again", "&amp;", "&amp;amp;"},
		{"an entity lookalike is not special", "&nbsp;", "&amp;nbsp;"},
		{"a numeric reference is not special", "&#x27;", "&amp;#x27;"},

		// Only the five characters change. Anything else, including characters
		// a more aggressive escaper might reach for, passes through verbatim.
		{"non-ascii text is not escaped", "café — 日本語 🙂", "café — 日本語 🙂"},
		{"non-breaking space stays a raw rune", "a b", "a b"},
		{"backtick and equals are not escaped", "a=`b`", "a=`b`"},
		{"newlines and tabs survive", "a\n\tb", "a\n\tb"},
		{"slash is not escaped", "</", "&lt;/"},

		{"repeats are all replaced", "'''", "&#x27;&#x27;&#x27;"},
		{"escapable characters at both ends", "&x&", "&amp;x&amp;"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrEscapeText(c.in); got != c.want {
				t.Errorf("ssrEscapeText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSSREscapeTextFastPathIdentity checks the optimization rather than the
// output: a string with nothing to escape must come back as the very same
// string, not a rebuilt copy. Comparing the data pointers is the only way to
// see the difference, and a regression here is invisible to every other test
// because the bytes stay correct while every text node starts allocating.
func TestSSREscapeTextFastPathIdentity(t *testing.T) {
	in := strings.Repeat("plain text ", 8)
	got := ssrEscapeText(in)
	if got != in {
		t.Fatalf("ssrEscapeText(%q) = %q, want it unchanged", in, got)
	}
	if unsafe.StringData(got) != unsafe.StringData(in) {
		t.Errorf("ssrEscapeText copied a string that needed no escaping")
	}
}

// TestSSREscapeTextInvalidUTF8 documents that the escaper is byte-oriented.
// Ranging over runes would replace each invalid byte with U+FFFD, quietly
// corrupting data the caller may not control; copying bytes passes it through.
func TestSSREscapeTextInvalidUTF8(t *testing.T) {
	in := "a\xffb'"
	want := "a\xffb&#x27;"
	if got := ssrEscapeText(in); got != want {
		t.Errorf("ssrEscapeText(%q) = %q, want %q", in, got, want)
	}
}

// TestSSREscapeTextInRenderedMarkup ties the escaper to the two places it is
// actually reached from: element content and a double-quoted attribute value.
// React escapes both positions identically, so these expectations are the same
// bytes in two contexts.
func TestSSREscapeTextInRenderedMarkup(t *testing.T) {
	cases := []struct {
		name string
		el   *Element
		want string
	}{
		{
			"all five in text",
			ssrTag("div", nil, `&<>"'`),
			`<div>&amp;&lt;&gt;&quot;&#x27;</div>`,
		},
		{
			"apostrophe in text",
			ssrTag("div", nil, "it's"),
			`<div>it&#x27;s</div>`,
		},
		{
			"all five in an attribute value",
			ssrTag("div", Props{"title": `&<>"'`}),
			`<div title="&amp;&lt;&gt;&quot;&#x27;"></div>`,
		},
		{
			"apostrophe in an attribute value",
			ssrTag("div", Props{"title": "it's"}),
			`<div title="it&#x27;s"></div>`,
		},
		{
			"apostrophe inside a style value takes the same route",
			ssrTag("div", Props{"style": map[string]any{"backgroundImage": "url('x')"}}),
			`<div style="background-image:url(&#x27;x&#x27;)"></div>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(c.el)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != c.want {
				t.Errorf("RenderToStaticMarkup = %q, want %q", got, c.want)
			}
		})
	}
}
