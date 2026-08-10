// Package parity drives the upstream oracle (the real SQLite, via the sqlite3
// 3.45.3 CLI) and the Go port (github.com/malcolmston/sqlite) over the same
// case files and compares their answers. Upstream is the oracle: nothing here
// hand-writes an expected value.
//
// Run with:
//
//	GOWORK=off go test ./parity/sqlite/
//
// The suite skips (never fails) when the sqlite3 CLI or python3 is unavailable,
// so a checkout with only Go stays green.
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
	upstreamPin = "sqlite3@3.45.3"
	caseTimeout = 30 * time.Second
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
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	readEr chan error

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
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		lines:  make(chan string, 64),
		readEr: make(chan error, 1),
	}

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		r.readEr <- sc.Err()
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
// numbers become float64, so 1 and 1.0 compare equal. That normalisation is
// applied to the payload of a value cell; the type tag next to it ("int" vs
// "real") is what keeps the INTEGER/REAL distinction observable.
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

// show renders a value compactly for test output.
func show(raw json.RawMessage) string {
	v, err := decode(raw)
	if err != nil {
		return fmt.Sprintf("<unparseable %s>", raw)
	}
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
	// SilentWrongAnswers are the mismatches where both sides succeeded but
	// disagreed on the rows: the port answered a query with no error and the
	// answer was wrong. That is the highest-value class of divergence.
	SilentWrongAnswers []string `json:"silentWrongAnswers"`
	// UnsupportedSQL are the mismatches where upstream accepted the script and
	// the port rejected it.
	UnsupportedSQL []string `json:"unsupportedSql"`
	// ExtraAccepted are the mismatches where upstream rejected the script and
	// the port accepted it.
	ExtraAccepted []string `json:"extraAccepted"`
}

var modLine = regexp.MustCompile(`github\.com/malcolmston/sqlite\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/sqlite " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not found in PATH; skipping cross-language parity")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH; the upstream runner needs it")
	}
	assertUpstreamVersion(t, sqlite3)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	upCmd := exec.Command(python, filepath.Join("c", "run.py"))
	upCmd.Env = append(os.Environ(), "SQLITE3_BIN="+sqlite3, "PYTHONUNBUFFERED=1")
	up := startRunner(t, "c", upCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:            "sqlite",
		Upstream:           upstreamPin,
		GoModule:           goModuleVersion(),
		Cases:              len(cases),
		Groups:             map[string]groupScore{},
		Mismatches:         []string{},
		SilentWrongAnswers: []string{},
		UnsupportedSQL:     []string{},
		ExtraAccepted:      []string{},
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
			switch {
			case upRep.OK && goRep.OK:
				sc.SilentWrongAnswers = append(sc.SilentWrongAnswers, c.ID)
			case upRep.OK && !goRep.OK:
				sc.UnsupportedSQL = append(sc.UnsupportedSQL, c.ID)
			case !upRep.OK && goRep.OK:
				sc.ExtraAccepted = append(sc.ExtraAccepted, c.ID)
			}
		}
		sc.Groups[c.group] = g
	}

	compared := sc.Match + sc.Mismatch
	if compared > 0 {
		sc.ParityPercent = float64(sc.Match) * 100 / float64(compared)
	}

	t.Logf("parity: %d/%d cases match (%.2f%%), %d deviations",
		sc.Match, compared, sc.ParityPercent, sc.Deviations)
	t.Logf("divergences: %d silent wrong answers, %d unsupported-by-port, %d accepted-only-by-port",
		len(sc.SilentWrongAnswers), len(sc.UnsupportedSQL), len(sc.ExtraAccepted))
	if len(sc.SilentWrongAnswers) > 0 {
		t.Logf("silent wrong answers: %s", strings.Join(sc.SilentWrongAnswers, ", "))
	}

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
			fail("ok mismatch (%s): upstream ok=%v (%s), go ok=%v (%s)",
				c.UpstreamFn, upRep.OK, describe(upRep), goRep.OK, describe(goRep))
			return
		}
		if !upRep.OK {
			// Both rejected the script: that is parity. Message text is not
			// compared; differences are recorded in COVERAGE.md.
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
			fail("SILENT WRONG ANSWER (%s): both sides succeeded, rows differ\n  upstream: %s\n        go: %s",
				c.UpstreamFn, show(upRep.Value), show(goRep.Value))
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

var versionLine = regexp.MustCompile(`^(\d+\.\d+\.\d+)`)

// assertUpstreamVersion checks the installed sqlite3 CLI matches the pin, so the
// score can never be attributed to the wrong oracle.
func assertUpstreamVersion(t *testing.T, sqlite3 string) {
	t.Helper()
	want := strings.SplitN(upstreamPin, "@", 2)[1]

	out, err := exec.Command(sqlite3, "--version").Output()
	if err != nil {
		t.Skipf("cannot run `sqlite3 --version`: %v", err)
	}
	m := versionLine.FindSubmatch(out)
	if m == nil {
		t.Fatalf("cannot parse `sqlite3 --version` output %q", out)
	}
	if got := string(m[1]); got != want {
		t.Fatalf("installed sqlite3 is %s, cases pin %s", got, want)
	}
}
