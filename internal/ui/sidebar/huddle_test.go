package sidebar

import (
	"strings"
	"testing"
)

// A channel with an active huddle renders a "🎧N" badge on its row.
func TestHuddleBadge_RendersWithCount(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel", HuddleCount: 3},
	})
	m.ToggleCollapse("Channels") // expand so the row renders
	view := m.View(10, 30)

	var line string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "general") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("general row not rendered:\n%s", view)
	}
	if !strings.Contains(line, "🎧") {
		t.Errorf("expected huddle headphone glyph on the row:\n%q", line)
	}
	if !strings.Contains(line, "3") {
		t.Errorf("expected participant count 3 in the badge:\n%q", line)
	}
}

// A channel without a huddle has no badge.
func TestHuddleBadge_AbsentWhenNoHuddle(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel", HuddleCount: 0},
	})
	m.ToggleCollapse("Channels")
	view := m.View(10, 30)
	if strings.Contains(view, "🎧") {
		t.Errorf("no huddle should mean no headphone glyph:\n%s", view)
	}
}
