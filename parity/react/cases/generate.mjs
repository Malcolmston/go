// Generates the case files in this directory.
//
//   cd parity/react/node && node ../cases/generate.mjs
//
// The cases are committed JSON — this script exists so that one invariant is
// mechanical rather than remembered: **prop keys are written in ascending byte
// order**. React emits attributes in the JavaScript object's insertion order,
// while the port sorts them (react/ssr.go: Go maps have no order, so sorting is
// the only way the same tree renders to the same bytes twice). Authoring props
// pre-sorted makes the two orders coincide, so every case measures serialization
// rather than measuring that difference 150 times over. The one case that *does*
// measure it, `order-insertion`, opts out via `rawProps` and is marked as a
// deviation.
//
// Nothing here encodes an expected answer. Both runners are fed these trees and
// their answers are compared; upstream is the oracle.

import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const UPSTREAM = 'react@19.2.7 react-dom@19.2.7';
const here = dirname(fileURLToPath(import.meta.url));

/** sortProps deep-sorts object keys so JSON.stringify emits them in byte order. */
function sortProps(v) {
  if (Array.isArray(v)) return v.map(sortProps);
  if (v === null || typeof v !== 'object') return v;
  const out = {};
  for (const k of Object.keys(v).sort()) out[k] = sortProps(v[k]);
  return out;
}

/** el builds an element spec. props keys are sorted unless raw is requested. */
function el(type, props, ...children) {
  const spec = { type };
  if (props && Object.keys(props).length) spec.props = sortProps(props);
  if (children.length) spec.children = children;
  return spec;
}
function rawEl(type, props, ...children) {
  const spec = { type };
  if (props) spec.props = props;
  if (children.length) spec.children = children;
  return spec;
}
const frag = (...children) => el('#fragment', null, ...children);
const comp = (...children) => el('#component', null, ...children);
const undef = { $undefined: true };
const html = (s) => ({ $html: s });

const files = new Map();
function group(name, note) {
  const cases = [];
  files.set(name, { group: name, upstream: UPSTREAM, note, cases });
  return (id, node, extra = {}) => {
    cases.push({ id, fn: extra.fn || 'renderToStaticMarkup', args: [node], ...omit(extra, 'fn') });
  };
}
function omit(o, k) {
  const { [k]: _drop, ...rest } = o;
  return rest;
}

const SSM = {
  upstreamFn: 'ReactDOMServer.renderToStaticMarkup',
  goFn: 'react.RenderToStaticMarkup',
};
const RTS = {
  fn: 'renderToString',
  upstreamFn: 'ReactDOMServer.renderToString',
  goFn: 'react.RenderToString',
};
const CE = { upstreamFn: 'React.createElement', goFn: 'react.CreateElement' };
const F = (feature, extra = {}) => ({ ...SSM, feature, ...extra });

// ------------------------------------------------------------------ elements
{
  const c = group('elements', 'Element and child-model serialization: nesting, fragments, arrays, falsy children, numbers.');
  c('el-empty-div', el('div'), F('createElement/host', CE));
  c('el-text-child', el('div', null, 'hello'), F('createElement/host', CE));
  c('el-two-elements', el('div', null, el('span', null, 'a'), el('b', null, 'b')), F('createElement/host'));
  c('el-nested-3', el('div', null, el('span', null, el('i', null, 'x'))), F('createElement/host'));
  c('el-nested-6', el('a', null, el('b', null, el('c', null, el('d', null, el('e', null, el('f', null, 'deep')))))), F('createElement/host'));
  c('el-mixed-text-element', el('p', null, 'a', el('em', null, 'b'), 'c'), F('createElement/host'));
  c('el-sibling-list', el('ul', null, el('li', null, '1'), el('li', null, '2'), el('li', null, '3')), F('createElement/host'));
  c('el-with-props-and-children', el('div', { className: 'box', id: 'root' }, 'x'), F('createElement/props'));
  c('el-fragment-top', frag('a', 'b'), { ...F('Fragment'), upstreamFn: 'React.Fragment', goFn: 'react.Fragment' });
  c('el-fragment-nested', el('div', null, frag(el('span', null, 'a'), el('span', null, 'b'))), { ...F('Fragment'), upstreamFn: 'React.Fragment', goFn: 'react.Fragment' });
  c('el-fragment-empty', el('div', null, frag()), { ...F('Fragment'), upstreamFn: 'React.Fragment', goFn: 'react.Fragment' });
  c('el-fragment-of-fragments', frag(frag('a'), frag(frag('b'))), { ...F('Fragment'), upstreamFn: 'React.Fragment', goFn: 'react.Fragment' });
  c('el-array-child', el('div', null, ['a', 'b', 'c']), F('array children'));
  c('el-array-nested', el('div', null, [['a', 'b'], 'c', [['d']]]), F('array children'));
  c('el-array-of-elements', el('div', null, [el('i', { key: 'a' }, '1'), el('i', { key: 'b' }, '2')]), F('array children + key'));
  c('el-array-keyed-top', [el('i', { key: '1' }, 'a'), el('b', { key: '2' }, 'b')], F('array children + key'));
  c('el-array-empty', el('div', null, []), F('array children'));
  c('el-child-null', el('div', null, null), F('falsy children'));
  c('el-child-undefined', el('div', null, undef), F('falsy children'));
  c('el-child-false', el('div', null, false), F('falsy children'));
  c('el-child-true', el('div', null, true), F('falsy children'));
  c('el-child-empty-string', el('div', null, ''), F('falsy children'));
  c('el-child-zero', el('div', null, 0), F('numeric children'));
  c('el-child-zero-string', el('div', null, '0'), F('numeric children'));
  c('el-child-falsy-mix', el('div', null, null, false, undef, '', 0, true, 'end'), F('falsy children'));
  c('el-child-int', el('div', null, 42), F('numeric children'));
  c('el-child-negative', el('div', null, -7), F('numeric children'));
  c('el-child-float', el('div', null, 1.5), F('numeric children'));
  c('el-child-float-repeating', el('div', null, 0.3333333333333333), F('numeric children'));
  c('el-child-small-float', el('div', null, 0.1), F('numeric children'));
  c('el-child-big-int', el('div', null, 9007199254740991), F('numeric children'));
  c('el-child-exponent', el('div', null, 1e21), {
    ...F('numeric children'),
    note: 'JS Number#toString switches to exponent form at 1e21; Go strconv.FormatFloat(-1) does not.',
  });
  c('el-child-numbers-adjacent', el('div', null, 1, 2, 3), F('numeric children'));
  c('el-component-passthrough', comp('a', el('b', null, 'c')), {
    ...F('function component'),
    upstreamFn: 'React function component',
    goFn: 'react.Component',
  });
  c('el-component-nested', el('div', null, comp(comp('x'))), F('function component'));
  c('el-component-empty', el('div', null, comp()), F('function component'));
  c('el-unknown-tag', el('my-element', { foo: 'bar' }, 'x'), F('custom element'));
  c('el-unknown-tag-nested', el('div', null, el('x-a', null, el('x-b', null, 'y'))), F('custom element'));
  c('el-textarea-children', el('textarea', null, 'hi'), F('textarea content'));
  c('el-p-empty-string-attr', el('p', { id: '' }, 'x'), F('empty attribute value'));
}

// ----------------------------------------------------------------- escaping
{
  const c = group('escaping', 'escapeTextForBrowser: the five escaped characters, in text and in attribute values, plus things that must NOT be touched.');
  const AMPS = `&<>"'`;
  c('esc-text-all-five', el('div', null, AMPS), F('escapeTextForBrowser'));
  c('esc-text-amp', el('div', null, 'a & b'), F('escapeTextForBrowser'));
  c('esc-text-lt', el('div', null, 'a < b'), F('escapeTextForBrowser'));
  c('esc-text-gt', el('div', null, 'a > b'), F('escapeTextForBrowser'));
  c('esc-text-quote', el('div', null, 'say "hi"'), F('escapeTextForBrowser'));
  c('esc-text-apostrophe', el('div', null, "it's"), {
    ...F('escapeTextForBrowser'),
    note: "React emits &#x27; for U+0027.",
  });
  c('esc-text-already-escaped', el('div', null, '&amp;'), F('escapeTextForBrowser'));
  c('esc-text-entity-lookalike', el('div', null, '&nbsp;&#39;&lt;'), F('escapeTextForBrowser'));
  c('esc-text-bare-amp-semicolon', el('div', null, 'a&;b'), F('escapeTextForBrowser'));
  c('esc-text-script-close', el('div', null, '</script><script>alert(1)</script>'), F('escapeTextForBrowser'));
  c('esc-text-script-inside-script', el('script', null, 'var a = 1 < 2;'), F('escapeTextForBrowser'));
  c('esc-text-html-comment', el('div', null, '<!-- comment -->'), F('escapeTextForBrowser'));
  c('esc-text-nbsp', el('div', null, 'a b'), F('escapeTextForBrowser'));
  c('esc-text-unicode', el('div', null, 'héllo — 日本語 🎉'), F('escapeTextForBrowser'));
  c('esc-text-control-chars', el('div', null, 'a\tb\nc\rd'), F('escapeTextForBrowser'));
  c('esc-text-line-separator', el('div', null, 'a b c'), F('escapeTextForBrowser'));
  c('esc-text-null-char', el('div', null, 'a b'), F('escapeTextForBrowser'));
  c('esc-attr-all-five', el('div', { title: AMPS }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-quote-breakout', el('div', { title: '" onmouseover="alert(1)' }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-apostrophe', el('div', { title: "it's" }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-lt-gt', el('div', { 'data-x': '<b>' }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-nbsp', el('div', { title: 'a b' }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-newline', el('div', { title: 'a\nb' }), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-url-amp', el('a', { href: '/x?a=1&b=2' }, 'l'), F('escapeTextForBrowser (attribute)'));
  c('esc-attr-javascript-url', el('a', { href: "javascript:alert('x')" }, 'l'), {
    ...F('escapeTextForBrowser (attribute)'),
    note: 'Neither implementation is expected to sanitize the scheme; the case records what they do.',
  });
  c('esc-attr-unicode', el('div', { title: '日本語 🎉' }), F('escapeTextForBrowser (attribute)'));
  c('esc-deep-text', el('div', null, el('span', null, 'a&b'), el('span', { title: 'c<d' }, 'e>f')), F('escapeTextForBrowser'));
}

// -------------------------------------------------------------------- void
{
  const c = group('void', 'Void elements: every one of the 13, self-closing output, and content on a void element (upstream throws).');
  const VOID = ['area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input', 'link', 'source', 'track', 'wbr'];
  for (const tag of VOID) {
    c(`void-${tag}`, el('div', null, el(tag)), F('void elements'));
  }
  // <meta> is hoisted into the document head by React 19 (Float) wherever it
  // appears, so nesting one measures hoisting rather than serialization. A bare
  // <link> with no rel is not hoisted, and stays an ordinary case above.
  for (const tag of ['meta']) {
    c(`void-${tag}`, el('div', null, el(tag)), {
      ...F('void elements'),
      deviation:
        'React 19 hoists a nested <' + tag + '> out of its parent into the head (Float); ' +
        'the port has no resource hoisting and emits it in place.',
    });
  }
  c('void-br-with-attr', el('div', null, el('br', { className: 'x' })), F('void elements'));
  c('void-input-attrs', el('input', { name: 'q' }), F('void elements'));
  c('void-hr-siblings', el('div', null, 'a', el('hr'), 'b'), F('void elements'));
  c('void-many', el('div', null, el('br'), el('br'), el('wbr')), F('void elements'));
  c('void-br-with-children', el('br', null, 'x'), {
    ...F('void elements'),
    note: 'React throws "br is a self-closing tag"; the port must fail too.',
  });
  c('void-img-with-children', el('img', null, el('span', null, 'x')), F('void elements'));
  c('void-input-with-raw-html', el('input', { dangerouslySetInnerHTML: html('<b>x</b>') }), F('void elements'));
  c('void-empty-non-void', el('div', null, el('span')), F('non-void empty element'));
  c('void-empty-non-void-p', el('p'), F('non-void empty element'));
  c('void-img-src', el('img', { alt: 'a', src: '/a.png' }), {
    ...F('void elements'),
    deviation: 'React 19 hoists a <link rel="preload" as="image"> for img[src] (Float); the port has no resource hoisting and emits only the img.',
  });
}

// -------------------------------------------------------------- attribute names
{
  const c = group('attr-names', 'The prop -> attribute name map: aliases, camelCase HTML attributes, SVG hyphenation, namespaced names, pass-through.');
  const A = (feature) => F(feature, { upstreamFn: 'ReactDOM property table', goFn: 'react.AttributeName' });
  const one = (id, tag, props) => c(id, el(tag, props), A('prop -> attribute name'));

  one('an-classname', 'div', { className: 'a b' });
  one('an-htmlfor', 'label', { htmlFor: 'x' });
  one('an-defaultvalue', 'input', { defaultValue: 'v' });
  one('an-tabindex', 'div', { tabIndex: -1 });
  one('an-tabindex-zero', 'div', { tabIndex: 0 });
  one('an-acceptcharset', 'form', { acceptCharset: 'utf-8' });
  one('an-httpequiv', 'meta', { content: '1', httpEquiv: 'refresh' });
  one('an-charset', 'meta', { charSet: 'utf-8' });
  one('an-colspan', 'td', { colSpan: 2 });
  one('an-rowspan', 'td', { rowSpan: 3 });
  one('an-maxlength', 'input', { maxLength: 10, minLength: 2 });
  one('an-crossorigin', 'script', { crossOrigin: 'anonymous', src: '/a.js' });
  one('an-datetime', 'time', { dateTime: '2026-01-01' });
  one('an-enctype', 'form', { encType: 'multipart/form-data' });
  one('an-formaction', 'button', { formAction: '/a', formMethod: 'post' });
  one('an-hreflang', 'a', { href: '/a', hrefLang: 'en' });
  one('an-inputmode', 'input', { inputMode: 'numeric' });
  one('an-autocomplete', 'input', { autoComplete: 'off' });
  one('an-autocapitalize', 'input', { autoCapitalize: 'none' });
  one('an-referrerpolicy', 'a', { href: '/a', referrerPolicy: 'no-referrer' });
  one('an-srcset', 'source', { srcSet: '/a.png 1x' });
  one('an-srcdoc', 'iframe', { srcDoc: '<p>x</p>' });
  one('an-srclang', 'track', { srcLang: 'en' });
  one('an-cellpadding', 'table', { cellPadding: 1, cellSpacing: 2 });
  one('an-usemap', 'img', { alt: '', useMap: '#m' });
  one('an-itemprop', 'div', { itemID: 'i', itemProp: 'p', itemRef: 'r', itemType: 't' });
  one('an-mediagroup', 'video', { mediaGroup: 'g' });
  one('an-radiogroup', 'input', { radioGroup: 'g' });
  one('an-keytype', 'input', { keyParams: 'k', keyType: 't' });
  one('an-classid', 'object', { classID: 'c' });
  one('an-frameborder', 'iframe', { frameBorder: '0', marginHeight: '1', marginWidth: '2' });
  one('an-nomodule', 'script', { noModule: true, src: '/a.js' });
  one('an-contextmenu', 'div', { contextMenu: 'm' });
  one('an-controlslist', 'video', { controlsList: 'nodownload' });
  one('an-imagesizes', 'link', { imageSizes: '100vw', imageSrcSet: '/a.png 1x', rel: 'x' });
  one('an-autocorrect', 'input', { autoCorrect: 'off' });
  one('an-lowercase-passthrough', 'div', { dir: 'rtl', id: 'a', lang: 'en', role: 'button', title: 't' });
  one('an-uppercase-passthrough', 'div', { ATTR: 'x' });
  one('an-hyphen-custom', 'div', { 'my-attr': 'x' });
  c('an-svg-hyphenated', el('svg', { viewBox: '0 0 2 2' }, el('path', { d: 'M0 0', fillOpacity: 0.5, fillRule: 'evenodd', strokeWidth: 2 })), A('SVG presentation attributes'));
  c('an-svg-camel-kept', el('svg', { preserveAspectRatio: 'none', viewBox: '0 0 1 1' }, el('linearGradient', { gradientUnits: 'userSpaceOnUse', id: 'g' })), A('SVG camelCase attributes'));
  c('an-svg-text-anchor', el('svg', null, el('text', { dominantBaseline: 'middle', textAnchor: 'middle' }, 'x')), A('SVG presentation attributes'));
  c('an-svg-clip-path', el('svg', null, el('g', { clipPath: 'url(#c)', clipRule: 'nonzero' })), A('SVG presentation attributes'));
  c('an-svg-marker', el('svg', null, el('path', { d: 'M0 0', markerEnd: 'url(#m)', markerMid: 'url(#m)', markerStart: 'url(#m)' })), A('SVG presentation attributes'));
  c('an-xlink-href', el('svg', null, el('use', { xlinkHref: '#a' })), A('namespaced attributes'));
  c('an-xml-lang', el('svg', { xmlLang: 'en', xmlSpace: 'preserve' }), A('namespaced attributes'));
  c('an-font-family', el('svg', null, el('text', { fontFamily: 'serif', fontSize: 12, fontWeight: 700 }, 'x')), A('SVG presentation attributes'));
  one('an-event-handler-dropped', 'div', { onclick: 'alert(1)' });
  one('an-onx-prefixed-custom', 'div', { once: 'x' });
  one('an-key-not-emitted', 'div', { id: 'a', key: 'k' });
  one('an-ref-not-emitted', 'div', { id: 'a' });
}

// ------------------------------------------------------------- attribute values
{
  const c = group('attr-values', 'Attribute value rules: boolean attributes, enumerated attributes, and the values that drop the attribute entirely.');
  const V = (feature) => F(feature);
  const one = (id, tag, props, extra = {}) => c(id, el(tag, props), { ...V('attribute values'), ...extra });

  one('av-disabled-true', 'input', { disabled: true });
  one('av-disabled-false', 'input', { disabled: false });
  one('av-disabled-null', 'input', { disabled: null });
  one('av-disabled-undefined', 'input', { disabled: undef });
  one('av-disabled-string', 'input', { disabled: 'disabled' });
  one('av-checked-true', 'input', { checked: true });
  one('av-checked-false', 'input', { checked: false });
  one('av-defaultchecked', 'input', { defaultChecked: true });
  one('av-readonly-true', 'input', { readOnly: true });
  one('av-required', 'input', { required: true });
  one('av-multiple', 'select', { multiple: true });
  one('av-hidden-true', 'div', { hidden: true });
  one('av-hidden-string', 'div', { hidden: 'until-found' });
  one('av-open', 'details', { open: true });
  one('av-async-defer', 'script', { async: true, defer: true, src: '/a.js' });
  one('av-autofocus', 'input', { autoFocus: true });
  one('av-autoplay-loop-muted', 'video', { autoPlay: true, loop: true, muted: true });
  one('av-controls', 'video', { controls: true });
  one('av-novalidate', 'form', { noValidate: true });
  one('av-formnovalidate', 'button', { formNoValidate: true });
  one('av-allowfullscreen', 'iframe', { allowFullScreen: true });
  one('av-playsinline', 'video', { playsInline: true });
  one('av-itemscope', 'div', { itemScope: true });
  one('av-reversed', 'ol', { reversed: true });
  one('av-selected', 'option', { selected: true });
  one('av-contenteditable-true', 'div', { contentEditable: 'true' });
  one('av-contenteditable-bool', 'div', { contentEditable: true });
  one('av-contenteditable-false-bool', 'div', { contentEditable: false });
  one('av-draggable-true', 'div', { draggable: true });
  one('av-draggable-false', 'div', { draggable: false });
  one('av-spellcheck-false', 'div', { spellCheck: false });
  one('av-custom-attr-true', 'div', { foo: true }, {
    note: 'React drops a boolean on a non-boolean, non-data/aria attribute (dev build also warns).',
  });
  one('av-custom-attr-false', 'div', { foo: false });
  one('av-null-drops', 'div', { id: null, lang: 'en' });
  one('av-undefined-drops', 'div', { lang: 'en', title: undef });
  one('av-empty-string-kept', 'div', { id: '' });
  one('av-number-int', 'div', { 'data-n': 3 });
  one('av-number-float', 'div', { 'data-n': 1.5 });
  one('av-number-zero', 'div', { 'data-n': 0 });
  one('av-number-negative', 'div', { 'data-n': -1 });
  one('av-number-exponent', 'div', { 'data-n': 1e21 }, {
    note: 'Same JS-vs-Go number formatting boundary as el-child-exponent.',
  });
  one('av-value-input', 'input', { defaultValue: 'v' });
  one('av-value-number', 'input', { defaultValue: 7 });
  one('av-alt-empty', 'img', { alt: '' });
  one('av-input-type-order', 'input', { name: 'q', type: 'text' }, {
    deviation: "React's input serializer emits type first and value/checked last regardless of prop order (ReactDOM pushInput); the port emits every prop in one sorted pass.",
  });
  one('av-option-value-order', 'option', { selected: true, value: 'a' }, {
    deviation: "React's option serializer emits selected after value; the port sorts prop names.",
  });
  one('av-multi-attrs', 'a', { className: 'c', href: '/h', id: 'i', rel: 'noopener', target: '_blank', title: 't' });
  c('order-insertion', rawEl('div', { id: 'a', className: 'b', title: 'c' }), {
    ...V('attribute output order'),
    deviation: 'React emits attributes in JS object insertion order; the port sorts prop names, because Go maps have no order (react/ssr.go). Every other case is authored with prop keys pre-sorted so the two agree.',
  });
}

// ---------------------------------------------------------------- data / aria
{
  const c = group('data-aria', 'data-* and aria-* pass through untranslated, including the boolean values React keeps for them.');
  const D = F('data-*/aria-* pass-through');
  const one = (id, props) => c(id, el('div', props), D);
  one('da-data-simple', { 'data-x': 'y' });
  one('da-data-multiple', { 'data-a': '1', 'data-b': '2', 'data-c': '3' });
  one('da-data-hyphenated', { 'data-my-long-name': 'v' });
  one('da-data-number', { 'data-n': 5 });
  one('da-data-true', { 'data-flag': true });
  one('da-data-false', { 'data-flag': false });
  one('da-data-null', { 'data-flag': null });
  one('da-data-empty', { 'data-flag': '' });
  one('da-data-escaped', { 'data-json': '{"a":1,"b":"<x>"}' });
  one('da-aria-label', { 'aria-label': 'Close' });
  one('da-aria-hidden-true', { 'aria-hidden': true });
  one('da-aria-hidden-false', { 'aria-hidden': false });
  one('da-aria-hidden-string', { 'aria-hidden': 'true' });
  one('da-aria-describedby', { 'aria-describedby': 'a b', 'aria-labelledby': 'c' });
  one('da-aria-level-number', { 'aria-level': 2, role: 'heading' });
  one('da-aria-null', { 'aria-label': null, id: 'x' });
  c('da-combined', el('button', { 'aria-pressed': false, className: 'btn', 'data-id': '7', type: 'button' }, 'Go'), D);
}

// -------------------------------------------------------------------- style
{
  const c = group('style', 'The style prop: camelCase -> kebab-case, the px/unitless rule, dropped declarations, vendor prefixes, custom properties.');
  const S = F('style prop serialization', { upstreamFn: 'ReactDOM style serializer', goFn: 'react.FormatStyle' });
  const one = (id, style, extra = {}) => c(id, el('div', { style }), { ...S, ...extra });

  one('st-single', { color: 'red' });
  one('st-two', { color: 'red', display: 'block' });
  one('st-camel-to-kebab', { backgroundColor: 'blue' });
  one('st-camel-long', { borderBottomLeftRadius: '2px' });
  one('st-already-kebab', { 'background-color': 'blue' });
  one('st-px-width', { width: 10 });
  one('st-px-float', { width: 1.5 });
  one('st-px-negative', { marginTop: -5 });
  one('st-px-zero', { margin: 0 });
  one('st-px-zero-float', { margin: 0.0 });
  one('st-unitless-zindex', { zIndex: 3 });
  one('st-unitless-flex', { flex: 1 });
  one('st-unitless-flexgrow', { flexGrow: 2, flexShrink: 0 });
  one('st-unitless-order', { order: 1 });
  one('st-unitless-opacity', { opacity: 0.5 });
  one('st-unitless-lineheight', { lineHeight: 1.5 });
  one('st-unitless-fontweight', { fontWeight: 700 });
  one('st-unitless-columncount', { columnCount: 2 });
  one('st-unitless-tabsize', { tabSize: 4 });
  one('st-unitless-orphans-widows', { orphans: 2, widows: 3 });
  one('st-unitless-aspectratio', { aspectRatio: 2 });
  one('st-unitless-zoom', { zoom: 2 });
  one('st-unitless-gridrow', { gridRow: 2 });
  one('st-unitless-strokewidth', { strokeWidth: 2 });
  one('st-unitless-fillopacity', { fillOpacity: 0.25 });
  one('st-unitless-animationiteration', { animationIterationCount: 3 });
  one('st-unitless-lineclamp', { WebkitLineClamp: 3 });
  one('st-string-number', { width: '10' });
  one('st-numeric-string-with-unit', { width: '10rem' });
  one('st-vendor-webkit', { WebkitTransform: 'rotate(1deg)' });
  one('st-vendor-ms', { msTransform: 'rotate(1deg)' });
  one('st-vendor-moz', { MozAppearance: 'none' });
  one('st-custom-property', { '--brand': '#fff' });
  one('st-custom-property-number', { '--count': 3 });
  one('st-custom-and-normal', { '--x': '1', color: 'red' });
  one('st-null-drops', { color: null, display: 'block' });
  one('st-undefined-drops', { color: undef, display: 'block' });
  one('st-empty-string-drops', { color: '', display: 'block' });
  one('st-all-dropped', { color: null, width: undef });
  one('st-empty-object', {});
  one('st-value-with-quote', { fontFamily: '"My Font", serif' });
  one('st-value-with-semicolon-escape', { content: '"a;b"' });
  one('st-value-with-url', { backgroundImage: 'url(/a.png?a=1&b=2)' });
  one('st-value-calc', { width: 'calc(100% - 10px)' });
  one('st-value-important', { color: 'red !important' });
  one('st-value-multi-comma', { transition: 'color 1s, background 2s' });
  one('st-value-bool', { width: true }, {
    note: 'React drops a boolean style value; the port errors.',
  });
  one('st-many', {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginTop: 8,
    zIndex: 10,
  });
  c('st-string-value', el('div', { style: 'color:red' }), {
    ...S,
    note: 'React throws for a string style prop; the port accepts it as a pre-formatted declaration list.',
  });
  c('st-nested-elements', el('div', { style: { color: 'red' } }, el('span', { style: { fontSize: 12 } }, 'x')), S);
  c('st-svg-style', el('svg', null, el('rect', { style: { fill: 'red', strokeWidth: 2 } })), S);
}

// ------------------------------------------------------------------- raw html
{
  const c = group('raw-html', 'dangerouslySetInnerHTML: written through byte for byte, and the error cases around it.');
  const R = F('dangerouslySetInnerHTML', { upstreamFn: 'props.dangerouslySetInnerHTML', goFn: 'react.DangerousHTML' });
  c('dsi-simple', el('div', { dangerouslySetInnerHTML: html('<b>bold</b>') }), R);
  c('dsi-unescaped-amp', el('div', { dangerouslySetInnerHTML: html('a & b < c') }), R);
  c('dsi-empty', el('div', { dangerouslySetInnerHTML: html('') }), R);
  c('dsi-script', el('div', { dangerouslySetInnerHTML: html('<script>alert(1)</script>') }), R);
  c('dsi-with-attrs', el('div', { className: 'c', dangerouslySetInnerHTML: html('<i>x</i>'), id: 'i' }), R);
  c('dsi-nested-parent', el('section', null, el('div', { dangerouslySetInnerHTML: html('<p>x</p>') }), el('span', null, 'after')), R);
  c('dsi-in-script-tag', el('script', { dangerouslySetInnerHTML: html('if (a < b) { x = "y"; }') }), R);
  c('dsi-in-style-tag', el('style', { dangerouslySetInnerHTML: html('a > b { color: red }') }), R);
  c('dsi-and-children', el('div', { dangerouslySetInnerHTML: html('<b>x</b>') }, 'child'), {
    ...R,
    note: 'React throws "Can only set one of children or props.dangerouslySetInnerHTML".',
  });
  c('dsi-null', el('div', { dangerouslySetInnerHTML: null }, 'child'), R);
  c('dsi-undefined', el('div', { dangerouslySetInnerHTML: undef }, 'child'), R);
  c('dsi-partial-tag', el('div', { dangerouslySetInnerHTML: html('<div>unclosed') }), R);
}

// ------------------------------------------------------------------ text flow
{
  const c = group('text-flow', 'Text ordering and whitespace: adjacent text nodes, preserved whitespace, and the renderToString/renderToStaticMarkup difference.');
  c('tf-adjacent-text', el('div', null, 'a', 'b'), F('adjacent text children'));
  c('tf-adjacent-text-three', el('div', null, 'a', 'b', 'c'), F('adjacent text children'));
  c('tf-text-element-text', el('div', null, 'a', el('b', null, 'x'), 'c'), F('child ordering'));
  c('tf-number-then-text', el('div', null, 1, 'a'), F('adjacent text children'));
  c('tf-leading-trailing-space', el('div', null, '  a  '), F('whitespace preservation'));
  c('tf-newlines', el('pre', null, 'line1\nline2\n  indented'), F('whitespace preservation'));
  c('tf-tabs', el('pre', null, 'a\tb'), F('whitespace preservation'));
  c('tf-only-space', el('span', null, ' '), F('whitespace preservation'));
  c('tf-space-between-elements', el('div', null, el('b', null, 'a'), ' ', el('b', null, 'b')), F('whitespace preservation'));
  c('tf-nbsp-between', el('div', null, el('b', null, 'a'), ' ', el('b', null, 'b')), F('whitespace preservation'));
  c('tf-order-deep', el('ol', null, el('li', null, '1', el('span', null, 'a')), el('li', null, el('span', null, 'b'), '2')), F('child ordering'));
  c('rts-simple', el('div', { id: 'x' }, 'hello'), { ...RTS, feature: 'renderToString' });
  c('rts-nested', el('div', null, el('span', null, 'a'), el('span', null, 'b')), { ...RTS, feature: 'renderToString' });
  c('rts-attrs', el('a', { className: 'c', href: '/h' }, 'l'), { ...RTS, feature: 'renderToString' });
  c('rts-void', el('div', null, el('br')), { ...RTS, feature: 'renderToString' });
  c('rts-adjacent-text', el('div', null, 'a', 'b'), {
    ...RTS,
    feature: 'renderToString',
    deviation: 'renderToString inserts a <!-- --> separator between adjacent text nodes so hydration can find the boundary. The port has no client and no hydration, so RenderToString is documented as an alias of RenderToStaticMarkup (react/ssr.go).',
  });
  c('rts-adjacent-numbers', el('div', null, 1, 2), {
    ...RTS,
    feature: 'renderToString',
    deviation: 'Same hydration text separator as rts-adjacent-text.',
  });
  c('rts-fragment-texts', frag('a', 'b'), {
    ...RTS,
    feature: 'renderToString',
    deviation: 'Same hydration text separator as rts-adjacent-text.',
  });
}

// ------------------------------------------------------------------- documents
{
  const c = group('document', 'Document-shaped trees: the head elements and page skeletons a server renderer actually emits.');
  c('doc-html-skeleton', el('html', { lang: 'en' }, el('head', null, el('title', null, 'T')), el('body', null, el('div', { id: 'root' }, 'x'))), F('document tree'));
  c('doc-title', el('title', null, 'Hello & goodbye'), F('document tree'));
  c('doc-meta-charset', el('meta', { charSet: 'utf-8' }), F('document tree'));
  c('doc-meta-viewport', el('meta', { content: 'width=device-width', name: 'viewport' }), F('document tree'));
  c('doc-link-stylesheet', el('link', { href: '/a.css', rel: 'stylesheet' }), F('document tree'));
  c('doc-style-tag', el('style', null, 'body{margin:0}'), F('document tree'));
  c('doc-table', el('table', null, el('thead', null, el('tr', null, el('th', { scope: 'col' }, 'H'))), el('tbody', null, el('tr', null, el('td', { colSpan: 2 }, 'C')))), F('document tree'));
  c('doc-form', el('form', { acceptCharset: 'utf-8', action: '/a', method: 'post' }, el('label', { htmlFor: 'q' }, 'Q'), el('input', { id: 'q', name: 'q', required: true, type: 'text' }), el('button', { type: 'submit' }, 'Go')), F('document tree'));
  c('doc-select', el('select', { name: 's' }, el('option', { value: '1' }, 'One'), el('option', { value: '2' }, 'Two')), F('document tree'));
  c('doc-svg-icon', el('svg', { fill: 'none', height: 16, viewBox: '0 0 16 16', width: 16, xmlns: 'http://www.w3.org/2000/svg' }, el('path', { d: 'M1 1L15 15', stroke: 'currentColor', strokeLinecap: 'round', strokeWidth: 2 })), F('document tree'));
  c('doc-email-table', el('table', { cellPadding: 0, cellSpacing: 0, role: 'presentation', style: { width: '100%' } }, el('tr', null, el('td', { align: 'center', style: { padding: 16 } }, 'Body'))), F('document tree'));
  c('doc-nested-components', comp(el('div', { className: 'page' }, comp(el('h1', null, 'Title')), comp(el('p', null, 'Text & more')))), F('function component'));
}

for (const [name, payload] of files) {
  writeFileSync(join(here, `${name}.json`), JSON.stringify(payload, null, 2) + '\n');
}
const total = [...files.values()].reduce((n, f) => n + f.cases.length, 0);
console.log(`wrote ${files.size} case files, ${total} cases`);
