package react

import (
	"fmt"
	"reflect"
	"strings"
)

// Controlled form values: <select>, <option> and <textarea>.
//
// These three tags are the place where React's server renderer stops treating a
// prop as an attribute. `value` on a <div> is an attribute; `value` on a
// <select> is a piece of context that decides which <option> below it carries
// `selected=""`, and `value` on a <textarea> is the element's text content. A
// generic prop loop cannot reproduce any of that, which is why react-dom has a
// hand-written serializer for each of the three, and why this file exists.
//
// Everything below was read off react-dom 19.2.7 -- the `case "select"`,
// `case "option"` and `case "textarea"` arms of pushStartInstance in
// react-dom-server-legacy.node.development.js, plus getChildFormatContext --
// and confirmed by probe renders quoted in ssr_control_test.go.
//
// The three rules, in the order the renderer applies them:
//
//   - <select> consumes `value` and `defaultValue`: neither is ever emitted as
//     an attribute. The effective value (value if non-nil, else defaultValue)
//     becomes the subtree's "selected value".
//     Probe: <select value="a"><option value="a">A</option></select>
//     ->     <select><option value="a" selected="">A</option></select>
//
//   - <option>, when a selected value is in scope, decides `selected=""` by
//     comparing its own stringified `value` prop -- or, when it has none, the
//     flattened string form of its children -- against the selected value, or
//     against any member of it when it is a list. The option's own `selected`
//     prop is read out of the prop loop and then ignored.
//     Probe: <select value="a"><option value="a"/><option value="b" selected/></select>
//     ->     <select><option value="a" selected=""></option><option value="b"></option></select>
//     With no selected value in scope, `selected` is honoured as written, which
//     is what an <option> outside any <select> already did in this port.
//
//   - <textarea> consumes `value` and `defaultValue` too, and emits the
//     effective value as escaped text content. Children are an error when a
//     value is also set, dangerouslySetInnerHTML is always an error, and a
//     string value beginning with a newline gets one extra leading newline --
//     the HTML parser eats the first newline after <textarea>, so React pushes a
//     spare one to keep the value intact through a round trip.
//     Probe: <textarea id="x" value="t"/>   -> <textarea id="x">t</textarea>
//     Probe: <textarea value={"\nhello"}/>  -> <textarea>\n\nhello</textarea>
//
// The port reaches the selected value by walking up the fiber tree from the
// option rather than threading a field down the emitter. The two are equivalent
// -- SSR mounts the whole tree before a byte is written, so every option's
// ancestors are already there -- and the walk keeps the shared emitter in
// ssr.go untouched. What the walk has to reproduce is which elements *stop*
// selectedValue from being inherited, because React resets it in
// getChildFormatContext for a fixed set of tags; see ssrSelectedValueBarrier.

// ssrControlValueProps are the prop names <select> and <textarea> consume rather
// than emit. Both spellings are listed because the port aliases defaultValue to
// the value attribute, so leaving either behind would put it in the markup.
var ssrControlValueProps = [...]string{"value", "defaultValue"}

// ssrSelectedValueBarrier reports whether tag stops an enclosing <select>'s
// value from reaching the options below it.
//
// React resets selectedValue to null in getChildFormatContext for noscript,
// svg, picture, math, foreignObject and every table-structure tag, and again
// for any element whose insertion mode leaves HTML_MODE -- which is what makes
// <td> inside a <table> reset too, already covered here by the table itself.
// Ordinary flow content (div, span, p, label, optgroup, template, and head in
// HTML mode) inherits; all of these were probed one by one.
func ssrSelectedValueBarrier(tag string) bool {
	switch tag {
	case "noscript", "svg", "picture", "math", "foreignObject",
		"table", "thead", "tbody", "tfoot", "colgroup", "tr":
		return true
	case PortalTag:
		// Not a React tag at all. A portal's content is emitted somewhere else
		// entirely, so letting a <select> it happens to sit inside select an
		// option in another document position would be meaningless.
		return true
	}
	return false
}

// ssrSelectedValue returns the value of the <select> governing f, if any.
//
// f itself is not examined: the walk starts at its parent, so calling this on a
// <select> reports the select above it, not itself. Non-host fibers --
// components, fragments, providers -- are transparent, exactly as they are for
// emission. A select whose value and defaultValue are both absent ends the walk
// with ok=false, matching React, where the subtree context is set to null and an
// option below falls back to its own `selected` prop.
func ssrSelectedValue(f *fiber) (any, bool) {
	for p := f.parent; p != nil; p = p.parent {
		tag, isHost := p.typ.(string)
		if !isHost {
			continue
		}
		if tag == "select" {
			for _, prop := range ssrControlValueProps {
				if v := p.props[prop]; v != nil {
					return v, true
				}
			}
			return nil, false
		}
		if ssrSelectedValueBarrier(tag) {
			return nil, false
		}
	}
	return nil, false
}

// ssrOptionSelected reports whether f, an <option> fiber, should carry
// selected="".
func ssrOptionSelected(f *fiber) bool {
	selectedValue, ok := ssrSelectedValue(f)
	if !ok {
		// No <select> in scope: React's `else selected && push(' selected=""')`.
		// The nil guard is not decoration -- ssrTruthy answers true for a value
		// it cannot classify, and an absent prop reads back as a nil any.
		own := f.props["selected"]
		return own != nil && ssrTruthy(own)
	}
	own := ssrOptionStringValue(f)
	for _, candidate := range ssrValueStrings(selectedValue) {
		if candidate == own {
			return true
		}
	}
	return false
}

// ssrOptionStringValue returns the string an <option> is identified by: its
// `value` prop when it has one, otherwise the flattened form of its children.
//
// A nil value counts as absent, because React's prop loop tests `null !=
// propValue` before recording it, so <option value={null}>x</option> is matched
// on "x".
func ssrOptionStringValue(f *fiber) string {
	if v := f.props["value"]; v != nil {
		return ssrJSStringValue(v)
	}
	return ssrFlattenOptionChildren(f)
}

// ssrFlattenOptionChildren is React's flattenOptionChildren: every child
// stringified and concatenated.
//
// A child that is not a primitive stringifies to "[object Object]" in
// JavaScript, and that is emitted literally here rather than being replaced by
// the child's own text, because the whole point of the string is to be compared
// against the select's value and React compares against that exact garbage. It
// is what makes <option><b>a</b></option> *not* match value="a" upstream.
func ssrFlattenOptionChildren(f *fiber) string {
	var b strings.Builder
	for c := f.child; c != nil; c = c.sibling {
		if _, isText := c.typ.(textTag); isText {
			s, _ := c.props[textValueKey].(string)
			b.WriteString(s)
			continue
		}
		b.WriteString(ssrJSObjectString)
	}
	return b.String()
}

// ssrJSObjectString is what JavaScript's string coercion produces for a plain
// object, which is what a React element is.
const ssrJSObjectString = "[object Object]"

// ssrValueStrings turns a select's value into the list of option values it
// selects: one string for a scalar, one per element for a list.
//
// React branches on isArray(selectedValue) alone and never consults `multiple`,
// so a list value selects matching options whether or not the select is a
// multi-select -- probed.
func ssrValueStrings(v any) []string {
	if elems, isList := ssrValueList(v); isList {
		out := make([]string, 0, len(elems))
		for _, e := range elems {
			out = append(out, ssrJSStringValue(e))
		}
		return out
	}
	return []string{ssrJSStringValue(v)}
}

// ssrValueList reports whether v is a Go slice or array standing in for a
// JavaScript array, and returns its elements.
//
// A string is not a list even though Go can index it, and neither is a byte
// slice: []byte is how Go carries text, and treating it as a list of numbers
// would turn "ab" into "97,98".
func ssrValueList(v any) ([]any, bool) {
	switch v.(type) {
	case nil, string, []byte:
		return nil, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

// ssrJSStringValue formats a prop value the way JavaScript's `"" + value`
// would, which is what every comparison and every piece of text content in this
// file is defined in terms of.
//
// It is textOf for scalars -- the same function the rest of the renderer uses,
// so 3 and "3" agree -- plus the one case textOf has no reason to know about: a
// list joins its elements with commas, and a nil element contributes nothing.
// String(['a','b']) is "a,b" and String([null]) is "".
func ssrJSStringValue(v any) string {
	if v == nil {
		return ""
	}
	elems, isList := ssrValueList(v)
	if !isList {
		return textOf(v)
	}
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = ssrJSStringValue(e)
	}
	return strings.Join(parts, ",")
}

// ssrTextareaText returns the text content of a <textarea> and reports whether
// there is any.
//
// The effective value is `value` when non-nil and `defaultValue` otherwise, and
// children are used only when neither is set. ok is false only for a textarea
// with no value of any kind, which is the difference between <textarea></textarea>
// and <textarea></textarea> with an empty-string value -- the same bytes, but
// worth keeping distinct so the caller is not guessing.
//
// The returned string is already escaped: React writes
// escapeTextForBrowser("" + value) here, so a value of <b> lands as &lt;b&gt;
// rather than as markup.
func ssrTextareaText(f *fiber) (string, bool, error) {
	var value any
	for _, prop := range ssrControlValueProps {
		if v := f.props[prop]; v != nil {
			value = v
			break
		}
	}

	if f.child != nil {
		if value != nil {
			return "", false, fmt.Errorf("react: <textarea> has both children and a "+
				"%s prop; a textarea's content is set one way or the other, not both",
				ssrTextareaValueProp(f))
		}
		text, err := ssrTextareaChildText(f)
		if err != nil {
			return "", false, err
		}
		value = text
	}

	if value == nil {
		return "", false, nil
	}
	s := ssrJSStringValue(value)
	// React: `"string" === typeof value && "\n" === value[0]` pushes a spare
	// newline. The test is on the value before coercion, so a number never
	// triggers it and neither does a list -- only a string, or the string a
	// textarea's children flattened to.
	if str, isString := value.(string); isString && strings.HasPrefix(str, "\n") {
		s = "\n" + s
	}
	return ssrEscapeText(s), true, nil
}

// ssrTextareaValueProp names the value prop a textarea actually carries, so the
// children error can quote the prop the caller wrote.
func ssrTextareaValueProp(f *fiber) string {
	if f.props["value"] != nil {
		return "value"
	}
	return "defaultValue"
}

// ssrTextareaChildText flattens a <textarea>'s children into its text content.
//
// React allows exactly one child and throws on more, because it coerces the
// child with `"" + children` rather than walking it.
//
// One documented divergence, in the same spirit as the raw-text elements in
// ssr_rawtext.go: React's coercion of a non-primitive child produces the
// useless string "[object Object]", where this port renders the text of the
// subtree instead. Nothing sensible can be built on the upstream answer, and a
// component that returns a textarea body is a shape a Go caller will write.
func ssrTextareaChildText(f *fiber) (string, error) {
	if f.child.sibling != nil {
		return "", fmt.Errorf("react: <textarea> has more than one child; " +
			"a textarea's content must be a single string, or set through the " +
			"value or defaultValue prop")
	}
	var b strings.Builder
	ssrCollectText(f, &b)
	return b.String(), nil
}

// ssrCollectText appends the text of every text fiber below f, in render order,
// ignoring host elements. It is deliberately more forgiving than React, per the
// divergence noted on ssrTextareaChildText.
func ssrCollectText(f *fiber, b *strings.Builder) {
	for c := f.child; c != nil; c = c.sibling {
		switch c.typ.(type) {
		case textTag:
			s, _ := c.props[textValueKey].(string)
			b.WriteString(s)
		default:
			// A host element, a component, a fragment: its own text still
			// counts as text content.
			ssrCollectText(c, b)
		}
	}
}

// ssrControlContent is the emitter's single entry point for a controlled
// element's content. It returns the text to write inside tag and whether tag
// has content of this kind at all, so ssr.go can route <textarea> the way it
// already routes <script> and <style>.
//
// hasRaw says whether the element carries dangerouslySetInnerHTML, which on a
// <textarea> is an error rather than content: there is no markup inside a
// textarea to set.
func ssrControlContent(f *fiber, tag string, hasRaw bool) (string, bool, error) {
	if tag != "textarea" {
		return "", false, nil
	}
	if hasRaw {
		return "", false, fmt.Errorf("react: <textarea> cannot use %s; "+
			"a textarea holds text, not markup, so set its content with the "+
			"value or defaultValue prop", ssrDangerousProp)
	}
	return ssrTextareaText(f)
}

// ssrControlFiber returns the fiber whose props the attribute pass should use.
//
// A controlled element's own props are not what goes in its start tag: a
// <select>'s and a <textarea>'s value props are not attributes at all, and an
// <option>'s `selected` is replaced by the decision the enclosing <select>
// made. Rewriting the props here, rather than in ssrEmitter.attributes, keeps
// every rule about *how* a prop becomes an attribute in one place and every
// rule about controlled values in this file.
//
// f is returned unchanged when tag is not a controlled element or carries none
// of the props in question, so the common case costs one map lookup and no
// allocation. When a copy is needed it is a shallow one: only props differ, and
// the attribute pass reads nothing else.
func ssrControlFiber(f *fiber, tag string) *fiber {
	switch tag {
	case "select", "textarea":
		_, hasValue := f.props["value"]
		_, hasDefault := f.props["defaultValue"]
		if !hasValue && !hasDefault {
			return f
		}
		props := ssrCopyPropsWithout(f.props, ssrControlValueProps[:]...)
		return ssrFiberWithProps(f, props)

	case "option":
		selected := ssrOptionSelected(f)
		if _, has := f.props["selected"]; !has && !selected {
			return f
		}
		props := ssrCopyPropsWithout(f.props, "selected")
		if selected {
			// The prop is kept rather than the attribute written directly, so
			// that ssr_order.go still gets to put selected="" last, where
			// React's option serializer puts it.
			props["selected"] = true
		}
		return ssrFiberWithProps(f, props)
	}
	return f
}

// ssrCopyPropsWithout copies props minus the named keys.
func ssrCopyPropsWithout(props Props, drop ...string) Props {
	out := make(Props, len(props))
	for k, v := range props {
		out[k] = v
	}
	for _, k := range drop {
		delete(out, k)
	}
	return out
}

// ssrFiberWithProps returns a shallow copy of f carrying props. The copy exists
// only to be read by the attribute pass; it is never linked into the tree, so
// the shared child and sibling pointers cannot be reached through it in a way
// that matters.
func ssrFiberWithProps(f *fiber, props Props) *fiber {
	copied := *f
	copied.props = props
	return &copied
}
