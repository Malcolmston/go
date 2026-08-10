// Package parity drives the upstream Node runner (@gltf-transform/core as the
// reference glTF reader/writer, gltf-validator as the Khronos conformance
// oracle) and the Go runner (github.com/malcolmston/gltf) over the same case
// files and compares their answers. Upstream is the oracle: nothing here
// hand-writes an expected value.
//
// Run with:
//
//	GOWORK=off go test ./parity/gltf/
//
// The suite skips (never fails) when Node or the pinned npm packages are
// unavailable, so a Go-only checkout stays green.
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
	// upstreamPin identifies both oracles; every case file must repeat it.
	upstreamPin = "@gltf-transform/core@4.2.1+gltf-validator@2.0.0-dev.3.10"
	coreVersion = "4.2.1"
	extVersion  = "4.2.1"
	valVersion  = "2.0.0-dev.3.10"

	caseTimeout = 60 * time.Second

	// Float comparison tolerance. Both sides funnel glTF data through float32
	// storage and (for node transforms) a float32 matrix compose, so values
	// agree to about seven significant digits, not bit-for-bit.
	absTol = 1e-6
	relTol = 1e-6
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
	// Note is diagnostic text (a rejection reason, for example). It is logged,
	// never compared: message wording is not part of parity.
	Note string `json:"note"`
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
		sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
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

// equalWithin is a deep-equal that treats two JSON numbers as equal when they
// agree to within absTol + relTol*max(|a|,|b|). It returns the JSON path of the
// first difference, or "" when the values agree.
func equalWithin(a, b any, path string) (string, bool) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return path, false
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		names := make([]string, 0, len(keys))
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			x, okx := av[k]
			y, oky := bv[k]
			if okx != oky {
				return path + "." + k, false
			}
			if p, ok := equalWithin(x, y, path+"."+k); !ok {
				return p, false
			}
		}
		return "", true
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return path, false
		}
		if len(av) != len(bv) {
			return fmt.Sprintf("%s (len %d vs %d)", path, len(av), len(bv)), false
		}
		for i := range av {
			if p, ok := equalWithin(av[i], bv[i], fmt.Sprintf("%s[%d]", path, i)); !ok {
				return p, false
			}
		}
		return "", true
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return path, false
		}
		if av == bv {
			return "", true
		}
		if math.IsNaN(av) && math.IsNaN(bv) {
			return "", true
		}
		scale := math.Max(math.Abs(av), math.Abs(bv))
		if math.Abs(av-bv) <= absTol+relTol*scale {
			return "", true
		}
		return path, false
	default:
		if a == b {
			return "", true
		}
		return path, false
	}
}

// show renders a value for test output, truncating very large structures.
func show(raw json.RawMessage) string {
	v, err := decode(raw)
	if err != nil {
		return fmt.Sprintf("<unparseable %s>", raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(out)
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	return s
}

// pluck returns the value at a dotted/indexed path inside a decoded JSON value,
// so a mismatch can be reported with just the offending subtree.
func pluck(v any, path string) any {
	cur := v
	for _, step := range splitPath(path) {
		switch c := cur.(type) {
		case map[string]any:
			cur = c[step]
		case []any:
			var i int
			if _, err := fmt.Sscanf(step, "%d", &i); err != nil || i < 0 || i >= len(c) {
				return cur
			}
			cur = c[i]
		default:
			return cur
		}
	}
	return cur
}

var pathStep = regexp.MustCompile(`[^.\[\]]+`)

func splitPath(path string) []string {
	steps := pathStep.FindAllString(path, -1)
	// Drop a trailing "(len N vs M)" annotation.
	for i, s := range steps {
		if strings.HasPrefix(s, "(len") {
			return steps[:i]
		}
	}
	return steps
}

// ---------------------------------------------------------------- the score

type groupScore struct {
	Cases      int `json:"cases"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type validatorReport struct {
	File       string   `json:"file"`
	NumErrors  int      `json:"numErrors"`
	NumWarns   int      `json:"numWarnings"`
	NumInfos   int      `json:"numInfos"`
	Errors     []string `json:"errors"`
	Warnings   []string `json:"warnings"`
	RunnerFail string   `json:"runnerError,omitempty"`
}

type score struct {
	Library           string                `json:"library"`
	Upstream          string                `json:"upstream"`
	UpstreamEcosystem string                `json:"upstreamEcosystem"`
	UpstreamPackages  map[string]string     `json:"upstreamPackages"`
	GoModule          string                `json:"goModule"`
	Tolerance         map[string]float64    `json:"floatTolerance"`
	Cases             int                   `json:"cases"`
	Match             int                   `json:"match"`
	Mismatch          int                   `json:"mismatch"`
	Deviations        int                   `json:"deviations"`
	ParityPercent     float64               `json:"parityPercent"`
	Groups            map[string]groupScore `json:"groups"`
	Mismatches        []string              `json:"mismatches"`
	ValidatorOnGo     []validatorReport     `json:"validatorOnGoOutput"`
}

var modLine = regexp.MustCompile(`github\.com/malcolmston/gltf\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/gltf " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "@gltf-transform", "core", "package.json")); err != nil {
		install := exec.Command("npm", "install", "--no-audit", "--no-fund")
		install.Dir = "node"
		if out, ierr := install.CombinedOutput(); ierr != nil {
			t.Skipf("upstream %s not installed and `npm install` failed: %v\n%s", upstreamPin, ierr, out)
		}
	}
	assertUpstreamVersions(t)
	requireFixtures(t)

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
	// Both runners write their GLBs into one shared scratch directory so each
	// can read what the other produced.
	out := t.TempDir()

	nodeCmd := exec.Command(node, "run.mjs")
	nodeCmd.Dir = "node"
	nodeCmd.Env = append(os.Environ(), "PARITY_FIXTURES="+fixtures, "PARITY_OUT="+out)
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off", "PARITY_FIXTURES="+fixtures, "PARITY_OUT="+out)
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:           "gltf",
		Upstream:          upstreamPin,
		UpstreamEcosystem: "node",
		UpstreamPackages: map[string]string{
			"@gltf-transform/core":       coreVersion,
			"@gltf-transform/extensions": extVersion,
			"gltf-validator":             valVersion,
		},
		GoModule:      goModuleVersion(),
		Tolerance:     map[string]float64{"abs": absTol, "rel": relTol},
		Cases:         len(cases),
		Groups:        map[string]groupScore{},
		Mismatches:    []string{},
		ValidatorOnGo: []validatorReport{},
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

	// A -run filter skips subtests, which would leave a misleading score on
	// disk, so only a complete pass may rewrite parity.json.
	if scored := compared + sc.Deviations; scored != len(cases) {
		t.Logf("partial run (%d of %d cases scored): leaving parity.json alone",
			scored, len(cases))
		return
	}

	sc.ValidatorOnGo = validateGoOutput(t, up, gor)
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
		for name, note := range map[string]string{"upstream": upRep.Note, "go": goRep.Note} {
			if strings.TrimSpace(note) != "" {
				t.Logf("%s note: %s", name, note)
			}
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
		if path, ok := equalWithin(upVal, goVal, ""); !ok {
			fail("value mismatch at %s\n  upstream: %s\n        go: %s",
				path, jsonish(pluck(upVal, path)), jsonish(pluck(goVal, path)))
			return
		}
		// A cross-write case additionally asserts, inside each runner, that the
		// GLB it wrote and the GLB the other implementation wrote describe the
		// same asset. Without this a systematic divergence present in both
		// writers would cancel out.
		for _, side := range []struct {
			name string
			val  any
		}{{"upstream", upVal}, {"go", goVal}} {
			m, isObj := side.val.(map[string]any)
			if !isObj {
				continue
			}
			if eq, present := m["selfEqualsCross"]; present && eq != true {
				fail("%s read its own GLB and the other implementation's GLB as different assets", side.name)
				return
			}
		}
		agreed = true
	})

	if c.Deviation != "" {
		return ran, ran
	}
	return ran, agreed
}

func jsonish(v any) string {
	enc, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(enc)
	if len(s) > 1200 {
		s = s[:1200] + "…"
	}
	return s
}

func describe(r reply) string {
	if r.OK {
		return "value " + show(r.Value)
	}
	return "error " + fmt.Sprintf("%q", r.Error)
}

// validateGoOutput is the conformance half of the harness: the Go port writes
// each fixture back out as .glb and .gltf, and the official Khronos validator
// grades the result. A port whose own output the validator rejects is a finding
// regardless of how well it matches @gltf-transform, so any error fails the
// test.
func validateGoOutput(t *testing.T, up, gor *runner) []validatorReport {
	t.Helper()

	fixtures := []string{"triangle.gltf", "triangle.glb", "scene.gltf", "animated.glb", "accessors.glb"}
	reports := []validatorReport{}

	t.Run("validator-on-go-output", func(t *testing.T) {
		for _, fx := range fixtures {
			for _, format := range []string{"glb", "gltf"} {
				id := fmt.Sprintf("emit-%s-%s", strings.ReplaceAll(fx, ".", "-"), format)
				emit := gor.call(caseSpec{
					ID:   id,
					Fn:   "emit",
					Args: []json.RawMessage{jsonArg(fx), jsonArg(format)},
				})
				if !emit.OK {
					t.Errorf("%s: Go runner could not emit %s as %s: %s", id, fx, format, emit.Error)
					reports = append(reports, validatorReport{File: fx + "." + format, RunnerFail: emit.Error})
					continue
				}
				var emitted struct{ File string }
				if err := json.Unmarshal(emit.Value, &emitted); err != nil {
					t.Errorf("%s: unparseable emit reply: %v", id, err)
					continue
				}

				vr := up.call(caseSpec{
					ID:   "validate-" + emitted.File,
					Fn:   "validateOut",
					Args: []json.RawMessage{jsonArg(emitted.File)},
				})
				if !vr.OK {
					t.Errorf("%s: validator failed on port-produced %s: %s", id, emitted.File, vr.Error)
					reports = append(reports, validatorReport{File: emitted.File, RunnerFail: vr.Error})
					continue
				}
				var issues struct {
					NumErrors   int `json:"numErrors"`
					NumWarnings int `json:"numWarnings"`
					NumInfos    int `json:"numInfos"`
					Messages    []struct {
						Severity int    `json:"severity"`
						Code     string `json:"code"`
						Pointer  string `json:"pointer"`
						Message  string `json:"message"`
					} `json:"messages"`
				}
				if err := json.Unmarshal(vr.Value, &issues); err != nil {
					t.Errorf("%s: unparseable validator reply: %v", id, err)
					continue
				}
				rep := validatorReport{
					File:      emitted.File,
					NumErrors: issues.NumErrors,
					NumWarns:  issues.NumWarnings,
					NumInfos:  issues.NumInfos,
					Errors:    []string{},
					Warnings:  []string{},
				}
				for _, m := range issues.Messages {
					line := strings.TrimSpace(fmt.Sprintf("%s %s: %s", m.Code, m.Pointer, m.Message))
					switch m.Severity {
					case 0:
						rep.Errors = append(rep.Errors, line)
					case 1:
						rep.Warnings = append(rep.Warnings, line)
					}
				}
				reports = append(reports, rep)
				if rep.NumErrors > 0 {
					t.Errorf("gltf-validator rejects port-produced %s: %d error(s)\n  %s",
						emitted.File, rep.NumErrors, strings.Join(rep.Errors, "\n  "))
				}
				for _, w := range rep.Warnings {
					t.Logf("gltf-validator warning on %s: %s", emitted.File, w)
				}
			}
		}
	})
	return reports
}

func jsonArg(v string) json.RawMessage {
	enc, _ := json.Marshal(v)
	return enc
}

func writeScore(t *testing.T, sc score) {
	t.Helper()
	sort.Slice(sc.ValidatorOnGo, func(i, j int) bool { return sc.ValidatorOnGo[i].File < sc.ValidatorOnGo[j].File })
	enc, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(enc, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}

// requireFixtures checks the committed fixtures are present; they are
// regenerated by `node fixtures/gen.mjs`.
func requireFixtures(t *testing.T) {
	t.Helper()
	for _, f := range []string{"triangle.gltf", "triangle.glb", "scene.gltf", "animated.glb", "accessors.glb"} {
		if _, err := os.Stat(filepath.Join("fixtures", f)); err != nil {
			t.Fatalf("missing fixture %s (regenerate with `node fixtures/gen.mjs`): %v", f, err)
		}
	}
}

// assertUpstreamVersions checks every installed oracle matches the pin, so the
// score can never be attributed to the wrong upstream.
func assertUpstreamVersions(t *testing.T) {
	t.Helper()
	for pkg, want := range map[string]string{
		"@gltf-transform/core":       coreVersion,
		"@gltf-transform/extensions": extVersion,
		"gltf-validator":             valVersion,
	} {
		raw, err := os.ReadFile(filepath.Join("node", "node_modules", filepath.FromSlash(pkg), "package.json"))
		if err != nil {
			t.Skipf("cannot read installed %s: %v", pkg, err)
		}
		var meta struct{ Version string }
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatalf("parse installed %s package.json: %v", pkg, err)
		}
		if meta.Version != want {
			t.Fatalf("installed %s is %s, cases pin %s", pkg, meta.Version, want)
		}
	}
}
