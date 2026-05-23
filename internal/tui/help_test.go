package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpViewListsAllPrefixKeys(t *testing.T) {
	h := newHelpModel()
	v := h.View()
	for _, want := range []string{"^B c", "^B n", "^B p", "^B x", "^B f", "^B ?", "^B q", "1..9"} {
		if !strings.Contains(v, want) {
			t.Fatalf("help missing %q in view:\n%s", want, v)
		}
	}
}

func TestHelpUpdateDismissesOnAnyKey(t *testing.T) {
	h := newHelpModel()
	cmd := h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("help Update should return a non-nil cmd")
	}
	if _, ok := cmd().(helpClosedMsg); !ok {
		t.Fatalf("cmd() = %T, want helpClosedMsg", cmd())
	}
}
