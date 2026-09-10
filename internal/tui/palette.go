package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages emitted by the palette.
type paletteChosenMsg struct{ cmd command }
type paletteClosedMsg struct{}

const (
	paletteWidth = 60
	// paletteInnerWidth is the content width inside Padding(1, 2) — lipgloss
	// Width sets the content+padding width and the border sits OUTSIDE it,
	// so the available text width is paletteWidth minus the 4 padding columns.
	paletteInnerWidth = paletteWidth - 4
)

// paletteModel is the centered fuzzy-search command palette.
type paletteModel struct {
	all    []command
	query  string
	cursor int

	// termH is the host terminal height in rows. View() uses it to cap
	// the visible command list so the modal never overflows the screen
	// and pushes the title off the top. 0 = unconstrained (tests bypass
	// setSize); render shows all visible commands then.
	termH int
}

// setSize records the host terminal dimensions so View() can pick how
// many command rows fit. Called from App.View() before every render.
func (p *paletteModel) setSize(_, h int) { p.termH = h }

// newPaletteModel constructs a palette from a pre-built command list.
func newPaletteModel(cmds []command) *paletteModel {
	return &paletteModel{all: cmds}
}

// visible returns the commands matching the current query (case-insensitive
// substring over label).
func (p *paletteModel) visible() []command {
	if p.query == "" {
		return p.all
	}
	q := strings.ToLower(p.query)
	out := make([]command, 0, len(p.all))
	for _, c := range p.all {
		if strings.Contains(strings.ToLower(c.label), q) {
			out = append(out, c)
		}
	}
	return out
}

// setQuery updates the search query and resets the cursor.
func (p *paletteModel) setQuery(q string) {
	p.query = q
	p.cursor = 0
}

// move advances the cursor by delta, wrapping around the visible list.
func (p *paletteModel) move(delta int) {
	v := p.visible()
	if len(v) == 0 {
		p.cursor = 0
		return
	}
	p.cursor = (p.cursor + delta + len(v)) % len(v)
}

// Update handles palette key events.
func (p *paletteModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		v := p.visible()
		if p.cursor < 0 || p.cursor >= len(v) {
			return nil
		}
		chosen := v[p.cursor]
		return func() tea.Msg { return paletteChosenMsg{cmd: chosen} }
	case tea.KeyEsc:
		return func() tea.Msg { return paletteClosedMsg{} }
	case tea.KeyUp, tea.KeyCtrlK:
		p.move(-1)
	case tea.KeyDown, tea.KeyCtrlJ:
		p.move(1)
	case tea.KeyBackspace:
		if p.query != "" {
			runes := []rune(p.query)
			p.setQuery(string(runes[:len(runes)-1]))
		}
	case tea.KeyRunes, tea.KeySpace:
		p.setQuery(p.query + string(msg.Runes))
	}
	return nil
}

// paletteChromeRows is the non-command vertical real estate the modal
// reserves: title (1) + blank (1) + search line (1) + blank (1) +
// blank-before-footer (1) + footer (1) + box border top (1) + box
// border bottom (1) + box padding top (1) + box padding bottom (1) +
// indicator rows (2, for "▴ N more above / ▾ N more below" when truncated)
// + lipgloss.Place vertical padding (2, conservative).
const paletteChromeRows = 14

// View renders the palette as a centered rounded-border modal. When the
// command list is taller than the terminal can show, the visible window
// scrolls to keep the cursor in view and emits "▴ N more / ▾ N more"
// indicators so the user knows there is content above/below.
func (p *paletteModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Command Palette"))
	b.WriteString("\n\n")
	b.WriteString(sty.dim.Render("> "))
	b.WriteString(sty.search.Render(p.query))
	b.WriteString("\n\n")

	v := p.visible()
	if len(v) == 0 {
		b.WriteString(sty.dim.Render("  (no matching commands)"))
	} else {
		maxRows := 0
		if p.termH > 0 {
			maxRows = p.termH - paletteChromeRows
		}
		start, end, more := listWindow(len(v), p.cursor, maxRows)
		if more.above > 0 {
			b.WriteString(sty.dim.Render(fmt.Sprintf("  ▴ %d more above", more.above)))
			b.WriteString("\n")
		}
		// Re-emit any group header that's already in effect at `start` so
		// users can tell what section the visible window belongs to.
		if start > 0 {
			groupAtStart := ""
			for k := 0; k <= start; k++ {
				if v[k].group != groupAtStart {
					groupAtStart = v[k].group
				}
			}
			if groupAtStart != "" {
				b.WriteString(sty.groupHeader.Render("  " + groupAtStart))
				b.WriteString("\n")
			}
		}
		lastGroup := ""
		if start > 0 {
			// We just emitted the group above, suppress an immediate repeat.
			for k := 0; k <= start; k++ {
				if v[k].group != lastGroup {
					lastGroup = v[k].group
				}
			}
		} else {
			lastGroup = "\x00" // force first header
		}
		for i := start; i < end; i++ {
			c := v[i]
			if c.group != lastGroup {
				lastGroup = c.group
				b.WriteString(sty.groupHeader.Render("  " + c.group))
				b.WriteString("\n")
			}
			if i == p.cursor {
				b.WriteString(sty.selectionBar.Width(paletteInnerWidth).Render("> " + c.label))
			} else {
				b.WriteString(sty.dim.Render("  " + c.label))
			}
			b.WriteString("\n")
		}
		if more.below > 0 {
			b.WriteString(sty.dim.Render(fmt.Sprintf("  ▾ %d more below", more.below)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("  enter: run   esc: cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2).
		Width(paletteWidth)
	return box.Render(b.String())
}

