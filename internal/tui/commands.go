package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/appmeta"
	"mterm/internal/config"
)

// command is one entry in the palette: a display label, a group header label,
// and the action to run when the user picks it.
type command struct {
	label  string
	group  string
	action func(a *App) tea.Cmd
}

// buildCommands assembles the v1 palette command set from the app's current
// state: navigation, direct-host connect, theme switches, and config reload.
func buildCommands(a *App) []command {
	var cmds []command

	// Navigation
	cmds = append(cmds,
		command{label: "Open picker", group: "Navigation", action: func(a *App) tea.Cmd {
			a.picker.setQuery("")
			a.mode = modePicker
			return nil
		}},
		command{label: "Open forwards panel", group: "Navigation", action: func(a *App) tea.Cmd {
			if t := a.activeTab(); t != nil {
				a.forwards = newForwardsPanel(t.host)
				a.mode = modeForwards
			}
			return nil
		}},
		command{label: "Next tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.cycleTab(1)
			return nil
		}},
		command{label: "Previous tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.cycleTab(-1)
			return nil
		}},
		command{label: "Close current tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.closeActiveTab()
			return nil
		}},
		command{label: "Quit " + appmeta.Name, group: "Navigation", action: func(a *App) tea.Cmd {
			return a.shutdown()
		}},
	)

	// One Switch: entry per open tab.
	for i, t := range a.tabs {
		idx, title := i, t.title()
		cmds = append(cmds, command{
			label: fmt.Sprintf("Switch: %d %s", idx+1, title),
			group: "Navigation",
			action: func(a *App) tea.Cmd {
				if idx >= 0 && idx < len(a.tabs) {
					a.active = idx
					a.mode = modeSession
				}
				return nil
			},
		})
	}

	// One Connect: entry per known host.
	for _, h := range a.picker.all {
		host := h
		cmds = append(cmds, command{
			label:  "Connect: " + host.Name,
			group:  "Connect",
			action: func(a *App) tea.Cmd { return a.openTab(host) },
		})
	}

	// One Theme: entry per available theme.
	for _, th := range Themes() {
		theme := th
		cmds = append(cmds, command{
			label: "Theme: " + theme.Name,
			group: "Theme",
			action: func(a *App) tea.Cmd {
				setTheme(theme)
				return nil
			},
		})
	}

	// Config
	cmds = append(cmds, command{
		label:  "Reload config",
		group:  "Config",
		action: func(a *App) tea.Cmd { reloadHosts(a); return nil },
	})

	// Session
	cmds = append(cmds, command{
		label:  "Show log path for current tab",
		group:  "Session",
		action: func(a *App) tea.Cmd { showLogPath(a); return nil },
	})

	// Broadcast / sync
	cmds = append(cmds,
		command{label: "Add active tab to sync set", group: "Broadcast", action: func(a *App) tea.Cmd {
			t := a.activeTab()
			if t != nil && !a.tabInSync(t.id) {
				a.toggleActiveTabSync()
			}
			return nil
		}},
		command{label: "Remove active tab from sync set", group: "Broadcast", action: func(a *App) tea.Cmd {
			t := a.activeTab()
			if t != nil && a.tabInSync(t.id) {
				a.toggleActiveTabSync()
			}
			return nil
		}},
		command{label: "Clear sync set", group: "Broadcast", action: func(a *App) tea.Cmd {
			a.clearSyncSet()
			return nil
		}},
	)

	return cmds
}

// showLogPath surfaces the active tab's session-log file path via statusMsg
// so the user can grab it (mouse-select / Cmd-C in their terminal). System
// clipboard write would need OSC52 plumbing — deferred.
func showLogPath(a *App) {
	t := a.activeTab()
	if t == nil {
		a.statusMsg = "log: no active tab"
		return
	}
	p := t.logPath()
	if p == "" {
		a.statusMsg = "log: not enabled for " + t.host.Name
		return
	}
	a.statusMsg = "log: " + p
}

// reloadHosts re-reads the config files and replaces the picker's host list.
// Open tabs keep running on their existing SSH connections — only the picker
// (and the next palette build) sees the change. A successful reload clears
// any prior transient status message; warnings from config.Load then go to
// a.statusMsg (last warning wins).
func reloadHosts(a *App) {
	res, err := config.Load()
	if err != nil {
		a.statusMsg = fmt.Sprintf("reload: %v", err)
		return
	}
	a.picker = newPicker(res.Hosts)
	a.statusMsg = ""
	for _, w := range res.Warnings {
		a.statusMsg = w
	}
}
