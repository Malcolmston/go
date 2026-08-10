// Package parity drives the upstream moment.js runner and the Go
// github.com/malcolmston/moment runner over the same case files and compares
// their answers.  Upstream is the oracle: no expected values are stored
// anywhere in this directory.
package parity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// perCaseTimeout bounds how long either runner may take to answer one case, so
// a hung runner fails that case rather than wedging the suite.
const perCaseTimeout = 20 * time.Second

// ---------------------------------------------------------------- case files

type caseDoc struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Cases    []caseSpec `json:"cases"`
}

type caseSpec struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note,omitempty"`
	Deviation  string            `json:"deviation,omitempty"`

	group string // filled in from the enclosing document
}

func loadCases(t *testing.T) ([]caseSpec, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("globbing cases: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no case files found in cases/")
	}
	sort.Strings(paths)

	var all []caseSpec
	upstream := ""
	seen := map[string]string{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		var doc caseDoc
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parsing %s: %v", p, err)
		}
		if doc.Upstream == "" {
			t.Fatalf("%s: missing pinned \"upstream\" version", p)
		}
		if upstream == "" {
			upstream = doc.Upstream
		} else if upstream != doc.Upstream {
			t.Fatalf("%s pins %q but another case file pins %q", p, doc.Upstream, upstream)
		}
		for _, c := range doc.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = doc.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ---------------------------------------------------------------- runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type result struct {
	ok    bool
	value any
	err   string
	fatal error // transport-level failure (timeout, dead runner)
}

// runner is a long-lived subprocess speaking JSON Lines: one process for the
// whole suite, not one per case.
type runner struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	errs   chan error
	stderr *bytes.Buffer
	dead   bool
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	// Both runners must agree on the zone; nothing in the cases relies on the
	// host's local time.
	cmd.Env = append(os.Environ(), "TZ=UTC", "LC_ALL=C", "GOWORK=off")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin pipe: %v", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", name, err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}

	r := &runner{
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		lines:  make(chan string, 64),
		errs:   make(chan error, 1),
		stderr: &stderr,
	}
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
	}()
	return r
}

func (r *runner) stop() {
	if r == nil {
		return
	}
	_ = r.stdin.Close()
	_ = r.cmd.Wait()
}

func (r *runner) ask(c caseSpec) result {
	if r.dead {
		return result{fatal: fmt.Errorf("%s runner is no longer responding", r.name)}
	}
	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	line, err := json.Marshal(req)
	if err != nil {
		return result{fatal: fmt.Errorf("encoding request: %w", err)}
	}
	if _, err := r.stdin.Write(append(line, '\n')); err != nil {
		r.dead = true
		return result{fatal: fmt.Errorf("%s: write: %w (stderr: %s)", r.name, err, r.stderr.String())}
	}

	timer := time.NewTimer(perCaseTimeout)
	defer timer.Stop()
	select {
	case raw, open := <-r.lines:
		if !open {
			r.dead = true
			return result{fatal: fmt.Errorf("%s exited early (stderr: %s)", r.name, r.stderr.String())}
		}
		var rep reply
		if err := json.Unmarshal([]byte(raw), &rep); err != nil {
			return result{fatal: fmt.Errorf("%s: unparseable reply %q: %w", r.name, raw, err)}
		}
		if rep.ID != c.ID {
			r.dead = true
			return result{fatal: fmt.Errorf("%s: out of sync, wanted id %q got %q", r.name, c.ID, rep.ID)}
		}
		if !rep.OK {
			return result{ok: false, err: rep.Error}
		}
		var v any
		if len(rep.Value) > 0 {
			if err := json.Unmarshal(rep.Value, &v); err != nil {
				return result{fatal: fmt.Errorf("%s: unparseable value: %w", r.name, err)}
			}
		}
		return result{ok: true, value: v}
	case <-timer.C:
		r.dead = true
		return result{fatal: fmt.Errorf("%s: timed out after %s on case %s", r.name, perCaseTimeout, c.ID)}
	}
}

// ---------------------------------------------------------------- comparison

// equalJSON is a normalising deep-equal: every JSON number is a float64, so 1
// and 1.0 compare equal, and NaN is never produced by either runner (both emit
// null instead).
func equalJSON(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, x := range av {
			y, present := bv[k]
			if !present || !equalJSON(x, y) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalJSON(av[i], bv[i]) {
				return false
			}
		}
		return true
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(av) || math.IsNaN(bv) {
			return math.IsNaN(av) && math.IsNaN(bv)
		}
		// Tolerate the last bit of float noise in fractional diffs.
		if av == bv {
			return true
		}
		scale := math.Max(math.Abs(av), math.Abs(bv))
		return math.Abs(av-bv) <= 1e-9*math.Max(scale, 1)
	case nil:
		return b == nil
	default:
		return a == b
	}
}

func show(r result) string {
	if r.fatal != nil {
		return "TRANSPORT ERROR: " + r.fatal.Error()
	}
	if !r.ok {
		return "ok=false (error: " + r.err + ")"
	}
	b, err := json.Marshal(r.value)
	if err != nil {
		return fmt.Sprintf("ok=true value=%#v", r.value)
	}
	return "ok=true value=" + string(b)
}

// ---------------------------------------------------------------- scoreboard

type score struct {
	Library     string                `json:"library"`
	Upstream    string                `json:"upstream"`
	GoModule    string                `json:"goModule"`
	GoVersion   string                `json:"goModuleVersion"`
	GeneratedBy string                `json:"generatedBy"`
	Cases       int                   `json:"cases"`
	Match       int                   `json:"match"`
	Mismatch    int                   `json:"mismatch"`
	Deviations  int                   `json:"deviations"`
	Errors      int                   `json:"transportErrors"`
	BothFailed  int                   `json:"bothFailed"`
	ParityPct   float64               `json:"parityPercent"`
	ByGroup     map[string]groupScore `json:"byGroup"`
	Mismatches  []string              `json:"mismatchIDs"`
}

type groupScore struct {
	Cases     int `json:"cases"`
	Match     int `json:"match"`
	Mismatch  int `json:"mismatch"`
	Deviation int `json:"deviations"`
}

// ---------------------------------------------------------------- the test

func TestParity(t *testing.T) {
	cases, upstream := loadCases(t)

	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping the moment parity comparison")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "moment")); err != nil {
		t.Skipf("pinned upstream not installed; run: (cd %s && npm install)", filepath.Join("parity", "moment", "node"))
	}

	// Build the Go runner once, into a temp dir, with the workspace disabled so
	// the published module version in go.mod is what gets tested.
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found in PATH")
	}
	binDir := t.TempDir()
	goRunner := filepath.Join(binDir, "go-runner")
	build := exec.Command(goBin, "build", "-o", goRunner, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the Go runner failed: %v\n%s", err, out)
	}

	up := startRunner(t, "node", exec.Command(nodeBin, filepath.Join("node", "run.js")))
	defer up.stop()
	port := startRunner(t, "go", exec.Command(goRunner))
	defer port.stop()

	// Stream every case through both long-lived runners, then compare.
	type pair struct {
		spec caseSpec
		a, b result
	}
	results := make([]pair, 0, len(cases))
	for _, c := range cases {
		results = append(results, pair{spec: c, a: up.ask(c), b: port.ask(c)})
	}

	s := score{
		Library:     "moment",
		Upstream:    upstream,
		GoModule:    "github.com/malcolmston/moment",
		GoVersion:   goModuleVersion(t),
		GeneratedBy: "go test ./parity/moment/",
		Cases:       len(results),
		ByGroup:     map[string]groupScore{},
	}

	for _, p := range results {
		c, a, b := p.spec, p.a, p.b
		g := s.ByGroup[c.group]
		g.Cases++

		matched := a.fatal == nil && b.fatal == nil && a.ok == b.ok &&
			(!a.ok || equalJSON(a.value, b.value))

		switch {
		case a.fatal != nil || b.fatal != nil:
			s.Errors++
			g.Mismatch++
		case c.Deviation != "":
			s.Deviations++
			g.Deviation++
		case matched:
			s.Match++
			g.Match++
			if !a.ok {
				s.BothFailed++
			}
		default:
			s.Mismatch++
			g.Mismatch++
			s.Mismatches = append(s.Mismatches, c.ID)
		}
		s.ByGroup[c.group] = g

		t.Run(c.ID, func(t *testing.T) {
			if a.fatal != nil {
				t.Fatalf("upstream runner: %v", a.fatal)
			}
			if b.fatal != nil {
				t.Fatalf("go runner: %v", b.fatal)
			}
			if c.Deviation != "" {
				if matched {
					t.Logf("declared deviation but the two agree (deviation may be stale): %s", c.Deviation)
				} else {
					t.Logf("declared deviation: %s\n  upstream(%s): %s\n  go      (%s): %s",
						c.Deviation, c.UpstreamFn, show(a), c.GoFn, show(b))
				}
				return
			}
			if a.ok != b.ok {
				t.Errorf("validity differs\n  upstream(%s): %s\n  go      (%s): %s",
					c.UpstreamFn, show(a), c.GoFn, show(b))
				return
			}
			if a.ok && !equalJSON(a.value, b.value) {
				t.Errorf("value differs\n  upstream(%s): %s\n  go      (%s): %s",
					c.UpstreamFn, show(a), c.GoFn, show(b))
			}
		})
	}

	compared := s.Match + s.Mismatch
	if compared > 0 {
		s.ParityPct = math.Round(float64(s.Match)/float64(compared)*10000) / 100
	}
	sort.Strings(s.Mismatches)
	writeScore(t, s)

	t.Logf("moment parity: %d/%d compared cases match (%.2f%%), %d declared deviations, %d transport errors",
		s.Match, compared, s.ParityPct, s.Deviations, s.Errors)
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		for i, tok := range f {
			if tok == "github.com/malcolmston/moment" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return "unknown"
}

func writeScore(t *testing.T, s score) {
	t.Helper()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Errorf("encoding parity.json: %v", err)
		return
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Errorf("writing parity.json: %v", err)
	}
}
