package thread

import (
	"bytes"
	"context"
	stdimage "image"
	imgcolor "image/color"
	imgpng "image/png"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
)

func makeThreadTestPNG(w, h int) []byte {
	src := stdimage.NewRGBA(stdimage.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.Set(x, y, imgcolor.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := imgpng.Encode(&buf, src); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestView_ParentImageEmitsKittyUpload is the regression for issue #4: a
// thread opened on a message that carries an inline image must upload the
// PARENT image to kitty. Before the fix the parent's image flushes were
// discarded, so the parent's kitty placeholder composited a stale/garbage
// image (often a recently-uploaded avatar) instead of the photo.
func TestView_ParentImageEmitsKittyUpload(t *testing.T) {
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	fetcher := imgpkg.NewFetcher(cache, nil)
	const fileID = "F0PARENT01"
	const key = fileID + "-720"
	if _, err := cache.Put(key, "png", makeThreadTestPNG(720, 720)); err != nil {
		t.Fatalf("cache.Put: %v", err)
	}
	// Prime the decoded memo so the renderer's Cached() lookup hits and
	// RenderBlock takes the kitty (flush-emitting) path. The square 720
	// thumb is MaxRows-constrained to a (320,320)px target.
	if _, err := fetcher.Fetch(context.Background(), imgpkg.FetchRequest{
		Key:    key,
		URL:    "unused://disk-cache-hits-skip-network",
		Target: stdimage.Pt(320, 320),
	}); err != nil {
		t.Fatalf("Fetch (memo prime): %v", err)
	}

	parent := messages.MessageItem{
		TS: "1700000000.000100", UserID: "U1", UserName: "alice",
		Text: "look at this", Timestamp: "10:30 AM",
		Attachments: []messages.Attachment{{
			Kind: "image", Name: "screenshot.png", URL: "https://example.com/perma",
			FileID: fileID, Mime: "image/png",
			Thumbs: []messages.ThumbSpec{{URL: "https://example.com/720.png", W: 720, H: 720}},
		}},
	}

	m := New()
	ctx := imgrender.ImageContext{
		Protocol:    imgpkg.ProtoKitty,
		Fetcher:     fetcher,
		CellPixels:  stdimage.Pt(8, 16),
		MaxRows:     20,
		KittyRender: imgpkg.NewKittyRenderer(imgpkg.NewRegistry()),
	}
	m.SetImageContext(ctx)
	m.SetThread(parent, nil, "C123", parent.TS) // thread on the image message, no replies

	saved := imgpkg.KittyOutput
	defer func() { imgpkg.KittyOutput = saved }()
	var buf bytes.Buffer
	imgpkg.KittyOutput = &buf

	_ = m.View(40, 80) // (height, width)
	if !bytes.Contains(buf.Bytes(), []byte("\x1b_G")) {
		t.Errorf("parent image did not emit a kitty upload escape (\\x1b_G); got %d bytes (issue #4 — parent flushes discarded)", buf.Len())
	}
}
