package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mterm/internal/appmeta"
	"mterm/internal/colors"
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
					a.setActive(idx)
					a.mode = modeSession
				}
				return nil
			},
		})
	}

	// Connect entries deliberately omitted — the picker (^B c) is the
	// dedicated UI for opening a connection, and inlining one Connect
	// entry per host here just drowns the actual actions in a sea of
	// hostnames.

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

	// Workspaces — one save command + N restore/delete commands per saved
	// workspace. Hidden entirely when no store is wired in.
	if a.workspaces != nil {
		cmds = append(cmds, command{
			label: "Save current tabs as workspace…",
			group: "Workspaces",
			action: func(a *App) tea.Cmd {
				if len(a.tabs) == 0 {
					a.statusMsg = "no tabs to save"
					return nil
				}
				// Do NOT touch a.prevMode here — it already holds the
				// "where we came from before the palette" mode that ^B :
				// set, and we want cancel/submit to jump straight back
				// there rather than landing on the now-empty palette.
				a.workspaceSave = newWorkspaceSaveModel(len(a.tabs))
				a.mode = modeWorkspaceSave
				return nil
			},
		})
		for _, w := range a.workspaces.List() {
			name := w.Name
			hosts := w.Hosts
			cmds = append(cmds,
				command{
					label: fmt.Sprintf("Restore workspace: %s (%d tabs)", name, len(hosts)),
					group: "Workspaces",
					action: func(a *App) tea.Cmd {
						return restoreWorkspace(a, name, hosts)
					},
				},
				command{
					label: "Delete workspace: " + name,
					group: "Workspaces",
					action: func(a *App) tea.Cmd {
						if err := a.workspaces.Delete(name); err != nil {
							a.statusMsg = fmt.Sprintf("workspace delete: %v", err)
						} else {
							a.statusMsg = "workspace deleted: " + name
						}
						return nil
					},
				},
			)
		}
	}

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

// restoreWorkspace opens tabs to reach the workspace's per-host count,
// preserving multiplicity from the saved list (e.g. three "sys-dev"
// entries → three sys-dev tabs after restore).
//
// Semantics are additive: each saved occurrence is satisfied by an existing
// tab when available, otherwise a fresh tab is opened. Existing tabs beyond
// the workspace's count for that host are left alone — restore never closes
// anything. Missing hosts (renamed/removed from hosts.yaml since save)
// surface as a single comma-joined statusMsg warning.
//
// Walks hostNames in saved order so visual tab ordering matches what the
// user captured.
func restoreWorkspace(a *App, name string, hostNames []string) tea.Cmd {
	openCount := map[string]int{}
	for _, t := range a.tabs {
		openCount[t.host.Name]++
	}
	byName := map[string]config.Host{}
	for _, h := range a.picker.all {
		byName[h.Name] = h
	}
	seenInWs := map[string]int{}
	seenMissing := map[string]bool{}
	var cmds []tea.Cmd
	var missing []string
	opened := 0
	for _, hn := range hostNames {
		seenInWs[hn]++
		// First openCount[hn] occurrences of this host are covered by
		// the tabs that are already open; only later occurrences need
		// a fresh tab.
		if seenInWs[hn] <= openCount[hn] {
			continue
		}
		host, ok := byName[hn]
		if !ok {
			if !seenMissing[hn] {
				missing = append(missing, hn)
				seenMissing[hn] = true
			}
			continue
		}
		if cmd := a.openTab(host); cmd != nil {
			cmds = append(cmds, cmd)
		}
		opened++
	}
	switch {
	case len(missing) > 0 && opened > 0:
		a.statusMsg = fmt.Sprintf("workspace %q: opened %d, missing: %s",
			name, opened, strings.Join(missing, ", "))
	case len(missing) > 0:
		a.statusMsg = fmt.Sprintf("workspace %q: nothing opened (missing: %s)",
			name, strings.Join(missing, ", "))
	case opened > 0:
		a.statusMsg = fmt.Sprintf("workspace restored: %s (%d new tabs)", name, opened)
	default:
		a.statusMsg = fmt.Sprintf("workspace %q: already satisfied", name)
	}
	return tea.Batch(cmds...)
}

// reloadHosts re-reads the config files and replaces the picker's host list.
// Re-reads colors.yaml too so a named bordercolor (e.g. "msftblue") that
// only exists in the user overlay still resolves after the user edits
// either file at runtime — without this re-read, the startup colors map
// stayed in main.go and reload fell back to nil-colors mode, breaking
// every host that used a custom name.
//
// Open tabs keep running on their existing SSH connections — only the
// picker (and the next palette build) see the change. A successful
// reload clears any prior transient status message; warnings from
// config.Load then go to a.statusMsg (last warning wins).
func reloadHosts(a *App) {
	var colorMap map[string]string
	if cpath, err := config.ColorsFile(); err == nil {
		if m, err := colors.Open(cpath); err == nil {
			colorMap = m
		} else {
			// Surface the colors-file parse error but keep going with
			// built-ins so reload still functions for hex bordercolors.
			a.statusMsg = fmt.Sprintf("reload colors: %v", err)
			colorMap = colors.Builtins()
		}
	}
	res, err := config.LoadWithColors(colorMap)
	if err != nil {
		a.statusMsg = fmt.Sprintf("reload: %v", err)
		return
	}
	a.picker = newPicker(res.Hosts)
	// Don't blank an existing colors-file warning above; only clear if
	// nothing was already complaining.
	if !strings.HasPrefix(a.statusMsg, "reload colors:") {
		a.statusMsg = ""
	}
	for _, w := range res.Warnings {
		a.statusMsg = w
	}
}
