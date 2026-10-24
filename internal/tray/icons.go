package tray

// Unread indicators for Windows, where the notification-area icon and the
// taskbar button show them as images (macOS uses the Dock badge and menu bar
// text instead). Drawn here as PNGs in plain Go, so they can be tested
// anywhere; Windows turns the PNGs into icons.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strconv"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var (
	badgeRed  = color.NRGBA{R: 0xE5, G: 0x39, B: 0x35, A: 0xFF}
	badgeRing = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// trayIconPNG is the app icon at size×size pixels, with a red dot in the
// bottom-right corner when dot is set. The dot has a white ring so it reads
// on light and dark taskbars.
func trayIconPNG(src image.Image, size int, dot bool) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), draw.Over, nil)
	if dot {
		r := float64(size) * 0.25
		cx, cy := float64(size)-r, float64(size)-r
		fillCircle(img, cx, cy, r, badgeRing)
		fillCircle(img, cx, cy, r-max(1, float64(size)/16), badgeRed)
	}
	return encodePNG(img)
}

// badgePNG is the taskbar overlay: a red disc with the count in white
// ("9+" above nine), size×size pixels.
func badgePNG(count, size int) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	half := float64(size) / 2
	fillCircle(img, half, half, half, badgeRed)
	label := strconv.Itoa(count)
	if count > 9 {
		label = "9+"
	}
	face := basicfont.Face7x13
	d := font.Drawer{Dst: img, Src: image.NewUniform(badgeRing), Face: face}
	width := d.MeasureString(label).Ceil()
	// basicfont's 13px line has its baseline at 11 and the digits' tops at 2.
	d.Dot = fixed.P((size-width)/2, (size-9)/2+9)
	d.DrawString(label)
	return encodePNG(img)
}

// fillCircle paints an anti-aliased disc over img (4×4 samples per pixel).
func fillCircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	const samples = 4
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			inside := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					dx := float64(x) + (float64(sx)+0.5)/samples - cx
					dy := float64(y) + (float64(sy)+0.5)/samples - cy
					if dx*dx+dy*dy <= r*r {
						inside++
					}
				}
			}
			if inside == 0 {
				continue
			}
			alpha := float64(inside) / samples / samples
			blend(img, x, y, c, alpha)
		}
	}
}

// blend lays c over the pixel at (x, y) with the given coverage.
func blend(img *image.NRGBA, x, y int, c color.NRGBA, coverage float64) {
	under := img.NRGBAAt(x, y)
	a := coverage * float64(c.A) / 255
	ua := float64(under.A) / 255
	outA := a + ua*(1-a)
	if outA == 0 {
		return
	}
	mix := func(top, bottom uint8) uint8 {
		return uint8((float64(top)*a + float64(bottom)*ua*(1-a)) / outA)
	}
	img.SetNRGBA(x, y, color.NRGBA{R: mix(c.R, under.R), G: mix(c.G, under.G), B: mix(c.B, under.B), A: uint8(outA * 255)})
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
