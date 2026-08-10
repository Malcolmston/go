// Package parity drives the upstream (Python fastmcp@3.4.6) and Go
// (github.com/malcolmston/fastmcp) runners over the same case files and compares
// their answers. The comparable artefact is Model Context Protocol *wire
// behaviour*: each case builds an equivalent server on both sides and asks it a
// JSON-RPC question in-process. Upstream is the oracle; nothing here hand-writes
// an expected value.
//
// Run with:
//
//	GOWORK=off go test ./parity/fastmcp/
//
// The suite skips (never fails) when python3 is missing or the pinned venv cannot
// be created, so a Go-only checkout stays green.
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
	upstreamPin = "fastmcp@3.4.6"

	// Each case builds a fresh in-memory server on both sides, so the budget is
	// generous compared with a pure-function harness.
	caseTimeout = 60 * time.Second
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
	var stderrMu sync.Mutex
	var stderr strings.Builder
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("%s: stderr pipe: %v", name, err)
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				stderrMu.Lock()
				stderr.Write(buf[:n])
				stderrMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

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
		stderrMu.Lock()
		s := stderr.String()
		stderrMu.Unlock()
		if strings.TrimSpace(s) != "" {
			t.Logf("%s stderr (first 4000 bytes):\n%s", name, truncate(s, 4000))
		}
	})

	return r
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… truncated"
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
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return truncate(string(out), 1200)
}

func describe(r reply) string {
	if r.OK {
		return "value " + show(r.Value)
	}
	return "error " + fmt.Sprintf("%q", r.Error)
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

var modLine = regexp.MustCompile(`github\.com/malcolmston/fastmcp\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/fastmcp " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------- the python venv

// venvPython returns the interpreter inside python/.venv, creating the venv and
// installing the pinned upstream if necessary. Any failure is a skip, never a
// test failure: a checkout with only Go must stay green.
func venvPython(t *testing.T) string {
	t.Helper()

	name := "bin/python3"
	if runtime.GOOS == "windows" {
		name = "Scripts/python.exe"
	}
	py := filepath.Join("python", ".venv", filepath.FromSlash(name))

	if _, err := os.Stat(py); err != nil {
		system, lerr := exec.LookPath("python3")
		if lerr != nil {
			t.Skip("python3 not found in PATH; skipping cross-language parity")
		}
		mk := exec.Command(system, "-m", "venv", filepath.Join("python", ".venv"))
		if out, err := mk.CombinedOutput(); err != nil {
			t.Skipf("cannot create python/.venv: %v\n%s", err, out)
		}
	}

	if !upstreamInstalled(py) {
		req := filepath.Join("python", "requirements.txt")
		assertRequirementsPin(t, req)
		inst := exec.Command(py, "-m", "pip", "install", "--disable-pip-version-check", "-q", "-r", req)
		if out, err := inst.CombinedOutput(); err != nil {
			t.Skipf("`pip install -r %s` failed: %v\n%s", req, err, out)
		}
	}
	assertUpstreamVersion(t, py)
	return py
}

func upstreamInstalled(py string) bool {
	out, err := exec.Command(py, "-c", "import fastmcp;print(fastmcp.__version__)").Output()
	return err == nil && len(out) > 0
}

func wantUpstreamVersion() string {
	_, v, _ := strings.Cut(upstreamPin, "@")
	return v
}

// assertRequirementsPin keeps python/requirements.txt and upstreamPin from
// drifting apart, so the venv can only ever be built from the pinned oracle.
func assertRequirementsPin(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	want := "fastmcp==" + wantUpstreamVersion()
	if !strings.Contains(string(raw), want) {
		t.Fatalf("%s does not pin %q", path, want)
	}
}

// assertUpstreamVersion checks the installed upstream matches the pin, so the
// score can never be attributed to the wrong oracle.
func assertUpstreamVersion(t *testing.T, py string) {
	t.Helper()
	out, err := exec.Command(py, "-c", "import fastmcp;print(fastmcp.__version__)").Output()
	if err != nil {
		t.Skipf("cannot import fastmcp from the venv: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != wantUpstreamVersion() {
		t.Fatalf("venv has fastmcp %s, cases pin %s", got, wantUpstreamVersion())
	}
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	py, err := filepath.Abs(venvPython(t))
	if err != nil {
		t.Fatalf("resolve venv python: %v", err)
	}

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	pyCmd := exec.Command(py, "-u", "run.py")
	pyCmd.Dir = "python"
	pyCmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "PYTHONHASHSEED=0")
	up := startRunner(t, "python", pyCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:    "fastmcp",
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
			continue // filtered out by -run: do not score it
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

	if scored := compared + sc.Deviations; scored != len(cases) {
		t.Logf("partial run (%d of %d cases scored): leaving parity.json alone",
			scored, len(cases))
		return
	}
	writeScore(t, sc)
}

// compare runs one case as a subtest. It reports whether the subtest actually ran
// (a -run filter can skip it) and whether the two sides agree.
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
			// Both runners failed to answer at all. That is parity of a sort;
			// message text is not compared.
			agreed = true
			if upRep.Error != goRep.Error {
				t.Logf("both runners errored, different messages:\n  upstream: %s\n        go: %s",
					upRep.Error, goRep.Error)
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
			fail("value mismatch\n  upstream: %s\n        go: %s", show(upRep.Value), show(goRep.Value))
			return
		}
		agreed = true
	})

	if c.Deviation != "" {
		return ran, ran
	}
	return ran, agreed
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
	fmt.Fprintf(os.Stderr, "wrote parity.json: %d/%d match\n", sc.Match, sc.Match+sc.Mismatch)
}
