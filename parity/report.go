package parity

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the parity.json schema this harness emits. Version 1 was a
// hand-maintained snapshot; version 2 is measured — every number below is
// produced by running the upstream library and the port side by side.
const SchemaVersion = 2

// Report is a repo's parity.json: what was compared against what, how it went,
// and where the port still differs from upstream. The landing aggregates these
// (see ../genparity), so the fields version 1 published are kept.
type Report struct {
	Schema      int    `json:"schema"`
	Repo        string `json:"repo"`
	Upstream    string `json:"upstream"`
	GeneratedAt string `json:"generatedAt"`
	Go          string `json:"go"`

	Oracle OracleInfo `json:"oracle"`
	Mode   string     `json:"mode"` // live | golden | mixed | unmeasured
	Cases  Counts     `json:"cases"`

	// v1-compatible fields.
	ParityBefore        string `json:"parityBefore"`
	ParityAfter         string `json:"parityAfter"`
	UpstreamCasesSynced int    `json:"upstreamCasesSynced"`
	GapsClosed          int    `json:"gapsClosed"`

	Baseline    string           `json:"baseline"` // measured | none
	Failing     []string         `json:"failing"`
	Regressions []string         `json:"regressions"`
	Suites      []SuiteReport    `json:"suites"`
	Platforms   []PlatformResult `json:"platforms"`
}

// Counts is the case tally a score is computed from. Skipped cases (upstream
// behavior the port knowingly does not implement) are counted and named but
// left out of the score's denominator only when explicitly excused; unexcused
// skips count against the port.
type Counts struct {
	Total      int `json:"total"`
	Matched    int `json:"matched"`
	Mismatched int `json:"mismatched"`
	Skipped    int `json:"skipped"`
	Errored    int `json:"errored"`
}

// SuiteReport is one Suite's contribution — a repo may audit several upstream
// entry points (a sub-package each) in one run.
type SuiteReport struct {
	Name      string     `json:"name"`
	Upstream  string     `json:"upstream"`
	Oracle    OracleInfo `json:"oracle"`
	Mode      string     `json:"mode"`
	Counts    Counts     `json:"counts"`
	Names     []string   `json:"caseNames"`
	Failing   []string   `json:"failing"`
	Skipped   []string   `json:"skipped"`
	Unmatched string     `json:"unavailableReason,omitempty"`
	Elapsed   string     `json:"elapsed"`

	Platforms []PlatformResult `json:"platforms,omitempty"`
}

// PlatformResult records that the port compiles for a target — parity is a
// claim about behavior everywhere it ships, so every parity run cross-compiles.
type PlatformResult struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Case outcomes.
const (
	outcomeMatched    = "matched"
	outcomeMismatched = "mismatched"
	outcomeSkipped    = "skipped"
	outcomeErrored    = "errored"
)

// Oracle modes.
const (
	ModeLive       = "live"       // answers came from the upstream library itself
	ModeGolden     = "golden"     // answers replayed from recorded upstream output
	ModeMixed      = "mixed"      // some of each, across suites
	ModeUnmeasured = "unmeasured" // no upstream answers available at all
)

// FragmentDir is where each test binary drops its suite fragment before they
// are assembled into parity.json. `go test ./...` runs one process per package,
// so the fragments are how several packages' suites end up in one report.
func FragmentDir() string {
	if d := os.Getenv("PARITY_DIR"); d != "" {
		return d
	}
	return ".parity"
}

// ReportPath is the assembled report's path.
func ReportPath() string {
	if p := os.Getenv("PARITY_OUT"); p != "" {
		return p
	}
	return "parity.json"
}

// runOnces keys the once-per-run preparation by fragment directory, so a
// process auditing into more than one directory prepares each of them.
var runOnces sync.Map

// beginRun makes the fragment directory ready for this `go test` invocation:
// fragments left by an earlier run are cleared, and the committed parity.json
// is preserved as the baseline the new run is scored against.
func beginRun() {
	dir := FragmentDir()
	once, _ := runOnces.LoadOrStore(dir, new(sync.Once))
	once.(*sync.Once).Do(func() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		// All package test binaries of one `go test` invocation share the same
		// parent process, which makes ppid a serviceable run identifier.
		id := fmt.Sprintf("%d %d", os.Getppid(), time.Now().Unix())
		stampPath := filepath.Join(dir, "run")
		if prev, err := os.ReadFile(stampPath); err == nil && sameRun(string(prev), id) {
			return
		}
		for _, e := range dirEntries(dir) {
			if strings.HasSuffix(e, ".json") {
				os.Remove(filepath.Join(dir, e))
			}
		}
		if b, err := os.ReadFile(ReportPath()); err == nil {
			os.WriteFile(filepath.Join(dir, "baseline"), b, 0o644)
		} else {
			os.Remove(filepath.Join(dir, "baseline"))
		}
		os.WriteFile(stampPath, []byte(id), 0o644)
	})
}

// sameRun reports whether a stamp belongs to the invocation running now: same
// parent process, and recent enough that a recycled pid cannot be mistaken for
// a continuation.
func sameRun(prev, cur string) bool {
	pp, pt, ok := strings.Cut(strings.TrimSpace(prev), " ")
	cp, _, _ := strings.Cut(cur, " ")
	if !ok || pp != cp {
		return false
	}
	sec, err := strconv.ParseInt(pt, 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(sec, 0)) < 6*time.Hour
}

func dirEntries(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// baseline is the previous run's report, used to tell a gap that was closed
// from one that was never open.
func baseline() (Report, bool) {
	b, err := os.ReadFile(filepath.Join(FragmentDir(), "baseline"))
	if err != nil {
		return Report{}, false
	}
	var r Report
	if json.Unmarshal(b, &r) != nil {
		return Report{}, false
	}
	return r, true
}

func writeFragment(repo string, sr SuiteReport) error {
	beginRun()
	name := slug(repo) + "." + slug(sr.Name) + ".json"
	b, err := json.MarshalIndent(fragment{Repo: repo, Suite: sr}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(FragmentDir(), name), append(b, '\n'), 0o644)
}

type fragment struct {
	Repo  string      `json:"repo"`
	Suite SuiteReport `json:"suite"`
}

// Assemble merges every suite fragment of the current run into one report and
// writes it to ReportPath. It is safe to call repeatedly — each suite calls it
// as it finishes, so a partial parity.json exists even if a later suite dies.
func Assemble() (Report, error) {
	beginRun()
	dir := FragmentDir()
	var frags []fragment
	for _, name := range dirEntries(dir) {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var f fragment
		if json.Unmarshal(b, &f) == nil && f.Suite.Name != "" {
			frags = append(frags, f)
		}
	}
	if len(frags) == 0 {
		return Report{}, fmt.Errorf("parity: no suite fragments in %s — did any TestParity run?", dir)
	}
	sort.Slice(frags, func(i, j int) bool { return frags[i].Suite.Name < frags[j].Suite.Name })

	rep := build(frags)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return rep, err
	}
	return rep, os.WriteFile(ReportPath(), append(b, '\n'), 0o644)
}

func build(frags []fragment) Report {
	rep := Report{
		Schema:      SchemaVersion,
		Repo:        frags[0].Repo,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Go:          runtime.Version(),
		Baseline:    "none",
	}
	modes := map[string]bool{}
	for _, f := range frags {
		s := f.Suite
		rep.Suites = append(rep.Suites, s)
		rep.Cases.Total += s.Counts.Total
		rep.Cases.Matched += s.Counts.Matched
		rep.Cases.Mismatched += s.Counts.Mismatched
		rep.Cases.Skipped += s.Counts.Skipped
		rep.Cases.Errored += s.Counts.Errored
		rep.Failing = append(rep.Failing, s.Failing...)
		if rep.Upstream == "" {
			rep.Upstream = s.Upstream
		}
		if rep.Oracle.Runtime == "" {
			rep.Oracle = s.Oracle
		}
		modes[s.Mode] = true
		if len(rep.Platforms) == 0 {
			rep.Platforms = s.Platforms
		}
	}
	sort.Strings(rep.Failing)
	rep.Mode = mergeModes(modes)
	rep.UpstreamCasesSynced = rep.Cases.Total

	// after: today's measured agreement with upstream.
	compared := rep.Cases.Matched + rep.Cases.Mismatched + rep.Cases.Errored
	rep.ParityAfter = percent(rep.Cases.Matched, compared)

	// before: the same case set scored with the previous run's failures, so
	// "before → after" is exactly the movement this run is responsible for.
	rep.ParityBefore = rep.ParityAfter
	if base, ok := baseline(); ok {
		rep.Baseline = "measured"
		was := map[string]bool{}
		for _, name := range base.Failing {
			was[name] = true
		}
		now := map[string]bool{}
		for _, name := range rep.Failing {
			now[name] = true
		}
		wasFailing := 0
		for name := range now {
			if !was[name] {
				rep.Regressions = append(rep.Regressions, name)
			}
		}
		sort.Strings(rep.Regressions)
		for _, name := range allCaseNames(rep.Suites) {
			if was[name] {
				wasFailing++
			}
		}
		rep.ParityBefore = percent(compared-wasFailing, compared)
		for name := range was {
			if !now[name] && isKnownCase(rep.Suites, name) {
				rep.GapsClosed++
			}
		}
	}
	return rep
}

// allCaseNames is every case this run compared. Scoring "before" over exactly
// today's case set is what makes before → after a statement about the port
// rather than about which cases happened to be synced when.
func allCaseNames(suites []SuiteReport) []string {
	var out []string
	for _, s := range suites {
		out = append(out, s.Names...)
	}
	return out
}

// isKnownCase reports whether a baseline failure was re-measured this run, so a
// gap only counts as closed when the case that exposed it actually ran again.
func isKnownCase(suites []SuiteReport, name string) bool {
	for _, s := range suites {
		for _, n := range s.Names {
			if n == name {
				return true
			}
		}
	}
	return false
}

func mergeModes(modes map[string]bool) string {
	switch {
	case len(modes) == 1:
		for m := range modes {
			return m
		}
		return ModeUnmeasured
	case modes[ModeLive] || modes[ModeGolden]:
		return ModeMixed
	default:
		return ModeUnmeasured
	}
}

func percent(n, total int) string {
	if total <= 0 {
		return "n/a"
	}
	if n >= total {
		return "100%"
	}
	// Floor, so a port one case short never advertises a round 100%.
	return strconv.Itoa(int(math.Floor(float64(n)/float64(total)*100))) + "%"
}

// Summary is the one-screen human rendering of a report, printed at the end of
// a parity run and into the CI job summary.
func (r Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "parity: %s vs %s — %s (was %s), %d/%d cases matched, mode=%s\n",
		r.Repo, r.Upstream, r.ParityAfter, r.ParityBefore, r.Cases.Matched, r.Cases.Total, r.Mode)
	if r.Oracle.Runtime != "" {
		fmt.Fprintf(&b, "  upstream: %s %s (%s)\n", r.Oracle.Runtime, r.Oracle.Package, r.Oracle.Version)
	}
	for _, s := range r.Suites {
		fmt.Fprintf(&b, "  %-28s %3d matched  %3d mismatched  %3d skipped  %3d errored  (%s, %s)\n",
			s.Name, s.Counts.Matched, s.Counts.Mismatched, s.Counts.Skipped, s.Counts.Errored, s.Mode, s.Elapsed)
		if s.Unmatched != "" {
			fmt.Fprintf(&b, "      upstream unavailable: %s\n", s.Unmatched)
		}
	}
	if len(r.Failing) > 0 {
		fmt.Fprintf(&b, "  still differing from upstream (%d): %s\n", len(r.Failing), strings.Join(clip(r.Failing, 20), ", "))
	}
	if len(r.Regressions) > 0 {
		fmt.Fprintf(&b, "  REGRESSIONS (%d): %s\n", len(r.Regressions), strings.Join(clip(r.Regressions, 20), ", "))
	}
	if len(r.Platforms) > 0 {
		var bad []string
		for _, p := range r.Platforms {
			if !p.OK {
				bad = append(bad, p.GOOS+"/"+p.GOARCH)
			}
		}
		if len(bad) == 0 {
			fmt.Fprintf(&b, "  cross-compiles for all %d targets\n", len(r.Platforms))
		} else {
			fmt.Fprintf(&b, "  FAILS TO BUILD for: %s\n", strings.Join(bad, ", "))
		}
	}
	return b.String()
}

func clip(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return append(append([]string{}, s[:n]...), fmt.Sprintf("… +%d more", len(s)-n))
}
