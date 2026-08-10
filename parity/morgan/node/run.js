'use strict'

// Upstream oracle runner for the morgan parity harness.
//
// Protocol: one JSON object per line on stdin, one JSON object per line on
// stdout, same order (see ../../HARNESS.md).
//
//   in : {"id":"...","fn":"log","args":[<spec>]}
//   out: {"id":"...","ok":true,"value":"<normalised log line>"}
//        {"id":"...","ok":false,"error":"..."}
//
// <spec> drives a real request/response exchange through a real express app
// with morgan mounted on it; the emitted access-log line (normalised, see
// normalise() below) is the comparable artefact.

const http = require('http')
const readline = require('readline')
const express = require('express')
const morgan = require('morgan')

// ---------------------------------------------------------------------------
// custom tokens — registered identically in go/run.go
// ---------------------------------------------------------------------------

morgan.token('ctok-const', function () { return 'CONST' })
morgan.token('ctok-method', function (req) { return req.method.toLowerCase() })
morgan.token('ctok-hdr', function (req, res, arg) {
  if (arg === undefined) return undefined
  return req.headers[String(arg).toLowerCase()]
})
morgan.token('ctok-empty', function () { return '' })

// ---------------------------------------------------------------------------
// named formats registered at runtime — registered identically in go/run.go
// ---------------------------------------------------------------------------

morgan.format('parity-named', ':method :url :status len=:res[content-length]')
morgan.format('parity-fn', function (tokens, req, res) {
  return 'FN=' + tokens.method(req, res) + ' ' + (tokens.status(req, res) || '-')
})

// ---------------------------------------------------------------------------
// named skip functions — registered identically in go/run.go
// ---------------------------------------------------------------------------

const SKIPS = {
  'none': undefined,
  'status-lt-400': function (req, res) { return res.statusCode < 400 },
  'path-healthz': function (req) { return req.url.split('?')[0] === '/healthz' }
}

// ---------------------------------------------------------------------------
// normalisation — MUST stay byte-identical with go/run.go normalise()
// ---------------------------------------------------------------------------

const RE_CLF = /\d{2}\/[A-Z][a-z]{2}\/\d{4}:\d{2}:\d{2}:\d{2} [+-]\d{4}/g
const RE_ISO = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z/g
const RE_WEB = /[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT/g
const RE_TIME_DEC = /(\d+)\.(\d+) ms/g
const RE_TIME_INT = /(\d+) ms/g
const RE_JSON_TIME = /"(responseTime|totalTime)":(\d+)(?:\.(\d+))?/g
const RE_HOSTPORT = /127\.0\.0\.1:\d+/g
const RE_PID = /PID=\d+/g

function normalise (line) {
  return String(line)
    .replace(/\n+$/, '')
    .replace(RE_CLF, '<DATE-CLF>')
    .replace(RE_ISO, '<DATE-ISO>')
    .replace(RE_WEB, '<DATE-WEB>')
    .replace(RE_TIME_DEC, function (_, i, frac) { return '<T:' + frac.length + '> ms' })
    .replace(RE_TIME_INT, '<T:0> ms')
    .replace(RE_JSON_TIME, function (_, k, i, frac) {
      return '"' + k + '":<T:' + (frac === undefined ? 0 : frac.length) + '>'
    })
    .replace(RE_HOSTPORT, '127.0.0.1:<PORT>')
    .replace(RE_PID, 'PID=<PID>')
}

// ---------------------------------------------------------------------------
// the server: one instance for the whole run, driven by `current`
// ---------------------------------------------------------------------------

let current = null // {spec, mw, lines}

const app = express()
app.use(function (req, res, next) {
  if (current === null || current.mw === null) return next()
  current.mw(req, res, next)
})
app.use(function (req, res) {
  const spec = (current && current.spec) || {}
  const r = spec.response || {}
  const body = typeof r.body === 'string' ? r.body : ''
  const headers = r.headers || {}
  for (const k of Object.keys(headers)) res.setHeader(k, headers[k])
  res.status(typeof r.status === 'number' ? r.status : 200)
  if (r.contentLength === 'explicit') {
    res.setHeader('Content-Length', String(Buffer.byteLength(body)))
    res.end(body)
  } else if (r.contentLength === 'none') {
    // chunked: neither side reports a Content-Length
    res.end(body)
  } else {
    // 'express' (default): res.send sets Content-Length itself, which is what
    // the Go port has no equivalent for.
    res.send(body)
  }
})

function sendRequest (port, spec) {
  return new Promise(function (resolve, reject) {
    const rq = (spec.request || {})
    const headers = Object.assign({}, rq.headers || {})
    const body = typeof rq.body === 'string' ? rq.body : null
    if (body !== null && headers['Content-Length'] === undefined) {
      headers['Content-Length'] = String(Buffer.byteLength(body))
    }
    const req = http.request({
      host: '127.0.0.1',
      port: port,
      method: rq.method || 'GET',
      path: rq.path || '/',
      headers: headers,
      agent: false
    }, function (res) {
      res.resume()
      res.on('end', resolve)
      res.on('error', reject)
    })
    req.on('error', reject)
    if (body !== null) req.write(body)
    req.end()
  })
}

function waitForLine (state, ms) {
  return new Promise(function (resolve) {
    const deadline = Date.now() + ms
    ;(function poll () {
      if (state.lines.length > 0 || state.error !== null || Date.now() > deadline) return resolve()
      setTimeout(poll, 5)
    })()
  })
}

// resolveFormat mirrors morgan's own getFormatFunction: a registered name wins,
// otherwise the string is compiled as a raw format.
function resolveFormat (name) {
  const fmt = morgan[name] || name || morgan.default
  return typeof fmt !== 'function' ? morgan.compile(fmt) : fmt
}

function buildMiddleware (spec) {
  const state = { lines: [], error: null }
  const inner = resolveFormat(spec.format)
  // Rendering happens inside morgan's on-finished callback, where a throw would
  // be an uncaught exception and kill the runner. Capture it instead and return
  // null, which morgan treats as "skip this line"; runCase re-throws it so the
  // case is reported as ok:false.
  const guarded = function (tokens, req, res) {
    try {
      return inner(tokens, req, res)
    } catch (err) {
      state.error = err
      return null
    }
  }
  const opts = {
    stream: { write: function (s) { state.lines.push(s) } }
  }
  const o = spec.options || {}
  if (o.immediate) opts.immediate = true
  if (o.skip && o.skip !== 'none') {
    if (!(o.skip in SKIPS)) throw new Error('unknown skip: ' + o.skip)
    opts.skip = SKIPS[o.skip]
  }
  return { mw: morgan(guarded, opts), state: state }
}

async function runCase (spec, port) {
  const built = buildMiddleware(spec)
  current = { spec: spec, mw: built.mw, lines: built.state.lines }
  await sendRequest(port, spec)
  await waitForLine(built.state, 400)
  if (built.state.error !== null) throw built.state.error
  const lines = built.state.lines
  if (lines.length === 0) return ''
  return normalise(lines[0])
}

// ---------------------------------------------------------------------------
// main loop
// ---------------------------------------------------------------------------

function main () {
  const server = http.createServer(app)
  server.listen(0, '127.0.0.1', function () {
    const port = server.address().port
    const rl = readline.createInterface({ input: process.stdin })
    let chain = Promise.resolve()

    rl.on('line', function (line) {
      const text = line.trim()
      if (text === '') return
      chain = chain.then(async function () {
        let id = null
        try {
          const msg = JSON.parse(text)
          id = msg.id
          const spec = (msg.args || [])[0]
          if (!spec) throw new Error('missing spec argument')
          const value = await runCase(spec, port)
          process.stdout.write(JSON.stringify({ id: id, ok: true, value: value }) + '\n')
        } catch (err) {
          process.stdout.write(JSON.stringify({
            id: id,
            ok: false,
            error: String((err && err.message) || err)
          }) + '\n')
        }
      })
    })

    rl.on('close', function () {
      chain.then(function () { server.close(); process.exit(0) })
    })
  })
}

main()
