package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ResolveProxyJump turns a `proxy_jump:` value into a Host suitable for
// dialing as a jump server.
//
// Resolution rules:
//
//  1. If value matches the Name of an existing host in hosts, return a
//     copy of that host. The rich config (identityfile, port, etc.) is
//     reused — the user already taught mterm everything it needs to
//     connect there.
//  2. Otherwise, parse value as a literal in the form
//     `[user@]hostname[:port]`. Port defaults to 22; user defaults to
//     empty (the caller picks the OS user at connect time).
//
// Empty input returns (nil, nil) — no proxy is a valid state.
func ResolveProxyJump(value string, hosts []Host) (*Host, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	for _, h := range hosts {
		if h.Name == value {
			cp := h
			return &cp, nil
		}
	}
	return parseProxyJumpLiteral(value)
}

// parseProxyJumpLiteral parses `[user@]hostname[:port]` and returns a
// synthesized Host. The Host's Name mirrors the literal so display code
// has something to render.
func parseProxyJumpLiteral(value string) (*Host, error) {
	user := ""
	rest := value
	if idx := strings.LastIndex(value, "@"); idx >= 0 {
		user = value[:idx]
		rest = value[idx+1:]
		if user == "" {
			return nil, fmt.Errorf("proxy_jump %q: empty user before @", value)
		}
	}
	host := rest
	port := 22
	if idx := strings.LastIndex(rest, ":"); idx >= 0 {
		host = rest[:idx]
		portStr := rest[idx+1:]
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("proxy_jump %q: bad port %q: %w", value, portStr, err)
		}
		if p <= 0 || p > 65535 {
			return nil, fmt.Errorf("proxy_jump %q: port %d out of range", value, p)
		}
		port = p
	}
	if host == "" {
		return nil, fmt.Errorf("proxy_jump %q: empty hostname", value)
	}
	return &Host{
		Name:     value, // surface the literal in any user-facing rendering
		HostName: host,
		User:     user,
		Port:     port,
		Source:   SourceMterm,
	}, nil
}
