package react

import (
	"strings"
	"testing"
)

// Every want string in this file is a byte-for-byte probe render from the pinned
// oracle, react-dom@19.2.7, except where a comment says otherwise: the port sorts
// attributes where React keeps insertion order, so a case with two or more
// attributes on a hoisted element cannot match React byte for byte and is noted
// where it appears.

func TestSSRHoistClassify(t *testing.T) {
	tests := []struct {
		name  string
		tag   string
		props Props
		want  ssrHoistKind
	}{
		// Tags Float never looks at.
		{"div", "div", nil, ssrHoistNone},
		{"base", "base", Props{"href": "/"}, ssrHoistNone},
		{"img", "img", Props{"src": "/a.png"}, ssrHoistNone},

		// meta.
		{"meta bare", "meta", nil, ssrHoistGeneric},
		{"meta named", "meta", Props{"name": "x", "content": "y"}, ssrHoistGeneric},
		{"meta charSet", "meta", Props{"charSet": "utf-8"}, ssrHoistCharset},
		{"meta charSet not a string", "meta", Props{"charSet": 8}, ssrHoistGeneric},
		{"meta lowercase charset", "meta", Props{"charset": "utf-8"}, ssrHoistGeneric},
		{"meta viewport", "meta", Props{"name": "viewport", "content": "w"}, ssrHoistViewport},
		{"meta viewport is case sensitive", "meta", Props{"name": "VIEWPORT"}, ssrHoistGeneric},
		{"meta itemProp", "meta", Props{"itemProp": "x", "content": "y"}, ssrHoistNone},
		{"meta itemProp empty still pins", "meta", Props{"itemProp": "", "name": "x"}, ssrHoistNone},
		{"meta itemProp false still pins", "meta", Props{"itemProp": false, "name": "x"}, ssrHoistNone},
		{"meta itemProp nil does not pin", "meta", Props{"itemProp": nil, "name": "x"}, ssrHoistGeneric},

		// title.
		{"title", "title", nil, ssrHoistTitle},
		{"title itemProp", "title", Props{"itemProp": "x"}, ssrHoistNone},

		// link.
		{"link bare", "link", nil, ssrHoistNone},
		{"link href only", "link", Props{"href": "/x"}, ssrHoistNone},
		{"link rel only", "link", Props{"rel": "icon"}, ssrHoistNone},
		{"link empty href", "link", Props{"rel": "icon", "href": ""}, ssrHoistNone},
		{"link rel not a string", "link", Props{"rel": 1, "href": "/x"}, ssrHoistNone},
		{"link icon", "link", Props{"rel": "icon", "href": "/f.ico"}, ssrHoistGeneric},
		{"link empty rel", "link", Props{"rel": "", "href": "/x"}, ssrHoistGeneric},
		{"link preload", "link", Props{"rel": "preload", "as": "image", "href": "/a.png"}, ssrHoistGeneric},
		{"link preconnect", "link", Props{"rel": "preconnect", "href": "https://x"}, ssrHoistGeneric},
		{"link modulepreload", "link", Props{"rel": "modulepreload", "href": "/m.js"}, ssrHoistGeneric},
		{"link onLoad pins", "link", Props{"rel": "icon", "href": "/f", "onLoad": func() {}}, ssrHoistNone},
		{"link onError pins", "link", Props{"rel": "icon", "href": "/f", "onError": func() {}}, ssrHoistNone},
		{"link itemProp pins", "link", Props{"rel": "icon", "href": "/f", "itemProp": "q"}, ssrHoistNone},
		{"sheet without precedence", "link", Props{"rel": "stylesheet", "href": "/a.css"}, ssrHoistNone},
		{"sheet with precedence", "link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p"}, ssrHoistSheet},
		{"sheet empty precedence", "link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": ""}, ssrHoistSheet},
		{"sheet precedence not a string", "link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": 1}, ssrHoistNone},
		{"sheet disabled", "link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p", "disabled": true}, ssrHoistNone},
		{"sheet disabled false still pins", "link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p", "disabled": false}, ssrHoistNone},
		{"sheet uppercase rel is generic", "link", Props{"rel": "STYLESHEET", "href": "/a.css"}, ssrHoistGeneric},

		// script.
		{"script async src", "script", Props{"async": true, "src": "/a.js"}, ssrHoistScript},
		{"script async string", "script", Props{"async": "async", "src": "/a.js"}, ssrHoistScript},
		{"script async false", "script", Props{"async": false, "src": "/a.js"}, ssrHoistNone},
		{"script async func", "script", Props{"async": func() {}, "src": "/a.js"}, ssrHoistNone},
		{"script no async", "script", Props{"src": "/a.js"}, ssrHoistNone},
		{"script defer", "script", Props{"defer": true, "src": "/a.js"}, ssrHoistNone},
		{"script async no src", "script", Props{"async": true}, ssrHoistNone},
		{"script async empty src", "script", Props{"async": true, "src": ""}, ssrHoistNone},
		{"script async src not a string", "script", Props{"async": true, "src": 1}, ssrHoistNone},
		{"script async onLoad", "script", Props{"async": true, "src": "/a.js", "onLoad": func() {}}, ssrHoistNone},
		{"script async itemProp", "script", Props{"async": true, "src": "/a.js", "itemProp": "q"}, ssrHoistNone},

		// style.
		{"style plain", "style", nil, ssrHoistNone},
		{"style precedence only", "style", Props{"precedence": "p"}, ssrHoistNone},
		{"style href only", "style", Props{"href": "s"}, ssrHoistNone},
		{"style managed", "style", Props{"precedence": "p", "href": "s"}, ssrHoistStyle},
		{"style managed itemProp", "style", Props{"precedence": "p", "href": "s", "itemProp": "q"}, ssrHoistNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ssrHoistClassify(tt.tag, tt.props); got != tt.want {
				t.Errorf("ssrHoistClassify(%q, %v) = %d, want %d", tt.tag, tt.props, got, tt.want)
			}
		})
	}
}

func TestSSRHoistAsyncTruthy(t *testing.T) {
	// async is the one prop where a function is not truthy.
	if ssrHoistAsyncTruthy(func() {}) {
		t.Error("a function-valued async must not hoist a script")
	}
	if !ssrHoistAsyncTruthy("async") {
		t.Error(`async="async" must hoist a script`)
	}
	if ssrHoistAsyncTruthy(false) {
		t.Error("async={false} must not hoist a script")
	}
	if ssrHoistAsyncTruthy(nil) {
		t.Error("a missing async must not hoist a script")
	}
}

func TestSSRHoistTruthy(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"nil", nil, false},
		{"true", true, true},
		{"false", false, false},
		{"empty string", "", false},
		{"string", "async", true},
		{"zero int", 0, false},
		{"int", 1, true},
		{"negative int", -1, true},
		{"zero float", 0.0, false},
		{"float", 0.5, true},
		{"func", func() {}, true},
		{"struct", struct{}{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ssrHoistTruthy(tt.value); got != tt.want {
				t.Errorf("ssrHoistTruthy(%#v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestSSRHoistRenderedOutput(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			"a bare meta leaves its parent",
			H("div", nil, H("meta", nil)),
			"<meta/><div></div>",
		},
		{
			"a title leaves its parent",
			H("div", nil, H("title", nil, "T")),
			"<title>T</title><div></div>",
		},
		{
			"an async script leaves its parent",
			H("div", nil, H("script", Props{"async": true, "src": "/a.js"})),
			`<script async="" src="/a.js"></script><div></div>`,
		},
		{
			"a non-async script stays",
			H("div", nil, H("script", Props{"src": "/a.js"})),
			`<div><script src="/a.js"></script></div>`,
		},
		{
			"an inline script stays",
			H("div", nil, H("script", nil, "var a=1")),
			"<div><script>var a=1</script></div>",
		},
		{
			"base is not head-managed",
			H("div", nil, H("base", Props{"href": "/"})),
			`<div><base href="/"/></div>`,
		},
		{
			"a bare link stays",
			H("div", nil, H("link", nil)),
			"<div><link/></div>",
		},
		{
			"a stylesheet without precedence stays",
			// Attributes sorted, not in React's insertion order: React writes
			// rel before href here.
			H("div", nil, H("link", Props{"rel": "stylesheet", "href": "/a.css"})),
			`<div><link href="/a.css" rel="stylesheet"/></div>`,
		},
		{
			"itemProp pins a meta in place",
			// Sorted attributes again; React writes itemProp first.
			H("div", nil, H("meta", Props{"itemProp": "x", "content": "y"})),
			`<div><meta content="y" itemProp="x"/></div>`,
		},
		{
			"an SVG title is not a document title",
			H("svg", nil, H("title", nil, "T"), H("desc", nil, "D")),
			"<svg><title>T</title><desc>D</desc></svg>",
		},
		{
			"an SVG title nested in HTML is still an SVG title",
			H("div", nil, H("svg", nil, H("title", nil, "T"))),
			"<div><svg><title>T</title></svg></div>",
		},
		{
			"a foreignObject holds HTML, so its title hoists",
			H("svg", nil, H("foreignObject", nil, H("title", nil, "T"))),
			"<title>T</title><svg><foreignObject></foreignObject></svg>",
		},
		{
			"MathML does not pin a meta",
			H("math", nil, H("meta", Props{"name": "x"})),
			`<meta name="x"/><math></math>`,
		},
		{
			"nothing hoists out of a noscript",
			H("noscript", nil, H("meta", Props{"name": "a"})),
			`<noscript><meta name="a"/></noscript>`,
		},
		{
			"a noscript deep in the tree still suppresses",
			H("div", nil, H("noscript", nil, H("title", nil, "T"))),
			"<div><noscript><title>T</title></noscript></div>",
		},
		{
			"a meta beside a noscript still hoists",
			H("div", nil, H("noscript", nil, H("meta", Props{"name": "a"})), H("meta", Props{"name": "b"})),
			`<meta name="b"/><div><noscript><meta name="a"/></noscript></div>`,
		},
		{
			"hoisting reaches arbitrarily deep",
			H("div", nil, H("p", nil, H("span", nil, H("meta", Props{"name": "a"})))),
			`<meta name="a"/><div><p><span></span></p></div>`,
		},
		{
			"relative order is preserved",
			H("div", nil, H("meta", Props{"name": "a"}), H("span", nil, "x"), H("meta", Props{"name": "b"})),
			`<meta name="a"/><meta name="b"/><div><span>x</span></div>`,
		},
		{
			"charset flushes ahead of a plain meta written before it",
			H("div", nil, H("meta", Props{"name": "a"}), H("meta", Props{"charSet": "utf-8"})),
			`<meta charSet="utf-8"/><meta name="a"/><div></div>`,
		},
		{
			"viewport flushes between charset and the generic bucket",
			H("div", nil,
				H("meta", Props{"name": "a"}),
				H("meta", Props{"name": "viewport"}),
				H("meta", Props{"charSet": "utf-8"}),
			),
			`<meta charSet="utf-8"/><meta name="viewport"/><meta name="a"/><div></div>`,
		},
		{
			"an async script flushes ahead of the generic bucket",
			H("div", nil, H("meta", Props{"name": "a"}), H("script", Props{"async": true, "src": "/s.js"})),
			`<script async="" src="/s.js"></script><meta name="a"/><div></div>`,
		},
		{
			"identical metas are not deduplicated",
			H("div", nil, H("meta", Props{"name": "a"}), H("meta", Props{"name": "a"})),
			`<meta name="a"/><meta name="a"/><div></div>`,
		},
		{
			"two titles both hoist",
			H("div", nil, H("title", nil, "A"), H("title", nil, "B")),
			"<title>A</title><title>B</title><div></div>",
		},
		{
			"an async script is deduplicated by src",
			H("div", nil,
				H("script", Props{"async": true, "src": "/a.js"}),
				H("script", Props{"async": true, "src": "/a.js"}),
			),
			`<script async="" src="/a.js"></script><div></div>`,
		},
		{
			"a module script and a classic script with one src are two resources",
			// React writes src before type; the port sorts. Both scripts survive,
			// which is the rule under test.
			H("div", nil,
				H("script", Props{"async": true, "src": "/a.js"}),
				H("script", Props{"async": true, "src": "/a.js", "type": "module"}),
			),
			`<script async="" src="/a.js"></script><script async="" src="/a.js" type="module"></script><div></div>`,
		},
		{
			"a top-level hoistable is just itself",
			H("title", nil, "Hello & goodbye"),
			"<title>Hello &amp; goodbye</title>",
		},
		{
			"an already-empty parent stays empty",
			H("div", nil, "hello", H("meta", Props{"name": "a"})),
			`<meta name="a"/><div>hello</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestSSRHoistManagedStylesheets(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			"precedence becomes data-precedence and the link hoists",
			// React writes data-precedence last; the port sorts attributes.
			H("div", nil, H("link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p"})),
			`<link data-precedence="p" href="/a.css" rel="stylesheet"/><div></div>`,
		},
		{
			"a managed sheet is deduplicated by href alone",
			H("div", nil,
				H("link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p"}),
				H("link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "q"}),
			),
			`<link data-precedence="p" href="/a.css" rel="stylesheet"/><div></div>`,
		},
		{
			"precedence groups flush in first-seen order, not alphabetically",
			H("div", nil,
				H("link", Props{"rel": "stylesheet", "href": "/1.css", "precedence": "zz"}),
				H("link", Props{"rel": "stylesheet", "href": "/2.css", "precedence": "aa"}),
				H("link", Props{"rel": "stylesheet", "href": "/3.css", "precedence": "zz"}),
			),
			`<link data-precedence="zz" href="/1.css" rel="stylesheet"/>` +
				`<link data-precedence="zz" href="/3.css" rel="stylesheet"/>` +
				`<link data-precedence="aa" href="/2.css" rel="stylesheet"/><div></div>`,
		},
		{
			"a group of links alone grows no style element",
			H("div", nil, H("link", Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p"})),
			`<link data-precedence="p" href="/a.css" rel="stylesheet"/><div></div>`,
		},
		{
			"a managed style keeps only its href and its rules",
			H("div", nil, H("style", Props{"precedence": "p", "href": "s1"}, "a{}")),
			`<style data-precedence="p" data-href="s1">a{}</style><div></div>`,
		},
		{
			"styles sharing a precedence merge into one element",
			H("div", nil,
				H("style", Props{"precedence": "p", "href": "s1"}, "a{}"),
				H("style", Props{"precedence": "p", "href": "s2"}, "b{}"),
			),
			`<style data-precedence="p" data-href="s1 s2">a{}b{}</style><div></div>`,
		},
		{
			"a repeated style href contributes nothing",
			H("div", nil,
				H("style", Props{"precedence": "p", "href": "s1"}, "a{}"),
				H("style", Props{"precedence": "p", "href": "s1"}, "b{}"),
			),
			`<style data-precedence="p" data-href="s1">a{}</style><div></div>`,
		},
		{
			"a group's links come before its merged style",
			H("div", nil,
				H("style", Props{"precedence": "p", "href": "s1"}, "a{}"),
				H("link", Props{"rel": "stylesheet", "href": "/l.css", "precedence": "p"}),
			),
			`<link data-precedence="p" href="/l.css" rel="stylesheet"/>` +
				`<style data-precedence="p" data-href="s1">a{}</style><div></div>`,
		},
		{
			"a style without an href stays in place",
			H("div", nil, H("style", Props{"precedence": "p"}, "a{}")),
			`<div><style precedence="p">a{}</style></div>`,
		},
		{
			"an SVG style is never managed",
			H("svg", nil, H("style", Props{"precedence": "p", "href": "s"}, "a{}")),
			`<svg><style href="s" precedence="p">a{}</style></svg>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestSSRHoistDocumentAssembly(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			"a document with a head is untouched when nothing needs moving",
			H("html", Props{"lang": "en"},
				H("head", nil, H("title", nil, "T")),
				H("body", nil, H("div", Props{"id": "root"}, "x")),
			),
			`<html lang="en"><head><title>T</title></head><body><div id="root">x</div></body></html>`,
		},
		{
			"a meta in the body lands at the end of the head",
			H("html", nil,
				H("head", nil, H("title", nil, "T")),
				H("body", nil, H("meta", Props{"name": "b"})),
			),
			`<html><head><title>T</title><meta name="b"/></head><body></body></html>`,
		},
		{
			"a charset written after the title still lands first in the head",
			H("html", nil,
				H("head", nil, H("title", nil, "T"), H("meta", Props{"charSet": "utf-8"})),
				H("body", nil, "b"),
			),
			`<html><head><meta charSet="utf-8"/><title>T</title></head><body>b</body></html>`,
		},
		{
			"an element that stays in the head keeps its place after the hoisted ones",
			H("html", nil,
				H("head", nil, H("script", Props{"src": "/s.js"}), H("title", nil, "T")),
				H("body", nil, "b"),
			),
			`<html><head><title>T</title><script src="/s.js"></script></head><body>b</body></html>`,
		},
		{
			"an html without a head gets one, holding the hoisted elements",
			H("html", nil, H("body", nil, H("meta", Props{"name": "x"}))),
			`<html><head><meta name="x"/></head><body></body></html>`,
		},
		{
			"an html without a head gets an empty one even with nothing to hoist",
			H("html", nil, H("body", nil, "x")),
			"<html><head></head><body>x</body></html>",
		},
		{
			"a head at the root is the document head",
			H("head", nil, H("div", nil), H("meta", Props{"name": "x"})),
			`<head><meta name="x"/><div></div></head>`,
		},
		{
			"a head inside a div is an ordinary element",
			H("div", nil, H("head", nil, H("div", nil), H("meta", Props{"name": "x"}))),
			`<meta name="x"/><div><head><div></div></head></div>`,
		},
		{
			"an html inside a div is an ordinary element",
			H("div", nil, H("html", nil, H("body", nil, H("meta", Props{"name": "x"})))),
			`<meta name="x"/><div><html><body></body></html></div>`,
		},
		{
			"a body at the root gets no synthesized head",
			H("body", nil, H("meta", Props{"name": "x"}), "y"),
			`<meta name="x"/><body>y</body>`,
		},
		{
			"a head with attributes keeps them, and the splice lands after the open tag",
			// Not an oracle byte string: React does not accept a > inside an
			// attribute value unescaped either, and this pins the open-tag scan.
			H("head", Props{"id": "a>b"}, H("meta", Props{"name": "x"})),
			`<head id="a&gt;b"><meta name="x"/></head>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestSSRHoistTextSeparator(t *testing.T) {
	// React writes the hydration separator in place of a hoisted meta, link,
	// script or style, so two text nodes that were not adjacent do not merge.
	// A hoisted title is the exception: it writes none, but it still ends the run
	// of text, which is why "a" and "b" come out as "ab" and not "a<!-- -->b".
	tests := []struct {
		name string
		node Node
		want string
	}{
		{"meta between two texts", H("div", nil, "a", H("meta", Props{"name": "x"}), "b"), `<meta name="x"/><div>a<!-- -->b</div>`},
		{"meta after a text", H("div", nil, "a", H("meta", Props{"name": "x"})), `<meta name="x"/><div>a<!-- --></div>`},
		{"meta before a text", H("div", nil, H("meta", Props{"name": "x"}), "b"), `<meta name="x"/><div>b</div>`},
		{"link between two texts", H("div", nil, "a", H("link", Props{"rel": "icon", "href": "/x"}), "b"), `<link href="/x" rel="icon"/><div>a<!-- -->b</div>`},
		{"script between two texts", H("div", nil, "a", H("script", Props{"async": true, "src": "/a.js"}), "b"), `<script async="" src="/a.js"></script><div>a<!-- -->b</div>`},
		{"style between two texts", H("div", nil, "a", H("style", Props{"precedence": "p", "href": "s"}, "x{}"), "b"), `<style data-precedence="p" data-href="s">x{}</style><div>a<!-- -->b</div>`},
		{"title between two texts", H("div", nil, "a", H("title", nil, "T"), "b"), "<title>T</title><div>ab</div>"},
		{"title after a text", H("div", nil, "a", H("title", nil, "T")), "<title>T</title><div>a</div>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToString(tt.node)
			if err != nil {
				t.Fatalf("RenderToString: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestSSRHoistStaticMarkupWritesNoSeparator(t *testing.T) {
	got, err := RenderToStaticMarkup(H("div", nil, "a", H("meta", Props{"name": "x"}), "b"))
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	if want := `<meta name="x"/><div>ab</div>`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestSSRHoistCandidateTreeGatesBuffering(t *testing.T) {
	// A tree with nothing head-managed in it must still stream: the buffering the
	// hoisting path needs is a cost only the trees that need it pay.
	tests := []struct {
		name    string
		node    Node
		streams bool
	}{
		{"plain elements", H("div", nil, H("b", nil, "x")), true},
		{"text only", H("p", nil, "hello"), true},
		{"a nested meta", H("div", nil, H("p", nil, H("meta", nil))), false},
		{"a head", H("html", nil, H("head", nil)), false},
		{"an img, whose preload is Float's too", H("div", nil, H("img", Props{"src": "/a.png"})), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chunks []string
			w := ssrWriterFunc(func(p []byte) (int, error) {
				chunks = append(chunks, string(p))
				return len(p), nil
			})
			if err := RenderToWriter(w, tt.node); err != nil {
				t.Fatalf("RenderToWriter: %v", err)
			}
			// The streaming path writes a chunk per tag and per text run; the
			// hoisting path assembles and writes once.
			if streamed := len(chunks) > 1; streamed != tt.streams {
				t.Errorf("got %d writes (streamed = %v), want streamed = %v: %q",
					len(chunks), streamed, tt.streams, strings.Join(chunks, ""))
			}
		})
	}
}

func TestSSRHoistThroughRenderToWriter(t *testing.T) {
	var b strings.Builder
	if err := RenderToWriter(&b, H("div", nil, H("meta", Props{"name": "x"}), "y")); err != nil {
		t.Fatalf("RenderToWriter: %v", err)
	}
	if want := `<meta name="x"/><div>y</div>`; b.String() != want {
		t.Errorf("got %s, want %s", b.String(), want)
	}
}

func TestSSRHoistReportsErrorsFromHoistedElements(t *testing.T) {
	// A hoisted element is still validated: a void element with children is an
	// error whether it was written in place or diverted into the buffer.
	_, err := RenderToStaticMarkup(H("div", nil, H("meta", Props{"name": "x"}, "content")))
	if err == nil {
		t.Fatal("expected an error for a void element with children")
	}
	if !strings.Contains(err.Error(), "void element") {
		t.Errorf("error = %v, want it to name the void element problem", err)
	}
}

func TestSSRHoistLeavesPortalOutputAlone(t *testing.T) {
	// A container's buffer is its own stream. Nothing in it is rearranged, and
	// nothing in it leaks into the document's hoisted chunks.
	main, portals, err := renderSeparatingPortals(H("div", nil,
		H("meta", Props{"name": "a"}),
		CreatePortal(H("meta", Props{"name": "b"}), "slot"),
	))
	if err != nil {
		t.Fatalf("renderSeparatingPortals: %v", err)
	}
	if want := `<meta name="a"/><div></div>`; main != want {
		t.Errorf("main = %s, want %s", main, want)
	}
	if want := `<meta name="b"/>`; portals["slot"] != want {
		t.Errorf("portals[slot] = %s, want %s", portals["slot"], want)
	}
}

func TestSSRHoistAfterOpenTag(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		start int
		want  int
	}{
		{"bare tag", "<head>x", 0, 6},
		{"with attributes", `<head id="a">x`, 0, 13},
		{"escaped angle in a value", `<head id="a&gt;b">x`, 0, 18},
		{"offset start", `<html><head>x`, 6, 12},
		{"unterminated", "<head", 0, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ssrHoistAfterOpenTag(tt.s, tt.start); got != tt.want {
				t.Errorf("ssrHoistAfterOpenTag(%q, %d) = %d, want %d", tt.s, tt.start, got, tt.want)
			}
		})
	}
}

func TestSSRHoistSheetPropsRenamesPrecedence(t *testing.T) {
	in := Props{"rel": "stylesheet", "href": "/a.css", "precedence": "p"}
	out := ssrHoistSheetProps(in)
	if _, present := out["precedence"]; present {
		t.Error("precedence survived the rename")
	}
	if got := out["data-precedence"]; got != "p" {
		t.Errorf("data-precedence = %v, want %q", got, "p")
	}
	if got := out["href"]; got != "/a.css" {
		t.Errorf("href = %v, want %q", got, "/a.css")
	}
	// The copy must not disturb the fiber's own props.
	if _, present := in["data-precedence"]; present {
		t.Error("the original props were modified")
	}
	if in["precedence"] != "p" {
		t.Error("the original precedence was removed")
	}
}
