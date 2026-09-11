package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/config"
)

type snippetChosenMsg struct{ send string }
type snippetClosedMsg struct{}

const snippetsChromeRows = 14

// snippetsModel is the fuzzy-search palette for saved commands.
type snippetsModel struct {
	all    []config.SnippetEntry
	query  string
	cursor int
	termH  int
}

func newSnippetsModel(entries []config.SnippetEntry) *snippetsModel {
	return &snippetsModel{all: entries}
}

func (s *snippetsModel) setSize(_, h int) { s.termH = h }

func (s *snippetsModel) visible() []config.SnippetEntry {
	if s.query == "" {
		return s.all
	}
	q := strings.ToLower(s.query)
	out := make([]config.SnippetEntry, 0, len(s.all))
	for _, e := range s.all {
		hay := strings.ToLower(e.Name + " " + e.Send + " " + e.Group)
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

func (s *snippetsModel) setQuery(q string) {
	s.query = q
	s.cursor = 0
}

func (s *snippetsModel) move(delta int) {
	v := s.visible()
	if len(v) == 0 {
		s.cursor = 0
		return
	}
	s.cursor = (s.cursor + delta + len(v)) % len(v)
}

func (s *snippetsModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		v := s.visible()
		if s.cursor < 0 || s.cursor >= len(v) {
			return nil
		}
		chosen := v[s.cursor]
		return func() tea.Msg { return snippetChosenMsg{send: chosen.Send} }
	case tea.KeyEsc:
		return func() tea.Msg { return snippetClosedMsg{} }
	case tea.KeyUp, tea.KeyCtrlK:
		s.move(-1)
	case tea.KeyDown, tea.KeyCtrlJ:
		s.move(1)
	case tea.KeyBackspace:
		if s.query != "" {
			runes := []rune(s.query)
			s.setQuery(string(runes[:len(runes)-1]))
		}
	case tea.KeyRunes, tea.KeySpace:
		s.setQuery(s.query + string(msg.Runes))
	}
	return nil
}

func (s *snippetsModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Snippets"))
	b.WriteString("\n\n")
	b.WriteString(sty.dim.Render("> "))
	b.WriteString(sty.search.Render(s.query))
	b.WriteString("\n\n")

	v := s.visible()
	if len(v) == 0 {
		b.WriteString(sty.dim.Render("  (no matching snippets)"))
	} else {
		maxRows := 0
		if s.termH > 0 {
			maxRows = s.termH - snippetsChromeRows
		}
		start, end, more := listWindow(len(v), s.cursor, maxRows)
		if more.above > 0 {
			b.WriteString(sty.dim.Render(fmt.Sprintf("  ▴ %d more above", more.above)))
			b.WriteString("\n")
		}
		lastGroup := ""
		if start > 0 {
			for k := 0; k <= start; k++ {
				lastGroup = v[k].Group
			}
		} else {
			lastGroup = "\x00"
		}
		for i := start; i < end; i++ {
			e := v[i]
			if e.Group != lastGroup {
				lastGroup = e.Group
				b.WriteString(sty.groupHeader.Render("  " + e.Group))
				b.WriteString("\n")
			}
			label := snippetLabel(e)
			if i == s.cursor {
				b.WriteString(sty.selectionBar.Width(paletteInnerWidth).Render("> " + label))
			} else {
				b.WriteString(sty.dim.Render("  " + label))
			}
			b.WriteString("\n")
		}
		if more.below > 0 {
			b.WriteString(sty.dim.Render(fmt.Sprintf("  ▾ %d more below", more.below)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("  enter: send   esc: cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2).
		Width(paletteWidth)
	return box.Render(b.String())
}

// snippetLabel renders the one-line palette label for a snippet entry.
func snippetLabel(e config.SnippetEntry) string {
	preview := strings.ReplaceAll(e.Send, "\n", " ")
	preview = strings.TrimSpace(preview)
	if len(preview) > 40 {
		preview = preview[:37] + "..."
	}
	if preview == "" {
		return e.Name
	}
	return e.Name + "  " + preview
}

// snippetPayload prepares snippet text for the remote PTY. A trailing newline
// is appended when absent so single-line commands execute immediately.
func snippetPayload(send string) []byte {
	if send == "" {
		return nil
	}
	if !strings.HasSuffix(send, "\n") && !strings.HasSuffix(send, "\r") {
		send += "\r"
	}
	return []byte(send)
}
