package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"mterm/internal/appmeta"
	"mterm/internal/config"
	mssh "mterm/internal/ssh"
	"mterm/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, appmeta.Name+":", err)
		os.Exit(1)
	}
}

func run() error {
	// Best-effort: ensure ~/.config/<DirName>/ exists so a fresh user can
	// drop hosts.yaml in place without having to mkdir manually. Failures
	// fall through to the existing "no hosts" error path.
	_ = config.BootstrapConfigDir()

	res, err := config.Load()
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
		sess := mssh.NewSession(host, auth, verifier.Callback())
		if err := sess.Connect(cols, rows); err != nil {
			return nil, err
		}
		return sess, nil
	}

	app := tui.NewApp(res.Hosts, connect)
	if logsDir, err := config.LogsDir(); err == nil {
		app.SetLogRoot(logsDir)
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
