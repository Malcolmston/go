// Package parity drives the real `jq` binary and the Go port over the same JSON
// case files and compares their answers.
//
// Upstream is the oracle: nothing here hand-writes an expected value. Every case
// is a (filter program, input JSON) pair, optionally with `--argjson`-style
// variables, and the compared answer is the **complete output stream** of the
// filter as a JSON array. jq filters yield zero, one or many values, so a port
// that collapses a many-valued stream to a single value has to show up as a
// mismatch — comparing only the first value would hide exactly the class of bug
// that matters most.
//
// Run with:  GOWORK=off go test ./parity/jq/
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	caseTimeout = 60 * time.Second

	// pinnedJQ is the upstream oracle version the case files are pinned to. An
	// unpinned oracle makes the score unreproducible, and jq 1.8 changed several
	// behaviours (trim, reverse on strings, limit with a negative count), so the
	// harness insists on the pinned major.minor.patch.
	pinnedJQ       = "jq-1.7.1"
	pinnedUpstream = "jq@1.7.1"

	goModulePath = "github.com/malcolmston/jq"
)

// ------------------------------------------------------------------ case files

type caseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
	Encoding string  `json:"encoding"`
	Cases    []tcase `json:"cases"`
}

type tcase struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

func loadCases(t *testing.T) []tcase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no case files in cases/")
	}
	var all []tcase
	seen := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if cf.Upstream != pinnedUpstream {
			t.Fatalf("%s pins upstream %q, want %q", p, cf.Upstream, pinnedUpstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q in %s and %s", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ---------------------------------------------------------------------- runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin *bufio.Writer
	lines chan string
	errs  chan error
	once  sync.Once
}

func startRunner(t *testing.T, name string, bin string, args ...string) *runner {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	// Neither runner may inherit the developer's environment: `env`, `$ENV`,
	// `now` and the strftime family must see the same fixed view on both sides.
	// Each runner installs its own fixed environment internally; this keeps the
	// process that does the installing quiet too.
	cmd.Env = []string{"TZ=UTC", "LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin pipe: %v", name, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", name, err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{
		name:  name,
		cmd:   cmd,
		stdin: bufio.NewWriter(in),
		lines: make(chan string, 4),
		errs:  make(chan error, 1),
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<26)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
		close(r.lines)
	}()
	t.Cleanup(func() { r.stop() })
	return r
}

func (r *runner) stop() {
	r.once.Do(func() {
		r.stdin.Flush()
		if p, ok := r.cmd.Stdin.(interface{ Close() error }); ok {
			_ = p.Close()
		}
		if r.cmd.Process != nil {
			done := make(chan struct{})
			go func() { _ = r.cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = r.cmd.Process.Kill()
			}
		}
	})
}

// call sends one case and waits for exactly one reply, with a per-case timeout.
func (r *runner) call(line string) (*reply, error) {
	if _, err := r.stdin.WriteString(line); err != nil {
		return nil, err
	}
	if err := r.stdin.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := r.stdin.Flush(); err != nil {
		return nil, err
	}
	select {
	case l, ok := <-r.lines:
		if !ok {
			return nil, fmt.Errorf("%s: runner closed its output", r.name)
		}
		var rp reply
		if err := json.Unmarshal([]byte(l), &rp); err != nil {
			return nil, fmt.Errorf("%s: bad reply %q: %v", r.name, truncate(l), err)
		}
		return &rp, nil
	case err := <-r.errs:
		return nil, fmt.Errorf("%s: read: %v", r.name, err)
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("%s: timed out after %s", r.name, caseTimeout)
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// ------------------------------------------------------------------ comparison

// normalise decodes JSON into interface{} so all numbers become float64 and
// 1 vs 1.0 compare equal.
func normalise(raw json.RawMessage) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func show(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return truncate(string(b))
}

// ------------------------------------------------------------------ the report

type groupScore struct {
	Cases      int `json:"cases"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type report struct {
	Library       string                `json:"library"`
	Upstream      string                `json:"upstream"`
	UpstreamKind  string                `json:"upstreamEcosystem"`
	UpstreamTool  string                `json:"upstreamTool"`
	GoModule      string                `json:"goModule"`
	Cases         int                   `json:"cases"`
	Match         int                   `json:"match"`
	Mismatch      int                   `json:"mismatch"`
	Deviations    int                   `json:"deviations"`
	ParityPercent float64               `json:"parityPercent"`
	Groups        map[string]groupScore `json:"groups"`
	Mismatches    []string              `json:"mismatches"`
	DeviationIDs  []string              `json:"deviationCases"`
}

// --------------------------------------------------------------------- helpers

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", goModulePath)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return goModulePath + " (version unknown)"
	}
	return strings.TrimSpace(string(out))
}

func buildGoRunner(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-runner")
	cmd := exec.Command("go", "build", "-o", bin, "./go")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the Go runner failed: %v\n%s", err, out)
	}
	return bin
}

// jqVersion asks a candidate binary for its version string.
func jqVersion(bin string) (string, error) {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// findJQ locates a jq binary whose version matches the pin. PATH is consulted
// first; a handful of common install prefixes are then tried, because a machine
// may have several jq builds and only one of them is the pinned oracle.
// A missing jq skips the suite: someone who only has Go must not see a red build.
func findJQ(t *testing.T) (bin, version string) {
	t.Helper()
	var candidates []string
	if p, err := exec.LookPath("jq"); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/jq",
		"/usr/local/bin/jq",
		"/usr/bin/jq",
		"/opt/anaconda3/bin/jq",
	)
	var found []string
	for _, c := range candidates {
		v, err := jqVersion(c)
		if err != nil {
			continue
		}
		found = append(found, c+" ("+v+")")
		if v == pinnedJQ {
			return c, v
		}
	}
	if len(found) == 0 {
		t.Skipf("no jq binary found in PATH or the usual prefixes; skipping the jq parity suite")
	}
	t.Skipf("no jq matching the pinned oracle %s; found %s. Install jq 1.7.1 or re-pin the cases",
		pinnedJQ, strings.Join(found, ", "))
	return "", ""
}

// startUpstream launches the python wrapper that shells out to jq per case. One
// long-lived wrapper process per suite, as the harness contract requires.
func startUpstream(t *testing.T, jqBin string) *runner {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH; skipping the jq parity suite")
	}
	script, err := filepath.Abs(filepath.Join("c", "run.py"))
	if err != nil {
		t.Fatalf("resolve c/run.py: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("upstream runner missing: %v", err)
	}
	return startRunner(t, "c", py, script, jqBin)
}

// ------------------------------------------------------------------------ test

func TestParity(t *testing.T) {
	cases := loadCases(t)

	jqBin, jqVer := findJQ(t)
	t.Logf("upstream oracle: %s (%s)", jqBin, jqVer)

	goBin := buildGoRunner(t)

	up := startUpstream(t, jqBin)
	port := startRunner(t, "go", goBin)

	rep := report{
		Library:      "jq",
		Upstream:     pinnedUpstream,
		UpstreamKind: "c",
		UpstreamTool: jqVer,
		GoModule:     goModuleVersion(t),
		Groups:       map[string]groupScore{},
	}

	skipped := 0
	for _, c := range cases {
		c := c
		line, err := json.Marshal(map[string]interface{}{
			"id":   c.ID,
			"fn":   c.Fn,
			"args": c.Args,
		})
		if err != nil {
			t.Fatalf("%s: marshal: %v", c.ID, err)
		}

		// Both runners are fed in lockstep, outside t.Run, so the two streams
		// stay aligned even when a subtest reports a failure.
		upRep, upErr := up.call(string(line))
		goRep, goErr := port.call(string(line))

		matched, ran := false, false
		t.Run(c.ID, func(t *testing.T) {
			ran = true
			if upErr != nil {
				t.Fatalf("upstream runner: %v", upErr)
			}
			if goErr != nil {
				t.Fatalf("go runner: %v", goErr)
			}
			if upRep.ID != c.ID || goRep.ID != c.ID {
				t.Fatalf("reply id mismatch: upstream=%q go=%q want %q", upRep.ID, goRep.ID, c.ID)
			}

			fail := func(format string, args ...interface{}) {
				if c.Deviation != "" {
					t.Logf("deviation (%s): "+format, append([]interface{}{c.Deviation}, args...)...)
					return
				}
				t.Errorf(format, args...)
			}

			if upRep.OK != goRep.OK {
				fail("case %s (%s): ok differs: upstream=%v (%s) go=%v (%s)",
					c.ID, c.Note, upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				return
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is deliberately not
				// compared; wording differences belong in COVERAGE.md.
				matched = true
				return
			}
			upVal, err := normalise(upRep.Value)
			if err != nil {
				t.Fatalf("upstream value not JSON: %v", err)
			}
			goVal, err := normalise(goRep.Value)
			if err != nil {
				t.Fatalf("go value not JSON: %v", err)
			}
			if !reflect.DeepEqual(upVal, goVal) {
				fail("case %s (%s):\n  program  %s\n  upstream %s = %s\n  go       %s = %s",
					c.ID, c.Note, string(c.Args[0]), c.UpstreamFn, show(upVal), c.GoFn, show(goVal))
				return
			}
			matched = true
		})

		if !ran {
			// filtered out by -run: do not score it
			skipped++
			continue
		}
		g := rep.Groups[c.group]
		g.Cases++
		rep.Cases++
		switch {
		case matched:
			g.Match++
			rep.Match++
		case c.Deviation != "":
			g.Deviations++
			rep.Deviations++
			rep.DeviationIDs = append(rep.DeviationIDs, c.ID)
		default:
			g.Mismatch++
			rep.Mismatch++
			rep.Mismatches = append(rep.Mismatches, c.ID)
		}
		rep.Groups[c.group] = g
	}

	denom := rep.Cases - rep.Deviations
	if denom > 0 {
		rep.ParityPercent = 100 * float64(rep.Match) / float64(denom)
	}
	if rep.Mismatches == nil {
		rep.Mismatches = []string{}
	}
	if rep.DeviationIDs == nil {
		rep.DeviationIDs = []string{}
	}

	if skipped > 0 {
		t.Logf("%d of %d cases were filtered out by -run; leaving parity.json untouched",
			skipped, len(cases))
		return
	}

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile("parity.json", b, 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("jq parity: %d cases, %d match, %d mismatch, %d deviations, %.2f%%",
		rep.Cases, rep.Match, rep.Mismatch, rep.Deviations, rep.ParityPercent)
}
