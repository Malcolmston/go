// Command run is the Go-side parity runner for github.com/malcolmston/rrule.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Every case runs inside a
// deferred recover(), so an error *or a panic* inside the port becomes
// {"ok": false, "error": "..."} and the runner keeps reading. That matters
// here: the published port panics on some inputs (a huge INTERVAL), and a dead
// runner would score every remaining case as a loss.
//
// Nothing but response lines goes to stdout; diagnostics go to stderr.
//
// No case may depend on "now": every request carries an explicit DTSTART, and
// occurrences are emitted as fixed-format ISO-8601 strings so the harness can
// compare them byte for byte against dateutil.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/malcolmston/rrule"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// spec is the single object argument every case carries. Unused fields are
// simply absent for a given op.
type spec struct {
	Rule    string `json:"rule"`
	DTStart string `json:"dtstart"`
	TZ      string `json:"tz"`
	Limit   int    `json:"limit"`

	After  string `json:"after"`
	Before string `json:"before"`
	Inc    bool   `json:"inc"`

	Text    string   `json:"text"`
	RRules  []string `json:"rrules"`
	ExRules []string `json:"exrules"`
	RDates  []string `json:"rdates"`
	ExDates []string `json:"exdates"`
}

// ------------------------------------------------------------------- times

// loc resolves the case's timezone. An absent tz means "naive": upstream uses
// a tzinfo-less datetime, which the port models as UTC, and both sides then
// render the instant without an offset.
func (s spec) loc() (*time.Location, bool, error) {
	if s.TZ == "" {
		return time.UTC, true, nil
	}
	l, err := time.LoadLocation(s.TZ)
	if err != nil {
		return nil, false, fmt.Errorf("unknown timezone %q: %w", s.TZ, err)
	}
	return l, false, nil
}

const naiveLayout = "2006-01-02T15:04:05"

// parseInstant reads "YYYY-MM-DDTHH:MM:SS" in loc. Strict: no defaults, no
// "now" fallback.
func parseInstant(v string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(naiveLayout, v, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad instant %q: %w", v, err)
	}
	return t, nil
}

// format renders an occurrence exactly as the Python runner does: a numeric
// +HH:MM offset for aware instants, none for naive ones.
func format(t time.Time, naive bool) string {
	if naive {
		return t.Format(naiveLayout)
	}
	return t.Format("2006-01-02T15:04:05-07:00")
}

// ------------------------------------------------------------------- rules

// compile parses a bare RRULE value and re-anchors it at the case's DTSTART.
//
// The port's Parse has no dtstart parameter (an absent DTSTART silently means
// "now"), so the rule is parsed, its Options are read back, DTStart is
// replaced and the rule is rebuilt. That is the exact analogue of upstream's
// rrulestr(value, dtstart=...) and keeps "now" out of every case.
func compile(value, dtstart string, loc *time.Location) (*rrule.RRule, error) {
	r, err := rrule.Parse(value)
	if err != nil {
		return nil, err
	}
	start, err := parseInstant(dtstart, loc)
	if err != nil {
		return nil, err
	}
	opts := r.Options()
	opts.DTStart = start
	return rrule.New(opts)
}

// takeRule pulls at most limit occurrences. Iterator is used rather than All
// so an unbounded rule is capped by the case, not materialised.
func takeRule(r *rrule.RRule, limit int, naive bool) []string {
	return drain(r.Iterator(), limit, naive)
}

func drain(next func() (time.Time, bool), limit int, naive bool) []string {
	out := []string{}
	for i := 0; i < limit; i++ {
		t, ok := next()
		if !ok {
			break
		}
		out = append(out, format(t, naive))
	}
	return out
}

// ruleParts normalises the RRULE line of a serialised rule: rule parts
// uppercased and sorted, so ordering is not what is being compared. The
// DTSTART line is ignored -- upstream's serialisation is zone-blind and that
// difference is recorded in COVERAGE.md instead.
func ruleParts(text string) (string, error) {
	value := ""
	found := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "RRULE:") {
			value = line[len("RRULE:"):]
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("no RRULE line in serialised rule: %q", text)
	}
	var parts []string
	for _, p := range strings.Split(value, ";") {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			parts = append(parts, p)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ";"), nil
}

// --------------------------------------------------------------------- ops

func opExpand(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	return takeRule(r, s.Limit, naive), nil
}

func opCount(s spec) (any, error) {
	loc, _, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	return len(r.All()), nil
}

func opBetween(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	after, err := parseInstant(s.After, loc)
	if err != nil {
		return nil, err
	}
	before, err := parseInstant(s.Before, loc)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, t := range r.Between(after, before, s.Inc) {
		out = append(out, format(t, naive))
	}
	return out, nil
}

func opAfter(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	t, err := parseInstant(s.After, loc)
	if err != nil {
		return nil, err
	}
	got := r.After(t, s.Inc)
	if got.IsZero() {
		return nil, nil
	}
	return format(got, naive), nil
}

func opBefore(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	t, err := parseInstant(s.Before, loc)
	if err != nil {
		return nil, err
	}
	got := r.Before(t, s.Inc)
	if got.IsZero() {
		return nil, nil
	}
	return format(got, naive), nil
}

func opRuleString(s spec) (any, error) {
	loc, _, err := s.loc()
	if err != nil {
		return nil, err
	}
	r, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	return ruleParts(r.String())
}

func opRoundTrip(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	first, err := compile(s.Rule, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	value := ""
	for _, line := range strings.Split(first.String(), "\n") {
		if strings.HasPrefix(strings.ToUpper(line), "RRULE:") {
			value = line[len("RRULE:"):]
		}
	}
	if value == "" {
		return nil, fmt.Errorf("no RRULE line in serialised rule")
	}
	again, err := compile(value, s.DTStart, loc)
	if err != nil {
		return nil, err
	}
	return takeRule(again, s.Limit, naive), nil
}

func opSet(s spec) (any, error) {
	_, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	set, err := rrule.StrToSet(s.Text)
	if err != nil {
		return nil, err
	}
	return drain(set.Iterator(), s.Limit, naive), nil
}

func opSetBuild(s spec) (any, error) {
	loc, naive, err := s.loc()
	if err != nil {
		return nil, err
	}
	set := rrule.NewSet()
	for _, value := range s.RRules {
		r, err := compile(value, s.DTStart, loc)
		if err != nil {
			return nil, err
		}
		set.RRule(r)
	}
	for _, value := range s.ExRules {
		r, err := compile(value, s.DTStart, loc)
		if err != nil {
			return nil, err
		}
		set.ExRule(r)
	}
	for _, d := range s.RDates {
		t, err := parseInstant(d, loc)
		if err != nil {
			return nil, err
		}
		set.RDate(t)
	}
	for _, d := range s.ExDates {
		t, err := parseInstant(d, loc)
		if err != nil {
			return nil, err
		}
		set.ExDate(t)
	}
	return drain(set.Iterator(), s.Limit, naive), nil
}

var ops = map[string]func(spec) (any, error){
	"expand":     opExpand,
	"count":      opCount,
	"between":    opBetween,
	"after":      opAfter,
	"before":     opBefore,
	"rulestring": opRuleString,
	"roundtrip":  opRoundTrip,
	"set":        opSet,
	"setbuild":   opSetBuild,
}

// handle runs one case. The deferred recover is the whole reason a panicking
// port does not take the suite down with it.
func handle(req request) (value any, err error) {
	defer func() {
		if p := recover(); p != nil {
			value = nil
			err = fmt.Errorf("panic: %v", p)
		}
	}()

	op, ok := ops[req.Fn]
	if !ok {
		return nil, fmt.Errorf("unknown fn: %q", req.Fn)
	}
	if len(req.Args) != 1 {
		return nil, fmt.Errorf("fn %s takes exactly one object argument", req.Fn)
	}
	var s spec
	if err := json.Unmarshal(req.Args[0], &s); err != nil {
		return nil, fmt.Errorf("bad spec: %w", err)
	}
	return op(s)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		resp := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp = response{OK: false, Error: "bad request: " + err.Error()}
		} else {
			resp.ID = req.ID
			value, err := handle(req)
			if err != nil {
				resp.OK, resp.Error = false, err.Error()
			} else {
				resp.OK, resp.Value = true, value
			}
		}
		enc, err := json.Marshal(resp)
		if err != nil {
			enc, _ = json.Marshal(response{ID: req.ID, OK: false, Error: "encode: " + err.Error()})
		}
		out.Write(enc)
		out.WriteByte('\n')
		// Flush every line or the harness deadlocks waiting for the reply.
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			return
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}
}
