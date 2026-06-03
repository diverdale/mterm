package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/pkg/sftp"
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
	case tea.KeyEnter, tea.KeyRight:
		return m.descendActive()
	case tea.KeyBackspace, tea.KeyLeft:
		return m.ascendActive()
	case tea.KeyF5:
		return m.copyActive()
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
			case 'l':
				return m.descendActive()
			case 'h':
				return m.ascendActive()
			case 'c':
				return m.copyActive()
			case '.':
				m.showHidden = !m.showHidden
				return func() tea.Msg { return fileBrowserRefreshMsg{} }
			case 'r':
				return func() tea.Msg { return fileBrowserRefreshMsg{} }
			}
		}
	}
	return nil
}

// fileBrowserClosedMsg pops the browser off the mode stack.
type fileBrowserClosedMsg struct{}

// fileBrowserRefreshMsg asks the App to re-list both panes.
type fileBrowserRefreshMsg struct{}

// directory-listing results — one msg per pane, fired by listLocal /
// listRemote tea.Cmds.
type localListedMsg struct {
	path    string
	entries []fileEntry
	err     error
}

type remoteListedMsg struct {
	path    string
	entries []fileEntry
	err     error
}

// transferProgressTickMsg drives the progress-line redraw while a copy
// is running. The handler in app.go re-schedules itself until the
// transfer is done.
type transferProgressTickMsg struct{}

// transferDoneMsg fires when a copy finishes (success or error). The
// handler clears m.transfer and re-lists the destination pane so the
// new file appears.
type transferDoneMsg struct {
	name    string
	bytes   int64
	elapsed time.Duration
	err     error
	dest    fileSide // which pane to re-list
}

// Init returns the tea.Cmd that fires the initial directory listings
// once the model is in scope. Local starts at os.Getwd(); remote starts
// at the SFTP connection's working directory (typically $HOME).
func (m *fileBrowserModel) Init() tea.Cmd {
	if cwd, err := os.Getwd(); err == nil {
		m.localPath = cwd
	} else {
		m.localPath = "/"
	}
	// Remote starting path is the SFTP CWD; if SFTP isn't ready yet
	// (session still connecting), fall back to "/" and the user can
	// navigate to $HOME themselves.
	m.remotePath = "/"
	if client := m.sftpClient(); client != nil {
		if wd, err := client.Getwd(); err == nil {
			m.remotePath = wd
		}
	}
	return tea.Batch(m.listLocal(m.localPath), m.listRemote(m.remotePath))
}

// sftpClient returns the SFTP client for the current tab's session, or
// nil if the session isn't ready (still connecting / disconnected).
func (m *fileBrowserModel) sftpClient() *sftp.Client {
	if m.tab == nil {
		return nil
	}
	sess := m.tab.sess.Load()
	if sess == nil {
		return nil
	}
	c, err := sess.SFTP()
	if err != nil {
		return nil
	}
	return c
}

// listLocal returns a tea.Cmd that reads dir from the local filesystem
// and dispatches localListedMsg.
func (m *fileBrowserModel) listLocal(dir string) tea.Cmd {
	showHidden := m.showHidden
	return func() tea.Msg {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return localListedMsg{path: dir, err: err}
		}
		out := make([]fileEntry, 0, len(ents)+1)
		// Synthetic ".." for navigating up — always at the top.
		out = append(out, fileEntry{Name: "..", IsDir: true})
		for _, e := range ents {
			if !showHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			fe := fileEntry{Name: e.Name(), IsDir: e.IsDir()}
			if info, err := e.Info(); err == nil {
				if !fe.IsDir {
					fe.Size = info.Size()
				}
				fe.Mode = info.Mode().String()
				fe.When = info.ModTime()
			}
			out = append(out, fe)
		}
		// Keep ".." pinned at index 0; sort the rest.
		rest := out[1:]
		sortFileEntries(rest)
		return localListedMsg{path: dir, entries: out}
	}
}

// listRemote is the SFTP-backed twin of listLocal. Dispatches
// remoteListedMsg. Falls back to an error msg when the SFTP client
// isn't available.
func (m *fileBrowserModel) listRemote(dir string) tea.Cmd {
	client := m.sftpClient()
	showHidden := m.showHidden
	return func() tea.Msg {
		if client == nil {
			return remoteListedMsg{path: dir, err: errors.New("sftp client unavailable (session not connected?)")}
		}
		ents, err := client.ReadDir(dir)
		if err != nil {
			return remoteListedMsg{path: dir, err: err}
		}
		out := make([]fileEntry, 0, len(ents)+1)
		out = append(out, fileEntry{Name: "..", IsDir: true})
		for _, info := range ents {
			if !showHidden && strings.HasPrefix(info.Name(), ".") {
				continue
			}
			fe := fileEntry{
				Name:  info.Name(),
				IsDir: info.IsDir(),
				Mode:  info.Mode().String(),
				When:  info.ModTime(),
			}
			if !fe.IsDir {
				fe.Size = info.Size()
			}
			out = append(out, fe)
		}
		rest := out[1:]
		sortFileEntries(rest)
		return remoteListedMsg{path: dir, entries: out}
	}
}

// applyListing wires a fresh listing result into the model. Called by the
// app.go handlers for localListedMsg / remoteListedMsg.
func (m *fileBrowserModel) applyLocalListing(msg localListedMsg) {
	if msg.err != nil {
		m.status = "local: " + msg.err.Error()
		return
	}
	m.localPath = msg.path
	m.localEntries = msg.entries
	if m.localCursor >= len(m.localEntries) {
		m.localCursor = len(m.localEntries) - 1
	}
	if m.localCursor < 0 {
		m.localCursor = 0
	}
	if msg.err == nil {
		m.status = ""
	}
}

func (m *fileBrowserModel) applyRemoteListing(msg remoteListedMsg) {
	if msg.err != nil {
		m.status = "remote: " + msg.err.Error()
		return
	}
	m.remotePath = msg.path
	m.remoteEntries = msg.entries
	if m.remoteCursor >= len(m.remoteEntries) {
		m.remoteCursor = len(m.remoteEntries) - 1
	}
	if m.remoteCursor < 0 {
		m.remoteCursor = 0
	}
	if msg.err == nil {
		m.status = ""
	}
}

// activeEntry returns the entry under the active pane's cursor, or nil
// when the pane is empty.
func (m *fileBrowserModel) activeEntry() *fileEntry {
	entries, cursor, _ := m.activeEntriesPtr()
	if *cursor < 0 || *cursor >= len(*entries) {
		return nil
	}
	return &(*entries)[*cursor]
}

// activePath returns the current dir path for the active pane.
func (m *fileBrowserModel) activePath() string {
	if m.active == sideLocal {
		return m.localPath
	}
	return m.remotePath
}

// descendActive handles Enter: enter a directory in the active pane. ".." goes
// up one level; a regular dir name appends; a file is a no-op.
func (m *fileBrowserModel) descendActive() tea.Cmd {
	e := m.activeEntry()
	if e == nil || !e.IsDir {
		return nil
	}
	cur := m.activePath()
	var next string
	if e.Name == ".." {
		next = parentPath(cur, m.active == sideRemote)
	} else if m.active == sideRemote {
		next = path.Join(cur, e.Name)
	} else {
		next = filepath.Join(cur, e.Name)
	}
	if m.active == sideLocal {
		m.localCursor = 0
		return m.listLocal(next)
	}
	m.remoteCursor = 0
	return m.listRemote(next)
}

// ascendActive handles Backspace: go to the parent of the active pane.
func (m *fileBrowserModel) ascendActive() tea.Cmd {
	cur := m.activePath()
	next := parentPath(cur, m.active == sideRemote)
	if m.active == sideLocal {
		m.localCursor = 0
		return m.listLocal(next)
	}
	m.remoteCursor = 0
	return m.listRemote(next)
}

// parentPath returns the parent of p. Uses forward slashes when remote
// (POSIX paths) and the host OS separator when local.
func parentPath(p string, remote bool) string {
	if remote {
		clean := path.Clean(p)
		if clean == "/" || clean == "." {
			return "/"
		}
		return path.Dir(clean)
	}
	clean := filepath.Clean(p)
	parent := filepath.Dir(clean)
	if parent == clean {
		return parent
	}
	return parent
}

// copyActive kicks off a file copy from the active pane to the other.
// Returns the cmd that starts the goroutine + the first progress tick.
// Direction is implicit: active pane = source.
func (m *fileBrowserModel) copyActive() tea.Cmd {
	if m.transfer != nil {
		return nil
	}
	e := m.activeEntry()
	if e == nil || e.IsDir || e.Name == ".." {
		m.status = "select a file to copy"
		return nil
	}
	client := m.sftpClient()
	if client == nil {
		m.status = "sftp client unavailable"
		return nil
	}

	var (
		srcSide = m.active
		dstSide fileSide
		srcPath string
		dstPath string
		dstDir  string
	)
	if srcSide == sideLocal {
		dstSide = sideRemote
		srcPath = filepath.Join(m.localPath, e.Name)
		dstDir = m.remotePath
		dstPath = path.Join(dstDir, e.Name)
	} else {
		dstSide = sideLocal
		srcPath = path.Join(m.remotePath, e.Name)
		dstDir = m.localPath
		dstPath = filepath.Join(dstDir, e.Name)
	}

	ctx, cancel := context.WithCancel(context.Background())
	written := &atomic.Int64{}
	m.transfer = &transferState{
		name:    e.Name,
		total:   e.Size,
		written: written,
		started: time.Now(),
		cancel:  cancel,
	}

	return tea.Batch(
		runTransfer(ctx, client, srcSide, srcPath, dstPath, e.Size, written, dstSide),
		tickTransfer(),
	)
}

// runTransfer streams srcPath → dstPath and returns a transferDoneMsg.
// Direction inferred from srcSide: sideLocal = upload (file → sftp),
// sideRemote = download (sftp → file). The ctx aborts when Esc is
// pressed mid-transfer (m.transfer.cancel is the func()).
func runTransfer(ctx context.Context, client *sftp.Client, srcSide fileSide, srcPath, dstPath string, size int64, written *atomic.Int64, dstSide fileSide) tea.Cmd {
	return func() tea.Msg {
		started := time.Now()
		var (
			src io.ReadCloser
			dst io.WriteCloser
			err error
		)
		if srcSide == sideLocal {
			src, err = os.Open(srcPath)
			if err != nil {
				return transferDoneMsg{name: filepath.Base(srcPath), err: err, dest: dstSide}
			}
			defer src.Close()
			dst, err = client.Create(dstPath)
			if err != nil {
				return transferDoneMsg{name: filepath.Base(srcPath), err: err, dest: dstSide}
			}
			defer dst.Close()
		} else {
			src, err = client.Open(srcPath)
			if err != nil {
				return transferDoneMsg{name: path.Base(srcPath), err: err, dest: dstSide}
			}
			defer src.Close()
			dst, err = os.Create(dstPath)
			if err != nil {
				return transferDoneMsg{name: path.Base(srcPath), err: err, dest: dstSide}
			}
			defer dst.Close()
		}

		reader := &progressReader{src: src, written: written, ctx: ctx}
		copied, copyErr := io.Copy(dst, reader)
		_ = copied
		return transferDoneMsg{
			name:    filepath.Base(srcPath),
			bytes:   size,
			elapsed: time.Since(started),
			err:     copyErr,
			dest:    dstSide,
		}
	}
}

// progressReader wraps the source so io.Copy updates `written` after each
// chunk. The ctx check inserts a cancellation point between chunks —
// Read returns ctx.Err() once cancel is invoked.
type progressReader struct {
	src     io.Reader
	written *atomic.Int64
	ctx     context.Context
}

func (p *progressReader) Read(buf []byte) (int, error) {
	select {
	case <-p.ctx.Done():
		return 0, p.ctx.Err()
	default:
	}
	n, err := p.src.Read(buf)
	if n > 0 {
		p.written.Add(int64(n))
	}
	return n, err
}

// transferProgressInterval is how often the progress line redraws while a
// copy runs. Fast enough to feel responsive; slow enough not to burn
// CPU on render thrash.
const transferProgressInterval = 200 * time.Millisecond

// tickTransfer fires a transferProgressTickMsg every interval. App.update
// re-schedules it until the transfer is done.
func tickTransfer() tea.Cmd {
	return tea.Tick(transferProgressInterval, func(time.Time) tea.Msg {
		return transferProgressTickMsg{}
	})
}

// applyTransferDone tucks the result onto the model + clears the
// transfer pointer. Caller re-lists the destination pane.
func (m *fileBrowserModel) applyTransferDone(msg transferDoneMsg) tea.Cmd {
	if m.transfer != nil && m.transfer.cancel != nil {
		m.transfer.cancel() // no-op when ctx already canceled; releases resources
	}
	m.transfer = nil
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.status = fmt.Sprintf("cancelled: %s", msg.name)
		} else {
			m.status = fmt.Sprintf("error: %s — %v", msg.name, msg.err)
		}
		return nil
	}
	m.status = fmt.Sprintf("copied %s (%s in %s)", msg.name,
		formatBytes(msg.bytes), msg.elapsed.Truncate(time.Millisecond))
	if msg.dest == sideLocal {
		return m.listLocal(m.localPath)
	}
	return m.listRemote(m.remotePath)
}

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

// transferState tracks a single in-flight copy. Set on the model by the
// copy action; cleared by the transferDoneMsg handler. View() reads it
// every render to draw the progress line; Esc invokes its cancel func.
type transferState struct {
	name    string
	total   int64
	written *atomic.Int64
	started time.Time
	cancel  func()
	done    bool
	err     error
}

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

