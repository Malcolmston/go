#!/usr/bin/env node
// Generates parity/yaml/cases/suite-*.json from a checkout of
// github.com/yaml/yaml-test-suite (the `main` branch, where every test lives as
// a YAML file in src/).
//
//   node gen-cases.mjs /path/to/yaml-test-suite
//
// The suite is the corpus, not the oracle: this script only extracts each test's
// input document, whether the suite says it must fail, and (when present) the
// suite's expected JSON. Nothing here decides what the two implementations
// should answer.
//
// src/<ID>.yaml holds a *list* of tests. Later entries inherit the keys they
// omit from the previous entry -- the rule implemented by the suite's own
// bin/YAMLTestSuite.pm -- except `fail`, which is per-entry. `yaml` bodies use
// printable stand-ins for characters that would otherwise be invisible; the
// same file's `unescape` defines them.

import * as fs from 'node:fs';
import * as path from 'node:path';
import {execFileSync} from 'node:child_process';
import {parseAllDocuments} from 'yaml';

const suiteDir = process.argv[2];
if (!suiteDir) {
	console.error('usage: node gen-cases.mjs <path-to-yaml-test-suite-checkout>');
	process.exit(2);
}

const casesDir = path.join(import.meta.dirname, '..', 'cases');
const nodePin = JSON.parse(
	fs.readFileSync(path.join(import.meta.dirname, 'node_modules', 'yaml', 'package.json'), 'utf8'),
).version;
const suiteRev = execFileSync('git', ['-C', suiteDir, 'rev-parse', 'HEAD'], {encoding: 'utf8'}).trim();
const UPSTREAM = `yaml@${nodePin}+yaml-test-suite@${suiteRev}`;

// bin/YAMLTestSuite.pm `unescape`.
function unescape(text) {
	return text
		.replaceAll('␣', ' ')      // ␣ trailing space
		.replace(/—*»/g, '\t') // ——» hard tab
		.replaceAll('←', '\r')    // ← carriage return
		.replaceAll('⇔', '﻿') // ⇔ byte order mark
		.replaceAll('↵', '')      // ↵ (marks a trailing newline that is already there)
		.replace(/∎\n$/, '');     // ∎ no final newline
}

const INHERIT = ['name', 'tags', 'yaml', 'tree', 'json', 'dump', 'emit', 'toke'];

function readSuite() {
	const out = [];
	const files = fs.readdirSync(path.join(suiteDir, 'src')).filter(f => f.endsWith('.yaml')).sort();

	for (const file of files) {
		const id = path.basename(file, '.yaml');
		const docs = parseAllDocuments(fs.readFileSync(path.join(suiteDir, 'src', file), 'utf8'));
		if (docs.length !== 1) throw new Error(`${file}: expected one document`);
		const entries = docs[0].toJS();
		if (!Array.isArray(entries)) throw new Error(`${file}: expected a list of tests`);
		// "Valid by the productions but not usefully valid"; the suite tells
		// processors not to support these, so they are not parity evidence.
		if (entries[0] && entries[0].skip) continue;

		let cache = {};
		const multi = entries.length > 1;
		entries.forEach((entry, i) => {
			const test = {...entry};
			for (const key of INHERIT) {
				if (test[key] === undefined && cache[key] !== undefined) test[key] = cache[key];
			}
			cache = {...test};
			out.push({
				id: multi ? `${id}-${String(i).padStart(2, '0')}` : id,
				name: test.name ?? '',
				tags: (test.tags ?? '').split(/\s+/).filter(Boolean),
				yaml: unescape(test.yaml),
				fail: entry.fail === true,
				json: test.json === undefined ? null : test.json,
			});
		});
	}
	return out;
}

function write(group, cases) {
	cases = cases.map(deviate);
	const file = path.join(casesDir, `${group}.json`);
	fs.writeFileSync(file, JSON.stringify({group, upstream: UPSTREAM, cases}, null, 2) + '\n');
	console.error(`${file}: ${cases.length} cases`);
}

// Symbol names recorded on every case for COVERAGE.md.
const REEMIT_UP = 'String(yaml.parseDocument(...))';
const REEMIT_GO = 'yaml.Parse + yaml.Marshal(*yaml.Node)';
const PJ_UP = 'yaml.parseAllDocuments().map(d => d.toJS())';
const PJ_GO = 'yaml.Parse + (*Node).Decode';
const MARSHAL_UP = 'String(new yaml.Document(v))';
const MARSHAL_GO = 'yaml.Marshal(v)';

const c = (id, fn, args, upstreamFn, goFn, note, extra = {}) =>
	({id, fn, args, upstreamFn, goFn, note, ...extra});

// Differences the port documents in its own API-DEVIATIONS.md, keyed by case id.
// The harness counts these separately from a bug.
const DEVIATIONS = {
	'json-565N': 'the port base64-decodes !!binary, so it does not reproduce the suite\'s undecoded JSON (API-DEVIATIONS.md, "!!binary is decoded")',
	'reemit-565N': 'the port base64-decodes !!binary (API-DEVIATIONS.md, "!!binary is decoded")',
};
const deviate = spec => (DEVIATIONS[spec.id] ? {...spec, deviation: DEVIATIONS[spec.id]} : spec);

const tests = readSuite();
const accept = tests.filter(t => !t.fail);
const fail = tests.filter(t => t.fail);

// (a) parse acceptance -- does each side accept, or reject, this document.
write('suite-accept', accept.map(t => ({
	id: `accept-${t.id}`,
	fn: 'parse',
	args: [t.yaml],
	upstreamFn: 'yaml.parseAllDocuments',
	goFn: 'yaml.Parse',
	expect: 'accept',
	suite: t.id,
	suiteTags: t.tags,
	note: t.name,
})));

write('suite-fail', fail.map(t => ({
	id: `fail-${t.id}`,
	fn: 'parse',
	args: [t.yaml],
	upstreamFn: 'yaml.parseAllDocuments',
	goFn: 'yaml.Parse',
	expect: 'fail',
	suite: t.id,
	suiteTags: t.tags,
	note: t.name,
})));

// (b) parse-to-JSON equivalence, for the tests the suite gives a JSON value for.
write('suite-json', accept.filter(t => t.json !== null).map(t => ({
	id: `json-${t.id}`,
	fn: 'parseJSON',
	args: [t.yaml],
	upstreamFn: 'yaml.parseAllDocuments().map(d => d.toJS())',
	goFn: 'yaml.Parse + (*Node).Decode',
	expect: 'accept',
	suite: t.id,
	suiteTags: t.tags,
	suiteJSON: t.json,
	note: t.name,
})));

// (c) emitter fidelity: re-emit the parsed tree three times and re-parse it.
write('suite-reemit', accept.map(t => ({
	id: `reemit-${t.id}`,
	fn: 'reemit',
	args: [t.yaml, 3],
	upstreamFn: REEMIT_UP,
	goFn: REEMIT_GO,
	expect: 'accept',
	suite: t.id,
	suiteTags: t.tags,
	note: t.name,
})));

// -------------------------------------------------------------------------
// Groups the suite does not cover: the emitter, and the YAML 1.1-vs-1.2
// resolution rules. These inputs are written here rather than derived, but the
// expected answers still come from the oracle -- nothing below states a result.

write('emitter-bugs', [
	c('emit-flow-seq-tag', 'reemit', ['!lst [1, 2]\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 1: a tag on a flow sequence is emitted twice (!lst !lst [1, 2]) and no longer parses'),
	c('emit-flow-map-tag', 'reemit', ['!m {a: 1}\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 1: same doubling for a flow mapping'),
	c('emit-flow-seq-tag-nested', 'reemit', ['a: !lst [1, 2]\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 1: doubled tag on a flow sequence used as a mapping value'),
	c('emit-block-seq-tag-control', 'reemit', ['!lst\n- 1\n- 2\n', 3], REEMIT_UP, REEMIT_GO,
		'control for bug 1: a tag on a BLOCK collection is expected to survive'),
	c('emit-verbatim-global-tag', 'reemit', ['!<tag:example.com,2000:app/foo> bar\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 2: a global/URI tag is emitted as a bare URI, so it re-parses as a plain string -- silent data corruption'),
	c('emit-tag-directive-global', 'reemit', ['%TAG ! tag:example.com,2000:app/\n---\n!foo bar\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 2: %TAG is resolved away and the expanded URI is emitted bare'),
	c('emit-tag-directive-secondary', 'reemit', ['%TAG !e! tag:example.com,2000:app/\n---\n!e!foo bar\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 2: secondary tag handle, same bare-URI emission'),
	c('emit-local-tag-control', 'reemit', ['!custom bar\n', 3], REEMIT_UP, REEMIT_GO,
		'control for bug 2: a local tag (!custom) is expected to survive'),
	c('emit-line-comment', 'comment', [{a: 1}, 'line', 'why'], 'node.comment = text', 'Node.LineComment = text',
		"bug 3: LineComment is written without a leading '#', so 'why' is absorbed into the value"),
	c('emit-line-comment-string', 'comment', [{a: 'x'}, 'line', 'why'], 'node.comment = text', 'Node.LineComment = text',
		'bug 3: same absorption when the value is a string'),
	c('emit-head-comment', 'comment', [{a: 1}, 'head', 'why'], 'node.commentBefore = text', 'Node.HeadComment = text',
		'HeadComment set on a mapping value node'),
	c('emit-foot-comment', 'comment', [{a: 1}, 'foot', 'why'], 'doc.comment = text', 'Node.FootComment = text',
		'FootComment set on the root mapping'),
	c('emit-foot-comment-growth', 'reemit', ['a: 1\n# foot\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 4: a foot comment is duplicated on every round trip, so the document grows without bound (grew:true)'),
	c('emit-foot-comment-growth-seq', 'reemit', ['- 1\n# foot\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 4: same growth under a sequence'),
	c('emit-head-comment-roundtrip', 'reemit', ['# head\na: 1\n', 3], REEMIT_UP, REEMIT_GO,
		'control for bug 4: a head comment is expected to be stable across round trips'),
	c('emit-timestamp-tag', 'reemit', ['t: !!timestamp 2001-12-15T02:59:43.1Z\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 5: !!timestamp is dropped on emit, so the value comes back as a plain string'),
	c('emit-timestamp-tag-spaced', 'reemit', ['t: !!timestamp 2001-12-14 21:59:43.10 -5\n', 3], REEMIT_UP, REEMIT_GO,
		'bug 5: !!timestamp written in the YAML spaced form'),
	c('parse-timestamp-tag', 'parseJSON', ['t: !!timestamp 2001-12-15T02:59:43.1Z\n'], PJ_UP, PJ_GO,
		'!!timestamp resolution, before any emitting is involved'),
	c('utf8-illformed-accept', 'parse', [{hex: '613a20fffe0a'}], 'yaml.parseAllDocuments', 'yaml.Parse',
		'bug 6: the port accepts ill-formed UTF-8 (0xff 0xfe). A JavaScript string cannot hold ill-formed bytes at all, so this case can only record that the port does not reject them'),
	c('utf8-illformed-is-illformed', 'wellFormedUTF8', [{hex: '613a20fffe0a'}], 'Buffer round trip', 'utf8.Valid',
		'asserts the fixture really is ill-formed UTF-8, on both sides'),
	c('utf8-illformed-reemit', 'reemit', [{hex: '613a20fffe0a'}, 2], REEMIT_UP, REEMIT_GO,
		'bug 6: the port re-emits the ill-formed bytes as a single-quoted scalar of U+FFFD',
		{deviation: 'a JavaScript string cannot hold ill-formed UTF-8, so the oracle only ever sees latin1-decoded bytes; nothing but acceptance is comparable here'}),
	c('emit-binary-tag', 'reemit', ['b: !!binary aGk=\n', 3], REEMIT_UP, REEMIT_GO, '!!binary round trip'),
	c('parse-binary-tag', 'parseJSON', ['b: !!binary aGk=\n'], PJ_UP, PJ_GO,
		'!!binary resolution: the port yields a decoded string, the oracle yields bytes',
		{deviation: 'API-DEVIATIONS.md, "!!binary is decoded"'}),
]);

write('schema-1-1-vs-1-2', [
	c('schema-bool-yes-no', 'parseJSON', ['a: yes\nb: no\n'], PJ_UP, PJ_GO, '1.1 booleans; the 1.2 core schema makes these strings'),
	c('schema-bool-on-off', 'parseJSON', ['a: on\nb: off\n'], PJ_UP, PJ_GO, '1.1 booleans'),
	c('schema-bool-y-n', 'parseJSON', ['a: y\nb: n\n'], PJ_UP, PJ_GO, '1.1 single-letter booleans'),
	c('schema-bool-case', 'parseJSON', ['a: Yes\nb: YES\nc: True\nd: TRUE\ne: tRue\n'], PJ_UP, PJ_GO, 'capitalisation of booleans'),
	c('schema-bool-true-false', 'parseJSON', ['a: true\nb: false\nc: True\nd: FALSE\n'], PJ_UP, PJ_GO, '1.2 core booleans'),
	c('schema-null-tilde', 'parseJSON', ['a: ~\nb: null\nc: Null\nd: NULL\ne:\n'], PJ_UP, PJ_GO, 'null spellings, including the empty value'),
	c('schema-int-leading-zero', 'parseJSON', ['a: 017\n'], PJ_UP, PJ_GO, '1.1 octal; the 1.2 core schema has no leading-zero octal'),
	c('schema-int-octal-1-2', 'parseJSON', ['a: 0o17\n'], PJ_UP, PJ_GO, '1.2 octal'),
	c('schema-int-hex', 'parseJSON', ['a: 0xC\nb: 0XC\n'], PJ_UP, PJ_GO, 'hexadecimal'),
	c('schema-int-binary', 'parseJSON', ['a: 0b1010\n'], PJ_UP, PJ_GO, '1.1 binary; not in the 1.2 core schema'),
	c('schema-int-underscores', 'parseJSON', ['a: 1_000\nb: 1_000.5\n'], PJ_UP, PJ_GO, '1.1 digit separators'),
	c('schema-int-plus', 'parseJSON', ['a: +12\nb: -12\n'], PJ_UP, PJ_GO, 'signed integers'),
	c('schema-sexagesimal', 'parseJSON', ['a: 12:30\nb: 190:20:30\n'], PJ_UP, PJ_GO, '1.1 sexagesimal; the 1.2 core schema makes these strings'),
	c('schema-float-forms', 'parseJSON', ['a: 1.5\nb: 1e3\nc: 1.0e+3\nd: .5\ne: 5.\n'], PJ_UP, PJ_GO, 'float spellings'),
	c('schema-float-specials', 'parseJSON', ['a: .inf\nb: -.Inf\nc: .NAN\nd: .nan\n'], PJ_UP, PJ_GO, 'infinities and not-a-number'),
	c('schema-float-specials-11', 'parseJSON', ['a: inf\nb: nan\nc: Infinity\n'], PJ_UP, PJ_GO, 'spellings that are NOT in the 1.2 core schema'),
	c('schema-explicit-tags', 'parseJSON', ['a: !!str 1\nb: !!int 1.0\nc: !!bool yes\nd: !!float 1\ne: !!null x\n'], PJ_UP, PJ_GO, 'explicit tags override resolution'),
	c('schema-empty-string', 'parseJSON', ["a: ''\nb: \"\"\n"], PJ_UP, PJ_GO, 'quoted empty scalars stay strings'),
	c('schema-string-lookalikes', 'parseJSON', ["a: '017'\nb: '1.5'\nc: 'true'\nd: '~'\n"], PJ_UP, PJ_GO, 'quoting defeats resolution'),
	c('schema-big-int', 'parseJSON', ['a: 12345678901234567890\n'], PJ_UP, PJ_GO, 'beyond 2^53',
		{deviation: 'a JavaScript number cannot represent this exactly; the port keeps full precision'}),
	c('schema-dup-keys', 'parse', ['a: 1\na: 2\n'], 'yaml.parseAllDocuments', 'yaml.Parse',
		'duplicate mapping keys'),
	c('schema-complex-key', 'parseJSON', ['? [1, 2]\n: v\n'], PJ_UP, PJ_GO, 'a sequence used as a mapping key'),
	c('schema-merge-key', 'parseJSON', ['base: &b {a: 1}\nderived:\n  <<: *b\n  b: 2\n'], PJ_UP, PJ_GO, 'the merge key',
		{deviation: 'the port resolves merge keys by default; yaml@2 needs {merge: true} and otherwise keeps a literal "<<" key'}),
	c('schema-alias-bomb', 'parseJSON', ['a: &a [x, x, x, x, x, x, x, x, x]\nb: &b [*a, *a, *a, *a, *a, *a, *a, *a, *a]\nc: &c [*b, *b, *b, *b, *b, *b, *b, *b, *b]\nd: [*c, *c, *c, *c, *c, *c, *c, *c, *c]\n'], PJ_UP, PJ_GO,
		'alias expansion: both sides guard against it, at different thresholds',
		{deviation: 'API-DEVIATIONS.md, "Alias-expansion budget": the port allows 1<<21 nodes, where yaml@2 rejects this document outright'}),
]);

write('emit-values', [
	c('marshal-scalars', 'marshal', [{i: 1, f: 1.5, b: true, n: null, s: 'x'}], MARSHAL_UP, MARSHAL_GO, 'plain scalars'),
	c('marshal-string-yes', 'marshal', [{a: 'yes', b: 'no', c: 'on', d: 'off', e: 'y'}], MARSHAL_UP, MARSHAL_GO,
		'strings a YAML 1.1 reader would take for booleans'),
	c('marshal-string-null', 'marshal', [{a: 'null', b: '~', c: '', d: 'NULL'}], MARSHAL_UP, MARSHAL_GO,
		'strings that would be read back as null'),
	c('marshal-string-numeric', 'marshal', [{a: '1', b: '1.5', c: '017', d: '0x1', e: '1e3', f: '-1'}], MARSHAL_UP, MARSHAL_GO,
		'strings that would be read back as numbers'),
	c('marshal-string-indicators', 'marshal', [{a: '#c', b: 'a: b', c: '- x', d: '[x]', e: '{x}', f: '*a', g: '&a', h: '!t', i: '%d', j: '@x', k: '`x', l: '? x', m: 'x #c'}], MARSHAL_UP, MARSHAL_GO,
		'strings starting with, or containing, YAML indicators'),
	c('marshal-string-whitespace', 'marshal', [{a: ' lead', b: 'trail ', c: 'a\tb', d: 'a  b'}], MARSHAL_UP, MARSHAL_GO,
		'significant whitespace'),
	c('marshal-multiline', 'marshal', [{a: 'one\ntwo\n', b: 'one\ntwo', c: '\n', d: 'a\n\nb\n', e: 'a\n b\n'}], MARSHAL_UP, MARSHAL_GO,
		'multi-line strings: a literal block scalar must round-trip exactly'),
	c('marshal-unicode', 'marshal', [{a: 'héllo', b: '日本語', b2: '\u00a0', c: '\u{1f600}'}], MARSHAL_UP, MARSHAL_GO, 'non-ASCII'),
	c('marshal-control-chars', 'marshal', [{a: '\u0001', b: '\u007f', c: '\u001b[0m', d: '\u0085', e: '\u2028'}], MARSHAL_UP, MARSHAL_GO,
		'control characters have to be escaped'),
	c('marshal-nested', 'marshal', [{a: {b: {c: [1, [2, 3], {d: null}]}}}], MARSHAL_UP, MARSHAL_GO, 'nesting'),
	c('marshal-empty-collections', 'marshal', [{a: [], b: {}}], MARSHAL_UP, MARSHAL_GO, 'empty sequence and mapping'),
	c('marshal-seq-root', 'marshal', [[1, 'two', null, [3]]], MARSHAL_UP, MARSHAL_GO, 'a sequence at the document root'),
	c('marshal-scalar-root', 'marshal', ['just a string'], MARSHAL_UP, MARSHAL_GO, 'a scalar at the document root'),
	c('marshal-null-root', 'marshal', [null], MARSHAL_UP, MARSHAL_GO, 'null at the document root'),
	c('marshal-long', 'marshal', [{['k'.repeat(200)]: 'v', k: 'v '.repeat(100)}], MARSHAL_UP, MARSHAL_GO,
		'long keys and values -- line folding must not change the value'),
	c('marshal-numbers-edge', 'marshal', [{zero: 0, big: 1e300, small: 1e-300, int53: 9007199254740991, negf: -1.5}], MARSHAL_UP, MARSHAL_GO,
		'numeric edges'),
	c('marshal-deep', 'marshal', [[[[[[[[[['deep']]]]]]]]]], MARSHAL_UP, MARSHAL_GO, 'deep nesting'),
	c('marshal-key-lookalikes', 'marshal', [{yes: 1, '017': 2, '~': 3, '': 4, 'a b': 5}], MARSHAL_UP, MARSHAL_GO, 'keys that need quoting'),
]);

// The stream-level API: Unmarshal/Decoder on the way in, Encoder (with
// SetIndent) on the way out.
const UM_UP = 'yaml.parse', UM_GO = 'yaml.Unmarshal';
const DS_UP = 'yaml.parseAllDocuments', DS_GO = 'yaml.NewDecoder + Decoder.Decode';
const ES_UP = 'yaml.stringify(v, {indent})', ES_GO = 'yaml.NewEncoder + SetIndent + Encode + Close';

write('stream', [
	c('unmarshal-map', 'unmarshal', ['a: 1\nb: [x, y]\n'], UM_UP, UM_GO, 'single-document decode'),
	c('unmarshal-scalar', 'unmarshal', ['just text\n'], UM_UP, UM_GO, 'a scalar document'),
	c('unmarshal-empty', 'unmarshal', [''], UM_UP, UM_GO, 'an empty stream'),
	c('unmarshal-nested', 'unmarshal', ['a:\n  b:\n    - 1\n    - c: d\n'], UM_UP, UM_GO, 'nesting'),
	c('unmarshal-anchors', 'unmarshal', ['a: &x [1, 2]\nb: *x\n'], UM_UP, UM_GO, 'anchor and alias'),
	c('unmarshal-invalid', 'unmarshal', ['a: [1\n'], UM_UP, UM_GO, 'a malformed document must fail on both sides'),
	c('decode-stream-three', 'decodeStream', ['--- a\n--- b\n--- c\n'], DS_UP, DS_GO, 'three documents'),
	c('decode-stream-one', 'decodeStream', ['a: 1\n'], DS_UP, DS_GO, 'one document, no markers'),
	c('decode-stream-empty', 'decodeStream', [''], DS_UP, DS_GO, 'an empty stream'),
	c('decode-stream-mixed', 'decodeStream', ['---\n- 1\n---\nk: v\n---\nplain\n'], DS_UP, DS_GO, 'documents of different shapes'),
	c('decode-stream-invalid', 'decodeStream', ['--- a\n--- [b\n'], DS_UP, DS_GO, 'a stream whose second document is malformed'),
	c('encode-stream-one-2', 'encodeStream', [[{a: 1, b: [1, 2]}], 2], ES_UP, ES_GO, 'one document at indent 2'),
	c('encode-stream-one-4', 'encodeStream', [[{a: 1, b: [1, 2]}], 4], ES_UP, ES_GO, 'one document at indent 4'),
	c('encode-stream-three', 'encodeStream', [[{a: 1}, [1, 2], 'plain'], 2], ES_UP, ES_GO, 'three documents'),
	c('encode-stream-nulls', 'encodeStream', [[null, {a: null}], 2], ES_UP, ES_GO, 'nulls at document and value level'),
	c('encode-stream-nested', 'encodeStream', [[{a: {b: {c: [1, {d: 2}]}}}], 2], ES_UP, ES_GO, 'nesting at indent 2'),
	c('encode-stream-quoting', 'encodeStream', [[{a: 'yes', b: '1', c: '', d: 'x\ny\n'}], 2], ES_UP, ES_GO, 'values that must be quoted to survive'),
]);
