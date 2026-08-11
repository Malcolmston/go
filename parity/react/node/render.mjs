// The oracle itself: one case in, one answer out, using real react-dom/server.
//
// Kept separate from run.mjs so the vitest suite (parity.test.mjs) can call it
// in-process while the JSON-Lines runner used by parity_test.go wraps the same
// function.

import { renderToStaticMarkup, renderToString } from 'react-dom/server';
import { buildNode } from './tree.mjs';

export const VERSION = 'react@19.2.7 / react-dom@19.2.7';

/**
 * @param {{fn: string, args: any[]}} req
 * @returns {{ok: true, value: any} | {ok: false, error: string}}
 */
export function answer(req) {
  try {
    const node = buildNode(req.args[0]);
    switch (req.fn) {
      case 'renderToStaticMarkup':
        return { ok: true, value: renderToStaticMarkup(node) };
      case 'renderToString':
        return { ok: true, value: renderToString(node) };
      default:
        throw new Error(`unknown fn ${req.fn}`);
    }
  } catch (err) {
    return { ok: false, error: String((err && err.message) || err) };
  }
}
