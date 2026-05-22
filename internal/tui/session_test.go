package tui

import (
	"testing"

	"mterm/internal/config"
)

func TestSessionTabTitleUsesHostName(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "prod-web"}, 80, 24)
	if tab.title() != "prod-web" {
		t.Fatalf("title() = %q, want prod-web", tab.title())
	}
	if tab.id != 1 {
		t.Fatalf("id = %d, want 1", tab.id)
	}
}

func TestSessionTabResizeUpdatesTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.resize(100, 30)
	w, h := tab.term.Size()
	// Terminal height is the tab body: total rows minus the tab bar row.
	if w != 100 || h != 29 {
		t.Fatalf("terminal size = %d,%d want 100,29", w, h)
	}
}

func TestSessionTabViewRendersTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.term.Write([]byte("session-output"))
	if got := tab.View(); got == "" {
		t.Fatal("View() returned empty")
	}
}
