#!/usr/bin/env node
// Upstream parity runner for chalk@5.3.0.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- a throw is reported as {"ok": false, "error": "..."}.
//
// Colour support is *never* auto-detected here: every case names the level it
// wants and we build a `new Chalk({level})` for it, so the output does not
// depend on whether stdout happens to be a TTY.

import * as readline from 'node:readline';
import {
	Chalk,
	backgroundColorNames,
	colorNames,
	foregroundColorNames,
	modifierNames,
} from 'chalk';

const NAME_LISTS = {
	colorNames,
	modifierNames,
	foregroundColorNames,
	backgroundColorNames,
};

const instances = new Map();

function chalkAt(level) {
	if (!instances.has(level)) {
		// Throws for a level outside 0..3 -- that is a case outcome, not a crash.
		instances.set(level, new Chalk({level}));
	}

	return instances.get(level);
}

// A "chain" is an array of steps. A step is either a bare style name
// ("bold") or [name, ...params] for the parameterised models
// (["hex", "#ff8800"], ["rgb", 255, 136, 0], ["ansi256", 42]).
// The pseudo-step ["level", n] rebases the chain onto a fresh Chalk of that
// level; it is only ever used as the first step.
function buildStyler(base, chain) {
	let current = base;

	for (const raw of chain) {
		const step = Array.isArray(raw) ? raw : [raw];
		const [name, ...params] = step;

		if (name === 'level') {
			current = chalkAt(params[0]);
			continue;
		}

		const member = current[name];
		if (member === undefined) {
			throw new Error(`unknown style: ${name}`);
		}

		current = params.length > 0 ? current[name](...params) : member;
	}

	return current;
}

// A "node" is either a string (literal text) or
// {chain: [...], children: [node...]} / {chain: [...], args: [...]}.
// `children` are rendered and concatenated with '' before styling, which is
// how nesting is expressed; `args` are passed as separate arguments so the
// multi-argument joining behaviour can be compared.
function renderNode(base, node) {
	if (typeof node === 'string') {
		return node;
	}

	if (node === null || typeof node !== 'object') {
		return String(node);
	}

	const styler = buildStyler(base, node.chain ?? []);

	if (Object.hasOwn(node, 'args')) {
		return styler(...node.args);
	}

	const inner = (node.children ?? []).map(child => renderNode(base, child)).join('');
	return styler(inner);
}

function handle(request) {
	const {fn, args = []} = request;

	switch (fn) {
		case 'render': {
			const [level, node] = args;
			return renderNode(chalkAt(level), node);
		}

		case 'level': {
			// chalk.level of a Chalk built at this level.
			return chalkAt(args[0]).level;
		}

		case 'enabled': {
			// chalk has no boolean "enabled"; level 0 means no output.
			return chalkAt(args[0]).level > 0;
		}

		case 'names': {
			const list = NAME_LISTS[args[0]];
			if (list === undefined) {
				throw new Error(`unknown name list: ${args[0]}`);
			}

			return [...list].sort();
		}

		case 'template': {
			// chalk 5 has no template parser of its own (that moved to
			// chalk-template), but a styler is still a plain function and can be
			// used as a tag, so this records what that actually produces.
			const [level, chain, strings, values] = args;
			const styler = buildStyler(chalkAt(level), chain);
			const parts = [...strings];
			parts.raw = [...strings];
			return styler(parts, ...values);
		}

		case 'strip': {
			throw new Error('chalk has no strip export');
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
