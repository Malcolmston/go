// Package parity drives the upstream (Ruby / ActiveRecord) migrate runner and
// the Go port runner over the same JSON case files and compares their answers.
//
// Upstream is the oracle: nothing here hand-writes an expected value. Every case
// is a script of schema or migration operations; the compared answer is the
// per-step outcome plus a canonical read-back of the resulting SQLite schema, so
// a divergence in *semantics* shows up rather than a formatting difference in the
// generated SQL.
//
// Each side's emitted SQL and its un-normalised index/constraint names travel
// with the reply under the "sql" and "raw" keys. Those keys are stripped before
// comparison — SQL text legitimately differs — and dumped to sql.json so
// type-mapping differences stay visible.
//
// Run with:  GOWORK=off go test ./parity/migrate/
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
	caseTimeout   = 120 * time.Second
	bundleTimeout = 10 * time.Minute
)

// goModulePath is the published port under test; there is no replace directive.
const goModulePath = "github.com/malcolmston/migrate"

// ------------------------------------------------------------------ case files

type caseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
	Encoding string  `json:"encoding"`
	Note     string  `json:"note"`
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

	group    string
	upstream string
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
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q in %s and %s", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			c.upstream = cf.Upstream
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

func startRunner(t *testing.T, name, dir, bin string, args ...string) *runner {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GOWORK=off")
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
		sc.Buffer(make([]byte, 0, 1<<20), 1<<27)
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
		_ = r.stdin.Flush()
		if p, ok := r.cmd.Stdin.(interface{ Close() error }); ok {
			_ = p.Close()
		}
		if r.cmd.Process == nil {
			return
		}
		done := make(chan struct{})
		go func() { _ = r.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = r.cmd.Process.Kill()
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
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}

// ------------------------------------------------------------------- comparison

// normalise decodes JSON into interface{} so all numbers become float64 and 1 vs
// 1.0 compare equal.
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

// diagnosticKeys are carried by every reply but never compared: SQL text
// legitimately differs between the two implementations, and the raw
// autogenerated names are exactly the thing being normalised away.
var diagnosticKeys = []string{"sql", "raw"}

// split separates the comparable part of a value from its diagnostics.
func split(v interface{}) (compared interface{}, diag map[string]interface{}) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v, nil
	}
	diag = map[string]interface{}{}
	out := map[string]interface{}{}
	for k, val := range m {
		out[k] = val
	}
	for _, k := range diagnosticKeys {
		if val, present := out[k]; present {
			diag[k] = val
			delete(out, k)
		}
	}
	return out, diag
}

func show(v interface{}) string {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return truncate(string(b))
}

// ------------------------------------------------------------------- the report

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

// prepareRubyRunner makes sure the self-contained Bundler project in ruby/ has
// its pinned gems installed. A missing ruby or bundler, or an install that
// cannot reach rubygems, skips the suite rather than failing it.
func prepareRubyRunner(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not found in PATH; skipping migrate parity suite")
	}
	bundle, err := exec.LookPath("bundle")
	if err != nil {
		t.Skip("bundler not found in PATH; skipping migrate parity suite")
	}
	// A successful `bundle check` means the pinned gems are already vendored, so
	// the suite runs offline once it has been installed.
	check := exec.Command(bundle, "check")
	check.Dir = "ruby"
	if err := check.Run(); err != nil {
		install := exec.Command(bundle, "install", "--path", "vendor/bundle")
		install.Dir = "ruby"
		var out strings.Builder
		install.Stdout = &out
		install.Stderr = &out
		if err := install.Start(); err != nil {
			t.Skipf("cannot start bundler: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- install.Wait() }()
		select {
		case err = <-done:
		case <-time.After(bundleTimeout):
			_ = install.Process.Kill()
			t.Skipf("bundle install timed out after %s", bundleTimeout)
		}
		if err != nil {
			t.Skipf("bundle install failed (no network for the pinned gems?): %v\n%s", err, out.String())
		}
	}
	return bundle
}

// ------------------------------------------------------------------------ test

func TestParity(t *testing.T) {
	cases := loadCases(t)

	bundle := prepareRubyRunner(t)
	goBin := buildGoRunner(t)

	up := startRunner(t, "ruby", "ruby", bundle, "exec", "ruby", "run.rb")
	port := startRunner(t, "go", ".", goBin)

	upstreamLabel := "rails/rails activerecord@8.0.2"
	if len(cases) > 0 && cases[0].upstream != "" {
		upstreamLabel = cases[0].upstream
	}

	rep := report{
		Library:      "migrate",
		Upstream:     upstreamLabel,
		UpstreamKind: "ruby",
		GoModule:     goModuleVersion(t),
		Groups:       map[string]groupScore{},
	}

	type sqlRecord struct {
		Case     string      `json:"case"`
		Group    string      `json:"group"`
		Upstream interface{} `json:"upstreamSQL"`
		Go       interface{} `json:"goSQL"`
		UpRaw    interface{} `json:"upstreamRawNames"`
		GoRaw    interface{} `json:"goRawNames"`
	}
	var sqlLog []sqlRecord

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

		// Both runners are fed in lockstep, outside t.Run, so the streams stay
		// aligned even when a subtest reports a failure.
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

			upVal, err := normalise(upRep.Value)
			if err != nil {
				t.Fatalf("upstream value not JSON: %v", err)
			}
			goVal, err := normalise(goRep.Value)
			if err != nil {
				t.Fatalf("go value not JSON: %v", err)
			}
			upCmp, upDiag := split(upVal)
			goCmp, goDiag := split(goVal)
			sqlLog = append(sqlLog, sqlRecord{
				Case: c.ID, Group: c.group,
				Upstream: upDiag["sql"], Go: goDiag["sql"],
				UpRaw: upDiag["raw"], GoRaw: goDiag["raw"],
			})

			fail := func(format string, args ...interface{}) {
				if c.Deviation != "" {
					t.Logf("deviation (%s): "+format, append([]interface{}{c.Deviation}, args...)...)
					return
				}
				t.Errorf(format, args...)
			}

			if upRep.OK != goRep.OK {
				fail("case %s: ok differs: upstream=%v (%s) go=%v (%s)",
					c.ID, upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				return
			}
			if !upRep.OK {
				// Both runners refused the case outright: that is parity. Message
				// text is not compared.
				matched = true
				return
			}
			if !reflect.DeepEqual(upCmp, goCmp) {
				fail("case %s (%s):\n  upstream %s =\n%s\n  go       %s =\n%s\n  upstream SQL: %s\n  go SQL:       %s",
					c.ID, c.Note, c.UpstreamFn, show(upCmp), c.GoFn, show(goCmp),
					show(upDiag["sql"]), show(goDiag["sql"]))
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
		t.Logf("%d of %d cases were filtered out by -run; leaving parity.json untouched", skipped, len(cases))
		return
	}

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	sb, err := json.MarshalIndent(sqlLog, "", "  ")
	if err != nil {
		t.Fatalf("marshal sql log: %v", err)
	}
	if err := os.WriteFile("sql.json", append(sb, '\n'), 0o644); err != nil {
		t.Fatalf("write sql.json: %v", err)
	}
	t.Logf("migrate parity: %d cases, %d match, %d mismatch, %d deviations, %.2f%%",
		rep.Cases, rep.Match, rep.Mismatch, rep.Deviations, rep.ParityPercent)
}
