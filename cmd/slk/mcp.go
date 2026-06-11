package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/gammons/slk/internal/config"
	slkmcp "github.com/gammons/slk/internal/mcp"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/messages"
)

// mcpSocketPath resolves the MCP socket path from config (override) or the
// shared default. Both the running TUI and the `slk mcp` subprocess use it.
func mcpSocketPath(cfg config.Config) string {
	if cfg.MCP.SocketPath != "" {
		return cfg.MCP.SocketPath
	}
	return slkmcp.DefaultSocketPath()
}

// startMCPServer opens the unix socket the `slk mcp` subprocess connects to,
// and wires the App to publish snapshots into it. Returns the listener to
// close on shutdown. Called from the TUI path only when [mcp] enabled.
func startMCPServer(cfg config.Config, app *ui.App, p *tea.Program, imagesDir string) (io.Closer, error) {
	socket := mcpSocketPath(cfg)
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, err
	}
	state := slkmcp.NewState()
	app.SetMCPState(state)
	app.SetMCPImageResolver(func(m messages.MessageItem) []string {
		return cachedMessageImagePaths(m, imagesDir)
	})
	return slkmcp.Serve(socket, &mcpBridge{state: state, prog: p})
}

// mcpBridge answers socket requests from the running TUI: reads the published
// snapshot, and drives a compose draft via the program's message queue.
type mcpBridge struct {
	state *slkmcp.State
	prog  *tea.Program
}

func (b *mcpBridge) Context() slkmcp.Snapshot { return b.state.Get() }

func (b *mcpBridge) SetDraft(text string) slkmcp.DraftResult {
	reply := make(chan slkmcp.DraftResult, 1) // buffered: the reducer never blocks
	b.prog.Send(ui.SetComposeDraftMsg{Text: text, Reply: reply})
	select {
	case r := <-reply:
		return r
	case <-time.After(3 * time.Second):
		return slkmcp.DraftResult{OK: false, Reason: "slk did not respond"}
	}
}

// runMCP is the `slk mcp` subcommand: a stdio MCP server that bridges tool
// calls to a running slk over its unix socket. Blocks until stdin closes.
func runMCP() error {
	cfg, _ := config.Load(filepath.Join(xdgConfig(), "config.toml")) // best-effort
	client := slkmcp.NewClient(mcpSocketPath(cfg))

	s := mcpserver.NewMCPServer("slk", version)

	s.AddTool(
		mcpgo.NewTool("slk_get_context",
			mcpgo.WithDescription("Read what the user is currently looking at in slk: the active workspace and channel, the selected message, the open thread (parent + replies) if any, and recent channel messages for context. Returns JSON. slk must be running with [mcp] enabled.")),
		func(ctx context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			snap, err := client.Context()
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			out, _ := json.MarshalIndent(snap, "", "  ")
			return mcpgo.NewToolResultText(string(out)), nil
		},
	)

	s.AddTool(
		mcpgo.NewTool("slk_set_draft",
			mcpgo.WithDescription("Put a drafted message into slk's compose box — the open thread's composer if a thread is open, otherwise the channel composer — for the user to review, edit, and send. slk does NOT send; the user always presses Enter. Call slk_get_context first to know what is selected."),
			mcpgo.WithString("text", mcpgo.Required(), mcpgo.Description("The draft message text to place in the compose box."))),
		func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			text, err := req.RequireString("text")
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			dr, err := client.SetDraft(text)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			if !dr.OK {
				return mcpgo.NewToolResultError("could not set draft: " + dr.Reason), nil
			}
			return mcpgo.NewToolResultText(fmt.Sprintf(
				"Draft placed in the %s composer for #%s. Ask the user to review and press Enter to send.",
				dr.Target, dr.Channel)), nil
		},
	)

	return mcpserver.ServeStdio(s)
}
