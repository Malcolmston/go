package react

import (
	"strings"
	"testing"
)

// The documentation makes byte-level promises. API-DEVIATIONS.md, README.md,
// CHANGELOG.md and doc.go all quote exact markup for the two render modes, for
// attribute ordering, and for Float hoisting — and prose that quotes bytes rots
// silently, because nothing compiles it.
//
// Every case below is a claim some document makes, pinned to the renderer that
// is supposed to back it. The point is not to re-test the emitter: ssr_textsep.go,
// ssr_order.go, ssr_preload.go and ssr_hoist.go each have their own tests with
// the oracle probes quoted in them. The point is that if one of those rules
// changes, the test that fails names the document that now lies.
//
// Every want string here was read off a probe against the pinned oracle,
// react-dom@19.2.7, renderToString / renderToStaticMarkup — not off the port,
// and not from memory.

// TestDocClaimsRenderModes pins the "the two render modes are not aliases"
// section of API-DEVIATIONS.md, the matching README block, and the doc.go and
// CHANGELOG examples.
func TestDocClaimsRenderModes(t *testing.T) {
	tests := []struct {
		name string
		node Node
		// rts is RenderToString: React's renderToString, separator included.
		rts string
		// rtsm is RenderToStaticMarkup: React's renderToStaticMarkup, no
		// separator. Equal to rts whenever the tree has no adjacent text.
		rtsm string
		// doc names the claim this case exists to defend.
		doc string
	}{
		{
			name: "adjacent text separates in RenderToString only",
			node: H("div", nil, "a", "b"),
			rts:  "<div>a<!-- -->b</div>",
			rtsm: "<div>ab</div>",
			doc:  "the headline example in all four documents",
		},
		{
			name: "adjacent numbers separate the same way",
			node: H("div", nil, 1, 2),
			rts:  "<div>1<!-- -->2</div>",
			rtsm: "<div>12</div>",
			doc:  "adjacency is about text nodes, not about strings",
		},
		{
			name: "a fragment's texts are adjacent",
			node: Frag("a", "b"),
			rts:  "a<!-- -->b",
			rtsm: "ab",
			doc:  "a fragment is transparent, so its children are adjacent",
		},
		{
			name: "a tag ends the run of text",
			node: H("div", nil, "a", H("b", nil, "x"), "c"),
			rts:  "<div>a<b>x</b>c</div>",
			rtsm: "<div>a<b>x</b>c</div>",
			doc:  "API-DEVIATIONS: only adjacent text separates",
		},
		{
			name: "an empty text node is not a boundary",
			node: H("div", nil, "a", "", "b"),
			rts:  "<div>a<!-- -->b</div>",
			rtsm: "<div>ab</div>",
			doc:  "API-DEVIATIONS: emits nothing and decides nothing",
		},
		{
			name: "the run is tracked in document order, not per parent",
			node: H("div", nil, "a", H("span", nil, "b", "c"), "d"),
			rts:  "<div>a<span>b<!-- -->c</span>d</div>",
			rtsm: "<div>a<span>bc</span>d</div>",
			doc:  "API-DEVIATIONS: separates inside the span but not around it",
		},
		{
			name: "a single text node never separates",
			node: H("div", nil, "a"),
			rts:  "<div>a</div>",
			rtsm: "<div>a</div>",
			doc:  "the separator is a boundary, not a prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToString(tt.node)
			if err != nil {
				t.Fatalf("RenderToString: %v", err)
			}
			if got != tt.rts {
				t.Errorf("RenderToString = %q, want %q\n(claim: %s)", got, tt.rts, tt.doc)
			}

			got, err = RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.rtsm {
				t.Errorf("RenderToStaticMarkup = %q, want %q\n(claim: %s)", got, tt.rtsm, tt.doc)
			}
		})
	}
}

// TestDocClaimsRenderToStringIsNotAnAlias is the claim stated as a claim. The
// table above would still pass if both functions separated; this cannot.
func TestDocClaimsRenderToStringIsNotAnAlias(t *testing.T) {
	node := H("div", nil, "a", "b")

	rts, err := RenderToString(node)
	if err != nil {
		t.Fatalf("RenderToString: %v", err)
	}
	rtsm, err := RenderToStaticMarkup(node)
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	if rts == rtsm {
		t.Fatalf("RenderToString and RenderToStaticMarkup both produced %q; "+
			"every document now claims they differ on adjacent text", rts)
	}
	if !strings.Contains(rts, ssrTextSeparator) {
		t.Errorf("RenderToString = %q, want it to contain the separator %q", rts, ssrTextSeparator)
	}
	if strings.Contains(rtsm, ssrTextSeparator) {
		t.Errorf("RenderToStaticMarkup = %q, want no separator", rtsm)
	}
}

// TestDocClaimsWriterModes pins which mode each of the other entry points
// renders in. README and API-DEVIATIONS both say RenderToWriter renders in
// RenderToString mode and RenderPortalsToString in static-markup mode, and a
// caller picking an entry point is relying on exactly that.
func TestDocClaimsWriterModes(t *testing.T) {
	node := H("div", nil, "a", "b")

	t.Run("RenderToWriter is RenderToString mode", func(t *testing.T) {
		var b strings.Builder
		if err := RenderToWriter(&b, node); err != nil {
			t.Fatalf("RenderToWriter: %v", err)
		}
		if got, want := b.String(), "<div>a<!-- -->b</div>"; got != want {
			t.Errorf("RenderToWriter wrote %q, want %q", got, want)
		}
	})

	t.Run("MustRenderToString is RenderToString mode", func(t *testing.T) {
		if got, want := MustRenderToString(node), "<div>a<!-- -->b</div>"; got != want {
			t.Errorf("MustRenderToString = %q, want %q", got, want)
		}
	})

	t.Run("RenderPortalsToString is static-markup mode", func(t *testing.T) {
		main, portals, err := RenderPortalsToString(node)
		if err != nil {
			t.Fatalf("RenderPortalsToString: %v", err)
		}
		if got, want := main, "<div>ab</div>"; got != want {
			t.Errorf("main = %q, want %q (a split document cannot be hydrated as one tree)", got, want)
		}
		if len(portals) != 0 {
			t.Errorf("portals = %v, want none", portals)
		}
	})
}

// TestDocClaimsAttributeOrder pins the "Attribute output order" section: the
// sorted default, and React's four hand-written per-tag orderings that sit on
// top of it. The order-insertion case is the documented deviation and is
// asserted as a deviation — the port is expected NOT to match React here.
func TestDocClaimsAttributeOrder(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
		doc  string
	}{
		{
			name: "the default pass sorts by prop name",
			node: H("div", Props{"id": "a", "className": "b", "title": "c"}),
			want: `<div class="b" id="a" title="c"></div>`,
			doc:  "API-DEVIATIONS: sorted, because a Go map has no order",
		},
		{
			name: "input defers name past the sorted props",
			node: H("input", Props{"id": "q", "name": "q", "required": true, "type": "text"}),
			want: `<input id="q" required="" type="text" name="q"/>`,
			doc:  "README and CHANGELOG both quote these exact bytes",
		},
		{
			name: "option defers selected",
			node: H("option", Props{"selected": true, "value": "a"}),
			want: `<option value="a" selected=""></option>`,
			doc:  "API-DEVIATIONS: React's option serializer emits selected after value",
		},
		{
			name: "form defers action and method",
			node: H("form", Props{"acceptCharset": "utf-8", "action": "/a", "method": "post"}),
			want: `<form accept-charset="utf-8" action="/a" method="post"></form>`,
			doc:  "the deferral is real even where it coincides with sorted order",
		},
		{
			name: "button defers name",
			node: H("button", Props{"name": "n", "type": "submit"}, "Go"),
			want: `<button type="submit" name="n">Go</button>`,
			doc:  "API-DEVIATIONS lists button among the four hand-written serializers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.want {
				t.Errorf("= %q, want %q\n(claim: %s)", got, tt.want, tt.doc)
			}
		})
	}
}

// TestDocClaimsInsertionOrderStaysADeviation is the one documented deviation
// asserted from the deviating side. React renders this tree as
// `<div id="a" class="b" title="c">` because it follows the props object's
// insertion order; the port sorts. If the port ever gains ordered props this
// test fails, which is the signal to rewrite the "Attribute output order"
// section rather than to leave it standing as an excuse.
func TestDocClaimsInsertionOrderStaysADeviation(t *testing.T) {
	// Two authored orders that a JS object would keep apart. A Go map cannot.
	forward := H("div", Props{"id": "a", "className": "b", "title": "c"})
	reverse := H("div", Props{"title": "c", "className": "b", "id": "a"})

	a, err := RenderToStaticMarkup(forward)
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	b, err := RenderToStaticMarkup(reverse)
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	if a != b {
		t.Fatalf("the two authored orders rendered differently (%q vs %q); "+
			"the port has gained prop order, so rewrite the "+
			"\"Attribute output order\" section of API-DEVIATIONS.md", a, b)
	}

	const upstream = `<div id="a" class="b" title="c"></div>`
	const sorted = `<div class="b" id="a" title="c"></div>`
	if a != sorted {
		t.Errorf("= %q, want the documented sorted output %q", a, sorted)
	}
	if a == upstream {
		t.Errorf("= %q, which is React's insertion order; the documented "+
			"deviation no longer exists and API-DEVIATIONS.md must be updated", a)
	}
}

// TestDocClaimsHoistedAttributesKeepPortOrder pins the interaction the hoisting
// section calls out: an element moves to React's position, but its own
// attributes are still written by the ordinary sorted pass. React renders this
// meta as `<meta name="a" content="b"/>`.
func TestDocClaimsHoistedAttributesKeepPortOrder(t *testing.T) {
	got, err := RenderToStaticMarkup(
		H("div", nil, "x", H("meta", Props{"name": "a", "content": "b"}), "y"))
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	const want = `<meta content="b" name="a"/><div>xy</div>`
	if got != want {
		t.Errorf("= %q, want %q\n(claim: the element moves; its attributes keep "+
			"the port's sorted order)", got, want)
	}
}

// TestDocClaimsImagePreloadNotYetFlushed pins the fourth limit in the hoisting
// section: ssr_preload.go decides the preload, the emitter collects it, and
// nothing flushes it until the link's href is sanitized the way React sanitizes
// it. When that lands this test fails, which is the signal to strike the limit
// from API-DEVIATIONS.md and the deviation from the parity case — not to relax
// the assertion.
func TestDocClaimsImagePreloadNotYetFlushed(t *testing.T) {
	got, err := RenderToStaticMarkup(H("img", Props{"alt": "a", "src": "/a.png"}))
	if err != nil {
		t.Fatalf("RenderToStaticMarkup: %v", err)
	}
	const want = `<img alt="a" src="/a.png"/>`
	if got != want {
		t.Errorf("= %q, want %q\n(claim: image preloads are decided but not yet "+
			"flushed; React emits "+
			`<link rel="preload" as="image" href="/a.png"/>`+" first)", got, want)
	}
}

// TestDocClaimsHoisting pins the Float examples quoted in the "Document metadata
// and resource hoisting" section, including the four things that look hoistable
// and are not. Copying the oracle rather than generalizing from "it is a head
// tag" is the documented rule; these are the cases that make it a rule.
func TestDocClaimsHoisting(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
		doc  string
	}{
		{
			name: "a meta written in the body is hoisted ahead of it",
			node: H("div", nil, "x", H("meta", Props{"name": "a"}), "y"),
			want: `<meta name="a"/><div>xy</div>`,
			doc:  "API-DEVIATIONS quotes this probe verbatim",
		},
		{
			name: "a title written in the body is hoisted ahead of it",
			node: H("div", nil, "x", H("title", nil, "T"), "y"),
			want: `<title>T</title><div>xy</div>`,
			doc:  "API-DEVIATIONS quotes this probe verbatim",
		},
		{
			name: "an html with no head gets one synthesized to hold the hoist",
			node: H("html", nil, H("body", nil, H("meta", Props{"name": "x"}))),
			want: `<html><head><meta name="x"/></head><body></body></html>`,
			doc:  "API-DEVIATIONS quotes this probe verbatim",
		},
		{
			name: "base is not head-managed",
			node: H("div", nil, H("base", Props{"href": "/"})),
			want: `<div><base href="/"/></div>`,
			doc:  "API-DEVIATIONS: base stays where it is written",
		},
		{
			name: "a stylesheet link without precedence stays put",
			node: H("div", nil, H("link", Props{"rel": "stylesheet"})),
			want: `<div><link rel="stylesheet"/></div>`,
			doc:  "API-DEVIATIONS: React only takes ownership once you opt in",
		},
		{
			name: "a style without precedence stays put",
			node: H("div", nil, H("style", nil, "a{}")),
			want: `<div><style>a{}</style></div>`,
			doc:  "API-DEVIATIONS: same opt-in as the stylesheet link",
		},
		{
			name: "charSet flushes ahead of a generic meta",
			node: H("div", nil, H("meta", Props{"name": "a"}), H("meta", Props{"charSet": "utf-8"})),
			want: `<meta charSet="utf-8"/><meta name="a"/><div></div>`,
			doc:  "API-DEVIATIONS: charSet first, then viewport, then the rest",
		},
		{
			name: "an itemProp pins the element in place",
			node: H("div", nil, H("meta", Props{"itemProp": "x"})),
			want: `<div><meta itemProp="x"/></div>`,
			doc:  "API-DEVIATIONS: moving microdata would change what it says",
		},
		{
			name: "an svg title is not a document title",
			node: H("svg", nil, H("title", nil, "T")),
			want: `<svg><title>T</title></svg>`,
			doc:  "API-DEVIATIONS: the exceptions are rules, not oversights",
		},
		{
			name: "nothing hoists out of a noscript",
			node: H("noscript", nil, H("meta", Props{"name": "a"})),
			want: `<noscript><meta name="a"/></noscript>`,
			doc:  "API-DEVIATIONS: the exceptions are rules, not oversights",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.node)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tt.want {
				t.Errorf("= %q, want %q\n(claim: %s)", got, tt.want, tt.doc)
			}
		})
	}
}
