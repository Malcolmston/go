// Command run is the Go-side parity runner for github.com/malcolmston/cheerio.
//
// Protocol: one JSON object per line on stdin, exactly one JSON object per line
// on stdout, in the same order. A failing case is reported as
// {"ok":false,"error":"..."} — the process never exits early.
//
// Usage: run [path/to/fixtures.json]
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malcolmston/cheerio"
)

var fixtures map[string]string

// ------------------------------------------------------------ canonical HTML
//
// Mirrors canon() in ../node/run.js exactly: attributes are lower-cased and
// sorted, valueless attributes become k="", quoting is normalised to double
// quotes, the self-closing slash is dropped and declarations are lower-cased.
// Raw-text element contents are copied verbatim.

var rawText = map[string]bool{
	"script": true, "style": true, "xmp": true, "iframe": true,
	"noembed": true, "noframes": true, "plaintext": true,
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func canon(html string) string {
	var out strings.Builder
	i, n := 0, len(html)
	at := func(k int) byte {
		if k < n {
			return html[k]
		}
		return 0
	}
	for i < n {
		if html[i] != '<' {
			out.WriteByte(html[i])
			i++
			continue
		}
		if strings.HasPrefix(html[i:], "<!--") {
			end := strings.Index(html[i+4:], "-->")
			if end < 0 {
				out.WriteString(html[i:])
				break
			}
			out.WriteString(html[i : i+4+end+3])
			i = i + 4 + end + 3
			continue
		}
		if strings.HasPrefix(html[i:], "<!") {
			end := strings.IndexByte(html[i:], '>')
			if end < 0 {
				out.WriteString(html[i:])
				break
			}
			out.WriteString(strings.ToLower(html[i : i+end+1]))
			i = i + end + 1
			continue
		}
		if strings.HasPrefix(html[i:], "</") && isAlpha(at(i+2)) {
			j := i + 2
			for j < n && html[j] != '>' {
				j++
			}
			out.WriteString("</" + strings.ToLower(strings.TrimSpace(html[i+2:j])) + ">")
			i = j + 1
			if i > n {
				i = n
			}
			continue
		}
		if !isAlpha(at(i + 1)) {
			out.WriteByte(html[i])
			i++
			continue
		}
		j := i + 1
		for j < n && !isSpace(html[j]) && html[j] != '>' && html[j] != '/' {
			j++
		}
		name := strings.ToLower(html[i+1 : j])
		type kv struct{ k, v string }
		var attrs []kv
		for j < n {
			for j < n && isSpace(html[j]) {
				j++
			}
			if j >= n {
				break
			}
			if html[j] == '>' {
				j++
				break
			}
			if html[j] == '/' {
				j++
				continue
			}
			k := j
			for k < n && !isSpace(html[k]) && html[k] != '=' && html[k] != '>' && html[k] != '/' {
				k++
			}
			key := strings.ToLower(html[j:k])
			val := ""
			j = k
			for j < n && isSpace(html[j]) {
				j++
			}
			if j < n && html[j] == '=' {
				j++
				for j < n && isSpace(html[j]) {
					j++
				}
				if j < n && (html[j] == '"' || html[j] == '\'') {
					q := html[j]
					end := strings.IndexByte(html[j+1:], q)
					if end < 0 {
						val = html[j+1:]
						j = n
					} else {
						val = html[j+1 : j+1+end]
						j = j + 1 + end + 1
					}
				} else {
					s := j
					for j < n && !isSpace(html[j]) && html[j] != '>' {
						j++
					}
					val = html[s:j]
				}
			}
			if key != "" {
				attrs = append(attrs, kv{key, val})
			}
		}
		sort.SliceStable(attrs, func(a, b int) bool { return attrs[a].k < attrs[b].k })
		out.WriteString("<" + name)
		for _, a := range attrs {
			out.WriteString(" " + a.k + "=\"" + a.v + "\"")
		}
		out.WriteString(">")
		i = j
		if rawText[name] {
			close := strings.Index(strings.ToLower(html[i:]), "</"+name)
			if close < 0 {
				out.WriteString(html[i:])
				i = n
			} else {
				out.WriteString(html[i : i+close])
				i = i + close
			}
		}
	}
	return out.String()
}

// ------------------------------------------------------------ helpers

func nodeName(n *cheerio.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case cheerio.ElementNode:
		return strings.ToLower(n.Data)
	case cheerio.TextNode:
		return "#text"
	case cheerio.CommentNode:
		return "#comment"
	case cheerio.DoctypeNode:
		return "#doctype"
	case cheerio.DocumentNode:
		return "#document"
	}
	return "#unknown"
}

type state struct {
	doc    *cheerio.Document
	sel    *cheerio.Selection
	isRoot bool
}

func outValue(st *state, kind string) (any, error) {
	if kind == "" {
		kind = "outerAll"
	}
	nodes := st.sel.Nodes()
	switch kind {
	case "docHtml":
		return canon(st.doc.Html()), nil
	case "docText":
		return st.doc.Text(), nil
	case "outerAll":
		out := []any{}
		for _, n := range nodes {
			out = append(out, canon(n.OuterHTML()))
		}
		return out, nil
	case "outerFirst":
		if len(nodes) == 0 {
			return nil, nil
		}
		return canon(nodes[0].OuterHTML()), nil
	case "innerAll":
		out := []any{}
		for _, n := range nodes {
			out = append(out, canon(n.InnerHTML()))
		}
		return out, nil
	case "tags":
		out := []any{}
		for _, n := range nodes {
			out = append(out, nodeName(n))
		}
		return out, nil
	case "texts":
		out := []any{}
		for _, n := range nodes {
			out = append(out, n.Text())
		}
		return out, nil
	case "text":
		return st.sel.Text(), nil
	case "count":
		return st.sel.Length(), nil
	case "ids":
		out := []any{}
		for _, n := range nodes {
			if v, ok := n.AttrValue("id"); ok {
				out = append(out, v)
			} else {
				out = append(out, nil)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown out kind: %s", kind)
}

type stepResult struct {
	sel      *cheerio.Selection
	value    any
	terminal bool
}

func str(a []any, i int) (string, error) {
	if i >= len(a) {
		return "", fmt.Errorf("missing string argument %d", i)
	}
	s, ok := a[i].(string)
	if !ok {
		return "", fmt.Errorf("argument %d is not a string", i)
	}
	return s, nil
}

func num(a []any, i int) (int, error) {
	if i >= len(a) {
		return 0, fmt.Errorf("missing numeric argument %d", i)
	}
	f, ok := a[i].(float64)
	if !ok {
		return 0, fmt.Errorf("argument %d is not a number", i)
	}
	return int(f), nil
}

func strs(a []any) ([]string, error) {
	out := make([]string, 0, len(a))
	for i := range a {
		s, err := str(a, i)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func optSel(sel *cheerio.Selection, a []any, f0 func() *cheerio.Selection, f1 func(string) *cheerio.Selection) (stepResult, error) {
	if len(a) == 0 {
		return stepResult{sel: f0()}, nil
	}
	s, err := str(a, 0)
	if err != nil {
		return stepResult{}, err
	}
	return stepResult{sel: f1(s)}, nil
}

func nullable(v string, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

func applyStep(st *state, step []any) (stepResult, error) {
	if len(step) == 0 {
		return stepResult{}, fmt.Errorf("empty step")
	}
	m, err := str(step, 0)
	if err != nil {
		return stepResult{}, err
	}
	a := step[1:]
	sel := st.sel
	wasRoot := st.isRoot
	st.isRoot = false

	term := func(v any) (stepResult, error) { return stepResult{value: v, terminal: true}, nil }

	switch m {
	// ---- traversal
	case "find":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		if wasRoot {
			return stepResult{sel: st.doc.Find(s)}, nil
		}
		return stepResult{sel: sel.Find(s)}, nil
	case "children":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Children() },
			func(s string) *cheerio.Selection { return sel.Children(s) })
	case "contents":
		return stepResult{sel: sel.Contents()}, nil
	case "parent":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Parent() },
			func(s string) *cheerio.Selection { return sel.Parent(s) })
	case "parents":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Parents() },
			func(s string) *cheerio.Selection { return sel.Parents(s) })
	case "parentsUntil":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		if len(a) > 1 {
			f, err := str(a, 1)
			if err != nil {
				return stepResult{}, err
			}
			return stepResult{sel: sel.ParentsUntil(s, f)}, nil
		}
		return stepResult{sel: sel.ParentsUntil(s)}, nil
	case "closest":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.Closest(s)}, nil
	case "siblings":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Siblings() },
			func(s string) *cheerio.Selection { return sel.Siblings(s) })
	case "next":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Next() },
			func(s string) *cheerio.Selection { return sel.Next(s) })
	case "prev":
		return optSel(sel, a, func() *cheerio.Selection { return sel.Prev() },
			func(s string) *cheerio.Selection { return sel.Prev(s) })
	case "nextAll":
		return optSel(sel, a, func() *cheerio.Selection { return sel.NextAll() },
			func(s string) *cheerio.Selection { return sel.NextAll(s) })
	case "prevAll":
		return optSel(sel, a, func() *cheerio.Selection { return sel.PrevAll() },
			func(s string) *cheerio.Selection { return sel.PrevAll(s) })
	case "nextUntil", "prevUntil":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		var filters []string
		if len(a) > 1 {
			f, err := str(a, 1)
			if err != nil {
				return stepResult{}, err
			}
			filters = append(filters, f)
		}
		if m == "nextUntil" {
			return stepResult{sel: sel.NextUntil(s, filters...)}, nil
		}
		return stepResult{sel: sel.PrevUntil(s, filters...)}, nil
	case "filter", "not", "has", "add", "addSelection":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		switch m {
		case "filter":
			return stepResult{sel: sel.Filter(s)}, nil
		case "not":
			return stepResult{sel: sel.Not(s)}, nil
		case "has":
			return stepResult{sel: sel.Has(s)}, nil
		case "add":
			return stepResult{sel: sel.Add(s)}, nil
		default:
			return stepResult{sel: sel.AddSelection(st.doc.Find(s))}, nil
		}
	case "addBack":
		if len(a) == 0 {
			return stepResult{sel: sel.AddBack()}, nil
		}
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.AddBack(s)}, nil
	case "end":
		return stepResult{sel: sel.End()}, nil
	case "eq":
		i, err := num(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.Eq(i)}, nil
	case "first":
		return stepResult{sel: sel.First()}, nil
	case "last":
		return stepResult{sel: sel.Last()}, nil
	case "slice":
		s0, err := num(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		if len(a) < 2 {
			return stepResult{}, fmt.Errorf("Go Slice requires both bounds")
		}
		s1, err := num(a, 1)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.Slice(s0, s1)}, nil
	case "clone":
		return stepResult{sel: sel.Clone()}, nil

	// ---- accessors
	case "attr":
		switch len(a) {
		case 0:
			m := sel.Attrs()
			out := map[string]any{}
			for k, v := range m {
				out[k] = v
			}
			return term(out)
		case 1:
			k, err := str(a, 0)
			if err != nil {
				return stepResult{}, err
			}
			v, ok := sel.Attr(k)
			return term(nullable(v, ok))
		default:
			k, err := str(a, 0)
			if err != nil {
				return stepResult{}, err
			}
			v, err := str(a, 1)
			if err != nil {
				return stepResult{}, err
			}
			return stepResult{sel: sel.SetAttr(k, v)}, nil
		}
	case "removeAttr":
		k, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.RemoveAttr(k)}, nil
	case "prop":
		k, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		if len(a) == 1 {
			v, ok := sel.Prop(k)
			return term(nullable(v, ok))
		}
		v, err := str(a, 1)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.SetProp(k, v)}, nil
	case "data":
		switch len(a) {
		case 0:
			out := map[string]any{}
			for k, v := range sel.DataMap() {
				out[k] = v
			}
			return term(out)
		case 1:
			k, err := str(a, 0)
			if err != nil {
				return stepResult{}, err
			}
			v, ok := sel.Data(k)
			return term(nullable(v, ok))
		default:
			k, err := str(a, 0)
			if err != nil {
				return stepResult{}, err
			}
			v, err := str(a, 1)
			if err != nil {
				return stepResult{}, err
			}
			return stepResult{sel: sel.SetData(k, v)}, nil
		}
	case "removeData":
		keys, err := strs(a)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.RemoveData(keys...)}, nil
	case "val":
		if len(a) == 0 {
			return term(sel.Val())
		}
		v, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.SetVal(v)}, nil
	case "text":
		if len(a) == 0 {
			return term(sel.Text())
		}
		v, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.SetText(v)}, nil
	case "html":
		if len(a) == 0 {
			if sel.Length() == 0 {
				return term(nil)
			}
			return term(canon(sel.Html()))
		}
		v, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.SetHtml(v)}, nil
	case "toString":
		return term(canon(sel.ToString()))
	case "outerHtml":
		return term(canon(sel.OuterHtml()))
	case "hasClass":
		c, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return term(sel.HasClass(c))
	case "css":
		p, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		if len(a) == 1 {
			return term(sel.Css(p))
		}
		v, err := str(a, 1)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.SetCss(p, v)}, nil
	case "tagName":
		return term(sel.TagName())
	case "length":
		return term(sel.Length())
	case "index":
		if len(a) == 0 {
			return term(sel.Index())
		}
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return term(sel.IndexSelector(s))
	case "is":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return term(sel.Is(s))
	case "serialize":
		return term(sel.Serialize())
	case "serializeArray":
		out := []any{}
		for _, f := range sel.SerializeArray() {
			out = append(out, map[string]any{"name": f.Name, "value": f.Value})
		}
		return term(out)

	// ---- class / manipulation
	case "addClass":
		c, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.AddClass(c)}, nil
	case "removeClass":
		if len(a) == 0 {
			return stepResult{sel: sel.RemoveClass()}, nil
		}
		c, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.RemoveClass(c)}, nil
	case "toggleClass":
		c, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.ToggleClass(c)}, nil
	case "append", "prepend", "before", "after":
		h, err := strs(a)
		if err != nil {
			return stepResult{}, err
		}
		switch m {
		case "append":
			return stepResult{sel: sel.Append(h...)}, nil
		case "prepend":
			return stepResult{sel: sel.Prepend(h...)}, nil
		case "before":
			return stepResult{sel: sel.Before(h...)}, nil
		default:
			return stepResult{sel: sel.After(h...)}, nil
		}
	case "appendTo", "prependTo", "insertBefore", "insertAfter", "replaceWith",
		"wrap", "wrapAll", "wrapInner":
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		switch m {
		case "appendTo":
			return stepResult{sel: sel.AppendTo(s)}, nil
		case "prependTo":
			return stepResult{sel: sel.PrependTo(s)}, nil
		case "insertBefore":
			return stepResult{sel: sel.InsertBefore(s)}, nil
		case "insertAfter":
			return stepResult{sel: sel.InsertAfter(s)}, nil
		case "replaceWith":
			return stepResult{sel: sel.ReplaceWith(s)}, nil
		case "wrap":
			return stepResult{sel: sel.Wrap(s)}, nil
		case "wrapAll":
			return stepResult{sel: sel.WrapAll(s)}, nil
		default:
			return stepResult{sel: sel.WrapInner(s)}, nil
		}
	case "remove":
		if len(a) == 0 {
			return stepResult{sel: sel.Remove()}, nil
		}
		s, err := str(a, 0)
		if err != nil {
			return stepResult{}, err
		}
		return stepResult{sel: sel.Remove(s)}, nil
	case "empty":
		return stepResult{sel: sel.Empty()}, nil
	case "unwrap":
		if len(a) != 0 {
			return stepResult{}, fmt.Errorf("Go Unwrap takes no selector argument")
		}
		return stepResult{sel: sel.Unwrap()}, nil
	}
	return stepResult{}, fmt.Errorf("unknown step: %s", m)
}

func fixture(name string) (string, error) {
	h, ok := fixtures[name]
	if !ok {
		return "", fmt.Errorf("unknown fixture: %s", name)
	}
	return h, nil
}

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

func handle(req request) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	a := req.Args
	switch req.Fn {
	case "chain":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		doc := cheerio.Load(html)
		st := &state{doc: doc, sel: doc.Root(), isRoot: true}
		var rawSteps []any
		if len(a) > 1 && a[1] != nil {
			rs, ok := a[1].([]any)
			if !ok {
				return nil, fmt.Errorf("steps must be an array")
			}
			rawSteps = rs
		}
		terminated := false
		var value any
		for _, s := range rawSteps {
			if terminated {
				return nil, fmt.Errorf("step after terminal accessor")
			}
			step, ok := s.([]any)
			if !ok {
				return nil, fmt.Errorf("step must be an array")
			}
			r, err := applyStep(st, step)
			if err != nil {
				return nil, err
			}
			if r.terminal {
				value, terminated = r.value, true
			} else {
				st.sel = r.sel
			}
		}
		if terminated {
			return value, nil
		}
		kind := ""
		if len(a) > 2 && a[2] != nil {
			kind, err = str(a, 2)
			if err != nil {
				return nil, err
			}
		}
		return outValue(st, kind)

	case "parse":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		return canon(cheerio.Load(html).Html()), nil

	case "parseText":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		return cheerio.Load(html).Text(), nil

	case "static.text", "static.html":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		selector, err := str(a, 1)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		nodes := cheerio.Load(html).Find(selector).Nodes()
		if req.Fn == "static.text" {
			return cheerio.TextOf(nodes...), nil
		}
		return canon(cheerio.RenderNodes(nodes...)), nil

	case "static.contains":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		s1, err := str(a, 1)
		if err != nil {
			return nil, err
		}
		s2, err := str(a, 2)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		doc := cheerio.Load(html)
		return cheerio.Contains(doc.Find(s1).Get(0), doc.Find(s2).Get(0)), nil

	case "static.merge":
		name, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		s1, err := str(a, 1)
		if err != nil {
			return nil, err
		}
		s2, err := str(a, 2)
		if err != nil {
			return nil, err
		}
		html, err := fixture(name)
		if err != nil {
			return nil, err
		}
		doc := cheerio.Load(html)
		merged := cheerio.Merge(doc.Find(s1).ToArray(), doc.Find(s2).Nodes()...)
		tags := []any{}
		for _, n := range merged {
			tags = append(tags, nodeName(n))
		}
		return map[string]any{"length": len(merged), "tags": tags}, nil

	case "static.parseHTML":
		h, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		nodes := cheerio.ParseHTML(h)
		if nodes == nil {
			return nil, nil // upstream returns null for empty input
		}
		tags := []any{}
		for _, n := range nodes {
			tags = append(tags, nodeName(n))
		}
		return map[string]any{
			"length": len(nodes),
			"html":   canon(cheerio.RenderNodes(nodes...)),
			"tags":   tags,
		}, nil

	case "compile":
		s, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		if _, err := cheerio.Compile(s); err != nil {
			return nil, err
		}
		return true, nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

func main() {
	path := filepath.Join("..", "fixtures", "fixtures.json")
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot read fixtures:", err)
		os.Exit(2)
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		fmt.Fprintln(os.Stderr, "cannot parse fixtures:", err)
		os.Exit(2)
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		resp := map[string]any{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp["id"] = nil
			resp["ok"] = false
			resp["error"] = err.Error()
		} else if v, err := handle(req); err != nil {
			resp["id"] = req.ID
			resp["ok"] = false
			resp["error"] = err.Error()
		} else {
			resp["id"] = req.ID
			resp["ok"] = true
			resp["value"] = v
		}
		b, err := json.Marshal(resp)
		if err != nil {
			b, _ = json.Marshal(map[string]any{"id": req.ID, "ok": false, "error": "unencodable value: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
	out.Flush()
}
