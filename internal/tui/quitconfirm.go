package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/appmeta"
)

// quitConfirmedMsg dispatches the actual shutdown after the user said yes.
type quitConfirmedMsg struct{}

// quitCancelledMsg returns the app to its prior mode.
type quitCancelledMsg struct{}

// quitConfirmModel is a tiny modal that asks "really quit?" before exit.
// Bare Ctrl-C opens it (the accidental-press guard); ^B q still quits
// directly (already a deliberate two-step).
type quitConfirmModel struct {
	tabCount int
}

func newQuitConfirmModel(tabCount int) *quitConfirmModel {
	return &quitConfirmModel{tabCount: tabCount}
}

// Update consumes one key. Only y/Y/Enter confirm; any other key (including
// a second Ctrl-C) cancels.
func (q *quitConfirmModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		return func() tea.Msg { return quitConfirmedMsg{} }
	case tea.KeyRunes:
		if len(msg.Runes) == 1 && (msg.Runes[0] == 'y' || msg.Runes[0] == 'Y') {
			return func() tea.Msg { return quitConfirmedMsg{} }
		}
	}
	return func() tea.Msg { return quitCancelledMsg{} }
}

// View renders the centered confirm modal.
func (q *quitConfirmModel) View() string {
	var body string
	switch q.tabCount {
	case 0:
		body = "Exit?"
	case 1:
		body = "Disconnect 1 session and exit?"
	default:
		body = fmt.Sprintf("Disconnect %d sessions and exit?", q.tabCount)
	}

	var b strings.Builder
	b.WriteString(sty.title.Render("Quit " + appmeta.Name + "?"))
	b.WriteString("\n\n")
	b.WriteString(body)
	b.WriteString("\n\n")
	b.WriteString(sty.footerKey.Render("[Y]"))
	b.WriteString(" ")
	b.WriteString(sty.footerHint.Render("quit"))
	b.WriteString("       ")
	b.WriteString(sty.footerKey.Render("[N/Esc]"))
	b.WriteString(" ")
	b.WriteString(sty.footerHint.Render("cancel"))

	// Amber border conveys "wait, are you sure" — distinct from the accent
	// blue/green of the regular modals.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Warning).
		Padding(1, 2)
	return box.Render(b.String())
}
