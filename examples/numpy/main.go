// Command numpy-example exercises the github.com/malcolmston/numpy port:
// array creation, shapes/reshape, views and slicing, broadcasting, element-wise
// and matrix ops, axis reductions, sorting/searching, statistics, comparison
// masking and the linear-algebra helpers.
//
// Every interesting result is checked against a hand-computed expectation and
// any mismatch is reported (and counted) rather than silently ignored.
package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	np "github.com/malcolmston/numpy"
)

var (
	checks   int
	failures []string
)

const tol = 1e-9

func section(name string) {
	fmt.Printf("\n== %s %s\n", name, strings.Repeat("=", max(0, 62-len(name))))
}

func fail(what string, got, want any) {
	msg := fmt.Sprintf("MISMATCH %s: got %v, want %v", what, got, want)
	failures = append(failures, msg)
	fmt.Println("  !! " + msg)
}

// checkSlice compares a flat float slice against expected values.
func checkSlice(what string, got, want []float64) {
	checks++
	if len(got) != len(want) {
		fail(what, got, want)
		return
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol || (math.IsNaN(got[i]) != math.IsNaN(want[i])) {
			fail(what, got, want)
			return
		}
	}
	fmt.Printf("  ok  %-34s %v\n", what, got)
}

func checkFloat(what string, got, want float64) {
	checks++
	if math.Abs(got-want) > tol {
		fail(what, got, want)
		return
	}
	fmt.Printf("  ok  %-34s %v\n", what, got)
}

func checkInts(what string, got, want []int) {
	checks++
	if fmt.Sprint(got) != fmt.Sprint(want) {
		fail(what, got, want)
		return
	}
	fmt.Printf("  ok  %-34s %v\n", what, got)
}

func checkInt(what string, got, want int) {
	checks++
	if got != want {
		fail(what, got, want)
		return
	}
	fmt.Printf("  ok  %-34s %v\n", what, got)
}

func checkBool(what string, got, want bool) {
	checks++
	if got != want {
		fail(what, got, want)
		return
	}
	fmt.Printf("  ok  %-34s %v\n", what, got)
}

func main() {
	fmt.Println("numpy port example — github.com/malcolmston/numpy")

	creation()
	shapes()
	slicingAndViews()
	broadcasting()
	elementwise()
	reductions()
	sortingAndStats()
	maskingAndWhere()
	linalg()
	dtypes()
	errorBehavior()

	section("SUMMARY")
	fmt.Printf("  %d numeric checks run, %d mismatches\n", checks, len(failures))
	for _, f := range failures {
		fmt.Println("   - " + f)
	}
	if len(failures) > 0 {
		os.Exit(1)
	}
}

func creation() {
	section("1. Array creation")

	a := np.Arange(0, 6, 1)
	checkSlice("Arange(0,6,1)", a.Data(), []float64{0, 1, 2, 3, 4, 5})
	checkInts("  .Shape()", a.Shape(), []int{6})

	// Linspace with endpoint=true includes the stop value.
	lin := np.Linspace(0, 1, 5, true)
	checkSlice("Linspace(0,1,5,true)", lin.Data(), []float64{0, 0.25, 0.5, 0.75, 1})
	// endpoint=false behaves like NumPy's endpoint=False: step = (stop-start)/num.
	linNoEnd := np.Linspace(0, 1, 5, false)
	checkSlice("Linspace(0,1,5,false)", linNoEnd.Data(), []float64{0, 0.2, 0.4, 0.6, 0.8})

	z := np.Zeros(2, 3)
	checkSlice("Zeros(2,3)", z.Data(), []float64{0, 0, 0, 0, 0, 0})
	o := np.Ones(3)
	checkSlice("Ones(3)", o.Data(), []float64{1, 1, 1})
	f := np.Full(7, 2, 2)
	checkSlice("Full(7,2,2)", f.Data(), []float64{7, 7, 7, 7})
	checkSlice("ZerosLike(Full)", np.ZerosLike(f).Data(), []float64{0, 0, 0, 0})
	checkSlice("OnesLike(Full)", np.OnesLike(f).Data(), []float64{1, 1, 1, 1})

	// FromData takes flat data + shape; FromNested infers shape from a
	// nested Go literal.
	m := np.FromData([]float64{1, 2, 3, 4, 5, 6}, 2, 3)
	checkInts("FromData(...,2,3).Shape()", m.Shape(), []int{2, 3})
	nested := np.FromNested([][]float64{{1, 2}, {3, 4}, {5, 6}})
	checkInts("FromNested 3x2 shape", nested.Shape(), []int{3, 2})
	checkSlice("FromNested data", nested.Data(), []float64{1, 2, 3, 4, 5, 6})
	deep := np.FromNested([][][]float64{{{1, 2}, {3, 4}}, {{5, 6}, {7, 8}}})
	checkInts("FromNested 2x2x2 shape", deep.Shape(), []int{2, 2, 2})

	// Eye(n, m, k): k offsets the diagonal.
	checkSlice("Eye(3,3,0)", np.Eye(3, 3, 0).Data(),
		[]float64{1, 0, 0, 0, 1, 0, 0, 0, 1})
	checkSlice("Eye(3,3,1)", np.Eye(3, 3, 1).Data(),
		[]float64{0, 1, 0, 0, 0, 1, 0, 0, 0})
	checkSlice("Identity(2)", np.Identity(2).Data(), []float64{1, 0, 0, 1})

	fmt.Println("  String() of a 2x3 array:")
	fmt.Println("   ", strings.ReplaceAll(m.String(), "\n", "\n    "))
}

func shapes() {
	section("2. Shapes, reshape, transpose")

	a := np.Arange(0, 12, 1)
	b := a.Reshape(3, 4)
	checkInts("Reshape(3,4).Shape()", b.Shape(), []int{3, 4})
	checkInts("Reshape(3,4).Strides()", b.Strides(), []int{4, 1})
	checkInt("Ndim()", b.Ndim(), 2)
	checkInt("Size()", b.Size(), 12)

	// One axis may be -1 and is inferred.
	inferred := a.Reshape(2, -1)
	checkInts("Reshape(2,-1).Shape()", inferred.Shape(), []int{2, 6})

	// At/Set with negative indexing.
	checkFloat("b.At(1,2)", b.At(1, 2), 6)
	checkFloat("b.At(-1,-1)", b.At(-1, -1), 11)
	c := b.Copy()
	c.Set(99, 0, 0)
	checkFloat("Copy+Set does not alias", b.At(0, 0), 0)
	checkFloat("  copy sees the write", c.At(0, 0), 99)

	// Transpose is a stride permutation (a view).
	t := b.T()
	checkInts("b.T().Shape()", t.Shape(), []int{4, 3})
	checkInts("b.T().Strides()", t.Strides(), []int{1, 4})
	checkFloat("b.T().At(2,1)", t.At(2, 1), b.At(1, 2))
	checkSlice("b.T().Ravel() row-major", t.Ravel().Data(),
		[]float64{0, 4, 8, 1, 5, 9, 2, 6, 10, 3, 7, 11})

	// Explicit axis permutation on 3-D.
	cube := np.Arange(0, 24, 1).Reshape(2, 3, 4)
	p := cube.Transpose(2, 0, 1)
	checkInts("cube.Transpose(2,0,1).Shape()", p.Shape(), []int{4, 2, 3})
	checkFloat("permuted element identity", p.At(3, 1, 2), cube.At(1, 2, 3))

	// Squeeze / ExpandDims / Flip / Roll.
	sq := np.Zeros(1, 3, 1).Squeeze()
	checkInts("Zeros(1,3,1).Squeeze()", sq.Shape(), []int{3})
	ex := np.Arange(0, 3, 1).ExpandDims(0)
	checkInts("ExpandDims(0)", ex.Shape(), []int{1, 3})
	checkSlice("Flip(axis 0) of [0..3]", np.Arange(0, 4, 1).Flip(0).Data(),
		[]float64{3, 2, 1, 0})
	checkSlice("Roll(+1) of [0..4]", np.Arange(0, 5, 1).Roll(1).Data(),
		[]float64{4, 0, 1, 2, 3})
	checkSlice("Roll(-2) of [0..4]", np.Arange(0, 5, 1).Roll(-2).Data(),
		[]float64{2, 3, 4, 0, 1})

	// Concatenate / Stack.
	x := np.FromData([]float64{1, 2, 3, 4}, 2, 2)
	y := np.FromData([]float64{5, 6, 7, 8}, 2, 2)
	cat0 := np.Concatenate(0, x, y)
	checkInts("Concatenate(0) shape", cat0.Shape(), []int{4, 2})
	checkSlice("Concatenate(0) data", cat0.Data(), []float64{1, 2, 3, 4, 5, 6, 7, 8})
	cat1 := np.Concatenate(1, x, y)
	checkInts("Concatenate(1) shape", cat1.Shape(), []int{2, 4})
	checkSlice("Concatenate(1) data", cat1.Data(), []float64{1, 2, 5, 6, 3, 4, 7, 8})
	st := np.Stack(0, x, y)
	checkInts("Stack(0) shape", st.Shape(), []int{2, 2, 2})
}

func slicingAndViews() {
	section("3. Slicing and views")

	a := np.Arange(0, 12, 1).Reshape(3, 4)

	// Slice requires one Range per axis; a zero Range means "whole axis".
	sub := a.Slice(np.R(1, 3), np.R(1, 3))
	checkInts("a[1:3,1:3] shape", sub.Shape(), []int{2, 2})
	checkSlice("a[1:3,1:3] values", []float64{
		sub.At(0, 0), sub.At(0, 1), sub.At(1, 0), sub.At(1, 1),
	}, []float64{5, 6, 9, 10})

	row := a.Slice(np.R(2, 3), np.Range{})
	checkSlice("a[2:3, :] ravelled", row.Ravel().Data(), []float64{8, 9, 10, 11})

	// Negative Start/Stop count from the end, but Stop must be given
	// explicitly as a positive index to reach the last element (see below).
	neg := a.Slice(np.R(-1, 3), np.R(-2, 4))
	checkInts("a[-1:3,-2:4] shape", neg.Shape(), []int{1, 2})
	checkSlice("a[-1:3,-2:4] values", neg.Ravel().Data(), []float64{10, 11})

	// HOLE: Range's zero value is overloaded to mean "whole axis", so an
	// open-ended slice like NumPy's a[1:] is inexpressible: R(1, 0) resolves
	// Stop to 0, then clamps Stop up to Start, producing an empty axis instead
	// of running to the end. There is no sentinel for "to the end" and no
	// step/stride field either (a[::2] cannot be expressed).
	openEnded := a.Slice(np.R(1, 0), np.Range{})
	checkInts("R(1,0) yields an EMPTY axis", openEnded.Shape(), []int{0, 4})
	fmt.Println("      (NumPy a[1:,:] would be shape [2 4] — must write R(1,3))")

	// Slice returns a view: writing through it is visible in the parent.
	sub.Set(-1, 0, 0)
	checkFloat("view write visible in parent", a.At(1, 1), -1)
	a.Set(5, 1, 1) // restore

	// ...while Ravel of a non-contiguous view copies.
	rv := sub.Ravel()
	rv.Set(1234, 0)
	checkFloat("Ravel of view is a copy", a.At(1, 1), 5)
}

func broadcasting() {
	section("4. Broadcasting")

	a := np.Arange(0, 6, 1).Reshape(2, 3)
	b := np.FromSlice([]float64{10, 20, 30}) // (3,)

	// (2,3) + (3,) -> (2,3)
	sum := a.Add(b)
	checkInts("(2,3)+(3,) shape", sum.Shape(), []int{2, 3})
	checkSlice("(2,3)+(3,) data", sum.Data(), []float64{10, 21, 32, 13, 24, 35})

	// (3,1) * (1,4) -> (3,4) outer-product style broadcast.
	col := np.FromData([]float64{1, 2, 3}, 3, 1)
	rowv := np.FromData([]float64{10, 20, 30, 40}, 1, 4)
	grid := col.Mul(rowv)
	checkInts("(3,1)*(1,4) shape", grid.Shape(), []int{3, 4})
	checkSlice("(3,1)*(1,4) data", grid.Data(), []float64{
		10, 20, 30, 40,
		20, 40, 60, 80,
		30, 60, 90, 120,
	})

	// Explicit BroadcastTo. Note: unlike NumPy's broadcast_to (a read-only
	// view), this one materializes a contiguous copy, so its strides are the
	// ordinary row-major ones rather than containing a 0.
	bt := np.FromData([]float64{1, 2, 3}, 1, 3).BroadcastTo(4, 3)
	checkInts("BroadcastTo(4,3) shape", bt.Shape(), []int{4, 3})
	checkInts("BroadcastTo strides (a copy)", bt.Strides(), []int{3, 1})
	checkFloat("broadcast element", bt.At(3, 2), 3)

	// Incompatible shapes panic (documented behavior).
	err := capturePanic(func() { a.Add(np.Zeros(4)) })
	checkBool("(2,3)+(4,) panics", err != "", true)
	fmt.Printf("      panic text: %s\n", err)
}

func elementwise() {
	section("5. Element-wise and matrix ops")

	a := np.FromSlice([]float64{1, 2, 3, 4})
	b := np.FromSlice([]float64{4, 3, 2, 1})

	checkSlice("Add", a.Add(b).Data(), []float64{5, 5, 5, 5})
	checkSlice("Sub", a.Sub(b).Data(), []float64{-3, -1, 1, 3})
	checkSlice("Mul", a.Mul(b).Data(), []float64{4, 6, 6, 4})
	checkSlice("Div", a.Div(b).Data(), []float64{0.25, 2.0 / 3.0, 1.5, 4})
	checkSlice("Pow", a.Pow(np.FromSlice([]float64{2, 2, 2, 2})).Data(),
		[]float64{1, 4, 9, 16})
	checkSlice("AddScalar(10)", a.AddScalar(10).Data(), []float64{11, 12, 13, 14})
	checkSlice("MulScalar(-2)", a.MulScalar(-2).Data(), []float64{-2, -4, -6, -8})
	checkSlice("PowScalar(0.5)", a.PowScalar(0.5).Data(),
		[]float64{1, math.Sqrt2, math.Sqrt(3), 2})
	checkSlice("Neg", a.Neg().Data(), []float64{-1, -2, -3, -4})
	checkSlice("Square", a.Square().Data(), []float64{1, 4, 9, 16})
	checkSlice("Reciprocal", a.Reciprocal().Data(), []float64{1, 0.5, 1.0 / 3.0, 0.25})
	checkSlice("Maximum(a,b)", a.Maximum(b).Data(), []float64{4, 3, 3, 4})
	checkSlice("Minimum(a,b)", a.Minimum(b).Data(), []float64{1, 2, 2, 1})
	checkSlice("Clip(2,3)", a.Clip(2, 3).Data(), []float64{2, 2, 3, 3})

	// Transcendentals / rounding.
	ang := np.FromSlice([]float64{0, math.Pi / 6, math.Pi / 2})
	checkSlice("Sin", ang.Sin().Data(), []float64{0, 0.5, 1})
	checkSlice("Cos", ang.Cos().Data(), []float64{1, math.Sqrt(3) / 2, 0})
	checkSlice("Exp then Log round-trips", a.Exp().Log().Data(), []float64{1, 2, 3, 4})
	checkSlice("Log2([1 2 4 8])", np.FromSlice([]float64{1, 2, 4, 8}).Log2().Data(),
		[]float64{0, 1, 2, 3})
	checkSlice("Log10([1 10 100 1000])",
		np.FromSlice([]float64{1, 10, 100, 1000}).Log10().Data(),
		[]float64{0, 1, 2, 3})
	checkSlice("Cbrt([1 8 27 64])",
		np.FromSlice([]float64{1, 8, 27, 64}).Cbrt().Data(), []float64{1, 2, 3, 4})
	checkSlice("Hypot([3],[4])", np.FromSlice([]float64{3}).Hypot(np.FromSlice([]float64{4})).Data(),
		[]float64{5})
	checkSlice("Arctan2(1,1) == pi/4",
		np.FromSlice([]float64{1}).Arctan2(np.FromSlice([]float64{1})).Data(),
		[]float64{math.Pi / 4})
	r := np.FromSlice([]float64{-1.7, -0.5, 0.5, 1.5, 2.5})
	checkSlice("Floor", r.Floor().Data(), []float64{-2, -1, 0, 1, 2})
	checkSlice("Ceil", r.Ceil().Data(), []float64{-1, 0, 1, 2, 3})
	checkSlice("Trunc", r.Trunc().Data(), []float64{-1, 0, 0, 1, 2})
	// Round uses math.RoundToEven (NumPy's banker's rounding).
	checkSlice("Round (half-to-even)", r.Round().Data(), []float64{-2, -0, 0, 2, 2})
	checkSlice("Sign", r.Sign().Data(), []float64{-1, -1, 1, 1, 1})
	checkSlice("Mod([7 8],[3 3])",
		np.FromSlice([]float64{7, 8}).Mod(np.FromSlice([]float64{3, 3})).Data(),
		[]float64{1, 2})

	// Matrix products. Hand-computed:
	//   [[1 2][3 4]] @ [[1 2][3 4]] = [[7 10][15 22]]
	m := np.FromData([]float64{1, 2, 3, 4}, 2, 2)
	checkSlice("MatMul 2x2", m.MatMul(m).Data(), []float64{7, 10, 15, 22})

	//   (2,3) @ (3,2):
	//   [[1 2 3][4 5 6]] @ [[7 8][9 10][11 12]] = [[58 64][139 154]]
	p := np.FromData([]float64{1, 2, 3, 4, 5, 6}, 2, 3)
	q := np.FromData([]float64{7, 8, 9, 10, 11, 12}, 3, 2)
	pq := p.MatMul(q)
	checkInts("MatMul (2,3)x(3,2) shape", pq.Shape(), []int{2, 2})
	checkSlice("MatMul (2,3)x(3,2) data", pq.Data(), []float64{58, 64, 139, 154})

	// Dot: 1-D dot product returns a scalar array; 2-D behaves as MatMul.
	dot := np.FromSlice([]float64{1, 2, 3}).Dot(np.FromSlice([]float64{4, 5, 6}))
	checkInt("Dot(1-D) size", dot.Size(), 1)
	checkFloat("Dot(1-D) value (=32)", dot.Data()[0], 32)
	checkSlice("Dot(2-D) == MatMul", m.Dot(m).Data(), m.MatMul(m).Data())

	// MatMul on a transposed (non-contiguous) operand.
	checkSlice("p.MatMul(p.T())", p.MatMul(p.T()).Data(), []float64{14, 32, 32, 77})
}

func reductions() {
	section("6. Reductions (whole array and along axes)")

	a := np.FromData([]float64{1, 2, 3, 4, 5, 6}, 2, 3)

	checkFloat("Sum", a.Sum(), 21)
	checkFloat("Prod", a.Prod(), 720)
	checkFloat("Mean", a.Mean(), 3.5)
	checkFloat("Max", a.Max(), 6)
	checkFloat("Min", a.Min(), 1)
	// Population variance of 1..6 = 35/12 = 2.9166..., std = 1.7078...
	checkFloat("Var (population)", a.Var(), 35.0/12.0)
	checkFloat("Std (population)", a.Std(), math.Sqrt(35.0/12.0))
	checkFloat("Ptp (max-min)", a.Ptp(), 5)
	// Frobenius norm of 1..6 = sqrt(91)
	checkFloat("Norm", a.Norm(), math.Sqrt(91))

	// Axis reductions: axis 0 collapses rows, axis 1 collapses columns.
	checkSlice("SumAxis(0)", a.SumAxis(0, false).Data(), []float64{5, 7, 9})
	checkInts("SumAxis(0) shape", a.SumAxis(0, false).Shape(), []int{3})
	checkSlice("SumAxis(1)", a.SumAxis(1, false).Data(), []float64{6, 15})
	checkInts("SumAxis(0,keepdims) shape", a.SumAxis(0, true).Shape(), []int{1, 3})
	checkInts("SumAxis(1,keepdims) shape", a.SumAxis(1, true).Shape(), []int{2, 1})
	checkSlice("MeanAxis(0)", a.MeanAxis(0, false).Data(), []float64{2.5, 3.5, 4.5})
	checkSlice("MeanAxis(1)", a.MeanAxis(1, false).Data(), []float64{2, 5})
	checkSlice("MaxAxis(0)", a.MaxAxis(0, false).Data(), []float64{4, 5, 6})
	checkSlice("MinAxis(1)", a.MinAxis(1, false).Data(), []float64{1, 4})
	checkSlice("ProdAxis(1)", a.ProdAxis(1, false).Data(), []float64{6, 120})
	// StdAxis(0): each column is {x, x+3}; population std = 1.5.
	checkSlice("StdAxis(0)", a.StdAxis(0, false).Data(), []float64{1.5, 1.5, 1.5})

	// Mean-centering a matrix via keepdims broadcasting (a classic NumPy idiom).
	centered := a.Sub(a.MeanAxis(1, true))
	checkSlice("a - MeanAxis(1,keepdims)", centered.Data(),
		[]float64{-1, 0, 1, -1, 0, 1})

	// 3-D axis reduction.
	cube := np.Arange(0, 24, 1).Reshape(2, 3, 4)
	checkInts("cube.SumAxis(1) shape", cube.SumAxis(1, false).Shape(), []int{2, 4})
	// Sum over axis 1 for the first plane: cols 0..3 of rows 0,4,8 etc.
	checkSlice("cube.SumAxis(1) data", cube.SumAxis(1, false).Data(),
		[]float64{12, 15, 18, 21, 48, 51, 54, 57})

	// Cumulative ops (flattened).
	checkSlice("Cumsum", np.FromSlice([]float64{1, 2, 3, 4}).Cumsum().Data(),
		[]float64{1, 3, 6, 10})
	checkSlice("Cumprod", np.FromSlice([]float64{1, 2, 3, 4}).Cumprod().Data(),
		[]float64{1, 2, 6, 24})
	checkSlice("Diff", np.FromSlice([]float64{1, 4, 9, 16}).Diff().Data(),
		[]float64{3, 5, 7})
}

func sortingAndStats() {
	section("7. Sorting, searching, statistics")

	v := np.FromSlice([]float64{5, 1, 4, 1, 3})
	checkSlice("Sort", v.Sort().Data(), []float64{1, 1, 3, 4, 5})
	checkSlice("Argsort", v.Argsort().Data(), []float64{1, 3, 4, 2, 0})
	checkInt("Argmax", v.Argmax(), 0)
	checkInt("Argmin", v.Argmin(), 1)
	checkSlice("Unique", v.Unique().Data(), []float64{1, 3, 4, 5})
	checkInt("SearchSorted(sorted,3.5)",
		np.FromSlice([]float64{1, 1, 3, 4, 5}).SearchSorted(3.5), 3)
	checkSlice("original untouched by Sort", v.Data(), []float64{5, 1, 4, 1, 3})

	// Median of an even-length sample: mean of the two middle values.
	even := np.FromSlice([]float64{4, 1, 3, 2})
	checkFloat("Median (even n)", even.Median(), 2.5)
	checkFloat("Median (odd n)", np.FromSlice([]float64{3, 1, 2}).Median(), 2)

	// Linear-interpolation percentiles over 1..5 (NumPy default method).
	p := np.FromSlice([]float64{1, 2, 3, 4, 5})
	checkFloat("Percentile(0)", p.Percentile(0), 1)
	checkFloat("Percentile(25)", p.Percentile(25), 2)
	checkFloat("Percentile(50)", p.Percentile(50), 3)
	checkFloat("Percentile(100)", p.Percentile(100), 5)
	// A value that requires interpolation: q=0.1 -> index 0.4 -> 1.4
	checkFloat("Quantile(0.1)", p.Quantile(0.1), 1.4)

	// ddof variants: sample variance of 1..5 is 2.5, population is 2.0.
	checkFloat("VarDDof(0)", p.VarDDof(0), 2)
	checkFloat("VarDDof(1)", p.VarDDof(1), 2.5)
	checkFloat("StdDDof(1)", p.StdDDof(1), math.Sqrt(2.5))
}

func maskingAndWhere() {
	section("8. Comparison, masking, Where")

	a := np.Arange(0, 6, 1).Reshape(2, 3)
	b := np.FromData([]float64{0, 9, 2, 9, 4, 9}, 2, 3)

	// Masks are 1.0/0.0 float arrays.
	checkSlice("Greater", a.Greater(b).Data(), []float64{0, 0, 0, 0, 0, 0})
	checkSlice("Less", a.Less(b).Data(), []float64{0, 1, 0, 1, 0, 1})
	checkSlice("EqualMask", a.EqualMask(b).Data(), []float64{1, 0, 1, 0, 1, 0})
	checkSlice("NotEqualMask", a.NotEqualMask(b).Data(), []float64{0, 1, 0, 1, 0, 1})
	checkSlice("GreaterEqual", a.GreaterEqual(b).Data(), []float64{1, 0, 1, 0, 1, 0})
	checkSlice("LessEqual", a.LessEqual(b).Data(), []float64{1, 1, 1, 1, 1, 1})
	checkSlice("GreaterScalar(2)", a.GreaterScalar(2).Data(), []float64{0, 0, 0, 1, 1, 1})
	checkSlice("LessScalar(2)", a.LessScalar(2).Data(), []float64{1, 1, 0, 0, 0, 0})
	checkSlice("EqualScalar(4)", a.EqualScalar(4).Data(), []float64{0, 0, 0, 0, 1, 0})

	// MaskSelect flattens to the selected elements.
	sel := a.MaskSelect(a.GreaterScalar(2))
	checkInts("MaskSelect shape", sel.Shape(), []int{3})
	checkSlice("MaskSelect(a>2)", sel.Data(), []float64{3, 4, 5})

	// Where picks element-wise between two arrays.
	w := np.Where(a.GreaterScalar(2), a.MulScalar(10), a.MulScalar(-1))
	checkInts("Where shape", w.Shape(), []int{2, 3})
	checkSlice("Where(a>2, 10a, -a)", w.Data(), []float64{0, -1, -2, 30, 40, 50})

	checkBool("Any(a>4)", a.GreaterScalar(4).Any(), true)
	checkBool("Any(a>10)", a.GreaterScalar(10).Any(), false)
	checkBool("All(a>=0)", a.GreaterEqual(np.Zeros(2, 3)).All(), true)
	checkBool("All(a>0)", a.GreaterScalar(0).All(), false)

	checkBool("Equal (exact)", a.Equal(a.Copy()), true)
	checkBool("AllClose (tolerant)", a.AllClose(a.AddScalar(1e-12), 1e-9), true)
	checkBool("AllClose rejects big delta", a.AllClose(a.AddScalar(1), 1e-9), false)
}

func linalg() {
	section("9. Linear algebra")

	m := np.FromData([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9}, 3, 3)
	checkFloat("Trace(3x3)", m.Trace(), 15) // 1+5+9
	checkSlice("Diagonal(0)", m.Diagonal(0).Data(), []float64{1, 5, 9})
	checkSlice("Diagonal(1)", m.Diagonal(1).Data(), []float64{2, 6})
	checkSlice("Diagonal(-1)", m.Diagonal(-1).Data(), []float64{4, 8})

	// Diag is overloaded like NumPy's: 1-D -> matrix, 2-D -> diagonal.
	dm := np.Diag(np.FromSlice([]float64{1, 2, 3}))
	checkInts("Diag(1-D) shape", dm.Shape(), []int{3, 3})
	checkSlice("Diag(1-D) data", dm.Data(), []float64{1, 0, 0, 0, 2, 0, 0, 0, 3})
	checkSlice("Diag(2-D) == diagonal", np.Diag(m).Data(), []float64{1, 5, 9})

	// Outer product of [1 2 3] and [4 5]:
	//   [[4 5][8 10][12 15]]
	out := np.FromSlice([]float64{1, 2, 3}).Outer(np.FromSlice([]float64{4, 5}))
	checkInts("Outer shape", out.Shape(), []int{3, 2})
	checkSlice("Outer data", out.Data(), []float64{4, 5, 8, 10, 12, 15})

	// Cross product of the basis vectors: x cross y = z.
	cross := np.FromSlice([]float64{1, 0, 0}).Cross(np.FromSlice([]float64{0, 1, 0}))
	checkSlice("Cross(x,y)", cross.Data(), []float64{0, 0, 1})
	// (1,2,3) x (4,5,6) = (-3, 6, -3)
	checkSlice("Cross([1 2 3],[4 5 6])",
		np.FromSlice([]float64{1, 2, 3}).Cross(np.FromSlice([]float64{4, 5, 6})).Data(),
		[]float64{-3, 6, -3})

	// Two 2-vectors: like NumPy, only the z component is returned, here as a
	// 0-dimensional array. (1,2) x (3,4) = 1*4 - 2*3 = -2.
	z := np.FromSlice([]float64{1, 2}).Cross(np.FromSlice([]float64{3, 4}))
	checkInt("Cross(2-vec) ndim", z.Ndim(), 0)
	checkFloat("Cross([1 2],[3 4])", z.Data()[0], -2)

	checkFloat("Norm([3 4])", np.FromSlice([]float64{3, 4}).Norm(), 5)

	// Identity is the multiplicative identity for MatMul.
	checkSlice("m @ I == m", m.MatMul(np.Identity(3)).Data(), m.Data())

	// HOLE: there is no matrix inverse, determinant, solver, eigen or SVD
	// routine anywhere in the package (grep for Inv/Det/Solve/Eig/SVD/QR/
	// Cholesky finds nothing). NumPy's numpy.linalg core is therefore absent;
	// the following cannot be written:
	//   inv := m.Inv()
	//   d   := m.Det()
	//   x   := np.Solve(m, b)
	fmt.Println("  (no Inv/Det/Solve/Eig/SVD in this port — see README holes)")
}

func dtypes() {
	section("10. Dtypes")

	// HOLE: the port has exactly one element type, float64. There is no Dtype
	// type, no AsType/Astype conversion, and no integer/bool/complex arrays;
	// NDArray.Data() is []float64 unconditionally. Boolean results are encoded
	// as 1.0/0.0 float64 masks and integer index results come back as float64
	// (Argsort) or plain int (Argmax/Argmin) rather than an integer array.
	// So none of these compile:
	//   a := np.Zeros(2, 2).AsType(np.Int32)
	//   fmt.Println(a.Dtype())
	mask := np.Arange(0, 4, 1).GreaterScalar(1)
	fmt.Printf("  only dtype is float64; masks are floats: %v\n", mask.Data())
	idx := np.FromSlice([]float64{3, 1, 2}).Argsort()
	fmt.Printf("  Argsort returns float64 indices:        %v\n", idx.Data())
	fmt.Printf("  Argmax returns a Go int:                %d\n",
		np.FromSlice([]float64{3, 1, 2}).Argmax())
}

func errorBehavior() {
	section("11. Panic-based error reporting")

	cases := []struct {
		name string
		fn   func()
	}{
		{"Reshape to wrong size", func() { np.Arange(0, 6, 1).Reshape(4, 2) }},
		{"MatMul dim mismatch", func() { np.Zeros(2, 3).MatMul(np.Zeros(2, 3)) }},
		{"At with too few indices", func() { np.Zeros(2, 2).At(1) }},
		{"Trace on 1-D", func() { np.FromSlice([]float64{1, 2}).Trace() }},
		{"Cross on length-4 vectors", func() {
			np.FromSlice([]float64{1, 2, 3, 4}).Cross(np.FromSlice([]float64{1, 2, 3, 4}))
		}},
		{"Slice with wrong range count", func() { np.Zeros(2, 2).Slice(np.R(0, 1)) }},
	}
	for _, c := range cases {
		msg := capturePanic(c.fn)
		checkBool(c.name+" panics", msg != "", true)
		if msg != "" {
			checkBool("  message prefixed \"numpy:\"", strings.HasPrefix(msg, "numpy:"), true)
			fmt.Printf("      %s\n", msg)
		}
	}
}

// capturePanic runs fn and returns the panic value formatted as a string, or ""
// if fn returned normally.
func capturePanic(fn func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprint(r)
		}
	}()
	fn()
	return ""
}
