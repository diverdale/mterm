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

	// User-defined themes from ~/.config/<app>/themes.yaml. Built-ins
	// (Midnight/Matrix/Synthwave) always remain available; user entries
	// extend the list.
	if tpath, err := config.ThemesFile(); err == nil {
		if themes, err := tui.LoadUserThemes(tpath); err == nil {
			tui.SetUserThemes(themes)
		} else {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: themes load:", err)
		}
	}

	// Global toggles (frame style today; more in future cycles) from
	// ~/.config/<app>/settings.yaml.
	if spath, err := config.SettingsFile(); err == nil {
		if err := tui.LoadSettings(spath); err != nil {
			fmt.Fprintln(os.Stderr, appmeta.Name+": warning: settings load:", err)
		}
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

	// buildAuth assembles the per-host auth chain:
	//   identityfile  (when set)   tried first — mimics ssh's preference
	//   agent         (shared)     covers most modern setups
	//   password_command            evaluated lazily; for vault / 1password etc
	//   password                   plaintext literal — last-resort fallback
	// Earlier methods that fail (wrong key, locked agent) flow through to
	// the next so a host with stale agent state still connects.
	buildAuth := func(host config.Host) mssh.AuthProvider {
		providers := []mssh.AuthProvider{}
		if host.IdentityFile != "" {
			providers = append(providers, mssh.NewIdentityFileProvider(host.IdentityFile))
		}
		providers = append(providers, auth)
		if host.PasswordCommand != "" {
			providers = append(providers, mssh.NewPasswordCommandProvider(host.PasswordCommand))
		}
		if host.Password != "" {
			providers = append(providers, mssh.NewPasswordProvider(host.Password))
		}
		return mssh.ChainProvider(providers...)
	}
	// v1: trust-on-first-use accepts unknown keys automatically. A future
	// version routes this through an interactive modal.
	hostKeyCallback := func() gossh.HostKeyCallback {
		v := mssh.NewHostKeyVerifier(knownHostsPath,
			func(string, gossh.PublicKey) bool { return true })
		return v.Callback()
	}

	connect := func(host config.Host, cols, rows int) (*mssh.Session, error) {
		// Resolve a `proxy_jump:` value (alias or literal) into a Host we
		// can dial. Empty value → nil proxy → straight-through dial.
		proxyHost, err := config.ResolveProxyJump(host.ProxyJump, res.Hosts)
		if err != nil {
			return nil, fmt.Errorf("proxy_jump for %s: %w", host.Name, err)
		}

		if proxyHost == nil {
			sess := mssh.NewSession(host, buildAuth(host), hostKeyCallback())
			if err := sess.Connect(cols, rows); err != nil {
				return nil, err
			}
			return sess, nil
		}

		// Two-hop dial: bring up the jump session first, then tunnel the
		// target SSH conn through it. Any failure on the proxy aborts
		// before the target dial.
		proxySess := mssh.NewSession(*proxyHost, buildAuth(*proxyHost), hostKeyCallback())
		// The proxy doesn't get a user-visible PTY (we never read/write
		// its shell). 80x24 is enough to satisfy the handshake.
		if err := proxySess.Connect(80, 24); err != nil {
			return nil, fmt.Errorf("dial jump host %s: %w", proxyHost.Name, err)
		}
		sess := mssh.NewSessionVia(host, buildAuth(host), hostKeyCallback(), proxySess)
		if err := sess.Connect(cols, rows); err != nil {
			// Tear down the proxy too — target ownership hasn't transferred yet.
			_ = proxySess.Close()
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
