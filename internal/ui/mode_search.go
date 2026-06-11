// internal/ui/mode_search.go
//
// Search-mode key handler: the in-channel `/` prompt.
//
// The prompt is an input buffer rendered in the status line's search
// segment. Enter executes the FTS query for the active channel (via
// the SearchService) and returns to Normal mode; the results land as
// a ChannelSearchResultsMsg (see reducer_search.go). Esc cancels.
package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
)

func handleSearchMode(a *App, msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.keys.Escape):
		a.searchInput = ""
		a.statusbar.SetSearch("")
		a.SetMode(ModeNormal)
		return nil
	case key.Matches(msg, a.keys.Enter):
		query := strings.TrimSpace(a.searchInput)
		a.searchInput = ""
		a.SetMode(ModeNormal)
		if query == "" {
			a.clearActiveSearch()
			a.statusbar.SetSearch("")
			return nil
		}
		a.statusbar.SetSearch("/" + query + "  …")
		search := a.searchSvc
		chID := a.activeChannelID
		return func() tea.Msg {
			return search.SearchChannel(ids.ChannelID(chID), query)
		}
	}
	// Text editing: append printable runes, backspace deletes.
	switch s := normalizeFinderKey(msg); s {
	case "backspace":
		if a.searchInput != "" {
			r := []rune(a.searchInput)
			a.searchInput = string(r[:len(r)-1])
		}
	case "space":
		// Key.String() renders a literal space as "space"; queries
		// can be multi-term, so map it back.
		a.searchInput += " "
	default:
		if len([]rune(s)) == 1 {
			a.searchInput += s
		}
	}
	a.statusbar.SetSearch("/" + a.searchInput)
	return nil
}
