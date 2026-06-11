package searchresults

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// hlSGR derives the open/close SGR sequences of the search-highlight
// style via the same sentinel-split pattern the renderer uses.
func hlSGR(t *testing.T) (string, string) {
	t.Helper()
	parts := strings.SplitN(styles.SearchHighlightStyle().Render("\x00"), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" {
		t.Fatal("could not derive highlight SGR (theme not applied?)")
	}
	return parts[0], parts[1]
}

// applyTheme installs a real theme so SearchHighlightStyle renders
// actual SGR sequences (the styles package colors are nil until Apply).
func applyTheme(t *testing.T) {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
}

func TestSetHighlightTermsClonesInput(t *testing.T) {
	m := New()
	terms := []string{"deploy"}
	m.SetHighlightTerms(terms)
	terms[0] = "mutated"
	if m.highlightTerms[0] != "deploy" {
		t.Fatalf("SetHighlightTerms aliased caller slice: %v", m.highlightTerms)
	}
}

// TestSnippetHighlightsTerms verifies matched terms in the snippet
// lines are wrapped in the search-highlight SGR, while the metadata
// line (channel/author/timestamp) is never highlighted even when a
// term occurs there.
func TestSnippetHighlightsTerms(t *testing.T) {
	applyTheme(t)
	hlStart, _ := hlSGR(t)

	m := New()
	m.Open()
	submitQuery(&m, "deploy")
	m.SetHighlightTerms([]string{"deploy"})
	m.SetResults([]Item{
		// Channel and author both contain the term: metadata must not
		// light up.
		{ChannelID: "C1", ChannelName: "deploys", UserName: "deployer", TS: "1.0",
			Text: "the deploy went fine"},
	}, 1)

	lines := strings.Split(m.View(80, 30), "\n")
	meta := lines[listTopOffset]
	snippet := lines[listTopOffset+1]
	if !strings.Contains(snippet, hlStart+"deploy") {
		t.Errorf("snippet line missing highlight around term:\n%q", snippet)
	}
	if strings.Contains(meta, hlStart) {
		t.Errorf("metadata line must not be highlighted:\n%q", meta)
	}
}

// TestSelectedRowSnippetStillHighlights locks the highlight-within-
// selected-row interaction: the selected row's Primary/Bold snippet
// style and the highlight coexist (the highlighter re-applies active
// SGRs after each match close).
func TestSelectedRowSnippetStillHighlights(t *testing.T) {
	applyTheme(t)
	hlStart, _ := hlSGR(t)

	m := New()
	m.Open()
	submitQuery(&m, "deploy")
	m.SetHighlightTerms([]string{"deploy"})
	m.SetResults([]Item{
		{ChannelID: "C1", ChannelName: "general", UserName: "grant", TS: "1.0",
			Text: "deploy went fine"},
		{ChannelID: "C2", ChannelName: "ops", UserName: "sam", TS: "2.0",
			Text: "deploy bad"},
	}, 2)
	m.HandleKey("down") // select row 1

	lines := strings.Split(m.View(80, 30), "\n")
	selSnippet := lines[listTopOffset+rowLines+1]
	if !strings.Contains(selSnippet, hlStart+"deploy") {
		t.Errorf("selected row snippet missing highlight:\n%q", selSnippet)
	}
	// The selected row still carries its ▌ indicator and the visible
	// text survives intact.
	if plain := ansi.Strip(selSnippet); !strings.Contains(plain, "▌  deploy bad") {
		t.Errorf("selected snippet line content mangled: %q", plain)
	}
}

func TestNoHighlightWithoutTerms(t *testing.T) {
	applyTheme(t)
	hlStart, _ := hlSGR(t)

	m := New()
	m.Open()
	submitQuery(&m, "deploy")
	// No SetHighlightTerms call (or an empty result from a
	// modifiers-only query): nothing lights up.
	m.SetResults([]Item{
		{ChannelID: "C1", ChannelName: "general", UserName: "grant", TS: "1.0",
			Text: "deploy went fine"},
	}, 1)

	if out := m.View(80, 30); strings.Contains(out, hlStart) {
		t.Errorf("highlight SGR present without terms:\n%q", out)
	}
}

// TestHighlightDoesNotAffectGeometry verifies the zero-width highlight
// SGRs leave the box geometry untouched: BoxSize still matches the
// render and every line is exactly box-wide, including a long snippet
// that exercises the wrap + ellipsis paths.
func TestHighlightDoesNotAffectGeometry(t *testing.T) {
	applyTheme(t)

	m := New()
	m.Open()
	submitQuery(&m, "deploy")
	m.SetHighlightTerms([]string{"deploy", "lorem"})
	long := strings.Repeat("deploy lorem ipsum ", 20)
	m.SetResults([]Item{
		{ChannelID: "C1", ChannelName: "general", UserName: "grant", TS: "1.0", Text: long},
		{ChannelID: "C2", ChannelName: "ops", UserName: "sam", TS: "2.0", Text: "deploy"},
	}, 2)

	box := m.renderBox(80, 30)
	w, h := m.BoxSize(80, 30)
	if gw, gh := lipgloss.Width(box), lipgloss.Height(box); gw != w || gh != h {
		t.Errorf("rendered %dx%d, BoxSize %dx%d (highlight ANSI broke geometry?)", gw, gh, w, h)
	}
	for i, l := range strings.Split(box, "\n") {
		if lw := lipgloss.Width(l); lw != w {
			t.Errorf("line %d width = %d, want %d", i, lw, w)
		}
	}
}
