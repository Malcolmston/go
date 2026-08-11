// Package parity drives the upstream (otpauth) and Go
// (github.com/malcolmston/passport/otpauth) runners over an identical set of
// cases and compares their answers. Upstream is the oracle: nothing here
// hand-writes an expectation.
//
// This is a NESTED harness. parity/passport/ scores passport itself against
// passport@0.7.0 and its strategies; this directory scores the nested otpauth
// package against its own upstream, the npm package of the same name, which owns
// both the otpauth:// key-URI format and the HOTP/TOTP construction.
//
// Run with:
//
//	GOWORK=off go test ./parity/passport/nested/otpauth/
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
	"sync"
	"testing"
	"time"
)

const (
	// library names this harness in parity.json.
	library = "passport/otpauth"
	// goPkg is the Go import path under test, used to read its version out of
	// go.mod.
	goPkg = "github.com/malcolmston/passport"
	// caseTimeout bounds one request/reply exchange, so a hung runner fails
	// that case rather than the suite.
	caseTimeout = 30 * time.Second
)

// nodeDeps must exist under node/node_modules before the harness can run.
var nodeDeps = []string{"otpauth"}

// securityIDs collects cases where the Go port ACCEPTED a credential upstream
// rejected. Those are never filed as public issues; they go to security.json.
var (
	securityMu  sync.Mutex
	securityIDs []string
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
	// Acceptance marks a case whose value is an accept/reject verdict on a
	// credential. Only for those does "port says yes, upstream says no" mean a
	// security finding rather than a behavioural difference.
	Acceptance bool `json:"acceptance"`

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

// ask writes one case and waits for exactly one reply, bounded by caseTimeout.
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

// ------------------------------------------------------- security detection

// portAcceptsWhereUpstreamRejects reports whether an acceptance case ended with
// the port admitting a credential upstream refused: either upstream errored
// while the port returned a verdict of true, or both answered and upstream said
// false where the port said true. Any other divergence is a behavioural
// difference, not a security finding.
func portAcceptsWhereUpstreamRejects(c testCase, upstreamOK bool, upstream any, portOK bool, port any) bool {
	if !c.Acceptance || !portOK {
		return false
	}
	accepted, ok := port.(bool)
	if !ok || !accepted {
		return false
	}
	if !upstreamOK {
		return true
	}
	rejected, ok := upstream.(bool)
	return ok && !rejected
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
	GoModule     string       `json:"goModule"`
	GoPackage    string       `json:"goPackage"`
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
	Security     []string     `json:"securityFindings"`
}

type groupScore struct {
	Group      string `json:"group"`
	Total      int    `json:"total"`
	Match      int    `json:"match"`
	Mismatch   int    `json:"mismatch"`
	Deviations int    `json:"deviations"`
}

// -------------------------------------------------------------------- test

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not found in PATH; skipping cross-language parity for %s", library)
	}
	for _, dep := range nodeDeps {
		if _, err := os.Stat(filepath.Join("node", "node_modules", dep)); err != nil {
			t.Skipf("node/node_modules/%s missing; run `npm install` in this harness's node/ directory", dep)
		}
	}

	cases, upstream := loadCases(t)

	// Build the Go runner once so no case pays compilation cost or hits the
	// per-case timeout on a cold build cache.
	bin := filepath.Join(t.TempDir(), "parity-go-runner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build go runner: %v", err)
	}

	up := startRunner(t, "node", exec.Command(node, filepath.Join("node", "run.js")))
	gorun := startRunner(t, "go", exec.Command(bin))

	s := score{
		Library:   library,
		GoModule:  goModuleVersion(t),
		GoPackage: goPkg + "/otpauth",
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

			uv, gv := decodeValue(t, ur.Value), decodeValue(t, gr.Value)
			same := ur.OK == gr.OK
			if same && ur.OK {
				same = normEqual(uv, gv)
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
				// A documented, deliberate difference: reported, not a bug, and
				// never a silent security finding — labelling a case as a
				// deviation is a human decision that has already weighed what
				// the difference means.
				t.Skipf("known deviation: %s\n%s", c.Deviation, msg)
			}
			if portAcceptsWhereUpstreamRejects(c, ur.OK, uv, gr.OK, gv) {
				securityMu.Lock()
				securityIDs = append(securityIDs, c.ID)
				securityMu.Unlock()
				msg = "SECURITY: the port ACCEPTS a credential upstream REJECTS\n" + msg
			}
			t.Error(msg)
		})

		switch {
		case c.Deviation != "":
			s.Deviations++
			g.Deviations++
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
	securityMu.Lock()
	s.Security = append([]string(nil), securityIDs...)
	securityMu.Unlock()
	sort.Strings(s.Security)
	s.Generated = time.Now().UTC().Format(time.RFC3339)
	writeScore(t, s)
	if len(s.Security) > 0 {
		t.Logf("SECURITY findings (port accepts, upstream rejects): %s", strings.Join(s.Security, ", "))
	}

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
			if w == goPkg && i+1 < len(f) {
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
