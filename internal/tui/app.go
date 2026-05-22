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
	}
	return nil
}

func (a *App) handleKey(k tea.KeyMsg) tea.Cmd {
	// Ctrl-C always quits.
	if k.Type == tea.KeyCtrlC {
		return tea.Quit
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
		return tea.Quit
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

// openTab connects to a host and adds a new tab.
func (a *App) openTab(host config.Host) tea.Cmd {
	a.statusMsg = ""
	tab := newSessionTab(a.nextID, host, a.width, a.height)
	a.nextID++
	if a.connect != nil {
		bodyH := a.height - tabBarRows
		if bodyH < 1 {
			bodyH = 1
		}
		sess, err := a.connect(host, a.width, bodyH)
		if err != nil {
			a.statusMsg = fmt.Sprintf("connect %s: %v", host.Name, err)
		} else {
			tab.attach(sess)
		}
	}
	a.tabs = append(a.tabs, tab)
	a.active = len(a.tabs) - 1
	a.mode = modeSession
	return nil
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

func (a *App) closeActiveTab() {
	t := a.activeTab()
	if t == nil {
		return
	}
	t.close()
	a.tabs = append(a.tabs[:a.active], a.tabs[a.active+1:]...)
	if a.active >= len(a.tabs) {
		a.active = len(a.tabs) - 1
	}
	if len(a.tabs) == 0 {
		a.mode = modePicker
		a.picker.setQuery("")
	}
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
