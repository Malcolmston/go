// Package cheerioparity drives the upstream (Node) cheerio@1.0.0 runner and the
// github.com/malcolmston/cheerio runner over identical cases and compares their
// answers. Upstream is the oracle; no expectation is hand-written.
package cheerioparity

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

// ---------------------------------------------------------------- case files

type caseSpec struct {
	ID         string `json:"id"`
	Fn         string `json:"fn"`
	Args       []any  `json:"args"`
	UpstreamFn string `json:"upstreamFn"`
	GoFn       string `json:"goFn"`
	Note       string `json:"note"`
	Deviation  string `json:"deviation"`
	group      string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []caseSpec `json:"cases"`
}

func loadCases(t *testing.T) ([]caseSpec, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no case files under cases/: %v", err)
	}
	sort.Strings(paths)
	var all []caseSpec
	upstream := ""
	seen := map[string]string{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(raw, &cf); err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q but %q was already seen", p, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			if other, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, other, p)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ---------------------------------------------------------------- runner

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error"`
}

type runner struct {
	name  string
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan string
	errs  chan error
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
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, in: stdin, lines: make(chan string, 8), errs: make(chan error, 1)}
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 1<<25)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		if err := sc.Err(); err != nil {
			r.errs <- err
		}
	}()
	return r
}

func (r *runner) ask(c caseSpec) (reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	b, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.in.Write(append(b, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case line, ok := <-r.lines:
		if !ok {
			return reply{}, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return reply{}, fmt.Errorf("%s: undecodable reply %q: %w", r.name, line, err)
		}
		if rep.ID != c.ID {
			return reply{}, fmt.Errorf("%s: out-of-order reply: want id %q got %q", r.name, c.ID, rep.ID)
		}
		return rep, nil
	case <-time.After(caseTimeout):
		return reply{}, fmt.Errorf("%s: timeout after %s on %s", r.name, caseTimeout, c.ID)
	}
}

func (r *runner) close() {
	_ = r.in.Close()
	_ = r.cmd.Wait()
}

// ---------------------------------------------------------------- comparison

// normalise turns every JSON number into a float64 and sorts nothing (both
// runners already emit deterministic order), so 1 and 1.0 compare equal.
func normalise(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = normalise(x[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalise(val)
		}
		return out
	default:
		return v
	}
}

func deepEqual(a, b any) bool {
	a, b = normalise(a), normalise(b)
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok {
			return af == bf || (math.IsNaN(af) && math.IsNaN(bf))
		}
		return false
	}
	return reflect.DeepEqual(a, b)
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(b)
	if len(s) > 900 {
		s = s[:900] + "…"
	}
	return s
}

// ---------------------------------------------------------------- score file

type scoreCase struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Status   string `json:"status"`
	Upstream any    `json:"upstream,omitempty"`
	Go       any    `json:"go,omitempty"`
	Note     string `json:"note,omitempty"`
}

type score struct {
	Library    string      `json:"library"`
	Upstream   string      `json:"upstream"`
	GoModule   string      `json:"goModule"`
	Generated  string      `json:"generatedBy"`
	Total      int         `json:"total"`
	Match      int         `json:"match"`
	Differ     int         `json:"differ"`
	Deviations int         `json:"deviations"`
	Percent    float64     `json:"parityPercent"`
	Groups     map[string]map[string]int `json:"groups"`
	Mismatches []scoreCase `json:"mismatches"`
}

func goModuleVersion() string {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Version}}", "github.com/malcolmston/cheerio").Output()
	if err != nil {
		return "github.com/malcolmston/cheerio (version unknown)"
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------- the harness

func TestParity(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity run")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "cheerio")); err != nil {
		t.Skip("node/node_modules/cheerio is missing; run `npm install` in parity/cheerio/node")
	}

	cases, upstream := loadCases(t)

	fixtures, err := filepath.Abs(filepath.Join("fixtures", "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Build the Go runner once. GOWORK=off keeps the published module version in
	// go.mod authoritative instead of the repo's workspace checkout.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go/")
	build.Env = append(os.Environ(), "GOWORK=off")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("building the Go runner failed: %v", err)
	}

	up := startRunner(t, "node", exec.Command(nodeBin, filepath.Join("node", "run.js"), fixtures))
	defer up.close()
	gorun := startRunner(t, "go", exec.Command(bin, fixtures))
	defer gorun.close()

	sc := score{
		Library:   "cheerio",
		Upstream:  upstream,
		GoModule:  goModuleVersion(),
		Generated: "go test ./parity/cheerio/",
		Groups:    map[string]map[string]int{},
	}

	bump := func(group, status string) {
		if sc.Groups[group] == nil {
			sc.Groups[group] = map[string]int{}
		}
		sc.Groups[group][status]++
	}

	for _, c := range cases {
		c := c
		upRep, upErr := up.ask(c)
		goRep, goErr := gorun.ask(c)
		sc.Total++

		t.Run(c.ID, func(t *testing.T) {
			if upErr != nil || goErr != nil {
				sc.Differ++
				bump(c.group, "differ")
				sc.Mismatches = append(sc.Mismatches, scoreCase{c.ID, c.group, "transport-error", fmt.Sprint(upErr), fmt.Sprint(goErr), c.Note})
				t.Fatalf("runner transport error: upstream=%v go=%v", upErr, goErr)
			}
			same := upRep.OK == goRep.OK && (!upRep.OK || deepEqual(upRep.Value, goRep.Value))
			switch {
			case same:
				sc.Match++
				bump(c.group, "match")
			case c.Deviation != "":
				sc.Deviations++
				bump(c.group, "deviation")
				t.Logf("known deviation (%s): upstream=%s go=%s", c.Deviation, replyString(upRep), replyString(goRep))
			default:
				sc.Differ++
				bump(c.group, "differ")
				sc.Mismatches = append(sc.Mismatches, scoreCase{
					ID: c.ID, Group: c.group, Status: "differs",
					Upstream: replyString(upRep), Go: replyString(goRep), Note: c.Note,
				})
				t.Errorf("divergence\n  upstream(%s): %s\n  go       (%s): %s\n  note: %s",
					c.UpstreamFn, replyString(upRep), c.GoFn, replyString(goRep), c.Note)
			}
		})
	}

	compared := sc.Match + sc.Differ
	if compared > 0 {
		sc.Percent = math.Round(float64(sc.Match)/float64(compared)*10000) / 100
	}
	sort.Slice(sc.Mismatches, func(i, j int) bool { return sc.Mismatches[i].ID < sc.Mismatches[j].ID })

	out, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("encoding parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(out, '\n'), 0o644); err != nil {
		t.Fatalf("writing parity.json: %v", err)
	}
	t.Logf("cases=%d match=%d differ=%d deviations=%d parity=%.2f%%", sc.Total, sc.Match, sc.Differ, sc.Deviations, sc.Percent)
}

func replyString(r reply) string {
	if !r.OK {
		return "ERROR: " + r.Error
	}
	return show(r.Value)
}
