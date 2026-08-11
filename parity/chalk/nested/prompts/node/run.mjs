#!/usr/bin/env node
// Upstream parity runner for prompts@2.4.2 (terkelg/prompts, the npm package
// that github.com/malcolmston/chalk/prompts ports).
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- a throw, or a cancelled prompt, is reported as
// {"ok": false, "error": "..."}.
//
// prompts is interactive, so the deterministic surface is driven with a
// *scripted keyboard*: each case carries a list of key tokens, which are turned
// into the same raw bytes on both sides and fed to the prompt through an
// injectable stream (opts.stdin here, cfg.In in the Go port). Nothing is written
// to a TTY: stdout for the prompt is a sink, and no clock or random value ever
// reaches a returned value.
//
// prompts' own inject() API is deliberately *not* used: it bypasses the prompt
// element entirely and returns the injected value unvalidated and uncoerced, so
// it would compare the harness against itself rather than against the prompt.

import * as readline from 'node:readline';
import {PassThrough, Writable} from 'node:stream';
import prompts from 'prompts';

// ------------------------------------------------------------- key scripting

// KEYS maps a token to the bytes a terminal would send. Anything that is not a
// token is written literally, so "abc" types three characters.
const KEYS = {
	'<enter>': '\r',
	'<up>': '\x1b[A',
	'<down>': '\x1b[B',
	'<right>': '\x1b[C',
	'<left>': '\x1b[D',
	'<space>': ' ',
	'<tab>': '\t',
	'<backspace>': '\x7f',
	'<ctrl-c>': '\x03',
};

function bytesFor(token) {
	if (Object.hasOwn(KEYS, token)) return KEYS[token];
	if (token.startsWith('<')) throw new Error(`unknown key token: ${token}`);
	return token;
}

const yieldToLoop = () => new Promise((resolve) => setImmediate(resolve));

// ------------------------------------------------------- named hook registry

// The case files are language-agnostic JSON and cannot carry functions, so
// validators and formatters are referenced by name and defined identically in
// both runners.
const VALIDATORS = {
	nonEmpty: (v) => (String(v).length > 0 ? true : 'a value is required'),
	min3: (v) => (String(v).length >= 3 ? true : 'at least three characters'),
	reject: () => 'never acceptable',
	even: (v) => (Number(v) % 2 === 0 ? true : 'must be even'),
};

const FORMATS = {
	upper: (v) => String(v).toUpperCase(),
	trim: (v) => String(v).trim(),
};

// ---------------------------------------------------------------- the prompt

// question builds the upstream question object from the case's config.
function question(type, cfg) {
	const q = {type, name: 'answer', message: cfg.message ?? 'q'};
	if (cfg.initial !== undefined) q.initial = cfg.initial;
	if (cfg.validate !== undefined) {
		const v = VALIDATORS[cfg.validate];
		if (!v) throw new Error(`unknown validator: ${cfg.validate}`);
		q.validate = v;
	}
	if (cfg.format !== undefined) {
		const f = FORMATS[cfg.format];
		if (!f) throw new Error(`unknown format: ${cfg.format}`);
		q.format = f;
	}
	if (cfg.float !== undefined) q.float = cfg.float;
	if (cfg.min !== undefined) q.min = cfg.min;
	if (cfg.max !== undefined) q.max = cfg.max;
	if (cfg.choices !== undefined) {
		q.choices = cfg.choices.map((c) => ({
			title: c.title,
			value: c.value === undefined ? c.title : c.value,
			disabled: !!c.disabled,
			selected: !!c.selected,
		}));
	}
	return q;
}

class Cancelled extends Error {}

async function run(type, cfg, keys) {
	const stdin = new PassThrough();
	// A sink for the prompt's own drawing. columns is fixed so the redraw
	// arithmetic cannot depend on the terminal this runs in.
	const stdout = new Writable({write(_chunk, _enc, cb) { cb(); }});
	stdout.columns = 80;

	const q = question(type, cfg);
	q.stdin = stdin;
	q.stdout = stdout;

	let cancelled = false;
	const pending = prompts([q], {onCancel: () => { cancelled = true; return false; }});

	// Feed one key at a time, yielding to the event loop between them so an
	// async validator has run before the next key arrives. Yielding (rather than
	// sleeping) keeps the script deterministic.
	for (const token of keys) {
		await yieldToLoop();
		stdin.write(bytesFor(token));
	}

	const answers = await pending;
	if (cancelled) throw new Cancelled('canceled');
	return answers.answer;
}

const TYPES = ['text', 'password', 'number', 'confirm', 'select', 'multiselect'];

// ------------------------------------------------------------------- the loop

const rl = readline.createInterface({input: process.stdin, terminal: false});
const queue = [];
let draining = false;

async function drain() {
	if (draining) return;
	draining = true;
	while (queue.length > 0) {
		const line = queue.shift();
		let req;
		try {
			req = JSON.parse(line);
		} catch (err) {
			process.stdout.write(JSON.stringify({id: null, ok: false, error: 'bad request: ' + err.message}) + '\n');
			continue;
		}
		let out;
		try {
			if (!TYPES.includes(req.fn)) throw new Error(`unknown fn: ${req.fn}`);
			const [cfg, keys] = req.args || [{}, []];
			out = {id: req.id, ok: true, value: await run(req.fn, cfg || {}, keys || [])};
		} catch (err) {
			out = {id: req.id, ok: false, error: err && err.message ? err.message : String(err)};
		}
		process.stdout.write(JSON.stringify(out) + '\n');
	}
	draining = false;
}

rl.on('line', (line) => {
	if (line.trim() === '') return;
	queue.push(line);
	void drain();
});
