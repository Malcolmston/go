# rsc testdata

## `*.flight` — reference payloads

Each file is a Flight payload **produced by React itself**, captured verbatim.
`parity_test.go` builds the equivalent tree in Go and asserts this encoder
produces the same thing.

They exist because the Flight format is undocumented. The only trustworthy
description of it is what React emits, so that is what is checked in — and a
diff here after a React upgrade is the signal that the wire format moved.

To regenerate, in a scratch directory with `react` and
`react-server-dom-webpack` installed at the target version:

```js
// fixtures.mjs — run with:
//   NODE_ENV=production NODE_OPTIONS="--conditions react-server" node fixtures.mjs
import { renderToPipeableStream, registerClientReference } from 'react-server-dom-webpack/server.node';
import React from 'react';
import { Writable } from 'node:stream';
import fs from 'node:fs';

const Button = registerClientReference(function () {}, '/app/Button.js', 'Button');
const manifest = {
  '/app/Button.js#Button': { id: './src/Button.js', chunks: ['static/button.js'], name: 'Button' },
};

// …build the same trees the tests in parity_test.go build…

function collect(name, tree) {
  return new Promise((resolve, reject) => {
    const bufs = [];
    const sink = new Writable({ write(c, e, cb) { bufs.push(c); cb(); } });
    sink.on('finish', () => { fs.writeFileSync(`${name}.flight`, Buffer.concat(bufs)); resolve(); });
    renderToPipeableStream(tree, manifest, { onError: reject }).pipe(sink);
  });
}
```

`NODE_ENV=production` matters: the development build emits extra debug rows
(stack traces, owner information) that the production client refuses to read.

## `roundtrip/` — the live compatibility check

A payload compared against a captured payload only proves the two agree. The
round trip proves the thing that actually matters: that **React's own decoder
accepts what this encoder writes**.

```sh
cd roundtrip && npm install
RSC_ROUNDTRIP=1 go test ./rsc/...
```

Without `RSC_ROUNDTRIP` the round-trip tests skip, so the package's normal test
run needs no Node and no network. `node_modules/` is gitignored.
