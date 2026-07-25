// Package parity measures a Go port against the library it ports by running
// them both.
//
// The upstream library is not transcribed, summarized or remembered — it is
// installed and executed. A parity case is one question asked twice: once of
// the real npm/PyPI/RubyGems package in its own runtime, once of the Go port.
// Agreement is parity; disagreement is a gap, named and published. Because the
// expectations come from upstream itself, they cannot drift out of date the way
// hand-written ones do, and a new upstream release re-scores the port for free.
//
// A suite looks like this:
//
//	func TestParityChalk(t *testing.T) {
//		s := parity.New(t, parity.Config{
//			Repo:     "chalk",
//			Upstream: "chalk/chalk",
//			Oracle:   parity.Node("chalk@5.3.0"),
//		})
//		s.Case("bold", parity.Case{
//			Upstream: `const c = (await imp('chalk')).default; return c.bold(input)`,
//			Input:    "hello",
//			Go:       func() (any, error) { return chalk.Bold("hello"), nil },
//		})
//		s.Run()
//	}
//
// Run compares every case, cross-compiles the package for each supported
// GOOS/GOARCH, and writes the repo's parity.json — the artifact the landing
// aggregates. When the upstream runtime is not installed, cases replay from
// goldens recorded when it was (see PARITY_RECORD), and the report says so.
//
// Environment:
//
//	PARITY_RECORD=1     record upstream's answers into the golden files
//	PARITY_GOLDEN=1     replay goldens even if upstream could run
//	PARITY_OFFLINE=1    never install upstream packages
//	PARITY_CROSS=0      skip the cross-compile check
//	PARITY_DIR=path     suite fragment directory (default .parity)
//	PARITY_OUT=path     assembled report path (default parity.json)
package parity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Config describes what is being compared to what.
type Config struct {
	// Repo is the port's repo/directory name ("chalk"); it keys the landing's
	// aggregated table.
	Repo string
	// Upstream is the original library's GitHub slug ("chalk/chalk").
	Upstream string
	// Name distinguishes suites within a repo — a port with several upstream
	// entry points audits each separately. Defaults to the test's name.
	Name string
	// Oracle runs the original library. Nil means the suite can only check
	// cases that carry their own Want, and the report says "unmeasured".
	Oracle Oracle
	// Tolerance is the allowed float difference, relative for large
	// magnitudes. Defaults to 1e-9; set it wider for numeric ports whose
	// upstream suite allows more.
	Tolerance float64
	// GoldenDir holds recorded upstream answers. Defaults to testdata/parity.
	GoldenDir string
	// Package is what the cross-compile check builds. Defaults to "./...".
	Package string
	// Platforms overrides DefaultPlatforms for the cross-compile check.
	Platforms []Platform
	// Timeout bounds a single upstream evaluation. Defaults to two minutes.
	Timeout time.Duration
}

// A Case is one question asked of both implementations.
type Case struct {
	// Upstream is the program run inside the original library: a function body
	// with the case Input in scope, returning the upstream answer. What it
	// raises counts too — a port that succeeds where upstream throws is not at
	// parity, and vice versa.
	Upstream string
	// Input is JSON-encoded and handed to the upstream program.
	Input any
	// Go is the port's answer to the same question.
	Go func() (any, error)
	// Want pins the expected value for cases upstream cannot answer here (a
	// behavior removed from the current release, a platform-specific result).
	// It is used only when no upstream answer is available.
	Want any
	// Tolerance overrides Config.Tolerance for this case.
	Tolerance float64
	// Skip records a known, deliberate divergence. The case still appears in
	// the report — as a gap, not a pass — so nothing is quietly dropped.
	Skip string
}

// Suite collects cases and, on Run, measures them.
type Suite struct {
	t        *testing.T
	cfg      Config
	cases    []namedCase
	outcomes outcomeMap
}

type namedCase struct {
	name string
	c    Case
}

// New starts a suite. Defaults are filled in from t.
func New(t *testing.T, cfg Config) *Suite {
	t.Helper()
	if cfg.Name == "" {
		cfg.Name = t.Name()
	}
	if cfg.Repo == "" {
		cfg.Repo = "port"
	}
	if cfg.Tolerance == 0 {
		cfg.Tolerance = 1e-9
	}
	if cfg.GoldenDir == "" {
		cfg.GoldenDir = "testdata/parity"
	}
	if cfg.Package == "" {
		cfg.Package = "./..."
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Minute
	}
	return &Suite{t: t, cfg: cfg}
}

// Case adds a case. Names are the report's stable identifiers — a renamed case
// reads as one gap closed and one opened, so rename sparingly.
func (s *Suite) Case(name string, c Case) {
	if c.Go == nil {
		s.t.Fatalf("parity: case %q has no Go func", name)
	}
	s.cases = append(s.cases, namedCase{name, c})
}

// Run measures every case as a subtest, cross-compiles the port, writes the
// report and fails the test if the port disagrees with upstream anywhere.
func (s *Suite) Run() {
	t := s.t
	t.Helper()
	if len(s.cases) == 0 {
		t.Fatalf("parity: suite %q has no cases", s.cfg.Name)
	}
	start := time.Now()

	sess, mode, why := s.openUpstream()
	if sess != nil {
		defer sess.Close()
	}
	goldens, _ := loadGoldens(s.cfg.GoldenDir, s.cfg.Name)
	recording := map[string]Result{}

	rep := SuiteReport{
		Name:      s.cfg.Name,
		Upstream:  s.cfg.Upstream,
		Mode:      mode,
		Unmatched: why,
	}
	if s.cfg.Oracle != nil {
		rep.Oracle = s.cfg.Oracle.Info()
	}

	for _, nc := range s.cases {
		qualified := s.cfg.Name + "/" + nc.name
		rep.Names = append(rep.Names, qualified)
		rep.Counts.Total++

		t.Run(nc.name, func(t *testing.T) {
			s.measure(t, nc, sess, goldens, recording)
		})

		switch s.outcomes[nc.name] {
		case outcomeMatched:
			rep.Counts.Matched++
		case outcomeMismatched:
			rep.Counts.Mismatched++
			rep.Failing = append(rep.Failing, qualified)
		case outcomeErrored:
			rep.Counts.Errored++
			rep.Failing = append(rep.Failing, qualified)
		default:
			rep.Counts.Skipped++
			rep.Skipped = append(rep.Skipped, qualified)
		}
	}

	if len(recording) > 0 && recordingEnabled() {
		if err := saveGoldens(s.cfg.GoldenDir, s.cfg.Name, s.cfg.Upstream, rep.Oracle, recording); err != nil {
			t.Errorf("parity: recording goldens: %v", err)
		} else {
			t.Logf("parity: recorded %d upstream answers into %s", len(recording), goldenPath(s.cfg.GoldenDir, s.cfg.Name))
		}
	}

	if crossEnabled() && !testing.Short() {
		rep.Platforms = crossCompile(s.cfg.Package, s.cfg.Platforms)
		for _, p := range rep.Platforms {
			if !p.OK {
				t.Errorf("parity: port does not build for %s/%s: %s", p.GOOS, p.GOARCH, p.Error)
			}
		}
	}

	rep.Elapsed = time.Since(start).Round(time.Millisecond).String()
	if err := writeFragment(s.cfg.Repo, rep); err != nil {
		t.Errorf("parity: writing suite fragment: %v", err)
	}
	full, err := Assemble()
	if err != nil {
		t.Errorf("parity: assembling %s: %v", ReportPath(), err)
		return
	}
	t.Log("\n" + full.Summary())
}

// outcomes records each case's verdict, since a subtest's own pass/fail cannot
// distinguish "differs from upstream" from "upstream could not be asked".
type outcomeMap map[string]string

func (s *Suite) note(name, outcome string) {
	if s.outcomes == nil {
		s.outcomes = outcomeMap{}
	}
	s.outcomes[name] = outcome
}

// measure runs one case against both implementations.
func (s *Suite) measure(t *testing.T, nc namedCase, sess Session, goldens goldenFile, recording map[string]Result) {
	t.Helper()
	name, c := nc.name, nc.c

	if c.Skip != "" {
		s.note(name, outcomeSkipped)
		t.Skipf("parity gap (recorded, not hidden): %s", c.Skip)
	}

	want, source, err := s.upstreamAnswer(name, c, sess, goldens, recording)
	if err != nil {
		s.note(name, outcomeSkipped)
		t.Skipf("parity: no upstream answer available: %v", err)
	}

	got, goErr := callGo(c.Go)

	// Upstream refusing is a behavior of its own. A port that refuses too is at
	// parity; one that happily returns a value is not.
	if !want.OK {
		if goErr != nil {
			s.note(name, outcomeMatched)
			return
		}
		s.note(name, outcomeMismatched)
		t.Errorf("upstream (%s) raised %q but the port returned %s", source, want.Error, brief(got))
		return
	}
	if goErr != nil {
		s.note(name, outcomeErrored)
		t.Errorf("upstream (%s) returned %s but the port failed: %v", source, brief(decode(want.Value)), goErr)
		return
	}

	gotJSON, err := canonical(got)
	if err != nil {
		s.note(name, outcomeErrored)
		t.Errorf("parity: the port's value is not comparable: %v", err)
		return
	}
	tol := c.Tolerance
	if tol == 0 {
		tol = s.cfg.Tolerance
	}
	gotVal, wantVal := decode(gotJSON), decode(want.Value)
	if equal(gotVal, wantVal, tol) {
		s.note(name, outcomeMatched)
		return
	}
	s.note(name, outcomeMismatched)
	t.Errorf("differs from upstream (%s)\n  %s", source, diff(gotVal, wantVal, tol))
}

// upstreamAnswer gets the original library's answer: from the live library when
// it is running, from a recording when it is not, from the case's own Want when
// upstream cannot express the question at all.
func (s *Suite) upstreamAnswer(name string, c Case, sess Session, goldens goldenFile, recording map[string]Result) (Result, string, error) {
	if sess != nil && c.Upstream != "" {
		input, err := json.Marshal(c.Input)
		if err != nil {
			return Result{}, "", fmt.Errorf("encoding input: %w", err)
		}
		res, err := sess.Eval(Request{ID: name, Src: c.Upstream, Input: input})
		if err != nil {
			return Result{}, "", err
		}
		recording[name] = res
		return res, "live", nil
	}
	if res, ok := goldens.Cases[name]; ok {
		return res, "recorded", nil
	}
	if c.Want != nil {
		v, err := canonical(c.Want)
		if err != nil {
			return Result{}, "", err
		}
		return Result{OK: true, Value: v}, "declared", nil
	}
	return Result{}, "", errNoOracle
}

// openUpstream starts the original library, or explains why it could not.
func (s *Suite) openUpstream() (Session, string, string) {
	if s.cfg.Oracle == nil {
		return nil, ModeUnmeasured, "no upstream oracle configured"
	}
	if goldenOnly() {
		return nil, ModeGolden, "PARITY_GOLDEN is set"
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Timeout+installTimeout())
	sess, err := s.cfg.Oracle.Open(ctx)
	if err != nil {
		cancel()
		var reason string
		var un *Unavailable
		if errors.As(err, &un) {
			reason = un.Reason
		} else {
			reason = err.Error()
		}
		s.t.Logf("parity: running the original library is not possible here (%s); replaying recorded upstream answers", reason)
		return nil, ModeGolden, reason
	}
	s.t.Cleanup(cancel)
	return sess, ModeLive, ""
}

// callGo runs the port's side, turning a panic into an ordinary failure so one
// bad case cannot take the whole audit down with it.
func callGo(fn func() (any, error)) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v, err = nil, fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}

func decode(raw json.RawMessage) any {
	var v any
	if len(raw) == 0 {
		return nil
	}
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	return v
}

func recordingEnabled() bool { return truthy(os.Getenv("PARITY_RECORD")) }
func goldenOnly() bool       { return truthy(os.Getenv("PARITY_GOLDEN")) }

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
