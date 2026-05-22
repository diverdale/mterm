package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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

// View renders the panel.
func (p *forwardsPanel) View() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Port forwards — %s\n\n", p.host.Name))
	if len(p.host.Forwards) == 0 {
		b.WriteString(statusBar.Render("  (no forwards configured for this host)"))
		b.WriteString("\n")
	}
	for i, f := range p.host.Forwards {
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		mark := "[ ]"
		if p.on[i] {
			mark = "[x]"
		}
		b.WriteString(fmt.Sprintf("%s%s %-6s %d -> %s:%d\n",
			cursor, mark, f.Type.String(), f.BindPort, f.DialAddr, f.DialPort))
	}
	b.WriteString("\n")
	b.WriteString(statusBar.Render("  space: toggle   esc: close"))
	return modalBox.Render(b.String())
}
