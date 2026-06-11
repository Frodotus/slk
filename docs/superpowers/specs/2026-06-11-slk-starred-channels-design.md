# Starred channels — surface Slack's native `stars` section

**Status:** design approved (2026-06-11); data-sourcing corrected after live API verification.
**Scope:** read-only surfacing of the user's Slack-starred channels in the sidebar.

## Motivation

The official Slack client shows a **Starred** section at the top of the sidebar
containing the conversations you've starred. slk sees the `stars` *section* (via
`users.channelSections.list`) but filters it out. The goal is to surface a
Starred section in slk, live-synced with Slack.

## Decision: read-only

slk reflects what you star/unstar **in the official Slack client**; it does not
add a star/unstar action of its own. Rationale: slk's section support is
read-only by design (v1), and starring would require writing to Slack's private
API (Enterprise-Grid anomaly-detection risk). Star-from-slk is a possible future
follow-up, explicitly out of scope here.

## Data sourcing (verified against the live API)

**Correction to the original assumption.** The first draft assumed the `stars`
section arrives with its channels like other sections do (via `channel_ids_page`).
It does **not**. Verified against the live workspace (`slk --dump-sections` +
direct API calls): `users.channelSections.list` returns the `stars` section but
its **membership is always empty** (`channel_ids_page.count == 0`, even with
channels starred). Slack stores starred conversations in the **legacy `stars.list`
API** instead.

So surfacing requires *fetching* the membership and feeding it into the
otherwise-empty stars section:

- **Bootstrap:** call `stars.list`, keep `type ∈ {channel,im,group}` items, take
  their conversation IDs, and `ApplyChannelsAdded(starsSectionID, ids)` — before
  the first channel build, so the initial bucketing already sees them.
- **Live:** handle `star_added` / `star_removed` WS events, applying the same
  `ApplyChannelsAdded` / `ApplyChannelsRemoved` to the stars section.

Once the stars section has members, the **rendering** half falls out of the
existing section machinery (flip one filter):

- `SectionForChannel` then claims the channel → it leaves the catch-all (no
  duplication), automatically.
- `IsStale` exempts `Section != ""` → starred channels are never auto-hidden,
  automatically.

## Behavior

- **What shows.** The `stars` section renders **only when non-empty** (an empty
  Starred section is hidden, mirroring `recent_apps`).
- **Placement.** Pinned to the **top** of the sidebar — `OrderedSections` hoists
  the `stars` section to the head deterministically, regardless of Slack's `Next`
  chain (the official client always shows Starred first).
- **Label / emoji.** Use the section's `Name`/`Emoji` from Slack; Slack sends the
  stars section's name/emoji **empty**, so fall back to `Starred` + ⭐.
- **No duplication.** A starred channel appears **only** in Starred, not also in
  its normal section.
- **Never auto-hidden.** Starred channels are exempt from `hide_inactive_after_days`.
- **DMs included.** Starred DMs (`im`/`group` stars) render too.
- **Live.** `star_added` / `star_removed` WS events update Starred in-session.

## Implementation

1. **`stars.list` client call** (`internal/slack/stars.go`) — `Client.StarsList`
   POSTs `stars.list` (classic page paging), filters `type ∈ {channel,im,group}`,
   returns deduped conversation IDs. Plus a `wsStarEvent` type for the WS payload.

2. **Bootstrap** (`cmd/slk/main.go`, after `SectionStore.Bootstrap`) — call
   `client.StarsList(ctx)`; if non-empty, `store.ApplyChannelsAdded(starsID, ids)`
   where `starsID = store.SectionIDByType("stars")`. Best-effort (log on failure).

3. **WS handlers** (`internal/slack/events.go` dispatch + `EventHandler` +
   `cmd/slk/main.go`) — new `OnStarAdded` / `OnStarRemoved`. The dispatcher passes
   only channel/im/group stars (messages/files filtered). The rtm handlers
   `ApplyChannelsAdded`/`Removed` on the stars section and `refreshSectionsForActive()`.

4. **`includeInSidebar`** (`internal/service/sectionstore.go`) — render `stars`
   when non-empty (this also yields claim + staleness-exemption via existing paths).

5. **`OrderedSections`** — hoist the `stars` section to the head.

6. **`SectionIDByType`** (`SectionStore`) — find the stars section ID to target.

7. **Label/emoji fallback** — at the `SectionMeta` construction site
   (`cmd/slk/main.go`), default an empty `stars` name to `Starred`, emoji to `star`.

## Scope limitation

Slack-native mode only: the Starred section appears when `use_slack_sections =
true` (the default). In glob-section mode there is no Slack section data, so no
Starred section. Noted in `wiki/Configuration.md`.

## Testing

- `sectionstore_test.go`: non-empty `stars` renders + claims + pins-to-top; empty
  `stars` hidden; **`stars` populated via `ApplyChannelsAdded`/`SectionIDByType`**
  (mirrors the stars.list flow) renders + claims; non-renderable regression test
  repointed to `slack_connect`.
- `events_test.go`: `star_added`/`star_removed` dispatch → `OnStarAdded`/`Removed`
  with the channel ID; a starred **message** is filtered out.
- `cmd/slk`: `Starred`/⭐ label fallback.

## Files touched

- `internal/slack/stars.go` (new) — `StarsList` + `wsStarEvent`.
- `internal/slack/events.go` (+ `events_test.go`) — `EventHandler` methods + dispatch.
- `internal/service/sectionstore.go` (+ test) — filter flip, ordering, `SectionIDByType`.
- `cmd/slk/main.go` (+ `starred_section_test.go`) — bootstrap fetch, WS handlers, label fallback.
- `wiki/Configuration.md` — Starred-section note + Slack-native-mode requirement.

## Out of scope

- Star/unstar from within slk (write path).
- A Starred section in glob/config-section mode.
