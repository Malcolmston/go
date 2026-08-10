// Package parity drives the upstream (Node) pdfkit runner and the Go pdfkit
// runner over the same cases and compares the PDFs they generate.
//
// PDF bytes are never compared: two independent generators disagree on object
// numbering, whitespace, resource naming and operator spelling without any
// difference in the document. Instead each runner returns the finished file and
// this package parses it back out (canonical_test.go) into a canonical
// structural description — header version, page geometry, referenced resources,
// annotations, metadata, outline, AcroForm, and a normalised per-page
// content-stream operator trace — and compares that.
package parity

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
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
	upstreamPin = "pdfkit@0.15.1"
	caseTimeout = 30 * time.Second
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
	sc.Buffer(make([]byte, 0, 1<<20), 1<<27)
	r := &runner{name: name, cmd: cmd, in: in, out: sc, errb: &errb}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})
	return r
}

var errRunnerDead = errors.New("runner produced no reply")

// call sends one case and waits for its reply, enforcing a per-case timeout so a
// hung runner fails that case rather than the suite.
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
			ch <- res{err: fmt.Errorf("%s: bad reply: %w", r.name, err)}
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
	OnlyGoFailed            int      `json:"onlyGoFailed"`
	OnlyUpstreamFailed      int      `json:"onlyUpstreamFailed"`
	InvalidStructure        []string `json:"invalidStructureCases"`
	Panics                  []string `json:"goPanicCases"`
	ParityPercent           float64  `json:"parityPercent"`
	StructuralMatch         int      `json:"structuralMatch"`
	StructuralParityPercent float64  `json:"structuralParityPercent"`
	Mismatched              []string `json:"mismatchedCases"`
	StructuralMismatched    []string `json:"structuralMismatchedCases"`
	Rounding                string   `json:"rounding"`
	Comparison              string   `json:"comparison"`
	StructuralComparison    string   `json:"structuralComparison"`
	Pinned                  []string `json:"pinnedForDeterminism"`
}

const (
	roundingNote   = "every number is snapped to the 1e-4 grid (the resolution at which the Go port serialises coordinates, so its own rounding cannot split the two sides) and then rounded to 3 decimal places. 0.001 pt is 1/72000 inch, about 0.35 um and ~1/4000 of a pixel at 72 dpi: far below any rendering-relevant threshold, and far above the 1e-9..1e-12 noise the two implementations differ by when they reach the same coordinate through different arithmetic. Colours live in [0,1] where 0.001 still separates adjacent 8-bit levels (1/255 = 0.0039), so the same precision serves them."
	comparisonNote = "canonical PDF structure parsed back out of both files by one parser: header version, page count, MediaBox/CropBox/Rotate, referenced font/XObject/ExtGState/Shading/Pattern/ColorSpace/ProcSet resources, annotations with rect and action, /Info entries, outline tree, AcroForm fields, named destinations, encryption dictionary, plus a per-page content-stream operator trace with coordinates mapped into device space and resource names replaced by the resource they name"
	structuralNote = "same structure with the port's six systematic divergences masked: header version, /Info CreationDate+ModDate, trailer /ID, the /ProcSet resource array, the cs+scn vs rg/g/k colour-operator spelling, and the q/Q/cm bookkeeping operators (coordinates are already reported in device space)"
)

var pinnedNotes = []string{
	"upstream info.CreationDate fixed to 2026-01-02T15:04:05Z, which also fixes pdfkit's file id (an MD5 over /Info)",
	"info.Producer and info.Creator pinned to \"parity\" on both sides (upstream defaults to \"PDFKit\", the port to \"pdfkit\")",
	"upstream compress:false, because the port never compresses page content streams",
	"upstream autoFirstPage:false and bufferPages:true, so page creation is explicit and pages stay addressable",
	"the trailer /ID is normalised to presence-only: upstream derives it from /Info, the port from a digest of the whole document and only when encrypting",
	"/Info CreationDate and ModDate are normalised to <date>: the port has no API to set either",
	"image inputs are committed fixtures generated by ./fixtures/gen, so both sides read identical bytes",
}

// ---------------------------------------------------------------------------

func TestParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH; skipping pdfkit parity harness")
	}
	if _, err := os.Stat(filepath.Join("node", "node_modules", "pdfkit", "package.json")); err != nil {
		t.Skip("parity/pdfkit/node/node_modules is missing; run `npm install` there to enable the harness")
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

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
		Library:              "pdfkit",
		Upstream:             upstreamPin,
		GoModule:             goModuleVersion(),
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		Cases:                len(cases),
		Rounding:             roundingNote,
		Comparison:           comparisonNote,
		StructuralComparison: structuralNote,
		Pinned:               pinnedNotes,
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
			if strings.HasPrefix(goRep.Error, "panic:") {
				sc.Panics = append(sc.Panics, c.ID)
			}

			// 1. Compare success/failure first.
			if upRep.OK != goRep.OK {
				if upRep.OK {
					sc.OnlyGoFailed++
				} else {
					sc.OnlyUpstreamFailed++
				}
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
				sc.BothFailed++
				sc.Match++
				sc.StructuralMatch++
				t.Logf("both failed (as expected)\n  upstream: %s\n  go:       %s", upRep.Error, goRep.Error)
				return
			}

			// 2. Parse both files with the same parser and compare.
			upPDF, err := hex.DecodeString(upRep.Value)
			if err != nil {
				sc.Errors++
				t.Fatalf("upstream returned non-hex output: %v", err)
			}
			goPDF, err := hex.DecodeString(goRep.Value)
			if err != nil {
				sc.Errors++
				t.Fatalf("go returned non-hex output: %v", err)
			}
			upCanon := Canonicalise(upPDF)
			goCanon := Canonicalise(goPDF)

			// A structurally invalid file is a failure regardless of parity.
			if len(upCanon.Broken) > 0 || len(goCanon.Broken) > 0 {
				sc.InvalidStructure = append(sc.InvalidStructure, c.ID)
				t.Errorf("invalid PDF structure\n  upstream: %v\n  go:       %v", upCanon.Broken, goCanon.Broken)
			}

			equal := reflect.DeepEqual(upCanon, goCanon)
			structural := reflect.DeepEqual(StripSystematic(upCanon), StripSystematic(goCanon))
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
				t.Skipf("declared deviation: %s\n%s", c.Deviation, diff(upCanon, goCanon))
			}
			sc.Differs++
			sc.Mismatched = append(sc.Mismatched, c.ID)
			t.Errorf("canonical PDF structures differ (structural-only=%v)\n%s", structural, diff(upCanon, goCanon))
		})
	}

	compared := sc.Match + sc.Differs
	if compared > 0 {
		sc.ParityPercent = round2(100 * float64(sc.Match) / float64(compared))
		sc.StructuralParityPercent = round2(100 * float64(sc.StructuralMatch) / float64(compared))
	}
	sort.Strings(sc.Mismatched)
	sort.Strings(sc.StructuralMismatched)
	sort.Strings(sc.InvalidStructure)
	sort.Strings(sc.Panics)
	writeScore(t, sc)
	t.Logf("parity: %d/%d cases match (%.2f%%); structural (6 systematic divergences masked): %d (%.2f%%); "+
		"deviations: %d; both-failed: %d; only-go-failed: %d; only-upstream-failed: %d; go panics: %d; invalid structure: %d",
		sc.Match, compared, sc.ParityPercent, sc.StructuralMatch, sc.StructuralParityPercent,
		sc.Deviations, sc.BothFailed, sc.OnlyGoFailed, sc.OnlyUpstreamFailed, len(sc.Panics), len(sc.InvalidStructure))
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func writeScore(t *testing.T, sc score) {
	t.Helper()
	// parity.json is the committed score for the whole suite, so a filtered run
	// (go test -run …) must not overwrite it with a partial total.
	if f := flag.Lookup("test.run"); f != nil && f.Value.String() != "" {
		t.Logf("not rewriting parity.json: -run=%s selected a subset of the cases", f.Value.String())
		return
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		t.Errorf("marshalling parity.json: %v", err)
		return
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Errorf("writing parity.json: %v", err)
	}
}

func goModuleVersion() string {
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/pdfkit")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// diff

func diff(a, b *canonDoc) string {
	var lines []string
	add := func(format string, args ...any) { lines = append(lines, "  "+fmt.Sprintf(format, args...)) }

	if a.Version != b.Version {
		add("version: upstream %q != go %q", a.Version, b.Version)
	}
	if a.PageCount != b.PageCount {
		add("pageCount: upstream %d != go %d", a.PageCount, b.PageCount)
	}
	if a.IDPresent != b.IDPresent {
		add("trailer /ID present: upstream %v != go %v", a.IDPresent, b.IDPresent)
	}
	if a.Encrypted != b.Encrypted {
		add("encrypted: upstream %v != go %v", a.Encrypted, b.Encrypted)
	}
	diffStrings(add, "encrypt", a.Encrypt, b.Encrypt)
	diffStrings(add, "info", a.Info, b.Info)
	if a.Metadata != b.Metadata {
		add("xmpMetadata: upstream %v != go %v", a.Metadata, b.Metadata)
	}
	if a.HasOutline != b.HasOutline {
		add("hasOutline: upstream %v != go %v", a.HasOutline, b.HasOutline)
	}
	if !reflect.DeepEqual(a.Outline, b.Outline) {
		add("outline: upstream %s != go %s", mustJSON(a.Outline), mustJSON(b.Outline))
	}
	if a.HasAcroForm != b.HasAcroForm {
		add("hasAcroForm: upstream %v != go %v", a.HasAcroForm, b.HasAcroForm)
	}
	diffStrings(add, "acroForm", a.AcroForm, b.AcroForm)
	diffStrings(add, "namedDests", a.NamedDests, b.NamedDests)
	diffStrings(add, "broken", a.Broken, b.Broken)

	n := len(a.Pages)
	if len(b.Pages) < n {
		n = len(b.Pages)
	}
	for i := 0; i < n; i++ {
		p, q := a.Pages[i], b.Pages[i]
		pre := fmt.Sprintf("page[%d]", i)
		if p.MediaBox != q.MediaBox {
			add("%s.mediaBox: upstream %q != go %q", pre, p.MediaBox, q.MediaBox)
		}
		if p.CropBox != q.CropBox {
			add("%s.cropBox: upstream %q != go %q", pre, p.CropBox, q.CropBox)
		}
		if p.Rotate != q.Rotate {
			add("%s.rotate: upstream %q != go %q", pre, p.Rotate, q.Rotate)
		}
		diffStrings(add, pre+".procSet", p.ProcSet, q.ProcSet)
		diffStrings(add, pre+".fonts", p.Fonts, q.Fonts)
		diffStrings(add, pre+".xObjects", p.XObjects, q.XObjects)
		diffStrings(add, pre+".extGState", p.ExtGState, q.ExtGState)
		diffStrings(add, pre+".shading", p.Shading, q.Shading)
		diffStrings(add, pre+".pattern", p.Pattern, q.Pattern)
		diffStrings(add, pre+".colorSpace", p.ColorSpace, q.ColorSpace)
		diffStrings(add, pre+".contentFilters", p.ContentFilters, q.ContentFilters)
		if !reflect.DeepEqual(p.Annots, q.Annots) {
			add("%s.annots: upstream %s != go %s", pre, mustJSON(p.Annots), mustJSON(q.Annots))
		}
		if !reflect.DeepEqual(p.Trace, q.Trace) {
			add("%s.trace differs:\n%s", pre, sideBySide(p.Trace, q.Trace))
		}
	}
	if len(a.Pages) != len(b.Pages) {
		add("pages: upstream %d != go %d", len(a.Pages), len(b.Pages))
	}

	if len(lines) == 0 {
		lines = append(lines, "  (no field-level difference found; compare the full canonical forms)")
	}
	return "differing fields:\n" + strings.Join(lines, "\n") + "\n"
}

func diffStrings(add func(string, ...any), label string, a, b []string) {
	if reflect.DeepEqual(a, b) {
		return
	}
	add("%s: upstream %v != go %v", label, a, b)
}

// sideBySide renders two operator traces aligned by index, marking the rows that
// differ, which is what makes a trace failure readable.
func sideBySide(a, b []string) string {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		var x, y string
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		mark := "   "
		if x != y {
			mark = " ! "
		}
		fmt.Fprintf(&sb, "    %s%-4d upstream: %s\n    %s%-4s go:       %s\n", mark, i, x, mark, "", y)
	}
	return sb.String()
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<unmarshalable: %v>", err)
	}
	return string(b)
}
