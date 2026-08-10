// Upstream runner for the jose parity harness.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- every throw is reported as {ok:false,error}.
//
//   node run.mjs <keysDir>
//
// Upstream: jose@5.9.6 (panva/jose), ESM-only.

import * as jose from 'jose'
import { readFileSync } from 'node:fs'
import { createInterface } from 'node:readline'
import path from 'node:path'

const KEYS = process.argv[2] ?? path.join(import.meta.dirname, '..', 'keys')

const td = new TextDecoder()
const te = new TextEncoder()

const jwkCache = new Map()
function jwkOf(ref) {
  if (typeof ref !== 'string') throw new TypeError('key reference must be a string')
  if (!jwkCache.has(ref)) {
    jwkCache.set(ref, JSON.parse(readFileSync(path.join(KEYS, ref + '.json'), 'utf8')))
  }
  return structuredClone(jwkCache.get(ref))
}

// sortDeep gives every emitted object a stable key order so the harness can
// compare values without worrying about map iteration order.
function sortDeep(v) {
  if (Array.isArray(v)) return v.map(sortDeep)
  if (v && typeof v === 'object' && v.constructor === Object) {
    const out = {}
    for (const k of Object.keys(v).sort()) out[k] = sortDeep(v[k])
    return out
  }
  return v
}

const bytes = (s) => te.encode(s)
const text = (u8) => td.decode(u8)
const hex = (u8) => Buffer.from(u8).toString('hex')

// ---------------------------------------------------------------- key loading

// jwsKey passes the JWK straight through: jose enforces the JWK "use",
// "key_ops" and "alg" constraints itself on the JWS paths, which is exactly
// what the misuse cases are there to observe.
function jwsKey(ref) {
  return jwkOf(ref)
}

// jweKey has to import first: jose's JWE key management rejects bare JWK
// objects (lib/check_key_type.js is bound with allowJwk=false there).
async function jweKey(ref, alg) {
  const jwk = jwkOf(ref)
  if (jwk.kty === 'oct') return jose.base64url.decode(jwk.k)
  return await jose.importJWK(jwk, alg)
}

// ------------------------------------------------------------------- headers

function protectedHeaderFor(o) {
  const h = { ...(o.protected ?? {}) }
  if (o.alg !== undefined) h.alg = o.alg
  if (o.enc !== undefined) h.enc = o.enc
  if (o.kid !== undefined) h.kid = o.kid
  if (o.zip !== undefined) h.zip = o.zip
  if (o.b64 !== undefined) h.b64 = o.b64
  if (o.crit !== undefined) h.crit = o.crit
  // JOSE puts no ordering requirement on header members, but the protected
  // header is serialized before it is signed, so the two implementations can
  // only be compared byte-for-byte if both emit the same member order. The Go
  // port marshals Go maps, which encoding/json always sorts; sort here too.
  return sortDeep(h)
}

function kmParams(o) {
  const p = {}
  if (o.apu !== undefined) p.apu = jose.base64url.decode(o.apu)
  if (o.apv !== undefined) p.apv = jose.base64url.decode(o.apv)
  if (o.p2s !== undefined) p.p2s = jose.base64url.decode(o.p2s)
  if (o.p2c !== undefined) p.p2c = o.p2c
  if (o.iv !== undefined) p.iv = jose.base64url.decode(o.iv)
  return Object.keys(p).length ? p : undefined
}

// consumeOptions maps the case's shared option names onto jose's verify /
// decrypt option bag.
function consumeOptions(o) {
  const opts = {}
  if (o.algorithms) opts.algorithms = o.algorithms
  if (o.encryptions) opts.contentEncryptionAlgorithms = o.encryptions
  if (o.keyManagement) opts.keyManagementAlgorithms = o.keyManagement
  if (o.knownCrit) {
    opts.crit = {}
    for (const n of o.knownCrit) opts.crit[n] = true
  }
  return opts
}

const mergeHeaders = (...hs) => sortDeep(Object.assign({}, ...hs.filter(Boolean)))

// ----------------------------------------------------------------- operations

const ops = {
  // -- JWS ------------------------------------------------------------------

  async 'sign.compact'([payload, keyRef, o = {}]) {
    const s = new jose.CompactSign(bytes(payload)).setProtectedHeader(protectedHeaderFor(o))
    return { out: await s.sign(jwsKey(keyRef), produceOptions(o)) }
  },

  async 'verify.compact'([token, keyRef, o = {}]) {
    if (o.detachedPayload !== undefined) {
      // jose has no detached-payload option on compactVerify; the documented
      // way (README "Detached Payload") is to split the compact form and hand
      // the payload to flattenedVerify.
      const [protectedHeader, , signature] = token.split('.')
      const payload = o.b64 === false ? bytes(o.detachedPayload) : jose.base64url.encode(bytes(o.detachedPayload))
      const r = await jose.flattenedVerify(
        { protected: protectedHeader, payload, signature },
        jwsKey(keyRef),
        consumeOptions(o),
      )
      return { payload: text(r.payload), header: mergeHeaders(r.protectedHeader) }
    }
    const r = await jose.compactVerify(token, jwsKey(keyRef), consumeOptions(o))
    return { payload: text(r.payload), header: mergeHeaders(r.protectedHeader) }
  },

  async 'sign.flattened'([payload, keyRef, o = {}]) {
    const s = new jose.FlattenedSign(bytes(payload))
    s.setProtectedHeader(protectedHeaderFor(o))
    if (o.unprotected) s.setUnprotectedHeader(o.unprotected)
    let jws = await s.sign(jwsKey(keyRef), produceOptions(o))
    jws = { ...jws }
    if (o.detached) jws.payload = ''
    return { out: sortDeep(jws) }
  },

  async 'verify.flattened'([jws, keyRef, o = {}]) {
    const input = { ...jws }
    if (o.detachedPayload !== undefined) {
      input.payload = o.b64 === false ? bytes(o.detachedPayload) : jose.base64url.encode(bytes(o.detachedPayload))
    }
    const r = await jose.flattenedVerify(input, jwsKey(keyRef), consumeOptions(o))
    return {
      payload: text(r.payload),
      header: mergeHeaders(r.protectedHeader, r.unprotectedHeader),
    }
  },

  async 'sign.general'([payload, signers, o = {}]) {
    const s = new jose.GeneralSign(bytes(payload))
    for (const sg of signers) {
      const rec = s.addSignature(jwsKey(sg.key), sg.crit ? { crit: critMap(sg.crit) } : undefined)
      rec.setProtectedHeader(protectedHeaderFor(sg))
      if (sg.unprotected) rec.setUnprotectedHeader(sg.unprotected)
    }
    let jws = await s.sign()
    jws = { ...jws }
    if (o.detached) jws.payload = ''
    return { out: sortDeep(jws) }
  },

  async 'verify.general'([jws, keyRef, o = {}]) {
    const input = { ...jws }
    if (o.detachedPayload !== undefined) {
      input.payload = o.b64 === false ? bytes(o.detachedPayload) : jose.base64url.encode(bytes(o.detachedPayload))
    }
    const r = await jose.generalVerify(input, jwsKey(keyRef), consumeOptions(o))
    return {
      payload: text(r.payload),
      header: mergeHeaders(r.protectedHeader, r.unprotectedHeader),
    }
  },

  // -- JWE ------------------------------------------------------------------

  async 'encrypt.compact'([plaintext, keyRef, o = {}]) {
    const e = new jose.CompactEncrypt(bytes(plaintext)).setProtectedHeader(protectedHeaderFor(o))
    const km = kmParams(o)
    if (km) e.setKeyManagementParameters(km)
    return { out: await e.encrypt(await jweKey(keyRef, o.alg), produceOptions(o)) }
  },

  async 'decrypt.compact'([token, keyRef, o = {}]) {
    const r = await jose.compactDecrypt(token, await jweKey(keyRef, o.alg), consumeOptions(o))
    return { payload: text(r.plaintext), header: mergeHeaders(r.protectedHeader) }
  },

  async 'encrypt.flattened'([plaintext, keyRef, o = {}]) {
    const e = new jose.FlattenedEncrypt(bytes(plaintext)).setProtectedHeader(protectedHeaderFor(o))
    if (o.unprotected) e.setSharedUnprotectedHeader(o.unprotected)
    if (o.recipientHeader) e.setUnprotectedHeader(o.recipientHeader)
    if (o.aad !== undefined) e.setAdditionalAuthenticatedData(bytes(o.aad))
    const km = kmParams(o)
    if (km) e.setKeyManagementParameters(km)
    return { out: sortDeep(await e.encrypt(await jweKey(keyRef, o.alg), produceOptions(o))) }
  },

  async 'decrypt.flattened'([jwe, keyRef, o = {}]) {
    const r = await jose.flattenedDecrypt(jwe, await jweKey(keyRef, o.alg), consumeOptions(o))
    return {
      payload: text(r.plaintext),
      header: mergeHeaders(r.protectedHeader, r.sharedUnprotectedHeader, r.unprotectedHeader),
    }
  },

  async 'encrypt.general'([plaintext, recipients, o = {}]) {
    const e = new jose.GeneralEncrypt(bytes(plaintext)).setProtectedHeader(protectedHeaderFor(o))
    if (o.unprotected) e.setSharedUnprotectedHeader(o.unprotected)
    if (o.aad !== undefined) e.setAdditionalAuthenticatedData(bytes(o.aad))
    for (const r of recipients) {
      const rec = e.addRecipient(await jweKey(r.key, r.alg ?? o.alg))
      const h = {}
      if (r.alg !== undefined) h.alg = r.alg
      if (r.kid !== undefined) h.kid = r.kid
      Object.assign(h, r.header ?? {})
      if (Object.keys(h).length) rec.setUnprotectedHeader(h)
    }
    return { out: sortDeep(await e.encrypt()) }
  },

  async 'decrypt.general'([jwe, keyRef, o = {}]) {
    const r = await jose.generalDecrypt(jwe, await jweKey(keyRef, o.alg), consumeOptions(o))
    return {
      payload: text(r.plaintext),
      header: mergeHeaders(r.protectedHeader, r.sharedUnprotectedHeader, r.unprotectedHeader),
    }
  },

  // -- JWK ------------------------------------------------------------------

  async 'jwk.roundtrip'([ref, o = {}]) {
    const jwk = jwkOf(ref)
    const key = jwk.kty === 'oct' ? jose.base64url.decode(jwk.k) : await jose.importJWK(jwk, o.alg)
    return { out: sortDeep(await jose.exportJWK(key)) }
  },

  async 'jwk.thumbprint'([ref]) {
    return { out: await jose.calculateJwkThumbprint(jwkOf(ref), 'sha256') }
  },

  async 'jwk.thumbprintUri'([ref]) {
    return { out: await jose.calculateJwkThumbprintUri(jwkOf(ref), 'sha256') }
  },

  // jwk.public exports the public half of a private JWK.
  async 'jwk.public'([ref, o = {}]) {
    const jwk = jwkOf(ref)
    const alg = o.alg ?? jwk.alg
    const key = await jose.importJWK({ ...jwk, ext: true }, alg)
    const exported = await jose.exportJWK(key)
    for (const p of ['d', 'p', 'q', 'dp', 'dq', 'qi', 'k']) delete exported[p]
    return { out: sortDeep(exported) }
  },

  // jwks.lookup resolves a kid through a local JWK Set and reports the
  // thumbprint of the key that was selected, so both sides can agree on
  // *which* key was picked without exporting private material.
  async 'jwks.lookup'([ref, kid, o = {}]) {
    const set = jwkOf(ref)
    const resolve = jose.createLocalJWKSet(set)
    const key = await resolve({ alg: o.alg ?? 'RS256', kid })
    const exported = await jose.exportJWK(key)
    return { out: await jose.calculateJwkThumbprint(exported, 'sha256') }
  },

  // -- util -----------------------------------------------------------------

  async 'b64.encode'([hexIn]) {
    return { out: jose.base64url.encode(Buffer.from(hexIn, 'hex')) }
  },

  async 'b64.decode'([s]) {
    return { out: hex(jose.base64url.decode(s)) }
  },

  async 'header.protected'([token]) {
    return { out: sortDeep(jose.decodeProtectedHeader(token)) }
  },
}

// produceOptions declares the "crit" extension names a producer is allowed to
// emit; jose refuses to write an unrecognized one otherwise.
function produceOptions(o) {
  const names = o.knownCrit ?? o.crit
  return names ? { crit: critMap(names) } : undefined
}

function critMap(names) {
  const m = {}
  for (const n of names) m[n] = true
  return m
}

// ---------------------------------------------------------------------- loop

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity })
for await (const line of rl) {
  const trimmed = line.trim()
  if (!trimmed) continue
  let req
  try {
    req = JSON.parse(trimmed)
  } catch (e) {
    process.stdout.write(JSON.stringify({ id: null, ok: false, error: `bad request: ${e.message}` }) + '\n')
    continue
  }
  let res
  try {
    const op = ops[req.fn]
    if (!op) throw new Error(`unknown fn ${req.fn}`)
    const value = await op(req.args ?? [])
    res = { id: req.id, ok: true, value: sortDeep(value) }
  } catch (e) {
    res = { id: req.id, ok: false, error: `${e?.name ?? 'Error'}: ${e?.message ?? String(e)}` }
  }
  process.stdout.write(JSON.stringify(res) + '\n')
}
