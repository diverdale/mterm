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
	if s.border.GetForeground() != sty.border.GetForeground() {
		t.Fatalf("empty override should match global sty.border foreground")
	}
	if s.title.GetForeground() != sty.title.GetForeground() {
		t.Fatalf("empty override should match global sty.title foreground")
	}
	if s.tabActive.GetForeground() != sty.tabActive.GetForeground() {
		t.Fatalf("empty override should match global sty.tabActive foreground")
	}
	if s.footerInfo.GetForeground() != sty.footerInfo.GetForeground() {
		t.Fatalf("empty override should match global sty.footerInfo foreground")
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
	} {
		if got != want {
			t.Errorf("%s foreground = %v, want %v", name, got, want)
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
}
