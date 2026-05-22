package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// statusGlyph returns the colored glyph for a tab's connection status. While
// connecting it animates via the spinner counter.
func statusGlyph(s tabStatus, spinnerCounter int) string {
	switch s {
	case statusConnected:
		return sty.connected.Render("●")
	case statusFailed:
		return sty.errorText.Render("✖")
	default:
		return sty.footerKey.Render(spinnerGlyph(spinnerCounter))
	}
}

// renderTabStrip renders the one-line row of tab chips. activeIdx is the
// focused tab; spinnerCounter drives any connecting spinners. The result is
// padded/truncated to width and contains no newline.
func renderTabStrip(tabs []*sessionTab, activeIdx, spinnerCounter, width int) string {
	chips := make([]string, 0, len(tabs))
	for i, t := range tabs {
		label := fmt.Sprintf("%s %d %s", statusGlyph(t.status(), spinnerCounter), i+1, t.title())
		if i == activeIdx {
			chips = append(chips, sty.tabActive.Render(label))
		} else {
			chips = append(chips, sty.tabInactive.Render(label))
		}
	}
	strip := lipgloss.JoinHorizontal(lipgloss.Top, chips...)
	// Pad or truncate to exactly width so the frame stays aligned.
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strip)
}
