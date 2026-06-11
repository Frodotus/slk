package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
)

// TestKeepFocusOnListChannelSelect is the headline of the
// keep_focus_on_list config: selecting a channel keeps keyboard focus on
// the sidebar when enabled, and moves it into the messages pane (the
// long-standing default) when not. Drives the real ChannelSelectedMsg
// reducer through Update so it exercises the production path.
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

// TestKeepFocusOnListThreadFromMessage drives the real Enter keystroke
// (handleEnter) on a selected channel message: by default it opens and
// focuses the thread pane; with keep_focus_on_list the thread still opens
// but focus stays on the message list.
func TestKeepFocusOnListThreadFromMessage(t *testing.T) {
	mk := func(keep bool) *App {
		a := NewApp()
		_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
		a.SetKeepFocusOnList(keep)
		a.activeChannelID = "C1"
		a.focusedPanel = PanelMessages
		a.messagepane.SetMessages([]messages.MessageItem{{TS: "100.0", Text: "hi"}})
		return a
	}

	a := mk(false)
	a.handleEnter()
	if a.focusedPanel != PanelThread {
		t.Fatalf("default: Enter on a message should focus the thread pane, got %v", a.focusedPanel)
	}

	b := mk(true)
	b.handleEnter()
	if !b.threadVisible {
		t.Fatal("keep_focus_on_list: thread should still open")
	}
	if b.focusedPanel != PanelMessages {
		t.Errorf("keep_focus_on_list: Enter on a message should keep focus on the message list, got %v", b.focusedPanel)
	}
}

// TestKeepFocusOnListThreadFromThreadsView drives Enter (handleEnter) on a
// highlighted summary in the Threads view: by default it focuses the
// thread pane; with keep_focus_on_list focus stays on the threads list so
// the user can keep walking it.
func TestKeepFocusOnListThreadFromThreadsView(t *testing.T) {
	mk := func(keep bool) *App {
		a := NewApp()
		_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
		a.SetKeepFocusOnList(keep)
		a.view = ViewThreads
		a.focusedPanel = PanelMessages
		a.threadsView.SetSummaries([]cache.ThreadSummary{
			{ChannelID: "C1", ThreadTS: "100.0", ParentTS: "100.0", ParentText: "hi"},
		})
		return a
	}

	a := mk(false)
	a.handleEnter()
	if a.focusedPanel != PanelThread {
		t.Fatalf("default: Enter in threads view should focus the thread pane, got %v", a.focusedPanel)
	}

	b := mk(true)
	b.handleEnter()
	if b.focusedPanel != PanelMessages {
		t.Errorf("keep_focus_on_list: Enter in threads view should keep focus on the threads list, got %v", b.focusedPanel)
	}
}
