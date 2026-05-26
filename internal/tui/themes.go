package tui

import "github.com/charmbracelet/lipgloss"

// Matrix is the phosphor-green theme.
var Matrix = Theme{
	Name:      "matrix",
	Accent:    lipgloss.Color("#00FF41"),
	Frame:     lipgloss.Color("#1A1A1A"),
	Dim:       lipgloss.Color("#6B7280"),
	Text:      lipgloss.Color("#00CC33"),
	Warning:   lipgloss.Color("#FFB347"),
	Error:     lipgloss.Color("#FF4747"),
	Connected: lipgloss.Color("#00FF41"),
}

// Synthwave is the magenta theme.
var Synthwave = Theme{
	Name:      "synthwave",
	Accent:    lipgloss.Color("#F92AAD"),
	Frame:     lipgloss.Color("#3A1A40"),
	Dim:       lipgloss.Color("#7A4A7A"),
	Text:      lipgloss.Color("#E0E0E0"),
	Warning:   lipgloss.Color("#FFD93D"),
	Error:     lipgloss.Color("#FF5C7A"),
	Connected: lipgloss.Color("#36F1CD"),
}

// Themes returns the available themes in canonical order.
func Themes() []Theme {
	return []Theme{Midnight, Matrix, Synthwave}
}

// setTheme switches the active theme and rebuilds the chrome style set.
// Subsequent renders use the new colors on the next render tick.
func setTheme(t Theme) {
	active = t
	sty = buildStyles(active)
}
