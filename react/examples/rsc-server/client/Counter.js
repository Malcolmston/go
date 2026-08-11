'use client';

// The directive above is the whole declaration. This file never runs on the
// server: Go sends a *reference* to it, and the browser loads and renders it.
// That is why the state below actually works — this code runs in the browser,
// with a real React runtime, hooks and all.
//
// It is plain ES-module JavaScript rather than JSX so a browser can load it with
// no transform, which is what keeps this example free of a JavaScript build
// step. The trade-off is React.createElement instead of JSX — the same trade-off
// the Go side makes, for the same reason.

import React, { useState } from 'react';

export function Counter({ start = 0, label, children }) {
  const [count, setCount] = useState(start);

  return React.createElement(
    'div',
    { className: 'counter' },
    // `children` was rendered in Go and sent along with the props. Server and
    // client compose in both directions: Go markup can sit inside a JavaScript
    // component, and JavaScript components can sit inside Go markup.
    children,
    React.createElement(
      'button',
      { type: 'button', onClick: () => setCount((n) => n + 1) },
      label,
      ': ',
      count
    )
  );
}
