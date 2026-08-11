// Command run is the Go side of the express/jsonwebtoken parity harness. It
// speaks the same JSON Lines protocol as node/run.js and never exits on a
// failing case.
//
// Determinism: verify cases carry clockTimestamp (mapped onto
// VerifyOptions.ClockTimestamp) and sign cases carry an explicit iat, so no
// reply depends on the wall clock.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/malcolmston/express/jsonwebtoken"
)

var fixtures = "fixtures"

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string      `json:"id"`
	OK    bool        `json:"ok"`
	Value interface{} `json:"value,omitempty"`
	Error string      `json:"error,omitempty"`
}

// keySpec mirrors the case-file key description shared with the Node runner.
type keySpec struct {
	Kind   string  `json:"kind"`
	Secret *string `json:"secret"`
	File   *string `json:"file"`
}

func resolveKey(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s keySpec
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("bad key spec %s: %v", raw, err)
	}
	switch s.Kind {
	case "none":
		return nil, nil
	case "oct":
		if s.File != nil {
			name := *s.File
			if strings.Contains(name, "..") || filepath.IsAbs(name) {
				return nil, fmt.Errorf("bad fixture: %s", name)
			}
			return os.ReadFile(filepath.Join(fixtures, name))
		}
		if s.Secret == nil {
			return nil, nil
		}
		return []byte(*s.Secret), nil
	default:
		return nil, fmt.Errorf("unknown key kind: %q", s.Kind)
	}
}

// signOpts is the Node sign-option shape, translated onto SignOptions.
type signOpts struct {
	Algorithm   *string                `json:"algorithm"`
	ExpiresIn   *int64                 `json:"expiresIn"`
	NotBefore   *int64                 `json:"notBefore"`
	Audience    json.RawMessage        `json:"audience"`
	Issuer      *string                `json:"issuer"`
	Subject     *string                `json:"subject"`
	JwtID       *string                `json:"jwtid"`
	KeyID       *string                `json:"keyid"`
	NoTimestamp *bool                  `json:"noTimestamp"`
	Header      map[string]interface{} `json:"header"`
}

// strList accepts either a JSON string or an array of strings, the two shapes
// the Node audience and issuer options take.
func strList(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, fmt.Errorf("expected a string or an array of strings, got %s", raw)
	}
	// A non-nil empty slice must stay non-nil: an empty allowlist permits
	// nothing, and the port distinguishes that from "option absent".
	if many == nil {
		many = []string{}
	}
	return many, nil
}

func toSignOptions(raw json.RawMessage) (*jsonwebtoken.SignOptions, error) {
	var o signOpts
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, fmt.Errorf("bad sign options %s: %v", raw, err)
		}
	}
	out := &jsonwebtoken.SignOptions{Header: o.Header}
	if o.Algorithm != nil {
		out.Alg = *o.Algorithm
	}
	if o.ExpiresIn != nil {
		out.ExpiresIn = time.Duration(*o.ExpiresIn) * time.Second
	}
	if o.NotBefore != nil {
		out.NotBefore = time.Duration(*o.NotBefore) * time.Second
	}
	aud, err := strList(o.Audience)
	if err != nil {
		return nil, err
	}
	out.Audience = aud
	if o.Issuer != nil {
		out.Issuer = *o.Issuer
	}
	if o.Subject != nil {
		out.Subject = *o.Subject
	}
	if o.JwtID != nil {
		out.JwtID = *o.JwtID
	}
	if o.KeyID != nil {
		out.KeyID = *o.KeyID
	}
	if o.NoTimestamp != nil {
		out.NoTimestamp = *o.NoTimestamp
	}
	return out, nil
}

// verifyOpts is the Node verify-option shape, translated onto VerifyOptions.
type verifyOpts struct {
	Algorithms       json.RawMessage `json:"algorithms"`
	Audience         *string         `json:"audience"`
	Issuer           json.RawMessage `json:"issuer"`
	Subject          *string         `json:"subject"`
	JwtID            *string         `json:"jwtid"`
	ClockTolerance   *int64          `json:"clockTolerance"`
	ClockTimestamp   *int64          `json:"clockTimestamp"`
	MaxAge           *int64          `json:"maxAge"`
	IgnoreExpiration *bool           `json:"ignoreExpiration"`
	IgnoreNotBefore  *bool           `json:"ignoreNotBefore"`
	Complete         bool            `json:"complete"`
}

func toVerifyOptions(raw json.RawMessage) (*jsonwebtoken.VerifyOptions, bool, error) {
	var o verifyOpts
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, false, fmt.Errorf("bad verify options %s: %v", raw, err)
		}
	}
	out := &jsonwebtoken.VerifyOptions{}
	algs, err := strList(o.Algorithms)
	if err != nil {
		return nil, false, err
	}
	out.Algorithms = algs
	if o.Audience != nil {
		out.Audience = *o.Audience
	}
	iss, err := strList(o.Issuer)
	if err != nil {
		return nil, false, err
	}
	out.Issuer = iss
	if o.Subject != nil {
		out.Subject = *o.Subject
	}
	if o.JwtID != nil {
		out.JwtID = *o.JwtID
	}
	if o.ClockTolerance != nil {
		out.ClockTolerance = time.Duration(*o.ClockTolerance) * time.Second
	}
	if o.ClockTimestamp != nil {
		out.ClockTimestamp = time.Unix(*o.ClockTimestamp, 0).UTC()
	}
	if o.MaxAge != nil {
		out.MaxAge = time.Duration(*o.MaxAge) * time.Second
	}
	if o.IgnoreExpiration != nil {
		out.IgnoreExpiration = *o.IgnoreExpiration
	}
	if o.IgnoreNotBefore != nil {
		out.IgnoreNotBefore = *o.IgnoreNotBefore
	}
	return out, o.Complete, nil
}

func str(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string argument, got %s", raw)
	}
	return s, nil
}

func call(fn string, args []json.RawMessage) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	switch fn {
	case "sign":
		if len(args) < 2 {
			return nil, fmt.Errorf("sign needs a payload and a key")
		}
		var claims jsonwebtoken.Claims
		if err := json.Unmarshal(args[0], &claims); err != nil {
			return nil, fmt.Errorf("bad payload %s: %v", args[0], err)
		}
		key, err := resolveKey(args[1])
		if err != nil {
			return nil, err
		}
		var rawOpts json.RawMessage
		if len(args) > 2 {
			rawOpts = args[2]
		}
		opts, err := toSignOptions(rawOpts)
		if err != nil {
			return nil, err
		}
		token, err := jsonwebtoken.Sign(claims, key, opts)
		if err != nil {
			return nil, err
		}
		return token, nil
	case "verify":
		if len(args) < 2 {
			return nil, fmt.Errorf("verify needs a token and a key")
		}
		token, err := str(args[0])
		if err != nil {
			return nil, err
		}
		key, err := resolveKey(args[1])
		if err != nil {
			return nil, err
		}
		var rawOpts json.RawMessage
		if len(args) > 2 {
			rawOpts = args[2]
		}
		opts, complete, err := toVerifyOptions(rawOpts)
		if err != nil {
			return nil, err
		}
		claims, err := jsonwebtoken.Verify(token, key, opts)
		if err != nil {
			return nil, err
		}
		if !complete {
			return claims, nil
		}
		full, err := jsonwebtoken.DecodeComplete(token)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"header":    full.Header,
			"payload":   claims,
			"signature": full.Signature,
		}, nil
	case "decode":
		if len(args) < 1 {
			return nil, fmt.Errorf("decode needs a token")
		}
		token, err := str(args[0])
		if err != nil {
			return nil, err
		}
		complete := false
		if len(args) > 1 && len(args[1]) > 0 {
			var o struct {
				Complete bool `json:"complete"`
			}
			if err := json.Unmarshal(args[1], &o); err != nil {
				return nil, err
			}
			complete = o.Complete
		}
		if complete {
			full, err := jsonwebtoken.DecodeComplete(token)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"header":    full.Header,
				"payload":   full.Payload,
				"signature": full.Signature,
			}, nil
		}
		claims, err := jsonwebtoken.Decode(token)
		if err != nil {
			return nil, err
		}
		return claims, nil
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] != "" {
		fixtures = os.Args[1]
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	enc := json.NewEncoder(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := call(req.Fn, req.Args)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		_ = enc.Encode(response{ID: req.ID, OK: true, Value: v})
	}
}
