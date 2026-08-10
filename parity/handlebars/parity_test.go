// Package parity drives the upstream handlebars.js runner and the Go
// github.com/malcolmston/handlebars runner over the same case files and compares
// their answers. Upstream is the oracle: no expectations are hand-written.
//
//	GOWORK=off go test ./parity/handlebars/
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

const caseTimeout = 15 * time.Second

// ---------------------------------------------------------------------------
// case files

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
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
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no case files found in cases/")
	}
	var out []testCase
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
			t.Fatalf("%s: upstream %q disagrees with %q", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			out = append(out, c)
		}
	}
	return out, upstream
}

// ---------------------------------------------------------------------------
// runners

type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
	errc  chan error
	dead  bool
}

func start(t *testing.T, name string, cmd *exec.Cmd) *runner {
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
	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 64), errc: make(chan error, 1)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		r.errc <- sc.Err()
		close(r.lines)
	}()
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})
	return r
}

func (r *runner) call(c testCase) reply {
	if r.dead {
		return reply{ID: c.ID, OK: false, Error: "runner is dead"}
	}
	req := map[string]interface{}{"id": c.ID, "fn": c.Fn, "args": c.Args}
	b, err := json.Marshal(req)
	if err != nil {
		return reply{ID: c.ID, OK: false, Error: "marshal: " + err.Error()}
	}
	if _, err := r.stdin.Write(append(b, '\n')); err != nil {
		r.dead = true
		return reply{ID: c.ID, OK: false, Error: "write: " + err.Error()}
	}
	select {
	case line, open := <-r.lines:
		if !open {
			r.dead = true
			return reply{ID: c.ID, OK: false, Error: "runner exited"}
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{ID: c.ID, OK: false, Error: "bad reply: " + err.Error()}
		}
		return rep
	case <-time.After(caseTimeout):
		r.dead = true
		_ = r.cmd.Process.Kill()
		return reply{ID: c.ID, OK: false, Error: "timeout"}
	}
}

// ---------------------------------------------------------------------------
// comparison

// normalise decodes JSON into a shape where all numbers are float64 so that 1
// and 1.0 compare equal, and where map key order is irrelevant.
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

func deepEqual(a, b interface{}) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && (x == y || (math.IsNaN(x) && math.IsNaN(y)))
	case map[string]interface{}:
		y, ok := b.(map[string]interface{})
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, present := y[k]
			if !present || !deepEqual(xv, yv) {
				return false
			}
		}
		return true
	case []interface{}:
		y, ok := b.([]interface{})
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !deepEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func show(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// ---------------------------------------------------------------------------

type score struct {
	Library      string            `json:"library"`
	Upstream     string            `json:"upstream"`
	GoModule     string            `json:"goModule"`
	Cases        int               `json:"cases"`
	Match        int               `json:"match"`
	Mismatch     int               `json:"mismatch"`
	Deviations   int               `json:"deviations"`
	ParityPct    float64           `json:"parityPercent"`
	ByGroup      map[string]string `json:"byGroup"`
	Mismatched   []string          `json:"mismatchedCases"`
	BothFailed   int               `json:"bothFailedAsExpected"`
	Note         string            `json:"note"`
	CaseTimeoutS float64           `json:"caseTimeoutSeconds"`
}

func goModuleVersion(t *testing.T) string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "github.com/malcolmston/handlebars") {
			f := strings.Fields(strings.TrimSpace(line))
			for i, w := range f {
				if w == "github.com/malcolmston/handlebars" && i+1 < len(f) {
					return w + "@" + f[i+1]
				}
			}
		}
	}
	return ""
}

func TestParity(t *testing.T) {
	cases, upstream := loadCases(t)

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not found in PATH: %v", err)
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "handlebars")); err != nil {
		t.Skipf("upstream not installed: run `npm install` in parity/handlebars/node (%v)", err)
	}

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if out, err := build.Output(); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}

	up := start(t, "node", exec.Command(node, filepath.Join("node", "run.js")))
	gorun := start(t, "go", exec.Command(bin))

	var (
		match, mismatch, deviations, bothFailed int
		mismatched                              []string
		groupTotal                              = map[string]int{}
		groupMatch                              = map[string]int{}
	)

	for _, c := range cases {
		c := c
		ur := up.call(c)
		gr := gorun.call(c)
		groupTotal[c.group]++

		ok := true
		t.Run(c.ID, func(t *testing.T) {
			if ur.OK != gr.OK {
				ok = false
				t.Errorf("ok mismatch: upstream ok=%v (%s) go ok=%v (%s)\n  upstream value: %s\n  go value:       %s",
					ur.OK, ur.Error, gr.OK, gr.Error, string(ur.Value), string(gr.Value))
				return
			}
			if !ur.OK {
				// Both failed: that is parity. Message text is recorded in
				// COVERAGE.md, never compared.
				return
			}
			uv, err := normalise(ur.Value)
			if err != nil {
				ok = false
				t.Errorf("upstream value not JSON: %v", err)
				return
			}
			gv, err := normalise(gr.Value)
			if err != nil {
				ok = false
				t.Errorf("go value not JSON: %v", err)
				return
			}
			if !deepEqual(uv, gv) {
				ok = false
				t.Errorf("value mismatch\n  upstream: %s\n  go:       %s\n  note: %s", show(uv), show(gv), c.Note)
			}
		})

		switch {
		case ok:
			match++
			groupMatch[c.group]++
			if !ur.OK {
				bothFailed++
			}
		case c.Deviation != "":
			deviations++
			mismatched = append(mismatched, c.ID+" (deviation: "+c.Deviation+")")
		default:
			mismatch++
			mismatched = append(mismatched, c.ID)
		}
	}

	total := len(cases)
	byGroup := map[string]string{}
	for g, n := range groupTotal {
		byGroup[g] = fmt.Sprintf("%d/%d", groupMatch[g], n)
	}
	pct := 0.0
	if total > 0 {
		pct = math.Round(float64(match)/float64(total)*1000) / 10
	}
	s := score{
		Library:      "handlebars",
		Upstream:     upstream,
		GoModule:     goModuleVersion(t),
		Cases:        total,
		Match:        match,
		Mismatch:     mismatch,
		Deviations:   deviations,
		ParityPct:    pct,
		ByGroup:      byGroup,
		Mismatched:   mismatched,
		BothFailed:   bothFailed,
		Note:         "parityPercent = match/cases over every case in cases/*.json; a case counts as a match when both runners agree on ok, and on value when ok is true. Regenerated by `GOWORK=off go test ./parity/handlebars/`.",
		CaseTimeoutS: caseTimeout.Seconds(),
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("parity: %d/%d cases (%.1f%%), %d mismatches, %d deviations, %d cases where both sides failed",
		match, total, pct, mismatch, deviations, bothFailed)
}
