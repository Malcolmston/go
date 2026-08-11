package react

import "testing"

// TestSSRStyleCSSName pins the name rewrite against react-dom's
// name.replace(/([A-Z])/g, '-$1').toLowerCase().replace(/^ms-/, '-ms-').
// Every expectation here was read off a render with react-dom 19.2.7.
func TestSSRStyleCSSName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"color", "color"},
		{"backgroundColor", "background-color"},
		{"borderBottomLeftRadius", "border-bottom-left-radius"},
		{"background-color", "background-color"},
		{"WebkitTransform", "-webkit-transform"},
		{"msTransform", "-ms-transform"},
		{"MsTransform", "-ms-transform"},
		{"MozAppearance", "-moz-appearance"},
		{"OTransform", "-o-transform"},
		// React only warns about a hyphenated camelCase mix and still rewrites it.
		{"foo-Bar", "foo--bar"},
		// Custom properties keep their case and are never rewritten.
		{"--brand", "--brand"},
		{"--Brand", "--Brand"},
	}
	for _, c := range cases {
		if got := ssrStyleCSSName(c.in); got != c.want {
			t.Errorf("ssrStyleCSSName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSSRStyleAttrValue covers every rule of the serializer: dropping,
// kebab-casing, the px/unitless split, custom properties, trimming and
// joining. Values arrive as float64 in the parity harness (JSON numbers), so
// both float64 and int are exercised.
func TestSSRStyleAttrValue(t *testing.T) {
	cases := []struct {
		name  string
		style any
		want  string
		mode  ssrAttrMode
	}{
		{"single", map[string]any{"color": "red"}, "color:red", ssrAttrValued},
		{"two joined without spaces", map[string]any{"color": "red", "display": "block"}, "color:red;display:block", ssrAttrValued},
		{"camel to kebab", map[string]any{"backgroundColor": "blue"}, "background-color:blue", ssrAttrValued},
		{"already kebab", map[string]any{"background-color": "blue"}, "background-color:blue", ssrAttrValued},

		{"number gets px", map[string]any{"width": float64(10)}, "width:10px", ssrAttrValued},
		{"int gets px", map[string]any{"width": 10}, "width:10px", ssrAttrValued},
		{"float keeps js formatting", map[string]any{"width": 1.5}, "width:1.5px", ssrAttrValued},
		{"negative gets px", map[string]any{"marginTop": -5}, "margin-top:-5px", ssrAttrValued},
		{"zero has no unit", map[string]any{"margin": 0}, "margin:0", ssrAttrValued},
		{"zero float has no unit", map[string]any{"margin": 0.0}, "margin:0", ssrAttrValued},
		{"uint gets px", map[string]any{"width": uint(3)}, "width:3px", ssrAttrValued},

		{"unitless zIndex", map[string]any{"zIndex": 3}, "z-index:3", ssrAttrValued},
		{"unitless flex", map[string]any{"flex": 1}, "flex:1", ssrAttrValued},
		{"unitless lineHeight", map[string]any{"lineHeight": 1.5}, "line-height:1.5", ssrAttrValued},
		{"unitless strokeWidth", map[string]any{"strokeWidth": 2}, "stroke-width:2", ssrAttrValued},
		// The set is keyed on the camelCase spelling, vendor prefix included.
		{"unitless WebkitLineClamp", map[string]any{"WebkitLineClamp": 3}, "-webkit-line-clamp:3", ssrAttrValued},
		{"unitless MozLineClamp", map[string]any{"MozLineClamp": 3}, "-moz-line-clamp:3", ssrAttrValued},
		{"unitless lineClamp", map[string]any{"lineClamp": 3}, "line-clamp:3", ssrAttrValued},
		{"unitless msFlex", map[string]any{"msFlex": 1}, "-ms-flex:1", ssrAttrValued},
		// A hand-hyphenated name is not in React's set, so it gets px.
		{"hyphenated name is not unitless", map[string]any{"z-index": 3}, "z-index:3px", ssrAttrValued},
		// React's own typo: the capital-K spelling is the one in the set.
		{"WebKitBoxFlexGroup typo is unitless", map[string]any{"WebKitBoxFlexGroup": 1}, "-web-kit-box-flex-group:1", ssrAttrValued},
		{"WebkitBoxFlexGroup is not", map[string]any{"WebkitBoxFlexGroup": 1}, "-webkit-box-flex-group:1px", ssrAttrValued},

		{"numeric string keeps no unit", map[string]any{"width": "10"}, "width:10", ssrAttrValued},
		{"string with unit", map[string]any{"width": "10rem"}, "width:10rem", ssrAttrValued},
		{"string value is trimmed", map[string]any{"color": "  red  "}, "color:red", ssrAttrValued},
		{"blank string is kept and empties the value", map[string]any{"width": " "}, "width:", ssrAttrValued},

		{"custom property", map[string]any{"--brand": "#fff"}, "--brand:#fff", ssrAttrValued},
		{"custom property number has no px", map[string]any{"--count": 3}, "--count:3", ssrAttrValued},
		{"custom property zero", map[string]any{"--c": 0}, "--c:0", ssrAttrValued},
		{"custom and normal", map[string]any{"--x": "1", "color": "red"}, "--x:1;color:red", ssrAttrValued},

		{"nil drops the declaration", map[string]any{"color": nil, "display": "block"}, "display:block", ssrAttrValued},
		{"empty string drops the declaration", map[string]any{"color": "", "display": "block"}, "display:block", ssrAttrValued},
		{"true drops the declaration", map[string]any{"width": true, "display": "block"}, "display:block", ssrAttrValued},
		{"false drops the declaration", map[string]any{"width": false, "display": "block"}, "display:block", ssrAttrValued},
		{"all dropped omits the attribute", map[string]any{"color": nil, "width": nil}, "", ssrAttrOmit},
		{"empty map omits the attribute", map[string]any{}, "", ssrAttrOmit},
		{"nil style omits the attribute", nil, "", ssrAttrOmit},

		// Values are handed to the caller unescaped; ssr.go escapes them.
		{"quote is left to the caller", map[string]any{"fontFamily": `"My Font", serif`}, `font-family:"My Font", serif`, ssrAttrValued},
		{"semicolon in a value is not escaped", map[string]any{"content": `"a;b"`}, `content:"a;b"`, ssrAttrValued},
		{"url value", map[string]any{"backgroundImage": "url(/a.png?a=1&b=2)"}, "background-image:url(/a.png?a=1&b=2)", ssrAttrValued},
		{"calc value", map[string]any{"width": "calc(100% - 10px)"}, "width:calc(100% - 10px)", ssrAttrValued},
		{"important value", map[string]any{"color": "red !important"}, "color:red !important", ssrAttrValued},
		{"multi comma value", map[string]any{"transition": "color 1s, background 2s"}, "transition:color 1s, background 2s", ssrAttrValued},

		// The other two accepted map shapes.
		{"map[string]string", map[string]string{"color": "red", "fontSize": "12px"}, "color:red;font-size:12px", ssrAttrValued},
		{"Props", Props{"zIndex": 10, "marginTop": 8}, "margin-top:8px;z-index:10", ssrAttrValued},

		// Declarations come out sorted by key, the port's one deviation here.
		{"sorted keys", map[string]any{"display": "flex", "alignItems": "center"}, "align-items:center;display:flex", ssrAttrValued},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, mode, err := ssrStyleAttrValue(c.style)
			if err != nil {
				t.Fatalf("ssrStyleAttrValue(%#v) returned error %v", c.style, err)
			}
			if mode != c.mode {
				t.Fatalf("ssrStyleAttrValue(%#v) mode = %v, want %v", c.style, mode, c.mode)
			}
			if got != c.want {
				t.Fatalf("ssrStyleAttrValue(%#v) = %q, want %q", c.style, got, c.want)
			}
		})
	}
}

// TestSSRStyleAttrValueErrors pins the two shapes React refuses. React throws
// for a non-object style prop, and the harness scores an upstream throw against
// a port error as a match, so this path must stay an error.
func TestSSRStyleAttrValueErrors(t *testing.T) {
	for _, style := range []any{
		"color:red",
		42,
		[]string{"color:red"},
		struct{ Color string }{"red"},
	} {
		if _, _, err := ssrStyleAttrValue(style); err == nil {
			t.Errorf("ssrStyleAttrValue(%#v) = nil error, want an error", style)
		}
	}

	// A value type CSS cannot express is an error too, rather than React's
	// "[object Object]".
	if _, _, err := ssrStyleAttrValue(map[string]any{"width": []int{1}}); err == nil {
		t.Error("ssrStyleAttrValue with a slice value = nil error, want an error")
	}
}

// TestSSRStyleAttrValueFuncDropped keeps the style prop consistent with every
// other prop: a func is dropped, not stringified into the page.
func TestSSRStyleAttrValueFuncDropped(t *testing.T) {
	if _, mode, err := ssrStyleAttrValue(func() {}); err != nil || mode != ssrAttrOmit {
		t.Errorf("func style prop = (%v, %v), want (omit, nil)", mode, err)
	}
	got, mode, err := ssrStyleAttrValue(map[string]any{"width": func() {}, "color": "red"})
	if err != nil {
		t.Fatalf("func style value returned error %v", err)
	}
	if mode != ssrAttrValued || got != "color:red" {
		t.Errorf("func style value = (%q, %v), want (\"color:red\", valued)", got, mode)
	}
}

// TestSSRStyleRendered walks the whole emitter for the cases where escaping and
// the style attribute interact, so the joined-then-escaped shortcut stays
// equivalent to React's escape-each-part.
func TestSSRStyleRendered(t *testing.T) {
	cases := []struct {
		name  string
		props Props
		want  string
	}{
		{"plain", Props{"style": map[string]any{"color": "red", "display": "block"}}, `<div style="color:red;display:block"></div>`},
		{"px", Props{"style": map[string]any{"width": 10}}, `<div style="width:10px"></div>`},
		{"quotes escaped", Props{"style": map[string]any{"fontFamily": `"Fancy"`}}, `<div style="font-family:&quot;Fancy&quot;"></div>`},
		{"url ampersand escaped", Props{"style": map[string]any{"backgroundImage": "url(/a.png?a=1&b=2)"}}, `<div style="background-image:url(/a.png?a=1&amp;b=2)"></div>`},
		{"custom property name escaped", Props{"style": map[string]any{"--a<b": "1"}}, `<div style="--a&lt;b:1"></div>`},
		{"empty object drops the attribute", Props{"style": map[string]any{}}, `<div></div>`},
		{"bool value drops the declaration", Props{"style": map[string]any{"width": true}}, `<div></div>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(CreateElement("div", c.props))
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}

	if _, err := RenderToStaticMarkup(CreateElement("div", Props{"style": "color:red"})); err == nil {
		t.Error("a string style prop rendered without error, want the React-equivalent failure")
	}
}
