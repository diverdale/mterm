# mterm keybinding reference

The prefix is **`^B`** (Ctrl-B), tmux-style. Press it, then the command key.
Tabs that don't need a prefix are noted explicitly.

## Prefix chords

| Key       | Action                                              |
|-----------|-----------------------------------------------------|
| `^B c`    | open the picker (new connection)                    |
| `^B n`    | next tab                                            |
| `^B p`    | previous tab                                        |
| `^B 1..9` | jump to tab N                                       |
| `^B x`    | close the current tab                               |
| `^B f`    | open the port-forwards panel                        |
| `^B s`    | toggle the current tab in the broadcast sync set    |
| `^B m`    | toggle minimal frame ↔ previous style (clipboard-friendly chrome) |
| `^B [`    | enter keyboard copy mode (select scrollback → system clipboard) |
| `^B u`    | open the two-pane SFTP file browser for the active tab |
| `^B :`    | open the command palette                            |
| `^B ?`    | open the keybinding help overlay                    |
| `^B D`    | dump every goroutine's stack to `/tmp/mterm-stacks-…` for diagnosis (status line shows the file path) |
| `^B q`    | quit mterm                                          |

## Session-mode keys (no prefix)

| Key             | Action |
|-----------------|--------|
| `^←` / `^→`     | previous / next tab. Shadows shell word-jump on the remote — use Option-Left/Right on macOS for word-jump |
| `^PgUp` / `^PgDn` | previous / next tab; no shell conflict |
| `^C`            | passes through to the remote session (SIGINT) |
| wheel ↑/↓       | scroll the active tab's scrollback; any key snaps back to live |

## Picker

| Key      | Action                |
|----------|-----------------------|
| (type)   | fuzzy-filter hosts    |
| ↑ / ↓    | move cursor           |
| Enter    | connect               |
| Esc      | back to active session|

## Command palette

| Key      | Action                |
|----------|-----------------------|
| (type)   | filter commands       |
| ↑ / ↓    | move cursor           |
| Enter    | run                   |
| Esc      | cancel                |

## File browser (`^B u`)

| Key            | Action |
|----------------|--------|
| `Tab`          | switch active pane (left ↔ right) |
| `↑` / `↓` or `j` / `k` | move cursor in the active pane |
| `Enter` / `→` / `l`    | descend into the highlighted directory; `..` goes up |
| `Backspace` / `←` / `h` | go up to the parent directory |
| `F5` or `c`    | copy highlighted file from active pane → other pane (with live progress) |
| `Esc`          | cancel an in-flight transfer; otherwise close the browser |
| `r`            | refresh both panes |
| `.`            | toggle hidden files (filenames starting with `.`) |

## Copy mode (`^B [`)

| Key                | Action |
|--------------------|--------|
| `↑` / `k`          | cursor up one line |
| `↓` / `j`          | cursor down one line |
| `PgUp` / `^B`      | page up |
| `PgDn` / `^F`      | page down |
| `g`                | jump to oldest line |
| `G`                | jump to bottom (most recent line) |
| `v` or `Space`     | toggle the selection anchor at the cursor |
| `Enter` or `y`     | copy the selection to the clipboard and exit |
| `q` or `Esc`       | exit without copying |
