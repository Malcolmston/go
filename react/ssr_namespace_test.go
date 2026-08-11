package react

import (
	"strings"
	"testing"
)

// The attribute tables in this file are not hand-written. They were produced by
// rendering a probe element per candidate prop through the real react-dom@19.2.8
// renderToStaticMarkup and recording the attribute name it emitted, in five
// nesting contexts (a bare <div>, the <svg> root, a <path> inside <svg>, a <div>
// inside <foreignObject> inside <svg>, and an <mi> inside <math>). All five
// contexts agreed for all 471 probed props, and the result was cross-checked
// entry-for-entry against React's internal `aliases` Map read out of the same
// bundle. See ssr_namespace.go for the full account of what that measurement
// established.
//
// Reproduce with:
//
//	mkdir probe && cd probe && npm init -y && npm install react react-dom
//	node probe.mjs   # one element per candidate prop, prints the emitted name

// nsRender renders node and fails the test on error. It duplicates ssr_test.go's
// helper on purpose: this file should be readable and runnable without depending
// on which other test file happens to define what.
func nsRender(t *testing.T, node Node) string {
	t.Helper()
	got, err := RenderToString(node)
	if err != nil {
		t.Fatalf("RenderToString: unexpected error: %v", err)
	}
	return got
}

// nsAttrCase is one measured prop-to-attribute observation.
type nsAttrCase struct {
	prop string
	want string
}

// nsSVGRenames are the props whose emitted attribute name is hyphenated or
// namespaced: the SVG presentation attributes, the font-metric and font-face
// family, the text-decoration metrics, and the XLink/XML namespaced set. These
// are the entries a port that assumes "SVG attributes are just camelCase" gets
// wrong.
var nsSVGRenames = []nsAttrCase{
	{prop: "accentHeight", want: "accent-height"},
	{prop: "alignmentBaseline", want: "alignment-baseline"},
	{prop: "arabicForm", want: "arabic-form"},
	{prop: "baselineShift", want: "baseline-shift"},
	{prop: "capHeight", want: "cap-height"},
	{prop: "clipPath", want: "clip-path"},
	{prop: "clipRule", want: "clip-rule"},
	{prop: "colorInterpolation", want: "color-interpolation"},
	{prop: "colorInterpolationFilters", want: "color-interpolation-filters"},
	{prop: "colorProfile", want: "color-profile"},
	{prop: "colorRendering", want: "color-rendering"},
	{prop: "dominantBaseline", want: "dominant-baseline"},
	{prop: "enableBackground", want: "enable-background"},
	{prop: "fillOpacity", want: "fill-opacity"},
	{prop: "fillRule", want: "fill-rule"},
	{prop: "floodColor", want: "flood-color"},
	{prop: "floodOpacity", want: "flood-opacity"},
	{prop: "fontFamily", want: "font-family"},
	{prop: "fontSize", want: "font-size"},
	{prop: "fontSizeAdjust", want: "font-size-adjust"},
	{prop: "fontStretch", want: "font-stretch"},
	{prop: "fontStyle", want: "font-style"},
	{prop: "fontVariant", want: "font-variant"},
	{prop: "fontWeight", want: "font-weight"},
	{prop: "glyphName", want: "glyph-name"},
	{prop: "glyphOrientationHorizontal", want: "glyph-orientation-horizontal"},
	{prop: "glyphOrientationVertical", want: "glyph-orientation-vertical"},
	{prop: "horizAdvX", want: "horiz-adv-x"},
	{prop: "horizOriginX", want: "horiz-origin-x"},
	{prop: "imageRendering", want: "image-rendering"},
	{prop: "letterSpacing", want: "letter-spacing"},
	{prop: "lightingColor", want: "lighting-color"},
	{prop: "markerEnd", want: "marker-end"},
	{prop: "markerMid", want: "marker-mid"},
	{prop: "markerStart", want: "marker-start"},
	{prop: "overlinePosition", want: "overline-position"},
	{prop: "overlineThickness", want: "overline-thickness"},
	{prop: "paintOrder", want: "paint-order"},
	{prop: "pointerEvents", want: "pointer-events"},
	{prop: "renderingIntent", want: "rendering-intent"},
	{prop: "shapeRendering", want: "shape-rendering"},
	{prop: "stopColor", want: "stop-color"},
	{prop: "stopOpacity", want: "stop-opacity"},
	{prop: "strikethroughPosition", want: "strikethrough-position"},
	{prop: "strikethroughThickness", want: "strikethrough-thickness"},
	{prop: "strokeDasharray", want: "stroke-dasharray"},
	{prop: "strokeDashoffset", want: "stroke-dashoffset"},
	{prop: "strokeLinecap", want: "stroke-linecap"},
	{prop: "strokeLinejoin", want: "stroke-linejoin"},
	{prop: "strokeMiterlimit", want: "stroke-miterlimit"},
	{prop: "strokeOpacity", want: "stroke-opacity"},
	{prop: "strokeWidth", want: "stroke-width"},
	{prop: "textAnchor", want: "text-anchor"},
	{prop: "textDecoration", want: "text-decoration"},
	{prop: "textRendering", want: "text-rendering"},
	{prop: "transformOrigin", want: "transform-origin"},
	{prop: "underlinePosition", want: "underline-position"},
	{prop: "underlineThickness", want: "underline-thickness"},
	{prop: "unicodeBidi", want: "unicode-bidi"},
	{prop: "unicodeRange", want: "unicode-range"},
	{prop: "unitsPerEm", want: "units-per-em"},
	{prop: "vAlphabetic", want: "v-alphabetic"},
	{prop: "vHanging", want: "v-hanging"},
	{prop: "vIdeographic", want: "v-ideographic"},
	{prop: "vMathematical", want: "v-mathematical"},
	{prop: "vectorEffect", want: "vector-effect"},
	{prop: "vertAdvY", want: "vert-adv-y"},
	{prop: "vertOriginX", want: "vert-origin-x"},
	{prop: "vertOriginY", want: "vert-origin-y"},
	{prop: "wordSpacing", want: "word-spacing"},
	{prop: "writingMode", want: "writing-mode"},
	{prop: "xHeight", want: "x-height"},
	{prop: "xlinkActuate", want: "xlink:actuate"},
	{prop: "xlinkArcrole", want: "xlink:arcrole"},
	{prop: "xlinkHref", want: "xlink:href"},
	{prop: "xlinkRole", want: "xlink:role"},
	{prop: "xlinkShow", want: "xlink:show"},
	{prop: "xlinkTitle", want: "xlink:title"},
	{prop: "xlinkType", want: "xlink:type"},
	{prop: "xmlBase", want: "xml:base"},
	{prop: "xmlLang", want: "xml:lang"},
	{prop: "xmlSpace", want: "xml:space"},
	{prop: "xmlnsXlink", want: "xmlns:xlink"},
}

// nsSVGPassThrough are props that must survive *unchanged*, because the SVG
// grammar's own attribute names genuinely are camelCase.
//
// Getting these wrong is worse than getting the hyphenated ones wrong. HTML
// attribute names are case-insensitive, so a lowercased HTML attribute still
// works; SVG attribute names are case-sensitive, so emitting `viewbox` does not
// produce a slightly-off attribute, it produces an ignored one — and an <svg>
// with an ignored viewBox is a blank image.
var nsSVGPassThrough = []nsAttrCase{
	{prop: "viewBox", want: "viewBox"},
	{prop: "preserveAspectRatio", want: "preserveAspectRatio"},
	{prop: "gradientTransform", want: "gradientTransform"},
	{prop: "patternUnits", want: "patternUnits"},
	{prop: "baseFrequency", want: "baseFrequency"},
	{prop: "gradientUnits", want: "gradientUnits"},
	{prop: "patternTransform", want: "patternTransform"},
	{prop: "patternContentUnits", want: "patternContentUnits"},
	{prop: "clipPathUnits", want: "clipPathUnits"},
	{prop: "maskUnits", want: "maskUnits"},
	{prop: "maskContentUnits", want: "maskContentUnits"},
	{prop: "filterUnits", want: "filterUnits"},
	{prop: "primitiveUnits", want: "primitiveUnits"},
	{prop: "markerWidth", want: "markerWidth"},
	{prop: "markerHeight", want: "markerHeight"},
	{prop: "markerUnits", want: "markerUnits"},
	{prop: "refX", want: "refX"},
	{prop: "refY", want: "refY"},
	{prop: "attributeName", want: "attributeName"},
	{prop: "attributeType", want: "attributeType"},
	{prop: "calcMode", want: "calcMode"},
	{prop: "keyTimes", want: "keyTimes"},
	{prop: "keySplines", want: "keySplines"},
	{prop: "keyPoints", want: "keyPoints"},
	{prop: "repeatCount", want: "repeatCount"},
	{prop: "repeatDur", want: "repeatDur"},
	{prop: "stdDeviation", want: "stdDeviation"},
	{prop: "numOctaves", want: "numOctaves"},
	{prop: "stitchTiles", want: "stitchTiles"},
	{prop: "edgeMode", want: "edgeMode"},
	{prop: "kernelMatrix", want: "kernelMatrix"},
	{prop: "kernelUnitLength", want: "kernelUnitLength"},
	{prop: "diffuseConstant", want: "diffuseConstant"},
	{prop: "specularConstant", want: "specularConstant"},
	{prop: "specularExponent", want: "specularExponent"},
	{prop: "surfaceScale", want: "surfaceScale"},
	{prop: "limitingConeAngle", want: "limitingConeAngle"},
	{prop: "pointsAtX", want: "pointsAtX"},
	{prop: "pointsAtY", want: "pointsAtY"},
	{prop: "pointsAtZ", want: "pointsAtZ"},
	{prop: "spreadMethod", want: "spreadMethod"},
	{prop: "startOffset", want: "startOffset"},
	{prop: "lengthAdjust", want: "lengthAdjust"},
	{prop: "textLength", want: "textLength"},
	{prop: "pathLength", want: "pathLength"},
	{prop: "tableValues", want: "tableValues"},
	{prop: "targetX", want: "targetX"},
	{prop: "targetY", want: "targetY"},
	{prop: "xChannelSelector", want: "xChannelSelector"},
	{prop: "yChannelSelector", want: "yChannelSelector"},
	{prop: "zoomAndPan", want: "zoomAndPan"},
	{prop: "requiredExtensions", want: "requiredExtensions"},
	{prop: "requiredFeatures", want: "requiredFeatures"},
	{prop: "systemLanguage", want: "systemLanguage"},
	{prop: "baseProfile", want: "baseProfile"},
	{prop: "allowReorder", want: "allowReorder"},
	{prop: "autoReverse", want: "autoReverse"},
	{prop: "externalResourcesRequired", want: "externalResourcesRequired"},
	{prop: "focusable", want: "focusable"},
	{prop: "preserveAlpha", want: "preserveAlpha"},
	{prop: "glyphRef", want: "glyphRef"},
	{prop: "viewTarget", want: "viewTarget"},
	// React's alias table contains an identity entry for "panose-1" rather than
	// the camelCase "panose1", so the camelCase spelling falls through to the
	// pass-through rule. Kept as a case because it looks like it should be
	// hyphenated and measurably is not.
	{prop: "panose1", want: "panose1"},
}

// TestNamespaceSVGAttributeNames renders each measured prop on a <path> inside
// an <svg> and checks the attribute name that comes out.
func TestNamespaceSVGAttributeNames(t *testing.T) {
	for _, group := range []struct {
		name  string
		cases []nsAttrCase
	}{
		{"renamed", nsSVGRenames},
		{"pass through", nsSVGPassThrough},
	} {
		for _, tc := range group.cases {
			t.Run(group.name+"/"+tc.prop, func(t *testing.T) {
				got := nsRender(t, H("svg", nil, H("path", Props{tc.prop: "v"})))
				want := `<svg><path ` + tc.want + `="v"></path></svg>`
				if got != want {
					t.Errorf("prop %q:\n got %s\nwant %s", tc.prop, got, want)
				}
			})
		}
	}
}

// TestNamespaceAttributeNamingIsNamespaceBlind locks in the probe's central
// finding: React 19 names attributes through one switch that never looks at the
// insertion mode, so the same prop produces the same attribute in HTML, in SVG,
// inside a foreignObject and in MathML.
//
// This test exists to stop a well-meaning future change from introducing a
// separate "SVG table" that only applies inside <svg>. That would look more
// correct and would produce markup React does not.
func TestNamespaceAttributeNamingIsNamespaceBlind(t *testing.T) {
	// One prop from each interesting shape: hyphenated, namespaced, camelCase.
	probes := []nsAttrCase{
		{prop: "strokeWidth", want: "stroke-width"},
		{prop: "xlinkHref", want: "xlink:href"},
		{prop: "viewBox", want: "viewBox"},
		{prop: "className", want: "class"},
	}
	contexts := []struct {
		name string
		wrap func(inner Node) Node
	}{
		{"html", func(inner Node) Node { return H("div", nil, inner) }},
		{"svg", func(inner Node) Node { return H("svg", nil, inner) }},
		{"foreignObject", func(inner Node) Node {
			return H("svg", nil, H("foreignObject", nil, inner))
		}},
		{"mathml", func(inner Node) Node { return H("math", nil, inner) }},
	}
	for _, p := range probes {
		for _, ctx := range contexts {
			t.Run(p.prop+"/"+ctx.name, func(t *testing.T) {
				got := nsRender(t, ctx.wrap(H("span", Props{p.prop: "v"})))
				if !strings.Contains(got, `<span `+p.want+`="v">`) {
					t.Errorf("prop %q in %s context: want attribute %q in output, got %s",
						p.prop, ctx.name, p.want, got)
				}
			})
		}
	}
}

// TestNamespaceTransitions covers the namespace stack directly, without going
// through the serializer, so a transition bug is reported as a transition bug
// rather than as a surprising string.
//
// The pair being tested is the thing most ports get wrong: an element's own
// namespace is not its children's. <foreignObject> is an SVG element whose
// children are HTML.
func TestNamespaceTransitions(t *testing.T) {
	cases := []struct {
		name        string
		parent      Namespace
		tag         string
		wantElement Namespace
		wantContent Namespace
	}{
		{"html stays html", NamespaceHTML, "div", NamespaceHTML, NamespaceHTML},
		{"svg opens svg", NamespaceHTML, "svg", NamespaceSVG, NamespaceSVG},
		{"math opens mathml", NamespaceHTML, "math", NamespaceMathML, NamespaceMathML},
		{"svg child inherits", NamespaceSVG, "path", NamespaceSVG, NamespaceSVG},
		{"unknown svg child inherits", NamespaceSVG, "feDropShadow", NamespaceSVG, NamespaceSVG},

		// The integration points: the element is SVG, the content is HTML.
		{"foreignObject is svg", NamespaceSVG, "foreignObject", NamespaceSVG, NamespaceHTML},
		{"desc is an integration point", NamespaceSVG, "desc", NamespaceSVG, NamespaceHTML},
		{"title is an integration point", NamespaceSVG, "title", NamespaceSVG, NamespaceHTML},
		// Matched case-insensitively, as an HTML parser would.
		{"foreignobject lowercased", NamespaceSVG, "foreignobject", NamespaceSVG, NamespaceHTML},

		// MathML token elements hold HTML.
		{"mtext holds html", NamespaceMathML, "mtext", NamespaceMathML, NamespaceHTML},
		{"mi holds html", NamespaceMathML, "mi", NamespaceMathML, NamespaceHTML},
		{"mo holds html", NamespaceMathML, "mo", NamespaceMathML, NamespaceHTML},
		{"mn holds html", NamespaceMathML, "mn", NamespaceMathML, NamespaceHTML},
		{"ms holds html", NamespaceMathML, "ms", NamespaceMathML, NamespaceHTML},
		{"annotation-xml holds html", NamespaceMathML, "annotation-xml", NamespaceMathML, NamespaceHTML},
		{"mrow does not", NamespaceMathML, "mrow", NamespaceMathML, NamespaceMathML},

		// Re-entry: <svg> nested inside HTML that is itself inside a
		// foreignObject opens SVG again. This is what makes the model a stack
		// rather than a one-way switch.
		{"svg re-enters from html", NamespaceHTML, "svg", NamespaceSVG, NamespaceSVG},
		{"math inside svg content", NamespaceHTML, "math", NamespaceMathML, NamespaceMathML},
		// title in HTML is an ordinary HTML element, not an integration point.
		{"html title is plain", NamespaceHTML, "title", NamespaceHTML, NamespaceHTML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			element := ElementNamespace(tc.parent, tc.tag)
			if element != tc.wantElement {
				t.Errorf("ElementNamespace(%v, %q) = %v, want %v",
					tc.parent, tc.tag, element, tc.wantElement)
			}
			if content := ContentNamespace(element, tc.tag); content != tc.wantContent {
				t.Errorf("ContentNamespace(%v, %q) = %v, want %v",
					element, tc.tag, content, tc.wantContent)
			}
		})
	}
}

// TestNamespaceForeignObjectNesting walks the transition through the serializer,
// including leaving an integration point and getting SVG rules back.
//
// Every want string here is the exact output of react-dom@19.2.8
// renderToStaticMarkup on the equivalent tree.
func TestNamespaceForeignObjectNesting(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			// Leaving <foreignObject> restores SVG for the following sibling:
			// the <path> after it still gets stroke-width, not strokeWidth.
			name: "html inside, svg after",
			node: H("svg", Props{"viewBox": "0 0 1 1"},
				H("foreignObject", Props{"width": 10},
					H("div", Props{"className": "c", "tabIndex": 2},
						H("span", nil, "x"))),
				H("path", Props{"strokeWidth": 2})),
			want: `<svg viewBox="0 0 1 1">` +
				`<foreignObject width="10"><div class="c" tabindex="2"><span>x</span></div></foreignObject>` +
				`<path stroke-width="2"></path></svg>`,
		},
		{
			// The full round trip: html > svg > foreignObject > html > svg.
			name: "svg re-entered inside a foreignObject",
			node: H("div", Props{"className": "a"},
				H("svg", Props{"viewBox": "0 0"},
					H("foreignObject", nil,
						H("div", Props{"className": "b"},
							H("svg", Props{"viewBox": "1 1"},
								H("path", Props{"strokeWidth": 1})))))),
			want: `<div class="a"><svg viewBox="0 0"><foreignObject>` +
				`<div class="b"><svg viewBox="1 1"><path stroke-width="1"></path></svg></div>` +
				`</foreignObject></svg></div>`,
		},
		{
			// HTML void elements inside a foreignObject still self-close, and
			// the surrounding SVG is unaffected.
			name: "void html inside foreignObject",
			node: H("svg", nil,
				H("foreignObject", nil,
					H("div", nil, H("br", nil), H("hr", nil)))),
			want: `<svg><foreignObject><div><br/><hr/></div></foreignObject></svg>`,
		},
		{
			name: "mathml token element holds html",
			node: H("math", Props{"display": "block"},
				H("mtext", nil, H("span", Props{"className": "c"}, "x"))),
			want: `<math display="block"><mtext><span class="c">x</span></mtext></math>`,
		},
		{
			name: "svg inside a mathml token element",
			node: H("math", nil,
				H("mtext", nil,
					H("svg", Props{"viewBox": "0 0 1 1"}, H("path", Props{"strokeWidth": 1})))),
			want: `<math><mtext><svg viewBox="0 0 1 1"><path stroke-width="1"></path></svg></mtext></math>`,
		},
		{
			name: "mathml inside a foreignObject",
			node: H("svg", nil,
				H("foreignObject", nil,
					H("math", nil, H("mi", nil, "x")))),
			want: `<svg><foreignObject><math><mi>x</mi></math></foreignObject></svg>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nsRender(t, tc.node); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestNamespaceSVGElementsAreNotSelfClosed is the deliberate contradiction of a
// widely repeated claim.
//
// The folklore — and this port's own backlog entry — says React self-closes
// childless SVG elements, emitting `<path d="…"/>`. It does not. Measured
// against react-dom@19.2.8, a childless <path> renders as `<path d="…"></path>`;
// the self-closing set is the HTML void list, matched by tag name with no regard
// for namespace whatsoever. `<svg><br/></svg>` really does self-close, and
// `<svg><image/></svg>` really does not.
//
// The probe is the oracle, so the port follows the measurement rather than the
// folklore, and this test is here so nobody "fixes" it back.
func TestNamespaceSVGElementsAreNotSelfClosed(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{"childless path", H("svg", nil, H("path", Props{"d": "M0 0"})),
			`<svg><path d="M0 0"></path></svg>`},
		{"childless g", H("svg", nil, H("g", nil)),
			`<svg><g></g></svg>`},
		{"childless circle", H("svg", nil, H("circle", Props{"r": 1})),
			`<svg><circle r="1"></circle></svg>`},
		{"empty svg", H("svg", nil),
			`<svg></svg>`},
		{"svg image is not img", H("svg", nil, H("image", Props{"href": "a"})),
			`<svg><image href="a"></image></svg>`},
		{"svg use", H("svg", nil, H("use", Props{"href": "#a"})),
			`<svg><use href="#a"></use></svg>`},
		{"svg stop", H("svg", nil, H("stop", Props{"offset": "0"})),
			`<svg><stop offset="0"></stop></svg>`},
		{"childless mspace", H("math", nil, H("mspace", Props{"width": "1em"})),
			`<math><mspace width="1em"></mspace></math>`},
		// The void set is namespace-blind, so an HTML void name inside <svg>
		// self-closes even though that is not valid SVG.
		{"html void name inside svg", H("svg", nil, H("br", nil)),
			`<svg><br/></svg>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nsRender(t, tc.node); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestNamespaceTagNamesAreCaseSensitive checks that the tag written to the
// output is the caller's exact spelling.
//
// SVG element names are case-sensitive, and several of the ones people actually
// use are camelCase. A serializer that lowercased tags — a tempting thing to do,
// since HTML tags are case-insensitive — would silently turn every gradient,
// clip path and filter primitive into an unknown element. The namespace
// transitions in ssr_namespace.go match tag names case-insensitively on purpose;
// this test proves that does not leak into the output.
func TestNamespaceTagNamesAreCaseSensitive(t *testing.T) {
	tags := []string{
		"linearGradient", "radialGradient", "clipPath", "textPath",
		"feGaussianBlur", "feColorMatrix", "feComponentTransfer", "feDropShadow",
		"foreignObject", "animateTransform", "animateMotion", "glyphRef",
	}
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			got := nsRender(t, H("svg", nil, H(tag, Props{"id": "x"})))
			want := `<svg><` + tag + ` id="x"></` + tag + `></svg>`
			if got != want {
				t.Errorf("\n got %s\nwant %s", got, want)
			}
		})
	}
}

// TestNamespaceRealisticSVG renders the shapes people actually write, so the
// pieces are exercised together rather than one attribute at a time. Both want
// strings were taken from react-dom@19.2.8.
func TestNamespaceRealisticSVG(t *testing.T) {
	gradient := H("svg", Props{"viewBox": "0 0 100 100", "xmlnsXlink": "http://www.w3.org/1999/xlink"},
		H("defs", nil,
			H("linearGradient", Props{"id": "g", "gradientUnits": "objectBoundingBox", "gradientTransform": "rotate(45)"},
				H("stop", Props{"offset": "0", "stopColor": "red", "stopOpacity": 0.5}),
				H("stop", Props{"offset": "1", "stopColor": "blue"})),
			H("clipPath", Props{"id": "c", "clipPathUnits": "userSpaceOnUse"},
				H("rect", Props{"width": 50, "height": 50})),
			H("filter", Props{"id": "f"},
				H("feGaussianBlur", Props{"in": "SourceGraphic", "stdDeviation": 2}),
				H("feTurbulence", Props{"baseFrequency": "0.05", "numOctaves": 3}))),
		H("path", Props{
			"d": "M0 0 L1 1", "clipPath": "url(#c)", "fillRule": "evenodd",
			"strokeWidth": 2, "strokeLinecap": "round", "markerEnd": "url(#m)",
		}),
		H("text", Props{"textAnchor": "middle", "dominantBaseline": "central", "xmlSpace": "preserve"},
			H("textPath", Props{"xlinkHref": "#p", "startOffset": "50%"}, "label")))

	// Attributes are emitted in sorted order by this port (Go maps have no
	// insertion order), which is the one place the bytes differ from React.
	want := `<svg viewBox="0 0 100 100" xmlns:xlink="http://www.w3.org/1999/xlink">` +
		`<defs>` +
		`<linearGradient gradientTransform="rotate(45)" gradientUnits="objectBoundingBox" id="g">` +
		`<stop offset="0" stop-color="red" stop-opacity="0.5"></stop>` +
		`<stop offset="1" stop-color="blue"></stop>` +
		`</linearGradient>` +
		`<clipPath clipPathUnits="userSpaceOnUse" id="c"><rect height="50" width="50"></rect></clipPath>` +
		`<filter id="f">` +
		`<feGaussianBlur in="SourceGraphic" stdDeviation="2"></feGaussianBlur>` +
		`<feTurbulence baseFrequency="0.05" numOctaves="3"></feTurbulence>` +
		`</filter>` +
		`</defs>` +
		`<path clip-path="url(#c)" d="M0 0 L1 1" fill-rule="evenodd" marker-end="url(#m)" ` +
		`stroke-linecap="round" stroke-width="2"></path>` +
		`<text dominant-baseline="central" text-anchor="middle" xml:space="preserve">` +
		`<textPath startOffset="50%" xlink:href="#p">label</textPath>` +
		`</text>` +
		`</svg>`
	if got := nsRender(t, gradient); got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

// TestNamespaceDangerousInnerHTML checks that raw content works inside SVG and
// inside an integration point, and — the part worth a test — that the namespace
// is restored afterwards even though the walk never descended into children.
func TestNamespaceDangerousInnerHTML(t *testing.T) {
	cases := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "raw svg markup",
			node: H("svg", nil, H("g", Props{
				"dangerouslySetInnerHTML": DangerousHTML{HTML: `<path d="M0 0"/>`},
			})),
			want: `<svg><g><path d="M0 0"/></g></svg>`,
		},
		{
			name: "raw html inside foreignObject",
			node: H("svg", nil, H("foreignObject", Props{
				"dangerouslySetInnerHTML": DangerousHTML{HTML: `<b>x</b>`},
			})),
			want: `<svg><foreignObject><b>x</b></foreignObject></svg>`,
		},
		{
			// A raw-content element consumes no children, so the namespace
			// restore has to happen on the way out of the element rather than
			// after the child walk. If it did not, the sibling <path> would be
			// named by whatever namespace the raw element left behind.
			name: "namespace restored after a raw element",
			node: H("svg", nil,
				H("foreignObject", Props{
					"dangerouslySetInnerHTML": DangerousHTML{HTML: `<b>x</b>`},
				}),
				H("path", Props{"strokeWidth": 1})),
			want: `<svg><foreignObject><b>x</b></foreignObject><path stroke-width="1"></path></svg>`,
		},
		{
			name: "raw markup escapes nothing",
			node: H("svg", Props{
				"dangerouslySetInnerHTML": DangerousHTML{HTML: `<text>a & b < c</text>`},
			}),
			want: `<svg><text>a & b < c</text></svg>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nsRender(t, tc.node); got != tc.want {
				t.Errorf("\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestNamespaceAttributeTableCoversMeasuredOracle checks the port's attribute
// table against the measurement rather than against a second copy of itself.
//
// The tables at the top of this file are the only authority here: they came out
// of react-dom. Whichever table in the package happens to implement the mapping
// — today ssrAttributeNames in ssr_attr.go — has to reproduce every one of them.
// Asserting through [AttributeName] rather than against a specific map means
// this keeps working if the table is reorganized, and fails loudly if an entry
// is dropped.
func TestNamespaceAttributeTableCoversMeasuredOracle(t *testing.T) {
	all := make([]nsAttrCase, 0, len(nsSVGRenames)+len(nsSVGPassThrough))
	all = append(all, nsSVGRenames...)
	all = append(all, nsSVGPassThrough...)
	for _, tc := range all {
		got, ok := AttributeName(tc.prop)
		if !ok || got != tc.want {
			t.Errorf("AttributeName(%q) = %q, %v; react-dom emits %q",
				tc.prop, got, ok, tc.want)
		}
	}
}

// TestNamespaceResolveAttributeNamePreservesOmissions pins the one invariant the
// namespace seam has to hold: routing attribute naming through
// ssrResolveAttributeName must not change any decision [AttributeName] makes,
// in any namespace. The props below are the ones that must never reach the
// output at all, and a seam that leaked one of them would put a Go func or a raw
// HTML struct into an attribute value.
func TestNamespaceResolveAttributeNamePreservesOmissions(t *testing.T) {
	omitted := []string{ChildrenKey, KeyProp, RefProp, "dangerouslySetInnerHTML", "onClick", ""}
	for _, ns := range []Namespace{NamespaceHTML, NamespaceSVG, NamespaceMathML} {
		for _, prop := range omitted {
			if attr, ok := ssrResolveAttributeName(prop, ns); ok {
				t.Errorf("ssrResolveAttributeName(%q, %v) = %q, true; want omitted", prop, ns, attr)
			}
		}
		for prop, want := range ssrAttributeNames {
			got, ok := ssrResolveAttributeName(prop, ns)
			if !ok || got != want {
				t.Errorf("ssrResolveAttributeName(%q, %v) = %q, %v; want %q, true",
					prop, ns, got, ok, want)
			}
		}
	}
}

// TestNamespaceString keeps the diagnostic names stable, since they appear in
// test failures above.
func TestNamespaceString(t *testing.T) {
	for ns, want := range map[Namespace]string{
		NamespaceHTML:   "html",
		NamespaceSVG:    "svg",
		NamespaceMathML: "mathml",
		Namespace(99):   "html",
	} {
		if got := ns.String(); got != want {
			t.Errorf("Namespace(%d).String() = %q, want %q", int(ns), got, want)
		}
	}
}
