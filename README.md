# mterm

A Go-powered multi-connection SSH TUI. Tabbed sessions, fuzzy host picker,
full-fidelity VT terminal, tmux-style prefix navigation. Aiming to be the
no-bullshit alternative to SecureCRT for engineers who live in the terminal.

## Contents

- [Quick start](#quick-start)
- [Features](#features)
- [File browser](#file-browser-b-u)
- [Copy mode](#copy-mode-b-)
- [hosts.yaml reference](#hostsyaml-reference)
- [ssh_config interaction](#ssh_config-interaction)
- [Customization](#customization)
- [Configuration directory](#configuration-directory)
- [Common keys](#common-keys) — full reference: [docs/keybindings.md](docs/keybindings.md)
- [Building & distribution](docs/building.md)
- [Roadmap](docs/superpowers/roadmap.md)
- [License](#license)

## Quick start

```bash
go build -o mterm .
./mterm
```

On first launch mterm reads `~/.ssh/config` for your existing host aliases
and lists them in the picker. To add hosts that aren't in ssh_config — or
to overlay extra metadata (groups, color tags, port forwards, logging
opt-out, etc.) on existing ones — create `~/.config/mterm/hosts.yaml`:

```yaml
hosts:
  - name: prod-db
    address: 10.0.0.5
    user: alice
    group: production
    bordercolor: "#FF3344"
```

The `~/.config/mterm/` directory is created automatically on first run.

## Features

- **Tabbed SSH sessions** with `^B` prefix navigation (tmux-style); native
  Go SSH client, agent auth + per-host `identityfile:` fallback, lenient
  `known_hosts`
- **Full-fidelity VT terminal** — cursor, 24-bit color, mouse-wheel
  scrollback that snaps back to live on any keystroke
- **Activity / silence indicators** in the tab strip — output on background
  tabs lights a warning glyph; tabs going quiet after activity transition
  to a "done" glyph
- **Fuzzy host picker** with per-host connection state (`◉` in tab, `●`
  previously connected with relative time, `○` never connected); group
  headers show counts
- **Two-pane SFTP file browser** (`^B u`) — upload/download against the
  active session with live progress + cancel ([details](#file-browser-b-u))
- **Keyboard copy mode** (`^B [`) — vim-style scrollback selection that
  pushes to the system clipboard via OSC 52 + native fallbacks; frame
  chars never leak into the payload ([details](#copy-mode-b-))
- **Workspaces** — save+restore named tab sets from the command palette;
  reopens every host in one keystroke
- **Per-host startup commands** — `on_connect:` runs shell commands right
  after handshake (auto-attach tmux, jump into a dir, etc.)
- **Broadcast input** — `^B s` toggles a tab into a sync set; type once,
  fan to every member
- **Themable UI** — built-in themes (Midnight / Matrix / Synthwave), plus
  user-defined themes via `themes.yaml`; six frame styles (`^B m` quick
  toggles to minimal for clipboard grabs) ([details](#customization))
- **Per-host visual identity** — `bordercolor:` tints frame, active tab
  chip, footer hints so prod ≠ staging at a glance
- **Per-session output logging** to `~/.config/mterm/logs/<host>/<timestamp>.log`
  (default on; per-host opt-out)
- **Command palette** (`^B :`) with fuzzy command search;
  **help overlay** (`^B ?`); **port-forwarding panel** (`^B f`, UI only —
  toggles don't open real forwards yet); **auto-reconnect** on transport
  failure
- **ProxyJump (jump host) support** — set `proxy_jump:` on a host in
  hosts.yaml (or use `ProxyJump` in `~/.ssh/config`); mterm tunnels the
  target SSH conn through the jump host transparently. Single hop in v1.

## File browser (`^B u`)

A two-pane SFTP browser for the active session — left pane is your local
filesystem, right pane is the remote. Copies are direction-implicit: the
**active pane is the source**, the other pane is the destination. So
local-active → `F5` uploads; remote-active → `F5` downloads.

```
╭─ mterm  [1 tabs] ──────────────────────────────────────────────────────╮
│ [ ● 1 home-server ]                                                    │
├────────────────────────────────────────────────────────────────────────┤
│ ▶ LOCAL                              REMOTE: home-server               │
│   /Users/alice/Desktop               /home/alice                       │
│   ─────────                          ─────────                         │
│   ▸ ..                               ▸ ..                              │
│   ▸ Documents/                       ▸ work/                           │
│   ▸ Downloads/                       ▸ .config/                        │
│ ▶ screenshot.png       142.3 KB        notes.md           2.1 KB       │
│   readme.md             4.6 KB         todo.txt            612 B       │
│   notes.txt             1.3 KB                                         │
│                                                                        │
│  transferring screenshot.png: 47% · 67.3 / 142.3 KB · 18.4 KB/s        │
├────────────────────────────────────────────────────────────────────────┤
│ Tab switch · F5 copy · r refresh · Esc close       alice@home-server:22│
╰────────────────────────────────────────────────────────────────────────╯
```

Full key list: [docs/keybindings.md → File browser](docs/keybindings.md#file-browser-b-u).

### Behavior

- **Initial paths.** Local starts at the directory mterm was launched
  from (`$PWD`); remote starts at the SFTP working directory of the
  session (typically `$HOME`).
- **Sort.** Directories first (alphabetical), then files (alphabetical,
  case-insensitive). `..` is always pinned at the top of each list.
- **Progress.** During a copy, the status line at the bottom of the
  browser updates every 200 ms with percent, bytes transferred / total,
  and transfer rate. Press `Esc` to cancel cleanly — the partial file
  on the destination is left in place (no auto-rollback in v1; remove
  it manually if needed).
- **Errors.** SFTP errors (permission denied, file not found, broken
  pipes) surface as a one-line status. Browse around the panes to
  clear the message; the next successful listing erases it.
- **Hidden files.** Off by default — dotfiles are filtered out of both
  panes. Toggle with `.` and the panes re-list immediately.
- **Concurrency.** A single transfer at a time per browser. While one
  is running, `F5` is ignored until it completes or you cancel.

### Not yet shipped

`F6` rename, `F7` mkdir, `F8` delete (with confirmation), multi-select
with `Insert`/`Space`, view-file (pipe through `less`), edit-file,
sort/filter modes, drag-and-drop from the host terminal. None are hard
to add — each is a small standalone branch when the demand shows up.

## Copy mode (`^B [`)

Keyboard-driven selection out of the active session's scrollback. Skips
the terminal's mouse-selection layer entirely, so the window border and
tab/footer chrome never enter the clipboard.

Press `^B [` to enter. The body swaps to a frozen snapshot of all
available scrollback plus the live screen at the moment you opened the
mode — new output keeps streaming to the underlying session but doesn't
shift the view under you.

Full key list: [docs/keybindings.md → Copy mode](docs/keybindings.md#copy-mode-b-).

Selection is **line-range only** in v1: marking with `v` and moving the
cursor selects every line between (and including) the anchor and the
cursor; pressing `v` again drops the anchor. Without an anchor, Enter
copies just the cursor line. The bottom status bar reports state:

```
COPY · 12 lines · v unmark · Enter copy · q quit
```

The clipboard payload is **ANSI-stripped plain text**, lines joined with
`\n`, trailing whitespace trimmed from each line. mterm tries two delivery
paths in parallel — OSC 52 (works in iTerm2, kitty, alacritty, wezterm,
foot, recent gnome-terminal, recent Terminal.app, recent xterm) and a
native shell-out (`pbcopy` on macOS, `wl-copy` on Wayland, `xclip` or
`xsel` on X11). Either path succeeding counts; the footer reports
`copied N line(s) to clipboard` on success.

### Quick alternative — `^B m`

If you want to mouse-drag-select instead, `^B m` swaps the frame chars
for spaces so a drag doesn't pick them up. Second press restores. The
footer briefly reports `frame: minimal` / `frame: <prior>` so you know
which state you're in.

## hosts.yaml reference

All fields are optional unless noted. Field names are **lowercase** by
convention; unknown or wrong-cased keys surface as startup warnings
rather than being silently dropped.

### Host fields

| Field         | Type      | Default       | Notes |
|---------------|-----------|---------------|-------|
| `name`        | string    | **required**  | Friendly label shown in the picker. Also accepted as `hostname` or `host` (synonyms). |
| `address`     | string    | `name`        | IP or DNS-resolvable hostname to dial. If omitted, mterm dials `name` directly via DNS. |
| `user`        | string    | current user  | SSH username. |
| `port`        | int       | `22`          | SSH port. |
| `group`       | string    | (none)        | Picker group label. Slash-delimited paths nest sub-groups (`Home/Media`, `Work/Project1`). For multi-host hierarchies prefer the structured `groups:` form (see below). |
| `tags`        | []string  | (none)        | Tag list shown in `[…]` brackets next to the host in the picker. |
| `forwards`    | []Forward | (none)        | Port-forwarding rules. See below. |
| `bordercolor` | string    | (theme accent)| Hex `#RRGGBB`/`#RGB` OR a named color (see [Color names](#color-names)). When this host's tab is active, the window frame, active tab chip, footer key hints, and `user@host:port` segment all use this color. Invalid values warn and are ignored. |
| `identityfile`| string    | (none)        | Path to an SSH private key file. Tried *before* the agent, mimicking `ssh`'s fallback. `~/` and `$HOME` are expanded. Encrypted keys are not supported — load those via `ssh-add --apple-use-keychain` instead. |
| `on_connect`  | []string  | (none)        | Shell commands to send to the remote after handshake. Each gets a trailing CR. Common uses: auto-attach to tmux/screen (`tmux new -A -s mterm-$USER`), `cd` into a working dir. |
| `proxy_jump`  | string    | (none)        | Jump host to dial through. Two forms: (a) the **name** of another host in your registry (`proxy_jump: bastion`) — mterm uses that host's full config (port, identityfile, etc.); (b) a literal `[user@]host[:port]` (`proxy_jump: alice@bastion.example.com:2222`). Single-hop chains only in v1. ssh_config's `ProxyJump` is also honored; hosts.yaml wins on overlap. |
| `log`         | bool      | `true`        | Set `false` to opt out of per-session output logging for this host. |

### Forward fields

Each entry inside `forwards:`:

| Field      | Type   | Default      | Notes |
|------------|--------|--------------|-------|
| `type`     | string | `local`      | `local` (Local-Forward, like `ssh -L`) or `remote` (Remote-Forward, like `ssh -R`). |
| `bindaddr` | string | `localhost`  | Address to bind on the listening side. Empty = localhost. |
| `bindport` | int    | **required** | Port to listen on. |
| `dialaddr` | string | **required** | Destination address. |
| `dialport` | int    | **required** | Destination port. |

### Two ways to define hosts

There are two top-level keys: `hosts:` (a flat list) and `groups:` (a
nested tree). They can coexist in the same file.

| Form                  | Best for                                          |
|-----------------------|---------------------------------------------------|
| `hosts:` (flat)       | A handful of hosts; ssh_config overlays; quick one-offs |
| `groups:` (nested)    | Multi-section setups (Home/Work, Prod/Staging, per-customer); when order in the picker matters |

Sort rules (apply to both forms together):

- Hosts appear in the order they're written in the file.
- Groups appear in the order their first host appears in the file.
- ssh_config-only hosts (no yaml counterpart) trail at the end as "ungrouped."

### Nested form (recommended for hierarchies)

```yaml
groups:
  - name: Work
    groups:
      - name: Lab
        hosts:
          - name: lab-host-01
            address: 198.51.100.36
            user: alice
            log: false
      - name: Vendors
        hosts:
          - name: vendor-switch-01
            address: 198.51.100.81
            user: administrator

  - name: Home
    groups:
      - name: Media
        hosts:
          - name: media-server
            address: 192.168.1.50
            bordercolor: "#FF3344"
          - name: arr-stack
            address: 192.168.1.12
      - name: Development
        hosts:
          - name: home-server
            address: 192.168.1.20
            user: alice
```

Hosts inherit their slash-delimited group path from their position in
the tree — no `group:` field needed under nested groups (any value
would be overridden by the walk anyway). Sub-groups can nest arbitrarily
deep; the picker indents two spaces per level.

### Flat form (simple / ssh_config overlay)

```yaml
hosts:
  # Pure mterm host with every field shown.
  - name: prod-db-01
    address: 10.0.0.5
    user: alice
    port: 22
    group: production
    tags: [primary, postgres]
    bordercolor: "#FF3344"
    identityfile: ~/.ssh/prod-db-key
    log: true
    forwards:
      - type: local
        bindport: 5432
        dialaddr: 127.0.0.1
        dialport: 5432

  # Overlay extra metadata on an alias already in ~/.ssh/config.
  # `name` matches the ssh_config "Host" alias; only the listed fields
  # override; the rest (HostName, User, IdentityFile, …) come from
  # ssh_config.
  - name: my-existing-alias
    group: dev
    bordercolor: "#3344FF"

  # Name aliases are interchangeable — pick whichever reads best.
  - hostname: another-host       # same as `name:`
    address: 10.0.0.7
    user: ubuntu

  # Logging opt-out for a noisy host.
  - name: log-firehose
    address: 10.0.0.8
    log: false

  # A flat entry can land in a nested-form group via a slash path —
  # useful for ssh_config aliases you want to slot into a hierarchy
  # without restructuring.
  - name: ssh-aliased-host
    group: Work/Lab
    bordercolor: "#3344FF"
```

### Kitchen sink — both forms together

```yaml
# Nested groups define the hierarchy and the picker order.
groups:
  - name: Work
    groups:
      - name: Lab
        hosts:
          - name: lab-host-01
            address: 198.51.100.36
            user: alice
            identityfile: ~/.ssh/lab-host-key  # tried before the agent
            on_connect:                        # auto-attach a persistent tmux
              - "tmux new -A -s mterm-$USER"
          - name: lab-bastion
            address: 198.51.100.1
            user: alice
            forwards:
              - type: local
                bindport: 8080
                dialaddr: 127.0.0.1
                dialport: 80
          - name: caged-router
            address: 10.20.0.42       # only reachable via lab-bastion
            user: admin
            proxy_jump: lab-bastion   # SSH dial tunnels through the jump host
          - name: end-device-quick
            address: 10.20.0.43
            user: admin
            proxy_jump: alice@198.51.100.1:2222  # one-off literal: user@host:port
      - name: Vendors
        hosts:
          - name: vendor-switch-01
            address: 198.51.100.81
            user: administrator

  - name: Home
    groups:
      - name: Media
        hosts:
          - name: media-server
            address: 192.168.1.50
            bordercolor: "#FF3344"
            log: false                         # noisy host; skip session log
          - name: arr-stack
            address: 192.168.1.12
      - name: Development
        hosts:
          - name: home-server
            address: 192.168.1.20
            user: alice
            tags: [linux, primary]
            on_connect:
              - "cd /var/log/app"
              - "tail -F app.log"              # drops you into a live tail

# Flat overlays for ssh_config entries — no need to nest these.
hosts:
  - name: existing-ssh-alias-1
    bordercolor: "#3344FF"
  - name: existing-ssh-alias-2
    group: Work/Lab        # slip into a nested group via slash path
```

## ssh_config interaction

mterm reads `~/.ssh/config` first; each non-wildcard `Host` block becomes
a base host. Then `hosts.yaml` is layered on:

- A `hosts.yaml` entry whose `name` matches an ssh_config alias **overlays**
  — non-empty mterm fields win; the ssh_config base fills in everything
  else (`HostName`, `User`, `IdentityFile`, etc.).
- A `hosts.yaml` entry whose `name` does NOT match any ssh_config alias
  becomes a new mterm-source host.

In other words: ssh_config carries connection essentials; hosts.yaml adds
mterm-specific polish (groups, colors, logging, tags) and any hosts you
don't want in your ssh_config.

## Customization

Three independent surfaces — pick none, one, or all:

| Surface             | File                              | What it controls |
|---------------------|-----------------------------------|------------------|
| **Border colors**   | `~/.config/mterm/colors.yaml`     | Named color shortcuts usable from `bordercolor:` |
| **Themes**          | `~/.config/mterm/themes.yaml`     | The seven-color palette mterm uses for chrome |
| **Frame style**     | `~/.config/mterm/settings.yaml`   | Glyph set drawn for the window borders / dividers |

All three are re-read by `^B :` → `Reload config` so edits take effect
without restarting mterm.

### Color names

Names that `bordercolor:` accepts out of the box (case-insensitive). Add
or override any of them in `~/.config/mterm/colors.yaml`.

| Family            | Names |
|-------------------|-------|
| Reds / pinks      | `red` `#FF3344` · `darkred` `#8B0000` · `crimson` `#DC143C` · `hotpink` `#FF69B4` · `pink` `#FFB6C1` |
| Oranges / yellows | `orange` `#FF8C00` · `gold` `#FFD700` · `yellow` `#FFD93D` · `amber` `#FFB347` |
| Greens            | `green` `#33CC66` · `limegreen` `#32CD32` · `darkgreen` `#006400` · `olive` `#808000` · `teal` `#008080` |
| Blues / cyans     | `blue` `#3344FF` · `navy` `#000080` · `sky` `#87CEEB` · `cyan` `#00CCDD` |
| Purples           | `purple` `#8A2BE2` · `magenta` `#F92AAD` · `violet` `#8B5CF6` |
| Neutrals          | `white` `#FFFFFF` · `gray` / `grey` `#808080` · `black` `#000000` · `silver` `#C0C0C0` |
| Environment flags | `prodred` `#CC0000` · `stageyellow` `#E5C07B` · `devgreen` `#33CC66` |

Custom palette example (`~/.config/mterm/colors.yaml`):

```yaml
# User entries override built-ins of the same name.
myprodred: "#CC0000"
slackpurple: "#4A154B"
corpblue: "#3344FF"
vendorblue: "#1BA0D7"
```

Then in `hosts.yaml`:

```yaml
hosts:
  - name: prod-db
    address: 10.0.0.5
    bordercolor: corpblue        # resolves through colors.yaml
  - name: lab-box
    address: 10.0.0.6
    bordercolor: limegreen       # built-in
  - name: stage-db
    address: 10.0.0.7
    bordercolor: "#E5C07B"       # hex still works
```

### Themes (themes.yaml)

mterm ships three built-in themes — `midnight` (default), `matrix`,
`synthwave` — switchable from `^B :` → `Theme: <name>`. Add your own in
`~/.config/mterm/themes.yaml`; they appear in the palette alongside the
built-ins and survive across runs.

A theme is a flat map of seven color slots. Each is a hex string
(`#RRGGBB` or `#RGB`). **All slots are optional** — unset fields inherit
the value from `midnight`, so partial themes like "just darken the frame"
work cleanly without copying the whole palette.

| Slot        | What it tints |
|-------------|----------------|
| `accent`    | Active tab text, host name in title, focus highlights, palette selection bar background, PREFIX badge background, `^B` keys in the footer |
| `frame`     | Window borders and dividers |
| `dim`       | Inactive tab text, secondary text, group headers in the picker, palette command labels |
| `text`      | Primary foreground (where it's distinct from terminal output) |
| `warning`   | `[sync N]` / `[scroll N]` / `[restored]` status badges, broadcast sync `*` glyph, tab activity indicator |
| `error`     | Connection errors, palette validation errors |
| `connected` | Tab status dot for a connected session, silence indicator on idle background tabs |

Example — a Nord-inspired theme plus a quick partial override:

```yaml
# ~/.config/mterm/themes.yaml
nordlike:
  accent:    "#88C0D0"
  frame:     "#3B4252"
  dim:       "#4C566A"
  text:      "#ECEFF4"
  warning:   "#EBCB8B"
  error:     "#BF616A"
  connected: "#A3BE8C"

just-the-accent:
  accent: "#FF00FF"
  # frame/dim/text/warning/error/connected → inherit midnight
```

Theme names are case-insensitive and trimmed; the palette lists user
themes alphabetically after the built-ins.

### Frame style (settings.yaml)

The chrome borders (window frame, dividers between tab strip / body /
footer) can be drawn in six different glyph sets. Set the default in
`~/.config/mterm/settings.yaml`:

```yaml
frame: rounded   # default — matches the pre-customization look
```

Six options:

| Name      | Corners + edge sample          | Notes |
|-----------|--------------------------------|-------|
| `rounded` | `╭─╮ ├ ┤ ╰─╯`                  | default; soft modern look |
| `square`  | `┌─┐ ├ ┤ └─┘`                  | sharp corners |
| `thick`   | `┏━┓ ┣ ┫ ┗━┛`                  | heavy lines — handy for "loud prod" feel |
| `double`  | `╔═╗ ╠ ╣ ╚═╝`                  | classic two-line |
| `ascii`   | `+-+ + + +-+`                  | for terminals without box-drawing support |
| `minimal` | spaces                         | quiet chrome, no visible border |

Switch at runtime from `^B :` → `Frame: <name>`. Unknown names fall back
to the previous setting and surface as a warning.

**Quick toggle:** `^B m` flips between `minimal` and your previously
active style. Handy for clipboard grabs (see [Copy mode](#copy-mode-b-)).

`settings.yaml` is also where future global toggles (a confirm-quit
switch, a default theme name, etc.) will land — single file, room to grow.

## Configuration directory

mterm lives under `~/.config/mterm/`:

| Path                                              | Purpose                          |
|---------------------------------------------------|----------------------------------|
| `~/.config/mterm/hosts.yaml`                      | Host overlay (see [hosts.yaml reference](#hostsyaml-reference)) |
| `~/.config/mterm/logs/<host>/<YYYYMMDD-HHMMSS>.log` | Per-session output logs        |
| `~/.config/mterm/history.json`                    | Last-connected timestamps per host (powers the picker's "2h ago" decorations) |
| `~/.config/mterm/workspaces.json`                 | Saved workspaces (named tab sets) |
| `~/.config/mterm/colors.yaml`                     | User-defined `bordercolor` names — yaml map of name → hex, e.g. `myprodred: "#CC0000"`. Merged on top of mterm's built-in palette. |
| `~/.config/mterm/themes.yaml`                     | User-defined themes (yaml map of theme-name → field map). Unset fields inherit Midnight. Appears in `^B :` → `Theme: <name>`. |
| `~/.config/mterm/settings.yaml`                   | Global toggles. Today: `frame: rounded\|square\|thick\|double\|ascii\|minimal`. |

The directory is created automatically on first run. Logs are raw bytes
including ANSI escapes — replay faithfully with `less -R <file>`, or
strip ANSI for grep:

```bash
sed 's/\x1b\[[0-9;]*m//g' ~/.config/mterm/logs/prod-db-01/*.log | grep ERROR
```

## Common keys

The prefix is **`^B`** (Ctrl-B). Twelve chords cover ~90% of the use:

| Key       | Action |
|-----------|--------|
| `^B c`    | open the picker (new connection) |
| `^B n` / `^B p` | next / previous tab |
| `^B 1..9` | jump to tab N |
| `^B x`    | close the current tab |
| `^B s`    | toggle the current tab in the broadcast sync set |
| `^B m`    | toggle minimal frame (clipboard-friendly chrome) |
| `^B [`    | enter keyboard copy mode |
| `^B u`    | open the SFTP file browser |
| `^B :`    | open the command palette |
| `^B ?`    | help overlay |
| `^B q`    | quit mterm |

Full reference — every chord, every overlay, every mode: **[docs/keybindings.md](docs/keybindings.md)**.

## Building & distribution

Three scripts in the repo root: `build.sh` (cross-compile + signed darwin
zips), `installer.sh` (universal macOS .pkg), `debian.sh` (.deb for
amd64/arm64). Full guide including code-signing + notarization setup:
**[docs/building.md](docs/building.md)**.

## License

TBD.
