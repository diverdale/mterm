package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/config"
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
