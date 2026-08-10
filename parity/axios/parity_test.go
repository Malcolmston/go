// Package parity drives the upstream (node/axios) and Go (github.com/malcolmston/axios)
// runners over the same case files and compares their answers.
//
// axios is an HTTP client, so the harness starts exactly one local echo/fixture
// server on a random loopback port and hands its base URL to both runners via
// argv and PARITY_BASE_URL. No request ever leaves the machine.
package parity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const caseTimeout = 25 * time.Second

// ------------------------------------------------------------- case files ---

type caseFile struct {
	Group    string  `json:"group"`
	Upstream string  `json:"upstream"`
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
	group      string
}

func loadCases(t *testing.T) []tcase {
	t.Helper()
	paths, err := filepath.Glob("cases/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no case files found in cases/: %v", err)
	}
	sort.Strings(paths)
	var all []tcase
	seen := map[string]bool{}
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
			if seen[c.ID] {
				t.Fatalf("duplicate case id %q in %s", c.ID, p)
			}
			seen[c.ID] = true
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ------------------------------------------------------------ echo server ---

func startEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	observe := func(r *http.Request) map[string]any {
		body, _ := io.ReadAll(r.Body)
		hs := map[string]any{}
		for k, v := range r.Header {
			hs[strings.ToLower(k)] = strings.Join(v, ", ")
		}
		pairs := [][2]string{}
		// Preserve wire order of query pairs.
		if q := r.URL.RawQuery; q != "" {
			for _, kv := range strings.Split(q, "&") {
				k, v, _ := strings.Cut(kv, "=")
				dk, err1 := urlUnescape(k)
				dv, err2 := urlUnescape(v)
				if err1 != nil || err2 != nil {
					dk, dv = k, v
				}
				pairs = append(pairs, [2]string{dk, dv})
			}
		}
		return map[string]any{
			"method":   r.Method,
			"path":     r.URL.Path,
			"rawQuery": r.URL.RawQuery,
			"query":    pairs,
			"headers":  hs,
			"body":     string(body),
		}
	}

	writeJSON := func(w http.ResponseWriter, code int, v any) {
		b, _ := json.Marshal(v)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Fixture", "echo")
		w.WriteHeader(code)
		_, _ = w.Write(b)
	}

	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, observe(r))
	})
	mux.HandleFunc("/echo/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, observe(r))
	})
	// /status/{code} — echoes the request with the requested status code.
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/status/"))
		if err != nil || code < 100 || code > 599 {
			code = 400
		}
		if code == 204 || code == 304 {
			w.Header().Set("X-Fixture", "echo")
			w.WriteHeader(code)
			return
		}
		// Deliberately does NOT echo the request: several cases stringify this
		// body (transformResponse), which would drag volatile headers into a
		// string the normaliser cannot reach into.
		_, _ = io.ReadAll(r.Body)
		writeJSON(w, code, map[string]any{"code": code})
	})
	// /redirect/{n} — 302 chain of n hops ending at /echo.
	mux.HandleFunc("/redirect/", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/redirect/"))
		if err != nil {
			n = 1
		}
		next := "/echo"
		if n > 1 {
			next = fmt.Sprintf("/redirect/%d", n-1)
		}
		w.Header().Set("Location", next)
		w.Header().Set("X-Fixture", "echo")
		w.WriteHeader(302)
	})
	// /slow/{ms} — sleeps before replying, for timeout cases.
	mux.HandleFunc("/slow/", func(w http.ResponseWriter, r *http.Request) {
		ms, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/slow/"))
		if err != nil {
			ms = 100
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		writeJSON(w, 200, map[string]any{"slept": ms})
	})
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Fixture", "echo")
		_, _ = w.Write([]byte("hello text"))
	})
	// application/json content type, body is not valid JSON.
	mux.HandleFunc("/badjson", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Fixture", "echo")
		_, _ = w.Write([]byte("{not json"))
	})
	// No content type at all, body IS valid JSON.
	mux.HandleFunc("/nocontenttype", func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.Header().Set("X-Fixture", "echo")
		_, _ = w.Write([]byte(`{"a":1}`))
	})
	// A fixed-size body, for maxContentLength.
	mux.HandleFunc("/bytes/", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/bytes/"))
		if err != nil || n < 0 || n > 1<<20 {
			n = 16
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Fixture", "echo")
		_, _ = w.Write([]byte(strings.Repeat("x", n)))
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &httptest.Server{Listener: l, Config: &http.Server{Handler: mux}}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// urlUnescape decodes a query component with '+' meaning space.
func urlUnescape(s string) (string, error) { return url.QueryUnescape(s) }

// ---------------------------------------------------------------- runners ---

type runner struct {
	name string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	dead bool
}

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
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
		t.Fatalf("%s start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, in: stdin, out: bufio.NewReaderSize(stdout, 1<<20)}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return r
}

func (r *runner) call(c tcase) reply {
	if r.dead {
		return reply{ID: c.ID, OK: false, Error: "runner dead"}
	}
	payload, _ := json.Marshal(map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args})
	if _, err := r.in.Write(append(payload, '\n')); err != nil {
		r.dead = true
		return reply{ID: c.ID, OK: false, Error: "write: " + err.Error()}
	}
	type res struct {
		line string
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		line, err := r.out.ReadString('\n')
		ch <- res{line, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil && strings.TrimSpace(got.line) == "" {
			r.dead = true
			return reply{ID: c.ID, OK: false, Error: "read: " + got.err.Error()}
		}
		var rep reply
		if err := json.Unmarshal([]byte(got.line), &rep); err != nil {
			r.dead = true
			return reply{ID: c.ID, OK: false, Error: "bad reply line: " + err.Error()}
		}
		return rep
	case <-time.After(caseTimeout):
		r.dead = true
		return reply{ID: c.ID, OK: false, Error: "case timeout after " + caseTimeout.String()}
	}
}

// ----------------------------------------------------------- normalisation ---

// Request headers both clients inject that are volatile or transport-level.
// They are stripped from the generic comparison and asserted explicitly in the
// headers-default-* cases via "observeHeaders" (which bypasses stripping).
var volatileRequestHeaders = map[string]bool{
	"host":              true, // random port
	"connection":        true, // transport
	"keep-alive":        true, // transport
	"transfer-encoding": true, // transport / chunking strategy
	"content-length":    true, // differs where boundaries/encodings legitimately differ
	"user-agent":        true, // "axios/1.7.9" vs "Go-http-client/1.1"
	"accept-encoding":   true, // negotiated codec list differs by stdlib
	"accept":            true, // axios sets a default, the port does not
	"referer":           true, // net/http sets Referer when following a redirect; follow-redirects does not
}

var boundaryRe = regexp.MustCompile(`boundary=([^;\s"]+)`)

// normalise rewrites both sides identically before deep-equal.
func normalise(v any) any {
	// Collect multipart boundary tokens so bodies compare.
	var boundaries []string
	collect(v, &boundaries)
	out := walk(v)
	if len(boundaries) > 0 {
		out = replaceStrings(out, boundaries)
	}
	return out
}

func collect(v any, acc *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for _, vv := range x {
			collect(vv, acc)
		}
	case []any:
		for _, vv := range x {
			collect(vv, acc)
		}
	case string:
		if m := boundaryRe.FindStringSubmatch(x); m != nil {
			*acc = append(*acc, strings.Trim(m[1], `"`))
		}
	}
}

func walk(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := make(map[string]any, len(x))
		for k, vv := range x {
			if k == "headers" {
				if hm, ok := vv.(map[string]any); ok {
					nh := make(map[string]any, len(hm))
					for hk, hv := range hm {
						if volatileRequestHeaders[strings.ToLower(hk)] {
							continue
						}
						nh[strings.ToLower(hk)] = hv
					}
					o[k] = nh
					continue
				}
			}
			o[k] = walk(vv)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, vv := range x {
			o[i] = walk(vv)
		}
		return o
	default:
		return v
	}
}

func replaceStrings(v any, boundaries []string) any {
	switch x := v.(type) {
	case map[string]any:
		o := make(map[string]any, len(x))
		for k, vv := range x {
			o[k] = replaceStrings(vv, boundaries)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i, vv := range x {
			o[i] = replaceStrings(vv, boundaries)
		}
		return o
	case string:
		s := x
		for _, b := range boundaries {
			s = strings.ReplaceAll(s, b, "BOUNDARY")
		}
		return s
	default:
		return v
	}
}

// deepEqual treats all JSON numbers as float64 and is order-insensitive for
// object keys (Go maps and JS objects both land in map[string]any).
func deepEqual(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && (x == y || (math.IsNaN(x) && math.IsNaN(y)))
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, ok := y[k]
			if !ok || !deepEqual(xv, yv) {
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
			if !deepEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}

func decodeNorm(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return normalise(v)
}

func pretty(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if len(b) > 1200 {
		return string(b[:1200]) + "…"
	}
	return string(b)
}

// --------------------------------------------------------------- the test ---

type score struct {
	Library     string         `json:"library"`
	Upstream    string         `json:"upstream"`
	GoModule    string         `json:"goModule"`
	GeneratedAt string         `json:"generatedAt"`
	Total       int            `json:"total"`
	Match       int            `json:"match"`
	Differs     int            `json:"differs"`
	Deviations  int            `json:"deviations"`
	Percent     string         `json:"casePercent"`
	ByGroup     map[string]any `json:"byGroup"`
	Failing     []string       `json:"failingCases"`
}

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity")
	}
	if _, err := os.Stat("node/node_modules/axios/package.json"); err != nil {
		t.Skip("upstream axios not installed: run `npm install` in parity/axios/node")
	}
	upstreamVersion := "unknown"
	if b, err := os.ReadFile("node/node_modules/axios/package.json"); err == nil {
		var pj struct{ Version string }
		if json.Unmarshal(b, &pj) == nil {
			upstreamVersion = "axios@" + pj.Version
		}
	}

	cases := loadCases(t)
	srv := startEchoServer(t)
	base := srv.URL
	t.Logf("echo server on %s; %d cases; upstream %s", base, len(cases), upstreamVersion)

	// Build the Go runner once.
	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building go runner: %v\n%s", err, out)
	}

	env := append(os.Environ(), "PARITY_BASE_URL="+base, "TZ=UTC", "NODE_OPTIONS=")

	nodeCmd := exec.Command(node, "run.js", base)
	nodeCmd.Dir = "node"
	nodeCmd.Env = env
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(bin, base)
	goCmd.Env = env
	gr := startRunner(t, "go", goCmd)

	var match, differs, deviations int
	byGroup := map[string]any{}
	groupTotals := map[string][2]int{}
	var failing []string

	for _, c := range cases {
		upRep := up.call(c)
		goRep := gr.call(c)

		upVal := decodeNorm(upRep.Value)
		goVal := decodeNorm(goRep.Value)

		ok := upRep.OK == goRep.OK
		if ok && upVal != nil && goVal != nil {
			ok = deepEqual(upVal, goVal)
		} else if ok && (upVal == nil) != (goVal == nil) && upRep.OK {
			ok = false
		}

		gt := groupTotals[c.group]
		gt[1]++
		if ok {
			match++
			gt[0]++
		} else if c.Deviation != "" {
			deviations++
			gt[0]++
		} else {
			differs++
			failing = append(failing, c.ID)
		}
		groupTotals[c.group] = gt

		cc, okk, uv, gv := c, ok, upVal, goVal
		t.Run(c.ID, func(t *testing.T) {
			if okk {
				return
			}
			if cc.Deviation != "" {
				t.Skipf("declared deviation: %s\n  upstream: ok=%v %s\n  go:       ok=%v %s",
					cc.Deviation, upRep.OK, pretty(uv), goRep.OK, pretty(gv))
				return
			}
			t.Errorf("parity mismatch (%s / %s)\n  upstream: ok=%v err=%q value=%s\n  go:       ok=%v err=%q value=%s\n  note: %s",
				cc.UpstreamFn, cc.GoFn, upRep.OK, upRep.Error, pretty(uv), goRep.OK, goRep.Error, pretty(gv), cc.Note)
		})
	}

	total := len(cases)
	for g, v := range groupTotals {
		byGroup[g] = map[string]any{"match": v[0], "total": v[1]}
	}
	pct := 0.0
	if total > 0 {
		pct = float64(match+deviations) / float64(total) * 100
	}
	sort.Strings(failing)
	s := score{
		Library:     "axios",
		Upstream:    upstreamVersion,
		GoModule:    goModuleVersion(),
		GeneratedAt: time.Now().UTC().Format("2006-01-02"),
		Total:       total, Match: match, Differs: differs, Deviations: deviations,
		Percent: fmt.Sprintf("%.1f%%", pct),
		ByGroup: byGroup, Failing: failing,
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Errorf("writing parity.json: %v", err)
	}
	t.Logf("cases: %d total, %d match, %d differ, %d declared deviations => %s",
		total, match, differs, deviations, s.Percent)
}

func goModuleVersion() string {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "github.com/malcolmston/axios") {
			f := strings.Fields(line)
			for i, w := range f {
				if w == "github.com/malcolmston/axios" && i+1 < len(f) {
					return w + "@" + f[i+1]
				}
			}
		}
	}
	return "unknown"
}
