package tui

import (
	"strings"
	"testing"

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
