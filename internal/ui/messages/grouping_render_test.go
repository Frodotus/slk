package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// twoSameAuthor builds two consecutive messages from the same author a
// minute apart (same day — tiny epoch ts), with distinct display stamps.
func twoSameAuthor() []MessageItem {
	return []MessageItem{
		{TS: "100.0", UserID: "U1", UserName: "alice", Text: "first message", Timestamp: "3:04 PM"},
		{TS: "160.0", UserID: "U1", UserName: "alice", Text: "second message", Timestamp: "3:05 PM"},
	}
}

func TestAuthorGrouping_CollapsesContinuation(t *testing.T) {
	m := New(twoSameAuthor(), "general")
	m.SetGroupWithinMinutes(5)
	out := ansi.Strip(m.View(20, 60))

	if got := strings.Count(out, "alice"); got != 1 {
		t.Errorf("username should render once for a grouped run, appeared %d times:\n%s", got, out)
	}
	// The continuation's own timestamp is suppressed.
	if strings.Contains(out, "3:05 PM") {
		t.Errorf("continuation timestamp should be hidden, but found 3:05 PM:\n%s", out)
	}
	// Both bodies still render.
	for _, want := range []string{"first message", "second message", "3:04 PM"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in grouped output:\n%s", want, out)
		}
	}
}

func TestAuthorGrouping_DisabledShowsEveryHeader(t *testing.T) {
	m := New(twoSameAuthor(), "general")
	// Default window is 0 (disabled); do not enable it.
	out := ansi.Strip(m.View(20, 60))

	if got := strings.Count(out, "alice"); got != 2 {
		t.Errorf("with grouping disabled both messages keep their header; got %d alice headers:\n%s", got, out)
	}
	if !strings.Contains(out, "3:05 PM") {
		t.Errorf("with grouping disabled the second timestamp must show:\n%s", out)
	}
}

func TestAuthorGrouping_DifferentAuthorBreaksGroup(t *testing.T) {
	msgs := []MessageItem{
		{TS: "100.0", UserID: "U1", UserName: "alice", Text: "hi", Timestamp: "3:04 PM"},
		{TS: "160.0", UserID: "U2", UserName: "bob", Text: "yo", Timestamp: "3:05 PM"},
	}
	m := New(msgs, "general")
	m.SetGroupWithinMinutes(5)
	out := ansi.Strip(m.View(20, 60))

	for _, want := range []string{"alice", "bob", "3:04 PM", "3:05 PM"} {
		if !strings.Contains(out, want) {
			t.Errorf("different authors must each keep a full header; missing %q:\n%s", want, out)
		}
	}
}
