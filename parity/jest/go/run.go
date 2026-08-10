// Go runner for the jest parity harness.
//
// Speaks the JSON-Lines contract from parity/HARNESS.md: one case object per
// line on stdin, one result object per line on stdout, same order.
//
// The comparable artefact is a single boolean: did the assertion hold?
//
//	{"id":..., "ok":true, "value":{"pass":true|false}}
//
// github.com/malcolmston/jest reports assertion failures through a
// TestReporter rather than by panicking, so this runner passes a recording
// reporter and reads the outcome back off it. The port funnels two different
// things through Errorf:
//
//   - "assertion failed: expected value ..."  -> the assertion did not hold
//     (pass:false), the analogue of a JestAssertionError.
//   - "assertion failed: <Matcher> requires ..." (and friends) -> the matcher
//     was handed a value it cannot work with, the analogue of expect throwing a
//     plain Error. Reported as ok:false.
//
// Panics are recovered and reported as ok:false so one bad case never kills the
// runner.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/malcolmston/jest"
)

// ---------------------------------------------------------------------------
// wire types
// ---------------------------------------------------------------------------

type caseIn struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Not  bool              `json:"not"`
	Args []json.RawMessage `json:"args"`
	// Actual is the value under test.
	Actual json.RawMessage `json:"actual"`
}

type result struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value *value `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

type value struct {
	Pass bool `json:"pass"`
}

// ---------------------------------------------------------------------------
// recording reporter
// ---------------------------------------------------------------------------

type reporter struct{ msgs []string }

func (r *reporter) Errorf(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}
func (r *reporter) Fatalf(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}
func (r *reporter) Helper() {}

const assertionPrefix = "assertion failed: expected value"

// outcome classifies what the reporter recorded.
func (r *reporter) outcome() (pass bool, usageErr string) {
	if len(r.msgs) == 0 {
		return true, ""
	}
	for _, m := range r.msgs {
		if !strings.HasPrefix(m, assertionPrefix) {
			return false, firstLine(m)
		}
	}
	return false, ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ---------------------------------------------------------------------------
// value specs
// ---------------------------------------------------------------------------

// unsupported marks a call shape the Go port cannot express at all.
type unsupported struct{ why string }

func (u unsupported) Error() string { return "unsupported: " + u.why }

// absent is the sentinel for a key that must not be present at all.
type absentT struct{}

var absent = absentT{}

// jsTypeError stands in for JS's TypeError: a distinct concrete error type.
type jsTypeError struct{ msg string }

func (e *jsTypeError) Error() string { return e.msg }

// jsRangeError stands in for JS's RangeError.
type jsRangeError struct{ msg string }

func (e *jsRangeError) Error() string { return e.msg }

func decodeSpec(raw json.RawMessage) (any, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return convert(v)
}

// convert turns the generic JSON tree into the Go value the spec describes.
// Plain JSON numbers become float64, mirroring the fact that every JavaScript
// number is a float64; {"$":"int"} asks for a Go int explicitly.
func convert(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case bool, string:
		return t, nil
	case json.Number:
		f, err := t.Float64()
		return f, err
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			c, err := convert(e)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, nil
	case map[string]any:
		tag, ok := t["$"]
		if !ok {
			return convertPlainObject(t)
		}
		s, _ := tag.(string)
		return convertTagged(s, t)
	default:
		return nil, fmt.Errorf("unhandled json value %T", v)
	}
}

func convertPlainObject(t map[string]any) (any, error) {
	out := map[string]any{}
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c, err := convert(t[k])
		if err != nil {
			return nil, err
		}
		if _, skip := c.(absentT); skip {
			continue
		}
		out[k] = c
	}
	return out, nil
}

func num(t map[string]any, key string) (float64, bool) {
	n, ok := t[key].(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

func str(t map[string]any, key string) string {
	s, _ := t[key].(string)
	return s
}

func entries(t map[string]any) ([][2]any, error) {
	raw, _ := t["entries"].([]any)
	out := make([][2]any, 0, len(raw))
	for _, e := range raw {
		pair, ok := e.([]any)
		if !ok || len(pair) != 2 {
			return nil, errors.New("bad entries pair")
		}
		k, err := convert(pair[0])
		if err != nil {
			return nil, err
		}
		val, err := convert(pair[1])
		if err != nil {
			return nil, err
		}
		out = append(out, [2]any{k, val})
	}
	return out, nil
}

func entriesToMap(t map[string]any) (map[string]any, error) {
	es, err := entries(t)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, e := range es {
		if _, skip := e[1].(absentT); skip {
			continue
		}
		out[fmt.Sprintf("%v", e[0])] = e[1]
	}
	return out, nil
}

func values(t map[string]any) ([]any, error) {
	raw, _ := t["values"].([]any)
	out := make([]any, 0, len(raw))
	for _, e := range raw {
		c, err := convert(e)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// classSample returns the Go value whose type stands in for a JS constructor.
// The port's Any/ToBeInstanceOf take a sample value, not a class, so a JS
// constructor can only be approximated by an instance of the nearest Go type.
func classSample(name string) (any, error) {
	switch name {
	case "String":
		return "", nil
	case "Number":
		return float64(0), nil
	case "Boolean":
		return false, nil
	case "Object":
		return map[string]any{}, nil
	case "Array":
		return []any{}, nil
	case "Function":
		return func() {}, nil
	case "Error":
		return errors.New(""), nil
	case "TypeError":
		return &jsTypeError{}, nil
	case "RangeError":
		return &jsRangeError{}, nil
	case "Date":
		return time.Time{}, nil
	case "RegExp":
		return regexp.MustCompile(""), nil
	case "Map":
		return map[any]any{}, nil
	case "Set":
		return map[any]struct{}{}, nil
	default:
		return nil, fmt.Errorf("unknown class: %s", name)
	}
}

func makeError(ctor, msg string) (error, error) {
	switch ctor {
	case "", "Error":
		return errors.New(msg), nil
	case "TypeError":
		return &jsTypeError{msg}, nil
	case "RangeError":
		return &jsRangeError{msg}, nil
	default:
		return nil, fmt.Errorf("unknown error ctor: %s", ctor)
	}
}

func convertTagged(tag string, t map[string]any) (any, error) {
	switch tag {
	case "undefined", "null":
		// Go has a single nil; JS's null/undefined split is not expressible.
		return nil, nil
	case "absent":
		return absent, nil
	case "nan":
		return math.NaN(), nil
	case "negzero":
		return math.Copysign(0, -1), nil
	case "inf":
		return math.Inf(1), nil
	case "ninf":
		return math.Inf(-1), nil
	case "int":
		f, ok := num(t, "v")
		if !ok {
			return nil, errors.New("int spec needs numeric v")
		}
		return int(f), nil
	case "obj":
		return entriesToMap(t)
	case "map":
		es, err := entries(t)
		if err != nil {
			return nil, err
		}
		out := map[any]any{}
		for _, e := range es {
			out[e[0]] = e[1]
		}
		return out, nil
	case "set":
		vs, err := values(t)
		if err != nil {
			return nil, err
		}
		out := map[any]struct{}{}
		for _, v := range vs {
			out[v] = struct{}{}
		}
		return out, nil
	case "date":
		return time.Parse(time.RFC3339Nano, str(t, "iso"))
	case "regexp":
		return regexp.Compile(str(t, "source"))
	case "error":
		return makeError(str(t, "ctor"), str(t, "message"))
	case "class":
		return classSample(str(t, "name"))
	case "sparse":
		f, _ := num(t, "length")
		out := make([]any, int(f))
		if vals, ok := t["values"].(map[string]any); ok {
			keys := make([]string, 0, len(vals))
			for k := range vals {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				var idx int
				if _, err := fmt.Sscanf(k, "%d", &idx); err != nil || idx < 0 || idx >= len(out) {
					return nil, fmt.Errorf("bad sparse index %q", k)
				}
				c, err := convert(vals[k])
				if err != nil {
					return nil, err
				}
				out[idx] = c
			}
		}
		return out, nil
	case "cyclic":
		m, err := entriesToMap(t)
		if err != nil {
			return nil, err
		}
		key := str(t, "selfKey")
		if key == "" {
			key = "self"
		}
		m[key] = m
		return m, nil
	case "fn":
		if r, ok := t["returns"]; ok {
			c, err := convert(r)
			if err != nil {
				return nil, err
			}
			return func() any { return c }, nil
		}
		return func() {}, nil
	case "throws":
		ctor := str(t, "ctor")
		if ctor == "" {
			ctor = "Error"
		}
		msg := str(t, "message")
		switch ctor {
		case "none":
			return func() {}, nil
		case "string":
			return func() { panic(msg) }, nil
		default:
			e, err := makeError(ctor, msg)
			if err != nil {
				return nil, err
			}
			return func() { panic(e) }, nil
		}
	case "mock":
		return buildMock(t)
	// ---- asymmetric matchers ----
	case "any":
		s, err := classSample(str(t, "type"))
		if err != nil {
			return nil, err
		}
		return jest.Any(s), nil
	case "anything":
		return jest.Anything(), nil
	case "arrayContaining", "notArrayContaining":
		vs, err := values(t)
		if err != nil {
			return nil, err
		}
		if tag == "notArrayContaining" {
			return jest.NotArrayContaining(vs...), nil
		}
		return jest.ArrayContaining(vs...), nil
	case "objectContaining", "notObjectContaining":
		m, err := entriesToMap(t)
		if err != nil {
			return nil, err
		}
		if tag == "notObjectContaining" {
			return jest.NotObjectContaining(m), nil
		}
		return jest.ObjectContaining(m), nil
	case "stringContaining":
		return jest.StringContaining(str(t, "value")), nil
	case "notStringContaining":
		return jest.NotStringContaining(str(t, "value")), nil
	case "stringMatching":
		return jest.StringMatching(str(t, "value")), nil
	case "notStringMatching":
		return jest.NotStringMatching(str(t, "value")), nil
	case "closeTo", "notCloseTo":
		f, ok := num(t, "value")
		if !ok {
			return nil, errors.New("closeTo needs numeric value")
		}
		var prec []int
		if p, ok := num(t, "precision"); ok {
			prec = append(prec, int(p))
		}
		if tag == "notCloseTo" {
			return nil, unsupported{"jest has no Not(CloseTo(...)) constructor"}
		}
		return jest.CloseTo(f, prec...), nil
	default:
		return nil, fmt.Errorf("unknown spec tag: %s", tag)
	}
}

// buildMock creates a jest.Mock, primes its return value and replays the
// recorded call list.
func buildMock(t map[string]any) (any, error) {
	name := str(t, "name")
	if name == "" {
		name = "mock"
	}
	m := jest.NewMock(name)
	if r, ok := t["returns"]; ok {
		c, err := convert(r)
		if err != nil {
			return nil, err
		}
		m.Return(c)
	}
	if msg := str(t, "throwsOnCall"); msg != "" {
		m.MockImplementation(func(args ...any) []any { panic(errors.New(msg)) })
	}
	calls, _ := t["calls"].([]any)
	for _, call := range calls {
		argSpecs, ok := call.([]any)
		if !ok {
			return nil, errors.New("bad mock call list")
		}
		args := make([]any, 0, len(argSpecs))
		for _, a := range argSpecs {
			c, err := convert(a)
			if err != nil {
				return nil, err
			}
			args = append(args, c)
		}
		safeCall(m, args)
	}
	return m, nil
}

func safeCall(m *jest.Mock, args []any) {
	defer func() { _ = recover() }()
	m.Call(args...)
}

// ---------------------------------------------------------------------------
// dispatch
// ---------------------------------------------------------------------------

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	}
	return 0, false
}

func asInt(v any) (int, bool) {
	f, ok := asFloat(v)
	return int(f), ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// throwArg maps the optional argument of toThrow onto what the Go port accepts,
// which is only an optional substring.
func throwArg(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case *regexp.Regexp:
		// The port has no regexp form; passing the source is the closest call
		// available and the case is marked as a deviation.
		return []string{t.String()}, nil
	case error:
		return nil, unsupported{"ToThrow does not accept an error value, only a substring"}
	default:
		if v == nil {
			return nil, nil
		}
		return nil, unsupported{fmt.Sprintf("ToThrow does not accept %T", v)}
	}
}

// dispatch runs one matcher. It returns an error only for call shapes the port
// cannot express; assertion outcomes come back through the reporter.
func dispatch(m *jest.Matcher[any], fn string, args []any) error {
	arg := func(i int) any {
		if i < len(args) {
			return args[i]
		}
		return nil
	}
	needStr := func(i int) (string, error) {
		s, ok := asString(arg(i))
		if !ok {
			return "", unsupported{fmt.Sprintf("%s requires a string argument, got %T", fn, arg(i))}
		}
		return s, nil
	}
	needInt := func(i int) (int, error) {
		n, ok := asInt(arg(i))
		if !ok {
			return 0, unsupported{fmt.Sprintf("%s requires a numeric argument, got %T", fn, arg(i))}
		}
		return n, nil
	}

	switch fn {
	case "toBe":
		m.ToBe(arg(0))
	case "toEqual":
		m.ToEqual(arg(0))
	case "toStrictEqual":
		m.ToStrictEqual(arg(0))
	case "toBeTruthy":
		m.ToBeTruthy()
	case "toBeFalsy":
		m.ToBeFalsy()
	case "toBeNull":
		m.ToBeNull()
	case "toBeUndefined":
		m.ToBeUndefined()
	case "toBeDefined":
		m.ToBeDefined()
	case "toBeNaN":
		m.ToBeNaN()
	case "toBeGreaterThan":
		m.ToBeGreaterThan(arg(0))
	case "toBeGreaterThanOrEqual":
		m.ToBeGreaterThanOrEqual(arg(0))
	case "toBeLessThan":
		m.ToBeLessThan(arg(0))
	case "toBeLessThanOrEqual":
		m.ToBeLessThanOrEqual(arg(0))
	case "toBeCloseTo":
		f, ok := asFloat(arg(0))
		if !ok {
			return unsupported{"ToBeCloseTo requires a numeric expected value"}
		}
		if len(args) > 1 {
			// JS takes a digit count; the port takes an absolute epsilon. Map
			// precision p onto the epsilon JS itself uses, 10^-p / 2.
			p, err := needInt(1)
			if err != nil {
				return err
			}
			m.ToBeCloseTo(f, math.Pow(10, -float64(p))/2)
		} else {
			m.ToBeCloseTo(f)
		}
	case "toMatch":
		switch t := arg(0).(type) {
		case string:
			m.ToMatch(t)
		case *regexp.Regexp:
			m.ToMatch(t.String())
		default:
			return unsupported{fmt.Sprintf("ToMatch does not accept %T", arg(0))}
		}
	case "toContain":
		m.ToContain(arg(0))
	case "toContainEqual":
		m.ToContainEqual(arg(0))
	case "toHaveLength":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToHaveLength(n)
	case "toMatchObject":
		m.ToMatchObject(arg(0))
	case "toHaveProperty":
		p, err := needStr(0)
		if err != nil {
			return err
		}
		if len(args) > 1 {
			m.ToHaveProperty(p, arg(1))
		} else {
			m.ToHaveProperty(p)
		}
	case "toBeInstanceOf":
		m.ToBeInstanceOf(arg(0))
	case "toThrow", "toThrowError":
		a, err := throwArg(arg(0))
		if err != nil {
			return err
		}
		m.ToThrow(a...)
	case "toBeOneOf":
		m.ToBeOneOf(args...)

	// ---- mock call matchers ----
	case "toHaveBeenCalled":
		m.ToHaveBeenCalled()
	case "toBeCalled":
		m.ToBeCalled()
	case "toHaveBeenCalledTimes":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToHaveBeenCalledTimes(n)
	case "toBeCalledTimes":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToBeCalledTimes(n)
	case "toHaveBeenCalledWith":
		m.ToHaveBeenCalledWith(args...)
	case "toBeCalledWith":
		m.ToBeCalledWith(args...)
	case "toHaveBeenLastCalledWith":
		m.ToHaveBeenLastCalledWith(args...)
	case "lastCalledWith":
		m.LastCalledWith(args...)
	case "toHaveBeenNthCalledWith":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToHaveBeenNthCalledWith(n, args[1:]...)
	case "nthCalledWith":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.NthCalledWith(n, args[1:]...)
	case "toHaveReturned":
		m.ToHaveReturned()
	case "toReturn":
		m.ToReturn()
	case "toHaveReturnedTimes":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToHaveReturnedTimes(n)
	case "toHaveReturnedWith":
		m.ToHaveReturnedWith(arg(0))
	case "toReturnWith":
		m.ToReturnWith(arg(0))
	case "toHaveLastReturnedWith":
		m.ToHaveLastReturnedWith(arg(0))
	case "toHaveNthReturnedWith":
		n, err := needInt(0)
		if err != nil {
			return err
		}
		m.ToHaveNthReturnedWith(n, arg(1))

	// ---- matchers the port does not have ----
	case "toReturnTimes", "lastReturnedWith", "nthReturnedWith":
		return unsupported{"the port has no " + fn + " alias"}
	case "resolves", "rejects":
		return unsupported{"the port has no promise modifiers"}
	case "toMatchSnapshot", "toMatchInlineSnapshot", "toThrowErrorMatchingSnapshot",
		"toThrowErrorMatchingInlineSnapshot":
		return unsupported{"snapshot matchers are out of scope for this harness"}
	default:
		return fmt.Errorf("no such matcher: %s", fn)
	}
	return nil
}

func runCase(c caseIn) (res result) {
	res.ID = c.ID
	defer func() {
		if r := recover(); r != nil {
			res = result{ID: c.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	actual, err := decodeSpec(c.Actual)
	if err != nil {
		return result{ID: c.ID, OK: false, Error: "decode actual: " + err.Error()}
	}
	args := make([]any, 0, len(c.Args))
	for i, raw := range c.Args {
		a, err := decodeSpec(raw)
		if err != nil {
			return result{ID: c.ID, OK: false, Error: fmt.Sprintf("decode arg %d: %v", i, err)}
		}
		if u, ok := a.(unsupported); ok {
			return result{ID: c.ID, OK: false, Error: u.Error()}
		}
		args = append(args, a)
	}

	rep := &reporter{}
	m := jest.Expect[any](rep, actual)
	if c.Not {
		m = m.Not()
	}
	if err := dispatch(m, c.Fn, args); err != nil {
		return result{ID: c.ID, OK: false, Error: err.Error()}
	}
	pass, usage := rep.outcome()
	if usage != "" {
		return result{ID: c.ID, OK: false, Error: usage}
	}
	return result{ID: c.ID, OK: true, Value: &value{Pass: pass}}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var c caseIn
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			_ = enc.Encode(result{ID: "?", OK: false, Error: "bad json: " + err.Error()})
			_ = out.Flush()
			continue
		}
		_ = enc.Encode(runCase(c))
		_ = out.Flush()
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
}
