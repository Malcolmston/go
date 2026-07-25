package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Running the original library is the point, but it cannot always be done: a
// laptop offline on a plane, a CI job without a Ruby toolchain, a machine where
// the upstream package no longer installs. Goldens are upstream's own answers,
// recorded on a run that did have it, so parity stays measurable — and stays
// honest, because the report says which mode produced each number.

// goldenFile is the on-disk record of one suite's upstream answers.
type goldenFile struct {
	Upstream   string            `json:"upstream"`
	Oracle     OracleInfo        `json:"oracle"`
	RecordedAt string            `json:"recordedAt"`
	Cases      map[string]Result `json:"cases"`
}

func goldenPath(dir, suite string) string {
	return filepath.Join(dir, slug(suite)+".json")
}

func loadGoldens(dir, suite string) (goldenFile, bool) {
	b, err := os.ReadFile(goldenPath(dir, suite))
	if err != nil {
		return goldenFile{}, false
	}
	var g goldenFile
	if json.Unmarshal(b, &g) != nil || g.Cases == nil {
		return goldenFile{}, false
	}
	return g, true
}

// saveGoldens rewrites a suite's recording. Results are keyed by case name and
// the file is written sorted, so re-recording produces a reviewable diff rather
// than a reshuffle.
func saveGoldens(dir, suite, upstream string, info OracleInfo, cases map[string]Result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ordered := make(map[string]Result, len(cases))
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r := cases[name]
		r.ID = "" // the id is a transport detail, not part of the record
		ordered[name] = r
	}
	b, err := json.MarshalIndent(goldenFile{
		Upstream:   upstream,
		Oracle:     info,
		RecordedAt: time.Now().UTC().Format(time.RFC3339),
		Cases:      ordered,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(goldenPath(dir, suite), append(b, '\n'), 0o644)
}
