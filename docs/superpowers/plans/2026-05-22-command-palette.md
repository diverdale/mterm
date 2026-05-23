# Command Palette & Help Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `Ctrl-B p` command palette, `Ctrl-B ?` help overlay, Matrix and Synthwave themes, and a `Reload config` command on top of the existing mterm TUI.

**Architecture:** Two new view modes (`modePalette`, `modeHelp`) overlay any existing view via centered `lipgloss.Place`. A `command` type pairs a display label with an `action func(*App) tea.Cmd`; `buildCommands(*App)` produces the list dynamically. The theme switch reassigns the package-level `active`/`sty` — view code already reads `sty` each render.

**Tech Stack:** Go 1.25, Bubble Tea v1, Lip Gloss, x/ansi.

**Design source:** `docs/superpowers/specs/2026-05-22-command-palette-design.md`

---

## Conventions

- Conventional Commits. Run `gofmt -l .` and `go vet ./...` before each commit — both clean.
- Do NOT run `go mod tidy` or `go get`.
- TDD: failing test first, confirm fail, implement, confirm pass.
- This work goes on a feature branch `feat/command-palette` (created at the start).

## File Structure

```
internal/tui/
  themes.go       NEW  — Matrix, Synthwave Theme values; Themes(); setTheme
  commands.go     NEW  — command type, buildCommands(*App)
  palette.go      NEW  — paletteModel (Update/View), palette messages
  help.go         NEW  — helpModel (static View), help message
  app.go          MODIFY — modePalette/modeHelp, prevMode, prefix routing, View overlay
```

---

## Task 0: Feature branch

**Files:** none (branch creation only).

- [ ] **Step 1: Verify clean main and create the branch**

```bash
git status              # must be clean
git checkout -b feat/command-palette
git branch --show-current
```

Expected: working tree clean; branch switched to `feat/command-palette`.

---

## Task 1: Themes and runtime switcher

**Files:**
- Create: `internal/tui/themes.go`
- Test: `internal/tui/themes_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/themes_test.go`:

```go
package tui

import (
	"reflect"
	"testing"
)

func TestThemesIncludesAllShipped(t *testing.T) {
	got := []string{}
	for _, th := range Themes() {
		got = append(got, th.Name)
	}
	want := []string{"midnight", "matrix", "synthwave"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Themes() names = %v, want %v", got, want)
	}
}

func TestSetThemeSwitchesActiveAndStyles(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })

	setTheme(Matrix)
	if active.Name != "matrix" {
		t.Fatalf("active.Name = %q, want matrix", active.Name)
	}
	if got := sty.title.GetForeground(); got != Matrix.Accent {
		t.Fatalf("after setTheme(Matrix), title fg = %v, want %v", got, Matrix.Accent)
	}

	setTheme(Synthwave)
	if active.Name != "synthwave" {
		t.Fatalf("active.Name = %q, want synthwave", active.Name)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run "TestThemes|TestSetTheme" -v`
Expected: FAIL — `Matrix`, `Synthwave`, `Themes`, `setTheme` undefined.

- [ ] **Step 3: Create `internal/tui/themes.go`**

```go
package tui

import "github.com/charmbracelet/lipgloss"

// Matrix is the phosphor-green theme.
var Matrix = Theme{
	Name:      "matrix",
	Accent:    lipgloss.Color("#00FF41"),
	Frame:     lipgloss.Color("#1A1A1A"),
	Dim:       lipgloss.Color("#3A3A3A"),
	Text:      lipgloss.Color("#00CC33"),
	Warning:   lipgloss.Color("#FFB347"),
	Error:     lipgloss.Color("#FF4747"),
	Connected: lipgloss.Color("#00FF41"),
}

// Synthwave is the magenta theme.
var Synthwave = Theme{
	Name:      "synthwave",
	Accent:    lipgloss.Color("#F92AAD"),
	Frame:     lipgloss.Color("#3A1A40"),
	Dim:       lipgloss.Color("#7A4A7A"),
	Text:      lipgloss.Color("#E0E0E0"),
	Warning:   lipgloss.Color("#FFD93D"),
	Error:     lipgloss.Color("#FF5C7A"),
	Connected: lipgloss.Color("#36F1CD"),
}

// Themes returns the available themes in canonical order.
func Themes() []Theme {
	return []Theme{Midnight, Matrix, Synthwave}
}

// setTheme switches the active theme and rebuilds the chrome style set.
// Subsequent renders use the new colors on the next render tick.
func setTheme(t Theme) {
	active = t
	sty = buildStyles(active)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestThemes|TestSetTheme" -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/themes.go internal/tui/themes_test.go
git commit -m "feat: add Matrix and Synthwave themes plus runtime switcher"
```

---

## Task 2: Help overlay model

**Files:**
- Create: `internal/tui/help.go`
- Test: `internal/tui/help_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/help_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpViewListsAllPrefixKeys(t *testing.T) {
	h := newHelpModel()
	v := h.View()
	for _, want := range []string{"^B c", "^B n", "^B p", "^B x", "^B f", "^B ?", "^B q", "1..9"} {
		if !strings.Contains(v, want) {
			t.Fatalf("help missing %q in view:\n%s", want, v)
		}
	}
}

func TestHelpUpdateDismissesOnAnyKey(t *testing.T) {
	h := newHelpModel()
	cmd := h.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("help Update should return a non-nil cmd")
	}
	if _, ok := cmd().(helpClosedMsg); !ok {
		t.Fatalf("cmd() = %T, want helpClosedMsg", cmd())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestHelp -v`
Expected: FAIL — `newHelpModel`, `helpClosedMsg` undefined.

- [ ] **Step 3: Create `internal/tui/help.go`**

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpClosedMsg dismisses the help overlay.
type helpClosedMsg struct{}

// helpModel is the static keybinding help overlay.
type helpModel struct{}

// newHelpModel constructs the help overlay model.
func newHelpModel() *helpModel { return &helpModel{} }

// Update dismisses the overlay on any key (Esc or otherwise — the overlay
// holds no internal state worth navigating).
func (h *helpModel) Update(_ tea.KeyMsg) tea.Cmd {
	return func() tea.Msg { return helpClosedMsg{} }
}

type kb struct{ key, desc string }

var helpSections = []struct {
	name  string
	binds []kb
}{
	{"Prefix", []kb{
		{"^B c", "open picker / new connection"},
		{"^B n / ^B p", "next / previous tab"},
		{"^B 1..9", "jump to tab N"},
		{"^B x", "close current tab"},
		{"^B f", "open forwards panel"},
		{"^B p", "open command palette"},
		{"^B ?", "this help"},
		{"^B q", "quit mterm"},
	}},
	{"Picker", []kb{
		{"type", "filter hosts"},
		{"↑ / ↓", "move cursor"},
		{"enter", "connect"},
		{"esc", "back to active session"},
	}},
	{"Forwards panel", []kb{
		{"space / enter", "toggle"},
		{"esc", "close"},
	}},
	{"Palette", []kb{
		{"type", "filter commands"},
		{"↑ / ↓", "move cursor"},
		{"enter", "run"},
		{"esc", "cancel"},
	}},
	{"Global", []kb{
		{"^C", "quit mterm"},
	}},
}

// View renders the keybinding table inside a rounded modal box.
func (h *helpModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("mterm — keybindings"))
	b.WriteString("\n")
	for _, sec := range helpSections {
		b.WriteString("\n")
		b.WriteString(sty.groupHeader.Render("  " + sec.name))
		b.WriteString("\n")
		for _, k := range sec.binds {
			b.WriteString(fmt.Sprintf("    %s  %s\n",
				sty.footerKey.Render(padRight(k.key, 14)),
				sty.dim.Render(k.desc)))
		}
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("           esc to dismiss"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2)
	return box.Render(b.String())
}

// padRight pads s with spaces on the right to width n; if s is already wider,
// it is returned unchanged.
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestHelp -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/help.go internal/tui/help_test.go
git commit -m "feat: keybinding help overlay"
```

---

## Task 3: Command type and buildCommands

**Files:**
- Create: `internal/tui/commands.go`
- Test: `internal/tui/commands_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/commands_test.go`:

```go
package tui

import (
	"testing"

	"mterm/internal/config"
)

func TestBuildCommandsCounts(t *testing.T) {
	app := newTestApp() // sampleHosts() has 3 hosts; no open tabs
	cmds := buildCommands(app)

	counts := map[string]int{}
	for _, c := range cmds {
		counts[c.group]++
	}
	// Navigation: open picker, open forwards, next, previous, close, quit (6).
	// + 0 "Switch:" entries (no open tabs).
	if counts["Navigation"] != 6 {
		t.Fatalf("Navigation count = %d, want 6", counts["Navigation"])
	}
	if counts["Connect"] != len(app.picker.all) {
		t.Fatalf("Connect count = %d, want %d", counts["Connect"], len(app.picker.all))
	}
	if counts["Theme"] != len(Themes()) {
		t.Fatalf("Theme count = %d, want %d", counts["Theme"], len(Themes()))
	}
	if counts["Config"] != 1 {
		t.Fatalf("Config count = %d, want 1", counts["Config"])
	}
}

func TestBuildCommandsAddsSwitchEntriesPerOpenTab(t *testing.T) {
	app := newTestApp()
	app.tabs = []*sessionTab{
		newSessionTab(1, config.Host{Name: "a"}, 80, 24),
		newSessionTab(2, config.Host{Name: "b"}, 80, 24),
	}
	cmds := buildCommands(app)
	switches := 0
	for _, c := range cmds {
		if c.group == "Navigation" && len(c.label) >= 8 && c.label[:8] == "Switch: " {
			switches++
		}
	}
	if switches != 2 {
		t.Fatalf("Switch: count = %d, want 2", switches)
	}
}

func TestThemeCommandActionChangesActive(t *testing.T) {
	t.Cleanup(func() { setTheme(Midnight) })
	app := newTestApp()
	cmds := buildCommands(app)
	var matrix *command
	for i := range cmds {
		if cmds[i].label == "Theme: matrix" {
			matrix = &cmds[i]
			break
		}
	}
	if matrix == nil {
		t.Fatal("Theme: matrix command not found")
	}
	matrix.action(app)
	if active.Name != "matrix" {
		t.Fatalf("after running Theme: matrix action, active = %q", active.Name)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run "TestBuildCommands|TestThemeCommand" -v`
Expected: FAIL — `buildCommands`, `command` undefined.

- [ ] **Step 3: Create `internal/tui/commands.go`**

```go
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// command is one entry in the palette: a display label, a group header label,
// and the action to run when the user picks it.
type command struct {
	label  string
	group  string
	action func(a *App) tea.Cmd
}

// buildCommands assembles the v1 palette command set from the app's current
// state: navigation, direct-host connect, theme switches, and config reload.
func buildCommands(a *App) []command {
	var cmds []command

	// Navigation
	cmds = append(cmds,
		command{label: "Open picker", group: "Navigation", action: func(a *App) tea.Cmd {
			a.picker.setQuery("")
			a.mode = modePicker
			return nil
		}},
		command{label: "Open forwards panel", group: "Navigation", action: func(a *App) tea.Cmd {
			if t := a.activeTab(); t != nil {
				a.forwards = newForwardsPanel(t.host)
				a.mode = modeForwards
			}
			return nil
		}},
		command{label: "Next tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.cycleTab(1)
			return nil
		}},
		command{label: "Previous tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.cycleTab(-1)
			return nil
		}},
		command{label: "Close current tab", group: "Navigation", action: func(a *App) tea.Cmd {
			a.closeActiveTab()
			return nil
		}},
		command{label: "Quit mterm", group: "Navigation", action: func(a *App) tea.Cmd {
			return a.shutdown()
		}},
	)

	// One Switch: entry per open tab.
	for i, t := range a.tabs {
		idx, title := i, t.title()
		cmds = append(cmds, command{
			label: fmt.Sprintf("Switch: %d %s", idx+1, title),
			group: "Navigation",
			action: func(a *App) tea.Cmd {
				if idx >= 0 && idx < len(a.tabs) {
					a.active = idx
					a.mode = modeSession
				}
				return nil
			},
		})
	}

	// One Connect: entry per known host.
	for _, h := range a.picker.all {
		host := h
		cmds = append(cmds, command{
			label:  "Connect: " + host.Name,
			group:  "Connect",
			action: func(a *App) tea.Cmd { return a.openTab(host) },
		})
	}

	// One Theme: entry per available theme.
	for _, th := range Themes() {
		theme := th
		cmds = append(cmds, command{
			label: "Theme: " + theme.Name,
			group: "Theme",
			action: func(a *App) tea.Cmd {
				setTheme(theme)
				return nil
			},
		})
	}

	// Config
	cmds = append(cmds, command{
		label:  "Reload config",
		group:  "Config",
		action: func(a *App) tea.Cmd { reloadHosts(a); return nil },
	})

	return cmds
}

// reloadHosts re-reads the config files and replaces the picker's host list.
// Open tabs keep running on their existing SSH connections — only the picker
// (and the next palette build) sees the change. Warnings from config.Load go
// to a.statusMsg (last warning wins).
func reloadHosts(a *App) {
	res, err := config.Load()
	if err != nil {
		a.statusMsg = fmt.Sprintf("reload: %v", err)
		return
	}
	a.picker = newPicker(res.Hosts)
	for _, w := range res.Warnings {
		a.statusMsg = w
	}
}
```

Ensure `commands.go` imports `"mterm/internal/config"` (for `config.Load`) in addition to `fmt` and `tea`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestBuildCommands|TestThemeCommand" -v`
Expected: PASS — all three tests. The package builds cleanly with `reloadHosts` defined here.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/commands.go internal/tui/commands_test.go
git commit -m "feat: command type, buildCommands, and reloadHosts helper"
```

---

## Task 4: Palette model

**Files:**
- Create: `internal/tui/palette.go`
- Test: `internal/tui/palette_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/palette_test.go`:

```go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleCommands() []command {
	return []command{
		{label: "Foo bar", group: "G1", action: func(*App) tea.Cmd { return nil }},
		{label: "Baz qux", group: "G2", action: func(*App) tea.Cmd { return nil }},
		{label: "Foo other", group: "G1", action: func(*App) tea.Cmd { return nil }},
	}
}

func TestPaletteFilterCaseInsensitive(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.setQuery("FOO")
	v := p.visible()
	if len(v) != 2 {
		t.Fatalf("filter 'FOO' = %d, want 2", len(v))
	}
}

func TestPaletteEnterEmitsChosen(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter must return a non-nil cmd")
	}
	msg, ok := cmd().(paletteChosenMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want paletteChosenMsg", cmd())
	}
	if msg.cmd.label != "Foo bar" {
		t.Fatalf("chosen.label = %q, want Foo bar", msg.cmd.label)
	}
}

func TestPaletteEscEmitsClosed(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc must return a non-nil cmd")
	}
	if _, ok := cmd().(paletteClosedMsg); !ok {
		t.Fatalf("cmd() = %T, want paletteClosedMsg", cmd())
	}
}

func TestPaletteCursorWrapsAround(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.move(-1)
	if p.cursor != 2 {
		t.Fatalf("after move(-1) cursor = %d, want 2", p.cursor)
	}
	p.move(1)
	if p.cursor != 0 {
		t.Fatalf("after move(1) wrap cursor = %d, want 0", p.cursor)
	}
}

func TestPaletteEnterWithNoVisibleNoOps(t *testing.T) {
	p := newPaletteModel(sampleCommands())
	p.setQuery("zzznomatch")
	if cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatalf("Enter with no visible cmds returned non-nil: %T", cmd())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestPalette -v`
Expected: FAIL — `newPaletteModel`, `paletteChosenMsg`, `paletteClosedMsg` undefined.

- [ ] **Step 3: Create `internal/tui/palette.go`**

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages emitted by the palette.
type paletteChosenMsg struct{ cmd command }
type paletteClosedMsg struct{}

const (
	paletteWidth      = 60
	paletteInnerWidth = paletteWidth - 6 // 2 border + 2*2 padding
)

// paletteModel is the centered fuzzy-search command palette.
type paletteModel struct {
	all    []command
	query  string
	cursor int
}

// newPaletteModel constructs a palette from a pre-built command list.
func newPaletteModel(cmds []command) *paletteModel {
	return &paletteModel{all: cmds}
}

// visible returns the commands matching the current query (case-insensitive
// substring over label).
func (p *paletteModel) visible() []command {
	if p.query == "" {
		return p.all
	}
	q := strings.ToLower(p.query)
	out := make([]command, 0, len(p.all))
	for _, c := range p.all {
		if strings.Contains(strings.ToLower(c.label), q) {
			out = append(out, c)
		}
	}
	return out
}

// setQuery updates the search query and resets the cursor.
func (p *paletteModel) setQuery(q string) {
	p.query = q
	p.cursor = 0
}

// move advances the cursor by delta, wrapping around the visible list.
func (p *paletteModel) move(delta int) {
	v := p.visible()
	if len(v) == 0 {
		p.cursor = 0
		return
	}
	p.cursor = (p.cursor + delta + len(v)) % len(v)
}

// Update handles palette key events.
func (p *paletteModel) Update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		v := p.visible()
		if p.cursor < 0 || p.cursor >= len(v) {
			return nil
		}
		chosen := v[p.cursor]
		return func() tea.Msg { return paletteChosenMsg{cmd: chosen} }
	case tea.KeyEsc:
		return func() tea.Msg { return paletteClosedMsg{} }
	case tea.KeyUp, tea.KeyCtrlK:
		p.move(-1)
	case tea.KeyDown, tea.KeyCtrlJ:
		p.move(1)
	case tea.KeyBackspace:
		if p.query != "" {
			runes := []rune(p.query)
			p.setQuery(string(runes[:len(runes)-1]))
		}
	case tea.KeyRunes, tea.KeySpace:
		p.setQuery(p.query + string(msg.Runes))
	}
	return nil
}

// View renders the palette as a centered rounded-border modal.
func (p *paletteModel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Command Palette"))
	b.WriteString("\n\n")
	b.WriteString(sty.dim.Render("> "))
	b.WriteString(sty.search.Render(p.query))
	b.WriteString("\n\n")

	v := p.visible()
	if len(v) == 0 {
		b.WriteString(sty.dim.Render("  (no matching commands)"))
	} else {
		lastGroup := "\x00"
		for i, c := range v {
			if c.group != lastGroup {
				lastGroup = c.group
				b.WriteString(sty.groupHeader.Render("  " + c.group))
				b.WriteString("\n")
			}
			if i == p.cursor {
				b.WriteString(sty.selectionBar.Width(paletteInnerWidth).Render("> " + c.label))
			} else {
				b.WriteString(sty.dim.Render("  " + c.label))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("  enter: run   esc: cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2).
		Width(paletteWidth)
	return box.Render(b.String())
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestPalette -v`
Expected: PASS — all five palette tests.

- [ ] **Step 5: Commit the palette model**

```bash
git add internal/tui/palette.go internal/tui/palette_test.go
git commit -m "feat: command palette model (Update/View, filter, cursor)"
```

---

## Task 5: App integration

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

This is the integration task. It adds the new view modes, the `prevMode`/`palette`/`help` App fields, the prefix routing for `p` and `?`, the palette/help message handling, and the overlay in `View`.

- [ ] **Step 1: Write the failing tests** — append to `internal/tui/app_test.go`:

```go
func TestAppPrefixPOpensPalette(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	app.mode = modeSession
	app.tabs = []*sessionTab{newSessionTab(1, config.Host{Name: "a"}, 80, 24)}
	app.active = 0

	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	if app.mode != modePalette {
		t.Fatalf("mode = %v, want modePalette", app.mode)
	}
	if app.palette == nil {
		t.Fatal("palette is nil")
	}
	if app.prevMode != modeSession {
		t.Fatalf("prevMode = %v, want modeSession", app.prevMode)
	}
}

func TestAppPrefixQuestionOpensHelp(t *testing.T) {
	app := newTestApp()
	app.mode = modeSession
	app.update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if app.mode != modeHelp {
		t.Fatalf("mode = %v, want modeHelp", app.mode)
	}
	if app.help == nil {
		t.Fatal("help is nil")
	}
}

func TestAppPaletteClosedRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modePalette
	app.prevMode = modeSession
	app.update(paletteClosedMsg{})
	if app.mode != modeSession {
		t.Fatalf("after paletteClosedMsg, mode = %v, want modeSession", app.mode)
	}
}

func TestAppHelpClosedRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modeHelp
	app.prevMode = modePicker
	app.update(helpClosedMsg{})
	if app.mode != modePicker {
		t.Fatalf("after helpClosedMsg, mode = %v, want modePicker", app.mode)
	}
}

func TestAppPaletteChosenRunsActionAndRestoresPrevMode(t *testing.T) {
	app := newTestApp()
	app.mode = modePalette
	app.prevMode = modeSession

	ran := false
	c := command{label: "test", group: "G", action: func(a *App) tea.Cmd {
		ran = true
		return nil
	}}
	app.update(paletteChosenMsg{cmd: c})

	if !ran {
		t.Fatal("command action did not run")
	}
	if app.mode != modeSession {
		t.Fatalf("after running command, mode = %v, want modeSession", app.mode)
	}
}

func TestReloadHostsKeepsPickerNonNil(t *testing.T) {
	app := newTestApp()
	// reloadHosts re-reads the real ~/.ssh/config and mterm yaml. Without
	// fixture isolation we cannot assert specific content, but the call must
	// not panic and must leave a non-nil picker.
	reloadHosts(app)
	if app.picker == nil {
		t.Fatal("picker became nil after reloadHosts")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -v`
Expected: build failure on `modePalette`, `modeHelp`, `app.palette`, `app.help`, `app.prevMode` — added in Step 3.

- [ ] **Step 3: Edit `internal/tui/app.go`**

(a) Add the new view-mode constants. The current `viewMode` block looks roughly like:
```go
const (
	modePicker viewMode = iota
	modeSession
	modeForwards
)
```
Change to:
```go
const (
	modePicker viewMode = iota
	modeSession
	modeForwards
	modePalette
	modeHelp
)
```

(b) Add the new fields to the `App` struct (next to `picker`, `forwards`, etc.):
```go
	palette  *paletteModel
	help     *helpModel
	prevMode viewMode
```

(c) Add `?` and `p` cases to `handleCommandKey`. Find the `switch keyString(k) {` block and add:
```go
	case "p":
		a.prevMode = a.mode
		a.palette = newPaletteModel(buildCommands(a))
		a.mode = modePalette
	case "?":
		a.prevMode = a.mode
		a.help = newHelpModel()
		a.mode = modeHelp
```

(d) Route key events when in the new modes. In `handleKey`'s mode switch, add cases for `modePalette` and `modeHelp` BEFORE the `modeSession` case:
```go
	case modePalette:
		return a.palette.Update(k)
	case modeHelp:
		return a.help.Update(k)
```

(e) Handle the palette/help messages in `update`. Add cases to the `switch m := msg.(type)` block:
```go
	case paletteChosenMsg:
		var cmd tea.Cmd
		if m.cmd.action != nil {
			cmd = m.cmd.action(a)
		}
		a.mode = a.prevMode
		return cmd

	case paletteClosedMsg:
		a.mode = a.prevMode
		return nil

	case helpClosedMsg:
		a.mode = a.prevMode
		return nil
```

(f) Render the overlays in `View`. The current `View()` switch has cases for `modeForwards`, `modeSession`, and a default for picker. Add cases for the two new modes BEFORE the default:
```go
	case modePalette:
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.palette.View())
	case modeHelp:
		return lipgloss.Place(a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			a.help.View())
```

(g) `app.go` must import `github.com/charmbracelet/lipgloss` (used in `View`'s overlay branches). It is already imported by other files in the package, but confirm `app.go` itself has it. No other new imports.

- [ ] **Step 4: Run the full suite**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./... -race`
Expected: build clean, vet clean, gofmt empty, every package passes with no races.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat: wire palette and help overlays into App"
```

---

## Self-Review Notes

- **Spec coverage:** Themes + setTheme (Task 1), help overlay (Task 2), command type + buildCommands + reloadHosts (Task 3), palette model with filter/cursor/Enter/Esc (Task 4), app integration: modes, prevMode, prefix `p`/`?` routing, `paletteChosenMsg`/`paletteClosedMsg`/`helpClosedMsg` handling, View overlay (Task 5). All spec sections map to a task.
- **Type consistency:** `command` (label/group/action), `paletteModel`, `helpModel`, `modePalette`/`modeHelp`, `paletteChosenMsg`/`paletteClosedMsg`/`helpClosedMsg`, `setTheme`/`Themes`, `prevMode`, `reloadHosts` — used with identical names across tasks.
- **Each task builds clean on its own.** `reloadHosts` lives in `commands.go` as a package-level function, so Task 3 does not leave the package broken pending Task 5.
- **Behavior unchanged for users:** every existing keybinding, navigation path, and SSH behavior is untouched. The palette/help overlays sit on top of `App.View()` as additional modes.
