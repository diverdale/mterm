package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mterm/internal/config"
)

func sampleHosts() []config.Host {
	return []config.Host{
		{Name: "prod-web", HostName: "10.0.1.4", Group: "production", Tags: []string{"web"}},
		{Name: "prod-db", HostName: "10.0.2.9", Group: "production"},
		{Name: "lab-box", HostName: "10.9.0.12", Group: "lab"},
	}
}

func TestPickerFiltersByQuery(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("lab")

	visible := p.visibleHosts()
	if len(visible) != 1 || visible[0].Name != "lab-box" {
		t.Fatalf("filter 'lab' = %v, want [lab-box]", names(visible))
	}
}

func TestPickerFilterIsCaseInsensitive(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("PROD")
	if got := len(p.visibleHosts()); got != 2 {
		t.Fatalf("filter 'PROD' matched %d hosts, want 2", got)
	}
}

func TestPickerSelectionReturnsHost(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	p.setQuery("lab")
	h, ok := p.selected()
	if !ok || h.Name != "lab-box" {
		t.Fatalf("selected() = %v,%v want lab-box,true", h.Name, ok)
	}
}

func names(hs []config.Host) []string {
	out := []string{}
	for _, h := range hs {
		out = append(out, h.Name)
	}
	return out
}

func TestPickerCursorWrapsAround(t *testing.T) {
	p := newPicker(sampleHosts())
	// cursor starts at 0 (prod-web)
	p.moveCursor(-1)
	h, ok := p.selected()
	if !ok || h.Name != "lab-box" {
		t.Fatalf("moveCursor(-1) from 0 = %v,%v; want lab-box,true", h.Name, ok)
	}
	p.moveCursor(1)
	h, ok = p.selected()
	if !ok || h.Name != "prod-web" {
		t.Fatalf("moveCursor(1) from last = %v,%v; want prod-web,true", h.Name, ok)
	}
}

func TestPickerUpdateKeyDispatch(t *testing.T) {
	p := newPicker(sampleHosts())

	// KeyDown moves cursor to second host
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	h, ok := p.selected()
	if !ok || h.Name != "prod-db" {
		t.Fatalf("after KeyDown selected = %v,%v; want prod-db,true", h.Name, ok)
	}

	// KeyEnter returns a command that emits pickerChosenMsg
	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("KeyEnter returned nil cmd, want non-nil")
	}
	msg := cmd()
	chosen, ok := msg.(pickerChosenMsg)
	if !ok {
		t.Fatalf("cmd() type = %T; want pickerChosenMsg", msg)
	}
	if chosen.host.Name != "prod-db" {
		t.Fatalf("pickerChosenMsg.host = %v; want prod-db", chosen.host.Name)
	}

	// KeyEsc returns a command that emits pickerCancelledMsg
	cmd = p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("KeyEsc returned nil cmd, want non-nil")
	}
	msg = cmd()
	if _, ok := msg.(pickerCancelledMsg); !ok {
		t.Fatalf("cmd() type = %T; want pickerCancelledMsg", msg)
	}
}

func TestPickerViewEmptyDoesNotPanic(t *testing.T) {
	// nil host list
	p := newPicker(nil)
	s := p.View()
	if s == "" {
		t.Fatal("View() with nil hosts returned empty string")
	}

	// non-nil hosts but query filters all out
	p2 := newPicker(sampleHosts())
	p2.setQuery("zzzznomatch")
	s2 := p2.View()
	if s2 == "" {
		t.Fatal("View() with no-match query returned empty string")
	}
	if len(p2.visibleHosts()) != 0 {
		t.Fatal("visibleHosts() with no-match query should be empty")
	}
}

func TestPickerSelectionBarIsFullWidth(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()
	// The cursor starts at row 0; find the rendered line for the first host
	// and confirm it spans the full picker width.
	h0 := p.visibleHosts()[0]
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, h0.Name) {
			if w := lipgloss.Width(ln); w != p.w {
				t.Fatalf("selection bar width = %d, want %d (full width)", w, p.w)
			}
			return
		}
	}
	t.Fatal("could not find the selected host row in the picker view")
}

func TestPickerViewHasSelectionAndGroups(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()
	// Group headers from sampleHosts: "production" and "lab".
	if !strings.Contains(out, "production") || !strings.Contains(out, "lab") {
		t.Fatalf("picker body missing group headers:\n%s", out)
	}
	// The selected host's row must carry styling (ANSI escape codes), since
	// the cursor row is a selection bar.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("picker body has no styling at all:\n%s", out)
	}
}
