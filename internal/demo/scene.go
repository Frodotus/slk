// Package demo builds a curated, deterministic "hero" scene used by
// `slk --demo` to render the real TUI with fake data for screenshots —
// no Slack connection, DB, or network. The scene is pure data plus
// in-memory image generation (see images.go); the cmd/slk runDemo path
// seeds it into a normal *ui.App and runs the TUI.
package demo

import (
	"image/color"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/gammons/slk/internal/ui/workspace"
)

// Person is one fake participant; Color drives their generated avatar.
type Person struct {
	ID       string
	Name     string
	Initials string
	Color    color.RGBA
}

// Thread is the open thread-panel content (parent + replies).
type Thread struct {
	ChannelID string
	ThreadTS  string
	Parent    messages.MessageItem
	Replies   []messages.MessageItem
}

// Scene is the whole curated hero scene.
type Scene struct {
	CurrentUserID     string
	People            []Person
	UserNames         map[string]string
	Workspaces        []workspace.WorkspaceItem
	Channels          []sidebar.ChannelItem
	Sections          []sidebar.SectionMeta
	ActiveChannelID   string
	ActiveChannelName string
	Messages          []messages.MessageItem
	Thread            Thread
}

// Section IDs for the fake sidebar provider.
const (
	secStarred = "S_STARS"
	secApps    = "S_APPS"
	secChans   = "S_CHANNELS"
	secDMs     = "S_DMS"
)

// people in the scene. "me" is the viewer (Sam Rivera).
var people = []Person{
	{ID: "me", Name: "Sam Rivera", Initials: "SR", Color: color.RGBA{0x4A, 0x9E, 0xFF, 0xFF}},
	{ID: "alice", Name: "Alice Chen", Initials: "AC", Color: color.RGBA{0x50, 0xC8, 0x78, 0xFF}},
	{ID: "bob", Name: "Bob Mendez", Initials: "BM", Color: color.RGBA{0xE0, 0x6C, 0x75, 0xFF}},
	{ID: "carol", Name: "Carol Singh", Initials: "CS", Color: color.RGBA{0xC6, 0x78, 0xDD, 0xFF}},
	{ID: "deploybot", Name: "DeployBot", Initials: "DB", Color: color.RGBA{0xE5, 0xA0, 0x4A, 0xFF}},
}

// Build returns the curated scene. Deterministic — identical every run.
func Build() Scene {
	const date = "Today"

	names := map[string]string{}
	for _, p := range people {
		names[p.ID] = p.Name
	}

	react := func(emoji string, n int, mine bool) messages.ReactionItem {
		return messages.ReactionItem{Emoji: emoji, Count: n, HasReacted: mine}
	}
	msg := func(ts, user, text, at string, rs ...messages.ReactionItem) messages.MessageItem {
		return messages.MessageItem{
			TS: ts, UserID: user, UserName: names[user],
			Text: text, Timestamp: at, DateStr: date, Reactions: rs,
		}
	}

	// The active channel (#engineering) conversation. The open thread is
	// on a TEXT message (1.0); the inline photo is on a separate,
	// non-threaded message (3.0). They're kept apart because the thread
	// panel discards a parent message's image upload (slk v1 limitation),
	// so a photo+thread on one message would render a broken parent image.
	msgs := []messages.MessageItem{
		msg("1.0", "alice", "Morning! Ship-readiness check for the 0.9 release \U0001F680 — anything still open?", "9:02 AM",
			react("rocket", 3, true), react("eyes", 2, false)),
		msg("2.0", "deploybot", "Deploy `v0.9.0-rc2` to staging succeeded ✅ (build 4821)", "9:05 AM"),
		// 3.0 carries the inline image (no thread).
		withImage(msg("3.0", "carol", "And the view from the team offsite yesterday \U0001F4F8 couldn't resist sharing", "9:11 AM",
			react("heart_eyes", 5, true), react("fire", 2, false))),
		msg("4.0", "me", "Incredible \U0001F60D — alright, cutting the release. Great work everyone \U0001F680", "9:15 AM",
			react("rocket", 4, true)),
	}
	// Mark the threaded parent (the text message 1.0).
	msgs[0].ThreadTS = "1.0"
	msgs[0].ReplyCount = 2

	thread := Thread{
		ChannelID: "C_ENG", ThreadTS: "1.0",
		Parent: msgs[0],
		Replies: []messages.MessageItem{
			msg("1.1", "bob", "Backend's green — all migrations applied, p95 down 18% \U0001F44D", "9:04 AM", react("tada", 3, false)),
			msg("1.2", "carol", "Docs + changelog merged ✅ I'm good to ship", "9:06 AM", react("raised_hands", 2, true)),
		},
	}

	channels := []sidebar.ChannelItem{
		{ID: "C_ENG", Name: "engineering", Type: "channel", Section: secStarred, IsStarred: true},
		{ID: "C_GEN", Name: "general", Type: "channel", Section: secChans},
		{ID: "C_DESIGN", Name: "design", Type: "channel", Section: secChans},
		{ID: "C_RANDOM", Name: "random", Type: "channel", Section: secChans},
		{ID: "D_ALICE", Name: "Alice Chen", Type: "dm", Section: secDMs, DMUserID: "alice", Presence: "active"},
		{ID: "D_BOB", Name: "Bob Mendez", Type: "dm", Section: secDMs, DMUserID: "bob", Presence: "away"},
	}

	sections := []sidebar.SectionMeta{
		{ID: secStarred, Name: "Starred", Type: "stars"},
		{ID: secChans, Name: "Channels", Type: "channels"},
		{ID: secDMs, Name: "Direct Messages", Type: "direct_messages"},
	}

	return Scene{
		CurrentUserID: "me",
		People:        people,
		UserNames:     names,
		Workspaces: []workspace.WorkspaceItem{
			{ID: "T_ACME", Name: "Acme", Initials: "AC"},
			{ID: "T_SIDE", Name: "Side Project", Initials: "SP", HasUnread: true},
		},
		Channels:          channels,
		Sections:          sections,
		ActiveChannelID:   "C_ENG",
		ActiveChannelName: "engineering",
		Messages:          msgs,
		Thread:            thread,
	}
}

// withImage attaches the demo inline image to a message. The FileID is
// the key the demo image-source resolves (see cmd/slk runDemo).
func withImage(m messages.MessageItem) messages.MessageItem {
	m.Attachments = []messages.Attachment{{
		Kind:   "image",
		Name:   "offsite.jpg",
		FileID: DemoImageFileID,
		Mime:   "image/jpeg",
		Thumbs: []messages.ThumbSpec{{URL: "demo://offsite", W: 640, H: 400}},
	}}
	return m
}

// DemoImageFileID is the synthetic file ID for the one inline image.
const DemoImageFileID = "FDEMO0001"

// sectionsProvider adapts the scene's sections to sidebar.SectionsProvider.
type sectionsProvider struct{ metas []sidebar.SectionMeta }

func (p sectionsProvider) Ready() bool                                 { return true }
func (p sectionsProvider) OrderedSlackSections() []sidebar.SectionMeta { return p.metas }

// SectionsProvider returns a sidebar.SectionsProvider for the scene's
// sections, so the demo sidebar renders Slack-native-style sections
// (including Starred) without a real SectionStore.
func (s Scene) SectionsProvider() sidebar.SectionsProvider {
	return sectionsProvider{metas: s.Sections}
}
