package service

import (
	"reflect"
	"testing"
)

func TestHuddleStore_SetTransitions(t *testing.T) {
	s := NewHuddleStore()

	// First non-empty list -> becomes active.
	active, inactive := s.Set("C1", []string{"U1"}, "https://huddle/C1")
	if !active || inactive {
		t.Fatalf("first set: active=%v inactive=%v, want active", active, inactive)
	}
	if !s.Active("C1") || s.Count("C1") != 1 {
		t.Fatalf("C1 should have 1 participant, got active=%v count=%d", s.Active("C1"), s.Count("C1"))
	}
	if s.URL("C1") != "https://huddle/C1" {
		t.Errorf("URL(C1) = %q, want the join link", s.URL("C1"))
	}

	// Grows to two -> still active, no new transition.
	active, inactive = s.Set("C1", []string{"U1", "U2"}, "https://huddle/C1")
	if active || inactive {
		t.Errorf("growth should not transition: active=%v inactive=%v", active, inactive)
	}
	if s.Count("C1") != 2 {
		t.Errorf("C1 count = %d, want 2", s.Count("C1"))
	}

	// Empty list -> becomes inactive (the sh_room_leave that empties the room).
	active, inactive = s.Set("C1", nil, "")
	if active || !inactive {
		t.Errorf("emptying: active=%v inactive=%v, want inactive", active, inactive)
	}
	if s.Active("C1") || s.URL("C1") != "" {
		t.Errorf("C1 should be cleared, got active=%v url=%q", s.Active("C1"), s.URL("C1"))
	}
}

func TestHuddleStore_IdempotentAndIgnoresEmptyChannel(t *testing.T) {
	s := NewHuddleStore()
	s.Set("C1", []string{"U1"}, "u")
	if a, _ := s.Set("C1", []string{"U1"}, "u"); a {
		t.Errorf("re-setting the same active list should not re-transition")
	}
	if a, i := s.Set("", []string{"U1"}, "u"); a || i {
		t.Errorf("empty channel must be ignored")
	}
	// Empty user IDs inside the list are filtered out.
	s.Set("C2", []string{"", ""}, "u")
	if s.Active("C2") {
		t.Errorf("a list of only empty user IDs must clear/skip the huddle")
	}
}

func TestHuddleStore_ParticipantsAndActiveChannelsSorted(t *testing.T) {
	s := NewHuddleStore()
	s.Set("C2", []string{"Ub"}, "u2")
	s.Set("C1", []string{"Uc", "Ua"}, "u1")

	if got := s.Participants("C1"); !reflect.DeepEqual(got, []string{"Ua", "Uc"}) {
		t.Errorf("Participants(C1) = %v, want sorted [Ua Uc]", got)
	}
	if got := s.Participants("none"); got != nil {
		t.Errorf("Participants(none) = %v, want nil", got)
	}
	if got := s.ActiveChannels(); !reflect.DeepEqual(got, []string{"C1", "C2"}) {
		t.Errorf("ActiveChannels = %v, want sorted [C1 C2]", got)
	}
}
