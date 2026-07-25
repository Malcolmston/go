package parity

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Comparing a Go value to an upstream one is the whole point of the harness, so
// both sides are first flattened into the same canonical JSON shape: numbers as
// numbers (with NaN/±Inf as their names, which JSON cannot express), bytes as
// base64, time as RFC 3339, functions as {"$fn": name}. What remains is a
// behavioral difference rather than a difference in how two ecosystems happen
// to spell the same value.

// canonical renders a Go value the way the drivers render upstream values.
func canonical(v any) (json.RawMessage, error) {
	return json.Marshal(normalize(reflect.ValueOf(v), 0))
}

const maxDepth = 64

func normalize(rv reflect.Value, depth int) any {
	if depth > maxDepth {
		return map[string]any{"$cycle": true}
	}
	if !rv.IsValid() {
		return nil
	}
	switch rv.Kind() {
	case reflect.Interface, reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		// Errors are values here: a port raising where upstream throws is a
		// match, so they must survive normalization as comparable data.
		if err, ok := rv.Interface().(error); ok {
			return map[string]any{"$error": err.Error()}
		}
		if m, ok := marshalerJSON(rv); ok {
			return m
		}
		return normalize(rv.Elem(), depth+1)
	}

	if m, ok := marshalerJSON(rv); ok {
		return m
	}
	if rv.Type() == reflect.TypeOf(time.Time{}) {
		return rv.Interface().(time.Time).Format(time.RFC3339Nano)
	}

	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint()
	case reflect.Float32, reflect.Float64:
		return normalizeFloat(rv.Float())
	case reflect.Complex64, reflect.Complex128:
		c := rv.Complex()
		return map[string]any{"real": normalizeFloat(real(c)), "imag": normalizeFloat(imag(c))}
	case reflect.Slice:
		if rv.IsNil() {
			return nil
		}
		fallthrough
	case reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return bytesValue(rv) // base64, as encoding/json and the drivers do
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = normalize(rv.Index(i), depth+1)
		}
		return out
	case reflect.Map:
		if rv.IsNil() {
			return nil
		}
		out := make(map[string]any, rv.Len())
		for _, k := range rv.MapKeys() {
			out[keyString(k)] = normalize(rv.MapIndex(k), depth+1)
		}
		return out
	case reflect.Struct:
		return normalizeStruct(rv, depth)
	case reflect.Func:
		return map[string]any{"$fn": "anonymous"}
	case reflect.Chan, reflect.UnsafePointer:
		return fmt.Sprint(rv)
	default:
		return fmt.Sprint(rv)
	}
}

func normalizeFloat(f float64) any {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	default:
		return f
	}
}

// normalizeStruct walks exported fields, honoring encoding/json tags so a port's
// own wire shape is what gets compared.
func normalizeStruct(rv reflect.Value, depth int) any {
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue // unexported
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" && opts == "" {
			continue
		}
		fv := rv.Field(i)
		if f.Anonymous && name == "" {
			if embedded, ok := normalize(fv, depth+1).(map[string]any); ok {
				for k, v := range embedded {
					out[k] = v
				}
				continue
			}
		}
		if name == "" {
			name = f.Name
		}
		if strings.Contains(opts, "omitempty") && fv.IsZero() {
			continue
		}
		out[name] = normalize(fv, depth+1)
	}
	return out
}

// marshalerJSON gives types with their own JSON encoding the final say.
func marshalerJSON(rv reflect.Value) (any, bool) {
	if !rv.CanInterface() {
		return nil, false
	}
	m, ok := rv.Interface().(json.Marshaler)
	if !ok {
		return nil, false
	}
	b, err := m.MarshalJSON()
	if err != nil {
		return map[string]any{"$error": err.Error()}, true
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return string(b), true
	}
	return out, true
}

func bytesValue(rv reflect.Value) any {
	b := make([]byte, rv.Len())
	for i := range b {
		b[i] = byte(rv.Index(i).Uint())
	}
	var out any
	raw, _ := json.Marshal(b)
	json.Unmarshal(raw, &out)
	return out
}

func keyString(k reflect.Value) string {
	if k.Kind() == reflect.String {
		return k.String()
	}
	return fmt.Sprint(k.Interface())
}

// equal reports whether two canonical values agree. Floats compare within tol
// (relative for large magnitudes) because no two ecosystems round identically —
// upstream's own test suites carry the same allowance.
func equal(a, b any, tol float64) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
		// "NaN"/"Infinity" may meet a real float from the other side only if
		// that float is itself special, which normalization already excluded.
		return false
	case float64:
		bv, ok := b.(float64)
		return ok && floatsEqual(av, bv, tol)
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equal(av[i], bv[i], tol) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, x := range av {
			y, ok := bv[k]
			if !ok || !equal(x, y, tol) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func floatsEqual(a, b, tol float64) bool {
	if a == b {
		return true
	}
	if tol <= 0 {
		return false
	}
	diff := math.Abs(a - b)
	if diff <= tol {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(b))
	return diff <= tol*scale
}

// diff renders a short, readable explanation of where two canonical values
// part ways — the first differing path, not a wall of JSON.
func diff(got, want any, tol float64) string {
	if p, g, w, ok := firstDiff(got, want, tol, ""); ok {
		where := p
		if where == "" {
			where = "value"
		}
		return fmt.Sprintf("%s: go=%s upstream=%s", where, brief(g), brief(w))
	}
	return ""
}

func firstDiff(got, want any, tol float64, path string) (string, any, any, bool) {
	switch gv := got.(type) {
	case []any:
		wv, ok := want.([]any)
		if !ok {
			break
		}
		if len(gv) != len(wv) {
			return path + fmt.Sprintf(" (len %d vs %d)", len(gv), len(wv)), got, want, true
		}
		for i := range gv {
			if p, g, w, ok := firstDiff(gv[i], wv[i], tol, fmt.Sprintf("%s[%d]", path, i)); ok {
				return p, g, w, true
			}
		}
		return "", nil, nil, false
	case map[string]any:
		wv, ok := want.(map[string]any)
		if !ok {
			break
		}
		keys := make([]string, 0, len(gv)+len(wv))
		for k := range gv {
			keys = append(keys, k)
		}
		for k := range wv {
			if _, dup := gv[k]; !dup {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			g, hasG := gv[k]
			w, hasW := wv[k]
			if !hasG || !hasW {
				return path + "." + k, g, w, true
			}
			if p, g, w, ok := firstDiff(g, w, tol, path+"."+k); ok {
				return p, g, w, true
			}
		}
		return "", nil, nil, false
	}
	if equal(got, want, tol) {
		return "", nil, nil, false
	}
	return path, got, want, true
}

func brief(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
