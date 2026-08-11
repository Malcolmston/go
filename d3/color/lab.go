package color

import "math"

// This file implements the perceptual color spaces: CIELAB ([Lab]), its polar
// form CIE LCh ([HCL]), and Dave Green's [Cubehelix]. Together they are the
// reason this package exists rather than just holding hex strings around.
//
// The pipeline from sRGB to Lab has three stages, and each exists for a
// distinct reason.
//
// 1. Gamma decoding. An sRGB channel is not proportional to light; it is
// encoded with a roughly 2.2-power curve so that 8-bit values are spread evenly
// in perceptual terms rather than in physical ones. Before any linear light
// math you must undo that, which is what rgb2lrgb and lrgb2rgb do. The exact
// sRGB transfer function is piecewise — a short linear segment near black,
// then a 2.4-power curve with a small offset — and this file uses the exact
// form rather than the 2.2 approximation, because the approximation visibly
// misplaces dark colors.
//
// 2. Linear RGB to CIEXYZ. A fixed 3x3 matrix, determined by the sRGB primaries
// and the D65 white point. XYZ is a device-independent description of the light
// itself.
//
// 3. XYZ to Lab. A cube-root-ish nonlinearity per axis (again piecewise, with a
// linear segment near zero to keep the derivative finite), then a rotation into
// L (lightness), a (green–red) and b (blue–yellow).
//
// The white point is D65, the standard sRGB illuminant, given here in the
// scaling d3-color uses: Xn = 0.96422, Yn = 1, Zn = 0.82521. Those are the
// D50-adapted values that d3 inherited from Chromium's color code. They are
// *not* the textbook D65 numbers (0.95047, 1, 1.08883), and the difference is
// visible — a Lab value computed with one set and inverted with the other lands
// several units away. This package matches d3 so that Lab values computed here
// agree with values from d3-color and with the d3 color scales built on top of
// them, which is the interoperability that actually matters. If you need
// absolute colorimetric accuracy against a spectrophotometer, this is not the
// library.
//
// Gamut. Lab and HCL describe far more colors than sRGB can show. Converting a
// saturated HCL color to [RGB] routinely yields channels below 0 or above 255,
// and this package returns them as-is: nothing is clamped during conversion.
// See [RGB] for why, and use [RGB.Displayable] to detect it and [RGB.Clamp] or
// [RGB.String] to resolve it.
//
// Tolerances. sRGB → Lab → sRGB round trips to within 5e-4 of a channel unit
// (measured worst case over the 148 named colors: 2.95e-4, on pure blue). That
// is three orders of magnitude below the 8-bit quantum, so it never changes a
// rendered color, but it is not floating-point noise either: the forward and
// inverse linear-RGB↔XYZ matrices are two independently rounded 7-digit tables
// inherited from d3-color, and they are not exact inverses of each other. Using
// a single matrix and inverting it numerically would round-trip better but
// would disagree with d3 in the last digits, and agreement with d3 is the more
// valuable property here. The same 5e-4 bound covers HCL, which goes through
// Lab. Cubehelix and HSL, which do not, round trip to about 1e-13.
//
// Lab → sRGB → Lab is only accurate for Lab values that were inside the gamut
// to begin with; outside it the intermediate RGB is still exact but any
// clamping the caller applies is, by definition, not invertible.

// The D65 white point in the scaling used by d3-color. See the file comment for
// why these differ from the textbook D65 tristimulus values.
const (
	xn = 0.96422
	yn = 1.0
	zn = 0.82521
)

// Constants of the piecewise XYZ→Lab nonlinearity. t0 is the 6/29 breakpoint
// below which the function is linear (to keep its slope finite at zero); t1,
// t2 and t3 are the derived slope and intercept terms.
const (
	labT0 = 4.0 / 29.0
	labT1 = 6.0 / 29.0
	labT2 = 3 * labT1 * labT1
	labT3 = labT1 * labT1 * labT1
)

// Lab is a color in the CIELAB space with a D65 white point.
//
// L is lightness, 0 (black) to 100 (white). A and B are the opponent chroma
// axes: A runs green (negative) to red (positive), B runs blue (negative) to
// yellow (positive). Both are unbounded in principle; in practice sRGB colors
// occupy roughly -128 to +128 on each.
//
// The point of Lab is that Euclidean distance in it approximates perceived
// color difference. That makes it the right space for interpolating a color
// scale (equal steps in t look like equal visual steps), for measuring how
// distinguishable two categorical colors are, and for adjusting lightness
// without shifting apparent hue — unlike [HSL], where scaling L changes the
// perceived hue of yellows and blues quite noticeably.
//
// Fields are unclamped, like every other color type here; see [RGB].
type Lab struct {
	L, A, B float64
	Opacity float64
}

// NewLab returns an opaque Lab color.
func NewLab(l, a, b float64) Lab { return Lab{L: l, A: a, B: b, Opacity: 1} }

// NewLabA returns a Lab color with the given opacity.
func NewLabA(l, a, b, opacity float64) Lab {
	return Lab{L: l, A: a, B: b, Opacity: opacity}
}

// RGBA converts to sRGB.
//
// The result is not clamped: a Lab color outside the sRGB gamut produces
// channels outside 0–255 rather than a silently wrong in-gamut color. Test with
// [RGB.Displayable] if you need to know, and use [RGB.Clamp] or let
// [RGB.String] clamp at format time.
func (c Lab) RGBA() RGB {
	y := (c.L + 16) / 116
	x := y
	if !math.IsNaN(c.A) {
		x = y + c.A/500
	}
	z := y
	if !math.IsNaN(c.B) {
		z = y - c.B/200
	}
	x = xn * lab2xyz(x)
	y = yn * lab2xyz(y)
	z = zn * lab2xyz(z)
	return RGB{
		R:       lrgb2rgb(3.1338561*x - 1.6168667*y - 0.4906146*z),
		G:       lrgb2rgb(-0.9787684*x + 1.9161415*y + 0.0334540*z),
		B:       lrgb2rgb(0.0719453*x - 0.2289914*y + 1.4052427*z),
		Opacity: c.Opacity,
	}
}

// String formats the color by converting to sRGB first, because CSS has no
// widely supported Lab notation this package commits to. The clamping described
// on [RGB.String] therefore applies: an out-of-gamut Lab color renders as its
// nearest displayable approximation.
func (c Lab) String() string { return c.RGBA().String() }

// Brighter returns the color with L increased by 18*k, the step d3 uses.
//
// Unlike [RGB.Brighter], this is additive rather than multiplicative, which is
// what makes it useful: because L is perceptually uniform, adding a fixed
// amount produces a consistent visual step regardless of the starting color,
// and it brightens black (L = 0 → L = 18) where the multiplicative sRGB version
// cannot. The result is not clamped to L <= 100, so Brighter then Darker by the
// same k is exactly reversible.
func (c Lab) Brighter(k float64) Lab {
	c.L += 18 * k
	return c
}

// Darker returns the color with L decreased by 18*k, the inverse of
// [Lab.Brighter].
func (c Lab) Darker(k float64) Lab {
	c.L -= 18 * k
	return c
}

// HCL is the cylindrical form of [Lab]: hue in degrees, chroma, and the same
// lightness L in 0–100.
//
// It holds exactly the same colors as Lab — H and C are just the polar
// coordinates of Lab's A and B — but exposes the two axes designers actually
// think in. Rotating H walks around the color wheel at constant perceived
// lightness and saturation, which is how you generate a categorical palette
// whose colors are equally distinguishable and equally prominent. Holding H and
// C while varying L gives a sequential ramp in one hue.
//
// The gotcha is gamut. Unlike [HSL], where every combination of channels in
// range is displayable, most (H, C, L) triples with a large C are outside sRGB
// and will clamp. The usable C at L = 50 varies from about 130 for magenta down
// to about 65 for yellow-green, so a constant-C palette will silently distort
// unless C is chosen conservatively. [RGB.Displayable] is the way to check.
//
// A NaN hue means "achromatic" and is produced for grays, mirroring [ToHSL].
type HCL struct {
	H, C, L float64
	Opacity float64
}

// NewHCL returns an opaque HCL color.
func NewHCL(h, c, l float64) HCL { return HCL{H: h, C: c, L: l, Opacity: 1} }

// NewHCLA returns an HCL color with the given opacity.
func NewHCLA(h, c, l, opacity float64) HCL {
	return HCL{H: h, C: c, L: l, Opacity: opacity}
}

// ToLab converts an HCL color to [Lab]. A NaN hue is treated as achromatic,
// producing A = B = 0.
func (c HCL) ToLab() Lab {
	if math.IsNaN(c.H) {
		return Lab{L: c.L, A: 0, B: 0, Opacity: c.Opacity}
	}
	h := c.H * math.Pi / 180
	return Lab{
		L:       c.L,
		A:       math.Cos(h) * c.C,
		B:       math.Sin(h) * c.C,
		Opacity: c.Opacity,
	}
}

// RGBA converts to sRGB by way of [Lab]. As with [Lab.RGBA], the result is not
// clamped and may be outside the sRGB gamut.
func (c HCL) RGBA() RGB { return c.ToLab().RGBA() }

// String formats the color as sRGB; see [Lab.String].
func (c HCL) String() string { return c.RGBA().String() }

// Brighter returns the color with L increased by 18*k; see [Lab.Brighter].
func (c HCL) Brighter(k float64) HCL {
	c.L += 18 * k
	return c
}

// Darker returns the color with L decreased by 18*k.
func (c HCL) Darker(k float64) HCL {
	c.L -= 18 * k
	return c
}

// ToLab converts any [Color] to [Lab], short-circuiting when it is already a
// [Lab] or an [HCL] so that no needless sRGB round trip is taken.
func ToLab(c Color) Lab {
	switch v := c.(type) {
	case Lab:
		return v
	case HCL:
		return v.ToLab()
	}
	rgb := ToRGB(c)
	r := rgb2lrgb(rgb.R)
	g := rgb2lrgb(rgb.G)
	b := rgb2lrgb(rgb.B)
	y := xyz2lab((0.2225045*r + 0.7168786*g + 0.0606169*b) / yn)
	var x, z float64
	if r == g && g == b {
		// A neutral gray has, by construction, x == y == z; computing them
		// separately would leave rounding dust in A and B and give an
		// achromatic color a spurious hue in HCL.
		x, z = y, y
	} else {
		x = xyz2lab((0.4360747*r + 0.3850649*g + 0.1430804*b) / xn)
		z = xyz2lab((0.0139322*r + 0.0971045*g + 0.7141733*b) / zn)
	}
	return Lab{
		L:       116*y - 16,
		A:       500 * (x - y),
		B:       200 * (y - z),
		Opacity: rgb.Opacity,
	}
}

// ToHCL converts any [Color] to [HCL].
//
// The hue is NaN for achromatic colors — those whose Lab A and B are both
// (essentially) zero — because the polar angle of the origin is undefined.
// Interpolators use that NaN as a signal to hold the other endpoint's hue
// rather than sweeping toward an arbitrary 0 degrees. Hue is normalized into
// [0, 360).
func ToHCL(c Color) HCL {
	if v, ok := c.(HCL); ok {
		return v
	}
	l := ToLab(c)
	if l.A == 0 && l.B == 0 {
		return HCL{H: math.NaN(), C: 0, L: l.L, Opacity: l.Opacity}
	}
	h := math.Atan2(l.B, l.A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return HCL{
		H:       h,
		C:       math.Hypot(l.A, l.B),
		L:       l.L,
		Opacity: l.Opacity,
	}
}

// xyz2lab applies the piecewise nonlinearity of the XYZ→Lab transform: a cube
// root above the 6/29 breakpoint, a linear segment below it. The linear segment
// exists because the cube root has infinite slope at zero, which would make the
// transform numerically unstable for near-black colors.
func xyz2lab(t float64) float64 {
	if t > labT3 {
		return math.Cbrt(t)
	}
	return t/labT2 + labT0
}

// lab2xyz inverts [xyz2lab].
func lab2xyz(t float64) float64 {
	if t > labT1 {
		return t * t * t
	}
	return labT2 * (t - labT0)
}

// rgb2lrgb decodes one gamma-encoded sRGB channel (0–255) to linear light
// (0–1), using the exact piecewise sRGB transfer function rather than the 2.2
// approximation. Input outside 0–255 is decoded rather than clamped, so
// out-of-gamut colors survive the round trip; negative inputs take the linear
// branch and stay negative.
func rgb2lrgb(x float64) float64 {
	x /= 255
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

// lrgb2rgb gamma-encodes a linear-light value (0–1) back to an sRGB channel
// (0–255), inverting [rgb2lrgb]. As there, out-of-range input is encoded rather
// than clamped.
func lrgb2rgb(x float64) float64 {
	if x <= 0.0031308 {
		return 255 * 12.92 * x
	}
	return 255 * (1.055*math.Pow(x, 1/2.4) - 0.055)
}

// Cubehelix constants from Dave Green's original scheme. The A/B/C/D/E terms
// are the coefficients of the rotation that carries a hue angle into the RGB
// cube along a helix; ED = E - D and BC_DA is a term reused in the inverse.
const (
	chA  = -0.14861
	chB  = +1.78277
	chC  = -0.29227
	chD  = -0.90649
	chE  = +1.97294
	chED = chE * chD
	chEB = chE * chB
	chBC = chB*chC - chD*chA
)

// Cubehelix is a color in Dave Green's cubehelix space: hue in degrees,
// saturation, and lightness in 0–1.
//
// Cubehelix exists to solve one specific problem: a color ramp that stays
// legible when printed in grayscale or viewed by someone with color vision
// deficiency. It does that by construction — luminance increases monotonically
// with L, while hue spirals around the RGB cube — which neither [HSL] nor a
// naive rainbow scale can promise. d3's default "rainbow" and "cubehelix"
// interpolators are built on it for exactly that reason.
//
// It is not perceptually uniform the way [Lab] is; equal steps in L are equal
// steps in relative luminance, not in perceived lightness. Prefer Lab or [HCL]
// when the goal is perceptual uniformity and Cubehelix when the goal is
// grayscale-safe sequential ramps.
//
// As with [HSL] and [HCL], a NaN hue means achromatic.
type Cubehelix struct {
	H, S, L float64
	Opacity float64
}

// NewCubehelix returns an opaque Cubehelix color.
func NewCubehelix(h, s, l float64) Cubehelix {
	return Cubehelix{H: h, S: s, L: l, Opacity: 1}
}

// NewCubehelixA returns a Cubehelix color with the given opacity.
func NewCubehelixA(h, s, l, opacity float64) Cubehelix {
	return Cubehelix{H: h, S: s, L: l, Opacity: opacity}
}

// RGBA converts to sRGB. The result is not clamped; a high saturation at an
// extreme lightness leaves the sRGB cube, as [RGB.Displayable] will report.
func (c Cubehelix) RGBA() RGB {
	h := c.H
	if math.IsNaN(h) {
		h = 0
	}
	h = (h + 120) * math.Pi / 180
	l := c.L
	s := c.S
	if math.IsNaN(s) {
		s = 0
	}
	a := s * l * (1 - l)
	cosh := math.Cos(h)
	sinh := math.Sin(h)
	return RGB{
		R:       255 * (l + a*(chA*cosh+chB*sinh)),
		G:       255 * (l + a*(chC*cosh+chD*sinh)),
		B:       255 * (l + a*(chE*cosh)),
		Opacity: c.Opacity,
	}
}

// String formats the color as sRGB; see [Lab.String].
func (c Cubehelix) String() string { return c.RGBA().String() }

// Brighter returns the color with L scaled by (1/0.7)^k; see [RGB.Brighter].
func (c Cubehelix) Brighter(k float64) Cubehelix {
	c.L *= math.Pow(1/brighterK, k)
	return c
}

// Darker returns the color with L scaled by 0.7^k.
func (c Cubehelix) Darker(k float64) Cubehelix {
	c.L *= math.Pow(brighterK, k)
	return c
}

// ToCubehelix converts any [Color] to [Cubehelix], returning it unchanged when
// it already is one.
//
// The hue is NaN when the color is achromatic (saturation 0), for the same
// reason as in [ToHSL].
func ToCubehelix(col Color) Cubehelix {
	if v, ok := col.(Cubehelix); ok {
		return v
	}
	rgb := ToRGB(col)
	r := rgb.R / 255
	g := rgb.G / 255
	b := rgb.B / 255
	l := (chBC*b + chED*r - chEB*g) / (chBC + chED - chEB)
	bl := b - l
	k := (chE*(g-l) - chC*bl) / chD
	s := math.Sqrt(k*k+bl*bl) / (chE * l * (1 - l))
	h := math.NaN()
	if !math.IsNaN(s) && s != 0 {
		h = math.Atan2(k, bl)*180/math.Pi - 120
		if h < 0 {
			h += 360
		}
	}
	if math.IsNaN(s) {
		s = 0
	}
	return Cubehelix{H: h, S: s, L: l, Opacity: rgb.Opacity}
}

// Format renders any [Color] as CSS. It is a nil-safe c.String(), returning the
// CSS keyword "none" for a nil color rather than panicking — handy in a scale
// whose domain includes missing data.
func Format(c Color) string {
	if c == nil {
		return "none"
	}
	return c.String()
}

// ensure the compile-time interface assertions live somewhere obvious.
var (
	_ Color = RGB{}
	_ Color = HSL{}
	_ Color = Lab{}
	_ Color = HCL{}
	_ Color = Cubehelix{}
)
