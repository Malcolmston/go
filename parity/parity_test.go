package parity

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanonicalMatchesDriverEncoding(t *testing.T) {
	type inner struct {
		N int    `json:"n"`
		S string `json:"s,omitempty"`
	}
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "hi", `"hi"`},
		{"int", 42, `42`},
		{"float", 1.5, `1.5`},
		{"nan", math.NaN(), `"NaN"`},
		{"posinf", math.Inf(1), `"Infinity"`},
		{"neginf", math.Inf(-1), `"-Infinity"`},
		{"bytes", []byte("hi"), `"aGk="`},
		{"nilslice", []int(nil), `null`},
		{"slice", []int{1, 2}, `[1,2]`},
		{"map", map[string]int{"a": 1}, `{"a":1}`},
		{"struct", inner{N: 3}, `{"n":3}`},
		{"pointer", &inner{N: 4, S: "x"}, `{"n":4,"s":"x"}`},
		{"error", errors.New("boom"), `{"$error":"boom"}`},
		{"time", time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), `"2026-07-24T12:00:00Z"`},
		{"nil", nil, `null`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := canonical(c.in)
			if err != nil {
				t.Fatalf("canonical(%v): %v", c.in, err)
			}
			if string(got) != c.want {
				t.Errorf("canonical(%v) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestEqualTolerance(t *testing.T) {
	if !equal(decode([]byte(`0.1`)), decode([]byte(`0.10000000001`)), 1e-9) {
		t.Error("floats within tolerance should compare equal")
	}
	if equal(decode([]byte(`0.1`)), decode([]byte(`0.2`)), 1e-9) {
		t.Error("floats outside tolerance should differ")
	}
	if !equal(decode([]byte(`1e12`)), decode([]byte(`1.0000000000001e12`)), 1e-9) {
		t.Error("tolerance should scale with magnitude")
	}
	if !equal(decode([]byte(`{"a":[1,"x"]}`)), decode([]byte(`{"a":[1,"x"]}`)), 0) {
		t.Error("identical nested values should compare equal")
	}
	if equal(decode([]byte(`{"a":1}`)), decode([]byte(`{"a":1,"b":2}`)), 0) {
		t.Error("an extra key is a difference")
	}
}

func TestDiffNamesTheFirstDifference(t *testing.T) {
	got := decode([]byte(`{"a":{"b":[1,2,3]}}`))
	want := decode([]byte(`{"a":{"b":[1,9,3]}}`))
	if d := diff(got, want, 0); !strings.Contains(d, ".a.b[1]") || !strings.Contains(d, "upstream=9") {
		t.Errorf("diff = %q, want it to point at .a.b[1] and upstream=9", d)
	}
}

func TestPercentFloors(t *testing.T) {
	for _, c := range []struct {
		n, total int
		want     string
	}{
		{10, 10, "100%"}, {99, 100, "99%"}, {999, 1000, "99%"}, {0, 10, "0%"}, {1, 0, "n/a"},
	} {
		if got := percent(c.n, c.total); got != c.want {
			t.Errorf("percent(%d,%d) = %s, want %s", c.n, c.total, got, c.want)
		}
	}
}

func TestBuildScoresAgainstBaseline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PARITY_DIR", dir)
	base := Report{Failing: []string{"S/two", "S/three"}}
	b, _ := json.Marshal(base)
	if err := os.WriteFile(filepath.Join(dir, "baseline"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	rep := build([]fragment{{Repo: "demo", Suite: SuiteReport{
		Name:   "S",
		Mode:   ModeLive,
		Counts: Counts{Total: 4, Matched: 3, Mismatched: 1},
		Names:  []string{"S/one", "S/two", "S/three", "S/four"},
		// three still differs; two was fixed since the baseline.
		Failing: []string{"S/three"},
	}}})
	if rep.ParityAfter != "75%" {
		t.Errorf("after = %s, want 75%%", rep.ParityAfter)
	}
	if rep.ParityBefore != "50%" {
		t.Errorf("before = %s, want 50%% (two of four were failing)", rep.ParityBefore)
	}
	if rep.GapsClosed != 1 {
		t.Errorf("gapsClosed = %d, want 1", rep.GapsClosed)
	}
	if len(rep.Regressions) != 0 {
		t.Errorf("regressions = %v, want none", rep.Regressions)
	}
	if rep.Baseline != "measured" {
		t.Errorf("baseline = %q, want measured", rep.Baseline)
	}
}

func TestBuildFlagsRegressions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PARITY_DIR", dir)
	b, _ := json.Marshal(Report{Failing: []string{}})
	os.WriteFile(filepath.Join(dir, "baseline"), b, 0o644)
	rep := build([]fragment{{Repo: "demo", Suite: SuiteReport{
		Name: "S", Mode: ModeLive,
		Counts:  Counts{Total: 2, Matched: 1, Mismatched: 1},
		Names:   []string{"S/one", "S/two"},
		Failing: []string{"S/two"},
	}}})
	if len(rep.Regressions) != 1 || rep.Regressions[0] != "S/two" {
		t.Errorf("regressions = %v, want [S/two]", rep.Regressions)
	}
}

func TestSameRunDistinguishesInvocations(t *testing.T) {
	now := time.Now().Unix()
	cur := fmt.Sprintf("123 %d", now)
	if !sameRun(fmt.Sprintf("123 %d", now-60), cur) {
		t.Error("the same parent process, seconds ago, is the same run")
	}
	if sameRun(fmt.Sprintf("124 %d", now-60), cur) {
		t.Error("a different parent process is a different run")
	}
	if sameRun(fmt.Sprintf("123 %d", now-24*3600), cur) {
		t.Error("a day-old stamp is a recycled pid, not a continuation")
	}
	if sameRun("nonsense", cur) {
		t.Error("an unreadable stamp is not the current run")
	}
}

// TestParitySelf is the harness auditing itself: it runs a real Node process,
// asks it questions, and compares the answers to Go's. It exercises the exact
// path a port's suite takes — including a deliberate mismatch, which is checked
// through the outcome map rather than by failing the test.
func TestParitySelf(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	t.Setenv("PARITY_DIR", filepath.Join(dir, "frag"))
	t.Setenv("PARITY_OUT", filepath.Join(dir, "parity.json"))
	t.Setenv("PARITY_CROSS", "0") // covered by TestCrossCompile

	s := New(t, Config{
		Repo:      "parity",
		Upstream:  "nodejs/node",
		Name:      "self",
		Oracle:    Node(""), // node's own builtins: nothing to install
		GoldenDir: filepath.Join(dir, "golden"),
	})
	s.Case("upper", Case{
		Upstream: `return input.toUpperCase()`,
		Input:    "höla wörld",
		Go:       func() (any, error) { return strings.ToUpper("höla wörld"), nil },
	})
	s.Case("json-roundtrip", Case{
		Upstream: `return JSON.parse(input)`,
		Input:    `{"b":2,"a":[1,null,true]}`,
		Go: func() (any, error) {
			var v any
			return v, json.Unmarshal([]byte(`{"b":2,"a":[1,null,true]}`), &v)
		},
	})
	s.Case("throws", Case{
		Upstream: `JSON.parse("{oops");`,
		Go:       func() (any, error) { return nil, errors.New("invalid character 'o'") },
	})
	s.Case("known-gap", Case{
		Upstream: `return 1`,
		Go:       func() (any, error) { return 1, nil },
		Skip:     "demonstrates a recorded, unhidden gap",
	})
	s.Run()

	want := map[string]string{
		"upper":          outcomeMatched,
		"json-roundtrip": outcomeMatched,
		"throws":         outcomeMatched,
		"known-gap":      outcomeSkipped,
	}
	for name, w := range want {
		if got := s.outcomes[name]; got != w {
			t.Errorf("case %q outcome = %q, want %q", name, got, w)
		}
	}

	b, err := os.ReadFile(filepath.Join(dir, "parity.json"))
	if err != nil {
		t.Fatalf("no report written: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if rep.Mode != ModeLive {
		t.Errorf("mode = %q, want live — the point is to run the original code", rep.Mode)
	}
	if rep.Cases.Matched != 3 || rep.Cases.Skipped != 1 || rep.Cases.Mismatched != 0 {
		t.Errorf("counts = %+v, want 3 matched / 1 skipped / 0 mismatched", rep.Cases)
	}
	if rep.ParityAfter != "100%" {
		t.Errorf("after = %s, want 100%%", rep.ParityAfter)
	}
	if rep.Oracle.Runtime != "node" || rep.Oracle.Version == "" {
		t.Errorf("oracle = %+v, want a node version", rep.Oracle)
	}
}

func TestCrossCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiling is slow")
	}
	res := crossCompile("./...", []Platform{{"linux", "amd64"}, {"windows", "amd64"}})
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2", len(res))
	}
	for _, p := range res {
		if !p.OK {
			t.Errorf("parity does not build for %s/%s: %s", p.GOOS, p.GOARCH, p.Error)
		}
	}
}
