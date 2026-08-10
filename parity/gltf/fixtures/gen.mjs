// Fixture generator for the gltf parity harness.
//
// Deliberately dependency-free: it uses only Node built-ins and hand-writes the
// glTF JSON and the GLB container, so the fixtures are not biased toward either
// implementation under test (neither the Go port nor @gltf-transform produced
// them). Output is byte-for-byte reproducible.
//
//   node fixtures/gen.mjs
//
// Writes:
//   fixtures/triangle.gltf   fixtures/triangle.glb
//   fixtures/scene.gltf      fixtures/animated.glb
//   fixtures/accessors.glb
//   fixtures/malformed/*.gltf, *.glb
import { writeFileSync, mkdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateSync } from 'node:zlib';

const HERE = dirname(fileURLToPath(import.meta.url));

const COMP_SIZE = { 5120: 1, 5121: 1, 5122: 2, 5123: 2, 5125: 4, 5126: 4 };
const TYPE_COMPS = { SCALAR: 1, VEC2: 2, VEC3: 3, VEC4: 4, MAT2: 4, MAT3: 9, MAT4: 16 };
const ARRAY_BUFFER = 34962;
const ELEMENT_ARRAY_BUFFER = 34963;

// ------------------------------------------------------------------ builder

class Builder {
	constructor() {
		this.chunks = [];
		this.length = 0;
		this.json = {
			asset: { version: '2.0', generator: 'malcolmston/go parity fixture generator' },
			scene: 0,
			scenes: [],
			nodes: [],
			meshes: [],
			accessors: [],
			bufferViews: [],
			buffers: [],
			materials: [],
			textures: [],
			images: [],
			samplers: [],
			animations: [],
			skins: [],
			cameras: [],
		};
	}

	// align pads the binary blob so the next bufferView starts on a 4-byte
	// boundary, as required for GLB BIN chunks and vertex data.
	align() {
		const pad = (4 - (this.length % 4)) % 4;
		if (pad) this.raw(Buffer.alloc(pad));
	}

	raw(buf) {
		this.chunks.push(buf);
		this.length += buf.length;
		return this.length - buf.length;
	}

	addView(buf, { target, byteStride } = {}) {
		this.align();
		const offset = this.raw(buf);
		const view = { buffer: 0, byteOffset: offset, byteLength: buf.length };
		if (byteStride !== undefined) view.byteStride = byteStride;
		if (target !== undefined) view.target = target;
		this.json.bufferViews.push(view);
		return this.json.bufferViews.length - 1;
	}

	// addAccessor registers an accessor over an existing bufferView, computing
	// min/max from `values` (the raw, un-normalized components) so the declared
	// bounds always agree with the data — gltf-validator checks this.
	addAccessor({ bufferView, componentType, type, count, normalized, byteOffset, values, name, minmax = true }) {
		const comps = TYPE_COMPS[type];
		const acc = { componentType, count, type };
		if (bufferView !== undefined) acc.bufferView = bufferView;
		if (byteOffset) acc.byteOffset = byteOffset;
		if (normalized) acc.normalized = true;
		if (name) acc.name = name;
		if (minmax && values) {
			const min = new Array(comps).fill(Infinity);
			const max = new Array(comps).fill(-Infinity);
			for (let i = 0; i < count; i++) {
				for (let j = 0; j < comps; j++) {
					const v = values[i * comps + j];
					if (v < min[j]) min[j] = v;
					if (v > max[j]) max[j] = v;
				}
			}
			// Only FLOAT bounds are rounded to float32: rounding an integer
			// extreme such as 4294967295 would push it out of its GL range and
			// gltf-validator rejects that (INVALID_GL_VALUE).
			const round = componentType === 5126 ? fround : (v) => v;
			acc.min = min.map(round);
			acc.max = max.map(round);
		}
		this.json.accessors.push(acc);
		return this.json.accessors.length - 1;
	}

	// data packs flat numeric components into a little-endian buffer.
	static data(componentType, values) {
		const size = COMP_SIZE[componentType];
		const buf = Buffer.alloc(size * values.length);
		values.forEach((v, i) => {
			const at = i * size;
			switch (componentType) {
				case 5120: buf.writeInt8(v, at); break;
				case 5121: buf.writeUInt8(v, at); break;
				case 5122: buf.writeInt16LE(v, at); break;
				case 5123: buf.writeUInt16LE(v, at); break;
				case 5125: buf.writeUInt32LE(v, at); break;
				case 5126: buf.writeFloatLE(v, at); break;
				default: throw new Error('bad componentType ' + componentType);
			}
		});
		return buf;
	}

	// packed writes values as their own bufferView plus accessor.
	packed({ componentType, type, values, normalized, target, name }) {
		const count = values.length / TYPE_COMPS[type];
		const view = this.addView(Builder.data(componentType, values), { target });
		return this.addAccessor({ bufferView: view, componentType, type, count, normalized, values, name });
	}

	bin() {
		this.align();
		return Buffer.concat(this.chunks);
	}

	// finish drops empty top-level arrays so the JSON stays minimal, then
	// returns {json, bin}.
	finish() {
		const bin = this.bin();
		this.json.buffers = [{ byteLength: bin.length }];
		const json = {};
		for (const key of Object.keys(this.json)) {
			const v = this.json[key];
			if (Array.isArray(v) && v.length === 0) continue;
			json[key] = v;
		}
		return { json, bin };
	}
}

const fround = (v) => Math.fround(v);

// ------------------------------------------------------------------- output

function writeGLTFEmbedded(name, { json, bin }) {
	const doc = JSON.parse(JSON.stringify(json));
	doc.buffers[0].uri = 'data:application/octet-stream;base64,' + bin.toString('base64');
	writeFileSync(join(HERE, name), JSON.stringify(doc, null, 2) + '\n');
}

function writeGLB(name, { json, bin }, { magic = 0x46546c67, version = 2, truncate = 0 } = {}) {
	writeFileSync(join(HERE, name), buildGLB(json, bin, { magic, version, truncate }));
}

function buildGLB(json, bin, { magic = 0x46546c67, version = 2, truncate = 0 } = {}) {
	const jsonBuf = padTo(Buffer.from(JSON.stringify(json), 'utf8'), 0x20);
	const binBuf = bin && bin.length ? padTo(bin, 0x00) : null;
	const total = 12 + 8 + jsonBuf.length + (binBuf ? 8 + binBuf.length : 0);
	const out = Buffer.alloc(total);
	out.writeUInt32LE(magic, 0);
	out.writeUInt32LE(version, 4);
	out.writeUInt32LE(total, 8);
	out.writeUInt32LE(jsonBuf.length, 12);
	out.writeUInt32LE(0x4e4f534a, 16); // 'JSON'
	jsonBuf.copy(out, 20);
	if (binBuf) {
		const at = 20 + jsonBuf.length;
		out.writeUInt32LE(binBuf.length, at);
		out.writeUInt32LE(0x004e4942, at + 4); // 'BIN\0'
		binBuf.copy(out, at + 8);
	}
	return truncate ? out.subarray(0, out.length - truncate) : out;
}

function padTo(buf, fill) {
	const pad = (4 - (buf.length % 4)) % 4;
	if (!pad) return buf;
	return Buffer.concat([buf, Buffer.alloc(pad, fill)]);
}

// ------------------------------------------------------------- 1x1 PNG image

const CRC_TABLE = (() => {
	const t = new Int32Array(256);
	for (let n = 0; n < 256; n++) {
		let c = n;
		for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
		t[n] = c;
	}
	return t;
})();

function crc32(buf) {
	let c = -1;
	for (const b of buf) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8);
	return (c ^ -1) >>> 0;
}

function pngChunk(type, data) {
	const len = Buffer.alloc(4);
	len.writeUInt32BE(data.length, 0);
	const body = Buffer.concat([Buffer.from(type, 'ascii'), data]);
	const crc = Buffer.alloc(4);
	crc.writeUInt32BE(crc32(body), 0);
	return Buffer.concat([len, body, crc]);
}

// png2x2 builds a valid 2x2 8-bit RGBA PNG with fixed pixel data.
function png2x2() {
	const ihdr = Buffer.alloc(13);
	ihdr.writeUInt32BE(2, 0);
	ihdr.writeUInt32BE(2, 4);
	ihdr[8] = 8; // bit depth
	ihdr[9] = 6; // colour type RGBA
	const px = [
		[255, 0, 0, 255], [0, 255, 0, 255],
		[0, 0, 255, 255], [255, 255, 255, 128],
	];
	const rows = [];
	for (let y = 0; y < 2; y++) {
		rows.push(Buffer.from([0, ...px[y * 2], ...px[y * 2 + 1]]));
	}
	const idat = deflateSync(Buffer.concat(rows), { level: 9 });
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		pngChunk('IHDR', ihdr),
		pngChunk('IDAT', idat),
		pngChunk('IEND', Buffer.alloc(0)),
	]);
}

const PNG_DATA_URI = 'data:image/png;base64,' + png2x2().toString('base64');

// ---------------------------------------------------------------- fixture: triangle

function triangle() {
	const b = new Builder();
	const positions = [0, 0, 0, 1, 0, 0, 0, 1, 0];
	const pos = b.packed({ componentType: 5126, type: 'VEC3', values: positions, target: ARRAY_BUFFER, name: 'positions' });
	const idx = b.packed({ componentType: 5123, type: 'SCALAR', values: [0, 1, 2], target: ELEMENT_ARRAY_BUFFER, name: 'indices' });
	b.json.materials.push({
		name: 'flat',
		pbrMetallicRoughness: { baseColorFactor: [0.8, 0.2, 0.1, 1], metallicFactor: 0.25, roughnessFactor: 0.75 },
	});
	b.json.meshes.push({ name: 'tri', primitives: [{ attributes: { POSITION: pos }, indices: idx, material: 0 }] });
	b.json.nodes.push({ name: 'triNode', mesh: 0 });
	b.json.scenes.push({ name: 'main', nodes: [0] });
	return b.finish();
}

// ------------------------------------------------------------------- fixture: scene

function scene() {
	const b = new Builder();
	const positions = [0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 0];
	const pos = b.packed({ componentType: 5126, type: 'VEC3', values: positions, target: ARRAY_BUFFER, name: 'POSITION' });
	const nrm = b.packed({ componentType: 5126, type: 'VEC3', values: [0, 0, 1, 0, 0, 1, 0, 0, 1, 0, 0, 1], target: ARRAY_BUFFER, name: 'NORMAL' });
	const uv = b.packed({ componentType: 5126, type: 'VEC2', values: [0, 0, 1, 0, 1, 1, 0, 1], target: ARRAY_BUFFER, name: 'TEXCOORD_0' });
	const col = b.packed({
		componentType: 5121, type: 'VEC4', normalized: true, target: ARRAY_BUFFER, name: 'COLOR_0',
		values: [255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 128, 128, 128, 64],
	});
	const idx = b.packed({ componentType: 5123, type: 'SCALAR', values: [0, 1, 2, 0, 2, 3], target: ELEMENT_ARRAY_BUFFER, name: 'indices' });

	b.json.images.push({ name: 'checker', uri: PNG_DATA_URI, mimeType: 'image/png' });
	b.json.samplers.push({ name: 'clamp', magFilter: 9728, minFilter: 9987, wrapS: 33071, wrapT: 33648 });
	b.json.textures.push({ name: 'checkerTex', source: 0, sampler: 0 });

	b.json.materials.push({
		name: 'pbr',
		pbrMetallicRoughness: {
			baseColorFactor: [0.5, 0.6, 0.7, 0.8],
			metallicFactor: 0.125,
			roughnessFactor: 0.875,
			baseColorTexture: {
				index: 0,
				texCoord: 0,
				extensions: {
					KHR_texture_transform: { offset: [0.25, 0.5], rotation: 0.7853981633974483, scale: [2, 3] },
				},
			},
			metallicRoughnessTexture: { index: 0 },
		},
		normalTexture: { index: 0, scale: 0.5 },
		occlusionTexture: { index: 0, strength: 0.25 },
		emissiveTexture: { index: 0 },
		emissiveFactor: [0.1, 0.2, 0.3],
		alphaMode: 'MASK',
		alphaCutoff: 0.25,
		doubleSided: true,
	});
	b.json.materials.push({
		name: 'unlit',
		pbrMetallicRoughness: { baseColorFactor: [1, 0.5, 0, 1] },
		extensions: {
			KHR_materials_unlit: {},
			KHR_materials_emissive_strength: { emissiveStrength: 3.5 },
		},
		emissiveFactor: [0.4, 0.4, 0.4],
	});
	b.json.materials.push({
		name: 'glass',
		pbrMetallicRoughness: { baseColorFactor: [0.9, 0.9, 1, 1], metallicFactor: 0, roughnessFactor: 0.1 },
		extensions: {
			KHR_materials_ior: { ior: 1.7 },
			KHR_materials_transmission: { transmissionFactor: 0.9 },
			KHR_materials_volume: { thicknessFactor: 0.4, attenuationDistance: 2.5, attenuationColor: [0.8, 0.9, 1] },
			KHR_materials_clearcoat: { clearcoatFactor: 0.6, clearcoatRoughnessFactor: 0.2 },
			KHR_materials_sheen: { sheenColorFactor: [0.3, 0.2, 0.1], sheenRoughnessFactor: 0.5 },
			KHR_materials_specular: { specularFactor: 0.7, specularColorFactor: [0.2, 0.4, 0.6] },
		},
	});

	b.json.meshes.push({
		name: 'quad',
		primitives: [
			{ attributes: { POSITION: pos, NORMAL: nrm, TEXCOORD_0: uv, COLOR_0: col }, indices: idx, material: 0 },
			{ attributes: { POSITION: pos }, mode: 0, material: 1 },
			{ attributes: { POSITION: pos }, mode: 1, material: 2 },
		],
	});

	b.json.cameras.push({
		name: 'persp', type: 'perspective',
		perspective: { yfov: 0.8, aspectRatio: 1.5, znear: 0.1, zfar: 100 },
	});
	b.json.cameras.push({
		name: 'ortho', type: 'orthographic',
		orthographic: { xmag: 2, ymag: 1.5, znear: 0.01, zfar: 50 },
	});

	b.json.nodes.push({
		name: 'trs',
		translation: [1, 2, 3],
		rotation: [0, 0.7071067811865476, 0, 0.7071067811865476],
		scale: [2, 2, 2],
		children: [1],
	});
	b.json.nodes.push({
		name: 'matrix',
		matrix: [2, 0, 0, 0, 0, 3, 0, 0, 0, 0, 4, 0, 5, 6, 7, 1],
		mesh: 0,
	});
	b.json.nodes.push({ name: 'meshNode', mesh: 0 });
	b.json.nodes.push({ name: 'perspNode', camera: 0, translation: [0, 0, 5] });
	b.json.nodes.push({ name: 'orthoNode', camera: 1 });
	b.json.nodes.push({
		name: 'spotNode',
		translation: [0, 4, 0],
		extensions: { KHR_lights_punctual: { light: 0 } },
	});
	b.json.scenes.push({ name: 'main', nodes: [0, 2, 3, 4, 5] });

	b.json.extensions = {
		KHR_lights_punctual: {
			lights: [{
				name: 'spot',
				type: 'spot',
				color: [1, 0.9, 0.8],
				intensity: 5,
				range: 10,
				spot: { innerConeAngle: 0.2, outerConeAngle: 0.6 },
			}],
		},
	};

	b.json.extensionsUsed = [
		'KHR_lights_punctual',
		'KHR_materials_clearcoat',
		'KHR_materials_emissive_strength',
		'KHR_materials_ior',
		'KHR_materials_sheen',
		'KHR_materials_specular',
		'KHR_materials_transmission',
		'KHR_materials_unlit',
		'KHR_materials_volume',
		'KHR_texture_transform',
	];
	return b.finish();
}

// ---------------------------------------------------------------- fixture: animated

function animated() {
	const b = new Builder();
	const pos = b.packed({
		componentType: 5126, type: 'VEC3', target: ARRAY_BUFFER, name: 'POSITION',
		values: [0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 1, 0],
	});
	const joints = b.packed({
		componentType: 5121, type: 'VEC4', target: ARRAY_BUFFER, name: 'JOINTS_0',
		values: [0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0],
	});
	const weights = b.packed({
		componentType: 5126, type: 'VEC4', target: ARRAY_BUFFER, name: 'WEIGHTS_0',
		values: [1, 0, 0, 0, 0.5, 0.5, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0],
	});
	const morph = b.packed({
		componentType: 5126, type: 'VEC3', target: ARRAY_BUFFER, name: 'morphPOSITION',
		values: [0, 0, 0.5, 0, 0, 0.5, 0, 0, 1, 0, 0, 0],
	});
	const idx = b.packed({ componentType: 5123, type: 'SCALAR', values: [0, 1, 2, 0, 2, 3], target: ELEMENT_ARRAY_BUFFER, name: 'indices' });

	// Inverse bind matrices: identity and a translation of (-1,0,0).
	const ibmValues = [
		1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1,
		1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, -1, 0, 0, 1,
	];
	const ibm = b.packed({ componentType: 5126, type: 'MAT4', values: ibmValues, name: 'IBM' });

	// Animation samplers.
	const times = b.packed({ componentType: 5126, type: 'SCALAR', values: [0, 0.5, 1], name: 'times' });
	const trans = b.packed({ componentType: 5126, type: 'VEC3', values: [0, 0, 0, 0, 1, 0, 0, 2, 0], name: 'translations' });
	const rots = b.packed({
		componentType: 5126, type: 'VEC4', name: 'rotations',
		values: [0, 0, 0, 1, 0, 0.7071067811865476, 0, 0.7071067811865476, 0, 1, 0, 0],
	});
	const scales = b.packed({ componentType: 5126, type: 'VEC3', values: [1, 1, 1, 2, 2, 2, 0.5, 0.5, 0.5], name: 'scales' });
	const morphW = b.packed({ componentType: 5126, type: 'SCALAR', values: [0, 1, 0.25], name: 'morphWeights' });
	// CUBICSPLINE: three values (in-tangent, value, out-tangent) per keyframe.
	const spline = b.packed({
		componentType: 5126, type: 'VEC3', name: 'splineTranslations',
		values: [
			0, 0, 0, 0, 0, 0, 0, 0, 1,
			0, 0, -1, 1, 0, 0, 0, 0, 1,
			0, 0, -1, 2, 0, 0, 0, 0, 0,
		],
	});

	b.json.meshes.push({
		name: 'skinned',
		weights: [0.5],
		primitives: [{
			attributes: { POSITION: pos, JOINTS_0: joints, WEIGHTS_0: weights },
			indices: idx,
			targets: [{ POSITION: morph }],
		}],
	});

	// The skinned-mesh node is kept a scene root: a non-root skinned mesh is
	// legal but draws a gltf-validator warning about ignored parent transforms.
	b.json.nodes.push({ name: 'root', children: [1] });
	b.json.nodes.push({ name: 'joint0', children: [2], translation: [0, 0, 0] });
	b.json.nodes.push({ name: 'joint1', translation: [1, 0, 0] });
	b.json.nodes.push({ name: 'skinnedMesh', mesh: 0, skin: 0 });
	b.json.skins.push({ name: 'skin', joints: [1, 2], skeleton: 1, inverseBindMatrices: ibm });
	b.json.scenes.push({ name: 'main', nodes: [0] });

	b.json.animations.push({
		name: 'anim',
		samplers: [
			{ input: times, output: trans, interpolation: 'LINEAR' },
			{ input: times, output: rots, interpolation: 'STEP' },
			{ input: times, output: scales },
			{ input: times, output: morphW, interpolation: 'LINEAR' },
			{ input: times, output: spline, interpolation: 'CUBICSPLINE' },
		],
		channels: [
			{ sampler: 0, target: { node: 2, path: 'translation' } },
			{ sampler: 1, target: { node: 2, path: 'rotation' } },
			{ sampler: 2, target: { node: 1, path: 'scale' } },
			{ sampler: 3, target: { node: 3, path: 'weights' } },
			{ sampler: 4, target: { node: 0, path: 'translation' } },
		],
	});
	return b.finish();
}

// --------------------------------------------------------------- fixture: accessors

// componentSamples returns four elements' worth of components for a given
// component type, exercising the extremes of its range.
function componentSamples(componentType, comps) {
	const ranges = {
		5120: [-128, -1, 0, 127],
		5121: [0, 1, 128, 255],
		5122: [-32768, -1, 0, 32767],
		5123: [0, 1, 4096, 65535],
		5125: [0, 1, 70000, 4294967295],
		5126: [-1.5, 0, 0.25, 12345.678],
	}[componentType];
	const out = [];
	for (let i = 0; i < 4; i++) {
		for (let j = 0; j < comps; j++) out.push(ranges[(i + j) % ranges.length]);
	}
	return out;
}

function accessors() {
	const b = new Builder();
	const compTypes = [5120, 5121, 5122, 5123, 5125, 5126];
	const types = ['SCALAR', 'VEC2', 'VEC3', 'VEC4'];

	// Every componentType x type combination, unnormalized.
	for (const ct of compTypes) {
		for (const type of types) {
			const values = componentSamples(ct, TYPE_COMPS[type]);
			b.packed({ componentType: ct, type, values, name: `${ct}_${type}` });
		}
	}
	// Matrix types (FLOAT only: byte-sized matrix columns need spec padding).
	for (const type of ['MAT2', 'MAT3', 'MAT4']) {
		const values = componentSamples(5126, TYPE_COMPS[type]);
		b.packed({ componentType: 5126, type, values, name: `5126_${type}` });
	}
	// Normalized integer accessors.
	for (const ct of [5120, 5121, 5122, 5123]) {
		const values = componentSamples(ct, 4);
		b.packed({ componentType: ct, type: 'VEC4', values, normalized: true, name: `${ct}_VEC4_norm` });
	}

	// Sparse accessor: a dense FLOAT VEC3 base with two elements replaced.
	const baseValues = [
		0, 0, 0, 1, 1, 1, 2, 2, 2, 3, 3, 3, 4, 4, 4, 5, 5, 5,
	];
	const baseView = b.addView(Builder.data(5126, baseValues));
	const sparseIdxView = b.addView(Builder.data(5123, [1, 4]));
	const sparseValView = b.addView(Builder.data(5126, [-1, -2, -3, -4, -5, -6]));
	const effective = baseValues.slice();
	effective.splice(1 * 3, 3, -1, -2, -3);
	effective.splice(4 * 3, 3, -4, -5, -6);
	const sparseAcc = b.addAccessor({
		bufferView: baseView, componentType: 5126, type: 'VEC3', count: 6,
		values: effective, name: 'sparse',
	});
	b.json.accessors[sparseAcc].sparse = {
		count: 2,
		indices: { bufferView: sparseIdxView, componentType: 5123 },
		values: { bufferView: sparseValView },
	};

	// Interleaved bufferView: POSITION (VEC3) and TEXCOORD_0 (VEC2) share one
	// 20-byte stride.
	const interleaved = Buffer.alloc(20 * 4);
	const ipos = [];
	const iuv = [];
	for (let i = 0; i < 4; i++) {
		const p = [i, i * 0.5, -i];
		const t = [i / 4, 1 - i / 4];
		ipos.push(...p);
		iuv.push(...t);
		interleaved.writeFloatLE(p[0], i * 20 + 0);
		interleaved.writeFloatLE(p[1], i * 20 + 4);
		interleaved.writeFloatLE(p[2], i * 20 + 8);
		interleaved.writeFloatLE(t[0], i * 20 + 12);
		interleaved.writeFloatLE(t[1], i * 20 + 16);
	}
	const iview = b.addView(interleaved, { target: ARRAY_BUFFER, byteStride: 20 });
	const iposAcc = b.addAccessor({
		bufferView: iview, componentType: 5126, type: 'VEC3', count: 4,
		values: ipos, name: 'interleavedPOSITION',
	});
	const iuvAcc = b.addAccessor({
		bufferView: iview, byteOffset: 12, componentType: 5126, type: 'VEC2', count: 4,
		values: iuv, name: 'interleavedTEXCOORD_0',
	});
	const idx = b.packed({
		componentType: 5121, type: 'SCALAR', values: [0, 1, 2, 0, 2, 3],
		target: ELEMENT_ARRAY_BUFFER, name: 'indices',
	});

	b.json.meshes.push({
		name: 'interleaved',
		primitives: [{ attributes: { POSITION: iposAcc, TEXCOORD_0: iuvAcc }, indices: idx }],
	});
	b.json.nodes.push({ name: 'meshNode', mesh: 0 });
	b.json.scenes.push({ name: 'main', nodes: [0] });
	return b.finish();
}

// ---------------------------------------------------------------- malformed set

function malformed() {
	mkdirSync(join(HERE, 'malformed'), { recursive: true });
	const out = (name, bytes) => writeFileSync(join(HERE, 'malformed', name), bytes);
	const tri = triangle();
	// embedded copy of the triangle, used as the base for JSON mutations
	const base = () => {
		const j = JSON.parse(JSON.stringify(tri.json));
		j.buffers[0].uri = 'data:application/octet-stream;base64,' + tri.bin.toString('base64');
		return j;
	};
	const gltf = (name, mutate) => {
		const j = base();
		mutate(j);
		out(name, JSON.stringify(j, null, 2) + '\n');
	};

	// Container-level damage.
	out('bad-json.gltf', '{"asset":{"version":"2.0"},,}\n');
	out('empty.gltf', '\n');
	out('bad-magic.glb', buildGLB(base(), tri.bin, { magic: 0x12345678 }));
	out('bad-glb-version.glb', buildGLB(base(), tri.bin, { version: 1 }));
	out('truncated.glb', buildGLB(base(), tri.bin, { truncate: 16 }));

	// Structural damage.
	gltf('no-asset.gltf', (j) => { delete j.asset; });
	gltf('no-asset-version.gltf', (j) => { delete j.asset.version; });
	gltf('bad-accessor-index.gltf', (j) => { j.meshes[0].primitives[0].attributes.POSITION = 99; });
	gltf('bad-node-child-index.gltf', (j) => { j.nodes[0].children = [42]; });
	gltf('bad-component-type.gltf', (j) => { j.accessors[0].componentType = 1234; });
	gltf('bad-accessor-type.gltf', (j) => { j.accessors[0].type = 'VEC7'; });
	gltf('accessor-too-long.gltf', (j) => { j.accessors[0].count = 4096; });
	gltf('bad-primitive-mode.gltf', (j) => { j.meshes[0].primitives[0].mode = 9; });
	gltf('node-matrix-and-trs.gltf', (j) => {
		j.nodes[0].matrix = [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1];
		j.nodes[0].translation = [1, 2, 3];
	});
	gltf('min-mismatch.gltf', (j) => { j.accessors[0].min = [-99, -99, -99]; });
	gltf('camera-missing-projection.gltf', (j) => {
		j.cameras = [{ type: 'perspective' }];
		j.nodes.push({ camera: 0 });
		j.scenes[0].nodes.push(1);
	});
	gltf('bad-buffer-view-range.gltf', (j) => { j.bufferViews[0].byteLength = 1 << 20; });
	gltf('skin-bad-joint.gltf', (j) => {
		j.skins = [{ joints: [7] }];
		j.nodes[0].skin = 0;
	});
	gltf('animation-bad-sampler.gltf', (j) => {
		j.animations = [{
			samplers: [{ input: 0, output: 0 }],
			channels: [{ sampler: 5, target: { node: 0, path: 'translation' } }],
		}];
	});
	gltf('bad-alpha-mode.gltf', (j) => { j.materials[0].alphaMode = 'GHOST'; });
	gltf('unresolved-material.gltf', (j) => { j.meshes[0].primitives[0].material = 3; });
	gltf('bad-scene-index.gltf', (j) => { j.scene = 5; });
}

// ------------------------------------------------------------------------ main

const tri = triangle();
writeGLTFEmbedded('triangle.gltf', tri);
writeGLB('triangle.glb', tri);
writeGLTFEmbedded('scene.gltf', scene());
writeGLB('animated.glb', animated());
writeGLB('accessors.glb', accessors());
malformed();
console.error('fixtures written to', HERE);
