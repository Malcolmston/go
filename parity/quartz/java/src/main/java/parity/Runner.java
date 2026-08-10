package parity;

/*
 * Upstream (oracle) runner for the quartz parity harness.
 *
 * Speaks JSON Lines on stdio: one request object per line in, exactly one
 * response object per line out, in the same order. Never exits on a failing
 * case -- every throwable is reported as {"ok":false,"error":...}.
 *
 * Every fire-time computation used here is a pure function of an explicitly
 * supplied instant and time zone. Nothing reads the wall clock, and no
 * Scheduler is ever started.
 */

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;

import org.quartz.CronExpression;
import org.quartz.DateBuilder.IntervalUnit;
import org.quartz.JobKey;
import org.quartz.SimpleTrigger;
import org.quartz.TimeOfDay;
import org.quartz.TriggerKey;
import org.quartz.impl.triggers.CalendarIntervalTriggerImpl;
import org.quartz.impl.triggers.CronTriggerImpl;
import org.quartz.impl.triggers.DailyTimeIntervalTriggerImpl;
import org.quartz.impl.triggers.SimpleTriggerImpl;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.PrintStream;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Date;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.TimeZone;

public final class Runner {

    private static final ObjectMapper M = new ObjectMapper();

    /** The single fixed wire format for every emitted instant: UTC, second precision. */
    private static final DateTimeFormatter OUT =
            DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss").withZone(ZoneOffset.UTC);

    public static void main(String[] args) throws Exception {
        // Pin the ambient zone so nothing depends on the host's setting.
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"));
        PrintStream out = new PrintStream(System.out, true, StandardCharsets.UTF_8);
        BufferedReader in = new BufferedReader(new InputStreamReader(System.in, StandardCharsets.UTF_8));
        String line;
        while ((line = in.readLine()) != null) {
            if (line.isBlank()) {
                continue;
            }
            ObjectNode resp = M.createObjectNode();
            String id = "?";
            try {
                JsonNode req = M.readTree(line);
                id = req.path("id").asText("?");
                resp.put("id", id);
                Object value = dispatch(req.path("fn").asText(""), req.path("args"));
                resp.put("ok", true);
                resp.set("value", M.valueToTree(value));
            } catch (Throwable t) {
                resp.removeAll();
                resp.put("id", id);
                resp.put("ok", false);
                resp.put("error", describe(t));
            }
            out.println(M.writeValueAsString(resp));
            out.flush();
        }
    }

    private static String describe(Throwable t) {
        String m = t.getMessage();
        return t.getClass().getSimpleName() + (m == null ? "" : ": " + m);
    }

    // ---------------------------------------------------------------- dispatch

    private static Object dispatch(String fn, JsonNode a) throws Exception {
        switch (fn) {
            case "cronParse":
                return cronParse(str(a, 0));
            case "cronNext":
                return cronNext(str(a, 0), str(a, 1), str(a, 2), intAt(a, 3));
            case "cronSatisfiedBy":
                return cronSatisfiedBy(str(a, 0), str(a, 1), str(a, 2));
            case "cronTriggerNext":
                return cronTriggerNext(str(a, 0), str(a, 1), str(a, 2), str(a, 3), intAt(a, 4));
            case "simpleNext":
                return simpleNext(str(a, 0), longAt(a, 1), intAt(a, 2), nullableStr(a, 3), str(a, 4), intAt(a, 5));
            case "calendarIntervalNext":
                return calendarIntervalNext(str(a, 0), str(a, 1), str(a, 2), intAt(a, 3),
                        boolAt(a, 4), str(a, 5), intAt(a, 6));
            case "dailyTimeIntervalNext":
                return dailyTimeIntervalNext(str(a, 0), str(a, 1), str(a, 2), str(a, 3), str(a, 4),
                        intAt(a, 5), str(a, 6), str(a, 7), intAt(a, 8));
            case "misfireIgnoreNext":
                return misfireIgnoreNext(str(a, 0), a);
            default:
                throw new IllegalArgumentException("unknown fn: " + fn);
        }
    }

    // ------------------------------------------------------------------- args

    private static String str(JsonNode a, int i) {
        JsonNode n = a.path(i);
        if (n.isMissingNode() || n.isNull()) {
            throw new IllegalArgumentException("missing string arg " + i);
        }
        return n.asText();
    }

    private static String nullableStr(JsonNode a, int i) {
        JsonNode n = a.path(i);
        return (n.isMissingNode() || n.isNull()) ? null : n.asText();
    }

    private static int intAt(JsonNode a, int i) {
        JsonNode n = a.path(i);
        if (!n.isNumber()) {
            throw new IllegalArgumentException("missing int arg " + i);
        }
        return n.asInt();
    }

    private static long longAt(JsonNode a, int i) {
        JsonNode n = a.path(i);
        if (!n.isNumber()) {
            throw new IllegalArgumentException("missing long arg " + i);
        }
        return n.asLong();
    }

    private static boolean boolAt(JsonNode a, int i) {
        return a.path(i).asBoolean(false);
    }

    // ------------------------------------------------------------- conversion

    /** Parses an instant. Always an explicit UTC RFC-3339 string -- never "now". */
    private static Date instant(String iso) {
        return Date.from(Instant.parse(iso));
    }

    private static String fmt(Date d) {
        return OUT.format(d.toInstant()) + "+00:00";
    }

    private static TimeZone zone(String id) {
        if (!"UTC".equals(id) && !"GMT".equals(id)) {
            // TimeZone.getTimeZone silently answers GMT for an unknown id; reject
            // that so a typo in a case file cannot look like a passing case.
            boolean known = false;
            for (String s : TimeZone.getAvailableIDs()) {
                if (s.equals(id)) {
                    known = true;
                    break;
                }
            }
            if (!known) {
                throw new IllegalArgumentException("unknown time zone: " + id);
            }
        }
        return TimeZone.getTimeZone(id);
    }

    private static IntervalUnit unit(String name) {
        return IntervalUnit.valueOf(name.toUpperCase(Locale.ROOT));
    }

    /** "SUN,MON" or "*" -> java.util.Calendar day numbers (SUNDAY == 1). */
    private static Set<Integer> daysOfWeek(String csv) {
        if (csv == null || csv.isBlank() || "*".equals(csv)) {
            return new HashSet<>(org.quartz.DailyTimeIntervalScheduleBuilder.ALL_DAYS_OF_THE_WEEK);
        }
        Set<Integer> set = new LinkedHashSet<>();
        for (String part : csv.split(",")) {
            switch (part.trim().toUpperCase(Locale.ROOT)) {
                case "SUN": set.add(java.util.Calendar.SUNDAY); break;
                case "MON": set.add(java.util.Calendar.MONDAY); break;
                case "TUE": set.add(java.util.Calendar.TUESDAY); break;
                case "WED": set.add(java.util.Calendar.WEDNESDAY); break;
                case "THU": set.add(java.util.Calendar.THURSDAY); break;
                case "FRI": set.add(java.util.Calendar.FRIDAY); break;
                case "SAT": set.add(java.util.Calendar.SATURDAY); break;
                default: throw new IllegalArgumentException("bad weekday: " + part);
            }
        }
        return set;
    }

    private static TimeOfDay timeOfDay(String hhmmss) {
        String[] p = hhmmss.split(":");
        if (p.length != 3) {
            throw new IllegalArgumentException("bad time-of-day: " + hhmmss);
        }
        return new TimeOfDay(Integer.parseInt(p[0]), Integer.parseInt(p[1]), Integer.parseInt(p[2]));
    }

    // ------------------------------------------------------------------- fns

    /** org.quartz.CronExpression(String) -- accepted or rejected. */
    private static Object cronParse(String expr) throws Exception {
        new CronExpression(expr);
        return Boolean.TRUE;
    }

    /** org.quartz.CronExpression.getNextValidTimeAfter, chained n times. */
    private static Object cronNext(String expr, String tz, String afterIso, int n) throws Exception {
        CronExpression ce = new CronExpression(expr);
        ce.setTimeZone(zone(tz));
        List<String> outv = new ArrayList<>();
        Date cur = instant(afterIso);
        for (int i = 0; i < n; i++) {
            Date next = ce.getNextValidTimeAfter(cur);
            if (next == null) {
                break;
            }
            outv.add(fmt(next));
            cur = next;
        }
        return outv;
    }

    /** org.quartz.CronExpression.isSatisfiedBy. */
    private static Object cronSatisfiedBy(String expr, String tz, String iso) throws Exception {
        CronExpression ce = new CronExpression(expr);
        ce.setTimeZone(zone(tz));
        return ce.isSatisfiedBy(instant(iso));
    }

    /** org.quartz.impl.triggers.CronTriggerImpl.getFireTimeAfter, chained. */
    private static Object cronTriggerNext(String expr, String tz, String startIso, String afterIso, int n)
            throws Exception {
        CronTriggerImpl t = new CronTriggerImpl();
        t.setKey(new TriggerKey("t", "g"));
        t.setJobKey(new JobKey("j", "g"));
        t.setTimeZone(zone(tz));
        t.setCronExpression(expr);
        t.setStartTime(instant(startIso));
        return chain(d -> t.getFireTimeAfter(d), instant(afterIso), n);
    }

    /** org.quartz.impl.triggers.SimpleTriggerImpl.getFireTimeAfter, chained. */
    private static Object simpleNext(String startIso, long intervalMillis, int repeatCount,
                                     String endIso, String afterIso, int n) {
        SimpleTriggerImpl t = new SimpleTriggerImpl();
        t.setKey(new TriggerKey("t", "g"));
        t.setJobKey(new JobKey("j", "g"));
        t.setStartTime(instant(startIso));
        t.setRepeatInterval(intervalMillis);
        t.setRepeatCount(repeatCount);
        if (endIso != null) {
            t.setEndTime(instant(endIso));
        }
        return chain(d -> t.getFireTimeAfter(d), instant(afterIso), n);
    }

    /** org.quartz.impl.triggers.CalendarIntervalTriggerImpl.getFireTimeAfter, chained. */
    private static Object calendarIntervalNext(String tz, String startIso, String unitName, int count,
                                               boolean preserveWallClock, String afterIso, int n) {
        CalendarIntervalTriggerImpl t = new CalendarIntervalTriggerImpl();
        t.setKey(new TriggerKey("t", "g"));
        t.setJobKey(new JobKey("j", "g"));
        t.setTimeZone(zone(tz));
        t.setStartTime(instant(startIso));
        t.setRepeatIntervalUnit(unit(unitName));
        t.setRepeatInterval(count);
        t.setPreserveHourOfDayAcrossDaylightSavings(preserveWallClock);
        return chain(d -> t.getFireTimeAfter(d), instant(afterIso), n);
    }

    /** org.quartz.impl.triggers.DailyTimeIntervalTriggerImpl.getFireTimeAfter, chained. */
    private static Object dailyTimeIntervalNext(String tz, String startIso, String startTod, String endTod,
                                                String unitName, int count, String days,
                                                String afterIso, int n) {
        // DailyTimeIntervalTriggerImpl has no setTimeZone in this version: it
        // resolves its time-of-day window against the JVM default zone, so the
        // requested zone has to be installed as the default around the call.
        TimeZone saved = TimeZone.getDefault();
        try {
        TimeZone.setDefault(zone(tz));
        DailyTimeIntervalTriggerImpl t = new DailyTimeIntervalTriggerImpl();
        t.setKey(new TriggerKey("t", "g"));
        t.setJobKey(new JobKey("j", "g"));
        t.setStartTime(instant(startIso));
        t.setStartTimeOfDay(timeOfDay(startTod));
        t.setEndTimeOfDay(timeOfDay(endTod));
        t.setRepeatIntervalUnit(unit(unitName));
        t.setRepeatInterval(count);
        t.setDaysOfWeek(daysOfWeek(days));
        return chain(d -> t.getFireTimeAfter(d), instant(afterIso), n);
        } finally {
            TimeZone.setDefault(saved);
        }
    }

    /**
     * The one misfire instruction whose handling reads no wall clock on either
     * side: MISFIRE_INSTRUCTION_IGNORE_MISFIRE_POLICY must leave the computed
     * next fire time untouched. Every other instruction routes through
     * updateAfterMisfire, which upstream implements against {@code new Date()},
     * so it cannot be compared deterministically (see COVERAGE.md).
     *
     * args: [kind, ...kind-specific..., nowIso]
     */
    private static Object misfireIgnoreNext(String kind, JsonNode a) throws Exception {
        if ("cron".equals(kind)) {
            CronTriggerImpl t = new CronTriggerImpl();
            t.setKey(new TriggerKey("t", "g"));
            t.setJobKey(new JobKey("j", "g"));
            t.setTimeZone(zone(str(a, 1)));
            t.setCronExpression(str(a, 2));
            t.setStartTime(instant(str(a, 3)));
            t.setMisfireInstruction(org.quartz.Trigger.MISFIRE_INSTRUCTION_IGNORE_MISFIRE_POLICY);
            t.computeFirstFireTime(null);
            t.updateAfterMisfire(null);
            Date next = t.getNextFireTime();
            return next == null ? null : fmt(next);
        }
        if ("simple".equals(kind)) {
            SimpleTriggerImpl t = new SimpleTriggerImpl();
            t.setKey(new TriggerKey("t", "g"));
            t.setJobKey(new JobKey("j", "g"));
            t.setStartTime(instant(str(a, 1)));
            t.setRepeatInterval(longAt(a, 2));
            t.setRepeatCount(intAt(a, 3));
            t.setMisfireInstruction(SimpleTrigger.MISFIRE_INSTRUCTION_IGNORE_MISFIRE_POLICY);
            t.computeFirstFireTime(null);
            t.updateAfterMisfire(null);
            Date next = t.getNextFireTime();
            return next == null ? null : fmt(next);
        }
        throw new IllegalArgumentException("unknown misfire kind: " + kind);
    }

    // ------------------------------------------------------------------ chain

    private interface Step {
        Date after(Date d);
    }

    private static List<String> chain(Step step, Date after, int n) {
        List<String> outv = new ArrayList<>();
        Date cur = after;
        for (int i = 0; i < n; i++) {
            Date next = step.after(cur);
            if (next == null) {
                break;
            }
            outv.add(fmt(next));
            cur = next;
        }
        return outv;
    }

    private Runner() {
    }
}
