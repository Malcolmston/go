'use strict'

// Upstream oracle runner for the express/negotiator nested parity harness.
//
// JSON-Lines contract per parity/HARNESS.md: one request per line in, exactly
// one reply per line out, in order, never exiting on a failing case.
//
// Case shape: {"fn": "<method>", "args": [headers, available|null]}. A null (or
// missing) `available` means "call the method with no argument", which is how
// upstream asks for the full sorted list. An empty array is deliberately never
// used: JS treats `[]` as truthy so `mediaTypes([])` returns [], a distinction
// the variadic Go signature cannot express.
//
// Representation note, applied identically by the Go runner: the singular
// accessors return `undefined` when nothing is acceptable; that is emitted as
// JSON null, matching the Go runner's "" sentinel.

const readline = require('readline')
const Negotiator = require('negotiator')

function fakeRequest (headers) {
  const h = Object.create(null)
  for (const key of Object.keys(headers || {})) {
    const value = headers[key]
    if (value === null || value === undefined) continue
    h[key.toLowerCase()] = value
  }
  return { headers: h }
}

function nil (v) {
  return v === undefined || v === false ? null : v
}

// avail turns a case's second argument into either an array or `undefined`, so
// that the upstream method is genuinely called with no argument.
function avail (a) {
  return a === null || a === undefined ? undefined : a
}

const handlers = {
  mediaType: (h, a) => nil(new Negotiator(fakeRequest(h)).mediaType(avail(a))),
  mediaTypes: (h, a) => new Negotiator(fakeRequest(h)).mediaTypes(avail(a)),
  language: (h, a) => nil(new Negotiator(fakeRequest(h)).language(avail(a))),
  languages: (h, a) => new Negotiator(fakeRequest(h)).languages(avail(a)),
  charset: (h, a) => nil(new Negotiator(fakeRequest(h)).charset(avail(a))),
  charsets: (h, a) => new Negotiator(fakeRequest(h)).charsets(avail(a)),
  encoding: (h, a) => nil(new Negotiator(fakeRequest(h)).encoding(avail(a))),
  encodings: (h, a) => new Negotiator(fakeRequest(h)).encodings(avail(a)),

  // Backwards-compatibility aliases kept by upstream; the Go port has no
  // separate symbols for them, so they resolve to the same methods.
  preferredMediaType: (h, a) => nil(new Negotiator(fakeRequest(h)).preferredMediaType(avail(a))),
  preferredMediaTypes: (h, a) => new Negotiator(fakeRequest(h)).preferredMediaTypes(avail(a)),
  preferredLanguage: (h, a) => nil(new Negotiator(fakeRequest(h)).preferredLanguage(avail(a))),
  preferredLanguages: (h, a) => new Negotiator(fakeRequest(h)).preferredLanguages(avail(a)),
  preferredCharset: (h, a) => nil(new Negotiator(fakeRequest(h)).preferredCharset(avail(a))),
  preferredCharsets: (h, a) => new Negotiator(fakeRequest(h)).preferredCharsets(avail(a)),
  preferredEncoding: (h, a) => nil(new Negotiator(fakeRequest(h)).preferredEncoding(avail(a))),
  preferredEncodings: (h, a) => new Negotiator(fakeRequest(h)).preferredEncodings(avail(a))
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
