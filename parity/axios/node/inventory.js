// Mechanical upstream API inventory for COVERAGE.md.
// Run: node inventory.js   (from parity/axios/node)
const ax = require('axios');
const fs = require('fs');
const skip = new Set(['constructor', 'default', '_request', 'length', 'name', 'prototype',
  'toString', 'apply', 'bind', 'call', 'caller', 'arguments']);
const rows = [];
const add = (p, k) => { if (!skip.has(k)) rows.push(p + k); };
Object.keys(ax).forEach((k) => add('axios.', k));
Object.getOwnPropertyNames(ax.Axios.prototype).forEach((k) => add('Axios#', k));
Object.getOwnPropertyNames(ax.AxiosHeaders.prototype).forEach((k) => add('AxiosHeaders#', k));
Object.keys(ax.AxiosError).forEach((k) => add('AxiosError.', k));
Object.getOwnPropertyNames(ax.AxiosError.prototype).forEach((k) => add('AxiosError#', k));
Object.getOwnPropertyNames(ax.CancelToken.prototype).forEach((k) => add('CancelToken#', k));
Object.keys(ax.defaults).forEach((k) => add('defaults.', k));
Object.keys(ax.defaults.headers).forEach((k) => add('defaults.headers.', k));
Object.keys(ax.defaults.transitional).forEach((k) => add('transitional.', k));
const dts = fs.readFileSync('node_modules/axios/index.d.ts', 'utf8');
const blk = dts.match(/export interface AxiosRequestConfig<[^>]*> \{([\s\S]*?)\n\}/)[1];
[...blk.matchAll(/^  ([a-zA-Z]+)\??:/gm)].forEach((m) => add('config.', m[1]));
console.error('symbols: ' + rows.length);
console.log(rows.join('\n'));
