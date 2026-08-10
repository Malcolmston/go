// Package parity drives the two matplotlib runners over the same cases and
// compares their answers. See HARNESS.md in the parent directory.
//
// Upstream (Python/matplotlib) is the oracle; nothing here contains a
// hand-written expectation.
package parity

import (
	"bufio"
	"encoding/json"
	"flag"
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

// defaultTol is the relative tolerance for numeric comparison. Cases that read
// the Go port's values back out of two-decimal SVG geometry override it with a
// per-case "tol" (1e-4); see cases/*.json.
const defaultTol = 1e-9

const caseTimeout = 30 * time.Second

// ------------------------------------------------------------------ case files

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []testCase `json:"cases"`
}

type testCase struct {
	ID         string          `json:"id"`
	Fn         string          `json:"fn"`
	Args       json.RawMessage `json:"args"`
	UpstreamFn string          `json:"upstreamFn"`
	GoFn       string          `json:"goFn"`
	Note       string          `json:"note"`
	Deviation  string          `json:"deviation"`
	Tol        *float64        `json:"tol"`

	group string
}

func (c testCase) tol() float64 {
	if c.Tol != nil {
		return *c.Tol
	}
	return defaultTol
}

func loadCases(t *testing.T) ([]testCase, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("globbing cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no case files in cases/")
	}
	var all []testCase
	upstream := ""
	seen := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parsing %s: %v", p, err)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q but %q was pinned earlier", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ------------------------------------------------------------------- runners

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// runner is one long-lived subprocess speaking JSON Lines.
type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	errs  chan error
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
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{
		name:  name,
		cmd:   cmd,
		stdin: stdin,
		lines: make(chan string, 64),
		errs:  make(chan error, 1),
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
		close(r.lines)
	}()
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})
	return r
}

// ask sends one case and waits for its reply, enforcing a per-case timeout so a
// hung runner fails that case rather than the suite.
func (r *runner) ask(c testCase) (reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn}
	if len(c.Args) > 0 {
		req["args"] = c.Args
	}
	b, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.stdin.Write(append(b, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return reply{}, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{}, fmt.Errorf("%s: undecodable reply %q: %w", r.name, line, err)
		}
		if rep.ID != c.ID {
			return reply{}, fmt.Errorf("%s: reply out of order: want id %q, got %q", r.name, c.ID, rep.ID)
		}
		return rep, nil
	case err := <-r.errs:
		return reply{}, fmt.Errorf("%s: read: %w", r.name, err)
	case <-time.After(caseTimeout):
		return reply{}, fmt.Errorf("%s: timed out after %s on case %s", r.name, caseTimeout, c.ID)
	}
}

// ------------------------------------------------------------------ comparison

// decode unmarshals a runner value, turning every JSON number into float64.
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

// equal is a normalising deep-equal: all JSON numbers are float64, so 1 and 1.0
// compare equal, and floats are compared with a relative tolerance.
func equal(a, b any, tol float64) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return false
		}
		return numEqual(av, bv, tol)
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
		// A non-finite sentinel on one side and a real number on the other is
		// never equal, so a type mismatch here is a genuine difference.
		return false
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equal(av[i], bv[i], tol) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, x := range av {
			y, present := bv[k]
			if !present || !equal(x, y, tol) {
				return false
			}
		}
		return true
	}
	return false
}

func numEqual(a, b, tol float64) bool {
	if a == b {
		return true
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= tol*scale
}

func render(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "<none>"
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// ------------------------------------------------------------------ the harness

// pythonCmd locates an interpreter that can import matplotlib, preferring a
// venv inside python/ if one was created there.
func pythonCmd(t *testing.T) []string {
	t.Helper()
	candidates := [][]string{
		{filepath.Join("python", ".venv", "bin", "python3")},
		{"python3"},
		{"python"},
	}
	for _, c := range candidates {
		bin := c[0]
		if !strings.ContainsRune(bin, os.PathSeparator) {
			p, err := exec.LookPath(bin)
			if err != nil {
				continue
			}
			bin = p
		} else if _, err := os.Stat(bin); err != nil {
			continue
		}
		probe := exec.Command(bin, "-c", "import matplotlib; print(matplotlib.__version__)")
		out, err := probe.CombinedOutput()
		if err != nil {
			t.Logf("skipping interpreter %s: %s", bin, strings.TrimSpace(string(out)))
			continue
		}
		t.Logf("using %s with matplotlib %s", bin, strings.TrimSpace(string(out)))
		return []string{bin}
	}
	return nil
}

type score struct {
	Library    string         `json:"library"`
	Upstream   string         `json:"upstream"`
	GoModule   string         `json:"goModule"`
	Tolerance  float64        `json:"tolerance"`
	Cases      int            `json:"cases"`
	Match      int            `json:"match"`
	Differs    int            `json:"differs"`
	Deviations int            `json:"deviations"`
	Errors     int            `json:"errors"`
	Percent    float64        `json:"percent"`
	Groups     map[string]*gs `json:"groups"`
	Failed     []string       `json:"failed"`
}

type gs struct {
	Cases   int `json:"cases"`
	Match   int `json:"match"`
	Differs int `json:"differs"`
}

func goModuleVersion(t *testing.T) string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "github.com/malcolmston/matplotlib" {
			return f[0] + "@" + f[1]
		}
		if len(f) >= 3 && f[0] == "require" && f[1] == "github.com/malcolmston/matplotlib" {
			return f[1] + "@" + f[2]
		}
	}
	return "unknown"
}

func TestParity(t *testing.T) {
	cases, upstream := loadCases(t)

	py := pythonCmd(t)
	if py == nil {
		t.Skip("no python3 with matplotlib available; skipping parity comparison")
	}

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the Go runner: %v\n%s", err, out)
	}

	up := startRunner(t, "python", exec.Command(py[0], filepath.Join("python", "run.py")))
	goRun := startRunner(t, "go", exec.Command(bin))

	sc := &score{
		Library:   "matplotlib",
		Upstream:  upstream,
		GoModule:  goModuleVersion(t),
		Tolerance: defaultTol,
		Cases:     len(cases),
		Groups:    map[string]*gs{},
	}

	for _, c := range cases {
		g := sc.Groups[c.group]
		if g == nil {
			g = &gs{}
			sc.Groups[c.group] = g
		}
		g.Cases++

		upRep, upErr := up.ask(c)
		goRep, goErr := goRun.ask(c)

		ok := t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				t.Fatalf("upstream runner failure: %v", upErr)
			}
			if goErr != nil {
				t.Fatalf("go runner failure: %v", goErr)
			}
			if upRep.OK != goRep.OK {
				t.Errorf("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
					upRep.OK, firstNonEmpty(upRep.Error, render(upRep.Value)),
					goRep.OK, firstNonEmpty(goRep.Error, render(goRep.Value)))
				return
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is not compared.
				return
			}
			uv, err := decode(upRep.Value)
			if err != nil {
				t.Fatalf("undecodable upstream value: %v", err)
			}
			gv, err := decode(goRep.Value)
			if err != nil {
				t.Fatalf("undecodable go value: %v", err)
			}
			if !equal(uv, gv, c.tol()) {
				t.Errorf("value mismatch (tol %g)\n  upstream: %s\n        go: %s",
					c.tol(), render(upRep.Value), render(goRep.Value))
			}
		})

		switch {
		case upErr != nil || goErr != nil:
			sc.Errors++
			sc.Failed = append(sc.Failed, c.ID)
			g.Differs++
		case ok && c.Deviation != "":
			sc.Deviations++
			g.Match++
		case ok:
			sc.Match++
			g.Match++
		default:
			sc.Differs++
			g.Differs++
			sc.Failed = append(sc.Failed, c.ID)
		}
	}

	if sc.Cases > 0 {
		sc.Percent = math.Round(float64(sc.Match)/float64(sc.Cases)*10000) / 100
	}
	// Only a full run produces a meaningful score; a -run filter would record
	// the skipped subtests as matches.
	if f := flag.Lookup("test.run"); f == nil || f.Value.String() == "" {
		b, err := json.MarshalIndent(sc, "", "  ")
		if err != nil {
			t.Fatalf("encoding parity.json: %v", err)
		}
		if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
			t.Fatalf("writing parity.json: %v", err)
		}
	} else {
		t.Logf("-run filter active: not rewriting parity.json")
	}
	t.Logf("parity: %d/%d cases match (%.2f%%), %d differ, %d runner errors",
		sc.Match, sc.Cases, sc.Percent, sc.Differs, sc.Errors)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
