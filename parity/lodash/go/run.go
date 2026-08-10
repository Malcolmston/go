// Command run is the Go-side runner for the lodash parity harness.
//
// Protocol: one JSON object per line on stdin -> exactly one JSON object per
// line on stdout, in the same order.
//
//	in : {"id":"...","fn":"chunk","args":[[1,2,3],2]}
//	out: {"id":"...","ok":true,"value":[[1,2],[3]]}
//	out: {"id":"...","ok":false,"error":"..."}
//
// A panicking case is caught and reported as ok:false; the runner never exits
// early. Nothing but protocol lines is written to stdout.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	lo "github.com/malcolmston/lodash"
)

// ---------------------------------------------------------------------------
// value model
// ---------------------------------------------------------------------------

// undef stands for JavaScript `undefined`, which Go has no value for. Adapters
// return it where lodash returns undefined (a missing element, a failed lookup)
// so the two sides can be compared.
type undef struct{}

type fnRef struct{ name string }

func resolve(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = resolve(e)
		}
		return out
	case map[string]any:
		if n, ok := t["$fn"]; ok {
			return fnRef{name: fmt.Sprint(n)}
		}
		if _, ok := t["$undefined"]; ok {
			return undef{}
		}
		if _, ok := t["$nan"]; ok {
			return math.NaN()
		}
		if s, ok := t["$inf"]; ok {
			if toF(s) > 0 {
				return math.Inf(1)
			}
			return math.Inf(-1)
		}
		if s, ok := t["$regexp"]; ok {
			re, err := regexp.Compile(fmt.Sprint(s))
			if err != nil {
				panic(err)
			}
			return re
		}
		if s, ok := t["$date"]; ok {
			ts, err := time.Parse(time.RFC3339, fmt.Sprint(s))
			if err != nil {
				panic(err)
			}
			return ts
		}
		if n, ok := t["$sparse"]; ok {
			arr := make([]any, int(toF(n)))
			if set, ok := t["set"].(map[string]any); ok {
				for k, e := range set {
					var i int
					fmt.Sscanf(k, "%d", &i)
					if i >= 0 && i < len(arr) {
						arr[i] = resolve(e)
					}
				}
			}
			return arr
		}
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = resolve(e)
		}
		return out
	default:
		return v
	}
}

// normalise renders a Go value as the JSON-comparable form the node runner also
// emits. encoding/json already sorts map[string]any keys, so object key order
// needs no extra work.
func normalise(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case undef:
		return map[string]any{"$undefined": true}
	case *regexp.Regexp:
		return map[string]any{"$regexp": t.String()}
	case time.Time:
		return map[string]any{"$date": t.UTC().Format("2006-01-02T15:04:05.000Z")}
	case fnRef:
		return map[string]any{"$function": true}
	case float64:
		if math.IsNaN(t) {
			return map[string]any{"$nan": true}
		}
		if math.IsInf(t, 1) {
			return map[string]any{"$inf": float64(1)}
		}
		if math.IsInf(t, -1) {
			return map[string]any{"$inf": float64(-1)}
		}
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case float32:
		return float64(t)
	case bool, string:
		return t
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalise(e)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = e
		}
		return out
	case []float64:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalise(e)
		}
		return out
	case []int:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = float64(e)
		}
		return out
	case [][]any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalise(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalise(e)
		}
		return out
	case map[string]int:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = float64(e)
		}
		return out
	case map[string][]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalise(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[keyString(k)] = normalise(e)
		}
		return out
	case map[any][]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[keyString(k)] = normalise(e)
		}
		return out
	case map[float64]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[keyString(k)] = normalise(e)
		}
		return out
	case error:
		return map[string]any{"$errorValue": true}
	default:
		// last resort: round-trip through JSON so structs become objects
		b, err := json.Marshal(t)
		if err != nil {
			panic(fmt.Sprintf("cannot normalise %T", t))
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			panic(err)
		}
		return out
	}
}

// keyString renders a map key the way JavaScript object keys appear: always a
// string, numbers without a trailing ".0".
func keyString(k any) string {
	switch t := k.(type) {
	case string:
		return t
	case float64:
		return jsNumString(t)
	case int:
		return fmt.Sprint(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return fmt.Sprint(t)
	}
}

func jsNumString(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		return fmt.Sprintf("%d", int64(f))
	}
	b, _ := json.Marshal(f)
	return string(b)
}

// ---------------------------------------------------------------------------
// coercions
// ---------------------------------------------------------------------------

func toF(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case nil:
		return 0
	case string:
		return lo.ToNumber(t)
	default:
		return math.NaN()
	}
}

func toI(v any) int {
	f := toF(v)
	if math.IsNaN(f) {
		return 0
	}
	return int(f)
}

func toS(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case undef:
		return ""
	case float64:
		return jsNumString(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(t)
	}
}

// jsStr mirrors JavaScript's String(), which spells nil "null" and a missing
// value "undefined" -- unlike lodash's own toString, which yields "".
func jsStr(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case undef:
		return "undefined"
	default:
		return toS(v)
	}
}

func toArr(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case undef:
		return nil
	case []any:
		return t
	default:
		panic(fmt.Sprintf("expected array, got %T", v))
	}
}

func toObj(v any) map[string]any {
	switch t := v.(type) {
	case nil:
		return nil
	case undef:
		return nil
	case map[string]any:
		return t
	default:
		panic(fmt.Sprintf("expected object, got %T", v))
	}
}

func toStrs(v any) []string {
	a := toArr(v)
	out := make([]string, len(a))
	for i, e := range a {
		out[i] = toS(e)
	}
	return out
}

func toFloats(v any) []float64 {
	a := toArr(v)
	out := make([]float64, len(a))
	for i, e := range a {
		out[i] = toF(e)
	}
	return out
}

func toInts(v any) []int {
	a := toArr(v)
	out := make([]int, len(a))
	for i, e := range a {
		out[i] = toI(e)
	}
	return out
}

// truthy mirrors JavaScript truthiness so that predicates and Compact agree.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil, undef:
		return false
	case bool:
		return t
	case float64:
		return t != 0 && !math.IsNaN(t)
	case int:
		return t != 0
	case string:
		return t != ""
	default:
		return true
	}
}

// ---------------------------------------------------------------------------
// named function registry -- mirrors node/run.js FNS. Each entry is a factory
// so every case gets a fresh (possibly stateful) closure.
// ---------------------------------------------------------------------------

var zeroFns = map[string]func() func() any{
	"counter": func() func() any { c := 0.0; return func() any { c++; return c } },
	"const42": func() func() any { return func() any { return float64(42) } },
}

var predFns = map[string]func() func(any) bool{
	"isEven": func() func(any) bool { return func(v any) bool { return math.Mod(toF(v), 2) == 0 } },
	"isOdd":  func() func(any) bool { return func(v any) bool { return math.Abs(math.Mod(toF(v), 2)) == 1 } },
	"gt2":    func() func(any) bool { return func(v any) bool { return toF(v) > 2 } },
	"lt3":    func() func(any) bool { return func(v any) bool { return toF(v) < 3 } },
	"truthy": func() func(any) bool { return truthy },
	"isNeg":  func() func(any) bool { return func(v any) bool { return toF(v) < 0 } },
	"eqA":    func() func(any) bool { return func(v any) bool { s, ok := v.(string); return ok && s == "a" } },
	"hasFlag": func() func(any) bool {
		return func(v any) bool { m, ok := v.(map[string]any); return ok && truthy(m["flag"]) }
	},
	"isNumType":   func() func(any) bool { return func(v any) bool { _, ok := v.(float64); return ok } },
	"alwaysTrue":  func() func(any) bool { return func(any) bool { return true } },
	"alwaysFalse": func() func(any) bool { return func(any) bool { return false } },
	"isEmptyStr":  func() func(any) bool { return func(v any) bool { s, ok := v.(string); return ok && s == "" } },
}

func prop(key string) func(any) any {
	return func(v any) any {
		m, ok := v.(map[string]any)
		if !ok {
			return undef{}
		}
		x, ok := m[key]
		if !ok {
			return undef{}
		}
		return x
	}
}

var unFns = map[string]func() func(any) any{
	"identity": func() func(any) any { return func(v any) any { return v } },
	"double":   func() func(any) any { return func(v any) any { return toF(v) * 2 } },
	"square":   func() func(any) any { return func(v any) any { return toF(v) * toF(v) } },
	"mod3":     func() func(any) any { return func(v any) any { return math.Mod(toF(v), 3) } },
	"absv":     func() func(any) any { return func(v any) any { return math.Abs(toF(v)) } },
	"floorv":   func() func(any) any { return func(v any) any { return math.Floor(toF(v)) } },
	"lenOf":    func() func(any) any { return func(v any) any { return lengthOf(v) } },
	"numOf":    func() func(any) any { return func(v any) any { return toF(v) } },
	"toStr":    func() func(any) any { return func(v any) any { return jsStr(v) } },
	"upper":    func() func(any) any { return func(v any) any { return strings.ToUpper(jsStr(v)) } },
	"firstChar": func() func(any) any {
		return func(v any) any {
			s := jsStr(v)
			if s == "" {
				return ""
			}
			return s[:1]
		}
	},
	"propN":    func() func(any) any { return prop("n") },
	"propAge":  func() func(any) any { return prop("age") },
	"propName": func() func(any) any { return prop("name") },
	"wrapArr":  func() func(any) any { return func(v any) any { return []any{v} } },
	"dupArr":   func() func(any) any { return func(v any) any { return []any{v, v} } },
}

func lengthOf(v any) float64 {
	switch t := v.(type) {
	case string:
		return float64(len(t))
	case []any:
		return float64(len(t))
	default:
		return 0
	}
}

var numKeyNames = map[string]bool{
	"double": true, "square": true, "mod3": true, "absv": true, "floorv": true,
	"lenOf": true, "numOf": true, "propN": true, "propAge": true, "identity": true,
}

var strKeyNames = map[string]bool{
	"toStr": true, "upper": true, "firstChar": true, "propName": true,
}

var binFns = map[string]func() func(any, any) any{
	"add2":    func() func(any, any) any { return func(a, b any) any { return toF(a) + toF(b) } },
	"sub2":    func() func(any, any) any { return func(a, b any) any { return toF(a) - toF(b) } },
	"mulPair": func() func(any, any) any { return func(a, b any) any { return toF(a) * toF(b) } },
	"concat2": func() func(any, any) any { return func(a, b any) any { return toS(a) + toS(b) } },
}

var eqFns = map[string]func() func(any, any) bool{
	"eqStrict": func() func(any, any) bool { return func(x, y any) bool { return x == y } },
	"eqNum":    func() func(any, any) bool { return func(x, y any) bool { return toF(x) == toF(y) } },
	"eqMod3": func() func(any, any) bool {
		return func(x, y any) bool { return math.Mod(toF(x), 3) == math.Mod(toF(y), 3) }
	},
}

var idxFns = map[string]func() func(any, int) any{
	"mulIdx":  func() func(any, int) any { return func(v any, i int) any { return toF(v) * float64(i) } },
	"pairIdx": func() func(any, int) any { return func(v any, i int) any { return []any{v, float64(i)} } },
}

func refName(v any) string {
	r, ok := v.(fnRef)
	if !ok {
		panic(fmt.Sprintf("expected $fn reference, got %T", v))
	}
	return r.name
}

func predOf(v any) func(any) bool {
	n := refName(v)
	if f, ok := predFns[n]; ok {
		return f()
	}
	if f, ok := unFns[n]; ok {
		g := f()
		return func(x any) bool { return truthy(g(x)) }
	}
	panic("unknown predicate $fn: " + n)
}

func unOf(v any) func(any) any {
	n := refName(v)
	if f, ok := unFns[n]; ok {
		return f()
	}
	if f, ok := predFns[n]; ok {
		g := f()
		return func(x any) any { return g(x) }
	}
	panic("unknown unary $fn: " + n)
}

func numKeyOf(v any) func(any) float64 {
	g := unOf(v)
	return func(x any) float64 { return toF(g(x)) }
}

func strKeyOf(v any) func(any) string {
	g := unOf(v)
	return func(x any) string { return toS(g(x)) }
}

// anyKeyOf produces a comparable key usable as a Go map key.
func anyKeyOf(v any) func(any) any {
	g := unOf(v)
	return func(x any) any {
		r := g(x)
		switch r.(type) {
		case string, float64, bool, nil:
			return r
		case undef:
			return "undefined"
		default:
			b, _ := json.Marshal(normalise(r))
			return string(b)
		}
	}
}

// strGroupOf keys a group by its JS-style string form, matching the way
// JavaScript object keys stringify.
func strGroupOf(v any) func(any) string {
	g := unOf(v)
	return func(x any) string {
		r := g(x)
		if _, ok := r.(undef); ok {
			return "undefined"
		}
		if r == nil {
			return "null"
		}
		return toS(r)
	}
}

func binOf(v any) func(any, any) any {
	n := refName(v)
	if f, ok := binFns[n]; ok {
		return f()
	}
	panic("unknown binary $fn: " + n)
}

func eqOf(v any) func(any, any) bool {
	n := refName(v)
	if f, ok := eqFns[n]; ok {
		return f()
	}
	panic("unknown comparator $fn: " + n)
}

func idxOf(v any) func(any, int) any {
	n := refName(v)
	if f, ok := idxFns[n]; ok {
		return f()
	}
	panic("unknown indexed $fn: " + n)
}

func zeroOf(v any) func() any {
	n := refName(v)
	if f, ok := zeroFns[n]; ok {
		return f()
	}
	panic("unknown nullary $fn: " + n)
}

// ---------------------------------------------------------------------------
// helpers shared by adapters
// ---------------------------------------------------------------------------

func optOK(v any, ok bool) any {
	if !ok {
		return undef{}
	}
	return v
}

func restArrs(args []any, from int) [][]any {
	out := make([][]any, 0, len(args)-from)
	for _, a := range args[from:] {
		out = append(out, toArr(a))
	}
	return out
}

func sortNormed(v []any) []any {
	type kv struct {
		k string
		v any
	}
	tmp := make([]kv, len(v))
	for i, e := range v {
		b, _ := json.Marshal(e)
		tmp[i] = kv{string(b), e}
	}
	sort.SliceStable(tmp, func(i, j int) bool { return tmp[i].k < tmp[j].k })
	out := make([]any, len(v))
	for i, e := range tmp {
		out[i] = e.v
	}
	return out
}

// pairsOf converts [[a,b],...] into []lo.Pair[any, any].
func pairsOf(v any) []lo.Pair[any, any] {
	a := toArr(v)
	out := make([]lo.Pair[any, any], 0, len(a))
	for _, e := range a {
		p := toArr(e)
		var first, second any
		if len(p) > 0 {
			first = p[0]
		}
		if len(p) > 1 {
			second = p[1]
		}
		out = append(out, lo.Pair[any, any]{First: first, Second: second})
	}
	return out
}

func pairSlice[A, B any](ps []lo.Pair[A, B]) []any {
	out := make([]any, len(ps))
	for i, p := range ps {
		out[i] = []any{p.First, p.Second}
	}
	return out
}

func entrySlice(es []lo.Entry[string, any]) []any {
	out := make([]any, len(es))
	for i, e := range es {
		out[i] = []any{e.Key, e.Value}
	}
	return out
}

func callsOf(v any) [][]any {
	a := toArr(v)
	out := make([][]any, len(a))
	for i, e := range a {
		out[i] = toArr(e)
	}
	return out
}

func numsSorted(a []any) ([]float64, bool) {
	out := make([]float64, len(a))
	for i, e := range a {
		f, ok := e.(float64)
		if !ok {
			return nil, false
		}
		out[i] = f
	}
	return out, true
}

func allStrings(a []any) ([]string, bool) {
	out := make([]string, len(a))
	for i, e := range a {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

// ---------------------------------------------------------------------------
// ops
// ---------------------------------------------------------------------------

// sortAlways mirrors node/run.js SORT_ALWAYS: results whose element order is
// meaningless on the Go side (map iteration is randomised) are sorted on both
// sides before comparison.
var sortAlways = map[string]bool{
	"keys": true, "values": true, "toPairs": true, "entries": true,
	"sortedKeys": true, "mapToSlice": true, "filterMap": true,
	"rejectMap": true, "forOwn-collect": true,
}

var ops map[string]func([]any) any

func init() {
	ops = map[string]func([]any) any{
		// ---------------- array ----------------
		"chunk":   func(a []any) any { return toAnySlices(lo.Chunk(toArr(a[0]), toI(a[1]))) },
		"compact": func(a []any) any { return lo.Compact(toArr(a[0])) },
		"concat":  func(a []any) any { return lo.Concat(restArrs(a, 0)...) },
		"difference": func(a []any) any {
			return lo.Difference(toArr(a[0]), restArrs(a, 1)...)
		},
		"differenceBy": func(a []any) any {
			return lo.DifferenceBy(anyKeyOf(a[len(a)-1]), toArr(a[0]), restArrs(a[:len(a)-1], 1)...)
		},
		"drop":           func(a []any) any { return lo.Drop(toArr(a[0]), toI(a[1])) },
		"dropRight":      func(a []any) any { return lo.DropRight(toArr(a[0]), toI(a[1])) },
		"dropWhile":      func(a []any) any { return lo.DropWhile(toArr(a[0]), predOf(a[1])) },
		"dropRightWhile": func(a []any) any { return lo.DropRightWhile(toArr(a[0]), predOf(a[1])) },
		"fill": func(a []any) any {
			s := append([]any(nil), toArr(a[0])...)
			start, end := 0, len(s)
			if len(a) > 2 {
				start = toI(a[2])
			}
			if len(a) > 3 {
				end = toI(a[3])
			}
			return lo.FillRange(s, a[1], start, end)
		},
		"findIndex":     func(a []any) any { return float64(lo.FindIndex(toArr(a[0]), predOf(a[1]))) },
		"findLastIndex": func(a []any) any { return float64(lo.FindLastIndex(toArr(a[0]), predOf(a[1]))) },
		"first":         func(a []any) any { return optOK(lo.First(toArr(a[0]))) },
		"head":          func(a []any) any { return optOK(lo.Head(toArr(a[0]))) },
		"flatten":       func(a []any) any { return lo.FlattenDepth[any](toArr(a[0]), 1) },
		"flattenDeep":   func(a []any) any { return lo.FlattenDeep[any](toArr(a[0])) },
		"flattenDepth":  func(a []any) any { return lo.FlattenDepth[any](toArr(a[0]), toI(a[1])) },
		"fromPairs":     func(a []any) any { return lo.FromPairs(pairsOf(a[0])) },
		"indexOf": func(a []any) any {
			if len(a) > 2 {
				return float64(lo.IndexOfFrom(toArr(a[0]), a[1], toI(a[2])))
			}
			return float64(lo.IndexOf(toArr(a[0]), a[1]))
		},
		"initial": func(a []any) any { return lo.Initial(toArr(a[0])) },
		"intersection": func(a []any) any {
			return lo.Intersection(restArrs(a, 0)...)
		},
		"intersectionBy": func(a []any) any {
			return lo.IntersectionBy(anyKeyOf(a[len(a)-1]), restArrs(a[:len(a)-1], 0)...)
		},
		"join": func(a []any) any { return lo.Join(toStrs(a[0]), toS(a[1])) },
		"last": func(a []any) any { return optOK(lo.Last(toArr(a[0]))) },
		"lastIndexOf": func(a []any) any {
			if len(a) > 2 {
				return float64(lo.LastIndexOfFrom(toArr(a[0]), a[1], toI(a[2])))
			}
			return float64(lo.LastIndexOf(toArr(a[0]), a[1]))
		},
		"nth":     func(a []any) any { return optOK(lo.Nth(toArr(a[0]), toI(a[1]))) },
		"pull":    func(a []any) any { return lo.Pull(append([]any(nil), toArr(a[0])...), a[1:]...) },
		"pullAll": func(a []any) any { return lo.PullAll(append([]any(nil), toArr(a[0])...), toArr(a[1])) },
		"pullAllBy": func(a []any) any {
			return lo.PullAllBy(append([]any(nil), toArr(a[0])...), toArr(a[1]), anyKeyOf(a[2]))
		},
		"pullAt":     func(a []any) any { return lo.PullAt(append([]any(nil), toArr(a[0])...), toInts(a[1])...) },
		"remove":     func(a []any) any { return lo.Remove(append([]any(nil), toArr(a[0])...), predOf(a[1])) },
		"reverse":    func(a []any) any { return lo.Reverse(toArr(a[0])) },
		"slice":      func(a []any) any { return lo.Slice(toArr(a[0]), toI(a[1]), toI(a[2])) },
		"sortedUniq": func(a []any) any { return lo.SortedUniq(toArr(a[0])) },
		"sortedUniqBy": func(a []any) any {
			return lo.SortedUniqBy(toArr(a[0]), anyKeyOf(a[1]))
		},
		"sortedIndex": func(a []any) any {
			if s, ok := allStrings(toArr(a[0])); ok {
				return float64(lo.SortedIndex(s, toS(a[1])))
			}
			return float64(lo.SortedIndex(toFloats(a[0]), toF(a[1])))
		},
		"sortedIndexBy": func(a []any) any {
			return float64(lo.SortedIndexBy(toArr(a[0]), a[1], numKeyOf(a[2])))
		},
		"sortedIndexOf": func(a []any) any {
			if s, ok := allStrings(toArr(a[0])); ok {
				return float64(lo.SortedIndexOf(s, toS(a[1])))
			}
			return float64(lo.SortedIndexOf(toFloats(a[0]), toF(a[1])))
		},
		"sortedLastIndex": func(a []any) any {
			return float64(lo.SortedLastIndex(toFloats(a[0]), toF(a[1])))
		},
		"sortedLastIndexBy": func(a []any) any {
			return float64(lo.SortedLastIndexBy(toArr(a[0]), a[1], numKeyOf(a[2])))
		},
		"sortedLastIndexOf": func(a []any) any {
			return float64(lo.SortedLastIndexOf(toFloats(a[0]), toF(a[1])))
		},
		"tail":           func(a []any) any { return lo.Tail(toArr(a[0])) },
		"take":           func(a []any) any { return lo.Take(toArr(a[0]), toI(a[1])) },
		"takeRight":      func(a []any) any { return lo.TakeRight(toArr(a[0]), toI(a[1])) },
		"takeWhile":      func(a []any) any { return lo.TakeWhile(toArr(a[0]), predOf(a[1])) },
		"takeRightWhile": func(a []any) any { return lo.TakeRightWhile(toArr(a[0]), predOf(a[1])) },
		"union":          func(a []any) any { return lo.Union(restArrs(a, 0)...) },
		"unionBy": func(a []any) any {
			return lo.UnionBy(anyKeyOf(a[len(a)-1]), restArrs(a[:len(a)-1], 0)...)
		},
		"uniq":   func(a []any) any { return lo.Uniq(toArr(a[0])) },
		"uniqBy": func(a []any) any { return lo.UniqBy(toArr(a[0]), anyKeyOf(a[1])) },
		"without": func(a []any) any {
			return lo.Without(toArr(a[0]), a[1:]...)
		},
		"uniqWith":         func(a []any) any { return lo.UniqWith(toArr(a[0]), eqOf(a[1])) },
		"differenceWith":   func(a []any) any { return lo.DifferenceWith(eqOf(a[2]), toArr(a[0]), toArr(a[1])) },
		"intersectionWith": func(a []any) any { return lo.IntersectionWith(eqOf(a[2]), toArr(a[0]), toArr(a[1])) },
		"unionWith":        func(a []any) any { return lo.UnionWith(eqOf(a[2]), toArr(a[0]), toArr(a[1])) },
		"xorWith":          func(a []any) any { return lo.XorWith(eqOf(a[2]), toArr(a[0]), toArr(a[1])) },
		"pullAllWith": func(a []any) any {
			return lo.PullAllWith(append([]any(nil), toArr(a[0])...), toArr(a[1]), eqOf(a[2]))
		},
		"xor": func(a []any) any { return lo.Xor(restArrs(a, 0)...) },
		"xorBy": func(a []any) any {
			return lo.XorBy(anyKeyOf(a[len(a)-1]), restArrs(a[:len(a)-1], 0)...)
		},
		"zip":   func(a []any) any { return pairSlice(lo.Zip(toArr(a[0]), toArr(a[1]))) },
		"unzip": func(a []any) any { x, y := lo.Unzip(pairsOf(a[0])); return []any{x, y} },
		"unzipWith": func(a []any) any {
			f := binOf(a[1])
			return lo.UnzipWith(pairsOf(a[0]), func(x, y any) any { return f(x, y) })
		},
		"zipWith": func(a []any) any {
			f := binOf(a[2])
			return lo.ZipWith(toArr(a[0]), toArr(a[1]), func(x, y any) any { return f(x, y) })
		},
		"zipObject":     func(a []any) any { return lo.ZipObject(toStrs(a[0]), toArr(a[1])) },
		"zipObjectDeep": func(a []any) any { return lo.ZipObjectDeep(toStrs(a[0]), toArr(a[1])) },
		"castArray":     func(a []any) any { return lo.CastArray(a...) },

		// ---------------- collection (slices) ----------------
		"countBy": func(a []any) any { return lo.CountBy(toArr(a[0]), strGroupOf(a[1])) },
		"every":   func(a []any) any { return lo.Every(toArr(a[0]), predOf(a[1])) },
		"filter":  func(a []any) any { return lo.Filter(toArr(a[0]), predOf(a[1])) },
		"find":    func(a []any) any { return optOK(lo.Find(toArr(a[0]), predOf(a[1]))) },
		"findLast": func(a []any) any {
			return optOK(lo.FindLast(toArr(a[0]), predOf(a[1])))
		},
		"flatMap": func(a []any) any {
			f := unOf(a[1])
			return lo.FlatMap(toArr(a[0]), func(v any) []any { return toArr(f(v)) })
		},
		"flatMapDeep": func(a []any) any {
			f := unOf(a[1])
			return lo.FlatMapDeep[any](toArr(a[0]), func(v any) []any { return toArr(f(v)) })
		},
		"flatMapDepth": func(a []any) any {
			f := unOf(a[1])
			return lo.FlatMapDepth[any](toArr(a[0]), func(v any) []any { return toArr(f(v)) }, toI(a[2]))
		},
		"forEach-collect": func(a []any) any {
			f := unOf(a[1])
			out := []any{}
			lo.ForEach(toArr(a[0]), func(v any) { out = append(out, f(v)) })
			return out
		},
		"forEachRight-collect": func(a []any) any {
			f := unOf(a[1])
			out := []any{}
			lo.ForEachRight(toArr(a[0]), func(v any) { out = append(out, f(v)) })
			return out
		},
		"forEachI-collect": func(a []any) any {
			f := idxOf(a[1])
			out := []any{}
			lo.ForEachI(toArr(a[0]), func(v any, i int) { out = append(out, f(v, i)) })
			return out
		},
		"groupBy": func(a []any) any { return lo.GroupBy(toArr(a[0]), strGroupOf(a[1])) },
		"includes": func(a []any) any {
			if s, ok := a[0].(string); ok {
				return strings.Contains(s, toS(a[1]))
			}
			return lo.Includes(toArr(a[0]), a[1])
		},
		"keyBy": func(a []any) any { return lo.KeyBy(toArr(a[0]), strGroupOf(a[1])) },
		"map":   func(a []any) any { return lo.Map(toArr(a[0]), unOf(a[1])) },
		"mapI":  func(a []any) any { f := idxOf(a[1]); return lo.MapI(toArr(a[0]), f) },
		"orderBy": func(a []any) any {
			keys := toArr(a[1])
			cmps := make([]func(x, y any) int, 0, len(keys))
			for _, k := range keys {
				n := refName(k)
				if strKeyNames[n] {
					g := strKeyOf(k)
					cmps = append(cmps, func(x, y any) int { return strings.Compare(g(x), g(y)) })
				} else {
					g := numKeyOf(k)
					cmps = append(cmps, func(x, y any) int {
						gx, gy := g(x), g(y)
						switch {
						case gx < gy:
							return -1
						case gx > gy:
							return 1
						}
						return 0
					})
				}
			}
			orders := []lo.Order{}
			for _, o := range toArr(a[2]) {
				if toS(o) == "desc" {
					orders = append(orders, lo.Desc)
				} else {
					orders = append(orders, lo.Asc)
				}
			}
			return lo.OrderBy(toArr(a[0]), cmps, orders)
		},
		"partition": func(a []any) any {
			t, f := lo.Partition(toArr(a[0]), predOf(a[1]))
			if t == nil {
				t = []any{}
			}
			if f == nil {
				f = []any{}
			}
			return []any{t, f}
		},
		"reduce": func(a []any) any {
			f := binOf(a[1])
			return lo.Reduce(toArr(a[0]), func(acc, cur any) any { return f(acc, cur) }, a[2])
		},
		"reduceRight": func(a []any) any {
			f := binOf(a[1])
			return lo.ReduceRight(toArr(a[0]), func(acc, cur any) any { return f(acc, cur) }, a[2])
		},
		"reject": func(a []any) any { return lo.Reject(toArr(a[0]), predOf(a[1])) },
		"sample": func(a []any) any {
			src := toArr(a[0])
			v, ok := lo.Sample(src, rand.New(rand.NewSource(1)))
			if !ok {
				return false
			}
			return lo.Includes(src, v)
		},
		"sampleSize": func(a []any) any {
			src := toArr(a[0])
			got := lo.SampleN(src, toI(a[1]), rand.New(rand.NewSource(1)))
			all := true
			for _, v := range got {
				if !lo.Includes(src, v) {
					all = false
				}
			}
			return []any{float64(len(got)), all}
		},
		"shuffle": func(a []any) any {
			return sortNormed(normalise(lo.Shuffle(toArr(a[0]), rand.New(rand.NewSource(1)))).([]any))
		},
		"size": func(a []any) any { return float64(lo.Size(a[0])) },
		"some": func(a []any) any { return lo.Some(toArr(a[0]), predOf(a[1])) },
		"none": func(a []any) any { return lo.None(toArr(a[0]), predOf(a[1])) },
		"sortBy": func(a []any) any {
			n := refName(a[1])
			if strKeyNames[n] {
				return lo.SortBy(toArr(a[0]), strKeyOf(a[1]))
			}
			return lo.SortBy(toArr(a[0]), numKeyOf(a[1]))
		},

		// ---------------- collection (maps) ----------------
		"mapToSlice": func(a []any) any {
			f := unOf(a[1])
			return lo.MapToSlice(toObj(a[0]), func(v any, _ string) any { return f(v) })
		},
		"filterMap": func(a []any) any {
			p := predOf(a[1])
			return lo.FilterMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
		},
		"rejectMap": func(a []any) any {
			p := predOf(a[1])
			return lo.RejectMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
		},
		"everyMap": func(a []any) any {
			p := predOf(a[1])
			return lo.EveryMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
		},
		"someMap": func(a []any) any {
			p := predOf(a[1])
			return lo.SomeMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
		},
		"noneMap": func(a []any) any {
			p := predOf(a[1])
			return lo.NoneMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
		},
		"findMap": func(a []any) any {
			p := predOf(a[1])
			return optOK(lo.FindMap(toObj(a[0]), func(v any, _ string) bool { return p(v) }))
		},
		"partitionMap": func(a []any) any {
			p := predOf(a[1])
			t, f := lo.PartitionMap(toObj(a[0]), func(v any, _ string) bool { return p(v) })
			return []any{sortNormed(normalise(orEmpty(t)).([]any)), sortNormed(normalise(orEmpty(f)).([]any))}
		},
		"groupByMap": func(a []any) any {
			g := strGroupOf(a[1])
			m := lo.GroupByMap(toObj(a[0]), func(v any) string { return g(v) })
			out := map[string]any{}
			for k, v := range m {
				out[k] = sortNormed(normalise(v).([]any))
			}
			return out
		},
		"countByMap": func(a []any) any {
			g := strGroupOf(a[1])
			out := map[string]any{}
			for _, v := range lo.MapToSlice(toObj(a[0]), func(v any, _ string) any { return v }) {
				k := g(v)
				out[k] = toF(out[k]) + 1
			}
			return out
		},
		"maxByMap": func(a []any) any { return optOK(lo.MaxByMap(toObj(a[0]), numKeyOf(a[1]))) },
		"minByMap": func(a []any) any { return optOK(lo.MinByMap(toObj(a[0]), numKeyOf(a[1]))) },
		"reduceMap": func(a []any) any {
			f := binOf(a[1])
			return lo.ReduceMap(toObj(a[0]), func(acc, v any, _ string) any { return f(acc, v) }, a[2])
		},
		"includesValue": func(a []any) any {
			return lo.IncludesValue(strKeyMap(toObj(a[0])), cmpKey(a[1]))
		},
		"forOwn-collect": func(a []any) any {
			f := unOf(a[1])
			out := []any{}
			lo.ForOwn(toObj(a[0]), func(k string, v any) { out = append(out, []any{k, f(v)}) })
			return out
		},
		"transform": func(a []any) any {
			f := binOf(a[1])
			return lo.Transform(toObj(a[0]), func(acc, v any, _ string) any { return f(acc, v) }, a[2])
		},

		// ---------------- object ----------------
		"assign": func(a []any) any { return lo.Assign(cloneObj(a[0]), objs(a[1:])...) },
		"assignWith": func(a []any) any {
			f := binOf(a[2])
			return lo.AssignWith(cloneObj(a[0]), func(d, s any) any { return f(d, s) }, toObj(a[1]))
		},
		"at":       func(a []any) any { return lo.At(toObj(a[0]), toStrs(a[1])...) },
		"defaults": func(a []any) any { return lo.Defaults(cloneObj(a[0]), objs(a[1:])...) },
		"defaultsDeep": func(a []any) any {
			return lo.DefaultsDeep(cloneObj(a[0]), objs(a[1:])...)
		},
		"entries": func(a []any) any { return entrySlice(lo.Entries(toObj(a[0]))) },
		"findKey": func(a []any) any {
			p := predOf(a[1])
			return optOK(lo.FindKey(toObj(a[0]), func(_ string, v any) bool { return p(v) }))
		},
		"fromEntries": func(a []any) any {
			es := []lo.Entry[string, any]{}
			for _, e := range toArr(a[0]) {
				p := toArr(e)
				es = append(es, lo.Entry[string, any]{Key: toS(p[0]), Value: p[1]})
			}
			return lo.FromEntries(es)
		},
		"get":    func(a []any) any { return optOK(lo.Get(toObj(a[0]), toS(a[1]))) },
		"getOr":  func(a []any) any { return lo.GetOr(toObj(a[0]), toS(a[1]), a[2]) },
		"has":    func(a []any) any { return lo.Has(toObj(a[0]), toS(a[1])) },
		"invert": func(a []any) any { return remapKeys(lo.Invert(anyMap(toObj(a[0])))) },
		"invertByKeys": func(a []any) any {
			g := strGroupOf(a[1])
			m := toObj(a[0])
			out := map[string]any{}
			for _, k := range lo.SortedKeys(m) {
				gk := g(m[k])
				lst, _ := out[gk].([]any)
				out[gk] = append(lst, k)
			}
			for k, v := range out {
				out[k] = sortNormed(v.([]any))
			}
			return out
		},
		"keys":       func(a []any) any { return anyOfStrings(lo.Keys(toObj(a[0]))) },
		"sortedKeys": func(a []any) any { return anyOfStrings(lo.SortedKeys(toObj(a[0]))) },
		"values": func(a []any) any {
			if _, ok := a[0].([]any); ok {
				return toArr(a[0])
			}
			return lo.Values(toObj(a[0]))
		},
		"mapKeys": func(a []any) any {
			f := strGroupOf(a[1])
			return lo.MapKeys(toObj(a[0]), func(k string, v any) string { return f(v) })
		},
		"mapValues": func(a []any) any {
			f := unOf(a[1])
			return lo.MapValues(toObj(a[0]), func(_ string, v any) any { return f(v) })
		},
		"merge": func(a []any) any { return lo.Merge(append([]map[string]any{cloneObj(a[0])}, objs(a[1:])...)...) },
		"mergeWith": func(a []any) any {
			f := binOf(a[2])
			return lo.MergeWith(func(e, i any) any { return f(e, i) }, cloneObj(a[0]), toObj(a[1]))
		},
		"omit": func(a []any) any { return lo.Omit(toObj(a[0]), toStrs(a[1])...) },
		"omitBy": func(a []any) any {
			p := predOf(a[1])
			return lo.OmitBy(toObj(a[0]), func(_ string, v any) bool { return p(v) })
		},
		"pick": func(a []any) any { return lo.Pick(toObj(a[0]), toStrs(a[1])...) },
		"pickBy": func(a []any) any {
			p := predOf(a[1])
			return lo.PickBy(toObj(a[0]), func(_ string, v any) bool { return p(v) })
		},
		"result": func(a []any) any { return optOK(lo.Result(toObj(a[0]), toS(a[1]))) },
		"set":    func(a []any) any { return lo.Set(cloneObj(a[0]), toS(a[1]), a[2]) },
		"setWith": func(a []any) any {
			return lo.SetWith(cloneObj(a[0]), toS(a[1]), a[2], func(string) any { return map[string]any{} })
		},
		"toPairs": func(a []any) any { return pairSlice(lo.ToPairs(toObj(a[0]))) },
		"toPath":  func(a []any) any { return anyOfStrings(lo.ToPath(toS(a[0]))) },
		"unset": func(a []any) any {
			m, ok := lo.Unset(cloneObj(a[0]), toS(a[1]))
			return map[string]any{"ok": ok, "object": m}
		},
		"update": func(a []any) any {
			f := unOf(a[2])
			return lo.Update(cloneObj(a[0]), toS(a[1]), func(old any) any {
				if old == nil {
					return f(undef{})
				}
				return f(old)
			})
		},

		"updateWith": func(a []any) any {
			f := unOf(a[2])
			return lo.UpdateWith(cloneObj(a[0]), toS(a[1]), func(old any) any {
				if old == nil {
					return f(undef{})
				}
				return f(old)
			}, func(string) any { return map[string]any{} })
		},

		// ---------------- string ----------------
		"camelCase":    func(a []any) any { return lo.CamelCase(toS(a[0])) },
		"capitalize":   func(a []any) any { return lo.Capitalize(toS(a[0])) },
		"deburr":       func(a []any) any { return lo.Deburr(toS(a[0])) },
		"endsWith":     func(a []any) any { return lo.EndsWith(toS(a[0]), toS(a[1])) },
		"escape":       func(a []any) any { return lo.Escape(toS(a[0])) },
		"escapeRegExp": func(a []any) any { return lo.EscapeRegExp(toS(a[0])) },
		"kebabCase":    func(a []any) any { return lo.KebabCase(toS(a[0])) },
		"lowerCase":    func(a []any) any { return lo.LowerCase(toS(a[0])) },
		"lowerFirst":   func(a []any) any { return lo.LowerFirst(toS(a[0])) },
		"pad":          func(a []any) any { return lo.Pad(toS(a[0]), toI(a[1]), padChars(a, 2)) },
		"padEnd":       func(a []any) any { return lo.PadEnd(toS(a[0]), toI(a[1]), padChars(a, 2)) },
		"padStart":     func(a []any) any { return lo.PadStart(toS(a[0]), toI(a[1]), padChars(a, 2)) },
		"parseFloat": func(a []any) any {
			v, err := lo.ParseFloat(toS(a[0]))
			if err != nil {
				return math.NaN()
			}
			return v
		},
		"parseInt": func(a []any) any {
			base := 10
			if len(a) > 1 && toI(a[1]) != 0 {
				base = toI(a[1])
			}
			n, err := lo.ParseInt(toS(a[0]), base)
			if err != nil {
				return math.NaN()
			}
			return float64(n)
		},
		"repeat":     func(a []any) any { return lo.Repeat(toS(a[0]), toI(a[1])) },
		"replace":    func(a []any) any { return lo.Replace(toS(a[0]), toS(a[1]), toS(a[2])) },
		"snakeCase":  func(a []any) any { return lo.SnakeCase(toS(a[0])) },
		"split":      func(a []any) any { return anyOfStrings(lo.Split(toS(a[0]), toS(a[1]), splitLimit(a))) },
		"startCase":  func(a []any) any { return lo.StartCase(toS(a[0])) },
		"startsWith": func(a []any) any { return lo.StartsWith(toS(a[0]), toS(a[1])) },
		"template":   func(a []any) any { return lo.Template(toS(a[0]))(toObj(a[1])) },
		"toLower":    func(a []any) any { return lo.ToLower(toS(a[0])) },
		"toUpper":    func(a []any) any { return lo.ToUpper(toS(a[0])) },
		"trim":       func(a []any) any { return lo.Trim(toS(a[0]), cutset(a, 1)) },
		"trimEnd":    func(a []any) any { return lo.TrimEnd(toS(a[0]), cutset(a, 1)) },
		"trimStart":  func(a []any) any { return lo.TrimStart(toS(a[0]), cutset(a, 1)) },
		"truncate":   func(a []any) any { return lo.Truncate(toS(a[0]), toI(a[1]), toS(a[2])) },
		"unescape":   func(a []any) any { return lo.Unescape(toS(a[0])) },
		"upperCase":  func(a []any) any { return lo.UpperCase(toS(a[0])) },
		"upperFirst": func(a []any) any { return lo.UpperFirst(toS(a[0])) },
		"words":      func(a []any) any { return anyOfStrings(lo.Words(toS(a[0]))) },
		"pascalCase": func(a []any) any { return lo.PascalCase(toS(a[0])) },

		// ---------------- number / math ----------------
		"add":      func(a []any) any { return lo.Add(toF(a[0]), toF(a[1])) },
		"ceil":     func(a []any) any { return lo.Ceil(toF(a[0]), toI(a[1])) },
		"clamp":    func(a []any) any { return lo.Clamp(toF(a[0]), toF(a[1]), toF(a[2])) },
		"divide":   func(a []any) any { return lo.Divide(toF(a[0]), toF(a[1])) },
		"floor":    func(a []any) any { return lo.Floor(toF(a[0]), toI(a[1])) },
		"inRange":  func(a []any) any { return lo.InRange(toF(a[0]), toF(a[1]), toF(a[2])) },
		"max":      func(a []any) any { return optOK(lo.Max(toFloats(a[0]))) },
		"maxBy":    func(a []any) any { return optOK(lo.MaxBy(toArr(a[0]), numKeyOf(a[1]))) },
		"mean":     func(a []any) any { return lo.Mean(toFloats(a[0])) },
		"meanBy":   func(a []any) any { return lo.MeanBy(toArr(a[0]), numKeyOf(a[1])) },
		"min":      func(a []any) any { return optOK(lo.Min(toFloats(a[0]))) },
		"minBy":    func(a []any) any { return optOK(lo.MinBy(toArr(a[0]), numKeyOf(a[1]))) },
		"multiply": func(a []any) any { return lo.Multiply(toF(a[0]), toF(a[1])) },
		"round":    func(a []any) any { return lo.Round(toF(a[0]), toI(a[1])) },
		"subtract": func(a []any) any { return lo.Subtract(toF(a[0]), toF(a[1])) },
		"sum":      func(a []any) any { return lo.Sum(toFloats(a[0])) },
		"sumBy":    func(a []any) any { return lo.SumBy(toArr(a[0]), numKeyOf(a[1])) },
		"range":    func(a []any) any { return rangeOp(a, false) },
		"rangeRight": func(a []any) any {
			return rangeOp(a, true)
		},
		"random": func(a []any) any {
			v := lo.Random(toI(a[0]), toI(a[1]), rand.New(rand.NewSource(1)))
			return v >= toI(a[0]) && v <= toI(a[1])
		},

		// ---------------- lang ----------------
		"clone":       func(a []any) any { return lo.Clone(a[0]) },
		"cloneDeep":   func(a []any) any { return lo.CloneDeep(a[0]) },
		"eq":          func(a []any) any { return lo.Eq(a[0], a[1]) },
		"gt":          func(a []any) any { return cmpOp(a, "gt") },
		"gte":         func(a []any) any { return cmpOp(a, "gte") },
		"lt":          func(a []any) any { return cmpOp(a, "lt") },
		"lte":         func(a []any) any { return cmpOp(a, "lte") },
		"isArrayLike": func(a []any) any { return lo.IsArrayLike(undefNil(a[0])) },
		"isArrayLikeObject": func(a []any) any {
			return lo.IsArrayLikeObject(undefNil(a[0]))
		},
		"isBoolean":     func(a []any) any { return lo.IsBool(undefNil(a[0])) },
		"isBuffer":      func(a []any) any { return lo.IsBuffer(undefNil(a[0])) },
		"isDate":        func(a []any) any { return lo.IsDate(undefNil(a[0])) },
		"isEmpty":       func(a []any) any { return lo.IsEmpty(undefNil(a[0])) },
		"isEqual":       func(a []any) any { return lo.IsEqual(undefNil(a[0]), undefNil(a[1])) },
		"isError":       func(a []any) any { return lo.IsError(undefNil(a[0])) },
		"isFinite":      func(a []any) any { return lo.IsFinite(undefNil(a[0])) },
		"isFunction":    func(a []any) any { return lo.IsFunction(undefNil(a[0])) },
		"isInteger":     func(a []any) any { return lo.IsInteger(undefNil(a[0])) },
		"isLength":      func(a []any) any { return lo.IsLength(undefNil(a[0])) },
		"isMap":         func(a []any) any { return lo.IsMap(undefNil(a[0])) },
		"isMatch":       func(a []any) any { return lo.IsMatch(toObj(a[0]), toObj(a[1])) },
		"isNaN":         func(a []any) any { return lo.IsNaN(undefNil(a[0])) },
		"isNil":         func(a []any) any { return lo.IsNil(undefNil(a[0])) },
		"isNull":        func(a []any) any { return lo.IsNull(undefNil(a[0])) },
		"isNumber":      func(a []any) any { return lo.IsNumber(undefNil(a[0])) },
		"isObject":      func(a []any) any { return lo.IsObject(undefNil(a[0])) },
		"isObjectLike":  func(a []any) any { return lo.IsObjectLike(undefNil(a[0])) },
		"isPlainObject": func(a []any) any { return lo.IsPlainObject(undefNil(a[0])) },
		"isRegExp":      func(a []any) any { return lo.IsRegExp(undefNil(a[0])) },
		"isSafeInteger": func(a []any) any { return lo.IsSafeInteger(undefNil(a[0])) },
		"isString":      func(a []any) any { return lo.IsString(undefNil(a[0])) },
		"isUndefined":   func(a []any) any { return lo.IsUndefined(undefNil(a[0])) },
		"isArray":       func(a []any) any { return lo.IsSlice(undefNil(a[0])) },
		"toArray":       func(a []any) any { return lo.ToArray(toArr(a[0])) },
		"toFinite":      func(a []any) any { return lo.ToFinite(undefNil(a[0])) },
		"toInteger":     func(a []any) any { return float64(lo.ToInteger(undefNil(a[0]))) },
		"toLength":      func(a []any) any { return float64(lo.ToLength(undefNil(a[0]))) },
		"toNumber":      func(a []any) any { return lo.ToNumber(undefNil(a[0])) },
		"toSafeInteger": func(a []any) any { return float64(lo.ToSafeInteger(undefNil(a[0]))) },
		"toString":      func(a []any) any { return lo.ToString(undefNil(a[0])) },
		"defaultTo":     func(a []any) any { return defaultToOp(a) },
		"conformsTo":    func(a []any) any { return lo.ConformsTo(toObj(a[0]), specOf(a[1])) },
		"isEqualWith": func(a []any) any {
			return lo.IsEqualWith(undefNil(a[0]), undefNil(a[1]), func(x, y any) (bool, bool) { return false, false })
		},

		// ---------------- function / combinators ----------------
		"after": func(a []any) any {
			f := zeroFns["counter"]()
			g := lo.After(toI(a[0]), f)
			out := []any{}
			for range callsOf(a[1]) {
				out = append(out, optOK(g()))
			}
			return out
		},
		"before": func(a []any) any {
			f := zeroFns["counter"]()
			g := lo.Before(toI(a[0]), f)
			out := []any{}
			for range callsOf(a[1]) {
				out = append(out, g())
			}
			return out
		},
		"once": func(a []any) any {
			f := zeroFns["counter"]()
			g := lo.Once(f)
			out := []any{}
			for range callsOf(a[0]) {
				out = append(out, g())
			}
			return out
		},
		"memoize": func(a []any) any {
			f := zeroFns["counter"]()
			g := lo.Memoize(func(k any) any { return jsNumString(toF(f())) + ":" + toS(k) })
			out := []any{}
			for _, c := range callsOf(a[0]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"negate": func(a []any) any {
			g := lo.Negate(predOf(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"unary": func(a []any) any {
			f := unOf(a[0])
			g := lo.Unary(func(vs ...any) any {
				if len(vs) == 0 {
					return f(undef{})
				}
				return f(vs[0])
			})
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"ary": func(a []any) any {
			f := binOf(a[0])
			g := lo.Ary(func(vs ...any) any {
				var x, y any = undef{}, undef{}
				if len(vs) > 0 {
					x = vs[0]
				}
				if len(vs) > 1 {
					y = vs[1]
				}
				return f(x, y)
			}, toI(a[1]))
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c...))
			}
			return out
		},
		"flip": func(a []any) any {
			f := binOf(a[0])
			g := lo.Flip(func(x, y any) any { return f(x, y) })
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0], c[1]))
			}
			return out
		},
		"partial": func(a []any) any {
			f := binOf(a[0])
			g := lo.Partial(func(x, y any) any { return f(x, y) }, a[1])
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"partialRight": func(a []any) any {
			f := binOf(a[0])
			g := lo.PartialRight(func(x, y any) any { return f(x, y) }, a[1])
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"rearg": func(a []any) any {
			f := binOf(a[0])
			g := lo.Rearg(func(vs ...any) any { return f(vs[0], vs[1]) }, toInts(a[1])...)
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c...))
			}
			return out
		},
		"spread": func(a []any) any {
			f := binOf(a[0])
			g := lo.Spread(func(vs ...any) any { return f(vs[0], vs[1]) })
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(toArr(c[0])))
			}
			return out
		},
		"rest": func(a []any) any {
			g := lo.Rest(func(vs []any) any { return orEmpty(vs) })
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c...))
			}
			return out
		},
		"overArgs": func(a []any) any {
			f := binOf(a[0])
			ts := toArr(a[1])
			transforms := make([]func(any) any, len(ts))
			for i, t := range ts {
				transforms[i] = unOf(t)
			}
			g := lo.OverArgs(func(vs ...any) any { return f(vs[0], vs[1]) }, transforms...)
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c...))
			}
			return out
		},
		"wrap": func(a []any) any {
			f := binOf(a[1])
			g := lo.Wrap(a[0], func(v, arg any) any { return f(v, arg) })
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"curry2": func(a []any) any {
			f := binOf(a[0])
			return lo.Curry(func(x, y any) any { return f(x, y) })(a[1])(a[2])
		},
		"curryRight2": func(a []any) any {
			f := binOf(a[0])
			return lo.CurryRight(func(x, y any) any { return f(x, y) })(a[1])(a[2])
		},
		"flow": func(a []any) any {
			fs := fnList(a[0])
			g := lo.Flow(fs...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"flowRight": func(a []any) any {
			fs := fnList(a[0])
			g := lo.FlowRight(fs...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"over": func(a []any) any {
			fs := fnList(a[0])
			g := lo.Over(fs...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"overEvery": func(a []any) any {
			g := lo.OverEvery(predList(a[0])...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"overSome": func(a []any) any {
			g := lo.OverSome(predList(a[0])...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c[0]))
			}
			return out
		},
		"cond": func(a []any) any {
			pairs := []lo.CondPair[any, any]{}
			for _, p := range toArr(a[0]) {
				pp := toArr(p)
				pairs = append(pairs, lo.CondPair[any, any]{Pred: predOf(pp[0]), Result: unOf(pp[1])})
			}
			g := lo.Cond(pairs...)
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, optOK(g(c[0])))
			}
			return out
		},
		"constant": func(a []any) any {
			g := lo.Constant(a[0])
			out := []any{}
			for range callsOf(a[1]) {
				out = append(out, g())
			}
			return out
		},
		"identityFn": func(a []any) any {
			out := []any{}
			for _, c := range callsOf(a[0]) {
				out = append(out, lo.Identity(c[0]))
			}
			return out
		},
		"nthArg": func(a []any) any {
			g := lo.NthArg[any](toI(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(c...))
			}
			return out
		},
		"property": func(a []any) any {
			g := lo.Property(toS(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, optOK(g(toObj(c[0]))))
			}
			return out
		},
		"propertyOf": func(a []any) any {
			g := lo.PropertyOf(toObj(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, optOK(g(toS(c[0]))))
			}
			return out
		},
		"matches": func(a []any) any {
			g := lo.Matches(toObj(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(toObj(c[0])))
			}
			return out
		},
		"matchesProperty": func(a []any) any {
			g := lo.MatchesProperty(toS(a[0]), a[1])
			out := []any{}
			for _, c := range callsOf(a[2]) {
				out = append(out, g(toObj(c[0])))
			}
			return out
		},
		"conforms": func(a []any) any {
			g := lo.Conforms(specOf(a[0]))
			out := []any{}
			for _, c := range callsOf(a[1]) {
				out = append(out, g(toObj(c[0])))
			}
			return out
		},
		"times": func(a []any) any {
			f := unOf(a[1])
			return lo.Times(toI(a[0]), func(i int) any { return f(float64(i)) })
		},
		"noop":       func(a []any) any { lo.Noop(); return undef{} },
		"stubArray":  func(a []any) any { return lo.StubArray[any]() },
		"stubObject": func(a []any) any { return lo.StubObject() },
		"stubString": func(a []any) any { return lo.StubString() },
		"stubTrue":   func(a []any) any { return lo.StubTrue() },
		"stubFalse":  func(a []any) any { return lo.StubFalse() },
		"attempt": func(a []any) any {
			mode := toS(a[0])
			v, err := lo.Attempt(func() any {
				if mode == "throw" {
					panic(errors.New("boom"))
				}
				return float64(7)
			})
			if err != nil {
				return map[string]any{"$threw": true}
			}
			return v
		},
		"uniqueId": func(a []any) any {
			p := toS(a[0])
			x := lo.UniqueID(p)
			y := lo.UniqueID(p)
			return []any{stripDigits(x), stripDigits(y), float64(trailingNum(y) - trailingNum(x))}
		},

		// ---------------- seq / chain ----------------
		"seq":        func(a []any) any { return runSeq(a[0], toArr(a[1]), false) },
		"seqSet":     func(a []any) any { return runSeq(a[0], toArr(a[1]), true) },
		"chainValue": func(a []any) any { return lo.Chain(a[0]).Value() },
		"thru":       func(a []any) any { f := unOf(a[1]); return lo.Thru(lo.Chain(a[0]), f).Value() },
		"tap": func(a []any) any {
			seen := []any{}
			f := unOf(a[1])
			out := lo.Tap(lo.Chain(a[0]), func(v any) { seen = append(seen, v); f(v) }).Value()
			return map[string]any{"seen": seen, "value": out}
		},
	}
	// aliases: lodash exposes several names for the same behaviour
	ops["each"] = ops["forEach-collect"]
	ops["eachRight"] = ops["forEachRight-collect"]
	ops["extend"] = ops["assign"]
	ops["assignIn"] = ops["assign"]
	ops["forOwn"] = ops["forOwn-collect"]
	ops["forIn"] = ops["forOwn-collect"]
	ops["extendWith"] = ops["assignWith"]
	ops["assignInWith"] = ops["assignWith"]
	ops["keysIn"] = ops["sortedKeys"]
	ops["valuesIn"] = ops["values"]
	ops["toPairsIn"] = ops["toPairs"]
	ops["entriesIn"] = ops["entries"]
	ops["forOwnRight"] = ops["forOwn-collect"]
	ops["forInRight"] = ops["forOwn-collect"]
	ops["findLastKey"] = ops["findKey"]
	ops["isMatchWith"] = func(a []any) any {
		eq := eqOf(a[2])
		return lo.IsMatchWith(toObj(a[0]), toObj(a[1]), eq)
	}
	for _, k := range []string{"keysIn", "valuesIn", "toPairsIn", "entriesIn", "forOwnRight", "forInRight"} {
		sortAlways[k] = true
	}
	sortAlways["forOwn"] = true
	sortAlways["forIn"] = true
	ops["first"] = ops["head"]
}

func orEmpty(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

func toAnySlices(ss [][]any) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func anyOfStrings(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func cloneObj(v any) map[string]any {
	src := toObj(v)
	out := make(map[string]any, len(src))
	for k, e := range src {
		out[k] = deepCopy(e)
	}
	return out
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopy(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = deepCopy(e)
		}
		return out
	default:
		return v
	}
}

func objs(vs []any) []map[string]any {
	out := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		out = append(out, toObj(v))
	}
	return out
}

func anyMap(m map[string]any) map[string]any { return m }

// strKeyMap and cmpKey narrow a decoded object to the comparable value type
// IncludesValue needs.
func strKeyMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = cmpKey(v)
	}
	return out
}

func cmpKey(v any) string {
	b, _ := json.Marshal(normalise(v))
	return string(b)
}

func remapKeys(m map[any]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[keyString(k)] = v
	}
	return out
}

func fnList(v any) []func(any) any {
	a := toArr(v)
	out := make([]func(any) any, len(a))
	for i, e := range a {
		out[i] = unOf(e)
	}
	return out
}

func predList(v any) []func(any) bool {
	a := toArr(v)
	out := make([]func(any) bool, len(a))
	for i, e := range a {
		out[i] = predOf(e)
	}
	return out
}

func specOf(v any) map[string]func(any) bool {
	m := toObj(v)
	out := make(map[string]func(any) bool, len(m))
	for k, e := range m {
		out[k] = predOf(e)
	}
	return out
}

func undefNil(v any) any {
	if _, ok := v.(undef); ok {
		return nil
	}
	return v
}

func padChars(a []any, i int) string {
	if len(a) > i {
		return toS(a[i])
	}
	return " "
}

func cutset(a []any, i int) string {
	if len(a) > i {
		return toS(a[i])
	}
	return " \t\n\v\f\r"
}

func splitLimit(a []any) int {
	if len(a) > 2 {
		if _, ok := a[2].(undef); !ok {
			return toI(a[2])
		}
	}
	return -1
}

func rangeOp(a []any, right bool) any {
	var s []int
	switch len(a) {
	case 1:
		if right {
			s = lo.RangeRight(toI(a[0]))
		} else {
			s = lo.Range(toI(a[0]))
		}
	case 2:
		s = lo.RangeStep(toI(a[0]), toI(a[1]), 1)
		if right {
			s = lo.Reverse(s)
		}
	default:
		s = lo.RangeStep(toI(a[0]), toI(a[1]), toI(a[2]))
		if right {
			s = lo.Reverse(s)
		}
	}
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = float64(v)
	}
	return out
}

func cmpOp(a []any, which string) any {
	if x, ok := a[0].(string); ok {
		y := toS(a[1])
		switch which {
		case "gt":
			return lo.Gt(x, y)
		case "gte":
			return lo.Gte(x, y)
		case "lt":
			return lo.Lt(x, y)
		default:
			return lo.Lte(x, y)
		}
	}
	x, y := toF(a[0]), toF(a[1])
	switch which {
	case "gt":
		return lo.Gt(x, y)
	case "gte":
		return lo.Gte(x, y)
	case "lt":
		return lo.Lt(x, y)
	default:
		return lo.Lte(x, y)
	}
}

func defaultToOp(a []any) any {
	switch a[0].(type) {
	case string:
		return lo.DefaultTo(toS(a[0]), toS(a[1]))
	case float64:
		return lo.DefaultTo(toF(a[0]), toF(a[1]))
	case bool:
		return lo.DefaultTo(a[0].(bool), a[1].(bool))
	default:
		return lo.DefaultTo[any](undefNil(a[0]), undefNil(a[1]))
	}
}

func stripDigits(s string) string {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[:i] + "#"
}

func trailingNum(s string) int {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	var n int
	fmt.Sscanf(s[i:], "%d", &n)
	return n
}

// runSeq interprets the mini pipeline shared with node/run.js, exercising the
// chained SliceSeq / SetSeq wrappers.
func runSeq(input any, steps []any, asSet bool) any {
	if asSet {
		cur := lo.ChainSet(toArr(input))
		for _, st := range steps {
			s := toArr(st)
			switch toS(s[0]) {
			case "filter":
				cur = cur.Filter(predOf(s[1]))
			case "reverse":
				cur = cur.Reverse()
			case "uniq":
				cur = cur.Uniq()
			case "compact":
				cur = cur.Compact()
			case "without":
				cur = cur.Without(toArr(s[1])...)
			case "union":
				cur = cur.Union(toArr(s[1]))
			case "intersection":
				cur = cur.Intersection(toArr(s[1]))
			case "difference":
				cur = cur.Difference(toArr(s[1]))
			case "value":
				return cur.Value()
			case "size":
				return float64(cur.Size())
			case "includes":
				return cur.Includes(s[1])
			case "indexOf":
				return float64(cur.IndexOf(s[1]))
			default:
				panic("unknown seqSet step: " + toS(s[0]))
			}
		}
		return cur.Value()
	}
	cur := lo.ChainSlice(toArr(input))
	for _, st := range steps {
		s := toArr(st)
		switch toS(s[0]) {
		case "filter":
			cur = cur.Filter(predOf(s[1]))
		case "reject":
			cur = cur.Reject(predOf(s[1]))
		case "take":
			cur = cur.Take(toI(s[1]))
		case "takeRight":
			cur = cur.TakeRight(toI(s[1]))
		case "drop":
			cur = cur.Drop(toI(s[1]))
		case "dropRight":
			cur = cur.DropRight(toI(s[1]))
		case "takeWhile":
			cur = cur.TakeWhile(predOf(s[1]))
		case "dropWhile":
			cur = cur.DropWhile(predOf(s[1]))
		case "reverse":
			cur = cur.Reverse()
		case "tail":
			cur = cur.Tail()
		case "initial":
			cur = cur.Initial()
		case "slice":
			cur = cur.Slice(toI(s[1]), toI(s[2]))
		case "concat":
			cur = cur.Concat(toArr(s[1]))
		case "chunk":
			return toAnySlices(cur.Chunk(toI(s[1])))
		case "value":
			return cur.Value()
		case "head":
			return optOK(cur.Head())
		case "last":
			return optOK(cur.Last())
		case "nth":
			return optOK(cur.Nth(toI(s[1])))
		case "join":
			return cur.Join(toS(s[1]))
		case "size":
			return float64(cur.Size())
		case "isEmpty":
			return cur.IsEmpty()
		default:
			panic("unknown seq step: " + toS(s[0]))
		}
	}
	return cur.Value()
}

// ---------------------------------------------------------------------------
// main loop
// ---------------------------------------------------------------------------

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func dispatch(fn string, args []any) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	op, ok := ops[fn]
	if !ok {
		return nil, fmt.Errorf("unknown op: %s", fn)
	}
	out := normalise(op(args))
	if sortAlways[fn] {
		if arr, ok := out.([]any); ok {
			out = sortNormed(arr)
		}
	}
	return out, nil
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request json: " + err.Error()})
			_ = out.Flush()
			continue
		}
		args := make([]any, len(req.Args))
		for i, a := range req.Args {
			args[i] = resolve(a)
		}
		v, err := dispatch(req.Fn, args)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: v})
		}
		_ = out.Flush()
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
	}
}
