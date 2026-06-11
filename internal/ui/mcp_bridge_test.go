package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/mcp"
	"github.com/gammons/slk/internal/ui/messages"
)

// TestSetComposeDraft is the headline of the MCP write path: a
// SetComposeDraftMsg fills the channel composer and acks where it landed.
func TestSetComposeDraft(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.activeChannelID = "C1"

	reply := make(chan mcp.DraftResult, 1)
	a.Update(SetComposeDraftMsg{Text: "hello from ai", Reply: reply})

	if got := a.compose.Value(); got != "hello from ai" {
		t.Errorf("compose value = %q, want %q", got, "hello from ai")
	}
	select {
	case r := <-reply:
		if !r.OK || r.Target != "channel" {
			t.Errorf("draft reply = %+v, want OK channel", r)
		}
	default:
		t.Error("expected a draft reply")
	}
}

func TestSetComposeDraftNoChannel(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	// activeChannelID is empty
	reply := make(chan mcp.DraftResult, 1)
	a.Update(SetComposeDraftMsg{Text: "x", Reply: reply})
	if r := <-reply; r.OK {
		t.Errorf("expected failure with no active channel, got %+v", r)
	}
}

// TestBuildSnapshot covers the read path: channel + recent messages +
// selected message are reflected in the published snapshot.
func TestBuildSnapshot(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.activeChannelID = "C1"
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", UserName: "alice", Text: "hi"},
		{TS: "2.0", UserName: "bob", Text: "yo"},
	})

	snap := a.buildSnapshot()
	if snap.Channel == nil || snap.Channel.ID != "C1" {
		t.Errorf("channel = %+v, want C1", snap.Channel)
	}
	if len(snap.RecentMessages) != 2 || snap.RecentMessages[0].Text != "hi" {
		t.Errorf("recent messages = %+v", snap.RecentMessages)
	}
	// SetMessages selects the newest, so it's the selected message.
	if snap.SelectedMessage == nil || snap.SelectedMessage.Text != "yo" {
		t.Errorf("selected = %+v, want 'yo'", snap.SelectedMessage)
	}
}
