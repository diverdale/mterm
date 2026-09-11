package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMidnightIsActiveByDefault(t *testing.T) {
	if active.Name != "midnight" {
		t.Fatalf("active theme = %q, want midnight", active.Name)
	}
}

func TestBuildStylesUsesThemeColors(t *testing.T) {
	s := buildStyles(Midnight)
	if got := s.title.GetForeground(); got != Midnight.Accent {
		t.Fatalf("title foreground = %v, want accent %v", got, Midnight.Accent)
	}
	if got := s.errorText.GetForeground(); got != Midnight.Error {
		t.Fatalf("errorText foreground = %v, want error %v", got, Midnight.Error)
	}
}

func TestChromeStylesForEmptyReturnsGlobal(t *testing.T) {
	s := chromeStylesFor("")
	// Compare colors AND bold attributes — a buggy "rebuild styles on empty"
	// implementation would likely reproduce the colors but drop bold on
	// title/tabActive, and this test should catch that.
	if s.border.GetForeground() != sty.border.GetForeground() {
		t.Fatalf("border foreground differs from global")
	}
	if s.title.GetForeground() != sty.title.GetForeground() || s.title.GetBold() != sty.title.GetBold() {
		t.Fatalf("title style differs from global (fg or bold)")
	}
	if s.tabActive.GetForeground() != sty.tabActive.GetForeground() || s.tabActive.GetBold() != sty.tabActive.GetBold() {
		t.Fatalf("tabActive style differs from global (fg or bold)")
	}
	if s.footerInfo.GetForeground() != sty.footerInfo.GetForeground() {
		t.Fatalf("footerInfo foreground differs from global")
	}
	if s.footerKey.GetForeground() != sty.footerKey.GetForeground() {
		t.Fatalf("footerKey foreground differs from global")
	}
}

func TestChromeStylesForOverridesHostTintedStyles(t *testing.T) {
	want := lipgloss.Color("#FF3344")
	s := chromeStylesFor("#FF3344")
	for name, got := range map[string]lipgloss.TerminalColor{
		"border":     s.border.GetForeground(),
		"title":      s.title.GetForeground(),
		"tabActive":  s.tabActive.GetForeground(),
		"footerInfo": s.footerInfo.GetForeground(),
		"footerKey":  s.footerKey.GetForeground(),
	} {
		if got != want {
			t.Errorf("%s foreground = %v, want %v", name, got, want)
		}
	}
}

func TestChromeStylesForPreservesBoldOnOverride(t *testing.T) {
	// title and tabActive are bold in the base theme; the host-color override
	// must inherit that — otherwise a future theme tweak that adds Italic or
	// Underline would silently get dropped on host-tinted tabs.
	s := chromeStylesFor("#FF3344")
	if !s.title.GetBold() {
		t.Errorf("title lost Bold after override")
	}
	if !s.tabActive.GetBold() {
		t.Errorf("tabActive lost Bold after override")
	}
}

func TestContrastingForegroundPicksReadablePair(t *testing.T) {
	cases := []struct {
		bg   lipgloss.Color
		want lipgloss.Color
	}{
		{Midnight.Accent, selectionFgDark},
		{Matrix.Accent, selectionFgDark},
		{Synthwave.Accent, selectionFgLight},
		{lipgloss.Color("#0B0F14"), selectionFgLight},
		{lipgloss.Color("#E8ECF0"), selectionFgDark},
	}
	for _, tc := range cases {
		if got := contrastingForeground(tc.bg); got != tc.want {
			t.Errorf("contrastingForeground(%v) = %v, want %v", tc.bg, got, tc.want)
		}
	}
}

func TestSelectionBarForegroundDiffersFromAccent(t *testing.T) {
	for _, th := range Themes() {
		s := buildStyles(th)
		if s.selectionBar.GetForeground() == s.selectionBar.GetBackground() {
			t.Errorf("theme %q: selection bar fg == bg (%v)", th.Name, s.selectionBar.GetBackground())
		}
	}
}

func TestChromeStylesForPreservesUntintedStyles(t *testing.T) {
	s := chromeStylesFor("#FF3344")
	// These stay theme colors and must not be overridden.
	if s.dim.GetForeground() != sty.dim.GetForeground() {
		t.Errorf("dim foreground should not change")
	}
	if s.errorText.GetForeground() != sty.errorText.GetForeground() {
		t.Errorf("errorText foreground should not change")
	}
	if s.connected.GetForeground() != sty.connected.GetForeground() {
		t.Errorf("connected foreground should not change")
	}
	// footerHint (the "menu/next/close" labels) and prefixBadge stay theme-default
	// so they read consistently across hosts.
	if s.footerHint.GetForeground() != sty.footerHint.GetForeground() {
		t.Errorf("footerHint foreground should not change")
	}
	if s.prefixBadge.GetBackground() != sty.prefixBadge.GetBackground() {
		t.Errorf("prefixBadge background should not change")
	}
}
