package tui

import (
	"testing"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
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
	// Body is the inner window area: width minus side borders, height minus
	// the 5 chrome rows (top border, tab strip, divider, footer, bottom border).
	if w != 100-chromeCols || h != 30-chromeRows {
		t.Fatalf("terminal size = %d,%d want %d,%d", w, h, 100-chromeCols, 30-chromeRows)
	}
}

func TestSessionTabStatus(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	// No session attached yet → connecting.
	if got := tab.status(); got != statusConnecting {
		t.Fatalf("status with no session = %v, want statusConnecting", got)
	}
	// After the reader goroutine has marked the tab ended → failed.
	tab.ended.Store(true)
	if got := tab.status(); got != statusFailed {
		t.Fatalf("status after ended = %v, want statusFailed", got)
	}

	// A live (non-nil) session → connected.
	tabC := newSessionTab(2, config.Host{Name: "h"}, 80, 24)
	var liveSess mssh.Session // zero value; status() only nil-checks the pointer
	tabC.sess.Store(&liveSess)
	if got := tabC.status(); got != statusConnected {
		t.Fatalf("status with live session = %v, want statusConnected", got)
	}

	// ended==true while sess is still set (the post-readLoop, pre-reap window)
	// must report failed — ended is absorbing.
	tabE := newSessionTab(3, config.Host{Name: "h"}, 80, 24)
	tabE.sess.Store(&liveSess)
	tabE.ended.Store(true)
	if got := tabE.status(); got != statusFailed {
		t.Fatalf("status ended+sess-set = %v, want statusFailed", got)
	}
}

func TestSessionTabViewRendersTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.term.Write([]byte("session-output"))
	if got := tab.View(); got == "" {
		t.Fatal("View() returned empty")
	}
}

func TestSessionTabNilSessionIsSafe(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	// No session attached — these must all be safe no-ops.
	tab.sendInput([]byte("hello"))
	tab.resize(50, 20)
	tab.close()
	// close() on a never-attached tab must also be safe.
	tab.close()
}
