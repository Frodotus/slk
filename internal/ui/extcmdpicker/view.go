package extcmdpicker

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

const maxVisibleRows = 10

func boxWidth(termWidth int) int {
	w := termWidth / 2
	if w < 40 {
		w = 40
	}
	if w > 70 {
		w = 70
	}
	if w > termWidth-4 {
		w = termWidth - 4
	}
	return w
}

// ViewOverlay renders the picker centered on a dimmed copy of the screen.
func (m Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	box := m.renderBox(termWidth)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	overlayWidth := boxWidth(termWidth)
	innerWidth := overlayWidth - 4
	bg := styles.Background

	title := lipgloss.NewStyle().Bold(true).Background(bg).Foreground(styles.Primary).
		Render("Run command")

	input := m.renderInputRow(innerWidth, bg)
	rows := m.renderRows(innerWidth, bg)
	footer := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).
		Render("enter run  esc cancel")

	content := title + "\n" + input + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
	content = messages.ReapplyBgAfterResets(content, messages.BgANSI()+messages.FgANSI())

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(bg).
		Background(bg).
		Padding(1, 1).
		Width(overlayWidth).
		Render(content)
}

func (m Model) renderInputRow(innerWidth int, bg color.Color) string {
	prefix := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Render("> ")
	q := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
	var b strings.Builder
	b.WriteString(prefix)
	if m.query == "" {
		b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Render("filter…"))
	} else {
		b.WriteString(q.Render(m.query))
		b.WriteString(q.Render("█"))
	}
	row := b.String()
	if lipgloss.Width(row) > innerWidth {
		row = truncate.StringWithTail(row, uint(innerWidth), "…")
	}
	return row
}

func (m Model) renderRows(innerWidth int, bg color.Color) []string {
	if len(m.filtered) == 0 {
		msg := "No commands configured"
		if len(m.items) > 0 {
			msg = fmt.Sprintf("No commands match %q", m.query)
		}
		return []string{lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Italic(true).Render(msg)}
	}

	// Scroll the visible window to keep the highlight in view.
	start := 0
	if m.highlight >= maxVisibleRows {
		start = m.highlight - maxVisibleRows + 1
	}
	end := start + maxVisibleRows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var rows []string
	for i := start; i < end; i++ {
		name := m.items[m.filtered[i]]
		highlight := i == m.highlight
		var nameStyle lipgloss.Style
		if highlight {
			nameStyle = lipgloss.NewStyle().Background(bg).Foreground(styles.Primary).Bold(true)
		} else {
			nameStyle = lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
		}
		line := nameStyle.Render(name)
		if lipgloss.Width(line) > innerWidth-1 {
			line = truncate.StringWithTail(line, uint(innerWidth-1), "…")
		}
		if pad := innerWidth - 1 - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		if highlight {
			rows = append(rows, lipgloss.NewStyle().Background(bg).Foreground(styles.Accent).Render("▌")+line)
		} else {
			rows = append(rows, " "+line)
		}
	}
	return rows
}
