#!/usr/bin/env node
// Upstream parity runner for figlet@1.11.4 (the npm package that
// github.com/malcolmston/chalk/figlet ports).
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- a throw is reported as {"ok": false, "error": "..."}.
//
// Determinism: no clock, no randomness, no TTY. Every font is read from the
// harness' own fonts/ directory (FIGLET_FONTS), so both runners are fed
// byte-identical FIGfont files.

import * as readline from 'node:readline';
import * as fs from 'node:fs';
import * as path from 'node:path';
import figlet from 'figlet';

const fontDir = process.env.FIGLET_FONTS;
if (!fontDir) {
	process.stderr.write('FIGLET_FONTS is not set\n');
	process.exit(2);
}
figlet.defaults({fontPath: fontDir});

// ---------------------------------------------------------------- operations

// buildOptions turns the language-agnostic option object used in the case files
// into figlet's own options object. Absent keys stay absent so figlet applies
// its own defaults.
function buildOptions(spec) {
	const o = {};
	if (spec === undefined || spec === null) return o;
	for (const key of [
		'horizontalLayout',
		'verticalLayout',
		'width',
		'whitespaceBreak',
		'showHardBlanks',
		'printDirection',
	]) {
		if (spec[key] !== undefined) o[key] = spec[key];
	}

	return o;
}

// metaOf projects figlet's font options onto the fields both ports expose.
function metaOf(opts) {
	return {
		hardBlank: opts.hardBlank,
		height: opts.height,
		baseline: opts.baseline,
		maxLength: opts.maxLength,
		oldLayout: opts.oldLayout,
		numCommentLines: opts.numCommentLines,
		printDirection: opts.printDirection,
		// A font without the extended header field reports null upstream; the Go
		// port reports the same through a "has full layout" flag, so the two are
		// normalised to -1 here and there.
		fullLayout: opts.fullLayout === null || opts.fullLayout === undefined ? -1 : opts.fullLayout,
		fittingRules: {
			hLayout: opts.fittingRules.hLayout,
			hRule1: !!opts.fittingRules.hRule1,
			hRule2: !!opts.fittingRules.hRule2,
			hRule3: !!opts.fittingRules.hRule3,
			hRule4: !!opts.fittingRules.hRule4,
			hRule5: !!opts.fittingRules.hRule5,
			hRule6: !!opts.fittingRules.hRule6,
			vLayout: opts.fittingRules.vLayout,
			vRule1: !!opts.fittingRules.vRule1,
			vRule2: !!opts.fittingRules.vRule2,
			vRule3: !!opts.fittingRules.vRule3,
			vRule4: !!opts.fittingRules.vRule4,
			vRule5: !!opts.fittingRules.vRule5,
		},
	};
}

const ops = {
	// textSync(font, text, options) -> the rendered art, compared exactly.
	textSync([font, text, options]) {
		return figlet.textSync(text, {font, ...buildOptions(options)});
	},

	// registryRender(font, text) -> render by font *name*. Upstream resolves the
	// name to its own .flf file; the Go port resolves it against its own font
	// registry, which is why these cases are recorded as deviations.
	registryRender([font, text]) {
		return figlet.textSync(text, {font});
	},

	// metadata(font) -> the header fields both implementations parse.
	metadata([font]) {
		return metaOf(figlet.loadFontSync(font));
	},

	// parseFont(data) -> header fields, or an error for a malformed font. The
	// font is registered under a throwaway name so repeated cases stay
	// independent.
	parseFont([data]) {
		const name = '__parity__';
		figlet.clearLoadedFonts();
		figlet.defaults({fontPath: fontDir});
		return metaOf(figlet.parseFont(name, data));
	},

	// fontsSync() -> the sorted names of the FIGfonts in the harness' font dir.
	fontsSync() {
		return figlet.fontsSync().slice().sort();
	},

	// readAndRender(file, text, options) -> render a font read straight off disk
	// by file name (rather than by registered font name).
	readAndRender([file, text, options]) {
		const data = fs.readFileSync(path.join(fontDir, file), 'utf8');
		const name = '__file__' + file;
		figlet.parseFont(name, data);
		return figlet.textSync(text, {font: name, ...buildOptions(options)});
	},
};

// ------------------------------------------------------------------- the loop

const rl = readline.createInterface({input: process.stdin, terminal: false});

rl.on('line', (line) => {
	if (line.trim() === '') return;

	let req;
	try {
		req = JSON.parse(line);
	} catch (err) {
		process.stdout.write(JSON.stringify({id: null, ok: false, error: 'bad request: ' + err.message}) + '\n');
		return;
	}

	let out;
	try {
		const op = ops[req.fn];
		if (!op) throw new Error(`unknown fn: ${req.fn}`);
		out = {id: req.id, ok: true, value: op(req.args || [])};
	} catch (err) {
		out = {id: req.id, ok: false, error: err && err.message ? err.message : String(err)};
	}
	process.stdout.write(JSON.stringify(out) + '\n');
});
