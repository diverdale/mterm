package config

import (
	"errors"
	"fmt"
	"strings"
)

var errBadBorderColor = errors.New("want #RRGGBB, #RGB, or a known color name")

// parseBorderColor validates a host bordercolor value. Empty input is
// allowed and returns ("", nil). Hex (#RRGGBB / #RGB) is accepted as-is.
// Non-hex strings are resolved through the colors map (built-in palette +
// user overlay); unknown names return a clear error so the caller can warn
// and continue.
func parseBorderColor(s string, colors map[string]string) (string, error) {
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, "#") {
		return parseHexColor(s)
	}
	if hex, ok := colors[strings.ToLower(strings.TrimSpace(s))]; ok {
		return parseHexColor(hex)
	}
	return "", fmt.Errorf("unknown color name %q (and not hex)", s)
}

func parseHexColor(s string) (string, error) {
	if len(s) != 7 && len(s) != 4 {
		return "", errBadBorderColor
	}
	if s[0] != '#' {
		return "", errBadBorderColor
	}
	for _, c := range s[1:] {
		if !isHexDigit(c) {
			return "", errBadBorderColor
		}
	}
	return s, nil
}

func isHexDigit(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}
