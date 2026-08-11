// Command run is the Go-side runner for the express/sanitizehtml parity harness.
//
// It speaks the same JSON Lines contract as node/run.js: one request object per
// line on stdin ({id, fn, args}), one reply object per line on stdout
// ({id, ok, value} or {id, ok:false, error}). A bad request or a bad argument
// shape is reported as ok:false rather than terminating the process, so a single
// losing case never costs the rest of the run.
//
// Two idiom differences are translated here rather than pushed into the cases,
// so that both runners can be fed the very same JSON:
//
//   - Upstream merges a supplied options object over sanitizeHtml.defaults, so
//     an option a case does not mention keeps its default. buildOptions starts
//     from sanitizehtml.DefaultOptions() for the same reason.
//   - Upstream spells "allow every tag" as allowedTags:false; the Go port spells
//     it as the single entry "*".
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/malcolmston/express/sanitizehtml"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID string `json:"id"`
	OK bool   `json:"ok"`
	// Value is never omitempty: an empty-string result must still serialize as
	// "value":"" so it compares equal to the upstream reply.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// rawOptions mirrors the sanitize-html options object. Every field is a pointer
// so that "absent" is distinguishable from "present but empty" — the difference
// between keeping the default and overriding it with nothing.
type rawOptions struct {
	AllowedTags                       *json.RawMessage     `json:"allowedTags"`
	AllowedAttributes                 *map[string][]string `json:"allowedAttributes"`
	AllowedClasses                    *map[string][]string `json:"allowedClasses"`
	AllowedSchemes                    *[]string            `json:"allowedSchemes"`
	AllowedSchemesByTag               *map[string][]string `json:"allowedSchemesByTag"`
	AllowedSchemesAppliedToAttributes *[]string            `json:"allowedSchemesAppliedToAttributes"`
	AllowProtocolRelative             *bool                `json:"allowProtocolRelative"`
	NonTextTags                       *[]string            `json:"nonTextTags"`
	SelfClosing                       *[]string            `json:"selfClosing"`
	AllowedEmptyAttributes            *[]string            `json:"allowedEmptyAttributes"`
	NonBooleanAttributes              *[]string            `json:"nonBooleanAttributes"`
	DisallowedTagsMode                *string              `json:"disallowedTagsMode"`
	TextFilterKind                    *string              `json:"textFilterKind"`
}

// textFilters are the named text filters a case may select. A function cannot
// travel as JSON, so both runners implement the same set by name.
var textFilters = map[string]func(text, tagName string) string{
	"upper": func(text, _ string) string { return strings.ToUpper(text) },
	"brackets": func(text, tagName string) string {
		if tagName == "" {
			tagName = "root"
		}
		return "[" + tagName + ":" + text + "]"
	},
	"drop":   func(string, string) string { return "" },
	"markup": func(string, string) string { return "<script>alert(1)</script>" },
}

// buildOptions turns the JSON options object into sanitizehtml.Options, starting
// from the defaults exactly as upstream's Object.assign does.
func buildOptions(args []json.RawMessage, i int) (sanitizehtml.Options, error) {
	opts := sanitizehtml.DefaultOptions()
	if i >= len(args) || string(args[i]) == "null" {
		return opts, nil
	}
	var raw rawOptions
	if err := json.Unmarshal(args[i], &raw); err != nil {
		return opts, fmt.Errorf("argument %d is not an options object: %w", i, err)
	}

	if raw.AllowedTags != nil {
		body := strings.TrimSpace(string(*raw.AllowedTags))
		switch body {
		case "false":
			// Upstream's "allow every tag"; the port's spelling is "*".
			opts.AllowedTags = []string{"*"}
		case "null":
			opts.AllowedTags = nil
		default:
			var tags []string
			if err := json.Unmarshal(*raw.AllowedTags, &tags); err != nil {
				return opts, fmt.Errorf("allowedTags: %w", err)
			}
			opts.AllowedTags = tags
		}
	}
	if raw.AllowedAttributes != nil {
		opts.AllowedAttributes = *raw.AllowedAttributes
	}
	if raw.AllowedClasses != nil {
		opts.AllowedClasses = *raw.AllowedClasses
	}
	if raw.AllowedSchemes != nil {
		opts.AllowedSchemes = *raw.AllowedSchemes
	}
	if raw.AllowedSchemesByTag != nil {
		opts.AllowedSchemesByTag = *raw.AllowedSchemesByTag
	}
	if raw.AllowedSchemesAppliedToAttributes != nil {
		opts.AllowedSchemesAppliedToAttributes = *raw.AllowedSchemesAppliedToAttributes
	}
	if raw.AllowProtocolRelative != nil {
		opts.AllowProtocolRelative = raw.AllowProtocolRelative
	}
	if raw.NonTextTags != nil {
		opts.NonTextTags = *raw.NonTextTags
	}
	if raw.SelfClosing != nil {
		opts.SelfClosing = *raw.SelfClosing
	}
	if raw.AllowedEmptyAttributes != nil {
		opts.AllowedEmptyAttributes = *raw.AllowedEmptyAttributes
	}
	if raw.NonBooleanAttributes != nil {
		opts.NonBooleanAttributes = *raw.NonBooleanAttributes
	}
	if raw.DisallowedTagsMode != nil {
		opts.DisallowedTagsMode = *raw.DisallowedTagsMode
	}
	if raw.TextFilterKind != nil {
		f, ok := textFilters[*raw.TextFilterKind]
		if !ok {
			return opts, fmt.Errorf("unknown textFilterKind: %s", *raw.TextFilterKind)
		}
		opts.TextFilter = f
	}
	return opts, nil
}

func argString(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	if string(args[i]) == "null" {
		return "", fmt.Errorf("argument %d is null, not a string", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %w", i, err)
	}
	return s, nil
}

func sorted(v []string) []string {
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "sanitize":
		html, err := argString(req.Args, 0)
		if err != nil {
			return nil, err
		}
		opts, err := buildOptions(req.Args, 1)
		if err != nil {
			return nil, err
		}
		return sanitizehtml.Sanitize(html, opts), nil

	case "defaultAllowedTags":
		return sorted(sanitizehtml.DefaultOptions().AllowedTags), nil
	case "defaultAllowedAttributes":
		out := map[string][]string{}
		for k, v := range sanitizehtml.DefaultOptions().AllowedAttributes {
			out[k] = sorted(v)
		}
		return out, nil
	case "defaultSelfClosing":
		return sorted(sanitizehtml.DefaultSelfClosing), nil
	case "defaultAllowedSchemes":
		return sorted(sanitizehtml.DefaultSchemes), nil
	case "defaultAllowedSchemesAppliedToAttributes":
		return sorted(sanitizehtml.DefaultSchemeAttributes), nil
	case "defaultNonTextTags":
		return sorted(sanitizehtml.DefaultNonTextTags), nil
	case "defaultDisallowedTagsMode":
		return sanitizehtml.DefaultOptions().DisallowedTagsMode, nil
	case "defaultAllowProtocolRelative":
		// A nil pointer means true, the upstream default.
		p := sanitizehtml.DefaultOptions().AllowProtocolRelative
		return p == nil || *p, nil

	default:
		return nil, fmt.Errorf("unknown fn: %s", req.Fn)
	}
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
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
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		resp := response{ID: req.ID}
		if v, err := dispatch(req); err != nil {
			resp.OK, resp.Error = false, err.Error()
		} else {
			resp.OK, resp.Value = true, v
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
		}
		// Flush per line or the harness deadlocks waiting for the reply.
		_ = out.Flush()
	}
}
