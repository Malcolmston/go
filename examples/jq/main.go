// Command jqdemo exercises github.com/malcolmston/jq: compiling and running a
// range of jq programs over in-memory JSON, plus the error-handling surface.
package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/malcolmston/jq"
)

const inputJSON = `{
  "org": "acme",
  "users": [
    {"name": "ada",   "age": 36, "tags": ["admin", "dev"],  "active": true,  "score": 91.5},
    {"name": "grace", "age": 45, "tags": ["dev"],           "active": true,  "score": 78.25},
    {"name": "alan",  "age": 41, "tags": ["ops", "dev"],    "active": false, "score": 64.0},
    {"name": "edsger","age": 52, "tags": ["arch"],          "active": true,  "score": 88.0}
  ],
  "limits": {"cpu": 4, "mem": 8192},
  "note": "  hello, JQ World  "
}`

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

// run compiles and runs program, printing each output as compact JSON.
func run(label, program string, input any) {
	q, err := jq.Compile(program)
	if err != nil {
		fmt.Printf("%-28s COMPILE ERROR: %v\n", label, err)
		return
	}
	out, err := q.Run(input)
	if err != nil {
		fmt.Printf("%-28s RUNTIME ERROR: %v\n", label, err)
		return
	}
	parts := make([]string, 0, len(out))
	for _, v := range out {
		b, mErr := jq.Marshal(v)
		if mErr != nil {
			parts = append(parts, "<marshal error: "+mErr.Error()+">")
			continue
		}
		parts = append(parts, string(b))
	}
	fmt.Printf("%-28s %s => %s\n", label, program, strings.Join(parts, " | "))
}

func main() {
	input, err := jq.Unmarshal([]byte(inputJSON))
	if err != nil {
		panic(err)
	}

	section("1. Unmarshal / Marshal round trip")
	back, err := jq.Marshal(input)
	if err != nil {
		panic(err)
	}
	fmt.Printf("re-marshalled %d bytes; keys sorted: %s\n", len(back), string(back[:40])+"...")
	fmt.Printf("top-level Go type: %T\n", input)

	section("2. Field access, indexing, slicing")
	run("identity-ish", `.org`, input)
	run("nested field", `.limits.mem`, input)
	run("array index", `.users[1].name`, input)
	run("negative index", `.users[-1].name`, input)
	run("array slice", `.users[1:3] | map(.name)`, input)
	run("optional missing", `.nope.deeper?`, input)
	run("has / in", `[.limits | has("cpu"), has("gpu")]`, input)

	section("3. Iteration, pipes, select, map")
	run("iterate names", `.users[].name`, input)
	run("select active", `[.users[] | select(.active) | .name]`, input)
	run("select + compare", `[.users[] | select(.age > 40) | .name]`, input)
	run("map_values", `.limits | map_values(. * 2)`, input)
	run("map + object build", `[.users[] | {n: .name, a: .age}] | length`, input)
	run("to_entries", `.limits | to_entries`, input)
	run("from_entries", `.limits | to_entries | from_entries`, input)
	run("with_entries", `.limits | with_entries(.value |= . + 1)`, input)

	section("4. Arithmetic and reduction builtins")
	run("add ages", `[.users[].age] | add`, input)
	run("length/min/max", `[.users[].score] | {n: length, min: min, max: max}`, input)
	run("floor/sqrt/pow", `[( .limits.mem | sqrt | floor ), pow(2; 10)]`, input)
	run("reduce", `reduce .users[].age as $a (0; . + $a)`, input)
	run("foreach running sum", `[foreach .users[].age as $a (0; . + $a)]`, input)
	run("division & modulo", `[(.limits.mem / .limits.cpu), (.limits.mem % 1000)]`, input)
	run("object merge (*)", `.limits * {mem: 1, disk: 100}`, input)
	run("array concat (+)", `.users[0].tags + .users[2].tags | unique`, input)
	run("array subtract (-)", `["a","b","c"] - ["b"]`, input)

	section("5. String operations")
	run("ascii_upcase", `.org | ascii_upcase`, input)
	run("ltrimstr/rtrimstr", `"prefix-body-suffix" | ltrimstr("prefix-") | rtrimstr("-suffix")`, input)
	run("split/join", `.note | split(",") | map(gsub("^\\s+|\\s+$"; "")) | join("|")`, input)
	run("interpolation", `.users[0] | "\(.name) is \(.age)"`, input)
	run("test/match regex", `[.users[].name | select(test("^a"))]`, input)
	run("capture", `"2026-08-10" | capture("(?<y>\\d{4})-(?<m>\\d{2})")`, input)
	run("sub with capture", `"a-b" | sub("(?<x>[a-z])-"; "[\(.x)]")`, input)
	run("@base64 / @base64d", `.org | @base64 | . + " -> " + (. | @base64d)`, input)
	run("@csv", `[.users[].name] | @csv`, input)
	run("tostring/tonumber", `["7" | tonumber, (7 | tostring)]`, input)
	run("explode/implode", `"jq" | explode | implode`, input)

	section("6. Sorting, grouping, flattening, paths")
	run("sort_by", `[.users[] | .name] | sort`, input)
	run("sort_by desc", `[.users | sort_by(-.score)[] | .name]`, input)
	run("group_by", `.users | group_by(.active) | map(length)`, input)
	run("unique_by", `[.users[].tags[]] | unique`, input)
	run("flatten", `[[1,[2,[3]]]] | flatten`, input)
	run("any/all", `[( .users | any(.active) ), ( .users | all(.active) )]`, input)
	run("paths", `.limits | [paths]`, input)
	run("getpath/setpath", `getpath(["limits","cpu"])`, input)
	run("del", `.limits | del(.mem)`, input)
	run("recurse leaves", `[.limits | recurse | numbers]`, input)
	run("walk", `.limits | walk(if type == "number" then . + 1 else . end)`, input)
	run("tostream", `.limits | [tostream]`, input)
	// HOLE: fromstream/1 and truncate_stream/2 are not implemented in this
	// release ("fromstream/1 is not defined"), so the usual
	// `fromstream(tostream)` round trip cannot be written. tostream itself
	// works, which makes the omission asymmetric.
	// run("tostream/fromstream", `.limits | fromstream(tostream)`, input)

	section("7. Type predicates, alternatives, try/catch")
	run("type", `[.org, .limits, .users, 1, true, null] | map(type)`, input)
	run("alternative //", `.missing // "default"`, input)
	run("try/catch", `try (1 / 0) catch "caught: \(.)"`, input)
	run("error object caught", `try error({code: 42}) catch .code`, input)
	run("if/elif/else", `.users[0].age | if . > 50 then "old" elif . > 30 then "mid" else "young" end`, input)
	run("label/break", `label $out | .users[] | if .name == "alan" then ., break $out else . end | .name`, input)
	run("limit/first/last", `[limit(2; .users[].name)] + [first(.users[].name)] + [last(.users[].name)]`, input)
	run("range", `[range(0; 10; 3)]`, input)
	run("user-defined func", `def double: . * 2; [.users[].age | double]`, input)
	run("def with args", `def clamp($lo; $hi): if . < $lo then $lo elif . > $hi then $hi else . end; [.users[].score | clamp(70; 90)]`, input)

	section("8. Variables via WithVariables")
	q := jq.MustCompile(`[.users[] | select(.age >= $minAge) | {name, org: $org}]`)
	out, err := q.WithVariables(map[string]any{
		"minAge": 45.0,
		"org":    "acme-inc",
	}).Run(input)
	if err != nil {
		fmt.Println("error:", err)
	} else {
		// Run returns one entry per emitted output; this program emits one array.
		b, _ := jq.Marshal(out[0])
		fmt.Printf("program: %s\noutputs: %d\nresult:  %s\n", q.String(), len(out), b)
	}

	section("9. Streaming with RunFunc and early stop")
	stop := errors.New("enough")
	seen := 0
	err = jq.MustCompile(`range(1; 1000000)`).RunFunc(input, func(v any) error {
		seen++
		if seen == 4 {
			return stop
		}
		fmt.Printf("  emitted %v\n", v)
		return nil
	})
	fmt.Printf("stopped after %d outputs, err is our sentinel: %v\n", seen, errors.Is(err, stop))

	section("10. Error handling")

	// Compile error: unbalanced parenthesis.
	if _, err := jq.Compile(`.users[] | select(.age >`); err != nil {
		var ce *jq.CompileError
		fmt.Printf("compile: %v\n  errors.Is(ErrCompile)=%v errors.Is(ErrSyntax)=%v as *CompileError=%v",
			err, errors.Is(err, jq.ErrCompile), errors.Is(err, jq.ErrSyntax), errors.As(err, &ce))
		if ce != nil {
			fmt.Printf(" offset=%d msg=%q", ce.Offset, ce.Msg)
		}
		fmt.Println()
	} else {
		fmt.Println("HOLE?: bad program compiled without error")
	}

	// Runtime type error.
	if _, err := jq.Eval(`.org + 1`, input); err != nil {
		var re *jq.RuntimeError
		fmt.Printf("runtime: %v\n  errors.Is(ErrRuntime)=%v errors.Is(ErrType)=%v as *RuntimeError=%v\n",
			err, errors.Is(err, jq.ErrRuntime), errors.Is(err, jq.ErrType), errors.As(err, &re))
	}

	// error/1 carrying a non-string value: RuntimeError.Value gives it back.
	if _, err := jq.Eval(`error({code: 7, msg: "boom"})`, nil); err != nil {
		var re *jq.RuntimeError
		if errors.As(err, &re) {
			v, ok := re.Value()
			b, _ := jq.Marshal(v)
			fmt.Printf("raised value: ok=%v value=%s (Msg=%q)\n", ok, b, re.Msg)
		}
	}

	// Undefined function: reported at run time as a *CompileError, per API-DEVIATIONS.
	if _, err := jq.Compile(`no_such_builtin`); err != nil {
		fmt.Printf("undefined at compile: %v\n", err)
	} else if _, err := jq.MustCompile(`no_such_builtin`).Run(input); err != nil {
		fmt.Printf("undefined at run: %v (Is ErrCompile=%v, Is ErrUndefined=%v)\n",
			err, errors.Is(err, jq.ErrCompile), errors.Is(err, jq.ErrUndefined))
	}

	// Step budget stops a non-terminating program instead of hanging.
	if _, err := jq.MustCompile(`def f: f; f`).WithMaxSteps(50_000).Run(nil); err != nil {
		fmt.Printf("runaway: Is(ErrStepLimit)=%v err=%v\n", errors.Is(err, jq.ErrStepLimit), err)
	} else {
		fmt.Println("HOLE?: runaway program returned without error")
	}

	// Strict JSON: jq's non-standard literals are rejected on input.
	if _, err := jq.Unmarshal([]byte(`{"x": NaN}`)); err != nil {
		fmt.Printf("Unmarshal(NaN) rejected: %v\n", err)
	}

	section("11. Marshal of special numbers")
	run("nan/infinite", `[(0/0? // "nan-raises"), (infinite | tostring), (nan | isnan)]`, input)
	run("large int", `9007199254740993`, input)
	run("number formatting", `[1.0, 1.5, 1e100, -0.0]`, input)

	section("12. Documented parity gaps (see API-DEVIATIONS.md)")
	// Object key order is not preserved: values are map[string]any, so
	// keys_unsorted is identical to keys. Real jq would answer ["b","a"].
	run("keys_unsorted == keys", `{"b":1,"a":2} | [keys_unsorted, keys]`, nil)
	// Regexes are RE2, so lookahead/backreferences are rejected.
	if _, err := jq.Eval(`"ab" | test("a(?=b)")`, nil); err != nil {
		fmt.Printf("RE2 lookahead rejected: %v\n", err)
	} else {
		fmt.Println("unexpected: RE2 accepted lookahead")
	}
	// input/inputs are stubs: there is no input stream behind the library.
	run("inputs yields nothing", `[inputs]`, nil)
	if _, err := jq.Eval(`input`, nil); err != nil {
		fmt.Printf("input stub: %v\n", err)
	}
	// $__loc__ and time builtins do work.
	run("$__loc__", `$__loc__`, nil)
	run("strftime", `1754784000 | strftime("%Y-%m-%dT%H:%M:%SZ")`, nil)
	run("fromdate/todate", `"2026-08-10T00:00:00Z" | fromdate | todate`, nil)
	// HOLE (undocumented): @base32 / @base32d are absent even though upstream
	// jq has both and API-DEVIATIONS.md's "not implemented" list does not
	// mention them. @base64/@base64d/@csv/@tsv/@sh/@uri/@html all work.
	if _, err := jq.Eval(`"hi" | @base32`, nil); err != nil {
		fmt.Printf("@base32 missing: %v\n", err)
	}
	// HOLE: an escaped "break $label" is reported as a syntax error naming the
	// mangled internal symbol, and never matches ErrBreak.
	if _, err := jq.Eval(`def f: break $out; label $out | f`, nil); err != nil {
		fmt.Printf("escaped break: %v (Is ErrBreak=%v)\n", err, errors.Is(err, jq.ErrBreak))
	}
	// HOLE: an out-of-scope break and an undefined $variable both compile
	// cleanly, contradicting Compile's own doc comment.
	if _, err := jq.Compile(`break $nope`); err == nil {
		fmt.Println("Compile(`break $nope`) returned no error; failure is deferred to Run")
	}
	if _, err := jq.Compile(`$undefinedVar`); err == nil {
		_, rerr := jq.Eval(`$undefinedVar`, nil)
		fmt.Printf("Compile(`$undefinedVar`) ok; Run says: %v\n", rerr)
	}
	// The module system is not implemented.
	if _, err := jq.Compile(`import "foo" as f; .`); err != nil {
		fmt.Printf("module system absent: %v\n", err)
	} else {
		fmt.Println("unexpected: import compiled")
	}

	fmt.Println("\nall sections complete")
}
