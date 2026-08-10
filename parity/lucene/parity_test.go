// Package parity drives the upstream Apache Lucene runner (Java) and the Go port
// runner over the same JSON case files and compares their answers.
//
// Upstream is the oracle: nothing here hand-writes an expected value. Both
// runners index the identical corpus from fixtures/corpus.json, so a search
// divergence is a divergence in semantics and not in the data.
//
// Scores are never compared. Two independent BM25 implementations will not agree
// to the last bit and it would prove nothing if they did; what is compared is the
// number of matches, the result SET, and the ranked ORDER of doc ids with
// equal-scoring ids sorted inside their tie.
//
// Run with:  GOWORK=off go test ./parity/lucene/
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	caseTimeout  = 60 * time.Second
	buildTimeout = 10 * time.Minute
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
	name  string
	cmd   *exec.Cmd
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
		if p, ok := r.cmd.Stdin.(interface{ Close() error }); ok {
			_ = p.Close()
		}
		if sc, ok := any(r.cmd.Process).(interface{ Kill() error }); ok && r.cmd.Process != nil {
			// give it a moment to exit on EOF, then make sure it is gone
			done := make(chan struct{})
			go func() { _ = r.cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = sc.Kill()
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
		return fmt.Sprintf("%v", v)
	}
	return truncate(string(b))
}

// ------------------------------------------------------------------- the report

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
}

// --------------------------------------------------------------------- helpers

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/lucene")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "github.com/malcolmston/lucene (version unknown)"
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

// buildJavaRunner builds the upstream oracle: a self-contained Maven project
// with a pinned lucene-core. A missing java or mvn, or a build that cannot fetch
// the pinned artifacts, skips the suite rather than failing it -- someone with
// only a Go toolchain must not see a red build.
func buildJavaRunner(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found in PATH; skipping lucene parity suite")
	}
	mvn, err := exec.LookPath("mvn")
	if err != nil {
		t.Skip("mvn not found in PATH; skipping lucene parity suite")
	}
	jar, jerr := filepath.Abs(filepath.Join("java", "target", "runner-shaded.jar"))
	if jerr != nil {
		t.Skipf("cannot resolve the runner jar path: %v", jerr)
	}

	// Offline first: a warm ~/.m2 makes the suite work with no network at all.
	// Only fall back to a networked build if the offline one cannot produce the jar.
	if err := runMvn(t, mvn, "-o"); err != nil {
		if _, serr := os.Stat(jar); serr == nil {
			return jar // an offline plugin gap, but a usable jar is already there
		}
		if err2 := runMvn(t, mvn, ""); err2 != nil {
			t.Skipf("maven could not build the upstream runner (no network for the pinned lucene artifacts?): offline: %v; online: %v", err, err2)
		}
	}
	if _, serr := os.Stat(jar); serr != nil {
		t.Skipf("maven produced no jar at %s: %v", jar, serr)
	}
	return jar
}

func runMvn(t *testing.T, mvn, flag string) error {
	t.Helper()
	args := []string{"-q", "package"}
	if flag != "" {
		args = append([]string{flag}, args...)
	}
	cmd := exec.Command(mvn, args...)
	cmd.Dir = "java"
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start mvn: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w\n%s", err, truncate(out.String()))
		}
		return nil
	case <-time.After(buildTimeout):
		_ = cmd.Process.Kill()
		return fmt.Errorf("mvn timed out after %s", buildTimeout)
	}
}

// ------------------------------------------------------------------------ test

func TestParity(t *testing.T) {
	cases := loadCases(t)

	jar := buildJavaRunner(t)
	goBin := buildGoRunner(t)

	corpus, err := filepath.Abs(filepath.Join("fixtures", "corpus.json"))
	if err != nil {
		t.Fatalf("resolve corpus fixture: %v", err)
	}
	if _, err := os.Stat(corpus); err != nil {
		t.Fatalf("shared corpus fixture missing: %v", err)
	}

	up := startRunner(t, "java", "java", "-jar", jar, corpus)
	port := startRunner(t, "go", goBin, corpus)

	rep := report{
		Library:      "lucene",
		Upstream:     "lucene@9.11.1",
		UpstreamKind: "java",
		GoModule:     goModuleVersion(t),
		Groups:       map[string]groupScore{},
	}

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

		// Both runners must be fed in lockstep, outside of t.Run's ordering, so
		// the streams stay aligned even if a subtest reports a failure.
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
				// Both failed: that is parity. Message text is not compared.
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
				fail("case %s (%s):\n  upstream %s = %s\n  go       %s = %s",
					c.ID, c.Note, c.UpstreamFn, show(upVal), c.GoFn, show(goVal))
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
	t.Logf("lucene parity: %d cases, %d match, %d mismatch, %d deviations, %.2f%%",
		rep.Cases, rep.Match, rep.Mismatch, rep.Deviations, rep.ParityPercent)
}
