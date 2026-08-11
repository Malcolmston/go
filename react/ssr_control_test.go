package react

import (
	"strings"
	"testing"
)

// Every want string in this file is a byte-for-byte transcript of a
// renderToStaticMarkup probe against the pinned oracle, react-dom 19.2.7, run
// from parity/react/node. The probe source and its raw output are reproduced in
// the case comments where the shape is surprising; nothing here is recalled.

func TestSSRSelectValueIsNotAnAttribute(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
		want string
	}{
		// The headline fix. A select's value picks an option; it never reaches
		// the markup itself.
		{
			"value selects the matching option",
			ssrTag("select", Props{"value": "b"},
				ssrTag("option", Props{"value": "a"}, "A"),
				ssrTag("option", Props{"value": "b"}, "B")),
			`<select><option value="a">A</option><option value="b" selected="">B</option></select>`,
		},
		{
			"defaultValue behaves the same",
			ssrTag("select", Props{"defaultValue": "a"}, ssrTag("option", Props{"value": "a"}, "A")),
			`<select><option value="a" selected="">A</option></select>`,
		},
		{
			"value wins over defaultValue",
			ssrTag("select", Props{"value": "a", "defaultValue": "b"},
				ssrTag("option", Props{"value": "a"}, "A"),
				ssrTag("option", Props{"value": "b"}, "B")),
			`<select><option value="a" selected="">A</option><option value="b">B</option></select>`,
		},
		{
			// React's prop loop skips null, so a nil value is the same as none.
			"nil value selects nothing",
			ssrTag("select", Props{"value": nil}, ssrTag("option", Props{"value": "a"}, "A")),
			`<select><option value="a">A</option></select>`,
		},
		{
			"other props still become attributes",
			ssrTag("select", Props{"value": "a", "name": "n"}, ssrTag("option", Props{"value": "a"}, "A")),
			`<select name="n"><option value="a" selected="">A</option></select>`,
		},
		{
			// A list value matches every option in it. React branches on
			// isArray(selectedValue) and never looks at multiple.
			"multiple with a list value",
			ssrTag("select", Props{"multiple": true, "defaultValue": []string{"a", "c"}},
				ssrTag("option", Props{"value": "a"}, "A"),
				ssrTag("option", Props{"value": "b"}, "B"),
				ssrTag("option", Props{"value": "c"}, "C")),
			`<select multiple=""><option value="a" selected="">A</option>` +
				`<option value="b">B</option><option value="c" selected="">C</option></select>`,
		},
		{
			"a list value without multiple still matches",
			ssrTag("select", Props{"value": []string{"a"}}, ssrTag("option", Props{"value": "a"}, "A")),
			`<select><option value="a" selected="">A</option></select>`,
		},
		{
			// Both sides are stringified before the comparison, so 2 and "2"
			// are the same option.
			"a numeric value matches a string option",
			ssrTag("select", Props{"value": 2}, ssrTag("option", Props{"value": "2"}, "two")),
			`<select><option value="2" selected="">two</option></select>`,
		},
		{
			"an empty-string value matches an empty-string option",
			ssrTag("select", Props{"value": ""}, ssrTag("option", Props{"value": ""}, "blank")),
			`<select><option value="" selected="">blank</option></select>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.el)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("render =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestSSROptionSelectedDecision(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
		want string
	}{
		{
			// With no value in scope, the option's own prop is honoured, which
			// is what this port already did.
			"no select value honours the option's selected prop",
			ssrTag("select", nil, ssrTag("option", Props{"value": "a", "selected": true}, "A")),
			`<select><option value="a" selected="">A</option></select>`,
		},
		{
			// And with a value in scope the option's prop is read out of the
			// loop and thrown away.
			"a select value overrides the option's selected prop",
			ssrTag("select", Props{"value": "a"},
				ssrTag("option", Props{"value": "b", "selected": true}, "B")),
			`<select><option value="b">B</option></select>`,
		},
		{
			"an option with no value is identified by its children",
			ssrTag("select", Props{"value": "B"},
				ssrTag("option", nil, "A"),
				ssrTag("option", nil, "B")),
			`<select><option>A</option><option selected="">B</option></select>`,
		},
		{
			"a value prop beats the children",
			ssrTag("select", Props{"value": "A"}, ssrTag("option", Props{"value": "x"}, "A")),
			`<select><option value="x">A</option></select>`,
		},
		{
			// An empty value is still a value, so the children are not consulted.
			"an empty value prop beats the children",
			ssrTag("select", Props{"value": "A"}, ssrTag("option", Props{"value": ""}, "A")),
			`<select><option value="">A</option></select>`,
		},
		{
			"an option with neither matches the empty string",
			ssrTag("select", Props{"value": ""}, ssrTag("option", nil)),
			`<select><option selected=""></option></select>`,
		},
		{
			// React flattens the children with string coercion, so an element
			// child contributes "[object Object]" and cannot match "a".
			"an element child cannot identify an option",
			ssrTag("select", Props{"value": "a"}, ssrTag("option", nil, ssrTag("b", nil, "a"))),
			`<select><option><b>a</b></option></select>`,
		},
		{
			"an option outside any select keeps its selected prop",
			ssrTag("option", Props{"value": "a", "selected": true}, "A"),
			`<option value="a" selected="">A</option>`,
		},
		{
			"an option outside any select without the prop stays plain",
			ssrTag("option", Props{"value": "a"}, "A"),
			`<option value="a">A</option>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.el)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("render =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestSSRSelectedValueScope(t *testing.T) {
	// The tags React resets selectedValue for in getChildFormatContext, against
	// the ordinary flow content that inherits it. Each row was probed one by one.
	inherits := []string{"div", "span", "p", "label", "fieldset", "optgroup", "template", "head"}
	barriers := []string{"noscript", "svg", "picture", "math", "table", "thead", "tbody", "tfoot", "colgroup", "tr"}

	for _, tag := range inherits {
		el := ssrTag("select", Props{"value": "a"},
			ssrTag(tag, nil, ssrTag("option", Props{"value": "a"}, "A")))
		want := "<select><" + tag + `><option value="a" selected="">A</option></` + tag + "></select>"
		got, err := RenderToStaticMarkup(el)
		if err != nil {
			t.Fatalf("<%s>: render: %v", tag, err)
		}
		if got != want {
			t.Errorf("<%s> should inherit the select value: got %q, want %q", tag, got, want)
		}
	}

	for _, tag := range barriers {
		el := ssrTag("select", Props{"value": "a"},
			ssrTag(tag, nil, ssrTag("option", Props{"value": "a"}, "A")))
		want := "<select><" + tag + `><option value="a">A</option></` + tag + "></select>"
		got, err := RenderToStaticMarkup(el)
		if err != nil {
			t.Fatalf("<%s>: render: %v", tag, err)
		}
		if got != want {
			t.Errorf("<%s> should stop the select value: got %q, want %q", tag, got, want)
		}
	}

	// foreignObject is its own barrier upstream; inside an <svg> it is reached
	// only after svg has already cleared the value, so this asserts the pair.
	el := ssrTag("select", Props{"value": "a"},
		ssrTag("svg", nil, ssrTag("foreignObject", nil, ssrTag("option", Props{"value": "a"}, "A"))))
	want := `<select><svg><foreignObject><option value="a">A</option></foreignObject></svg></select>`
	if got, err := RenderToStaticMarkup(el); err != nil || got != want {
		t.Errorf("svg > foreignObject = %q, %v, want %q", got, err, want)
	}

	// A nested select shadows the outer one for its own subtree.
	el = ssrTag("select", Props{"value": "a"},
		ssrTag("div", nil, ssrTag("select", Props{"value": "b"},
			ssrTag("option", Props{"value": "a"}, "A"),
			ssrTag("option", Props{"value": "b"}, "B"))))
	want = `<select><div><select><option value="a">A</option>` +
		`<option value="b" selected="">B</option></select></div></select>`
	if got, err := RenderToStaticMarkup(el); err != nil || got != want {
		t.Errorf("nested select = %q, %v, want %q", got, err, want)
	}

	// A select with neither value nor defaultValue provides no value at all, so
	// an option below it is back on its own prop.
	el = ssrTag("select", nil, ssrTag("div", nil,
		ssrTag("option", Props{"value": "a", "selected": true}, "A")))
	want = `<select><div><option value="a" selected="">A</option></div></select>`
	if got, err := RenderToStaticMarkup(el); err != nil || got != want {
		t.Errorf("valueless select = %q, %v, want %q", got, err, want)
	}
}

func TestSSRSelectedValueThroughTransparentFibers(t *testing.T) {
	// Components and fragments contribute no markup, so they must not interrupt
	// the value either: React's format context is threaded through them.
	option := func(Props) Node { return ssrTag("option", Props{"value": "a"}, "A") }

	el := ssrTag("select", Props{"value": "a"}, ssrFrag(ssrComp(option, nil)))
	want := `<select><option value="a" selected="">A</option></select>`
	if got, err := RenderToStaticMarkup(el); err != nil || got != want {
		t.Errorf("render = %q, %v, want %q", got, err, want)
	}
}

func TestSSRTextareaValueIsTextContent(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
		want string
	}{
		{
			"value becomes the content",
			ssrTag("textarea", Props{"id": "x", "value": "t"}),
			`<textarea id="x">t</textarea>`,
		},
		{
			"defaultValue behaves the same",
			ssrTag("textarea", Props{"defaultValue": "d"}),
			`<textarea>d</textarea>`,
		},
		{
			"value wins over defaultValue",
			ssrTag("textarea", Props{"value": "v", "defaultValue": "d"}),
			`<textarea>v</textarea>`,
		},
		{
			"other props still become attributes",
			ssrTag("textarea", Props{"value": "t", "rows": 3}),
			`<textarea rows="3">t</textarea>`,
		},
		{
			// The content is escaped, exactly as any other text is: React writes
			// escapeTextForBrowser here, apostrophe included.
			"the content is escaped",
			ssrTag("textarea", Props{"value": `<b>&"'</b>`}),
			`<textarea>&lt;b&gt;&amp;&quot;&#x27;&lt;/b&gt;</textarea>`,
		},
		{
			// The HTML parser drops the first newline after <textarea>, so React
			// writes a spare one to keep the value intact.
			"a leading newline is doubled",
			ssrTag("textarea", Props{"value": "\nhello"}),
			"<textarea>\n\nhello</textarea>",
		},
		{
			"a lone newline is doubled too",
			ssrTag("textarea", Props{"value": "\n"}),
			"<textarea>\n\n</textarea>",
		},
		{
			// The test is on the first character only, so a CRLF value is left
			// alone.
			"a leading carriage return is not doubled",
			ssrTag("textarea", Props{"value": "\r\nx"}),
			"<textarea>\r\nx</textarea>",
		},
		{
			"an empty value renders an empty textarea",
			ssrTag("textarea", Props{"value": ""}),
			`<textarea></textarea>`,
		},
		{
			"a nil value renders an empty textarea",
			ssrTag("textarea", Props{"value": nil}),
			`<textarea></textarea>`,
		},
		{
			"no value at all renders an empty textarea",
			ssrTag("textarea", Props{"rows": 2}),
			`<textarea rows="2"></textarea>`,
		},
		{
			// A number is stringified, and the newline rule does not apply to it.
			"a numeric value is stringified",
			ssrTag("textarea", Props{"value": 0}),
			`<textarea>0</textarea>`,
		},
		{
			// String(['a','b']) is "a,b".
			"a list value joins with commas",
			ssrTag("textarea", Props{"value": []string{"a", "b"}}),
			`<textarea>a,b</textarea>`,
		},
		{
			"children stand in for a value when there is none",
			ssrTag("textarea", nil, "kid"),
			`<textarea>kid</textarea>`,
		},
		{
			"children get the leading-newline treatment too",
			ssrTag("textarea", nil, "\nkid"),
			"<textarea>\n\nkid</textarea>",
		},
		{
			"a numeric child is stringified",
			ssrTag("textarea", nil, 5),
			`<textarea>5</textarea>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.el)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if got != tt.want {
				t.Errorf("render =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}

func TestSSRTextareaErrors(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
		want string // substring the error must contain
	}{
		{
			// Upstream: "If you supply `defaultValue` on a <textarea>, do not
			// pass children."
			"value and children together",
			ssrTag("textarea", Props{"value": "v"}, "kid"),
			"both children and a value prop",
		},
		{
			"defaultValue and children together",
			ssrTag("textarea", Props{"defaultValue": "d"}, "kid"),
			"both children and a defaultValue prop",
		},
		{
			// Upstream: "<textarea> can only have at most one child."
			"more than one child",
			ssrTag("textarea", nil, "a", "b"),
			"more than one child",
		},
		{
			// Upstream throws: "`dangerouslySetInnerHTML` does not make sense on
			// <textarea>."
			"dangerouslySetInnerHTML",
			ssrTag("textarea", Props{ssrDangerousProp: DangerousHTML{HTML: "<i>x</i>"}}),
			"cannot use " + ssrDangerousProp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tt.el)
			if err == nil {
				t.Fatalf("render = %q, want an error", got)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestSSRControlFiberLeavesOtherTagsAlone(t *testing.T) {
	// The rewrite is per-tag: value on anything else is an ordinary attribute,
	// and an element with none of the controlled props is not copied at all.
	for _, tt := range []struct {
		tag  string
		want string
	}{
		{"div", `<div value="a"></div>`},
		{"input", `<input value="a"/>`},
	} {
		el := ssrTag(tt.tag, Props{"value": "a"})
		got, err := RenderToStaticMarkup(el)
		if err != nil {
			t.Fatalf("<%s>: render: %v", tt.tag, err)
		}
		if got != tt.want {
			t.Errorf("<%s> = %q, want %q", tt.tag, got, tt.want)
		}
	}

	f := &fiber{typ: "select", props: Props{"name": "n"}}
	if ssrControlFiber(f, "select") != f {
		t.Error("a select with no value props should not be copied")
	}
	f = &fiber{typ: "option", props: Props{"value": "a"}}
	if ssrControlFiber(f, "option") != f {
		t.Error("an unselected option with no selected prop should not be copied")
	}
	f = &fiber{typ: "div", props: Props{"value": "a"}}
	if ssrControlFiber(f, "div") != f {
		t.Error("a non-controlled tag should not be copied")
	}
}

func TestSSRValueListAndStringValue(t *testing.T) {
	listTests := []struct {
		name   string
		in     any
		isList bool
	}{
		{"string slice", []string{"a"}, true},
		{"any slice", []any{1, "a"}, true},
		{"array", [2]int{1, 2}, true},
		{"string", "ab", false},
		{"byte slice is text", []byte("ab"), false},
		{"nil", nil, false},
		{"int", 3, false},
		{"map", map[string]string{"a": "b"}, false},
	}
	for _, tt := range listTests {
		if _, got := ssrValueList(tt.in); got != tt.isList {
			t.Errorf("%s: ssrValueList list = %v, want %v", tt.name, got, tt.isList)
		}
	}

	// JavaScript's "" + value, which is what every comparison in this file is
	// defined against.
	stringTests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"a", "a"},
		{2, "2"},
		{2.5, "2.5"},
		{true, "true"},
		{[]string{"a", "b"}, "a,b"},
		{[]any{"a", nil, "b"}, "a,,b"},
		{[]any{"a", []string{"b", "c"}}, "a,b,c"},
		{[]string{}, ""},
	}
	for _, tt := range stringTests {
		if got := ssrJSStringValue(tt.in); got != tt.want {
			t.Errorf("ssrJSStringValue(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSSRSelectedValueBarrierTable(t *testing.T) {
	for _, tag := range []string{
		"noscript", "svg", "picture", "math", "foreignObject",
		"table", "thead", "tbody", "tfoot", "colgroup", "tr", PortalTag,
	} {
		if !ssrSelectedValueBarrier(tag) {
			t.Errorf("ssrSelectedValueBarrier(%q) = false, want true", tag)
		}
	}
	// head resets selectedValue upstream only when it appears outside the
	// document body, which cannot happen inside a select; probed as inheriting.
	for _, tag := range []string{"div", "span", "p", "optgroup", "label", "td", "head", "html", "select", ""} {
		if ssrSelectedValueBarrier(tag) {
			t.Errorf("ssrSelectedValueBarrier(%q) = true, want false", tag)
		}
	}
}
