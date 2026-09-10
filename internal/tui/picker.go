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

type pickerRowKind int

const (
	pickerRowGroup pickerRowKind = iota
	pickerRowHost
)

type pickerRow struct {
	kind  pickerRowKind
	path  string // group path for headers; "" for hosts
	label string // display segment for headers
	depth int
	count int
	host  config.Host
}

// picker is the fuzzy host-selection view.
type picker struct {
	all            []config.Host
	query          string
	cursor         int
	collapsed      map[string]bool
	savedCollapsed map[string]bool
	w, h           int // w = content width; h = terminal height (0 = unconstrained)
	deco           pickerDecorations
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
	return &picker{
		all:       hosts,
		collapsed: map[string]bool{},
	}
}

func (p *picker) setSize(w, termH int) { p.w, p.h = w, termH }

// setDecorations is called by App.View() each render — cheap (just pointer
// copies of the deco maps) so the picker always sees current state.
func (p *picker) setDecorations(d pickerDecorations) { p.deco = d }

func cloneCollapsed(m map[string]bool) map[string]bool {
	if m == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (p *picker) setQuery(q string) {
	old := p.query
	p.query = q
	p.cursor = 0
	if old == "" && q != "" {
		p.savedCollapsed = cloneCollapsed(p.collapsed)
	}
	if old != "" && q == "" && p.savedCollapsed != nil {
		p.collapsed = cloneCollapsed(p.savedCollapsed)
		p.savedCollapsed = nil
	}
}

func (p *picker) effectiveCollapsed() map[string]bool {
	if p.query != "" {
		return map[string]bool{}
	}
	return p.collapsed
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

func hostGroupSegments(h config.Host) []string {
	segs := config.GroupSegments(h.Group)
	if len(segs) == 0 {
		return []string{"ungrouped"}
	}
	return segs
}

func groupPath(segs []string, level int) string {
	return strings.Join(segs[:level+1], "/")
}

func isAnyPrefixCollapsed(collapsed map[string]bool, segs []string) bool {
	path := ""
	for _, s := range segs {
		if path == "" {
			path = s
		} else {
			path = path + "/" + s
		}
		if collapsed[path] {
			return true
		}
	}
	return false
}

func prefixCounts(hosts []config.Host) map[string]int {
	counts := map[string]int{}
	for _, h := range hosts {
		segs := hostGroupSegments(h)
		path := ""
		for _, s := range segs {
			if path == "" {
				path = s
			} else {
				path = path + "/" + s
			}
			counts[path]++
		}
	}
	return counts
}

// buildRows constructs the visible picker row list from filtered hosts and
// collapse state.
func buildRows(hosts []config.Host, collapsed map[string]bool) []pickerRow {
	if len(hosts) == 0 {
		return nil
	}
	counts := prefixCounts(hosts)
	var rows []pickerRow
	var prev []string
	for _, h := range hosts {
		segs := hostGroupSegments(h)
		hidden := isAnyPrefixCollapsed(collapsed, segs)
		common := config.CommonPrefixLen(prev, segs)
		for level := common; level < len(segs); level++ {
			if level > 0 {
				parentPath := groupPath(segs, level-1)
				if collapsed[parentPath] {
					break
				}
			}
			path := groupPath(segs, level)
			rows = append(rows, pickerRow{
				kind:  pickerRowGroup,
				path:  path,
				label: segs[level],
				depth: level,
				count: counts[path],
			})
			if collapsed[path] {
				break
			}
		}
		if !hidden {
			rows = append(rows, pickerRow{
				kind:  pickerRowHost,
				depth: len(segs),
				host:  h,
			})
		}
		prev = segs
	}
	return rows
}

func (p *picker) visibleRows() []pickerRow {
	return buildRows(p.visibleHosts(), p.effectiveCollapsed())
}

func (p *picker) cursorRow() *pickerRow {
	rows := p.visibleRows()
	if p.cursor < 0 || p.cursor >= len(rows) {
		return nil
	}
	return &rows[p.cursor]
}

func rowIdentifier(r pickerRow) string {
	if r.kind == pickerRowGroup {
		return "g:" + r.path
	}
	return "h:" + r.host.Name
}

func (p *picker) clampCursor() {
	rows := p.visibleRows()
	if len(rows) == 0 {
		p.cursor = 0
		return
	}
	if p.cursor >= len(rows) {
		p.cursor = len(rows) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

func (p *picker) setCollapsed(path string, collapsed bool) {
	oldRows := p.visibleRows()
	oldID := ""
	if p.cursor >= 0 && p.cursor < len(oldRows) {
		oldID = rowIdentifier(oldRows[p.cursor])
	}

	if collapsed {
		p.collapsed[path] = true
	} else {
		delete(p.collapsed, path)
	}

	newRows := p.visibleRows()
	if len(newRows) == 0 {
		p.cursor = 0
		return
	}
	if oldID != "" {
		for i, r := range newRows {
			if rowIdentifier(r) == oldID {
				p.cursor = i
				return
			}
		}
	}
	for i, r := range newRows {
		if r.kind == pickerRowGroup && r.path == path {
			p.cursor = i
			return
		}
	}
	p.clampCursor()
}

func (p *picker) toggleCollapsed(path string) {
	p.setCollapsed(path, !p.collapsed[path])
}

func (p *picker) collapse(path string) {
	if p.collapsed[path] {
		return
	}
	p.setCollapsed(path, true)
}

func (p *picker) expand(path string) {
	if !p.collapsed[path] {
		return
	}
	p.setCollapsed(path, false)
}

// selected returns the host under the cursor, if the cursor is on a host row.
func (p *picker) selected() (config.Host, bool) {
	row := p.cursorRow()
	if row == nil || row.kind != pickerRowHost {
		return config.Host{}, false
	}
	return row.host, true
}

func (p *picker) moveCursor(delta int) {
	rows := p.visibleRows()
	if len(rows) == 0 {
		p.cursor = 0
		return
	}
	p.cursor = (p.cursor + delta + len(rows)) % len(rows)
}

// pickerBodyChrome is non-row vertical space inside the framed body: search
// line, blank line, and two optional truncation-indicator slots.
const pickerBodyChrome = 4

// maxVisibleRows is how many picker rows fit in the window body.
func (p *picker) maxVisibleRows() int {
	if p.h <= 0 {
		return 0
	}
	bodyH := p.h - 4 // renderWindow body budget (title + divider + footer + bottom)
	rows := bodyH - pickerBodyChrome
	if rows < 3 {
		rows = 3
	}
	return rows
}

// Update handles picker key events. It returns a command carrying a
// pickerChosenMsg or pickerCancelledMsg when the picker resolves.
func (p *picker) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		if row := p.cursorRow(); row != nil && row.kind == pickerRowGroup {
			p.toggleCollapsed(row.path)
			return nil
		}
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
	case tea.KeyLeft:
		if row := p.cursorRow(); row != nil && row.kind == pickerRowGroup {
			p.collapse(row.path)
		}
	case tea.KeyRight:
		if row := p.cursorRow(); row != nil && row.kind == pickerRowGroup {
			p.expand(row.path)
		}
	case tea.KeyBackspace:
		if p.query != "" {
			runes := []rune(p.query)
			p.setQuery(string(runes[:len(runes)-1]))
		}
	case tea.KeySpace:
		if row := p.cursorRow(); row != nil && row.kind == pickerRowGroup {
			p.toggleCollapsed(row.path)
		}
	case tea.KeyRunes:
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

	if len(p.visibleHosts()) == 0 {
		b.WriteString(sty.dim.Render("  (no matching hosts)"))
		return b.String()
	}

	rows := p.visibleRows()
	if len(rows) == 0 {
		b.WriteString(sty.dim.Render("  (no matching hosts)"))
		return b.String()
	}

	collapsed := p.effectiveCollapsed()
	start, end, more := listWindow(len(rows), p.cursor, p.maxVisibleRows())
	if more.above > 0 {
		b.WriteString(sty.dim.Render(fmt.Sprintf("  ▴ %d more above", more.above)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		row := rows[i]
		if row.kind == pickerRowGroup && row.depth == 0 && i > 0 {
			b.WriteString("\n")
		}
		if i == p.cursor {
			b.WriteString(renderPickerRow(row, collapsed, p.deco, p.w, true))
		} else {
			b.WriteString(renderPickerRow(row, collapsed, p.deco, p.w, false))
		}
		b.WriteString("\n")
	}
	if more.below > 0 {
		b.WriteString(sty.dim.Render(fmt.Sprintf("  ▾ %d more below", more.below)))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderPickerRow(row pickerRow, collapsed map[string]bool, deco pickerDecorations, width int, selected bool) string {
	if row.kind == pickerRowGroup {
		expanded := !collapsed[row.path]
		line := renderGroupHeader(row.label, row.depth, row.count, width, expanded)
		if selected {
			return sty.selectionBar.Width(width).Render(ansi.Truncate("> "+line, width, ""))
		}
		return sty.dim.Render(line)
	}
	indent := strings.Repeat("  ", row.depth)
	text := formatHostRow(row.host, deco)
	if selected {
		line := ansi.Truncate(indent+"> "+text, width, "")
		return sty.selectionBar.Width(width).Render(line)
	}
	return sty.dim.Render(indent + "  " + text)
}

// renderGroupHeader formats a picker group separator like:
//
//	▾ DEVELOPMENT (3) ─────────────────────────────
//
// with the glyph + label in the accent group-header style, a parenthesized
// count of hosts in this section (zero hides the count), and a trailing
// rule in dim. depth indents nested sub-groups (2 spaces per level) so the
// hierarchy is visible without changing colors.
func renderGroupHeader(label string, depth, count, width int, expanded bool) string {
	upper := strings.ToUpper(label)
	glyph := "▾"
	if !expanded {
		glyph = "▸"
	}
	prefix := strings.Repeat("  ", depth) + glyph + " " + upper
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
