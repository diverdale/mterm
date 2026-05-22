package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/config"
)

// forwardsClosedMsg is emitted when the forwards panel is dismissed.
type forwardsClosedMsg struct{}

// forwardsPanel lists a host's port forwards and lets the user toggle each.
type forwardsPanel struct {
	host   config.Host
	on     []bool // enabled state, parallel to host.Forwards
	cursor int
}

func newForwardsPanel(host config.Host) *forwardsPanel {
	return &forwardsPanel{
		host: host,
		on:   make([]bool, len(host.Forwards)),
	}
}

func (p *forwardsPanel) enabled(i int) bool {
	return i >= 0 && i < len(p.on) && p.on[i]
}

func (p *forwardsPanel) toggle(i int) {
	if i >= 0 && i < len(p.on) {
		p.on[i] = !p.on[i]
	}
}

// Update handles panel key events.
func (p *forwardsPanel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		return func() tea.Msg { return forwardsClosedMsg{} }
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case tea.KeyDown:
		if p.cursor < len(p.host.Forwards)-1 {
			p.cursor++
		}
	case tea.KeySpace, tea.KeyEnter:
		p.toggle(p.cursor)
	}
	return nil
}

// View renders the forwards modal.
func (p *forwardsPanel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Port forwards — " + p.host.Name))
	b.WriteString("\n\n")
	if len(p.host.Forwards) == 0 {
		b.WriteString(sty.dim.Render("  (no forwards configured for this host)"))
		b.WriteString("\n")
	}
	for i, f := range p.host.Forwards {
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		mark := sty.dim.Render("[ ]")
		if p.on[i] {
			mark = sty.connected.Render("[x]")
		}
		line := fmt.Sprintf("%s%s %-6s %d -> %s:%d",
			cursor, mark, f.Type.String(), f.BindPort, f.DialAddr, f.DialPort)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("  space: toggle   esc: close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2)
	return box.Render(b.String())
}
