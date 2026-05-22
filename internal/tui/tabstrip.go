package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		// The status glyph carries its own color. Render it as its own span
		// between separately-styled text spans — never nest pre-styled content
		// inside another style, since the glyph's embedded reset would break
		// the outer style for everything after it.
		glyph := statusGlyph(t.status(), spinnerCounter)
		text := fmt.Sprintf(" %d %s", i+1, t.title())
		if i == activeIdx {
			// The active tab is outlined with brackets; inactive tabs use
			// matching blank padding so a tab's label does not shift when it
			// gains or loses focus.
			chips = append(chips, sty.tabActive.Render("[ ")+glyph+sty.tabActive.Render(text+" ]"))
		} else {
			chips = append(chips, sty.tabInactive.Render("  ")+glyph+sty.tabInactive.Render(text+"  "))
		}
	}
	strip := lipgloss.JoinHorizontal(lipgloss.Top, chips...)
	// Hard-truncate to width (ANSI-aware) so the result is always one line;
	// lipgloss MaxWidth word-wraps rather than truncating.
	strip = ansi.Truncate(strip, width, "")
	return lipgloss.NewStyle().Width(width).Render(strip)
}
