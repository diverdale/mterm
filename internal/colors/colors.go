// Package colors resolves named colors (like "limegreen" or "prodred") into
// the hex strings the rest of the app expects. Ships with a small built-in
// CSS-ish palette and merges in any names the user adds via
// ~/.config/<app>/colors.yaml.
package colors

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Builtins returns the names mterm ships with. Returns a fresh map so the
// caller can freely modify it.
func Builtins() map[string]string {
	return map[string]string{
		// Reds / pinks
		"red":     "#FF3344",
		"darkred": "#8B0000",
		"crimson": "#DC143C",
		"hotpink": "#FF69B4",
		"pink":    "#FFB6C1",
		// Oranges / yellows
		"orange": "#FF8C00",
		"gold":   "#FFD700",
		"yellow": "#FFD93D",
		"amber":  "#FFB347",
		// Greens
		"green":     "#33CC66",
		"limegreen": "#32CD32",
		"darkgreen": "#006400",
		"olive":     "#808000",
		"teal":      "#008080",
		// Blues / cyans
		"blue": "#3344FF",
		"navy": "#000080",
		"sky":  "#87CEEB",
		"cyan": "#00CCDD",
		// Purples
		"purple":  "#8A2BE2",
		"magenta": "#F92AAD",
		"violet":  "#8B5CF6",
		// Neutrals
		"white":  "#FFFFFF",
		"gray":   "#808080",
		"grey":   "#808080",
		"black":  "#000000",
		"silver": "#C0C0C0",
		// Common branding-ish picks
		"prodred":     "#CC0000",
		"stageyellow": "#E5C07B",
		"devgreen":    "#33CC66",
	}
}

// Open loads colors from path and merges them on top of Builtins. Missing
// file is not an error — Builtins still wins. User entries override
// built-ins by name (case-insensitive lookup; storage is lowercased).
func Open(path string) (map[string]string, error) {
	out := Builtins()
	// Lowercase the built-in keys so user lookups are case-insensitive
	// without per-call ToLower.
	lc := make(map[string]string, len(out))
	for k, v := range out {
		lc[strings.ToLower(k)] = v
	}
	out = lc

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	if len(data) == 0 {
		return out, nil
	}
	user := map[string]string{}
	if err := yaml.Unmarshal(data, &user); err != nil {
		return out, err
	}
	for k, v := range user {
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out, nil
}

// Resolve looks up name in colors and returns the hex value. Case
// insensitive. Returns ok=false when the name is unknown.
func Resolve(colors map[string]string, name string) (string, bool) {
	if colors == nil {
		return "", false
	}
	v, ok := colors[strings.ToLower(strings.TrimSpace(name))]
	return v, ok
}
