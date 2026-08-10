// Package parity drives the upstream (Node/sharp@0.33.5) and Go
// (github.com/malcolmston/sharp) runners over the same case files and compares
// their answers. Upstream is the oracle: nothing here hand-writes an expected
// value.
//
// Run with:
//
//	GOWORK=off go test ./parity/sharp/
//
// The suite skips (never fails) when Node or the pinned upstream install is
// unavailable -- sharp ships prebuilt libvips binaries, but if npm cannot fetch
// one for this platform there is no oracle and a Go-only checkout must stay
// green.
//
// # What is compared, and what deliberately is not
//
// Two independent resamplers never produce byte-identical pixels, so encoded
// bytes and exact pixel values are NOT compared. Four things are:
//
//  1. Geometry -- output width/height/channels/hasAlpha for every
//     resize/fit/crop/extend/rotate/extract combination. Exact, deterministic,
//     and where ports actually diverge.
//  2. Metadata -- format/width/height/channels/hasAlpha/space for the same
//     input file. Exact.
//  3. Coarse pixel statistics with a tolerance -- per-channel mean/min/max and
//     the mean luma of a 4x4 downsample grid. See the tolerance note below.
//  4. Format acceptance -- which inputs each side decodes and which outputs
//     each side encodes, as ok/error outcomes.
//
// Tolerance: a case carries `tol` (the absolute tolerance applied to every
// numeric leaf) and `tolByKey` (per-top-level-key overrides). The pixel cases
// use tol=2.0 with min/max at 8.0 and width/height at 0. Justification, and the
// measurements behind those numbers, are in COVERAGE.md.
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
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	upstreamPin = "sharp@0.33.5"
	goModulePkg = "github.com/malcolmston/sharp"
	caseTimeout = 30 * time.Second
)

// ---------------------------------------------------------------- case files

type caseSpec struct {
	ID         string             `json:"id"`
	Fn         string             `json:"fn"`
	Args       []json.RawMessage  `json:"args"`
	UpstreamFn string             `json:"upstreamFn,omitempty"`
	GoFn       string             `json:"goFn,omitempty"`
	Note       string             `json:"note,omitempty"`
	Deviation  string             `json:"deviation,omitempty"`
	Tol        float64            `json:"tol,omitempty"`
	TolByKey   map[string]float64 `json:"tolByKey,omitempty"`

	group string
}

// tolFor resolves the absolute tolerance for a numeric leaf reached under the
// given top-level key.
func (c caseSpec) tolFor(key string) float64 {
	if t, ok := c.TolByKey[key]; ok {
		return t
	}
	return c.Tol
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
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}

	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 64)}

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
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

// equalWithin is a normalising deep-equal in which numeric leaves may differ by
// up to the tolerance the case assigns to the top-level key they sit under.
// Everything else (strings, bools, null, shape) must match exactly.
func equalWithin(up, got any, key string, c caseSpec) (bool, string) {
	switch u := up.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false, fmt.Sprintf("%s: upstream object, go %T", pathOf(key), got)
		}
		if len(u) != len(g) {
			return false, fmt.Sprintf("%s: key count %d vs %d (%v vs %v)",
				pathOf(key), len(u), len(g), sortedKeys(u), sortedKeys(g))
		}
		for k, uv := range u {
			gv, present := g[k]
			if !present {
				return false, fmt.Sprintf("%s: go is missing key %q", pathOf(key), k)
			}
			// The key that owns a tolerance is the one named at object level.
			if ok, why := equalWithin(uv, gv, k, c); !ok {
				return false, why
			}
		}
		return true, ""

	case []any:
		g, ok := got.([]any)
		if !ok {
			return false, fmt.Sprintf("%s: upstream array, go %T", pathOf(key), got)
		}
		if len(u) != len(g) {
			return false, fmt.Sprintf("%s: length %d vs %d", pathOf(key), len(u), len(g))
		}
		for i := range u {
			if ok, why := equalWithin(u[i], g[i], key, c); !ok {
				return false, fmt.Sprintf("%s[%d] -> %s", pathOf(key), i, why)
			}
		}
		return true, ""

	case float64:
		g, ok := got.(float64)
		if !ok {
			return false, fmt.Sprintf("%s: upstream number %v, go %T", pathOf(key), u, got)
		}
		tol := c.tolFor(key)
		if d := math.Abs(u - g); d > tol {
			return false, fmt.Sprintf("%s: %g vs %g (|d|=%g > tol %g)", pathOf(key), u, g, d, tol)
		}
		return true, ""

	default:
		if up != got {
			return false, fmt.Sprintf("%s: %#v vs %#v", pathOf(key), up, got)
		}
		return true, ""
	}
}

func pathOf(key string) string {
	if key == "" {
		return "<root>"
	}
	return key
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
	Compared      string                `json:"compared"`
	NotCompared   string                `json:"notCompared"`
	Tolerance     string                `json:"tolerance"`
	Cases         int                   `json:"cases"`
	Match         int                   `json:"match"`
	Mismatch      int                   `json:"mismatch"`
	Deviations    int                   `json:"deviations"`
	ParityPercent float64               `json:"parityPercent"`
	Groups        map[string]groupScore `json:"groups"`
	Mismatches    []string              `json:"mismatches"`
}

var modLine = regexp.MustCompile(regexp.QuoteMeta(goModulePkg) + `\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return goModulePkg + " " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}

	if _, err := os.Stat(filepath.Join("node", "node_modules", "sharp", "package.json")); err != nil {
		install := exec.Command("npm", "install", "--no-audit", "--no-fund")
		install.Dir = "node"
		if out, ierr := install.CombinedOutput(); ierr != nil {
			t.Skipf("upstream %s not installed and `npm install` failed "+
				"(no prebuilt libvips for this platform?): %v\n%s", upstreamPin, ierr, out)
		}
	}
	assertUpstreamVersion(t, node)
	// A load-bearing smoke test: the package can be present but its prebuilt
	// binary unusable on this platform, which is a skip and not a failure.
	probe := exec.Command(node, "-e", "require('sharp')(Buffer.alloc(0));")
	probe.Dir = "node"
	if out, err := probe.CombinedOutput(); err != nil && strings.Contains(string(out), "Could not load") {
		t.Skipf("sharp's prebuilt libvips binary is not loadable here: %s", out)
	}

	ensureFixtures(t, node)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	fixtures, err := filepath.Abs("fixtures")
	if err != nil {
		t.Fatalf("resolve fixtures dir: %v", err)
	}

	nodeCmd := exec.Command(node, "run.mjs")
	nodeCmd.Dir = "node"
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off", "PARITY_FIXTURES="+fixtures)
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:  "sharp",
		Upstream: upstreamPin,
		GoModule: goModuleVersion(),
		Compared: "output geometry (width/height/channels/hasAlpha); reported metadata; " +
			"coarse pixel statistics (per-channel mean/min/max plus 4x4 grid luma) within a tolerance; " +
			"input-decode and output-encode acceptance",
		NotCompared: "encoded image bytes and exact pixel values: two independent resamplers " +
			"cannot agree bit-for-bit, so comparing them would be pure noise",
		Tolerance: "pixel cases: |delta| <= 2.0/255 for means and grid luma, <= 8.0/255 for " +
			"single-pixel min/max, 0 for dimensions; every other case is exact",
		Cases:      len(cases),
		Groups:     map[string]groupScore{},
		Mismatches: []string{},
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
		}
		sc.Groups[c.group] = g
	}

	compared := sc.Match + sc.Mismatch
	if compared > 0 {
		sc.ParityPercent = float64(sc.Match) * 100 / float64(compared)
	}

	t.Logf("parity: %d/%d cases match (%.2f%%), %d deviations",
		sc.Match, compared, sc.ParityPercent, sc.Deviations)
	for _, name := range sortedGroupNames(sc.Groups) {
		g := sc.Groups[name]
		t.Logf("  %-22s %3d/%3d match", name, g.Match, g.Cases)
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

func sortedGroupNames(m map[string]groupScore) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
			fail("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
				upRep.OK, describe(upRep), goRep.OK, describe(goRep))
			return
		}
		if !upRep.OK {
			// Both failed: that is parity. Message text is not compared;
			// differences are recorded in COVERAGE.md.
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
		if ok, why := equalWithin(upVal, goVal, "", c); !ok {
			fail("value mismatch at %s\n  upstream: %s\n        go: %s", why, show(upRep.Value), show(goRep.Value))
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
	sort.Strings(sc.Mismatches)
	enc, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(enc, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}

// ensureFixtures generates the shared input files once. Both runners read the
// exact same bytes off disk, so no case can be lost to the two sides having
// synthesised subtly different inputs.
func ensureFixtures(t *testing.T, node string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join("fixtures", "gradient.avif")); err == nil {
		return
	}
	gen := exec.Command(node, "gen-fixtures.mjs")
	gen.Dir = "node"
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate fixtures: %v\n%s", err, out)
	}
}

// assertUpstreamVersion checks the installed upstream matches the pin, so the
// score can never be attributed to the wrong oracle.
func assertUpstreamVersion(t *testing.T, node string) {
	t.Helper()
	want := strings.SplitN(upstreamPin, "@", 2)[1]

	raw, err := os.ReadFile(filepath.Join("node", "node_modules", "sharp", "package.json"))
	if err != nil {
		t.Skipf("cannot read installed sharp: %v", err)
	}
	var pkg struct{ Version string }
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("parse installed sharp package.json: %v", err)
	}
	if pkg.Version != want {
		t.Fatalf("installed sharp is %s, cases pin %s", pkg.Version, want)
	}
}
