// Command run is the Go-side parity runner for github.com/malcolmston/numpy.
//
// It speaks JSON Lines on stdio: one request object per line in, one response
// object per line out, in the same order. See ../../HARNESS.md.
//
// Value encoding is identical to python/run.py:
//
//	*NDArray -> {"kind":"array","shape":[...],"data":<nested lists>}
//	float64  -> {"kind":"scalar","value":<number|sentinel>}
//	int      -> {"kind":"int","value":<int>}
//	bool     -> {"kind":"bool","value":<bool>}
//
// Non-finite float64 values are encoded as the sentinel strings "NaN",
// "Infinity" and "-Infinity" because JSON has no literal for them.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"

	np "github.com/malcolmston/numpy"
)

// ------------------------------------------------------------------ decoding

func decNum(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		switch t {
		case "NaN":
			return math.NaN()
		case "Infinity":
			return math.Inf(1)
		case "-Infinity":
			return math.Inf(-1)
		}
		panic("unknown numeric sentinel: " + t)
	case bool:
		panic("expected number, got bool")
	}
	panic(fmt.Sprintf("expected number, got %T", v))
}

// nestedShape walks a nested []any and returns its rectangular shape.
func nestedShape(v any) []int {
	var out []int
	for {
		l, ok := v.([]any)
		if !ok {
			return out
		}
		out = append(out, len(l))
		if len(l) == 0 {
			return out
		}
		v = l[0]
	}
}

func flatten(v any, dst *[]float64) {
	if l, ok := v.([]any); ok {
		for _, e := range l {
			flatten(e, dst)
		}
		return
	}
	*dst = append(*dst, decNum(v))
}

func decArray(v any) *np.NDArray {
	shape := nestedShape(v)
	var data []float64
	flatten(v, &data)
	if len(shape) == 0 {
		// scalar literal: a length-1 1-D array is the closest the port has
		return np.FromSlice(data)
	}
	return np.FromData(data, shape...)
}

// ------------------------------------------------------------------ encoding

func encNum(x float64) any {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	return x
}

// encNested builds nested JSON lists from row-major data and a shape.
func encNested(data []float64, shape []int) any {
	if len(shape) == 0 {
		return encNum(data[0])
	}
	if len(shape) == 1 {
		out := make([]any, shape[0])
		for i := range out {
			out[i] = encNum(data[i])
		}
		return out
	}
	stride := 1
	for _, d := range shape[1:] {
		stride *= d
	}
	out := make([]any, shape[0])
	for i := 0; i < shape[0]; i++ {
		out[i] = encNested(data[i*stride:(i+1)*stride], shape[1:])
	}
	return out
}

func enc(v any) map[string]any {
	switch t := v.(type) {
	case *np.NDArray:
		sh := t.Shape()
		if sh == nil {
			sh = []int{}
		}
		return map[string]any{"kind": "array", "shape": sh, "data": encNested(t.Data(), sh)}
	case bool:
		return map[string]any{"kind": "bool", "value": t}
	case int:
		return map[string]any{"kind": "int", "value": t}
	case float64:
		return map[string]any{"kind": "scalar", "value": encNum(t)}
	}
	panic(fmt.Sprintf("cannot encode %T", v))
}

// ------------------------------------------------------------------ args

type args []any

func (a args) at(i int) any {
	if i >= len(a) {
		panic(fmt.Sprintf("missing argument %d", i))
	}
	return a[i]
}

func (a args) arr(i int) *np.NDArray {
	m, ok := a.at(i).(map[string]any)
	if !ok {
		panic(fmt.Sprintf("argument %d is not an array", i))
	}
	return decArray(m["arr"])
}

func (a args) f(i int) float64 { return decNum(a.at(i)) }

func (a args) i(i int) int {
	f := decNum(a.at(i))
	return int(f)
}

func (a args) b(i int) bool {
	v, ok := a.at(i).(bool)
	if !ok {
		panic(fmt.Sprintf("argument %d is not a bool", i))
	}
	return v
}

func (a args) ints(i int) []int {
	l, ok := a.at(i).([]any)
	if !ok {
		panic(fmt.Sprintf("argument %d is not a list", i))
	}
	out := make([]int, len(l))
	for k, v := range l {
		out[k] = int(decNum(v))
	}
	return out
}

func (a args) ranges(i int, ndim int) []np.Range {
	l, ok := a.at(i).([]any)
	if !ok {
		panic(fmt.Sprintf("argument %d is not a list", i))
	}
	if len(l) != ndim {
		panic(fmt.Sprintf("slice: expected %d ranges, got %d", ndim, len(l)))
	}
	out := make([]np.Range, len(l))
	for k, v := range l {
		p, ok := v.([]any)
		if !ok || len(p) != 2 {
			panic("slice: each range must be a [start, stop] pair")
		}
		out[k] = np.R(int(decNum(p[0])), int(decNum(p[1])))
	}
	return out
}

// rest returns the array arguments from index i onwards (for concatenate/stack).
func (a args) rest(i int) []*np.NDArray {
	var out []*np.NDArray
	for k := i; k < len(a); k++ {
		out = append(out, a.arr(k))
	}
	return out
}

func shapeArray(a *np.NDArray) *np.NDArray {
	sh := a.Shape()
	d := make([]float64, len(sh))
	for i, v := range sh {
		d[i] = float64(v)
	}
	return np.FromSlice(d)
}

// ------------------------------------------------------------------ dispatch

var ops = map[string]func(args) any{
	// ---- creation
	"zeros":       func(a args) any { return np.Zeros(a.ints(0)...) },
	"ones":        func(a args) any { return np.Ones(a.ints(0)...) },
	"full":        func(a args) any { return np.Full(a.f(1), a.ints(0)...) },
	"full_scalar": func(a args) any { return np.Full(a.f(1), a.ints(0)...) },
	"arange":      func(a args) any { return np.Arange(a.f(0), a.f(1), a.f(2)) },
	"linspace":    func(a args) any { return np.Linspace(a.f(0), a.f(1), a.i(2), a.b(3)) },
	"eye":         func(a args) any { return np.Eye(a.i(0), a.i(1), a.i(2)) },
	"identity":    func(a args) any { return np.Identity(a.i(0)) },
	"zeros_like":  func(a args) any { return np.ZerosLike(a.arr(0)) },
	"ones_like":   func(a args) any { return np.OnesLike(a.arr(0)) },
	"from_nested": func(a args) any { return a.arr(0) },

	// ---- shape
	"reshape":        func(a args) any { return a.arr(0).Reshape(a.ints(1)...) },
	"transpose":      func(a args) any { return a.arr(0).Transpose() },
	"transpose_axes": func(a args) any { return a.arr(0).Transpose(a.ints(1)...) },
	"ravel":          func(a args) any { return a.arr(0).Ravel() },
	"flatten":        func(a args) any { return a.arr(0).Flatten() },
	"squeeze":        func(a args) any { return a.arr(0).Squeeze() },
	"expand_dims":    func(a args) any { return a.arr(0).ExpandDims(a.i(1)) },
	"concatenate":    func(a args) any { return np.Concatenate(a.i(0), a.rest(1)...) },
	"stack":          func(a args) any { return np.Stack(a.i(0), a.rest(1)...) },
	"flip":           func(a args) any { return a.arr(0).Flip(a.i(1)) },
	"roll":           func(a args) any { return a.arr(0).Roll(a.i(1)) },
	"diag":           func(a args) any { return np.Diag(a.arr(0)) },
	"diagonal":       func(a args) any { return a.arr(0).Diagonal(a.i(1)) },
	"shape":          func(a args) any { return shapeArray(a.arr(0)) },
	"ndim":           func(a args) any { return a.arr(0).Ndim() },
	"size":           func(a args) any { return a.arr(0).Size() },

	// ---- indexing / slicing
	"slice": func(a args) any {
		arr := a.arr(0)
		return arr.Slice(a.ranges(1, arr.Ndim())...).Copy()
	},
	"at": func(a args) any { return a.arr(0).At(a.ints(1)...) },

	// ---- broadcasting
	"broadcast_to": func(a args) any { return a.arr(0).BroadcastTo(a.ints(1)...) },

	// ---- elementwise arithmetic
	"add":        func(a args) any { return a.arr(0).Add(a.arr(1)) },
	"sub":        func(a args) any { return a.arr(0).Sub(a.arr(1)) },
	"mul":        func(a args) any { return a.arr(0).Mul(a.arr(1)) },
	"div":        func(a args) any { return a.arr(0).Div(a.arr(1)) },
	"pow":        func(a args) any { return a.arr(0).Pow(a.arr(1)) },
	"mod":        func(a args) any { return a.arr(0).Mod(a.arr(1)) },
	"add_scalar": func(a args) any { return a.arr(0).AddScalar(a.f(1)) },
	"sub_scalar": func(a args) any { return a.arr(0).SubScalar(a.f(1)) },
	"mul_scalar": func(a args) any { return a.arr(0).MulScalar(a.f(1)) },
	"div_scalar": func(a args) any { return a.arr(0).DivScalar(a.f(1)) },
	"pow_scalar": func(a args) any { return a.arr(0).PowScalar(a.f(1)) },

	// ---- unary ufuncs
	"neg":        func(a args) any { return a.arr(0).Neg() },
	"abs":        func(a args) any { return a.arr(0).Abs() },
	"sqrt":       func(a args) any { return a.arr(0).Sqrt() },
	"cbrt":       func(a args) any { return a.arr(0).Cbrt() },
	"square":     func(a args) any { return a.arr(0).Square() },
	"reciprocal": func(a args) any { return a.arr(0).Reciprocal() },
	"exp":        func(a args) any { return a.arr(0).Exp() },
	"expm1":      func(a args) any { return a.arr(0).Expm1() },
	"log":        func(a args) any { return a.arr(0).Log() },
	"log1p":      func(a args) any { return a.arr(0).Log1p() },
	"log2":       func(a args) any { return a.arr(0).Log2() },
	"log10":      func(a args) any { return a.arr(0).Log10() },
	"sin":        func(a args) any { return a.arr(0).Sin() },
	"cos":        func(a args) any { return a.arr(0).Cos() },
	"tan":        func(a args) any { return a.arr(0).Tan() },
	"arcsin":     func(a args) any { return a.arr(0).Arcsin() },
	"arccos":     func(a args) any { return a.arr(0).Arccos() },
	"arctan":     func(a args) any { return a.arr(0).Arctan() },
	"sinh":       func(a args) any { return a.arr(0).Sinh() },
	"cosh":       func(a args) any { return a.arr(0).Cosh() },
	"tanh":       func(a args) any { return a.arr(0).Tanh() },
	"floor":      func(a args) any { return a.arr(0).Floor() },
	"ceil":       func(a args) any { return a.arr(0).Ceil() },
	"trunc":      func(a args) any { return a.arr(0).Trunc() },
	"round":      func(a args) any { return a.arr(0).Round() },
	"sign":       func(a args) any { return a.arr(0).Sign() },
	"clip":       func(a args) any { return a.arr(0).Clip(a.f(1), a.f(2)) },

	// ---- binary ufuncs
	"maximum": func(a args) any { return a.arr(0).Maximum(a.arr(1)) },
	"minimum": func(a args) any { return a.arr(0).Minimum(a.arr(1)) },
	"hypot":   func(a args) any { return a.arr(0).Hypot(a.arr(1)) },
	"arctan2": func(a args) any { return a.arr(0).Arctan2(a.arr(1)) },

	// ---- linalg
	"matmul": func(a args) any { return a.arr(0).MatMul(a.arr(1)) },
	"dot":    func(a args) any { return a.arr(0).Dot(a.arr(1)) },
	"outer":  func(a args) any { return a.arr(0).Outer(a.arr(1)) },
	"trace":  func(a args) any { return a.arr(0).Trace() },
	"norm":   func(a args) any { return a.arr(0).Norm() },
	"cross":  func(a args) any { return a.arr(0).Cross(a.arr(1)) },

	// ---- reductions
	"sum":       func(a args) any { return a.arr(0).Sum() },
	"mean":      func(a args) any { return a.arr(0).Mean() },
	"max":       func(a args) any { return a.arr(0).Max() },
	"min":       func(a args) any { return a.arr(0).Min() },
	"prod":      func(a args) any { return a.arr(0).Prod() },
	"ptp":       func(a args) any { return a.arr(0).Ptp() },
	"all":       func(a args) any { return a.arr(0).All() },
	"any":       func(a args) any { return a.arr(0).Any() },
	"sum_axis":  func(a args) any { return a.arr(0).SumAxis(a.i(1), a.b(2)) },
	"mean_axis": func(a args) any { return a.arr(0).MeanAxis(a.i(1), a.b(2)) },
	"max_axis":  func(a args) any { return a.arr(0).MaxAxis(a.i(1), a.b(2)) },
	"min_axis":  func(a args) any { return a.arr(0).MinAxis(a.i(1), a.b(2)) },
	"prod_axis": func(a args) any { return a.arr(0).ProdAxis(a.i(1), a.b(2)) },
	"std_axis":  func(a args) any { return a.arr(0).StdAxis(a.i(1), a.b(2)) },

	// ---- sorting / searching
	"sort":         func(a args) any { return a.arr(0).Sort() },
	"argsort":      func(a args) any { return a.arr(0).Argsort() },
	"argmax":       func(a args) any { return a.arr(0).Argmax() },
	"argmin":       func(a args) any { return a.arr(0).Argmin() },
	"unique":       func(a args) any { return a.arr(0).Unique() },
	"searchsorted": func(a args) any { return a.arr(0).SearchSorted(a.f(1)) },
	"where":        func(a args) any { return np.Where(a.arr(0), a.arr(1), a.arr(2)) },
	"mask_select":  func(a args) any { return a.arr(0).MaskSelect(a.arr(1)) },

	"greater":        func(a args) any { return a.arr(0).Greater(a.arr(1)) },
	"greater_equal":  func(a args) any { return a.arr(0).GreaterEqual(a.arr(1)) },
	"less":           func(a args) any { return a.arr(0).Less(a.arr(1)) },
	"less_equal":     func(a args) any { return a.arr(0).LessEqual(a.arr(1)) },
	"equal_mask":     func(a args) any { return a.arr(0).EqualMask(a.arr(1)) },
	"not_equal_mask": func(a args) any { return a.arr(0).NotEqualMask(a.arr(1)) },
	"greater_scalar": func(a args) any { return a.arr(0).GreaterScalar(a.f(1)) },
	"less_scalar":    func(a args) any { return a.arr(0).LessScalar(a.f(1)) },
	"equal_scalar":   func(a args) any { return a.arr(0).EqualScalar(a.f(1)) },
	"array_equal":    func(a args) any { return a.arr(0).Equal(a.arr(1)) },
	"allclose":       func(a args) any { return a.arr(0).AllClose(a.arr(1), a.f(2)) },

	// ---- statistics
	"std":        func(a args) any { return a.arr(0).Std() },
	"var":        func(a args) any { return a.arr(0).Var() },
	"std_ddof":   func(a args) any { return a.arr(0).StdDDof(a.i(1)) },
	"var_ddof":   func(a args) any { return a.arr(0).VarDDof(a.i(1)) },
	"median":     func(a args) any { return a.arr(0).Median() },
	"percentile": func(a args) any { return a.arr(0).Percentile(a.f(1)) },
	"quantile":   func(a args) any { return a.arr(0).Quantile(a.f(1)) },

	// ---- cumulative
	"cumsum":  func(a args) any { return a.arr(0).Cumsum() },
	"cumprod": func(a args) any { return a.arr(0).Cumprod() },
	"diff":    func(a args) any { return a.arr(0).Diff() },
}

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

// call runs one op, converting any panic into an error. The port panics on all
// shape/dimension validation failures, so this is the error channel.
func call(req request) (value map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()
	op, ok := ops[req.Fn]
	if !ok {
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
	return enc(op(args(req.Args))), nil
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			emit(out, map[string]any{"id": "?", "ok": false, "error": err.Error()})
			continue
		}
		v, err := call(req)
		if err != nil {
			emit(out, map[string]any{"id": req.ID, "ok": false, "error": err.Error()})
			continue
		}
		emit(out, map[string]any{"id": req.ID, "ok": true, "value": v})
	}
}

func emit(w *bufio.Writer, m map[string]any) {
	b, err := json.Marshal(m)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"id": m["id"], "ok": false, "error": err.Error()})
	}
	w.Write(b)
	w.WriteByte('\n')
	w.Flush()
}
