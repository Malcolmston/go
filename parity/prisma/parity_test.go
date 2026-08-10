// Package parity drives the upstream (Node/prisma@5.22.0 over SQLite) and Go
// (github.com/malcolmston/prisma) runners over the same case files and compares
// their answers. Upstream is the oracle: nothing here hand-writes an expected
// value.
//
// Run with:
//
//	cd parity/prisma && GOWORK=off go test ./
//
// The suite skips (never fails) when Node, the pinned upstream install, the
// generated Prisma Client or the `prisma db push` step are unavailable, so a
// Go-only checkout stays green.
//
// # What is and is not being compared
//
// The Go port has no schema language, no migration engine and no code generator:
// a model is a Go struct with `prisma:"..."` tags, read by reflection at run
// time. There is therefore no schema workflow to compare, and none is claimed.
// What is compared is QUERY SEMANTICS: the two logical models are declared once
// as a Prisma datamodel (node/schema.prisma) and once as tagged Go structs
// (go/run.go), the DDL is produced ONCE by `prisma db push` and copied by both
// runners so neither side hand-wrote a table, and every case seeds the same
// fixed rows, runs one query and emits the resulting rows. Values are compared,
// never SQL text. The absent schema/migrate/generate surface is inventoried as
// `missing` in COVERAGE.md.
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
	upstreamPin = "prisma@5.22.0"
	// A case is a whole script — reset, seed, query, dump — against a real
	// database, so it is allowed considerably longer than a pure-function case.
	caseTimeout = 60 * time.Second
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
	Note     string     `json:"note,omitempty"`
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
		// Both runners log every failing step's message to stderr. Those
		// messages are deliberately not part of the comparison, but they are
		// what makes a mismatch diagnosable.
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

// show renders a value for test output, re-encoding it so the two sides print in
// the same key order and a diff is readable.
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

// firstStepDiff points at the first step of a script whose two answers differ,
// which is far more useful than a diff of the whole script when a case seeds
// eight users and dumps them twice.
func firstStepDiff(up, gov any) string {
	ups, uok := stepsOf(up)
	gos, gok := stepsOf(gov)
	if !uok || !gok {
		return ""
	}
	n := len(ups)
	if len(gos) > n {
		n = len(gos)
	}
	for i := 0; i < n; i++ {
		var a, b any
		if i < len(ups) {
			a = ups[i]
		}
		if i < len(gos) {
			b = gos[i]
		}
		if !reflect.DeepEqual(a, b) {
			ja, _ := json.Marshal(a)
			jb, _ := json.Marshal(b)
			return fmt.Sprintf("first differing step is #%d\n  upstream: %s\n        go: %s", i, ja, jb)
		}
	}
	return ""
}

func stepsOf(v any) ([]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	s, ok := m["steps"].([]any)
	return s, ok
}

// ---------------------------------------------------------------- the score

type groupScore struct {
	Cases      int `json:"cases"`
	Match      int `json:"match"`
	Mismatch   int `json:"mismatch"`
	Deviations int `json:"deviations"`
}

type score struct {
	Library           string                `json:"library"`
	Upstream          string                `json:"upstream"`
	UpstreamEcosystem string                `json:"upstreamEcosystem"`
	GoModule          string                `json:"goModule"`
	Cases             int                   `json:"cases"`
	Match             int                   `json:"match"`
	Mismatch          int                   `json:"mismatch"`
	Deviations        int                   `json:"deviations"`
	ParityPercent     float64               `json:"parityPercent"`
	Groups            map[string]groupScore `json:"groups"`
	Mismatches        []string              `json:"mismatches"`
}

var modLine = regexp.MustCompile(`github\.com/malcolmston/prisma\s+(v\S+)`)

func goModuleVersion() string {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	if m := modLine.FindSubmatch(raw); m != nil {
		return "github.com/malcolmston/prisma " + string(m[1])
	}
	return "unknown"
}

// ------------------------------------------------------------------- harness

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}

	if _, err := os.Stat(filepath.Join("node", "node_modules", "prisma", "package.json")); err != nil {
		install := exec.Command("npm", "install", "--no-audit", "--no-fund")
		install.Dir = "node"
		if out, ierr := install.CombinedOutput(); ierr != nil {
			t.Skipf("upstream %s not installed and `npm install` failed: %v\n%s", upstreamPin, ierr, out)
		}
	}
	assertUpstreamVersion(t)

	template := prepareSchema(t)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go runner: %v\n%s", err, out)
	}

	cases := loadCases(t, "cases")

	// Each runner gets its own copy of the pushed schema in its own temp
	// directory, so the two never share a database file.
	nodeCmd := exec.Command(node, "run.js")
	nodeCmd.Dir = "node"
	nodeCmd.Env = append(os.Environ(), "PARITY_TEMPLATE_DB="+template)
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off", "PARITY_TEMPLATE_DB="+template)
	gor := startRunner(t, "go", goCmd)

	sc := score{
		Library:           "prisma",
		Upstream:          upstreamPin,
		UpstreamEcosystem: "node",
		GoModule:          goModuleVersion(),
		Cases:             len(cases),
		Groups:            map[string]groupScore{},
		Mismatches:        []string{},
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
	// disk, so only a complete pass is allowed to rewrite parity.json.
	if scored := compared + sc.Deviations; scored != len(cases) {
		t.Logf("partial run (%d of %d cases scored): leaving parity.json alone",
			scored, len(cases))
		return
	}
	writeScore(t, sc)
}

// prepareSchema runs the two upstream schema steps once — `prisma generate` for
// the client and `prisma db push` for the tables — and returns the path of the
// pushed SQLite file. Both runners copy it, which is what guarantees the oracle
// and the port execute against identical DDL without either one hand-writing a
// CREATE TABLE. If the steps cannot run (no engines, no network for a first
// install) the suite skips rather than fails.
func prepareSchema(t *testing.T) string {
	t.Helper()

	cli := filepath.Join("node", "node_modules", ".bin", "prisma")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("prisma CLI not installed: %v", err)
	}

	// The template lives beside the runner rather than in t.TempDir() so a
	// repeated run reuses it and `db push` becomes a no-op.
	dir := filepath.Join("node", ".parity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	abs, err := filepath.Abs(filepath.Join(dir, "template.db"))
	if err != nil {
		t.Fatalf("abs template path: %v", err)
	}

	run := func(args ...string) error {
		cmd := exec.Command(filepath.Join("node_modules", ".bin", "prisma"), args...)
		cmd.Dir = "node"
		cmd.Env = append(os.Environ(), "PARITY_DB_URL=file:"+abs)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("prisma %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return nil
	}

	if err := run("generate", "--schema", "schema.prisma"); err != nil {
		t.Skipf("`prisma generate` cannot run here, so there is no client to compare against: %v", err)
	}
	if err := run("db", "push", "--schema", "schema.prisma", "--skip-generate", "--accept-data-loss"); err != nil {
		t.Skipf("`prisma db push` cannot run here, so there is no schema to compare against: %v", err)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", ".prisma", "client", "index.js")); err != nil {
		t.Skipf("generated Prisma Client is missing: %v", err)
	}
	return abs
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
			if c.Note != "" {
				t.Logf("note: %s", c.Note)
			}
			t.Errorf(format, args...)
		}

		// A runner-level failure means the script could not be driven at all;
		// individual steps report their own ok/code inside `value`.
		if upRep.OK != goRep.OK {
			fail("runner ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
				upRep.OK, describe(upRep), goRep.OK, describe(goRep))
			return
		}
		if !upRep.OK {
			agreed = true
			if upRep.Error != goRep.Error {
				t.Logf("both runners refused the case, different messages: upstream %q, go %q",
					upRep.Error, goRep.Error)
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
			if d := firstStepDiff(upVal, goVal); d != "" {
				fail("value mismatch: %s", d)
			} else {
				fail("value mismatch\n  upstream: %s\n        go: %s",
					show(upRep.Value), show(goRep.Value))
			}
			return
		}
		agreed = true
	})

	if c.Deviation != "" {
		return ran, ran
	}
	return ran, agreed
}

func describe(r reply) string {
	if r.OK {
		return "value " + show(r.Value)
	}
	return "error " + fmt.Sprintf("%q", r.Error)
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

// assertUpstreamVersion checks the installed upstream matches the pin, for both
// the CLI and the client, so the score can never be attributed to the wrong
// oracle.
func assertUpstreamVersion(t *testing.T) {
	t.Helper()
	want := strings.SplitN(upstreamPin, "@", 2)[1]

	for _, pkg := range []string{"prisma", filepath.Join("@prisma", "client")} {
		path := filepath.Join("node", "node_modules", pkg, "package.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("cannot read installed %s: %v", pkg, err)
		}
		var meta struct{ Version string }
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if meta.Version != want {
			t.Fatalf("installed %s is %s, cases pin %s", pkg, meta.Version, want)
		}
	}
}
