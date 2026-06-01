package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Workspace-save modal messages.
type workspaceSaveSubmitMsg struct{ name string }
type workspaceSaveCancelMsg struct{}

// workspaceSaveModel is a tiny single-line text input shown after the user
// picks "Save current tabs as workspace" from the command palette. Enter
// submits the name; Esc cancels. The footer reminds the user of both.
type workspaceSaveModel struct {
	name     string
	tabCount int
}

func newWorkspaceSaveModel(tabCount int) *workspaceSaveModel {
	return &workspaceSaveModel{tabCount: tabCount}
}

func (w *workspaceSaveModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		name := strings.TrimSpace(w.name)
		if name == "" {
			return nil
		}
		return func() tea.Msg { return workspaceSaveSubmitMsg{name: name} }
	case tea.KeyEsc:
		return func() tea.Msg { return workspaceSaveCancelMsg{} }
	case tea.KeyBackspace:
		if w.name != "" {
			r := []rune(w.name)
			w.name = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		w.name += string(msg.Runes)
	}
	return nil
}

func (w *workspaceSaveModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Save workspace as"))
	b.WriteString("\n\n")
	b.WriteString(sty.dim.Render(fmt.Sprintf("Saving %d tab(s)", w.tabCount)))
	b.WriteString("\n\n")
	b.WriteString(sty.dim.Render("name: "))
	b.WriteString(sty.search.Render(w.name))
	b.WriteString(sty.dim.Render("_"))
	b.WriteString("\n\n")
	b.WriteString(sty.footerKey.Render("enter"))
	b.WriteString(" ")
	b.WriteString(sty.footerHint.Render("save"))
	b.WriteString("    ")
	b.WriteString(sty.footerKey.Render("esc"))
	b.WriteString(" ")
	b.WriteString(sty.footerHint.Render("cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2).
		Width(50)
	return box.Render(b.String())
}
