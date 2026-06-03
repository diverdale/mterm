package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

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

// userThemes holds themes loaded from ~/.config/<app>/themes.yaml.
// Mutated by SetUserThemes at startup and by reloadHosts on demand.
// Reads happen on the main goroutine; no synchronization needed.
var userThemes []Theme

// SetUserThemes replaces the user-defined theme slice. Pass the output of
// LoadUserThemes here at startup. nil clears any prior user themes — the
// built-in trio (Midnight/Matrix/Synthwave) always remains available.
func SetUserThemes(t []Theme) { userThemes = t }

// Themes returns built-ins first, then user-defined themes in alphabetical
// order so the palette listing is stable across runs.
func Themes() []Theme {
	out := []Theme{Midnight, Matrix, Synthwave}
	out = append(out, userThemes...)
	return out
}

// setTheme switches the active theme and rebuilds the chrome style set.
// Subsequent renders use the new colors on the next render tick.
func setTheme(t Theme) {
	active = t
	sty = buildStyles(active)
}

// themeYAML is the on-disk shape of a single user theme. Every field is
// optional; missing colors fall back to the corresponding Midnight value
// so partial themes ("just override the accent") work cleanly.
type themeYAML struct {
	Accent    string `yaml:"accent"`
	Frame     string `yaml:"frame"`
	Dim       string `yaml:"dim"`
	Text      string `yaml:"text"`
	Warning   string `yaml:"warning"`
	Error     string `yaml:"error"`
	Connected string `yaml:"connected"`
}

// LoadUserThemes parses themes.yaml at path and returns the themes in
// alphabetical order. Missing file is fine — returns (nil, nil). A parse
// error returns an empty slice plus the error so callers can warn and
// keep going with built-ins.
//
// Format: top-level map of theme-name → field map.
//
//	mytheme:
//	  accent: "#3344FF"
//	  frame: "#1A1A2E"
//	  ...
//
// Unset fields inherit Midnight's value, so users can ship "just darker
// frame, everything else default" without copying the whole palette.
func LoadUserThemes(path string) ([]Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var raw map[string]themeYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("themes.yaml: %w", err)
	}
	names := make([]string, 0, len(raw))
	for k := range raw {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Theme, 0, len(names))
	for _, name := range names {
		t := raw[name]
		out = append(out, Theme{
			Name:      strings.ToLower(strings.TrimSpace(name)),
			Accent:    colorOr(t.Accent, Midnight.Accent),
			Frame:     colorOr(t.Frame, Midnight.Frame),
			Dim:       colorOr(t.Dim, Midnight.Dim),
			Text:      colorOr(t.Text, Midnight.Text),
			Warning:   colorOr(t.Warning, Midnight.Warning),
			Error:     colorOr(t.Error, Midnight.Error),
			Connected: colorOr(t.Connected, Midnight.Connected),
		})
	}
	return out, nil
}

func colorOr(hex string, fallback lipgloss.Color) lipgloss.Color {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return fallback
	}
	return lipgloss.Color(hex)
}
