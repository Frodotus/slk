package demo

import "testing"

func TestBuild_WellFormed(t *testing.T) {
	s := Build()
	if s.ActiveChannelID == "" || len(s.Messages) == 0 {
		t.Fatal("scene missing active channel / messages")
	}
	if len(s.Channels) == 0 || len(s.Sections) == 0 {
		t.Fatal("scene missing channels / sections")
	}
	hasStarred := false
	for _, sec := range s.Sections {
		if sec.Type == "stars" {
			hasStarred = true
		}
	}
	if !hasStarred {
		t.Error("scene is missing a Starred section")
	}
	if s.Thread.Parent.TS == "" || len(s.Thread.Replies) == 0 {
		t.Error("scene thread missing parent or replies")
	}
	for _, m := range s.Messages {
		if s.UserNames[m.UserID] == "" {
			t.Errorf("message %s has author %q with no display name", m.TS, m.UserID)
		}
	}
}

func TestGenerateImages(t *testing.T) {
	av := GenerateAvatar("SR", people[0].Color, 64)
	if av.Bounds().Dx() != 64 || av.Bounds().Dy() != 64 {
		t.Errorf("avatar size = %v, want 64x64", av.Bounds())
	}
	img := InlineImage()
	if img.Bounds().Dx() < 100 || img.Bounds().Dy() < 100 {
		t.Errorf("inline image decoded too small: %v", img.Bounds())
	}
}
