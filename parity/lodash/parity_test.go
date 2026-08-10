// Package lodashparity drives the upstream (node) and port (go) runners over
// the same cases and compares their answers. See ../HARNESS.md.
package lodashparity

import (
	"bufio"
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

const caseTimeout = 10 * time.Second

// ---------------------------------------------------------------------------
// cases
// ---------------------------------------------------------------------------

type caseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
	Cases    []tCase `json:"cases"`
}

type tCase struct {
	ID         string          `json:"id"`
	Fn         string          `json:"fn"`
	Args       json.RawMessage `json:"args"`
	UpstreamFn string          `json:"upstreamFn"`
	GoFn       string          `json:"goFn"`
	Note       string          `json:"note"`
	Deviation  string          `json:"deviation"`

	group    string
	upstream string
}

func loadCases(t *testing.T) ([]tCase, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no case files in cases/")
	}
	var all []tCase
	seen := map[string]string{}
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
			t.Fatalf("%s pins upstream %q but %q was already pinned", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			c.upstream = cf.Upstream
			if c.Args == nil {
				c.Args = json.RawMessage("[]")
			}
			all = append(all, c)
		}
	}
	return all, upstream
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error"`
}

type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	done  chan struct{}
	dead  bool
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s stdin: %v", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s stdout: %v", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s runner: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 64), done: make(chan struct{})}
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return r
}

// ask sends one case and waits for the reply with the matching id.
func (r *runner) ask(id string, payload []byte) (reply, error) {
	if r.dead {
		return reply{}, fmt.Errorf("%s runner is dead", r.name)
	}
	if _, err := r.stdin.Write(append(payload, '\n')); err != nil {
		r.dead = true
		return reply{}, fmt.Errorf("write to %s runner: %w", r.name, err)
	}
	deadline := time.After(caseTimeout)
	for {
		select {
		case line, ok := <-r.lines:
			if !ok {
				r.dead = true
				return reply{}, fmt.Errorf("%s runner exited", r.name)
			}
			var rep reply
			if err := json.Unmarshal([]byte(line), &rep); err != nil {
				return reply{}, fmt.Errorf("%s runner emitted non-JSON %q: %w", r.name, line, err)
			}
			if rep.ID != id {
				// a stale reply from a timed-out case; keep draining
				continue
			}
			return rep, nil
		case <-deadline:
			r.dead = true
			return reply{}, fmt.Errorf("%s runner timed out after %s", r.name, caseTimeout)
		}
	}
}

// ---------------------------------------------------------------------------
// comparison
// ---------------------------------------------------------------------------

// deepEqual compares two decoded JSON values. All JSON numbers decode to
// float64, so 1 and 1.0 are already the same value.
func deepEqual(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
		return x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !deepEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, present := y[k]
			if !present || !deepEqual(v, w) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func render(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// score
// ---------------------------------------------------------------------------

type score struct {
	Library      string         `json:"library"`
	GoModule     string         `json:"goModule"`
	GoVersion    string         `json:"goModuleVersion"`
	Upstream     string         `json:"upstream"`
	Ecosystem    string         `json:"ecosystem"`
	Cases        int            `json:"cases"`
	Match        int            `json:"match"`
	Mismatch     int            `json:"mismatch"`
	Deviations   int            `json:"deviations"`
	BothFailed   int            `json:"bothFailed"`
	ParityPct    float64        `json:"parityPercent"`
	Groups       map[string]int `json:"casesByGroup"`
	Mismatched   []string       `json:"mismatchedCases"`
	DeviationIDs []string       `json:"deviationCases"`
}

func goModuleVersion(t *testing.T) string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "require "))
		if strings.HasPrefix(line, "github.com/malcolmston/lodash ") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				return f[1]
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	cases, upstream := loadCases(t)

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "lodash", "package.json")); err != nil {
		npm, nerr := exec.LookPath("npm")
		if nerr != nil {
			t.Skipf("upstream lodash not installed and npm is missing: %v", nerr)
		}
		install := exec.Command(npm, "install", "--no-audit", "--no-fund")
		install.Dir = "node"
		install.Stdout, install.Stderr = os.Stderr, os.Stderr
		if err := install.Run(); err != nil {
			t.Skipf("npm install in node/ failed: %v", err)
		}
	}

	// Confirm the pinned upstream version is the one actually installed.
	verOut, err := exec.Command(node, "-p", "require('./node_modules/lodash/package.json').version").Output()
	if err == nil {
		got := "lodash@" + strings.TrimSpace(string(verOut))
		if got != upstream {
			t.Fatalf("cases pin %s but node/node_modules has %s", upstream, got)
		}
	}

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build go runner: %v", err)
	}

	nodeCmd := exec.Command(node, "run.js")
	nodeCmd.Dir = "node"
	up := startRunner(t, "node", nodeCmd)
	gp := startRunner(t, "go", exec.Command(bin))

	sc := score{
		Library:   "lodash",
		GoModule:  "github.com/malcolmston/lodash",
		GoVersion: goModuleVersion(t),
		Upstream:  upstream,
		Ecosystem: "node",
		Cases:     len(cases),
		Groups:    map[string]int{},
	}

	for _, c := range cases {
		c := c
		sc.Groups[c.group]++
		payload, err := json.Marshal(struct {
			ID   string          `json:"id"`
			Fn   string          `json:"fn"`
			Args json.RawMessage `json:"args"`
		}{c.ID, c.Fn, c.Args})
		if err != nil {
			t.Fatalf("%s: marshal request: %v", c.ID, err)
		}

		upRep, upErr := up.ask(c.ID, payload)
		goRep, goErr := gp.ask(c.ID, payload)

		// Decide the outcome first, then report it: a documented deviation is
		// logged, not failed, but it never counts as a match.
		var problem string
		switch {
		case upErr != nil:
			problem = fmt.Sprintf("upstream runner: %v", upErr)
		case goErr != nil:
			problem = fmt.Sprintf("go runner: %v", goErr)
		case upRep.OK != goRep.OK:
			problem = fmt.Sprintf("ok differs: upstream ok=%v (%s), go ok=%v (%s)\n  upstream value: %s\n  go value:       %s",
				upRep.OK, upRep.Error, goRep.OK, goRep.Error, render(upRep.Value), render(goRep.Value))
		case upRep.OK && !deepEqual(upRep.Value, goRep.Value):
			problem = fmt.Sprintf("value differs for %s (%s vs %s)\n  upstream: %s\n  go:       %s",
				c.Fn, c.UpstreamFn, c.GoFn, render(upRep.Value), render(goRep.Value))
		}

		t.Run(c.ID, func(t *testing.T) {
			switch {
			case problem == "":
				// pass
			case c.Deviation != "":
				t.Logf("deviation (%s): %s", c.Deviation, problem)
			default:
				t.Errorf("%s", problem)
			}
		})

		switch {
		case problem != "":
			if c.Deviation != "" {
				sc.Deviations++
				sc.DeviationIDs = append(sc.DeviationIDs, c.ID)
			} else {
				sc.Mismatch++
				sc.Mismatched = append(sc.Mismatched, c.ID)
			}
		default:
			sc.Match++
			if !upRep.OK {
				sc.BothFailed++
			}
		}
	}

	compared := sc.Match + sc.Mismatch
	if compared > 0 {
		sc.ParityPct = math.Round(float64(sc.Match)/float64(compared)*10000) / 100
	}
	sort.Strings(sc.Mismatched)
	sort.Strings(sc.DeviationIDs)

	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("marshal parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("cases=%d match=%d mismatch=%d deviations=%d bothFailed=%d parity=%.2f%%",
		sc.Cases, sc.Match, sc.Mismatch, sc.Deviations, sc.BothFailed, sc.ParityPct)
}

// TestDeviationsAreDeliberate guards against a case being marked as a deviation
// without an explanation.
func TestDeviationsAreDeliberate(t *testing.T) {
	cases, _ := loadCases(t)
	for _, c := range cases {
		if c.Deviation != "" && len(c.Deviation) < 10 {
			t.Errorf("%s: deviation needs a reason, got %q", c.ID, c.Deviation)
		}
	}
}
