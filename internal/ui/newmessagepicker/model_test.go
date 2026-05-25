package newmessagepicker

import "testing"

func testUsers() []User {
	return []User{
		{ID: "U1", DisplayName: "Alice Chen", Username: "alice", Recency: 500},
		{ID: "U2", DisplayName: "Bob Singh", Username: "bob", Recency: 400},
		{ID: "U3", DisplayName: "Carla Diaz", Username: "carla", Recency: 300},
		{ID: "U4", DisplayName: "Dan Evans", Username: "dan", Recency: 200},
		{ID: "U5", DisplayName: "Eva Frank", Username: "eva", IsExternal: true, Recency: 100},
	}
}

func TestNew_NotVisibleByDefault(t *testing.T) {
	m := New()
	if m.IsVisible() {
		t.Error("expected new model to not be visible")
	}
}

func TestOpen_MakesVisible(t *testing.T) {
	m := New()
	m.SetUsers(testUsers())
	m.Open()
	if !m.IsVisible() {
		t.Error("expected Open() to make model visible")
	}
}

func TestClose_HidesModel(t *testing.T) {
	m := New()
	m.SetUsers(testUsers())
	m.Open()
	m.Close()
	if m.IsVisible() {
		t.Error("expected Close() to hide model")
	}
}

func TestOpen_ResetsState(t *testing.T) {
	m := New()
	m.SetUsers(testUsers())
	m.Open()
	// Simulate dirty state from a previous session.
	m.query = "old query"
	m.selected["U1"] = struct{}{}
	m.highlight = 3

	m.Close()
	m.Open()

	if m.query != "" {
		t.Errorf("expected empty query after Open, got %q", m.query)
	}
	if len(m.selected) != 0 {
		t.Errorf("expected empty selection after Open, got %d entries", len(m.selected))
	}
	if m.highlight != 0 {
		t.Errorf("expected highlight=0 after Open, got %d", m.highlight)
	}
}

func TestSetCurrentUserID_ExcludesSelfFromList(t *testing.T) {
	users := testUsers()
	m := New()
	m.SetCurrentUserID("U2") // Bob is "self"
	m.SetUsers(users)
	m.Open()

	for _, idx := range m.filtered {
		if m.users[idx].ID == "U2" {
			t.Error("self user U2 should not appear in filtered list")
		}
	}
}
