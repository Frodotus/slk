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
// "Starred" name + "star" emoji when Slack sends the stars section with an
// empty name/emoji (its system sections often do).
func TestStarredSectionLabelFallback(t *testing.T) {
	store := service.NewSectionStore()
	c := &fakeStarSectionsClient{sections: []slackclient.SidebarSection{
		{ID: "T", Type: "stars", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	metas := sectionsProviderAdapter{store: store}.OrderedSlackSections()
	if len(metas) != 1 {
		t.Fatalf("want 1 section meta, got %d (%+v)", len(metas), metas)
	}
	if metas[0].Name != "Starred" {
		t.Errorf("Name = %q, want Starred", metas[0].Name)
	}
	if metas[0].Emoji != "star" {
		t.Errorf("Emoji = %q, want star", metas[0].Emoji)
	}
}
