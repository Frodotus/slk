package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// TestKeepFocusOnListChannelSelect is the headline of the
// keep_focus_on_list config: selecting a channel keeps keyboard focus on
// the sidebar when enabled, and moves it into the messages pane (the
// long-standing default) when not.
func TestKeepFocusOnListChannelSelect(t *testing.T) {
	// Default: focus follows the selection into the messages pane.
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.focusedPanel = PanelSidebar
	a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	if a.focusedPanel != PanelMessages {
		t.Fatalf("default: expected focus to move to PanelMessages, got %v", a.focusedPanel)
	}

	// keep_focus_on_list: focus stays on the sidebar so the user can keep
	// browsing channels with j/k + Enter.
	b := NewApp()
	_, _ = b.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	b.SetKeepFocusOnList(true)
	b.focusedPanel = PanelSidebar
	b.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	if b.focusedPanel != PanelSidebar {
		t.Errorf("keep_focus_on_list: expected focus to stay on PanelSidebar, got %v", b.focusedPanel)
	}
}

// TestKeepFocusOnListThreadOpen verifies opening a thread keeps focus on
// the message list when keep_focus_on_list is set (the thread panel still
// becomes visible), versus moving focus into the thread pane by default.
func TestKeepFocusOnListThreadOpen(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "hi"}

	// Default: opening a thread focuses the thread pane.
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.focusedPanel = PanelMessages
	a.openThreadPanel(parent, "C1", "100.0")
	if a.focusedPanel != PanelThread {
		t.Fatalf("default: expected focus to move to PanelThread, got %v", a.focusedPanel)
	}

	// keep_focus_on_list: focus stays on the message list, thread still opens.
	b := NewApp()
	_, _ = b.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	b.SetKeepFocusOnList(true)
	b.focusedPanel = PanelMessages
	b.openThreadPanel(parent, "C1", "100.0")
	if b.focusedPanel != PanelMessages {
		t.Errorf("keep_focus_on_list: expected focus to stay on PanelMessages, got %v", b.focusedPanel)
	}
	if !b.threadVisible {
		t.Error("keep_focus_on_list: thread panel should still become visible")
	}
}
