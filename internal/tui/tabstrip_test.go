package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"mterm/internal/config"
)

func TestTabStripShowsAllTabs(t *testing.T) {
	tabs := []*sessionTab{
		newSessionTab(1, config.Host{Name: "prod-web"}, 80, 24),
		newSessionTab(2, config.Host{Name: "prod-db"}, 80, 24),
	}
	out := renderTabStrip(tabs, 0, 0, 80)
	if !strings.Contains(out, "prod-web") || !strings.Contains(out, "prod-db") {
		t.Fatalf("tab strip missing a host name:\n%s", out)
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Fatalf("tab strip missing tab numbers:\n%s", out)
	}
}

func TestTabStripIsOneLine(t *testing.T) {
	tabs := []*sessionTab{newSessionTab(1, config.Host{Name: "h"}, 80, 24)}
	if strings.Contains(renderTabStrip(tabs, 0, 0, 80), "\n") {
		t.Fatal("tab strip must be a single line")
	}
}

func TestTabStripEmptyTabs(t *testing.T) {
	if got := renderTabStrip(nil, 0, 0, 80); strings.Contains(got, "\n") {
		t.Fatal("empty tab strip must still be a single line")
	}
}

func TestTabStripTruncatesOverflow(t *testing.T) {
	var tabs []*sessionTab
	for i := 0; i < 8; i++ {
		tabs = append(tabs, newSessionTab(i+1, config.Host{Name: "longhostname-prod"}, 80, 24))
	}
	out := renderTabStrip(tabs, 0, 0, 40)
	if strings.Contains(out, "\n") {
		t.Fatalf("an overflowing tab strip must stay a single line, got:\n%s", out)
	}
	if w := lipgloss.Width(out); w > 40 {
		t.Fatalf("tab strip width = %d, want <= 40", w)
	}
}

func TestTabStripStylesTabs(t *testing.T) {
	tabs := []*sessionTab{newSessionTab(1, config.Host{Name: "h"}, 80, 24)}
	out := renderTabStrip(tabs, 0, 0, 80)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("tab strip must be styled (ANSI escape codes), got: %q", out)
	}
}

func TestTabStripActiveTabBracketed(t *testing.T) {
	tabs := []*sessionTab{
		newSessionTab(1, config.Host{Name: "alpha"}, 80, 24),
		newSessionTab(2, config.Host{Name: "beta"}, 80, 24),
	}
	out := renderTabStrip(tabs, 0, 0, 80)
	// The active tab is outlined with brackets.
	if !strings.Contains(out, "[ ") || !strings.Contains(out, " ]") {
		t.Fatalf("active tab should be bracketed, got:\n%s", out)
	}
}
