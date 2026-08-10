// Package morganparity is the cross-language parity harness for
// github.com/malcolmston/morgan against upstream expressjs/morgan.
//
// It starts two subprocess runners — ../morgan/node/run.js (upstream oracle) and
// ./go/run.go (the port) — streams every case in cases/*.json through both over
// JSON Lines, and compares the access-log line each one produced. Expectations
// are never hand-written: upstream is the oracle.
//
// Run with:
//
//	GOWORK=off go test ./parity/morgan/
package morganparity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

const caseTimeout = 20 * time.Second

// ---------------------------------------------------------------------------
// case files
// ---------------------------------------------------------------------------

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []testCase `json:"cases"`
}

type testCase struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

func loadCases(t *testing.T, dir string) []testCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no case files in %s", dir)
	}

	var all []testCase
	seen := map[string]string{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(raw, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if cf.Upstream == "" {
			t.Fatalf("%s: missing pinned \"upstream\" version", p)
		}
		for _, c := range cf.Cases {
			if c.ID == "" {
				t.Fatalf("%s: case with empty id", p)
			}
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

type wireResponse struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	errs  chan error
	once  sync.Once
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

	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 8), errs: make(chan error, 1)}
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			select {
			case r.errs <- err:
			default:
			}
		}
	}()
	t.Cleanup(r.stop)
	return r
}

func (r *runner) stop() {
	r.once.Do(func() {
		_ = r.stdin.Close()
		done := make(chan struct{})
		go func() { _ = r.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = r.cmd.Process.Kill()
			<-done
		}
	})
}

// call sends one case and waits for exactly one reply, enforcing a per-case
// timeout so a hung runner fails that case and not the whole suite.
func (r *runner) call(c testCase) (wireResponse, error) {
	payload, err := json.Marshal(map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args})
	if err != nil {
		return wireResponse{}, err
	}
	if _, err := r.stdin.Write(append(payload, '\n')); err != nil {
		return wireResponse{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return wireResponse{}, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var resp wireResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return wireResponse{}, fmt.Errorf("%s: bad reply %q: %w", r.name, line, err)
		}
		if resp.ID != c.ID {
			return wireResponse{}, fmt.Errorf("%s: reply out of order: got id %q, want %q", r.name, resp.ID, c.ID)
		}
		return resp, nil
	case err := <-r.errs:
		return wireResponse{}, fmt.Errorf("%s: read: %w", r.name, err)
	case <-time.After(caseTimeout):
		return wireResponse{}, fmt.Errorf("%s: timeout after %s waiting for %s", r.name, caseTimeout, c.ID)
	}
}

// ---------------------------------------------------------------------------
// comparison
// ---------------------------------------------------------------------------

// decode turns a raw JSON value into normalised Go values: every number becomes
// float64, so 1 and 1.0 compare equal.
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

func equalValues(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && (av == bv || (math.IsNaN(av) && math.IsNaN(bv)))
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalValues(av[i], bv[i]) {
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
			ov, present := bv[k]
			if !present || !equalValues(v, ov) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func render(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// score
// ---------------------------------------------------------------------------

type caseResult struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Status   string `json:"status"` // match | mismatch | deviation | harness-error
	Upstream string `json:"upstream,omitempty"`
	Go       string `json:"go,omitempty"`
}

type score struct {
	Library      string            `json:"library"`
	Upstream     string            `json:"upstream"`
	GoModule     string            `json:"goModule"`
	Runners      map[string]string `json:"runners"`
	Cases        int               `json:"cases"`
	Match        int               `json:"match"`
	Mismatch     int               `json:"mismatch"`
	Deviation    int               `json:"deviation"`
	HarnessError int               `json:"harnessError"`
	ParityPct    float64           `json:"parityPercent"`
	Groups       map[string]struct {
		Cases    int `json:"cases"`
		Match    int `json:"match"`
		Mismatch int `json:"mismatch"`
	} `json:"groups"`
	Mismatches []caseResult `json:"mismatches"`
	Note       string       `json:"note"`
}

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

func goModuleVersion(t *testing.T) string {
	t.Helper()
	out, err := runGo(t, "list", "-m", "github.com/malcolmston/morgan")
	if err != nil {
		return "unknown"
	}
	return strings.ReplaceAll(strings.TrimSpace(out), " ", "@")
}

func runGo(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func upstreamVersion(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("node", "node_modules", "morgan", "package.json"))
	if err != nil {
		return "unknown"
	}
	var pkg struct{ Version string }
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "unknown"
	}
	return "morgan@" + pkg.Version
}

func TestParity(t *testing.T) {
	cases := loadCases(t, "cases")

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not found in PATH, skipping upstream comparison: %v", err)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "morgan")); err != nil {
		t.Skipf("upstream not installed: run (cd parity/morgan/node && npm install)")
	}

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "morgan-parity-go-runner")
	if out, err := runGo(t, "build", "-o", bin, "./go"); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}

	goCmd := exec.Command(bin)
	nodeCmd := exec.Command(node, filepath.Join("node", "run.js"))

	up := startRunner(t, "node", nodeCmd)
	port := startRunner(t, "go", goCmd)

	results := make([]caseResult, 0, len(cases))

	for _, c := range cases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			res := caseResult{ID: c.ID, Group: c.group}

			upResp, upErr := up.call(c)
			goResp, goErr := port.call(c)
			if upErr != nil || goErr != nil {
				res.Status = "harness-error"
				results = append(results, res)
				if upErr != nil {
					t.Errorf("upstream runner: %v", upErr)
				}
				if goErr != nil {
					t.Errorf("go runner: %v", goErr)
				}
				return
			}

			upVal, err1 := decode(upResp.Value)
			goVal, err2 := decode(goResp.Value)
			if err1 != nil || err2 != nil {
				res.Status = "harness-error"
				results = append(results, res)
				t.Errorf("undecodable value (upstream=%v go=%v)", err1, err2)
				return
			}

			describe := func(r wireResponse, v any) string {
				if !r.OK {
					return "error: " + r.Error
				}
				return render(v)
			}
			res.Upstream = describe(upResp, upVal)
			res.Go = describe(goResp, goVal)

			same := upResp.OK == goResp.OK && (!upResp.OK || equalValues(upVal, goVal))

			switch {
			case same:
				res.Status = "match"
			case c.Deviation != "":
				res.Status = "deviation"
			default:
				res.Status = "mismatch"
			}
			results = append(results, res)

			switch res.Status {
			case "mismatch":
				t.Errorf("mismatch\n  upstream: %s\n  go:       %s\n  note:     %s",
					res.Upstream, res.Go, c.Note)
			case "deviation":
				t.Logf("deliberate deviation (%s)\n  upstream: %s\n  go:       %s",
					c.Deviation, res.Upstream, res.Go)
			}
		})
	}

	writeScore(t, cases, results)
}

func writeScore(t *testing.T, cases []testCase, results []caseResult) {
	t.Helper()

	s := score{
		Library:  "morgan",
		Upstream: upstreamVersion(t),
		GoModule: goModuleVersion(t),
		Runners: map[string]string{
			"upstream": "node node/run.js",
			"go":       "go run ./go",
		},
		Cases: len(results),
		Groups: map[string]struct {
			Cases    int `json:"cases"`
			Match    int `json:"match"`
			Mismatch int `json:"mismatch"`
		}{},
		Note: "Regenerated by TestParity. :response-time/:total-time values and :date output are normalised to placeholders on both sides before comparison; see COVERAGE.md.",
	}
	if len(cases) != len(results) {
		s.Note += fmt.Sprintf(" WARNING: %d of %d cases produced no result.", len(cases)-len(results), len(cases))
	}

	for _, r := range results {
		g := s.Groups[r.Group]
		g.Cases++
		switch r.Status {
		case "match":
			s.Match++
			g.Match++
		case "mismatch":
			s.Mismatch++
			g.Mismatch++
			s.Mismatches = append(s.Mismatches, r)
		case "deviation":
			s.Deviation++
		default:
			s.HarnessError++
		}
		s.Groups[r.Group] = g
	}

	compared := s.Match + s.Mismatch
	if compared > 0 {
		s.ParityPct = math.Round(float64(s.Match)/float64(compared)*10000) / 100
	}

	sort.Slice(s.Mismatches, func(i, j int) bool { return s.Mismatches[i].ID < s.Mismatches[j].ID })

	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(buf, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("parity: %d/%d cases match (%.2f%%), %d mismatch, %d deviation, %d harness error",
		s.Match, compared, s.ParityPct, s.Mismatch, s.Deviation, s.HarnessError)
}
