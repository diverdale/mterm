package tui

import "testing"

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
