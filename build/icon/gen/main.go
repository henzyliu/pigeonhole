// Command gen renders build/icon/appicon.svg into the app icon files:
// build/appicon.png (1024px, which Wails turns into the macOS .icns) and
// build/windows/icon.ico (16-256px), plus PNG previews for checking.
//
// Run from the repo root:
//
//	go run -C build/icon/gen . -svg ../appicon.svg -out ../..
//
// It renders once at 1024px and downsamples, since oksvg mis-scales
// userSpaceOnUse gradients when rendering directly at small sizes.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
)

const designSize = 1024

var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	svgPath := flag.String("svg", "../appicon.svg", "icon source")
	outDir := flag.String("out", "../..", "the build/ directory")
	preview := flag.String("preview", "", "if set, also write PNG previews (16-1024px) to this directory")
	flag.Parse()

	full, err := render(*svgPath)
	if err != nil {
		fail(err)
	}
	if err = writePNG(filepath.Join(*outDir, "appicon.png"), full); err != nil {
		fail(err)
	}
	var entries [][]byte
	for _, size := range icoSizes {
		var buf bytes.Buffer
		if err = png.Encode(&buf, scale(full, size)); err != nil {
			fail(err)
		}
		entries = append(entries, buf.Bytes())
	}
	if err = os.WriteFile(filepath.Join(*outDir, "windows", "icon.ico"), packICO(icoSizes, entries), 0o644); err != nil {
		fail(err)
	}
	if *preview != "" {
		for _, size := range []int{16, 32, 64, 128, 256, 1024} {
			if err = writePNG(filepath.Join(*preview, fmt.Sprintf("icon_%d.png", size)), scale(full, size)); err != nil {
				fail(err)
			}
		}
	}
}

func render(path string) (*image.RGBA, error) {
	icon, err := oksvg.ReadIcon(path, oksvg.StrictErrorMode)
	if err != nil {
		return nil, err
	}
	icon.SetTarget(0, 0, designSize, designSize)
	img := image.NewRGBA(image.Rect(0, 0, designSize, designSize))
	icon.Draw(rasterx.NewDasher(designSize, designSize, rasterx.NewScannerGV(designSize, designSize, img, img.Bounds())), 1)
	return img, nil
}

func scale(src *image.RGBA, size int) *image.RGBA {
	if size == designSize {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// packICO builds a .ico with PNG-compressed entries (supported since Vista).
func packICO(sizes []int, pngs [][]byte) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(pngs))})
	offset := 6 + 16*len(pngs)
	for i, data := range pngs {
		dim := byte(sizes[i])
		if sizes[i] >= 256 {
			dim = 0 // 0 means 256 in the ICO format
		}
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, le, [2]uint16{1, 32})
		binary.Write(&buf, le, [2]uint32{uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range pngs {
		buf.Write(data)
	}
	return buf.Bytes()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
