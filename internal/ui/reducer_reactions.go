// internal/ui/reducer_reactions.go
//
// Reaction-family reducer for App.Update (Phase 4g).
//
// Owns the three Update arms for reaction WS echoes / API results:
//
//	ReactionAddedMsg    - server confirmed a reaction was added.
//	                      Applied unconditionally; updateReactionOnMessage
//	                      is idempotent per (emoji, userID), so the echo
//	                      of our own optimistic update collapses to one
//	                      count while reactions from other users (or our
//	                      own from another device) still merge in live.
//	ReactionRemovedMsg  - server confirmed a reaction was removed
//	                      (same idempotent application as Added).
//	ReactionSentMsg     - our reaction API call completed. No-op
//	                      today: the optimistic update is already
//	                      on screen and a failed call has no surface.
//
// Free reducer (no dedicated controller) because reactions are a
// per-message annotation with no cross-message invariant and no
// in-flight state to track. The update helper itself
// (updateReactionOnMessage) stays on App because it touches
// messagepane / threadPanel caches.
package ui

import (
	tea "charm.land/bubbletea/v2"
)

var reduceReactions reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case ReactionAddedMsg:
		// Apply every reaction event, including echoes of our own.
		// updateReactionOnMessage is idempotent per (emoji, userID), so
		// our optimistic update and the WS echo that follows it collapse
		// to a single count — while reactions we make from another device
		// (web/mobile), which have no optimistic update here, still show
		// up live instead of being dropped.
		a.updateReactionOnMessage(m.ChannelID, m.MessageTS, m.Emoji, m.UserID, false)
		return nil, true

	case ReactionRemovedMsg:
		a.updateReactionOnMessage(m.ChannelID, m.MessageTS, m.Emoji, m.UserID, true)
		return nil, true

	case ReactionSentMsg:
		_ = m
		// API call completed. Optimistic update is already on
		// screen; a failed call has no user surface today.
		return nil, true
	}
	return nil, false
}
