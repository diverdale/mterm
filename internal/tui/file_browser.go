package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	mssh "mterm/internal/ssh"
)

// fileSide labels which pane has focus. The browser is a two-pane layout
// (local | remote) and most key handlers operate on whichever pane is
// active — Tab cycles between them.
type fileSide int

const (
	sideLocal fileSide = iota
	sideRemote
)

// fileEntry is one row in either pane — directory or regular file.
type fileEntry struct {
	Name  string
	IsDir bool
	Size  int64    // 0 for directories
	Mode  string   // "drwxr-xr-x" style; rendered as-is
	When  time.Time // mtime; "—" rendered when zero
}

// fileBrowserModel holds the entire two-pane file-browser state. One per
// active session — created when ^B u opens the browser, torn down when
// Esc closes it.
type fileBrowserModel struct {
	tab *sessionTab // for the SFTP client + host display in the header

	localPath  string
	remotePath string

	localEntries  []fileEntry
	remoteEntries []fileEntry

	localCursor  int
	remoteCursor int

	active fileSide

	showHidden bool

	// Status line shown below the panes. Plain text by default; switches
	// to transfer-progress rendering when `transfer` is non-nil.
	status   string
	transfer *transferState // C3 wires this up; declared here so View() can hide/show

	// termH limits how many entry rows each pane shows. Set by App.View()
	// via setSize before render. 0 means unconstrained (tests).
	termW, termH int
}

// newFileBrowserModel builds the model for the given tab. Initial paths
// are placeholders — C3 will fill them in via real getcwd / sftp.Realpath
// queries on construction.
func newFileBrowserModel(tab *sessionTab) *fileBrowserModel {
	return &fileBrowserModel{
		tab:        tab,
		localPath:  "/",
		remotePath: "/",
		active:     sideLocal,
	}
}

// setSize records the host terminal dimensions for the next render.
func (m *fileBrowserModel) setSize(w, h int) { m.termW, m.termH = w, h }

// activeEntries returns a pointer to the slice + cursor that the active
// pane is currently showing. Used by the navigation key handlers so they
// don't have to branch on side every time.
func (m *fileBrowserModel) activeEntriesPtr() (*[]fileEntry, *int, *string) {
	if m.active == sideLocal {
		return &m.localEntries, &m.localCursor, &m.localPath
	}
	return &m.remoteEntries, &m.remoteCursor, &m.remotePath
}

// Update routes key events. C3 will add the actual SFTP operations; this
// scaffold handles only the in-model navigation (Tab to switch, ↑/↓ to
// move cursor, . to toggle hidden, Esc to close).
func (m *fileBrowserModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		// If a transfer is in flight, cancel it instead of closing.
		if m.transfer != nil && m.transfer.cancel != nil {
			m.transfer.cancel()
			return nil
		}
		return func() tea.Msg { return fileBrowserClosedMsg{} }
	case tea.KeyTab:
		if m.active == sideLocal {
			m.active = sideRemote
		} else {
			m.active = sideLocal
		}
	case tea.KeyUp:
		_, cursor, _ := m.activeEntriesPtr()
		if *cursor > 0 {
			*cursor--
		}
	case tea.KeyDown:
		entries, cursor, _ := m.activeEntriesPtr()
		if *cursor < len(*entries)-1 {
			*cursor++
		}
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'j':
				entries, cursor, _ := m.activeEntriesPtr()
				if *cursor < len(*entries)-1 {
					*cursor++
				}
			case 'k':
				_, cursor, _ := m.activeEntriesPtr()
				if *cursor > 0 {
					*cursor--
				}
			case '.':
				m.showHidden = !m.showHidden
				return func() tea.Msg { return fileBrowserRefreshMsg{} }
			}
		}
	}
	return nil
}

// fileBrowserClosedMsg pops the browser off the mode stack.
type fileBrowserClosedMsg struct{}

// fileBrowserRefreshMsg asks the App to re-list both panes. Fired by the
// hidden-toggle today; C3 adds the `r` keybinding.
type fileBrowserRefreshMsg struct{}

// sortFileEntries imposes the standard "dirs first then alphabetical"
// order both panes display.
func sortFileEntries(e []fileEntry) {
	sort.SliceStable(e, func(i, j int) bool {
		if e[i].IsDir != e[j].IsDir {
			return e[i].IsDir
		}
		return strings.ToLower(e[i].Name) < strings.ToLower(e[j].Name)
	})
}

// View renders the two-pane layout. The model's setSize hint determines
// how many entry rows each pane shows.
func (m *fileBrowserModel) View() string {
	// Each pane gets half the available width minus the vertical divider
	// (1 col). Subtract 1 more for the right pane's leading divider.
	w := m.termW
	if w <= 0 {
		w = 80
	}
	paneW := (w - 1) / 2
	if paneW < 10 {
		paneW = 10
	}

	// Vertical real estate budget: terminal height - chrome (frame border
	// 2, tab strip + divider 2, header line 1, path line 1, separator 1,
	// status line 1, hint line 1, footer divider + footer + bottom 3) ≈
	// terminal - 12. Floor at 5 entry rows.
	rowBudget := m.termH - 12
	if rowBudget < 5 {
		rowBudget = 5
	}

	leftCol := m.renderPane(sideLocal, paneW, rowBudget)
	rightCol := m.renderPane(sideRemote, paneW, rowBudget)

	var b strings.Builder
	for i := 0; i < len(leftCol); i++ {
		b.WriteString(leftCol[i])
		b.WriteString(sty.border.Render(frame.Vertical))
		if i < len(rightCol) {
			b.WriteString(rightCol[i])
		}
		b.WriteString("\n")
	}
	b.WriteString(strings.TrimRight(m.renderStatus(w), "\n"))
	b.WriteString("\n")
	b.WriteString(m.renderHints(w))
	return b.String()
}

// renderPane returns the per-row strings for one side, exactly rowBudget+3
// rows tall: a header, a path line, a separator, then rowBudget entry
// rows.
func (m *fileBrowserModel) renderPane(side fileSide, w, rowBudget int) []string {
	var (
		label   string
		path    string
		entries []fileEntry
		cursor  int
	)
	if side == sideLocal {
		label = "LOCAL"
		path = m.localPath
		entries = m.localEntries
		cursor = m.localCursor
	} else {
		host := "remote"
		if m.tab != nil {
			host = m.tab.host.Name
		}
		label = "REMOTE: " + host
		path = m.remotePath
		entries = m.remoteEntries
		cursor = m.remoteCursor
	}

	out := make([]string, 0, rowBudget+3)

	header := label
	if side == m.active {
		header = sty.tabActive.Render("▶ " + header)
	} else {
		header = sty.dim.Render("  " + header)
	}
	out = append(out, padOrTrunc(header, w))
	out = append(out, padOrTrunc(sty.dim.Render(path), w))
	out = append(out, padOrTrunc(sty.border.Render(strings.Repeat(frame.Horizontal, w)), w))

	// Entries window — scroll to keep cursor visible.
	start := 0
	if cursor >= rowBudget {
		start = cursor - rowBudget + 1
	}
	for i := 0; i < rowBudget; i++ {
		idx := start + i
		if idx >= len(entries) {
			out = append(out, padOrTrunc("", w))
			continue
		}
		e := entries[idx]
		row := formatFileEntry(e)
		if idx == cursor && side == m.active {
			out = append(out, sty.selectionBar.Width(w).Render(row))
		} else if e.IsDir {
			out = append(out, padOrTrunc(sty.tabActive.Render(row), w))
		} else {
			out = append(out, padOrTrunc(sty.dim.Render(row), w))
		}
	}
	return out
}

// formatFileEntry renders one row's plain (unstyled) text: directory marker,
// name, size for files.
func formatFileEntry(e fileEntry) string {
	marker := "  "
	if e.IsDir {
		marker = "▸ "
	}
	if e.IsDir {
		return marker + e.Name + "/"
	}
	return marker + fmt.Sprintf("%-30s %10s", e.Name, formatBytes(e.Size))
}

// renderStatus shows transfer progress when active, otherwise the user's
// last status message (or a default hint).
func (m *fileBrowserModel) renderStatus(w int) string {
	if m.transfer != nil {
		return padOrTrunc(sty.errorText.Render(m.transfer.line()), w+1)
	}
	if m.status != "" {
		return padOrTrunc(sty.dim.Render(m.status), w+1)
	}
	return padOrTrunc(sty.dim.Render(" "), w+1)
}

// renderHints is the keybinding crib sheet at the bottom of the panel.
func (m *fileBrowserModel) renderHints(w int) string {
	hints := []string{
		"Tab switch", "↑/↓ move", "Enter open", "Bksp up",
		"F5 copy", "F7 mkdir", "F8 delete", ". hidden", "r refresh", "Esc close",
	}
	line := strings.Join(hints, sty.dim.Render(" · "))
	for _, h := range hints {
		// Re-render with footer styling so the key/label distinction is
		// consistent with the existing footer chrome.
		_ = h
	}
	return padOrTrunc(sty.dim.Render(line), w+1)
}

// padOrTrunc force-fits s to exactly width display columns. lipgloss's
// MaxWidth wraps; ansi.Truncate respects ANSI escapes but doesn't pad —
// so we truncate then style-pad to width.
func padOrTrunc(s string, width int) string {
	s = ansi.Truncate(s, width, "")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

// formatBytes renders a byte count in IEC-binary form ("12.3 MB").
func formatBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// transferState tracks a single in-flight copy. C3 fills this in;
// declared here so View() can render its line and Esc can cancel.
type transferState struct {
	name    string
	total   int64
	written *atomicInt64Stub // C3 hooks up the real atomic.Int64
	started time.Time
	cancel  func()
	done    bool
	err     error
}

// atomicInt64Stub is a placeholder so this file compiles before C3 lands.
// C3 replaces it with sync/atomic's Int64.
type atomicInt64Stub struct{ v int64 }

func (s *atomicInt64Stub) Load() int64 { return s.v }

// line renders the progress display: name, percent, bytes, rate. Called
// from renderStatus; returns "" when no transfer is in progress.
func (t *transferState) line() string {
	if t == nil {
		return ""
	}
	if t.done && t.err != nil {
		return fmt.Sprintf("error: %s — %v", t.name, t.err)
	}
	if t.done {
		return fmt.Sprintf("done: %s (%s in %s)", t.name, formatBytes(t.total),
			t.started.Sub(t.started).Truncate(time.Millisecond))
	}
	written := t.written.Load()
	pct := 0
	if t.total > 0 {
		pct = int(written * 100 / t.total)
	}
	elapsed := time.Since(t.started)
	rate := float64(written)
	if elapsed > 0 {
		rate = float64(written) / elapsed.Seconds()
	}
	return fmt.Sprintf("transferring %s: %d%% · %s / %s · %s/s",
		t.name, pct, formatBytes(written), formatBytes(t.total),
		formatBytes(int64(rate)))
}

// shorten cuts s down to maxLen chars with a leading "…" preserving the
// tail. Used to keep long paths from blowing the pane width.
func shorten(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return s[:maxLen]
	}
	return "…" + s[len(s)-maxLen+1:]
}

// unused placeholder so the Session import sticks until C3 wires SFTP.
var _ = mssh.NewSession
