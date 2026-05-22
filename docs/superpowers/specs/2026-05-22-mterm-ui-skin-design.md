# mterm UI Skin — Design

**Date:** 2026-05-22
**Status:** Approved design

## Summary

A visual overhaul of mterm's terminal UI. mterm currently works but looks
spartan: no borders, a one-line tab bar, no footer, ad-hoc colors. This skin
wraps each view in a real window frame, adds a context-aware footer, a polished
tab strip with animated connection-status indicators, and a cohesive color
theme built on a swappable `Theme` abstraction.

This is a **pure visual skin**. Every keybinding, navigation path, and
interaction stays exactly as it is. The embedded SSH session content
(`terminal.Render()` output) passes through untouched — only mterm's own chrome
is restyled.

## Goals

- Make mterm look like a designed TUI: framed windows, a status footer, themed
  chrome.
- Always show which host is focused and its connection state.
- Establish a `Theme` abstraction so alternate themes can be added later.

## Non-Goals

- No layout or interaction changes — same picker, same tabs, same `Ctrl-B`
  prefix keys.
- No recoloring of remote session content.
- No theme *switcher* and no themes beyond Midnight in this iteration (see
  Future Work).
- No mouse support.

## Decisions

| Decision | Choice |
|----------|--------|
| Scope | Pure visual skin over existing layouts/keybindings |
| Windowing | Full window frame around each view |
| Footer | Keybind hints + PREFIX badge + connection detail + clock |
| Status indicator | Animated braille spinner for in-progress states |
| Tab style | Styled chips (standard glyphs, no powerline font needed) |
| Theme | Midnight (cyan accent), built on a swappable `Theme` type |

## Implementation Approach

Compose all chrome with **Lip Gloss** layout primitives — `Border`,
`JoinVertical`, `JoinHorizontal`, `Place`, and width/height styling. Lip Gloss
measures ANSI-styled content display-width-correctly, which matters because the
live session body is full of color codes. Lip Gloss is already a dependency.

Hand-rolled box-drawing with manual width math was rejected: it is far more
error-prone once ANSI-colored content sits inside a bordered box.

## Theme Abstraction

A `Theme` is a plain value type holding the palette. All chrome styles are
derived from the *active* theme; no color literals are scattered through the
view code. This replaces today's `styles.go` (which hardcodes values like
`lipgloss.Color("63")`).

```go
// Theme is a named chrome color palette.
type Theme struct {
	Name      string
	Accent    lipgloss.Color // active tab, host name, focus highlights
	Frame     lipgloss.Color // window borders, dividers
	Dim       lipgloss.Color // inactive tabs, secondary text, group headers
	Text      lipgloss.Color // primary foreground
	Warning   lipgloss.Color // amber — warnings
	Error     lipgloss.Color // red — connection/auth errors
	Connected lipgloss.Color // green — connected status dot
}
```

### Midnight (v1 default)

```go
var Midnight = Theme{
	Name:      "midnight",
	Accent:    lipgloss.Color("#5ED4E0"), // bright cyan
	Frame:     lipgloss.Color("#5C6370"), // slate gray-blue
	Dim:       lipgloss.Color("#6B7280"), // muted gray
	Text:      lipgloss.Color("#D8DEE9"), // light fog
	Warning:   lipgloss.Color("#E5C07B"), // amber
	Error:     lipgloss.Color("#E06C75"), // red
	Connected: lipgloss.Color("#98C379"), // green
}
```

A package-level `active` theme holds the current `Theme` (initialized to
`Midnight`). Because v1 has no runtime theme switching, the chrome
`lipgloss.Style` set is built **once** from `active` — a `buildStyles(Theme)`
function called at package init produces the styles the views use. When a
switcher is added later, it re-runs `buildStyles` with the new theme. Exact hex
values above may be tuned during implementation; they are the starting palette,
not placeholders.

The session content is never recolored: the remote's own ANSI passes straight
through `terminal.Render()`.

## Session Window

The session view is one Lip Gloss bordered box, composed top-to-bottom:

```
┌─ ⠹ prod-web ─────────────────────[2 tabs]─┐   top border: status glyph + active host + tab count
│  ● 1 prod-web    ⠹ 2 prod-db               │   tab strip (1 row)
│ $ tail -f app.log                          │   terminal body
│ 200 GET /api/users                         │
│ █                                          │
├────────────────────────────────────────────┤   divider
│ ^B menu  ^B n next  ^B x close   dale@…:22 ⏱│   footer (1 row)
└────────────────────────────────────────────┘
```

### Frame

- A bordered box spanning the full terminal width and height, drawn in the
  theme `Frame` color.
- **Top border** carries a title: the active tab's status glyph, the active
  host name (in `Accent`), and an `[N tabs]` count. Rendered into the border
  line.
- **Bottom region**, inside the box: a divider line, then the footer row.

### Tab strip

The first row inside the box. Each tab is a chip:

```
<status glyph> <index> <host name>
```

- The **active** chip has a filled background in `Accent` with bold text.
- **Inactive** chips render in `Dim`.
- Each chip's status glyph reflects that tab's own connection state.
- Chips are space-separated and joined with `lipgloss.JoinHorizontal`.

### Status indicator

A tab's connection status is *derived* — no new persisted state:

| Condition | Status | Glyph |
|-----------|--------|-------|
| `sess` is nil and the tab has not ended | connecting | animated braille spinner |
| `sess` is set | connected | `●` in `Connected` (green) |
| connection failed | failed | `✖` in `Error` (red) |

A `sessionTab.status()` method returns this derived state. (Failed is mostly
transient — a failed connect removes the tab — but the mapping is defined for
completeness and future reconnect work.)

The **spinner** cycles the braille frames
`⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏`. The root `App` carries a tick counter incremented on
every render tick (~33 ms). The current frame is
`frames[(counter / 3) % 10]`, so the spinner advances roughly every 100 ms
instead of at 30 fps. The same counter drives any tab showing a spinner.

### Body sizing — reserved space

The embedded `terminal.Terminal` and the remote PTY must be sized to the box's
**inner content area** so output fills the frame exactly with no overflow or
wrapping.

The chrome reserves:
- **2 columns** — left and right borders.
- **5 rows** — top border, tab strip, divider, footer, bottom border.

So a session terminal is sized `width-2 × height-5` (each clamped to a minimum
of 1). This replaces today's single reserved tab-bar row. The reserved-rows
constant grows from 1 to 5; `sessionTab` sizing and resize logic use the new
figure. Tests that assert the old `height-1` body height are updated to
`height-5`.

## Footer

One row inside the bottom border, built by a context-aware footer component.
Layout: hints left-aligned, status info right-aligned, composed with
`lipgloss.JoinHorizontal` / `Place` to fill the width.

### Session mode

- **Left:** keybind hints — `^B menu · ^B n next · ^B x close` (dim text,
  keys in `Accent`).
- **Right:** `user@host:port` of the active tab, then a live clock (`HH:MM`).
- **Prefix pending:** when the user has pressed `Ctrl-B` and mterm is waiting
  for a command key, a highlighted `PREFIX` badge (bg `Accent`) takes the left
  slot, replacing the hints, so it is unmissable.

### Picker mode

- **Left:** `enter connect · type to filter · esc back`.
- **Right:** host count (`N hosts`), then the clock.
- No connection detail (no active session).

### Graceful degradation

When the terminal is too narrow to fit everything, drop elements right-to-left:
connection detail first, then the clock. The keybind hints (or the `PREFIX`
badge) always survive.

The clock is live because the existing 30 fps render tick re-renders the view;
the footer reads `time.Now()` at render time. No extra timer.

## Picker

The picker gets the same window frame:

- **Title:** `mterm` with the host count.
- **Body:** the host list.
- **Footer:** picker-mode hints + host count + clock.

Inside the body:
- The cursor row is a **full-width selection bar** — background in `Accent`,
  text in a contrasting color.
- **Group headers** render in `Dim` with a thin horizontal rule.
- **Tags** render in a subtle `Dim` color.
- The **search line** is themed with a prompt glyph and `Accent` query text.
- **Connect errors** render in the `Error` color in the footer/status area
  (the existing `statusMsg` mechanism, themed).

## Forwards Modal

The forwards panel keeps its centered-modal placement (`lipgloss.Place`) and is
restyled to the theme: an `Accent` border, a title row, themed `[x]` / `[ ]`
toggle marks (`[x]` in `Connected`), and footer hints. Content and behavior
unchanged.

## What Stays the Same

- Every keybinding and the `Ctrl-B` prefix state machine.
- All navigation: picker filtering, tab open/switch/close, forwards panel.
- The async connect flow, tab reaping, and all session/SSH behavior.
- The session content itself — `terminal.Render()` output is rendered as-is.

## Testing

- `theme.go` is pure data — a trivial sanity test that `Midnight` is the active
  default.
- The frame, tab strip, and footer renderers are **pure functions** (inputs →
  string). Tests assert:
  - the composed view has the expected line count and equals the terminal
    width;
  - the body region is sized to `width-2 × height-5`;
  - the spinner glyph advances as the tick counter increases and cycles
    correctly;
  - the footer shows session-mode hints vs picker-mode hints appropriately,
    shows the `PREFIX` badge when prefix is pending, and drops elements on a
    narrow width;
  - tab chips show the right status glyph for connecting vs connected tabs.
- Existing `tui` tests that assert view structure or body-height math are
  updated for the new layout (notably the `height-1` → `height-5` change and
  any `View()` content assertions).
- Full suite must pass under `-race`; `go vet` and `gofmt` clean.

## Future Work

- A theme **switcher** (config key and/or a keybind) and additional themes —
  **Matrix** (green accent) is the next one wanted, plus Synthwave.
- Mouse support.
