#!/usr/bin/env node
// Upstream parity runner for commonmark@0.31.2 (the CommonMark reference
// implementation in JavaScript).
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- a throw is reported as {"ok": false, "error": "..."}.
//
// Determinism: commonmark.js has no clocks, randomness or locale dependence,
// and every renderer/parser used here is constructed with explicit options, so
// nothing depends on the environment. `time` is never read.

import * as readline from 'node:readline';
import {HtmlRenderer, Node, Parser} from 'commonmark';

// One parser/renderer pair per option set, reused across cases. commonmark.js
// parsers are stateless between parse() calls.
const plainParser = new Parser();
const smartParser = new Parser({smart: true});

const plainRenderer = new HtmlRenderer();
const safeRenderer = new HtmlRenderer({safe: true});
const brRenderer = new HtmlRenderer({softbreak: '<br />'});

function requireString(value, what) {
	if (typeof value !== 'string') {
		throw new TypeError(`${what} must be a string, got ${typeof value}`);
	}

	return value;
}

// ---------------------------------------------------------------- AST shape
//
// The comparable subset of the AST: the fields commonmark.js and the Go port
// both define with the same meaning. Deliberately excluded (recorded in
// COVERAGE.md instead): sourcepos (Go has none), the list bullet/delimiter
// character (Go-only), and the per-item copy of the parent list's data
// (commonmark.js stores it on items too, with per-item `listStart`).

// Fields are masked by node type -- identically here and in the Go runner --
// so that a field one side happens to reuse for something else (the Go port
// stores the list bullet character in Literal and the ordered-list start in
// Level, where commonmark.js has dedicated listDelimiter/listStart) cannot show
// up as a spurious difference.
const LITERAL_TYPES = new Set(['text', 'code', 'code_block', 'html_block', 'html_inline']);
const LEVEL_TYPES = new Set(['heading']);
const INFO_TYPES = new Set(['code_block']);
const LIST_TYPES = new Set(['list']);

function astOf(node) {
	const out = {
		type: node.type,
		literal: LITERAL_TYPES.has(node.type) && node.literal !== null && node.literal !== undefined ? node.literal : '',
		level: LEVEL_TYPES.has(node.type) && node.level !== null && node.level !== undefined ? node.level : 0,
		destination: node.destination === null || node.destination === undefined ? '' : node.destination,
		title: node.title === null || node.title === undefined ? '' : node.title,
		info: INFO_TYPES.has(node.type) && node.info !== null && node.info !== undefined ? node.info : '',
	};

	if (LIST_TYPES.has(node.type)) {
		out.listType = node.listType ?? '';
		out.listTight = node.listTight === true;
		out.listStart = node.listStart === null || node.listStart === undefined ? 0 : node.listStart;
	}

	const children = [];
	for (let child = node.firstChild; child !== null; child = child.next) {
		children.push(astOf(child));
	}

	out.children = children;
	return out;
}

function walkTypes(node) {
	const seen = new Set();
	const walker = node.walker();
	let event;
	while ((event = walker.next()) !== null) {
		if (event.entering) {
			seen.add(event.node.type);
		}
	}

	return [...seen].sort();
}

function handle(request) {
	const {fn, args = []} = request;

	switch (fn) {
		// The spec corpus, and the security group's "what does it do by
		// default" question: stock parser, stock renderer.
		case 'render':
		case 'render_to': {
			return plainRenderer.render(plainParser.parse(requireString(args[0], 'src')));
		}

		// commonmark.js HtmlRenderer({safe: true}): unsafe link destinations are
		// dropped and raw HTML is replaced by a comment.
		case 'render_safe': {
			return safeRenderer.render(plainParser.parse(requireString(args[0], 'src')));
		}

		// Parser({smart: true}) -- smart punctuation, the closest upstream has
		// to the port's Typographer option.
		case 'render_smart': {
			return plainRenderer.render(smartParser.parse(requireString(args[0], 'src')));
		}

		// HtmlRenderer({softbreak: '<br />'}) -- upstream's equivalent of the
		// port's LineBreaks option.
		case 'render_softbreak': {
			return brRenderer.render(plainParser.parse(requireString(args[0], 'src')));
		}

		case 'ast': {
			return astOf(plainParser.parse(requireString(args[0], 'src')));
		}

		case 'node_types': {
			return walkTypes(plainParser.parse(requireString(args[0], 'src')));
		}

		// Node navigation: firstChild / lastChild / parent, plus the walker.
		case 'node_nav': {
			const doc = plainParser.parse(requireString(args[0], 'src'));
			let last = null;
			for (let child = doc.firstChild; child !== null; child = child.next) {
				last = child;
			}

			return {
				docType: doc.type,
				firstChild: doc.firstChild === null ? null : doc.firstChild.type,
				lastChild: last === null ? null : last.type,
				firstChildParent: doc.firstChild === null ? null : doc.firstChild.parent.type,
				rootParent: doc.parent === null ? null : doc.parent.type,
				nodeCount: (() => {
					let n = 0;
					const walker = doc.walker();
					let event;
					while ((event = walker.next()) !== null) {
						if (event.entering) {
							n++;
						}
					}

					return n;
				})(),
			};
		}

		// A tree built by hand with appendChild, reported as its AST shape. The
		// Go port has no public renderer for a Node, so the tree itself -- not
		// its HTML -- is what gets compared.
		case 'append_child': {
			const doc = new Node('document');
			const para = new Node('paragraph');
			const text = new Node('text');
			text.literal = 'hi';
			para.appendChild(text);
			doc.appendChild(para);
			const emph = new Node('emph');
			const inner = new Node('text');
			inner.literal = 'there';
			emph.appendChild(inner);
			para.appendChild(emph);
			return astOf(doc);
		}

		// Options the port has and upstream does not, and vice versa. Reported
		// as failures rather than faked, so COVERAGE.md can call them what they
		// are: extra API surface with no oracle.
		case 'render_nohtml': {
			throw new Error('commonmark.js has no HTML:false option (only safe:true, which also rewrites links)');
		}

		case 'render_linktarget': {
			throw new Error('commonmark.js has no link-target option');
		}

		case 'default_options': {
			throw new Error('commonmark.js exposes no default-options accessor');
		}

		case 'block_types': {
			throw new Error('commonmark.js has no block/inline type predicate (only Node#isContainer)');
		}

		case 'render_to_bad_writer': {
			throw new Error('commonmark.js renders to a string; there is no writer to fail');
		}

		default:
			throw new Error(`unknown fn: ${fn}`);
	}
}

const rl = readline.createInterface({input: process.stdin, crlfDelay: Infinity});

rl.on('line', line => {
	const text = line.trim();
	if (text === '') {
		return;
	}

	let id = null;
	let response;
	try {
		const request = JSON.parse(text);
		id = request.id ?? null;
		response = {id, ok: true, value: handle(request)};
	} catch (error) {
		response = {id, ok: false, error: String(error && error.message ? error.message : error)};
	}

	process.stdout.write(JSON.stringify(response) + '\n');
});
