// internal/ui/presence.go
//
// Per-workspace presence + DND state, plus the custom-snooze numeric
// input buffer.
//
// Phase 2f of the SOLID refactor of internal/ui/app.go: extracts the
// workspaceStatus type, the statusByTeam map, dndTickerOn guard, and
// the presenceCustomBuf input buffer out of App. The four mode/event
// orchestrators (handlePresenceMenuMode, handlePresenceCustomSnoozeMode,
// StatusChangeMsg arm, DNDTickMsg arm) stay on App because they touch
// statusbar / mode FSM / tea.Cmd scheduling, but they now route every
// state read and mutation through this controller.
package ui

import (
	"time"

	"github.com/gammons/slk/internal/ui/presencemenu"
)

// workspaceStatus caches the latest StatusChangeMsg per team so the
// status bar can refresh on workspace switch without round-tripping.
type workspaceStatus struct {
	Presence   string
	DNDEnabled bool
	DNDEndTS   time.Time
}

// presenceController owns presence/DND state caching + the custom
// snooze input buffer + the DND-tick guard.
type presenceController struct {
	byTeam      map[string]workspaceStatus
	dndTickerOn bool
	customBuf   string
}

func newPresenceController() *presenceController {
	return &presenceController{
		byTeam: make(map[string]workspaceStatus),
	}
}

// Status returns the cached presence/DND for teamID. Zero values when
// the team has no cached entry; the third bool reports whether an
// entry exists (callers that distinguish "no entry yet" from "entry
// happens to be all zeros" should check it).
func (p *presenceController) Status(teamID string) (presence string, dndEnabled bool, dndEnd time.Time, ok bool) {
	st, exists := p.byTeam[teamID]
	if !exists {
		return "", false, time.Time{}, false
	}
	return st.Presence, st.DNDEnabled, st.DNDEndTS, true
}

// Set records the status for teamID and returns the resulting struct
// so the caller (typically StatusChangeMsg arm) can push to the
// statusbar without a second lookup.
func (p *presenceController) Set(teamID string, presence string, dndEnabled bool, dndEnd time.Time) workspaceStatus {
	st := workspaceStatus{
		Presence:   presence,
		DNDEnabled: dndEnabled,
		DNDEndTS:   dndEnd,
	}
	p.byTeam[teamID] = st
	return st
}

// ClearDNDFor clears the DND fields on teamID's cached entry,
// preserving Presence. Used by the DNDTickMsg arm when DND expires
// locally. Returns the updated struct.
func (p *presenceController) ClearDNDFor(teamID string) workspaceStatus {
	st := p.byTeam[teamID]
	st.DNDEnabled = false
	st.DNDEndTS = time.Time{}
	p.byTeam[teamID] = st
	return st
}

// Apply mutates teamID's cached status based on the chosen presencemenu
// action. SetActive/SetAway touch only Presence; Snooze sets DND fields
// and leaves Presence alone; EndDND clears DND fields and leaves
// Presence alone. Returns the resulting struct.
func (p *presenceController) Apply(teamID string, action presencemenu.Action, snoozeMinutes int) workspaceStatus {
	st := p.byTeam[teamID]
	switch action {
	case presencemenu.ActionSetActive:
		st.Presence = "active"
	case presencemenu.ActionSetAway:
		st.Presence = "away"
	case presencemenu.ActionSnooze:
		st.DNDEnabled = true
		st.DNDEndTS = time.Now().Add(time.Duration(snoozeMinutes) * time.Minute)
	case presencemenu.ActionEndDND:
		st.DNDEnabled = false
		st.DNDEndTS = time.Time{}
	}
	p.byTeam[teamID] = st
	return st
}

// ClaimTicker returns true exactly once until ClearTicker is called,
// guarding against parallel DNDTickMsg tea.Tick chains accumulating
// when multiple StatusChangeMsgs arrive in rapid succession.
func (p *presenceController) ClaimTicker() bool {
	if p.dndTickerOn {
		return false
	}
	p.dndTickerOn = true
	return true
}

// ClearTicker resets the ticker-claim flag. Called from the DNDTickMsg
// arm when DND has expired or the active workspace no longer has DND.
func (p *presenceController) ClearTicker() {
	p.dndTickerOn = false
}

// SnoozeBuf returns the current custom-snooze input buffer.
func (p *presenceController) SnoozeBuf() string { return p.customBuf }

// AppendSnoozeDigit appends a single decimal digit to the snooze
// buffer (capped at 6 chars so absurd minute counts can't overflow).
// No-op for non-digit input.
func (p *presenceController) AppendSnoozeDigit(r string) {
	if len(r) != 1 || r[0] < '0' || r[0] > '9' {
		return
	}
	if len(p.customBuf) >= 6 {
		return
	}
	p.customBuf += r
}

// BackspaceSnooze drops the last digit from the snooze buffer.
// No-op when empty.
func (p *presenceController) BackspaceSnooze() {
	if len(p.customBuf) == 0 {
		return
	}
	p.customBuf = p.customBuf[:len(p.customBuf)-1]
}

// ClearSnoozeBuf empties the snooze buffer.
func (p *presenceController) ClearSnoozeBuf() { p.customBuf = "" }
