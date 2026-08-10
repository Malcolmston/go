# gltf example

A single runnable program that exercises [`github.com/malcolmston/gltf`](https://github.com/Malcolmston/gltf)
— a dependency-free glTF 2.0 reader/writer/inspector — as an outside consumer
would: the dependency is the published module, with no `replace` directive.

**Module version under test:
`github.com/malcolmston/gltf v0.0.0-20260719012632-d6d42aa8bd54`**
(the repo has no semver tags, so `@latest` resolves to this pseudo-version).

## Run

```sh
cd examples/gltf
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

Everything happens in memory except one temp directory (created with
`os.MkdirTemp` and removed on exit) used to prove the file helpers work. The
program terminates on its own.

## What it demonstrates

**Building an asset from scratch** (`buildScene`) — nothing is loaded from disk:

- Vertex data through the write path: `AddAccessorVec3` (positions, normals,
  morph deltas, animation outputs), `AddAccessorVec2` (UVs), `AddAccessorVec4`
  (vertex colors), `AddAccessorFloat32` with `AccessorScalar` (keyframe times)
  and `AccessorMat4` (inverse bind matrices), `AddIndicesUint32` (triangle
  indices). Each call appends 4-byte-aligned bytes to buffer 0, creates a
  `BufferView` with the right `target`, and computes per-component `min`/`max`.
- An embedded PNG texture: `AddBinData` for the encoded bytes plus an `Image`
  with a `bufferView`, a `Sampler` (`FilterNearest`, `FilterLinearMipmapLinear`,
  `WrapRepeat`) and a `Texture`.
- A PBR `Material` with `AlphaBlend`, a base-color texture carrying
  `KHR_texture_transform`, and `KHR_materials_emissive_strength`,
  `KHR_materials_ior`, `KHR_materials_transmission` set via
  `Material.SetExtension` — plus an unknown `VENDOR_secret_sauce` extension to
  prove verbatim round-tripping.
- A mesh primitive with `POSITION`/`NORMAL`/`TEXCOORD_0`/`COLOR_0`, indices and
  one morph target.
- A perspective `Camera`, a `KHR_lights_punctual` spot light (document-level
  `LightsPunctual` plus a node-level `NodeLight`), a six-node hierarchy with TRS
  on the root, a two-joint `Skin` with an inverse-bind-matrix accessor, and a
  LINEAR translation `Animation`.

**Validation**: `Document.Validate` on the built document (clean) and on a
deliberately broken one, decoded with `AsValidationErrors` — it reports the
missing `asset.version`, an out-of-range `nodes[0].mesh` and an unknown accessor
type, each with a JSON path.

**Four round trips**, each re-decoding every accessor and comparing against the
source arrays: in-memory GLB (`WriteGLB` → `ReadGLB` → `ResolveBuffers`),
in-memory `.gltf` with the buffer inlined as a base64 data URI (`EncodeDataURI` →
`MarshalJSON` → `Decode` → `ResolveBuffers`), and the file helpers `SaveGLB`/
`OpenGLB` and `Save`/`Open`.

**Reading and analysis** on the reloaded document:

- `DecodeAccessorVec2/Vec3/Vec4/Float32/Uint32/Mat4`, `DecodeIndices`.
- Bounds: `AccessorBounds`, `PrimitiveBounds`, `SceneBounds`, `Box.Center`,
  `Size`, `Transform`, `Union`, `EmptyBox`.
- Transforms: `Node.LocalMatrix`, `Mat4.Decompose`, `GlobalMatrix`,
  `GlobalMatrices`, `NodesInScene`, `RootNodes`,
  `CameraPerspective.ProjectionMatrix`.
- Animation: `SamplerKeyframes`, `AnimationSampler.GetInterpolation`,
  `SampleChannel` at six times (including before/after the clip), and
  `ApplyAnimation` posing nodes in place.
- Morph targets: `EffectiveWeights` (node weights overriding mesh weights),
  `DecodeMorphTargetVec3`, `DecodeMorphTargetsVec3`, `BlendMorphTargetsVec3`,
  `MorphedPositions`.
- Skinning: `InverseBindMatrices`, `JointMatrices`, `JointMatricesForNode`.
- Extensions: the typed accessors (`Material.Unlit`, `EmissiveStrength`, `IOR`,
  `Transmission`, `TextureInfo.TextureTransform` + `UVMatrix`,
  `Document.Lights`, `Node.NodeLight`) and the raw ones (`ExtensionMap`,
  `MarshalExtensions`, `GetExtension`, `SetExtension`, including removal by
  passing a nil value).
- Images: `ImageBytes` and `DecodeImage` (decodes the embedded PNG back to an
  `image.Image`).
- Math: `QuatFromAxisAngle`, `QuatFromEuler`, `Slerp`, `Quat.Rotate`,
  `Conjugate`, `TRS`, `Mat4.Mul`, `Inverse`, `TransformPoint`, `LookAt`,
  `Perspective`, `Orthographic`, `Vec3` ops, and the enum `String()` /
  `ComponentCount()` / `SizeInBytes()` helpers.

## Holes found

**None.** Every API the README and package documentation advertise exists with
the documented signature and behaved correctly; the example needed no `// HOLE:`
workaround. The published module is also byte-identical to the repository working
tree, so there were no uncommitted-change surprises. Numerically the round trips
are exact (float32 in, float32 out), animation interpolation and clamping match
the spec, sparse-free accessor decode honors `byteStride`/`byteOffset`, and the
unknown vendor extension survived a GLB round trip untouched.

## Rough edges (not bugs, but worth knowing)

- **`WriteGLB(w, doc, bin)` takes the BIN chunk separately** even though a
  document built with the `Add*` helpers already carries it in
  `doc.Buffers[0].Data`. You must remember to pass `doc.Buffers[0].Data`; passing
  `nil` writes a JSON-only GLB that only fails later, at `ResolveBuffers` time.
  There is no `WriteGLBFromDocument`-style convenience.
- **`Save` never writes buffer bytes.** Saving an `Add*`-built document as
  `.gltf` produces an unloadable file unless you first set
  `doc.Buffers[0].URI = gltf.EncodeDataURI(doc.Buffers[0].Data)` yourself (or
  write a sidecar `.bin` by hand). The godoc says so, but the failure mode is a
  silently broken asset rather than an error. Only the `Triangle` helper
  (`WriteTriangleGLTF`) does the data-URI step for you.
- **`ResolveBuffers` aliases the GLB BIN chunk** into `Buffer.Data`
  (`b.Data = bin[:b.ByteLength]`) rather than copying, so two documents resolved
  from the same chunk share backing memory. The example copies explicitly in
  `cloneViaGLB` before mutating.
- **The zero `Box` is not empty.** `Box{}` is a degenerate box at the origin, so
  `someBox.Union(gltf.Box{})` silently stretches the result to include the
  origin. `EmptyBox()` is the correct identity, and it is easy to forget.
- **`type Index = int` is an alias, not a distinct type**, so nothing stops you
  from passing an accessor index where a node index is wanted; the compiler
  cannot help.
- **Optional spec fields are pointers throughout** (`*Index`, `*float64`,
  `*[3]float64`, `*Filter`, ...), which is faithful to glTF's "absent means
  default" semantics but means a lot of local temporaries just to take addresses
  when constructing a document by hand.
- `Quat.Conjugate` can produce `-0` components (cosmetic only; they compare equal
  to `0` and serialize as `-0`).
