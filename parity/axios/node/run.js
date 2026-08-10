'use strict';
// Upstream parity runner for axios (node ecosystem).
// Reads one JSON object per line on stdin, writes exactly one JSON object per
// line to stdout. Never exits on a failing case.

const readline = require('readline');
const axios = require('axios');

const BASE = process.env.PARITY_BASE_URL || process.argv[2] || '';

function out(obj) {
  process.stdout.write(JSON.stringify(obj) + '\n');
}
function log(...a) {
  process.stderr.write(a.map(String).join(' ') + '\n');
}

function sub(s) {
  if (typeof s !== 'string') return s;
  return s.split('$BASE').join(BASE);
}

// ---------------------------------------------------------------- presets ---

const VALIDATE = {
  all: () => true,
  lt400: (s) => s < 400,
  lt500: (s) => s < 500,
  only418: (s) => s === 418,
  none: null,
};

const TRANSFORM_REQUEST = {
  // Operates on the already-serialised body so both ports can express it.
  prefix: (data) => 'PRE|' + (typeof data === 'string' ? data : JSON.stringify(data)),
  setHeader: (data, headers) => {
    headers['X-Tr'] = '1';
    return typeof data === 'string' ? data : JSON.stringify(data);
  },
  boom: () => {
    throw new Error('transformRequest boom');
  },
};

const TRANSFORM_RESPONSE = {
  identity: (data) => data,
  upper: (data) => (typeof data === 'string' ? data.toUpperCase() : data),
  wrap: (data) => 'WRAP|' + (typeof data === 'string' ? data : JSON.stringify(data)),
};

const REQ_INTERCEPTORS = {
  addHeader: (cfg) => {
    cfg.headers['X-Req-Ic'] = '1';
    return cfg;
  },
  addHeader2: (cfg) => {
    cfg.headers['X-Req-Ic2'] = '2';
    return cfg;
  },
  reject: () => {
    throw new Error('request interceptor rejected');
  },
};

const RES_INTERCEPTORS = {
  tagHeader: (res) => {
    res.headers['x-res-ic'] = '1';
    return res;
  },
  reject: () => {
    throw new Error('response interceptor rejected');
  },
};

function serializerFor(ps) {
  if (ps == null) return undefined;
  if (ps.custom === 'fixed') return { serialize: () => 'custom=1&x=Y' };
  if ('indexes' in ps) return { indexes: ps.indexes };
  return undefined;
}

// ------------------------------------------------------------ spec -> cfg ---

// Headers arrive as a plain object; nested objects are axios method groups
// (common/get/post/...) and are passed straight through.
function headersFor(h) {
  if (h == null) return undefined;
  return JSON.parse(JSON.stringify(h));
}

function bodyFor(d) {
  if (d == null) return undefined;
  if ('json' in d) return d.json;
  if ('text' in d) return d.text;
  if ('urlencoded' in d) {
    const p = new URLSearchParams();
    for (const k of Object.keys(d.urlencoded)) p.append(k, String(d.urlencoded[k]));
    return p;
  }
  if ('bytesHex' in d) return Buffer.from(d.bytesHex, 'hex');
  if ('form' in d) {
    const fd = new FormData();
    for (const k of Object.keys(d.form)) fd.append(k, String(d.form[k]));
    return fd;
  }
  return undefined;
}

function applySpec(spec, cfg) {
  if (!spec) return cfg;
  if (spec.baseURL !== undefined) cfg.baseURL = sub(spec.baseURL);
  if (spec.url !== undefined) cfg.url = sub(spec.url);
  if (spec.method !== undefined) cfg.method = spec.method;
  if (spec.headers !== undefined) cfg.headers = headersFor(spec.headers);
  if (spec.params !== undefined) cfg.params = spec.params;
  if (spec.paramsSerializer !== undefined) cfg.paramsSerializer = serializerFor(spec.paramsSerializer);
  if (spec.timeoutMs !== undefined) cfg.timeout = spec.timeoutMs;
  if (spec.validateStatus !== undefined) cfg.validateStatus = VALIDATE[spec.validateStatus];
  if (spec.maxRedirects !== undefined) cfg.maxRedirects = spec.maxRedirects;
  if (spec.auth !== undefined) cfg.auth = spec.auth;
  if (spec.responseType !== undefined) cfg.responseType = spec.responseType;
  if (spec.maxContentLength !== undefined) cfg.maxContentLength = spec.maxContentLength;
  if (spec.maxBodyLength !== undefined) cfg.maxBodyLength = spec.maxBodyLength;
  if (spec.decompress !== undefined) cfg.decompress = spec.decompress;
  if (spec.transformRequest !== undefined) cfg.transformRequest = [TRANSFORM_REQUEST[spec.transformRequest]];
  if (spec.transformResponse !== undefined) cfg.transformResponse = [TRANSFORM_RESPONSE[spec.transformResponse]];
  if (spec.data !== undefined) cfg.data = bodyFor(spec.data);
  return cfg;
}

function instanceFor(spec) {
  const cfg = applySpec(spec && spec.client, {});
  const inst = axios.create(cfg);
  const cs = (spec && spec.client) || {};
  for (const name of cs.interceptRequest || []) inst.interceptors.request.use(REQ_INTERCEPTORS[name]);
  for (const name of cs.interceptResponse || []) inst.interceptors.response.use(RES_INTERCEPTORS[name]);
  return inst;
}

// ------------------------------------------------------------- rendering ---

const DROP_RES_HEADERS = new Set(['date', 'connection', 'keep-alive', 'transfer-encoding', 'content-length']);

function renderResHeaders(h) {
  const o = {};
  const raw = h && typeof h.toJSON === 'function' ? h.toJSON() : h || {};
  for (const k of Object.keys(raw)) {
    const lk = k.toLowerCase();
    if (DROP_RES_HEADERS.has(lk)) continue;
    const v = raw[k];
    o[lk] = Array.isArray(v) ? v.join(', ') : String(v);
  }
  return o;
}

function renderData(res, expose) {
  if (expose === 'none') return null;
  if (expose === 'text') {
    return typeof res.data === 'string' ? res.data : JSON.stringify(res.data);
  }
  if (expose === 'auto') {
    const t = Array.isArray(res.data) ? 'array' : res.data === null ? 'null' : typeof res.data;
    return { dataType: t, text: typeof res.data === 'string' ? res.data : JSON.stringify(res.data) };
  }
  return res.data;
}

function renderOk(res, opts) {
  const v = { status: res.status, data: renderData(res, opts.expose || 'json'), resHeaders: renderResHeaders(res.headers) };
  if (opts.assertStatusText) v.statusText = res.statusText;
  if (opts.observeHeaders && res.data && res.data.headers) {
    v.pickHeaders = {};
    for (const n of opts.observeHeaders) v.pickHeaders[n] = res.data.headers[n] === undefined ? null : res.data.headers[n];
  }
  return v;
}

function renderErr(e) {
  const v = {
    isAxiosError: axios.isAxiosError(e) === true,
    isCancel: axios.isCancel(e) === true,
    code: e && e.code ? e.code : null,
    hasResponse: !!(e && e.response),
    status: e && e.response ? e.response.status : null,
  };
  return v;
}

// ------------------------------------------------------------------- fns ---

const FNS = {
  async request(a) {
    const spec = a[0] || {};
    const inst = instanceFor(spec);
    const rc = applySpec(spec.request, {});
    const res = await inst.request(rc);
    return renderOk(res, spec);
  },

  async getUri(a) {
    const spec = a[0] || {};
    const inst = instanceFor(spec);
    return inst.getUri(applySpec(spec.request, {}));
  },

  async mergeConfig(a) {
    const m = axios.mergeConfig(applySpec(a[0], {}), applySpec(a[1], {}));
    const hs = {};
    const raw = m.headers && typeof m.headers.toJSON === 'function' ? m.headers.toJSON() : m.headers || {};
    for (const k of Object.keys(raw).sort()) hs[k.toLowerCase()] = String(raw[k]);
    return {
      baseURL: m.baseURL === undefined ? null : m.baseURL,
      timeoutMs: m.timeout === undefined ? null : m.timeout,
      headers: hs,
      params: m.params === undefined ? null : m.params,
    };
  },

  async formToJSON(a) {
    const fd = new FormData();
    for (const [k, v] of a[0].pairs) fd.append(k, String(v));
    return axios.formToJSON(fd);
  },

  async all(a) {
    const spec = a[0] || {};
    const inst = instanceFor(spec);
    const results = await axios.all(a[0].requests.map((r) => inst.request(applySpec(r, {}))));
    return results.map((r) => r.status);
  },
};

// ------------------------------------------------------------------ loop ---

const rl = readline.createInterface({ input: process.stdin, terminal: false });
const queue = [];
let draining = false;

async function drain() {
  if (draining) return;
  draining = true;
  while (queue.length) {
    const line = queue.shift();
    let msg = null;
    try {
      msg = JSON.parse(line);
    } catch (e) {
      out({ id: '?', ok: false, error: 'bad input line: ' + e.message });
      continue;
    }
    try {
      const fn = FNS[msg.fn];
      if (!fn) throw new Error('unknown fn: ' + msg.fn);
      const value = await fn(msg.args || []);
      out({ id: msg.id, ok: true, value });
    } catch (e) {
      out({ id: msg.id, ok: false, error: (e && e.message) || String(e), value: renderErr(e) });
    }
  }
  draining = false;
}

rl.on('line', (line) => {
  if (!line.trim()) return;
  queue.push(line);
  drain().catch((e) => log('drain error', e));
});
rl.on('close', () => {
  drain().catch((e) => log('drain error', e));
});
process.on('unhandledRejection', (e) => log('unhandledRejection', e));
