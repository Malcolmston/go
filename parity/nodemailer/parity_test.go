// Package parity drives the upstream (Node) nodemailer runner and the Go
// nodemailer runner over the same cases and compares the MIME messages they
// generate.
//
// Nothing is sent over the network: both runners hand back the composed message
// (upstream via its stream transport, Go via Message.Build).
package parity

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

const (
	upstreamPin = "nodemailer@6.9.16"
	caseTimeout = 20 * time.Second
)

// ---------------------------------------------------------------------------
// case files

type caseFile struct {
	Group    string `json:"group"`
	Upstream string `json:"upstream"`
	Cases    []Case `json:"cases"`
}

type Case struct {
	ID         string            `json:"id"`
	Fn         string            `json:"fn"`
	Args       []json.RawMessage `json:"args"`
	UpstreamFn string            `json:"upstreamFn"`
	GoFn       string            `json:"goFn"`
	Note       string            `json:"note"`
	Deviation  string            `json:"deviation"`

	group string
}

func loadCases(t *testing.T) []Case {
	t.Helper()
	paths, err := filepath.Glob("cases/*.json")
	if err != nil {
		t.Fatalf("globbing cases: %v", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no case files found in cases/")
	}
	var all []Case
	seen := map[string]string{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("reading %s: %v", p, err)
		}
		var cf caseFile
		if err := json.Unmarshal(raw, &cf); err != nil {
			t.Fatalf("parsing %s: %v", p, err)
		}
		if cf.Upstream != upstreamPin {
			t.Fatalf("%s pins upstream %q, want %q", p, cf.Upstream, upstreamPin)
		}
		for _, c := range cf.Cases {
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q in %s (already in %s)", c.ID, p, prev)
			}
			seen[c.ID] = p
			c.group = cf.Group
			all = append(all, c)
		}
	}
	return all
}

// ---------------------------------------------------------------------------
// runners

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value string `json:"value"`
	Error string `json:"error"`
}

// runner is a long-lived subprocess speaking JSON Lines on stdio. It is started
// once and every case is streamed through it.
type runner struct {
	name string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Scanner
	errb *strings.Builder
}

func startRunner(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin pipe: %v", name, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout pipe: %v", name, err)
	}
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<25)
	r := &runner{name: name, cmd: cmd, in: in, out: sc, errb: &errb}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})
	return r
}

var errRunnerDead = errors.New("runner produced no reply")

// call sends one case and waits for its reply, enforcing a per-case timeout so
// a hung runner fails that case rather than the suite.
func (r *runner) call(c Case) (reply, error) {
	req := map[string]any{"id": c.ID, "fn": c.Fn, "args": c.Args}
	line, err := json.Marshal(req)
	if err != nil {
		return reply{}, err
	}
	if _, err := r.in.Write(append(line, '\n')); err != nil {
		return reply{}, fmt.Errorf("%s: write: %w", r.name, err)
	}

	type res struct {
		rep reply
		err error
	}
	ch := make(chan res, 1)
	go func() {
		if !r.out.Scan() {
			if err := r.out.Err(); err != nil {
				ch <- res{err: fmt.Errorf("%s: read: %w", r.name, err)}
				return
			}
			ch <- res{err: fmt.Errorf("%s: %w (stderr: %s)", r.name, errRunnerDead, r.errb.String())}
			return
		}
		var rep reply
		if err := json.Unmarshal(r.out.Bytes(), &rep); err != nil {
			ch <- res{err: fmt.Errorf("%s: bad reply %q: %w", r.name, truncate(r.out.Text()), err)}
			return
		}
		ch <- res{rep: rep}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), caseTimeout)
	defer cancel()
	select {
	case out := <-ch:
		return out.rep, out.err
	case <-ctx.Done():
		return reply{}, fmt.Errorf("%s: timed out after %s", r.name, caseTimeout)
	}
}

// ---------------------------------------------------------------------------
// score

type score struct {
	Library                 string   `json:"library"`
	Upstream                string   `json:"upstream"`
	GoModule                string   `json:"goModule"`
	GeneratedAt             string   `json:"generatedAt"`
	Cases                   int      `json:"cases"`
	Match                   int      `json:"match"`
	Differs                 int      `json:"differs"`
	Deviations              int      `json:"deviations"`
	Errors                  int      `json:"harnessErrors"`
	BothFailed              int      `json:"bothFailed"`
	ParityPercent           float64  `json:"parityPercent"`
	StructuralMatch         int      `json:"structuralMatch"`
	StructuralParityPercent float64  `json:"structuralParityPercent"`
	Mismatched              []string `json:"mismatchedCases"`
	StructuralMismatched    []string `json:"structuralMismatchedCases"`
	Comparison              string   `json:"comparison"`
	StructuralComparison    string   `json:"structuralComparison"`
}

// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping nodemailer parity harness")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "nodemailer", "package.json")); err != nil {
		t.Skip("parity/nodemailer/node/node_modules is missing; run `npm install` there to enable the harness")
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	// Build the Go runner once.
	goBin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", goBin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building Go runner: %v\n%s", err, out)
	}

	nodeCmd := exec.Command(node, filepath.Join(root, "node", "run.js"))
	nodeCmd.Dir = root
	nodeCmd.Env = append(os.Environ(), "PARITY_ROOT="+root)
	up := startRunner(t, "node", nodeCmd)

	goCmd := exec.Command(goBin)
	goCmd.Dir = root
	goCmd.Env = append(os.Environ(), "PARITY_ROOT="+root, "GOWORK=off")
	gr := startRunner(t, "go", goCmd)

	cases := loadCases(t)
	sc := score{
		Library:              "nodemailer",
		Upstream:             upstreamPin,
		GoModule:             goModuleVersion(t),
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		Cases:                len(cases),
		Comparison:           "canonical MIME tree: boundaries pinned to B0..Bn in order of first appearance, headers unfolded, RFC 2047-decoded, address lists and media-type parameters (RFC 2231) normalised, names canonically cased and sorted, bodies transfer-decoded, malformed constructs recorded",
		StructuralComparison: "same tree with Content-Transfer-Encoding and empty-valued headers masked out (the port's two systematic divergences)",
	}

	for _, c := range cases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			upRep, upErr := up.call(c)
			goRep, goErr := gr.call(c)
			if upErr != nil || goErr != nil {
				sc.Errors++
				t.Fatalf("harness error: upstream=%v go=%v", upErr, goErr)
			}
			if upRep.ID != c.ID || goRep.ID != c.ID {
				sc.Errors++
				t.Fatalf("reply id mismatch: upstream=%q go=%q want %q", upRep.ID, goRep.ID, c.ID)
			}

			// 1. Compare success/failure first.
			if upRep.OK != goRep.OK {
				if c.Deviation != "" {
					sc.Deviations++
					t.Skipf("declared deviation: %s\n  upstream ok=%v err=%q\n  go       ok=%v err=%q",
						c.Deviation, upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				}
				sc.Differs++
				sc.Mismatched = append(sc.Mismatched, c.ID)
				t.Errorf("ok differs: upstream ok=%v (error %q), go ok=%v (error %q)",
					upRep.OK, upRep.Error, goRep.OK, goRep.Error)
				return
			}
			if !upRep.OK {
				// Both failed: parity on failure, message text is not compared.
				sc.BothFailed++
				sc.Match++
				sc.StructuralMatch++
				t.Logf("both failed (as expected)\n  upstream: %s\n  go:       %s", upRep.Error, goRep.Error)
				return
			}

			// 2. Compare the canonical MIME trees.
			upTree := Canonicalise(upRep.Value)
			goTree := Canonicalise(goRep.Value)
			equal := reflect.DeepEqual(upTree, goTree)
			structural := reflect.DeepEqual(StripSystematic(upTree), StripSystematic(goTree))
			if structural {
				sc.StructuralMatch++
			} else {
				sc.StructuralMismatched = append(sc.StructuralMismatched, c.ID)
			}
			if equal {
				sc.Match++
				return
			}
			if c.Deviation != "" {
				sc.Deviations++
				t.Skipf("declared deviation: %s\n%s", c.Deviation, diff(upTree, goTree))
			}
			sc.Differs++
			sc.Mismatched = append(sc.Mismatched, c.ID)
			t.Errorf("canonical MIME trees differ (structural-only=%v)\n%s\n--- upstream raw ---\n%s\n--- go raw ---\n%s",
				structural, diff(upTree, goTree), upRep.Value, goRep.Value)
		})
	}

	compared := sc.Match + sc.Differs
	if compared > 0 {
		sc.ParityPercent = round2(100 * float64(sc.Match) / float64(compared))
		sc.StructuralParityPercent = round2(100 * float64(sc.StructuralMatch) / float64(compared))
	}
	sort.Strings(sc.Mismatched)
	sort.Strings(sc.StructuralMismatched)
	writeScore(t, sc)
	t.Logf("parity: %d/%d cases match (%.2f%%); structural (transfer-encoding and empty headers masked): %d (%.2f%%); deviations: %d; both-failed: %d",
		sc.Match, compared, sc.ParityPercent, sc.StructuralMatch, sc.StructuralParityPercent, sc.Deviations, sc.BothFailed)
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func writeScore(t *testing.T, sc score) {
	t.Helper()
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Errorf("marshalling parity.json: %v", err)
		return
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Errorf("writing parity.json: %v", err)
	}
}

func goModuleVersion(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/nodemailer")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// diff renders the two canonical trees side by side, listing the differing
// paths first so a failure is readable.
func diff(a, b *Node) string {
	var sb strings.Builder
	var paths []string
	walkDiff("", a, b, &paths)
	if len(paths) > 0 {
		sb.WriteString("differing fields:\n")
		for _, p := range paths {
			sb.WriteString("  " + p + "\n")
		}
	}
	sb.WriteString("upstream canonical:\n" + mustJSON(a) + "\ngo canonical:\n" + mustJSON(b) + "\n")
	return sb.String()
}

func walkDiff(path string, a, b *Node, out *[]string) {
	if a == nil || b == nil {
		if a != b {
			*out = append(*out, path+": one side is absent")
		}
		return
	}
	if a.ContentType != b.ContentType {
		*out = append(*out, fmt.Sprintf("%s.contentType: %q != %q", path, a.ContentType, b.ContentType))
	}
	if a.TransferEncoding != b.TransferEncoding {
		*out = append(*out, fmt.Sprintf("%s.transferEncoding: %q != %q", path, a.TransferEncoding, b.TransferEncoding))
	}
	if a.BodyKind != b.BodyKind {
		*out = append(*out, fmt.Sprintf("%s.bodyKind: %q != %q", path, a.BodyKind, b.BodyKind))
	}
	if a.Body != b.Body {
		*out = append(*out, fmt.Sprintf("%s.body: %q != %q", path, truncate(a.Body), truncate(b.Body)))
	}
	if !reflect.DeepEqual(a.Malformed, b.Malformed) {
		*out = append(*out, fmt.Sprintf("%s.malformed: %v != %v", path, a.Malformed, b.Malformed))
	}
	// Headers: report by name.
	ah, bh := indexHeaders(a.Headers), indexHeaders(b.Headers)
	names := map[string]bool{}
	for n := range ah {
		names[n] = true
	}
	for n := range bh {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		if !reflect.DeepEqual(ah[n], bh[n]) {
			*out = append(*out, fmt.Sprintf("%s.header[%s]: %q != %q", path, n, ah[n], bh[n]))
		}
	}
	if len(a.Parts) != len(b.Parts) {
		*out = append(*out, fmt.Sprintf("%s.parts: %d != %d", path, len(a.Parts), len(b.Parts)))
	}
	n := len(a.Parts)
	if len(b.Parts) < n {
		n = len(b.Parts)
	}
	for i := 0; i < n; i++ {
		walkDiff(fmt.Sprintf("%s.parts[%d]", path, i), a.Parts[i], b.Parts[i], out)
	}
}

func indexHeaders(hs []Header) map[string][]string {
	m := map[string][]string{}
	for _, h := range hs {
		m[h.Name] = append(m[h.Name], h.Value)
	}
	return m
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(b)
}
