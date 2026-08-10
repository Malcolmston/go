// Command run is the Go-side runner for the gltf parity harness.
//
// It speaks the same JSON-Lines protocol as node/run.mjs: one request object per
// line on stdin, exactly one reply object per line on stdout. A panic or error
// in any operation is reported as {"ok":false,"error":...} — the process never
// exits early, so one bad case cannot cost the remaining ones.
//
// Environment:
//
//	PARITY_FIXTURES  directory holding the committed fixture assets
//	PARITY_OUT       shared scratch directory for cross-implementation writes
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/malcolmston/gltf"
)

const (
	lang  = "go"
	other = "node"
	// absent is the sentinel both runners use for "no reference"/"unset".
	absent = -1.0
)

var (
	fixturesDir = env("PARITY_FIXTURES", "fixtures")
	outDir      = env("PARITY_OUT", ".")
	unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ------------------------------------------------------------------ protocol

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
	// Note carries diagnostic text the harness logs but never compares, so a
	// differently worded rejection reason cannot fail a case on its own.
	Note string `json:"note,omitempty"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			emit(out, response{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		value, note, err := invoke(req)
		if err != nil {
			emit(out, response{ID: req.ID, OK: false, Error: err.Error()})
			continue
		}
		emit(out, response{ID: req.ID, OK: true, Value: value, Note: note})
	}
}

func emit(out *bufio.Writer, resp response) {
	enc, err := json.Marshal(resp)
	if err != nil {
		enc, _ = json.Marshal(response{ID: resp.ID, OK: false, Error: "encode reply: " + err.Error()})
	}
	out.Write(enc)
	out.WriteByte('\n')
	out.Flush()
}

// invoke dispatches one request, converting a panic into an error so a bug in
// the library under test scores as a failed case rather than killing the runner.
func invoke(req request) (value any, note string, err error) {
	defer func() {
		if r := recover(); r != nil {
			value, note, err = nil, "", fmt.Errorf("panic: %v", r)
		}
	}()

	str := func(i int) (string, error) {
		if i >= len(req.Args) {
			return "", fmt.Errorf("%s: missing argument %d", req.Fn, i)
		}
		var s string
		if err := json.Unmarshal(req.Args[i], &s); err != nil {
			return "", fmt.Errorf("%s: argument %d is not a string", req.Fn, i)
		}
		return s, nil
	}
	num := func(i int) (int, error) {
		if i >= len(req.Args) {
			return 0, fmt.Errorf("%s: missing argument %d", req.Fn, i)
		}
		var n int
		if err := json.Unmarshal(req.Args[i], &n); err != nil {
			return 0, fmt.Errorf("%s: argument %d is not a number", req.Fn, i)
		}
		return n, nil
	}

	switch req.Fn {
	case "describe":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		doc, dir, err := readDoc(fixture(rel))
		if err != nil {
			return nil, "", err
		}
		desc, err := describe(doc, dir)
		return desc, "", err

	case "xwrite":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		if err := writeGLB(rel); err != nil {
			return nil, "", err
		}
		return map[string]any{"wrote": true, "format": "glb"}, "", nil

	case "xread":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		selfDoc, selfDir, err := readDoc(outPath(rel, lang))
		if err != nil {
			return nil, "", fmt.Errorf("reading own output: %w", err)
		}
		selfDesc, err := describe(selfDoc, selfDir)
		if err != nil {
			return nil, "", fmt.Errorf("describing own output: %w", err)
		}
		crossDoc, crossDir, err := readDoc(outPath(rel, other))
		if err != nil {
			return nil, "", fmt.Errorf("reading %s output: %w", other, err)
		}
		crossDesc, err := describe(crossDoc, crossDir)
		if err != nil {
			return nil, "", fmt.Errorf("describing %s output: %w", other, err)
		}
		return map[string]any{
			"self":            selfDesc,
			"cross":           crossDesc,
			"selfEqualsCross": canonical(selfDesc) == canonical(crossDesc),
		}, "", nil

	case "decodeAccessor":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		index, err := num(1)
		if err != nil {
			return nil, "", err
		}
		doc, _, err := readDoc(fixture(rel))
		if err != nil {
			return nil, "", err
		}
		values, err := decodeAccessor(doc, index)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"values": values}, "", nil

	case "accessorCount":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		doc, _, err := readDoc(fixture(rel))
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"count": len(doc.Accessors)}, "", nil

	case "validate":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		doc, _, err := readDoc(fixture(rel))
		if err != nil {
			return map[string]any{"valid": false}, "parse: " + err.Error(), nil
		}
		if err := doc.Validate(); err != nil {
			return map[string]any{"valid": false}, oneLine(err.Error()), nil
		}
		return map[string]any{"valid": true}, "", nil

	case "parses":
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		if _, _, err := readDoc(fixture(rel)); err != nil {
			return nil, "", err
		}
		return map[string]any{"parses": true}, "", nil

	case "emit":
		// emit writes the fixture back out in the requested container so the
		// official validator can grade the port's own output.
		rel, err := str(0)
		if err != nil {
			return nil, "", err
		}
		format, err := str(1)
		if err != nil {
			return nil, "", err
		}
		name, err := emitAsset(rel, format)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"file": name}, "", nil
	}
	return nil, "", fmt.Errorf("unknown fn %s", req.Fn)
}

// oneLine flattens a multi-line error (Validate returns one line per problem)
// so it fits in the single-line `note` field.
func oneLine(s string) string {
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// ------------------------------------------------------------------- file I/O

func fixture(rel string) string { return filepath.Join(fixturesDir, rel) }

func outPath(rel, which string) string {
	base := unsafeChars.ReplaceAllString(filepath.Base(rel), "_")
	return filepath.Join(outDir, base+"."+which+".glb")
}

// readDoc opens a .gltf or .glb asset with buffers resolved, returning the
// document and the directory external references resolve against.
func readDoc(path string) (*gltf.Document, string, error) {
	dir := filepath.Dir(path)
	if strings.HasSuffix(strings.ToLower(path), ".glb") {
		doc, err := gltf.OpenGLB(path)
		return doc, dir, err
	}
	doc, err := gltf.Open(path)
	return doc, dir, err
}

// embedBuffers rewrites the document so its single buffer is carried by the GLB
// BIN chunk, returning the chunk bytes.
func binChunk(doc *gltf.Document) ([]byte, error) {
	if len(doc.Buffers) == 0 {
		return nil, nil
	}
	if len(doc.Buffers) > 1 {
		return nil, fmt.Errorf("GLB requires a single buffer, document has %d", len(doc.Buffers))
	}
	data := doc.Buffers[0].Data
	doc.Buffers[0].URI = ""
	doc.Buffers[0].ByteLength = len(data)
	return data, nil
}

func writeGLB(rel string) error {
	doc, _, err := readDoc(fixture(rel))
	if err != nil {
		return err
	}
	bin, err := binChunk(doc)
	if err != nil {
		return err
	}
	return gltf.SaveGLB(outPath(rel, lang), doc, bin)
}

// emitAsset re-encodes a fixture as a self-contained .glb or .gltf under the
// shared output directory, and returns the file's base name.
func emitAsset(rel, format string) (string, error) {
	doc, _, err := readDoc(fixture(rel))
	if err != nil {
		return "", err
	}
	base := unsafeChars.ReplaceAllString(filepath.Base(rel), "_")
	switch format {
	case "glb":
		bin, err := binChunk(doc)
		if err != nil {
			return "", err
		}
		name := base + ".emit.go.glb"
		return name, gltf.SaveGLB(filepath.Join(outDir, name), doc, bin)
	case "gltf":
		if len(doc.Buffers) == 1 {
			doc.Buffers[0].URI = gltf.EncodeDataURI(doc.Buffers[0].Data)
			doc.Buffers[0].ByteLength = len(doc.Buffers[0].Data)
		}
		name := base + ".emit.go.gltf"
		return name, gltf.Save(filepath.Join(outDir, name), doc)
	}
	return "", fmt.Errorf("unknown format %q", format)
}

// ---------------------------------------------------------------- describe

// canonical renders a description as JSON with sorted keys, for the
// self-vs-cross equality check inside this runner.
func canonical(v any) string {
	enc, err := json.Marshal(v)
	if err != nil {
		return "<unencodable>"
	}
	return string(enc)
}

func idx(p *gltf.Index) float64 {
	if p == nil {
		return absent
	}
	return float64(*p)
}

func idxList(v []gltf.Index) []float64 {
	out := make([]float64, 0, len(v))
	for _, i := range v {
		out = append(out, float64(i))
	}
	return out
}

func floats(v []float64) []float64 {
	if v == nil {
		return []float64{}
	}
	return v
}

// describe renders the document into the implementation-independent structure
// the harness compares, resolving every glTF default explicitly.
func describe(doc *gltf.Document, baseDir string) (map[string]any, error) {
	nodes := make([]any, 0, len(doc.Nodes))
	for i := range doc.Nodes {
		n := &doc.Nodes[i]
		m := n.LocalMatrix()
		light, err := describeNodeLight(doc, n)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, map[string]any{
			"name":     n.Name,
			"matrix":   m[:],
			"children": idxList(n.Children),
			"mesh":     idx(n.Mesh),
			"camera":   idx(n.Camera),
			"skin":     idx(n.Skin),
			"weights":  floats(n.Weights),
			"light":    light,
		})
	}

	meshes := make([]any, 0, len(doc.Meshes))
	for i := range doc.Meshes {
		mesh := &doc.Meshes[i]
		prims := make([]any, 0, len(mesh.Primitives))
		for j := range mesh.Primitives {
			p := &mesh.Primitives[j]
			targets := make([]any, 0, len(p.Targets))
			for _, t := range p.Targets {
				targets = append(targets, attributePairs(t))
			}
			prims = append(prims, map[string]any{
				"mode":       p.GetMode().String(),
				"attributes": attributePairs(p.Attributes),
				"indices":    idx(p.Indices),
				"material":   idx(p.Material),
				"targets":    targets,
			})
		}
		meshes = append(meshes, map[string]any{
			"name":       mesh.Name,
			"weights":    floats(mesh.Weights),
			"primitives": prims,
		})
	}

	accessors := make([]any, 0, len(doc.Accessors))
	for i := range doc.Accessors {
		a := &doc.Accessors[i]
		min, max, err := accessorBounds(doc, i)
		if err != nil {
			return nil, fmt.Errorf("accessor %d: %w", i, err)
		}
		accessors = append(accessors, map[string]any{
			"componentType": int(a.ComponentType),
			"type":          string(a.Type),
			"count":         a.Count,
			"normalized":    a.Normalized,
			"elementSize":   a.Type.ComponentCount(),
			"min":           min,
			"max":           max,
		})
	}

	materials := make([]any, 0, len(doc.Materials))
	for i := range doc.Materials {
		m, err := describeMaterial(doc, &doc.Materials[i])
		if err != nil {
			return nil, fmt.Errorf("material %d: %w", i, err)
		}
		materials = append(materials, m)
	}

	textures := make([]any, 0, len(doc.Textures))
	for i := range doc.Textures {
		t, err := describeTexture(doc, i, baseDir)
		if err != nil {
			return nil, fmt.Errorf("texture %d: %w", i, err)
		}
		textures = append(textures, t)
	}

	scenes := make([]any, 0, len(doc.Scenes))
	for i := range doc.Scenes {
		scenes = append(scenes, map[string]any{
			"name":  doc.Scenes[i].Name,
			"nodes": idxList(doc.Scenes[i].Nodes),
		})
	}

	animations := make([]any, 0, len(doc.Animations))
	for i := range doc.Animations {
		a := &doc.Animations[i]
		channels := make([]any, 0, len(a.Channels))
		for _, c := range a.Channels {
			channels = append(channels, map[string]any{
				"node":    idx(c.Target.Node),
				"path":    string(c.Target.Path),
				"sampler": float64(c.Sampler),
			})
		}
		samplers := make([]any, 0, len(a.Samplers))
		for j := range a.Samplers {
			s := &a.Samplers[j]
			samplers = append(samplers, map[string]any{
				"input":         float64(s.Input),
				"output":        float64(s.Output),
				"interpolation": string(s.GetInterpolation()),
			})
		}
		animations = append(animations, map[string]any{
			"name":     a.Name,
			"channels": channels,
			"samplers": samplers,
		})
	}

	skins := make([]any, 0, len(doc.Skins))
	for i := range doc.Skins {
		s := &doc.Skins[i]
		skins = append(skins, map[string]any{
			"name":                s.Name,
			"joints":              idxList(s.Joints),
			"skeleton":            idx(s.Skeleton),
			"inverseBindMatrices": idx(s.InverseBindMatrices),
		})
	}

	cameras := make([]any, 0, len(doc.Cameras))
	for i := range doc.Cameras {
		cameras = append(cameras, describeCamera(&doc.Cameras[i]))
	}

	used := append([]string{}, doc.ExtensionsUsed...)
	sort.Strings(used)
	required := append([]string{}, doc.ExtensionsRequired...)
	sort.Strings(required)

	return map[string]any{
		"asset":              map[string]any{"version": doc.Asset.Version},
		"extensionsUsed":     used,
		"extensionsRequired": required,
		"defaultScene":       idx(doc.Scene),
		"scenes":             scenes,
		"nodes":              nodes,
		"meshes":             meshes,
		"accessors":          accessors,
		"materials":          materials,
		"textures":           textures,
		"animations":         animations,
		"skins":              skins,
		"cameras":            cameras,
	}, nil
}

// attributePairs renders an attribute map as a name-sorted list of
// [semantic, accessorIndex] pairs, which is order-stable across languages.
func attributePairs(attrs map[string]gltf.Index) []any {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, []any{k, float64(attrs[k])})
	}
	return out
}

// decodeAccessor returns every component of the accessor as float64. Unsigned
// integer accessors go through DecodeAccessorUint32 so values above 2^24 keep
// their exact value; everything else uses the general float decoder, which
// applies the normalization rule.
func decodeAccessor(doc *gltf.Document, index int) ([]float64, error) {
	if index < 0 || index >= len(doc.Accessors) {
		return nil, fmt.Errorf("no accessor at index %d", index)
	}
	a := &doc.Accessors[index]
	if a.ComponentType == gltf.ComponentUnsignedInt && !a.Normalized {
		raw, err := doc.DecodeAccessorUint32(index)
		if err != nil {
			return nil, err
		}
		out := make([]float64, len(raw))
		for i, v := range raw {
			out[i] = float64(v)
		}
		return out, nil
	}
	raw, err := doc.DecodeAccessorFloat32(index)
	if err != nil {
		return nil, err
	}
	out := make([]float64, len(raw))
	for i, v := range raw {
		out[i] = float64(v)
	}
	return out, nil
}

// accessorBounds returns the per-component min and max of the accessor's
// decoded values. The declared min/max are deliberately not used:
// @gltf-transform only writes them for POSITION and animation inputs, so they
// cannot survive a round-trip comparison.
func accessorBounds(doc *gltf.Document, index int) ([]float64, []float64, error) {
	values, err := decodeAccessor(doc, index)
	if err != nil {
		return nil, nil, err
	}
	comps := doc.Accessors[index].Type.ComponentCount()
	if comps == 0 {
		return nil, nil, fmt.Errorf("unknown accessor type %q", doc.Accessors[index].Type)
	}
	min := make([]float64, comps)
	max := make([]float64, comps)
	for j := 0; j < comps; j++ {
		min[j] = math.Inf(1)
		max[j] = math.Inf(-1)
	}
	for i := 0; i+comps <= len(values); i += comps {
		for j := 0; j < comps; j++ {
			v := values[i+j]
			if v < min[j] {
				min[j] = v
			}
			if v > max[j] {
				max[j] = v
			}
		}
	}
	return min, max, nil
}

func describeCamera(c *gltf.Camera) map[string]any {
	out := map[string]any{
		"name":        c.Name,
		"type":        string(c.Type),
		"znear":       absent,
		"zfar":        absent,
		"yfov":        absent,
		"aspectRatio": absent,
		"xmag":        absent,
		"ymag":        absent,
	}
	if p := c.Perspective; p != nil && c.Type == gltf.CameraTypePerspective {
		out["znear"] = p.ZNear
		out["yfov"] = p.YFOV
		if p.ZFar != nil {
			out["zfar"] = *p.ZFar
		}
		if p.AspectRatio != nil {
			out["aspectRatio"] = *p.AspectRatio
		}
	}
	if o := c.Orthographic; o != nil && c.Type == gltf.CameraTypeOrthographic {
		out["znear"] = o.ZNear
		out["zfar"] = o.ZFar
		out["xmag"] = o.XMag
		out["ymag"] = o.YMag
	}
	return out
}

func describeNodeLight(doc *gltf.Document, n *gltf.Node) (any, error) {
	li, ok := n.NodeLight()
	if !ok {
		return nil, nil
	}
	lights, present := doc.Lights()
	if !present || li < 0 || li >= len(lights) {
		return nil, fmt.Errorf("node references light %d but the document has no such light", li)
	}
	l := lights[li]
	color := []float64{1, 1, 1}
	if l.Color != nil {
		color = []float64{l.Color[0], l.Color[1], l.Color[2]}
	}
	intensity := 1.0
	if l.Intensity != nil {
		intensity = *l.Intensity
	}
	rng := absent
	if l.Range != nil {
		rng = *l.Range
	}
	inner, outer := 0.0, math.Pi/4
	if l.Spot != nil {
		inner = l.Spot.InnerConeAngle
		if l.Spot.OuterConeAngle != nil {
			outer = *l.Spot.OuterConeAngle
		}
	}
	return map[string]any{
		"type":           string(l.Type),
		"color":          color,
		"intensity":      intensity,
		"range":          rng,
		"innerConeAngle": inner,
		"outerConeAngle": outer,
	}, nil
}

func describeTexture(doc *gltf.Document, index int, baseDir string) (map[string]any, error) {
	t := &doc.Textures[index]
	out := map[string]any{"mimeType": "", "sha256": "", "width": absent, "height": absent}
	if t.Source == nil {
		return out, nil
	}
	raw, mime, err := doc.ImageBytes(*t.Source, baseDir)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	out["sha256"] = hex.EncodeToString(sum[:])
	if declared := doc.Images[*t.Source].MimeType; declared != "" {
		out["mimeType"] = declared
	} else {
		out["mimeType"] = mime
	}
	img, _, err := doc.DecodeImage(*t.Source, baseDir)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	out["width"] = float64(b.Dx())
	out["height"] = float64(b.Dy())
	return out, nil
}

// samplerOf returns the sampler bound to a texture, or nil when the texture
// leaves filtering and wrapping to the runtime.
func samplerOf(doc *gltf.Document, texture gltf.Index) *gltf.Sampler {
	if texture < 0 || texture >= len(doc.Textures) {
		return nil
	}
	s := doc.Textures[texture].Sampler
	if s == nil || *s < 0 || *s >= len(doc.Samplers) {
		return nil
	}
	return &doc.Samplers[*s]
}

// describeTextureInfo mirrors the upstream shape: @gltf-transform folds the
// document-level sampler into each texture reference, so the sampler's filters
// and wrap modes are reported per reference here too.
func describeTextureInfo(doc *gltf.Document, ti *gltf.TextureInfo, extra map[string]any) any {
	if ti == nil {
		return nil
	}
	out := map[string]any{
		"texture":   float64(ti.Index),
		"texCoord":  float64(ti.TexCoord),
		"magFilter": absent,
		"minFilter": absent,
		"wrapS":     absent,
		"wrapT":     absent,
		"transform": nil,
	}
	if s := samplerOf(doc, ti.Index); s != nil {
		if s.MagFilter != nil {
			out["magFilter"] = float64(*s.MagFilter)
		}
		if s.MinFilter != nil {
			out["minFilter"] = float64(*s.MinFilter)
		}
		if s.WrapS != nil {
			out["wrapS"] = float64(*s.WrapS)
		}
		if s.WrapT != nil {
			out["wrapT"] = float64(*s.WrapT)
		}
	}
	if tf, ok := ti.TextureTransform(); ok {
		offset := []float64{0, 0}
		if tf.Offset != nil {
			offset = []float64{tf.Offset[0], tf.Offset[1]}
		}
		scale := []float64{1, 1}
		if tf.Scale != nil {
			scale = []float64{tf.Scale[0], tf.Scale[1]}
		}
		tc := absent
		if tf.TexCoord != nil {
			tc = float64(*tf.TexCoord)
		}
		out["transform"] = map[string]any{
			"offset":   offset,
			"rotation": tf.Rotation,
			"scale":    scale,
			"texCoord": tc,
		}
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func describeMaterial(doc *gltf.Document, m *gltf.Material) (map[string]any, error) {
	baseColor := []float64{1, 1, 1, 1}
	metallic, roughness := 1.0, 1.0
	var baseColorTex, mrTex any
	if p := m.PBRMetallicRoughness; p != nil {
		if p.BaseColorFactor != nil {
			baseColor = []float64{p.BaseColorFactor[0], p.BaseColorFactor[1], p.BaseColorFactor[2], p.BaseColorFactor[3]}
		}
		if p.MetallicFactor != nil {
			metallic = *p.MetallicFactor
		}
		if p.RoughnessFactor != nil {
			roughness = *p.RoughnessFactor
		}
		baseColorTex = describeTextureInfo(doc, p.BaseColorTexture, nil)
		mrTex = describeTextureInfo(doc, p.MetallicRoughnessTexture, nil)
	}

	emissive := []float64{0, 0, 0}
	if m.EmissiveFactor != nil {
		emissive = []float64{m.EmissiveFactor[0], m.EmissiveFactor[1], m.EmissiveFactor[2]}
	}
	alphaMode := string(m.AlphaMode)
	if alphaMode == "" {
		alphaMode = string(gltf.AlphaOpaque)
	}
	alphaCutoff := 0.5
	if m.AlphaCutoff != nil {
		alphaCutoff = *m.AlphaCutoff
	}

	var normalTex, occlusionTex any
	if nt := m.NormalTexture; nt != nil {
		scale := 1.0
		if nt.Scale != nil {
			scale = *nt.Scale
		}
		normalTex = describeTextureInfo(doc,
			&gltf.TextureInfo{Index: nt.Index, TexCoord: nt.TexCoord, Extensions: nt.Extensions},
			map[string]any{"scale": scale})
	}
	if ot := m.OcclusionTexture; ot != nil {
		strength := 1.0
		if ot.Strength != nil {
			strength = *ot.Strength
		}
		occlusionTex = describeTextureInfo(doc,
			&gltf.TextureInfo{Index: ot.Index, TexCoord: ot.TexCoord, Extensions: ot.Extensions},
			map[string]any{"strength": strength})
	}

	emissiveStrength, _ := m.EmissiveStrength()
	ior, _ := m.IOR()
	transmissionFactor := 0.0
	if tr, ok := m.Transmission(); ok {
		transmissionFactor = tr.TransmissionFactor
	}
	clearcoatFactor, clearcoatRoughness := 0.0, 0.0
	if cc, ok := m.Clearcoat(); ok {
		clearcoatFactor = cc.ClearcoatFactor
		clearcoatRoughness = cc.ClearcoatRoughnessFactor
	}
	sheenColor := []float64{0, 0, 0}
	sheenRoughness := 0.0
	if sh, ok := m.Sheen(); ok {
		if sh.SheenColorFactor != nil {
			sheenColor = []float64{sh.SheenColorFactor[0], sh.SheenColorFactor[1], sh.SheenColorFactor[2]}
		}
		sheenRoughness = sh.SheenRoughnessFactor
	}
	specularFactor := 1.0
	specularColor := []float64{1, 1, 1}
	if sp, ok := m.Specular(); ok {
		if sp.SpecularFactor != nil {
			specularFactor = *sp.SpecularFactor
		}
		if sp.SpecularColorFactor != nil {
			specularColor = []float64{sp.SpecularColorFactor[0], sp.SpecularColorFactor[1], sp.SpecularColorFactor[2]}
		}
	}
	thickness := 0.0
	attenuationDistance := absent
	attenuationColor := []float64{1, 1, 1}
	if vol, ok := m.Volume(); ok {
		thickness = vol.ThicknessFactor
		if vol.AttenuationDistance != nil {
			attenuationDistance = *vol.AttenuationDistance
		}
		if vol.AttenuationColor != nil {
			attenuationColor = []float64{vol.AttenuationColor[0], vol.AttenuationColor[1], vol.AttenuationColor[2]}
		}
	}

	return map[string]any{
		"name":                     m.Name,
		"alphaMode":                alphaMode,
		"alphaCutoff":              alphaCutoff,
		"doubleSided":              m.DoubleSided,
		"baseColorFactor":          baseColor,
		"metallicFactor":           metallic,
		"roughnessFactor":          roughness,
		"emissiveFactor":           emissive,
		"baseColorTexture":         baseColorTex,
		"metallicRoughnessTexture": mrTex,
		"emissiveTexture":          describeTextureInfo(doc, m.EmissiveTexture, nil),
		"normalTexture":            normalTex,
		"occlusionTexture":         occlusionTex,
		"ext": map[string]any{
			"unlit":                     m.Unlit(),
			"emissiveStrength":          emissiveStrength,
			"ior":                       ior,
			"transmissionFactor":        transmissionFactor,
			"clearcoatFactor":           clearcoatFactor,
			"clearcoatRoughnessFactor":  clearcoatRoughness,
			"sheenColorFactor":          sheenColor,
			"sheenRoughnessFactor":      sheenRoughness,
			"specularFactor":            specularFactor,
			"specularColorFactor":       specularColor,
			"volumeThicknessFactor":     thickness,
			"volumeAttenuationDistance": attenuationDistance,
			"volumeAttenuationColor":    attenuationColor,
		},
	}, nil
}
