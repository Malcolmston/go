// Command run is the Go-side runner for the quartz parity harness.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. It never exits on a failing
// case -- errors and panics are reported as {"ok":false,"error":...}.
//
// Every fire-time computation is a pure function of an explicitly supplied
// instant and time zone. Nothing reads the wall clock and no Scheduler is
// started.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/malcolmston/quartz"
)

type request struct {
	ID   string `json:"id"`
	Fn   string `json:"fn"`
	Args []any  `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// outLayout is the single fixed wire format for every emitted instant: UTC at
// second precision, with an explicit offset so the string is unambiguous.
const outLayout = "2006-01-02T15:04:05"

func fmtTime(t time.Time) string { return t.UTC().Format(outLayout) + "+00:00" }

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		resp := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp = response{ID: "?", OK: false, Error: "bad request: " + err.Error()}
		} else {
			resp.ID = req.ID
			v, err := call(req.Fn, req.Args)
			if err != nil {
				resp.OK = false
				resp.Error = err.Error()
			} else {
				resp.OK = true
				resp.Value = v
			}
		}
		enc, err := json.Marshal(resp)
		if err != nil {
			enc = []byte(`{"id":"` + resp.ID + `","ok":false,"error":"unencodable value"}`)
		}
		out.Write(enc)
		out.WriteByte('\n')
		out.Flush()
	}
}

// call dispatches one case, converting a panic into an error so a single bad
// case can never take the runner down.
func call(fn string, args []any) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			v = nil
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(fn, args)
}

func dispatch(fn string, a []any) (any, error) {
	switch fn {
	case "cronParse":
		s, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		return cronParse(s)
	case "cronNext":
		return withArgs(a, 4, func() (any, error) {
			expr, _ := str(a, 0)
			tz, _ := str(a, 1)
			after, _ := str(a, 2)
			n, _ := intAt(a, 3)
			return cronNext(expr, tz, after, n)
		})
	case "cronSatisfiedBy":
		return withArgs(a, 3, func() (any, error) {
			expr, _ := str(a, 0)
			tz, _ := str(a, 1)
			ts, _ := str(a, 2)
			return cronSatisfiedBy(expr, tz, ts)
		})
	case "cronTriggerNext":
		return withArgs(a, 5, func() (any, error) {
			expr, _ := str(a, 0)
			tz, _ := str(a, 1)
			start, _ := str(a, 2)
			after, _ := str(a, 3)
			n, _ := intAt(a, 4)
			return cronTriggerNext(expr, tz, start, after, n)
		})
	case "simpleNext":
		return withArgs(a, 6, func() (any, error) {
			start, _ := str(a, 0)
			ms, _ := intAt(a, 1)
			repeat, _ := intAt(a, 2)
			end := nullableStr(a, 3)
			after, _ := str(a, 4)
			n, _ := intAt(a, 5)
			return simpleNext(start, int64(ms), repeat, end, after, n)
		})
	case "calendarIntervalNext":
		return withArgs(a, 7, func() (any, error) {
			tz, _ := str(a, 0)
			start, _ := str(a, 1)
			unit, _ := str(a, 2)
			count, _ := intAt(a, 3)
			preserve := boolAt(a, 4)
			after, _ := str(a, 5)
			n, _ := intAt(a, 6)
			return calendarIntervalNext(tz, start, unit, count, preserve, after, n)
		})
	case "dailyTimeIntervalNext":
		return withArgs(a, 9, func() (any, error) {
			tz, _ := str(a, 0)
			start, _ := str(a, 1)
			startTod, _ := str(a, 2)
			endTod, _ := str(a, 3)
			unit, _ := str(a, 4)
			count, _ := intAt(a, 5)
			days, _ := str(a, 6)
			after, _ := str(a, 7)
			n, _ := intAt(a, 8)
			return dailyTimeIntervalNext(tz, start, startTod, endTod, unit, count, days, after, n)
		})
	case "misfireIgnoreNext":
		kind, err := str(a, 0)
		if err != nil {
			return nil, err
		}
		return misfireIgnoreNext(kind, a)
	default:
		return nil, fmt.Errorf("unknown fn: %s", fn)
	}
}

// withArgs validates the arity up front, then runs body; the inner accessors
// are known-safe once the arity and types have been checked.
func withArgs(a []any, want int, body func() (any, error)) (any, error) {
	if len(a) < want {
		return nil, fmt.Errorf("expected %d args, got %d", want, len(a))
	}
	return body()
}

// ---------------------------------------------------------------------- args

func str(a []any, i int) (string, error) {
	if i >= len(a) {
		return "", fmt.Errorf("missing string arg %d", i)
	}
	s, ok := a[i].(string)
	if !ok {
		return "", fmt.Errorf("arg %d is not a string", i)
	}
	return s, nil
}

func nullableStr(a []any, i int) string {
	if i >= len(a) || a[i] == nil {
		return ""
	}
	s, _ := a[i].(string)
	return s
}

func intAt(a []any, i int) (int, error) {
	if i >= len(a) {
		return 0, fmt.Errorf("missing int arg %d", i)
	}
	f, ok := a[i].(float64)
	if !ok {
		return 0, fmt.Errorf("arg %d is not a number", i)
	}
	return int(f), nil
}

func boolAt(a []any, i int) bool {
	if i >= len(a) {
		return false
	}
	b, _ := a[i].(bool)
	return b
}

// ---------------------------------------------------------------- conversion

// instant parses an explicit UTC RFC-3339 instant. There is deliberately no
// "now" path anywhere in this runner.
func instant(iso string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad instant %q: %w", iso, err)
	}
	return t.UTC(), nil
}

func zone(id string) (*time.Location, error) {
	loc, err := time.LoadLocation(id)
	if err != nil {
		return nil, fmt.Errorf("unknown time zone: %s", id)
	}
	return loc, nil
}

func unit(name string) (quartz.IntervalUnit, error) {
	switch strings.ToUpper(name) {
	case "SECOND":
		return quartz.IntervalSecond, nil
	case "MINUTE":
		return quartz.IntervalMinute, nil
	case "HOUR":
		return quartz.IntervalHour, nil
	case "DAY":
		return quartz.IntervalDay, nil
	case "WEEK":
		return quartz.IntervalWeek, nil
	case "MONTH":
		return quartz.IntervalMonth, nil
	case "YEAR":
		return quartz.IntervalYear, nil
	default:
		return 0, fmt.Errorf("unsupported interval unit: %s", name)
	}
}

func weekdays(csv string) ([]time.Weekday, error) {
	all := []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday}
	if csv == "" || csv == "*" {
		return all, nil
	}
	var out []time.Weekday
	for _, part := range strings.Split(csv, ",") {
		switch strings.ToUpper(strings.TrimSpace(part)) {
		case "SUN":
			out = append(out, time.Sunday)
		case "MON":
			out = append(out, time.Monday)
		case "TUE":
			out = append(out, time.Tuesday)
		case "WED":
			out = append(out, time.Wednesday)
		case "THU":
			out = append(out, time.Thursday)
		case "FRI":
			out = append(out, time.Friday)
		case "SAT":
			out = append(out, time.Saturday)
		default:
			return nil, fmt.Errorf("bad weekday: %s", part)
		}
	}
	return out, nil
}

func timeOfDay(hhmmss string) (quartz.TimeOfDay, error) {
	p := strings.Split(hhmmss, ":")
	if len(p) != 3 {
		return quartz.TimeOfDay{}, fmt.Errorf("bad time-of-day: %s", hhmmss)
	}
	n := make([]int, 3)
	for i, s := range p {
		v, err := strconv.Atoi(s)
		if err != nil {
			return quartz.TimeOfDay{}, fmt.Errorf("bad time-of-day: %s", hhmmss)
		}
		n[i] = v
	}
	return quartz.NewTimeOfDay(n[0], n[1], n[2]), nil
}

var (
	trigKey = quartz.NewKeyInGroup("t", "g")
	jobKey  = quartz.NewKeyInGroup("j", "g")
)

// chain walks a pure "first fire strictly after x" function n times, stopping
// early on the zero time.
func chain(step func(time.Time) time.Time, after time.Time, n int) []string {
	out := []string{}
	cur := after
	for i := 0; i < n; i++ {
		next := step(cur)
		if next.IsZero() {
			break
		}
		out = append(out, fmtTime(next))
		cur = next
	}
	return out
}

// ----------------------------------------------------------------------- fns

// quartz.ParseCron -- accepted or rejected.
func cronParse(expr string) (any, error) {
	if _, err := quartz.ParseCron(expr); err != nil {
		return nil, err
	}
	return true, nil
}

// quartz.CronExpression.Next, chained n times.
func cronNext(expr, tz, afterISO string, n int) (any, error) {
	ce, err := quartz.ParseCron(expr)
	if err != nil {
		return nil, err
	}
	loc, err := zone(tz)
	if err != nil {
		return nil, err
	}
	after, err := instant(afterISO)
	if err != nil {
		return nil, err
	}
	return chain(func(t time.Time) time.Time { return ce.Next(t.In(loc)) }, after, n), nil
}

// quartz.CronExpression.IsSatisfiedBy.
func cronSatisfiedBy(expr, tz, iso string) (any, error) {
	ce, err := quartz.ParseCron(expr)
	if err != nil {
		return nil, err
	}
	loc, err := zone(tz)
	if err != nil {
		return nil, err
	}
	t, err := instant(iso)
	if err != nil {
		return nil, err
	}
	return ce.IsSatisfiedBy(t.In(loc)), nil
}

// quartz.CronTrigger: ComputeFirstFireTime then Triggered, which together are
// the port's equivalent of CronTriggerImpl.getFireTimeAfter chained.
func cronTriggerNext(expr, tz, startISO, afterISO string, n int) (any, error) {
	loc, err := zone(tz)
	if err != nil {
		return nil, err
	}
	start, err := instant(startISO)
	if err != nil {
		return nil, err
	}
	after, err := instant(afterISO)
	if err != nil {
		return nil, err
	}
	out := []string{}
	tr, err := quartz.NewCronTriggerWithKeys(trigKey, jobKey, expr)
	if err != nil {
		return nil, err
	}
	tr.In(loc).StartingAt(start)
	next := tr.ComputeFirstFireTime(after)
	for i := 0; i < n && !next.IsZero(); i++ {
		out = append(out, fmtTime(next))
		next = tr.Triggered(next)
	}
	return out, nil
}

// quartz.SimpleTrigger.FireTimeAfter, chained.
func simpleNext(startISO string, intervalMillis int64, repeatCount int, endISO, afterISO string, n int) (any, error) {
	start, err := instant(startISO)
	if err != nil {
		return nil, err
	}
	after, err := instant(afterISO)
	if err != nil {
		return nil, err
	}
	tr := quartz.NewSimpleTrigger(trigKey, jobKey, start,
		time.Duration(intervalMillis)*time.Millisecond, repeatCount)
	if endISO != "" {
		end, err := instant(endISO)
		if err != nil {
			return nil, err
		}
		tr.WithEndTime(end)
	}
	return chain(tr.FireTimeAfter, after, n), nil
}

// quartz.CalendarIntervalTrigger.FireTimeAfter, chained.
func calendarIntervalNext(tz, startISO, unitName string, count int, preserveWallClock bool, afterISO string, n int) (any, error) {
	loc, err := zone(tz)
	if err != nil {
		return nil, err
	}
	u, err := unit(unitName)
	if err != nil {
		return nil, err
	}
	start, err := instant(startISO)
	if err != nil {
		return nil, err
	}
	after, err := instant(afterISO)
	if err != nil {
		return nil, err
	}
	tr := quartz.NewCalendarIntervalTrigger(trigKey, jobKey, start, u, count)
	tr.In(loc).PreserveWallClock(preserveWallClock)
	return chain(tr.FireTimeAfter, after, n), nil
}

// quartz.DailyTimeIntervalTrigger has no pure FireTimeAfter, so the sequence is
// produced with ComputeFirstFireTime plus Triggered. Because every fire time is
// at whole-second granularity, seeding with after+1s is exactly "strictly after
// after".
func dailyTimeIntervalNext(tz, startISO, startTod, endTod, unitName string, count int, days, afterISO string, n int) (any, error) {
	loc, err := zone(tz)
	if err != nil {
		return nil, err
	}
	u, err := unit(unitName)
	if err != nil {
		return nil, err
	}
	start, err := instant(startISO)
	if err != nil {
		return nil, err
	}
	after, err := instant(afterISO)
	if err != nil {
		return nil, err
	}
	sod, err := timeOfDay(startTod)
	if err != nil {
		return nil, err
	}
	eod, err := timeOfDay(endTod)
	if err != nil {
		return nil, err
	}
	wd, err := weekdays(days)
	if err != nil {
		return nil, err
	}
	tr := quartz.NewDailyTimeIntervalTrigger(trigKey, jobKey, sod, eod, u, count)
	tr.OnDaysOfWeek(wd...).In(loc).StartingAt(start)

	out := []string{}
	next := tr.ComputeFirstFireTime(after.Add(time.Second))
	for i := 0; i < n && !next.IsZero(); i++ {
		out = append(out, fmtTime(next))
		next = tr.Triggered(next)
	}
	return out, nil
}

// misfireIgnoreNext exercises the one misfire instruction whose handling reads
// no clock on either side: "ignore" must leave the computed next fire time
// untouched. Every other instruction routes through upstream's
// updateAfterMisfire, which is implemented against new Date() and therefore is
// not deterministically comparable (see COVERAGE.md).
func misfireIgnoreNext(kind string, a []any) (any, error) {
	switch kind {
	case "cron":
		tz, err := str(a, 1)
		if err != nil {
			return nil, err
		}
		expr, err := str(a, 2)
		if err != nil {
			return nil, err
		}
		startISO, err := str(a, 3)
		if err != nil {
			return nil, err
		}
		loc, err := zone(tz)
		if err != nil {
			return nil, err
		}
		start, err := instant(startISO)
		if err != nil {
			return nil, err
		}
		tr, err := quartz.NewCronTriggerWithKeys(trigKey, jobKey, expr)
		if err != nil {
			return nil, err
		}
		tr.In(loc).StartingAt(start).WithMisfirePolicy(quartz.MisfireIgnore)
		// Upstream's computeFirstFireTime searches from startTime-1s so the
		// start instant itself can be the first fire; mirror that.
		tr.ComputeFirstFireTime(start.Add(-time.Second))
		next := tr.UpdateAfterMisfire(start)
		if next.IsZero() {
			return nil, nil
		}
		return fmtTime(next), nil
	case "simple":
		startISO, err := str(a, 1)
		if err != nil {
			return nil, err
		}
		ms, err := intAt(a, 2)
		if err != nil {
			return nil, err
		}
		repeat, err := intAt(a, 3)
		if err != nil {
			return nil, err
		}
		start, err := instant(startISO)
		if err != nil {
			return nil, err
		}
		tr := quartz.NewSimpleTrigger(trigKey, jobKey, start,
			time.Duration(ms)*time.Millisecond, repeat)
		tr.WithMisfirePolicy(quartz.MisfireIgnore)
		tr.ComputeFirstFireTime(start)
		next := tr.UpdateAfterMisfire(start)
		if next.IsZero() {
			return nil, nil
		}
		return fmtTime(next), nil
	default:
		return nil, fmt.Errorf("unknown misfire kind: %s", kind)
	}
}
