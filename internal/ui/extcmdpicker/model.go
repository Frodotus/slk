// Package extcmdpicker is a small single-select overlay for choosing an
// external command to run against the selected message. It is decoupled
// from config: it holds display names and returns the chosen index.
package extcmdpicker

import "strings"

// Result is returned when the user picks a command; Index is the offset
// into the items slice passed to SetItems.
type Result struct {
	Index int
}

type Model struct {
	items     []string
	query     string
	filtered  []int // indices into items matching query
	highlight int   // index into filtered
	visible   bool
}

// New constructs an empty picker. Call SetItems before Open.
func New() Model { return Model{} }

// SetItems replaces the command names the picker filters over.
func (m *Model) SetItems(items []string) {
	m.items = items
	m.filter()
}

// Open resets the query/selection and shows the picker.
func (m *Model) Open() {
	m.query = ""
	m.highlight = 0
	m.visible = true
	m.filter()
}

// Close hides the picker.
func (m *Model) Close() { m.visible = false }

// IsVisible reports whether the picker is showing.
func (m Model) IsVisible() bool { return m.visible }

func (m *Model) filter() {
	m.filtered = m.filtered[:0]
	q := strings.ToLower(m.query)
	for i, it := range m.items {
		if q == "" || strings.Contains(strings.ToLower(it), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.highlight >= len(m.filtered) {
		m.highlight = len(m.filtered) - 1
	}
	if m.highlight < 0 {
		m.highlight = 0
	}
}

// HandleKey advances the picker for a keypress. It returns a non-nil
// Result only when the user commits a selection with Enter.
func (m *Model) HandleKey(keyStr string) *Result {
	switch keyStr {
	case "enter":
		return m.submit()
	case "esc":
		m.Close()
		return nil
	case "down", "ctrl+n":
		if m.highlight < len(m.filtered)-1 {
			m.highlight++
		}
		return nil
	case "up", "ctrl+p":
		if m.highlight > 0 {
			m.highlight--
		}
		return nil
	case "backspace":
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
			m.highlight = 0
			m.filter()
		}
		return nil
	}
	if len(keyStr) == 1 && keyStr[0] >= 32 && keyStr[0] <= 126 {
		m.query += keyStr
		m.highlight = 0
		m.filter()
	}
	return nil
}

func (m *Model) submit() *Result {
	if m.highlight < 0 || m.highlight >= len(m.filtered) {
		return nil
	}
	r := &Result{Index: m.filtered[m.highlight]}
	m.Close()
	return r
}
