'use strict'

// Upstream oracle runner for the express/typeis nested parity harness.
//
// JSON-Lines contract per parity/HARNESS.md: one request per line in, exactly
// one reply per line out, in order, never exiting on a failing case.
//
// Representation note, applied identically by the Go runner: upstream returns
// `false` for "no match / not a media type", which is emitted as JSON null to
// line up with the Go port's (value, ok) pairs.
//
// `normalizeType` is not exported upstream; calling typeis.is(value) with no
// candidate types exercises exactly that code path (it returns the normalised
// content type), so that is what the runner invokes.

const readline = require('readline')
const typeis = require('type-is')

function nil (v) {
  return v === false || v === undefined || v === null ? null : v
}

const handlers = {
  is: (value, types) => nil(types === null || types === undefined ? typeis.is(value) : typeis.is(value, types)),
  normalizeType: (value) => nil(typeis.is(value)),
  normalize: (type) => nil(typeis.normalize(type)),
  match: (expected, actual) => typeis.match(expected, actual)
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
