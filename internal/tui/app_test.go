package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
	mssh "mterm/internal/ssh"
)

func newTestApp() *App {
	return NewApp(sampleHosts(), nil)
}

func TestAppStartsInPickerMode(t *testing.T) {
	app := newTestApp()
	if app.mode != modePicker {
		t.Fatalf("mode = %v, want modePicker", app.mode)
	}
}

func TestAppPrefixThenNextTabSwitchesTab(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 0
	app.mode = modeSession

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if !app.prefixPending {
		t.Fatal("Ctrl-B must set prefixPending")
	}
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.active != 1 {
		t.Fatalf("active tab = %d, want 1", app.active)
	}
	if app.prefixPending {
		t.Fatal("prefixPending must clear after the command key")
	}
}

func TestAppPlainKeyInSessionModeIsNotACommand(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	app.mode = modeSession

	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if app.prefixPending {
		t.Fatal("a plain key must not set prefixPending")
	}
}

func TestAppPrefixCQuitWithNoTabs(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.prefixPending = true
	cmd := app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("prefix-q with no live tabs should return a quit command")
	}
}

func prefix(app *App) {
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
}

func TestAppCloseActiveTab(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
		newSessionTab(3, config.Host{Name: "c"}, 80, 24),
	}
	app.mode = modeSession
	app.active = 1

	// Close the middle tab.
	prefix(app)
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if len(app.tabs) != 2 {
		t.Fatalf("len(tabs) = %d, want 2 after first close", len(app.tabs))
	}
	if app.active < 0 || app.active >= len(app.tabs) {
		t.Fatalf("active = %d out of range after first close (len=%d)", app.active, len(app.tabs))
	}

	// Close another tab.
	prefix(app)
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if len(app.tabs) != 1 {
		t.Fatalf("len(tabs) = %d, want 1 after second close", len(app.tabs))
	}

	// Close the last tab — should return to picker.
	prefix(app)
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if len(app.tabs) != 0 {
		t.Fatalf("len(tabs) = %d, want 0 after final close", len(app.tabs))
	}
	if app.mode != modePicker {
		t.Fatalf("mode = %v, want modePicker after closing all tabs", app.mode)
	}
}

func TestAppWindowResizePropagates(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.mode = modeSession
	app.active = 0

	app.update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if app.width != 100 {
		t.Fatalf("app.width = %d, want 100", app.width)
	}
	if app.height != 40 {
		t.Fatalf("app.height = %d, want 40", app.height)
	}
	w, h := app.tabs[0].term.Size()
	if w != 100 {
		t.Fatalf("term width = %d, want 100", w)
	}
	if h != 39 {
		t.Fatalf("term height = %d, want 39 (40 - 1 tab bar row)", h)
	}
}

func TestAppPrefixFOpensForwards(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.mode = modeSession
	app.active = 0

	prefix(app)
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})

	if app.mode != modeForwards {
		t.Fatalf("mode = %v, want modeForwards", app.mode)
	}
	if app.forwards == nil {
		t.Fatal("app.forwards must not be nil after prefix-f")
	}
}

func TestAppOpenTabViaConnector(t *testing.T) {
	called := false
	connect := func(config.Host, int, int) (*mssh.Session, error) {
		called = true
		return nil, nil
	}
	app := NewApp(sampleHosts(), connect)
	app.width, app.height = 80, 24

	// Tab is created synchronously; connection happens in the returned cmd.
	cmd := app.update(pickerChosenMsg{host: sampleHosts()[0]})
	if len(app.tabs) != 1 {
		t.Fatalf("len(tabs) = %d, want 1 after openTab", len(app.tabs))
	}
	if app.mode != modeSession {
		t.Fatalf("mode = %v, want modeSession", app.mode)
	}
	if app.active != 0 {
		t.Fatalf("active = %d, want 0", app.active)
	}
	if cmd == nil {
		t.Fatal("openTab must return a non-nil cmd when a connector is set")
	}

	// Run the cmd to trigger the actual connection.
	msg := cmd()
	if !called {
		t.Fatal("connector was not called")
	}
	cm, ok := msg.(connectedMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want connectedMsg", msg)
	}

	// Deliver the result; tab stays (stub returns nil session, no error).
	app.update(cm)
	if len(app.tabs) != 1 {
		t.Fatalf("len(tabs) = %d, want 1 after connectedMsg", len(app.tabs))
	}
}

func TestAppConnectErrorSetsAndClearsStatus(t *testing.T) {
	// Connector that always fails.
	failConnect := func(config.Host, int, int) (*mssh.Session, error) {
		return nil, errors.New("boom")
	}
	app := NewApp(sampleHosts(), failConnect)
	app.width, app.height = 80, 24

	// Drive the async flow: open → run cmd → deliver connectedMsg.
	cmd := app.update(pickerChosenMsg{host: sampleHosts()[0]})
	msg := cmd()
	app.update(msg)

	if app.statusMsg == "" {
		t.Fatal("statusMsg should be set after a connect error")
	}
	if len(app.tabs) != 0 {
		t.Fatalf("len(tabs) = %d, want 0 after connect error", len(app.tabs))
	}
	if app.mode != modePicker {
		t.Fatalf("mode = %v, want modePicker after connect error removes last tab", app.mode)
	}

	// Now switch to a connector that succeeds; statusMsg should clear on next open.
	app.connect = func(config.Host, int, int) (*mssh.Session, error) {
		return nil, nil
	}
	app.update(pickerChosenMsg{host: sampleHosts()[0]})

	if app.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty after successful open (fix #2)", app.statusMsg)
	}
}
