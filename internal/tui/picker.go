package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"mterm/internal/config"
	"mterm/internal/history"
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
	deco   pickerDecorations
}

// pickerDecorations is the per-render context the App injects into the
// picker before drawing — connection history, currently-open tabs, and the
// reference "now" used to render relative timestamps. All fields are
// optional; an empty deco renders the host list without history columns.
type pickerDecorations struct {
	LastConnected map[string]time.Time
	OpenHosts     map[string]bool // hostname → host is in an open tab right now
	Now           time.Time
}

func newPicker(hosts []config.Host) *picker {
	return &picker{all: hosts}
}

func (p *picker) setSize(w, h int) { p.w, p.h = w, h }

// setDecorations is called by App.View() each render — cheap (just pointer
// copies of the deco maps) so the picker always sees current state.
func (p *picker) setDecorations(d pickerDecorations) { p.deco = d }

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

	// Pre-compute count-of-hosts per prefix path so headers can render
	// "▸ HOME (5)" with the total under that section.
	prefixCount := map[string]int{}
	for _, h := range v {
		segs := config.GroupSegments(h.Group)
		if len(segs) == 0 {
			segs = []string{"ungrouped"}
		}
		path := ""
		for _, s := range segs {
			if path == "" {
				path = s
			} else {
				path = path + "/" + s
			}
			prefixCount[path]++
		}
	}

	var prev []string
	firstHeader := true
	for i, h := range v {
		segs := config.GroupSegments(h.Group)
		if len(segs) == 0 {
			segs = []string{"ungrouped"}
		}
		// Emit headers only for path segments beyond the common prefix with
		// the previous host's path. A blank line separates top-level groups;
		// nested sub-group transitions stay flush.
		common := config.CommonPrefixLen(prev, segs)
		for level := common; level < len(segs); level++ {
			if level == 0 && !firstHeader {
				b.WriteString("\n")
			}
			firstHeader = false
			path := strings.Join(segs[:level+1], "/")
			b.WriteString(renderGroupHeader(segs[level], level, prefixCount[path], p.w))
			b.WriteString("\n")
		}
		prev = segs

		indent := strings.Repeat("  ", len(segs))
		row := formatHostRow(h, p.deco)
		if i == p.cursor {
			line := ansi.Truncate(indent+"> "+row, p.w, "")
			b.WriteString(sty.selectionBar.Width(p.w).Render(line))
		} else {
			b.WriteString(sty.dim.Render(indent + "  " + row))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderGroupHeader formats a picker group separator like:
//
//	▸ DEVELOPMENT (3) ─────────────────────────────
//
// with the glyph + label in the accent group-header style, a parenthesized
// count of hosts in this section (zero hides the count), and a trailing
// rule in dim. depth indents nested sub-groups (2 spaces per level) so the
// hierarchy is visible without changing colors.
func renderGroupHeader(label string, depth, count, width int) string {
	upper := strings.ToUpper(label)
	prefix := strings.Repeat("  ", depth) + "▸ " + upper
	if count > 0 {
		prefix += fmt.Sprintf(" (%d)", count)
	}
	prefix += " "
	fillW := width - lipgloss.Width(prefix)
	if fillW < 1 {
		fillW = 1
	}
	return sty.groupHeader.Render(prefix) + sty.dim.Render(strings.Repeat("─", fillW))
}

// formatHostRow renders one host's row as PLAIN text:
//
//	<glyph> <name>           <hostname>          <last-connected>   [tags]
//
// glyph is "◉" (currently in an open tab), "●" (previously connected at
// least once), or "○" (never connected). last-connected is a compact
// relative time ("2h ago"); "—" for never-connected hosts.
//
// The caller wraps the whole row in a single style (the selection bar or
// the dim style); embedding styling here would break that wrapping style's
// background, so this returns no ANSI of its own.
func formatHostRow(h config.Host, deco pickerDecorations) string {
	glyph := connectionGlyph(h.Name, deco)
	last, _ := deco.LastConnected[h.Name]
	now := deco.Now
	if now.IsZero() {
		now = time.Now()
	}
	row := fmt.Sprintf("%s %-16s %-18s  %-9s", glyph, h.Name, h.HostName, history.FormatRelative(now, last))
	if len(h.Tags) > 0 {
		row += "  [" + strings.Join(h.Tags, ",") + "]"
	}
	return row
}

// connectionGlyph returns a single-character indicator for whether the user
// is currently connected to host (◉), has previously connected to it (●),
// or has never connected (○). Pure cell character — no ANSI styling here;
// the picker row's outer style wraps everything.
func connectionGlyph(name string, deco pickerDecorations) string {
	if deco.OpenHosts[name] {
		return "◉"
	}
	if _, ok := deco.LastConnected[name]; ok {
		return "●"
	}
	return "○"
}
