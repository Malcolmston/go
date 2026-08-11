package react

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// DangerousHTML is the value type of the "dangerouslySetInnerHTML" prop. In
// React the prop takes an object literal `{__html: "…"}`; the wrapper object
// exists purely so that setting raw HTML is impossible to do by accident, and
// that reasoning survives the port intact. Go has no object literals, so the
// wrapper becomes a named struct: passing a bare string is a compile-time
// mismatch here rather than a runtime warning.
//
//	&Element{Type: "div", Props: Props{
//		"dangerouslySetInnerHTML": DangerousHTML{HTML: "<b>raw</b>"},
//	}}
//
// The content is written to the output byte for byte with no escaping
// whatsoever, so it must never hold data that came from a user. An element
// carrying a DangerousHTML may not also have children — see [RenderToWriter],
// which reports that combination as an error instead of silently picking one.
type DangerousHTML struct {
	// HTML is the raw markup to use as the element's inner content. The field
	// is named HTML rather than React's __html because Go has no convention of
	// leading underscores for "do not touch", and the type name already carries
	// the warning.
	HTML string
}

// ssrDangerousProp is the props key that supplies raw inner content. It is
// never emitted as an attribute.
const ssrDangerousProp = "dangerouslySetInnerHTML"

// ssrStyleProp is the props key whose value is run through [FormatStyle]
// instead of being formatted like an ordinary attribute value.
const ssrStyleProp = "style"

// ssrVoidElements is the set of HTML elements that have no closing tag and no
// content model. The list is fixed by the HTML spec rather than by React, so it
// is exhaustive and will not need to grow: any element outside it gets an
// open/close pair even when it happens to be empty.
var ssrVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"source": true, "track": true, "wbr": true,
}

// ssrIsVoidElement reports whether tag is a void element, matched
// case-insensitively because HTML tag names are case-insensitive while Go map
// lookups are not.
func ssrIsVoidElement(tag string) bool {
	return ssrVoidElements[strings.ToLower(tag)]
}

// ssrAttributeNames maps a React prop name to the HTML/SVG attribute name it is
// written as. Only the props whose attribute name is *not* the prop name appear
// here; everything else falls through to a verbatim pass-through in
// [AttributeName].
//
// The table is not a judgement call: it is React 19's own rename set, taken from
// react-dom's server build. React renames a prop in exactly two places — the
// `aliases` Map that the default branch of `pushAttribute` consults (78 entries,
// almost all of them hyphenated SVG presentation attributes), and a handful of
// props with a dedicated `case` that hard-codes a different output name
// (className, tabIndex, the six xlink:*/three xml:* namespaced names, and the
// three booleans React lowercases by hand). Both sets are reproduced verbatim
// below, grouped by which mechanism they come from.
//
// The surprise, and the reason this table used to be much longer: React does
// *not* lowercase camelCase HTML attributes. `maxLength`, `colSpan`, `srcSet`,
// `readOnly`, `referrerPolicy`, `dateTime`, `inputMode`, `itemProp`,
// `allowFullScreen`, `autoPlay`, `encType`, `formMethod`, `hrefLang`,
// `noValidate`, `spellCheck`, `useMap` and the rest of that family are emitted
// with their JSX spelling intact, because HTML attribute names are
// ASCII-case-insensitive and React never bothers. Only `tabIndex` → `tabindex`,
// `crossOrigin` → `crossorigin`, `autoFocus` → `autofocus`, `multiple` and
// `muted` get lowercased, and only because those five have a `case` of their own
// that happens to call `.toLowerCase()`. Entries for the rest were removed from
// this table: they produced valid, arguably nicer HTML, but not the bytes React
// produces, which is what a port is measured against.
//
// Because the fallback preserves the prop name exactly, the SVG attributes that
// are genuinely camelCase in the SVG spec (viewBox, preserveAspectRatio,
// gradientUnits, patternTransform, refX, baseFrequency, attributeName, …) come
// out correct with no entry at all — they are absent on purpose, not by
// omission.
var ssrAttributeNames = map[string]string{
	// Props with a dedicated case in React's pushAttribute that hard-codes a
	// different output name.
	"className": "class",
	"tabIndex":  "tabindex",

	// The three booleans React writes through pushBooleanAttribute with an
	// explicit name.toLowerCase(). Every *other* boolean prop keeps its
	// camelCase spelling; these three do not, and that asymmetry is upstream's.
	"autoFocus": "autofocus",
	"multiple":  "multiple",
	"muted":     "muted",

	// Namespaced attributes. The colon cannot appear in a JSX prop name, so
	// React spells them camelCase and restores the namespace on output. Each has
	// its own case in pushAttribute rather than living in the aliases map.
	"xlinkActuate": "xlink:actuate",
	"xlinkArcrole": "xlink:arcrole",
	"xlinkHref":    "xlink:href",
	"xlinkRole":    "xlink:role",
	"xlinkShow":    "xlink:show",
	"xlinkTitle":   "xlink:title",
	"xlinkType":    "xlink:type",
	"xmlBase":      "xml:base",
	"xmlLang":      "xml:lang",
	"xmlSpace":     "xml:space",

	// React's controlled-input aliases. React scopes these to <input>, whose
	// own serializer folds them into value/checked before the generic path runs
	// — on any other element React drops them. The port applies them
	// everywhere, which is the one intentional divergence in this table; see
	// [AttributeName].
	"defaultValue":   "value",
	"defaultChecked": "checked",

	// Everything below is React's `aliases` Map, verbatim and in its order.
	"acceptCharset":              "accept-charset",
	"htmlFor":                    "for",
	"httpEquiv":                  "http-equiv",
	"crossOrigin":                "crossorigin",
	"accentHeight":               "accent-height",
	"alignmentBaseline":          "alignment-baseline",
	"arabicForm":                 "arabic-form",
	"baselineShift":              "baseline-shift",
	"capHeight":                  "cap-height",
	"clipPath":                   "clip-path",
	"clipRule":                   "clip-rule",
	"colorInterpolation":         "color-interpolation",
	"colorInterpolationFilters":  "color-interpolation-filters",
	"colorProfile":               "color-profile",
	"colorRendering":             "color-rendering",
	"dominantBaseline":           "dominant-baseline",
	"enableBackground":           "enable-background",
	"fillOpacity":                "fill-opacity",
	"fillRule":                   "fill-rule",
	"floodColor":                 "flood-color",
	"floodOpacity":               "flood-opacity",
	"fontFamily":                 "font-family",
	"fontSize":                   "font-size",
	"fontSizeAdjust":             "font-size-adjust",
	"fontStretch":                "font-stretch",
	"fontStyle":                  "font-style",
	"fontVariant":                "font-variant",
	"fontWeight":                 "font-weight",
	"glyphName":                  "glyph-name",
	"glyphOrientationHorizontal": "glyph-orientation-horizontal",
	"glyphOrientationVertical":   "glyph-orientation-vertical",
	"horizAdvX":                  "horiz-adv-x",
	"horizOriginX":               "horiz-origin-x",
	"imageRendering":             "image-rendering",
	"letterSpacing":              "letter-spacing",
	"lightingColor":              "lighting-color",
	"markerEnd":                  "marker-end",
	"markerMid":                  "marker-mid",
	"markerStart":                "marker-start",
	"overlinePosition":           "overline-position",
	"overlineThickness":          "overline-thickness",
	"paintOrder":                 "paint-order",
	"panose-1":                   "panose-1",
	"pointerEvents":              "pointer-events",
	"renderingIntent":            "rendering-intent",
	"shapeRendering":             "shape-rendering",
	"stopColor":                  "stop-color",
	"stopOpacity":                "stop-opacity",
	"strikethroughPosition":      "strikethrough-position",
	"strikethroughThickness":     "strikethrough-thickness",
	"strokeDasharray":            "stroke-dasharray",
	"strokeDashoffset":           "stroke-dashoffset",
	"strokeLinecap":              "stroke-linecap",
	"strokeLinejoin":             "stroke-linejoin",
	"strokeMiterlimit":           "stroke-miterlimit",
	"strokeOpacity":              "stroke-opacity",
	"strokeWidth":                "stroke-width",
	"textAnchor":                 "text-anchor",
	"textDecoration":             "text-decoration",
	"textRendering":              "text-rendering",
	"transformOrigin":            "transform-origin",
	"underlinePosition":          "underline-position",
	"underlineThickness":         "underline-thickness",
	"unicodeBidi":                "unicode-bidi",
	"unicodeRange":               "unicode-range",
	"unitsPerEm":                 "units-per-em",
	"vAlphabetic":                "v-alphabetic",
	"vHanging":                   "v-hanging",
	"vIdeographic":               "v-ideographic",
	"vMathematical":              "v-mathematical",
	"vectorEffect":               "vector-effect",
	"vertAdvY":                   "vert-adv-y",
	"vertOriginX":                "vert-origin-x",
	"vertOriginY":                "vert-origin-y",
	"wordSpacing":                "word-spacing",
	"writingMode":                "writing-mode",
	"xmlnsXlink":                 "xmlns:xlink",
	"xHeight":                    "x-height",
}

// ssrAttrClass says how a prop's *value* becomes an attribute value. React's
// pushAttribute is one large switch whose cases fall into these seven shapes;
// naming them turns a 200-line switch into a lookup table plus seven short
// rules, and makes "which class is `download` in?" answerable by reading one
// map entry.
type ssrAttrClass int

const (
	// ssrAttrClassString is React's pushStringAttribute and the common case of
	// its default branch: the value is written as-is, except that a bool drops
	// the attribute entirely. This is the class of every prop with no entry in
	// ssrAttrClasses.
	ssrAttrClassString ssrAttrClass = iota

	// ssrAttrClassEnumerated always writes the value, bools included, so
	// contentEditable={false} becomes contentEditable="false". React calls
	// these "overloaded string" attributes: HTML gives "true" and "false"
	// distinct meanings, so omitting the attribute is not the same as
	// requesting false.
	ssrAttrClassEnumerated

	// ssrAttrClassBoolean writes a valueless attribute for any truthy value and
	// nothing at all otherwise. disabled={false} must produce no attribute,
	// because disabled="" in HTML means disabled.
	ssrAttrClassBoolean

	// ssrAttrClassOverloadedBoolean is `capture` and `download`, which are
	// either a flag or a value: true is the bare flag, false omits, and
	// anything else — including the empty string — is written as a value.
	ssrAttrClassOverloadedBoolean

	// ssrAttrClassPositiveNumeric is cols/rows/size/span, where HTML requires a
	// positive integer and React drops anything that is not numeric or is below
	// 1 rather than emitting an invalid attribute.
	ssrAttrClassPositiveNumeric

	// ssrAttrClassNumeric is rowSpan/start, where React drops only values that
	// are not numeric at all; zero and negatives are allowed through.
	ssrAttrClassNumeric

	// ssrAttrClassURL is `src` and `href`, which behave like
	// ssrAttrClassString except that the empty string omits the attribute.
	// src="" makes a browser re-request the current document, and href="" makes
	// a link that reloads the page; React drops both with a warning, and the
	// port drops them silently because it has nowhere to warn to.
	ssrAttrClassURL

	// ssrAttrClassStyle routes the value through [FormatStyle].
	ssrAttrClassStyle

	// ssrAttrClassDrop is never emitted. React's noop cases plus the port's own
	// bookkeeping props.
	ssrAttrClassDrop
)

// ssrAttrClasses assigns the props React special-cases to their value class.
// Anything absent is ssrAttrClassString, which is both React's default branch
// and the right answer for every plain attribute (id, href, alt, …).
//
// Each group below corresponds to one arm of React's pushAttribute switch, so a
// future react-dom upgrade can be diffed against it directly.
var ssrAttrClasses = map[string]ssrAttrClass{
	ssrStyleProp: ssrAttrClassStyle,

	// React's noop arm, plus the port's props that describe the tree rather
	// than the markup. defaultValue/defaultChecked are in React's noop arm
	// because <input> consumes them before the generic path ever sees them;
	// the port maps them instead (see ssrAttributeNames) and so must *not*
	// list them here.
	"innerHTML":                      ssrAttrClassDrop,
	"suppressContentEditableWarning": ssrAttrClassDrop,
	"suppressHydrationWarning":       ssrAttrClassDrop,
	ChildrenKey:                      ssrAttrClassDrop,
	KeyProp:                          ssrAttrClassDrop,
	RefProp:                          ssrAttrClassDrop,
	ssrDangerousProp:                 ssrAttrClassDrop,

	// "Overloaded string" attributes: bools render as the words true/false.
	"contentEditable":           ssrAttrClassEnumerated,
	"spellCheck":                ssrAttrClassEnumerated,
	"draggable":                 ssrAttrClassEnumerated,
	"value":                     ssrAttrClassEnumerated,
	"autoReverse":               ssrAttrClassEnumerated,
	"externalResourcesRequired": ssrAttrClassEnumerated,
	"focusable":                 ssrAttrClassEnumerated,
	"preserveAlpha":             ssrAttrClassEnumerated,

	// Real HTML boolean attributes.
	"allowFullScreen":         ssrAttrClassBoolean,
	"async":                   ssrAttrClassBoolean,
	"autoFocus":               ssrAttrClassBoolean,
	"autoPlay":                ssrAttrClassBoolean,
	"controls":                ssrAttrClassBoolean,
	"default":                 ssrAttrClassBoolean,
	"defer":                   ssrAttrClassBoolean,
	"disabled":                ssrAttrClassBoolean,
	"disablePictureInPicture": ssrAttrClassBoolean,
	"disableRemotePlayback":   ssrAttrClassBoolean,
	"formNoValidate":          ssrAttrClassBoolean,
	"hidden":                  ssrAttrClassBoolean,
	"inert":                   ssrAttrClassBoolean,
	"itemScope":               ssrAttrClassBoolean,
	"loop":                    ssrAttrClassBoolean,
	"multiple":                ssrAttrClassBoolean,
	"muted":                   ssrAttrClassBoolean,
	"noModule":                ssrAttrClassBoolean,
	"noValidate":              ssrAttrClassBoolean,
	"open":                    ssrAttrClassBoolean,
	"playsInline":             ssrAttrClassBoolean,
	"readOnly":                ssrAttrClassBoolean,
	"required":                ssrAttrClassBoolean,
	"reversed":                ssrAttrClassBoolean,
	"scoped":                  ssrAttrClassBoolean,
	"seamless":                ssrAttrClassBoolean,

	// Boolean in the port, string in React's generic path. React reaches these
	// three only through <input>/<option>, whose serializers treat them as
	// booleans; on a <div> React's default branch drops a bool outright. The
	// port has no per-element serializer, so it classes them the way the only
	// elements that accept them do — `defaultChecked` on an <input> has to
	// produce `checked`, not nothing.
	"checked":        ssrAttrClassBoolean,
	"defaultChecked": ssrAttrClassBoolean,
	"selected":       ssrAttrClassBoolean,

	// Flag-or-value.
	"capture":  ssrAttrClassOverloadedBoolean,
	"download": ssrAttrClassOverloadedBoolean,

	// HTML requires a positive integer here.
	"cols": ssrAttrClassPositiveNumeric,
	"rows": ssrAttrClassPositiveNumeric,
	"size": ssrAttrClassPositiveNumeric,
	"span": ssrAttrClassPositiveNumeric,

	// Any number, including zero and negatives.
	"rowSpan": ssrAttrClassNumeric,
	"start":   ssrAttrClassNumeric,

	// An empty URL is worse than no URL.
	"src":  ssrAttrClassURL,
	"href": ssrAttrClassURL,
}

// AttributeName maps a React prop name to the HTML attribute name it should be
// serialized as, reporting ok=false when the prop must not appear in the output
// at all. It is exported because the mapping is useful on its own — a test, a
// custom renderer or a linter can ask "what does this prop become?" without
// rendering a tree — and because it is the honest place to document where the
// port's knowledge of HTML stops.
//
// Props that are never emitted, and for which ok is false:
//
//   - [ChildrenKey], [KeyProp] and [RefProp] — runtime bookkeeping, not markup.
//     React strips key and ref from props before rendering; the port keeps them
//     in the map and drops them here, which is the same observable result.
//   - "dangerouslySetInnerHTML" — it supplies the element's inner content
//     instead ([DangerousHTML]), so emitting it as an attribute would leak the
//     raw markup into a quoted string.
//   - "innerHTML", "suppressContentEditableWarning" and
//     "suppressHydrationWarning" — React's own noop cases. The first is the
//     DOM property React refuses to let you set this way; the other two are
//     instructions to React's warning machinery and have no HTML meaning.
//   - anything React treats as an event handler: longer than two characters and
//     starting with "on" in any case. Handlers are Go funcs and mean nothing to
//     a server-rendered document. See ssrIsEventHandlerProp for why the rule is
//     spelled that precisely.
//   - any name that is not a legal XML name, per ssrIsAttributeNameSafe.
//
// Props that pass through verbatim: everything else. React 19 does not maintain
// an allowlist of known attributes — since React 16 the default branch of
// pushAttribute writes any prop whose name is a legal XML name, so "aria-*",
// "data-*", custom hyphenated attributes, vendor extensions, pre-namespaced
// names such as "xlink:href", and even a plain unknown "myAttr" all reach the
// output. The frequently repeated rule that React "passes data-* and aria-*
// through and drops the rest" describes React 15; in React 19 the data-/aria-
// test survives only as the exception that lets a *boolean* value through, and
// that lives in ssrAttrValue rather than here.
//
// A name that is not a legal XML name — one containing a space, a quote, an
// equals sign, or starting with a digit — is dropped, matching React's
// isAttributeNameSafe. Without that check a prop name could close the attribute
// it is being written into, so this is a correctness rule and not a nicety.
func AttributeName(propName string) (string, bool) {
	if propName == "" {
		return "", false
	}
	if ssrAttrClasses[propName] == ssrAttrClassDrop {
		return "", false
	}
	if ssrIsEventHandlerProp(propName) {
		return "", false
	}
	if mapped, ok := ssrAttributeNames[propName]; ok {
		// A rename always produces a name React itself emits, so re-validating
		// it would only cost time.
		return mapped, true
	}
	if !ssrIsAttributeNameSafe(propName) {
		return "", false
	}
	return propName, true
}

// ssrIsEventHandlerProp reports whether a prop name is one React treats as an
// event handler and never serializes. The test is React's exactly: longer than
// two characters and starting with "on" in any mix of case. It is worth
// mirroring the oddities rather than tidying them, because both are observable:
// a prop named exactly "on" *is* emitted (it is a real attribute on some legacy
// elements), and a prop named "only" is *not*, since React never looks past the
// first two letters.
func ssrIsEventHandlerProp(propName string) bool {
	if len(propName) <= 2 {
		return false
	}
	return (propName[0] == 'o' || propName[0] == 'O') &&
		(propName[1] == 'n' || propName[1] == 'N')
}

// ssrIsAttributeNameSafe reports whether name may be written into a start tag,
// using the character ranges of React's VALID_ATTRIBUTE_NAME_REGEX — which is
// the XML 1.0 Name production, minus the astral planes that its UTF-16 spelling
// cannot reach. Go strings are UTF-8, so a rune above U+FFFD simply matches no
// range and is rejected, which is the same answer React gives.
func ssrIsAttributeNameSafe(name string) bool {
	for i, r := range name {
		if ssrIsNameStartRune(r) {
			continue
		}
		if i > 0 && ssrIsNameContinueRune(r) {
			continue
		}
		return false
	}
	return name != ""
}

// ssrIsNameStartRune reports whether r may begin an XML name.
func ssrIsNameStartRune(r rune) bool {
	switch {
	case r == ':' || r == '_':
		return true
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		return true
	case r >= 0x00C0 && r <= 0x00D6, r >= 0x00D8 && r <= 0x00F6:
		return true
	case r >= 0x00F8 && r <= 0x02FF, r >= 0x0370 && r <= 0x037D:
		return true
	case r >= 0x037F && r <= 0x1FFF, r >= 0x200C && r <= 0x200D:
		return true
	case r >= 0x2070 && r <= 0x218F, r >= 0x2C00 && r <= 0x2FEF:
		return true
	case r >= 0x3001 && r <= 0xD7FF, r >= 0xF900 && r <= 0xFDCF:
		return true
	case r >= 0xFDF0 && r <= 0xFFFD:
		return true
	}
	return false
}

// ssrIsNameContinueRune reports whether r may appear in an XML name after the
// first rune. It covers only the extra characters; a name-start rune is also a
// valid continuation.
func ssrIsNameContinueRune(r rune) bool {
	switch {
	case r == '-' || r == '.' || r == 0x00B7:
		return true
	case r >= '0' && r <= '9':
		return true
	case r >= 0x0300 && r <= 0x036F, r >= 0x203F && r <= 0x2040:
		return true
	}
	return false
}

// ssrUnitlessStyleProps is the set of CSS properties whose numeric values are
// plain numbers rather than lengths, keyed by the hyphenated name so that both
// spellings of a prop ("zIndex" and "z-index") hit the same entry. A number
// given for any property *not* in this set gets "px" appended, which is React's
// rule and the reason `style={{width: 10}}` produces `width: 10px`.
//
// The list is React's isUnitlessNumber table: the flex and grid shorthands, the
// counting and ratio properties, the opacity family, font-weight, line-height,
// order, orphans/widows, tab-size, z-index, zoom, and the SVG stroke/fill
// numerics. It stops where React's does — a property CSS treats as unitless but
// React never listed (a very new one, or a custom property) will wrongly get
// "px"; pass such a value as a string to opt out.
var ssrUnitlessStyleProps = map[string]bool{
	"animation-iteration-count": true,
	"aspect-ratio":              true,
	"border-image-outset":       true,
	"border-image-slice":        true,
	"border-image-width":        true,
	"box-flex":                  true,
	"box-flex-group":            true,
	"box-ordinal-group":         true,
	"column-count":              true,
	"columns":                   true,
	"fill-opacity":              true,
	"flex":                      true,
	"flex-grow":                 true,
	"flex-negative":             true,
	"flex-order":                true,
	"flex-positive":             true,
	"flex-shrink":               true,
	"flood-opacity":             true,
	"font-weight":               true,
	"grid-area":                 true,
	"grid-column":               true,
	"grid-column-end":           true,
	"grid-column-span":          true,
	"grid-column-start":         true,
	"grid-row":                  true,
	"grid-row-end":              true,
	"grid-row-span":             true,
	"grid-row-start":            true,
	"line-clamp":                true,
	"line-height":               true,
	"opacity":                   true,
	"order":                     true,
	"orphans":                   true,
	"scale":                     true,
	"stop-opacity":              true,
	"stroke-dasharray":          true,
	"stroke-dashoffset":         true,
	"stroke-miterlimit":         true,
	"stroke-opacity":            true,
	"stroke-width":              true,
	"tab-size":                  true,
	"widows":                    true,
	"z-index":                   true,
	"zoom":                      true,
}

// ssrStyleName converts a style-map key to its CSS spelling: camelCase becomes
// kebab-case ("zIndex" → "z-index"), a leading capital becomes a vendor prefix
// ("WebkitTransform" → "-webkit-transform"), and the Microsoft special case
// React inherits from the DOM ("msTransform", lowercase ms) becomes
// "-ms-transform". A name that already contains a hyphen is returned unchanged,
// which lets a caller write plain CSS names and custom properties ("--brand")
// directly.
func ssrStyleName(name string) string {
	if strings.ContainsRune(name, '-') {
		return name
	}
	var b strings.Builder
	b.Grow(len(name) + 4)
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('-')
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	// "ms" is the one vendor prefix spelled lowercase in DOM style objects, so
	// the loop above cannot see it as a prefix boundary.
	if len(name) > 2 && strings.HasPrefix(name, "ms") && name[2] >= 'A' && name[2] <= 'Z' {
		out = "-" + out
	}
	return out
}

// FormatStyle turns a "style" prop into the string that goes inside the style
// attribute. It is exported so that the port's one piece of CSS knowledge — the
// camelCase/kebab-case translation and the automatic "px" — is testable and
// reusable rather than buried in the serializer.
//
// Accepted values:
//
//   - string — returned unchanged. This is the escape hatch for anything the
//     map form cannot express (`!important`, multi-value shorthands with
//     commas, calc()).
//   - map[string]string and map[string]any — each entry becomes one
//     declaration. Keys are converted by ssrStyleName. A nil or empty-string
//     value drops the declaration entirely, so a conditional style is written
//     as `map[string]any{"color": maybeColor}` with no branch.
//   - nil — the empty string, so an absent style is not an error.
//
// Numeric values format the way a text child does (see textOf, so 1.5 is "1.5"
// and not "1.500000") and gain a "px" suffix unless the value is zero or the
// property is in ssrUnitlessStyleProps.
//
// Any other value type — a bool, a struct, a slice — is an error rather than a
// silent fmt.Sprint, because a wrong style silently baked into a page is much
// harder to notice than a failed render.
//
// Declarations are emitted sorted by CSS property name and joined exactly as
// React joins them: "a:1;b:2", with no space after the colon or the semicolon.
// The spacing is not cosmetic — the parity harness compares the rendered
// attribute byte for byte against react-dom/server, and "top: 1px; left: 0"
// differs from React's "top:1px;left:0" on every styled element.
//
// React preserves the JavaScript object's insertion order; Go maps have no
// order at all, so sorting is the only way to make the output deterministic,
// and determinism matters more here than matching React's arbitrary order.
func FormatStyle(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case map[string]string:
		// Widen to the general case rather than duplicating the loop.
		wide := make(map[string]any, len(v))
		for k, s := range v {
			wide[k] = s
		}
		return ssrFormatStyleMap(wide)
	case map[string]any:
		return ssrFormatStyleMap(v)
	case Props:
		// Props is a map[string]any underneath; accepting it means a caller can
		// build a style bag with the same helper they build props with.
		return ssrFormatStyleMap(v)
	}
	return "", fmt.Errorf("react: style prop of type %T is not supported; "+
		"use a string, a map[string]string or a map[string]any", value)
}

// ssrFormatStyleMap is FormatStyle's map case, factored out so the three map
// shapes share one implementation.
func ssrFormatStyleMap(m map[string]any) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	decls := make([]string, 0, len(keys))
	for _, k := range keys {
		name := ssrStyleName(k)
		val, ok, err := ssrStyleValue(name, m[k])
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		decls = append(decls, name+":"+val)
	}
	return strings.Join(decls, ";"), nil
}

// ssrStyleValue renders one declaration's value. It reports ok=false for values
// that drop the declaration (nil, empty string) and an error for value types
// CSS cannot express.
func ssrStyleValue(name string, v any) (value string, ok bool, err error) {
	switch t := v.(type) {
	case nil:
		return "", false, nil
	case string:
		if t == "" {
			return "", false, nil
		}
		return t, true, nil
	}

	switch reflect.ValueOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		s := textOf(v)
		// Zero needs no unit, and a unitless property must not get one; every
		// other number is a length, which is what React assumes too.
		if s != "0" && !ssrUnitlessStyleProps[name] {
			s += "px"
		}
		return s, true, nil
	}

	return "", false, fmt.Errorf("react: style value for %q has unsupported type %T; "+
		"style values must be strings or numbers", name, v)
}

// ssrEscapeHTML escapes the five characters React's escapeTextForBrowser
// escapes: & < > " and '. It is used for *both* text content and attribute
// values, exactly as React uses one function for both.
//
// Only & and < are strictly required in text, and only & and " in a
// double-quoted attribute value; escaping the other three as well costs a few
// bytes and removes a whole class of "this string was pasted into the wrong
// context" bugs. The apostrophe becomes the numeric &#39; rather than &apos;
// because &apos; is not defined in HTML 4 and React avoids it for the same
// reason.
func ssrEscapeHTML(s string) string {
	// Fast path: most strings need no escaping, and building a Builder for them
	// would allocate on every text node in the tree.
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/8 + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ssrAttrMode says how one prop turns into markup: dropped entirely, written as
// a flag whose value carries no information, or written as name="value".
//
// Booleans are the reason this is three-valued rather than a (string, bool)
// pair. The distinction is not about the bytes — a flag is emitted as
// `disabled=""`, which is what react-dom writes and what the parity oracle
// compares against — but about whether a value exists to be escaped and
// inspected at all. `disabled` is not `disabled="true"`, and a caller reasoning
// about the output needs those to be different answers.
//
// An earlier version of this port wrote the bare form, `<button disabled>`. It
// is identical to every HTML parser and three bytes shorter, and it was still
// wrong: react-dom writes `disabled=""` for every one of its ~26 boolean
// attributes, so the bare form cost 22 cases in parity/react for an aesthetic
// preference.
type ssrAttrMode int

const (
	ssrAttrOmit ssrAttrMode = iota
	ssrAttrBare
	ssrAttrValued
)

// ssrAttrValue formats a prop's value for output. propName is the *React* name,
// which is what selects the value rule — the rules are keyed on the prop the
// caller wrote, not on the attribute it becomes. attrName is the already-mapped
// HTML name, used only in error messages.
//
// The rules are React 19's, one per [ssrAttrClass], and every one of them was
// read off a probe render rather than recalled:
//
//	Props{"disabled": true}           → disabled
//	Props{"disabled": false}          → (omitted)
//	Props{"disabled": nil}            → (omitted)
//	Props{"contentEditable": false}   → contentEditable="false"
//	Props{"download": false}          → (omitted)
//	Props{"download": ""}             → download=""
//	Props{"download": true}           → download
//	Props{"capture": "user"}          → capture="user"
//	Props{"tabIndex": 0}              → tabindex="0"
//	Props{"cols": 0}                  → (omitted; HTML wants a positive integer)
//	Props{"rowSpan": 0}               → rowSpan="0"
//	Props{"rowSpan": "wide"}          → (omitted; not a number)
//	Props{"aria-hidden": true}        → aria-hidden="true"
//	Props{"myAttr": true}             → (omitted; bool on an unknown attribute)
//
// Three cross-cutting rules come first, before the class is consulted:
//
//   - nil omits the attribute. This is what makes `Props{"disabled": maybe}`
//     work with a nil-able value, and it is where React's `null`/`undefined`
//     check lands too.
//   - a func value omits the attribute, matching React's refusal to serialize
//     `typeof value === 'function'`. A func in props is either a handler the
//     "on" rule missed or a mistake; stringifying a Go closure into a page is
//     never the intent.
//   - a [DangerousHTML] in any prop but "dangerouslySetInnerHTML" is an error,
//     because it can only be a misplaced attempt to set raw markup.
//
// Values are formatted with textOf — the same function that formats a numeric
// text child — so 3 and "3" render identically.
func ssrAttrValue(propName, attrName string, v any) (string, ssrAttrMode, error) {
	if v == nil {
		return "", ssrAttrOmit, nil
	}
	if _, isDanger := v.(DangerousHTML); isDanger {
		return "", ssrAttrOmit, fmt.Errorf("react: prop %q holds a DangerousHTML; "+
			"raw HTML may only be set through the %q prop", attrName, ssrDangerousProp)
	}
	if reflect.ValueOf(v).Kind() == reflect.Func {
		return "", ssrAttrOmit, nil
	}

	switch ssrAttrClasses[propName] {
	case ssrAttrClassDrop:
		// Unreachable through the renderer, since AttributeName already
		// refused; handled anyway so that a direct caller cannot get a value
		// for a prop that has none.
		return "", ssrAttrOmit, nil

	case ssrAttrClassStyle:
		css, err := FormatStyle(v)
		if err != nil {
			return "", ssrAttrOmit, err
		}
		if css == "" {
			// style="" is noise, and React skips an empty style object too.
			return "", ssrAttrOmit, nil
		}
		return css, ssrAttrValued, nil

	case ssrAttrClassEnumerated:
		// The whole point of this class: a bool reaches the output as a word.
		return textOf(v), ssrAttrValued, nil

	case ssrAttrClassURL:
		if s, ok := v.(string); ok && s == "" {
			return "", ssrAttrOmit, nil
		}
		if _, ok := v.(bool); ok {
			return "", ssrAttrOmit, nil
		}
		return textOf(v), ssrAttrValued, nil

	case ssrAttrClassBoolean:
		if ssrTruthy(v) {
			return "", ssrAttrBare, nil
		}
		return "", ssrAttrOmit, nil

	case ssrAttrClassOverloadedBoolean:
		if b, ok := v.(bool); ok {
			if b {
				return "", ssrAttrBare, nil
			}
			return "", ssrAttrOmit, nil
		}
		return textOf(v), ssrAttrValued, nil

	case ssrAttrClassPositiveNumeric:
		if n, ok := ssrNumericValue(v); !ok || n < 1 {
			return "", ssrAttrOmit, nil
		}
		return textOf(v), ssrAttrValued, nil

	case ssrAttrClassNumeric:
		if _, ok := ssrNumericValue(v); !ok {
			return "", ssrAttrOmit, nil
		}
		return textOf(v), ssrAttrValued, nil
	}

	// ssrAttrClassString: React's pushStringAttribute and the default branch.
	// A bool is dropped, with one exception — a data-* or aria-* attribute takes
	// it, because `aria-hidden={true}` has to reach the page as the string
	// "true" for assistive technology to see it, and that single carve-out is
	// the whole of the data-/aria- rule in React 19.
	if _, ok := v.(bool); ok && !ssrTakesBooleanValue(attrName) {
		return "", ssrAttrOmit, nil
	}
	return textOf(v), ssrAttrValued, nil
}

// ssrTakesBooleanValue reports whether an unrecognized attribute may carry a
// bool as its value, which React allows for exactly the data-* and aria-*
// prefixes, compared case-insensitively as React does.
func ssrTakesBooleanValue(attrName string) bool {
	if len(attrName) < 5 {
		return false
	}
	prefix := strings.ToLower(attrName[:5])
	return prefix == "data-" || prefix == "aria-"
}

// ssrTruthy reports whether a value counts as true for a boolean attribute,
// following JavaScript's truthiness because React's `value &&` test does: the
// empty string and a zero number are false, every other string and number is
// true, and a value of any other type is true because it exists. nil is handled
// by the caller.
//
// This is why `disabled="false"` still disables the control — a non-empty
// string is truthy in JavaScript, and copying that is less surprising than
// having the port disagree with React about a footgun React documents.
func ssrTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t != ""
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		return f != 0 && !math.IsNaN(f)
	}
	return true
}

// ssrNumericValue coerces a value the way JavaScript's Number() does for the
// numeric attribute classes, reporting ok=false where Number() would give NaN.
// The coercions that look strange are all deliberate: the empty string and a
// whitespace-only string are 0, and a bool is 0 or 1, because `isNaN("")` and
// `isNaN(false)` are both false in JavaScript and React's numeric attributes
// consequently let those values through. Only the *test* uses this number; the
// attribute is written from the original value, so `rowSpan={false}` emits
// rowSpan="false" exactly as React does.
func ssrNumericValue(v any) (float64, bool) {
	switch t := v.(type) {
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, true
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) {
			return 0, false
		}
		return n, true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		n := rv.Float()
		return n, !math.IsNaN(n)
	}
	return 0, false
}

// ssrDangerousContent extracts the raw inner HTML from a props value and
// reports whether the prop was present at all. Both DangerousHTML and
// *DangerousHTML are accepted, since a caller storing one in a Props map may
// naturally reach for either.
func ssrDangerousContent(props Props) (html string, present bool, err error) {
	if !props.Has(ssrDangerousProp) {
		return "", false, nil
	}
	switch d := props[ssrDangerousProp].(type) {
	case nil:
		return "", false, nil
	case DangerousHTML:
		return d.HTML, true, nil
	case *DangerousHTML:
		if d == nil {
			return "", false, nil
		}
		return d.HTML, true, nil
	default:
		return "", false, fmt.Errorf("react: %q must hold a react.DangerousHTML, got %T",
			ssrDangerousProp, props[ssrDangerousProp])
	}
}
