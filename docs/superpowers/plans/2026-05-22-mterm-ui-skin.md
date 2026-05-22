# mterm UI Skin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-skin mterm's TUI — full window frames, a context-aware footer, animated status spinners, styled tab chips, and a swappable `Theme` — without changing any behavior.

**Architecture:** All chrome is composed with Lip Gloss. A `theme.go` defines a `Theme` value type and derives a `styleSet` from the active theme (Midnight). New focused files render the spinner, tab strip, footer, and window frame as pure functions. `app.View()` composes them; `picker`/`forwards` provide themed body content. The embedded SSH terminal content is never recolored.

**Tech Stack:** Go 1.25, Lip Gloss (already a dependency), Bubble Tea v1.

**Design source:** `docs/superpowers/specs/2026-05-22-mterm-ui-skin-design.md`

---

## Conventions

- Module path `mterm`; package `tui`; all files under `internal/tui/`.
- Conventional Commits. Run `gofmt -l .` and `go vet ./...` before each commit — both clean.
- Do NOT run `go mod tidy` / `go get` — no new dependencies; Lip Gloss is already present.
- TDD: execute each task's steps in order.
- This is a **pure visual change** — no keybinding, navigation, or SSH behavior changes.

## File Structure

```
internal/tui/
  theme.go        NEW  — Theme type, Midnight, active theme, styleSet, buildStyles
  spinner.go      NEW  — braille status spinner
  tabstrip.go     NEW  — renders the tab chip row
  footer.go       NEW  — renders the context-aware footer line
  frame.go        NEW  — renderWindow: composes the bordered window box
  styles.go       DELETE (replaced by theme.go) — done in Task 9
  session.go      MODIFY — tabStatus + status(); chromeCols/chromeRows sizing
  picker.go       MODIFY — themed body content (selection bar, headers, tags)
  forwards.go     MODIFY — themed modal
  app.go          MODIFY — compose the window in View(); spinner tick counter
  keys.go         unchanged
```

Each new file is one focused unit. `frame.go`, `tabstrip.go`, `footer.go`, and
`spinner.go` are pure functions (inputs → string) so they are directly
testable.

---

## Task 1: Theme foundation

**Files:**
- Create: `internal/tui/theme.go`
- Test: `internal/tui/theme_test.go`

This task is **additive** — `styles.go` stays in place (the views still use it)
until Task 9 migrates them and deletes it.

- [ ] **Step 1: Write the failing test** — create `internal/tui/theme_test.go`:

```go
package tui

import "testing"

func TestMidnightIsActiveByDefault(t *testing.T) {
	if active.Name != "midnight" {
		t.Fatalf("active theme = %q, want midnight", active.Name)
	}
}

func TestBuildStylesUsesThemeColors(t *testing.T) {
	s := buildStyles(Midnight)
	if got := s.title.GetForeground(); got != Midnight.Accent {
		t.Fatalf("title foreground = %v, want accent %v", got, Midnight.Accent)
	}
	if got := s.errorText.GetForeground(); got != Midnight.Error {
		t.Fatalf("errorText foreground = %v, want error %v", got, Midnight.Error)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run "TestMidnight|TestBuildStyles" -v`
Expected: FAIL — `active`, `buildStyles`, `Midnight` undefined.

- [ ] **Step 3: Create `internal/tui/theme.go`**

```go
package tui

import "github.com/charmbracelet/lipgloss"

// Theme is a named chrome color palette. The session content is never themed;
// only mterm's own frame/tabs/footer use these colors.
type Theme struct {
	Name      string
	Accent    lipgloss.Color // active tab, host name, focus highlights
	Frame     lipgloss.Color // window borders, dividers
	Dim       lipgloss.Color // inactive tabs, secondary text, group headers
	Text      lipgloss.Color // primary foreground
	Warning   lipgloss.Color // amber
	Error     lipgloss.Color // red — connection/auth errors
	Connected lipgloss.Color // green — connected status dot
}

// Midnight is the v1 default theme: dark slate with a bright cyan accent.
var Midnight = Theme{
	Name:      "midnight",
	Accent:    lipgloss.Color("#5ED4E0"),
	Frame:     lipgloss.Color("#5C6370"),
	Dim:       lipgloss.Color("#6B7280"),
	Text:      lipgloss.Color("#D8DEE9"),
	Warning:   lipgloss.Color("#E5C07B"),
	Error:     lipgloss.Color("#E06C75"),
	Connected: lipgloss.Color("#98C379"),
}

// onAccent is the foreground used on top of an Accent background.
const onAccent = lipgloss.Color("#0B0F14")

// styleSet holds every chrome style, derived from a Theme.
type styleSet struct {
	border       lipgloss.Style // window border characters
	title        lipgloss.Style // active host name in the title
	titleDim     lipgloss.Style // "[N tabs]" count
	tabActive    lipgloss.Style // active tab chip
	tabInactive  lipgloss.Style // inactive tab chip
	footerHint   lipgloss.Style // footer hint label text
	footerKey    lipgloss.Style // footer hint key text
	footerInfo   lipgloss.Style // footer right-side info
	prefixBadge  lipgloss.Style // PREFIX badge
	selectionBar lipgloss.Style // picker cursor row
	groupHeader  lipgloss.Style // picker group header
	tag          lipgloss.Style // picker host tags
	search       lipgloss.Style // picker search query text
	errorText    lipgloss.Style // error messages
	connected    lipgloss.Style // green status glyph
	dim          lipgloss.Style // generic secondary text
	modalBorder  lipgloss.Style // forwards modal border color
}

// active is the current theme. v1 has no runtime switcher.
var active = Midnight

// sty is the chrome style set, built once from the active theme.
var sty = buildStyles(active)

// buildStyles derives the full style set from a theme. A future theme switcher
// re-runs this with a different theme.
func buildStyles(t Theme) styleSet {
	return styleSet{
		border:       lipgloss.NewStyle().Foreground(t.Frame),
		title:        lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		titleDim:     lipgloss.NewStyle().Foreground(t.Dim),
		tabActive:    lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent).Bold(true).Padding(0, 1),
		tabInactive:  lipgloss.NewStyle().Foreground(t.Dim).Padding(0, 1),
		footerHint:   lipgloss.NewStyle().Foreground(t.Dim),
		footerKey:    lipgloss.NewStyle().Foreground(t.Accent),
		footerInfo:   lipgloss.NewStyle().Foreground(t.Dim),
		prefixBadge:  lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent).Bold(true).Padding(0, 1),
		selectionBar: lipgloss.NewStyle().Background(t.Accent).Foreground(onAccent),
		groupHeader:  lipgloss.NewStyle().Foreground(t.Dim).Bold(true),
		tag:          lipgloss.NewStyle().Foreground(t.Dim),
		search:       lipgloss.NewStyle().Foreground(t.Accent),
		errorText:    lipgloss.NewStyle().Foreground(t.Error),
		connected:    lipgloss.NewStyle().Foreground(t.Connected),
		dim:          lipgloss.NewStyle().Foreground(t.Dim),
		modalBorder:  lipgloss.NewStyle().Foreground(t.Accent),
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestMidnight|TestBuildStyles" -v`
Expected: PASS — both tests.

> If `lipgloss.Style.GetForeground()` returns a `lipgloss.TerminalColor`
> interface rather than a comparable `lipgloss.Color`, adjust the test to
> compare `.GetForeground()` against `Midnight.Accent` via `==` on the
> interface value (a `lipgloss.Color` is comparable). If the comparison genuinely
> cannot be made, assert on the rendered output instead (e.g. the style applied
> to "x" is non-empty and differs between `title` and `errorText`). Keep
> `Theme`, `Midnight`, `active`, `sty`, `buildStyles` named exactly as above.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/theme.go internal/tui/theme_test.go
git commit -m "feat: theme abstraction with Midnight palette"
```

---

## Task 2: Status spinner

**Files:**
- Create: `internal/tui/spinner.go`
- Test: `internal/tui/spinner_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/spinner_test.go`:

```go
package tui

import "testing"

func TestSpinnerGlyphCycles(t *testing.T) {
	// Frame advances every 3 ticks; 10 glyphs total.
	first := spinnerGlyph(0)
	if first == "" {
		t.Fatal("spinnerGlyph(0) is empty")
	}
	// Same glyph for ticks 0,1,2; advances at tick 3.
	if spinnerGlyph(1) != first || spinnerGlyph(2) != first {
		t.Fatal("spinner must hold a glyph for 3 ticks")
	}
	if spinnerGlyph(3) == first {
		t.Fatal("spinner must advance on the 3rd tick")
	}
	// Wraps after 10 glyphs (30 ticks).
	if spinnerGlyph(30) != first {
		t.Fatalf("spinner must wrap after 10 glyphs: glyph(30)=%q first=%q", spinnerGlyph(30), first)
	}
}

func TestSpinnerHandlesNegative(t *testing.T) {
	// Must not panic on a zero/negative counter.
	_ = spinnerGlyph(-1)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestSpinner -v`
Expected: FAIL — `spinnerGlyph` undefined.

- [ ] **Step 3: Create `internal/tui/spinner.go`**

```go
package tui

// spinnerFrames are the braille spinner glyphs, in animation order.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerGlyph returns the spinner glyph for the given render-tick counter. The
// counter increments once per render tick (~33ms); the glyph advances every 3
// ticks (~100ms) so the animation is legible rather than a blur.
func spinnerGlyph(counter int) string {
	if counter < 0 {
		counter = 0
	}
	return spinnerFrames[(counter/3)%len(spinnerFrames)]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSpinner -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/spinner.go internal/tui/spinner_test.go
git commit -m "feat: braille status spinner"
```

---

## Task 3: Session tab status and chrome sizing

**Files:**
- Modify: `internal/tui/session.go`
- Modify: `internal/tui/app.go` (the one use of the old `tabBarRows` constant)
- Modify: `internal/tui/session_test.go` (body-height expectation)

This task adds a derived connection status to `sessionTab` and replaces the
single reserved tab-bar row with the full chrome reservation (2 columns, 5
rows).

- [ ] **Step 1: Write the failing test** — replace the body of
`TestSessionTabResizeUpdatesTerminal` in `internal/tui/session_test.go` and add
a status test. The current resize test expects `height-1`; it must now expect
`height - chromeRows`. Replace that test function with:

```go
func TestSessionTabResizeUpdatesTerminal(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	tab.resize(100, 30)
	w, h := tab.term.Size()
	// Body is the inner window area: width minus side borders, height minus
	// the 5 chrome rows (top border, tab strip, divider, footer, bottom border).
	if w != 100-chromeCols || h != 30-chromeRows {
		t.Fatalf("terminal size = %d,%d want %d,%d", w, h, 100-chromeCols, 30-chromeRows)
	}
}

func TestSessionTabStatus(t *testing.T) {
	tab := newSessionTab(1, config.Host{Name: "h"}, 80, 24)
	// No session attached yet → connecting.
	if got := tab.status(); got != statusConnecting {
		t.Fatalf("status with no session = %v, want statusConnecting", got)
	}
	// After the reader goroutine has marked the tab ended → failed.
	tab.ended.Store(true)
	if got := tab.status(); got != statusFailed {
		t.Fatalf("status after ended = %v, want statusFailed", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestSessionTab -v`
Expected: FAIL — `chromeCols`, `chromeRows`, `status`, `statusConnecting`, `statusFailed` undefined.

- [ ] **Step 3: Edit `internal/tui/session.go`**

Replace the `tabBarRows` constant declaration:

```go
// tabBarRows is the height reserved for the tab bar.
const tabBarRows = 1
```

with the chrome reservation constants and the status type:

```go
// chromeCols and chromeRows are the screen space the window frame reserves
// around a session's terminal body: left+right borders, and (top border, tab
// strip, divider, footer, bottom border) respectively.
const (
	chromeCols = 2
	chromeRows = 5
)

// tabStatus is a session tab's connection state, derived from the tab.
type tabStatus int

const (
	statusConnecting tabStatus = iota
	statusConnected
	statusFailed
)

// status derives the tab's connection state. A live session is connected; a
// tab whose reader goroutine has exited (the session ended or never connected)
// is failed; otherwise it is still connecting.
func (t *sessionTab) status() tabStatus {
	if t.sess.Load() != nil {
		return statusConnected
	}
	if t.ended.Load() {
		return statusFailed
	}
	return statusConnecting
}
```

Then update `newSessionTab` and `resize`: every occurrence of `h - tabBarRows`
becomes `h - chromeRows`, and the terminal width becomes `w - chromeCols`.

In `newSessionTab`, change:

```go
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	return &sessionTab{
		id:   id,
		host: host,
		term: terminal.New(w, bodyH),
		w:    w,
		h:    h,
	}
```

to:

```go
	bodyW, bodyH := h2body(w, h)
	return &sessionTab{
		id:   id,
		host: host,
		term: terminal.New(bodyW, bodyH),
		w:    w,
		h:    h,
	}
```

In `resize`, change:

```go
	t.w, t.h = w, h
	bodyH := h - tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	t.term.Resize(w, bodyH)
	if s := t.sess.Load(); s != nil {
		_ = s.Resize(w, bodyH)
	}
```

to:

```go
	t.w, t.h = w, h
	bodyW, bodyH := h2body(w, h)
	t.term.Resize(bodyW, bodyH)
	if s := t.sess.Load(); s != nil {
		_ = s.Resize(bodyW, bodyH)
	}
```

And add the helper at the end of the file:

```go
// h2body converts a full tab area (w, h) into the inner terminal body size,
// clamped to a minimum of 1×1.
func h2body(w, h int) (int, int) {
	bw, bh := w-chromeCols, h-chromeRows
	if bw < 1 {
		bw = 1
	}
	if bh < 1 {
		bh = 1
	}
	return bw, bh
}
```

- [ ] **Step 4: Edit `internal/tui/app.go`** — `openTab` computes the body
height for the connector. Find the block in `openTab`:

```go
	connect := a.connect
	w, bodyH := a.width, a.height-tabBarRows
	if bodyH < 1 {
		bodyH = 1
	}
	return func() tea.Msg {
		sess, err := connect(host, w, bodyH)
		return connectedMsg{tabID: id, sess: sess, err: err}
	}
```

and replace the size calculation so it uses `h2body`:

```go
	connect := a.connect
	bodyW, bodyH := h2body(a.width, a.height)
	return func() tea.Msg {
		sess, err := connect(host, bodyW, bodyH)
		return connectedMsg{tabID: id, sess: sess, err: err}
	}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -v`
Expected: PASS — all `tui` tests, including the updated `TestSessionTab*`.
(`go build ./...` must also succeed — `tabBarRows` is now gone; confirm no other
file references it: `grep -rn tabBarRows internal/` should return nothing.)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/session.go internal/tui/app.go internal/tui/session_test.go
git commit -m "feat: derive tab status and reserve window-chrome space"
```

---

## Task 4: Tab strip renderer

**Files:**
- Create: `internal/tui/tabstrip.go`
- Test: `internal/tui/tabstrip_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/tabstrip_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"mterm/internal/config"
)

func TestTabStripShowsAllTabs(t *testing.T) {
	tabs := []*sessionTab{
		newSessionTab(1, config.Host{Name: "prod-web"}, 80, 24),
		newSessionTab(2, config.Host{Name: "prod-db"}, 80, 24),
	}
	out := renderTabStrip(tabs, 0, 0, 80)
	if !strings.Contains(out, "prod-web") || !strings.Contains(out, "prod-db") {
		t.Fatalf("tab strip missing a host name:\n%s", out)
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Fatalf("tab strip missing tab numbers:\n%s", out)
	}
}

func TestTabStripIsOneLine(t *testing.T) {
	tabs := []*sessionTab{newSessionTab(1, config.Host{Name: "h"}, 80, 24)}
	if strings.Contains(renderTabStrip(tabs, 0, 0, 80), "\n") {
		t.Fatal("tab strip must be a single line")
	}
}

func TestTabStripEmptyTabs(t *testing.T) {
	if got := renderTabStrip(nil, 0, 0, 80); strings.Contains(got, "\n") {
		t.Fatal("empty tab strip must still be a single line")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestTabStrip -v`
Expected: FAIL — `renderTabStrip` undefined.

- [ ] **Step 3: Create `internal/tui/tabstrip.go`**

```go
package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// statusGlyph returns the colored glyph for a tab's connection status. While
// connecting it animates via the spinner counter.
func statusGlyph(s tabStatus, spinnerCounter int) string {
	switch s {
	case statusConnected:
		return sty.connected.Render("●")
	case statusFailed:
		return sty.errorText.Render("✖")
	default:
		return sty.footerKey.Render(spinnerGlyph(spinnerCounter))
	}
}

// renderTabStrip renders the one-line row of tab chips. activeIdx is the
// focused tab; spinnerCounter drives any connecting spinners. The result is
// padded/truncated to width and contains no newline.
func renderTabStrip(tabs []*sessionTab, activeIdx, spinnerCounter, width int) string {
	chips := make([]string, 0, len(tabs))
	for i, t := range tabs {
		label := fmt.Sprintf("%s %d %s", statusGlyph(t.status(), spinnerCounter), i+1, t.title())
		if i == activeIdx {
			chips = append(chips, sty.tabActive.Render(label))
		} else {
			chips = append(chips, sty.tabInactive.Render(label))
		}
	}
	strip := lipgloss.JoinHorizontal(lipgloss.Top, chips...)
	// Pad or truncate to exactly width so the frame stays aligned.
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(strip)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestTabStrip -v`
Expected: PASS — all three tests.

> If `lipgloss.Style.Width(...).Render(...)` does not hard-truncate overlong
> input, also apply `.MaxWidth(width)` (shown above) or truncate the joined
> string with `lipgloss`'s `ansi`/`truncate` helper. The contract the tests
> enforce: output is a single line. Keep the function name `renderTabStrip`
> and signature exactly.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/tabstrip.go internal/tui/tabstrip_test.go
git commit -m "feat: styled tab strip renderer"
```

---

## Task 5: Footer renderer

**Files:**
- Create: `internal/tui/footer.go`
- Test: `internal/tui/footer_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/footer_test.go`:

```go
package tui

import (
	"strings"
	"testing"
)

func sessionHints() []keyHint {
	return []keyHint{{"^B", "menu"}, {"^B n", "next"}, {"^B x", "close"}}
}

func TestFooterShowsHintsAndInfo(t *testing.T) {
	out := renderFooter(footerOpts{
		hints: sessionHints(),
		info:  "dale@host:22",
		width: 80,
	})
	if strings.Contains(out, "\n") {
		t.Fatal("footer must be a single line")
	}
	if !strings.Contains(out, "menu") || !strings.Contains(out, "next") {
		t.Fatalf("footer missing hints:\n%s", out)
	}
	if !strings.Contains(out, "dale@host:22") {
		t.Fatalf("footer missing info:\n%s", out)
	}
}

func TestFooterPrefixBadge(t *testing.T) {
	out := renderFooter(footerOpts{
		hints:         sessionHints(),
		prefixPending: true,
		width:         80,
	})
	if !strings.Contains(out, "PREFIX") {
		t.Fatalf("prefix-pending footer must show the PREFIX badge:\n%s", out)
	}
}

func TestFooterDropsInfoWhenNarrow(t *testing.T) {
	// A very narrow width must still produce one line and keep the hints.
	out := renderFooter(footerOpts{
		hints: sessionHints(),
		info:  "dale@averylonghostname.example.com:22",
		width: 24,
	})
	if strings.Contains(out, "\n") {
		t.Fatal("narrow footer must still be one line")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestFooter -v`
Expected: FAIL — `keyHint`, `footerOpts`, `renderFooter` undefined.

- [ ] **Step 3: Create `internal/tui/footer.go`**

```go
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// keyHint is one keybinding hint shown in the footer.
type keyHint struct {
	key   string // e.g. "^B n"
	label string // e.g. "next"
}

// footerOpts configures a footer line.
type footerOpts struct {
	hints         []keyHint // left-side keybinding hints
	info          string    // right-side context (user@host:port, or "N hosts")
	clock         string    // far-right clock text, e.g. "14:05"; "" to omit
	prefixPending bool       // show the PREFIX badge instead of hints
	width         int
}

// renderFooter renders the one-line footer. Hints sit on the left, info and
// clock on the right. On a narrow width the right side is dropped before the
// left. The result has no newline and is exactly width wide.
func renderFooter(o footerOpts) string {
	var left string
	if o.prefixPending {
		left = sty.prefixBadge.Render("PREFIX")
	} else {
		parts := make([]string, 0, len(o.hints))
		for _, h := range o.hints {
			parts = append(parts, sty.footerKey.Render(h.key)+" "+sty.footerHint.Render(h.label))
		}
		left = strings.Join(parts, sty.footerHint.Render("  ·  "))
	}

	right := o.info
	if o.clock != "" {
		if right != "" {
			right += "  "
		}
		right += o.clock
	}
	right = sty.footerInfo.Render(right)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	// Drop the right side if it would not fit with at least one space gap.
	if leftW+rightW+1 > o.width {
		right = ""
		rightW = 0
	}
	gap := o.width - leftW - rightW
	if gap < 0 {
		gap = 0
	}
	line := left + strings.Repeat(" ", gap) + right
	// Final clamp so a too-long left side cannot break the frame width.
	return lipgloss.NewStyle().Width(o.width).MaxWidth(o.width).Render(line)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestFooter -v`
Expected: PASS — all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/footer.go internal/tui/footer_test.go
git commit -m "feat: context-aware footer renderer"
```

---

## Task 6: Window frame renderer

**Files:**
- Create: `internal/tui/frame.go`
- Test: `internal/tui/frame_test.go`

- [ ] **Step 1: Write the failing test** — create `internal/tui/frame_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestWindowHasExactGeometry(t *testing.T) {
	body := strings.Repeat("x\n", 18) // 19 lines of body content
	out := renderWindow(windowOpts{
		title:  "prod-web",
		body:   strings.TrimRight(body, "\n"),
		footer: "footer here",
		width:  80,
		height: 24,
	})
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("window has %d lines, want 24", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 80 {
			t.Fatalf("line %d width = %d, want 80", i, w)
		}
	}
}

func TestWindowShowsTitleAndFooter(t *testing.T) {
	out := renderWindow(windowOpts{
		title:  "prod-web",
		body:   "hello",
		footer: "the-footer",
		width:  60,
		height: 12,
	})
	if !strings.Contains(out, "prod-web") {
		t.Fatal("window must show the title")
	}
	if !strings.Contains(out, "the-footer") {
		t.Fatal("window must show the footer")
	}
}

func TestWindowWithTabStrip(t *testing.T) {
	out := renderWindow(windowOpts{
		title:    "prod-web",
		tabStrip: "1 prod-web",
		body:     "hello",
		footer:   "f",
		width:    60,
		height:   12,
	})
	if !strings.Contains(out, "1 prod-web") {
		t.Fatal("window must show the tab strip when provided")
	}
	if got := len(strings.Split(out, "\n")); got != 12 {
		t.Fatalf("window with tab strip has %d lines, want 12", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestWindow -v`
Expected: FAIL — `renderWindow`, `windowOpts` undefined.

- [ ] **Step 3: Create `internal/tui/frame.go`**

```go
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// windowOpts configures a framed window. tabStrip is optional ("" omits the
// tab-strip row). body must already be sized to the inner content area; the
// frame does not resize it.
type windowOpts struct {
	title    string
	tabStrip string // optional; "" for views without tabs (the picker)
	body     string
	footer   string
	width    int
	height   int
}

// renderWindow composes a bordered window: a titled top border, an optional
// tab-strip row, the body, a divider, the footer, and a bottom border. The
// result is exactly width × height.
func renderWindow(o windowOpts) string {
	if o.width < 4 {
		o.width = 4
	}
	if o.height < 4 {
		o.height = 4
	}
	inner := o.width - 2 // content width between the side borders

	var rows []string
	rows = append(rows, topBorder(o.title, o.width))
	if o.tabStrip != "" {
		rows = append(rows, sideRow(o.tabStrip, inner))
	}

	// Body fills whatever vertical space is left between the rows already
	// placed and the divider+footer+bottom border (3 rows).
	used := len(rows) + 3
	bodyH := o.height - used
	if bodyH < 1 {
		bodyH = 1
	}
	for _, ln := range fitLines(o.body, inner, bodyH) {
		rows = append(rows, sideRow(ln, inner))
	}

	rows = append(rows, divider(o.width))
	rows = append(rows, sideRow(o.footer, inner))
	rows = append(rows, bottomBorder(o.width))
	return strings.Join(rows, "\n")
}

// sideRow wraps one inner line in left/right border characters, padding the
// content to exactly innerWidth.
func sideRow(content string, innerWidth int) string {
	content = lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(content)
	bar := sty.border.Render("│")
	return bar + content + bar
}

// topBorder builds "┌─ title ─────┐" exactly width wide.
func topBorder(title string, width int) string {
	styled := sty.title.Render(title)
	// "┌─ " + title + " " + fill + "┐"
	prefix := "┌─ "
	fixed := lipgloss.Width(prefix) + lipgloss.Width(styled) + 2 // +1 space +1 "┐"
	fill := width - fixed
	if fill < 0 {
		fill = 0
	}
	line := sty.border.Render(prefix) + styled + sty.border.Render(" "+strings.Repeat("─", fill)+"┐")
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// divider builds "├────────┤".
func divider(width int) string {
	mid := width - 2
	if mid < 0 {
		mid = 0
	}
	return sty.border.Render("├" + strings.Repeat("─", mid) + "┤")
}

// bottomBorder builds "└────────┘".
func bottomBorder(width int) string {
	mid := width - 2
	if mid < 0 {
		mid = 0
	}
	return sty.border.Render("└" + strings.Repeat("─", mid) + "┘")
}

// fitLines splits s into lines and returns exactly want lines, each rendered to
// exactly width (padded/truncated). Short input is padded with blank lines;
// long input is truncated.
func fitLines(s string, width, want int) []string {
	src := strings.Split(s, "\n")
	out := make([]string, want)
	pad := lipgloss.NewStyle().Width(width).MaxWidth(width)
	for i := 0; i < want; i++ {
		if i < len(src) {
			out[i] = pad.Render(src[i])
		} else {
			out[i] = pad.Render("")
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestWindow -v`
Expected: PASS — all three tests.

> The geometry tests are strict: every line must be exactly `width` display
> columns and there must be exactly `height` lines. If `lipgloss.Style.Width`
> pads but does not truncate (or vice versa), combine `Width` + `MaxWidth` as
> shown, or truncate explicitly. The title is styled (contains ANSI), so all
> width math MUST use `lipgloss.Width()`, never `len()`. Adjust the internals
> if needed, but keep `renderWindow`/`windowOpts` exactly as specified — the
> tests are the contract.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/frame.go internal/tui/frame_test.go
git commit -m "feat: window frame renderer"
```

---

## Task 7: Picker themed body

**Files:**
- Modify: `internal/tui/picker.go`
- Modify: `internal/tui/picker_test.go`

The picker's `View()` currently renders a full screen (search line + grouped
list + help footer). It must now render only the **body content** (search line
+ grouped list) — the frame and footer are added by `app.View()` in Task 9.
The body gets themed: a selection bar on the cursor row, themed group headers,
themed tags.

- [ ] **Step 1: Update the picker tests** — in `internal/tui/picker_test.go`,
the existing tests call `p.View()`. They must still pass. Add a styling test;
replace the file's final test region by appending:

```go
func TestPickerViewHasSelectionAndGroups(t *testing.T) {
	p := newPicker(sampleHosts())
	p.setSize(80, 24)
	out := p.View()
	// Group headers from sampleHosts: "production" and "lab".
	if !strings.Contains(out, "production") || !strings.Contains(out, "lab") {
		t.Fatalf("picker body missing group headers:\n%s", out)
	}
	// The selected host's row must carry styling (ANSI escape codes), since
	// the cursor row is a selection bar.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("picker body has no styling at all:\n%s", out)
	}
}
```

Ensure `picker_test.go` imports `strings` (add it to the import block if not
already present).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestPickerViewHasSelection -v`
Expected: FAIL — the current plain `View()` produces no ANSI styling.

- [ ] **Step 3: Rewrite `View()` in `internal/tui/picker.go`**

Replace the entire `View()` method with the themed, body-only version:

```go
// View renders the picker body: a search line and the grouped, filtered host
// list. The window frame and footer are added by the app.
func (p *picker) View() string {
	var b strings.Builder
	b.WriteString(sty.dim.Render("Search: "))
	b.WriteString(sty.search.Render(p.query))
	b.WriteString("\n\n")

	v := p.visibleHosts()
	if len(v) == 0 {
		b.WriteString(sty.dim.Render("  (no matching hosts)"))
		return b.String()
	}

	lastGroup := "\x00"
	for i, h := range v {
		if h.Group != lastGroup {
			lastGroup = h.Group
			group := h.Group
			if group == "" {
				group = "(ungrouped)"
			}
			b.WriteString(sty.groupHeader.Render("  " + group))
			b.WriteString("\n")
		}
		row := formatHostRow(h)
		if i == p.cursor {
			b.WriteString(sty.selectionBar.Render("> " + row))
		} else {
			b.WriteString(sty.dim.Render("  ") + row)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatHostRow renders one host's name, hostname, and tags.
func formatHostRow(h config.Host) string {
	row := fmt.Sprintf("%-16s %-18s", h.Name, h.HostName)
	if len(h.Tags) > 0 {
		row += sty.tag.Render("  [" + strings.Join(h.Tags, ",") + "]")
	}
	return row
}
```

The old `View()` referenced `statusBar` (from `styles.go`); the new one does
not. `picker.go` still imports `fmt`, `strings`, `tea`, and `config` — all
still used. Do not change any other method in `picker.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestPicker -v`
Expected: PASS — all picker tests, including the new styling test.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/picker.go internal/tui/picker_test.go
git commit -m "feat: themed picker body with selection bar"
```

---

## Task 8: Themed forwards modal

**Files:**
- Modify: `internal/tui/forwards.go`
- Modify: `internal/tui/forwards_test.go`

- [ ] **Step 1: Add a styling test** — append to `internal/tui/forwards_test.go`:

```go
func TestForwardsPanelIsThemed(t *testing.T) {
	host := config.Host{
		Name:     "h",
		Forwards: []config.Forward{{Type: config.ForwardLocal, BindPort: 8080}},
	}
	panel := newForwardsPanel(host)
	out := panel.View()
	// Themed output carries ANSI escape codes.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("forwards panel has no styling:\n%s", out)
	}
	// Still shows the rounded modal border.
	if !strings.Contains(out, "╭") && !strings.Contains(out, "─") {
		t.Fatalf("forwards panel lost its border:\n%s", out)
	}
}
```

Ensure `forwards_test.go` imports `strings` (add if missing).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run TestForwardsPanelIsThemed -v`
Expected: FAIL or PASS-by-accident — if it fails, it is because the old
`modalBox`/`statusBar` styling differs. Proceed to make the panel theme-driven.

- [ ] **Step 3: Update `View()` in `internal/tui/forwards.go`**

Replace the `View()` method so it builds styles from the theme instead of the
old `statusBar`/`modalBox` package vars:

```go
// View renders the forwards modal.
func (p *forwardsPanel) View() string {
	var b strings.Builder
	b.WriteString(sty.title.Render("Port forwards — " + p.host.Name))
	b.WriteString("\n\n")
	if len(p.host.Forwards) == 0 {
		b.WriteString(sty.dim.Render("  (no forwards configured for this host)"))
		b.WriteString("\n")
	}
	for i, f := range p.host.Forwards {
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		mark := sty.dim.Render("[ ]")
		if p.on[i] {
			mark = sty.connected.Render("[x]")
		}
		line := fmt.Sprintf("%s%s %-6s %d -> %s:%d",
			cursor, mark, f.Type.String(), f.BindPort, f.DialAddr, f.DialPort)
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(sty.dim.Render("  space: toggle   esc: close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(active.Accent).
		Padding(1, 2)
	return box.Render(b.String())
}
```

`forwards.go` must import `github.com/charmbracelet/lipgloss` (add it to the
import block). `fmt`, `strings`, `tea`, and `config` remain in use. Do not
change any other method.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestForwards -v`
Expected: PASS — all forwards tests.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/forwards.go internal/tui/forwards_test.go
git commit -m "feat: theme the forwards modal"
```

---

## Task 9: Compose the window in app.View

**Files:**
- Modify: `internal/tui/app.go`
- Delete: `internal/tui/styles.go`
- Modify: `internal/tui/app_test.go`

This task wires everything together: `App` gains a spinner tick counter,
`View()` composes the framed window for picker and session modes, the forwards
modal overlays, and the obsolete `styles.go` is removed.

- [ ] **Step 1: Update app tests** — in `internal/tui/app_test.go`, the
existing tests must still pass. Add an integration test; append:

```go
func TestAppViewIsFramed(t *testing.T) {
	app := newTestApp()
	app.width, app.height = 80, 24
	out := app.View() // picker mode
	lines := strings.Split(out, "\n")
	if len(lines) != 24 {
		t.Fatalf("framed picker view has %d lines, want 24", len(lines))
	}
	if !strings.Contains(lines[0], "┌") {
		t.Fatalf("picker view is not framed; line 0 = %q", lines[0])
	}
	if !strings.Contains(out, "mterm") {
		t.Fatal("picker frame title should say mterm")
	}
}

func TestAppSpinnerCounterAdvances(t *testing.T) {
	app := newTestApp()
	before := app.tickCount
	app.update(tickMsg{})
	if app.tickCount != before+1 {
		t.Fatalf("tickCount = %d, want %d", app.tickCount, before+1)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/tui/ -run "TestAppViewIsFramed|TestAppSpinnerCounter" -v`
Expected: FAIL — `tickCount` undefined and the view is not framed.

- [ ] **Step 3: Edit `internal/tui/app.go`**

(a) Add a `tickCount` field to the `App` struct (next to the other fields):

```go
	tickCount int // render-tick counter, drives status spinners
```

(b) In `update`, the `tickMsg` case increments it:

```go
	case tickMsg:
		a.tickCount++
		a.reapEndedTabs()
		return tea.Tick(renderInterval, func(time.Time) tea.Msg { return tickMsg{} })
```

(c) Replace `View()`, `sessionView()`, and `tabBar()` with the framed
composition. Delete the old `tabBar()` method entirely and replace `View()` /
`sessionView()` with:

```go
// View renders the current view as a framed window.
func (a *App) View() string {
	if a.width < 1 || a.height < 1 {
		return ""
	}
	switch a.mode {
	case modeForwards:
		return a.forwardsView()
	case modeSession:
		return a.sessionView()
	default:
		return a.pickerView()
	}
}

// clock returns the current HH:MM string for the footer.
func clock() string { return time.Now().Format("15:04") }

func (a *App) pickerView() string {
	a.picker.setSize(a.width-chromeCols, a.height-4)
	footer := renderFooter(footerOpts{
		hints: []keyHint{{"enter", "connect"}, {"type", "filter"}, {"esc", "back"}},
		info:  fmt.Sprintf("%d hosts", len(a.hostsForFooter())),
		clock: clock(),
		width: a.width - 2,
	})
	body := a.picker.View()
	if a.statusMsg != "" {
		body += "\n" + sty.errorText.Render("  "+a.statusMsg)
	}
	return renderWindow(windowOpts{
		title:  "mterm",
		body:   body,
		footer: footer,
		width:  a.width,
		height: a.height,
	})
}

func (a *App) sessionView() string {
	t := a.activeTab()
	if t == nil {
		return a.pickerView()
	}
	title := fmt.Sprintf("%s %s", statusGlyph(t.status(), a.tickCount), t.title())
	tabs := renderTabStrip(a.tabs, a.active, a.tickCount, a.width-2)
	footer := renderFooter(footerOpts{
		hints:         []keyHint{{"^B", "menu"}, {"^B n", "next"}, {"^B x", "close"}},
		info:          sessionInfo(t.host),
		clock:         clock(),
		prefixPending: a.prefixPending,
		width:         a.width - 2,
	})
	return renderWindow(windowOpts{
		title:    title + statusCount(len(a.tabs)),
		tabStrip: tabs,
		body:     t.View(),
		footer:   footer,
		width:    a.width,
		height:   a.height,
	})
}

func (a *App) forwardsView() string {
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center,
		a.forwards.View())
}

// statusCount renders the " [N tabs]" suffix for the window title.
func statusCount(n int) string {
	noun := "tabs"
	if n == 1 {
		noun = "tab"
	}
	return sty.titleDim.Render(fmt.Sprintf("  [%d %s]", n, noun))
}

// sessionInfo renders user@host:port for the footer.
func sessionInfo(h config.Host) string {
	user := h.User
	if user == "" {
		user = "?"
	}
	return fmt.Sprintf("%s@%s:%d", user, h.HostName, h.Port)
}

// hostsForFooter returns the picker's host list for the footer count.
func (a *App) hostsForFooter() []config.Host { return a.picker.all }
```

> **Notes for the implementer:**
> - `a.picker.all` is the picker's stored host slice — confirm the field name
>   in `picker.go` (`all`) and use it. If the field has a different name, use
>   that.
> - `forwardsView` centers the modal on a blank field — a true dim-behind
>   overlay is deferred (more work, not in scope).
> - `app.go` must import `github.com/charmbracelet/lipgloss` (add to imports).
> - `a.picker.setSize(...)` is called so the picker body knows its area; the
>   picker already has `setSize`.

- [ ] **Step 4: Delete `internal/tui/styles.go`**

```bash
git rm internal/tui/styles.go
```

Then build: `go build ./...`. If the compiler reports any remaining reference
to a `styles.go` symbol (`tabActive`, `tabInactive`, `statusBar`, `errorText`,
`modalBox`), fix that reference to use the theme `sty` set — but Tasks 7 and 8
should already have removed the picker and forwards uses, and this task removes
the app uses, so the build should be clean.

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: build clean, vet clean, every package's tests PASS, no races.
Also run `gofmt -l .` — must be empty.

- [ ] **Step 6: Manual smoke check**

Run `go build -o mterm . && HOME=$(mktemp -d) ./mterm` — it should print the
"no hosts" error and exit non-zero with no panic (the framed UI never starts
without hosts, but the binary must still build and run). Full interactive
verification (the actual framed UI, colors, spinner) requires a real host and
is done by the user.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat: compose framed windows with footer and spinner"
```

---

## Self-Review Notes

- **Spec coverage:** Theme abstraction (Task 1), spinner (Task 2), tab status +
  chrome sizing (Task 3), tab strip (Task 4), footer incl. PREFIX badge and
  degradation (Task 5), window frame with titled border + divider (Task 6),
  themed picker with selection bar (Task 7), themed forwards modal (Task 8),
  composition + clock + delete styles.go (Task 9). All spec sections map to a
  task.
- **Behavior unchanged:** no task touches `keys.go`, the prefix state machine,
  navigation, or SSH code. Only rendering changes.
- **Type consistency:** `chromeCols`/`chromeRows`, `tabStatus`/`status()`,
  `spinnerGlyph`, `renderTabStrip`, `keyHint`/`footerOpts`/`renderFooter`,
  `windowOpts`/`renderWindow`, `styleSet`/`sty`/`buildStyles` are used with
  identical names across tasks.
- **lipgloss caveat:** several tasks depend on `lipgloss.Style.Width`/`MaxWidth`
  padding-and-truncating to an exact width, and on `lipgloss.Width()` for
  ANSI-aware measurement. The geometry tests (Task 6) are the hard contract; if
  a lipgloss call behaves differently, the implementer adjusts the internals to
  satisfy the tests, keeping the public function signatures fixed.
