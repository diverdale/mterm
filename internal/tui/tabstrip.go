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
// focused tab; spinnerCounter drives any connecting spinners. syncIDs holds
// the tab IDs in the broadcast set — those tabs get a leading sync glyph in
// warning color. The result is padded/truncated to width and contains no
// newline.
func renderTabStrip(tabs []*sessionTab, activeIdx, spinnerCounter, width int, s styleSet, syncIDs map[int]bool) string {
	chips := make([]string, 0, len(tabs))
	for i, t := range tabs {
		// The status glyph carries its own color. Render it as its own span
		// between separately-styled text spans — never nest pre-styled content
		// inside another style, since the glyph's embedded reset would break
		// the outer style for everything after it.
		glyph := statusGlyph(t.status(), spinnerCounter)
		text := fmt.Sprintf(" %d %s", i+1, t.title())
		inSync := syncIDs[t.id]
		// The active tab is outlined with brackets; inactive tabs use matching
		// blank padding so labels do not shift when a tab gains or loses focus.
		// Sync membership replaces the inner padding cell with a warning-color
		// asterisk — width stays uniform either way.
		if i == activeIdx {
			if inSync {
				chips = append(chips, s.tabActive.Render("[")+syncGlyph()+glyph+s.tabActive.Render(text+" ]"))
			} else {
				chips = append(chips, s.tabActive.Render("[ ")+glyph+s.tabActive.Render(text+" ]"))
			}
		} else {
			if inSync {
				chips = append(chips, syncGlyph()+s.tabInactive.Render(" ")+glyph+s.tabInactive.Render(text+"  "))
			} else {
				chips = append(chips, s.tabInactive.Render("  ")+glyph+s.tabInactive.Render(text+"  "))
			}
		}
	}
	strip := lipgloss.JoinHorizontal(lipgloss.Top, chips...)
	// Hard-truncate to width (ANSI-aware) so the result is always one line;
	// lipgloss MaxWidth word-wraps rather than truncating.
	strip = ansi.Truncate(strip, width, "")
	return lipgloss.NewStyle().Width(width).Render(strip)
}

// syncGlyph is the leading mark for tabs in the broadcast set. Warning color
// signals "what you type goes to multiple machines."
func syncGlyph() string {
	return lipgloss.NewStyle().Foreground(active.Warning).Bold(true).Render("*")
}
