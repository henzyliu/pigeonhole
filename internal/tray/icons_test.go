package tray

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func loadAppIcon(t *testing.T) image.Image {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "build", "appicon.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(img image.Image, x, y int) bool {
	r, g, b, a := img.At(x, y).RGBA()
	return a > 0xF000 && r > 0xC000 && g < 0x6000 && b < 0x6000
}

// The tray icon is the app icon at tray size, with a red dot in the corner
// only when there's something unread.
func TestTrayIcon(t *testing.T) {
	src := loadAppIcon(t)
	for _, size := range []int{16, 20, 24, 32} {
		plain, err := trayIconPNG(src, size, false)
		if err != nil {
			t.Fatal(err)
		}
		dotted, err := trayIconPNG(src, size, true)
		if err != nil {
			t.Fatal(err)
		}
		p, d := decode(t, plain), decode(t, dotted)
		if p.Bounds().Dx() != size || d.Bounds().Dy() != size {
			t.Fatalf("%d: wrong size %v", size, p.Bounds())
		}
		cx, cy := size-size/4, size-size/4
		if isRed(p, cx, cy) {
			t.Errorf("%d: plain icon shouldn't have the dot", size)
		}
		if !isRed(d, cx, cy) {
			t.Errorf("%d: expected a red dot at (%d,%d), got %v", size, cx, cy, d.At(cx, cy))
		}
		if !isRed(d, cx, cy) || isRed(d, size/4, size/4) {
			t.Errorf("%d: the dot should only be in the corner", size)
		}
	}
	if dir := os.Getenv("PH_ICON_OUT"); dir != "" {
		for _, dot := range []bool{false, true} {
			data, _ := trayIconPNG(src, 32, dot)
			name := map[bool]string{false: "tray.png", true: "tray-unread.png"}[dot]
			_ = os.WriteFile(filepath.Join(dir, name), data, 0o644)
		}
		for _, n := range []int{3, 12} {
			data, _ := badgePNG(n, 32)
			_ = os.WriteFile(filepath.Join(dir, "badge-"+map[int]string{3: "3", 12: "12"}[n]+".png"), data, 0o644)
		}
	}
}

// The taskbar badge is a red disc with a white count, "9+" above nine.
func TestBadge(t *testing.T) {
	for _, n := range []int{1, 7, 12} {
		data, err := badgePNG(n, 16)
		if err != nil {
			t.Fatal(err)
		}
		img := decode(t, data)
		white := 0
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				if r, g, b, a := img.At(x, y).RGBA(); a > 0xF000 && r > 0xE000 && g > 0xE000 && b > 0xE000 {
					white++
				}
			}
		}
		if !isRed(img, 2, 8) || white == 0 {
			t.Errorf("%d: expected a red disc with white digits (%d white pixels)", n, white)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%d: corners should be transparent", n)
		}
	}
}
