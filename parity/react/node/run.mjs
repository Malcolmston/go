// Upstream runner for parity/react, speaking the JSON-Lines stdio contract in
// parity/HARNESS.md: one request object per stdin line, exactly one reply object
// per stdout line, in order, never exiting on a failing case.
//
//   in  {"id":"el-nested","fn":"renderToStaticMarkup","args":[{"type":"div",...}]}
//   out {"id":"el-nested","ok":true,"value":"<div>…</div>"}
//
// React's development build writes invalid-prop warnings with console.error,
// which goes to stderr; stdout carries nothing but replies.

import { createInterface } from 'node:readline';
import { answer } from './render.mjs';

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });

for await (const line of rl) {
  const text = line.trim();
  if (!text) continue;
  let req;
  try {
    req = JSON.parse(text);
  } catch (err) {
    process.stdout.write(
      JSON.stringify({ id: '', ok: false, error: `bad request: ${err.message}` }) + '\n',
    );
    continue;
  }
  const rep = answer(req);
  process.stdout.write(JSON.stringify({ id: req.id, ...rep }) + '\n');
}
