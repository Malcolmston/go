// Decodes a Flight payload produced by the Go encoder using React's own client,
// then renders the result to static HTML.
//
// This is the test that matters: it does not check the payload against a
// description of the format, it hands the bytes to the real decoder and asks
// whether React can render them.
//
// Usage: node decode.mjs <payload-file>
import { createFromNodeStream } from 'react-server-dom-webpack/client.node';
import { renderToStaticMarkup } from 'react-dom/server';
import React from 'react';
import fs from 'node:fs';
import { Readable } from 'node:stream';

// A stand-in for the bundler runtime. A real app has webpack, Turbopack or Vite
// supply these; the round-trip only needs one module id to resolve to one real
// component, so that a client reference sent from Go can be shown to load and
// render with the props it was given.
const modules = {
  'client-button': {
    Button: ({ label, count, children }) =>
      React.createElement('button', { 'data-count': count }, label, children),
  },
};
globalThis.__webpack_require__ = (id) => modules[id];
globalThis.__webpack_chunk_load__ = () => Promise.resolve();

const serverConsumerManifest = {
  moduleMap: {
    './src/Button.js': { Button: { id: 'client-button', chunks: [], name: 'Button' } },
  },
  moduleLoading: { prefix: '' },
};

const stream = Readable.from([fs.readFileSync(process.argv[2])]);
const element = await createFromNodeStream(stream, serverConsumerManifest);
process.stdout.write(renderToStaticMarkup(element));
