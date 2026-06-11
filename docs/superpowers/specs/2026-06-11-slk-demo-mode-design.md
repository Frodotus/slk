# `--demo` mode — curated hero scene for screenshots

**Status:** design approved (2026-06-11)
**Scope:** a `--demo` launch mode that renders slk's real TUI seeded with a
curated, fake "hero" scene — for taking marketing/README screenshots without
real Slack data or a connection.

## Goal

Run `slk --demo` in kitty, get a polished, deterministic scene, take a
screenshot. No tokens, no DB, no WebSocket, no network. The output is the
**real TUI** (real layout, theme, emoji-as-images, kitty avatars) — not a
headless render.

## Decisions (from brainstorming)

- **Output:** live TUI you screenshot yourself (not headless capture).
- **Data:** a single curated, deterministic hero scene (not randomized).
- **Scene includes:** generated avatars, an open thread panel, reactions +
  emoji + a Starred section + a 2–3 workspace rail, and one inline image.
- **Flag:** `--demo`.
- **Interaction:** static scene — navigation works; actions that would hit
  Slack (send, react, etc.) are harmless no-ops.

## Architecture

**Entry point.** Add `case "--demo":` to the arg switch in `cmd/slk/main.go`
(next to `--dump-sections`), routing to a new `runDemo(cfg)` that replaces the
Slack-connecting `run()`.

**`runDemo(cfg)`** does three things:

1. **Rendering setup** — the same wiring `run()` already does before any data:
   image-protocol detection (`imgpkg.Detect`), kitty renderer, emoji context,
   theme apply, avatar cache, image fetcher. Extract this into a shared helper
   (e.g. `setupRendering(app *ui.App, cfg config.Config) renderingDeps`) so
   demo and real mode can't drift. `run()` is refactored to call it too.
2. **Seed the curated scene** from `internal/demo` (below) via the existing
   `App` setters — no Slack, no DB.
3. `p.Run()` — the normal bubbletea program, with no connection manager / WS
   goroutines started.

**New `internal/demo` package.** Pure data + image generation, UI-independent
and unit-testable:

- `Scene()` returns the curated content: workspaces, people (name + avatar
  color), channels + DMs with their section assignment, the active channel's
  messages (text with emoji shortcodes + pill reactions, one with an inline
  image), and one open thread (parent + replies).
- `GenerateAvatar(initials string, color color.Color) image.Image` — a
  colored-initial avatar (Go `image/draw`).
- `GenerateInlineImage() image.Image` — one simple sample graphic.

## Seeding (existing setters)

- **Rail:** `app.SetWorkspaces([]workspace.WorkspaceItem{…})`.
- **Sidebar + sections:** `app.SetChannels([]sidebar.ChannelItem{…})` with
  `Section` set, plus a fake `sidebar.SectionsProvider` (one method,
  `OrderedSlackSections() []SectionMeta`) returning Starred / Channels / DMs
  via `sidebar.SetSectionsProvider`.
- **Identity:** `app.SetCurrentUserID(...)`, `app.SetUserNames(...)`.
- **Active channel + messages:** `app.SetInitialChannel(id, name, msgs)` (or
  `messagepane.SetMessages`).
- **Open thread:** `threadPanel.SetThread(parent, replies, channelID, ts)` +
  make the thread panel visible (the same path `openThreadPanel` uses, minus
  the fetch).

## Generated visuals

Both reuse slk's existing image renderer; neither fetches.

- **Avatars (easy).** For each person, generate a colored-initial PNG, render
  it to a terminal string via the existing image renderer (kitty / half-block,
  per detected protocol) at the avatar cell size, and serve them from a
  `map[userID]string` through `app.SetAvatarFunc(func(id) string {…})`.
- **Inline image (the hard part).** Prefer the same pattern as avatars —
  inject a **pre-rendered** image through the message render hook
  (`SetImageContext` / the attachment-render path) keyed by the demo
  attachment, rather than reconstructing the on-disk cache key
  (`<fileID>-<size>`), which is geometry-fragile (cf. the MCP image feature).
  The exact hook is the main thing to pin down in the implementation plan.

## Scope & safety

- No Slack client, DB, connection manager, or WS goroutines are constructed.
- Send/react/open-real-thread paths are no-ops in demo mode (nothing to call);
  ensure they fail safe rather than panic.
- Deterministic: fixed names, text, reactions, and relative timestamps so the
  scene is identical every run.

## Testing

- `internal/demo`: unit-test that `Scene()` returns a well-formed scene
  (non-empty channels/messages, the active channel has messages, the thread
  has a parent + replies) and that `GenerateAvatar` / `GenerateInlineImage`
  return non-nil images of the expected size.
- `cmd/slk`: a smoke test that builds the demo App, seeds the scene, and
  renders one `View()` frame without panicking (no `p.Run()`).

## Out of scope

- Headless / automated image capture (you screenshot kitty yourself).
- Randomized or multi-scene generation.
- Interactivity beyond navigation.

## Main risk

The inline-image injection (matching slk's render pipeline so a generated image
actually displays). Avatars are low-risk (`AvatarFunc` returns a string). If
the inline image proves fiddly, it can ship in a follow-up without blocking the
rest of the scene.
