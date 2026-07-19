// health.js — Vercel serverless health probe.
//
// GET /api/health -> { ok: true, es: <boolean> }
// The frontend uses this to detect whether the serverless API is reachable
// (and whether Elasticsearch is configured) before deciding to use the live
// API or the bundled fallback data. Permissive CORS; handles OPTIONS preflight.

import { esEnabled } from './_lib/es.js';

function setCors(res) {
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  res.setHeader('Access-Control-Max-Age', '86400');
}

export default function handler(req, res) {
  setCors(res);

  if ((req.method || 'GET').toUpperCase() === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }

  res.statusCode = 200;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.end(JSON.stringify({ ok: true, es: esEnabled() }));
}
