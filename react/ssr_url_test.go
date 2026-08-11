package react

import "testing"

func TestSSRIsJavaScriptURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain", "javascript:alert(1)", true},
		{"mixed case", "JavaScript:alert(1)", true},
		{"upper case", "JAVASCRIPT:x", true},
		{"leading spaces", "  javascript:alert(1)", true},
		{"leading control chars", "\x01\x1fjavascript:x", true},
		{"leading mixed whitespace", " \t\n javascript:x", true},
		{"tab inside scheme", "java\tscript:alert(1)", true},
		{"cr inside scheme", "j\ravascript:alert(1)", true},
		{"lf before colon", "javascript\n:alert(1)", true},
		{"tab before colon", "javascript\t:x", true},
		{"tab between every letter", "j\ta\tv\ta\ts\tc\tr\ti\tp\tt\t:x", true},
		{"no payload after colon", "javascript:", true},

		{"no colon", "javascript", false},
		{"prefixed", "xjavascript:x", false},
		{"leading punctuation", "!javascript:x", false},
		{"other scheme", "vbscript:x", false},
		{"data url", "data:text/html,x", false},
		{"relative path", "/x?a=1", false},
		{"empty", "", false},
		{"space inside scheme", "java script:x", false},
		{"non-ascii lookalike", "ｊavascript:x", false},
		{"truncated scheme", "javascrip:x", false},
		{"colon only", ":", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ssrIsJavaScriptURL(tc.in); got != tc.want {
				t.Fatalf("ssrIsJavaScriptURL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSSRSanitizeURLValue(t *testing.T) {
	tests := []struct {
		name    string
		prop    string
		value   string
		want    string
		blocked bool
	}{
		{"href", "href", "javascript:alert(1)", ssrBlockedJavaScriptURL, true},
		{"src", "src", "javascript:alert(1)", ssrBlockedJavaScriptURL, true},
		{"action", "action", "javascript:alert(1)", ssrBlockedJavaScriptURL, true},
		{"formAction", "formAction", "javascript:alert(1)", ssrBlockedJavaScriptURL, true},
		{"xlinkHref", "xlinkHref", "javascript:alert(1)", ssrBlockedJavaScriptURL, true},

		{"href safe", "href", "/about", "/about", false},
		{"href data url", "href", "data:text/html,x", "data:text/html,x", false},
		{"unlisted prop", "data-x", "javascript:alert(1)", "javascript:alert(1)", false},
		{"title prop", "title", "javascript:alert(1)", "javascript:alert(1)", false},
		{"cite prop", "cite", "javascript:alert(1)", "javascript:alert(1)", false},
		{"resolved attr name is not the key", "xlink:href", "javascript:x", "javascript:x", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, blocked := ssrSanitizeURLValue(tc.prop, tc.value)
			if got != tc.want || blocked != tc.blocked {
				t.Fatalf("ssrSanitizeURLValue(%q, %q) = (%q, %v), want (%q, %v)",
					tc.prop, tc.value, got, blocked, tc.want, tc.blocked)
			}
		})
	}
}

// TestSSRSanitizeURLRendered pins the rendered markup, since the replacement is
// escaped like any other attribute value and the whole point is the bytes.
func TestSSRSanitizeURLRendered(t *testing.T) {
	esc := ssrEscapeText(ssrBlockedJavaScriptURL)

	tests := []struct {
		name string
		el   *Element
		want string
	}{
		{
			name: "anchor href is blocked",
			el:   CreateElement("a", Props{"href": "javascript:alert('x')"}, "l"),
			want: `<a href="` + esc + `">l</a>`,
		},
		{
			name: "obfuscated scheme is blocked",
			el:   CreateElement("a", Props{"href": "  java\tscript:alert(1)"}, "l"),
			want: `<a href="` + esc + `">l</a>`,
		},
		{
			name: "form action is blocked",
			el:   CreateElement("form", Props{"action": "javascript:alert(1)"}, "l"),
			want: `<form action="` + esc + `">l</form>`,
		},
		{
			name: "img src is blocked",
			el:   CreateElement("img", Props{"src": "javascript:alert(1)"}),
			want: `<img src="` + esc + `"/>`,
		},
		{
			name: "the rule is keyed by prop, not tag",
			el:   CreateElement("div", Props{"href": "javascript:alert(1)"}),
			want: `<div href="` + esc + `"></div>`,
		},
		{
			name: "data attribute is untouched",
			el:   CreateElement("div", Props{"data-x": "javascript:alert(1)"}),
			want: `<div data-x="javascript:alert(1)"></div>`,
		},
		{
			name: "other schemes are untouched",
			el:   CreateElement("a", Props{"href": "vbscript:x"}, "l"),
			want: `<a href="vbscript:x">l</a>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tc.el)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}
