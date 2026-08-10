// Command run is the Go-side parity runner for github.com/malcolmston/nodemailer.
//
// It reads one JSON object per line on stdin and writes exactly one JSON object
// per line on stdout, in the same order. It never exits on a failing case: a
// build error (or a panic) is reported as {"ok": false, "error": ...}.
//
// The comparable artefact is the generated MIME message, obtained from
// Message.Build. Nothing is ever sent over the network.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/malcolmston/nodemailer"
)

// Pinned so the output is byte-stable: the encoder would otherwise stamp the
// current time, a random Message-ID and a random multipart boundary.
var (
	fixedDate      = time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	fixedMessageID = "parity-fixed@example.com"
	fixedBoundary  = "PARITYBOUNDARY"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value string `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// attachmentSpec is the language-neutral attachment shape used in cases/.
type attachmentSpec struct {
	Filename                *string `json:"filename"`
	ContentType             string  `json:"contentType"`
	CID                     string  `json:"cid"`
	ContentDisposition      string  `json:"contentDisposition"`
	ContentTransferEncoding string  `json:"contentTransferEncoding"`
	Mode                    string  `json:"mode"`
	Content                 string  `json:"content"`
	Path                    string  `json:"path"`
}

type alternativeSpec struct {
	ContentType             string `json:"contentType"`
	Content                 string `json:"content"`
	ContentTransferEncoding string `json:"contentTransferEncoding"`
}

type headerSpec struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type icalSpec struct {
	Method   string `json:"method"`
	Content  string `json:"content"`
	Filename string `json:"filename"`
}

// listSpec mirrors the nodemailer `list` option in the reduced, order-stable
// form the case files use.
type listSpec struct {
	Unsubscribe     []string     `json:"unsubscribe"`
	UnsubscribePost bool         `json:"unsubscribePost"`
	Headers         []headerSpec `json:"headers"`
}

type mailSpec struct {
	From         string            `json:"from"`
	To           []string          `json:"to"`
	Cc           []string          `json:"cc"`
	Bcc          []string          `json:"bcc"`
	ReplyTo      []string          `json:"replyTo"`
	Subject      *string           `json:"subject"`
	Text         *string           `json:"text"`
	HTML         *string           `json:"html"`
	WatchHTML    *string           `json:"watchHtml"`
	AMP          *string           `json:"amp"`
	Alternatives []alternativeSpec `json:"alternatives"`
	ICalEvent    *icalSpec         `json:"icalEvent"`
	Headers      []headerSpec      `json:"headers"`
	List         *listSpec         `json:"list"`
	Priority     string            `json:"priority"`
	InReplyTo    string            `json:"inReplyTo"`
	References   []string          `json:"references"`
	Attachments  []attachmentSpec  `json:"attachments"`
	Date         string            `json:"date"`
	MessageID    *string           `json:"messageId"`
}

var root = func() string {
	if r := os.Getenv("PARITY_ROOT"); r != "" {
		return r
	}
	wd, _ := os.Getwd()
	return wd
}()

var schemeRe = regexp.MustCompile(`^[a-zA-Z]+://`)

func resolveFixture(p string) (string, error) {
	if schemeRe.MatchString(p) {
		return "", fmt.Errorf("remote attachment paths are not allowed in the parity harness")
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(root, p), nil
}

// stripAngles removes the angle brackets nodemailer's messageId option carries,
// because the Go API takes a bare id.
func stripAngles(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

// isGroup reports whether an address string is an RFC 5322 group construct
// ("Name: a@x, b@y;"). The Go port takes groups through a dedicated setter.
func isGroup(s string) (name string, members []string, ok bool) {
	t := strings.TrimSpace(s)
	if !strings.HasSuffix(t, ";") {
		return "", nil, false
	}
	i := strings.Index(t, ":")
	if i < 0 {
		return "", nil, false
	}
	// A colon before any '@' and a trailing ';' is a group.
	if at := strings.Index(t, "@"); at >= 0 && at < i {
		return "", nil, false
	}
	name = strings.TrimSpace(t[:i])
	body := strings.TrimSpace(strings.TrimSuffix(t[i+1:], ";"))
	if body != "" {
		for _, part := range splitAddressList(body) {
			members = append(members, part)
		}
	}
	return name, members, true
}

// splitAddressList splits on commas that are not inside quotes or angles.
func splitAddressList(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, depth := false, 0
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == '<' && !inQuote:
			depth++
			cur.WriteRune(r)
		case r == '>' && !inQuote:
			depth--
			cur.WriteRune(r)
		case r == ',' && !inQuote && depth == 0:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

func buildMessage(spec mailSpec) (string, error) {
	m := nodemailer.New()

	// Determinism.
	if spec.Date != "" {
		t, err := time.Parse(time.RFC3339, spec.Date)
		if err != nil {
			return "", fmt.Errorf("bad date: %w", err)
		}
		m.SetDate(t.UTC())
	} else {
		m.SetDate(fixedDate)
	}
	if spec.MessageID != nil {
		m.SetMessageID(stripAngles(*spec.MessageID))
	} else {
		m.SetMessageID(fixedMessageID)
	}
	m.SetBoundary(fixedBoundary)

	if spec.From != "" {
		m.SetFrom(spec.From)
	}

	addAddrs := func(addrs []string, flat func(...string) *nodemailer.Message, group func(string, ...string) *nodemailer.Message) {
		for _, a := range addrs {
			if name, members, ok := isGroup(a); ok {
				if group != nil {
					group(name, members...)
					continue
				}
			}
			flat(a)
		}
	}
	addAddrs(spec.To, m.AddTo, m.AddToGroup)
	addAddrs(spec.Cc, m.AddCc, m.AddCcGroup)
	addAddrs(spec.Bcc, m.AddBcc, nil)
	addAddrs(spec.ReplyTo, m.AddReplyTo, nil)

	if spec.Subject != nil {
		m.SetSubject(*spec.Subject)
	}
	if spec.Text != nil {
		m.SetText(*spec.Text)
	}
	// Order mirrors upstream's alternative ordering: text, watch-html, amp,
	// html, ical, then explicit alternatives.
	if spec.WatchHTML != nil {
		m.SetWatchHTML(*spec.WatchHTML)
	}
	if spec.AMP != nil {
		m.SetAMP(*spec.AMP)
	}
	if spec.HTML != nil {
		m.SetHTML(*spec.HTML)
	}
	if spec.ICalEvent != nil {
		m.ICalEvent = &nodemailer.ICalEvent{
			Method:   spec.ICalEvent.Method,
			Content:  spec.ICalEvent.Content,
			Filename: spec.ICalEvent.Filename,
		}
	}
	for _, alt := range spec.Alternatives {
		m.AddAlternative(alt.ContentType, alt.Content)
	}

	for _, h := range spec.Headers {
		m.AddHeader(h.Key, h.Value)
	}
	if spec.List != nil {
		if len(spec.List.Unsubscribe) > 0 {
			m.SetListUnsubscribe(spec.List.Unsubscribe...)
		}
		if spec.List.UnsubscribePost {
			m.SetListUnsubscribePost()
		}
		for _, h := range spec.List.Headers {
			m.AddListHeader(h.Key, h.Value)
		}
	}
	switch strings.ToLower(spec.Priority) {
	case "", "normal":
	case "high":
		m.SetPriority(nodemailer.PriorityHigh)
	case "low":
		m.SetPriority(nodemailer.PriorityLow)
	default:
		return "", fmt.Errorf("unknown priority: %q", spec.Priority)
	}
	if spec.InReplyTo != "" {
		m.SetInReplyTo(stripAngles(spec.InReplyTo))
	}
	if len(spec.References) > 0 {
		refs := make([]string, 0, len(spec.References))
		for _, r := range spec.References {
			refs = append(refs, stripAngles(r))
		}
		m.AddReferences(refs...)
	}

	for _, a := range spec.Attachments {
		if err := attach(m, a); err != nil {
			return "", err
		}
	}

	raw, err := m.Build()
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func attach(m *nodemailer.Message, a attachmentSpec) error {
	filename := ""
	if a.Filename != nil {
		filename = *a.Filename
	}
	inline := a.CID != "" || strings.EqualFold(a.ContentDisposition, "inline")

	mode := a.Mode
	if mode == "" {
		mode = "string"
	}

	var content []byte
	switch mode {
	case "string":
		content = []byte(a.Content)
	case "buffer", "base64":
		b, err := base64.StdEncoding.DecodeString(a.Content)
		if err != nil {
			return fmt.Errorf("bad base64 attachment content: %w", err)
		}
		content = b
	case "path":
		p, err := resolveFixture(a.Path)
		if err != nil {
			return err
		}
		if a.CID != "" {
			m.EmbedFile(a.CID, p)
		} else {
			m.AttachFile(p)
		}
		// Honour explicit overrides that the *File helpers cannot express.
		if n := len(m.Attachments); n > 0 {
			if filename != "" {
				m.Attachments[n-1].Filename = filename
			}
			if a.ContentType != "" {
				m.Attachments[n-1].ContentType = a.ContentType
			}
			if inline {
				m.Attachments[n-1].Inline = true
			}
		}
		return nil
	case "stream":
		p, err := resolveFixture(a.Path)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		if filename == "" {
			filename = filepath.Base(p)
		}
		if a.CID != "" {
			m.EmbedReader(a.CID, filename, a.ContentType, f)
		} else {
			m.AttachReader(filename, a.ContentType, f)
		}
		if inline && a.CID == "" {
			if n := len(m.Attachments); n > 0 {
				m.Attachments[n-1].Inline = true
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown attachment mode: %q", mode)
	}

	m.Attach(nodemailer.Attachment{
		Filename:    filename,
		Content:     content,
		ContentType: a.ContentType,
		ContentID:   a.CID,
		Inline:      inline,
	})
	return nil
}

func handle(req request) (value string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if req.Fn != "buildMessage" {
		return "", fmt.Errorf("unknown fn: %q", req.Fn)
	}
	if len(req.Args) == 0 {
		return "", fmt.Errorf("missing mail options argument")
	}
	var spec mailSpec
	if err := json.Unmarshal(req.Args[0], &spec); err != nil {
		return "", fmt.Errorf("bad mail options: %w", err)
	}
	return buildMessage(spec)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{ID: "<unparseable>", OK: false, Error: "bad request line: " + err.Error()})
			out.Flush()
			continue
		}
		value, err := handle(req)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: value})
		}
		out.Flush()
	}
	out.Flush()
}
