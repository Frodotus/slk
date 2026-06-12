# slk — author grouping (compact consecutive messages)

**Date:** 2026-06-12
**Status:** implemented

## Summary

Collapse consecutive messages from the same author into a single visual
group, Slack/Discord "compact" style. A *continuation* message — same author
as the one directly above it, within a configurable time window — drops the
repeated avatar, username, and timestamp and renders just its body (plus
reactions / attachments / thread indicator), aligned under the group leader.

## Configuration

`[appearance] group_within_minutes` (int, default `0` = disabled). When > 0,
that many minutes is the maximum gap between two same-author messages for the
second to render as a continuation. Opt-in; `0` reproduces today's output
byte-for-byte.

## Grouping rule

A message `cur` continues the group led by the message above it (`prev`) when
**all** hold (`messages.ContinuesGroup`, a pure function):

- `group_within_minutes > 0`
- both messages are plain (`Subtype == ""`) — joins, topic changes, and
  `thread_broadcast` carry their own header/label and keep it
- `cur.UserID` is non-empty and equals `prev.UserID`
- same calendar day (`DateFromTS`), so a date separator never sits inside a
  group
- `0 ≤ cur.TS − prev.TS ≤ window`

Pane-specific breaks applied by the caller:

- **Message pane:** the `── new ──` unread landmark between `prev` and `cur`
  forces a full header.
- **Thread pane:** reply 0 is never a continuation (it follows the parent,
  which is always a full header); the unread landmark between two replies
  forces a full header.

## Architecture

- `internal/ui/messages/grouping.go` — `ContinuesGroup(prev, cur, windowMin)`
  + `tsEpochSeconds`. Shared by both panes.
- `messages.Model` — `groupWithin int` field, `SetGroupWithinMinutes`
  (invalidates the render cache), `isContinuation(i)`. `renderMessageEntry`
  computes the flag; `renderMessagePlain` takes a `continuation bool` and, when
  set, omits the header row (relocating any `(edited)` marker to the body) and
  reserves a blank avatar gutter (`BlankAvatarGutter`) so bodies stay aligned.
- `thread.Model` — same `groupWithin`/setter, `replyIsContinuation(i)`;
  `renderThreadMessage` gains the `continuation bool`. Parent is always
  `false`.
- `internal/ui/app.go` — `SetGroupWithinMinutes` forwards to both panes.
- `cmd/slk/main.go` — wires `cfg.Appearance.GroupWithinMinutes` at startup.

### Cache correctness

A message's `UserID`/`TS` are immutable after arrival, and any change to
message order/membership (append, prepend-history, delete) already forces a
full `buildCache` (guarded by `cacheMsgLen`). `partialRebuild` only re-renders
same-content image/stale refreshes, which never change grouping. So computing
the continuation flag inline from the neighbor needs **no new
cache-invalidation machinery**.

## Non-goals (v1)

- No "reveal the timestamp on the selected continuation" affordance.
- No hover/gutter timestamps on continuations.
- No grouping across non-empty subtypes.

## Testing

- `TestContinuesGroup` table test (author, window edges, subtype, day
  boundary, window=0, out-of-order).
- Message-pane render tests: continuation collapses (author once, second
  timestamp hidden, both bodies present); disabled shows every header;
  different author breaks the group.
- Thread render tests: consecutive same-author replies collapse; disabled
  control.
