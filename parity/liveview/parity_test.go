// Package parity drives the upstream (Phoenix LiveView) and Go (github.com/malcolmston/liveview)
// runners over the same cases and compares their answers.
//
// Both runners are subprocesses speaking JSON Lines on stdio; each is started
// once and every case is streamed through it. A missing Elixir toolchain, or a
// hex.pm that cannot be reached, skips the suite rather than failing it.
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	caseTimeout  = 20 * time.Second
	buildTimeout = 10 * time.Minute
)

// extraPATH holds directories that are commonly on a developer's interactive
// PATH but not on a test runner's. Homebrew installs elixir/mix here.
var extraPATH = []string{"/opt/homebrew/bin", "/usr/local/bin"}

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

	group    string
	upstream string
}

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// ---------------------------------------------------------------- runner ----

// runner is a live subprocess speaking JSON Lines. Replies are read on a
// goroutine so that a single hung case fails only that case.
type runner struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan reply
	readErr chan error
	dead    bool
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
		t.Fatalf("%s: start %s: %v", name, cmd.Path, err)
	}

	r := &runner{
		name:    name,
		cmd:     cmd,
		stdin:   stdin,
		replies: make(chan reply, 8),
		readErr: make(chan error, 1),
	}

	go func() {
		defer close(r.replies)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rep reply
			if err := json.Unmarshal([]byte(line), &rep); err != nil {
				r.readErr <- fmt.Errorf("%s: undecodable reply %q: %w", name, line, err)
				return
			}
			r.replies <- rep
		}
		if err := sc.Err(); err != nil {
			r.readErr <- fmt.Errorf("%s: read: %w", name, err)
		}
	}()

	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})

	return r
}

// call sends one case and waits for its reply, enforcing a per-case timeout.
func (r *runner) call(c testCase) (reply, error) {
	if r.dead {
		return reply{}, fmt.Errorf("%s: runner is no longer usable", r.name)
	}

	req := struct {
		ID   string            `json:"id"`
		Fn   string            `json:"fn"`
		Args []json.RawMessage `json:"args"`
	}{ID: c.ID, Fn: c.Fn, Args: c.Args}

	line, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.stdin.Write(append(line, '\n')); err != nil {
		r.dead = true
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}

	select {
	case rep, ok := <-r.replies:
		if !ok {
			r.dead = true
			select {
			case err := <-r.readErr:
				return reply{}, err
			default:
				return reply{}, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
			}
		}
		if rep.ID != c.ID {
			r.dead = true
			return reply{}, fmt.Errorf("%s: out of order reply: want %s, got %s", r.name, c.ID, rep.ID)
		}
		return rep, nil
	case err := <-r.readErr:
		r.dead = true
		return reply{}, err
	case <-time.After(caseTimeout):
		// The stream position is now unknown, so no further case can be trusted.
		r.dead = true
		return reply{}, fmt.Errorf("%s: timed out after %s on %s", r.name, caseTimeout, c.ID)
	}
}

// ------------------------------------------------------------------ test ----

func TestParity(t *testing.T) {
	root := repoDir(t)

	mixPath := lookPath("mix")
	if mixPath == "" {
		t.Skip("elixir/mix not found on PATH: cannot run the upstream liveview runner")
	}
	if lookPath("elixir") == "" {
		t.Skip("elixir not found on PATH: cannot run the upstream liveview runner")
	}

	cases := loadCases(t, filepath.Join(root, "cases"))
	if len(cases) == 0 {
		t.Fatal("no cases found")
	}

	upstreamBin := buildUpstream(t, root, mixPath)
	goBin := buildGo(t, root)

	up := startRunner(t, "elixir", exec.Command(upstreamBin))
	gp := startRunner(t, "go", exec.Command(goBin))

	var (
		total, matched, mismatched, deviations int
		divergences                            []string
	)

	for _, c := range cases {
		c := c
		total++

		upRep, upErr := up.call(c)
		goRep, goErr := gp.call(c)

		ok := t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				t.Fatalf("upstream runner failed: %v", upErr)
			}
			if goErr != nil {
				t.Fatalf("go runner failed: %v", goErr)
			}
			if upRep.OK != goRep.OK {
				t.Errorf("ok mismatch: upstream ok=%v (%s) value=%s / go ok=%v (%s) value=%s",
					upRep.OK, upRep.Error, upRep.Value, goRep.OK, goRep.Error, goRep.Value)
				return
			}
			if !upRep.OK {
				// Both failed: parity is about *whether* it failed, not about
				// the message text.
				return
			}
			upVal, err := decode(upRep.Value)
			if err != nil {
				t.Fatalf("upstream value undecodable: %v", err)
			}
			goVal, err := decode(goRep.Value)
			if err != nil {
				t.Fatalf("go value undecodable: %v", err)
			}
			if !deepEqual(upVal, goVal) {
				t.Errorf("value mismatch:\n  upstream: %s\n        go: %s", upRep.Value, goRep.Value)
			}
		})

		switch {
		case ok:
			matched++
		case c.Deviation != "":
			deviations++
		default:
			mismatched++
			divergences = append(divergences, fmt.Sprintf("%s/%s: upstream ok=%v value=%s err=%q | go ok=%v value=%s err=%q",
				c.group, c.ID, upRep.OK, upRep.Value, upRep.Error, goRep.OK, goRep.Value, goRep.Error))
		}
	}

	writeParityJSON(t, root, cases, total, matched, mismatched, deviations)

	if len(divergences) > 0 {
		t.Logf("%d divergence(s):\n%s", len(divergences), strings.Join(divergences, "\n"))
	}
}

// ----------------------------------------------------------------- build ----

func buildUpstream(t *testing.T, root, mixPath string) string {
	t.Helper()
	dir := filepath.Join(root, "elixir")

	env := append(os.Environ(), "MIX_ENV=prod", "PATH="+augmentedPATH())

	deps := exec.Command(mixPath, "deps.get")
	deps.Dir = dir
	deps.Env = env
	if out, err := combinedWithTimeout(deps, buildTimeout); err != nil {
		if unreachableHex(out) {
			t.Skipf("mix deps.get could not reach hex.pm: %v\n%s", err, out)
		}
		t.Fatalf("mix deps.get: %v\n%s", err, out)
	}

	build := exec.Command(mixPath, "escript.build")
	build.Dir = dir
	build.Env = env
	if out, err := combinedWithTimeout(build, buildTimeout); err != nil {
		t.Fatalf("mix escript.build: %v\n%s", err, out)
	}

	bin := filepath.Join(dir, "liveview_parity")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("escript not produced at %s: %v", bin, err)
	}
	return bin
}

func buildGo(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-liveview-runner")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	goTool := "go"
	if _, err := exec.LookPath(goTool); err != nil && runtime.GOROOT() != "" {
		goTool = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	build := exec.Command(goTool, "build", "-o", bin, "./go")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := combinedWithTimeout(build, buildTimeout); err != nil {
		t.Fatalf("go build ./go: %v\n%s", err, out)
	}
	return bin
}

func combinedWithTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	done := make(chan struct{})
	var (
		out []byte
		err error
	)
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(out), err
	case <-time.After(d):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return string(out), fmt.Errorf("timed out after %s", d)
	}
}

// unreachableHex reports whether mix failed because it could not reach the
// package registry, which must skip rather than fail the suite.
func unreachableHex(out string) bool {
	needles := []string{
		"could not fetch",
		"failed to fetch record",
		"no such host",
		"nxdomain",
		"connection refused",
		"econnrefused",
		"etimedout",
		"timed out",
		"network is unreachable",
		"certificate verify failed",
	}
	low := strings.ToLower(out)
	for _, n := range needles {
		if strings.Contains(low, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------- cases ----

func loadCases(t *testing.T, dir string) []testCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)

	var (
		all  []testCase
		seen = map[string]string{}
	)
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
			t.Errorf("%s: missing pinned upstream version", p)
		}
		for _, c := range cf.Cases {
			if c.ID == "" || c.Fn == "" {
				t.Errorf("%s: case with empty id or fn", p)
				continue
			}
			if prev, dup := seen[c.ID]; dup {
				t.Errorf("duplicate case id %q in %s and %s", c.ID, prev, p)
				continue
			}
			seen[c.ID] = p
			c.group = cf.Group
			c.upstream = cf.Upstream
			all = append(all, c)
		}
	}
	return all
}

// ------------------------------------------------------------- comparing ----

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

// deepEqual compares two decoded JSON values, treating all numbers as float64
// so that 1 and 1.0 are equal.
func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			other, ok := bv[k]
			if !ok || !deepEqual(v, other) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return false
		}
		if av == bv {
			return true
		}
		// Tolerate the last bit of float noise, but nothing more.
		return math.Abs(av-bv) <= 1e-9*math.Max(math.Abs(av), math.Abs(bv))
	case nil:
		return b == nil
	default:
		return a == b
	}
}

// ------------------------------------------------------------- reporting ----

func writeParityJSON(t *testing.T, root string, cases []testCase, total, matched, mismatched, deviations int) {
	t.Helper()

	groups := map[string]int{}
	upstreamSymbols := map[string]bool{}
	for _, c := range cases {
		groups[c.group]++
		if c.UpstreamFn != "" && c.UpstreamFn != "—" {
			upstreamSymbols[c.UpstreamFn] = true
		}
	}
	groupNames := make([]string, 0, len(groups))
	for g := range groups {
		groupNames = append(groupNames, g)
	}
	sort.Strings(groupNames)

	byGroup := make([]map[string]any, 0, len(groupNames))
	for _, g := range groupNames {
		byGroup = append(byGroup, map[string]any{"group": g, "cases": groups[g]})
	}

	pct := 0.0
	if total > 0 {
		pct = float64(matched) / float64(total) * 100
	}

	report := map[string]any{
		"repo":                         "liveview",
		"upstream":                     "phoenixframework/phoenix_live_view",
		"upstreamVersion":              upstreamVersion(cases),
		"goModule":                     goModuleVersion(t, root),
		"cases":                        total,
		"matched":                      matched,
		"mismatched":                   mismatched,
		"deviations":                   deviations,
		"casePassRate":                 fmt.Sprintf("%.1f%%", pct),
		"upstreamSymbolsCased":         len(upstreamSymbols),
		"byGroup":                      byGroup,
		"notComparableWithoutEndpoint": "the transport, channel, lifecycle, testing, navigation, upload, form, JS-command and client-side halves of LiveView all need a running Phoenix endpoint; see COVERAGE.md",
		"comparableSurface":            "the rendered diff format: static/dynamic split, diff minimality, comprehension encoding, HTML escaping, nested component encoding under \"c\"",
	}

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal parity.json: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(root, "parity.json"), raw, 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}

// upstreamVersion returns the pinned version recorded in the case files, or a
// comma-joined list if they somehow disagree.
func upstreamVersion(cases []testCase) string {
	seen := map[string]bool{}
	var versions []string
	for _, c := range cases {
		if c.upstream != "" && !seen[c.upstream] {
			seen[c.upstream] = true
			versions = append(versions, c.upstream)
		}
	}
	sort.Strings(versions)
	return strings.Join(versions, ",")
}

func goModuleVersion(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require github.com/malcolmston/liveview ") ||
			strings.HasPrefix(line, "github.com/malcolmston/liveview ") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "github.com/malcolmston/liveview" && i+1 < len(fields) {
					return "github.com/malcolmston/liveview@" + fields[i+1]
				}
			}
		}
	}
	return "unknown"
}

// ----------------------------------------------------------------- paths ----

func repoDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return dir
}

// lookPath finds a binary on PATH, falling back to the well-known Homebrew and
// /usr/local locations that a bare test environment may omit.
func lookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, dir := range extraPATH {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func augmentedPATH() string {
	parts := []string{}
	seen := map[string]bool{}
	for _, dir := range append(extraPATH, filepath.SplitList(os.Getenv("PATH"))...) {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		parts = append(parts, dir)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
