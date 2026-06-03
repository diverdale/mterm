package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/appmeta"
	"mterm/internal/config"
	"mterm/internal/diag"
	"mterm/internal/history"
	"mterm/internal/sessionlog"
	mssh "mterm/internal/ssh"
	"mterm/internal/workspaces"
)

// viewMode is the current top-level view.
type viewMode int

const (
	modePicker viewMode = iota
	modeSession
	modeForwards
	modePalette
	modeHelp
	modeWorkspaceSave
	modeFileBrowser
)

// renderInterval is the coalesced redraw cadence (~30 fps).
const renderInterval = 33 * time.Millisecond

// tickMsg drives periodic re-renders so session output is coalesced.
type tickMsg struct{}

// connectedMsg delivers the result of an asynchronous connection attempt.
type connectedMsg struct {
	tabID int
	sess  *mssh.Session
	err   error
}

// Connector turns a host into a connected session. main.go supplies the real
// implementation; tests may pass nil and avoid connecting.
type Connector func(config.Host, int, int) (*mssh.Session, error)

// App is the root Bubble Tea model.
type App struct {
	connect Connector

	mode          viewMode
	prefixPending bool

	picker        *picker
	forwards      *forwardsPanel
	palette       *paletteModel
	help          *helpModel
	workspaceSave *workspaceSaveModel
	fileBrowser   *fileBrowserModel
	prevMode      viewMode
	tabs          []*sessionTab
	active        int
	nextID        int

	// syncTabs holds the IDs of tabs in the broadcast (sync) set. Keyed by
	// tab.id (stable across close/reorder, unlike index). When the active
	// tab is in this set, every plain keystroke fans out to every other
	// tab in the set as well.
	syncTabs map[int]bool

	width, height int
	statusMsg     string
	tickCount     int    // render-tick counter, drives status spinners
	logRoot       string // root dir for per-session logs; "" disables logging

	// history tracks the last successful connect time per host. Optional —
	// nil disables the connection-history decorations in the picker.
	history *history.Store

	// workspaces persists named tab sets. Optional — nil hides the
	// workspace commands from the palette.
	workspaces *workspaces.Store
}

// NewApp builds the root model.
func NewApp(hosts []config.Host, connect Connector) *App {
	return &App{
		connect:  connect,
		mode:     modePicker,
		picker:   newPicker(hosts),
		nextID:   1,
		syncTabs: map[int]bool{},
	}
}

// toggleActiveTabSync flips the active tab's membership in the broadcast
// set. No-op when there is no active tab.
func (a *App) toggleActiveTabSync() {
	t := a.activeTab()
	if t == nil {
		return
	}
	if a.syncTabs[t.id] {
		delete(a.syncTabs, t.id)
	} else {
		a.syncTabs[t.id] = true
	}
}

// tabInSync reports whether the given tab ID is in the broadcast set.
func (a *App) tabInSync(id int) bool { return a.syncTabs[id] }

// clearSyncSet empties the broadcast set.
func (a *App) clearSyncSet() { a.syncTabs = map[int]bool{} }

// SetLogRoot sets the per-session log directory. Empty disables logging for
// every tab regardless of host opt-in. Call before Run.
func (a *App) SetLogRoot(path string) { a.logRoot = path }

// SetHistory attaches a connection-history store. nil disables the picker's
// last-connected decorations and the on-connect record. Call before Run.
func (a *App) SetHistory(h *history.Store) { a.history = h }

// SetWorkspaces attaches a workspace store. nil hides the workspace
// commands from the palette. Call before Run.
func (a *App) SetWorkspaces(w *workspaces.Store) { a.workspaces = w }

// Init starts the render ticker.
func (a *App) Init() tea.Cmd {
	return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// Update is the Bubble Tea entry point; it delegates to update.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return a, a.update(msg)
}

// update handles one message and returns a command. Split out so tests can call
// it directly.
func (a *App) update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tickMsg:
		a.tickCount++
		a.reapEndedTabs()
		return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })

	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		for _, t := range a.tabs {
			t.resize(m.Width, m.Height)
		}
		return nil

	case tea.KeyMsg:
		return a.handleKey(m)

	case tea.MouseMsg:
		return a.handleMouse(m)

	case pickerChosenMsg:
		return a.openTab(m.host)

	case pickerCancelledMsg:
		if len(a.tabs) > 0 {
			a.mode = modeSession
		}
		return nil

	case forwardsClosedMsg:
		a.mode = modeSession
		return nil

	case paletteChosenMsg:
		// Run the action, then restore prevMode ONLY if the action did not
		// itself change the mode. Several palette commands (Open picker,
		// Open forwards panel, Switch tab, Connect host) intentionally
		// transition to a new mode; unconditionally restoring prevMode
		// would silently undo those.
		before := a.mode
		var cmd tea.Cmd
		if m.cmd.action != nil {
			cmd = m.cmd.action(a)
		}
		if a.mode == before {
			a.mode = a.prevMode
		}
		return cmd

	case paletteClosedMsg:
		a.mode = a.prevMode
		return nil

	case helpClosedMsg:
		a.mode = a.prevMode
		return nil

	case workspaceSaveSubmitMsg:
		a.mode = a.prevMode
		if a.workspaces == nil {
			a.statusMsg = "workspaces store unavailable"
			return nil
		}
		hosts := make([]string, 0, len(a.tabs))
		for _, t := range a.tabs {
			hosts = append(hosts, t.host.Name)
		}
		if err := a.workspaces.Save(m.name, hosts); err != nil {
			a.statusMsg = fmt.Sprintf("workspace save: %v", err)
		} else {
			a.statusMsg = fmt.Sprintf("workspace saved: %s (%d tabs)", m.name, len(hosts))
		}
		return nil

	case workspaceSaveCancelMsg:
		a.mode = a.prevMode
		return nil

	case fileBrowserClosedMsg:
		a.fileBrowser = nil
		a.mode = a.prevMode
		return nil

	case fileBrowserRefreshMsg:
		if a.fileBrowser == nil {
			return nil
		}
		return tea.Batch(
			a.fileBrowser.listLocal(a.fileBrowser.localPath),
			a.fileBrowser.listRemote(a.fileBrowser.remotePath),
		)

	case localListedMsg:
		if a.fileBrowser != nil {
			a.fileBrowser.applyLocalListing(m)
		}
		return nil

	case remoteListedMsg:
		if a.fileBrowser != nil {
			a.fileBrowser.applyRemoteListing(m)
		}
		return nil

	case transferProgressTickMsg:
		// Schedule the next tick as long as a transfer is in progress;
		// the View() reads the atomic counter on every render so we
		// don't actually carry data in the tick message itself.
		if a.fileBrowser == nil || a.fileBrowser.transfer == nil {
			return nil
		}
		return tickTransfer()

	case transferDoneMsg:
		if a.fileBrowser == nil {
			return nil
		}
		return a.fileBrowser.applyTransferDone(m)

	case connectedMsg:
		a.handleConnected(m)
		return nil
	}
	return nil
}

func (a *App) handleKey(k tea.KeyMsg) tea.Cmd {
	// Modal overlays consume every key as their own input — the prefix
	// machine and mode dispatch below do not apply.
	switch a.mode {
	case modePalette:
		return a.palette.Update(k)
	case modeHelp:
		return a.help.Update(k)
	case modeWorkspaceSave:
		return a.workspaceSave.Update(k)
	case modeFileBrowser:
		return a.fileBrowser.Update(k)
	}

	// Direct tab-switch shortcuts. Active only in session mode with >1
	// tab open so they don't surprise picker/forwards users. Ctrl-Left/
	// Right are the "what the user asked for" pair (note: they shadow
	// shell word-jump on the remote — use Option-Left/Right on macOS
	// for word-jump instead); Ctrl-PgUp/PgDn are the no-conflict iTerm2
	// / browser convention.
	if a.mode == modeSession && len(a.tabs) > 1 {
		switch k.Type {
		case tea.KeyCtrlLeft, tea.KeyCtrlPgUp:
			a.cycleTab(-1)
			return nil
		case tea.KeyCtrlRight, tea.KeyCtrlPgDown:
			a.cycleTab(1)
			return nil
		}
	}

	// Prefix state machine is global to picker / session / forwards so
	// ^B : (palette) and ^B ? (help) work from anywhere.
	if a.prefixPending {
		a.prefixPending = false
		return a.handleCommandKey(k)
	}
	if isPrefixKey(k) {
		a.prefixPending = true
		return nil
	}

	// Non-prefix keys go to the mode's own model.
	switch a.mode {
	case modePicker:
		return a.picker.Update(k)
	case modeForwards:
		return a.forwards.Update(k)
	case modeSession:
		t := a.activeTab()
		if t == nil {
			return nil
		}
		// Typing snaps the view back to live so output from the remote
		// doesn't keep landing invisibly below the user's frozen
		// scrollback view.
		t.snapToLive()
		bytes := keyToBytes(k)
		t.sendInput(bytes)
		// If the active tab is in the broadcast set, fan the same bytes
		// out to every other tab in the set. Prefix keys never reach
		// this branch (handled above), so broadcast is keystroke-only.
		if a.syncTabs[t.id] {
			for _, other := range a.tabs {
				if other.id != t.id && a.syncTabs[other.id] {
					other.sendInput(bytes)
				}
			}
		}
		return nil
	}
	return nil
}

// handleCommandKey runs a prefix-key command.
func (a *App) handleCommandKey(k tea.KeyMsg) tea.Cmd {
	switch keyString(k) {
	case "c":
		a.picker.setQuery("")
		a.mode = modePicker
	case "n":
		a.cycleTab(1)
	case "p":
		a.cycleTab(-1)
	case ":":
		a.prevMode = a.mode
		a.palette = newPaletteModel(buildCommands(a))
		a.mode = modePalette
	case "?":
		a.prevMode = a.mode
		a.help = newHelpModel()
		a.mode = modeHelp
	case "x":
		a.closeActiveTab()
	case "f":
		if t := a.activeTab(); t != nil {
			a.forwards = newForwardsPanel(t.host)
			a.mode = modeForwards
		}
	case "u":
		if t := a.activeTab(); t != nil {
			a.prevMode = a.mode
			a.fileBrowser = newFileBrowserModel(t)
			a.mode = modeFileBrowser
			return a.fileBrowser.Init()
		}
		a.statusMsg = "open a session first"
	case "s":
		a.toggleActiveTabSync()
	case "D":
		// Diagnostic: dump every goroutine's stack to /tmp and surface the
		// path in the footer. Useful when the snapshot worker for one tab
		// freezes — `^B D` from another (still-responsive) tab captures
		// the wedged tab's stack without needing a second terminal.
		if path, err := diag.DumpGoroutines(appmeta.DirName); err != nil {
			a.statusMsg = fmt.Sprintf("dump failed: %v", err)
		} else {
			a.statusMsg = "dump: " + path
		}
	case "q":
		return a.shutdown()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(k.Runes[0] - '1')
		a.setActive(idx)
	}
	return nil
}

// wheelStep is how many lines a single wheel-tick scrolls the view. Matches
// the typical "3 lines per notch" terminal convention so the feel is close
// to scrolling outside mterm.
const wheelStep = 3

// handleMouse routes mouse events. Today only wheel-up / wheel-down do
// anything — they scroll the active session tab's scrollback offset.
// Clicks and motion are ignored; if a future feature wants them (e.g.
// mouse passthrough to remote vim) it'll plug in here.
func (a *App) handleMouse(m tea.MouseMsg) tea.Cmd {
	if a.mode != modeSession {
		return nil
	}
	t := a.activeTab()
	if t == nil {
		return nil
	}
	switch m.Button {
	case tea.MouseButtonWheelUp:
		t.scrollBy(wheelStep)
	case tea.MouseButtonWheelDown:
		t.scrollBy(-wheelStep)
	}
	return nil
}

// keyString renders a command key as a comparable string.
func keyString(k tea.KeyMsg) string {
	if k.Type == tea.KeyRunes && len(k.Runes) == 1 {
		return string(k.Runes)
	}
	return ""
}

// openTab creates a tab for host and kicks off an async connection.
func (a *App) openTab(host config.Host) tea.Cmd {
	a.statusMsg = ""
	id := a.nextID
	a.nextID++
	tab := newSessionTab(id, host, a.width, a.height)
	a.tabs = append(a.tabs, tab)
	a.setActive(len(a.tabs) - 1)
	a.mode = modeSession
	if a.connect == nil {
		return nil
	}
	connect := a.connect
	bodyW, bodyH := h2body(a.width, a.height)
	return func() tea.Msg {
		sess, err := connect(host, bodyW, bodyH)
		return connectedMsg{tabID: id, sess: sess, err: err}
	}
}

func (a *App) activeTab() *sessionTab {
	if a.active < 0 || a.active >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.active]
}

func (a *App) cycleTab(delta int) {
	if len(a.tabs) == 0 {
		return
	}
	a.setActive((a.active + delta + len(a.tabs)) % len(a.tabs))
}

// setActive switches focus to tab idx and marks it caught-up on activity so
// its activity badge clears. Bounds-checked; out-of-range idx is a no-op.
// Centralizes the marking so every code path that changes the active tab
// gets the badge-clear behavior for free.
func (a *App) setActive(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	a.active = idx
	a.tabs[idx].markSeen()
}

// removeTabAt closes and removes the tab at index idx.
func (a *App) removeTabAt(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	delete(a.syncTabs, a.tabs[idx].id)
	a.tabs[idx].close()
	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	if a.active >= len(a.tabs) {
		a.setActive(len(a.tabs) - 1)
	}
	if len(a.tabs) == 0 {
		a.mode = modePicker
		a.picker.setQuery("")
	}
}

func (a *App) closeActiveTab() {
	if a.activeTab() == nil {
		return
	}
	a.removeTabAt(a.active)
}

// reapEndedTabs removes any tab whose remote session has finished, so a tab
// closes automatically when its shell exits.
func (a *App) reapEndedTabs() {
	for i := 0; i < len(a.tabs); {
		if a.tabs[i].hasEnded() {
			a.removeTabAt(i)
		} else {
			i++
		}
	}
}

// shutdown closes every open session before ending the program.
func (a *App) shutdown() tea.Cmd {
	for _, t := range a.tabs {
		t.close()
	}
	a.tabs = nil
	return tea.Quit
}

// handleConnected applies the result of an async connection attempt.
func (a *App) handleConnected(m connectedMsg) {
	idx := -1
	for i, t := range a.tabs {
		if t.id == m.tabID {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Tab was closed before the connection resolved — don't leak it.
		if m.sess != nil {
			m.sess.Close()
		}
		return
	}
	if m.err != nil {
		a.statusMsg = fmt.Sprintf("connect %s: %v", a.tabs[idx].host.Name, m.err)
		a.removeTabAt(idx)
		return
	}
	// Stamp the successful connect into the history store so the picker
	// can show "last connected" decorations. Failures don't update history
	// (we only count actual sessions). Errors are silent — losing one
	// history write isn't worth crashing the connect path over.
	if a.history != nil {
		_ = a.history.RecordConnect(a.tabs[idx].host.Name, time.Now())
	}
	a.openLoggerForTab(a.tabs[idx])
	a.tabs[idx].attach(m.sess)
}

// openLoggerForTab tries to open a session log for the tab. Errors surface to
// statusMsg but never block the connection — logging is best-effort.
func (a *App) openLoggerForTab(t *sessionTab) {
	if a.logRoot == "" || !t.host.Logging() {
		return
	}
	path := sessionlog.PathFor(a.logRoot, t.host.Name, time.Now())
	lg, err := sessionlog.Open(path)
	if err != nil {
		a.statusMsg = fmt.Sprintf("log %s: %v", t.host.Name, err)
		return
	}
	t.setLogger(lg)
}

// View renders the current view as a framed window.
func (a *App) View() string {
	if a.width < 1 || a.height < 1 {
		return ""
	}
	switch a.mode {
	case modeForwards:
		return a.forwardsView()
	case modeSession:
		return a.sessionView()
	case modePalette:
		a.palette.setSize(a.width, a.height)
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.palette.View())
	case modeHelp:
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.help.View())
	case modeWorkspaceSave:
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.workspaceSave.View())
	case modeFileBrowser:
		return a.fileBrowserView()
	default:
		return a.pickerView()
	}
}

// fmtUptime formats a duration as HH:MM:SS.
func fmtUptime(d time.Duration) string {
	s := int(d.Seconds())
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s/60)%60, s%60)
}

// pickerDecorations gathers per-render context the picker uses to render
// host rows — connection history (from the persisted store) and the set of
// hosts currently in an open tab (derived from a.tabs in-memory).
// fileBrowserView frames the active tab's file browser inside the standard
// window chrome so the visual surface matches every other top-level mode
// (picker, session, forwards). The file browser model handles its own
// internal two-pane layout.
func (a *App) fileBrowserView() string {
	t := a.activeTab()
	if t == nil || a.fileBrowser == nil {
		// Defensive — open path should have rejected this combo.
		a.mode = a.prevMode
		return a.View()
	}
	// Reserve the same chrome as the session view: top border,
	// tab strip, divider, [body], divider, footer, bottom border = 6 rows.
	bodyW := a.width - chromeCols
	bodyH := a.height - chromeRows
	if bodyW < 10 {
		bodyW = 10
	}
	if bodyH < 5 {
		bodyH = 5
	}
	a.fileBrowser.setSize(bodyW, bodyH)
	s := chromeStylesFor(t.host.BorderColor)
	title := s.title.Render(appmeta.Name) + statusCount(len(a.tabs)) + statusMsgBadge(a.statusMsg)
	tabs := renderTabStrip(a.tabs, a.active, a.tickCount, a.width-2, s, a.syncTabs)
	footer := renderFooter(footerOpts{
		hints: []keyHint{{"Tab", "switch"}, {"F5", "copy"}, {"r", "refresh"}, {"Esc", "close"}},
		info:  sessionInfo(t.host),
		width: a.width - 2,
	}, s)
	return renderWindow(windowOpts{
		title:    title,
		tabStrip: tabs,
		body:     a.fileBrowser.View(),
		footer:   footer,
		width:    a.width,
		height:   a.height,
	}, s)
}

func (a *App) pickerDecorations() pickerDecorations {
	deco := pickerDecorations{Now: time.Now()}
	if a.history != nil {
		deco.LastConnected = a.history.All()
	}
	if len(a.tabs) > 0 {
		open := make(map[string]bool, len(a.tabs))
		for _, t := range a.tabs {
			open[t.host.Name] = true
		}
		deco.OpenHosts = open
	}
	return deco
}

func (a *App) pickerView() string {
	a.picker.setSize(a.width-chromeCols, a.height-4)
	a.picker.setDecorations(a.pickerDecorations())
	footer := renderFooter(footerOpts{
		hints: []keyHint{{"enter", "connect"}, {"type", "filter"}, {"esc", "back"}},
		info:  fmt.Sprintf("%d hosts", len(a.picker.all)),
		width: a.width - 2,
	}, sty)
	body := a.picker.View()
	if a.statusMsg != "" {
		body += "\n" + sty.errorText.Render("  "+a.statusMsg)
	}
	return renderWindow(windowOpts{
		title:  sty.title.Render(appmeta.Name),
		body:   body,
		footer: footer,
		width:  a.width,
		height: a.height,
	}, sty)
}

func (a *App) sessionView() string {
	t := a.activeTab()
	if t == nil {
		return a.pickerView()
	}
	s := chromeStylesFor(t.host.BorderColor)
	off, _ := t.scrollPos()
	title := s.title.Render(appmeta.Name) + statusCount(len(a.tabs)) + syncCount(len(a.syncTabs)) + scrollIndicator(off) + statusMsgBadge(a.statusMsg)
	tabs := renderTabStrip(a.tabs, a.active, a.tickCount, a.width-2, s, a.syncTabs)
	footer := renderFooter(footerOpts{
		hints:         []keyHint{{"^B", "menu"}, {"^B n", "next"}, {"^B x", "close"}},
		info:          sessionInfo(t.host),
		timer:         fmtUptime(t.uptime()),
		prefixPending: a.prefixPending,
		width:         a.width - 2,
	}, s)
	return renderWindow(windowOpts{
		title:    title,
		tabStrip: tabs,
		body:     t.View(),
		footer:   footer,
		width:    a.width,
		height:   a.height,
	}, s)
}

func (a *App) forwardsView() string {
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
		a.forwards.View())
}

// statusCount renders the " [N tabs]" suffix for the window title.
func statusCount(n int) string {
	noun := "tabs"
	if n == 1 {
		noun = "tab"
	}
	return sty.titleDim.Render(fmt.Sprintf("  [%d %s]", n, noun))
}

// syncCount renders a warning-color " [sync N]" suffix when the broadcast set
// is non-empty. Empty returns "" so non-sync windows look unchanged.
func syncCount(n int) string {
	if n == 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(active.Warning).Bold(true).
		Render(fmt.Sprintf("  [sync %d]", n))
}

// scrollIndicator renders a " [scroll N]" suffix when the user is looking
// back into scrollback so it's obvious the view is frozen. Hidden when at
// live (offset 0). Uses the warning color to discourage stale-view confusion.
func scrollIndicator(offset int) string {
	if offset <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(active.Warning).Bold(true).
		Render(fmt.Sprintf("  [scroll %d]", offset))
}

// statusMsgBadge renders a warning-color " · <msg>" suffix in the title
// bar so transient feedback (palette command results, log paths, restore
// summaries, etc.) is visible in session mode — the picker view appends
// statusMsg to the body, but the session body is the live VT and can't
// be touched. Truncated to keep the title from blowing out the frame.
func statusMsgBadge(msg string) string {
	if msg == "" {
		return ""
	}
	const maxLen = 60
	runes := []rune(msg)
	if len(runes) > maxLen {
		msg = string(runes[:maxLen-1]) + "…"
	}
	return lipgloss.NewStyle().Foreground(active.Warning).Render("  · " + msg)
}

// sessionInfo renders user@host:port for the footer.
func sessionInfo(h config.Host) string {
	user := h.User
	if user == "" {
		user = "?"
	}
	return fmt.Sprintf("%s@%s:%d", user, h.HostName, h.Port)
}
