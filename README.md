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
- Native Go SSH client + agent auth + lenient `known_hosts`
- Full-fidelity VT terminal: cursor, 24-bit color, scrollback, copy mode
- Auto-reconnect on transport failure
- Fuzzy host picker reading `~/.ssh/config` + `~/.config/mterm/hosts.yaml`
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
| `^B x`    | close the current tab                               |
| `^B f`    | open the port-forwards panel                        |
| `^B s`    | toggle the current tab in the broadcast sync set    |
| `^B :`    | open the command palette                            |
| `^B ?`    | open the keybinding help overlay                    |
| `^B q`    | quit mterm                                          |
| `^C`      | passes through to the remote session (SIGINT)       |

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
| `bordercolor` | string    | (theme accent)| Hex `#RRGGBB` or `#RGB`. When this host's tab is active, the window frame, active tab chip, footer key hints, and `user@host:port` segment all use this color. Invalid values warn and are ignored. |
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
          - name: binarr
            address: 192.168.2.12
      - name: Development
        hosts:
          - name: sys-dev
            address: 192.168.2.20
            user: dale
            tags: [linux, primary]

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

The directory is created automatically on first run. Logs are raw bytes including ANSI escapes — replay faithfully with `less -R <file>`, or strip ANSI for grep:

```bash
sed 's/\x1b\[[0-9;]*m//g' ~/.config/mterm/logs/prod-db-01/*.log | grep ERROR
```

## License

TBD.
