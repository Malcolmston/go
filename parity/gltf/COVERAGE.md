# glTF parity coverage

`github.com/malcolmston/gltf` ported to Go against the glTF 2.0 specification.

## Why these oracles

`KhronosGroup/glTF` is a **specification plus a JSON schema**, not a runnable
library, so it cannot be driven as a subprocess. This harness therefore uses two
concrete oracles from the ecosystem the specification's own tooling lives in
(`node`):

| oracle | pinned version | role |
| --- | --- | --- |
| [`@gltf-transform/core`](https://www.npmjs.com/package/@gltf-transform/core) | `4.2.1` | reference reader/writer — the most widely used independent glTF 2.0 implementation in JS, and the only one exposing a full read **and** write path for both `.gltf` and `.glb` |
| [`@gltf-transform/extensions`](https://www.npmjs.com/package/@gltf-transform/extensions) | `4.2.1` | typed classes for the ratified `KHR_*`/`EXT_*` extensions, so extension data is compared semantically rather than as raw JSON |
| [`gltf-validator`](https://www.npmjs.com/package/gltf-validator) | `2.0.0-dev.3.10` | the **official Khronos validator** (`KhronosGroup/glTF-Validator`), used as the conformance oracle: it decides whether an asset is spec-legal, including assets the Go port itself produced |

Exact pins live in `node/package.json` (all three are pinned to exact versions,
no ranges) and are re-asserted at test start by `assertUpstreamVersions`, so a
score can never be attributed to the wrong oracle.

Go module under test: `github.com/malcolmston/gltf v0.0.0-20260810111530-84677d83f157`
(resolved by `GOWORK=off go get github.com/malcolmston/gltf@latest`).

## Fixtures

No sample models are downloaded. `fixtures/gen.mjs` is a committed,
dependency-free Node script (built-ins only — it hand-writes both the glTF JSON
and the GLB container, so the fixtures are not biased toward either
implementation) that reproducibly regenerates every asset:

```
node fixtures/gen.mjs
```

| fixture | what it exercises |
| --- | --- |
| `triangle.gltf` | minimal asset, buffer as a base64 data URI |
| `triangle.glb` | the same asset in the GLB container (BIN chunk) |
| `scene.gltf` | node hierarchy (TRS parent + matrix child), 3 primitives with 3 modes, full PBR material, all five texture slots, sampler filters/wrap modes, an embedded 2×2 PNG, perspective + orthographic cameras, a punctual spot light, and nine KHR extensions |
| `animated.glb` | 5 animation channels (LINEAR/STEP/CUBICSPLINE + morph weights), a skin with MAT4 inverse bind matrices, JOINTS_0/WEIGHTS_0, and a morph target |
| `accessors.glb` | 35 accessors: every componentType × SCALAR/VEC2/VEC3/VEC4, MAT2/MAT3/MAT4, four normalized integer accessors, a sparse accessor, and two accessors interleaved in one `byteStride: 20` bufferView |
| `malformed/*` (22 files) | container damage (bad magic, GLB version 1, truncation, unparseable JSON, empty file) and structural damage (dangling indices, unknown enums, out-of-range accessors, `matrix` + TRS together, declared min mismatch, camera without a projection block, …) |

All five well-formed fixtures pass `gltf-validator` with **zero errors**
(`scene.gltf` carries two design-intent warnings: `MULTIPLE_EXTENSIONS` for
`KHR_materials_unlit` beside `KHR_materials_emissive_strength`, and
`MESH_PRIMITIVE_GENERATED_TANGENT_SPACE`).

## Case groups

| group | cases | what it compares |
| --- | --- | --- |
| `roundtrip` | 10 | both sides parse each fixture and emit one **canonical description** — hierarchy and composed local transforms, primitive modes and attribute names, accessor componentType/type/count/normalized/bounds, material and PBR factors with texture references and sampler state, animation channels and samplers, skins, cameras, images (sha-256 + size), and the extensions used — which is then deep-compared |
| `crosswrite` | 10 | the true interop test: each side **writes a GLB** and then describes both its own output and the output of the other implementation. A case passes only when node's reading of Go's GLB equals Go's reading of node's GLB *and* each runner reports its own and the foreign GLB as the same asset (`selfEqualsCross`) |
| `validation` | 33 | accept/reject conformance. `gltf-validator` (severity-0 errors) versus `gltf.Open`/`OpenGLB` + `Document.Validate`, over the five good fixtures, all 22 malformed assets, and six parser-level cases |
| `accessors` | 48 | numeric decode of every accessor in `accessors.glb` (all component types, matrix types, normalized integers, sparse, interleaved), plus the scene and animation accessors, compared component-by-component |

Comparison rules: booleans and strings must be exactly equal; numbers are equal
when `|a-b| <= 1e-6 + 1e-6*max(|a|,|b|)` (both sides funnel glTF data through
float32 storage, and @gltf-transform composes node matrices in float32, so
bit-equality is not achievable). Binary payloads are never embedded in JSON:
image data is compared as a lowercase-hex sha-256 digest, and buffer contents as
decoded numeric arrays. Object keys are sorted on both sides; a rejection reason
travels in a `note` field the harness logs but never compares.

## Result

`GOWORK=off go test ./parity/gltf/` — **98 of 101 compared cases match (97.03 %)**,
0 deviations. Per-group scores and the full validator report live in
`parity.json`, rewritten by every complete run.

| group | cases | match | mismatch |
| --- | --- | --- | --- |
| `roundtrip` | 10 | 10 | 0 |
| `crosswrite` | 10 | 10 | 0 |
| `accessors` | 48 | 48 | 0 |
| `validation` | 33 | 30 | 3 |

### The three divergences

1. **`validate-bad-min-mismatch-gltf`** — an accessor whose declared `min` is
   `[-99,-99,-99]` while its data starts at `[0,0,0]`. `gltf-validator` rejects
   it (`ACCESSOR_MIN_MISMATCH`); `Document.Validate` accepts it. The port reads
   `min`/`max` but never checks them against the buffer.
2. **`validate-bad-bad-alpha-mode-gltf`** — `alphaMode: "GHOST"`.
   `Document.Validate` rejects it; `gltf-validator` only warns. Here the port is
   *stricter* than the official validator.
3. **`parses-bad-no-asset-gltf`** — a document with no `asset` object at all.
   `gltf.Open` parses it happily; `@gltf-transform` throws. `Document.Validate`
   does reject it, so the divergence is purely parser-level leniency.

### Official validator on port-produced assets

Every fixture was re-encoded by the Go port as both `.glb` and `.gltf` (10 files)
and graded by `gltf-validator`:

**zero errors on all 10 files.** The only warnings are inherited from the
fixtures themselves (`MULTIPLE_EXTENSIONS`,
`MESH_PRIMITIVE_GENERATED_TANGENT_SPACE`) plus one that is specific to the port's
write path: `DATA_URI_GLB /images/0/uri` — when converting a `.gltf` whose image
is a base64 data URI into a GLB, the port keeps the data URI instead of moving
the image into the BIN chunk. That is legal but non-idiomatic;
`@gltf-transform` relocates such images into a bufferView. Full per-file issue
lists are in `parity.json` under `validatorOnGoOutput`.

## How the inventories below were derived

Mechanically, never from memory or a README:

```bash
# glTF 2.0 object types and their properties: reflect over each gltf struct and
# print its `json:"…"` tags (throwaway program in a scratch module that requires
# the pinned gltf version):
#   t := reflect.TypeOf(gltf.Document{})
#   for i := range t.NumField() { fmt.Println(t.Field(i).Tag.Get("json")) }
# repeated for Asset, Scene, Node, Mesh, Primitive, Accessor, Sparse,
# BufferView, Buffer, Material, PBRMetallicRoughness, TextureInfo,
# NormalTexture, OcclusionTexture, Texture, Image, Sampler, Animation,
# AnimationChannel, AnimationChannelTarget, AnimationSampler, Skin, Camera,
# CameraPerspective, CameraOrthographic.

# the upstream API surface actually exercised
node -e "…Object.getOwnPropertyNames(core.<Class>.prototype)
          .filter(n => /^(get|list|is)[A-Z]/.test(n))…"   # per class, minus
                                                          # getDefaults/getExtras/getName

# the extension sets
node -e "console.log(require('gltf-validator').supportedExtensions())"   # Khronos-ratified
node -e "import('@gltf-transform/extensions').then(e =>
           console.log(e.ALL_EXTENSIONS.map(x => x.EXTENSION_NAME)))"    # oracle-modelled
GOWORK=off go doc github.com/malcolmston/gltf | grep '^\s*Ext'           # port-modelled
```

The property list is cross-checked against `@gltf-transform`'s `Root.list*`
accessors: the two agree on all 17 top-level collections plus `asset` and
`scene`, i.e. the port's `Document` covers the whole glTF 2.0 root object.

## Spec surface: glTF 2.0 objects and properties

Statuses: `match` = compared and agreeing, `differs` = compared and diverging,
`missing` = in the spec but not in the port, `extra` = Go-only, `untested` = no
case exercises it. `↑` repeats the object above.

| glTF 2.0 object | property | Go field | status | cases | note |
| --- | --- | --- | --- | --- | --- |
| `glTF (root)` | `extensionsUsed` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `extensionsRequired` | `gltf.Document` | untested | — | no fixture declares a required extension |
| ↑ | `asset` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `scene` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `scenes` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `nodes` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `meshes` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `accessors` | `gltf.Document` | match | `describe-*`, `decode-*` |  |
| ↑ | `bufferViews` | `gltf.Document` | match | `decode-accessors-32`, `decode-accessors-33` | compared through decoded values (incl. byteStride 20 interleaving) |
| ↑ | `buffers` | `gltf.Document` | match | `describe-*`, `xread-*` | GLB BIN chunk and base64 data URI both exercised |
| ↑ | `materials` | `gltf.Document` | match | `describe-*`, `xread-*` |  |
| ↑ | `textures` | `gltf.Document` | match | `describe-scene-gltf` |  |
| ↑ | `images` | `gltf.Document` | match | `describe-scene-gltf`, `xread-scene-gltf` | data URI on read; bufferView-backed after @gltf-transform writes GLB |
| ↑ | `samplers` | `gltf.Document` | match | `describe-scene-gltf` |  |
| ↑ | `animations` | `gltf.Document` | match | `describe-animated-glb` |  |
| ↑ | `skins` | `gltf.Document` | match | `describe-animated-glb` |  |
| ↑ | `cameras` | `gltf.Document` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.Document` | match | `describe-scene-gltf` | document-level `KHR_lights_punctual` |
| ↑ | `extras` | `gltf.Document` | untested | — | not described by either runner |
| `asset` | `copyright` | `gltf.Asset` | untested | — |  |
| ↑ | `generator` | `gltf.Asset` | untested | — | each writer stamps its own; deliberately not compared |
| ↑ | `version` | `gltf.Asset` | differs | `describe-*`, `parses-bad-no-asset-gltf` | values agree, but Go's parser accepts a document with no `asset` object where @gltf-transform throws (Validate does reject it) |
| ↑ | `minVersion` | `gltf.Asset` | untested | — |  |
| ↑ | `extensions` | `gltf.Asset` | untested | — |  |
| ↑ | `extras` | `gltf.Asset` | untested | — |  |
| `scene` | `name` | `gltf.Scene` | match | `describe-*`, `xread-*` |  |
| ↑ | `nodes` | `gltf.Scene` | match | `describe-*`, `xread-*` |  |
| ↑ | `extensions` | `gltf.Scene` | untested | — |  |
| ↑ | `extras` | `gltf.Scene` | untested | — |  |
| `node` | `name` | `gltf.Node` | match | `describe-*`, `xread-*` |  |
| ↑ | `camera` | `gltf.Node` | match | `describe-scene-gltf` |  |
| ↑ | `children` | `gltf.Node` | match | `describe-*`, `xread-*` |  |
| ↑ | `skin` | `gltf.Node` | match | `describe-animated-glb` |  |
| ↑ | `matrix` | `gltf.Node` | match | `describe-scene-gltf` | compared as the composed local matrix |
| ↑ | `mesh` | `gltf.Node` | match | `describe-*`, `xread-*` |  |
| ↑ | `rotation` | `gltf.Node` | match | `describe-scene-gltf` | @gltf-transform decomposes `matrix` into TRS, so TRS and matrix are compared through the composed local matrix only |
| ↑ | `scale` | `gltf.Node` | match | `describe-scene-gltf` | as above |
| ↑ | `translation` | `gltf.Node` | match | `describe-scene-gltf` | as above |
| ↑ | `weights` | `gltf.Node` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.Node` | match | `describe-scene-gltf` | node-level `KHR_lights_punctual` |
| ↑ | `extras` | `gltf.Node` | untested | — |  |
| `mesh` | `name` | `gltf.Mesh` | match | `describe-*`, `xread-*` |  |
| ↑ | `primitives` | `gltf.Mesh` | match | `describe-*`, `xread-*` |  |
| ↑ | `weights` | `gltf.Mesh` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.Mesh` | untested | — |  |
| ↑ | `extras` | `gltf.Mesh` | untested | — |  |
| `mesh.primitive` | `attributes` | `gltf.Primitive` | match | `describe-*`, `xread-*` | POSITION/NORMAL/TEXCOORD_0/COLOR_0/JOINTS_0/WEIGHTS_0 |
| ↑ | `indices` | `gltf.Primitive` | match | `describe-*`, `xread-*` |  |
| ↑ | `material` | `gltf.Primitive` | match | `describe-*`, `xread-*` |  |
| ↑ | `mode` | `gltf.Primitive` | match | `describe-scene-gltf` | POINTS, LINES, TRIANGLES and the default |
| ↑ | `targets` | `gltf.Primitive` | match | `describe-animated-glb` | morph target POSITION deltas |
| ↑ | `extensions` | `gltf.Primitive` | untested | — |  |
| ↑ | `extras` | `gltf.Primitive` | untested | — |  |
| `accessor` | `bufferView` | `gltf.Accessor` | match | `decode-*` |  |
| ↑ | `byteOffset` | `gltf.Accessor` | match | `decode-accessors-33` | non-zero offset inside an interleaved view |
| ↑ | `componentType` | `gltf.Accessor` | match | `describe-*`, `decode-accessors-*` | all six component types |
| ↑ | `normalized` | `gltf.Accessor` | match | `decode-accessors-27`..`30`, `decode-scene-03` |  |
| ↑ | `count` | `gltf.Accessor` | match | `describe-*`, `xread-*` |  |
| ↑ | `type` | `gltf.Accessor` | match | `describe-*`, `xread-*` | SCALAR/VEC2/VEC3/VEC4/MAT2/MAT3/MAT4 |
| ↑ | `max` | `gltf.Accessor` | differs | `validate-bad-min-mismatch-gltf` | declared bounds are read but never checked against the data; gltf-validator rejects the same asset (ACCESSOR_MIN_MISMATCH) |
| ↑ | `min` | `gltf.Accessor` | differs | `validate-bad-min-mismatch-gltf` | as above |
| ↑ | `sparse` | `gltf.Accessor` | match | `decode-accessors-31` |  |
| ↑ | `name` | `gltf.Accessor` | untested | — | not described |
| ↑ | `extensions` | `gltf.Accessor` | untested | — |  |
| ↑ | `extras` | `gltf.Accessor` | untested | — |  |
| `accessor.sparse` | `count` | `gltf.Sparse` | match | `decode-accessors-31` |  |
| ↑ | `indices` | `gltf.Sparse` | match | `decode-accessors-31` |  |
| ↑ | `values` | `gltf.Sparse` | match | `decode-accessors-31` |  |
| ↑ | `extensions` | `gltf.Sparse` | untested | — |  |
| ↑ | `extras` | `gltf.Sparse` | untested | — |  |
| `bufferView` | `buffer` | `gltf.BufferView` | match | `decode-*` |  |
| ↑ | `byteOffset` | `gltf.BufferView` | match | `decode-*` |  |
| ↑ | `byteLength` | `gltf.BufferView` | match | `decode-*` |  |
| ↑ | `byteStride` | `gltf.BufferView` | match | `decode-accessors-32`, `decode-accessors-33` |  |
| ↑ | `target` | `gltf.BufferView` | untested | — | @gltf-transform recomputes the target on write, so it cannot be compared |
| ↑ | `name` | `gltf.BufferView` | untested | — |  |
| ↑ | `extensions` | `gltf.BufferView` | untested | — |  |
| ↑ | `extras` | `gltf.BufferView` | untested | — |  |
| `buffer` | `uri` | `gltf.Buffer` | match | `describe-triangle-gltf`, `xread-*` | base64 data URI and GLB BIN chunk |
| ↑ | `byteLength` | `gltf.Buffer` | match | `decode-*` |  |
| ↑ | `name` | `gltf.Buffer` | untested | — |  |
| ↑ | `extensions` | `gltf.Buffer` | untested | — |  |
| ↑ | `extras` | `gltf.Buffer` | untested | — |  |
| `material` | `name` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `pbrMetallicRoughness` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `normalTexture` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `occlusionTexture` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `emissiveTexture` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `emissiveFactor` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `alphaMode` | `gltf.Material` | differs | `describe-scene-gltf`, `validate-bad-bad-alpha-mode-gltf` | values agree; Go's Validate rejects an unknown alphaMode, gltf-validator only warns |
| ↑ | `alphaCutoff` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `doubleSided` | `gltf.Material` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.Material` | match | `describe-scene-gltf` | eight KHR material extensions |
| ↑ | `extras` | `gltf.Material` | untested | — |  |
| `material.pbrMetallicRoughness` | `baseColorFactor` | `gltf.PBRMetallicRoughness` | match | `describe-scene-gltf` |  |
| ↑ | `baseColorTexture` | `gltf.PBRMetallicRoughness` | match | `describe-scene-gltf` |  |
| ↑ | `metallicFactor` | `gltf.PBRMetallicRoughness` | match | `describe-scene-gltf` |  |
| ↑ | `roughnessFactor` | `gltf.PBRMetallicRoughness` | match | `describe-scene-gltf` |  |
| ↑ | `metallicRoughnessTexture` | `gltf.PBRMetallicRoughness` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.PBRMetallicRoughness` | untested | — |  |
| ↑ | `extras` | `gltf.PBRMetallicRoughness` | untested | — |  |
| `textureInfo` | `index` | `gltf.TextureInfo` | match | `describe-scene-gltf` |  |
| ↑ | `texCoord` | `gltf.TextureInfo` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.TextureInfo` | match | `describe-scene-gltf` | `KHR_texture_transform` |
| ↑ | `extras` | `gltf.TextureInfo` | untested | — |  |
| `material.normalTextureInfo` | `index` | `gltf.NormalTexture` | match | `describe-scene-gltf` |  |
| ↑ | `texCoord` | `gltf.NormalTexture` | match | `describe-scene-gltf` |  |
| ↑ | `scale` | `gltf.NormalTexture` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.NormalTexture` | untested | — |  |
| ↑ | `extras` | `gltf.NormalTexture` | untested | — |  |
| `material.occlusionTextureInfo` | `index` | `gltf.OcclusionTexture` | match | `describe-scene-gltf` |  |
| ↑ | `texCoord` | `gltf.OcclusionTexture` | match | `describe-scene-gltf` |  |
| ↑ | `strength` | `gltf.OcclusionTexture` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.OcclusionTexture` | untested | — |  |
| ↑ | `extras` | `gltf.OcclusionTexture` | untested | — |  |
| `texture` | `name` | `gltf.Texture` | untested | — | not described |
| ↑ | `sampler` | `gltf.Texture` | match | `describe-scene-gltf` | compared as the filters/wrap modes on each texture reference, matching @gltf-transform's model |
| ↑ | `source` | `gltf.Texture` | match | `describe-scene-gltf` | compared as the image's sha-256, width and height |
| ↑ | `extensions` | `gltf.Texture` | untested | — |  |
| ↑ | `extras` | `gltf.Texture` | untested | — |  |
| `image` | `name` | `gltf.Image` | untested | — |  |
| ↑ | `uri` | `gltf.Image` | match | `describe-scene-gltf` | base64 PNG data URI |
| ↑ | `mimeType` | `gltf.Image` | match | `describe-scene-gltf` |  |
| ↑ | `bufferView` | `gltf.Image` | match | `xread-scene-gltf` | exercised on the cross read: @gltf-transform moves images into the buffer when writing GLB |
| ↑ | `extensions` | `gltf.Image` | untested | — |  |
| ↑ | `extras` | `gltf.Image` | untested | — |  |
| `sampler` | `name` | `gltf.Sampler` | untested | — |  |
| ↑ | `magFilter` | `gltf.Sampler` | match | `describe-scene-gltf` |  |
| ↑ | `minFilter` | `gltf.Sampler` | match | `describe-scene-gltf` |  |
| ↑ | `wrapS` | `gltf.Sampler` | match | `describe-scene-gltf` |  |
| ↑ | `wrapT` | `gltf.Sampler` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.Sampler` | untested | — |  |
| ↑ | `extras` | `gltf.Sampler` | untested | — |  |
| `animation` | `name` | `gltf.Animation` | match | `describe-animated-glb` |  |
| ↑ | `channels` | `gltf.Animation` | match | `describe-animated-glb` |  |
| ↑ | `samplers` | `gltf.Animation` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.Animation` | untested | — |  |
| ↑ | `extras` | `gltf.Animation` | untested | — |  |
| `animation.channel` | `sampler` | `gltf.AnimationChannel` | match | `describe-animated-glb` |  |
| ↑ | `target` | `gltf.AnimationChannel` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.AnimationChannel` | untested | — |  |
| ↑ | `extras` | `gltf.AnimationChannel` | untested | — |  |
| `animation.channel.target` | `node` | `gltf.AnimationChannelTarget` | match | `describe-animated-glb` |  |
| ↑ | `path` | `gltf.AnimationChannelTarget` | match | `describe-animated-glb` | translation, rotation, scale, weights |
| ↑ | `extensions` | `gltf.AnimationChannelTarget` | untested | — |  |
| ↑ | `extras` | `gltf.AnimationChannelTarget` | untested | — |  |
| `animation.sampler` | `input` | `gltf.AnimationSampler` | match | `describe-animated-glb` |  |
| ↑ | `interpolation` | `gltf.AnimationSampler` | match | `describe-animated-glb` | LINEAR, STEP, CUBICSPLINE and the default |
| ↑ | `output` | `gltf.AnimationSampler` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.AnimationSampler` | untested | — |  |
| ↑ | `extras` | `gltf.AnimationSampler` | untested | — |  |
| `skin` | `name` | `gltf.Skin` | match | `describe-animated-glb` |  |
| ↑ | `inverseBindMatrices` | `gltf.Skin` | match | `describe-animated-glb`, `decode-animated-05` |  |
| ↑ | `skeleton` | `gltf.Skin` | match | `describe-animated-glb` |  |
| ↑ | `joints` | `gltf.Skin` | match | `describe-animated-glb` |  |
| ↑ | `extensions` | `gltf.Skin` | untested | — |  |
| ↑ | `extras` | `gltf.Skin` | untested | — |  |
| `camera` | `name` | `gltf.Camera` | match | `describe-scene-gltf` |  |
| ↑ | `type` | `gltf.Camera` | match | `describe-scene-gltf` |  |
| ↑ | `perspective` | `gltf.Camera` | match | `describe-scene-gltf` |  |
| ↑ | `orthographic` | `gltf.Camera` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.Camera` | untested | — |  |
| ↑ | `extras` | `gltf.Camera` | untested | — |  |
| `camera.perspective` | `aspectRatio` | `gltf.CameraPerspective` | match | `describe-scene-gltf` |  |
| ↑ | `yfov` | `gltf.CameraPerspective` | match | `describe-scene-gltf` |  |
| ↑ | `zfar` | `gltf.CameraPerspective` | match | `describe-scene-gltf` |  |
| ↑ | `znear` | `gltf.CameraPerspective` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.CameraPerspective` | untested | — |  |
| ↑ | `extras` | `gltf.CameraPerspective` | untested | — |  |
| `camera.orthographic` | `xmag` | `gltf.CameraOrthographic` | match | `describe-scene-gltf` |  |
| ↑ | `ymag` | `gltf.CameraOrthographic` | match | `describe-scene-gltf` |  |
| ↑ | `zfar` | `gltf.CameraOrthographic` | match | `describe-scene-gltf` |  |
| ↑ | `znear` | `gltf.CameraOrthographic` | match | `describe-scene-gltf` |  |
| ↑ | `extensions` | `gltf.CameraOrthographic` | untested | — |  |
| ↑ | `extras` | `gltf.CameraOrthographic` | untested | — |  |

**Spec-surface totals** — 175 properties enumerated across the root object and
25 object types: **112 match, 4 differs, 0 missing, 0 extra, 59 untested**.
Parity over the properties actually compared: **112 / 116 = 96.55 %**.

Nothing is `missing`: the port models every property of every glTF 2.0 object.
The untested 59 are dominated by `extras` (26 objects) and per-object
`extensions` containers that no fixture populates, plus a handful of cosmetic
`name` fields that the canonical description deliberately omits.

## Extension surface

The ratified list comes from `gltf-validator.supportedExtensions()` (18 names) —
the official validator is the authority on what Khronos has ratified — unioned
with `@gltf-transform/extensions`' `ALL_EXTENSIONS` (24 names); 25 names in all.

| extension | ratified (validator) | oracle models it | port models it | status | cases |
| --- | --- | --- | --- | --- | --- |
| `KHR_materials_unlit` | yes | yes | `ExtMaterialsUnlit` | match | `describe-scene-gltf` |
| `KHR_materials_emissive_strength` | yes | yes | `ExtMaterialsEmissiveStrength` | match | `describe-scene-gltf` |
| `KHR_materials_ior` | yes | yes | `ExtMaterialsIOR` | match | `describe-scene-gltf` |
| `KHR_materials_transmission` | yes | yes | `ExtMaterialsTransmission` | match | `describe-scene-gltf` |
| `KHR_materials_volume` | yes | yes | `ExtMaterialsVolume` | match | `describe-scene-gltf` |
| `KHR_materials_clearcoat` | yes | yes | `ExtMaterialsClearcoat` | match | `describe-scene-gltf` |
| `KHR_materials_sheen` | yes | yes | `ExtMaterialsSheen` | match | `describe-scene-gltf` |
| `KHR_materials_specular` | yes | yes | `ExtMaterialsSpecular` | match | `describe-scene-gltf` |
| `KHR_texture_transform` | yes | yes | `ExtTextureTransform` | match | `describe-scene-gltf` |
| `KHR_lights_punctual` | yes | yes | `ExtLightsPunctual` | match | `describe-scene-gltf` |
| `KHR_materials_pbrSpecularGlossiness` | yes (archived) | yes | `ExtMaterialsPBRSpecularGlossiness` | untested | — |
| `KHR_materials_anisotropy` | yes | yes | — | missing | — |
| `KHR_materials_iridescence` | yes | yes | — | missing | — |
| `KHR_materials_dispersion` | yes | yes | — | missing | — |
| `KHR_materials_diffuse_transmission` | no | yes | — | missing | — |
| `KHR_materials_variants` | yes | yes | — | missing | — |
| `KHR_mesh_quantization` | yes | yes | — | missing | — |
| `KHR_animation_pointer` | yes | no | — | missing | — |
| `KHR_texture_basisu` | no | yes | — | missing | — |
| `KHR_draco_mesh_compression` | no | yes | — | missing | — |
| `KHR_xmp_json_ld` | no | yes | — | missing | — |
| `EXT_texture_webp` | yes | yes | — | missing | — |
| `EXT_texture_avif` | no | yes | — | missing | — |
| `EXT_mesh_gpu_instancing` | no | yes | — | missing | — |
| `EXT_meshopt_compression` | no | yes | — | missing | — |

**Extension totals** — 25 names: **10 match, 0 differs, 14 missing, 1 untested**,
parity over the extensions actually compared: **10 / 10 = 100 %**.

The port models 11 of the 25 (10 tested). The 14 it does not model are still
preserved: unknown extensions round-trip verbatim through
`ExtensionMap`/`MarshalExtensions`, which is why an asset using them survives a
Go round-trip — but the port offers no typed accessors for them, and none of the
compression or quantization extensions (`KHR_draco_mesh_compression`,
`KHR_mesh_quantization`, `EXT_meshopt_compression`, `KHR_texture_basisu`,
`EXT_texture_webp`) can be *decoded*, so such an asset's geometry or textures
cannot be read.

## Upstream API surface exercised

Read-side accessors on `@gltf-transform/core`'s property classes
(`^(get|list|is)[A-Z]` on each prototype, excluding the framework-level
`getDefaults`/`getExtras`/`getName`):

| upstream class | accessors | exercised | Go counterpart | untested upstream accessors |
| --- | --- | --- | --- | --- |
| `Root` | 14 | 13 | `gltf.Document` fields | `listBuffers` |
| `Scene` | 1 | 1 | `gltf.Scene` | — |
| `Node` | 14 | 6 | `gltf.Node`, `Node.LocalMatrix` | `getTranslation` `getRotation` `getScale` `getWorldTranslation` `getWorldRotation` `getWorldScale` `getWorldMatrix` `getParentNode` |
| `Mesh` | 2 | 2 | `gltf.Mesh` | — |
| `Primitive` | 7 | 6 | `gltf.Primitive`, `Primitive.GetMode` | `listAttributes` |
| `PrimitiveTarget` | 3 | 2 | `Primitive.Targets` | `listAttributes` |
| `Accessor` | 16 | 8 | `Document.DecodeAccessor*`, `Accessor` fields | `getMin` `getMax` `getComponentSize` `getScalar` `getSparse` `getBuffer` `getArray` `getByteLength` |
| `Buffer` | 1 | 0 | `gltf.Buffer` | `getURI` |
| `Material` | 20 | 19 | `gltf.Material`, `Material.*()` extension getters | `getAlpha` |
| `Texture` | 4 | 3 | `Document.ImageBytes`, `Document.DecodeImage` | `getURI` |
| `TextureInfo` | 5 | 5 | `gltf.TextureInfo` + `gltf.Sampler` | — |
| `Animation` | 2 | 2 | `gltf.Animation` | — |
| `AnimationChannel` | 3 | 3 | `gltf.AnimationChannel` | — |
| `AnimationSampler` | 4 | 3 | `AnimationSampler.GetInterpolation` | `getDefaultAttributes` |
| `Skin` | 3 | 3 | `gltf.Skin` | — |
| `Camera` | 7 | 7 | `gltf.Camera`, `CameraPerspective`, `CameraOrthographic` | — |
| **total** | **106** | **83** | | **23** |

I/O and validation entry points: `NodeIO.read`, `NodeIO.readBinary`,
`NodeIO.writeBinary` and `validator.validateBytes` are exercised;
`NodeIO.write`/`writeJSON` and `validator.validateString` are not.

### Semantics the oracle cannot see

Deliberate shape choices, each forced by the oracle rather than by the port:

* `matrix` vs TRS — `@gltf-transform` decomposes a node's `matrix` into
  translation/rotation/scale on read, so the distinction is unobservable. Both
  sides are compared on the **composed local matrix** instead.
* declared `min`/`max` — `@gltf-transform` writes them only for `POSITION` and
  animation inputs, so they cannot survive a round-trip comparison. Both sides
  report the bounds of the **decoded** values; the declared bounds are graded by
  `gltf-validator` instead (finding 1 above).
* `bufferView.target` — recomputed by the upstream writer, so not comparable.
* `asset.generator` — each writer stamps its own name.
* standalone `sampler` objects — `@gltf-transform` folds sampler state into each
  texture reference, so filters and wrap modes are compared per reference.

## Go-only surface (`extra`)

`@gltf-transform/core` has no counterpart for these, so they are unscored:
`Document.EvaluateSampler`, `SampleChannel`, `ApplyAnimation`,
`SamplerKeyframes`, `JointMatrices`, `JointMatricesForNode`,
`InverseBindMatrices`, `MorphedPositions`, `DecodeMorphTargetsVec3`,
`BlendMorphTargetsVec3`, `EffectiveWeights`, `GlobalMatrix`, `GlobalMatrices`,
`NodesInScene`, `RootNodes`, `AccessorBounds`, `PrimitiveBounds`,
`SceneBounds`, `Camera.ProjectionMatrix`, `TextureTransform.UVMatrix`, and the
`Mat4`/`Quat`/`Vec3`/`Box` math helpers. (`getBounds` in `@gltf-transform/core`
loosely corresponds to `SceneBounds`, but its result is a world-space AABB of a
scene only and is not part of this comparison.)
