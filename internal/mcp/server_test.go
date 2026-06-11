package mcp

import (
	"path/filepath"
	"testing"
)

type fakeBridge struct {
	snap      Snapshot
	lastDraft string
	result    DraftResult
}

func (f *fakeBridge) Context() Snapshot                { return f.snap }
func (f *fakeBridge) SetDraft(text string) DraftResult { f.lastDraft = text; return f.result }

// TestServeRoundTrip is the headline of the socket layer: a Client talking
// to a Serve-backed Bridge gets the snapshot and can set a draft.
func TestServeRoundTrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mcp.sock")
	fb := &fakeBridge{
		snap: Snapshot{
			Workspace:       "Viilu",
			Channel:         &Channel{ID: "C1", Name: "general", Type: "channel"},
			SelectedMessage: &Message{TS: "1.0", User: "alice", Text: "hi"},
		},
		result: DraftResult{OK: true, Target: "channel", Channel: "general"},
	}
	ln, err := Serve(sock, fb)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer ln.Close()

	c := NewClient(sock)

	snap, err := c.Context()
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if snap.Workspace != "Viilu" || snap.SelectedMessage == nil || snap.SelectedMessage.Text != "hi" {
		t.Errorf("unexpected snapshot: %+v", snap)
	}

	dr, err := c.SetDraft("draft text")
	if err != nil {
		t.Fatalf("SetDraft: %v", err)
	}
	if !dr.OK || dr.Target != "channel" || dr.Channel != "general" {
		t.Errorf("unexpected draft result: %+v", dr)
	}
	if fb.lastDraft != "draft text" {
		t.Errorf("bridge received %q, want %q", fb.lastDraft, "draft text")
	}
}

func TestClientNotRunning(t *testing.T) {
	c := NewClient(filepath.Join(t.TempDir(), "absent.sock"))
	if _, err := c.Context(); err == nil {
		t.Error("expected an error when the socket is absent")
	}
}
