package service

import (
	"context"
	"testing"

	slk "github.com/gammons/slk/internal/slack"
)

// fakeSectionsClient implements the subset of slk.Client SectionStore needs.
type fakeSectionsClient struct {
	sections []slk.SidebarSection
	getErr   error
}

func (f *fakeSectionsClient) GetChannelSections(ctx context.Context) ([]slk.SidebarSection, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	// Return a fresh deep copy on every call, like the real client (which
	// decodes a fresh HTTP response). This prevents the store's in-place
	// section mutations from leaking back into the fake and corrupting a
	// later (re-)Bootstrap — which would mask the re-bootstrap behavior.
	out := make([]slk.SidebarSection, len(f.sections))
	for i, sec := range f.sections {
		out[i] = sec
		if sec.ChannelIDs != nil {
			out[i].ChannelIDs = append([]string(nil), sec.ChannelIDs...)
		}
	}
	return out, nil
}

func TestSectionStore_Bootstrap_Empty(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !store.Ready() {
		t.Errorf("Ready=false after empty bootstrap")
	}
	if got := store.OrderedSections(); len(got) != 0 {
		t.Errorf("OrderedSections len = %d, want 0", len(got))
	}
}

func TestSectionStore_Bootstrap_BuildsLinkedListOrder(t *testing.T) {
	// Build: head=A → B → C → tail
	sections := []slk.SidebarSection{
		{ID: "B", Name: "Books", Type: "standard", Next: "C", LastUpdate: 100, ChannelIDs: []string{"C2"}, ChannelsCount: 1},
		{ID: "A", Name: "Alerts", Type: "standard", Next: "B", LastUpdate: 100, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
		{ID: "C", Name: "Channels", Type: "channels", Next: "", LastUpdate: 100},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	got := store.OrderedSections()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (got: %+v)", len(got), got)
	}
	wantOrder := []string{"A", "B", "C"}
	for i, w := range wantOrder {
		if got[i].ID != w {
			t.Errorf("[%d] ID = %q, want %q", i, got[i].ID, w)
		}
	}
}

func TestSectionStore_Bootstrap_TruncatedSection_LogsAndContinues(t *testing.T) {
	// Section "A" reports count=5 but only first 3 channels were returned
	// in channel_ids_page. v1 trusts the first-page data and lets the
	// remaining 2 stay in the catch-all "Channels" bucket until WS
	// deltas migrate them. Bootstrap must NOT fail in this case.
	sections := []slk.SidebarSection{
		{ID: "A", Type: "standard", Next: "", LastUpdate: 100,
			ChannelIDs:     []string{"C1", "C2", "C3"},
			ChannelsCount:  5,
			ChannelsCursor: "C3"},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !store.Ready() {
		t.Errorf("Ready=false after truncated bootstrap")
	}
	// First-page channels are mapped.
	if id, ok := store.SectionForChannel("C1"); !ok || id != "A" {
		t.Errorf("SectionForChannel(C1) = (%q, %v), want (A, true)", id, ok)
	}
	// Channels beyond the first page are NOT mapped.
	if _, ok := store.SectionForChannel("C5"); ok {
		t.Errorf("SectionForChannel(C5) ok=true, want false (channel beyond first page must stay unmapped in v1)")
	}
}

func TestSectionStore_OrderedSections_FiltersSystemTypes(t *testing.T) {
	sections := []slk.SidebarSection{
		{ID: "S", Type: "salesforce_records", Next: "G", LastUpdate: 1},
		{ID: "G", Type: "agents", Next: "T", LastUpdate: 1},
		{ID: "T", Type: "stars", Next: "K", LastUpdate: 1},
		{ID: "K", Type: "slack_connect", Next: "U", LastUpdate: 1},
		{ID: "U", Type: "standard", Name: "Mine", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	_ = store.Bootstrap(context.Background(), c)
	got := store.OrderedSections()
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (only standard)", len(got))
	}
	if got[0].ID != "U" {
		t.Errorf("got %q, want U", got[0].ID)
	}
}

// TestSectionStore_Stars_RendersClaimsAndPinsTop is the headline of the
// starred-channels feature: a non-empty stars ("Starred") section renders,
// is pinned to the top regardless of its place in Slack's linked list, and
// claims its channels (so they don't also appear in the catch-all).
func TestSectionStore_Stars_RendersClaimsAndPinsTop(t *testing.T) {
	// Chain: head=U(standard, C1) -> T(stars). Stars is LAST in the chain
	// but must be hoisted to the top. The stars section bootstraps EMPTY
	// (as it always does from channelSections); C2's membership arrives via
	// the durable stars.list set, exactly like production.
	sections := []slk.SidebarSection{
		{ID: "U", Name: "Mine", Type: "standard", Next: "T", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
		{ID: "T", Name: "", Type: "stars", Next: "", LastUpdate: 1},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	store.SetStarred([]string{"C2"})

	got := store.OrderedSections()
	if len(got) != 2 {
		t.Fatalf("OrderedSections len = %d, want 2 (%+v)", len(got), got)
	}
	if got[0].Type != "stars" {
		t.Errorf("stars not pinned to top: got[0] = {ID:%q Type:%q}, want the stars section", got[0].ID, got[0].Type)
	}
	// C2 is claimed by the stars section (no duplication in the catch-all).
	if id, ok := store.SectionForChannel("C2"); !ok || id != "T" {
		t.Errorf("SectionForChannel(C2) = (%q, %v), want (T, true) — claimed by stars", id, ok)
	}
	// C1 stays in its standard section.
	if id, ok := store.SectionForChannel("C1"); !ok || id != "U" {
		t.Errorf("SectionForChannel(C1) = (%q, %v), want (U, true)", id, ok)
	}
}

// TestSectionStore_Stars_PopulatedViaApplyChannels mirrors how stars.list
// membership reaches the store in production: Slack's stars section
// bootstraps EMPTY (its channel_ids_page is always empty), so slk fetches
// the starred channels from the legacy stars.list API and feeds them in
// via ApplyChannelsAdded targeting SectionIDByType("stars"). After that
// the section renders, claims its channels, and pins to the top.
func TestSectionStore_Stars_PopulatedViaApplyChannels(t *testing.T) {
	sections := []slk.SidebarSection{
		{ID: "L_STARS", Type: "stars", Next: "L_STD", LastUpdate: 1}, // empty at bootstrap
		{ID: "L_STD", Type: "standard", Name: "Mine", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), &fakeSectionsClient{sections: sections}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Empty stars is hidden until membership arrives.
	if got := store.OrderedSections(); len(got) != 1 {
		t.Fatalf("before stars.list: want only the standard section, got %+v", got)
	}

	secID, ok := store.SectionIDByType("stars")
	if !ok || secID != "L_STARS" {
		t.Fatalf("SectionIDByType(stars) = (%q, %v), want (L_STARS, true)", secID, ok)
	}
	store.ApplyChannelsAdded(secID, []string{"C_STARRED"})

	got := store.OrderedSections()
	if len(got) != 2 || got[0].Type != "stars" {
		t.Fatalf("after stars.list: want stars pinned first + standard, got %+v", got)
	}
	if id, ok := store.SectionForChannel("C_STARRED"); !ok || id != "L_STARS" {
		t.Errorf("SectionForChannel(C_STARRED) = (%q, %v), want (L_STARS, true) — claimed by stars", id, ok)
	}
}

// TestSectionStore_Stars_SurviveRebootstrap is the regression for the
// "Starred channels disappear after a while" bug. Starred membership comes
// from stars.list (out-of-band); the channelSections "stars" section always
// bootstraps EMPTY. A reconnect triggers MaybeRebootstrap -> Bootstrap,
// which rebuilds the section maps from channelSections. Before the fix the
// stars membership lived only in those maps, so the re-bootstrap wiped it
// and Starred vanished. The durable starred set must be re-applied on every
// Bootstrap.
func TestSectionStore_Stars_SurviveRebootstrap(t *testing.T) {
	sections := []slk.SidebarSection{
		{ID: "L_STARS", Type: "stars", Next: "L_STD", LastUpdate: 1}, // empty from channelSections
		{ID: "L_STD", Type: "standard", Name: "Mine", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	// stars.list membership arrives out-of-band.
	store.SetStarred([]string{"C_STARRED"})
	if got := store.OrderedSections(); len(got) != 2 || got[0].Type != "stars" {
		t.Fatalf("after stars.list: want stars pinned first + standard, got %+v", got)
	}

	// Simulate the reconnect re-bootstrap (MaybeRebootstrap -> Bootstrap):
	// channelSections STILL reports the stars section empty.
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("re-Bootstrap: %v", err)
	}

	got := store.OrderedSections()
	if len(got) != 2 || got[0].Type != "stars" {
		t.Fatalf("after re-bootstrap: Starred disappeared; want stars pinned first + standard, got %+v", got)
	}
	if id, ok := store.SectionForChannel("C_STARRED"); !ok || id != "L_STARS" {
		t.Errorf("SectionForChannel(C_STARRED) = (%q, %v), want (L_STARS, true) — starred membership must survive re-bootstrap", id, ok)
	}
}

// TestSectionStore_Stars_RemoveStar verifies an un-starred channel drops out
// of the Starred section (and the section hides when it empties).
func TestSectionStore_Stars_RemoveStar(t *testing.T) {
	sections := []slk.SidebarSection{
		{ID: "L_STARS", Type: "stars", Next: "L_STD", LastUpdate: 1},
		{ID: "L_STD", Type: "standard", Name: "Mine", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), &fakeSectionsClient{sections: sections}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	store.SetStarred([]string{"C_STARRED"})
	store.RemoveStar("C_STARRED")
	if got := store.OrderedSections(); len(got) != 1 || got[0].ID != "L_STD" {
		t.Fatalf("after RemoveStar the empty Starred section should hide; want only L_STD, got %+v", got)
	}
	if _, ok := store.SectionForChannel("C_STARRED"); ok {
		t.Errorf("SectionForChannel(C_STARRED) ok=true after un-star; want false")
	}
}

// TestSectionStore_Stars_EmptyHidden verifies an empty Starred section is
// not rendered (hide-when-empty).
func TestSectionStore_Stars_EmptyHidden(t *testing.T) {
	sections := []slk.SidebarSection{
		{ID: "T", Type: "stars", Next: "U", LastUpdate: 1}, // no channels
		{ID: "U", Name: "Mine", Type: "standard", Next: "", LastUpdate: 1, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}
	c := &fakeSectionsClient{sections: sections}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	got := store.OrderedSections()
	if len(got) != 1 || got[0].ID != "U" {
		t.Fatalf("empty stars should be hidden; want only U, got %+v", got)
	}
}

func TestSectionStore_BootstrapFailure_NotReady(t *testing.T) {
	c := &fakeSectionsClient{getErr: context.DeadlineExceeded}
	store := NewSectionStore()
	if err := store.Bootstrap(context.Background(), c); err == nil {
		t.Errorf("expected error")
	}
	if store.Ready() {
		t.Errorf("Ready=true after failure; should remain false")
	}
}

func TestSectionStore_NotReady_SectionForChannelFalse(t *testing.T) {
	store := NewSectionStore()
	if _, ok := store.SectionForChannel("C1"); ok {
		t.Errorf("ok=true on never-bootstrapped store")
	}
}

func TestApplyUpsert_NewSection(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Name: "A", LastUpdate: 100},
	}}
	_ = store.Bootstrap(context.Background(), c)

	store.ApplyUpsert(slk.ChannelSectionUpserted{
		ID: "B", Name: "Brand New", Type: "standard", Next: "", LastUpdate: 200,
	})
	got := store.OrderedSections()
	// Both A and B exist now; the head is whichever isn't pointed at.
	// A.Next="" (set in fixture), B.Next="" too — multiple heads.
	// Our heuristic picks the highest LastUpdate.
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (multi-head heuristic picks newest)", len(got))
	}
	if got[0].ID != "B" {
		t.Errorf("head = %q, want B (newer LastUpdate wins)", got[0].ID)
	}
}

func TestApplyUpsert_RenameExistingByID(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Name: "Old", Next: "", LastUpdate: 100},
	}}
	_ = store.Bootstrap(context.Background(), c)
	store.ApplyUpsert(slk.ChannelSectionUpserted{
		ID: "A", Name: "New", Type: "standard", Next: "", LastUpdate: 200,
	})
	got := store.OrderedSections()
	if len(got) != 1 || got[0].Name != "New" {
		t.Errorf("got %+v, want one section named New", got)
	}
}

func TestApplyUpsert_StaleEventIgnored(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Name: "Latest", Next: "", LastUpdate: 200},
	}}
	_ = store.Bootstrap(context.Background(), c)
	// Older event arrives.
	store.ApplyUpsert(slk.ChannelSectionUpserted{
		ID: "A", Name: "Stale", Type: "standard", LastUpdate: 100,
	})
	got := store.OrderedSections()
	if got[0].Name != "Latest" {
		t.Errorf("name = %q, want Latest (stale event must be dropped)", got[0].Name)
	}
}

func TestApplyDelete_RemovesSectionAndChannels(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Name: "A", Next: "", LastUpdate: 100, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}}
	_ = store.Bootstrap(context.Background(), c)
	store.ApplyDelete("A")
	if _, ok := store.SectionForChannel("C1"); ok {
		t.Errorf("channel still mapped after section delete")
	}
	if got := store.OrderedSections(); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestApplyChannelsAdded_UpdatesIndex(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Next: "", LastUpdate: 100},
	}}
	_ = store.Bootstrap(context.Background(), c)
	store.ApplyChannelsAdded("A", []string{"C1", "C2"})
	if id, ok := store.SectionForChannel("C1"); !ok || id != "A" {
		t.Errorf("C1 → (%q,%v), want (A,true)", id, ok)
	}
	if id, ok := store.SectionForChannel("C2"); !ok || id != "A" {
		t.Errorf("C2 → (%q,%v), want (A,true)", id, ok)
	}
}

func TestApplyChannelsAdded_OverwritesPreviousSection(t *testing.T) {
	// Channel moves from A to B via remove-then-add (Slack's pattern):
	// upsert into B should replace its membership in A.
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Next: "B", LastUpdate: 100, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
		{ID: "B", Type: "standard", Next: "", LastUpdate: 100},
	}}
	_ = store.Bootstrap(context.Background(), c)
	store.ApplyChannelsAdded("B", []string{"C1"})
	if id, _ := store.SectionForChannel("C1"); id != "B" {
		t.Errorf("C1 in %q, want B (add must overwrite)", id)
	}
}

func TestApplyChannelsRemoved_DropsIndex(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Next: "", LastUpdate: 100, ChannelIDs: []string{"C1"}, ChannelsCount: 1},
	}}
	_ = store.Bootstrap(context.Background(), c)
	store.ApplyChannelsRemoved("A", []string{"C1"})
	if _, ok := store.SectionForChannel("C1"); ok {
		t.Errorf("C1 still mapped after removal")
	}
}

func TestMaybeRebootstrap_DebouncedWithin30s(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "A", Type: "standard", Next: "", LastUpdate: 100},
	}}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	// First call: too soon, skipped.
	calledAgain := false
	c2 := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "B", Type: "standard", Next: "", LastUpdate: 200},
	}}
	wrap := &countingClient{inner: c2, onCall: func() { calledAgain = true }}
	store.MaybeRebootstrap(context.Background(), wrap)
	if calledAgain {
		t.Errorf("MaybeRebootstrap should be debounced within 30s")
	}
}

type countingClient struct {
	inner  SectionsClient
	onCall func()
}

func (cc *countingClient) GetChannelSections(ctx context.Context) ([]slk.SidebarSection, error) {
	cc.onCall()
	return cc.inner.GetChannelSections(ctx)
}

// TestSectionForChannel_HidesNonRenderableSections regresses a sidebar
// crash where a channel mapped to a non-renderable section (slack_connect,
// salesforce_records, agents) ended up with a Section ID the sidebar's
// modelOrderedSections never emitted, causing a nil-pointer dereference in
// buildCache. SectionForChannel returns ok=false for such channels so the
// resolver falls through to type-default bucketing. (The stars section is
// now renderable — see TestSectionStore_Stars_RendersClaimsAndPinsTop.)
func TestSectionForChannel_HidesNonRenderableSections(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		// A channel in a non-renderable section: real, indexed, but the
		// section type is hidden by the v1 renderability filter.
		{ID: "L_CONN", Type: "slack_connect", Next: "L_STD", LastUpdate: 100,
			ChannelIDs: []string{"C_HIDDEN"}, ChannelsCount: 1},
		// A regular standard section, fully renderable.
		{ID: "L_STD", Type: "standard", Name: "Mine", Next: "", LastUpdate: 100,
			ChannelIDs: []string{"C_STD"}, ChannelsCount: 1},
	}}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Channel in the renderable section — returns the ID.
	if id, ok := store.SectionForChannel("C_STD"); !ok || id != "L_STD" {
		t.Errorf("C_STD → (%q, %v), want (L_STD, true)", id, ok)
	}
	// Channel in the non-renderable (slack_connect) section — returns
	// ("", false) even though the channelToSection index has it. This
	// prevents the sidebar from receiving a Section ID it can't bucket.
	if id, ok := store.SectionForChannel("C_HIDDEN"); ok {
		t.Errorf("C_HIDDEN → (%q, %v), want ('', false) for non-renderable section", id, ok)
	}
}

// TestSectionForChannel_HidesRedactedSections is the parallel guard for
// is_redacted=true sections: even if the type would otherwise render,
// a redacted section is hidden from the sidebar, and channels in it
// must not leak their Section ID upward.
func TestSectionForChannel_HidesRedactedSections(t *testing.T) {
	store := NewSectionStore()
	c := &fakeSectionsClient{sections: []slk.SidebarSection{
		{ID: "L_R", Type: "standard", Name: "Hidden", Next: "", LastUpdate: 100,
			IsRedacted: true,
			ChannelIDs: []string{"C_REDACTED"}, ChannelsCount: 1},
	}}
	if err := store.Bootstrap(context.Background(), c); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if id, ok := store.SectionForChannel("C_REDACTED"); ok {
		t.Errorf("C_REDACTED → (%q, %v), want ('', false) for redacted section", id, ok)
	}
}
