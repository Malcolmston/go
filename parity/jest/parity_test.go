// Package parity drives the upstream `expect` runner and the
// github.com/malcolmston/jest runner over the same case files and compares
// their answers.
//
// The comparable artefact for every case is a single boolean: whether the
// matcher passed. Failure-message text is deliberately not compared; message
// differences are recorded in COVERAGE.md instead.
package parity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

const caseTimeout = 20 * time.Second

// ---------------------------------------------------------------------------
// case files
// ---------------------------------------------------------------------------

type caseFile struct {
	Group    string          `json:"group"`
	Upstream string          `json:"upstream"`
	Note     string          `json:"note"`
	Cases    []parityCase    `json:"cases"`
	Raw      json.RawMessage `json:"-"`
}

type parityCase struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Not        bool              `json:"not"`
	Actual     json.RawMessage   `json:"actual"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

// wire is the exact object handed to both runners.
type wire struct {
	ID     string            `json:"id"`
	Fn     string            `json:"fn"`
	Not    bool              `json:"not"`
	Actual json.RawMessage   `json:"actual"`
	Args   []json.RawMessage `json:"args"`
}

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// pass extracts value.pass. A reply with ok:false has no pass at all.
func (r reply) pass() (bool, bool) {
	if !r.OK || len(r.Value) == 0 {
		return false, false
	}
	var v struct {
		Pass *bool `json:"pass"`
	}
	if err := json.Unmarshal(r.Value, &v); err != nil || v.Pass == nil {
		return false, false
	}
	return *v.Pass, true
}

func (r reply) describe() string {
	if !r.OK {
		return "error: " + r.Error
	}
	if p, ok := r.pass(); ok {
		return fmt.Sprintf("pass=%v", p)
	}
	return "malformed value: " + string(r.Value)
}

func loadCases(t *testing.T) ([]parityCase, map[string]string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no case files found in cases/")
	}
	var all []parityCase
	seen := map[string]string{}
	upstreams := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if cf.Upstream == "" {
			t.Fatalf("%s: missing pinned upstream version", p)
		}
		upstreams[cf.Group] = cf.Upstream
		for _, c := range cf.Cases {
			if c.ID == "" {
				t.Fatalf("%s: a case is missing an id", p)
			}
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstreams
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

// runner is one long-lived subprocess speaking JSON Lines. It is started once
// and every case is streamed through it. A runner is only ever restarted when it
// dies mid-case: a fatal Go runtime error such as a stack overflow cannot be
// recovered inside the runner, so the harness records that case as a crash and
// brings a fresh process up for the rest of the run instead of scoring every
// remaining case as a loss.
type runner struct {
	name     string
	newCmd   func() *exec.Cmd
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan string
	restarts int
	t        *testing.T
}

func startRunner(t *testing.T, name string, newCmd func() *exec.Cmd) *runner {
	t.Helper()
	r := &runner{name: name, newCmd: newCmd, t: t}
	r.spawn()
	t.Cleanup(r.stop)
	return r
}

func (r *runner) spawn() {
	r.t.Helper()
	cmd := r.newCmd()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.t.Fatalf("%s: stdin pipe: %v", r.name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.t.Fatalf("%s: stdout pipe: %v", r.name, err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		r.t.Fatalf("%s: start: %v", r.name, err)
	}
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	r.cmd, r.stdin, r.lines = cmd, stdin, lines
}

func (r *runner) stop() {
	if r.stdin != nil {
		_ = r.stdin.Close()
	}
	if r.cmd != nil {
		_ = r.cmd.Wait()
	}
}

func (r *runner) restart() {
	r.stop()
	r.restarts++
	r.spawn()
}

// ask sends one case and waits for exactly one reply, bounded by caseTimeout.
func (r *runner) ask(c parityCase) reply {
	w := wire{ID: c.ID, Fn: c.Fn, Not: c.Not, Actual: c.Actual, Args: c.Args}
	b, err := json.Marshal(w)
	if err != nil {
		return reply{ID: c.ID, OK: false, Error: "harness: marshal: " + err.Error()}
	}
	if _, err := r.stdin.Write(append(b, '\n')); err != nil {
		r.restart()
		return reply{ID: c.ID, OK: false, Error: "harness: " + r.name + " runner was not accepting input (crashed on an earlier case)"}
	}
	select {
	case line, open := <-r.lines:
		if !open {
			r.restart()
			return reply{ID: c.ID, OK: false, Error: "harness: " + r.name + " runner died on this case (unrecoverable runtime fault)"}
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{ID: c.ID, OK: false, Error: "harness: bad reply: " + line}
		}
		return rep
	case <-time.After(caseTimeout):
		r.restart()
		return reply{ID: c.ID, OK: false, Error: "harness: timeout after " + caseTimeout.String()}
	}
}

// ---------------------------------------------------------------------------
// score
// ---------------------------------------------------------------------------

type score struct {
	Library        string                 `json:"library"`
	GoModule       string                 `json:"goModule"`
	Upstream       map[string]string      `json:"upstream"`
	Generated      string                 `json:"generated"`
	Cases          int                    `json:"cases"`
	Compared       int                    `json:"compared"`
	Match          int                    `json:"match"`
	Mismatch       int                    `json:"mismatch"`
	Deviations     int                    `json:"deviations"`
	DeviationMatch int                    `json:"deviationsAgreeing"`
	ParityPercent  float64                `json:"parityPercent"`
	Restarts       map[string]int         `json:"runnerRestarts"`
	Groups         map[string]*groupScore `json:"groups"`
	Mismatches     []mismatch             `json:"mismatches"`
}

type groupScore struct {
	Cases      int `json:"cases"`
	Compared   int `json:"compared"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type mismatch struct {
	ID         string `json:"id"`
	Group      string `json:"group"`
	Fn         string `json:"fn"`
	Upstream   string `json:"upstream"`
	Go         string `json:"go"`
	UpstreamFn string `json:"upstreamFn"`
	GoFn       string `json:"goFn"`
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/jest")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return string(bytes.TrimSpace(out))
}

// ---------------------------------------------------------------------------
// the test
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "expect")); err != nil {
		t.Skip("node/node_modules/expect missing; run `npm install` in parity/jest/node")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found in PATH")
	}

	cases, upstreams := loadCases(t)

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}

	nodeRunner := startRunner(t, "node", func() *exec.Cmd {
		return exec.Command(node, filepath.Join("node", "run.js"))
	})
	goRunner := startRunner(t, "go", func() *exec.Cmd { return exec.Command(bin) })

	up := make([]reply, len(cases))
	gp := make([]reply, len(cases))
	for i, c := range cases {
		up[i] = nodeRunner.ask(c)
		gp[i] = goRunner.ask(c)
	}

	s := &score{
		Library:   "jest",
		GoModule:  goModuleVersion(t),
		Upstream:  upstreams,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Cases:     len(cases),
		Groups:    map[string]*groupScore{},
		Restarts:  map[string]int{"node": nodeRunner.restarts, "go": goRunner.restarts},
	}

	for i, c := range cases {
		gs := s.Groups[c.group]
		if gs == nil {
			gs = &groupScore{}
			s.Groups[c.group] = gs
		}
		gs.Cases++

		u, g := up[i], gp[i]
		agree := repliesAgree(u, g)

		if c.Deviation != "" {
			s.Deviations++
			gs.Deviations++
			if agree {
				s.DeviationMatch++
			}
			// A deviation is counted separately and never fails the suite, but
			// it is still reported so a deviation that quietly starts agreeing
			// (or a note that has gone stale) is visible.
			t.Run(c.ID, func(t *testing.T) {
				t.Logf("deviation (%s): upstream %s, go %s -- %s",
					c.Fn, u.describe(), g.describe(), c.Deviation)
			})
			continue
		}

		s.Compared++
		gs.Compared++
		if agree {
			s.Match++
			gs.Match++
		} else {
			s.Mismatch++
			gs.Mismatch++
			s.Mismatches = append(s.Mismatches, mismatch{
				ID: c.ID, Group: c.group, Fn: c.Fn,
				Upstream: u.describe(), Go: g.describe(),
				UpstreamFn: c.UpstreamFn, GoFn: c.GoFn,
			})
		}

		cc, uu, gg := c, u, g
		t.Run(cc.ID, func(t *testing.T) {
			if repliesAgree(uu, gg) {
				return
			}
			t.Errorf("%s (%s): upstream %s, go %s%s",
				cc.Fn, cc.group, uu.describe(), gg.describe(), noteSuffix(cc))
		})
	}

	if s.Compared > 0 {
		s.ParityPercent = float64(s.Match) / float64(s.Compared) * 100
	}
	sort.Slice(s.Mismatches, func(i, j int) bool { return s.Mismatches[i].ID < s.Mismatches[j].ID })

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("cases=%d compared=%d match=%d mismatch=%d deviations=%d parity=%.1f%%",
		s.Cases, s.Compared, s.Match, s.Mismatch, s.Deviations, s.ParityPercent)
}

func noteSuffix(c parityCase) string {
	if c.Note == "" {
		return ""
	}
	return " -- " + c.Note
}

// repliesAgree compares ok first, then the pass boolean. Error text is never
// compared: what matters is that both sides refused, not how they phrased it.
func repliesAgree(u, g reply) bool {
	if u.OK != g.OK {
		return false
	}
	if !u.OK {
		return true
	}
	up, uok := u.pass()
	gpv, gok := g.pass()
	if !uok || !gok {
		return false
	}
	return up == gpv
}
