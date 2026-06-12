package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
)

// TestThreadAuthorGrouping_CollapsesConsecutiveReplies verifies that two
// consecutive same-author replies within the window collapse: the author's
// header renders once, and the continuation reply's timestamp is hidden.
func TestThreadAuthorGrouping_CollapsesConsecutiveReplies(t *testing.T) {
	parent := messages.MessageItem{
		TS: "50.0", UserID: "U1", UserName: "alice", Text: "the question", Timestamp: "3:00 PM",
	}
	replies := []messages.MessageItem{
		{TS: "100.0", UserID: "U2", UserName: "bob", Text: "first reply", Timestamp: "3:04 PM"},
		{TS: "160.0", UserID: "U2", UserName: "bob", Text: "second reply", Timestamp: "3:05 PM"},
	}

	m := New()
	m.SetThread(parent, replies, "C1", parent.TS)
	m.SetGroupWithinMinutes(5)
	out := ansi.Strip(m.View(30, 70))

	if got := strings.Count(out, "bob"); got != 1 {
		t.Errorf("grouped replies should show the author once; got %d 'bob' headers:\n%s", got, out)
	}
	if strings.Contains(out, "3:05 PM") {
		t.Errorf("continuation reply timestamp should be hidden, found 3:05 PM:\n%s", out)
	}
	for _, want := range []string{"alice", "first reply", "second reply", "3:04 PM"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in grouped thread output:\n%s", want, out)
		}
	}
}

// TestThreadAuthorGrouping_DisabledShowsEveryHeader is the control: with the
// window at 0 (default) both replies keep their header.
func TestThreadAuthorGrouping_DisabledShowsEveryHeader(t *testing.T) {
	parent := messages.MessageItem{TS: "50.0", UserID: "U1", UserName: "alice", Text: "q", Timestamp: "3:00 PM"}
	replies := []messages.MessageItem{
		{TS: "100.0", UserID: "U2", UserName: "bob", Text: "first reply", Timestamp: "3:04 PM"},
		{TS: "160.0", UserID: "U2", UserName: "bob", Text: "second reply", Timestamp: "3:05 PM"},
	}
	m := New()
	m.SetThread(parent, replies, "C1", parent.TS)
	// leave grouping disabled (default 0)
	out := ansi.Strip(m.View(30, 70))

	if got := strings.Count(out, "bob"); got != 2 {
		t.Errorf("with grouping off both replies keep a header; got %d 'bob' headers:\n%s", got, out)
	}
	if !strings.Contains(out, "3:05 PM") {
		t.Errorf("with grouping off the second timestamp must show:\n%s", out)
	}
}
