// Package parity drives the upstream Node (negotiator) runner and the Go
// (github.com/malcolmston/express/negotiator) runner over an identical set of
// cases and compares their answers. See ../../../HARNESS.md.
//
// This is a *nested* harness: the oracle is the nested package's own upstream
// (negotiator), not express, and it is pinned separately in node/package.json.
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
	"sort"
	"strings"
	"testing"
	"time"
)

// library is the harness identity written into parity.json.
const (
	library     = "express/negotiator"
	npmPackage  = "negotiator"
	caseTimeout = 10 * time.Second
)

// ---------------------------------------------------------------------------
// case files
// ---------------------------------------------------------------------------

type caseSpec struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []caseSpec `json:"cases"`
}

func loadCases(t *testing.T) (cases []caseSpec, upstream string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no case files in cases/")
	}
	seen := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q but another file pins %q", f, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, f)
			}
			seen[c.ID] = f
			c.group = cf.Group
			cases = append(cases, c)
		}
	}
	return cases, upstream
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   chan string
	readErr chan error
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
	cmd.Stderr = &prefixWriter{prefix: name + ": "}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 8), readErr: make(chan error, 1)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		r.readErr <- sc.Err()
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
	})
	return r
}

func (r *runner) ask(c caseSpec) (*reply, error) {
	line, err := json.Marshal(map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args})
	if err != nil {
		return nil, err
	}
	if _, err := r.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case l, ok := <-r.lines:
		if !ok {
			return nil, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(l), &rep); err != nil {
			return nil, fmt.Errorf("%s: bad reply %q: %w", r.name, l, err)
		}
		if rep.ID != c.ID {
			return nil, fmt.Errorf("%s: reply out of order: want %s got %s", r.name, c.ID, rep.ID)
		}
		return &rep, nil
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("%s: timeout after %s waiting for %s", r.name, caseTimeout, c.ID)
	}
}

type prefixWriter struct{ prefix string }

func (w *prefixWriter) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if l != "" {
			fmt.Fprintln(os.Stderr, w.prefix+l)
		}
	}
	return len(p), nil
}

// ---------------------------------------------------------------------------
// comparison
// ---------------------------------------------------------------------------

// decode turns a raw JSON value into the generic form used for comparison, so
// that ints and floats (1 vs 1.0) compare equal — encoding/json makes every
// number a float64.
func decode(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func pretty(v any) string {
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
	Library    string         `json:"library"`
	Upstream   string         `json:"upstream"`
	GoModule   string         `json:"goModule"`
	GoPackage  string         `json:"goPackage"`
	GoVersion  string         `json:"goVersion"`
	Generated  string         `json:"generatedBy"`
	Total      int            `json:"total"`
	Match      int            `json:"match"`
	Mismatch   int            `json:"mismatch"`
	Deviations int            `json:"deviations"`
	Percent    float64        `json:"parityPercent"`
	Groups     map[string]*gp `json:"groups"`
	Failing    []string       `json:"failingCases"`
}

type gp struct {
	Total      int `json:"total"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

func goModuleVersion() string {
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/express")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not found in PATH; skipping %s parity (Go-only checkout)", library)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", npmPackage, "package.json")); err != nil {
		t.Skipf("upstream %s not installed; run `npm install` in parity/%s/node", npmPackage, library)
	}

	// Build the Go runner once. GOWORK=off: this harness is a separate module
	// outside the aggregator workspace.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}

	cases, upstream := loadCases(t)

	upCmd := exec.Command(node, filepath.Join("node", "run.js"))
	upCmd.Env = os.Environ()
	up := startRunner(t, "node", upCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gorun := startRunner(t, "go", goCmd)

	sc := &score{
		Library:   library,
		Upstream:  upstream,
		GoModule:  goModuleVersion(),
		GoPackage: "github.com/malcolmston/express/negotiator",
		GoVersion: strings.TrimSpace(runOut("go", "version")),
		Generated: "GOWORK=off go test ./parity/express/nested/negotiator/",
		Groups:    map[string]*gp{},
	}

	for _, c := range cases {
		// Runners are stateful subprocesses shared by every subtest, so cases
		// must stay sequential: no t.Parallel here.
		upRep, upErr := up.ask(c)
		goRep, goErr := gorun.ask(c)

		g := sc.Groups[c.group]
		if g == nil {
			g = &gp{}
			sc.Groups[c.group] = g
		}
		sc.Total++
		g.Total++

		// A declared deviation is scored separately from both a match and a
		// bug, as HARNESS.md requires: it never turns the suite red and it is
		// excluded from the parity denominator.
		deviation := c.Deviation != ""
		ok := true
		t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				ok = false
				t.Fatalf("upstream runner failure: %v", upErr)
			}
			if goErr != nil {
				ok = false
				t.Fatalf("go runner failure: %v", goErr)
			}
			if upRep.OK != goRep.OK {
				if deviation {
					t.Skipf("declared deviation (%s): upstream ok=%v (%s), go ok=%v (%s)",
						c.Deviation, upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				}
				ok = false
				t.Fatalf("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
					upRep.OK, upRep.Error, goRep.OK, goRep.Error)
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is not compared.
				return
			}
			uv, gv := decode(upRep.Value), decode(goRep.Value)
			if !reflect.DeepEqual(uv, gv) {
				if deviation {
					t.Skipf("declared deviation (%s)\n  upstream: %s\n        go: %s",
						c.Deviation, pretty(uv), pretty(gv))
				}
				ok = false
				t.Errorf("value mismatch\n  upstream: %s\n        go: %s", pretty(uv), pretty(gv))
			}
		})

		switch {
		case deviation:
			sc.Deviations++
			g.Deviations++
		case ok:
			sc.Match++
			g.Match++
		default:
			sc.Mismatch++
			g.Mismatch++
			sc.Failing = append(sc.Failing, c.ID)
		}
	}

	if compared := sc.Total - sc.Deviations; compared > 0 {
		sc.Percent = float64(sc.Match) * 100 / float64(compared)
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("%s parity: %d/%d cases match (%.1f%%), %d mismatches, %d declared deviations",
		library, sc.Match, sc.Total, sc.Percent, sc.Mismatch, sc.Deviations)
}

func runOut(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
