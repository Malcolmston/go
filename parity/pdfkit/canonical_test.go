package parity

// A canonical, structural description of a PDF file.
//
// Comparing generated PDFs byte for byte proves nothing: two independent
// generators will never agree on object numbering, whitespace, resource naming
// or operator spelling. So both runners hand back the finished file and *this*
// file parses it — one parser, both sides, so any difference in the canonical
// form is a difference in the document, never a difference in how it was read.
//
// What is compared, per document:
//
//   - the %PDF-x.y header version;
//   - the page count;
//   - per page: MediaBox, CropBox, /Rotate, and the resource dictionaries that
//     are actually referenced (fonts with base font name/subtype/encoding,
//     XObjects with subtype/size/filter/colour space/soft-mask, ExtGState,
//     Shading, Pattern, colour spaces, ProcSet);
//   - per page: the annotations, with rectangle, subtype and action/destination;
//   - per page: a normalised content-stream operator trace (see traceStream);
//   - the /Info dictionary keys and values;
//   - the presence and shape of the outline, AcroForm and named destinations;
//   - whether the document is encrypted, and the standard security handler's
//     V/R/Length/P.
//
// Numeric precision. Every number in the trace and in the geometry fields is
// rounded to THREE decimal places. 0.001 pt is 1/72000 inch — about 0.35 µm, and
// roughly 1/4000 of a pixel at 72 dpi, so it is far below anything that could
// affect rendering. It is also comfortably above the noise floor: the two
// implementations reach the same coordinates through different arithmetic
// (Bézier circle approximation with a kappa constant, CTM composition, AFM
// metric division by 1000), which introduces error in the 1e-9..1e-12 range.
// Three decimals is therefore the widest tolerance that still distinguishes
// genuinely different geometry and the tightest that ignores float noise.
// Colours live in [0,1], where 0.001 still separates adjacent 8-bit levels
// (1/255 = 0.0039), so the same precision serves them.

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// PDF object model

type pdfNull struct{}

type pdfRef int

type pdfName string

type pdfStr []byte

type pdfDict map[string]any

type pdfStream struct {
	Dict pdfDict
	Raw  []byte
}

// ---------------------------------------------------------------------------
// lexer / parser

type lexer struct {
	buf []byte
	pos int
}

func isWS(c byte) bool {
	return c == 0x00 || c == 0x09 || c == 0x0a || c == 0x0c || c == 0x0d || c == 0x20
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (l *lexer) skipWS() {
	for l.pos < len(l.buf) {
		c := l.buf[l.pos]
		if isWS(c) {
			l.pos++
			continue
		}
		if c == '%' {
			for l.pos < len(l.buf) && l.buf[l.pos] != '\n' && l.buf[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		return
	}
}

// token returns the next raw token: a delimiter run, a name, a string, or a
// bare keyword/number.
func (l *lexer) token() string {
	l.skipWS()
	if l.pos >= len(l.buf) {
		return ""
	}
	c := l.buf[l.pos]
	switch {
	case c == '<' && l.pos+1 < len(l.buf) && l.buf[l.pos+1] == '<':
		l.pos += 2
		return "<<"
	case c == '>' && l.pos+1 < len(l.buf) && l.buf[l.pos+1] == '>':
		l.pos += 2
		return ">>"
	case c == '[' || c == ']' || c == '{' || c == '}':
		l.pos++
		return string(c)
	case c == '/':
		start := l.pos
		l.pos++
		for l.pos < len(l.buf) && !isWS(l.buf[l.pos]) && !isDelim(l.buf[l.pos]) {
			l.pos++
		}
		return string(l.buf[start:l.pos])
	case c == '(' || c == '<':
		start := l.pos
		l.readStringRaw()
		return string(l.buf[start:l.pos])
	}
	start := l.pos
	for l.pos < len(l.buf) && !isWS(l.buf[l.pos]) && !isDelim(l.buf[l.pos]) {
		l.pos++
	}
	if l.pos == start { // lone delimiter we do not understand
		l.pos++
	}
	return string(l.buf[start:l.pos])
}

func (l *lexer) peekToken() string {
	save := l.pos
	t := l.token()
	l.pos = save
	return t
}

// readStringRaw advances past a literal "(...)" or hex "<...>" string.
func (l *lexer) readStringRaw() {
	if l.buf[l.pos] == '(' {
		depth := 0
		for l.pos < len(l.buf) {
			c := l.buf[l.pos]
			if c == '\\' {
				l.pos += 2
				continue
			}
			l.pos++
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					return
				}
			}
		}
		return
	}
	for l.pos < len(l.buf) {
		c := l.buf[l.pos]
		l.pos++
		if c == '>' {
			return
		}
	}
}

// decodeString turns a raw PDF string token into its bytes.
func decodeString(tok string) []byte {
	if len(tok) == 0 {
		return nil
	}
	if tok[0] == '<' {
		var digits []byte
		for i := 1; i < len(tok); i++ {
			c := tok[i]
			if c == '>' {
				break
			}
			if !isWS(c) {
				digits = append(digits, c)
			}
		}
		if len(digits)%2 == 1 {
			digits = append(digits, '0')
		}
		out := make([]byte, len(digits)/2)
		if _, err := hex.Decode(out, digits); err != nil {
			return nil
		}
		return out
	}
	// literal string
	body := tok
	if body[0] == '(' {
		body = body[1:]
	}
	if strings.HasSuffix(body, ")") {
		body = body[:len(body)-1]
	}
	var out []byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			out = append(out, c)
			continue
		}
		i++
		if i >= len(body) {
			break
		}
		switch e := body[i]; e {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, e)
		case '\n':
		case '\r':
			if i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
		default:
			if e >= '0' && e <= '7' {
				v := int(e - '0')
				for k := 0; k < 2 && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '7'; k++ {
					i++
					v = v*8 + int(body[i]-'0')
				}
				out = append(out, byte(v))
			} else {
				out = append(out, e)
			}
		}
	}
	return out
}

var numRe = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)$`)

// object parses one object at the current position.
func (l *lexer) object() any {
	l.skipWS()
	if l.pos >= len(l.buf) {
		return pdfNull{}
	}
	save := l.pos
	tok := l.token()
	switch {
	case tok == "":
		return pdfNull{}
	case tok == "<<":
		d := pdfDict{}
		for {
			l.skipWS()
			t := l.peekToken()
			if t == ">>" || t == "" {
				l.token()
				break
			}
			if !strings.HasPrefix(t, "/") {
				// malformed key; consume and continue
				l.token()
				continue
			}
			l.token()
			d[t[1:]] = l.object()
		}
		// a stream may follow
		mark := l.pos
		l.skipWS()
		if bytes.HasPrefix(l.buf[l.pos:], []byte("stream")) {
			l.pos += len("stream")
			if l.pos < len(l.buf) && l.buf[l.pos] == '\r' {
				l.pos++
			}
			if l.pos < len(l.buf) && l.buf[l.pos] == '\n' {
				l.pos++
			}
			start := l.pos
			end := -1
			if n, ok := d["Length"].(float64); ok {
				cand := start + int(n)
				if cand <= len(l.buf) {
					probe := cand
					for probe < len(l.buf) && isWS(l.buf[probe]) {
						probe++
					}
					if bytes.HasPrefix(l.buf[probe:], []byte("endstream")) {
						end = cand
						l.pos = probe + len("endstream")
					}
				}
			}
			if end < 0 {
				idx := bytes.Index(l.buf[start:], []byte("endstream"))
				if idx < 0 {
					end = len(l.buf)
					l.pos = end
				} else {
					end = start + idx
					l.pos = end + len("endstream")
					for end > start && (l.buf[end-1] == '\n' || l.buf[end-1] == '\r') {
						end--
					}
				}
			}
			return pdfStream{Dict: d, Raw: l.buf[start:end]}
		}
		l.pos = mark
		return d
	case tok == "[":
		var arr []any
		for {
			l.skipWS()
			t := l.peekToken()
			if t == "]" || t == "" {
				l.token()
				break
			}
			arr = append(arr, l.object())
		}
		if arr == nil {
			arr = []any{}
		}
		return arr
	case strings.HasPrefix(tok, "/"):
		return pdfName(tok[1:])
	case tok[0] == '(' || tok[0] == '<':
		return pdfStr(decodeString(tok))
	case tok == "true":
		return true
	case tok == "false":
		return false
	case tok == "null":
		return pdfNull{}
	case numRe.MatchString(tok):
		// possibly "N G R" (an indirect reference)
		mark := l.pos
		t2 := l.token()
		if numRe.MatchString(t2) {
			t3 := l.token()
			if t3 == "R" {
				n, _ := strconv.Atoi(strings.TrimPrefix(tok, "+"))
				return pdfRef(n)
			}
		}
		l.pos = mark
		f, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return pdfNull{}
		}
		return f
	}
	// unknown keyword
	_ = save
	return pdfName("<keyword:" + tok + ">")
}

// ---------------------------------------------------------------------------
// document

type pdfDoc struct {
	objs    map[int]any
	trailer pdfDict
	version string
	// pageIndex maps a page object number to its 0-based index.
	pageIndex map[int]int
	broken    []string
}

var objHeaderRe = regexp.MustCompile(`(?s)(?:^|[\r\n\s])(\d+)[\s]+(\d+)[\s]+obj\b`)

// parsePDF reads a whole PDF by scanning for indirect objects rather than
// following the xref table, so a broken xref cannot hide content. Object streams
// are expanded. Anything structurally impossible is recorded in broken.
func parsePDF(data []byte) *pdfDoc {
	d := &pdfDoc{objs: map[int]any{}, trailer: pdfDict{}, pageIndex: map[int]int{}}

	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		d.broken = append(d.broken, "missing %PDF- header")
	} else {
		end := bytes.IndexAny(data[5:], "\r\n \t")
		if end < 0 {
			end = len(data) - 5
		}
		d.version = string(data[5 : 5+end])
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		d.broken = append(d.broken, "missing %%EOF")
	}
	if !bytes.Contains(data, []byte("startxref")) {
		d.broken = append(d.broken, "missing startxref")
	}

	for _, m := range objHeaderRe.FindAllSubmatchIndex(data, -1) {
		num, err := strconv.Atoi(string(data[m[2]:m[3]]))
		if err != nil {
			continue
		}
		l := &lexer{buf: data, pos: m[1]}
		obj := l.object()
		if _, dup := d.objs[num]; dup {
			// A later definition wins, as an incremental update would.
			d.broken = append(d.broken, fmt.Sprintf("object %d defined more than once", num))
		}
		d.objs[num] = obj
	}

	// trailer dictionaries (there may be several; merge, later wins)
	for _, idx := range allIndexes(data, []byte("trailer")) {
		l := &lexer{buf: data, pos: idx + len("trailer")}
		if t, ok := l.object().(pdfDict); ok {
			for k, v := range t {
				d.trailer[k] = v
			}
		}
	}

	// expand object streams and pick up xref-stream trailers
	for num, o := range d.objs {
		st, ok := o.(pdfStream)
		if !ok {
			continue
		}
		switch st.Dict["Type"] {
		case pdfName("XRef"):
			for _, k := range []string{"Root", "Info", "Encrypt", "ID", "Size"} {
				if v, ok := st.Dict[k]; ok {
					if _, have := d.trailer[k]; !have {
						d.trailer[k] = v
					}
				}
			}
		case pdfName("ObjStm"):
			d.expandObjStm(num, st)
		}
	}

	if _, ok := d.trailer["Root"]; !ok {
		// fall back to locating the catalog directly
		for num, o := range d.objs {
			if dd, ok := o.(pdfDict); ok && dd["Type"] == pdfName("Catalog") {
				d.trailer["Root"] = pdfRef(num)
				break
			}
		}
		if _, ok := d.trailer["Root"]; !ok {
			d.broken = append(d.broken, "no /Root and no /Type /Catalog object")
		} else {
			d.broken = append(d.broken, "trailer has no /Root")
		}
	}
	return d
}

func (d *pdfDoc) expandObjStm(num int, st pdfStream) {
	data, err := decodeStream(st)
	if err != nil {
		d.broken = append(d.broken, fmt.Sprintf("object stream %d: %v", num, err))
		return
	}
	n, _ := d.num(st.Dict["N"])
	first, _ := d.num(st.Dict["First"])
	hdr := &lexer{buf: data}
	type ent struct{ num, off int }
	var ents []ent
	for i := 0; i < int(n); i++ {
		a := hdr.token()
		b := hdr.token()
		an, err1 := strconv.Atoi(a)
		bn, err2 := strconv.Atoi(b)
		if err1 != nil || err2 != nil {
			d.broken = append(d.broken, fmt.Sprintf("object stream %d: bad index", num))
			return
		}
		ents = append(ents, ent{an, bn})
	}
	for _, e := range ents {
		p := int(first) + e.off
		if p < 0 || p > len(data) {
			continue
		}
		l := &lexer{buf: data, pos: p}
		d.objs[e.num] = l.object()
	}
}

func allIndexes(hay, needle []byte) []int {
	var out []int
	off := 0
	for {
		i := bytes.Index(hay[off:], needle)
		if i < 0 {
			return out
		}
		out = append(out, off+i)
		off += i + len(needle)
	}
}

func (d *pdfDoc) resolve(v any) any {
	for i := 0; i < 32; i++ {
		r, ok := v.(pdfRef)
		if !ok {
			return v
		}
		nv, ok := d.objs[int(r)]
		if !ok {
			return pdfNull{}
		}
		v = nv
	}
	return pdfNull{}
}

func (d *pdfDoc) dict(v any) pdfDict {
	switch x := d.resolve(v).(type) {
	case pdfDict:
		return x
	case pdfStream:
		return x.Dict
	}
	return nil
}

func (d *pdfDoc) arr(v any) []any {
	if a, ok := d.resolve(v).([]any); ok {
		return a
	}
	return nil
}

func (d *pdfDoc) num(v any) (float64, bool) {
	f, ok := d.resolve(v).(float64)
	return f, ok
}

func (d *pdfDoc) name(v any) string {
	if n, ok := d.resolve(v).(pdfName); ok {
		return string(n)
	}
	return ""
}

func (d *pdfDoc) str(v any) (string, bool) {
	s, ok := d.resolve(v).(pdfStr)
	if !ok {
		return "", false
	}
	return decodeTextString(s), true
}

// decodeTextString renders a PDF text string: UTF-16BE when it carries the BOM,
// otherwise the bytes as-is (PDFDocEncoding is ASCII-compatible for our cases).
func decodeTextString(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		var sb strings.Builder
		for i := 2; i+1 < len(b); i += 2 {
			sb.WriteRune(rune(int(b[i])<<8 | int(b[i+1])))
		}
		return sb.String()
	}
	return string(b)
}

func decodeStream(st pdfStream) ([]byte, error) {
	filters := []string{}
	switch f := st.Dict["Filter"].(type) {
	case pdfName:
		filters = append(filters, string(f))
	case []any:
		for _, x := range f {
			if n, ok := x.(pdfName); ok {
				filters = append(filters, string(n))
			}
		}
	}
	data := st.Raw
	for _, f := range filters {
		switch f {
		case "FlateDecode":
			zr, err := zlib.NewReader(bytes.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("FlateDecode: %w", err)
			}
			out, err := io.ReadAll(zr)
			zr.Close()
			if err != nil {
				return nil, fmt.Errorf("FlateDecode: %w", err)
			}
			data = out
		default:
			return nil, fmt.Errorf("unsupported stream filter %s", f)
		}
	}
	return data, nil
}

func filterNames(d pdfDict) []string {
	switch f := d["Filter"].(type) {
	case pdfName:
		return []string{string(f)}
	case []any:
		var out []string
		for _, x := range f {
			if n, ok := x.(pdfName); ok {
				out = append(out, string(n))
			}
		}
		return out
	}
	return nil
}

// ---------------------------------------------------------------------------
// numbers

// fnum quantises a number for comparison and formats it without trailing zeros
// or a negative zero.
//
// Two steps, both necessary:
//
//  1. snap to the 1e-4 grid. The Go port serialises every coordinate with
//     exactly four decimal places, so its own output is already quantised to
//     1e-4 while upstream writes full double precision. Snapping first means
//     the two sides land on the *same* value whenever they agree to within the
//     port's output resolution, and cannot be split by it.
//  2. round the snapped value to three decimals (see the precision note at the
//     top of this file). Without step 1 this would split, for example,
//     upstream's 103.4314575 (-> 103.431) from the port's "103.4315"
//     (-> 103.432), which is the port's rounding, not different geometry.
func fnum(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 0) {
		return "Inf"
	}
	r := math.Round(f*1e4) / 1e4
	r = math.Round(r*1000) / 1000
	if r == 0 {
		r = 0
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

func fnums(fs ...float64) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = fnum(f)
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// matrices (PDF row-vector convention: [a b c d e f])

type mat [6]float64

var identity = mat{1, 0, 0, 1, 0, 0}

// mul returns m concatenated onto n, i.e. the matrix product m × n.
func mul(m, n mat) mat {
	return mat{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m mat) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

// ---------------------------------------------------------------------------
// canonical form

type canonAnnot struct {
	Subtype string   `json:"subtype"`
	Rect    string   `json:"rect"`
	Extra   []string `json:"extra,omitempty"`
}

type canonPage struct {
	MediaBox       string       `json:"mediaBox"`
	CropBox        string       `json:"cropBox,omitempty"`
	Rotate         string       `json:"rotate"`
	ProcSet        []string     `json:"procSet,omitempty"`
	Fonts          []string     `json:"fonts,omitempty"`
	XObjects       []string     `json:"xObjects,omitempty"`
	ExtGState      []string     `json:"extGState,omitempty"`
	Shading        []string     `json:"shading,omitempty"`
	Pattern        []string     `json:"pattern,omitempty"`
	ColorSpace     []string     `json:"colorSpace,omitempty"`
	ContentFilters []string     `json:"contentFilters,omitempty"`
	Annots         []canonAnnot `json:"annots,omitempty"`
	Trace          []string     `json:"trace"`
}

type canonOutline struct {
	Title    string         `json:"title"`
	Dest     string         `json:"dest"`
	Children []canonOutline `json:"children,omitempty"`
}

type canonDoc struct {
	Version     string         `json:"version"`
	Broken      []string       `json:"broken,omitempty"`
	PageCount   int            `json:"pageCount"`
	IDPresent   bool           `json:"idPresent"`
	Encrypted   bool           `json:"encrypted"`
	Encrypt     []string       `json:"encrypt,omitempty"`
	Info        []string       `json:"info,omitempty"`
	Metadata    bool           `json:"xmpMetadata"`
	HasOutline  bool           `json:"hasOutline"`
	Outline     []canonOutline `json:"outline,omitempty"`
	HasAcroForm bool           `json:"hasAcroForm"`
	AcroForm    []string       `json:"acroForm,omitempty"`
	NamedDests  []string       `json:"namedDests,omitempty"`
	Pages       []canonPage    `json:"pages"`
}

// Canonicalise parses a PDF and derives its canonical structural description.
func Canonicalise(pdf []byte) *canonDoc {
	d := parsePDF(pdf)
	c := &canonDoc{Version: d.version, Broken: d.broken}

	if _, ok := d.trailer["ID"]; ok {
		c.IDPresent = true
	}
	if enc := d.dict(d.trailer["Encrypt"]); enc != nil {
		c.Encrypted = true
		c.Encrypt = append(c.Encrypt, "Filter="+d.name(enc["Filter"]))
		for _, k := range []string{"V", "R", "Length", "P"} {
			if v, ok := d.num(enc[k]); ok {
				c.Encrypt = append(c.Encrypt, k+"="+fnum(v))
			}
		}
		if sub := d.dict(enc["CF"]); sub != nil {
			if std := d.dict(sub["StdCF"]); std != nil {
				c.Encrypt = append(c.Encrypt, "StdCF.CFM="+d.name(std["CFM"]))
			}
		}
	}

	root := d.dict(d.trailer["Root"])
	if root == nil {
		c.Broken = append(c.Broken, "catalog is not a dictionary")
		return c
	}

	// Info. Once the document is encrypted every string in it — including these
	// — is ciphertext under a key derived from the file id, so only the key names
	// are comparable.
	if info := d.dict(d.trailer["Info"]); info != nil {
		keys := make([]string, 0, len(info))
		for k := range info {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			var val string
			switch {
			case c.Encrypted:
				val = "<encrypted>"
			case k == "CreationDate" || k == "ModDate":
				// Pinned on both sides, but the two encode the value
				// differently; only presence is compared.
				val = "<date>"
			default:
				if s, ok := d.str(info[k]); ok {
					val = s
				} else {
					val = fmt.Sprintf("%v", d.resolve(info[k]))
				}
			}
			c.Info = append(c.Info, k+"="+val)
		}
	}
	if _, ok := root["Metadata"]; ok {
		c.Metadata = true
	}

	// pages
	var pageObjs []pdfDict
	var pageNums []int
	seen := map[int]bool{}
	var walk func(v any, inherited pdfDict, depth int)
	walk = func(v any, inherited pdfDict, depth int) {
		if depth > 64 {
			c.Broken = append(c.Broken, "page tree deeper than 64 levels")
			return
		}
		if r, ok := v.(pdfRef); ok {
			if seen[int(r)] {
				c.Broken = append(c.Broken, fmt.Sprintf("page tree revisits object %d", int(r)))
				return
			}
			seen[int(r)] = true
		}
		node := d.dict(v)
		if node == nil {
			c.Broken = append(c.Broken, "page tree node is not a dictionary")
			return
		}
		merged := pdfDict{}
		for k, val := range inherited {
			merged[k] = val
		}
		for _, k := range []string{"Resources", "MediaBox", "CropBox", "Rotate"} {
			if val, ok := node[k]; ok {
				merged[k] = val
			}
		}
		switch node["Type"] {
		case pdfName("Pages"):
			kids := d.arr(node["Kids"])
			if kids == nil {
				c.Broken = append(c.Broken, "/Pages node with no /Kids")
			}
			for _, k := range kids {
				walk(k, merged, depth+1)
			}
		case pdfName("Page"):
			eff := pdfDict{}
			for k, val := range node {
				eff[k] = val
			}
			for _, k := range []string{"Resources", "MediaBox", "CropBox", "Rotate"} {
				if _, ok := eff[k]; !ok {
					if val, ok := merged[k]; ok {
						eff[k] = val
					}
				}
			}
			if r, ok := v.(pdfRef); ok {
				pageNums = append(pageNums, int(r))
			} else {
				pageNums = append(pageNums, -1)
			}
			pageObjs = append(pageObjs, eff)
		default:
			c.Broken = append(c.Broken, fmt.Sprintf("page tree node with unexpected /Type %v", node["Type"]))
		}
	}
	walk(root["Pages"], pdfDict{}, 0)
	for i, n := range pageNums {
		if n >= 0 {
			d.pageIndex[n] = i
		}
	}
	c.PageCount = len(pageObjs)
	if cnt, ok := d.num(d.dict(root["Pages"])["Count"]); ok && int(cnt) != len(pageObjs) {
		c.Broken = append(c.Broken, fmt.Sprintf("/Pages /Count is %d but %d page objects are reachable", int(cnt), len(pageObjs)))
	}

	for _, pg := range pageObjs {
		c.Pages = append(c.Pages, d.canonPage(c, pg))
	}

	// outline
	if ol := d.dict(root["Outlines"]); ol != nil {
		c.HasOutline = true
		c.Outline = d.canonOutlineChildren(ol, 0)
	}
	// AcroForm
	if af := d.dict(root["AcroForm"]); af != nil {
		c.HasAcroForm = true
		if v, ok := d.resolve(af["NeedAppearances"]).(bool); ok {
			c.AcroForm = append(c.AcroForm, "NeedAppearances="+strconv.FormatBool(v))
		}
		var fields []string
		for _, f := range d.arr(af["Fields"]) {
			fd := d.dict(f)
			if fd == nil {
				continue
			}
			name, _ := d.str(fd["T"])
			parts := []string{"name=" + name, "FT=" + d.name(fd["FT"])}
			if v, ok := d.str(fd["V"]); ok {
				parts = append(parts, "V="+v)
			} else if n := d.name(fd["V"]); n != "" {
				parts = append(parts, "V=/"+n)
			}
			if ff, ok := d.num(fd["Ff"]); ok {
				parts = append(parts, "Ff="+fnum(ff))
			}
			if _, ok := fd["AP"]; ok {
				parts = append(parts, "AP=present")
			} else {
				parts = append(parts, "AP=absent")
			}
			fields = append(fields, strings.Join(parts, " "))
		}
		sort.Strings(fields)
		c.AcroForm = append(c.AcroForm, fields...)
	}
	// named destinations
	if names := d.dict(root["Names"]); names != nil {
		c.NamedDests = d.collectNameTree(names["Dests"], 0)
	}
	if dests := d.dict(root["Dests"]); dests != nil {
		keys := make([]string, 0, len(dests))
		for k := range dests {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c.NamedDests = append(c.NamedDests, k+" -> "+d.destString(dests[k]))
		}
	}
	sort.Strings(c.NamedDests)
	return c
}

func (d *pdfDoc) collectNameTree(v any, depth int) []string {
	if depth > 32 {
		return nil
	}
	node := d.dict(v)
	if node == nil {
		return nil
	}
	var out []string
	nameArr := d.arr(node["Names"])
	for i := 0; i+1 < len(nameArr); i += 2 {
		key, _ := d.str(nameArr[i])
		out = append(out, key+" -> "+d.destString(nameArr[i+1]))
	}
	for _, k := range d.arr(node["Kids"]) {
		out = append(out, d.collectNameTree(k, depth+1)...)
	}
	sort.Strings(out)
	return out
}

// destString renders a destination: the target page index plus the view mode
// and its (rounded) arguments.
func (d *pdfDoc) destString(v any) string {
	rv := d.resolve(v)
	if dd, ok := rv.(pdfDict); ok {
		if inner, ok := dd["D"]; ok {
			return d.destString(inner)
		}
	}
	if s, ok := rv.(pdfStr); ok {
		return "named:" + string(s)
	}
	if n, ok := rv.(pdfName); ok {
		return "named:" + string(n)
	}
	a, ok := rv.([]any)
	if !ok || len(a) == 0 {
		return "<none>"
	}
	target := "?"
	if r, ok := a[0].(pdfRef); ok {
		if idx, ok := d.pageIndex[int(r)]; ok {
			target = "page" + strconv.Itoa(idx)
		} else {
			target = "obj" + strconv.Itoa(int(r))
		}
	} else if f, ok := a[0].(float64); ok {
		target = "page" + fnum(f)
	}
	parts := []string{target}
	for _, x := range a[1:] {
		switch t := d.resolve(x).(type) {
		case pdfName:
			parts = append(parts, string(t))
		case float64:
			parts = append(parts, fnum(t))
		case pdfNull:
			parts = append(parts, "null")
		default:
			parts = append(parts, "null")
		}
	}
	return strings.Join(parts, " ")
}

func (d *pdfDoc) canonOutlineChildren(node pdfDict, depth int) []canonOutline {
	if depth > 32 {
		return nil
	}
	var out []canonOutline
	cur := node["First"]
	for i := 0; i < 4096; i++ {
		cd := d.dict(cur)
		if cd == nil {
			break
		}
		title, _ := d.str(cd["Title"])
		item := canonOutline{Title: title, Dest: d.destString(cd["Dest"])}
		if _, ok := cd["A"]; ok {
			if a := d.dict(cd["A"]); a != nil {
				item.Dest = "action:" + d.name(a["S"]) + " " + d.destString(a["D"])
			}
		}
		item.Children = d.canonOutlineChildren(cd, depth+1)
		out = append(out, item)
		nxt, ok := cd["Next"]
		if !ok {
			break
		}
		cur = nxt
	}
	return out
}

func (d *pdfDoc) rectString(v any) string {
	a := d.arr(v)
	if len(a) != 4 {
		return "<invalid>"
	}
	vals := make([]float64, 4)
	for i := range a {
		f, _ := d.num(a[i])
		vals[i] = f
	}
	// normalise corner order
	x0, x1 := math.Min(vals[0], vals[2]), math.Max(vals[0], vals[2])
	y0, y1 := math.Min(vals[1], vals[3]), math.Max(vals[1], vals[3])
	return fnums(x0, y0, x1, y1)
}

func (d *pdfDoc) canonPage(c *canonDoc, pg pdfDict) canonPage {
	out := canonPage{Rotate: "0"}
	if mb, ok := pg["MediaBox"]; ok {
		out.MediaBox = d.rectString(mb)
	} else {
		out.MediaBox = "<missing>"
		c.Broken = append(c.Broken, "page has no /MediaBox")
	}
	if cb, ok := pg["CropBox"]; ok {
		out.CropBox = d.rectString(cb)
	}
	if r, ok := d.num(pg["Rotate"]); ok {
		out.Rotate = fnum(r)
	}

	res := d.dict(pg["Resources"])
	names := newNameMap()

	if res != nil {
		switch ps := d.resolve(res["ProcSet"]).(type) {
		case []any:
			for _, x := range ps {
				out.ProcSet = append(out.ProcSet, d.name(x))
			}
			sort.Strings(out.ProcSet)
		case pdfName:
			out.ProcSet = []string{string(ps)}
		}
		if fonts := d.dict(res["Font"]); fonts != nil {
			for _, k := range sortedKeys(fonts) {
				desc := d.fontDesc(fonts[k])
				names.font[k] = desc
				out.Fonts = append(out.Fonts, desc)
			}
			sort.Strings(out.Fonts)
		}
		if xo := d.dict(res["XObject"]); xo != nil {
			for _, k := range sortedKeys(xo) {
				desc := d.xobjDesc(xo[k])
				names.xobj[k] = desc
				out.XObjects = append(out.XObjects, desc)
			}
			sort.Strings(out.XObjects)
		}
		if gs := d.dict(res["ExtGState"]); gs != nil {
			for _, k := range sortedKeys(gs) {
				desc := d.gsDesc(gs[k])
				names.extg[k] = desc
				out.ExtGState = append(out.ExtGState, desc)
			}
			sort.Strings(out.ExtGState)
		}
		if sh := d.dict(res["Shading"]); sh != nil {
			for _, k := range sortedKeys(sh) {
				desc := d.shadingDesc(sh[k])
				names.shading[k] = desc
				out.Shading = append(out.Shading, desc)
			}
			sort.Strings(out.Shading)
		}
		if pt := d.dict(res["Pattern"]); pt != nil {
			for _, k := range sortedKeys(pt) {
				desc := d.patternDesc(pt[k])
				names.pattern[k] = desc
				out.Pattern = append(out.Pattern, desc)
			}
			sort.Strings(out.Pattern)
		}
		if cs := d.dict(res["ColorSpace"]); cs != nil {
			for _, k := range sortedKeys(cs) {
				desc := describeAny(d, cs[k], 0)
				names.cspace[k] = desc
				out.ColorSpace = append(out.ColorSpace, desc)
			}
			sort.Strings(out.ColorSpace)
		}
	}

	// annotations
	for _, a := range d.arr(pg["Annots"]) {
		ad := d.dict(a)
		if ad == nil {
			c.Broken = append(c.Broken, "annotation is not a dictionary")
			continue
		}
		an := canonAnnot{Subtype: d.name(ad["Subtype"]), Rect: d.rectString(ad["Rect"])}
		if act := d.dict(ad["A"]); act != nil {
			s := "A.S=" + d.name(act["S"])
			if uri, ok := d.str(act["URI"]); ok {
				s += " URI=" + uri
			}
			if dst, ok := act["D"]; ok {
				s += " D=" + d.destString(dst)
			}
			an.Extra = append(an.Extra, s)
		}
		if dst, ok := ad["Dest"]; ok {
			an.Extra = append(an.Extra, "Dest="+d.destString(dst))
		}
		if ft := d.name(ad["FT"]); ft != "" {
			an.Extra = append(an.Extra, "FT="+ft)
		}
		if t, ok := d.str(ad["T"]); ok {
			an.Extra = append(an.Extra, "T="+t)
		}
		if v, ok := d.str(ad["Contents"]); ok {
			an.Extra = append(an.Extra, "Contents="+v)
		}
		if _, ok := ad["AP"]; ok {
			an.Extra = append(an.Extra, "AP=present")
		}
		if qp := d.arr(ad["QuadPoints"]); qp != nil {
			var fs []float64
			for _, x := range qp {
				f, _ := d.num(x)
				fs = append(fs, f)
			}
			an.Extra = append(an.Extra, "QuadPoints="+fnums(fs...))
		}
		if fl, ok := d.num(ad["F"]); ok {
			an.Extra = append(an.Extra, "F="+fnum(fl))
		}
		if bd := d.arr(ad["Border"]); bd != nil {
			var fs []float64
			for _, x := range bd {
				f, _ := d.num(x)
				fs = append(fs, f)
			}
			an.Extra = append(an.Extra, "Border="+fnums(fs...))
		}
		out.Annots = append(out.Annots, an)
	}

	// content
	if c.Encrypted {
		out.Trace = []string{"<encrypted: content stream not comparable>"}
		return out
	}
	var content []byte
	var filters []string
	appendStream := func(v any) {
		st, ok := d.resolve(v).(pdfStream)
		if !ok {
			c.Broken = append(c.Broken, "page /Contents is not a stream")
			return
		}
		filters = append(filters, filterNames(st.Dict)...)
		data, err := decodeStream(st)
		if err != nil {
			c.Broken = append(c.Broken, "page content stream: "+err.Error())
			return
		}
		if len(content) > 0 {
			content = append(content, '\n')
		}
		content = append(content, data...)
	}
	switch cv := d.resolve(pg["Contents"]).(type) {
	case pdfStream:
		appendStream(pg["Contents"])
	case []any:
		for _, x := range cv {
			appendStream(x)
		}
	default:
		c.Broken = append(c.Broken, "page has no /Contents stream")
	}
	out.ContentFilters = filters
	out.Trace = traceStream(content, names)
	return out
}

func sortedKeys(d pdfDict) []string {
	out := make([]string, 0, len(d))
	for k := range d {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (d *pdfDoc) fontDesc(v any) string {
	f := d.dict(v)
	if f == nil {
		return "<invalid font>"
	}
	parts := []string{"Subtype=" + d.name(f["Subtype"]), "BaseFont=" + d.name(f["BaseFont"])}
	if enc := d.resolve(f["Encoding"]); enc != nil {
		switch e := enc.(type) {
		case pdfName:
			parts = append(parts, "Encoding="+string(e))
		case pdfDict:
			parts = append(parts, "Encoding=dict:"+d.name(e["BaseEncoding"]))
		default:
			parts = append(parts, "Encoding=<none>")
		}
	}
	if _, ok := f["FirstChar"]; ok {
		parts = append(parts, "Widths=present")
	}
	if _, ok := f["FontDescriptor"]; ok {
		parts = append(parts, "FontDescriptor=present")
	}
	if _, ok := f["ToUnicode"]; ok {
		parts = append(parts, "ToUnicode=present")
	}
	if kids := d.arr(f["DescendantFonts"]); kids != nil {
		parts = append(parts, "DescendantFonts="+strconv.Itoa(len(kids)))
	}
	return strings.Join(parts, " ")
}

func (d *pdfDoc) xobjDesc(v any) string {
	st, ok := d.resolve(v).(pdfStream)
	if !ok {
		return "<invalid xobject>"
	}
	x := st.Dict
	parts := []string{"Subtype=" + d.name(x["Subtype"])}
	if w, ok := d.num(x["Width"]); ok {
		h, _ := d.num(x["Height"])
		parts = append(parts, "Width="+fnum(w), "Height="+fnum(h))
	}
	if f := filterNames(x); len(f) > 0 {
		parts = append(parts, "Filter="+strings.Join(f, ","))
	} else {
		parts = append(parts, "Filter=<none>")
	}
	if cs := x["ColorSpace"]; cs != nil {
		parts = append(parts, "ColorSpace="+describeAny(d, cs, 0))
	}
	if b, ok := d.num(x["BitsPerComponent"]); ok {
		parts = append(parts, "BitsPerComponent="+fnum(b))
	}
	if dp := d.dict(x["DecodeParms"]); dp != nil {
		var kv []string
		for _, k := range sortedKeys(dp) {
			kv = append(kv, k+"="+describeAny(d, dp[k], 0))
		}
		parts = append(parts, "DecodeParms{"+strings.Join(kv, " ")+"}")
	}
	if _, ok := x["SMask"]; ok {
		parts = append(parts, "SMask=present")
	}
	if _, ok := x["Mask"]; ok {
		parts = append(parts, "Mask=present")
	}
	if bb := d.arr(x["BBox"]); bb != nil {
		parts = append(parts, "BBox="+d.rectString(x["BBox"]))
	}
	return strings.Join(parts, " ")
}

func (d *pdfDoc) gsDesc(v any) string {
	g := d.dict(v)
	if g == nil {
		return "<invalid extgstate>"
	}
	var parts []string
	for _, k := range sortedKeys(g) {
		if k == "Type" {
			continue
		}
		parts = append(parts, k+"="+describeAny(d, g[k], 0))
	}
	return strings.Join(parts, " ")
}

func (d *pdfDoc) shadingDesc(v any) string {
	return d.shadingDescIn(v, identity)
}

// shadingDescIn describes a shading with its gradient geometry expressed in the
// space m maps into. A shading pattern's /Matrix and its /Coords are two ways of
// saying where the gradient runs, so the matrix is composed into the coordinates
// (and the radii scaled by its scale factor) rather than compared separately.
func (d *pdfDoc) shadingDescIn(v any, m mat) string {
	s := d.dict(v)
	if s == nil {
		return "<invalid shading>"
	}
	parts := []string{}
	if val, ok := s["ShadingType"]; ok {
		parts = append(parts, "ShadingType="+describeAny(d, val, 0))
	}
	if val, ok := s["ColorSpace"]; ok {
		parts = append(parts, "ColorSpace="+describeAny(d, val, 0))
	}
	if coords := d.arr(s["Coords"]); coords != nil {
		vals := make([]float64, len(coords))
		for i := range coords {
			vals[i], _ = d.num(coords[i])
		}
		scale := math.Sqrt(math.Abs(m[0]*m[3] - m[1]*m[2]))
		var mapped []float64
		switch len(vals) {
		case 4: // axial: x0 y0 x1 y1
			x0, y0 := m.apply(vals[0], vals[1])
			x1, y1 := m.apply(vals[2], vals[3])
			mapped = []float64{x0, y0, x1, y1}
		case 6: // radial: x0 y0 r0 x1 y1 r1
			x0, y0 := m.apply(vals[0], vals[1])
			x1, y1 := m.apply(vals[3], vals[4])
			mapped = []float64{x0, y0, vals[2] * scale, x1, y1, vals[5] * scale}
		default:
			mapped = vals
		}
		parts = append(parts, "CoordsInPageSpace=["+fnums(mapped...)+"]")
	}
	for _, k := range []string{"Domain", "Extend", "Background"} {
		if val, ok := s[k]; ok {
			parts = append(parts, k+"="+describeAny(d, val, 0))
		}
	}
	if fn, ok := s["Function"]; ok {
		parts = append(parts, "Function="+d.functionDesc(fn, 0))
	}
	return strings.Join(parts, " ")
}

func (d *pdfDoc) functionDesc(v any, depth int) string {
	if depth > 8 {
		return "<deep>"
	}
	if a, ok := d.resolve(v).([]any); ok {
		var parts []string
		for _, x := range a {
			parts = append(parts, d.functionDesc(x, depth+1))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	f := d.dict(v)
	if f == nil {
		return "<invalid function>"
	}
	parts := []string{}
	for _, k := range []string{"FunctionType", "Domain", "Range", "C0", "C1", "N", "Bounds", "Encode", "Size", "BitsPerSample"} {
		if val, ok := f[k]; ok {
			parts = append(parts, k+"="+describeAny(d, val, 0))
		}
	}
	if fns, ok := f["Functions"]; ok {
		parts = append(parts, "Functions="+d.functionDesc(fns, depth+1))
	}
	if st, ok := d.resolve(v).(pdfStream); ok {
		if data, err := decodeStream(st); err == nil {
			parts = append(parts, "Samples="+hex.EncodeToString(data))
		}
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func (d *pdfDoc) patternDesc(v any) string {
	p := d.dict(v)
	if p == nil {
		return "<invalid pattern>"
	}
	// A pattern's /Matrix maps pattern space onto the page's default space. It is
	// composed into the geometry it governs rather than compared on its own,
	// because upstream folds the page coordinate flip into it while the port
	// pre-flips the coordinates and leaves the matrix at identity: the same
	// gradient, expressed two ways.
	m := identity
	if a := d.arr(p["Matrix"]); len(a) == 6 {
		for i := range a {
			m[i], _ = d.num(a[i])
		}
	}
	parts := []string{}
	for _, k := range []string{"PatternType", "PaintType", "TilingType"} {
		if val, ok := p[k]; ok {
			parts = append(parts, k+"="+describeAny(d, val, 0))
		}
	}
	if bb := d.arr(p["BBox"]); len(bb) == 4 {
		vals := make([]float64, 4)
		for i := range bb {
			vals[i], _ = d.num(bb[i])
		}
		x0, y0 := m.apply(vals[0], vals[1])
		x1, y1 := m.apply(vals[2], vals[3])
		parts = append(parts, "BBoxInPageSpace=["+fnums(math.Min(x0, x1), math.Min(y0, y1), math.Max(x0, x1), math.Max(y0, y1))+"]")
	}
	if xs, ok := d.num(p["XStep"]); ok {
		parts = append(parts, "XStepInPageSpace="+fnum(xs*math.Hypot(m[0], m[1])))
	}
	if ys, ok := d.num(p["YStep"]); ok {
		parts = append(parts, "YStepInPageSpace="+fnum(ys*math.Hypot(m[2], m[3])))
	}
	if sh, ok := p["Shading"]; ok {
		parts = append(parts, "Shading{"+d.shadingDescIn(sh, m)+"}")
	}
	if st, ok := d.resolve(v).(pdfStream); ok {
		if data, err := decodeStream(st); err == nil {
			parts = append(parts, "Cell["+strings.Join(traceStream(data, newNameMap()), "; ")+"]")
		}
	}
	return strings.Join(parts, " ")
}

// describeAny renders an arbitrary PDF value compactly, with numbers rounded.
func describeAny(d *pdfDoc, v any, depth int) string {
	if depth > 12 {
		return "<deep>"
	}
	switch t := d.resolve(v).(type) {
	case nil:
		return "<nil>"
	case pdfNull:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return fnum(t)
	case pdfName:
		return "/" + string(t)
	case pdfStr:
		return "(" + decodeTextString(t) + ")"
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, describeAny(d, x, depth+1))
		}
		return "[" + strings.Join(parts, " ") + "]"
	case pdfDict:
		var parts []string
		for _, k := range sortedKeys(t) {
			parts = append(parts, k+"="+describeAny(d, t[k], depth+1))
		}
		return "<<" + strings.Join(parts, " ") + ">>"
	case pdfStream:
		var parts []string
		for _, k := range sortedKeys(t.Dict) {
			parts = append(parts, k+"="+describeAny(d, t.Dict[k], depth+1))
		}
		return "stream<<" + strings.Join(parts, " ") + ">>"
	}
	return fmt.Sprintf("%v", v)
}

// ---------------------------------------------------------------------------
// content-stream trace

// nameMap translates the arbitrary resource names each generator invents
// (/F1 vs /F0, /I1 vs /Im0) into a description of the thing referenced, so a
// trace compares what was drawn rather than what it happened to be called.
type nameMap struct {
	font, xobj, extg, shading, pattern, cspace map[string]string
}

func newNameMap() *nameMap {
	return &nameMap{
		font: map[string]string{}, xobj: map[string]string{}, extg: map[string]string{},
		shading: map[string]string{}, pattern: map[string]string{}, cspace: map[string]string{},
	}
}

func (n *nameMap) lookup(kind map[string]string, name string) string {
	if v, ok := kind[name]; ok {
		return "{" + v + "}"
	}
	return "/" + name + "<unresolved>"
}

// traceStream tokenises a decoded content stream into a normalised operator
// trace. The normalisations, all of which preserve drawing behaviour:
//
//   - numeric operands are rounded (see the precision note at the top);
//   - path and clipping coordinates are reported in *device* space, i.e. with
//     the current transformation matrix applied, so the same drawing reached
//     through a different sequence of transforms compares equal;
//   - "re" is expanded to its four transformed corners, because a rectangle is
//     not axis-aligned once a rotation or skew is in effect;
//   - text-positioning operators (Td, TD, Tm, T*) are folded into the
//     text-showing operator, which reports the full device-space text rendering
//     matrix; Td and Tm are interchangeable ways of reaching the same matrix;
//   - Tj, TJ, ' and " are all reported as SHOW with the shown bytes as hex,
//     dropping zero-valued TJ adjustments (a no-op) and keeping non-zero ones;
//   - resource names are replaced by a description of the resource.
func traceStream(content []byte, names *nameMap) []string {
	var out []string
	l := &lexer{buf: content}

	ctm := identity
	var ctmStack []mat
	// text state
	tm, tlm := identity, identity
	leading := 0.0
	inText := false
	var pending []any // operand stack

	push := func(v any) { pending = append(pending, v) }
	numAt := func(i int) float64 {
		if i < 0 || i >= len(pending) {
			return 0
		}
		if f, ok := pending[i].(float64); ok {
			return f
		}
		return 0
	}
	nums := func() []float64 {
		out := make([]float64, 0, len(pending))
		for _, p := range pending {
			if f, ok := p.(float64); ok {
				out = append(out, f)
			}
		}
		return out
	}
	nameAt := func(i int) string {
		if i < 0 || i >= len(pending) {
			return ""
		}
		if n, ok := pending[i].(pdfName); ok {
			return string(n)
		}
		return ""
	}
	dev := func(x, y float64) string {
		dx, dy := ctm.apply(x, y)
		return fnums(dx, dy)
	}
	emit := func(s string) { out = append(out, s) }

	for {
		l.skipWS()
		if l.pos >= len(content) {
			break
		}
		c := content[l.pos]
		if c == '/' || c == '[' || c == '<' || c == '(' || c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
			push(l.object())
			continue
		}
		op := l.token()
		if op == "" {
			break
		}
		switch op {
		case "true":
			push(true)
			continue
		case "false":
			push(false)
			continue
		case "null":
			push(pdfNull{})
			continue
		case "q":
			ctmStack = append(ctmStack, ctm)
			emit("q")
		case "Q":
			if n := len(ctmStack); n > 0 {
				ctm = ctmStack[n-1]
				ctmStack = ctmStack[:n-1]
			} else {
				emit("<unbalanced Q>")
			}
			emit("Q")
		case "cm":
			v := nums()
			if len(v) == 6 {
				m := mat{v[0], v[1], v[2], v[3], v[4], v[5]}
				ctm = mul(m, ctm)
				emit("cm " + fnums(v...))
			} else {
				emit("<bad cm>")
			}
		case "m", "l":
			v := nums()
			if len(v) >= 2 {
				emit(op + " " + dev(v[0], v[1]))
			} else {
				emit("<bad " + op + ">")
			}
		case "c":
			v := nums()
			if len(v) >= 6 {
				emit("c " + dev(v[0], v[1]) + " " + dev(v[2], v[3]) + " " + dev(v[4], v[5]))
			} else {
				emit("<bad c>")
			}
		case "v", "y":
			v := nums()
			if len(v) >= 4 {
				emit(op + " " + dev(v[0], v[1]) + " " + dev(v[2], v[3]))
			} else {
				emit("<bad " + op + ">")
			}
		case "re":
			v := nums()
			if len(v) >= 4 {
				x, y, w, h := v[0], v[1], v[2], v[3]
				emit("re " + dev(x, y) + " " + dev(x+w, y) + " " + dev(x+w, y+h) + " " + dev(x, y+h))
			} else {
				emit("<bad re>")
			}
		case "h", "n", "f", "F", "f*", "S", "s", "B", "B*", "b", "b*", "W", "W*":
			if op == "F" {
				op = "f"
			}
			emit(op)
		case "BT":
			inText = true
			tm, tlm = identity, identity
			emit("BT")
		case "ET":
			inText = false
			emit("ET")
		case "Tf":
			emit("Tf " + names.lookup(names.font, nameAt(0)) + " " + fnum(numAt(1)))
		case "Td":
			v := nums()
			if len(v) >= 2 {
				tlm = mul(mat{1, 0, 0, 1, v[0], v[1]}, tlm)
				tm = tlm
			}
		case "TD":
			v := nums()
			if len(v) >= 2 {
				leading = -v[1]
				tlm = mul(mat{1, 0, 0, 1, v[0], v[1]}, tlm)
				tm = tlm
			}
		case "Tm":
			v := nums()
			if len(v) >= 6 {
				tlm = mat{v[0], v[1], v[2], v[3], v[4], v[5]}
				tm = tlm
			}
		case "T*":
			tlm = mul(mat{1, 0, 0, 1, 0, -leading}, tlm)
			tm = tlm
		case "TL":
			leading = numAt(0)
			emit("TL " + fnum(leading))
		case "Tc", "Tw", "Tz", "Ts", "Tr":
			emit(op + " " + fnum(numAt(0)))
		case "Tj", "TJ", "'", "\"":
			if op == "'" || op == "\"" {
				tlm = mul(mat{1, 0, 0, 1, 0, -leading}, tlm)
				tm = tlm
			}
			var shown []byte
			var adj []float64
			src := pending
			if op == "\"" && len(pending) >= 3 {
				emit("Tw " + fnum(numAt(0)))
				emit("Tc " + fnum(numAt(1)))
				src = pending[2:]
			}
			for _, p := range src {
				switch t := p.(type) {
				case pdfStr:
					shown = append(shown, t...)
				case []any:
					for _, e := range t {
						switch et := e.(type) {
						case pdfStr:
							shown = append(shown, et...)
						case float64:
							if et != 0 {
								adj = append(adj, et)
							}
						}
					}
				}
			}
			trm := mul(tm, ctm)
			s := "SHOW trm=" + fnums(trm[0], trm[1], trm[2], trm[3], trm[4], trm[5]) + " bytes=" + hex.EncodeToString(shown)
			if len(adj) > 0 {
				s += " adj=" + fnums(adj...)
			}
			if !inText {
				s += " <outside BT/ET>"
			}
			emit(s)
		case "rg", "RG":
			v := nums()
			emit(op + " " + fnums(v...))
		case "g", "G":
			emit(op + " " + fnum(numAt(0)))
		case "k", "K":
			v := nums()
			emit(op + " " + fnums(v...))
		case "cs", "CS":
			n := nameAt(0)
			if desc, ok := names.cspace[n]; ok {
				emit(op + " {" + desc + "}")
			} else {
				emit(op + " /" + n)
			}
		case "sc", "SC", "scn", "SCN":
			v := nums()
			s := op
			if len(v) > 0 {
				s += " " + fnums(v...)
			}
			if n := nameAt(len(pending) - 1); n != "" {
				s += " " + names.lookup(names.pattern, n)
			}
			emit(s)
		case "w":
			emit("w " + fnum(numAt(0)))
		case "J", "j":
			emit(op + " " + fnum(numAt(0)))
		case "M":
			emit("M " + fnum(numAt(0)))
		case "d":
			var pat []float64
			if len(pending) > 0 {
				if a, ok := pending[0].([]any); ok {
					for _, x := range a {
						if f, ok := x.(float64); ok {
							pat = append(pat, f)
						}
					}
				}
			}
			emit("d [" + fnums(pat...) + "] " + fnum(numAt(1)))
		case "i", "ri":
			emit(op + " " + fnum(numAt(0)))
		case "gs":
			emit("gs " + names.lookup(names.extg, nameAt(0)))
		case "Do":
			// The device matrix is what actually places the XObject, so it is
			// reported alongside the resource.
			emit("Do " + names.lookup(names.xobj, nameAt(0)) +
				" ctm=" + fnums(ctm[0], ctm[1], ctm[2], ctm[3], ctm[4], ctm[5]))
		case "sh":
			emit("sh " + names.lookup(names.shading, nameAt(0)) +
				" ctm=" + fnums(ctm[0], ctm[1], ctm[2], ctm[3], ctm[4], ctm[5]))
		case "BMC", "EMC", "BDC", "BX", "EX", "MP", "DP", "d0", "d1":
			emit(op)
		case "BI":
			// inline image: skip to EI, recording only that one was present
			idx := bytes.Index(content[l.pos:], []byte("EI"))
			if idx < 0 {
				l.pos = len(content)
			} else {
				l.pos += idx + 2
			}
			emit("BI<inline image>")
		default:
			emit("<unknown operator " + op + ">")
		}
		pending = pending[:0]
	}
	if len(ctmStack) != 0 {
		out = append(out, fmt.Sprintf("<%d unbalanced q>", len(ctmStack)))
	}
	if inText {
		out = append(out, "<unterminated BT>")
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// ---------------------------------------------------------------------------
// systematic-divergence masking

var reColorOp = regexp.MustCompile(`^(cs|CS) \{?/?(Device(?:RGB|Gray|CMYK))\}?$`)

// foldColorOps rewrites the "select a device colour space, then set a colour
// with scn" spelling into the equivalent single-operator spelling (rg/g/k),
// because "/DeviceRGB cs 1 0 0 scn" and "1 0 0 rg" are the same instruction.
func foldColorOps(trace []string) []string {
	out := make([]string, 0, len(trace))
	fillCS, strokeCS := "", ""
	for _, t := range trace {
		if m := reColorOp.FindStringSubmatch(t); m != nil {
			if m[1] == "cs" {
				fillCS = m[2]
			} else {
				strokeCS = m[2]
			}
			continue // the cs/CS operator itself carries no drawing effect
		}
		fields := strings.Fields(t)
		if len(fields) >= 2 && (fields[0] == "scn" || fields[0] == "SCN") {
			cs := fillCS
			target := map[string]string{"DeviceRGB": "rg", "DeviceGray": "g", "DeviceCMYK": "k"}
			if fields[0] == "SCN" {
				cs = strokeCS
				target = map[string]string{"DeviceRGB": "RG", "DeviceGray": "G", "DeviceCMYK": "K"}
			}
			if op, ok := target[cs]; ok && !strings.Contains(t, "{") {
				out = append(out, op+" "+strings.Join(fields[1:], " "))
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// dropBookkeeping removes the q, Q and cm operators from a trace.
func dropBookkeeping(trace []string) []string {
	out := make([]string, 0, len(trace))
	for _, t := range trace {
		if t == "q" || t == "Q" || strings.HasPrefix(t, "cm ") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// StripSystematic masks the port's systematic divergences from upstream, so a
// second score can report structural agreement independently of them. Each mask
// is a difference that is uniform across every case and is documented in
// COVERAGE.md:
//
//  1. the %PDF header version (upstream writes 1.3, the port always 1.7);
//  2. /Info CreationDate and ModDate (the port has no API for either);
//  3. the trailer /ID (upstream always writes one, the port only when encrypting);
//  4. the /ProcSet resource array (upstream writes one, the port never does);
//  5. the colour-operator spelling (see foldColorOps);
//  6. the q / Q / cm bookkeeping operators. Neither marks the page: their only
//     effect is on the coordinates of later operators, which the trace already
//     reports in device space (and the device matrix is attached to Do and sh
//     directly). They differ systematically because the port has no flipped
//     user space, so the runner has to re-apply pdfkit's page flip around every
//     text and image call, adding one q/cm/Q nesting level that upstream does
//     not need.
func StripSystematic(c *canonDoc) *canonDoc {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Version = "<masked>"
	cp.IDPresent = false
	var info []string
	for _, e := range c.Info {
		if strings.HasPrefix(e, "CreationDate=") || strings.HasPrefix(e, "ModDate=") {
			continue
		}
		info = append(info, e)
	}
	cp.Info = info
	cp.Pages = make([]canonPage, len(c.Pages))
	for i, p := range c.Pages {
		q := p
		q.ProcSet = nil
		q.Trace = dropBookkeeping(foldColorOps(p.Trace))
		cp.Pages[i] = q
	}
	return &cp
}
