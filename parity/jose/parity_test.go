// Package parity drives the upstream (panva/jose, node) and Go
// (github.com/malcolmston/jose) runners over the same cases and compares their
// answers.
//
// Two case shapes exist:
//
//   - kind "same": the identical request goes to both runners and the ok flag
//     and then the value are compared. Used for everything deterministic --
//     HMAC and PKCS1-v1_5 signatures, EdDSA, JWK import/export, thumbprints,
//     base64url -- and for every rejection case.
//
//   - kind "cross": the request is a *producer*. Its output is handed to the
//     *other* runner's consumer, in both directions, and the two consumed
//     results are compared. This is the only meaningful test for randomised
//     algorithms (RSA-OAEP, PSS, ECDSA, ECDH-ES, AES-GCM's random IV, PBES2's
//     random salt), where byte equality is impossible by construction.
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

const callTimeout = 30 * time.Second

// volatileHeader lists header parameters that carry fresh randomness or a
// per-implementation default, so a cross-language round-trip can never match on
// them. Everything else in the header is compared.
var volatileHeader = map[string]bool{
	"iv": true, "tag": true, "epk": true, "p2s": true, "p2c": true,
}

// ------------------------------------------------------------------ case files

type caseFile struct {
	Group    string     `json:"group"`
	Upstream string     `json:"upstream"`
	Cases    []testCase `json:"cases"`
}

type testCase struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // "same" (default) or "cross"

	Fn   string `json:"fn"`
	Args []any  `json:"args"`

	ConsumeFn   string   `json:"consumeFn"`
	ConsumeArgs []any    `json:"consumeArgs"`
	Directions  []string `json:"directions"` // subset of {"n2g","g2n"}

	MustFail bool           `json:"mustFail"`
	Expect   map[string]any `json:"expect"`

	UpstreamFn string `json:"upstreamFn"`
	GoFn       string `json:"goFn"`
	Note       string `json:"note"`
	Deviation  string `json:"deviation"`

	group string
}

// ---------------------------------------------------------------------- runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name string
	cmd  *exec.Cmd
	in   *bufio.Writer
	out  *bufio.Scanner
	mu   sync.Mutex
	dead error
	seq  int
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
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<25)
	r := &runner{name: name, cmd: cmd, in: bufio.NewWriter(stdin), out: sc}
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})
	return r
}

// call sends one request and reads the single reply. A per-call timeout keeps a
// hung runner from stalling the suite; once a call times out the stream is out
// of sync, so the runner is marked dead and every later call fails fast.
func (r *runner) call(fn string, args []any) (*reply, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dead != nil {
		return nil, r.dead
	}
	r.seq++
	id := fmt.Sprintf("q%d", r.seq)
	req := map[string]any{"id": id, "fn": fn, "args": args}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := r.in.Write(append(line, '\n')); err != nil {
		r.dead = err
		return nil, err
	}
	if err := r.in.Flush(); err != nil {
		r.dead = err
		return nil, err
	}

	type readResult struct {
		rep *reply
		err error
	}
	ch := make(chan readResult, 1)
	go func() {
		if !r.out.Scan() {
			err := r.out.Err()
			if err == nil {
				err = fmt.Errorf("runner closed its output")
			}
			ch <- readResult{nil, err}
			return
		}
		var rep reply
		if err := json.Unmarshal(r.out.Bytes(), &rep); err != nil {
			ch <- readResult{nil, fmt.Errorf("undecodable reply %q: %w", r.out.Text(), err)}
			return
		}
		ch <- readResult{&rep, nil}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			r.dead = res.err
			return nil, res.err
		}
		if res.rep.ID != id {
			r.dead = fmt.Errorf("reply out of order: want %s got %s", id, res.rep.ID)
			return nil, r.dead
		}
		return res.rep, nil
	case <-time.After(callTimeout):
		r.dead = fmt.Errorf("timed out after %s on %s", callTimeout, fn)
		return nil, r.dead
	}
}

// ------------------------------------------------------------------ comparison

// decodeValue turns a reply value into generic JSON types so that ints and
// floats compare equal (everything becomes float64).
func decodeValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// stripVolatile removes the randomised header parameters from a consumer's
// value so a cross-language round-trip can be compared.
func stripVolatile(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := map[string]any{}
	for k, val := range m {
		if k == "header" {
			h, ok := val.(map[string]any)
			if !ok {
				out[k] = val
				continue
			}
			clean := map[string]any{}
			for hk, hv := range h {
				if !volatileHeader[hk] {
					clean[hk] = hv
				}
			}
			out[k] = clean
			continue
		}
		out[k] = val
	}
	return out
}

func render(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(data)
}

// substitute replaces the "$OUT" placeholder anywhere in the consumer's
// arguments with the producer's output.
func substitute(args []any, out any) []any {
	res := make([]any, len(args))
	for i, a := range args {
		res[i] = substituteOne(a, out)
	}
	return res
}

func substituteOne(a any, out any) any {
	switch t := a.(type) {
	case string:
		if t == "$OUT" {
			return out
		}
		return t
	case []any:
		return substitute(t, out)
	case map[string]any:
		m := map[string]any{}
		for k, v := range t {
			m[k] = substituteOne(v, out)
		}
		return m
	default:
		return a
	}
}

// ---------------------------------------------------------------------- scores

type score struct {
	Library      string         `json:"library"`
	Upstream     string         `json:"upstream"`
	GoModule     string         `json:"goModule"`
	Total        int            `json:"total"`
	Passed       int            `json:"passed"`
	Failed       int            `json:"failed"`
	Deviations   int            `json:"deviations"`
	Parity       string         `json:"parity"`
	ByGroup      map[string]int `json:"casesByGroup"`
	FailedIDs    []string       `json:"failedCases"`
	DeviationIDs []string       `json:"deviationCases"`
	GeneratedBy  string         `json:"generatedBy"`
}

// ------------------------------------------------------------------------ test

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping cross-language parity run")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "jose", "package.json")); err != nil {
		t.Skip("parity/jose/node/node_modules/jose is missing; run `npm install` in parity/jose/node")
	}

	files, err := filepath.Glob(filepath.Join("cases", "*.json"))
	if err != nil {
		t.Fatalf("glob cases: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no case files under cases/")
	}
	sort.Strings(files)

	var cases []testCase
	upstream := ""
	byGroup := map[string]int{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var cf caseFile
		if err := json.Unmarshal(data, &cf); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		if upstream == "" {
			upstream = cf.Upstream
		} else if cf.Upstream != upstream {
			t.Fatalf("%s pins upstream %q, but %q was pinned earlier", f, cf.Upstream, upstream)
		}
		for _, c := range cf.Cases {
			c.group = cf.Group
			cases = append(cases, c)
		}
		byGroup[cf.Group] += len(cf.Cases)
	}

	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] {
			t.Fatalf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
	}

	keys, err := filepath.Abs("keys")
	if err != nil {
		t.Fatalf("keys dir: %v", err)
	}

	up := startRunner(t, "node", exec.Command(node, filepath.Join("node", "run.mjs"), keys))

	goBin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", goBin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build go runner: %v\n%s", err, out)
	}
	port := startRunner(t, "go", exec.Command(goBin, keys))

	var passed, failed, deviations int
	var failedIDs, deviationIDs []string
	var mu sync.Mutex

	for _, c := range cases {
		c := c
		ok := t.Run(c.ID, func(t *testing.T) {
			switch c.Kind {
			case "", "same":
				runSame(t, up, port, c)
			case "cross":
				runCross(t, up, port, c)
			default:
				t.Fatalf("unknown case kind %q", c.Kind)
			}
		})
		mu.Lock()
		if c.Deviation != "" {
			deviations++
			deviationIDs = append(deviationIDs, c.ID)
		}
		if ok {
			passed++
		} else {
			failed++
			failedIDs = append(failedIDs, c.ID)
		}
		mu.Unlock()
	}

	pct := 0.0
	if len(cases) > 0 {
		pct = 100 * float64(passed) / float64(len(cases))
	}
	s := score{
		Library:      "jose",
		Upstream:     upstream,
		GoModule:     goModuleVersion(t),
		Total:        len(cases),
		Passed:       passed,
		Failed:       failed,
		Deviations:   deviations,
		Parity:       fmt.Sprintf("%.1f%%", pct),
		ByGroup:      byGroup,
		FailedIDs:    failedIDs,
		DeviationIDs: deviationIDs,
		GeneratedBy:  "go test ./parity/jose/",
	}
	if s.FailedIDs == nil {
		s.FailedIDs = []string{}
	}
	if s.DeviationIDs == nil {
		s.DeviationIDs = []string{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal parity.json: %v", err)
	}
	if err := os.WriteFile("parity.json", append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("parity: %d/%d cases (%s), %d deviations", passed, len(cases), s.Parity, deviations)
}

// runSame feeds the identical request to both runners and compares.
func runSame(t *testing.T, up, port *runner, c testCase) {
	t.Helper()
	upRep, err := up.call(c.Fn, c.Args)
	if err != nil {
		t.Fatalf("upstream runner: %v", err)
	}
	goRep, err := port.call(c.Fn, c.Args)
	if err != nil {
		t.Fatalf("go runner: %v", err)
	}

	if c.MustFail {
		if upRep.OK {
			t.Errorf("case declares mustFail but upstream accepted it: %s", string(upRep.Value))
		}
		if goRep.OK {
			t.Errorf("SECURITY: the port ACCEPTS what upstream REJECTS.\n  upstream error: %s\n  go value:      %s",
				upRep.Error, string(goRep.Value))
		}
		return
	}

	if upRep.OK != goRep.OK {
		t.Errorf("ok differs: upstream ok=%v (%s), go ok=%v (%s)",
			upRep.OK, firstNonEmpty(upRep.Error, string(upRep.Value)),
			goRep.OK, firstNonEmpty(goRep.Error, string(goRep.Value)))
		return
	}
	if !upRep.OK {
		// Both failed the same way; message text is deliberately not compared.
		return
	}
	upVal, err := decodeValue(upRep.Value)
	if err != nil {
		t.Fatalf("decode upstream value: %v", err)
	}
	goVal, err := decodeValue(goRep.Value)
	if err != nil {
		t.Fatalf("decode go value: %v", err)
	}
	if !reflect.DeepEqual(upVal, goVal) {
		t.Errorf("value differs:\n  upstream: %s\n  go:       %s", render(upVal), render(goVal))
	}
}

// runCross produces with one runner and consumes with the other, in both
// directions, then compares the two consumed results.
func runCross(t *testing.T, up, port *runner, c testCase) {
	t.Helper()
	if c.ConsumeFn == "" {
		t.Fatalf("cross case %s has no consumeFn", c.ID)
	}
	dirs := c.Directions
	if len(dirs) == 0 {
		dirs = []string{"n2g", "g2n"}
	}

	results := map[string]any{}
	for _, d := range dirs {
		var producer, consumer *runner
		switch d {
		case "n2g":
			producer, consumer = up, port
		case "g2n":
			producer, consumer = port, up
		default:
			t.Fatalf("unknown direction %q", d)
		}
		ok := t.Run(d, func(t *testing.T) {
			prod, err := producer.call(c.Fn, c.Args)
			if err != nil {
				t.Fatalf("%s producer: %v", producer.name, err)
			}
			if c.MustFail {
				// A mustFail cross case asserts the *consumer* rejects it, so
				// the producer still has to succeed.
				if !prod.OK {
					t.Skipf("%s producer refused to build the input: %s", producer.name, prod.Error)
				}
			} else if !prod.OK {
				t.Fatalf("%s producer failed: %s", producer.name, prod.Error)
			}
			prodVal, err := decodeValue(prod.Value)
			if err != nil {
				t.Fatalf("decode producer value: %v", err)
			}
			out, ok := prodVal.(map[string]any)["out"]
			if !ok {
				t.Fatalf("%s producer returned no \"out\": %s", producer.name, render(prodVal))
			}

			cons, err := consumer.call(c.ConsumeFn, substitute(c.ConsumeArgs, out))
			if err != nil {
				t.Fatalf("%s consumer: %v", consumer.name, err)
			}
			if c.MustFail {
				if cons.OK {
					if consumer.name == "go" {
						t.Errorf("SECURITY: the port ACCEPTS what upstream REJECTS: %s", string(cons.Value))
					} else {
						t.Errorf("%s consumer accepted an input the case says must fail: %s", consumer.name, string(cons.Value))
					}
				}
				return
			}
			if !cons.OK {
				t.Fatalf("%s consumer rejected %s's output: %s", consumer.name, producer.name, cons.Error)
			}
			consVal, err := decodeValue(cons.Value)
			if err != nil {
				t.Fatalf("decode consumer value: %v", err)
			}
			consVal = stripVolatile(consVal)
			if c.Expect != nil {
				for k, want := range c.Expect {
					got := consVal.(map[string]any)[k]
					if !reflect.DeepEqual(normalizeJSON(want), got) {
						t.Errorf("%s consuming %s: %s = %s, want %s", consumer.name, producer.name, k, render(got), render(want))
					}
				}
			}
			results[d] = consVal
		})
		if !ok {
			return
		}
	}

	if len(dirs) == 2 {
		a, b := results["n2g"], results["g2n"]
		if !reflect.DeepEqual(a, b) {
			t.Errorf("round-trip results differ:\n  go consuming node:  %s\n  node consuming go:  %s", render(a), render(b))
		}
	}
}

// normalizeJSON re-encodes a value through JSON so that numbers coming from
// the case file and from a runner compare as the same type.
func normalizeJSON(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return v
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "github.com/malcolmston/jose") {
			f := strings.Fields(line)
			for i, tok := range f {
				if tok == "github.com/malcolmston/jose" && i+1 < len(f) {
					return "github.com/malcolmston/jose " + f[i+1]
				}
			}
		}
	}
	return "unknown"
}
