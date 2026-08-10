// Package parity drives the upstream (Python/streamlit==1.61.1) and Go
// (github.com/malcolmston/streamlit) runners over the same case files and
// compares the element trees they produce. Upstream is the oracle: nothing here
// hand-writes an expected value.
//
// Run with:
//
//	GOWORK=off go test ./parity/streamlit/
//
// The suite skips (never fails) when python3, the venv, or the pinned
// streamlit install is unavailable, so a Go-only checkout stays green.
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
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	upstreamPin = "streamlit==1.61.1"
	// An AppTest run boots Streamlit's script runner, so cases are slower than
	// a pure function call; several drive half a dozen reruns.
	caseTimeout = 120 * time.Second
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

	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 64)}

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 32*1024*1024)
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
			t.Logf("%s stderr:\n%s", name, truncate(s, 4000))
		}
	})

	return r
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…truncated…"
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

func show(raw json.RawMessage) string {
	v, err := decode(raw)
	if err != nil {
		return fmt.Sprintf("<unparseable %s>", raw)
	}
	out, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return truncate(string(out), 6000)
}

// firstDiff walks two decoded values in step and names the first path at which
// they disagree, so a mismatch inside a deep element tree is readable.
func firstDiff(path string, a, b any) string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: object vs %T", path, b)
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			x, okx := av[k]
			y, oky := bv[k]
			if okx != oky {
				return fmt.Sprintf("%s.%s: present=%v vs present=%v", path, k, okx, oky)
			}
			if d := firstDiff(path+"."+k, x, y); d != "" {
				return d
			}
		}
		return ""
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return fmt.Sprintf("%s: array vs %T", path, b)
		}
		if len(av) != len(bv) {
			return fmt.Sprintf("%s: length %d vs %d", path, len(av), len(bv))
		}
		for i := range av {
			if d := firstDiff(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i]); d != "" {
				return d
			}
		}
		return ""
	default:
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: upstream %s vs go %s", path, brief(a), brief(b))
		}
		return ""
	}
}

func brief(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(out)
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

var modLine = regexp.MustCompile(`github\.com/malcolmston/streamlit\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/streamlit " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

// venvPython is the interpreter inside the per-library virtualenv. The upstream
// package is never installed globally.
func venvPython() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("python", ".venv", "Scripts", "python.exe")
	}
	return filepath.Join("python", ".venv", "bin", "python")
}

func TestParity(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found in PATH; skipping cross-language parity")
	}
	py, err := filepath.Abs(venvPython())
	if err != nil {
		t.Fatalf("resolve venv interpreter: %v", err)
	}
	if _, err := os.Stat(py); err != nil {
		t.Skipf("no venv at %s; create it with\n"+
			"  python3 -m venv parity/streamlit/python/.venv && "+
			"parity/streamlit/python/.venv/bin/pip install %q", py, upstreamPin)
	}
	assertUpstreamVersion(t, py)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	// The repo root holds a directory literally named "streamlit" (the Go
	// library), which Python 3 would import as a namespace package and shadow
	// the real one; running from python/ avoids that entirely.
	pyCmd := exec.Command(py, "run.py")
	pyCmd.Dir = "python"
	up := startRunner(t, "python", pyCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:    "streamlit",
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
			fail("element tree mismatch\n  first difference at %s\n  upstream: %s\n        go: %s",
				firstDiff("$", upVal, goVal), show(upRep.Value), show(goRep.Value))
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
	enc, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(enc, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}

// assertUpstreamVersion checks the installed upstream matches the pin, so the
// score can never be attributed to the wrong oracle.
func assertUpstreamVersion(t *testing.T, py string) {
	t.Helper()
	want := strings.SplitN(upstreamPin, "==", 2)[1]

	cmd := exec.Command(py, "-c", "import streamlit; print(streamlit.__version__)")
	cmd.Dir = "python"
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("streamlit is not importable in the venv (%v); install it with\n"+
			"  parity/streamlit/python/.venv/bin/pip install %q", err, upstreamPin)
	}
	got := strings.TrimSpace(string(out))
	if got != want {
		t.Fatalf("installed streamlit is %s, cases pin %s", got, want)
	}
}
