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
	// The emoji and count render inside one styled span (no ANSI between
	// them), so assert the contiguous "🎧3" — a bare "3" could match a digit
	// inside an ANSI escape.
	if !strings.Contains(line, "🎧3") {
		t.Errorf("expected the huddle badge 🎧3 on the row:\n%q", line)
	}
}

// A multi-digit participant count renders fully (the width budget scales
// with the digit count rather than a fixed 4 columns).
func TestHuddleBadge_MultiDigitCount(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel", HuddleCount: 12},
	})
	m.ToggleCollapse("Channels")
	view := m.View(10, 40)
	if !strings.Contains(view, "🎧12") {
		t.Errorf("expected the full 🎧12 badge:\n%s", view)
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
