// Command run is the Go-side parity runner for github.com/malcolmston/yaml.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. An error or a panic inside a
// case becomes {"ok": false, "error": "..."} and the runner keeps reading, so a
// single bad case never costs the rest of the suite.
//
// A response may also carry a "detail" object. It is never compared -- the
// harness only logs it when a case mismatches -- and exists so an emitter case
// can show the text that failed to re-parse.
//
// Determinism: values are normalised into a form whose JSON encoding has its
// mapping keys sorted (encoding/json sorts map[string]any), and no case depends
// on wall-clock time or locale.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/malcolmston/yaml"
)

// isoMillis matches JavaScript's Date.prototype.toISOString: UTC, always three
// fractional digits. Both runners use it so that a timestamp compares by
// instant rather than by how the source happened to spell it.
const isoMillis = "2006-01-02T15:04:05.000Z"

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Value  any    `json:"value"`
	Detail any    `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ------------------------------------------------------------------ input

// inputBytes reads a case's document argument, which is either a JSON string
// (UTF-8 text) or {"hex": "..."} for a byte sequence that is not valid UTF-8
// and so cannot be written as a JSON string.
func inputBytes(raw json.RawMessage) ([]byte, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []byte(s), nil
	}
	var obj struct{ Hex string }
	if err := json.Unmarshal(raw, &obj); err != nil || obj.Hex == "" {
		return nil, fmt.Errorf(`input must be a string or {"hex": "..."}`)
	}
	return hex.DecodeString(obj.Hex)
}

// jsonValue decodes a JSON argument into a Go value, mapping integral numbers
// to int64 rather than float64 so that the emitter is asked to write "1" and
// not "1.0" -- matching what the JavaScript side does with the same literal.
func jsonValue(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return fixNumbers(v), nil
}

func fixNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	case []any:
		for i := range t {
			t[i] = fixNumbers(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = fixNumbers(t[k])
		}
		return t
	}
	return v
}

// ----------------------------------------------------------- normalisation

// normalise turns a decoded YAML value into a shape that can be compared
// against the JavaScript runner's answer through JSON:
//
//	NaN/Inf   -> ".nan" / ".inf" / "-.inf" (JSON has no such literals)
//	[]byte    -> {"!!binary": "<lowercase hex>"}
//	time.Time -> {"!!timestamp": "<ISO 8601, UTC, milliseconds>"}, tagged so that
//	             a timestamp is not silently equal to the plain string it was
//	mapping   -> map[string]any (encoding/json sorts the keys); a non-string
//	             key becomes the JSON text of its normalised form
func normalise(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case bool, string:
		return t
	case []byte:
		return map[string]any{"!!binary": hex.EncodeToString(t)}
	case time.Time:
		return map[string]any{"!!timestamp": t.UTC().Format(isoMillis)}
	case float32:
		return normaliseFloat(float64(t))
	case float64:
		return normaliseFloat(t)
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint()
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = normalise(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[normaliseKey(iter.Key().Interface())] = normalise(iter.Value().Interface())
		}
		return out
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return normalise(rv.Elem().Interface())
	}
	return fmt.Sprint(v)
}

func normaliseFloat(f float64) any {
	switch {
	case math.IsNaN(f):
		return ".nan"
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	}
	return f
}

// normaliseKey mirrors the JavaScript side: a string key is used as-is, any
// other key becomes the JSON text of its normalised value.
func normaliseKey(k any) string {
	if s, ok := k.(string); ok {
		return s
	}
	enc, err := json.Marshal(normalise(k))
	if err != nil {
		return fmt.Sprint(k)
	}
	return string(enc)
}

func same(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(x, y)
}

// ------------------------------------------------------------- parse helpers

func parseDocs(raw json.RawMessage) ([]*yaml.Node, error) {
	src, err := inputBytes(raw)
	if err != nil {
		return nil, err
	}
	return yaml.Parse(src)
}

func docValue(n *yaml.Node) (any, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return normalise(v), nil
}

// ------------------------------------------------------------------- the ops

type result struct {
	value  any
	detail any
}

func opParse(args []json.RawMessage) (result, error) {
	docs, err := parseDocs(args[0])
	if err != nil {
		return result{}, err
	}
	return result{value: map[string]any{"docs": len(docs)}}, nil
}

func opParseJSON(args []json.RawMessage) (result, error) {
	docs, err := parseDocs(args[0])
	if err != nil {
		return result{}, err
	}
	out := make([]any, 0, len(docs))
	for _, d := range docs {
		v, err := docValue(d)
		if err != nil {
			return result{}, err
		}
		out = append(out, v)
	}
	return result{value: out}, nil
}

func opReemit(args []json.RawMessage) (result, error) {
	docs, err := parseDocs(args[0])
	if err != nil {
		return result{}, err
	}
	if len(docs) == 0 {
		return result{}, fmt.Errorf("no documents to re-emit")
	}
	var times int
	if err := json.Unmarshal(args[1], &times); err != nil {
		return result{}, err
	}

	decodeOK := true
	base, err := docValue(docs[0])
	if err != nil {
		decodeOK = false
		base = nil
	}

	current := docs[0]
	var first, last []byte
	for i := 0; i < times; i++ {
		out, err := yaml.Marshal(current)
		if err != nil {
			return result{
				value:  emitOutcome(false, false, decodeOK, nil, nil, nil),
				detail: map[string]any{"round": i + 1, "error": err.Error()},
			}, nil
		}
		last = out
		if first == nil {
			first = out
		}
		next, err := yaml.Parse(out)
		if err != nil || len(next) == 0 {
			msg := "no documents"
			if err != nil {
				msg = err.Error()
			}
			return result{
				value:  emitOutcome(true, false, decodeOK, nil, nil, nil),
				detail: map[string]any{"round": i + 1, "emitted": string(out), "error": msg},
			}, nil
		}
		current = next[0]
	}

	final, err := docValue(current)
	if err != nil {
		decodeOK = false
		final = nil
	}

	var stable, value any
	if decodeOK {
		stable = same(base, final)
		value = final
	}
	grew := len(last) > len(first)

	return result{
		value:  emitOutcome(true, true, decodeOK, stable, grew, value),
		detail: map[string]any{"first": string(first), "last": string(last)},
	}, nil
}

func emitOutcome(emitOK, reparseOK, decodeOK bool, stable, grew, value any) map[string]any {
	return map[string]any{
		"emitOK":    emitOK,
		"reparseOK": reparseOK,
		"decodeOK":  decodeOK,
		"stable":    stable,
		"grew":      grew,
		"value":     value,
	}
}

func opMarshal(args []json.RawMessage) (result, error) {
	v, err := jsonValue(args[0])
	if err != nil {
		return result{}, err
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return result{}, err
	}
	docs, perr := yaml.Parse(out)
	if perr != nil || len(docs) == 0 {
		msg := "no documents"
		if perr != nil {
			msg = perr.Error()
		}
		return result{
			value:  map[string]any{"emitOK": true, "reparseOK": false, "stable": nil, "value": nil},
			detail: map[string]any{"emitted": string(out), "error": msg},
		}, nil
	}
	got, err := docValue(docs[0])
	if err != nil {
		return result{}, err
	}
	return result{
		value:  map[string]any{"emitOK": true, "reparseOK": true, "stable": same(got, normalise(v)), "value": got},
		detail: map[string]any{"emitted": string(out)},
	}, nil
}

func opComment(args []json.RawMessage) (result, error) {
	v, err := jsonValue(args[0])
	if err != nil {
		return result{}, err
	}
	var kind, text string
	if err := json.Unmarshal(args[1], &kind); err != nil {
		return result{}, err
	}
	if err := json.Unmarshal(args[2], &text); err != nil {
		return result{}, err
	}

	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return result{}, err
	}
	root := &n
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode || len(root.Content) != 2 {
		return result{}, fmt.Errorf("comment cases need a single-entry mapping")
	}
	target := root.Content[1]
	switch kind {
	case "line":
		target.LineComment = text
	case "head":
		target.HeadComment = text
	case "foot":
		// The JavaScript side has no per-node foot comment; its nearest
		// equivalent is a comment written after the document's contents, which
		// is what a foot comment on the root mapping means here.
		root.FootComment = text
	default:
		return result{}, fmt.Errorf("unknown comment kind: %s", kind)
	}

	out, err := yaml.Marshal(&n)
	if err != nil {
		return result{}, err
	}
	docs, perr := yaml.Parse(out)
	if perr != nil || len(docs) == 0 {
		msg := "no documents"
		if perr != nil {
			msg = perr.Error()
		}
		return result{
			value:  map[string]any{"emitOK": true, "reparseOK": false, "value": nil},
			detail: map[string]any{"emitted": string(out), "error": msg},
		}, nil
	}
	got, err := docValue(docs[0])
	if err != nil {
		return result{}, err
	}
	return result{
		value:  map[string]any{"emitOK": true, "reparseOK": true, "value": got},
		detail: map[string]any{"emitted": string(out)},
	}, nil
}

// opUnmarshal is the single-document path: yaml.Unmarshal rather than
// Parse + (*Node).Decode.
func opUnmarshal(args []json.RawMessage) (result, error) {
	src, err := inputBytes(args[0])
	if err != nil {
		return result{}, err
	}
	var v any
	if err := yaml.Unmarshal(src, &v); err != nil {
		return result{}, err
	}
	return result{value: normalise(v)}, nil
}

// opDecodeStream reads the whole stream through a Decoder, which is the API a
// caller uses for multi-document input.
func opDecodeStream(args []json.RawMessage) (result, error) {
	src, err := inputBytes(args[0])
	if err != nil {
		return result{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	out := []any{}
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return result{value: out}, nil
		}
		if err != nil {
			return result{}, err
		}
		out = append(out, normalise(v))
	}
}

// opEncodeStream writes every value as one document at the requested indent and
// reads the stream back, so Encoder, SetIndent and Close are all exercised.
func opEncodeStream(args []json.RawMessage) (result, error) {
	raw, err := jsonValue(args[0])
	if err != nil {
		return result{}, err
	}
	values, ok := raw.([]any)
	if !ok {
		return result{}, fmt.Errorf("encodeStream wants a list of values")
	}
	var indent int
	if err := json.Unmarshal(args[1], &indent); err != nil {
		return result{}, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	for _, v := range values {
		if err := enc.Encode(v); err != nil {
			return result{}, err
		}
	}
	if err := enc.Close(); err != nil {
		return result{}, err
	}

	docs, perr := yaml.Parse(buf.Bytes())
	if perr != nil {
		return result{
			value:  map[string]any{"emitOK": true, "reparseOK": false, "docs": nil, "stable": nil, "value": nil},
			detail: map[string]any{"emitted": buf.String(), "error": perr.Error()},
		}, nil
	}
	got := []any{}
	for _, d := range docs {
		v, err := docValue(d)
		if err != nil {
			return result{}, err
		}
		got = append(got, v)
	}
	want := make([]any, len(values))
	for i, v := range values {
		want[i] = normalise(v)
	}
	return result{
		value: map[string]any{
			"emitOK":    true,
			"reparseOK": true,
			"docs":      len(docs),
			"stable":    same(got, want),
			"value":     got,
		},
		detail: map[string]any{"emitted": buf.String()},
	}, nil
}

func opWellFormedUTF8(args []json.RawMessage) (result, error) {
	src, err := inputBytes(args[0])
	if err != nil {
		return result{}, err
	}
	return result{value: map[string]any{"wellFormed": utf8.Valid(src)}}, nil
}

var ops = map[string]func([]json.RawMessage) (result, error){
	"decodeStream":   opDecodeStream,
	"encodeStream":   opEncodeStream,
	"parse":          opParse,
	"parseJSON":      opParseJSON,
	"reemit":         opReemit,
	"unmarshal":      opUnmarshal,
	"marshal":        opMarshal,
	"comment":        opComment,
	"wellFormedUTF8": opWellFormedUTF8,
}

func opNames() []string {
	names := make([]string, 0, len(ops))
	for k := range ops {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------- the loop

// dispatch turns a panic inside the library into an ordinary failing case.
func dispatch(req request) (res result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	op, ok := ops[req.Fn]
	if !ok {
		return result{}, fmt.Errorf("unknown fn: %s (have %v)", req.Fn, opNames())
	}
	return op(req.Args)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := bytes.TrimSpace(in.Bytes())
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			emit(out, response{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			continue
		}

		res, err := dispatch(req)
		if err != nil {
			emit(out, response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(out, response{ID: req.ID, OK: true, Value: res.value, Detail: res.detail})
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
	}
	_ = out.Flush()
}

func emit(out *bufio.Writer, res response) {
	enc, err := json.Marshal(res)
	if err != nil {
		// A value that cannot be encoded is a failing case, not a dead runner.
		enc, _ = json.Marshal(response{ID: res.ID, OK: false, Error: "encode value: " + err.Error()})
	}
	out.Write(enc)
	out.WriteByte('\n')
	_ = out.Flush()
}
