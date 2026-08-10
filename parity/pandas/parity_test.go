// Package parity drives the upstream (pandas) runner and the Go
// (github.com/malcolmston/pandas) runner over the same cases and compares
// their answers. Upstream is the oracle; nothing here is hand-written
// expectation.
//
// Run with:
//
//	GOWORK=off go test ./parity/pandas/
//
// The test skips (never fails) when python3 or pandas is unavailable.
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// floatTol is the comparison tolerance for floating-point values: two numbers
// match when |a-b| <= 1e-9 or |a-b|/max(|a|,|b|) <= 1e-9. Integral values
// compare exactly under this rule, and 1 vs 1.0 are equal because every JSON
// number is decoded as float64.
const floatTol = 1e-9

const caseTimeout = 20 * time.Second

// ---------------------------------------------------------------------------
// case loading
// ---------------------------------------------------------------------------

type parityCase struct {
	ID         string          `json:"id"`
	Fn         string          `json:"fn"`
	Args       json.RawMessage `json:"args"`
	UpstreamFn string          `json:"upstreamFn"`
	GoFn       string          `json:"goFn"`
	Note       string          `json:"note"`
	Deviation  string          `json:"deviation"`

	group string
}

type caseFile struct {
	Group    string       `json:"group"`
	Upstream string       `json:"upstream"`
	Cases    []parityCase `json:"cases"`
}

type request struct {
	ID   string          `json:"id"`
	Fn   string          `json:"fn"`
	Args json.RawMessage `json:"args"`
}

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

// loadTables reads the shared input tables.
func loadTables(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("fixtures/tables.json")
	if err != nil {
		t.Fatalf("read fixtures/tables.json: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("parse fixtures/tables.json: %v", err)
	}
	delete(raw, "_comment")
	return raw
}

// inlineTables replaces args.table / args.right table *names* with the table
// object itself, so both runners receive byte-identical input data and neither
// needs filesystem access.
func inlineTables(t *testing.T, c parityCase, tables map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	if len(c.Args) == 0 {
		return c.Args
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(c.Args, &m); err != nil {
		t.Fatalf("case %s: parse args: %v", c.ID, err)
	}
	for _, key := range []string{"table", "right"} {
		raw, ok := m[key]
		if !ok {
			continue
		}
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			continue // already an inline table object
		}
		tbl, ok := tables[name]
		if !ok {
			t.Fatalf("case %s: unknown table %q", c.ID, name)
		}
		m[key] = tbl
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("case %s: re-marshal args: %v", c.ID, err)
	}
	return out
}

func loadCases(t *testing.T) ([]parityCase, string) {
	t.Helper()
	paths, err := filepath.Glob("cases/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no case files under cases/: %v", err)
	}
	sort.Strings(paths)
	tables := loadTables(t)
	var all []parityCase
	upstream := ""
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
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q but %q was already pinned", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			c.Args = inlineTables(t, c, tables)
			all = append(all, c)
		}
	}
	return all, upstream
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

type runner struct {
	name string
	cmd  *exec.Cmd
	in   *bufio.Writer
	out  chan string
	errc chan error
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin: %v", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout: %v", name, err)
	}
	cmd.Stderr = os.Stderr // runners log to stderr only
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{
		name: name,
		cmd:  cmd,
		in:   bufio.NewWriter(stdin),
		out:  make(chan string, 8),
		errc: make(chan error, 1),
	}
	go func() {
		defer close(r.out)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<25)
		for sc.Scan() {
			r.out <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errc <- err
		}
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	return r
}

// ask sends one request and returns the single reply, or an error on timeout.
func (r *runner) ask(req request) (reply, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.in.Write(append(b, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	if err := r.in.Flush(); err != nil {
		return reply{}, fmt.Errorf("%s: flush: %w", r.name, err)
	}
	select {
	case line, ok := <-r.out:
		if !ok {
			return reply{}, fmt.Errorf("%s: runner exited before answering %s", r.name, req.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{}, fmt.Errorf("%s: bad reply for %s: %v (%.200s)", r.name, req.ID, err, line)
		}
		if rep.ID != req.ID {
			return reply{}, fmt.Errorf("%s: reply out of order: want %s got %s", r.name, req.ID, rep.ID)
		}
		return rep, nil
	case <-time.After(caseTimeout):
		return reply{}, fmt.Errorf("%s: timeout after %s waiting for %s", r.name, caseTimeout, req.ID)
	}
}

// ---------------------------------------------------------------------------
// comparison
// ---------------------------------------------------------------------------

// equal is a normalising deep-equal: all JSON numbers are float64 and compared
// with floatTol, maps compare key-by-key, arrays element-by-element.
func equal(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		return floatsEqual(x, y)
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equal(v, w) {
				return false
			}
		}
		return true
	}
	return false
}

func floatsEqual(a, b float64) bool {
	if a == b {
		return true
	}
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return false
	}
	d := math.Abs(a - b)
	if d <= floatTol {
		return true
	}
	m := math.Max(math.Abs(a), math.Abs(b))
	return m > 0 && d/m <= floatTol
}

func decode(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func brief(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 900 {
		return s[:900] + "…"
	}
	return s
}

// ---------------------------------------------------------------------------
// score file
// ---------------------------------------------------------------------------

type scoreEntry struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Status   string `json:"status"`
	Upstream string `json:"upstreamFn,omitempty"`
	Go       string `json:"goFn,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type score struct {
	Library        string       `json:"library"`
	Upstream       string       `json:"upstream"`
	GoModule       string       `json:"goModule"`
	FloatTolerance float64      `json:"floatTolerance"`
	NASentinel     string       `json:"naSentinel"`
	Generated      string       `json:"generatedBy"`
	Cases          int          `json:"cases"`
	Match          int          `json:"match"`
	Differ         int          `json:"differ"`
	Deviations     int          `json:"deviations"`
	ParityPercent  float64      `json:"parityPercent"`
	Entries        []scoreEntry `json:"caseResults"`
}

func goModuleVersion() string {
	out, err := exec.Command("go", "list", "-m", "github.com/malcolmston/pandas").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// the test
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found in PATH; skipping pandas parity")
	}
	// Prefer a venv inside python/ if one exists, else the system python3.
	if venv := filepath.Join("python", ".venv", "bin", "python3"); fileExists(venv) {
		abs, err := filepath.Abs(venv)
		if err == nil {
			py = abs
		}
	}
	if out, err := exec.Command(py, "-c", "import pandas").CombinedOutput(); err != nil {
		t.Skipf("pandas not importable by %s: %v (%s)", py, err, strings.TrimSpace(string(out)))
	}
	verOut, err := exec.Command(py, "python/run.py", "--version").Output()
	if err != nil {
		t.Skipf("cannot query pandas version: %v", err)
	}
	pyVersion := "pandas==" + strings.TrimSpace(string(verOut))

	cases, pinned := loadCases(t)
	if pyVersion != pinned {
		t.Logf("WARNING: cases pin %s but the installed oracle is %s; the score below is against the installed version", pinned, pyVersion)
	}

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build go runner: %v", err)
	}

	upstream := startRunner(t, "python", exec.Command(py, "python/run.py"))
	goRun := startRunner(t, "go", exec.Command(bin))

	sc := score{
		Library:        "pandas",
		Upstream:       pyVersion,
		GoModule:       goModuleVersion(),
		FloatTolerance: floatTol,
		NASentinel:     "__NA__",
		Generated:      "GOWORK=off go test ./parity/pandas/",
		Cases:          len(cases),
	}

	for _, c := range cases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			req := request{ID: c.ID, Fn: c.Fn, Args: c.Args}
			up, upErr := upstream.ask(req)
			gr, goErr := goRun.ask(req)

			entry := scoreEntry{ID: c.ID, Group: c.group, Upstream: c.UpstreamFn, Go: c.GoFn}
			record := func(status, detail string) {
				entry.Status = status
				entry.Detail = detail
				sc.Entries = append(sc.Entries, entry)
			}

			if upErr != nil || goErr != nil {
				sc.Differ++
				record("error", fmt.Sprintf("transport: up=%v go=%v", upErr, goErr))
				t.Fatalf("runner transport failure: upstream=%v go=%v", upErr, goErr)
			}

			mismatch := ""
			switch {
			case up.OK != gr.OK:
				mismatch = fmt.Sprintf("ok differs: upstream ok=%v (%s) but go ok=%v (%s)",
					up.OK, oneLine(up.Error), gr.OK, oneLine(gr.Error))
			case !up.OK && !gr.OK:
				// both failed: parity on failure. Message text is not compared.
			default:
				if !equal(decode(up.Value), decode(gr.Value)) {
					mismatch = fmt.Sprintf("value differs:\n  upstream: %s\n        go: %s",
						brief(up.Value), brief(gr.Value))
				}
			}

			if mismatch == "" {
				sc.Match++
				record("match", "")
				return
			}
			if c.Deviation != "" {
				sc.Deviations++
				record("deviation", c.Deviation+" | "+mismatch)
				t.Skipf("known deviation (%s): %s", c.Deviation, mismatch)
				return
			}
			sc.Differ++
			record("differs", mismatch)
			t.Errorf("%s [%s -> %s]: %s", c.ID, c.UpstreamFn, c.GoFn, mismatch)
			if c.Note != "" {
				t.Logf("note: %s", c.Note)
			}
		})
	}

	denom := sc.Match + sc.Differ
	if denom > 0 {
		sc.ParityPercent = math.Round(float64(sc.Match)/float64(denom)*10000) / 100
	}
	sort.Slice(sc.Entries, func(i, j int) bool { return sc.Entries[i].ID < sc.Entries[j].ID })
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("marshal parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("pandas parity: %d/%d = %.2f%% (%d deviations excluded) upstream=%s go=%s tol=%g",
		sc.Match, denom, sc.ParityPercent, sc.Deviations, sc.Upstream, sc.GoModule, floatTol)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
