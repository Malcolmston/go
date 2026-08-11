// Command run is the Go side of the prompts parity harness: the same JSON Lines
// protocol as node/run.mjs, answered by github.com/malcolmston/chalk/prompts.
//
// Every prompt is driven by a scripted keyboard rather than a TTY: the case's key
// tokens become the same raw bytes as on the Node side and are handed to the
// prompt through cfg.In, with cfg.Out pointed at io.Discard. No clock or random
// value can reach a returned value.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/malcolmston/chalk/prompts"
)

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

// ------------------------------------------------------------- key scripting

// keyBytes maps a token to the bytes a terminal would send. Anything that is not
// a token is written literally, so "abc" types three characters.
var keyBytes = map[string]string{
	"<enter>":     "\r",
	"<up>":        "\x1b[A",
	"<down>":      "\x1b[B",
	"<right>":     "\x1b[C",
	"<left>":      "\x1b[D",
	"<space>":     " ",
	"<tab>":       "\t",
	"<backspace>": "\x7f",
	"<ctrl-c>":    "\x03",
}

func script(tokens []string) (io.Reader, error) {
	var b bytes.Buffer
	for _, t := range tokens {
		if s, ok := keyBytes[t]; ok {
			b.WriteString(s)
			continue
		}
		if strings.HasPrefix(t, "<") {
			return nil, fmt.Errorf("unknown key token: %s", t)
		}
		b.WriteString(t)
	}
	return &b, nil
}

// ------------------------------------------------------- named hook registry

// The case files are language-agnostic JSON and cannot carry functions, so
// validators and formatters are referenced by name and defined identically in
// both runners.
var stringValidators = map[string]func(string) error{
	"nonEmpty": func(s string) error {
		if s == "" {
			return errors.New("a value is required")
		}
		return nil
	},
	"min3": func(s string) error {
		if len([]rune(s)) < 3 {
			return errors.New("at least three characters")
		}
		return nil
	},
	"reject": func(string) error { return errors.New("never acceptable") },
}

var numberValidators = map[string]func(float64) error{
	"even": func(v float64) error {
		if int64(v)%2 != 0 {
			return errors.New("must be even")
		}
		return nil
	},
}

var formats = map[string]func(string) string{
	"upper": strings.ToUpper,
	"trim":  strings.TrimSpace,
}

// ------------------------------------------------------------------ the case

type choiceSpec struct {
	Title    string          `json:"title"`
	Value    json.RawMessage `json:"value"`
	Disabled bool            `json:"disabled"`
	Selected bool            `json:"selected"`
}

type config struct {
	Message  string          `json:"message"`
	Initial  json.RawMessage `json:"initial"`
	Validate string          `json:"validate"`
	Format   string          `json:"format"`
	Mask     string          `json:"mask"`
	Float    *bool           `json:"float"`
	Min      *float64        `json:"min"`
	Max      *float64        `json:"max"`
	Choices  []choiceSpec    `json:"choices"`
}

func (c config) message() string {
	if c.Message == "" {
		return "q"
	}
	return c.Message
}

func (c config) stringValidator() (func(string) error, error) {
	if c.Validate == "" {
		return nil, nil
	}
	v, ok := stringValidators[c.Validate]
	if !ok {
		return nil, fmt.Errorf("unknown validator: %s", c.Validate)
	}
	return v, nil
}

func (c config) numberValidator() (func(float64) error, error) {
	if c.Validate == "" {
		return nil, nil
	}
	v, ok := numberValidators[c.Validate]
	if !ok {
		return nil, fmt.Errorf("unknown number validator: %s", c.Validate)
	}
	return v, nil
}

func (c config) transform() (func(string) string, error) {
	if c.Format == "" {
		return nil, nil
	}
	f, ok := formats[c.Format]
	if !ok {
		return nil, fmt.Errorf("unknown format: %s", c.Format)
	}
	return f, nil
}

func (c config) choices() ([]prompts.Choice, error) {
	out := make([]prompts.Choice, 0, len(c.Choices))
	for _, s := range c.Choices {
		ch := prompts.Choice{Name: s.Title, Disabled: s.Disabled, Checked: s.Selected}
		if len(s.Value) > 0 {
			var v any
			if err := json.Unmarshal(s.Value, &v); err != nil {
				return nil, err
			}
			ch.Value = v
		}
		out = append(out, ch)
	}
	return out, nil
}

func (c config) initialString() string {
	var s string
	if len(c.Initial) > 0 {
		_ = json.Unmarshal(c.Initial, &s)
	}
	return s
}

func (c config) initialBool() bool {
	var b bool
	if len(c.Initial) > 0 {
		_ = json.Unmarshal(c.Initial, &b)
	}
	return b
}

func (c config) initialInt() int {
	var n int
	if len(c.Initial) > 0 {
		_ = json.Unmarshal(c.Initial, &n)
	}
	return n
}

func (c config) initialFloat() *float64 {
	if len(c.Initial) == 0 {
		return nil
	}
	var f float64
	if err := json.Unmarshal(c.Initial, &f); err != nil {
		return nil
	}
	return &f
}

// ------------------------------------------------------------------ dispatch

func dispatch(req request) (any, error) {
	var cfg config
	var tokens []string
	if len(req.Args) > 0 && len(req.Args[0]) > 0 {
		if err := json.Unmarshal(req.Args[0], &cfg); err != nil {
			return nil, err
		}
	}
	if len(req.Args) > 1 && len(req.Args[1]) > 0 {
		if err := json.Unmarshal(req.Args[1], &tokens); err != nil {
			return nil, err
		}
	}
	in, err := script(tokens)
	if err != nil {
		return nil, err
	}
	out := io.Discard

	switch req.Fn {
	case "text":
		validate, err := cfg.stringValidator()
		if err != nil {
			return nil, err
		}
		transform, err := cfg.transform()
		if err != nil {
			return nil, err
		}
		return prompts.Input(prompts.InputConfig{
			Message:   cfg.message(),
			Default:   cfg.initialString(),
			Validate:  validate,
			Transform: transform,
			In:        in,
			Out:       out,
		})

	case "password":
		validate, err := cfg.stringValidator()
		if err != nil {
			return nil, err
		}
		var mask rune
		if cfg.Mask != "" {
			mask = []rune(cfg.Mask)[0]
		}
		return prompts.Password(prompts.PasswordConfig{
			Message:  cfg.message(),
			Validate: validate,
			Mask:     mask,
			In:       in,
			Out:      out,
		})

	case "number":
		validate, err := cfg.numberValidator()
		if err != nil {
			return nil, err
		}
		// Upstream's `float` flag selects parseFloat over parseInt; the port's
		// Integer flag is its inverse.
		integer := true
		if cfg.Float != nil {
			integer = !*cfg.Float
		}
		return prompts.Number(prompts.NumberConfig{
			Message:  cfg.message(),
			Default:  cfg.initialFloat(),
			Min:      cfg.Min,
			Max:      cfg.Max,
			Integer:  integer,
			Validate: validate,
			In:       in,
			Out:      out,
		})

	case "confirm":
		return prompts.Confirm(prompts.ConfirmConfig{
			Message: cfg.message(),
			Default: cfg.initialBool(),
			In:      in,
			Out:     out,
		})

	case "select":
		choices, err := cfg.choices()
		if err != nil {
			return nil, err
		}
		_, choice, err := prompts.Select(prompts.SelectConfig{
			Message: cfg.message(),
			Choices: choices,
			Default: cfg.initialInt(),
			In:      in,
			Out:     out,
		})
		if err != nil {
			return nil, err
		}
		return choice.ResolvedValue(), nil

	case "multiselect":
		choices, err := cfg.choices()
		if err != nil {
			return nil, err
		}
		_, chosen, err := prompts.MultiSelect(prompts.MultiSelectConfig{
			Message: cfg.message(),
			Choices: choices,
			In:      in,
			Out:     out,
		})
		if err != nil {
			return nil, err
		}
		// Upstream returns [] for "nothing selected", never null.
		values := make([]any, 0, len(chosen))
		for _, c := range chosen {
			values = append(values, c.ResolvedValue())
		}
		return values, nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

// call runs one case, turning a panic into an ordinary failed case.
func call(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(req)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		value, err := call(req)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, prompts.ErrCanceled) {
				msg = "canceled"
			}
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: msg})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: value})
		}
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			os.Exit(1)
		}
	}
}
