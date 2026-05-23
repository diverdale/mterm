# mterm Command Palette & Help — Design

**Date:** 2026-05-22
**Status:** Approved (proceed-and-build); shipped — note: the palette key is `Ctrl-B :` in shipping code (the originally-specced `Ctrl-B p` conflicted with the existing prev-tab binding).

## Summary

Add a fuzzy-search **command palette** (`Ctrl-B p`) and a **keybinding help
overlay** (`Ctrl-B ?`) to mterm, and use the palette as the access surface for
a runtime **theme switcher** (shipping Midnight, Matrix, and Synthwave) and a
**config reload** action.

## Goals

- One discoverable surface (the palette) for power actions, instead of
  inventing a new keybinding per feature.
- Ship the user's wanted **Matrix** theme (and **Synthwave**), built on the
  existing `Theme` abstraction.
- Surface every keybinding via `Ctrl-B ?` so users don't have to memorize.
- Keep behavior backwards-compatible — no existing key changes.

## Non-Goals

- Auto-reconnect *integration* into the TUI (banner, etc.) — separate cycle.
- Scrollback / copy mode — separate cycle.
- Wiring forwards-panel toggles to actually start/stop forwards — separate
  cycle.
- A persistent side panel — different direction; not in scope here.

## Decisions

| Decision | Choice |
|----------|--------|
| Invocation | `Ctrl-B p` opens the palette; `Ctrl-B ?` opens the help overlay |
| Layout | Centered popup, ANSI rounded border in `Accent`, ~60% of terminal width |
| Filter | Case-insensitive substring over command labels |
| Command commit | Single Enter — no two-step chained input; data is pre-expanded into one command per choice |
| Command categories (v1) | Navigation, Direct host connect, Theme switcher, Reload config |
| Themes shipped | Midnight (existing), Matrix (green), Synthwave (magenta) |

## Architecture

Two new view modes added to `App.viewMode`: `modePalette` and `modeHelp`.
`Ctrl-B p` and `Ctrl-B ?` enter them; `Esc` returns to the previous mode.

The palette is a fresh list-driven model (similar in spirit to the picker, but
operating over `command` values instead of `config.Host`). The help overlay is
a static rendering of a keybinding table.

Both popups overlay the underlying view via `lipgloss.Place` (centered) and use
the rounded modal border styled by the active theme — consistent with the
forwards panel.

The theme switcher reassigns the package-level `active` and `sty` variables.
View code already reads `sty` each render, so the new colors apply on the next
render tick — no view-side changes required.

## File Structure

```
internal/tui/
  themes.go       NEW   — Matrix and Synthwave Theme values; setTheme(Theme)
  commands.go     NEW   — command type + buildCommands(*App) []command
  palette.go      NEW   — paletteModel (Update/View)
  help.go         NEW   — helpModel (static View)
  app.go          MODIFY — modes, prefix-key dispatch, palette/help routing
  keys.go         unchanged — no new key encoding needed
```

Each new file has one job. `commands.go` is the only place app state is
inspected to produce palette entries; `palette.go` knows nothing about what
commands *do*, only how to list/filter/run them.

## Theme Abstraction Changes

`theme.go` already defines `Theme` and `buildStyles`. Add:

- `themes.go` with `Matrix` and `Synthwave` Theme values:
  - **Matrix:** Accent `#00FF41`, Frame `#1A1A1A`, Dim `#3A3A3A`,
    Text `#00CC33`, Warning `#FFB347`, Error `#FF4747`, Connected `#00FF41`.
  - **Synthwave:** Accent `#F92AAD`, Frame `#3A1A40`, Dim `#7A4A7A`,
    Text `#E0E0E0`, Warning `#FFD93D`, Error `#FF5C7A`, Connected `#36F1CD`.
- A package-level `setTheme(t Theme)` function that assigns `active = t` and
  `sty = buildStyles(active)`. Called by the theme commands; the render tick
  picks up the new styles on its next pass.
- A `Themes()` slice (or map) returning `[]Theme{Midnight, Matrix, Synthwave}`
  for the palette to enumerate.

Values may be tuned during implementation; the names and structure are fixed.

## Command Palette

### Command type

```go
// command is one selectable entry in the palette.
type command struct {
    label  string                  // shown in the list; fuzzed against query
    group  string                  // section header label
    action func(a *App) tea.Cmd    // executed when the user presses Enter
}
```

Each command pre-binds its data (e.g. one command per host, one per theme) so
the palette never needs a two-step input.

### Commands shipped in v1

`buildCommands(a *App) []command` returns, in order:

**Navigation** (group `"Navigation"`):
- `Open picker` — sets mode to picker.
- `Open forwards panel` — sets mode to forwards (no-op if no active tab).
- `Next tab` / `Previous tab` — `cycleTab(±1)`.
- `Close current tab` — `closeActiveTab()`.
- `Quit mterm` — `shutdown()`.
- `Switch: <N> <host>` — one entry per open tab; sets `a.active = idx`.

**Direct host connect** (group `"Connect"`):
- `Connect: <host>` — one entry per host returned by `config.Load()` (using
  `a.picker.all`). Action: `a.openTab(host)`. Always opens a new tab — even if
  the host already has one (matching today's `openTab` semantics).

**Theme** (group `"Theme"`):
- `Theme: midnight`, `Theme: matrix`, `Theme: synthwave` — one entry per
  `Themes()` value. Action: `setTheme(t)`.

**Config** (group `"Config"`):
- `Reload config` — re-runs `config.Load()` and replaces `a.picker.all`. Open
  tabs are unaffected (their sessions live independently of the config file).
  Warnings from `Load()` go to `a.statusMsg`.

### Palette behavior

- Opened with `Ctrl-B p` from any session/picker mode. The palette captures
  the previous mode and returns to it on dismiss.
- Type to filter (case-insensitive substring on `label`).
- `↑/↓` and `Ctrl-J/K` move the cursor; cursor wraps; selection resets when
  query changes.
- `Enter` runs the focused command's `action`, then dismisses the palette.
- `Esc` dismisses without running.
- The palette stays themed with the active theme — switching themes inside the
  palette is allowed and visible on the next render.

### Palette view

```
╭─ Command Palette ─────────────────────────────╮
│ > theme_                                      │
│                                               │
│   Theme                                       │
│   Theme: midnight                             │
│ > Theme: matrix                               │
│   Theme: synthwave                            │
│                                               │
│   enter: run   esc: cancel                    │
╰───────────────────────────────────────────────╯
```

- Centered overlay via `lipgloss.Place`.
- ~60% of terminal width, height fits content up to ~60% of terminal height
  (then the visible list scrolls; the cursor stays in view).
- Group headers rendered in `sty.groupHeader` (dim+bold). Cursor row uses the
  same full-width selection bar as the picker. Border in `Accent`.

## Help Overlay

`Ctrl-B ?` opens a centered modal listing every keybinding, grouped by mode.
`Esc` (or any key) dismisses. Content is static — no input, no filtering. The
table is composed once in `help.go`'s `View()`.

```
╭─ mterm — keybindings ─────────────────────────╮
│  Prefix                                       │
│    ^B c          open picker / new connection │
│    ^B n / ^B p   next / previous tab          │
│    ^B 1..9       jump to tab N                │
│    ^B x          close current tab            │
│    ^B f          open forwards panel          │
│    ^B p          open command palette         │
│    ^B ?          this help                    │
│    ^B q          quit mterm                   │
│                                               │
│  Picker                                       │
│    type          filter hosts                 │
│    ↑/↓           move cursor                  │
│    enter         connect                      │
│    esc           back to active session       │
│                                               │
│  Forwards panel                               │
│    space/enter   toggle                       │
│    esc           close                        │
│                                               │
│  Palette                                      │
│    type          filter commands              │
│    ↑/↓           move cursor                  │
│    enter         run                          │
│    esc           cancel                       │
│                                               │
│  Global                                       │
│    ^C            quit mterm                   │
│                                               │
│           esc to dismiss                      │
╰───────────────────────────────────────────────╯
```

## App Integration

`App` gains `palette *paletteModel`, `help *helpModel`, and a `prevMode
viewMode` field (the mode to return to when a popup dismisses).

`handleCommandKey`:
- `p` — `a.prevMode = a.mode; a.palette = newPaletteModel(buildCommands(a)); a.mode = modePalette`.
- `?` — `a.prevMode = a.mode; a.help = newHelpModel(); a.mode = modeHelp`.

`update` routes `tea.KeyMsg` to `palette.Update` when `mode == modePalette` and
`help.Update` when `mode == modeHelp`. New message types `paletteClosedMsg`
and `helpClosedMsg` (emitted on Esc / Enter-run) return the app to
`a.prevMode`.

`View` adds two cases — both render the underlying view first, then overlay
the modal via `lipgloss.Place`.

## Reload Config

The `Reload config` command calls a small helper:

```go
func (a *App) reloadHosts() {
    res, err := config.Load()
    if err != nil {
        a.statusMsg = fmt.Sprintf("reload: %v", err)
        return
    }
    a.picker = newPicker(res.Hosts)
    for _, w := range res.Warnings {
        a.statusMsg = w // last warning wins
    }
}
```

Open tabs continue to run on their existing SSH connections — the config
reload only affects what the picker shows and the next time
`buildCommands` runs (i.e. next palette open).

## Testing

- `themes.go`: trivial — Matrix/Synthwave are pure data; `setTheme(Matrix)`
  causes `sty.title.GetForeground()` to equal `Matrix.Accent`.
- `commands.go`: `buildCommands` over a fake App produces the expected
  command set — at least one entry per category; `Connect:` count equals the
  host count; `Theme:` count equals `len(Themes())`; `Switch:` count equals
  the open-tab count.
- `palette.go`: filter is case-insensitive substring; cursor wraps; Enter
  invokes the focused command's action; Esc emits `paletteClosedMsg`.
- `help.go`: `View()` is non-empty, contains every prefix key letter once.
- `app.go` integration: `prefix p` sets `modePalette` and builds the palette;
  `prefix ?` sets `modeHelp`; `paletteClosedMsg` and `helpClosedMsg` restore
  `prevMode`; running a "Theme: matrix" command changes the theme — verified
  by checking `active.Name == "matrix"`.
- Full suite green under `-race`; `go vet`, `gofmt -l`, `go mod verify` clean.

## Future Work

- Auto-reconnect *integration* (banner in tabs, palette command `Reconnect
  current tab`).
- Scrollback / copy mode (`Ctrl-B [`).
- Live port-forward toggles (the forwards panel wired to `Session.StartForward`).
- Recent connections / connection history in the palette.
- A `:` command-line mode for actions with arguments (currently unnecessary —
  every command is pre-expanded).
