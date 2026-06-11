# Starred channels — surface Slack's native `stars` section

**Status:** design approved (2026-06-11)
**Scope:** read-only surfacing of the user's Slack-starred channels in the sidebar.

## Motivation

The official Slack client shows a **Starred** section at the top of the sidebar
containing the conversations you've starred. slk already *receives* this data —
Slack sends it via `users.channelSections.list` (+ the `channel_section_*`
WebSocket events) as a sidebar section of type `stars` — but slk deliberately
filters that type out. The goal is to surface it so your starred channels appear
in slk, live-synced with Slack.

## Decision: read-only

slk reflects what you star/unstar **in the official Slack client**; it does not
add a star/unstar action of its own. Rationale: slk's section support is
read-only by design (v1), and starring would require writing to Slack's private
section API (Enterprise-Grid anomaly-detection risk). Star-from-slk is a possible
future follow-up, explicitly out of scope here.

## Behavior

- **What shows.** The `stars` section renders as a sidebar section, **only when
  non-empty** (an empty Starred section is hidden, mirroring `recent_apps`).
- **Placement.** Pinned to the **top** of the sidebar, above all other sections
  — the official client always shows Starred first, so `OrderedSections` hoists
  the `stars` section to the head deterministically rather than relying on
  Slack's `Next` chain.
- **Label / emoji.** Use the section's `Name`/`Emoji` from Slack; when Slack
  sends them empty (system sections can), fall back to `Starred` + ⭐.
- **No duplication.** A starred channel appears **only** in Starred, not also in
  its normal section — matching the official client.
- **Never auto-hidden.** Starred channels are exempt from
  `hide_inactive_after_days`.
- **DMs included.** Slack lets you star DMs; whatever IDs the section contains
  render, channels and DMs alike.
- **Live.** `channel_section_*` WS events already flow through `SectionStore`, so
  starring/unstarring in the official client updates slk within an event.

## Why this is almost entirely one change

The membership and live updates already arrive and are stored; only the
*render/claim* filter excludes `stars`. Two of the three behaviors fall out for
free once the filter flips:

- `SectionStore.SectionForChannel` returns "unclaimed" for a channel whose
  section is non-renderable (it calls `includeInSidebar`). Make `stars`
  renderable and the channel becomes **claimed** by Starred → `item.Section` is
  set → it is removed from the catch-all/type-default bucket. **No duplication,
  automatically.**
- `sidebar.IsStale` returns `false` (exempt) when `item.Section != ""`. A claimed
  starred channel therefore is **never auto-hidden, automatically.**

## Implementation

1. **`internal/service/sectionstore.go` — `includeInSidebar`.** Add a `stars`
   case that renders only when non-empty:
   ```go
   case "stars":
       return len(sec.ChannelIDs) > 0
   ```
   This single change makes the section render, claims its channels (no
   duplication), and exempts them from staleness — all via existing paths.

2. **Ordering.** In `OrderedSections`, hoist the `stars` section to the head of
   the returned slice (after the normal linked-list walk), so Starred always
   renders at the top regardless of Slack's `Next` chain.

3. **Label/emoji fallback.** Where the Slack `SidebarSection` is turned into the
   sidebar's `SectionMeta` (Name/Emoji for the header), default an empty `stars`
   name to `Starred` and an empty emoji to `star` (⭐).

## Scope limitation

This surfaces *Slack-native* sections, so the Starred section only appears when
`use_slack_sections = true` (the default). In glob-section mode
(`use_slack_sections = false`) there is no Slack section data, so no Starred
section. Documented as a one-line note in `wiki/Configuration.md`.

## Testing

Extend `internal/service/sectionstore_test.go`:

- A non-empty `stars` section **renders**, and its channels are **claimed**
  (`SectionForChannel` returns the stars id) and therefore **removed from the
  catch-all** bucket.
- An **empty** `stars` section does **not** render.
- `OrderedSections` returns the `stars` section **first**, regardless of its
  position in Slack's `Next` chain.
- Label/emoji fallback: a `stars` section with empty `Name`/`Emoji` resolves to
  `Starred` / ⭐ at the header layer.

Plus a sidebar-level assertion that a starred channel is exempt from staleness
hiding (covered by `IsStale` returning false for `Section != ""`).

## Files touched

- `internal/service/sectionstore.go` (+ `sectionstore_test.go`) — the filter flip
  and ordering.
- The section-meta construction site (Name/Emoji fallback) — small.
- `wiki/Configuration.md` — a note on the Starred section and the
  Slack-native-mode requirement.

## Out of scope

- Star/unstar from within slk (write path).
- A Starred section in glob/config-section mode.
