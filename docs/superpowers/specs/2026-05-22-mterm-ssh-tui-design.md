# mterm — Multi-Connection SSH TUI

**Date:** 2026-05-22
**Status:** Approved design

## Summary

`mterm` is a Go-powered terminal UI for managing multiple SSH connections. It
presents a fuzzy-searchable picker of saved hosts and opens each selected host
as a **live, tabbed session**. One session is full-screen at a time with a tab
bar to switch between them — like a browser for SSH connections. Each tab is a
genuine terminal: full-screen remote programs (vim, htop, less, tmux) render
correctly via an embedded VT emulator.

## Goals

- Browse hosts from `~/.ssh/config` plus an mterm-specific config, with fuzzy
  search, groups, and tags.
- Open multiple concurrent SSH sessions as tabs; switch instantly.
- Full terminal fidelity inside every tab.
- Self-contained static binary with no runtime dependency on a system `ssh`.

## Non-Goals (v1)

- **SFTP file browser** — deferred to future work.
- Split-pane / tiled multiplexing — tabs only, one full-screen session at a time.
- Key-file, password, and jump-host authentication — the auth layer is designed
  to accept them later, but v1 ships SSH-agent auth only.

## Core Decisions

| Decision | Choice |
|----------|--------|
| Core model | Tabbed live sessions (one full-screen at a time) |
| Host config source | `~/.ssh/config` base + mterm `hosts.yaml` overlay |
| Authentication (v1) | SSH agent only, behind a pluggable interface |
| Terminal fidelity | Full — embedded VT emulator per tab |
| Connection method | Native Go SSH (`golang.org/x/crypto/ssh`) |
| v1 extras | Auto-reconnect, scrollback/copy mode, port forwarding |

## Tech Stack

- **`charmbracelet/bubbletea`** — TUI framework
- **`charmbracelet/lipgloss`** — styling
- **`charmbracelet/bubbles`** — list and textinput widgets for the picker
- **`golang.org/x/crypto/ssh`** — SSH client, plus `ssh/knownhosts` and `ssh/agent`
- **`charmbracelet/x/vt`** — in-memory VT terminal emulator (one per tab)
- **`kevinburke/ssh_config`** — parse `~/.ssh/config`
- **`gopkg.in/yaml.v3`** — parse mterm's own config
- **`atotto/clipboard`** — system clipboard for copy mode

## Package Layout

```
mterm/
  main.go                  entrypoint: flags, bootstrap, tea.Program
  internal/
    config/                load + merge ~/.ssh/config and hosts.yaml -> []Host
    ssh/                   AuthProvider iface, Session (ssh+PTY), reconnect, forwards
    terminal/              wraps x/vt: feed bytes in, expose a renderable grid
    tui/
      app.go               root model: picker<->session views, tab bar, prefix key
      picker.go            fuzzy host picker (Bubbles list + textinput)
      session.go           per-tab model: holds a terminal grid, routes input
      forwards.go          port-forward panel
      keys.go              key bindings
      styles.go            Lip Gloss styles
```

Each package has a single job and a narrow interface: `config` produces
`[]Host`; `ssh` turns a `Host` into a live `Session`; `terminal` turns a byte
stream into a renderable grid; `tui` composes them.

## Data Model

```go
type Source int

const (
    SourceSSHConfig Source = iota
    SourceMterm
)

type ForwardType int

const (
    ForwardLocal ForwardType = iota // -L
    ForwardRemote                   // -R
)

type Forward struct {
    Type     ForwardType
    BindAddr string // empty = localhost
    BindPort int
    DialAddr string
    DialPort int
}

type Host struct {
    Name         string    // alias / Host block name
    HostName     string    // address
    User         string
    Port         int
    Group        string    // from mterm overlay
    Tags         []string  // from mterm overlay
    Source       Source    // SSHConfig | Mterm
    IdentityFile string    // parsed now, used when key-file auth lands
    ProxyJump    string    // parsed now, used when jump-host support lands
    Forwards     []Forward // from ssh_config LocalForward/RemoteForward + overlay
}
```

## Configuration

### Sources and merge

- **Base:** `~/.ssh/config`. Every `Host` block becomes a `Host`. Wildcard-only
  blocks (e.g. `Host *`) are not listed as connectable hosts.
- **Overlay:** `~/.config/mterm/hosts.yaml`. Two roles:
  1. Define additional hosts not present in `ssh_config`.
  2. Provide an overlay keyed by host name that attaches `group` and `tags` to
     existing `ssh_config` hosts without duplicating them.
- On field conflict, mterm-defined values win.
- Parse errors in either file are logged to a startup notice and skipped; they
  are never fatal. mterm starts with whatever parsed successfully.

### Example `hosts.yaml`

```yaml
hosts:
  - name: prod-web-1
    group: production
    tags: [web, critical]
  - name: lab-box
    hostName: 10.9.0.12
    user: dale
    port: 22
    group: lab
    tags: [scratch]
    forwards:
      - type: local
        bindPort: 8080
        dialAddr: 127.0.0.1
        dialPort: 80
```

The first entry is an overlay (`prod-web-1` exists in `ssh_config`; only
`group`/`tags` are added). The second defines a new mterm-only host.

## Connection Layer (`internal/ssh`)

### Authentication

```go
type AuthProvider interface {
    Methods() ([]ssh.AuthMethod, error)
}
```

v1 ships one implementation, `AgentProvider`, which reads `SSH_AUTH_SOCK` and
exposes the agent's keys. Future `KeyFileProvider`, `PasswordProvider`, and
jump-host support slot in behind the same interface with no caller changes.

### Host keys

A `HostKeyCallback` is built from `~/.ssh/known_hosts` via
`golang.org/x/crypto/ssh/knownhosts`. An unknown host key raises a TUI modal
offering accept/reject (trust-on-first-use). On accept, the key is appended to
`known_hosts`. On reject, the connection is aborted and the tab shows an error
state.

### Session lifecycle

A `Session` owns the `*ssh.Client`, an `ssh.Session` with a requested PTY, and
the session's stdin pipe. It exposes a state:

```
Connecting -> Connected -> Reconnecting -> Failed
                   |            |
                   +------------+--> Closed (user-initiated)
```

- **Auto-reconnect:** a non-user-initiated EOF or error transitions the tab to
  `Reconnecting` and retries with exponential backoff (capped). The tab stays in
  place and shows a reconnecting banner. A user-initiated close goes straight to
  `Closed` with no retry.
- **Isolation:** each session goroutine runs under `recover()`, so a broken
  connection degrades that tab to `Failed` rather than crashing the TUI.

### Port forwarding

Forwards are **defined in configuration**, not created ad-hoc: `LocalForward`
and `RemoteForward` directives in `~/.ssh/config`, plus a `forwards:` list in a
host's `hosts.yaml` entry. They populate `Host.Forwards`.

Local (`-L`) forwards use `ssh.Client.Dial`; remote (`-R`) forwards use
`ssh.Client.Listen`. Each forward runs in its own goroutine and is started when
the session connects. The forwards panel (`prefix f`) lists every forward for
the focused session with its live status and lets the user toggle each one on or
off. Adding ad-hoc forwards interactively is future work.

## Rendering & Concurrency

Bubble Tea's `Update` loop is single-threaded. Each `Session` runs one **reader
goroutine**:

```
ssh PTY stream ──▶ reader goroutine ──▶ terminal (x/vt) updates the grid
                          │
                          └──▶ program.Send(sessionDirtyMsg{tabID})
```

- The goroutine writes bytes directly into that tab's `terminal` emulator, which
  maintains the screen grid and scrollback.
- It then signals the program via `tea.Program.Send`.
- **Coalescing:** dirty signals are debounced to at most one re-render per frame
  (~30 fps). A high-volume stream (e.g. `tail -f`) updates the emulator grid
  every time but cannot outpace the UI render loop.

### Input routing

When a session tab is focused, key messages are encoded to bytes and written to
the session's stdin — *except* the prefix key and its immediate follow-up key,
which are consumed by mterm. Terminal resize calls `ssh.Session.WindowChange`
for every session and resizes every tab's VT emulator.

## User Experience

### Views

- **Picker** — a fuzzy-search list of hosts grouped by `Group`, with tags shown
  inline. Selecting a host opens it as a new tab.
- **Session** — a one-row tab bar above the focused tab's terminal body.

### Prefix key

A configurable prefix key (default `Ctrl-A`), tmux-style, separates mterm
commands from the remote shell. Everything that is not the prefix sequence is
sent to the focused session.

| Key | Action |
|-----|--------|
| `prefix c` | open picker → new connection tab |
| `prefix n` / `prefix p` | next / previous tab |
| `prefix 1`–`9` | jump to tab N |
| `prefix x` | close current tab (confirm if live) |
| `prefix [` | enter scrollback / copy mode |
| `prefix f` | open port-forward panel |
| `prefix ?` | help overlay |
| `prefix q` | quit mterm (confirm if tabs open) |

### Picker keys

Type to filter; `↑`/`↓` or `Ctrl-j`/`Ctrl-k` to move; `Enter` to connect; `Esc`
to return to the current session; `q` to quit.

### Copy mode

`prefix [` enters copy mode: `↑`/`↓` and `PgUp`/`PgDn` scroll the emulator's
scrollback, `v` starts a selection, `y` yanks the selection to the system
clipboard, `Esc` exits.

## Error Handling

- **Connection / auth failure** — the tab renders an in-tab error state with a
  retry key; the app never crashes.
- **No agent / no keys** — a clear message naming the fix (start an agent, run
  `ssh-add`).
- **Unknown host key** — the accept/reject modal described above.
- **Config parse errors** — logged to a startup notice; startup continues.
- **Session goroutine panic** — recovered; the tab moves to `Failed`.

## Testing

- **`config`** — table tests over fixture `ssh_config` and `hosts.yaml` files;
  assert the merged `[]Host`, including overlay precedence and parse-error
  tolerance.
- **`terminal`** — feed known ANSI/VT sequences; assert grid cell contents and
  scrollback behavior.
- **`ssh`** — spin up an in-process SSH server (`x/crypto/ssh` server side) in
  tests; cover connect, PTY I/O, reconnect with backoff, and a port forward.
  `AgentProvider` is tested against a fake agent.
- **`tui`** — `teatest` drives the root model: picker filtering, tab
  open/switch/close, and prefix-key routing.

## Future Work

- SFTP file browser panel.
- Additional `AuthProvider` implementations: key files (with passphrase prompt),
  password, jump hosts / bastion (`ProxyJump`).
- Split-pane / tiled layout.
