package emoji

import (
	"context"
	"errors"
	goimage "image"
	"io"
	"sync"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// fakeFetcher implements PlaceFetcher for unit tests. Behavior is
// controlled by the prerender map (warm hits) and a fetchFn closure
// (cold-path fetch behavior).
type fakeFetcher struct {
	mu         sync.Mutex
	prerender  map[string]imgpkg.Render // keyed by "<key>|<cx>x<cy>"
	fetchFn    func(ctx context.Context, req imgpkg.FetchRequest) (imgpkg.FetchResult, error)
	fetchCalls []imgpkg.FetchRequest
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{prerender: map[string]imgpkg.Render{}}
}

func (f *fakeFetcher) prerenderKey(key string, t goimage.Point) string {
	return key + "|" + itoa(t.X) + "x" + itoa(t.Y)
}

func itoa(n int) string {
	// avoid strconv import here for brevity in tests
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (f *fakeFetcher) setPrerendered(key string, target goimage.Point, r imgpkg.Render) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prerender[f.prerenderKey(key, target)] = r
}

func (f *fakeFetcher) Prerendered(key string, t goimage.Point, proto imgpkg.Protocol) (imgpkg.Render, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.prerender[f.prerenderKey(key, t)]
	return r, ok
}

func (f *fakeFetcher) Fetch(ctx context.Context, req imgpkg.FetchRequest) (imgpkg.FetchResult, error) {
	f.mu.Lock()
	f.fetchCalls = append(f.fetchCalls, req)
	fn := f.fetchFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return imgpkg.FetchResult{}, errors.New("fakeFetcher: no fetchFn set")
}

func TestPlace_InvalidInputs(t *testing.T) {
	ff := newFakeFetcher()
	ctx := PlaceContext{Fetcher: ff}

	cases := []struct {
		name string
		url  string
		cell int
		fctx PlaceContext
	}{
		{"empty url", "", 2, ctx},
		{"zero cells", "https://x", 0, ctx},
		{"negative cells", "https://x", -1, ctx},
		{"nil fetcher", "https://x", 2, PlaceContext{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, flush, ok := Place(c.fctx, c.url, c.cell)
			if ok {
				t.Errorf("Place(%q, %d) = (%q, flush=%v, true), want ok=false", c.url, c.cell, s, flush != nil)
			}
			if s != "" {
				t.Errorf("Place(%q, %d) placement = %q, want \"\"", c.url, c.cell, s)
			}
		})
	}

	// The fetcher should not have been called for any of these inputs.
	if len(ff.fetchCalls) != 0 {
		t.Errorf("fetcher was called %d times for invalid inputs, want 0", len(ff.fetchCalls))
	}
}

func TestPlace_WarmPath_ReturnsKittyLine(t *testing.T) {
	ff := newFakeFetcher()
	url := "https://a.slack-edge.com/...1f44d.png"
	key := EmojiCacheKey(url)
	target := goimage.Pt(2, 1)

	// Seed a prerender hit: 2-cell-wide kitty placement string.
	wantLine := "\U0010EEEE\U0010EEEE" // two kitty placeholder runes (the real renderer emits this with diacritics + SGR fg; for the unit test, any deterministic placement string is fine)
	flushCalled := 0
	ff.setPrerendered(key, target, imgpkg.Render{
		Cells:    target,
		Lines:    []string{wantLine},
		Fallback: []string{wantLine},
		OnFlush: func(_ io.Writer) error {
			flushCalled++
			return nil
		},
	})

	ctx := PlaceContext{Fetcher: ff}
	got, flush, ok := Place(ctx, url, 2)
	if !ok {
		t.Fatalf("Place: ok=false, want true (warm path)")
	}
	if got != wantLine {
		t.Errorf("Place placement = %q, want %q", got, wantLine)
	}
	if flush == nil {
		t.Fatalf("Place: flush is nil, want a callback for the warm path")
	}
	// flush is io.Writer-shaped; call with a discarding writer to
	// verify it doesn't panic and increments the counter.
	if err := flush(discardWriter{}); err != nil {
		t.Errorf("flush returned err = %v, want nil", err)
	}
	if flushCalled != 1 {
		t.Errorf("flush invocation count = %d, want 1", flushCalled)
	}

	// No fetch goroutine should have been spawned on the warm path.
	if len(ff.fetchCalls) != 0 {
		t.Errorf("fetcher.Fetch called %d times on warm path, want 0", len(ff.fetchCalls))
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
