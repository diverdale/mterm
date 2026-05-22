package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// windowOpts configures a framed window. tabStrip is optional ("" omits the
// tab-strip row). body need not be pre-sized; the frame fits it to the inner
// content area.
type windowOpts struct {
	title    string
	tabStrip string // optional; "" for views without tabs (the picker)
	body     string
	footer   string
	width    int
	height   int
}

// renderWindow composes a bordered window: a titled top border, an optional
// tab-strip row, the body, a divider, the footer, and a bottom border. The
// result is exactly width × height when height is at least the chrome height
// (4 rows without a tab strip, 5 with one); for a smaller height the result
// is that chrome minimum.
func renderWindow(o windowOpts) string {
	if o.width < 4 {
		o.width = 4
	}
	inner := o.width - 2 // content width between the side borders

	var rows []string
	rows = append(rows, topBorder(o.title, o.width))
	if o.tabStrip != "" {
		rows = append(rows, sideRow(o.tabStrip, inner))
	}

	// Body fills whatever vertical space is left between the rows already
	// placed and the divider+footer+bottom-border (3 rows).
	bodyH := o.height - len(rows) - 3
	if bodyH < 0 {
		bodyH = 0
	}
	for _, ln := range fitLines(o.body, bodyH) {
		rows = append(rows, sideRow(ln, inner))
	}

	rows = append(rows, divider(o.width))
	rows = append(rows, sideRow(o.footer, inner))
	rows = append(rows, bottomBorder(o.width))
	return strings.Join(rows, "\n")
}

// clampLine truncates s to at most w display columns (ANSI-aware) and pads it
// out to exactly w. The result is always a single line exactly w wide.
// lipgloss MaxWidth is NOT used because it word-wraps overlong content.
func clampLine(s string, w int) string {
	if w < 0 {
		w = 0
	}
	s = ansi.Truncate(s, w, "")
	return lipgloss.NewStyle().Width(w).Render(s)
}

// sideRow wraps one inner line in left/right border characters. content is
// clamped to exactly innerWidth, so the row is exactly innerWidth+2 wide.
func sideRow(content string, innerWidth int) string {
	bar := sty.border.Render("│")
	return bar + clampLine(content, innerWidth) + bar
}

// topBorder builds "┌─ title ─────┐" exactly width wide. title is placed
// as-is; the caller pre-styles it.
func topBorder(title string, width int) string {
	prefix := "┌─ "
	fill := width - lipgloss.Width(prefix) - lipgloss.Width(title) - 2
	if fill < 0 {
		fill = 0
	}
	line := sty.border.Render(prefix) + title +
		sty.border.Render(" "+strings.Repeat("─", fill)+"┐")
	return ansi.Truncate(line, width, "")
}

// divider builds "├────────┤" exactly width wide.
func divider(width int) string {
	mid := width - 2
	if mid < 0 {
		mid = 0
	}
	return sty.border.Render("├" + strings.Repeat("─", mid) + "┤")
}

// bottomBorder builds "└────────┘" exactly width wide.
func bottomBorder(width int) string {
	mid := width - 2
	if mid < 0 {
		mid = 0
	}
	return sty.border.Render("└" + strings.Repeat("─", mid) + "┘")
}

// fitLines splits s into lines and returns exactly want lines: short input is
// padded with blank lines, long input is truncated. Width clamping is done by
// sideRow, not here.
func fitLines(s string, want int) []string {
	src := strings.Split(s, "\n")
	out := make([]string, want)
	for i := 0; i < want; i++ {
		if i < len(src) {
			out[i] = src[i]
		}
	}
	return out
}
