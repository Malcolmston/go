// Package parity drives the upstream (Node/commonmark@0.31.2) and Go
// (github.com/malcolmston/markdown) runners over the same case files and
// compares their answers.
//
// Run with:
//
//	GOWORK=off go test ./parity/markdown/
//
// Unusually for this harness, the oracle here is better than a library: the
// CommonMark project publishes `spec.json`, a machine-readable conformance
// suite of 652 (markdown, html, section) triples, vendored at
// cases/spec.json. So every case generated from it is compared THREE ways:
//
//	go   vs spec      -> conformance (the headline number)
//	node vs spec      -> a cross-check that this harness is wired up correctly;
//	                     a failure here indicts the harness, not the port
//	go   vs node      -> parity, the number the rest of this harness reports
//
// The hand-written groups (security, options, api) have no spec answer and are
// compared go-vs-node only.
//
// The suite skips (never fails) when Node, the pinned upstream install, or
// cases/spec.json is unavailable, so a Go-only checkout stays green.
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
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	upstreamPin = "commonmark@0.31.2"
	specVersion = "0.31.2"
	specExample = 652 // examples in CommonMark 0.31.2 spec.json
	caseTimeout = 20 * time.Second
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

	// SpecHTML is the specification's own expected output. Present only on
	// cases generated from spec.json, and never hand-written.
	SpecHTML  *string `json:"specHTML,omitempty"`
	SpecLines string  `json:"specLines,omitempty"`

	group string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Cases    []caseSpec `json:"cases"`
}

// loadCases reads every case file except cases/spec.json, which is the raw
// vendored corpus rather than a case file.
func loadCases(t *testing.T, dir string) []caseSpec {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(paths)

	var all []caseSpec
	seen := map[string]string{}
	for _, path := range paths {
		if filepath.Base(path) == "spec.json" {
			continue
		}
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
	if len(all) == 0 {
		t.Fatalf("no case files in %s", dir)
	}
	return all
}

// ------------------------------------------------------------- normalisation

// normaliseHTML is the ONLY massaging applied to rendered HTML, and it is
// applied identically to all three answers (spec, upstream, Go):
//
//  1. CRLF and lone CR become LF, so a checkout with CRLF line endings in
//     cases/spec.json cannot invent differences.
//  2. A single trailing LF is removed.
//
// Nothing else. In particular tags, attribute order, indentation and interior
// whitespace are left exactly as produced, because in HTML those are precisely
// where a real CommonMark rendering bug shows up (`<pre>` content, hard breaks,
// tight vs loose list items). As of v0.1.0 this normalisation is the identity
// function on all 652 spec examples for both implementations: they already
// agree byte for byte.
func normaliseHTML(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSuffix(s, "\n")
}

var normalisationNotes = []string{
	"CRLF and lone CR normalised to LF",
	"a single trailing LF removed",
	"nothing else: interior whitespace, indentation, attribute order and tag text compared verbatim",
	"applied identically to the spec HTML, the upstream HTML and the Go HTML",
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
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	readEr chan error

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

	r := &runner{
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		lines:  make(chan string, 64),
		readEr: make(chan error, 1),
	}

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		r.readEr <- sc.Err()
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
// numbers become float64 (so 1 and 1.0 compare equal) and every string that
// reaches a comparison passes through normaliseHTML.
func decode(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return normaliseValue(v), nil
}

// normaliseValue applies normaliseHTML to every string in a decoded JSON value.
// Rendered HTML is always the top-level string; AST values contain literals,
// which the same rules leave untouched apart from line-ending folding.
func normaliseValue(v any) any {
	switch t := v.(type) {
	case string:
		return normaliseHTML(t)
	case []any:
		for i := range t {
			t[i] = normaliseValue(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = normaliseValue(t[k])
		}
		return t
	}
	return v
}

// show renders a value so escape sequences are readable in test output.
func show(raw json.RawMessage) string {
	v, err := decode(raw)
	if err != nil {
		return fmt.Sprintf("<unparseable %s>", raw)
	}
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
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
	Cases int `json:"cases"`

	// go vs upstream
	ParityMatch    int `json:"parityMatch"`
	ParityMismatch int `json:"parityMismatch"`

	// go vs spec.json (0 for the hand-written groups)
	SpecCases    int     `json:"specCases"`
	SpecMatch    int     `json:"specMatch"`
	SpecMismatch int     `json:"specMismatch"`
	SpecPercent  float64 `json:"specPercent"`

	// upstream vs spec.json: the harness self-check
	UpstreamSpecMismatch int `json:"upstreamSpecMismatch"`

	Deviations int `json:"deviations"`
}

type score struct {
	Library     string `json:"library"`
	Upstream    string `json:"upstream"`
	SpecVersion string `json:"specVersion"`
	GoModule    string `json:"goModule"`

	Normalisation []string `json:"htmlNormalisation"`

	Cases      int `json:"cases"`
	Deviations int `json:"deviations"`

	// Headline: conformance of the Go port against spec.json.
	SpecCases      int     `json:"specCases"`
	SpecMatch      int     `json:"specMatch"`
	SpecMismatch   int     `json:"specMismatch"`
	ConformancePct float64 `json:"conformancePercent"`

	// Cross-check: conformance of commonmark.js against spec.json. Anything
	// other than 100% means this harness, not the port, is at fault.
	UpstreamSpecMatch    int     `json:"upstreamSpecMatch"`
	UpstreamSpecMismatch int     `json:"upstreamSpecMismatch"`
	UpstreamPct          float64 `json:"upstreamConformancePercent"`

	// go vs upstream over every case, including the hand-written groups.
	ParityMatch    int     `json:"parityMatch"`
	ParityMismatch int     `json:"parityMismatch"`
	ParityPercent  float64 `json:"parityPercent"`

	Groups map[string]groupScore `json:"groups"`

	SpecMismatches   []string `json:"specMismatches"`
	ParityMismatches []string `json:"parityMismatches"`
}

var modLine = regexp.MustCompile(`github\.com/malcolmston/markdown\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/markdown " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}

	specPath := filepath.Join("cases", "spec.json")
	if _, err := os.Stat(specPath); err != nil {
		t.Skipf("cases/spec.json (CommonMark %s conformance suite) is absent: %v", specVersion, err)
	}
	assertSpecCorpus(t, specPath)

	if _, err := os.Stat(filepath.Join("node", "node_modules", "commonmark", "package.json")); err != nil {
		install := exec.Command("npm", "install", "--no-audit", "--no-fund")
		install.Dir = "node"
		if out, ierr := install.CombinedOutput(); ierr != nil {
			t.Skipf("upstream %s not installed and `npm install` failed: %v\n%s", upstreamPin, ierr, out)
		}
	}
	assertUpstreamVersion(t)

	// The spec-derived case files are generated, so regenerate them if a
	// checkout is missing them rather than scoring a partial corpus.
	if generated, _ := filepath.Glob(filepath.Join("cases", "spec-*.json")); len(generated) == 0 {
		gen := exec.Command(node, "gen-cases.mjs")
		if out, gerr := gen.CombinedOutput(); gerr != nil {
			t.Fatalf("generate spec cases: %v\n%s", gerr, out)
		}
	}

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	nodeCmd := exec.Command(node, "run.mjs")
	nodeCmd.Dir = "node"
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:          "markdown",
		Upstream:         upstreamPin,
		SpecVersion:      specVersion,
		GoModule:         goModuleVersion(),
		Normalisation:    normalisationNotes,
		Cases:            len(cases),
		Groups:           map[string]groupScore{},
		SpecMismatches:   []string{},
		ParityMismatches: []string{},
	}

	scored := 0
	for _, c := range cases {
		upRep := up.call(c)
		goRep := gor.call(c)

		r := compare(t, c, upRep, goRep)
		if !r.ran {
			// Subtest filtered out by -run: do not score it.
			continue
		}
		scored++

		g := sc.Groups[c.group]
		g.Cases++

		if c.Deviation != "" {
			g.Deviations++
			sc.Deviations++
		} else if r.parityAgreed {
			g.ParityMatch++
			sc.ParityMatch++
		} else {
			g.ParityMismatch++
			sc.ParityMismatch++
			sc.ParityMismatches = append(sc.ParityMismatches, c.ID)
		}

		if c.SpecHTML != nil {
			g.SpecCases++
			sc.SpecCases++
			if r.specAgreed {
				g.SpecMatch++
				sc.SpecMatch++
			} else {
				g.SpecMismatch++
				sc.SpecMismatch++
				sc.SpecMismatches = append(sc.SpecMismatches, c.ID)
			}
			if !r.upstreamSpecAgreed {
				g.UpstreamSpecMismatch++
				sc.UpstreamSpecMismatch++
			} else {
				sc.UpstreamSpecMatch++
			}
			if g.SpecCases > 0 {
				g.SpecPercent = float64(g.SpecMatch) * 100 / float64(g.SpecCases)
			}
		}

		sc.Groups[c.group] = g
	}

	if sc.SpecCases > 0 {
		sc.ConformancePct = float64(sc.SpecMatch) * 100 / float64(sc.SpecCases)
		sc.UpstreamPct = float64(sc.UpstreamSpecMatch) * 100 / float64(sc.SpecCases)
	}
	if compared := sc.ParityMatch + sc.ParityMismatch; compared > 0 {
		sc.ParityPercent = float64(sc.ParityMatch) * 100 / float64(compared)
	}

	t.Logf("CommonMark %s conformance (Go port vs spec.json): %d/%d (%.2f%%)",
		specVersion, sc.SpecMatch, sc.SpecCases, sc.ConformancePct)
	t.Logf("harness cross-check (commonmark.js vs spec.json): %d/%d (%.2f%%)",
		sc.UpstreamSpecMatch, sc.SpecCases, sc.UpstreamPct)
	t.Logf("parity (Go port vs commonmark.js): %d/%d (%.2f%%), %d deviations",
		sc.ParityMatch, sc.ParityMatch+sc.ParityMismatch, sc.ParityPercent, sc.Deviations)
	logWorstSections(t, sc)

	// A -run filter skips subtests, which would leave a misleading score on
	// disk, so only a complete pass is allowed to rewrite parity.json.
	if scored != len(cases) {
		t.Logf("partial run (%d of %d cases scored): leaving parity.json alone", scored, len(cases))
		return
	}
	writeScore(t, sc)
}

// logWorstSections names the spec sections the port does worst on, which is the
// point of grouping by section in the first place.
func logWorstSections(t *testing.T, sc score) {
	t.Helper()

	type row struct {
		name string
		g    groupScore
	}
	var rows []row
	for name, g := range sc.Groups {
		if g.SpecCases > 0 && g.SpecMismatch > 0 {
			rows = append(rows, row{name, g})
		}
	}
	if len(rows) == 0 {
		t.Logf("no spec section has a failing example")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].g.SpecPercent != rows[j].g.SpecPercent {
			return rows[i].g.SpecPercent < rows[j].g.SpecPercent
		}
		return rows[i].name < rows[j].name
	})
	for _, r := range rows {
		t.Logf("worst sections: %s: %d/%d (%.1f%%)", r.name, r.g.SpecMatch, r.g.SpecCases, r.g.SpecPercent)
	}
}

type result struct {
	ran                bool
	parityAgreed       bool // go == upstream
	specAgreed         bool // go == spec.json (true when the case has no spec answer)
	upstreamSpecAgreed bool // upstream == spec.json
}

// compare runs one case as a subtest, three ways.
func compare(t *testing.T, c caseSpec, upRep, goRep reply) result {
	var r result
	r.specAgreed = true
	r.upstreamSpecAgreed = true

	t.Run(c.ID, func(t *testing.T) {
		r.ran = true
		fail := func(format string, args ...any) {
			if c.Deviation != "" {
				t.Logf("deviation (%s): "+format, append([]any{c.Deviation}, args...)...)
				return
			}
			t.Errorf(format, args...)
		}

		// --- conformance: the Go answer against the specification itself.
		if c.SpecHTML != nil {
			want := normaliseHTML(*c.SpecHTML)

			if !goRep.OK {
				r.specAgreed = false
				fail("spec %s: Go failed where the spec expects HTML: %s", c.SpecLines, describe(goRep))
			} else if got, err := asString(goRep.Value); err != nil {
				r.specAgreed = false
				fail("spec %s: Go value is not a string: %v", c.SpecLines, err)
			} else if got != want {
				r.specAgreed = false
				fail("spec %s: Go output differs from CommonMark %s\n  spec: %q\n    go: %q",
					c.SpecLines, specVersion, want, got)
			}

			// Upstream against the spec is a harness self-check, never a
			// verdict on the port: log, do not fail.
			if !upRep.OK {
				r.upstreamSpecAgreed = false
				t.Logf("HARNESS WARNING spec %s: commonmark.js failed: %s", c.SpecLines, describe(upRep))
			} else if got, err := asString(upRep.Value); err != nil {
				r.upstreamSpecAgreed = false
				t.Logf("HARNESS WARNING spec %s: commonmark.js value is not a string: %v", c.SpecLines, err)
			} else if got != want {
				r.upstreamSpecAgreed = false
				t.Logf("HARNESS WARNING spec %s: commonmark.js differs from the spec it implements\n  spec: %q\n  node: %q",
					c.SpecLines, want, got)
			}
		}

		// --- parity: the Go answer against the upstream implementation.
		if upRep.OK != goRep.OK {
			fail("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
				upRep.OK, describe(upRep), goRep.OK, describe(goRep))
			return
		}
		if !upRep.OK {
			// Both failed: that is parity. Message text is not compared;
			// differences are recorded in COVERAGE.md.
			r.parityAgreed = true
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
		if !reflect.DeepEqual(upVal, goVal) {
			fail("value mismatch\n  upstream: %s\n        go: %s", show(upRep.Value), show(goRep.Value))
			return
		}
		r.parityAgreed = true
	})

	if c.Deviation != "" && r.ran {
		r.parityAgreed = true
		r.specAgreed = true
	}
	return r
}

func asString(raw json.RawMessage) (string, error) {
	v, err := decode(raw)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected a string, got %T", v)
	}
	return s, nil
}

func writeScore(t *testing.T, sc score) {
	t.Helper()
	enc, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encode parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(enc, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}

// assertSpecCorpus checks the vendored conformance suite is the pinned one, so
// the conformance figure can never be attributed to the wrong spec.
func assertSpecCorpus(t *testing.T, path string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read %s: %v", path, err)
	}
	var entries []struct {
		Markdown string `json:"markdown"`
		HTML     string `json:"html"`
		Example  int    `json:"example"`
		Section  string `json:"section"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(entries) != specExample {
		t.Fatalf("%s holds %d examples, CommonMark %s has %d", path, len(entries), specVersion, specExample)
	}
	for _, e := range entries {
		if e.Section == "" || e.Example == 0 {
			t.Fatalf("%s: example %d has no section/number; not a spec.json corpus", path, e.Example)
		}
	}
}

// assertUpstreamVersion checks the installed upstream matches the pin, so the
// score can never be attributed to the wrong oracle.
func assertUpstreamVersion(t *testing.T) {
	t.Helper()
	want := strings.SplitN(upstreamPin, "@", 2)[1]

	raw, err := os.ReadFile(filepath.Join("node", "node_modules", "commonmark", "package.json"))
	if err != nil {
		t.Skipf("cannot read installed commonmark: %v", err)
	}
	var pkg struct{ Version string }
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatalf("parse installed commonmark package.json: %v", err)
	}
	if pkg.Version != want {
		t.Fatalf("installed commonmark is %s, cases pin %s", pkg.Version, want)
	}
}
