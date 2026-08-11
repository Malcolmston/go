'use strict'

// Upstream oracle runner for the express/accepts nested parity harness.
//
// Speaks the JSON-Lines contract from parity/HARNESS.md: one request object per
// line on stdin, exactly one reply object per line on stdout, in order. A case
// that throws is reported as {"ok": false, "error": ...}; the runner never
// exits early.
//
// Representation notes, applied identically by the Go runner:
//   * upstream returns `false` for "nothing acceptable"; that is emitted as
//     JSON null, which is what the Go runner emits for its "" sentinel.
//   * the single-value accessors (type/language/charset/encoding) are invoked
//     with the offers spread as separate arguments, matching how express calls
//     them; the plural accessors are invoked with no argument.

const readline = require('readline')
const accepts = require('accepts')

// fakeRequest builds the minimal shape accepts/negotiator read: a `headers`
// object with lower-cased keys. A key that is absent from the case object stays
// absent (upstream treats an absent Accept* header differently from an empty
// one), and an explicit null is also treated as absent.
function fakeRequest (headers) {
  const h = Object.create(null)
  for (const key of Object.keys(headers || {})) {
    const value = headers[key]
    if (value === null || value === undefined) continue
    h[key.toLowerCase()] = value
  }
  return { headers: h }
}

// nil maps upstream's `false` miss sentinel onto JSON null.
function nil (v) {
  return v === false || v === undefined ? null : v
}

const handlers = {
  type: (headers, offers) => nil(accepts(fakeRequest(headers)).type(...offers)),
  types: (headers) => accepts(fakeRequest(headers)).types(),
  language: (headers, offers) => nil(accepts(fakeRequest(headers)).language(...offers)),
  languages: (headers) => accepts(fakeRequest(headers)).languages(),
  charset: (headers, offers) => nil(accepts(fakeRequest(headers)).charset(...offers)),
  charsets: (headers) => accepts(fakeRequest(headers)).charsets(),
  encoding: (headers, offers) => nil(accepts(fakeRequest(headers)).encoding(...offers)),
  encodings: (headers) => accepts(fakeRequest(headers)).encodings()
}

const rl = readline.createInterface({ input: process.stdin, terminal: false })

rl.on('line', (line) => {
  if (line.trim() === '') return
  let req
  try {
    req = JSON.parse(line)
  } catch (err) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: 'bad request line: ' + err.message }) + '\n')
    return
  }
  let out
  try {
    const fn = handlers[req.fn]
    if (!fn) throw new Error('unknown fn: ' + req.fn)
    out = { id: req.id, ok: true, value: fn(...(req.args || [])) }
  } catch (err) {
    out = { id: req.id, ok: false, error: String((err && err.message) || err) }
  }
  process.stdout.write(JSON.stringify(out) + '\n')
})
