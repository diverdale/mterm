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
	"mterm/internal/config"
	"mterm/internal/diag"
	"mterm/internal/history"
	mssh "mterm/internal/ssh"
	"mterm/internal/tui"
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
	if histPath, err := config.HistoryFile(); err == nil {
		if hist, err := history.Open(histPath); err == nil {
			app.SetHistory(hist)
		} else {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: history load:", err)
		}
	}
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
