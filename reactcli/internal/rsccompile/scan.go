// Package rsccompile is the "use client" compiler: it finds the JavaScript
// modules a project marks as client components and generates the two things
// that have to agree about them — the Go declarations the server references
// them by, and the browser entry that loads them.
//
// # Why a compiler is needed at all
//
// The Go server has to name a client component in a way the browser can later
// resolve: a module id, the chunks to load first, and the export to pick. Written
// by hand, those strings live in two places and drift the moment a file is
// renamed — and the failure mode is a blank component at runtime, not a build
// error. Deriving them from the source removes the possibility.
//
// # No bundler
//
// React's browser client passes a null bundler config, and its reference
// resolver then uses the module id and chunk list from the payload verbatim.
// Nothing consults a webpack manifest. So if the id and the chunk are both just
// the module's URL, and the two functions React calls to load it are shimmed
// with a dynamic import, client components work as plain ES modules — no
// webpack, no bundle step, no manifest file.
//
// The cost of that choice is that client components must be browser-ready
// JavaScript. A file containing JSX needs a transform first, from any tool; this
// package neither provides one nor pretends to.
package rsccompile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Module is one JavaScript module marked with a "use client" directive.
type Module struct {
	// Path is the absolute path of the source file.
	Path string

	// Rel is its path relative to the client root, with forward slashes. It is
	// what the browser URL is built from.
	Rel string

	// URL is the module's browser URL — the module id, the chunk to load, and
	// the import specifier, all at once. Keeping them the same value is what
	// removes the need for a manifest.
	URL string

	// Exports are the component names the module exports, sorted. "default" is
	// included when there is a default export.
	Exports []string
}

// Export pairs a module with one of its exported components.
type Export struct {
	Module Module

	// Name is the JavaScript export name, which may be "default".
	Name string

	// GoName is the identifier the generated Go declaration uses. For a default
	// export it is derived from the file name, since "default" is neither a
	// usable Go identifier nor a meaningful one.
	GoName string
}

// clientDirective matches a "use client" directive. It must be the first
// statement in the module, so only leading comments and blank lines may precede
// it — the same rule the JavaScript specification applies to directives.
// Go's regexp engine has no backreferences, so the two quote styles are spelled
// out rather than captured and matched.
var clientDirective = regexp.MustCompile(`^\s*(?:'use client'|"use client")\s*;?\s*$`)

// The export forms recognised. This is a lexical scan rather than a parse: a
// full JavaScript parser would be a large dependency for a job that in practice
// reads a handful of declaration lines, and every form it misses fails loudly
// at generation time rather than silently.
var (
	exportDecl        = regexp.MustCompile(`^\s*export\s+(?:async\s+)?(?:function\s*\*?|class|const|let|var)\s+([A-Za-z_$][\w$]*)`)
	exportDefaultFunc = regexp.MustCompile(`^\s*export\s+default\s+(?:async\s+)?(?:function\s*\*?|class)\s*([A-Za-z_$][\w$]*)?`)
	exportDefaultAny  = regexp.MustCompile(`^\s*export\s+default\s`)
	exportList        = regexp.MustCompile(`^\s*export\s*\{([^}]*)\}`)
	exportStar        = regexp.MustCompile(`^\s*export\s*\*`)
)

// sourceExtensions are the files considered. JSX and TypeScript extensions are
// scanned so that a project using them still gets a diagnostic rather than
// silence; see [Scan] for what happens next.
var sourceExtensions = map[string]bool{
	".js": true, ".mjs": true, ".jsx": true, ".ts": true, ".tsx": true,
}

// browserReady reports whether a browser can load this file without a
// transform.
func browserReady(ext string) bool { return ext == ".js" || ".mjs" == ext }

// Result is a completed scan.
type Result struct {
	// Modules are the client modules found, ordered by URL so that generated
	// output is stable across runs.
	Modules []Module

	// Warnings describe things worth telling the user that did not stop the
	// scan: a module needing a transform, an export form not understood.
	Warnings []string
}

// Exports flattens the result into one entry per exported component.
func (r Result) Exports() []Export {
	var out []Export
	for _, m := range r.Modules {
		for _, name := range m.Exports {
			out = append(out, Export{Module: m, Name: name, GoName: goName(m, name)})
		}
	}
	return out
}

// Scan walks root and returns every module carrying a "use client" directive.
//
// urlPrefix is the path the client directory is served under, so that a module
// at <root>/ui/Button.js with prefix "/client" gets the URL "/client/ui/Button.js".
func Scan(root, urlPrefix string) (Result, error) {
	var result Result

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			// A project with no client components is a valid project; it just
			// has nothing to compile.
			return result, nil
		}
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("rsccompile: %s is not a directory", root)
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", ".react":
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !sourceExtensions[ext] {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !hasClientDirective(string(source)) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if !browserReady(ext) {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"%s is a client component but %s cannot be loaded by a browser as-is; "+
					"compile it to .js first (esbuild, swc, tsc — any of them) and keep the "+
					"output under the client directory", rel, ext))
			return nil
		}

		exports, unknown := scanExports(string(source))
		for _, u := range unknown {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", rel, u))
		}
		if len(exports) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"%s is marked \"use client\" but exports nothing the scanner recognised", rel))
			return nil
		}

		result.Modules = append(result.Modules, Module{
			Path:    path,
			Rel:     rel,
			URL:     strings.TrimSuffix(urlPrefix, "/") + "/" + rel,
			Exports: exports,
		})
		return nil
	})
	if err != nil {
		return result, err
	}

	sort.Slice(result.Modules, func(i, j int) bool {
		return result.Modules[i].URL < result.Modules[j].URL
	})
	sort.Strings(result.Warnings)
	return result, nil
}

// hasClientDirective reports whether the module opens with "use client".
//
// The directive is only a directive in the prologue, so the search stops at the
// first line that is neither blank, a comment, nor a directive — a "use client"
// string appearing later in the file is just a string.
func hasClientDirective(source string) bool {
	inBlockComment := false

	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)

		if inBlockComment {
			if idx := strings.Index(trimmed, "*/"); idx >= 0 {
				inBlockComment = false
				trimmed = strings.TrimSpace(trimmed[idx+2:])
			} else {
				continue
			}
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
				continue
			}
			trimmed = strings.TrimSpace(trimmed[strings.Index(trimmed, "*/")+2:])
			if trimmed == "" {
				continue
			}
		}

		if clientDirective.MatchString(trimmed) {
			return true
		}
		// Another directive ("use strict") may precede it; anything else ends
		// the prologue.
		if strings.HasPrefix(trimmed, `"use `) || strings.HasPrefix(trimmed, `'use `) {
			continue
		}
		return false
	}
	return false
}

// scanExports collects exported names, returning the names and any diagnostics
// about forms it could not interpret.
func scanExports(source string) (names []string, unknown []string) {
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	for _, line := range strings.Split(source, "\n") {
		switch {
		case exportStar.MatchString(line):
			unknown = append(unknown, "`export *` cannot be followed without resolving the "+
				"re-exported module; name the components explicitly")

		case exportList.MatchString(line):
			inner := exportList.FindStringSubmatch(line)[1]
			for _, spec := range strings.Split(inner, ",") {
				spec = strings.TrimSpace(spec)
				if spec == "" {
					continue
				}
				// "A as B" exports B; the local name is irrelevant here.
				if parts := strings.Fields(spec); len(parts) == 3 && parts[1] == "as" {
					add(parts[2])
				} else {
					add(spec)
				}
			}

		case exportDefaultFunc.MatchString(line):
			add("default")

		case exportDecl.MatchString(line):
			add(exportDecl.FindStringSubmatch(line)[1])

		case exportDefaultAny.MatchString(line):
			add("default")
		}
	}

	sort.Strings(names)
	return names, unknown
}

// goName derives the Go identifier for an export.
func goName(m Module, export string) string {
	if export != "default" {
		return exportIdentifier(export)
	}
	// A default export is named after its file, which is the convention the
	// JavaScript side already follows when importing it.
	base := filepath.Base(m.Rel)
	return exportIdentifier(strings.TrimSuffix(base, filepath.Ext(base)))
}

// exportIdentifier turns a JavaScript name into an exported Go identifier.
func exportIdentifier(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			if upper {
				b.WriteRune(toUpper(r))
				upper = false
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if b.Len() == 0 {
				b.WriteRune('C')
			}
			b.WriteRune(r)
			upper = false
		default:
			upper = true
		}
	}
	if b.Len() == 0 {
		return "Client"
	}
	return b.String()
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}
