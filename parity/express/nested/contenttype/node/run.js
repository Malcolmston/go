'use strict'

// Upstream oracle runner for the express/contenttype nested parity harness.
//
// JSON-Lines contract per parity/HARNESS.md: one request per line in, exactly
// one reply per line out, in order, never exiting on a failing case. Upstream
// signals every malformed input by throwing a TypeError, so those cases come
// back as ok:false — the harness compares *whether* both sides failed, not the
// message text.
//
// `parse` is reported as {type, parameters} so the two runners agree on shape;
// the parameters object is copied onto a plain object because upstream builds it
// with a null prototype.

const readline = require('readline')
const contentType = require('content-type')

const handlers = {
  parse: (s) => {
    const obj = contentType.parse(s)
    return { type: obj.type, parameters: Object.assign({}, obj.parameters) }
  },
  format: (obj) => contentType.format({
    type: obj.type,
    parameters: obj.parameters === null || obj.parameters === undefined ? undefined : obj.parameters
  }),
  // parse followed by format, which is how callers normalise a header value.
  roundTrip: (s) => contentType.format(contentType.parse(s))
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
