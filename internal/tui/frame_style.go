package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// frameChars is the full set of border glyphs the chrome uses. Six corners
// (top-left/right, bottom-left/right, divider-tee-left/right), one
// horizontal, one vertical. Picked together for consistency so we never
// accidentally mix a rounded corner with a square edge.
type frameChars struct {
	TopLeft, TopRight       string
	BottomLeft, BottomRight string
	TeeLeft, TeeRight       string // dividers between tab strip / body / footer
	Horizontal, Vertical    string
}

// frameStylePresets holds the named frame styles. Map lookup is
// case-insensitive — see resolveFrameStyle.
var frameStylePresets = map[string]frameChars{
	"rounded": {
		TopLeft: "╭", TopRight: "╮",
		BottomLeft: "╰", BottomRight: "╯",
		TeeLeft: "├", TeeRight: "┤",
		Horizontal: "─", Vertical: "│",
	},
	"square": {
		TopLeft: "┌", TopRight: "┐",
		BottomLeft: "└", BottomRight: "┘",
		TeeLeft: "├", TeeRight: "┤",
		Horizontal: "─", Vertical: "│",
	},
	"thick": {
		TopLeft: "┏", TopRight: "┓",
		BottomLeft: "┗", BottomRight: "┛",
		TeeLeft: "┣", TeeRight: "┫",
		Horizontal: "━", Vertical: "┃",
	},
	"double": {
		TopLeft: "╔", TopRight: "╗",
		BottomLeft: "╚", BottomRight: "╝",
		TeeLeft: "╠", TeeRight: "╣",
		Horizontal: "═", Vertical: "║",
	},
	"ascii": {
		TopLeft: "+", TopRight: "+",
		BottomLeft: "+", BottomRight: "+",
		TeeLeft: "+", TeeRight: "+",
		Horizontal: "-", Vertical: "|",
	},
	"minimal": {
		// Hidden — just vertical bars on the sides, blank elsewhere. The
		// title row is a horizontal under the heading, tab/footer
		// dividers vanish. Maximally quiet chrome.
		TopLeft: " ", TopRight: " ",
		BottomLeft: " ", BottomRight: " ",
		TeeLeft: " ", TeeRight: " ",
		Horizontal: " ", Vertical: " ",
	},
}

// FrameStyleNames returns the preset names in canonical order for
// listing / palette purposes.
func FrameStyleNames() []string {
	return []string{"square", "rounded", "thick", "double", "ascii", "minimal"}
}

// frame is the active glyph set. Reads happen on the main goroutine
// (chrome rendering), writes only at startup or via the palette switcher.
// Default is "rounded" — matches the pre-customization chrome look.
var frame = frameStylePresets["rounded"]

// SetFrameStyle switches the active chrome glyph set. Unknown name is a
// no-op so a typo in settings.yaml doesn't blank the frame. Returns true
// when the name resolved, false when it didn't.
func SetFrameStyle(name string) bool {
	if fc, ok := resolveFrameStyle(name); ok {
		frame = fc
		return true
	}
	return false
}

func resolveFrameStyle(name string) (frameChars, bool) {
	fc, ok := frameStylePresets[strings.ToLower(strings.TrimSpace(name))]
	return fc, ok
}

// settingsYAML is the on-disk shape of ~/.config/<app>/settings.yaml.
// Every field is optional. New global toggles land here in future cycles
// (confirm_quit, default theme, etc.).
type settingsYAML struct {
	Frame string `yaml:"frame"`
}

// LoadSettings reads settings.yaml at path and applies any global
// toggles it contains (currently just frame style). Missing file is
// fine — returns nil. Parse error returns the error so the caller can
// warn and proceed with defaults.
//
// Today's surface is small (frame) but the signature is deliberately a
// one-call "apply everything" so adding `confirm_quit: false` later
// doesn't touch every caller again.
func LoadSettings(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var s settingsYAML
	if err := yaml.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("settings.yaml: %w", err)
	}
	if s.Frame != "" {
		if !SetFrameStyle(s.Frame) {
			return fmt.Errorf("settings.yaml: unknown frame style %q (want one of %v)",
				s.Frame, FrameStyleNames())
		}
	}
	return nil
}
