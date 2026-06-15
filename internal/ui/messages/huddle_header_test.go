package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The channel header shows a huddle line when one is set, and drops it when
// cleared.
func TestChannelHuddleHeaderLine(t *testing.T) {
	m := New([]MessageItem{
		{TS: "1.0", UserID: "U1", UserName: "alice", Text: "hi", Timestamp: "3:04 PM"},
	}, "general")

	out := ansi.Strip(m.View(20, 60))
	if strings.Contains(out, "Huddle") {
		t.Fatalf("no huddle set, but header shows a huddle line:\n%s", out)
	}

	m.SetChannelHuddle("🎧 Huddle · alice, bob")
	out = ansi.Strip(m.View(20, 60))
	if !strings.Contains(out, "Huddle · alice, bob") {
		t.Errorf("expected the huddle header line in output:\n%s", out)
	}

	m.SetChannelHuddle("")
	out = ansi.Strip(m.View(20, 60))
	if strings.Contains(out, "Huddle") {
		t.Errorf("cleared huddle, but header still shows a huddle line:\n%s", out)
	}
}
