// Go runner for the jose parity harness.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. A failing case is reported
// as {"ok":false,"error":...}; the process never exits early.
//
//	go run ./go <keysDir>
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malcolmston/jose"
)

// ---------------------------------------------------------------- wire types

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// options is the shared, language-neutral option bag used by every case.
type options struct {
	Alg string `json:"alg"`
	Enc string `json:"enc"`
	Kid string `json:"kid"`
	Zip string `json:"zip"`

	B64  *bool    `json:"b64"`
	Crit []string `json:"crit"`

	Protected       map[string]any `json:"protected"`
	Unprotected     map[string]any `json:"unprotected"`
	RecipientHeader map[string]any `json:"recipientHeader"`
	AAD             *string        `json:"aad"`

	Detached        bool    `json:"detached"`
	DetachedPayload *string `json:"detachedPayload"`

	APU string   `json:"apu"`
	APV string   `json:"apv"`
	P2S string   `json:"p2s"`
	P2C *float64 `json:"p2c"`

	Algorithms    []string `json:"algorithms"`
	Encryptions   []string `json:"encryptions"`
	KeyManagement []string `json:"keyManagement"`
	KnownCrit     []string `json:"knownCrit"`
}

type recipientSpec struct {
	Key    string         `json:"key"`
	Alg    string         `json:"alg"`
	Kid    string         `json:"kid"`
	Header map[string]any `json:"header"`
}

type signerSpec struct {
	Key         string         `json:"key"`
	Alg         string         `json:"alg"`
	Kid         string         `json:"kid"`
	Protected   map[string]any `json:"protected"`
	Unprotected map[string]any `json:"unprotected"`
	Crit        []string       `json:"crit"`
	B64         *bool          `json:"b64"`
}

// ------------------------------------------------------------------ key store

var keysDir string

var jwkCache = map[string]*jose.JWK{}

func loadJWK(ref string) (*jose.JWK, error) {
	if k, ok := jwkCache[ref]; ok {
		return k, nil
	}
	data, err := os.ReadFile(filepath.Join(keysDir, ref+".json"))
	if err != nil {
		return nil, err
	}
	k, err := jose.ParseJWK(data)
	if err != nil {
		return nil, err
	}
	jwkCache[ref] = k
	return k, nil
}

// keyOf returns the *jose.JWK for a reference. The port accepts a *JWK
// everywhere, which is also the only way it can see the "use"/"key_ops"/"alg"
// members that upstream enforces.
func keyOf(raw json.RawMessage) (any, error) {
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return nil, fmt.Errorf("key reference must be a string: %w", err)
	}
	k, err := loadJWK(ref)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// ------------------------------------------------------------------- helpers

func decodeArg(args []json.RawMessage, i int, dst any) error {
	if i >= len(args) {
		return nil
	}
	return json.Unmarshal(args[i], dst)
}

func mustString(args []json.RawMessage, i int) string {
	var s string
	if i < len(args) {
		_ = json.Unmarshal(args[i], &s)
	}
	return s
}

func optionsAt(args []json.RawMessage, i int) (options, error) {
	var o options
	if i < len(args) {
		if err := json.Unmarshal(args[i], &o); err != nil {
			return o, err
		}
	}
	return o, nil
}

// protectedHeader builds the protected header parameters the case asks for.
// "alg" and "enc" are passed through the port's dedicated option fields, so
// they are not repeated here.
func protectedHeader(o options) map[string]any {
	h := map[string]any{}
	for k, v := range o.Protected {
		h[k] = v
	}
	if o.Zip != "" {
		h["zip"] = o.Zip
	}
	if o.B64 != nil {
		h["b64"] = *o.B64
	}
	if o.APU != "" {
		h["apu"] = o.APU
	}
	if o.APV != "" {
		h["apv"] = o.APV
	}
	if o.P2S != "" {
		h["p2s"] = o.P2S
	}
	if o.P2C != nil {
		h["p2c"] = int(*o.P2C)
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

func signOptions(o options) jose.SignOptions {
	return jose.SignOptions{
		Algorithm:     o.Alg,
		KeyID:         o.Kid,
		Header:        protectedHeader(o),
		Unprotected:   o.Unprotected,
		Critical:      o.Crit,
		DetachPayload: o.Detached,
	}
}

func verifyOptions(o options) jose.VerifyOptions {
	v := jose.VerifyOptions{
		Algorithms:    o.Algorithms,
		KnownCritical: o.KnownCrit,
	}
	if o.DetachedPayload != nil {
		v.DetachedPayload = []byte(*o.DetachedPayload)
	}
	return v
}

func encryptOptions(o options) jose.EncryptOptions {
	e := jose.EncryptOptions{
		Algorithm:   o.Alg,
		Encryption:  o.Enc,
		KeyID:       o.Kid,
		Header:      protectedHeader(o),
		Unprotected: o.Unprotected,
		Compress:    o.Zip == "DEF",
	}
	if o.Zip == "DEF" {
		// Compress sets "zip" itself; leaving it in Header too would be a
		// duplicate.
		delete(e.Header, "zip")
		if len(e.Header) == 0 {
			e.Header = nil
		}
	}
	if o.AAD != nil {
		e.AdditionalAuthenticatedData = []byte(*o.AAD)
	}
	if len(o.Crit) > 0 {
		if e.Header == nil {
			e.Header = map[string]any{}
		}
		e.Header["crit"] = o.Crit
	}
	return e
}

func decryptOptions(o options) jose.DecryptOptions {
	return jose.DecryptOptions{
		Algorithms:    append(append([]string{}, o.Algorithms...), o.KeyManagement...),
		Encryptions:   o.Encryptions,
		KnownCritical: o.KnownCrit,
	}
}

// normalise turns a decoded header into something JSON-comparable with a
// stable key order (Go maps marshal sorted already, but nested values coming
// from the port may be []string, int, etc.).
func normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = normalise(t[k])
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = normalise(t[i])
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i := range t {
			out[i] = t[i]
		}
		return out
	case int:
		return float64(t)
	default:
		return v
	}
}

// jwkToMap renders a *jose.JWK as a plain map, so it compares against
// upstream's exportJWK output.
func jwkToMap(k *jose.JWK) (map[string]any, error) {
	data, err := json.Marshal(k)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

var errNotPorted = fmt.Errorf("not implemented by the port")

// --------------------------------------------------------------- operations

func dispatch(fn string, args []json.RawMessage) (any, error) {
	switch fn {

	// -- JWS ---------------------------------------------------------------

	case "sign.compact":
		payload := mustString(args, 0)
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		token, err := jose.Sign([]byte(payload), key, signOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": token}, nil

	case "verify.compact":
		token := mustString(args, 0)
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		payload, header, err := jose.VerifyWithOptions(token, key, verifyOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": string(payload), "header": normalise(header)}, nil

	case "sign.flattened", "encrypt.flattened":
		return nil, fmt.Errorf("%w: the port produces only the general JSON serialization", errNotPorted)

	case "sign.general":
		payload := mustString(args, 0)
		var specs []signerSpec
		if err := decodeArg(args, 1, &specs); err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		signers := make([]jose.Signer, 0, len(specs))
		for _, s := range specs {
			k, err := loadJWK(s.Key)
			if err != nil {
				return nil, err
			}
			so := jose.SignOptions{
				Algorithm:   s.Alg,
				KeyID:       s.Kid,
				Header:      s.Protected,
				Unprotected: s.Unprotected,
				Critical:    s.Crit,
			}
			if s.B64 != nil {
				if so.Header == nil {
					so.Header = map[string]any{}
				}
				so.Header["b64"] = *s.B64
			}
			signers = append(signers, jose.Signer{Key: k, Options: so})
		}
		data, err := jose.SignJSONMulti([]byte(payload), signers...)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		if o.Detached {
			doc["payload"] = ""
		}
		return map[string]any{"out": normalise(doc)}, nil

	case "verify.flattened", "verify.general":
		if len(args) < 2 {
			return nil, fmt.Errorf("missing arguments")
		}
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		payload, header, err := jose.VerifyJSONWithOptions(args[0], key, verifyOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": string(payload), "header": normalise(header)}, nil

	// -- JWE ---------------------------------------------------------------

	case "encrypt.compact":
		plaintext := mustString(args, 0)
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		token, err := jose.Encrypt([]byte(plaintext), key, encryptOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": token}, nil

	case "decrypt.compact":
		token := mustString(args, 0)
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		plaintext, header, err := jose.DecryptWithOptions(token, key, decryptOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": string(plaintext), "header": normalise(header)}, nil

	case "encrypt.general":
		plaintext := mustString(args, 0)
		var specs []recipientSpec
		if err := decodeArg(args, 1, &specs); err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		recipients := make([]jose.Recipient, 0, len(specs))
		for _, r := range specs {
			k, err := loadJWK(r.Key)
			if err != nil {
				return nil, err
			}
			alg := r.Alg
			if alg == "" {
				alg = o.Alg
			}
			recipients = append(recipients, jose.Recipient{
				Key: k, Algorithm: alg, KeyID: r.Kid, Header: r.Header,
			})
		}
		eo := encryptOptions(o)
		eo.Algorithm = ""
		data, err := jose.EncryptJSONMulti([]byte(plaintext), eo, recipients...)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		return map[string]any{"out": normalise(doc)}, nil

	case "decrypt.flattened", "decrypt.general":
		if len(args) < 2 {
			return nil, fmt.Errorf("missing arguments")
		}
		key, err := keyOf(args[1])
		if err != nil {
			return nil, err
		}
		o, err := optionsAt(args, 2)
		if err != nil {
			return nil, err
		}
		plaintext, header, err := jose.DecryptJSONWithOptions(args[0], key, decryptOptions(o))
		if err != nil {
			return nil, err
		}
		return map[string]any{"payload": string(plaintext), "header": normalise(header)}, nil

	// -- JWK ---------------------------------------------------------------

	case "jwk.roundtrip":
		ref := mustString(args, 0)
		jwk, err := loadJWK(ref)
		if err != nil {
			return nil, err
		}
		key, err := jwk.Key()
		if err != nil {
			return nil, err
		}
		back, err := jose.FromKey(key)
		if err != nil {
			return nil, err
		}
		m, err := jwkToMap(back)
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": normalise(m)}, nil

	case "jwk.thumbprint":
		ref := mustString(args, 0)
		jwk, err := loadJWK(ref)
		if err != nil {
			return nil, err
		}
		tp, err := jwk.Thumbprint()
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": tp}, nil

	case "jwk.thumbprintUri":
		return nil, fmt.Errorf("%w: no JWK thumbprint URI helper", errNotPorted)

	case "jwk.public":
		ref := mustString(args, 0)
		jwk, err := loadJWK(ref)
		if err != nil {
			return nil, err
		}
		pub, err := jwk.Public()
		if err != nil {
			return nil, err
		}
		m, err := jwkToMap(pub)
		if err != nil {
			return nil, err
		}
		// Upstream's exportJWK never carries "use"/"alg"/"kid"; drop them so
		// the comparison is about key material.
		delete(m, "use")
		delete(m, "alg")
		delete(m, "kid")
		delete(m, "key_ops")
		return map[string]any{"out": normalise(m)}, nil

	case "jwks.lookup":
		ref := mustString(args, 0)
		kid := mustString(args, 1)
		data, err := os.ReadFile(filepath.Join(keysDir, ref+".json"))
		if err != nil {
			return nil, err
		}
		set, err := jose.ParseJWKSet(data)
		if err != nil {
			return nil, err
		}
		jwk, ok := set.LookupKeyID(kid)
		if !ok {
			return nil, fmt.Errorf("%w: kid %q", jose.ErrKeyNotFound, kid)
		}
		tp, err := jwk.Thumbprint()
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": tp}, nil

	// -- util --------------------------------------------------------------

	case "b64.encode":
		h := mustString(args, 0)
		raw, err := hexDecode(h)
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": jose.EncodeSegment(raw)}, nil

	case "b64.decode":
		s := mustString(args, 0)
		raw, err := jose.DecodeSegment(s)
		if err != nil {
			return nil, err
		}
		return map[string]any{"out": hexEncode(raw)}, nil

	case "header.protected":
		return nil, fmt.Errorf("%w: no standalone protected-header decoder", errNotPorted)
	}
	return nil, fmt.Errorf("unknown fn %q", fn)
}

const hexDigits = "0123456789abcdef"

func hexEncode(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		sb.WriteByte(hexDigits[c>>4])
		sb.WriteByte(hexDigits[c&0x0f])
	}
	return sb.String()
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex string")
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi := strings.IndexByte(hexDigits, lower(s[2*i]))
		lo := strings.IndexByte(hexDigits, lower(s[2*i+1]))
		if hi < 0 || lo < 0 {
			return nil, fmt.Errorf("invalid hex string")
		}
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'F' {
		return c + 32
	}
	return c
}

// ------------------------------------------------------------------ run loop

func main() {
	keysDir = "../keys"
	if len(os.Args) > 1 {
		keysDir = os.Args[1]
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)

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
		emit(out, run(req))
	}
	out.Flush()
}

// run executes one request, converting a panic into a failing case rather than
// letting it take the whole runner down.
func run(req request) (res response) {
	defer func() {
		if r := recover(); r != nil {
			res = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	value, err := dispatch(req.Fn, req.Args)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: value}
}

func emit(out *bufio.Writer, res response) {
	data, err := json.Marshal(res)
	if err != nil {
		data, _ = json.Marshal(response{ID: res.ID, OK: false, Error: "unencodable value: " + err.Error()})
	}
	out.Write(data)
	out.WriteByte('\n')
	out.Flush()
}
