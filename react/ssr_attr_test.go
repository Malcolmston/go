package react

import (
	"strings"
	"testing"
)

func TestAttributeName(t *testing.T) {
	cases := []struct {
		name string
		prop string
		want string
		ok   bool
	}{
		// The renames with a dedicated case in React's pushAttribute.
		{"className", "className", "class", true},
		{"tabIndex", "tabIndex", "tabindex", true},
		{"htmlFor", "htmlFor", "for", true},
		{"httpEquiv", "httpEquiv", "http-equiv", true},
		{"acceptCharset", "acceptCharset", "accept-charset", true},
		{"crossOrigin", "crossOrigin", "crossorigin", true},

		// The three booleans React lowercases by hand, and no others.
		{"autoFocus", "autoFocus", "autofocus", true},
		{"multiple", "multiple", "multiple", true},
		{"muted", "muted", "muted", true},

		// The port's controlled-input aliases. React folds these into
		// value/checked inside its <input> serializer; the port does it here.
		{"defaultValue", "defaultValue", "value", true},
		{"defaultChecked", "defaultChecked", "checked", true},

		// The long tail React does *not* rename. Every one of these was a
		// lowercasing entry in an earlier version of the table, and every one
		// of them is emitted by react-dom with its JSX spelling intact, because
		// HTML attribute names are ASCII-case-insensitive and React never
		// bothers to fold them.
		{"maxLength", "maxLength", "maxLength", true},
		{"minLength", "minLength", "minLength", true},
		{"colSpan", "colSpan", "colSpan", true},
		{"rowSpan", "rowSpan", "rowSpan", true},
		{"srcSet", "srcSet", "srcSet", true},
		{"srcDoc", "srcDoc", "srcDoc", true},
		{"srcLang", "srcLang", "srcLang", true},
		{"readOnly", "readOnly", "readOnly", true},
		{"noValidate", "noValidate", "noValidate", true},
		{"formNoValidate", "formNoValidate", "formNoValidate", true},
		{"formAction", "formAction", "formAction", true},
		{"formEncType", "formEncType", "formEncType", true},
		{"formMethod", "formMethod", "formMethod", true},
		{"formTarget", "formTarget", "formTarget", true},
		{"encType", "encType", "encType", true},
		{"allowFullScreen", "allowFullScreen", "allowFullScreen", true},
		{"autoComplete", "autoComplete", "autoComplete", true},
		{"autoPlay", "autoPlay", "autoPlay", true},
		{"contentEditable", "contentEditable", "contentEditable", true},
		{"spellCheck", "spellCheck", "spellCheck", true},
		{"dateTime", "dateTime", "dateTime", true},
		{"hrefLang", "hrefLang", "hrefLang", true},
		{"inputMode", "inputMode", "inputMode", true},
		{"referrerPolicy", "referrerPolicy", "referrerPolicy", true},
		{"radioGroup", "radioGroup", "radioGroup", true},
		{"useMap", "useMap", "useMap", true},
		{"keyParams", "keyParams", "keyParams", true},
		{"keyType", "keyType", "keyType", true},
		{"itemProp", "itemProp", "itemProp", true},
		{"itemScope", "itemScope", "itemScope", true},
		{"itemType", "itemType", "itemType", true},
		{"itemID", "itemID", "itemID", true},
		{"itemRef", "itemRef", "itemRef", true},

		// SVG presentation attributes are hyphenated — React's aliases map.
		{"strokeWidth", "strokeWidth", "stroke-width", true},
		{"strokeLinecap", "strokeLinecap", "stroke-linecap", true},
		{"fillOpacity", "fillOpacity", "fill-opacity", true},
		{"stopColor", "stopColor", "stop-color", true},
		{"clipPath", "clipPath", "clip-path", true},
		{"transformOrigin", "transformOrigin", "transform-origin", true},
		{"vAlphabetic", "vAlphabetic", "v-alphabetic", true},
		{"horizAdvX", "horizAdvX", "horiz-adv-x", true},
		{"glyphOrientationVertical", "glyphOrientationVertical", "glyph-orientation-vertical", true},
		{"panose-1 maps to itself", "panose-1", "panose-1", true},

		// Namespaced attributes regain their colon.
		{"xlinkHref", "xlinkHref", "xlink:href", true},
		{"xlinkArcrole", "xlinkArcrole", "xlink:arcrole", true},
		{"xlinkTitle", "xlinkTitle", "xlink:title", true},
		{"xmlLang", "xmlLang", "xml:lang", true},
		{"xmlBase", "xmlBase", "xml:base", true},
		{"xmlnsXlink", "xmlnsXlink", "xmlns:xlink", true},

		// Genuinely camelCase SVG attributes survive the pass-through rule with
		// no table entry at all — this is the case the fallback exists for.
		{"viewBox", "viewBox", "viewBox", true},
		{"preserveAspectRatio", "preserveAspectRatio", "preserveAspectRatio", true},
		{"gradientTransform", "gradientTransform", "gradientTransform", true},
		{"refX", "refX", "refX", true},
		{"attributeName", "attributeName", "attributeName", true},

		// Already-hyphenated and namespaced names pass through untouched.
		{"aria", "aria-hidden", "aria-hidden", true},
		{"data", "data-test-id", "data-test-id", true},
		{"custom hyphenated", "my-widget-mode", "my-widget-mode", true},
		{"pre-namespaced", "xlink:href", "xlink:href", true},

		// Plain lowercase attributes need no translation.
		{"id", "id", "id", true},
		{"href", "href", "href", true},
		{"style", "style", "style", true},

		// Unknown props are *not* dropped. React 15 kept an allowlist; React 16
		// and later write any legally named prop straight through, which is why
		// custom-element attributes work without ceremony.
		{"unknown camelCase", "myAttr", "myAttr", true},
		{"unknown lowercase", "wibble", "wibble", true},
		{"unknown with digits", "x2", "x2", true},
		{"unknown underscored", "_private", "_private", true},

		// Never emitted.
		{"children", ChildrenKey, "", false},
		{"key", KeyProp, "", false},
		{"ref", RefProp, "", false},
		{"dangerouslySetInnerHTML", "dangerouslySetInnerHTML", "", false},
		{"innerHTML", "innerHTML", "", false},
		{"suppressContentEditableWarning", "suppressContentEditableWarning", "", false},
		{"suppressHydrationWarning", "suppressHydrationWarning", "", false},
		{"onClick", "onClick", "", false},
		{"onclick", "onclick", "", false},
		{"onPointerDown", "onPointerDown", "", false},
		{"empty", "", "", false},

		// React's on-prefix test looks at exactly two characters and requires a
		// third, so both of these oddities are upstream's and are pinned here
		// deliberately: "only" is treated as a handler, and "on" is not.
		{"only is dropped like a handler", "only", "", false},
		{"ON in caps is dropped", "ONCLICK", "", false},
		{"oNclick mixed case is dropped", "oNclick", "", false},
		{"on alone survives", "on", "on", true},

		// Names that could break out of the attribute are refused.
		{"space", "data foo", "", false},
		{"quote", `x"y`, "", false},
		{"equals", "x=y", "", false},
		{"angle bracket", "x>y", "", false},
		{"leading digit", "2x", "", false},
		{"leading hyphen", "-x", "", false},
		{"digit inside is fine", "aria-level2", "aria-level2", true},
		{"colon start is a legal XML name", ":x", ":x", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := AttributeName(c.prop)
			if got != c.want || ok != c.ok {
				t.Errorf("AttributeName(%q) = (%q, %v), want (%q, %v)", c.prop, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestFormatStyle(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil is empty", nil, ""},
		{"string passthrough", "color:red;", "color:red;"},
		{"string is not reformatted", "COLOR: Red !important", "COLOR: Red !important"},
		{"empty map", map[string]any{}, ""},

		{"camelCase becomes kebab-case", map[string]any{"backgroundColor": "red"}, "background-color:red"},
		{"already kebab-case", map[string]any{"background-color": "red"}, "background-color:red"},
		{"custom property", map[string]any{"--brand": "#f00"}, "--brand:#f00"},
		{"vendor prefix", map[string]any{"WebkitLineClamp": "2"}, "-webkit-line-clamp:2"},
		{"microsoft prefix", map[string]any{"msGridColumn": "2"}, "-ms-grid-column:2"},

		{"px is added to lengths", map[string]any{"width": 10}, "width:10px"},
		{"floats get px too", map[string]any{"marginTop": 1.5}, "margin-top:1.5px"},
		{"zero never gets a unit", map[string]any{"width": 0}, "width:0"},

		{"opacity is unitless", map[string]any{"opacity": 1}, "opacity:1"},
		{"zIndex is unitless", map[string]any{"zIndex": 10}, "z-index:10"},
		{"z-index spelled out is unitless too", map[string]any{"z-index": 10}, "z-index:10"},
		{"lineHeight is unitless", map[string]any{"lineHeight": 1.4}, "line-height:1.4"},
		{"fontWeight is unitless", map[string]any{"fontWeight": 700}, "font-weight:700"},
		{"flex is unitless", map[string]any{"flex": 1}, "flex:1"},
		{"order is unitless", map[string]any{"order": 2}, "order:2"},
		{"zoom is unitless", map[string]any{"zoom": 2}, "zoom:2"},
		{"strokeWidth is unitless", map[string]any{"strokeWidth": 3}, "stroke-width:3"},
		{"but fontSize is a length", map[string]any{"fontSize": 12}, "font-size:12px"},

		{"a string value keeps its own unit", map[string]any{"width": "50%"}, "width:50%"},
		{"nil values drop the declaration", map[string]any{"color": nil, "top": 0}, "top:0"},
		{"empty strings drop the declaration", map[string]any{"color": "", "top": 0}, "top:0"},

		{
			"declarations are sorted and joined with a semicolon",
			map[string]any{"zIndex": 1, "color": "red", "marginTop": 4},
			"color:red;margin-top:4px;z-index:1",
		},
		{
			"map[string]string is accepted",
			map[string]string{"color": "red", "border": "1px solid"},
			"border:1px solid;color:red",
		},
		{
			"Props is accepted",
			Props{"color": "red"},
			"color:red",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := FormatStyle(c.value)
			if err != nil {
				t.Fatalf("FormatStyle(%#v): unexpected error: %v", c.value, err)
			}
			if got != c.want {
				t.Errorf("FormatStyle(%#v) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}

func TestFormatStyleErrors(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"a number is not a style", 42, "style prop of type int"},
		{"a slice is not a style", []string{"color:red"}, "style prop of type"},
		{"a bool value is not a declaration", map[string]any{"color": true}, `style value for "color"`},
		{"a struct value is not a declaration", map[string]any{"color": struct{}{}}, "unsupported type"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := FormatStyle(c.value)
			if err == nil {
				t.Fatalf("FormatStyle(%#v) = %q, want an error", c.value, got)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestSSRStyleName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"color", "color"},
		{"zIndex", "z-index"},
		{"backgroundColor", "background-color"},
		{"borderTopLeftRadius", "border-top-left-radius"},
		{"already-kebab", "already-kebab"},
		{"--custom", "--custom"},
		{"WebkitTransform", "-webkit-transform"},
		{"MozAppearance", "-moz-appearance"},
		{"msTransform", "-ms-transform"},
		{"ms", "ms"}, // too short to be a prefix; left alone
		{"", ""},
	}
	for _, c := range cases {
		if got := ssrStyleName(c.in); got != c.want {
			t.Errorf("ssrStyleName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSSREscapeHTML(t *testing.T) {
	// One escaper serves both text content and attribute values, matching
	// React: & < > " and ' are replaced, everything else is byte-identical.
	cases := []struct{ name, in, want string }{
		{"nothing to escape", "plain text", "plain text"},
		{"ampersand", "a & b", "a &amp; b"},
		{"less than", "a < b", "a &lt; b"},
		{"greater than", "a > b", "a &gt; b"},
		{"double quote", `say "hi"`, "say &quot;hi&quot;"},
		{"apostrophe uses the numeric form", "it's", "it&#39;s"},
		{"all five at once", `&<>"'`, "&amp;&lt;&gt;&quot;&#39;"},
		{"entities are re-escaped, never decoded", "&lt;", "&amp;lt;"},
		{"multibyte runes survive", "naïve — 日本", "naïve — 日本"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssrEscapeHTML(c.in); got != c.want {
				t.Errorf("ssrEscapeHTML(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSSRIsVoidElement(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"br", true}, {"img", true}, {"input", true}, {"meta", true},
		{"link", true}, {"hr", true}, {"wbr", true}, {"area", true},
		{"base", true}, {"col", true}, {"embed", true}, {"source", true},
		{"track", true},
		{"IMG", true}, // HTML tag names are case-insensitive
		{"div", false}, {"span", false}, {"p", false}, {"script", false},
		{"textarea", false}, {"", false},
	}
	for _, c := range cases {
		if got := ssrIsVoidElement(c.tag); got != c.want {
			t.Errorf("ssrIsVoidElement(%q) = %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestSSRAttrValueModes(t *testing.T) {
	cases := []struct {
		name  string
		prop  string
		value any
		want  string
		mode  ssrAttrMode
	}{
		// Boolean attributes: truthy renders the flag, everything falsy omits.
		{"nil omits", "disabled", nil, "", ssrAttrOmit},
		{"true is bare", "disabled", true, "", ssrAttrBare},
		{"false omits", "disabled", false, "", ssrAttrOmit},
		{"zero is falsy", "disabled", 0, "", ssrAttrOmit},
		{"one is truthy", "disabled", 1, "", ssrAttrBare},
		{"empty string is falsy", "disabled", "", "", ssrAttrOmit},
		{"the string false is truthy", "disabled", "false", "", ssrAttrBare},
		{"allowFullScreen is boolean", "allowFullScreen", true, "", ssrAttrBare},
		{"readOnly is boolean", "readOnly", false, "", ssrAttrOmit},
		{"itemScope is boolean", "itemScope", true, "", ssrAttrBare},
		{"inert is boolean", "inert", true, "", ssrAttrBare},
		{"defaultChecked is boolean", "defaultChecked", true, "", ssrAttrBare},

		// Enumerated ("overloaded string") attributes: false is a value.
		{"contentEditable false is a value", "contentEditable", false, "false", ssrAttrValued},
		{"contentEditable true is a value", "contentEditable", true, "true", ssrAttrValued},
		{"contentEditable string", "contentEditable", "plaintext-only", "plaintext-only", ssrAttrValued},
		{"spellCheck false is a value", "spellCheck", false, "false", ssrAttrValued},
		{"draggable false is a value", "draggable", false, "false", ssrAttrValued},
		{"focusable false is a value", "focusable", false, "false", ssrAttrValued},
		{"value false is a value", "value", false, "false", ssrAttrValued},
		{"enumerated nil still omits", "contentEditable", nil, "", ssrAttrOmit},

		// Overloaded booleans: a flag or a value, depending on the type.
		{"download true is bare", "download", true, "", ssrAttrBare},
		{"download false omits", "download", false, "", ssrAttrOmit},
		{"download empty string is a value", "download", "", "", ssrAttrValued},
		{"download string is a value", "download", "report.pdf", "report.pdf", ssrAttrValued},
		{"capture true is bare", "capture", true, "", ssrAttrBare},
		{"capture string is a value", "capture", "user", "user", ssrAttrValued},

		// Positive-integer attributes drop anything HTML would reject.
		{"cols 2", "cols", 2, "2", ssrAttrValued},
		{"cols 1", "cols", 1, "1", ssrAttrValued},
		{"cols 0 omits", "cols", 0, "", ssrAttrOmit},
		{"cols -1 omits", "cols", -1, "", ssrAttrOmit},
		{"cols non-numeric omits", "cols", "wide", "", ssrAttrOmit},
		{"cols numeric string", "cols", "3", "3", ssrAttrValued},
		{"rows 0 omits", "rows", 0, "", ssrAttrOmit},
		{"size 4", "size", 4, "4", ssrAttrValued},
		{"span 0 omits", "span", 0, "", ssrAttrOmit},

		// Plain-numeric attributes allow zero and negatives, reject non-numbers.
		{"rowSpan 0 is emitted", "rowSpan", 0, "0", ssrAttrValued},
		{"rowSpan -1 is emitted", "rowSpan", -1, "-1", ssrAttrValued},
		{"rowSpan non-numeric omits", "rowSpan", "wide", "", ssrAttrOmit},
		{"rowSpan empty string coerces to zero", "rowSpan", "", "", ssrAttrValued},
		{"start 0 is emitted", "start", 0, "0", ssrAttrValued},

		// String attributes: bools are dropped, unless data-*/aria-*.
		{"string", "id", "x", "x", ssrAttrValued},
		{"empty string still renders", "alt", "", "", ssrAttrValued},
		{"int formats like text", "tabIndex", 0, "0", ssrAttrValued},
		{"numeric zero is emitted not omitted", "maxLength", 0, "0", ssrAttrValued},
		{"float formats like text", "data-n", 2.50, "2.5", ssrAttrValued},
		{"bool on a string attribute omits", "title", true, "", ssrAttrOmit},
		{"bool on an unknown attribute omits", "myAttr", true, "", ssrAttrOmit},
		{"bool on data-* is a value", "data-open", true, "true", ssrAttrValued},
		{"false on data-* is a value", "data-open", false, "false", ssrAttrValued},
		{"bool on aria-* is a value", "aria-hidden", true, "true", ssrAttrValued},
		{"false on aria-* is a value", "aria-expanded", false, "false", ssrAttrValued},
		{"bool on ARIA- uppercase is a value", "ARIA-hidden", true, "true", ssrAttrValued},

		// src and href refuse the empty string, which would re-request the page.
		{"empty src omits", "src", "", "", ssrAttrOmit},
		{"empty href omits", "href", "", "", ssrAttrOmit},
		{"src with a value", "src", "/a.png", "/a.png", ssrAttrValued},

		// A func is never serialized, even when the "on" rule did not catch it.
		{"func omits", "title", func() {}, "", ssrAttrOmit},

		// Style.
		{"style map", "style", map[string]any{"top": 1}, "top:1px", ssrAttrValued},
		{"empty style omits", "style", map[string]any{}, "", ssrAttrOmit},
		{"empty style string omits", "style", "", "", ssrAttrOmit},

		// Dropped props have no value even when asked directly.
		{"key has no value", KeyProp, "k", "", ssrAttrOmit},
		{"suppressHydrationWarning has no value", "suppressHydrationWarning", true, "", ssrAttrOmit},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			attr, _ := AttributeName(c.prop)
			got, mode, err := ssrAttrValue(c.prop, attr, c.value)
			if err != nil {
				t.Fatalf("ssrAttrValue: unexpected error: %v", err)
			}
			if got != c.want || mode != c.mode {
				t.Errorf("ssrAttrValue(%q, %#v) = (%q, %v), want (%q, %v)",
					c.prop, c.value, got, mode, c.want, c.mode)
			}
		})
	}

	t.Run("a DangerousHTML in an ordinary prop is an error", func(t *testing.T) {
		_, _, err := ssrAttrValue("title", "title", DangerousHTML{HTML: "x"})
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("a bad style value is an error", func(t *testing.T) {
		_, _, err := ssrAttrValue("style", "style", 42)
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

// TestSSRAttributeParityWithReact pins whole-element output for one case per
// rule class. Every want string is byte-for-byte what react-dom 19.2 emits for
// the same element, copied from a renderToStaticMarkup probe rather than typed
// from memory; not one of them is a paraphrase.
//
// Keeping these as rendered markup rather than as (name, value) pairs is the
// point: it is the only form that can be diffed against upstream by eye.
func TestSSRAttributeParityWithReact(t *testing.T) {
	cases := []struct {
		name  string
		tag   string
		props Props
		want  string
	}{
		{"renamed alias", "div", Props{"className": "a b"}, `<div class="a b"></div>`},
		{"lowercased alias", "div", Props{"tabIndex": 0}, `<div tabindex="0"></div>`},
		{"camelCase kept verbatim", "input", Props{"maxLength": 10}, `<input maxLength="10"/>`},
		{"camelCase kept verbatim on td", "td", Props{"colSpan": 2}, `<td colSpan="2"></td>`},
		{"hyphenated svg alias", "path", Props{"strokeWidth": 2}, `<path stroke-width="2"></path>`},
		{"namespaced alias", "use", Props{"xlinkHref": "#a"}, `<use xlink:href="#a"></use>`},
		{"svg camelCase needs no entry", "svg", Props{"viewBox": "0 0 1 1"}, `<svg viewBox="0 0 1 1"></svg>`},

		{"boolean true", "button", Props{"disabled": true}, `<button disabled=""></button>`},
		{"boolean false", "button", Props{"disabled": false}, `<button></button>`},
		{"boolean keeps camelCase", "iframe", Props{"allowFullScreen": true}, `<iframe allowFullScreen=""></iframe>`},
		{"autoFocus is lowercased", "input", Props{"autoFocus": true}, `<input autofocus=""/>`},

		{"enumerated false is emitted", "div", Props{"contentEditable": false}, `<div contentEditable="false"></div>`},
		{"enumerated true is emitted", "div", Props{"spellCheck": true}, `<div spellCheck="true"></div>`},

		{"overloaded boolean flag", "a", Props{"download": true}, `<a download=""></a>`},
		{"overloaded boolean false", "a", Props{"download": false}, `<a></a>`},
		{"overloaded boolean empty value", "a", Props{"download": ""}, `<a download=""></a>`},
		{"overloaded boolean value", "input", Props{"capture": "user"}, `<input capture="user"/>`},

		{"positive numeric below one", "textarea", Props{"cols": 0}, `<textarea></textarea>`},
		{"positive numeric", "textarea", Props{"cols": 20}, `<textarea cols="20"></textarea>`},
		{"numeric zero", "td", Props{"rowSpan": 0}, `<td rowSpan="0"></td>`},
		{"numeric non-number", "td", Props{"rowSpan": "wide"}, `<td></td>`},

		{"unknown prop passes through", "div", Props{"myAttr": "1"}, `<div myAttr="1"></div>`},
		{"unknown bool prop is dropped", "div", Props{"myAttr": true}, `<div></div>`},
		{"data bool is stringified", "div", Props{"data-open": true}, `<div data-open="true"></div>`},
		{"aria bool is stringified", "div", Props{"aria-hidden": false}, `<div aria-hidden="false"></div>`},
		{"illegal name is dropped", "div", Props{"bad name": "x"}, `<div></div>`},

		{"empty src is dropped", "img", Props{"src": ""}, `<img/>`},
		{"handler is dropped", "div", Props{"onClick": "x"}, `<div></div>`},
		{"suppression props are dropped", "div", Props{"suppressHydrationWarning": true}, `<div></div>`},
		{"key and ref never appear", "div", Props{KeyProp: "k", RefProp: "r"}, `<div></div>`},

		{"defaultValue becomes value", "input", Props{"defaultValue": "a"}, `<input value="a"/>`},
		{"defaultChecked becomes checked", "input", Props{"defaultChecked": true}, `<input checked=""/>`},
		{"defaultChecked false omits", "input", Props{"defaultChecked": false}, `<input/>`},
		{"selected on option", "option", Props{"selected": true}, `<option selected=""></option>`},

		{"numeric zero is not falsy", "div", Props{"title": 0}, `<div title="0"></div>`},
		{"nil drops", "div", Props{"title": nil}, `<div></div>`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(&Element{Type: c.tag, Props: c.props})
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != c.want {
				t.Errorf("render = %s, want %s", got, c.want)
			}
		})
	}
}

// TestSSRAttributeNamesMatchUpstream guards the table's shape rather than its
// contents: every entry must map to a name that could be written into a tag, and
// no entry may be a no-op, since a no-op entry means someone added a rename that
// React does not perform and then "fixed" it by making it identity.
func TestSSRAttributeNamesMatchUpstream(t *testing.T) {
	for prop, attr := range ssrAttributeNames {
		if attr == "" {
			t.Errorf("ssrAttributeNames[%q] is empty", prop)
			continue
		}
		if !ssrIsAttributeNameSafe(attr) {
			t.Errorf("ssrAttributeNames[%q] = %q, which is not a legal attribute name", prop, attr)
		}
		if attr == prop && prop != "panose-1" && prop != "multiple" && prop != "muted" {
			t.Errorf("ssrAttributeNames[%q] is an identity mapping; delete it", prop)
		}
	}
	// The event-handler rule must not shadow a real rename.
	for prop := range ssrAttributeNames {
		if ssrIsEventHandlerProp(prop) {
			t.Errorf("ssrAttributeNames[%q] can never be reached: it looks like a handler", prop)
		}
	}
}

func TestSSRDangerousContent(t *testing.T) {
	cases := []struct {
		name    string
		props   Props
		want    string
		present bool
		wantErr bool
	}{
		{"absent", Props{"id": "x"}, "", false, false},
		{"nil props", nil, "", false, false},
		{"value", Props{"dangerouslySetInnerHTML": DangerousHTML{HTML: "<b/>"}}, "<b/>", true, false},
		{"pointer", Props{"dangerouslySetInnerHTML": &DangerousHTML{HTML: "<i/>"}}, "<i/>", true, false},
		{"nil interface", Props{"dangerouslySetInnerHTML": nil}, "", false, false},
		{"nil pointer", Props{"dangerouslySetInnerHTML": (*DangerousHTML)(nil)}, "", false, false},
		{"empty markup is still present", Props{"dangerouslySetInnerHTML": DangerousHTML{}}, "", true, false},
		{"wrong type", Props{"dangerouslySetInnerHTML": "<b/>"}, "", false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html, present, err := ssrDangerousContent(c.props)
			if (err != nil) != c.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			if html != c.want || present != c.present {
				t.Errorf("ssrDangerousContent = (%q, %v), want (%q, %v)",
					html, present, c.want, c.present)
			}
		})
	}
}
