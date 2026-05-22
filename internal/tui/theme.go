package tui

import "github.com/charmbracelet/lipgloss"

// Theme is a named chrome color palette. The session content is never themed;
// only mterm's own frame/tabs/footer use these colors.
type Theme struct {
	Name      string
	Accent    lipgloss.Color // active tab, host name, focus highlights
	Frame     lipgloss.Color // window borders, dividers
	Dim       lipgloss.Color // inactive tabs, secondary text, group headers
	Text      lipgloss.Color // primary foreground
	Warning   lipgloss.Color // amber
	Error     lipgloss.Color // red — connection/auth errors
	Connected lipgloss.Color // green — connected status dot
}

// Midnight is the v1 default theme: dark slate with a bright cyan accent.
var Midnight = Theme{
	Name:      "midnight",
	Accent:    lipgloss.Color("#5ED4E0"),
	Frame:     lipgloss.Color("#5C6370"),
	Dim:       lipgloss.Color("#6B7280"),
	Text:      lipgloss.Color("#D8DEE9"),
	Warning:   lipgloss.Color("#E5C07B"),
	Error:     lipgloss.Color("#E06C75"),
	Connected: lipgloss.Color("#98C379"),
}

// onAccent is the foreground used on top of an Accent background.
const onAccent = lipgloss.Color("#0B0F14")

// styleSet holds every chrome style, derived from a Theme.
type styleSet struct {
	border       lipgloss.Style // window border characters
	title        lipgloss.Style // active host name in the title
	titleDim     lipgloss.Style // "[N tabs]" count
	tabActive    lipgloss.Style // active tab chip
	tabInactive  lipgloss.Style // inactive tab chip
	footerHint   lipgloss.Style // footer hint label text
	footerKey    lipgloss.Style // footer hint key text
	footerInfo   lipgloss.Style // footer right-side info
	prefixBadge  lipgloss.Style // PREFIX badge
	selectionBar lipgloss.Style // picker cursor row
	groupHeader  lipgloss.Style // picker group header
	search       lipgloss.Style // picker search query text
	errorText    lipgloss.Style // error messages
	connected    lipgloss.Style // green status glyph
	dim          lipgloss.Style // generic secondary text
}

// active is the current theme. v1 has no runtime switcher.
var active = Midnight

// sty is the chrome style set, built once from the active theme.
var sty = buildStyles(active)

// buildStyles derives the full style set from a theme. A future theme switcher
// re-runs this with a different theme.
func buildStyles(t Theme) styleSet {
	return styleSet{
		border:       lipgloss.NewStyle().Foreground(t.Frame),
		title:        lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		titleDim:     lipgloss.NewStyle().Foreground(t.Dim),
		tabActive:    lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent).Bold(true).Padding(0, 1),
		tabInactive:  lipgloss.NewStyle().Foreground(t.Dim).Padding(0, 1),
		footerHint:   lipgloss.NewStyle().Foreground(t.Dim),
		footerKey:    lipgloss.NewStyle().Foreground(t.Accent),
		footerInfo:   lipgloss.NewStyle().Foreground(t.Dim),
		prefixBadge:  lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent).Bold(true).Padding(0, 1),
		selectionBar: lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent),
		groupHeader:  lipgloss.NewStyle().Foreground(t.Dim).Bold(true),
		search:       lipgloss.NewStyle().Foreground(t.Accent),
		errorText:    lipgloss.NewStyle().Foreground(t.Error),
		connected:    lipgloss.NewStyle().Foreground(t.Connected),
		dim:          lipgloss.NewStyle().Foreground(t.Dim),
	}
}
