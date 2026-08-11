// Command run is the Go half of parity/passport/nested/otpauth: the same JSON
// Lines protocol as node/run.js, answered by
// github.com/malcolmston/passport/otpauth.
//
// Conventions shared with the upstream runner: a key is the object {type,
// issuer, account, secret, algorithm, digits, period, counter} with a base32
// secret, timestamps are explicit milliseconds, raw bytes are lowercase hex,
// `url` answers a structured {host, path, params} rendering, and `verify`
// answers {ok, delta}. A failing case is reported as {ok:false,error} and never
// terminates the process.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/malcolmston/passport/otpauth"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// No omitempty: a legitimate answer may be false, 0 or "".
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

type keyArgs struct {
	Type      string `json:"type"`
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
	Secret    string `json:"secret"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Counter   uint64 `json:"counter"`
}

// key builds the port's Key from the case's object, applying the same defaults
// the node runner applies: totp, SHA1, 6 digits, a 30-second period.
func (k keyArgs) key() otpauth.Key {
	typ := otpauth.Type(k.Type)
	if typ == "" {
		typ = otpauth.TOTP
	}
	alg := k.Algorithm
	if alg == "" {
		alg = "SHA1"
	}
	digits := k.Digits
	if digits == 0 {
		digits = 6
	}
	out := otpauth.Key{
		Type:      typ,
		Issuer:    k.Issuer,
		Account:   k.Account,
		Secret:    k.Secret,
		Algorithm: alg,
		Digits:    digits,
	}
	if typ == otpauth.HOTP {
		out.Counter = k.Counter
	} else if out.Period = k.Period; out.Period == 0 {
		out.Period = 30
	}
	return out
}

func decodeKey(raw json.RawMessage) (otpauth.Key, error) {
	var k keyArgs
	if err := json.Unmarshal(raw, &k); err != nil {
		return otpauth.Key{}, fmt.Errorf("argument is not a key object: %w", err)
	}
	return k.key(), nil
}

func normalise(k otpauth.Key) map[string]any {
	period, counter := k.Period, uint64(0)
	if k.Type == otpauth.HOTP {
		period, counter = 0, k.Counter
	}
	return map[string]any{
		"type":      string(k.Type),
		"issuer":    k.Issuer,
		"account":   k.Account,
		"secret":    k.Secret,
		"algorithm": k.Algorithm,
		"digits":    k.Digits,
		"period":    period,
		"counter":   counter,
	}
}

// structuredURI splits a rendered otpauth URI into the shape both runners emit:
// host, raw path, and the query as a key/value object with sorted keys.
func structuredURI(raw string) (map[string]any, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	params := make(map[string]any, len(keys))
	for _, k := range keys {
		params[k] = q.Get(k)
	}
	return map[string]any{"host": u.Host, "path": u.EscapedPath(), "params": params}, nil
}

func millis(raw json.RawMessage) (time.Time, error) {
	var ms int64
	if err := json.Unmarshal(raw, &ms); err != nil {
		return time.Time{}, fmt.Errorf("timestamp is not a number of milliseconds: %w", err)
	}
	return time.UnixMilli(ms).UTC(), nil
}

func dispatch(req request) (any, error) {
	need := func(n int) error {
		if len(req.Args) < n {
			return fmt.Errorf("expected %d arguments, got %d", n, len(req.Args))
		}
		return nil
	}

	switch req.Fn {
	case "parse":
		if err := need(1); err != nil {
			return nil, err
		}
		var uri string
		if err := json.Unmarshal(req.Args[0], &uri); err != nil {
			return nil, err
		}
		k, err := otpauth.Parse(uri)
		if err != nil {
			return nil, err
		}
		return normalise(k), nil

	case "url":
		if err := need(1); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		return structuredURI(k.URL())

	case "decodeSecret":
		if err := need(1); err != nil {
			return nil, err
		}
		var secret string
		if err := json.Unmarshal(req.Args[0], &secret); err != nil {
			return nil, err
		}
		b, err := otpauth.DecodeSecret(secret)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(b), nil

	case "usable":
		if err := need(1); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		return k.Validate() == nil, nil

	case "code":
		if err := need(2); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		t, err := millis(req.Args[1])
		if err != nil {
			return nil, err
		}
		return k.Code(t)

	case "codeAtCounter":
		if err := need(2); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		var counter uint64
		if err := json.Unmarshal(req.Args[1], &counter); err != nil {
			return nil, err
		}
		return k.CodeAtCounter(counter)

	case "movingFactor":
		if err := need(2); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		t, err := millis(req.Args[1])
		if err != nil {
			return nil, err
		}
		return k.MovingFactor(t), nil

	case "verify":
		if err := need(4); err != nil {
			return nil, err
		}
		k, err := decodeKey(req.Args[0])
		if err != nil {
			return nil, err
		}
		var code string
		if err := json.Unmarshal(req.Args[1], &code); err != nil {
			return nil, err
		}
		t, err := millis(req.Args[2])
		if err != nil {
			return nil, err
		}
		var window int
		if err := json.Unmarshal(req.Args[3], &window); err != nil {
			return nil, err
		}
		matched, ok := k.Verify(code, t, window)
		if !ok {
			return map[string]any{"ok": false, "delta": 0}, nil
		}
		// Upstream reports the offset from the expected moving factor, so the
		// port's absolute counter is converted to the same delta.
		base := k.MovingFactor(t)
		return map[string]any{"ok": true, "delta": int64(matched) - int64(base)}, nil

	default:
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		v, err := dispatch(req)
		if err != nil {
			emit(response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(response{ID: req.ID, OK: true, Value: v})
	}
}

func emit(r response) {
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{ID: r.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	_, _ = os.Stdout.Write(append(b, '\n'))
	_ = os.Stdout.Sync()
}
