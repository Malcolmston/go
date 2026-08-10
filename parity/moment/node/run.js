'use strict';
// Upstream oracle runner for the `moment` parity harness.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order.  Never exits on a failing
// case -- a throw becomes {"ok":false,"error":...}.
//
// Determinism: nothing here ever consults the real clock.  Every relative-time
// operation receives an explicit reference instant from the case file, and the
// process is expected to run under TZ=UTC.

process.env.TZ = process.env.TZ || 'UTC';

const readline = require('readline');
const moment = require('moment');

moment.suppressDeprecationWarnings = true;

// ---------------------------------------------------------------- helpers

// Parse an ISO instant supplied by a case file.  Always UTC, always strict-ish
// ISO so the oracle never falls back to Date's implementation-defined parser.
function at(iso) {
  return moment.utc(iso, moment.ISO_8601);
}

// The uniform "instant" result shape: validity as a boolean plus the formatted
// instant as a string.  Error text is deliberately never part of the value.
function inst(m) {
  return m && m.isValid()
    ? { valid: true, iso: m.toISOString() }
    : { valid: false, iso: '' };
}

function num(v) {
  return typeof v === 'number' && !isFinite(v) ? null : v;
}

function dur(obj) {
  return moment.duration(obj);
}

const ops = {
  // ---- parsing -------------------------------------------------------
  parse: (input) => inst(moment.utc(input)),
  parseISO: (input) => inst(moment.utc(input, moment.ISO_8601)),
  parseRFC2822: (input) => inst(moment.utc(input, moment.RFC_2822)),
  parseFormat: (input, format) => inst(moment.utc(input, format)),
  parseFormatStrict: (input, format) => inst(moment.utc(input, format, true)),
  parseFormats: (input, formats) => inst(moment.utc(input, formats)),
  parseFormatLocale: (input, format, locale) =>
    inst(moment.utc(input, format, locale)),
  parseZone: (input) => {
    const m = moment.parseZone(input);
    return {
      valid: m.isValid(),
      iso: m.isValid() ? m.toISOString() : '',
      offsetMinutes: m.isValid() ? m.utcOffset() : 0,
      formatted: m.isValid() ? m.format('YYYY-MM-DDTHH:mm:ssZ') : '',
    };
  },
  fromArray: (parts) => inst(moment.utc(parts)),
  fromObject: (obj) => inst(moment.utc(obj)),
  isValid: (input) => moment.utc(input).isValid(),
  isValidFormat: (input, format) => moment.utc(input, format).isValid(),
  isValidFormatStrict: (input, format) => moment.utc(input, format, true).isValid(),
  invalidAt: (input, format) => moment.utc(input, format).invalidAt(),

  // ---- formatting ----------------------------------------------------
  format: (iso, format) => at(iso).format(format),
  formatLocale: (iso, format, locale) => at(iso).locale(locale).format(format),
  formatAtOffset: (iso, offsetMinutes, format) =>
    at(iso).utcOffset(offsetMinutes).format(format),
  toISOString: (iso) => at(iso).toISOString(),
  toISOStringZone: (iso, offsetMinutes) =>
    at(iso).utcOffset(offsetMinutes).toISOString(true),
  toJSON: (iso) => at(iso).toJSON(),
  toArray: (iso) => at(iso).toArray(),
  toObject: (iso) => at(iso).toObject(),
  valueOf: (iso) => at(iso).valueOf(),
  unix: (iso) => at(iso).unix(),

  // ---- manipulation --------------------------------------------------
  add: (iso, n, unit) => inst(at(iso).add(n, unit)),
  subtract: (iso, n, unit) => inst(at(iso).subtract(n, unit)),
  startOf: (iso, unit) => inst(at(iso).startOf(unit)),
  endOf: (iso, unit) => inst(at(iso).endOf(unit)),
  set: (iso, unit, value) => inst(at(iso).set(unit, value)),
  setDate: (iso, v) => inst(at(iso).date(v)),
  setDayOfYear: (iso, v) => inst(at(iso).dayOfYear(v)),
  setWeekday: (iso, v) => inst(at(iso).day(v)),
  setISOWeekday: (iso, v) => inst(at(iso).isoWeekday(v)),
  setQuarter: (iso, v) => inst(at(iso).quarter(v)),
  setYear: (iso, v) => inst(at(iso).year(v)),
  setMonth1: (iso, v) => inst(at(iso).month(v - 1)),
  clone: (iso) => inst(at(iso).clone()),

  // ---- comparison ----------------------------------------------------
  diff: (a, b, unit) => num(at(a).diff(at(b), unit)),
  diffFloat: (a, b, unit) => num(at(a).diff(at(b), unit, true)),
  isBefore: (a, b) => at(a).isBefore(at(b)),
  isAfter: (a, b) => at(a).isAfter(at(b)),
  isSame: (a, b) => at(a).isSame(at(b)),
  isSameOrBefore: (a, b) => at(a).isSameOrBefore(at(b)),
  isSameOrAfter: (a, b) => at(a).isSameOrAfter(at(b)),
  isSameUnit: (a, b, unit) => at(a).isSame(at(b), unit),
  isBetween: (x, s, e, inclusive) =>
    at(x).isBetween(at(s), at(e), null, inclusive ? '[]' : '()'),
  max: (isos) => inst(moment.max(isos.map(at))),
  min: (isos) => inst(moment.min(isos.map(at))),

  // ---- getters (one case covers many prototype getters) --------------
  components: (iso) => {
    const m = at(iso);
    return {
      year: m.year(),
      month0: m.month(),
      date: m.date(),
      hour: m.hour(),
      minute: m.minute(),
      second: m.second(),
      millisecond: m.millisecond(),
      dayOfYear: m.dayOfYear(),
      quarter: m.quarter(),
      weekday: m.day(),
      isoWeekday: m.isoWeekday(),
      week: m.week(),
      isoWeek: m.isoWeek(),
      weekYear: m.weekYear(),
      isoWeekYear: m.isoWeekYear(),
      daysInMonth: m.daysInMonth(),
      isLeapYear: m.isLeapYear(),
      weeksInYear: m.weeksInYear(),
      isoWeeksInYear: m.isoWeeksInYear(),
      localeWeekday: m.weekday(),
      utcOffset: m.utcOffset(),
      isUTC: m.isUTC(),
      zoneAbbr: m.zoneAbbr(),
    };
  },
  get: (iso, unit) => num(at(iso).get(unit)),

  // ---- relative time -------------------------------------------------
  from: (target, reference) => at(target).from(at(reference)),
  to: (reference, target) => at(reference).to(at(target)),
  calendar: (iso, referenceIso) => at(iso).calendar(at(referenceIso)),
  calendarLocale: (iso, referenceIso, locale) =>
    at(iso).locale(locale).calendar(at(referenceIso)),

  // ---- durations -----------------------------------------------------
  humanize: (obj, withSuffix) => dur(obj).humanize(!!withSuffix),
  humanizeLocale: (obj, withSuffix, locale) =>
    dur(obj).locale(locale).humanize(!!withSuffix),
  durationAs: (obj, unit) => num(dur(obj).as(unit)),
  durationGet: (obj, unit) => num(dur(obj).get(unit)),
  durationISO: (obj) => dur(obj).toISOString(),
  durationValueOf: (obj) => num(dur(obj).valueOf()),
  durationAdd: (a, b) => dur(a).add(dur(b)).toISOString(),
  durationSubtract: (a, b) => dur(a).subtract(dur(b)).toISOString(),
  durationAbs: (obj) => dur(obj).abs().toISOString(),
  parseDuration: (isoDur) => {
    const d = moment.duration(isoDur);
    // moment.duration() of garbage yields a zero duration, so report both the
    // round-tripped ISO string and the millisecond length.
    return { valid: d.isValid(), iso: d.toISOString(), ms: num(d.asMilliseconds()) };
  },

  // ---- static helpers ------------------------------------------------
  normalizeUnit: (name) => moment.normalizeUnits(name) || null,
  months: (locale) => moment.localeData(locale).months(),
  monthsShort: (locale) => moment.localeData(locale).monthsShort(),
  weekdays: (locale) => moment.localeData(locale).weekdays(),
  weekdaysShort: (locale) => moment.localeData(locale).weekdaysShort(),
  weekdaysMin: (locale) => moment.localeData(locale).weekdaysMin(),
  longDateFormat: (locale, token) =>
    moment.localeData(locale).longDateFormat(token),
  // The Go port's package-level Ordinal always resolves the "D" token, so the
  // oracle is asked the same question rather than being called token-less
  // (which several moment locales answer with a bare number).
  ordinal: (locale, n) => moment.localeData(locale).ordinal(n, 'D'),
  isMoment: (kind) =>
    kind === 'moment'
      ? moment.isMoment(moment.utc(0))
      : kind === 'duration'
        ? moment.isMoment(moment.duration(1))
        : moment.isMoment(kind),
  isDuration: (kind) =>
    kind === 'duration'
      ? moment.isDuration(moment.duration(1))
      : kind === 'moment'
        ? moment.isDuration(moment.utc(0))
        : moment.isDuration(kind),
  isDate: (kind) =>
    kind === 'date'
      ? moment.isDate(new Date(0))
      : kind === 'moment'
        ? moment.isDate(moment.utc(0))
        : moment.isDate(kind),
};

const rl = readline.createInterface({ input: process.stdin, terminal: false });

rl.on('line', (line) => {
  const text = line.trim();
  if (!text) return;
  let req;
  try {
    req = JSON.parse(text);
  } catch (err) {
    process.stdout.write(
      JSON.stringify({ id: '?', ok: false, error: 'bad request: ' + err.message }) + '\n',
    );
    return;
  }
  let out;
  try {
    const fn = ops[req.fn];
    if (!fn) throw new Error('unknown fn: ' + req.fn);
    out = { id: req.id, ok: true, value: fn.apply(null, req.args || []) };
  } catch (err) {
    out = { id: req.id, ok: false, error: String((err && err.message) || err) };
  }
  process.stdout.write(JSON.stringify(out) + '\n');
});
