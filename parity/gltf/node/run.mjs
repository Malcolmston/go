// Upstream (oracle) runner for the gltf parity harness.
//
// Reads one JSON request per line on stdin, writes exactly one JSON reply per
// line on stdout. Never exits on a failing case: every throw is reported as
// {ok:false,error}.
//
// Oracles:
//   @gltf-transform/core       reference reader/writer for glTF 2.0 / GLB
//   @gltf-transform/extensions the ratified KHR_* extension classes
//   gltf-validator             the official Khronos conformance validator
//
// Environment:
//   PARITY_FIXTURES  directory holding the committed fixture assets
//   PARITY_OUT       shared scratch directory for cross-implementation writes
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';
import { NodeIO } from '@gltf-transform/core';
import { ALL_EXTENSIONS } from '@gltf-transform/extensions';
import validator from 'gltf-validator';

const FIXTURES = process.env.PARITY_FIXTURES || '../fixtures';
const OUT = process.env.PARITY_OUT || '.';
const LANG = 'node';
const OTHER = 'go';

const io = new NodeIO().registerExtensions(ALL_EXTENSIONS);

// ------------------------------------------------------------------- helpers

const fixture = (rel) => join(FIXTURES, rel);
const outPath = (rel, lang) => join(OUT, basename(rel).replace(/[^A-Za-z0-9._-]/g, '_') + '.' + lang + '.glb');

const sha256 = (bytes) => createHash('sha256').update(bytes).digest('hex');

// num coerces a value into a plain JSON number, mapping "absent" to the
// sentinel the Go runner also uses.
const ABSENT = -1;

async function readDoc(path) {
	if (path.endsWith('.glb')) return io.readBinary(new Uint8Array(readFileSync(path)));
	return io.read(path);
}

// indexer builds an object->index map from a gltf-transform list, so the
// canonical description can talk about indices like the glTF JSON does.
function indexer(list) {
	const m = new Map();
	list.forEach((o, i) => m.set(o, i));
	return (o) => (o && m.has(o) ? m.get(o) : ABSENT);
}

const MODE_NAMES = ['POINTS', 'LINES', 'LINE_LOOP', 'LINE_STRIP', 'TRIANGLES', 'TRIANGLE_STRIP', 'TRIANGLE_FAN'];

// ---------------------------------------------------------- canonical describe

// describe renders a document into the implementation-independent structure the
// harness compares. Every glTF default is resolved explicitly so the two sides
// describe semantics rather than JSON shape.
function describe(doc) {
	const root = doc.getRoot();
	const nodes = root.listNodes();
	const meshes = root.listMeshes();
	const accessors = root.listAccessors();
	const materials = root.listMaterials();
	const textures = root.listTextures();
	const cameras = root.listCameras();
	const skins = root.listSkins();
	const nodeIdx = indexer(nodes);
	const meshIdx = indexer(meshes);
	const accIdx = indexer(accessors);
	const matIdx = indexer(materials);
	const texIdx = indexer(textures);
	const camIdx = indexer(cameras);
	const skinIdx = indexer(skins);
	const sceneIdx = indexer(root.listScenes());

	return {
		asset: { version: root.getAsset().version },
		extensionsUsed: root.listExtensionsUsed().map((e) => e.extensionName).sort(),
		extensionsRequired: root.listExtensionsRequired().map((e) => e.extensionName).sort(),
		defaultScene: sceneIdx(root.getDefaultScene()),
		scenes: root.listScenes().map((s) => ({
			name: s.getName(),
			nodes: s.listChildren().map(nodeIdx),
		})),
		nodes: nodes.map((n) => ({
			name: n.getName(),
			matrix: Array.from(n.getMatrix()),
			children: n.listChildren().map(nodeIdx),
			mesh: meshIdx(n.getMesh()),
			camera: camIdx(n.getCamera()),
			skin: skinIdx(n.getSkin()),
			weights: n.getWeights() ? Array.from(n.getWeights()) : [],
			light: describeLight(n.getExtension('KHR_lights_punctual')),
		})),
		meshes: meshes.map((m) => ({
			name: m.getName(),
			weights: m.getWeights() ? Array.from(m.getWeights()) : [],
			primitives: m.listPrimitives().map((p) => ({
				mode: MODE_NAMES[p.getMode()] || String(p.getMode()),
				attributes: p.listSemantics()
					.map((s) => [s, accIdx(p.getAttribute(s))])
					.sort((a, b) => (a[0] < b[0] ? -1 : 1)),
				indices: accIdx(p.getIndices()),
				material: matIdx(p.getMaterial()),
				targets: p.listTargets().map((t) => t.listSemantics()
					.map((s) => [s, accIdx(t.getAttribute(s))])
					.sort((a, b) => (a[0] < b[0] ? -1 : 1))),
			})),
		})),
		accessors: accessors.map((a) => ({
			componentType: a.getComponentType(),
			type: a.getType(),
			count: a.getCount(),
			normalized: a.getNormalized(),
			elementSize: a.getElementSize(),
			// Normalized bounds: @gltf-transform only writes min/max for
			// POSITION and animation inputs, so declared bounds cannot be
			// compared after a round-trip. Both sides therefore report the
			// bounds of the *decoded* values.
			min: a.getMinNormalized(new Array(a.getElementSize()).fill(0)),
			max: a.getMaxNormalized(new Array(a.getElementSize()).fill(0)),
		})),
		materials: materials.map((m) => describeMaterial(m, texIdx)),
		textures: textures.map((t) => {
			const image = t.getImage();
			const size = t.getSize();
			return {
				mimeType: t.getMimeType(),
				sha256: image ? sha256(image) : '',
				width: size ? size[0] : ABSENT,
				height: size ? size[1] : ABSENT,
			};
		}),
		animations: root.listAnimations().map((a) => {
			const smpIdx = indexer(a.listSamplers());
			return {
				name: a.getName(),
				channels: a.listChannels().map((c) => ({
					node: nodeIdx(c.getTargetNode()),
					path: c.getTargetPath(),
					sampler: smpIdx(c.getSampler()),
				})),
				samplers: a.listSamplers().map((s) => ({
					input: accIdx(s.getInput()),
					output: accIdx(s.getOutput()),
					interpolation: s.getInterpolation(),
				})),
			};
		}),
		skins: skins.map((s) => ({
			name: s.getName(),
			joints: s.listJoints().map(nodeIdx),
			skeleton: nodeIdx(s.getSkeleton()),
			inverseBindMatrices: accIdx(s.getInverseBindMatrices()),
		})),
		cameras: cameras.map((c) => ({
			name: c.getName(),
			type: c.getType(),
			znear: c.getZNear(),
			zfar: c.getZFar() ?? ABSENT,
			yfov: c.getType() === 'perspective' ? c.getYFov() : ABSENT,
			aspectRatio: c.getType() === 'perspective' ? (c.getAspectRatio() ?? ABSENT) : ABSENT,
			xmag: c.getType() === 'orthographic' ? c.getXMag() : ABSENT,
			ymag: c.getType() === 'orthographic' ? c.getYMag() : ABSENT,
		})),
	};
}

function describeLight(light) {
	if (!light) return null;
	return {
		type: light.getType(),
		color: Array.from(light.getColor()),
		intensity: light.getIntensity(),
		range: light.getRange() ?? ABSENT,
		innerConeAngle: light.getInnerConeAngle(),
		outerConeAngle: light.getOuterConeAngle(),
	};
}

function describeTextureInfo(texture, info, texIdx, extra) {
	if (!texture) return null;
	const transform = info ? info.getExtension('KHR_texture_transform') : null;
	return {
		texture: texIdx(texture),
		texCoord: info ? info.getTexCoord() : 0,
		magFilter: (info && info.getMagFilter()) ?? ABSENT,
		minFilter: (info && info.getMinFilter()) ?? ABSENT,
		wrapS: (info && info.getWrapS()) ?? ABSENT,
		wrapT: (info && info.getWrapT()) ?? ABSENT,
		transform: transform
			? {
				offset: Array.from(transform.getOffset()),
				rotation: transform.getRotation(),
				scale: Array.from(transform.getScale()),
				texCoord: transform.getTexCoord() ?? ABSENT,
			}
			: null,
		...extra,
	};
}

function describeMaterial(m, texIdx) {
	const unlit = m.getExtension('KHR_materials_unlit');
	const emissive = m.getExtension('KHR_materials_emissive_strength');
	const ior = m.getExtension('KHR_materials_ior');
	const transmission = m.getExtension('KHR_materials_transmission');
	const clearcoat = m.getExtension('KHR_materials_clearcoat');
	const sheen = m.getExtension('KHR_materials_sheen');
	const specular = m.getExtension('KHR_materials_specular');
	const volume = m.getExtension('KHR_materials_volume');
	return {
		name: m.getName(),
		alphaMode: m.getAlphaMode(),
		alphaCutoff: m.getAlphaCutoff(),
		doubleSided: m.getDoubleSided(),
		baseColorFactor: Array.from(m.getBaseColorFactor()),
		metallicFactor: m.getMetallicFactor(),
		roughnessFactor: m.getRoughnessFactor(),
		emissiveFactor: Array.from(m.getEmissiveFactor()),
		baseColorTexture: describeTextureInfo(m.getBaseColorTexture(), m.getBaseColorTextureInfo(), texIdx),
		metallicRoughnessTexture: describeTextureInfo(m.getMetallicRoughnessTexture(), m.getMetallicRoughnessTextureInfo(), texIdx),
		emissiveTexture: describeTextureInfo(m.getEmissiveTexture(), m.getEmissiveTextureInfo(), texIdx),
		normalTexture: describeTextureInfo(m.getNormalTexture(), m.getNormalTextureInfo(), texIdx, { scale: m.getNormalScale() }),
		occlusionTexture: describeTextureInfo(m.getOcclusionTexture(), m.getOcclusionTextureInfo(), texIdx, { strength: m.getOcclusionStrength() }),
		ext: {
			unlit: !!unlit,
			emissiveStrength: emissive ? emissive.getEmissiveStrength() : 1,
			ior: ior ? ior.getIOR() : 1.5,
			transmissionFactor: transmission ? transmission.getTransmissionFactor() : 0,
			clearcoatFactor: clearcoat ? clearcoat.getClearcoatFactor() : 0,
			clearcoatRoughnessFactor: clearcoat ? clearcoat.getClearcoatRoughnessFactor() : 0,
			sheenColorFactor: sheen ? Array.from(sheen.getSheenColorFactor()) : [0, 0, 0],
			sheenRoughnessFactor: sheen ? sheen.getSheenRoughnessFactor() : 0,
			specularFactor: specular ? specular.getSpecularFactor() : 1,
			specularColorFactor: specular ? Array.from(specular.getSpecularColorFactor()) : [1, 1, 1],
			volumeThicknessFactor: volume ? volume.getThicknessFactor() : 0,
			volumeAttenuationDistance: volume ? (volume.getAttenuationDistance() ?? ABSENT) : ABSENT,
			volumeAttenuationColor: volume ? Array.from(volume.getAttenuationColor()) : [1, 1, 1],
		},
	};
}

// sortKeys returns a structurally identical value with every object key sorted,
// so the emitted JSON is byte-stable.
function sortKeys(v) {
	if (Array.isArray(v)) return v.map(sortKeys);
	if (v && typeof v === 'object') {
		const out = {};
		for (const k of Object.keys(v).sort()) out[k] = sortKeys(v[k]);
		return out;
	}
	if (typeof v === 'number' && !Number.isFinite(v)) return String(v);
	return v;
}

// ------------------------------------------------------------------ operations

// decodeAccessor returns every component of one accessor as plain numbers,
// applying the glTF normalization rule for normalized integer accessors.
function decodeAccessor(doc, index) {
	const acc = doc.getRoot().listAccessors()[index];
	if (!acc) throw new Error(`no accessor at index ${index}`);
	const size = acc.getElementSize();
	const out = [];
	const el = new Array(size).fill(0);
	for (let i = 0; i < acc.getCount(); i++) {
		acc.getElement(i, el);
		for (let j = 0; j < size; j++) out.push(el[j]);
	}
	return out;
}

function validateFile(path) {
	const bytes = new Uint8Array(readFileSync(path));
	return validator.validateBytes(bytes, {
		uri: basename(path),
		externalResourceFunction: () => Promise.reject(new Error('external resources are not used by the fixtures')),
	});
}

const OPS = {
	// describe: parse a committed fixture and emit the canonical description.
	async describe([rel]) {
		return sortKeys(describe(await readDoc(fixture(rel))));
	},

	// xwrite: read a fixture and write it back out as a GLB under PARITY_OUT,
	// tagged with this implementation's name. The counterpart runner writes the
	// same fixture with its own tag; xread then crosses them over.
	async xwrite([rel]) {
		const doc = await readDoc(fixture(rel));
		const glb = await io.writeBinary(doc);
		writeFileSync(outPath(rel, LANG), Buffer.from(glb));
		return { wrote: true, format: 'glb' };
	},

	// xread: describe both the GLB this implementation wrote and the one the
	// other implementation wrote. Equal values across the two runners prove
	// each side reads what the other side writes.
	async xread([rel]) {
		const self = sortKeys(describe(await readDoc(outPath(rel, LANG))));
		const cross = sortKeys(describe(await readDoc(outPath(rel, OTHER))));
		return {
			self,
			cross,
			selfEqualsCross: JSON.stringify(self) === JSON.stringify(cross),
		};
	},

	// decodeAccessor: numeric decode of one accessor of a fixture.
	async decodeAccessor([rel, index]) {
		return { values: decodeAccessor(await readDoc(fixture(rel)), index) };
	},

	// accessorCount: how many accessors the document exposes.
	async accessorCount([rel]) {
		return { count: (await readDoc(fixture(rel))).getRoot().listAccessors().length };
	},

	// validate: does this implementation accept the asset? The oracle is
	// gltf-validator; only severity-0 issues (errors) count as rejection.
	async validate([rel]) {
		let report;
		try {
			report = await validateFile(fixture(rel));
		} catch (err) {
			// A validator that refuses to even parse the asset is a rejection.
			return { value: { valid: false }, note: 'validator threw: ' + (err && err.message ? err.message : String(err)) };
		}
		const note = report.issues.messages
			.filter((m) => m.severity === 0)
			.map((m) => `${m.code} ${m.pointer || ''}`)
			.join('; ');
		return { value: { valid: report.issues.numErrors === 0 }, note };
	},

	// parses: can this implementation parse the asset at all?
	async parses([rel]) {
		await readDoc(fixture(rel));
		return { parses: true };
	},

	// validateOut: run the official validator over a file under PARITY_OUT,
	// returning its full issue list. Used to grade the Go port's own output.
	async validateOut([name]) {
		const report = await validateFile(join(OUT, name));
		return {
			numErrors: report.issues.numErrors,
			numWarnings: report.issues.numWarnings,
			numInfos: report.issues.numInfos,
			messages: report.issues.messages.map((m) => ({
				severity: m.severity,
				code: m.code,
				pointer: m.pointer || '',
				message: m.message,
			})),
		};
	},
};

// ---------------------------------------------------------------- main loop

let buffered = '';
process.stdin.setEncoding('utf8');
const queue = [];
let draining = false;

async function drain() {
	if (draining) return;
	draining = true;
	while (queue.length) {
		const req = queue.shift();
		let out;
		try {
			const op = OPS[req.fn];
			if (!op) throw new Error(`unknown fn ${req.fn}`);
			const result = await op(req.args || []);
			// An operation may return {value, note}: `note` is diagnostic text
			// the harness logs but never compares.
			out = (result && typeof result === 'object' && 'value' in result && 'note' in result)
				? { id: req.id, ok: true, value: result.value, note: result.note }
				: { id: req.id, ok: true, value: result };
		} catch (err) {
			out = { id: req.id, ok: false, error: err && err.message ? err.message : String(err) };
		}
		process.stdout.write(JSON.stringify(out) + '\n');
	}
	draining = false;
}

process.stdin.on('data', (chunk) => {
	buffered += chunk;
	let nl;
	while ((nl = buffered.indexOf('\n')) >= 0) {
		const line = buffered.slice(0, nl).trim();
		buffered = buffered.slice(nl + 1);
		if (!line) continue;
		try {
			queue.push(JSON.parse(line));
		} catch (err) {
			process.stdout.write(JSON.stringify({ id: '?', ok: false, error: 'bad request: ' + err.message }) + '\n');
		}
	}
	void drain();
});
