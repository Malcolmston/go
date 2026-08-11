package rsc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/malcolmston/react"
)

// React encodes values that JSON cannot represent as strings with a "$" prefix,
// and escapes a genuine string starting with "$" by doubling it. These are the
// markers this encoder emits; the full set the client accepts is larger (server
// references, temporary references, blobs, typed arrays), and the ones not
// produced here are listed in the package's limitations.
const (
	markerUndefined = "$undefined"
	markerInfinity  = "$Infinity"
	markerNegInf    = "$-Infinity"
	markerNegZero   = "$-0"
	markerNaN       = "$NaN"

	// markerElement is the first entry of a serialized element tuple. The
	// client sees ["$", type, key, props] and reconstructs a React element.
	markerElement = "$"

	// prefixDate, prefixLazy and prefixSymbol introduce the encodings this
	// package emits with a payload after the marker character.
	prefixDate   = "$D"
	prefixLazy   = "$L"
	prefixSymbol = "$S"

	// isoMillis is JavaScript Date.toISOString: UTC, always three fractional
	// digits, always a literal Z.
	isoMillis = "2006-01-02T15:04:05.000Z"
)

// Undefined is the Go stand-in for JavaScript's undefined.
//
// Go has exactly one empty value where JavaScript has two, and the difference is
// visible across this boundary: React distinguishes an absent prop from one
// explicitly set to undefined, and `null` and `undefined` are different values
// on the client. Passing Undefined encodes JavaScript `undefined`; passing a Go
// nil encodes `null`.
type undefinedType struct{}

// Undefined encodes as JavaScript `undefined`. See [undefinedType].
var Undefined any = undefinedType{}

// Symbol encodes as JavaScript `Symbol.for(name)` — the form React uses for its
// own well-known element types, such as "react.suspense".
type Symbol string

// encodeString applies React's escape for a string that would otherwise be read
// as a marker. Any string beginning with "$" is sent with an extra "$", which
// the client strips.
func encodeString(s string) string {
	if len(s) > 0 && s[0] == '$' {
		return "$" + s
	}
	return s
}

// encodeFloat maps the IEEE values JSON cannot carry onto React's markers.
// Negative zero is included because JavaScript distinguishes it and a round
// trip through plain JSON would not.
func encodeFloat(f float64) any {
	switch {
	case math.IsNaN(f):
		return markerNaN
	case math.IsInf(f, 1):
		return markerInfinity
	case math.IsInf(f, -1):
		return markerNegInf
	case f == 0 && math.Signbit(f):
		return markerNegZero
	default:
		return f
	}
}

// encodeValue converts an arbitrary Go prop value into its Flight model form.
//
// Node-valued props are not handled here: they are rendered as part of the tree
// and reattached by the encoder, because rendering them anywhere else would cut
// them off from the context they were written inside. See [Encoder.clientModel].
func (e *Encoder) encodeValue(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case undefinedType:
		return markerUndefined, nil
	case Symbol:
		return prefixSymbol + string(t), nil
	case string:
		return encodeString(t), nil
	case bool:
		return t, nil
	case float64:
		return encodeFloat(t), nil
	case float32:
		return encodeFloat(float64(t)), nil
	case time.Time:
		// JavaScript's Date.toJSON always emits exactly three fractional digits
		// and a Z designator, and React sends that verbatim. RFC3339Nano would
		// drop trailing zeros and produce a different — still parseable, but not
		// byte-identical — string.
		return prefixDate + t.UTC().Format(isoMillis), nil
	case react.Props:
		return e.encodeMap(map[string]any(t))
	case map[string]any:
		return e.encodeMap(t)
	case *react.Element:
		return nil, fmt.Errorf("rsc: an element appears in a prop that the encoder did not "+
			"render; pass elements as children or as props of a client reference built with "+
			"ClientRef.El, which renders them in place (offending value: %T)", v)
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v, nil

	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			encoded, err := e.encodeValue(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			out[i] = encoded
		}
		return out, nil

	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("rsc: cannot encode a map with %s keys; "+
				"JavaScript object keys are strings", rv.Type().Key())
		}
		m := make(map[string]any, rv.Len())
		for _, k := range rv.MapKeys() {
			m[k.String()] = rv.MapIndex(k).Interface()
		}
		return e.encodeMap(m)

	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return nil, nil
		}
		return e.encodeValue(rv.Elem().Interface())

	case reflect.Struct:
		// Structs go through encoding/json so that field tags, omitempty and
		// custom marshalers all behave as their authors intended. The result is
		// re-decoded and re-encoded so nested markers still apply.
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("rsc: encoding %T: %w", v, err)
		}
		var generic any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return nil, fmt.Errorf("rsc: re-reading encoded %T: %w", v, err)
		}
		return e.encodeValue(generic)

	case reflect.Func, reflect.Chan:
		return nil, fmt.Errorf("rsc: cannot send a %s across the wire; "+
			"a function prop has to be a client-side concern — put the handler in the "+
			"client component, or use a server action", rv.Kind())
	}

	return nil, fmt.Errorf("rsc: cannot encode a value of type %T", v)
}

// encodeMap encodes an object's values, dropping the keys React strips itself.
func (e *Encoder) encodeMap(m map[string]any) (any, error) {
	out := make(map[string]any, len(m))
	for k, v := range m {
		encoded, err := e.encodeValue(v)
		if err != nil {
			return nil, fmt.Errorf("in property %q: %w", k, err)
		}
		out[k] = encoded
	}
	return out, nil
}

// marshal writes JSON the way React writes it: HTML characters are left alone.
//
// Go's encoder escapes <, > and & by default, which would still decode
// correctly but would not match React's own output byte for byte, and byte
// equality is what makes payloads comparable in tests. Callers embedding a
// payload directly into a <script> tag must escape it there, exactly as they
// would with React's.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode appends a newline; rows supply their own framing.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
