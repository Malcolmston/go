// Package analyze implements `react doctor`: static checks for the mistakes
// this port makes easy to write and hard to notice.
//
// The checks are deliberately specific to the react library rather than being
// general Go lint. Everything here is something `go vet` will never flag, that
// compiles cleanly, and that fails at run time — or worse, produces a subtly
// wrong tree — with a message far from the cause:
//
//   - a hook called conditionally, which hands the next hook another hook's
//     state, because a hook slot is identified by call index and nothing else;
//   - a component written as a bare func(Props) Node, which the runtime
//     dispatches past because it matches on the named Component type;
//   - a keyless element built in a loop, which makes state follow list
//     positions instead of list items.
//
// Analysis is syntactic. It parses with go/parser and never type-checks, which
// keeps `react doctor` instant and dependency-free at the cost of reasoning
// about names rather than types — the trade-off is called out on each check
// where it can produce a false positive.
package analyze

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Severity distinguishes a definite defect from a likely one.
type Severity int

const (
	// Warning marks a pattern that is usually but not always wrong.
	Warning Severity = iota

	// Error marks a pattern that is wrong in every case the checker can
	// construct — a genuine defect, not a style preference.
	Error
)

// String renders the severity for output.
func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Finding is one reported problem.
type Finding struct {
	Severity Severity
	Pos      token.Position

	// Check is the short identifier of the rule that fired, so a finding can be
	// looked up, discussed, or suppressed by name.
	Check string

	// Message states what is wrong.
	Message string

	// Hint states what to do about it. A checker that reports a problem without
	// a remedy makes the reader do the work twice.
	Hint string
}

// Result is a whole run.
type Result struct {
	Findings []Finding

	// Files is the number of Go files parsed, reported so a run that checked
	// nothing is visibly different from a run that found nothing.
	Files int
}

// Errors returns how many findings are of Error severity.
func (r Result) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// hookExemptions lists react functions that begin with "Use" but are not hooks
// and therefore carry none of the ordering rules.
//
// Use is the important entry: React 19's use() is defined to be callable
// conditionally and in loops, and this port honours that by never claiming a
// hook slot for it. Flagging it would be actively wrong — it would push people
// away from the one API designed for conditional calls.
var hookExemptions = map[string]bool{
	"Use": true,
}

// Run analyses every Go file under dir and returns what it found.
func Run(dir string) (Result, error) {
	var result Result
	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the directories that never contain project source: build
			// output, the CLI's own tool cache, VCS metadata, and vendored code
			// whose problems are not this project's to fix.
			switch d.Name() {
			case ".git", ".react", "dist", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			// A file that does not parse is the compiler's problem to report,
			// with a far better message than this tool would produce.
			return nil
		}

		result.Files++
		alias, ok := reactImportAlias(file)
		if !ok {
			// A file that does not import react cannot contain react mistakes.
			return nil
		}

		fa := &fileAnalysis{fset: fset, alias: alias}
		fa.check(file)
		result.Findings = append(result.Findings, fa.findings...)
		return nil
	})
	if err != nil {
		return result, err
	}

	sort.Slice(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i].Pos, result.Findings[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.Offset < b.Offset
	})
	return result, nil
}

// reactImportAlias returns the name the react package is bound to in this file,
// honouring an import alias. Matching on the local name rather than assuming
// "react" is what lets the checks survive `import r "github.com/..."`.
func reactImportAlias(file *ast.File) (string, bool) {
	const path = `"github.com/malcolmston/react"`
	for _, imp := range file.Imports {
		if imp.Path.Value != path {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				// A blank import cannot be called through, and a dot import
				// erases the qualifier every check relies on.
				return "", false
			}
			return imp.Name.Name, true
		}
		return "react", true
	}
	return "", false
}

// fileAnalysis carries the per-file state the checks share.
type fileAnalysis struct {
	fset     *token.FileSet
	alias    string
	findings []Finding
}

func (fa *fileAnalysis) report(pos token.Pos, sev Severity, check, msg, hint string) {
	fa.findings = append(fa.findings, Finding{
		Severity: sev,
		Pos:      fa.fset.Position(pos),
		Check:    check,
		Message:  msg,
		Hint:     hint,
	})
}

// reactCall returns the react function being called, if the expression is a
// call to one.
func (fa *fileAnalysis) reactCall(n ast.Node) (string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != fa.alias {
		return "", false
	}
	return sel.Sel.Name, true
}

// isHookName reports whether a react function is a hook subject to the ordering
// rules.
func (fa *fileAnalysis) isHookName(name string) bool {
	return strings.HasPrefix(name, "Use") && !hookExemptions[name]
}

// check runs every rule over one file.
func (fa *fileAnalysis) check(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Body != nil {
				fa.checkHookOrder(node.Body)
			}
			fa.checkUnconvertedComponentType(node.Name, node.Type, node.Name.Pos())
		case *ast.FuncLit:
			fa.checkHookOrder(node.Body)
		case *ast.ValueSpec:
			fa.checkValueSpecComponentType(node)
		case *ast.RangeStmt:
			fa.checkKeysInLoop(node.Body)
		case *ast.ForStmt:
			fa.checkKeysInLoop(node.Body)
		}
		return true
	})
}

// checkHookOrder reports hooks called anywhere other than unconditionally at
// the top level of a function body.
//
// The walk stops descending into nested function literals and handles them
// separately, because a hook inside a closure belongs to that closure — and a
// closure is never a component body, so a hook there is always wrong for a
// second, more specific reason.
func (fa *fileAnalysis) checkHookOrder(body *ast.BlockStmt) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		fa.walkForHooks(stmt, nil)
	}
}

// walkForHooks descends one statement, recording the control-flow constructs it
// passed through in enclosing.
func (fa *fileAnalysis) walkForHooks(n ast.Node, enclosing []string) {
	switch node := n.(type) {
	case *ast.FuncLit:
		// A hook inside a closure — an effect body, a callback, a goroutine.
		// Report at the hook, not at the closure, and stop: the closure's own
		// top level is not a component's top level.
		ast.Inspect(node.Body, func(inner ast.Node) bool {
			if name, ok := fa.reactCall(inner); ok && fa.isHookName(name) {
				fa.report(inner.Pos(), Error, "hook-in-closure",
					fmt.Sprintf("%s is called inside a function literal", name),
					"Hooks may only be called from the body of a component. Move the call "+
						"to the component body and capture the value the closure needs.")
			}
			return true
		})
		return

	case *ast.IfStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "an if statement"))
		if node.Else != nil {
			fa.walkForHooks(node.Else, append(enclosing, "an else branch"))
		}
		// The condition itself is evaluated unconditionally, so a hook there is
		// fine and is intentionally not walked as nested.
		fa.walkForHooks(node.Cond, enclosing)
		return

	case *ast.ForStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "a for loop"))
		return

	case *ast.RangeStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "a range loop"))
		return

	case *ast.SwitchStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "a switch statement"))
		return

	case *ast.TypeSwitchStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "a type switch"))
		return

	case *ast.SelectStmt:
		fa.walkForHooksIn(node.Body, append(enclosing, "a select statement"))
		return
	}

	if len(enclosing) > 0 {
		if name, ok := fa.reactCall(n); ok && fa.isHookName(name) {
			fa.report(n.Pos(), Error, "conditional-hook",
				fmt.Sprintf("%s is called inside %s", name, enclosing[len(enclosing)-1]),
				"Hooks are matched to their state by call order, so a hook that does not "+
					"run on every render shifts every later hook onto the wrong slot. Call it "+
					"unconditionally and move the condition inside — pass the branch to the "+
					"hook's arguments, or guard the value it returns.")
		}
	}

	// Descend into everything else, carrying the enclosing context along.
	switch node := n.(type) {
	case *ast.BlockStmt:
		fa.walkForHooksIn(node, enclosing)
	default:
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n || child == nil {
				return child == n
			}
			switch child.(type) {
			case *ast.FuncLit, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
				*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				fa.walkForHooks(child, enclosing)
				return false
			}
			if len(enclosing) > 0 {
				if name, ok := fa.reactCall(child); ok && fa.isHookName(name) {
					fa.report(child.Pos(), Error, "conditional-hook",
						fmt.Sprintf("%s is called inside %s", name, enclosing[len(enclosing)-1]),
						"Hooks are matched to their state by call order, so a hook that does "+
							"not run on every render shifts every later hook onto the wrong slot.")
				}
			}
			return true
		})
	}
}

// walkForHooksIn walks every statement of a block.
func (fa *fileAnalysis) walkForHooksIn(block *ast.BlockStmt, enclosing []string) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		fa.walkForHooks(stmt, enclosing)
	}
}

// isComponentSignature reports whether a function type is exactly
// func(react.Props) react.Node — the shape a component has.
func (fa *fileAnalysis) isComponentSignature(ft *ast.FuncType) bool {
	if ft == nil || ft.Params == nil || ft.Results == nil {
		return false
	}
	if len(ft.Params.List) != 1 || len(ft.Results.List) != 1 {
		return false
	}
	return fa.isReactType(ft.Params.List[0].Type, "Props") &&
		fa.isReactType(ft.Results.List[0].Type, "Node")
}

// isReactType reports whether an expression names react.<name>.
func (fa *fileAnalysis) isReactType(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == fa.alias && sel.Sel.Name == name
}

// checkUnconvertedComponentType reports a top-level func declaration with a
// component's signature.
//
// This is the mistake every newcomer makes. The runtime dispatches on the named
// Component type, so `func Card(p react.Props) react.Node` compiles, reads
// exactly like a component, and renders as nothing at all — there is no error,
// just a missing subtree.
func (fa *fileAnalysis) checkUnconvertedComponentType(name *ast.Ident, ft *ast.FuncType, pos token.Pos) {
	if name == nil || !fa.isComponentSignature(ft) {
		return
	}
	fa.report(pos, Error, "unconverted-component",
		fmt.Sprintf("%s has a component's signature but is declared as a plain func", name.Name),
		fmt.Sprintf("The runtime matches components by their named type, so this will render "+
			"as nothing. Declare it as `var %s %s.Component = func(props %s.Props) %s.Node { … }`, "+
			"or convert at the call site with %s.Component(%s).",
			name.Name, fa.alias, fa.alias, fa.alias, fa.alias, name.Name))
}

// checkValueSpecComponentType reports a var or const whose declared type is the
// bare function signature rather than react.Component.
func (fa *fileAnalysis) checkValueSpecComponentType(spec *ast.ValueSpec) {
	ft, ok := spec.Type.(*ast.FuncType)
	if !ok || !fa.isComponentSignature(ft) {
		return
	}
	for _, name := range spec.Names {
		fa.report(name.Pos(), Error, "unconverted-component",
			fmt.Sprintf("%s is declared as func(Props) Node rather than %s.Component",
				name.Name, fa.alias),
			fmt.Sprintf("Change the declared type to %s.Component; the runtime dispatches on "+
				"the named type and will not recognise this value as a component.", fa.alias))
	}
}

// checkKeysInLoop reports elements built inside a loop with no key prop.
//
// Without a key, siblings are reconciled by position: inserting at the front of
// a list shifts every item's state onto its neighbour. The check is syntactic
// and only looks at literal props maps, so a props value built elsewhere is
// invisible to it — which is why this is a warning rather than an error.
func (fa *fileAnalysis) checkKeysInLoop(body *ast.BlockStmt) {
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		name, ok := fa.reactCall(n)
		if !ok || (name != "H" && name != "CreateElement") {
			return true
		}
		call := n.(*ast.CallExpr)
		if len(call.Args) < 2 {
			return true
		}

		props, ok := call.Args[1].(*ast.CompositeLit)
		if !ok {
			// nil props, or a props value from elsewhere. A nil props map in a
			// loop is definitely keyless, and worth saying so.
			if ident, isIdent := call.Args[1].(*ast.Ident); isIdent && ident.Name == "nil" {
				fa.reportMissingKey(call.Pos(), name)
			}
			return true
		}

		for _, elt := range props.Elts {
			kv, isKV := elt.(*ast.KeyValueExpr)
			if !isKV {
				continue
			}
			if lit, isLit := kv.Key.(*ast.BasicLit); isLit && lit.Value == `"key"` {
				return true
			}
		}
		fa.reportMissingKey(call.Pos(), name)
		return true
	})
}

func (fa *fileAnalysis) reportMissingKey(pos token.Pos, fn string) {
	fa.report(pos, Warning, "missing-key",
		fmt.Sprintf("%s.%s is called inside a loop with no \"key\" prop", fa.alias, fn),
		"Keyless siblings are matched by position, so inserting or reordering items moves "+
			"state onto the wrong ones. Give each item a key that identifies the item itself "+
			"— its ID, not its index.")
}
