// Command run is the Go-side parity runner for github.com/malcolmston/puppeteer.
//
// The port's selector engine is only reachable through a Page that has completed
// a real HTTP navigation: Parse() hands back a *Node that nothing exported can
// query, and there is no SetContent. This runner therefore stands up a loopback
// httptest server that serves every fixture at /f/<name> (plus the HTTP
// behaviour routes) and navigates to it — see ../COVERAGE.md ("Usability").
//
// Protocol: one JSON object per line on stdin, exactly one JSON object per line
// on stdout, in the same order. A failing case is reported as
// {"ok":false,"error":"..."} — the process never exits early.
//
// Usage: run [path/to/fixtures.json]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	pp "github.com/malcolmston/puppeteer"
)

var (
	fixtures map[string]string
	srv      *httptest.Server
	base     string
)

// ------------------------------------------------------------ canonical HTML
//
// Mirrors canon() in ../node/run.js exactly: tag and attribute names are
// lower-cased, attributes are sorted by name, valueless attributes become k="",
// quoting is normalised to double quotes, the self-closing slash is dropped and
// declarations are lower-cased. Raw-text element contents are copied verbatim.

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

func nodeName(n *pp.Node) string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case pp.ElementNode:
		return strings.ToLower(n.Data)
	case pp.TextNode:
		return "#text"
	case pp.CommentNode:
		return "#comment"
	case pp.DoctypeNode:
		return "#doctype"
	case pp.DocumentNode:
		return "#document"
	default:
		return "#unknown"
	}
}

// isDocumentSource mirrors isDocumentSource() in ../node/run.js: a fixture whose
// first markup is <!doctype> or <html> is treated as a whole document.
var documentSourceRe = regexp.MustCompile(`(?i)^\s*(<!doctype|<html[\s>])`)

func isDocumentSource(html string) bool {
	return documentSourceRe.MatchString(html)
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func norm(s string) string {
	return strings.ReplaceAll(s, base, "SRV")
}

// ------------------------------------------------------------ fixture server

func fixture(name string) (string, error) {
	h, ok := fixtures[name]
	if !ok {
		return "", fmt.Errorf("unknown fixture: %s", name)
	}
	return h, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	send := func(code int, body string, headers map[string][]string) {
		for k, vs := range headers {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}

	if strings.HasPrefix(p, "/f/") {
		name, err := url.PathUnescape(strings.TrimPrefix(p, "/f/"))
		if err != nil {
			send(400, "", nil)
			return
		}
		h, err := fixture(name)
		if err != nil {
			send(500, err.Error(), nil)
			return
		}
		send(200, h, nil)
		return
	}
	if strings.HasPrefix(p, "/status/") {
		code, err := strconv.Atoi(strings.TrimPrefix(p, "/status/"))
		if err != nil {
			send(400, "", nil)
			return
		}
		if code == 204 || code == 304 {
			w.WriteHeader(code)
			return
		}
		send(code, "<title>s"+strconv.Itoa(code)+"</title><p id=\"s\">status "+strconv.Itoa(code)+"</p>", nil)
		return
	}
	switch p {
	case "/redirect/absolute":
		send(302, "", map[string][]string{"Location": {base + "/final"}})
	case "/redirect/relative":
		send(302, "", map[string][]string{"Location": {"../final"}})
	case "/redirect/pathonly":
		send(302, "", map[string][]string{"Location": {"/final"}})
	case "/redirect/chain/1":
		send(302, "", map[string][]string{"Location": {"/redirect/chain/2"}})
	case "/redirect/chain/2":
		send(302, "", map[string][]string{"Location": {"/redirect/chain/3"}})
	case "/redirect/chain/3":
		send(302, "", map[string][]string{"Location": {"/final"}})
	case "/redirect/permanent":
		send(301, "", map[string][]string{"Location": {"/final"}})
	case "/final":
		send(200, "<title>final</title><p id=\"f\">final page</p>", nil)
	case "/deep/a/b/page":
		send(200, "<title>page</title><link rel=\"stylesheet\" href=\"/css/site.css\">"+
			"<a href=\"sibling\">s</a><a href=\"../other\">o</a><a href=\"/final\">r</a>"+
			"<a href=\"https://example.com/w\">w</a><a href=\"sibling\">dup</a>"+
			"<map><area href=\"/area-target\"></map>", nil)
	case "/deep/a/b/sibling":
		send(200, "<title>sibling</title>", nil)
	case "/deep/a/other":
		send(200, "<title>other</title>", nil)
	case "/cookie/set":
		send(200, "<title>set</title>", map[string][]string{"Set-Cookie": {"sid=abc123; Path=/"}})
	case "/cookie/set-scoped":
		send(200, "<title>set-scoped</title>", map[string][]string{"Set-Cookie": {"scoped=zzz; Path=/scoped"}})
	case "/cookie/set-two":
		send(200, "<title>set-two</title>", map[string][]string{"Set-Cookie": {"one=1; Path=/", "two=2; Path=/"}})
	case "/cookie/echo", "/scoped/echo":
		send(200, "<p id=\"c\">"+r.Header.Get("Cookie")+"</p>", nil)
	case "/echo":
		body, _ := io.ReadAll(r.Body)
		esc := strings.ReplaceAll(string(body), "&", "&amp;")
		esc = strings.ReplaceAll(esc, "<", "&lt;")
		send(200, "<p id=\"m\">"+r.Method+"</p>"+
			"<p id=\"ct\">"+r.Header.Get("Content-Type")+"</p>"+
			"<p id=\"q\">"+r.URL.RawQuery+"</p>"+
			"<p id=\"b\">"+esc+"</p>", nil)
	default:
		send(404, "<title>404</title><p id=\"s\">not found</p>", nil)
	}
}

// ------------------------------------------------------------ navigation

// newPage returns a page on a brand-new browser, so every case starts with an
// empty cookie jar and empty history.
func newPage(followRedirects bool) (*pp.Browser, *pp.Page, error) {
	opts := &pp.LaunchOptions{}
	if !followRedirects {
		f := false
		opts.FollowRedirects = &f
	}
	b, err := pp.Launch(opts)
	if err != nil {
		return nil, nil, err
	}
	return b, b.NewPage(), nil
}

// loadFixture serves the named fixture over HTTP and navigates to it, which is
// the only way to reach the port's selector engine.
func loadFixture(name string) (*pp.Browser, *pp.Page, error) {
	if _, err := fixture(name); err != nil {
		return nil, nil, err
	}
	b, p, err := newPage(true)
	if err != nil {
		return nil, nil, err
	}
	if _, err := p.Goto(base + "/f/" + url.PathEscape(name)); err != nil {
		b.Close()
		return nil, nil, err
	}
	return b, p, nil
}

func texts(els []*pp.Element) []string {
	out := make([]string, 0, len(els))
	for _, e := range els {
		out = append(out, e.TextContent())
	}
	return out
}

func outValue(els []*pp.Element, kind string) (any, error) {
	if kind == "" {
		kind = "count"
	}
	switch kind {
	case "count":
		return len(els), nil
	case "tags":
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, e.TagName())
		}
		return out, nil
	case "ids":
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, e.AttrOr("id", ""))
		}
		return out, nil
	case "texts":
		return texts(els), nil
	case "innerTexts":
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, e.InnerText())
		}
		return out, nil
	case "outerAll":
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, canon(e.OuterHTML()))
		}
		return out, nil
	case "innerAll":
		out := make([]string, 0, len(els))
		for _, e := range els {
			out = append(out, canon(e.InnerHTML()))
		}
		return out, nil
	case "classes":
		out := make([][]string, 0, len(els))
		for _, e := range els {
			cl := e.ClassList()
			if cl == nil {
				cl = []string{}
			}
			out = append(out, cl)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown out kind: %s", kind)
	}
}

func pageTitle(p *pp.Page) string {
	return collapse(p.Title())
}

// pagePath reports the path of Page.URL(), the port's analogue of page.url().
func pagePath(p *pp.Page) string {
	u, err := url.Parse(p.URL())
	if err != nil {
		return p.URL()
	}
	return u.Path
}

func str(args []any, i int) string {
	if i >= len(args) {
		return ""
	}
	s, _ := args[i].(string)
	return s
}

// ------------------------------------------------------------ forms

func sortedPairs(v url.Values) []map[string]string {
	out := []map[string]string{}
	for name, vals := range v {
		for _, val := range vals {
			out = append(out, map[string]string{"name": name, "value": val})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i]["name"] != out[j]["name"] {
			return out[i]["name"] < out[j]["name"]
		}
		return out[i]["value"] < out[j]["value"]
	})
	return out
}

// ------------------------------------------------------------ dispatch

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

func handle(req request) (any, error) {
	a := req.Args
	switch req.Fn {
	// ---- parsing
	case "parse":
		name := str(a, 0)
		b, p, err := loadFixture(name)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		if src, _ := fixture(name); isDocumentSource(src) {
			// The oracle loads a self-declared document in document mode, so
			// compare against the port's browser-equivalent serialisation.
			return canon(p.NormalizedHTML()), nil
		}
		return canon(p.HTML()), nil

	case "tree":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		var kids []*pp.Node
		if sel := str(a, 1); sel == "" {
			doc := p.Document()
			if doc == nil {
				return nil, fmt.Errorf("no document")
			}
			kids = doc.Children
		} else {
			el, err := p.QuerySelector(sel)
			if err != nil {
				return nil, err
			}
			if el == nil {
				return nil, fmt.Errorf("no element matches %s", sel)
			}
			kids = el.Node().Children
		}
		out := make([]string, 0, len(kids))
		for _, k := range kids {
			out = append(out, nodeName(k))
		}
		return out, nil

	case "title":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return pageTitle(p), nil

	case "text":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		t, ok, err := p.TextContent(str(a, 1))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("no element matches %s", str(a, 1))
		}
		return t, nil

	// ---- selectors
	case "select":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		els, err := p.QuerySelectorAll(str(a, 1))
		if err != nil {
			return nil, err
		}
		return outValue(els, str(a, 2))

	case "matches":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		el, err := p.QuerySelector(str(a, 1))
		if err != nil {
			return nil, err
		}
		if el == nil {
			return nil, fmt.Errorf("no element matches %s", str(a, 1))
		}
		return el.Matches(str(a, 2))

	case "closest":
		b, p, err := loadFixture(str(a, 0))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		el, err := p.QuerySelector(str(a, 1))
		if err != nil {
			return nil, err
		}
		if el == nil {
			return nil, fmt.Errorf("no element matches %s", str(a, 1))
		}
		c, err := el.Closest(str(a, 2))
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, nil
		}
		return c.AttrOr("id", ""), nil

	case "compile":
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		if _, err := p.Goto(base + "/f/deep-tree"); err != nil {
			return nil, err
		}
		if _, err := p.QuerySelectorAll(str(a, 0)); err != nil {
			return nil, err
		}
		return true, nil

	// ---- HTTP / navigation
	case "http":
		return httpScenario(str(a, 0))

	// ---- forms
	case "form.serialize":
		b, f, err := loadForm(str(a, 0), str(a, 1))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return f.Serialize(), nil

	case "form.fields":
		b, f, err := loadForm(str(a, 0), str(a, 1))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return sortedPairs(f.Values()), nil

	case "form.request":
		b, f, err := loadForm(str(a, 0), str(a, 1))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		r, err := f.BuildRequest(context.Background())
		if err != nil {
			return nil, err
		}
		body := ""
		if r.Body != nil {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			body = string(raw)
		}
		return map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"query":       r.URL.RawQuery,
			"contentType": r.Header.Get("Content-Type"),
			"body":        body,
		}, nil

	case "form.submit":
		b, p, f, err := loadFormPage(str(a, 0), str(a, 1))
		if err != nil {
			return nil, err
		}
		defer b.Close()
		resp, err := f.Submit()
		if err != nil {
			return nil, err
		}
		get := func(sel string) string {
			t, _, err := p.TextContent(sel)
			if err != nil {
				return ""
			}
			return t
		}
		return map[string]any{
			"status":      resp.StatusCode,
			"method":      get("#m"),
			"contentType": get("#ct"),
			"query":       get("#q"),
			"body":        get("#b"),
		}, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func httpScenario(name string) (any, error) {
	switch name {
	case "status-200", "status-201", "status-404", "status-500", "status-204":
		code := strings.TrimPrefix(name, "status-")
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		resp, err := p.Goto(base + "/status/" + code)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"status": resp.StatusCode,
			"ok":     resp.OK(),
			"title":  pageTitle(p),
			"path":   pagePath(p),
		}, nil

	case "redirect-absolute", "redirect-relative", "redirect-pathonly", "redirect-permanent":
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		resp, err := p.Goto(base + "/redirect/" + strings.TrimPrefix(name, "redirect-"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": resp.StatusCode, "path": pagePath(p), "title": pageTitle(p)}, nil

	case "redirect-chain":
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		resp, err := p.Goto(base + "/redirect/chain/1")
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": resp.StatusCode, "path": pagePath(p), "title": pageTitle(p)}, nil

	case "redirect-no-follow":
		b, p, err := newPage(false)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		resp, err := p.Goto(base + "/redirect/pathonly")
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": resp.StatusCode, "location": norm(resp.Header.Get("Location"))}, nil

	case "relative-goto-dotdot", "relative-goto-sibling", "relative-goto-absolute-path":
		rel := map[string]string{
			"relative-goto-dotdot":        "../other",
			"relative-goto-sibling":       "sibling",
			"relative-goto-absolute-path": "/final",
		}[name]
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		if _, err := p.Goto(base + "/deep/a/b/page"); err != nil {
			return nil, err
		}
		resp, err := p.Goto(rel)
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": resp.StatusCode, "path": pagePath(p), "title": pageTitle(p)}, nil

	case "links-resolved":
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		if _, err := p.Goto(base + "/deep/a/b/page"); err != nil {
			return nil, err
		}
		ls := p.Links()
		out := make([]string, 0, len(ls))
		for _, l := range ls {
			out = append(out, norm(l))
		}
		return out, nil

	case "cookie-roundtrip":
		return cookieScenario("/cookie/set", "/cookie/echo", false)
	case "cookie-two":
		return cookieScenario("/cookie/set-two", "/cookie/echo", true)
	case "cookie-path-excluded":
		return cookieScenario("/cookie/set-scoped", "/cookie/echo", false)
	case "cookie-path-included":
		return cookieScenario("/cookie/set-scoped", "/scoped/echo", false)
	case "cookie-not-leaked-before-set":
		return cookieScenario("", "/cookie/echo", false)

	case "echo-get-query":
		b, p, err := newPage(true)
		if err != nil {
			return nil, err
		}
		defer b.Close()
		if _, err := p.Goto(base + "/echo?a=1&b=2"); err != nil {
			return nil, err
		}
		get := func(sel string) string {
			t, _, _ := p.TextContent(sel)
			return t
		}
		return map[string]any{"method": get("#m"), "query": get("#q"), "body": get("#b")}, nil

	case "page-title":
		b, p, err := loadFixture("full-document")
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return pageTitle(p), nil

	case "page-content":
		b, p, err := loadFixture("full-document")
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return canon(p.Content()), nil

	case "page-html":
		b, p, err := loadFixture("full-document")
		if err != nil {
			return nil, err
		}
		defer b.Close()
		return canon(p.HTML()), nil

	default:
		return nil, fmt.Errorf("unknown http scenario: %s", name)
	}
}

func cookieScenario(setPath, echoPath string, sortCookies bool) (any, error) {
	b, p, err := newPage(true)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if setPath != "" {
		if _, err := p.Goto(base + setPath); err != nil {
			return nil, err
		}
	}
	if _, err := p.Goto(base + echoPath); err != nil {
		return nil, err
	}
	got, _, err := p.TextContent("#c")
	if err != nil {
		return nil, err
	}
	if sortCookies {
		parts := strings.Split(got, "; ")
		keep := parts[:0]
		for _, s := range parts {
			if s != "" {
				keep = append(keep, s)
			}
		}
		sort.Strings(keep)
		got = strings.Join(keep, "; ")
	}
	return got, nil
}

func loadFormPage(fixtureName, selector string) (*pp.Browser, *pp.Page, *pp.Form, error) {
	if selector == "" {
		selector = "#f"
	}
	b, p, err := loadFixture(fixtureName)
	if err != nil {
		return nil, nil, nil, err
	}
	f, err := p.FormBySelector(selector)
	if err != nil {
		b.Close()
		return nil, nil, nil, err
	}
	return b, p, f, nil
}

func loadForm(fixtureName, selector string) (*pp.Browser, *pp.Form, error) {
	b, _, f, err := loadFormPage(fixtureName, selector)
	return b, f, err
}

// ------------------------------------------------------------ main loop

type reply struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func main() {
	path := filepath.Join("..", "fixtures", "fixtures.json")
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal: reading fixtures:", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		fmt.Fprintln(os.Stderr, "fatal: parsing fixtures:", err)
		os.Exit(1)
	}

	srv = httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()
	base = srv.URL

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<25)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		rep := reply{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			rep = reply{OK: false, Error: "undecodable request: " + err.Error()}
		} else {
			rep.ID = req.ID
			v, err := safeHandle(req)
			if err != nil {
				rep.OK = false
				rep.Error = err.Error()
			} else {
				rep.OK = true
				rep.Value = v
			}
		}
		b, err := json.Marshal(rep)
		if err != nil {
			b, _ = json.Marshal(reply{ID: req.ID, OK: false, Error: "unencodable reply: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}

// safeHandle turns a panic in the library under test into an ok:false answer so
// one bad case never takes the runner (and the rest of the suite) down.
func safeHandle(req request) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v = nil
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return handle(req)
}
