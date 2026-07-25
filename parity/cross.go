package parity

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A port is only at parity with upstream where it actually runs, so every
// parity run also cross-compiles the package under test for the targets the
// family claims to support. A behavior score for code that does not build on
// windows/amd64 would be a score for something nobody can use there.

// Platform is a GOOS/GOARCH build target.
type Platform struct{ GOOS, GOARCH string }

// DefaultPlatforms are the targets every malcolmston/* port claims. They match
// the family's cross-compile workflow, so a parity failure here is the same
// failure that workflow would report — found earlier.
var DefaultPlatforms = []Platform{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"},
}

var (
	crossOnce sync.Once
	crossRes  []PlatformResult
)

// crossCompile builds pkg for each platform, once per test binary however many
// suites ask for it. CGO is off: a port that needs a C toolchain per target is
// not portable in the sense this claim makes.
func crossCompile(pkg string, platforms []Platform) []PlatformResult {
	crossOnce.Do(func() {
		if len(platforms) == 0 {
			platforms = DefaultPlatforms
		}
		if _, err := exec.LookPath("go"); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		results := make([]PlatformResult, len(platforms))
		var wg sync.WaitGroup
		for i, p := range platforms {
			// The host's own target is already proven by this test binary
			// existing; building it again would just burn cache lookups.
			if p.GOOS == runtime.GOOS && p.GOARCH == runtime.GOARCH {
				results[i] = PlatformResult{GOOS: p.GOOS, GOARCH: p.GOARCH, OK: true}
				continue
			}
			wg.Add(1)
			go func(i int, p Platform) {
				defer wg.Done()
				results[i] = buildFor(ctx, pkg, p)
			}(i, p)
		}
		wg.Wait()
		crossRes = results
	})
	return crossRes
}

func buildFor(ctx context.Context, pkg string, p Platform) PlatformResult {
	res := PlatformResult{GOOS: p.GOOS, GOARCH: p.GOARCH}
	cmd := exec.CommandContext(ctx, "go", "build", pkg)
	cmd.Env = append(os.Environ(), "GOOS="+p.GOOS, "GOARCH="+p.GOARCH, "CGO_ENABLED=0")
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		res.Error = firstLine(out.String())
		if res.Error == "" {
			res.Error = err.Error()
		}
		return res
	}
	res.OK = true
	return res
}

// crossEnabled reports whether this run should cross-compile. It is on by
// default — the claim is not worth making unproven — and off for -short runs
// and when PARITY_CROSS is set to a false value.
func crossEnabled() bool {
	switch strings.ToLower(os.Getenv("PARITY_CROSS")) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}
