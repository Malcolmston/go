#!/usr/bin/env node
// Upstream parity runner for sharp@0.33.5.
//
// Speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. Never exits on a failing
// case -- a throw or a rejected promise is reported as
// {"ok": false, "error": "..."} and the runner keeps reading.
//
// sharp is asynchronous, so requests are serialised through a promise chain:
// the reply for line N is written before line N+1 is dispatched, which is what
// keeps the stream ordered and the harness in sync.
//
// Determinism: libvips concurrency is pinned to 1 and its operation cache is
// disabled, so a case can never see a different answer because of thread
// scheduling or a warm cache.

import * as readline from 'node:readline';
import {mkdtempSync} from 'node:fs';
import {readFile, unlink} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import sharp from 'sharp';

sharp.concurrency(1);
sharp.cache(false);
sharp.simd(true);

const FIXTURES = path.join(import.meta.dirname, '..', 'fixtures');
const TMP = mkdtempSync(path.join(os.tmpdir(), 'parity-sharp-'));

const fixture = name => path.join(FIXTURES, name);

// ------------------------------------------------------------- format sniffing

// sniff names an encoded byte stream from its magic number only. Both runners
// implement the identical table, so "what did this entry point actually write"
// is comparable without either library's own detector being trusted.
function sniff(buf) {
	const has = (off, ...bytes) => bytes.every((b, i) => buf[off + i] === b);
	if (buf.length >= 8 && has(0, 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A)) {
		return 'png';
	}

	if (buf.length >= 3 && has(0, 0xFF, 0xD8, 0xFF)) {
		return 'jpeg';
	}

	if (buf.length >= 6 && has(0, 0x47, 0x49, 0x46, 0x38)) {
		return 'gif';
	}

	if (buf.length >= 2 && has(0, 0x42, 0x4D)) {
		return 'bmp';
	}

	if (buf.length >= 4 && (has(0, 0x49, 0x49, 0x2A, 0x00) || has(0, 0x4D, 0x4D, 0x00, 0x2A))) {
		return 'tiff';
	}

	if (buf.length >= 12 && has(0, 0x52, 0x49, 0x46, 0x46) && has(8, 0x57, 0x45, 0x42, 0x50)) {
		return 'webp';
	}

	if (buf.length >= 12 && has(4, 0x66, 0x74, 0x79, 0x70)) {
		return 'isobmff'; // avif / heic / mp4 family
	}

	return 'unknown';
}

// ------------------------------------------------------------------ op mapping

// Every op name maps to exactly one sharp call. An op the case file asks for
// that sharp cannot express throws, and that becomes ok:false -- the same way
// the Go runner reports an operation the port lacks.
const OPS = {
	resize(p, o) {
		const opts = {};
		for (const k of ['width', 'height', 'fit', 'kernel', 'position', 'withoutEnlargement', 'withoutReduction']) {
			if (o[k] !== undefined) {
				opts[k] = o[k];
			}
		}

		if (o.background !== undefined) {
			opts.background = o.background;
		}

		return p.resize(opts);
	},

	// The zero-configuration call, recorded so the two libraries' *defaults*
	// are compared rather than an explicitly-pinned fit and kernel.
	resizeDefault(p, o) {
		return p.resize(o.width ?? null, o.height ?? null);
	},

	extract(p, o) {
		return p.extract({left: o.left, top: o.top, width: o.width, height: o.height});
	},

	extend(p, o) {
		const opts = {top: o.top ?? 0, right: o.right ?? 0, bottom: o.bottom ?? 0, left: o.left ?? 0};
		if (o.background !== undefined) {
			opts.background = o.background;
		}

		if (o.extendWith !== undefined) {
			opts.extendWith = o.extendWith;
		}

		return p.extend(opts);
	},

	rotate(p, o) {
		const opts = {};
		if (o.background !== undefined) {
			opts.background = o.background;
		}

		return p.rotate(o.angle, opts);
	},

	flip: p => p.flip(),
	flop: p => p.flop(),
	grayscale: p => p.grayscale(),
	negate: p => p.negate(),
	removeAlpha: p => p.removeAlpha(),
	ensureAlpha: (p, o) => p.ensureAlpha(o.alpha ?? 1),
	flatten: (p, o) => p.flatten(o.background === undefined ? true : {background: o.background}),
	blur: (p, o) => p.blur(o.sigma),
	sharpen: (p, o) => p.sharpen({sigma: o.sigma}),
	gamma: (p, o) => p.gamma(o.gamma),
	linear: (p, o) => p.linear(o.a, o.b),
	threshold: (p, o) => p.threshold(o.threshold),
	tint: (p, o) => p.tint(o.colour),
	median: (p, o) => p.median(o.size),
	recomb: (p, o) => p.recomb(o.matrix),
	modulate: (p, o) => p.modulate(o),
	extractChannel: (p, o) => p.extractChannel(o.channel),
	toColourspace: (p, o) => p.toColourspace(o.space),
	trim: (p, o) => p.trim(o.threshold === undefined ? true : {threshold: o.threshold}),
	unflatten: p => p.unflatten(),
	boolean: (p, o) => p.boolean(fixture(o.operand), o.operator),
	bandbool: (p, o) => p.bandbool(o.operator),
	composite(p, o) {
		const layer = {input: fixture(o.operand)};
		if (o.blend !== undefined) {
			layer.blend = o.blend;
		}

		if (o.gravity !== undefined) {
			layer.gravity = o.gravity;
		}

		if (o.left !== undefined) {
			layer.left = o.left;
			layer.top = o.top;
		}

		return p.composite([layer]);
	},
	convolve: (p, o) => p.convolve({width: o.size, height: o.size, kernel: o.kernelWeights, scale: o.scale ?? 1, offset: o.offset ?? 0}),
	normalise: (p, o) => p.normalise({lower: o.lower ?? 1, upper: o.upper ?? 99}),
	affine: (p, o) => p.affine(o.matrix, o.background === undefined ? {} : {background: o.background}),
	clahe: (p, o) => p.clahe({width: o.width, height: o.height, maxSlope: o.maxSlope ?? 3}),
};

function applyOps(p, ops = []) {
	let cur = p;
	for (const raw of ops) {
		const [name, opts = {}] = Array.isArray(raw) ? raw : [raw, {}];
		const fn = OPS[name];
		if (fn === undefined) {
			throw new Error(`unknown op: ${name}`);
		}

		cur = fn(cur, opts);
	}

	return cur;
}

// ------------------------------------------------------------------ pixel view

// rawRGBA runs the pipeline and normalises whatever band count libvips ends up
// with into a flat 4-channel RGBA view, so a 1-band greyscale result and a
// 4-band result are described in the same coordinates on both sides.
async function rawRGBA(p) {
	const {data, info} = await p.raw().toBuffer({resolveWithObject: true});
	const {width, height, channels} = info;
	const out = Buffer.alloc(width * height * 4);
	for (let i = 0, n = width * height; i < n; i++) {
		const s = i * channels;
		const d = i * 4;
		switch (channels) {
			case 1: {
				out[d] = data[s];
				out[d + 1] = data[s];
				out[d + 2] = data[s];
				out[d + 3] = 255;
				break;
			}

			case 2: {
				out[d] = data[s];
				out[d + 1] = data[s];
				out[d + 2] = data[s];
				out[d + 3] = data[s + 1];
				break;
			}

			case 3: {
				out[d] = data[s];
				out[d + 1] = data[s + 1];
				out[d + 2] = data[s + 2];
				out[d + 3] = 255;
				break;
			}

			default: {
				out[d] = data[s];
				out[d + 1] = data[s + 1];
				out[d + 2] = data[s + 2];
				out[d + 3] = data[s + 3];
			}
		}
	}

	return {pix: out, width, height, channels};
}

const GRID = 4;

// pixelSummary is the coarse, tolerance-compared description of an image:
// per-channel mean/min/max plus the mean luma of each cell of a 4x4 grid. It is
// deliberately *not* a pixel comparison -- see COVERAGE.md.
function pixelSummary({pix, width, height}) {
	const sum = [0, 0, 0, 0];
	const min = [255, 255, 255, 255];
	const max = [0, 0, 0, 0];
	const n = width * height;
	for (let i = 0; i < n; i++) {
		for (let c = 0; c < 4; c++) {
			const v = pix[(i * 4) + c];
			sum[c] += v;
			if (v < min[c]) {
				min[c] = v;
			}

			if (v > max[c]) {
				max[c] = v;
			}
		}
	}

	const grid = [];
	for (let gy = 0; gy < GRID; gy++) {
		const y0 = Math.floor((gy * height) / GRID);
		const y1 = Math.floor(((gy + 1) * height) / GRID);
		for (let gx = 0; gx < GRID; gx++) {
			const x0 = Math.floor((gx * width) / GRID);
			const x1 = Math.floor(((gx + 1) * width) / GRID);
			let acc = 0;
			let count = 0;
			for (let y = y0; y < y1; y++) {
				for (let x = x0; x < x1; x++) {
					const i = ((y * width) + x) * 4;
					acc += (0.299 * pix[i]) + (0.587 * pix[i + 1]) + (0.114 * pix[i + 2]);
					count++;
				}
			}

			grid.push(count === 0 ? 0 : acc / count);
		}
	}

	return {
		width,
		height,
		mean: sum.map(s => (n === 0 ? 0 : s / n)),
		min,
		max,
		grid,
	};
}

// ------------------------------------------------------------------- handlers

const EXT = {jpeg: 'jpg', raw: 'raw', tiff: 'tif'};

const HANDLERS = {
	async metadata([name]) {
		const m = await sharp(fixture(name)).metadata();
		return {
			format: m.format,
			width: m.width,
			height: m.height,
			channels: m.channels,
			hasAlpha: Boolean(m.hasAlpha),
			space: m.space,
		};
	},

	async decode([name]) {
		const m = await sharp(fixture(name)).metadata();
		return {format: m.format, width: m.width, height: m.height};
	},

	async geometry([name, ops]) {
		const {width, height, channels} = await rawRGBA(applyOps(sharp(fixture(name)), ops));
		return {width, height, channels, hasAlpha: channels === 2 || channels === 4};
	},

	async pixels([name, ops]) {
		return pixelSummary(await rawRGBA(applyOps(sharp(fixture(name)), ops)));
	},

	async encode([name, format, opts = {}]) {
		const buf = await sharp(fixture(name)).toFormat(format, opts).toBuffer();
		const m = await sharp(buf).metadata();
		return {
			sniff: sniff(buf),
			format: m.format,
			width: m.width,
			height: m.height,
			channels: m.channels,
		};
	},

	// Two entry points, one question: does writing to a file agree with
	// writing to a buffer about what format came out?
	async tofile([name, format]) {
		const target = path.join(TMP, `out.${EXT[format] ?? format}`);
		const buf = await sharp(fixture(name)).toFormat(format).toBuffer();
		await sharp(fixture(name)).toFormat(format).toFile(target);
		const onDisk = await readFile(target);
		await unlink(target).catch(() => {});
		return {buffer: sniff(buf), file: sniff(onDisk), agree: sniff(buf) === sniff(onDisk)};
	},

	// Both format-support answers are *probes*, not introspection: each
	// candidate is actually encoded (or actually decoded) and kept only if that
	// succeeded. The Go runner probes the identical candidate list the same way,
	// which is the only way the two answers are comparable at all.
	async outputFormats([names]) {
		const out = [];
		for (const name of names) {
			try {
				// eslint-disable-next-line no-await-in-loop
				await sharp(fixture('gradient.png')).toFormat(name).toBuffer();
				out.push(name);
			} catch {
				// Not an output format here.
			}
		}

		return out.sort();
	},

	async inputFormats([names]) {
		const out = [];
		for (const name of names) {
			try {
				// eslint-disable-next-line no-await-in-loop
				await sharp(fixture(name)).metadata();
				out.push(name);
			} catch {
				// Not a decodable input here.
			}
		}

		return out.sort();
	},

	// Round-trip through raw bytes: does the library read back what it wrote?
	async rawRoundTrip([name]) {
		const {data, info} = await sharp(fixture(name)).raw().toBuffer({resolveWithObject: true});
		const back = await sharp(data, {raw: {width: info.width, height: info.height, channels: info.channels}})
			.raw()
			.toBuffer();
		return {bytes: data.length, channels: info.channels, identical: Buffer.compare(data, back) === 0};
	},
};

// ---------------------------------------------------------------- the loop

const rl = readline.createInterface({input: process.stdin, crlfDelay: Infinity});
let chain = Promise.resolve();

rl.on('line', line => {
	const text = line.trim();
	if (text === '') {
		return;
	}

	chain = chain.then(async () => {
		let id = null;
		let response;
		try {
			const request = JSON.parse(text);
			id = request.id ?? null;
			const handler = HANDLERS[request.fn];
			if (handler === undefined) {
				throw new Error(`unknown fn: ${request.fn}`);
			}

			response = {id, ok: true, value: await handler(request.args ?? [])};
		} catch (error) {
			const message = error && error.message ? error.message : String(error);
			response = {id, ok: false, error: String(message).split('\n')[0]};
		}

		process.stdout.write(JSON.stringify(response) + '\n');
	});
});
