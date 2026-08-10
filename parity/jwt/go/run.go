// Command run is the Go-side parity runner for github.com/malcolmston/jwt.
//
// It speaks JSON Lines on stdio exactly as parity/HARNESS.md specifies and as
// parity/jwt/node/run.js does, so the harness can stream identical cases through
// both and compare the answers.
//
// It never exits on a failing case: every panic and error becomes {"ok":false}.
package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	jwtlib "github.com/malcolmston/jwt"
)

var keysDir string

// ------------------------------------------------------------------- wire

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

// ---------------------------------------------------------------- key specs

type keySpec struct {
	Kind    string `json:"kind"`
	Secret  string `json:"secret"`
	File    string `json:"file"`
	Private bool   `json:"private"`
}

func readKeyFile(name string) ([]byte, error) {
	if strings.Contains(name, "..") || filepath.IsAbs(name) {
		return nil, fmt.Errorf("bad key file: %s", name)
	}
	return os.ReadFile(filepath.Join(keysDir, name))
}

// resolveKey turns a case's key spec into the Go value the port expects. The
// port type-asserts keys inside every signing method, so handing it the right
// concrete type is the whole point of this function.
func resolveKey(spec *keySpec) (any, error) {
	if spec == nil {
		return nil, nil
	}
	switch spec.Kind {
	case "none":
		return jwtlib.UnsafeAllowNoneSignatureType, nil
	case "oct":
		if spec.File != "" {
			b, err := readKeyFile(spec.File)
			if err != nil {
				return nil, err
			}
			return b, nil
		}
		return []byte(spec.Secret), nil
	case "rsa":
		b, err := readKeyFile(spec.File)
		if err != nil {
			return nil, err
		}
		if spec.Private {
			return jwtlib.ParseRSAPrivateKeyFromPEM(b)
		}
		return jwtlib.ParseRSAPublicKeyFromPEM(b)
	case "ec":
		b, err := readKeyFile(spec.File)
		if err != nil {
			return nil, err
		}
		if spec.Private {
			return jwtlib.ParseECPrivateKeyFromPEM(b)
		}
		return jwtlib.ParseECPublicKeyFromPEM(b)
	case "ed":
		b, err := readKeyFile(spec.File)
		if err != nil {
			return nil, err
		}
		if spec.Private {
			return jwtlib.ParseEdPrivateKeyFromPEM(b)
		}
		return jwtlib.ParseEdPublicKeyFromPEM(b)
	default:
		return nil, fmt.Errorf("unknown key kind: %q", spec.Kind)
	}
}

// ------------------------------------------------------------------ options

type signOpts struct {
	Algorithm                      string         `json:"algorithm"`
	ExpiresIn                      any            `json:"expiresIn"`
	NotBefore                      any            `json:"notBefore"`
	Audience                       any            `json:"audience"`
	Issuer                         *string        `json:"issuer"`
	Subject                        *string        `json:"subject"`
	JWTID                          *string        `json:"jwtid"`
	KeyID                          *string        `json:"keyid"`
	NoTimestamp                    bool           `json:"noTimestamp"`
	Header                         map[string]any `json:"header"`
	Encoding                       *string        `json:"encoding"`
	MutatePayload                  *bool          `json:"mutatePayload"`
	AllowInsecureKeySizes          *bool          `json:"allowInsecureKeySizes"`
	AllowInvalidAsymmetricKeyTypes *bool          `json:"allowInvalidAsymmetricKeyTypes"`
}

type verifyOpts struct {
	Algorithms                     []string `json:"algorithms"`
	Audience                       any      `json:"audience"`
	Issuer                         any      `json:"issuer"`
	Subject                        *string  `json:"subject"`
	JWTID                          *string  `json:"jwtid"`
	Nonce                          *string  `json:"nonce"`
	ClockTolerance                 *float64 `json:"clockTolerance"`
	ClockTimestamp                 *float64 `json:"clockTimestamp"`
	MaxAge                         any      `json:"maxAge"`
	IgnoreExpiration               bool     `json:"ignoreExpiration"`
	IgnoreNotBefore                bool     `json:"ignoreNotBefore"`
	Complete                       bool     `json:"complete"`
	AllowInvalidAsymmetricKeyTypes *bool    `json:"allowInvalidAsymmetricKeyTypes"`
}

func intSeconds(v any, what string) (int64, error) {
	f, ok := v.(float64)
	if !ok || f != float64(int64(f)) {
		return 0, fmt.Errorf("port has no equivalent for a non-integer %q (%v)", what, v)
	}
	return int64(f), nil
}

// buildSignClaims reproduces node-jsonwebtoken's option->claim projection. The
// Go port has no sign-time options: it signs whatever claims it is given, so the
// runner does the projection and the case still exercises the same behaviour.
func buildSignClaims(payload map[string]any, o signOpts) (jwtlib.MapClaims, error) {
	claims := jwtlib.MapClaims{}
	for k, v := range payload {
		claims[k] = v
	}

	if o.NoTimestamp {
		delete(claims, "iat")
	} else if _, ok := claims["iat"]; !ok {
		// Upstream would default iat to Date.now(); the harness requires an
		// explicit iat so both sides stay deterministic.
		return nil, fmt.Errorf("parity runner requires an explicit iat (or noTimestamp)")
	}

	var ts int64
	if v, ok := payload["iat"]; ok {
		f, err := intSeconds(v, "iat")
		if err != nil {
			return nil, err
		}
		ts = f
	}

	if o.ExpiresIn != nil {
		if _, dup := payload["exp"]; dup {
			return nil, fmt.Errorf("bad \"options.expiresIn\" option: the payload already has an \"exp\" property")
		}
		n, err := intSeconds(o.ExpiresIn, "expiresIn")
		if err != nil {
			return nil, err
		}
		claims["exp"] = float64(ts + n)
	}
	if o.NotBefore != nil {
		if _, dup := payload["nbf"]; dup {
			return nil, fmt.Errorf("bad \"options.notBefore\" option: the payload already has an \"nbf\" property")
		}
		n, err := intSeconds(o.NotBefore, "notBefore")
		if err != nil {
			return nil, err
		}
		claims["nbf"] = float64(ts + n)
	}
	if o.Audience != nil {
		claims["aud"] = o.Audience
	}
	if o.Issuer != nil {
		claims["iss"] = *o.Issuer
	}
	if o.Subject != nil {
		claims["sub"] = *o.Subject
	}
	if o.JWTID != nil {
		claims["jti"] = *o.JWTID
	}
	if o.Encoding != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"encoding\" sign option")
	}
	if o.AllowInsecureKeySizes != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"allowInsecureKeySizes\" sign option")
	}
	if o.AllowInvalidAsymmetricKeyTypes != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"allowInvalidAsymmetricKeyTypes\" sign option")
	}
	return claims, nil
}

func buildParserOptions(o verifyOpts, alg string) ([]jwtlib.ParserOption, error) {
	var opts []jwtlib.ParserOption

	if o.ClockTimestamp == nil {
		return nil, fmt.Errorf("parity runner requires an explicit clockTimestamp")
	}
	now := time.Unix(int64(*o.ClockTimestamp), 0).UTC()
	opts = append(opts, jwtlib.WithTimeFunc(func() time.Time { return now }))

	if o.ClockTolerance != nil {
		opts = append(opts, jwtlib.WithLeeway(time.Duration(*o.ClockTolerance*float64(time.Second))))
	}
	if o.Algorithms != nil {
		opts = append(opts, jwtlib.WithValidMethods(o.Algorithms))
		for _, a := range o.Algorithms {
			if a == "none" {
				opts = append(opts, jwtlib.WithAllowNone())
			}
		}
	}
	switch a := o.Audience.(type) {
	case nil:
	case string:
		opts = append(opts, jwtlib.WithAudience(a))
	case []any:
		// Upstream treats an array as "the token must match any one of these".
		// The port exposes only a single-value WithAudience.
		if len(a) == 1 {
			s, ok := a[0].(string)
			if !ok {
				return nil, fmt.Errorf("audience array element is not a string")
			}
			opts = append(opts, jwtlib.WithAudience(s))
			break
		}
		return nil, fmt.Errorf("port has no equivalent for a multi-valued \"audience\" verify option")
	default:
		return nil, fmt.Errorf("port has no equivalent for audience of type %T", o.Audience)
	}
	switch i := o.Issuer.(type) {
	case nil:
	case string:
		opts = append(opts, jwtlib.WithIssuer(i))
	case []any:
		if len(i) == 1 {
			s, ok := i[0].(string)
			if !ok {
				return nil, fmt.Errorf("issuer array element is not a string")
			}
			opts = append(opts, jwtlib.WithIssuer(s))
			break
		}
		return nil, fmt.Errorf("port has no equivalent for a multi-valued \"issuer\" verify option")
	default:
		return nil, fmt.Errorf("port has no equivalent for issuer of type %T", o.Issuer)
	}
	if o.Subject != nil {
		opts = append(opts, jwtlib.WithSubject(*o.Subject))
	}
	if o.JWTID != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"jwtid\" verify option")
	}
	if o.Nonce != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"nonce\" verify option")
	}
	if o.MaxAge != nil {
		n, err := intSeconds(o.MaxAge, "maxAge")
		if err != nil {
			return nil, err
		}
		opts = append(opts, jwtlib.WithMaxTokenAge(time.Duration(n)*time.Second))
	}
	if o.IgnoreExpiration {
		opts = append(opts, jwtlib.WithIgnoreExpiration())
	}
	if o.IgnoreNotBefore {
		opts = append(opts, jwtlib.WithIgnoreNotBefore())
	}
	if o.AllowInvalidAsymmetricKeyTypes != nil {
		return nil, fmt.Errorf("port has no equivalent for the \"allowInvalidAsymmetricKeyTypes\" verify option")
	}
	_ = alg
	return opts, nil
}

// ------------------------------------------------------------------- ops

func doSign(payload map[string]any, spec *keySpec, o signOpts) (string, error) {
	alg := o.Algorithm
	if alg == "" {
		alg = "HS256"
	}
	method := jwtlib.GetSigningMethod(alg)
	if method == nil {
		return "", fmt.Errorf("unsupported alg %q", alg)
	}
	claims, err := buildSignClaims(payload, o)
	if err != nil {
		return "", err
	}
	key, err := resolveKey(spec)
	if err != nil {
		return "", err
	}
	tok := jwtlib.NewWithClaims(method, claims)
	if o.KeyID != nil {
		tok.SetKID(*o.KeyID)
	}
	for _, k := range sortedKeys(o.Header) {
		tok.SetHeader(k, o.Header[k])
	}
	return tok.SignedString(key)
}

func doVerify(token string, spec *keySpec, o verifyOpts) (any, error) {
	key, err := resolveKey(spec)
	if err != nil {
		return nil, err
	}
	opts, err := buildParserOptions(o, "")
	if err != nil {
		return nil, err
	}
	tok, err := jwtlib.Parse(token, func(*jwtlib.Token) (any, error) { return key, nil }, opts...)
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, fmt.Errorf("jwt: token is invalid")
	}
	if o.Complete {
		return map[string]any{"header": tok.Header, "payload": tok.Claims}, nil
	}
	return tok.Claims, nil
}

func decodeSeg(seg string) (map[string]any, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func doDecode(token string, complete bool) (any, error) {
	// jwt.ParseUnverified is the port's counterpart to jwt.decode: it decodes
	// the header and claims without checking the signature.
	claims := jwtlib.MapClaims{}
	tok, _, err := jwtlib.ParseUnverified(token, claims)
	if err != nil {
		// Upstream jwt.decode returns null instead of throwing for a token whose
		// payload is not JSON, but throws for a structurally broken token. Report
		// the failure and let the harness compare outcomes.
		return nil, err
	}
	if complete {
		return map[string]any{"header": tok.Header, "payload": tok.Claims}, nil
	}
	return tok.Claims, nil
}

var fns = map[string]func(args []json.RawMessage) (any, error){
	"sign": func(args []json.RawMessage) (any, error) {
		var payload map[string]any
		var spec keySpec
		var o signOpts
		if err := unpack(args, &payload, &spec, &o); err != nil {
			return nil, err
		}
		return doSign(payload, &spec, o)
	},

	"verify": func(args []json.RawMessage) (any, error) {
		var token string
		var spec keySpec
		var o verifyOpts
		if err := unpack(args, &token, &spec, &o); err != nil {
			return nil, err
		}
		return doVerify(token, &spec, o)
	},

	"decode": func(args []json.RawMessage) (any, error) {
		var token string
		var o struct {
			Complete bool `json:"complete"`
		}
		if err := unpack(args, &token, &o); err != nil {
			return nil, err
		}
		return doDecode(token, o.Complete)
	},

	"roundtrip": func(args []json.RawMessage) (any, error) {
		var payload map[string]any
		var signSpec, verifySpec keySpec
		var so signOpts
		var vo verifyOpts
		if err := unpack(args, &payload, &signSpec, &verifySpec, &so, &vo); err != nil {
			return nil, err
		}
		token, err := doSign(payload, &signSpec, so)
		if err != nil {
			return nil, err
		}
		return doVerify(token, &verifySpec, vo)
	},

	"tokenParts": func(args []json.RawMessage) (any, error) {
		var payload map[string]any
		var spec keySpec
		var o signOpts
		if err := unpack(args, &payload, &spec, &o); err != nil {
			return nil, err
		}
		token, err := doSign(payload, &spec, o)
		if err != nil {
			return nil, err
		}
		p := strings.Split(token, ".")
		if len(p) != 3 {
			return nil, fmt.Errorf("signed token did not have three parts")
		}
		hdr, err := decodeSeg(p[0])
		if err != nil {
			return nil, err
		}
		pl, err := decodeSeg(p[1])
		if err != nil {
			return nil, err
		}
		sig, err := base64.RawURLEncoding.DecodeString(p[2])
		if err != nil {
			return nil, err
		}
		return map[string]any{"header": hdr, "payload": pl, "sigLen": len(sig)}, nil
	},
}

// unpack decodes positional args into the given destinations. A missing arg
// leaves its destination at its zero value, matching the JS runner's `|| {}`.
func unpack(args []json.RawMessage, dsts ...any) error {
	for i, dst := range dsts {
		if i >= len(args) || len(args[i]) == 0 || string(args[i]) == "null" {
			continue
		}
		if err := json.Unmarshal(args[i], dst); err != nil {
			return fmt.Errorf("arg %d: %w", i, err)
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ------------------------------------------------------------------- loop

func handle(c request) (res response) {
	res = response{ID: c.ID}
	defer func() {
		if r := recover(); r != nil {
			res = response{ID: c.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	fn, ok := fns[c.Fn]
	if !ok {
		return response{ID: c.ID, OK: false, Error: "unknown fn: " + c.Fn}
	}
	v, err := fn(c.Args)
	if err != nil {
		return response{ID: c.ID, OK: false, Error: err.Error()}
	}
	return response{ID: c.ID, OK: true, Value: v}
}

func main() {
	keysDir = filepath.Join("..", "keys")
	if len(os.Args) > 1 {
		keysDir = os.Args[1]
	}
	// Referenced so the concrete key types the port asserts on are visibly part
	// of this runner's contract.
	var (
		_ *rsa.PrivateKey
		_ *ecdsa.PrivateKey
		_ ed25519.PrivateKey
	)

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var c request
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad case json: " + err.Error()})
			_ = out.Flush()
			continue
		}
		_ = enc.Encode(handle(c))
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			return
		}
	}
}
