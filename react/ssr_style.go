package react

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// This file holds the serializer the HTML emitter uses for the "style" prop.
// It is a port of react-dom's pushStyleAttribute (react-dom 19.2.7,
// cjs/react-dom-server.node.development.js) and is deliberately byte-faithful
// to it, because the parity harness compares the rendered attribute against
// react-dom/server exactly.
//
// [FormatStyle] in ssr_attr.go stays as the exported, hand-friendly helper: it
// accepts a pre-formatted string, writes "name: value" with a space, and errors
// on a bool. Those are ergonomic choices React does not make, so the emitter no
// longer goes through it — see the note in API-DEVIATIONS.md. The two agree on
// the common case (camelCase keys, numbers, dropped nils); they differ only
// where React's exact behaviour is surprising.

// ssrUnitlessNumbers is React's unitlessNumbers set, copied verbatim from
// react-dom 19.2.7. It is keyed on the *camelCase spelling given in the style
// object*, not on the hyphenated CSS name, which is why the vendor variants are
// listed separately and why "WebkitLineClamp" is unitless while
// "-webkit-line-clamp" (written out by hand) is not.
//
// "WebKitBoxFlexGroup" is spelled with a capital K in React's own list while
// every other Webkit entry is not. That is an upstream typo, but it is
// observable — {WebKitBoxFlexGroup: 1} renders "-web-kit-box-flex-group:1"
// while {WebkitBoxFlexGroup: 1} renders "...:1px" — so it is reproduced here
// rather than corrected.
var ssrUnitlessNumbers = map[string]bool{
	"animationIterationCount":       true,
	"aspectRatio":                   true,
	"borderImageOutset":             true,
	"borderImageSlice":              true,
	"borderImageWidth":              true,
	"boxFlex":                       true,
	"boxFlexGroup":                  true,
	"boxOrdinalGroup":               true,
	"columnCount":                   true,
	"columns":                       true,
	"flex":                          true,
	"flexGrow":                      true,
	"flexPositive":                  true,
	"flexShrink":                    true,
	"flexNegative":                  true,
	"flexOrder":                     true,
	"gridArea":                      true,
	"gridRow":                       true,
	"gridRowEnd":                    true,
	"gridRowSpan":                   true,
	"gridRowStart":                  true,
	"gridColumn":                    true,
	"gridColumnEnd":                 true,
	"gridColumnSpan":                true,
	"gridColumnStart":               true,
	"fontWeight":                    true,
	"lineClamp":                     true,
	"lineHeight":                    true,
	"opacity":                       true,
	"order":                         true,
	"orphans":                       true,
	"scale":                         true,
	"tabSize":                       true,
	"widows":                        true,
	"zIndex":                        true,
	"zoom":                          true,
	"fillOpacity":                   true,
	"floodOpacity":                  true,
	"stopOpacity":                   true,
	"strokeDasharray":               true,
	"strokeDashoffset":              true,
	"strokeMiterlimit":              true,
	"strokeOpacity":                 true,
	"strokeWidth":                   true,
	"MozAnimationIterationCount":    true,
	"MozBoxFlex":                    true,
	"MozBoxFlexGroup":               true,
	"MozLineClamp":                  true,
	"msAnimationIterationCount":     true,
	"msFlex":                        true,
	"msZoom":                        true,
	"msFlexGrow":                    true,
	"msFlexNegative":                true,
	"msFlexOrder":                   true,
	"msFlexPositive":                true,
	"msFlexShrink":                  true,
	"msGridColumn":                  true,
	"msGridColumnSpan":              true,
	"msGridRow":                     true,
	"msGridRowSpan":                 true,
	"WebkitAnimationIterationCount": true,
	"WebkitBoxFlex":                 true,
	"WebKitBoxFlexGroup":            true,
	"WebkitBoxOrdinalGroup":         true,
	"WebkitColumnCount":             true,
	"WebkitColumns":                 true,
	"WebkitFlex":                    true,
	"WebkitFlexGrow":                true,
	"WebkitFlexPositive":            true,
	"WebkitFlexShrink":              true,
	"WebkitLineClamp":               true,
}

// ssrStyleCSSName converts a style-object key to the CSS property name React
// writes, which is exactly
//
//	name.replace(/([A-Z])/g, '-$1').toLowerCase().replace(/^ms-/, '-ms-')
//
// so "backgroundColor" becomes "background-color", "WebkitTransform" becomes
// "-webkit-transform", and both "msTransform" and "MsTransform" become
// "-ms-transform" (the second because the leading capital already produced a
// hyphen, the first because of the ^ms- rule).
//
// A custom property is returned untouched, including its case: React tests
// indexOf("--") === 0 before it ever reaches the name rewrite, so
// {"--Brand": …} really does render "--Brand".
//
// A name that already contains a hyphen is *not* special-cased. React only
// logs a development warning for it and still runs the rewrite, so "foo-Bar"
// becomes "foo--bar". Passing hyphenated names is a mistake in React and it is
// a mistake here; the port reproduces the result instead of quietly fixing it.
func ssrStyleCSSName(name string) string {
	if strings.HasPrefix(name, "--") {
		return name
	}
	var b strings.Builder
	b.Grow(len(name) + 4)
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	out := strings.ToLower(b.String())
	if strings.HasPrefix(out, "ms-") {
		out = "-" + out
	}
	return out
}

// ssrStyleAttrValue serializes a "style" prop into the contents of the style
// attribute, following react-dom's pushStyleAttribute declaration by
// declaration:
//
//   - a value that is nil, a bool, or the empty string drops the declaration.
//     A blank-but-not-empty string does not: {"width": " "} renders "width:".
//   - a key starting with "--" is a custom property. Its name is emitted as
//     written and a numeric value gets no unit, so {"--count": 3} is
//     "--count:3".
//   - any other key is rewritten by ssrStyleCSSName.
//   - a numeric value is printed bare when it is zero or when its *original*
//     key is in ssrUnitlessNumbers, and gains "px" otherwise.
//   - a string value is trimmed. Names are not.
//   - declarations are joined "name:value" with ";" and no spaces.
//
// The returned string is not escaped; the caller escapes it, exactly as React
// escapes both names and values with escapeTextForBrowser. Escaping the joined
// string is equivalent because ":" and ";" are not escaped.
//
// A style prop that is not a mapping is an error, mirroring the exception
// react-dom throws for `style="color:red"`. The exported [FormatStyle] still
// accepts a string; the emitter does not.
//
// Declarations come out sorted by key, where React keeps the JavaScript
// object's insertion order. Go maps have no order to keep, so sorting is the
// only deterministic choice; it is a recorded deviation and the only one left
// in this file.
func ssrStyleAttrValue(v any) (string, ssrAttrMode, error) {
	switch t := v.(type) {
	case nil:
		return "", ssrAttrOmit, nil
	case map[string]any:
		return ssrStyleDeclarations(t)
	case Props:
		return ssrStyleDeclarations(t)
	case map[string]string:
		wide := make(map[string]any, len(t))
		for k, s := range t {
			wide[k] = s
		}
		return ssrStyleDeclarations(wide)
	}
	if reflect.ValueOf(v).Kind() == reflect.Func {
		// Consistent with every other prop: a func value is dropped rather
		// than stringified into the page.
		return "", ssrAttrOmit, nil
	}
	return "", ssrAttrOmit, fmt.Errorf("react: the style prop expects a mapping from "+
		"style properties to values, not %T; use map[string]any, map[string]string or Props", v)
}

// ssrStyleDeclarations is ssrStyleAttrValue's map case.
func ssrStyleDeclarations(m map[string]any) (string, ssrAttrMode, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	wrote := false
	for _, k := range keys {
		val, ok, err := ssrStyleDeclarationValue(k, m[k])
		if err != nil {
			return "", ssrAttrOmit, err
		}
		if !ok {
			continue
		}
		if wrote {
			b.WriteByte(';')
		}
		b.WriteString(ssrStyleCSSName(k))
		b.WriteByte(':')
		b.WriteString(val)
		wrote = true
	}
	if !wrote {
		// React pushes nothing at all when every declaration is dropped, so
		// there is no style="" in the output.
		return "", ssrAttrOmit, nil
	}
	return b.String(), ssrAttrValued, nil
}

// ssrStyleDeclarationValue renders one declaration's value, reporting ok=false
// for the values that drop the whole declaration. key is the original
// style-object key, because both the "--" test and the unitless lookup are
// keyed on the spelling the caller used.
func ssrStyleDeclarationValue(key string, v any) (value string, ok bool, err error) {
	switch t := v.(type) {
	case nil:
		return "", false, nil
	case bool:
		// React skips a boolean value silently; it is what `style={{width:
		// flag && 10}}` produces when flag is false.
		return "", false, nil
	case string:
		// The emptiness test happens before the trim, so " " survives it and
		// renders as an empty value.
		if t == "" {
			return "", false, nil
		}
		return strings.TrimSpace(t), true, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return ssrStyleNumber(key, textOf(v), rv.Int() == 0), true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return ssrStyleNumber(key, textOf(v), rv.Uint() == 0), true, nil
	case reflect.Float32, reflect.Float64:
		return ssrStyleNumber(key, textOf(v), rv.Float() == 0), true, nil
	case reflect.Func:
		return "", false, nil
	}

	return "", false, fmt.Errorf("react: style value for %q has unsupported type %T; "+
		"style values must be strings, numbers or bools", key, v)
}

// ssrStyleNumber applies React's px rule: no unit for zero, no unit for a
// custom property, no unit for a property in the unitless set, "px" otherwise.
func ssrStyleNumber(key, digits string, isZero bool) string {
	if isZero || strings.HasPrefix(key, "--") || ssrUnitlessNumbers[key] {
		return digits
	}
	return digits + "px"
}
