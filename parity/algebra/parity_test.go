// Package parity drives the sympy runner and the algebra runner over the same
// cases and compares their answers.  See HARNESS.md one directory up.
//
// Run with:  GOWORK=off go test ./parity/algebra/
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

const (
	// Comparison tolerance for the numeric and set modes.  Two values agree
	// when |a-b| <= absTol + relTol*|b|.  Documented in COVERAGE.md.
	absTol = 1e-9
	relTol = 1e-9

	// A hung runner must fail its case, not the suite.
	caseTimeout = 60 * time.Second

	upstreamPin = "sympy@1.14.0"
	goModulePin = "github.com/malcolmston/algebra@v0.8.0"
)

// ---------------------------------------------------------------- case loading

type Case struct {
	ID           string          `json:"id"`
	Fn           string          `json:"fn"`
	Args         json.RawMessage `json:"args"`
	Mode         string          `json:"mode"`
	Vars         []string        `json:"vars"`
	Samples      json.RawMessage `json:"samples"`
	UpToConstant bool            `json:"upToConstant"`
	UpstreamFn   string          `json:"upstreamFn"`
	GoFn         string          `json:"goFn"`
	Note         string          `json:"note"`
	Deviation    string          `json:"deviation"`

	Group string `json:"-"`
	raw   map[string]any
}

type CaseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
	Cases    []*Case `json:"cases"`
}

func loadCases(t *testing.T) []*Case {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no case files in cases/")
	}
	sort.Strings(paths)

	var all []*Case
	seen := map[string]string{}
	for _, p := range paths {
		blob, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf CaseFile
		if err := json.Unmarshal(blob, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if cf.Upstream != upstreamPin {
			t.Fatalf("%s pins upstream %q, harness expects %q", p, cf.Upstream, upstreamPin)
		}
		// Keep the raw objects so the runners receive exactly what the file says.
		var rawFile struct {
			Cases []map[string]any `json:"cases"`
		}
		if err := json.Unmarshal(blob, &rawFile); err != nil {
			t.Fatalf("parse %s raw: %v", p, err)
		}
		for i, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			switch c.Mode {
			case "numeric", "set", "exact":
			default:
				t.Fatalf("case %s: mode must be numeric, set or exact, got %q", c.ID, c.Mode)
			}
			c.Group = cf.Group
			c.raw = rawFile.Cases[i]
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

type runner struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	replies chan *reply
	stderr  *strings.Builder
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
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, stdin: stdin, replies: make(chan *reply, 1024), stderr: &errBuf}
	go func() {
		defer close(r.replies)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var rep reply
			if err := json.Unmarshal([]byte(line), &rep); err != nil {
				rep = reply{OK: false, Error: "unparseable runner output: " + line}
			}
			r.replies <- &rep
		}
	}()
	return r
}

func (r *runner) feed(cases []*Case) {
	go func() {
		w := bufio.NewWriter(r.stdin)
		for _, c := range cases {
			blob, err := json.Marshal(c.raw)
			if err != nil {
				break
			}
			w.Write(blob)
			w.WriteByte('\n')
		}
		w.Flush()
		r.stdin.Close()
	}()
}

func (r *runner) next() (*reply, bool) {
	select {
	case rep, ok := <-r.replies:
		if !ok {
			return nil, false
		}
		return rep, true
	case <-time.After(caseTimeout):
		return nil, false
	}
}

func (r *runner) stop() {
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	_ = r.cmd.Wait()
}

// ------------------------------------------------------------------ comparison

func cmpNumeric(a, b json.RawMessage) error {
	var xs, ys [][]float64
	if err := json.Unmarshal(a, &xs); err != nil {
		return fmt.Errorf("upstream value is not a list of [re,im] pairs: %s", a)
	}
	if err := json.Unmarshal(b, &ys); err != nil {
		return fmt.Errorf("go value is not a list of [re,im] pairs: %s", b)
	}
	if len(xs) != len(ys) {
		return fmt.Errorf("length %d != %d", len(xs), len(ys))
	}
	for i := range xs {
		if len(xs[i]) != 2 || len(ys[i]) != 2 {
			return fmt.Errorf("element %d is not a [re,im] pair", i)
		}
		for k := 0; k < 2; k++ {
			if !approxEq(xs[i][k], ys[i][k]) {
				return fmt.Errorf("element %d component %d: %.17g != %.17g", i, k, xs[i][k], ys[i][k])
			}
		}
	}
	return nil
}

func approxEq(x, y float64) bool {
	if x == y {
		return true
	}
	return math.Abs(x-y) <= absTol+relTol*math.Abs(y)
}

func cmpExact(a, b json.RawMessage) error {
	var xs, ys []string
	if err := json.Unmarshal(a, &xs); err != nil {
		return fmt.Errorf("upstream value is not a list of strings: %s", a)
	}
	if err := json.Unmarshal(b, &ys); err != nil {
		return fmt.Errorf("go value is not a list of strings: %s", b)
	}
	if len(xs) != len(ys) {
		return fmt.Errorf("length %d != %d", len(xs), len(ys))
	}
	for i := range xs {
		if xs[i] != ys[i] {
			return fmt.Errorf("element %d: %q != %q", i, xs[i], ys[i])
		}
	}
	return nil
}

func compare(c *Case, up, go_ *reply) error {
	if up == nil {
		return fmt.Errorf("upstream runner produced no answer (timeout or crash)")
	}
	if go_ == nil {
		return fmt.Errorf("go runner produced no answer (timeout or crash)")
	}
	if up.ID != c.ID {
		return fmt.Errorf("upstream replied out of order: got id %q, want %q", up.ID, c.ID)
	}
	if go_.ID != c.ID {
		return fmt.Errorf("go replied out of order: got id %q, want %q", go_.ID, c.ID)
	}
	if up.OK != go_.OK {
		return fmt.Errorf("ok mismatch: upstream ok=%v (%s value=%s), go ok=%v (%s value=%s)",
			up.OK, up.Error, up.Value, go_.OK, go_.Error, go_.Value)
	}
	if !up.OK {
		return nil // both failed: that is parity
	}
	switch c.Mode {
	case "exact":
		return cmpExact(up.Value, go_.Value)
	default: // numeric, set
		return cmpNumeric(up.Value, go_.Value)
	}
}

// ----------------------------------------------------------------------- test

type mismatch struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Fn       string `json:"fn"`
	Mode     string `json:"mode"`
	Reason   string `json:"reason"`
	Upstream string `json:"upstream"`
	Go       string `json:"go"`
	Note     string `json:"note,omitempty"`
}

type groupScore struct {
	Group    string `json:"group"`
	Cases    int    `json:"cases"`
	Match    int    `json:"match"`
	Mismatch int    `json:"mismatch"`
}

type report struct {
	Library    string            `json:"library"`
	GoModule   string            `json:"goModule"`
	Upstream   string            `json:"upstream"`
	Comparison map[string]any    `json:"comparison"`
	Cases      int               `json:"cases"`
	Match      int               `json:"match"`
	Mismatch   int               `json:"mismatch"`
	BothFailed int               `json:"bothFailedAsExpected"`
	ParityPct  float64           `json:"parityPercent"`
	ByMode     map[string]int    `json:"casesByMode"`
	Groups     []groupScore      `json:"groups"`
	Mismatches []mismatch        `json:"mismatches"`
	CaseFiles  []string          `json:"caseFiles"`
	Deviations map[string]string `json:"deviations,omitempty"`
}

func TestParity(t *testing.T) {
	cases := loadCases(t)

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH; skipping cross-language parity")
	}
	if out, err := exec.Command(python, "-c", "import sympy; print(sympy.__version__)").CombinedOutput(); err != nil {
		t.Skipf("sympy not importable with %s; skipping cross-language parity: %v\n%s", python, err, out)
	} else if got := strings.TrimSpace(string(out)); got != strings.TrimPrefix(upstreamPin, "sympy@") {
		t.Skipf("sympy %s installed but cases pin %s; skipping rather than scoring against the wrong oracle", got, upstreamPin)
	}

	bin := filepath.Join(t.TempDir(), "algebra-parity-runner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the Go runner failed: %v\n%s", err, out)
	}

	upCmd := exec.Command(python, filepath.Join("python", "run.py"))
	upCmd.Env = append(os.Environ(), "PYTHONHASHSEED=0", "PYTHONDONTWRITEBYTECODE=1")
	up := startRunner(t, "sympy", upCmd)
	defer up.stop()

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gor := startRunner(t, "algebra", goCmd)
	defer gor.stop()

	up.feed(cases)
	gor.feed(cases)

	rep := report{
		Library:  "algebra",
		GoModule: goModulePin,
		Upstream: upstreamPin,
		Comparison: map[string]any{
			"modes":             []string{"numeric", "set", "exact"},
			"absoluteTolerance": absTol,
			"relativeTolerance": relTol,
			"defaultSamples":    []float64{0.3170, 0.7391, 1.4142, 2.2360, 3.1415},
			"note":              "expression strings are never compared except in the three explicit 'str' cases; numeric mode samples both answers, set mode sorts numerically evaluated solution sets, exact mode compares canonical rational/integer strings",
		},
		ByMode:     map[string]int{},
		Deviations: map[string]string{},
	}
	groups := map[string]*groupScore{}
	var groupOrder []string

	upDead, goDead := false, false
	for _, c := range cases {
		var upRep, goRep *reply
		if !upDead {
			var ok bool
			if upRep, ok = up.next(); !ok {
				upDead = true
				upRep = nil
			}
		}
		if !goDead {
			var ok bool
			if goRep, ok = gor.next(); !ok {
				goDead = true
				goRep = nil
			}
		}

		gs := groups[c.Group]
		if gs == nil {
			gs = &groupScore{Group: c.Group}
			groups[c.Group] = gs
			groupOrder = append(groupOrder, c.Group)
		}
		gs.Cases++
		rep.Cases++
		rep.ByMode[c.Mode]++
		if c.Deviation != "" {
			rep.Deviations[c.ID] = c.Deviation
		}

		err := compare(c, upRep, goRep)
		bothFailed := err == nil && upRep != nil && !upRep.OK
		if bothFailed {
			rep.BothFailed++
		}
		if err == nil {
			gs.Match++
			rep.Match++
		} else {
			gs.Mismatch++
			rep.Mismatch++
			rep.Mismatches = append(rep.Mismatches, mismatch{
				ID:       c.ID,
				Group:    c.Group,
				Fn:       c.Fn,
				Mode:     c.Mode,
				Reason:   err.Error(),
				Upstream: describe(upRep),
				Go:       describe(goRep),
				Note:     c.Note,
			})
		}

		c := c
		err2 := err
		up2, go2 := upRep, goRep
		t.Run(c.ID, func(t *testing.T) {
			if err2 != nil {
				t.Errorf("parity mismatch (%s, mode=%s): %v\n  upstream: %s\n  go:       %s\n  note: %s",
					c.Fn, c.Mode, err2, describe(up2), describe(go2), c.Note)
			}
		})
	}

	if s := up.stderr.String(); strings.TrimSpace(s) != "" {
		t.Logf("sympy runner stderr:\n%s", s)
	}
	if s := gor.stderr.String(); strings.TrimSpace(s) != "" {
		t.Logf("algebra runner stderr:\n%s", s)
	}

	sort.Strings(groupOrder)
	for _, g := range groupOrder {
		rep.Groups = append(rep.Groups, *groups[g])
	}
	files, _ := filepath.Glob(filepath.Join("cases", "*.json"))
	sort.Strings(files)
	rep.CaseFiles = files
	if rep.Cases > 0 {
		rep.ParityPct = math.Round(float64(rep.Match)/float64(rep.Cases)*1000) / 10
	}

	blob, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	if err := os.WriteFile("parity.json", append(blob, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("parity: %d/%d cases agree (%.1f%%), %d of those are agreed failures; wrote parity.json",
		rep.Match, rep.Cases, rep.ParityPct, rep.BothFailed)
}

func describe(r *reply) string {
	if r == nil {
		return "<no answer>"
	}
	if !r.OK {
		return "error: " + r.Error
	}
	s := string(r.Value)
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
