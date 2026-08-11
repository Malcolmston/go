// Package color is a Go port of d3-color — parsing, representing and
// converting between the color spaces that data visualization actually needs.
// It is the computational half of the problem: there is no DOM here and nothing
// is rendered, so a color is just a value you can read channels off of, convert,
// and format back to a CSS string for whatever does the drawing.
//
//	c := color.MustParse("steelblue")     // any CSS color string
//	fmt.Println(c.RGBA())                 // {70 130 180 1}
//	fmt.Println(color.ToHSL(c).String())  // hsl(207.272..., 44%, 49%)
//	fmt.Println(color.ToLab(c).L)         // perceptual lightness
//
// Why more than one color space? Because sRGB is a terrible space to compute
// in. Averaging two sRGB colors, or walking a hue ramp in sRGB, produces muddy
// midpoints and bands of uneven apparent brightness, because the sRGB channels
// are not perceptually uniform — a step of 10 in the green channel is a much
// bigger visual change than a step of 10 in blue. [Lab] and [HCL] are designed
// so that equal numeric distances look like equal perceptual distances, which
// is exactly what you want when interpolating a color scale or generating a
// categorical palette. [HSL] is not perceptual at all, but it is the space most
// people reason about ("same color, lighter"), and CSS speaks it natively.
// [Cubehelix] trades perceptual accuracy for a guarantee sequential ramps care
// about: monotonically increasing luminance along the ramp, so the scale still
// reads correctly in grayscale.
//
// The types are plain structs with exported float64 fields, deliberately. d3
// lets you reach in and adjust a channel — nudge the lightness, rotate the hue
// — and so does this package. Channels are stored unclamped and unrounded:
// [RGB] channels are 0–255 but nothing stops you holding 300 or -12 in one, and
// [HSL.H] is in degrees but need not lie in [0, 360). That is not sloppiness,
// it is what makes interpolation and extrapolation work. Clamping happens at
// exactly one place — [RGB.String], and [RGB.Hex] — where a value has to become
// a legal CSS token. See [RGB] for the full rationale.
//
// Every color type satisfies [Color]: it can convert itself to sRGB and format
// itself as CSS. Conversions between spaces go through sRGB when there is no
// direct path, which means a round trip is lossy in the last few decimal places
// (and genuinely lossy when a Lab or HCL value falls outside the sRGB gamut —
// see [Lab] for how that is handled). [ToRGB], [ToHSL], [ToLab], [ToHCL] and
// [ToCubehelix] are the conversion entry points; each is a no-op fast path when
// the value is already in the target space, so converting is cheap to do
// defensively.
//
// [Parse] accepts the CSS color syntaxes that appear in real stylesheets and
// data files: the 148 named colors plus "transparent", #rgb, #rgba, #rrggbb,
// #rrggbbaa, and the functional rgb()/rgba()/hsl()/hsla() forms with either
// comma or whitespace separators, integer or percentage channels. It does not
// implement the newer CSS Color 4 functions (lab(), lch(), oklab(), color());
// those are listed in the package's known gaps below.
//
// Known gaps relative to d3-color: the CSS Color 4 functional notations are not
// parsed, and there is no equivalent of d3's color.copy() — Go structs are
// values, so assignment already copies.
package color

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Color is the behavior every color space in this package shares: it can
// project itself into sRGB and it can render itself as a CSS string.
//
// The interface is deliberately narrow. Anything richer — reading channels,
// adjusting lightness, rotating hue — is done on the concrete types, because
// those operations only mean something in a particular space. Code that merely
// needs "a color" (an interpolator, a scale's output, a renderer) takes a
// Color; code that wants to do color math converts to the space it wants first
// with [ToRGB], [ToLab] and friends.
//
// RGBA is named for the four channels it carries, not for a conversion to
// image/color.RGBA. This package intentionally does not implement the standard
// library's color.Color interface: that one uses alpha-premultiplied 16-bit
// channels, which would force premature clamping and quantization on values
// this package needs to keep in floating point and out of gamut. Use
// [RGB.NRGBA] at the boundary if you need to hand a color to image/draw.
type Color interface {
	// RGBA converts this color to sRGB. Channels may fall outside 0–255 if
	// the source color is outside the sRGB gamut; see [RGB].
	RGBA() RGB
	// String formats this color as a CSS color string.
	String() string
}

// RGB is a color in the sRGB space: red, green and blue channels nominally in
// 0–255 and an opacity (alpha) nominally in 0–1.
//
// "Nominally" is the important word. The fields are not clamped and not
// rounded. An RGB may legitimately hold R = 271.4 or Opacity = -0.2, and this
// package will happily do arithmetic on it. Three reasons:
//
// First, interpolation. d3 interpolators do not clamp t, so asking for t = 1.5
// between two colors is a defined operation that extrapolates past the
// endpoint. If RGB clamped on construction, the extrapolated value would be
// silently destroyed before it could be used as the input to a further
// computation.
//
// Second, the wider gamuts. [Lab] and [HCL] describe colors that sRGB cannot
// reproduce — a vivid Lab cyan converts to a negative red channel. Clamping at
// conversion time would make Lab→RGB→Lab round trips lose information for every
// out-of-gamut color instead of only for the ones actually displayed.
//
// Third, precision. Rounding to integers on every intermediate step compounds
// error across a chain of conversions.
//
// Clamping and rounding therefore happen at the boundary, when a value has to
// become a legal CSS token: [RGB.String], [RGB.Hex] and [RGB.NRGBA] all clamp
// to 0–255 (opacity to 0–1) and round. [RGB.Displayable] reports whether that
// clamp would actually change anything, which is the check to run before
// trusting a converted Lab or HCL color.
type RGB struct {
	R, G, B float64 // channels, nominally 0–255
	Opacity float64 // alpha, nominally 0–1
}

// NewRGB returns an opaque RGB with the given channels.
//
// It exists because the zero RGB is transparent black, not opaque black: a bare
// color.RGB{R: 255} has Opacity 0 and renders as fully transparent, which is
// almost never what the caller meant. Use this constructor unless you are
// deliberately setting an alpha.
func NewRGB(r, g, b float64) RGB { return RGB{R: r, G: g, B: b, Opacity: 1} }

// NewRGBA returns an RGB with the given channels and opacity.
func NewRGBA(r, g, b, opacity float64) RGB {
	return RGB{R: r, G: g, B: b, Opacity: opacity}
}

// RGBA returns c unchanged, satisfying [Color].
func (c RGB) RGBA() RGB { return c }

// String formats the color as CSS: "rgb(r, g, b)" when fully opaque, otherwise
// "rgba(r, g, b, a)".
//
// Channels are clamped to 0–255 and rounded to integers, and opacity is clamped
// to 0–1, because that is what CSS accepts — see [RGB] for why the clamp lives
// here and not in the struct. Opacity is formatted with the shortest
// representation that round-trips, so 0.5 prints as "0.5" rather than
// "0.500000". A NaN channel is treated as 0, matching d3's behavior of emitting
// a syntactically valid string rather than propagating the NaN into CSS.
func (c RGB) String() string {
	r, g, b := clamp255(c.R), clamp255(c.G), clamp255(c.B)
	a := clampUnit(c.Opacity)
	if a >= 1 {
		return fmt.Sprintf("rgb(%d, %d, %d)", r, g, b)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", r, g, b, formatFloat(a))
}

// Hex formats the color as a lowercase "#rrggbb" string, clamping and rounding
// as [RGB.String] does. Opacity is dropped: use [RGB.HexA] for "#rrggbbaa".
func (c RGB) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", clamp255(c.R), clamp255(c.G), clamp255(c.B))
}

// HexA formats the color as a lowercase "#rrggbbaa" string, encoding opacity in
// the fourth byte. Note that "#rrggbbaa" is a CSS Color 4 syntax; older parsers
// reject it, which is why [RGB.Hex] remains the default.
func (c RGB) HexA() string {
	a := int(math.Round(clampUnit(c.Opacity) * 255))
	return fmt.Sprintf("#%02x%02x%02x%02x", clamp255(c.R), clamp255(c.G), clamp255(c.B), a)
}

// Displayable reports whether every channel is already inside the sRGB gamut
// (0–255, opacity 0–1), meaning [RGB.String] would not have to clamp anything.
//
// This is the check to run after converting from [Lab], [HCL] or [Cubehelix],
// all of which can describe colors sRGB cannot show. A false result means the
// rendered color will be a clamped approximation of the one you asked for.
// NaN channels are not displayable.
func (c RGB) Displayable() bool {
	return inRange(c.R, 0, 255) && inRange(c.G, 0, 255) && inRange(c.B, 0, 255) &&
		inRange(c.Opacity, 0, 1)
}

// Clamp returns the color with every channel forced into the sRGB gamut,
// without rounding to integers.
//
// Use it when you want the clamping that [RGB.String] performs but want to keep
// computing in floating point afterwards. NaN channels become 0.
func (c RGB) Clamp() RGB {
	return RGB{
		R:       clampFloat(c.R, 0, 255),
		G:       clampFloat(c.G, 0, 255),
		B:       clampFloat(c.B, 0, 255),
		Opacity: clampFloat(c.Opacity, 0, 1),
	}
}

// NRGBA converts to the standard library's non-alpha-premultiplied 8-bit color
// type, clamping and rounding as [RGB.String] does.
//
// This is the bridge to image/draw and friends. It returns the four bytes
// rather than an image/color value so that this package keeps its promise of
// importing nothing beyond fmt/math/strconv/strings; construct
// color.NRGBA{R: r, G: g, B: b, A: a} on the caller's side.
func (c RGB) NRGBA() (r, g, b, a uint8) {
	return uint8(clamp255(c.R)), uint8(clamp255(c.G)), uint8(clamp255(c.B)),
		uint8(math.Round(clampUnit(c.Opacity) * 255))
}

// brighterK is the per-step channel multiplier d3 uses for [RGB.Brighter] and
// [RGB.Darker]: 0.7 darkens, its reciprocal brightens. The value is inherited
// from Protovis, which picked it as a step that reads as "noticeably lighter"
// without washing out. It is a multiplicative operation in sRGB, so it is fast
// and reversible but not perceptually uniform; [Lab] arithmetic is the better
// tool when uniform steps matter.
const brighterK = 0.7

// Brighter returns the color with each channel scaled by (1/0.7)^k, leaving
// opacity untouched.
//
// k is a number of steps, not a percentage, and it need not be a whole number
// or positive: Brighter(-1) equals Darker(1). The default step in d3 is k = 1.
//
// Two properties are worth knowing before reaching for this. Because the
// operation is a pure multiplication, it cannot brighten black — every channel
// is 0 and stays 0 no matter how large k is. And because channels are not
// clamped (see [RGB]), a large k will push channels well past 255; they are
// clamped only when the color is formatted, so brightening then darkening by
// the same k recovers the original value exactly.
func (c RGB) Brighter(k float64) RGB {
	f := math.Pow(1/brighterK, k)
	return RGB{R: c.R * f, G: c.G * f, B: c.B * f, Opacity: c.Opacity}
}

// Darker returns the color with each channel scaled by 0.7^k, leaving opacity
// untouched. It is the exact inverse of [RGB.Brighter] for the same k; see
// there for the semantics of k and for why the result is not clamped.
func (c RGB) Darker(k float64) RGB {
	f := math.Pow(brighterK, k)
	return RGB{R: c.R * f, G: c.G * f, B: c.B * f, Opacity: c.Opacity}
}

// HSL is a color in the CSS HSL space: hue in degrees, saturation and lightness
// in 0–1, opacity in 0–1.
//
// Note the units. CSS writes saturation and lightness as percentages and d3
// stores them as fractions; this package follows d3, so S = 0.5 is "50%".
// [HSL.String] does the multiplication on the way out. Hue is in degrees rather
// than fractions because that is the one channel CSS and d3 agree on.
//
// As with [RGB] the fields are unclamped, and hue in particular is allowed to
// leave [0, 360). That is not a defect: interpolating hue is done by taking the
// shortest path around the circle, which routinely produces intermediate hues
// like -20 or 400, and wrapping them eagerly would break the monotonicity that
// makes the interpolation look smooth. Conversion wraps hue as needed.
//
// HSL is convenient, not perceptual. Two colors with the same L can differ
// enormously in apparent brightness — hsl(60, 1, 0.5) (yellow) is far brighter
// than hsl(240, 1, 0.5) (blue). If you are building a sequential scale, use
// [Lab], [HCL] or [Cubehelix] instead.
type HSL struct {
	H       float64 // hue in degrees, nominally 0–360
	S       float64 // saturation, nominally 0–1
	L       float64 // lightness, nominally 0–1
	Opacity float64 // alpha, nominally 0–1
}

// NewHSL returns an opaque HSL. As with [NewRGB], it exists because the zero
// value has Opacity 0 and would render as fully transparent.
func NewHSL(h, s, l float64) HSL { return HSL{H: h, S: s, L: l, Opacity: 1} }

// NewHSLA returns an HSL with the given channels and opacity.
func NewHSLA(h, s, l, opacity float64) HSL {
	return HSL{H: h, S: s, L: l, Opacity: opacity}
}

// RGBA converts to sRGB using the standard CSS HSL algorithm.
//
// Hue is wrapped into [0, 360) first, and saturation and lightness are clamped
// to 0–1 — unlike the other conversions in this package, which preserve
// out-of-range input, because the HSL→RGB formula is only defined on that
// domain and feeding it S = 3 produces nonsense rather than a usefully
// extrapolated color. A NaN hue is treated as 0 (gray), matching d3, which
// produces NaN hues for achromatic colors.
func (c HSL) RGBA() RGB {
	h := c.H
	if math.IsNaN(h) {
		h = 0
	}
	h = wrapHue(h)
	s := clampFloat(c.S, 0, 1)
	l := clampFloat(c.L, 0, 1)

	var m2 float64
	if l <= 0.5 {
		m2 = l * (1 + s)
	} else {
		m2 = l + s - l*s
	}
	m1 := 2*l - m2
	return RGB{
		R:       hue2rgb(h+120, m1, m2),
		G:       hue2rgb(h, m1, m2),
		B:       hue2rgb(h-120, m1, m2),
		Opacity: c.Opacity,
	}
}

// String formats the color as CSS: "hsl(h, s%, l%)" when opaque, otherwise
// "hsla(h, s%, l%, a)".
//
// Hue is wrapped into [0, 360) and printed with full precision (CSS accepts a
// fractional hue); saturation and lightness are clamped to 0–1, multiplied by
// 100 and rounded to the nearest whole percent, which is what CSS parsers and
// d3 both do. Rounding percentages means [HSL.String] is lossy: parse the
// result back and you get a color within about half a percent of the original,
// not the original. Use [HSL.RGBA] then [RGB.Hex] if you need an exact
// round trip.
func (c HSL) String() string {
	h := c.H
	if math.IsNaN(h) {
		h = 0
	}
	h = wrapHue(h)
	s := math.Round(clampFloat(c.S, 0, 1) * 100)
	l := math.Round(clampFloat(c.L, 0, 1) * 100)
	a := clampUnit(c.Opacity)
	if a >= 1 {
		return fmt.Sprintf("hsl(%s, %d%%, %d%%)", formatFloat(h), int(s), int(l))
	}
	return fmt.Sprintf("hsla(%s, %d%%, %d%%, %s)", formatFloat(h), int(s), int(l), formatFloat(a))
}

// Brighter returns the color with lightness scaled by (1/0.7)^k. See
// [RGB.Brighter] for the meaning of k; as there, the result is not clamped, so
// brightening and then darkening by the same k is exactly reversible.
func (c HSL) Brighter(k float64) HSL {
	c.L *= math.Pow(1/brighterK, k)
	return c
}

// Darker returns the color with lightness scaled by 0.7^k, the inverse of
// [HSL.Brighter].
func (c HSL) Darker(k float64) HSL {
	c.L *= math.Pow(brighterK, k)
	return c
}

// hue2rgb evaluates one channel of the CSS HSL→RGB conversion. h is in degrees
// and may be outside [0, 360); m1 and m2 are the standard lightness bounds.
// The result is scaled to 0–255.
func hue2rgb(h, m1, m2 float64) float64 {
	h = wrapHue(h)
	switch {
	case h < 60:
		return (m1 + (m2-m1)*h/60) * 255
	case h < 180:
		return m2 * 255
	case h < 240:
		return (m1 + (m2-m1)*(240-h)/60) * 255
	default:
		return m1 * 255
	}
}

// ToRGB converts any [Color] to sRGB. It is shorthand for c.RGBA() that also
// tolerates a nil Color, returning transparent black — useful in interpolators
// and scales where a missing color is a normal state rather than an error.
func ToRGB(c Color) RGB {
	if c == nil {
		return RGB{}
	}
	return c.RGBA()
}

// ToHSL converts any [Color] to [HSL], returning it unchanged when it is
// already an HSL (which avoids a needless lossy round trip through sRGB).
//
// The hue of an achromatic color — one where all three channels are equal — is
// undefined, and this function returns NaN for it, exactly as d3 does. That is
// a feature: it lets an interpolator notice that one endpoint has no hue and
// hold the other endpoint's hue constant instead of sweeping to an arbitrary
// zero. [HSL.String] and [HSL.RGBA] both treat a NaN hue as 0, so a NaN never
// escapes into output.
func ToHSL(c Color) HSL {
	if h, ok := c.(HSL); ok {
		return h
	}
	rgb := ToRGB(c)
	r := rgb.R / 255
	g := rgb.G / 255
	b := rgb.B / 255
	min := math.Min(r, math.Min(g, b))
	max := math.Max(r, math.Max(g, b))
	h, s := math.NaN(), max-min
	l := (max + min) / 2
	if s != 0 {
		switch max {
		case r:
			h = (g-b)/s + 6*boolToFloat(g < b)
		case g:
			h = (b-r)/s + 2
		default:
			h = (r-g)/s + 4
		}
		if l < 0.5 {
			s /= max + min
		} else {
			s /= 2 - max - min
		}
		h *= 60
	}
	// When max == min the color is achromatic: s is already 0 and h stays NaN,
	// because a gray has no hue to report.
	return HSL{H: h, S: s, L: l, Opacity: rgb.Opacity}
}

// boolToFloat returns 1 for true and 0 for false. It keeps the hue computation
// in [ToHSL] a single expression, mirroring the JavaScript original.
func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Parse parses a CSS color string.
//
// Accepted syntaxes:
//   - a named color, e.g. "steelblue" or "REBECCAPURPLE" (see [NamedColors])
//   - "transparent", which is rgba(0, 0, 0, 0)
//   - "#rgb", "#rgba", "#rrggbb", "#rrggbbaa"
//   - "rgb(r, g, b)" / "rgba(r, g, b, a)", channels as integers or percentages
//   - "hsl(h, s%, l%)" / "hsla(h, s%, l%, a)"
//
// Leading and trailing whitespace is ignored, matching is case-insensitive, and
// the functional forms accept either commas or plain whitespace as separators
// (the CSS Color 4 space-separated syntax, where alpha follows a "/"). Alpha
// may be written as a number or a percentage in every form that takes one.
//
// The returned [Color] is an [RGB] for named and hex inputs and for rgb()/rgba(),
// and an [HSL] for hsl()/hsla(). Preserving the input space matters: parsing
// "hsl(120, 100%, 50%)" and immediately interpolating hue should not have to
// undo an sRGB round trip first.
//
// Out-of-range channels are kept as written rather than rejected — CSS clamps
// rather than errors, and so does this package, at format time. An input that
// is not a recognizable color at all returns an error whose text includes the
// offending string.
func Parse(s string) (Color, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, fmt.Errorf("color: cannot parse empty string")
	}
	lower := strings.ToLower(t)

	if lower == "transparent" {
		return RGB{}, nil
	}
	if v, ok := namedColors[lower]; ok {
		return NewRGB(float64(v[0]), float64(v[1]), float64(v[2])), nil
	}
	if strings.HasPrefix(lower, "#") {
		return parseHex(lower)
	}
	if i := strings.IndexByte(lower, '('); i > 0 && strings.HasSuffix(lower, ")") {
		fn := strings.TrimSpace(lower[:i])
		args := lower[i+1 : len(lower)-1]
		switch fn {
		case "rgb", "rgba":
			return parseRGBFunc(args, s)
		case "hsl", "hsla":
			return parseHSLFunc(args, s)
		}
	}
	return nil, fmt.Errorf("color: cannot parse %q", s)
}

// MustParse is [Parse] but panics instead of returning an error.
//
// It is for color literals that are known-good at authoring time — palette
// constants, defaults, test fixtures — where threading an error return through
// would be pure noise. Never call it on input that came from a user or a file.
func MustParse(s string) Color {
	c, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return c
}

// parseHex parses "#rgb", "#rgba", "#rrggbb" and "#rrggbbaa".
//
// The 3- and 4-digit forms expand each digit by duplication, not by scaling:
// "#f00" is #ff0000, not #f00000. That is what CSS specifies, and it is why
// the shorthand can express pure white and pure black exactly.
func parseHex(s string) (Color, error) {
	digits := s[1:]
	for i := 0; i < len(digits); i++ {
		if !isHexDigit(digits[i]) {
			return nil, fmt.Errorf("color: cannot parse %q: invalid hex digit %q", s, digits[i])
		}
	}
	nib := func(i int) float64 {
		v := hexVal(digits[i])
		return float64(v<<4 | v)
	}
	pair := func(i int) float64 {
		return float64(hexVal(digits[i])<<4 | hexVal(digits[i+1]))
	}
	switch len(digits) {
	case 3:
		return NewRGB(nib(0), nib(1), nib(2)), nil
	case 4:
		return NewRGBA(nib(0), nib(1), nib(2), nib(3)/255), nil
	case 6:
		return NewRGB(pair(0), pair(2), pair(4)), nil
	case 8:
		return NewRGBA(pair(0), pair(2), pair(4), pair(6)/255), nil
	}
	return nil, fmt.Errorf("color: cannot parse %q: hex color must have 3, 4, 6 or 8 digits", s)
}

// parseRGBFunc parses the argument list of rgb()/rgba().
//
// CSS allows channels as integers (0–255) or percentages (0%–100%); it does not
// allow mixing the two in one color, but this parser is permissive there
// because rejecting the mix buys nothing and complicates the code. orig is the
// caller's original string, used only for error messages.
func parseRGBFunc(args, orig string) (Color, error) {
	parts, err := splitArgs(args, orig)
	if err != nil {
		return nil, err
	}
	if len(parts) != 3 && len(parts) != 4 {
		return nil, fmt.Errorf("color: cannot parse %q: rgb() takes 3 or 4 arguments, got %d", orig, len(parts))
	}
	ch := make([]float64, 3)
	for i := 0; i < 3; i++ {
		v, pct, err := parseNumber(parts[i], orig)
		if err != nil {
			return nil, err
		}
		if pct {
			v = v * 255 / 100
		}
		ch[i] = v
	}
	opacity := 1.0
	if len(parts) == 4 {
		if opacity, err = parseAlpha(parts[3], orig); err != nil {
			return nil, err
		}
	}
	return NewRGBA(ch[0], ch[1], ch[2], opacity), nil
}

// parseHSLFunc parses the argument list of hsl()/hsla(). Hue is a plain number
// of degrees (a trailing "deg" is tolerated); saturation and lightness are
// percentages, though a bare number is accepted and read as a fraction, which
// is how d3 values are usually written in Go code.
func parseHSLFunc(args, orig string) (Color, error) {
	parts, err := splitArgs(args, orig)
	if err != nil {
		return nil, err
	}
	if len(parts) != 3 && len(parts) != 4 {
		return nil, fmt.Errorf("color: cannot parse %q: hsl() takes 3 or 4 arguments, got %d", orig, len(parts))
	}
	h, _, err := parseNumber(strings.TrimSuffix(parts[0], "deg"), orig)
	if err != nil {
		return nil, err
	}
	s, spct, err := parseNumber(parts[1], orig)
	if err != nil {
		return nil, err
	}
	l, lpct, err := parseNumber(parts[2], orig)
	if err != nil {
		return nil, err
	}
	if spct {
		s /= 100
	}
	if lpct {
		l /= 100
	}
	opacity := 1.0
	if len(parts) == 4 {
		if opacity, err = parseAlpha(parts[3], orig); err != nil {
			return nil, err
		}
	}
	return NewHSLA(h, s, l, opacity), nil
}

// splitArgs splits a functional-notation argument list on commas, or — when
// there are no commas — on whitespace with "/" introducing the alpha, which is
// the CSS Color 4 space-separated syntax: rgb(255 0 0 / 50%).
func splitArgs(args, orig string) ([]string, error) {
	var parts []string
	if strings.ContainsRune(args, ',') {
		parts = strings.Split(args, ",")
	} else {
		parts = strings.FieldsFunc(args, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '/'
		})
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("color: cannot parse %q: empty argument", orig)
		}
		out = append(out, p)
	}
	return out, nil
}

// parseNumber parses one channel, reporting whether it carried a percent sign.
func parseNumber(s, orig string) (v float64, percent bool, err error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		percent = true
		s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	}
	v, err = strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, fmt.Errorf("color: cannot parse %q: bad number %q", orig, s)
	}
	return v, percent, nil
}

// parseAlpha parses an alpha argument, which may be a fraction ("0.5") or a
// percentage ("50%").
func parseAlpha(s, orig string) (float64, error) {
	v, pct, err := parseNumber(s, orig)
	if err != nil {
		return 0, err
	}
	if pct {
		v /= 100
	}
	return v, nil
}

func isHexDigit(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

func hexVal(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	default:
		return int(b-'A') + 10
	}
}

// clamp255 clamps and rounds a channel to an integer in 0–255. NaN becomes 0
// so that formatting never emits a NaN into a CSS string.
func clamp255(v float64) int {
	if math.IsNaN(v) {
		return 0
	}
	return int(math.Max(0, math.Min(255, math.Round(v))))
}

// clampUnit clamps a value to 0–1, mapping NaN to 0.
func clampUnit(v float64) float64 { return clampFloat(v, 0, 1) }

// clampFloat clamps v to [lo, hi] without rounding, mapping NaN to lo.
func clampFloat(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

// inRange reports whether v lies in [lo, hi]. NaN is never in range.
func inRange(v, lo, hi float64) bool { return v >= lo && v <= hi }

// wrapHue normalizes a hue in degrees into [0, 360).
func wrapHue(h float64) float64 {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	return h
}

// formatFloat renders a float with the shortest decimal representation that
// round-trips, so 0.5 prints as "0.5" rather than "0.500000" and a hue like
// 207.27272727272728 prints in full rather than being truncated.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
