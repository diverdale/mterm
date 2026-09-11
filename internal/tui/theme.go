package tui

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

// selectionFgDark and selectionFgLight are the two candidate foregrounds for
// text rendered on top of an accent-colored background (selection bar, PREFIX
// badge). contrastingForeground picks between them from the accent luminance.
const (
	selectionFgDark  = lipgloss.Color("#0B0F14")
	selectionFgLight = lipgloss.Color("#E8ECF0")
)

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

// chromeStylesFor returns a styleSet identical to the active set, except that
// the host-tinted styles use c instead of the theme's Frame/Accent. When c is
// "", returns the global sty unchanged so callers can use this unconditionally.
//
// The override preserves every non-Foreground attribute of the base style
// (Bold, Italic, etc.) by copying-then-replacing the foreground, so future
// theme tweaks like adding Italic to title carry through automatically.
//
// footerKey is included so the ^B / ^B n / ^B x cues in the footer match the
// frame color. footerHint (the "menu/next/close" labels) and prefixBadge are
// deliberately not tinted — the labels stay dim for readability, and the
// PREFIX badge is a transient input-mode signal, not host-related chrome.
func chromeStylesFor(c string) styleSet {
	if c == "" {
		return sty
	}
	col := lipgloss.Color(c)
	s := sty
	s.border = sty.border.Foreground(col)
	s.title = sty.title.Foreground(col)
	s.tabActive = sty.tabActive.Foreground(col)
	s.footerInfo = sty.footerInfo.Foreground(col)
	s.footerKey = sty.footerKey.Foreground(col)
	return s
}

// buildStyles derives the full style set from a theme. A future theme switcher
// re-runs this with a different theme.
func buildStyles(t Theme) styleSet {
	return styleSet{
		border:       lipgloss.NewStyle().Foreground(t.Frame),
		title:        lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		titleDim:     lipgloss.NewStyle().Foreground(t.Dim),
		tabActive:    lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		tabInactive:  lipgloss.NewStyle().Foreground(t.Dim),
		footerHint:   lipgloss.NewStyle().Foreground(t.Dim),
		footerKey:    lipgloss.NewStyle().Foreground(t.Accent),
		footerInfo:   lipgloss.NewStyle().Foreground(t.Dim),
		prefixBadge:  lipgloss.NewStyle().Background(t.Accent).Foreground(contrastingForeground(t.Accent)).Bold(true).Padding(0, 1),
		selectionBar: lipgloss.NewStyle().Background(t.Accent).Foreground(contrastingForeground(t.Accent)),
		groupHeader:  lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		search:       lipgloss.NewStyle().Foreground(t.Accent),
		errorText:    lipgloss.NewStyle().Foreground(t.Error),
		connected:    lipgloss.NewStyle().Foreground(t.Connected),
		dim:          lipgloss.NewStyle().Foreground(t.Dim),
	}
}

// contrastingForeground returns a readable text color for content rendered on
// top of bg. Used for selection bars and badges so user themes with light or
// dark accents stay legible.
func contrastingForeground(bg lipgloss.Color) lipgloss.Color {
	r, g, b, ok := parseRGB(bg)
	if !ok {
		return selectionFgDark
	}
	if relativeLuminance(r, g, b) > 0.45 {
		return selectionFgDark
	}
	return selectionFgLight
}

func parseRGB(c lipgloss.Color) (r, g, b float64, ok bool) {
	s := strings.TrimPrefix(strings.ToLower(string(c)), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	rv, err1 := strconv.ParseUint(s[0:2], 16, 8)
	gv, err2 := strconv.ParseUint(s[2:4], 16, 8)
	bv, err3 := strconv.ParseUint(s[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return float64(rv) / 255, float64(gv) / 255, float64(bv) / 255, true
}

func relativeLuminance(r, g, b float64) float64 {
	toLinear := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}
