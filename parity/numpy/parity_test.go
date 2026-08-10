// Package parity drives the upstream (numpy) runner and the Go
// (github.com/malcolmston/numpy) runner over the same cases and compares their
// answers. See ../HARNESS.md.
//
// Float comparison uses a combined relative + absolute epsilon of 1e-12:
// two numbers are equal when |a-b| <= 1e-12 or |a-b| <= 1e-12*max(|a|,|b|).
// Non-finite values arrive as the sentinel strings "NaN", "Infinity" and
// "-Infinity" and are compared exactly.
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

const (
	epsilon     = 1e-12
	caseTimeout = 20 * time.Second
)

// ------------------------------------------------------------------ cases

type Case struct {
	ID         string          `json:"id"`
	Fn         string          `json:"fn"`
	Args       json.RawMessage `json:"args"`
	UpstreamFn string          `json:"upstreamFn"`
	GoFn       string          `json:"goFn"`
	Note       string          `json:"note,omitempty"`
	Deviation  string          `json:"deviation,omitempty"`

	Group string `json:"-"`
}

type caseFile struct {
	Group    string `json:"group"`
	Upstream string `json:"upstream"`
	Cases    []Case `json:"cases"`
}

func loadCases(t *testing.T) ([]Case, string) {
	t.Helper()
	paths, err := filepath.Glob("cases/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no case files found in cases/: %v", err)
	}
	sort.Strings(paths)
	var all []Case
	upstream := ""
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q but another file pins %q", p, cf.Upstream, upstream)
		}
		for i := range cf.Cases {
			cf.Cases[i].Group = cf.Group
			all = append(all, cf.Cases[i])
		}
	}
	return all, upstream
}

// ------------------------------------------------------------------ runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name   string
	cmd    *exec.Cmd
	in     io.WriteCloser
	lines  chan string
	errs   chan error
	stderr *strings.Builder
}

func start(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin: %v", name, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout: %v", name, err)
	}
	var errbuf strings.Builder
	cmd.Stderr = &errbuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, in: in, lines: make(chan string, 8),
		errs: make(chan error, 1), stderr: &errbuf}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<25)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		close(r.lines)
	}()
	t.Cleanup(func() {
		r.in.Close()
		_ = r.cmd.Wait()
	})
	return r
}

// ask sends one case and waits for exactly one reply, enforcing a per-case
// timeout so a hung runner fails that case rather than the suite.
func (r *runner) ask(c Case) reply {
	// The args are spliced in verbatim rather than re-marshalled: round-tripping
	// through Go's encoder would turn -0.0 into "-0", which Python then reads as
	// the integer 0 and the sign of the zero is silently lost.
	id, _ := json.Marshal(c.ID)
	fn, _ := json.Marshal(c.Fn)
	args := []byte("[]")
	if len(c.Args) > 0 {
		// Case files are pretty-printed; compact so the request stays one line.
		var buf bytes.Buffer
		if err := json.Compact(&buf, c.Args); err != nil {
			return reply{ID: c.ID, OK: false, Error: "harness: " + err.Error()}
		}
		args = buf.Bytes()
	}
	b := []byte(fmt.Sprintf(`{"id":%s,"fn":%s,"args":%s}`, id, fn, args))
	if _, err := r.in.Write(append(b, '\n')); err != nil {
		return reply{ID: c.ID, OK: false, Error: "harness: write: " + err.Error()}
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return reply{ID: c.ID, OK: false,
				Error: fmt.Sprintf("harness: %s runner exited (stderr: %s)", r.name, tail(r.stderr.String()))}
		}
		var rp reply
		if err := json.Unmarshal([]byte(line), &rp); err != nil {
			return reply{ID: c.ID, OK: false, Error: "harness: bad reply: " + err.Error()}
		}
		return rp
	case <-time.After(caseTimeout):
		return reply{ID: c.ID, OK: false, Error: "harness: timeout"}
	}
}

func tail(s string) string {
	if len(s) > 400 {
		return "…" + s[len(s)-400:]
	}
	return s
}

// ------------------------------------------------------------------ compare

func numEqual(a, b float64) bool {
	if a == b {
		return true
	}
	d := math.Abs(a - b)
	if d <= epsilon {
		return true
	}
	m := math.Max(math.Abs(a), math.Abs(b))
	return d <= epsilon*m
}

// deepEqual treats all JSON numbers as float64 and compares them with the
// epsilon policy; everything else must match structurally and exactly.
func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && numEqual(av, bv)
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case nil:
		return b == nil
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			w, ok := bv[k]
			if !ok || !deepEqual(v, w) {
				return false
			}
		}
		return true
	}
	return false
}

func decode(raw json.RawMessage) any {
	var v any
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return "harness: undecodable value: " + string(raw)
	}
	return v
}

func show(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// ------------------------------------------------------------------ test

type score struct {
	Library     string         `json:"library"`
	Upstream    string         `json:"upstream"`
	GoModule    string         `json:"goModule"`
	Epsilon     float64        `json:"floatEpsilon"`
	Cases       int            `json:"cases"`
	Match       int            `json:"match"`
	Mismatch    int            `json:"mismatch"`
	Deviations  int            `json:"deviations"`
	BothFailed  int            `json:"bothFailed"`
	ParityPct   float64        `json:"parityPercent"`
	ByGroup     map[string]int `json:"casesByGroup"`
	Mismatches  []string       `json:"mismatchIDs"`
	DeviationID []string       `json:"deviationIDs"`
	GeneratedBy string         `json:"generatedBy"`
}

func TestParity(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found; skipping numpy parity")
	}
	if out, err := exec.Command(py, "-c", "import numpy").CombinedOutput(); err != nil {
		t.Skipf("numpy not importable by %s (%s); skipping numpy parity", py, strings.TrimSpace(string(out)))
	}

	cases, upstream := loadCases(t)

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building go runner: %v\n%s", err, out)
	}

	up := start(t, "python", exec.Command(py, "-u", "python/run.py"))
	go_ := start(t, "go", exec.Command(bin))

	s := score{
		Library: "numpy", Upstream: upstream, GoModule: goModuleVersion(t),
		Epsilon: epsilon, Cases: len(cases), ByGroup: map[string]int{},
		GeneratedBy: "go test ./parity/numpy/",
	}

	for _, c := range cases {
		c := c
		s.ByGroup[c.Group]++
		ur := up.ask(c)
		gr := go_.ask(c)
		ok := true
		t.Run(c.ID, func(t *testing.T) {
			report := t.Errorf
			if c.Deviation != "" {
				// Deliberate divergence: recorded, not a build failure.
				report = t.Logf
			}
			switch {
			case ur.OK != gr.OK:
				ok = false
				report("%s: upstream ok=%v (%s%s) but go ok=%v (%s%s)",
					c.ID, ur.OK, show(ur.Value), ur.Error, gr.OK, show(gr.Value), gr.Error)
			case !ur.OK && !gr.OK:
				// Both failed: parity. Messages are allowed to differ.
			case !deepEqual(decode(ur.Value), decode(gr.Value)):
				ok = false
				report("%s: value mismatch\n  upstream: %s\n        go: %s",
					c.ID, show(ur.Value), show(gr.Value))
			}
		})
		switch {
		case c.Deviation != "":
			s.Deviations++
			s.DeviationID = append(s.DeviationID, c.ID)
		case !ok:
			s.Mismatch++
			s.Mismatches = append(s.Mismatches, c.ID)
		default:
			s.Match++
			if !ur.OK && !gr.OK {
				s.BothFailed++
			}
		}
	}

	compared := s.Match + s.Mismatch
	if compared > 0 {
		s.ParityPct = math.Round(float64(s.Match)/float64(compared)*10000) / 100
	}
	sort.Strings(s.Mismatches)
	sort.Strings(s.DeviationID)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("cases=%d match=%d mismatch=%d deviations=%d bothFailed=%d parity=%.2f%%",
		s.Cases, s.Match, s.Mismatch, s.Deviations, s.BothFailed, s.ParityPct)
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/numpy")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
