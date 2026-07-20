// Command gendocs generates a machine-readable Go API reference (a DocIndex
// JSON, the same schema the shared go-ui DocsApp renderer consumes) for every
// library submodule vendored into this aggregator repo. It writes one
// web/public/docs/<lib>.json per submodule so the /go landing can render each
// library's FULL API docs inline — package-by-package types, functions,
// methods, constants and examples — rather than only linking out to the
// per-library site.
//
// Usage (from the repo root):
//
//	go run ./gendocs                        # -> web/public/docs/<lib>.json for all submodules
//	go run ./gendocs -root . -out web/public/docs
//
// It walks each submodule with go/doc + go/parser and is dependency-free,
// matching the family's standard-library-only ethos. Submodules without a
// go.mod (or that fail to parse) are skipped with a warning rather than
// aborting the whole run.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", ".", "aggregator repo root (contains .gitmodules and the library submodules)")
	out := flag.String("out", filepath.Join("public", "docs"), "output directory for the per-library <lib>.json files")
	flag.Parse()

	subs, err := submodulePaths(filepath.Join(*root, ".gitmodules"))
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	var manifest []manifestEntry
	wrote := 0
	for _, sub := range subs {
		dir := filepath.Join(*root, sub)
		modFile := filepath.Join(dir, "go.mod")
		modPath, err := modulePath(modFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gendocs: skip %s: %v\n", sub, err)
			continue
		}
		pkgs, err := collectPackages(dir, modPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gendocs: skip %s: %v\n", sub, err)
			continue
		}
		sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].ImportPath < pkgs[j].ImportPath })
		// The file key is the submodule directory name (e.g. "socket.io"), which
		// matches the last path segment of each library's GitHub repo URL — the
		// key LibView derives to fetch this file.
		key := filepath.Base(sub)
		outPath := filepath.Join(*out, key+".json")
		if err := writeJSON(outPath, modPath, pkgs); err != nil {
			fatal(err)
		}
		sym := 0
		for _, p := range pkgs {
			sym += p.symbolCount()
		}
		manifest = append(manifest, manifestEntry{Lib: key, Module: modPath, Packages: len(pkgs), Symbols: sym})
		fmt.Printf("gendocs: %-14s %3d packages -> %s\n", key, len(pkgs), outPath)
		wrote++
	}

	// A small manifest so the site (and humans) can see coverage at a glance.
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Lib < manifest[j].Lib })
	if data, err := json.MarshalIndent(map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"libraries":   manifest,
	}, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(*out, "index.json"), data, 0o644)
	}
	fmt.Printf("gendocs: wrote %d library doc indexes to %s\n", wrote, *out)
}

type manifestEntry struct {
	Lib      string `json:"lib"`
	Module   string `json:"module"`
	Packages int    `json:"packages"`
	Symbols  int    `json:"symbols"`
}

// submodulePaths parses the `path = ...` entries out of a .gitmodules file.
func submodulePaths(gitmodules string) ([]string, error) {
	f, err := os.Open(gitmodules)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var paths []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "path") {
			if i := strings.IndexByte(line, '='); i >= 0 {
				p := strings.TrimSpace(line[i+1:])
				if p != "" {
					paths = append(paths, p)
				}
			}
		}
	}
	return paths, sc.Err()
}

// ---- package collection (go/doc) --------------------------------------------

// pkgInfo is a documented package.
type pkgInfo struct {
	ImportPath string
	Rel        string
	Doc        *doc.Package
	Fset       *token.FileSet
}

func (p pkgInfo) Synopsis() string {
	if p.Doc == nil {
		return ""
	}
	return doc.Synopsis(p.Doc.Doc)
}

func (p pkgInfo) Name() string {
	if p.Doc != nil {
		return p.Doc.Name
	}
	return filepath.Base(p.ImportPath)
}

func (p pkgInfo) symbolCount() int {
	if p.Doc == nil {
		return 0
	}
	n := len(p.Doc.Consts) + len(p.Doc.Vars) + len(p.Doc.Funcs)
	for _, t := range p.Doc.Types {
		n += 1 + len(t.Consts) + len(t.Vars) + len(t.Funcs) + len(t.Methods)
	}
	return n
}

func collectPackages(root, modPath string) ([]pkgInfo, error) {
	var pkgs []pkgInfo
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		base := info.Name()
		if path != root && (base == "testdata" || base == "_site" || base == ".git" ||
			strings.HasPrefix(base, ".") || base == "node_modules" ||
			base == "web" || base == "vendor" || base == "docs" || base == "examples") {
			return filepath.SkipDir
		}
		p, ok := parsePackage(path, root, modPath)
		if ok {
			pkgs = append(pkgs, p)
		}
		return nil
	})
	return pkgs, err
}

func parsePackage(dir, root, modPath string) (pkgInfo, bool) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil || len(parsed) == 0 {
		return pkgInfo{}, false
	}
	var primary string
	for name := range parsed {
		if !strings.HasSuffix(name, "_test") {
			primary = name
			break
		}
	}
	if primary == "" {
		return pkgInfo{}, false
	}
	var files []*ast.File
	for name, ap := range parsed {
		if name != primary && name != primary+"_test" {
			continue
		}
		for _, f := range ap.Files {
			files = append(files, f)
		}
	}
	rel, _ := filepath.Rel(root, dir)
	if rel == "." {
		rel = ""
	}
	importPath := modPath
	if rel != "" {
		importPath = modPath + "/" + filepath.ToSlash(rel)
	}
	dpkg, err := doc.NewFromFiles(fset, files, importPath)
	if err != nil {
		dpkg = doc.New(parsed[primary], importPath, 0)
	}
	return pkgInfo{ImportPath: importPath, Rel: rel, Doc: dpkg, Fset: fset}, true
}

func modulePath(goMod string) (string, error) {
	f, err := os.Open(goMod)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("no module directive in %s", goMod)
}

// ---- JSON emit (mirrors ui/src/docs/types.ts) -------------------------------

type jsonIndex struct {
	Module      string        `json:"module"`
	GeneratedAt string        `json:"generatedAt,omitempty"`
	Packages    []jsonPackage `json:"packages"`
}

type jsonPackage struct {
	ImportPath string        `json:"importPath"`
	Name       string        `json:"name"`
	Synopsis   string        `json:"synopsis"`
	Doc        string        `json:"doc"`
	IsCommand  bool          `json:"isCommand,omitempty"`
	Consts     []jsonValue   `json:"consts"`
	Vars       []jsonValue   `json:"vars"`
	Types      []jsonType    `json:"types"`
	Funcs      []jsonFunc    `json:"funcs"`
	Examples   []jsonExample `json:"examples,omitempty"`
}

type jsonExample struct {
	Name   string `json:"name"`
	Code   string `json:"code"`
	Output string `json:"output,omitempty"`
	Doc    string `json:"doc,omitempty"`
}

type jsonType struct {
	Name      string      `json:"name"`
	Signature string      `json:"signature"`
	Doc       string      `json:"doc"`
	Consts    []jsonValue `json:"consts"`
	Vars      []jsonValue `json:"vars"`
	Funcs     []jsonFunc  `json:"funcs"`
	Methods   []jsonFunc  `json:"methods"`
}

type jsonFunc struct {
	Name      string `json:"name"`
	Recv      string `json:"recv,omitempty"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
}

type jsonValue struct {
	Names     []string `json:"names"`
	Signature string   `json:"signature"`
	Doc       string   `json:"doc"`
}

func writeJSON(path, modPath string, pkgs []pkgInfo) error {
	idx := jsonIndex{
		Module:      modPath,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Packages:    make([]jsonPackage, 0, len(pkgs)),
	}
	for _, p := range pkgs {
		idx.Packages = append(idx.Packages, jsonPackageOf(p))
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func jsonPackageOf(p pkgInfo) jsonPackage {
	jp := jsonPackage{
		ImportPath: p.ImportPath,
		Name:       p.Name(),
		Synopsis:   p.Synopsis(),
		Consts:     make([]jsonValue, 0),
		Vars:       make([]jsonValue, 0),
		Types:      make([]jsonType, 0),
		Funcs:      make([]jsonFunc, 0),
	}
	if p.Doc == nil {
		return jp
	}
	jp.Doc = p.Doc.Doc
	jp.IsCommand = p.Doc.Name == "main"
	jp.Consts = jsonValuesOf(p, p.Doc.Consts)
	jp.Vars = jsonValuesOf(p, p.Doc.Vars)
	jp.Funcs = jsonFuncsOf(p, p.Doc.Funcs)
	for _, t := range p.Doc.Types {
		jp.Types = append(jp.Types, jsonType{
			Name:      t.Name,
			Signature: nodeString(p.Fset, t.Decl),
			Doc:       t.Doc,
			Consts:    jsonValuesOf(p, t.Consts),
			Vars:      jsonValuesOf(p, t.Vars),
			Funcs:     jsonFuncsOf(p, t.Funcs),
			Methods:   jsonFuncsOf(p, t.Methods),
		})
	}
	jp.Examples = collectExamples(p)
	return jp
}

func collectExamples(p pkgInfo) []jsonExample {
	if p.Doc == nil {
		return nil
	}
	var out []jsonExample
	seen := map[string]bool{}
	add := func(exs []*doc.Example) {
		for _, ex := range exs {
			code := exampleCode(p.Fset, ex)
			if strings.TrimSpace(code) == "" || seen[code] {
				continue
			}
			seen[code] = true
			out = append(out, jsonExample{
				Name:   exampleTitle(ex.Name),
				Code:   code,
				Output: strings.TrimRight(ex.Output, "\n"),
				Doc:    ex.Doc,
			})
		}
	}
	add(p.Doc.Examples)
	for _, f := range p.Doc.Funcs {
		add(f.Examples)
	}
	for _, t := range p.Doc.Types {
		add(t.Examples)
		for _, f := range t.Funcs {
			add(f.Examples)
		}
		for _, m := range t.Methods {
			add(m.Examples)
		}
	}
	return out
}

func exampleTitle(name string) string {
	if name == "" {
		return ""
	}
	if i := strings.IndexByte(name, '_'); i >= 0 {
		base, suffix := name[:i], strings.ReplaceAll(name[i+1:], "_", " ")
		if base == "" {
			return suffix
		}
		return base + " (" + suffix + ")"
	}
	return name
}

func exampleCode(fset *token.FileSet, ex *doc.Example) string {
	var buf bytes.Buffer
	if ex.Play != nil {
		if err := format.Node(&buf, fset, ex.Play); err == nil {
			return strings.TrimSpace(buf.String())
		}
		buf.Reset()
	}
	if ex.Code != nil {
		if err := format.Node(&buf, fset, ex.Code); err == nil {
			return dedentBlock(buf.String())
		}
	}
	return ""
}

func dedentBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "{"), "}")
		lines := strings.Split(s, "\n")
		for i, ln := range lines {
			lines[i] = strings.TrimPrefix(ln, "\t")
		}
		s = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return s
}

func jsonValuesOf(p pkgInfo, vals []*doc.Value) []jsonValue {
	out := make([]jsonValue, 0, len(vals))
	for _, v := range vals {
		out = append(out, jsonValue{
			Names:     v.Names,
			Signature: nodeString(p.Fset, v.Decl),
			Doc:       v.Doc,
		})
	}
	return out
}

func jsonFuncsOf(p pkgInfo, fns []*doc.Func) []jsonFunc {
	out := make([]jsonFunc, 0, len(fns))
	for _, fn := range fns {
		out = append(out, jsonFunc{
			Name:      fn.Name,
			Recv:      fn.Recv,
			Signature: oneLine(nodeString(p.Fset, fn.Decl)),
			Doc:       fn.Doc,
		})
	}
	return out
}

func nodeString(fset *token.FileSet, node any) string {
	var b strings.Builder
	if err := printerFprint(&b, fset, node); err != nil {
		return fmt.Sprintf("%v", node)
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func printerFprint(w io.Writer, fset *token.FileSet, node any) error {
	cfg := &printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	return cfg.Fprint(w, fset, node)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gendocs:", err)
	os.Exit(1)
}
