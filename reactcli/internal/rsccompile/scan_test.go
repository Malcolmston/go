package rsccompile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule creates one source file in a throwaway client directory.
func writeModule(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestClientDirectiveDetection(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"double quotes", "\"use client\";\nexport function A() {}\n", true},
		{"single quotes", "'use client';\nexport function A() {}\n", true},
		{"no semicolon", "'use client'\nexport function A() {}\n", true},
		{"after a line comment", "// header\n'use client';\nexport function A() {}\n", true},
		{"after a block comment", "/* header */\n'use client';\nexport function A() {}\n", true},
		{"after a multi-line block comment", "/*\n a\n*/\n'use client';\nexport function A() {}\n", true},
		{"after use strict", "'use strict';\n'use client';\nexport function A() {}\n", true},
		{"leading blank lines", "\n\n'use client';\nexport function A() {}\n", true},

		{"absent", "export function A() {}\n", false},
		// The directive is only a directive in the prologue. Once real code has
		// started, a matching string is just a string.
		{"after code", "import x from 'y';\n'use client';\nexport function A() {}\n", false},
		{"inside a string later", "export const s = \"use client\";\n", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeModule(t, "M.js", c.body)
			result, err := Scan(dir, "/client")
			if err != nil {
				t.Fatal(err)
			}
			if got := len(result.Modules) == 1; got != c.want {
				t.Errorf("detected = %v, want %v", got, c.want)
			}
		})
	}
}

func TestExportForms(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"function", "export function Button() {}", []string{"Button"}},
		{"async function", "export async function Button() {}", []string{"Button"}},
		{"const arrow", "export const Button = () => {};", []string{"Button"}},
		{"let", "export let Button = 1;", []string{"Button"}},
		{"class", "export class Button {}", []string{"Button"}},
		{"default function", "export default function Button() {}", []string{"default"}},
		{"anonymous default", "export default function () {}", []string{"default"}},
		{"default expression", "export default Button;", []string{"default"}},
		{"export list", "function A() {}\nfunction B() {}\nexport { A, B };", []string{"A", "B"}},
		{"renamed export", "function a() {}\nexport { a as Button };", []string{"Button"}},
		{"several", "export function A() {}\nexport const B = 1;", []string{"A", "B"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeModule(t, "M.js", "'use client';\n"+c.body+"\n")
			result, err := Scan(dir, "/client")
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Modules) != 1 {
				t.Fatalf("modules = %d, want 1 (warnings: %v)", len(result.Modules), result.Warnings)
			}
			got := strings.Join(result.Modules[0].Exports, ",")
			if want := strings.Join(c.want, ","); got != want {
				t.Errorf("exports = %q, want %q", got, want)
			}
		})
	}
}

// TestExportStarWarns pins the one form the scanner deliberately refuses to
// guess at: following it would mean resolving another module.
func TestExportStarWarns(t *testing.T) {
	dir := writeModule(t, "M.js", "'use client';\nexport * from './other.js';\n")
	result, err := Scan(dir, "/client")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a warning for `export *`")
	}
	if !strings.Contains(strings.Join(result.Warnings, " "), "export *") {
		t.Errorf("warnings = %v, want one naming `export *`", result.Warnings)
	}
}

// TestJSXWarnsRatherThanSilentlySucceeding matters because the failure it
// prevents is invisible: a browser served JSX fails at parse time, in the
// console, long after the build said it was fine.
func TestJSXWarnsRatherThanSilentlySucceeding(t *testing.T) {
	dir := writeModule(t, "Button.jsx", "'use client';\nexport function Button() { return <b/>; }\n")
	result, err := Scan(dir, "/client")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Modules) != 0 {
		t.Errorf("modules = %d, want 0 — a .jsx file is not browser-ready", len(result.Modules))
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "browser") {
		t.Errorf("warnings = %v, want one explaining the transform is missing", result.Warnings)
	}
}

func TestURLsAndGoNames(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Button.js":        "'use client';\nexport function Button() {}\n",
		"forms/Field.js":   "'use client';\nexport function Field() {}\n",
		"nav/side-menu.js": "'use client';\nexport default function () {}\n",
		"plain.js":         "export function NotAClientComponent() {}\n",
		"styles.css":       "body {}\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Scan(dir, "/client")
	if err != nil {
		t.Fatal(err)
	}

	exports := result.Exports()
	if len(exports) != 3 {
		t.Fatalf("exports = %d, want 3 (got %+v)", len(exports), exports)
	}

	want := []struct{ url, name, goName string }{
		// Sorted by URL, which is what keeps generated output stable.
		{"/client/Button.js", "Button", "Button"},
		{"/client/forms/Field.js", "Field", "Field"},
		// A default export is named after its file, and the hyphen becomes a
		// word boundary rather than an invalid identifier character.
		{"/client/nav/side-menu.js", "default", "SideMenu"},
	}
	for i, w := range want {
		got := exports[i]
		if got.Module.URL != w.url || got.Name != w.name || got.GoName != w.goName {
			t.Errorf("export %d = {%s %s %s}, want {%s %s %s}",
				i, got.Module.URL, got.Name, got.GoName, w.url, w.name, w.goName)
		}
	}
}

func TestMissingClientDirectoryIsNotAnError(t *testing.T) {
	result, err := Scan(filepath.Join(t.TempDir(), "absent"), "/client")
	if err != nil {
		t.Fatalf("a project with no client directory is valid: %v", err)
	}
	if len(result.Modules) != 0 {
		t.Errorf("modules = %d, want 0", len(result.Modules))
	}
}

// TestGeneratedGoCompiles is the guard on the code generator: its output is
// formatted with go/format, which parses it, so a template mistake fails here.
func TestGeneratedGoCompiles(t *testing.T) {
	dir := writeModule(t, "Counter.js", "'use client';\nexport function Counter() {}\nexport default function () {}\n")
	result, err := Scan(dir, "/client")
	if err != nil {
		t.Fatal(err)
	}

	source, err := GenerateGo(result.Exports(), Options{Package: "app"})
	if err != nil {
		t.Fatalf("generating: %v", err)
	}

	for _, want := range []string{
		"package app",
		`var Counter = rsc.Client("/client/Counter.js", "Counter", "/client/Counter.js")`,
		"func ClientHead() []react.Node",
		`const MountID = "root"`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated Go is missing %q\n%s", want, source)
		}
	}
}

func TestGeneratedEntryMentionsItsConfiguration(t *testing.T) {
	entry, err := GenerateEntry(Options{RSCPath: "/api/rsc", MountID: "app"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`fetch("/api/rsc")`,
		`getElementById("app")`,
		"__webpack_chunk_load__",
		"__webpack_require__",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("generated entry is missing %q\n%s", want, entry)
		}
	}
}

func TestImportMapPinsTheVerifiedVersion(t *testing.T) {
	imports := Options{ReactVersion: "19.2.8"}.ImportMap()["imports"].(map[string]string)
	if got := imports["react"]; !strings.Contains(got, "19.2.8") {
		t.Errorf("react = %q, want it pinned to 19.2.8", got)
	}
	// The wire format is coupled to the React version, so the client must not
	// float to whatever is newest.
	for specifier, url := range imports {
		if !strings.Contains(url, "19.2.8") {
			t.Errorf("%s = %q, want a pinned version", specifier, url)
		}
	}
}
