package tui

import (
	"strings"
	"testing"
)

func sessionHints() []keyHint {
	return []keyHint{{"^B", "menu"}, {"^B n", "next"}, {"^B x", "close"}}
}

func TestFooterShowsHintsAndInfo(t *testing.T) {
	out := renderFooter(footerOpts{
		hints: sessionHints(),
		info:  "dale@host:22",
		width: 80,
	}, sty)
	if strings.Contains(out, "\n") {
		t.Fatal("footer must be a single line")
	}
	if !strings.Contains(out, "menu") || !strings.Contains(out, "next") {
		t.Fatalf("footer missing hints:\n%s", out)
	}
	if !strings.Contains(out, "dale@host:22") {
		t.Fatalf("footer missing info:\n%s", out)
	}
}

func TestFooterPrefixBadge(t *testing.T) {
	out := renderFooter(footerOpts{
		hints:         sessionHints(),
		prefixPending: true,
		width:         80,
	}, sty)
	if !strings.Contains(out, "PREFIX") {
		t.Fatalf("prefix-pending footer must show the PREFIX badge:\n%s", out)
	}
}

func TestFooterDropsInfoWhenNarrow(t *testing.T) {
	// A very narrow width must still produce one line and keep the hints.
	out := renderFooter(footerOpts{
		hints: sessionHints(),
		info:  "dale@averylonghostname.example.com:22",
		width: 24,
	}, sty)
	if strings.Contains(out, "\n") {
		t.Fatal("narrow footer must still be one line")
	}
}
