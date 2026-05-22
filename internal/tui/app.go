package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
)

// viewMode is the current top-level view.
type viewMode int

const (
	modePicker viewMode = iota
	modeSession
	modeForwards
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
	tabs     []*sessionTab
	active   int
	nextID   int

	width, height int
	statusMsg     string
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
		return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })

	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.picker.setSize(m.Width, m.Height)
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
	w, bodyH := a.width, a.height-tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	return func() tea.Msg {
		sess, err := connect(host, w, bodyH)
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

// View renders the current view.
func (a *App) View() string {
	switch a.mode {
	case modePicker:
		return a.picker.View()
	case modeForwards:
		return a.forwards.View()
	case modeSession:
		return a.sessionView()
	}
	return ""
}

func (a *App) sessionView() string {
	t := a.activeTab()
	if t == nil {
		return "no active session"
	}
	return a.tabBar() + "\n" + t.View()
}

func (a *App) tabBar() string {
	var parts []string
	for i, t := range a.tabs {
		label := fmt.Sprintf("%d:%s", i+1, t.title())
		if i == a.active {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabInactive.Render(label))
		}
	}
	bar := strings.Join(parts, " ")
	if a.prefixPending {
		bar += "  " + statusBar.Render("[prefix]")
	}
	if a.statusMsg != "" {
		bar += "  " + errorText.Render(a.statusMsg)
	}
	return bar
}
