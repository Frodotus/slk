package main

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/service"
	slackclient "github.com/gammons/slk/internal/slack"
)

// fakeStarSectionsClient feeds canned sections to SectionStore.Bootstrap.
type fakeStarSectionsClient struct {
	sections []slackclient.SidebarSection
}

func (f *fakeStarSectionsClient) GetChannelSections(ctx context.Context) ([]slackclient.SidebarSection, error) {
	return f.sections, nil
}

// TestStarredSectionLabelFallback verifies the sidebar adapter supplies the
// "Starred" name (plain text, no emoji) when Slack sends the stars section
// with an empty name, which its system sections do.
func TestStarredSectionLabelFallback(t *testing.T) {
	store := service.NewSectionStore()
	// The stars section bootstraps EMPTY from channelSections (as it always
	// does); membership arrives via the durable stars.list set.
	c := &fakeStarSectionsClient{sections: []slackclient.SidebarSection{
		{ID: "T", Type: "stars", Next: "", LastUpdate: 1},
	}}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	store.SetStarred([]string{"C1"})

	metas := sectionsProviderAdapter{store: store}.OrderedSlackSections()
	if len(metas) != 1 {
		t.Fatalf("want 1 section meta, got %d (%+v)", len(metas), metas)
	}
	if metas[0].Name != "Starred" {
		t.Errorf("Name = %q, want Starred", metas[0].Name)
	}
	if metas[0].Emoji != "" {
		t.Errorf("Emoji = %q, want empty (plain-text label)", metas[0].Emoji)
	}
}
