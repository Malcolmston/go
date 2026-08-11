// Command run is the Go side of the express/otpauth parity harness. It speaks
// the same JSON Lines protocol as node/run.js and never exits on a failing
// case.
//
// Nothing here reads the clock: TOTP cases carry an explicit timestamp in
// milliseconds (converted to a time.Time) and HOTP cases an explicit counter.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/express/otpauth"
)

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

// config mirrors the case-file shape shared with the Node runner. Pointers make
// "absent" distinguishable from "zero", so each side can apply its own defaults.
type config struct {
	Type                string  `json:"type"`
	Issuer              *string `json:"issuer"`
	Label               *string `json:"label"`
	Secret              *string `json:"secret"`
	Algorithm           *string `json:"algorithm"`
	Digits              *int    `json:"digits"`
	Period              *int    `json:"period"`
	Counter             *uint64 `json:"counter"`
	OmitIssuerFromLabel bool    `json:"omitIssuerFromLabel"`
}

type options struct {
	Token     *string `json:"token"`
	Timestamp *int64  `json:"timestamp"`
	Counter   *uint64 `json:"counter"`
	Window    *int    `json:"window"`
}

func (c config) toConfig() otpauth.Config {
	out := otpauth.Config{Type: c.Type, OmitIssuerFromLabel: c.OmitIssuerFromLabel}
	if c.Issuer != nil {
		out.Issuer = *c.Issuer
	}
	if c.Label != nil {
		out.Account = *c.Label
	}
	if c.Secret != nil {
		out.Secret = *c.Secret
	}
	if c.Algorithm != nil {
		out.Algorithm = *c.Algorithm
	}
	if c.Digits != nil {
		out.Digits = *c.Digits
	}
	if c.Period != nil {
		out.Period = *c.Period
	}
	if c.Counter != nil {
		out.Counter = *c.Counter
	}
	return out
}

// describe renders a Config the way node/run.js renders an HOTP/TOTP object,
// filling in the same defaults the npm constructor applies.
func describe(c otpauth.Config) map[string]interface{} {
	typ := strings.ToLower(strings.TrimSpace(c.Type))
	if typ == "" {
		typ = "totp"
	}
	alg := strings.ToUpper(c.Algorithm)
	if alg == "" {
		alg = otpauth.DefaultAlgorithm
	}
	digits := c.Digits
	if digits <= 0 {
		digits = otpauth.DefaultDigits
	}
	out := map[string]interface{}{
		"type":                typ,
		"issuer":              c.Issuer,
		"label":               c.Account,
		"secret":              otpauth.NormalizeSecret(c.Secret),
		"algorithm":           alg,
		"digits":              digits,
		"omitIssuerFromLabel": c.OmitIssuerFromLabel,
	}
	if typ == "hotp" {
		out["counter"] = c.Counter
	} else {
		period := c.Period
		if period <= 0 {
			period = otpauth.DefaultPeriod
		}
		out["period"] = period
	}
	return out
}

func parseConfig(raw json.RawMessage) (config, error) {
	var c config
	if len(raw) == 0 {
		return c, fmt.Errorf("missing config")
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("bad config %s: %v", raw, err)
	}
	return c, nil
}

func parseOptions(args []json.RawMessage) (options, error) {
	var o options
	if len(args) < 2 || len(args[1]) == 0 {
		return o, nil
	}
	if err := json.Unmarshal(args[1], &o); err != nil {
		return o, fmt.Errorf("bad options %s: %v", args[1], err)
	}
	return o, nil
}

func str(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("expected a string argument, got %s", raw)
	}
	return s, nil
}

// at converts an upstream millisecond timestamp to a time.Time.
func at(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// validateSecret mirrors the npm constructor, which builds a Secret up front and
// throws on a secret that is not base32 before any code is generated.
func validateSecret(c otpauth.Config) error {
	_, err := otpauth.DecodeSecret(c.Secret)
	return err
}

func call(fn string, args []json.RawMessage) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if len(args) == 0 {
		return nil, fmt.Errorf("%s needs an argument", fn)
	}

	switch fn {
	case "parse", "parseThenUri":
		uri, err := str(args[0])
		if err != nil {
			return nil, err
		}
		cfg, err := otpauth.Parse(uri)
		if err != nil {
			return nil, err
		}
		if err := validateSecret(cfg); err != nil {
			return nil, err
		}
		if fn == "parse" {
			return describe(cfg), nil
		}
		return otpauth.URL(cfg), nil
	case "secretHex":
		b32, err := str(args[0])
		if err != nil {
			return nil, err
		}
		raw, err := otpauth.DecodeSecret(b32)
		if err != nil {
			return nil, err
		}
		return hex.EncodeToString(raw), nil
	}

	spec, err := parseConfig(args[0])
	if err != nil {
		return nil, err
	}
	opts, err := parseOptions(args)
	if err != nil {
		return nil, err
	}
	cfg := spec.toConfig()

	switch fn {
	case "uri", "stringify":
		if err := validateSecret(cfg); err != nil {
			return nil, err
		}
		return otpauth.URL(cfg), nil
	case "counter":
		if opts.Timestamp == nil {
			return nil, fmt.Errorf("counter needs a timestamp")
		}
		c := cfg
		c.Type = "totp"
		n, err := c.CounterAt(at(*opts.Timestamp))
		if err != nil {
			return nil, err
		}
		return n, nil
	case "generate":
		if opts.Counter != nil {
			cfg.Counter = *opts.Counter
		}
		t := time.Unix(0, 0).UTC()
		if opts.Timestamp != nil {
			t = at(*opts.Timestamp)
		} else if strings.ToLower(cfg.Type) != "hotp" {
			return nil, fmt.Errorf("totp generate needs a timestamp")
		}
		if err := validateSecret(cfg); err != nil {
			return nil, err
		}
		code, err := cfg.Generate(t)
		if err != nil {
			return nil, err
		}
		return code, nil
	case "validate":
		if opts.Token == nil {
			return nil, fmt.Errorf("validate needs a token")
		}
		if opts.Counter != nil {
			cfg.Counter = *opts.Counter
		}
		window := 1
		if opts.Window != nil {
			window = *opts.Window
		}
		t := time.Unix(0, 0).UTC()
		if opts.Timestamp != nil {
			t = at(*opts.Timestamp)
		}
		// The npm library builds the Secret before validating, so an invalid
		// secret is an error rather than a failed comparison.
		if err := validateSecret(cfg); err != nil {
			return nil, err
		}
		delta, ok := cfg.Validate(*opts.Token, t, window)
		if !ok {
			return nil, nil
		}
		return delta, nil
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

func main() {
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
