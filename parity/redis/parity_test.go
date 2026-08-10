// Package parity drives the upstream (real redis-server) runner and the Go port
// runner over the same JSON case files and compares their answers.
//
// Upstream is the oracle: nothing here hand-writes an expected value. Every case
// is a SCRIPT of commands against a fresh, empty database; the compared answer is
// the normalised reply to every command plus a canonical dump of every key, so a
// divergence in *state* shows up and not merely a single return value.
//
// The Go side is driven through Store.Do, the RESP dispatcher, which is the
// surface a real RESP client reaches via redis.NewServer.
//
// Run with:  GOWORK=off go test ./parity/redis/
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
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	caseTimeout  = 60 * time.Second
	buildTimeout = 5 * time.Minute
)

// ------------------------------------------------------------------ case files

type caseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
	Encoding string  `json:"encoding"`
	Cases    []tcase `json:"cases"`
}

type tcase struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

func loadCases(t *testing.T) []tcase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no case files in cases/")
	}
	var all []tcase
	seen := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q in %s and %s", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ---------------------------------------------------------------------- runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name string
	cmd  *exec.Cmd
	// pipe is the raw stdin pipe. It must be closed for the child to see EOF and
	// shut down its own subprocesses; exec.Cmd.Stdin stays nil when StdinPipe is
	// used, so the pipe has to be kept here rather than fished back out of cmd.
	pipe  io.WriteCloser
	stdin *bufio.Writer
	lines chan string
	errs  chan error
	once  sync.Once
}

func startRunner(t *testing.T, name string, bin string, args ...string) *runner {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin pipe: %v", name, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", name, err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{
		name:  name,
		cmd:   cmd,
		pipe:  in,
		stdin: bufio.NewWriter(in),
		lines: make(chan string, 4),
		errs:  make(chan error, 1),
	}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<26)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
		close(r.lines)
	}()
	t.Cleanup(func() { r.stop() })
	return r
}

func (r *runner) stop() {
	r.once.Do(func() {
		r.stdin.Flush()
		// Closing stdin is what lets the child exit on its own and run its
		// cleanup (the upstream runner shuts its redis-server down there).
		if r.pipe != nil {
			_ = r.pipe.Close()
		}
		if r.cmd.Process != nil {
			done := make(chan struct{})
			go func() { _ = r.cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				// Ask nicely first: SIGKILL would orphan the child's own
				// subprocesses.
				_ = r.cmd.Process.Signal(syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					_ = r.cmd.Process.Kill()
				}
			}
		}
	})
}

// call sends one case and waits for exactly one reply, with a per-case timeout.
func (r *runner) call(line string) (*reply, error) {
	if _, err := r.stdin.WriteString(line); err != nil {
		return nil, err
	}
	if err := r.stdin.WriteByte('\n'); err != nil {
		return nil, err
	}
	if err := r.stdin.Flush(); err != nil {
		return nil, err
	}
	select {
	case l, ok := <-r.lines:
		if !ok {
			return nil, fmt.Errorf("%s: runner closed its output", r.name)
		}
		var rp reply
		if err := json.Unmarshal([]byte(l), &rp); err != nil {
			return nil, fmt.Errorf("%s: bad reply %q: %v", r.name, truncate(l), err)
		}
		return &rp, nil
	case err := <-r.errs:
		return nil, fmt.Errorf("%s: read: %v", r.name, err)
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("%s: timed out after %s", r.name, caseTimeout)
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// ------------------------------------------------------------------- comparison

// normalise decodes JSON into interface{} so all numbers become float64 and
// 1 vs 1.0 compare equal.
func normalise(raw json.RawMessage) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func show(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return truncate(string(b))
}

// firstDiff walks two decoded script results and names the first divergence, so
// a failure message points at the offending step rather than dumping both trees.
func firstDiff(path string, a, b interface{}) string {
	if reflect.DeepEqual(a, b) {
		return ""
	}
	am, aok := a.(map[string]interface{})
	bm, bok := b.(map[string]interface{})
	if aok && bok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		var ks []string
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			if d := firstDiff(path+"."+k, am[k], bm[k]); d != "" {
				return d
			}
		}
	}
	as, aok := a.([]interface{})
	bs, bok := b.([]interface{})
	if aok && bok {
		n := len(as)
		if len(bs) < n {
			n = len(bs)
		}
		for i := 0; i < n; i++ {
			if d := firstDiff(fmt.Sprintf("%s[%d]", path, i), as[i], bs[i]); d != "" {
				return d
			}
		}
		if len(as) != len(bs) {
			return fmt.Sprintf("%s: length %d vs %d", path, len(as), len(bs))
		}
		return ""
	}
	return fmt.Sprintf("%s: upstream=%s go=%s", path, show(a), show(b))
}

// ----------------------------------------------------------------------- report

type groupScore struct {
	Cases      int `json:"cases"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type report struct {
	Library       string                `json:"library"`
	Upstream      string                `json:"upstream"`
	UpstreamKind  string                `json:"upstreamEcosystem"`
	GoModule      string                `json:"goModule"`
	Cases         int                   `json:"cases"`
	Match         int                   `json:"match"`
	Mismatch      int                   `json:"mismatch"`
	Deviations    int                   `json:"deviations"`
	ParityPercent float64               `json:"parityPercent"`
	Groups        map[string]groupScore `json:"groups"`
	Mismatches    []string              `json:"mismatches"`
	DeviationIDs  []string              `json:"deviationCases"`
	Surface       *surface              `json:"commandSurface,omitempty"`
}

// surface is the mechanically derived upstream command inventory versus the set
// of commands the port's RESP dispatcher actually carries.
type surface struct {
	UpstreamCount    int      `json:"upstreamCommandCount"`
	DispatchableGo   int      `json:"dispatchableInGo"`
	SurfacePercent   float64  `json:"dispatchablePercent"`
	Dispatchable     []string `json:"dispatchable"`
	NotDispatchableN int      `json:"notDispatchable"`
}

// commandSurface asks the oracle for COMMAND COUNT/COMMAND LIST and then probes
// Store.Do with every one of those names, so the reachable-command list is
// measured rather than remembered.
func commandSurface(t *testing.T, up, port *runner) *surface {
	t.Helper()
	line, _ := json.Marshal(map[string]interface{}{"id": "__commands__", "fn": "commands"})
	rep, err := up.call(string(line))
	if err != nil || !rep.OK {
		t.Errorf("upstream command surface: %v (%s)", err, rep.GetError())
		return nil
	}
	var cs struct {
		Count int      `json:"commandCount"`
		Cmds  []string `json:"commands"`
	}
	if err := json.Unmarshal(rep.Value, &cs); err != nil {
		t.Errorf("upstream command surface: %v", err)
		return nil
	}
	pline, _ := json.Marshal(map[string]interface{}{
		"id": "__probe__", "fn": "probe", "args": []interface{}{cs.Cmds}})
	prep, err := port.call(string(pline))
	if err != nil || !prep.OK {
		t.Errorf("go dispatcher probe: %v (%s)", err, prep.GetError())
		return nil
	}
	var got []string
	if err := json.Unmarshal(prep.Value, &got); err != nil {
		t.Errorf("go dispatcher probe: %v", err)
		return nil
	}
	s := &surface{
		UpstreamCount:    len(cs.Cmds),
		DispatchableGo:   len(got),
		Dispatchable:     got,
		NotDispatchableN: len(cs.Cmds) - len(got),
	}
	if s.UpstreamCount > 0 {
		s.SurfacePercent = 100 * float64(s.DispatchableGo) / float64(s.UpstreamCount)
	}
	if cs.Count != 0 && cs.Count != len(cs.Cmds) {
		t.Logf("COMMAND COUNT reports %d, COMMAND LIST yields %d top-level names",
			cs.Count, len(cs.Cmds))
	}
	t.Logf("upstream command surface: %d commands; %d (%.1f%%) reachable through Store.Do",
		s.UpstreamCount, s.DispatchableGo, s.SurfacePercent)
	return s
}

func (r *reply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

// --------------------------------------------------------------------- helpers

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/redis")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "github.com/malcolmston/redis (version unknown)"
	}
	return strings.TrimSpace(string(out))
}

func buildGoRunner(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-runner")
	cmd := exec.Command("go", "build", "-o", bin, "./go")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the Go runner failed: %v\n%s", err, out)
	}
	return bin
}

// upstreamCmd locates the oracle runner. A missing redis-server or python3 skips
// the suite rather than failing it.
func upstreamCmd(t *testing.T) (string, []string) {
	t.Helper()
	if _, err := exec.LookPath("redis-server"); err != nil {
		t.Skip("redis-server not found in PATH; skipping the redis parity suite")
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH; skipping the redis parity suite")
	}
	script, err := filepath.Abs(filepath.Join("c", "run.py"))
	if err != nil {
		t.Fatalf("locating c/run.py: %v", err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("c/run.py missing: %v", err)
	}
	return py, []string{"-u", script}
}

func redisServerVersion() string {
	out, err := exec.Command("redis-server", "--version").Output()
	if err != nil {
		return "redis-server (version unknown)"
	}
	f := strings.Fields(strings.TrimSpace(string(out)))
	for _, tok := range f {
		if strings.HasPrefix(tok, "v=") {
			return "redis-server@" + strings.TrimPrefix(tok, "v=")
		}
	}
	return strings.TrimSpace(string(out))
}

// ------------------------------------------------------------------------ test

func TestParity(t *testing.T) {
	cases := loadCases(t)

	upBin, upArgs := upstreamCmd(t)
	goBin := buildGoRunner(t)

	up := startRunner(t, "redis-server", upBin, upArgs...)
	port := startRunner(t, "go", goBin)

	rep := report{
		Library:      "redis",
		Upstream:     redisServerVersion(),
		UpstreamKind: "c",
		GoModule:     goModuleVersion(t),
		Groups:       map[string]groupScore{},
	}
	rep.Surface = commandSurface(t, up, port)

	skipped := 0
	for _, c := range cases {
		c := c
		line, err := json.Marshal(map[string]interface{}{
			"id":   c.ID,
			"fn":   c.Fn,
			"args": c.Args,
		})
		if err != nil {
			t.Fatalf("%s: marshal: %v", c.ID, err)
		}

		// Both runners are fed in lockstep, outside of t.Run, so the two streams
		// stay aligned even when a subtest reports a failure.
		upRep, upErr := up.call(string(line))
		goRep, goErr := port.call(string(line))

		matched, ran := false, false
		t.Run(c.ID, func(t *testing.T) {
			ran = true
			if upErr != nil {
				t.Fatalf("upstream runner: %v", upErr)
			}
			if goErr != nil {
				t.Fatalf("go runner: %v", goErr)
			}
			if upRep.ID != c.ID || goRep.ID != c.ID {
				t.Fatalf("reply id mismatch: upstream=%q go=%q want %q", upRep.ID, goRep.ID, c.ID)
			}

			fail := func(format string, args ...interface{}) {
				if c.Deviation != "" {
					t.Logf("deviation (%s): "+format, append([]interface{}{c.Deviation}, args...)...)
					return
				}
				t.Errorf(format, args...)
			}

			if upRep.OK != goRep.OK {
				fail("case %s: ok differs: upstream=%v (%s) go=%v (%s)",
					c.ID, upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				return
			}
			if !upRep.OK {
				// Both runners failed the whole case: that is parity, but it is
				// also suspicious, so surface it.
				t.Logf("case %s: both runners reported ok:false (upstream=%q go=%q)",
					c.ID, upRep.Error, goRep.Error)
				matched = true
				return
			}
			upVal, err := normalise(upRep.Value)
			if err != nil {
				t.Fatalf("upstream value not JSON: %v", err)
			}
			goVal, err := normalise(goRep.Value)
			if err != nil {
				t.Fatalf("go value not JSON: %v", err)
			}
			if !reflect.DeepEqual(upVal, goVal) {
				fail("case %s (%s):\n  first divergence at %s\n  upstream %s = %s\n  go       %s = %s",
					c.ID, c.Note, firstDiff("$", upVal, goVal),
					c.UpstreamFn, show(upVal), c.GoFn, show(goVal))
				return
			}
			matched = true
		})

		if !ran {
			// filtered out by -run: do not score it
			skipped++
			continue
		}
		g := rep.Groups[c.group]
		g.Cases++
		rep.Cases++
		switch {
		case matched:
			g.Match++
			rep.Match++
		case c.Deviation != "":
			g.Deviations++
			rep.Deviations++
			rep.DeviationIDs = append(rep.DeviationIDs, c.ID)
		default:
			g.Mismatch++
			rep.Mismatch++
			rep.Mismatches = append(rep.Mismatches, c.ID)
		}
		rep.Groups[c.group] = g
	}

	denom := rep.Cases - rep.Deviations
	if denom > 0 {
		rep.ParityPercent = 100 * float64(rep.Match) / float64(denom)
	}
	if rep.Mismatches == nil {
		rep.Mismatches = []string{}
	}
	if rep.DeviationIDs == nil {
		rep.DeviationIDs = []string{}
	}

	if skipped > 0 {
		t.Logf("%d of %d cases were filtered out by -run; leaving parity.json untouched", skipped, len(cases))
		return
	}

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile("parity.json", b, 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("redis parity: %d cases, %d match, %d mismatch, %d deviations, %.2f%%",
		rep.Cases, rep.Match, rep.Mismatch, rep.Deviations, rep.ParityPercent)
}
