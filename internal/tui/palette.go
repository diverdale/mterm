package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages emitted by the palette.
type paletteChosenMsg struct{ cmd command }
type paletteClosedMsg struct{}

const (
	paletteWidth      = 60
	paletteInnerWidth = paletteWidth - 6 // 2 border + 2*2 padding
)

// paletteModel is the centered fuzzy-search command palette.
type paletteModel struct {
	all    []command
	query  string
	cursor int
}

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

// View renders the palette as a centered rounded-border modal.
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
		lastGroup := "\x00"
		for i, c := range v {
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
