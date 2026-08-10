// Package parity drives the upstream OpenCV runner (python/run.py, cv2) and the
// Go runner (go/run.go, github.com/malcolmston/opencv) over the same cases and
// compares their answers. See ../HARNESS.md.
//
// # Comparison modes
//
// Two independent implementations of the same image operation legitimately
// disagree in the last bit, so every case declares how it is to be compared:
//
//   - "exact" — integer-valued, well-defined operations: thresholding, bitwise
//     logic, morphology with a given structuring element, connected-component
//     labels and counts, matrix shape/type, LUTs. Images are compared
//     byte-for-byte and numbers with a 1e-12 relative/absolute epsilon that only
//     absorbs JSON formatting, not arithmetic.
//
//   - "tolerance" — filters, geometric transforms and float results. Images are
//     compared per pixel against a declared max-abs-difference budget (`tol`),
//     with an optional fraction of pixels allowed to exceed it (`frac`), and
//     *additionally* against summary statistics: per-channel mean absolute
//     difference (`meanTol`), per-channel min/max, and a 32-bin per-channel
//     histogram L1 distance (`histTol`). Float matrices use a combined
//     absolute/relative budget (`tol`, `relTol`).
//
//   - "structural" — detectors and contour finders. The runners emit counts and
//     sorted, rounded coordinates rather than raw float scores, and those
//     reduced structures are then compared exactly.
//
// Encoded PNG/JPEG bytes are never compared: images cross the wire as the raw
// row-major buffer in hex plus an explicit shape, or as an explicit float grid.
package parity

import (
	"bufio"
	"bytes"
	"encoding/hex"
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
	// exactEpsilon absorbs JSON number formatting only.
	exactEpsilon = 1e-12
	// histBins is the resolution of the summary histogram compared in
	// tolerance mode.
	histBins    = 32
	caseTimeout = 60 * time.Second
)

// ------------------------------------------------------------------ cases

type Case struct {
	ID         string          `json:"id"`
	Fn         string          `json:"fn"`
	Args       json.RawMessage `json:"args"`
	UpstreamFn string          `json:"upstreamFn"`
	GoFn       string          `json:"goFn"`
	Mode       string          `json:"mode"`
	Tol        *float64        `json:"tol"`
	RelTol     *float64        `json:"relTol"`
	Frac       *float64        `json:"frac"`
	MeanTol    *float64        `json:"meanTol"`
	HistTol    *float64        `json:"histTol"`
	Note       string          `json:"note,omitempty"`
	Deviation  string          `json:"deviation,omitempty"`

	Group string `json:"-"`
}

type budget struct {
	mode    string
	abs     float64
	rel     float64
	frac    float64
	meanTol float64
	histTol float64
}

func (c Case) budget() budget {
	b := budget{mode: c.Mode, abs: exactEpsilon, rel: exactEpsilon,
		frac: 0, meanTol: math.Inf(1), histTol: 0.05}
	if c.Mode == "tolerance" {
		b.rel = 0
		if c.Tol != nil {
			b.abs = *c.Tol
		}
		if c.RelTol != nil {
			b.rel = *c.RelTol
		}
		if c.Frac != nil {
			b.frac = *c.Frac
		}
		if c.MeanTol != nil {
			b.meanTol = *c.MeanTol
		} else {
			b.meanTol = b.abs
		}
		if c.HistTol != nil {
			b.histTol = *c.HistTol
		}
	}
	return b
}

type caseFile struct {
	Group    string `json:"group"`
	Upstream string `json:"upstream"`
	Cases    []Case `json:"cases"`
}

func loadCases(t *testing.T) ([]Case, string) {
	t.Helper()
	paths, err := filepath.Glob("cases/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no case files found in cases/: %v", err)
	}
	sort.Strings(paths)
	var all []Case
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
			t.Fatalf("%s pins upstream %q but another file pins %q", p, cf.Upstream, upstream)
		}
		for i := range cf.Cases {
			c := cf.Cases[i]
			if c.Mode == "" {
				c.Mode = "exact"
			}
			switch c.Mode {
			case "exact", "tolerance", "structural":
			default:
				t.Fatalf("%s: case %s has unknown mode %q", p, c.ID, c.Mode)
			}
			if prev, dup := seen[c.ID]; dup {
				t.Fatalf("duplicate case id %q (in %s and %s)", c.ID, prev, p)
			}
			seen[c.ID] = p
			c.Group = cf.Group
			all = append(all, c)
		}
	}
	return all, upstream
}

// ----------------------------------------------------------------- runner

type reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

type runner struct {
	name   string
	cmd    *exec.Cmd
	in     io.WriteCloser
	lines  chan string
	stderr *strings.Builder
}

func start(t *testing.T, name string, cmd *exec.Cmd) *runner {
	t.Helper()
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%s: stdin: %v", name, err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%s: stdout: %v", name, err)
	}
	var errbuf strings.Builder
	cmd.Stderr = &errbuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	r := &runner{name: name, cmd: cmd, in: in, lines: make(chan string, 8), stderr: &errbuf}
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<27)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
		close(r.lines)
	}()
	t.Cleanup(func() {
		_ = r.in.Close()
		_ = r.cmd.Wait()
	})
	return r
}

// ask sends one case and waits for exactly one reply, enforcing a per-case
// timeout so a hung runner fails that case rather than the suite.
func (r *runner) ask(c Case) reply {
	// Args are spliced in verbatim rather than re-marshalled so that no float
	// literal in a case file is reformatted on the way to either runner.
	id, _ := json.Marshal(c.ID)
	fn, _ := json.Marshal(c.Fn)
	args := []byte("[]")
	if len(c.Args) > 0 {
		var buf bytes.Buffer
		if err := json.Compact(&buf, c.Args); err != nil {
			return reply{ID: c.ID, OK: false, Error: "harness: " + err.Error()}
		}
		args = buf.Bytes()
	}
	line := []byte(fmt.Sprintf(`{"id":%s,"fn":%s,"args":%s}`, id, fn, args))
	if _, err := r.in.Write(append(line, '\n')); err != nil {
		return reply{ID: c.ID, OK: false, Error: "harness: write: " + err.Error()}
	}
	select {
	case l, ok := <-r.lines:
		if !ok {
			return reply{ID: c.ID, OK: false,
				Error: fmt.Sprintf("harness: %s runner exited (stderr: %s)", r.name, tail(r.stderr.String()))}
		}
		var rp reply
		if err := json.Unmarshal([]byte(l), &rp); err != nil {
			return reply{ID: c.ID, OK: false, Error: "harness: bad reply: " + err.Error()}
		}
		return rp
	case <-time.After(caseTimeout):
		return reply{ID: c.ID, OK: false, Error: "harness: timeout"}
	}
}

func tail(s string) string {
	if len(s) > 600 {
		return "…" + s[len(s)-600:]
	}
	return s
}

// ---------------------------------------------------------------- compare

func numEqual(a, b float64, b_ budget) bool {
	if a == b {
		return true
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	d := math.Abs(a - b)
	if d <= b_.abs {
		return true
	}
	m := math.Max(math.Abs(a), math.Abs(b))
	return b_.rel > 0 && d <= b_.rel*m
}

type mismatch struct{ what string }

func (m *mismatch) Error() string { return m.what }

func fail(format string, args ...any) error { return &mismatch{fmt.Sprintf(format, args...)} }

func isKind(v any, kind string) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	k, _ := m["kind"].(string)
	return m, k == kind
}

func matShape(m map[string]any) (rows, cols, ch int, buf []byte, err error) {
	f := func(k string) int {
		v, _ := m[k].(float64)
		return int(v)
	}
	rows, cols, ch = f("rows"), f("cols"), f("channels")
	h, _ := m["hex"].(string)
	buf, err = hex.DecodeString(h)
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("undecodable hex buffer: %v", err)
	}
	if len(buf) != rows*cols*ch {
		return 0, 0, 0, nil, fmt.Errorf("buffer length %d does not match shape %dx%dx%d",
			len(buf), rows, cols, ch)
	}
	return rows, cols, ch, buf, nil
}

type chanStats struct {
	mean, min, max float64
}

func statsOf(buf []byte, ch, c int) chanStats {
	s := chanStats{min: math.Inf(1), max: math.Inf(-1)}
	n := 0
	for i := c; i < len(buf); i += ch {
		v := float64(buf[i])
		s.mean += v
		s.min = math.Min(s.min, v)
		s.max = math.Max(s.max, v)
		n++
	}
	if n > 0 {
		s.mean /= float64(n)
	}
	return s
}

func histOf(buf []byte, ch, c int) ([histBins]int, int) {
	var h [histBins]int
	n := 0
	for i := c; i < len(buf); i += ch {
		h[int(buf[i])*histBins/256]++
		n++
	}
	return h, n
}

// compareMat compares two uint8 images. In exact mode the buffers must be
// identical; in tolerance mode the declared per-pixel budget, the per-channel
// summary statistics and a 32-bin per-channel histogram must all agree.
func compareMat(a, b map[string]any, bg budget) error {
	ar, ac, ach, abuf, err := matShape(a)
	if err != nil {
		return fail("upstream mat: %v", err)
	}
	br, bc, bch, bbuf, err := matShape(b)
	if err != nil {
		return fail("go mat: %v", err)
	}
	if ar != br || ac != bc || ach != bch {
		return fail("shape differs: upstream %dx%dx%d, go %dx%dx%d", ar, ac, ach, br, bc, bch)
	}
	if bg.mode != "tolerance" {
		if !bytes.Equal(abuf, bbuf) {
			diff, maxAbs, first := 0, 0, -1
			for i := range abuf {
				d := int(abuf[i]) - int(bbuf[i])
				if d != 0 {
					diff++
					if first < 0 {
						first = i
					}
				}
				if d < 0 {
					d = -d
				}
				if d > maxAbs {
					maxAbs = d
				}
			}
			y, x, c := first/(ac*ach), (first/ach)%ac, first%ach
			return fail("%d of %d samples differ (max |Δ| = %d); first at (y=%d,x=%d,c=%d): upstream %d, go %d",
				diff, len(abuf), maxAbs, y, x, c, abuf[first], bbuf[first])
		}
		return nil
	}

	n := len(abuf)
	exceed, maxAbs, sumAbs := 0, 0.0, 0.0
	firstBad := -1
	for i := range abuf {
		d := math.Abs(float64(abuf[i]) - float64(bbuf[i]))
		sumAbs += d
		if d > maxAbs {
			maxAbs = d
		}
		if d > bg.abs {
			exceed++
			if firstBad < 0 {
				firstBad = i
			}
		}
	}
	meanAbs := 0.0
	if n > 0 {
		meanAbs = sumAbs / float64(n)
	}
	var problems []string
	if float64(exceed) > bg.frac*float64(n) {
		y, x, c := firstBad/(ac*ach), (firstBad/ach)%ac, firstBad%ach
		problems = append(problems, fmt.Sprintf(
			"%d of %d samples exceed the |Δ|<=%g budget (allowed %g%%, max |Δ| = %g); first at (y=%d,x=%d,c=%d): upstream %d, go %d",
			exceed, n, bg.abs, bg.frac*100, maxAbs, y, x, c, abuf[firstBad], bbuf[firstBad]))
	}
	if meanAbs > bg.meanTol {
		problems = append(problems, fmt.Sprintf("mean |Δ| = %.4f exceeds meanTol %g", meanAbs, bg.meanTol))
	}
	for c := 0; c < ach; c++ {
		as, bs := statsOf(abuf, ach, c), statsOf(bbuf, ach, c)
		if math.Abs(as.mean-bs.mean) > bg.meanTol ||
			math.Abs(as.min-bs.min) > bg.abs || math.Abs(as.max-bs.max) > bg.abs {
			problems = append(problems, fmt.Sprintf(
				"channel %d summary differs: upstream mean/min/max %.4f/%g/%g, go %.4f/%g/%g",
				c, as.mean, as.min, as.max, bs.mean, bs.min, bs.max))
		}
		ah, an := histOf(abuf, ach, c)
		bh, _ := histOf(bbuf, ach, c)
		l1 := 0
		for i := range ah {
			d := ah[i] - bh[i]
			if d < 0 {
				d = -d
			}
			l1 += d
		}
		if an > 0 {
			dist := float64(l1) / (2 * float64(an))
			if dist > bg.histTol {
				problems = append(problems, fmt.Sprintf(
					"channel %d %d-bin histogram L1 distance %.4f exceeds histTol %g",
					c, histBins, dist, bg.histTol))
			}
		}
	}
	if len(problems) > 0 {
		return fail("%s", strings.Join(problems, "; "))
	}
	return nil
}

func compareFMat(a, b map[string]any, bg budget) error {
	shape := func(m map[string]any) (int, int, []any) {
		r, _ := m["rows"].(float64)
		c, _ := m["cols"].(float64)
		d, _ := m["data"].([]any)
		return int(r), int(c), d
	}
	ar, ac, ad := shape(a)
	br, bc, bd := shape(b)
	if ar != br || ac != bc || len(ad) != len(bd) {
		return fail("float shape differs: upstream %dx%d (%d values), go %dx%d (%d values)",
			ar, ac, len(ad), br, bc, len(bd))
	}
	exceed, first := 0, -1
	maxAbs := 0.0
	for i := range ad {
		av, aok := asFloat(ad[i])
		bv, bok := asFloat(bd[i])
		if !aok || !bok {
			// Sentinel strings for non-finite values: compared exactly.
			if fmt.Sprint(ad[i]) != fmt.Sprint(bd[i]) {
				return fail("value %d differs: upstream %v, go %v", i, ad[i], bd[i])
			}
			continue
		}
		d := math.Abs(av - bv)
		if d > maxAbs {
			maxAbs = d
		}
		if !numEqual(av, bv, bg) {
			exceed++
			if first < 0 {
				first = i
			}
		}
	}
	if float64(exceed) > bg.frac*float64(len(ad)) {
		i := first
		av, _ := asFloat(ad[i])
		bv, _ := asFloat(bd[i])
		return fail("%d of %d float values outside the budget (abs %g, rel %g; max |Δ| = %g); first at [%d,%d]: upstream %v, go %v",
			exceed, len(ad), bg.abs, bg.rel, maxAbs, i/ac, i%ac, av, bv)
	}
	return nil
}

func asFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func compareValue(path string, a, b any, bg budget) error {
	if am, ok := isKind(a, "mat"); ok {
		bm, ok := isKind(b, "mat")
		if !ok {
			return fail("%s: upstream returned an image, go returned %T", path, b)
		}
		if err := compareMat(am, bm, bg); err != nil {
			return fail("%s: %v", path, err)
		}
		return nil
	}
	if am, ok := isKind(a, "fmat"); ok {
		bm, ok := isKind(b, "fmat")
		if !ok {
			return fail("%s: upstream returned a float matrix, go returned %T", path, b)
		}
		if err := compareFMat(am, bm, bg); err != nil {
			return fail("%s: %v", path, err)
		}
		return nil
	}
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		if !ok || !numEqual(av, bv, bg) {
			return fail("%s: upstream %v, go %v", path, a, b)
		}
	case string:
		if bv, ok := b.(string); !ok || av != bv {
			return fail("%s: upstream %q, go %v", path, av, b)
		}
	case bool:
		if bv, ok := b.(bool); !ok || av != bv {
			return fail("%s: upstream %v, go %v", path, av, b)
		}
	case nil:
		if b != nil {
			return fail("%s: upstream null, go %v", path, b)
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return fail("%s: upstream returned a list, go returned %T", path, b)
		}
		if len(av) != len(bv) {
			return fail("%s: length differs: upstream %d, go %d", path, len(av), len(bv))
		}
		for i := range av {
			if err := compareValue(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i], bg); err != nil {
				return err
			}
		}
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return fail("%s: upstream returned an object, go returned %T", path, b)
		}
		keys := make([]string, 0, len(av))
		for k := range av {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			w, present := bv[k]
			if !present {
				return fail("%s: go is missing key %q", path, k)
			}
			if err := compareValue(path+"."+k, av[k], w, bg); err != nil {
				return err
			}
		}
		if len(av) != len(bv) {
			return fail("%s: go has extra keys (upstream %d keys, go %d)", path, len(av), len(bv))
		}
	default:
		return fail("%s: uncomparable value %T", path, a)
	}
	return nil
}

func decode(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "harness: undecodable value: " + string(raw)
	}
	return v
}

func show(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

// ------------------------------------------------------------------- test

type score struct {
	Library      string             `json:"library"`
	Upstream     string             `json:"upstream"`
	GoModule     string             `json:"goModule"`
	Modes        map[string]int     `json:"casesByMode"`
	Cases        int                `json:"cases"`
	Match        int                `json:"match"`
	Mismatch     int                `json:"mismatch"`
	Deviations   int                `json:"deviations"`
	BothFailed   int                `json:"bothFailed"`
	GoPanics     int                `json:"goPanicsWhereUpstreamSucceeded"`
	Denominator  int                `json:"parityDenominator"`
	ParityPct    float64            `json:"parityPercent"`
	ByGroup      map[string]int     `json:"casesByGroup"`
	GroupParity  map[string]float64 `json:"parityPercentByGroup"`
	Mismatches   []string           `json:"mismatchIDs"`
	DeviationIDs []string           `json:"deviationIDs"`
	GeneratedBy  string             `json:"generatedBy"`
}

func TestParity(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found; skipping opencv parity")
	}
	if out, err := exec.Command(py, "-c", "import cv2").CombinedOutput(); err != nil {
		t.Skipf("cv2 not importable by %s (%s); skipping opencv parity", py, strings.TrimSpace(string(out)))
	}

	cases, upstream := loadCases(t)

	bin := filepath.Join(t.TempDir(), "gorunner")
	build := exec.Command("go", "build", "-o", bin, "./go")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building go runner: %v\n%s", err, out)
	}

	up := start(t, "python", exec.Command(py, "-u", "python/run.py"))
	goRun := start(t, "go", exec.Command(bin))

	s := score{
		Library: "opencv", Upstream: upstream, GoModule: goModuleVersion(),
		Cases: len(cases), ByGroup: map[string]int{}, Modes: map[string]int{},
		GroupParity: map[string]float64{},
		GeneratedBy: "GOWORK=off go test ./parity/opencv/",
	}
	groupMatch := map[string]int{}
	groupCompared := map[string]int{}

	for _, c := range cases {
		c := c
		s.ByGroup[c.Group]++
		s.Modes[c.Mode]++
		ur := up.ask(c)
		gr := goRun.ask(c)
		bg := c.budget()
		ok := true
		t.Run(c.ID, func(t *testing.T) {
			report := t.Errorf
			if c.Deviation != "" {
				report = t.Logf // deliberate divergence: recorded, not a red build
			}
			switch {
			case ur.OK != gr.OK:
				ok = false
				report("%s [%s/%s]: upstream ok=%v (%s%s) but go ok=%v (%s%s)",
					c.ID, c.Group, c.Mode, ur.OK, show(ur.Value), ur.Error,
					gr.OK, show(gr.Value), gr.Error)
			case !ur.OK && !gr.OK:
				// Both failed: parity. Message text is allowed to differ.
			default:
				if err := compareValue("value", decode(ur.Value), decode(gr.Value), bg); err != nil {
					ok = false
					report("%s [%s/%s]: %v", c.ID, c.Group, c.Mode, err)
				}
			}
		})
		if ur.OK && !gr.OK && strings.Contains(gr.Error, "panic:") {
			s.GoPanics++
		}
		groupCompared[c.Group]++
		switch {
		case c.Deviation != "":
			s.Deviations++
			s.DeviationIDs = append(s.DeviationIDs, c.ID)
			groupCompared[c.Group]--
		case !ok:
			s.Mismatch++
			s.Mismatches = append(s.Mismatches, c.ID)
		default:
			s.Match++
			groupMatch[c.Group]++
			if !ur.OK && !gr.OK {
				s.BothFailed++
			}
		}
	}

	s.Denominator = s.Match + s.Mismatch
	if s.Denominator > 0 {
		s.ParityPct = math.Round(float64(s.Match)/float64(s.Denominator)*10000) / 100
	}
	for g, n := range groupCompared {
		if n > 0 {
			s.GroupParity[g] = math.Round(float64(groupMatch[g])/float64(n)*10000) / 100
		}
	}
	sort.Strings(s.Mismatches)
	sort.Strings(s.DeviationIDs)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	if err := os.WriteFile("parity.json", append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write parity.json: %v", err)
	}
	t.Logf("cases=%d match=%d mismatch=%d deviations=%d bothFailed=%d goPanics=%d parity=%.2f%% (n=%d)",
		s.Cases, s.Match, s.Mismatch, s.Deviations, s.BothFailed, s.GoPanics,
		s.ParityPct, s.Denominator)
}

func goModuleVersion() string {
	cmd := exec.Command("go", "list", "-m", "github.com/malcolmston/opencv")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
