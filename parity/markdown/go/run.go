// Command run is the Go-side parity runner for github.com/malcolmston/markdown.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. A panic or error inside a
// case is recovered and reported as {"ok": false, "error": "..."} and the runner
// keeps going, so one bad case never costs the rest of the suite.
//
// Determinism: the package has no clocks, randomness or locale dependence, and
// every renderer is constructed from an explicit Options value, so nothing here
// depends on the environment.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/markdown"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// Renderers matching the option sets the case groups exercise. Each *Markdown
// is safe for concurrent use and is reused across cases.
var (
	stock      = markdown.New(markdown.DefaultOptions())
	noHTML     = markdown.New(markdown.Options{HTML: false})
	typography = markdown.New(markdown.Options{HTML: true, Typographer: true})
	lineBreaks = markdown.New(markdown.Options{HTML: true, LineBreaks: true})
	linkTarget = markdown.New(markdown.Options{HTML: true, LinkTarget: "_blank"})
)

// ---------------------------------------------------------------- AST shape

// astNode is the comparable subset of the AST, in the same shape the Node
// runner emits. Fields are masked by node type identically on both sides: the
// port reuses Literal for a list's bullet character, Info for its bullet/
// ordered kind and Level for an ordered list's start number, and it copies all
// three onto each Item, none of which commonmark.js does.
type astNode struct {
	Type        string    `json:"type"`
	Literal     string    `json:"literal"`
	Level       int       `json:"level"`
	Destination string    `json:"destination"`
	Title       string    `json:"title"`
	Info        string    `json:"info"`
	ListType    *string   `json:"listType,omitempty"`
	ListTight   *bool     `json:"listTight,omitempty"`
	ListStart   *int      `json:"listStart,omitempty"`
	Children    []astNode `json:"children"`
}

func hasLiteral(t markdown.NodeType) bool {
	switch t {
	case markdown.Text, markdown.Code, markdown.CodeBlock, markdown.HTMLBlock, markdown.HTMLInline:
		return true
	}
	return false
}

func astOf(n *markdown.Node) astNode {
	out := astNode{
		Type:        n.Type.String(),
		Destination: n.Destination,
		Title:       n.Title,
		Children:    []astNode{},
	}
	if hasLiteral(n.Type) {
		out.Literal = n.Literal
	}
	if n.Type == markdown.Heading {
		out.Level = n.Level
	}
	if n.Type == markdown.CodeBlock {
		out.Info = n.Info
	}
	if n.Type == markdown.List {
		kind, tight, start := n.Info, n.Tight, n.Level
		out.ListType, out.ListTight, out.ListStart = &kind, &tight, &start
	}
	for _, c := range n.Children {
		out.Children = append(out.Children, astOf(c))
	}
	return out
}

// walkTypes collects the distinct node-type names reachable from n, sorted, via
// the public Walk method.
func walkTypes(n *markdown.Node) []string {
	seen := map[string]bool{}
	n.Walk(func(node *markdown.Node) bool {
		seen[node.Type.String()] = true
		return true
	})
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func countNodes(n *markdown.Node) int {
	count := 0
	n.Walk(func(*markdown.Node) bool {
		count++
		return true
	})
	return count
}

// failingWriter always errors, so RenderTo's ErrWrite path can be exercised.
type failingWriter struct{}

var errBoom = errors.New("boom")

func (failingWriter) Write([]byte) (int, error) { return 0, errBoom }

// ------------------------------------------------------------------ dispatch

func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d must be a string: %w", i, err)
	}
	return s, nil
}

func handle(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	switch req.Fn {
	// The spec corpus, and the security group's "what does it do by default"
	// question: package-level Render, i.e. DefaultOptions().
	case "render":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return markdown.Render(src), nil

	// The other package-level entry point must agree with Render.
	case "render_to":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		var buf strings.Builder
		if err := markdown.RenderTo(&buf, src); err != nil {
			return nil, err
		}
		return buf.String(), nil

	// No sanitising option exists in v0.1.0. Reported as a failure rather than
	// silently falling back to Render, so the gap is scored, not hidden.
	case "render_safe":
		return nil, errors.New("github.com/malcolmston/markdown v0.1.0 has no safe/sanitising mode: " +
			"no link-scheme allowlist and no way to strip raw HTML while keeping links")

	case "render_nohtml":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return noHTML.Render(src), nil

	case "render_smart":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return typography.Render(src), nil

	case "render_softbreak":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return lineBreaks.Render(src), nil

	case "render_linktarget":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return linkTarget.Render(src), nil

	case "default_options":
		o := markdown.DefaultOptions()
		return map[string]any{
			"HTML":        o.HTML,
			"Typographer": o.Typographer,
			"LineBreaks":  o.LineBreaks,
			"LinkTarget":  o.LinkTarget,
		}, nil

	case "ast":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return astOf(stock.Parse(src)), nil

	case "node_types":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return walkTypes(markdown.Parse(src)), nil

	case "node_nav":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		doc := markdown.Parse(src)
		out := map[string]any{
			"docType":          doc.Type.String(),
			"firstChild":       nil,
			"lastChild":        nil,
			"firstChildParent": nil,
			"rootParent":       nil,
			"nodeCount":        countNodes(doc),
		}
		if fc := doc.FirstChild(); fc != nil {
			out["firstChild"] = fc.Type.String()
			if p := fc.Parent(); p != nil {
				out["firstChildParent"] = p.Type.String()
			}
		}
		if lc := doc.LastChild(); lc != nil {
			out["lastChild"] = lc.Type.String()
		}
		if p := doc.Parent(); p != nil {
			out["rootParent"] = p.Type.String()
		}
		return out, nil

	// The same hand-built tree the Node runner builds with appendChild. There
	// is no exported way to render a *Node, so the tree shape is compared.
	case "append_child":
		doc := &markdown.Node{Type: markdown.Document}
		para := &markdown.Node{Type: markdown.Paragraph}
		para.AppendChild(&markdown.Node{Type: markdown.Text, Literal: "hi"})
		doc.AppendChild(para)
		emph := &markdown.Node{Type: markdown.Emph}
		emph.AppendChild(&markdown.Node{Type: markdown.Text, Literal: "there"})
		para.AppendChild(emph)
		return astOf(doc), nil

	// NodeType.IsBlock over every defined node type.
	case "block_types":
		var block []string
		for t := markdown.Document; t <= markdown.HardBreak; t++ {
			if t.IsBlock() {
				block = append(block, t.String())
			}
		}
		sortStrings(block)
		return block, nil

	case "render_to_bad_writer":
		src, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		werr := markdown.RenderTo(failingWriter{}, src)
		if werr == nil {
			return nil, errors.New("RenderTo returned nil for a failing writer")
		}
		if !errors.Is(werr, markdown.ErrWrite) || !errors.Is(werr, markdown.ErrMarkdown) || !errors.Is(werr, errBoom) {
			return nil, fmt.Errorf("RenderTo error does not wrap ErrWrite/ErrMarkdown/the writer error: %v", werr)
		}
		return nil, fmt.Errorf("RenderTo failed as expected: %w", werr)

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}

		var req request
		res := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			res = response{OK: false, Error: "bad request: " + err.Error()}
		} else {
			res.ID = req.ID
			value, err := handle(req)
			if err != nil {
				res.OK, res.Error = false, err.Error()
			} else {
				res.OK, res.Value = true, value
			}
		}

		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(os.Stderr, "encode reply:", err)
			return
		}
		// Flush after every line or the harness deadlocks waiting for a reply.
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			return
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
	}
}
