package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpClosedMsg dismisses the help overlay.
type helpClosedMsg struct{}

// helpModel is the static keybinding help overlay.
type helpModel struct{}

// newHelpModel constructs the help overlay model.
func newHelpModel() *helpModel { return &helpModel{} }

// Update dismisses the overlay on any key (Esc or otherwise — the overlay
// holds no internal state worth navigating).
func (h *helpModel) Update(_ tea.KeyMsg) tea.Cmd {
	return func() tea.Msg { return helpClosedMsg{} }
}

type kb struct{ key, desc string }

var helpSections = []struct {
	name  string
	binds []kb
}{
	{"Prefix", []kb{
		{"^B c", "open picker / new connection"},
		{"^B n / ^B p", "next / previous tab"},
		{"^B 1..9", "jump to tab N"},
		{"^B x", "close current tab"},
		{"^B f", "open forwards panel"},
		{"^B p", "open command palette"},
		{"^B ?", "this help"},
		{"^B q", "quit mterm"},
	}},
	{"Picker", []kb{
		{"type", "filter hosts"},
		{"↑ / ↓", "move cursor"},
		{"enter", "connect"},
		{"esc", "back to active session"},
	}},
	{"Forwards panel", []kb{
		{"space / enter", "toggle"},
		{"esc", "close"},
	}},
	{"Palette", []kb{
		{"type", "filter commands"},
		{"↑ / ↓", "move cursor"},
		{"enter", "run"},
		{"esc", "cancel"},
	}},
	{"Global", []kb{
		{"^C", "quit mterm"},
	}},
}

// View renders the keybinding table inside a rounded modal box.
func (h *helpModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("mterm — keybindings"))
	b.WriteString("\n")
	for _, sec := range helpSections {
		b.WriteString("\n")
		b.WriteString(sty.groupHeader.Render("  " + sec.name))
		b.WriteString("\n")
		for _, k := range sec.binds {
			b.WriteString(fmt.Sprintf("    %s  %s\n",
				sty.footerKey.Render(padRight(k.key, 14)),
				sty.dim.Render(k.desc)))
		}
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("           esc to dismiss"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2)
	return box.Render(b.String())
}

// padRight pads s with spaces on the right to width n; if s is already wider,
// it is returned unchanged.
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}
