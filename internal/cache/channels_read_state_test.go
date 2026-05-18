package cache

import (
	"testing"
)

func newRSChannel(t *testing.T, db *DB, id, workspaceID string) {
	t.Helper()
	if err := db.UpsertWorkspace(Workspace{ID: workspaceID, Name: "ws"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}
	if err := db.UpsertChannel(Channel{ID: id, WorkspaceID: workspaceID, Name: id, Type: "channel"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
}

func TestUpdateChannelReadState_WritesBothColumns(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	newRSChannel(t, db, "C1", "T1")

	if err := db.UpdateChannelReadState("C1", "1700000000.000001", true); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}
	state, err := db.GetChannelReadState("C1")
	if err != nil {
		t.Fatalf("GetChannelReadState: %v", err)
	}
	if state.LastReadTS != "1700000000.000001" {
		t.Errorf("LastReadTS = %q, want %q", state.LastReadTS, "1700000000.000001")
	}
	if !state.HasUnread {
		t.Errorf("HasUnread = false, want true")
	}
}

func TestUpdateChannelReadState_EmptyTSPreservesExisting(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	newRSChannel(t, db, "C1", "T1")

	if err := db.UpdateChannelReadState("C1", "1700000000.000001", false); err != nil {
		t.Fatalf("first update: %v", err)
	}
	if err := db.UpdateChannelReadState("C1", "", true); err != nil {
		t.Fatalf("second update: %v", err)
	}
	state, err := db.GetChannelReadState("C1")
	if err != nil {
		t.Fatalf("GetChannelReadState: %v", err)
	}
	if state.LastReadTS != "1700000000.000001" {
		t.Errorf("LastReadTS = %q, want preserved %q", state.LastReadTS, "1700000000.000001")
	}
	if !state.HasUnread {
		t.Errorf("HasUnread = false, want true")
	}
}

func TestUpdateChannelReadState_Idempotent(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()
	newRSChannel(t, db, "C1", "T1")

	for i := 0; i < 3; i++ {
		if err := db.UpdateChannelReadState("C1", "1700000000.000001", true); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	state, _ := db.GetChannelReadState("C1")
	if state.LastReadTS != "1700000000.000001" || !state.HasUnread {
		t.Errorf("state = %+v after 3 writes", state)
	}
}
