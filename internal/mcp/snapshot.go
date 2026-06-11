// Package mcp exposes slk's current focus (selected message / open thread /
// active channel) to a local MCP client and lets that client drop a draft
// into slk's compose box. slk never sends — it only reads context and fills
// the input for the user to review and send.
//
// The running TUI serves a tiny request/response protocol over a unix-domain
// socket (Serve + Bridge). The `slk mcp` subcommand is a stdio MCP adapter
// that connects to that socket (Client). This package is stdlib-only; the
// MCP/stdio layer lives in cmd/slk.
package mcp

import (
	"os"
	"path/filepath"
	"sync"
)

// Reaction is one reaction pill on a message.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

// Message is a single message in the snapshot.
type Message struct {
	TS        string     `json:"ts"`
	User      string     `json:"user"`
	Text      string     `json:"text"`
	Reactions []Reaction `json:"reactions,omitempty"`
	// Images holds on-disk paths of the message's already-cached images
	// (the same ones slk has rendered). An MCP client can read these to
	// transcribe / describe screenshots. Empty when there are no images or
	// none are cached yet.
	Images []string `json:"images,omitempty"`
}

// Channel identifies the active channel.
type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// Thread is the open thread, if any.
type Thread struct {
	Open    bool      `json:"open"`
	Parent  *Message  `json:"parent,omitempty"`
	Replies []Message `json:"replies,omitempty"`
}

// Snapshot is the current focus exposed to the MCP client.
type Snapshot struct {
	Workspace       string    `json:"workspace"`
	Channel         *Channel  `json:"channel,omitempty"`
	SelectedMessage *Message  `json:"selected_message,omitempty"`
	Thread          Thread    `json:"thread"`
	RecentMessages  []Message `json:"recent_messages,omitempty"`
}

// State holds the latest Snapshot for concurrent access: the TUI publishes
// it from its Update loop, the socket server reads it. The zero value is a
// usable empty state.
type State struct {
	mu   sync.RWMutex
	snap Snapshot
}

// NewState returns an empty State.
func NewState() *State { return &State{} }

// Set replaces the published snapshot.
func (s *State) Set(snap Snapshot) {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

// Get returns a copy of the current snapshot.
func (s *State) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// DefaultSocketPath returns the standard socket location,
// $XDG_DATA_HOME/slk/mcp.sock (falling back to ~/.local/share/slk/mcp.sock).
// Both the TUI (server) and the `slk mcp` subcommand (client) use this so
// they agree without configuration.
func DefaultSocketPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "slk-mcp.sock")
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "slk", "mcp.sock")
}
