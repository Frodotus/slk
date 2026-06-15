package slackclient

import "fmt"

// HuddleOpenURL returns a URL that opens channelID in the official Slack
// client so the user can join its huddle (slk itself never carries huddle
// audio). The slack:// deep link launches the desktop app straight into the
// channel; when teamID is unknown it falls back to the https archive URL
// (which opens the web client / desktop app via the browser). Returns "" when
// nothing usable can be built.
//
// Note: Slack exposes no documented deep link that *auto-joins* a huddle, so
// this opens the conversation — the user presses Join there.
func HuddleOpenURL(teamID, domain, channelID string) string {
	if channelID == "" {
		return ""
	}
	if teamID != "" {
		return fmt.Sprintf("slack://channel?team=%s&id=%s", teamID, channelID)
	}
	if domain != "" {
		return fmt.Sprintf("https://%s.slack.com/archives/%s", domain, channelID)
	}
	return ""
}
