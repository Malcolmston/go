// Command markdown-example exercises the published
// github.com/malcolmston/markdown module: it renders a document covering every
// CommonMark block and inline construct, checks a batch of renderings against
// the expected CommonMark 0.31.2 output, walks the AST, demonstrates the
// renderer options, and probes the HTML escaping and link-destination handling.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/malcolmston/markdown"
)

// doc is a single document exercising headings, emphasis, lists (tight, loose,
// ordered, nested), links (inline, reference, autolink), images, code (inline,
// fenced with info string, indented), block quotes, thematic breaks, hard
// breaks, entities, backslash escapes and a GFM-style table.
const doc = "" +
	"# Release notes\n" +
	"\n" +
	"Subtitle set with setext\n" +
	"========================\n" +
	"\n" +
	"## Highlights\n" +
	"\n" +
	"This release is *fast*, _really fast_, and **stable** — even __very__ stable.\n" +
	"Nested: **bold with *italic* inside**, and ***both at once***.\n" +
	"An intraword_underscore_stays literal, but a*star*does not.\n" +
	"\n" +
	"### Tight bullet list\n" +
	"\n" +
	"- first item\n" +
	"- second item with `inline code`\n" +
	"- third item\n" +
	"  - nested with a [link](https://example.com/docs \"Docs title\")\n" +
	"  - nested with an ![image](https://example.com/logo.png \"Logo\")\n" +
	"\n" +
	"### Loose ordered list starting at 3\n" +
	"\n" +
	"3. first paragraph\n" +
	"\n" +
	"4. second paragraph\n" +
	"\n" +
	"   with a continuation line\n" +
	"\n" +
	"## Links\n" +
	"\n" +
	"A reference link: [the spec][cm], a collapsed one: [cm][], a shortcut: [cm].\n" +
	"An autolink: <https://example.org/autolink> and an email <nobody@example.org>.\n" +
	"\n" +
	"[cm]: https://spec.commonmark.org/0.31.2/ \"CommonMark 0.31.2\"\n" +
	"\n" +
	"## Code\n" +
	"\n" +
	"```go\n" +
	"func main() {\n" +
	"\tfmt.Println(\"a < b && c > d\")\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"    indented code block\n" +
	"    second line\n" +
	"\n" +
	"## Quotes\n" +
	"\n" +
	"> A block quote with **emphasis**.\n" +
	">\n" +
	"> > and a nested quote\n" +
	">\n" +
	"> - containing a list\n" +
	"> - with two items\n" +
	"\n" +
	"---\n" +
	"\n" +
	"## Escaping and entities\n" +
	"\n" +
	"Raw markup in text: <script>alert(1)</script> & an ampersand, 5 < 6 > 4.\n" +
	"Entities: &copy; &amp; &#42; &#x1F600;.\n" +
	"Backslash escapes: \\*not emphasis\\*, \\`not code\\`, \\# not a heading.\n" +
	"A hard break ends this line with two spaces,  \nand this is the next line.\n" +
	"\n" +
	"## Table (GFM, not CommonMark)\n" +
	"\n" +
	"| Feature | Supported |\n" +
	"| ------- | --------- |\n" +
	"| tables  | ?         |\n"

func rule(title string) {
	fmt.Printf("\n========== %s ==========\n", title)
}

func main() {
	// The package-level Render uses DefaultOptions(), which in the published
	// module is Options{HTML: true} — CommonMark-strict, raw HTML passed through.
	// A renderer that escapes raw HTML has to be built by hand.
	spec := markdown.New(markdown.DefaultOptions()) // HTML: true
	safe := markdown.New(markdown.Options{})        // HTML: false

	// -----------------------------------------------------------------
	rule("1. Full document, package defaults (CommonMark-strict)")
	// -----------------------------------------------------------------
	fmt.Print(markdown.Render(doc))

	// -----------------------------------------------------------------
	rule("2. Options{HTML:false} escapes raw HTML instead")
	// -----------------------------------------------------------------
	fmt.Printf("DefaultOptions() = %+v\n", markdown.DefaultOptions())
	fmt.Println("HTML:true  (package default):", grepLine(spec.Render(doc), "alert(1)"))
	fmt.Println("HTML:false                  :", grepLine(safe.Render(doc), "alert(1)"))
	// HOLE: there is no CommonMarkOptions() in the published module (its
	// API-DEVIATIONS.md and README describe one, and describe DefaultOptions() as
	// the *safe* HTML:false preset). Only DefaultOptions() exists, and it is
	// HTML:true. Uncommenting the next line does not compile:
	//   fmt.Printf("%+v\n", markdown.CommonMarkOptions())

	// -----------------------------------------------------------------
	rule("3. Conformance spot-checks against CommonMark 0.31.2")
	// -----------------------------------------------------------------
	checks := []struct {
		name, src, want string
	}{
		{"atx heading", "# hi\n", "<h1>hi</h1>\n"},
		{"setext heading", "hi\n===\n", "<h1>hi</h1>\n"},
		{"h6 and h7", "###### six\n####### seven\n", "<h6>six</h6>\n<p>####### seven</p>\n"},
		{"emphasis", "*a* _b_ **c** __d__\n", "<p><em>a</em> <em>b</em> <strong>c</strong> <strong>d</strong></p>\n"},
		{"nested emphasis", "***abc***\n", "<p><em><strong>abc</strong></em></p>\n"},
		{"intraword underscore", "foo_bar_baz\n", "<p>foo_bar_baz</p>\n"},
		{"tight list", "- a\n- b\n", "<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n"},
		{"loose list", "- a\n\n- b\n", "<ul>\n<li>\n<p>a</p>\n</li>\n<li>\n<p>b</p>\n</li>\n</ul>\n"},
		{"ordered start", "3. a\n4. b\n", "<ol start=\"3\">\n<li>a</li>\n<li>b</li>\n</ol>\n"},
		{"inline link", "[a](/b \"t\")\n", "<p><a href=\"/b\" title=\"t\">a</a></p>\n"},
		{"image", "![alt](/i.png \"t\")\n", "<p><img src=\"/i.png\" alt=\"alt\" title=\"t\" /></p>\n"},
		{"autolink", "<https://x.com>\n", "<p><a href=\"https://x.com\">https://x.com</a></p>\n"},
		{"email autolink", "<a@b.com>\n", "<p><a href=\"mailto:a@b.com\">a@b.com</a></p>\n"},
		{"code span", "`a < b`\n", "<p><code>a &lt; b</code></p>\n"},
		{"fenced code info", "```go\nx\n```\n", "<pre><code class=\"language-go\">x\n</code></pre>\n"},
		{"indented code", "    x < y\n", "<pre><code>x &lt; y\n</code></pre>\n"},
		{"block quote", "> a\n", "<blockquote>\n<p>a</p>\n</blockquote>\n"},
		{"thematic break", "***\n", "<hr />\n"},
		{"hard break spaces", "a  \nb\n", "<p>a<br />\nb</p>\n"},
		{"hard break backslash", "a\\\nb\n", "<p>a<br />\nb</p>\n"},
		{"soft break", "a\nb\n", "<p>a\nb</p>\n"},
		{"raw html block", "<div>x</div>\n", "<div>x</div>\n"},
		{"entity", "&copy; &#42;\n", "<p>© *</p>\n"},
		{"NUL ref", "&#0;\n", "<p>�</p>\n"},
		{"out-of-range ref", "&#9999999;\n", "<p>�</p>\n"},
		{"8-digit ref not a ref", "&#99999999;\n", "<p>&amp;#99999999;</p>\n"},
		{"bare ampersand", "&copy &nope;\n", "<p>&amp;copy &amp;nope;</p>\n"},
		{"escapes", "\\*a\\*\n", "<p>*a*</p>\n"},
		{"text escaping", "5 < 6 & 7 > 4 \"q\"\n", "<p>5 &lt; 6 &amp; 7 &gt; 4 &quot;q&quot;</p>\n"},
		{"tab expansion", "\tcode\n", "<pre><code>code\n</code></pre>\n"},
		{"reference link", "[a][r]\n\n[r]: /u \"t\"\n", "<p><a href=\"/u\" title=\"t\">a</a></p>\n"},
		{"gfm table (unsupported)", "| a | b |\n| - | - |\n| 1 | 2 |\n",
			"<p>| a | b |\n| - | - |\n| 1 | 2 |</p>\n"},
	}
	pass, fail := 0, 0
	for _, c := range checks {
		got := spec.Render(c.src)
		if got == c.want {
			pass++
			fmt.Printf("  ok   %-24s %s\n", c.name, oneLine(got))
			continue
		}
		fail++
		fmt.Printf("  FAIL %-24s\n        want %s\n        got  %s\n", c.name, oneLine(c.want), oneLine(got))
	}
	fmt.Printf("spot-checks: %d ok, %d failing\n", pass, fail)

	// -----------------------------------------------------------------
	rule("4. HTML escaping and link destinations")
	// -----------------------------------------------------------------
	fmt.Println("-- raw HTML, package default (HTML:true, passed through verbatim) --")
	for _, src := range []string{
		"<script>alert(1)</script>\n",
		"<div onclick=\"evil()\">block</div>\n",
		"inline <b onmouseover=x>tag</b>\n",
		"<!-- a comment -->\n",
	} {
		fmt.Printf("  %-40q -> %s\n", src, oneLine(markdown.Render(src)))
	}

	fmt.Println("-- raw HTML with Options{HTML:false} (escaped) --")
	for _, src := range []string{
		"<script>alert(1)</script>\n",
		"<div onclick=\"evil()\">block</div>\n",
		"inline <b onmouseover=x>tag</b>\n",
		"<!-- a comment -->\n",
	} {
		fmt.Printf("  %-40q -> %s\n", src, oneLine(safe.Render(src)))
	}

	// HOLE: the published module has no link-destination sanitizing at all —
	// no ValidateLink function, no Options.ValidateLink hook, no security.go.
	// Its own README/API-DEVIATIONS.md document both. So javascript: URLs reach
	// the output verbatim and there is no supported way to reject them.
	// Uncommenting either of these does not compile:
	//   markdown.ValidateLink("javascript:x")
	//   markdown.New(markdown.Options{ValidateLink: func(string) bool { return true }})
	fmt.Println("-- link destinations (NO sanitizing available: see HOLE) --")
	for _, src := range []string{
		"[x](javascript:alert(1))",
		"[x](JaVaScRiPt:alert(1))",
		"[x](<javascript:alert(1)>)",
		"[x](&#x6a;avascript:alert&lpar;1&rpar;)",
		"[x](java&#x09;script:alert(1))",
		"[x](vbscript:x)",
		"[x](file:///etc/passwd)",
		"![x](data:text/html,<script>alert(1)</script>)",
		"![x](data:image/png;base64,iVBORw0KGgo=)",
		"<javascript:alert(1)>",
		"[x](https://ok.example/path?a=1&b=2)",
		"[x](/relative/path)",
		"[x](\"><img src=x onerror=alert(1)>)",
	} {
		out := markdown.Render(src + "\n")
		flag := "      "
		if strings.Contains(strings.ToLower(out), "javascript:") ||
			strings.Contains(strings.ToLower(out), "vbscript:") ||
			strings.Contains(strings.ToLower(out), "data:text/html") {
			flag = "UNSAFE"
		}
		fmt.Printf("  %s %-46q -> %s\n", flag, src, oneLine(out))
	}

	fmt.Println("-- attribute-value escaping (this part does work) --")
	fmt.Print(markdown.Render("[a](/u \"ti\\\"tle & <b>\")\n"))
	fmt.Print(markdown.Render("![al\\\"t & <b>](/i.png)\n"))
	fmt.Print(markdown.Render("```go\" onload=\"x\ncode\n```\n"))

	// -----------------------------------------------------------------
	rule("5. Options: Typographer, LineBreaks, LinkTarget")
	// -----------------------------------------------------------------
	typo := markdown.New(markdown.Options{HTML: true, Typographer: true})
	src := "\"Smart quotes\", 'single', an en--dash, an em---dash, and ellipses...\n"
	fmt.Println("Typographer off:", oneLine(markdown.Render(src)))
	fmt.Println("Typographer on: ", oneLine(typo.Render(src)))

	brs := markdown.New(markdown.Options{HTML: true, LineBreaks: true})
	fmt.Println("LineBreaks off: ", oneLine(markdown.Render("line one\nline two\n")))
	fmt.Println("LineBreaks on:  ", oneLine(brs.Render("line one\nline two\n")))

	tgt := markdown.New(markdown.Options{HTML: true, LinkTarget: "_blank"})
	fmt.Println("LinkTarget:     ", oneLine(tgt.Render("[a](https://x.com)\n")))
	tgtEvil := markdown.New(markdown.Options{HTML: true, LinkTarget: `_blank" onload="x`})
	fmt.Println("LinkTarget esc: ", oneLine(tgtEvil.Render("[a](https://x.com)\n")))

	// -----------------------------------------------------------------
	rule("6. AST walking")
	// -----------------------------------------------------------------
	tree := markdown.Parse(doc)
	counts := map[markdown.NodeType]int{}
	var visitedAll int
	fmt.Println("outline (headings, lists, links, images, code blocks):")
	tree.Walk(func(n *markdown.Node) bool {
		counts[n.Type]++
		visitedAll++
		switch n.Type {
		case markdown.Heading:
			fmt.Printf("  h%d %q\n", n.Level, textOf(n))
		case markdown.Link:
			fmt.Printf("  link -> %-45q title=%q\n", n.Destination, n.Title)
		case markdown.Image:
			fmt.Printf("  img  -> %-45q alt=%q\n", n.Destination, textOf(n))
		case markdown.CodeBlock:
			fmt.Printf("  code info=%-6q %d bytes\n", n.Info, len(n.Literal))
		case markdown.List:
			fmt.Printf("  list kind=%-8s marker=%q start=%d tight=%v\n", n.Info, n.Literal, n.Level, n.Tight)
		}
		return true
	})
	fmt.Printf("visited %d nodes\n", visitedAll)
	fmt.Println("node type histogram:")
	for _, t := range []markdown.NodeType{
		markdown.Document, markdown.Paragraph, markdown.Heading, markdown.BlockQuote,
		markdown.List, markdown.Item, markdown.CodeBlock, markdown.HTMLBlock,
		markdown.ThematicBreak, markdown.Text, markdown.Emph, markdown.Strong,
		markdown.Link, markdown.Image, markdown.Code, markdown.HTMLInline,
		markdown.SoftBreak, markdown.HardBreak,
	} {
		fmt.Printf("  %-14s block=%-5v count=%d\n", t, t.IsBlock(), counts[t])
	}

	first := tree.FirstChild()
	last := tree.LastChild()
	fmt.Printf("FirstChild=%s LastChild=%s LastChild.Parent=%s\n",
		first.Type, last.Type, last.Parent().Type)

	// Walk can prune: returning false skips a subtree.
	var pruned int
	tree.Walk(func(n *markdown.Node) bool {
		pruned++
		return n.Type != markdown.List // do not descend into lists
	})
	fmt.Printf("pruned walk visited %d nodes (vs %d)\n", pruned, visitedAll)

	// The AST carries raw destinations: nothing is filtered at parse time.
	markdown.Parse("[x](javascript:alert(1))\n").Walk(func(n *markdown.Node) bool {
		if n.Type == markdown.Link {
			fmt.Printf("AST Destination for a javascript: link = %q (unsanitized)\n", n.Destination)
		}
		return true
	})

	// HOLE: no exported way to render a *Node. htmlRenderer and its render method
	// are unexported, and Render/RenderTo only accept a source string, so a
	// transformed or hand-built AST cannot be turned back into HTML. Uncommenting
	// the line below does not compile:
	//   fmt.Print(markdown.RenderNode(tree))
	synth := &markdown.Node{Type: markdown.Document}
	h := &markdown.Node{Type: markdown.Heading, Level: 2}
	h.AppendChild(&markdown.Node{Type: markdown.Text, Literal: "built by hand"})
	synth.AppendChild(h)
	fmt.Printf("hand-built tree: root=%s child=%s level=%d text=%q (cannot be rendered)\n",
		synth.Type, synth.FirstChild().Type, synth.FirstChild().Level, textOf(synth))

	// -----------------------------------------------------------------
	rule("7. RenderTo and error handling")
	// -----------------------------------------------------------------
	var buf bytes.Buffer
	if err := markdown.RenderTo(&buf, "# streamed\n"); err != nil {
		fmt.Fprintln(os.Stderr, "RenderTo:", err)
		os.Exit(1)
	}
	fmt.Printf("RenderTo wrote %d bytes: %s", buf.Len(), buf.String())

	writerErr := errors.New("disk on fire")
	err := markdown.RenderTo(failWriter{writerErr}, "# boom\n")
	fmt.Printf("failing writer -> %v\n", err)
	fmt.Printf("  errors.Is(err, markdown.ErrWrite)    = %v\n", errors.Is(err, markdown.ErrWrite))
	fmt.Printf("  errors.Is(err, markdown.ErrMarkdown) = %v\n", errors.Is(err, markdown.ErrMarkdown))
	fmt.Printf("  errors.Is(err, writerErr)            = %v\n", errors.Is(err, writerErr))

	// -----------------------------------------------------------------
	rule("8. Edge cases")
	// -----------------------------------------------------------------
	for _, c := range []struct{ name, src string }{
		{"empty input", ""},
		{"only whitespace", "   \n\n\t\n"},
		{"CRLF line endings", "# a\r\n\r\nb\r\n"},
		{"no trailing newline", "# a"},
		{"lone NUL byte", "a\x00b\n"},
		{"unterminated fence", "```go\nx\n"},
		{"unclosed emphasis", "*a **b\n"},
		{"deep nesting", strings.Repeat("> ", 8) + "deep\n"},
		{"invalid utf-8", "a\xffb\n"},
		{"lone surrogate ref", "&#xD800;\n"},
		{"huge numeric ref", "&#99999999;\n"},
	} {
		fmt.Printf("  %-20s -> %s\n", c.name, oneLine(markdown.Render(c.src)))
	}

	reused := markdown.New(markdown.DefaultOptions())
	fmt.Printf("reusing one *Markdown is stable: %v\n", reused.Render("# x\n") == reused.Render("# x\n"))

	fmt.Println("\nAll sections completed.")
}

// failWriter always fails, to exercise RenderTo's error path.
type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

var _ io.Writer = failWriter{}

// textOf concatenates the Text and Code descendants of n.
func textOf(n *markdown.Node) string {
	var sb strings.Builder
	n.Walk(func(c *markdown.Node) bool {
		if c.Type == markdown.Text || c.Type == markdown.Code {
			sb.WriteString(c.Literal)
		}
		return true
	})
	return sb.String()
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\\n")
}

func grepLine(html, needle string) string {
	for _, l := range strings.Split(html, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return "(not found)"
}
