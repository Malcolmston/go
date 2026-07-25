// Command genparity aggregates the per-library parity.json files that each
// malcolmston/* port emits into the landing's src/parity.ts, so the go
// aggregator always reports the current upstream-parity numbers.
//
// Those files are measurements, not claims: each port's parity suite installs
// and runs the ORIGINAL library (see github.com/malcolmston/go/parity), asks it
// and the Go port the same questions, and records where they disagree. This
// command only collects the results.
//
// Run it from the repo root once the library submodules have been bumped to
// commits that contain a parity.json:
//
//	go run ./genparity
//
// It scans every top-level directory for a parity.json and rewrites
// src/parity.ts. Libraries without a parity.json are simply omitted.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// parityFile mirrors the schema a port writes to its parity.json. Fields the
// hand-maintained schema 1 lacked are optional, so a library whose port has not
// been re-audited yet still aggregates — with fewer columns, not wrong ones.
type parityFile struct {
	Schema      int    `json:"schema"`
	Repo        string `json:"repo"`
	Upstream    string `json:"upstream"`
	GeneratedAt string `json:"generatedAt"`
	Mode        string `json:"mode"`

	Oracle struct {
		Runtime string `json:"runtime"`
		Package string `json:"package"`
		Version string `json:"version"`
	} `json:"oracle"`

	Cases struct {
		Total      int `json:"total"`
		Matched    int `json:"matched"`
		Mismatched int `json:"mismatched"`
		Skipped    int `json:"skipped"`
		Errored    int `json:"errored"`
	} `json:"cases"`

	ParityBefore        string `json:"parityBefore"`
	ParityAfter         string `json:"parityAfter"`
	UpstreamCasesSynced int    `json:"upstreamCasesSynced"`
	GapsClosed          int    `json:"gapsClosed"`

	Failing   []string `json:"failing"`
	Platforms []struct {
		GOOS   string `json:"goos"`
		GOARCH string `json:"goarch"`
		OK     bool   `json:"ok"`
	} `json:"platforms"`
}

func main() {
	rootFlag := flag.String("root", ".", "directory holding the library repos/submodules (each with a parity.json)")
	outFlag := flag.String("out", "", "output path for parity.ts (default <root>/src/parity.ts)")
	flag.Parse()
	root := *rootFlag
	// Backward-compatible positional root argument.
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fatal(err)
	}
	type rec struct {
		id string
		p  parityFile
	}
	var recs []rec
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name(), "parity.json"))
		if err != nil {
			continue
		}
		var p parityFile
		if json.Unmarshal(b, &p) != nil {
			continue
		}
		id := p.Repo
		if id == "" {
			id = e.Name()
		}
		recs = append(recs, rec{id, p})
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].id < recs[j].id })

	var b strings.Builder
	b.WriteString(header)
	measured := 0
	for _, r := range recs {
		if r.p.Schema >= 2 {
			measured++
		}
		fmt.Fprintf(&b, "  %s: %s,\n", tsKey(r.id), entry(r.p))
	}
	b.WriteString("};\n")

	out := *outFlag
	if out == "" {
		out = filepath.Join(root, "src", "parity.ts")
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("genparity: wrote %s with %d libraries (%d measured against a live upstream)\n", out, len(recs), measured)
}

// entry renders one library's record. The five original fields are always
// present; the measured ones appear only for ports on the current schema, so
// the landing can tell a measurement from a legacy snapshot.
func entry(p parityFile) string {
	var f []string
	add := func(format string, a ...any) { f = append(f, fmt.Sprintf(format, a...)) }

	cases := p.UpstreamCasesSynced
	if cases == 0 {
		cases = p.Cases.Total
	}
	add("upstream: %q", p.Upstream)
	add("before: %q", or(p.ParityBefore, "n/a"))
	add("after: %q", or(p.ParityAfter, "n/a"))
	add("casesSynced: %d", cases)
	add("gapsClosed: %d", p.GapsClosed)

	if p.Schema < 2 {
		return "{ " + strings.Join(f, ", ") + " }"
	}
	add("mode: %q", or(p.Mode, "unmeasured"))
	if p.Oracle.Runtime != "" {
		add("runtime: %q", p.Oracle.Runtime)
	}
	if p.Oracle.Package != "" {
		add("upstreamPkg: %q", p.Oracle.Package)
	}
	add("matched: %d", p.Cases.Matched)
	add("differing: %d", p.Cases.Mismatched+p.Cases.Errored)
	add("skipped: %d", p.Cases.Skipped)
	if n := len(p.Platforms); n > 0 {
		built := 0
		for _, pl := range p.Platforms {
			if pl.OK {
				built++
			}
		}
		add("platforms: %d", n)
		add("platformsOK: %d", built)
	}
	if p.GeneratedAt != "" {
		add("measuredAt: %q", p.GeneratedAt)
	}
	return "{ " + strings.Join(f, ", ") + " }"
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// tsKey quotes an object key when it is not a plain JS identifier.
func tsKey(k string) string {
	plain := k != ""
	for i, r := range k {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		plain = false
		break
	}
	if plain {
		return k
	}
	return "'" + strings.ReplaceAll(k, "'", "\\'") + "'"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genparity:", err)
	os.Exit(1)
}

const header = `// Upstream-parity metrics per library, aggregated from each repo's parity.json.
//
// Each number is MEASURED, not asserted: the port's parity suite installs the
// original library — the real npm/PyPI/RubyGems package — runs it in its own
// runtime, asks it and the Go port the same questions, and records every
// disagreement. See github.com/malcolmston/go/parity.
//
// GENERATED by ` + "`go run ./genparity`" + ` — do not edit by hand.

export interface Parity {
  /** The original library this port is measured against, as a GitHub slug. */
  upstream: string;
  /** Score on the same case set using the previous run's failures. */
  before: string;
  /** Share of cases where the port agrees with upstream, measured now. */
  after: string;
  casesSynced: number;
  gapsClosed: number;

  // Present for ports on the measured schema (parity.json v2).

  /** live: upstream ran here. golden: its recorded answers were replayed. */
  mode?: 'live' | 'golden' | 'mixed' | 'unmeasured';
  /** Runtime the original library ran in: node, python, ruby, … */
  runtime?: string;
  /** The exact upstream package that answered, e.g. "chalk@5.3.0". */
  upstreamPkg?: string;
  matched?: number;
  differing?: number;
  skipped?: number;
  /** Cross-compile targets attempted, and how many built. */
  platforms?: number;
  platformsOK?: number;
  measuredAt?: string;
}

// PARITY is keyed by Lib.id.
export const PARITY: Record<string, Parity> = {
`
