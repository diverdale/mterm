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

func TestPaletteWindowWhenListFits(t *testing.T) {
	// With a huge termH the window is the full list.
	start, end, more := paletteWindow(5, 2, 100)
	if start != 0 || end != 5 || more.above != 0 || more.below != 0 {
		t.Fatalf("fits: got start=%d end=%d more=%+v", start, end, more)
	}
	// termH=0 means "unconstrained" — same result.
	s2, e2, m2 := paletteWindow(5, 2, 0)
	if s2 != 0 || e2 != 5 || m2.above != 0 || m2.below != 0 {
		t.Fatalf("termH=0 should mean unconstrained; got start=%d end=%d more=%+v", s2, e2, m2)
	}
}

func TestPaletteWindowScrollsToKeepCursorVisible(t *testing.T) {
	// 50 items, terminal that fits ~3 rows after chrome. Cursor at index
	// 40 must be inside the returned [start, end) slice.
	const total = 50
	start, end, more := paletteWindow(total, 40, paletteChromeRows+3)
	if 40 < start || 40 >= end {
		t.Fatalf("cursor 40 not in window [%d, %d)", start, end)
	}
	if more.above+more.below+(end-start) != total {
		t.Fatalf("counters drift: above=%d below=%d window=%d total=%d",
			more.above, more.below, end-start, total)
	}
}

func TestPaletteWindowClampsAtTopAndBottom(t *testing.T) {
	const total = 50
	start, _, more := paletteWindow(total, 0, paletteChromeRows+5)
	if start != 0 || more.above != 0 {
		t.Fatalf("cursor at top: got start=%d above=%d", start, more.above)
	}
	_, end, more := paletteWindow(total, total-1, paletteChromeRows+5)
	if end != total || more.below != 0 {
		t.Fatalf("cursor at bottom: got end=%d below=%d", end, more.below)
	}
}

func TestPaletteWindowMinimumRowsOnTinyTerminal(t *testing.T) {
	// Even on an absurdly short terminal we should still see at least a
	// few items — never zero (otherwise the user can't see what's
	// selected).
	start, end, _ := paletteWindow(50, 25, 1)
	if end-start < 3 {
		t.Fatalf("expected at least 3 rows on tiny terminal; got %d", end-start)
	}
}
