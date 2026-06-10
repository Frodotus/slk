// internal/ui/reducer_extcmd.go
//
// External-command feature: the `x` hotkey opens a picker of configured
// commands to run against the selected message. The message text is piped
// to the command's stdin and metadata (incl. cached image paths) is passed
// via SLK_* env vars (see internal/extcmd). Default execution is async with
// a completion toast; per-command flags switch to a captured-output overlay
// or an interactive (terminal-attached) run, with an optional confirm.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/extcmd"
	"github.com/gammons/slk/internal/ui/messages"
)

// SetExternalCommands registers the configured commands and seeds the
// picker's item list.
func (a *App) SetExternalCommands(cmds []config.ExternalCommand) {
	a.externalCommands = cmds
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	a.extCmdPicker.SetItems(names)
}

// SetExtCmdImageResolver injects the function mapping a message to the
// on-disk paths of its cached images (owned by main, which holds the
// image cache). Nil means no images are passed.
func (a *App) SetExtCmdImageResolver(fn func(messages.MessageItem) []string) {
	a.extCmdImageResolver = fn
}

var reduceExtCmd reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case EnterExternalCmdMsg:
		return a.openExtCmdPicker(), true
	case ExternalCmdDoneMsg:
		return a.finishExtCmd(m), true
	}
	return nil, false
}

// openExtCmdPicker captures the selected message (channel or thread) and
// opens the command picker. Toasts instead when nothing is configured or
// no message is selected.
func (a *App) openExtCmdPicker() tea.Cmd {
	if len(a.externalCommands) == 0 {
		return func() tea.Msg { return ToastMsg{Text: "No external commands configured"} }
	}
	var target messages.MessageItem
	var channelID string
	ok := false
	if a.focusedPanel == PanelThread {
		if r := a.threadPanel.SelectedReply(); r != nil {
			target, channelID, ok = *r, a.threadPanel.ChannelID(), true
		}
	} else if mm, has := a.messagepane.SelectedMessage(); has {
		target, channelID, ok = mm, a.activeChannelID, true
	}
	if !ok {
		return func() tea.Msg { return ToastMsg{Text: "No message selected"} }
	}
	a.extCmdTargetMsg = target
	a.extCmdTargetChannel = channelID
	a.extCmdPicker.Open()
	a.SetMode(ModeExtCmd)
	return nil
}

// finishExtCmd routes a completed non-interactive run to either the
// captured-output overlay or a completion toast.
func (a *App) finishExtCmd(m ExternalCmdDoneMsg) tea.Cmd {
	if m.Capture && !m.Failed {
		out := m.Stdout
		if strings.TrimSpace(out) == "" {
			out = "(no output)"
		}
		a.extCmdOutput = out
		a.extCmdOutputName = m.Name
		a.SetMode(ModeExtCmdOutput)
		return nil
	}
	var text string
	if m.Failed {
		detail := extcmd.FirstLine(m.Stderr)
		if detail == "" {
			detail = fmt.Sprintf("exit %d", m.ExitCode)
		}
		text = fmt.Sprintf("✗ %q failed: %s", m.Name, detail)
	} else {
		text = fmt.Sprintf("✓ Ran %q", m.Name)
	}
	return func() tea.Msg { return ToastMsg{Text: text} }
}

func (a *App) extCmdContext() extcmd.Context {
	msg := a.extCmdTargetMsg
	ctx := extcmd.Context{
		Text:        msg.Text,
		UserName:    msg.UserName,
		UserID:      msg.UserID,
		TS:          msg.TS,
		ChannelID:   a.extCmdTargetChannel,
		ChannelName: a.channelDisplayName(a.extCmdTargetChannel),
		Permalink:   a.messagePermalink(a.extCmdTargetChannel, msg.TS),
	}
	if a.extCmdImageResolver != nil {
		ctx.ImagePaths = a.extCmdImageResolver(msg)
	}
	return ctx
}

// runExternalCommand returns the tea.Cmd that executes a command against
// the captured target message. Interactive commands suspend the TUI via
// ExecProcess; others run async and report back via ExternalCmdDoneMsg.
func (a *App) runExternalCommand(c config.ExternalCommand) tea.Cmd {
	ctx := a.extCmdContext()
	if c.Interactive {
		cmd := extcmd.BuildCmd(c, ctx)
		name := c.Name
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return ToastMsg{Text: fmt.Sprintf("✗ %q failed: %v", name, err)}
			}
			return ToastMsg{Text: fmt.Sprintf("✓ Ran %q", name)}
		})
	}
	name, capture := c.Name, c.CaptureOutput
	return func() tea.Msg {
		res := extcmd.Run(c, ctx)
		return ExternalCmdDoneMsg{
			Name:     name,
			Stdout:   res.Stdout,
			Stderr:   res.Stderr,
			ExitCode: res.ExitCode,
			Failed:   res.Err != nil,
			Capture:  capture,
		}
	}
}

// channelDisplayName returns the sidebar name for a channel ID, or "".
func (a *App) channelDisplayName(channelID string) string {
	for _, it := range a.sidebar.Items() {
		if it.ID == channelID {
			return it.Name
		}
	}
	return ""
}

// messagePermalink builds a best-effort archive URL for a message
// (empty when the workspace domain isn't known).
func (a *App) messagePermalink(channelID, ts string) string {
	domain := a.workspaceDomains[a.activeTeamID]
	if domain == "" || channelID == "" || ts == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s",
		domain, channelID, strings.ReplaceAll(ts, ".", ""))
}

// handleExtCmdMode forwards keys to the command picker; on selection it
// runs the command (via a confirm prompt first when configured).
func handleExtCmdMode(a *App, msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()
	switch msg.Key().Code {
	case tea.KeyEnter:
		keyStr = "enter"
	case tea.KeyEscape:
		keyStr = "esc"
	case tea.KeyUp:
		keyStr = "up"
	case tea.KeyDown:
		keyStr = "down"
	case tea.KeyBackspace:
		keyStr = "backspace"
	}

	result := a.extCmdPicker.HandleKey(keyStr)
	if result != nil {
		c := a.externalCommands[result.Index]
		runCmd := a.runExternalCommand(c)
		if c.Confirm {
			a.confirmPrompt.Open(
				fmt.Sprintf("Run %q?", c.Name),
				strings.Join(c.Argv, " "),
				runCmd,
			)
			a.SetMode(ModeConfirm)
			return nil
		}
		a.SetMode(ModeNormal)
		return runCmd
	}
	if !a.extCmdPicker.IsVisible() {
		a.SetMode(ModeNormal)
	}
	return nil
}

// handleExtCmdOutputMode closes the captured-output overlay.
func handleExtCmdOutputMode(a *App, msg tea.KeyMsg) tea.Cmd {
	switch msg.Key().Code {
	case tea.KeyEscape, tea.KeyEnter:
		a.extCmdOutput = ""
		a.SetMode(ModeNormal)
		return nil
	}
	if msg.String() == "q" {
		a.extCmdOutput = ""
		a.SetMode(ModeNormal)
	}
	return nil
}
