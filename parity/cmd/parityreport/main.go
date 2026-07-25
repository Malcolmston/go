// Command parityreport turns the suite fragments a parity run leaves behind
// into the repo's parity.json, prints the human summary, and — with -check —
// fails the build when the port has drifted further from upstream than the
// last recorded run.
//
// A parity suite already assembles the report as it finishes, so this command
// exists for the cases where that is not enough: a suite that crashed, several
// packages audited in one CI job, or a pipeline that wants the markdown summary
// and the regression gate as their own step.
//
//	go run github.com/malcolmston/go/parity/cmd/parityreport@main -check -markdown "$GITHUB_STEP_SUMMARY"
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/go/parity"
)

func main() {
	check := flag.Bool("check", false, "exit non-zero if the port regressed against upstream or fails to cross-compile")
	markdown := flag.String("markdown", "", "also append a markdown summary to this file (e.g. $GITHUB_STEP_SUMMARY)")
	flag.Parse()

	rep, err := parity.Assemble()
	if err != nil {
		fmt.Fprintln(os.Stderr, "parityreport:", err)
		os.Exit(1)
	}
	fmt.Print(rep.Summary())

	if *markdown != "" {
		if err := appendMarkdown(*markdown, rep); err != nil {
			fmt.Fprintln(os.Stderr, "parityreport: writing markdown:", err)
		}
	}

	if !*check {
		return
	}
	var problems []string
	if len(rep.Regressions) > 0 {
		problems = append(problems, fmt.Sprintf("%d case(s) regressed: %s", len(rep.Regressions), strings.Join(rep.Regressions, ", ")))
	}
	for _, p := range rep.Platforms {
		if !p.OK {
			problems = append(problems, fmt.Sprintf("does not build for %s/%s", p.GOOS, p.GOARCH))
		}
	}
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "parityreport: "+strings.Join(problems, "; "))
		os.Exit(1)
	}
}

// appendMarkdown renders the report for a CI job summary: the headline, the
// per-suite tally, and the named gaps — the same facts as parity.json, in the
// shape a reviewer reads first.
func appendMarkdown(path string, rep parity.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "## Upstream parity — %s vs `%s`\n\n", rep.Repo, rep.Upstream)
	fmt.Fprintf(&b, "**%s** (was %s) · %d/%d cases matched · mode `%s`",
		rep.ParityAfter, rep.ParityBefore, rep.Cases.Matched, rep.Cases.Total, rep.Mode)
	if rep.Oracle.Runtime != "" {
		fmt.Fprintf(&b, " · upstream `%s %s` on `%s`", rep.Oracle.Runtime, rep.Oracle.Package, rep.Oracle.Version)
	}
	b.WriteString("\n\n")

	b.WriteString("| suite | matched | differs | skipped | errored | mode |\n|---|--:|--:|--:|--:|---|\n")
	for _, s := range rep.Suites {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %s |\n", s.Name,
			s.Counts.Matched, s.Counts.Mismatched, s.Counts.Skipped, s.Counts.Errored, s.Mode)
	}
	b.WriteString("\n")

	if len(rep.Platforms) > 0 {
		var targets []string
		for _, p := range rep.Platforms {
			mark := "✅"
			if !p.OK {
				mark = "❌"
			}
			targets = append(targets, fmt.Sprintf("%s `%s/%s`", mark, p.GOOS, p.GOARCH))
		}
		fmt.Fprintf(&b, "Cross-compile: %s\n\n", strings.Join(targets, " · "))
	}
	if len(rep.Regressions) > 0 {
		fmt.Fprintf(&b, "### ⚠️ Regressions\n\n%s\n\n", list(rep.Regressions))
	}
	if len(rep.Failing) > 0 {
		fmt.Fprintf(&b, "### Still differing from upstream\n\n%s\n", list(rep.Failing))
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(b.String())
	return err
}

func list(names []string) string {
	var b strings.Builder
	for i, n := range names {
		if i == 40 {
			fmt.Fprintf(&b, "- … and %d more (see `parity.json`)\n", len(names)-i)
			break
		}
		fmt.Fprintf(&b, "- `%s`\n", n)
	}
	return b.String()
}
