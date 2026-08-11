package shape

import (
	"math"

	"github.com/malcolmston/d3/path"
)

// SymbolType draws one scatterplot marker, centered on the origin, with the
// given area in square pixels.
//
// Sizing by area rather than by radius or width is the whole point: a viewer
// judges a marker by how much ink it uses, so two symbols of different shape
// but equal size must cover equal area to encode the same value. Every built-in
// type below solves for its dimensions from the requested area.
type SymbolType interface {
	Draw(p *path.Path, size float64)
}

type symbolFunc func(p *path.Path, size float64)

func (f symbolFunc) Draw(p *path.Path, size float64) { f(p, size) }

// Symbol generates the path data for a single marker. Position it with a
// transform, which is what makes one generated string reusable across every
// point of a scatterplot:
//
//	d := shape.NewSymbol().Type(shape.SymbolTriangle).Size(80).Generate()
//	react.H("path", react.Props{"d": d, "transform": fmt.Sprintf("translate(%v,%v)", x, y)})
//
// Unlike the other generators Symbol is not generic: it has no data to iterate
// and no accessors to apply, so a datum type would only add noise. Compute the
// type and size per point yourself and call Generate again.
type Symbol struct {
	typ  SymbolType
	size float64
	digits
}

// NewSymbol returns a circle of area 64, d3's defaults.
func NewSymbol() *Symbol {
	return &Symbol{typ: SymbolCircle, size: 64, digits: noDigits()}
}

// Type sets the marker shape.
func (s *Symbol) Type(t SymbolType) *Symbol { s.typ = t; return s }

// Size sets the marker's area in square pixels.
func (s *Symbol) Size(v float64) *Symbol { s.size = v; return s }

// Digits rounds every emitted coordinate to n decimal places.
func (s *Symbol) Digits(n int) *Symbol { s.digits = digits{n: n}; return s }

// Generate returns the SVG path data for the marker.
func (s *Symbol) Generate() string {
	p := s.newPath()
	s.typ.Draw(p, s.size)
	return p.String()
}

var (
	// SymbolCircle is a filled circle: the default, and the only symbol whose
	// outline has no orientation, which makes it the safest choice when
	// markers overlap.
	SymbolCircle SymbolType = symbolFunc(func(p *path.Path, size float64) {
		r := math.Sqrt(size / pi)
		p.MoveTo(r, 0)
		p.Arc(0, 0, r, 0, tau, false)
	})

	// SymbolCross is a Greek cross (plus sign) built from five unit squares.
	SymbolCross SymbolType = symbolFunc(func(p *path.Path, size float64) {
		r := math.Sqrt(size/5) / 2
		p.MoveTo(-3*r, -r)
		p.LineTo(-r, -r)
		p.LineTo(-r, -3*r)
		p.LineTo(r, -3*r)
		p.LineTo(r, -r)
		p.LineTo(3*r, -r)
		p.LineTo(3*r, r)
		p.LineTo(r, r)
		p.LineTo(r, 3*r)
		p.LineTo(-r, 3*r)
		p.LineTo(-r, r)
		p.LineTo(-3*r, r)
		p.ClosePath()
	})

	// SymbolDiamond is a rhombus with the proportions of a 30-degree rhombus,
	// which reads as distinct from a rotated square.
	SymbolDiamond SymbolType = symbolFunc(func(p *path.Path, size float64) {
		y := math.Sqrt(size / (tan30 * 2))
		x := y * tan30
		p.MoveTo(0, -y)
		p.LineTo(x, 0)
		p.LineTo(0, y)
		p.LineTo(-x, 0)
		p.ClosePath()
	})

	// SymbolSquare is an axis-aligned square.
	SymbolSquare SymbolType = symbolFunc(func(p *path.Path, size float64) {
		w := math.Sqrt(size)
		x := -w / 2
		p.Rect(x, x, w, w)
	})

	// SymbolStar is a five-pointed star.
	SymbolStar SymbolType = symbolFunc(func(p *path.Path, size float64) {
		r := math.Sqrt(size * starKA)
		x := starKX * r
		y := starKY * r
		p.MoveTo(0, -r)
		p.LineTo(x, y)
		for i := 1; i < 5; i++ {
			a := tau * float64(i) / 5
			c, s := math.Cos(a), math.Sin(a)
			p.LineTo(s*r, -c*r)
			p.LineTo(c*x-s*y, s*x+c*y)
		}
		p.ClosePath()
	})

	// SymbolTriangle is an equilateral triangle pointing up.
	SymbolTriangle SymbolType = symbolFunc(func(p *path.Path, size float64) {
		y := -math.Sqrt(size / (sqrt3 * 3))
		p.MoveTo(0, y*2)
		p.LineTo(-sqrt3*y, -y)
		p.LineTo(sqrt3*y, -y)
		p.ClosePath()
	})

	// SymbolWye is a three-armed "Y", the shape that fills the gap between a
	// triangle and a cross in a categorical symbol scale.
	SymbolWye SymbolType = symbolFunc(func(p *path.Path, size float64) {
		r := math.Sqrt(size / wyeA)
		x0, y0 := r/2, r*wyeK
		x1, y1 := x0, r*wyeK+r
		x2, y2 := -x1, y1
		p.MoveTo(x0, y0)
		p.LineTo(x1, y1)
		p.LineTo(x2, y2)
		// The other two arms are the first rotated by ±120°, applied inline
		// with the constant sine and cosine of that angle.
		p.LineTo(wyeC*x0-wyeS*y0, wyeS*x0+wyeC*y0)
		p.LineTo(wyeC*x1-wyeS*y1, wyeS*x1+wyeC*y1)
		p.LineTo(wyeC*x2-wyeS*y2, wyeS*x2+wyeC*y2)
		p.LineTo(wyeC*x0+wyeS*y0, wyeC*y0-wyeS*x0)
		p.LineTo(wyeC*x1+wyeS*y1, wyeC*y1-wyeS*x1)
		p.LineTo(wyeC*x2+wyeS*y2, wyeC*y2-wyeS*x2)
		p.ClosePath()
	})
)

// SymbolsFill is d3's categorical symbol scale: seven shapes chosen to stay
// distinguishable when filled, in the order to assign them.
var SymbolsFill = []SymbolType{
	SymbolCircle, SymbolCross, SymbolDiamond, SymbolSquare,
	SymbolStar, SymbolTriangle, SymbolWye,
}

var (
	tan30 = math.Sqrt(1.0 / 3.0)
	sqrt3 = math.Sqrt(3)

	// Solving a five-pointed star's area for its circumradius has no tidy
	// closed form, so d3 carries the constant; kr is the ratio of the inner
	// radius to the outer.
	starKA = 0.89081309152928522810
	starKR = math.Sin(pi/10) / math.Sin(7*pi/10)
	starKX = math.Sin(tau/10) * starKR
	starKY = -math.Cos(tau/10) * starKR

	// The wye's arms are 120° apart; c and s are the cosine and sine of that
	// rotation, k the offset of an arm's root from the center, and a the area
	// of the unit wye.
	wyeC = -0.5
	wyeS = math.Sqrt(3) / 2
	wyeK = 1 / math.Sqrt(12)
	wyeA = (wyeK/2 + 1) * 3
)
