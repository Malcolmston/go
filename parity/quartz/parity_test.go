// Package quartzparity is the cross-language parity harness for the Go port of
// Quartz. It drives two subprocess runners -- the upstream Java oracle in java/
// and the Go port in go/ -- over the identical case files in cases/, compares
// their answers, and rewrites parity.json with the totals.
//
// Run it with GOWORK=off so the published module version in go.mod is what gets
// measured rather than a local checkout:
//
//	GOWORK=off go test ./parity/quartz/
package quartzparity

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
	"sync"
	"testing"
	"time"
)

// caseTimeout bounds a single request/response exchange so one hung case fails
// that case rather than wedging the suite.
const caseTimeout = 15 * time.Second

// buildTimeout bounds the one-off runner builds (Maven may need to download the
// pinned upstream artifacts on a cold cache).
const buildTimeout = 10 * time.Minute

type caseFile struct {
	Group    string       `json:"group"`
	Upstream string       `json:"upstream"`
	Note     string       `json:"note"`
	Cases    []parityCase `json:"cases"`
}

type parityCase struct {
	ID         string `json:"id"`
	Fn         string `json:"fn"`
	Args       []any  `json:"args"`
	UpstreamFn string `json:"upstreamFn"`
	GoFn       string `json:"goFn"`
	Note       string `json:"note"`
	Deviation  string `json:"deviation"`

	group string
}

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error"`
}

// runner is one long-lived subprocess speaking JSON Lines.
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
	r := &runner{name: name, cmd: cmd, stdin: stdin,
		lines: make(chan string, 64), errs: make(chan error, 1)}
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
	t.Cleanup(r.close)
	return r
}

func (r *runner) close() {
	r.once.Do(func() {
		_ = r.stdin.Close()
		done := make(chan struct{})
		go func() { _ = r.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = r.cmd.Process.Kill()
		}
	})
}

// kill tears the process down immediately, used when a case has hung it.
func (r *runner) kill() {
	r.once.Do(func() {
		if r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
		_ = r.stdin.Close()
		_ = r.cmd.Wait()
	})
}

// restarter keeps one runner alive across the whole suite, but replaces it when
// a case hangs or crashes it. Without this, a single non-terminating computation
// on one side would desynchronise -- and therefore lose -- every case after it.
type restarter struct {
	t      *testing.T
	name   string
	newCmd func() *exec.Cmd
	cur    *runner
	starts int
}

func (h *restarter) ask(c parityCase) (reply, error) {
	if h.cur == nil {
		h.cur = startRunner(h.t, h.name, h.newCmd())
		h.starts++
	}
	rep, err := h.cur.ask(c)
	if err != nil {
		h.cur.kill()
		h.cur = nil
	}
	return rep, err
}

// ask sends one case and waits for its reply, enforcing the per-case timeout.
func (r *runner) ask(c parityCase) (reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	enc, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.stdin.Write(append(enc, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return reply{}, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{}, fmt.Errorf("%s: undecodable reply for %s: %v (%q)", r.name, c.ID, err, line)
		}
		if rep.ID != c.ID {
			return reply{}, fmt.Errorf("%s: reply out of order: want %s, got %s", r.name, c.ID, rep.ID)
		}
		return rep, nil
	case err := <-r.errs:
		return reply{}, fmt.Errorf("%s: read: %w", r.name, err)
	case <-time.After(caseTimeout):
		return reply{}, fmt.Errorf("%s: timed out after %s on %s", r.name, caseTimeout, c.ID)
	}
}

// ---------------------------------------------------------------- comparison

// equal is a normalising deep-equal: all JSON numbers are float64, so 1 and 1.0
// compare equal, and object key order is irrelevant.
func equal(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case float64:
		bv, ok := toFloat(b)
		return ok && (av == bv || (math.IsNaN(av) && math.IsNaN(bv)))
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equal(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		keys := make([]string, 0, len(av))
		for k := range av {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			other, present := bv[k]
			if !present || !equal(av[k], other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// ------------------------------------------------------------------- loading

func loadCases(t *testing.T, dir string) ([]parityCase, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no case files in %s", dir)
	}
	sort.Strings(paths)

	var all []parityCase
	upstream := ""
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
			t.Fatalf("%s: missing pinned upstream version", p)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if upstream != cf.Upstream {
			t.Fatalf("%s pins %q but another file pins %q; the oracle must be one exact version",
				p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q in %s (also in %s)", c.ID, filepath.Base(p), prev)
			}
			seen[c.ID] = filepath.Base(p)
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ------------------------------------------------------------------ building

// requireToolchain skips (never fails) when the upstream toolchain is absent, so
// a Go-only machine does not see a red build.
func requireToolchain(t *testing.T) (java string, mvn string) {
	t.Helper()
	var err error
	if java, err = exec.LookPath("java"); err != nil {
		t.Skip("java not found in PATH; skipping quartz parity")
	}
	if mvn, err = exec.LookPath("mvn"); err != nil {
		t.Skip("mvn not found in PATH; skipping quartz parity")
	}
	return java, mvn
}

func buildJava(t *testing.T, mvn, dir string) string {
	t.Helper()
	jar := filepath.Join(dir, "target", "runner.jar")
	deps := filepath.Join(dir, "target", "deps")
	cmd := exec.Command(mvn, "-B", "-q", "package")
	cmd.Dir = dir
	out, err := combinedWithTimeout(cmd, buildTimeout)
	if err != nil {
		// A cold Maven cache with no network cannot fetch the pinned oracle.
		// That is an absent toolchain, not a parity failure.
		t.Skipf("mvn package failed (no network or unresolvable dependencies?); skipping quartz parity\n%s\n%v",
			out, err)
	}
	if _, err := os.Stat(jar); err != nil {
		t.Skipf("mvn package produced no %s; skipping quartz parity", jar)
	}
	if _, err := os.Stat(deps); err != nil {
		t.Skipf("mvn package produced no %s; skipping quartz parity", deps)
	}
	return jar + string(os.PathListSeparator) + filepath.Join(deps, "*")
}

func buildGo(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-runner")
	cmd := exec.Command("go", "build", "-o", bin, "./go/")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := combinedWithTimeout(cmd, buildTimeout)
	if err != nil {
		t.Fatalf("go build ./go/: %v\n%s", err, out)
	}
	return bin
}

func combinedWithTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		ch <- res{out, err}
	}()
	select {
	case r := <-ch:
		return string(r.out), r.err
	case <-time.After(d):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("timed out after %s", d)
	}
}

// --------------------------------------------------------------------- score

type groupScore struct {
	Cases     int `json:"cases"`
	Match     int `json:"match"`
	Differ    int `json:"differs"`
	Deviation int `json:"deviations"`
}

type report struct {
	Repo           string                `json:"repo"`
	Upstream       string                `json:"upstream"`
	UpstreamPkg    string                `json:"upstreamPackage"`
	GoModule       string                `json:"goModule"`
	Ecosystem      string                `json:"ecosystem"`
	Cases          int                   `json:"cases"`
	Match          int                   `json:"match"`
	Differ         int                   `json:"differs"`
	Deviations     int                   `json:"deviations"`
	Parity         string                `json:"parity"`
	RunnerRestarts map[string]int        `json:"runnerRestarts"`
	Groups         map[string]groupScore `json:"groups"`
	Diverged       []string              `json:"divergedCases"`
}

func goModuleVersion(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "github.com/malcolmston/quartz" {
				return f[i] + "@" + f[i+1]
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------- test

func TestParity(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	cases, upstream := loadCases(t, filepath.Join(dir, "cases"))
	java, mvn := requireToolchain(t)

	classpath := buildJava(t, mvn, filepath.Join(dir, "java"))
	goBin := buildGo(t, dir)

	up := &restarter{t: t, name: "java",
		newCmd: func() *exec.Cmd { return exec.Command(java, "-cp", classpath, "parity.Runner") }}
	gp := &restarter{t: t, name: "go",
		newCmd: func() *exec.Cmd { return exec.Command(goBin) }}

	scores := map[string]groupScore{}
	var diverged []string
	total, match, differ, deviations := 0, 0, 0, 0

	for _, c := range cases {
		c := c
		upRep, upErr := up.ask(c)
		goRep, goErr := gp.ask(c)

		total++
		gs := scores[c.group]
		gs.Cases++

		ok := t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				t.Fatalf("upstream runner: %v", upErr)
			}
			if goErr != nil {
				t.Fatalf("go runner: %v", goErr)
			}
			if upRep.OK != goRep.OK {
				t.Errorf("ok mismatch\n  upstream: ok=%v value=%s error=%s\n  go:       ok=%v value=%s error=%s",
					upRep.OK, show(upRep.Value), upRep.Error, goRep.OK, show(goRep.Value), goRep.Error)
				return
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is not compared;
				// wording differences belong in COVERAGE.md.
				return
			}
			if !equal(upRep.Value, goRep.Value) {
				t.Errorf("value mismatch\n  upstream: %s\n  go:       %s", show(upRep.Value), show(goRep.Value))
			}
		})

		switch {
		case c.Deviation != "":
			deviations++
			gs.Deviation++
		case ok:
			match++
			gs.Match++
		default:
			differ++
			gs.Differ++
			diverged = append(diverged, c.ID)
		}
		scores[c.group] = gs
	}

	compared := match + differ
	pct := 0.0
	if compared > 0 {
		pct = 100 * float64(match) / float64(compared)
	}
	sort.Strings(diverged)

	rep := report{
		Repo:           "quartz",
		Upstream:       "quartz-scheduler/quartz",
		UpstreamPkg:    upstream,
		GoModule:       goModuleVersion(t, dir),
		Ecosystem:      "java",
		Cases:          total,
		Match:          match,
		Differ:         differ,
		Deviations:     deviations,
		Parity:         fmt.Sprintf("%.1f%%", pct),
		RunnerRestarts: map[string]int{"java": up.starts - 1, "go": gp.starts - 1},
		Groups:         scores,
		Diverged:       diverged,
	}
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parity.json"), append(out, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("quartz parity: %d/%d cases match (%s) across %d groups", match, compared, rep.Parity, len(scores))
}
