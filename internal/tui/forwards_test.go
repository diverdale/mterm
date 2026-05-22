package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
)

func TestForwardsPanelListsForwards(t *testing.T) {
	host := config.Host{
		Name: "h",
		Forwards: []config.Forward{
			{Type: config.ForwardLocal, BindPort: 8080, DialAddr: "127.0.0.1", DialPort: 80},
			{Type: config.ForwardRemote, BindPort: 9000, DialAddr: "127.0.0.1", DialPort: 3000},
		},
	}
	panel := newForwardsPanel(host)
	view := panel.View()
	if !strings.Contains(view, "8080") || !strings.Contains(view, "9000") {
		t.Fatalf("panel must list both forwards, got:\n%s", view)
	}
	if !strings.Contains(view, "local") || !strings.Contains(view, "remote") {
		t.Fatalf("panel must show forward types, got:\n%s", view)
	}
}

func TestForwardsPanelToggle(t *testing.T) {
	host := config.Host{
		Name:     "h",
		Forwards: []config.Forward{{Type: config.ForwardLocal, BindPort: 8080}},
	}
	panel := newForwardsPanel(host)
	if panel.enabled(0) {
		t.Fatal("forward should start disabled")
	}
	panel.toggle(0)
	if !panel.enabled(0) {
		t.Fatal("toggle must enable the forward")
	}
	panel.toggle(0)
	if panel.enabled(0) {
		t.Fatal("toggle must disable the forward")
	}
}

func TestForwardsPanelEscEmitsClose(t *testing.T) {
	panel := newForwardsPanel(config.Host{Name: "h"})
	cmd := panel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Update(Esc) must return a non-nil Cmd")
	}
	msg := cmd()
	if _, ok := msg.(forwardsClosedMsg); !ok {
		t.Fatalf("Esc cmd must emit forwardsClosedMsg, got %T", msg)
	}
}

func TestForwardsPanelViewEmptyDoesNotPanic(t *testing.T) {
	panel := newForwardsPanel(config.Host{Name: "h"})
	view := panel.View()
	if view == "" {
		t.Fatal("View() must return a non-empty string for a host with no forwards")
	}
}

func TestForwardsPanelIsThemed(t *testing.T) {
	host := config.Host{
		Name:     "h",
		Forwards: []config.Forward{{Type: config.ForwardLocal, BindPort: 8080}},
	}
	panel := newForwardsPanel(host)
	out := panel.View()
	// Themed output carries ANSI escape codes.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("forwards panel has no styling:\n%s", out)
	}
	// Still shows the rounded modal border.
	if !strings.Contains(out, "╭") && !strings.Contains(out, "─") {
		t.Fatalf("forwards panel lost its border:\n%s", out)
	}
}

func TestForwardsPanelUpdateCursorAndToggle(t *testing.T) {
	host := config.Host{
		Name: "h",
		Forwards: []config.Forward{
			{Type: config.ForwardLocal, BindPort: 8080, DialAddr: "127.0.0.1", DialPort: 80},
			{Type: config.ForwardRemote, BindPort: 9000, DialAddr: "127.0.0.1", DialPort: 3000},
		},
	}
	panel := newForwardsPanel(host)

	// Move cursor down to index 1.
	panel.Update(tea.KeyMsg{Type: tea.KeyDown})
	if panel.cursor != 1 {
		t.Fatalf("expected cursor 1 after first KeyDown, got %d", panel.cursor)
	}

	// Second KeyDown must clamp at 1 (only two forwards, max index is 1).
	panel.Update(tea.KeyMsg{Type: tea.KeyDown})
	if panel.cursor != 1 {
		t.Fatalf("expected cursor to clamp at 1, got %d", panel.cursor)
	}

	// Toggle the forward at cursor (index 1).
	panel.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !panel.enabled(1) {
		t.Fatal("forward at index 1 should be enabled after toggle")
	}
	if panel.enabled(0) {
		t.Fatal("forward at index 0 should still be disabled")
	}

	// Move cursor back up to index 0.
	panel.Update(tea.KeyMsg{Type: tea.KeyUp})
	if panel.cursor != 0 {
		t.Fatalf("expected cursor 0 after KeyUp, got %d", panel.cursor)
	}

	// Second KeyUp must clamp at 0.
	panel.Update(tea.KeyMsg{Type: tea.KeyUp})
	if panel.cursor != 0 {
		t.Fatalf("expected cursor to clamp at 0, got %d", panel.cursor)
	}
}
