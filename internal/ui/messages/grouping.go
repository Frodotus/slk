package messages

import "strconv"

// ContinuesGroup reports whether cur should render as a same-author
// continuation collapsed under prev, given the grouping window in minutes
// (windowMinutes <= 0 disables grouping). It is a pure function of the two
// messages; callers apply any pane-specific breaks (e.g. an unread "── new ──"
// landmark sitting between them) themselves.
//
// A continuation requires all of: a positive window; both messages plain
// (empty Subtype — joins, topic changes, and thread_broadcast carry their
// own header/label and must keep it); a non-empty, identical author; the
// same calendar day (so a date separator never sits between grouped
// messages); and a non-negative gap within the window.
func ContinuesGroup(prev, cur MessageItem, windowMinutes int) bool {
	if windowMinutes <= 0 {
		return false
	}
	if prev.Subtype != "" || cur.Subtype != "" {
		return false
	}
	if cur.UserID == "" || cur.UserID != prev.UserID {
		return false
	}
	if DateFromTS(prev.TS) != DateFromTS(cur.TS) {
		return false
	}
	prevSec, ok1 := tsEpochSeconds(prev.TS)
	curSec, ok2 := tsEpochSeconds(cur.TS)
	if !ok1 || !ok2 {
		return false
	}
	gap := curSec - prevSec
	return gap >= 0 && gap <= float64(windowMinutes)*60
}

// tsEpochSeconds parses a Slack ts ("1700000000.000100") into fractional
// unix seconds.
func tsEpochSeconds(ts string) (float64, bool) {
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
