package react

import (
	"strings"
	"testing"
)

// TestSSRTextSeparator is the whole contract of ssr_textsep.go, and every want
// string in it was produced by react@19.2.7 / react-dom@19.2.7 rather than
// reasoned out: each row was rendered with both ReactDOMServer.renderToString
// and ReactDOMServer.renderToStaticMarkup against the copy of react-dom the
// parity harness installs, and the two columns are those two outputs verbatim.
// That is what makes this table an oracle test rather than a description of the
// port's own behaviour.
func TestSSRTextSeparator(t *testing.T) {
	var greeting Component = func(p Props) Node {
		return ssrTag("span", nil, "hello, ", p.String("who"))
	}

	cases := []struct {
		name string
		node Node
		// wantString is ReactDOMServer.renderToString's output, wantStatic is
		// ReactDOMServer.renderToStaticMarkup's.
		wantString string
		wantStatic string
	}{
		{
			"two adjacent texts are separated",
			ssrTag("div", nil, "a", "b"),
			"<div>a<!-- -->b</div>",
			"<div>ab</div>",
		},
		{
			"three adjacent texts are separated pairwise",
			ssrTag("div", nil, "a", "b", "c"),
			"<div>a<!-- -->b<!-- -->c</div>",
			"<div>abc</div>",
		},
		{
			"numbers are text like any other",
			ssrTag("div", nil, 1, 2),
			"<div>1<!-- -->2</div>",
			"<div>12</div>",
		},
		{
			"a float and a string are still two text nodes",
			ssrTag("div", nil, 1.5, "x"),
			"<div>1.5<!-- -->x</div>",
			"<div>1.5x</div>",
		},
		{
			"a single text needs no separator",
			ssrTag("div", nil, "a"),
			"<div>a</div>",
			"<div>a</div>",
		},
		{
			"a fragment is not a tag, so its texts are adjacent",
			ssrFrag("a", "b"),
			"a<!-- -->b",
			"ab",
		},
		{
			"nested fragments flatten into one run of adjacent texts",
			ssrFrag("a", ssrFrag("b", "c"), "d"),
			"a<!-- -->b<!-- -->c<!-- -->d",
			"abcd",
		},
		{
			"a component boundary is not a tag either",
			ssrTag("div", nil, ssrComp(greeting, Props{"who": "world"})),
			"<div><span>hello, <!-- -->world</span></div>",
			"<div><span>hello, world</span></div>",
		},
		{
			"an element between two texts separates them itself",
			ssrTag("div", nil, "a", ssrTag("b", nil, "x"), "c"),
			"<div>a<b>x</b>c</div>",
			"<div>a<b>x</b>c</div>",
		},
		{
			"the end tag resets the run, the text after it does not",
			ssrTag("div", nil, "a", ssrTag("b", nil, "x"), "c", "d"),
			"<div>a<b>x</b>c<!-- -->d</div>",
			"<div>a<b>x</b>cd</div>",
		},
		{
			"a void element ends the run without a closing tag",
			ssrTag("div", nil, "a", ssrTag("br", nil), "b"),
			"<div>a<br/>b</div>",
			"<div>a<br/>b</div>",
		},
		{
			"the flag is document order, not per parent",
			ssrTag("div", nil, "a", ssrTag("span", nil, "b", "c"), "d"),
			"<div>a<span>b<!-- -->c</span>d</div>",
			"<div>a<span>bc</span>d</div>",
		},
		{
			"a start tag resets the run for its own first child",
			ssrFrag(ssrTag("div", nil, "a", "b"), ssrTag("div", nil, "c", "d")),
			"<div>a<!-- -->b</div><div>c<!-- -->d</div>",
			"<div>ab</div><div>cd</div>",
		},
		{
			"text after a closed element is not adjacent to anything",
			ssrFrag(ssrTag("i", nil, "x"), "a"),
			"<i>x</i>a",
			"<i>x</i>a",
		},
		{
			"children that vanish do not break adjacency",
			ssrTag("div", nil, "a", nil, false, "b"),
			"<div>a<!-- -->b</div>",
			"<div>ab</div>",
		},
		{
			"an empty text emits nothing and decides nothing",
			ssrTag("div", nil, "a", "", "b"),
			"<div>a<!-- -->b</div>",
			"<div>ab</div>",
		},
		{
			"a leading empty text starts no run",
			ssrTag("div", nil, "", "a"),
			"<div>a</div>",
			"<div>a</div>",
		},
		{
			"the separator goes before the escaped text, not the raw text",
			ssrTag("div", nil, "a<b", "&c"),
			"<div>a&lt;b<!-- -->&amp;c</div>",
			"<div>a&lt;b&amp;c</div>",
		},
		{
			"SVG text separates like HTML text",
			ssrTag("svg", nil, "a", "b"),
			"<svg>a<!-- -->b</svg>",
			"<svg>ab</svg>",
		},
		{
			"raw text content is one string, never two text nodes",
			ssrTag("script", nil, "a<b"),
			"<script>a<b</script>",
			"<script>a<b</script>",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderToString(c.node)
			if err != nil {
				t.Fatalf("RenderToString: unexpected error: %v", err)
			}
			if got != c.wantString {
				t.Errorf("RenderToString = %q, want %q", got, c.wantString)
			}

			static, err := RenderToStaticMarkup(c.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: unexpected error: %v", err)
			}
			if static != c.wantStatic {
				t.Errorf("RenderToStaticMarkup = %q, want %q", static, c.wantStatic)
			}
		})
	}
}

// TestSSRTextSeparatorRenderToWriter checks that the streaming primitive is the
// separating one: RenderToString is a thin wrapper over it, so a caller writing
// straight to an http.ResponseWriter must get the same bytes rather than
// accidentally getting static markup.
func TestSSRTextSeparatorRenderToWriter(t *testing.T) {
	var b strings.Builder
	if err := RenderToWriter(&b, ssrTag("div", nil, "a", "b")); err != nil {
		t.Fatalf("RenderToWriter: unexpected error: %v", err)
	}
	if want := "<div>a<!-- -->b</div>"; b.String() != want {
		t.Errorf("RenderToWriter wrote %q, want %q", b.String(), want)
	}
}

// TestSSRTextSeparatorPortalsAreStatic pins the decision documented on
// [RenderPortalsToString]: split markup cannot be hydrated as one tree, so no
// separator is written in either half, and neither half's run of text leaks into
// the other.
func TestSSRTextSeparatorPortalsAreStatic(t *testing.T) {
	page := ssrTag("div", nil,
		"a",
		CreatePortal(ssrFrag("p", "q"), "modal"),
		"b",
	)

	main, portals, err := RenderPortalsToString(page)
	if err != nil {
		t.Fatalf("RenderPortalsToString: unexpected error: %v", err)
	}
	if want := "<div>ab</div>"; main != want {
		t.Errorf("main = %q, want %q", main, want)
	}
	if want := "pq"; portals["modal"] != want {
		t.Errorf("portals[modal] = %q, want %q", portals["modal"], want)
	}
}
