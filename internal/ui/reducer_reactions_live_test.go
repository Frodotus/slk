package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// TestReactionAddedAppliesOwnUserLive is the headline of the live-reactions
// fix: a ReactionAddedMsg by OUR OWN user that has no local optimistic
// update (e.g. made from web/mobile) must be applied by the reducer. The
// previous code dropped any echo whose userID == currentUserID.
func TestReactionAddedAppliesOwnUserLive(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.SetCurrentUserID("ME")
	a.messagepane.SetMessages([]messages.MessageItem{{TS: "100.0", Text: "hi"}})

	// No optimistic update here — this is purely the WS echo of a reaction
	// we made elsewhere. It must show live.
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "ME", Emoji: "tada"})
	msg, _ := a.messagepane.SelectedMessage()
	if len(msg.Reactions) != 1 || msg.Reactions[0].Count != 1 {
		t.Fatalf("own-device reaction must show live, got %+v", msg.Reactions)
	}

	// Optimistic add + its own WS echo must collapse to one count —
	// idempotency is what makes dropping the self-filter safe.
	a.updateReactionOnMessage("C1", "100.0", "rocket", "ME", false) // optimistic
	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "ME", Emoji: "rocket"})
	msg, _ = a.messagepane.SelectedMessage()
	var rocketCount int
	found := false
	for _, r := range msg.Reactions {
		if r.Emoji == "rocket" {
			rocketCount = r.Count
			found = true
		}
	}
	if !found || rocketCount != 1 {
		t.Errorf("optimistic + echo must collapse to count 1, got %+v", msg.Reactions)
	}
}
