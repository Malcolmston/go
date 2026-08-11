// Package parity scores github.com/malcolmston/socketio/client against the npm
// package it ports, socket.io-client, by running both over an identical set of
// cases and comparing their answers. See ../../../HARNESS.md.
//
// This is a NESTED harness: parity/socket.io/ scores the socket.io SERVER and
// uses this client only as a driver. Everything here is deterministic and
// socket-free: the reconnection schedule (upstream's bundled backo2), endpoint
// addressing (url() + Transport.uri()), Manager option defaults, and the packets
// an emit builds — all with autoConnect off, so no connection is ever opened.
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
	"sort"
	"strings"
	"testing"
	"time"
)

// caseTimeout bounds one case, so a hung runner fails that case and not the
// whole suite. Every operation here is a pure function call.
const caseTimeout = 10 * time.Second

// ---------------------------------------------------------------------------
// case files
// ---------------------------------------------------------------------------

type caseSpec struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []caseSpec `json:"cases"`
}

// loadCases reads every cases/*.json file, rejecting duplicate ids and files
// that do not pin an upstream version.
func loadCases(t *testing.T) (cases []caseSpec, upstream string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no case files in cases/")
	}
	seen := map[string]string{}
	var pins []string
	pinSeen := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var cf caseFile
		if err := json.Unmarshal(b, &cf); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		if cf.Upstream == "" {
			t.Fatalf("%s does not pin an upstream version", f)
		}
		if !pinSeen[cf.Upstream] {
			pinSeen[cf.Upstream] = true
			pins = append(pins, cf.Upstream)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, f)
			}
			seen[c.ID] = f
			c.group = cf.Group
			cases = append(cases, c)
		}
	}
	sort.Strings(pins)
	return cases, strings.Join(pins, "; ")
}

// ---------------------------------------------------------------------------
// runners
// ---------------------------------------------------------------------------

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
}

// startRunner launches one long-lived subprocess. Both runners are started
// exactly once and every case is streamed through them.
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
	cmd.Stderr = &prefixWriter{prefix: name + ": "}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 8)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
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
	})
	return r
}

// ask sends one case and waits for exactly one reply, bounded by caseTimeout.
func (r *runner) ask(c caseSpec) (*reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn}
	if c.Args != nil {
		req["args"] = c.Args
	}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := r.stdin.Write(append(line, '\n')); err != nil {
		return nil, fmt.Errorf("%s: write: %w", r.name, err)
	}
	select {
	case l, ok := <-r.lines:
		if !ok {
			return nil, fmt.Errorf("%s: runner exited before answering %s", r.name, c.ID)
		}
		var rep reply
		if err := json.Unmarshal([]byte(l), &rep); err != nil {
			return nil, fmt.Errorf("%s: bad reply %q: %w", r.name, l, err)
		}
		if rep.ID != c.ID {
			return nil, fmt.Errorf("%s: reply out of order: want %s got %s", r.name, c.ID, rep.ID)
		}
		return &rep, nil
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("%s: timeout after %s waiting for %s", r.name, caseTimeout, c.ID)
	}
}

type prefixWriter struct{ prefix string }

func (w *prefixWriter) Write(p []byte) (int, error) {
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if l != "" {
			fmt.Fprintln(os.Stderr, w.prefix+l)
		}
	}
	return len(p), nil
}

// ---------------------------------------------------------------------------
// comparison
// ---------------------------------------------------------------------------

// decode turns a raw JSON value into the generic form used for comparison, so
// that ints and floats (1 vs 1.0) compare equal: encoding/json makes every
// number a float64 on both sides.
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

func pretty(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// score
// ---------------------------------------------------------------------------

// score is the shape of parity.json. Compared is the number of cases whose
// answers were required to be identical — the total minus the declared
// deviations, which are expected to differ and are verified to still do so.
// Percent is Match over Compared.
type score struct {
	Library    string         `json:"library"`
	Package    string         `json:"package"`
	Upstream   string         `json:"upstream"`
	GoModule   string         `json:"goModule"`
	GoVersion  string         `json:"goVersion"`
	Generated  string         `json:"generatedBy"`
	Total      int            `json:"total"`
	Compared   int            `json:"compared"`
	Match      int            `json:"match"`
	Mismatch   int            `json:"mismatch"`
	Deviations int            `json:"deviations"`
	Percent    float64        `json:"parityPercent"`
	Groups     map[string]*gp `json:"groups"`
	Failing    []string       `json:"failingCases"`
	Deviating  []string       `json:"deviationCases"`
}

type gp struct {
	Total     int `json:"total"`
	Match     int `json:"match"`
	Mismatch  int `json:"mismatch"`
	Deviation int `json:"deviations"`
}

func goModuleVersion() string {
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/socketio")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func runOut(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping client parity (Go-only checkout)")
	}
	for _, dep := range []string{"socket.io-client", "socket.io-parser", "engine.io-client"} {
		if _, err := os.Stat(filepath.Join("node", "node_modules", dep, "package.json")); err != nil {
			t.Skipf("upstream %s not installed; run `npm install` in parity/socket.io/nested/client/node", dep)
		}
	}

	// Build the Go runner once. GOWORK=off: this harness is its own module,
	// deliberately outside the aggregator workspace.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}

	cases, upstream := loadCases(t)

	upCmd := exec.Command(node, "run.mjs")
	upCmd.Dir = "node"
	up := startRunner(t, "node", upCmd)

	goCmd := exec.Command(bin)
	goCmd.Env = append(os.Environ(), "GOWORK=off")
	gorun := startRunner(t, "go", goCmd)

	sc := &score{
		Library:   "socket.io",
		Package:   "client",
		Upstream:  upstream,
		GoModule:  goModuleVersion(),
		GoVersion: strings.TrimSpace(runOut("go", "version")),
		Generated: "GOWORK=off go test ./parity/socket.io/nested/client/",
		Groups:    map[string]*gp{},
	}

	// outcome of one case: identical answers, a divergence, or a divergence that
	// the case file declares and explains.
	const (
		same = iota
		differs
		declared
	)

	record := func(c caseSpec, outcome int) {
		g := sc.Groups[c.group]
		if g == nil {
			g = &gp{}
			sc.Groups[c.group] = g
		}
		sc.Total++
		g.Total++
		switch outcome {
		case same:
			sc.Compared++
			sc.Match++
			g.Match++
		case differs:
			sc.Compared++
			sc.Mismatch++
			g.Mismatch++
			sc.Failing = append(sc.Failing, c.ID)
		case declared:
			sc.Deviations++
			g.Deviation++
			sc.Deviating = append(sc.Deviating, c.ID)
		}
	}

	for _, c := range cases {
		// The runners are stateful subprocesses shared by every subtest, so the
		// cases must stay sequential: no t.Parallel here.
		upRep, upErr := up.ask(c)
		goRep, goErr := gorun.ask(c)

		outcome := same
		t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				outcome = differs
				t.Fatalf("upstream runner failure: %v", upErr)
			}
			if goErr != nil {
				outcome = differs
				t.Fatalf("go runner failure: %v", goErr)
			}

			// A case marked "deviation" is expected to diverge, and the
			// divergence is documented in socket.io/API-DEVIATIONS.md. It is
			// reported, not failed — but a deviation that has stopped diverging
			// is a stale claim, and that does fail.
			diverged := upRep.OK != goRep.OK
			var uv, gv any
			if !diverged && upRep.OK {
				uv, gv = decode(upRep.Value), decode(goRep.Value)
				diverged = !reflect.DeepEqual(uv, gv)
			}
			if c.Deviation != "" {
				if !diverged {
					outcome = differs
					t.Errorf("declared deviation no longer diverges — the deviation is stale and should be removed: %s", c.Deviation)
					return
				}
				outcome = declared
				t.Logf("declared deviation holds: %s\n  upstream: ok=%v %s%s\n        go: ok=%v %s%s",
					c.Deviation, upRep.OK, upRep.Error, pretty(uv), goRep.OK, goRep.Error, pretty(gv))
				return
			}
			if upRep.OK != goRep.OK {
				outcome = differs
				t.Fatalf("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
					upRep.OK, upRep.Error, goRep.OK, goRep.Error)
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is not compared.
				return
			}
			if diverged {
				outcome = differs
				t.Errorf("value mismatch\n  upstream: %s\n        go: %s", pretty(uv), pretty(gv))
			}
		})
		record(c, outcome)
	}

	if sc.Compared > 0 {
		sc.Percent = float64(sc.Match) * 100 / float64(sc.Compared)
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("socket.io/client parity: %d/%d compared cases match (%.1f%%), %d mismatches, %d declared deviations, %d cases total",
		sc.Match, sc.Compared, sc.Percent, sc.Mismatch, sc.Deviations, sc.Total)
}
