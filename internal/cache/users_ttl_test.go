package cache

import (
	"testing"
	"time"
)

// TestUpsertUserStampsUpdatedAt verifies that UpsertUser records a sync
// timestamp when the caller leaves UpdatedAt unset — the freshness signal
// the identity-TTL refresh relies on. Without it every row would read as
// epoch-old and re-resolve on every render.
func TestUpsertUserStampsUpdatedAt(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := db.UpsertWorkspace(Workspace{ID: "T1", Name: "team"}); err != nil {
		t.Fatalf("UpsertWorkspace: %v", err)
	}

	before := time.Now().Unix()
	if err := db.UpsertUser(User{ID: "U1", WorkspaceID: "T1", Name: "alice"}); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	u, err := db.GetUser("U1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.UpdatedAt < before {
		t.Errorf("UpdatedAt not stamped: got %d, want >= %d", u.UpdatedAt, before)
	}

	// An explicit UpdatedAt is respected (not overwritten).
	if err := db.UpsertUser(User{ID: "U2", WorkspaceID: "T1", Name: "bob", UpdatedAt: 12345}); err != nil {
		t.Fatalf("UpsertUser explicit: %v", err)
	}
	u2, _ := db.GetUser("U2")
	if u2.UpdatedAt != 12345 {
		t.Errorf("explicit UpdatedAt overwritten: got %d, want 12345", u2.UpdatedAt)
	}
}
