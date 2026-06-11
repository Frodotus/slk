// internal/ui/mcp_bridge.go
//
// TUI side of the MCP integration: publishes a snapshot of the current
// focus (selected message / open thread / channel + recent messages) for
// the socket server to read, and handles SetComposeDraftMsg by filling the
// active composer for the user to review and send. slk never sends here.
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/mcp"
	"github.com/gammons/slk/internal/ui/messages"
)

// SetComposeDraftMsg asks the App to put Text into the active composer (the
// thread composer when a thread is open, otherwise the channel composer)
// and drop into insert mode. Reply, if non-nil and buffered, receives the
// outcome. Dispatched by the MCP socket bridge via p.Send.
type SetComposeDraftMsg struct {
	Text  string
	Reply chan mcp.DraftResult
}

// SetMCPState wires the shared snapshot state the App publishes to. Nil
// (the default) disables publishing.
func (a *App) SetMCPState(s *mcp.State) { a.mcpState = s }

var reduceMCP reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	if m, ok := msg.(SetComposeDraftMsg); ok {
		return a.setComposeDraft(m), true
	}
	return nil, false
}

// publishMCP refreshes the shared snapshot from current App state. A no-op
// when MCP is disabled; cheap otherwise. Called at the end of every Update.
func (a *App) publishMCP() {
	if a.mcpState == nil {
		return
	}
	a.mcpState.Set(a.buildSnapshot())
}

func (a *App) buildSnapshot() mcp.Snapshot {
	snap := mcp.Snapshot{Workspace: a.workspaceNameForActive()}
	if a.activeChannelID != "" {
		name, ctype := a.channelNameType(a.activeChannelID)
		snap.Channel = &mcp.Channel{ID: a.activeChannelID, Name: name, Type: ctype}
	}
	if sel, ok := a.messagepane.SelectedMessage(); ok {
		snap.SelectedMessage = toMCPMessage(sel)
	}
	if a.threadVisible && a.threadPanel.ThreadTS() != "" {
		snap.Thread = mcp.Thread{
			Open:    true,
			Parent:  toMCPMessage(a.threadPanel.ParentMsg()),
			Replies: toMCPMessages(a.threadPanel.Replies()),
		}
	}
	msgs := a.messagepane.Messages()
	const recentN = 20
	if len(msgs) > recentN {
		msgs = msgs[len(msgs)-recentN:]
	}
	snap.RecentMessages = toMCPMessages(msgs)
	return snap
}

// setComposeDraft fills the active composer with the draft and enters
// insert mode so the user can review, edit, and send.
func (a *App) setComposeDraft(m SetComposeDraftMsg) tea.Cmd {
	reply := func(r mcp.DraftResult) {
		if m.Reply != nil {
			m.Reply <- r // Reply is buffered (cap 1) — never blocks the UI goroutine
		}
	}
	if a.activeChannelID == "" {
		reply(mcp.DraftResult{OK: false, Reason: "no active channel"})
		return nil
	}
	chName, _ := a.channelNameType(a.activeChannelID)
	a.SetMode(ModeInsert)
	if a.threadVisible {
		a.threadCompose.SetValue(m.Text)
		a.focusedPanel = PanelThread
		reply(mcp.DraftResult{OK: true, Target: "thread", Channel: chName})
		return a.threadCompose.Focus()
	}
	a.compose.SetValue(m.Text)
	a.focusedPanel = PanelMessages
	reply(mcp.DraftResult{OK: true, Target: "channel", Channel: chName})
	return a.compose.Focus()
}

func (a *App) channelNameType(channelID string) (name, ctype string) {
	for _, it := range a.sidebar.Items() {
		if it.ID == channelID {
			return it.Name, it.Type
		}
	}
	return "", ""
}

func toMCPMessage(m messages.MessageItem) *mcp.Message {
	if m.TS == "" {
		return nil
	}
	out := &mcp.Message{TS: m.TS, User: m.UserName, Text: m.Text}
	for _, r := range m.Reactions {
		out.Reactions = append(out.Reactions, mcp.Reaction{Emoji: r.Emoji, Count: r.Count})
	}
	return out
}

func toMCPMessages(items []messages.MessageItem) []mcp.Message {
	out := make([]mcp.Message, 0, len(items))
	for _, it := range items {
		if m := toMCPMessage(it); m != nil {
			out = append(out, *m)
		}
	}
	return out
}
