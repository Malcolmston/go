package analyze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write lays out a throwaway project containing one Go file and returns its
// directory. Using a real directory rather than an in-memory fs keeps the test
// exercising the same WalkDir path the command uses.
func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	src := "package app\n\nimport \"github.com/malcolmston/react\"\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// checks returns the set of check names reported for a snippet.
func checks(t *testing.T, body string) []string {
	t.Helper()
	result, err := Run(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(result.Findings))
	for _, f := range result.Findings {
		out = append(out, f.Check)
	}
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestConditionalHook(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "hook at top level is fine",
			body: `var C react.Component = func(p react.Props) react.Node {
	n, _ := react.UseState(0)
	return react.H("b", nil, n)
}`,
			want: false,
		},
		{
			name: "hook inside if",
			body: `var C react.Component = func(p react.Props) react.Node {
	if p.Has("x") {
		react.UseState(0)
	}
	return nil
}`,
			want: true,
		},
		{
			name: "hook inside else",
			body: `var C react.Component = func(p react.Props) react.Node {
	if p.Has("x") {
	} else {
		react.UseState(0)
	}
	return nil
}`,
			want: true,
		},
		{
			name: "hook inside for",
			body: `var C react.Component = func(p react.Props) react.Node {
	for i := 0; i < 3; i++ {
		react.UseRef(i)
	}
	return nil
}`,
			want: true,
		},
		{
			name: "hook inside range",
			body: `var C react.Component = func(p react.Props) react.Node {
	for range p.Children() {
		react.UseMemo(func() int { return 1 }, nil)
	}
	return nil
}`,
			want: true,
		},
		{
			name: "hook inside switch",
			body: `var C react.Component = func(p react.Props) react.Node {
	switch p.String("kind") {
	case "a":
		react.UseState(0)
	}
	return nil
}`,
			want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := has(checks(t, c.body), "conditional-hook")
			if got != c.want {
				t.Errorf("conditional-hook reported = %v, want %v", got, c.want)
			}
		})
	}
}

// TestUseIsExemptFromHookOrder pins the most important exemption: React 19's
// use() is defined to be callable conditionally, and this port honours that by
// not giving it a hook slot. Flagging it would push people away from the one
// API designed for the case.
func TestUseIsExemptFromHookOrder(t *testing.T) {
	body := `var C react.Component = func(p react.Props) react.Node {
	if p.Has("x") {
		r := react.Resolved("v")
		_ = react.Use(r)
	}
	return nil
}`
	if got := checks(t, body); len(got) != 0 {
		t.Errorf("findings = %v, want none — react.Use may be called conditionally", got)
	}
}

func TestHookInClosure(t *testing.T) {
	body := `var C react.Component = func(p react.Props) react.Node {
	react.UseEffectFn(func() {
		react.UseId()
	}, nil)
	return nil
}`
	if !has(checks(t, body), "hook-in-closure") {
		t.Error("expected hook-in-closure for a hook called inside an effect body")
	}
}

func TestUnconvertedComponent(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "func declaration with component signature",
			body: `func Card(p react.Props) react.Node { return nil }`,
			want: true,
		},
		{
			name: "var declared with the bare func type",
			body: `var Card func(react.Props) react.Node`,
			want: true,
		},
		{
			name: "correctly typed component",
			body: `var Card react.Component = func(p react.Props) react.Node { return nil }`,
			want: false,
		},
		{
			name: "unrelated function",
			body: `func helper(s string) string { return s }`,
			want: false,
		},
		{
			name: "wrong result type is not a component",
			body: `func Card(p react.Props) string { return "" }`,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := has(checks(t, c.body), "unconverted-component")
			if got != c.want {
				t.Errorf("unconverted-component reported = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMissingKey(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "keyless element in a range loop",
			body: `var C react.Component = func(p react.Props) react.Node {
	var kids []react.Node
	for _, s := range []string{"a"} {
		kids = append(kids, react.H("li", nil, s))
	}
	return react.H("ul", nil, kids)
}`,
			want: true,
		},
		{
			name: "keyed element in a range loop",
			body: `var C react.Component = func(p react.Props) react.Node {
	var kids []react.Node
	for _, s := range []string{"a"} {
		kids = append(kids, react.H("li", react.Props{"key": s}, s))
	}
	return react.H("ul", nil, kids)
}`,
			want: false,
		},
		{
			name: "element outside a loop needs no key",
			body: `var C react.Component = func(p react.Props) react.Node {
	return react.H("li", nil, "solo")
}`,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := has(checks(t, c.body), "missing-key")
			if got != c.want {
				t.Errorf("missing-key reported = %v, want %v", got, c.want)
			}
		})
	}
}

// TestImportAliasIsHonoured checks that the analysis keys off the local name of
// the react import rather than assuming "react".
func TestImportAliasIsHonoured(t *testing.T) {
	dir := t.TempDir()
	src := `package app

import r "github.com/malcolmston/react"

var C r.Component = func(p r.Props) r.Node {
	if p.Has("x") {
		r.UseState(0)
	}
	return nil
}
`
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected a finding through the aliased import")
	}
	if !strings.Contains(result.Findings[0].Message, "UseState") {
		t.Errorf("message = %q, want it to name UseState", result.Findings[0].Message)
	}
}

// TestFilesWithoutReactAreSkipped keeps the checks from firing on unrelated Go
// code that happens to have a similarly shaped function.
func TestFilesWithoutReactAreSkipped(t *testing.T) {
	dir := t.TempDir()
	src := `package other

type Props map[string]any
type Node any

func Card(p Props) Node { return nil }
`
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings = %v, want none in a file that does not import react", result.Findings)
	}
	if result.Files != 1 {
		t.Errorf("files = %d, want 1", result.Files)
	}
}

// TestUnparseableFileIsSkipped confirms the analyser leaves syntax errors to the
// compiler instead of producing a worse duplicate of the same message.
func TestUnparseableFileIsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package app\nfunc ("), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Run(dir)
	if err != nil {
		t.Fatalf("Run returned an error for an unparseable file: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings = %v, want none", result.Findings)
	}
}

// TestSkippedDirectories keeps generated and vendored trees out of the report.
func TestSkippedDirectories(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"dist", "vendor", ".react"} {
		full := filepath.Join(dir, sub)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		src := "package app\n\nimport \"github.com/malcolmston/react\"\n\nfunc Card(p react.Props) react.Node { return nil }\n"
		if err := os.WriteFile(filepath.Join(full, "app.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 0 {
		t.Errorf("files = %d, want 0 — dist, vendor and .react must be skipped", result.Files)
	}
}

func TestResultErrorsCount(t *testing.T) {
	body := `func Card(p react.Props) react.Node {
	var kids []react.Node
	for _, s := range []string{"a"} {
		kids = append(kids, react.H("li", nil, s))
	}
	return react.H("ul", nil, kids)
}`
	result, err := Run(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.Errors(), 1; got != want {
		t.Errorf("errors = %d, want %d", got, want)
	}
	if got, want := len(result.Findings)-result.Errors(), 1; got != want {
		t.Errorf("warnings = %d, want %d", got, want)
	}
}
