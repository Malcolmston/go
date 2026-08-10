// Command run is the Go-side runner for the `moment` parity harness.
//
// It speaks the same JSON Lines protocol as node/run.js: one request object per
// line on stdin, exactly one response object per line on stdout, same order.
// A panic or error inside an operation is reported as {"ok":false,...} and the
// loop keeps going, so one bad case never costs the rest of the run.
//
// Determinism: no operation ever consults the wall clock.  Relative-time
// operations take their reference instant from the case file, and the process
// is expected to run under TZ=UTC.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/malcolmston/moment"
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

// ---------------------------------------------------------------- arg helpers

func argStr(args []json.RawMessage, i int) string {
	var s string
	mustUnmarshal(args, i, &s)
	return s
}

func argInt(args []json.RawMessage, i int) int {
	var f float64
	mustUnmarshal(args, i, &f)
	return int(f)
}

func argBool(args []json.RawMessage, i int) bool {
	var b bool
	mustUnmarshal(args, i, &b)
	return b
}

func argStrs(args []json.RawMessage, i int) []string {
	var v []string
	mustUnmarshal(args, i, &v)
	return v
}

func argInts(args []json.RawMessage, i int) []int {
	var v []float64
	mustUnmarshal(args, i, &v)
	out := make([]int, len(v))
	for j, f := range v {
		out[j] = int(f)
	}
	return out
}

func argIntMap(args []json.RawMessage, i int) map[string]int {
	var v map[string]float64
	mustUnmarshal(args, i, &v)
	out := make(map[string]int, len(v))
	for k, f := range v {
		out[k] = int(f)
	}
	return out
}

func mustUnmarshal(args []json.RawMessage, i int, dst any) {
	if i >= len(args) {
		panic(fmt.Sprintf("missing argument %d", i))
	}
	if err := json.Unmarshal(args[i], dst); err != nil {
		panic(fmt.Sprintf("argument %d: %v", i, err))
	}
}

// ---------------------------------------------------------------- value shapes

// instValue mirrors the node runner's instant shape: validity as a boolean and
// the instant as an ISO string ("" when invalid).  Error text is never part of
// the compared value.
func instValue(m moment.Moment, err error) any {
	if err != nil || !m.IsValid() {
		return map[string]any{"valid": false, "iso": ""}
	}
	return map[string]any{"valid": true, "iso": m.ToISOString()}
}

// num turns a non-finite float into JSON null, matching JSON.stringify(NaN).
func num(f float64) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return f
}

// at parses an ISO instant supplied by a case file, always in UTC.
func at(iso string) moment.Moment {
	m, err := moment.Parse(iso)
	if err != nil {
		panic(fmt.Sprintf("case instant %q is not parseable: %v", iso, err))
	}
	return m.UTC()
}

func unit(name string) moment.Unit {
	u, ok := moment.NormalizeUnit(name)
	if !ok {
		panic(fmt.Sprintf("unknown unit: %q", name))
	}
	return u
}

func duration(obj map[string]int) moment.Duration {
	return moment.NewDurationFromObject(obj)
}

func fmtOrInvalid(m moment.Moment, layout string) string {
	return m.Format(layout)
}

// ---------------------------------------------------------------- dispatch

func dispatch(fn string, args []json.RawMessage) any {
	switch fn {

	// ---- parsing -------------------------------------------------------
	case "parse", "parseISO":
		return instValue(moment.Parse(argStr(args, 0)))
	case "parseRFC2822":
		return instValue(moment.ParseRFC2822(argStr(args, 0)))
	case "parseFormat":
		return instValue(moment.ParseFormat(argStr(args, 0), argStr(args, 1)))
	case "parseFormatStrict":
		return instValue(moment.ParseFormatStrict(argStr(args, 0), argStr(args, 1)))
	case "parseFormats":
		return instValue(moment.ParseFormats(argStr(args, 0), argStrs(args, 1)))
	case "parseFormatLocale":
		return instValue(moment.ParseFormatLocale(argStr(args, 0), argStr(args, 1), argStr(args, 2)))
	case "parseZone":
		m, err := moment.ParseZone(argStr(args, 0))
		if err != nil || !m.IsValid() {
			return map[string]any{"valid": false, "iso": "", "offsetMinutes": float64(0), "formatted": ""}
		}
		return map[string]any{
			"valid":         true,
			"iso":           m.ToISOString(),
			"offsetMinutes": float64(m.UTCOffset()),
			"formatted":     m.Format("YYYY-MM-DDTHH:mm:ssZ"),
		}
	case "fromArray":
		return instValue(moment.FromArray(argInts(args, 0)), nil)
	case "fromObject":
		return instValue(moment.FromObject(argIntMap(args, 0)), nil)
	case "isValid":
		m, err := moment.Parse(argStr(args, 0))
		return err == nil && m.IsValid()
	case "isValidFormat":
		m, err := moment.ParseFormat(argStr(args, 0), argStr(args, 1))
		return err == nil && m.IsValid()
	case "isValidFormatStrict":
		m, err := moment.ParseFormatStrict(argStr(args, 0), argStr(args, 1))
		return err == nil && m.IsValid()

	// ---- formatting ----------------------------------------------------
	case "format":
		return fmtOrInvalid(at(argStr(args, 0)), argStr(args, 1))
	case "formatLocale":
		return at(argStr(args, 0)).Locale(argStr(args, 2)).Format(argStr(args, 1))
	case "formatAtOffset":
		return at(argStr(args, 0)).SetUTCOffset(argInt(args, 1)).Format(argStr(args, 2))
	case "toISOString":
		return at(argStr(args, 0)).ToISOString()
	case "toISOStringZone":
		return at(argStr(args, 0)).SetUTCOffset(argInt(args, 1)).ToISOStringZone()
	case "toJSON":
		return at(argStr(args, 0)).ToJSON()
	case "toArray":
		arr := at(argStr(args, 0)).ToArray()
		out := make([]any, len(arr))
		for i, v := range arr {
			out[i] = float64(v)
		}
		return out
	case "toObject":
		obj := at(argStr(args, 0)).ToObject()
		out := make(map[string]any, len(obj))
		for k, v := range obj {
			out[k] = float64(v)
		}
		return out
	case "valueOf":
		return float64(at(argStr(args, 0)).ValueOf())
	case "unix":
		return float64(at(argStr(args, 0)).Unix())

	// ---- manipulation --------------------------------------------------
	case "add":
		return instValue(at(argStr(args, 0)).Add(argInt(args, 1), unit(argStr(args, 2))), nil)
	case "subtract":
		return instValue(at(argStr(args, 0)).Subtract(argInt(args, 1), unit(argStr(args, 2))), nil)
	case "startOf":
		return instValue(at(argStr(args, 0)).StartOf(unit(argStr(args, 1))), nil)
	case "endOf":
		return instValue(at(argStr(args, 0)).EndOf(unit(argStr(args, 1))), nil)
	case "set":
		return instValue(at(argStr(args, 0)).Set(unit(argStr(args, 1)), argInt(args, 2)), nil)
	case "setDate":
		return instValue(at(argStr(args, 0)).SetDate(argInt(args, 1)), nil)
	case "setDayOfYear":
		return instValue(at(argStr(args, 0)).SetDayOfYear(argInt(args, 1)), nil)
	case "setWeekday":
		return instValue(at(argStr(args, 0)).SetWeekday(time.Weekday(argInt(args, 1))), nil)
	case "setISOWeekday":
		return instValue(at(argStr(args, 0)).SetISOWeekday(argInt(args, 1)), nil)
	case "setQuarter":
		return instValue(at(argStr(args, 0)).SetQuarter(argInt(args, 1)), nil)
	case "setYear":
		return instValue(at(argStr(args, 0)).SetYear(argInt(args, 1)), nil)
	case "setMonth1":
		return instValue(at(argStr(args, 0)).SetMonth(time.Month(argInt(args, 1))), nil)
	case "clone":
		return instValue(at(argStr(args, 0)).Clone(), nil)

	// ---- comparison ----------------------------------------------------
	case "diff":
		return num(float64(at(argStr(args, 0)).DiffInt(at(argStr(args, 1)), unit(argStr(args, 2)))))
	case "diffFloat":
		return num(at(argStr(args, 0)).Diff(at(argStr(args, 1)), unit(argStr(args, 2))))
	case "isBefore":
		return at(argStr(args, 0)).IsBefore(at(argStr(args, 1)))
	case "isAfter":
		return at(argStr(args, 0)).IsAfter(at(argStr(args, 1)))
	case "isSame":
		return at(argStr(args, 0)).IsSame(at(argStr(args, 1)))
	case "isSameOrBefore":
		return at(argStr(args, 0)).IsSameOrBefore(at(argStr(args, 1)))
	case "isSameOrAfter":
		return at(argStr(args, 0)).IsSameOrAfter(at(argStr(args, 1)))
	case "isSameUnit":
		return at(argStr(args, 0)).IsSameUnit(at(argStr(args, 1)), unit(argStr(args, 2)))
	case "isBetween":
		return at(argStr(args, 0)).IsBetween(at(argStr(args, 1)), at(argStr(args, 2)), argBool(args, 3))
	case "max":
		return instValue(moment.Max(moments(argStrs(args, 0))...), nil)
	case "min":
		return instValue(moment.Min(moments(argStrs(args, 0))...), nil)

	// ---- getters -------------------------------------------------------
	case "components":
		m := at(argStr(args, 0))
		return map[string]any{
			"year":           float64(m.Year()),
			"month0":         float64(int(m.Month()) - 1),
			"date":           float64(m.Date()),
			"hour":           float64(m.Hour()),
			"minute":         float64(m.Minute()),
			"second":         float64(m.Second()),
			"millisecond":    float64(m.Millisecond()),
			"dayOfYear":      float64(m.DayOfYear()),
			"quarter":        float64(m.Quarter()),
			"weekday":        float64(int(m.Weekday())),
			"isoWeekday":     float64(m.ISOWeekday()),
			"week":           float64(m.Week()),
			"isoWeek":        float64(m.ISOWeekNumber()),
			"weekYear":       float64(m.WeekYear()),
			"isoWeekYear":    float64(m.ISOWeekYear()),
			"daysInMonth":    float64(m.DaysInMonth()),
			"isLeapYear":     m.IsLeapYear(),
			"weeksInYear":    float64(m.WeeksInYear()),
			"isoWeeksInYear": float64(m.ISOWeeksInYear()),
			"localeWeekday":  float64(m.LocaleWeekday()),
			"utcOffset":      float64(m.UTCOffset()),
			"isUTC":          m.IsUTC(),
			"zoneAbbr":       m.ZoneAbbr(),
		}
	case "get":
		return num(float64(at(argStr(args, 0)).Get(unit(argStr(args, 1)))))

	// ---- relative time -------------------------------------------------
	case "from":
		return at(argStr(args, 0)).From(at(argStr(args, 1)))
	case "to":
		return at(argStr(args, 0)).To(at(argStr(args, 1)))
	case "calendar":
		return at(argStr(args, 0)).Calendar(at(argStr(args, 1)))
	case "calendarLocale":
		return at(argStr(args, 0)).Locale(argStr(args, 2)).Calendar(at(argStr(args, 1)))

	// ---- durations -----------------------------------------------------
	case "humanize":
		return duration(argIntMap(args, 0)).Humanize(argBool(args, 1))
	case "humanizeLocale":
		return duration(argIntMap(args, 0)).Locale(argStr(args, 2)).Humanize(argBool(args, 1))
	case "durationAs":
		return num(duration(argIntMap(args, 0)).As(unit(argStr(args, 1))))
	case "durationGet":
		return num(float64(duration(argIntMap(args, 0)).Get(unit(argStr(args, 1)))))
	case "durationISO":
		return duration(argIntMap(args, 0)).ISOString()
	case "durationValueOf":
		return num(float64(duration(argIntMap(args, 0)).ValueOf()))
	case "durationAdd":
		return duration(argIntMap(args, 0)).Add(duration(argIntMap(args, 1))).ISOString()
	case "durationSubtract":
		return duration(argIntMap(args, 0)).Subtract(duration(argIntMap(args, 1))).ISOString()
	case "durationAbs":
		return duration(argIntMap(args, 0)).Abs().ISOString()
	case "parseDuration":
		d, err := moment.ParseDuration(argStr(args, 0))
		if err != nil {
			return map[string]any{"valid": false, "iso": "P0D", "ms": float64(0)}
		}
		return map[string]any{"valid": true, "iso": d.ISOString(), "ms": num(d.As(moment.Millisecond))}

	// ---- static helpers ------------------------------------------------
	case "normalizeUnit":
		u, ok := moment.NormalizeUnit(argStr(args, 0))
		if !ok {
			return nil
		}
		return string(u)
	case "months":
		return strs(moment.Months(argStr(args, 0)))
	case "monthsShort":
		return strs(moment.MonthsShort(argStr(args, 0)))
	case "weekdays":
		return strs(moment.Weekdays(argStr(args, 0)))
	case "weekdaysShort":
		return strs(moment.WeekdaysShort(argStr(args, 0)))
	case "weekdaysMin":
		return strs(moment.WeekdaysMin(argStr(args, 0)))
	case "longDateFormat":
		return moment.LongDateFormat(argStr(args, 0), argStr(args, 1))
	case "ordinal":
		return moment.Ordinal(argStr(args, 0), argInt(args, 1))
	case "isMoment":
		switch argStr(args, 0) {
		case "moment":
			return moment.IsMoment(moment.Unix(0))
		case "duration":
			return moment.IsMoment(moment.NewDuration(1, moment.Millisecond))
		default:
			return moment.IsMoment(argStr(args, 0))
		}
	case "isDuration":
		switch argStr(args, 0) {
		case "duration":
			return moment.IsDuration(moment.NewDuration(1, moment.Millisecond))
		case "moment":
			return moment.IsDuration(moment.Unix(0))
		default:
			return moment.IsDuration(argStr(args, 0))
		}
	case "isDate":
		switch argStr(args, 0) {
		case "date":
			return moment.IsDate(time.Unix(0, 0))
		case "moment":
			return moment.IsDate(moment.Unix(0))
		default:
			return moment.IsDate(argStr(args, 0))
		}
	}
	panic("unknown fn: " + fn)
}

func moments(isos []string) []moment.Moment {
	out := make([]moment.Moment, 0, len(isos))
	for _, s := range isos {
		out = append(out, at(s))
	}
	return out
}

func strs(v []string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}

func call(fn string, args []json.RawMessage) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return dispatch(fn, args), nil
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, response{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		value, err := call(req.Fn, req.Args)
		if err != nil {
			emit(out, response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(out, response{ID: req.ID, OK: true, Value: value})
	}
}

func emit(out *bufio.Writer, r response) {
	// Marshal into a map so that a legitimately null/false/zero value is still
	// carried (encoding/json's omitempty would otherwise drop it).
	payload := map[string]any{"id": r.ID, "ok": r.OK}
	if r.OK {
		payload["value"] = r.Value
	} else {
		payload["error"] = r.Error
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"id": r.ID, "ok": false, "error": "unencodable value: " + err.Error()})
	}
	out.Write(b)
	out.WriteByte('\n')
	out.Flush()
}
