package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// TestThreadRendersAvatars guards that, when an avatar function is wired,
// the thread panel renders an avatar gutter for the parent and each reply
// (previously the avatar fn was set but never called by the renderer).
func TestThreadRendersAvatars(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })

	m := New()
	m.SetAvatarFunc(func(userID string) string {
		// Two-row marker standing in for the half-block avatar.
		return "AV" + userID + "\nAV" + userID
	})

	parent := messages.MessageItem{TS: "100.0", UserID: "UP", UserName: "alice", Text: "parent", Timestamp: "10:30 AM"}
	replies := []messages.MessageItem{
		{TS: "101.0", UserID: "UB", UserName: "bob", Text: "a reply", Timestamp: "10:31 AM"},
	}
	m.SetThread(parent, replies, "C1", "100.0")

	out := ansi.Strip(m.View(24, 80))
	if !strings.Contains(out, "AVUP") {
		t.Errorf("expected the parent's avatar (AVUP) in the thread render")
	}
	if !strings.Contains(out, "AVUB") {
		t.Errorf("expected the reply author's avatar (AVUB) in the thread render")
	}

	// Control: with no avatar function, no avatar gutter is rendered.
	m2 := New()
	m2.SetThread(parent, replies, "C1", "100.0")
	if strings.Contains(ansi.Strip(m2.View(24, 80)), "AVU") {
		t.Errorf("no avatar function set, but an avatar gutter rendered")
	}
}
