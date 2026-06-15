package slackclient

import "testing"

func TestHuddleOpenURL(t *testing.T) {
	cases := []struct {
		name                      string
		teamID, domain, channelID string
		want                      string
	}{
		{"deep link preferred", "T123", "acme", "C9", "slack://channel?team=T123&id=C9"},
		{"falls back to web when no team", "", "acme", "C9", "https://acme.slack.com/archives/C9"},
		{"empty channel -> empty", "T123", "acme", "", ""},
		{"no team or domain -> empty", "", "", "C9", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HuddleOpenURL(tc.teamID, tc.domain, tc.channelID); got != tc.want {
				t.Errorf("HuddleOpenURL(%q,%q,%q) = %q, want %q", tc.teamID, tc.domain, tc.channelID, got, tc.want)
			}
		})
	}
}
