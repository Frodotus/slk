// internal/ui/drag.go
//
// Mouse-drag selection FSM state.
//
// Phase 2h of the SOLID refactor of internal/ui/app.go: extracts the
// dragState struct + its primitive transitions out of App. The four
// Update arms that drive the FSM (MouseClickMsg, MouseMotionMsg,
// autoScrollTickMsg, MouseReleaseMsg) stay on App because they touch
// sub-models (messagepane, threadPanel) and dispatch tea.Cmds — but
// they now go through this controller for every state read and
// mutation.
//
// State machine:
//
//	IDLE                  panel == PanelWorkspace (zero value)
//	  │ MouseClickMsg on PanelMessages / PanelThread (Begin)
//	  ▼
//	PRESS_NOT_MOVED       panel set, moved == false
//	  │ MouseMotionMsg    (Extend → moved = true)
//	  ▼
//	DRAGGING              moved == true
//	  │ optionally: cursor at pane edge (ClaimAutoScroll → autoscroll
//	  │             tick chain starts), self-terminates on ClearAutoScroll
//	  │ MouseReleaseMsg   (Finish → IDLE; caller branches on moved
//	  ▼                    flag to either commit selection or treat as
//	IDLE                   a plain click)
package ui

// dragState captures an in-progress mouse drag. The originating panel
// (PanelMessages or PanelThread; PanelWorkspace == idle) clamps where
// the drag's selection extends to — leaving the pane pins the extend
// at the last known position inside it.
//
// clickedMessage records whether the press landed on a real message
// row (vs chrome or empty space). MouseReleaseMsg consults it on
// plain-click finalization: if a plain click landed on a message, that
// message's thread is opened (mirrors the Enter keypress).
//
// autoScrollActive is the once-claim guard for the edge-autoscroll
// tea.Tick chain; ClaimAutoScroll returns true exactly once until
// ClearAutoScroll resets it.
type dragState struct {
	panel            Panel
	pressX, pressY   int
	lastX, lastY     int
	moved            bool
	autoScrollActive bool
	clickedMessage   bool
}

func newDragState() *dragState { return &dragState{} }

// IsActive reports whether a drag is in progress on a real pane.
func (d *dragState) IsActive() bool {
	return d.panel == PanelMessages || d.panel == PanelThread
}

// Panel returns the originating panel. Meaningless when !IsActive.
func (d *dragState) Panel() Panel { return d.panel }

// LastPos returns the most recent cursor position recorded by Extend
// (or the initial press position if no motion has occurred yet).
func (d *dragState) LastPos() (x, y int) { return d.lastX, d.lastY }

// Begin records a fresh press on panel at (px, py). Any prior drag
// state is overwritten.
func (d *dragState) Begin(panel Panel, px, py int) {
	*d = dragState{
		panel:  panel,
		pressX: px, pressY: py,
		lastX: px, lastY: py,
	}
}

// SetClickedMessage records whether the press landed on a real message
// row. Called immediately after Begin from the MouseClickMsg arm on
// PanelMessages.
func (d *dragState) SetClickedMessage(b bool) { d.clickedMessage = b }

// Extend updates the cursor position to (px, py) and marks the drag
// as moved. If panel doesn't match the originating drag panel, the
// position is clamped to the previous lastX/lastY (pinning extension
// at the last known coordinates inside the originating pane).
// Returns the effective (lastX, lastY) after clamping.
func (d *dragState) Extend(panel Panel, px, py int) (x, y int) {
	if panel != d.panel {
		px, py = d.lastX, d.lastY
	}
	d.lastX, d.lastY = px, py
	d.moved = true
	return px, py
}

// ClaimAutoScroll flips on the autoscroll-in-flight gate. Returns
// true on first call (caller schedules an autoScrollTickMsg);
// false if a chain is already in flight (caller does nothing).
func (d *dragState) ClaimAutoScroll() bool {
	if d.autoScrollActive {
		return false
	}
	d.autoScrollActive = true
	return true
}

// ClearAutoScroll resets the autoscroll-in-flight gate. Called from
// the autoScrollTickMsg arm when the cursor leaves the pane edge or
// the drag ends.
func (d *dragState) ClearAutoScroll() { d.autoScrollActive = false }

// Finish returns the captured release context and resets the state
// to idle. Called from MouseReleaseMsg.
func (d *dragState) Finish() (moved bool, panel Panel, clickedMessage bool) {
	moved = d.moved
	panel = d.panel
	clickedMessage = d.clickedMessage
	*d = dragState{}
	return
}
