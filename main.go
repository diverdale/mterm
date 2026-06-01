package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"mterm/internal/appmeta"
	"mterm/internal/colors"
	"mterm/internal/config"
	"mterm/internal/diag"
	"mterm/internal/history"
	mssh "mterm/internal/ssh"
	"mterm/internal/tui"
	"mterm/internal/workspaces"
)

// installGoroutineDumpHandler wires SIGUSR1 to diag.DumpGoroutines. Lets
// users capture a running mterm's state during a hang from another terminal:
//
//	pkill -USR1 mterm    # or: kill -USR1 <pid>
//	ls -t /tmp/mterm-stacks-* | head -1
//
// The dump path is printed to stderr; under bubbletea's alt screen that
// becomes visible only after mterm exits. ^B D triggers the same dump
// in-app and surfaces the path via the footer statusMsg.
func installGoroutineDumpHandler() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	go func() {
		for range ch {
			path, err := diag.DumpGoroutines(appmeta.DirName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: stack dump failed: %v\n", appmeta.Name, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "%s: stack dump written to %s\n", appmeta.Name, path)
		}
	}()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, appmeta.Name+":", err)
		os.Exit(1)
	}
}

func run() error {
	installGoroutineDumpHandler()

	// Best-effort: ensure ~/.config/<DirName>/ exists so a fresh user can
	// drop hosts.yaml in place without having to mkdir manually. Failures
	// fall through to the existing "no hosts" error path.
	_ = config.BootstrapConfigDir()

	// Load the named-color palette first (built-ins + ~/.config/<app>/
	// colors.yaml overlay) so bordercolor values like "limegreen" resolve
	// during config parsing.
	var colorMap map[string]string
	if cpath, err := config.ColorsFile(); err == nil {
		if m, err := colors.Open(cpath); err == nil {
			colorMap = m
		} else {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: colors load:", err)
			colorMap = colors.Builtins()
		}
	}

	res, err := config.LoadWithColors(colorMap)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, appmeta.Name+": warning:", w)
	}
	if len(res.Hosts) == 0 {
		hostsPath, _ := config.HostsFile()
		return fmt.Errorf("no hosts found in ~/.ssh/config or %s", hostsPath)
	}

	auth, err := mssh.AgentProviderFromEnv()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")

	connect := func(host config.Host, cols, rows int) (*mssh.Session, error) {
		// v1: trust-on-first-use accepts unknown keys automatically. A future
		// version routes this through an interactive modal.
		verifier := mssh.NewHostKeyVerifier(knownHostsPath,
			func(string, gossh.PublicKey) bool { return true })

		// Build the per-host auth chain. IdentityFile (from hosts.yaml or
		// ssh_config) is tried first when set, then the agent — matches
		// `ssh`'s own fallback order and means a reboot-cleared agent
		// doesn't break auth when a key is also on disk.
		providers := []mssh.AuthProvider{}
		if host.IdentityFile != "" {
			providers = append(providers, mssh.NewIdentityFileProvider(host.IdentityFile))
		}
		providers = append(providers, auth)
		hostAuth := mssh.ChainProvider(providers...)

		sess := mssh.NewSession(host, hostAuth, verifier.Callback())
		if err := sess.Connect(cols, rows); err != nil {
			return nil, err
		}
		return sess, nil
	}

	app := tui.NewApp(res.Hosts, connect)
	if logsDir, err := config.LogsDir(); err == nil {
		app.SetLogRoot(logsDir)
	}
	if histPath, err := config.HistoryFile(); err == nil {
		if hist, err := history.Open(histPath); err == nil {
			app.SetHistory(hist)
		} else {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: history load:", err)
		}
	}
	if wsPath, err := config.WorkspacesFile(); err == nil {
		if ws, err := workspaces.Open(wsPath); err == nil {
			app.SetWorkspaces(ws)
		} else {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: workspaces load:", err)
		}
	}
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
