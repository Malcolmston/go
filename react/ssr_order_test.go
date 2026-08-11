package react

import (
	"reflect"
	"sort"
	"testing"
)

// sortedNames mirrors what ssrEmitter.attributes hands to ssrAttributeOrder.
func sortedNames(props Props) []string {
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func TestSSRAttributeOrder(t *testing.T) {
	tests := []struct {
		name  string
		tag   string
		props Props
		want  []string
	}{
		{
			name:  "tag without a hand-written serializer is untouched",
			tag:   "div",
			props: Props{"id": "d", "className": "c", "value": "v"},
			want:  []string{"className", "id", "value"},
		},
		{
			name:  "input defers name past the sorted props",
			tag:   "input",
			props: Props{"id": "q", "name": "q", "required": true, "type": "text"},
			want:  []string{"id", "required", "type", "name"},
		},
		{
			name:  "input emits checked then value last",
			tag:   "input",
			props: Props{"value": "v", "checked": true, "type": "checkbox", "name": "n", "id": "i"},
			want:  []string{"id", "type", "name", "checked", "value"},
		},
		{
			name:  "input defaults take the controlled slots when alone",
			tag:   "input",
			props: Props{"defaultValue": "dv", "defaultChecked": true, "type": "checkbox", "id": "i"},
			want:  []string{"id", "type", "defaultChecked", "defaultValue"},
		},
		{
			name:  "input controlled props beat the defaults, which are dropped",
			tag:   "input",
			props: Props{"value": "v", "defaultValue": "dv", "checked": true, "defaultChecked": false, "type": "checkbox", "id": "i"},
			want:  []string{"id", "type", "checked", "value"},
		},
		{
			name:  "input nil controlled prop leaves the default in charge",
			tag:   "input",
			props: Props{"value": nil, "defaultValue": "dv", "type": "text", "id": "i"},
			want:  []string{"id", "type", "value", "defaultValue"},
		},
		{
			name:  "input false checked still beats defaultChecked",
			tag:   "input",
			props: Props{"checked": false, "defaultChecked": true, "type": "checkbox", "id": "i"},
			want:  []string{"id", "type", "checked"},
		},
		{
			name: "input form-action props come after everything else in React's order",
			tag:  "input",
			props: Props{
				"type": "submit", "id": "i", "name": "n",
				"formTarget": "_blank", "formMethod": "post",
				"formEncType": "text/plain", "formAction": "/a",
			},
			want: []string{"id", "type", "name", "formAction", "formEncType", "formMethod", "formTarget"},
		},
		{
			name:  "button defers name",
			tag:   "button",
			props: Props{"id": "i", "name": "b", "type": "submit"},
			want:  []string{"id", "type", "name"},
		},
		{
			name: "button defers the whole form-action set",
			tag:  "button",
			props: Props{
				"formAction": "/a", "formMethod": "post", "formEncType": "x",
				"formTarget": "_t", "name": "b", "id": "i", "type": "submit",
			},
			want: []string{"id", "type", "name", "formAction", "formEncType", "formMethod", "formTarget"},
		},
		{
			name:  "button leaves value alone: it is not deferred",
			tag:   "button",
			props: Props{"value": "v", "id": "i", "name": "b"},
			want:  []string{"id", "value", "name"},
		},
		{
			name: "form defers action, encType, method and target",
			tag:  "form",
			props: Props{
				"acceptCharset": "utf-8", "action": "/a", "method": "post",
				"target": "_x", "encType": "multipart/form-data", "id": "f",
			},
			want: []string{"acceptCharset", "id", "action", "encType", "method", "target"},
		},
		{
			name:  "form with only sorted-stable props is unchanged",
			tag:   "form",
			props: Props{"acceptCharset": "utf-8", "action": "/a", "method": "post"},
			want:  []string{"acceptCharset", "action", "method"},
		},
		{
			name:  "option defers selected but leaves value in place",
			tag:   "option",
			props: Props{"selected": true, "value": "a"},
			want:  []string{"value", "selected"},
		},
		{
			name:  "option keeps non-deferred props sorted around value",
			tag:   "option",
			props: Props{"zzz": "1", "aaa": "2", "value": "a", "selected": true},
			want:  []string{"aaa", "value", "zzz", "selected"},
		},
		{
			name:  "option without selected is unchanged",
			tag:   "option",
			props: Props{"value": "a", "id": "o"},
			want:  []string{"id", "value"},
		},
		{
			name:  "empty props",
			tag:   "input",
			props: Props{},
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ssrAttributeOrder(tc.tag, tc.props, sortedNames(tc.props))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ssrAttributeOrder(%q) = %v, want %v", tc.tag, got, tc.want)
			}
		})
	}
}

// TestSSRAttributeOrderMarkup pins the rendered bytes against real React's
// output for the same props, captured from react-dom 19.2.7's
// renderToStaticMarkup. The prop maps here are ones whose non-deferred props
// happen to be in sorted order already, so the port can match React exactly.
func TestSSRAttributeOrderMarkup(t *testing.T) {
	tests := []struct {
		name string
		el   *Element
		want string
	}{
		{
			name: "input name last",
			el:   CreateElement("input", Props{"id": "q", "name": "q", "required": true, "type": "text"}),
			want: `<input id="q" required="" type="text" name="q"/>`,
		},
		{
			name: "input checked and value last",
			el:   CreateElement("input", Props{"id": "i", "name": "n", "type": "checkbox", "checked": true, "value": "v"}),
			want: `<input id="i" type="checkbox" name="n" checked="" value="v"/>`,
		},
		{
			name: "input defaults render as the controlled attributes",
			el:   CreateElement("input", Props{"id": "i", "type": "checkbox", "defaultChecked": true, "defaultValue": "dv"}),
			want: `<input id="i" type="checkbox" checked="" value="dv"/>`,
		},
		{
			name: "input controlled wins over default without duplicating",
			el:   CreateElement("input", Props{"id": "i", "type": "checkbox", "checked": true, "defaultChecked": false, "value": "v", "defaultValue": "dv"}),
			want: `<input id="i" type="checkbox" checked="" value="v"/>`,
		},
		{
			name: "button name last",
			el:   CreateElement("button", Props{"id": "i", "name": "b", "type": "submit"}, "Go"),
			want: `<button id="i" type="submit" name="b">Go</button>`,
		},
		{
			name: "form action set last",
			el:   CreateElement("form", Props{"acceptCharset": "utf-8", "id": "f", "action": "/a", "method": "post"}, "x"),
			want: `<form accept-charset="utf-8" id="f" action="/a" method="post">x</form>`,
		},
		{
			name: "option selected last",
			el:   CreateElement("option", Props{"value": "a", "selected": true}, "o"),
			want: `<option value="a" selected="">o</option>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderToStaticMarkup(tc.el)
			if err != nil {
				t.Fatalf("RenderToStaticMarkup: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
