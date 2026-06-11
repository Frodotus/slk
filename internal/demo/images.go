package demo

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // register the JPEG decoder for the embedded sample

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// sampleJPEG is the stock photo used as the demo inline image. Source:
// Unsplash (via Lorem Picsum), free to use under the Unsplash License.
// Swap internal/demo/sample.jpg to change the hero-shot photo.
//
//go:embed sample.jpg
var sampleJPEG []byte

// InlineImage returns the embedded stock photo for the demo inline image,
// falling back to a generated graphic if the embedded file fails to
// decode (so --demo never hard-fails on the image).
func InlineImage() image.Image {
	if img, _, err := image.Decode(bytes.NewReader(sampleJPEG)); err == nil {
		return img
	}
	return GenerateInlineImage(640, 400)
}

// GenerateAvatar renders a square avatar: a solid color background with
// the person's initials in white, centered and scaled up. Pure in-memory;
// no fetching. size is the pixel side length.
func GenerateAvatar(initials string, c color.RGBA, size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	if initials == "" {
		return img
	}

	// Render the initials with the built-in bitmap font into a tight
	// buffer, then scale that buffer up to ~52% of the avatar height so
	// the letters are proportional regardless of `size`.
	face := basicfont.Face7x13
	tw := font.MeasureString(face, initials).Ceil()
	th := face.Metrics().Ascent.Ceil() + face.Metrics().Descent.Ceil()
	if tw <= 0 || th <= 0 {
		return img
	}
	buf := image.NewRGBA(image.Rect(0, 0, tw, th))
	d := &font.Drawer{
		Dst:  buf,
		Src:  image.NewUniform(color.White),
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(initials)

	scale := float64(size) * 0.52 / float64(th)
	dw, dh := int(float64(tw)*scale), int(float64(th)*scale)
	ox, oy := (size-dw)/2, (size-dh)/2
	dst := image.Rect(ox, oy, ox+dw, oy+dh)
	xdraw.CatmullRom.Scale(img, dst, buf, buf.Bounds(), xdraw.Over, nil)
	return img
}

// GenerateInlineImage returns a clean sample graphic standing in for an
// uploaded screenshot: a vertical gradient with a few "UI" bars, so the
// inline-image feature is visible in the hero shot.
func GenerateInlineImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Vertical gradient from a deep indigo to a lighter slate.
	top := color.RGBA{0x1A, 0x1A, 0x2E, 0xFF}
	bot := color.RGBA{0x33, 0x37, 0x4A, 0xFF}
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		row := color.RGBA{
			R: lerp(top.R, bot.R, t),
			G: lerp(top.G, bot.G, t),
			B: lerp(top.B, bot.B, t),
			A: 0xFF,
		}
		draw.Draw(img, image.Rect(0, y, w, y+1), &image.Uniform{row}, image.Point{}, draw.Src)
	}
	// A few rounded-ish bars suggesting a sidebar / list.
	bars := []color.RGBA{
		{0x4A, 0x9E, 0xFF, 0xFF}, {0x50, 0xC8, 0x78, 0xFF},
		{0xC6, 0x78, 0xDD, 0xFF}, {0xE0, 0x6C, 0x75, 0xFF},
	}
	bx, bw := w/12, w/2
	by, bh, gap := h/5, h/12, h/10
	for i, c := range bars {
		y0 := by + i*gap
		draw.Draw(img, image.Rect(bx, y0, bx+bw-i*w/16, y0+bh), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	return img
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}
