package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"mterm/internal/terminal"
)

// copyModeCopyMsg is emitted when the user confirms a selection with
// Enter / y. The App handles it (writes to system clipboard via the
// clipboard package + closes the overlay). Surfacing this as a message
// rather than calling the clipboard package directly from the model
// keeps the model side-effect-free and unit-testable without mocking.
type copyModeCopyMsg struct {
	Text  string // plain text, ANSI stripped, lines joined by \n
	Lines int    // number of source lines that contributed
}

// copyModeClosedMsg dismisses the overlay without copying.
type copyModeClosedMsg struct{}

// copyModeModel is the keyboard-driven scrollback selection overlay.
// Activated by ^B [, it takes a frozen snapshot of the active session's
// scrollback + live screen so the view doesn't shift under the user as
// new output streams in. Line-range selection only in v1.
type copyModeModel struct {
	// raw is the per-line snapshot with ANSI sequences preserved (for
	// rendering); plain is the ANSI-stripped form used both for clipboard
	// payload and for the reverse-video selection highlight (so explicit
	// colors in the source don't override the highlight).
	raw   []string
	plain []string

	cursor int // absolute index into raw / plain. -1 when no lines.
	mark   int // anchor index or -1 (cursor-only mode).

	// View dimensions. Set by the App at View time so the body matches
	// the session's body size.
	termW, termH int
}

// newCopyModeModel snapshots the given terminal and returns a model
// positioned at the bottom of available content (most recent line).
// Safe to call with a nil terminal — yields an empty model that draws
// a "no content yet" placeholder.
func newCopyModeModel(term *terminal.Terminal) *copyModeModel {
	m := &copyModeModel{mark: -1, cursor: -1}
	if term == nil {
		return m
	}
	m.raw = term.Snapshot()
	m.plain = make([]string, len(m.raw))
	for i, ln := range m.raw {
		m.plain[i] = ansi.Strip(ln)
	}
	if len(m.raw) > 0 {
		m.cursor = trimTrailingBlanks(m.plain)
	}
	return m
}

// trimTrailingBlanks returns the index of the last non-blank line, or 0
// if every line is blank. Lets the cursor start on the last line with
// content rather than down in the empty rows of the live screen.
func trimTrailingBlanks(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i
		}
	}
	return 0
}

func (m *copyModeModel) setSize(w, h int) { m.termW, m.termH = w, h }

// Update handles a key press. Returns a tea.Cmd when the press emits a
// message (Enter → copyModeCopyMsg, q/Esc → copyModeClosedMsg).
func (m *copyModeModel) Update(k tea.KeyMsg) tea.Cmd {
	switch k.Type {
	case tea.KeyEsc:
		return func() tea.Msg { return copyModeClosedMsg{} }
	case tea.KeyEnter:
		text, n := m.selectionText()
		return func() tea.Msg { return copyModeCopyMsg{Text: text, Lines: n} }
	case tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyDown:
		m.moveCursor(1)
	case tea.KeyPgUp, tea.KeyCtrlB:
		m.moveCursor(-m.pageSize())
	case tea.KeyPgDown, tea.KeyCtrlF:
		m.moveCursor(m.pageSize())
	case tea.KeySpace:
		m.toggleMark()
	case tea.KeyRunes:
		if len(k.Runes) != 1 {
			return nil
		}
		switch k.Runes[0] {
		case 'q':
			return func() tea.Msg { return copyModeClosedMsg{} }
		case 'y':
			text, n := m.selectionText()
			return func() tea.Msg { return copyModeCopyMsg{Text: text, Lines: n} }
		case 'k':
			m.moveCursor(-1)
		case 'j':
			m.moveCursor(1)
		case 'g':
			if len(m.plain) > 0 {
				m.cursor = 0
			}
		case 'G':
			if len(m.plain) > 0 {
				m.cursor = len(m.plain) - 1
			}
		case 'v':
			m.toggleMark()
		}
	}
	return nil
}

// toggleMark pins / unpins the selection anchor at the current cursor.
// Matches tmux's v: first press starts a visual range, second press
// cancels it.
func (m *copyModeModel) toggleMark() {
	if m.cursor < 0 {
		return
	}
	if m.mark == -1 {
		m.mark = m.cursor
	} else {
		m.mark = -1
	}
}

func (m *copyModeModel) moveCursor(delta int) {
	if m.cursor < 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.plain) {
		m.cursor = len(m.plain) - 1
	}
}

func (m *copyModeModel) pageSize() int {
	if m.termH > 2 {
		return m.termH - 2
	}
	return 1
}

// selectionRange returns the inclusive [lo, hi] index pair that the
// current mark + cursor cover. When mark is unset, the range is just
// the cursor line.
func (m *copyModeModel) selectionRange() (lo, hi int) {
	if m.cursor < 0 {
		return 0, -1
	}
	if m.mark < 0 {
		return m.cursor, m.cursor
	}
	if m.mark < m.cursor {
		return m.mark, m.cursor
	}
	return m.cursor, m.mark
}

// selectionText returns the plain-text payload that would land on the
// clipboard for the current selection, plus the line count.
func (m *copyModeModel) selectionText() (string, int) {
	lo, hi := m.selectionRange()
	if hi < lo {
		return "", 0
	}
	lines := m.plain[lo : hi+1]
	// Trim trailing whitespace from each line — terminal grids pad rows
	// out to width, which pastes as ugly trailing spaces.
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = strings.TrimRight(ln, " \t")
	}
	return strings.Join(out, "\n"), len(out)
}

// windowTop computes the top-of-view line index so the cursor stays
// visible. Renders the cursor centered when possible; pins to ends when
// it can't.
func (m *copyModeModel) windowTop(h int) int {
	if len(m.plain) <= h || m.cursor < 0 {
		return 0
	}
	half := h / 2
	top := m.cursor - half
	if top < 0 {
		top = 0
	}
	if top+h > len(m.plain) {
		top = len(m.plain) - h
	}
	return top
}

// View renders the snapshot window with selection highlighting and a
// status line. Exactly termH lines tall.
func (m *copyModeModel) View() string {
	if m.termH <= 0 {
		return ""
	}
	bodyH := m.termH - 1 // last row reserved for the status line
	if bodyH < 1 {
		bodyH = 1
	}

	if len(m.raw) == 0 {
		blanks := strings.Repeat("\n", bodyH-1)
		return blanks + m.statusLine()
	}

	top := m.windowTop(bodyH)
	lo, hi := m.selectionRange()

	var b strings.Builder
	for row := 0; row < bodyH; row++ {
		idx := top + row
		if idx >= len(m.raw) {
			b.WriteString("\n")
			continue
		}
		line := m.raw[idx]
		if idx >= lo && idx <= hi {
			// Selection highlight. Strip the source line's ANSI so the
			// reverse-video applies to the whole row (otherwise embedded
			// SGR codes would punch holes in the highlight).
			line = lipgloss.NewStyle().Reverse(true).Render(m.plain[idx])
		}
		b.WriteString(line)
		if row < bodyH-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.statusLine())
	return b.String()
}

// statusLine builds the one-row prompt at the bottom of the overlay.
// Tints itself with the active theme accent so it stands out from the
// session output it's overlaying.
func (m *copyModeModel) statusLine() string {
	lo, hi := m.selectionRange()
	count := hi - lo + 1
	var label string
	if m.mark < 0 || count == 1 {
		label = "cursor only"
	} else {
		label = fmt.Sprintf("%d lines", count)
	}
	hint := "v mark · Enter copy · q quit"
	if m.mark >= 0 {
		hint = "v unmark · Enter copy · q quit"
	}
	status := fmt.Sprintf("COPY · %s · %s", label, hint)
	return lipgloss.NewStyle().
		Foreground(active.Warning).
		Render(status)
}
