package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
)

// pickerChosenMsg is emitted when the user selects a host in the picker.
type pickerChosenMsg struct{ host config.Host }

// pickerCancelledMsg is emitted when the user dismisses the picker.
type pickerCancelledMsg struct{}

// picker is the fuzzy host-selection view.
type picker struct {
	all    []config.Host
	query  string
	cursor int
	w, h   int
}

func newPicker(hosts []config.Host) *picker {
	return &picker{all: hosts}
}

func (p *picker) setSize(w, h int) { p.w, p.h = w, h }

func (p *picker) setQuery(q string) {
	p.query = q
	p.cursor = 0
}

// visibleHosts returns hosts matching the current query (case-insensitive
// substring over name, hostname, group, and tags).
func (p *picker) visibleHosts() []config.Host {
	if p.query == "" {
		return p.all
	}
	q := strings.ToLower(p.query)
	var out []config.Host
	for _, h := range p.all {
		if hostMatches(h, q) {
			out = append(out, h)
		}
	}
	return out
}

func hostMatches(h config.Host, q string) bool {
	hay := strings.ToLower(strings.Join(append([]string{
		h.Name, h.HostName, h.Group,
	}, h.Tags...), " "))
	return strings.Contains(hay, q)
}

// selected returns the host under the cursor, if any.
func (p *picker) selected() (config.Host, bool) {
	v := p.visibleHosts()
	if len(v) == 0 || p.cursor < 0 || p.cursor >= len(v) {
		return config.Host{}, false
	}
	return v[p.cursor], true
}

func (p *picker) moveCursor(delta int) {
	v := p.visibleHosts()
	if len(v) == 0 {
		p.cursor = 0
		return
	}
	p.cursor = (p.cursor + delta + len(v)) % len(v)
}

// Update handles picker key events. It returns a command carrying a
// pickerChosenMsg or pickerCancelledMsg when the picker resolves.
func (p *picker) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		if h, ok := p.selected(); ok {
			return func() tea.Msg { return pickerChosenMsg{host: h} }
		}
	case tea.KeyEsc:
		return func() tea.Msg { return pickerCancelledMsg{} }
	case tea.KeyUp:
		p.moveCursor(-1)
	case tea.KeyDown:
		p.moveCursor(1)
	case tea.KeyCtrlK:
		p.moveCursor(-1)
	case tea.KeyCtrlJ:
		p.moveCursor(1)
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

// View renders the picker body: a search line and the grouped, filtered host
// list. The window frame and footer are added by the app.
func (p *picker) View() string {
	var b strings.Builder
	b.WriteString(sty.dim.Render("Search: "))
	b.WriteString(sty.search.Render(p.query))
	b.WriteString("\n\n")

	v := p.visibleHosts()
	if len(v) == 0 {
		b.WriteString(sty.dim.Render("  (no matching hosts)"))
		return b.String()
	}

	lastGroup := "\x00"
	for i, h := range v {
		if h.Group != lastGroup {
			lastGroup = h.Group
			group := h.Group
			if group == "" {
				group = "(ungrouped)"
			}
			b.WriteString(sty.groupHeader.Render("  " + group))
			b.WriteString("\n")
		}
		row := formatHostRow(h)
		if i == p.cursor {
			b.WriteString(sty.selectionBar.Render("> " + row))
		} else {
			b.WriteString(sty.dim.Render("  ") + row)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatHostRow renders one host's name, hostname, and tags.
func formatHostRow(h config.Host) string {
	row := fmt.Sprintf("%-16s %-18s", h.Name, h.HostName)
	if len(h.Tags) > 0 {
		row += sty.tag.Render("  [" + strings.Join(h.Tags, ",") + "]")
	}
	return row
}
