// Package parity drives real React (react-dom/server, pinned in node/) and the
// Go port (github.com/malcolmston/react) over an identical corpus of trees and
// compares the HTML they produce. Upstream is the oracle: no expectation is
// written down anywhere in this directory.
//
// The primary gate for this library is the vitest suite in node/, because the
// user-facing question — "does the port render what React renders" — is asked
// most naturally from the ecosystem that owns the oracle:
//
//	cd parity/react/node && npm install && npx vitest run
//
// This test exists so the same corpus is reachable from Go, for someone who has
// the toolchain for the port but not the one for the oracle:
//
//	GOWORK=off go test ./parity/react/
//
// Both producers write the same parity/react/parity.json. When Node is missing
// this test skips — a Go-only checkout must not go red over an absent oracle.
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
	"sort"
	"strings"
	"testing"
	"time"
)

const caseTimeout = 20 * time.Second

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
	Feature    string            `json:"feature"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value string `json:"value"`
	Error string `json:"error"`
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

// runner is one long-lived subprocess speaking JSON Lines: started once, fed
// every case in order.
type runner struct {
	name  string
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
		stdin: stdin,
		lines: make(chan string, 64),
		errs:  make(chan error, 1),
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
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
	b, err := json.Marshal(map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args})
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

func show(rep reply) string {
	if !rep.OK {
		return "FAIL(" + rep.Error + ")"
	}
	s := rep.Value
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return quote(s)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ------------------------------------------------------------------ score

type score struct {
	Library      string       `json:"library"`
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
		t.Skip("node not found in PATH; skipping cross-language parity for react")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "react-dom")); err != nil {
		t.Skip("parity/react/node/node_modules/react-dom missing; run `npm install` in parity/react/node")
	}

	cases, upstream := loadCases(t)

	// Build the Go runner once so no case pays compilation cost or trips the
	// per-case timeout on a cold build cache.
	bin := filepath.Join(t.TempDir(), "react-parity-go-runner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build go runner: %v", err)
	}

	up := startRunner(t, "node", exec.Command(node, filepath.Join("node", "run.mjs")))
	gorun := startRunner(t, "go", exec.Command(bin))

	s := score{
		Library:  "react",
		GoModule: goModuleVersion(),
		Upstream: upstream,
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

			same := ur.OK == gr.OK && (!ur.OK || ur.Value == gr.Value)
			msg := fmt.Sprintf("case %q (%s)\n  fn:       %s\n  upstream: %s\n  port:     %s",
				c.ID, featureOf(c.Feature, c.group), c.Fn, show(ur), show(gr))
			if c.Note != "" {
				msg += "\n  note:     " + c.Note
			}
			if c.Deviation != "" {
				// A documented, deliberate difference: reported, never a failure —
				// but it stops being a deviation the moment the two agree.
				if same {
					t.Fatalf("case %q is marked as a deviation but upstream and the port "+
						"now agree; remove the deviation marker.\n%s", c.ID, msg)
				}
				t.Skipf("known deviation: %s\n%s", c.Deviation, msg)
			}
			if !same {
				t.Error(msg)
			}
		})

		g.Total++
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
	sort.Strings(order)
	for _, name := range order {
		s.Groups = append(s.Groups, *byGroup[name])
	}
	s.Generated = time.Now().UTC().Format(time.RFC3339)
	writeScore(t, s)

	t.Logf("react parity: %d/%d cases match (%.2f%%), %d documented deviations, upstream %s, port %s",
		s.Match, compared, s.ParityPct, s.Deviations, s.Upstream, s.GoModule)
}

func featureOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func goModuleVersion() string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		for i, w := range f {
			if w == "github.com/malcolmston/react" && i+1 < len(f) && strings.HasPrefix(f[i+1], "v") {
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
