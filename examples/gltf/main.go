// Command gltf-example builds a non-trivial glTF 2.0 asset entirely in memory
// with github.com/malcolmston/gltf — a two-triangle quad with normals, UVs,
// indices, an embedded PNG texture, a PBR material carrying KHR extensions, a
// morph target, a two-joint skin, a perspective camera, a punctual light and a
// translation animation — then validates it, round-trips it through both the
// GLB and the JSON containers (in memory and on disk) and re-decodes every
// accessor to prove the data survived.
//
// The program always terminates on its own.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/malcolmston/gltf"
)

// quad geometry, shared between the builder and the round-trip checks.
var (
	quadPositions = [][3]float32{
		{-1, -1, 0}, {1, -1, 0}, {1, 1, 0}, {-1, 1, 0},
	}
	quadNormals = [][3]float32{
		{0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1},
	}
	quadUVs = [][2]float32{
		{0, 1}, {1, 1}, {1, 0}, {0, 0},
	}
	quadIndices = []uint32{0, 1, 2, 0, 2, 3}
	// morphDeltas bulges the top two vertices along +Z.
	morphDeltas = [][3]float32{
		{0, 0, 0}, {0, 0, 0}, {0, 0, 0.5}, {0, 0, 0.5},
	}
	// animTimes / animTranslations drive the root node's translation channel.
	animTimes        = []float32{0, 1, 2}
	animTranslations = [][3]float32{{0, 0, 0}, {0, 2, 0}, {0, 0, 0}}
)

func header(s string) { fmt.Printf("\n=== %s ===\n", s) }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "example failed:", err)
		os.Exit(1)
	}
}

func run() error {
	header("build a scene in memory")
	doc, err := buildScene()
	if err != nil {
		return err
	}
	describe(doc)

	header("validate")
	if err := doc.Validate(); err != nil {
		if errs, ok := gltf.AsValidationErrors(err); ok {
			for _, e := range errs {
				fmt.Printf("- %s\n", e)
			}
		}
		return fmt.Errorf("freshly built document did not validate: %w", err)
	}
	fmt.Println("document is structurally valid")
	reportInvalid()

	header("GLB round trip (in memory)")
	bin := doc.Buffers[0].Data
	var glb bytes.Buffer
	if err := gltf.WriteGLB(&glb, doc, bin); err != nil {
		return err
	}
	fmt.Printf("encoded GLB: %d bytes (JSON + %d byte BIN chunk)\n", glb.Len(), len(bin))

	back, chunk, err := gltf.ReadGLB(bytes.NewReader(glb.Bytes()))
	if err != nil {
		return err
	}
	if err := back.ResolveBuffers("", chunk); err != nil {
		return err
	}
	if err := checkRoundTrip("glb", back); err != nil {
		return err
	}

	header("JSON (.gltf) round trip with an embedded data URI")
	jsonDoc := cloneViaGLB(doc, bin)
	jsonDoc.Buffers[0].URI = gltf.EncodeDataURI(bin)
	raw, err := gltf.MarshalJSON(jsonDoc)
	if err != nil {
		return err
	}
	fmt.Printf("encoded .gltf: %d bytes of JSON (buffer inlined as base64)\n", len(raw))
	decoded, err := gltf.Decode(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := decoded.ResolveBuffers("", nil); err != nil {
		return err
	}
	if err := checkRoundTrip("gltf", decoded); err != nil {
		return err
	}

	header("file round trip (Save/Open, SaveGLB/OpenGLB)")
	dir, err := os.MkdirTemp("", "gltf-example")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	glbPath := filepath.Join(dir, "quad.glb")
	if err := gltf.SaveGLB(glbPath, doc, bin); err != nil {
		return err
	}
	fromGLB, err := gltf.OpenGLB(glbPath)
	if err != nil {
		return err
	}
	if err := checkRoundTrip("OpenGLB", fromGLB); err != nil {
		return err
	}

	gltfPath := filepath.Join(dir, "quad.gltf")
	if err := gltf.Save(gltfPath, jsonDoc); err != nil {
		return err
	}
	fromGLTF, err := gltf.Open(gltfPath)
	if err != nil {
		return err
	}
	if err := checkRoundTrip("Open", fromGLTF); err != nil {
		return err
	}
	fmt.Printf("wrote and reloaded %s and %s\n", filepath.Base(glbPath), filepath.Base(gltfPath))

	header("bounds")
	if err := reportBounds(back); err != nil {
		return err
	}

	header("scene graph transforms")
	if err := reportTransforms(back); err != nil {
		return err
	}

	header("animation sampling")
	if err := reportAnimation(back); err != nil {
		return err
	}

	header("morph targets")
	if err := reportMorph(back); err != nil {
		return err
	}

	header("skinning")
	if err := reportSkin(back); err != nil {
		return err
	}

	header("material extensions")
	if err := reportExtensions(back); err != nil {
		return err
	}

	header("embedded image")
	if err := reportImage(back); err != nil {
		return err
	}

	header("standalone math helpers")
	reportMath()

	header("done")
	fmt.Println("all sections completed")
	return nil
}

// buildScene assembles the whole asset with the Add* write-path helpers.
func buildScene() (*gltf.Document, error) {
	doc := &gltf.Document{
		Asset: gltf.Asset{
			Version:   gltf.Version,
			Generator: "go-examples/gltf",
			Copyright: "public domain",
		},
	}

	// --- geometry accessors (each appends to buffer 0) ---
	posAcc := doc.AddAccessorVec3(quadPositions)
	nrmAcc := doc.AddAccessorVec3(quadNormals)
	uvAcc := doc.AddAccessorVec2(quadUVs)
	idxAcc := doc.AddIndicesUint32(quadIndices)
	morphAcc := doc.AddAccessorVec3(morphDeltas)
	colAcc := doc.AddAccessorVec4([][4]float32{
		{1, 0, 0, 1}, {0, 1, 0, 1}, {0, 0, 1, 1}, {1, 1, 0, 1},
	})

	// --- animation accessors ---
	timeAcc := doc.AddAccessorFloat32(animTimes, gltf.AccessorScalar)
	transAcc := doc.AddAccessorVec3(animTranslations)

	// --- skin inverse bind matrices (MAT4 accessor, flattened) ---
	ibm0 := gltf.IdentityMatrix()
	ibm1 := gltf.TRS(gltf.Vec3{0, -1, 0}, gltf.Quat{0, 0, 0, 1}, gltf.Vec3{1, 1, 1})
	flatIBM := make([]float32, 0, 32)
	for _, m := range []gltf.Mat4{ibm0, ibm1} {
		for _, v := range m {
			flatIBM = append(flatIBM, float32(v))
		}
	}
	ibmAcc := doc.AddAccessorFloat32(flatIBM, gltf.AccessorMat4)

	// --- an embedded PNG texture stored in a bufferView ---
	pngBytes, err := makePNG()
	if err != nil {
		return nil, err
	}
	imgView := doc.AddBinData(pngBytes)
	doc.Images = append(doc.Images, gltf.Image{
		Name:       "checker",
		MimeType:   "image/png",
		BufferView: &imgView,
	})
	magFilter := gltf.FilterNearest
	minFilter := gltf.FilterLinearMipmapLinear
	wrap := gltf.WrapRepeat
	doc.Samplers = append(doc.Samplers, gltf.Sampler{
		Name:      "nearest-repeat",
		MagFilter: &magFilter,
		MinFilter: &minFilter,
		WrapS:     &wrap,
		WrapT:     &wrap,
	})
	texSampler, texSource := 0, 0
	doc.Textures = append(doc.Textures, gltf.Texture{
		Name:    "checker",
		Sampler: &texSampler,
		Source:  &texSource,
	})

	// --- material: PBR + KHR_texture_transform + three material extensions ---
	metallic, roughness := 0.1, 0.6
	baseColorTex := gltf.TextureInfo{Index: 0}
	uvXform := gltf.TextureTransform{
		Offset:   &[2]float64{0.1, 0.2},
		Rotation: math.Pi / 4,
		Scale:    &[2]float64{2, 2},
	}
	texExt, err := gltf.SetExtension(baseColorTex.Extensions, gltf.ExtTextureTransform, uvXform)
	if err != nil {
		return nil, err
	}
	baseColorTex.Extensions = texExt

	mat := gltf.Material{
		Name:        "glass-ish",
		AlphaMode:   gltf.AlphaBlend,
		DoubleSided: true,
		PBRMetallicRoughness: &gltf.PBRMetallicRoughness{
			BaseColorFactor:  &[4]float64{0.8, 0.9, 1.0, 0.5},
			BaseColorTexture: &baseColorTex,
			MetallicFactor:   &metallic,
			RoughnessFactor:  &roughness,
		},
		EmissiveFactor: &[3]float64{0.2, 0.2, 0.2},
	}
	if err := mat.SetExtension(gltf.ExtMaterialsEmissiveStrength,
		gltf.MaterialsEmissiveStrength{EmissiveStrength: 4.5}); err != nil {
		return nil, err
	}
	if err := mat.SetExtension(gltf.ExtMaterialsIOR, gltf.MaterialsIOR{IOR: 1.33}); err != nil {
		return nil, err
	}
	if err := mat.SetExtension(gltf.ExtMaterialsTransmission,
		gltf.MaterialsTransmission{TransmissionFactor: 0.9}); err != nil {
		return nil, err
	}
	// An unknown, vendor-specific extension must round-trip verbatim.
	if err := mat.SetExtension("VENDOR_secret_sauce", map[string]any{"spice": 11}); err != nil {
		return nil, err
	}
	doc.Materials = append(doc.Materials, mat)
	matIdx := 0

	// --- mesh with a morph target ---
	mode := gltf.PrimitiveTriangles
	doc.Meshes = append(doc.Meshes, gltf.Mesh{
		Name:    "quad",
		Weights: []float64{0.25},
		Primitives: []gltf.Primitive{{
			Attributes: map[string]int{
				"POSITION":   posAcc,
				"NORMAL":     nrmAcc,
				"TEXCOORD_0": uvAcc,
				"COLOR_0":    colAcc,
			},
			Indices:  &idxAcc,
			Material: &matIdx,
			Mode:     &mode,
			Targets:  []map[string]int{{"POSITION": morphAcc}},
		}},
	})

	// --- camera ---
	zfar, aspect := 100.0, 16.0/9.0
	doc.Cameras = append(doc.Cameras, gltf.Camera{
		Name: "main",
		Type: gltf.CameraTypePerspective,
		Perspective: &gltf.CameraPerspective{
			AspectRatio: &aspect,
			YFOV:        math.Pi / 3,
			ZNear:       0.1,
			ZFar:        &zfar,
		},
	})

	// --- punctual light (document-level extension + node reference) ---
	intensity := 800.0
	outer := math.Pi / 6
	lights := gltf.LightsPunctual{Lights: []gltf.Light{{
		Name:      "spot",
		Type:      gltf.LightTypeSpot,
		Color:     &[3]float64{1, 0.9, 0.8},
		Intensity: &intensity,
		Spot:      &gltf.LightSpot{InnerConeAngle: 0.1, OuterConeAngle: &outer},
	}}}
	docExt, err := gltf.SetExtension(doc.Extensions, gltf.ExtLightsPunctual, lights)
	if err != nil {
		return nil, err
	}
	doc.Extensions = docExt

	nodeLightExt, err := gltf.SetExtension(nil, gltf.ExtLightsPunctual, gltf.NodeLight{Light: 0})
	if err != nil {
		return nil, err
	}

	// --- node hierarchy ---
	// 0 root (TRS)  -> 1 mesh(skinned), 2 camera, 3 light, 4 joint-a -> 5 joint-b
	meshIdx, camIdx, skinIdx := 0, 0, 0
	rot := gltf.QuatFromAxisAngle(gltf.Vec3{0, 0, 1}, math.Pi/2).Normalize()
	doc.Nodes = []gltf.Node{
		{
			Name:        "root",
			Children:    []int{1, 2, 3, 4},
			Translation: &[3]float64{1, 0, 0},
			Rotation:    &[4]float64{rot[0], rot[1], rot[2], rot[3]},
			Scale:       &[3]float64{2, 2, 2},
		},
		{Name: "quad", Mesh: &meshIdx, Skin: &skinIdx, Weights: []float64{0.5}},
		{Name: "camera", Camera: &camIdx, Translation: &[3]float64{0, 0, 5}},
		{Name: "light", Extensions: nodeLightExt, Translation: &[3]float64{0, 3, 0}},
		{Name: "joint-a", Children: []int{5}, Translation: &[3]float64{0, 1, 0}},
		{Name: "joint-b", Translation: &[3]float64{0, 1, 0}},
	}

	// --- skin over the two joint nodes ---
	skeleton := 4
	doc.Skins = append(doc.Skins, gltf.Skin{
		Name:                "two-bones",
		Joints:              []int{4, 5},
		Skeleton:            &skeleton,
		InverseBindMatrices: &ibmAcc,
	})

	// --- animation: translate the root node ---
	animNode := 0
	doc.Animations = append(doc.Animations, gltf.Animation{
		Name: "bob",
		Samplers: []gltf.AnimationSampler{{
			Input:         timeAcc,
			Output:        transAcc,
			Interpolation: gltf.InterpolationLinear,
		}},
		Channels: []gltf.AnimationChannel{{
			Sampler: 0,
			Target: gltf.AnimationChannelTarget{
				Node: &animNode,
				Path: gltf.PathTranslation,
			},
		}},
	})

	// --- scene ---
	sceneIdx := 0
	doc.Scenes = append(doc.Scenes, gltf.Scene{Name: "main", Nodes: []int{0}})
	doc.Scene = &sceneIdx

	doc.ExtensionsUsed = []string{
		gltf.ExtLightsPunctual,
		gltf.ExtTextureTransform,
		gltf.ExtMaterialsEmissiveStrength,
		gltf.ExtMaterialsIOR,
		gltf.ExtMaterialsTransmission,
		"VENDOR_secret_sauce",
	}

	return doc, nil
}

// makePNG encodes a tiny 2x2 checker as PNG bytes.
func makePNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{0, 0, 0, 255})
	img.Set(0, 1, color.RGBA{0, 0, 0, 255})
	img.Set(1, 1, color.RGBA{255, 255, 255, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// describe prints a census of the built document.
func describe(d *gltf.Document) {
	fmt.Printf("asset:      version=%s generator=%q\n", d.Asset.Version, d.Asset.Generator)
	fmt.Printf("counts:     scenes=%d nodes=%d meshes=%d accessors=%d bufferViews=%d buffers=%d\n",
		len(d.Scenes), len(d.Nodes), len(d.Meshes), len(d.Accessors), len(d.BufferViews), len(d.Buffers))
	fmt.Printf("            materials=%d textures=%d images=%d samplers=%d animations=%d skins=%d cameras=%d\n",
		len(d.Materials), len(d.Textures), len(d.Images), len(d.Samplers),
		len(d.Animations), len(d.Skins), len(d.Cameras))
	fmt.Printf("buffer 0:   %d bytes (byteLength field = %d)\n", len(d.Buffers[0].Data), d.Buffers[0].ByteLength)
	fmt.Printf("extensions: %v\n", d.ExtensionsUsed)
	fmt.Printf("root nodes: %v\n", d.RootNodes())
	acc := d.Accessors[0]
	fmt.Printf("accessor 0: %s/%s count=%d min=%v max=%v (min/max computed by Add*)\n",
		acc.Type, acc.ComponentType, acc.Count, acc.Min, acc.Max)
}

// reportInvalid shows Validate rejecting a deliberately broken document.
func reportInvalid() {
	broken := &gltf.Document{
		Asset:     gltf.Asset{}, // missing required version
		Accessors: []gltf.Accessor{{ComponentType: gltf.ComponentFloat, Count: 1, Type: "VEC9"}},
		Nodes:     []gltf.Node{{Name: "dangling", Mesh: intp(7)}},
	}
	err := broken.Validate()
	errs, ok := gltf.AsValidationErrors(err)
	if !ok {
		fmt.Println("unexpected: broken document validated cleanly")
		return
	}
	fmt.Printf("a deliberately broken document yields %d validation error(s):\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  - %s\n", e)
	}
}

// cloneViaGLB deep-copies a document by encoding and decoding it, so mutating
// the copy cannot disturb the original.
func cloneViaGLB(d *gltf.Document, bin []byte) *gltf.Document {
	var buf bytes.Buffer
	if err := gltf.WriteGLB(&buf, d, bin); err != nil {
		panic(err)
	}
	c, chunk, err := gltf.ReadGLB(bytes.NewReader(buf.Bytes()))
	if err != nil {
		panic(err)
	}
	if err := c.ResolveBuffers("", chunk); err != nil {
		panic(err)
	}
	// Detach from the shared BIN chunk so a URI can be assigned safely.
	c.Buffers[0].Data = append([]byte(nil), c.Buffers[0].Data...)
	return c
}

// checkRoundTrip re-decodes every accessor of a reloaded document and compares
// it against the source data.
func checkRoundTrip(label string, d *gltf.Document) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("%s: reloaded document failed validation: %w", label, err)
	}
	prim := &d.Meshes[0].Primitives[0]

	pos, err := d.DecodeAccessorVec3(prim.Attributes["POSITION"])
	if err != nil {
		return err
	}
	nrm, err := d.DecodeAccessorVec3(prim.Attributes["NORMAL"])
	if err != nil {
		return err
	}
	uv, err := d.DecodeAccessorVec2(prim.Attributes["TEXCOORD_0"])
	if err != nil {
		return err
	}
	col, err := d.DecodeAccessorVec4(prim.Attributes["COLOR_0"])
	if err != nil {
		return err
	}
	idx, err := d.DecodeIndices(prim)
	if err != nil {
		return err
	}
	flat, err := d.DecodeAccessorFloat32(prim.Attributes["POSITION"])
	if err != nil {
		return err
	}
	u32, err := d.DecodeAccessorUint32(*prim.Indices)
	if err != nil {
		return err
	}
	mats, err := d.DecodeAccessorMat4(*d.Skins[0].InverseBindMatrices)
	if err != nil {
		return err
	}

	switch {
	case !eqVec3(pos, quadPositions):
		return fmt.Errorf("%s: POSITION mismatch: %v", label, pos)
	case !eqVec3(nrm, quadNormals):
		return fmt.Errorf("%s: NORMAL mismatch: %v", label, nrm)
	case !eqVec2(uv, quadUVs):
		return fmt.Errorf("%s: TEXCOORD_0 mismatch: %v", label, uv)
	case len(col) != 4:
		return fmt.Errorf("%s: COLOR_0 count %d", label, len(col))
	case !eqU32(idx, quadIndices) || !eqU32(u32, quadIndices):
		return fmt.Errorf("%s: index mismatch: %v", label, idx)
	case len(flat) != len(quadPositions)*3:
		return fmt.Errorf("%s: flattened float count %d", label, len(flat))
	case len(mats) != 2:
		return fmt.Errorf("%s: inverse bind matrix count %d", label, len(mats))
	case d.Meshes[0].Name != "quad" || d.Nodes[0].Name != "root":
		return fmt.Errorf("%s: names not preserved", label)
	case len(d.Animations) != 1 || len(d.Animations[0].Channels) != 1:
		return fmt.Errorf("%s: animation not preserved", label)
	}
	fmt.Printf("%-8s round trip OK: %d verts, %d indices, %d uvs, %d colors, %d IBMs, mode=%s\n",
		label, len(pos), len(idx), len(uv), len(col), len(mats), primModeName(prim.GetMode()))
	return nil
}

func reportBounds(d *gltf.Document) error {
	prim := &d.Meshes[0].Primitives[0]
	ab, err := d.AccessorBounds(prim.Attributes["POSITION"])
	if err != nil {
		return err
	}
	pb, err := d.PrimitiveBounds(prim)
	if err != nil {
		return err
	}
	sb, err := d.SceneBounds(0)
	if err != nil {
		return err
	}
	fmt.Printf("accessor bounds:  min=%v max=%v\n", ab.Min, ab.Max)
	fmt.Printf("primitive bounds: min=%v max=%v center=%v size=%v\n", pb.Min, pb.Max, pb.Center(), pb.Size())
	fmt.Printf("scene bounds:     min=%v max=%v (root TRS applied)\n", round3(sb.Min), round3(sb.Max))

	box := gltf.EmptyBox()
	fmt.Printf("EmptyBox empty=%v; after Add({1,2,3}) empty=%v\n",
		box.Empty(), box.Add(gltf.Vec3{1, 2, 3}).Empty())
	moved := pb.Transform(gltf.TRS(gltf.Vec3{10, 0, 0}, gltf.Quat{0, 0, 0, 1}, gltf.Vec3{1, 1, 1}))
	fmt.Printf("Box.Transform(+10x): min=%v max=%v\n", moved.Min, moved.Max)
	fmt.Printf("Union with origin box: %v\n", pb.Union(gltf.Box{}).Min)
	return nil
}

func reportTransforms(d *gltf.Document) error {
	local := d.Nodes[0].LocalMatrix()
	t, r, s := local.Decompose()
	fmt.Printf("node 0 local TRS -> t=%v r=%v s=%v\n", round3(t), round4(r), round3(s))

	all, err := d.GlobalMatrices()
	if err != nil {
		return err
	}
	for i, m := range all {
		tt, _, ss := m.Decompose()
		fmt.Printf("  node %d %-8s global translation=%v scale=%v\n", i, d.Nodes[i].Name, round3(tt), round3(ss))
	}

	g, err := d.GlobalMatrix(5)
	if err != nil {
		return err
	}
	fmt.Printf("joint-b world position: %v\n", round3(g.TransformPoint(gltf.Vec3{0, 0, 0})))

	inScene, err := d.NodesInScene(0)
	if err != nil {
		return err
	}
	sort.Ints(inScene)
	fmt.Printf("nodes reachable from scene 0: %v\n", inScene)

	cam := d.Cameras[0]
	proj := cam.Perspective.ProjectionMatrix()
	fmt.Printf("camera projection [0]=%.4f [5]=%.4f [11]=%.1f\n", proj[0], proj[5], proj[11])
	return nil
}

func reportAnimation(d *gltf.Document) error {
	anim := &d.Animations[0]
	times, values, stride, err := d.SamplerKeyframes(&anim.Samplers[0])
	if err != nil {
		return err
	}
	fmt.Printf("sampler %q: %d keyframes, stride=%d, interpolation=%s\n",
		anim.Name, len(times), stride, anim.Samplers[0].GetInterpolation())
	fmt.Printf("  times=%v values=%v\n", times, values)

	for _, t := range []float64{0, 0.5, 1, 1.5, 2, 9} {
		path, vals, err := d.SampleChannel(anim, 0, t)
		if err != nil {
			return err
		}
		fmt.Printf("  t=%-4.1f %s -> %v\n", t, path, vals)
	}

	if err := d.ApplyAnimation(anim, 1.0); err != nil {
		return err
	}
	fmt.Printf("after ApplyAnimation(t=1): node 0 translation=%v\n", *d.Nodes[0].Translation)
	if err := d.ApplyAnimation(anim, 0.0); err != nil {
		return err
	}
	fmt.Printf("after ApplyAnimation(t=0): node 0 translation=%v\n", *d.Nodes[0].Translation)
	return nil
}

func reportMorph(d *gltf.Document) error {
	prim := &d.Meshes[0].Primitives[0]
	weights := gltf.EffectiveWeights(&d.Nodes[1], &d.Meshes[0])
	fmt.Printf("effective weights (node overrides mesh): %v\n", weights)

	deltas, err := d.DecodeMorphTargetVec3(prim, 0, "POSITION")
	if err != nil {
		return err
	}
	fmt.Printf("morph target 0 deltas: %v\n", deltas)

	all, err := d.DecodeMorphTargetsVec3(prim, "POSITION")
	if err != nil {
		return err
	}
	fmt.Printf("morph target count: %d\n", len(all))

	morphed, err := d.MorphedPositions(prim, weights)
	if err != nil {
		return err
	}
	fmt.Printf("morphed positions at w=%v: %v\n", weights, morphed)

	blended := gltf.BlendMorphTargetsVec3(quadPositions, all, []float64{1.0})
	fmt.Printf("fully blended (w=1):           %v\n", blended)

	if _, err := d.DecodeMorphTargetVec3(prim, 0, "TANGENT"); err != nil {
		fmt.Printf("missing morph attribute reports: %v\n", err)
	}
	return nil
}

func reportSkin(d *gltf.Document) error {
	ibm, err := d.InverseBindMatrices(0)
	if err != nil {
		return err
	}
	fmt.Printf("inverse bind matrices: %d (first translation column=%v)\n",
		len(ibm), []float64{ibm[1][12], ibm[1][13], ibm[1][14]})

	jm, err := d.JointMatrices(0)
	if err != nil {
		return err
	}
	for i, m := range jm {
		tt, _, _ := m.Decompose()
		fmt.Printf("  joint %d matrix translation=%v\n", i, round3(tt))
	}

	jmNode, err := d.JointMatricesForNode(0, 1)
	if err != nil {
		return err
	}
	tt, _, _ := jmNode[0].Decompose()
	fmt.Printf("joint 0 relative to mesh node 1: translation=%v\n", round3(tt))
	return nil
}

func reportExtensions(d *gltf.Document) error {
	m := &d.Materials[0]
	fmt.Printf("material %q alphaMode=%s doubleSided=%v unlit=%v\n",
		m.Name, m.AlphaMode, m.DoubleSided, m.Unlit())

	if v, ok := m.EmissiveStrength(); ok {
		fmt.Printf("KHR_materials_emissive_strength: %.2f\n", v)
	}
	if v, ok := m.IOR(); ok {
		fmt.Printf("KHR_materials_ior:               %.2f\n", v)
	}
	if t, ok := m.Transmission(); ok {
		fmt.Printf("KHR_materials_transmission:      %.2f\n", t.TransmissionFactor)
	}

	if xf, ok := m.PBRMetallicRoughness.BaseColorTexture.TextureTransform(); ok {
		uv := xf.UVMatrix()
		fmt.Printf("KHR_texture_transform: offset=%v rotation=%.4f scale=%v\n", *xf.Offset, xf.Rotation, *xf.Scale)
		fmt.Printf("  UV matrix: [%.3f %.3f %.3f | %.3f %.3f %.3f | %.3f %.3f %.3f]\n",
			uv[0], uv[1], uv[2], uv[3], uv[4], uv[5], uv[6], uv[7], uv[8])
	}

	lights, ok := d.Lights()
	if !ok {
		return fmt.Errorf("KHR_lights_punctual missing after round trip")
	}
	l := lights[0]
	fmt.Printf("KHR_lights_punctual: %q type=%s intensity=%.0f outerCone=%.4f\n",
		l.Name, l.Type, *l.Intensity, *l.Spot.OuterConeAngle)
	li, ok := d.Nodes[3].NodeLight()
	fmt.Printf("node 3 references light index %d (present=%v)\n", li, ok)

	// The unknown vendor extension must have survived verbatim.
	all, err := gltf.ExtensionMap(m.Extensions)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Printf("material extensions present: %v\n", names)
	var sauce struct{ Spice int }
	found, err := gltf.GetExtension(m.Extensions, "VENDOR_secret_sauce", &sauce)
	if err != nil {
		return err
	}
	fmt.Printf("unknown extension preserved: found=%v spice=%d\n", found, sauce.Spice)

	// Removing an extension by passing nil.
	stripped, err := gltf.SetExtension(m.Extensions, "VENDOR_secret_sauce", nil)
	if err != nil {
		return err
	}
	after, _ := gltf.ExtensionMap(stripped)
	fmt.Printf("after removal, %d extension(s) remain\n", len(after))

	raw, err := gltf.MarshalExtensions(map[string]json.RawMessage{})
	fmt.Printf("MarshalExtensions(empty) -> nil=%v err=%v\n", raw == nil, err)
	return nil
}

func reportImage(d *gltf.Document) error {
	raw, mime, err := d.ImageBytes(0, "")
	if err != nil {
		return err
	}
	fmt.Printf("image 0: %d bytes, mime=%s\n", len(raw), mime)

	img, format, err := d.DecodeImage(0, "")
	if err != nil {
		return err
	}
	b := img.Bounds()
	r, g, bl, a := img.At(0, 0).RGBA()
	fmt.Printf("decoded as %s: %dx%d, pixel(0,0)=(%d,%d,%d,%d)\n",
		format, b.Dx(), b.Dy(), r>>8, g>>8, bl>>8, a>>8)
	return nil
}

func reportMath() {
	q := gltf.QuatFromAxisAngle(gltf.Vec3{0, 0, 1}, math.Pi/2)
	fmt.Printf("QuatFromAxisAngle(z, 90deg) rotates {1,0,0} -> %v\n", round3(q.Rotate(gltf.Vec3{1, 0, 0})))
	fmt.Printf("QuatFromEuler(0,0,pi/2)                    -> %v\n", round4(gltf.QuatFromEuler(0, 0, math.Pi/2)))
	half := gltf.Slerp(gltf.Quat{0, 0, 0, 1}, q, 0.5).Normalize()
	fmt.Printf("Slerp halfway rotates {1,0,0}              -> %v\n", round3(half.Rotate(gltf.Vec3{1, 0, 0})))
	fmt.Printf("Quat conjugate of %v = %v\n", round4(q), round4(q.Conjugate()))

	m := gltf.TRS(gltf.Vec3{1, 2, 3}, q, gltf.Vec3{2, 2, 2})
	inv, ok := m.Inverse()
	fmt.Printf("TRS invertible=%v; m*inv*point{4,5,6} = %v\n", ok, round3(m.Mul(inv).TransformPoint(gltf.Vec3{4, 5, 6})))

	view := gltf.LookAt(gltf.Vec3{0, 0, 5}, gltf.Vec3{0, 0, 0}, gltf.Vec3{0, 1, 0})
	fmt.Printf("LookAt eye z=5: origin in view space = %v\n", round3(view.TransformPoint(gltf.Vec3{0, 0, 0})))
	persp := gltf.Perspective(math.Pi/2, 1, 1, 100)
	ortho := gltf.Orthographic(2, 2, 1, 100)
	fmt.Printf("Perspective[0]=%.4f Orthographic[0]=%.4f\n", persp[0], ortho[0])

	v := gltf.Vec3{1, 2, 3}
	fmt.Printf("Vec3 ops: add=%v sub=%v cross=%v normalized=%v\n",
		v.Add(gltf.Vec3{1, 1, 1}), v.Sub(gltf.Vec3{1, 1, 1}),
		v.Cross(gltf.Vec3{0, 1, 0}), round3(v.Normalize()))
	fmt.Printf("enum strings: %s / %s / %s / %s\n",
		gltf.ComponentFloat, gltf.FilterNearest, gltf.WrapRepeat, gltf.TargetArrayBuffer)
	fmt.Printf("AccessorMat4 component count = %d; ComponentUnsignedShort size = %d\n",
		gltf.AccessorMat4.ComponentCount(), gltf.ComponentUnsignedShort.SizeInBytes())
}

// --- small helpers ---

func intp(v int) *int { return &v }

func primModeName(m gltf.PrimitiveMode) string { return m.String() }

func eqVec3(got [][3]float32, want [][3]float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		for c := 0; c < 3; c++ {
			if got[i][c] != want[i][c] {
				return false
			}
		}
	}
	return true
}

func eqVec2(got [][2]float32, want [][2]float32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			return false
		}
	}
	return true
}

func eqU32(got, want []uint32) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func round3(v gltf.Vec3) gltf.Vec3 {
	for i := range v {
		v[i] = roundTo(v[i], 4)
	}
	return v
}

func round4(q gltf.Quat) gltf.Quat {
	for i := range q {
		q[i] = roundTo(q[i], 4)
	}
	return q
}

func roundTo(f float64, places int) float64 {
	p := math.Pow10(places)
	return math.Round(f*p) / p
}
