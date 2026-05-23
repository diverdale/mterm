# Per-Host Visual Identity — Design

**Status:** approved 2026-05-23
**Roadmap item:** Tier 2 — per-host visual identity (item #1 in suggested sequence)

> **Post-merge note (2026-05-23):** the yaml field is `bordercolor` (lowercase), not `borderColor`. All mterm yaml tags switched to lowercase + strict KnownFields decoding shortly after merge; see `fix(config): lowercase yaml tags + strict unknown-key warnings`.

## Goal

Let users assign a frame color to a host so that prod, staging, and dev look
visually distinct when their tab is active. Single-purpose safety feature: if
the red border is on screen, you're typing into prod.

## Architecture

One optional field on the mterm overlay (`borderColor` in `hosts.yaml`). When a
session tab is active and its host has a non-empty `BorderColor`, the chrome
rendering uses a per-host derived style set instead of the global one. The
session body, inactive tabs, picker, palette, and help overlays are unaffected.

## What gets colored

When the active tab's host has a `BorderColor`:

- Window frame: top border (incl. title bar fill), side bars, dividers (tab
  divider and footer divider), bottom border
- Active tab chip: bracket characters and the label text
- Footer's `user@host:port` segment (currently `sty.footerInfo`)

What does *not* get re-tinted:

- Inactive tab chips (stay dim)
- The status glyph on every tab chip (keeps its own connected/failed/connecting
  semantic color)
- Footer hint labels (e.g. `^B c · new`), footer hint keys, PREFIX badge, clock
- Picker, palette, help, forwards modal overlays (host-agnostic)
- The session body — remote terminal output is never modified

## Config schema

Existing `mtermHost` struct in `internal/config/mterm.go` gets one new field:

```go
type mtermHost struct {
    Name        string         `yaml:"name"`
    HostName    string         `yaml:"hostName"`
    User        string         `yaml:"user"`
    Port        int            `yaml:"port"`
    Group       string         `yaml:"group"`
    Tags        []string       `yaml:"tags"`
    Forwards    []mtermForward `yaml:"forwards"`
    BorderColor string         `yaml:"borderColor"` // NEW
}
```

Existing `Host` struct in `internal/config/types.go`:

```go
type Host struct {
    // ... existing fields ...
    BorderColor string // NEW; "" means use theme default
}
```

`applyOverlay` and `hostFromMterm` both copy `BorderColor` through. Non-empty
overlay value wins (consistent with every other overlay field).

### Color format and validation

Accepted at config load time:

- 7-char hex `#RRGGBB`
- 4-char shorthand hex `#RGB` (e.g. `#F33`)

Rejected (warned + ignored, host stays uncolored):

- Anything else: named colors (`"red"`), ANSI 256 indices (`"196"`), missing `#`,
  wrong length, non-hex characters, etc.

Hex-only for v1 keeps validation trivial and the config self-documenting. Named
colors can come later if anyone asks.

Validation lives in a new helper `parseBorderColor(s string) (string, error)`
called from `loadAndMerge` before the value reaches `Host`. On parse failure,
the host's `BorderColor` is left empty and `Result.Warnings` gets one entry per
invalid host:

```
mterm config: host "prod-db-01" borderColor "red": want #RRGGBB or #RGB
```

## Rendering: deriving the per-host chrome

A new package-level helper in `internal/tui/theme.go`:

```go
// chromeStylesFor returns a styleSet identical to the active set except that
// the four host-tinted styles use c instead of the theme's Frame/Accent.
// When c is the empty string, returns the global sty unchanged.
func chromeStylesFor(c string) styleSet
```

Overrides (vs `buildStyles`):

| field        | normal source     | per-host source            |
|--------------|-------------------|----------------------------|
| `border`     | `t.Frame`         | host color                 |
| `title`      | `t.Accent` (bold) | host color (bold)          |
| `tabActive`  | `t.Accent` (bold) | host color (bold)          |
| `footerInfo` | `t.Dim`           | host color                 |

Every other field of the styleSet is copied from the active global set.

### Wiring it in

Today the frame, tab strip, and footer render with the package-level `sty`.
Move the session-mode render path to read styles from a local variable
`s := sty` (or `s := chromeStylesFor(host.BorderColor)` when present), and have
the helpers accept it:

- `renderWindow(o windowOpts)` → `renderWindow(o windowOpts, s styleSet)`
- `renderTabStrip(...)` → take `s styleSet`
- `topBorder`, `bottomBorder`, `divider`, `sideRow` → take `s styleSet`
- footer rendering → take `s styleSet`

Other call sites (picker view, palette modal, help modal, forwards modal) pass
the global `sty` unchanged. Only session-mode is host-aware.

## Out of scope

- Per-host *theme* switching (`theme: synthwave`). Repainting the entire UI on
  tab switch is jarring; the safety win is in the border alone.
- Named colors, ANSI 256 indices. Hex only.
- Inactive-tab tinting. Inactive chips stay dim regardless of host color so the
  user can tell at a glance which host owns the screen.
- Body tinting / per-host cursor color.
- Color suggestions / presets in any picker UI.

## Testing approach

Unit-level tests, no end-to-end:

1. **Config parsing** — `internal/config`:
   - `borderColor: "#FF3344"` round-trips to `Host.BorderColor == "#FF3344"`.
   - `borderColor: "#F33"` round-trips to `Host.BorderColor == "#F33"`.
   - `borderColor: "red"` → empty string and a warning containing the host name.
   - `borderColor: "#GGGGGG"` → empty string and a warning.
   - `borderColor: ""` → empty string, no warning.
   - Overlay merges: ssh_config host gains `BorderColor` from hosts.yaml; pure
     hosts.yaml host gets it directly.

2. **Style derivation** — `internal/tui`:
   - `chromeStylesFor("")` returns the same border foreground as `sty.border`.
   - `chromeStylesFor("#FF3344")` returns a `border` whose `GetForeground()` is
     `lipgloss.Color("#FF3344")`. Same check for `title`, `tabActive`,
     `footerInfo`.

3. **Render integration**:
   - All render helpers (`renderWindow`, `renderTabStrip`, `renderFooter`,
     `topBorder`/`bottomBorder`/`divider`/`sideRow`) are pure functions of
     their styleSet argument. Their existing tests pass `sty` explicitly and
     still pass after the signature change.
   - One new integration test on `App.View()`: build an app with a session tab
     whose host has `BorderColor: "#FF3344"`, render the session view, and
     assert the output contains the RGB ANSI escape derived from `#FF3344`
     (force TrueColor profile via `lipgloss.SetColorProfile(termenv.TrueColor)`
     in the test). Then change the active tab's host to one without a
     BorderColor and assert that escape is absent.
   - Picker, palette, and help views are tested separately and pass `sty`
     directly — they remain host-agnostic by construction.

No new test files unless a clean home doesn't already exist; extend
`config_test.go`, `theme_test.go`, `frame_test.go`, and either `tabstrip_test.go`
or `session_test.go`.

## Acceptance

- Adding `borderColor: "#FF3344"` to a host in `hosts.yaml` and connecting to
  it shows a red frame, red active-tab brackets/label, and a red `user@host:port`
  in the footer.
- Switching to a tab whose host has no `borderColor` returns the chrome to the
  theme default within one render.
- A malformed `borderColor` produces a footer warning on launch (via the
  existing `statusMsg` warning surfacing) and the host renders normally.
- `go test ./... -race` passes; `go vet`, `gofmt -l` clean.
