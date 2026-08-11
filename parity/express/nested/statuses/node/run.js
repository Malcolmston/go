'use strict'

// Upstream oracle runner for the express/statuses nested parity harness.
//
// JSON-Lines contract per parity/HARNESS.md: one request per line in, exactly
// one reply per line out, in order, never exiting on a failing case.
//
// The upstream module is a callable with lookup tables hanging off it. The
// runner exposes each of those as its own fn so the Go port's separate functions
// line up one-for-one:
//
//   message      -> statuses.message[code], via the callable so an unknown code
//                   throws (which is what the Go port's "" sentinel means)
//   code         -> statuses.code[message.toLowerCase()], likewise
//   status       -> the raw polymorphic callable, including its numeric-string
//                   coercion, which the typed Go API cannot express
//   codes        -> statuses.codes, sorted ascending on both sides
//   isRedirect / isEmpty / isRetry -> the three classification tables

const readline = require('readline')
const statuses = require('statuses')

const handlers = {
  message: (code) => {
    const msg = statuses.message[code]
    if (msg === undefined) throw new Error('invalid status code: ' + code)
    return msg
  },
  code: (message) => {
    const code = statuses.code[String(message).toLowerCase()]
    if (code === undefined) throw new Error('invalid status message: ' + JSON.stringify(message))
    return code
  },
  status: (arg) => statuses(arg),
  codes: () => statuses.codes.slice().sort((a, b) => a - b),
  isRedirect: (code) => Boolean(statuses.redirect[code]),
  isEmpty: (code) => Boolean(statuses.empty[code]),
  isRetry: (code) => Boolean(statuses.retry[code])
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
