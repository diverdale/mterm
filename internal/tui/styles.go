package tui

import "github.com/charmbracelet/lipgloss"

var (
	tabActive = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("231"))

	tabInactive = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("245"))

	statusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	errorText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))

	modalBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)
)
