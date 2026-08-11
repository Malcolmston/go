// Package parity drives the upstream (jwt-decode@4.0.0) and Go
// (github.com/malcolmston/express/jwtdecode) runners over an identical set of
// cases and compares their answers. Upstream is the oracle: nothing here
// hand-writes an expectation.
//
// Run with:
//
//	GOWORK=off go test .
//
// GOWORK=off is required: this harness is a module of its own, deliberately
// outside the aggregator workspace, so that it can `replace` the express
// submodule and measure the local checkout.
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
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	caseTimeout = 20 * time.Second
	// library is the nested port under test, as it appears in parity.json.
	library = "express/jwtdecode"
	// npmPackage is the upstream npm package that acts as the oracle.
	npmPackage = "jwt-decode"
	// goImport is the Go package the runner exercises.
	goImport = "github.com/malcolmston/express/jwtdecode"
)

// ------------------------------------------------------------- case files

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

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

func loadCases(t *testing.T) ([]testCase, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no case files in cases/")
	}
	sort.Strings(paths)

	var all []testCase
	seen := map[string]string{}
	upstream := ""
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
			t.Fatalf("%s: missing pinned \"upstream\" version", p)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if upstream != cf.Upstream {
			t.Fatalf("%s pins %q but another file pins %q", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ----------------------------------------------------------------- runner

// runner is one long-lived subprocess speaking JSON Lines. Started once, fed
// every case in order.
type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	errs  chan error
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s stdin: %v", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s stdout: %v", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}

	r := &runner{
		name:  name,
		cmd:   cmd,
		stdin: stdin,
		lines: make(chan string, 64),
		errs:  make(chan error, 1),
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				r.lines <- line
			}
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
		close(r.lines)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	return r
}

// ask writes one case and waits for exactly one reply, bounded by caseTimeout so
// a hung runner fails that case rather than the whole suite.
func (r *runner) ask(c testCase) (reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	b, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.stdin.Write(append(b, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return reply{}, fmt.Errorf("%s: runner exited before answering %q", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{}, fmt.Errorf("%s: bad reply %q: %w", r.name, line, err)
		}
		if rep.ID != c.ID {
			return reply{}, fmt.Errorf("%s: reply out of order: got %q want %q", r.name, rep.ID, c.ID)
		}
		return rep, nil
	case err := <-r.errs:
		return reply{}, fmt.Errorf("%s: read: %w", r.name, err)
	case <-time.After(caseTimeout):
		return reply{}, fmt.Errorf("%s: timed out after %s on %q", r.name, caseTimeout, c.ID)
	}
}

// --------------------------------------------------------- normalising eq

// normEqual is a deep-equal that treats all JSON numbers as float64, so 1 and
// 1.0 compare equal, and ignores object key order.
func normEqual(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, av := range x {
			bv, ok := y[k]
			if !ok || !normEqual(av, bv) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !normEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
		return x == y
	case nil:
		return b == nil
	default:
		return reflect.DeepEqual(a, b)
	}
}

func decodeValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode value %q: %v", raw, err)
	}
	return v
}

func show(rep reply) string {
	if !rep.OK {
		return "FAIL(" + rep.Error + ")"
	}
	if len(rep.Value) == 0 {
		return "OK(<absent>)"
	}
	s := string(rep.Value)
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return "OK(" + s + ")"
}

// ------------------------------------------------------------------ score

type score struct {
	Library      string       `json:"library"`
	GoPackage    string       `json:"goPackage"`
	GoModule     string       `json:"goModule"`
	Upstream     string       `json:"upstream"`
	Generated    string       `json:"generated"`
	Total        int          `json:"total"`
	Match        int          `json:"match"`
	Mismatch     int          `json:"mismatch"`
	Deviations   int          `json:"deviations"`
	ParityPct    float64      `json:"parityPct"`
	Groups       []groupScore `json:"groups"`
	Mismatched   []string     `json:"mismatchedCases"`
	DeviationIDs []string     `json:"deviationCases"`
}

type groupScore struct {
	Group    string `json:"group"`
	Total    int    `json:"total"`
	Match    int    `json:"match"`
	Mismatch int    `json:"mismatch"`
}

// -------------------------------------------------------------------- test

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not found in PATH; skipping cross-language parity for %s", library)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", npmPackage)); err != nil {
		t.Skipf("node/node_modules/%s missing; run `npm install` in the node/ directory", npmPackage)
	}

	cases, upstream := loadCases(t)

	// fixtures/ holds any file-backed inputs (keys, PEMs). Passed to both
	// runners as argv[1] so neither has to guess a path.
	fixtures, err := filepath.Abs("fixtures")
	if err != nil {
		t.Fatalf("fixtures dir: %v", err)
	}

	// Build the Go runner once so no case pays compilation cost or hits the
	// per-case timeout on a cold build cache.
	bin := filepath.Join(t.TempDir(), "parity-go-runner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build go runner: %v", err)
	}

	up := startRunner(t, "node", exec.Command(node, filepath.Join("node", "run.js"), fixtures))
	gorun := startRunner(t, "go", exec.Command(bin, fixtures))

	s := score{
		Library:   library,
		GoPackage: goImport,
		GoModule:  goModuleVersion(t),
		Upstream:  upstream,
	}
	byGroup := map[string]*groupScore{}
	var order []string

	for _, c := range cases {
		g, ok := byGroup[c.group]
		if !ok {
			g = &groupScore{Group: c.group}
			byGroup[c.group] = g
			order = append(order, c.group)
		}
		g.Total++
		s.Total++

		matched := t.Run(c.ID, func(t *testing.T) {
			ur, err := up.ask(c)
			if err != nil {
				t.Fatalf("upstream runner: %v", err)
			}
			gr, err := gorun.ask(c)
			if err != nil {
				t.Fatalf("go runner: %v", err)
			}

			same := ur.OK == gr.OK
			if same && ur.OK {
				same = normEqual(decodeValue(t, ur.Value), decodeValue(t, gr.Value))
			}
			if same {
				return
			}
			msg := fmt.Sprintf("case %q (%s)\n  fn:       %s\n  upstream: %s\n  go:       %s",
				c.ID, c.group, c.Fn, show(ur), show(gr))
			if c.Note != "" {
				msg += "\n  note:     " + c.Note
			}
			if c.Deviation != "" {
				// A documented, deliberate difference: reported but not a bug.
				t.Skipf("known deviation: %s\n%s", c.Deviation, msg)
			}
			if !ur.OK && gr.OK {
				msg = "SECURITY: the port ACCEPTS what upstream REJECTS\n" + msg
			}
			t.Error(msg)
		})

		switch {
		case c.Deviation != "":
			s.Deviations++
			s.DeviationIDs = append(s.DeviationIDs, c.ID)
		case matched:
			s.Match++
			g.Match++
		default:
			s.Mismatch++
			g.Mismatch++
			s.Mismatched = append(s.Mismatched, c.ID)
		}
	}

	compared := s.Match + s.Mismatch
	if compared > 0 {
		s.ParityPct = math.Round(float64(s.Match)/float64(compared)*10000) / 100
	}
	for _, name := range order {
		s.Groups = append(s.Groups, *byGroup[name])
	}
	s.Generated = time.Now().UTC().Format(time.RFC3339)
	writeScore(t, s)

	t.Logf("%s parity: %d/%d cases match (%.2f%%), %d deviations, upstream %s, port %s",
		library, s.Match, compared, s.ParityPct, s.Deviations, s.Upstream, s.GoModule)
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		for i, w := range f {
			if w == "github.com/malcolmston/express" && i+1 < len(f) {
				return w + "@" + f[i+1]
			}
		}
	}
	return "unknown"
}

func writeScore(t *testing.T, s score) {
	t.Helper()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
}
