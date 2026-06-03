package tui

import (
	"testing"

	"mterm/internal/config"
)

func TestBuildCommandsCounts(t *testing.T) {
	app := newTestApp() // sampleHosts() has 3 hosts; no open tabs
	cmds := buildCommands(app)

	counts := map[string]int{}
	for _, c := range cmds {
		counts[c.group]++
	}
	if counts["Navigation"] != 6 {
		t.Fatalf("Navigation count = %d, want 6", counts["Navigation"])
	}
	// Connect entries are intentionally NOT in the palette — the picker
	// (^B c) is the dedicated UI for opening connections. Counting them
	// in case a future refactor accidentally reintroduces them.
	if counts["Connect"] != 0 {
		t.Fatalf("Connect count = %d, want 0 (handled by picker)", counts["Connect"])
	}
	// Theme group covers both "Theme: <name>" and "Frame: <name>" entries.
	wantThemeGroup := len(Themes()) + len(FrameStyleNames())
	if counts["Theme"] != wantThemeGroup {
		t.Fatalf("Theme count = %d, want %d", counts["Theme"], wantThemeGroup)
	}
	if counts["Config"] != 1 {
		t.Fatalf("Config count = %d, want 1", counts["Config"])
	}
}

func TestBuildCommandsAddsSwitchEntriesPerOpenTab(t *testing.T) {
	app := newTestApp()
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	cmds := buildCommands(app)
	switches := 0
	for _, c := range cmds {
		if c.group == "Navigation" && len(c.label) >= 8 && c.label[:8] == "Switch: " {
			switches++
		}
	}
	if switches != 2 {
		t.Fatalf("Switch: count = %d, want 2", switches)
	}
}

func TestThemeCommandActionChangesActive(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })
	app := newTestApp()
	cmds := buildCommands(app)
	var matrix *command
	for i := range cmds {
		if cmds[i].label == "Theme: matrix" {
			matrix = &cmds[i]
			break
		}
	}
	if matrix == nil {
		t.Fatal("Theme: matrix command not found")
	}
	matrix.action(app)
	if active.Name != "matrix" {
		t.Fatalf("after running Theme: matrix action, active = %q", active.Name)
	}
}
