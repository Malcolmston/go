'use strict'

// Upstream oracle runner for the express/mimetypes nested parity harness.
//
// JSON-Lines contract per parity/HARNESS.md: one request per line in, exactly
// one reply per line out, in order, never exiting on a failing case.
//
// Representation note, applied identically by the Go runner: every upstream
// accessor returns `false` when it has no answer, which is emitted as JSON null
// to line up with the Go port's (value, ok) pairs.

const readline = require('readline')
const mime = require('mime-types')

function nil (v) {
  return v === false || v === undefined ? null : v
}

const handlers = {
  lookup: (s) => nil(mime.lookup(s)),
  charset: (s) => nil(mime.charset(s)),
  contentType: (s) => nil(mime.contentType(s)),
  extension: (s) => nil(mime.extension(s))
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
