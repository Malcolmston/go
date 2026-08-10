#!/usr/bin/env node
// Fixture generator for the sharp parity harness.
//
// Writes the shared inputs into ../fixtures/ ONCE, so both runners read the
// exact same bytes off disk. Nothing here is compared -- these are only inputs.
//
// The PNG fixtures are synthesised by a dependency-free PNG writer in this file
// from closed-form pixel formulas, so no image library has any say in what the
// canonical inputs look like. The non-PNG fixtures exist purely to probe
// *decode acceptance*; PNG, BMP and the "broken" files are written by hand
// here, while JPEG/GIF/TIFF/WebP/AVIF are transcoded from gradient.png by
// upstream sharp because it is the only side that can encode all of them. That
// biases nothing that is scored: the question those cases ask is "can the port
// read what sharp writes", which is the question that matters for a port.
//
// Usage: node gen-fixtures.mjs [outDir]

import {deflateSync} from 'node:zlib';
import {mkdirSync, writeFileSync, readFileSync} from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const outDir = process.argv[2] ?? path.join(import.meta.dirname, '..', 'fixtures');
mkdirSync(outDir, {recursive: true});

// ------------------------------------------------------------ PNG writer

const CRC_TABLE = (() => {
	const table = new Int32Array(256);
	for (let n = 0; n < 256; n++) {
		let c = n;
		for (let k = 0; k < 8; k++) {
			c = (c & 1) ? (0xEDB88320 ^ (c >>> 1)) : (c >>> 1);
		}

		table[n] = c;
	}

	return table;
})();

function crc32(buf) {
	let c = 0xFFFFFFFF;
	for (const byte of buf) {
		c = CRC_TABLE[(c ^ byte) & 0xFF] ^ (c >>> 8);
	}

	return (c ^ 0xFFFFFFFF) >>> 0;
}

function chunk(type, data) {
	const len = Buffer.alloc(4);
	len.writeUInt32BE(data.length);
	const body = Buffer.concat([Buffer.from(type, 'latin1'), data]);
	const crc = Buffer.alloc(4);
	crc.writeUInt32BE(crc32(body));
	return Buffer.concat([len, body, crc]);
}

// writePNG encodes width x height pixels as an 8-bit PNG. `channels` is 3 (RGB,
// colour type 2) or 4 (RGBA, colour type 6). `pixel(x, y)` returns an array of
// that many 0..255 values. Filter type 0 on every row keeps this trivially
// reproducible.
function writePNG(file, width, height, channels, pixel) {
	const colourType = channels === 4 ? 6 : 2;
	const raw = Buffer.alloc(height * (1 + (width * channels)));
	let o = 0;
	for (let y = 0; y < height; y++) {
		raw[o++] = 0;
		for (let x = 0; x < width; x++) {
			const px = pixel(x, y);
			for (let c = 0; c < channels; c++) {
				raw[o++] = px[c] & 0xFF;
			}
		}
	}

	const ihdr = Buffer.alloc(13);
	ihdr.writeUInt32BE(width, 0);
	ihdr.writeUInt32BE(height, 4);
	ihdr[8] = 8;
	ihdr[9] = colourType;
	const png = Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]),
		chunk('IHDR', ihdr),
		chunk('IDAT', deflateSync(raw, {level: 9})),
		chunk('IEND', Buffer.alloc(0)),
	]);
	writeFileSync(path.join(outDir, file), png);
}

// ------------------------------------------------------------ pixel formulas

const gradient = (w, h) => (x, y) => [
	Math.round((x * 255) / (w - 1)),
	Math.round((y * 255) / (h - 1)),
	((x * 3) + (y * 5)) & 0xFF,
];

// 8px black/white checkerboard: the harshest possible input for a resampler,
// which is exactly why it is a good gross-error detector.
const checker = (x, y) => {
	const v = ((Math.floor(x / 8) + Math.floor(y / 8)) % 2) === 0 ? 0 : 255;
	return [v, v, v];
};

// A semi-transparent disc on a fully transparent field.
const circle = (x, y) => {
	const dx = x - 19.5;
	const dy = y - 19.5;
	const d = Math.sqrt((dx * dx) + (dy * dy));
	if (d <= 15) {
		return [220, 60, 40, 160];
	}

	if (d <= 18) {
		return [80, 140, 200, 64];
	}

	return [0, 0, 0, 0];
};

writePNG('gradient.png', 64, 48, 3, gradient(64, 48));
writePNG('checker.png', 32, 32, 3, checker);
writePNG('circle.png', 40, 40, 4, circle);
// Every pixel fully opaque, but stored in an RGBA (colour type 6) PNG: the file
// genuinely has 4 channels.
writePNG('opaque-rgba.png', 24, 24, 4, (x, y) => [...gradient(24, 24)(x, y), 255]);
// Deliberately awkward, coprime dimensions so aspect-ratio rounding shows up.
writePNG('odd.png', 33, 21, 3, gradient(33, 21));
writePNG('wide.png', 90, 10, 3, gradient(90, 10));
writePNG('tall.png', 10, 90, 3, gradient(10, 90));
// A second same-size operand for boolean/composite cases.
writePNG('mask.png', 64, 48, 3, (x, y) => {
	const v = ((x % 16) < 8) === ((y % 16) < 8) ? 0xF0 : 0x0F;
	return [v, v, v];
});

// ------------------------------------------------------------ BMP writer
// 24bpp bottom-up BMP. Neither library's own encoder is involved.
function writeBMP(file, width, height, pixel) {
	const rowSize = ((width * 3) + 3) & ~3;
	const pixels = Buffer.alloc(rowSize * height);
	for (let y = 0; y < height; y++) {
		let o = (height - 1 - y) * rowSize;
		for (let x = 0; x < width; x++) {
			const [r, g, b] = pixel(x, y);
			pixels[o++] = b;
			pixels[o++] = g;
			pixels[o++] = r;
		}
	}

	const header = Buffer.alloc(54);
	header.write('BM', 0, 'latin1');
	header.writeUInt32LE(54 + pixels.length, 2);
	header.writeUInt32LE(54, 10);
	header.writeUInt32LE(40, 14);
	header.writeInt32LE(width, 18);
	header.writeInt32LE(height, 22);
	header.writeUInt16LE(1, 26);
	header.writeUInt16LE(24, 28);
	header.writeUInt32LE(pixels.length, 34);
	writeFileSync(path.join(outDir, file), Buffer.concat([header, pixels]));
}

writeBMP('gradient.bmp', 64, 48, gradient(64, 48));

// ------------------------------------------------------------ broken inputs

// Deterministic non-image bytes (a fixed LCG, no Math.random anywhere).
const garbage = Buffer.alloc(512);
let seed = 12345;
for (let i = 0; i < garbage.length; i++) {
	seed = ((seed * 1103515245) + 12345) & 0x7FFFFFFF;
	garbage[i] = (seed >>> 16) & 0xFF;
}

writeFileSync(path.join(outDir, 'garbage.bin'), garbage);
writeFileSync(path.join(outDir, 'empty.bin'), Buffer.alloc(0));
// A valid PNG signature and IHDR, then nothing.
writeFileSync(
	path.join(outDir, 'truncated.png'),
	readFileSync(path.join(outDir, 'gradient.png')).subarray(0, 40),
);

// ------------------------------------------------- transcoded probe inputs

const sharp = (await import('sharp')).default;
const src = path.join(outDir, 'gradient.png');
const jobs = [
	['gradient.jpg', {jpeg: {quality: 90, chromaSubsampling: '4:4:4'}}],
	['gradient.gif', {gif: {}}],
	['gradient.tiff', {tiff: {compression: 'none'}}],
	['gradient.webp', {webp: {quality: 90}}],
	['gradient.avif', {avif: {quality: 60}}],
];
for (const [file, opts] of jobs) {
	const [format, o] = Object.entries(opts)[0];
	// eslint-disable-next-line no-await-in-loop
	await sharp(src).toFormat(format, o).toFile(path.join(outDir, file));
}

process.stderr.write(`fixtures written to ${outDir}\n`);
