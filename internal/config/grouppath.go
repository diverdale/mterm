package config

import "strings"

// GroupSegments splits a slash-delimited group path into clean segments.
// Leading, trailing, and interior empty segments are dropped, and each
// segment is trimmed of surrounding whitespace. "Home/Media" → ["Home",
// "Media"]; "/Home//Media/" → ["Home", "Media"]; "" → nil.
func GroupSegments(group string) []string {
	if group == "" {
		return nil
	}
	parts := strings.Split(group, "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// CommonPrefixLen returns the count of matching leading segments between two
// group paths. Used by the picker to know which sub-group headers stay vs
// need re-emitting on transition between hosts.
func CommonPrefixLen(a, b []string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
