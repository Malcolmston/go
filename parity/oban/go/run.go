// Command run is the Go side of the oban cross-language parity harness.
//
// It speaks JSON Lines on stdio: one request object per line in, one response
// object per line out, in the same order. A failing case is reported as
// {"ok":false,"error":...} and never terminates the process.
//
// Only database-free parts of the port are exercised: cron parsing/evaluation
// (oban.Schedule), the retry backoff policies, and oban.NewJob normalisation.
// See ../COVERAGE.md for what cannot be compared without PostgreSQL.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/malcolmston/oban"
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

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			writeLine(out, response{OK: false, Error: "bad request line: " + err.Error()})
			continue
		}
		writeLine(out, handle(req))
	}
	out.Flush()
}

func writeLine(out *bufio.Writer, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(response{ID: resp.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}

func handle(req request) (resp response) {
	resp.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()

	value, err := dispatch(req.Fn, req.Args)
	if err != nil {
		return response{ID: req.ID, OK: false, Error: err.Error()}
	}
	return response{ID: req.ID, OK: true, Value: value}
}

func dispatch(fn string, args []json.RawMessage) (any, error) {
	switch fn {
	// ------------------------------------------------------------ cron ----

	// oban.ParseCronSpec
	case "cron_parse":
		spec, err := argString(args, 0)
		if err != nil {
			return nil, err
		}
		if _, err := oban.ParseCronSpec(spec); err != nil {
			return nil, err
		}
		return map[string]any{"valid": true}, nil

	// (*oban.Schedule).Matches
	case "cron_now":
		sched, ref, err := schedAndRef(args)
		if err != nil {
			return nil, err
		}
		return sched.Matches(ref), nil

	// (*oban.Schedule).Next
	case "cron_next":
		sched, ref, err := schedAndRef(args)
		if err != nil {
			return nil, err
		}
		return iso(sched.Next(ref)), nil

	// (*oban.Schedule).Prev
	case "cron_prev":
		sched, ref, err := schedAndRef(args)
		if err != nil {
			return nil, err
		}
		return iso(sched.Prev(ref)), nil

	// (*oban.Schedule).Upcoming
	case "cron_upcoming":
		sched, ref, err := schedAndRef(args)
		if err != nil {
			return nil, err
		}
		n, err := argInt(args, 2)
		if err != nil {
			return nil, err
		}
		times := sched.Upcoming(ref, n)
		out := make([]string, 0, len(times))
		for _, t := range times {
			out = append(out, iso(t))
		}
		return out, nil

	// --------------------------------------------------------- backoff ----

	// No counterpart: the port has no standalone exponential helper taking
	// mult/max_pow/min_pad.
	case "backoff_exponential":
		return nil, fmt.Errorf("oban-parity: no Go counterpart for Oban.Backoff.exponential/2")

	// Default retry policy used by oban.New when Config.Backoff is nil,
	// expressed in whole seconds. The port's default has no jitter, so this is
	// the whole value rather than just a deterministic component.
	case "worker_backoff_base":
		attempt, err := argInt(args, 0)
		if err != nil {
			return nil, err
		}
		// max_attempts (args[1]) has no effect: the port does not clamp the
		// attempt against max_attempts the way Oban.Worker.backoff/1 does.
		if _, err := argInt(args, 1); err != nil {
			return nil, err
		}
		b := &oban.ExponentialBackoff{}
		return int64(b.Next(attempt) / time.Second), nil

	case "worker_backoff_jitter":
		return "none", nil

	case "worker_backoff_in_bounds":
		attempt, err := argInt(args, 0)
		if err != nil {
			return nil, err
		}
		if _, err := argInt(args, 1); err != nil {
			return nil, err
		}
		b := &oban.ExponentialBackoff{}
		base := int64(b.Next(attempt) / time.Second)
		actual := int64((&oban.ExponentialBackoff{}).Next(attempt) / time.Second)
		return actual >= base && actual <= base+base/10, nil

	// ------------------------------------------------------------- job ----

	// oban.NewJob
	case "job_new":
		worker, err := argString(args, 0)
		if err != nil {
			return nil, err
		}
		var jobArgs map[string]any
		if err := argJSON(args, 1, &jobArgs); err != nil {
			return nil, err
		}
		var opts map[string]any
		if err := argJSON(args, 2, &opts); err != nil {
			return nil, err
		}
		jobOpts, err := jobOptions(opts)
		if err != nil {
			return nil, err
		}
		job, err := oban.NewJob(worker, jobArgs, jobOpts...)
		if err != nil {
			return nil, err
		}
		var decoded map[string]any
		if err := job.UnmarshalArgs(&decoded); err != nil {
			return nil, err
		}
		return map[string]any{
			"queue":        job.Queue,
			"worker":       job.Worker,
			"max_attempts": job.MaxAttempts,
			"priority":     job.Priority,
			"state":        string(job.State),
			"args":         canon(decoded),
		}, nil
	}

	return nil, fmt.Errorf("oban-parity: unsupported fn %q", fn)
}

// jobOptions translates the language-agnostic option object into JobOptions,
// rejecting anything the case file has no mapping for.
func jobOptions(opts map[string]any) ([]oban.JobOption, error) {
	// Deterministic application order, since options are applied in sequence.
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []oban.JobOption
	for _, k := range keys {
		v := opts[k]
		switch k {
		case "queue":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("oban-parity: queue must be a string")
			}
			out = append(out, oban.WithQueue(s))
		case "max_attempts":
			n, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("oban-parity: max_attempts must be a number")
			}
			out = append(out, oban.WithMaxAttempts(int(n)))
		case "priority":
			n, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("oban-parity: priority must be a number")
			}
			// Deliberately not calling oban.ValidatePriority here: the point
			// of the case is what oban.NewJob alone accepts, versus what
			// Oban.Job.new/2 accepts.
			out = append(out, oban.WithPriority(int(n)))
		case "schedule_in":
			n, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("oban-parity: schedule_in must be a number of seconds")
			}
			out = append(out, oban.WithScheduleIn(time.Duration(n)*time.Second))
		case "state":
			return nil, fmt.Errorf("oban-parity: the port has no state option on NewJob")
		default:
			return nil, fmt.Errorf("oban-parity: unknown job opt %q", k)
		}
	}
	return out, nil
}

// canon renders a decoded JSON value as sorted [key, value] pairs so that map
// ordering cannot differ between the two runners.
func canon(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, k := range keys {
			out = append(out, []any{k, canon(t[k])})
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, canon(e))
		}
		return out
	default:
		return v
	}
}

func schedAndRef(args []json.RawMessage) (*oban.Schedule, time.Time, error) {
	spec, err := argString(args, 0)
	if err != nil {
		return nil, time.Time{}, err
	}
	ref, err := argString(args, 1)
	if err != nil {
		return nil, time.Time{}, err
	}
	at, err := time.Parse(time.RFC3339, ref)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("oban-parity: bad reference instant %q: %w", ref, err)
	}
	sched, err := oban.ParseCronSpec(spec)
	if err != nil {
		return nil, time.Time{}, err
	}
	return sched, at.UTC(), nil
}

// iso renders a fire time as UTC ISO-8601 with second precision, matching the
// upstream runner. A zero time means "no match inside the search horizon".
func iso(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func argString(args []json.RawMessage, i int) (string, error) {
	var s string
	if err := argJSON(args, i, &s); err != nil {
		return "", err
	}
	return s, nil
}

func argInt(args []json.RawMessage, i int) (int, error) {
	var f float64
	if err := argJSON(args, i, &f); err != nil {
		return 0, err
	}
	return int(f), nil
}

func argJSON(args []json.RawMessage, i int, dst any) error {
	if i >= len(args) {
		return fmt.Errorf("oban-parity: missing argument %d", i)
	}
	if err := json.Unmarshal(args[i], dst); err != nil {
		return fmt.Errorf("oban-parity: argument %d: %w", i, err)
	}
	return nil
}
