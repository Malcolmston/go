package react

import "testing"

// The two escape sequences React writes, spelled out once so the test tables
// cannot disagree with themselves about how many backslashes there are.
const (
	testScriptS = "\\u0073" // backslash u 0 0 7 3
	testScriptU = "\\u0053"
	testStyleS  = "\\73 " // backslash 7 3 space
	testStyleU  = "\\53 "
)

func TestSSRIsRawTextTag(t *testing.T) {
	raw := []string{"script", "style"}
	notRaw := []string{"textarea", "title", "noscript", "pre", "div", "p", "SCRIPT", "STYLE", "", "iframe"}

	for _, tag := range raw {
		if !ssrIsRawTextTag(tag) {
			t.Errorf("ssrIsRawTextTag(%q) = false, want true", tag)
		}
	}
	// The tag names reaching the emitter are the ones the caller wrote, and React
	// only treats the canonical lowercase names as raw text.
	for _, tag := range notRaw {
		if ssrIsRawTextTag(tag) {
			t.Errorf("ssrIsRawTextTag(%q) = true, want false", tag)
		}
	}
}

func TestSSRRawTextContent(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		in   string
		want string
	}{
		// No entity escaping anywhere: this is the whole point.
		{"script keeps the five", "script", `var a = 1 < 2 && "x" + 'y' & z > w;`, `var a = 1 < 2 && "x" + 'y' & z > w;`},
		{"script keeps entities", "script", "a = '&amp;'", "a = '&amp;'"},
		{"style keeps the five", "style", `a{content:'&<>"'}`, `a{content:'&<>"'}`},

		// Tag breakout, both prefixes.
		{"script close tag", "script", "a</script>b", "a</" + testScriptS + "cript>b"},
		{"script open tag", "script", "a<script>b", "a<" + testScriptS + "cript>b"},
		{"script both", "script", "a</script>b<script>c",
			"a</" + testScriptS + "cript>b<" + testScriptS + "cript>c"},
		{"script uppercase", "script", "a</SCRIPT>b", "a</" + testScriptU + "CRIPT>b"},
		{"script mixed case keeps suffix case", "script", "a</ScRiPt>b", "a</" + testScriptU + "cRiPt>b"},
		{"script lowercase s upper rest", "script", "a</sCRIPT>b", "a</" + testScriptS + "CRIPT>b"},
		{"style close tag", "style", "a</style>b", "a</" + testStyleS + "tyle>b"},
		{"style open tag", "style", "a<style>b", "a<" + testStyleS + "tyle>b"},
		{"style uppercase", "style", "a</STYLE>b", "a</" + testStyleU + "TYLE>b"},

		// Near misses: the regex requires the whole tag name right after the "<"
		// or "</", with nothing in between.
		{"script truncated name", "script", "a<scr b</scr c", "a<scr b</scr c"},
		{"script space before name", "script", "a</ script>b", "a</ script>b"},
		{"script slash slash", "script", "a<//script>b", "a<//script>b"},
		{"script bare lt", "script", "a < b", "a < b"},
		{"script trailing lt", "script", "a<", "a<"},
		{"script trailing lt slash", "script", "a</", "a</"},
		{"script other tag", "script", "a</style>b", "a</style>b"},
		{"style other tag", "style", "a</script>b", "a</script>b"},

		// A doubled "<" must not hide a match at the second one.
		{"script doubled lt", "script", "a<<script>b", "a<<" + testScriptS + "cript>b"},
		{"script lt slash then tag", "script", "a</<script>b", "a</<" + testScriptS + "cript>b"},

		// Adjacent matches, and a match at the very start and end.
		{"script back to back", "script", "<script></script>",
			"<" + testScriptS + "cript></" + testScriptS + "cript>"},

		// Multibyte content is untouched and does not confuse the byte scan.
		{"script multibyte", "script", "var s = 'héllo → ✓ </script>';",
			"var s = 'héllo → ✓ </" + testScriptS + "cript>';"},

		// Empty input, and tags that are not raw text at all.
		{"script empty", "script", "", ""},
		{"non raw tag untouched", "div", "a</script>b", "a</script>b"},
		{"textarea untouched", "textarea", "a<b&c", "a<b&c"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ssrRawTextContent(tc.tag, tc.in); got != tc.want {
				t.Errorf("ssrRawTextContent(%q, %q):\n got %q\nwant %q", tc.tag, tc.in, got, tc.want)
			}
		})
	}
}

// TestSSRRawTextRendering renders whole trees, which is the level the parity
// harness compares at. Each want was produced by react-dom 19.2.7.
func TestSSRRawTextRendering(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			// parity case esc-text-script-inside-script.
			name: "script body is not escaped",
			node: CreateElement("script", nil, "var a = 1 < 2;"),
			want: "<script>var a = 1 < 2;</script>",
		},
		{
			name: "script body with all five characters",
			node: CreateElement("script", nil, `var a = 1 < 2 && "x" + 'y';`),
			want: `<script>var a = 1 < 2 && "x" + 'y';</script>`,
		},
		{
			name: "script breakout is neutralized not escaped",
			node: CreateElement("script", nil, "a</script>b<script>c"),
			want: "<script>a</" + testScriptS + "cript>b<" + testScriptS + "cript>c</script>",
		},
		{
			name: "style body is not escaped",
			node: CreateElement("style", nil, "a{content:'&'}"),
			want: "<style>a{content:'&'}</style>",
		},
		{
			name: "style breakout is neutralized",
			node: CreateElement("style", nil, "a</style>b"),
			want: "<style>a</" + testStyleS + "tyle>b</style>",
		},
		{
			// Probed: React renders an SVG <style> raw too.
			name: "style inside svg is still raw",
			node: CreateElement("svg", nil, CreateElement("style", nil, "a{content:'&<>'}")),
			want: "<svg><style>a{content:'&<>'}</style></svg>",
		},
		{
			name: "empty script",
			node: CreateElement("script", nil),
			want: "<script></script>",
		},
		{
			name: "script with attributes and no body",
			node: CreateElement("script", Props{"src": "/a.js", "type": "module"}),
			want: `<script src="/a.js" type="module"></script>`,
		},
		{
			// The escaping rule is the element's, not the tree's: a script's
			// sibling text still escapes.
			name: "sibling text still escapes",
			node: CreateElement("div", nil,
				CreateElement("script", nil, "a < b"),
				"a < b",
			),
			want: "<div><script>a < b</script>a &lt; b</div>",
		},
		{
			// Probed: textarea, title, noscript and pre all escape. The <title>
			// leads the output rather than sitting in the div because React's
			// Float machinery hoists a document title out of the tree; the case
			// here is about the escaping, and the hoist is the oracle's own
			// placement (probed against react-dom 19.2.7, renderToStaticMarkup
			// and renderToString agree). See ssr_hoist.go.
			name: "textarea title noscript pre still escape",
			node: CreateElement("div", nil,
				CreateElement("title", nil, "a<b&c"),
				CreateElement("noscript", nil, "a<b&c"),
				CreateElement("pre", nil, "a<b&c"),
			),
			want: "<title>a&lt;b&amp;c</title><div><noscript>a&lt;b&amp;c</noscript><pre>a&lt;b&amp;c</pre></div>",
		},
		{
			// dangerouslySetInnerHTML keeps its own contract: verbatim, with no
			// breakout substitution at all (probed).
			name: "dangerous html in script is fully verbatim",
			node: CreateElement("script", Props{
				ssrDangerousProp: DangerousHTML{HTML: "a</script>b"},
			}),
			want: "<script>a</script>b</script>",
		},
		{
			// Port deviation: React drops non-string children with a dev warning.
			// The port concatenates its text children instead.
			name: "several text children are concatenated",
			node: CreateElement("script", nil, "a<", "/script>b"),
			want: "<script>a</" + testScriptS + "cript>b</script>",
		},
		{
			name: "text through a fragment child",
			node: CreateElement("script", nil, CreateElement(Fragment, nil, "a</script>b")),
			want: "<script>a</" + testScriptS + "cript>b</script>",
		},
		{
			// Also a deviation: React drops an element child; the port renders it
			// with ordinary escaped-child rules rather than silently losing it.
			name: "element child falls back to escaped rendering",
			node: CreateElement("script", nil, CreateElement("span", nil, "a < b")),
			want: "<script><span>a &lt; b</span></script>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tc.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tc.want {
				t.Errorf("\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
