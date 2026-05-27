package emoji

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	goimage "image"
	"io"
	"strings"
	"sync"

	imgpkg "github.com/gammons/slk/internal/image"
)

// PlaceFetcher is the subset of *image.Fetcher's API that Place uses.
// Defined as an interface so tests can substitute a fake without
// constructing a full Fetcher + HTTP server.
type PlaceFetcher interface {
	Fetch(ctx context.Context, req imgpkg.FetchRequest) (imgpkg.FetchResult, error)
	Prerendered(key string, cellTarget goimage.Point, proto imgpkg.Protocol) (imgpkg.Render, bool)
}

// PlaceContext bundles the dependencies Place needs. Built once per
// app instance and passed to every UI surface that renders emoji.
type PlaceContext struct {
	// Fetcher downloads + decodes + (via ConfigurePrerender) pre-encodes
	// the kitty payload. Required.
	Fetcher PlaceFetcher

	// SendMsg dispatches an EmojiImageReadyMsg when a cold-cache fetch
	// completes. Typically wraps bubbletea Program.Send. If nil, the
	// fetch still runs but no re-render is signaled — useful in tests.
	SendMsg func(msg any)
}

// EmojiImageReadyMsg is dispatched via PlaceContext.SendMsg when a
// previously-cold emoji image has finished fetching and is now
// renderable from the warm path. UI surfaces that buffer emoji
// placements should invalidate their render caches for any entry
// referencing this URL.
//
// Unlike BlockImageReadyMsg (per-message), EmojiImageReadyMsg is
// global — the same emoji can appear in any message, the picker, the
// autocomplete dropdown, etc. Reducers should treat it as a
// coarse-grained invalidation signal across all surfaces that render
// emoji.
type EmojiImageReadyMsg struct {
	URL string
}

// inflightEmoji guards against firing multiple fetch goroutines for
// the same URL when the cold-cache branch is reached concurrently
// from multiple UI surfaces (e.g., the same :thumbsup: appearing in
// 10 messages on the same View() pass).
var (
	inflightEmoji   = map[string]struct{}{}
	inflightEmojiMu sync.Mutex
)

// EmojiCacheKey returns the cache key used by Place for url. Stable
// hash of the URL with an "E-" prefix to isolate emoji entries from
// avatars, attachments, and block-kit images in the shared disk cache.
//
// Exported so reducers can correlate EmojiImageReadyMsg URLs to cache
// entries when wiring re-render invalidation.
func EmojiCacheKey(url string) string {
	sum := sha1.Sum([]byte(url))
	return "E-" + hex.EncodeToString(sum[:8])
}

// Place returns the kitty placement string + optional flush callback
// for url at the given cell footprint (cells wide x 1 row tall).
//
// Returns ("", nil, false) when:
//   - url is empty
//   - ctx.Fetcher is nil
//   - cells < 1
//
// Warm path (image already fetched + prerendered for this cells target):
// returns (placement, flush, true). placement is exactly cells chars
// wide; flush MUST be invoked once per frame to upload the kitty
// payload (idempotent via the registry — multiple invocations are
// safe and the first wins).
//
// Cold path: returns (strings.Repeat(" ", cells), nil, true). Spawns
// an async fetch (deduplicated per URL across all callers) and, on
// completion, dispatches EmojiImageReadyMsg{URL: url} via
// ctx.SendMsg.
func Place(ctx PlaceContext, url string, cells int) (string, func(io.Writer) error, bool) {
	if url == "" || ctx.Fetcher == nil || cells < 1 {
		return "", nil, false
	}
	// Placeholder; tasks 5.3 / 5.5 fill in warm and cold paths.
	return strings.Repeat(" ", cells), nil, true
}
