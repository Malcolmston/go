// The case encoding, decoded into real React elements.
//
// A case's `args[0]` is a *node*, described in language-agnostic JSON so that the
// upstream runner here and the Go runner in ../go/run.go build the same tree from
// the same bytes. The encoding is documented in ../COVERAGE.md; the two builders
// are the normative statement of it.
//
//   null | true | false        -> renders nothing
//   string                     -> a text node
//   number                     -> a text node (numeric formatting is under test)
//   [node, ...]                -> an array child, spliced in place
//   {"type": t, "props": p, "children": [...]}
//                              -> an element. `t` is an HTML tag name, or one of
//                                 the reserved pseudo-types below.
//
// Reserved pseudo-types:
//   "#fragment"   React.Fragment / react.Fragment
//   "#component"  a function component that returns exactly its children, so a
//                 case can check that components are transparent to the
//                 serializer.
//
// Prop values are JSON values, with two wrappers for things JSON cannot say:
//   {"$undefined": true}  -> JS `undefined` / Go `nil`
//   {"$html": "..."}      -> the dangerouslySetInnerHTML payload
// and one structural value: the `style` prop takes a plain JSON object, whose
// values are strings, numbers, null or the wrappers above.

import React from 'react';

const UNDEFINED = Symbol('undefined-wrapper');

function isPlainObject(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}

// Transparent function component: whatever children it was handed.
function PassThrough(props) {
  return props.children === undefined ? null : props.children;
}
PassThrough.displayName = 'PassThrough';

function decodeValue(v) {
  if (isPlainObject(v)) {
    if (v.$undefined === true) return UNDEFINED;
    if (typeof v.$html === 'string') return { __html: v.$html };
    // A style map (or any other object-valued prop): decode member-wise.
    const out = {};
    for (const k of Object.keys(v)) {
      const dv = decodeValue(v[k]);
      if (dv === UNDEFINED) {
        out[k] = undefined;
        continue;
      }
      out[k] = dv;
    }
    return out;
  }
  return v;
}

function decodeProps(props) {
  if (!props) return null;
  const out = {};
  // Object.keys preserves insertion order for string keys, and the case files
  // are written with prop keys in ascending byte order precisely so that
  // React's insertion-ordered output lines up with the port's sorted output.
  for (const k of Object.keys(props)) {
    const v = decodeValue(props[k]);
    out[k] = v === UNDEFINED ? undefined : v;
  }
  return out;
}

export function buildNode(spec) {
  if (spec === null) return null;
  if (Array.isArray(spec)) return spec.map(buildNode);
  const t = typeof spec;
  if (t === 'string' || t === 'number' || t === 'boolean') return spec;
  if (!isPlainObject(spec)) {
    throw new Error(`unrepresentable node: ${JSON.stringify(spec)}`);
  }
  if (spec.$undefined === true) return undefined;
  if (typeof spec.type !== 'string') {
    throw new Error(`element spec needs a string "type": ${JSON.stringify(spec)}`);
  }

  let type;
  switch (spec.type) {
    case '#fragment':
      type = React.Fragment;
      break;
    case '#component':
      type = PassThrough;
      break;
    default:
      type = spec.type;
  }

  const props = decodeProps(spec.props);
  const kids = spec.children === undefined ? [] : spec.children.map(buildNode);
  if (kids.length === 0) return React.createElement(type, props);
  return React.createElement(type, props, ...kids);
}
