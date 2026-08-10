// Command gen writes the committed image fixtures used by both parity runners.
//
// Both runners read the same bytes off disk, so the image inputs are identical
// on either side of the comparison. Regenerate with:
//
//	GOWORK=off go run ./fixtures/gen
//
// The output is deterministic: every pixel is computed from a fixed formula and
// the encoders are configured with fixed options.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	out := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}

	// 8x8 opaque RGB gradient.
	rgb := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			rgb.Set(x, y, color.NRGBA{R: uint8(x * 32), G: uint8(y * 32), B: 128, A: 255})
		}
	}
	writePNG(filepath.Join(out, "rgb8.png"), rgb)

	// 8x8 RGBA with a hard alpha ramp, to exercise the soft-mask path.
	rgba := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			rgba.Set(x, y, color.NRGBA{R: 255, G: uint8(x * 30), B: uint8(y * 30), A: uint8(x * 36)})
		}
	}
	writePNG(filepath.Join(out, "rgba8.png"), rgba)

	// 8x8 greyscale.
	gray := image.NewGray(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			gray.SetGray(x, y, color.Gray{Y: uint8((x + y) * 16)})
		}
	}
	writePNG(filepath.Join(out, "gray8.png"), gray)

	// 16x16 baseline JPEG at a fixed quality.
	jsrc := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			jsrc.Set(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(255 - y*16), B: uint8((x ^ y) * 16), A: 255})
		}
	}
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, jsrc, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	write(filepath.Join(out, "rgb16.jpg"), jb.Bytes())
}

func writePNG(path string, img image.Image) {
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&b, img); err != nil {
		panic(err)
	}
	write(path, b.Bytes())
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", path, len(data))
}
