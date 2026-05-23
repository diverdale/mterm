package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleCommands() []command {
	return []command{
		{label: "Foo bar", group: "G1", action: func(*App) tea.Cmd { return nil }},
		{label: "Baz qux", group: "G2", action: func(*App) tea.Cmd { return nil }},
		{label: "Foo other", group: "G1", action: func(*App) tea.Cmd { return nil }},
	}
}

func TestPaletteFilterCaseInsensitive(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.setQuery("FOO")
	v := p.visible()
	if len(v) != 2 {
		t.Fatalf("filter 'FOO' = %d, want 2", len(v))
	}
}

func TestPaletteEnterEmitsChosen(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter must return a non-nil cmd")
	}
	msg, ok := cmd().(paletteChosenMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want paletteChosenMsg", cmd())
	}
	if msg.cmd.label != "Foo bar" {
		t.Fatalf("chosen.label = %q, want Foo bar", msg.cmd.label)
	}
}

func TestPaletteEscEmitsClosed(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc must return a non-nil cmd")
	}
	if _, ok := cmd().(paletteClosedMsg); !ok {
		t.Fatalf("cmd() = %T, want paletteClosedMsg", cmd())
	}
}

func TestPaletteCursorWrapsAround(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.move(-1)
	if p.cursor != 2 {
		t.Fatalf("after move(-1) cursor = %d, want 2", p.cursor)
	}
	p.move(1)
	if p.cursor != 0 {
		t.Fatalf("after move(1) wrap cursor = %d, want 0", p.cursor)
	}
}

func TestPaletteEnterWithNoVisibleNoOps(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.setQuery("zzznomatch")
	if cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatalf("Enter with no visible cmds returned non-nil: %T", cmd())
	}
}
