package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func init() { register(&tailwind{}) }

// tailwindRepo is the upstream project the standalone binary is released from.
const tailwindRepo = "tailwindlabs/tailwindcss"

// tailwind wires Tailwind CSS into a Go react project using the standalone
// binary — a single self-contained executable that needs no Node, no npm and no
// node_modules.
//
// The part worth understanding is how Tailwind finds classes in a Go codebase.
// Tailwind's extractor does not parse a language; it scans files for candidate
// class-name substrings. Class names in this port live in ordinary Go string
// literals (react.Props{"className": "flex gap-2"}), which the extractor finds
// exactly as it finds them in JSX. The one requirement is telling Tailwind to
// look at .go files at all, which the generated stylesheet does with an
// @source directive.
//
// The consequence is the same one JSX has: a class name assembled at runtime
// from pieces ("bg-" + colour) is invisible to the scanner. Write whole class
// names in literals, or list the dynamic ones in a @source inline directive.
type tailwind struct{}

func (t *tailwind) Name() string { return "tailwind" }

func (t *tailwind) Summary() string {
	return "Tailwind CSS via the standalone binary — no Node, no npm, no node_modules"
}

// binaryName is the release asset for the current platform. Tailwind publishes
// one binary per OS/arch pair under names that do not match Go's GOOS/GOARCH
// spelling, so the mapping is explicit.
func (t *tailwind) binaryName() (string, error) {
	var osPart string
	switch runtime.GOOS {
	case "darwin":
		osPart = "macos"
	case "linux":
		osPart = "linux"
	case "windows":
		osPart = "windows"
	default:
		return "", fmt.Errorf("Tailwind does not publish a standalone binary for %s; "+
			"install it yourself and put it on PATH", runtime.GOOS)
	}

	var archPart string
	switch runtime.GOARCH {
	case "amd64":
		archPart = "x64"
	case "arm64":
		archPart = "arm64"
	default:
		return "", fmt.Errorf("Tailwind does not publish a standalone binary for %s/%s",
			runtime.GOOS, runtime.GOARCH)
	}

	name := fmt.Sprintf("tailwindcss-%s-%s", osPart, archPart)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name, nil
}

// binaryPath is where the pinned binary lives inside the project.
func (t *tailwind) binaryPath(m interface{ ToolsDir() string }) string {
	name := "tailwindcss"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.ToolsDir(), name)
}

// Version is the version to install; empty means "resolve the latest release
// now and pin it".
var TailwindVersion string

func (t *tailwind) Install(ctx context.Context, env Env) (Entry, error) {
	entry := Entry{
		Input:  "styles/app.css",
		Output: filepath.ToSlash(filepath.Join(env.Manifest.Out, "assets", "app.css")),
	}

	version := TailwindVersion
	if version == "" {
		env.Printer.Step("resolving the latest Tailwind release")
		resolved, err := latestTag(ctx, tailwindRepo)
		if err != nil {
			return entry, err
		}
		version = resolved
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	entry.Version = version
	env.Printer.Success("Tailwind %s", version)

	asset, err := t.binaryName()
	if err != nil {
		return entry, err
	}

	dest := t.binaryPath(env.Manifest)
	if _, err := os.Stat(dest); err == nil {
		env.Printer.Success("binary already present at %s", dest)
	} else {
		base := fmt.Sprintf("https://github.com/%s/releases/download/%s", tailwindRepo, version)

		env.Printer.Step("verifying checksums")
		sums, err := checksums(ctx, base+"/sha256sums.txt")
		if err != nil {
			return entry, err
		}
		want := sums[asset]
		if want == "" {
			// Proceed, but say so. Silently skipping verification would be the
			// worst of both worlds: no safety and no signal that there is none.
			env.Printer.Warn("no published checksum for %s; the download cannot be verified", asset)
		}

		env.Printer.Step("downloading %s", asset)
		if err := downloadBinary(ctx, base+"/"+asset, dest, want); err != nil {
			return entry, err
		}
		env.Printer.Success("installed %s", dest)
	}

	// The stylesheet entry point. @source is what points Tailwind's scanner at
	// Go files; without it the generated CSS would contain nothing but the
	// preflight, which is a genuinely baffling failure to debug.
	cssPath := env.Manifest.Path(entry.Input)
	if _, err := os.Stat(cssPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cssPath), 0o755); err != nil {
			return entry, err
		}
		if err := os.WriteFile(cssPath, []byte(tailwindEntryCSS), 0o644); err != nil {
			return entry, err
		}
		env.Printer.Success("created %s", cssPath)
	}

	return entry, nil
}

// Head contributes the stylesheet link. The href is the output path made
// site-absolute, since the build writes it under the output directory.
func (t *tailwind) Head(entry Entry) []string {
	href := "/" + strings.TrimPrefix(filepath.ToSlash(entry.Output), "dist/")
	return []string{
		fmt.Sprintf(`react.H("link", react.Props{"rel": "stylesheet", "href": %q})`, href),
	}
}

func (t *tailwind) Build(ctx context.Context, env Env, entry Entry) error {
	bin := t.binaryPath(env.Manifest)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("Tailwind is recorded in %s but its binary is missing from %s; "+
			"run `react install` to restore it", "react.json", env.Manifest.ToolsDir())
	}

	out := env.Manifest.Path(entry.Output)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin,
		"--input", env.Manifest.Path(entry.Input),
		"--output", out,
		"--minify",
	)
	cmd.Dir = env.Manifest.Root()

	// Tailwind writes its progress banner to stderr. Surfacing it only on
	// failure keeps a successful build quiet while leaving a broken stylesheet
	// fully diagnosable.
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailwind failed: %w\n%s", err, strings.TrimSpace(string(combined)))
	}
	return nil
}

const tailwindEntryCSS = `@import "tailwindcss";

/* Tailwind scans files for class-name candidates rather than parsing a
   language, so pointing it at Go sources is all it takes to pick up class names
   written in react.Props literals:

       react.H("div", react.Props{"className": "flex items-center gap-2"}, ...)

   As in JSX, a class name assembled at runtime ("bg-" + colour) is invisible to
   the scanner. Write whole names, or declare the dynamic ones with
   @source inline("bg-red-500 bg-blue-500"). */
@source "./**/*.go";
`
