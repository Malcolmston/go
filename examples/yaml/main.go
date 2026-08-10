// Command yamldemo exercises github.com/malcolmston/yaml: marshalling and
// unmarshalling Go structs and maps, round-trip fidelity, anchors and aliases,
// multi-document streams, custom tags, the Node API, custom
// Marshaler/Unmarshaler implementations, and error handling.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/malcolmston/yaml"
)

type Server struct {
	Host    string            `yaml:"host"`
	Port    int               `yaml:"port"`
	Tags    []string          `yaml:"tags,flow"`
	Labels  map[string]string `yaml:"labels,omitempty"`
	Enabled bool              `yaml:"enabled"`
	Secret  string            `yaml:"-"`
}

type Timeouts struct {
	Read  time.Duration `yaml:"read"`
	Write time.Duration `yaml:"write"`
}

type Meta struct {
	Owner   string `yaml:"owner"`
	Version int    `yaml:"version"`
}

type Config struct {
	Meta     `yaml:",inline"`
	Name     string    `yaml:"name"`
	Replicas int       `yaml:"replicas"`
	Servers  []Server  `yaml:"servers"`
	Timeouts Timeouts  `yaml:"timeouts"`
	Notes    string    `yaml:"notes"`
	Created  time.Time `yaml:"created"`
	Extra    any       `yaml:"extra"`
}

func section(title string) {
	fmt.Printf("\n=== %s ===\n", title)
}

func indent(s string) string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return "  (empty)"
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// ---------------------------------------------------------------------------
// Custom Marshaler / Unmarshaler
// ---------------------------------------------------------------------------

// Semver marshals as a single string and parses itself back from a scalar node.
type Semver struct{ Major, Minor, Patch int }

func (s Semver) MarshalYAML() (any, error) {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch), nil
}

func (s *Semver) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("semver: want scalar, got %s at line %d", n.Kind, n.Line)
	}
	if _, err := fmt.Sscanf(n.Value, "%d.%d.%d", &s.Major, &s.Minor, &s.Patch); err != nil {
		return fmt.Errorf("semver: %q: %w", n.Value, err)
	}
	return nil
}

// Celsius round-trips through a *Node so it can carry a custom tag and style.
type Celsius float64

func (c Celsius) MarshalYAML() (any, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!celsius",
		Value: fmt.Sprintf("%.1f", float64(c)),
		Style: yaml.TaggedStyle,
		// The "# " has to be supplied by hand: the emitter writes LineComment
		// verbatim (see the HOLE note in nodeAPI).
		LineComment: "# degrees",
	}, nil
}

func (c *Celsius) UnmarshalYAML(n *yaml.Node) error {
	if n.Tag != "!celsius" && n.Tag != yaml.FloatTag && n.Tag != yaml.IntTag {
		return fmt.Errorf("celsius: unexpected tag %q", n.Tag)
	}
	var f float64
	if _, err := fmt.Sscanf(n.Value, "%g", &f); err != nil {
		return err
	}
	*c = Celsius(f)
	return nil
}

type Release struct {
	Version Semver  `yaml:"version"`
	Temp    Celsius `yaml:"temp"`
}

func main() {
	marshalStructs()
	marshalMaps()
	roundTrip()
	anchorsAliases()
	multiDocument()
	streamEncode()
	nodeAPI()
	customTags()
	customMarshalers()
	errorHandling()
	fmt.Println("\nall sections complete")
}

// ---------------------------------------------------------------------------

func marshalStructs() {
	section("1. Marshal a Go struct")
	cfg := Config{
		Meta:     Meta{Owner: "platform", Version: 3},
		Name:     "prod-cluster",
		Replicas: 4,
		Servers: []Server{
			{Host: "a.example.com", Port: 8080, Tags: []string{"edge", "eu"},
				Labels: map[string]string{"tier": "gold"}, Enabled: true, Secret: "hunter2"},
			{Host: "b.example.com", Port: 9090, Tags: []string{"core"}, Enabled: false},
		},
		Timeouts: Timeouts{Read: 5 * time.Second, Write: 1500 * time.Millisecond},
		Notes:    "line one\nline two\nline three\n",
		Created:  time.Date(2026, 8, 10, 12, 30, 0, 0, time.UTC),
		Extra:    map[string]any{"ratio": 0.75, "flags": []any{"x", "y"}, "nested": map[string]any{"deep": true}},
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(indent(string(out)))
	fmt.Println("note: `yaml:\"-\"` dropped Secret; `,inline` hoisted Meta; `,flow` inlined tags;")
	fmt.Println("      `,omitempty` dropped the second server's empty labels.")

	section("2. Unmarshal back into the struct")
	var got Config
	if err := yaml.Unmarshal(out, &got); err != nil {
		fmt.Println("unmarshal error:", err)
		return
	}
	fmt.Printf("  owner=%s version=%d name=%s replicas=%d\n", got.Owner, got.Version, got.Name, got.Replicas)
	fmt.Printf("  servers=%d first=%s:%d tags=%v labels=%v\n",
		len(got.Servers), got.Servers[0].Host, got.Servers[0].Port, got.Servers[0].Tags, got.Servers[0].Labels)
	fmt.Printf("  timeouts read=%s write=%s\n", got.Timeouts.Read, got.Timeouts.Write)
	fmt.Printf("  created=%s (type %T)\n", got.Created.Format(time.RFC3339), got.Created)
	fmt.Printf("  notes=%q\n", got.Notes)
	fmt.Printf("  extra=%#v\n", got.Extra)

	// Compare everything except the intentionally-dropped Secret field.
	want := cfg
	want.Servers[0].Secret = ""
	fmt.Printf("  struct equal (ignoring `yaml:\"-\"` field): %v\n", reflect.DeepEqual(want, got))
}

func marshalMaps() {
	section("3. Marshal / unmarshal plain maps and any")
	m := map[string]any{
		"str":       "plain",
		"quoted":    "yes",  // YAML 1.2: a string, but must be quoted to stay one
		"numberish": "007",  // would resolve to int if left plain
		"nullish":   "null", // would resolve to null if left plain
		"int":       42,
		"float":     3.5,
		"bool":      true,
		"null":      nil,
		"list":      []any{1, "two", 3.0, nil, false},
		"map":       map[string]int{"b": 2, "a": 1},
		"bytes":     []byte{0x00, 0x01, 0xff},
	}
	out, err := yaml.Marshal(m)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(indent(string(out)))

	var back map[string]any
	if err := yaml.Unmarshal(out, &back); err != nil {
		fmt.Println("unmarshal error:", err)
		return
	}
	for _, k := range []string{"str", "quoted", "numberish", "nullish", "int", "float", "bool", "null", "bytes"} {
		fmt.Printf("  %-10s %#v\n", k, back[k])
	}
	fmt.Printf("  map keys sorted on output: %v\n", strings.Contains(string(out), "a: 1"))
}

func roundTrip() {
	section("4. Round-trip fidelity and idempotence")
	src := []byte(`
name: widget
count: 3
ratio: 0.5
empty:
truthy: true
notbool: yes
list:
  - one
  - two
nested:
  a:
    b:
      c: deep
literal: |
  keep
  these
  lines
folded: >
  fold
  these
quoted: "has: colon"
sq: 'it''s'
`)
	var v1 any
	if err := yaml.Unmarshal(src, &v1); err != nil {
		fmt.Println("unmarshal error:", err)
		return
	}
	enc1, err := yaml.Marshal(v1)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	var v2 any
	if err := yaml.Unmarshal(enc1, &v2); err != nil {
		fmt.Println("second unmarshal error:", err)
		return
	}
	enc2, err := yaml.Marshal(v2)
	if err != nil {
		fmt.Println("second marshal error:", err)
		return
	}
	fmt.Println(indent(string(enc1)))
	fmt.Printf("  value stable (v1 == v2):   %v\n", reflect.DeepEqual(v1, v2))
	fmt.Printf("  bytes stable (Marshal x2): %v\n", bytes.Equal(enc1, enc2))
	m := v1.(map[string]any)
	fmt.Printf("  YAML 1.2 core schema: notbool=%#v (string, not bool)\n", m["notbool"])
	fmt.Printf("  literal block preserved: %q\n", m["literal"])
	fmt.Printf("  folded block:            %q\n", m["folded"])
	fmt.Printf("  single-quote escape:     %q\n", m["sq"])
	fmt.Printf("  empty value:             %#v\n", m["empty"])

	section("5. Encoder.SetIndent")
	for _, n := range []int{2, 6} {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(n)
		if err := enc.Encode(map[string]any{"outer": map[string]any{"inner": []any{1, 2}}}); err != nil {
			fmt.Println("encode error:", err)
			continue
		}
		if err := enc.Close(); err != nil {
			fmt.Println("close error:", err)
			continue
		}
		fmt.Printf("  indent=%d:\n%s\n", n, indent(buf.String()))
	}
}

func anchorsAliases() {
	section("6. Anchors, aliases and merge keys")
	src := []byte(`
defaults: &defaults
  adapter: postgres
  pool: 5
  host: localhost

development:
  <<: *defaults
  database: dev_db

production:
  <<: *defaults
  host: db.prod.internal
  database: prod_db

shared_list: &list [a, b, c]
copy_of_list: *list
scalar_anchor: &greeting hello
scalar_alias: *greeting
`)
	var v map[string]any
	if err := yaml.Unmarshal(src, &v); err != nil {
		fmt.Println("unmarshal error:", err)
		return
	}
	fmt.Printf("  development:  %#v\n", v["development"])
	fmt.Printf("  production:   %#v\n", v["production"])
	fmt.Printf("  merge override kept: production.host=%v\n", v["production"].(map[string]any)["host"])
	fmt.Printf("  copy_of_list: %#v\n", v["copy_of_list"])
	fmt.Printf("  scalar_alias: %#v\n", v["scalar_alias"])

	// Anchors survive into the Node tree; aliases are resolvable via Node.Alias.
	docs, err := yaml.Parse(src)
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	root := docs[0].Content[0]
	var anchors, aliases []string
	walk(root, func(n *yaml.Node) {
		if n.Anchor != "" {
			anchors = append(anchors, n.Anchor)
		}
		if n.Kind == yaml.AliasNode {
			target := "<nil>"
			if n.Alias != nil {
				target = n.Alias.Kind.String()
			}
			aliases = append(aliases, fmt.Sprintf("*%s->%s", n.Value, target))
		}
	})
	fmt.Printf("  anchors in tree: %v\n", anchors)
	fmt.Printf("  aliases in tree: %v\n", aliases)

	// Alias bomb guard.
	bomb := "a: &a [x,x,x,x,x,x,x,x,x]\n"
	for i := 1; i <= 12; i++ {
		prev := string(rune('a' + i - 1))
		cur := string(rune('a' + i))
		bomb += fmt.Sprintf("%s: &%s [*%s,*%s,*%s,*%s,*%s,*%s,*%s,*%s,*%s]\n",
			cur, cur, prev, prev, prev, prev, prev, prev, prev, prev, prev)
	}
	var sink any
	err = yaml.Unmarshal([]byte(bomb), &sink)
	fmt.Printf("  billion-laughs rejected: Is(ErrAliasBomb)=%v err=%v\n",
		errors.Is(err, yaml.ErrAliasBomb), err)
}

func walk(n *yaml.Node, fn func(*yaml.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for _, c := range n.Content {
		walk(c, fn)
	}
}

func multiDocument() {
	section("7. Multi-document input via Decoder")
	stream := `
%YAML 1.2
---
kind: Service
name: frontend
ports: [80, 443]
---
kind: Deployment
name: frontend
replicas: 3
...
---
kind: ConfigMap
name: frontend-env
data:
  LOG_LEVEL: debug
`
	type doc struct {
		Kind     string            `yaml:"kind"`
		Name     string            `yaml:"name"`
		Ports    []int             `yaml:"ports,omitempty"`
		Replicas int               `yaml:"replicas,omitempty"`
		Data     map[string]string `yaml:"data,omitempty"`
	}
	dec := yaml.NewDecoder(strings.NewReader(stream))
	for i := 0; ; i++ {
		var d doc
		err := dec.Decode(&d)
		if errors.Is(err, io.EOF) {
			fmt.Printf("  io.EOF after %d documents\n", i)
			break
		}
		if err != nil {
			fmt.Printf("  decode error at doc %d: %v\n", i, err)
			break
		}
		fmt.Printf("  doc %d: kind=%-11s name=%-14s ports=%v replicas=%d data=%v\n",
			i, d.Kind, d.Name, d.Ports, d.Replicas, d.Data)
	}

	// Parse gives one DocumentNode per document.
	docs, err := yaml.Parse([]byte(stream))
	if err != nil {
		fmt.Println("  parse error:", err)
		return
	}
	fmt.Printf("  Parse returned %d document nodes; kinds:", len(docs))
	for _, d := range docs {
		fmt.Printf(" %s/%s", d.Kind, d.Content[0].Kind)
	}
	fmt.Println()

	// Unmarshal only sees the first document, by design.
	var first map[string]any
	if err := yaml.Unmarshal([]byte(stream), &first); err != nil {
		fmt.Println("  unmarshal error:", err)
		return
	}
	fmt.Printf("  Unmarshal took only doc 0: kind=%v\n", first["kind"])
}

func streamEncode() {
	section("8. Multi-document output via Encoder")
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	for i, name := range []string{"alpha", "beta", "gamma"} {
		if err := enc.Encode(map[string]any{"idx": i, "name": name}); err != nil {
			fmt.Println("encode error:", err)
			return
		}
	}
	if err := enc.Close(); err != nil {
		fmt.Println("close error:", err)
		return
	}
	fmt.Println(indent(buf.String()))

	// Encoding after Close reports ErrClosed.
	err := enc.Encode(map[string]any{"late": true})
	fmt.Printf("  Encode after Close: Is(ErrClosed)=%v err=%v\n", errors.Is(err, yaml.ErrClosed), err)

	// Read the stream back to prove it is three documents.
	dec := yaml.NewDecoder(bytes.NewReader(buf.Bytes()))
	n := 0
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			break
		}
		n++
	}
	fmt.Printf("  re-decoded %d documents\n", n)

	// Encoder also writes to os.Stdout directly.
	fmt.Println("  direct to stdout:")
	e2 := yaml.NewEncoder(os.Stdout)
	e2.SetIndent(2)
	_ = e2.Encode(map[string]any{"stdout": true, "list": []int{1, 2}})
	_ = e2.Close()
}

func nodeAPI() {
	section("9. Node API: inspect, rewrite, comments")
	src := []byte(`# top of file
# second head line
name: widget   # the widget's name
version: 2
tags: [a, b]
# trailing block
`)
	docs, err := yaml.Parse(src)
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	doc := docs[0]
	root := doc.Content[0]
	fmt.Printf("  document head comment: %q\n", doc.HeadComment)
	fmt.Printf("  root: kind=%s tag=%s short=%s long=%s line=%d col=%d\n",
		root.Kind, root.Tag, root.ShortTag(), root.LongTag(), root.Line, root.Column)
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		fmt.Printf("  key %-8q -> kind=%-8s tag=%-6s value=%-8q line=%d head=%q line-comment=%q\n",
			k.Value, v.Kind, v.Tag, v.Value, v.Line, k.HeadComment, k.LineComment)
	}
	fmt.Printf("  zero Node IsZero: %v\n", (&yaml.Node{}).IsZero())

	// Rewrite in place: bump version, force a style, add a comment, then emit.
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "version":
			root.Content[i+1].Value = "3"
		case "name":
			root.Content[i+1].Style = yaml.DoubleQuotedStyle
			// HOLE: LineComment is written out verbatim, without the "# "
			// prefix that writeComment adds to Head/FootComment. Setting it to
			// "rewritten" emits `name: "widget" rewritten`, which re-parses as
			// the string `widget" rewritten` rather than a comment, so the
			// value is silently corrupted. Head/Foot comments do get prefixed.
			// Passing the "#" ourselves is the only safe form.
			root.Content[i+1].LineComment = "# rewritten"
		case "tags":
			root.Content[i+1].Style = 0 // flow -> block
		}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		fmt.Println("re-emit error:", err)
	} else {
		fmt.Println("  re-emitted after edits:")
		fmt.Println(indent(string(out)))
		// HOLE: the trailing comment block is emitted twice (once as the
		// mapping's FootComment and once as the document's), and it doubles
		// again on every further round trip.
		fmt.Printf("  foot comment emitted %d times (source had 1)\n",
			strings.Count(string(out), "# trailing block"))
	}

	// Node.Decode on a subtree, Node.Encode to build one.
	var tags []string
	if err := root.Content[5].Decode(&tags); err != nil {
		fmt.Println("subtree decode error:", err)
	} else {
		fmt.Printf("  Node.Decode(subtree) -> %#v\n", tags)
	}
	built := &yaml.Node{}
	if err := built.Encode(map[string]any{"built": []any{"by", "Node.Encode"}}); err != nil {
		fmt.Println("Node.Encode error:", err)
	} else {
		b, _ := yaml.Marshal(built)
		fmt.Printf("  Node.Encode kind=%s ->\n%s\n", built.Kind, indent(string(b)))
	}

	// SetString sets value and picks a safe style.
	sn := &yaml.Node{}
	sn.SetString("needs: quoting")
	b, _ := yaml.Marshal(sn)
	fmt.Printf("  SetString(%q) tag=%s -> %s", "needs: quoting", sn.Tag, string(b))
}

func customTags() {
	section("10. Explicit and custom tags")
	src := []byte(`
explicit_str: !!str 123
explicit_int: !!int "42"
explicit_float: !!float 7
explicit_bool: !!bool "true"
explicit_null: !!null ""
binary: !!binary QUJD
stamp: !!timestamp 2026-08-10T12:00:00Z
local: !point {x: 1, y: 2}
verbatim: !<tag:example.com,2000:thing> value
`)
	var v map[string]any
	if err := yaml.Unmarshal(src, &v); err != nil {
		fmt.Println("unmarshal error:", err)
	}
	for _, k := range []string{"explicit_str", "explicit_int", "explicit_float", "explicit_bool", "explicit_null", "binary", "stamp"} {
		fmt.Printf("  %-14s %#v\n", k, v[k])
	}
	fmt.Printf("  %-14s %#v\n", "local", v["local"])

	docs, err := yaml.Parse(src)
	if err != nil {
		fmt.Println("parse error:", err)
		return
	}
	root := docs[0].Content[0]
	fmt.Println("  tags as seen on the Node tree:")
	for i := 0; i+1 < len(root.Content); i += 2 {
		fmt.Printf("    %-14s tag=%s\n", root.Content[i].Value, root.Content[i+1].Tag)
	}

	// %TAG directive handles are expanded into resolved URIs.
	withDirective := []byte("%TAG !e! tag:example.com,2000:\n---\nthing: !e!widget 1\n")
	d2, err := yaml.Parse(withDirective)
	if err != nil {
		fmt.Printf("  %%TAG parse error: %v\n", err)
	} else {
		fmt.Printf("  %%TAG handle expanded to: %s\n", d2[0].Content[0].Content[1].Tag)
		out, err := yaml.Marshal(d2[0])
		fmt.Printf("  re-emitted (err=%v): %s", err, strings.TrimRight(string(out), "\n"))
		fmt.Println()
	}

	// Round-tripping the whole tagged document. Two emitter bugs show up here.
	out, err := yaml.Marshal(docs[0])
	fmt.Printf("  full tagged doc re-emit err=%v:\n%s\n", err, indent(string(out)))
	_, reErr := yaml.Parse(out)
	fmt.Printf("  HOLE: re-parsing that output fails: %v\n", reErr)

	// HOLE: a tag on a *flow* collection is emitted twice ("!lst !lst [1, 2]"),
	// which is a "repeated tag" syntax error on the way back in. Block-style
	// tagged collections and tagged scalars are emitted correctly.
	d3, _ := yaml.Parse([]byte("q: !lst [1, 2]\n"))
	o3, _ := yaml.Marshal(d3[0])
	_, e3 := yaml.Parse(o3)
	fmt.Printf("  flow tag doubled: %q -> re-parse err=%v\n", strings.TrimRight(string(o3), "\n"), e3)

	// HOLE: a global/resolved URI tag is emitted as a bare URI instead of the
	// verbatim "!<...>" form, so it silently becomes part of the scalar. This
	// one does not even error: the data is just wrong.
	d4, _ := yaml.Parse([]byte("v: !<tag:example.com,2000:thing> value\n"))
	o4, _ := yaml.Marshal(d4[0])
	d5, e5 := yaml.Parse(o4)
	fmt.Printf("  URI tag emitted bare: %q re-parse err=%v\n", strings.TrimRight(string(o4), "\n"), e5)
	if e5 == nil {
		v := d5[0].Content[0].Content[1]
		fmt.Printf("    value became kind=%s tag=%q value=%q (tag lost, scalar corrupted)\n",
			v.Kind, v.Tag, v.Value)
	}

	// HOLE: !!timestamp is dropped on output, so an `any` round trip degrades
	// time.Time to string (a plain timestamp is !!str under the core schema).
	var t1 any
	_ = yaml.Unmarshal([]byte("t: !!timestamp 2026-08-10T12:00:00Z\n"), &t1)
	o6, _ := yaml.Marshal(t1)
	var t2 any
	_ = yaml.Unmarshal(o6, &t2)
	fmt.Printf("  timestamp round trip: %T -> %q -> %T\n",
		t1.(map[string]any)["t"], strings.TrimRight(string(o6), "\n"), t2.(map[string]any)["t"])

	// Decoding a !!binary into []byte.
	var bs struct {
		Binary []byte `yaml:"binary"`
	}
	if err := yaml.Unmarshal(src, &bs); err != nil {
		fmt.Println("  binary decode error:", err)
	} else {
		fmt.Printf("  !!binary into []byte: %q\n", bs.Binary)
	}
}

func customMarshalers() {
	section("11. Custom Marshaler / Unmarshaler")
	r := Release{Version: Semver{1, 4, 12}, Temp: Celsius(21.5)}
	out, err := yaml.Marshal(r)
	if err != nil {
		fmt.Println("marshal error:", err)
		return
	}
	fmt.Println(indent(string(out)))

	var back Release
	if err := yaml.Unmarshal(out, &back); err != nil {
		fmt.Println("unmarshal error:", err)
	} else {
		fmt.Printf("  decoded: version=%d.%d.%d temp=%v\n",
			back.Version.Major, back.Version.Minor, back.Version.Patch, float64(back.Temp))
		fmt.Printf("  round trip equal: %v\n", back == r)
	}

	// An Unmarshaler that rejects its input propagates the error.
	err = yaml.Unmarshal([]byte("version: {not: scalar}\ntemp: 1.0\n"), &Release{})
	fmt.Printf("  Unmarshaler rejection surfaced: %v\n", err)

	// encoding.TextMarshaler/TextUnmarshaler is honoured too.
	type withIP struct {
		Addr *time.Time `yaml:"addr,omitempty"`
	}
	_ = withIP{}
	var dur struct {
		D time.Duration `yaml:"d"`
	}
	if err := yaml.Unmarshal([]byte("d: 2m30s\n"), &dur); err != nil {
		fmt.Println("  duration decode error:", err)
	} else {
		fmt.Printf("  time.Duration from \"2m30s\": %s\n", dur.D)
	}
}

func errorHandling() {
	section("12. Error handling")

	bad := []struct {
		what string
		src  string
	}{
		{"tab used for indentation", "a:\n\tb: 1\n"},
		{"unclosed flow sequence", "a: [1, 2\n"},
		{"unclosed double quote", "a: \"oops\n"},
		{"duplicate-ish bad mapping", "a: 1\n b: 2\n"},
		{"undefined alias", "a: *nope\n"},
		{"bad %YAML directive", "%YAML 2.0\n---\na: 1\n"},
		{"block scalar bad indicator", "a: |9x\n  b\n"},
	}
	for _, tc := range bad {
		err := yaml.Unmarshal([]byte(tc.src), new(any))
		var se *yaml.SyntaxError
		asSyntax := errors.As(err, &se)
		pos := ""
		if asSyntax {
			pos = fmt.Sprintf(" line=%d col=%d", se.Line, se.Column)
		}
		fmt.Printf("  %-26s err=%v\n    Is(ErrSyntax)=%v as *SyntaxError=%v%s\n",
			tc.what, err, errors.Is(err, yaml.ErrSyntax), asSyntax, pos)
	}

	// HOLE: ill-formed UTF-8 is accepted without complaint (libyaml, PyYAML and
	// js-yaml all reject it, since a YAML stream is Unicode text). The invalid
	// bytes survive into the Go string and are written back out inside a
	// single-quoted scalar, so Marshal produces output that is not valid YAML
	// text either.
	badUTF8 := []byte("a: \xff\xfe\n")
	var uv any
	uerr := yaml.Unmarshal(badUTF8, &uv)
	uout, _ := yaml.Marshal(uv)
	fmt.Printf("  ill-formed UTF-8: unmarshal err=%v value=%q re-emitted=%q\n",
		uerr, uv.(map[string]any)["a"], string(uout))

	// Unknown anchor sentinel.
	err := yaml.Unmarshal([]byte("a: *nope\n"), new(any))
	fmt.Printf("  Is(ErrUnknownAnchor) for undefined alias: %v\n", errors.Is(err, yaml.ErrUnknownAnchor))

	// Type errors are accumulated, and the rest of the document still decodes.
	var target struct {
		N    int    `yaml:"n"`
		M    int    `yaml:"m"`
		OK   string `yaml:"ok"`
		List []int  `yaml:"list"`
	}
	err = yaml.Unmarshal([]byte("n: not-a-number\nm: also-bad\nok: fine\nlist: [1, two, 3]\n"), &target)
	var te *yaml.TypeError
	fmt.Printf("  type mismatches: as *TypeError=%v Is(ErrTypeMismatch)=%v\n",
		errors.As(err, &te), errors.Is(err, yaml.ErrTypeMismatch))
	if te != nil {
		for _, e := range te.Errors {
			fmt.Printf("    - %s\n", e)
		}
	}
	fmt.Printf("  partial decode still stored: OK=%q N=%d List=%v\n", target.OK, target.N, target.List)

	// Non-pointer / nil pointer destination.
	fmt.Printf("  Unmarshal(non-pointer): Is(ErrInvalidUnmarshal)=%v\n",
		errors.Is(yaml.Unmarshal([]byte("a: 1"), struct{}{}), yaml.ErrInvalidUnmarshal))
	var nilp *map[string]any
	fmt.Printf("  Unmarshal(nil *T):      Is(ErrInvalidUnmarshal)=%v\n",
		errors.Is(yaml.Unmarshal([]byte("a: 1"), nilp), yaml.ErrInvalidUnmarshal))

	// Unsupported Go types.
	_, err = yaml.Marshal(map[string]any{"ch": make(chan int)})
	fmt.Printf("  Marshal(chan): Is(ErrUnsupportedType)=%v err=%v\n",
		errors.Is(err, yaml.ErrUnsupportedType), err)
	_, err = yaml.Marshal(func() {})
	fmt.Printf("  Marshal(func): Is(ErrUnsupportedType)=%v err=%v\n",
		errors.Is(err, yaml.ErrUnsupportedType), err)

	// Cyclic Go value: bounded, not a stack overflow.
	type cyc struct{ Next *cyc }
	c := &cyc{}
	c.Next = c
	_, err = yaml.Marshal(c)
	fmt.Printf("  Marshal(cyclic struct): err=%v\n", err)

	// Deeply nested text hits the parser's node-depth limit as a SyntaxError.
	deep := strings.Repeat("[", 2000) + strings.Repeat("]", 2000)
	err = yaml.Unmarshal([]byte(deep), new(any))
	fmt.Printf("  2000-deep flow sequence: Is(ErrSyntax)=%v err=%v\n", errors.Is(err, yaml.ErrSyntax), err)

	// Empty input is not an error; the destination is left alone.
	v := "untouched"
	fmt.Printf("  Unmarshal(empty): err=%v value=%q\n", yaml.Unmarshal(nil, &v), v)

	// Decoder over a failing reader.
	dec := yaml.NewDecoder(iotest{})
	fmt.Printf("  Decoder over failing reader: %v\n", dec.Decode(new(any)))
}

// iotest is an io.Reader that always fails, to show Decoder surfaces read errors.
type iotest struct{}

func (iotest) Read([]byte) (int, error) { return 0, errors.New("boom: reader failed") }
