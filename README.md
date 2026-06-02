# mterm

A Go-powered multi-connection SSH TUI. Tabbed sessions, fuzzy host picker, full-fidelity VT terminal, tmux-style prefix navigation. Aiming to be the no-bullshit alternative to SecureCRT for engineers who live in the terminal.

## Quick start

```bash
go build -o mterm .
./mterm
```

On first launch mterm reads `~/.ssh/config` for your existing host aliases and lists them in the picker. To add hosts that aren't in ssh_config — or to overlay extra metadata (groups, color tags, port forwards, logging opt-out, etc.) on existing ones — create `~/.config/mterm/hosts.yaml`:

```yaml
hosts:
  - name: prod-db
    address: 10.0.0.5
    user: dale
    group: production
    bordercolor: "#FF3344"
```

The `~/.config/mterm/` directory is created automatically on first run.

## Features

- Tabbed SSH sessions with `^B` prefix navigation (tmux-style)
- Native Go SSH client — agent auth + per-host `identityfile:` fallback
  (so a reboot-cleared agent doesn't break connections when the key is
  on disk), lenient `known_hosts`
- Full-fidelity VT terminal: cursor, 24-bit color
- Mouse-wheel scrollback on the active session — scroll up to browse
  history, type any key to snap back to live. Viewport stays anchored to
  the same content as new output streams in
- Activity / silence indicators in the tab strip — background tabs that
  receive new output get a warning-color glyph; tabs that go quiet after
  activity transition to an accent-color "done" glyph. Focusing a tab
  clears its badge.
- Per-host startup commands — `on_connect: [...]` in hosts.yaml runs
  shell commands right after handshake (auto-attach to tmux/screen,
  jump into a working dir, etc.)
- Workspaces — save+restore named tab sets via the command palette.
  "Save current tabs as workspace…" pops a name prompt; "Restore
  workspace: …" reopens every host in one keystroke; missing hosts
  surface in the status line. Persisted at `~/.config/mterm/workspaces.json`.
- Auto-reconnect on transport failure
- Fuzzy host picker reading `~/.ssh/config` + `~/.config/mterm/hosts.yaml`
  with per-host connection state — `◉` currently in a tab, `●` previously
  connected (with "5m ago" / "2h ago" / "3d ago" relative time), `○` never
  connected; group headers show counts (`▸ HOME (3)`)
- Themable UI (Midnight / Matrix / Synthwave) with frame, tab strip, footer
- Per-host visual identity — `bordercolor:` tints the frame, active tab chip, and footer key hints so prod ≠ staging at a glance
- Per-session output logging to `~/.config/mterm/logs/<host>/<timestamp>.log` (default on; per-host opt-out)
- Broadcast input — `^B s` toggles a tab into a sync set; type once, fan to every member
- Command palette (`^B :`) with fuzzy command search
- Help overlay (`^B ?`)
- Port-forwarding panel (`^B f`)

Roadmap and pending items: `docs/superpowers/roadmap.md`.

## Keybindings

The prefix is **`^B`** (Ctrl-B), tmux-style. Press it, then the command key.

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `^B c`    | open the picker (new connection)                    |
| `^B n`    | next tab                                            |
| `^B p`    | previous tab                                        |
| `^B 1..9` | jump to tab N                                       |
| `^←` / `^→`     | previous / next tab in session mode (no prefix needed). Note: shadows shell word-jump on the remote — use Option-Left/Right on macOS for word-jump |
| `^PgUp` / `^PgDn` | previous / next tab in session mode (no prefix needed; no shell conflict) |
| `^B x`    | close the current tab                               |
| `^B f`    | open the port-forwards panel                        |
| `^B s`    | toggle the current tab in the broadcast sync set    |
| `^B :`    | open the command palette                            |
| `^B ?`    | open the keybinding help overlay                    |
| `^B D`    | dump every goroutine's stack to `/tmp/mterm-stacks-…` for diagnosis (status line shows the file path) |
| `^B q`    | quit mterm                                          |
| `^C`      | passes through to the remote session (SIGINT)       |
| wheel ↑/↓ | scroll the active tab's scrollback; any key snaps back to live |

In the picker:

| Key      | Action                |
|----------|-----------------------|
| (type)   | fuzzy-filter hosts    |
| ↑ / ↓    | move cursor           |
| Enter    | connect               |
| Esc      | back to active session|

In the command palette:

| Key      | Action                |
|----------|-----------------------|
| (type)   | filter commands       |
| ↑ / ↓    | move cursor           |
| Enter    | run                   |
| Esc      | cancel                |

## hosts.yaml reference

All fields are optional unless noted. Field names are **lowercase** by convention; unknown or wrong-cased keys surface as startup warnings rather than being silently dropped.

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
| `bordercolor` | string    | (theme accent)| Hex `#RRGGBB`/`#RGB` OR a named color (see ["Built-in color names"](#built-in-color-names) below). When this host's tab is active, the window frame, active tab chip, footer key hints, and `user@host:port` segment all use this color. Invalid values warn and are ignored. |
| `identityfile`| string    | (none)        | Path to an SSH private key file. Tried *before* the agent, mimicking `ssh`'s fallback. `~/` and `$HOME` are expanded. Encrypted keys are not supported — load those via `ssh-add --apple-use-keychain` instead. |
| `on_connect`  | []string  | (none)        | Shell commands to send to the remote after handshake. Each gets a trailing CR. Common uses: auto-attach to tmux/screen (`tmux new -A -s mterm-$USER`), `cd` into a working dir. |
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

### Built-in color names

Names that `bordercolor:` accepts out of the box (case-insensitive). Add or
override any of them in `~/.config/mterm/colors.yaml`.

| Family    | Names                                                          |
|-----------|----------------------------------------------------------------|
| Reds / pinks   | `red` `#FF3344` · `darkred` `#8B0000` · `crimson` `#DC143C` · `hotpink` `#FF69B4` · `pink` `#FFB6C1` |
| Oranges / yellows | `orange` `#FF8C00` · `gold` `#FFD700` · `yellow` `#FFD93D` · `amber` `#FFB347` |
| Greens         | `green` `#33CC66` · `limegreen` `#32CD32` · `darkgreen` `#006400` · `olive` `#808000` · `teal` `#008080` |
| Blues / cyans  | `blue` `#3344FF` · `navy` `#000080` · `sky` `#87CEEB` · `cyan` `#00CCDD` |
| Purples        | `purple` `#8A2BE2` · `magenta` `#F92AAD` · `violet` `#8B5CF6` |
| Neutrals       | `white` `#FFFFFF` · `gray` / `grey` `#808080` · `black` `#000000` · `silver` `#C0C0C0` |
| Environment flags | `prodred` `#CC0000` · `stageyellow` `#E5C07B` · `devgreen` `#33CC66` |

Custom palette example (`~/.config/mterm/colors.yaml`):

```yaml
# User entries override built-ins of the same name.
myprodred: "#CC0000"
slackpurple: "#4A154B"
msftblue: "#3344FF"
ciscoblue: "#1BA0D7"
```

Then in `hosts.yaml`:

```yaml
hosts:
  - name: prod-db
    address: 10.0.0.5
    bordercolor: msftblue        # resolves through colors.yaml
  - name: lab-box
    address: 10.0.0.6
    bordercolor: limegreen       # built-in
  - name: stage-db
    address: 10.0.0.7
    bordercolor: "#E5C07B"       # hex still works
```

`^B :` → `Reload config` re-reads both files so edits take effect without
restarting mterm.

### Two ways to define hosts

There are two top-level keys: `hosts:` (a flat list) and `groups:` (a nested
tree). They can coexist in the same file.

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
          - name: sao-dev
            address: 10.122.26.36
            user: dale
            log: false
      - name: MSFT
        hosts:
          - name: msft-optical-1
            address: 10.122.161.81
            user: administrator

  - name: Home
    groups:
      - name: Media
        hosts:
          - name: plex-ubuntu
            address: 192.168.2.50
            bordercolor: "#FF3344"
          - name: binarr
            address: 192.168.2.12
      - name: Development
        hosts:
          - name: sys-dev
            address: 192.168.2.20
            user: dale
```

Hosts inherit their slash-delimited group path from their position in the
tree — no `group:` field needed under nested groups (any value would be
overridden by the walk anyway). Sub-groups can nest arbitrarily deep; the
picker indents two spaces per level.

### Flat form (simple / ssh_config overlay)

```yaml
hosts:
  # Pure mterm host with every field shown.
  - name: prod-db-01
    address: 10.0.0.5
    user: dale
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
          - name: sao-dev
            address: 10.122.26.36
            user: dale
            identityfile: ~/.ssh/sao-dev-key   # tried before the agent
            on_connect:                        # auto-attach a persistent tmux
              - "tmux new -A -s mterm-dale"
          - name: lab-bastion
            address: 10.122.26.1
            user: dale
            forwards:
              - type: local
                bindport: 8080
                dialaddr: 127.0.0.1
                dialport: 80
      - name: MSFT
        hosts:
          - name: msft-optical-1
            address: 10.122.161.81
            user: administrator

  - name: Home
    groups:
      - name: Media
        hosts:
          - name: plex-ubuntu
            address: 192.168.2.50
            bordercolor: "#FF3344"
            log: false                         # noisy host; skip session log
          - name: binarr
            address: 192.168.2.12
      - name: Development
        hosts:
          - name: sys-dev
            address: 192.168.2.20
            user: dale
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

mterm reads `~/.ssh/config` first; each non-wildcard `Host` block becomes a base host. Then `hosts.yaml` is layered on:

- A `hosts.yaml` entry whose `name` matches an ssh_config alias **overlays** — non-empty mterm fields win; the ssh_config base fills in everything else (`HostName`, `User`, `IdentityFile`, etc.).
- A `hosts.yaml` entry whose `name` does NOT match any ssh_config alias becomes a new mterm-source host.

In other words: ssh_config carries connection essentials; hosts.yaml adds mterm-specific polish (groups, colors, logging, tags) and any hosts you don't want in your ssh_config.

## Configuration directory

mterm lives under `~/.config/mterm/`:

| Path                                              | Purpose                          |
|---------------------------------------------------|----------------------------------|
| `~/.config/mterm/hosts.yaml`                      | Host overlay (this document)     |
| `~/.config/mterm/logs/<host>/<YYYYMMDD-HHMMSS>.log` | Per-session output logs        |
| `~/.config/mterm/history.json`                    | Last-connected timestamps per host (powers the picker's "2h ago" decorations) |
| `~/.config/mterm/workspaces.json`                 | Saved workspaces (named tab sets) |
| `~/.config/mterm/colors.yaml`                     | User-defined `bordercolor` names (`yaml` map of name → hex, e.g. `myprodred: "#CC0000"`). Merged on top of mterm's built-in palette. |

The directory is created automatically on first run. Logs are raw bytes including ANSI escapes — replay faithfully with `less -R <file>`, or strip ANSI for grep:

```bash
sed 's/\x1b\[[0-9;]*m//g' ~/.config/mterm/logs/prod-db-01/*.log | grep ERROR
```

## Building for distribution

The repo ships a `build.sh` that cross-compiles stripped binaries to `dist/`
for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64 and emits a
`SHA256SUMS` file. Windows is excluded because mterm uses `SIGUSR1` for the
in-app goroutine dump.

```bash
./build.sh                 # all default platforms
./build.sh darwin/arm64    # build just one
```

### Codesign + notarize for macOS distribution

Unsigned macOS binaries hit Gatekeeper and require a `xattr -d
com.apple.quarantine` dance. If you have a Developer ID Application
certificate, `build.sh` can codesign + notarize the darwin binaries so
colleagues just unzip and run.

One-time keychain setup:

```bash
# Generate an app-specific password at
# https://appleid.apple.com → Sign-In and Security → App-Specific Passwords.
xcrun notarytool store-credentials mterm-notary \
    --apple-id <your-apple-id> \
    --team-id <10-char-team-id> \
    --password <app-specific-password>
```

Build + sign + notarize:

```bash
export APPLE_SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export APPLE_KEYCHAIN_PROFILE="mterm-notary"
./build.sh
```

Output goes to `dist/mterm-darwin-<arch>.zip` — that's the file to hand out.
First launch on a colleague's machine does an online notarization check (~1s)
then runs cleanly with no Gatekeeper prompt.

To inspect what identities are available:

```bash
security find-identity -v -p codesigning
```

### macOS .pkg installer

For a one-click install experience, `installer.sh` fuses the two darwin
binaries into a universal binary, wraps it in a signed + notarized .pkg
that drops `mterm` into `/usr/local/bin/`, and staples the notarization
ticket. Colleagues double-click the .pkg, enter their password, then run
`mterm` from any terminal afterward.

Requires a Developer ID **Installer** certificate (different from the
Application cert used above — Apple issues them separately):

```bash
# See your installer identities:
security find-identity -v -p basic | grep "Developer ID Installer"

# Then build + sign + notarize the installer:
export APPLE_SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export APPLE_INSTALLER_SIGNING_IDENTITY="Developer ID Installer: Your Name (TEAMID)"
export APPLE_KEYCHAIN_PROFILE="mterm-notary"
./build.sh && ./installer.sh
```

Output: `dist/mterm-installer.pkg` (~12 MiB; universal binary inside).

Override defaults via:
- `PKG_IDENTIFIER` (default `dev.mterm`) — reverse-DNS bundle id
- `PKG_VERSION`  (default derived from `git describe`) — installer version

Uninstall (for colleagues if asked):

```bash
sudo rm /usr/local/bin/mterm
pkgutil --forget dev.mterm
```

### Debian / Ubuntu / Linux Mint .deb installer

`debian.sh` wraps the cross-compiled Linux binaries into installable
`.deb` packages — one per architecture. Colleagues install with `apt`
or `dpkg`; mterm lands at `/usr/bin/mterm` so it's on PATH everywhere.

```bash
# macOS:  brew install dpkg
# Linux:  already built in
./build.sh && ./debian.sh
```

Output: `dist/mterm_<version>_amd64.deb` and `dist/mterm_<version>_arm64.deb`.

The version string defaults to `git describe`; if there are no tags
yet, the SHA is prefixed with `0.0.0+` so Debian's version parser
accepts it. Override either piece via env vars when you ship:

```bash
DEB_VERSION=1.0.0 \
DEB_MAINTAINER="Your Name <you@example.com>" \
    ./debian.sh
```

Colleagues install:

```bash
sudo apt install ./mterm_<version>_amd64.deb   # or _arm64
# or, equivalently with plain dpkg (no auto-deps; mterm has none):
sudo dpkg -i mterm_<version>_amd64.deb
```

Uninstall:

```bash
sudo apt remove mterm    # or: sudo dpkg -r mterm
```

The .deb is not signed — for internal distribution this is fine, and
the `SHA256SUMS` file in `dist/` covers integrity verification.
Signing requires a Debian package-signing key and `dpkg-sig`;
deferred until there's a reason to.

## License

TBD.
