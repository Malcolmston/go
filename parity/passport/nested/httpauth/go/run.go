// Command run is the Go half of parity/passport/nested/httpauth: the same JSON
// Lines protocol as node/run.js, answered by
// github.com/malcolmston/passport/httpauth.
//
// Conventions shared with the upstream runner: byte arguments are lowercase hex
// ("bodyHex"), Digest algorithms use their RFC 7616 spelling, and parsed Digest
// credentials are projected onto a fixed field set so an absent parameter and an
// empty one compare equal. A failing case is reported as {ok:false,error} and
// never terminates the process.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/passport/httpauth"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	// Value carries the answer for a successful case. It deliberately has no
	// omitempty: a legitimate answer may be false or "", which omitempty would
	// erase.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// digestArgs mirrors the object form the case files use for the digest
// operations.
type digestArgs struct {
	Method    string `json:"method"`
	URI       string `json:"uri"`
	Username  string `json:"username"`
	Realm     string `json:"realm"`
	Password  string `json:"password"`
	Nonce     string `json:"nonce"`
	NC        string `json:"nc"`
	CNonce    string `json:"cnonce"`
	QOP       string `json:"qop"`
	Algorithm string `json:"algorithm"`
	BodyHex   string `json:"bodyHex"`
}

type challengeArgs struct {
	Realm     string `json:"realm"`
	Nonce     string `json:"nonce"`
	Domain    string `json:"domain"`
	Opaque    string `json:"opaque"`
	Stale     bool   `json:"stale"`
	Algorithm string `json:"algorithm"`
	QOP       string `json:"qop"`
	Charset   string `json:"charset"`
	Userhash  bool   `json:"userhash"`
}

func str(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string argument: %w", err)
	}
	return s, nil
}

func args(a []json.RawMessage, n int) ([]string, error) {
	if len(a) < n {
		return nil, fmt.Errorf("expected %d arguments, got %d", n, len(a))
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		s, err := str(a[i])
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func decodeBody(bodyHex string) ([]byte, error) {
	if bodyHex == "" {
		return nil, nil
	}
	b, err := hex.DecodeString(bodyHex)
	if err != nil {
		return nil, fmt.Errorf("bodyHex is not hex: %w", err)
	}
	return b, nil
}

func normaliseDigest(c httpauth.DigestCredentials) map[string]any {
	return map[string]any{
		"username":  c.Username,
		"realm":     c.Realm,
		"nonce":     c.Nonce,
		"uri":       c.URI,
		"response":  c.Response,
		"algorithm": c.Algorithm,
		"cnonce":    c.CNonce,
		"opaque":    c.Opaque,
		"qop":       c.QOP,
		"nc":        c.NC,
	}
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "schemeOf":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return httpauth.SchemeOf(a[0]), nil

	case "hasScheme":
		a, err := args(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return httpauth.HasScheme(a[0], a[1]), nil

	case "encodeBasic":
		a, err := args(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return httpauth.EncodeBasic(a[0], a[1]), nil

	case "parseBasic":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		c, err := httpauth.ParseBasic(a[0])
		if err != nil {
			return nil, err
		}
		return map[string]any{"username": c.Username, "password": c.Password}, nil

	case "basicMatches":
		a, err := args(req.Args, 3)
		if err != nil {
			return nil, err
		}
		c, err := httpauth.ParseBasic(a[0])
		if err != nil {
			return nil, err
		}
		return c.Matches(a[1], a[2]), nil

	case "basicChallenge":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return httpauth.BasicChallenge(a[0]), nil

	case "encodeBearer":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return httpauth.EncodeBearer(a[0]), nil

	case "parseBearer":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return httpauth.ParseBearer(a[0])

	case "bearerChallenge":
		a, err := args(req.Args, 2)
		if err != nil {
			return nil, err
		}
		return httpauth.BearerChallenge(a[0], a[1]), nil

	case "parseDigest":
		a, err := args(req.Args, 1)
		if err != nil {
			return nil, err
		}
		c, err := httpauth.ParseDigest(a[0])
		if err != nil {
			return nil, err
		}
		return normaliseDigest(c), nil

	case "digestChallenge":
		if len(req.Args) < 1 {
			return nil, fmt.Errorf("expected 1 argument, got %d", len(req.Args))
		}
		var p challengeArgs
		if err := json.Unmarshal(req.Args[0], &p); err != nil {
			return nil, err
		}
		return httpauth.DigestChallenge(httpauth.DigestChallengeParams{
			Realm:     p.Realm,
			Nonce:     p.Nonce,
			Opaque:    p.Opaque,
			Algorithm: p.Algorithm,
			QOP:       p.QOP,
			Domain:    p.Domain,
			Stale:     p.Stale,
			Charset:   p.Charset,
			Userhash:  p.Userhash,
		}), nil

	case "digestResponse":
		if len(req.Args) < 1 {
			return nil, fmt.Errorf("expected 1 argument, got %d", len(req.Args))
		}
		var p digestArgs
		if err := json.Unmarshal(req.Args[0], &p); err != nil {
			return nil, err
		}
		body, err := decodeBody(p.BodyHex)
		if err != nil {
			return nil, err
		}
		return httpauth.DigestResponse(httpauth.DigestParams{
			Method:    p.Method,
			URI:       p.URI,
			Username:  p.Username,
			Realm:     p.Realm,
			Password:  p.Password,
			Nonce:     p.Nonce,
			NC:        p.NC,
			CNonce:    p.CNonce,
			QOP:       p.QOP,
			Algorithm: p.Algorithm,
			Body:      body,
		})

	case "verifyDigest":
		a, err := args(req.Args, 4)
		if err != nil {
			return nil, err
		}
		c, err := httpauth.ParseDigest(a[0])
		if err != nil {
			return nil, err
		}
		body, err := decodeBody(a[3])
		if err != nil {
			return nil, err
		}
		return httpauth.VerifyDigest(c, a[1], a[2], body), nil

	default:
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := os.Stdout
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := dispatch(req)
		if err != nil {
			emit(out, response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(out, response{ID: req.ID, OK: true, Value: v})
	}
}

func emit(f *os.File, r response) {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{ID: r.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Sync()
}
