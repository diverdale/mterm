package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	if w != 100-chromeCols {
		t.Fatalf("term width = %d, want %d", w, 100-chromeCols)
	}
	if h != 40-chromeRows {
		t.Fatalf("term height = %d, want %d (40 - %d chrome rows)", h, 40-chromeRows, chromeRows)
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

func TestAppPickerViewShowsConnectError(t *testing.T) {
	failConnect := func(config.Host, int, int) (*mssh.Session, error) {
		return nil, errors.New("kaboom")
	}
	app := NewApp(sampleHosts(), failConnect)
	app.width, app.height = 80, 24

	// A failed connect removes the only tab and returns to the picker.
	cmd := app.update(pickerChosenMsg{host: sampleHosts()[0]})
	app.update(cmd())

	if app.mode != modePicker {
		t.Fatalf("mode = %v, want modePicker after failed connect", app.mode)
	}
	if !strings.Contains(app.View(), "kaboom") {
		t.Fatalf("picker View must surface the connect error, got:\n%s", app.View())
	}
}

func TestAppReapsEndedTabs(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 1
	app.mode = modeSession

	// The first tab's remote shell exited.
	app.tabs[0].ended.Store(true)
	app.update(tickMsg{})
	if len(app.tabs) != 1 {
		t.Fatalf("ended tab not reaped: len(tabs) = %d, want 1", len(app.tabs))
	}
	if app.tabs[0].title() != "b" {
		t.Fatalf("wrong tab survived reaping: %q", app.tabs[0].title())
	}

	// When the last tab ends, the app returns to the picker.
	app.tabs[0].ended.Store(true)
	app.update(tickMsg{})
	if len(app.tabs) != 0 || app.mode != modePicker {
		t.Fatalf("after last tab ends: tabs=%d mode=%v, want 0/modePicker", len(app.tabs), app.mode)
	}
}

func TestAppViewIsFramed(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	out := app.View() // picker mode
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("framed picker view has %d lines, want 24", len(lines))
	}
	if !strings.Contains(lines[0], "┌") {
		t.Fatalf("picker view is not framed; line 0 = %q", lines[0])
	}
	if !strings.Contains(out, "mterm") {
		t.Fatal("picker frame title should say mterm")
	}
}

func TestAppSpinnerCounterAdvances(t *testing.T) {
	app := newTestApp()
	before := app.tickCount
	app.update(tickMsg{})
	if app.tickCount != before+1 {
		t.Fatalf("tickCount = %d, want %d", app.tickCount, before+1)
	}
}

func TestFmtUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00:00"},
		{90 * time.Second, "00:01:30"},
		{3661 * time.Second, "01:01:01"},
	}
	for _, c := range cases {
		if got := fmtUptime(c.d); got != c.want {
			t.Fatalf("fmtUptime(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestAppSessionViewIsFramed(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "h"}, 80, 24)}
	app.active = 0
	app.mode = modeSession

	out := app.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("framed session view has %d lines, want 24", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 80 {
			t.Fatalf("session view line %d width = %d, want 80", i, w)
		}
	}
}

func TestAppPrefixPOpensPalette(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	if app.mode != modePalette {
		t.Fatalf("mode = %v, want modePalette", app.mode)
	}
	if app.palette == nil {
		t.Fatal("palette is nil")
	}
	if app.prevMode != modeSession {
		t.Fatalf("prevMode = %v, want modeSession", app.prevMode)
	}
}

func TestAppPrefixQuestionOpensHelp(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if app.mode != modeHelp {
		t.Fatalf("mode = %v, want modeHelp", app.mode)
	}
	if app.help == nil {
		t.Fatal("help is nil")
	}
}

func TestAppPaletteClosedRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modePalette
	app.prevMode = modeSession
	app.update(paletteClosedMsg{})
	if app.mode != modeSession {
		t.Fatalf("after paletteClosedMsg, mode = %v, want modeSession", app.mode)
	}
}

func TestAppHelpClosedRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modeHelp
	app.prevMode = modePicker
	app.update(helpClosedMsg{})
	if app.mode != modePicker {
		t.Fatalf("after helpClosedMsg, mode = %v, want modePicker", app.mode)
	}
}

func TestAppPaletteChosenRunsActionAndRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modePalette
	app.prevMode = modeSession

	ran := false
	c := command{label: "test", group: "G", action: func(a *App) tea.Cmd {
		ran = true
		return nil
	}}
	app.update(paletteChosenMsg{cmd: c})

	if !ran {
		t.Fatal("command action did not run")
	}
	if app.mode != modeSession {
		t.Fatalf("after running command, mode = %v, want modeSession", app.mode)
	}
}

func TestReloadHostsKeepsPickerNonNil(t *testing.T) {
	app := newTestApp()
	reloadHosts(app)
	if app.picker == nil {
		t.Fatal("picker became nil after reloadHosts")
	}
}
