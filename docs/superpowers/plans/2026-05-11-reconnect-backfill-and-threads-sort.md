# Reconnect backfill and threads-view sort — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After WebSocket reconnect, backfill messages and thread replies missed during the disconnect for channels with cached history; sort the threads view by LastReplyTS DESC only.

**Architecture:** Two independent fixes in one branch. (1) A new `reconnectBackfill` goroutine kicked from `rtmEventHandler.OnConnect`. It queries the cache for channels that have any cached messages, fetches `conversations.history(oldest=synced_at)` per channel via a bounded worker pool, upserts results, then fetches `conversations.replies` for any threads the cache says the user is involved in, and finally fires `ThreadsListDirtyMsg` to trigger a UI refresh. (2) A one-line change to `ListInvolvedThreads`'s sort comparator dropping the unread-first tier.

**Tech Stack:** Go, SQLite (`mattn/go-sqlite3`), `slack-go/slack`, Bubble Tea / lipgloss for UI.

**Spec:** `docs/superpowers/specs/2026-05-11-reconnect-backfill-and-threads-sort-design.md`

---

## File map

**New files:**
- `internal/cache/channels_sync.go` — `ChannelSyncRow` struct, `ChannelsWithMessages` query.
- `internal/cache/channels_sync_test.go` — tests for above.
- `cmd/slk/reconnect_backfill.go` — orchestrator that fetches per-channel history, dispatches involved-thread replies, fires `ThreadsListDirtyMsg`.
- `cmd/slk/reconnect_backfill_test.go` — integration test using a fake `SlackAPI`.

**Modified files:**
- `internal/debuglog/debuglog.go` — new `Backfill` category.
- `internal/debuglog/debuglog_test.go` — new test for `Backfill` category.
- `internal/cache/threads.go` — add `ThreadInvolvesUser` helper; change sort comparator.
- `internal/cache/threads_test.go` — rename/rewrite ordering test; new test for the empty-`last_read_ts` inversion case; new tests for `ThreadInvolvesUser`.
- `internal/slack/client.go` — new `GetHistorySince` method.
- `internal/slack/client_test.go` — tests for `GetHistorySince` pagination + cap.
- `cmd/slk/main.go` — extend `rtmEventHandler` with `backfillMu sync.Mutex` and `lastBackfillAt time.Time`; call `triggerReconnectBackfill` from `OnConnect`.

---

## Task 1: Add `Backfill` debuglog category

**Files:**
- Modify: `internal/debuglog/debuglog.go:81-180` (the existing category functions live here; add a new one alongside)
- Modify: `internal/debuglog/debuglog_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/debuglog/debuglog_test.go` and find the existing per-category tests (`TestCache`, `TestWS`, etc.). Add a new test mirroring their style. If unsure of exact pattern, read the existing tests first; the new test must follow the same structure.

```go
func TestBackfill(t *testing.T) {
    f, cleanup := initTestLog(t)
    defer cleanup()

    Backfill("hello %d", 7)

    got := readLog(t, f.Name())
    if !strings.Contains(got, "[backfill] hello 7") {
        t.Errorf("expected [backfill] tag, got %q", got)
    }
}
```

- [ ] **Step 2: Run test, see fail**

```
go test ./internal/debuglog/ -run TestBackfill -v
```

Expected: FAIL — "undefined: Backfill" compile error.

- [ ] **Step 3: Implement**

In `internal/debuglog/debuglog.go`, add this function next to the other category helpers (e.g., right after `Cache`):

```go
// Backfill logs a message tagged [backfill] for reconnect-driven
// history and thread-reply catch-up. No-op when !Enabled().
func Backfill(format string, args ...any) {
	if !enabled.Load() {
		return
	}
	logger.Printf("[backfill] "+format, args...)
}
```

Also update the package doc comment block near the top (the "Categories:" list) to mention `Backfill — reconnect-driven history backfill`.

- [ ] **Step 4: Run, see pass**

```
go test ./internal/debuglog/ -run TestBackfill -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/debuglog/
git commit -m "debuglog: add [backfill] category"
```

---

## Task 2: Cache helper — `ChannelsWithMessages`

**Files:**
- Create: `internal/cache/channels_sync.go`
- Create: `internal/cache/channels_sync_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cache/channels_sync_test.go`:

```go
package cache

import (
	"testing"
)

func TestChannelsWithMessages_EmptyWorkspace(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	got, err := db.ChannelsWithMessages("T1")
	if err != nil {
		t.Fatalf("ChannelsWithMessages: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d rows: %+v", len(got), got)
	}
}

func TestChannelsWithMessages_ReturnsChannelsWithAnyMessage(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertChannel(Channel{ID: "C2", WorkspaceID: "T1", Name: "random", Type: "channel"})
	db.UpsertChannel(Channel{ID: "C3", WorkspaceID: "T1", Name: "empty", Type: "channel"})
	db.SetChannelSyncedAt("C1", 1700000000)
	db.SetChannelSyncedAt("C2", 1700001000)

	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: "hi"})
	db.UpsertMessage(Message{TS: "2.000000", ChannelID: "C2", WorkspaceID: "T1", UserID: "U1", Text: "yo"})

	got, err := db.ChannelsWithMessages("T1")
	if err != nil {
		t.Fatalf("ChannelsWithMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(got), got)
	}
	byID := map[string]ChannelSyncRow{}
	for _, r := range got {
		byID[r.ChannelID] = r
	}
	if byID["C1"].SyncedAt != 1700000000 {
		t.Errorf("C1 synced_at = %d, want 1700000000", byID["C1"].SyncedAt)
	}
	if byID["C2"].SyncedAt != 1700001000 {
		t.Errorf("C2 synced_at = %d, want 1700001000", byID["C2"].SyncedAt)
	}
	if _, present := byID["C3"]; present {
		t.Errorf("C3 (no messages) should not be in result")
	}
}

func TestChannelsWithMessages_ChannelRowMissing(t *testing.T) {
	// A message can land via WS for a channel never UpsertChannel'd
	// (the OnMessage handler only upserts the message, not the channel).
	// In that case synced_at is 0.
	db := setupDBWithWorkspace(t)
	defer db.Close()

	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C99", WorkspaceID: "T1", UserID: "U1", Text: "orphan"})

	got, err := db.ChannelsWithMessages("T1")
	if err != nil {
		t.Fatalf("ChannelsWithMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ChannelID != "C99" || got[0].SyncedAt != 0 {
		t.Errorf("got %+v, want {C99, 0}", got[0])
	}
}

func TestChannelsWithMessages_WorkspaceIsolation(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertWorkspace(Workspace{ID: "T2", Name: "Other"})

	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertChannel(Channel{ID: "C2", WorkspaceID: "T2", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: "a"})
	db.UpsertMessage(Message{TS: "2.000000", ChannelID: "C2", WorkspaceID: "T2", UserID: "U1", Text: "b"})

	got, err := db.ChannelsWithMessages("T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ChannelID != "C1" {
		t.Errorf("expected only C1, got %+v", got)
	}
}
```

- [ ] **Step 2: Run, see fail**

```
go test ./internal/cache/ -run TestChannelsWithMessages -v
```

Expected: FAIL — `ChannelsWithMessages` and `ChannelSyncRow` undefined.

- [ ] **Step 3: Implement**

Create `internal/cache/channels_sync.go`:

```go
package cache

import "fmt"

// ChannelSyncRow is a (channelID, synced_at) pair used by the
// reconnect backfill to drive per-channel conversations.history calls.
// SyncedAt is the unix-second timestamp recorded by
// SetChannelSyncedAt; 0 means the channel row is missing or the
// column was never set (treat as "no prior sync — fetch latest page
// only" upstream).
type ChannelSyncRow struct {
	ChannelID string
	SyncedAt  int64
}

// ChannelsWithMessages returns one ChannelSyncRow per distinct
// channel_id in the messages table for the given workspace. Channels
// without any cached messages are excluded — they were either never
// visited in slk or never received a WS message event, so there is
// nothing to "catch up on" via reconnect backfill.
//
// The LEFT JOIN against channels means messages whose channel row
// was never UpsertChannel'd still appear (with SyncedAt=0). This
// happens when WS pushes a message for a channel slk hadn't
// discovered via conversations.list yet.
func (db *DB) ChannelsWithMessages(workspaceID string) ([]ChannelSyncRow, error) {
	const q = `
SELECT DISTINCT m.channel_id, COALESCE(c.synced_at, 0) AS synced_at
FROM messages m
LEFT JOIN channels c ON c.id = m.channel_id
WHERE m.workspace_id = ?
ORDER BY m.channel_id
`
	rows, err := db.conn.Query(q, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing channels with messages: %w", err)
	}
	defer rows.Close()

	var out []ChannelSyncRow
	for rows.Next() {
		var r ChannelSyncRow
		if err := rows.Scan(&r.ChannelID, &r.SyncedAt); err != nil {
			return nil, fmt.Errorf("scanning channels_sync row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run, see pass**

```
go test ./internal/cache/ -run TestChannelsWithMessages -v
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```
git add internal/cache/channels_sync.go internal/cache/channels_sync_test.go
git commit -m "cache: add ChannelsWithMessages for reconnect backfill"
```

---

## Task 3: Cache helper — `ThreadInvolvesUser`

**Files:**
- Modify: `internal/cache/threads.go`
- Modify: `internal/cache/threads_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cache/threads_test.go`:

```go
func TestThreadInvolvesUser_AuthoredParent(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "parent", ThreadTS: "1.000000"})

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if !involved {
		t.Error("self-authored parent should count as involved")
	}
}

func TestThreadInvolvesUser_RepliedToThread(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "parent", ThreadTS: "1.000000"})
	db.UpsertMessage(Message{TS: "2.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "my reply", ThreadTS: "1.000000"})

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if !involved {
		t.Error("self reply should count as involved")
	}
}

func TestThreadInvolvesUser_MentionedAngleBracket(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "hey <@USELF> ping", ThreadTS: "1.000000"})

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if !involved {
		t.Error("<@USELF> mention should count as involved")
	}
}

func TestThreadInvolvesUser_PlainTextNotInvolved(t *testing.T) {
	// Bare "USELF" without <@…> wrapping must NOT count, matching
	// ListInvolvedThreads' semantics.
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "discussing USELF in plain text", ThreadTS: "1.000000"})

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if involved {
		t.Error("plain-text USELF should not count as involved")
	}
}

func TestThreadInvolvesUser_NoneMatch(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "parent", ThreadTS: "1.000000"})
	db.UpsertMessage(Message{TS: "2.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U3", Text: "reply", ThreadTS: "1.000000"})

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if involved {
		t.Error("no self / no mention thread should not count")
	}
}

func TestThreadInvolvesUser_RespectsDeleted(t *testing.T) {
	// A deleted message should not count as involvement, matching the
	// is_deleted = 0 clause in ListInvolvedThreads.
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})
	db.UpsertMessage(Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "parent", ThreadTS: "1.000000"})
	db.UpsertMessage(Message{TS: "2.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "my reply", ThreadTS: "1.000000"})
	if err := db.DeleteMessage("C1", "2.000000"); err != nil {
		t.Fatal(err)
	}

	involved, err := db.ThreadInvolvesUser("T1", "C1", "1.000000", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if involved {
		t.Error("deleted self reply should not count as involved")
	}
}
```

`DeleteMessage` exists at `internal/cache/messages.go:103` and sets `is_deleted = 1`. Confirmed via codebase inspection during plan authoring.

- [ ] **Step 2: Run, see fail**

```
go test ./internal/cache/ -run TestThreadInvolvesUser -v
```

Expected: FAIL — `ThreadInvolvesUser` undefined.

- [ ] **Step 3: Implement**

Append to `internal/cache/threads.go`:

```go
// ThreadInvolvesUser reports whether the given thread (identified by
// workspaceID, channelID, threadTS) has any cached message authored
// by selfUserID or containing the angle-bracketed mention "<@selfUserID>".
// Mirrors the involvement predicate used by ListInvolvedThreads. Used
// by the reconnect backfill to filter which threads warrant a
// conversations.replies catch-up call.
func (db *DB) ThreadInvolvesUser(workspaceID, channelID, threadTS, selfUserID string) (bool, error) {
	mention := "%<@" + selfUserID + ">%"
	const q = `
SELECT 1 FROM messages
WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?
  AND is_deleted = 0
  AND (user_id = ? OR text LIKE ?)
LIMIT 1
`
	var one int
	err := db.conn.QueryRow(q, workspaceID, channelID, threadTS, selfUserID, mention).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking thread involvement: %w", err)
	}
	return true, nil
}
```

If `threads.go` does not already import `"database/sql"`, add the import. (Check the top of the file first.)

- [ ] **Step 4: Run, see pass**

```
go test ./internal/cache/ -run TestThreadInvolvesUser -v
```

Expected: PASS for all six tests.

- [ ] **Step 5: Commit**

```
git add internal/cache/threads.go internal/cache/threads_test.go
git commit -m "cache: add ThreadInvolvesUser helper"
```

---

## Task 4: Change threads-view sort to LastReplyTS DESC only

**Files:**
- Modify: `internal/cache/threads.go:102-108`
- Modify: `internal/cache/threads_test.go:57-89`

- [ ] **Step 1: Replace the existing ordering test and add a new test for the inversion case**

In `internal/cache/threads_test.go`, replace `TestListInvolvedThreads_OrderingUnreadFirst` (lines 57–89 of the current file) with:

```go
func TestListInvolvedThreads_OrderingByLastReplyTS(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	seedThreadFixtures(t, db, "USELF")

	got, err := db.ListInvolvedThreads("T1", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 threads, got %d", len(got))
	}
	// Sort is now purely LastReplyTS DESC.
	// Thread C last_reply_ts = 700, Thread B last_reply_ts = 400, Thread A last_reply_ts = 200.
	if got[0].ThreadTS != "1700000600.000000" {
		t.Errorf("got[0] = %s, want C (1700000600.000000)", got[0].ThreadTS)
	}
	if got[1].ThreadTS != "1700000300.000000" {
		t.Errorf("got[1] = %s, want B (1700000300.000000)", got[1].ThreadTS)
	}
	if got[2].ThreadTS != "1700000100.000000" {
		t.Errorf("got[2] = %s, want A (1700000100.000000)", got[2].ThreadTS)
	}
}

func TestListInvolvedThreads_UnreadDoesNotChangeOrder(t *testing.T) {
	// Regression for the screenshot bug: an Unread=true thread with
	// an older LastReplyTS must NOT sort ahead of an Unread=false
	// thread with a newer LastReplyTS.
	db := setupDBWithWorkspace(t)
	defer db.Close()
	// Channel with empty last_read_ts → Unread heuristic at threads.go:95
	// flips to true whenever LastReplyBy != selfUserID.
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"})

	// Older thread: someone-else replied last → Unread=true under heuristic.
	db.UpsertMessage(Message{TS: "1000.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "old self parent", ThreadTS: "1000.000000"})
	db.UpsertMessage(Message{TS: "1100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "old other reply", ThreadTS: "1000.000000"})

	// Newer thread: self replied last → Unread=false.
	db.UpsertMessage(Message{TS: "2000.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "newer parent", ThreadTS: "2000.000000"})
	db.UpsertMessage(Message{TS: "2100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "newer self reply", ThreadTS: "2000.000000"})

	got, err := db.ListInvolvedThreads("T1", "USELF")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(got))
	}
	if got[0].ThreadTS != "2000.000000" {
		t.Errorf("got[0] = %s, want newer thread 2000.000000 (LastReplyTS DESC must win regardless of Unread)", got[0].ThreadTS)
	}
	if got[1].ThreadTS != "1000.000000" {
		t.Errorf("got[1] = %s, want older thread 1000.000000", got[1].ThreadTS)
	}
	// And confirm Unread heuristic still computes as expected — the
	// dot indicator should still light up.
	if !got[1].Unread {
		t.Errorf("got[1] (older thread with other-replied-last) should still be Unread=true under heuristic")
	}
	if got[0].Unread {
		t.Errorf("got[0] (newer thread with self-replied-last) should be Unread=false")
	}
}
```

- [ ] **Step 2: Run, see fail**

```
go test ./internal/cache/ -run 'TestListInvolvedThreads_(OrderingByLastReplyTS|UnreadDoesNotChangeOrder)' -v
```

Expected: FAIL for `TestListInvolvedThreads_UnreadDoesNotChangeOrder` (the inversion is the current bug). The renamed `OrderingByLastReplyTS` test may already pass or fail depending on the seed-fixture ordering; both are valid before the fix.

- [ ] **Step 3: Change the sort**

In `internal/cache/threads.go`, replace lines 102–108 (the current `sort.SliceStable`):

```go
	// Order: newest LastReplyTS first. The Unread field is still
	// computed and returned so the UI can render the dot indicator,
	// but it no longer participates in ordering. The previous
	// "unread first" tier produced confusing results when
	// channels.last_read_ts was empty (string compare LastReplyTS >
	// "" was always true), pushing genuinely-recent activity below
	// older activity. See
	// docs/superpowers/specs/2026-05-11-reconnect-backfill-and-threads-sort-design.md
	// for context.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastReplyTS > out[j].LastReplyTS
	})
```

- [ ] **Step 4: Run, see pass**

```
go test ./internal/cache/ -run TestListInvolvedThreads -v
```

Expected: PASS for all `TestListInvolvedThreads_*` tests including the two new ones.

- [ ] **Step 5: Commit**

```
git add internal/cache/threads.go internal/cache/threads_test.go
git commit -m "cache: sort threads view by LastReplyTS only

Drop the unread-first sort tier. The Unread heuristic at
threads.go:95 misfires when channels.last_read_ts is empty (the
string comparison LastReplyTS > '' is always true), flagging every
thread someone-else-replied-to-last as Unread regardless of how
old it actually is. The result was that a 1-hour-old self-replied
thread sorted below an 8-row block of older 'unread' threads.

The Unread field is still populated and read by the threadsview to
render the dot indicator and feed the sidebar badge; it just no
longer changes row order."
```

---

## Task 5: Add `GetHistorySince` to the Slack client

**Files:**
- Modify: `internal/slack/client.go`
- Modify: `internal/slack/client_test.go`

- [ ] **Step 1: Write the failing tests**

Open `internal/slack/client_test.go`. Look at an existing pagination test like the one for `GetChannels` (search for `TestGetChannels_Paginates` or similar) to see how the mock-cursor pattern works. Then add at the bottom of the file:

```go
func TestGetHistorySince_PaginatesUntilExhausted(t *testing.T) {
	page1 := []slack.Message{
		{Msg: slack.Msg{Timestamp: "100.000000", User: "U1", Text: "a"}},
		{Msg: slack.Msg{Timestamp: "200.000000", User: "U1", Text: "b"}},
	}
	page2 := []slack.Message{
		{Msg: slack.Msg{Timestamp: "300.000000", User: "U1", Text: "c"}},
	}
	mock := &mockSlackAPI{
		historyResponses: []*slack.GetConversationHistoryResponse{
			{Messages: page1, HasMore: true, ResponseMetaData: slack.ResponseMetadata{Cursor: "cur1"}},
			{Messages: page2, HasMore: false},
		},
	}
	c := &Client{api: mock}

	msgs, err := c.GetHistorySince(context.Background(), "C1", "50.000000", 500)
	if err != nil {
		t.Fatalf("GetHistorySince: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("got %d messages, want 3", len(msgs))
	}
	if len(mock.historyCalls) != 2 {
		t.Fatalf("expected 2 history calls, got %d", len(mock.historyCalls))
	}
	if mock.historyCalls[0].Oldest != "50.000000" || mock.historyCalls[0].Cursor != "" {
		t.Errorf("call[0] = %+v, want Oldest=50.000000, Cursor=''", mock.historyCalls[0])
	}
	if mock.historyCalls[1].Cursor != "cur1" {
		t.Errorf("call[1].Cursor = %q, want %q", mock.historyCalls[1].Cursor, "cur1")
	}
}

func TestGetHistorySince_RespectsHardCap(t *testing.T) {
	// 3 pages of 2 messages each = 6; cap=4 should stop after page 2.
	mkPage := func(start int, hasMore bool, cursor string) *slack.GetConversationHistoryResponse {
		return &slack.GetConversationHistoryResponse{
			Messages: []slack.Message{
				{Msg: slack.Msg{Timestamp: fmt.Sprintf("%d.000000", start), User: "U1"}},
				{Msg: slack.Msg{Timestamp: fmt.Sprintf("%d.000000", start+1), User: "U1"}},
			},
			HasMore:          hasMore,
			ResponseMetaData: slack.ResponseMetadata{Cursor: cursor},
		}
	}
	mock := &mockSlackAPI{
		historyResponses: []*slack.GetConversationHistoryResponse{
			mkPage(100, true, "c1"),
			mkPage(200, true, "c2"),
			mkPage(300, false, ""),
		},
	}
	c := &Client{api: mock}

	msgs, err := c.GetHistorySince(context.Background(), "C1", "0", 4)
	if err != nil {
		t.Fatalf("GetHistorySince: %v", err)
	}
	if len(msgs) != 4 {
		t.Errorf("got %d, want 4 (cap)", len(msgs))
	}
	if len(mock.historyCalls) != 2 {
		t.Errorf("expected 2 calls before cap stop, got %d", len(mock.historyCalls))
	}
}

func TestGetHistorySince_NoMessagesEmptyResult(t *testing.T) {
	mock := &mockSlackAPI{
		historyResponses: []*slack.GetConversationHistoryResponse{
			{Messages: nil, HasMore: false},
		},
	}
	c := &Client{api: mock}

	msgs, err := c.GetHistorySince(context.Background(), "C1", "100", 500)
	if err != nil {
		t.Fatalf("GetHistorySince: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty, got %+v", msgs)
	}
}
```

The mock needs new fields. Find the `mockSlackAPI` struct definition (around line 133 of `client_test.go`) and extend it:

```go
type mockSlackAPI struct {
	// ... existing fields ...

	// NEW: GetHistorySince support
	historyResponses []*slack.GetConversationHistoryResponse
	historyCalls     []slack.GetConversationHistoryParameters
}
```

And replace the existing `GetConversationHistory` mock (around line 155) so it consumes from `historyResponses` and records into `historyCalls`. Read the existing method first; keep any existing fields it uses for other tests:

```go
func (m *mockSlackAPI) GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	m.historyCalls = append(m.historyCalls, *params)
	if len(m.historyResponses) > 0 {
		resp := m.historyResponses[0]
		m.historyResponses = m.historyResponses[1:]
		return resp, nil
	}
	// Fall back to whatever the existing mock returned for other tests.
	// If existing tests set a single `historyResp` field, return that.
	if m.historyResp != nil {
		return m.historyResp, nil
	}
	return &slack.GetConversationHistoryResponse{}, nil
}
```

If the existing mock's field for the single-response case isn't `historyResp`, use the actual name; the goal is to preserve existing behavior. Run all `internal/slack` tests after this change to verify no existing test broke.

- [ ] **Step 2: Run, see fail**

```
go test ./internal/slack/ -run TestGetHistorySince -v
```

Expected: FAIL — `GetHistorySince` undefined.

Also run all slack tests to make sure the mock extension doesn't break existing tests:

```
go test ./internal/slack/ -v
```

Expected: all existing tests still PASS; new `TestGetHistorySince_*` FAIL.

- [ ] **Step 3: Implement**

Add to `internal/slack/client.go`, next to `GetHistory` and `GetOlderHistory`:

```go
// GetHistorySince fetches all messages newer than `oldest` in the
// channel, paginating forward via response_metadata.next_cursor.
// Stops when has_more is false or when the cumulative message count
// reaches maxTotal (a hard cap that protects against runaway
// backfills after very long disconnects in busy channels).
//
// Returns messages in the order Slack delivered them (newest-first
// per page, oldest page first since pagination walks forward through
// time). Callers that need oldest-first order should reverse the
// slice.
//
// If oldest == "", behaves like GetHistory(limit=200) on the most
// recent page only (no pagination). This matches the spec's
// "synced_at == 0 → fetch latest page only" rule.
func (c *Client) GetHistorySince(ctx context.Context, channelID, oldest string, maxTotal int) ([]slack.Message, error) {
	if maxTotal <= 0 {
		maxTotal = 500
	}

	// No prior sync — fetch latest page only.
	if oldest == "" {
		params := &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     200,
		}
		resp, err := c.api.GetConversationHistory(params)
		if err != nil {
			return nil, fmt.Errorf("get history (no oldest): %w", err)
		}
		if len(resp.Messages) > maxTotal {
			return resp.Messages[:maxTotal], nil
		}
		return resp.Messages, nil
	}

	var all []slack.Message
	cursor := ""
	for {
		params := &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Oldest:    oldest,
			Limit:     200,
			Cursor:    cursor,
		}
		resp, err := c.api.GetConversationHistory(params)
		if err != nil {
			// Rate-limit retry mirrors GetChannels' pattern.
			if rlErr, ok := err.(*slack.RateLimitedError); ok {
				wait := rlErr.RetryAfter
				if wait == 0 {
					wait = 30 * time.Second
				}
				select {
				case <-ctx.Done():
					return all, ctx.Err()
				case <-time.After(wait):
				}
				continue
			}
			return all, fmt.Errorf("get history since %s: %w", oldest, err)
		}

		all = append(all, resp.Messages...)
		if len(all) >= maxTotal {
			return all[:maxTotal], nil
		}
		if !resp.HasMore || resp.ResponseMetaData.Cursor == "" {
			return all, nil
		}
		cursor = resp.ResponseMetaData.Cursor
	}
}
```

- [ ] **Step 4: Run, see pass**

```
go test ./internal/slack/ -v
```

Expected: ALL tests pass (existing + new `TestGetHistorySince_*`).

- [ ] **Step 5: Commit**

```
git add internal/slack/client.go internal/slack/client_test.go
git commit -m "slack: add GetHistorySince with pagination and hard cap

Wraps conversations.history with forward pagination via
response_metadata.next_cursor. Capped at maxTotal messages per
call to bound work after very long disconnects. Used by the
reconnect backfill to catch up on missed messages since the
channel's last synced_at."
```

---

## Task 6: Backfill orchestrator — channel phase only

**Files:**
- Create: `cmd/slk/reconnect_backfill.go`
- Create: `cmd/slk/reconnect_backfill_test.go`

This task builds the channel-phase part. Task 7 layers thread replies on top.

- [ ] **Step 1: Write the failing test**

Create `cmd/slk/reconnect_backfill_test.go`:

```go
package main

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/slack-go/slack"
)

// fakeHistoryAPI implements just enough of slackclient.SlackAPI for the
// backfill channel phase. Embedded zero-value methods on the SlackAPI
// interface would not satisfy it, so we explicitly implement each
// method we need; the rest can be a noopSlackAPI base if necessary.
type fakeHistoryAPI struct {
	noopSlackAPI

	mu           sync.Mutex
	inFlight     int32
	maxInFlight  int32
	delay        time.Duration // simulate network so concurrency is observable
	responsesPer map[string][]*slack.GetConversationHistoryResponse
	calls        map[string]int
}

func (f *fakeHistoryAPI) GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	cur := atomic.AddInt32(&f.inFlight, 1)
	defer atomic.AddInt32(&f.inFlight, -1)
	for {
		hi := atomic.LoadInt32(&f.maxInFlight)
		if cur <= hi || atomic.CompareAndSwapInt32(&f.maxInFlight, hi, cur) {
			break
		}
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[params.ChannelID]++
	resps := f.responsesPer[params.ChannelID]
	if len(resps) == 0 {
		return &slack.GetConversationHistoryResponse{}, nil
	}
	resp := resps[0]
	f.responsesPer[params.ChannelID] = resps[1:]
	return resp, nil
}

func newTestDB(t *testing.T) *cache.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := cache.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "T"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	return db
}

func TestBackfillChannels_FetchesPerChannelSinceSyncedAt(t *testing.T) {
	db := newTestDB(t)

	// Two channels with cached messages and synced_at.
	db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "a", Type: "channel"})
	db.UpsertChannel(cache.Channel{ID: "C2", WorkspaceID: "T1", Name: "b", Type: "channel"})
	db.UpsertMessage(cache.Message{TS: "10.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: "old"})
	db.UpsertMessage(cache.Message{TS: "20.000000", ChannelID: "C2", WorkspaceID: "T1", UserID: "U1", Text: "old"})
	db.SetChannelSyncedAt("C1", 100)
	db.SetChannelSyncedAt("C2", 200)

	api := &fakeHistoryAPI{
		responsesPer: map[string][]*slack.GetConversationHistoryResponse{
			"C1": {{Messages: []slack.Message{{Msg: slack.Msg{Timestamp: "150.000000", User: "U2", Text: "new in c1"}}}}},
			"C2": {{Messages: []slack.Message{{Msg: slack.Msg{Timestamp: "250.000000", User: "U2", Text: "new in c2"}}}}},
		},
		calls: map[string]int{},
	}
	client := &slackclient.Client{}
	slackclient.SetAPIForTest(client, api) // helper added in Task 6 if needed

	bf := newBackfiller(client, db, "T1", "USELF", nil, 4, 500)
	if err := bf.runChannelPhase(context.Background()); err != nil {
		t.Fatalf("runChannelPhase: %v", err)
	}

	if api.calls["C1"] != 1 || api.calls["C2"] != 1 {
		t.Errorf("expected 1 call each for C1 and C2, got %+v", api.calls)
	}
	// New messages were upserted.
	if _, err := db.GetMessage("C1", "150.000000"); err != nil {
		t.Errorf("missing upserted message C1/150: %v", err)
	}
	if _, err := db.GetMessage("C2", "250.000000"); err != nil {
		t.Errorf("missing upserted message C2/250: %v", err)
	}
}

func TestBackfillChannels_BoundedConcurrency(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 8; i++ {
		id := "C" + string(rune('1'+i))
		db.UpsertChannel(cache.Channel{ID: id, WorkspaceID: "T1", Name: id, Type: "channel"})
		db.UpsertMessage(cache.Message{TS: "1.000000", ChannelID: id, WorkspaceID: "T1", UserID: "U", Text: "x"})
	}

	responses := map[string][]*slack.GetConversationHistoryResponse{}
	for i := 0; i < 8; i++ {
		id := "C" + string(rune('1'+i))
		responses[id] = []*slack.GetConversationHistoryResponse{{}}
	}
	api := &fakeHistoryAPI{
		delay:        50 * time.Millisecond,
		responsesPer: responses,
		calls:        map[string]int{},
	}
	client := &slackclient.Client{}
	slackclient.SetAPIForTest(client, api)

	bf := newBackfiller(client, db, "T1", "USELF", nil, 4, 500)
	if err := bf.runChannelPhase(context.Background()); err != nil {
		t.Fatalf("runChannelPhase: %v", err)
	}

	if got := atomic.LoadInt32(&api.maxInFlight); got > 4 {
		t.Errorf("max in-flight = %d, want ≤ 4", got)
	}
	if len(api.calls) != 8 {
		t.Errorf("expected 8 channels called, got %d", len(api.calls))
	}
}
```

The test file references `noopSlackAPI` (a base struct that returns zero/nil for every SlackAPI method except the ones we override), `SetAPIForTest`, and `db.GetMessage`. Verify each exists in the codebase. If `noopSlackAPI` doesn't exist as a test fixture, create a minimal one in `cmd/slk/reconnect_backfill_test.go`:

```go
// noopSlackAPI satisfies slackclient.SlackAPI by returning zero values.
// Tests embed it and override only the methods they care about.
type noopSlackAPI struct{}

func (noopSlackAPI) GetConversations(*slack.GetConversationsParameters) ([]slack.Channel, string, error) { return nil, "", nil }
// ... (one no-op per SlackAPI method)
```

The simpler approach is to use the existing `mockSlackAPI` from `internal/slack/client_test.go` if it's exported. If it's package-private, copy a thin equivalent. If `SetAPIForTest` doesn't exist, add it as a test-only helper:

```go
// internal/slack/client_export_test.go (new file)
package slack

// SetAPIForTest replaces the inner SlackAPI on c. Tests-only.
func SetAPIForTest(c *Client, api SlackAPI) {
	c.api = api
}
```

- [ ] **Step 2: Run, see fail**

```
go test ./cmd/slk/ -run TestBackfillChannels -v
```

Expected: FAIL — `newBackfiller`, `runChannelPhase`, possibly `noopSlackAPI`, `SetAPIForTest`, `db.GetMessage` undefined or compile errors.

- [ ] **Step 3: Add `SetAPIForTest` if missing**

If the test compile fails on `slackclient.SetAPIForTest`, add the helper described in Step 1 (`internal/slack/client_export_test.go`).

If `db.GetMessage` doesn't exist, check `internal/cache/messages.go` for the actual function name (likely `GetMessage` or similar). Adapt the test to use whatever exists. If nothing exists, add a minimal helper:

```go
// internal/cache/messages.go (append)
func (db *DB) GetMessage(channelID, ts string) (Message, error) {
	var m Message
	var isDeleted int
	err := db.conn.QueryRow(`
		SELECT ts, channel_id, workspace_id, user_id, text, thread_ts, reply_count, edited_at, is_deleted, raw_json, created_at, subtype
		FROM messages WHERE channel_id = ? AND ts = ?
	`, channelID, ts).Scan(&m.TS, &m.ChannelID, &m.WorkspaceID, &m.UserID, &m.Text, &m.ThreadTS, &m.ReplyCount, &m.EditedAt, &isDeleted, &m.RawJSON, &m.CreatedAt, &m.Subtype)
	if err != nil {
		return m, err
	}
	m.IsDeleted = isDeleted == 1
	return m, nil
}
```

Add a quick test for `GetMessage` if you create it (TDD).

- [ ] **Step 4: Implement the backfiller skeleton**

Create `cmd/slk/reconnect_backfill.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/debuglog"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
)

// backfiller orchestrates a single reconnect backfill pass for one
// workspace. Holds all per-pass state so the dedupe in OnConnect only
// needs to track timestamps, not in-flight work.
type backfiller struct {
	client       *slackclient.Client
	db           *cache.DB
	workspaceID  string
	selfUserID   string
	program      *tea.Program // nil in tests; used to send ThreadsListDirtyMsg
	concurrency  int
	perChannelCap int

	// Threads discovered while iterating channel-phase results. Populated
	// during runChannelPhase; consumed by runThreadPhase. Stored as a
	// set of (channelID, threadTS) pairs.
	mu              sync.Mutex
	discoveredThreads map[threadKey]struct{}
}

type threadKey struct {
	ChannelID string
	ThreadTS  string
}

// newBackfiller constructs a backfiller. concurrency caps simultaneous
// HTTP calls (use 4 in production); perChannelCap is the maxTotal
// passed to GetHistorySince (use 500).
func newBackfiller(client *slackclient.Client, db *cache.DB, workspaceID, selfUserID string, program *tea.Program, concurrency, perChannelCap int) *backfiller {
	if concurrency < 1 {
		concurrency = 1
	}
	if perChannelCap < 1 {
		perChannelCap = 500
	}
	return &backfiller{
		client:            client,
		db:                db,
		workspaceID:       workspaceID,
		selfUserID:        selfUserID,
		program:           program,
		concurrency:       concurrency,
		perChannelCap:     perChannelCap,
		discoveredThreads: map[threadKey]struct{}{},
	}
}

// runChannelPhase fetches conversations.history(oldest=synced_at) for
// every channel in the workspace that has cached messages. Upserts
// all returned messages and bumps synced_at on success. Records any
// thread_ts seen in the results into b.discoveredThreads for the
// thread phase to consume.
func (b *backfiller) runChannelPhase(ctx context.Context) error {
	channels, err := b.db.ChannelsWithMessages(b.workspaceID)
	if err != nil {
		return err
	}
	debuglog.Backfill("team=%s trigger=reconnect channels=%d start", b.workspaceID, len(channels))
	start := time.Now()

	sem := make(chan struct{}, b.concurrency)
	var wg sync.WaitGroup
	totalMsgs := 0
	var totalMu sync.Mutex

	for _, row := range channels {
		wg.Add(1)
		go func(row cache.ChannelSyncRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			n, err := b.backfillOneChannel(ctx, row)
			if err != nil {
				debuglog.Backfill("team=%s channel=%s err=%v", b.workspaceID, row.ChannelID, err)
				return
			}
			totalMu.Lock()
			totalMsgs += n
			totalMu.Unlock()
		}(row)
	}
	wg.Wait()

	debuglog.Backfill("team=%s channel-phase done total_msgs=%d dur_ms=%d",
		b.workspaceID, totalMsgs, time.Since(start).Milliseconds())
	return nil
}

// backfillOneChannel fetches missed history for a single channel and
// upserts every returned message. Returns the count of upserted
// messages. Errors are logged but not propagated as failures — one
// bad channel does not abort the workspace pass.
func (b *backfiller) backfillOneChannel(ctx context.Context, row cache.ChannelSyncRow) (int, error) {
	oldest := ""
	if row.SyncedAt > 0 {
		oldest = strconv.FormatInt(row.SyncedAt, 10) + ".000000"
	}
	start := time.Now()

	msgs, err := b.client.GetHistorySince(ctx, row.ChannelID, oldest, b.perChannelCap)
	if err != nil {
		return 0, err
	}

	for _, m := range msgs {
		raw, _ := json.Marshal(m)
		b.db.UpsertMessage(cache.Message{
			TS:          m.Timestamp,
			ChannelID:   row.ChannelID,
			WorkspaceID: b.workspaceID,
			UserID:      m.User,
			Text:        m.Text,
			ThreadTS:    m.ThreadTimestamp,
			ReplyCount:  m.ReplyCount,
			Subtype:     m.SubType,
			RawJSON:     string(raw),
			CreatedAt:   time.Now().Unix(),
		})
		if m.ThreadTimestamp != "" {
			b.mu.Lock()
			b.discoveredThreads[threadKey{ChannelID: row.ChannelID, ThreadTS: m.ThreadTimestamp}] = struct{}{}
			b.mu.Unlock()
		}
	}
	// Bump synced_at exactly once per channel after the batch.
	b.db.SetChannelSyncedAt(row.ChannelID, time.Now().Unix())

	cap := ""
	if len(msgs) >= b.perChannelCap {
		cap = " capped=true"
	}
	debuglog.Backfill("team=%s channel=%s oldest=%s count=%d dur_ms=%d%s",
		b.workspaceID, row.ChannelID, oldest, len(msgs), time.Since(start).Milliseconds(), cap)
	return len(msgs), nil
}
```

- [ ] **Step 5: Run, see pass**

```
go test ./cmd/slk/ -run TestBackfillChannels -v
```

Expected: PASS for both tests.

Also run the full test suite to verify nothing else broke:

```
go test ./...
```

Expected: ALL tests PASS.

- [ ] **Step 6: Commit**

```
git add cmd/slk/reconnect_backfill.go cmd/slk/reconnect_backfill_test.go internal/cache/messages.go internal/slack/client_export_test.go
git commit -m "backfill: channel phase fetches per-channel since synced_at

Adds backfiller with bounded-concurrency worker pool. Each channel
gets a GetHistorySince call with oldest=synced_at (or '' when
never synced). Returned messages are upserted and any thread_ts
seen is recorded for the thread phase (added next)."
```

Adjust the staged files to whatever you actually changed (e.g., only add `internal/cache/messages.go` if you created `GetMessage` there).

---

## Task 7: Backfill orchestrator — thread phase + UI refresh

**Files:**
- Modify: `cmd/slk/reconnect_backfill.go`
- Modify: `cmd/slk/reconnect_backfill_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/slk/reconnect_backfill_test.go`:

```go
type fakeRepliesAPI struct {
	fakeHistoryAPI
	repliesCalls    []struct{ Channel, TS string }
	repliesResponse []slack.Message
}

func (f *fakeRepliesAPI) GetConversationReplies(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repliesCalls = append(f.repliesCalls, struct{ Channel, TS string }{params.ChannelID, params.Timestamp})
	return f.repliesResponse, false, "", nil
}

func TestBackfillThreads_FetchesRepliesForInvolvedThreads(t *testing.T) {
	db := newTestDB(t)
	db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "a", Type: "channel"})
	// Existing cached parent in thread T1: self authored → involved.
	db.UpsertMessage(cache.Message{TS: "100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "USELF", Text: "self parent", ThreadTS: "100.000000"})
	// Existing cached parent in thread T2: not involved.
	db.UpsertMessage(cache.Message{TS: "200.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "other parent", ThreadTS: "200.000000"})
	db.SetChannelSyncedAt("C1", 50)

	api := &fakeRepliesAPI{
		fakeHistoryAPI: fakeHistoryAPI{
			responsesPer: map[string][]*slack.GetConversationHistoryResponse{
				"C1": {{Messages: []slack.Message{
					// New reply on involved thread T1.
					{Msg: slack.Msg{Timestamp: "150.000000", User: "U2", Text: "reply to self", ThreadTimestamp: "100.000000"}},
					// New reply on non-involved thread T2.
					{Msg: slack.Msg{Timestamp: "250.000000", User: "U3", Text: "reply on other", ThreadTimestamp: "200.000000"}},
				}}},
			},
			calls: map[string]int{},
		},
		repliesResponse: []slack.Message{
			{Msg: slack.Msg{Timestamp: "100.000000", User: "USELF", Text: "self parent", ThreadTimestamp: "100.000000"}},
			{Msg: slack.Msg{Timestamp: "150.000000", User: "U2", Text: "reply to self", ThreadTimestamp: "100.000000"}},
		},
	}
	client := &slackclient.Client{}
	slackclient.SetAPIForTest(client, api)

	bf := newBackfiller(client, db, "T1", "USELF", nil, 4, 500)
	if err := bf.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(api.repliesCalls) != 1 {
		t.Fatalf("expected 1 replies call (for involved thread T1), got %d: %+v", len(api.repliesCalls), api.repliesCalls)
	}
	if api.repliesCalls[0].Channel != "C1" || api.repliesCalls[0].TS != "100.000000" {
		t.Errorf("replies call = %+v, want C1/100.000000", api.repliesCalls[0])
	}
}

func TestBackfill_FiresThreadsListDirtyMsg(t *testing.T) {
	// Verifies a ui.ThreadsListDirtyMsg{TeamID:T1} is dispatched.
	// We capture program.Send by passing a captureProgram (a tiny
	// tea.Program shim) — but since tea.Program is concrete we use a
	// channel injected through the backfiller's program field. The
	// production code calls `b.program.Send(msg)`; for the test we
	// extract that into a small interface to allow substitution.
	//
	// Implementation note: add `type teaSender interface{ Send(tea.Msg) }`
	// in reconnect_backfill.go, change program field to teaSender, and
	// pass either *tea.Program or a captureSender in tests.

	db := newTestDB(t)
	db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "a", Type: "channel"})
	db.UpsertMessage(cache.Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U", Text: "x"})
	db.SetChannelSyncedAt("C1", 100)

	api := &fakeRepliesAPI{
		fakeHistoryAPI: fakeHistoryAPI{
			responsesPer: map[string][]*slack.GetConversationHistoryResponse{
				"C1": {{}},
			},
			calls: map[string]int{},
		},
	}
	client := &slackclient.Client{}
	slackclient.SetAPIForTest(client, api)

	captured := &captureSender{}
	bf := newBackfiller(client, db, "T1", "USELF", captured, 4, 500)
	if err := bf.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(captured.sent) != 1 {
		t.Fatalf("expected 1 sent msg, got %d", len(captured.sent))
	}
	dirty, ok := captured.sent[0].(ui.ThreadsListDirtyMsg)
	if !ok {
		t.Fatalf("expected ThreadsListDirtyMsg, got %T", captured.sent[0])
	}
	if dirty.TeamID != "T1" {
		t.Errorf("TeamID = %s, want T1", dirty.TeamID)
	}
}

type captureSender struct {
	mu   sync.Mutex
	sent []tea.Msg
}

func (c *captureSender) Send(msg tea.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, msg)
}
```

- [ ] **Step 2: Run, see fail**

```
go test ./cmd/slk/ -run 'TestBackfill(Threads|_)' -v
```

Expected: FAIL — `(*backfiller).run` undefined; possibly `teaSender` interface issues.

- [ ] **Step 3: Implement**

Modify `cmd/slk/reconnect_backfill.go`:

1. Replace `program *tea.Program` field with an interface so tests can substitute:

```go
// teaSender is the subset of *tea.Program the backfiller uses. *tea.Program
// satisfies it implicitly; tests pass a captureSender.
type teaSender interface {
	Send(msg tea.Msg)
}
```

And change the struct field:

```go
type backfiller struct {
	// ...
	program teaSender
	// ...
}
```

Update `newBackfiller` signature accordingly:

```go
func newBackfiller(client *slackclient.Client, db *cache.DB, workspaceID, selfUserID string, program teaSender, concurrency, perChannelCap int) *backfiller {
```

2. Add `runThreadPhase` and a top-level `run`:

```go
// runThreadPhase iterates b.discoveredThreads, filters to threads
// where the user is involved per the cache, and dispatches a
// conversations.replies fetch for each through a bounded worker pool.
// Failures are logged and skipped — one bad thread does not abort
// the pass.
func (b *backfiller) runThreadPhase(ctx context.Context) error {
	b.mu.Lock()
	threads := make([]threadKey, 0, len(b.discoveredThreads))
	for k := range b.discoveredThreads {
		threads = append(threads, k)
	}
	b.mu.Unlock()

	start := time.Now()
	// Filter to involved threads using the cache (cheap, no network).
	involved := make([]threadKey, 0, len(threads))
	for _, k := range threads {
		ok, err := b.db.ThreadInvolvesUser(b.workspaceID, k.ChannelID, k.ThreadTS, b.selfUserID)
		if err != nil {
			debuglog.Backfill("team=%s thread-filter err channel=%s thread_ts=%s err=%v", b.workspaceID, k.ChannelID, k.ThreadTS, err)
			continue
		}
		if ok {
			involved = append(involved, k)
		}
	}

	sem := make(chan struct{}, b.concurrency)
	var wg sync.WaitGroup
	for _, k := range involved {
		wg.Add(1)
		go func(k threadKey) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := b.backfillOneThread(ctx, k); err != nil {
				debuglog.Backfill("team=%s thread channel=%s thread_ts=%s err=%v", b.workspaceID, k.ChannelID, k.ThreadTS, err)
			}
		}(k)
	}
	wg.Wait()

	debuglog.Backfill("team=%s thread-phase threads_involved=%d done dur_ms=%d",
		b.workspaceID, len(involved), time.Since(start).Milliseconds())
	return nil
}

// backfillOneThread fetches the full reply list for a thread and
// upserts every returned message.
func (b *backfiller) backfillOneThread(ctx context.Context, k threadKey) error {
	replies, err := b.client.GetReplies(ctx, k.ChannelID, k.ThreadTS)
	if err != nil {
		return err
	}
	for _, m := range replies {
		raw, _ := json.Marshal(m)
		b.db.UpsertMessage(cache.Message{
			TS:          m.Timestamp,
			ChannelID:   k.ChannelID,
			WorkspaceID: b.workspaceID,
			UserID:      m.User,
			Text:        m.Text,
			ThreadTS:    m.ThreadTimestamp,
			ReplyCount:  m.ReplyCount,
			Subtype:     m.SubType,
			RawJSON:     string(raw),
			CreatedAt:   time.Now().Unix(),
		})
	}
	return nil
}

// run executes the full backfill pass: channel phase, then thread
// phase, then a ThreadsListDirtyMsg dispatch so the UI re-queries
// the threads view from the now-current cache.
func (b *backfiller) run(ctx context.Context) error {
	start := time.Now()
	if err := b.runChannelPhase(ctx); err != nil {
		debuglog.Backfill("team=%s channel-phase err=%v", b.workspaceID, err)
	}
	if err := b.runThreadPhase(ctx); err != nil {
		debuglog.Backfill("team=%s thread-phase err=%v", b.workspaceID, err)
	}
	if b.program != nil {
		b.program.Send(ui.ThreadsListDirtyMsg{TeamID: b.workspaceID})
	}
	debuglog.Backfill("team=%s trigger=reconnect total_dur_ms=%d status=ok",
		b.workspaceID, time.Since(start).Milliseconds())
	return nil
}
```

If the existing `*slackclient.Client` does not expose `GetReplies(ctx, channelID, threadTS)`, find the actual method name (search `internal/slack/client.go`). Per the agent's earlier exploration this exists; if its signature differs, adapt accordingly.

- [ ] **Step 4: Run, see pass**

```
go test ./cmd/slk/ -run 'TestBackfill' -v
```

Expected: PASS for all three backfill tests.

Run the full suite:

```
go test ./...
```

Expected: ALL PASS.

- [ ] **Step 5: Commit**

```
git add cmd/slk/reconnect_backfill.go cmd/slk/reconnect_backfill_test.go
git commit -m "backfill: thread phase + ThreadsListDirtyMsg dispatch

After the channel phase populates discoveredThreads, filter to
threads where the cache shows the user is involved, fetch replies
for each via conversations.replies, and upsert. Finally dispatch
ThreadsListDirtyMsg so the UI re-queries the threads view from
the now-current cache."
```

---

## Task 8: Wire backfill into `OnConnect` with 30 s dedupe

**Files:**
- Modify: `cmd/slk/main.go` — `rtmEventHandler` struct (around line 2410) and `OnConnect` (lines 2664–2688)

- [ ] **Step 1: Write the failing test**

Append to `cmd/slk/reconnect_backfill_test.go`:

```go
func TestBackfill_DedupeWindow(t *testing.T) {
	// Two triggers within the 30s window: only one pass runs.
	db := newTestDB(t)
	db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "a", Type: "channel"})
	db.UpsertMessage(cache.Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U", Text: "x"})

	api := &fakeRepliesAPI{
		fakeHistoryAPI: fakeHistoryAPI{
			responsesPer: map[string][]*slack.GetConversationHistoryResponse{
				"C1": {{}, {}}, // enough for two passes if dedupe failed
			},
			calls: map[string]int{},
		},
	}
	client := &slackclient.Client{}
	slackclient.SetAPIForTest(client, api)

	gate := &dedupeGate{window: 30 * time.Second}

	first := gate.tryStart(time.Unix(1000, 0))
	if !first {
		t.Fatal("first call should be allowed")
	}
	second := gate.tryStart(time.Unix(1010, 0))
	if second {
		t.Error("second call within 30s should be blocked")
	}
	third := gate.tryStart(time.Unix(1031, 0))
	if !third {
		t.Error("call after window should be allowed")
	}
}
```

- [ ] **Step 2: Run, see fail**

```
go test ./cmd/slk/ -run TestBackfill_DedupeWindow -v
```

Expected: FAIL — `dedupeGate`, `tryStart` undefined.

- [ ] **Step 3: Implement the dedupe gate**

Append to `cmd/slk/reconnect_backfill.go`:

```go
// dedupeGate enforces a minimum interval between backfill passes. Used
// by OnConnect so a rapid disconnect/reconnect flap doesn't trigger
// thundering backfills. Safe for concurrent calls.
type dedupeGate struct {
	mu     sync.Mutex
	last   time.Time
	window time.Duration
}

// tryStart reports whether a new backfill pass may begin at `now`. If
// the previous pass started less than `window` ago, returns false and
// leaves `last` unchanged. Otherwise records `last = now` and returns
// true.
func (g *dedupeGate) tryStart(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.last.IsZero() && now.Sub(g.last) < g.window {
		return false
	}
	g.last = now
	return true
}
```

- [ ] **Step 4: Run dedupe test, see pass**

```
go test ./cmd/slk/ -run TestBackfill_DedupeWindow -v
```

Expected: PASS.

- [ ] **Step 5: Wire into rtmEventHandler**

Modify `cmd/slk/main.go`. Find the `rtmEventHandler` struct (currently around line 2410). Add fields:

```go
type rtmEventHandler struct {
	// ... existing fields ...

	// backfillGate enforces a 30 s minimum between reconnect-driven
	// backfill passes. Per-handler so each workspace has its own gate.
	backfillGate dedupeGate
}
```

Initialize `backfillGate.window = 30 * time.Second` at construction. Find where `rtmEventHandler` is constructed in `main.go` (search for `&rtmEventHandler{`) and add the field. If it's a multi-line struct literal, add `backfillGate: dedupeGate{window: 30 * time.Second},`.

Then modify `OnConnect` (lines 2664-2688). After the existing section-rebootstrap block (around line 2687), append:

```go
	// Reconnect backfill: catch up on messages missed while the WS
	// was dead. The 30 s dedupe in backfillGate prevents disconnect
	// flaps from spawning overlapping passes. Runs in its own
	// goroutine so the WS read loop isn't blocked on HTTP work.
	//
	// Note on first-connect: the initial WS connect also fires
	// OnConnect, so backfill runs at startup too. This is harmless
	// — synced_at for freshly-bootstrapped channels is current, so
	// most GetHistorySince calls return zero messages quickly. The
	// 4-wide concurrency cap bounds the cost.
	if h.wsCtx != nil && h.db != nil && h.wsCtx.Client != nil {
		if h.backfillGate.tryStart(time.Now()) {
			go func() {
				bf := newBackfiller(h.wsCtx.Client, h.db, h.workspaceID, h.wsCtx.Client.UserID(), h.program, 4, 500)
				_ = bf.run(context.Background())
			}()
		} else {
			debuglog.Backfill("team=%s trigger=reconnect skipped reason=dedupe", h.workspaceID)
		}
	}
```

The plan's earlier draft tried to gate on a `firstReady` field of `WorkspaceContext`. Codebase inspection during plan authoring shows `firstReady` is actually a local `sync.Once` in `main()` (around line 1177) governing which workspace claims the initial active slot, not a per-handler "is bootstrap done" signal. The 30 s dedupe in `backfillGate` is sufficient on its own: even if the very first `OnConnect` fires before any user-driven channel fetches, the backfill is idempotent (every `UpsertMessage` is `INSERT OR UPDATE` on a primary key) and won't conflict with the initial bootstrap path.

- [ ] **Step 6: Build and run all tests**

```
go build ./...
go test ./...
```

Expected: builds cleanly, all tests PASS.

- [ ] **Step 7: Commit**

```
git add cmd/slk/main.go cmd/slk/reconnect_backfill.go cmd/slk/reconnect_backfill_test.go
git commit -m "main: wire reconnect backfill into OnConnect

After WS reconnect, dispatch a backfill goroutine (per workspace)
that catches up on missed messages and thread replies. Gated by a
30 s dedupe so disconnect flaps don't thunder."
```

---

## Task 9: Manual verification + cleanup

- [ ] **Step 1: Build a debug binary and verify**

```
SLK_DEBUG=1 go build -o /tmp/slk-test ./cmd/slk/
```

Run it on the `grant-work` user (or whichever account has UE configured). Open the threads view. Verify:

- Jayana thread is near the top (position 2 or 3), not position 9.
- The unread dot still renders for threads where the heuristic still says Unread.

- [ ] **Step 2: Force a reconnect to test backfill**

Stop the laptop's network for 90 s (turn wifi off, wait, turn it back on). When connection restores, watch `slk-debug.log`:

```
tail -f slk-debug.log | grep '\[backfill\]'
```

Expected lines like:

```
[backfill] team=TUJLNE62Z trigger=reconnect channels=37 start
[backfill] team=TUJLNE62Z channel=C050WUX3W95 oldest=... count=N dur_ms=...
[backfill] team=TUJLNE62Z channel-phase done total_msgs=... dur_ms=...
[backfill] team=TUJLNE62Z thread-phase threads_involved=... done dur_ms=...
[backfill] team=TUJLNE62Z trigger=reconnect total_dur_ms=... status=ok
```

After the backfill completes, switch to the threads view and verify the Emerson thread (and any other previously-missed activity) is now present.

- [ ] **Step 3: Self-review the diff**

```
git log --oneline main..HEAD
git diff main..HEAD --stat
```

Look for accidental scope creep, dead code, leftover debug `fmt.Println`, etc.

- [ ] **Step 4: Run vet and final test sweep**

```
go vet ./...
go test ./...
```

Expected: no vet warnings, all tests pass.

- [ ] **Step 5: Done — push branch or open PR per project convention**

No commit step (nothing to commit).

---

## Self-review checklist

**Spec coverage:**
- [x] Goal 1 (reconnect backfill) → Tasks 2, 5, 6, 7, 8
- [x] Goal 2 (sort by LastReplyTS DESC) → Task 4
- [x] `ChannelsWithMessages` cache helper → Task 2
- [x] `ThreadInvolvesUser` cache helper → Task 3
- [x] `GetHistorySince` with pagination + cap → Task 5
- [x] Backfill channel phase with bounded concurrency → Task 6
- [x] Backfill thread phase + ThreadsListDirtyMsg dispatch → Task 7
- [x] OnConnect wiring + 30 s dedupe → Task 8
- [x] New `[backfill]` debuglog category → Task 1
- [x] Tests for all the above → Tasks 1–8 each include test-first steps
- [x] Manual verification path → Task 9

**Type consistency check:**
- `ChannelSyncRow` defined in Task 2, used in Task 6 (`runChannelPhase` and `backfillOneChannel`). Consistent: `ChannelID string`, `SyncedAt int64`.
- `threadKey` defined in Task 6, used in Task 7 (`runThreadPhase`, `backfillOneThread`). Consistent.
- `newBackfiller` signature: `(client, db, workspaceID, selfUserID, program, concurrency, perChannelCap)`. Used identically in Tasks 6, 7, 8.
- `dedupeGate.tryStart(now time.Time) bool` — defined Task 8, used in Task 8 only.
- `teaSender` interface — Task 7 introduces, Task 8 satisfies via `*tea.Program`.
- `ThreadInvolvesUser(workspaceID, channelID, threadTS, selfUserID string) (bool, error)` — defined Task 3, used in Task 7.
- `GetHistorySince(ctx, channelID, oldest string, maxTotal int) ([]slack.Message, error)` — defined Task 5, used in Task 6.
- `ChannelsWithMessages(workspaceID string) ([]ChannelSyncRow, error)` — defined Task 2, used in Task 6.

**Placeholder scan:** No "TBD", no "add appropriate error handling", every code step has the complete code block, every test step has the actual test code or a clear adaptation note when the surrounding code might vary. The few "verify name X exists in the codebase, substitute if different" instructions (e.g., `db.GetMessage`, `client.GetReplies`, `WorkspaceContext.firstReady`) are deliberate guard rails for an implementer who doesn't have full context — they include the exact substitution rule, not a vague "figure it out".
