package service

import (
	"sort"
	"sync"
)

// HuddleStore tracks which conversations currently have an active huddle, who
// is in each, and the link to join it. It is sourced entirely from the
// browser-protocol WebSocket sh_room_join / sh_room_leave events (event-driven;
// huddle state is ephemeral, so there is no REST bootstrap). Those events carry
// the authoritative full participant list for the room, so the store is
// updated by replacement (Set), not by per-user deltas. All methods are safe
// for concurrent use.
type HuddleStore struct {
	mu        sync.RWMutex
	byChannel map[string]*huddleState
}

type huddleState struct {
	participants map[string]bool
	url          string // room.huddle_link, for the join handoff
}

// NewHuddleStore returns an empty store (no active huddles).
func NewHuddleStore() *HuddleStore {
	return &HuddleStore{byChannel: map[string]*huddleState{}}
}

// Set replaces the participant list (and join URL) for channelID's huddle from
// an authoritative sh_room_join / sh_room_leave event. An empty participants
// slice clears the huddle. It returns transition flags so callers can fire a
// one-shot toast when a huddle starts and refresh the UI when one ends:
//   - becameActive: this call turned an inactive channel into an active huddle.
//   - becameInactive: this call cleared the last participant.
//
// Empty channelID is ignored. Idempotent: re-applying the same non-empty list
// reports no transition.
func (s *HuddleStore) Set(channelID string, participants []string, url string) (becameActive, becameInactive bool) {
	if channelID == "" {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.byChannel[channelID]
	wasActive := prev != nil && len(prev.participants) > 0

	set := make(map[string]bool, len(participants))
	for _, u := range participants {
		if u != "" {
			set[u] = true
		}
	}
	if len(set) == 0 {
		delete(s.byChannel, channelID)
	} else {
		s.byChannel[channelID] = &huddleState{participants: set, url: url}
	}
	nowActive := len(set) > 0
	return !wasActive && nowActive, wasActive && !nowActive
}

// Active reports whether channelID currently has an active huddle.
func (s *HuddleStore) Active(channelID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.byChannel[channelID]
	return st != nil && len(st.participants) > 0
}

// Count returns the number of participants in channelID's huddle (0 if none).
func (s *HuddleStore) Count(channelID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st := s.byChannel[channelID]; st != nil {
		return len(st.participants)
	}
	return 0
}

// Participants returns the user IDs in channelID's huddle, sorted for
// deterministic rendering, or nil when there is no active huddle.
func (s *HuddleStore) Participants(channelID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.byChannel[channelID]
	if st == nil || len(st.participants) == 0 {
		return nil
	}
	out := make([]string, 0, len(st.participants))
	for id := range st.participants {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// URL returns the join link for channelID's huddle (room.huddle_link), or ""
// when there is no active huddle or the event carried no link.
func (s *HuddleStore) URL(channelID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st := s.byChannel[channelID]; st != nil {
		return st.url
	}
	return ""
}

// ActiveChannels returns every channelID with an active huddle, sorted.
func (s *HuddleStore) ActiveChannels() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.byChannel))
	for ch := range s.byChannel {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}
