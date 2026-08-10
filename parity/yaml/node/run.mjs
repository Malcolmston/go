#!/usr/bin/env node
// Upstream parity runner: the reference JavaScript implementation, yaml@2.6.1.
//
// Speaks JSON Lines on stdio -- one request object per line in, exactly one
// response object per line out, in the same order. A throw inside a case is
// reported as {"ok": false, "error": "..."}; the process never exits early.
//
// A response may carry a "detail" object. It is never compared -- the harness
// only logs it when a case mismatches -- and exists so an emitter case can show
// the text that failed to re-parse.
//
// Determinism: mapping keys are sorted before a value is emitted, and no case
// depends on wall-clock time, locale or iteration order.

import * as readline from 'node:readline';
import {parse, parseAllDocuments, parseDocument, stringify, Document, isMap} from 'yaml';

// ------------------------------------------------------------------ input

// A case's document argument is either a JSON string (UTF-8 text) or
// {"hex": "..."} for a byte sequence that is not valid UTF-8 and so cannot be
// written as a JSON string.
function inputBytes(arg) {
	if (typeof arg === 'string') return Buffer.from(arg, 'utf8');
	if (arg && typeof arg.hex === 'string') return Buffer.from(arg.hex, 'hex');
	throw new Error('input must be a string or {"hex": "..."}');
}

// yaml@2 decodes from a string, so ill-formed UTF-8 reaches it as replacement
// characters unless we hand it the raw bytes reinterpreted losslessly. latin1
// is the lossless byte->string mapping; utf8 is used whenever the input really
// is valid UTF-8, which is every case but the ill-formed ones.
function inputText(arg) {
	const bytes = inputBytes(arg);
	const asUTF8 = bytes.toString('utf8');
	if (Buffer.compare(Buffer.from(asUTF8, 'utf8'), bytes) === 0) {
		return {text: asUTF8, wellFormed: true};
	}
	return {text: bytes.toString('latin1'), wellFormed: false};
}

// ----------------------------------------------------------- normalisation

const hex = buf => Buffer.from(buf).toString('hex');

// normalise turns a decoded YAML value into a shape that can be compared
// against the Go runner's answer through JSON:
//
//   NaN/Infinity      -> ".nan" / ".inf" / "-.inf" (JSON has no such literals)
//   bytes             -> {"!!binary": "<lowercase hex>"}
//   Date              -> {"!!timestamp": "<ISO 8601, UTC, milliseconds>"}
//   mapping           -> object with keys sorted; a non-string key becomes the
//                        JSON text of its normalised form
function normalise(v) {
	if (v === null || v === undefined) return null;

	switch (typeof v) {
		case 'boolean':
		case 'string':
			return v;
		case 'bigint':
			return Number(v);
		case 'number':
			if (Number.isNaN(v)) return '.nan';
			if (v === Infinity) return '.inf';
			if (v === -Infinity) return '-.inf';
			return v;
		case 'symbol':
		case 'function':
			return String(v);
	}

	if (v instanceof Uint8Array) return {'!!binary': hex(v)};
	// Tagged so that a timestamp is not silently equal to the plain string it
	// was written as -- an emitter that drops !!timestamp has to show up.
	if (v instanceof Date) return {'!!timestamp': v.toISOString()};
	if (Array.isArray(v)) return v.map(normalise);

	const entries = v instanceof Map ? [...v.entries()] : Object.entries(v);
	const keyed = entries.map(([k, val]) => [
		typeof k === 'string' ? k : JSON.stringify(normalise(k)),
		normalise(val),
	]);
	keyed.sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0));

	const out = {};
	for (const [k, val] of keyed) out[k] = val;

	return out;
}

const same = (a, b) => JSON.stringify(a) === JSON.stringify(b);

// ------------------------------------------------------------- parse helpers

// yaml@2 collects problems on the document rather than throwing, so rejection
// has to be read off doc.errors. Warnings (recoverable, e.g. an unknown
// directive) are not rejections.
function docErrors(docs) {
	const msgs = [];
	for (const doc of docs) for (const e of doc.errors) msgs.push(e.message);
	return msgs;
}

function parseDocs(arg) {
	const {text} = inputText(arg);
	const docs = parseAllDocuments(text, {prettyErrors: false});
	const errs = docErrors(docs);
	if (errs.length > 0) throw new Error(errs.join('; '));
	return docs;
}

// docValue decodes one document. mapAsMap keeps non-string keys intact so
// normalise can stringify them the same way on both sides.
const docValue = doc => normalise(doc.toJS({mapAsMap: true}));

// ------------------------------------------------------------------- the ops

const ops = {
	// (a) parse acceptance. Rejection is reported as ok:false.
	parse(arg) {
		const docs = parseDocs(arg);
		return {value: {docs: docs.length}};
	},

	// (b) parse-to-JSON equivalence: one normalised value per document.
	parseJSON(arg) {
		return {value: parseDocs(arg).map(docValue)};
	},

	// Emitter fidelity: emit the parsed tree `times` times, re-parsing between
	// each round, and report whether the text still parses, whether the value
	// survived, and whether the document grew.
	// Only the first document of a stream is re-emitted, on both sides, so that
	// the comparison is between two single documents and not between two ways
	// of joining a stream back together.
	reemit(arg, times) {
		const firstDoc = src => {
			const docs = parseAllDocuments(src, {prettyErrors: false});
			const errs = docErrors(docs);
			if (errs.length > 0) throw new Error(errs.join('; '));
			if (docs.length === 0) throw new Error('no documents to re-emit');
			return docs[0];
		};

		const {text} = inputText(arg);
		const doc = firstDoc(text);

		let base = null;
		let decodeOK = true;
		try {
			base = docValue(doc);
		} catch (err) {
			decodeOK = false;
		}

		let current = doc;
		let first = null;
		let last = null;
		for (let i = 0; i < times; i++) {
			last = String(current);
			if (first === null) first = last;
			try {
				current = firstDoc(last);
			} catch (err) {
				return {
					value: {emitOK: true, reparseOK: false, decodeOK, stable: null, grew: null, value: null},
					detail: {round: i + 1, emitted: last, error: String(err.message)},
				};
			}
		}

		let final = null;
		try {
			final = docValue(current);
		} catch (err) {
			decodeOK = false;
		}

		return {
			value: {
				emitOK: true,
				reparseOK: true,
				decodeOK,
				stable: decodeOK ? same(base, final) : null,
				grew: last.length > first.length,
				value: decodeOK ? final : null,
			},
			detail: {first, last},
		};
	},

	// Emitter round trip from a value: marshal it, then read it back.
	marshal(value) {
		const doc = new Document(value);
		const text = String(doc);
		const back = parseDocument(text, {prettyErrors: false});
		if (back.errors.length > 0) {
			return {
				value: {emitOK: true, reparseOK: false, stable: null, value: null},
				detail: {emitted: text, error: back.errors[0].message},
			};
		}
		const got = docValue(back);
		return {
			value: {emitOK: true, reparseOK: true, stable: same(got, normalise(value)), value: got},
			detail: {emitted: text},
		};
	},

	// Comment fidelity: attach a comment to the value node of a single-entry
	// mapping, emit, and read the result back. A comment that is written
	// without its "#" is absorbed into the value, which this exposes.
	comment(value, kind, text) {
		const doc = new Document(value);
		if (!isMap(doc.contents) || doc.contents.items.length !== 1) {
			throw new Error('comment cases need a single-entry mapping');
		}
		const target = doc.contents.items[0].value;
		switch (kind) {
			case 'line':
				target.comment = text;
				break;
			case 'head':
				target.commentBefore = text;
				break;
			case 'foot':
				// yaml@2 has no separate foot comment; the nearest equivalent is a
				// comment after the document's contents.
				doc.comment = text;
				break;
			default:
				throw new Error(`unknown comment kind: ${kind}`);
		}

		const out = String(doc);
		const back = parseDocument(out, {prettyErrors: false});
		if (back.errors.length > 0) {
			return {
				value: {emitOK: true, reparseOK: false, value: null},
				detail: {emitted: out, error: back.errors[0].message},
			};
		}
		return {
			value: {emitOK: true, reparseOK: true, value: docValue(back)},
			detail: {emitted: out},
		};
	},

	// Single-document decode, the counterpart of the port's Unmarshal.
	unmarshal(arg) {
		const {text} = inputText(arg);
		return {value: normalise(parse(text, {prettyErrors: false, mapAsMap: true}))};
	},

	// Stream decode, the counterpart of the port's Decoder.
	decodeStream(arg) {
		return {value: parseDocs(arg).map(docValue)};
	},

	// Stream encode at a chosen indent, the counterpart of Encoder/SetIndent:
	// write every value as one document, then read the stream back.
	encodeStream(values, indent) {
		const text = values.map(v => stringify(v, {indent})).join('---\n');
		const docs = parseAllDocuments(text, {prettyErrors: false});
		const errs = docErrors(docs);
		if (errs.length > 0) {
			return {
				value: {emitOK: true, reparseOK: false, docs: null, stable: null, value: null},
				detail: {emitted: text, error: errs[0]},
			};
		}
		const got = docs.map(docValue);
		return {
			value: {
				emitOK: true,
				reparseOK: true,
				docs: docs.length,
				stable: same(got, values.map(normalise)),
				value: got,
			},
			detail: {emitted: text},
		};
	},

	// Reports whether the input is well-formed UTF-8 at all, so the ill-formed
	// cases can state what they are feeding in.
	wellFormedUTF8(arg) {
		return {value: {wellFormed: inputText(arg).wellFormed}};
	},
};

// ---------------------------------------------------------------- the loop

const rl = readline.createInterface({input: process.stdin, crlfDelay: Infinity});

rl.on('line', line => {
	if (line.trim() === '') return;

	let req;
	try {
		req = JSON.parse(line);
	} catch (err) {
		process.stdout.write(JSON.stringify({id: '?', ok: false, error: `bad request: ${err.message}`}) + '\n');
		return;
	}

	let res;
	try {
		const op = ops[req.fn];
		if (!op) throw new Error(`unknown fn: ${req.fn}`);
		const {value, detail} = op(...(req.args ?? []));
		res = {id: req.id, ok: true, value};
		if (detail !== undefined) res.detail = detail;
	} catch (err) {
		res = {id: req.id, ok: false, error: String(err && err.message ? err.message : err)};
	}

	process.stdout.write(JSON.stringify(res) + '\n');
});
