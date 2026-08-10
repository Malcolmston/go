package parity

// Canonicalisation of a generated MIME message.
//
// Raw-string comparison of two MIME encoders is hopeless: header order, fold
// points, encoded-word split points and random multipart boundaries all differ
// without any semantic difference. So the harness parses each message into a
// canonical tree and compares that.
//
// The same parser runs over both implementations' output, so the comparison
// never inherits a divergence between two different MIME parsers. The parser is
// deliberately tolerant: instead of failing on invalid MIME it records what is
// wrong in the node's Malformed list, which is itself compared. That is how the
// port's bare-encoded-word parameter defect shows up as a difference rather
// than as a crash.

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Node is one MIME entity: its headers, its content type and either a decoded
// body or child parts.
type Node struct {
	Headers          []Header `json:"headers"`
	ContentType      string   `json:"contentType"`
	TransferEncoding string   `json:"transferEncoding"`
	BodyKind         string   `json:"bodyKind"` // "text", "base64" or "parts"
	Body             string   `json:"body,omitempty"`
	Parts            []*Node  `json:"parts,omitempty"`
	Malformed        []string `json:"malformed,omitempty"`
}

// Header is a canonicalised header field: the name in canonical MIME case and
// the value normalised according to the field's grammar.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Canonicalise turns a raw generated message into a comparable tree.
func Canonicalise(raw string) *Node {
	normalised, _ := normaliseBoundaries(raw)
	return parseEntity(normalised, 0)
}

// ---------------------------------------------------------------------------
// boundary pinning

var boundaryParamRe = regexp.MustCompile(`(?i)boundary\s*=\s*("[^"]*"|[^;\r\n]+)`)

// normaliseBoundaries replaces every multipart boundary token with a stable
// index-based placeholder (B0, B1, ...) assigned in order of first appearance.
// Both encoders pick their boundaries at random (or from a pinned base with
// suffixes), so without this step every multipart message would differ.
func normaliseBoundaries(raw string) (string, []string) {
	var order []string
	seen := map[string]bool{}
	for _, m := range boundaryParamRe.FindAllStringSubmatch(raw, -1) {
		tok := strings.TrimSpace(m[1])
		tok = strings.Trim(tok, `"`)
		// Boundary params can be folded; the fold has not been removed yet.
		tok = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(tok, "\r", ""), "\n", ""))
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		order = append(order, tok)
	}
	// Replace longest tokens first so that a token which is a prefix of another
	// (e.g. "BASE" and "BASE_2") cannot corrupt it.
	byLen := make([]string, len(order))
	copy(byLen, order)
	sort.SliceStable(byLen, func(i, j int) bool { return len(byLen[i]) > len(byLen[j]) })
	names := make(map[string]string, len(order))
	for i, tok := range order {
		names[tok] = fmt.Sprintf("B%d", i)
	}
	out := raw
	for _, tok := range byLen {
		out = strings.ReplaceAll(out, tok, names[tok])
	}
	placeholders := make([]string, len(order))
	for i := range order {
		placeholders[i] = names[order[i]]
	}
	return out, placeholders
}

// ---------------------------------------------------------------------------
// entity parsing

const maxDepth = 8

func parseEntity(raw string, depth int) *Node {
	n := &Node{}
	headerBlock, body := splitHeaders(raw)
	fields := unfold(headerBlock)

	rawByName := map[string][]string{}
	for _, f := range fields {
		name, value, ok := splitField(f)
		if !ok {
			n.Malformed = append(n.Malformed, "unparseable header line: "+truncate(f))
			continue
		}
		canon := textproto.CanonicalMIMEHeaderKey(name)
		rawByName[canon] = append(rawByName[canon], value)
		norm, bad := normaliseHeaderValue(canon, value)
		if bad != "" {
			n.Malformed = append(n.Malformed, bad)
		}
		n.Headers = append(n.Headers, Header{Name: canon, Value: norm})
	}
	sort.SliceStable(n.Headers, func(i, j int) bool {
		if n.Headers[i].Name != n.Headers[j].Name {
			return n.Headers[i].Name < n.Headers[j].Name
		}
		return n.Headers[i].Value < n.Headers[j].Value
	})

	ctRaw := last(rawByName["Content-Type"])
	if ctRaw == "" {
		ctRaw = "text/plain; charset=us-ascii"
	}
	mt := parseMediaType(ctRaw)
	n.ContentType = mt.canonical()

	n.TransferEncoding = strings.ToLower(strings.TrimSpace(last(rawByName["Content-Transfer-Encoding"])))
	if n.TransferEncoding == "" {
		n.TransferEncoding = "7bit"
	}

	if strings.HasPrefix(mt.Type, "multipart/") && depth < maxDepth {
		boundary := mt.Params["boundary"]
		if boundary == "" {
			n.Malformed = append(n.Malformed, "multipart entity without a boundary parameter")
			n.BodyKind = "text"
			n.Body = normaliseText(body)
			return n
		}
		parts, ok := splitMultipart(body, boundary)
		if !ok {
			n.Malformed = append(n.Malformed, "multipart boundary --"+boundary+" not found in body")
			n.BodyKind = "text"
			n.Body = normaliseText(body)
			return n
		}
		n.BodyKind = "parts"
		for _, p := range parts {
			n.Parts = append(n.Parts, parseEntity(p, depth+1))
		}
		return n
	}

	decoded, err := decodeBody(body, n.TransferEncoding)
	if err != nil {
		n.Malformed = append(n.Malformed, "body decode failed: "+err.Error())
	}
	if utf8.Valid(decoded) && !hasControl(decoded) {
		n.BodyKind = "text"
		n.Body = normaliseText(string(decoded))
	} else {
		n.BodyKind = "base64"
		n.Body = base64.StdEncoding.EncodeToString(decoded)
	}
	return n
}

func hasControl(b []byte) bool {
	for _, c := range b {
		if c < 0x09 || (c > 0x0d && c < 0x20) || c == 0x7f {
			return true
		}
	}
	return false
}

// splitHeaders splits an entity at the first empty line.
func splitHeaders(raw string) (headers, body string) {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return s[:i], s[i+2:]
	}
	return s, ""
}

// unfold joins continuation lines (RFC 5322 §2.2.3) into single logical fields.
func unfold(block string) []string {
	var fields []string
	for _, line := range strings.Split(block, "\n") {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(fields) > 0 {
			fields[len(fields)-1] += " " + strings.TrimSpace(line)
			continue
		}
		fields = append(fields, strings.TrimRight(line, " \t"))
	}
	return fields
}

func splitField(f string) (name, value string, ok bool) {
	i := strings.Index(f, ":")
	if i <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(f[:i]), strings.TrimSpace(f[i+1:]), true
}

func last(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}

// splitMultipart cuts a multipart body on its boundary delimiters.
func splitMultipart(body, boundary string) ([]string, bool) {
	delim := "--" + boundary
	lines := strings.Split(body, "\n")
	var parts []string
	var cur []string
	started, closed := false, false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == delim || trimmed == delim+"--" {
			if started {
				parts = append(parts, strings.Join(cur, "\n"))
				cur = nil
			}
			started = true
			if trimmed == delim+"--" {
				closed = true
				break
			}
			continue
		}
		if started {
			cur = append(cur, line)
		}
	}
	if !started {
		return nil, false
	}
	if !closed {
		// An unterminated multipart is still parseable; the caller records it.
		if len(cur) > 0 {
			parts = append(parts, strings.Join(cur, "\n"))
		}
	}
	return parts, true
}

func decodeBody(body, cte string) ([]byte, error) {
	switch cte {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, body)
		return base64.StdEncoding.WithPadding(base64.StdPadding).DecodeString(clean)
	case "quoted-printable":
		crlf := strings.ReplaceAll(body, "\n", "\r\n")
		b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(crlf)))
		return b, err
	default:
		return []byte(body), nil
	}
}

// normaliseText makes body text comparable: LF line endings and no trailing
// blank lines. The two encoders disagree about the final CRLF of a body and
// about whether a lone LF in the input is rewritten to CRLF.
func normaliseText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, "\n")
}

// ---------------------------------------------------------------------------
// header value normalisation

var addressHeaders = map[string]bool{
	"From": true, "To": true, "Cc": true, "Bcc": true,
	"Reply-To": true, "Sender": true, "Disposition-Notification-To": true,
}

var msgIDHeaders = map[string]bool{
	"Message-Id": true, "In-Reply-To": true, "References": true,
}

func normaliseHeaderValue(name, value string) (norm string, malformed string) {
	switch {
	case addressHeaders[name]:
		return canonicalAddressList(value), ""
	case name == "Content-Type" || name == "Content-Disposition":
		mt := parseMediaType(value)
		if len(mt.Bad) > 0 {
			return mt.canonical(), name + ": " + strings.Join(mt.Bad, "; ")
		}
		return mt.canonical(), ""
	case msgIDHeaders[name]:
		return strings.Join(strings.Fields(value), " "), ""
	case name == "Date":
		return strings.Join(strings.Fields(value), " "), ""
	default:
		return collapseWS(decodeWords(value)), ""
	}
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// RFC 2047 encoded words

var wordDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, r io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "utf-8", "utf8", "us-ascii", "ascii":
			return r, nil
		case "iso-8859-1", "latin1", "windows-1252":
			b, err := io.ReadAll(r)
			if err != nil {
				return nil, err
			}
			var sb strings.Builder
			for _, c := range b {
				sb.WriteRune(rune(c))
			}
			return strings.NewReader(sb.String()), nil
		}
		return nil, fmt.Errorf("unsupported charset %q", charset)
	},
}

var encodedWordRe = regexp.MustCompile(`=\?[^?]+\?[bBqQ]\?[^?]*\?=`)

// decodeWords decodes every RFC 2047 encoded word in s, dropping the linear
// whitespace that separates two adjacent encoded words (RFC 2047 §6.2). That is
// what makes the two encoders' different encoded-word split points comparable.
func decodeWords(s string) string {
	locs := encodedWordRe.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var out strings.Builder
	prevEnd := 0
	prevWasWord := false
	for _, loc := range locs {
		gap := s[prevEnd:loc[0]]
		if !(prevWasWord && strings.TrimSpace(gap) == "") {
			out.WriteString(gap)
		}
		word := s[loc[0]:loc[1]]
		if dec, err := wordDecoder.Decode(word); err == nil {
			out.WriteString(dec)
		} else {
			out.WriteString(word)
		}
		prevEnd = loc[1]
		prevWasWord = true
	}
	out.WriteString(s[prevEnd:])
	return out.String()
}

// ---------------------------------------------------------------------------
// media types and parameters

type mediaType struct {
	Type   string
	Params map[string]string
	Bad    []string
}

func (m mediaType) canonical() string {
	keys := make([]string, 0, len(m.Params))
	for k := range m.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(m.Type)
	for _, k := range keys {
		fmt.Fprintf(&sb, "; %s=%q", k, m.Params[k])
	}
	return sb.String()
}

// tokenSpecials are the RFC 2045 tspecials plus SP and CTL: a parameter value
// containing any of them MUST be a quoted string.
const tokenSpecials = "()<>@,;:\\\"/[]?= \t"

// parseMediaType is a deliberately tolerant replacement for mime.ParseMediaType.
// It never fails; instead it records each violation in Bad so that malformed
// output is compared rather than swallowed.
func parseMediaType(v string) mediaType {
	m := mediaType{Params: map[string]string{}}
	segs := splitSemis(v)
	if len(segs) == 0 {
		return m
	}
	m.Type = strings.ToLower(strings.TrimSpace(segs[0]))

	// Raw params first, then RFC 2231 continuation/charset assembly.
	type rawParam struct {
		key   string
		value string
	}
	var raws []rawParam
	for _, seg := range segs[1:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		i := strings.Index(seg, "=")
		if i < 0 {
			m.Bad = append(m.Bad, "parameter without a value: "+truncate(seg))
			continue
		}
		key := strings.ToLower(strings.TrimSpace(seg[:i]))
		val := strings.TrimSpace(seg[i+1:])
		if strings.HasPrefix(val, `"`) {
			val = unquote(val)
		} else if strings.ContainsAny(val, tokenSpecials) && !strings.HasSuffix(key, "*") {
			// The value is a bare token containing tspecials: not legal MIME.
			m.Bad = append(m.Bad, fmt.Sprintf("unquoted parameter %s contains tspecials: %s", key, truncate(val)))
		}
		raws = append(raws, rawParam{key, val})
	}

	// RFC 2231: name*0*=utf-8''%C3%BC / name*1=... / name*=utf-8''%C3%BC
	continued := map[string]map[int]string{}
	extended := map[string]bool{}
	for _, rp := range raws {
		key, idx, isExt := parse2231Key(rp.key)
		if idx < 0 {
			// Not a continuation.
			if isExt {
				m.Params[key] = decode2231Value(rp.value)
				extended[key] = true
			} else {
				if _, ok := m.Params[key]; !ok || !extended[key] {
					m.Params[key] = decodeWords(rp.value)
				}
			}
			continue
		}
		if continued[key] == nil {
			continued[key] = map[int]string{}
		}
		if isExt {
			continued[key][idx] = "*" + rp.value
		} else {
			continued[key][idx] = rp.value
		}
	}
	for key, segsByIdx := range continued {
		idxs := make([]int, 0, len(segsByIdx))
		for i := range segsByIdx {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		var sb strings.Builder
		var isExt bool
		for _, i := range idxs {
			s := segsByIdx[i]
			if strings.HasPrefix(s, "*") {
				isExt = true
				s = s[1:]
			}
			sb.WriteString(s)
		}
		if isExt {
			m.Params[key] = decode2231Value(sb.String())
		} else {
			m.Params[key] = decodeWords(sb.String())
		}
	}
	return m
}

// parse2231Key splits "name*3*" into ("name", 3, true).
func parse2231Key(key string) (name string, idx int, extended bool) {
	idx = -1
	if strings.HasSuffix(key, "*") {
		extended = true
		key = strings.TrimSuffix(key, "*")
	}
	if i := strings.LastIndex(key, "*"); i >= 0 {
		numPart := key[i+1:]
		n := 0
		ok := numPart != ""
		for _, r := range numPart {
			if r < '0' || r > '9' {
				ok = false
				break
			}
			n = n*10 + int(r-'0')
		}
		if ok {
			return key[:i], n, extended
		}
	}
	return key, -1, extended
}

// decode2231Value decodes charset'lang'percent-encoded values.
func decode2231Value(v string) string {
	charset := ""
	rest := v
	if i := strings.Index(rest, "'"); i >= 0 {
		charset = strings.ToLower(rest[:i])
		rest = rest[i+1:]
		if j := strings.Index(rest, "'"); j >= 0 {
			rest = rest[j+1:]
		}
	}
	dec, err := url.PathUnescape(rest)
	if err != nil {
		dec = rest
	}
	if charset == "iso-8859-1" {
		var sb strings.Builder
		for i := 0; i < len(dec); i++ {
			sb.WriteRune(rune(dec[i]))
		}
		dec = sb.String()
	}
	return dec
}

func unquote(s string) string {
	if len(s) < 2 || s[0] != '"' {
		return s
	}
	s = s[1:]
	var sb strings.Builder
	esc := false
	for _, r := range s {
		if esc {
			sb.WriteRune(r)
			esc = false
			continue
		}
		switch r {
		case '\\':
			esc = true
		case '"':
			return sb.String()
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// splitSemis splits on ';' outside quoted strings.
func splitSemis(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, esc := false, false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && inQuote:
			cur.WriteRune(r)
			esc = true
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ';' && !inQuote:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

// ---------------------------------------------------------------------------
// address lists

// canonicalAddressList renders an address header in a single canonical form:
// display names decoded and unquoted, domains lower-cased, groups written as
// Name:[members]. It is tolerant of malformed input.
func canonicalAddressList(v string) string {
	items := splitAddrItems(v)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, canonicalAddrItem(it))
	}
	return strings.Join(out, ", ")
}

// splitAddrItems splits an address header into top-level items, keeping a group
// ("Name: a, b;") together as one item.
func splitAddrItems(v string) []string {
	var out []string
	var cur strings.Builder
	inQuote, inAngle, inGroup := false, false, false
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, strings.TrimSpace(cur.String()))
		}
		cur.Reset()
	}
	for _, r := range v {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == '<' && !inQuote:
			inAngle = true
			cur.WriteRune(r)
		case r == '>' && !inQuote:
			inAngle = false
			cur.WriteRune(r)
		case r == ':' && !inQuote && !inAngle:
			inGroup = true
			cur.WriteRune(r)
		case r == ';' && !inQuote && inGroup:
			cur.WriteRune(r)
			inGroup = false
			flush()
		case r == ',' && !inQuote && !inAngle && !inGroup:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func canonicalAddrItem(it string) string {
	if i := strings.Index(it, ":"); i >= 0 && strings.HasSuffix(strings.TrimSpace(it), ";") {
		// Only a group if the colon precedes the '@' (if any).
		at := strings.Index(it, "@")
		if at < 0 || at > i {
			name := collapseWS(decodeWords(unquoteIfQuoted(strings.TrimSpace(it[:i]))))
			body := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(it[i+1:]), ";"))
			var members []string
			if body != "" {
				for _, m := range splitAddrItems(body) {
					members = append(members, canonicalMailbox(m))
				}
			}
			return name + ":[" + strings.Join(members, ", ") + "]"
		}
	}
	return canonicalMailbox(it)
}

func canonicalMailbox(it string) string {
	it = strings.TrimSpace(it)
	name, addr := "", it
	if i := strings.LastIndex(it, "<"); i >= 0 {
		if j := strings.Index(it[i:], ">"); j >= 0 {
			name = strings.TrimSpace(it[:i])
			addr = strings.TrimSpace(it[i+1 : i+j])
		}
	}
	name = collapseWS(decodeWords(unquoteIfQuoted(name)))
	addr = normaliseAddrSpec(addr)
	if name == "" {
		return "<" + addr + ">"
	}
	return name + " <" + addr + ">"
}

func unquoteIfQuoted(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		return unquote(s)
	}
	return s
}

func normaliseAddrSpec(a string) string {
	a = strings.TrimSpace(a)
	if i := strings.LastIndex(a, "@"); i >= 0 {
		return a[:i] + "@" + strings.ToLower(a[i+1:])
	}
	return a
}

// ---------------------------------------------------------------------------
// comparison helpers

// StripSystematic returns a copy of the tree with the two systematic,
// message-wide divergences of the current port masked out:
//
//   - Content-Transfer-Encoding, as a node field and as a header. The port
//     always quoted-printable-encodes text bodies where upstream chooses
//     7bit / quoted-printable / base64 per body.
//   - headers whose value is empty. The port always emits a Subject header,
//     even when there is no subject.
//
// It powers the secondary "structural" metric, which answers "how close is the
// MIME tree once those two known defects are set aside".
func StripSystematic(n *Node) *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.TransferEncoding = ""
	c.Headers = nil
	for _, h := range n.Headers {
		if h.Name == "Content-Transfer-Encoding" || h.Value == "" {
			continue
		}
		c.Headers = append(c.Headers, h)
	}
	c.Parts = nil
	for _, p := range n.Parts {
		c.Parts = append(c.Parts, StripSystematic(p))
	}
	return &c
}
