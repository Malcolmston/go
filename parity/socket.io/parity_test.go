// Package parity drives the upstream (Node/socket.io) runner and the Go
// (github.com/malcolmston/socketio) runner over an identical set of cases and
// compares their answers. See ../HARNESS.md.
//
// Three phases, all scored into parity.json:
//
//   - wire-encode / wire-decode / eio-frame: deterministic, socket-free codec
//     comparison — the encoded bytes of a Socket.IO packet and the decoded
//     structure of a corpus of literal wire strings.
//   - behaviour: each runner starts its own server and connects with its own
//     client, and the ordered transcripts are compared.
//   - interop: cross-implementation pairings (upstream client against the Go
//     server, Go client against the upstream server, and both ported) compared
//     against the all-upstream pairing.
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

const caseTimeout = 30 * time.Second

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
	Server     string            `json:"server"`
	Client     string            `json:"client"`

	group string
}

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Note     string     `json:"note"`
	Cases    []caseSpec `json:"cases"`
}

// loadCases reads every cases/*.json file. Groups may pin different upstream
// packages (socket.io-parser, engine.io-parser, socket.io itself), so the pins
// are collected rather than required to be identical.
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
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   chan string
	readErr chan error
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
	cmd.Stderr = &prefixWriter{prefix: name + ": "}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, stdin: stdin, lines: make(chan string, 8), readErr: make(chan error, 1)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		r.readErr <- sc.Err()
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

func (r *runner) ask(c caseSpec) (*reply, error) {
	return r.call(c.ID, c.Fn, c.Args)
}

// call sends one request and waits for exactly one reply, bounded by
// caseTimeout so a hung runner fails its own case rather than the suite.
func (r *runner) call(id, fn string, args []json.RawMessage) (*reply, error) {
	req := map[string]any{"id": id, "fn": fn}
	if args != nil {
		req["args"] = args
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
			return nil, fmt.Errorf("%s: runner exited before answering %s", r.name, id)
		}
		var rep reply
		if err := json.Unmarshal([]byte(l), &rep); err != nil {
			return nil, fmt.Errorf("%s: bad reply %q: %w", r.name, l, err)
		}
		if rep.ID != id {
			return nil, fmt.Errorf("%s: reply out of order: want %s got %s", r.name, id, rep.ID)
		}
		return &rep, nil
	case <-time.After(caseTimeout):
		return nil, fmt.Errorf("%s: timeout after %s waiting for %s", r.name, caseTimeout, id)
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
// that ints and floats (1 vs 1.0) compare equal — encoding/json makes every
// number a float64.
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

type score struct {
	Library    string         `json:"library"`
	Upstream   string         `json:"upstream"`
	GoModule   string         `json:"goModule"`
	GoVersion  string         `json:"goVersion"`
	Generated  string         `json:"generatedBy"`
	Total      int            `json:"total"`
	Match      int            `json:"match"`
	Mismatch   int            `json:"mismatch"`
	Deviations int            `json:"deviations"`
	Percent    float64        `json:"parityPercent"`
	Groups     map[string]*gp `json:"groups"`
	Failing    []string       `json:"failingCases"`
}

type gp struct {
	Total    int `json:"total"`
	Match    int `json:"match"`
	Mismatch int `json:"mismatch"`
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/socketio")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// the harness
// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping socket.io parity (Go-only checkout)")
	}
	for _, dep := range []string{"socket.io", "socket.io-client", "socket.io-parser", "engine.io-parser"} {
		if _, err := os.Stat(filepath.Join("node", "node_modules", dep, "package.json")); err != nil {
			t.Skipf("upstream %s not installed; run `npm install` in parity/socket.io/node", dep)
		}
	}

	// Build the Go runner once.
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
		Upstream:  upstream,
		GoModule:  goModuleVersion(t),
		GoVersion: strings.TrimSpace(runOut("go", "version")),
		Generated: "go test ./parity/socket.io/",
		Groups:    map[string]*gp{},
	}

	record := func(c caseSpec, ok bool) {
		g := sc.Groups[c.group]
		if g == nil {
			g = &gp{}
			sc.Groups[c.group] = g
		}
		sc.Total++
		g.Total++
		if c.Deviation != "" {
			sc.Deviations++
		}
		if ok {
			sc.Match++
			g.Match++
		} else {
			sc.Mismatch++
			g.Mismatch++
			sc.Failing = append(sc.Failing, c.ID)
		}
	}

	// ---- phases 1 and 2: identical cases through both runners.
	var interop []caseSpec
	for _, c := range cases {
		if c.group == "interop" {
			interop = append(interop, c)
			continue
		}
		// Runners are stateful subprocesses shared by every subtest, so cases
		// must stay sequential: no t.Parallel here.
		upRep, upErr := up.ask(c)
		goRep, goErr := gorun.ask(c)

		ok := true
		t.Run(c.ID, func(t *testing.T) {
			if upErr != nil {
				ok = false
				t.Fatalf("upstream runner failure: %v", upErr)
			}
			if goErr != nil {
				ok = false
				t.Fatalf("go runner failure: %v", goErr)
			}
			if upRep.OK != goRep.OK {
				ok = false
				t.Fatalf("ok mismatch: upstream ok=%v (%s), go ok=%v (%s)",
					upRep.OK, upRep.Error, goRep.OK, goRep.Error)
			}
			if !upRep.OK {
				// Both failed: that is parity. Message text is not compared.
				return
			}
			uv, gv := decode(upRep.Value), decode(goRep.Value)
			if !reflect.DeepEqual(uv, gv) {
				ok = false
				label := ""
				if c.Deviation != "" {
					label = " (declared deviation: " + c.Deviation + ")"
				}
				t.Errorf("value mismatch%s\n  upstream: %s\n        go: %s", label, pretty(uv), pretty(gv))
			}
		})
		record(c, ok)
	}

	// ---- phase 3: cross-implementation interop.
	if len(interop) > 0 {
		pick := func(which string) *runner {
			if which == "go" {
				return gorun
			}
			return up
		}
		// The all-upstream pairing is the oracle.
		oracle, oracleErr := runInterop(pick("node"), pick("node"), "oracle", "basic")
		for _, c := range interop {
			script := strings.TrimPrefix(c.Fn, "interop.")
			got, gotErr := runInterop(pick(c.Server), pick(c.Client), c.ID, script)
			ok := true
			t.Run(c.ID, func(t *testing.T) {
				if oracleErr != nil {
					ok = false
					t.Fatalf("upstream/upstream oracle pairing failed: %v", oracleErr)
				}
				if gotErr != nil {
					ok = false
					t.Fatalf("pairing server=%s client=%s failed: %v", c.Server, c.Client, gotErr)
				}
				if !reflect.DeepEqual(oracle, got) {
					ok = false
					t.Errorf("interop transcript mismatch (server=%s client=%s)\n  upstream/upstream: %s\n        this pairing: %s",
						c.Server, c.Client, pretty(oracle), pretty(got))
				}
			})
			record(c, ok)
		}
	}

	if sc.Total > 0 {
		sc.Percent = float64(sc.Match) * 100 / float64(sc.Total)
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("socket.io parity: %d/%d cases match (%.1f%%), %d mismatches, %d declared deviations",
		sc.Match, sc.Total, sc.Percent, sc.Mismatch, sc.Deviations)
}

// runInterop starts the scripted server on srv, drives it with a client from
// cli, then shuts the server down and returns both transcripts. The server is
// always closed, so a failure cannot leak a listener into the next pairing.
func runInterop(srv, cli *runner, id, script string) (any, error) {
	rep, err := srv.call(id+"/serve", "serve."+script, nil)
	if err != nil {
		return nil, err
	}
	if !rep.OK {
		return nil, fmt.Errorf("serve.%s on %s: %s", script, srv.name, rep.Error)
	}
	var served struct{ URL string `json:"url"` }
	if err := json.Unmarshal(rep.Value, &served); err != nil {
		return nil, fmt.Errorf("serve.%s on %s: bad reply: %w", script, srv.name, err)
	}

	urlArg, _ := json.Marshal(served.URL)
	driveRep, driveErr := cli.call(id+"/drive", "drive."+script, []json.RawMessage{urlArg})

	closeRep, closeErr := srv.call(id+"/close", "serve.close", nil)

	if driveErr != nil {
		return nil, driveErr
	}
	if !driveRep.OK {
		return nil, fmt.Errorf("drive.%s on %s: %s", script, cli.name, driveRep.Error)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if !closeRep.OK {
		return nil, fmt.Errorf("serve.close on %s: %s", srv.name, closeRep.Error)
	}

	var driven struct {
		Transcript []string `json:"transcript"`
	}
	if err := json.Unmarshal(driveRep.Value, &driven); err != nil {
		return nil, err
	}
	var closed struct {
		Transcript []string `json:"transcript"`
	}
	if err := json.Unmarshal(closeRep.Value, &closed); err != nil {
		return nil, err
	}
	return map[string]any{"server": closed.Transcript, "client": driven.Transcript}, nil
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
