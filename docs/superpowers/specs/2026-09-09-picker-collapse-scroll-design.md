# Picker Collapsible Groups & Viewport Scrolling — Design

**Date:** 2026-09-09  
**Status:** Implemented (2026-09-10)  
**Roadmap item:** Tier 3 — Connection groups in picker (`docs/superpowers/roadmap.md`)

## Summary

Refactor the host picker from a flat, unbounded list into a **navigable tree**
with collapsible group headers and a **viewport window** that keeps the cursor
visible. Fixes two related problems: (1) long host lists overflow the terminal
with silent truncation today, and (2) group headers look collapsible (`▸`) but
are purely decorative.

Ship as one feature. Collapse reduces row count; scrolling handles what
remains. Fuzzy search behavior is preserved and gains explicit rules for
collapse interaction.

## Problem

Today the picker:

1. Builds a flat `visibleHosts()` slice and injects group headers at render time.
2. Renders every row into a string, then `renderWindow` → `fitLines` **truncates**
   the body to the available height with **no indicator** that content exists
   below the fold.
3. Moves the cursor over the full flat list — the selected host can be off-screen.
4. Uses `▸` on group headers, implying expand/collapse, but headers are not
   interactive.

The command palette already solved viewport scrolling (`paletteWindow` in
`palette.go` with `▴ N more above / ▾ N more below`). The picker should adopt
the same pattern over a row-based model.

## Goals

- Collapse/expand any group prefix (`production`, `production/db`, …).
- Viewport scrolling: cursor always visible; truncation indicators when clipped.
- Preserve existing fuzzy filter (name, hostname, group, tags).
- Preserve nested group rendering (slash-delimited paths, indentation, counts).
- Backward-compatible defaults: all groups expanded on first open.
- No new config files in v1 (session-local collapse state only).

## Non-Goals (v1)

- Persisting collapse state across restarts (`settings.yaml` / `collapsed.json`).
- `default_collapsed` / `default_collapsed: top_level` settings.
- Mouse click on headers (mouse is ignored in picker mode today).
- Mouse wheel scroll in picker mode.
- Palette commands "Collapse all" / "Expand all".
- Changing how hosts are loaded, sorted, or merged from config sources.
- Virtualized rendering for thousands of hosts (viewport window is sufficient).

## Decisions

| Decision | Choice |
|----------|--------|
| Row model | Explicit `pickerRow` list (header rows + host rows) built before render |
| Collapse key | Full group path, e.g. `"production/db"` (same as `config.GroupSegments` joined by `/`) |
| Default state | All groups expanded |
| Collapse persistence (v1) | In-memory for the picker session; reset when picker is re-created (`reloadHosts`, app restart) |
| Header glyph collapsed | `▸` (unchanged) |
| Header glyph expanded | `▾` |
| Enter on header row | Toggle expand/collapse |
| Enter on host row | Connect (unchanged) |
| `Space` on header row | Toggle expand/collapse |
| `←` / `→` on header row | Collapse / expand respectively |
| `←` / `→` on host row | No-op |
| Search active (`query != ""`) | Auto-expand all groups containing matches; collapse state restored when query cleared |
| Scrolling | Viewport window over row list; reuse palette's windowing approach |
| Wheel in picker | Out of scope v1 (session mode keeps wheel) |
| Ungrouped hosts | Virtual group `"ungrouped"` (current behavior); header is toggleable like any other |
| Empty collapsed group | Header still visible; count shows total hosts in subtree |

## Architecture

### Row model

Replace flat `cursor` over `visibleHosts()` with `cursor` over `rows []pickerRow`.

```go
type pickerRowKind int

const (
    pickerRowGroup pickerRowKind = iota
    pickerRowHost
)

type pickerRow struct {
    kind  pickerRowKind
    path  string       // group path for headers; "" for hosts
    label string       // display segment for headers (uppercased at render)
    depth int          // 0-based; drives indent
    count int          // total hosts in subtree (headers only)
    host  config.Host  // valid when kind == pickerRowHost
}
```

`buildRows(filtered []config.Host, collapsed map[string]bool) []pickerRow`:

1. Walk filtered hosts in config sort order (same order as today's flat list).
2. For each host, compute `segs := config.GroupSegments(h.Group)` (or
   `["ungrouped"]`).
3. Emit header rows for path segments not already "open" from the previous host
   (same `CommonPrefixLen` logic as today), **but skip** segments whose prefix
   path is in `collapsed`.
4. Emit a host row only if no prefix of its path is collapsed.
5. Header `count` = number of hosts in `filtered` whose group path has the
   header's path as a prefix (matches today's `prefixCount` semantics).

Collapsed check: path `P` is collapsed if `collapsed[P] == true`. A host under
`production/db/replica` is hidden when `collapsed["production"]` OR
`collapsed["production/db"]` OR `collapsed["production/db/replica"]` is set.

### Picker state

```go
type picker struct {
    all       []config.Host
    query     string
    cursor    int              // index into current rows slice
    collapsed map[string]bool  // path → collapsed; absent = expanded
    savedCollapsed map[string]bool // snapshot while query non-empty; nil when query empty
    w, h      int
    deco      pickerDecorations
}
```

`visibleHosts()` remains for filtering logic. New `visibleRows()` calls
`buildRows(visibleHosts(), effectiveCollapsed())`.

`effectiveCollapsed()`:

- `query == ""` → return `collapsed`
- `query != ""` → return empty map (all expanded for display)

When `setQuery` transitions from non-empty → empty, restore `collapsed` from
`savedCollapsed` (captured when query first became non-empty). When
non-empty → non-empty, leave `savedCollapsed` unchanged.

### Viewport window

Extract or generalize `paletteWindow` into a shared helper (e.g.
`listWindow(total, cursor, maxRows) (start, end, more)` in a small
`viewport.go` or at the top of `picker.go`). Palette continues to call it
with its chrome budget; picker uses its own.

`picker.View()`:

1. `rows := p.visibleRows()`
2. `maxRows := p.bodyRows()` — rows available inside the window body after
   subtracting search line, blank lines, and truncation indicator lines.
3. `start, end, more := listWindow(len(rows), p.cursor, maxRows)`
4. Render `▴ N more above` / `▾ N more below` when `more.above/below > 0`
5. Render only `rows[start:end]`

`bodyRows()` budget (picker chrome inside the framed body string):

| Rows | Content |
|------|---------|
| 1 | `Search: <query>` |
| 1 | blank |
| 0–1 | `▴ N more above` (when truncated) |
| *N* | visible row window |
| 0–1 | `▾ N more below` (when truncated) |

`p.h` is set by `pickerView()` from terminal height minus frame chrome
(today: `a.height - 4` for width; height passed as `a.height` or a dedicated
body budget — implementation should pass **terminal height** so the picker can
compute `bodyRows` accurately, matching how `paletteModel.termH` works).

**Important:** Stop relying on `fitLines` truncation as the scrolling
mechanism. The picker body string should be ≤ `bodyRows` lines so `fitLines`
becomes a safety net, not the primary clip.

### Cursor movement

`moveCursor(delta)` operates on `visibleRows()` length, not `visibleHosts()`.
Wrap behavior unchanged (modulo row count).

`selected()` returns `(config.Host, true)` only when `rows[cursor].kind ==
pickerRowHost`.

### Toggle collapse

`toggleCollapsed(path string)`:

- If `collapsed[path]`, delete the key (expand).
- Else set `collapsed[path] = true`.
- After toggle, clamp `cursor` to `len(visibleRows())-1` if needed.
- If cursor landed on a row that disappeared, move to nearest visible row
  (prefer same index, else index-1).

`expand(path)` / `collapse(path)` for arrow keys.

## User Experience

### Picker view (expanded)

```
╭─ mterm ───────────────────────────────────────────────────────────────╮
│ Search: prod                                                          │
│                                                                       │
│   ▾ PRODUCTION (12) ─────────────────────────────────────────────     │
│       ▾ DB (4) ─────────────────────────────────────────────────      │
│         > ◉ prod-db       10.0.2.9     2h ago                         │
│           ○ prod-db-2     10.0.2.10    —                              │
│       ▸ WEB (8) ────────────────────────────────────────────────      │
│                                                                       │
│   ▾ more below                                                        │
├───────────────────────────────────────────────────────────────────────┤
│ enter: connect · space: toggle · ←/→: fold · type: filter · esc: back │
╰───────────────────────────────────────────────────────────────────────╯
```

### Picker view (collapsed group)

```
│   ▸ PRODUCTION (12) ─────────────────────────────────────────────     │
```

Hosts and child headers under a collapsed prefix are omitted from `rows`.

### Keybindings (picker)

| Key | Header row | Host row |
|-----|------------|----------|
| (type) | filter (unchanged) | filter |
| `↑` / `↓` | move cursor | move cursor |
| `Ctrl-j` / `Ctrl-k` | move cursor | move cursor |
| `Enter` | toggle collapse | connect |
| `Space` | toggle collapse | no-op |
| `←` | collapse | no-op |
| `→` | expand | no-op |
| `Esc` | back to session | back to session |
| `Backspace` | edit query | edit query |

Update footer hints in `pickerView()`:

```
enter: connect · space: toggle · ←/→: fold · type: filter · esc: back
```

Update `docs/keybindings.md` picker section to match.

### Search interaction

When the user types a filter:

1. On first character (empty → non-empty), snapshot `savedCollapsed = clone(collapsed)`.
2. While `query != ""`, `effectiveCollapsed()` returns empty → all matching
   hosts visible regardless of collapse state.
3. Filter semantics unchanged: case-insensitive substring over name, hostname,
   group, tags.
4. When query cleared, restore `collapsed` from `savedCollapsed`.

Rationale: avoids "I searched for `prod-db` but production is collapsed" and
matches file-tree search UX.

### Counts when collapsed

Header count always shows **total hosts in subtree** (from filtered set), not
"visible right now." Example: `▸ PRODUCTION (12)` even when collapsed hides
all 12. Optionally add a follow-up `(0 visible)` suffix — **not in v1**; keep
counts simple.

### Reload config

`reloadHosts()` replaces `a.picker` with `newPicker(res.Hosts)` — collapse
state resets. Acceptable for v1; persistence is a follow-up.

## File Structure

```
internal/tui/
  picker.go       MODIFY — row model, collapse state, viewport render, new keys
  picker_test.go  MODIFY — collapse, scroll window, search override, key dispatch
  viewport.go     NEW (optional) — shared listWindow extracted from palette.go
  palette.go      MODIFY (optional) — call shared listWindow instead of paletteWindow
  app.go          MODIFY — pass terminal height to picker; footer hints
docs/keybindings.md MODIFY — picker key table
```

Prefer extracting `listWindow` only if it keeps `palette.go` and `picker.go`
DRY without awkward coupling. Duplicating ~30 lines in `picker.go` is acceptable
if extraction feels forced.

## Edge Cases

| Case | Behavior |
|------|----------|
| All hosts filtered out | `(no matching hosts)` — no rows, cursor 0 |
| Single host, no group | One `ungrouped` header + one host row |
| Collapse parent | All descendant headers and hosts hidden; child collapse flags preserved in map |
| Expand parent | Children respect their own collapse flags |
| Enter on header | Toggle only; never connects |
| Cursor on last row, collapse removes it | Clamp to `len(rows)-1` |
| Tiny terminal (< 8 rows body) | `listWindow` shows minimum 3 item rows (palette precedent) |
| `visibleRows()` empty after collapse | Show group headers only if any top-level group exists with all collapsed; else empty state message |
| Host row selected, user collapses ancestor | Cursor moves to ancestor header row |

## Testing

### `picker.go` unit tests

- **Row building:** collapsed `production` hides all `production/*` hosts;
  sibling groups unaffected.
- **Nested collapse:** collapse `Home` hides `Home/Media` and `Home/Development`
  subtrees; expand `Home` restores child collapse state.
- **Search override:** with `production` collapsed, query matching a production
  host shows that host; clearing query re-collapses.
- **Counts:** `▸ HOME (3)` with 2 Media + 1 Development hosts (existing test
  adapted for `▾` when expanded).
- **Toggle keys:** Enter/Space on header toggles glyph; Enter on host emits
  `pickerChosenMsg`.
- **Arrow keys:** `←` collapses expanded header; `→` expands collapsed header.
- **Viewport:** with 20 rows and `h=10`, output contains `▾ N more below`;
  cursor row visible in output; moving cursor updates which `more` indicator shows.
- **Cursor wrap:** wraps over row count, not host count.
- **Blank line between top-level groups:** preserved from current behavior.

### Integration (`app_test.go`)

- Open picker, collapse a group via simulated keys, confirm `View()` omits
  hidden hosts.
- Optional: reload config resets collapse (if easily testable).

### Regression

- Existing picker tests updated for `▾` on expanded headers.
- Full `go test ./...` green under `-race`.
- `gofmt`, `go vet` clean.

## Implementation Phases

### Phase 1 (this spec — ship together)

- Row model + collapse toggle + viewport scrolling + search auto-expand
- Footer + keybindings doc update

### Phase 2 (follow-up)

- Persist collapse state to `~/.config/mterm/picker.json` (or `settings.yaml`
  `picker.remember_collapsed: true`)
- `default_collapsed: false | top_level | all` setting
- Palette commands: "Collapse all groups" / "Expand all groups"
- Mouse click on header rows in picker mode
- Mouse wheel scroll in picker mode

## Future Work

- "Recently used" group pinned at top.
- Per-group default collapse in `hosts.yaml` (`collapsed: true` on group — needs schema).
- vim-style `za` / `zM` / `zR` fold commands in copy-mode spirit.
- Fuzzy scoring (substring → fzf-style ranking) — separate from this feature.

## Open Questions (resolve before coding)

1. **Height plumbing:** Pass full `a.height` into picker (like palette's `termH`)
   vs derive body rows inside `pickerView`. **Recommendation:** pass terminal
   height; picker owns chrome math.
2. **Space on host row:** no-op (spec default) vs connect. **Recommendation:**
   no-op — Space is a tree convention; Enter connects.
3. **Shared `listWindow`:** extract to `viewport.go` or duplicate. **Recommendation:**
   extract if palette and picker budgets differ only in `maxRows` input.
