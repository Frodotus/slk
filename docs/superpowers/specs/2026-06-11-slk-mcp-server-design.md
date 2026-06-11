# slk as an MCP server (draft-only) — design

**Status:** approved design, pre-implementation
**Date:** 2026-06-11

## Goal

Let an external AI client (specifically Claude Code) read the message/thread
the user is currently looking at in slk, discuss it, and **draft a reply into
slk's compose box** for the user to review, edit, and send. slk never sends on
the user's behalf — it only exposes read context plus a single "fill the input
box" action.

User story: *"Select a message or open a thread, ask Claude to summarize it and
draft a reply; the draft appears in slk's input; I review/edit and press Enter."*

## Non-goals

- No AI/LLM inside slk. slk is purely an MCP **server**; the model lives in the
  user's AI client.
- No posting/sending from the AI. There is no `post_message` tool. (Decided: the
  draft-only model removes the prompt-injection risk that comes with Slack's
  untrusted message text.)
- No browsing arbitrary channels, search, reactions, or DMs in v1 — only the
  current focus (selected message / open thread / active channel context).

## Architecture

Two processes bridged by a unix-domain socket:

```
Claude Code  ──stdio(MCP)──►  `slk mcp`  ──unix socket──►  running slk TUI
                              (adapter)    mcp.sock          (state + compose)
```

1. **Running slk TUI** — when `[mcp].enabled = true`, slk opens a unix socket at
   `$XDG_DATA_HOME/slk/mcp.sock` (default `~/.local/share/slk/mcp.sock`), created
   with mode `0600`. A goroutine serves a small line/JSON request-response
   protocol. The socket only exists while slk runs.
2. **`slk mcp` subcommand** — a thin stdio MCP server (built on a Go MCP SDK).
   Claude Code launches it (`claude mcp add slk -- slk mcp`). It connects to the
   socket and maps MCP tool calls to socket requests. If the socket is absent or
   refuses, every tool returns a clean *"slk is not running (or MCP is disabled
   in config)."*
3. **State access inside the TUI:**
   - **Reads** come from a `mcp.Snapshot` the App refreshes at the end of its
     `Update` loop, guarded by a mutex. No reaching into bubbletea state from the
     socket goroutine.
   - The **one write** (`set_draft`) is delivered via `p.Send(SetComposeDraftMsg{…})`,
     the same thread-safe path every other background producer uses. The socket
     handler waits for an ack (a reply message routed back) so the tool can report
     success and where the draft landed.

Rationale for stdio-subprocess + socket over a TUI-hosted HTTP server: stdio is
the universal MCP transport (simple `command: slk mcp` config, works in every
client), and a unix socket with `0600` perms is a stronger, simpler trust
boundary than a loopback TCP port.

## MCP surface

Exactly two tools (kept minimal; Claude Code drives tools more reliably than
resources).

### `slk_get_context` (read)

Returns the current focus as JSON:

```json
{
  "workspace": "Viilu",
  "channel": { "id": "C123", "name": "talous2026", "type": "channel" },
  "selected_message": {
    "ts": "1718000000.000100",
    "user": "Joni Paloniemi",
    "text": "…",
    "reactions": [{ "emoji": "+1", "count": 2 }]
  },
  "thread": {
    "open": true,
    "parent": { "ts": "…", "user": "…", "text": "…" },
    "replies": [ { "ts": "…", "user": "…", "text": "…" }, … ]
  },
  "recent_messages": [ { "ts": "…", "user": "…", "text": "…" }, … ]
}
```

- `thread.open = false` (and no `parent`/`replies`) when no thread is open.
- `recent_messages` is the last ~20 messages in the active channel for
  surrounding context, oldest→newest.
- All names are display names (already resolved by slk); text is the rendered
  Slack text.

### `slk_set_draft` (write)

Input: `{ "text": "string" }`.
Effect: sets the **active compose box** — the thread composer when a thread is
open, otherwise the channel composer (auto-targeted; the AI does not choose).
Returns `{ "target": "thread" | "channel", "channel": "…", "ok": true }`.
The user sees the text appear and edits/sends normally. Replaces any current
draft (the user can undo by clearing, `Ctrl+U`).

## Data flow

```
slk_get_context:
  Claude → `slk mcp` → socket "get_context" → TUI reads mcp.Snapshot (mutex) → JSON back

slk_set_draft:
  Claude → `slk mcp` → socket "set_draft {text}" → TUI p.Send(SetComposeDraftMsg)
        → reducer sets compose.SetValue(text) on active composer, returns ack → JSON back
```

The App publishes the snapshot whenever the selection / channel / thread / recent
messages change (cheap struct copy under a mutex at the end of `Update`).

## Security

- **Draft-only:** the server cannot send to Slack. The worst case is unwanted
  text appearing in the compose box, which the user must still send manually.
- **Unix socket, `0600`:** no network port; only the user's own processes can
  connect. This is the trust boundary — no additional token in v1.
- **Opt-in:** `[mcp].enabled` defaults to `false`. Enabling it is a conscious
  choice that exposes the active message context to local MCP clients.
- Snapshot exposes only the **active** workspace/channel context, not the whole
  cache.

## Config

```toml
[mcp]
enabled = false                  # default; set true to open the socket
# socket_path = "~/.local/share/slk/mcp.sock"   # optional override
```

## Components / structure

- `internal/mcp/` — `Snapshot` type + JSON shaping; socket server (TUI side) and
  client (adapter side); the request/response protocol (pure encode/decode).
- `internal/ui/` — a `SetComposeDraftMsg` + reducer arm that targets the active
  composer; a snapshot publisher hook in `Update`; accessors to build the snapshot
  (reuses `messagepane.SelectedMessage`, `threadPanel` parent/replies,
  `ActiveChannelID`, recent messages).
- `cmd/slk/` — `mcp` subcommand wiring the stdio MCP server (Go MCP SDK) to the
  socket client; the TUI startup opens the socket server when enabled.
- `internal/config/` — `[mcp]` block.

## Error handling

- slk not running / MCP disabled → tools return a clear message, no crash.
- Stale socket file (slk died) → adapter detects connect failure and reports it;
  the TUI removes a stale socket on startup before binding.
- Empty selection (nothing highlighted) → `selected_message` is null; the tool
  still returns channel + recent context.
- `set_draft` while no channel is active → returns `ok:false` with a reason.

## Testing

- **Unit:** snapshot JSON shaping from a synthetic App state; protocol
  encode/decode round-trip; draft-target selection (thread-open vs not).
- **Integration:** start the socket server backed by a fake snapshot + a channel
  capturing `SetComposeDraftMsg`; drive it through the adapter client; assert
  `get_context` JSON and that `set_draft` reaches the (fake) composer with the
  right target.
- The stdio MCP layer itself is a thin SDK adapter; covered by the integration
  test through the socket, not re-tested against the SDK.

## Open implementation choices (to settle in the plan)

- Which Go MCP SDK (`github.com/modelcontextprotocol/go-sdk` vs `mark3labs/mcp-go`).
- Exact socket wire format (newline-delimited JSON is sufficient).
- Whether `recent_messages` count is configurable (default ~20).
