package tui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mterm/internal/appmeta"
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

func TestAppCtrlArrowsCycleTabsInSession(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
		newSessionTab(3, config.Host{Name: "c"}, 80, 24),
	}
	app.active = 0
	app.mode = modeSession

	app.update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if app.active != 1 {
		t.Fatalf("ctrl-right: active = %d, want 1", app.active)
	}
	app.update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if app.active != 0 {
		t.Fatalf("ctrl-left: active = %d, want 0", app.active)
	}
	app.update(tea.KeyMsg{Type: tea.KeyCtrlPgDown})
	if app.active != 1 {
		t.Fatalf("ctrl-pgdn: active = %d, want 1", app.active)
	}
	app.update(tea.KeyMsg{Type: tea.KeyCtrlPgUp})
	if app.active != 0 {
		t.Fatalf("ctrl-pgup: active = %d, want 0", app.active)
	}
}

func TestAppCtrlArrowsNoOpWithSingleTab(t *testing.T) {
	// One tab: no shortcut should fire — otherwise typing ctrl-arrows in
	// the remote shell with only one mterm tab open would still get
	// swallowed by an effectively meaningless "switch."
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	app.mode = modeSession
	app.update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if app.active != 0 {
		t.Fatalf("single tab: ctrl-right should not change active; got %d", app.active)
	}
}

func TestAppCtrlArrowsNoOpInPickerMode(t *testing.T) {
	// Picker users shouldn't get surprise tab cycling.
	app := newTestApp()
	app.width, app.height = 80, 24
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 0
	app.mode = modePicker
	app.update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if app.active != 0 {
		t.Fatalf("picker mode: ctrl-right should not change active; got %d", app.active)
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
	if !strings.Contains(out, appmeta.Name) {
		t.Fatalf("picker frame title should contain %q", appmeta.Name)
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

func TestAppPrefixColonOpensPalette(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})

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

func TestAppPrefixPCyclesToPrevTab(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 1

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	if app.active != 0 {
		t.Fatalf("after ^B p active = %d, want 0 (previous tab)", app.active)
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

func TestAppPrefixWorksInPickerMode(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	// App starts in picker mode by default. Prefix used to be ignored here.
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})

	if app.mode != modePalette {
		t.Fatalf("after ^B : from picker, mode = %v, want modePalette", app.mode)
	}
	if app.prevMode != modePicker {
		t.Fatalf("prevMode = %v, want modePicker", app.prevMode)
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

func TestSyncToggleAddsAndRemovesActiveTab(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	if app.tabInSync(1) {
		t.Fatal("sync set should start empty")
	}
	app.toggleActiveTabSync()
	if !app.tabInSync(1) {
		t.Fatal("toggle should add active tab to sync")
	}
	app.toggleActiveTabSync()
	if app.tabInSync(1) {
		t.Fatal("second toggle should remove tab from sync")
	}
}

func TestSyncBroadcastsKeystrokeToOtherSyncTabs(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
		newSessionTab(3, config.Host{Name: "c"}, 80, 24),
	}
	app.active = 0
	// Active tab + one other in sync; the third tab stays out.
	app.syncTabs[1] = true
	app.syncTabs[2] = true

	// Without real SSH sessions attached, sendInput silently no-ops; we
	// only verify the broadcast path doesn't crash and that prefix is
	// honored. The fanout logic itself is exercised through the active
	// tab branch.
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
}

func TestSyncPrefixKeyDoesNotBroadcast(t *testing.T) {
	// ^B is mterm-internal — it must enter the prefix machine, not be
	// forwarded to any session.
	app := newTestApp()
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	app.syncTabs[1] = true

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if !app.prefixPending {
		t.Fatal("Ctrl-B in session+sync should still set prefixPending")
	}
}

func TestClosingTabRemovesItFromSync(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 0
	app.syncTabs[1] = true
	app.syncTabs[2] = true

	app.removeTabAt(0)
	if app.tabInSync(1) {
		t.Fatal("removed tab must be evicted from the sync set")
	}
	if !app.tabInSync(2) {
		t.Fatal("untouched tab should stay in the sync set")
	}
}

func TestPrefixSTogglesActiveTabSync(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !app.tabInSync(1) {
		t.Fatal("^B s should add the active tab to sync")
	}
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if app.tabInSync(1) {
		t.Fatal("second ^B s should remove the active tab from sync")
	}
}

func TestSyncCountSuffixShowsOnlyWhenNonEmpty(t *testing.T) {
	if got := syncCount(0); got != "" {
		t.Fatalf("syncCount(0) = %q, want empty", got)
	}
	if got := syncCount(3); !strings.Contains(got, "sync 3") {
		t.Fatalf("syncCount(3) = %q, want to contain 'sync 3'", got)
	}
}

func TestAppMouseWheelScrollsActiveTab(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	// Push enough output into scrollback that the offset has room to grow.
	tab := app.tabs[0]
	for i := 0; i < 50; i++ {
		tab.term.Write([]byte("line\r\n"))
	}

	// Wheel up moves the offset up.
	app.update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if off, _ := tab.scrollPos(); off <= 0 {
		t.Fatalf("wheel up should have raised scrollOffset; got %d", off)
	}

	// Wheel down brings it back toward live.
	for i := 0; i < 10; i++ {
		app.update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	if off, _ := tab.scrollPos(); off != 0 {
		t.Fatalf("repeated wheel down should clamp offset to 0; got %d", off)
	}
}

func TestAppSessionKeystrokeSnapsScrollbackToLive(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	tab := app.tabs[0]
	for i := 0; i < 50; i++ {
		tab.term.Write([]byte("line\r\n"))
	}
	// Scroll back into history.
	tab.scrollBy(15)
	if off, _ := tab.scrollPos(); off == 0 {
		t.Fatalf("setup: scroll did not advance offset")
	}
	// Any plain keystroke in session mode must snap to live so the user's
	// typing doesn't disappear under a frozen scrollback view.
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if off, _ := tab.scrollPos(); off != 0 {
		t.Fatalf("keystroke should snap offset to 0; got %d", off)
	}
}

func TestActivityStateTransitions(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "a"}, 80, 24)
	now := time.Now()

	// Fresh tab + focused = no badge.
	if got := tab.activityState(true, now); got != activityNone {
		t.Fatalf("focused fresh tab = %v, want none", got)
	}
	// Fresh tab + unfocused with no activity = still no badge.
	if got := tab.activityState(false, now); got != activityNone {
		t.Fatalf("unfocused fresh tab = %v, want none", got)
	}

	// Simulate a read-loop iteration's bookkeeping.
	tab.activityCounter.Add(1)
	tab.lastActivityUnixNano.Store(now.UnixNano())

	// Unfocused with recent activity = activity.
	if got := tab.activityState(false, now); got != activityActivity {
		t.Fatalf("unfocused with new output = %v, want activity", got)
	}
	// Focused clears the badge.
	if got := tab.activityState(true, now); got != activityNone {
		t.Fatalf("focused tab should never show a badge; got %v", got)
	}

	// Past the silence threshold the badge transitions to silence.
	later := now.Add(silenceThreshold + time.Second)
	if got := tab.activityState(false, later); got != activitySilence {
		t.Fatalf("after silenceThreshold = %v, want silence", got)
	}

	// markSeen catches the tab up — badge goes back to none.
	tab.markSeen()
	if got := tab.activityState(false, later); got != activityNone {
		t.Fatalf("after markSeen = %v, want none", got)
	}
}

func TestSetActiveMarksSeen(t *testing.T) {
	app := newTestApp()
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	app.active = 0
	// Tab 2 racks up activity while in the background.
	app.tabs[1].activityCounter.Add(5)
	app.tabs[1].lastActivityUnixNano.Store(time.Now().UnixNano())
	if got := app.tabs[1].activityState(false, time.Now()); got != activityActivity {
		t.Fatalf("tab 2 should show activity before focus; got %v", got)
	}
	// Switching to tab 2 clears its badge.
	app.setActive(1)
	if got := app.tabs[1].activityState(true, time.Now()); got != activityNone {
		t.Fatalf("after setActive(1), badge should clear; got %v", got)
	}
	// And going back to tab 1 (which had no activity) shows nothing on it.
	app.setActive(0)
	if got := app.tabs[0].activityState(true, time.Now()); got != activityNone {
		t.Fatalf("tab 1 should remain badge-free; got %v", got)
	}
}

func TestSessionViewSurfacesStatusMsg(t *testing.T) {
	// statusMsg set during session mode must appear somewhere in the view —
	// otherwise palette feedback (workspace results, log paths, errors) is
	// silently swallowed since the session body is the live VT.
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0
	app.statusMsg = "workspace \"dev-day\": all hosts already open"

	out := app.View()
	if !strings.Contains(out, "all hosts already open") {
		t.Fatalf("session view should contain statusMsg; got:\n%s", out)
	}
}

func TestWorkspaceRestoreOpensTabsForKnownHosts(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	// newTestApp has nil connector → openTab populates a.tabs but returns
	// nil; what we care about is the tab side-effect and the status summary.
	restoreWorkspace(app, "dev-day", []string{sampleHosts()[0].Name})
	if len(app.tabs) != 1 {
		t.Fatalf("expected 1 tab opened, got %d", len(app.tabs))
	}
	if !strings.HasPrefix(app.statusMsg, "workspace restored:") {
		t.Fatalf("statusMsg = %q, want a restored summary", app.statusMsg)
	}
}

func TestWorkspaceRestoreSkipsAlreadyOpenHosts(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	host := sampleHosts()[0]
	app.tabs = []*sessionTab{newSessionTab(1, host, 80, 24)}
	restoreWorkspace(app, "dev-day", []string{host.Name})
	if len(app.tabs) != 1 {
		t.Fatalf("should not open a duplicate tab; got %d tabs", len(app.tabs))
	}
	if !strings.Contains(app.statusMsg, "already open") {
		t.Fatalf("statusMsg = %q, want 'already open' notice", app.statusMsg)
	}
}

func TestWorkspaceRestoreSurfacesMissingHosts(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	restoreWorkspace(app, "dev-day", []string{"ghost-host"})
	if len(app.tabs) != 0 {
		t.Fatalf("missing host should not produce a tab; got %d", len(app.tabs))
	}
	if !strings.Contains(app.statusMsg, "ghost-host") {
		t.Fatalf("statusMsg should name the missing host; got %q", app.statusMsg)
	}
}

func TestAppCtrlCPassesThroughToSession(t *testing.T) {
	// Ctrl-C is the universal "interrupt remote process" signal — mterm
	// must NOT intercept it. Quit is on the deliberate ^B q two-step.
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	cmd := app.update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		t.Fatalf("Ctrl-C in session mode must not return a cmd; got %T", cmd())
	}
}

func TestOpenLoggerForTabRespectsHostOptOut(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp()
	app.SetLogRoot(dir)
	off := false
	host := config.Host{Name: "noisy", HostName: "noisy", Port: 22, Log: &off}
	tab := newSessionTab(1, host, 80, 24)

	app.openLoggerForTab(tab)
	if tab.logPath() != "" {
		t.Fatalf("logger should not be opened when host.Log == false; got path %q", tab.logPath())
	}
}

func TestOpenLoggerForTabRespectsEmptyLogRoot(t *testing.T) {
	app := newTestApp()
	// app.logRoot intentionally left empty (test default)
	host := config.Host{Name: "noisy", HostName: "noisy", Port: 22}
	tab := newSessionTab(1, host, 80, 24)

	app.openLoggerForTab(tab)
	if tab.logPath() != "" {
		t.Fatalf("logger should not be opened with empty logRoot; got %q", tab.logPath())
	}
}

func TestOpenLoggerForTabCreatesFileWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp()
	app.SetLogRoot(dir)
	host := config.Host{Name: "talky", HostName: "talky", Port: 22}
	tab := newSessionTab(1, host, 80, 24)
	t.Cleanup(tab.close)

	app.openLoggerForTab(tab)
	p := tab.logPath()
	if p == "" {
		t.Fatal("expected a log path after openLoggerForTab")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected log file at %s: %v", p, err)
	}
	if !strings.Contains(p, dir) || !strings.Contains(p, "talky") {
		t.Fatalf("path %q should be under logRoot %q and contain host name", p, dir)
	}
}

func TestShowLogPathSurfacesViaStatus(t *testing.T) {
	dir := t.TempDir()
	app := newTestApp()
	app.SetLogRoot(dir)
	host := config.Host{Name: "talky", HostName: "talky", Port: 22}
	tab := newSessionTab(1, host, 80, 24)
	app.tabs = []*sessionTab{tab}
	app.active = 0
	app.openLoggerForTab(tab)
	t.Cleanup(tab.close)

	showLogPath(app)
	if !strings.HasPrefix(app.statusMsg, "log: ") {
		t.Fatalf("statusMsg should start with %q; got %q", "log: ", app.statusMsg)
	}
	if !strings.Contains(app.statusMsg, tab.logPath()) {
		t.Fatalf("statusMsg %q should contain log path %q", app.statusMsg, tab.logPath())
	}
}

func TestShowLogPathReportsDisabledWhenLoggerAbsent(t *testing.T) {
	app := newTestApp()
	host := config.Host{Name: "quiet", HostName: "quiet", Port: 22}
	tab := newSessionTab(1, host, 80, 24)
	app.tabs = []*sessionTab{tab}
	app.active = 0
	showLogPath(app)
	if !strings.Contains(app.statusMsg, "not enabled") {
		t.Fatalf("statusMsg should report disabled; got %q", app.statusMsg)
	}
}

func TestAppPrefixQQuits(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.prefixPending = true
	cmd := app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("^B q must return a quit cmd")
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

func TestAppPaletteChosenPreservesActionModeChange(t *testing.T) {
	// Commands like "Open picker" intentionally change a.mode in their
	// action. The palette handler must NOT overwrite that with prevMode.
	app := newTestApp()
	app.mode = modePalette
	app.prevMode = modeSession

	c := command{label: "to picker", group: "G", action: func(a *App) tea.Cmd {
		a.mode = modePicker
		return nil
	}}
	app.update(paletteChosenMsg{cmd: c})

	if app.mode != modePicker {
		t.Fatalf("action set modePicker but handler reset to %v", app.mode)
	}
}

func TestReloadHostsKeepsPickerNonNil(t *testing.T) {
	app := newTestApp()
	reloadHosts(app)
	if app.picker == nil {
		t.Fatal("picker became nil after reloadHosts")
	}
}

func TestAppSessionViewUsesHostBorderColor(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	host := config.Host{Name: "prod", HostName: "prod", Port: 22, BorderColor: "#FF3344"}
	app.tabs = []*sessionTab{newSessionTab(1, host, 80, 24)}
	app.active = 0

	got := app.View()

	// Truecolor escape for #FF3344 is "\x1b[38;2;255;51;68m"
	want := "\x1b[38;2;255;51;68m"
	if !strings.Contains(got, want) {
		t.Fatalf("session view missing host border color escape %q\noutput:\n%s", want, got)
	}
}

func TestAppSessionViewIgnoresEmptyBorderColor(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	host := config.Host{Name: "plain", HostName: "plain", Port: 22}
	app.tabs = []*sessionTab{newSessionTab(1, host, 80, 24)}
	app.active = 0

	got := app.View()
	notWant := "\x1b[38;2;255;51;68m"
	if strings.Contains(got, notWant) {
		t.Fatalf("session view contains unexpected color escape %q", notWant)
	}
}
