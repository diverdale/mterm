# mterm Roadmap

Living plan for getting mterm to "legit SecureCRT alternative for working engineers."
Each item below is the seed for its own brainstorm → spec → plan → branch cycle.
Order in the "Suggested sequence" section reflects dependencies and effort; nothing
here is a deadline.

**Sizing key:** S ≈ a session; M ≈ a few sessions; L ≈ a week of focused work;
XL ≈ structural change to the app, multiple branches.

---

## Already shipped

- Tabbed SSH sessions with `^B`-prefix navigation
- Native Go SSH client + agent auth + lenient known_hosts
- Full-fidelity VT terminal (incl. cursor, colors, scrollback, copy mode)
- Auto-reconnect on transport failures
- Fuzzy host picker reading `~/.ssh/config` + `~/.config/mterm/hosts.yaml`
- Themable UI (Midnight / Matrix / Synthwave) with frame, tab strip, footer
- Per-host visual identity (`bordercolor:` tints frame / active tab / footer hints)
- Per-session output logging to `~/.config/mterm/logs/<host>/<timestamp>.log`
  (default-on; per-host `log: false` opt-out; "Show log path for current tab"
  palette command)
- Broadcast input — `^B s` toggles the active tab into a sync set; every
  keystroke in any sync-set tab fans out to the rest. Warning-color asterisk
  in the tab chip + `[sync N]` count in the window title.
- Mouse-wheel scrollback on the active session tab; any keystroke snaps the
  view back to live. Auto-anchored: the user's viewport stays glued to the
  same content as new output pushes scrollback further up. Title gains a
  warning-color `[scroll N]` indicator when not at live.
- Command palette (`^B :`) and help overlay (`^B ?`)
- Port-forwarding plumbing (panel exists; integration deferred)
- Per-tab session timer, animated status spinner
- `^B m` minimal-frame toggle — flips chrome to spaces for clipboard-friendly
  mouse-drag grabs; second press restores the prior style
- `^B [` keyboard copy mode — vim-style scrollback navigation, line-range
  selection with `v` mark + Enter, OSC 52 + native (pbcopy/wl-copy/xclip)
  fallback delivery; frame chars never enter the payload

---

## Tier 1 — table stakes for SecureCRT defectors

### Clipboard copy mode (tmux-style) ✓ shipped
Both copy paths now exist: `^B m` for the lightweight "flip frame and
drag-select" workflow, and `^B [` for fully-keyboard line-range selection
that pushes through OSC 52 + native helpers (pbcopy / wl-copy / xclip /
xsel). Selection model is line-range only in v1 — character-precise
selection is a small follow-on if/when someone hits the limit.

Follow-ons left on the table:
- Character-precise selection (cells within a line)
- Scrollback regex search (`/` inside copy mode) — overlaps with the
  Scrollback search Tier 1 item below
- Remember last selection across re-entries

### Broadcast input (sync mode)
**What:** Type once, keystrokes go to every tab in the sync set. Toggle membership per tab.
**Why:** THE killer feature for NOC/SRE work — run the same command across N routers at once.
**Size:** M
**Open:** Sync-set membership UI — palette-driven "Add tab to sync" or per-tab `^B s` toggle? Sync-on indicator on tabs?
**Depends on:** nothing.

### Per-session logging
**What:** Timestamped log file per host (`~/.config/mterm/logs/<host>/<date>.log`); appended on every byte received.
**Why:** Change-management evidence; replay; `grep` across yesterday's sessions.
**Size:** S–M
**Open:** Raw bytes vs ANSI-stripped vs both; rotation policy; opt-in vs always-on.
**Depends on:** nothing. Unblocks the "log to file" trigger action.

### Scrollback search
**What:** `/` in copy mode prompts for a regex; jump to matches, `n`/`N` to navigate, highlight all.
**Why:** 30k lines of `kubectl logs` and you need the one stack trace.
**Size:** M
**Open:** Regex vs substring default; case-insensitivity toggle.
**Depends on:** existing copy mode (already shipped).

### ProxyJump / bastion support
**What:** Honor `ProxyJump` from `~/.ssh/config` and a `proxy_jump:` field in `hosts.yaml`.
**Why:** Corporate networks live behind bastions — without this, mterm can't reach the targets that matter.
**Size:** M
**Open:** Multi-hop chains (rare) vs single hop (covers ~95%).
**Depends on:** nothing. `golang.org/x/crypto/ssh` supports Dial-over-conn already.

### File transfer (SFTP) ✓ shipped
Two-pane file browser via `^B u`. Live progress + cancel during copies.
Local pane reads `os.ReadDir`; remote pane reads via `sftp.Client`. F5
copies the highlighted entry from active pane to the other. Hidden files
toggled with `.`; refresh with `r`. Future iterations may add mkdir,
delete, rename, multi-select, edit, and view-file.

### File transfer (SFTP) — original entry
**What:** `^B u` upload, `^B d` download, tiny TUI file browser. SFTP over the existing SSH conn.
**Why:** SecureCRT users live in SFTP.
**Size:** L
**Open:** Browser UX; drag-drop from host terminal; progress bar treatment; zmodem (rz/sz) auto-detection as a follow-on.
**Depends on:** nothing.

### Mode-aware mouse forwarding
**What:** When the remote app turns on mouse tracking (DECSET `?1000`, `?1002`, `?1003`, `?1006`), forward mouse events — wheel, click, drag — to the remote as encoded SGR (`\x1b[<...M`) or legacy X10 sequences instead of consuming them locally. When tracking is off, keep today's behavior (wheel → mterm scrollback).
**Why:** Without this, full-screen TUIs run over SSH — opencode, modern vim with `set mouse=a`, htop, ncdu, lazygit — are crippled. Wheel events go to mterm's local scrollback instead of the app the user is looking at. Surfaced during dogfood on opencode-over-SSH.
**Size:** M
**Open:** Where to track the DECSET state — likely a small addition to `internal/terminal` exposing an `IsMouseTracked()` flag the app can query. Encoding flavor — start with SGR (1006) since it's the modern default and matches what every recent TUI emits; X10 only if a real terminal forces our hand. Whether to ship wheel-only first (fixes the opencode case in <100 LOC) or click+drag together (covers vim's full mouse mode).
**Depends on:** `xvt` emulator surfacing the mouse-mode flag.

---

## Tier 2 — what makes mterm *better* than SecureCRT

### Triggers
**What:** Per-host regex patterns matched against incoming output; actions: highlight, beep, desktop notify, append-to-log, send snippet.
**Why:** Watch a deploy without watching; auto-respond to predictable prompts; flag `ERROR` automatically.
**Size:** L
**Open:** Match pre- or post-ANSI text; compiled at config-load vs lazy; action sandboxing; rate-limiting.
**Depends on:** per-session logging (for log-to-file action); snippets (for send-snippet action).

### Per-host visual identity
**What:** `theme:` and/or `border_color:` per host in `hosts.yaml`. Frame and active tab pick up that color.
**Why:** prod = red, staging = yellow, dev = green. Single biggest "did I just `rm -rf` prod?" prevention.
**Size:** S
**Open:** Override whole theme vs only border.
**Depends on:** existing Theme abstraction (already shipped).

### Snippets / send-command palette
**What:** Saved commands in `~/.config/mterm/snippets.yaml`; `^B e` (placeholder — `^B m` is now the frame toggle, final letter TBD) opens a palette; enter sends to active session. Per-host and global namespaces.
**Why:** Stop re-typing `kubectl get pods -A -o wide` and `show ip int brief`.
**Size:** M
**Open:** Variable interpolation (`{{host}}`, prompted vars); multi-line sequencing with wait-for-prompt.
**Depends on:** command palette (already shipped) — same UI pattern.

### Live host monitor panel
**What:** Optional split-pane panel showing the active host's CPU, memory, disk, load, network throughput, and top processes in real time, alongside the shell. Sparkline history for CPU/network, bar gauges for disk/memory, list of top-N processes by CPU/mem. Toggle with a prefix key (placeholder: `^B h` for "host monitor" — pick the final letter when this lands; note `^B m` is now the frame toggle).
**Why:** "Operational dashboard with a shell inside it" — sees what the box is doing without leaving the SSH session or installing a remote agent. Inspired by essh's host monitor, which is its single biggest differentiator vs every other multiplexer in the category. If mterm wants to compete head-on with essh on operator-tool feel, this is the gap to close.
**Size:** L
**Open:** Collection strategy — periodic `top -bn1` / `vmstat` / `/proc/stat` reads over an SSH exec channel (no remote agent), parsed per-distro (Linux flavors differ enough that a small adapter layer is needed; macOS targets would need a separate path). Cadence (1s? 5s? configurable?). Where the panel lives (right side / bottom?) — interacts with future pane splitting. Caching when the session is backgrounded. Whether to also probe network round-trip from the local side.
**Depends on:** nothing strictly, but composes with pane splitting (Tier 4) for the cleanest layout.

### Left sidebar — persistent windows list
**What:** Replace (or augment) the top tabstrip with a tall left-edge sidebar showing all open tabs as chips with host + status + connection age. tmux's `^B s` window-list, but always visible. Body shrinks; you always see what else is running.
**Why:** Once you're juggling 4+ sessions the horizontal tabstrip becomes the limiting factor. A vertical sidebar scales to N tabs comfortably and surfaces inactive-tab state (activity, silence, sync membership) without cycling.
**Size:** L
**Open:** Width policy (fixed cols vs proportional)? Toggle keybinding (`^B b`?) to hide/show? Replace tabstrip entirely or coexist? Per-chip activity indicator (recent output, idle time)?
**Depends on:** activity/silence indicators (Tier-2) feed naturally into this. Snapshot architecture handles the per-tab body render fine; the chrome refactor is the main lift.

### Workspaces
**What:** `^B w s <name>` saves current tab set; `^B w r <name>` restores. Stored as YAML.
**Why:** Resume yesterday's investigation in one keystroke.
**Size:** M
**Open:** Just hostlist or also pane layout (once splits exist)? Auto-restore on launch?
**Depends on:** pane splits (for layout-aware saves) — can ship hostlist-only first.

### Activity / silence indicators
**What:** Tab chip flashes / badges when a background tab gets new output. Separate badge when output has been silent for N seconds after activity (deploy-done detection).
**Why:** Long-running deploys; "ping me when this builds."
**Size:** S
**Open:** Per-tab opt-in vs always-on; configurable silence threshold.
**Depends on:** nothing.

---

## Tier 3 — polish that compounds

### True color + copy-on-select
**What:** 24-bit color passthrough; selection auto-copies to system clipboard (OSC52 + native fallback).
**Why:** Modern terminal expectations.
**Size:** M
**Open:** OSC52 only (works over SSH) vs also native `pbcopy`/`xclip`.
**Depends on:** nothing.
**Note:** The "forward mouse events to the VT" piece of the original entry has been promoted to Tier 1 as a standalone — see [Mode-aware mouse forwarding](#mode-aware-mouse-forwarding).

### Per-host startup commands
**What:** `on_connect: ["screen -DR work"]` in `hosts.yaml`; commands sent after handshake.
**Why:** Auto-attach screen, set environment, `cd` to a working dir.
**Size:** S
**Open:** Bundled one-shot vs sequenced with wait-for-prompt.
**Depends on:** nothing. Doubles as the cheap path to remote-multiplexer detach (see below).

### Connection groups in picker
**What:** Picker rows grouped by `group:` field in `hosts.yaml`; collapsible group headers.
**Why:** 50 routers + 30 servers + 10 dev boxes — flat list becomes unusable.
**Size:** S
**Open:** Group ordering; default-collapsed vs default-expanded.
**Depends on:** nothing. (Finishes the earlier "how do I group items" question.)

### Configurable keymap
**What:** User-overridable prefix and binding map in `~/.config/mterm/keymap.yaml`. Validation + conflict detection.
**Why:** Some users will scream at `^B` and the v2 prefix flag we wrote ourselves.
**Size:** M
**Open:** YAML schema; how to express modifier combinations; whether to surface conflicts at startup or lazily.
**Depends on:** nothing — refactor of existing key dispatch.

---

## Tier 4 — the big bets

### Pane splitting
**What:** A tab becomes a layout tree of panes. `^B %` vertical split, `^B "` horizontal split, `^B <arrow>` navigate, `^B z` zoom, `^B x` close pane. "Combining tabs" = moving a pane between tab trees.
**Why:** tmux/SecureCRT users expect it; it's the single most-requested missing thing.
**Size:** XL — structural refactor of `sessionTab` from "wraps one SSH+VT" to "layout tree whose leaves wrap one SSH+VT each."
**Open:** Binary splits vs arbitrary tiling; fresh SSH conn per pane vs shared channel; resize math.
**Depends on:** nothing strictly. Broadcast input, workspaces, activity indicators all become pane-aware downstream.

### Detach / reattach (screen-style)
**What:** Sessions survive closing mterm; relaunch and pick them back up.
**Why:** Network drop, laptop sleep, accidental exit — don't lose the running deploy.
**Size:** depends on architecture; see below.

This one needs an architectural decision before it can be specced. Two real options
and one hybrid:

**Architecture A — Local mterm daemon.**
Split mterm into `mterm-server` (long-running, holds the SSH conns + VT state) and `mterm` (thin client, connects via unix socket and renders). Detach = exit client; daemon keeps running. Reattach = launch client; resumes from live VT state.
- Pros: Works against any remote. Zero remote config. True session preservation.
- Cons: Massive shift — process split, IPC protocol, lifecycle (start-on-demand, idle exit), upgrade story. **Size: XL.**

**Architecture B — Remote multiplexer passthrough.**
mterm wraps connections with `tmux new -A -s mterm-{user}` (or `screen -DR`). Detach = close tab; remote tmux keeps the shell. Reattach = open the host again; tmux reattaches.
- Pros: Two-day project. Real shell + scrollback persistence on the remote.
- Cons: Requires tmux/screen installed remotely (most servers have it). Remote reboot loses everything. Local mterm chrome (frame, tabs) doesn't survive — only the shell does. **Size: M.**

**Architecture C — Hybrid.**
Ship B first as `auto_multiplex: tmux` (per host) for cheap wins; revisit A later if users want true local persistence including mterm chrome.

**Recommendation:** start with B (folds into Per-host startup commands as a config knob). Promote to A as its own brainstorm cycle once we have user signal.

---

## Suggested sequence

Roughly ordered by dependency + effort + bang-for-buck. Each is its own branch.

1. **Per-host visual identity** (S) — tiny diff, big safety, no deps
2. **Connection groups in picker** (S) — finishes earlier deferred work
3. **Per-session logging** (S–M) — unblocks trigger "log to file"
4. **Broadcast input** (M) — the killer feature
5. **Scrollback search** (M) — improves shipped copy mode
5a. **Mode-aware mouse forwarding** (M) — unblocks opencode, vim mouse, htop over SSH
6. **ProxyJump** (M) — unblocks every user behind a bastion
7. **Per-host startup commands** (S) — quick add; enables Architecture B trivially
8. **Detach/reattach via Architecture B** (M) — `auto_multiplex: tmux` config knob
9. **Snippets** (M)
10. **Activity / silence indicators** (S)
11. **Workspaces** — hostlist-only first (M)
12. **SFTP file transfer** (L)
13. **Triggers** (L)
14. **True color + copy-on-select** (M) — mouse-forwarding split out to Tier 1
15. **Configurable keymap** (M)
16. **Pane splitting** (XL)
17. **Detach/reattach via Architecture A — local daemon** (XL)

---

## Out of scope (for now)

- Windows-native build (current target: macOS/Linux)
- GUI launcher / system-tray helper
- Plugin / scripting runtime (revisit after triggers prove out the use case)
- Built-in Kerberos / smart-card auth (agent + key cover ~all our users)
- Telemetry / phone-home
