package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestThemesIncludesAllShipped(t *testing.T) {
	got := []string{}
	for _, th := range Themes() {
		got = append(got, th.Name)
	}
	want := []string{"midnight", "matrix", "synthwave"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Themes() names = %v, want %v", got, want)
	}
}

func TestSetThemeSwitchesActiveAndStyles(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })

	setTheme(Matrix)
	if active.Name != "matrix" {
		t.Fatalf("active.Name = %q, want matrix", active.Name)
	}
	if got := sty.title.GetForeground(); got != Matrix.Accent {
		t.Fatalf("after setTheme(Matrix), title fg = %v, want %v", got, Matrix.Accent)
	}

	setTheme(Synthwave)
	if active.Name != "synthwave" {
		t.Fatalf("active.Name = %q, want synthwave", active.Name)
	}
}

func TestLoadUserThemesMissingFileIsNotError(t *testing.T) {
	got, err := LoadUserThemes(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice for missing file, got %v", got)
	}
}

func TestLoadUserThemesRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "themes.yaml")
	if err := os.WriteFile(path, []byte(`
nordlike:
  accent: "#88C0D0"
  frame: "#3B4252"
  dim: "#4C566A"
  text: "#ECEFF4"
  warning: "#EBCB8B"
  error: "#BF616A"
  connected: "#A3BE8C"

just-the-accent:
  accent: "#FF00FF"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	themes, err := LoadUserThemes(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 2 {
		t.Fatalf("expected 2 themes, got %d", len(themes))
	}
	// Sorted alphabetically.
	if themes[0].Name != "just-the-accent" || themes[1].Name != "nordlike" {
		t.Fatalf("themes out of order: %v", []string{themes[0].Name, themes[1].Name})
	}
	if themes[1].Accent != lipgloss.Color("#88C0D0") {
		t.Errorf("nordlike accent = %v, want #88C0D0", themes[1].Accent)
	}
	// Partial theme — unset fields inherit Midnight.
	if themes[0].Frame != Midnight.Frame {
		t.Errorf("partial theme should fall back to Midnight.Frame; got %v", themes[0].Frame)
	}
	if themes[0].Accent != lipgloss.Color("#FF00FF") {
		t.Errorf("partial theme accent = %v, want #FF00FF", themes[0].Accent)
	}
}

func TestUserThemesAppearInThemes(t *testing.T) {
	t.Cleanup(func() { SetUserThemes(nil) })
	SetUserThemes([]Theme{
		{Name: "ztheme", Accent: lipgloss.Color("#FF00FF")},
	})
	all := Themes()
	if len(all) != 4 {
		t.Fatalf("expected built-ins + 1 user theme = 4; got %d", len(all))
	}
	if all[3].Name != "ztheme" {
		t.Fatalf("user theme should appear after built-ins; got order %v",
			[]string{all[0].Name, all[1].Name, all[2].Name, all[3].Name})
	}
}
