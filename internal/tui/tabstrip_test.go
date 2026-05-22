package tui

import (
	"strings"
	"testing"

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
