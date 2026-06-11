// internal/ui/reducer_search.go
//
// Reducer for the in-channel `/` search results plus the App-level
// helpers that drive match navigation (n/N with wrap) and the
// status-line `/query  i/N` segment.
//
// Message family:
//   ChannelSearchResultsMsg - FTS match list for the active channel
//
// Stale results (the user switched channels while the query ran) are
// dropped. An error clears search state and toasts; an empty result
// clears state and shows "no matches" in the status line. Otherwise
// the match list becomes the active search, highlights are pushed to
// the messages pane, and the selection jumps to the nearest match at
// or older than the cursor (FetchAround-ing a history window when the
// match is outside the loaded buffer).
package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
)

// activeSearch is the in-channel `/` search state: the query, its
// folded terms (for highlighting), the match ts list newest-first,
// and the index of the currently-selected match.
type activeSearch struct {
	query   string
	terms   []string
	matches []string
	idx     int
}

var reduceSearch reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(ChannelSearchResultsMsg)
	if !ok {
		return nil, false
	}
	if m.ChannelID != a.activeChannelID {
		return nil, true // stale: channel changed since query
	}
	if m.Err != nil {
		a.clearActiveSearch()
		return func() tea.Msg { return ToastMsg{Text: "Search failed"} }, true
	}
	if len(m.TSes) == 0 {
		a.clearActiveSearch()
		a.statusbar.SetSearch(fmt.Sprintf("/%s  no matches", m.Query))
		return nil, true
	}

	a.search = &activeSearch{query: m.Query, terms: m.Terms, matches: m.TSes}
	a.messagepane.SetSearchTerms(m.Terms)

	// Nearest match at or older than the cursor (matches are newest
	// first; slack ts strings compare lexicographically).
	cursorTS := ""
	if sel, ok := a.messagepane.SelectedMessage(); ok {
		cursorTS = sel.TS
	}
	a.search.idx = 0
	for i, ts := range m.TSes {
		if ts <= cursorTS {
			a.search.idx = i
			break
		}
	}
	return a.jumpToCurrentMatch(), true
}

// jumpToCurrentMatch selects the match at a.search.idx, fetching a
// history window when it is outside the loaded buffer, and refreshes
// the status-line search segment.
func (a *App) jumpToCurrentMatch() tea.Cmd {
	s := a.search
	if s == nil || len(s.matches) == 0 {
		return nil
	}
	a.statusbar.SetSearch(fmt.Sprintf("/%s  %d/%d", s.query, s.idx+1, len(s.matches)))
	ts := s.matches[s.idx]
	if a.messagepane.SelectByTS(ts) {
		return nil
	}
	channels := a.channels
	chID := a.activeChannelID
	return func() tea.Msg {
		return channels.FetchAround(ids.ChannelID(chID), ids.MessageTS(ts))
	}
}

// searchNext moves to the next-older match (wrapping to newest);
// searchPrev to the next-newer (wrapping to oldest). Both no-op
// without an active search.
func (a *App) searchNext() tea.Cmd {
	s := a.search
	if s == nil || len(s.matches) == 0 {
		return nil
	}
	var wrapped bool
	if s.idx++; s.idx >= len(s.matches) {
		s.idx, wrapped = 0, true
	}
	cmd := a.jumpToCurrentMatch()
	if wrapped {
		return tea.Batch(cmd, func() tea.Msg { return ToastMsg{Text: "Search wrapped"} })
	}
	return cmd
}

func (a *App) searchPrev() tea.Cmd {
	s := a.search
	if s == nil || len(s.matches) == 0 {
		return nil
	}
	var wrapped bool
	if s.idx--; s.idx < 0 {
		s.idx, wrapped = len(s.matches)-1, true
	}
	cmd := a.jumpToCurrentMatch()
	if wrapped {
		return tea.Batch(cmd, func() tea.Msg { return ToastMsg{Text: "Search wrapped"} })
	}
	return cmd
}
