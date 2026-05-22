package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// keyHint is one keybinding hint shown in the footer.
type keyHint struct {
	key   string // e.g. "^B n"
	label string // e.g. "next"
}

// footerOpts configures a footer line.
type footerOpts struct {
	hints         []keyHint // left-side keybinding hints
	info          string    // right-side context (user@host:port, or "N hosts")
	timer         string    // far-right timer text, e.g. "00:01:30"; "" to omit
	prefixPending bool      // show the PREFIX badge instead of hints
	width         int
}

// renderFooter renders the one-line footer. Hints sit on the left, info and
// clock on the right. On a narrow width the right side is dropped before the
// left. The result has no newline and is exactly width wide.
func renderFooter(o footerOpts) string {
	var left string
	if o.prefixPending {
		left = sty.prefixBadge.Render("PREFIX")
	} else {
		parts := make([]string, 0, len(o.hints))
		for _, h := range o.hints {
			parts = append(parts, sty.footerKey.Render(h.key)+" "+sty.footerHint.Render(h.label))
		}
		left = strings.Join(parts, sty.footerHint.Render("  ·  "))
	}

	right := o.info
	if o.timer != "" {
		if right != "" {
			right += "  "
		}
		right += o.timer
	}
	right = sty.footerInfo.Render(right)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	// Drop the right side if it would not fit with at least one space gap.
	if leftW+rightW+1 > o.width {
		right = ""
		rightW = 0
	}
	gap := o.width - leftW - rightW
	if gap < 0 {
		gap = 0
	}
	line := left + strings.Repeat(" ", gap) + right
	// Hard-truncate (ANSI-aware) so a too-long left side cannot wrap and break
	// the frame width; then pad to exactly width.
	line = ansi.Truncate(line, o.width, "")
	return lipgloss.NewStyle().Width(o.width).Render(line)
}
