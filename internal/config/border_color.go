package config

import "errors"

var errBadBorderColor = errors.New("want #RRGGBB or #RGB")

// parseBorderColor validates a host borderColor value. Empty input is allowed
// and returns ("", nil) — the host has no override. A non-empty value must be
// a 7-char "#RRGGBB" or 4-char "#RGB" hex string; anything else returns an
// error and an empty result so callers can warn and continue.
func parseBorderColor(s string) (string, error) {
	if s == "" {
		return "", nil
	}
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
