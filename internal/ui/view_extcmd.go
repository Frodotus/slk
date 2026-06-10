package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

// extCmdOutputOverlay renders a captured command's stdout in a centered
// box over a dimmed screen. Long output is truncated to fit the height
// (scrolling is a follow-up).
func (a *App) extCmdOutputOverlay(termWidth, termHeight int, background string) string {
	bg := styles.Background
	boxW := termWidth * 3 / 4
	if boxW < 40 {
		boxW = 40
	}
	if boxW > termWidth-4 {
		boxW = termWidth - 4
	}
	innerW := boxW - 4

	title := lipgloss.NewStyle().Bold(true).Background(bg).Foreground(styles.Primary).
		Render("Output: " + a.extCmdOutputName)
	footer := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Render("esc close")

	maxLines := termHeight - 8
	if maxLines < 3 {
		maxLines = 3
	}
	lines := strings.Split(strings.TrimRight(a.extCmdOutput, "\n"), "\n")
	truncated := false
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	bodyStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
	body := make([]string, 0, len(lines)+1)
	for _, ln := range lines {
		if lipgloss.Width(ln) > innerW {
			ln = truncate.StringWithTail(ln, uint(innerW), "…")
		}
		body = append(body, bodyStyle.Render(ln))
	}
	if truncated {
		body = append(body, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).
			Italic(true).Render("… (output truncated)"))
	}

	content := title + "\n\n" + strings.Join(body, "\n") + "\n\n" + footer
	content = messages.ReapplyBgAfterResets(content, messages.BgANSI()+messages.FgANSI())
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(bg).
		Background(bg).
		Padding(1, 1).
		Width(boxW).
		Render(content)
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}
