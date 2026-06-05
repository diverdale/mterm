package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newCopyModeWithLines constructs a model directly from a line slice,
// bypassing the terminal-snapshot path. Lets unit tests pin a known
// content set without spinning up a real Terminal.
func newCopyModeWithLines(lines []string) *copyModeModel {
	m := &copyModeModel{mark: -1, cursor: -1}
	m.raw = append([]string(nil), lines...)
	m.plain = append([]string(nil), lines...)
	if len(m.plain) > 0 {
		m.cursor = trimTrailingBlanks(m.plain)
	}
	m.setSize(40, 10)
	return m
}

func TestCopyModeInitialCursorOnLastNonBlankLine(t *testing.T) {
	m := newCopyModeWithLines([]string{
		"first",
		"middle",
		"last",
		"",
		"",
	})
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 (last non-blank)", m.cursor)
	}
	if m.mark != -1 {
		t.Fatalf("mark = %d, want -1 (no anchor on entry)", m.mark)
	}
}

func TestCopyModeUpDownClamp(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c"})
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("after Up from 1, cursor = %d, want 0", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp}) // past top → clamp
	if m.cursor != 0 {
		t.Fatalf("after second Up, cursor = %d, want 0 (clamp)", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // past bottom → clamp
	if m.cursor != 2 {
		t.Fatalf("after 3x Down, cursor = %d, want 2 (clamp)", m.cursor)
	}
}

func TestCopyModeJKEquivalentToArrowKeys(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c"})
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.cursor != 0 {
		t.Fatalf("k didn't move cursor up: %d", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.cursor != 1 {
		t.Fatalf("j didn't move cursor down: %d", m.cursor)
	}
}

func TestCopyModeGAndShiftGJump(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c", "d", "e"})
	m.cursor = 2
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if m.cursor != 0 {
		t.Fatalf("g should jump to top, cursor = %d", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if m.cursor != 4 {
		t.Fatalf("G should jump to bottom, cursor = %d", m.cursor)
	}
}

func TestCopyModePageUpDownUsesViewportHeight(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c", "d", "e", "f", "g", "h"})
	m.setSize(40, 6) // pageSize() == 4
	m.cursor = 7
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.cursor != 3 {
		t.Fatalf("PgUp from 7 with page=4 → cursor = %d, want 3", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.cursor != 7 {
		t.Fatalf("PgDn back → cursor = %d, want 7", m.cursor)
	}
}

func TestCopyModeVTogglesMarkAtCursor(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c"})
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.mark != 1 {
		t.Fatalf("first v should pin mark at cursor (1), mark = %d", m.mark)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.mark != -1 {
		t.Fatalf("second v should drop mark, mark = %d", m.mark)
	}
}

func TestCopyModeSelectionRangeWithMark(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c", "d", "e"})
	m.cursor = 1
	m.mark = 3
	lo, hi := m.selectionRange()
	if lo != 1 || hi != 3 {
		t.Fatalf("selectionRange = (%d,%d), want (1,3)", lo, hi)
	}

	// Reversed (mark below cursor) still produces ordered range.
	m.cursor = 3
	m.mark = 1
	lo, hi = m.selectionRange()
	if lo != 1 || hi != 3 {
		t.Fatalf("reversed selectionRange = (%d,%d), want (1,3)", lo, hi)
	}
}

func TestCopyModeSelectionWithoutMarkIsJustCursor(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c"})
	m.cursor = 1
	lo, hi := m.selectionRange()
	if lo != 1 || hi != 1 {
		t.Fatalf("no-mark selectionRange = (%d,%d), want (1,1)", lo, hi)
	}
}

func TestCopyModeSelectionTextJoinsWithNewlinesAndTrimsTrailingSpaces(t *testing.T) {
	m := newCopyModeWithLines([]string{
		"first line   ",
		"middle  ",
		"last line",
	})
	m.cursor = 2
	m.mark = 0
	text, n := m.selectionText()
	if n != 3 {
		t.Fatalf("line count = %d, want 3", n)
	}
	want := "first line\nmiddle\nlast line"
	if text != want {
		t.Fatalf("selectionText = %q, want %q", text, want)
	}
}

func TestCopyModeEnterEmitsCopyMsg(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c"})
	m.cursor = 1
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return a cmd")
	}
	msg, ok := cmd().(copyModeCopyMsg)
	if !ok {
		t.Fatalf("Enter cmd() = %T, want copyModeCopyMsg", cmd())
	}
	if msg.Text != "b" || msg.Lines != 1 {
		t.Fatalf("copy msg = %+v, want {Text: \"b\", Lines: 1}", msg)
	}
}

func TestCopyModeYEquivalentToEnter(t *testing.T) {
	m := newCopyModeWithLines([]string{"a"})
	cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("y should return a cmd")
	}
	if _, ok := cmd().(copyModeCopyMsg); !ok {
		t.Fatalf("y cmd() = %T, want copyModeCopyMsg", cmd())
	}
}

func TestCopyModeQAndEscEmitClosed(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune("q")},
	} {
		m := newCopyModeWithLines([]string{"a"})
		cmd := m.Update(k)
		if cmd == nil {
			t.Fatalf("%v should return a cmd", k)
		}
		if _, ok := cmd().(copyModeClosedMsg); !ok {
			t.Fatalf("%v cmd() = %T, want copyModeClosedMsg", k, cmd())
		}
	}
}

func TestCopyModeViewIncludesStatusLine(t *testing.T) {
	m := newCopyModeWithLines([]string{"first", "second", "third"})
	m.setSize(40, 6)
	out := m.View()
	if !strings.Contains(out, "COPY") {
		t.Fatalf("View missing COPY badge: %q", out)
	}
	if !strings.Contains(out, "cursor only") {
		t.Fatalf("View should report cursor-only state when no mark: %q", out)
	}
}

func TestCopyModeViewReportsLineCountWhenMarked(t *testing.T) {
	m := newCopyModeWithLines([]string{"a", "b", "c", "d", "e"})
	m.setSize(40, 8)
	m.cursor = 0
	m.mark = 3
	out := m.View()
	if !strings.Contains(out, "4 lines") {
		t.Fatalf("View should show \"4 lines\" with mark+cursor 4 apart: %q", out)
	}
}

func TestCopyModeEmptySnapshotRendersStatusOnly(t *testing.T) {
	m := newCopyModeWithLines(nil)
	m.setSize(40, 4)
	out := m.View()
	if !strings.Contains(out, "COPY") {
		t.Fatalf("empty View should still show status: %q", out)
	}
}
