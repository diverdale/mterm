package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
)

// viewMode is the current top-level view.
type viewMode int

const (
	modePicker viewMode = iota
	modeSession
	modeForwards
	modePalette
	modeHelp
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

	picker   *picker
	forwards *forwardsPanel
	palette  *paletteModel
	help     *helpModel
	prevMode viewMode
	tabs     []*sessionTab
	active   int
	nextID   int

	width, height int
	statusMsg     string
	tickCount     int // render-tick counter, drives status spinners
}

// NewApp builds the root model.
func NewApp(hosts []config.Host, connect Connector) *App {
	return &App{
		connect: connect,
		mode:    modePicker,
		picker:  newPicker(hosts),
		nextID:  1,
	}
}

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

	case connectedMsg:
		a.handleConnected(m)
		return nil
	}
	return nil
}

func (a *App) handleKey(k tea.KeyMsg) tea.Cmd {
	// Ctrl-C always quits.
	if k.Type == tea.KeyCtrlC {
		return a.shutdown()
	}

	switch a.mode {
	case modePicker:
		return a.picker.Update(k)

	case modeForwards:
		return a.forwards.Update(k)

	case modePalette:
		return a.palette.Update(k)

	case modeHelp:
		return a.help.Update(k)

	case modeSession:
		if a.prefixPending {
			a.prefixPending = false
			return a.handleCommandKey(k)
		}
		if isPrefixKey(k) {
			a.prefixPending = true
			return nil
		}
		if t := a.activeTab(); t != nil {
			t.sendInput(keyToBytes(k))
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
	case "q":
		return a.shutdown()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(k.Runes[0] - '1')
		if idx < len(a.tabs) {
			a.active = idx
		}
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
	a.active = len(a.tabs) - 1
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
	a.active = (a.active + delta + len(a.tabs)) % len(a.tabs)
}

// removeTabAt closes and removes the tab at index idx.
func (a *App) removeTabAt(idx int) {
	if idx < 0 || idx >= len(a.tabs) {
		return
	}
	a.tabs[idx].close()
	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
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
	a.tabs[idx].attach(m.sess)
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
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.palette.View())
	case modeHelp:
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.help.View())
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

func (a *App) pickerView() string {
	a.picker.setSize(a.width-chromeCols, a.height-4)
	footer := renderFooter(footerOpts{
		hints: []keyHint{{"enter", "connect"}, {"type", "filter"}, {"esc", "back"}},
		info:  fmt.Sprintf("%d hosts", len(a.picker.all)),
		width: a.width - 2,
	})
	body := a.picker.View()
	if a.statusMsg != "" {
		body += "\n" + sty.errorText.Render("  "+a.statusMsg)
	}
	return renderWindow(windowOpts{
		title:  sty.title.Render("mterm"),
		body:   body,
		footer: footer,
		width:  a.width,
		height: a.height,
	})
}

func (a *App) sessionView() string {
	t := a.activeTab()
	if t == nil {
		return a.pickerView()
	}
	title := sty.title.Render("mterm") + statusCount(len(a.tabs))
	tabs := renderTabStrip(a.tabs, a.active, a.tickCount, a.width-2)
	footer := renderFooter(footerOpts{
		hints:         []keyHint{{"^B", "menu"}, {"^B n", "next"}, {"^B x", "close"}},
		info:          sessionInfo(t.host),
		timer:         fmtUptime(t.uptime()),
		prefixPending: a.prefixPending,
		width:         a.width - 2,
	})
	return renderWindow(windowOpts{
		title:    title,
		tabStrip: tabs,
		body:     t.View(),
		footer:   footer,
		width:    a.width,
		height:   a.height,
	})
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

// sessionInfo renders user@host:port for the footer.
func sessionInfo(h config.Host) string {
	user := h.User
	if user == "" {
		user = "?"
	}
	return fmt.Sprintf("%s@%s:%d", user, h.HostName, h.Port)
}
