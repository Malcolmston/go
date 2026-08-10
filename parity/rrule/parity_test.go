// Package parity drives the upstream (Python/python-dateutil) and Go
// (github.com/malcolmston/rrule) runners over the same case files and compares
// their answers. Upstream is the oracle: nothing here hand-writes an expected
// occurrence list.
//
// Run with:
//
//	GOWORK=off go test ./parity/rrule/
//
// The suite skips (never fails) when python3 or the pinned python-dateutil is
// unavailable, so a Go-only checkout stays green.
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	upstreamPin = "python-dateutil@2.9.0.post0"
	caseTimeout = 20 * time.Second
)

// ---------------------------------------------------------------- case files

type caseSpec struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn,omitempty"`
	GoFn       string            `json:"goFn,omitempty"`
	Note       string            `json:"note,omitempty"`
	Deviation  string            `json:"deviation,omitempty"`

	group string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Cases    []caseSpec `json:"cases"`
}

func loadCases(t *testing.T, dir string) []caseSpec {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no case files in %s", dir)
	}

	var all []caseSpec
	seen := map[string]string{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var cf caseFile
		if err := json.Unmarshal(raw, &cf); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if cf.Upstream != upstreamPin {
			t.Fatalf("%s pins upstream %q, want %q", path, cf.Upstream, upstreamPin)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, path)
			}
			seen[c.ID] = path
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ------------------------------------------------------------------- runners

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// runner is one long-lived subprocess speaking JSON Lines. Requests are
// serialised; a case that times out marks the runner broken so the remaining
// cases fail fast instead of hanging the suite.
type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string

	mu     sync.Mutex
	broken string
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin pipe: %v", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", name, err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}

	r := &runner{
		name:  name,
		cmd:   cmd,
		stdin: stdin,
		lines: make(chan string, 64),
	}

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		close(r.lines)
	}()

	t.Cleanup(func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
		if s := stderr.String(); strings.TrimSpace(s) != "" {
			t.Logf("%s stderr:\n%s", name, s)
		}
	})

	return r
}

func (r *runner) call(c caseSpec) reply {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.broken != "" {
		return reply{ID: c.ID, OK: false, Error: "runner unusable: " + r.broken}
	}

	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	enc, err := json.Marshal(req)
	if err != nil {
		return reply{ID: c.ID, OK: false, Error: "encode request: " + err.Error()}
	}
	if _, err := r.stdin.Write(append(enc, '\n')); err != nil {
		r.broken = "write: " + err.Error()
		return reply{ID: c.ID, OK: false, Error: r.broken}
	}

	select {
	case line, open := <-r.lines:
		if !open {
			r.broken = "runner closed stdout"
			return reply{ID: c.ID, OK: false, Error: r.broken}
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			r.broken = "unparseable reply: " + err.Error()
			return reply{ID: c.ID, OK: false, Error: r.broken}
		}
		if rep.ID != c.ID {
			r.broken = fmt.Sprintf("out of sync: got reply for %q while asking %q", rep.ID, c.ID)
			return reply{ID: c.ID, OK: false, Error: r.broken}
		}
		return rep
	case <-time.After(caseTimeout):
		r.broken = fmt.Sprintf("timed out after %s on case %q", caseTimeout, c.ID)
		return reply{ID: c.ID, OK: false, Error: r.broken}
	}
}

// ------------------------------------------------------------------ compare

// decode turns a JSON value into the normalised form used for comparison: all
// numbers become float64, so 1 and 1.0 compare equal.
func decode(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// show renders a value for test output, trimming very long occurrence lists so
// a failure stays readable.
func show(raw json.RawMessage) string {
	v, err := decode(raw)
	if err != nil {
		return fmt.Sprintf("<unparseable %s>", raw)
	}
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(out)
	if len(s) > 600 {
		s = s[:600] + "...(truncated)"
	}
	return s
}

// firstDiff points at the first element where two occurrence lists part ways,
// which is what actually identifies a wrong date.
func firstDiff(up, got any) string {
	ua, uok := up.([]any)
	ga, gok := got.([]any)
	if !uok || !gok {
		return ""
	}
	n := len(ua)
	if len(ga) < n {
		n = len(ga)
	}
	for i := 0; i < n; i++ {
		if !reflect.DeepEqual(ua[i], ga[i]) {
			return fmt.Sprintf("first difference at index %d: upstream %v, go %v", i, ua[i], ga[i])
		}
	}
	return fmt.Sprintf("common prefix agrees; lengths differ: upstream %d, go %d", len(ua), len(ga))
}

// ---------------------------------------------------------------- the score

type groupScore struct {
	Cases      int `json:"cases"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type score struct {
	Library       string                `json:"library"`
	Upstream      string                `json:"upstream"`
	GoModule      string                `json:"goModule"`
	Cases         int                   `json:"cases"`
	Match         int                   `json:"match"`
	Mismatch      int                   `json:"mismatch"`
	Deviations    int                   `json:"deviations"`
	ParityPercent float64               `json:"parityPercent"`
	Groups        map[string]groupScore `json:"groups"`
	Mismatches    []string              `json:"mismatches"`
}

var modLine = regexp.MustCompile(`github\.com/malcolmston/rrule\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/rrule " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

// pythonInterpreter returns an interpreter whose dateutil matches the pin. A
// venv inside python/ wins over the system interpreter, so a machine whose
// system dateutil is the wrong version can still run the suite without any
// global install.
func pythonInterpreter(t *testing.T) string {
	t.Helper()
	want := strings.SplitN(upstreamPin, "@", 2)[1]

	var candidates []string
	if venv := filepath.Join("python", ".venv", "bin", "python3"); fileExists(venv) {
		abs, err := filepath.Abs(venv)
		if err == nil {
			candidates = append(candidates, abs)
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		t.Skip("python3 not found in PATH and no python/.venv; skipping cross-language parity")
	}

	var tried []string
	for _, py := range candidates {
		out, err := exec.Command(py, "-c",
			"import dateutil, sys; sys.stdout.write(dateutil.__version__)").Output()
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s: dateutil not importable (%v)", py, err))
			continue
		}
		got := strings.TrimSpace(string(out))
		if got != want {
			tried = append(tried, fmt.Sprintf("%s: dateutil %s, want %s", py, got, want))
			continue
		}
		return py
	}
	t.Skipf("no interpreter with python-dateutil %s:\n  %s\n"+
		"create one with: python3 -m venv parity/rrule/python/.venv && "+
		"parity/rrule/python/.venv/bin/pip install python-dateutil==%s",
		want, strings.Join(tried, "\n  "), want)
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func TestParity(t *testing.T) {
	py := pythonInterpreter(t)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	pyCmd := exec.Command(py, "-u", "run.py")
	pyCmd.Dir = "python"
	// A stray PYTHONPATH could shadow the pinned dateutil.
	pyCmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	up := startRunner(t, "python", pyCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:    "rrule",
		Upstream:   upstreamPin,
		GoModule:   goModuleVersion(),
		Cases:      len(cases),
		Groups:     map[string]groupScore{},
		Mismatches: []string{},
	}

	for _, c := range cases {
		upRep := up.call(c)
		goRep := gor.call(c)

		ran, ok := compare(t, c, upRep, goRep)
		if !ran {
			// Subtest filtered out by -run: do not score it.
			continue
		}

		g := sc.Groups[c.group]
		g.Cases++
		switch {
		case c.Deviation != "":
			g.Deviations++
			sc.Deviations++
		case ok:
			g.Match++
			sc.Match++
		default:
			g.Mismatch++
			sc.Mismatch++
			sc.Mismatches = append(sc.Mismatches, c.ID)
		}
		sc.Groups[c.group] = g
	}

	compared := sc.Match + sc.Mismatch
	if compared > 0 {
		sc.ParityPercent = float64(sc.Match) * 100 / float64(compared)
	}

	t.Logf("parity: %d/%d cases match (%.2f%%), %d deviations",
		sc.Match, compared, sc.ParityPercent, sc.Deviations)

	// A -run filter skips subtests, which would leave a misleading score on
	// disk, so only a complete pass is allowed to rewrite parity.json.
	if scored := compared + sc.Deviations; scored != len(cases) {
		t.Logf("partial run (%d of %d cases scored): leaving parity.json alone",
			scored, len(cases))
		return
	}
	writeScore(t, sc)
}

// compare runs one case as a subtest. It reports whether the subtest actually
// ran (a -run filter can skip it) and whether the two sides agree.
func compare(t *testing.T, c caseSpec, upRep, goRep reply) (ran, agreed bool) {
	t.Run(c.ID, func(t *testing.T) {
		ran = true
		fail := func(format string, args ...any) {
			if c.Deviation != "" {
				t.Logf("deviation (%s): "+format, append([]any{c.Deviation}, args...)...)
				return
			}
			t.Errorf(format, args...)
		}

		if upRep.OK != goRep.OK {
			fail("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
				upRep.OK, describe(upRep), goRep.OK, describe(goRep))
			return
		}
		if !upRep.OK {
			// Both failed: that is parity. Message text is not compared;
			// differences are recorded in COVERAGE.md.
			agreed = true
			if upRep.Error != goRep.Error {
				t.Logf("both failed, different messages: upstream %q, go %q", upRep.Error, goRep.Error)
			}
			return
		}

		upVal, err := decode(upRep.Value)
		if err != nil {
			fail("upstream value is not JSON: %v", err)
			return
		}
		goVal, err := decode(goRep.Value)
		if err != nil {
			fail("go value is not JSON: %v", err)
			return
		}
		if !reflect.DeepEqual(upVal, goVal) {
			fail("value mismatch\n  upstream: %s\n        go: %s\n  %s",
				show(upRep.Value), show(goRep.Value), firstDiff(upVal, goVal))
			return
		}
		agreed = true
	})

	if c.Deviation != "" {
		return ran, ran
	}
	return ran, agreed
}

func describe(r reply) string {
	if r.OK {
		return "value " + show(r.Value)
	}
	return "error " + fmt.Sprintf("%q", r.Error)
}

func writeScore(t *testing.T, sc score) {
	t.Helper()
	sort.Strings(sc.Mismatches)
	enc, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(enc, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}
