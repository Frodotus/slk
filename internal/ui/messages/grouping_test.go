package messages

import "testing"

// ts builds a Slack ts string for the given unix seconds.
func tsAt(sec int64) string {
	return itoa(sec) + ".000000"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestContinuesGroup(t *testing.T) {
	const base = 1_700_000_000 // some weekday noon-ish epoch
	mk := func(user string, sec int64, subtype string) MessageItem {
		return MessageItem{UserID: user, TS: tsAt(sec), Subtype: subtype}
	}

	cases := []struct {
		name   string
		prev   MessageItem
		cur    MessageItem
		window int
		want   bool
	}{
		{"same author within window", mk("U1", base, ""), mk("U1", base+60, ""), 5, true},
		{"same author at exact window edge", mk("U1", base, ""), mk("U1", base+300, ""), 5, true},
		{"same author just past window", mk("U1", base, ""), mk("U1", base+301, ""), 5, false},
		{"different author", mk("U1", base, ""), mk("U2", base+30, ""), 5, false},
		{"empty author never groups", mk("", base, ""), mk("", base+30, ""), 5, false},
		{"window zero disables", mk("U1", base, ""), mk("U1", base+10, ""), 0, false},
		{"prev subtype breaks", mk("U1", base, "channel_join"), mk("U1", base+30, ""), 5, false},
		{"cur subtype breaks", mk("U1", base, ""), mk("U1", base+30, "thread_broadcast"), 5, false},
		{"negative gap (out of order) does not group", mk("U1", base+30, ""), mk("U1", base, ""), 5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContinuesGroup(tc.prev, tc.cur, tc.window); got != tc.want {
				t.Errorf("ContinuesGroup = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestContinuesGroup_DayBoundary verifies that two same-author messages on
// different calendar days do NOT group (a date separator renders between
// them), even within a large window. DateFromTS resolves in local time, so
// rather than hard-code a midnight-crossing pair, this first sanity-checks
// the same-day path (when the two stamps happen to land on the same local
// day) and then forces a guaranteed different-day case by adding a full
// 86,400 seconds.
func TestContinuesGroup_DayBoundary(t *testing.T) {
	prevTS := tsAt(1_700_000_000)
	curTS := tsAt(1_700_000_180) // +3 minutes; same local day in any zone
	prev := MessageItem{UserID: "U1", TS: prevTS}
	cur := MessageItem{UserID: "U1", TS: curTS}
	if DateFromTS(prevTS) == DateFromTS(curTS) {
		if !ContinuesGroup(prev, cur, 5) {
			t.Fatalf("same-day same-author within window should group")
		}
	}
	// Force a different day: one full day later, same author, but with a
	// huge window so only the day check can break it.
	curNextDay := MessageItem{UserID: "U1", TS: tsAt(1_700_000_000 + 86_400)}
	if ContinuesGroup(prev, curNextDay, 100_000) {
		t.Errorf("messages on different calendar days must not group even within a large window")
	}
}
